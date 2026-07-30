package freshness

import (
	"context"
	"fmt"
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

func TestCaptureRepositoryIgnoresUntrackedNestedRepository(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	nested := filepath.Join(repo, "scratch", "tool")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, nested, "init", "--quiet")
	writeFile(t, filepath.Join(nested, "main.go"), "package nested\n")
	writeFile(t, filepath.Join(nested, ".env"), "SECRET=must-not-be-read\n")
	gitCommand(t, nested, "add", "main.go")
	gitCommand(t, nested, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "nested")

	first := capture(t, repo)
	if len(first.Dirty) != 0 || len(first.Submodules) != 0 {
		t.Fatalf("nested untracked repository affected parent state: %#v", first)
	}

	writeFile(t, filepath.Join(nested, "main.go"), "package nested\n\nconst changed = true\n")
	writeFile(t, filepath.Join(nested, ".env"), "SECRET=changed-but-still-not-read\n")
	second := capture(t, repo)
	if differences := CompareRepository(first, second); len(differences) != 0 {
		t.Fatalf("nested untracked repository affected parent freshness: %#v", differences)
	}
}

func TestCaptureRepositoryTreatsExcludedSubmoduleDirtAsInformational(t *testing.T) {
	root, submodule := repositoryWithSubmodule(t)
	clean := capture(t, root)
	if len(clean.Submodules) != 1 || clean.Submodules[0].Path != "deps/platform" ||
		clean.Submodules[0].Availability != SubmoduleClean || clean.Submodules[0].IncludedInAnalysis {
		t.Fatalf("clean submodule = %#v", clean.Submodules)
	}

	writeFile(t, filepath.Join(submodule, "module.go"), "package platform\n\nconst Value = 2\n")
	modified := capture(t, root)
	if !modified.Submodules[0].WorktreeModified || len(modified.Dirty) != 0 {
		t.Fatalf("modified excluded submodule = %#v / dirty=%#v", modified.Submodules, modified.Dirty)
	}

	writeFile(t, filepath.Join(submodule, "scratch.txt"), "untracked\n")
	untracked := capture(t, root)
	if !untracked.Submodules[0].WorktreeUntracked {
		t.Fatalf("untracked excluded submodule = %#v", untracked.Submodules)
	}

	writeFile(t, filepath.Join(submodule, ".gitignore"), ".env\n")
	writeFile(t, filepath.Join(submodule, ".env"), "SECRET=must-not-be-read\n")
	ignored := capture(t, root)
	if len(ignored.Submodules) != 1 || ignored.Submodules[0].Path != "deps/platform" {
		t.Fatalf("ignored submodule secret leaked into state: %#v", ignored)
	}
}

func TestCaptureRepositoryRecordsExcludedSubmoduleHeadMismatch(t *testing.T) {
	root, submodule := repositoryWithSubmodule(t)
	writeFile(t, filepath.Join(submodule, "module.go"), "package platform\n\nconst Value = 3\n")
	gitCommand(t, submodule, "add", "module.go")
	gitCommand(t, submodule, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "advance")

	state := capture(t, root)
	if len(state.Submodules) != 1 || !state.Submodules[0].GitlinkChanged ||
		state.Submodules[0].CurrentHead == state.Submodules[0].RecordedGitlink {
		t.Fatalf("submodule HEAD mismatch = %#v", state.Submodules)
	}
}

