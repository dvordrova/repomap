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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/programindex"
	reportpkg "github.com/dvordrova/repomap/internal/report"
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
	run, err := loadRun(fixture.runsDir, fixture.runID)
	if err != nil {
		t.Fatalf("load backing page without report.html: %v", err)
	}
	if run.id != fixture.runID || len(run.rendered) == 0 {
		t.Fatalf("restored virtual page = %#v", run)
	}
	if _, err := NewHandler(Options{
		RunsDir:      fixture.runsDir,
		InitialRunID: fixture.runID,
		Capability:   testCapability,
		OpenFile:     func(context.Context, string, int, int) error { return nil },
	}); err == nil || !strings.Contains(err.Error(), "report.html is unavailable") {
		t.Fatalf("owner without physical report.html error = %v", err)
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
		initialRunID        = "20260822-120000-api"
		siblingRunID        = "20260822-120000-worker"
		containerSHA        = "container-artifact-sha"
		portfolioSHA        = "portfolio-artifact-sha"
		programPageSHA      = "program-page-artifact-sha"
		runtimePortfolioSHA = "runtime-portfolio-artifact-sha"
		targetOutcomeSHA    = "target-outcome-artifact-sha"
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
				RuntimePortfolioSHA256:    runtimePortfolioSHA,
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
				RuntimePortfolioSHA256:    runtimePortfolioSHA,
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
	neutralInitial := initial
	neutralInitial.manifest.MaterialInputs.TargetRunContainerSHA256 = ""
	neutralInitial.manifest.MaterialInputs.TargetPagePortfolioSHA256 = ""
	neutralInitial.manifest.MaterialInputs.ProgramPagePortfolioSHA256 = programPageSHA
	neutralInitial.manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = targetOutcomeSHA
	neutralSibling := sibling
	neutralSibling.manifest.MaterialInputs.TargetRunContainerSHA256 = ""
	neutralSibling.manifest.MaterialInputs.TargetPagePortfolioSHA256 = ""
	neutralSibling.manifest.MaterialInputs.ProgramPagePortfolioSHA256 = programPageSHA
	neutralSibling.manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = targetOutcomeSHA
	if err := authorizeSiblingRun(neutralInitial, neutralSibling, "program-target-worker"); err != nil {
		t.Fatalf("language-neutral program page was not authorized: %v", err)
	}
	neutralUnbound := neutralSibling
	neutralUnbound.manifest.MaterialInputs.ProgramPagePortfolioSHA256 = "other-program-page-portfolio"
	if err := authorizeSiblingRun(neutralInitial, neutralUnbound, "program-target-worker"); err == nil {
		t.Fatal("program page outside the neutral portfolio was authorized")
	}
	neutralOutcomeMismatch := neutralSibling
	neutralOutcomeMismatch.manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = "other-target-outcome-portfolio"
	if err := authorizeSiblingRun(neutralInitial, neutralOutcomeMismatch, "program-target-worker"); err == nil {
		t.Fatal("program page with different target outcomes was authorized")
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
	runsDir    string
	runID      string
	sourceID   string
	sourcePath string
	reportData reportpkg.ReportData
	staticHTML []byte
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
	if reportData.CapturedRevision == "" {
		reportData.CapturedRevision = strings.Repeat("0", 40)
	}
	reportData.CapturedInputCount = len(reportData.OpenablePaths)
	index := reportServerStructuralProgramIndexFixture(t, reportData.RepoName)
	portfolio, err := reportpkg.NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	reportData.ProgramPortfolio = portfolio
	if err := programindex.Persist(runDir, programindex.ArtifactFilename, index); err != nil {
		t.Fatal(err)
	}
	set, err := programindex.BuildArtifactSet(
		index.Target.ID,
		[]programindex.Index{index},
		[]string{programindex.ArtifactFilename},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := programindex.PersistArtifactSet(runDir, set); err != nil {
		t.Fatal(err)
	}
	setJSON, err := programindex.EncodeArtifactSet(set)
	if err != nil {
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
	}{
		RepoName: reportData.RepoName, FilteredFiles: reportData.OpenablePaths,
	})
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
	targetJSON, err := json.Marshal(index.Target.Snapshot())
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
			ProgramTargetID:       index.Target.ID,
			ProgramTargetSHA256:   hashBytes(targetJSON),
			ProgramIndexSetSHA256: hashBytes(setJSON),
			InputPolicyVersion:    "captured-inputs-v1",
			ReportContract:        reportpkg.CurrentFormatVersion,
		},
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

func reportServerStructuralProgramIndexFixture(t *testing.T, name string) programindex.Index {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("1", 64),
		SourceSHA256:   strings.Repeat("2", 64),
		Target: programindex.TargetInput{
			Language: "fixture", Kind: "library", Name: name, Selector: "reportserver-fixture",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "batch.go"}},
			AnchorFileRef: "f1",
			Seeds:         []programindex.TargetSeedInput{},
		},
		Objects:   []programindex.ObjectInput{},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
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
