package workspacegraph_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesearch"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestGraphFeedsNeutralConsumerWithoutPresentationPackages(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspacegraph-consumer"
	repository := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: filepath.Clean(repositoryRoot),
		Head:     strings.Repeat("a", 40),
		Dirty:    []freshness.DirtyFile{},
	}
	allowed := []string{"cmd/app/main.go", "internal/core/core.go"}
	captured := make([]freshness.CapturedInput, 0, len(allowed))
	for _, sourcePath := range allowed {
		id := sha256.Sum256([]byte("id:" + sourcePath))
		content := sha256.Sum256([]byte("content:" + sourcePath))
		captured = append(captured, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id),
			Path:          sourcePath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", content),
			Stages:        []string{"workspace_graph_consumer"},
		})
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   repository.Identity,
		Repository:     repository,
		CapturedInputs: captured,
		AllowedPaths:   allowed,
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}

	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "nested-id", ModulePath: "example.com/repo/tools", ModuleDir: "tools"},
			{ID: "root-id", ModulePath: "example.com/repo", ModuleDir: "."},
		},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/repo/tools/tool", Name: "tool",
				ModuleID: "nested-id", ModulePath: "example.com/repo/tools",
				PackageDir: "tools/tool", ModuleRelativeDir: "tool",
				Files: []string{"tools/tool/tool.go"},
			},
			{
				CanonicalPath: "example.com/repo/internal/core", Name: "core",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
				Files: []string{"internal/core/core.go"},
			},
			{
				CanonicalPath: "example.com/repo/cmd/app", Name: "main",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
				Files: []string{"cmd/app/main.go", "cmd/app/generated.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/repo/tools/tool", To: "example.com/repo/internal/core"},
			{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
		},
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	if len(graph.Modules()) != 2 || len(graph.Packages()) != 3 || len(graph.Edges()) != 2 {
		t.Fatalf(
			"graph shape = %d modules, %d packages, %d edges",
			len(graph.Modules()),
			len(graph.Packages()),
			len(graph.Edges()),
		)
	}
	tool, ok := graph.Package("example.com/repo/tools/tool")
	if !ok || tool.ModuleID != "nested-id" || tool.ModulePath != "example.com/repo/tools" {
		t.Fatalf("nested module ownership = %#v, %v", tool, ok)
	}
	app, ok := graph.Package("example.com/repo/cmd/app")
	if !ok {
		t.Fatal("app package missing")
	}
	if want := []workspacegraph.File{
		{Path: "cmd/app/generated.go", Openable: false},
		{Path: "cmd/app/main.go", Openable: true},
	}; !reflect.DeepEqual(app.Files, want) {
		t.Fatalf("app files = %#v, want %#v", app.Files, want)
	}

	index, err := workspacesearch.New(workspacesearch.Input{Catalog: snapshot.Catalog()})
	if err != nil {
		t.Fatalf("workspacesearch.New: %v", err)
	}
	matches, err := index.Search(workspacesearch.Query{Text: app.Files[1].Path})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 || matches[0].Entry.Path != app.Files[1].Path ||
		matches[0].Match != workspacesearch.MatchExactPath {
		t.Fatalf("openable graph file search = %#v", matches)
	}

	beforeModules := graph.Modules()
	beforePackages := graph.Packages()
	beforeEdges := graph.Edges()
	unrelated := repository
	unrelated.Dirty = []freshness.DirtyFile{{
		Status: "modified", Path: "notes.txt", Kind: freshness.FileRegular,
		Mode: "100644", ContentSHA256: strings.Repeat("b", 64),
	}}
	if state := snapshot.Assess(unrelated).State; state != freshness.FreshnessUnrelatedChanges {
		t.Fatalf("unrelated repository state = %q", state)
	}
	if !reflect.DeepEqual(beforeModules, graph.Modules()) ||
		!reflect.DeepEqual(beforePackages, graph.Packages()) ||
		!reflect.DeepEqual(beforeEdges, graph.Edges()) {
		t.Fatal("unrelated repository change altered immutable graph facts")
	}

	exposed := fmt.Sprintf("%#v %#v %#v", graph.Modules(), graph.Packages(), graph.Edges())
	if strings.Contains(exposed, repository.Identity) ||
		strings.Contains(exposed, "/definitely-not-present") {
		t.Fatalf("graph exposed absolute repository root: %s", exposed)
	}
}

func TestGraphUsesAnalysisRelativePathsForSubdirectoryRoot(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspacegraph-subdirectory"
	const analysisRoot = repositoryRoot + "/service"
	sourcePath := "cmd/app/main.go"
	id := sha256.Sum256([]byte("id"))
	content := sha256.Sum256([]byte("content"))
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: analysisRoot,
		Repository: freshness.RepositoryState{
			Version: freshness.RepositoryStateVersion, Identity: repositoryRoot,
			Head: strings.Repeat("c", 40), Dirty: []freshness.DirtyFile{},
		},
		CapturedInputs: []freshness.CapturedInput{{
			Version: freshness.CapturedInputVersion, ID: fmt.Sprintf("%x", id),
			Path: "service/" + sourcePath, Kind: freshness.FileRegular,
			Mode: "100644", ContentSHA256: fmt.Sprintf("%x", content),
			Stages: []string{"workspace_graph_consumer"},
		}},
		AllowedPaths: []string{sourcePath},
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{{
				ID: "service-id", ModulePath: "example.com/service", ModuleDir: ".",
			}},
			Packages: []gofacts.PackageFact{
				{
					CanonicalPath: "example.com/service/cmd/app", Name: "main",
					ModuleID: "service-id", ModulePath: "example.com/service",
					PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
					Files: []string{sourcePath},
				},
				{
					CanonicalPath: "example.com/outside", Name: "outside",
					ModuleID: "service-id", ModulePath: "example.com/service",
					PackageDir: "../outside", ModuleRelativeDir: "../outside",
					Files: []string{"../outside/out.go"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	packages := graph.Packages()
	if len(packages) != 1 || packages[0].Dir != "cmd/app" ||
		len(packages[0].Files) != 1 || packages[0].Files[0].Path != sourcePath ||
		!packages[0].Files[0].Openable {
		t.Fatalf("subdirectory packages = %#v", packages)
	}
	exposed := fmt.Sprintf("%#v", packages)
	if strings.Contains(exposed, repositoryRoot) ||
		strings.Contains(exposed, `Path:"service/cmd/app/main.go"`) ||
		strings.Contains(exposed, `Dir:"service/cmd/app"`) {
		t.Fatalf("subdirectory graph leaked or re-prefixed path: %s", exposed)
	}
}
