package deepseek

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestBuildLocalizationRequestIsExactDeterministicAndUnwrapped(t *testing.T) {
	t.Parallel()

	client := &Client{
		Endpoint:  "https://gateway.example.test:8443/v1/chat/%63ompletions",
		Auth:      authBearer,
		Model:     "company-model",
		MaxTokens: 4096,
		APIKey:    "must-never-appear",
	}
	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "exact localization system",
		User:    "exact localization user",
	}

	first, err := client.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	second, err := client.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() second error = %v", err)
	}
	if first.Version != LocalizationRequestVersion ||
		first.Provider != localizationProvider ||
		first.Endpoint != client.Endpoint ||
		first.AuthMode != authBearer ||
		first.Model != client.Model ||
		first.MaxTokens != client.MaxTokens ||
		first.ResponseFormat != "json_object" ||
		first.Thinking != "" ||
		first.ReasoningEffort != "" {
		t.Fatalf("unexpected evidence = %#v", first)
	}
	if first.Temperature == nil || *first.Temperature != 0 {
		t.Fatalf("temperature = %v, want 0", first.Temperature)
	}
	if !bytes.Equal(first.Body, second.Body) {
		t.Fatalf("request is not deterministic\nfirst:  %s\nsecond: %s", first.Body, second.Body)
	}
	want, err := json.Marshal(chatRequest{
		Model: client.Model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.User},
		},
		Temperature:    float64Pointer(0),
		MaxTokens:      client.MaxTokens,
		ResponseFormat: &jsonFormat{Type: "json_object"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Body, want) {
		t.Fatalf("request JSON differs\ngot:  %s\nwant: %s", first.Body, want)
	}
	body := string(first.Body)
	if strings.Contains(body, "OUTPUT LANGUAGE CONTRACT") ||
		strings.Contains(body, client.APIKey) {
		t.Fatalf("request contains wrapper or API key: %s", body)
	}
}

func TestBuildLocalizationRequestCanonicalizesEndpointAndThinking(t *testing.T) {
	t.Parallel()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	tests := []struct {
		name         string
		endpoint     string
		wantEndpoint string
		wantThinking string
	}{
		{
			name:         "official deepseek default port",
			endpoint:     "HTTPS://API.DeepSeek.COM:443/chat/%63ompletions",
			wantEndpoint: "https://api.deepseek.com/chat/%63ompletions",
			wantThinking: "disabled",
		},
		{
			name:         "generic default port",
			endpoint:     "HTTP://Gateway.Example.TEST:80/v1/%63hat/completions",
			wantEndpoint: "http://gateway.example.test/v1/%63hat/completions",
		},
		{
			name:         "ipv6 non-default port",
			endpoint:     "https://[2001:DB8::1]:8443/v1/chat",
			wantEndpoint: "https://[2001:db8::1]:8443/v1/chat",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			evidence, err := (&Client{
				Endpoint: test.endpoint, Auth: authNone,
				Model: "fixture-model", MaxTokens: 100,
			}).BuildLocalizationRequest(prompt)
			if err != nil {
				t.Fatalf("BuildLocalizationRequest() error = %v", err)
			}
			if evidence.Endpoint != test.wantEndpoint ||
				evidence.Thinking != test.wantThinking {
				t.Fatalf(
					"endpoint, thinking = %q, %q; want %q, %q",
					evidence.Endpoint,
					evidence.Thinking,
					test.wantEndpoint,
					test.wantThinking,
				)
			}
			var request chatRequest
			if err := json.Unmarshal(evidence.Body, &request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if test.wantThinking == "" {
				if request.Thinking != nil {
					t.Fatalf("generic thinking = %#v, want omitted", request.Thinking)
				}
			} else if request.Thinking == nil ||
				request.Thinking.Type != test.wantThinking {
				t.Fatalf("official thinking = %#v, want disabled", request.Thinking)
			}
		})
	}
}

func TestBuildLocalizationRequestRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	validPrompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	validClient := func() *Client {
		return &Client{
			Endpoint: "https://example.test/v1/chat/completions",
			Auth:     authBearer, Model: "model", MaxTokens: 100,
		}
	}
	tests := []struct {
		name   string
		client *Client
		prompt localization.Prompt
	}{
		{name: "nil client", prompt: validPrompt},
		{
			name: "invalid prompt version", client: validClient(),
			prompt: localization.Prompt{
				Version: "future", System: "system", User: "user",
			},
		},
		{
			name: "credential in prompt", client: validClient(),
			prompt: localization.Prompt{
				Version: localization.PromptVersion,
				System:  "system",
				User:    `api_key := "company-secret-value"`,
			},
		},
		{
			name: "empty endpoint", client: func() *Client {
				client := validClient()
				client.Endpoint = ""
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "endpoint userinfo", client: func() *Client {
				client := validClient()
				client.Endpoint = "https://user@example.test/v1"
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "endpoint query", client: func() *Client {
				client := validClient()
				client.Endpoint = "https://example.test/v1?token=value"
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "endpoint fragment", client: func() *Client {
				client := validClient()
				client.Endpoint = "https://example.test/v1#fragment"
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "endpoint scheme", client: func() *Client {
				client := validClient()
				client.Endpoint = "file:///tmp/provider"
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "blank model", client: func() *Client {
				client := validClient()
				client.Model = " "
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "non-positive max tokens", client: func() *Client {
				client := validClient()
				client.MaxTokens = 0
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "invalid auth", client: func() *Client {
				client := validClient()
				client.Auth = "basic"
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "oversized endpoint", client: func() *Client {
				client := validClient()
				client.Endpoint = "https://" + strings.Repeat("x", maxLocalizationRequestScalarBytes)
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "oversized model", client: func() *Client {
				client := validClient()
				client.Model = strings.Repeat("x", maxLocalizationRequestScalarBytes+1)
				return client
			}(),
			prompt: validPrompt,
		},
		{
			name: "model control character", client: func() *Client {
				client := validClient()
				client.Model = "model\tvariant"
				return client
			}(),
			prompt: validPrompt,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := test.client.BuildLocalizationRequest(test.prompt); err == nil {
				t.Fatal("BuildLocalizationRequest() error = nil")
			}
		})
	}
}

func TestBuildLocalizationRequestDoesNotEchoMalformedEndpoint(t *testing.T) {
	t.Parallel()

	const secret = `company-secret-value-123456789`
	client := &Client{
		Endpoint:  `https://example.test/%zz-api_key="` + secret + `"`,
		Auth:      authBearer,
		Model:     "model",
		MaxTokens: 100,
	}
	_, err := client.BuildLocalizationRequest(localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	})
	if err == nil {
		t.Fatal("BuildLocalizationRequest() error = nil")
	}
	if strings.Contains(err.Error(), secret) ||
		strings.Contains(err.Error(), "api_key") {
		t.Fatalf("malformed endpoint leaked through error: %v", err)
	}
}

func TestLocalizationRequestEvidenceRejectsContradictoryBody(t *testing.T) {
	t.Parallel()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	client := &Client{
		Endpoint:  "https://example.test/v1/chat/completions",
		Auth:      authBearer,
		Model:     "model",
		MaxTokens: 100,
	}
	valid, err := client.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*LocalizationRequestEvidence)
	}{
		{
			name: "version",
			mutate: func(evidence *LocalizationRequestEvidence) {
				evidence.Version = "future"
			},
		},
		{
			name: "provider",
			mutate: func(evidence *LocalizationRequestEvidence) {
				evidence.Provider = "other"
			},
		},
		{
			name: "endpoint",
			mutate: func(evidence *LocalizationRequestEvidence) {
				evidence.Endpoint = "HTTPS://example.test:443/v1/chat/completions"
			},
		},
		{
			name: "model identity",
			mutate: func(evidence *LocalizationRequestEvidence) {
				evidence.Model = "other"
			},
		},
		{
			name: "body message",
			mutate: func(evidence *LocalizationRequestEvidence) {
				var request chatRequest
				if err := json.Unmarshal(evidence.Body, &request); err != nil {
					t.Fatal(err)
				}
				request.Messages[1].Content = "different"
				evidence.Body, err = json.Marshal(request)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := valid
			evidence.Body = append([]byte(nil), valid.Body...)
			test.mutate(&evidence)
			if err := evidence.Validate(prompt); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestBuildLocalizationRequestAcceptsLargestValidPromptBody(t *testing.T) {
	t.Parallel()

	prompt, promptJSON := largestValidLocalizationPrompt(t)
	evidence, err := (&Client{
		Endpoint:  "https://example.test/v1/chat/completions",
		Auth:      authNone,
		Model:     "fixture-model",
		MaxTokens: 4096,
	}).BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Body) <= len(promptJSON) ||
		len(evidence.Body) > MaxLocalizationRequestBodyBytes {
		t.Fatalf(
			"request bytes = %d, prompt bytes = %d, shared max = %d",
			len(evidence.Body),
			len(promptJSON),
			MaxLocalizationRequestBodyBytes,
		)
	}
}

func largestValidLocalizationPrompt(t *testing.T) (localization.Prompt, []byte) {
	t.Helper()

	low, high := 1, 1<<20
	var best localization.Prompt
	var bestJSON []byte
	for low <= high {
		middle := low + (high-low)/2
		candidate := localization.Prompt{
			Version: localization.PromptVersion,
			System:  "system",
			User:    strings.Repeat("x", middle),
		}
		encoded, err := localization.MarshalPrompt(candidate)
		if err != nil {
			high = middle - 1
			continue
		}
		best = candidate
		bestJSON = encoded
		low = middle + 1
	}
	if len(bestJSON) == 0 {
		t.Fatal("no valid localization prompt found")
	}
	return best, bestJSON
}
