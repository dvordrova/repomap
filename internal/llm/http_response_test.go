package llm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type httpResponseTestProvider struct {
	testProvider
	diagnostic *HTTPResponse
}

func (p *httpResponseTestProvider) Complete(ctx context.Context, prepared Prepared) (Completion, error) {
	completion, err := p.testProvider.Complete(ctx, prepared)
	completion.HTTPResponse = p.diagnostic
	return completion, err
}

func TestHTTPResponseFlowsToOutcomeAndEventWithoutChangingCacheOrReplay(t *testing.T) {
	p := &httpResponseTestProvider{
		testProvider: testProvider{state: []byte(`{"provider":"http-diagnostic-test"}`)},
		diagnostic:   &HTTPResponse{StatusCode: 200, Headers: map[string][]string{"X-Request-Id": {"first"}}},
	}
	var events []Event
	executor := Executor{Enabled: true, RootDir: t.TempDir(), Observer: ObserverFunc(func(event Event) error {
		events = append(events, event)
		if event.HTTPResponse != nil {
			event.HTTPResponse.Headers["X-Request-Id"][0] = "observer mutation"
		}
		return nil
	})}
	call := baseTestCall("stage", "same exact request")
	cold, err := ExecuteJSON(t.Context(), executor, p, call)
	if err != nil || cold.Cached || cold.HTTPResponse == nil || cold.HTTPResponse.Headers["X-Request-Id"][0] != "first" ||
		p.diagnostic.Headers["X-Request-Id"][0] != "first" || len(events) != 1 {
		t.Fatalf("HTTP response propagation/ownership: %#v / %#v / %v", cold, events, err)
	}
	p.diagnostic.Headers["X-Request-Id"][0] = "next"
	warm, err := ExecuteJSON(t.Context(), executor, p, call)
	if err != nil || !warm.Cached || warm.HTTPResponse != nil || warm.CacheKey != cold.CacheKey || p.completeCalls != 1 || events[1].HTTPResponse != nil {
		t.Fatalf("HTTP diagnostic changed cache identity or fabricated warm response: %#v / %v", warm, err)
	}
	raw, err := os.ReadFile(filepath.Join(executor.RootDir, CacheDirectoryName, cold.CacheKey+".json"))
	if err != nil || bytes.Contains(raw, []byte("http_response")) || bytes.Contains(raw, []byte("X-Request-Id")) {
		t.Fatalf("HTTP diagnostic entered accepted cache record: %s / %v", raw, err)
	}
	prepared, _ := NewPrepared(cold.Request)
	replayed, err := ReplayJSON(t.Context(), executor, p, prepared)
	if err != nil || replayed.CacheKey != cold.CacheKey || !bytes.Equal(replayed.Request, cold.Request) ||
		replayed.HTTPResponse == nil || replayed.HTTPResponse.Headers["X-Request-Id"][0] != "next" || p.completeCalls != 2 {
		t.Fatalf("replay lost HTTP diagnostic or exact identity: %#v / %v", replayed, err)
	}
	warm, err = ExecuteJSON(t.Context(), executor, p, call)
	if err != nil || !warm.Cached || warm.HTTPResponse != nil || p.completeCalls != 2 {
		t.Fatalf("replay cache reused stale HTTP metadata: %#v / %v", warm, err)
	}
}

func TestHTTPResponseSurvivesProviderFailureAndParallelEventBufferClonesIt(t *testing.T) {
	want := &HTTPResponse{StatusCode: 500, Headers: map[string][]string{"Traceparent": {"trace"}}}
	p := &httpResponseTestProvider{
		testProvider: testProvider{state: []byte(`{"provider":"http-diagnostic-test"}`), errors: []error{errors.New("provider failure")}},
		diagnostic:   want.Clone(),
	}
	var event Event
	executor := Executor{Observer: ObserverFunc(func(got Event) error { event = got; return nil })}
	outcome, err := ExecuteJSON(t.Context(), executor, p, baseTestCall("stage", "failure"))
	if err == nil || !reflect.DeepEqual(outcome.HTTPResponse, want) || !reflect.DeepEqual(event.HTTPResponse, want) || event.Failure != FailureProvider {
		t.Fatalf("failure lost status/trace: %#v / %#v / %v", outcome, event, err)
	}
	buffer := &batchEventBuffer{}
	if err := buffer.Observe(event); err != nil {
		t.Fatal(err)
	}
	event.HTTPResponse.Headers["Traceparent"][0] = "mutated"
	if !reflect.DeepEqual(buffer.events[0].HTTPResponse, want) || !reflect.DeepEqual(outcome.HTTPResponse, want) {
		t.Fatal("parallel buffer or outcome aliases caller-owned HTTP headers")
	}
}
