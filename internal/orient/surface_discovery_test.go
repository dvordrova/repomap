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

func TestRunSurfaceDiscoveryRequiresArtifactDirectory(t *testing.T) {
	_, err := Run(context.Background(), Options{RepoPath: t.TempDir(), DiscoverSurfaces: true})
	if err == nil || !strings.Contains(err.Error(), "requires a debug directory") {
		t.Fatalf("error = %v", err)
	}
}

func writeSurfaceTestFile(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
