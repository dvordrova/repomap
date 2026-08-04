package workspaceentrypoint_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspaceentrypoint"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestIndexFeedsExternalNeutralConsumer(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspace-entrypoint-consumer"
	sourceID := sha256.Sum256([]byte("source-id"))
	content := sha256.Sum256([]byte("source-content"))
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: filepath.Clean(repositoryRoot),
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: filepath.Clean(repositoryRoot),
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		CapturedInputs: []freshness.CapturedInput{{
			Version: freshness.CapturedInputVersion,
			ID:      fmt.Sprintf("%x", sourceID),
			Path:    "cmd/app/main.go",
			Kind:    freshness.FileRegular,
			Mode:    "100644",
			ContentSHA256: fmt.Sprintf(
				"%x",
				content,
			),
			Stages: []string{"workspace_entrypoint_consumer"},
		}},
		AllowedPaths: []string{"cmd/app/main.go"},
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	graphFacts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/repo/cmd/app", Name: "main",
			ModuleID: "root-id", ModulePath: "example.com/repo",
			PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
			Files: []string{"cmd/app/main.go"},
		}},
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  graphFacts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	exactFacts := gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{{
		ModulePath: "example.com/repo", ImportPath: "example.com/repo/cmd/app",
		PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app", ModuleDir: ".",
		Kind: "ignored-editorial-role",
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
			Path: "cmd/app/main.go", Line: 12,
		}},
	}}}
	index, err := workspaceentrypoint.New(workspaceentrypoint.Input{
		GoFacts: exactFacts,
		Graph:   graph,
	})
	if err != nil {
		t.Fatalf("workspaceentrypoint.New: %v", err)
	}
	entry, ok := index.Lookup("example.com/repo/cmd/app", "cmd/app/main.go", 12)
	if !ok || entry.Symbol != "main" || !entry.Openable {
		t.Fatalf("Lookup = %#v, %t", entry, ok)
	}
}
