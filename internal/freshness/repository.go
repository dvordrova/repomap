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

	"github.com/dvordrova/repomap/internal/corpus"
)

// CaptureRepository records the checked-out commit plus exact content
// identities for tracked working-tree changes. It consumes submodule index
// authority from the already-built corpus and never inventories tracked paths
// again. Untracked and ignored paths are outside the shared repository corpus
// and therefore outside this authority.
func CaptureRepository(
	ctx context.Context,
	path string,
	repositoryCorpus *corpus.Corpus,
) (RepositoryState, error) {
	if repositoryCorpus == nil {
		return RepositoryState{}, fmt.Errorf("freshness: repository corpus is required")
	}
	if err := repositoryCorpus.Snapshot().Validate(); err != nil {
		return RepositoryState{}, fmt.Errorf("freshness: repository corpus: %w", err)
	}
	rootOutput, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepositoryState{}, err
	}
	root, err := canonicalRoot(strings.TrimSpace(string(rootOutput)))
	if err != nil {
		return RepositoryState{}, err
	}
	return captureRepositoryOnce(ctx, root, repositoryCorpus.Gitlinks())
}

func captureRepositoryOnce(
	ctx context.Context,
	root string,
	indexGitlinks []corpus.Gitlink,
) (RepositoryState, error) {
	headOutput, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return RepositoryState{}, err
	}
	statusOutput, err := gitOutput(ctx, root, "status", "--porcelain=v2", "-z", "--untracked-files=no", "--ignored=no", "--ignore-submodules=none")
	if err != nil {
		return RepositoryState{}, err
	}
	entries, err := parseStatus(statusOutput)
	if err != nil {
		return RepositoryState{}, fmt.Errorf("freshness: parse git status: %w", err)
	}
	gitlinks, err := captureSubmodules(ctx, root, entries, indexGitlinks)
	if err != nil {
		return RepositoryState{}, err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return RepositoryState{}, fmt.Errorf("freshness: open repository root: %w", err)
	}
	defer rootHandle.Close()

	dirty := make([]DirtyFile, 0, len(entries))
	for _, entry := range entries {
		if entry.submodule != "" {
			continue
		}
		file, err := fingerprintDirtyFile(rootHandle, entry)
		if err != nil {
			return RepositoryState{}, err
		}
		dirty = append(dirty, file)
	}
	sort.Slice(dirty, func(i, j int) bool { return dirtyFileKey(dirty[i]) < dirtyFileKey(dirty[j]) })
	state := RepositoryState{
		Version:    RepositoryStateVersion,
		Identity:   root,
		Head:       strings.TrimSpace(string(headOutput)),
		Dirty:      dirty,
		Submodules: gitlinks,
	}
	if err := state.Validate(); err != nil {
		return RepositoryState{}, err
	}
	return state, nil
}

type statusEntry struct {
	xy              string
	path            string
	fromPath        string
	submodule       string
	recordedGitlink string
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
		if len(record) < 3 || !utf8.Valid(record) {
			return nil, fmt.Errorf("invalid status record")
		}
		var entry statusEntry
		switch record[0] {
		case '1':
			fields := strings.SplitN(string(record), " ", 9)
			if len(fields) != 9 {
				return nil, fmt.Errorf("invalid ordinary status record")
			}
			entry = statusEntry{xy: fields[1], path: fields[8]}
			if strings.HasPrefix(fields[2], "S") {
				entry.submodule = fields[2]
				entry.recordedGitlink = fields[7]
			}
		case '2':
			fields := strings.SplitN(string(record), " ", 10)
			if len(fields) != 10 {
				return nil, fmt.Errorf("invalid rename status record")
			}
			entry = statusEntry{xy: fields[1], path: fields[9]}
			if strings.HasPrefix(fields[2], "S") {
				entry.submodule = fields[2]
				entry.recordedGitlink = fields[7]
			}
			end = bytes.IndexByte(data, 0)
			if end < 0 || !utf8.Valid(data[:end]) {
				return nil, fmt.Errorf("invalid rename source")
			}
			entry.fromPath = string(data[:end])
			data = data[end+1:]
		case 'u':
			fields := strings.SplitN(string(record), " ", 11)
			if len(fields) != 11 {
				return nil, fmt.Errorf("invalid unmerged status record")
			}
			entry = statusEntry{xy: fields[1], path: fields[10]}
		default:
			return nil, fmt.Errorf("unsupported porcelain-v2 status record")
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

func captureSubmodules(
	ctx context.Context,
	root string,
	entries []statusEntry,
	indexGitlinks []corpus.Gitlink,
) ([]SubmoduleState, error) {
	states := make(map[string]SubmoduleState)
	for _, gitlink := range indexGitlinks {
		states[gitlink.Path] = SubmoduleState{
			Path: gitlink.Path, RecordedGitlink: gitlink.RecordedObjectID, Availability: SubmoduleUnavailable,
		}
	}
	for _, entry := range entries {
		if entry.submodule == "" {
			continue
		}
		state, exists := states[entry.path]
		if !exists {
			state = SubmoduleState{Path: entry.path, RecordedGitlink: entry.recordedGitlink, Availability: SubmoduleUnavailable}
		}
		if len(entry.submodule) == 4 {
			state.GitlinkChanged = entry.submodule[1] != '.'
			state.WorktreeModified = entry.submodule[2] != '.'
			state.WorktreeUntracked = entry.submodule[3] != '.'
		}
		states[entry.path] = state
	}
	result := make([]SubmoduleState, 0, len(states))
	for path, state := range states {
		head, err := gitOutput(ctx, filepath.Join(root, filepath.FromSlash(path)), "rev-parse", "--verify", "HEAD")
		if err == nil {
			state.CurrentHead = strings.TrimSpace(string(head))
			state.Availability = SubmoduleClean
			state.GitlinkChanged = state.GitlinkChanged || state.CurrentHead != state.RecordedGitlink
		}
		result = append(result, state)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
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
		file.Mode = fmt.Sprintf("%04o", info.Mode().Perm())
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
		file.Mode = "120000"
		target, err := os.Readlink(filepath.Join(root.Name(), filepath.FromSlash(entry.path)))
		if err != nil {
			return DirtyFile{}, fmt.Errorf("freshness: read dirty symlink %q: %w", entry.path, err)
		}
		file.ContentSHA256 = sha256Hex([]byte(target))
	case info.IsDir():
		return DirtyFile{}, fmt.Errorf("freshness: tracked path %q unexpectedly resolves to a directory", entry.path)
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

func semanticStatus(xy string) string {
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
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", path,
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	command.Env = isolatedGitEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("freshness: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func isolatedGitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+4)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_SYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
			"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER":
			continue
		}
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
}
