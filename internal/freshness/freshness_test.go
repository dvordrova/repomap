package freshness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestCaptureRepositoryDistinguishesDirtyContentsAtSameHead(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	clean := capture(t, repo)
	if len(clean.Dirty) != 0 {
		t.Fatalf("clean dirty files = %#v", clean.Dirty)
	}

	writeFile(t, filepath.Join(repo, "main.go"), "package fixture\n\nconst value = 1\n")
	first := capture(t, repo)
	writeFile(t, filepath.Join(repo, "main.go"), "package fixture\n\nconst value = 2\n")
	second := capture(t, repo)

	if first.Head != second.Head || first.Identity != second.Identity {
		t.Fatalf("test did not preserve repository identity: first=%#v second=%#v", first, second)
	}
	if len(first.Dirty) != 1 || first.Dirty[0].Status != "modified" || first.Dirty[0].Kind != FileRegular ||
		first.Dirty[0].Path != "main.go" || first.Dirty[0].ContentSHA256 == second.Dirty[0].ContentSHA256 {
		t.Fatalf("dirty fingerprints = %#v / %#v", first.Dirty, second.Dirty)
	}
	firstDigest, err := first.Digest()
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := second.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("different dirty contents produced the same repository digest")
	}
	assertReason(t, CompareRepository(first, second), ReasonRepositoryDirty, "main.go")
}

func TestCaptureRepositoryRecordsRenameAndUntrackedSymlinkWithoutFollowingIt(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	gitCommand(t, repo, "mv", "main.go", "renamed.go")
	outside := filepath.Join(t.TempDir(), "outside-secret")
	writeFile(t, outside, "must not be read")
	if err := os.Symlink(outside, filepath.Join(repo, "external-link")); err != nil {
		t.Fatal(err)
	}

	state := capture(t, repo)
	if len(state.Dirty) != 2 {
		t.Fatalf("dirty files = %#v", state.Dirty)
	}
	byPath := make(map[string]DirtyFile, len(state.Dirty))
	for _, file := range state.Dirty {
		byPath[file.Path] = file
	}
	rename := byPath["renamed.go"]
	if rename.Status != "renamed" || rename.FromPath != "main.go" || rename.Kind != FileRegular {
		t.Fatalf("rename fingerprint = %#v", rename)
	}
	link := byPath["external-link"]
	if link.Status != "untracked" || link.Kind != FileSymlink || link.ContentSHA256 != sha256Hex([]byte(outside)) {
		t.Fatalf("symlink fingerprint = %#v", link)
	}
}

func TestCaptureRepositoryHashesIgnoredBuildInputsButNotIgnoredSecrets(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	writeFile(t, filepath.Join(repo, ".gitignore"), "generated.go\n.env\n")
	gitCommand(t, repo, "add", ".gitignore")
	gitCommand(t, repo, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "ignore generated input")
	writeFile(t, filepath.Join(repo, "generated.go"), "package fixture\n\nconst generated = 1\n")
	writeFile(t, filepath.Join(repo, ".env"), "SECRET=must-not-be-read\n")

	first := capture(t, repo)
	if len(first.Dirty) != 1 || first.Dirty[0].Path != "generated.go" || first.Dirty[0].Status != "ignored" {
		t.Fatalf("captured inputs = %#v", first.Dirty)
	}
	writeFile(t, filepath.Join(repo, ".env"), "SECRET=changed-but-still-not-read\n")
	second := capture(t, repo)
	if differences := CompareRepository(first, second); len(differences) != 0 {
		t.Fatalf("ignored secret affected repository state: %#v", differences)
	}
	writeFile(t, filepath.Join(repo, "generated.go"), "package fixture\n\nconst generated = 2\n")
	third := capture(t, repo)
	assertReason(t, CompareRepository(second, third), ReasonRepositoryDirty, "generated.go")
}

