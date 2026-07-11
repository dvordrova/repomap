package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dvordrova/repomap/internal/flowexplain"
)

type snapshotJSON struct {
	RepoName string `json:"repo_name"`
}

type orientationReportJSON struct {
	ProjectGuess   string                     `json:"project_guess"`
	CandidateFlows []orientationCandidateJSON `json:"candidate_flows"`
	Warnings       []string                   `json:"warnings"`
}

type orientationCandidateJSON struct {
	Name string `json:"name"`
}

type flowReportJSON struct {
	Summary            string             `json:"summary"`
	Confidence         float64            `json:"confidence"`
	FlowName           string             `json:"flow_name"`
	LikelyChain        []chainStepJSON    `json:"likely_chain"`
	FilesToReadInOrder []fileItemJSON     `json:"files_to_read_in_order"`
	TestsToRead        []fileItemJSON     `json:"tests_to_read"`
	UnverifiedPaths    []pathItemJSON     `json:"unverified_paths"`
	Unknowns           []string           `json:"unknowns"`
	Warnings           []string           `json:"warnings"`
}

type chainStepJSON struct {
	Step          int      `json:"step"`
	Name          string   `json:"name"`
	WhatHappens   string   `json:"what_happens"`
	EvidenceFiles []string `json:"evidence_files"`
	Confidence    float64  `json:"confidence"`
}

type fileItemJSON struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

type pathItemJSON struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func ReadRunDir(runDir string) (*ReportData, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, fmt.Errorf("resolve run dir: %w", err)
	}
	data := &ReportData{ArtifactsDir: absDir}
	var parseWarnings []string

	if w := parseSnapshot(filepath.Join(absDir, "snapshot.json"), data); w != "" {
		parseWarnings = append(parseWarnings, w)
	}
	if w := parseOrientationReport(filepath.Join(absDir, "orientation_report.json"), data); w != "" {
		parseWarnings = append(parseWarnings, w)
	}

	flowWarnings, err := parseFlows(filepath.Join(absDir, "flows"), data)
	if err != nil {
		return nil, fmt.Errorf("read flows from %s: %w", absDir, err)
	}
	parseWarnings = append(parseWarnings, flowWarnings...)

	enrich(data)

	sort.Slice(data.Flows, func(i, j int) bool {
		if data.Flows[i].ID == data.RecommendedFlow {
			return true
		}
		if data.Flows[j].ID == data.RecommendedFlow {
			return false
		}
		return data.Flows[i].ID < data.Flows[j].ID
	})

	data.Warnings = append(data.Warnings, parseWarnings...)
	return data, nil
}

func parseSnapshot(path string, data *ReportData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("snapshot: %v", err)
	}
	var snap snapshotJSON
	if err := json.Unmarshal(b, &snap); err != nil {
		return fmt.Sprintf("snapshot unmarshal: %v", err)
	}
	data.RepoName = snap.RepoName
	return ""
}

func parseOrientationReport(path string, data *ReportData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("orientation: %v", err)
	}
	var or orientationReportJSON
	if err := json.Unmarshal(b, &or); err != nil {
		return fmt.Sprintf("orientation unmarshal: %v", err)
	}
	data.ProjectGuess = or.ProjectGuess
	for _, cf := range or.CandidateFlows {
		data.CandidateFlows = append(data.CandidateFlows, cf.Name)
	}
	data.Warnings = append(data.Warnings, or.Warnings...)
	return ""
}

func parseFlows(flowsDir string, data *ReportData) ([]string, error) {
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var warnings []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		fd := FlowData{ID: e.Name()}
		flowDir := filepath.Join(flowsDir, e.Name())

		if w := parseFlowBundle(filepath.Join(flowDir, "flow_bundle.json"), &fd); w != "" {
			warnings = append(warnings, fmt.Sprintf("flow %s: %s", fd.ID, w))
		}
		if w := parseFlowReport(filepath.Join(flowDir, "flow_report.json"), &fd); w != "" {
			warnings = append(warnings, fmt.Sprintf("flow %s: %s", fd.ID, w))
		}

		data.Flows = append(data.Flows, fd)
	}
	return warnings, nil
}

func parseFlowBundle(path string, fd *FlowData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("bundle: %v", err)
	}
	var fb flowexplain.FlowBundle
	if err := json.Unmarshal(b, &fb); err != nil {
		return fmt.Sprintf("bundle unmarshal: %v", err)
	}
	fd.BundleSummary.SelectedFilesCount = len(fb.SelectedFiles)
	fd.BundleSummary.SelectedTestsCount = len(fb.SelectedTests)
	fd.BundleSummary.SelectedDocsCount = len(fb.SelectedDocs)
	fd.BundleSummary.SelectedPkgsCount = len(fb.SelectedPackages)
	fd.BundleSummary.RelatedEdgesCount = len(fb.RelatedEdges)
	if fb.FlowSeed.Name != "" {
		fd.Name = fb.FlowSeed.Name
	}
	return ""
}

func parseFlowReport(path string, fd *FlowData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		fd.Error = fmt.Sprintf("cannot read flow report: %v", err)
		return fd.Error
	}
	if len(b) == 0 {
		fd.Error = "flow report is empty"
		return fd.Error
	}
	var fr flowReportJSON
	if err := json.Unmarshal(b, &fr); err != nil {
		fd.Error = fmt.Sprintf("invalid flow report JSON: %v", err)
		return fd.Error
	}
	for _, fi := range fr.FilesToReadInOrder {
		fd.FilesToRead = append(fd.FilesToRead, FileItem{
			Path:     fi.Path,
			Reason:   fi.Reason,
			Priority: fi.Priority,
		})
	}
	for _, ti := range fr.TestsToRead {
		fd.TestsToRead = append(fd.TestsToRead, FileItem{
			Path:   ti.Path,
			Reason: ti.Reason,
		})
	}
	for _, cs := range fr.LikelyChain {
		fd.LikelyChain = append(fd.LikelyChain, ChainStep{
			Step:          cs.Step,
			Name:          cs.Name,
			WhatHappens:   cs.WhatHappens,
			EvidenceFiles: cs.EvidenceFiles,
			Confidence:    cs.Confidence,
		})
	}
	for _, up := range fr.UnverifiedPaths {
		fd.UnverifiedPaths = append(fd.UnverifiedPaths, PathItem{
			Path:   up.Path,
			Reason: up.Reason,
		})
	}
	if fr.FlowName != "" {
		fd.Name = fr.FlowName
	}
	fd.Summary = fr.Summary
	fd.Confidence = fr.Confidence
	fd.Unknowns = fr.Unknowns
	fd.Warnings = fr.Warnings
	return ""
}
