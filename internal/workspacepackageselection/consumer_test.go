package workspacepackageselection_test

import (
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacepackageselection"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestSelectionFeedsExternalNeutralConsumer(t *testing.T) {
	const (
		repositoryRoot = "/definitely-not-present/workspace-package-consumer"
		canonicalPath  = "example.com/repo/internal/core"
	)
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: repositoryRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Dirty:    []freshness.DirtyFile{},
		},
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{{
				ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
			}},
			Packages: []gofacts.PackageFact{{
				CanonicalPath:     canonicalPath,
				Name:              "core",
				ModuleID:          "root-id",
				ModulePath:        "example.com/repo",
				PackageDir:        "internal/core",
				ModuleRelativeDir: "internal/core",
			}},
		},
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}

	selection, err := workspacepackageselection.New(
		workspacepackageselection.Input{
			Graph: graph,
			Candidates: []workspacepackageselection.Candidate{{
				CanonicalPath:     canonicalPath,
				Name:              "core",
				ModuleID:          "root-id",
				ModulePath:        "example.com/repo",
				PackageDir:        "internal/core",
				ModuleRelativeDir: "internal/core",
			}},
		},
	)
	if err != nil {
		t.Fatalf("workspacepackageselection.New: %v", err)
	}
	packages := selection.Packages()
	if len(packages) != 1 ||
		packages[0].CanonicalPath != canonicalPath ||
		packages[0].PackageDir != "internal/core" {
		t.Fatalf("Packages = %#v", packages)
	}
}
