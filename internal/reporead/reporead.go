// Package reporead provides bounded reads of regular files contained in a
// resolved repository root.
package reporead

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxReadBytes = int64(^uint64(0) >> 1)

// Reader confines file reads to one resolved repository root.
type Reader struct {
	root *os.Root
}

// Content is a bounded file prefix. Truncated reports whether more bytes were
// available than the requested limit.
type Content struct {
	Bytes     []byte
	Truncated bool
}

// New resolves repoPath and prepares a repository-confined reader.
func New(repoPath string) (*Reader, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("reporead: repository path is required")
	}

	absolute, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("reporead: resolve repository path: %w", err)
	}
	openedRoot, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("reporead: open repository root: %w", err)
	}

	return &Reader{root: openedRoot}, nil
}

// Close releases the repository root opened by New.
func (r *Reader) Close() error {
	if r == nil || r.root == nil {
		return nil
	}
	if err := r.root.Close(); err != nil {
		return fmt.Errorf("reporead: close repository root: %w", err)
	}
	return nil
}

// ReadFile reads at most maxBytes from a repository-relative regular file.
// Absolute paths, traversal, and symlinks resolving outside the repository are
// rejected.
func (r *Reader) ReadFile(repoRelativePath string, maxBytes int64) (Content, error) {
	if r == nil || r.root == nil {
		return Content{}, fmt.Errorf("reporead: reader is not initialized")
	}
	if maxBytes < 0 || maxBytes == maxReadBytes {
		return Content{}, fmt.Errorf("reporead: invalid byte limit %d", maxBytes)
	}

	localPath := filepath.FromSlash(repoRelativePath)
	if repoRelativePath == "" || filepath.IsAbs(localPath) || !filepath.IsLocal(localPath) {
		return Content{}, fmt.Errorf("reporead: invalid repository-relative path %q", repoRelativePath)
	}
	cleanPath := filepath.Clean(localPath)
	if cleanPath == "." || cleanPath != localPath {
		return Content{}, fmt.Errorf("reporead: path must be a clean repository-relative file")
	}

	file, err := r.root.Open(cleanPath)
	if err != nil {
		return Content{}, fmt.Errorf("reporead: open file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Content{}, fmt.Errorf("reporead: stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Content{}, fmt.Errorf("reporead: path is not a regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return Content{}, fmt.Errorf("reporead: read file: %w", err)
	}
	if int64(len(data)) <= maxBytes {
		return Content{Bytes: data[:len(data):len(data)]}, nil
	}

	limit := int(maxBytes)
	return Content{
		Bytes:     data[:limit:limit],
		Truncated: true,
	}, nil
}
