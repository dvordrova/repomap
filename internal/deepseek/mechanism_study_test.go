package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func mechanismStudyPromptFixture() mechanismstudy.Prompt {
	return mechanismstudy.Prompt{
		Version: mechanismstudy.PromptVersion,
		System:  "Use only supplied refs. Return exactly one JSON object.",
		User:    `Response JSON schema: {"version":1,"cards":[]}. Exact request bundle JSON: {"cards":[]}`,
	}
}

func TestMechanismStudyPromptJSONPreservesExactGenericContract(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint:  "https://provider.example.test/v1/chat/completions",
		Model:     "fixture-model",
		MaxTokens: 64_000,
	}
	prompt := mechanismStudyPromptFixture()

	got, err := client.MechanismStudyPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(chatRequest{
		Model: client.Model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System + "\n\n" + canonicalEnglishSystemContract},
			{Role: "user", Content: canonicalEnglishUserContract + "\n\n" + prompt.User},
		},
		Temperature:    float64Pointer(0.1),
		MaxTokens:      client.MaxTokens,
		ResponseFormat: &jsonFormat{Type: "json_object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("request JSON differs\ngot:  %s\nwant: %s", got, want)
	}
	if bytes.Contains(got, []byte(`"thinking"`)) {
		t.Fatalf("generic mechanism request gained an unowned extension: %s", got)
	}
}

func TestMechanismStudyPromptJSONDisablesOfficialDeepSeekThinking(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint:  "https://api.deepseek.com/chat/completions",
		Model:     "deepseek-v4-flash",
		MaxTokens: 64_000,
	}
	body, err := client.MechanismStudyPromptJSON(mechanismStudyPromptFixture())
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.Thinking == nil || request.Thinking.Type != "disabled" {
		t.Fatalf("thinking = %#v, want disabled", request.Thinking)
	}
	if request.MaxTokens != client.MaxTokens || request.ResponseFormat == nil ||
		request.ResponseFormat.Type != "json_object" {
		t.Fatalf("request contract = %#v", request)
	}
}

func TestMechanismStudyPromptJSONRejectsInvalidPrompt(t *testing.T) {
	t.Parallel()
	valid := mechanismStudyPromptFixture()
	tests := []struct {
		name   string
		client *Client
		prompt mechanismstudy.Prompt
	}{
		{name: "nil client", prompt: valid},
		{
			name: "unsupported version", client: &Client{},
			prompt: mechanismstudy.Prompt{Version: "future", System: valid.System, User: valid.User},
		},
		{
			name: "blank system", client: &Client{},
			prompt: mechanismstudy.Prompt{Version: valid.Version, System: " \n\t", User: valid.User},
		},
		{
			name: "blank user", client: &Client{},
			prompt: mechanismstudy.Prompt{Version: valid.Version, System: valid.System, User: " \n\t"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := test.client.MechanismStudyPromptJSON(test.prompt); err == nil {
				t.Fatal("MechanismStudyPromptJSON() error = nil")
			}
		})
	}
}

func TestMechanismStudyBodyMeasuredSendsExactImmutableBody(t *testing.T) {
	t.Parallel()
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		sent, _ = io.ReadAll(request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"version\":1,\"cards\":[]}"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_cache_hit_tokens":3,"prompt_cache_miss_tokens":8}}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	exactBody := []byte("{\n  \"exact\": \"request bytes\"\n}\n")
	result, err := client.MechanismStudyBodyMeasured(t.Context(), exactBody)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sent, exactBody) {
		t.Fatalf("sent body differs\nsent: %q\nwant: %q", sent, exactBody)
	}
	if result.Attempts != 1 || result.RequestBytes != len(exactBody) ||
		string(result.Content) != `{"version":1,"cards":[]}` || result.FinishReason != "stop" ||
		result.ChoiceCount != 1 || !result.UsageReported || result.InputTokens != 11 ||
		result.OutputTokens != 7 || result.PromptCacheHitTokens != 3 ||
		result.PromptCacheMissTokens != 8 {
		t.Fatalf("measured result = %#v", result)
	}
}

