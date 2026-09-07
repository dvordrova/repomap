package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReplayRefreshesExactCacheAndStageRevalidates(t *testing.T) {
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	provider := &testProvider{state: []byte(`{"endpoint":"test"}`), responses: [][]byte{
		[]byte(`{"value":"old"}`), []byte(`{"value":"new"}`), []byte(`{"wrong":"shape"}`), []byte(`{"value":"recovered"}`),
	}}
	call := baseTestCall("stage-v1", "same input")
	first, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := NewPrepared(first.Request)
	prepareCalls := provider.prepareCalls
	replayed, err := ReplayJSON(t.Context(), executor, provider, prepared)
	if err != nil || provider.prepareCalls != prepareCalls || replayed.CacheKey != first.CacheKey {
		t.Fatalf("replay rebuilt request or changed identity: %+v %v", replayed, err)
	}
	warm, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || !warm.Cached || warm.Value.Value != "new" || provider.completeCalls != 2 {
		t.Fatalf("replayed answer not reused: %+v %v", warm, err)
	}
	if _, err := ReplayJSON(t.Context(), executor, provider, prepared); err != nil {
		t.Fatal(err)
	}
	validated, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || validated.Value.Value != "recovered" || provider.completeCalls != 4 {
		t.Fatalf("stage reused an incompatible replay: %+v %v", validated, err)
	}
	// Both versions remain available by content hash; the current pointer has
	// no inline payload. Repeated request bytes occupy exactly one file.
	paths, _ := filepath.Glob(filepath.Join(executor.RootDir, CacheDirectoryName, "payloads", "*"))
	if len(paths) != 5 {
		t.Fatalf("want one request and four responses, got %d files", len(paths))
	}
	raw, err := os.ReadFile(filepath.Join(executor.RootDir, CacheDirectoryName, first.CacheKey+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record["response"] != nil || record["request_file"] == nil || record["response_file"] == nil {
		t.Fatal("accepted cache must reference shared payloads")
	}
}

func TestFailedReplayKeepsPreviousAcceptedAnswer(t *testing.T) {
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	provider := &testProvider{state: []byte(`{"endpoint":"test"}`), responses: [][]byte{[]byte(`{"value":"old"}`), []byte(`not json`)}}
	call := baseTestCall("stage", "same")
	first, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := NewPrepared(first.Request)
	if _, err := ReplayJSON(t.Context(), executor, provider, prepared); err == nil {
		t.Fatal("invalid replay accepted")
	}
	warm, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || warm.Value.Value != "old" || provider.completeCalls != 2 {
		t.Fatal("failed replay replaced accepted answer")
	}
}
