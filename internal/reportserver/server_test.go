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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/programindex"
	reportpkg "github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
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
	wantHTML, err := reportpkg.RenderHTMLWithOptions(&wantReport, reportpkg.RenderOptions{})
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
	wantLocation := capabilityURLPrefix(testCapability) + "/runs/" + fixture.runID + "/report.html#/program"
	if rootResponse.StatusCode != http.StatusFound || rootResponse.Header.Get("Location") != wantLocation {
		t.Fatalf("root status=%d location=%q", rootResponse.StatusCode, rootResponse.Header.Get("Location"))
	}
}

func TestLoadTargetNavigationAllowsSinglePageManifest(t *testing.T) {
	navigation, err := loadTargetNavigation("unneeded", reportpkg.RunManifest{})
	if err != nil || navigation != nil {
		t.Fatalf("single-page navigation = %#v, err = %v", navigation, err)
	}
	manifest := reportpkg.RunManifest{
		MaterialInputs: reportpkg.MaterialInputs{TargetPagePortfolioSHA256: strings.Repeat("a", 64)},
	}
	if _, err := loadTargetNavigation(t.TempDir(), manifest); err == nil ||
		!strings.Contains(err.Error(), "load target navigation") {
		t.Fatalf("bound portfolio was not required: %v", err)
	}
}

func TestSiblingPagesRequirePortfolioRouteAndManifestBinding(t *testing.T) {
	const (
		initialRunID = "20260822-120000-api"
		siblingRunID = "20260822-120000-worker"
		containerSHA = "container-artifact-sha"
		portfolioSHA = "portfolio-artifact-sha"
	)
	item := reportpkg.TargetNavigationItem{
		TargetID: "program-target-worker",
		Href:     "../" + siblingRunID + "/report.html#/program",
	}
	gotRunID, err := navigationRunID(initialRunID, "program-target-api", item)
	if err != nil || gotRunID != siblingRunID {
		t.Fatalf("navigation run id=%q err=%v", gotRunID, err)
	}
	for _, href := range []string{
		"../../outside/report.html#/program",
		"https://example.test/report.html#/program",
		"../" + siblingRunID + "/report.html?raw=1#/program",
	} {
		item.Href = href
		if _, err := navigationRunID(initialRunID, "program-target-api", item); err == nil {
			t.Fatalf("accepted unbound sibling route %q", href)
		}
	}

	initialNavigation := &reportpkg.TargetNavigationPortfolio{
		DefaultTargetID: "program-target-api",
		CurrentTargetID: "program-target-api",
	}
	initial := runRecord{
		id: initialRunID,
		manifest: reportpkg.RunManifest{
			RepositoryState: freshness.RepositoryState{Identity: "/repo"},
			MaterialInputs: reportpkg.MaterialInputs{
				AnalysisTargetRef:         "target-api",
				ProgramTargetID:           "program-target-api",
				TargetRunContainerSHA256:  containerSHA,
				TargetPagePortfolioSHA256: portfolioSHA,
				SelectedRevision:          "before-change",
			},
		},
		targetNavigation: initialNavigation,
	}
	sibling := runRecord{
		id: siblingRunID,
		manifest: reportpkg.RunManifest{
			RepositoryState:       freshness.RepositoryState{Identity: "/repo"},
			RepositoryStateSHA256: "changed-during-run",
			MaterialInputs: reportpkg.MaterialInputs{
				AnalysisTargetRef:         "target-worker",
				ProgramTargetID:           "program-target-worker",
				TargetRunContainerSHA256:  containerSHA,
				TargetPagePortfolioSHA256: portfolioSHA,
				SelectedRevision:          "after-change",
			},
		},
		targetNavigation: &reportpkg.TargetNavigationPortfolio{
			DefaultTargetID: "program-target-api",
			CurrentTargetID: "program-target-worker",
		},
	}
	if err := authorizeSiblingRun(initial, sibling, "program-target-worker"); err != nil {
		t.Fatalf("repository change during run blocked sibling page: %v", err)
	}
	foreign := sibling
	foreign.manifest.RepositoryState.Identity = "/other-repo"
	if err := authorizeSiblingRun(initial, foreign, "program-target-worker"); err == nil {
		t.Fatal("sibling page from another repository was authorized")
	}
	unbound := sibling
	unbound.manifest.MaterialInputs.TargetPagePortfolioSHA256 = "other-portfolio"
	if err := authorizeSiblingRun(initial, unbound, "program-target-worker"); err == nil {
		t.Fatal("sibling page outside the initial portfolio was authorized")
	}
	outerAlias := sibling
	outerAlias.manifest.MaterialInputs.AnalysisTargetRef = "target-api"
	if err := authorizeSiblingRun(initial, outerAlias, "program-target-worker"); err == nil {
		t.Fatal("sibling page reused the initial outer target authority")
	}
}

