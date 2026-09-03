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
	// consolidateSampleMembers is how many member names ride along with a
	// candidate. Enough to tell "Middleware" from "Middleware tests" without
	// sending the memberships themselves.
	consolidateSampleMembers = 6
	// consolidateEnough is the number of groups a target page reads well at.
	// Reaching it stops the loop asking for further passes.
	consolidateEnough = 14
	// consolidateMaxPasses bounds the loop. Each pass is one request, and a
	// pass that does not reduce the count stops it early anyway.
	consolidateMaxPasses = 3
)

type consolidateRequest struct {
	Version    int                    `json:"version"`
	Phase      phase                  `json:"phase"`
	Target     targetWire             `json:"target"`
	Candidates []consolidateCandidate `json:"candidates"`
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
func (compilation Compilation) consolidateRequestFor(candidates proposalSet) consolidateRequest {
	request := consolidateRequest{
		Version: requestVersion, Phase: phaseConsolidate,
		Target: targetWire{
			Language: compilation.index.Target.Language,
			Kind:     compilation.index.Target.Kind,
			Name:     compilation.index.Target.Name,
			Selector: compilation.index.Target.Selector,
		},
		Candidates: make([]consolidateCandidate, 0, len(candidates.groups)),
	}
	for position, group := range candidates.groups {
		request.Candidates = append(request.Candidates, consolidateCandidate{
			Ref:     candidateRef(position),
			Title:   group.Title,
			Summary: group.Summary,
			Lane:    group.Lane,
			Members: len(group.MemberSubjectIDs),
			Sample:  compilation.sampleMemberNames(group.MemberSubjectIDs),
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
	for pass := 1; pass <= consolidateMaxPasses; pass++ {
		// The first pass always runs: two shards that saw different halves of
		// one thing propose it twice however few groups there are, and only
		// this phase can see both. Later passes are for count alone.
		if pass > 1 && len(candidates.groups) <= consolidateEnough {
			break
		}
		next, passDiagnostics, err := consolidateOnce(ctx, executor, provider, compilation, candidates)
		if err != nil {
			if ctx.Err() != nil {
				return proposalSet{}, err
			}
			diagnostics = append(diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMergeSkipped, Reason: err.Error(),
			})
			break
		}
		diagnostics = append(diagnostics, passDiagnostics...)
		if len(next.groups) >= len(candidates.groups) {
			// A pass that consolidates nothing will not consolidate anything
			// on the next try either.
			candidates = next
			break
		}
		candidates = next
	}
	candidates.diagnostics = canonicalDiagnostics(append(diagnostics, candidates.diagnostics...))
	return candidates, nil
}

func consolidateOnce(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	candidates proposalSet,
) (proposalSet, []groupindex.Diagnostic, error) {
	request := compilation.consolidateRequestFor(candidates)
	wire, err := json.Marshal(request)
	if err != nil {
		return proposalSet{}, nil, fmt.Errorf("program grouping: encode consolidation request: %w", err)
	}
	state, err := cubeState(compilation.index.SHA256, phaseConsolidate, wire)
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
		lane := group.Lane
		members := make([]string, 0)
		evidence := make([]string, 0)
		absorbed := make([]string, 0, len(group.CandidateRefs))
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
			if !lane.Valid() {
				lane = candidate.Lane
			}
			if candidate.Lane != lane {
				// A lane is decided by a member's own categories. A grouping
				// of titles may not move one.
				diagnostics = append(diagnostics, groupindex.Diagnostic{
					Kind:   diagnosticConsolidationLaneMismatch,
					Reason: fmt.Sprintf("%s is %s, group is %s", ref, candidate.Lane, lane),
				})
				continue
			}
			claimed[ref] = group.Title
			absorbed = append(absorbed, ref)
			members = append(members, candidate.MemberSubjectIDs...)
			evidence = append(evidence, candidate.EvidenceSubjectIDs...)
		}
		if len(absorbed) == 0 {
			continue
		}
		key := fmt.Sprintf("k%d", position+1)
		title, summary := group.Title, group.Summary
		if len(absorbed) == 1 {
			// One candidate on its own keeps the words its own shard chose;
			// a rename here would be a claim nothing was measured for.
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
		})
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
