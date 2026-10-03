package llm

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A payload is shared by name: every record, journal and window ref that used
// these bytes links the same file. A damaged one costs the request one live
// answer, whose save restores the exact bytes in place; the next run is a hit
// (review A6: the record was evicted and rewritten over the same corrupt
// file, so the same request paid the provider on every run). The damaged
// record itself, rather than its payload, is TestExecuteJSONEvictsUnsafeCacheAndRefetchesOnce.
func TestADamagedPayloadCostsOneAnswerAndTheNextRunHits(t *testing.T) {
	damages := map[string]func(t *testing.T, path string, exact []byte){
		"other bytes of the same size": func(t *testing.T, path string, exact []byte) {
			other := bytes.Clone(exact)
			other[len(other)/2] ^= 0x20
			if err := os.WriteFile(path, other, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"truncated": func(t *testing.T, path string, exact []byte) {
			if err := os.WriteFile(path, exact[:len(exact)/2], 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"empty": func(t *testing.T, path string, _ []byte) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"unreadable": func(t *testing.T, path string, _ []byte) {
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			if _, err := os.ReadFile(path); err == nil {
				t.Skip("current user can read files without read permissions")
			}
		},
		"a link elsewhere": func(t *testing.T, path string, exact []byte) {
			target := filepath.Join(t.TempDir(), "outside.json")
			if err := os.WriteFile(target, []byte(`{"outside":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if got, err := os.ReadFile(target); err != nil || string(got) != `{"outside":true}` {
					t.Errorf("restoring the payload wrote through its link: %q, %v", got, err)
				}
			})
		},
	}
	for _, which := range []string{"request", "response"} {
		for name, damage := range damages {
			t.Run(which+"/"+name, func(t *testing.T) {
				provider := baseTestProvider()
				executor := Executor{RootDir: t.TempDir(), Enabled: true}
				call := baseTestCall("cube-v1", "one exact request")
				cold, err := ExecuteJSON(t.Context(), executor, provider, call)
				if err != nil {
					t.Fatal(err)
				}
				if warm, err := ExecuteJSON(t.Context(), executor, provider, call); err != nil || !warm.Cached || provider.completeCalls != 1 {
					t.Fatalf("control warm run: %+v / %v, calls=%d", warm, err, provider.completeCalls)
				}
				exact := cold.Request
				if which == "response" {
					exact = cold.Response
				}
				path := payloadPath(t, executor.RootDir, exact)
				damage(t, path, exact)

				again, err := ExecuteJSON(t.Context(), executor, provider, call)
				if err != nil || again.Cached || provider.completeCalls != 2 || !hasIssue(again.Issues, IssueCacheRead) || hasIssue(again.Issues, IssueCacheWrite) {
					t.Fatalf("damaged payload: %+v / %v, calls=%d", again, err, provider.completeCalls)
				}
				for range 2 {
					hit, err := ExecuteJSON(t.Context(), executor, provider, call)
					if err != nil || !hit.Cached || hit.Value != cold.Value || provider.completeCalls != 2 || len(hit.Issues) != 0 {
						t.Fatalf("the run after one re-answer paid again: %+v / %v, calls=%d", hit, err, provider.completeCalls)
					}
				}
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("restored payload is not a regular file: %v / %v", info, err)
				}
				if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, exact) {
					t.Fatalf("payload bytes were not restored: %q / %v", got, err)
				}
			})
		}
	}
}

// Two accepted records whose answers are the same bytes share one response
// payload. Damaging it costs only the request read first: its save restores
// the shared file in place, so the other record, never rewritten, hits again.
func TestARestoredPayloadServesEveryRecordThatSharesIt(t *testing.T) {
	provider := baseTestProvider()
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	first, second := baseTestCall("cube-v1", "first request"), baseTestCall("cube-v1", "second request")
	a, err := ExecuteJSON(t.Context(), executor, provider, first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExecuteJSON(t.Context(), executor, provider, second)
	if err != nil || !bytes.Equal(a.Response, b.Response) || provider.completeCalls != 2 {
		t.Fatalf("fixture answers differ: %q / %q, %v", a.Response, b.Response, err)
	}
	secondRecord := filepath.Join(executor.RootDir, CacheDirectoryName, b.CacheKey+".json")
	recordBefore, err := os.ReadFile(secondRecord)
	if err != nil {
		t.Fatal(err)
	}
	path := payloadPath(t, executor.RootDir, a.Response)
	if err := os.WriteFile(path, []byte("torn"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := ExecuteJSON(t.Context(), executor, provider, first); err != nil || again.Cached || provider.completeCalls != 3 {
		t.Fatalf("damaged shared payload: %+v / %v, calls=%d", again, err, provider.completeCalls)
	}
	hit, err := ExecuteJSON(t.Context(), executor, provider, second)
	if err != nil || !hit.Cached || provider.completeCalls != 3 {
		t.Fatalf("the other owner lost its answer: %+v / %v, calls=%d", hit, err, provider.completeCalls)
	}
	if recordAfter, err := os.ReadFile(secondRecord); err != nil || !bytes.Equal(recordAfter, recordBefore) {
		t.Fatalf("restoring a shared payload rewrote another owner's record: %v", err)
	}
}

// A directory in a payload's place is not a payload the cache may remove: the
// save fails, the answer stays accepted and the failure is a cache_write issue.
// The run's own payloads are restored the same way as the shared ones.
func TestAPayloadThatCannotBeRestoredIsAnError(t *testing.T) {
	provider := baseTestProvider()
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	call := baseTestCall("cube-v1", "one exact request")
	prepared, err := Prepare(provider, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SavePayload(executor.RootDir, []byte("placeholder")); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(executor.RootDir, CacheDirectoryName, "payloads", sha256Hex(prepared.Bytes())+".json")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || outcome.Value.Value != "ok" || !hasIssue(outcome.Issues, IssueCacheWrite) {
		t.Fatalf("an unrestorable payload changed the answer: %+v / %v", outcome, err)
	}
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() {
		t.Fatalf("the directory in the payload's place was removed: %v", err)
	}

	run := t.TempDir()
	raw := []byte(`{"refused":"answer"}`)
	path, err := SaveRunPayload(run, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"refused":"answe!"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := SaveRunPayload(run, raw); err != nil || again != path {
		t.Fatalf("run payload: %s / %v", again, err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("run payload was not restored: %q / %v", got, err)
	}
	if _, err := SaveRunPayload("", raw); err == nil {
		t.Fatal("an empty run directory was accepted")
	}
}

func payloadPath(t *testing.T, root string, exact []byte) string {
	t.Helper()
	path, err := SavePayload(root, exact)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
