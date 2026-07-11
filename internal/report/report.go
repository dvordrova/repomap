package report

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/flowproof"
)

type ReportData struct {
	FormatVersion int `json:"format_version"`

	RepoName                   string               `json:"repo_name"`
	ProjectGuess               string               `json:"project_guess"`
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
	ArchitectureCanvas         *ArchitectureCanvas  `json:"architecture_canvas,omitempty"`
	RepositoryGraph            *RepositoryGraph     `json:"repository_graph,omitempty"`

	RecommendedFlow string `json:"recommended_flow,omitempty"`
	FlowCount       int    `json:"flow_count"`
}

// RepositoryGraph is the bounded saved package projection used as exact
// structural input for conceptual architecture. It does not imply runtime
// execution order.
type RepositoryGraph struct {
	Modules      []ModuleInfo `json:"modules,omitempty"`
	PackageEdges []EdgeInfo   `json:"package_edges,omitempty"`
}

// ModuleInfo maps a repository-relative directory to its Go import root.
type ModuleInfo struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`
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
	ProviderLatencyMillis   *int64 `json:"provider_latency_ms,omitempty"`
}

// Subsystem is one grounded component from the orientation-stage system map.
type Subsystem struct {
	Name         string   `json:"name"`
	Evidence     []string `json:"evidence"`
	WhyItMatters string   `json:"why_it_matters"`
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
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Trigger          string             `json:"trigger"`
	LikelyEntrypoint string             `json:"likely_entrypoint"`
	LikelyFiles      []string           `json:"likely_files"`
	WhyInteresting   string             `json:"why_interesting"`
	Evidence         []string           `json:"evidence"`
	Confidence       float64            `json:"confidence"`
	LocalProof       *flowproof.Session `json:"local_proof,omitempty"`
}

type FlowData struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
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
		if f.Error != "" || f.Summary == "" {
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
	data.FormatVersion = 5
	data.FlowCount = len(data.Flows)
	for i := range data.Flows {
		data.Flows[i].ConfidenceLabel = confidenceLabel(data.Flows[i].Confidence)
		data.Flows[i].BundleStatsLabel = bundleStatsLabel(data.Flows[i].BundleSummary)
	}
	data.RecommendedFlow = findBestFlow(data.Flows)
}
