package report

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
)

// TestAtlasStudyReportStatusAlwaysSerializesFourFlags is the D211 HOLD-repair
// regression test: the four independent coverage flags are part of the
// documented report projection v6 contract and must serialize even when
// false. report.json consumers (the Study diagnostics UI and the manifest
// validator) depend on the keys being present.
func TestAtlasStudyReportStatusAlwaysSerializesFourFlags(t *testing.T) {
	cases := []struct {
		name  string
		flags []bool
	}{
		{"all false", []bool{false, false, false, false}},
		{"all true", []bool{true, true, true, true}},
		{"mixed", []bool{true, false, true, false}},
	}
	keys := []string{
		"frontier_complete",
		"selected_items_complete",
		"support_coverage_complete",
		"portfolio_target_met",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := AtlasStudyReportStatus{
				Version: atlasstudy.ResultVersion, ProjectionVersion: AtlasStudyReportProjectionVersion,
				State:                   atlasstudy.ProductStateAccepted,
				DirectionCount:          2,
				FrontierComplete:        tc.flags[0],
				SelectedItemsComplete:   tc.flags[1],
				SupportCoverageComplete: tc.flags[2],
				PortfolioTargetMet:      tc.flags[3],
			}
			raw, err := json.Marshal(status)
			if err != nil {
				t.Fatalf("marshal status: %v", err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatalf("unmarshal status: %v\n%s", err, raw)
			}
			for index, key := range keys {
				value, ok := object[key]
				if !ok {
					t.Fatalf("status JSON missing %q (flags %v):\n%s", key, tc.flags, raw)
				}
				var flag bool
				if err := json.Unmarshal(value, &flag); err != nil {
					t.Fatalf("status JSON flag %q is not a boolean: %v", key, err)
				}
				if flag != tc.flags[index] {
					t.Fatalf("status JSON flag %q = %v, want %v", key, flag, tc.flags[index])
				}
			}
		})
	}
}
