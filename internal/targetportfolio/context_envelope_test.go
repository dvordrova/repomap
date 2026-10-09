package targetportfolio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

type blockedPortfolioHTTP struct{ calls atomic.Int64 }

func (transport *blockedPortfolioHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("unexpected portfolio HTTP")
}

type contextPortfolioProvider struct {
	client          *deepseek.Client
	preset          exhaustivePortfolioProvider
	mu              sync.Mutex
	ready           map[string]llm.Prepared
	contextRefusals int
	classification  []Request
	comparisons     []DefaultRequest
	completeError   error
}

func (provider *contextPortfolioProvider) State() []byte { return provider.client.State() }
func (provider *contextPortfolioProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	prepared, err := provider.client.Prepare(prompt, limits)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if err != nil {
		var resource *llm.ResourceLimitError
		if errors.As(err, &resource) && resource.Kind == llm.ResourceLimitContextTokens {
			provider.contextRefusals++
		}
		return prepared, err
	}
	local, err := provider.preset.Prepare(prompt, limits)
	if err != nil {
		return llm.Prepared{}, err
	}
	if provider.ready == nil {
		provider.ready = make(map[string]llm.Prepared)
	}
	provider.ready[string(prepared.Bytes())] = local
	return prepared, nil
}
func (provider *contextPortfolioProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.mu.Lock()
	local, known := provider.ready[string(prepared.Bytes())]
	if !known {
		provider.mu.Unlock()
		return llm.Completion{}, errors.New("completion without prepared authority")
	}
	if provider.completeError != nil {
		provider.mu.Unlock()
		return llm.Completion{}, provider.completeError
	}
	text := string(local.Bytes())
	if strings.Contains(text, "Exact bounded default-comparison JSON:\n") {
		var request DefaultRequest
		if err := decodePromptRequest(text, "Exact bounded default-comparison JSON:\n", "\n\nEnd of quoted default-comparison JSON.", &request); err != nil {
			provider.mu.Unlock()
			return llm.Completion{}, err
		}
		provider.comparisons = append(provider.comparisons, request)
	} else {
		var request Request
		if err := decodePromptRequest(text, "Exact bounded classification-batch JSON:\n", "\n\nEnd of quoted classification-batch JSON.", &request); err != nil {
			provider.mu.Unlock()
			return llm.Completion{}, err
		}
		provider.classification = append(provider.classification, request)
	}
	provider.mu.Unlock()
	return provider.preset.Complete(ctx, local)
}
func realPortfolioProvider(contextAllowance int) (*contextPortfolioProvider, *blockedPortfolioHTTP) {
	transport := &blockedPortfolioHTTP{}
	return &contextPortfolioProvider{client: &deepseek.Client{Endpoint: "https://portfolio.test/chat/completions", Auth: "none", Model: "test", MaxTokens: 320, ContextTokens: contextAllowance, HTTPClient: &http.Client{Transport: transport}}}, transport
}
func contextPortfolioFixture(t *testing.T) Compilation {
	t.Helper()
	var paths []string
	for i := 0; i < 8; i++ {
		paths = append(paths, fmt.Sprintf("services/%d/main.py", i))
	}
	snapshot := testSnapshot(t, paths)
	var candidates []Candidate
	var required []corpus.FileID
	for _, entry := range snapshot.Entries {
		candidates = append(candidates, Candidate{FileRef: entry.ID, Hypotheses: []string{strings.Repeat("Complete source-bound evidence α. ", 60) + entry.Path}})
		required = append(required, entry.ID)
	}
	compilation, err := CompileWithRequiredTargetAuthority(snapshot, candidates, required)
	if err != nil {
		t.Fatal(err)
	}
	return compilation
}

func contextPortfolioAllowance(t *testing.T, compilation Compilation, provider *contextPortfolioProvider, defaultOnly bool) int {
	t.Helper()
	var prompt llm.Prompt
	if defaultOnly {
		batch, err := compileDefaultBatch(compilation, compilation.requiredTargetFileRefs[:2])
		if err != nil {
			t.Fatal(err)
		}
		prompt, err = batch.buildPrompt()
		if err != nil {
			t.Fatal(err)
		}
	} else {
		batch, err := compileSubset(compilation, compilation.candidates[:2])
		if err != nil {
			t.Fatal(err)
		}
		prompt = llm.Prompt{System: promptSystem, User: fmt.Sprintf(promptUserShape, batch.wire), ResponseFormatJSON: true, ResponseExample: responseExample}
	}
	prepared, err := llm.Prepare(provider, prompt, portfolioCallLimits())
	if err != nil {
		t.Fatal(err)
	}
	return prepared.Len() + provider.client.MaxTokens + 32
}

