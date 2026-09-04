package report

import (
	"github.com/dvordrova/repomap/internal/programindex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newProgramIndexManifestIndex(t *testing.T, sourceRef string) programindex.Index {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: sourceRef, Selector: sourceRef,
			Sources: []programindex.TargetSource{{FileRef: sourceRef, Path: sourceRef}}, AnchorFileRef: sourceRef,
		},
		Coverage: programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func manifestProgramIndexSHA256(t *testing.T, runDir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, programindex.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return index.SHA256
}

func writeProgramIndexManifestFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedTargetNavigationPageUsesCurrentRunProjection(t *testing.T) {
	t.Parallel()
	index := newProgramIndexManifestIndex(t, "app/main.py")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), "20260828-120000-python-a1b2c3")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		ArtifactsDir: runDir, ProgramPortfolio: portfolio,
		defaultProgramIndexArtifactFilename: programindex.ArtifactFilename,
	}
	page, err := PreparedTargetNavigationPage(runDir, data)
	if err != nil {
		t.Fatal(err)
	}
	if page.RunID != filepath.Base(runDir) || page.ProgramTarget.ID != index.Target.ID ||
		page.ArtifactFilename != programindex.ArtifactFilename {
		t.Fatalf("prepared page = %#v", page)
	}
	if entries, err := os.ReadDir(runDir); err != nil || len(entries) != 0 {
		t.Fatalf("prepared page unexpectedly read or wrote run artifacts: %v / %#v", err, entries)
	}
	if _, err := PreparedTargetNavigationPage(t.TempDir(), data); err == nil {
		t.Fatal("prepared page accepted a foreign run directory")
	}
}
