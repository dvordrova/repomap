package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

// handlerRoundTripper serves the client's real HTTP requests from a local
// handler in memory, so a test's waits run on synctest's clock.
type handlerRoundTripper struct{ handler http.Handler }

func (transport handlerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	transport.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func handlerClient(handler http.Handler) *Client {
	return &Client{
		HTTPClient: &http.Client{Transport: handlerRoundTripper{handler}},
		Endpoint:   "https://jev.example/v1/systemone", Model: "jev-test", APIKey: "secret",
	}
}

// questionCall is one categorizer request asking question key.
func questionCall(t *testing.T, client *Client, key string) llm.Call[map[string]any] {
	t.Helper()
	prompt, err := client.Prompt("task", map[string]any{}, map[string]llm.Question{key: {Ask: "Yes?"}})
	if err != nil {
		t.Fatal(err)
	}
	return llm.Call[map[string]any]{
		State: []byte(`{"test":"jev transport"}`), Prompt: prompt,
		Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1},
	}
}

// questionOf is the one question key a request asks.
func questionOf(t *testing.T, request *http.Request) string {
	var body struct {
		Questions map[string]json.RawMessage `json:"questions"`
	}
	raw, _ := io.ReadAll(request.Body)
	if err := json.Unmarshal(raw, &body); err != nil || len(body.Questions) != 1 {
		t.Errorf("request: %s", raw)
		return ""
	}
	for key := range body.Questions {
		return key
	}
	return ""
}

func answer(w http.ResponseWriter, key string) {
	_, _ = w.Write([]byte(`{"answers":{"` + key + `":{"type":"noul","noul":0.9}},"usage":{"input_tokens":5,"output_tokens":1}}`))
}

// A 429 cools down every new attempt to Jev, not only its own retry: a
// neighbour already on the wire finishes, a neighbour queued in the same
// batch and one in another stage's batch (its own executor and controller,
// as two readings have) wait for the cooldown the 429 set, then go one at a
// time through the run's one Jev gate.
func TestARateLimitCoolsDownEveryNeighbourThroughTheSharedGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			mu         sync.Mutex
			attempts   = map[string]int{}
			starts     = map[string][]time.Time{}
			active     int
			maxActive  int
			limitedAt  time.Time
			afterLimit int
		)
		limited, arrived := make(chan struct{}), make(chan struct{})
		client := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := questionOf(t, r)
			mu.Lock()
			attempts[key]++
			attempt := attempts[key]
			starts[key] = append(starts[key], time.Now())
			active++
			if !limitedAt.IsZero() {
				afterLimit = max(afterLimit, active)
			}
			maxActive = max(maxActive, active)
			mu.Unlock()
			defer func() {
				mu.Lock()
				active--
				mu.Unlock()
			}()
			switch {
			case key == "a" && attempt == 1:
				<-arrived // b is on the wire too
				mu.Lock()
				limitedAt = time.Now()
				mu.Unlock()
				w.Header().Set("Retry-After", "90")
				w.WriteHeader(http.StatusTooManyRequests)
				close(limited)
				return
			case key == "b":
				close(arrived)
				<-limited // on the wire when the 429 arrives
				time.Sleep(time.Second)
			}
			time.Sleep(time.Second)
			answer(w, key)
		}))
		client.Controller = llm.NewBatchController(2)
		first := llm.Executor{BatchConcurrency: 2, BatchController: &llm.BatchController{}}
		other := llm.Executor{BatchConcurrency: 4, BatchController: &llm.BatchController{}}
		done := make(chan []llm.EachResult[map[string]any], 2)
		go func() {
			done <- llm.ExecuteJSONEach(t.Context(), first, client, []llm.Call[map[string]any]{
				questionCall(t, client, "a"), questionCall(t, client, "b"), questionCall(t, client, "c"),
			})
		}()
		<-limited
		go func() {
			done <- llm.ExecuteJSONEach(t.Context(), other, client, []llm.Call[map[string]any]{questionCall(t, client, "d")})
		}()
		for range 2 {
			for _, result := range <-done {
				if result.Err != nil {
					t.Fatalf("a neighbour failed: %v", result.Err)
				}
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if attempts["a"] != 2 || attempts["b"] != 1 || attempts["c"] != 1 || attempts["d"] != 1 {
			t.Fatalf("attempts %v", attempts)
		}
		for _, start := range []time.Time{starts["a"][1], starts["c"][0], starts["d"][0]} {
			if start.Sub(limitedAt) < 90*time.Second {
				t.Fatalf("an attempt started %v after the 429, inside its 90 s cooldown: %v", start.Sub(limitedAt), starts)
			}
		}
		if maxActive != 2 || afterLimit != 1 {
			t.Fatalf("concurrency %d, after the 429 %d", maxActive, afterLimit)
		}
	})
}

