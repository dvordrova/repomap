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
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestHandlerServesTheSameManifestBoundTargetMenuAsStaticPages(t *testing.T) {
	repository, container := serverTargetNavigationContainer(t)
	runIDs := make(map[string]string, len(container.Targets))
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	for index, projection := range container.Targets {
		runID := fmt.Sprintf("20260810-12000%d-target-%d", index, index)
		runIDs[projection.Target.Ref] = runID
		outcomes = append(outcomes, snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			State:     snapshot.TargetPageReady,
			RunID:     runID,
		})
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	canonicalReports := make(map[string][]byte, len(container.Targets))
	staticNavigation := make(map[string]report.TargetNavigationPortfolio, len(container.Targets))
	var repositoryState freshness.RepositoryState
	for _, projection := range container.Targets {
		runID := runIDs[projection.Target.Ref]
		canonicalReports[runID] = writeServerTargetNavigationRun(
			t,
			runsDir,
			runID,
			repository,
			container,
			portfolio,
			projection,
		)
		manifest, err := report.ReadRunManifest(filepath.Join(runsDir, runID))
		if err != nil {
			t.Fatalf("verify target run %s: %v", runID, err)
		}
		repositoryState = manifest.RepositoryState
		staticHTML, err := os.ReadFile(filepath.Join(runsDir, runID, "report.html"))
		if err != nil {
			t.Fatal(err)
		}
		staticNavigation[runID] = embeddedTargetNavigation(t, staticHTML)
	}
	var logMu sync.Mutex
	globalLoads := 0

	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: runIDs[container.DefaultTargetRef],
		Capability:   testCapability,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return repositoryState, nil
		},
		Logf: func(format string, _ ...any) {
			if strings.HasPrefix(format, "loaded ") {
				logMu.Lock()
				globalLoads++
				logMu.Unlock()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	servedPrefix := server.URL + capabilityURLPrefix(testCapability)

	for _, projection := range container.Targets {
		runID := runIDs[projection.Target.Ref]
		reportURL := servedPrefix + "/runs/" + runID + "/report.html"
		response, err := server.Client().Get(reportURL)
		if err != nil {
			t.Fatal(err)
		}
		servedHTML, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("served target %s status=%d: %s", runID, response.StatusCode, servedHTML)
		}
		servedNavigation := embeddedTargetNavigation(t, servedHTML)
		if !reflect.DeepEqual(servedNavigation, staticNavigation[runID]) {
			t.Fatalf(
				"served target navigation for %s = %#v, want static %#v",
				runID,
				servedNavigation,
				staticNavigation[runID],
			)
		}
		if servedNavigation.DefaultTargetRef != container.DefaultTargetRef ||
			servedNavigation.CurrentTargetRef != projection.Target.Ref ||
			len(servedNavigation.Targets) != len(container.Targets) {
			t.Fatalf("served target identity for %s = %#v", runID, servedNavigation)
		}

		var sibling report.TargetNavigationItem
		currentFound := false
		for _, item := range servedNavigation.Targets {
			if item.TargetRef == projection.Target.Ref {
				currentFound = item.Available && item.Href == "#/map"
				continue
			}
			if sibling.TargetRef == "" {
				sibling = item
			}
		}
		if !currentFound || !sibling.Available {
			t.Fatalf("served current/sibling states for %s = %#v", runID, servedNavigation.Targets)
		}
		base, err := url.Parse(reportURL)
		if err != nil {
			t.Fatal(err)
		}
		reference, err := url.Parse(sibling.Href)
		if err != nil {
			t.Fatal(err)
		}
		resolved := base.ResolveReference(reference)
		if resolved.Host != base.Host ||
			!strings.HasPrefix(resolved.Path, capabilityURLPrefix(testCapability)+"/runs/") ||
			resolved.Fragment != "/map" {
			t.Fatalf("served sibling href %q resolved outside capability route: %s", sibling.Href, resolved)
		}
		siblingResponse, err := server.Client().Get(resolved.String())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, siblingResponse.Body)
		siblingResponse.Body.Close()
		if siblingResponse.StatusCode != http.StatusOK {
			t.Fatalf("served sibling href %q status=%d", sibling.Href, siblingResponse.StatusCode)
		}
	}

	for runID, want := range canonicalReports {
		got, err := os.ReadFile(filepath.Join(runsDir, runID, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("serving target %s changed canonical report.json", runID)
		}
	}
	logMu.Lock()
	defer logMu.Unlock()
	if globalLoads != 1 {
		t.Fatalf("unchanged target clicks performed %d global loads, want startup load only", globalLoads)
	}
}

