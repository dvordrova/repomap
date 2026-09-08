package deepseek

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestNewFromEnvUsesGenericConfigurationWithoutMixingAliases(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envEndpoint, "https://internal.example.com/v1/chat/completions")
	t.Setenv(envModel, "internal-code-model")
	t.Setenv(envAPIKey, "repomap-key")
	t.Setenv(envMaxTokens, "1234")
	t.Setenv(envTimeout, "7.5s")
	t.Setenv(envAuth, authBearer)
	t.Setenv(envChatTemplateKwargs, `{"enable_thinking":false}`)
	t.Setenv(legacyEnvEndpoint, "https://legacy.example.com/chat/completions")
	t.Setenv(legacyEnvModel, "legacy-model")
	t.Setenv(legacyEnvAPIKey, "legacy-key")
	t.Setenv(legacyEnvTimeout, "45s")
	t.Setenv(legacyEnvAuth, authNone)
	t.Setenv(legacyEnvChatTemplateKwargs, `{"enable_thinking":true}`)

	client, err := NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if client.Endpoint != "https://internal.example.com/v1/chat/completions" ||
		client.Model != "internal-code-model" || client.APIKey != "repomap-key" ||
		client.MaxTokens != 1234 || client.HTTPClient.Timeout != 7500*time.Millisecond ||
		client.Auth != authBearer || string(client.ChatTemplateKwargs["enable_thinking"]) != "false" {
		t.Fatalf("generic configuration = %#v, timeout = %s", client, client.HTTPClient.Timeout)
	}
}

func TestNewFromEnvSupportsDeepSeekAliasesAndDefaults(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(legacyEnvAPIKey, "legacy-key")
	t.Setenv(legacyEnvChatTemplateKwargs, `{"enable_thinking":false}`)

	client, err := NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if client.Endpoint != defaultEndpoint || client.Model != defaultModel ||
		client.APIKey != "legacy-key" || client.MaxTokens != defaultMaxTokens ||
		client.HTTPClient.Timeout != defaultTimeout || client.Auth != authBearer ||
		string(client.ChatTemplateKwargs["enable_thinking"]) != "false" {
		t.Fatalf("default configuration = %#v, timeout = %s", client, client.HTTPClient.Timeout)
	}
}

func TestLegacyEndpointSelectsTemplateControlByActualHost(t *testing.T) {
	for _, test := range []struct {
		endpoint string
		official bool
	}{
		{endpoint: "https://api.deepseek.com/chat/completions", official: true},
		{endpoint: "https://API.DEEPSEEK.COM/v1/chat/completions", official: true},
		{endpoint: "http://127.0.0.1:8000/v1/chat/completions"},
		{endpoint: "https://api.deepseek.com.proxy.example/v1/chat/completions"},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			clearLLMConfigEnv(t)
			t.Setenv(legacyEnvEndpoint, test.endpoint)
			t.Setenv(legacyEnvAPIKey, "fixture-key")
			client, err := NewFromEnv()
			if err != nil {
				t.Fatal(err)
			}
			for _, reasoning := range []bool{false, true} {
				prepared, err := client.Prepare(llm.Prompt{System: "system", User: "user", Reasoning: reasoning}, llmProviderTestLimits(400))
				if err != nil {
					t.Fatal(err)
				}
				var request chatRequest
				if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
					t.Fatal(err)
				}
				if test.official {
					want := "disabled"
					if reasoning {
						want = "enabled"
					}
					if request.ChatTemplateKwargs != nil || request.Thinking == nil || request.Thinking.Type != want {
						t.Fatalf("official controls for reasoning=%v: %#v", reasoning, request)
					}
				} else {
					var enabled bool
					if err := json.Unmarshal(request.ChatTemplateKwargs["enable_thinking"], &enabled); err != nil || enabled != reasoning || request.Thinking != nil {
						t.Fatalf("compatible controls for reasoning=%v: %#v (%v)", reasoning, request, err)
					}
				}
			}
		})
	}
}

