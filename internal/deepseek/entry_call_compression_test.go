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

	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func entryCallCompressionPromptFixture() entrycall.Prompt {
	return entrycall.Prompt{
		Version: entrycall.PromptVersion,
		System:  "Use only supplied refs. Return exactly one JSON object.",
		User:    `Response JSON schema: {"version":1,"request_ref":"q-fixture","entries":[]}. Exact request JSON: {"entries":[]}`,
	}
}

func TestEntryCallCompressionPromptJSONPreservesExactGenericContract(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions",
		Model:    "fixture-model", MaxTokens: 64_000,
	}
	prompt := entryCallCompressionPromptFixture()

	got, err := client.EntryCallCompressionPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(chatRequest{
		Model: client.Model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System + "\n\n" + canonicalEnglishSystemContract},
			{Role: "user", Content: canonicalEnglishUserContract + "\n\n" + prompt.User},
		},
		Temperature: float64Pointer(0.1), MaxTokens: client.MaxTokens,
		ResponseFormat: &jsonFormat{Type: "json_object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("request JSON differs\ngot:  %s\nwant: %s", got, want)
	}
	if bytes.Contains(got, []byte(`"thinking"`)) {
		t.Fatalf("generic entry-call request gained an unowned extension: %s", got)
	}
}

func TestEntryCallCompressionPromptJSONDisablesOfficialDeepSeekThinking(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint: "https://api.deepseek.com/chat/completions",
		Model:    "deepseek-v4-flash", MaxTokens: 64_000,
	}
	body, err := client.EntryCallCompressionPromptJSON(entryCallCompressionPromptFixture())
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.Thinking == nil || request.Thinking.Type != "disabled" ||
		request.MaxTokens != client.MaxTokens || request.ResponseFormat == nil ||
		request.ResponseFormat.Type != "json_object" {
		t.Fatalf("request contract = %#v", request)
	}
}

func TestEntryCallCompressionPromptJSONRejectsInvalidPrompt(t *testing.T) {
	t.Parallel()
	valid := entryCallCompressionPromptFixture()
	tests := []struct {
		name   string
		client *Client
		prompt entrycall.Prompt
	}{
		{name: "nil client", prompt: valid},
		{name: "unsupported version", client: &Client{}, prompt: entrycall.Prompt{Version: "future", System: valid.System, User: valid.User}},
		{name: "blank system", client: &Client{}, prompt: entrycall.Prompt{Version: valid.Version, System: " \n\t", User: valid.User}},
		{name: "blank user", client: &Client{}, prompt: entrycall.Prompt{Version: valid.Version, System: valid.System, User: " \n\t"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := test.client.EntryCallCompressionPromptJSON(test.prompt); err == nil {
				t.Fatal("EntryCallCompressionPromptJSON() error = nil")
			}
		})
	}
}

func TestEntryCallCompressionPromptJSONCapsExactProviderEnvelope(t *testing.T) {
	t.Parallel()
	client := &Client{Model: "fixture-model", MaxTokens: 64_000}
	prompt := entryCallCompressionPromptFixture()
	prompt.User = strings.Repeat("x", entrycall.MaxRequestBytes)

	_, err := client.EntryCallCompressionPromptJSON(prompt)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != entryCallCompressionStage ||
		limitErr.Kind != modelresearch.ResourceLimitRequestBytes ||
		limitErr.Limit != entrycall.MaxRequestBytes || limitErr.Observed <= limitErr.Limit ||
		!limitErr.ObservedKnown || limitErr.ConfiguredMaxTokens != client.MaxTokens {
		t.Fatalf("request limit error = %#v / %v", limitErr, err)
	}
}

func TestEntryCallCompressionBodyMeasuredSendsExactImmutableBody(t *testing.T) {
	t.Parallel()
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		sent, _ = io.ReadAll(request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"version\":1,\"request_ref\":\"q-fixture\",\"entries\":[]}"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_cache_hit_tokens":3,"prompt_cache_miss_tokens":8}}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	exactBody := []byte("{\n  \"exact\": \"request bytes\"\n}\n")
	result, err := client.EntryCallCompressionBodyMeasured(t.Context(), exactBody)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sent, exactBody) {
		t.Fatalf("sent body differs\nsent: %q\nwant: %q", sent, exactBody)
	}
	if result.Attempts != 1 || result.RequestBytes != len(exactBody) ||
		string(result.Content) != `{"version":1,"request_ref":"q-fixture","entries":[]}` ||
		result.FinishReason != "stop" || result.ChoiceCount != 1 ||
		!result.UsageReported || result.InputTokens != 11 || result.OutputTokens != 7 ||
		result.PromptCacheHitTokens != 3 || result.PromptCacheMissTokens != 8 {
		t.Fatalf("measured result = %#v", result)
	}
}

