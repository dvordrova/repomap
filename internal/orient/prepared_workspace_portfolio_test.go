package orient

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestSelectedTargetPortfolioSharesOnePreparedGoWorkspace(t *testing.T) {
	repository := t.TempDir()
	for _, directory := range []string{"cmd/app", "cmd/helper", "internal/shared"} {
		if err := os.MkdirAll(filepath.Join(repository, filepath.FromSlash(directory)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/shared-workspace\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "cmd/app/main.go", `package main
import "example.com/shared-workspace/internal/shared"
func main() { shared.FromApp() }
`)
	writeSurfaceTestFile(t, repository, "cmd/helper/main.go", `package main
import "example.com/shared-workspace/internal/shared"
func main() { shared.FromHelper() }
`)
	writeSurfaceTestFile(t, repository, "internal/shared/shared.go", `package shared
func FromApp() { common() }
func FromHelper() { common() }
func common() {}
`)
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go", "cmd/helper/main.go", "internal/shared/shared.go")

	var packageLoads, ssaBuilds atomic.Int32
	progress := func(event ProgressEvent) {
		if event.Stage != ProgressProgramPhase || event.PhaseState != "started" {
			return
		}
		switch event.Phase {
		case "package_load":
			packageLoads.Add(1)
		case "ssa_build":
			ssaBuilds.Add(1)
		}
	}
	var container snapshot.TargetRunContainer
	var workspace *surfacediscovery.PreparedWorkspace
	var defaultIndex surfacediscovery.DirectCallIndex
	defaultOptions := prepareOrientRunOptions(t, repository, Options{
		RepoPath: repository, DebugDir: t.TempDir(), RunID: "portfolio-default",
		RequireArtifacts: true, AnalyzeGoProgram: true,
		DirectCallDepth: 3, DirectCallEdgeLimit: 100,
		Progress: progress,
		AnalysisTargetSelector: func(
			_ context.Context,
			_ string,
			catalog analysistarget.TargetCatalog,
			_ gofacts.Facts,
		) (snapshot.TargetRunSelection, error) {
			refs := make(map[string]string, len(catalog.Entries))
			for _, entry := range catalog.Entries {
				refs[entry.DisplayPath] = entry.Candidate.Target.Ref
			}
			return snapshot.TargetRunSelection{
				DefaultTargetRef: refs["cmd/app"],
				TargetRefs:       []string{refs["cmd/helper"], refs["cmd/app"]},
			}, nil
		},
		TargetRunContainerSink:  func(value snapshot.TargetRunContainer) { container = value },
		PreparedGoWorkspaceSink: func(value *surfacediscovery.PreparedWorkspace) { workspace = value },
		DirectCallIndexSink:     func(value surfacediscovery.DirectCallIndex) { defaultIndex = value },
	})
	if err := Run(t.Context(), defaultOptions); err != nil {
		t.Fatal(err)
	}
	if workspace == nil || len(container.Targets) != 2 {
		t.Fatalf("prepared workspace/container = %p / %#v", workspace, container.Targets)
	}

	var siblingProjection snapshot.TargetRunProjection
	for _, projection := range container.Targets {
		if projection.Target.Ref != container.DefaultTargetRef {
			siblingProjection = projection
			break
		}
	}
	siblingSnapshot, err := container.ScopedSnapshot(siblingProjection.Target.Ref)
	if err != nil {
		t.Fatal(err)
	}
	var siblingIndex surfacediscovery.DirectCallIndex
	err = Run(t.Context(), Options{
		RepoPath: repository, GoTarget: runtime.GOOS + "/" + runtime.GOARCH,
		DebugDir: t.TempDir(), RunID: "portfolio-sibling", RequireArtifacts: true,
		AnalyzeGoProgram: true, DirectCallDepth: 3, DirectCallEdgeLimit: 100,
		PrecomputedSnapshot: &siblingSnapshot, PreparedGoWorkspace: workspace,
		Progress:            progress,
		DirectCallIndexSink: func(value surfacediscovery.DirectCallIndex) { siblingIndex = value },
	})
	if err != nil {
		t.Fatal(err)
	}
	if packageLoads.Load() != 1 || ssaBuilds.Load() != 1 {
		t.Fatalf(
			"two-target portfolio package-load/SSA starts = %d/%d, want exactly 1/1",
			packageLoads.Load(), ssaBuilds.Load(),
		)
	}
	if defaultIndex.Scope.TargetRef != container.DefaultTargetRef ||
		siblingIndex.Scope.TargetRef != siblingProjection.Target.Ref ||
		defaultIndex.Scope.TargetRef == siblingIndex.Scope.TargetRef {
		t.Fatalf("target scopes = %#v / %#v", defaultIndex.Scope, siblingIndex.Scope)
	}
	if indexContainsPackage(defaultIndex, "example.com/shared-workspace/cmd/helper") ||
		indexContainsPackage(siblingIndex, "example.com/shared-workspace/cmd/app") {
		t.Fatalf("sibling package leaked across target projections")
	}
}

func indexContainsPackage(index surfacediscovery.DirectCallIndex, packagePath string) bool {
	for _, node := range index.Nodes {
		if node.Package == packagePath {
			return true
		}
	}
	return false
}
