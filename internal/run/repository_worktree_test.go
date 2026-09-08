package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestRepositoryCorpusWorkingTreeChangesUseTheAnalysisRoot(t *testing.T) {
	gitRoot := t.TempDir()
	nestedRoot := filepath.Join(gitRoot, "testdata", "acceptance", "python-tutorial-game")
	if err := os.MkdirAll(nestedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := []string{"README.md", "src/main.py"}
	nested, err := corpus.New(t.Context(), nestedRoot, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nested.Close() })
	root, err := corpus.New(t.Context(), gitRoot, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	const prefix = "testdata/acceptance/python-tutorial-game/"
	for _, test := range []struct {
		name, path, from string
		analysisRoot     string
		repository       *corpus.Corpus
		want             bool
	}{
		{"outside basename collision", "README.md", "", nestedRoot, nested, false},
		{"outside directory collision", "src/main.py", "", nestedRoot, nested, false},
		{"sibling with same prefix", "testdata/acceptance/python-tutorial-game-copy/README.md", "", nestedRoot, nested, false},
		{"modified nested README", prefix + "README.md", "", nestedRoot, nested, true},
		{"modified nested source", prefix + "src/main.py", "", nestedRoot, nested, true},
		{"nested file outside corpus", prefix + "notes.md", "", nestedRoot, nested, false},
		{"rename into analyzed source", prefix + "README.md", "README.md", nestedRoot, nested, true},
		{"rename from analyzed source", "moved.md", prefix + "README.md", nestedRoot, nested, true},
		{"rename outside with basename collision", "moved.md", "README.md", nestedRoot, nested, false},
		{"root analysis still detects modification", "README.md", "", gitRoot, root, true},
		{"root analysis still detects rename source", "moved.md", "src/main.py", gitRoot, root, true},
		{"root analysis still excludes noncorpus file", "notes.md", "", gitRoot, root, false},
		{"missing corpus", prefix + "README.md", "", nestedRoot, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := "modified"
			if test.from != "" {
				status = "renamed"
			}
			state := freshness.RepositoryState{
				Version: freshness.RepositoryStateVersion, Identity: gitRoot, Head: strings.Repeat("a", 40),
				Dirty: []freshness.DirtyFile{{Status: status, Path: test.path, FromPath: test.from,
					Kind: freshness.FileRegular, ContentSHA256: strings.Repeat("b", 64)}},
			}
			if err := state.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := repositoryCorpusHasWorkingTreeChanges(test.repository, test.analysisRoot, state); got != test.want {
				t.Fatalf("working-tree changes = %t, want %t for %s (from %s), analyzing %s", got, test.want, test.path, test.from, test.analysisRoot)
			}
		})
	}
}
