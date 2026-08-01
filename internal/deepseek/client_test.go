package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewFromEnvUsesRepomapConfigBeforeAliases(t *testing.T) {
	clearLLMConfigEnv(t)

	t.Setenv(envEndpoint, "https://internal.example.com/v1/chat/completions")
	t.Setenv(envModel, "internal-code-model")
	t.Setenv(envAPIKey, "repomap-key")
	t.Setenv(envMaxTokens, "1234")
	t.Setenv(envTimeout, "7.5s")
	t.Setenv(envAuth, authBearer)

	t.Setenv(legacyEnvEndpoint, "https://legacy.example.com/chat/completions")
	t.Setenv(legacyEnvModel, "legacy-model")
	t.Setenv(legacyEnvAPIKey, "legacy-key")
	t.Setenv(legacyEnvMaxTokens, "9999")
	t.Setenv(legacyEnvTimeout, "45s")
	t.Setenv(legacyEnvAuth, authNone)

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	if client.Endpoint != "https://internal.example.com/v1/chat/completions" {
		t.Errorf("Endpoint = %q", client.Endpoint)
	}
	if client.Model != "internal-code-model" {
		t.Errorf("Model = %q", client.Model)
	}
	if client.APIKey != "repomap-key" {
		t.Errorf("APIKey = %q, want primary key", client.APIKey)
	}
	if client.MaxTokens != 1234 {
		t.Errorf("MaxTokens = %d", client.MaxTokens)
	}
	if client.HTTPClient.Timeout != 7500*time.Millisecond {
		t.Errorf("Timeout = %s", client.HTTPClient.Timeout)
	}
	if client.Auth != authBearer {
		t.Errorf("Auth = %q", client.Auth)
	}
}

func TestNewFromEnvSupportsDeepSeekCompatibilityAliases(t *testing.T) {
	clearLLMConfigEnv(t)

	t.Setenv(legacyEnvEndpoint, "http://127.0.0.1:8080/v1/chat/completions")
	t.Setenv(legacyEnvModel, "legacy-compatible-model")
	t.Setenv(legacyEnvAPIKey, "legacy-key")
	t.Setenv(legacyEnvMaxTokens, "2048")
	t.Setenv(legacyEnvTimeout, "12s")
	t.Setenv(legacyEnvAuth, authBearer)

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	if client.Endpoint != "http://127.0.0.1:8080/v1/chat/completions" ||
		client.Model != "legacy-compatible-model" ||
		client.APIKey != "legacy-key" ||
		client.MaxTokens != 2048 ||
		client.HTTPClient.Timeout != 12*time.Second ||
		client.Auth != authBearer {
		t.Fatalf("compatibility config = %#v, timeout = %s", client, client.HTTPClient.Timeout)
	}
}

func TestNewFromEnvSupportsDeepSeekNoAuthAlias(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(legacyEnvEndpoint, "http://127.0.0.1:11434/v1/chat/completions")
	t.Setenv(legacyEnvAuth, authNone)

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	if client.Auth != authNone || client.APIKey != "" {
		t.Fatalf("legacy no-auth config = auth %q, key %q", client.Auth, client.APIKey)
	}
}

func TestNewFromEnvLegacyNoAuthRequiresExplicitEndpoint(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(legacyEnvAuth, authNone)

	_, err := NewFromEnv()
	if err == nil || !strings.Contains(err.Error(), legacyEnvEndpoint) {
		t.Fatalf("NewFromEnv() error = %v, want explicit legacy endpoint", err)
	}
}

func TestNewFromEnvDefaultsToDeepSeekBearerConfig(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(legacyEnvAPIKey, "test-key")

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	if client.Endpoint != defaultEndpoint || client.Model != defaultModel {
		t.Fatalf("default endpoint/model = %q / %q", client.Endpoint, client.Model)
	}
	if client.MaxTokens != defaultMaxTokens || client.HTTPClient.Timeout != defaultTimeout {
		t.Fatalf("default limits = %d / %s", client.MaxTokens, client.HTTPClient.Timeout)
	}
	if client.Auth != authBearer || client.APIKey != "test-key" {
		t.Fatalf("default auth = %q, key = %q", client.Auth, client.APIKey)
	}
}

