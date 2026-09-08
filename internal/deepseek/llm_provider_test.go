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
	"testing/synctest"
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
		string(request.ChatTemplateKwargs["enable_thinking"]) != "false" ||
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
	request = chatRequest{}
	if err := json.Unmarshal(officialPrepared.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.MaxTokens != 200 || request.Thinking == nil || request.Thinking.Type != "disabled" || request.ChatTemplateKwargs != nil {
		t.Fatalf("official request output controls = %#v", request)
	}
	prompt.Reasoning = true
	reasoned, err := official.Prepare(prompt, llmProviderTestLimits(900))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(reasoned.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.Thinking == nil || request.Thinking.Type != "enabled" || request.MaxTokens != client.MaxTokens {
		t.Fatalf("reasoning must retain the configured output ceiling: %#v", request)
	}
	withoutReasoning := prompt
	withoutReasoning.Reasoning = false
	plain, err := official.Prepare(withoutReasoning, llmProviderTestLimits(900))
	if err != nil || bytes.Equal(reasoned.Bytes(), plain.Bytes()) {
		t.Fatal("reasoning preference must change the exact request cache identity")
	}
	compatible, err := client.Prepare(prompt, llmProviderTestLimits(900))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(prepared.Bytes(), compatible.Bytes()) {
		t.Fatal("compatible reasoning preference must change the exact request cache identity")
	}
	if err := json.Unmarshal(compatible.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if string(request.ChatTemplateKwargs["enable_thinking"]) != "true" || request.MaxTokens != client.MaxTokens {
		t.Fatal("compatible request did not enable reasoning with the configured output allowance")
	}
}

func TestLLMProviderChatTemplateKwargsControlExactRequest(t *testing.T) {
	client := &Client{
		HTTPClient: &http.Client{}, Model: "qwen-test", MaxTokens: 400,
		Endpoint: "https://provider.example/v1/chat/completions", Auth: authNone,
	}
	bodiesByOptions := make(map[string][]byte)
	for _, test := range []struct {
		name      string
		reasoning bool
		kwargs    map[string]json.RawMessage
		want      string
	}{
		{name: "default off", want: `{"enable_thinking":false}`},
		{name: "cube reasoning on", reasoning: true, want: `{"enable_thinking":true}`},
		{name: "explicit omission", kwargs: map[string]json.RawMessage{}},
		{name: "explicit omission with reasoning", reasoning: true, kwargs: map[string]json.RawMessage{}},
		{name: "explicit on overrides fast cube", kwargs: map[string]json.RawMessage{"enable_thinking": json.RawMessage("true")}, want: `{"enable_thinking":true}`},
		{name: "explicit off overrides reasoning cube", reasoning: true, kwargs: map[string]json.RawMessage{"enable_thinking": json.RawMessage("false")}, want: `{"enable_thinking":false}`},
		{name: "explicit object replaces all defaults", reasoning: true, kwargs: map[string]json.RawMessage{"custom_option": json.RawMessage("7")}, want: `{"custom_option":7}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			client.ChatTemplateKwargs = test.kwargs
			before, _ := json.Marshal(client.ChatTemplateKwargs)
			prompt := llm.Prompt{System: "system", User: "user", Reasoning: test.reasoning}
			prepared, err := client.Prepare(prompt, llmProviderTestLimits(400))
			if err != nil {
				t.Fatal(err)
			}
			var request map[string]json.RawMessage
			if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
				t.Fatal(err)
			}
			if string(request["chat_template_kwargs"]) != test.want || request["extra_body"] != nil || request["thinking"] != nil {
				t.Fatalf("wire template options = %s", prepared.Bytes())
			}
			after, _ := json.Marshal(client.ChatTemplateKwargs)
			if !bytes.Equal(before, after) {
				t.Fatal("preparing a cube mutated shared client configuration")
			}
			if previous, ok := bodiesByOptions[test.want]; ok && !bytes.Equal(previous, prepared.Bytes()) {
				t.Fatal("identical effective kwargs must keep the same exact request identity")
			}
			bodiesByOptions[test.want] = prepared.Bytes()
		})
	}
	for options, body := range bodiesByOptions {
		for otherOptions, otherBody := range bodiesByOptions {
			if options != otherOptions && bytes.Equal(body, otherBody) {
				t.Fatal("template options must change the exact request cache identity")
			}
		}
	}
}

func TestCompatibleReasoningKeepsThinkingOutsideAnswerAndSeparatesCache(t *testing.T) {
	var seenMu sync.Mutex
	var seen []bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			Kwargs struct {
				Thinking bool `json:"enable_thinking"`
			} `json:"chat_template_kwargs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		seenMu.Lock()
		seen = append(seen, request.Kwargs.Thinking)
		seenMu.Unlock()
		content := `{"thinking":false}`
		if request.Kwargs.Thinking {
			content = "<think>\n```python\ndraft = {\"thinking\": false}\n```\n</think>\n```json\n{\"thinking\":true}\n```"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(llmProviderResponse("stop", content, nil))
	}))
	defer server.Close()
	client := llmProviderTestClient(server)
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
	keys := make(map[bool]string)
	for i, reasoning := range []bool{false, true, false, true} {
		outcome, err := llm.ExecuteJSON[map[string]bool](t.Context(), executor, client, llm.Call[map[string]bool]{
			State:  []byte("same-cube"),
			Prompt: llm.Prompt{System: "Return one JSON object.", User: "same evidence", ResponseFormatJSON: true, Reasoning: reasoning},
			Limits: llmProviderTestLimits(400),
		})
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Value["thinking"] != reasoning || outcome.Cached != (i >= 2) {
			t.Fatalf("reasoning=%v outcome=%+v", reasoning, outcome)
		}
		if reasoning && !bytes.HasPrefix(outcome.Response, []byte("<think>")) {
			t.Fatal("normalization discarded original thinking bytes from the saved response")
		}
		if previous := keys[reasoning]; previous != "" && previous != outcome.CacheKey {
			t.Fatal("unchanged reasoning did not reuse its request identity")
		}
		keys[reasoning] = outcome.CacheKey
	}
	seenMu.Lock()
	defer seenMu.Unlock()
	if len(seen) != 2 || seen[0] || !seen[1] || keys[false] == keys[true] {
		t.Fatalf("reasoning wire calls or cache identities differ: seen=%v keys=%v", seen, keys)
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
	if !bytes.Equal(state, changedModel.State()) {
		t.Fatal("model defaults should not change transport identity")
	}
	prompt := llm.Prompt{System: "system", User: "input"}
	first, err := base.Prepare(prompt, llmProviderTestLimits(100))
	if err != nil {
		t.Fatal(err)
	}
	second, err := changedModel.Prepare(prompt, llmProviderTestLimits(100))
	if err != nil || bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("model change did not change exact request identity")
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
	synctest.Test(t, testLLMProviderRateLimitCollapsesSharedAttemptGateBeforeRetry)
}

func testLLMProviderRateLimitCollapsesSharedAttemptGateBeforeRetry(t *testing.T) {
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
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})
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
	client := llmProviderHandlerClient(handler)
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
	case <-time.After(61 * time.Second):
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
	client.ChatTemplateKwargs = map[string]json.RawMessage{"enable_thinking": json.RawMessage("true")}
	exact := []byte(`{"stable":[1,2,3],"chat_template_kwargs":{"enable_thinking":false}}`)
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
	synctest.Test(t, testLLMProviderHTTPRetryExhaustionIsStructured)
}

func testLLMProviderHTTPRetryExhaustionIsStructured(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":"rate limited"}`))
	})
	client := llmProviderHandlerClient(handler)
	started := time.Now()
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
	if elapsed := time.Since(started); elapsed != 3*time.Minute {
		t.Fatalf("429 retries waited %v, want one minute before each retry", elapsed)
	}
	rendered := err.Error()
	if !strings.Contains(rendered, "class=http_status status=429 attempts=4 retries_exhausted=true") ||
		!strings.Contains(rendered, "check provider rate limits or quota, then retry") {
		t.Fatalf("retry exhaustion error = %q", rendered)
	}
}

