package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const CurrentFormatVersion = 31

const AtlasStudyReportProjectionVersion = 8

// MaxAtlasStudyBrowseSpans bounds the report-side provider-free per-span
// browse. Truthful Total/Shown keep larger repositories honest; the complete
// considered set stays bound by the status artifact's CandidateSHA256 digest.
const MaxAtlasStudyBrowseSpans = 256

// The Study theme shelf is not capped: every published card is shown.
// The complete reduced portfolio stays bound by the study_themes artifact
// digest and its bounded encode (MaxStudyThemesArtifactBytes), so the
// report-side projection never needs to hide cards — more facts is better,
// the product investigates as far as the evidence goes.

const maxAtlasStudyReportCoverageCount = 1_000_000

const maxExactDiscoveryDeclarations = 16

// ExactDiscoveryAnchor is a deterministic declaration found inside one
// already-saved, authorized source window. It is deliberately weaker than a
// behavior fact: it says where study can start, not what the code does.
type ExactDiscoveryAnchor struct {
	Path          string
	Language      string
	Symbol        string
	Line          int
	Statement     string
	ContentSHA256 string
}

// ExactDiscoveryAnchors returns bounded declaration anchors in saved line
// order. Callers remain responsible for proving that the supplied window came
// from the authorized source catalog and has matching local provenance.
func ExactDiscoveryAnchors(
	sourcePath string,
	startLine int,
	lines []string,
) []ExactDiscoveryAnchor {
	if !validUserTopicPath(sourcePath) || startLine <= 0 ||
		len(lines) == 0 || len(lines) > maxFullFunctionSourceLines {
		return nil
	}
	language := sourceLanguage(sourcePath)
	if language == "text" {
		return nil
	}
	for _, line := range lines {
		if len(line) > 64<<10 {
			return nil
		}
	}
	result := make([]ExactDiscoveryAnchor, 0, min(len(lines), maxExactDiscoveryDeclarations))
	seen := make(map[string]struct{}, cap(result))
	for index, text := range lines {
		symbol, _, _, ok := boundedSourceDeclaration(sourcePath, text)
		if !ok || !boundedUserTopicText(symbol, maxUserTopicSymbolBytes) {
			continue
		}
		line := startLine + index
		key := sourcePath + "\x00" + language + "\x00" + symbol + "\x00" + fmt.Sprint(line)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ExactDiscoveryAnchor{
			Path:     sourcePath,
			Language: language,
			Symbol:   symbol,
			Line:     line,
			Statement: fmt.Sprintf(
				"The %s declaration is present in an exact %s source window available for local behavior investigation.",
				symbol,
				language,
			),
			ContentSHA256: sourceLinesSHA256([]string{text}),
		})
		if len(result) == maxExactDiscoveryDeclarations {
			break
		}
	}
	return result
}

