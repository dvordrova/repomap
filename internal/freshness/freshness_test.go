package freshness

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

func TestCaptureRepositoryTracksOnlyTrackedChanges(t *testing.T) {
	repository := testRepository(t)
	writeTestFile(t, repository, "main.go", "package main\n")
	writeTestFile(t, repository, ".npmrc", "//registry.example.test/:_authToken=initial\n")
	writeTestFile(t, repository, "client/.env.production", "SECRET=initial\n")
	writeTestFile(t, repository, "node_modules/tracked.js", "initial\n")
	gitTest(t, repository, "add", "main.go", ".npmrc", "client/.env.production", "node_modules/tracked.js")
	gitTest(t, repository, "commit", "-m", "initial")

	clean, err := captureTestRepository(t, repository)
	if err != nil {
		t.Fatalf("capture clean repository: %v", err)
	}
	writeTestFile(t, repository, "main.go", "package changed\n")
	writeTestFile(t, repository, ".npmrc", "//registry.example.test/:_authToken=changed\n")
	writeTestFile(t, repository, "client/.env.production", "SECRET=changed\n")
	writeTestFile(t, repository, "node_modules/tracked.js", "changed\n")
	writeTestFile(t, repository, "scratch.txt", "untracked\n")
	writeTestFile(t, repository, ".gitignore", "generated.go\n")
	writeTestFile(t, repository, "generated.go", "package generated\n")

	dirty, err := captureTestRepository(t, repository)
	if err != nil {
		t.Fatalf("capture dirty repository: %v", err)
	}
	if dirty.Head != clean.Head || len(dirty.Dirty) != 1 || dirty.Dirty[0].Path != "main.go" {
		t.Fatalf("unexpected tracked authority: %#v", dirty)
	}
	want := fmt.Sprintf("%x", sha256.Sum256([]byte("package changed\n")))
	if dirty.Dirty[0].ContentSHA256 != want {
		t.Fatalf("dirty content digest = %q, want %q", dirty.Dirty[0].ContentSHA256, want)
	}
}

func TestCaptureInputsUsesCapturedRevisionAndDirtyIdentity(t *testing.T) {
	repository := testRepository(t)
	writeTestFile(t, repository, "clean.go", "package clean\n")
	writeTestFile(t, repository, "dirty.go", "package initial\n")
	gitTest(t, repository, "add", "clean.go", "dirty.go")
	gitTest(t, repository, "commit", "-m", "initial")
	writeTestFile(t, repository, "dirty.go", "package dirty\n")

	state, err := captureTestRepository(t, repository)
	if err != nil {
		t.Fatalf("capture repository: %v", err)
	}
	writeTestFile(t, repository, "clean.go", "package changed_later\n")
	inputs, err := CaptureInputs(
		context.Background(),
		state,
		[]string{"dirty.go", "clean.go", "missing.go"},
	)
	if err != nil {
		t.Fatalf("capture inputs: %v", err)
	}
	if len(inputs) != 3 || inputs[0].Path != "clean.go" || inputs[1].Path != "dirty.go" || inputs[2].Path != "missing.go" {
		t.Fatalf("unexpected captured inputs: %#v", inputs)
	}
	cleanDigest := fmt.Sprintf("%x", sha256.Sum256([]byte("package clean\n")))
	dirtyDigest := fmt.Sprintf("%x", sha256.Sum256([]byte("package dirty\n")))
	if inputs[0].ContentSHA256 != cleanDigest || inputs[1].ContentSHA256 != dirtyDigest || inputs[2].Kind != FileMissing {
		t.Fatalf("captured inputs do not match captured authority: %#v", inputs)
	}
	if _, err := CapturedInputsDigest(inputs); err != nil {
		t.Fatalf("digest captured inputs: %v", err)
	}
}

func TestCaptureInputsRejectsUnavailableCapturedTree(t *testing.T) {
	repository := testRepository(t)
	writeTestFile(t, repository, "main.go", "package main\n")
	gitTest(t, repository, "add", "main.go")
	gitTest(t, repository, "commit", "-m", "initial")
	state, err := captureTestRepository(t, repository)
	if err != nil {
		t.Fatalf("capture repository: %v", err)
	}
	state.Head = strings.Repeat("0", len(state.Head))
	if inputs, err := CaptureInputs(context.Background(), state, []string{"main.go"}); err == nil || inputs != nil ||
		!strings.Contains(err.Error(), "validate captured commit tree") {
		t.Fatalf("inputs = %#v, error = %v", inputs, err)
	}
}

func TestCapturedInputsDigestRejectsDuplicatePaths(t *testing.T) {
	input := CapturedInput{
		Version:       CapturedInputVersion,
		ID:            strings.Repeat("a", 64),
		Path:          "main.go",
		Kind:          FileRegular,
		Mode:          "100644",
		ContentSHA256: strings.Repeat("b", 64),
		Stages:        []string{"report_evidence"},
	}
	if _, err := CapturedInputsDigest([]CapturedInput{input, input}); err == nil {
		t.Fatal("duplicate captured paths were accepted")
	}
}

func testRepository(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	gitTest(t, directory, "init")
	gitTest(t, directory, "config", "user.email", "repomap@example.test")
	gitTest(t, directory, "config", "user.name", "repomap test")
	return directory
}

func captureTestRepository(t *testing.T, repository string) (RepositoryState, error) {
	t.Helper()
	repositoryCorpus, err := corpus.Open(context.Background(), repository)
	if err != nil {
		return RepositoryState{}, err
	}
	defer repositoryCorpus.Close()
	return CaptureRepository(context.Background(), repository, repositoryCorpus)
}

func gitTest(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func writeTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}
