package readmetargetscout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

// Run prepares the complete authority through the provider and splits only
// observed envelopes. Every final request covers an original doc-by-file
// rectangle; independent accepted calls survive without being asked again.
func Run(ctx context.Context, executor llm.Executor, provider llm.Provider, compilation Compilation) (Execution, error) {
	if err := validateReadyCompilation(compilation); err != nil {
		return Execution{}, err
	}
	build := func(item guidanceBatch) (llm.Call[responseResult], error) {
		if item.err != nil {
			return llm.Call[responseResult]{}, item.err
		}
		batch := item.compilation
		prompt, err := BuildPrompt(batch)
		if err != nil {
			return llm.Call[responseResult]{}, err
		}
		state, err := batchExecutionState(compilation, batch)
		if err != nil {
			return llm.Call[responseResult]{}, err
		}
		return llm.Call[responseResult]{
			State:          state,
			Prompt:         llm.Prompt{System: prompt.System, User: prompt.User, ResponseFormatJSON: true, ResponseExample: responseExample},
			Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: MaxResponseBytes, MaxOutputTokens: MaxOutputTokens},
			DecodeValidate: func(raw []byte) (responseResult, error) { return resolveResponse(batch, raw) },
		}, nil
	}
	responses, err := llm.ExecuteAdaptiveJSONEachResults(ctx, executor, provider,
		[]guidanceBatch{{compilation: compilation}}, build, splitGuidanceBatch)
	if err != nil {
		return Execution{}, err
	}
	execution := Execution{Outcomes: make([]llm.Outcome[responseResult], len(responses))}
	results := make([]Result, len(responses))
	// Populate every outcome before considering terminal errors, preserving the
	// complete accepted/refused leaf evidence even when discovery cannot finish.
	for index, response := range responses {
		execution.Outcomes[index] = response.Outcome
	}
	for index, response := range responses {
		if response.Err == nil {
			results[index] = response.Outcome.Value.Result
			continue
		}
		if ctx.Err() != nil {
			return execution, ctx.Err()
		}
		var resource *llm.ResourceLimitError
		if errors.As(response.Err, &resource) {
			return execution, response.Err
		}
		modelFailure := false
		for _, rejected := range response.Outcome.ResponseRejections {
			if rejected.Kind == "response_validation" || rejected.Kind == "response_envelope" {
				modelFailure = true
			}
		}
		if !modelFailure {
			return execution, response.Err
		}
		execution.UnavailableBatches++
	}
	result, err := MergeResults(compilation, results)
	if err != nil {
		return execution, err
	}
	execution.Result = result
	return execution, nil
}

func batchExecutionState(compilation, batch Compilation) ([]byte, error) {
	state, err := json.Marshal(struct {
		Contract       json.RawMessage `json:"contract"`
		CompilationSHA string          `json:"compilation_sha256"`
		RequestSHA     string          `json:"request_sha256"`
	}{ExecutionState(), compilation.RequestSHA256, batch.RequestSHA256})
	if err != nil {
		return nil, fmt.Errorf("README file classifier: encode request execution state: %w", err)
	}
	return state, nil
}

// MergeResults unions all compatible set-valued rows against the aggregate
// authority. Unknown refs have already been discarded by each request-local
// reducer; this second boundary rejects malformed internal input rather than
// inventing or repairing it.
func MergeResults(compilation Compilation, results []Result) (Result, error) {
	if err := validateReadyCompilation(compilation); err != nil {
		return nil, err
	}
	files := make(map[corpus.FileID]map[string]struct{})
	for _, result := range results {
		for _, file := range result {
			if _, known := compilation.authority[file.FileRef]; !known || len(file.Classifications) != 1 {
				return nil, fmt.Errorf("README file classifier: merged result has invalid file authority")
			}
			classification := file.Classifications[0]
			if !validFileClass(classification.Class) || len(classification.Hypotheses) == 0 {
				return nil, fmt.Errorf("README file classifier: merged result has invalid classification")
			}
			hypotheses := files[file.FileRef]
			if hypotheses == nil {
				hypotheses = make(map[string]struct{})
				files[file.FileRef] = hypotheses
			}
			for _, hypothesis := range classification.Hypotheses {
				if !validHypothesis(hypothesis) {
					return nil, fmt.Errorf("README file classifier: merged result has invalid hypothesis")
				}
				hypotheses[hypothesis] = struct{}{}
			}
		}
	}
	merged := make(Result, 0, len(files))
	for fileRef, hypotheses := range files {
		merged = append(merged, entryFile(fileRef, hypotheses))
	}
	sortByPath(merged, compilation.authority)
	return merged, nil
}
