package targetportfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

// Run classifies the complete candidate reservoir through a deterministic
// disjoint batch cover. If more than one batch retains targets, a separate
// closed-ref tournament chooses the global default without changing any
// retained membership. Any terminal item failure rejects the whole cube.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
) (Execution, error) {
	if err := validateCompilation(compilation); err != nil {
		return Execution{}, err
	}
	if provider == nil {
		return Execution{}, fmt.Errorf("target portfolio: provider is nil")
	}
	batches, err := classificationBatchesWithFit(compilation, func(wire []byte) (bool, error) {
		prompt := llm.Prompt{
			System: promptSystem, User: fmt.Sprintf(promptUserShape, wire), ResponseFormatJSON: true,
		}
		_, prepareErr := provider.Prepare(prompt, portfolioCallLimits())
		return requestFitResult(prepareErr)
	})
	if err != nil {
		return Execution{}, err
	}
	calls := make([]llm.Call[Selection], len(batches))
	for index, batch := range batches {
		prompt := Prompt{
			Version: PromptVersion, System: promptSystem,
			User: fmt.Sprintf(promptUserShape, batch.compilation.wire),
		}
		state, err := portfolioCallState(
			compilation, batch.compilation.wire, "classify", 1, index+1, len(batches),
		)
		if err != nil {
			return Execution{}, err
		}
		batchCompilation := batch.compilation
		calls[index] = llm.Call[Selection]{
			State: state,
			Prompt: llm.Prompt{
				System: prompt.System, User: prompt.User, ResponseFormatJSON: true,
			},
			Limits: portfolioCallLimits(),
			DecodeValidate: func(raw []byte) (Selection, error) {
				return ResolveResponse(batchCompilation, raw)
			},
		}
	}
	classificationOutcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	execution := Execution{Outcomes: append([]llm.Outcome[Selection](nil), classificationOutcomes...)}
	if err != nil {
		return execution, fmt.Errorf("target portfolio: classification batches: %w", err)
	}

	targetSet := make(map[corpus.FileID]struct{})
	positiveBatches := 0
	var soleBatchDefault corpus.FileID
	for _, outcome := range classificationOutcomes {
		if len(outcome.Value.Targets) == 0 {
			continue
		}
		positiveBatches++
		if outcome.Value.Default == nil {
			return execution, fmt.Errorf("target portfolio: classification batch retained targets without a default")
		}
		soleBatchDefault = outcome.Value.Default.FileRef
		for _, candidate := range outcome.Value.Targets {
			targetSet[candidate.FileRef] = struct{}{}
		}
	}
	if len(targetSet) == 0 {
		selection, err := restoreCompleteSelection(compilation, nil, targetSet)
		execution.Selection = selection
		return execution, err
	}

	eligibleDefaults, err := eligibleDefaultRefs(compilation, targetSet)
	if err != nil {
		return execution, err
	}
	var defaultRef corpus.FileID
	switch {
	case len(eligibleDefaults) == 1:
		defaultRef = eligibleDefaults[0]
	case positiveBatches == 1 && containsFileRef(eligibleDefaults, soleBatchDefault):
		defaultRef = soleBatchDefault
	default:
		defaultRef, execution.Outcomes, err = runDefaultTournament(
			ctx, executor, provider, compilation, eligibleDefaults, execution.Outcomes,
		)
		if err != nil {
			return execution, err
		}
	}
	selection, err := restoreCompleteSelection(compilation, &defaultRef, targetSet)
	if err != nil {
		return execution, err
	}
	execution.Selection = selection
	return execution, nil
}

func runDefaultTournament(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	refs []corpus.FileID,
	outcomes []llm.Outcome[Selection],
) (corpus.FileID, []llm.Outcome[Selection], error) {
	remaining := append([]corpus.FileID(nil), refs...)
	for round := 1; len(remaining) > 1; round++ {
		batches, err := defaultBatchesWithFit(compilation, remaining, func(wire []byte) (bool, error) {
			_, prepareErr := provider.Prepare(llm.Prompt{
				System: defaultPromptSystem, User: fmt.Sprintf(defaultPromptUserShape, wire),
				ResponseFormatJSON: true,
			}, portfolioCallLimits())
			return requestFitResult(prepareErr)
		})
		if err != nil {
			return "", outcomes, err
		}
		calls := make([]llm.Call[Selection], 0, len(batches))
		for batchIndex, batch := range batches {
			if len(batch.request.Candidates) == 1 {
				continue
			}
			prompt, err := batch.buildPrompt()
			if err != nil {
				return "", outcomes, err
			}
			state, err := portfolioCallState(
				compilation, batch.wire, "default", round, batchIndex+1, len(batches),
			)
			if err != nil {
				return "", outcomes, err
			}
			batch := batch
			calls = append(calls, llm.Call[Selection]{
				State: state, Prompt: prompt, Limits: portfolioCallLimits(),
				DecodeValidate: batch.resolve,
			})
		}
		if len(calls) == 0 {
			return "", outcomes, fmt.Errorf(
				"target portfolio: provider comparison window cannot compare any two retained defaults",
			)
		}
		roundOutcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
		outcomes = append(outcomes, roundOutcomes...)
		if err != nil {
			return "", outcomes, fmt.Errorf("target portfolio: default round %d: %w", round, err)
		}
		next := make([]corpus.FileID, 0, len(batches))
		outcomeIndex := 0
		for _, batch := range batches {
			if len(batch.request.Candidates) == 1 {
				next = append(next, batch.request.Candidates[0].FileRef)
				continue
			}
			if outcomeIndex >= len(roundOutcomes) || roundOutcomes[outcomeIndex].Value.Default == nil {
				return "", outcomes, fmt.Errorf("target portfolio: incomplete default comparison round")
			}
			next = append(next, roundOutcomes[outcomeIndex].Value.Default.FileRef)
			outcomeIndex++
		}
		if outcomeIndex != len(roundOutcomes) || len(next) >= len(remaining) {
			return "", outcomes, fmt.Errorf("target portfolio: default comparison made no complete progress")
		}
		remaining = next
	}
	if len(remaining) != 1 {
		return "", outcomes, fmt.Errorf("target portfolio: default comparison produced no winner")
	}
	return remaining[0], outcomes, nil
}