func TestHandlerServesManifestAuthorizedSiblingPageAndOpensItsSource(t *testing.T) {
	initial, sibling := writeTargetPortfolioFixture(t)
	var opened string
	handler, err := NewHandler(Options{
		RunsDir:      initial.runsDir,
		InitialRunID: initial.runID,
		Capability:   testCapability,
		OpenFile: func(_ context.Context, absolutePath string, _, _ int) error {
			opened = absolutePath
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)

	response, err := server.Client().Get(baseURL + "/runs/" + sibling.runID + "/report.html")
	if err != nil {
		t.Fatal(err)
	}
	html := readResponse(t, response, http.StatusOK)
	wantReport := sibling.reportData
	wantReport.SourceIDs = map[string]string{"batch.go": sibling.sourceID}
	wantHTML, err := reportpkg.RenderHTMLWithOptions(
		&wantReport,
		reportpkg.RenderOptions{TargetNavigation: sibling.targetNavigation},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(html, wantHTML) || bytes.Contains(sibling.staticHTML, []byte(sibling.sourceID)) {
		t.Fatal("served sibling differs from its static page beyond transient source authority")
	}
	openResponse := postOpen(t, baseURL, openRequest{
		RunID: sibling.runID, SourceID: sibling.sourceID,
	})
	var payload map[string]any
	decodeResponse(t, openResponse, http.StatusOK, &payload)
	if opened != sibling.sourcePath {
		t.Fatalf("opened %q, want sibling source %q", opened, sibling.sourcePath)
	}
	unboundResponse, err := server.Client().Get(baseURL + "/runs/unbound-run/report.html")
	if err != nil {
		t.Fatal(err)
	}
	readResponse(t, unboundResponse, http.StatusNotFound)
}

func TestOpenRejectsRawPathsAndSymlinkReplacement(t *testing.T) {
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
	readResponse(t, unknownResponse, http.StatusForbidden)
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
	symlinkResponse := postOpen(t, baseURL, openRequest{
		RunID: fixture.runID, SourceID: fixture.sourceID,
	})
	var failure map[string]string
	decodeResponse(t, symlinkResponse, http.StatusConflict, &failure)
	if failure["error"] != "authorized source is unavailable" || launches != 1 {
		t.Fatalf("symlink failure=%#v launches=%d", failure, launches)
	}
}

func TestOpenRequiresLoopbackHostOriginCapabilityAndAction(t *testing.T) {
	fixture := writeTestRun(t)
	handler, err := NewHandler(Options{
		RunsDir:      fixture.runsDir,
		InitialRunID: fixture.runID,
		Capability:   testCapability,
		ExpectedHost: "127.0.0.1:4321",
		OpenFile:     func(context.Context, string, int, int) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(openRequest{RunID: fixture.runID, SourceID: fixture.sourceID})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://127.0.0.1:4321" + capabilityURLPrefix(testCapability) + "/api/open"

	tests := []struct {
		name       string
		path       string
		host       string
		origin     string
		action     string
		wantStatus int
	}{
		{name: "wrong capability", path: "/_repomap/wrong/api/open", host: "127.0.0.1:4321", origin: "http://127.0.0.1:4321", action: "open-file", wantStatus: http.StatusNotFound},
		{name: "foreign host", path: endpoint, host: "example.test", origin: "http://example.test", action: "open-file", wantStatus: http.StatusForbidden},
		{name: "foreign origin", path: endpoint, host: "127.0.0.1:4321", origin: "https://evil.test", action: "open-file", wantStatus: http.StatusForbidden},
		{name: "missing action", path: endpoint, host: "127.0.0.1:4321", origin: "http://127.0.0.1:4321", wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(payload))
			request.Host = test.host
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", test.origin)
			if test.action != "" {
				request.Header.Set("X-Repomap-Action", test.action)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
}

func TestHandlerRejectsReportBytesOutsideManifestAuthority(t *testing.T) {
	fixture := writeTestRun(t)
	reportPath := filepath.Join(fixture.runsDir, fixture.runID, "report.json")
	if err := os.WriteFile(reportPath, []byte(`{"format_version":39,"repo_name":"tampered"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHandler(Options{
		RunsDir: fixture.runsDir, InitialRunID: fixture.runID, Capability: testCapability,
	}); err == nil {
		t.Fatal("handler accepted report bytes outside manifest authority")
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
		parsed.Fragment != "/program" {
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
	staticHTML       []byte
	targetNavigation *reportpkg.TargetNavigationPortfolio
}

func writeTestRun(t *testing.T) testRunFixture {
	t.Helper()
	repository := t.TempDir()
	canonicalRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(canonicalRepository, "batch.go")
	sourceContent := []byte("package batch\n")
	if err := os.WriteFile(sourcePath, sourceContent, 0o600); err != nil {
		t.Fatal(err)
	}
	const runID = "20260822-120000-server"
	return writeTestRunAt(
		t,
		t.TempDir(),
		runID,
		canonicalRepository,
		reportpkg.ReportData{
			FormatVersion: reportpkg.CurrentFormatVersion,
			RepoName:      filepath.Base(canonicalRepository),
			OpenablePaths: []string{"batch.go"},
		},
	)
}

func writeTestRunAt(
	t *testing.T,
	runsDir,
	runID,
	canonicalRepository string,
	reportData reportpkg.ReportData,
) testRunFixture {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	reportJSON, err := json.Marshal(reportData)
	if err != nil {
		t.Fatal(err)
	}
	staticHTML, err := reportpkg.RenderHTMLWithOptions(&reportData, reportpkg.RenderOptions{})
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
	state := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: canonicalRepository,
		Head:     strings.Repeat("0", 40),
	}
	stateDigest, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]freshness.CapturedInput, 0, len(reportData.OpenablePaths))
	for _, relativePath := range reportData.OpenablePaths {
		sourceContent, err := os.ReadFile(filepath.Join(canonicalRepository, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            hashString("input\x00" + relativePath),
			Path:          relativePath,
			Kind:          freshness.FileRegular,
			Mode:          string(freshness.FileRegular),
			ContentSHA256: hashBytes(sourceContent),
			Stages:        []string{"report_evidence"},
		})
	}
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest := reportpkg.RunManifest{
		Version:               reportpkg.CurrentRunManifestVersion,
		RepositoryState:       state,
		AnalysisRoot:          canonicalRepository,
		RepositoryStateSHA256: stateDigest,
		SnapshotSHA256:        hashBytes(snapshotJSON),
		ReportSHA256:          hashBytes(reportJSON),
		ReportFormatVersion:   reportpkg.CurrentFormatVersion,
		OpenablePaths:         append([]string(nil), reportData.OpenablePaths...),
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		MaterialInputs: reportpkg.MaterialInputs{
			SelectedRevision:      state.Head,
			ProgramTargetID:       "pt-server-fixture",
			ProgramTargetSHA256:   strings.Repeat("8", 64),
			ProgramIndexSetSHA256: strings.Repeat("9", 64),
			InputPolicyVersion:    "captured-inputs-v1",
			ReportContract:        reportpkg.CurrentFormatVersion,
		},
	}
	if reportData.AnalysisTarget != nil {
		targetJSON, err := reportData.AnalysisTarget.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		manifest.MaterialInputs.AnalysisTargetRef = reportData.AnalysisTarget.Ref
		manifest.MaterialInputs.AnalysisTargetSHA256 = hashBytes(targetJSON)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, reportpkg.RunManifestFilename), manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	primarySourcePath := ""
	primarySourceID := ""
	if len(reportData.OpenablePaths) > 0 {
		primarySourcePath = filepath.Join(
			canonicalRepository,
			filepath.FromSlash(reportData.OpenablePaths[0]),
		)
		primarySourceID = manifestSourceID(runID, manifest.ReportSHA256, reportData.OpenablePaths[0])
	}
	return testRunFixture{
		runsDir: runsDir, runID: runID,
		sourceID: primarySourceID, sourcePath: primarySourcePath,
		reportData: reportData, staticHTML: staticHTML,
	}
}

func writeTargetPortfolioFixture(t *testing.T) (testRunFixture, testRunFixture) {
	t.Helper()
	repository := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.test/server-fixture\n\ngo 1.24\n",
		"batch.go":           "package fixture\n",
		"cmd/api/main.go":    "package main\nfunc main() {}\n",
		"cmd/worker/main.go": "package main\nfunc main() {}\n",
	}
	for name, content := range files {
		absolutePath := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolutePath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"add", "--", "go.mod", "batch.go", "cmd/api/main.go", "cmd/worker/main.go"},
	} {
		command := exec.Command("git", append([]string{"-C", repository}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	canonicalRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	repositoryCorpus, err := corpus.Open(context.Background(), canonicalRepository)
	if err != nil {
		t.Fatal(err)
	}
	defer repositoryCorpus.Close()
	deferred, err := snapshot.BuildContext(context.Background(), snapshot.Options{
		RepoPath: canonicalRepository, RepositoryCorpus: repositoryCorpus,
		GoTarget: runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.TargetCatalog == nil {
		t.Fatal("target catalog is unavailable")
	}
	selected := make([]string, 0, 2)
	defaultTargetRef := ""
	for _, entry := range deferred.TargetCatalog.Entries {
		target := entry.Candidate.Target
		if target.Kind != analysistarget.KindExecutablePackage ||
			(target.PackageDir != "cmd/api" && target.PackageDir != "cmd/worker") {
			continue
		}
		selected = append(selected, target.Ref)
		if target.PackageDir == "cmd/api" {
			defaultTargetRef = target.Ref
		}
	}
	if len(selected) != 2 || defaultTargetRef == "" {
		t.Fatalf("selected executable targets=%v default=%q", selected, defaultTargetRef)
	}
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: defaultTargetRef,
		TargetRefs:       selected,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	runIDs := make(map[string]string, len(container.Targets))
	for _, projection := range container.Targets {
		runID := "20260822-portfolio-" + filepath.Base(projection.Target.PackageDir)
		runIDs[projection.Target.Ref] = runID
		outcomes = append(outcomes, snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			RunID:     runID,
		})
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	containerJSON, err := container.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	portfolioJSON, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	fixtures := make(map[string]testRunFixture, len(container.Targets))
	navigationPages := make([]reportpkg.TargetNavigationPage, 0, len(container.Targets))
	navigationTargetIDs := make(map[string]string, len(container.Targets))
	defaultProgramTargetID := ""
	for _, projection := range container.Targets {
		index := reportServerProgramIndexFixture(t, projection.Target)
		runID := runIDs[projection.Target.Ref]
		navigationPages = append(navigationPages, reportpkg.TargetNavigationPage{
			RunID:            runID,
			ProgramTarget:    index.Target,
			ArtifactFilename: programindex.ArtifactFilename,
		})
		navigationTargetIDs[projection.Target.Ref] = index.Target.ID
		if projection.Target.Ref == container.DefaultTargetRef {
			defaultProgramTargetID = index.Target.ID
		}
	}
	for _, projection := range container.Targets {
		target := projection.Target.Snapshot()
		runID := runIDs[target.Ref]
		fixture := writeTestRunAt(
			t,
			runsDir,
			runID,
			canonicalRepository,
			reportpkg.ReportData{
				FormatVersion:  reportpkg.CurrentFormatVersion,
				RepoName:       filepath.Base(canonicalRepository),
				OpenablePaths:  []string{"batch.go"},
				AnalysisTarget: &target,
			},
		)
		runDir := filepath.Join(runsDir, runID)
		if err := os.WriteFile(
			filepath.Join(runDir, snapshot.TargetRunContainerArtifactFilename),
			containerJSON,
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(runDir, snapshot.TargetPagePortfolioArtifactFilename),
			portfolioJSON,
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		manifestPath := filepath.Join(runDir, reportpkg.RunManifestFilename)
		manifestJSON, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := reportpkg.DecodeRunManifest(manifestJSON)
		if err != nil {
			t.Fatal(err)
		}
		manifest.MaterialInputs.TargetRunContainerSHA256 = hashBytes(containerJSON)
		manifest.MaterialInputs.TargetPagePortfolioSHA256 = hashBytes(portfolioJSON)
		manifestJSON, err = json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, manifestJSON, 0o600); err != nil {
			t.Fatal(err)
		}
		navigation, err := reportpkg.BuildTargetNavigation(
			navigationPages,
			defaultProgramTargetID,
			navigationTargetIDs[target.Ref],
		)
		if err != nil {
			t.Fatal(err)
		}
		staticHTML, err := reportpkg.RenderHTMLWithOptions(
			&fixture.reportData,
			reportpkg.RenderOptions{TargetNavigation: navigation},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "report.html"), staticHTML, 0o600); err != nil {
			t.Fatal(err)
		}
		fixture.staticHTML = staticHTML
		fixture.targetNavigation = navigation
		fixtures[target.Ref] = fixture
	}
	initial := fixtures[container.DefaultTargetRef]
	for targetRef, fixture := range fixtures {
		if targetRef != container.DefaultTargetRef {
			return initial, fixture
		}
	}
	t.Fatal("sibling target fixture is unavailable")
	return testRunFixture{}, testRunFixture{}
}

func reportServerProgramIndexFixture(t *testing.T, target analysistarget.Target) programindex.Index {
	t.Helper()
	if target.Kind != analysistarget.KindExecutablePackage || len(target.Roots) == 0 {
		t.Fatalf("unsupported report-server target fixture: %#v", target)
	}
	sources := make([]programindex.TargetSource, 0, len(target.Roots))
	seeds := make([]programindex.TargetSeedInput, 0, len(target.Roots))
	objects := make([]programindex.ObjectInput, 0, len(target.Roots))
	for index, root := range target.Roots {
		fileRef := fmt.Sprintf("f%d", index)
		objectRef := fmt.Sprintf("root%d", index)
		location := &programindex.Location{Path: root.Path, Line: root.Line, Column: 1}
		sources = append(sources, programindex.TargetSource{FileRef: fileRef, Path: root.Path})
		seeds = append(seeds, programindex.TargetSeedInput{
			ObjectRef: objectRef,
			Kind:      programindex.SeedCallable,
			Location:  location,
		})
		objects = append(objects, programindex.ObjectInput{
			SourceRef:  objectRef,
			Kind:       programindex.ObjectFunction,
			Name:       "main",
			Visibility: programindex.VisibilityInternal,
			Location:   location,
		})
	}
	result, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("1", 64),
		SourceSHA256:   strings.Repeat("2", 64),
		Target: programindex.TargetInput{
			Language:      "go",
			Kind:          "executable",
			Name:          target.PackagePath,
			Selector:      target.PackagePath,
			Sources:       sources,
			AnchorFileRef: sources[0].FileRef,
			Seeds:         seeds,
		},
		Objects: objects,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: 0,
		},
	})
	if err != nil {
		t.Fatalf("build report-server program index fixture: %v", err)
	}
	return result
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
