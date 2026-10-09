package readmetargetscout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

type forbiddenGuidanceHTTP struct{ calls atomic.Int64 }

func (transport *forbiddenGuidanceHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("test attempted forbidden HTTP")
}

// Only Prepare is real. Complete supplies a closed, request-bound response
// and records its full native authority; it cannot contact a provider.
type preparedGuidanceProvider struct {
	client    *deepseek.Client
	mu        sync.Mutex
	ready     map[string]Request
	calls     map[string]int
	completed []Request
	refused   int
	fail      func(Request) error
}

func (provider *preparedGuidanceProvider) State() []byte { return provider.client.State() }
func (provider *preparedGuidanceProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	at := strings.Index(prompt.User, `{"repo_name"`)
	if at < 0 {
		return llm.Prepared{}, errors.New("guidance request missing")
	}
	var request Request
	if err := json.NewDecoder(strings.NewReader(prompt.User[at:])).Decode(&request); err != nil {
		return llm.Prepared{}, err
	}
	prepared, err := provider.client.Prepare(prompt, limits)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if err != nil {
		provider.refused++
		return prepared, err
	}
	if provider.ready == nil {
		provider.ready = make(map[string]Request)
		provider.calls = make(map[string]int)
	}
	provider.ready[sha256Hex(prepared.Bytes())] = request
	return prepared, nil
}
func (provider *preparedGuidanceProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	key := sha256Hex(prepared.Bytes())
	request, known := provider.ready[key]
	if !known {
		return llm.Completion{}, errors.New("complete without exact prepared request")
	}
	provider.calls[key]++
	if provider.fail != nil {
		if err := provider.fail(request); err != nil {
			return llm.Completion{}, err
		}
	}
	provider.completed = append(provider.completed, request)
	authority, err := fileTreeDictionary(request.FileTree, request.FileCount)
	if err != nil {
		return llm.Completion{}, err
	}
	rows := make([]map[string]any, 0, len(authority))
	for _, ref := range canonicalAuthorityRefs(authority) {
		hypotheses := make([]string, len(request.GuidanceDocuments))
		for i, document := range request.GuidanceDocuments {
			hypotheses[i] = "Guidance " + string(document.FileRef) + " names the exact entry."
		}
		rows = append(rows, map[string]any{"file_ref": ref, "hypotheses": hypotheses})
	}
	raw, err := json.Marshal(map[string]any{"files": rows})
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1}, err
}
func guidanceProvider() (*preparedGuidanceProvider, *forbiddenGuidanceHTTP) {
	transport := &forbiddenGuidanceHTTP{}
	return &preparedGuidanceProvider{client: &deepseek.Client{Endpoint: "https://guidance.test/chat/completions", Auth: "none", Model: "test", MaxTokens: 320, HTTPClient: &http.Client{Transport: transport}}}, transport
}
func guidanceLimits() llm.Limits {
	return llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: MaxResponseBytes, MaxOutputTokens: MaxOutputTokens}
}
func assertGuidanceCover(t *testing.T, compilation Compilation, requests []Request) {
	t.Helper()
	documents := make(map[string]RequestGuidanceDocument)
	for _, doc := range compilation.Request.GuidanceDocuments {
		documents[string(doc.FileRef)] = doc
	}
	coverage := make(map[string]int)
	for _, request := range requests {
		authority, err := fileTreeDictionary(request.FileTree, request.FileCount)
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range request.GuidanceDocuments {
			if want, known := documents[string(doc.FileRef)]; !known || doc != want {
				t.Fatalf("guidance bytes/anchors changed: %+v", doc)
			}
			for ref, path := range authority {
				if compilation.authority[ref] != path {
					t.Fatalf("original ref/path changed: %s=%s", ref, path)
				}
				coverage[string(doc.FileRef)+"\x00"+string(ref)]++
			}
		}
	}
	for doc := range documents {
		for ref := range compilation.authority {
			if count := coverage[doc+"\x00"+string(ref)]; count != 1 {
				t.Fatalf("document/file pair %s/%s has %d uses", doc, ref, count)
			}
		}
	}
}

func TestRunActualDeepSeekPreparationPreservesEveryDocumentFilePairWithoutHTTP(t *testing.T) {
	files := map[string]string{"README.md": strings.Repeat("complete guidance. ", 90), "docs/AGENTS.md": strings.Repeat("complete guidance. ", 90), "docs/README.md": strings.Repeat("complete guidance. ", 90)}
	for i := 0; i < 8; i++ {
		files[fmt.Sprintf("pkg/%02d/%s.go", i, strings.Repeat("long_name_", 14))] = "package p"
	}
	repository, _ := testCorpus(t, files)
	compilation, err := Compile("sample", repository)
	if err != nil {
		t.Fatal(err)
	}
	provider, transport := guidanceProvider()
	atomic, err := compileBatchSubset(compilation, compilation.Request.GuidanceDocuments[:1], canonicalAuthorityRefs(compilation.authority)[:1])
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildPrompt(atomic)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := llm.Prepare(provider.client, llm.Prompt{System: prompt.System, User: prompt.User, ResponseFormatJSON: true, ResponseExample: responseExample}, guidanceLimits())
	if err != nil {
		t.Fatal(err)
	}
	provider.client.ContextTokens = prepared.Len() + provider.client.MaxTokens + 32
	execution, err := Run(t.Context(), llm.Executor{BatchConcurrency: 4}, provider, compilation)
	if err != nil {
		t.Fatal(err)
	}
	if provider.refused == 0 || len(provider.completed) <= 1 || len(execution.Result) != len(compilation.authority) || execution.UnavailableBatches != 0 {
		t.Fatalf("no real adaptive cover: refusals=%d requests=%d result=%d", provider.refused, len(provider.completed), len(execution.Result))
	}
	assertGuidanceCover(t, compilation, provider.completed)
	for _, request := range provider.completed {
		if request.FileCount == compilation.Request.FileCount && len(request.GuidanceDocuments) == len(compilation.Request.GuidanceDocuments) {
			t.Fatal("oversized whole input reached Complete")
		}
	}
	if transport.calls.Load() != 0 {
		t.Fatal("actual Prepare made HTTP")
	}
	// The changed prompt must never promote local uniqueness to global proof.
	if !strings.Contains(prompt.System, "absence of another candidate from this request never proves uniqueness") {
		t.Fatal("missing shard uniqueness guard")
	}
}

