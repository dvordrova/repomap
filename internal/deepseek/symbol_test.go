package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSymbolPromptContainsContractAndEvidenceRules(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 6000}
	bundle := []byte(`{"query":"kvServer.Put","target":{"evidence_id":"resolution-001"},"allowed_paths":["server/key.go"]}`)
	payload, err := client.SymbolPromptJSON(bundle)
	if err != nil {
		t.Fatalf("SymbolPromptJSON() error = %v", err)
	}

	var request chatRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatal("symbol request must use json_object response format")
	}
	if len(request.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(request.Messages))
	}
	content := request.Messages[0].Content + "\n" + request.Messages[1].Content
	for _, expected := range []string{
		"Static call edges are not runtime truth",
		"Every substantive interpretation must cite",
		"read_evidence_ids",
		"test_evidence_ids",
		"next_queries",
		"Do not emit target, callers, or callees",
		"resolution-001",
		"server/key.go",
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
}

func TestSymbolTaggedPromptUsesTextMode(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 6000}
	payload, err := client.SymbolTaggedPromptJSON([]byte(`{"query":"kvServer.Put","target":{"evidence_id":"resolution-001"}}`))
	if err != nil {
		t.Fatalf("SymbolTaggedPromptJSON() error = %v", err)
	}
	var request chatRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if request.ResponseFormat != nil {
		t.Fatal("tagged request must not force JSON response format")
	}
	content := request.Messages[0].Content + "\n" + request.Messages[1].Content
	for _, expected := range []string{"SUMMARY:", "READ:", "NEXT_QUERY:", "Do not emit TARGET"} {
		if !strings.Contains(content, expected) {
			t.Errorf("tagged prompt missing %q", expected)
		}
	}
}

func TestSymbolPromptRejectsInvalidBundleJSON(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 6000}
	if _, err := client.SymbolPromptJSON([]byte(`not json`)); err == nil {
		t.Fatal("SymbolPromptJSON() error = nil, want invalid json error")
	}
}

func TestNewPromptFromEnvDoesNotRetainAPIKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "must-not-be-retained")
	t.Setenv("DEEPSEEK_MODEL", "deepseek-v4-flash")
	t.Setenv("DEEPSEEK_MAX_TOKENS", "6000")
	t.Setenv("DEEPSEEK_ENDPOINT", "https://api.example.com/chat/completions")

	client, err := NewPromptFromEnv()
	if err != nil {
		t.Fatalf("NewPromptFromEnv() error = %v", err)
	}
	if client.APIKey != "" {
		t.Fatal("prompt-only client retained API key")
	}
}

func TestExplainSymbolCallsJSONEndpoint(t *testing.T) {
	t.Parallel()

	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ = io.ReadAll(request.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		APIKey:     "test-key",
		Model:      "deepseek-v4-flash",
		MaxTokens:  6000,
		Endpoint:   server.URL,
	}
	if _, err := client.ExplainSymbol(context.Background(), []byte(`{"query":"Run"}`)); err != nil {
		t.Fatalf("ExplainSymbol() error = %v", err)
	}

	var request chatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if request.Model != "deepseek-v4-flash" || request.MaxTokens != 6000 {
		t.Fatalf("request config = model %q, max_tokens %d", request.Model, request.MaxTokens)
	}
}

func TestExplainSymbolDoesNotRejectWeakNonJSONContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"SUMMARY: inferred\\nSUMMARY_EVIDENCE: resolution-001"}}]}`)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(), APIKey: "test-key", Model: "deepseek-v4-flash",
		MaxTokens: 6000, Endpoint: server.URL,
	}
	raw, err := client.ExplainSymbol(context.Background(), []byte(`{"query":"Run"}`))
	if err != nil {
		t.Fatalf("ExplainSymbol() error = %v", err)
	}
	if !strings.HasPrefix(string(raw), "SUMMARY:") {
		t.Fatalf("response = %q", raw)
	}
}
