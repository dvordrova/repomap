package run

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// A provider that writes the configured key back, in a refusal's body, a
// diagnostic header or even an accepted answer, leaves it in no file of the
// run, the cache or the console, for the text model and Jev alike; the
// status and the diagnostics stay (review B5's counter-probe: the key was in
// the saved debug payload of a 401).
func TestAKeyEchoedByTheProviderIsInNoFile(t *testing.T) {
	const key = "sk-fake-configured-key-0123456789"
	for _, test := range []struct {
		name     string
		status   int
		provider func(url string, client *http.Client) (llm.Provider, llm.Call[map[string]any])
		body     string
	}{
		{name: "text model refusal", status: http.StatusUnauthorized, body: `{"error":{"message":"invalid key ` + key + `"}}`, provider: textModel},
		{name: "text model answer", status: http.StatusOK, provider: textModel,
			body: `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"value\":\"` + key + `\"}"}}]}`},
		{name: "Jev refusal", status: http.StatusUnauthorized, body: `{"detail":"invalid key ` + key + `"}`, provider: jev},
		{name: "Jev answer", status: http.StatusOK, provider: jev, body: `{"answers":{"q":{"type":"noul","noul":0.9,"note":"` + key + `"}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "req for "+key)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			root := t.TempDir()
			writer, err := debugdump.NewWriter(root, "key-echo")
			if err != nil {
				t.Fatal(err)
			}
			var console bytes.Buffer
			output := newRunOutput(&console)
			output.abort = func(error) {}
			provider, call := test.provider(server.URL, server.Client())
			executor := debugdump.BindStage(llm.Executor{RootDir: root, Enabled: true,
				Observer: timed(output, debugdump.NewSemanticObserver(writer)),
			}, debugdump.SemanticStageGlossary)
			outcome, err := llm.ExecuteJSON(t.Context(), executor, provider, call)
			writer.Close()
			if (err == nil) != (test.status == http.StatusOK) || outcome.HTTPResponse == nil && test.status != http.StatusOK {
				t.Fatalf("outcome %+v, err %v", outcome, err)
			}
			if test.status != http.StatusOK && (outcome.HTTPResponse.StatusCode != test.status || !bytes.Contains(outcome.Response, []byte(llm.CredentialMarker)) ||
				outcome.HTTPResponse.Headers["X-Request-Id"][0] != "req for "+llm.CredentialMarker) {
				t.Fatalf("the refusal lost its diagnostics: %q %+v", outcome.Response, outcome.HTTPResponse)
			}
			if bytes.Contains(outcome.Response, []byte(key)) || strings.Contains(console.String(), key) || strings.Contains(fmt.Sprint(err), key) {
				t.Fatalf("the key is in the outcome, the console or the error:\n%s", console.String())
			}
			found := 0
			if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				raw, err := os.ReadFile(path)
				if err == nil && bytes.Contains(raw, []byte(key)) {
					return errors.New("the key reached " + path)
				}
				found++
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if found == 0 {
				t.Fatal("nothing was journaled or cached to check")
			}
		})
	}
}

func textModel(url string, client *http.Client) (llm.Provider, llm.Call[map[string]any]) {
	const key = "sk-fake-configured-key-0123456789"
	return &deepseek.Client{HTTPClient: client, Endpoint: url, APIKey: key, Model: "local-test", MaxTokens: llm.DefaultMaxOutputTokens},
		llm.Call[map[string]any]{
			State:  []byte(`{"stage":"glossary"}`),
			Prompt: llm.Prompt{System: "Answer.", User: `{"text":"exact request"}`, ResponseFormatJSON: true},
			Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
		}
}

func jev(url string, client *http.Client) (llm.Provider, llm.Call[map[string]any]) {
	const key = "sk-fake-configured-key-0123456789"
	jev := &typesafe.Client{HTTPClient: client, Endpoint: url, Model: "jev-test", APIKey: key}
	prompt, _ := jev.Prompt("task", map[string]any{}, map[string]llm.Question{"q": {Ask: "Yes?"}})
	return jev, llm.Call[map[string]any]{State: []byte(`{"stage":"glossary"}`), Prompt: prompt,
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 1}}
}