// TestLLMProviderRetriesItsOwnTimeout pins that a slow attempt is tried again
// rather than taking a whole target page with it, and that the failure it
// eventually reports still says what happened.
func TestLLMProviderRetriesItsOwnTimeout(t *testing.T) {
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
	if failure.Kind != llm.ProviderFailureTimeout || failure.Attempts != maxRetries+1 ||
		!failure.RetryExhausted || outcome.Metrics.Attempts != maxRetries+1 {
		t.Fatalf("timeout failure = %#v / metrics=%#v", failure, outcome.Metrics)
	}
	if !strings.Contains(err.Error(), "class=timeout") {
		t.Fatalf("timeout error = %q", err.Error())
	}
}

// TestLLMProviderDoesNotRetryAfterTheCallerGaveUp separates "this attempt was
// slow" from "the run was cancelled". Only the first is worth another attempt.
func TestLLMProviderDoesNotRetryAfterTheCallerGaveUp(t *testing.T) {
	client := &Client{
		HTTPClient: &http.Client{Transport: failingRoundTripper{err: context.Canceled}},
		Model:      "test-model", MaxTokens: 100,
		Endpoint: "https://provider.example/v1/chat/completions", Auth: authNone,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := llm.ExecuteJSON[map[string]any](
		ctx, llm.Executor{Enabled: false}, client, llmProviderFailureCall(),
	)
	if err == nil || outcome.Metrics.Attempts > 1 {
		t.Fatalf("cancelled call = %v / attempts=%d", err, outcome.Metrics.Attempts)
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

type handlerRoundTripper struct{ handler http.Handler }

func (transport handlerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	transport.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func llmProviderHandlerClient(handler http.Handler) *Client {
	return &Client{
		HTTPClient: &http.Client{Transport: handlerRoundTripper{handler}},
		Model:      "test-model", MaxTokens: 100,
		Endpoint: "https://provider.example/chat/completions", Auth: authNone,
	}
}

func TestLLMProviderRateLimitWaitUsesMinuteFloorAndRetryAfter(t *testing.T) {
	for _, test := range []struct {
		name       string
		header     string
		body       string
		dateOffset time.Duration
		want       time.Duration
	}{
		{name: "missing", want: time.Minute},
		{name: "short", header: "5", want: time.Minute},
		{name: "zero", header: "0", want: time.Minute},
		{name: "longer", header: "90", want: 90 * time.Second},
		{name: "date", dateOffset: 2 * time.Minute, want: 2 * time.Minute},
		{name: "past date", dateOffset: -time.Minute, want: time.Minute},
		{name: "invalid", header: "not a date", want: time.Minute},
		{name: "reported body waits below minute", body: "rate limit exceeded: retry after 9.636307001s, reset after 45.636307001s", want: time.Minute},
		{name: "fractional reset extends header", header: "90", body: "rate limit exceeded: retry after 9.636307001s, reset after 95.636307001s", want: 95*time.Second + 636307001*time.Nanosecond},
		{name: "retry longer than reset", body: "rate limit exceeded: retry after 2m0.25s, reset after 95s", want: 2*time.Minute + 250*time.Millisecond},
		{name: "header longer than body", header: "180", body: "rate limit exceeded: retry after 9s, reset after 95s", want: 3 * time.Minute},
		{name: "compatible JSON error", body: `{"error":{"message":"rate limit exceeded: retry after 9.636307001s, reset after 95.636307001s"}}`, want: 95*time.Second + 636307001*time.Nanosecond},
		{name: "JSON string error", body: `{"error":"rate limit exceeded: retry after 9s, reset after 2m"}`, want: 2 * time.Minute},
		{name: "JSON top-level message", body: `{"message":"rate limit exceeded: retry after 9s, reset after 2m."}`, want: 2 * time.Minute},
		{name: "negative waits", body: "rate limit exceeded: retry after -90s, reset after -120s", want: time.Minute},
		{name: "invalid or unitless waits", body: "rate limit exceeded: retry after unknown, reset after 120", want: time.Minute},
		{name: "overflow wait", body: "rate limit exceeded: reset after 999999999999999999999999s", want: time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var bodies [][]byte
				var starts []time.Time
				client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					bodies = append(bodies, body)
					starts = append(starts, time.Now())
					if len(starts) == 1 {
						header := test.header
						if test.dateOffset != 0 {
							header = time.Now().Add(test.dateOffset).UTC().Format(http.TimeFormat)
						}
						w.Header().Set("Retry-After", header)
						w.WriteHeader(http.StatusTooManyRequests)
						_, _ = io.WriteString(w, test.body)
						return
					}
					_, _ = w.Write(llmProviderResponse("stop", `{"ok":true}`, nil))
				}))
				outcome, err := llm.ExecuteJSON[map[string]any](t.Context(), llm.Executor{}, client, llmProviderFailureCall())
				if err != nil || len(starts) != 2 || outcome.Metrics.Attempts != 2 {
					t.Fatalf("attempts=%v outcome=%#v error=%v", starts, outcome, err)
				}
				if delay := starts[1].Sub(starts[0]); delay != test.want {
					t.Fatalf("retry delay=%v, want %v", delay, test.want)
				}
				if !bytes.Equal(bodies[0], bodies[1]) {
					t.Fatal("retry changed prepared request bytes")
				}
			})
		})
	}
}