func TestNewFromEnvDoesNotMixGenericAndDeepSeekCredentials(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envEndpoint, "https://internal.example.com/v1/chat/completions")
	t.Setenv(envModel, "internal-model")
	t.Setenv(legacyEnvAPIKey, "deepseek-secret")

	_, err := NewFromEnv()
	if err == nil || !strings.Contains(err.Error(), envAPIKey) {
		t.Fatalf("NewFromEnv() error = %v, want missing generic key", err)
	}
}

func TestNewFromEnvGenericConfigRequiresExplicitEndpoint(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envAPIKey, "internal-secret")

	_, err := NewFromEnv()
	if err == nil || !strings.Contains(err.Error(), envEndpoint) {
		t.Fatalf("NewFromEnv() error = %v, want explicit generic endpoint", err)
	}
}

func TestNewFromEnvDefaultBearerRequiresKey(t *testing.T) {
	clearLLMConfigEnv(t)

	_, err := NewFromEnv()
	if err == nil {
		t.Fatal("NewFromEnv() error = nil, want missing key error")
	}
	if !strings.Contains(err.Error(), envAPIKey) || strings.Contains(strings.ToLower(err.Error()), "deepseek") {
		t.Fatalf("error = %q, want provider-neutral %s guidance", err, envAPIKey)
	}
}

func TestNewFromEnvExplicitBlankPrimaryDoesNotRevealAlias(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envEndpoint, "https://internal.example.com/v1/chat/completions")
	t.Setenv(envAPIKey, "   ")
	t.Setenv(legacyEnvAPIKey, "stale-legacy-key")

	_, err := NewFromEnv()
	if err == nil || !strings.Contains(err.Error(), envAPIKey) {
		t.Fatalf("NewFromEnv() error = %v, want missing primary key", err)
	}
}

func TestNewFromEnvAuthNoneDoesNotRetainKey(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envEndpoint, "http://127.0.0.1:11434/v1/chat/completions")
	t.Setenv(envAuth, authNone)
	t.Setenv(envAPIKey, "must-not-be-retained")

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	if client.Auth != authNone {
		t.Fatalf("Auth = %q, want %q", client.Auth, authNone)
	}
	if client.APIKey != "" {
		t.Fatal("no-auth client retained an unused API key")
	}
}

func TestNewPromptFromEnvNeverRetainsConfiguredKeys(t *testing.T) {
	clearLLMConfigEnv(t)
	t.Setenv(envAPIKey, "primary-secret")
	t.Setenv(legacyEnvAPIKey, "legacy-secret")
	t.Setenv(envEndpoint, "https://internal.example.com/v1/chat/completions")

	client, err := NewPromptFromEnv()
	if err != nil {
		t.Fatalf("NewPromptFromEnv() error = %v", err)
	}
	if client.APIKey != "" {
		t.Fatal("prompt-only client retained an API key")
	}
	prompt, err := client.OrientPromptJSON([]byte(`{}`))
	if err != nil {
		t.Fatalf("OrientPromptJSON() error = %v", err)
	}
	if strings.Contains(string(prompt), "primary-secret") || strings.Contains(string(prompt), "legacy-secret") {
		t.Fatal("prompt-only request contains an API key")
	}
}

func TestOrientPromptJSONMatchesRequestBody(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Model:      "capture-model",
		MaxTokens:  123,
		Endpoint:   server.URL,
		Auth:       authNone,
	}
	bundle := []byte(`{"repo_name":"fixture"}`)
	want, err := client.OrientPromptJSON(bundle)
	if err != nil {
		t.Fatalf("OrientPromptJSON() error = %v", err)
	}
	if _, err := client.Orient(context.Background(), bundle); err != nil {
		t.Fatalf("Orient() error = %v", err)
	}
	if string(received) != string(want) {
		t.Fatalf("provider body differs from preview\nprovider: %s\npreview:  %s", received, want)
	}
}

