package orient

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestDeliverDirectCallIndexIsLiveRunOnlyAndExact(t *testing.T) {
	want := surfacediscovery.DirectCallIndex{
		Version: surfacediscovery.DirectCallIndexVersion,
		State:   surfacediscovery.DirectCallIndexReady,
		SHA256:  "exact-index-digest",
	}
	called := 0
	var got surfacediscovery.DirectCallIndex
	opts := Options{DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
		called++
		got = index
	}}

	deliverDirectCallIndex(opts, nil)
	if called != 0 {
		t.Fatalf("nil index invoked sink %d time(s)", called)
	}
	deliverDirectCallIndex(opts, &want)
	if called != 1 || got.Version != want.Version || got.State != want.State || got.SHA256 != want.SHA256 {
		t.Fatalf("direct-call handoff = calls:%d index:%+v, want exact %+v", called, got, want)
	}

	// A missing consumer is an intentional no-op: ordinary local artifact
	// production remains unchanged until a live Study investigation asks for
	// the private substrate.
	deliverDirectCallIndex(Options{}, &want)
}

func TestDeliverDirectCallIndexHandsOffIndependentSnapshot(t *testing.T) {
	want := surfacediscovery.DirectCallIndex{
		Version: surfacediscovery.DirectCallIndexVersion,
		State:   surfacediscovery.DirectCallIndexReady,
		Scenario: surfacediscovery.Scenario{
			ID: "scenario", GOOS: "test-os", GOARCH: "test-arch", Tags: []string{"tag"},
		},
		Modules: []surfacediscovery.DirectCallModule{{ID: "module", Path: "example.com/module"}},
		Nodes: []surfacediscovery.DirectCallNode{{
			ID: "node", Symbol: surfacediscovery.Symbol{EquivalentIDs: []string{"node-alias"}},
		}},
		Edges:     []surfacediscovery.DirectCallEdge{{ID: "edge", CallerID: "node"}},
		Frontiers: []surfacediscovery.DirectCallNodeFrontier{{CallerID: "node", NonStaticCallsExcluded: 1}},
	}
	deliverDirectCallIndex(Options{DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
		index.Scenario.Tags[0] = "changed"
		index.Modules[0].Path = "changed/module"
		index.Nodes[0].Symbol.EquivalentIDs[0] = "changed-alias"
		index.Edges[0].CallerID = "changed-node"
		index.Frontiers[0].NonStaticCallsExcluded = 99
	}}, &want)

	if want.Scenario.Tags[0] != "tag" || want.Modules[0].Path != "example.com/module" ||
		want.Nodes[0].Symbol.EquivalentIDs[0] != "node-alias" ||
		want.Edges[0].CallerID != "node" || want.Frontiers[0].NonStaticCallsExcluded != 1 {
		t.Fatalf("sink mutation changed producer-owned index: %#v", want)
	}
}

func TestRunDirectCallIndexHandoffUsesOneExistingSSABuild(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/direct-handoff\n\ngo 1.22\n")
	writeSurfaceTestFile(t, repository, "main.go", `package main

func main() { boot() }
func boot() { work() }
func work() {}
`)
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "main.go")

	var packageLoads atomic.Int32
	var ssaBuilds atomic.Int32
	var sinkCalls atomic.Int32
	var received surfacediscovery.DirectCallIndex
	debugDirectory := t.TempDir()
	err := Run(context.Background(), prepareOrientRunOptions(t, repository, Options{
		RepoPath: repository,
		DebugDir: debugDirectory, RunID: "direct-handoff", RequireArtifacts: true,
		AnalyzeGoProgram: true, DirectCallDepth: 2, DirectCallEdgeLimit: 17,
		Progress: func(event ProgressEvent) {
			if event.Stage != ProgressProgramPhase || event.PhaseState != "started" {
				return
			}
			switch event.Phase {
			case "package_load":
				packageLoads.Add(1)
			case "ssa_build":
				ssaBuilds.Add(1)
			}
		},
		DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
			sinkCalls.Add(1)
			received = index
		},
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if packageLoads.Load() != 1 || ssaBuilds.Load() != 1 {
		t.Fatalf("surface load/SSA phases = %d/%d, want exactly 1/1", packageLoads.Load(), ssaBuilds.Load())
	}
	if sinkCalls.Load() != 1 {
		t.Fatalf("DirectCallIndex sink calls = %d, want 1", sinkCalls.Load())
	}
	if err := received.Validate(); err != nil {
		t.Fatalf("received DirectCallIndex: %v", err)
	}
	if received.State != surfacediscovery.DirectCallIndexReady || len(received.Edges) < 2 {
		t.Fatalf("received DirectCallIndex = %#v, want connected ready index", received)
	}
	if received.Scope.TargetKind != surfacediscovery.AnalysisTargetExecutablePackage ||
		received.Scope.TargetPackage != "example.com/direct-handoff" ||
		received.Scope.MaxDepth != 2 || received.Scope.EdgeLimit != 17 {
		t.Fatalf("received DirectCallIndex scope = %#v, want exact target and requested bounds", received.Scope)
	}

	runDirectory := filepath.Join(debugDirectory, "direct-handoff")
	if err := filepath.WalkDir(runDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, []byte(received.SHA256)) || bytes.Contains(data, []byte("direct_call_index")) {
			t.Fatalf("live DirectCallIndex leaked into %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("inspect run artifacts: %v", err)
	}
}

func TestRunDirectCallIndexSinkSkipsDisabledAndNonGoRuns(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		repository := t.TempDir()
		writeSurfaceTestFile(t, repository, "go.mod", "module example.com/direct-disabled\n\ngo 1.22\n")
		writeSurfaceTestFile(t, repository, "main.go", "package main\nfunc main() {}\n")
		runOrientGit(t, repository, "init", "--quiet")
		runOrientGit(t, repository, "add", "--", "go.mod", "main.go")
		assertNoDirectCallIndexHandoff(t, repository, false)
	})

	t.Run("non-Go", func(t *testing.T) {
		repository := t.TempDir()
		writeSurfaceTestFile(t, repository, "README.md", "non-Go fixture\n")
		runOrientGit(t, repository, "init", "--quiet")
		runOrientGit(t, repository, "add", "--", "README.md")
		assertNoDirectCallIndexHandoff(t, repository, true)
	})
}

func assertNoDirectCallIndexHandoff(t *testing.T, repository string, discoverSurfaces bool) {
	t.Helper()
	var sinkCalls atomic.Int32
	err := Run(context.Background(), prepareOrientRunOptions(t, repository, Options{
		RepoPath: repository,
		DebugDir: t.TempDir(), RunID: "no-direct-handoff", RequireArtifacts: true,
		AnalyzeGoProgram: discoverSurfaces,
		DirectCallIndexSink: func(surfacediscovery.DirectCallIndex) {
			sinkCalls.Add(1)
		},
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sinkCalls.Load() != 0 {
		t.Fatalf("DirectCallIndex sink calls = %d, want 0", sinkCalls.Load())
	}
}
