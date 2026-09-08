package freshness

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// UnavailableSourcePaths returns every corpus path without a source link at the
// captured revision: absent from HEAD or changed in the captured working tree.
// Paths are analysis-root relative. This does not remove any corpus entries.
func UnavailableSourcePaths(ctx context.Context, analysisRoot string, repository *corpus.Corpus, state RepositoryState) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, fmt.Errorf("freshness: repository corpus is required")
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	root, err := canonicalRoot(analysisRoot)
	if err != nil {
		return nil, err
	}
	prefix, err := filepath.Rel(state.Identity, root)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("freshness: analysis root is outside the captured repository")
	}
	paths, err := gitOutput(ctx, state.Identity, "ls-tree", "-r", "-z", "--name-only", "--full-tree", state.Head)
	if err != nil {
		return nil, err
	}
	committed := make(map[string]bool)
	for _, path := range bytes.Split(paths, []byte{0}) {
		committed[string(path)] = true
	}
	changed := make(map[string]bool, len(state.Dirty))
	for _, file := range state.Dirty {
		changed[file.Path] = true
		if file.FromPath != "" {
			changed[file.FromPath] = true
		}
	}
	unavailable := make([]string, 0)
	for _, entry := range repository.Snapshot().Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(entry.Path)))
		if !committed[path] || changed[path] {
			unavailable = append(unavailable, entry.Path)
		}
	}
	sort.Strings(unavailable)
	return unavailable, nil
}
