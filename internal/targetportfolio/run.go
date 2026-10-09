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
// disjoint batch cover. The batches are independent: a refused classification
// answer loses only its own batch's decisions (its required native
// representatives stay targets, its native targets keep a recorded standalone
// fallback, its guidance stays unclassified) and never cancels a sibling.
// Unless exactly one target is eligible, or exactly one batch retains targets
// and chose one of them, a separate closed-ref tournament chooses the global
// default without changing any retained membership. A refused comparison
// answer, such as one naming an unknown ref or none, leaves the default
// unresolved: Selection.Default stays nil beside the retained targets and
// nothing picks one in its place. A provider, request or resource failure
// still ends the stage.
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
			System: promptSystem, User: fmt.Sprintf(promptUserShape, wire), ResponseFormatJSON: true, ResponseExample: responseExample,
		}
		_, prepareErr := llm.Prepare(provider, prompt, portfolioCallLimits())
		return requestFitResult(prepareErr)
	})
	if err != nil {
		return Execution{}, err
	}
	calls := make([]llm.Call[batchSelection], len(batches))
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
		calls[index] = llm.Call[batchSelection]{
			State: state,
			Prompt: llm.Prompt{
				System: prompt.System, User: prompt.User, ResponseFormatJSON: true, ResponseExample: responseExample,
			},
			Limits: portfolioCallLimits(),
			DecodeValidate: func(raw []byte) (batchSelection, error) {
				return resolveResponse(batchCompilation, raw)
			},
		}
	}
	responses := llm.ExecuteJSONEach(ctx, executor, provider, calls)
	execution := Execution{Outcomes: make([]llm.Outcome[Selection], 0, len(responses))}
	selections := make([]Selection, len(responses))
	for index, response := range responses {
		execution.Outcomes = append(execution.Outcomes, selectionOutcome(response.Outcome))
		if response.Err == nil {
			selections[index] = response.Outcome.Value.Selection
			continue
		}
		if err := ctx.Err(); err != nil {
			return execution, err
		}
		if !refusedAnswer(response.Outcome.ResponseRejections) {
			return execution, fmt.Errorf("target portfolio: classification batch %d: %w", index+1, response.Err)
		}
		selections[index] = refusedBatchSelection(batches[index].compilation, response.Err)
	}

	targetSet := make(map[corpus.FileID]struct{})
	positiveBatches := 0
	var soleBatchDefault *corpus.FileID
	for _, selection := range selections {
		if len(selection.Targets) == 0 {
			continue
		}
		positiveBatches++
		soleBatchDefault = nil
		if selection.Default != nil {
			ref := selection.Default.FileRef
			soleBatchDefault = &ref
		}
		for _, candidate := range selection.Targets {
			targetSet[candidate.FileRef] = struct{}{}
		}
	}
	if len(targetSet) == 0 {
		selection, err := restoreCompleteSelection(compilation, nil, targetSet)
		execution.Selection = selection
		return execution, err
	}

	eligibleDefaults := eligibleDefaultRefs(compilation, targetSet)
	var defaultRef *corpus.FileID
	switch {
	case len(eligibleDefaults) == 1:
		defaultRef = &eligibleDefaults[0]
	case positiveBatches == 1 && soleBatchDefault != nil && containsFileRef(eligibleDefaults, *soleBatchDefault):
		defaultRef = soleBatchDefault
	default:
		defaultRef, execution.Outcomes, err = runDefaultTournament(
			ctx, executor, provider, compilation, eligibleDefaults, execution.Outcomes,
		)
		if err != nil {
			return execution, err
		}
	}
	selection, err := restoreCompleteSelection(compilation, defaultRef, targetSet)
	if err != nil {
		return execution, err
	}
	var decisions []NativeDecision
	original := make(map[string]Placement)
	for _, batch := range selections {
		for _, placement := range batch.Placements {
			original[placement.Candidate.Ref] = placement
			decision := placement.Decision
			if placement.Reason != "" {
				decision = placement.Rejected
			}
			decisions = append(decisions, NativeDecision{Ref: placement.Candidate.Ref, Decision: decision})
		}
	}
	selection.Placements = nativeDecisions(compilation.native, decisions, true)
	for i := range selection.Placements {
		prior := original[selection.Placements[i].Candidate.Ref]
		selection.Placements[i].LaunchDecision = prior.LaunchDecision
		if prior.Reason != "" {
			selection.Placements[i].Reason = prior.Reason
			selection.Placements[i].Rejected = prior.Rejected
		}
	}
	execution.Selection = selection
	return execution, nil
}