func TestActualDeepSeekContextPortfolioCoversOriginalClassificationAndDefaultChoices(t *testing.T) {
	compilation := contextPortfolioFixture(t)
	provider, transport := realPortfolioProvider(0)
	provider.client.ContextTokens = contextPortfolioAllowance(t, compilation, provider, false)
	execution, err := Run(t.Context(), llm.Executor{BatchConcurrency: 2}, provider, compilation)
	if err != nil {
		t.Fatal(err)
	}
	if provider.contextRefusals == 0 || len(provider.classification) <= 1 || len(provider.comparisons) == 0 || len(execution.Selection.Targets) != len(compilation.candidates) {
		t.Fatalf("no actual partition: refused=%d classified=%d comparisons=%d targets=%d", provider.contextRefusals, len(provider.classification), len(provider.comparisons), len(execution.Selection.Targets))
	}
	originals := map[corpus.FileID]VisibleCandidate{}
	for _, row := range compilation.Request.Candidates {
		originals[row.FileRef] = row
	}
	uses := map[corpus.FileID]int{}
	for _, request := range provider.classification {
		for _, row := range request.Candidates {
			if !reflect.DeepEqual(row, originals[row.FileRef]) {
				t.Fatal("classification evidence changed")
			}
			uses[row.FileRef]++
		}
		if request.RequiredTargetFileRefs == nil || len(*request.RequiredTargetFileRefs) != len(request.Candidates) {
			t.Fatal("required local authority lost")
		}
	}
	for ref := range originals {
		if uses[ref] != 1 {
			t.Fatalf("%s classification uses%d", ref, uses[ref])
		}
	}
	for _, request := range provider.comparisons {
		for _, row := range request.Candidates {
			if !reflect.DeepEqual(row, originals[row.FileRef]) {
				t.Fatal("default native candidate evidence changed")
			}
		}
	}
	if execution.Selection.Default == nil || execution.Selection.Default.FileRef != "f1" || transport.calls.Load() != 0 {
		t.Fatalf("default=%v HTTP%d", execution.Selection.Default, transport.calls.Load())
	}
}

func TestActualDeepSeekContextDefaultPackingRefusesWholeAndPreservesFullRows(t *testing.T) {
	compilation := contextPortfolioFixture(t)
	provider, transport := realPortfolioProvider(0)
	provider.client.ContextTokens = contextPortfolioAllowance(t, compilation, provider, true)
	refs := compilation.requiredTargetFileRefs
	whole, err := compileDefaultBatch(compilation, refs)
	if err != nil {
		t.Fatal(err)
	}
	wholePrompt, err := whole.buildPrompt()
	if err != nil {
		t.Fatal(err)
	}
	_, err = llm.Prepare(provider, wholePrompt, portfolioCallLimits())
	fit, fitErr := requestFitResult(err)
	if fit || fitErr != nil {
		t.Fatalf("whole default context=%v/%v", fit, fitErr)
	}
	batches, err := defaultBatchesWithFit(compilation, refs, func(wire []byte) (bool, error) {
		_, err := llm.Prepare(provider, llm.Prompt{System: defaultPromptSystem, User: fmt.Sprintf(defaultPromptUserShape, wire), ResponseFormatJSON: true, ResponseExample: defaultResponseExample}, portfolioCallLimits())
		return requestFitResult(err)
	})
	if err != nil {
		t.Fatal(err)
	}
	var restored []VisibleCandidate
	for _, batch := range batches {
		prompt, err := batch.buildPrompt()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = llm.Prepare(provider, prompt, portfolioCallLimits()); err != nil {
			t.Fatal(err)
		}
		restored = append(restored, batch.request.Candidates...)
	}
	if len(batches) <= 1 || !reflect.DeepEqual(restored, whole.request.Candidates) || transport.calls.Load() != 0 {
		t.Fatal("default catalogue changed or was not split")
	}
}

type portfolioPaymentFailure struct{}

func (portfolioPaymentFailure) Error() string { return "payment required" }
func (portfolioPaymentFailure) ProviderFailure() llm.ProviderFailure {
	return llm.ProviderFailure{Kind: llm.ProviderFailureHTTPStatus, HTTPStatus: 402}
}
func TestPortfolioFitKeepsTechnicalAndOtherResourceFailures(t *testing.T) {
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitRequestBytes, llm.ResourceLimitContextTokens, llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
		cause := &llm.ResourceLimitError{Kind: kind}
		fit, err := requestFitResult(cause)
		input := kind == llm.ResourceLimitRequestBytes || kind == llm.ResourceLimitContextTokens
		if fit || (input && err != nil) || (!input && !errors.Is(err, cause)) {
			t.Fatalf("%s=%v/%v", kind, fit, err)
		}
	}
	compilation := contextPortfolioFixture(t)
	provider, transport := realPortfolioProvider(1)
	if _, err := Run(t.Context(), llm.Executor{}, provider, compilation); err == nil || !strings.Contains(err.Error(), "indivisible") {
		t.Fatalf("indivisible accepted: %v", err)
	}
	provider.client.ContextTokens = 0
	provider.completeError = portfolioPaymentFailure{}
	_, err := Run(t.Context(), llm.Executor{}, provider, compilation)
	var payment portfolioPaymentFailure
	if !errors.As(err, &payment) || transport.calls.Load() != 0 {
		t.Fatalf("402 hidden %v", err)
	}
}
