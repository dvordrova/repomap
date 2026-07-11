package freshness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxIgnoredBuildInputs = 10_000

// CaptureRepository records the exact checked-out commit plus the content of
// every non-ignored dirty path visible to the working tree. It never stores
// file contents and never follows a symlink outside the repository.
func CaptureRepository(ctx context.Context, path string) (RepositoryState, error) {
	rootOutput, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepositoryState{}, err
	}
	root, err := canonicalRoot(strings.TrimSpace(string(rootOutput)))
	if err != nil {
		return RepositoryState{}, err
	}
	var previous RepositoryState
	for attempt := 0; attempt < 3; attempt++ {
		current, err := captureRepositoryOnce(ctx, root)
		if err != nil {
			return RepositoryState{}, err
		}
		if attempt > 0 {
			previousDigest, err := previous.Digest()
			if err != nil {
				return RepositoryState{}, err
			}
			currentDigest, err := current.Digest()
			if err != nil {
				return RepositoryState{}, err
			}
			if previousDigest == currentDigest {
				return current, nil
			}
		}
		previous = current
	}
	return RepositoryState{}, fmt.Errorf("freshness: repository changed while its state was being captured")
}

func captureRepositoryOnce(ctx context.Context, root string) (RepositoryState, error) {
	headOutput, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return RepositoryState{}, err
	}
	statusOutput, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=no")
	if err != nil {
		return RepositoryState{}, err
	}
	entries, err := parseStatus(statusOutput)
	if err != nil {
		return RepositoryState{}, fmt.Errorf("freshness: parse git status: %w", err)
	}
	ignoredOutput, err := gitOutput(ctx, root, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return RepositoryState{}, err
	}
	ignored, err := parseIgnoredBuildInputs(ignoredOutput)
	if err != nil {
		return RepositoryState{}, fmt.Errorf("freshness: parse ignored build inputs: %w", err)
	}
	entries = append(entries, ignored...)
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return RepositoryState{}, fmt.Errorf("freshness: open repository root: %w", err)
	}
	defer rootHandle.Close()

	dirty := make([]DirtyFile, 0, len(entries))
	for _, entry := range entries {
		file, err := fingerprintDirtyFile(rootHandle, entry)
		if err != nil {
			return RepositoryState{}, err
		}
		dirty = append(dirty, file)
	}
	sort.Slice(dirty, func(i, j int) bool { return dirtyFileKey(dirty[i]) < dirtyFileKey(dirty[j]) })
	state := RepositoryState{
		Version:  RepositoryStateVersion,
		Identity: root,
		Head:     strings.TrimSpace(string(headOutput)),
		Dirty:    dirty,
	}
	if err := state.Validate(); err != nil {
		return RepositoryState{}, err
	}
	return state, nil
}

type statusEntry struct {
	xy       string
	path     string
	fromPath string
}

