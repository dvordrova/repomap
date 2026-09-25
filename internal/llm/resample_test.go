package llm

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func resampledTestCall() Call[testValue] {
	call := baseTestCall("cube-v1", "same-input")
	call.Resample = true
	return call
}

// refusedFirstDraws are the whole refusals a second draw can fix. Each sets
// up the provider's first answer; the second answer is always valid.
var refusedFirstDraws = map[string]func(*testProvider){
	"undecodable JSON": func(provider *testProvider) { provider.responses[0] = []byte(`{"value":`) },
	"validator":        func(provider *testProvider) { provider.responses[0] = []byte(`{"value":""}`) },
	"output cap": func(provider *testProvider) {
		provider.finishReasons = []FinishReason{FinishLength, FinishStop}
	},
	"output-token refusal": func(provider *testProvider) {
		provider.errors = []error{NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitOutputTokens, FinishReason: "length"}), nil}
	},
	"empty answer": func(provider *testProvider) {
		provider.responses[0] = nil
		provider.errors = []error{&classifiedTestProviderError{failure: ProviderFailure{Kind: ProviderFailureResponse}, cause: errors.New("empty content")}, nil}
	},
}

// A whole refusal is asked again with the same bytes; the accepted second
// draw is the answer, it is cached, and the outcome carries both draws.
func TestResampleAcceptsASecondDrawAfterAWholeRefusal(t *testing.T) {
	for name, refuse := range refusedFirstDraws {
		t.Run(name, func(t *testing.T) {
			provider := baseTestProvider()
			provider.responses = [][]byte{[]byte(`{"value":"ok"}`), []byte(`{"value":"ok"}`)}
			refuse(provider)
			var events []Event
			executor := Executor{RootDir: t.TempDir(), Enabled: true, Observer: ObserverFunc(func(event Event) error {
				events = append(events, event)
				return nil
			})}
			outcome, err := ExecuteJSON(t.Context(), executor, provider, resampledTestCall())
			if err != nil || outcome.Value.Value != "ok" || outcome.Cached || provider.completeCalls != 2 {
				t.Fatalf("outcome = %#v, calls = %d, err = %v", outcome, provider.completeCalls, err)
			}
			if outcome.Metrics.InputTokens != 22 || outcome.Metrics.OutputTokens != 14 ||
				outcome.Metrics.Attempts != 2 || outcome.Metrics.Latency != 10*time.Millisecond {
				t.Fatalf("metrics do not carry both draws: %#v", outcome.Metrics)
			}
			if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{EventFailure, EventLive}) ||
				events[0].RequestSHA256 != events[1].RequestSHA256 || !reflect.DeepEqual(provider.completeOrder, []string{"same-input", "same-input"}) {
				t.Fatalf("events = %v, order = %v", got, provider.completeOrder)
			}
			warm, err := ExecuteJSON(t.Context(), executor, provider, resampledTestCall())
			if err != nil || !warm.Cached || warm.CacheKey != outcome.CacheKey || provider.completeCalls != 2 || warm.Metrics.InputTokens != 11 {
				t.Fatalf("second draw not cached under the same key: %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
			}
		})
	}
}

// Two refused draws are one refusal after exactly two provider calls, and
// neither answer reaches the cache.
func TestResampleRefusesAfterTwoRefusedDraws(t *testing.T) {
	provider := baseTestProvider()
	provider.responses = [][]byte{[]byte(`{"value":""}`)}
	root := t.TempDir()
	var events []Event
	executor := Executor{RootDir: root, Enabled: true, Observer: ObserverFunc(func(event Event) error {
		events = append(events, event)
		return nil
	})}
	outcome, err := ExecuteJSON(t.Context(), executor, provider, resampledTestCall())
	if err == nil || provider.completeCalls != 2 || outcome.Metrics.Attempts != 2 || outcome.Metrics.InputTokens != 22 {
		t.Fatalf("outcome = %#v, calls = %d, err = %v", outcome, provider.completeCalls, err)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{EventFailure, EventFailure}) {
		t.Fatalf("events = %v", got)
	}
	if _, err := os.Lstat(filepath.Join(root, cacheDirectoryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused draw populated the cache: %v", err)
	}
}

// Failures a second identical draw cannot fix, and refusals the owner
// recovers from itself, keep their one provider call.
func TestResampleLeavesTransportRepeatableAndRecoveredFailures(t *testing.T) {
	providerFailure := func(failure ProviderFailure) func(*testProvider, *Call[testValue]) {
		return func(provider *testProvider, _ *Call[testValue]) {
			provider.errors = []error{&classifiedTestProviderError{failure: failure, cause: errors.New("provider failure")}, nil}
		}
	}
	invalid := func(provider *testProvider) {
		provider.responses = [][]byte{[]byte(`{"value":""}`), []byte(`{"value":"ok"}`)}
	}
	cases := map[string]func(*testProvider, *Call[testValue]){
		"network retries exhausted": providerFailure(ProviderFailure{Kind: ProviderFailureNetwork, Attempts: 4, RetryExhausted: true}),
		"timeout":                   providerFailure(ProviderFailure{Kind: ProviderFailureTimeout, Attempts: 4, RetryExhausted: true}),
		"HTTP 500":                  providerFailure(ProviderFailure{Kind: ProviderFailureHTTPStatus, HTTPStatus: 500, Attempts: 4, RetryExhausted: true}),
		"HTTP 400":                  providerFailure(ProviderFailure{Kind: ProviderFailureHTTPStatus, HTTPStatus: 400, Attempts: 1}),
		"context limit": func(provider *testProvider, _ *Call[testValue]) {
			provider.errors = []error{NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitContextTokens, HTTPStatus: 400}), nil}
		},
		"request size": func(provider *testProvider, _ *Call[testValue]) {
			provider.errors = []error{NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitRequestBytes}), nil}
		},
		"content filter": func(provider *testProvider, _ *Call[testValue]) {
			provider.finishReasons = []FinishReason{FinishContentFilter, FinishStop}
			providerFailure(ProviderFailure{Kind: ProviderFailureResponse})(provider, nil)
		},
		"not opted in": func(provider *testProvider, call *Call[testValue]) {
			invalid(provider)
			call.Resample = false
		},
		"owner splits refused answers": func(provider *testProvider, call *Call[testValue]) {
			invalid(provider)
			call.SplitRejectedResponse = true
		},
		"owner splits HTTP 500": func(provider *testProvider, call *Call[testValue]) {
			invalid(provider)
			call.SplitHTTP500 = true
		},
		"owner bounds the attempt time": func(provider *testProvider, call *Call[testValue]) {
			invalid(provider)
			call.Limits.AttemptTimeout = time.Minute
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			provider := baseTestProvider()
			call := resampledTestCall()
			setup(provider, &call)
			if _, err := ExecuteJSON(t.Context(), Executor{RootDir: t.TempDir(), Enabled: true}, provider, call); err == nil || provider.completeCalls != 1 {
				t.Fatalf("calls = %d, err = %v", provider.completeCalls, err)
			}
		})
	}
}

