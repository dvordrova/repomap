package programgrouping

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Consolidation asks one question — which of these candidates are the same
// thing — and lets the code do the joining.
//
// The merge it replaces asked the model to copy every membership of every
// candidate back into its answer, and rejected the answer whole if one ref was
// missing: chi's router package sent 33 candidates in a 1.35 MB request and
// lost the consolidation that way. Worse, candidates travelled in batches of
// twelve, so two shards that had both proposed "Middleware" could only be
// joined if they happened to land in the same batch. When a batch failed its
// twelve candidates stayed as they were, and the target's page grew from ten
// groups to forty-five.
//
// A list of candidate titles is small enough that every candidate goes in one
// request, so consolidation is global rather than per batch. The response
// names which candidates belong together and nothing else; the members are
// unioned here. A union cannot drop a member, so there is no answer to reject.
const (
	phaseConsolidate phase = "consolidate"
	// phaseContainers asks a different question of the same graph. "Which of
	// these are the same thing" answered twice returns the same answer twice:
	// basic auth and compression are not the same thing, and asking again
	// says so. "Which part does each belong to" is what puts them both under
	// middleware, and it is the only question a level above can be built on.
	phaseContainers phase = "containers"
	// consolidateSampleMembers is how many member names ride along with a
	// candidate. Enough to tell "Middleware" from "Middleware tests" without
	// sending the memberships themselves.
	consolidateSampleMembers = 6
	// consolidateEnough is the number of groups a target page reads well at.
	// Reaching it stops the loop asking for further passes.
	consolidateEnough = 14
)

type consolidateRequest struct {
	Version    int                    `json:"version"`
	Phase      phase                  `json:"phase"`
	Target     targetWire             `json:"target"`
	Candidates []consolidateCandidate `json:"candidates"`
	// Connections make this a graph rather than a list of names. Two
	// candidates that talk to each other constantly are usually one thing,
	// and two that share a word in their title often are not. Without them
	// the only evidence for joining is how the titles read.
	Connections []consolidateConnection `json:"connections"`
}

type consolidateConnection struct {
	FromRef      string `json:"from_ref"`
	ToRef        string `json:"to_ref"`
	SemanticKind string `json:"semantic_kind,omitempty"`
	Label        string `json:"label,omitempty"`
}

type consolidateCandidate struct {
	Ref     string          `json:"ref"`
	Title   string          `json:"title"`
	Summary string          `json:"summary"`
	Lane    groupindex.Lane `json:"lane"`
	Members int             `json:"members"`
	Sample  []string        `json:"sample_members,omitempty"`
}

type consolidateResponse struct {
	Groups []consolidateGroup `json:"groups"`
}

type consolidateGroup struct {
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	Lane          groupindex.Lane `json:"lane"`
	CandidateRefs []string        `json:"candidate_refs"`
}

// consolidateRequestFor describes every candidate by what it is, never by whom
// it holds. Members are the code's business.
func (compilation Compilation) consolidateRequestFor(
	requestPhase phase,
	candidates proposalSet,
) consolidateRequest {
	request := consolidateRequest{
		Version: requestVersion, Phase: requestPhase,
		Target: targetWire{
			Language: compilation.index.Target.Language,
			Kind:     compilation.index.Target.Kind,
			Name:     compilation.index.Target.Name,
			Selector: compilation.index.Target.Selector,
		},
		Candidates:  make([]consolidateCandidate, 0, len(candidates.groups)),
		Connections: make([]consolidateConnection, 0, len(candidates.connections)),
	}
	refByKey := make(map[string]string, len(candidates.groups))
	for position, group := range candidates.groups {
		ref := candidateRef(position)
		refByKey[group.Key] = ref
		request.Candidates = append(request.Candidates, consolidateCandidate{
			Ref:     ref,
			Title:   group.Title,
			Summary: group.Summary,
			Lane:    group.Lane,
			Members: len(group.MemberSubjectIDs),
			Sample:  compilation.sampleMemberNames(group.MemberSubjectIDs),
		})
	}
	for _, connection := range candidates.connections {
		from, fromKnown := refByKey[connection.FromGroupKey]
		to, toKnown := refByKey[connection.ToGroupKey]
		if !fromKnown || !toKnown || from == to {
			continue
		}
		request.Connections = append(request.Connections, consolidateConnection{
			FromRef: from, ToRef: to,
			SemanticKind: connection.SemanticKind, Label: connection.Label,
		})
	}
	return request
}

func candidateRef(position int) string { return fmt.Sprintf("c%d", position+1) }