// runDefaultTournament narrows the eligible refs round by round until one
// remains. The comparisons of a round are independent calls. When any of them
// is refused, the default is unresolved: it returns nil and asks no later
// round, since no remaining winner was weighed against the refused
// comparison's candidates.
func runDefaultTournament(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	refs []corpus.FileID,
	outcomes []llm.Outcome[Selection],
) (*corpus.FileID, []llm.Outcome[Selection], error) {
	remaining := append([]corpus.FileID(nil), refs...)
	for round := 1; len(remaining) > 1; round++ {
		batches, err := defaultBatchesWithFit(compilation, remaining, func(wire []byte) (bool, error) {
			_, prepareErr := llm.Prepare(provider, llm.Prompt{
				System: defaultPromptSystem, User: fmt.Sprintf(defaultPromptUserShape, wire),
				ResponseFormatJSON: true, ResponseExample: defaultResponseExample,
			}, portfolioCallLimits())
			return requestFitResult(prepareErr)
		})
		if err != nil {
			return nil, outcomes, err
		}
		calls := make([]llm.Call[Selection], 0, len(batches))
		for batchIndex, batch := range batches {
			if len(batch.request.Candidates) == 1 {
				continue
			}
			prompt, err := batch.buildPrompt()
			if err != nil {
				return nil, outcomes, err
			}
			state, err := portfolioCallState(
				compilation, batch.wire, "default", round, batchIndex+1, len(batches),
			)
			if err != nil {
				return nil, outcomes, err
			}
			batch := batch
			calls = append(calls, llm.Call[Selection]{
				State: state, Prompt: prompt, Limits: portfolioCallLimits(),
				DecodeValidate: batch.resolve,
			})
		}
		if len(calls) == 0 {
			return nil, outcomes, fmt.Errorf(
				"target portfolio: provider comparison window cannot compare any two retained defaults",
			)
		}
		results := llm.ExecuteJSONEach(ctx, executor, provider, calls)
		for _, result := range results {
			outcomes = append(outcomes, result.Outcome)
		}
		unresolved := false
		for index, result := range results {
			if result.Err == nil {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, outcomes, err
			}
			if !refusedAnswer(result.Outcome.ResponseRejections) {
				return nil, outcomes, fmt.Errorf(
					"target portfolio: default round %d comparison %d: %w", round, index+1, result.Err,
				)
			}
			unresolved = true
		}
		if unresolved {
			return nil, outcomes, nil
		}
		next := make([]corpus.FileID, 0, len(batches))
		resultIndex := 0
		for _, batch := range batches {
			if len(batch.request.Candidates) == 1 {
				next = append(next, batch.request.Candidates[0].FileRef)
				continue
			}
			next = append(next, results[resultIndex].Outcome.Value.Default.FileRef)
			resultIndex++
		}
		if len(next) >= len(remaining) {
			return nil, outcomes, fmt.Errorf("target portfolio: default comparison made no complete progress")
		}
		remaining = next
	}
	if len(remaining) != 1 {
		return nil, outcomes, fmt.Errorf("target portfolio: default comparison produced no winner")
	}
	return &remaining[0], outcomes, nil
}

func requestFitResult(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	var resourceErr *llm.ResourceLimitError
	if errors.As(err, &resourceErr) && (resourceErr.Kind == llm.ResourceLimitRequestBytes || resourceErr.Kind == llm.ResourceLimitContextTokens) {
		return false, nil
	}
	return false, err
}