func parseStatus(data []byte) ([]statusEntry, error) {
	var entries []statusEntry
	for len(data) > 0 {
		end := bytes.IndexByte(data, 0)
		if end < 0 {
			return nil, fmt.Errorf("unterminated status record")
		}
		record := data[:end]
		data = data[end+1:]
		if len(record) < 4 || record[2] != ' ' || !utf8.Valid(record[3:]) {
			return nil, fmt.Errorf("invalid status record")
		}
		entry := statusEntry{xy: string(record[:2]), path: string(record[3:])}
		if renameOrCopy(entry.xy) {
			end = bytes.IndexByte(data, 0)
			if end < 0 || !utf8.Valid(data[:end]) {
				return nil, fmt.Errorf("invalid rename source")
			}
			entry.fromPath = string(data[:end])
			data = data[end+1:]
		}
		entry.path, _ = normalizeGitPath(entry.path)
		if entry.path == "" {
			return nil, fmt.Errorf("invalid path in status record")
		}
		if entry.fromPath != "" {
			entry.fromPath, _ = normalizeGitPath(entry.fromPath)
			if entry.fromPath == "" {
				return nil, fmt.Errorf("invalid rename source path")
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseIgnoredBuildInputs(data []byte) ([]statusEntry, error) {
	var entries []statusEntry
	for len(data) > 0 {
		end := bytes.IndexByte(data, 0)
		if end < 0 {
			return nil, fmt.Errorf("unterminated ignored-path record")
		}
		raw := data[:end]
		data = data[end+1:]
		if !utf8.Valid(raw) {
			return nil, fmt.Errorf("invalid UTF-8 ignored path")
		}
		path, err := normalizeGitPath(string(raw))
		if err != nil {
			return nil, err
		}
		if !isGoBuildInput(path) {
			continue
		}
		entries = append(entries, statusEntry{xy: "!!", path: path})
		if len(entries) > maxIgnoredBuildInputs {
			return nil, fmt.Errorf("more than %d ignored Go build inputs", maxIgnoredBuildInputs)
		}
	}
	return entries, nil
}

func fingerprintDirtyFile(root *os.Root, entry statusEntry) (DirtyFile, error) {
	file := DirtyFile{
		Status:   semanticStatus(entry.xy),
		Path:     entry.path,
		FromPath: entry.fromPath,
	}
	info, err := root.Lstat(filepath.FromSlash(entry.path))
	if os.IsNotExist(err) {
		file.Kind = FileMissing
		return file, nil
	}
	if err != nil {
		return DirtyFile{}, fmt.Errorf("freshness: inspect dirty path %q: %w", entry.path, err)
	}
	switch {
	case info.Mode().IsRegular():
		file.Kind = FileRegular
		handle, err := root.Open(filepath.FromSlash(entry.path))
		if err != nil {
			return DirtyFile{}, fmt.Errorf("freshness: open dirty path %q: %w", entry.path, err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, handle)
		closeErr := handle.Close()
		if copyErr != nil {
			return DirtyFile{}, fmt.Errorf("freshness: hash dirty path %q: %w", entry.path, copyErr)
		}
		if closeErr != nil {
			return DirtyFile{}, fmt.Errorf("freshness: close dirty path %q: %w", entry.path, closeErr)
		}
		file.ContentSHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	case info.Mode()&os.ModeSymlink != 0:
		file.Kind = FileSymlink
		target, err := root.Readlink(filepath.FromSlash(entry.path))
		if err != nil {
			return DirtyFile{}, fmt.Errorf("freshness: read dirty symlink %q: %w", entry.path, err)
		}
		file.ContentSHA256 = sha256Hex([]byte(target))
	case info.IsDir():
		file.Kind = FileDirectory
		return DirtyFile{}, fmt.Errorf("freshness: dirty path %q is a directory; submodule freshness is not supported yet", entry.path)
	default:
		return DirtyFile{}, fmt.Errorf("freshness: dirty path %q has unsupported file type %s", entry.path, info.Mode())
	}
	return file, nil
}

func canonicalRoot(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("freshness: resolve repository root: %w", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("freshness: resolve repository root: %w", err)
	}
	return filepath.Clean(absolute), nil
}

func normalizeGitPath(path string) (string, error) {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if err := validateRelativePath(path); err != nil {
		return "", err
	}
	return path, nil
}

func renameOrCopy(xy string) bool {
	return len(xy) == 2 && (xy[0] == 'R' || xy[1] == 'R' || xy[0] == 'C' || xy[1] == 'C')
}

func semanticStatus(xy string) string {
	if xy == "!!" {
		return "ignored"
	}
	if xy == "??" {
		return "untracked"
	}
	if strings.ContainsRune(xy, 'U') || xy == "AA" || xy == "DD" {
		return "conflicted"
	}
	for _, candidate := range []struct {
		code   byte
		status string
	}{
		{code: 'R', status: "renamed"},
		{code: 'C', status: "copied"},
		{code: 'D', status: "deleted"},
		{code: 'T', status: "type_changed"},
		{code: 'A', status: "added"},
		{code: 'M', status: "modified"},
	} {
		if strings.IndexByte(xy, candidate.code) >= 0 {
			return candidate.status
		}
	}
	return "modified"
}

func isGoBuildInput(path string) bool {
	base := filepath.Base(path)
	switch base {
	case "go.mod", "go.sum", "go.work", "go.work.sum":
		return true
	}
	if filepath.ToSlash(path) == "vendor/modules.txt" || strings.HasSuffix(filepath.ToSlash(path), "/vendor/modules.txt") {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".go", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".s", ".syso":
		return true
	default:
		return false
	}
}

func gitOutput(ctx context.Context, path string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", path}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("freshness: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
