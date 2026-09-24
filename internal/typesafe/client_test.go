package typesafe

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestClientSendsTheBodyWithItsModelAndRetriesRateLimits(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization: %q", r.Header.Get("Authorization"))
		}
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		var body map[string]json.RawMessage
		raw, _ := io.ReadAll(r.Body)
		if json.Unmarshal(raw, &body) != nil || string(body["model"]) != `"jev-test"` || body["questions"] == nil {
			t.Errorf("body: %s", raw)
		}
		w.Write([]byte(`{"model":"jev-test","answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":12,"output_tokens":3}}`))
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Model: "jev-test", APIKey: "secret"}
	prepared, err := client.Prepare(llm.Prompt{User: `{"state":"s","questions":{"q":{"type":"noul","instructions":"yes?"}}}`}, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if string(prepared.Bytes()) == "" || json.Valid(prepared.Bytes()) == false {
		t.Fatal("prepared request is not JSON")
	}
	if bytes.Contains(prepared.Bytes(), []byte("secret")) {
		t.Fatal("the key entered the prepared request")
	}
	completion, err := client.Complete(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if string(completion.Response) != `{"answers":{"q":{"type":"noul","noul":0.9}}}` || completion.Metrics.InputTokens != 12 || completion.Metrics.Attempts != 2 {
		t.Fatalf("completion: %s %+v", completion.Response, completion.Metrics)
	}
}

func TestNoKeyMeansNoClassifier(t *testing.T) {
	t.Setenv(envAPIKey, "")
	client, err := NewFromEnv()
	if client != nil || err != nil {
		t.Fatalf("client %+v err %v", client, err)
	}
}
