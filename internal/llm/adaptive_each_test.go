package llm

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type adaptiveEachProvider struct {
	mu       sync.Mutex
	requests map[string]int
	started  chan struct{}
	failed   chan struct{}
	release  chan struct{}
	canceled chan struct{}
	atomic   bool
}

func (*adaptiveEachProvider) State() []byte { return []byte(`{"provider":"adaptive-each"}`) }
func (*adaptiveEachProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	return NewPrepared([]byte(prompt.User))
}
func (p *adaptiveEachProvider) Complete(ctx context.Context, prepared Prepared) (Completion, error) {
	var values []string
	if err := json.Unmarshal(prepared.Bytes(), &values); err != nil {
		return Completion{}, err
	}
	key := strings.Join(values, ",")
	p.mu.Lock()
	if p.requests == nil {
		p.requests = make(map[string]int)
	}
	p.requests[key]++
	p.mu.Unlock()
	if key == "slow" && p.started != nil {
		close(p.started)
		select {
		case <-ctx.Done():
			close(p.canceled)
			return Completion{}, ctx.Err()
		case <-p.release:
		}
	}
	if len(values) > 1 || p.atomic && key == "bad" {
		if p.started != nil {
			<-p.started
			p.failed <- struct{}{}
		}
		return Completion{}, NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitContextTokens, Limit: 1, Observed: len(values), ObservedKnown: true})
	}
	raw, _ := json.Marshal(testValue{Value: key})
	return Completion{Response: raw, FinishReason: FinishStop, ChoiceCount: 1, Metrics: Metrics{Attempts: 1}}, nil
}

func adaptiveEachBuild(item []string) (Call[testValue], error) {
	calls, err := adaptiveBatchTestBuild([][]string{item})
	if err != nil {
		return Call[testValue]{}, err
	}
	return calls[0], nil
}

func TestAdaptiveEachKeepsRunningSiblingAndSplitsEveryFailureWithoutCache(t *testing.T) {
	p := &adaptiveEachProvider{started: make(chan struct{}), failed: make(chan struct{}, 2), release: make(chan struct{}), canceled: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var notices []int
	built := make(map[string]int)
	var events []Event
	executor := Executor{BatchConcurrency: 3, Enabled: false,
		PlanNotice: func(count int) { notices = append(notices, count) },
		Observer:   ObserverFunc(func(event Event) error { events = append(events, event); return nil }),
	}
	type result struct {
		plan [][]string
		out  []Outcome[testValue]
		err  error
	}
	done := make(chan result, 1)
	go func() {
		plan, out, err := ExecuteAdaptiveJSONEach(ctx, executor, p, [][]string{{"a", "b"}, {"slow"}, {"c", "d"}},
			func(item []string) (Call[testValue], error) {
				built[strings.Join(item, ",")]++
				return adaptiveEachBuild(item)
			}, adaptiveBatchTestSplit)
		done <- result{plan, out, err}
	}()
	for range 2 {
		select {
		case <-p.failed:
		case <-ctx.Done():
			t.Fatal("independent failures did not finish")
		}
	}
	select {
	case <-p.canceled:
		t.Fatal("splittable failure canceled the running sibling")
	case <-time.After(30 * time.Millisecond):
	}
	close(p.release)
	var got result
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("adaptive execution did not complete")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !reflect.DeepEqual(notices, []int{3, 4}) {
		t.Fatalf("pending rounds = %v, want 3 then all four children", notices)
	}
	want := []string{"a", "b", "slow", "c", "d"}
	if len(got.out) != len(want) || len(got.plan) != len(want) {
		t.Fatalf("incomplete cover: %v / %v", got.plan, got.out)
	}
	for i, value := range want {
		if got.out[i].Value.Value != value || got.out[i].Cached || len(got.plan[i]) != 1 || got.plan[i][0] != value {
			t.Fatalf("item %d: %v / %+v", i, got.plan[i], got.out[i])
		}
	}
	if len(p.requests) != 7 || len(built) != 7 {
		t.Fatalf("requests/builds = %v / %v", p.requests, built)
	}
	for key, count := range p.requests {
		if count != 1 || built[key] != 1 {
			t.Fatalf("unchanged request rebuilt or repeated: %s called %d, built %d", key, count, built[key])
		}
	}
	slowSuccesses := 0
	for _, event := range events {
		if event.Kind == EventLive && string(event.Request) == `["slow"]` {
			slowSuccesses++
		}
	}
	if slowSuccesses != 1 {
		t.Fatalf("slow sibling emitted %d success events", slowSuccesses)
	}
}

func TestAdaptiveEachWarmCacheKeepsSplitMemosAndWholeParentReplayPrecedence(t *testing.T) {
	p := &adaptiveEachProvider{}
	executor := Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 2}
	items := [][]string{{"a", "b"}, {"slow"}}
	for range 2 {
		plan, outcomes, err := ExecuteAdaptiveJSONEach(t.Context(), executor, p, items, adaptiveEachBuild, adaptiveBatchTestSplit)
		if err != nil || len(plan) != 3 || len(outcomes) != 3 {
			t.Fatalf("split cover: %v / %v / %v", plan, outcomes, err)
		}
	}
	for key, count := range p.requests {
		if count != 1 {
			t.Fatalf("warm run repeated %s %d times", key, count)
		}
	}
	// An accepted parent in the same exact-request cache supersedes the hint.
	call, _ := adaptiveEachBuild(items[0])
	replay := &adaptiveEachReplayProvider{adaptiveEachProvider: p}
	if _, err := ExecuteJSON(t.Context(), executor, replay, call); err != nil {
		t.Fatal(err)
	}
	plan, outcomes, err := ExecuteAdaptiveJSONEach(t.Context(), executor, p, items, adaptiveEachBuild, adaptiveBatchTestSplit)
	if err != nil || len(plan) != 2 || len(outcomes) != 2 || outcomes[0].Value.Value != "replayed parent" || !outcomes[0].Cached {
		t.Fatalf("parent did not supersede split: %v / %v / %v", plan, outcomes, err)
	}
}

