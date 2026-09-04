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
	// phasePartNames names a target's areas without assigning anything to
	// them, so that the question after it is a choice from a closed list.
	phasePartNames phase = "part_names"
	// consolidateSampleMembers is how many member names ride along with a
	// candidate. Enough to tell "Middleware" from "Middleware tests" without
	// sending the memberships themselves.
	consolidateSampleMembers = 6
	// consolidateEnough is the number of groups a target page reads well at.
	// Reaching it stops the loop asking for further passes. It also decides
	// whether a target gets parts at all, and at fourteen a target of ten
	// groups got none — ten equal boxes and no architecture. Eight is where
	// naming the areas starts to earn its call.
	consolidateEnough = 8
	// consolidateWindow is how many candidates one consolidation question
	// carries. Asked about 130 at once the same input returned 47, 123 and 8
	// groups on three cold draws — the whole spread of the stage lives in
	// that one question. Windows keep each question small; several passes
	// keep it honest, because candidates that landed in different windows
	// meet in a later pass once the pool has shrunk. The code decides only
	// how much to ask at a time, never what belongs with what.
	// consolidateSmallest is the fewest labels a question is ever asked for.
	// Below it a window is already small enough that halving it says more
	// about arithmetic than about the target.
	consolidateSmallest = 4
	consolidateWindow   = 40
	// consolidatePasses bounds the loop. A pass that joins nothing stops it
	// first, and so does a pool that fits one question, so this is only the
	// ceiling. Three was too few to reach the ceiling: a pass joins about
	// three candidates in ten rather than the half it is asked for, so a
	// hundred and thirty candidates came out as a hundred and seven — a
	// stable number at a useless level. Five passes of seven tenths reach
	// forty from twice that.
	consolidatePasses = 8
)