func TestFlowExplainPromptJSONMatchesRequestBody(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Model:      "capture-model",
		MaxTokens:  123,
		Endpoint:   server.URL,
		Auth:       authNone,
	}
	want, err := client.FlowExplainPromptJSON("user", "system")
	if err != nil {
		t.Fatalf("FlowExplainPromptJSON() error = %v", err)
	}
	if _, err := client.FlowExplain(context.Background(), "user", "system"); err != nil {
		t.Fatalf("FlowExplain() error = %v", err)
	}
	if string(received) != string(want) {
		t.Fatalf("provider body differs from preview\nprovider: %s\npreview:  %s", received, want)
	}
}

func TestNewFromEnvRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		environ map[string]string
		want    string
	}{
		{name: "max tokens not integer", environ: map[string]string{envMaxTokens: "many"}, want: envMaxTokens},
		{name: "max tokens zero", environ: map[string]string{envMaxTokens: "0"}, want: "positive"},
		{name: "timeout malformed", environ: map[string]string{envTimeout: "later"}, want: envTimeout},
		{name: "timeout zero", environ: map[string]string{envTimeout: "0s"}, want: "positive"},
		{name: "timeout negative", environ: map[string]string{envTimeout: "-1s"}, want: "positive"},
		{name: "auth implicit spelling rejected", environ: map[string]string{envAuth: "NONE"}, want: envAuth},
		{name: "auth unsupported", environ: map[string]string{envAuth: "basic"}, want: envAuth},
		{name: "endpoint relative", environ: map[string]string{envEndpoint: "/v1/chat/completions", envAuth: authNone}, want: "scheme"},
		{name: "endpoint ftp", environ: map[string]string{envEndpoint: "ftp://models.example.com/chat", envAuth: authNone}, want: "scheme"},
		{name: "endpoint missing host", environ: map[string]string{envEndpoint: "https:///chat/completions", envAuth: authNone}, want: "host"},
		{name: "endpoint with userinfo", environ: map[string]string{envEndpoint: "https://user:password@models.example.com/chat", envAuth: authNone}, want: "userinfo"},
		{name: "endpoint with query secret", environ: map[string]string{envEndpoint: "https://models.example.com/chat?api_key=sk-secret", envAuth: authNone}, want: "query"},
		{name: "endpoint with fragment", environ: map[string]string{envEndpoint: "https://models.example.com/chat#secret", envAuth: authNone}, want: "fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearLLMConfigEnv(t)
			for name, value := range test.environ {
				t.Setenv(name, value)
			}
			_, err := NewFromEnv()
			if err == nil {
				t.Fatal("NewFromEnv() error = nil")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestOrientAuthNoneOmitsAuthorizationHeader(t *testing.T) {
	authHeaders := make(chan []string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders <- r.Header.Values("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "must-not-be-sent",
		Model:      "internal-model",
		MaxTokens:  100,
		Endpoint:   srv.URL,
		Auth:       authNone,
	}
	if _, err := client.Orient(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("Orient() error = %v", err)
	}
	if headers := <-authHeaders; len(headers) != 0 {
		t.Fatalf("Authorization headers = %q, want none", headers)
	}
}

func TestOrientRequestHeaders(t *testing.T) {
	var gotAuth, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	_, err := c.Orient(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
}

func TestOrientResponseFormat(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	_, err := c.Orient(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	var parsed chatRequest
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ResponseFormat == nil {
		t.Fatal("response_format is nil")
	}
	if parsed.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format.type = %q, want %q", parsed.ResponseFormat.Type, "json_object")
	}
	if parsed.MaxTokens != 4000 {
		t.Fatalf("max_tokens = %d, want %d", parsed.MaxTokens, 4000)
	}
	if parsed.Temperature == nil || *parsed.Temperature != 0.1 {
		t.Fatalf("temperature = %v, want 0.1", parsed.Temperature)
	}
}

func TestOrientValidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"key\": \"value\"}"}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	result, err := c.Orient(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"key": "value"}` {
		t.Fatalf("got %q, want %q", string(result), `{"key": "value"}`)
	}
}

func TestOrientMeasuredReportsPromptCacheTokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"content":"{}"}}],
			"usage":{
				"prompt_tokens":120,
				"completion_tokens":17,
				"prompt_cache_hit_tokens":96,
				"prompt_cache_miss_tokens":24
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
	result, err := client.OrientMeasured(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("OrientMeasured() error = %v", err)
	}
	if result.InputTokens != 120 || result.OutputTokens != 17 ||
		result.PromptCacheHitTokens != 96 || result.PromptCacheMissTokens != 24 {
		t.Fatalf("OrientMeasured() token usage = %#v", result)
	}
}

func TestOrientInvalidJSON(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"not json"}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	_, err := c.Orient(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("error should mention invalid JSON, got: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 for non-truncated invalid JSON", attempts)
	}
}

func TestOrientRetriesOneExplicitLengthCompletionWithMoreHeadroom(t *testing.T) {
	t.Parallel()

	attempts := 0
	seenMaxTokens := make([]int, 0, 2)
	seenRequests := make([]chatRequest, 0, 2)
	attemptedRequestBytes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		attemptedRequestBytes += len(requestBody)
		var request chatRequest
		if err := json.Unmarshal(requestBody, &request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		seenMaxTokens = append(seenMaxTokens, request.MaxTokens)
		seenRequests = append(seenRequests, request)
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			_, _ = io.WriteString(w, `{
				"choices":[{"finish_reason":"length","message":{"content":"{\"project_guess\":\"cut"}}],
				"usage":{"prompt_tokens":100,"completion_tokens":6000}
			}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"choices":[{"finish_reason":"stop","message":{"content":"{}"}}],
			"usage":{"prompt_tokens":100,"completion_tokens":20}
		}`)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		Model:      "deepseek-v4-flash",
		MaxTokens:  6000,
		Endpoint:   srv.URL,
		Auth:       authNone,
	}
	result, err := client.OrientMeasured(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("OrientMeasured() error = %v", err)
	}
	if attempts != 2 || !reflect.DeepEqual(seenMaxTokens, []int{6000, 12000}) {
		t.Fatalf("attempts/max_tokens = %d/%v, want 2/[6000 12000]", attempts, seenMaxTokens)
	}
	secondMaxTokens := seenRequests[1].MaxTokens
	seenRequests[1].MaxTokens = seenRequests[0].MaxTokens
	if !reflect.DeepEqual(seenRequests[0], seenRequests[1]) {
		t.Fatalf("recovery request changed fields other than max_tokens:\nfirst=%#v\nsecond=%#v", seenRequests[0], seenRequests[1])
	}
	seenRequests[1].MaxTokens = secondMaxTokens
	if result.Attempts != 2 || result.CompletionRetries != 1 ||
		result.RequestBytes != attemptedRequestBytes ||
		result.InputTokens != 200 || result.OutputTokens != 6020 {
		t.Fatalf("OrientMeasured() telemetry = %#v", result)
	}
	if string(result.Content) != `{}` {
		t.Fatalf("OrientMeasured() content = %q", result.Content)
	}
}

func TestOrientStopsAfterTwoExplicitLengthCompletions(t *testing.T) {
	t.Parallel()

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"finish_reason":"length","message":{"content":"{\"cut\":"}}],
			"usage":{"completion_tokens":6000}
		}`)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(), Model: "deepseek-v4-flash",
		MaxTokens: 6000, Endpoint: srv.URL, Auth: authNone,
	}
	result, err := client.OrientMeasured(context.Background(), []byte(`{}`))
	if err == nil || !errors.Is(err, errJSONCompletionTruncated) {
		t.Fatalf("OrientMeasured() error = %v, want typed truncation", err)
	}
	if attempts != 2 || result.Attempts != 2 || result.CompletionRetries != 1 {
		t.Fatalf("attempts/result = %d/%#v, want exactly two envelopes", attempts, result)
	}
}