func TestLLMProviderBodyWaitHintsOnlyApplyTo429AndPreserveErrorBytes(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := []byte(`{"error":{"message":"rate limit exceeded: retry after 9.636307001s, reset after 95.636307001s"}}`)
			client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write(body)
			}))
			completion, retryable, err := doChatMeasured(t.Context(), client.HTTPClient, client.Endpoint, client.APIKey, client.Auth, []byte(`{}`))
			if err == nil || !retryable || !bytes.Equal(completion.Content, body) || completion.ResponseBytes != len(body) {
				t.Fatalf("HTTP %d: completion=%#v retryable=%v err=%v", status, completion, retryable, err)
			}
			want := time.Duration(0)
			if status == http.StatusTooManyRequests {
				want = 95*time.Second + 636307001*time.Nanosecond
			}
			if completion.retryAfter != want {
				t.Fatalf("HTTP %d wait=%v, want %v", status, completion.retryAfter, want)
			}
		})
	}
}

func TestLLMProviderRateLimitWaitCanBeCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		calls := 0
		client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
			go func() { time.Sleep(10 * time.Second); cancel() }()
		}))
		started := time.Now()
		_, err := llm.ExecuteJSON[map[string]any](ctx, llm.Executor{}, client, llmProviderFailureCall())
		if !errors.Is(err, context.Canceled) || calls != 1 || time.Since(started) != 10*time.Second {
			t.Fatalf("cancel wait: calls=%d elapsed=%v err=%v", calls, time.Since(started), err)
		}
	})
}

