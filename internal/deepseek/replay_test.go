package deepseek

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestReplayUsesExactSavedOptionsThroughClient(t *testing.T) {
	raw := []byte(`{ "model":"saved-model", "messages":[{"role":"user","content":"json please"}], "max_tokens":731, "temperature":0.27, "thinking":{"type":"disabled"}, "response_format":{"type":"json_object"}, "future_option":true }`)
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("configured client auth missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(llmProviderResponse("stop", `{"ok":true}`, nil))
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: authBearer, APIKey: "test-key", Model: "different-default", MaxTokens: 1}
	prepared, err := PrepareReplay(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = llm.ReplayJSON(t.Context(), llm.Executor{RootDir: t.TempDir(), Enabled: true}, client, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(received, raw) {
		t.Fatalf("saved options changed on the wire: %s", received)
	}
}

func TestReplayRejectsTableInputAndIncompleteProviderRequests(t *testing.T) {
	for _, raw := range []string{`{"rows":[]}`, `{"model":"x","messages":[],"max_tokens":1,"temperature":0}`, `{"model":"x","messages":[{"role":"user","content":"x"}],"max_tokens":1}`, `{"model":"x","messages":[{"role":"user","content":"x"}],"max_tokens":1,"temperature":0,"stream":true}`} {
		if _, err := PrepareReplay([]byte(raw)); err == nil {
			t.Fatalf("accepted incomplete or unsupported request: %s", raw)
		}
	}
}
