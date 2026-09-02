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

func TestVerifiedRunsStartWithoutReportsAndLoadPagesLazily(t *testing.T) {
	fixture := writeVerifiedRunsFixture(t)
	ownerReportPath := filepath.Join(fixture.owner.runsDir, fixture.owner.runID, "report.json")
	siblingReportPath := filepath.Join(fixture.sibling.runsDir, fixture.sibling.runID, "report.json")
	ownerReport := readTestFile(t, ownerReportPath)
	for _, reportPath := range []string{ownerReportPath, siblingReportPath} {
		if err := os.WriteFile(reportPath, []byte(`{"invalid":"during-startup"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(
		fixture.owner.runsDir, fixture.owner.runID, "report.html",
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(
		fixture.sibling.runsDir, fixture.sibling.runID, "report.html",
	)); !os.IsNotExist(err) {
		t.Fatalf("sibling physical report.html exists or cannot be inspected: %v", err)
	}

	var openedPath string
	handler, err := NewHandler(Options{
		RunsDir:      fixture.owner.runsDir,
		InitialRunID: fixture.owner.runID,
		Capability:   testCapability,
		VerifiedRuns: fixture.receipts,
		OpenFile: func(_ context.Context, absolutePath string, _, _ int) error {
			openedPath = absolutePath
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewHandler read or rendered a verified report during startup: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)

	if err := os.WriteFile(ownerReportPath, ownerReport, 0o600); err != nil {
		t.Fatal(err)
	}
	ownerURL := baseURL + "/runs/" + fixture.owner.runID + "/report.html"
	ownerResponse, err := server.Client().Get(ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	ownerHTML := readResponse(t, ownerResponse, http.StatusOK)
	if !bytes.Contains(ownerHTML, []byte(fixture.owner.sourceID)) {
		t.Fatal("first verified page did not derive manifest-authorized source ids")
	}

	openResponse := postOpen(t, baseURL, openRequest{
		RunID: fixture.owner.runID, SourceID: fixture.owner.sourceID, Line: 1, Column: 1,
	})
	var opened map[string]any
	decodeResponse(t, openResponse, http.StatusOK, &opened)
	if opened["status"] != "opened" || openedPath != fixture.owner.sourcePath {
		t.Fatalf("verified source open=%#v path=%q", opened, openedPath)
	}

	if err := os.WriteFile(ownerReportPath, []byte(`{"invalid":"after-first-get"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	repeatedOwnerResponse, err := server.Client().Get(ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	repeatedOwnerHTML := readResponse(t, repeatedOwnerResponse, http.StatusOK)
	if !bytes.Equal(repeatedOwnerHTML, ownerHTML) {
		t.Fatal("repeated verified page GET was not served from the rendered cache")
	}

	if !bytes.Equal(readTestFile(t, siblingReportPath), []byte(`{"invalid":"during-startup"}`)) {
		t.Fatal("unvisited sibling report was loaded or changed")
	}
}

func TestVerifiedRunsRejectMismatchedReceipt(t *testing.T) {
	fixture := writeVerifiedRunsFixture(t)
	if _, err := NewHandler(Options{
		RunsDir: fixture.owner.runsDir, InitialRunID: fixture.owner.runID,
		Capability: testCapability, VerifiedRuns: fixture.receipts[1:],
		OpenFile: func(context.Context, string, int, int) error { return nil },
	}); err == nil || !strings.Contains(err.Error(), "do not contain the initial run") {
		t.Fatalf("mismatched verified receipt error = %v", err)
	}
	if _, err := NewHandler(Options{
		RunsDir: fixture.owner.runsDir, InitialRunID: fixture.owner.runID,
		Capability: testCapability, VerifiedRuns: fixture.unboundReceipts,
		OpenFile: func(context.Context, string, int, int) error { return nil },
	}); err == nil || !strings.Contains(err.Error(), "repository authority mismatch") {
		t.Fatalf("mismatched one-page portfolio receipts error = %v", err)
	}
}

func TestVerifiedRunsServeAuthorizedVirtualSiblingWithoutPhysicalHTML(t *testing.T) {
	fixture := writeVerifiedRunsFixture(t)
	if _, err := os.Stat(filepath.Join(
		fixture.sibling.runsDir, fixture.sibling.runID, "report.html",
	)); !os.IsNotExist(err) {
		t.Fatalf("sibling physical report.html exists or cannot be inspected: %v", err)
	}
	handler, err := NewHandler(Options{
		RunsDir:      fixture.owner.runsDir,
		InitialRunID: fixture.owner.runID,
		Capability:   testCapability,
		VerifiedRuns: fixture.receipts,
		OpenFile:     func(context.Context, string, int, int) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(
		server.URL + capabilityURLPrefix(testCapability) +
			"/runs/" + fixture.sibling.runID + "/report.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	served := readResponse(t, response, http.StatusOK)
	// The served page carries its own editor-open authority. It no longer
	// links back to the owner run: one page already holds every target.
	if !bytes.Contains(served, []byte(fixture.sibling.sourceID)) {
		t.Fatal("virtual sibling omitted its source authority")
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

func TestLoadTargetNavigationRequiresPortfolioAuthority(t *testing.T) {
	navigation, err := loadTargetNavigation("unneeded", reportpkg.RunManifest{})
	if err == nil || navigation != nil ||
		!strings.Contains(err.Error(), "neutral program page authority is missing") {
		t.Fatalf("missing program page authority navigation = %#v, err = %v", navigation, err)
	}
	manifest := reportpkg.RunManifest{
		MaterialInputs: reportpkg.MaterialInputs{ProgramPagePortfolioSHA256: strings.Repeat("a", 64)},
	}
	if _, err := loadTargetNavigation(t.TempDir(), manifest); err == nil ||
		!strings.Contains(err.Error(), "neutral target outcome authority is missing") {
		t.Fatalf("neutral outcome authority was not required: %v", err)
	}
	manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = strings.Repeat("b", 64)
	if _, err := loadTargetNavigation(t.TempDir(), manifest); err == nil ||
		!strings.Contains(err.Error(), "load target navigation") {
		t.Fatalf("bound neutral portfolio was not required: %v", err)
	}
}

func TestSiblingPagesRequirePortfolioRouteAndManifestBinding(t *testing.T) {
	const (
		initialRunID     = "20260822-120000-api"
		siblingRunID     = "20260822-120000-worker"
		programPageSHA   = "program-page-artifact-sha"
		targetOutcomeSHA = "target-outcome-artifact-sha"
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
				ProgramTargetID:              "program-target-api",
				ProgramPagePortfolioSHA256:   programPageSHA,
				TargetOutcomePortfolioSHA256: targetOutcomeSHA,
				SelectedRevision:             "before-change",
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
				ProgramTargetID:              "program-target-worker",
				ProgramPagePortfolioSHA256:   programPageSHA,
				TargetOutcomePortfolioSHA256: targetOutcomeSHA,
				SelectedRevision:             "after-change",
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
	neutralUnbound := sibling
	neutralUnbound.manifest.MaterialInputs.ProgramPagePortfolioSHA256 = "other-program-page-portfolio"
	if err := authorizeSiblingRun(initial, neutralUnbound, "program-target-worker"); err == nil {
		t.Fatal("program page outside the neutral portfolio was authorized")
	}
	neutralOutcomeMismatch := sibling
	neutralOutcomeMismatch.manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = "other-target-outcome-portfolio"
	if err := authorizeSiblingRun(initial, neutralOutcomeMismatch, "program-target-worker"); err == nil {
		t.Fatal("program page with different target outcomes was authorized")
	}
	foreign := sibling
	foreign.manifest.RepositoryState.Identity = "/other-repo"
	if err := authorizeSiblingRun(initial, foreign, "program-target-worker"); err == nil {
		t.Fatal("sibling page from another repository was authorized")
	}
	missingNeutral := sibling
	missingNeutral.manifest.MaterialInputs.ProgramPagePortfolioSHA256 = ""
	missingNeutral.manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = ""
	if err := authorizeSiblingRun(initial, missingNeutral, "program-target-worker"); err == nil {
		t.Fatal("sibling without neutral portfolio authority was accepted")
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
	runsDir          string
	runID            string
	sourceID         string
	sourcePath       string
	reportData       reportpkg.ReportData
	targetNavigation *reportpkg.TargetNavigationPortfolio
	staticHTML       []byte
}

type verifiedRunsFixture struct {
	owner           testRunFixture
	sibling         testRunFixture
	receipts        []reportpkg.VerifiedRunReceipt
	unboundReceipts []reportpkg.VerifiedRunReceipt
}

type reportServerGraphFixture struct {
	index      programindex.Index
	groups     groupindex.Index
	reduced    documentationreduce.Result
	groupsRaw  []byte
	reducedRaw []byte
}

func writeVerifiedRunsFixture(t *testing.T) verifiedRunsFixture {
	t.Helper()
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "batch.go"), []byte("package batch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	baseReport := reportpkg.ReportData{
		FormatVersion: reportpkg.CurrentFormatVersion,
		RepoName:      filepath.Base(repository),
		OpenablePaths: []string{"batch.go"},
	}
	owner := writeTestRunAtWithTargetName(
		t, runsDir, "20260822-120000-owner", repository, baseReport, "owner-target",
	)
	sibling := writeTestRunAtWithTargetName(
		t, runsDir, "20260822-120000-sibling", repository, baseReport, "sibling-target",
	)
	unboundReceipts := make([]reportpkg.VerifiedRunReceipt, 0, 2)
	for _, fixture := range []testRunFixture{owner, sibling} {
		receipt, readErr := reportpkg.ReadVerifiedRunManifest(
			filepath.Join(fixture.runsDir, fixture.runID),
		)
		if readErr != nil {
			t.Fatalf("read verified run %s: %v", fixture.runID, readErr)
		}
		unboundReceipts = append(unboundReceipts, receipt)
	}
	owner, sibling = bindVerifiedRunsFixture(t, owner, sibling)
	receipts := make([]reportpkg.VerifiedRunReceipt, 0, 2)
	for _, fixture := range []testRunFixture{owner, sibling} {
		receipt, readErr := reportpkg.ReadVerifiedRunManifest(
			filepath.Join(fixture.runsDir, fixture.runID),
		)
		if readErr != nil {
			t.Fatalf("read bound verified run %s: %v", fixture.runID, readErr)
		}
		receipts = append(receipts, receipt)
	}
	return verifiedRunsFixture{
		owner: owner, sibling: sibling, receipts: receipts, unboundReceipts: unboundReceipts,
	}
}

func bindVerifiedRunsFixture(
	t *testing.T,
	owner testRunFixture,
	sibling testRunFixture,
) (testRunFixture, testRunFixture) {
	t.Helper()
	ownerTarget := owner.reportData.GroupGraph.Indexes[0].Target
	siblingTarget := sibling.reportData.GroupGraph.Indexes[0].Target
	pages, err := programpage.Build(ownerTarget.ID, []programpage.Page{
		{Target: ownerTarget, RunID: owner.runID},
		{Target: siblingTarget, RunID: sibling.runID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerSelected, err := targetoutcome.NewSelectedTargetWithLanguages(
		targetoutcome.LanguageGroup("fixture"), []string{"fixture"},
		targetoutcome.ScopeLibrary, "owner-target", ownerTarget.Selector,
	)
	if err != nil {
		t.Fatal(err)
	}
	siblingSelected, err := targetoutcome.NewSelectedTargetWithLanguages(
		targetoutcome.LanguageGroup("fixture"), []string{"fixture"},
		targetoutcome.ScopeLibrary, "sibling-target", siblingTarget.Selector,
	)
	if err != nil {
		t.Fatal(err)
	}
	ownerOutcome, err := targetoutcome.NewAnalyzed(ownerSelected, ownerTarget, owner.runID)
	if err != nil {
		t.Fatal(err)
	}
	siblingOutcome, err := targetoutcome.NewAnalyzed(siblingSelected, siblingTarget, sibling.runID)
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := targetoutcome.Build(
		ownerSelected.ID, []targetoutcome.Outcome{ownerOutcome, siblingOutcome},
	)
	if err != nil {
		t.Fatal(err)
	}
	pageRaw, err := pages.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	outcomeRaw, err := outcomes.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	outcomeView, err := reportpkg.NewTargetOutcomePortfolioView(outcomes, pages)
	if err != nil {
		t.Fatal(err)
	}
	indexes := []groupindex.Index{
		owner.reportData.GroupGraph.Indexes[0],
		sibling.reportData.GroupGraph.Indexes[0],
	}
	navigationPages := []reportpkg.TargetNavigationPage{
		{
			RunID: owner.runID, ProgramTarget: ownerTarget,
			ArtifactFilename: programindex.ArtifactFilename,
		},
		{
			RunID: sibling.runID, ProgramTarget: siblingTarget,
			ArtifactFilename: programindex.ArtifactFilename,
		},
	}
	bound := []testRunFixture{owner, sibling}
	for position := range bound {
		fixture := &bound[position]
		currentTarget := ownerTarget
		if fixture.runID == sibling.runID {
			currentTarget = siblingTarget
		}
		groupGraph, graphErr := reportpkg.NewGroupGraphView(indexes, currentTarget.ID)
		if graphErr != nil {
			t.Fatal(graphErr)
		}
		navigation, navigationErr := reportpkg.BuildTargetNavigation(
			navigationPages, ownerTarget.ID, currentTarget.ID,
		)
		if navigationErr != nil {
			t.Fatal(navigationErr)
		}
		fixture.reportData.GroupGraph = groupGraph
		fixture.reportData.TargetOutcomePortfolio = outcomeView
		fixture.targetNavigation = navigation
		reportRaw, marshalErr := json.Marshal(fixture.reportData)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		html, renderErr := reportpkg.RenderHTMLWithOptions(
			&fixture.reportData, reportpkg.RenderOptions{TargetNavigation: navigation},
		)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		runDir := filepath.Join(fixture.runsDir, fixture.runID)
		for _, artifact := range []struct {
			name string
			data []byte
		}{
			{name: programpage.ArtifactFilename, data: pageRaw},
			{name: targetoutcome.ArtifactFilename, data: outcomeRaw},
			{name: "report.json", data: reportRaw},
			{name: "report.html", data: html},
		} {
			if writeErr := os.WriteFile(
				filepath.Join(runDir, artifact.name), artifact.data, 0o600,
			); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		manifestRaw := readTestFile(t, filepath.Join(runDir, reportpkg.RunManifestFilename))
		var manifest reportpkg.RunManifest
		if unmarshalErr := json.Unmarshal(manifestRaw, &manifest); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		manifest.ReportSHA256 = hashBytes(reportRaw)
		manifest.MaterialInputs.ProgramPagePortfolioSHA256 = hashBytes(pageRaw)
		manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = hashBytes(outcomeRaw)
		manifestRaw, marshalErr = json.Marshal(manifest)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(
			filepath.Join(runDir, reportpkg.RunManifestFilename), manifestRaw, 0o600,
		); writeErr != nil {
			t.Fatal(writeErr)
		}
		if fixture.runID != owner.runID {
			if removeErr := os.Remove(filepath.Join(runDir, "report.html")); removeErr != nil {
				t.Fatal(removeErr)
			}
		}
		fixture.staticHTML = html
		fixture.sourceID = manifestSourceID(
			fixture.runID, manifest.ReportSHA256, fixture.reportData.OpenablePaths[0],
		)
	}
	return bound[0], bound[1]
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
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
	return writeTestRunAtWithTargetName(
		t, runsDir, runID, canonicalRepository, reportData, reportData.RepoName,
	)
}

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
	reportData.CapturedInputCount = len(reportData.OpenablePaths)
	graph := reportServerGraphAuthorityFixture(
		t, reportServerStructuralProgramIndexFixture(t, targetName),
	)
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
		targetoutcome.LanguageGroup(index.Target.Language),
		[]string{index.Target.Language},
		targetoutcome.ScopeLibrary,
		index.Target.Name,
		index.Target.Selector,
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
		RunID:            runID,
		ProgramTarget:    index.Target.Snapshot(),
		ArtifactFilename: programindex.ArtifactFilename,
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
	for _, artifact := range []struct {
		name string
		data []byte
	}{
		{name: programpage.ArtifactFilename, data: pageRaw},
		{name: targetoutcome.ArtifactFilename, data: outcomeRaw},
	} {
		if err := os.WriteFile(filepath.Join(runDir, artifact.name), artifact.data, 0o600); err != nil {
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
	dependencyJSON, err := os.ReadFile(filepath.Join(runDir, dependencies.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := documentationreduce.Persist(runDir, graph.reduced); err != nil {
		t.Fatal(err)
	}
	if err := groupindex.Persist(runDir, graph.groups); err != nil {
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
			SelectedRevision:             state.Head,
			ProgramTargetID:              index.Target.ID,
			ProgramTargetSHA256:          hashBytes(targetJSON),
			ProgramIndexSetSHA256:        hashBytes(setJSON),
			DependencyCatalogSHA256:      hashBytes(dependencyJSON),
			ReducedDocumentationSHA256:   hashBytes(graph.reducedRaw),
			GroupsIndexSHA256:            hashBytes(graph.groupsRaw),
			ProgramPagePortfolioSHA256:   hashBytes(pageRaw),
			TargetOutcomePortfolioSHA256: hashBytes(outcomeRaw),
			InputPolicyVersion:           "captured-inputs-v1",
			ReportContract:               reportpkg.CurrentFormatVersion,
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
		reportData: reportData, targetNavigation: navigation, staticHTML: staticHTML,
	}
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
