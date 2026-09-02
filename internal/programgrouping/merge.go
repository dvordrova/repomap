package programgrouping

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
)

type mergeBatch struct {
	candidates proposalSet
}

func splitMergeBatch(value mergeBatch) (mergeBatch, mergeBatch, bool) {
	if len(value.candidates.groups) < 2 {
		return mergeBatch{}, mergeBatch{}, false
	}
	middle := len(value.candidates.groups) / 2
	return mergeBatch{candidates: proposalSubset(value.candidates, value.candidates.groups[:middle])},
		mergeBatch{candidates: proposalSubset(value.candidates, value.candidates.groups[middle:])}, true
}

func proposalSubset(all proposalSet, groups []groupProposal) proposalSet {
	result := proposalSet{groups: append([]groupProposal(nil), groups...)}
	keys := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		keys[group.Key] = struct{}{}
	}
	for _, connection := range all.connections {
		if _, ok := keys[connection.FromGroupKey]; !ok {
			continue
		}
		if _, ok := keys[connection.ToGroupKey]; !ok {
			continue
		}
		result.connections = append(result.connections, connection)
	}
	return result
}

func (compilation Compilation) mergeBatchesForProvider(
	provider llm.Provider,
	candidates proposalSet,
) ([]mergeBatch, error) {
	if provider == nil {
		return nil, fmt.Errorf("program grouping: provider is required")
	}
	candidates = canonicalProposalSet(candidates)
	if len(candidates.groups) == 0 {
		return []mergeBatch{}, nil
	}
	result := make([]mergeBatch, 0)
	current := make([]groupProposal, 0)
	flush := func() {
		if len(current) == 0 {
			return
		}
		result = append(result, mergeBatch{candidates: proposalSubset(candidates, current)})
		current = current[:0]
	}
	for _, group := range candidates.groups {
		probe := append(append([]groupProposal(nil), current...), group)
		subset := proposalSubset(candidates, probe)
		fits, err := compilation.mergeRequestFits(provider, subset)
		if err != nil {
			return nil, err
		}
		if fits {
			current = probe
			continue
		}
		if len(current) == 0 {
			request, requestErr := compilation.mergeRequest(subset)
			if requestErr != nil {
				return nil, requestErr
			}
			wire, marshalErr := json.Marshal(request)
			if marshalErr != nil {
				return nil, fmt.Errorf("program grouping: encode indivisible merge candidate: %w", marshalErr)
			}
			return nil, fmt.Errorf(
				"program grouping: restored group candidate is indivisible at %d semantic JSON bytes plus prompt in the configured provider request envelope",
				len(wire),
			)
		}
		flush()
		individual := proposalSubset(candidates, []groupProposal{group})
		fits, err = compilation.mergeRequestFits(provider, individual)
		if err != nil {
			return nil, err
		}
		if !fits {
			request, requestErr := compilation.mergeRequest(individual)
			if requestErr != nil {
				return nil, requestErr
			}
			wire, marshalErr := json.Marshal(request)
			if marshalErr != nil {
				return nil, fmt.Errorf("program grouping: encode indivisible merge candidate: %w", marshalErr)
			}
			return nil, fmt.Errorf(
				"program grouping: restored group candidate is indivisible at %d semantic JSON bytes plus prompt in the configured provider request envelope",
				len(wire),
			)
		}
		current = []groupProposal{group}
	}
	flush()
	return result, nil
}

func (compilation Compilation) mergeRequestFits(provider llm.Provider, candidates proposalSet) (bool, error) {
	request, err := compilation.mergeRequest(candidates)
	if err != nil {
		return false, err
	}
	return requestFits(provider, request)
}

