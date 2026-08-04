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
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestComponentSynthesisPromptJSONPreservesExactChatContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 6000}
	prompt := componentmap.SynthesisPrompt{
		Version:        componentmap.SynthesisPromptVersion,
		OutputLanguage: "en",
		System:         "exact system instruction\nwith a second line",
		User:           "exact bounded request\n{\"candidate\":\"opaque-1\"}",
	}

	got, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatalf("ComponentSynthesisPromptJSON() error = %v", err)
	}
	want, err := json.Marshal(chatRequest{
		Model: client.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: prompt.System,
			},
			{
				Role:    "user",
				Content: prompt.User,
			},
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
}

func TestComponentSynthesisPromptJSONDisablesThinkingForDeepSeek(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint:  "https://api.deepseek.com/chat/completions",
		Model:     "deepseek-v4-flash",
		MaxTokens: 6000,
	}
	prompt := componentmap.SynthesisPrompt{
		Version:        componentmap.SynthesisPromptVersion,
		OutputLanguage: "en",
		System:         "system json contract",
		User:           "bounded json request",
	}

	encoded, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatalf("ComponentSynthesisPromptJSON() error = %v", err)
	}
	var request chatRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if request.Thinking == nil || request.Thinking.Type != "disabled" {
		t.Fatalf("thinking = %#v, want disabled", request.Thinking)
	}
}

func TestComponentSynthesisPromptJSONUsesOneStageOwnedLanguageContract(t *testing.T) {
	t.Parallel()
	client := &Client{Model: "company-model", MaxTokens: 64_000}
	bodies := make(map[string][]byte)

	for _, test := range []struct {
		language  string
		directive string
		forbidden string
	}{
		{language: "en", directive: "name and description prose in English", forbidden: "prose in Russian"},
		{language: "ru", directive: "name and description prose in Russian", forbidden: "prose in English"},
	} {
		t.Run(test.language, func(t *testing.T) {
			prompt := componentmap.SynthesisPrompt{
				Version: componentmap.SynthesisPromptVersion, OutputLanguage: test.language,
				System: "Return JSON. Write only subsystem and component " + test.directive + ".",
				User:   `Bounded candidate request: {"candidates":[]}`,
			}
			body, err := client.ComponentSynthesisPromptJSON(prompt)
			if err != nil {
				t.Fatal(err)
			}
			var request chatRequest
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			joined := request.Messages[0].Content + "\n" + request.Messages[1].Content
			if strings.Count(joined, test.directive) != 1 || strings.Contains(joined, test.forbidden) {
				t.Fatalf("component synthesis language contract = %q", joined)
			}
			if strings.Contains(joined, canonicalEnglishSystemContract) ||
				strings.Contains(joined, canonicalEnglishUserContract) ||
				strings.Contains(joined, "CANONICAL OUTPUT LANGUAGE CONTRACT") {
				t.Fatalf("component synthesis retained shared language wrapper: %q", joined)
			}
			bodies[test.language] = body
		})
	}
	if bytes.Equal(bodies["en"], bodies["ru"]) {
		t.Fatal("component synthesis provider body did not bind the stage-owned output language")
	}
}

func TestComponentSynthesisPromptRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 6000}
	tests := []struct {
		name   string
		prompt componentmap.SynthesisPrompt
	}{
		{
			name: "unsupported version",
			prompt: componentmap.SynthesisPrompt{
				Version:        "component-landscape-future",
				OutputLanguage: "en",
				System:         "system",
				User:           "user",
			},
		},
		{
			name: "unsupported output language",
			prompt: componentmap.SynthesisPrompt{
				Version:        componentmap.SynthesisPromptVersion,
				OutputLanguage: "future",
				System:         "system",
				User:           "user",
			},
		},
		{
			name: "blank system",
			prompt: componentmap.SynthesisPrompt{
				Version:        componentmap.SynthesisPromptVersion,
				OutputLanguage: "en",
				System:         " \n\t",
				User:           "user",
			},
		},
		{
			name: "blank user",
			prompt: componentmap.SynthesisPrompt{
				Version:        componentmap.SynthesisPromptVersion,
				OutputLanguage: "en",
				System:         "system",
				User:           " \n\t",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.ComponentSynthesisPromptJSON(test.prompt); err == nil {
				t.Fatal("ComponentSynthesisPromptJSON() error = nil")
			}
		})
	}
}

