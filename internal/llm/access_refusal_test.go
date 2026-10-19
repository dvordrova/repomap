package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type refusingProviderError struct{ status int }

func (err *refusingProviderError) Error() string { return fmt.Sprintf("HTTP %d", err.status) }
func (err *refusingProviderError) ProviderFailure() ProviderFailure {
	return ProviderFailure{Kind: ProviderFailureHTTPStatus, HTTPStatus: err.status}
}

// refusingProvider answers every call with one HTTP status and counts them.
type refusingProvider struct {
	status int
	mu     sync.Mutex
	calls  int
}

func (p *refusingProvider) State() []byte { return []byte(`{"provider":"refusing"}`) }
func (p *refusingProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	return NewPrepared([]byte(prompt.User))
}
func (p *refusingProvider) Complete(context.Context, Prepared) (Completion, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return Completion{HTTPResponse: &HTTPResponse{StatusCode: p.status, Provider: "Test"}}, &refusingProviderError{status: p.status}
}

// A refused credential or balance stops the run from the executing call:
// with one worker the first refusal is the only provider call, the queued
// calls never start, and no observer or journal is needed for that.
func TestAnAccessRefusalStopsTheQueueFromTheExecutingCall(t *testing.T) {
	for _, status := range []int{401, 402, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			provider := &refusingProvider{status: status}
			ctx, stop := context.WithCancelCause(context.Background())
			defer stop(nil)
			var refusals []ProviderFailure
			var providers []string
			executor := Executor{BatchConcurrency: 1, BatchController: &BatchController{}, AccessRefused: func(failure ProviderFailure, response *HTTPResponse) {
				refusals = append(refusals, failure)
				providers = append(providers, response.Provider)
				stop(errors.New("the run stopped for the refusal"))
			}}
			calls := make([]Call[eachValue], 6)
			for i := range calls {
				calls[i] = eachCall(fmt.Sprintf("window %d", i))
			}
			results := ExecuteJSONEach(ctx, executor, provider, calls)
			if provider.calls != 1 {
				t.Fatalf("the provider was called %d times after the refusal", provider.calls)
			}
			if len(refusals) != 1 || refusals[0].HTTPStatus != status || providers[0] != "Test" {
				t.Fatalf("refusals %+v from %v", refusals, providers)
			}
			if results[0].Err == nil {
				t.Fatal("the refused call reported no error")
			}
			for i, result := range results[1:] {
				if !errors.Is(result.Err, context.Canceled) {
					t.Fatalf("queued call %d did not see the stop: %v", i+1, result.Err)
				}
			}
			if cause := context.Cause(ctx); cause == nil || cause.Error() != "the run stopped for the refusal" {
				t.Fatalf("cause %v", cause)
			}
		})
	}
}

// Other provider failures are one window's own; the hook stays silent and
// the neighbours run.
func TestOtherProviderFailuresDoNotStopTheQueue(t *testing.T) {
	provider := &refusingProvider{status: 500}
	fired := false
	executor := Executor{BatchConcurrency: 1, BatchController: &BatchController{}, AccessRefused: func(ProviderFailure, *HTTPResponse) { fired = true }}
	ExecuteJSONEach(context.Background(), executor, provider, []Call[eachValue]{eachCall("one"), eachCall("two")})
	if fired || provider.calls != 2 {
		t.Fatalf("a 500 fired the access refusal (%v) or stopped the queue (%d calls)", fired, provider.calls)
	}
}
