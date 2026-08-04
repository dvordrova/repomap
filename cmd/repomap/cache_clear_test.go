package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearPersistentCachesRemovesOnlyKnownCacheDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range persistentCacheDirectories {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "entry.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runPath := filepath.Join(root, "20260731-fixture")
	if err := os.Mkdir(runPath, 0o700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := clearPersistentCaches(root, &output); err != nil {
		t.Fatal(err)
	}
	for _, name := range persistentCacheDirectories {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("cache %q remains: %v", name, err)
		}
	}
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("cache clear removed run artifact directory: %v", err)
	}
	if !strings.Contains(output.String(), "cleared 3 persistent cache") {
		t.Fatalf("cache clear output = %q", output.String())
	}
}

func TestClearPersistentCachesRejectsSymlinkWithoutPartialRemoval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	first := filepath.Join(root, persistentCacheDirectories[0])
	if err := os.Mkdir(first, 0o700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	unsafe := filepath.Join(root, persistentCacheDirectories[1])
	if err := os.Symlink(target, unsafe); err != nil {
		t.Fatal(err)
	}
	if err := clearPersistentCaches(root, io.Discard); err == nil {
		t.Fatal("cache clear accepted a symlink target")
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("cache clear partially removed a validated target: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("cache clear followed the symlink target: %v", err)
	}
}
