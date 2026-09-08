package freshness

import (
	"context"
	"errors"
	"github.com/dvordrova/repomap/internal/corpus"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func openRevisionCorpus(t *testing.T, root string, exclusions ...string) *corpus.Corpus {
	t.Helper()
	repository, err := corpus.Open(t.Context(), root, exclusions...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository
}
func assertUnavailablePaths(t *testing.T, root string, repository *corpus.Corpus, state RepositoryState, want ...string) {
	t.Helper()
	before := repository.Snapshot()
	got, err := UnavailableSourcePaths(t.Context(), root, repository, state)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(want)
	if len(got) != len(want) || len(want) > 0 && !reflect.DeepEqual(got, want) {
		t.Fatalf("unavailable paths = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(repository.Snapshot(), before) {
		t.Fatal("source-link check changed the corpus")
	}
}
func TestUnavailableSourcePathsUseCapturedRevision(t *testing.T) {
	root := testRepository(t)
	writeTestFile(t, root, "main.py", "def main(): pass\n")
	gitTest(t, root, "add", "main.py")
	gitTest(t, root, "commit", "-m", "initial")
	state, err := captureTestRepository(t, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "new.py", "def new(): pass\n")
	repository := openRevisionCorpus(t, root)
	for _, stage := range []string{"untracked", "staged", "committed after capture"} {
		switch stage {
		case "staged":
			gitTest(t, root, "add", "new.py")
		case "committed after capture":
			gitTest(t, root, "commit", "-m", "later")
		}
		t.Run(stage, func(t *testing.T) { assertUnavailablePaths(t, root, repository, state, "new.py") })
	}
	current, err := CaptureRepository(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	assertUnavailablePaths(t, root, repository, current)
}
func TestUnavailableSourcePathsIncludeAllMissingAndChangedFiles(t *testing.T) {
	root := testRepository(t)
	for _, path := range []string{"clean.py", "dirty.py", "staged.py", "old name 한글.py", "excluded.py"} {
		writeTestFile(t, root, path, "original "+path+"\n")
	}
	writeTestFile(t, root, ".gitignore", "ignored notes.py\n")
	gitTest(t, root, "add", ".")
	gitTest(t, root, "commit", "-m", "initial")
	writeTestFile(t, root, "dirty.py", "worktree change\n")
	writeTestFile(t, root, "staged.py", "staged change\n")
	gitTest(t, root, "add", "staged.py")
	gitTest(t, root, "mv", "old name 한글.py", "new name 한국.py")
	// The old path exists in HEAD, but the captured rename's FromPath makes it
	// unavailable even after an untracked file recreates that readable name.
	writeTestFile(t, root, "old name 한글.py", "recreated old path\n")
	writeTestFile(t, root, "untracked 한글.py", "untracked\n")
	writeTestFile(t, root, "ignored notes.py", "ignored but readable\n")
	writeTestFile(t, root, "excluded.py", "changed outside the corpus\n")
	writeTestFile(t, root, ".env.local", "excluded\n")
	repository := openRevisionCorpus(t, root, "excluded.py")
	state, err := CaptureRepository(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	renamed := false
	for _, dirty := range state.Dirty {
		if dirty.Path == "new name 한국.py" && dirty.FromPath == "old name 한글.py" {
			renamed = true
		}
	}
	if !renamed {
		t.Fatal("fixture did not capture the staged rename")
	}
	assertUnavailablePaths(t, root, repository, state, "dirty.py", "staged.py", "old name 한글.py", "new name 한국.py", "untracked 한글.py", "ignored notes.py")
	// Later commits do not replace the captured dirty authority.
	gitTest(t, root, "add", "dirty.py", "new name 한국.py", "old name 한글.py", "staged.py")
	gitTest(t, root, "commit", "-m", "later")
	assertUnavailablePaths(t, root, repository, state, "dirty.py", "staged.py", "old name 한글.py", "new name 한국.py", "untracked 한글.py", "ignored notes.py")
}
func TestUnavailableSourcePathsUseExactNestedAnalysisPrefix(t *testing.T) {
	root := testRepository(t)
	for _, path := range []string{"app/name.py", "app/dirty.py", "app/outbound.py", "name.py", "app2/name.py", "inbound.py"} {
		writeTestFile(t, root, path, "original "+path+"\n")
	}
	gitTest(t, root, "add", ".")
	gitTest(t, root, "commit", "-m", "initial")
	writeTestFile(t, root, "name.py", "outside\n")
	writeTestFile(t, root, "app2/name.py", "similar prefix\n")
	writeTestFile(t, root, "app/dirty.py", "inside\n")
	gitTest(t, root, "mv", "app/outbound.py", "moved-out.py")
	writeTestFile(t, root, "app/outbound.py", "recreated\n")
	gitTest(t, root, "mv", "inbound.py", "app/moved-in.py")
	writeTestFile(t, root, "app/new space 한글.py", "new\n")
	analysisRoot := filepath.Join(root, "app")
	repository := openRevisionCorpus(t, analysisRoot)
	state, err := CaptureRepository(t.Context(), analysisRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	assertUnavailablePaths(t, analysisRoot, repository, state, "dirty.py", "outbound.py", "moved-in.py", "new space 한글.py")
}
func TestUnavailableSourcePathsKeepSubmoduleDescendants(t *testing.T) {
	submodule := testRepository(t)
	writeTestFile(t, submodule, "space 한글.py", "submodule source\n")
	gitTest(t, submodule, "add", ".")
	gitTest(t, submodule, "commit", "-m", "submodule")
	root := testRepository(t)
	writeTestFile(t, root, "main.py", "parent source\n")
	gitTest(t, root, "add", ".")
	gitTest(t, root, "commit", "-m", "initial")
	gitTest(t, root, "-c", "protocol.file.allow=always", "submodule", "add", submodule, "deps/local module")
	gitTest(t, root, "commit", "-am", "add submodule")
	repository := openRevisionCorpus(t, root)
	state, err := CaptureRepository(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	assertUnavailablePaths(t, root, repository, state, "deps/local module/space 한글.py")
}
func TestUnavailableSourcePathsRetainActualErrors(t *testing.T) {
	root := testRepository(t)
	writeTestFile(t, root, "main.py", "source\n")
	gitTest(t, root, "add", ".")
	gitTest(t, root, "commit", "-m", "initial")
	repository := openRevisionCorpus(t, root)
	state, err := CaptureRepository(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := UnavailableSourcePaths(ctx, root, repository, state); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled revision lookup = %v", err)
	}
	if _, err := UnavailableSourcePaths(t.Context(), root, nil, state); err == nil {
		t.Fatal("nil corpus was accepted")
	}
	if _, err := UnavailableSourcePaths(t.Context(), t.TempDir(), repository, state); err == nil {
		t.Fatal("analysis root outside captured repository was accepted")
	}
	state.Head = strings.Repeat("0", 40)
	if _, err := UnavailableSourcePaths(t.Context(), root, repository, state); err == nil {
		t.Fatal("invalid Git revision became unavailable paths")
	}
}