// refusedAnswer reports an answer the model gave but the executor or decoder
// refused. Such a refusal is local to its classification batch or default
// comparison; every other failure keeps its own error.
func refusedAnswer(rejections []llm.ResponseRejection) bool {
	for _, rejected := range rejections {
		if rejected.Kind == "response_validation" || rejected.Kind == "response_envelope" {
			return true
		}
	}
	return false
}

// refusedBatchSelection is the batch outcome when its answer was refused:
// nothing the answer said survives. The required native representatives are
// the compilation's own authority and stay targets; native targets keep a
// standalone fallback with the recorded refusal; guidance candidates stay
// unclassified. No default is chosen for the refused batch.
func refusedBatchSelection(compilation Compilation, cause error) Selection {
	required := make(map[corpus.FileID]struct{}, len(compilation.requiredTargetFileRefs))
	for _, ref := range compilation.requiredTargetFileRefs {
		required[ref] = struct{}{}
	}
	result := Selection{
		Placements: nativeDecisions(compilation.native, nil, false),
		Targets:    []VisibleCandidate{},
	}
	for index := range result.Placements {
		result.Placements[index].Reason = "decision not received: the classification answer was refused (" + cause.Error() + ")"
	}
	for _, candidate := range compilation.Request.Candidates {
		if _, selected := required[candidate.FileRef]; selected {
			result.Targets = append(result.Targets, cloneVisibleCandidate(candidate))
			continue
		}
		result.Unclassified = append(result.Unclassified, cloneVisibleCandidate(candidate))
	}
	return result
}

// selectionOutcome is the same exchange with only its restored Selection; the
// discarded members stay in ResponseRejections.
func selectionOutcome(outcome llm.Outcome[batchSelection]) llm.Outcome[Selection] {
	return llm.Outcome[Selection]{
		ResponseContext:    outcome.ResponseContext,
		ResponseRejections: outcome.ResponseRejections,
		HTTPResponse:       outcome.HTTPResponse,
		Value:              outcome.Value.Selection,
		CacheKey:           outcome.CacheKey,
		Cached:             outcome.Cached,
		Request:            outcome.Request,
		RequestSHA256:      outcome.RequestSHA256,
		RequestBytes:       outcome.RequestBytes,
		Response:           outcome.Response,
		ResponseSHA256:     outcome.ResponseSHA256,
		ResponseBytes:      outcome.ResponseBytes,
		FinishReason:       outcome.FinishReason,
		ChoiceCount:        outcome.ChoiceCount,
		Metrics:            outcome.Metrics,
		Issues:             outcome.Issues,
	}
}

func eligibleDefaultRefs(
	compilation Compilation,
	targetSet map[corpus.FileID]struct{},
) []corpus.FileID {
	refs := make([]corpus.FileID, 0, len(targetSet))
	for _, candidate := range compilation.Request.Candidates {
		if _, ok := targetSet[candidate.FileRef]; ok {
			refs = append(refs, candidate.FileRef)
		}
	}
	return refs
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
	// A nil default beside targets is the unresolved default: it stays nil.
	result := Selection{Targets: make([]VisibleCandidate, 0, len(targetSet))}
	if defaultRef != nil {
		if _, selected := targetSet[*defaultRef]; !selected {
			return Selection{}, fmt.Errorf("target portfolio: default is outside selected targets")
		}
		authority := make(map[corpus.FileID]VisibleCandidate, len(compilation.Request.Candidates))
		for _, candidate := range compilation.Request.Candidates {
			authority[candidate.FileRef] = candidate
		}
		defaultCandidate, known := authority[*defaultRef]
		if !known {
			return Selection{}, fmt.Errorf("target portfolio: default is outside candidate authority")
		}
		defaultCopy := cloneVisibleCandidate(defaultCandidate)
		result.Default = &defaultCopy
	}
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
