package llm

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dvordrova/repomap/internal/testbudget"
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
	wire, err := json.Marshal(item)
	if err != nil {
		return Call[testValue]{}, err
	}
	return Call[testValue]{
		State:  []byte(`{"item":"` + strings.Join(item, ",") + `"}`),
		Prompt: Prompt{User: string(wire), ResponseFormatJSON: true},
		Limits: Limits{
			MaxRequestBytes: 1024, MaxResponseBytes: ProviderResponseByteLimit,
			MaxOutputTokens: 128_000,
		},
	}, nil
}

// adaptiveEachBuildWith is adaptiveEachBuild with one adjustment per call.
func adaptiveEachBuildWith(adjust func(*Call[testValue])) func([]string) (Call[testValue], error) {
	return func(item []string) (Call[testValue], error) {
		call, err := adaptiveEachBuild(item)
		if err == nil {
			adjust(&call)
		}
		return call, err
	}
}

func adaptiveBatchTestSplit(item []string) ([]string, []string, bool) {
	if len(item) <= 1 {
		return nil, nil, false
	}
	middle := len(item) / 2
	return append([]string(nil), item[:middle]...), append([]string(nil), item[middle:]...), true
}

// adaptiveCover runs the adaptive executor and reads its leaves as one
// complete cover: the items and outcomes in order, or the first terminal
// leaf's error and no cover at all.
func adaptiveCover(ctx context.Context, executor Executor, provider Provider, items [][]string,
	build func([]string) (Call[testValue], error), split func([]string) ([]string, []string, bool),
) ([][]string, []Outcome[testValue], error) {
	results, err := ExecuteAdaptiveJSONEachResults(ctx, executor, provider, items, build, split)
	if err != nil {
		return nil, nil, err
	}
	plan := make([][]string, len(results))
	outcomes := make([]Outcome[testValue], len(results))
	for i, result := range results {
		if result.Err != nil {
			return nil, nil, result.Err
		}
		plan[i], outcomes[i] = result.Item, result.Outcome
	}
	return plan, outcomes, nil
}

type preflightEachProvider struct{ adaptiveEachProvider }

type completionEnvelopeEachProvider struct {
	adaptiveEachProvider
	kind ResourceLimitKind
}

func (p *completionEnvelopeEachProvider) Complete(ctx context.Context, prepared Prepared) (Completion, error) {
	completion, err := p.adaptiveEachProvider.Complete(ctx, prepared)
	var values []string
	_ = json.Unmarshal(prepared.Bytes(), &values)
	if len(values) > 1 {
		// Transport succeeded; the shared executor owns this envelope refusal.
		completion = Completion{Response: []byte(`{"value":"partial"}`), FinishReason: FinishLength, ChoiceCount: 1, Metrics: Metrics{Attempts: 1}}
		if p.kind == ResourceLimitResponseBytes {
			completion.FinishReason = FinishStop
			completion.Response = []byte(strings.Repeat(" ", 513))
		}
		return completion, nil
	}
	return completion, err
}

func TestAdaptiveOwnersSplitExecutorCompletionEnvelopesAndReuseLeaves(t *testing.T) {
	for _, kind := range []ResourceLimitKind{ResourceLimitOutputTokens, ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			p := &completionEnvelopeEachProvider{kind: kind}
			executor := Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 1}
			build := adaptiveEachBuildWith(func(call *Call[testValue]) { call.Limits.MaxResponseBytes = 512 })
			for range 2 {
				items := [][]string{{"a", "b"}, {"sibling"}}
				_, outcomes, err := adaptiveCover(t.Context(), executor, p, items, build, adaptiveBatchTestSplit)
				if err != nil || len(outcomes) != 3 {
					t.Fatalf("complete leaves unavailable: %+v / %v", outcomes, err)
				}
				for i, want := range []string{"a", "b", "sibling"} {
					if outcomes[i].Value.Value != want {
						t.Fatalf("leaf %d: %q, want %q", i, outcomes[i].Value.Value, want)
					}
				}
			}
			want := map[string]int{"a,b": 1, "a": 1, "b": 1, "sibling": 1}
			if !reflect.DeepEqual(p.requests, want) {
				t.Fatalf("exact failed parent or accepted sibling was repeated: %v", p.requests)
			}
		})
	}
}

func (p *preflightEachProvider) Prepare(prompt Prompt, limits Limits) (Prepared, error) {
	var values []string
	if err := json.Unmarshal([]byte(prompt.User), &values); err != nil {
		return Prepared{}, err
	}
	if len(values) > 1 {
		return Prepared{}, NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitContextTokens, Limit: 1, Observed: len(values), ObservedKnown: true})
	}
	return p.adaptiveEachProvider.Prepare(prompt, limits)
}

func TestAdaptiveOwnersPartitionKnownEnvelopeBeforeAnyTransport(t *testing.T) {
	p := &preflightEachProvider{}
	items := [][]string{{"a", "b", "c", "d"}, {"sibling"}}
	if _, _, err := adaptiveCover(t.Context(), Executor{BatchConcurrency: 2}, p, items, adaptiveEachBuild, adaptiveBatchTestSplit); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"a": 1, "b": 1, "c": 1, "d": 1, "sibling": 1}
	if !reflect.DeepEqual(p.requests, want) {
		t.Fatalf("transport calls %v, want complete leaf cover %v", p.requests, want)
	}
}

func TestAdaptiveEachKeepsRunningSiblingAndSplitsEveryFailureWithoutCache(t *testing.T) {
	p := &adaptiveEachProvider{started: make(chan struct{}), failed: make(chan struct{}, 2), release: make(chan struct{}), canceled: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), testbudget.Until(t))
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
		plan, out, err := adaptiveCover(ctx, executor, p, [][]string{{"a", "b"}, {"slow"}, {"c", "d"}},
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
		plan, outcomes, err := adaptiveCover(t.Context(), executor, p, items, adaptiveEachBuild, adaptiveBatchTestSplit)
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
	plan, outcomes, err := adaptiveCover(t.Context(), executor, p, items, adaptiveEachBuild, adaptiveBatchTestSplit)
	if err != nil || len(plan) != 2 || len(outcomes) != 2 || outcomes[0].Value.Value != "replayed parent" || !outcomes[0].Cached {
		t.Fatalf("parent did not supersede split: %v / %v / %v", plan, outcomes, err)
	}
}

type adaptiveEachReplayProvider struct{ *adaptiveEachProvider }

func (*adaptiveEachReplayProvider) Complete(context.Context, Prepared) (Completion, error) {
	return Completion{Response: []byte(`{"value":"replayed parent"}`), FinishReason: FinishStop, ChoiceCount: 1, Metrics: Metrics{Attempts: 1}}, nil
}

// A terminal leaf is handed back to its owner beside the completed cover
// while the other items keep splitting; the owner decides what it means.
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
