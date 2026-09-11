package terminology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

type glossaryTransportFunc func(*http.Request) (*http.Response, error)

func (f glossaryTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// Use the production compatible client and real http.Client deadlines, with
// exact request-bound local responses. synctest advances retries/deadlines
// without sleeping or contacting a provider.
func TestGlossaryClientTimeoutPreservesOptionalWorkButRunCancellationAborts(t *testing.T) {
	for _, stage := range []string{"generation", "reduction"} {
		for _, mode := range []string{"provider timeout", "run cancelled", "run deadline"} {
			t.Run(stage+"/"+mode, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					client := &deepseek.Client{HTTPClient: &http.Client{Timeout: 10 * time.Millisecond}, Model: "controlled-glossary", MaxTokens: llm.DefaultMaxOutputTokens, Endpoint: "https://glossary.invalid/chat/completions", Auth: "none"}
					responses := make(map[string]string)
					prepare := func(prompt llm.Prompt, limits llm.Limits, response string) {
						t.Helper()
						prepared, err := llm.Prepare(client, prompt, limits)
						if err != nil {
							t.Fatal(err)
						}
						responses[string(prepared.Bytes())] = response
					}
					var run func(context.Context, llm.Executor) error
					var retained func() bool
					if stage == "generation" {
						items := recoveryProse()
						for i, window := range [][]proseSource{items, items[:1], items[1:]} {
							call, err := generationCall(window)
							if err != nil {
								t.Fatal(err)
							}
							prepare(call.Prompt, call.Limits, []string{"length", `{"terms":[{"name":"Alpha","explanation":"Accepted sibling definition.","rows":["p1"]}]}`, "timeout"}[i])
						}
						collector := recoveryCollector(items)
						run = func(ctx context.Context, executor llm.Executor) error {
							return collector.Generate(ctx, executor, client)
						}
						retained = func() bool {
							got := collector.Snapshot()
							return len(collector.pending) == 2 && len(got) == 1 && got[0].Name == "Alpha" && reflect.DeepEqual(got[0].Sources, items[0].Sources) && reflect.DeepEqual(got[0].Origins, []Origin{items[0].Origin})
						}
					} else {
						items := []Candidate{termCandidate("Alpha", "First original definition.", "a.py"), termCandidate("alpha", "Second original definition.", "b.py")}
						var entries []Entry
						for _, item := range items {
							entry, err := makeEntry(item.Explanation, []Candidate{item})
							if err != nil {
								t.Fatal(err)
							}
							entries = append(entries, entry)
						}
						for i, window := range [][]Entry{entries, entries[:1], entries[1:]} {
							call, err := reductionCall(window)
							if err != nil {
								t.Fatal(err)
							}
							prepare(call.Prompt, call.Limits, []string{"length", `{"assignments":[{"ref":"g1","representative":"v1"}]}`, "timeout"}[i])
						}
						var result Catalog
						run = func(ctx context.Context, executor llm.Executor) (err error) {
							result, err = Reduce(ctx, executor, client, items)
							return err
						}
						retained = func() bool {
							var variants []Candidate
							for _, entry := range result.Entries {
								variants = append(variants, entry.Variants...)
							}
							originals, err := normalizeCandidates(variants)
							return err == nil && result.PartialComparison && len(result.Requests) == 1 && reflect.DeepEqual(originals, items)
						}
					}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if mode == "run deadline" {
						var deadlineCancel context.CancelFunc
						ctx, deadlineCancel = context.WithTimeout(ctx, 5*time.Millisecond)
						defer deadlineCancel()
					}
					var mu sync.Mutex
					attempts := 0
					client.HTTPClient.Transport = glossaryTransportFunc(func(request *http.Request) (*http.Response, error) {
						body, err := io.ReadAll(request.Body)
						if err != nil {
							return nil, err
						}
						response, known := responses[string(body)]
						if !known {
							t.Error("unexpected complete request in controlled client")
							return nil, errors.New("unadvertised request")
						}
						mu.Lock()
						attempts++
						mu.Unlock()
						if mode == "run cancelled" {
							cancel()
						}
						if mode != "provider timeout" || response == "timeout" {
							<-request.Context().Done()
							return nil, request.Context().Err()
						}
						finish := "stop"
						if response == "length" {
							finish, response = "length", "{}"
						}
						content, _ := json.Marshal(response)
						wire := fmt.Sprintf(`{"choices":[{"message":{"content":%s},"finish_reason":%q}]}`, content, finish)
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(wire)), Request: request}, nil
					})
					var timeoutEvent bool
					executor := llm.Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 2, Observer: llm.ObserverFunc(func(event llm.Event) error {
						if event.Kind == llm.EventFailure && event.Failure == llm.FailureProvider && event.Metrics.Attempts == 4 {
							timeoutEvent = true
						}
						return nil
					})}
					err := run(ctx, executor)
					if mode == "provider timeout" {
						if err != nil || ctx.Err() != nil || !retained() || attempts != 6 || !timeoutEvent {
							t.Fatalf("provider-local deadline invalidated accepted work: err=%v ctx=%v retained=%v attempts=%d timeoutRecorded=%v", err, ctx.Err(), retained(), attempts, timeoutEvent)
						}
					} else if ctx.Err() == nil || !errors.Is(err, ctx.Err()) || attempts != 1 {
						t.Fatalf("actual cancelled run was swallowed or retried: err=%v ctx=%v attempts=%d", err, ctx.Err(), attempts)
					}
				})
			})
		}
	}
}
