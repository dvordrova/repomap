package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestVerifyProgramPortfolioProjectionRebuildsCompletePortfolio(t *testing.T) {
	runDir, manifest, indexRaw, _ := programIndexManifestFixture(t, "src/app.py")
	index, err := programindex.Decode(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	report := ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "fixture",
		CapturedRevision: strings.Repeat("a", 40), ProgramPortfolio: portfolio,
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyProgramPortfolioProjection(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyProgramPortfolioProjection: %v", err)
	}

	tampered := *portfolio
	tampered.Entries = append([]ProgramPortfolioEntry(nil), portfolio.Entries...)
	tampered.Entries[0].View.Projection.Objects.Eligible++
	report.ProgramPortfolio = &tampered
	reportJSON, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyProgramPortfolioProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered ProgramView error = %v", err)
	}

	tampered = *portfolio
	tampered.Entries = append([]ProgramPortfolioEntry(nil), portfolio.Entries...)
	tampered.Entries[0].SemanticState = ProgramSemanticAvailable
	report.ProgramPortfolio = &tampered
	reportJSON, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyProgramPortfolioProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered semantic capability error = %v", err)
	}
}

func TestVerifyProgramIndexArtifactsBindsSetTargetAndIndex(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		runDir, manifest, _, _ := programIndexManifestFixture(t, "src/app.py")
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err != nil {
			t.Fatalf("VerifyProgramIndexArtifacts: %v", err)
		}
	})

	t.Run("set bytes are material", func(t *testing.T) {
		runDir, manifest, _, setRaw := programIndexManifestFixture(t, "src/app.py")
		writeProgramIndexManifestFile(t, runDir, programindex.ArtifactSetFilename, append(setRaw, '\n'))
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err == nil || !strings.Contains(err.Error(), "set sha256 mismatch") {
			t.Fatalf("VerifyProgramIndexArtifacts error = %v, want set digest rejection", err)
		}
	})

	t.Run("index semantic seal is material", func(t *testing.T) {
		runDir, manifest, indexRaw, _ := programIndexManifestFixture(t, "src/app.py")
		changed := strings.Replace(string(indexRaw), `"language":"python"`, `"language":"pyth0n"`, 1)
		if changed == string(indexRaw) {
			t.Fatal("fixture index did not contain expected language")
		}
		writeProgramIndexManifestFile(t, runDir, programindex.ArtifactFilename, []byte(changed))
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err == nil {
			t.Fatal("VerifyProgramIndexArtifacts accepted an index with a stale semantic seal")
		}
	})

	t.Run("entry target id is exact", func(t *testing.T) {
		runDir, manifest, _, _ := programIndexManifestFixture(t, "src/app.py")
		other := newProgramIndexManifestIndex(t, "src/worker.py")
		set, err := programindex.NewArtifactSet(other.Target.ID, []programindex.ArtifactSetEntry{{
			TargetID: other.Target.ID, Filename: programindex.ArtifactFilename, IndexSHA256: manifestProgramIndexSHA256(t, runDir),
		}})
		if err != nil {
			t.Fatal(err)
		}
		setRaw, err := programindex.EncodeArtifactSet(set)
		if err != nil {
			t.Fatal(err)
		}
		writeProgramIndexManifestFile(t, runDir, programindex.ArtifactSetFilename, setRaw)
		manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err = reportProgramTargetMaterial(&other.Target)
		if err != nil {
			t.Fatal(err)
		}
		manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(setRaw)
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err == nil || !strings.Contains(err.Error(), "target id mismatch") {
			t.Fatalf("VerifyProgramIndexArtifacts error = %v, want target id rejection", err)
		}
	})

	t.Run("default target content is exact", func(t *testing.T) {
		runDir, manifest, _, _ := programIndexManifestFixture(t, "src/app.py")
		manifest.MaterialInputs.ProgramTargetSHA256 = strings.Repeat("c", 64)
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err == nil || !strings.Contains(err.Error(), "default program target identity mismatch") {
			t.Fatalf("VerifyProgramIndexArtifacts error = %v, want target content rejection", err)
		}
	})

	t.Run("referenced index must be a regular file", func(t *testing.T) {
		runDir, manifest, _, _ := programIndexManifestFixture(t, "src/app.py")
		outside := filepath.Join(t.TempDir(), "outside.json")
		writeProgramIndexManifestFile(t, filepath.Dir(outside), filepath.Base(outside), []byte("{}"))
		if err := os.Remove(filepath.Join(runDir, programindex.ArtifactFilename)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(runDir, programindex.ArtifactFilename)); err != nil {
			t.Fatal(err)
		}
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("VerifyProgramIndexArtifacts error = %v, want symlink rejection", err)
		}
	})

	t.Run("program index authority is mandatory", func(t *testing.T) {
		runDir, manifest, _, _ := programIndexManifestFixture(t, "src/app.py")
		manifest.MaterialInputs.ProgramIndexSetSHA256 = ""
		if err := manifest.VerifyProgramIndexArtifacts(runDir); err == nil || !strings.Contains(err.Error(), "program index set sha256 is invalid") {
			t.Fatalf("VerifyProgramIndexArtifacts error = %v, want mandatory set rejection", err)
		}
	})
}

func programIndexManifestFixture(t *testing.T, sourceRef string) (string, RunManifest, []byte, []byte) {
	t.Helper()
	runDir := t.TempDir()
	index := newProgramIndexManifestIndex(t, sourceRef)
	indexRaw, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	set, err := programindex.NewArtifactSet(index.Target.ID, []programindex.ArtifactSetEntry{{
		TargetID: index.Target.ID, Filename: programindex.ArtifactFilename, IndexSHA256: index.SHA256,
	}})
	if err != nil {
		t.Fatal(err)
	}
	setRaw, err := programindex.EncodeArtifactSet(set)
	if err != nil {
		t.Fatal(err)
	}
	writeProgramIndexManifestFile(t, runDir, programindex.ArtifactFilename, indexRaw)
	writeProgramIndexManifestFile(t, runDir, programindex.ArtifactSetFilename, setRaw)

	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err = reportProgramTargetMaterial(&index.Target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(setRaw)
	return runDir, manifest, indexRaw, setRaw
}

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
