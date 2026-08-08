package reportserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	reportpkg "github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestServeAtlasFirstRussianReportWithoutLegacyLocalizationStatusShowsActive(t *testing.T) {
	repository := t.TempDir()
	runsDir := t.TempDir()
	const runID = "20260803-005724-atlas-first-ru"
	writeRun(t, runsDir, runID, repository, "saved report")
	runDir := filepath.Join(runsDir, runID)
	writeAtlasFirstMaterial(t, runDir)

	metadataPath := filepath.Join(runDir, "metadata.json")
	metadataJSON, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var runMetadata metadata
	if err := json.Unmarshal(metadataJSON, &runMetadata); err != nil {
		t.Fatal(err)
	}
	runMetadata.EffectiveOptions.ReportLanguage = "ru"
	metadataJSON, err = json.Marshal(runMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, metadataJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, reportpkg.PresentationLocalizationStatusFile)); !os.IsNotExist(err) {
		t.Fatalf("Atlas-first fixture unexpectedly has a legacy localization status: %v", err)
	}

	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: runID,
		Capability:   testCapability,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(
		server.URL + capabilityURLPrefix(testCapability) + "/runs/" + runID + "/report.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("serve status = %d: %s", response.StatusCode, body)
	}
	for _, marker := range [][]byte{
		[]byte(`<html lang="ru">`),
		[]byte(`rm-localization-status--stage_owned`),
		[]byte(`data-rm-message="main.localization.ru_active"`),
	} {
		if !bytes.Contains(body, marker) {
			t.Fatalf("served stage-owned RU report is missing %q", marker)
		}
	}
	if bytes.Contains(body, []byte(`data-rm-message="main.localization.ru_unavailable_canonical_en"`)) {
		t.Fatal("served stage-owned RU report showed the legacy unavailable warning")
	}
}

func TestLoadRunsUsesFullManifestAuthorityForAtlasFirstMaterial(t *testing.T) {
	tests := []struct {
		name     string
		artifact string
	}{{name: "repository Atlas", artifact: repositoryatlas.ArtifactFilename}}
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
	for name, data := range map[string][]byte{
		repositoryatlas.ArtifactFilename: atlasJSON,
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