type ReportData struct {
	FormatVersion int `json:"format_version"`
	// ReportLanguage is transient render state selected by the requested
	// presentation locale. It controls the typed product message catalog
	// independently of whether optional model-authored prose was translated.
	// Canonical report artifacts always omit it.
	ReportLanguage string `json:"report_language,omitempty"`
	// GitLabSourceLinks is present only when the report is intended to be a
	// standalone shareable artifact. Source actions then target the exact
	// captured revision instead of the localhost editor/source APIs.
	GitLabSourceLinks *GitLabSourceLinks `json:"gitlab_source_links,omitempty"`
	// GitHubSourceLinks is the GitHub-hosted equivalent of GitLabSourceLinks.
	// At most one external source host is present in a rendered report.
	GitHubSourceLinks *GitHubSourceLinks `json:"github_source_links,omitempty"`

	RepoName     string `json:"repo_name"`
	ProjectGuess string `json:"project_guess"`
	// DocumentedPurpose is a bounded presentation-only repository purpose
	// extracted from the repository's own documentation. It is kept separate
	// from model orientation so the onboarding thesis can prefer author-written
	// purpose without promoting it into semantic evidence.
	DocumentedPurpose          string               `json:"documented_purpose,omitempty"`
	OrientationConfidence      float64              `json:"orientation_confidence"`
	HighLevelMap               []Subsystem          `json:"high_level_map,omitempty"`
	FirstFilesToOpen           []FileItem           `json:"first_files_to_open,omitempty"`
	CandidateFlows             []string             `json:"candidate_flows"`
	CandidateDirections        []CandidateDirection `json:"candidate_directions,omitempty"`
	ImportantDomainWords       []DomainWord         `json:"important_domain_words,omitempty"`
	QuestionsForHuman          []string             `json:"questions_for_human,omitempty"`
	OrientationUnverifiedPaths []PathItem           `json:"unverified_paths,omitempty"`
	Flows                      []FlowData           `json:"flows"`
	ArtifactsDir               string               `json:"artifacts_dir"`
	FeedbackPath               string               `json:"feedback_path,omitempty"`
	Warnings                   []string             `json:"warnings,omitempty"`
	// PresentationWarnings is a render-only parallel copy of Warnings. A
	// localized render clone replaces dynamic warning prose in both slices so
	// the standalone terminal artifact does not retain copyable English prose.
	PresentationWarnings []string `json:"presentation_warnings,omitempty"`
	// PresentationWarningKinds is a render-only parallel list of typed catalog
	// message IDs for warnings whose product-owned identity is known from
	// structural run status. Canonical report artifacts never persist it.
	PresentationWarningKinds []string `json:"presentation_warning_kinds,omitempty"`
	// PresentationWarningMessages is the render-only typed message projection
	// for product warnings that require catalog parameters. Canonical report
	// artifacts keep only the unchanged legacy Warnings strings.
	PresentationWarningMessages []RunPresentationWarning `json:"presentation_warning_messages,omitempty"`
	Run                         *RunInfo                 `json:"run,omitempty"`
	OpenablePaths               []string                 `json:"openable_paths,omitempty"`
	// SourceIDs is an ephemeral server-rendered map from authorized
	// repository-relative paths to opaque navigation IDs. WriteReportJSON
	// deliberately excludes it from persisted report evidence.
	SourceIDs       map[string]string `json:"source_ids,omitempty"`
	RepositoryGraph *RepositoryGraph  `json:"repository_graph,omitempty"`
	// RepositoryAtlas is the exact language-neutral canonical Atlas persisted
	// beside the report. It contains only locally proven Units, entities,
	// observations, evidence and relations; files and symbols remain evidence
	// locators rather than entities.
	RepositoryAtlas *repositoryatlas.Atlas `json:"repository_atlas,omitempty"`
	// Navigator is the deliberately small report projection of the exact
	// persisted Atlas-first result. The full request and action catalog remain
	// separate, hash-bound run artifacts.
	Navigator          *NavigatorReportProduct `json:"navigator,omitempty"`
	Components         []Component             `json:"components,omitempty"`
	ComponentRelations []ComponentRelation     `json:"component_relations,omitempty"`
	ArchitectureCanvas *ArchitectureCanvas     `json:"architecture_canvas,omitempty"`
	// ArchitectureComponentNavigation is a report-owned navigation projection
	// over the exact accepted Canvas. It keeps conceptual map targets separate
	// from producer-owned source starts and never selects a representative
	// package, file, or symbol by presentation order.
	ArchitectureComponentNavigation *ArchitectureComponentNavigationProjection `json:"architecture_component_navigation,omitempty"`
	GuidedTour                      *guidedtour.Story                          `json:"guided_tour,omitempty"`
	SemanticArtifacts               []semanticdiscovery.Artifact               `json:"semantic_artifacts,omitempty"`
	// UserMechanisms is a presentation-only supported slice of independently
	// replayed canonical Mechanisms. Raw artifacts remain available for replay
	// and provenance, but default onboarding renders this narrower projection.
	UserMechanisms []UserMechanism `json:"user_mechanisms,omitempty"`
	// UserTopics exposes locally grounded questions that did not pass the
	// unchanged Mechanism publication gate. Topics contain exact starting
	// symbols and a bounded explanation of missing proof, never an answer,
	// ordered steps, an effect, or a claimed path.
	UserTopics []UserTopic `json:"user_topics,omitempty"`
	// RepositoryThesis is a presentation-only overview assembled exclusively
	// from bounded documented purpose and already validated report navigation
	// targets. It does not participate in semantic identity or evidence.
	RepositoryThesis *RepositoryThesis `json:"repository_thesis,omitempty"`
	// RepositoryGuide is the product-facing ordering of already accepted
	// Mechanisms, exact source continuations, and useful architecture areas.
	// It is a presentation projection only: canonical semantic objects remain
	// the sole authority for every explanation it references.
	RepositoryGuide *RepositoryGuide `json:"repository_guide,omitempty"`
	// StudyMap is a presentation-only repository brief and ordered reading
	// guide over exact, locally validated code anchors. Its order is editorial,
	// not a runtime sequence; canonical Mechanisms remain separate authority.
	StudyMap *RepositoryStudyMap `json:"study_map,omitempty"`
	// AtlasStudy is the closed publication state of the Atlas-first Brief and
	// Study stage. Accepted content is projected separately through StudyMap;
	// unavailable and failed states remain explicit without manufacturing an
	// old repository_study_map result.
	AtlasStudy *AtlasStudyReportStatus `json:"atlas_study,omitempty"`
	// StudyPublication records whether the independent Study editing stage
	// published a usable result. A failed stage remains explicit in the product
	// report instead of being silently replaced by the remaining Overview.
	StudyPublication *StudyPublicationStatus `json:"study_publication,omitempty"`
	// IncompleteStudy retains bounded provider questions that have exact saved
	// reading starts but did not satisfy the unchanged complete Reading Pack
	// contract. It is navigation only and never claims an ordered path.
	IncompleteStudy *RepositoryIncompleteStudy `json:"incomplete_study,omitempty"`
	// Operations is a presentation-only operating guide over exact, bounded
	// repository-owned commands, configuration, endpoints, and documentation.
	// It never authorizes command execution; CopyText is present only after
	// conservative local validation.
	Operations *RepositoryOperations `json:"operations,omitempty"`
	// TaskInvestigation is an optional task-first projection of one validated,
	// bounded Task Investigation Pack. It deliberately omits the pack's opaque
	// evidence identifiers and never replaces the canonical saved task artifacts.
	TaskInvestigation *TaskInvestigationWorkspace `json:"task_investigation,omitempty"`
	// UserSources contains bounded saved source for a useful Overview fallback.
	// SourceContextIDs is issued only by the verified localhost server and is
	// removed from persisted report evidence beside SourceIDs.
	UserSources      []SourceSnippet          `json:"user_sources,omitempty"`
	SourceContextIDs map[string]string        `json:"source_context_ids,omitempty"`
	SemanticCoverage *SemanticCoverageSummary `json:"semantic_coverage,omitempty"`
	// StartHereArtifactID is a presentation-only selection of one validated
	// semantic artifact. It is not evidence and does not participate in the
	// Mechanism content hash.
	StartHereArtifactID string `json:"start_here_artifact_id,omitempty"`
	// SemanticSupplementalFacts is an ephemeral, locally validated enrichment
	// used while replaying one bounded semantic experiment. It is deliberately
	// excluded from report.json; the authoritative copy remains the saved local
	// probe artifact beside the replay record.
	SemanticSupplementalFacts     []semanticdiscovery.Fact     `json:"-"`
	SemanticSearchDisabled        bool                         `json:"semantic_search_disabled,omitempty"`
	SemanticSearch                *SemanticSearchIndex         `json:"semantic_search,omitempty"`
	ArchitectureSynthesis         *ArchitectureSynthesisStatus `json:"architecture_synthesis,omitempty"`
	ArchitectureGrounding         *ArchitectureGrounding       `json:"architecture_grounding,omitempty"`
	ModelResearch                 *modelresearch.State         `json:"model_research,omitempty"`
	DiscoveredSurfaces            *DiscoveredSurfaces          `json:"discovered_surfaces,omitempty"`
	CommandTraces                 []gofacts.CommandTrace       `json:"command_traces,omitempty"`
	Freshness                     *freshness.FreshnessResult   `json:"freshness,omitempty"`
	CapturedRevision              string                       `json:"captured_revision,omitempty"`
	CapturedInputCount            int                          `json:"captured_input_count,omitempty"`
	RepositorySubmodules          []freshness.SubmoduleState   `json:"repository_submodules,omitempty"`
	evidenceLocations             []evidence.Location
	sourceSignals                 []SourceSignal
	studyDocumentSourceRoot       string
	standaloneLocalRoots          []string
	externalImports               []externalImportUsage
	repositoryGoFacts             *gofacts.Facts
	repositoryEntrypointFacts     *gofacts.Facts
	architectureDebugPresentation map[string]string
	semanticAttempted             int
	semanticInvestigated          int
	// Presentation localization is transient render state loaded from a
	// separately validated sidecar. It is never part of canonical report JSON.
	presentationLocalizationState     string
	presentationLocalizationMessageID string
	requestedPresentationLocale       string
	presentationSourceEpisode         *sourceEpisodeProjection
	runWarningDiagnostics             []runWarningDiagnostic
	// presentationMetadataErr quarantines an invalid optional presentation
	// sidecar without turning it into canonical report prose or failing EN
	// replay. Shared hydration surfaces it to RU localization and serving.
	presentationMetadataErr error

	RecommendedFlow string `json:"recommended_flow,omitempty"`
	FlowCount       int    `json:"flow_count"`
}

// NavigatorReportProduct carries only the product state and, when selected,
// the backend-owned action already validated against the exact persisted
// Repository Atlas. It contains no provider-authored prose.
type NavigatorReportProduct struct {
	Version         int                             `json:"version"`
	State           navigator.ProductState          `json:"state"`
	UnavailableCode navigator.UnavailableCode       `json:"unavailable_code,omitempty"`
	FailureCode     navigator.FailureCode           `json:"failure_code,omitempty"`
	Recommendation  *navigator.RecommendationAction `json:"recommendation,omitempty"`
}

