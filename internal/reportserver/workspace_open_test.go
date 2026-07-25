package reportserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

func TestOpenEndpointPreservesByteExactTargetWire(t *testing.T) {
	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	original := []byte("package original\n")
	if err := os.WriteFile(filePath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260725-100000-open-wire"
	writeRun(t, runsDir, runID, repo, "report")
	sourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	var openedPath string
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(_ context.Context, absolutePath string, _, _ int) error {
			launches++
			openedPath = absolutePath
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)

	response := postOpen(t, baseURL, openRequest{RunID: runID, SourceID: sourceID}, true)
	assertOpenWire(
		t,
		response,
		http.StatusOK,
		`{"source_changed":false,"status":"opened"}`+"\n",
	)
	wantPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 1 || openedPath != wantPath {
		t.Fatalf("unchanged launches=%d path=%q, want %q", launches, openedPath, wantPath)
	}

	if err := os.WriteFile(filePath, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	response = postOpen(t, baseURL, openRequest{RunID: runID, SourceID: sourceID}, true)
	assertOpenWire(
		t,
		response,
		http.StatusOK,
		`{"source_changed":true,"status":"opened"}`+"\n",
	)
	if launches != 2 || openedPath != wantPath {
		t.Fatalf("changed launches=%d path=%q, want %q", launches, openedPath, wantPath)
	}

	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	response = postOpen(t, baseURL, openRequest{RunID: runID, SourceID: sourceID}, true)
	assertOpenWire(
		t,
		response,
		http.StatusConflict,
		`{"code":"source_unavailable","error":"authorized source is unavailable"}`+"\n",
	)
	if launches != 2 {
		t.Fatalf("missing source launched editor %d times", launches)
	}

	otherPath := filepath.Join(repo, "other.go")
	if err := os.WriteFile(otherPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.go", filePath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	response = postOpen(t, baseURL, openRequest{RunID: runID, SourceID: sourceID}, true)
	assertOpenWire(
		t,
		response,
		http.StatusConflict,
		`{"code":"source_unavailable","error":"authorized source is unavailable"}`+"\n",
	)
	if launches != 2 {
		t.Fatalf("final symlink launched editor %d times", launches)
	}
}

func TestOpenEndpointRejectsReplacedSnapshotRootWithExistingWire(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260725-110000-root-replaced"
	writeRun(t, runsDir, runID, repo, "report")
	sourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			launches++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	held := filepath.Join(parent, "held")
	if err := os.Rename(repo, held); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(parent, "replacement")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "batch.go"), []byte("package replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("replacement", repo); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	response := postOpen(
		t,
		server.URL+capabilityURLPrefix(testCapability),
		openRequest{RunID: runID, SourceID: sourceID},
		true,
	)
	body := assertOpenWire(
		t,
		response,
		http.StatusConflict,
		`{"code":"source_unavailable","error":"authorized source is unavailable"}`+"\n",
	)
	if launches != 0 || bytes.Contains(body, []byte(parent)) {
		t.Fatalf("replaced root launches=%d body=%q", launches, body)
	}
}

func TestOpenEndpointPreservesCanceledRequestAdapterCompatibility(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260725-115000-canceled-open"
	writeRun(t, runsDir, runID, repo, "report")
	sourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	var launcherContextError error
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(ctx context.Context, _ string, _, _ int) error {
			launches++
			launcherContextError = ctx.Err()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(openRequest{RunID: runID, SourceID: sourceID})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"http://127.0.0.1"+capabilityURLPrefix(testCapability)+"/api/open",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1")
	request.Header.Set("X-Repomap-Action", "open-file")
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK ||
		response.Body.String() != `{"source_changed":false,"status":"opened"}`+"\n" ||
		launches != 1 || !errors.Is(launcherContextError, context.Canceled) {
		t.Fatalf("status=%d body=%q launches=%d launcher_ctx=%v",
			response.Code, response.Body.String(), launches, launcherContextError)
	}
}

func TestResolveOpenTargetUsesSnapshotCatalogAndLegacyFallback(t *testing.T) {
	repo, runsDir, _ := writeAnalysisRun(t)
	rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
		manifest.Version = 3
	})
	h := &handler{runsDir: runsDir}
	if err := h.reloadRuns(); err != nil {
		t.Fatal(err)
	}
	runs := h.runsSnapshot()
	if len(runs) != 1 || runs[0].WorkspaceSnapshot == nil {
		t.Fatalf("loaded runs = %#v", runs)
	}
	run := runs[0]
	sourceID := run.Report.SourceIDs["batch.go"]
	target, ok := run.Sources[sourceID]
	if !ok {
		t.Fatalf("source target %q unavailable", sourceID)
	}

	changedContent := []byte("package changed\n")
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), changedContent, 0o600); err != nil {
		t.Fatal(err)
	}
	target.capturedSHA256 = hashBytes(changedContent)
	absolutePath, changed, err := resolveOpenTarget(context.Background(), run, target)
	if err != nil || !changed || absolutePath != filepath.Join(repo, "batch.go") {
		t.Fatalf("snapshot resolve path=%q changed=%t err=%v", absolutePath, changed, err)
	}

	run.WorkspaceSnapshot = nil
	run.SourceCatalog = nil
	absolutePath, changed, err = resolveOpenTarget(context.Background(), run, target)
	if err != nil || changed || absolutePath != filepath.Join(repo, "batch.go") {
		t.Fatalf("legacy resolve path=%q changed=%t err=%v", absolutePath, changed, err)
	}
}

