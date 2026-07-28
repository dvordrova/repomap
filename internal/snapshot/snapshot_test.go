package snapshot

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBuildUsesSemanticRepositoryIdentity(t *testing.T) {
	t.Parallel()

	t.Run("root module precedes remote and manifest", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "task-labelled-checkout")
		writeSnapshotFile(t, repo, "go.mod", "module example.com/owner/module\n\ngo 1.26\n")
		writeSnapshotFile(t, repo, "main.go", "package main\n\nfunc main() {}\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"manifest-name"}`)
		trackSnapshotFiles(t, repo, "go.mod", "main.go", "package.json")
		runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "git@github.com:owner/remote-name.git")

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "example.com/owner/module" {
			t.Fatalf("repo_name = %q, want root module path", got.RepoName)
		}
		if got.DisplayName != "task-labelled-checkout" {
			t.Fatalf("display_name = %q, want local checkout label", got.DisplayName)
		}
	})

	t.Run("normalized remote precedes manifest", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "another-task-label")
		writeSnapshotFile(t, repo, "README.md", "# Project\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"manifest-name"}`)
		trackSnapshotFiles(t, repo, "README.md", "package.json")
		runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "https://token@GitHub.com/owner/remote-name.git")

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "github.com/owner/remote-name" {
			t.Fatalf("repo_name = %q, want normalized remote identity", got.RepoName)
		}
	})

	t.Run("tracked manifest precedes neutral fallback", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "manifest-task-label")
		writeSnapshotFile(t, repo, "package.json", `{"name":"@example/manifest-project"}`)
		trackSnapshotFiles(t, repo, "package.json")

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != "@example/manifest-project" {
			t.Fatalf("repo_name = %q, want manifest identity", got.RepoName)
		}
	})

	t.Run("untracked manifest cannot replace neutral fallback", func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "untracked-task-label")
		writeSnapshotFile(t, repo, "README.md", "# Project\n")
		writeSnapshotFile(t, repo, "package.json", `{"name":"leaked-task-name"}`)
		trackSnapshotFiles(t, repo, "README.md")

		got, err := Build(Options{RepoPath: repo})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoName != neutralRepositoryName {
			t.Fatalf("repo_name = %q, want neutral fallback", got.RepoName)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "leaked-task-name") {
			t.Fatalf("snapshot included untracked manifest identity: %s", encoded)
		}
	})
}

func TestNormalizeRemoteIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		remote   string
		expected string
	}{
		{name: "https", remote: "https://github.com/go-fuego/fuego.git", expected: "github.com/go-fuego/fuego"},
		{name: "https credentials", remote: "https://secret@example.com/owner/repo.git", expected: "example.com/owner/repo"},
		{name: "ssh url", remote: "ssh://git@GitHub.com/go-fuego/fuego.git", expected: "github.com/go-fuego/fuego"},
		{name: "scp syntax", remote: "git@github.com:go-fuego/fuego.git", expected: "github.com/go-fuego/fuego"},
		{name: "local absolute path", remote: "/tmp/task-labelled-checkout", expected: ""},
		{name: "file url", remote: "file:///tmp/task-labelled-checkout", expected: ""},
		{name: "local relative path", remote: "../task-labelled-checkout", expected: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeRemoteIdentity(test.remote); got != test.expected {
				t.Fatalf("normalizeRemoteIdentity(%q) = %q, want %q", test.remote, got, test.expected)
			}
		})
	}
}

func TestBuildIdentityIgnoresAmbientGitConfigOverrides(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "ambient-config-checkout")
	writeSnapshotFile(t, repo, "README.md", "# Repository\n")
	trackSnapshotFiles(t, repo, "README.md")
	runSnapshotGit(t, "-C", repo, "remote", "add", "origin", "https://github.com/wanted/repository.git")

	ambientConfig := filepath.Join(t.TempDir(), "ambient.gitconfig")
	if err := os.WriteFile(
		ambientConfig,
		[]byte("[remote \"origin\"]\n\turl = https://example.invalid/ambient/repository.git\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG", ambientConfig)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "remote.origin.url")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://example.invalid/injected/repository.git")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'remote.origin.url'='https://example.invalid/parameters/repository.git'")
	t.Setenv("GIT_CONFIG_GLOBAL", ambientConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", ambientConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	got, err := Build(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoName != "github.com/wanted/repository" {
		t.Fatalf("repo_name = %q, want repository-local origin", got.RepoName)
	}
}

func TestRepositoryGitEnvironmentDropsAmbientConfigOverrides(t *testing.T) {
	t.Parallel()

	environment := []string{
		"PATH=/usr/bin", "GIT_DIR=/tmp/other.git", "GIT_CONFIG=/tmp/config",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=remote.origin.url",
		"GIT_CONFIG_VALUE_0=https://example.invalid/repository.git",
		"GIT_CONFIG_PARAMETERS='remote.origin.url'='https://example.invalid/parameters.git'",
		"GIT_CONFIG_SYSTEM=/tmp/system", "GIT_CONFIG_GLOBAL=/tmp/global", "GIT_CONFIG_NOSYSTEM=1",
	}
	joined := strings.Join(repositoryGitEnvironment(environment), "\n")
	for _, forbidden := range []string{
		"GIT_DIR=", "GIT_CONFIG=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_",
		"GIT_CONFIG_VALUE_", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_SYSTEM=",
		"GIT_CONFIG_GLOBAL=", "GIT_CONFIG_NOSYSTEM=",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("repository environment retained %q: %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("repository environment lost PATH: %q", joined)
	}
}

func TestParseRepositoryManifestName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parse    func([]byte) string
		manifest string
		expected string
	}{
		{name: "package json", parse: parseJSONManifestName, manifest: `{"name":"web-project"}`, expected: "web-project"},
		{name: "python project", parse: parsePythonManifestName, manifest: "[project]\nname = \"python-project\"\n", expected: "python-project"},
		{name: "poetry project", parse: parsePythonManifestName, manifest: "[tool.poetry]\nname = 'poetry-project'\n", expected: "poetry-project"},
		{name: "cargo package", parse: parseCargoManifestName, manifest: "[package]\nname = \"rust-project\"\n", expected: "rust-project"},
		{name: "setup metadata", parse: parseSetupConfigName, manifest: "[metadata]\nname = python-legacy\n", expected: "python-legacy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.parse([]byte(test.manifest)); got != test.expected {
				t.Fatalf("manifest name = %q, want %q", got, test.expected)
			}
		})
	}
}

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

func TestBuildKeepsTrackedSymlinkVisibleButNotAnalyzable(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "regular.go"), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular.go", filepath.Join(repo, "linked.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	trackSnapshotFiles(t, repo, "regular.go", "linked.go")

	got, err := Build(Options{RepoPath: repo, MaxTreeLines: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.FileTree, "linked.go") ||
		!slices.Contains(got.FileTree, "regular.go") {
		t.Fatalf("file tree = %#v, want visible regular and symlink paths", got.FileTree)
	}
	if !slices.Equal(got.FilteredFiles, []string{"regular.go"}) {
		t.Fatalf("analysis files = %#v, want only regular.go", got.FilteredFiles)
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

func writeSnapshotFile(t *testing.T, repo, relativePath, contents string) {
	t.Helper()

	filePath := filepath.Join(repo, relativePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runSnapshotGit(t *testing.T, args ...string) {
	t.Helper()

	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