func TestRunFittingWholeGuidanceHasOneStableRequestLocalDecision(t *testing.T) {
	repository, _ := testCorpus(t, map[string]string{"README.md": "Run main.go and worker.go.", "main.go": "package main", "worker.go": "package main"})
	compilation, err := Compile("sample", repository)
	if err != nil {
		t.Fatal(err)
	}
	provider, transport := guidanceProvider()
	execution, err := Run(t.Context(), llm.Executor{}, provider, compilation)
	if err != nil || len(execution.Outcomes) != 1 || len(provider.completed) != 1 {
		t.Fatalf("whole fitting request: %d/%d, %v", len(execution.Outcomes), len(provider.completed), err)
	}
	assertGuidanceCover(t, compilation, provider.completed)
	batch, err := compileBatchSubset(compilation, compilation.Request.GuidanceDocuments, canonicalAuthorityRefs(compilation.authority))
	if err != nil {
		t.Fatal(err)
	}
	state, err := batchExecutionState(compilation, batch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "batch_index") || strings.Contains(string(state), "batch_count") || !strings.Contains(string(state), batch.RequestSHA256) {
		t.Fatalf("state depends on changing plan: %s", state)
	}
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}

func TestRunRemoteContextRefusalSplitsChangedCompleteInputAndKeepsAcceptedSibling(t *testing.T) {
	repository, _ := testCorpus(t, map[string]string{"README.md": "Run all the named entry files.", "a.go": "package a", "b.go": "package b", "c.go": "package c", "d.go": "package d"})
	compilation, err := Compile("sample", repository)
	if err != nil {
		t.Fatal(err)
	}
	first := canonicalAuthorityRefs(compilation.authority)[0]
	provider, transport := guidanceProvider()
	provider.fail = func(request Request) error {
		authority, err := fileTreeDictionary(request.FileTree, request.FileCount)
		if err != nil {
			return err
		}
		if request.FileCount > 2 || (request.FileCount > 1 && authority[first] != "") {
			return &llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens, Limit: 1000, Observed: 2000, ObservedKnown: true, HTTPStatus: 400}
		}
		return nil
	}
	execution, err := Run(t.Context(), llm.Executor{BatchConcurrency: 2}, provider, compilation)
	if err != nil || len(execution.Result) != 4 || len(provider.calls) != 5 {
		t.Fatalf("remote partition: result=%d calls=%d err=%v", len(execution.Result), len(provider.calls), err)
	}
	for _, count := range provider.calls {
		if count != 1 {
			t.Fatal("identical parent or accepted sibling repeated")
		}
	}
	assertGuidanceCover(t, compilation, provider.completed)
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}

type guidancePaymentError struct{}

func (guidancePaymentError) Error() string { return "payment required" }
func (guidancePaymentError) ProviderFailure() llm.ProviderFailure {
	return llm.ProviderFailure{Kind: llm.ProviderFailureHTTPStatus, HTTPStatus: http.StatusPaymentRequired}
}

func TestRunIndivisibleActualEnvelopeAndPaymentRemainExplicitErrors(t *testing.T) {
	repository, _ := testCorpus(t, map[string]string{"README.md": "Run main.go.", "main.go": "package main"})
	compilation, err := Compile("sample", repository)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("indivisible complete document and file", func(t *testing.T) {
		provider, transport := guidanceProvider()
		provider.client.ContextTokens = 1
		execution, err := Run(t.Context(), llm.Executor{}, provider, compilation)
		var resource *llm.ResourceLimitError
		if !errors.As(err, &resource) || resource.Kind != llm.ResourceLimitContextTokens || execution.Result != nil || len(provider.calls) != 0 || transport.calls.Load() != 0 {
			t.Fatalf("indivisible input silently accepted: %+v %v", execution, err)
		}
	})
	t.Run("HTTP402 is not semantic empty guidance", func(t *testing.T) {
		provider, transport := guidanceProvider()
		provider.fail = func(Request) error { return guidancePaymentError{} }
		execution, err := Run(t.Context(), llm.Executor{}, provider, compilation)
		var failure llm.ProviderFailureSource
		if err == nil || !errors.As(err, &failure) || failure.ProviderFailure().HTTPStatus != 402 || execution.Result != nil || execution.UnavailableBatches != 0 || len(provider.calls) != 1 || transport.calls.Load() != 0 {
			t.Fatalf("402 was swallowed or split: %+v %v", execution, err)
		}
	})
}
