package orient

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestAutoGoTargetSelectsFinalCatalogBeforeProviderSeam(t *testing.T) {
	repository := t.TempDir()
	for _, directory := range []string{"cmd/dockerd", "daemon"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/moby\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "cmd/dockerd/main.go", "package main\nimport \"example.com/moby/daemon\"\nfunc main() { daemon.Run() }\n")
	writeSurfaceTestFile(t, repository, "daemon/config_linux.go", "package daemon\nfunc Run() {}\n")
	writeSurfaceTestFile(t, repository, "daemon/network_linux.go", "package daemon\nconst network = true\n")
	writeSurfaceTestFile(t, repository, "daemon/storage_linux.go", "package daemon\nconst storage = true\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "cmd/dockerd/main.go",
		"daemon/config_linux.go", "daemon/network_linux.go", "daemon/storage_linux.go")

	selectionDelivered := false
	selectorCalls := 0
	containerDelivered := false
	var selected snapshot.GoTargetSelection
	var ready ProgressEvent
	_, err := Run(context.Background(), Options{
		RepoPath: repository, GoTarget: "darwin/amd64", AutoGoTarget: true,
		SnapshotOnly: true, Offline: true,
		AnalysisTargetSelectorOwnsResolution: true,
		GoTargetSelectionSink: func(got snapshot.GoTargetSelection) {
			selectionDelivered = true
			selected = got
		},
		AnalysisTargetSelector: func(
			_ context.Context,
			_ string,
			catalog analysistarget.TargetCatalog,
		) (snapshot.TargetRunSelection, error) {
			selectorCalls++
			if !selectionDelivered {
				t.Fatal("provider seam ran before final Go target selection")
			}
			for _, entry := range catalog.Entries {
				if entry.Candidate.Target.PackageDir == "cmd/dockerd" {
					return snapshot.TargetRunSelection{
						DefaultTargetRef: entry.Candidate.Target.Ref,
						TargetRefs:       []string{entry.Candidate.Target.Ref},
					}, nil
				}
			}
			t.Fatalf("final provider catalog omitted cmd/dockerd: %#v", catalog.Entries)
			return snapshot.TargetRunSelection{}, nil
		},
		TargetRunContainerSink: func(container snapshot.TargetRunContainer) {
			containerDelivered = true
			for _, projection := range container.Targets {
				scoped, scopeErr := container.ScopedSnapshot(projection.Target.Ref)
				if scopeErr != nil {
					t.Fatal(scopeErr)
				}
				if scoped.GoTargetSelection == nil ||
					scoped.GoTargetSelection.Target != "linux/amd64" ||
					scoped.GoTargetSelection.Baseline != "darwin/amd64" {
					t.Fatalf("target projection lost automatic scenario: %#v", scoped.GoTargetSelection)
				}
			}
		},
		Progress: func(event ProgressEvent) {
			if event.Stage == ProgressSnapshotReady {
				ready = event
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selectorCalls != 1 || !containerDelivered || selected.Target != "linux/amd64" ||
		selected.Baseline != "darwin/amd64" {
		t.Fatalf("selection/provider calls = %#v / %d", selected, selectorCalls)
	}
	if ready.GoTarget != "linux/amd64" ||
		ready.GoTargetProvenance != "auto: linux/amd64 (host darwin)" ||
		ready.SuggestedGoTarget != "linux/amd64" || ready.GoTargetEvidenceCount != 3 {
		t.Fatalf("snapshot progress = %#v", ready)
	}
}
