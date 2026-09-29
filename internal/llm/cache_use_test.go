package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A warm run sets the answer it reads, record and payloads, to the time of
// the read, so answers no run uses any more can be found by their age
// (owner, 2026-09-29: we paid for them, but may never read them again).
func TestACachedAnswerReadIsMarkedAsUsed(t *testing.T) {
	provider := baseTestProvider()
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	call := baseTestCall("cube-v1", "same input")
	cold, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(executor.RootDir, CacheDirectoryName)
	recordPath := filepath.Join(cacheDir, cold.CacheKey+".json")
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		RequestFile  string `json:"request_file"`
		ResponseFile string `json:"response_file"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	paths := []string{recordPath, filepath.Join(cacheDir, "payloads", record.RequestFile), filepath.Join(cacheDir, "payloads", record.ResponseFile)}
	old := time.Now().Add(-30 * 24 * time.Hour)
	for _, path := range paths {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	warm, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || !warm.Cached {
		t.Fatalf("warm read = %+v / %v", warm, err)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.ModTime().Before(time.Now().Add(-time.Hour)) {
			t.Fatalf("%s still reads as last used %s", filepath.Base(path), info.ModTime())
		}
	}
}
