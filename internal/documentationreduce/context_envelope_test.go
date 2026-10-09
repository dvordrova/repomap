package documentationreduce

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
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

type blockedDocumentationHTTP struct{ calls atomic.Int64 }

func (transport *blockedDocumentationHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("unexpected documentation HTTP")
}

// Preparation is the real provider path. Responses remain exact-request local
// presets, with no network or alternate product entry point.
type contextDocumentationProvider struct {
	client          *deepseek.Client
	preset          documentationPresetProvider
	mu              sync.Mutex
	ready           map[string]llm.Prepared
	contextRefusals int
	completeError   error
}

func (provider *contextDocumentationProvider) State() []byte { return provider.client.State() }
func (provider *contextDocumentationProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
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
func (provider *contextDocumentationProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.mu.Lock()
	local, known := provider.ready[string(prepared.Bytes())]
	provider.mu.Unlock()
	if !known {
		return llm.Completion{}, errors.New("completion without original prepared request")
	}
	if provider.completeError != nil {
		return llm.Completion{}, provider.completeError
	}
	return provider.preset.Complete(ctx, local)
}
func realDocumentationProvider(contextAllowance int) (*contextDocumentationProvider, *blockedDocumentationHTTP) {
	transport := &blockedDocumentationHTTP{}
	client := &deepseek.Client{Endpoint: "https://documentation.test/chat/completions", Auth: "none", Model: "test", MaxTokens: 320, ContextTokens: contextAllowance, HTTPClient: &http.Client{Transport: transport}}
	return &contextDocumentationProvider{client: client}, transport
}

func TestActualDeepSeekContextSourcePackingPreservesCompleteUTF8Guidance(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{
		{Path: "AGENTS.md", Kind: readmetargetscout.GuidanceAgents, Content: "Exact operator notes.\n"},
		{Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Content: strings.Repeat("Whole source α 日本語 🚀.\n", 1500)},
	})
	provider, transport := realDocumentationProvider(6000)
	result, err := Run(t.Context(), llm.Executor{BatchConcurrency: 2}, provider, guidance)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateAgainst(guidance); err != nil {
		t.Fatal(err)
	}
	if provider.contextRefusals == 0 || provider.preset.sourceCalls <= 1 || provider.preset.mergeCalls == 0 {
		t.Fatalf("context partition not exercised: refusals=%d source=%d merge=%d", provider.contextRefusals, provider.preset.sourceCalls, provider.preset.mergeCalls)
	}
	provider.preset.assertLossless(t, guidance)
	for _, parts := range provider.preset.parts {
		for _, part := range parts {
			if !utf8.ValidString(part.content) {
				t.Fatal("UTF-8 boundary split")
			}
		}
	}
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}

func TestActualDeepSeekContextMergePackingKeepsEveryOriginalCandidate(t *testing.T) {
	provider, transport := realDocumentationProvider(10000)
	authority := map[string]documentAuthority{}
	var candidates []normalizedReduction
	for i := 1; i <= 8; i++ {
		ref := fmt.Sprintf("d%d", i)
		authority[ref] = documentAuthority{path: fmt.Sprintf("docs/%d/README.md", i), kind: readmetargetscout.GuidanceReadme}
		candidates = append(candidates, normalizedReduction{overview: fmt.Sprintf("Original %d", i), sources: []responseSource{{Ref: ref, Concepts: []string{strings.Repeat("α日本語", 300)}}}})
	}
	fits, err := mergeCandidatesFit(provider, candidates, 1)
	if err != nil || fits {
		t.Fatalf("whole merge unexpectedly fits: %v/%v", fits, err)
	}
	batches, err := packMergeBatches(provider, candidates, 1, authority)
	if err != nil {
		t.Fatal(err)
	}
	var restored []normalizedReduction
	for _, batch := range batches {
		restored = append(restored, batch.candidates...)
		prepared, err := llm.Prepare(provider, reductionCall("merge", "", mergePrompt, batch.wire, batch.allowed).Prompt, limits())
		if err != nil {
			t.Fatal(err)
		}
		if prepared.Len()+provider.client.MaxTokens > provider.client.ContextTokens {
			t.Fatal("materialized framing/reserve exceeds actual context")
		}
		for ref, site := range batch.allowed {
			if site != authority[ref] {
				t.Fatal("original binding changed")
			}
		}
	}
	if len(batches) <= 1 || !reflect.DeepEqual(restored, candidates) || provider.contextRefusals == 0 {
		t.Fatal("merge candidates truncated or not partitioned")
	}
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}

type documentationPrepareFailure struct{ err error }

func (provider documentationPrepareFailure) State() []byte { return nil }
func (provider documentationPrepareFailure) Prepare(llm.Prompt, llm.Limits) (llm.Prepared, error) {
	return llm.Prepared{}, provider.err
}
func (provider documentationPrepareFailure) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{}, errors.New("unexpected complete")
}

type documentationPaymentFailure struct{}

func (documentationPaymentFailure) Error() string { return "payment required" }
func (documentationPaymentFailure) ProviderFailure() llm.ProviderFailure {
	return llm.ProviderFailure{Kind: llm.ProviderFailureHTTPStatus, HTTPStatus: 402}
}

func TestDocumentationFitRecognizesOnlyInputEnvelopeFailures(t *testing.T) {
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitContextTokens, llm.ResourceLimitRequestBytes, llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes, llm.ResourceLimitAttemptTime} {
		t.Run(string(kind), func(t *testing.T) {
			cause := &llm.ResourceLimitError{Kind: kind, Limit: 1}
			fit, err := requestFits(documentationPrepareFailure{cause}, sourcePrompt, sourceRequest{})
			input := kind == llm.ResourceLimitContextTokens || kind == llm.ResourceLimitRequestBytes
			if fit || (input && err != nil) || (!input && !errors.Is(err, cause)) {
				t.Fatalf("fit=%v err=%v", fit, err)
			}
		})
	}
	cause := documentationPaymentFailure{}
	fit, err := requestFits(documentationPrepareFailure{cause}, sourcePrompt, sourceRequest{})
	var payment documentationPaymentFailure
	if fit || !errors.As(err, &payment) {
		t.Fatalf("technical refusal hidden: %v/%v", fit, err)
	}
}

func TestDocumentationIndivisibleContextAndLivePaymentStayExplicit(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{{Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Content: "α"}})
	provider, transport := realDocumentationProvider(1)
	if _, err := Run(t.Context(), llm.Executor{}, provider, guidance); err == nil || !strings.Contains(err.Error(), "indivisible") {
		t.Fatalf("indivisible falsely accepted: %v", err)
	}
	provider.client.ContextTokens = 0
	provider.completeError = documentationPaymentFailure{}
	_, err := Run(t.Context(), llm.Executor{}, provider, guidance)
	var payment documentationPaymentFailure
	if !errors.As(err, &payment) {
		t.Fatalf("live402 converted to empty documentation: %v", err)
	}
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}
