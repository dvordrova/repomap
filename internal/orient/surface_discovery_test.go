package orient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPersistsOptInSurfaceArtifactsBesideReportRun(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/orient-surface\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "main.go", `package main

import "net/http"

func health(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	_ = http.ListenAndServe(":8080", mux)
}
`)
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "main.go")

	debugDirectory := t.TempDir()
	_, err := Run(context.Background(), Options{
		RepoPath: repository, Offline: true, OutputJSON: true,
		DebugDir: debugDirectory, RunID: "surface-run", RequireArtifacts: true,
		DiscoverSurfaces: true,
		MaxReadmeBytes:   1024, MaxReadmeLLMBytes: 512, MaxTreeLines: 50,
		MaxInterestingFiles: 50, MaxGoPkgs: 50, MaxGoEdges: 50,
		MaxLLMEntrypoints: 10, MaxLLMModules: 10, MaxLLMFiles: 20,
		MaxLLMEdges: 20, MaxLLMSignals: 10, MaxLLMSignalsPerFile: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(debugDirectory, "surface-run")
	for _, name := range []string{
		"trigger_catalog.json", "surface_coverage.json",
		"semantic_summaries.json", "surface_summary.md",
	} {
		data, err := os.ReadFile(filepath.Join(runDirectory, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "/health") && name == "trigger_catalog.json" {
			t.Fatalf("catalog does not contain route: %s", data)
		}
	}
}

func TestRunSurfaceDiscoveryWithoutArtifactDirectoryIsSkipped(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "README.md", "fixture\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "README.md")

	_, err := Run(context.Background(), Options{
		RepoPath: repository, SnapshotOnly: true, DiscoverSurfaces: true,
	})
	if err != nil {
		t.Fatalf("surface discovery without an artifact directory: %v", err)
	}
}

func TestRunSurfaceDiscoverySkipsNonGoRepository(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "README.md", "fixture\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "README.md")

	debugDirectory := t.TempDir()
	_, err := Run(context.Background(), Options{
		RepoPath: repository, Offline: true, OutputJSON: true,
		DebugDir: debugDirectory, RunID: "non-go", RequireArtifacts: true,
		DiscoverSurfaces: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(debugDirectory, "non-go", "trigger_catalog.json")); !os.IsNotExist(err) {
		t.Fatalf("non-Go run created a surface catalog: %v", err)
	}
}

func writeSurfaceTestFile(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
