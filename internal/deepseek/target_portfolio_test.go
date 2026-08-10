package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

func targetPortfolioPromptFixture() targetportfolio.Prompt {
	return targetportfolio.Prompt{
		Version: targetportfolio.PromptVersion,
		System:  "Select only supplied refs. Return exactly one JSON object.",
		User:    `Response JSON schema: {"version":3,"request_ref":"tpq-fixture","default_ref":"t1","target_refs":["t1"]}. Exact catalog JSON: {"targets":[{"ref":"t1","display_path":"cmd/app","kind":"executable","symbols":[{"kind":"func","names":["main","runProduct"]}]}]}`,
	}
}

func TestTargetPortfolioPromptJSONPreservesExactGenericContract(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions",
		Model:    "fixture-model", MaxTokens: 64_000,
	}
	prompt := targetPortfolioPromptFixture()

	got, err := client.TargetPortfolioPromptJSON(prompt)
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
		t.Fatalf("generic target-portfolio request gained an unowned extension: %s", got)
	}
}

func TestTargetPortfolioPromptJSONDisablesOfficialDeepSeekThinking(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint: "https://api.deepseek.com/chat/completions",
		Model:    "deepseek-v4-flash", MaxTokens: 64_000,
	}
	body, err := client.TargetPortfolioPromptJSON(targetPortfolioPromptFixture())
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

func TestTargetPortfolioPromptJSONRejectsInvalidPrompt(t *testing.T) {
	t.Parallel()
	valid := targetPortfolioPromptFixture()
	tests := []struct {
		name   string
		client *Client
		prompt targetportfolio.Prompt
	}{
		{name: "nil client", prompt: valid},
		{name: "unsupported version", client: &Client{}, prompt: targetportfolio.Prompt{Version: "future", System: valid.System, User: valid.User}},
		{name: "blank system", client: &Client{}, prompt: targetportfolio.Prompt{Version: valid.Version, System: " \n\t", User: valid.User}},
		{name: "blank user", client: &Client{}, prompt: targetportfolio.Prompt{Version: valid.Version, System: valid.System, User: " \n\t"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := test.client.TargetPortfolioPromptJSON(test.prompt); err == nil {
				t.Fatal("TargetPortfolioPromptJSON() error = nil")
			}
		})
	}
}

func TestTargetPortfolioPromptJSONCapsExactProviderEnvelope(t *testing.T) {
	t.Parallel()
	client := &Client{Model: "fixture-model", MaxTokens: 64_000}
	prompt := targetPortfolioPromptFixture()
	prompt.User = strings.Repeat("x", targetportfolio.MaxProviderRequestBytes)

	_, err := client.TargetPortfolioPromptJSON(prompt)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != targetPortfolioSelectionStage ||
		limitErr.Kind != modelresearch.ResourceLimitRequestBytes ||
		limitErr.Limit != targetportfolio.MaxProviderRequestBytes || limitErr.Observed <= limitErr.Limit ||
		!limitErr.ObservedKnown || limitErr.ConfiguredMaxTokens != client.MaxTokens {
		t.Fatalf("request limit error = %#v / %v", limitErr, err)
	}
}

func TestTargetPortfolioPromptJSONAllowsEnvelopeLargerThanSemanticWireLimit(t *testing.T) {
	t.Parallel()
	client := &Client{Model: "fixture-model", MaxTokens: 64_000}
	prompt := targetPortfolioPromptFixture()
	prompt.User = strings.Repeat("x", targetportfolio.MaxRequestBytes)
	body, err := client.TargetPortfolioPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= targetportfolio.MaxRequestBytes || len(body) > targetportfolio.MaxProviderRequestBytes {
		t.Fatalf("provider envelope bytes = %d, semantic=%d provider=%d", len(body), targetportfolio.MaxRequestBytes, targetportfolio.MaxProviderRequestBytes)
	}
}