// AtlasStudyReportStatus deliberately excludes provider prose, raw errors and
// private request identities. The exact request/result/status artifacts stay
// hash-bound material inputs of the authorized report. Under Decision 213 the
// Study section carries the editorial theme shelf (Themes) plus the re-based
// four-stage browse; the retired single-stage atlas-study provider call no
// longer contributes a Brief or per-span directions to new runs.
type AtlasStudyReportStatus struct {
	Version                 int                          `json:"version"`
	ProjectionVersion       int                          `json:"projection_version"`
	State                   atlasstudy.ProductState      `json:"state"`
	UnavailableCode         AtlasStudyUnavailableCode    `json:"unavailable_code,omitempty"`
	FailureCode             atlasstudy.FailureCode       `json:"failure_code,omitempty"`
	CandidateCoverage       *AtlasStudyCandidateCoverage `json:"candidate_coverage,omitempty"`
	DirectionCount          int                          `json:"direction_count,omitempty"`
	PublishedDirectionCount int                          `json:"published_direction_count,omitempty"`
	HiddenDirectionCount    int                          `json:"hidden_direction_count,omitempty"`
	// Four-stage span counts: considered (complete set), seed-advertised (a*
	// seeds in the Scout request catalog), scout-anchored (anchors in a
	// Scout-accepted candidate), published (anchors in final theme readings).
	ConsideredSpanCount    int `json:"considered_span_count,omitempty"`
	AdvertisedSpanCount    int `json:"advertised_span_count,omitempty"`
	ModelSelectedSpanCount int `json:"model_selected_span_count,omitempty"`
	AcceptedSpanCount      int `json:"accepted_span_count,omitempty"`
	// Four independent coverage flags. They are recorded independently and are
	// part of the documented projection contract, so they serialize even
	// when false.
	FrontierComplete        bool `json:"frontier_complete"`
	SelectedItemsComplete   bool `json:"selected_items_complete"`
	SupportCoverageComplete bool `json:"support_coverage_complete"`
	PortfolioTargetMet      bool `json:"portfolio_target_met"`
	// Omissions are bounded public-safe aggregates of considered spans omitted
	// from the advertised frontier: exact counts by closed reason plus the
	// bounded representative count. Canonical identities never enter the
	// report, so representative route-span refs are reduced to their count.
	Omissions []AtlasStudyOmissionAggregate `json:"omissions,omitempty"`
	// FrontierBrowse is the bounded provider-free per-span browse of the
	// complete considered Study question set. It is derived only inside
	// readAtlasStudyReportProduct from already-validated local artifacts and
	// stays nil for unavailable/prepared/uncalled states.
	FrontierBrowse *FrontierBrowse `json:"frontier_browse,omitempty"`
	// Themes is the editorial, source-grounded Study theme shelf (Decision
	// 213). It is derived only inside readAtlasStudyReportProduct from the
	// SHA-bound theme artifacts and stays nil for unavailable/prepared/
	// uncalled states and on Scout failure.
	Themes *AtlasStudyThemesProjection `json:"themes,omitempty"`
}

// AtlasStudySpanStage is the highest reached stage of one span, derived by
// exact set arithmetic at projection time. It is never provider-authored.
// Under Decision 213 the four stages are re-based onto the two-stage theme
// pipeline: considered / seed-advertised / scout-anchored / published.
type AtlasStudySpanStage string // "considered" | "seed_advertised" | "scout_anchored" | "published"

const (
	AtlasStudySpanStageConsidered      AtlasStudySpanStage = "considered"
	AtlasStudySpanStageAdvertised      AtlasStudySpanStage = "advertised"
	AtlasStudySpanStageModelSelected   AtlasStudySpanStage = "model_selected"
	AtlasStudySpanStageAccepted        AtlasStudySpanStage = "accepted"
	AtlasStudySpanStageSeedAdvertised  AtlasStudySpanStage = "seed_advertised"
	AtlasStudySpanStageScoutAnchored   AtlasStudySpanStage = "scout_anchored"
	AtlasStudySpanStagePublished       AtlasStudySpanStage = "published"
)

// AtlasStudyThemesProjection is the bounded public-safe theme shelf.
// Total/Shown are always truthful and equal: every published card renders,
// never truncated.
type AtlasStudyThemesProjection struct {
	Total int              `json:"total"`
	Shown int              `json:"shown"`
	Cards []StudyThemeCard `json:"cards"`
}

// StudyThemeCard is one published theme card. It carries editorial prose,
// ordered exact readings and an honest badge, and zero source bytes. The
// public Ordinal is manifest-relative (canonical theme order); CanonicalID is
// never serialized here.
type StudyThemeCard struct {
	Ordinal          int                  `json:"ordinal"`
	FinalTitle       string               `json:"final_title"`
	FinalQuestion    string               `json:"final_question"`
	WhyItMatters     string               `json:"why_it_matters"`
	ExpectedLearning string               `json:"expected_learning"`
	ThemeKind        string               `json:"theme_kind"`
	Readings         []StudyThemeReading  `json:"readings"`
	Badge            string               `json:"badge"`
	Limitation       string               `json:"limitation,omitempty"`
}

