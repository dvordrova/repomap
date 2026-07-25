package reportserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

func TestWorkspaceSnapshotForManifestPreservesVersionMatrix(t *testing.T) {
	t.Parallel()

	for _, version := range []int{3, report.CurrentRunManifestVersion} {
		manifest := workspaceTestManifest(t, version, "/repo")
		snapshot, catalog, err := workspaceSnapshotForManifest(manifest, "/repo")
		if err != nil {
			t.Fatalf("v%d workspaceSnapshotForManifest: %v", version, err)
		}
		if snapshot == nil || catalog == nil ||
			snapshot.RepositoryRoot() != manifest.RepositoryState.Identity ||
			snapshot.AnalysisRoot() != manifest.AnalysisRoot ||
			snapshot.RepositoryDigest() != manifest.RepositoryStateSHA256 ||
			snapshot.CapturedInputsDigest() != manifest.CapturedInputsSHA256 ||
			catalog.AnalysisRoot() != manifest.AnalysisRoot {
			t.Fatalf("v%d snapshot=%#v catalog=%#v", version, snapshot, catalog)
		}
		if _, _, err := workspaceSnapshotForManifest(manifest, "/other"); err == nil {
			t.Fatalf("v%d mismatched resolved root was accepted", version)
		}
	}

	legacy := workspaceTestManifest(t, 2, "/repo")
	snapshot, catalog, err := workspaceSnapshotForManifest(legacy, "/repo")
	if err != nil || snapshot != nil || catalog != nil {
		t.Fatalf("v2 snapshot=%#v catalog=%#v err=%v", snapshot, catalog, err)
	}
}

func TestAnalysisAvailabilityRequiresBoundedWorkspaceSnapshot(t *testing.T) {
	repo, runsDir, state := writeAnalysisRun(t)
	rewriteAnalysisManifestWithOversizedStages(t, runsDir, report.CurrentRunManifestVersion)

	resolver := &recordingLocationResolver{}
	exact := &recordingExactAnalyzer{}
	var logs []string
	h := &handler{
		runsDir:   runsDir,
		urlPrefix: capabilityURLPrefix(testCapability),
		analysis: newSymbolAnalysis(Options{
			LocationResolver:    resolver,
			ExactSymbolAnalyzer: exact,
		}),
		captureRepo: func(context.Context, string) (freshness.RepositoryState, error) {
			return state, nil
		},
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	if err := h.reloadRuns(); err != nil {
		t.Fatal(err)
	}
	loaded := h.runsSnapshot()
	if len(loaded) != 1 || loaded[0].Manifest == nil || loaded[0].Report == nil ||
		loaded[0].RepoPath != repo || loaded[0].WorkspaceSnapshot != nil ||
		loaded[0].SourceCatalog != nil || loaded[0].AnalysisAvailable {
		t.Fatalf("snapshot-rejected run = %#v", loaded)
	}
	if !h.analysisAvailable(*loaded[0].Manifest) {
		t.Fatal("analyzer fixture was unavailable independently of the snapshot")
	}

	runListRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/runs", nil)
	runListResponse := httptest.NewRecorder()
	h.serveRuns(runListResponse, runListRequest)
	var runList struct {
		Runs []RunSummary `json:"runs"`
	}
	if runListResponse.Code != http.StatusOK ||
		json.Unmarshal(runListResponse.Body.Bytes(), &runList) != nil ||
		len(runList.Runs) != 1 || runList.Runs[0].AnalysisAvailable {
		t.Fatalf("run list status=%d payload=%#v body=%s",
			runListResponse.Code, runList, runListResponse.Body.String())
	}

	reportRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/report.html", nil)
	reportRequest.SetPathValue("runID", "20260711-220000-pebble")
	reportResponse := httptest.NewRecorder()
	h.serveReport(reportResponse, reportRequest)
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("report status = %d, body=%s", reportResponse.Code, reportResponse.Body.String())
	}

	symbols := performSymbolsRequest(t, http.HandlerFunc(h.serveSymbols), symbolsRequest{
		RunID:       "20260711-220000-pebble",
		ComponentID: "component-batch",
		AnchorID:    "anchor-batch",
		Line:        395,
	})
	if symbols.Code != http.StatusForbidden || len(resolver.requests) != 0 || len(exact.requests) != 0 {
		t.Fatalf("symbols status=%d resolver_calls=%d exact_calls=%d body=%s",
			symbols.Code, len(resolver.requests), len(exact.requests), symbols.Body.String())
	}
	snapshotDiagnostics := 0
	for _, log := range logs {
		if len(log) > 512 || strings.Contains(log, repo) {
			t.Fatalf("snapshot diagnostic was unsafe: %q", log)
		}
		if strings.Contains(log, "source catalog unavailable; local analysis disabled") {
			snapshotDiagnostics++
		}
	}
	if snapshotDiagnostics == 0 {
		t.Fatalf("missing bounded snapshot diagnostic: %v", logs)
	}
}

