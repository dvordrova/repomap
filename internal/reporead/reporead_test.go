package reporead

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReaderReadFileBoundsContent(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})

	tests := []struct {
		name              string
		limit             int64
		expected          string
		expectedTruncated bool
	}{
		{name: "prefix", limit: 3, expected: "abc", expectedTruncated: true},
		{name: "exact bound", limit: 6, expected: "abcdef", expectedTruncated: false},
		{name: "larger bound", limit: 10, expected: "abcdef", expectedTruncated: false},
		{name: "zero bound", limit: 0, expected: "", expectedTruncated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			content, err := reader.ReadFile("README.md", test.limit)
			if err != nil {
				t.Fatal(err)
			}
			if string(content.Bytes) != test.expected {
				t.Fatalf("bytes = %q, want %q", content.Bytes, test.expected)
			}
			if content.Truncated != test.expectedTruncated {
				t.Fatalf("truncated = %v, want %v", content.Truncated, test.expectedTruncated)
			}
			if cap(content.Bytes) != len(content.Bytes) {
				t.Fatalf("byte capacity = %d, want %d", cap(content.Bytes), len(content.Bytes))
			}
		})
	}
}

func TestReaderReadFileRejectsUnsafeOrNonRegularPaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("readme"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})

	tests := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "root", path: "."},
		{name: "absolute", path: filepath.Join(repo, "README.md")},
		{name: "traversal", path: "../outside"},
		{name: "nested traversal", path: "dir/../../outside"},
		{name: "contained traversal", path: "dir/../README.md"},
		{name: "dot prefix", path: "./README.md"},
		{name: "directory", path: "dir"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := reader.ReadFile(test.path, 32); err == nil {
				t.Fatal("ReadFile() error = nil")
			}
		})
	}
}

func TestReaderReadFileRejectsSymlinkEscapeAndAllowsContainedSymlink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outsideDir := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(repo, "inside.go")
	if err := os.WriteFile(inside, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideDir, "outside.go")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside.go", filepath.Join(repo, "contained.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("../outside/outside.go", filepath.Join(repo, "escape.go")); err != nil {
		t.Fatal(err)
	}

	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	content, err := reader.ReadFile("contained.go", 32)
	if err != nil {
		t.Fatalf("contained symlink: %v", err)
	}
	if string(content.Bytes) != "inside" {
		t.Fatalf("contained symlink bytes = %q", content.Bytes)
	}
	if _, err := reader.ReadFile("escape.go", 32); err == nil {
		t.Fatal("ReadFile() accepted symlink escape")
	}
}

func TestNewRejectsFileAsRepositoryRoot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "repo")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("New() error = nil")
	}
}