func TestCaptureRepositoryDoesNotRecurseIntoDirtyNestedExcludedSubmodule(t *testing.T) {
	nestedSource := newRepository(t)
	outerSource := newRepository(t)
	gitCommand(t, outerSource, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", nestedSource, "nested/tool")
	gitCommand(t, outerSource, "add", ".gitmodules", "nested/tool")
	gitCommand(t, outerSource, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "nested submodule")

	root := newRepository(t)
	gitCommand(t, root, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", outerSource, "deps/platform")
	gitCommand(t, root, "add", ".gitmodules", "deps/platform")
	gitCommand(t, root, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "outer submodule")
	gitCommand(t, filepath.Join(root, "deps", "platform"), "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--quiet")
	writeFile(t, filepath.Join(root, "deps", "platform", "nested", "tool", "main.go"), "package fixture\n\nconst nested = true\n")

	state := capture(t, root)
	if len(state.Submodules) != 1 || state.Submodules[0].Path != "deps/platform" || !state.Submodules[0].WorktreeModified {
		t.Fatalf("root recursively exposed nested submodule state: %#v", state.Submodules)
	}
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

func TestAssessInputsSeparatesUnrelatedAndAnalyzedChanges(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	initial := capture(t, repo)
	inputs, err := CaptureInputs(context.Background(), initial, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "notes.txt"), "unrelated\n")
	unrelated := capture(t, repo)
	result := AssessInputs(context.Background(), initial, unrelated, inputs)
	if result.State != FreshnessUnrelatedChanges || result.AnalyzedChanges || !result.UnrelatedChanges {
		t.Fatalf("unrelated result = %#v", result)
	}

	writeFile(t, filepath.Join(repo, "main.go"), "package fixture\n\nconst changed = true\n")
	stale := capture(t, repo)
	result = AssessInputs(context.Background(), initial, stale, inputs)
	if result.State != FreshnessPartiallyStale || !result.AnalyzedChanges ||
		!reflect.DeepEqual(result.AffectedPaths, []string{"main.go"}) {
		t.Fatalf("stale result = %#v", result)
	}
}

func TestCaptureInputsIgnoresAmbientAlternateGitIdentity(t *testing.T) {
	wanted := newRepository(t)
	const wantedContent = "package fixture\n\nconst wanted = true\n"
	writeFile(t, filepath.Join(wanted, "main.go"), wantedContent)
	gitCommand(t, wanted, "add", "main.go")
	gitCommand(t, wanted, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "wanted content")
	wantedState := capture(t, wanted)

	other := newRepository(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))

	inputs, err := CaptureInputs(context.Background(), wantedState, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0].Path != "main.go" ||
		inputs[0].ContentSHA256 != sha256Hex([]byte(wantedContent)) {
		t.Fatalf("captured inputs = %#v", inputs)
	}
}

func TestCaptureInputsBatchesRegularSymlinkAndMissingPaths(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	if err := os.MkdirAll(filepath.Join(repo, "batch"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths := []string{"missing.go"}
	for index := 0; index < 64; index++ {
		path := fmt.Sprintf("batch/file-%02d.go", index)
		writeFile(t, filepath.Join(repo, path), fmt.Sprintf("package batch\n\nconst Value%02d = %d\n", index, index))
		paths = append(paths, path)
	}
	writeFile(t, filepath.Join(repo, ":literal.go"), "package fixture\n\nconst Literal = true\n")
	paths = append(paths, ":literal.go")
	if err := os.Symlink("batch/file-00.go", filepath.Join(repo, "batch-link")); err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "batch-link")
	gitCommand(t, repo, "add", "--", "batch", "batch-link", ":(literal):literal.go")
	gitCommand(
		t,
		repo,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"commit", "--quiet", "-m", "batch inputs",
	)

	inputs, err := CaptureInputs(context.Background(), capture(t, repo), paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != len(paths) {
		t.Fatalf("captured input count = %d, want %d", len(inputs), len(paths))
	}
	byPath := make(map[string]CapturedInput, len(inputs))
	for _, input := range inputs {
		byPath[input.Path] = input
	}
	if input := byPath["batch/file-00.go"]; input.Kind != FileRegular || input.Mode != "100644" ||
		input.ContentSHA256 != sha256Hex([]byte("package batch\n\nconst Value00 = 0\n")) {
		t.Fatalf("regular input = %#v", input)
	}
	if input := byPath["batch-link"]; input.Kind != FileSymlink || input.Mode != "120000" ||
		input.ContentSHA256 != sha256Hex([]byte("batch/file-00.go")) {
		t.Fatalf("symlink input = %#v", input)
	}
	if input := byPath[":literal.go"]; input.Kind != FileRegular ||
		input.ContentSHA256 != sha256Hex([]byte("package fixture\n\nconst Literal = true\n")) {
		t.Fatalf("literal-path input = %#v", input)
	}
	if input := byPath["missing.go"]; input.Kind != FileMissing || input.Mode != "" ||
		input.ContentSHA256 != "" {
		t.Fatalf("missing input = %#v", input)
	}
}

func TestIsolatedGitEnvironmentDropsGitConfigInjectionAndNeutralizesGlobalConfig(t *testing.T) {
	got := strings.Join(isolatedGitEnvironment([]string{
		"PATH=/usr/bin",
		"GIT_CONFIG=/tmp/injected",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.bare",
		"GIT_CONFIG_VALUE_0=true",
		"GIT_CONFIG_PARAMETERS='core.worktree'='/tmp/other'",
		"GIT_CONFIG_GLOBAL=/tmp/global",
		"GIT_CONFIG_SYSTEM=/tmp/system",
	}), "\n")
	for _, forbidden := range []string{
		"GIT_CONFIG=/tmp/injected", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=",
		"GIT_CONFIG_VALUE_0=", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_GLOBAL=/tmp/global",
		"GIT_CONFIG_SYSTEM=/tmp/system",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("isolated environment retained %q: %q", forbidden, got)
		}
	}
	for _, required := range []string{
		"PATH=/usr/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("isolated environment lacks %q: %q", required, got)
		}
	}
}

func TestAssessInputsDoesNotCallAnalyzedChangeUnrelated(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	initial := capture(t, repo)
	inputs, err := CaptureInputs(context.Background(), initial, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "main.go"), "package fixture\n\nconst changed = true\n")
	result := AssessInputs(context.Background(), initial, capture(t, repo), inputs)
	if result.State != FreshnessPartiallyStale || !result.AnalyzedChanges || result.UnrelatedChanges {
		t.Fatalf("analyzed-only result = %#v", result)
	}
}