func TestLLMProviderConcurrentRateLimitsShareLongestWaitAndRetrySerially(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		initial := make(chan struct{}, 3)
		releaseInitial := make(chan struct{})
		headers := map[string]string{"one": "30", "two": "90", "three": "120"}
		messages := map[string]string{
			"one":   "rate limit exceeded: retry after 9.636307001s, reset after 45.636307001s",
			"two":   `{"error":{"message":"rate limit exceeded: retry after 10s, reset after 100s"}}`,
			"three": "rate limit exceeded: retry after 20s, reset after 120.25s",
		}
		var mu sync.Mutex
		attempts := map[string]int{}
		var retryStarts []time.Time
		activeRetries, maxActiveRetries := 0, 0
		client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request chatRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			user := request.Messages[1].Content
			mu.Lock()
			attempts[user]++
			attempt := attempts[user]
			mu.Unlock()
			if attempt == 1 {
				initial <- struct{}{}
				<-releaseInitial
				w.Header().Set("Retry-After", headers[user])
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, messages[user])
				return
			}
			mu.Lock()
			retryStarts = append(retryStarts, time.Now())
			activeRetries++
			maxActiveRetries = max(maxActiveRetries, activeRetries)
			mu.Unlock()
			time.Sleep(time.Second)
			_, _ = w.Write(llmProviderResponse("stop", `{"ok":true}`, nil))
			mu.Lock()
			activeRetries--
			mu.Unlock()
		}))
		var calls []llm.Call[map[string]any]
		for _, user := range []string{"one", "two", "three"} {
			call := llmProviderFailureCall()
			call.Prompt.User = user
			calls = append(calls, call)
		}
		done := make(chan error, 1)
		go func() {
			outcomes, err := llm.ExecuteJSONBatch(t.Context(), llm.Executor{BatchConcurrency: 3}, client, calls)
			if err == nil && len(outcomes) != 3 {
				t.Errorf("outcomes=%d, want 3", len(outcomes))
			}
			done <- err
		}()
		for range 3 {
			<-initial
		}
		limitedAt := time.Now()
		close(releaseInitial)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(retryStarts) != 3 || maxActiveRetries != 1 || activeRetries != 0 {
			t.Fatalf("starts=%v concurrent retries=%d active=%d", retryStarts, maxActiveRetries, activeRetries)
		}
		for user, count := range attempts {
			if count != 2 {
				t.Errorf("%s attempts=%d, want 2", user, count)
			}
		}
		for _, start := range retryStarts {
			if start.Sub(limitedAt) < 2*time.Minute+250*time.Millisecond {
				t.Errorf("retry started after %v, before longest server wait", start.Sub(limitedAt))
			}
		}
	})
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
