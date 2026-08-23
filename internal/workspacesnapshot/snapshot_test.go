package workspacesnapshot

import (
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

func TestNewRejectsOversizedAuthority(t *testing.T) {
	repositoryRoot := filepath.Clean(t.TempDir())
	_, err := New(Input{
		AnalysisRoot: repositoryRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		AllowedPaths: make([]string, maxAllowedPaths+1),
	})
	if err == nil {
		t.Fatal("oversized source authority was accepted")
	}
}