func TestOpenEndpointPreservesLegacyAndCurrentVersionMatrix(t *testing.T) {
	tests := []struct {
		name       string
		rewrite    func(*testing.T, string)
		wantStatus int
		wantBody   string
		wantLaunch int
	}{
		{
			name: "version 2 legacy open",
			rewrite: func(t *testing.T, runsDir string) {
				rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
					manifest.Version = 2
					manifest.RepositoryState.Version = 1
					manifest.CapturedInputs = nil
					manifest.CapturedInputsSHA256 = ""
					manifest.Freshness = freshness.FreshnessResult{}
					manifest.MaterialInputs = report.MaterialInputs{}
					digest, err := manifest.RepositoryState.Digest()
					if err != nil {
						t.Fatal(err)
					}
					manifest.RepositoryStateSHA256 = digest
				})
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"source_changed":false,"status":"opened"}` + "\n",
			wantLaunch: 1,
		},
		{
			name: "valid version 3 snapshot open",
			rewrite: func(t *testing.T, runsDir string) {
				rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
					manifest.Version = 3
				})
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"source_changed":false,"status":"opened"}` + "\n",
			wantLaunch: 1,
		},
		{
			name: "degraded version 3 legacy open",
			rewrite: func(t *testing.T, runsDir string) {
				rewriteAnalysisManifestWithOversizedStages(t, runsDir, 3)
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"source_changed":false,"status":"opened"}` + "\n",
			wantLaunch: 1,
		},
		{
			name: "current version without snapshot is view only",
			rewrite: func(t *testing.T, runsDir string) {
				rewriteAnalysisManifestWithOversizedStages(t, runsDir, report.CurrentRunManifestVersion)
			},
			wantStatus: http.StatusConflict,
			wantBody:   `{"error":"this report is view-only; regenerate it to enable editor actions"}` + "\n",
			wantLaunch: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, runsDir, _ := writeAnalysisRun(t)
			test.rewrite(t, runsDir)
			sourceID := testSourceID(t, runsDir, "20260711-220000-pebble", "batch.go")
			launches := 0
			handler, err := NewHandler(Options{
				RunsDir: runsDir, Capability: testCapability,
				OpenFile: func(context.Context, string, int, int) error {
					launches++
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			response := postOpen(
				t,
				server.URL+capabilityURLPrefix(testCapability),
				openRequest{RunID: "20260711-220000-pebble", SourceID: sourceID},
				true,
			)
			assertOpenWire(t, response, test.wantStatus, test.wantBody)
			if launches != test.wantLaunch {
				t.Fatalf("launches=%d, want %d", launches, test.wantLaunch)
			}
		})
	}
}

func TestOpenEndpointPreservesEditorBusyAndFailureWire(t *testing.T) {
	t.Run("busy", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runsDir := t.TempDir()
		const runID = "20260725-120000-editor-busy"
		writeRun(t, runsDir, runID, repo, "report")
		sourceID := testSourceID(t, runsDir, runID, "batch.go")
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		var launches atomic.Int32
		handler, err := NewHandler(Options{
			RunsDir: runsDir, Capability: testCapability,
			OpenFile: func(context.Context, string, int, int) error {
				launches.Add(1)
				entered <- struct{}{}
				<-release
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		baseURL := server.URL + capabilityURLPrefix(testCapability)

		type result struct {
			status int
			body   []byte
			err    error
		}
		first := make(chan result, 1)
		go func() {
			response, err := sendOpen(baseURL, openRequest{RunID: runID, SourceID: sourceID})
			if err != nil {
				first <- result{err: err}
				return
			}
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			first <- result{status: response.StatusCode, body: body, err: readErr}
		}()
		<-entered

		response, err := sendOpen(baseURL, openRequest{RunID: runID, SourceID: sourceID})
		if err != nil {
			t.Fatal(err)
		}
		assertOpenWire(
			t,
			response,
			http.StatusTooManyRequests,
			`{"code":"editor_busy","error":"another editor action is still running"}`+"\n",
		)
		close(release)
		firstResult := <-first
		if firstResult.err != nil || firstResult.status != http.StatusOK ||
			string(firstResult.body) != `{"source_changed":false,"status":"opened"}`+"\n" ||
			launches.Load() != 1 {
			t.Fatalf("first result=%#v launches=%d", firstResult, launches.Load())
		}
	})

	for _, test := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{
			name:   "unavailable",
			err:    ErrEditorUnavailable,
			status: http.StatusServiceUnavailable,
			body:   `{"code":"editor_unavailable","error":"could not open file in VS Code"}` + "\n",
		},
		{
			name:   "launch failure",
			err:    errors.New("/Users/example/private.sock"),
			status: http.StatusBadGateway,
			body:   `{"code":"editor_launch_failed","error":"could not open file in VS Code"}` + "\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			runsDir := t.TempDir()
			const runID = "20260725-130000-editor-error"
			writeRun(t, runsDir, runID, repo, "report")
			handler, err := NewHandler(Options{
				RunsDir: runsDir, Capability: testCapability,
				OpenFile: func(context.Context, string, int, int) error {
					if errors.Is(test.err, ErrEditorUnavailable) {
						return errors.Join(ErrEditorUnavailable, test.err)
					}
					return test.err
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			response := postOpen(
				t,
				server.URL+capabilityURLPrefix(testCapability),
				openRequest{
					RunID:    runID,
					SourceID: testSourceID(t, runsDir, runID, "batch.go"),
				},
				true,
			)
			body := assertOpenWire(t, response, test.status, test.body)
			if bytes.Contains(body, []byte("/Users/")) {
				t.Fatalf("response leaked editor path: %q", body)
			}
		})
	}
}

func assertOpenWire(
	t *testing.T,
	response *http.Response,
	wantStatus int,
	wantBody string,
) []byte {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus || string(body) != wantBody {
		t.Fatalf("status=%d body=%q, want status=%d body=%q",
			response.StatusCode, body, wantStatus, wantBody)
	}
	if response.Header.Get("Content-Type") != "application/json; charset=utf-8" ||
		response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("response headers = %#v", response.Header)
	}
	return body
}

func sendOpen(baseURL string, request openRequest) (*http.Response, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequest(
		http.MethodPost,
		baseURL+"/api/open",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	parsedURL, err := neturl.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Origin", parsedURL.Scheme+"://"+parsedURL.Host)
	httpRequest.Header.Set("X-Repomap-Action", "open-file")
	return http.DefaultClient.Do(httpRequest)
}

func hashBytes(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}