func TestVersion3SnapshotRejectionPreservesLegacyDegradation(t *testing.T) {
	repo, runsDir, _ := writeAnalysisRun(t)
	rewriteAnalysisManifestWithOversizedStages(t, runsDir, 3)
	var logs []string
	h := &handler{
		runsDir: runsDir,
		analysis: newSymbolAnalysis(Options{
			LocationResolver:    &recordingLocationResolver{},
			ExactSymbolAnalyzer: &recordingExactAnalyzer{},
		}),
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	if err := h.reloadRuns(); err != nil {
		t.Fatal(err)
	}
	loaded := h.runsSnapshot()
	if len(loaded) != 1 || loaded[0].Manifest == nil || loaded[0].Report == nil ||
		loaded[0].RepoPath != repo || loaded[0].WorkspaceSnapshot != nil ||
		loaded[0].SourceCatalog != nil || loaded[0].AnalysisAvailable ||
		len(loaded[0].Sources) == 0 || loaded[0].Report.SourceIDs["batch.go"] == "" {
		t.Fatalf("snapshot-rejected v3 run = %#v", loaded)
	}
	for _, log := range logs {
		if strings.Contains(log, "source catalog unavailable; local analysis disabled") {
			t.Fatalf("v3 emitted current-only snapshot diagnostic: %v", logs)
		}
	}
}

func TestRunRecordUsesSnapshotForCurrentWorkspaceAuthority(t *testing.T) {
	t.Parallel()

	manifestAuthority := workspaceTestManifest(t, report.CurrentRunManifestVersion, "/manifest-root")
	snapshotManifest := workspaceTestManifest(t, report.CurrentRunManifestVersion, "/snapshot-root")
	snapshot, err := snapshotManifest.WorkspaceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	run := runRecord{
		RepoPath:          snapshot.AnalysisRoot(),
		Manifest:          &manifestAuthority,
		WorkspaceSnapshot: &snapshot,
		ReportSHA256:      manifestAuthority.ReportSHA256,
	}
	if manifestAuthority.VerifyRepositoryState(snapshotManifest.RepositoryState) == nil {
		t.Fatal("manifest fixture unexpectedly accepted the snapshot repository")
	}
	if err := run.verifyRepositoryState(snapshotManifest.RepositoryState); err != nil {
		t.Fatalf("run snapshot verification: %v", err)
	}
	if run.workspaceAnalysisRoot() != snapshot.AnalysisRoot() ||
		run.workspaceRepositoryDigest() != snapshot.RepositoryDigest() {
		t.Fatalf("run workspace identity did not come from snapshot")
	}
	if run.Manifest.ReportSHA256 != manifestAuthority.ReportSHA256 ||
		run.ReportSHA256 != manifestAuthority.ReportSHA256 {
		t.Fatal("report binding moved out of the manifest/reportserver adapter")
	}

	version3 := workspaceTestManifest(t, 3, "/legacy-v3")
	if err := (runRecord{Manifest: &version3}).verifyRepositoryState(version3.RepositoryState); err != nil {
		t.Fatalf("v3 degraded verification fallback: %v", err)
	}
	current := workspaceTestManifest(t, report.CurrentRunManifestVersion, "/current")
	if err := (runRecord{Manifest: &current}).verifyRepositoryState(current.RepositoryState); err == nil {
		t.Fatal("current run without a snapshot did not fail closed")
	}
}

func TestRefreshRunFreshnessUsesSnapshotRootAndAssessment(t *testing.T) {
	t.Parallel()

	manifestAuthority := workspaceTestManifest(t, report.CurrentRunManifestVersion, "/manifest-root")
	snapshotManifest := workspaceTestManifest(t, report.CurrentRunManifestVersion, "/snapshot-root")
	snapshot, err := snapshotManifest.WorkspaceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	current := snapshotManifest.RepositoryState
	current.Dirty = []freshness.DirtyFile{{
		Status:        "modified",
		Path:          "notes.txt",
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: strings.Repeat("e", 64),
	}}
	var capturedRoot string
	h := handler{
		captureRepo: func(_ context.Context, root string) (freshness.RepositoryState, error) {
			capturedRoot = root
			return current, nil
		},
	}
	reportData := report.ReportData{}
	run := runRecord{
		RunSummary:        RunSummary{ID: "20260725-010000-snapshot"},
		Manifest:          &manifestAuthority,
		WorkspaceSnapshot: &snapshot,
		Report:            &reportData,
	}
	h.refreshRunFreshness(context.Background(), &run)
	if capturedRoot != snapshot.RepositoryRoot() {
		t.Fatalf("captured root = %q, want %q", capturedRoot, snapshot.RepositoryRoot())
	}
	if run.Report.Freshness == nil ||
		run.Report.Freshness.State != freshness.FreshnessUnrelatedChanges {
		t.Fatalf("refreshed freshness = %#v", run.Report.Freshness)
	}
}

func TestLoadAndReadAuthorizedRunRestoreWorkspaceSnapshot(t *testing.T) {
	_, runsDir, _ := writeAnalysisRun(t)
	h := handler{
		runsDir: runsDir,
		analysis: newSymbolAnalysis(Options{
			LocationResolver:    &recordingLocationResolver{},
			ExactSymbolAnalyzer: &recordingExactAnalyzer{},
		}),
	}

	assertLoaded := func(version int) {
		t.Helper()
		runs, err := h.loadRuns()
		if err != nil {
			t.Fatalf("v%d loadRuns: %v", version, err)
		}
		if len(runs) != 1 {
			t.Fatalf("v%d runs = %d, want 1", version, len(runs))
		}
		loaded := runs[0]
		if loaded.Manifest == nil || loaded.Manifest.Version != version ||
			loaded.WorkspaceSnapshot == nil || loaded.SourceCatalog == nil ||
			!loaded.AnalysisAvailable ||
			loaded.RepoPath != loaded.WorkspaceSnapshot.AnalysisRoot() ||
			loaded.SourceCatalog.AnalysisRoot() != loaded.WorkspaceSnapshot.AnalysisRoot() {
			t.Fatalf("v%d loaded run = %#v", version, loaded)
		}

		runRoot, err := os.OpenRoot(filepath.Join(runsDir, loaded.ID))
		if err != nil {
			t.Fatal(err)
		}
		defer runRoot.Close()
		reopened, err := h.readAuthorizedRun(loaded.ID, runRoot)
		if err != nil {
			t.Fatalf("v%d readAuthorizedRun: %v", version, err)
		}
		if reopened.WorkspaceSnapshot == nil || reopened.SourceCatalog == nil ||
			reopened.WorkspaceSnapshot.RepositoryDigest() != loaded.WorkspaceSnapshot.RepositoryDigest() ||
			reopened.WorkspaceSnapshot.CapturedInputsDigest() != loaded.WorkspaceSnapshot.CapturedInputsDigest() {
			t.Fatalf("v%d reopened run = %#v", version, reopened)
		}
	}

	assertLoaded(report.CurrentRunManifestVersion)
	rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
		manifest.Version = 3
	})
	assertLoaded(3)
}

