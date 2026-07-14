package orient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/flowexplain"
)

func TestSelectFlowPrefersExplainedSeedAndPinsReport(t *testing.T) {
	t.Parallel()

	report := []byte(`{
  "repo_name":"etcd",
  "orientation":{"candidate_flows":[{"name":"HTTP/gRPC Put"}]},
  "explained_flows":[{"flow_seed":{"id":"http-grpc-put","name":"HTTP/gRPC Put","likely_entrypoint":"must.not.become.Symbol","valid_seed_files":["server/key.go"]},"flow_bundle_summary":{}}]
}`)
	selected, err := SelectFlow(report, "http-grpc-put")
	if err != nil {
		t.Fatal(err)
	}
	if selected.RepoName != "etcd" || selected.FlowName != "HTTP/gRPC Put" || len(selected.ReportSHA256) != 64 {
		t.Fatalf("selected = %#v", selected)
	}
	if strings.Contains(selected.FlowName, "must.not.become.Symbol") {
		t.Fatalf("entrypoint leaked into selection: %#v", selected)
	}
}

func TestSelectFlowUsesCandidateWhenNoExplanationExists(t *testing.T) {
	t.Parallel()

	selected, err := SelectFlow([]byte(`{
  "repo_name":"repo",
  "orientation":{"candidate_flows":[{"name":"Watch stream"}]},
  "explained_flows":[]
}`), "watch-stream")
	if err != nil {
		t.Fatal(err)
	}
	if selected.FlowName != "Watch stream" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectFlowUsesExplainedOfflineReportAndPinsExactBytes(t *testing.T) {
	t.Parallel()

	reportJSON, err := json.Marshal(combinedReport{
		RepoName: "repo",
		ExplainedFlows: []explainedFlow{
			{
				FlowSeed: flowexplain.FlowSeed{
					ID:               "watch-stream",
					Name:             "Watch stream",
					LikelyEntrypoint: "must.not.become.Symbol",
					ValidSeedFiles:   []string{"server/watch.go"},
				},
				FlowReport: json.RawMessage(`{"summary":"model prose must stay behind the handoff"}`),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	selected, err := SelectFlow(reportJSON, "watch-stream")
	if err != nil {
		t.Fatal(err)
	}
	wantSum := sha256.Sum256(reportJSON)
	if selected.ReportSHA256 != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("report sha256 = %q, want %q", selected.ReportSHA256, hex.EncodeToString(wantSum[:]))
	}
	if selected.RepoName != "repo" || selected.FlowID != "watch-stream" || selected.FlowName != "Watch stream" {
		t.Fatalf("selected = %#v", selected)
	}
	selectedJSON, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"must.not.become.Symbol", "server/watch.go", "model prose"} {
		if strings.Contains(string(selectedJSON), forbidden) {
			t.Fatalf("handoff contains excluded report detail %q: %s", forbidden, selectedJSON)
		}
	}
}

func TestSelectFlowRejectsMissingAndCollidingFlows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report string
		id     string
	}{
		{name: "missing", report: `{"repo_name":"repo","explained_flows":[]}`, id: "missing"},
		{name: "collision", report: `{
  "repo_name":"repo",
  "orientation":{"candidate_flows":[{"name":"Put flow"},{"name":"Put-flow"}]},
  "explained_flows":[]
}`, id: "put-flow"},
		{name: "conflicting explanation", report: `{
  "repo_name":"repo",
  "orientation":{"candidate_flows":[{"name":"Different name"}]},
  "explained_flows":[{"flow_seed":{"id":"different-name","name":"Original name"}}]
}`, id: "different-name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := SelectFlow([]byte(test.report), test.id); err == nil {
				t.Fatal("SelectFlow() error = nil")
			}
		})
	}
}