func TestSynthesizeComponentLandscapePreservesInvalidProviderContent(t *testing.T) {
	t.Parallel()

	const invalidProposal = "this is not json"
	var (
		calls       int
		requestBody []byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		requestBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": invalidProposal,
				},
			}},
		})
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Model:      "company-model",
		MaxTokens:  6000,
		Endpoint:   server.URL,
		Auth:       authNone,
	}
	prompt := componentmap.SynthesisPrompt{
		Version:        componentmap.SynthesisPromptVersion,
		OutputLanguage: "en",
		System:         "system json contract",
		User:           "user bounded json request",
	}
	wantRequest, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.SynthesizeComponentLandscape(context.Background(), prompt)
	if err != nil {
		t.Fatalf("SynthesizeComponentLandscape() error = %v", err)
	}
	if string(got) != invalidProposal {
		t.Fatalf("provider content = %q, want %q", got, invalidProposal)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	if !bytes.Equal(requestBody, wantRequest) {
		t.Fatalf("sent request differs from inspected request\nsent: %s\nwant: %s", requestBody, wantRequest)
	}

	var sent chatRequest
	if err := json.Unmarshal(requestBody, &sent); err != nil {
		t.Fatalf("decode sent request: %v", err)
	}
	if sent.ResponseFormat == nil || sent.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", sent.ResponseFormat)
	}
	if len(sent.Messages) != 2 ||
		sent.Messages[0].Content != prompt.System ||
		sent.Messages[1].Content != prompt.User {
		t.Fatalf("sent messages = %#v", sent.Messages)
	}
}

func TestSynthesizeComponentLandscapeMakesOneProviderAttempt(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Model:      "company-model",
		MaxTokens:  6000,
		Endpoint:   server.URL,
		Auth:       authNone,
	}
	prompt := componentmap.SynthesisPrompt{
		Version:        componentmap.SynthesisPromptVersion,
		OutputLanguage: "en",
		System:         "system json contract",
		User:           "user bounded json request",
	}

	if _, err := client.SynthesizeComponentLandscape(context.Background(), prompt); err == nil {
		t.Fatal("SynthesizeComponentLandscape() error = nil")
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

func TestSynthesizeComponentLandscapeBodyMeasuredSendsExactImmutableBody(t *testing.T) {
	t.Parallel()

	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}],"usage":{}}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "company-model", MaxTokens: 64_000,
	}
	prompt := componentmap.SynthesisPrompt{
		Version: componentmap.SynthesisPromptVersion, OutputLanguage: "en",
		System: "Return JSON. Write only subsystem and component name and description prose in English.",
		User:   `Bounded candidate request: {"candidates":[]}`,
	}
	exactBody, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SynthesizeComponentLandscapeBodyMeasured(t.Context(), exactBody)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(requestBody, exactBody) {
		t.Fatalf("sent body differs from exact inspected body\nsent: %s\nwant: %s", requestBody, exactBody)
	}
	if result.Attempts != 1 || result.RequestBytes != len(exactBody) ||
		string(result.Content) != "{}" || !result.UsageReported ||
		result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Fatalf("measured exact-body result = %#v", result)
	}

	if _, err := client.SynthesizeComponentLandscapeBodyMeasured(t.Context(), nil); err == nil {
		t.Fatal("SynthesizeComponentLandscapeBodyMeasured() accepted an empty exact body")
	}
}

func TestSynthesizeComponentLandscapeBodyMeasuredKeepsTerminalResourceSemantics(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}],"usage":{}}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), Endpoint: server.URL, Auth: authNone,
		Model: "company-model", MaxTokens: 64_000,
	}
	exactBody := []byte(`{"model":"company-model","messages":[]}`)
	result, err := client.SynthesizeComponentLandscapeBodyMeasured(t.Context(), exactBody)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 1 || result.Attempts != 1 ||
		!result.UsageReported || limitErr.Stage != "architecture_synthesis" ||
		limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		limitErr.Limit != client.MaxTokens || !limitErr.ObservedKnown ||
		limitErr.Observed != 0 || limitErr.FinishReason != "length" {
		t.Fatalf("terminal exact-body result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}
