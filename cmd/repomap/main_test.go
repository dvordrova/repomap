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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/tasklens"
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
		"orientation_json":             deepseek.OrientationPromptVersionJSON,
		"source_json":                  deepseek.SourcePromptVersionJSON,
		"symbol_json":                  deepseek.SymbolPromptVersionJSON,
		"symbol_tagged":                deepseek.SymbolPromptVersionTagged,
		"guided_tour":                  guidedtour.PromptVersion,
		"guided_tour_leaf":             guidedtour.LeafPromptVersion,
		"guided_tour_fan_in":           guidedtour.FanInPromptVersion,
		"semantic_opportunity":         semanticdiscovery.OpportunityPromptVersion,
		"semantic_leaf":                semanticdiscovery.LeafPromptVersion,
		"semantic_fan_in":              semanticdiscovery.FanInPromptVersion,
		"semantic_monolithic":          semanticdiscovery.MonolithicPromptVersion,
		"golden_mechanism":             semanticdiscovery.GoldenMechanismPromptVersion,
		"repository_onboarding_editor": semanticdiscovery.OnboardingEditorPromptVersion,
		"repository_brief_shape":       semanticdiscovery.StudyBriefPromptVersion,
		"study_direction_candidates":   semanticdiscovery.StudyCandidatesPromptVersion,
		"reading_pack_review":          semanticdiscovery.ReadingPackReviewPromptVersion,
		"paved_paths":                  pavedpath.PromptVersion,
		"task_investigation":           tasklens.PromptVersion,
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
			Stage: orient.ProgressSurfacePhase, Phase: "ssa_build", PhaseState: "started",
			Activity: "building SSA once for the loaded package dependency closure",
		},
		{
			Stage: orient.ProgressSurfacePhase, Phase: "ssa_build", PhaseState: "progress",
			CompletedCount: 50, TotalCount: 120, LatencyMillis: 10_000,
		},
		{
			Stage: orient.ProgressSurfacePhase, Phase: "ssa_build", PhaseState: "completed",
			CompletedCount: 120, TotalCount: 120, LatencyMillis: 12_500,
		},
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
		"surface discovery phase ssa_build: building SSA once",
		"surface discovery phase ssa_build: 50/120 after 10s",
		"surface discovery phase ssa_build completed in 12500 ms (120/120)",
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
		body, readErr := io.ReadAll(request.Body)
		if requestCount == 1 {
			requestBody = body
		}
		err = readErr
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
		t.Fatalf("provider request count = %d, want 1; model-only orientation is not semantic evidence", requestCount)
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
		fmt.Sprintf("repomap: prepared %d-byte orientation request for deepseek-v4-flash", len(requestBody)),
		"validated 1 candidate direction(s)",
		"Report: ",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), "discovering local Go runtime surfaces") {
		t.Fatalf("default run unexpectedly enabled slow runtime-surface discovery:\n%s", stderr.String())
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
		body, readErr := io.ReadAll(request.Body)
		if requestCount == 1 {
			requestBody = body
		}
		err = readErr
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
		t.Fatalf("provider request count = %d, want 1; model-only orientation is not semantic evidence", requestCount)
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

func TestRunDefaultNoServeSuppressesServer(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/no-serve\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	served := false
	if err := runDefaultWithDeps(repo, []string{
		"--offline", "--no-open", "--no-serve", "--debug-dir", t.TempDir(),
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
		serveReport: func(context.Context, reportserver.Options) error {
			served = true
			return nil
		},
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if served {
		t.Fatal("report server was started with --no-serve")
	}
}

func TestRunDefaultSurfaceDiscoveryIsOptIn(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/surface-opt-in\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "default disabled", want: false},
		{name: "explicit opt-in", args: []string{"--discover-surfaces=true"}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			debugDir := t.TempDir()
			args := append([]string{
				"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir,
			}, test.args...)
			if err := runDefaultWithDeps(repo, args, defaultRunDeps{
				stdout: io.Discard,
				stderr: io.Discard,
			}); err != nil {
				t.Fatalf("runDefaultWithDeps() error = %v", err)
			}

			runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
			if err != nil {
				t.Fatal(err)
			}
			metadataJSON, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
			if err != nil {
				t.Fatal(err)
			}
			var metadata struct {
				EffectiveOptions struct {
					DiscoverSurfaces bool `json:"discover_surfaces"`
				} `json:"effective_options"`
			}
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
				t.Fatal(err)
			}
			if got := metadata.EffectiveOptions.DiscoverSurfaces; got != test.want {
				t.Fatalf("discover_surfaces = %t, want %t; metadata: %s", got, test.want, metadataJSON)
			}
		})
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
	if served.SourceEpisodeJSON != nil {
		t.Fatalf("ordinary report unexpectedly received %d source episode bytes", len(served.SourceEpisodeJSON))
	}
	if opened != "http://127.0.0.1:4321/runs/fixture/report.html#/overview" {
		t.Fatalf("opened location = %q", opened)
	}
	metadataJSON, err := os.ReadFile(filepath.Join(debugDir, served.InitialRunID, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(metadataJSON, []byte(filepath.Clean(repo))) {
		t.Fatalf("metadata does not retain absolute repo path: %s", metadataJSON)
	}
	reportHTML, err := os.ReadFile(filepath.Join(debugDir, served.InitialRunID, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reportHTML, []byte(`"source_episode":`)) {
		t.Fatal("ordinary report unexpectedly contains a source episode")
	}
}

func TestRunDefaultSourceEpisodeGeneratesStaticHTMLAndPassesServerBytes(t *testing.T) {
	clearLLMEnv(t)
	sourceEpisodePath, sourceEpisodeJSON, revision, episodeID, question := sourceEpisodeCLIFixture(t)

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/source-episode\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	captureAtAcceptedRevision := func(ctx context.Context, root string) (freshness.RepositoryState, error) {
		state, err := freshness.CaptureRepository(ctx, root)
		if err == nil {
			state.Head = revision
		}
		return state, err
	}
	debugDir := t.TempDir()
	var served reportserver.Options
	err := runDefaultWithDeps(repo, []string{
		"--offline",
		"--no-open",
		"--debug-dir", debugDir,
		"--source-episode", sourceEpisodePath,
	}, defaultRunDeps{
		ctx:         context.Background(),
		stdout:      io.Discard,
		stderr:      io.Discard,
		captureRepo: captureAtAcceptedRevision,
		serveReport: func(_ context.Context, options reportserver.Options) error {
			served = options
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if served.InitialRunID == "" {
		t.Fatal("source episode run was not selected for the report server")
	}
	if !bytes.Equal(served.SourceEpisodeJSON, sourceEpisodeJSON) {
		t.Fatal("report server did not receive the exact accepted source episode bytes")
	}

	runDir := filepath.Join(debugDir, served.InitialRunID)
	reportHTML, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"source_episode":`, episodeID, question} {
		if !bytes.Contains(reportHTML, []byte(required)) {
			t.Fatalf("generated static HTML is missing %q", required)
		}
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{sourceEpisodePath, episodeID, question} {
		if bytes.Contains(reportJSON, []byte(forbidden)) {
			t.Fatalf("persisted report recorded transient source episode value %q", forbidden)
		}
	}
	if bytes.Contains(reportHTML, []byte(sourceEpisodePath)) {
		t.Fatal("generated HTML recorded the source episode input path")
	}
}

func TestRunServeSourceEpisodeRequiresSelectedRunAndPassesBytes(t *testing.T) {
	sourceEpisodePath, sourceEpisodeJSON, _, _, _ := sourceEpisodeCLIFixture(t)
	var served reportserver.Options
	serve := func(_ context.Context, options reportserver.Options) error {
		served = options
		return nil
	}
	err := runServeWithDeps(
		context.Background(),
		[]string{"--debug-dir", t.TempDir(), "--source-episode", sourceEpisodePath, "--no-open"},
		io.Discard,
		nil,
		serve,
	)
	if err == nil || !strings.Contains(err.Error(), "--source-episode requires --run") {
		t.Fatalf("serve without selected run error = %v", err)
	}
	if served.SourceEpisodeJSON != nil {
		t.Fatal("serve was called after rejecting a source episode without --run")
	}

	err = runServeWithDeps(
		context.Background(),
		[]string{
			"--debug-dir", t.TempDir(),
			"--run", "accepted-fixture-run",
			"--source-episode", sourceEpisodePath,
			"--no-open",
		},
		io.Discard,
		nil,
		serve,
	)
	if err != nil {
		t.Fatalf("runServeWithDeps() error = %v", err)
	}
	if served.InitialRunID != "accepted-fixture-run" {
		t.Fatalf("selected run = %q", served.InitialRunID)
	}
	if !bytes.Equal(served.SourceEpisodeJSON, sourceEpisodeJSON) {
		t.Fatal("serve did not receive the exact source episode bytes")
	}
}

func TestSourceEpisodeCLIInputFailsClosed(t *testing.T) {
	sourceEpisodePath, _, _, _, _ := sourceEpisodeCLIFixture(t)
	t.Run("default requires generated run", func(t *testing.T) {
		err := runDefaultWithDeps(t.TempDir(), []string{
			"--offline",
			"--no-debug",
			"--source-episode", sourceEpisodePath,
		}, defaultRunDeps{stdout: io.Discard, stderr: io.Discard})
		if err == nil || !strings.Contains(err.Error(), "--source-episode requires a generated report run") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		inputPath := filepath.Join(t.TempDir(), "oversized.json")
		if err := os.WriteFile(inputPath, bytes.Repeat([]byte("x"), report.MaxSourceEpisodeBytes+1), 0o600); err != nil {
			t.Fatal(err)
		}
		err := runDefaultWithDeps(t.TempDir(), []string{
			"--offline",
			"--debug-dir", t.TempDir(),
			"--source-episode", inputPath,
		}, defaultRunDeps{stdout: io.Discard, stderr: io.Discard})
		if err == nil || !strings.Contains(err.Error(), "exceeds the") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		inputDir := t.TempDir()
		targetPath := filepath.Join(inputDir, "episode.json")
		writeFile(t, targetPath, "{}")
		linkPath := filepath.Join(inputDir, "episode-link.json")
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Skipf("create symlink: %v", err)
		}
		err := runDefaultWithDeps(t.TempDir(), []string{
			"--offline",
			"--debug-dir", t.TempDir(),
			"--source-episode", linkPath,
		}, defaultRunDeps{stdout: io.Discard, stderr: io.Discard})
		if err == nil || !strings.Contains(err.Error(), "must be a regular file") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unapproved artifact before orientation", func(t *testing.T) {
		inputPath := filepath.Join(t.TempDir(), "unapproved.json")
		writeFile(t, inputPath, "{}")
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/source-episode-preflight\n\ngo 1.24\n")
		runGit(t, repo, "init", "--quiet")
		runGit(t, repo, "add", "--", "go.mod")
		commitTestRepository(t, repo)
		captures := 0
		err := runDefaultWithDeps(repo, []string{
			"--debug-dir", t.TempDir(),
			"--source-episode", inputPath,
		}, defaultRunDeps{
			stdout: io.Discard,
			stderr: io.Discard,
			captureRepo: func(_ context.Context, root string) (freshness.RepositoryState, error) {
				captures++
				return freshness.RepositoryState{
					Version:  freshness.RepositoryStateVersion,
					Identity: root,
					Head:     strings.Repeat("a", 40),
					Dirty:    []freshness.DirtyFile{},
				}, nil
			},
		})
		if err == nil || !strings.Contains(err.Error(), "before orientation") {
			t.Fatalf("error = %v", err)
		}
		if captures != 1 {
			t.Fatalf("repository captures = %d, want exactly the initial preflight capture", captures)
		}
	})
}

func TestRunDefaultAcceptsRepositoryAfterFlags(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/flags-first\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")

	var stdout bytes.Buffer
	if err := runDefaultWithDeps(".", []string{"--offline", "--no-search", "--no-debug", repo}, defaultRunDeps{
		stdout: &stdout,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "offline mode") {
		t.Fatalf("stdout does not describe offline run:\n%s", stdout.String())
	}
}

func TestRunDefaultNoSearchOmitsSearchFromSavedReport(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/no-search\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	debugDir := t.TempDir()
	if err := runDefaultWithDeps(".", []string{
		"--offline",
		"--discover-surfaces=false",
		"--no-search",
		"--no-open",
		"--no-serve",
		"--debug-dir", debugDir,
		repo,
	}, defaultRunDeps{
		ctx:    context.Background(),
		stdout: io.Discard,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}

	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		EffectiveOptions struct {
			NoSearch bool `json:"no_search"`
		} `json:"effective_options"`
	}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.EffectiveOptions.NoSearch {
		t.Fatalf("metadata effective options did not retain --no-search: %s", metadataJSON)
	}

	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(reportJSON, []byte(`"semantic_search_disabled": true`)) ||
		bytes.Contains(reportJSON, []byte(`"semantic_search":`)) {
		t.Fatalf("saved report does not honor --no-search: %s", reportJSON)
	}
	reportHTML, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range [][]byte{
		[]byte(`id="rm-semantic-search"`),
		[]byte(`id="rm-semantic-search-css"`),
		[]byte(`id="rm-semantic-search-js"`),
		[]byte(`<kbd>⌘/Ctrl K</kbd>`),
	} {
		if bytes.Contains(reportHTML, marker) {
			t.Fatalf("--no-search report unexpectedly contains %q", marker)
		}
	}
}

func TestRunDefaultNoSearchPreservesModelCallPlan(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "internal/a"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "internal/b"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/no-search-plan\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nimport \"example.com/no-search-plan/internal/a\"\n\nfunc main() { a.Run() }\n")
	writeFile(t, filepath.Join(repo, "internal/a/a.go"), "package a\n\nimport \"example.com/no-search-plan/internal/b\"\n\nfunc Run() { b.Run() }\n")
	writeFile(t, filepath.Join(repo, "internal/b/b.go"), "package b\n\nfunc Run() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go", "internal/a/a.go", "internal/b/b.go")
	commitTestRepository(t, repo)

	orientationJSON, err := json.Marshal(map[string]any{
		"project_guess": "three-stage Go command",
		"confidence":    0.9,
		"high_level_map": []any{
			map[string]any{"name": "command", "evidence": []string{"main.go"}, "why_it_matters": "starts the command"},
			map[string]any{"name": "service", "evidence": []string{"internal/a/a.go"}, "why_it_matters": "coordinates work"},
			map[string]any{"name": "worker", "evidence": []string{"internal/b/b.go"}, "why_it_matters": "finishes work"},
		},
		"first_files_to_open": []any{
			map[string]any{"path": "main.go", "reason": "entrypoint"},
			map[string]any{"path": "internal/a/a.go", "reason": "coordination"},
			map[string]any{"path": "internal/b/b.go", "reason": "terminal work"},
		},
		"candidate_flows": []any{map[string]any{
			"name":              "Command startup",
			"trigger":           "the executable starts",
			"likely_entrypoint": "main.go",
			"likely_files":      []string{"main.go", "internal/a/a.go", "internal/b/b.go"},
			"why_interesting":   "connects startup to the terminal worker",
			"evidence":          []string{"main.go", "internal/a/a.go", "internal/b/b.go"},
			"confidence":        0.9,
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []any{},
		"unverified_paths":       []any{},
		"warnings":               []any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	run := func(noSearch bool) [][]byte {
		t.Helper()
		var mu sync.Mutex
		var requests [][]byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			body, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				t.Errorf("read request: %v", readErr)
				return
			}
			mu.Lock()
			requests = append(requests, bytes.Clone(body))
			mu.Unlock()
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

		args := []string{
			"--discover-surfaces=false",
			"--no-open",
			"--no-serve",
			"--debug-dir", t.TempDir(),
		}
		if noSearch {
			args = append(args, "--no-search")
		}
		if err := runDefaultWithDeps(repo, args, defaultRunDeps{
			ctx: context.Background(), stdout: io.Discard, stderr: io.Discard,
		}); err != nil {
			t.Fatalf("runDefaultWithDeps(noSearch=%t) error = %v", noSearch, err)
		}

		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), requests...)
	}

	withSearch := run(false)
	withoutSearch := run(true)
	if !reflect.DeepEqual(withoutSearch, withSearch) {
		t.Fatalf("--no-search changed model request plan\nwith search: %q\nwithout search: %q", withSearch, withoutSearch)
	}
	if len(withSearch) != 4 {
		t.Fatalf("model request count = %d, want orientation, architecture, guided tour, and repository study map", len(withSearch))
	}
	wantStageMarkers := []string{
		"senior software engineer helping orient",
		"compact conceptual architecture landscape",
		"optional editorial guide for one bounded repository tour",
		"editorial onboarding planner for one bounded repository model",
	}
	for index, marker := range wantStageMarkers {
		if !bytes.Contains(withSearch[index], []byte(marker)) {
			t.Fatalf("model request %d does not contain stage marker %q: %s", index, marker, withSearch[index])
		}
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

func TestReportOverviewURL(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		location string
		want     string
	}{
		{
			name:     "plain report",
			location: "http://127.0.0.1:4321/runs/fixture/report.html",
			want:     "http://127.0.0.1:4321/runs/fixture/report.html#/overview",
		},
		{
			name:     "replace existing route",
			location: "http://127.0.0.1:4321/runs/fixture/report.html#/mechanisms",
			want:     "http://127.0.0.1:4321/runs/fixture/report.html#/overview",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := reportOverviewURL(test.location); got != test.want {
				t.Fatalf("reportOverviewURL(%q) = %q, want %q", test.location, got, test.want)
			}
		})
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

func sourceEpisodeCLIFixture(t *testing.T) (path string, raw []byte, revision, episodeID, question string) {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(
		"..", "..", "experiments", "source-episode", "django-atomic", "episode.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		EpisodeID  string `json:"episode_id"`
		Question   string `json:"question"`
		Repository struct {
			Revision string `json:"revision"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return path, raw, fixture.Repository.Revision, fixture.EpisodeID, fixture.Question
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
