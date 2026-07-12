package report

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
)

const CurrentFormatVersion = 14

type ReportData struct {
	FormatVersion int `json:"format_version"`

	RepoName                   string                       `json:"repo_name"`
	ProjectGuess               string                       `json:"project_guess"`
	OrientationConfidence      float64                      `json:"orientation_confidence"`
	HighLevelMap               []Subsystem                  `json:"high_level_map,omitempty"`
	FirstFilesToOpen           []FileItem                   `json:"first_files_to_open,omitempty"`
	CandidateFlows             []string                     `json:"candidate_flows"`
	CandidateDirections        []CandidateDirection         `json:"candidate_directions,omitempty"`
	ImportantDomainWords       []DomainWord                 `json:"important_domain_words,omitempty"`
	QuestionsForHuman          []string                     `json:"questions_for_human,omitempty"`
	OrientationUnverifiedPaths []PathItem                   `json:"unverified_paths,omitempty"`
	Flows                      []FlowData                   `json:"flows"`
	ArtifactsDir               string                       `json:"artifacts_dir"`
	FeedbackPath               string                       `json:"feedback_path,omitempty"`
	Warnings                   []string                     `json:"warnings,omitempty"`
	Run                        *RunInfo                     `json:"run,omitempty"`
	OpenablePaths              []string                     `json:"openable_paths,omitempty"`
	RepositoryGraph            *RepositoryGraph             `json:"repository_graph,omitempty"`
	Components                 []Component                  `json:"components,omitempty"`
	ComponentRelations         []ComponentRelation          `json:"component_relations,omitempty"`
	ArchitectureCanvas         *ArchitectureCanvas          `json:"architecture_canvas,omitempty"`
	ArchitectureSynthesis      *ArchitectureSynthesisStatus `json:"architecture_synthesis,omitempty"`
	ArchitectureGrounding      *ArchitectureGrounding       `json:"architecture_grounding,omitempty"`
	DiscoveredSurfaces         *DiscoveredSurfaces          `json:"discovered_surfaces,omitempty"`
	evidenceLocations          []evidence.Location
	sourceSignals              []SourceSignal

	RecommendedFlow string `json:"recommended_flow,omitempty"`
	FlowCount       int    `json:"flow_count"`
}

// RepositoryGraph is a bounded deterministic projection of local repository
// facts used to connect model-suggested components without asking the model to
// invent relationships between them.
type RepositoryGraph struct {
	Modules      []ModuleInfo `json:"modules,omitempty"`
	PackageEdges []EdgeInfo   `json:"package_edges,omitempty"`
}

// ModuleInfo maps repository-relative directories to import-path roots. The
// longest matching directory owns a file in repositories with nested modules.
type ModuleInfo struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`
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
	CreatedAt               string `json:"created_at,omitempty"`
	Model                   string `json:"model,omitempty"`
	PromptVersion           string `json:"prompt_version,omitempty"`
	CompactContextBytes     int    `json:"compact_context_bytes,omitempty"`
	ExternalRequestBytes    int    `json:"external_request_bytes,omitempty"`
	ProviderRequestCount    int    `json:"provider_request_count,omitempty"`
	CandidateDirectionCount int    `json:"candidate_direction_count,omitempty"`
	ProposedDirectionCount  int    `json:"proposed_direction_count,omitempty"`
	AcceptedDirectionCount  int    `json:"accepted_direction_count,omitempty"`
	RejectedDirectionCount  int    `json:"rejected_direction_count,omitempty"`
	SavedFlowCount          int    `json:"saved_flow_count,omitempty"`
	ArchitectureAnchorCount int    `json:"architecture_anchor_count,omitempty"`
	ProviderLatencyMillis   *int64 `json:"provider_latency_ms,omitempty"`
	SurfaceDiscoveryRan     bool   `json:"surface_discovery_ran,omitempty"`
	SurfaceDiscoveryCount   int    `json:"surface_discovery_count,omitempty"`
	SurfaceDiscoveryMillis  *int64 `json:"surface_discovery_ms,omitempty"`
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

	ConfidenceLabel  string `json:"confidence_label,omitempty"`
	BundleStatsLabel string `json:"bundle_stats_label,omitempty"`

	BundleFiles    []FileItem `json:"bundle_files,omitempty"`
	BundleTests    []FileItem `json:"bundle_tests,omitempty"`
	BundleDocs     []FileItem `json:"bundle_docs,omitempty"`
	BundlePackages []string   `json:"bundle_packages,omitempty"`
	BundleEdges    []EdgeInfo `json:"bundle_edges,omitempty"`
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
		data.Run.SavedFlowCount = len(data.Flows)
		if data.ArchitectureGrounding != nil {
			data.Run.ArchitectureAnchorCount = len(data.ArchitectureGrounding.BehaviorAnchors)
		}
	}
}
