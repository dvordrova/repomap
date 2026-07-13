package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
)

func TestPrintPromptVersions(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	printPromptVersions(&output)
	var versions map[string]string
	if err := json.Unmarshal(output.Bytes(), &versions); err != nil {
		t.Fatalf("prompt versions are not JSON: %v", err)
	}
	want := map[string]string{
		"orientation_json": deepseek.OrientationPromptVersionJSON,
		"source_json":      deepseek.SourcePromptVersionJSON,
		"symbol_json":      deepseek.SymbolPromptVersionJSON,
		"symbol_tagged":    deepseek.SymbolPromptVersionTagged,
	}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("prompt versions = %#v, want %#v", versions, want)
	}
}

func TestWriteProgressShowsWaitsAndProviderMeasurements(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	for _, event := range []orient.ProgressEvent{
		{
			Stage: orient.ProgressProviderWaiting, Model: "fixture-model",
			Activity: "orientation", LatencyMillis: 10_000,
		},
		{
			Stage: orient.ProgressOrientationDone, ResponseBytes: 4096,
			LatencyMillis: 12_500, InputTokens: 900, OutputTokens: 120,
			CandidateCount: 3, Cached: true,
		},
		{
			Stage: orient.ProgressResearchDone, Activity: "completed",
			RequestBytes: 8192, ResponseBytes: 2048, LatencyMillis: 15_000,
			InputTokens: 700, OutputTokens: 80, FindingCount: 2,
			RejectedCount: 1, NewFactCount: 2,
		},
	} {
		writeProgress(&output, event)
	}

	for _, want := range []string{
		"orientation from fixture-model still running after 10s (Ctrl-C to cancel)",
		"reused cached orientation response of 4096 bytes (original call: 12500 ms, 900 input / 120 output tokens)",
		"validated 3 candidate direction(s)",
		"targeted research completed: received 2048 bytes from a 8192-byte request",
		"2 validated, 1 rejected, 2 new grounded fact(s)",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("progress output missing %q:\n%s", want, output.String())
		}
	}
	if got := formatTokenUsage(0, 0); got != "tokens unavailable" {
		t.Fatalf("formatTokenUsage(0, 0) = %q", got)
	}
}

func TestRunDefaultStopsWhenOrientationIsCanceled(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/cancel\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	started := make(chan struct{}, 1)
	releaseHandler := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		select {
		case <-request.Context().Done():
		case <-releaseHandler:
		}
	}))
	defer server.Close()
	defer close(releaseHandler)

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()

	var stderr bytes.Buffer
	err := runDefaultWithDeps(
		repo,
		[]string{"--debug-dir", t.TempDir(), "--discover-surfaces=false", "--no-open", "--no-serve"},
		defaultRunDeps{ctx: ctx, stdout: io.Discard, stderr: &stderr},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runDefaultWithDeps() error = %v, want context.Canceled", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("provider requests = %d, want 1", got)
	}
	for _, unwanted := range []string{"targeted research failed", "synthesizing bounded architecture", "Report: "} {
		if strings.Contains(stderr.String(), unwanted) {
			t.Errorf("stderr contains post-cancel output %q:\n%s", unwanted, stderr.String())
		}
	}

	var userError bytes.Buffer
	writeDefaultRunError(&userError, err)
	if got := userError.String(); got != "repomap: canceled\n" {
		t.Fatalf("cancellation message = %q", got)
	}
	if got := defaultRunExitCode(err); got != 130 {
		t.Fatalf("cancellation exit code = %d, want 130", got)
	}
}

