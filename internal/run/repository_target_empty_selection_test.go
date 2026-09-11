package run

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/snapshot"
)

// A Go repository whose only Python file is a test script (air) has a Python
// catalog but no required or selected Python target. Restoring that empty
// selection used to fail the whole run with "selected file set is empty".
func TestRepositoryNativeCandidatesAcceptAnEmptyPythonSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	files := map[string]string{
		"go.mod":                   "module example.com/air\n\ngo 1.22\n",
		"main.go":                  "package main\n\nfunc main() {}\n",
		"smoke_test/smoke_test.py": "def test_smoke():\n    assert True\n",
	}
	var paths []string
	for name, contents := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	ordinaryGraphGit(t, root, "init", "--quiet")
	ordinaryGraphGit(t, root, "add", ".")
	ordinaryGraphGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "lone test script")
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	source, err := snapshot.BuildContext(t.Context(), snapshot.Options{RepoPath: root, GoTarget: runtime.GOOS + "/" + runtime.GOARCH, RepositoryCorpus: repository})
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, DiscoverPython: true, GoSnapshot: &source})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := repositoryNativeCandidates(discovery)
	if err != nil {
		t.Fatalf("an empty Python selection failed the run: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Row.Language != "go" {
		t.Fatalf("expected only the Go program as a native candidate: %+v", candidates)
	}
}
