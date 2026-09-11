package readmetargetscout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

// Run executes the complete deterministic batch cover and unions every
// compatible closed-ref result. A refused model shard contributes no guidance
// classification; independent native target inventories remain untouched.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
) (Execution, error) {
	batches, err := batches(compilation)
	if err != nil {
		return Execution{}, err
	}
	calls := make([]llm.Call[responseResult], len(batches))
	for index, batch := range batches {
		prompt, err := BuildPrompt(batch)
		if err != nil {
			return Execution{}, err
		}
		state, err := batchExecutionState(compilation, index, len(batches))
		if err != nil {
			return Execution{}, err
		}
		batch := batch
		calls[index] = llm.Call[responseResult]{
			State:  state,
			Prompt: llm.Prompt{System: prompt.System, User: prompt.User, ResponseFormatJSON: true, ResponseExample: responseExample},
			Limits: llm.Limits{
				MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: MaxResponseBytes,
				MaxOutputTokens: MaxOutputTokens,
			},
			DecodeValidate: func(raw []byte) (responseResult, error) {
				return resolveResponse(batch, raw)
			},
		}
	}
	responses := llm.ExecuteJSONEach(ctx, executor, provider, calls)
	execution := Execution{Outcomes: make([]llm.Outcome[responseResult], len(responses))}
	results := make([]Result, len(responses))
	for index, response := range responses {
		execution.Outcomes[index] = response.Outcome
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
			if rejected.Kind == "response_validation" || rejected.Kind == "response_envelope" || rejected.Kind == "provider_failed" {
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

func batchExecutionState(compilation Compilation, index, count int) ([]byte, error) {
	state, err := json.Marshal(struct {
		Contract       json.RawMessage `json:"contract"`
		CompilationSHA string          `json:"compilation_sha256"`
		BatchIndex     int             `json:"batch_index"`
		BatchCount     int             `json:"batch_count"`
	}{
		Contract: ExecutionState(), CompilationSHA: compilation.RequestSHA256,
		BatchIndex: index, BatchCount: count,
	})
	if err != nil {
		return nil, fmt.Errorf("README file classifier: encode batch execution state: %w", err)
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