func TestRunDefaultCompletesOneRequestOrientationJourney(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/friend-trial\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	orientation := map[string]any{
		"project_guess": "tiny Go command",
		"confidence":    0.9,
		"high_level_map": []any{map[string]any{
			"name":           "command",
			"evidence":       []string{"main.go"},
			"why_it_matters": "it owns process startup",
		}},
		"first_files_to_open": []any{map[string]any{
			"path":   "main.go",
			"reason": "process entrypoint",
		}},
		"candidate_flows": []any{map[string]any{
			"name":              "Process startup",
			"trigger":           "the executable starts",
			"likely_entrypoint": "main.go",
			"likely_files":      []string{"main.go"},
			"why_interesting":   "shows the complete behavior of this tiny command",
			"evidence":          []string{"main.go"},
			"confidence":        0.9,
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []string{"Which behavior should we inspect next?"},
		"unverified_paths":       []any{},
		"warnings":               []string{},
	}
	orientationJSON, err := json.Marshal(orientation)
	if err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		envelope, marshalErr := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": string(orientationJSON),
				},
			}},
		})
		if marshalErr != nil {
			t.Errorf("marshal envelope: %v", marshalErr)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(envelope)
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "deepseek-v4-flash")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	var preview bytes.Buffer
	previewOpened := false
	err = runDefaultWithDeps(repo, []string{"--preview-request"}, defaultRunDeps{
		stdout: &preview,
		stderr: io.Discard,
		openReport: func(string) error {
			previewOpened = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("preview request: %v", err)
	}
	if requestCount != 0 {
		t.Fatalf("preview made %d provider request(s), want 0", requestCount)
	}
	if previewOpened {
		t.Fatal("preview opened a report")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var openedReport string
	debugDir := t.TempDir()
	err = runDefaultWithDeps(repo, []string{"--debug-dir", debugDir}, defaultRunDeps{
		stdout: &stdout,
		stderr: &stderr,
		openReport: func(path string) error {
			openedReport = path
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}

	if requestCount != 1 {
		t.Fatalf("provider request count = %d, want 1", requestCount)
	}
	if !bytes.Equal(preview.Bytes(), requestBody) {
		t.Fatalf("preview differs from outbound request\npreview: %s\nrequest: %s", preview.Bytes(), requestBody)
	}
	for _, want := range []string{`"model":"deepseek-v4-flash"`, `"response_format":{"type":"json_object"}`} {
		if !bytes.Contains(requestBody, []byte(want)) {
			t.Fatalf("request body missing %q: %s", want, requestBody)
		}
	}
	for _, want := range []string{"Project: tiny Go command", "Candidate directions:", "Process startup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	for _, want := range []string{
		"repomap: collecting tracked repository facts from ",
		"repomap: repository facts ready: ",
		"repomap: compact local context ",
		"repomap: discovering local Go runtime surfaces",
		"repomap: discovered 0 local runtime surface(s)",
		fmt.Sprintf("repomap: prepared %d-byte orientation request for deepseek-v4-flash", len(requestBody)),
		"validated 1 candidate direction(s)",
		"Report: ",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
	if openedReport == "" {
		t.Fatal("generated report was not opened")
	}
	if _, err := os.Stat(openedReport); err != nil {
		t.Fatalf("opened report %q: %v", openedReport, err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(filepath.Dir(openedReport), "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	for _, want := range [][]byte{
		[]byte("Process startup"),
		[]byte(`"high_level_map"`),
		[]byte("it owns process startup"),
		[]byte(`"first_files_to_open"`),
		[]byte("Which behavior should we inspect next?"),
		[]byte(`"compact_context_bytes"`),
		[]byte(`"external_request_bytes"`),
		[]byte(`"discovered_surfaces"`),
		[]byte(`"evidence_only": true`),
	} {
		if !bytes.Contains(reportJSON, want) {
			t.Fatalf("report.json does not retain %q: %s", want, reportJSON)
		}
	}
	reportHTML, err := os.ReadFile(openedReport)
	if err != nil {
		t.Fatalf("read report HTML: %v", err)
	}
	for _, want := range [][]byte{
		[]byte("Explore this direction"),
		[]byte("Suggested files are selected from repository facts"),
		[]byte("Compact local context"),
		[]byte("Provider request bodies"),
	} {
		if !bytes.Contains(reportHTML, want) {
			t.Fatalf("report HTML does not contain %q", want)
		}
	}
	if bytes.Contains(reportHTML, []byte("· local evidence")) {
		t.Fatal("report HTML exposes internal local-evidence status in navigation")
	}
}

func TestRunDefaultCompletesPythonOrientationJourney(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src/tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "tests"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "README.md"), "# Tiny Python service\n")
	writeFile(t, filepath.Join(repo, "pyproject.toml"), "[project]\nname = \"tiny-python-service\"\n")
	writeFile(t, filepath.Join(repo, "src/tool/__main__.py"), "from tool.service import run\n\nrun()\n")
	writeFile(t, filepath.Join(repo, "src/tool/service.py"), "def run() -> None:\n    print(\"hello\")\n")
	writeFile(t, filepath.Join(repo, "tests/test_service.py"), "from tool.service import run\n\ndef test_run() -> None:\n    run()\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "README.md", "pyproject.toml", "src/tool/__main__.py", "src/tool/service.py", "tests/test_service.py")
	commitTestRepository(t, repo)

	orientationJSON, err := json.Marshal(map[string]any{
		"project_guess": "tiny Python service",
		"confidence":    0.85,
		"high_level_map": []any{map[string]any{
			"name": "CLI entry", "role": "entry", "evidence": []string{"src/tool/__main__.py"},
			"why_it_matters": "starts the utility",
		}},
		"first_files_to_open": []any{map[string]any{"path": "src/tool/__main__.py", "reason": "entrypoint"}},
		"candidate_flows": []any{map[string]any{
			"name": "CLI startup", "trigger": "python module execution",
			"likely_entrypoint": "src/tool/__main__.py",
			"likely_files":      []string{"src/tool/__main__.py", "src/tool/service.py"},
			"why_interesting":   "shows startup and delegation",
			"evidence":          []string{"src/tool/__main__.py", "src/tool/service.py"},
			"confidence":        0.85,
		}},
		"important_domain_words": []any{}, "questions_for_human": []any{},
		"unverified_paths": []any{}, "warnings": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestCount := 0
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": string(orientationJSON),
			}}},
		})
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "deepseek-v4-flash")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	debugDir := t.TempDir()
	if err := runDefaultWithDeps(repo, []string{"--debug-dir", debugDir, "--no-open", "--no-serve"}, defaultRunDeps{
		stdout: &stdout,
		stderr: &stderr,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("provider request count = %d, want 1", requestCount)
	}
	requestText := string(requestBody)
	for _, want := range []string{`\"language\":\"Python\"`, "src/tool/__main__.py", "src/tool/service.py", "tests/test_service.py"} {
		if !strings.Contains(requestText, want) {
			t.Fatalf("Python request missing %q: %s", want, requestText)
		}
	}
	if strings.Contains(requestText, "senior Go engineer") || strings.Contains(requestText, "unfamiliar Go repository") {
		t.Fatalf("Python request retained Go-only prompt: %s", requestText)
	}
	if !strings.Contains(stdout.String(), "Project: tiny Python service") ||
		!strings.Contains(stderr.String(), "repomap: collecting tracked repository facts from ") {
		t.Fatalf("stdout/stderr missing Python journey output\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}

	entries, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDir string
	for _, entry := range entries {
		if entry.IsDir() {
			runDir = filepath.Join(debugDir, entry.Name())
		}
	}
	if runDir == "" {
		t.Fatalf("debug entries = %#v, want a run directory", entries)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tiny Python service", "CLI startup", "src/tool/__main__.py"} {
		if !strings.Contains(string(reportJSON), want) {
			t.Fatalf("report missing %q: %s", want, reportJSON)
		}
	}
}

func TestRunDefaultNoOpenSuppressesBrowser(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/offline\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	opened := false
	if err := runDefaultWithDeps(repo, []string{"--offline", "--no-open", "--debug-dir", t.TempDir()}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
		openReport: func(string) error {
			opened = true
			return nil
		},
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if opened {
		t.Fatal("browser opener was called with --no-open")
	}
}

func TestRunDefaultPreservesReportWhenCapturedInputsBecomeStale(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/moving\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)
	initial, err := freshness.CaptureRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	changed := initial
	changed.Dirty = []freshness.DirtyFile{{
		Status: "modified", Path: "main.go", Kind: freshness.FileRegular,
		ContentSHA256: strings.Repeat("f", 64),
	}}
	captures := []freshness.RepositoryState{initial, changed}
	captureCount := 0
	debugDir := t.TempDir()
	err = runDefaultWithDeps(repo, []string{"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			if captureCount >= len(captures) {
				t.Fatalf("unexpected repository capture %d", captureCount+1)
			}
			state := captures[captureCount]
			captureCount++
			return state, nil
		},
	})
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if captureCount != 2 {
		t.Fatalf("repository capture count = %d, want 2", captureCount)
	}
	entries, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDirectories []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			runDirectories = append(runDirectories, entry)
		}
	}
	if len(runDirectories) != 1 {
		t.Fatalf("debug directory entries = %#v, want one run directory", entries)
	}
	manifestPath := filepath.Join(debugDir, runDirectories[0].Name(), report.RunManifestFilename)
	manifest, err := report.ReadRunManifest(filepath.Dir(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Freshness.State != freshness.FreshnessPartiallyStale {
		t.Fatalf("freshness = %#v", manifest.Freshness)
	}
}

func TestRunDefaultStrictSnapshotRejectsChangedAnalyzedInput(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/moving\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)
	initial, err := freshness.CaptureRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	changed := initial
	changed.Dirty = []freshness.DirtyFile{{
		Status: "modified", Path: "main.go", Kind: freshness.FileRegular,
		ContentSHA256: strings.Repeat("f", 64),
	}}
	captures := []freshness.RepositoryState{initial, changed}
	captureCount := 0
	err = runDefaultWithDeps(repo, []string{"--offline", "--strict-snapshot", "--no-open", "--no-serve", "--debug-dir", t.TempDir()}, defaultRunDeps{
		stdout: io.Discard, stderr: io.Discard,
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			state := captures[captureCount]
			captureCount++
			return state, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "strict snapshot is partially_stale") {
		t.Fatalf("strict snapshot error = %v", err)
	}
}

func TestRunDefaultAllowsDirtyExcludedSubmodule(t *testing.T) {
	clearLLMEnv(t)
	submoduleSource := t.TempDir()
	writeFile(t, filepath.Join(submoduleSource, "module.go"), "package platform\n\nconst Value = 1\n")
	runGit(t, submoduleSource, "init", "--quiet")
	runGit(t, submoduleSource, "add", "module.go")
	commitTestRepository(t, submoduleSource)

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/root\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "go.mod", "main.go")
	commitTestRepository(t, repo)
	runGit(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", submoduleSource, "internal/platform")
	runGit(t, repo, "add", ".gitmodules", "internal/platform")
	commitTestRepository(t, repo)
	writeFile(t, filepath.Join(repo, "internal", "platform", "module.go"), "package platform\n\nconst Value = 2\n")

	debugDir := t.TempDir()
	if err := runDefaultWithDeps(repo, []string{"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir}, defaultRunDeps{
		stdout: io.Discard, stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	entries, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDir string
	for _, entry := range entries {
		if entry.IsDir() {
			runDir = filepath.Join(debugDir, entry.Name())
		}
	}
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Freshness.State != freshness.FreshnessFresh || len(manifest.RepositoryState.Submodules) != 1 ||
		!manifest.RepositoryState.Submodules[0].WorktreeModified {
		t.Fatalf("dirty excluded submodule manifest = %#v", manifest)
	}
}

func TestRunDefaultPreservesSubdirectoryAnalysisRoot(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/scoped\n\ngo 1.24\n")
	writeFile(t, filepath.Join(analysisRoot, "main.go"), "package service\n\nfunc Start() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "service/main.go")
	commitTestRepository(t, repository)

	debugDir := t.TempDir()
	if err := runDefaultWithDeps(analysisRoot, []string{"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	entries, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			runDirs = append(runDirs, filepath.Join(debugDir, entry.Name()))
		}
	}
	if len(runDirs) != 1 {
		t.Fatalf("debug directory entries = %#v, want one run directory", entries)
	}
	runDir := runDirs[0]
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}
	wantAnalysisRoot, err := resolveAnalysisRoot(analysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	wantRepository, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.AnalysisRoot != wantAnalysisRoot {
		t.Fatalf("analysis root = %q, want %q", manifest.AnalysisRoot, wantAnalysisRoot)
	}
	if manifest.RepositoryState.Identity != wantRepository.Identity {
		t.Fatalf("repository identity = %q, want %q", manifest.RepositoryState.Identity, wantRepository.Identity)
	}
	if manifest.AnalysisRoot == manifest.RepositoryState.Identity {
		t.Fatal("subdirectory analysis root collapsed to git top-level")
	}
}

func TestRunDefaultServesGeneratedReportAndOpensServerURL(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/server\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	debugDir := t.TempDir()
	var served reportserver.Options
	var opened string
	err := runDefaultWithDeps(repo, []string{"--offline", "--debug-dir", debugDir, "--port", "4321"}, defaultRunDeps{
		ctx:    context.Background(),
		stdout: io.Discard,
		stderr: io.Discard,
		openReport: func(location string) error {
			opened = location
			return nil
		},
		serveReport: func(_ context.Context, opts reportserver.Options) error {
			served = opts
			return opts.OnReady("http://127.0.0.1:4321/runs/fixture/report.html")
		},
	})
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if served.RunsDir != debugDir || served.InitialRunID == "" || served.Port != 4321 {
		t.Fatalf("server options = %#v", served)
	}
	if served.LocationResolver == nil || served.ExactSymbolAnalyzer == nil {
		t.Fatal("interactive report server did not receive the local Go analyzer")
	}
	if opened != "http://127.0.0.1:4321/runs/fixture/report.html" {
		t.Fatalf("opened location = %q", opened)
	}
	metadataJSON, err := os.ReadFile(filepath.Join(debugDir, served.InitialRunID, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(metadataJSON, []byte(filepath.Clean(repo))) {
		t.Fatalf("metadata does not retain absolute repo path: %s", metadataJSON)
	}
}

func TestRunDefaultAcceptsRepositoryAfterFlags(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/flags-first\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")

	var stdout bytes.Buffer
	if err := runDefaultWithDeps(".", []string{"--offline", "--no-debug", repo}, defaultRunDeps{
		stdout: &stdout,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "offline mode") {
		t.Fatalf("stdout does not describe offline run:\n%s", stdout.String())
	}
}

func TestReportOpenCommand(t *testing.T) {
	tests := []struct {
		goos     string
		wantName string
	}{
		{goos: "darwin", wantName: "open"},
		{goos: "linux", wantName: "xdg-open"},
		{goos: "windows", wantName: "rundll32"},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			name, args, err := reportOpenCommand(test.goos, "/tmp/report.html")
			if err != nil {
				t.Fatal(err)
			}
			if name != test.wantName {
				t.Fatalf("command = %q, want %q", name, test.wantName)
			}
			if len(args) == 0 || args[len(args)-1] != "/tmp/report.html" {
				t.Fatalf("args = %v", args)
			}
		})
	}
	if _, _, err := reportOpenCommand("plan9", "/tmp/report.html"); err == nil {
		t.Fatal("unsupported OS did not return an error")
	}
}

func TestRepoRunLabelResolvesCurrentDirectory(t *testing.T) {
	label := repoRunLabel(".")
	if label == "" || label == "." || label == string(filepath.Separator) {
		t.Fatalf("repoRunLabel(.) = %q", label)
	}
}

func TestRunDoctorReportsConfigWithoutSecret(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("REPOMAP_LLM_ENDPOINT", "https://llm.company.example/v1/chat/completions")
	t.Setenv("REPOMAP_LLM_MODEL", "company-model")
	t.Setenv("REPOMAP_LLM_API_KEY", "must-not-be-printed")

	var stdout bytes.Buffer
	if err := runDoctor([]string{"llm"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"LLM configuration OK",
		"endpoint: https://llm.company.example/v1/chat/completions",
		"model: company-model",
		"auth: bearer",
		"network_check: skipped",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "must-not-be-printed") {
		t.Fatal("doctor output contains API key")
	}
}

func TestRunDoctorChecksNoAuthProviderWithoutRepositoryContent(t *testing.T) {
	clearLLMEnv(t)
	var authorization string
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		requestBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"status\":\"ok\"}"}}]}`)
	}))
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")

	var stdout bytes.Buffer
	if err := runDoctor([]string{"llm", "--check"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	if authorization != "" {
		t.Fatalf("Authorization = %q, want none", authorization)
	}
	if strings.Contains(requestBody, "repo_name") || strings.Contains(requestBody, "allowed_paths") {
		t.Fatalf("doctor sent repository content: %s", requestBody)
	}
	if !strings.Contains(stdout.String(), "network_check: passed") {
		t.Fatalf("doctor output = %q", stdout.String())
	}
}

func TestRunDoctorRejectsUnknownTarget(t *testing.T) {
	t.Parallel()

	err := runDoctor([]string{"database"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("runDoctor() error = %v", err)
	}
}

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"REPOMAP_LLM_ENDPOINT",
		"REPOMAP_LLM_MODEL",
		"REPOMAP_LLM_API_KEY",
		"REPOMAP_LLM_MAX_TOKENS",
		"REPOMAP_LLM_TIMEOUT",
		"REPOMAP_LLM_AUTH",
		"DEEPSEEK_ENDPOINT",
		"DEEPSEEK_MODEL",
		"DEEPSEEK_API_KEY",
		"DEEPSEEK_MAX_TOKENS",
		"DEEPSEEK_TIMEOUT",
		"DEEPSEEK_AUTH",
	} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", commandArgs, err, output)
	}
}

func commitTestRepository(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
}
