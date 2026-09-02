package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestPersistReadmeRoleAuthorityIsExactAndEmptyMeansAbsent(t *testing.T) {
	runDir := t.TempDir()
	rows := []readmeRoleLogRow{{
		FileRef: corpus.FileID("f1"),
		Path:    "README.md",
		Classifications: []readmetargetscout.Classification{{
			Class:      readmetargetscout.ClassDocumentation,
			Hypotheses: []string{"explains the repository"},
		}},
	}}

	if err := persistReadmeRoleAuthority(runDir, rows); err != nil {
		t.Fatalf("persistReadmeRoleAuthority: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, readmetargetscout.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReadmeRoleLog(raw); err != nil {
		t.Fatalf("persisted README authority: %v", err)
	}
	var persisted struct {
		Version int                `json:"version"`
		Files   []readmeRoleLogRow `json:"files"`
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Version != 1 || !reflect.DeepEqual(persisted.Files, rows) {
		t.Fatalf("persisted README authority differs from accepted rows")
	}

	if err := persistReadmeRoleAuthority(runDir, nil); err != nil {
		t.Fatalf("empty README authority: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(runDir, readmetargetscout.ArtifactFilename)); !os.IsNotExist(err) {
		t.Fatalf("empty README authority left an artifact: %v", err)
	}

}

func TestPersistReadmeRoleAuthorityFailureIsTerminal(t *testing.T) {
	runDir := t.TempDir()
	artifactPath := filepath.Join(runDir, readmetargetscout.ArtifactFilename)
	if err := os.Mkdir(artifactPath, 0o700); err != nil {
		t.Fatal(err)
	}
	rows := []readmeRoleLogRow{{
		FileRef: corpus.FileID("f1"),
		Path:    "README.md",
		Classifications: []readmetargetscout.Classification{{
			Class:      readmetargetscout.ClassDocumentation,
			Hypotheses: []string{"explains the repository"},
		}},
	}}
	if err := persistReadmeRoleAuthority(runDir, rows); err == nil {
		t.Fatal("persistReadmeRoleAuthority unexpectedly ignored persistence failure")
	}
}

func TestReadmeRoleAuthorityBeyondFormerArtifactLimitPersistsAndWarns(t *testing.T) {
	runDir := t.TempDir()
	rows := []readmeRoleLogRow{{
		FileRef: corpus.FileID("f1"), Path: "cmd/server/main.go",
		Classifications: []readmetargetscout.Classification{{
			Class:      readmetargetscout.ClassTargetEntry,
			Hypotheses: []string{strings.Repeat("x", readmetargetscout.AdvisoryArtifactBytes+1)},
		}},
	}}
	var console strings.Builder
	reportReadmeRoleArtifactScaleWarnings(newRunOutput(&console), rows)
	if !strings.Contains(console.String(), "Large repository-guidance classification retained") ||
		!strings.Contains(console.String(), "artifact_bytes") {
		t.Fatalf("warning output = %q", console.String())
	}
	if err := persistReadmeRoleAuthority(runDir, rows); err != nil {
		t.Fatalf("persist beyond former limit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, readmetargetscout.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= readmetargetscout.AdvisoryArtifactBytes {
		t.Fatalf("artifact = %d bytes", len(raw))
	}
}