func TestNewFromEnvNoAuthRequiresEndpointAndDiscardsKey(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envAuth, authNone)
	if _, err := NewFromEnv(); err == nil || !strings.Contains(err.Error(), envEndpoint) {
		t.Fatalf("missing no-auth endpoint error = %v", err)
	}

	clearLLMConfigEnv(t)
	t.Setenv(envEndpoint, "http://127.0.0.1:11434/v1/chat/completions")
	t.Setenv(envAuth, authNone)
	t.Setenv(envAPIKey, "must-not-be-retained")
	client, err := NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if client.Auth != authNone || client.APIKey != "" {
		t.Fatalf("no-auth configuration retained credentials: auth=%q key=%q", client.Auth, client.APIKey)
	}
}

func TestNewFromEnvRequiresCredentialFromSelectedConfigurationFamily(t *testing.T) {
	clearLLMConfigEnv(t)
	if _, err := NewFromEnv(); err == nil || !strings.Contains(err.Error(), envAPIKey) {
		t.Fatalf("default bearer error = %v", err)
	}

	clearLLMConfigEnv(t)
	t.Setenv(envEndpoint, "https://internal.example.com/v1/chat/completions")
	t.Setenv(legacyEnvAPIKey, "stale-legacy-key")
	if _, err := NewFromEnv(); err == nil || !strings.Contains(err.Error(), envAPIKey) {
		t.Fatalf("mixed-family credential error = %v", err)
	}
}

func TestNewFromEnvRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		environ map[string]string
		want    string
	}{
		{name: "max tokens not integer", environ: map[string]string{envMaxTokens: "many"}, want: envMaxTokens},
		{name: "max tokens zero", environ: map[string]string{envMaxTokens: "0"}, want: "positive"},
		{name: "timeout malformed", environ: map[string]string{envTimeout: "later"}, want: envTimeout},
		{name: "timeout negative", environ: map[string]string{envTimeout: "-1s"}, want: "positive"},
		{name: "auth unsupported", environ: map[string]string{envAuth: "basic"}, want: envAuth},
		{name: "template kwargs invalid JSON", environ: map[string]string{envChatTemplateKwargs: `{`}, want: envChatTemplateKwargs},
		{name: "template kwargs array", environ: map[string]string{envChatTemplateKwargs: `[]`}, want: "JSON object"},
		{name: "template kwargs null", environ: map[string]string{envChatTemplateKwargs: `null`}, want: "JSON object"},
		{name: "legacy template kwargs invalid", environ: map[string]string{legacyEnvChatTemplateKwargs: `false`}, want: legacyEnvChatTemplateKwargs},
		{name: "endpoint relative", environ: map[string]string{envEndpoint: "/v1/chat/completions", envAuth: authNone}, want: "scheme"},
		{name: "endpoint missing host", environ: map[string]string{envEndpoint: "https:///chat/completions", envAuth: authNone}, want: "host"},
		{name: "endpoint with userinfo", environ: map[string]string{envEndpoint: "https://user:password@models.example.com/chat", envAuth: authNone}, want: "userinfo"},
		{name: "endpoint with query", environ: map[string]string{envEndpoint: "https://models.example.com/chat?api_key=secret", envAuth: authNone}, want: "query"},
		{name: "endpoint with fragment", environ: map[string]string{envEndpoint: "https://models.example.com/chat#secret", envAuth: authNone}, want: "fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearLLMConfigEnv(t)
			for name, value := range test.environ {
				t.Setenv(name, value)
			}
			_, err := NewFromEnv()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestSafeProviderErrorTextIsBounded(t *testing.T) {
	plain := `{"error":"capacity temporarily unavailable"}`
	if got := safeProviderErrorText([]byte(plain)); got != plain {
		t.Fatalf("plain bounded provider error = %q, want %q", got, plain)
	}
}

func clearLLMConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		envEndpoint, envModel, envAPIKey, envMaxTokens, envTimeout, envAuth,
		envChatTemplateKwargs,
		legacyEnvEndpoint, legacyEnvModel, legacyEnvAPIKey, "DEEPSEEK_MAX_TOKENS",
		legacyEnvTimeout, legacyEnvAuth,
		legacyEnvChatTemplateKwargs,
	} {
		value, present := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, value)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}
