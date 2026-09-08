package run

import (
	"context"
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/freshness"
)

func validateRepositorySourceLinks(ctx context.Context, host string, repository *corpus.Corpus, analysisRoot string, state freshness.RepositoryState) error {
	if host == "" {
		return nil
	}
	if repositoryCorpusHasWorkingTreeChanges(repository, analysisRoot, state) {
		return fmt.Errorf("--no-serve cannot create exact %s source links for tracked working-tree changes; commit or stash those changes, or remove --no-serve to open code through the local VS Code server", host)
	}
	if repositoryStateHasAnalyzedSubmodule(state) {
		return fmt.Errorf("standalone %s reports do not support analyzed submodule source because one repository URL cannot address it", host)
	}
	missing, err := freshness.CorpusPathOutsideRevision(ctx, analysisRoot, repository, state)
	if err != nil {
		return fmt.Errorf("verify standalone %s source paths: %w", host, err)
	}
	if missing != "" {
		return fmt.Errorf("standalone %s report cannot link source %q: it is absent from captured revision %s; commit it, exclude it from analysis, or use local serving without --no-serve/--github-url/--gitlab-url", host, missing, state.Head)
	}
	return nil
}