// StudyThemeReading is one ordered exact reading on a theme card. Path/line
// publish only for paths in OpenablePaths; otherwise the neutral unavailable
// state renders (no dead buttons).
type StudyThemeReading struct {
	Label  string `json:"label"`
	Symbol string `json:"symbol"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
}

// FrontierBrowse is the bounded provider-free per-span browse of the complete
// considered Study question set. Total/Shown are always truthful; Spans never
// exceed MaxAtlasStudyBrowseSpans.
type FrontierBrowse struct {
	Total int    `json:"total"` // complete considered count (len of rebuilt input.RouteSpans)
	Shown int    `json:"shown"` // len(Spans)
	Spans []Span `json:"spans"`
}

// Span is one browse row. Ordinal is 1..N in canonical span-ID order within
// learning-stage groups and is manifest-relative; it is NOT a canonical ID.
// Stage is the four-value membership. Source/Endpoint are exact user-code
// locations published only for paths in OpenablePaths; a row whose source
// cannot open carries the neutral unavailable state instead of a dead button.
// ThemeRefs is present ONLY on published rows: it lists every matching
// published theme ordinal in canonical theme order (all matching themes
// render, D213 B1/N5); no canonical span ID is serialized. A published row
// with no matching theme (should not occur — fail closed) renders without
// links.
type Span struct {
	Ordinal     int                 `json:"ordinal"`
	Title       string              `json:"title"`    // exact source-card symbol/label; system-path "from → to" endpoints
	Question    string              `json:"question"` // backend-compiled question in the report language
	Stage       AtlasStudySpanStage `json:"stage"`
	Source      UserCodeLocation    `json:"source"`             // only when Source.Path ∈ data.OpenablePaths
	Endpoint    *UserCodeLocation   `json:"endpoint,omitempty"` // only for system-path spans whose endpoint path ∈ data.OpenablePaths
	ThemeRefs   []int               `json:"theme_refs,omitempty"` // published rows ONLY; canonical theme ordinals
}

// AtlasStudyOmissionAggregate is the public-safe report projection of one
// closed advertised-frontier omission reason. RepresentativeCount is the
// bounded number of representative typed refs recorded by the exact artifact;
// the canonical refs themselves are never projected.
type AtlasStudyOmissionAggregate struct {
	Reason              atlasstudy.CoverageOmissionReason `json:"reason"`
	Count               int                               `json:"count"`
	RepresentativeCount int                               `json:"representative_count,omitempty"`
}

// AtlasStudyCandidateCoverage is the public-safe report projection of the
// exact private candidate shelf bound by the Atlas Study request/status
// artifacts. It deliberately omits the candidate digest and canonical package
// bucket IDs. Package bucket counts remain an exact anonymous histogram, so
// bounded selection loss is visible without publishing backend identities.
type AtlasStudyCandidateCoverage struct {
	TargetsConsidered int                               `json:"targets_considered"`
	TargetsSelected   int                               `json:"targets_selected"`
	SpansConsidered   int                               `json:"spans_considered"`
	SpansSelected     int                               `json:"spans_selected"`
	Complete          bool                              `json:"complete"`
	PerRole           []AtlasStudyRoleCandidateCoverage `json:"per_role"`
	PackageBuckets    []AtlasStudyAnonymousCoverage     `json:"package_buckets"`
}

type AtlasStudyRoleCandidateCoverage struct {
	Role       atlasstudy.SupportRole `json:"role"`
	Considered int                    `json:"considered"`
	Selected   int                    `json:"selected"`
}

type AtlasStudyAnonymousCoverage struct {
	Considered int `json:"considered"`
	Selected   int `json:"selected"`
}

// projectAtlasStudyCandidateCoverage removes only private candidate and
// package identities. All counts, every producer-owned role lane, and the
// anonymous package-bucket histogram remain exact.
func projectAtlasStudyCandidateCoverage(
	coverage atlasstudy.CandidateCoverage,
) (*AtlasStudyCandidateCoverage, error) {
	projected := &AtlasStudyCandidateCoverage{
		TargetsConsidered: coverage.TargetsConsidered,
		TargetsSelected:   coverage.TargetsSelected,
		SpansConsidered:   coverage.SpansConsidered,
		SpansSelected:     coverage.SpansSelected,
		Complete:          coverage.Complete,
	}
	for _, count := range coverage.PerRole {
		role := atlasstudy.SupportRole(count.Key)
		projected.PerRole = append(projected.PerRole, AtlasStudyRoleCandidateCoverage{
			Role: role, Considered: count.Considered, Selected: count.Selected,
		})
	}
	for _, count := range coverage.PerPackage {
		projected.PackageBuckets = append(projected.PackageBuckets, AtlasStudyAnonymousCoverage{
			Considered: count.Considered, Selected: count.Selected,
		})
	}
	sort.Slice(projected.PerRole, func(i, j int) bool {
		return projected.PerRole[i].Role < projected.PerRole[j].Role
	})
	sort.Slice(projected.PackageBuckets, func(i, j int) bool {
		if projected.PackageBuckets[i].Considered != projected.PackageBuckets[j].Considered {
			return projected.PackageBuckets[i].Considered < projected.PackageBuckets[j].Considered
		}
		return projected.PackageBuckets[i].Selected < projected.PackageBuckets[j].Selected
	})
	if err := projected.validate(); err != nil {
		return nil, err
	}
	return projected, nil
}

// projectAtlasStudyOmissions projects the bounded omission aggregates in a
// public-safe form. Representative refs are canonical route-span identities and
// never enter the report; only the bounded representative count is published.
func projectAtlasStudyOmissions(omissions []atlasstudy.CoverageOmission) []AtlasStudyOmissionAggregate {
	if len(omissions) == 0 {
		return nil
	}
	projected := make([]AtlasStudyOmissionAggregate, 0, len(omissions))
	for _, omission := range omissions {
		projected = append(projected, AtlasStudyOmissionAggregate{
			Reason: omission.Reason, Count: omission.Count,
			RepresentativeCount: len(omission.Representatives),
		})
	}
	return projected
}

func validateAtlasStudyOmissionProjection(omissions []AtlasStudyOmissionAggregate) error {
	if len(omissions) == 0 {
		return nil
	}
	previous := atlasstudy.CoverageOmissionReason("")
	for _, omission := range omissions {
		if !omission.Reason.Valid() || omission.Count <= 0 ||
			omission.Count > maxAtlasStudyReportCoverageCount ||
			omission.RepresentativeCount < 0 ||
			omission.RepresentativeCount > atlasstudy.MaxOmissionRepresentatives {
			return fmt.Errorf("atlas study report: invalid omission aggregate")
		}
		if previous != "" && omission.Reason <= previous {
			return fmt.Errorf("atlas study report: omission aggregates are not canonical")
		}
		previous = omission.Reason
	}
	return nil
}

func (coverage AtlasStudyCandidateCoverage) validate() error {
	validCount := func(value int) bool {
		return value > 0 && value <= maxAtlasStudyReportCoverageCount
	}
	if !validCount(coverage.TargetsConsidered) || !validCount(coverage.TargetsSelected) ||
		coverage.TargetsSelected > coverage.TargetsConsidered ||
		!validCount(coverage.SpansConsidered) || !validCount(coverage.SpansSelected) ||
		coverage.SpansSelected > coverage.SpansConsidered ||
		coverage.Complete != (coverage.TargetsSelected == coverage.TargetsConsidered &&
			coverage.SpansSelected == coverage.SpansConsidered) ||
		len(coverage.PerRole) == 0 || len(coverage.PerRole) > 6 ||
		len(coverage.PackageBuckets) == 0 || len(coverage.PackageBuckets) > coverage.TargetsConsidered {
		return fmt.Errorf("atlas study report: invalid candidate coverage")
	}
	previousRole := atlasstudy.SupportRole("")
	for _, count := range coverage.PerRole {
		if !count.Role.Valid() || count.Role <= previousRole || !validCount(count.Considered) ||
			count.Selected < 0 || count.Selected > count.Considered {
			return fmt.Errorf("atlas study report: invalid role candidate coverage")
		}
		previousRole = count.Role
	}
	previous := AtlasStudyAnonymousCoverage{}
	for index, count := range coverage.PackageBuckets {
		if !validCount(count.Considered) || count.Selected < 0 || count.Selected > count.Considered ||
			(index > 0 && (count.Considered < previous.Considered ||
				(count.Considered == previous.Considered && count.Selected < previous.Selected))) {
			return fmt.Errorf("atlas study report: invalid anonymous package candidate coverage")
		}
		previous = count
	}
	return nil
}

type AtlasStudyUnavailableCode string

const (
	AtlasStudyUnavailableOffline                AtlasStudyUnavailableCode = "offline"
	AtlasStudyUnavailableArchitectureEnrichment AtlasStudyUnavailableCode = "architecture_enrichment_unavailable"
)

type runWarningDiagnostic struct {
	WarningIndex   int
	Code           orient.ConfidenceWarningCode
	CandidateIndex int
	Proposed       float64
	Capped         float64
}

// RunPresentationWarning addresses one raw warning and supplies a closed set
// of typed parameters to the shared EN/RU product-message catalog.
type RunPresentationWarning struct {
	WarningIndex   int    `json:"warning_index"`
	MessageID      string `json:"message_id"`
	CandidateIndex int    `json:"candidate_index"`
	Proposed       string `json:"proposed"`
	Capped         string `json:"capped"`
}

// UserTopic is a presentation-only projection of one rejected-but-grounded
// fresh-repository candidate. Its deliberately narrow shape prevents a topic
// from being confused with a published Mechanism.
type UserTopic struct {
	CandidateID     string            `json:"candidate_id"`
	Title           string            `json:"title"`
	Question        string            `json:"question"`
	StartingSymbols []UserTopicSymbol `json:"starting_symbols"`
	Uncertainty     string            `json:"uncertainty"`
}

// UserTopicSymbol is one exact repository-owned place from which a reader can
// continue through the existing authorized source navigation.
type UserTopicSymbol struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

const (
	freshRepoDemoStatusFileForTopics     = "fresh_repo_demo_status.json"
	freshRepoDemoCandidatesFileForTopics = "fresh_repo_candidates.json"
	freshRepoOpportunityFileForTopics    = "fresh_repo_opportunity_attempt.json"

	maxUserTopicArtifactBytes         = 1 << 20
	maxUserTopicOpportunityCandidates = 20
	maxUserTopics                     = 3
	maxUserTopicSymbols               = 4
	maxUserTopicAnchors               = 8
	maxUserTopicReasons               = 8
	maxUserTopicIDBytes               = 256
	maxUserTopicTitleBytes            = 240
	maxUserTopicQuestionBytes         = 800
	maxUserTopicPathBytes             = 4096
	maxUserTopicSymbolBytes           = 512
	maxUserTopicReasonBytes           = 128
)

type userTopicOpportunityArtifact struct {
	ValidationState    string `json:"validation_state"`
	NormalizedProposal struct {
		Candidates []struct {
			ID               string `json:"id"`
			Title            string `json:"title"`
			QuestionAnswered string `json:"question_answered"`
		} `json:"candidates"`
	} `json:"normalized_proposal"`
}

type userTopicEligibility struct {
	Status          string   `json:"status"`
	Reasons         []string `json:"reasons"`
	DistinctSymbols []string `json:"distinct_symbols"`
}

type userTopicCandidatesArtifact struct {
	Selected []struct {
		CandidateID string `json:"candidate_id"`
		Question    string `json:"question"`
		Primary     *struct {
			Status      string                `json:"status"`
			RootAnchors []userTopicRootAnchor `json:"root_anchors"`
			Eligibility userTopicEligibility  `json:"eligibility"`
			AnchorFacts []userTopicAnchorFact `json:"anchor_facts"`
		} `json:"primary_path"`
	} `json:"selected"`
}

type userTopicRootAnchor struct {
	OriginFactID string `json:"origin_fact_id"`
	Path         string `json:"path"`
	Symbol       string `json:"symbol"`
}

type userTopicAnchorFact struct {
	ID     string `json:"id"`
	Source *struct {
		Path            string `json:"path"`
		StartLine       int    `json:"start_line"`
		EnclosingSymbol string `json:"enclosing_symbol"`
	} `json:"source"`
}

type userTopicStatusArtifact struct {
	Attempts []struct {
		CandidateID        string                `json:"candidate_id"`
		Question           string                `json:"question"`
		State              string                `json:"state"`
		FailureStage       string                `json:"failure_stage"`
		PrimaryEligibility *userTopicEligibility `json:"primary_eligibility"`
	} `json:"attempts"`
}

// SemanticCoverageSummary keeps the publication funnel visible beside a
// small set of polished semantic artifacts. It is derived locally from saved
// replay records and never participates in semantic truth or Mechanism hashes.
type SemanticCoverageSummary struct {
	OpportunitiesAttempted       int    `json:"opportunities_attempted"`
	CandidatesInvestigated       int    `json:"candidates_investigated"`
	CanonicalMechanismsPublished int    `json:"canonical_mechanisms_published"`
	CentralRoutingMechanism      string `json:"central_routing_mechanism,omitempty"`
}

// RepositoryGraph is a bounded deterministic projection of local repository
// facts used to connect model-suggested components without asking the model to
// invent relationships between them.
type RepositoryGraph struct {
	Version      int           `json:"version,omitempty"`
	Modules      []ModuleInfo  `json:"modules,omitempty"`
	Packages     []PackageInfo `json:"packages,omitempty"`
	PackageEdges []EdgeInfo    `json:"package_edges,omitempty"`
}

// ModuleInfo maps repository-relative directories to import-path roots. The
// longest matching directory owns a file in repositories with nested modules.
type ModuleInfo struct {
	ID          string `json:"id,omitempty"`
	Path        string `json:"path"`
	Dir         string `json:"dir"`
	DisplayName string `json:"display_name,omitempty"`
}

type PackageInfo struct {
	CanonicalPath     string   `json:"canonical_package_path"`
	Name              string   `json:"name"`
	ModuleID          string   `json:"owning_module_id"`
	ModulePath        string   `json:"module_path"`
	Dir               string   `json:"package_directory"`
	ModuleRelativeDir string   `json:"module_relative_path"`
	DisplayPath       string   `json:"display_path"`
	Locality          string   `json:"locality"`
	Files             []string `json:"files,omitempty"`
}

// Component is a stable structured view of one model-oriented subsystem. Its
// anchors and relations are assembled locally from allowlisted repository
// evidence so the browser does not need to recover authority from prose.
type Component struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Role           componentmap.Role  `json:"role"`
	RoleBasis      evidence.Certainty `json:"role_basis"`
	ModelPurpose   string             `json:"model_purpose,omitempty"`
	AnchorGroups   []AnchorGroup      `json:"anchor_groups,omitempty"`
	RelatedFlowIDs []string           `json:"related_flow_ids,omitempty"`
	Packages       []string           `json:"packages,omitempty"`
	PrimaryPackage string             `json:"primary_package,omitempty"`
}

type AnchorGroup struct {
	ID             string              `json:"id"`
	Path           string              `json:"path"`
	Grounding      string              `json:"grounding"`
	Locations      []evidence.Location `json:"locations,omitempty"`
	ModelNotes     []string            `json:"model_notes,omitempty"`
	LocalContext   []SourceSignal      `json:"local_context,omitempty"`
	CanListSymbols bool                `json:"can_list_symbols"`
}

// SourceSignal is the presentation-safe subset of deterministic source
// scanning evidence kept with an anchor.
type SourceSignal struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Category string `json:"category,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type ComponentRelation struct {
	From      string             `json:"from"`
	To        string             `json:"to"`
	Kind      string             `json:"kind"`
	Certainty evidence.Certainty `json:"certainty"`
	Evidence  []EdgeInfo         `json:"evidence"`
}

