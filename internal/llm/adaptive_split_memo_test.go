package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type adaptiveSplitTestProvider struct {
	state    []byte
	kind     ResourceLimitKind
	calls    [][]string
	prepared int
	response func([]string) ([]byte, error, bool)
}

func (p *adaptiveSplitTestProvider) State() []byte {
	if p.state != nil {
		return p.state
	}
	return []byte(`{"provider":"adaptive-split-test","model":"one"}`)
}

func (p *adaptiveSplitTestProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	p.prepared++
	return NewPrepared([]byte(prompt.User))
}

func (p *adaptiveSplitTestProvider) Complete(_ context.Context, request Prepared) (Completion, error) {
	var values []string
	if err := json.Unmarshal(request.Bytes(), &values); err != nil {
		return Completion{}, err
	}
	p.calls = append(p.calls, values)
	finish := func(raw []byte, err error) (Completion, error) {
		return Completion{Response: raw, FinishReason: FinishStop, ChoiceCount: 1, Metrics: Metrics{Attempts: 1}}, err
	}
	if p.response != nil {
		if raw, err, handled := p.response(values); handled {
			return finish(raw, err)
		}
	}
	if len(values) > 1 {
		kind := p.kind
		if kind == "" {
			kind = ResourceLimitResponseBytes
		}
		return finish(nil, NewResourceLimitError(ResourceLimitError{Kind: kind, Limit: 1, Observed: len(values), ObservedKnown: true}))
	}
	raw, err := json.Marshal(testValue{Value: values[0]})
	return finish(raw, err)
}

func TestAdaptiveSplitMemoColdAndWarmKeepCompleteOrderedCover(t *testing.T) {
	for _, kind := range []ResourceLimitKind{ResourceLimitContextTokens, ResourceLimitOutputTokens, ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			p := &adaptiveSplitTestProvider{kind: kind}
			var notices []int
			executor := Executor{Enabled: true, RootDir: t.TempDir(), PlanNotice: func(n int) { notices = append(notices, n) }}
			items := [][]string{{"a", "b", "c", "d"}}
			for run := 0; run < 2; run++ {
				p.calls, notices = nil, nil
				plan, outcomes, accounting, err := ExecuteAdaptiveJSONBatchWithAccounting(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(plan, [][]string{{"a"}, {"b"}, {"c"}, {"d"}}) || len(outcomes) != 4 {
					t.Fatalf("plan/outcomes lost original coverage: %#v / %#v", plan, outcomes)
				}
				for i, want := range items[0] {
					if outcomes[i].Value.Value != want || run == 1 && !outcomes[i].Cached {
						t.Fatalf("run %d row %d: %#v", run, i, outcomes[i])
					}
				}
				if run == 0 {
					if len(p.calls) != 7 || !reflect.DeepEqual(notices, []int{1, 2, 3, 4}) {
						t.Fatalf("cold calls/notices = %d/%v, want 7/[1 2 3 4]", len(p.calls), notices)
					}
				} else if len(p.calls) != 0 || accounting != (AdaptiveBatchAccounting{}) || !reflect.DeepEqual(notices, []int{4}) {
					t.Fatalf("warm calls/accounting/notices = %d/%#v/%v", len(p.calls), accounting, notices)
				}
			}
			paths, err := filepath.Glob(filepath.Join(executor.RootDir, CacheDirectoryName, "memo-*.json"))
			if err != nil || len(paths) != 3 {
				t.Fatalf("nested resource hints = %v, %v", paths, err)
			}
			for _, path := range paths {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var record memoRecord
				if err := json.Unmarshal(raw, &record); err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(record.Value, &fields); err != nil || len(fields) != 2 || fields["version"] == nil || fields["resource_kind"] == nil {
					t.Fatalf("hint copied results or child plan: %s, %v", record.Value, err)
				}
			}
		})
	}
}

func TestAdaptiveSplitMemoDoesNotClaimMissingChildrenAccepted(t *testing.T) {
	p := &adaptiveSplitTestProvider{response: func(values []string) ([]byte, error, bool) {
		return []byte(`{"wrong":"shape"}`), nil, len(values) == 1
	}}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	items := [][]string{{"a", "b"}}
	if plan, outcomes, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit); err == nil || plan != nil || outcomes != nil {
		t.Fatalf("refused child supplied partial authority: %#v / %#v / %v", plan, outcomes, err)
	}
	p.calls, p.response = nil, nil
	plan, outcomes, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit)
	if err != nil || len(plan) != 2 || len(outcomes) != 2 || !reflect.DeepEqual(p.calls, [][]string{{"a"}, {"b"}}) {
		t.Fatalf("missing children not executed: %#v / %#v / %v", p.calls, outcomes, err)
	}
	for _, outcome := range outcomes {
		if outcome.Cached {
			t.Fatal("hint implied a nonexistent accepted child")
		}
	}
}