type consolidateRequest struct {
	Version    int                    `json:"version"`
	Phase      phase                  `json:"phase"`
	Target     targetWire             `json:"target"`
	Candidates []consolidateCandidate `json:"candidates"`
	// Labels is how many distinct labels this request should come back
	// with. The rule it replaces was written in units of a target — "a
	// target reads well at four to fourteen" — while the question is asked
	// of one window of candidates, so a model that obeyed it squeezed a
	// window of forty down to fourteen and a model that ignored it joined
	// nothing. Both are the same instruction read against a different
	// denominator. Counting is code's work, so code counts.
	Labels int `json:"labels"`
	// Parts is the closed list a containers request chooses from. Inventing a
	// partition of forty groups is many decisions and a model with no
	// reasoning makes many decisions badly — nineteen parts in one draw,
	// thirty-six in the next. Choosing one of five named areas is one.
	Parts []string `json:"parts,omitempty"`
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

// consolidateResponse is one flat assignment per candidate and nothing else.
// The model is asked for exactly one decision — which candidates are the same
// thing — and answers with a label; the code does the joining, the lane and
// the naming. Asking for titles and summaries in the same answer held 15 of
// 31 merges stable across three draws, and this form held 17 of 25 while
// placing all 108 candidates every time.
type consolidateResponse struct {
	Assign []consolidateAssign `json:"assign"`
}

// consolidateAssign is one candidate and where it goes. The field it comes
// back under follows the wording of the question: asked to choose from
// `parts`, three cold draws answered with "part", and asked about groups, with
// "group". All three name the same thing, and a reader that insisted on
// "cluster" threw away answers that were entirely correct.
type consolidateAssign struct {
	Ref     string `json:"ref"`
	Cluster string `json:"cluster"`
	Part    string `json:"part"`
	Group   string `json:"group"`
}

func (assign consolidateAssign) cluster() string {
	for _, named := range []string{assign.Cluster, assign.Part, assign.Group} {
		if named != "" {
			return named
		}
	}
	return ""
}

// consolidateRequestFor describes every candidate by what it is, never by whom
// it holds. Members are the code's business.
func (compilation Compilation) consolidateRequestFor(
	requestPhase phase,
	candidates proposalSet,
	parts []string,
) consolidateRequest {
	request := consolidateRequest{
		Version: requestVersion, Phase: requestPhase,
		Target: targetWire{
			Language: compilation.index.Target.Language,
			Kind:     compilation.index.Target.Kind,
			Name:     compilation.index.Target.Name,
			Selector: compilation.index.Target.Selector,
		},
		Labels:      wantedLabels(requestPhase, len(candidates.groups)),
		Parts:       parts,
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

// globalCandidateRefs moves a window's local candidate refs onto the whole
// candidate list.
func globalCandidateRefs(local []string, offset int) []string {
	result := make([]string, 0, len(local))
	for _, ref := range local {
		var position int
		if _, err := fmt.Sscanf(ref, "c%d", &position); err != nil {
			continue
		}
		result = append(result, candidateRef(offset+position-1))
	}
	return result
}

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

	// Join what separate shards proposed twice, a window at a time. Joining
	// past the truth is how thirty middlewares became four buckets, so a pass
	// stops as soon as it stops joining rather than grinding on.
	for pass := 0; pass < consolidatePasses; pass++ {
		joined, passDiagnostics, err := consolidateWindows(
			ctx, executor, provider, compilation, phaseConsolidate, candidates, nil,
		)
		diagnostics = append(diagnostics, passDiagnostics...)
		if err != nil {
			if ctx.Err() != nil {
				return proposalSet{}, err
			}
			diagnostics = append(diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMergeSkipped, Reason: err.Error(),
			})
			break
		}
		// What each pass did to the count, because the stage is judged by it:
		// a target that settles at a hundred groups and a target that settles
		// at thirty are the same code with a different number of passes, and
		// the run says which happened.
		diagnostics = append(diagnostics, groupindex.Diagnostic{
			Kind:   diagnosticConsolidationPass,
			Reason: fmt.Sprintf("%d to %d", len(candidates.groups), len(joined.groups)),
		})
		// Two windows each named a cluster "Client IP middleware" and the
		// next pass, asked, did not join them: fifty-six of chi's hundred
		// groups shared a title after four passes. Joined by code between
		// passes, so a pass starts from what the last one agreed on.
		joined = joinSameTitles(joined)
		if len(joined.groups) >= len(candidates.groups) {
			candidates = joined
			break
		}
		candidates = joined
		if len(candidates.groups) <= consolidateWindow {
			// The whole pool now fits one question, and the pass above just
			// asked it.
			break
		}
	}

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
	// The parts are the Overview level of the page, and the Overview holds
	// fourteen boxes. Asked of every group of a target through windows, the
	// answer covered twenty-three of ninety groups in one draw and none in
	// the next, because each window named parts of a fortieth of the target
	// and most came back holding one group. Ask instead about the groups the
	// first screen actually shows, which is one question and not three, and
	// what comes back covers what the reader sees. A group past the overview
	// belongs to no part, which is what standalone means.
	// The parts are named from the largest groups — they are the
	// architecture — and then every group is placed in one of them, a window
	// at a time against the same closed list. Placed only the largest forty,
	// repomap's own target of six hundred and ninety-six groups had zones
	// over thirty-seven of them and a page of cards that named no zone.
	overview := largestGroups(candidates, consolidateWindow)
	parts, err := proposePartNames(ctx, executor, provider, compilation, overview)
	if err != nil {
		return nil, []groupindex.Diagnostic{{
			Kind: diagnosticContainerSkipped, Reason: err.Error(),
		}}
	}
	merged, diagnostics, err := consolidateWindows(
		ctx, executor, provider, compilation, phaseContainers, candidates, parts,
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
	// Every window answered with the same part names, so the same name from
	// two windows is one part: the windows' clusters are joined by title.
	merged.groups = joinPartsByTitle(merged.groups)
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

// consolidateWindows asks the consolidation question over slices of the
// candidate list and returns everything, joined or not. Candidates keep the
// order they were compiled in, which is the order of the code they came from,
// so a window is a stretch of the repository and the duplicates two adjacent
// shards proposed land in it together.
func consolidateWindows(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	requestPhase phase,
	candidates proposalSet,
	parts []string,
) (proposalSet, []groupindex.Diagnostic, error) {
	if len(candidates.groups) <= consolidateWindow {
		return consolidateOnce(ctx, executor, provider, compilation, requestPhase, candidates, parts)
	}
	// The windows of one pass are independent questions, so they are asked
	// together through the batch executor and its gate. Asked one after
	// another, the grouping of repomap's own target ran fourteen minutes of
	// wall clock for twenty-six minutes of provider time — less than twice
	// parallel on a four-way pool.
	var windows []proposalSet
	var calls []llm.Call[consolidateResponse]
	for start := 0; start < len(candidates.groups); start += consolidateWindow {
		end := min(start+consolidateWindow, len(candidates.groups))
		window := proposalSet{
			groups:      candidates.groups[start:end],
			connections: candidates.connections,
		}
		call, err := consolidateCall(compilation, requestPhase, window, parts)
		if err != nil {
			return proposalSet{}, nil, err
		}
		windows = append(windows, window)
		calls = append(calls, call)
	}
	outcomes, batchErr := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	if ctx.Err() != nil {
		return proposalSet{}, nil, ctx.Err()
	}
	var diagnostics []groupindex.Diagnostic
	result := proposalSet{connections: candidates.connections}
	for position, window := range windows {
		start := position * consolidateWindow
		var joined proposalSet
		answered := position < len(outcomes) && len(outcomes[position].Value.Assign) > 0
		if answered {
			var windowDiagnostics []groupindex.Diagnostic
			joined, windowDiagnostics = applyConsolidation(window, outcomes[position].Value)
			diagnostics = append(diagnostics, windowDiagnostics...)
		} else {
			// A window that fails keeps its candidates exactly as they were.
			reason := "window was not answered"
			if batchErr != nil {
				reason = batchErr.Error()
			}
			diagnostics = append(diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMergeSkipped, Reason: reason,
			})
			joined = window
		}
		// A window numbers its candidates from one, so c1 in the second window
		// is the forty-first candidate overall. Translate before the result
		// leaves the window, or a later level resolves those refs against the
		// whole list and silently gathers the wrong groups.
		for index := range joined.groups {
			joined.groups[index].absorbed = globalCandidateRefs(joined.groups[index].absorbed, start)
		}
		namespaced := namespaceProposalSet(joined, fmt.Sprintf("w%d:", position+1))
		result.groups = append(result.groups, namespaced.groups...)
		diagnostics = append(diagnostics, namespaced.diagnostics...)
	}
	return canonicalProposalSet(result), diagnostics, nil
}

func consolidateOnce(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	requestPhase phase,
	candidates proposalSet,
	parts []string,
) (proposalSet, []groupindex.Diagnostic, error) {
	call, err := consolidateCall(compilation, requestPhase, candidates, parts)
	if err != nil {
		return proposalSet{}, nil, err
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, call)
	if err != nil {
		return proposalSet{}, nil, err
	}
	merged, diagnostics := applyConsolidation(candidates, outcome.Value)
	return merged, diagnostics, nil
}

// consolidateCall is one consolidation question as the provider is asked it,
// with its validation. A window's question is built the same way whether it
// is asked alone or in a batch with its neighbours.
func consolidateCall(
	compilation Compilation,
	requestPhase phase,
	candidates proposalSet,
	parts []string,
) (llm.Call[consolidateResponse], error) {
	request := compilation.consolidateRequestFor(requestPhase, candidates, parts)
	wire, err := json.Marshal(request)
	if err != nil {
		return llm.Call[consolidateResponse]{}, fmt.Errorf("program grouping: encode consolidation request: %w", err)
	}
	state, err := cubeState(requestPhase, wire)
	if err != nil {
		return llm.Call[consolidateResponse]{}, err
	}
	return llm.Call[consolidateResponse]{
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
			if len(decoded.Assign) == 0 {
				return consolidateResponse{}, fmt.Errorf("program grouping: consolidation assigned nothing")
			}
			if err := refuseFlatConsolidation(request, decoded); err != nil {
				return consolidateResponse{}, err
			}
			kept, err := keepNamedParts(request, decoded)
			if err != nil {
				return consolidateResponse{}, err
			}
			return kept, nil
		},
	}, nil
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

	// Gather the assignments into clusters. A candidate named twice keeps its
	// first cluster; one named for a cluster nobody else joins is a cluster of
	// one, which is the same as not being consolidated at all.
	clusterOf := make(map[string]string, len(byRef))
	var clusterOrder []string
	members := make(map[string][]string)
	for _, assign := range response.Assign {
		candidate, known := byRef[assign.Ref]
		if !known {
			diagnostics = append(diagnostics, groupindex.Diagnostic{
				Kind: diagnosticConsolidationUnknownCandidate, Reason: assign.Ref,
			})
			continue
		}
		if owner, taken := clusterOf[assign.Ref]; taken {
			diagnostics = append(diagnostics, groupindex.Diagnostic{
				Kind:   diagnosticConsolidationRepeatedCandidate,
				Reason: assign.Ref + " already in " + owner,
			})
			continue
		}
		// A lane follows from a member's own categories, so it joins the
		// cluster label rather than being decided by it: candidates of two
		// lanes under one label become one cluster per lane, and no member
		// moves.
		label := assign.cluster() + "\x00" + string(candidate.Lane)
		clusterOf[assign.Ref] = label
		if _, seen := members[label]; !seen {
			clusterOrder = append(clusterOrder, label)
		}
		members[label] = append(members[label], assign.Ref)
	}

	result := proposalSet{}
	parentOf := make(map[string]string, len(byRef))
	for position, label := range clusterOrder {
		absorbed := members[label]
		key := fmt.Sprintf("k%d", position+1)
		var subjectIDs, evidenceIDs []string
		for _, ref := range absorbed {
			candidate := byRef[ref]
			subjectIDs = append(subjectIDs, candidate.MemberSubjectIDs...)
			evidenceIDs = append(evidenceIDs, candidate.EvidenceSubjectIDs...)
			parentOf[ref] = key
		}
		// The cluster keeps the words of the largest candidate in it. The
		// model was not asked for a title, and inventing one here would be a
		// claim nothing was measured for.
		title, summary := clusterWords(byRef, absorbed)
		result.groups = append(result.groups, groupProposal{
			Key: key, Title: title, Summary: summary, Lane: byRef[absorbed[0]].Lane,
			MemberSubjectIDs:   distinctStrings(subjectIDs),
			EvidenceSubjectIDs: distinctStrings(evidenceIDs),
			absorbed:           absorbed,
		})
	}

	// Whatever the response left out survives untouched. This is the whole
	// point: a consolidation that helps with half the candidates is worth
	// keeping, and the other half is not lost for it.
	for position, group := range candidates.groups {
		ref := candidateRef(position)
		if _, taken := clusterOf[ref]; taken {
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

// clusterWords names a cluster after the largest candidate in it, which is the
// one whose words already cover the most of what the cluster holds.
func clusterWords(byRef map[string]groupProposal, absorbed []string) (string, string) {
	best := byRef[absorbed[0]]
	for _, ref := range absorbed[1:] {
		if candidate := byRef[ref]; len(candidate.MemberSubjectIDs) > len(best.MemberSubjectIDs) {
			best = candidate
		}
	}
	return best.Title, best.Summary
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

// Naming is its own cube, and the smallest one. A part gathers groups that are
// not the same thing, so it cannot borrow the name of the largest of them:
// three cold draws produced a part called "Compression middleware" holding
// fifty groups and one called "Recoverer stack formatting" holding thirty-two.
// The question here is tiny — four to eight parts, described by what they
// hold — and the answer is one title per part and nothing else.
type nameRequest struct {
	Version int        `json:"version"`
	Phase   phase      `json:"phase"`
	Target  targetWire `json:"target"`
	Parts   []namePart `json:"parts"`
}

type namePart struct {
	Ref    string   `json:"ref"`
	Lane   string   `json:"lane"`
	Groups []string `json:"groups"`
}

// nameResponse accepts the two shapes this answer arrives in. The model was
// first given the whole five-phase prompt and answered in the shape of its
// neighbours — {"assign":[{"ref","part"}]} — so every naming was refused. It
// has its own short prompt now, and the reader below still takes either.
type nameResponse struct {
	Name   []nameAssign `json:"name"`
	Assign []nameAssign `json:"assign"`
}

type nameAssign struct {
	Ref   string `json:"ref"`
	Title string `json:"title"`
	Part  string `json:"part"`
}

func (row nameAssign) title() string {
	if strings.TrimSpace(row.Title) != "" {
		return strings.TrimSpace(row.Title)
	}
	return strings.TrimSpace(row.Part)
}

func (response nameResponse) rows() []nameAssign {
	if len(response.Name) > 0 {
		return response.Name
	}
	return response.Assign
}

// namePrompt is this cube's whole instruction. A cube this small does not need
// to be told about the phases around it, and being told made it answer in
// their shape.
const namePrompt = `You name the parts of one software target.

Each part in ` + "`parts`" + ` lists the names of the groups inside it. Give every
part one title: the name a reader of this repository would use for that area,
three or four words, distinct from every other part's. A part is not the
largest group inside it — when it covers several different groups, do not
reuse one of their names.

Reply with strict json and nothing but titles:

{"name": [{"ref": "p1", "title": "Response handling middleware"}]}

No prose, no other fields in the json, one row per part.`

// nameGroupSample is how many of a part's group names travel with it. Enough
// to see what the part is, few enough to keep the question small.
const nameGroupSample = 12

// nameContainers replaces each part's borrowed title with one chosen for the
// part as a whole. A part the answer does not name keeps the title it had, so
// this can improve the page and never break it.
func nameContainers(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	set proposalSet,
) proposalSet {
	if len(set.containers) == 0 {
		return set
	}
	titleOf := make(map[string]string, len(set.groups))
	for _, group := range set.groups {
		titleOf[group.Key] = group.Title
	}
	request := nameRequest{
		Version: requestVersion, Phase: phaseNames,
		Target: targetWire{
			Language: compilation.index.Target.Language,
			Kind:     compilation.index.Target.Kind,
			Name:     compilation.index.Target.Name,
			Selector: compilation.index.Target.Selector,
		},
		Parts: make([]namePart, 0, len(set.containers)),
	}
	for position, container := range set.containers {
		part := namePart{Ref: fmt.Sprintf("p%d", position+1), Lane: string(container.Lane)}
		for _, key := range container.GroupKeys {
			if title := titleOf[key]; title != "" {
				part.Groups = append(part.Groups, title)
			}
			if len(part.Groups) == nameGroupSample {
				break
			}
		}
		request.Parts = append(request.Parts, part)
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return set
	}
	state, err := cubeStateWithPrompt(phaseNames, namePrompt, wire)
	if err != nil {
		return set
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[nameResponse]{
		State: state,
		Prompt: llm.Prompt{
			System: namePrompt, User: string(wire), ResponseFormatJSON: true,
		},
		Limits: limits(),
		DecodeValidate: func(raw []byte) (nameResponse, error) {
			var decoded nameResponse
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return nameResponse{}, fmt.Errorf("program grouping: decode part names: %w", err)
			}
			if len(decoded.rows()) == 0 {
				return nameResponse{}, fmt.Errorf("program grouping: no part was named")
			}
			return decoded, nil
		},
	})
	if err != nil {
		set.diagnostics = canonicalDiagnostics(append(set.diagnostics, groupindex.Diagnostic{
			Kind: diagnosticNamesSkipped, Reason: err.Error(),
		}))
		return set
	}
	for _, named := range outcome.Value.rows() {
		var position int
		if _, err := fmt.Sscanf(named.Ref, "p%d", &position); err != nil {
			continue
		}
		title := named.title()
		if position < 1 || position > len(set.containers) || title == "" {
			continue
		}
		set.containers[position-1].Title = title
		set.containers[position-1].Summary = title
	}
	return set
}

// wantedLabels is how many distinct labels a consolidation question of this
// size should answer with. Roughly half: enough to join the several shards
// that saw the same thing under different words, and not so few that the
// question turns into "gather these into a handful whatever they are".
// A pass that halves is repeated until the whole target fits one window, so
// a hundred candidates settle near thirty rather than at fourteen or at a
// hundred depending on how willing one draw of the model felt.
func wantedLabels(requestPhase phase, candidates int) int {
	if candidates < consolidateSmallest {
		return candidates
	}
	if requestPhase == phaseContainers {
		// A part gathers several groups, so there are far fewer parts than
		// groups — a third, the same number the rule is written in. Asked for
		// half, the model answered with half, and a target of forty groups
		// came back as twenty parts of two, which is the target again under
		// twenty new names.
		return max(consolidateSmallest, candidates/3)
	}
	return max(consolidateSmallest, (candidates+1)/2)
}

// refuseFlatConsolidation turns down an answer whose count is wrong. The count
// is the answer's other shape: a window of forty asked for twenty labels came
// back once with thirty-six and once with a single one, and a single one is
// forty unrelated things declared identical — an answer that reads as settled
// and destroys the target. Validation is the third face of the call, so it
// checks the number as well as the fields. The floor is half of what was asked
// for and not the number itself: a window that joins a little more eagerly
// than asked still says something true, while one that halves it twice is no
// longer answering the question. Too many labels needs no refusing:
// that is a window left unjoined, which the next pass takes up again.
func refuseFlatConsolidation(request consolidateRequest, response consolidateResponse) error {
	if request.Phase != phaseConsolidate {
		// Only joining has a floor. The parts question is answered well by a
		// small number — four parts for forty groups is an architecture, and
		// refusing it left three cold draws with no zones at all.
		return nil
	}
	labels := make(map[string]struct{}, len(response.Assign))
	for _, assign := range response.Assign {
		labels[assign.cluster()] = struct{}{}
	}
	if floor := max(2, request.Labels/2); len(labels) < floor {
		return fmt.Errorf(
			"program grouping: consolidation gathered %d candidates into %d labels, fewer than the %d asked for",
			len(request.Candidates), len(labels), request.Labels,
		)
	}
	return nil
}

// largestGroups is the part of a target a reader is asked about: the groups
// holding the most members, in the order they were already in. A target's
// parts are the shape of its first screen, and a group too small to appear
// there does not need one.
func largestGroups(candidates proposalSet, most int) proposalSet {
	if len(candidates.groups) <= most {
		return candidates
	}
	bySize := append([]groupProposal(nil), candidates.groups...)
	sort.SliceStable(bySize, func(left, right int) bool {
		return len(bySize[left].MemberSubjectIDs) > len(bySize[right].MemberSubjectIDs)
	})
	kept := make(map[string]struct{}, most)
	for _, group := range bySize[:most] {
		kept[group.Key] = struct{}{}
	}
	result := proposalSet{connections: candidates.connections}
	for _, group := range candidates.groups {
		if _, in := kept[group.Key]; in {
			result.groups = append(result.groups, group)
		}
	}
	return result
}

// The parts question used to ask one model call to invent a partition of forty
// groups. Three cold draws answered it with nineteen parts, thirty-six parts
// and two — because inventing a partition is not one decision, it is many, and
// a model with no reasoning makes many decisions badly. It is two cubes now:
// name the parts of this target, then choose one named part per group. The
// second is the shape the categorization phase has always used, and it is the
// steadiest phase in the run.
const partNamesPrompt = `You name the parts of one software target.

` + "`groups`" + ` lists what the target is made of. Name the few areas these
groups fall into — the parts a reader of this repository would name if asked
what the target contains: its entry points, its core, the things it talks to.
Give exactly as many as ` + "`parts`" + ` asks for, each three words or fewer,
each an area rather than one group's own name.

Reply with strict json and nothing else:

{"parts": ["request routing", "middleware chain", "route tree"]}

Return no groups, memberships, summaries, counts or prose.`

type partNamesRequest struct {
	Version int             `json:"version"`
	Phase   phase           `json:"phase"`
	Target  targetWire      `json:"target"`
	Parts   int             `json:"parts"`
	Groups  []partNameGroup `json:"groups"`
}

type partNameGroup struct {
	Title string `json:"title"`
	Lane  string `json:"lane"`
}

type partNamesResponse struct {
	Parts []string `json:"parts"`
}

// proposePartNames asks only for the names of a target's areas. Nothing is
// assigned here, so a bad answer costs a name and never a membership.
func proposePartNames(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	candidates proposalSet,
) ([]string, error) {
	request := partNamesRequest{
		Version: requestVersion, Phase: phasePartNames,
		Target: targetWire{
			Language: compilation.index.Target.Language,
			Kind:     compilation.index.Target.Kind,
			Name:     compilation.index.Target.Name,
			Selector: compilation.index.Target.Selector,
		},
		Parts:  wantedLabels(phaseContainers, len(candidates.groups)),
		Groups: make([]partNameGroup, 0, len(candidates.groups)),
	}
	for _, group := range candidates.groups {
		request.Groups = append(request.Groups, partNameGroup{
			Title: group.Title, Lane: string(group.Lane),
		})
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	state, err := cubeStateWithPrompt(phasePartNames, partNamesPrompt, wire)
	if err != nil {
		return nil, err
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[partNamesResponse]{
		State: state,
		Prompt: llm.Prompt{
			System: partNamesPrompt, User: string(wire), ResponseFormatJSON: true,
		},
		Limits: limits(),
		DecodeValidate: func(raw []byte) (partNamesResponse, error) {
			var decoded partNamesResponse
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return partNamesResponse{}, fmt.Errorf("program grouping: decode part names: %w", err)
			}
			named := make([]string, 0, len(decoded.Parts))
			seen := make(map[string]struct{}, len(decoded.Parts))
			for _, part := range decoded.Parts {
				part = strings.TrimSpace(part)
				folded := strings.ToLower(part)
				if part == "" {
					continue
				}
				if _, repeated := seen[folded]; repeated {
					continue
				}
				seen[folded] = struct{}{}
				named = append(named, part)
			}
			if len(named) < 2 {
				return partNamesResponse{}, fmt.Errorf(
					"program grouping: %d part names, and a target of one part has no parts", len(named),
				)
			}
			return partNamesResponse{Parts: named}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return outcome.Value.Parts, nil
}

// keepNamedParts drops an assignment to a part nobody named. The list was
// chosen one call earlier, so a cluster outside it is the model writing its
// own partition again, which is the freedom this phase was split up to remove.
// A group whose part is dropped belongs to none, which is what standalone
// means and is always a safe answer.
func keepNamedParts(request consolidateRequest, response consolidateResponse) (consolidateResponse, error) {
	if len(request.Parts) == 0 {
		return response, nil
	}
	named := make(map[string]string, len(request.Parts))
	for _, part := range request.Parts {
		named[strings.ToLower(strings.TrimSpace(part))] = part
	}
	kept := consolidateResponse{Assign: make([]consolidateAssign, 0, len(response.Assign))}
	for _, assign := range response.Assign {
		part, known := named[strings.ToLower(strings.TrimSpace(assign.cluster()))]
		if !known {
			continue
		}
		kept.Assign = append(kept.Assign, consolidateAssign{Ref: assign.Ref, Cluster: part})
	}
	if len(kept.Assign) == 0 {
		return consolidateResponse{}, fmt.Errorf(
			"program grouping: no group was put in any of the %d named parts", len(request.Parts),
		)
	}
	return kept, nil
}

// movedConnections carries a pool's connections onto the groups its windows
// produced. A connection whose ends landed in one group is that group talking
// to itself and goes; one whose end was in a window that failed keeps the key
// it had, which the next level resolves or drops on its own.
func movedConnections(connections []connectionProposal, renamed map[string]string) []connectionProposal {
	result := make([]connectionProposal, 0, len(connections))
	seen := make(map[[3]string]struct{}, len(connections))
	for _, connection := range connections {
		if moved, known := renamed[connection.FromGroupKey]; known {
			connection.FromGroupKey = moved
		}
		if moved, known := renamed[connection.ToGroupKey]; known {
			connection.ToGroupKey = moved
		}
		if connection.FromGroupKey == connection.ToGroupKey {
			continue
		}
		key := [3]string{connection.FromGroupKey, connection.ToGroupKey, connection.Label}
		if _, repeated := seen[key]; repeated {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, connection)
	}
	return result
}

// joinSameTitles unions groups that carry the same title in the same lane.
// Shards of one target each name what they see, and several see the same
// thing; the title they agree on is the plainest evidence there is that it is
// one thing, and a reader shown two cards with one name could not tell them
// apart anyway. Connections follow their groups. A title shared across lanes
// is left alone: a lane follows from the members' own categories, and this
// phase may not move one.
func joinSameTitles(set proposalSet) proposalSet {
	type sameThing struct {
		title string
		lane  groupindex.Lane
	}
	first := make(map[sameThing]int, len(set.groups))
	renamed := make(map[string]string)
	result := proposalSet{diagnostics: set.diagnostics}
	joined := 0
	for _, group := range set.groups {
		key := sameThing{strings.ToLower(strings.TrimSpace(group.Title)), group.Lane}
		position, seen := first[key]
		if !seen || key.title == "" {
			first[key] = len(result.groups)
			result.groups = append(result.groups, group)
			continue
		}
		into := &result.groups[position]
		into.MemberSubjectIDs = appendMissing(into.MemberSubjectIDs, group.MemberSubjectIDs)
		into.EvidenceSubjectIDs = appendMissing(into.EvidenceSubjectIDs, group.EvidenceSubjectIDs)
		into.absorbed = append(into.absorbed, group.absorbed...)
		if into.Summary == "" {
			into.Summary = group.Summary
		}
		renamed[group.Key] = into.Key
		joined++
	}
	result.connections = movedConnections(set.connections, renamed)
	if joined > 0 {
		result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
			Kind: diagnosticSameTitleJoined, Reason: fmt.Sprintf("%d", joined),
		})
	}
	return result
}

func appendMissing(into, more []string) []string {
	seen := make(map[string]struct{}, len(into))
	for _, value := range into {
		seen[value] = struct{}{}
	}
	for _, value := range more {
		if _, repeated := seen[value]; repeated {
			continue
		}
		seen[value] = struct{}{}
		into = append(into, value)
	}
	return into
}

// joinPartsByTitle unions clusters that carry one part name. A window's
// answer is namespaced to the window, so "router" from the first window and
// "router" from the second arrived as two clusters of one part.
func joinPartsByTitle(groups []groupProposal) []groupProposal {
	at := make(map[string]int, len(groups))
	result := make([]groupProposal, 0, len(groups))
	for _, group := range groups {
		key := strings.ToLower(strings.TrimSpace(group.Title))
		position, seen := at[key]
		if !seen || key == "" {
			at[key] = len(result)
			result = append(result, group)
			continue
		}
		into := &result[position]
		into.absorbed = append(into.absorbed, group.absorbed...)
		into.MemberSubjectIDs = appendMissing(into.MemberSubjectIDs, group.MemberSubjectIDs)
		into.EvidenceSubjectIDs = appendMissing(into.EvidenceSubjectIDs, group.EvidenceSubjectIDs)
	}
	return result
}