// The wait after a 429 is at least a minute, longer when Retry-After asks
// for more in seconds or as an HTTP date; a past date, an invalid value or a
// shorter one keeps the minute. Other transport failures keep their short
// backoff.
func TestTheRateLimitWaitReadsRetryAfterInSecondsAndAsADate(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		header     string
		dateOffset time.Duration
		body       string
		want       time.Duration
	}{
		{name: "missing", status: 429, want: time.Minute},
		{name: "shorter seconds", status: 429, header: "5", want: time.Minute},
		{name: "longer seconds", status: 429, header: "90", want: 90 * time.Second},
		{name: "date", status: 429, dateOffset: 2 * time.Minute, want: 2 * time.Minute},
		{name: "past date", status: 429, dateOffset: -time.Minute, want: time.Minute},
		{name: "invalid", status: 429, header: "soon", want: time.Minute},
		{name: "reset after in the body", status: 429, header: "90", body: `{"error":{"message":"rate limit exceeded: retry after 9s, reset after 95s"}}`, want: 95 * time.Second},
		{name: "server error", status: 503, header: "90", want: 2 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var starts []time.Time
				client := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					starts = append(starts, time.Now())
					if len(starts) == 1 {
						header := test.header
						if test.dateOffset != 0 {
							header = time.Now().Add(test.dateOffset).UTC().Format(http.TimeFormat)
						}
						w.Header().Set("Retry-After", header)
						w.WriteHeader(test.status)
						_, _ = io.WriteString(w, test.body)
						return
					}
					answer(w, "q")
				}))
				outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, questionCall(t, client, "q"))
				if err != nil || len(starts) != 2 || outcome.Metrics.Attempts != 2 {
					t.Fatalf("attempts=%v outcome=%+v err=%v", starts, outcome, err)
				}
				if delay := starts[1].Sub(starts[0]); delay != test.want {
					t.Fatalf("retry waited %v, want %v", delay, test.want)
				}
			})
		})
	}
}

// Cancelling the run during a cooldown stops the call at once: no further
// attempt, and the error is the cancellation.
func TestCancellingTheCooldownStopsTheCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		calls := 0
		client := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
			go func() { time.Sleep(10 * time.Second); cancel() }()
		}))
		client.Controller = llm.NewBatchController(4)
		started := time.Now()
		_, err := llm.ExecuteJSON(ctx, llm.Executor{}, client, questionCall(t, client, "q"))
		if !errors.Is(err, context.Canceled) || calls != 1 || time.Since(started) != 10*time.Second {
			t.Fatalf("cancelled wait: calls=%d elapsed=%v err=%v", calls, time.Since(started), err)
		}
		// A neighbour asked after the cancellation waits on no one's behalf.
		next := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { answer(w, "q") }))
		if _, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, next, questionCall(t, next, "q")); err != nil {
			t.Fatal(err)
		}
	})
}

// A refused key or balance is one attempt, never retried, and a typed
// failure: its class and status, the last response's diagnostic headers and
// body, and Jev named as the provider, so the run can say whose key or
// account to check. A body that echoes the key is not kept.
func TestARefusalIsTypedWithItsHTTPDiagnostics(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			body := []byte(`{"detail":"refused"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("X-Request-Id", "local-1")
				w.Header().Set("Set-Cookie", "private-cookie")
				w.WriteHeader(status)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Model: "jev-test", APIKey: "secret"}
			outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, questionCall(t, client, "q"))
			var source llm.ProviderFailureSource
			if !errors.As(err, &source) || calls != 1 {
				t.Fatalf("HTTP %d: calls=%d err=%v", status, calls, err)
			}
			failure := source.ProviderFailure()
			if failure.Kind != llm.ProviderFailureHTTPStatus || failure.HTTPStatus != status || failure.Attempts != 1 || failure.RetryExhausted {
				t.Fatalf("HTTP %d failure: %+v (%v)", status, failure, err)
			}
			response := outcome.HTTPResponse
			if response == nil || response.StatusCode != status || response.Provider != "Jev" ||
				len(response.Headers["X-Request-Id"]) != 1 || response.Headers["Set-Cookie"] != nil {
				t.Fatalf("HTTP %d diagnostics: %+v", status, response)
			}
			if !bytes.Equal(outcome.Response, body) || bytes.Contains([]byte(err.Error()), body) {
				t.Fatalf("HTTP %d: kept response %q, error %v", status, outcome.Response, err)
			}
		})
	}
	t.Run("a body and a header echoing the key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "for secret")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail":"invalid key secret"}`))
		}))
		defer server.Close()
		client := &Client{HTTPClient: server.Client(), Endpoint: server.URL, Model: "jev-test", APIKey: "secret"}
		outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, questionCall(t, client, "q"))
		if err == nil || string(outcome.Response) != `{"detail":"invalid key `+llm.CredentialMarker+`"}` || outcome.HTTPResponse == nil ||
			outcome.HTTPResponse.StatusCode != http.StatusUnauthorized || outcome.HTTPResponse.Headers["X-Request-Id"][0] != "for "+llm.CredentialMarker {
			t.Fatalf("echoed key: response %q, %+v, %v", outcome.Response, outcome.HTTPResponse, err)
		}
	})
}

