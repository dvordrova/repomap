package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// drawProvider answers like a model: each call is a new draw, "answer N".
// Every preparation and call reports itself; a call then waits for release.
type drawProvider struct {
	mu       sync.Mutex
	calls    int
	prepared chan struct{}
	arrived  chan int
	release  chan struct{}
	respond  func(n int) ([]byte, error)
}

func newDrawProvider(respond func(n int) ([]byte, error)) *drawProvider {
	return &drawProvider{prepared: make(chan struct{}, 8), arrived: make(chan int, 8), release: make(chan struct{}), respond: respond}
}

func (*drawProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"draws"}`)
}

func (provider *drawProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	provider.prepared <- struct{}{}
	raw, err := json.Marshal(map[string]string{"system": prompt.System, "user": prompt.User})
	if err != nil {
		return Prepared{}, err
	}
	return NewPrepared(raw)
}

func (provider *drawProvider) Complete(ctx context.Context, _ Prepared) (Completion, error) {
	provider.mu.Lock()
	provider.calls++
	n := provider.calls
	provider.mu.Unlock()
	provider.arrived <- n
	select {
	case <-provider.release:
	case <-ctx.Done():
		return Completion{}, ctx.Err()
	}
	response, err := provider.respond(n)
	return Completion{
		Response: response, FinishReason: FinishStop, ChoiceCount: 1, Metrics: Metrics{Attempts: 1, Latency: time.Millisecond},
		HTTPResponse: &HTTPResponse{StatusCode: 200, Headers: map[string][]string{"X-Trace": {fmt.Sprint(n)}}},
	}, err
}

func (provider *drawProvider) count() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

// requireNoArrival waits until the identical request has been prepared,
// which it is just before it joins the first one's flight, and then fails
// when it reaches the provider soon after.
func (provider *drawProvider) requireNoArrival(t *testing.T) {
	t.Helper()
	<-provider.prepared
	select {
	case n := <-provider.arrived:
		t.Fatalf("call %d of an identical request reached the provider while the first was in the air", n)
	case <-time.After(150 * time.Millisecond):
	}
}

type flightResult struct {
	outcome Outcome[testValue]
	err     error
}

func goExecute(ctx context.Context, executor Executor, provider Provider, call Call[testValue]) <-chan flightResult {
	done := make(chan flightResult, 1)
	go func() {
		outcome, err := ExecuteJSON(ctx, executor, provider, call)
		done <- flightResult{outcome, err}
	}()
	return done
}

type kindCounter struct {
	mu    sync.Mutex
	kinds map[EventKind]int
}

func (counter *kindCounter) Observe(event Event) error {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	counter.kinds[event.Kind]++
	return nil
}

func draw(n int) ([]byte, error) { return fmt.Appendf(nil, `{"value":"answer %d"}`, n), nil }

// An identical request asked while the first is in the air waits for that
// one answer: one provider call, one answer for both, and the cache keeps
// the answer both callers used.
func TestIdenticalRequestsInTheAirMakeOneCall(t *testing.T) {
	provider := newDrawProvider(draw)
	events := &kindCounter{kinds: map[EventKind]int{}}
	executor := Executor{RootDir: t.TempDir(), Enabled: true, BatchController: &BatchController{}, Observer: events}
	call := baseTestCall(`{"cube":"flight"}`, "same")
	first := goExecute(t.Context(), executor, provider, call)
	<-provider.prepared
	<-provider.arrived
	second := goExecute(t.Context(), executor, provider, call)
	provider.requireNoArrival(t)
	close(provider.release)
	leader, follower := <-first, <-second
	if leader.err != nil || follower.err != nil {
		t.Fatalf("errors: %v, %v", leader.err, follower.err)
	}
	if leader.outcome.Value.Value != "answer 1" || follower.outcome.Value != leader.outcome.Value {
		t.Fatalf("answers %q and %q, want the one answer", leader.outcome.Value.Value, follower.outcome.Value.Value)
	}
	if leader.outcome.Cached || !follower.outcome.Cached || provider.count() != 1 {
		t.Fatalf("leader cached %v, follower cached %v, provider calls %d", leader.outcome.Cached, follower.outcome.Cached, provider.count())
	}
	// The HTTP diagnostics belong to the one exchange that made the call.
	if leader.outcome.HTTPResponse == nil || follower.outcome.HTTPResponse != nil {
		t.Fatalf("HTTP responses: leader %+v, follower %+v", leader.outcome.HTTPResponse, follower.outcome.HTTPResponse)
	}
	if events.kinds[EventLive] != 1 || events.kinds[EventCacheHit] != 1 {
		t.Fatalf("events %v, want one live call and one reuse", events.kinds)
	}
	executor.BatchController = &BatchController{}
	next, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || !next.Cached || next.Value.Value != "answer 1" {
		t.Fatalf("next run: %+v %v", next.Value, err)
	}
}

// A refused answer is the answer of every identical request in the air: the
// follower decides on the same bytes with its own decoder, asks nothing and,
// like the leader, caches nothing.
func TestIdenticalRequestsShareARefusal(t *testing.T) {
	provider := newDrawProvider(func(int) ([]byte, error) { return []byte(`{"value":""}`), nil })
	executor := Executor{RootDir: t.TempDir(), Enabled: true, BatchController: &BatchController{}}
	call := baseTestCall(`{"cube":"flight"}`, "same")
	first := goExecute(t.Context(), executor, provider, call)
	<-provider.prepared
	<-provider.arrived
	second := goExecute(t.Context(), executor, provider, call)
	provider.requireNoArrival(t)
	close(provider.release)
	leader, follower := <-first, <-second
	if leader.err == nil || follower.err == nil || provider.count() != 1 {
		t.Fatalf("errors %v, %v after %d calls; want one refused call", leader.err, follower.err, provider.count())
	}
	if len(follower.outcome.ResponseRejections) != 1 || follower.outcome.ResponseRejections[0].Kind != adaptiveResponseRejected {
		t.Fatalf("follower rejections: %+v", follower.outcome.ResponseRejections)
	}
	provider.respond = draw
	if next, err := ExecuteJSON(t.Context(), executor, provider, call); err != nil || next.Cached || next.Value.Value != "answer 2" {
		t.Fatalf("a refused answer was reused: %+v %v", next.Value, err)
	}
}

// A provider failure is shared as the same error, so an adaptive owner
// splits every identical request alike.
func TestIdenticalRequestsShareAProviderFailure(t *testing.T) {
	failure := errors.New("upstream closed")
	provider := newDrawProvider(func(int) ([]byte, error) { return nil, failure })
	executor := Executor{RootDir: t.TempDir(), Enabled: true, BatchController: &BatchController{}}
	call := baseTestCall(`{"cube":"flight"}`, "same")
	first := goExecute(t.Context(), executor, provider, call)
	<-provider.prepared
	<-provider.arrived
	second := goExecute(t.Context(), executor, provider, call)
	provider.requireNoArrival(t)
	close(provider.release)
	leader, follower := <-first, <-second
	if !errors.Is(leader.err, failure) || follower.err != leader.err || provider.count() != 1 {
		t.Fatalf("errors %v, %v after %d calls", leader.err, follower.err, provider.count())
	}
}

// A leader stopped by its own context leaves no answer; its follower asks
// the provider itself instead of taking that cancellation.
func TestFollowerOfACanceledLeaderAsksItself(t *testing.T) {
	provider := newDrawProvider(draw)
	executor := Executor{RootDir: t.TempDir(), Enabled: true, BatchController: &BatchController{}}
	call := baseTestCall(`{"cube":"flight"}`, "same")
	leaderCtx, cancel := context.WithCancel(t.Context())
	first := goExecute(leaderCtx, executor, provider, call)
	<-provider.prepared
	<-provider.arrived
	second := goExecute(t.Context(), executor, provider, call)
	provider.requireNoArrival(t)
	cancel()
	if leader := <-first; !errors.Is(leader.err, context.Canceled) {
		t.Fatalf("leader: %v", leader.err)
	}
	if n := <-provider.arrived; n != 2 {
		t.Fatalf("the follower's own call was %d", n)
	}
	close(provider.release)
	follower := <-second
	if follower.err != nil || follower.outcome.Cached || follower.outcome.Value.Value != "answer 2" {
		t.Fatalf("follower: %+v %v", follower.outcome.Value, follower.err)
	}
}
