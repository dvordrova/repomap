package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/guidedtour"
)

func TestGuidedTourPromptJSONPreservesExactChatContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 2000}
	prompt := guidedtour.Prompt{
		Version: guidedtour.PromptVersion,
		System:  "exact editorial boundary",
		User:    "exact bounded guided-tour bundle",
	}

	got, err := client.GuidedTourPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
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

func TestGuidedTourPromptJSONAcceptsSupportedPromptVersions(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 2000}
	tests := []struct {
		name    string
		version string
	}{
		{name: "monolithic editor", version: guidedtour.PromptVersion},
		{name: "fan-out leaf", version: guidedtour.LeafPromptVersion},
		{name: "fan-in synthesis", version: guidedtour.FanInPromptVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.GuidedTourPromptJSON(guidedtour.Prompt{
				Version: tt.version,
				System:  "exact editorial boundary",
				User:    "exact bounded guided-tour bundle",
			})
			if err != nil {
				t.Fatalf("GuidedTourPromptJSON() error = %v", err)
			}
		})
	}
}

func TestGuidedTourPromptJSONUsesPurposeSpecificDeepSeekThinkingEffort(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint: "https://api.deepseek.com/chat/completions",
		Model:    "deepseek-v4-flash", MaxTokens: 6000,
	}
	tests := []struct {
		name       string
		version    string
		wantEffort string
		wantTokens int
	}{
		{name: "monolithic global planning", version: guidedtour.PromptVersion, wantEffort: "max", wantTokens: 6000},
		{name: "bounded semantic leaf", version: guidedtour.LeafPromptVersion, wantEffort: "high", wantTokens: 6000},
		{name: "fan-in global planning", version: guidedtour.FanInPromptVersion, wantEffort: "max", wantTokens: guidedTourFanInMinMaxTokens},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw, err := client.GuidedTourPromptJSON(guidedtour.Prompt{
				Version: tt.version, System: "return valid JSON", User: "bounded JSON task",
			})
			if err != nil {
				t.Fatal(err)
			}
			var request chatRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			if request.Thinking == nil || request.Thinking.Type != "enabled" {
				t.Fatalf("thinking = %#v, want enabled", request.Thinking)
			}
			if request.Temperature != nil {
				t.Fatalf("temperature = %v, want omitted in thinking mode", *request.Temperature)
			}
			if request.ReasoningEffort != tt.wantEffort {
				t.Fatalf("reasoning_effort = %q, want %q", request.ReasoningEffort, tt.wantEffort)
			}
			if request.MaxTokens != tt.wantTokens {
				t.Fatalf("max_tokens = %d, want %d", request.MaxTokens, tt.wantTokens)
			}
		})
	}
}

func TestGuidedTourPromptJSONPreservesLargerConfiguredFanInLimit(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint: "https://api.deepseek.com/chat/completions",
		Model:    "deepseek-v4-flash", MaxTokens: 16_000,
	}
	raw, err := client.GuidedTourPromptJSON(guidedtour.Prompt{
		Version: guidedtour.FanInPromptVersion,
		System:  "return valid JSON",
		User:    "bounded JSON task",
	})
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.MaxTokens != client.MaxTokens {
		t.Fatalf("max_tokens = %d, want configured %d", request.MaxTokens, client.MaxTokens)
	}
}

func TestGuidedTourPromptJSONRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 2000}
	tests := []struct {
		name   string
		prompt guidedtour.Prompt
	}{
		{name: "unsupported version", prompt: guidedtour.Prompt{Version: "future", System: "system", User: "user"}},
		{name: "empty system", prompt: guidedtour.Prompt{Version: guidedtour.PromptVersion, System: " ", User: "user"}},
		{name: "empty user", prompt: guidedtour.Prompt{Version: guidedtour.PromptVersion, System: "system", User: " "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.GuidedTourPromptJSON(tt.prompt); err == nil {
				t.Fatalf("GuidedTourPromptJSON(%#v) error = nil", tt.prompt)
			}
		})
	}
}

func TestEditGuidedTourMeasuredPreservesUsageWhenThinkingExhaustsContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{
				"finish_reason":"length",
				"message":{"content":null,"reasoning_content":"private provider reasoning"}
			}],
			"usage":{
				"prompt_tokens":120,
				"completion_tokens":6000,
				"prompt_cache_hit_tokens":96,
				"prompt_cache_miss_tokens":24,
				"completion_tokens_details":{"reasoning_tokens":6000}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Model:      "deepseek-v4-flash",
		MaxTokens:  6000,
		Endpoint:   server.URL,
		Auth:       authNone,
	}
	result, err := client.EditGuidedTourMeasured(context.Background(), guidedtour.Prompt{
		Version: guidedtour.PromptVersion,
		System:  "return valid JSON",
		User:    "bounded guided-tour task",
	})
	if err == nil || !strings.Contains(err.Error(), "content is empty") {
		t.Fatalf("EditGuidedTourMeasured() error = %v", err)
	}
	if len(result.Content) != 0 || result.Attempts != 1 ||
		result.InputTokens != 120 || result.OutputTokens != 6000 ||
		result.PromptCacheHitTokens != 96 || result.PromptCacheMissTokens != 24 {
		t.Fatalf("EditGuidedTourMeasured() failed-call metrics = %#v", result)
	}
}