// A cut at the output-token cap is a looping answer, not an oversized
// request: even an item its adaptive owner could split is asked once more
// whole, and the accepted second draw leaves it unsplit.
func TestResampleAsksAnOutputCapCutAgainBeforeAnAdaptiveSplit(t *testing.T) {
	build := func(user string) (Call[testValue], error) {
		call := resampledTestCall()
		call.Prompt.User = user
		return call, nil
	}
	split := func(user string) (string, string, bool) { return user + "-left", user + "-right", user == "whole" }
	executors := map[string]func(*testProvider) error{
		"each": func(provider *testProvider) error {
			results, err := ExecuteAdaptiveJSONEachResults(t.Context(), Executor{}, provider, []string{"whole"}, build, split)
			for _, result := range results {
				err = errors.Join(err, result.Err)
			}
			return err
		},
		"batch": func(provider *testProvider) error {
			buildAll := func(users []string) ([]Call[testValue], error) {
				calls := make([]Call[testValue], len(users))
				for i, user := range users {
					calls[i], _ = build(user)
				}
				return calls, nil
			}
			_, _, err := ExecuteAdaptiveJSONBatch(t.Context(), Executor{}, provider, []string{"whole"}, buildAll, split)
			return err
		},
	}
	for name, execute := range executors {
		provider := baseTestProvider()
		provider.errors = []error{NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitOutputTokens, FinishReason: "length"}), nil}
		if err := execute(provider); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := []string{"whole", "whole"}; !reflect.DeepEqual(provider.completeOrder, want) {
			t.Fatalf("%s: provider saw %v, want %v", name, provider.completeOrder, want)
		}
	}
}

// A cached answer is not a draw: an accepted one is served without a call,
// and one the current validator refuses is replaced by exactly one live call.
func TestResampleNeverRedrawsACachedAnswer(t *testing.T) {
	provider := baseTestProvider()
	provider.responses = [][]byte{[]byte(`{"value":"old"}`), []byte(`{"value":"new"}`)}
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	if _, err := ExecuteJSON(t.Context(), executor, provider, resampledTestCall()); err != nil {
		t.Fatal(err)
	}
	warm, err := ExecuteJSON(t.Context(), executor, provider, resampledTestCall())
	if err != nil || !warm.Cached || provider.completeCalls != 1 {
		t.Fatalf("warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
	}
	strict := resampledTestCall()
	strict.Validate = func(value testValue) error {
		if value.Value != "new" {
			return errors.New("stale value")
		}
		return nil
	}
	replaced, err := ExecuteJSON(t.Context(), executor, provider, strict)
	if err != nil || replaced.Cached || replaced.Value.Value != "new" || provider.completeCalls != 2 || replaced.Metrics.Attempts != 1 {
		t.Fatalf("replaced = %#v, calls = %d, err = %v", replaced, provider.completeCalls, err)
	}
}

// Replay always makes exactly one live call, whatever it receives.
func TestReplayIsNeverResampled(t *testing.T) {
	provider := baseTestProvider()
	provider.responses = [][]byte{[]byte(`{"value":`), []byte(`{"value":"ok"}`)}
	prepared, err := NewPrepared([]byte(`{"user":"same-input"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplayJSON(t.Context(), Executor{RootDir: t.TempDir(), Enabled: true}, provider, prepared); err == nil || provider.completeCalls != 1 {
		t.Fatalf("replay calls = %d, err = %v", provider.completeCalls, err)
	}
}
