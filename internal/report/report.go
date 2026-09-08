package report

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/terminology"
)

// CurrentFormatVersion is the canonical ProgramPortfolio report contract.
const CurrentFormatVersion = 85

// MaxReportJSONBytes is the former ordinary report.json threshold. It is
// advisory only; complete validated report authority is never rejected or
// shortened because it crosses this size.
const MaxReportJSONBytes = 64 << 20

// MaxOrdinaryReportHTMLBytes is the former advisory size covering the same browser projection plus the
// embedded application assets and transient source/navigation authority. The
// compact browser payload is mostly a subset of the pretty-printed report
// JSON. A served source-ID map repeats every encoded openable path once, so a
// second complete JSON envelope is reserved for that worst case. The final
// 16 MiB covers embedded assets, bounded target navigation, and source-ID
// values without making served publication stricter than static publication.
const MaxOrdinaryReportHTMLBytes = 2*MaxReportJSONBytes + 16<<20

// ReportData is the persisted Program report plus transient publication
// authority. It deliberately contains no retired Architecture, Study, Surface,
// workspace-graph, source-snippet, or orientation product.
type ReportData struct {
	FormatVersion int `json:"format_version"`

	ProgramPortfolio       *ProgramPortfolio           `json:"program_portfolio"`
	GroupGraph             *GroupGraphView             `json:"group_graph"`
	TargetOutcomePortfolio *TargetOutcomePortfolioView `json:"target_outcome_portfolio"`
	RepoName               string                      `json:"repo_name"`
	Warnings               []string                    `json:"warnings,omitempty"`
	// Timing is where the run's time went: the wall clock, and per model
	// stage the calls and the provider time. A reader deciding whether to
	// run this on a bigger repository wants the number, not a feeling.
	Timing *RunTiming `json:"timing,omitempty"`

	// Facts, Claims and Orientation are the repository-level first-day
	// artifacts. Orientation is absent when no model is used.
	Facts       *facts.Result         `json:"facts,omitempty"`
	Claims      *claims.Result        `json:"claims,omitempty"`
	Orientation *orientation.Result   `json:"orientation,omitempty"`
	Questions   []atlas.QuestionRoute `json:"questions,omitempty"`
	Learning    *atlas.LearningPlan   `json:"learning,omitempty"`
	Glossary    *terminology.Catalog  `json:"glossary,omitempty"`

	OpenablePaths []string          `json:"openable_paths"`
	SourceIDs     map[string]string `json:"source_ids,omitempty"`

	GitLabSourceLinks *GitLabSourceLinks `json:"gitlab_source_links,omitempty"`
	GitHubSourceLinks *GitHubSourceLinks `json:"github_source_links,omitempty"`

	CapturedRevision string `json:"captured_revision"`
	// ReadmeOverview is the repository's own account of itself, condensed
	// from its README by the documentation stage. It was computed on every
	// run, hashed into the manifest, handed to categorization as context,
	// and never shown to the one reader it was written for.
	ReadmeOverview string `json:"readme_overview,omitempty"`

	// ArtifactsDir and the following fields are process-local publication
	// authority. They are never persisted or embedded in the browser payload.
	ArtifactsDir                        string `json:"-"`
	standaloneLocalRoots                []string
	materialInputPaths                  []string
	defaultProgramIndex                 *programindex.Index
	programIndexes                      []programindex.Index
	defaultProgramIndexArtifactFilename string
	dependencyCatalog                   *dependencies.Catalog
	localGroupsIndex                    *groupindex.Index
	reducedDocumentation                *documentationreduce.Result
	targetMetadataBytes                 int
}

// RunTiming mirrors the run's Time stage as the page reads it.
type RunTiming struct {
	WallMS int64         `json:"wall_ms"`
	Stages []StageTiming `json:"stages,omitempty"`
}

type StageTiming struct {
	Stage      string `json:"stage"`
	Live       int    `json:"live"`
	Cached     int    `json:"cached"`
	ProviderMS int64  `json:"provider_ms"`
	SlowestMS  int64  `json:"slowest_ms"`
}
