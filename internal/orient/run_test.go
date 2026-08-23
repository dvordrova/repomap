package orient

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCanceledContextStopsBeforeSnapshotPublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, Options{RepoPath: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context canceled", err)
	}
}

func TestRunRequiredArtifactsNeedDebugDirectory(t *testing.T) {
	err := Run(context.Background(), Options{
		RepoPath:         t.TempDir(),
		RequireArtifacts: true,
	})
	if err == nil || !strings.Contains(err.Error(), "need a debug directory") {
		t.Fatalf("Run error = %v, want missing debug directory", err)
	}
}

func TestRunRequiredArtifactsRejectBlockedDebugDirectory(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/dump-failure\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "main.go", "package main\nfunc main() {}\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "main.go")

	blockedDebugDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedDebugDir, []byte("file blocks mkdir"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), prepareOrientRunOptions(t, repository, Options{
		RepoPath:         repository,
		RunID:            "required-browser-artifacts",
		DebugDir:         blockedDebugDir,
		DumpRedacted:     true,
		RequireArtifacts: true,
	}))
	if err == nil || !strings.Contains(err.Error(), "create required debug writer") {
		t.Fatalf("Run error = %v, want required writer failure", err)
	}
}

func runOrientGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repository}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", commandArgs, err, output)
	}
}