func TestOrientEmptyContentIncludesSafeCompletionDiagnostics(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{
				"finish_reason":"length",
				"message":{"content":null,"reasoning_content":"private provider reasoning"}
			}],
			"usage":{
				"completion_tokens":6000,
				"completion_tokens_details":{"reasoning_tokens":6000}
			}
		}`)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		Model:      "deepseek-v4-flash",
		MaxTokens:  6000,
		Endpoint:   srv.URL,
		Auth:       authNone,
	}
	_, err := client.Orient(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("Orient() error = nil")
	}
	for _, want := range []string{
		"llm response content is empty",
		"finish_reason=length",
		"completion_tokens=6000",
		"reasoning_tokens=6000",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Orient() error = %q, want %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "private provider reasoning") {
		t.Fatal("Orient() error exposed provider reasoning content")
	}
}

func TestOrientRetryOn500(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	_, err := c.Orient(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestCheckJSONCompatibilityUsesOneSmallRequest(t *testing.T) {
	requests := 0
	var maxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request chatRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		maxTokens = request.MaxTokens
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "internal-model",
		MaxTokens:  6000,
		Endpoint:   srv.URL,
		Auth:       authBearer,
	}
	if err := client.CheckJSONCompatibility(context.Background()); err == nil {
		t.Fatal("CheckJSONCompatibility() error = nil")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want exactly 1", requests)
	}
	if maxTokens != 64 {
		t.Fatalf("max_tokens = %d, want 64", maxTokens)
	}
}

func TestOrientNoRetryOn400(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	_, err := c.Orient(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry)", attempts)
	}
}

func TestOrientNon2xxIncludesBoundedSafeBody(t *testing.T) {
	t.Run("ordinary body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"unsupported response_format"}`)
		}))
		defer srv.Close()

		client := &Client{HTTPClient: srv.Client(), APIKey: "test-key", Endpoint: srv.URL, Auth: authBearer}
		_, err := client.Orient(context.Background(), []byte(`{}`))
		if err == nil || !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "unsupported response_format") {
			t.Fatalf("Orient() error = %v", err)
		}
	})

	t.Run("credential-like body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"Bearer company-secret-token-value"}`)
		}))
		defer srv.Close()

		client := &Client{HTTPClient: srv.Client(), APIKey: "test-key", Endpoint: srv.URL, Auth: authBearer}
		_, err := client.Orient(context.Background(), []byte(`{}`))
		if err == nil || !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "[redacted:") {
			t.Fatalf("Orient() error = %v", err)
		}
		if strings.Contains(err.Error(), "company-secret-token-value") {
			t.Fatal("Orient() error exposed credential-like provider content")
		}
	})
}

func TestOrientRetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	_, err := c.Orient(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestOrientContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-chat",
		MaxTokens:  4000,
		Endpoint:   srv.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := c.Orient(ctx, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestOrientPromptContainsJSONWord(t *testing.T) {
	c := &Client{
		HTTPClient: &http.Client{},
		APIKey:     "test-key",
		Model:      "deepseek-v4-flash",
		MaxTokens:  4000,
		Endpoint:   "https://api.example.com",
	}

	req, err := json.Marshal(c.buildRequest([]byte(`{"test": true}`)))
	if err != nil {
		t.Fatal(err)
	}

	body := strings.ToLower(string(req))

	// Check response_format is json_object
	if !strings.Contains(body, `json_object`) {
		t.Fatal("request must include response_format json_object")
	}

	// Check user prompt mentions json (after JSON escaping it'll appear as \"json\" or raw)
	msgs := c.buildRequest([]byte(`{}`)).Messages
	found := false
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m.Content), "json") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("prompt content must contain the word 'json'")
	}
}

func TestOrientPromptContainsExampleShape(t *testing.T) {
	if OrientationPromptVersionJSON != "orientation-json-v13" {
		t.Fatalf("OrientationPromptVersionJSON = %q", OrientationPromptVersionJSON)
	}
	c := &Client{
		HTTPClient: &http.Client{},
		APIKey:     "test-key",
		Model:      "deepseek-v4-flash",
		MaxTokens:  4000,
		Endpoint:   "https://api.example.com",
	}

	msgs := c.buildRequest([]byte(`{}`)).Messages
	if strings.Contains(msgs[0].Content, "Go repository") || !strings.Contains(msgs[0].Content, "language_hints") {
		t.Fatalf("system prompt is not language-aware: %q", msgs[0].Content)
	}
	body := msgs[1].Content // user message

	expected := []string{
		"file_ref",
		"evidence_refs",
		"project_guess",
		"candidate_flows",
		"flow_type",
		`"operational"`,
		"source_signal",
		"cap confidence at 0.3",
		"strongest grounded evidence regardless of flow type",
		"first_files_to_open",
		"high_level_map",
		`"role"`,
		"orientation hypothesis",
		"important_domain_words",
		"questions_for_human",
		"Never shorten, extend, prefix, substitute, or repair a ref",
		"Evidence is selected only by exact evidence_refs",
		"embedded in the facts bundle are closed",
		"Never use a file ref where an evidence ref is required",
		"There is no unverified_paths response field",
		"go.command_traces are locally extracted bounded syntax evidence",
	}

	for _, field := range expected {
		if !strings.Contains(body, field) {
			t.Fatalf("prompt must contain expected JSON field %s", field)
		}
	}
}