func (compilation Compilation) mergeRequest(candidates proposalSet) (Request, error) {
	memberIDs := make(map[string]struct{})
	for _, group := range candidates.groups {
		for _, id := range group.MemberSubjectIDs {
			memberIDs[id] = struct{}{}
		}
	}
	refs := make([]string, 0, len(memberIDs))
	for _, ref := range compilation.categorizedRefs {
		if _, included := memberIDs[compilation.subjectByRef[ref].id]; included {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return Request{}, fmt.Errorf("program grouping: merge candidates have no categorized members")
	}
	return compilation.request(phaseMerge, refs, candidates)
}

func runMergeTournament(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	initial proposalSet,
) (proposalSet, error) {
	candidates := canonicalProposalSet(initial)
	diagnostics := append([]groupindex.Diagnostic(nil), candidates.diagnostics...)
	candidates.diagnostics = nil
	if len(candidates.groups) < 2 {
		candidates.diagnostics = canonicalDiagnostics(diagnostics)
		return candidates, nil
	}
	for level := 1; ; level++ {
		batches, err := compilation.mergeBatchesForProvider(provider, candidates)
		if err != nil {
			return proposalSet{}, fmt.Errorf("program grouping: merge level %d: %w", level, err)
		}
		if len(batches) == 0 {
			return proposalSet{diagnostics: canonicalDiagnostics(diagnostics)}, nil
		}
		finalPlan, outcomes, err := llm.ExecuteAdaptiveJSONBatch(
			ctx, executor, provider, batches,
			func(plan []mergeBatch) ([]llm.Call[proposalSet], error) {
				calls := make([]llm.Call[proposalSet], len(plan))
				for position := range plan {
					request, requestErr := compilation.mergeRequest(plan[position].candidates)
					if requestErr != nil {
						return nil, requestErr
					}
					wire, encodeErr := json.Marshal(request)
					if encodeErr != nil {
						return nil, fmt.Errorf("program grouping: encode merge request: %w", encodeErr)
					}
					state, stateErr := cubeState(compilation.index.SHA256, phaseMerge, wire)
					if stateErr != nil {
						return nil, stateErr
					}
					requestCopy := request
					calls[position] = llm.Call[proposalSet]{
						State: state,
						Prompt: llm.Prompt{
							System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
						},
						Limits: limits(),
						DecodeValidate: func(raw []byte) (proposalSet, error) {
							return normalizeResponse(raw, compilation, requestCopy)
						},
					}
				}
				return calls, nil
			},
			splitMergeBatch,
		)
		if err != nil {
			return proposalSet{}, fmt.Errorf("program grouping: model merge level %d: %w", level, err)
		}
		next := proposalSet{}
		for position, outcome := range outcomes {
			namespaced := namespaceProposalSet(outcome.Value, fmt.Sprintf("m%db%d:", level, position+1))
			next.groups = append(next.groups, namespaced.groups...)
			next.connections = append(next.connections, namespaced.connections...)
			diagnostics = append(diagnostics, namespaced.diagnostics...)
		}
		next = canonicalProposalSet(next)
		diagnostics = append(diagnostics, next.diagnostics...)
		next.diagnostics = nil
		if len(finalPlan) == 1 || len(next.groups) < 2 {
			next.diagnostics = canonicalDiagnostics(diagnostics)
			return next, nil
		}
		before, err := proposalFootprint(candidates)
		if err != nil {
			return proposalSet{}, err
		}
		after, err := proposalFootprint(next)
		if err != nil {
			return proposalSet{}, err
		}
		if len(next.groups) >= len(candidates.groups) && after >= before {
			return proposalSet{}, fmt.Errorf(
				"program grouping: merge level %d made no count or byte progress; no proposal was truncated",
				level,
			)
		}
		candidates = next
	}
}

func proposalFootprint(value proposalSet) (int, error) {
	type groupFootprint struct {
		Title    string          `json:"title"`
		Summary  string          `json:"summary"`
		Lane     groupindex.Lane `json:"lane"`
		Members  []string        `json:"members"`
		Evidence []string        `json:"evidence"`
	}
	rows := make([]groupFootprint, len(value.groups))
	for position, group := range value.groups {
		rows[position] = groupFootprint{
			Title: group.Title, Summary: group.Summary, Lane: group.Lane,
			Members: group.MemberSubjectIDs, Evidence: group.EvidenceSubjectIDs,
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		left, _ := json.Marshal(rows[i])
		right, _ := json.Marshal(rows[j])
		return string(left) < string(right)
	})
	wire, err := json.Marshal(rows)
	if err != nil {
		return 0, fmt.Errorf("program grouping: encode merge progress: %w", err)
	}
	return len(wire), nil
}
