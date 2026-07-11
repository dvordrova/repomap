package orient

import (
	"testing"

	"github.com/dvordrova/repomap/internal/flowexplain"
)

func TestSelectTopFlows(t *testing.T) {
	flows := []flowexplain.CandidateFlow{
		{Name: "flow-a", Confidence: 0.3},
		{Name: "flow-b", Confidence: 0.9},
		{Name: "flow-c", Confidence: 0.5},
		{Name: "flow-d", Confidence: 0.7},
	}

	top := selectTopFlows(flows, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(top))
	}
	if top[0].Confidence < top[1].Confidence {
		t.Fatal("flows not sorted by confidence desc")
	}
	if top[0].Name != "flow-b" {
		t.Fatalf("top flow should be flow-b, got %s", top[0].Name)
	}
}

func TestFormatHumanReadable(t *testing.T) {
	report := combinedReport{
		RepoName: "etcd",
		Orientation: &orientationPart{
			ProjectGuess: "distributed key-value store",
		},
		ExplainedFlows: []explainedFlow{
			{
				FlowSeed: flowexplain.FlowSeed{
					Name: "gRPC Put request",
				},
				FlowBundleSummary: flowBundleSummary{
					SelectedFilesCount: 10,
					SelectedTestsCount: 3,
				},
			},
		},
	}

	text := formatHumanReadable(report, ".repomap-runs", "20260523-123456-etcd")
	if len(text) == 0 {
		t.Fatal("human readable output is empty")
	}
	if !contains(text, "gRPC Put request") {
		t.Fatal("should mention flow name")
	}
	if !contains(text, "Artifacts:") {
		t.Fatal("should mention artifacts path")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
