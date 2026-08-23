package report

import (
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// CurrentFormatVersion is the canonical ProgramPortfolio report contract.
const CurrentFormatVersion = 60

// MaxReportJSONBytes is the single ordinary report.json bound shared by
// generation, manifest verification, and the local report server.
const MaxReportJSONBytes = 64 << 20

// MaxOrdinaryReportHTMLBytes covers the same browser projection plus the
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

	// AnalysisTarget is present for the current Go semantic cube. Language-
	// neutral ProgramPortfolio publication does not require it.
	AnalysisTarget *analysistarget.Target `json:"analysis_target,omitempty"`

	ProgramPortfolio       *ProgramPortfolio       `json:"program_portfolio"`
	CubeMapView            *CubeMapView            `json:"cube_map_view,omitempty"`
	CoreMapView            *CoreMapView            `json:"core_map_view,omitempty"`
	ActivityEntrypointView *ActivityEntrypointView `json:"activity_entrypoint_view,omitempty"`
	IntegrationUsageView   *IntegrationUsageView   `json:"integration_usage_view,omitempty"`
	ActivityPathView       *ActivityPathView       `json:"activity_path_view,omitempty"`

	RepoName string   `json:"repo_name"`
	Warnings []string `json:"warnings,omitempty"`

	OpenablePaths []string          `json:"openable_paths"`
	SourceIDs     map[string]string `json:"source_ids,omitempty"`

	GitLabSourceLinks *GitLabSourceLinks `json:"gitlab_source_links,omitempty"`
	GitHubSourceLinks *GitHubSourceLinks `json:"github_source_links,omitempty"`

	CapturedRevision   string `json:"captured_revision"`
	CapturedInputCount int    `json:"captured_input_count"`

	// ArtifactsDir and the following fields are process-local publication
	// authority. They are never persisted or embedded in the browser payload.
	ArtifactsDir         string `json:"-"`
	standaloneLocalRoots []string
	materialInputPaths   []string
	defaultProgramIndex  *programindex.Index
	pythonTargetCatalog  *pythontarget.Catalog
	declaredDependencies *dependencydeclaration.Result
}
