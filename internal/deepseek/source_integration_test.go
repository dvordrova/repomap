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
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func TestSourceIntegrationWithFixedDeepSeekResponse(t *testing.T) {
	t.Parallel()

	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received, _ = io.ReadAll(request.Body)
		if request.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": string(deepseektest.SourceResponseJSON)},
			}},
		})
	}))
	defer server.Close()

	client := &deepseek.Client{
		HTTPClient: server.Client(),
		APIKey:     "fixture-key",
		Model:      "deepseek-v4-flash",
		MaxTokens:  2000,
		Endpoint:   server.URL,
	}
	var bundle sourceexplain.Bundle
	if err := json.Unmarshal(deepseektest.SourceBundleJSON, &bundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	explanation, err := sourceexplain.NewService(client).Explain(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if explanation.Evaluation.Score != 100 || len(explanation.Parsed.Report.Claims) != 4 {
		t.Fatalf("explanation = %#v", explanation)
	}
	var request map[string]any
	if err := json.Unmarshal(received, &request); err != nil {
		t.Fatalf("request is not JSON: %v", err)
	}
	responseFormat, ok := request["response_format"].(map[string]any)
	if !ok || responseFormat["type"] != "json_object" {
		t.Fatalf("response_format = %#v", request["response_format"])
	}
	if _, ok := request["authorization"]; ok {
		t.Fatal("request body contains authorization")
	}
}
