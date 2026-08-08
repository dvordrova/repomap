package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/mechanismstudy"
)

func TestStudyInvestigationLoaderFailsClosedOnMissingAndPartialCurrentFamily(t *testing.T) {
	for _, test := range []struct {
		name   string
		remove []string
		want   string
	}{
		{
			name:   "all four absent beside accepted Study themes",
			remove: append([]string(nil), mechanismstudy.ArtifactFilenames...),
			want:   "accepted study_themes requires the complete artifact family",
		},
		{
			name:   "partial family",
			remove: []string{mechanismstudy.StatusArtifactFilename},
			want:   "artifact family is incomplete",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := atlasStudyReportFixture(t)
			runDir := t.TempDir()
			writeThemeStudyAcceptedArtifacts(t, runDir, data)
			for _, name := range test.remove {
				if err := os.Remove(filepath.Join(runDir, name)); err != nil {
					t.Fatalf("remove %s: %v", name, err)
				}
			}
			data.studyInvestigationArtifactsChecked = false
			if _, _, err := readAtlasStudyReportProduct(runDir, data); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("read error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStudyInvestigationLoaderRejectsStatusVersionAndRevisionDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *ReportData)
		want   string
	}{
		{
			name: "status",
			mutate: func(t *testing.T, runDir string, _ *ReportData) {
				path := filepath.Join(runDir, mechanismstudy.StatusArtifactFilename)
				var status mechanismstudy.Status
				if err := json.Unmarshal(mustReadAtlasStudyFile(t, runDir, mechanismstudy.StatusArtifactFilename), &status); err != nil {
					t.Fatal(err)
				}
				status.State = mechanismstudy.StatusPartial
				encoded, err := json.Marshal(status)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, encoded, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "status",
		},
		{
			name: "artifact version",
			mutate: func(t *testing.T, runDir string, _ *ReportData) {
				path := filepath.Join(runDir, mechanismstudy.FactsArtifactFilename)
				var facts map[string]any
				if err := json.Unmarshal(mustReadAtlasStudyFile(t, runDir, mechanismstudy.FactsArtifactFilename), &facts); err != nil {
					t.Fatal(err)
				}
				facts["version"] = float64(mechanismstudy.ArtifactVersion + 1)
				encoded, err := json.Marshal(facts)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, encoded, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "facts",
		},
		{
			name: "repository revision",
			mutate: func(_ *testing.T, _ string, data *ReportData) {
				data.CapturedRevision = strings.Repeat("f", 40)
			},
			want: "study_themes revision",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := atlasStudyReportFixture(t)
			runDir := t.TempDir()
			writeThemeStudyAcceptedArtifacts(t, runDir, data)
			test.mutate(t, runDir, data)
			data.studyInvestigationArtifactsChecked = false
			if _, _, err := readAtlasStudyReportProduct(runDir, data); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("read error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestManifestVerifiesStudyInvestigationBytesAndSemanticStatus(t *testing.T) {
	manifest, reportJSON, runDir := d210ThemeManifestFixture(t, "accepted")
	var persisted ReportData
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.AtlasStudy == nil || persisted.AtlasStudy.Themes == nil ||
		len(persisted.AtlasStudy.Themes.Cards) == 0 ||
		persisted.AtlasStudy.Themes.Cards[0].Investigation == nil {
		t.Fatal("fixture report lacks the persisted Study investigation projection")
	}
	if err := manifest.VerifyStudyInvestigationArtifacts(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyStudyInvestigationArtifacts: %v", err)
	}
	if err := manifest.VerifyThemesArtifacts(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyThemesArtifacts with rehydrated investigation: %v", err)
	}

	statusPath := filepath.Join(runDir, mechanismstudy.StatusArtifactFilename)
	var status mechanismstudy.Status
	if err := json.Unmarshal(mustReadAtlasStudyFile(t, runDir, mechanismstudy.StatusArtifactFilename), &status); err != nil {
		t.Fatal(err)
	}
	status.State = mechanismstudy.StatusPartial
	tampered, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.StudyInvestigationStatusSHA256 = manifestSHA256(tampered)
	if err := manifest.VerifyStudyInvestigationArtifacts(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "status") {
		t.Fatalf("semantic status tamper error = %v", err)
	}
}

func TestManifestStudyInvestigationAbsenceAndUnboundFile(t *testing.T) {
	runDir := t.TempDir()
	manifest := RunManifest{Version: CurrentRunManifestVersion}
	reportJSON := []byte(`{}`)
	if err := manifest.VerifyStudyInvestigationArtifacts(runDir, reportJSON); err != nil {
		t.Fatalf("all-four absent without accepted Study: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, mechanismstudy.FactsArtifactFilename),
		[]byte(`{}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyStudyInvestigationArtifacts(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "unbound") {
		t.Fatalf("unbound artifact error = %v", err)
	}
}
