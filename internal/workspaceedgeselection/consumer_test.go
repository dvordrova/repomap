package workspaceedgeselection_test

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspaceedgeselection"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestSelectionFeedsExternalNeutralConsumer(t *testing.T) {
	const (
		repositoryRoot = "/definitely-not-present/workspace-edge-consumer"
		fromPackage    = "example.com/repo/cmd/app"
		toPackage      = "example.com/repo/internal/core"
	)
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: repositoryRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
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
			Packages: []gofacts.PackageFact{
				{
					CanonicalPath: fromPackage, Name: "main",
					ModuleID: "root-id", ModulePath: "example.com/repo",
					PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
				},
				{
					CanonicalPath: toPackage, Name: "core",
					ModuleID: "root-id", ModulePath: "example.com/repo",
					PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
				},
			},
			InternalEdges: []gofacts.Edge{{From: fromPackage, To: toPackage}},
		},
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}

	selection, err := workspaceedgeselection.New(workspaceedgeselection.Input{
		Graph: graph,
		Candidates: []workspaceedgeselection.Candidate{{
			From: fromPackage,
			To:   toPackage,
		}},
	})
	if err != nil {
		t.Fatalf("workspaceedgeselection.New: %v", err)
	}
	edges := selection.Edges()
	if len(edges) != 1 || edges[0].From != fromPackage || edges[0].To != toPackage {
		t.Fatalf("Edges = %#v", edges)
	}
}
