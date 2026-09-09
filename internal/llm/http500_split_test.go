package llm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTP500SplitMemoRequiresItsOwnOptInAndKeepsParentReplay(t *testing.T) {
	p := &adaptiveSplitTestProvider{response: func(values []string) ([]byte, error, bool) {
		return nil, &classifiedTestProviderError{cause: errors.New("server failed"), failure: ProviderFailure{Kind: ProviderFailureHTTPStatus, HTTPStatus: 500}}, len(values) > 1
	}}
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	items := [][]string{{"a", "b"}}
	optIn := true
	build := func(items [][]string) ([]Call[testValue], error) {
		calls, err := adaptiveBatchTestBuild(items)
		for i := range calls {
			calls[i].SplitRejectedResponse = true
			calls[i].SplitHTTP500 = optIn && len(items[i]) > 1
		}
		return calls, err
	}
	run := func(wantCalls int, success bool) {
		t.Helper()
		p.calls = nil
		_, outcomes, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, build, adaptiveBatchTestSplit)
		if (err == nil) != success || len(p.calls) != wantCalls || (!success && outcomes != nil) {
			t.Fatalf("calls=%v outcomes=%v error=%v", p.calls, outcomes, err)
		}
	}
	run(3, true)
	run(0, true)
	optIn = false
	run(1, false) // opting into invalid-response splits does not include HTTP 500
	optIn = true
	executor.Enabled = false
	run(3, true)
	executor.Enabled = true
	run(0, true)
	if err := os.RemoveAll(filepath.Join(executor.RootDir, CacheDirectoryName)); err != nil {
		t.Fatal(err)
	}
	run(3, true)
	prepared, _ := NewPrepared([]byte(`["a","b"]`))
	p.response = func([]string) ([]byte, error, bool) { return []byte(`{"value":"whole replay"}`), nil, true }
	if _, err := ReplayJSON(t.Context(), executor, p, prepared); err != nil {
		t.Fatal(err)
	}
	p.calls = nil
	plan, outcomes, err := ExecuteAdaptiveJSONBatch(t.Context(), executor, p, items, build, adaptiveBatchTestSplit)
	if err != nil || len(plan) != 1 || len(p.calls) != 0 || !outcomes[0].Cached || outcomes[0].Value.Value != "whole replay" {
		t.Fatalf("HTTP 500 memo hid accepted parent: %v / %v", plan, err)
	}
}