func TestAdaptiveSplitMemoNoCacheAndClear(t *testing.T) {
	p := &adaptiveSplitTestProvider{}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	items := [][]string{{"a", "b"}}
	run := func(executor Executor, wantCalls int) {
		t.Helper()
		p.calls = nil
		_, _, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit)
		if err != nil || len(p.calls) != wantCalls {
			t.Fatalf("calls = %d, want %d: %v", len(p.calls), wantCalls, err)
		}
	}
	run(executor, 3)
	run(executor, 0)
	disabled := executor
	disabled.Enabled = false
	prepared := p.prepared
	run(disabled, 3)
	if p.prepared-prepared != 3 {
		t.Fatalf("NoCache performed extra hint preparations: %d", p.prepared-prepared)
	}
	run(executor, 0)
	// The supported cache-clear command removes this one directory, including
	// its memo records; no second persistent target is introduced by hints.
	if err := os.RemoveAll(filepath.Join(executor.RootDir, CacheDirectoryName)); err != nil {
		t.Fatal(err)
	}
	run(executor, 3)
	disabled.RootDir = t.TempDir()
	run(disabled, 3)
	if _, err := os.Stat(filepath.Join(disabled.RootDir, CacheDirectoryName)); !os.IsNotExist(err) {
		t.Fatalf("NoCache created a persistent hint/cache: %v", err)
	}
}

func TestAdaptiveSplitMemoParentAndChildReplayRemainAuthoritative(t *testing.T) {
	p := &adaptiveSplitTestProvider{}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	items := [][]string{{"a", "b"}}
	run := func() ([][]string, []Outcome[testValue]) {
		t.Helper()
		p.calls = nil
		plan, outcomes, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit)
		if err != nil {
			t.Fatal(err)
		}
		return plan, outcomes
	}
	run()
	replay := func(item []string, raw string) {
		t.Helper()
		wire, _ := json.Marshal(item)
		prepared, _ := NewPrepared(wire)
		p.response = func([]string) ([]byte, error, bool) { return []byte(raw), nil, true }
		if _, err := ReplayJSON(t.Context(), executor, p, prepared); err != nil {
			t.Fatal(err)
		}
		p.response = nil
	}
	replay(items[0], `{"value":"whole replay"}`)
	plan, outcomes := run()
	if len(plan) != 1 || len(p.calls) != 0 || !outcomes[0].Cached || outcomes[0].Value.Value != "whole replay" {
		t.Fatalf("hint hid accepted parent replay: %#v / %#v", plan, outcomes)
	}
	// Replay accepts JSON; the stage still owns its schema and must reject a
	// wrong shape before it can supersede a split with semantic authority.
	replay(items[0], `{"wrong":"shape"}`)
	plan, outcomes = run()
	if len(plan) != 2 || len(p.calls) != 1 || !outcomes[0].Cached || outcomes[0].Value.Value != "a" {
		t.Fatalf("invalid replay bypassed domain validation: %#v / %#v / %#v", plan, outcomes, p.calls)
	}
	replay([]string{"a"}, `{"value":"child replay"}`)
	_, outcomes = run()
	if len(p.calls) != 0 || !outcomes[0].Cached || outcomes[0].Value.Value != "child replay" {
		t.Fatalf("hint hid current child replay: %#v / %#v", outcomes, p.calls)
	}
}

func TestAdaptiveSplitMemoIdentityIncludesCanonicalProviderExactBytesAndAllLimits(t *testing.T) {
	p := &adaptiveSplitTestProvider{}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	calls, _ := adaptiveBatchTestBuild([][]string{{"a", "b"}})
	call := calls[0]
	prepared, err := Prepare(p, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveAdaptiveSplit(executor, p, prepared.Bytes(), call.Limits, ResourceLimitOutputTokens); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		state  string
		change func(*Call[testValue])
		want   bool
	}{
		{name: "same", want: true},
		{name: "canonical state", state: ` { "model": "one", "provider": "adaptive-split-test" } `, want: true},
		{name: "different provider", state: `{"model":"two","provider":"adaptive-split-test"}`},
		{name: "request bytes", change: func(c *Call[testValue]) { c.Prompt.User = `["b","a"]` }},
		{name: "request limit", change: func(c *Call[testValue]) { c.Limits.MaxRequestBytes++ }},
		{name: "response limit", change: func(c *Call[testValue]) { c.Limits.MaxResponseBytes-- }},
		{name: "output limit", change: func(c *Call[testValue]) { c.Limits.MaxOutputTokens++ }},
		{name: "local validator version", change: func(c *Call[testValue]) { c.State = []byte("new local state") }, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p.state = nil
			if test.state != "" {
				p.state = []byte(test.state)
			}
			current := call
			if test.change != nil {
				test.change(&current)
			}
			found, err := loadAdaptiveSplit(executor, p, current)
			if err != nil || found != test.want || len(p.calls) != 0 {
				t.Fatalf("hint found=%v want=%v, calls=%d, err=%v", found, test.want, len(p.calls), err)
			}
		})
	}
}