func TestEntryCallCompressionBodyMeasuredRetriesOnlyExactTransportBytes(t *testing.T) {
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
	result, err := client.EntryCallCompressionBodyMeasured(t.Context(), exactBody)
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

func TestEntryCallCompressionBodyMeasuredDoesNotRetrySemanticContent(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"not-json"}}]}`)
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone, MaxTokens: 64_000}

	result, err := client.EntryCallCompressionBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Attempts != 1 || string(result.Content) != "not-json" {
		t.Fatalf("semantic content was retried or rewritten: calls=%d result=%#v", calls, result)
	}
}

func TestEntryCallCompressionBodyMeasuredRejectsOversizeBodyBeforeNetwork(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()
	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		MaxTokens: 64_000,
	}
	body := bytes.Repeat([]byte("x"), entrycall.MaxRequestBytes+1)

	result, err := client.EntryCallCompressionBodyMeasured(t.Context(), body)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 0 || result.Attempts != 0 ||
		limitErr.Stage != entryCallCompressionStage || limitErr.Kind != modelresearch.ResourceLimitRequestBytes ||
		limitErr.Limit != entrycall.MaxRequestBytes || limitErr.Observed != len(body) ||
		!limitErr.ObservedKnown || limitErr.ConfiguredMaxTokens != client.MaxTokens {
		t.Fatalf("pre-call result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}

func TestEntryCallCompressionBodyMeasuredPreservesBoundedErrorEvidence(t *testing.T) {
	t.Parallel()
	t.Run("non-2xx", func(t *testing.T) {
		response := []byte(`{"error":"unsupported response_format"}`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(response)
		}))
		defer server.Close()
		client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone, MaxTokens: 64_000}

		result, err := client.EntryCallCompressionBodyMeasured(t.Context(), []byte(`{"exact":true}`))
		if err == nil || !strings.Contains(err.Error(), "status 400") ||
			!bytes.Equal(result.Content, response) || result.ResponseBytes != len(response) || result.Attempts != 1 {
			t.Fatalf("result/error = %#v / %v", result, err)
		}
	})
	t.Run("malformed envelope", func(t *testing.T) {
		response := []byte(`{"choices":[`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(response)
		}))
		defer server.Close()
		client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone, MaxTokens: 64_000}

		result, err := client.EntryCallCompressionBodyMeasured(t.Context(), []byte(`{"exact":true}`))
		if !errors.Is(err, errResponseEnvelopeMalformed) || !bytes.Equal(result.Content, response) ||
			result.ResponseBytes != len(response) || result.Attempts != 1 {
			t.Fatalf("result/error = %#v / %v", result, err)
		}
	})
}

func TestEntryCallCompressionBodyMeasuredCapsSemanticResponse(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("x", entrycall.MaxResponseBytes+1)
	response, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"content": content},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone, MaxTokens: 64_000}

	result, callErr := client.EntryCallCompressionBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(callErr, &limitErr) || calls != 1 || result.Attempts != 1 ||
		limitErr.Stage != entryCallCompressionStage || limitErr.Kind != modelresearch.ResourceLimitResponseBytes ||
		limitErr.Limit != entrycall.MaxResponseBytes || limitErr.Observed != len(content) ||
		!limitErr.ObservedKnown || !bytes.Equal(limitErr.ProviderContent(), []byte(content)) {
		t.Fatalf("terminal result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, callErr)
	}
}

func TestEntryCallCompressionBodyMeasuredTreatsLengthAsTerminal(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}],"usage":{"prompt_tokens":20,"completion_tokens":64000}}`)
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone, MaxTokens: 64_000}

	result, err := client.EntryCallCompressionBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 1 || result.Attempts != 1 ||
		limitErr.Stage != entryCallCompressionStage || limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		limitErr.Limit != client.MaxTokens || limitErr.FinishReason != "length" {
		t.Fatalf("terminal result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}

func TestEntryCallCompressionBodyMeasuredHeartbeatIsContentFree(t *testing.T) {
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
		MaxTokens: 64_000, waitInterval: time.Millisecond,
		OnWait: func(progress WaitProgress) {
			select {
			case updates <- progress:
			default:
			}
		},
	}
	secretBody := []byte(`{"private_prompt_marker":"must-not-be-in-heartbeat"}`)
	done := make(chan error, 1)
	go func() {
		_, err := client.EntryCallCompressionBodyMeasured(t.Context(), secretBody)
		done <- err
	}()

	select {
	case update := <-updates:
		if update.Stage != "Entrypoint call compression" || update.Elapsed <= 0 ||
			strings.Contains(update.Stage, "private_prompt_marker") {
			t.Fatalf("unsafe heartbeat = %#v", update)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for entry-call heartbeat")
	}
	releaseServer()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEntryCallCompressionBodyMeasuredRejectsMissingInputs(t *testing.T) {
	t.Parallel()
	if _, err := (*Client)(nil).EntryCallCompressionBodyMeasured(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("nil client accepted")
	}
	if _, err := (&Client{}).EntryCallCompressionBodyMeasured(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("nil HTTP client accepted")
	}
	if _, err := (&Client{HTTPClient: http.DefaultClient}).EntryCallCompressionBodyMeasured(t.Context(), nil); err == nil {
		t.Fatal("empty body accepted")
	}
}
