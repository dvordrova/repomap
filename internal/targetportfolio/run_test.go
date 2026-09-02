package targetportfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

type exhaustivePortfolioProvider struct {
	calls        atomic.Int64
	requiredOnly bool
}

type semanticEnvelopePortfolioProvider struct {
	exhaustivePortfolioProvider
	maxRequestLimit atomic.Int64
}

func (provider *semanticEnvelopePortfolioProvider) Prepare(
	prompt llm.Prompt,
	limits llm.Limits,
) (llm.Prepared, error) {
	prepared, err := provider.exhaustivePortfolioProvider.Prepare(prompt, limits)
	if err != nil {
		return llm.Prepared{}, err
	}
	provider.maxRequestLimit.Store(int64(limits.MaxRequestBytes))
	if prepared.Len() > limits.MaxRequestBytes {
		return llm.Prepared{}, &llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: limits.MaxRequestBytes,
			Observed: prepared.Len(), ObservedKnown: true,
		}
	}
	return prepared, nil
}

func (provider *exhaustivePortfolioProvider) State() []byte {
	return []byte(`{"provider":"exhaustive-target-portfolio-test"}`)
}

func (provider *exhaustivePortfolioProvider) Prepare(
	prompt llm.Prompt,
	_ llm.Limits,
) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.System + "\n\x00\n" + prompt.User))
}

func (provider *exhaustivePortfolioProvider) Complete(
	_ context.Context,
	prepared llm.Prepared,
) (llm.Completion, error) {
	provider.calls.Add(1)
	text := string(prepared.Bytes())
	var response []byte
	if strings.Contains(text, "Exact bounded default-comparison JSON:\n") {
		var request DefaultRequest
		if err := decodePromptRequest(text, "Exact bounded default-comparison JSON:\n", "\n\nEnd of quoted default-comparison JSON.", &request); err != nil {
			return llm.Completion{}, err
		}
		response, _ = json.Marshal(DefaultResponse{DefaultFileRef: request.Candidates[0].FileRef})
	} else {
		var request Request
		if err := decodePromptRequest(text, "Exact bounded classification-batch JSON:\n", "\n\nEnd of quoted classification-batch JSON.", &request); err != nil {
			return llm.Completion{}, err
		}
		refs := make([]corpus.FileID, 0, len(request.Candidates))
		if provider.requiredOnly && request.RequiredTargetFileRefs != nil {
			refs = append(refs, (*request.RequiredTargetFileRefs)...)
		} else if !provider.requiredOnly {
			for _, candidate := range request.Candidates {
				refs = append(refs, candidate.FileRef)
			}
		}
		var defaultRef *corpus.FileID
		if len(refs) != 0 {
			value := refs[0]
			defaultRef = &value
		}
		response, _ = json.Marshal(Response{DefaultFileRef: defaultRef, TargetFileRefs: refs})
	}
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, ProviderResponseBytes: len(response)},
	}, nil
}

func TestRunPreservesRequiredRefsAcrossClassificationBatches(t *testing.T) {
	snapshot := testSnapshot(t, []string{"a.py", "b.py", "c.py"})
	long := strings.Repeat("complete independently launched target evidence ", 2500)
	compilation, err := CompileWithRequiredTargetAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{long + "one"}},
		{FileRef: "f2", Hypotheses: []string{long + "two"}},
		{FileRef: "f3", Hypotheses: []string{long + "three"}},
	}, []corpus.FileID{"f1", "f3"})
	if err != nil {
		t.Fatal(err)
	}
	if batches, err := classificationBatches(compilation); err != nil || len(batches) < 2 {
		t.Fatalf("classification batches = %d, err = %v", len(batches), err)
	}
	provider := &exhaustivePortfolioProvider{requiredOnly: true}
	execution, err := Run(
		context.Background(), llm.Executor{Enabled: false, BatchConcurrency: 4}, provider, compilation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Selection.Default == nil || execution.Selection.Default.FileRef != "f1" ||
		len(execution.Selection.Targets) != 2 ||
		execution.Selection.Targets[0].FileRef != "f1" || execution.Selection.Targets[1].FileRef != "f3" ||
		len(execution.Selection.Unclassified) != 1 || execution.Selection.Unclassified[0].FileRef != "f2" {
		t.Fatalf("required multi-batch selection = %#v", execution.Selection)
	}
}