type adaptiveEachReplayProvider struct{ *adaptiveEachProvider }

func (*adaptiveEachReplayProvider) Complete(context.Context, Prepared) (Completion, error) {
	return Completion{Response: []byte(`{"value":"replayed parent"}`), FinishReason: FinishStop, ChoiceCount: 1, Metrics: Metrics{Attempts: 1}}, nil
}

// The results variant hands a terminal leaf back to its owner beside the
// completed cover and keeps splitting the other items; the plain variant
// keeps failing closed (the test below).
func TestAdaptiveEachResultsKeepTerminalLeafBesideCompletedCover(t *testing.T) {
	p := &adaptiveEachProvider{atomic: true}
	results, err := ExecuteAdaptiveJSONEachResults(t.Context(), Executor{BatchConcurrency: 2}, p,
		[][]string{{"bad"}, {"a", "b"}, {"good"}}, adaptiveEachBuild, adaptiveBatchTestSplit)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bad", "a", "b", "good"}
	if len(results) != len(want) {
		t.Fatalf("leaves: %+v", results)
	}
	for i, result := range results {
		if len(result.Item) != 1 || result.Item[0] != want[i] {
			t.Fatalf("leaf %d covers %v, want %s", i, result.Item, want[i])
		}
		if want[i] == "bad" {
			var resourceErr *ResourceLimitError
			if !errors.As(result.Err, &resourceErr) || len(result.Outcome.ResponseRejections) == 0 || result.Outcome.RequestBytes == 0 {
				t.Fatalf("terminal leaf lost its refusal or outcome: %+v", result)
			}
			continue
		}
		if result.Err != nil || result.Outcome.Value.Value != want[i] {
			t.Fatalf("completed leaf %s: %+v", want[i], result)
		}
	}
	for _, key := range []string{"bad", "a,b", "a", "b", "good"} {
		if p.requests[key] != 1 {
			t.Fatalf("request %q made %d times: %v", key, p.requests[key], p.requests)
		}
	}
}

func TestAdaptiveEachTerminalFailureWaitsForSiblingsWithoutPartialCover(t *testing.T) {
	p := &adaptiveEachProvider{atomic: true}
	plan, outcomes, err := ExecuteAdaptiveJSONEach(t.Context(), Executor{BatchConcurrency: 2}, p,
		[][]string{{"bad"}, {"good"}}, adaptiveEachBuild, adaptiveBatchTestSplit)
	var itemErr *BatchItemError
	if !errors.As(err, &itemErr) || itemErr.Index != 0 || plan != nil || outcomes != nil || p.requests["good"] != 1 {
		t.Fatalf("terminal result: %v / %v / %v, calls %v", plan, outcomes, err, p.requests)
	}
}
