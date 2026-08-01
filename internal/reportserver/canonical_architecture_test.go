package reportserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	reportpkg "github.com/dvordrova/repomap/internal/report"
)

func TestOrdinaryStaticAndServerReportsShareCanonicalArchitectureBase(t *testing.T) {
	t.Parallel()

	runsDir := t.TempDir()
	repository := t.TempDir()
	writeCanonicalArchitectureFixture(t, repository, "main.go", []byte("package main\n\nfunc main() {}\n"))
	runCanonicalArchitectureGit(t, repository, "init", "--quiet")
	runCanonicalArchitectureGit(t, repository, "add", "--all")
	runCanonicalArchitectureGit(
		t,
		repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := reportpkg.ConfirmRunAuthority(repository, initial, current)
	if err != nil {
		t.Fatal(err)
	}
	const runID = "20260801-120000-canonical-architecture"
	runDir := filepath.Join(runsDir, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeCanonicalArchitectureFixture(t, runDir, "metadata.json", []byte(`{
		"repo_name":"canonical-architecture",
		"repo_path":`+canonicalArchitectureJSONString(t, repository)+`,
		"created_at":"2026-08-01T12:00:00Z"
	}`))
	writeCanonicalArchitectureFixture(t, runDir, "snapshot.json", []byte(`{
		"repo_name":"canonical-architecture",
		"file_tree":["main.go"],
		"files_considered":1
	}`))
	modelBundleJSON := []byte(`{
		"allowed_paths":["main.go"],
		"go": {
			"module_summaries":[{"module_path":"example.com/canonical","module_dir":"."}],
			"important_edges":[{"from":"example.com/canonical/cmd","to":"example.com/canonical/internal/service"}]
		}
	}`)
	writeCanonicalArchitectureFixture(t, runDir, "llm_bundle.json", modelBundleJSON)
	writeOrientationSelectionFixture(t, runDir, modelBundleJSON)
	writeCanonicalArchitectureFixture(t, runDir, "orientation_report.json", []byte(`{
		"project_guess":"Canonical architecture fixture",
		"high_level_map":[],
		"candidate_flows":[]
	}`))

	// These developer/replay artifacts are deliberately malformed. Their mere
	// presence must not participate in an ordinary saved report.
	for _, name := range []string{
		"semantic_artifacts.json",
		"golden_mechanism_facts.json",
		"golden_mechanism_semantic.json",
		"golden_mechanism_probe_attempt.json",
		"mechanism_v1.json",
		"repository_onboarding.json",
		"paved_paths.json",
	} {
		writeCanonicalArchitectureFixture(t, runDir, name, []byte(`{"dev_replay":`))
	}
	writeCanonicalArchitectureFixture(t, runDir, reportpkg.ArchitectureSynthesisFile, []byte(`{"broken"`))

	if err := reportpkg.GenerateAuthorized(runDir, authority); err != nil {
		t.Fatal(err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var canonical reportpkg.ReportData
	if err := json.Unmarshal(reportJSON, &canonical); err != nil {
		t.Fatal(err)
	}
	if canonical.ArchitectureCanvas == nil || canonical.ArchitectureCanvas.Fallback ||
		canonical.ArchitectureCanvas.ArchitectureSource != "local_packages" ||
		len(canonical.ArchitectureCanvas.Components) != 2 ||
		len(canonical.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("canonical architecture canvas = %#v", canonical.ArchitectureCanvas)
	}
	if len(canonical.SemanticArtifacts) != 0 || len(canonical.UserMechanisms) != 0 ||
		canonical.Operations != nil {
		t.Fatalf("developer artifacts changed ordinary report: semantic=%#v mechanisms=%#v operations=%#v", canonical.SemanticArtifacts, canonical.UserMechanisms, canonical.Operations)
	}

	staticHTML, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	staticData := embeddedCanonicalReportData(t, staticHTML)

	httpHandler, err := NewHandler(Options{
		RunsDir: runsDir, InitialRunID: runID, Capability: testCapability,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpHandler)
	defer server.Close()
	response, err := server.Client().Get(
		server.URL + capabilityURLPrefix(testCapability) + "/runs/" + runID + "/report.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	servedHTML, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("served report status = %d", response.StatusCode)
	}
	servedData := embeddedCanonicalReportData(t, servedHTML)

	staticCanvas, err := json.Marshal(staticData.ArchitectureCanvas)
	if err != nil {
		t.Fatal(err)
	}
	servedCanvas, err := json.Marshal(servedData.ArchitectureCanvas)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(staticCanvas, servedCanvas) {
		t.Fatalf("static/server architecture differ:\nstatic=%s\nserved=%s", staticCanvas, servedCanvas)
	}
	if !reflect.DeepEqual(
		canonicalArchitectureComponentIDs(staticData.ArchitectureCanvas),
		canonicalArchitectureComponentIDs(servedData.ArchitectureCanvas),
	) {
		t.Fatalf("static/server component IDs differ")
	}

	writeCanonicalArchitectureFixture(
		t,
		runDir,
		"orientation_context_selection.v1.json",
		[]byte(`{"version":1}`),
	)
	reloaded := &handler{runsDir: runsDir}
	if err := reloaded.reloadRuns(); err != nil {
		t.Fatal(err)
	}
	runs := reloaded.runsSnapshot()
	if len(runs) != 1 || runs[0].Manifest != nil || runs[0].Report == nil {
		t.Fatalf("tampered selection restored report authority: %#v", runs)
	}
}

func runCanonicalArchitectureGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func canonicalArchitectureJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func writeCanonicalArchitectureFixture(t *testing.T, runDir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func embeddedCanonicalReportData(t *testing.T, html []byte) reportpkg.ReportData {
	t.Helper()
	const marker = `<script type="application/json" id="rm-report-data">`
	start := bytes.Index(html, []byte(marker))
	if start < 0 {
		t.Fatal("report data script is missing")
	}
	start += len(marker)
	end := bytes.Index(html[start:], []byte(`</script>`))
	if end < 0 {
		t.Fatal("report data script is unterminated")
	}
	var data reportpkg.ReportData
	if err := json.Unmarshal(html[start:start+end], &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func canonicalArchitectureComponentIDs(canvas *reportpkg.ArchitectureCanvas) []string {
	if canvas == nil {
		return nil
	}
	ids := make([]string, 0, len(canvas.Components))
	for _, component := range canvas.Components {
		ids = append(ids, string(component.ID))
	}
	return ids
}
