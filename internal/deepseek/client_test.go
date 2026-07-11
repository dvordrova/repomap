package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
	if parsed.Temperature != 0.1 {
		t.Fatalf("temperature = %f, want 0.1", parsed.Temperature)
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

func TestOrientInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	c := &Client{
		HTTPClient: &http.Client{},
		APIKey:     "test-key",
		Model:      "deepseek-v4-flash",
		MaxTokens:  4000,
		Endpoint:   "https://api.example.com",
	}

	msgs := c.buildRequest([]byte(`{}`)).Messages
	body := msgs[1].Content // user message

	expected := []string{
		"project_guess",
		"candidate_flows",
		"first_files_to_open",
		"high_level_map",
		"important_domain_words",
		"questions_for_human",
	}

	for _, field := range expected {
		if !strings.Contains(body, field) {
			t.Fatalf("prompt must contain expected JSON field %s", field)
		}
	}
}
