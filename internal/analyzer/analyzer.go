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

type LocationRequest struct {
	RepoPath      string
	Location      evidence.Location
	MaxCandidates int
	RankTerms     []string
}

type LocationCandidate struct {
	Entity       evidence.Entity    `json:"entity"`
	Match        string             `json:"match"`
	Certainty    evidence.Certainty `json:"certainty"`
	Distance     int                `json:"distance_lines"`
	Investigable bool               `json:"investigable"`
	RankReasons  []string           `json:"rank_reasons,omitempty"`
}

type LocationResolution struct {
	Location   evidence.Location   `json:"location"`
	Candidates []LocationCandidate `json:"candidates"`
	Certainty  evidence.Certainty  `json:"certainty"`
	Provenance evidence.Provenance `json:"provenance"`
	Scenario   evidence.Scenario   `json:"scenario"`
	Warnings   []string            `json:"warnings,omitempty"`
}

type LocationResolver interface {
	ResolveLocation(ctx context.Context, request LocationRequest) (LocationResolution, error)
}

// ExactSymbolRequest identifies one already selected declaration. Symbol's
// location is part of its identity; implementations must not fall back to a
// name-only lookup when confirming it.
type ExactSymbolRequest struct {
	RepoPath string
	Symbol   evidence.Entity
}

// ExactSymbolAnalyzer confirms one selected declaration and collects its
// bounded structural neighborhood.
type ExactSymbolAnalyzer interface {
	AnalyzeExactSymbol(ctx context.Context, request ExactSymbolRequest) (evidence.Graph, error)
}
