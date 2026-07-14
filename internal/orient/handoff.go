package orient

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
)

// SelectedFlow is the bounded orientation provenance accepted by an
// investigation handoff. It deliberately excludes model prose, paths, and any
// claim that an entrypoint package is an exact symbol.
type SelectedFlow struct {
	RepoName     string `json:"repo_name"`
	FlowID       string `json:"flow_id"`
	FlowName     string `json:"flow_name"`
	ReportSHA256 string `json:"report_sha256"`
}

// SelectFlow resolves one unambiguous flow from an exact combined orientation
// report without promoting entrypoint hints or model prose into symbol facts.
func SelectFlow(reportJSON []byte, flowID string) (SelectedFlow, error) {
	if strings.TrimSpace(flowID) == "" {
		return SelectedFlow{}, fmt.Errorf("orient handoff: flow id is required")
	}
	var report combinedReport
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return SelectedFlow{}, fmt.Errorf("orient handoff: decode report: %w", err)
	}
	if strings.TrimSpace(report.RepoName) == "" {
		return SelectedFlow{}, fmt.Errorf("orient handoff: report repository name is missing")
	}

	type candidate struct {
		name      string
		explained bool
		listed    bool
	}
	candidates := make(map[string]candidate)
	for _, item := range report.ExplainedFlows {
		id := strings.TrimSpace(item.FlowSeed.ID)
		name := strings.TrimSpace(item.FlowSeed.Name)
		if id == "" || name == "" {
			return SelectedFlow{}, fmt.Errorf("orient handoff: explained flow identity is incomplete")
		}
		existing, exists := candidates[id]
		if exists && (existing.explained || existing.name != name) {
			return SelectedFlow{}, fmt.Errorf("orient handoff: flow id %q is ambiguous", id)
		}
		existing.name = name
		existing.explained = true
		candidates[id] = existing
	}
	if report.Orientation != nil {
		for _, item := range report.Orientation.CandidateFlows {
			id := flowexplain.GenerateFlowID(item.Name)
			name := strings.TrimSpace(item.Name)
			if name == "" {
				return SelectedFlow{}, fmt.Errorf("orient handoff: candidate flow name is missing")
			}
			existing, exists := candidates[id]
			if exists && (existing.listed || existing.name != name) {
				return SelectedFlow{}, fmt.Errorf("orient handoff: flow id %q is ambiguous", id)
			}
			existing.name = name
			existing.listed = true
			candidates[id] = existing
		}
	}
	selected, ok := candidates[flowID]
	if !ok {
		return SelectedFlow{}, fmt.Errorf("orient handoff: flow id %q was not found", flowID)
	}
	return SelectedFlow{
		RepoName:     report.RepoName,
		FlowID:       flowID,
		FlowName:     selected.name,
		ReportSHA256: fmt.Sprintf("%x", sha256.Sum256(reportJSON)),
	}, nil
}
