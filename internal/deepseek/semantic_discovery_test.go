package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestSemanticDiscoveryPromptJSONPreservesExactChatContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 6000}
	prompt := semanticdiscovery.Prompt{
		Version:         semanticdiscovery.OpportunityPromptVersion,
		System:          "exact semantic discovery JSON boundary",
		User:            "exact bounded opportunity JSON bundle",
		ThinkingProfile: semanticdiscovery.ThinkingMax,
	}
	got, err := client.SemanticDiscoveryPromptJSON(prompt)
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

func TestSemanticDiscoveryProgressLabelUsesBoundedLocalStageNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt semanticdiscovery.Prompt
		want   string
	}{
		{
			name: "builder label",
			prompt: semanticdiscovery.Prompt{
				Version: semanticdiscovery.LeafPromptVersion, ProgressLabel: "semantic leaf mechanism 12ab34cd",
			},
			want: "semantic leaf mechanism 12ab34cd",
		},
		{
			name: "missing label fallback",
			prompt: semanticdiscovery.Prompt{
				Version: semanticdiscovery.FanInPromptVersion,
			},
			want: "semantic fan-in synthesis",
		},
		{
			name: "newline label fallback",
			prompt: semanticdiscovery.Prompt{
				Version: semanticdiscovery.OpportunityPromptVersion, ProgressLabel: "bad\nlabel",
			},
			want: "semantic opportunity scan",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := semanticDiscoveryProgressLabel(test.prompt); got != test.want {
				t.Fatalf("semanticDiscoveryProgressLabel() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSemanticDiscoveryPromptJSONUsesPurposeSpecificDeepSeekThinking(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint:  "https://api.deepseek.com/chat/completions",
		Model:     "deepseek-v4-flash",
		MaxTokens: 6000,
	}
	tests := []struct {
		name       string
		version    string
		profile    semanticdiscovery.ThinkingProfile
		wantTokens int
	}{
		{
			name:       "opportunity scan uses max",
			version:    semanticdiscovery.OpportunityPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "bounded leaf uses high",
			version:    semanticdiscovery.LeafPromptVersion,
			profile:    semanticdiscovery.ThinkingHigh,
			wantTokens: 6000,
		},
		{
			name:       "fan in uses max and larger envelope",
			version:    semanticdiscovery.FanInPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "monolithic comparison uses max",
			version:    semanticdiscovery.MonolithicPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "golden mechanism uses max",
			version:    semanticdiscovery.GoldenMechanismPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "legacy golden v3 remains accepted",
			version:    semanticdiscovery.GoldenMechanismPromptVersionV3,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "repository onboarding editor uses max",
			version:    semanticdiscovery.OnboardingEditorPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "repository brief and shape uses max",
			version:    semanticdiscovery.StudyBriefPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryGlobalMinMaxTokens,
		},
		{
			name:       "repository study candidates uses max",
			version:    semanticdiscovery.StudyCandidatesPromptVersion,
			profile:    semanticdiscovery.ThinkingMax,
			wantTokens: semanticDiscoveryStudyCandidatesMinMaxTokens,
		},
		{
			name:       "bounded reading pack review uses high",
			version:    semanticdiscovery.ReadingPackReviewPromptVersion,
			profile:    semanticdiscovery.ThinkingHigh,
			wantTokens: 6000,
		},
		{
			name:       "bounded operating guide uses high",
			version:    pavedpath.PromptVersion,
			profile:    semanticdiscovery.ThinkingHigh,
			wantTokens: semanticDiscoveryPavedPathMinMaxTokens,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			raw, err := client.SemanticDiscoveryPromptJSON(semanticdiscovery.Prompt{
				Version:         test.version,
				System:          "return valid JSON",
				User:            "bounded JSON task",
				ThinkingProfile: test.profile,
			})
			if err != nil {
				t.Fatal(err)
			}
			var request chatRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
				t.Fatalf("response_format = %#v", request.ResponseFormat)
			}
			if request.Thinking == nil || request.Thinking.Type != "enabled" {
				t.Fatalf("thinking = %#v, want enabled", request.Thinking)
			}
			if request.Temperature != nil {
				t.Fatalf("temperature = %v, want omitted", *request.Temperature)
			}
			if request.ReasoningEffort != string(test.profile) {
				t.Fatalf("reasoning_effort = %q, want %q", request.ReasoningEffort, test.profile)
			}
			if request.MaxTokens != test.wantTokens {
				t.Fatalf("max_tokens = %d, want %d", request.MaxTokens, test.wantTokens)
			}
		})
	}
}

