package deepseek

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

type providerAnswer struct {
	status int
	body   []byte
}

func okAnswer(body []byte) providerAnswer { return providerAnswer{status: http.StatusOK, body: body} }

// serveAnswers answers the n-th request with answers[n], and every later one
// with the last answer. It returns the request bodies it received.
func serveAnswers(t *testing.T, answers ...providerAnswer) (*Client, func() [][]byte) {
	t.Helper()
	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		bodies = append(bodies, body)
		answer := answers[min(len(bodies), len(answers))-1]
		mu.Unlock()
		writer.WriteHeader(answer.status)
		_, _ = writer.Write(answer.body)
	}))
	t.Cleanup(server.Close)
	return llmProviderTestClient(server), func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), bodies...)
	}
}

// An HTTP 200 answer the provider left empty, or stopped for
// insufficient_system_resource, is the provider's fault: the same bytes go
// once more, counted as a transport attempt, and a second such answer is the
// refusal it always was.
func TestProviderNoAnswerIsSentOnceMore(t *testing.T) {
	success := llmProviderResponse("stop", `{"ok":true}`, nil)
	for name, noAnswer := range map[string][]byte{
		"empty content":                  llmProviderResponse("stop", "", nil),
		"blank content":                  llmProviderResponse("stop", " \n", nil),
		"insufficient resource, empty":   llmProviderResponse("insufficient_system_resource", "", nil),
		"insufficient resource, partial": llmProviderResponse("insufficient_system_resource", `{"ok":`, nil),
	} {
		t.Run(name, func(t *testing.T) {
			exact := []byte(`{"stable":[1,2,3]}`)
			prepared, err := llm.NewPrepared(exact)
			if err != nil {
				t.Fatal(err)
			}

			client, bodies := serveAnswers(t, okAnswer(noAnswer), okAnswer(success))
			completion, err := client.Complete(t.Context(), prepared)
			sent := bodies()
			if err != nil || string(completion.Response) != `{"ok":true}` || completion.FinishReason != llm.FinishStop {
				t.Fatalf("the second answer was not taken: %#v / %v", completion, err)
			}
			if len(sent) != 2 || !bytes.Equal(sent[0], exact) || !bytes.Equal(sent[1], exact) {
				t.Fatalf("the retry did not send the same bytes: %q", sent)
			}
			if completion.Metrics.Attempts != 2 || completion.Metrics.ProviderResponseBytes != len(noAnswer)+len(success) {
				t.Fatalf("the retry is missing from the transport metrics: %#v", completion.Metrics)
			}

			client, bodies = serveAnswers(t, okAnswer(noAnswer))
			completion, err = client.Complete(t.Context(), prepared)
			var source llm.ProviderFailureSource
			if !errors.As(err, &source) {
				t.Fatalf("a repeated empty answer was accepted: %#v / %v", completion, err)
			}
			failure := source.ProviderFailure()
			if calls := len(bodies()); calls != 2 || completion.Metrics.Attempts != 2 ||
				failure.Kind != llm.ProviderFailureResponse || failure.Attempts != 2 {
				t.Fatalf("a repeated empty answer: calls=%d, metrics=%#v, failure=%#v", calls, completion.Metrics, failure)
			}
		})
	}
}

// The provider bills an empty answer too, so the call's usage is that of both
// answers, whether the second one is accepted or refused.
func TestProviderNoAnswerRetryKeepsTheUsageOfBothAnswers(t *testing.T) {
	empty := okAnswer(llmProviderResponse("stop", "", map[string]any{
		"prompt_tokens": 1000, "completion_tokens": 3, "prompt_cache_miss_tokens": 1000,
		"completion_tokens_details": map[string]any{"reasoning_tokens": 2},
	}))
	answered := okAnswer(llmProviderResponse("stop", `{"ok":true}`, map[string]any{
		"prompt_tokens": 1000, "completion_tokens": 20, "prompt_cache_hit_tokens": 1000,
	}))
	for name, answers := range map[string][]providerAnswer{
		"second answer accepted": {empty, answered},
		"second answer empty":    {empty},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := serveAnswers(t, answers...)
			completion, _ := client.Complete(t.Context(), mustPrepared(t))
			got := completion.Metrics
			second := 3
			hit, miss := 0, 2000
			if len(answers) == 2 {
				second = 20
				hit, miss = 1000, 1000
			}
			if got.Attempts != 2 || !got.UsageReported || got.InputTokens != 2000 ||
				got.OutputTokens != 3+second || got.PromptCacheHitTokens != hit || got.PromptCacheMissTokens != miss {
				t.Fatalf("the first answer's usage is missing from the call: %#v", got)
			}
		})
	}
}

// The retry is the provider's missing answer only. A cut answer, another
// provider refusal or a decoder's refusal is returned from its one attempt.
func TestProviderRefusalsOtherThanNoAnswerAreNotSentAgain(t *testing.T) {
	success := okAnswer(llmProviderResponse("stop", `{"ok":true}`, nil))
	for name, refusal := range map[string]providerAnswer{
		"output cut with no content": okAnswer(llmProviderResponse("length", "", map[string]any{"completion_tokens": 100})),
		"content filter":             okAnswer(llmProviderResponse("content_filter", `{"ok":true}`, nil)),
		"no choices":                 okAnswer([]byte(`{"choices":[]}`)),
		"context limit": {status: http.StatusBadRequest,
			body: []byte(`{"error":{"code":"context_length_exceeded","message":"Input does not fit"}}`)},
	} {
		t.Run(name, func(t *testing.T) {
			client, bodies := serveAnswers(t, refusal, success)
			completion, err := client.Complete(t.Context(), mustPrepared(t))
			if calls := len(bodies()); err == nil || calls != 1 || completion.Metrics.Attempts != 1 {
				t.Fatalf("refusal was sent again: calls=%d, attempts=%d, err=%v", calls, completion.Metrics.Attempts, err)
			}
		})
	}

	t.Run("decoder refusal", func(t *testing.T) {
		client, bodies := serveAnswers(t, success)
		call := llmProviderFailureCall()
		call.Validate = func(map[string]any) error { return errors.New("the owner refuses this answer") }
		outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{Enabled: false}, client, call)
		if calls := len(bodies()); err == nil || calls != 1 || outcome.Metrics.Attempts != 1 {
			t.Fatalf("a decoder refusal was sent again: calls=%d, attempts=%d, err=%v", calls, outcome.Metrics.Attempts, err)
		}
	})
}

// The retry waits its ordinary backoff inside the caller's deadline; a
// deadline that ends first ends the call with no second request.
func TestProviderNoAnswerRetryStopsAtTheCallerDeadline(t *testing.T) {
	client, bodies := serveAnswers(t,
		okAnswer(llmProviderResponse("stop", "", nil)),
		okAnswer(llmProviderResponse("stop", `{"ok":true}`, nil)),
	)
	// The first retry waits at least 250 ms (backoffDuration(1)).
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	completion, err := client.Complete(ctx, mustPrepared(t))
	if calls := len(bodies()); !errors.Is(err, context.DeadlineExceeded) || calls > 1 || completion.Metrics.Attempts > 1 {
		t.Fatalf("the call did not end at the caller's deadline after one request: calls=%d, attempts=%d, err=%v", calls, completion.Metrics.Attempts, err)
	}
}

func mustPrepared(t *testing.T) llm.Prepared {
	t.Helper()
	prepared, err := llm.NewPrepared([]byte(`{"request":true}`))
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}