func TestMechanismStudyBodyMeasuredRetriesOnlyExactTransportBytes(t *testing.T) {
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies = append(bodies, append([]byte(nil), body...))
		if len(bodies) == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	exactBody := []byte(`{"opaque":"body"}`)
	result, err := client.MechanismStudyBodyMeasured(t.Context(), exactBody)
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || result.Attempts != 2 || result.RequestBytes != 2*len(exactBody) {
		t.Fatalf("calls/result = %d/%#v", len(bodies), result)
	}
	for index, body := range bodies {
		if !bytes.Equal(body, exactBody) {
			t.Fatalf("transport retry mutated body %d: %q", index, body)
		}
	}
}

func TestMechanismStudyBodyMeasuredDoesNotRetrySemanticContent(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"not-json"}}]}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	result, err := client.MechanismStudyBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Attempts != 1 || string(result.Content) != "not-json" {
		t.Fatalf("semantic content was retried or rewritten: calls=%d result=%#v", calls, result)
	}
}

func TestMechanismStudyBodyMeasuredPreservesNon2xxResponseEvidence(t *testing.T) {
	t.Parallel()
	var calls int
	response := []byte(`{"error":"unsupported response_format"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	exactBody := []byte(`{"exact":true}`)
	result, err := client.MechanismStudyBodyMeasured(t.Context(), exactBody)
	if err == nil || !strings.Contains(err.Error(), "status 400") ||
		!strings.Contains(err.Error(), "unsupported response_format") {
		t.Fatalf("MechanismStudyBodyMeasured() error = %v", err)
	}
	if calls != 1 || result.Attempts != 1 || result.RequestBytes != len(exactBody) ||
		result.ResponseBytes != len(response) || !bytes.Equal(result.Content, response) {
		t.Fatalf("calls/result = %d/%#v", calls, result)
	}
}

func TestMechanismStudyBodyMeasuredPreservesMalformedEnvelopeEvidenceWithoutRetry(t *testing.T) {
	t.Parallel()
	var calls int
	response := []byte(`{"choices":[`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	exactBody := []byte(`{"exact":true}`)
	result, err := client.MechanismStudyBodyMeasured(t.Context(), exactBody)
	if !errors.Is(err, errResponseEnvelopeMalformed) {
		t.Fatalf("MechanismStudyBodyMeasured() error = %v, want malformed envelope", err)
	}
	if calls != 1 || result.Attempts != 1 || result.RequestBytes != len(exactBody) ||
		result.ResponseBytes != len(response) || !bytes.Equal(result.Content, response) {
		t.Fatalf("semantic response was retried or lost: calls=%d result=%#v", calls, result)
	}
}

func TestMechanismStudyBodyMeasuredTreatsLengthAsTerminal(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}],"usage":{"prompt_tokens":20,"completion_tokens":64000}}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	result, err := client.MechanismStudyBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 1 || result.Attempts != 1 ||
		limitErr.Stage != mechanismStudyStage ||
		limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		limitErr.Limit != client.MaxTokens || limitErr.FinishReason != "length" {
		t.Fatalf("terminal result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}

func TestMechanismStudyBodyMeasuredHeartbeatIsContentFree(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseServer := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseServer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	updates := make(chan WaitProgress, 1)
	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000, waitInterval: time.Millisecond,
		OnWait: func(progress WaitProgress) {
			select {
			case updates <- progress:
			default:
			}
		},
	}
	secretBody := []byte(`{"private_prompt_marker":"must-not-be-in-heartbeat"}`)
	ctx := t.Context()
	done := make(chan error, 1)
	go func() {
		_, err := client.MechanismStudyBodyMeasured(ctx, secretBody)
		done <- err
	}()

	select {
	case update := <-updates:
		if update.Stage != "Study mechanism identification" || update.Elapsed <= 0 ||
			strings.Contains(update.Stage, "private_prompt_marker") {
			t.Fatalf("unsafe heartbeat = %#v", update)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for mechanism heartbeat")
	}
	releaseServer()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMechanismStudyBodyMeasuredRejectsMissingInputs(t *testing.T) {
	t.Parallel()
	if _, err := (*Client)(nil).MechanismStudyBodyMeasured(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("nil client accepted")
	}
	if _, err := (&Client{}).MechanismStudyBodyMeasured(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("nil HTTP client accepted")
	}
	if _, err := (&Client{HTTPClient: http.DefaultClient}).MechanismStudyBodyMeasured(t.Context(), nil); err == nil {
		t.Fatal("empty body accepted")
	}
}