func TestTargetPortfolioPromptJSONAllowsNearLimitEscapedSemanticPrompt(t *testing.T) {
	t.Parallel()
	client := &Client{Model: "fixture-model", MaxTokens: 64_000}
	prompt := targetPortfolioPromptFixture()
	names := make([]string, 30_000)
	for index := range names {
		names[index] = fmt.Sprintf("ExportedSymbol%05d", index)
	}
	request := targetportfolio.Request{
		Version: targetportfolio.RequestVersion, RequestRef: "tpq-fixture", RepoName: "fixture",
		Targets: []targetportfolio.Target{{
			Ref: "t1", DisplayPath: "pkg/api", Kind: targetportfolio.TargetLibrary,
			Symbols: []targetportfolio.SymbolGroup{{Kind: "func"}},
		}},
	}
	low, high := 1, len(names)
	var payload []byte
	for low <= high {
		middle := low + (high-low)/2
		request.Targets[0].Symbols[0].Names = names[:middle]
		candidate, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if len(candidate) <= targetportfolio.MaxRequestBytes {
			payload = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	if len(payload) < targetportfolio.MaxRequestBytes-1024 || !json.Valid(payload) {
		t.Fatalf("near-limit target request is invalid or too small (%d bytes)", len(payload))
	}
	prompt.User = "Response JSON schema is refs-only. Exact bounded target catalog JSON:\n" + string(payload)
	body, err := client.TargetPortfolioPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= len(prompt.User) || len(body) > targetportfolio.MaxProviderRequestBytes {
		t.Fatalf("escaped provider envelope bytes = %d, payload=%d cap=%d", len(body), len(payload), targetportfolio.MaxProviderRequestBytes)
	}
}

func TestTargetPortfolioMeasuredSendsExactBodyAndPreservesInvalidContent(t *testing.T) {
	t.Parallel()
	var sent []byte
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls++
		sent, _ = io.ReadAll(request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"not-json"}}],"usage":{"prompt_tokens":13,"completion_tokens":2,"completion_tokens_details":{"reasoning_tokens":1},"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":8}}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	prompt := targetPortfolioPromptFixture()
	exactBody, err := client.TargetPortfolioPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.TargetPortfolioMeasured(t.Context(), prompt)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !bytes.Equal(sent, exactBody) || result.Attempts != 1 ||
		result.RequestBytes != len(exactBody) || string(result.Content) != "not-json" ||
		result.FinishReason != "stop" || result.ChoiceCount != 1 ||
		!result.UsageReported || result.InputTokens != 13 || result.OutputTokens != 2 ||
		result.ReasoningTokens != 1 || result.PromptCacheHitTokens != 5 ||
		result.PromptCacheMissTokens != 8 {
		t.Fatalf("calls/sent/result = %d / %q / %#v", calls, sent, result)
	}
}

func TestTargetPortfolioBodyMeasuredRetriesOnlyExactTransportBytes(t *testing.T) {
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
	result, err := client.TargetPortfolioBodyMeasured(t.Context(), exactBody)
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

func TestTargetPortfolioBodyMeasuredRejectsOversizeBeforeNetwork(t *testing.T) {
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
	body := bytes.Repeat([]byte("x"), targetportfolio.MaxProviderRequestBytes+1)

	result, err := client.TargetPortfolioBodyMeasured(t.Context(), body)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 0 || result.Attempts != 0 ||
		limitErr.Stage != targetPortfolioSelectionStage ||
		limitErr.Kind != modelresearch.ResourceLimitRequestBytes ||
		limitErr.Limit != targetportfolio.MaxProviderRequestBytes || limitErr.Observed != len(body) ||
		!limitErr.ObservedKnown || limitErr.ConfiguredMaxTokens != client.MaxTokens {
		t.Fatalf("pre-call result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}

func TestTargetPortfolioBodyMeasuredCapsSemanticResponse(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("x", targetportfolio.MaxResponseBytes+1)
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

	result, callErr := client.TargetPortfolioBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(callErr, &limitErr) || calls != 1 || result.Attempts != 1 ||
		limitErr.Stage != targetPortfolioSelectionStage ||
		limitErr.Kind != modelresearch.ResourceLimitResponseBytes ||
		limitErr.Limit != targetportfolio.MaxResponseBytes || limitErr.Observed != len(content) ||
		!limitErr.ObservedKnown || !bytes.Equal(limitErr.ProviderContent(), []byte(content)) {
		t.Fatalf("terminal result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, callErr)
	}
}

func TestTargetPortfolioBodyMeasuredTreatsLengthAsTerminal(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}],"usage":{"prompt_tokens":20,"completion_tokens":64000}}`)
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone, MaxTokens: 64_000}

	result, err := client.TargetPortfolioBodyMeasured(t.Context(), []byte(`{"exact":true}`))
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 1 || result.Attempts != 1 ||
		limitErr.Stage != targetPortfolioSelectionStage ||
		limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		limitErr.Limit != client.MaxTokens || limitErr.FinishReason != "length" {
		t.Fatalf("terminal result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}

func TestTargetPortfolioBodyMeasuredRejectsMissingInputs(t *testing.T) {
	t.Parallel()
	if _, err := (*Client)(nil).TargetPortfolioBodyMeasured(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("nil client accepted")
	}
	if _, err := (&Client{}).TargetPortfolioBodyMeasured(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("nil HTTP client accepted")
	}
	if _, err := (&Client{HTTPClient: http.DefaultClient}).TargetPortfolioBodyMeasured(t.Context(), nil); err == nil {
		t.Fatal("empty body accepted")
	}
}
