package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"time"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/tasklens"
)

func TestRemovedOrdinaryProductFlagsAreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"--dump-llm"},
		{"--json"},
		{"--preview-request"},
		{"--out", "preview.json"},
		{"--flows", "1"},
		{"--guided-tour=false"},
		{"--no-debug"},
		{"--no-frameworks"},
		{"--discover-surfaces=false"},
	} {
		t.Run(args[0], func(t *testing.T) {
			var stderr bytes.Buffer
			err := runDefaultWithDeps(
				t.TempDir(),
				args,
				defaultRunDeps{stdout: io.Discard, stderr: &stderr},
			)
			if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
				t.Fatalf("removed flag %q error = %v", args[0], err)
			}
		})
	}
}

func TestDirectCallGraphFlagsRejectInvalidBoundsBeforeAnalysis(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"--depth", "0"}, want: "--depth must be at least 1"},
		{args: []string{"--edges-limit", "0"}, want: "--edges-limit must be between 1 and"},
	} {
		var stderr bytes.Buffer
		err := runDefaultWithDeps(t.TempDir(), test.args, defaultRunDeps{
			stdout: io.Discard, stderr: &stderr,
		})
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("args %v error = %v, want %q", test.args, err, test.want)
		}
	}
}

func TestDirectCallEdgeCeilingOffersTwoRealRerunOptions(t *testing.T) {
	err := directCallEdgeCeilingError("cmd/app", 10, 10_000, 4)
	for _, want := range []string{
		"exceeded --edges-limit=10000 at --depth=10",
		"decrease depth via --depth 4 (default 10;",
		"this depth is known to fit",
		"to preserve depth, try --edges-limit 20000 (default 10000",
		"full edge count is not computed",
		"rebuilds local SSA",
		"Go build cache remains reusable",
		"no provider call was made",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ceiling error omitted %q:\n%s", want, err)
		}
	}
	rootOverflow := directCallEdgeCeilingError("cmd/app", 10, 1, 0)
	if strings.Contains(rootOverflow.Error(), "decrease depth") {
		t.Fatalf("root-layer overflow offered a false depth recovery:\n%s", rootOverflow)
	}
}

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
		"localization":                 localization.PromptVersion,
		"localization_request":         deepseek.LocalizationRequestVersion,
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