// RunInfo contains safe, content-free facts about the model boundary used for
// this report. It intentionally excludes credentials, prompts, responses, and
// the provider endpoint.
type RunInfo struct {
	CreatedAt                    string `json:"created_at,omitempty"`
	Model                        string `json:"model,omitempty"`
	PromptVersion                string `json:"prompt_version,omitempty"`
	CompactContextBytes          int    `json:"compact_context_bytes,omitempty"`
	ExternalRequestBytes         int    `json:"external_request_bytes,omitempty"`
	ProviderRequestCount         int    `json:"provider_request_count,omitempty"`
	ProviderAccountingComplete   bool   `json:"provider_accounting_complete,omitempty"`
	CandidateDirectionCount      int    `json:"candidate_direction_count,omitempty"`
	ProposedDirectionCount       int    `json:"proposed_direction_count,omitempty"`
	AcceptedDirectionCount       int    `json:"accepted_direction_count,omitempty"`
	RejectedDirectionCount       int    `json:"rejected_direction_count,omitempty"`
	SavedFlowCount               int    `json:"saved_flow_count,omitempty"`
	ArchitectureAnchorCount      int    `json:"architecture_anchor_count,omitempty"`
	ProviderLatencyMillis        *int64 `json:"provider_latency_ms,omitempty"`
	SurfaceDiscoveryRan          bool   `json:"surface_discovery_ran,omitempty"`
	SurfaceDiscoveryCount        int    `json:"surface_discovery_count,omitempty"`
	SurfaceDiscoveryMillis       *int64 `json:"surface_discovery_ms,omitempty"`
	CLICommandSurfaceCount       int    `json:"cli_command_surface_count,omitempty"`
	GenericSurfaceCount          int    `json:"generic_surface_count,omitempty"`
	ApplicationSurfaceCount      int    `json:"application_surface_count,omitempty"`
	SecondaryServiceSurfaceCount int    `json:"secondary_service_surface_count,omitempty"`
	ToolingSurfaceCount          int    `json:"tooling_surface_count,omitempty"`
	TestHelperSurfaceCount       int    `json:"test_helper_surface_count,omitempty"`
	UnassignedSurfaceCount       int    `json:"unassigned_surface_count,omitempty"`
	UnavailableSurfaceCount      int    `json:"unavailable_surface_count,omitempty"`
	UnavailablePackageCount      int    `json:"unavailable_package_count,omitempty"`
	PackageDiagnosticCount       int    `json:"package_diagnostic_count,omitempty"`
	SupportingDependencyCount    int    `json:"supporting_dependency_surface_count,omitempty"`
	DependencyOnlySurfaceCount   int    `json:"dependency_only_surface_count,omitempty"`
	SuggestedInvestigationCount  int    `json:"suggested_investigation_count,omitempty"`
	DiscoveredSurfaceCount       int    `json:"discovered_surface_count,omitempty"`
	SavedTraceCount              int    `json:"saved_trace_count,omitempty"`
	CompleteTraceCount           int    `json:"complete_trace_count,omitempty"`
	PartialTraceCount            int    `json:"partial_trace_count,omitempty"`
	UnresolvedTraceCount         int    `json:"unresolved_trace_count,omitempty"`
	FailedTraceAttemptCount      int    `json:"failed_trace_attempt_count,omitempty"`
	EvidenceBundleCount          int    `json:"evidence_bundle_count,omitempty"`
}

