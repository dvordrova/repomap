package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

func TestComponentSynthesisPromptJSONPreservesExactChatContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 6000}
	prompt := componentmap.SynthesisPrompt{
		Version: componentmap.SynthesisPromptVersion,
		System:  "exact system instruction\nwith a second line",
		User:    "exact bounded request\n{\"candidate\":\"opaque-1\"}",
	}

	got, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatalf("ComponentSynthesisPromptJSON() error = %v", err)
	}
	want, err := json.Marshal(chatRequest{
		Model: client.Model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.User},
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
		Version: componentmap.SynthesisPromptVersion,
		System:  "system json contract",
		User:    "bounded json request",
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
				Version: "component-landscape-future",
				System:  "system",
				User:    "user",
			},
		},
		{
			name: "blank system",
			prompt: componentmap.SynthesisPrompt{
				Version: componentmap.SynthesisPromptVersion,
				System:  " \n\t",
				User:    "user",
			},
		},
		{
			name: "blank user",
			prompt: componentmap.SynthesisPrompt{
				Version: componentmap.SynthesisPromptVersion,
				System:  "system",
				User:    " \n\t",
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
		Version: componentmap.SynthesisPromptVersion,
		System:  "system json contract",
		User:    "user bounded json request",
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
		Version: componentmap.SynthesisPromptVersion,
		System:  "system json contract",
		User:    "user bounded json request",
	}

	if _, err := client.SynthesizeComponentLandscape(context.Background(), prompt); err == nil {
		t.Fatal("SynthesizeComponentLandscape() error = nil")
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}