func TestSemanticDiscoveryPromptJSONPreservesLargerConfiguredFanInLimit(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint:  "https://api.deepseek.com/chat/completions",
		Model:     "deepseek-v4-flash",
		MaxTokens: 24_000,
	}
	raw, err := client.SemanticDiscoveryPromptJSON(semanticdiscovery.Prompt{
		Version:         semanticdiscovery.FanInPromptVersion,
		System:          "return valid JSON",
		User:            "bounded JSON task",
		ThinkingProfile: semanticdiscovery.ThinkingMax,
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

func TestSemanticDiscoveryPromptJSONPreservesLargerConfiguredStudyCandidatesLimit(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint:  "https://api.deepseek.com/chat/completions",
		Model:     "deepseek-v4-flash",
		MaxTokens: 40_000,
	}
	raw, err := client.SemanticDiscoveryPromptJSON(semanticdiscovery.Prompt{
		Version:         semanticdiscovery.StudyCandidatesPromptVersion,
		System:          "return valid JSON",
		User:            "bounded JSON task",
		ThinkingProfile: semanticdiscovery.ThinkingMax,
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

func TestSemanticDiscoveryPromptJSONRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "company-model", MaxTokens: 6000}
	tests := []struct {
		name   string
		prompt semanticdiscovery.Prompt
	}{
		{
			name: "unsupported version",
			prompt: semanticdiscovery.Prompt{
				Version:         "semantic-future",
				System:          "system",
				User:            "user",
				ThinkingProfile: semanticdiscovery.ThinkingMax,
			},
		},
		{
			name: "wrong opportunity profile",
			prompt: semanticdiscovery.Prompt{
				Version:         semanticdiscovery.OpportunityPromptVersion,
				System:          "system",
				User:            "user",
				ThinkingProfile: semanticdiscovery.ThinkingHigh,
			},
		},
		{
			name: "wrong leaf profile",
			prompt: semanticdiscovery.Prompt{
				Version:         semanticdiscovery.LeafPromptVersion,
				System:          "system",
				User:            "user",
				ThinkingProfile: semanticdiscovery.ThinkingMax,
			},
		},
		{
			name: "wrong monolithic profile",
			prompt: semanticdiscovery.Prompt{
				Version:         semanticdiscovery.MonolithicPromptVersion,
				System:          "system",
				User:            "user",
				ThinkingProfile: semanticdiscovery.ThinkingHigh,
			},
		},
		{
			name: "empty system",
			prompt: semanticdiscovery.Prompt{
				Version:         semanticdiscovery.LeafPromptVersion,
				System:          " ",
				User:            "user",
				ThinkingProfile: semanticdiscovery.ThinkingHigh,
			},
		},
		{
			name: "empty user",
			prompt: semanticdiscovery.Prompt{
				Version:         semanticdiscovery.FanInPromptVersion,
				System:          "system",
				User:            " ",
				ThinkingProfile: semanticdiscovery.ThinkingMax,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := client.SemanticDiscoveryPromptJSON(test.prompt); err == nil {
				t.Fatalf("SemanticDiscoveryPromptJSON(%#v) error = nil", test.prompt)
			}
		})
	}
}

func TestDiscoverSemanticsMeasuredReturnsProviderCacheUsage(t *testing.T) {
	t.Parallel()

	var request chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"finish_reason":"stop","message":{"content":"{\"candidates\":[]}"}}],
			"usage":{
				"prompt_tokens":120,
				"completion_tokens":18,
				"prompt_cache_hit_tokens":96,
				"prompt_cache_miss_tokens":24
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Endpoint:   server.URL,
		Model:      "company-model",
		MaxTokens:  6000,
		Auth:       authNone,
	}
	result, err := client.DiscoverSemanticsMeasured(context.Background(), semanticdiscovery.Prompt{
		Version:         semanticdiscovery.OpportunityPromptVersion,
		System:          "return valid JSON",
		User:            "bounded opportunity JSON task",
		ThinkingProfile: semanticdiscovery.ThinkingMax,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Content) != `{"candidates":[]}` || result.Attempts != 1 ||
		result.InputTokens != 120 || result.OutputTokens != 18 ||
		result.PromptCacheHitTokens != 96 || result.PromptCacheMissTokens != 24 {
		t.Fatalf("DiscoverSemanticsMeasured() = %#v", result)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", request.ResponseFormat)
	}
}
