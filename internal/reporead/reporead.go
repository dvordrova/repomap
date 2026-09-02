// Package reporead provides confined reads of regular files contained in a
// resolved repository root.
package reporead

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	if maxBytes < 0 {
		return Content{}, fmt.Errorf("reporead: invalid byte limit %d", maxBytes)
	}

	file, err := r.openRegularFile(repoRelativePath)
	if err != nil {
		return Content{}, err
	}
	defer file.Close()

	if maxBytes == maxReadBytes {
		return readCompleteFile(file)
	}
	return readBoundedFile(file, maxBytes)
}

// ReadFileAll reads the complete current bytes of one confined regular file.
// It has no local size policy: operating-system or allocation failures are
// returned honestly to the caller.
func (r *Reader) ReadFileAll(repoRelativePath string) (Content, error) {
	if r == nil || r.root == nil {
		return Content{}, fmt.Errorf("reporead: reader is not initialized")
	}
	file, err := r.openRegularFile(repoRelativePath)
	if err != nil {
		return Content{}, err
	}
	defer file.Close()
	return readCompleteFile(file)
}

// ReadFileNoSymlinks is ReadFile with a stricter identity check: every path
// component must be non-symlink and the opened target must be the same regular
// file checked immediately before opening.
func (r *Reader) ReadFileNoSymlinks(repoRelativePath string, maxBytes int64) (Content, error) {
	if r == nil || r.root == nil {
		return Content{}, fmt.Errorf("reporead: reader is not initialized")
	}
	if maxBytes < 0 {
		return Content{}, fmt.Errorf("reporead: invalid byte limit %d", maxBytes)
	}

	file, err := r.openRegularFileNoSymlinks(repoRelativePath)
	if err != nil {
		return Content{}, err
	}
	defer file.Close()

	if maxBytes == maxReadBytes {
		return readCompleteFile(file)
	}
	return readBoundedFile(file, maxBytes)
}

// ReadFileNoSymlinksAll is the complete-read form of ReadFileNoSymlinks.
func (r *Reader) ReadFileNoSymlinksAll(repoRelativePath string) (Content, error) {
	if r == nil || r.root == nil {
		return Content{}, fmt.Errorf("reporead: reader is not initialized")
	}
	file, err := r.openRegularFileNoSymlinks(repoRelativePath)
	if err != nil {
		return Content{}, err
	}
	defer file.Close()
	return readCompleteFile(file)
}

func readCompleteFile(file *os.File) (Content, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return Content{}, fmt.Errorf("reporead: read file: %w", err)
	}
	return Content{Bytes: data[:len(data):len(data)]}, nil
}

func readBoundedFile(file *os.File, maxBytes int64) (Content, error) {
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

func (r *Reader) openRegularFile(repoRelativePath string) (*os.File, error) {
	cleanPath, err := cleanRelativePath(repoRelativePath)
	if err != nil {
		return nil, err
	}

	file, err := r.root.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("reporead: open file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("reporead: stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("reporead: path is not a regular file")
	}
	return file, nil
}

func (r *Reader) openRegularFileNoSymlinks(repoRelativePath string) (*os.File, error) {
	cleanPath, err := cleanRelativePath(repoRelativePath)
	if err != nil {
		return nil, err
	}

	var prefix string
	var checked os.FileInfo
	parts := strings.Split(cleanPath, string(filepath.Separator))
	for index, part := range parts {
		if prefix == "" {
			prefix = part
		} else {
			prefix = filepath.Join(prefix, part)
		}
		checked, err = r.root.Lstat(prefix)
		if err != nil {
			return nil, fmt.Errorf("reporead: lstat file: %w", err)
		}
		if checked.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("reporead: symbolic links are not allowed")
		}
		if index < len(parts)-1 && !checked.IsDir() {
			return nil, fmt.Errorf("reporead: path component is not a directory")
		}
	}
	if checked == nil || !checked.Mode().IsRegular() {
		return nil, fmt.Errorf("reporead: path is not a regular file")
	}

	file, err := r.root.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("reporead: open file: %w", err)
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("reporead: stat file: %w", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(checked, opened) {
		file.Close()
		return nil, fmt.Errorf("reporead: file changed while opening")
	}
	return file, nil
}

func cleanRelativePath(repoRelativePath string) (string, error) {
	localPath := filepath.FromSlash(repoRelativePath)
	if repoRelativePath == "" || filepath.IsAbs(localPath) || !filepath.IsLocal(localPath) {
		return "", fmt.Errorf("reporead: invalid repository-relative path %q", repoRelativePath)
	}
	cleanPath := filepath.Clean(localPath)
	if cleanPath == "." || cleanPath != localPath {
		return "", fmt.Errorf("reporead: path must be a clean repository-relative file")
	}
	return cleanPath, nil
}
