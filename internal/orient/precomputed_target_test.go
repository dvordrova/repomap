package orient

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestRunPrecomputedTargetKeepsExactScopedTarget(t *testing.T) {
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "cmd", "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/precomputed\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "cmd/app/main.go", "package main\nfunc main() {}\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go")

	deferred, err := snapshot.Build(snapshot.Options{
		RepoPath: repository, DeferAnalysisTargetResolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.TargetCatalog == nil || len(deferred.TargetCatalog.Entries) != 1 {
		t.Fatalf("target catalog = %#v", deferred.TargetCatalog)
	}
	targetRef := deferred.TargetCatalog.Entries[0].Candidate.Target.Ref
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: targetRef, TargetRefs: []string{targetRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := container.ScopedSnapshot(targetRef)
	if err != nil {
		t.Fatal(err)
	}

	debugDir := t.TempDir()
	var deliveredRef string
	_, err = Run(context.Background(), Options{
		RepoPath:            repository,
		AtlasFirst:          true,
		DebugDir:            debugDir,
		RunID:               "precomputed-target",
		RequireArtifacts:    true,
		DumpRedacted:        true,
		PrecomputedSnapshot: &scoped,
		AnalysisTargetSink: func(target analysistarget.Target) {
			deliveredRef = target.Ref
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if deliveredRef != targetRef {
		t.Fatalf("delivered target = %q, want %q", deliveredRef, targetRef)
	}
	if _, err := os.Stat(filepath.Join(debugDir, "precomputed-target", "snapshot.json")); err != nil {
		t.Fatal(err)
	}
}