func TestHandlerReloadsTargetMenuOnlyAfterManifestBoundFinalization(t *testing.T) {
	repository, container := serverTargetNavigationContainer(t)
	runIDs := make(map[string]string, len(container.Targets))
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	for index, projection := range container.Targets {
		runID := fmt.Sprintf("20260810-13000%d-target-%d", index, index)
		runIDs[projection.Target.Ref] = runID
		outcomes = append(outcomes, snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			State:     snapshot.TargetPageReady,
			RunID:     runID,
		})
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	canonicalReports := make(map[string][]byte, len(container.Targets))
	var repositoryState freshness.RepositoryState
	for _, projection := range container.Targets {
		runID := runIDs[projection.Target.Ref]
		canonicalReports[runID] = writeServerTargetNavigationRunBeforeFinalization(
			t,
			runsDir,
			runID,
			repository,
			container,
			projection,
		)
		manifest, err := report.ReadRunManifest(filepath.Join(runsDir, runID))
		if err != nil {
			t.Fatalf("verify pre-final target run %s: %v", runID, err)
		}
		if manifest.MaterialInputs.TargetPagePortfolioSHA256 != "" {
			t.Fatalf("pre-final target run %s already has portfolio authority", runID)
		}
		repositoryState = manifest.RepositoryState
	}

	var logMu sync.Mutex
	globalLoads := 0
	loadCount := func() int {
		logMu.Lock()
		defer logMu.Unlock()
		return globalLoads
	}
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: runIDs[container.DefaultTargetRef],
		Capability:   testCapability,
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			return repositoryState, nil
		},
		Logf: func(format string, _ ...any) {
			if strings.HasPrefix(format, "loaded ") {
				logMu.Lock()
				globalLoads++
				logMu.Unlock()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := loadCount(); got != 1 {
		t.Fatalf("startup global loads = %d, want 1", got)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	servedPrefix := server.URL + capabilityURLPrefix(testCapability)

	for _, projection := range container.Targets {
		runID := runIDs[projection.Target.Ref]
		html := getServerTargetReport(
			t,
			server.Client(),
			servedPrefix+"/runs/"+runID+"/report.html",
		)
		if navigation := embeddedTargetNavigationOptional(t, html); navigation != nil {
			t.Fatalf("pre-final target %s unexpectedly has navigation %#v", runID, navigation)
		}
	}
	if got := loadCount(); got != 1 {
		t.Fatalf("unchanged target clicks performed %d global loads, want startup load only", got)
	}

	for _, projection := range container.Targets {
		finalizeServerTargetNavigationRun(
			t,
			runsDir,
			runIDs[projection.Target.Ref],
			container,
			portfolio,
			projection,
		)
	}
	for index, projection := range container.Targets {
		runID := runIDs[projection.Target.Ref]
		html := getServerTargetReport(
			t,
			server.Client(),
			servedPrefix+"/runs/"+runID+"/report.html",
		)
		navigation := embeddedTargetNavigation(t, html)
		if navigation.CurrentTargetRef != projection.Target.Ref ||
			navigation.DefaultTargetRef != container.DefaultTargetRef ||
			len(navigation.Targets) != len(container.Targets) {
			t.Fatalf("finalized target %s navigation = %#v", runID, navigation)
		}
		if index == 0 {
			if got := loadCount(); got != 2 {
				t.Fatalf("manifest finalization global loads = %d, want one refresh", got)
			}
		}
	}
	if got := loadCount(); got != 2 {
		t.Fatalf("post-refresh sibling click performed %d global loads, want 2", got)
	}
	for runID, want := range canonicalReports {
		got, err := os.ReadFile(filepath.Join(runsDir, runID, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("target finalization or serving %s changed canonical report.json", runID)
		}
	}
}

func serverTargetNavigationContainer(t *testing.T) (string, snapshot.TargetRunContainer) {
	t.Helper()
	repository := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.test/served-navigation\n\ngo 1.24\n",
		"cmd/server/main.go": "package main\nfunc main() {}\n",
		"pkg/client/client.go": "package client\n" +
			"func Open() {}\n",
	}
	for name, contents := range files {
		absolute := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"add", "--", "go.mod", "cmd/server/main.go", "pkg/client/client.go"},
	} {
		command := exec.Command("git", append([]string{"-C", repository}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	deferred, err := snapshot.Build(snapshot.Options{
		RepoPath: repository, DeferAnalysisTargetResolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.TargetCatalog == nil {
		t.Fatal("target catalog is unavailable")
	}
	selected := make([]string, 0, 2)
	for _, entry := range deferred.TargetCatalog.Entries {
		if entry.Candidate.Target.PackageDir == "cmd/server" ||
			entry.Candidate.Target.Kind == analysistarget.KindModuleLibrary {
			selected = append(selected, entry.Candidate.Target.Ref)
		}
	}
	if len(selected) != 2 {
		t.Fatalf("selected target refs = %v, want server executable and module Library API", selected)
	}
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: selected[0],
		TargetRefs:       selected,
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository, container
}

func writeServerTargetNavigationRun(
	t *testing.T,
	runsDir,
	runID,
	repository string,
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	projection snapshot.TargetRunProjection,
) []byte {
	t.Helper()
	reportRaw := writeServerTargetNavigationRunBeforeFinalization(
		t,
		runsDir,
		runID,
		repository,
		container,
		projection,
	)
	finalizeServerTargetNavigationRun(
		t,
		runsDir,
		runID,
		container,
		portfolio,
		projection,
	)
	return reportRaw
}

func writeServerTargetNavigationRunBeforeFinalization(
	t *testing.T,
	runsDir,
	runID,
	repository string,
	container snapshot.TargetRunContainer,
	projection snapshot.TargetRunProjection,
) []byte {
	t.Helper()
	writeRun(t, runsDir, runID, repository, "pre-final target report")
	runDir := filepath.Join(runsDir, runID)
	reportPath := filepath.Join(runDir, "report.json")
	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var data report.ReportData
	if err := json.Unmarshal(reportRaw, &data); err != nil {
		t.Fatal(err)
	}
	target := projection.Target.Snapshot()
	data.AnalysisTarget = &target
	reportRaw, err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, reportRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	containerRaw, err := container.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, snapshot.TargetRunContainerArtifactFilename),
		containerRaw,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(runDir, report.RunManifestFilename)
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := report.DecodeRunManifest(manifestRaw)
	if err != nil {
		t.Fatal(err)
	}
	targetRaw, err := projection.Target.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	manifest.ReportSHA256 = fmt.Sprintf("%x", sha256.Sum256(reportRaw))
	manifest.MaterialInputs.AnalysisTargetRef = projection.Target.Ref
	manifest.MaterialInputs.AnalysisTargetSHA256 = fmt.Sprintf("%x", sha256.Sum256(targetRaw))
	manifest.MaterialInputs.TargetRunContainerSHA256 = fmt.Sprintf("%x", sha256.Sum256(containerRaw))
	manifestRaw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	staticHTML, err := report.RenderHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.html"), staticHTML, 0o600); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), reportRaw...)
}

func finalizeServerTargetNavigationRun(
	t *testing.T,
	runsDir,
	runID string,
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	projection snapshot.TargetRunProjection,
) {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	reportRaw, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data report.ReportData
	if err := json.Unmarshal(reportRaw, &data); err != nil {
		t.Fatal(err)
	}
	portfolioRaw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, snapshot.TargetPagePortfolioArtifactFilename),
		portfolioRaw,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(runDir, report.RunManifestFilename)
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := report.DecodeRunManifest(manifestRaw)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.TargetPagePortfolioSHA256 = fmt.Sprintf("%x", sha256.Sum256(portfolioRaw))
	navigation, err := report.BuildTargetNavigation(
		container,
		portfolio,
		projection.Target.Ref,
	)
	if err != nil {
		t.Fatal(err)
	}
	staticHTML, err := report.RenderHTMLWithOptions(
		&data,
		report.RenderOptions{TargetNavigation: navigation},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.html"), staticHTML, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestRaw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestRaw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func embeddedTargetNavigation(t *testing.T, html []byte) report.TargetNavigationPortfolio {
	t.Helper()
	navigation := embeddedTargetNavigationOptional(t, html)
	if navigation == nil {
		t.Fatal("rendered report lost manifest-bound target navigation")
	}
	return *navigation
}

func embeddedTargetNavigationOptional(
	t *testing.T,
	html []byte,
) *report.TargetNavigationPortfolio {
	t.Helper()
	const marker = `<script type="application/json" id="rm-report-data">`
	start := bytes.Index(html, []byte(marker))
	if start < 0 {
		t.Fatal("rendered report has no embedded data")
	}
	start += len(marker)
	end := bytes.Index(html[start:], []byte("</script>"))
	if end < 0 {
		t.Fatal("rendered report has unterminated embedded data")
	}
	var payload struct {
		TargetNavigation *report.TargetNavigationPortfolio `json:"target_navigation"`
	}
	if err := json.Unmarshal(html[start:start+end], &payload); err != nil {
		t.Fatalf("decode embedded report data: %v", err)
	}
	return payload.TargetNavigation
}

func getServerTargetReport(
	t *testing.T,
	client *http.Client,
	reportURL string,
) []byte {
	t.Helper()
	response, err := client.Get(reportURL)
	if err != nil {
		t.Fatal(err)
	}
	html, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("served target status=%d: %s", response.StatusCode, html)
	}
	return html
}