func TestOrientationThinkingPolicyIsEndpointSpecific(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		endpoint     string
		wantThinking string
	}{
		{name: "official DeepSeek", endpoint: "https://api.deepseek.com/chat/completions", wantThinking: "disabled"},
		{name: "compatible endpoint", endpoint: "https://llm.example.test/chat/completions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &Client{Model: "fixture", MaxTokens: 6000, Endpoint: test.endpoint}
			request := client.buildRequest([]byte(`{}`))
			if test.wantThinking == "" {
				if request.Thinking != nil {
					t.Fatalf("thinking = %#v, want omitted", request.Thinking)
				}
				return
			}
			if request.Thinking == nil || request.Thinking.Type != test.wantThinking {
				t.Fatalf("thinking = %#v, want %q", request.Thinking, test.wantThinking)
			}
		})
	}
}

func TestWaitProgressStopsCooperatively(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan WaitProgress, 2)
	client := &Client{
		waitInterval: time.Millisecond,
		OnWait: func(progress WaitProgress) {
			select {
			case updates <- progress:
			default:
			}
		},
	}
	stop := client.startWaitProgress(ctx, "fixture stage")
	select {
	case update := <-updates:
		if update.Stage != "fixture stage" || update.Elapsed <= 0 {
			t.Fatalf("wait update = %#v", update)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for progress heartbeat")
	}
	stop()
	cancel()
}

func clearLLMConfigEnv(t *testing.T) {
	t.Helper()

	names := []string{
		envEndpoint,
		envModel,
		envAPIKey,
		envMaxTokens,
		envTimeout,
		envAuth,
		legacyEnvEndpoint,
		legacyEnvModel,
		legacyEnvAPIKey,
		legacyEnvMaxTokens,
		legacyEnvTimeout,
		legacyEnvAuth,
	}
	for _, name := range names {
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