// Subsystem is one grounded component from the orientation-stage system map.
type Subsystem struct {
	Name         string            `json:"name"`
	Role         componentmap.Role `json:"role"`
	Evidence     []string          `json:"evidence"`
	WhyItMatters string            `json:"why_it_matters"`
}

// DomainWord records repository vocabulary that helps a new reader interpret
// names in the source without presenting the model's guess as a verified fact.
type DomainWord struct {
	Word     string   `json:"word"`
	Guess    string   `json:"guess"`
	Evidence []string `json:"evidence"`
}

// CandidateDirection is the orientation-stage view of a flow that can be
// explored further. CandidateFlows remains in ReportData as a names-only
// compatibility view for existing report.json consumers.
type CandidateDirection struct {
	ID                string                        `json:"id"`
	Name              string                        `json:"name"`
	FlowType          string                        `json:"flow_type,omitempty"`
	Trigger           string                        `json:"trigger"`
	LikelyEntrypoint  string                        `json:"likely_entrypoint"`
	LikelyFiles       []string                      `json:"likely_files"`
	WhyInteresting    string                        `json:"why_interesting"`
	Evidence          []string                      `json:"evidence"`
	Confidence        float64                       `json:"confidence"`
	LocalVerification *flowexplain.FlowVerification `json:"local_verification,omitempty"`
	LocalProof        *flowproof.Session            `json:"local_proof,omitempty"`
	Disposition       string                        `json:"disposition"`
	DispositionReason string                        `json:"disposition_reason,omitempty"`
	CandidateBasis    string                        `json:"candidate_basis,omitempty"`
}

type FlowData struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	FlowType        string      `json:"flow_type,omitempty"`
	Summary         string      `json:"summary"`
	Confidence      float64     `json:"confidence"`
	LikelyChain     []ChainStep `json:"likely_chain"`
	FilesToRead     []FileItem  `json:"files_to_read_in_order"`
	TestsToRead     []FileItem  `json:"tests_to_read"`
	UnverifiedPaths []PathItem  `json:"unverified_paths"`
	Unknowns        []string    `json:"unknowns"`
	Warnings        []string    `json:"warnings"`
	BundleSummary   BundleStats `json:"bundle_summary"`
	Error           string      `json:"error,omitempty"`
	EvidenceOnly    bool        `json:"evidence_only,omitempty"`
	FlowStatus      string      `json:"flow_status,omitempty"`
	CandidateBasis  string      `json:"candidate_basis,omitempty"`

	ConfidenceLabel  string `json:"confidence_label,omitempty"`
	BundleStatsLabel string `json:"bundle_stats_label,omitempty"`

	BundleFiles    []FileItem `json:"bundle_files,omitempty"`
	BundleTests    []FileItem `json:"bundle_tests,omitempty"`
	BundleDocs     []FileItem `json:"bundle_docs,omitempty"`
	BundlePackages []string   `json:"bundle_packages,omitempty"`
	BundleEdges    []EdgeInfo `json:"bundle_edges,omitempty"`
	bundleSignals  []SourceSignal
}

type externalImportUsage struct {
	ImportPath  string
	UsedByCount int
}

type EdgeInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ChainStep struct {
	Step          int      `json:"step"`
	Name          string   `json:"name"`
	WhatHappens   string   `json:"what_happens"`
	EvidenceFiles []string `json:"evidence_files"`
	Confidence    float64  `json:"confidence"`
}

type FileItem struct {
	Path               string `json:"path"`
	Reason             string `json:"reason"`
	PresentationReason string `json:"presentation_reason,omitempty"`
	Priority           int    `json:"priority"`
}

type PathItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type BundleStats struct {
	SelectedFilesCount int `json:"selected_files_count"`
	SelectedTestsCount int `json:"selected_tests_count"`
	SelectedDocsCount  int `json:"selected_docs_count"`
	SelectedPkgsCount  int `json:"selected_packages_count"`
	RelatedEdgesCount  int `json:"related_edges_count"`
}

func confidenceLabel(c float64) string {
	switch {
	case c >= 0.7:
		return "strong"
	case c >= 0.4:
		return "medium"
	case c > 0:
		return "weak"
	default:
		return ""
	}
}

func bundleStatsLabel(bs BundleStats) string {
	return fmt.Sprintf("%d source, %d test, %d doc", bs.SelectedFilesCount, bs.SelectedTestsCount, bs.SelectedDocsCount)
}

func findBestFlow(flows []FlowData) string {
	if len(flows) == 0 {
		return ""
	}
	var best struct {
		id         string
		confidence float64
		fileCount  int
		hasData    bool
	}
	for i := range flows {
		f := &flows[i]
		if f.Error != "" || f.Summary == "" || f.EvidenceOnly {
			continue
		}
		if !best.hasData || f.Confidence > best.confidence || (f.Confidence == best.confidence && len(f.FilesToRead) > best.fileCount) {
			best.id = f.ID
			best.confidence = f.Confidence
			best.fileCount = len(f.FilesToRead)
			best.hasData = true
		}
	}
	return best.id
}

func enrich(data *ReportData) {
	data.FormatVersion = CurrentFormatVersion
	data.FlowCount = len(data.Flows)
	if topics, warning := projectFreshRepoTopics(data); warning != "" {
		data.Warnings = append(data.Warnings, warning)
	} else {
		data.UserTopics = topics
		for _, topic := range topics {
			for _, location := range topic.StartingSymbols {
				data.OpenablePaths = appendUniqueString(data.OpenablePaths, location.Path)
			}
		}
		sort.Strings(data.OpenablePaths)
	}
	acceptedDirections := 0
	rejectedDirections := 0
	flowTypes := make(map[string]string, len(data.CandidateDirections))
	dispositions := make(map[string]string, len(data.CandidateDirections))
	for i := range data.CandidateDirections {
		if data.CandidateDirections[i].Disposition == flowexplain.DirectionRejected {
			rejectedDirections++
		} else {
			acceptedDirections++
		}
		flowTypes[data.CandidateDirections[i].ID] = data.CandidateDirections[i].FlowType
		dispositions[data.CandidateDirections[i].ID] = data.CandidateDirections[i].Disposition
	}
	for i := range data.Flows {
		if data.Flows[i].FlowType == "" {
			data.Flows[i].FlowType = flowTypes[data.Flows[i].ID]
		}
		if dispositions[data.Flows[i].ID] == flowexplain.DirectionRejected {
			data.Flows[i].EvidenceOnly = true
		}
		data.Flows[i].ConfidenceLabel = confidenceLabel(data.Flows[i].Confidence)
		data.Flows[i].BundleStatsLabel = bundleStatsLabel(data.Flows[i].BundleSummary)
	}
	data.RecommendedFlow = findBestFlow(data.Flows)
	if data.Run != nil {
		data.Run.ProposedDirectionCount = len(data.CandidateDirections)
		data.Run.CandidateDirectionCount = len(data.CandidateDirections)
		data.Run.AcceptedDirectionCount = acceptedDirections
		data.Run.RejectedDirectionCount = rejectedDirections
		data.Run.SavedFlowCount = len(canonicalSavedTraceSessions(data.CandidateDirections))
		if data.ArchitectureGrounding != nil {
			data.Run.ArchitectureAnchorCount = len(data.ArchitectureGrounding.BehaviorAnchors)
		}
		refreshProductCounts(data)
	}
}

