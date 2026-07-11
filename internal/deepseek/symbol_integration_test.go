package deepseek_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/symbol"
)

func TestSymbolIntegrationWithFixedDeepSeekResponse(t *testing.T) {
	t.Parallel()

	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received, _ = io.ReadAll(request.Body)
		if request.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": string(deepseektest.SymbolResponseJSON)},
			}},
		})
	}))
	defer server.Close()

	client := &deepseek.Client{
		HTTPClient: server.Client(), APIKey: "fixture-key", Model: "deepseek-v4-flash",
		MaxTokens: 6000, Endpoint: server.URL,
	}
	raw, err := client.ExplainSymbol(context.Background(), deepseektest.SymbolBundleJSON)
	if err != nil {
		t.Fatalf("ExplainSymbol() error = %v", err)
	}
	var request map[string]any
	if err := json.Unmarshal(received, &request); err != nil {
		t.Fatalf("request is not JSON: %v", err)
	}
	if _, ok := request["response_format"]; !ok {
		t.Fatal("JSON prompt request omitted response_format")
	}

	var bundle symbol.Bundle
	if err := json.Unmarshal(deepseektest.SymbolBundleJSON, &bundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	parsed, err := symbol.ParseReport(bundle, raw)
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if score := symbol.Evaluate(parsed).Score; score != 100 {
		t.Fatalf("evaluation score = %d, want 100; warnings = %#v", score, parsed.Warnings)
	}
}
