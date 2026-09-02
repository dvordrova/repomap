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
	reader := newTestReader(t, repo)

	tests := []struct {
		name      string
		limit     int64
		want      string
		truncated bool
	}{
		{name: "prefix", limit: 3, want: "abc", truncated: true},
		{name: "exact", limit: 6, want: "abcdef"},
		{name: "larger", limit: 10, want: "abcdef"},
		{name: "zero", limit: 0, want: "", truncated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := reader.ReadFile("README.md", test.limit)
			if err != nil {
				t.Fatal(err)
			}
			if string(content.Bytes) != test.want || content.Truncated != test.truncated {
				t.Fatalf("ReadFile() = %q, truncated %v; want %q, %v", content.Bytes, content.Truncated, test.want, test.truncated)
			}
			if cap(content.Bytes) != len(content.Bytes) {
				t.Fatalf("byte capacity = %d, want %d", cap(content.Bytes), len(content.Bytes))
			}
		})
	}
	complete, err := reader.ReadFileAll("README.md")
	if err != nil || string(complete.Bytes) != "abcdef" || complete.Truncated {
		t.Fatalf("ReadFileAll() = %#v, %v", complete, err)
	}
	complete, err = reader.ReadFile("README.md", maxReadBytes)
	if err != nil || string(complete.Bytes) != "abcdef" || complete.Truncated {
		t.Fatalf("ReadFile(max int) = %#v, %v", complete, err)
	}
}

func TestReaderRejectsUnsafePathsAndSymlinkEscape(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "inside.go"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "outside.go"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside.go", filepath.Join(repo, "contained.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("../outside.go", filepath.Join(repo, "escape.go")); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)

	content, err := reader.ReadFile("contained.go", 32)
	if err != nil || string(content.Bytes) != "inside" {
		t.Fatalf("contained symlink read = %q, %v", content.Bytes, err)
	}
	if _, err := reader.ReadFile("escape.go", 32); err == nil {
		t.Fatal("ReadFile() accepted symlink escape")
	}
	if _, err := reader.ReadFileNoSymlinks("contained.go", 32); err == nil {
		t.Fatal("ReadFileNoSymlinks() accepted a contained symlink")
	}
	for _, unsafePath := range []string{"", ".", "../outside.go", "./inside.go", repo} {
		if _, err := reader.ReadFile(unsafePath, 32); err == nil {
			t.Errorf("ReadFile(%q) error = nil", unsafePath)
		}
	}
}

func TestNewRejectsFileAsRepositoryRoot(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "repo")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(file); err == nil {
		t.Fatal("New() error = nil")
	}
}

func newTestReader(t *testing.T, repo string) *Reader {
	t.Helper()

	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	return reader
}