func TestAssessInputsScopesChangesAcrossCommits(t *testing.T) {
	t.Parallel()

	repo := newRepository(t)
	writeFile(t, filepath.Join(repo, "notes.txt"), "initial\n")
	gitCommand(t, repo, "add", "notes.txt")
	gitCommand(t, repo, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "notes")
	initial := capture(t, repo)
	inputs, err := CaptureInputs(context.Background(), initial, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(repo, "notes.txt"), "unrelated commit\n")
	gitCommand(t, repo, "add", "notes.txt")
	gitCommand(t, repo, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "unrelated")
	result := AssessInputs(context.Background(), initial, capture(t, repo), inputs)
	if result.State != FreshnessUnrelatedChanges || result.AnalyzedChanges {
		t.Fatalf("unrelated commit result = %#v", result)
	}

	writeFile(t, filepath.Join(repo, "main.go"), "package fixture\n\nconst committed = true\n")
	gitCommand(t, repo, "add", "main.go")
	gitCommand(t, repo, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "analyzed")
	result = AssessInputs(context.Background(), initial, capture(t, repo), inputs)
	if result.State != FreshnessPartiallyStale || !result.AnalyzedChanges || !result.UnrelatedChanges {
		t.Fatalf("analyzed commit result = %#v", result)
	}
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

func repositoryWithSubmodule(t *testing.T) (string, string) {
	t.Helper()
	source := newRepository(t)
	writeFile(t, filepath.Join(source, "module.go"), "package platform\n\nconst Value = 1\n")
	gitCommand(t, source, "add", "module.go")
	gitCommand(t, source, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "module")

	root := newRepository(t)
	gitCommand(t, root, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", source, "deps/platform")
	gitCommand(t, root, "add", ".gitmodules", "deps/platform")
	gitCommand(t, root, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid", "commit", "--quiet", "-m", "submodule")
	return root, filepath.Join(root, "deps", "platform")
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
