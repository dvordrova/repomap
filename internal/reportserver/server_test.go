package reportserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/freshness"
	reportpkg "github.com/dvordrova/repomap/internal/report"
)

const testCapability = "test-capability"

func TestHandlerListsReportsServesLatestAndOpensValidatedFile(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(filePath, []byte("package pebble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-190000-pebble", repo, "older")
	const unverifiedHTML = `<script>window.__unverified_saved_report_executed__ = true</script>`
	writeRun(t, runsDir, "20260711-200000-pebble", repo, unverifiedHTML)
	const legacyHTML = `<script>window.__legacy_saved_report_executed__ = true</script>`
	writeRun(t, runsDir, "20260711-210000-legacy", ".", legacyHTML)

	var openedPath string
	var openedLine int
	captureCalls := 0
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: "20260711-200000-pebble",
		Capability:   testCapability,
		OpenFile: func(_ context.Context, path string, line int) error {
			openedPath = path
			openedLine = line
			return nil
		},
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			captureCalls++
			return freshness.RepositoryState{
				Version: freshness.RepositoryStateVersion, Identity: filepath.Clean(repo), Head: strings.Repeat("0", 40),
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response, err := client.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != capabilityURLPrefix(testCapability)+"/runs/20260711-200000-pebble/report.html" {
		t.Fatalf("root response = %d location=%q", response.StatusCode, response.Header.Get("Location"))
	}

	response, err = server.Client().Get(baseURL + "/api/runs")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Runs []RunSummary `json:"runs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(list.Runs) != 3 || list.Runs[0].ID != "20260711-210000-legacy" {
		t.Fatalf("runs = %#v", list.Runs)
	}
	if captureCalls != 0 {
		t.Fatalf("run listing performed %d live freshness captures", captureCalls)
	}

	response, err = server.Client().Get(baseURL + "/runs/20260711-200000-pebble/report.html")
	if err != nil {
		t.Fatal(err)
	}
	report, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(report, []byte(`id="rm-report-data"`)) {
		t.Fatalf("trusted report response = %d body=%q", response.StatusCode, report)
	}
	if captureCalls != 1 {
		t.Fatalf("report request performed %d live freshness captures, want 1", captureCalls)
	}
	if bytes.Contains(report, []byte(unverifiedHTML)) {
		t.Fatal("server executed the saved report HTML instead of rendering verified report data")
	}
	if response.Header.Get("Content-Security-Policy") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("trusted report response missing security headers: %#v", response.Header)
	}

	response, err = server.Client().Get(baseURL + "/runs/20260711-210000-legacy/report.html")
	if err != nil {
		t.Fatal(err)
	}
	legacyResponse, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("legacy report status = %d, want %d", response.StatusCode, http.StatusConflict)
	}
	if bytes.Contains(legacyResponse, []byte(legacyHTML)) {
		t.Fatal("server returned unverified legacy HTML on the capability origin")
	}

	response = postOpen(t, baseURL, openRequest{
		RunID: "20260711-200000-pebble",
		Path:  "batch.go",
		Line:  288,
	}, true)
	response.Body.Close()
	canonicalFilePath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || openedPath != canonicalFilePath || openedLine != 288 {
		t.Fatalf("open response=%d path=%q line=%d", response.StatusCode, openedPath, openedLine)
	}
}

func TestOpenEndpointResolvesPathsFromManifestAnalysisRoot(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(analysisRoot, "batch.go")
	if err := os.WriteFile(filePath, []byte("package service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeScopedRun(t, runsDir, "20260711-200000-service", repository, analysisRoot, "report")

	var openedPath string
	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(_ context.Context, path string, _ int) error {
			openedPath = path
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID: "20260711-200000-service",
		Path:  "batch.go",
		Line:  1,
	}, true)
	response.Body.Close()
	want, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || openedPath != want {
		t.Fatalf("open response=%d path=%q, want %q", response.StatusCode, openedPath, want)
	}
}

func TestOpenEndpointRejectsCrossOriginShapeAndRepositoryEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", repo, "report")
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "escape.go")); err != nil {
		t.Fatal(err)
	}

	openCalls := 0
	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(context.Context, string, int) error {
			openCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response := postOpen(t, baseURL, openRequest{
		RunID: "20260711-200000-project",
		Path:  "../outside.go",
	}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("traversal status = %d", response.StatusCode)
	}

	response = postOpen(t, baseURL, openRequest{
		RunID: "20260711-200000-project",
		Path:  "escape.go",
	}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("symlink escape status = %d", response.StatusCode)
	}

	response = postOpen(t, baseURL, openRequest{
		RunID: "20260711-200000-project",
		Path:  "missing.go",
	}, false)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing action header status = %d", response.StatusCode)
	}

	body := strings.NewReader(`{"run_id":"20260711-200000-project","path":"missing.go"}`)
	hostRequest, err := http.NewRequest(http.MethodPost, baseURL+"/api/open", body)
	if err != nil {
		t.Fatal(err)
	}
	hostRequest.Host = "attacker.example"
	hostRequest.Header.Set("Content-Type", "application/json")
	hostRequest.Header.Set("Origin", "http://attacker.example")
	hostRequest.Header.Set("X-Repomap-Action", "open-file")
	response, err = http.DefaultClient.Do(hostRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host status = %d", response.StatusCode)
	}
	if openCalls != 0 {
		t.Fatalf("editor called %d times for rejected requests", openCalls)
	}
}

func TestOpenEndpointDoesNotExposeEditorCommandErrors(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package pebble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", repo, "saved report")

	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(context.Context, string, int) error {
			return fmt.Errorf("editor output leaked /Users/example/private.sock")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID: "20260711-200000-project",
		Path:  "batch.go",
		Line:  1,
	}, true)
	defer response.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
	if payload["error"] != "could not open file in VS Code" {
		t.Fatalf("public editor error = %q", payload["error"])
	}
}

func TestHandlerTransportRejectionMatrix(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package pebble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", repo, "report")

	openCalls := 0
	const expectedHost = "127.0.0.1:4321"
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		Capability:   testCapability,
		ExpectedHost: expectedHost,
		OpenFile: func(context.Context, string, int) error {
			openCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(openRequest{
		RunID: "20260711-200000-project",
		Path:  "batch.go",
		Line:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	prefix := capabilityURLPrefix(testCapability)
	tests := []struct {
		name       string
		method     string
		path       string
		host       string
		origin     string
		mediaType  string
		action     string
		wantStatus int
	}{
		{
			name:       "valid HTTP same origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusOK,
		},
		{
			name:       "HTTPS origin is rejected by the HTTP-only server",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "https://" + expectedHost,
			mediaType:  "application/json; charset=utf-8",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing capability",
			method:     http.MethodPost,
			path:       "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong capability",
			method:     http.MethodPost,
			path:       capabilityURLPrefix("wrong-capability") + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong host",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       "localhost:4321",
			origin:     "http://localhost:4321",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "null origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "null",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://attacker.example",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "origin with path",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost + "/unexpected",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing content type",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			action:     "open-file",
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:       "wrong content type",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "text/plain",
			action:     "open-file",
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:       "missing action header",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "OPTIONS preflight",
			method:     http.MethodOptions,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://"+expectedHost+test.path, bytes.NewReader(body))
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.mediaType != "" {
				request.Header.Set("Content-Type", test.mediaType)
			}
			if test.action != "" {
				request.Header.Set("X-Repomap-Action", test.action)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("unexpected CORS response header")
			}
		})
	}
	if openCalls != 1 {
		t.Fatalf("editor called %d times, want 1 accepted request", openCalls)
	}
}

func TestServeUsesRandomLoopbackPortAndStopsWithContext(t *testing.T) {
	t.Parallel()

	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", t.TempDir(), "report")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, Options{
			RunsDir:      runsDir,
			InitialRunID: "20260711-200000-project",
			OnReady: func(url string) error {
				ready <- url
				return nil
			},
		})
	}()

	var url string
	select {
	case url = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("report server did not become ready")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("server URL = %q", url)
	}
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(parsedURL.Path, "/_repomap/") || !strings.HasSuffix(parsedURL.Path, "/runs/20260711-200000-project/report.html") {
		t.Fatalf("server URL does not contain capability prefix and report path: %q", url)
	}
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	wrongHostRequest, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	wrongHostRequest.Host = "localhost:" + parsedURL.Port()
	response, err = http.DefaultClient.Do(wrongHostRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("alternate loopback host status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("report server did not stop after cancellation")
	}
}

func postOpen(t *testing.T, serverURL string, request openRequest, withHeader bool) *http.Response {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, serverURL+"/api/open", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	parsedURL, err := neturl.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Origin", parsedURL.Scheme+"://"+parsedURL.Host)
	if withHeader {
		httpRequest.Header.Set("X-Repomap-Action", "open-file")
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func writeRun(t *testing.T, runsDir, runID, repoPath, report string) {
	t.Helper()
	writeScopedRun(t, runsDir, runID, repoPath, repoPath, report)
}

func writeScopedRun(t *testing.T, runsDir, runID, repositoryPath, analysisRoot, report string) {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.html"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	reportData := reportpkg.ReportData{
		FormatVersion: reportpkg.CurrentFormatVersion,
		RepoName:      filepath.Base(analysisRoot),
		OpenablePaths: []string{"batch.go", "escape.go", "missing.go"},
	}
	reportJSON, err := json.Marshal(reportData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := json.Marshal(metadata{RepoName: filepath.Base(analysisRoot), RepoPath: analysisRoot, CreatedAt: runID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), metadataJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(repositoryPath) || !filepath.IsAbs(analysisRoot) {
		return
	}
	canonicalRepoPath, err := filepath.EvalSymlinks(repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRepoPath, err = filepath.Abs(canonicalRepoPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRepoPath = filepath.Clean(canonicalRepoPath)
	canonicalAnalysisRoot, err := filepath.EvalSymlinks(analysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalAnalysisRoot, err = filepath.Abs(canonicalAnalysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalAnalysisRoot = filepath.Clean(canonicalAnalysisRoot)
	state := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: canonicalRepoPath,
		Head:     strings.Repeat("0", 40),
	}
	stateDigest, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]freshness.CapturedInput, 0, len(reportData.OpenablePaths))
	for _, path := range reportData.OpenablePaths {
		inputs = append(inputs, freshness.CapturedInput{
			Version: freshness.CapturedInputVersion, ID: fmt.Sprintf("%x", sha256.Sum256([]byte("input\x00"+path))),
			Path: path, Kind: freshness.FileMissing, Stages: []string{"report_evidence"},
		})
	}
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest := reportpkg.RunManifest{
		Version:               reportpkg.CurrentRunManifestVersion,
		RepositoryState:       state,
		AnalysisRoot:          canonicalAnalysisRoot,
		RepositoryStateSHA256: stateDigest,
		ReportSHA256:          fmt.Sprintf("%x", sha256.Sum256(reportJSON)),
		ReportFormatVersion:   reportpkg.CurrentFormatVersion,
		OpenablePaths:         append([]string(nil), reportData.OpenablePaths...),
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		Freshness:             freshness.NewFreshnessResult(freshness.FreshnessFresh),
		MaterialInputs: reportpkg.MaterialInputs{
			SelectedRevision: state.Head, InputPolicyVersion: "captured-inputs-v1",
			ArchitectureContract: 1, ReportContract: reportpkg.CurrentFormatVersion,
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, reportpkg.RunManifestFilename), manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}
