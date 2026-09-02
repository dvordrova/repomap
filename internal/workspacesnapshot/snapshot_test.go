package workspacesnapshot

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestNewBuildsImmutableSourceAuthority(t *testing.T) {
	repositoryRoot := filepath.Clean(t.TempDir())
	analysisRoot := filepath.Join(repositoryRoot, "service")
	input := Input{
		AnalysisRoot: analysisRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		CapturedInputs: []freshness.CapturedInput{{
			Version:       freshness.CapturedInputVersion,
			ID:            strings.Repeat("b", 64),
			Path:          "service/main.go",
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: strings.Repeat("c", 64),
			Stages:        []string{"report_evidence"},
		}},
		AllowedPaths: []string{"main.go"},
	}

	snapshot, err := New(input)
	if err != nil {
		t.Fatalf("new snapshot: %v", err)
	}
	input.AllowedPaths[0] = "mutated.go"
	input.CapturedInputs[0].Path = "service/mutated.go"
	if snapshot.RepositoryRoot() != repositoryRoot || snapshot.AnalysisRoot() != analysisRoot ||
		snapshot.repository.Head != strings.Repeat("a", 40) || len(snapshot.Catalog().Paths()) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if source, ok := snapshot.Catalog().Lookup("main.go"); !ok || source.RepositoryPath != "service/main.go" {
		t.Fatalf("missing immutable source authority: %#v, %v", source, ok)
	}
}

func TestNewRejectsAllowedPathWithoutCapturedInput(t *testing.T) {
	repositoryRoot := filepath.Clean(t.TempDir())
	_, err := New(Input{
		AnalysisRoot: repositoryRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		AllowedPaths: []string{"main.go"},
	})
	if err == nil {
		t.Fatal("allowed path without captured content was accepted")
	}
}

func TestNewRetainsAuthorityBeyondFormerLocalThreshold(t *testing.T) {
	repositoryRoot := filepath.Clean(t.TempDir())
	input := Input{
		AnalysisRoot: repositoryRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		AllowedPaths:   make([]string, advisoryMaximumAllowedPaths+1),
		CapturedInputs: make([]freshness.CapturedInput, advisoryMaximumAllowedPaths+1),
	}
	for position := range input.AllowedPaths {
		filePath := fmt.Sprintf("src/%05d.go", position)
		input.AllowedPaths[position] = filePath
		input.CapturedInputs[position] = freshness.CapturedInput{
			Version: freshness.CapturedInputVersion, ID: strings.Repeat("b", 64),
			Path: filePath, Kind: freshness.FileRegular, Mode: "100644",
			ContentSHA256: strings.Repeat("c", 64), Stages: []string{"report_evidence"},
		}
	}
	snapshot, err := New(input)
	if err != nil {
		t.Fatalf("complete source authority was rejected: %v", err)
	}
	if len(snapshot.Catalog().Paths()) != len(input.AllowedPaths) {
		t.Fatalf("retained paths = %d, want %d", len(snapshot.Catalog().Paths()), len(input.AllowedPaths))
	}
	warnings := ScaleWarnings(input)
	if len(warnings) == 0 || warnings[0].Kind != ScaleWarningAllowedPaths {
		t.Fatalf("scale warnings = %#v", warnings)
	}
}
