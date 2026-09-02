package readmetargetscout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

// FileTree is a lossless prefix-compressed FileID/path dictionary. JSON object
// keys are repository path components. A string leaf is the exact FileID of
// the file named by that key; an object value is the child directory. Joining
// keys from the root to a leaf restores the complete repository-relative path.
type FileTree map[string]FileTreeEntry

// FileTreeEntry is exactly one file leaf or one non-empty directory. Its JSON
// union is deliberately compact and readable: "f31" for a file, or a nested
// object for a directory.
type FileTreeEntry struct {
	FileRef   corpus.FileID
	Directory FileTree
}

func (entry FileTreeEntry) MarshalJSON() ([]byte, error) {
	switch {
	case entry.FileRef != "" && entry.Directory == nil:
		return json.Marshal(entry.FileRef)
	case entry.FileRef == "" && len(entry.Directory) > 0:
		return json.Marshal(entry.Directory)
	default:
		return nil, fmt.Errorf("README file tree: entry must be exactly one file or non-empty directory")
	}
}

func (entry *FileTreeEntry) UnmarshalJSON(raw []byte) error {
	if entry == nil {
		return fmt.Errorf("README file tree: nil entry")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("README file tree: empty entry")
	}
	switch trimmed[0] {
	case '"':
		var fileRef corpus.FileID
		if err := json.Unmarshal(trimmed, &fileRef); err != nil || fileRef == "" {
			return fmt.Errorf("README file tree: invalid file leaf")
		}
		*entry = FileTreeEntry{FileRef: fileRef}
		return nil
	case '{':
		var directory FileTree
		if err := json.Unmarshal(trimmed, &directory); err != nil || len(directory) == 0 {
			return fmt.Errorf("README file tree: invalid directory entry")
		}
		*entry = FileTreeEntry{Directory: directory}
		return nil
	default:
		return fmt.Errorf("README file tree: entry must be a FileID string or directory object")
	}
}

func buildFileTree(entries []corpus.Entry) (FileTree, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("README file tree: invalid corpus entry count")
	}
	tree := make(FileTree)
	for _, entry := range entries {
		components := strings.Split(entry.Path, "/")
		if len(components) == 0 {
			return nil, fmt.Errorf("README file tree: path %q has no components", entry.Path)
		}
		directory := tree
		for _, component := range components[:len(components)-1] {
			if err := validateFileTreeComponent(component); err != nil {
				return nil, fmt.Errorf("README file tree: path %q: %w", entry.Path, err)
			}
			child, found := directory[component]
			if found && child.FileRef != "" {
				return nil, fmt.Errorf("README file tree: file/directory collision at %q", component)
			}
			if !found {
				child = FileTreeEntry{Directory: make(FileTree)}
				directory[component] = child
			}
			directory = child.Directory
		}
		base := components[len(components)-1]
		if err := validateFileTreeComponent(base); err != nil {
			return nil, fmt.Errorf("README file tree: path %q: %w", entry.Path, err)
		}
		if _, duplicate := directory[base]; duplicate {
			return nil, fmt.Errorf("README file tree: duplicate path %q", entry.Path)
		}
		directory[base] = FileTreeEntry{FileRef: entry.ID}
	}
	return tree, nil
}

func fileTreeDictionary(tree FileTree, fileCount int) (map[corpus.FileID]string, error) {
	if len(tree) == 0 || fileCount < 1 {
		return nil, fmt.Errorf("README file tree: invalid root or file count")
	}
	result := make(map[corpus.FileID]string, fileCount)
	paths := make(map[string]struct{}, fileCount)
	if err := walkFileTree(tree, "", result, paths); err != nil {
		return nil, err
	}
	if len(result) != fileCount || len(paths) != fileCount {
		return nil, fmt.Errorf(
			"README file tree: restored %d files, declared %d",
			len(result), fileCount,
		)
	}
	return result, nil
}

func walkFileTree(
	tree FileTree,
	parent string,
	result map[corpus.FileID]string,
	paths map[string]struct{},
) error {
	if len(tree) == 0 {
		return fmt.Errorf("README file tree: empty directory")
	}
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := validateFileTreeComponent(name); err != nil {
			return err
		}
		filePath := name
		if parent != "" {
			filePath = parent + "/" + name
		}
		entry := tree[name]
		switch {
		case entry.FileRef != "" && entry.Directory == nil:
			if validateRepoPath(filePath) != nil {
				return fmt.Errorf("README file tree: invalid restored path %q", filePath)
			}
			if _, duplicate := result[entry.FileRef]; duplicate {
				return fmt.Errorf("README file tree: duplicate file_ref %q", entry.FileRef)
			}
			if _, duplicate := paths[filePath]; duplicate {
				return fmt.Errorf("README file tree: duplicate path %q", filePath)
			}
			result[entry.FileRef] = filePath
			paths[filePath] = struct{}{}
		case entry.FileRef == "" && len(entry.Directory) > 0:
			if err := walkFileTree(entry.Directory, filePath, result, paths); err != nil {
				return err
			}
		default:
			return fmt.Errorf("README file tree: invalid entry at %q", filePath)
		}
	}
	return nil
}

func validateFileTreeComponent(value string) error {
	if value == "" || value == "." || value == ".." || !utf8.ValidString(value) ||
		containsControl(value) || strings.ContainsAny(value, `/\`) || path.Clean(value) != value {
		return fmt.Errorf("invalid path component %q", value)
	}
	return nil
}

func cloneFileTree(source FileTree) FileTree {
	if source == nil {
		return nil
	}
	result := make(FileTree, len(source))
	for name, entry := range source {
		result[name] = FileTreeEntry{
			FileRef: entry.FileRef, Directory: cloneFileTree(entry.Directory),
		}
	}
	return result
}
