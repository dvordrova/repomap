package readmetargetscout

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

// Run executes the complete deterministic batch cover and unions every
// compatible closed-ref result. A terminal failure in any shard rejects the
// whole semantic result; accepted sibling cache entries remain executor-owned.
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
	calls := make([]llm.Call[Result], len(batches))
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
		calls[index] = llm.Call[Result]{
			State:  state,
			Prompt: llm.Prompt{System: prompt.System, User: prompt.User, ResponseFormatJSON: false},
			Limits: llm.Limits{
				MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: MaxResponseBytes,
				MaxOutputTokens: MaxOutputTokens,
			},
			DecodeValidate: func(raw []byte) (Result, error) {
				return ResolveResponse(batch, raw)
			},
		}
	}
	outcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	execution := Execution{Outcomes: append([]llm.Outcome[Result](nil), outcomes...)}
	if err != nil {
		return execution, fmt.Errorf("README file classifier: exhaustive batches: %w", err)
	}
	results := make([]Result, len(outcomes))
	for index, outcome := range outcomes {
		results[index] = outcome.Value
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
	type classSet map[FileClass]map[string]struct{}
	files := make(map[corpus.FileID]classSet)
	for _, result := range results {
		for _, file := range result {
			filePath, known := compilation.authority[file.FileRef]
			if !known || len(file.Classifications) == 0 {
				return nil, fmt.Errorf("README file classifier: merged result has invalid file authority")
			}
			classes := files[file.FileRef]
			if classes == nil {
				classes = make(classSet)
				files[file.FileRef] = classes
			}
			for _, classification := range file.Classifications {
				if !validFileClass(classification.Class) || len(classification.Hypotheses) == 0 {
					return nil, fmt.Errorf("README file classifier: merged result has invalid classification")
				}
				if isProseEvidencePath(filePath) && classification.Class != ClassDocumentation {
					continue
				}
				hypotheses := classes[classification.Class]
				if hypotheses == nil {
					hypotheses = make(map[string]struct{})
					classes[classification.Class] = hypotheses
				}
				for _, hypothesis := range classification.Hypotheses {
					if !validHypothesis(hypothesis) {
						return nil, fmt.Errorf("README file classifier: merged result has invalid hypothesis")
					}
					hypotheses[hypothesis] = struct{}{}
				}
			}
		}
	}
	merged := make(Result, 0, len(files))
	for fileRef, classes := range files {
		if len(classes) == 0 {
			continue
		}
		classifications := make([]Classification, 0, len(classes))
		for class, hypothesisSet := range classes {
			hypotheses := make([]string, 0, len(hypothesisSet))
			for hypothesis := range hypothesisSet {
				hypotheses = append(hypotheses, hypothesis)
			}
			sort.Strings(hypotheses)
			classifications = append(classifications, Classification{Class: class, Hypotheses: hypotheses})
		}
		sort.Slice(classifications, func(i, j int) bool {
			return classifications[i].Class < classifications[j].Class
		})
		merged = append(merged, ClassifiedFile{FileRef: fileRef, Classifications: classifications})
	}
	sort.Slice(merged, func(i, j int) bool {
		left, right := compilation.authority[merged[i].FileRef], compilation.authority[merged[j].FileRef]
		if left != right {
			return left < right
		}
		return merged[i].FileRef < merged[j].FileRef
	})
	if merged == nil {
		return Result{}, nil
	}
	return merged, nil
}