func TestWriteStudyMapCompletionKeepsTypedLocalOutcome(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{
		string(studyMapNoSupportedSourceAdapter),
		string(studyMapNoEligibleSourceFunctions),
	} {
		var output bytes.Buffer
		writeStudyMapCompletion(
			&output,
			studyMapStatus{State: "failed", FailureReason: reason},
			125*time.Millisecond,
		)
		want := "decision study: published=0 provider_calls=0 reason=" + reason +
			" (after 125 ms)"
		if !strings.Contains(output.String(), want) {
			t.Fatalf("typed Study outcome %q was hidden:\n%s", reason, output.String())
		}
		if strings.Contains(output.String(), "selected 0 source-backed") {
			t.Fatalf("typed Study outcome %q used success copy:\n%s", reason, output.String())
		}
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
			CandidateCount: 3, RejectedCount: 1, Cached: true,
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
		"3 candidate direction(s) accepted, 1 rejected",
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
		[]string{"--debug-dir", t.TempDir(), "--no-open", "--no-serve"},
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
	if got := userError.String(); got != "Run:\n  state: canceled\n" {
		t.Fatalf("cancellation message = %q", got)
	}
	if got := defaultRunExitCode(err); got != 130 {
		t.Fatalf("cancellation exit code = %d, want 130", got)
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

func TestRunDefaultOfflinePublicationIsDegradedWithClosedSemanticStages(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/offline-publication\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	debugDir := t.TempDir()
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(repo, []string{
		"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir,
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: &stderr,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}

	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatalf("resolve published run: %v", err)
	}
	if _, err := report.ReadRunManifest(runDir); err != nil {
		t.Fatalf("read published manifest: %v", err)
	}
	if _, err := report.ReadRunDir(runDir); err != nil {
		t.Fatalf("read published report: %v", err)
	}
	if err := verifyPublishedHTML(filepath.Join(runDir, "report.html")); err != nil {
		t.Fatalf("verify published report HTML: %v", err)
	}

	finalIndex := strings.LastIndex(stderr.String(), "Run:\n")
	if finalIndex < 0 {
		t.Fatalf("offline run missing terminal publication state:\n%s", stderr.String())
	}
	final := stderr.String()[finalIndex:]
	wantPrefix := "Run:\n  state: degraded\n  report: "
	wantDetails := "\n" +
		"  Architecture: model grouping unavailable; the exact local Map remains available\n" +
		"  Study: unavailable\n"
	if !strings.HasPrefix(final, wantPrefix) || !strings.HasSuffix(final, wantDetails) {
		t.Fatalf("offline run missing degraded terminal publication details:\n%s", stderr.String())
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

func TestRunDefaultGitLabURLCreatesStandalonePinnedReport(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/static-gitlab\n\ngo 1.24\n")
	writeFile(t, filepath.Join(analysisRoot, "main.go"), "package service\n\nfunc Start() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "service/main.go")
	commitTestRepository(t, repository)
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}

	debugDir := t.TempDir()
	var openedPath string
	served := false
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(analysisRoot, []string{
		"--offline",
		"--debug-dir", debugDir,
		"--gitlab-url", "https://gitlab.example.test/group/project.git",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: &stderr,
		openReport: func(path string) error {
			openedPath = path
			return nil
		},
		serveReport: func(context.Context, reportserver.Options) error {
			served = true
			return nil
		},
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	if served {
		t.Fatal("--gitlab-url started the local report server")
	}

	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(runDir, "report.html")
	resolvedOpenedPath, err := filepath.EvalSymlinks(openedPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedOpenedPath != reportPath {
		t.Fatalf("opened path = %q, want %q", resolvedOpenedPath, reportPath)
	}
	html, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"repository_url":"https://gitlab.example.test/group/project"`,
		`"revision":"` + state.Head + `"`,
		`"path_prefix":"service"`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("standalone report missing %q", want)
		}
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reportJSON), "gitlab_source_links") {
		t.Fatalf("canonical report contains HTML-only GitLab config: %s", reportJSON)
	}
	metadataJSON, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		EffectiveOptions debugdump.EffectiveOptions `json:"effective_options"`
	}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.EffectiveOptions.NoServe ||
		metadata.EffectiveOptions.GitLabURL != "https://gitlab.example.test/group/project" {
		t.Fatalf("effective GitLab options = %#v", metadata.EffectiveOptions)
	}
	if !strings.Contains(stderr.String(), "standalone host: GitLab") ||
		!strings.Contains(stderr.String(), "captured revision: "+state.Head) {
		t.Fatalf("stderr missing pinned revision:\n%s", stderr.String())
	}
}

func TestRunDefaultGitHubURLCreatesStandalonePinnedReport(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/static-github\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repository)
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}

	debugDir := t.TempDir()
	served := false
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(repository, []string{
		"--offline",
		"--no-open",
		"--debug-dir", debugDir,
		"--github-url", "https://github.com/example/static-github.git",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: &stderr,
		serveReport: func(context.Context, reportserver.Options) error {
			served = true
			return nil
		},
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	if served {
		t.Fatal("--github-url started the local report server")
	}

	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"github_source_links"`,
		`"repository_url":"https://github.com/example/static-github"`,
		`"revision":"` + state.Head + `"`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("standalone report missing %q", want)
		}
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reportJSON), "github_source_links") {
		t.Fatalf("canonical report contains HTML-only GitHub config: %s", reportJSON)
	}
	metadataJSON, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		EffectiveOptions debugdump.EffectiveOptions `json:"effective_options"`
	}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.EffectiveOptions.NoServe ||
		metadata.EffectiveOptions.GitHubURL != "https://github.com/example/static-github" {
		t.Fatalf("effective GitHub options = %#v", metadata.EffectiveOptions)
	}
	if !strings.Contains(stderr.String(), "standalone host: GitHub") ||
		!strings.Contains(stderr.String(), "captured revision: "+state.Head) {
		t.Fatalf("stderr missing pinned revision:\n%s", stderr.String())
	}
}

func TestRunDefaultRejectsMultipleStandaloneSourceHosts(t *testing.T) {
	err := runDefaultWithDeps(t.TempDir(), []string{
		"--offline",
		"--gitlab-url", "https://gitlab.example.test/team/project",
		"--github-url", "https://github.com/team/project",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("multiple source hosts error = %v", err)
	}
}

func TestRunDefaultGitLabURLRejectsInvalidURLBeforeRepositoryCapture(t *testing.T) {
	clearLLMEnv(t)
	captured := false
	err := runDefaultWithDeps(t.TempDir(), []string{
		"--offline",
		"--gitlab-url", "ssh://git@gitlab.example.test/group/project.git",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			captured = true
			return freshness.RepositoryState{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "absolute http(s) project URL") {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if captured {
		t.Fatal("invalid --gitlab-url reached repository capture")
	}
}

func TestRunDefaultGitLabURLCreatesStandaloneReportFromStableDirtyRepository(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/dirty-static\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repository)
	committed, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	const dirtySourceMarker = "repomap-dirty-source-must-not-be-embedded-4f764179"
	const uncapturedDirtyMarker = "repomap-uncaptured-dirty-must-not-enter-report-8ddf6621"
	dirtySource := "package main\n\nfunc main() { println(\"" + dirtySourceMarker + "\") }\n"
	writeFile(
		t,
		filepath.Join(repository, "main.go"),
		dirtySource,
	)
	writeFile(t, filepath.Join(repository, "private-untracked.txt"), uncapturedDirtyMarker+"\n")

	debugDir := t.TempDir()
	var stderr bytes.Buffer
	err = runDefaultWithDeps(repository, []string{
		"--offline",
		"--no-open",
		"--debug-dir", debugDir,
		"--gitlab-url", "https://gitlab.example.test/group/project",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte(`"working_tree_dirty":true`)) {
		t.Fatalf("standalone report did not disclose the stable dirty checkout")
	}
	if !bytes.Contains(html, []byte(`"revision":"`+committed.Head+`"`)) {
		t.Fatalf("standalone report is not pinned to the captured HEAD")
	}
	if bytes.Contains(html, []byte(dirtySourceMarker)) {
		t.Fatalf("standalone report embedded dirty source content")
	}
	if bytes.Contains(html, []byte(uncapturedDirtyMarker)) {
		t.Fatalf("standalone report embedded uncaptured dirty content")
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"gitlab_source_links",
		"working_tree_dirty",
		"working_tree_paths",
		uncapturedDirtyMarker,
	} {
		if bytes.Contains(reportJSON, []byte(forbidden)) {
			t.Fatalf("canonical report contains HTML-only or uncaptured source data %q", forbidden)
		}
	}
	if !bytes.Contains(reportJSON, []byte(dirtySourceMarker)) {
		t.Fatal("canonical report lost the exact manifest-authorized dirty source excerpt")
	}
	var canonical report.ReportData
	if err := json.Unmarshal(reportJSON, &canonical); err != nil {
		t.Fatal(err)
	}
	withoutSources := canonical
	withoutSources.UserSources = nil
	withoutSourceJSON, err := json.Marshal(withoutSources)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(withoutSourceJSON, []byte(dirtySourceMarker)) {
		t.Fatal("dirty source bytes escaped the exact UserSources projection")
	}
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("manifest does not authorize canonical report: %v", err)
	}
	if len(manifest.RepositoryState.Dirty) != 2 {
		t.Fatalf("manifest dirty repository state = %#v", manifest.RepositoryState.Dirty)
	}
	catalog, err := manifest.SourceCatalog()
	if err != nil {
		t.Fatal(err)
	}
	mainSource, ok := catalog.Lookup("main.go")
	if !ok {
		t.Fatal("manifest source catalog does not authorize main.go")
	}
	if _, ok := catalog.Lookup("private-untracked.txt"); ok {
		t.Fatal("manifest source catalog authorized an untracked dirty file")
	}
	var dirtyMainDigest string
	for _, dirty := range manifest.RepositoryState.Dirty {
		if dirty.Path == "main.go" {
			dirtyMainDigest = dirty.ContentSHA256
		}
	}
	if dirtyMainDigest == "" || mainSource.ContentSHA256 != dirtyMainDigest {
		t.Fatalf("manifest-authorized main.go digest = %q, dirty capture = %q",
			mainSource.ContentSHA256, dirtyMainDigest)
	}
	foundAuthorizedDirtySource := false
	for index, source := range canonical.UserSources {
		if err := source.Validate(); err != nil {
			t.Fatalf("canonical UserSources[%d] is invalid: %v", index, err)
		}
		if _, ok := catalog.Lookup(source.Path); !ok {
			t.Fatalf("canonical UserSources[%d] path %q is not manifest-authorized", index, source.Path)
		}
		if source.Revision != manifest.RepositoryState.Head {
			t.Fatalf("canonical UserSources[%d] revision = %q, want %q",
				index, source.Revision, manifest.RepositoryState.Head)
		}
		if strings.Contains(source.Content, dirtySourceMarker) {
			if source.Path != "main.go" {
				t.Fatalf("dirty source marker attached to %q", source.Path)
			}
			foundAuthorizedDirtySource = true
		}
	}
	if !foundAuthorizedDirtySource {
		t.Fatal("manifest-authorized dirty source marker is absent from exact UserSources")
	}
	if !strings.Contains(stderr.String(), "report contains stable local changes") ||
		!strings.Contains(stderr.String(), "changed inputs: 2") ||
		!strings.Contains(stderr.String(), "changed source paths are local-only") {
		t.Fatalf("stderr missing dirty report explanation:\n%s", stderr.String())
	}
}

func TestRunDefaultGitLabURLRejectsWorkingTreeChangesDuringAnalysis(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/moving-static\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repository)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "analyzed source", path: "main.go"},
		{name: "unrelated file", path: "notes.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := initial
			changed.Dirty = []freshness.DirtyFile{{
				Status: "modified", Path: test.path, Kind: freshness.FileRegular,
				ContentSHA256: strings.Repeat("f", 64),
			}}
			captures := []freshness.RepositoryState{initial, changed}
			captureCount := 0
			debugDir := t.TempDir()
			err := runDefaultWithDeps(repository, []string{
				"--offline",
				"--no-open",
				"--debug-dir", debugDir,
				"--gitlab-url", "https://gitlab.example.test/group/project",
			}, defaultRunDeps{
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
			if err == nil || !strings.Contains(err.Error(), "working tree to remain unchanged") {
				t.Fatalf("runDefaultWithDeps() error = %v", err)
			}
			if captureCount != 2 {
				t.Fatalf("repository capture count = %d, want 2", captureCount)
			}
			entries, readErr := os.ReadDir(debugDir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				html, readErr := os.ReadFile(filepath.Join(debugDir, entry.Name(), "report.html"))
				if os.IsNotExist(readErr) {
					continue
				}
				if readErr != nil {
					t.Fatal(readErr)
				}
				if bytes.Contains(html, []byte(`"gitlab_source_links"`)) {
					t.Fatal("failed run published standalone GitLab routing")
				}
			}
		})
	}
}

func TestRunDefaultGitLabHostOnlyURLUsesRepositoryOrigin(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/static-gitlab\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repository)
	runGit(t, repository, "remote", "add", "origin", "git@gitlab.example.test:group/subgroup/project.git")

	debugDir := t.TempDir()
	if err := runDefaultWithDeps(repository, []string{
		"--offline",
		"--no-open",
		"--debug-dir", debugDir,
		"--gitlab-url", "https://gitlab.example.test",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte(
		`"repository_url":"https://gitlab.example.test/group/subgroup/project"`,
	)) {
		t.Fatal("standalone report did not infer the GitLab project from origin")
	}
}

func TestRunDefaultGitLabHostOnlyURLDoesNotGuessWithoutOrigin(t *testing.T) {
	clearLLMEnv(t)
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/static-gitlab\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "main.go"), "package main\n\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repository)
	runGit(t, repository, "remote", "add", "alpha", "git@gitlab.example.test:alpha/project.git")
	runGit(t, repository, "remote", "add", "beta", "git@gitlab.example.test:beta/project.git")

	captured := false
	err := runDefaultWithDeps(repository, []string{
		"--offline",
		"--gitlab-url", "https://gitlab.example.test",
	}, defaultRunDeps{
		stdout: io.Discard,
		stderr: io.Discard,
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			captured = true
			return freshness.RepositoryState{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "origin remote") {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if captured {
		t.Fatal("ambiguous host-only URL reached repository capture")
	}
}

func TestRunDefaultSurfaceDiscoveryUsesSingleCorePath(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/surface-opt-in\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	for _, test := range []struct {
		name         string
		args         []string
		wantSurfaces bool
	}{
		{name: "default enabled", wantSurfaces: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			debugDir := t.TempDir()
			var stderr bytes.Buffer
			args := append([]string{
				"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir,
			}, test.args...)
			if err := runDefaultWithDeps(repo, args, defaultRunDeps{
				stdout: io.Discard,
				stderr: &stderr,
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
			if got := metadata.EffectiveOptions.DiscoverSurfaces; got != test.wantSurfaces {
				t.Fatalf("discover_surfaces = %t, want %t; metadata: %s", got, test.wantSurfaces, metadataJSON)
			}
			if bytes.Contains(metadataJSON, []byte(`"no_frameworks"`)) {
				t.Fatalf("removed framework mode leaked into metadata: %s", metadataJSON)
			}
			if strings.Contains(stderr.String(), "Framework semantics:") {
				t.Fatalf("ordinary generic-only discovery leaked experiment chrome: %s", stderr.String())
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
	var stderr bytes.Buffer
	err = runDefaultWithDeps(repo, []string{"--offline", "--strict-snapshot", "--no-open", "--no-serve", "--debug-dir", t.TempDir()}, defaultRunDeps{
		stdout: io.Discard, stderr: &stderr,
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			state := captures[captureCount]
			captureCount++
			return state, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "strict snapshot is partially_stale") {
		t.Fatalf("strict snapshot error = %v", err)
	}
	if !strings.Contains(stderr.String(), "Run:\n  state: failed\n  report publication did not complete") {
		t.Fatalf("failed run did not emit terminal publication state:\n%s", stderr.String())
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
	if opened != "http://127.0.0.1:4321/runs/fixture/report.html#/map" {
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
	// The approved source episode pins an immutable external revision whose
	// tree is intentionally absent from this local fixture. Keep the local
	// analysis inputs dirty so CaptureInputs authorizes their exact captured
	// content hashes before the test substitutes that external HEAD.
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/source-episode\n\ngo 1.24\n\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n\n")

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
	commitTestRepository(t, repo)

	var stdout bytes.Buffer
	debugDir := t.TempDir()
	if err := runDefaultWithDeps(".", []string{
		"--offline", "--no-open", "--no-serve", "--debug-dir", debugDir, repo,
	}, defaultRunDeps{
		stdout: &stdout,
		stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("ordinary run wrote stale machine stdout:\n%s", stdout.String())
	}
	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "report.json")); err != nil {
		t.Fatalf("authoritative report.json is unavailable: %v", err)
	}
}

func TestRunDefaultOmitsSearchFromSavedReport(t *testing.T) {
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
		"--no-cache",
		"--depth", "7",
		"--edges-limit", "12345",
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
			NoCache             bool `json:"no_cache"`
			DirectCallDepth     int  `json:"direct_call_depth"`
			DirectCallEdgeLimit int  `json:"direct_call_edge_limit"`
		} `json:"effective_options"`
	}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(metadataJSON, []byte(`"no_search"`)) {
		t.Fatalf("metadata retained the removed Search option: %s", metadataJSON)
	}
	if !metadata.EffectiveOptions.NoCache {
		t.Fatalf("metadata effective options did not retain --no-cache: %s", metadataJSON)
	}
	if metadata.EffectiveOptions.DirectCallDepth != 7 || metadata.EffectiveOptions.DirectCallEdgeLimit != 12345 {
		t.Fatalf("metadata lost direct-call bounds: %#v", metadata.EffectiveOptions)
	}

	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reportJSON, []byte(`"semantic_search_disabled":`)) ||
		bytes.Contains(reportJSON, []byte(`"semantic_search":`)) ||
		bytes.Contains(reportJSON, []byte(`"report_language":`)) {
		t.Fatalf("saved report retained Search: %s", reportJSON)
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
			t.Fatalf("ordinary report unexpectedly contains Search marker %q", marker)
		}
	}
}

func TestRunDefaultNoSecretsIsScopedAndRecorded(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/no-secrets\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(repo, "README.md"), "API_KEY=actual-secret-value\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go", "README.md")
	commitTestRepository(t, repo)

	debugDir := t.TempDir()
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(repo, []string{
		"--offline",
		"--no-secrets",
		"--no-open",
		"--no-serve",
		"--debug-dir", debugDir,
	}, defaultRunDeps{
		ctx:    context.Background(),
		stdout: io.Discard,
		stderr: &stderr,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "ordinary input credential detection is disabled") ||
		!strings.Contains(stderr.String(), "mandatory provider-response and persisted-artifact scans remain active") ||
		!strings.Contains(stderr.String(), "selected tracked source may reach the model provider and debug artifacts") {
		t.Fatalf("unsafe override warning is absent:\n%s", stderr.String())
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
			NoSecrets bool `json:"no_secrets"`
		} `json:"effective_options"`
	}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.EffectiveOptions.NoSecrets {
		t.Fatalf("metadata does not retain --no-secrets: %s", metadataJSON)
	}
	manifestJSON, err := os.ReadFile(filepath.Join(runDir, report.RunManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	runManifest, err := report.DecodeRunManifest(manifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runManifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("run manifest does not bind Atlas-first report: %v", err)
	}
	reportHTML, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"metadata": metadataJSON,
		"manifest": manifestJSON,
		"report":   reportJSON,
		"html":     reportHTML,
	} {
		if bytes.Contains(content, []byte("actual-secret-value")) {
			t.Fatalf("%s leaked the credential assignment", name)
		}
	}
	for _, legacy := range []string{"llm_bundle.json", "orientation_context_selection.v2.json"} {
		if _, err := os.Stat(filepath.Join(runDir, legacy)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Atlas-first offline run wrote removed raw Orientation artifact %s: %v", legacy, err)
		}
	}
	if kind, found := secretscan.Detect("API_KEY=actual-secret-value"); !found || kind != "credential assignment" {
		t.Fatalf("credential detection was not restored after run: %q, %v", kind, found)
	}
}

func TestRunDefaultOfflineRussianRequestKeepsUILocaleWithoutModelLocalization(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/russian-report\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	debugDir := t.TempDir()
	if err := runDefaultWithDeps(repo, []string{
		"--offline",
		"--lang", "ru",
		"--no-open",
		"--no-serve",
		"--debug-dir", debugDir,
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
	if !bytes.Contains(metadataJSON, []byte(`"report_language": "ru"`)) {
		t.Fatalf("metadata did not retain --lang ru: %s", metadataJSON)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reportJSON, []byte(`"report_language"`)) {
		t.Fatalf("canonical report leaked requested locale: %s", reportJSON)
	}
	if _, err := os.Stat(filepath.Join(runDir, report.PresentationLocalizationStatusFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Atlas-first offline run wrote legacy whole-report localization status: %v", err)
	}
	reportHTML, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range [][]byte{
		[]byte(`<html lang="ru">`),
		[]byte(`rm-localization-status--stage_owned`),
		[]byte(`data-rm-message="main.localization.ru_active"`),
	} {
		if !bytes.Contains(reportHTML, marker) {
			t.Fatalf("RU Atlas-first report is missing UI locale marker %q", marker)
		}
	}
	if bytes.Contains(reportHTML, []byte(`data-rm-message="main.localization.ru_unavailable_canonical_en"`)) {
		t.Fatal("Atlas-first stage-owned RU report showed the legacy unavailable-localization warning")
	}
}

func TestRunDefaultOfflineRussianRequestDoesNotRecaptureAfterLocalization(t *testing.T) {
	clearLLMEnv(t)
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/russian-freshness\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repo)

	initial, err := freshness.CaptureRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	captures := []freshness.RepositoryState{initial, initial}
	captureCount := 0
	var stderr bytes.Buffer
	err = runDefaultWithDeps(repo, []string{
		"--offline",
		"--lang", "ru",
		"--no-open",
		"--no-serve",
		"--debug-dir", t.TempDir(),
	}, defaultRunDeps{
		ctx:    context.Background(),
		stdout: io.Discard,
		stderr: &stderr,
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
		t.Fatalf("repository capture count = %d, want initial and pre-render only", captureCount)
	}
	if strings.Contains(
		stderr.String(),
		"after presentation localization",
	) {
		t.Fatalf("offline run performed a post-localization freshness reconciliation:\n%s", stderr.String())
	}
}

func TestRunDefaultAtlasFirstCallPlanExcludesLegacyStages(t *testing.T) {
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

	provider := &atlasFirstAcceptanceProvider{t: t, repositoryType: atlasstudy.RepositoryService}
	server := httptest.NewServer(provider)
	defer server.Close()
	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "deepseek-v4-flash")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	if err := runDefaultWithDeps(repo, []string{
		"--no-open",
		"--no-serve",
		"--debug-dir", t.TempDir(),
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: io.Discard,
	}); err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v", err)
	}

	provider.assertStagesWithMechanismBatches(t, false, 0, mechanismstudy.MaxProviderCalls)
	provider.mu.Lock()
	requests := make([][]byte, 0, len(provider.stages))
	for _, stage := range provider.stages {
		stageBodies := provider.bodies[stage]
		requests = append(requests, bytes.Clone(stageBodies[len(stageBodies)-1]))
	}
	architectureRequests := len(provider.bodies[atlasFirstStageArchitecture])
	targetPortfolioRequests := len(provider.bodies[atlasFirstStageTargetPortfolio])
	entryCallRequests := len(provider.bodies[atlasFirstStageEntryCall])
	provider.mu.Unlock()
	if architectureRequests != 1 || targetPortfolioRequests != 0 || entryCallRequests != 1 {
		t.Fatalf(
			"current stage counts: target portfolio=%d Architecture=%d entry-call=%d",
			targetPortfolioRequests,
			architectureRequests,
			entryCallRequests,
		)
	}
	forbiddenStageMarkers := []string{
		"senior software engineer helping orient",
		"optional editorial guide for one bounded repository tour",
		"Propose central mechanism questions",
		"repository-owned ways to build, run, test, and operate",
	}
	for index, request := range requests {
		for _, marker := range forbiddenStageMarkers {
			if bytes.Contains(request, []byte(marker)) {
				t.Fatalf("ordinary model request %d unexpectedly scheduled %q: %s", index, marker, request)
			}
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

func TestReportMapURL(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		location string
		want     string
	}{
		{
			name:     "plain report",
			location: "http://127.0.0.1:4321/runs/fixture/report.html",
			want:     "http://127.0.0.1:4321/runs/fixture/report.html#/map",
		},
		{
			name:     "replace existing route",
			location: "http://127.0.0.1:4321/runs/fixture/report.html#/mechanisms",
			want:     "http://127.0.0.1:4321/runs/fixture/report.html#/map",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := reportMapURL(test.location); got != test.want {
				t.Fatalf("reportMapURL(%q) = %q, want %q", test.location, got, test.want)
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
		"max_tokens: 64000",
		"max_tokens_override: REPOMAP_LLM_MAX_TOKENS",
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

func TestRunDoctorReportsExactMaxTokensOverride(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("REPOMAP_LLM_ENDPOINT", "https://llm.company.example/v1/chat/completions")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "12345")
	t.Setenv("DEEPSEEK_MAX_TOKENS", "2048")

	var stdout bytes.Buffer
	if err := runDoctor([]string{"llm"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "max_tokens: 12345\n") ||
		!strings.Contains(output, "max_tokens_override: REPOMAP_LLM_MAX_TOKENS\n") {
		t.Fatalf("doctor max_tokens output = %q", output)
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
		"..", "..", "internal", "report", "testdata", "source_episode", "django-atomic", "episode.json",
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