func requestFitResult(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	var resourceErr *llm.ResourceLimitError
	if errors.As(err, &resourceErr) && resourceErr.Kind == llm.ResourceLimitRequestBytes {
		return false, nil
	}
	return false, err
}

func eligibleDefaultRefs(
	compilation Compilation,
	targetSet map[corpus.FileID]struct{},
) ([]corpus.FileID, error) {
	eligible := targetSet
	if compilation.executableAuthorityBound && len(compilation.executableFileRefs) != 0 {
		eligible = make(map[corpus.FileID]struct{})
		for _, ref := range compilation.executableFileRefs {
			if _, selected := targetSet[ref]; selected {
				eligible[ref] = struct{}{}
			}
		}
		if len(eligible) == 0 {
			return nil, fmt.Errorf("target portfolio: positive selection omits exact executable authority")
		}
	}
	refs := make([]corpus.FileID, 0, len(eligible))
	for _, candidate := range compilation.Request.Candidates {
		if _, ok := eligible[candidate.FileRef]; ok {
			refs = append(refs, candidate.FileRef)
		}
	}
	return refs, nil
}

func restoreCompleteSelection(
	compilation Compilation,
	defaultRef *corpus.FileID,
	targetSet map[corpus.FileID]struct{},
) (Selection, error) {
	if err := validateCompilation(compilation); err != nil {
		return Selection{}, err
	}
	if compilation.requiredAuthorityBound {
		for _, ref := range compilation.requiredTargetFileRefs {
			if _, selected := targetSet[ref]; !selected {
				return Selection{}, fmt.Errorf("target portfolio: selection omits exact required target authority")
			}
		}
	}
	if len(targetSet) == 0 {
		if defaultRef != nil {
			return Selection{}, fmt.Errorf("target portfolio: empty selection has a default")
		}
		unclassified := make([]VisibleCandidate, len(compilation.Request.Candidates))
		for index, candidate := range compilation.Request.Candidates {
			unclassified[index] = cloneVisibleCandidate(candidate)
		}
		return Selection{Targets: []VisibleCandidate{}, Unclassified: unclassified}, nil
	}
	if defaultRef == nil {
		return Selection{}, fmt.Errorf("target portfolio: non-empty selection has no default")
	}
	if _, selected := targetSet[*defaultRef]; !selected {
		return Selection{}, fmt.Errorf("target portfolio: default is outside selected targets")
	}
	if _, err := eligibleDefaultRefs(compilation, targetSet); err != nil {
		return Selection{}, err
	}
	authority := make(map[corpus.FileID]VisibleCandidate, len(compilation.Request.Candidates))
	for _, candidate := range compilation.Request.Candidates {
		authority[candidate.FileRef] = candidate
	}
	defaultCandidate, known := authority[*defaultRef]
	if !known {
		return Selection{}, fmt.Errorf("target portfolio: default is outside candidate authority")
	}
	if compilation.executableAuthorityBound && len(compilation.executableFileRefs) != 0 &&
		!containsFileRef(compilation.executableFileRefs, *defaultRef) {
		return Selection{}, fmt.Errorf("target portfolio: positive selection requires an exact executable default")
	}
	defaultCopy := cloneVisibleCandidate(defaultCandidate)
	result := Selection{Default: &defaultCopy, Targets: make([]VisibleCandidate, 0, len(targetSet))}
	for _, candidate := range compilation.Request.Candidates {
		if _, selected := targetSet[candidate.FileRef]; selected {
			result.Targets = append(result.Targets, cloneVisibleCandidate(candidate))
		} else {
			result.Unclassified = append(result.Unclassified, cloneVisibleCandidate(candidate))
		}
	}
	if len(result.Targets)+len(result.Unclassified) != len(compilation.Request.Candidates) {
		return Selection{}, fmt.Errorf("target portfolio: result does not restore complete candidate partition")
	}
	return result, nil
}

func containsFileRef(refs []corpus.FileID, wanted corpus.FileID) bool {
	for _, ref := range refs {
		if ref == wanted {
			return true
		}
	}
	return false
}

func portfolioCallLimits() llm.Limits {
	return llm.Limits{
		MaxRequestBytes:  llm.SemanticRecordByteLimit,
		MaxResponseBytes: MaxResponseBytes,
		MaxOutputTokens:  MaxOutputTokens,
	}
}

func portfolioCallState(
	compilation Compilation,
	request []byte,
	phase string,
	round int,
	batch int,
	batchCount int,
) ([]byte, error) {
	state, err := json.Marshal(struct {
		Contract          string `json:"contract"`
		CompilationSHA256 string `json:"compilation_sha256"`
		Phase             string `json:"phase"`
		Round             int    `json:"round"`
		Batch             int    `json:"batch"`
		BatchCount        int    `json:"batch_count"`
		RequestSHA256     string `json:"request_sha256"`
	}{
		Contract: executionContract, CompilationSHA256: sha256Hex(compilation.state),
		Phase: phase, Round: round, Batch: batch, BatchCount: batchCount,
		RequestSHA256: sha256Hex(request),
	})
	if err != nil {
		return nil, fmt.Errorf("target portfolio: encode call state: %w", err)
	}
	return state, nil
}
