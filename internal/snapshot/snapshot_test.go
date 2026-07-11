package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildReadsBoundedTrackedReadme(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	trackSnapshotFiles(t, repo, "README.md")

	got, err := Build(Options{RepoPath: repo, MaxReadmeBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Readme != "abc\n...[truncated]" {
		t.Fatalf("readme = %q", got.Readme)
	}
}

func TestBuildDoesNotReadTrackedReadmeSymlinkEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("do not disclose"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "README.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	trackSnapshotFiles(t, repo, "README.md")

	got, err := Build(Options{RepoPath: repo, MaxReadmeBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got.Readme != "" {
		t.Fatalf("readme = %q, want empty", got.Readme)
	}
}

func TestBuildDoesNotReadUntrackedReadme(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("local private notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	trackSnapshotFiles(t, repo)

	got, err := Build(Options{RepoPath: repo, MaxReadmeBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got.Readme != "" {
		t.Fatalf("readme = %q, want empty for untracked file", got.Readme)
	}
}

func TestShouldSkipPath(t *testing.T) {
	cases := []struct {
		path string
		skip bool
	}{
		{"vendor/a.go", true},
		{"node_modules/pkg/index.js", true},
		{".github/workflows/ci.yml", true},
		{"configs/.env", true},
		{"certs/tls.key", true},
		{"images/logo.png", true},
		{"cmd/repomap/main.go", false},
	}

	for _, tc := range cases {
		if got := shouldSkipPath(tc.path); got != tc.skip {
			t.Fatalf("shouldSkipPath(%q)=%v, want %v", tc.path, got, tc.skip)
		}
	}
}

func TestParseModuleName(t *testing.T) {
	got := parseModuleName([]byte("module github.com/example/demo\n\ngo 1.22\n"))
	if got != "github.com/example/demo" {
		t.Fatalf("parseModuleName()=%q, want %q", got, "github.com/example/demo")
	}
}

func TestBuildDoesNotReadTrackedGoModSymlinkEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(outside, []byte("module private.example/outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "go.mod")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	trackSnapshotFiles(t, repo, "go.mod")

	got, err := Build(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if got.Go.GoModExists || got.Go.ModuleName != "" {
		t.Fatalf("Go hints = %#v, want escaping go.mod ignored", got.Go)
	}
}

func TestTruncateUTF8Bytes(t *testing.T) {
	in := "ábc"
	got, truncated := truncateUTF8Bytes(in, 1)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if got != "" {
		t.Fatalf("got %q, want empty valid UTF-8 string", got)
	}

	got, truncated = truncateUTF8Bytes(in, 2)
	if !truncated {
		t.Fatal("expected truncation at 2")
	}
	if got != "á" {
		t.Fatalf("got %q, want %q", got, "á")
	}
}

func trackSnapshotFiles(t *testing.T, repo string, paths ...string) {
	t.Helper()

	runSnapshotGit(t, "init", "--quiet", repo)
	args := []string{"-C", repo, "add", "--"}
	args = append(args, paths...)
	runSnapshotGit(t, args...)
}

func runSnapshotGit(t *testing.T, args ...string) {
	t.Helper()

	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