func workspaceTestManifest(t *testing.T, version int, root string) report.RunManifest {
	t.Helper()

	repositoryVersion := freshness.RepositoryStateVersion
	if version == 2 {
		repositoryVersion = 1
	}
	repository := freshness.RepositoryState{
		Version:  repositoryVersion,
		Identity: root,
		Head:     strings.Repeat("a", 40),
		Dirty:    []freshness.DirtyFile{},
	}
	repositoryDigest, err := repository.Digest()
	if err != nil {
		t.Fatal(err)
	}
	manifest := report.RunManifest{
		Version:               version,
		RepositoryState:       repository,
		AnalysisRoot:          root,
		RepositoryStateSHA256: repositoryDigest,
		ReportSHA256:          strings.Repeat("b", 64),
		ReportFormatVersion:   report.CurrentFormatVersion,
		OpenablePaths:         []string{"main.go"},
	}
	if version == 2 {
		return manifest
	}
	manifest.CapturedInputs = []freshness.CapturedInput{{
		Version:       freshness.CapturedInputVersion,
		ID:            strings.Repeat("c", 64),
		Path:          "main.go",
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: strings.Repeat("d", 64),
		Stages:        []string{"report_evidence"},
	}}
	manifest.CapturedInputsSHA256, err = freshness.CapturedInputsDigest(manifest.CapturedInputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Freshness = freshness.NewFreshnessResult(freshness.FreshnessFresh)
	manifest.MaterialInputs = report.MaterialInputs{
		SelectedRevision:     repository.Head,
		InputPolicyVersion:   "captured-inputs-v1",
		ArchitectureContract: 1,
		ReportContract:       report.CurrentFormatVersion,
	}
	return manifest
}

func rewriteAnalysisManifestWithOversizedStages(t *testing.T, runsDir string, version int) {
	t.Helper()
	rewriteAnalysisManifest(t, runsDir, func(manifest *report.RunManifest) {
		manifest.Version = version
		manifest.CapturedInputs[0].Stages = make([]string, 65)
		for index := range manifest.CapturedInputs[0].Stages {
			manifest.CapturedInputs[0].Stages[index] = strings.Repeat("a", index+1)
		}
		digest, err := freshness.CapturedInputsDigest(manifest.CapturedInputs)
		if err != nil {
			t.Fatal(err)
		}
		manifest.CapturedInputsSHA256 = digest
	})
}
