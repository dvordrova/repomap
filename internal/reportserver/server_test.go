package reportserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	reportpkg "github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testCapability = "test-capability"

func TestHandlerServesInitialReportAndOpensItsOpaqueSource(t *testing.T) {
	fixture := writeTestRun(t)
	var openedPath string
	var openedLine, openedColumn int
	handler, err := NewHandler(Options{
		RunsDir:      fixture.runsDir,
		InitialRunID: fixture.runID,
		Capability:   testCapability,
		OpenFile: func(_ context.Context, absolutePath string, line, column int) error {
			openedPath = absolutePath
			openedLine = line
			openedColumn = column
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)

	response, err := server.Client().Get(baseURL + "/runs/" + fixture.runID + "/report.html")
	if err != nil {
		t.Fatal(err)
	}
	servedHTML := readResponse(t, response, http.StatusOK)
	wantReport := fixture.reportData
	wantReport.SourceIDs = map[string]string{"batch.go": fixture.sourceID}
	wantHTML, err := reportpkg.RenderHTMLWithOptions(&wantReport, reportpkg.RenderOptions{
		TargetNavigation: fixture.targetNavigation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(servedHTML, wantHTML) {
		t.Fatal("served report differs from the generated report beyond transient source authority")
	}
	if bytes.Contains(fixture.staticHTML, []byte(fixture.sourceID)) ||
		!bytes.Contains(servedHTML, []byte(fixture.sourceID)) {
		t.Fatal("opaque local-open authority was persisted or omitted from served HTML")
	}

	openResponse := postOpen(t, baseURL, openRequest{
		RunID: fixture.runID, SourceID: fixture.sourceID, Line: 7, Column: 3,
	})
	var payload map[string]any
	decodeResponse(t, openResponse, http.StatusOK, &payload)
	if len(payload) != 1 || payload["status"] != "opened" ||
		openedPath != fixture.sourcePath || openedLine != 7 || openedColumn != 3 {
		t.Fatalf("open result=%#v path=%q line=%d column=%d",
			payload, openedPath, openedLine, openedColumn)
	}

	rootClient := *server.Client()
	rootClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	rootResponse, err := rootClient.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer rootResponse.Body.Close()
	wantLocation := capabilityURLPrefix(testCapability) + "/runs/" + fixture.runID + "/report.html#/repository"
	if rootResponse.StatusCode != http.StatusFound || rootResponse.Header.Get("Location") != wantLocation {
		t.Fatalf("root status=%d location=%q", rootResponse.StatusCode, rootResponse.Header.Get("Location"))
	}
}

func TestLoadRunRestoresVirtualPageWithoutPhysicalHTML(t *testing.T) {
	fixture := writeTestRun(t)
	htmlPath := filepath.Join(fixture.runsDir, fixture.runID, "report.html")
	if err := os.Remove(htmlPath); err != nil {
		t.Fatal(err)
	}
	run, _, err := loadRun(fixture.runsDir, fixture.runID)
	if err != nil {
		t.Fatalf("load backing page without report.html: %v", err)
	}
	if run.id != fixture.runID || len(run.rendered) == 0 {
		t.Fatalf("restored virtual page = %#v", run)
	}
	// The page is rendered from report.json; a missing report.html is not
	// a reason to refuse to serve.
	if _, err := NewHandler(Options{
		RunsDir:      fixture.runsDir,
		InitialRunID: fixture.runID,
		Capability:   testCapability,
		OpenFile:     func(context.Context, string, int, int) error { return nil },
	}); err != nil {
		t.Fatalf("owner without physical report.html: %v", err)
	}
}

func TestOpenNamesSourcesByIDAndFollowsTheFile(t *testing.T) {
	fixture := writeTestRun(t)
	launches := 0
	handler, err := NewHandler(Options{
		RunsDir:      fixture.runsDir,
		InitialRunID: fixture.runID,
		Capability:   testCapability,
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
	baseURL := server.URL + capabilityURLPrefix(testCapability)

	rawPathResponse := postJSON(t, baseURL+"/api/open", map[string]any{
		"run_id": fixture.runID, "source_id": fixture.sourceID, "path": "../outside.go",
	}, true)
	readResponse(t, rawPathResponse, http.StatusBadRequest)
	unknownResponse := postOpen(t, baseURL, openRequest{
		RunID: fixture.runID, SourceID: strings.Repeat("a", 43),
	})
	readResponse(t, unknownResponse, http.StatusNotFound)
	if launches != 0 {
		t.Fatalf("unauthorized requests launched editor %d times", launches)
	}

	if err := os.WriteFile(fixture.sourcePath, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedResponse := postOpen(t, baseURL, openRequest{
		RunID: fixture.runID, SourceID: fixture.sourceID,
	})
	var changed map[string]any
	decodeResponse(t, changedResponse, http.StatusOK, &changed)
	if len(changed) != 1 || changed["status"] != "opened" || launches != 1 {
		t.Fatalf("changed source open=%#v launches=%d", changed, launches)
	}

	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, fixture.sourcePath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// A symlink is followed like any other file: the server is not a
	// security boundary, and the editor opens what the path resolves to.
	symlinkResponse := postOpen(t, baseURL, openRequest{
		RunID: fixture.runID, SourceID: fixture.sourceID,
	})
	var opened map[string]any
	decodeResponse(t, symlinkResponse, http.StatusOK, &opened)
	if opened["status"] != "opened" || launches != 2 {
		t.Fatalf("symlink open=%#v launches=%d", opened, launches)
	}
}

func TestHandlerRefusesAReportItCannotRender(t *testing.T) {
	fixture := writeTestRun(t)
	reportPath := filepath.Join(fixture.runsDir, fixture.runID, "report.json")
	if err := os.WriteFile(reportPath, []byte(`{"format_version":39,"repo_name":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHandler(Options{
		RunsDir: fixture.runsDir, InitialRunID: fixture.runID, Capability: testCapability,
	}); err == nil {
		t.Fatal("handler served a report.json it cannot render")
	}
}

func TestServeUsesLoopbackCapabilityURLAndStopsWithContext(t *testing.T) {
	fixture := writeTestRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, Options{
			RunsDir: fixture.runsDir, InitialRunID: fixture.runID, Port: 0,
			OpenFile: func(context.Context, string, int, int) error { return nil },
			OnReady:  func(serverURL string) error { ready <- serverURL; return nil },
		})
	}()

	var serverURL string
	select {
	case serverURL = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not become ready")
	}
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "127.0.0.1" ||
		!strings.HasPrefix(parsed.Path, "/_repomap/") ||
		!strings.HasSuffix(parsed.Path, "/runs/"+fixture.runID+"/report.html") ||
		parsed.Fragment != "/repository" {
		t.Fatalf("unexpected ready URL %q", serverURL)
	}
	response, err := http.Get(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	readResponse(t, response, http.StatusOK)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

type testRunFixture struct {
	runsDir          string
	runID            string
	sourceID         string
	sourcePath       string
	reportData       reportpkg.ReportData
	targetNavigation *reportpkg.TargetNavigationPortfolio
	staticHTML       []byte
}

type reportServerGraphFixture struct {
	index      programindex.Index
	groups     groupindex.Index
	reduced    documentationreduce.Result
	groupsRaw  []byte
	reducedRaw []byte
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeTestRunAt(
	t *testing.T,
	runsDir,
	runID,
	canonicalRepository string,
	reportData reportpkg.ReportData,
) testRunFixture {
	t.Helper()
	return writeTestRunAtWithTargetName(
		t, runsDir, runID, canonicalRepository, reportData, reportData.RepoName,
	)
}

func reportServerStructuralProgramIndexFixture(t *testing.T, name string) programindex.Index {
	t.Helper()
	location := &programindex.Location{Path: "batch.go", Line: 1, Column: 1}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("1", 64),
		SourceSHA256:   strings.Repeat("2", 64),
		Target: programindex.TargetInput{
			Language: "fixture", Kind: "library", Name: name, Selector: "reportserver-fixture:" + name,
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "batch.go"}},
			AnchorFileRef: "f1",
			Seeds:         []programindex.TargetSeedInput{},
		},
		Objects: []programindex.ObjectInput{
			{
				SourceRef: "batch-module", Kind: programindex.ObjectModule,
				Name: "batch", Visibility: programindex.VisibilityPublic, Location: location,
			},
			{
				SourceRef: "batch-function", Kind: programindex.ObjectFunction,
				Name: "Batch", Visibility: programindex.VisibilityPublic, Location: location,
				OwnerRef: "batch-module", ContainerRef: "batch-module",
			},
		},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func reportServerGraphAuthorityFixture(
	t *testing.T,
	base programindex.Index,
) reportServerGraphFixture {
	t.Helper()
	reduced, err := documentationreduce.Run(
		t.Context(), llm.Executor{}, nil, readmetargetscout.GuidanceSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	memberID := ""
	for _, object := range base.Objects {
		if object.Kind == programindex.ObjectFunction && object.Name == "Batch" {
			memberID = object.ID
			break
		}
	}
	if memberID == "" {
		t.Fatal("fixture Batch function is missing")
	}
	index, err := programindex.Enrich(base, reduced.ReductionSHA256, []programindex.CategoryAssignment{{
		SubjectID: memberID, Categories: []programindex.Category{programindex.CategoryCore},
	}})
	if err != nil {
		t.Fatal(err)
	}
	groups, diagnostics, err := groupindex.Build(index, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "batch-core", Title: "Batch core", Summary: "Owns the fixture operation.",
		Lane: groupindex.LaneCore, MemberSubjectIDs: []string{memberID},
		EvidenceSubjectIDs: []string{memberID},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("group fixture diagnostics: %#v", diagnostics)
	}
	groupsRaw, err := groupindex.Encode(groups)
	if err != nil {
		t.Fatal(err)
	}
	reducedRaw, err := documentationreduce.Encode(reduced)
	if err != nil {
		t.Fatal(err)
	}
	return reportServerGraphFixture{
		index: index, groups: groups, reduced: reduced,
		groupsRaw: groupsRaw, reducedRaw: reducedRaw,
	}
}

func postOpen(t *testing.T, baseURL string, request openRequest) *http.Response {
	t.Helper()
	return postJSON(t, baseURL+"/api/open", request, true)
}

func postJSON(t *testing.T, endpoint string, value any, withAction bool) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", parsed.Scheme+"://"+parsed.Host)
	if withAction {
		request.Header.Set("X-Repomap-Action", "open-file")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, wantStatus int, target any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%q, want %d", response.StatusCode, body, wantStatus)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func readResponse(t *testing.T, response *http.Response, wantStatus int) []byte {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("status=%d body=%q, want %d", response.StatusCode, body, wantStatus)
	}
	return body
}

func hashBytes(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func hashString(value string) string {
	return hashBytes([]byte(value))
}

func writeTestRun(t *testing.T) testRunFixture {
	t.Helper()
	repository := t.TempDir()
	canonicalRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(canonicalRepository, "batch.go")
	if err := os.WriteFile(sourcePath, []byte("package batch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const runID = "20260822-120000-server"
	return writeTestRunAt(t, t.TempDir(), runID, canonicalRepository, reportpkg.ReportData{
		FormatVersion: reportpkg.CurrentFormatVersion,
		RepoName:      filepath.Base(canonicalRepository),
		OpenablePaths: []string{"batch.go"},
	})
}

// writeTestRunAtWithTargetName lays out one run directory the way a real run
// leaves it: the artifacts, report.json, report.html and the manifest.
func writeTestRunAtWithTargetName(
	t *testing.T,
	runsDir,
	runID,
	canonicalRepository string,
	reportData reportpkg.ReportData,
	targetName string,
) testRunFixture {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if reportData.CapturedRevision == "" {
		reportData.CapturedRevision = strings.Repeat("0", 40)
	}
	graph := reportServerGraphAuthorityFixture(t, reportServerStructuralProgramIndexFixture(t, targetName))
	index := graph.index
	portfolio, err := reportpkg.NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	groupGraph, err := reportpkg.NewGroupGraphView([]groupindex.Index{graph.groups}, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	reportData.ProgramPortfolio = portfolio
	reportData.GroupGraph = groupGraph
	pagePortfolio, err := programpage.Build(index.Target.ID, []programpage.Page{{
		Target: index.Target.Snapshot(), RunID: runID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := targetoutcome.NewSelectedTargetWithLanguages(
		targetoutcome.LanguageGroup(index.Target.Language), []string{index.Target.Language},
		targetoutcome.ScopeLibrary, index.Target.Name, index.Target.Selector,
	)
	if err != nil {
		t.Fatal(err)
	}
	analyzed, err := targetoutcome.NewAnalyzed(selected, index.Target, runID)
	if err != nil {
		t.Fatal(err)
	}
	outcomePortfolio, err := targetoutcome.Build(selected.ID, []targetoutcome.Outcome{analyzed})
	if err != nil {
		t.Fatal(err)
	}
	outcomeView, err := reportpkg.NewTargetOutcomePortfolioView(outcomePortfolio, pagePortfolio)
	if err != nil {
		t.Fatal(err)
	}
	reportData.TargetOutcomePortfolio = outcomeView
	navigation, err := reportpkg.BuildTargetNavigation([]reportpkg.TargetNavigationPage{{
		RunID: runID, ProgramTarget: index.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename,
	}}, index.Target.ID, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	pageRaw, err := pagePortfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	outcomeRaw, err := outcomePortfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		programpage.ArtifactFilename: pageRaw, targetoutcome.ArtifactFilename: outcomeRaw,
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := programindex.Persist(runDir, programindex.ArtifactFilename, index); err != nil {
		t.Fatal(err)
	}
	set, err := programindex.BuildArtifactSet(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := programindex.PersistArtifactSet(runDir, set); err != nil {
		t.Fatal(err)
	}
	if err := dependencies.Persist(runDir, dependencies.Empty()); err != nil {
		t.Fatal(err)
	}
	if err := documentationreduce.Persist(runDir, graph.reduced); err != nil {
		t.Fatal(err)
	}
	if err := groupindex.Persist(runDir, graph.groups); err != nil {
		t.Fatal(err)
	}
	reportJSON, err := json.Marshal(reportData)
	if err != nil {
		t.Fatal(err)
	}
	staticHTML, err := reportpkg.RenderHTMLWithOptions(&reportData, reportpkg.RenderOptions{
		TargetNavigation: navigation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.html"), staticHTML, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshotJSON, err := json.Marshal(struct {
		RepoName      string   `json:"repo_name"`
		FilteredFiles []string `json:"filtered_files,omitempty"`
	}{RepoName: reportData.RepoName, FilteredFiles: reportData.OpenablePaths})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "snapshot.json"), snapshotJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := json.Marshal(map[string]string{"repo_name": reportData.RepoName})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), metadataJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := reportpkg.RunManifest{
		Version: reportpkg.CurrentRunManifestVersion,
		RepositoryState: freshness.RepositoryState{
			Version: freshness.RepositoryStateVersion, Identity: canonicalRepository, Head: strings.Repeat("0", 40),
		},
		AnalysisRoot:        canonicalRepository,
		ReportFormatVersion: reportpkg.CurrentFormatVersion,
		ProgramTargetID:     index.Target.ID,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, reportpkg.RunManifestFilename), manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	primarySourcePath, primarySourceID := "", ""
	if len(reportData.OpenablePaths) > 0 {
		primarySourcePath = filepath.Join(canonicalRepository, filepath.FromSlash(reportData.OpenablePaths[0]))
		primarySourceID = sourceID(runID, reportData.OpenablePaths[0])
	}
	return testRunFixture{
		runsDir: runsDir, runID: runID,
		sourceID: primarySourceID, sourcePath: primarySourcePath,
		reportData: reportData, targetNavigation: navigation, staticHTML: staticHTML,
	}
}
