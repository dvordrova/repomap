package deepseek

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestExecuteLocalizationRequestUsesExactEvidenceAndReturnsMetrics(t *testing.T) {
	t.Parallel()

	var receivedBody []byte
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		receivedAuth = request.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"content":"{\"version\":\"localization-projection-json-v1\",\"fields\":[]}"}}],
			"usage":{
				"prompt_tokens":101,
				"completion_tokens":23,
				"prompt_cache_hit_tokens":80,
				"prompt_cache_miss_tokens":21
			}
		}`)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "translate exact fields",
		User:    "canonical English payload",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authBearer,
		Model: "translation-model", MaxTokens: 2048,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	executor := &Client{
		HTTPClient: server.Client(),
		APIKey:     "local-test-key",
		// Execution must use the provider identity already bound into evidence.
		Endpoint: "https://must-not-be-used.example.test",
		Auth:     authNone,
		Model:    "must-not-be-used",
	}
	result, err := executor.ExecuteLocalizationRequest(
		context.Background(),
		prompt,
		evidence,
	)
	if err != nil {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if !bytes.Equal(receivedBody, evidence.Body) {
		t.Fatalf("request body changed\ngot:  %s\nwant: %s", receivedBody, evidence.Body)
	}
	if receivedAuth != "Bearer local-test-key" {
		t.Fatalf("Authorization = %q", receivedAuth)
	}
	if string(result.Content) != `{"version":"localization-projection-json-v1","fields":[]}` {
		t.Fatalf("Content = %s", result.Content)
	}
	if result.Attempts != 1 ||
		result.RequestBytes != len(evidence.Body) ||
		result.InputTokens != 101 ||
		result.OutputTokens != 23 ||
		result.PromptCacheHitTokens != 80 ||
		result.PromptCacheMissTokens != 21 {
		t.Fatalf("metrics = %#v", result)
	}
	body := string(receivedBody)
	if strings.Contains(body, canonicalEnglishSystemContract) ||
		strings.Contains(body, canonicalEnglishUserContract) {
		t.Fatalf("translation request contains semantic-English wrapper: %s", body)
	}
}

func TestExecuteLocalizationRequestDoesNotRetryNetworkFailure(t *testing.T) {
	t.Parallel()

	var serverCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"content":"{}"}}],
			"usage":{"prompt_tokens":7,"completion_tokens":3}
		}`)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authNone,
		Model: "translation-model", MaxTokens: 128,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	transport := &failOnceRoundTripper{
		next: server.Client().Transport,
		err:  errors.New("temporary connection reset"),
	}
	executor := &Client{
		HTTPClient: &http.Client{Transport: transport},
		Auth:       authNone,
	}
	result, err := executor.ExecuteLocalizationRequest(
		context.Background(),
		prompt,
		evidence,
	)
	if err == nil || !strings.Contains(err.Error(), "temporary connection reset") {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if transport.calls.Load() != 1 || serverCalls.Load() != 0 {
		t.Fatalf(
			"transport calls/server calls = %d/%d, want 1/0",
			transport.calls.Load(),
			serverCalls.Load(),
		)
	}
	if result.Attempts != 1 ||
		result.RequestBytes != len(evidence.Body) ||
		result.InputTokens != 0 ||
		result.OutputTokens != 0 {
		t.Fatalf("metrics = %#v", result)
	}
}

func TestExecuteLocalizationRequestRejectsInvalidEvidenceBeforeTransport(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authNone,
		Model: "translation-model", MaxTokens: 128,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	evidence.Body = append([]byte(nil), evidence.Body...)
	evidence.Body[len(evidence.Body)-1] = ' '

	result, err := (&Client{
		HTTPClient: server.Client(),
		Auth:       authNone,
	}).ExecuteLocalizationRequest(context.Background(), prompt, evidence)
	if err == nil || !strings.Contains(err.Error(), "invalid localization request evidence") {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if len(result.Content) != 0 ||
		result.Attempts != 0 ||
		result.RequestBytes != 0 ||
		result.InputTokens != 0 ||
		result.OutputTokens != 0 ||
		result.PromptCacheHitTokens != 0 ||
		result.PromptCacheMissTokens != 0 {
		t.Fatalf("result = %#v, want zero", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
}

func TestExecuteLocalizationRequestPreservesSafeProviderErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"Bearer company-secret-token-value"}`)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authNone,
		Model: "translation-model", MaxTokens: 128,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	result, err := (&Client{
		HTTPClient: server.Client(),
		Auth:       authNone,
	}).ExecuteLocalizationRequest(context.Background(), prompt, evidence)
	if err == nil ||
		!strings.Contains(err.Error(), "status 400") ||
		!strings.Contains(err.Error(), "[redacted:") ||
		strings.Contains(err.Error(), "company-secret-token-value") {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if calls.Load() != 1 ||
		result.Attempts != 1 ||
		result.RequestBytes != len(evidence.Body) {
		t.Fatalf("calls/result = %d/%#v", calls.Load(), result)
	}
}

type failOnceRoundTripper struct {
	next  http.RoundTripper
	err   error
	calls atomic.Int32
}

func (transport *failOnceRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	if transport.calls.Add(1) == 1 {
		return nil, transport.err
	}
	return transport.next.RoundTrip(request)
}
