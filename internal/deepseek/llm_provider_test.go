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

func TestLLMProviderSupportsConcurrentExecutorBatch(t *testing.T) {
	const callCount = 3
	started := make(chan struct{}, callCount)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()
	var (
		mu        sync.Mutex
		active    int
		maxActive int
		requests  int
	)
	response := llmProviderResponse("stop", `{"ok":true}`, nil)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		_, _ = io.ReadAll(request.Body)
		mu.Lock()
		active++
		requests++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		defer func() {
			mu.Lock()
			active--
			mu.Unlock()
		}()
		started <- struct{}{}
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(response)
	}))
	defer server.Close()

	client := llmProviderTestClient(server)
	calls := make([]llm.Call[map[string]any], 0, callCount)
	for _, user := range []string{"one", "two", "three"} {
		calls = append(calls, llm.Call[map[string]any]{
			State: []byte("cube-" + user),
			Prompt: llm.Prompt{
				System: "Return one bounded JSON object.", User: user, ResponseFormatJSON: true,
			},
			Limits: llmProviderTestLimits(100),
		})
	}
	type result struct {
		outcomes []llm.Outcome[map[string]any]
		err      error
	}
	done := make(chan result, 1)
	root := t.TempDir()
	go func() {
		outcomes, err := llm.ExecuteJSONBatch(context.Background(), llm.Executor{
			RootDir: root, Enabled: true, BatchConcurrency: callCount,
		}, client, calls)
		done <- result{outcomes: outcomes, err: err}
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for range callCount {
		select {
		case <-started:
		case <-timer.C:
			t.Fatal("provider calls did not execute concurrently")
		}
	}
	releaseAll()
	got := <-done
	if got.err != nil || len(got.outcomes) != callCount {
		t.Fatalf("concurrent batch = %#v / %v", got.outcomes, got.err)
	}
	for index, outcome := range got.outcomes {
		if value, ok := outcome.Value["ok"].(bool); !ok || !value || outcome.Cached {
			t.Fatalf("outcome %d = %#v", index, outcome)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != callCount || maxActive != callCount || active != 0 {
		t.Fatalf("requests/max-active/active = %d/%d/%d", requests, maxActive, active)
	}
}

func TestLLMProviderRateLimitCollapsesSharedAttemptGateBeforeRetry(t *testing.T) {
	type attemptEvent struct {
		user    string
		attempt int
		active  int
	}
	started := make(chan attemptEvent, 16)
	releases := map[string]chan struct{}{
		"one": make(chan struct{}), "two": make(chan struct{}), "three": make(chan struct{}),
		"four": make(chan struct{}), "five": make(chan struct{}),
	}
	var (
		mu        sync.Mutex
		attempts  = make(map[string]int)
		active    int
		maxActive int
	)
	success := llmProviderResponse("stop", `{"ok":true}`, nil)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		_ = request.Body.Close()
		var wire chatRequest
		if err := json.Unmarshal(body, &wire); err != nil || len(wire.Messages) != 2 {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		user := wire.Messages[1].Content
		mu.Lock()
		attempts[user]++
		attempt := attempts[user]
		active++
		if active > maxActive {
			maxActive = active
		}
		currentActive := active
		mu.Unlock()
		defer func() {
			mu.Lock()
			active--
			mu.Unlock()
		}()
		started <- attemptEvent{user: user, attempt: attempt, active: currentActive}
		if attempt == 1 {
			if release := releases[user]; release != nil {
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
			}
		}
		if user == "two" && attempt == 1 {
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = writer.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(success)
	}))
	defer server.Close()
	defer func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
	}()

	callFor := func(users ...string) []llm.Call[map[string]any] {
		calls := make([]llm.Call[map[string]any], 0, len(users))
		for _, user := range users {
			calls = append(calls, llm.Call[map[string]any]{
				Prompt: llm.Prompt{
					System: "Return one bounded JSON object.", User: user, ResponseFormatJSON: true,
				},
				Limits: llmProviderTestLimits(100),
			})
		}
		return calls
	}
	type result struct {
		outcomes []llm.Outcome[map[string]any]
		err      error
	}
	nextAttempt := func() attemptEvent {
		t.Helper()
		select {
		case event := <-started:
			return event
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for provider attempt")
			return attemptEvent{}
		}
	}
	controller := &llm.BatchController{}
	executor := llm.Executor{
		Enabled: false, BatchConcurrency: 3, BatchController: controller,
	}
	client := llmProviderTestClient(server)
	firstDone := make(chan result, 1)
	go func() {
		outcomes, err := llm.ExecuteJSONBatch(
			t.Context(), executor, client, callFor("one", "two", "three"),
		)
		firstDone <- result{outcomes: outcomes, err: err}
	}()
	initial := make(map[string]attemptEvent)
	for len(initial) < 3 {
		select {
		case event := <-started:
			initial[event.user] = event
		case <-time.After(time.Second):
			t.Fatalf("initial attempts = %v", initial)
		}
	}
	if initial["one"].attempt != 1 || initial["two"].attempt != 1 ||
		initial["three"].attempt != 1 {
		t.Fatalf("initial attempts = %#v", initial)
	}
	close(releases["two"])
	select {
	case event := <-started:
		t.Fatalf("attempt started while earlier leases remained active: %#v", event)
	case <-time.After(600 * time.Millisecond):
	}
	close(releases["one"])
	select {
	case event := <-started:
		t.Fatalf("attempt started with one earlier lease active: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
	close(releases["three"])
	var retry attemptEvent
	select {
	case retry = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("rate-limited request did not retry")
	}
	if retry.user != "two" || retry.attempt != 2 || retry.active != 1 {
		t.Fatalf("serialized retry = %#v", retry)
	}
	first := <-firstDone
	if first.err != nil || len(first.outcomes) != 3 {
		t.Fatalf("transient rate-limit batch = %#v / %v", first.outcomes, first.err)
	}

	secondDone := make(chan result, 1)
	go func() {
		outcomes, err := llm.ExecuteJSONBatch(
			t.Context(), executor, client, callFor("four", "five"),
		)
		secondDone <- result{outcomes: outcomes, err: err}
	}()
	four := nextAttempt()
	if four.user != "four" || four.attempt != 1 || four.active != 1 {
		t.Fatalf("first later attempt = %#v", four)
	}
	select {
	case event := <-started:
		t.Fatalf("shared collapsed gate admitted concurrent later attempt: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
	close(releases["four"])
	five := nextAttempt()
	if five.user != "five" || five.attempt != 1 || five.active != 1 {
		t.Fatalf("second later attempt = %#v", five)
	}
	close(releases["five"])
	second := <-secondDone
	if second.err != nil || len(second.outcomes) != 2 {
		t.Fatalf("later serialized batch = %#v / %v", second.outcomes, second.err)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxActive != 3 || active != 0 || attempts["two"] != 2 {
		t.Fatalf("max-active/active/attempts = %d/%d/%v", maxActive, active, attempts)
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
	var source llm.ProviderFailureSource
	if !errors.As(err, &source) {
		t.Fatalf("provider failure has no structured source: %v", err)
	}
	failure := source.ProviderFailure()
	if failure.Kind != llm.ProviderFailureHTTPStatus || failure.HTTPStatus != http.StatusBadRequest ||
		failure.Attempts != 1 || failure.RetryExhausted {
		t.Fatalf("HTTP provider failure = %#v", failure)
	}

	_, outerErr := llm.ExecuteJSON[map[string]any](
		context.Background(), llm.Executor{Enabled: false}, client, llmProviderFailureCall(),
	)
	var providerErr *llm.ProviderError
	if !errors.As(outerErr, &providerErr) {
		t.Fatalf("outer provider error = %v", outerErr)
	}
	rendered := outerErr.Error()
	if !strings.Contains(rendered, "class=http_status status=400 attempts=1") ||
		!strings.Contains(rendered, "check provider endpoint, request compatibility, and account access") ||
		strings.Contains(rendered, secret) || strings.Contains(rendered, "api_key") {
		t.Fatalf("outer provider failure = %q", rendered)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Complete(canceled, prepared)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("closed error lost cancellation cause: %v", err)
	}
}

func TestLLMProviderHTTPRetryExhaustionIsStructured(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()
	client := llmProviderTestClient(server)
	outcome, err := llm.ExecuteJSON[map[string]any](
		context.Background(), llm.Executor{Enabled: false}, client, llmProviderFailureCall(),
	)
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("retry exhaustion error = %v", err)
	}
	failure := providerErr.ProviderFailure()
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if failure.Kind != llm.ProviderFailureHTTPStatus || failure.HTTPStatus != http.StatusTooManyRequests ||
		failure.Attempts != maxRetries+1 || !failure.RetryExhausted ||
		outcome.Metrics.Attempts != maxRetries+1 || gotCalls != maxRetries+1 {
		t.Fatalf("retry exhaustion = %#v / metrics=%#v / calls=%d", failure, outcome.Metrics, gotCalls)
	}
	rendered := err.Error()
	if !strings.Contains(rendered, "class=http_status status=429 attempts=4 retries_exhausted=true") ||
		!strings.Contains(rendered, "check provider rate limits or quota, then retry") {
		t.Fatalf("retry exhaustion error = %q", rendered)
	}
}

func TestLLMProviderTimeoutIsStructuredWithoutRetry(t *testing.T) {
	client := &Client{
		HTTPClient: &http.Client{Transport: failingRoundTripper{err: context.DeadlineExceeded}},
		Model:      "test-model", MaxTokens: 100,
		Endpoint: "https://provider.example/v1/chat/completions", Auth: authNone,
	}
	outcome, err := llm.ExecuteJSON[map[string]any](
		context.Background(), llm.Executor{Enabled: false}, client, llmProviderFailureCall(),
	)
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	failure := providerErr.ProviderFailure()
	if failure.Kind != llm.ProviderFailureTimeout || failure.Attempts != 1 ||
		failure.RetryExhausted || outcome.Metrics.Attempts != 1 {
		t.Fatalf("timeout failure = %#v / metrics=%#v", failure, outcome.Metrics)
	}
	rendered := err.Error()
	if !strings.Contains(rendered, "class=timeout attempts=1") ||
		!strings.Contains(rendered, "check provider latency or increase the configured timeout") {
		t.Fatalf("timeout error = %q", rendered)
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

func llmProviderFailureCall() llm.Call[map[string]any] {
	return llm.Call[map[string]any]{
		Prompt: llm.Prompt{
			System: "Return one bounded JSON object.", User: "fixture", ResponseFormatJSON: true,
		},
		Limits: llmProviderTestLimits(100),
	}
}

type failingRoundTripper struct {
	err error
}

func (transport failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, transport.err
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
