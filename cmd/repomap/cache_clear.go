package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var persistentCacheDirectories = []string{
	".model-research",
	architectureSynthesisCacheDirectory,
	presentationLocalizationCacheDir,
}

func runCache(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "clear" {
		return fmt.Errorf("usage: repomap cache clear [--debug-dir <dir>]")
	}
	fs := flag.NewFlagSet("repomap cache clear", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	debugDir := fs.String(
		"debug-dir",
		defaultDebugDir(),
		"debug artifact root containing persistent caches",
	)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: repomap cache clear [--debug-dir <dir>]")
	}
	return clearPersistentCaches(*debugDir, stdout)
}

func clearPersistentCaches(debugDir string, stdout io.Writer) error {
	if strings.TrimSpace(debugDir) == "" {
		return fmt.Errorf("cache clear: debug directory is required")
	}
	root, err := filepath.Abs(debugDir)
	if err != nil {
		return fmt.Errorf("cache clear: resolve debug directory: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if os.IsNotExist(err) {
		fmt.Fprintf(stdout, "repomap: persistent cache already clear under %s\n", root)
		return nil
	}
	if err != nil {
		return fmt.Errorf("cache clear: inspect debug directory: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("cache clear: debug directory must be a real directory")
	}

	type target struct {
		path string
		info os.FileInfo
	}
	targets := make([]target, 0, len(persistentCacheDirectories))
	for _, name := range persistentCacheDirectories {
		path := filepath.Join(root, name)
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil || relative != name {
			return fmt.Errorf("cache clear: invalid cache target")
		}
		info, statErr := os.Lstat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("cache clear: inspect %s: %w", name, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("cache clear: %s must be a real directory", name)
		}
		targets = append(targets, target{path: path, info: info})
	}

	for _, target := range targets {
		current, statErr := os.Lstat(target.path)
		if statErr != nil || current.Mode()&os.ModeSymlink != 0 ||
			!current.IsDir() || !os.SameFile(target.info, current) {
			return fmt.Errorf("cache clear: cache target changed before removal")
		}
		if err := os.RemoveAll(target.path); err != nil {
			return fmt.Errorf("cache clear: remove %s: %w", filepath.Base(target.path), err)
		}
	}
	fmt.Fprintf(
		stdout,
		"repomap: cleared %d persistent cache directorie(s) under %s\n",
		len(targets),
		root,
	)
	return nil
}
