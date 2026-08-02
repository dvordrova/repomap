package reportserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/navigator"
	reportpkg "github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestLoadRunsUsesFullManifestAuthorityForAtlasFirstMaterial(t *testing.T) {
	tests := []struct {
		name     string
		artifact string
	}{
		{name: "repository Atlas", artifact: repositoryatlas.ArtifactFilename},
		{name: "Navigator status", artifact: navigator.StatusArtifactFilename},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			runsDir := t.TempDir()
			const runID = "20260802-104638-atlas-first"
			writeRun(t, runsDir, runID, repository, "saved report")
			runDir := filepath.Join(runsDir, runID)
			writeAtlasFirstMaterial(t, runDir)

			h := &handler{runsDir: runsDir}
			runs, err := h.loadRuns()
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != 1 || runs[0].Manifest == nil {
				t.Fatalf("verified Atlas-first run = %#v", runs)
			}

			artifactPath := filepath.Join(runDir, test.artifact)
			artifact, err := os.ReadFile(artifactPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(artifactPath, append(artifact, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}

			runs, err = h.loadRuns()
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != 1 || runs[0].Manifest != nil {
				t.Fatalf("tampered %s restored report authority: %#v", test.artifact, runs)
			}

			runRoot, err := os.OpenRoot(runDir)
			if err != nil {
				t.Fatal(err)
			}
			defer runRoot.Close()
			if _, err := h.readAuthorizedRun(runID, runRoot); err == nil {
				t.Fatalf("readAuthorizedRun accepted tampered %s", test.artifact)
			}
		})
	}
}

func writeAtlasFirstMaterial(t *testing.T, runDir string) {
	t.Helper()
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{{
			ID: "repository", Kind: repositoryatlas.UnitRepository, Name: "fixture",
		}},
	}
	atlasJSON, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		t.Fatal(err)
	}
	atlas, err = repositoryatlas.DecodeCanonicalJSON(atlasJSON)
	if err != nil {
		t.Fatal(err)
	}
	product, err := navigator.CompileProduct(navigator.ProductInput{Atlas: atlas})
	if err != nil {
		t.Fatal(err)
	}
	result, err := product.EmptyRecord()
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := navigator.EncodeRecommendationRecord(result)
	if err != nil {
		t.Fatal(err)
	}
	status := product.PreparedStatus()
	statusJSON, err := navigator.EncodeStatus(status)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		repositoryatlas.ArtifactFilename: atlasJSON,
		navigator.RecordArtifactFilename: resultJSON,
		navigator.StatusArtifactFilename: statusJSON,
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	reportPath := filepath.Join(runDir, "report.json")
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var reportData reportpkg.ReportData
	if err := json.Unmarshal(reportJSON, &reportData); err != nil {
		t.Fatal(err)
	}
	reportData.RepositoryAtlas = &atlas
	reportData.Navigator = &reportpkg.NavigatorReportProduct{
		Version: navigator.ProductVersion,
		State:   status.State,
	}
	reportJSON, err = json.Marshal(reportData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, reportJSON, 0o600); err != nil {
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
	manifest.ReportSHA256 = serverMaterialSHA256(reportJSON)
	manifest.MaterialInputs.RepositoryAtlasSHA256 = serverMaterialSHA256(atlasJSON)
	manifest.MaterialInputs.NavigatorResultSHA256 = serverMaterialSHA256(resultJSON)
	manifest.MaterialInputs.NavigatorStatusSHA256 = serverMaterialSHA256(statusJSON)
	manifestJSON, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reportpkg.ReadRunManifest(runDir); err != nil {
		t.Fatalf("ReadRunManifest fixture: %v", err)
	}
}

func serverMaterialSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