// Transport retries end: the last status, the attempts and that they were
// exhausted are the failure's, with the last attempt's diagnostics.
func TestExhaustedRetriesKeepTheLastAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		client := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusBadGateway)
		}))
		outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, questionCall(t, client, "q"))
		var source llm.ProviderFailureSource
		if !errors.As(err, &source) || calls != maxAttempts {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
		failure := source.ProviderFailure()
		if failure.Kind != llm.ProviderFailureHTTPStatus || failure.HTTPStatus != http.StatusBadGateway || failure.Attempts != maxAttempts || !failure.RetryExhausted ||
			outcome.HTTPResponse == nil || outcome.HTTPResponse.StatusCode != http.StatusBadGateway || outcome.Metrics.Attempts != maxAttempts {
			t.Fatalf("exhausted: %+v / %+v / %v", failure, outcome.HTTPResponse, err)
		}
	})
}

// An HTTP 200 that answers nothing is the provider's fault, as the text
// model's empty answer is: the same bytes go once more, through the gate,
// after the short backoff, and the call is billed for both answers. A second
// one is refused. Answers the decoder refuses, wholly or in part, and an
// unreadable envelope are never sent again.
func TestAnAnswerWithoutAnswersIsSentOnceMore(t *testing.T) {
	for _, test := range []struct {
		name     string
		replies  []string
		attempts int
		accepted bool
		input    int
	}{
		{name: "empty answers, then answers", replies: []string{`{"answers":{},"usage":{"input_tokens":5}}`, `{"answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":5,"output_tokens":1}}`}, attempts: 2, accepted: true, input: 10},
		{name: "null answers, then answers", replies: []string{`{"answers":null}`, `{"answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":5,"output_tokens":1}}`}, attempts: 2, accepted: true, input: 5},
		{name: "no answers field, then answers", replies: []string{`{"model":"jev-test"}`, `{"answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":5,"output_tokens":1}}`}, attempts: 2, accepted: true, input: 5},
		{name: "twice without answers", replies: []string{`{"answers":{}}`, `{"answers":[]}`, `{"answers":{"q":{"type":"noul","noul":0.9}}}`}, attempts: 2},
		{name: "an answer the decoder refuses", replies: []string{`{"answers":{"q":"unreadable"}}`, `{"answers":{"q":{"type":"noul","noul":0.9}}}`}, attempts: 1},
		{name: "an unreadable envelope", replies: []string{`not json`, `{"answers":{"q":{"type":"noul","noul":0.9}}}`}, attempts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var starts []time.Time
				client := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					starts = append(starts, time.Now())
					_, _ = io.WriteString(w, test.replies[min(len(starts), len(test.replies))-1])
				}))
				client.Controller = llm.NewBatchController(4)
				call := questionCall(t, client, "q")
				call.DecodeValidate = func(raw []byte) (map[string]any, error) {
					verdicts, err := client.Verdicts(raw)
					if err != nil || verdicts["q"].Yes == nil {
						return nil, errors.New("question q is unanswered")
					}
					return map[string]any{"q": *verdicts["q"].Yes}, nil
				}
				outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, call)
				if len(starts) != test.attempts || outcome.Metrics.Attempts != test.attempts || (err == nil) != test.accepted {
					t.Fatalf("attempts %d (metrics %d), err %v", len(starts), outcome.Metrics.Attempts, err)
				}
				if test.attempts == 2 && starts[1].Sub(starts[0]) != 2*time.Second {
					t.Fatalf("the second answer was asked after %v, not the short backoff", starts[1].Sub(starts[0]))
				}
				// Every answer is billed, the empty one too.
				if test.accepted && outcome.Metrics.InputTokens != test.input {
					t.Fatalf("usage %+v, want %d input tokens", outcome.Metrics, test.input)
				}
				var source llm.ProviderFailureSource
				if !test.accepted && test.attempts == 2 && (!errors.As(err, &source) || source.ProviderFailure().Kind != llm.ProviderFailureResponse) {
					t.Fatalf("a second empty answer was not refused as a response failure: %v", err)
				}
			})
		})
	}
}

// failingBody is a response body that fails after the status and headers.
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
func (failingBody) Close() error             { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// A 429 whose body fails to arrive is still the rate limit: its Retry-After
// sets the shared cooldown (the review's A8 note: it was lost with the body).
func TestARateLimitWhoseBodyFailsKeepsItsRetryAfter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var starts []time.Time
		client := handlerClient(nil)
		client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			starts = append(starts, time.Now())
			if len(starts) == 1 {
				header := http.Header{"Retry-After": []string{"90"}}
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: header, Body: failingBody{}, Request: request}, nil
			}
			recorder := httptest.NewRecorder()
			answer(recorder, "q")
			return recorder.Result(), nil
		})}
		outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, client, questionCall(t, client, "q"))
		if err != nil || len(starts) != 2 || starts[1].Sub(starts[0]) != 90*time.Second || outcome.Metrics.Attempts != 2 {
			t.Fatalf("starts %v, err %v", starts, err)
		}
	})
}