// sampleMemberNames names a few of a candidate's members so the model can see
// what it holds. Names, never refs: nothing in this response selects a member.
func (compilation Compilation) sampleMemberNames(memberIDs []string) []string {
	names := make([]string, 0, consolidateSampleMembers)
	seen := make(map[string]struct{}, consolidateSampleMembers)
	for _, id := range memberIDs {
		ref := compilation.refBySubjectID[id]
		if ref == "" {
			continue
		}
		name := groupingSubjectName(compilation.subjectByRef[ref])
		if name == "" {
			continue
		}
		if _, repeated := seen[name]; repeated {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
		if len(names) == consolidateSampleMembers {
			break
		}
	}
	return names
}

func groupingSubjectName(subject subjectAuthority) string {
	if subject.object != nil {
		if subject.object.External != nil {
			name := subject.object.External.Name
			if subject.object.External.Receiver != "" {
				name = subject.object.External.Receiver + "." + name
			}
			if subject.object.External.PackagePath != "" {
				name = subject.object.External.PackagePath + "." + name
			}
			return strings.TrimSpace(name)
		}
		return subject.object.Name
	}
	if subject.pattern != nil {
		return subject.pattern.Selector
	}
	return ""
}

// runConsolidation asks until the count is one a reader can hold, a pass stops
// reducing it, or the passes run out. Every outcome keeps every candidate:
// consolidation can fail to help, and can never lose anything.
func runConsolidation(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	initial proposalSet,
) (proposalSet, error) {
	candidates := canonicalProposalSet(initial)
	diagnostics := append([]groupindex.Diagnostic(nil), candidates.diagnostics...)
	candidates.diagnostics = nil

	// One pass joins what separate shards proposed twice. Asking again would
	// keep joining, and joining past the truth is how thirty middlewares
	// became four buckets: chi's router package really does hold one group
	// per middleware file, and the second question is not "join more" but
	// "which part does each of these belong to".
	joined, passDiagnostics, err := consolidateOnce(
		ctx, executor, provider, compilation, phaseConsolidate, candidates,
	)
	if err != nil {
		if ctx.Err() != nil {
			return proposalSet{}, err
		}
		diagnostics = append(diagnostics, groupindex.Diagnostic{
			Kind: diagnosticMergeSkipped, Reason: err.Error(),
		})
		candidates.diagnostics = canonicalDiagnostics(append(diagnostics, candidates.diagnostics...))
		return candidates, nil
	}
	diagnostics = append(diagnostics, passDiagnostics...)
	candidates = joined

	candidates.diagnostics = canonicalDiagnostics(append(diagnostics, candidates.diagnostics...))
	return candidates, nil
}

// runContainers names the parts a target has once its groups are settled. It
// is the last thing that happens to them: a group put inside a part is never
// split afterwards, so a part built before splitting hid the one group worth
// splitting — chi's largest held two fifths of its target and was skipped for
// being inside a part.
func runContainers(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	set proposalSet,
) proposalSet {
	if len(set.groups) <= consolidateEnough {
		return set
	}
	containers, diagnostics := consolidateIntoContainers(ctx, executor, provider, compilation, set)
	set.containers = append(set.containers, containers...)
	set.diagnostics = canonicalDiagnostics(append(set.diagnostics, diagnostics...))
	return set
}

// consolidateIntoContainers asks the same question one level up and keeps the
// answer as a level rather than a replacement. A container names groups; it
// never selects a member, so it cannot change what a group holds.
func consolidateIntoContainers(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	candidates proposalSet,
) ([]groupindex.ContainerProposal, []groupindex.Diagnostic) {
	merged, diagnostics, err := consolidateOnce(
		ctx, executor, provider, compilation, phaseContainers, candidates,
	)
	if err != nil {
		return nil, []groupindex.Diagnostic{{
			Kind: diagnosticContainerSkipped, Reason: err.Error(),
		}}
	}
	keyByCandidate := make(map[string]string, len(candidates.groups))
	for position, group := range candidates.groups {
		keyByCandidate[candidateRef(position)] = group.Key
	}
	containers := make([]groupindex.ContainerProposal, 0, len(merged.groups))
	for _, group := range merged.groups {
		container := groupindex.ContainerProposal{
			Key: group.Key, Title: group.Title, Summary: group.Summary, Lane: group.Lane,
		}
		for _, member := range group.absorbed {
			if key, known := keyByCandidate[member]; known {
				container.GroupKeys = append(container.GroupKeys, key)
			}
		}
		containers = append(containers, container)
	}
	return containers, diagnostics
}

func consolidateOnce(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	requestPhase phase,
	candidates proposalSet,
) (proposalSet, []groupindex.Diagnostic, error) {
	request := compilation.consolidateRequestFor(requestPhase, candidates)
	wire, err := json.Marshal(request)
	if err != nil {
		return proposalSet{}, nil, fmt.Errorf("program grouping: encode consolidation request: %w", err)
	}
	state, err := cubeState(requestPhase, wire)
	if err != nil {
		return proposalSet{}, nil, err
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[consolidateResponse]{
		State: state,
		Prompt: llm.Prompt{
			System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
		},
		Limits: limits(),
		DecodeValidate: func(raw []byte) (consolidateResponse, error) {
			var decoded consolidateResponse
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return consolidateResponse{}, fmt.Errorf("program grouping: decode consolidation: %w", err)
			}
			if len(decoded.Groups) == 0 {
				return consolidateResponse{}, fmt.Errorf("program grouping: consolidation named no groups")
			}
			return decoded, nil
		},
	})
	if err != nil {
		return proposalSet{}, nil, err
	}
	merged, diagnostics := applyConsolidation(candidates, outcome.Value)
	return merged, diagnostics, nil
}

