package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestOwnerHTTP500OrTimeoutSplitsAndWarmRunSkipsRefusedParent(t *testing.T) {
	for _, serverDelay := range []time.Duration{0, 3*time.Minute + 45*time.Second, 5 * time.Minute} {
		t.Run(serverDelay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var attempts atomic.Int32
				client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					var request chatRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if strings.Contains(request.Messages[1].Content, "a,b") {
						select {
						case <-r.Context().Done():
						case <-time.After(serverDelay):
						}
						w.WriteHeader(500) // opaque failure, even before the local deadline
						return
					}
					_, _ = w.Write(llmProviderResponse("stop", `{"ok":true}`, nil))
				}))
				client.OnRetry = func(RetryProgress) { t.Error("divisible refused request retried identical bytes") }
				build := func(items []string) ([]llm.Call[map[string]any], error) {
					calls := make([]llm.Call[map[string]any], len(items))
					for i, item := range items {
						calls[i] = llmProviderFailureCall()
						calls[i].State = []byte("test")
						calls[i].Prompt.User = item
						calls[i].Limits.AttemptTimeout = 4 * time.Minute
						calls[i].SplitHTTP500 = strings.Contains(item, ",")
					}
					return calls, nil
				}
				split := func(item string) (string, string, bool) {
					left, right, found := strings.Cut(item, ",")
					return left, right, found
				}
				executor := llm.Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 4}
				started := time.Now()
				for run := 0; run < 2; run++ {
					plan, outcomes, err := llm.ExecuteAdaptiveJSONBatch(t.Context(), executor, client, []string{"a,b", "c"}, build, split)
					if err != nil || len(plan) != 3 || len(outcomes) != 3 || attempts.Load() != 4 {
						t.Fatalf("run %d: plan %v, calls %d, error %v", run, plan, attempts.Load(), err)
					}
					for _, outcome := range outcomes {
						if outcome.Value["ok"] != true || run == 1 && !outcome.Cached {
							t.Fatal("a child was lost or its current cache was not reused")
						}
					}
				}
				if elapsed := time.Since(started); elapsed != min(serverDelay, 4*time.Minute) {
					t.Fatalf("refused request consumed %v, want only the first refusal", elapsed)
				}
			})
		})
	}
}

func TestOwnerAttemptTimeoutDoesNotSplitRunCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			w.WriteHeader(500)
		}))
		call := llmProviderFailureCall()
		call.Limits.AttemptTimeout = 4 * time.Minute
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		outcome, err := llm.ExecuteJSON(ctx, llm.Executor{}, client, call)
		var resource *llm.ResourceLimitError
		if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &resource) || outcome.Metrics.Attempts != 1 {
			t.Fatalf("run cancellation became a split or retry: %v / %+v", err, outcome.Metrics)
		}
	})
}

func TestRetryProgressAnnouncesFailuresBeforeWaitAndActualStarts(t *testing.T) {
	for _, status := range []int{500, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				var events []RetryProgress
				client := llmProviderHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts++
					if attempts == 1 {
						w.WriteHeader(status)
						_, _ = w.Write([]byte("private provider diagnostic"))
						return
					}
					if len(events) != 2 || !events[1].Starting {
						t.Error("second transport started before its progress event")
					}
					_, _ = w.Write(llmProviderResponse("stop", `{"ok":true}`, nil))
				}))
				client.OnRetry = func(event RetryProgress) { events = append(events, event) }
				call := llmProviderFailureCall()
				call.Limits.AttemptTimeout = 4 * time.Minute
				call.SplitHTTP500 = status != 500 // a singleton 500 and other statuses still retry
				outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, call)
				if err != nil || outcome.Metrics.Attempts != 2 || len(events) != 2 {
					t.Fatalf("ordinary HTTP retry changed: %v / %+v", err, events)
				}
				failed, started := events[0], events[1]
				if failed.Starting || failed.Attempt != 1 || started.Attempt != 2 || failed.MaxAttempts != 4 ||
					failed.HTTPStatus != status || failed.Failure != llm.ProviderFailureHTTPStatus || failed.Delay <= 0 ||
					failed.RequestSHA256 != started.RequestSHA256 || len(failed.RequestSHA256) != 64 ||
					started.Elapsed < failed.Delay {
					t.Fatalf("retry order, reason, request identity or delay lost: %+v", events)
				}
				if status == 429 && failed.Delay < time.Minute {
					t.Fatal("rate-limit progress omitted the shared cooldown")
				}
				wire, _ := json.Marshal(events)
				if strings.Contains(string(wire), "private") || strings.Contains(string(wire), "fixture") {
					t.Fatal("retry callback leaked provider or request prose")
				}
			})
		})
	}
}