func projectFreshRepoTopics(data *ReportData) ([]UserTopic, string) {
	if data == nil || data.ArtifactsDir == "" {
		return nil, ""
	}

	var opportunity userTopicOpportunityArtifact
	var candidates userTopicCandidatesArtifact
	var status userTopicStatusArtifact
	for _, input := range []struct {
		name   string
		target any
	}{
		{freshRepoOpportunityFileForTopics, &opportunity},
		{freshRepoDemoCandidatesFileForTopics, &candidates},
		{freshRepoDemoStatusFileForTopics, &status},
	} {
		present, err := readBoundedUserTopicArtifact(
			filepath.Join(data.ArtifactsDir, input.name),
			input.target,
		)
		if err != nil {
			return nil, fmt.Sprintf("topic shelf unavailable: %v", err)
		}
		if !present {
			return nil, ""
		}
	}

	if opportunity.ValidationState != "accepted" ||
		len(opportunity.NormalizedProposal.Candidates) == 0 ||
		len(opportunity.NormalizedProposal.Candidates) > maxUserTopicOpportunityCandidates ||
		len(candidates.Selected) == 0 || len(candidates.Selected) > maxUserTopics ||
		len(status.Attempts) == 0 || len(status.Attempts) > maxUserTopics ||
		len(candidates.Selected) != len(status.Attempts) {
		return nil, "topic shelf unavailable: saved candidate collections are outside the projection contract"
	}

	opportunityByID := make(map[string]struct {
		Title    string
		Question string
	}, len(opportunity.NormalizedProposal.Candidates))
	for _, candidate := range opportunity.NormalizedProposal.Candidates {
		if !boundedUserTopicText(candidate.ID, maxUserTopicIDBytes) ||
			!boundedUserTopicText(candidate.Title, maxUserTopicTitleBytes) ||
			!boundedUserTopicText(candidate.QuestionAnswered, maxUserTopicQuestionBytes) {
			return nil, "topic shelf unavailable: opportunity metadata is invalid"
		}
		if _, exists := opportunityByID[candidate.ID]; exists {
			return nil, "topic shelf unavailable: opportunity candidate IDs are not unique"
		}
		opportunityByID[candidate.ID] = struct {
			Title    string
			Question string
		}{Title: candidate.Title, Question: candidate.QuestionAnswered}
	}

	selectedByID := make(map[string]int, len(candidates.Selected))
	for index, candidate := range candidates.Selected {
		if !boundedUserTopicText(candidate.CandidateID, maxUserTopicIDBytes) {
			return nil, "topic shelf unavailable: selected candidate ID is invalid"
		}
		if _, exists := selectedByID[candidate.CandidateID]; exists {
			return nil, "topic shelf unavailable: selected candidate IDs are not unique"
		}
		selectedByID[candidate.CandidateID] = index
	}

	topics := make([]UserTopic, 0, min(len(status.Attempts), maxUserTopics))
	seenAttempts := make(map[string]struct{}, len(status.Attempts))
	for _, attempt := range status.Attempts {
		if !boundedUserTopicText(attempt.CandidateID, maxUserTopicIDBytes) {
			return nil, "topic shelf unavailable: attempt candidate ID is invalid"
		}
		if _, exists := seenAttempts[attempt.CandidateID]; exists {
			return nil, "topic shelf unavailable: attempt candidate IDs are not unique"
		}
		seenAttempts[attempt.CandidateID] = struct{}{}

		opportunityCandidate, exists := opportunityByID[attempt.CandidateID]
		selectedIndex, selected := selectedByID[attempt.CandidateID]
		if !exists || !selected {
			return nil, "topic shelf unavailable: saved candidate IDs do not join uniquely"
		}
		selectedCandidate := candidates.Selected[selectedIndex]
		if selectedCandidate.Primary == nil ||
			selectedCandidate.Question != opportunityCandidate.Question ||
			attempt.Question != opportunityCandidate.Question {
			return nil, "topic shelf unavailable: saved candidate questions do not agree"
		}

		if attempt.State != "insufficient_primary_evidence" {
			continue
		}
		if attempt.FailureStage != "eligibility" ||
			attempt.PrimaryEligibility == nil ||
			selectedCandidate.Primary.Status != "insufficient_primary_evidence" ||
			!equalUserTopicEligibility(selectedCandidate.Primary.Eligibility, *attempt.PrimaryEligibility) {
			return nil, "topic shelf unavailable: rejected candidate eligibility does not agree"
		}

		uncertainty, ok := userTopicUncertainty(attempt.PrimaryEligibility.Reasons)
		if !ok {
			return nil, "topic shelf unavailable: rejected candidate reason is unsupported"
		}
		startingSymbols, ok := projectUserTopicSymbols(selectedCandidate.Primary)
		if !ok {
			return nil, "topic shelf unavailable: rejected candidate symbols are invalid"
		}
		topics = append(topics, UserTopic{
			CandidateID:     attempt.CandidateID,
			Title:           opportunityCandidate.Title,
			Question:        opportunityCandidate.Question,
			StartingSymbols: startingSymbols,
			Uncertainty:     uncertainty,
		})
	}
	if len(topics) == 0 {
		return nil, ""
	}
	return topics, ""
}

func readBoundedUserTopicArtifact(filePath string, target any) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", filepath.Base(filePath), err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("%s: %w", filepath.Base(filePath), err)
	}
	if info.Size() <= 0 || info.Size() > maxUserTopicArtifactBytes {
		return false, fmt.Errorf("%s exceeds the byte budget", filepath.Base(filePath))
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxUserTopicArtifactBytes+1))
	if err != nil {
		return false, fmt.Errorf("%s: %w", filepath.Base(filePath), err)
	}
	if len(raw) > maxUserTopicArtifactBytes {
		return false, fmt.Errorf("%s exceeds the byte budget", filepath.Base(filePath))
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, fmt.Errorf("%s is invalid JSON", filepath.Base(filePath))
	}
	return true, nil
}

