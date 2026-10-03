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
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// Jev refusing the key or the balance reads like the text model's refusal:
// the receipt names the stage, Jev and the HTTP status with its diagnostic
// headers, links the exact request and the last response, and the run stops
// naming Jev's key or account (review A8: Jev's failed HTTP returned an
// empty completion, so the receipt had no status and the run walked on).
// The key never reaches the console or the run's files, even when Jev's
// error body echoes it.
func TestAJevRefusalIsAReadableReceiptAndStopsTheRun(t *testing.T) {
	const key = "jev-test-key-0123456789"
	for _, test := range []struct {
		status int
		echo   bool
		cause  string
	}{
		{http.StatusUnauthorized, true, "Jev refused the credentials: HTTP 401 at stage glossary; check Jev's API key"},
		{http.StatusForbidden, false, "Jev refused the credentials: HTTP 403 at stage glossary; check Jev's API key"},
		{http.StatusPaymentRequired, false, "Jev refused for balance: HTTP 402 at stage glossary; top up Jev's account"},
	} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			calls := 0
			body := `{"detail":"refused"}`
			if test.echo {
				body = `{"detail":"invalid key ` + key + `"}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("X-Request-Id", "jev-local-1")
				w.Header().Set("Set-Cookie", "private-cookie")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			root := t.TempDir()
			writer, err := debugdump.NewWriter(root, "jev-refusal")
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			var console bytes.Buffer
			output := newRunOutput(&console)
			var stopped error
			output.abort = func(cause error) { stopped = cause }
			client := &typesafe.Client{HTTPClient: server.Client(), Endpoint: server.URL, Model: "jev-test", APIKey: key}
			categorizer, err := newRunCategorizer(func() (llm.Categorizer, error) { return client, nil }, output)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := client.Prompt("task", map[string]any{}, map[string]llm.Question{"q": {Ask: "Yes?"}})
			if err != nil {
				t.Fatal(err)
			}
			executor := debugdump.BindStage(llm.Executor{RootDir: root, Enabled: true,
				Observer: timed(output, debugdump.NewSemanticObserver(writer)),
			}, debugdump.SemanticStageGlossary)
			_, err = llm.ExecuteJSON(t.Context(), executor, categorizer, llm.Call[map[string]any]{
				State: []byte(`{"stage":"glossary"}`), Prompt: prompt,
				Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 1},
			})
			text := console.String()
			if err == nil || calls != 1 || !strings.Contains(err.Error(), fmt.Sprintf("class=http_status status=%d attempts=1", test.status)) {
				t.Fatalf("refusal was retried or not typed: calls=%d err=%v\n%s", calls, err, text)
			}
			for _, want := range []string{"Model request failed", "stage: glossary", fmt.Sprintf("last HTTP response: %d from Jev", test.status),
				`response header X-Request-Id: "jev-local-1"`, "request: ", "journal: ", "Stopping the run", test.cause} {
				if !strings.Contains(text, want) {
					t.Fatalf("receipt omits %q:\n%s", want, text)
				}
			}
			if stopped == nil || !strings.Contains(stopped.Error(), test.cause) {
				t.Fatalf("the run was not stopped by the refusal: %v", stopped)
			}
			if strings.Contains(text, "raw response (last attempt): unavailable") {
				t.Fatalf("the refusal's response was dropped:\n%s", text)
			}
			if strings.Contains(text, "private-cookie") || strings.Contains(text, key) {
				t.Fatalf("the console shows a cookie or the key:\n%s", text)
			}
			if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				raw, err := os.ReadFile(path)
				if err == nil && bytes.Contains(raw, []byte(key)) {
					return errors.New("the key reached " + path)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
