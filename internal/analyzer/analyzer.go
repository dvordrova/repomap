// Package analyzer defines the language-neutral port implemented by local
// repository analyzers. Language and transport details belong in adapters.
package analyzer

import (
	"context"

	"github.com/dvordrova/repomap/internal/evidence"
)

type Request struct {
	RepoPath string
	Query    string
}

type Provider interface {
	Analyze(ctx context.Context, request Request) (evidence.Graph, error)
}
