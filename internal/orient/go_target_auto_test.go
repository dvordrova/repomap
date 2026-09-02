package orient

import (
	"context"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestGoTargetCatalogIncludesMainDeclaredInCgoFile(t *testing.T) {
	if !build.Default.CgoEnabled {
		t.Skip("cgo is disabled")
	}
	repository := t.TempDir()
	files := map[string]string{
		"go.mod":   "module example.com/cgoapp\n\ngo 1.24\n",
		"hypr.go":  "package main\n\nfunc hypr() {}\n",
		"tools.go": "package main\n\nfunc tools() {}\n",
		"main.go": `package main

/* typedef int repomap_int; */
import "C"

func main() { _ = C.repomap_int(0) }
`,
	}
	fileList := make([]string, 0, len(files))
	for name, source := range files {
		writeSurfaceTestFile(t, repository, name, source)
		fileList = append(fileList, name)
	}
	facts, err := gofacts.LoadWithOptions(
		context.Background(), repository, fileList,
		gofacts.LoadOptions{GoTarget: runtime.GOOS + "/" + runtime.GOARCH},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.EntrypointPackages) != 1 || len(facts.EntrypointPackages[0].Anchors) != 1 ||
		facts.EntrypointPackages[0].Anchors[0].Path != "main.go" {
		t.Fatalf("cgo entrypoint facts = %#v", facts.EntrypointPackages)
	}
	catalog, err := analysistarget.BuildCatalog(*facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 ||
		catalog.Entries[0].Candidate.Target.Kind != analysistarget.KindExecutablePackage ||
		len(catalog.Entries[0].Candidate.Target.Roots) != 1 ||
		catalog.Entries[0].Candidate.Target.Roots[0].Path != "main.go" {
		t.Fatalf("cgo executable catalog = %#v", catalog.Entries)
	}
}

func TestAutoGoTargetSelectionIsPreservedInDeferredSnapshot(t *testing.T) {
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

	var ready ProgressEvent
	err := Run(context.Background(), prepareOrientRunOptions(t, repository, Options{
		RepoPath: repository, GoTarget: "darwin/amd64", AutoGoTarget: true,
		Progress: func(event ProgressEvent) {
			if event.Stage == ProgressSnapshotReady {
				ready = event
			}
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if ready.GoTarget != "linux/amd64" ||
		ready.GoTargetProvenance != "auto: linux/amd64 (host darwin)" ||
		ready.SuggestedGoTarget != "linux/amd64" || ready.GoTargetEvidenceCount != 3 {
		t.Fatalf("snapshot progress = %#v", ready)
	}
}
