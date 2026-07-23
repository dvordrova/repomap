package llmbundle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestBuildIsInvariantToCheckoutBasename(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstRepo := filepath.Join(root, "openapi-required-nullable-semantics")
	secondRepo := filepath.Join(root, "accept-header-wrong-status")
	for _, repo := range []string{firstRepo, secondRepo} {
		writeIdentityFixture(t, repo, "package.json", `{"name":"example-project"}`)
		writeIdentityFixture(t, repo, "README.md", "# Example project\n")
		writeIdentityFixture(t, repo, "src/app.py", "def main():\n    return 0\n")
		initIdentityFixture(t, repo, "package.json", "README.md", "src/app.py")
	}

	firstSnapshot, err := snapshot.Build(snapshot.Options{RepoPath: firstRepo})
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := snapshot.Build(snapshot.Options{RepoPath: secondRepo})
	if err != nil {
		t.Fatal(err)
	}
	if firstSnapshot.DisplayName == secondSnapshot.DisplayName {
		t.Fatalf("display labels unexpectedly equal: %q", firstSnapshot.DisplayName)
	}
	if firstSnapshot.RepoName != "example-project" || secondSnapshot.RepoName != firstSnapshot.RepoName {
		t.Fatalf("semantic identities differ: first=%q second=%q", firstSnapshot.RepoName, secondSnapshot.RepoName)
	}

	firstBundle := Build(firstSnapshot, firstSnapshot.FilteredFiles, Options{})
	secondBundle := Build(secondSnapshot, secondSnapshot.FilteredFiles, Options{})
	if !reflect.DeepEqual(firstBundle, secondBundle) {
		t.Fatalf("provider bundles differ by checkout basename:\nfirst:  %#v\nsecond: %#v", firstBundle, secondBundle)
	}
	encoded, err := json.Marshal(firstBundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{filepath.Base(firstRepo), filepath.Base(secondRepo), "display_name"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("provider bundle leaked checkout display label %q: %s", forbidden, encoded)
		}
	}
}

func writeIdentityFixture(t *testing.T, repo, relativePath, contents string) {
	t.Helper()

	filePath := filepath.Join(repo, relativePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func initIdentityFixture(t *testing.T, repo string, paths ...string) {
	t.Helper()

	runIdentityGit(t, "init", "--quiet", repo)
	args := []string{"-C", repo, "add", "--"}
	args = append(args, paths...)
	runIdentityGit(t, args...)
}

func runIdentityGit(t *testing.T, args ...string) {
	t.Helper()

	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
