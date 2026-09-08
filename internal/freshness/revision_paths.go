package freshness

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// CorpusPathOutsideRevision returns the first readable corpus path absent from
// the captured revision. This standalone-link check does not change the corpus
// or the working-tree state used by local reports.
func CorpusPathOutsideRevision(ctx context.Context, analysisRoot string, repository *corpus.Corpus, state RepositoryState) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if repository == nil {
		return "", fmt.Errorf("freshness: repository corpus is required")
	}
	if err := state.Validate(); err != nil {
		return "", err
	}
	root, err := canonicalRoot(analysisRoot)
	if err != nil {
		return "", err
	}
	prefix, err := filepath.Rel(state.Identity, root)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("freshness: analysis root is outside the captured repository")
	}
	paths, err := gitOutput(ctx, state.Identity, "ls-tree", "-r", "-z", "--name-only", "--full-tree", state.Head)
	if err != nil {
		return "", err
	}
	committed := make(map[string]bool)
	for _, path := range bytes.Split(paths, []byte{0}) {
		committed[string(path)] = true
	}
	for _, entry := range repository.Snapshot().Entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		path := filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(entry.Path)))
		if !committed[path] {
			return entry.Path, nil
		}
	}
	return "", nil
}