// applyConsolidation unions the members of the candidates each returned group
// names. A candidate the response does not name, or names twice, or gathers
// under a lane that is not its own, stays exactly as it was and says so.
func applyConsolidation(
	candidates proposalSet,
	response consolidateResponse,
) (proposalSet, []groupindex.Diagnostic) {
	byRef := make(map[string]groupProposal, len(candidates.groups))
	for position, group := range candidates.groups {
		byRef[candidateRef(position)] = group
	}
	var diagnostics []groupindex.Diagnostic
	claimed := make(map[string]string, len(byRef))
	parentOf := make(map[string]string, len(byRef))
	result := proposalSet{}
	for position, group := range response.Groups {
		// A part of a target is naturally several lanes at once — a router is
		// its routes and its dispatch — but a lane follows from a member's own
		// categories and nothing here may move one. So a group naming
		// candidates of several lanes becomes one group per lane under the
		// same name, instead of one group and a pile of rejects that fall back
		// out and put the count up again.
		byLane := make(map[groupindex.Lane][]string)
		var laneOrder []groupindex.Lane
		for _, ref := range group.CandidateRefs {
			candidate, known := byRef[ref]
			if !known {
				diagnostics = append(diagnostics, groupindex.Diagnostic{
					Kind: diagnosticConsolidationUnknownCandidate, Reason: ref,
				})
				continue
			}
			if owner, taken := claimed[ref]; taken {
				diagnostics = append(diagnostics, groupindex.Diagnostic{
					Kind:   diagnosticConsolidationRepeatedCandidate,
					Reason: ref + " already in " + owner,
				})
				continue
			}
			claimed[ref] = group.Title
			if _, seen := byLane[candidate.Lane]; !seen {
				laneOrder = append(laneOrder, candidate.Lane)
			}
			byLane[candidate.Lane] = append(byLane[candidate.Lane], ref)
		}
		for laneIndex, lane := range laneOrder {
			absorbed := byLane[lane]
			members := make([]string, 0)
			evidence := make([]string, 0)
			for _, ref := range absorbed {
				candidate := byRef[ref]
				members = append(members, candidate.MemberSubjectIDs...)
				evidence = append(evidence, candidate.EvidenceSubjectIDs...)
			}
			key := fmt.Sprintf("k%d-%d", position+1, laneIndex+1)
			title, summary := group.Title, group.Summary
			if len(absorbed) == 1 {
				// One candidate on its own keeps the words its own shard
				// chose; a rename here would be a claim nothing was measured
				// for.
				only := byRef[absorbed[0]]
				if title == "" {
					title = only.Title
				}
				if summary == "" {
					summary = only.Summary
				}
			}
			for _, ref := range absorbed {
				parentOf[ref] = key
			}
			result.groups = append(result.groups, groupProposal{
				Key: key, Title: title, Summary: summary, Lane: lane,
				MemberSubjectIDs: distinctStrings(members), EvidenceSubjectIDs: distinctStrings(evidence),
				absorbed: absorbed,
			})
		}
	}
	// Whatever the response left out survives untouched. This is the whole
	// point: a consolidation that helps with half the candidates is worth
	// keeping, and the other half is not lost for it.
	for position, group := range candidates.groups {
		ref := candidateRef(position)
		if _, taken := claimed[ref]; taken {
			continue
		}
		key := fmt.Sprintf("u%d", position+1)
		parentOf[ref] = key
		kept := group
		kept.Key = key
		kept.absorbed = []string{ref}
		result.groups = append(result.groups, kept)
		diagnostics = append(diagnostics, groupindex.Diagnostic{
			Kind: diagnosticConsolidationUnclaimed, Reason: group.Title,
		})
	}
	result.connections = remapConnections(candidates, parentOf)
	return canonicalProposalSet(result), diagnostics
}

// remapConnections moves every connection onto the groups its endpoints ended
// up in. Two candidates that became one group had a connection between them
// that is now a connection from a group to itself, and a group does not talk
// to itself.
func remapConnections(candidates proposalSet, parentOf map[string]string) []connectionProposal {
	keyOfCandidate := make(map[string]string, len(candidates.groups))
	for position, group := range candidates.groups {
		keyOfCandidate[group.Key] = parentOf[candidateRef(position)]
	}
	seen := make(map[[3]string]struct{}, len(candidates.connections))
	result := make([]connectionProposal, 0, len(candidates.connections))
	for _, connection := range candidates.connections {
		from, fromKnown := keyOfCandidate[connection.FromGroupKey]
		to, toKnown := keyOfCandidate[connection.ToGroupKey]
		if !fromKnown || !toKnown || from == "" || to == "" || from == to {
			continue
		}
		key := [3]string{from, to, connection.SemanticKind}
		if _, repeated := seen[key]; repeated {
			continue
		}
		seen[key] = struct{}{}
		moved := connection
		moved.FromGroupKey, moved.ToGroupKey = from, to
		result = append(result, moved)
	}
	return result
}

func distinctStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, repeated := seen[value]; repeated {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

var _ = programindex.Category("")
