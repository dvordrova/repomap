package report

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const CurrentFormatVersion = 26

type ReportData struct {
	FormatVersion int `json:"format_version"`

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
	Run                        *RunInfo             `json:"run,omitempty"`
	OpenablePaths              []string             `json:"openable_paths,omitempty"`
	// SourceIDs is an ephemeral server-rendered map from authorized
	// repository-relative paths to opaque navigation IDs. WriteReportJSON
	// deliberately excludes it from persisted report evidence.
	SourceIDs          map[string]string            `json:"source_ids,omitempty"`
	RepositoryGraph    *RepositoryGraph             `json:"repository_graph,omitempty"`
	Components         []Component                  `json:"components,omitempty"`
	ComponentRelations []ComponentRelation          `json:"component_relations,omitempty"`
	ArchitectureCanvas *ArchitectureCanvas          `json:"architecture_canvas,omitempty"`
	GuidedTour         *guidedtour.Story            `json:"guided_tour,omitempty"`
	SemanticArtifacts  []semanticdiscovery.Artifact `json:"semantic_artifacts,omitempty"`
	// UserMechanisms is a presentation-only supported slice of independently
	// replayed canonical Mechanisms. Raw artifacts remain available for replay
	// and provenance, but default onboarding renders this narrower projection.
	UserMechanisms []UserMechanism `json:"user_mechanisms,omitempty"`
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
	SemanticSupplementalFacts []semanticdiscovery.Fact     `json:"-"`
	SemanticSearchDisabled    bool                         `json:"semantic_search_disabled,omitempty"`
	SemanticSearch            *SemanticSearchIndex         `json:"semantic_search,omitempty"`
	ArchitectureSynthesis     *ArchitectureSynthesisStatus `json:"architecture_synthesis,omitempty"`
	ArchitectureGrounding     *ArchitectureGrounding       `json:"architecture_grounding,omitempty"`
	ModelResearch             *modelresearch.State         `json:"model_research,omitempty"`
	DiscoveredSurfaces        *DiscoveredSurfaces          `json:"discovered_surfaces,omitempty"`
	CommandTraces             []gofacts.CommandTrace       `json:"command_traces,omitempty"`
	Freshness                 *freshness.FreshnessResult   `json:"freshness,omitempty"`
	CapturedRevision          string                       `json:"captured_revision,omitempty"`
	CapturedInputCount        int                          `json:"captured_input_count,omitempty"`
	RepositorySubmodules      []freshness.SubmoduleState   `json:"repository_submodules,omitempty"`
	evidenceLocations         []evidence.Location
	sourceSignals             []SourceSignal
	studyDocumentSourceRoot   string
	externalImports           []externalImportUsage
	semanticAttempted         int
	semanticInvestigated      int

	RecommendedFlow string `json:"recommended_flow,omitempty"`
	FlowCount       int    `json:"flow_count"`
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
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
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
