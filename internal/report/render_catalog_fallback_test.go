package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestLateCatalogIncompatibilityPublishesViewOnly(t *testing.T) {
	testLateCatalogIncompatibilityPublishesViewOnly(t, "", "batch_link.go", true)
}

func TestLiteralMetacharCatalogIncompatibilityPublishesViewOnly(t *testing.T) {
	testLateCatalogIncompatibilityPublishesViewOnly(t, "", ":batch_link.go", true)
}

func TestMetacharAnalysisRootCatalogIncompatibilityPublishesViewOnly(t *testing.T) {
	testLateCatalogIncompatibilityPublishesViewOnly(t, "[service]", "batch_link.go", true)
}

func TestDeferredLiteralMetacharCatalogIncompatibilityPublishesViewOnly(t *testing.T) {
	testLateCatalogIncompatibilityPublishesViewOnly(t, "", ":batch_link.go", false)
}

func testLateCatalogIncompatibilityPublishesViewOnly(
	t *testing.T,
	analysisDirectory string,
	linkName string,
	scoped bool,
) {
	t.Helper()
	repository := t.TempDir()
	sourceRoot := filepath.Join(repository, filepath.FromSlash(analysisDirectory))
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, sourceRoot, "batch.go", "package fixture\n\nfunc Commit() {}\n")
	if err := os.Symlink("batch.go", filepath.Join(sourceRoot, linkName)); err != nil {
		t.Fatal(err)
	}
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", "--all")
	runManifestGit(
		t,
		repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)

	initialState, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	analysisRoot := initialState.Identity
	if analysisDirectory != "" {
		analysisRoot = filepath.Join(initialState.Identity, filepath.FromSlash(analysisDirectory))
	}
	currentState, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}

	runDir := t.TempDir()
	writeRunManifestMetadata(t, runDir, initialState.Identity)
	writeTestFile(t, runDir, "snapshot.json", fmt.Sprintf(`{
		"repo_name":"catalog-fallback",
		"file_tree":["batch.go",%q],
		"files_considered":2
	}`, linkName))
	writeTestFile(t, runDir, "llm_bundle.json", fmt.Sprintf(`{
		"allowed_paths":["batch.go",%q]
	}`, linkName))
	writeTestFile(t, runDir, "orientation_report.json", `{
		"project_guess":"batch fixture",
		"high_level_map":[{
			"name":"Batch Operations",
			"evidence":["batch.go:3 defines Commit"],
			"why_it_matters":"groups writes"
		}],
		"candidate_flows":[],
		"warnings":[]
	}`)

	before, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir before generation: %v", err)
	}
	var authority RunAuthority
	if scoped {
		authority, err = ConfirmRunAuthorityScoped(
			context.Background(),
			analysisRoot,
			initialState,
			currentState,
			CapturedInputPaths(before),
			false,
		)
	} else {
		authority, err = ConfirmRunAuthority(analysisRoot, initialState, currentState)
	}
	if err != nil {
		t.Fatalf("confirm run authority: %v", err)
	}
	repositoryLinkPath := linkName
	if analysisDirectory != "" {
		repositoryLinkPath = path.Join(analysisDirectory, linkName)
	}
	if strings.HasPrefix(linkName, ":") {
		captured, captureErr := freshness.CaptureInputs(
			context.Background(),
			initialState,
			[]string{repositoryLinkPath},
		)
		if captureErr != nil {
			t.Fatal(captureErr)
		}
		if len(captured) != 1 || captured[0].Kind != freshness.FileSymlink {
			t.Fatalf(
				"literal captured-input lookup did not retain the symlink for %q: %#v",
				repositoryLinkPath,
				captured,
			)
		}
	}

	symlinkCaptured := false
	var capturedKind freshness.FileKind
	for _, input := range authority.inputs {
		if input.Path == repositoryLinkPath {
			capturedKind = input.Kind
			symlinkCaptured = input.Kind == freshness.FileSymlink
		}
	}
	if analysisDirectory == "" && linkName == "batch_link.go" && !symlinkCaptured {
		t.Fatalf("authority did not retain the tracked symlink: %#v", authority.inputs)
	}
	if scoped && strings.HasPrefix(linkName, ":") && capturedKind != freshness.FileSymlink {
		t.Fatalf(
			"literal scoped authority did not retain the symlink for %q: %#v",
			repositoryLinkPath,
			authority.inputs,
		)
	}

	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("GenerateAuthorized: %v", err)
	}
	for _, name := range []string{"report.json", "report.html", RunManifestFilename} {
		info, statErr := os.Stat(filepath.Join(runDir, name))
		if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("%s was not published as a non-empty regular file: info=%v err=%v", name, info, statErr)
		}
	}

	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated ReportData
	if err := json.Unmarshal(reportJSON, &generated); err != nil {
		t.Fatal(err)
	}
	if generated.FormatVersion != CurrentFormatVersion ||
		generated.SourceIDs != nil || generated.SourceContextIDs != nil {
		t.Fatalf(
			"view-only report versions/capabilities = version %d source_ids=%v context_ids=%v",
			generated.FormatVersion,
			generated.SourceIDs,
			generated.SourceContextIDs,
		)
	}
	if !reflect.DeepEqual(generated.OpenablePaths, before.OpenablePaths) {
		t.Fatalf(
			"view-only publication changed report authority:\nbefore: %#v\nafter:  %#v",
			before.OpenablePaths,
			generated.OpenablePaths,
		)
	}
	if generated.SemanticSearch != nil {
		t.Fatalf("catalog-unavailable publication retained removed Search data: %#v", generated.SemanticSearch)
	}

	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}
	if manifest.Version != CurrentRunManifestVersion ||
		manifest.ReportFormatVersion != CurrentFormatVersion {
		t.Fatalf(
			"view-only manifest/report versions = %d/%d",
			manifest.Version,
			manifest.ReportFormatVersion,
		)
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("view-only report is not manifest-bound: %v", err)
	}
	if sourcePathNeedsLiteralGitMode(repositoryLinkPath) {
		missingAuthority := false
		for _, input := range manifest.CapturedInputs {
			if input.Path == repositoryLinkPath && input.Kind == freshness.FileMissing {
				missingAuthority = true
			}
		}
		if !missingAuthority {
			t.Fatalf(
				"literal-path ambiguity retained source authority for %q: %#v",
				repositoryLinkPath,
				manifest.CapturedInputs,
			)
		}
	}
	if _, err := manifest.SourceCatalog(); err == nil {
		t.Fatalf("view-only manifest unexpectedly exposed a source catalog: %v", err)
	}
	if _, err := manifest.WorkspaceSnapshot(); err == nil {
		t.Fatalf("view-only manifest unexpectedly exposed a workspace snapshot: %v", err)
	}
}
