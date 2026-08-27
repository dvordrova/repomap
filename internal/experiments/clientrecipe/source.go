package clientrecipe

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepositoryFiles returns the complete regular-file view available to the
// experiment extractor. Its only filesystem authority is the supplied repo
// root; the adjacent oracle and experiment expectations are never inputs.
func RepositoryFiles(repoRoot string) ([]string, error) {
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("client recipe source: resolve repository root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("client recipe source: repository root is not a directory")
	}
	files := make([]string, 0)
	err = filepath.WalkDir(absoluteRoot, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("client recipe source: symlink is not allowed: %s", entry.Name())
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("client recipe source: non-regular file is not allowed: %s", entry.Name())
		}
		relative, err := filepath.Rel(absoluteRoot, filename)
		if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("client recipe source: file escaped repository root")
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