func projectUserTopicSymbols(
	primary *struct {
		Status      string                `json:"status"`
		RootAnchors []userTopicRootAnchor `json:"root_anchors"`
		Eligibility userTopicEligibility  `json:"eligibility"`
		AnchorFacts []userTopicAnchorFact `json:"anchor_facts"`
	},
) ([]UserTopicSymbol, bool) {
	if primary == nil ||
		len(primary.Eligibility.DistinctSymbols) == 0 ||
		len(primary.Eligibility.DistinctSymbols) > maxUserTopicSymbols ||
		len(primary.RootAnchors) == 0 || len(primary.RootAnchors) > maxUserTopicAnchors ||
		len(primary.AnchorFacts) == 0 || len(primary.AnchorFacts) > maxUserTopicAnchors {
		return nil, false
	}
	factsByID := make(map[string]userTopicAnchorFact, len(primary.AnchorFacts))
	for _, fact := range primary.AnchorFacts {
		if fact.Source == nil || !boundedUserTopicText(fact.ID, maxUserTopicIDBytes) {
			return nil, false
		}
		if _, exists := factsByID[fact.ID]; exists {
			return nil, false
		}
		factsByID[fact.ID] = fact
	}
	anchorsBySymbol := make(map[string]userTopicRootAnchor, len(primary.RootAnchors))
	for _, anchor := range primary.RootAnchors {
		if !boundedUserTopicText(anchor.OriginFactID, maxUserTopicIDBytes) ||
			!validUserTopicPath(anchor.Path) ||
			!boundedUserTopicText(anchor.Symbol, maxUserTopicSymbolBytes) {
			return nil, false
		}
		key := anchor.Path + "\x00" + anchor.Symbol
		if _, exists := anchorsBySymbol[key]; exists {
			return nil, false
		}
		anchorsBySymbol[key] = anchor
	}

	result := make([]UserTopicSymbol, 0, min(len(primary.Eligibility.DistinctSymbols), maxUserTopicSymbols))
	seen := make(map[string]struct{}, len(primary.Eligibility.DistinctSymbols))
	for _, key := range primary.Eligibility.DistinctSymbols {
		if len(key) == 0 || len(key) > maxUserTopicPathBytes+maxUserTopicSymbolBytes+1 {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		sourcePath, symbol, found := strings.Cut(key, "\x00")
		if !found || !validUserTopicPath(sourcePath) ||
			!boundedUserTopicText(symbol, maxUserTopicSymbolBytes) {
			return nil, false
		}
		anchor, exists := anchorsBySymbol[key]
		if !exists {
			return nil, false
		}
		fact, exists := factsByID[anchor.OriginFactID]
		if !exists || fact.Source.Path != sourcePath ||
			fact.Source.EnclosingSymbol != symbol || fact.Source.StartLine <= 0 {
			return nil, false
		}
		result = append(result, UserTopicSymbol{
			Path: sourcePath, Symbol: symbol, Line: fact.Source.StartLine,
		})
	}
	return result, len(result) > 0
}

func equalUserTopicEligibility(left, right userTopicEligibility) bool {
	return left.Status == right.Status &&
		equalUserTopicStrings(left.Reasons, right.Reasons) &&
		equalUserTopicStrings(left.DistinctSymbols, right.DistinctSymbols)
}

func equalUserTopicStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func userTopicUncertainty(reasons []string) (string, bool) {
	if len(reasons) == 0 || len(reasons) > maxUserTopicReasons {
		return "", false
	}
	messages := make([]string, 0, len(reasons))
	seen := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		if !boundedUserTopicText(reason, maxUserTopicReasonBytes) {
			return "", false
		}
		if _, duplicate := seen[reason]; duplicate {
			return "", false
		}
		seen[reason] = struct{}{}
		switch reason {
		case "observable_effect_fact_missing":
			messages = append(messages, "The observable result is not yet supported by exact local evidence.")
		case "core_work_fact_missing":
			messages = append(messages, "The exact starting point is known, but exact source evidence does not yet establish the core behavior.")
		case "fewer_than_two_exact_symbols":
			messages = append(messages, "Only one exact starting symbol is available, so no ordered mechanism is claimed.")
		case "bounded_static_analysis_limit":
			messages = append(messages, "The local evidence stops before the remaining behavior can be established.")
		case "unresolved_dynamic_dispatch":
			messages = append(messages, "Exact local evidence is available, but dynamic dispatch prevents proving the next target.")
		case "proof_adapter_unavailable":
			messages = append(messages, "A complete proof adapter is not available for this language yet, so this remains an exact starting point rather than a claimed mechanism.")
		default:
			return "", false
		}
	}
	return strings.Join(messages, " "), true
}

func boundedUserTopicText(value string, limit int) bool {
	return len(value) > 0 && len(value) <= limit && strings.TrimSpace(value) == value
}

func validUserTopicPath(value string) bool {
	if !boundedUserTopicText(value, maxUserTopicPathBytes) ||
		path.IsAbs(value) || path.Clean(value) != value ||
		value == "." || value == ".." || strings.HasPrefix(value, "../") ||
		strings.Contains(value, `\`) {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func refreshProductCounts(data *ReportData) {
	if data == nil || data.Run == nil {
		return
	}
	data.Run.SuggestedInvestigationCount = 0
	data.Run.DiscoveredSurfaceCount = 0
	data.Run.SavedTraceCount = 0
	data.Run.CompleteTraceCount = 0
	data.Run.PartialTraceCount = 0
	data.Run.UnresolvedTraceCount = 0
	data.Run.FailedTraceAttemptCount = 0
	data.Run.EvidenceBundleCount = 0
	data.Run.CLICommandSurfaceCount = 0
	data.Run.GenericSurfaceCount = 0
	data.Run.ApplicationSurfaceCount = 0
	data.Run.SecondaryServiceSurfaceCount = 0
	data.Run.ToolingSurfaceCount = 0
	data.Run.TestHelperSurfaceCount = 0
	data.Run.UnassignedSurfaceCount = 0
	data.Run.UnavailableSurfaceCount = 0
	data.Run.UnavailablePackageCount = 0
	data.Run.PackageDiagnosticCount = 0
	data.Run.SupportingDependencyCount = 0
	data.Run.DependencyOnlySurfaceCount = 0
	traced := make(map[string]struct{})
	for _, session := range canonicalSavedTraceSessions(data.CandidateDirections) {
		traced[session.Proof.ID] = struct{}{}
		data.Run.SavedTraceCount++
		switch flowproof.AssessTraceQuality(session.Proof) {
		case flowproof.TraceQualityComplete:
			data.Run.CompleteTraceCount++
		case flowproof.TraceQualityPartial:
			data.Run.PartialTraceCount++
		default:
			data.Run.UnresolvedTraceCount++
		}
	}
	if data.ArchitectureCanvas != nil {
		data.Run.SuggestedInvestigationCount = len(data.ArchitectureCanvas.Suggestions)
	}
	if data.DiscoveredSurfaces != nil {
		data.Run.DiscoveredSurfaceCount = len(data.DiscoveredSurfaces.Triggers)
		data.Run.CLICommandSurfaceCount = data.DiscoveredSurfaces.CLICommandCount
		data.Run.GenericSurfaceCount = data.DiscoveredSurfaces.GenericSurfaceCount
		data.Run.ApplicationSurfaceCount = data.DiscoveredSurfaces.ApplicationCount
		data.Run.SecondaryServiceSurfaceCount = data.DiscoveredSurfaces.SecondaryServiceCount
		data.Run.ToolingSurfaceCount = data.DiscoveredSurfaces.ToolingCount
		data.Run.TestHelperSurfaceCount = data.DiscoveredSurfaces.TestHelperCount
		data.Run.UnassignedSurfaceCount = data.DiscoveredSurfaces.UnassignedCount
		data.Run.UnavailableSurfaceCount = data.DiscoveredSurfaces.UnavailableSurfaceCount
		data.Run.UnavailablePackageCount = data.DiscoveredSurfaces.UnavailablePackageCount
		data.Run.PackageDiagnosticCount = data.DiscoveredSurfaces.PackageDiagnosticCount
		data.Run.SupportingDependencyCount = data.DiscoveredSurfaces.SupportingDependencyCount
		data.Run.DependencyOnlySurfaceCount = data.DiscoveredSurfaces.DependencyOnlyCount
	}
	if data.ArchitectureCanvas == nil {
		for _, direction := range data.CandidateDirections {
			if direction.Disposition == flowexplain.DirectionRejected {
				continue
			}
			if _, saved := traced[direction.ID]; !saved {
				data.Run.SuggestedInvestigationCount++
			}
		}
	}
	for _, flow := range data.Flows {
		if flow.EvidenceOnly || flow.FlowStatus == "local_only" {
			data.Run.EvidenceBundleCount++
		}
		if flow.Error != "" {
			data.Run.FailedTraceAttemptCount++
		}
	}
}

func savedFlowArtifactCount(flows []FlowData) int {
	count := 0
	for _, flow := range flows {
		if !flow.EvidenceOnly && flow.Error == "" && flow.FlowStatus != "local_only" {
			count++
		}
	}
	return count
}

func canonicalSavedTraceSessions(directions []CandidateDirection) []flowproof.Session {
	result := make([]flowproof.Session, 0)
	seen := make(map[string]struct{})
	for _, direction := range directions {
		if direction.Disposition == flowexplain.DirectionRejected || direction.LocalProof == nil {
			continue
		}
		session, ok := flowproof.UpgradeSession(*direction.LocalProof)
		if !ok || session.Proof.ID == "" || session.Proof.ID != direction.ID {
			continue
		}
		if _, duplicate := seen[session.Proof.ID]; duplicate {
			continue
		}
		seen[session.Proof.ID] = struct{}{}
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Proof.ID < result[j].Proof.ID })
	return result
}