func decodePromptRequest(text string, prefix string, suffix string, destination any) error {
	start := strings.Index(text, prefix)
	if start < 0 {
		return fmt.Errorf("missing prompt request prefix")
	}
	start += len(prefix)
	end := strings.Index(text[start:], suffix)
	if end < 0 {
		return fmt.Errorf("missing prompt request suffix")
	}
	return json.Unmarshal([]byte(text[start:start+end]), destination)
}

func TestRunClassifiesThresholdPlusOneCompleteReservoirAgainstProviderEnvelope(t *testing.T) {
	const count = 3000
	paths := make([]string, count)
	candidates := make([]Candidate, count)
	for index := range paths {
		paths[index] = fmt.Sprintf("services/service-%04d/main.py", index)
	}
	snapshot := testSnapshot(t, paths)
	for index, entry := range snapshot.Entries {
		candidates[index] = Candidate{
			FileRef: entry.ID,
			Hypotheses: []string{fmt.Sprintf(
				"independently launched service %04d with complete retained classification evidence", index,
			)},
		}
	}
	compilation, err := Compile(snapshot, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.wire) <= AdvisoryCompleteRequestBytes {
		t.Fatalf("fixture did not cross former aggregate threshold: %d", len(compilation.wire))
	}
	provider := &exhaustivePortfolioProvider{}
	execution, err := Run(
		context.Background(),
		llm.Executor{Enabled: false, BatchConcurrency: 4},
		provider,
		compilation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Selection.Default == nil || execution.Selection.Default.FileRef != "f1" ||
		len(execution.Selection.Targets) != count || len(execution.Selection.Unclassified) != 0 {
		t.Fatalf(
			"complete result = default %#v, targets %d, unclassified %d",
			execution.Selection.Default, len(execution.Selection.Targets), len(execution.Selection.Unclassified),
		)
	}
	if len(execution.Outcomes) < 1 || int(provider.calls.Load()) != len(execution.Outcomes) {
		t.Fatalf("model operations = outcomes %d / calls %d", len(execution.Outcomes), provider.calls.Load())
	}
	for index, candidate := range execution.Selection.Targets {
		if candidate.FileRef != snapshot.Entries[index].ID {
			t.Fatalf("target %d = %s, want %s", index, candidate.FileRef, snapshot.Entries[index].ID)
		}
	}
}

func TestRunSendsIndivisiblePackingSingletonThroughSemanticEnvelope(t *testing.T) {
	snapshot := testSnapshot(t, []string{"main.py"})
	long := strings.Repeat("complete-target-evidence-", MaxRequestBytes/25+256) + "end"
	compilation, err := Compile(snapshot, []Candidate{{
		FileRef: "f1", Hypotheses: []string{long},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.wire) <= MaxRequestBytes {
		t.Fatalf("fixture bytes = %d, want beyond packing window", len(compilation.wire))
	}
	provider := &semanticEnvelopePortfolioProvider{}
	execution, err := Run(
		t.Context(), llm.Executor{Enabled: false}, provider, compilation,
	)
	if err != nil {
		t.Fatalf("semantic-envelope singleton failed: %v", err)
	}
	if provider.maxRequestLimit.Load() != llm.SemanticRecordByteLimit ||
		execution.Selection.Default == nil || execution.Selection.Default.FileRef != "f1" ||
		len(execution.Selection.Targets) != 1 {
		t.Fatalf("semantic-envelope execution = %#v, limit=%d", execution.Selection, provider.maxRequestLimit.Load())
	}
}
