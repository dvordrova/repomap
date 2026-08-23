package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestLLMProviderPrepareUsesOnlyCubePromptAndEffectiveLimit(t *testing.T) {
	client := &Client{
		HTTPClient: &http.Client{Timeout: 7 * time.Second},
		APIKey:     "sk-configured-secret",
		Model:      "test-model",
		MaxTokens:  400,
		Endpoint:   "https://provider.example/v1/chat/completions",
		Auth:       authBearer,
	}
	prompt := llm.Prompt{
		System:             "cube system contract",
		User:               "cube user catalog",
		ResponseFormatJSON: true,
	}
	prepared, err := client.Prepare(prompt, llmProviderTestLimits(900))
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.Model != client.Model || request.MaxTokens != client.MaxTokens ||
		request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" ||
		request.Thinking != nil || len(request.Messages) != 2 ||
		request.Messages[0].Role != "system" || request.Messages[0].Content != prompt.System ||
		request.Messages[1].Role != "user" || request.Messages[1].Content != prompt.User {
		t.Fatalf("prepared request = %#v", request)
	}
	if bytes.Contains(prepared.Bytes(), []byte(client.APIKey)) ||
		bytes.Contains(client.State(), []byte(client.APIKey)) {
		t.Fatal("configured API key entered prepared request or provider state")
	}

	mutated := prepared.Bytes()
	mutated[0] = '['
	if prepared.Bytes()[0] != '{' {
		t.Fatal("prepared request bytes are mutable through Bytes")
	}

	official := *client
	official.Endpoint = defaultEndpoint
	official.Auth = authNone
	official.APIKey = ""
	officialPrepared, err := official.Prepare(prompt, llmProviderTestLimits(200))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(officialPrepared.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.MaxTokens != 200 || request.Thinking == nil || request.Thinking.Type != "disabled" {
		t.Fatalf("official request output controls = %#v", request)
	}
}

func TestLLMProviderStateIsStableAndCredentialFree(t *testing.T) {
	base := &Client{
		HTTPClient: &http.Client{Timeout: 3 * time.Second},
		APIKey:     "sk-first-secret",
		Model:      "model-a",
		MaxTokens:  1234,
		Endpoint:   "https://provider.example/v1/chat/completions",
		Auth:       authBearer,
	}
	state := base.State()
	if !json.Valid(state) || bytes.Contains(state, []byte(base.APIKey)) ||
		bytes.Contains(bytes.ToLower(state), []byte("api_key")) ||
		!bytes.Equal(state, base.State()) {
		t.Fatalf("provider state is not stable credential-free JSON: %s", state)
	}

	changedSecret := *base
	changedSecret.APIKey = "sk-second-secret"
	if !bytes.Equal(state, changedSecret.State()) {
		t.Fatal("API key changed cache identity")
	}
	changedModel := *base
	changedModel.Model = "model-b"
	if bytes.Equal(state, changedModel.State()) {
		t.Fatal("model change did not change provider state")
	}
}

func TestLLMProviderCompleteAuthNoneMetricsAndHeartbeat(t *testing.T) {
	response := llmProviderResponse(
		"stop",
		"  ```json\n{\"ok\":true}\n```  ",
		map[string]any{
			"prompt_tokens": 11, "completion_tokens": 7,
			"prompt_cache_hit_tokens": 5, "prompt_cache_miss_tokens": 2,
			"completion_tokens_details": map[string]any{"reasoning_tokens": 3},
		},
	)
	var (
		gotBody []byte
		gotAuth string
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuth = request.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(request.Body)
		time.Sleep(12 * time.Millisecond)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(response)
	}))
	defer server.Close()

	var (
		waitMu sync.Mutex
		waits  []WaitProgress
	)
	client := llmProviderTestClient(server)
	client.APIKey = "sk-must-not-be-sent"
	client.waitInterval = time.Millisecond
	client.OnWait = func(progress WaitProgress) {
		waitMu.Lock()
		defer waitMu.Unlock()
		waits = append(waits, progress)
	}
	exact := []byte(`{"request":"exact"}`)
	prepared, err := llm.NewPrepared(exact)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := client.Complete(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" || !bytes.Equal(gotBody, exact) {
		t.Fatalf("auth/body = %q / %s", gotAuth, gotBody)
	}
	if string(completion.Response) != "```json\n{\"ok\":true}\n```" ||
		completion.FinishReason != llm.FinishStop || completion.ChoiceCount != 1 ||
		completion.Metrics.Attempts != 1 ||
		completion.Metrics.ProviderResponseBytes != len(response) ||
		completion.Metrics.InputTokens != 11 || completion.Metrics.OutputTokens != 7 ||
		completion.Metrics.ReasoningTokens != 3 ||
		completion.Metrics.PromptCacheHitTokens != 5 ||
		completion.Metrics.PromptCacheMissTokens != 2 ||
		!completion.Metrics.UsageReported || completion.Metrics.Latency <= 0 {
		t.Fatalf("completion = %#v", completion)
	}
	waitMu.Lock()
	defer waitMu.Unlock()
	if len(waits) == 0 {
		t.Fatal("long provider call emitted no heartbeat")
	}
	for _, progress := range waits {
		if progress.Stage != llmProviderHeartbeat ||
			strings.Contains(progress.Stage, "cube") ||
			strings.Contains(progress.Stage, "exact") ||
			progress.Elapsed <= 0 {
			t.Fatalf("heartbeat contains request content or invalid time: %#v", progress)
		}
	}
}