func TestFactAndClaimContextsDigestAndExplainDifferences(t *testing.T) {
	t.Parallel()

	facts := fixtureFactContext()
	firstDigest, err := facts.Digest()
	if err != nil {
		t.Fatal(err)
	}
	changed := facts
	changed.Build.GOARCH = "arm64"
	changed.AnalyzerVersion = "v0.24.0"
	secondDigest, err := changed.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("different fact contexts produced the same digest")
	}
	differences := CompareFactContext(facts, changed)
	assertReason(t, differences, ReasonAnalyzerVersion, "")
	assertReason(t, differences, ReasonBuildContext, "")

	claim := ClaimContext{
		Version:          ClaimContextVersion,
		FactDigest:       firstDigest,
		Provider:         "openai-compatible",
		Model:            "deepseek-v4-flash",
		PromptVersion:    "source-v1",
		ParserVersion:    1,
		EvaluatorVersion: 1,
	}
	if _, err := claim.Digest(); err != nil {
		t.Fatal(err)
	}
	changedClaim := claim
	changedClaim.PromptVersion = "source-v2"
	changedClaim.EvaluatorVersion = 2
	claimDifferences := CompareClaimContext(claim, changedClaim)
	assertReason(t, claimDifferences, ReasonPromptVersion, "")
	assertReason(t, claimDifferences, ReasonEvaluatorVersion, "")
}

func TestContextValidationRejectsNonCanonicalInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*FactContext)
	}{
		{name: "relative identity", mutate: func(context *FactContext) { context.Repository.Identity = "repo" }},
		{name: "unsorted dirty files", mutate: func(context *FactContext) {
			context.Repository.Dirty = []DirtyFile{
				fixtureDirty("z.go", strings.Repeat("b", 64)),
				fixtureDirty("a.go", strings.Repeat("c", 64)),
			}
		}},
		{name: "unsorted build tags", mutate: func(context *FactContext) { context.Build.BuildTags = []string{"unit", "integration"} }},
		{name: "missing analyzer version", mutate: func(context *FactContext) { context.AnalyzerVersion = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := fixtureFactContext()
			test.mutate(&context)
			if err := context.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestCompareRepositoryTreatsNilAndEmptyDirtyListsEqually(t *testing.T) {
	t.Parallel()

	left := fixtureFactContext().Repository
	right := left
	left.Dirty = nil
	right.Dirty = []DirtyFile{}
	if differences := CompareRepository(left, right); len(differences) != 0 {
		t.Fatalf("differences = %#v", differences)
	}
	leftDigest, err := left.Digest()
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := right.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("nil and empty digests differ: %s != %s", leftDigest, rightDigest)
	}
}

func TestCompareRepositoryRenameReportsOldAndNewPaths(t *testing.T) {
	t.Parallel()

	clean := fixtureFactContext().Repository
	renamed := clean
	renamed.Dirty = []DirtyFile{{
		Status:        "renamed",
		Path:          "renamed.go",
		FromPath:      "main.go",
		Kind:          FileRegular,
		ContentSHA256: strings.Repeat("b", 64),
	}}

	differences := CompareRepository(clean, renamed)
	for _, difference := range differences {
		if difference.Reason == ReasonRepositoryDirty && reflect.DeepEqual(difference.Paths, []string{"main.go", "renamed.go"}) {
			return
		}
	}
	t.Fatalf("differences = %#v", differences)
}

func newRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCommand(t, dir, "init", "--quiet")
	writeFile(t, filepath.Join(dir, "main.go"), "package fixture\n")
	gitCommand(t, dir, "add", "main.go")
	gitCommand(t, dir, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "fixture")
	return dir
}

func capture(t *testing.T, repo string) RepositoryState {
	t.Helper()
	state, err := CaptureRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func fixtureFactContext() FactContext {
	return FactContext{
		Version: FactContextVersion,
		Repository: RepositoryState{
			Version:  RepositoryStateVersion,
			Identity: "/repo",
			Head:     strings.Repeat("a", 40),
			Dirty:    []DirtyFile{},
		},
		GoVersion:        "go1.24.0",
		Analyzer:         "gopls",
		AnalyzerVersion:  "v0.23.0",
		Collector:        "symbol-neighborhood",
		CollectorVersion: "v1",
		InputsSHA256:     strings.Repeat("d", 64),
		Build: evidence.BuildContext{
			GOOS:      "linux",
			GOARCH:    "amd64",
			BuildTags: []string{"integration", "unit"},
		},
	}
}

func fixtureDirty(path, digest string) DirtyFile {
	return DirtyFile{Status: "modified", Path: path, Kind: FileRegular, ContentSHA256: digest}
}

func assertReason(t *testing.T, differences []Difference, reason Reason, path string) {
	t.Helper()
	for _, difference := range differences {
		if difference.Reason != reason {
			continue
		}
		if path == "" || reflect.DeepEqual(difference.Paths, []string{path}) {
			return
		}
	}
	t.Fatalf("differences = %#v, want reason %q path %q", differences, reason, path)
}

func gitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
