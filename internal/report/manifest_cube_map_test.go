package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/cubemap"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestVerifyCubeMapProjectionRebuildsBoundView(t *testing.T) {
	runDir := t.TempDir()
	value := cubeMapViewFixture(t)
	raw, err := cubemap.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, cubemap.ArtifactFilename), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	programIndex := cubeMapProgramIndexFixture(t)
	programTarget := programIndex.Target
	view, err := NewCubeMapView(value, programTarget, programIndex.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.AnalysisTargetRef, manifest.MaterialInputs.AnalysisTargetSHA256, err =
		reportAnalysisTargetBinding(&value.Core.Target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&programTarget)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramIndexSetSHA256 = strings.Repeat("9", 64)
	manifest.MaterialInputs.CubeMapSHA256 = manifestSHA256(raw)
	portfolio, err := NewProgramPortfolio(programTarget.ID, []programindex.Index{programIndex})
	if err != nil {
		t.Fatal(err)
	}
	report := ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "fixture",
		CapturedRevision: strings.Repeat("a", 40), ProgramPortfolio: portfolio, CubeMapView: view,
	}

	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyCubeMapProjection(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyCubeMapProjection: %v", err)
	}
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err != nil {
		t.Fatalf("Go CubeMap was rejected by the Python semantic verifier: %v", err)
	}

	tampered := *view
	tampered.RefinedCore = append([]CubeMapViewCoreBlock(nil), view.RefinedCore...)
	tampered.RefinedCore[0].Purpose = "A different but still structurally valid claim."
	report.CubeMapView = &tampered
	tamperedJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyCubeMapProjection(runDir, tamperedJSON); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered projection error = %v", err)
	}

	manifest.MaterialInputs.CubeMapSHA256 = ""
	if err := manifest.VerifyCubeMapProjection(runDir, []byte(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "unbound cube map artifact") {
		t.Fatalf("unbound artifact error = %v", err)
	}
}
