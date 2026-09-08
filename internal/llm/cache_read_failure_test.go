package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheCorruptionPreservesUnderlyingError(t *testing.T) {
	if err := corruptCache(fmt.Errorf("decode: %w", io.EOF)); !errors.Is(err, io.EOF) || !isCacheCorruption(err) {
		t.Fatalf("corruption marker hid error identity: %v", err)
	}
	var value any
	decodeErr := json.Unmarshal([]byte("{"), &value)
	var syntaxErr *json.SyntaxError
	if !errors.As(corruptCache(decodeErr), &syntaxErr) {
		t.Fatal("corruption marker hid JSON syntax diagnostic")
	}
}

func TestExecuteJSONPreservesCacheAfterReadFailure(t *testing.T) {
	for _, cause := range []string{"permission", "smaller local response limit"} {
		t.Run(cause, func(t *testing.T) {
			provider := baseTestProvider()
			executor := Executor{RootDir: t.TempDir(), Enabled: true}
			call := baseTestCall("cube-v1", "same input")
			cold, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(executor.RootDir, CacheDirectoryName, cold.CacheKey+".json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			limited := call
			if cause == "permission" {
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
				if _, err := os.ReadFile(path); err == nil {
					t.Skip("current user can read files without read permissions")
				} else if !errors.Is(err, os.ErrPermission) {
					t.Fatal(err)
				}
			} else {
				limited.Limits.MaxResponseBytes = len(cold.Response) - 1
			}
			cacheFailures := 0
			executor.Observer = ObserverFunc(func(event Event) error {
				if event.Kind == EventFailure && event.Source == SourceCache {
					cacheFailures++
					// Restore access before any attempted eviction. The next provider
					// failure must not destroy the previously accepted response.
					return os.Chmod(path, 0o600)
				}
				return nil
			})
			provider.errors = []error{errors.New("provider unavailable")}
			failed, err := ExecuteJSON(t.Context(), executor, provider, limited)
			if err == nil || !hasIssue(failed.Issues, IssueCacheRead) || cacheFailures != 1 || provider.completeCalls != 2 {
				t.Fatalf("read failure = %+v / %v, notices=%d calls=%d", failed, err, cacheFailures, provider.completeCalls)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("accepted cache changed after failed read and provider call: %v", err)
			}
			warm, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil || !warm.Cached || warm.Value != cold.Value || provider.completeCalls != 2 {
				t.Fatalf("accepted response was not reusable: %+v / %v, calls=%d", warm, err, provider.completeCalls)
			}
		})
	}
}

func TestChangedOpenedCacheFileIsNotCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entry")
	replacement := filepath.Join(dir, "replacement")
	for _, filename := range []string{path, replacement} {
		if err := os.WriteFile(filename, []byte("valid cache data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOpenedCacheFile(before, opened, 1024); err == nil || isCacheCorruption(err) {
		t.Fatalf("concurrent replacement must be a read failure, not corruption: %v", err)
	}
	if err := validateOpenedCacheFile(opened, opened, 1024); err != nil {
		t.Fatalf("unchanged opened file: %v", err)
	}
}
