package report

import "fmt"

type ReportData struct {
	FormatVersion int `json:"format_version"`

	RepoName            string               `json:"repo_name"`
	ProjectGuess        string               `json:"project_guess"`
	CandidateFlows      []string             `json:"candidate_flows"`
	CandidateDirections []CandidateDirection `json:"candidate_directions,omitempty"`
	Flows               []FlowData           `json:"flows"`
	ArtifactsDir        string               `json:"artifacts_dir"`
	Warnings            []string             `json:"warnings,omitempty"`

	RecommendedFlow string `json:"recommended_flow,omitempty"`
	FlowCount       int    `json:"flow_count"`
}

// CandidateDirection is the orientation-stage view of a flow that can be
// explored further. CandidateFlows remains in ReportData as a names-only
// compatibility view for existing report.json consumers.
type CandidateDirection struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Trigger          string   `json:"trigger"`
	LikelyEntrypoint string   `json:"likely_entrypoint"`
	LikelyFiles      []string `json:"likely_files"`
	WhyInteresting   string   `json:"why_interesting"`
	Evidence         []string `json:"evidence"`
	Confidence       float64  `json:"confidence"`
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
	data.FormatVersion = 3
	data.FlowCount = len(data.Flows)
	for i := range data.Flows {
		data.Flows[i].ConfidenceLabel = confidenceLabel(data.Flows[i].Confidence)
		data.Flows[i].BundleStatsLabel = bundleStatsLabel(data.Flows[i].BundleSummary)
	}
	data.RecommendedFlow = findBestFlow(data.Flows)
}
