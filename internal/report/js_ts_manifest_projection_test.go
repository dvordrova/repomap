package report

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestVerifyJSTSProjectionBindsArtifactProgramIndexAndBothViews(t *testing.T) {
	runDir, manifest, reportJSON := reportJSTSManifestFixture(t)
	if err := manifest.VerifyJSTSProjection(runDir, reportJSON); err != nil {
		t.Fatalf("exact JavaScript/TypeScript projection: %v", err)
	}

	tamperedReport, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		t.Fatal(err)
	}
	tamperedReport.CrossSurfacePathView.Paths[0].Steps[3].Label = "Invented call edge"
	tamperedJSON, err := json.Marshal(tamperedReport)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyJSTSProjection(runDir, tamperedJSON); err == nil ||
		!strings.Contains(err.Error(), "do not match exact project artifact") {
		t.Fatalf("tampered report projection error = %v", err)
	}

	tamperedManifest := manifest
	tamperedManifest.MaterialInputs.JSTSProjectSHA256 = strings.Repeat("0", 64)
	if err := tamperedManifest.VerifyJSTSProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "project sha256 mismatch") {
		t.Fatalf("tampered artifact binding error = %v", err)
	}

	unbound := manifest
	unbound.MaterialInputs.JSTSProjectSHA256 = ""
	if err := unbound.VerifyJSTSProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "unbound JavaScript/TypeScript project artifact") {
		t.Fatalf("unbound artifact error = %v", err)
	}
}

func TestJSTSProjectionRejectsAnotherValidProgramIndex(t *testing.T) {
	result, _ := reportJSTSProjectFixture(t)
	other := reportProgramIndexFixture(t, "typescript", "application")
	if _, err := NewJSTSSurfaceCatalogView(result, other); err == nil ||
		!strings.Contains(err.Error(), "does not bind the exact ProgramTarget and ProgramIndex") {
		t.Fatalf("unrelated ProgramIndex error = %v", err)
	}
	if _, err := NewCrossSurfacePathView(result, other); err == nil ||
		!strings.Contains(err.Error(), "does not bind the exact ProgramTarget and ProgramIndex") {
		t.Fatalf("unrelated cross-surface ProgramIndex error = %v", err)
	}
}

func TestReadRunDirRestoresJSTSSurfacesPathsAndSourceAuthority(t *testing.T) {
	result, index := reportJSTSProjectFixture(t)
	runDir := t.TempDir()
	writeJSTSProjectionArtifacts(t, runDir, result, index)
	_, activityRaw := reportActivityEntrypointFixture(t, index)
	writeReportProgramFile(t, filepath.Join(runDir, activityentrypoint.ArtifactFilename), activityRaw)
	_, coreRaw := reportCoreMapFixture(t, index)
	writeReportProgramFile(t, filepath.Join(runDir, coremap.ArtifactFilename), coreRaw)
	_, catalogRaw, selectedRaw, usageRaw := reportIntegrationUsageFixture(t, index)
	writeReportProgramFile(t, filepath.Join(runDir, dependencies.ArtifactFilename), catalogRaw)
	writeReportProgramFile(t, filepath.Join(runDir, integrationdependency.ArtifactFilename), selectedRaw)
	writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), usageRaw)
	_, activityPathRaw := reportActivityPathFixture(t, index, activityRaw, selectedRaw, usageRaw)
	writeReportProgramFile(t, filepath.Join(runDir, activitypath.ArtifactFilename), activityPathRaw)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"caltodo"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"caltodo"}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	if data.JSTSSurfaceCatalogView == nil || data.CrossSurfacePathView == nil ||
		data.JSTSSurfaceCatalogView.ProgramTargetID != index.Target.ID ||
		data.CrossSurfacePathView.ProgramIndexSHA256 != index.SHA256 {
		t.Fatalf("restored JavaScript/TypeScript views = %#v / %#v", data.JSTSSurfaceCatalogView, data.CrossSurfacePathView)
	}
	for _, sourcePath := range []string{
		"client/src/pages/settings.tsx", "server/routes.ts", "shared/schema.ts", "server/storage.ts",
	} {
		found := false
		for _, openable := range data.OpenablePaths {
			if openable == sourcePath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("JavaScript/TypeScript source path %q is not openable: %#v", sourcePath, data.OpenablePaths)
		}
	}
}

func reportJSTSManifestFixture(t *testing.T) (string, RunManifest, []byte) {
	t.Helper()
	result, index := reportJSTSProjectFixture(t)
	runDir := t.TempDir()
	setRaw, projectRaw := writeJSTSProjectionArtifacts(t, runDir, result, index)

	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	surfaces, err := NewJSTSSurfaceCatalogView(result, index)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := NewCrossSurfacePathView(result, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.ValidateSurfaceJoins(surfaces); err != nil {
		t.Fatal(err)
	}
	reportJSON, err := json.Marshal(ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "caltodo",
		CapturedRevision: strings.Repeat("a", 40), ProgramPortfolio: portfolio,
		JSTSSurfaceCatalogView: surfaces, CrossSurfacePathView: paths,
		OpenablePaths: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}

	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&index.Target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(setRaw)
	manifest.MaterialInputs.JSTSProjectSHA256 = manifestSHA256(projectRaw)
	manifest.MaterialInputs.CoreMapSHA256 = strings.Repeat("1", 64)
	manifest.MaterialInputs.ActivityEntrypointsSHA256 = strings.Repeat("2", 64)
	manifest.MaterialInputs.DependencyCatalogSHA256 = strings.Repeat("3", 64)
	manifest.MaterialInputs.IntegrationDependenciesSHA256 = strings.Repeat("4", 64)
	manifest.MaterialInputs.IntegrationUsageSHA256 = strings.Repeat("5", 64)
	manifest.MaterialInputs.ActivityPathsSHA256 = strings.Repeat("6", 64)
	return runDir, manifest, reportJSON
}

func writeJSTSProjectionArtifacts(
	t *testing.T,
	runDir string,
	result jstsproject.Result,
	index programindex.Index,
) ([]byte, []byte) {
	t.Helper()
	indexRaw, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, jstsproject.ProgramIndexFilename), indexRaw)
	set, err := programindex.NewArtifactSet(index.Target.ID, []programindex.ArtifactSetEntry{{
		TargetID: index.Target.ID, Filename: jstsproject.ProgramIndexFilename, IndexSHA256: index.SHA256,
	}})
	if err != nil {
		t.Fatal(err)
	}
	setRaw, err := programindex.EncodeArtifactSet(set)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, programindex.ArtifactSetFilename), setRaw)
	projectRaw, err := jstsproject.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, jstsproject.ArtifactFilename), projectRaw)
	return setRaw, projectRaw
}
