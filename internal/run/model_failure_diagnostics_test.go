package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestModelFailureConsoleLinksExactHTTP500Response(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty=%t", empty), func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			var lastRequest, lastResponse []byte
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request, _ := io.ReadAll(r.Body)
				mu.Lock()
				defer mu.Unlock()
				attempts++
				lastRequest = request
				lastResponse = nil
				if !empty {
					lastResponse = []byte(fmt.Sprintf("<html>upstream exploded</html>\nlast attempt %d\n", attempts))
				}
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("X-Request-Id", fmt.Sprintf("local-id-%d", attempts))
				w.Header().Set("X-Correlation-Id", "quoted\"value")
				w.Header().Set("Set-Cookie", "private-cookie")
				w.Header().Set("Authorization", "private-auth")
				w.Header().Set("X-Unrelated", "private-unrelated")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(lastResponse)
			}))
			defer server.Close()
			root := t.TempDir()
			writer, err := debugdump.NewWriter(root, "http500")
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			var console bytes.Buffer
			output := newRunOutput(&console)
			provider := &deepseek.Client{HTTPClient: server.Client(), Endpoint: server.URL, Auth: "none", Model: "local-test", MaxTokens: llm.DefaultMaxOutputTokens}
			executor := debugdump.BindStage(llm.Executor{RootDir: root, Enabled: true,
				Observer: timed(output, debugdump.NewSemanticObserver(writer)),
			}, debugdump.SemanticStageReportTranslation)
			_, err = llm.ExecuteJSON(t.Context(), executor, provider, llm.Call[map[string]any]{
				State:  []byte(`{"stage":"report_translation"}`),
				Prompt: llm.Prompt{System: "Translate.", User: `{"text":"exact request"}`, ResponseFormatJSON: true},
				Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
			})
			if err == nil || !strings.Contains(err.Error(), "status=500") {
				t.Fatalf("HTTP failure was obscured: %v", err)
			}
			text := console.String()
			if strings.Count(text, "Model request failed") != 1 || !strings.Contains(text, "stage: report_translation") || !strings.Contains(text, "status=500") || strings.Contains(text, "upstream exploded") {
				t.Fatalf("failure receipt is missing, duplicated or prints the raw body:\n%s", text)
			}
			pathAfter := func(label string) string {
				t.Helper()
				for _, line := range strings.Split(text, "\n") {
					if value, ok := strings.CutPrefix(strings.TrimSpace(line), label+": "); ok {
						if !filepath.IsAbs(value) {
							t.Fatalf("%s is not a direct absolute path: %q", label, value)
						}
						return value
					}
				}
				t.Fatalf("missing %s path:\n%s", label, text)
				return ""
			}
			request, err := os.ReadFile(pathAfter("request"))
			mu.Lock()
			wantRequest, wantResponse, count := lastRequest, lastResponse, attempts
			mu.Unlock()
			if err != nil || !bytes.Equal(request, wantRequest) || count < 2 {
				t.Fatalf("request receipt differs from transport or retries changed: attempts=%d, err=%v", count, err)
			}
			journalBytes, err := os.ReadFile(pathAfter("journal"))
			if err != nil {
				t.Fatal(err)
			}
			var journal debugdump.SemanticExchangeRecord
			if err := json.Unmarshal(journalBytes, &journal); err != nil || journal.State != debugdump.SemanticStateProviderFailed || journal.TransportAttempts != count {
				t.Fatalf("journal lost final failure accounting: %+v / %v", journal, err)
			}
			if journal.HTTPResponse == nil || journal.HTTPResponse.StatusCode != 500 || len(journal.HTTPResponse.Headers["X-Request-Id"]) != 1 || journal.HTTPResponse.Headers["X-Request-Id"][0] != fmt.Sprintf("local-id-%d", count) {
				t.Fatalf("journal lost last-attempt HTTP diagnostics: %+v", journal.HTTPResponse)
			}
			for _, want := range []string{"last HTTP response: 500", fmt.Sprintf("transport attempts: %d", count),
				formatRunOutputDuration(journal.LatencyMS), fmt.Sprintf("response header X-Request-Id: %q", fmt.Sprintf("local-id-%d", count)),
				`response header X-Correlation-Id: "quoted\"value"`} {
				if !strings.Contains(text, want) {
					t.Fatalf("console omitted diagnostic %q:\n%s", want, text)
				}
			}
			for _, forbidden := range []string{"private-cookie", "private-auth", "private-unrelated"} {
				if strings.Contains(text, forbidden) || strings.Contains(string(journalBytes), forbidden) {
					t.Fatalf("non-diagnostic response header escaped: %s", forbidden)
				}
			}
			if empty {
				if !strings.Contains(text, "raw response (last attempt): unavailable ("+debugdump.SemanticUnavailableNoContent+")") {
					t.Fatalf("empty body was shown as a saved response:\n%s", text)
				}
			} else {
				response, err := os.ReadFile(pathAfter("raw response (last attempt)"))
				if err != nil || !bytes.Equal(response, wantResponse) || json.Valid(response) {
					t.Fatalf("last non-JSON response was changed: %q / %v", response, err)
				}
			}
			if got := output.modelCallSummary(debugdump.SemanticStageReportTranslation); got != "provider requests: 1 new, 0 reused from cache" {
				t.Fatalf("receipt changed semantic call accounting: %s", got)
			}
		})
	}
}