func TestAdaptiveSplitMemoDoesNotPersistSemanticOrOtherResourceFailures(t *testing.T) {
	for _, kind := range []string{"invalid JSON", "invalid shape", "semantic resource", "request resource", "atomic"} {
		t.Run(kind, func(t *testing.T) {
			p := &adaptiveSplitTestProvider{}
			build := adaptiveBatchTestBuild
			split := adaptiveBatchTestSplit
			switch kind {
			case "invalid JSON":
				p.response = func([]string) ([]byte, error, bool) { return []byte(`{"value":`), nil, true }
			case "invalid shape":
				p.response = func([]string) ([]byte, error, bool) { return []byte(`{"wrong":"shape"}`), nil, true }
			case "semantic resource":
				p.response = func([]string) ([]byte, error, bool) { return []byte(`{"value":"ok"}`), nil, true }
				build = func(items [][]string) ([]Call[testValue], error) {
					calls, err := adaptiveBatchTestBuild(items)
					for i := range calls {
						calls[i].Validate = func(testValue) error {
							return NewResourceLimitError(ResourceLimitError{Kind: ResourceLimitResponseBytes})
						}
					}
					return calls, err
				}
			case "request resource":
				p.kind = ResourceLimitRequestBytes
			case "atomic":
				split = func([]string) ([]string, []string, bool) { return nil, nil, false }
			}
			executor := Executor{Enabled: true, RootDir: t.TempDir()}
			for run := 0; run < 2; run++ {
				_, _, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, [][]string{{"a", "b"}}, build, split)
				if err == nil {
					t.Fatal("refused response became accepted")
				}
			}
			paths, err := filepath.Glob(filepath.Join(executor.RootDir, CacheDirectoryName, "memo-*.json"))
			if err != nil || len(paths) != 0 || len(p.calls) != 2 {
				t.Fatalf("non-resource/atomic failure acquired hint: %v, calls=%d, err=%v", paths, len(p.calls), err)
			}
		})
	}
}

func TestAdaptiveSplitMemoStillUsesCurrentOwnerSplit(t *testing.T) {
	p := &adaptiveSplitTestProvider{}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	items := [][]string{{"a", "b"}}
	if _, _, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit); err != nil {
		t.Fatal(err)
	}
	p.calls = nil
	atomic := func([]string) ([]string, []string, bool) { return nil, nil, false }
	_, _, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, atomic)
	var resourceErr *ResourceLimitError
	if !errors.As(err, &resourceErr) || len(p.calls) != 1 {
		t.Fatalf("old hint bypassed current owner atomicity: calls=%v err=%v", p.calls, err)
	}
}

func TestAdaptiveSplitMemoReadErrorIsVisibleButAcceptedParentStillWins(t *testing.T) {
	p := &adaptiveSplitTestProvider{}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	items := [][]string{{"a", "b"}}
	calls, _ := adaptiveBatchTestBuild(items)
	call := calls[0]
	prepared, _ := Prepare(p, call.Prompt, call.Limits)
	state, _ := canonicalProviderState(p.State())
	key := adaptiveSplitKey(state, prepared.Bytes(), call.Limits)
	if err := SaveMemo(executor, key, []byte(`{"version":99,"resource_kind":"response_bytes"}`)); err != nil {
		t.Fatal(err)
	}
	_, _, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit)
	if err == nil || len(p.calls) != 0 {
		t.Fatalf("corrupt hint silently retried the costly parent: calls=%v err=%v", p.calls, err)
	}
	p.response = func([]string) ([]byte, error, bool) { return []byte(`{"value":"whole"}`), nil, true }
	if _, err := ReplayJSON(t.Context(), executor, p, prepared); err != nil {
		t.Fatal(err)
	}
	p.calls = nil
	plan, outcomes, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, adaptiveBatchTestBuild, adaptiveBatchTestSplit)
	if err != nil || len(plan) != 1 || len(outcomes) != 1 || !outcomes[0].Cached || outcomes[0].Value.Value != "whole" || len(p.calls) != 0 {
		t.Fatalf("corrupt hint hid valid parent replay: %#v / %#v / %v", plan, outcomes, err)
	}
}