func TestLLMProviderCompleteRetriesExactBodyAndAccumulatesRawBytes(t *testing.T) {
	success := llmProviderResponse("stop", `{"ok":true}`, nil)
	failure := []byte("temporary provider failure")
	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), body...))
		attempt := len(bodies)
		mu.Unlock()
		if attempt == 1 {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write(failure)
			return
		}
		_, _ = writer.Write(success)
	}))
	defer server.Close()

	client := llmProviderTestClient(server)
	exact := []byte(`{"stable":[1,2,3]}`)
	prepared, err := llm.NewPrepared(exact)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := client.Complete(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || !bytes.Equal(bodies[0], exact) || !bytes.Equal(bodies[1], exact) {
		t.Fatalf("retry bodies = %q", bodies)
	}
	if completion.Metrics.Attempts != 2 ||
		completion.Metrics.ProviderResponseBytes != len(failure)+len(success) {
		t.Fatalf("retry metrics = %#v", completion.Metrics)
	}
}

func TestLLMProviderCompletePreservesTerminalLengthResourceOutcome(t *testing.T) {
	response := llmProviderResponse(
		"length", `{"partial":true}`,
		map[string]any{
			"prompt_tokens": 9, "completion_tokens": 100,
			"completion_tokens_details": map[string]any{"reasoning_tokens": 4},
		},
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(response)
	}))
	defer server.Close()
	client := llmProviderTestClient(server)
	prepared, _ := llm.NewPrepared([]byte(`{"request":true}`))
	completion, err := client.Complete(context.Background(), prepared)
	if err == nil {
		t.Fatal("length completion was accepted")
	}
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != ResourceLimitOutputTokens ||
		limitErr.Limit != client.MaxTokens || limitErr.Observed != 100 ||
		!limitErr.ObservedKnown ||
		completion.FinishReason != llm.FinishLength || completion.Metrics.Attempts != 1 ||
		completion.Metrics.ProviderResponseBytes != len(response) {
		t.Fatalf("length outcome = %#v / %#v / %v", completion, limitErr, err)
	}
}

func TestLLMProviderErrorsAreClosedButRetainTypedCause(t *testing.T) {
	secret := "sk-secret-shaped-provider-error"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"api_key":"` + secret + `"}`))
	}))
	defer server.Close()
	client := llmProviderTestClient(server)
	prepared, _ := llm.NewPrepared([]byte(`{"request":true}`))
	completion, err := client.Complete(context.Background(), prepared)
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "api_key") ||
		!bytes.Contains(completion.Response, []byte(secret)) || completion.Metrics.Attempts != 1 {
		t.Fatalf("closed provider failure = %#v / %v", completion, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Complete(canceled, prepared)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("closed error lost cancellation cause: %v", err)
	}
}

func TestLLMProviderPrepareEnforcesRequestBound(t *testing.T) {
	client := &Client{
		HTTPClient: &http.Client{}, Model: "test", MaxTokens: 10,
		Endpoint: "https://provider.example/v1/chat/completions", Auth: authNone,
	}
	_, err := client.Prepare(llm.Prompt{
		System: "system", User: strings.Repeat("x", 100), ResponseFormatJSON: true,
	}, llm.Limits{MaxRequestBytes: 32, MaxResponseBytes: 1024, MaxOutputTokens: 10})
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != ResourceLimitRequestBytes ||
		limitErr.Limit != 32 || !limitErr.ObservedKnown || limitErr.Observed <= limitErr.Limit {
		t.Fatalf("request bound = %#v / %v", limitErr, err)
	}
}

func llmProviderTestLimits(maxOutputTokens int) llm.Limits {
	return llm.Limits{
		MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20,
		MaxOutputTokens: maxOutputTokens,
	}
}

func llmProviderTestClient(server *httptest.Server) *Client {
	return &Client{
		HTTPClient: server.Client(), Model: "test-model", MaxTokens: 100,
		Endpoint: server.URL, Auth: authNone,
	}
}

func llmProviderResponse(finishReason, content string, usage map[string]any) []byte {
	response := map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": finishReason,
			"message":       map[string]any{"role": "assistant", "content": content},
		}},
	}
	if usage != nil {
		response["usage"] = usage
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	return encoded
}
