package orient

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/sourcesignals"
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

func TestValidateOutboundBundlesRejectCredentials(t *testing.T) {
	t.Parallel()

	if err := validateOrientationBundleForRemote(llmbundle.Bundle{
		ReadmeExcerpt: `export API_KEY="company-secret-value"`,
	}); err == nil || !contains(err.Error(), "readme excerpt") {
		t.Fatalf("readme validation error = %v", err)
	}
	if err := validateOrientationBundleForRemote(llmbundle.Bundle{
		SourceSignals: []sourcesignals.Signal{{
			Path:    "server/main.go",
			Line:    12,
			Snippet: `token := "company-secret-value"`,
		}},
	}); err == nil || !contains(err.Error(), "server/main.go:12") {
		t.Fatalf("source signal validation error = %v", err)
	}
	if err := validateFlowBundleForRemote(flowexplain.FlowBundle{
		FlowSeed: flowexplain.FlowSeed{Name: "startup"},
		SourceSignals: []sourcesignals.Signal{{
			Path:    "server/main.go",
			Line:    12,
			Snippet: `password = "company-secret-value"`,
		}},
	}); err == nil || !contains(err.Error(), "startup") {
		t.Fatalf("flow validation error = %v", err)
	}
	if err := validateProviderOutputForStorage("orientation", []byte(`{"summary":"Bearer company-secret-token-value"}`)); err == nil || !contains(err.Error(), "refusing to retain") {
		t.Fatalf("provider output validation error = %v", err)
	}
}

func TestFormatHumanReadable(t *testing.T) {
	report := combinedReport{
		RepoName: "etcd",
		Orientation: &orientationPart{
			ProjectGuess: "distributed key-value store",
			Confidence:   0.42,
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
	if !contains(text, "Confidence: 42%") {
		t.Fatalf("should preserve model confidence, got:\n%s", text)
	}
	if !contains(text, "Artifacts:") {
		t.Fatal("should mention artifacts path")
	}
}

func TestValidateOrientation(t *testing.T) {
	t.Parallel()

	valid := orientationPart{
		ProjectGuess: "distributed store",
		Confidence:   0.7,
		FirstFilesToOpen: []fileToOpen{{
			Path:   "server/main.go",
			Reason: "entrypoint",
		}},
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "server startup",
			Trigger:          "process starts",
			LikelyEntrypoint: "server/main.go",
			LikelyFiles:      []string{"server/main.go"},
			Evidence:         []string{"server/main.go is an entrypoint"},
			Confidence:       0.6,
		}},
		UnverifiedPaths: []unverifiedPath{{Path: "server/maybe.go", Reason: "suspected"}},
	}
	allowed := []string{"server/main.go"}

	tests := []struct {
		name   string
		mutate func(*orientationPart)
		want   string
	}{
		{
			name: "valid",
		},
		{
			name: "invented first file",
			mutate: func(report *orientationPart) {
				report.FirstFilesToOpen[0].Path = "server/invented.go"
			},
			want: "outside allowed_paths",
		},
		{
			name: "invented flow file",
			mutate: func(report *orientationPart) {
				report.CandidateFlows[0].LikelyFiles[0] = "server/invented.go"
			},
			want: "outside allowed_paths",
		},
		{
			name: "path traversal",
			mutate: func(report *orientationPart) {
				report.UnverifiedPaths[0].Path = "../secret"
			},
			want: "invalid path",
		},
		{
			name: "confidence",
			mutate: func(report *orientationPart) {
				report.Confidence = 1.1
			},
			want: "outside [0,1]",
		},
		{
			name: "empty evidence",
			mutate: func(report *orientationPart) {
				report.CandidateFlows[0].Evidence = nil
			},
			want: "has no evidence",
		},
		{
			name: "invented flow evidence path",
			mutate: func(report *orientationPart) {
				report.CandidateFlows[0].Evidence = []string{"invented.go handles payments"}
			},
			want: "path-like evidence outside allowed_paths",
		},
		{
			name: "invented high level evidence path",
			mutate: func(report *orientationPart) {
				report.HighLevelMap = []orientationMapItem{{Name: "billing", Evidence: []string{"internal/billing/pay.go"}}}
			},
			want: "path-like evidence outside allowed_paths",
		},
		{
			name: "invented domain evidence path",
			mutate: func(report *orientationPart) {
				report.ImportantDomainWords = []orientationDomainWord{{Word: "invoice", Evidence: []string{"../private"}}}
			},
			want: "invalid path-like evidence",
		},
		{
			name: "invented package entrypoint",
			mutate: func(report *orientationPart) {
				report.CandidateFlows[0].LikelyEntrypoint = "example.invalid/invented"
			},
			want: "not a provided path or package",
		},
		{
			name: "absolute entrypoint",
			mutate: func(report *orientationPart) {
				report.CandidateFlows[0].LikelyEntrypoint = "/etc/passwd"
			},
			want: "not a provided path or package",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report := valid
			report.FirstFilesToOpen = append([]fileToOpen{}, valid.FirstFilesToOpen...)
			report.CandidateFlows = append([]flowexplain.CandidateFlow{}, valid.CandidateFlows...)
			report.CandidateFlows[0].LikelyFiles = append([]string{}, valid.CandidateFlows[0].LikelyFiles...)
			report.UnverifiedPaths = append([]unverifiedPath{}, valid.UnverifiedPaths...)
			if test.mutate != nil {
				test.mutate(&report)
			}
			err := validateOrientation(report, allowed, nil)
			if test.want == "" {
				if err != nil {
					t.Fatalf("validateOrientation() error = %v", err)
				}
				return
			}
			if err == nil || !contains(err.Error(), test.want) {
				t.Fatalf("validateOrientation() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEvidencePathMentions(t *testing.T) {
	t.Parallel()

	statement := "server/main.go:12 and ../private support this; " +
		"GET /metrics and /v1/metrics are HTTP routes; " +
		"/api/v1/config.yaml, /v1/private.go, and /etc/passwd are files; " +
		"import go.etcd.io/etcd/server/v3 is metadata"
	paths := evidencePathMentions(statement)
	want := map[string]bool{
		"server/main.go":      true,
		"../private":          true,
		"/api/v1/config.yaml": true,
		"/v1/private.go":      true,
		"/etc/passwd":         true,
	}
	if len(paths) != len(want) {
		t.Fatalf("evidencePathMentions() = %q", paths)
	}
	for _, path := range paths {
		if !want[path] {
			t.Fatalf("evidencePathMentions() = %q, unexpected %q", paths, path)
		}
	}
}

func TestValidateOrientationAllowsHTTPRouteEvidence(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "service",
		Confidence:   0.7,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "metrics request",
			Trigger:          "GET /v1/metrics",
			LikelyEntrypoint: "server/main.go",
			LikelyFiles:      []string{"server/main.go"},
			Evidence:         []string{"server/main.go handles GET /v1/metrics"},
			Confidence:       0.6,
		}},
	}
	if err := validateOrientation(report, []string{"server/main.go"}, nil); err != nil {
		t.Fatalf("validateOrientation() error = %v", err)
	}
}

func TestValidateOrientationAllowsProvidedPackageEntrypoint(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "service",
		Confidence:   0.7,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "server startup",
			Trigger:          "process starts",
			LikelyEntrypoint: "example.com/project/cmd/server",
			LikelyFiles:      []string{"cmd/server/main.go"},
			Evidence:         []string{"entrypoint package and file are in the bundle"},
			Confidence:       0.6,
		}},
	}
	if err := validateOrientation(
		report,
		[]string{"cmd/server/main.go"},
		[]string{"example.com/project/cmd/server"},
	); err != nil {
		t.Fatalf("validateOrientation() error = %v", err)
	}
}

func TestParseOrientationRepairsSafeDriftWithWarnings(t *testing.T) {
	t.Parallel()

	report, err := parseOrientation([]byte(`{
		"project_guess":"service",
		"confidence":0.7,
		"high_level_map":[{"name":"runtime","evidence":["internal/runtime/*"],"why_it_matters":"core"}],
		"first_files_to_open":["server/main.go"],
		"candidate_flows":[{
			"name":"startup",
			"trigger":"process starts",
			"likely_entrypoint":"server/main.go",
			"likely_files":["server/main.go"],
			"why_interesting":"entrypoint",
			"evidence":["server/main.go is provided"],
			"confidence":0.6
		}],
		"important_domain_words":[],
		"questions_for_human":[],
		"unverified_paths":["server/maybe.go"],
		"warnings":[],
		"extra_provider_field":"ignored"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOrientation(report, []string{"server/main.go"}, nil); err != nil {
		t.Fatal(err)
	}
	joinedWarnings := strings.Join(report.Warnings, "\n")
	for _, want := range []string{"first_files_to_open", "unverified_paths", "extra_provider_field", "wildcard evidence"} {
		if !strings.Contains(joinedWarnings, want) {
			t.Fatalf("warnings = %q, want %q", report.Warnings, want)
		}
	}
}

func TestK6QualityOrientationMatchesProductContract(t *testing.T) {
	t.Parallel()

	response, err := os.ReadFile("../quality/testdata/k6-metrics-v1/orientation_response.json")
	if err != nil {
		t.Fatal(err)
	}
	contextData, err := os.ReadFile("../quality/testdata/k6-metrics-v1/orientation_context.json")
	if err != nil {
		t.Fatal(err)
	}
	var groundingContext struct {
		AllowedPaths []string `json:"allowed_paths"`
	}
	if err := json.Unmarshal(contextData, &groundingContext); err != nil {
		t.Fatal(err)
	}
	report, err := parseOrientation(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOrientation(report, groundingContext.AllowedPaths, nil); err != nil {
		t.Fatalf("validateOrientation() error = %v", err)
	}
	if !strings.Contains(strings.Join(report.Warnings, "\n"), "wildcard evidence") {
		t.Fatalf("parser warnings = %q, want wildcard evidence warning", report.Warnings)
	}
}

func TestValidateFlowReport(t *testing.T) {
	t.Parallel()

	bundle := flowexplain.FlowBundle{
		FlowSeed: flowexplain.FlowSeed{
			Name:           "server startup",
			ValidSeedFiles: []string{"server/main.go", "server/main_test.go"},
		},
	}
	tests := []struct {
		name   string
		report string
		want   string
	}{
		{
			name: "object and string lists",
			report: `{
				"summary":"startup",
				"confidence":0.6,
				"files_to_read_in_order":[{"path":"server/main.go","reason":"entry"}],
				"tests_to_read":["server/main_test.go"],
				"likely_chain":[{"step":"Step 1","what_happens":"server starts","file":"server/main.go","evidence_files":["server/main.go"],"confidence":0.5}],
				"unverified_paths":[{"path":"server/maybe.go","reason":"suspected"}],
				"unknowns":[],
				"warnings":[]
			}`,
		},
		{
			name:   "invented file",
			report: `{"summary":"startup","confidence":0.5,"files_to_read_in_order":["server/invented.go"],"tests_to_read":[],"likely_chain":[],"unknowns":[],"warnings":[]}`,
			want:   "unprovided path",
		},
		{
			name:   "invented chain evidence",
			report: `{"summary":"startup","confidence":0.5,"files_to_read_in_order":["server/main.go"],"tests_to_read":[],"likely_chain":[{"what_happens":"starts","evidence_files":["server/invented.go"]}],"unknowns":[],"warnings":[]}`,
			want:   "unprovided path",
		},
		{
			name:   "unverified traversal",
			report: `{"summary":"startup","confidence":0.5,"files_to_read_in_order":["server/main.go"],"tests_to_read":[],"likely_chain":[],"unverified_paths":["../secret"],"unknowns":[],"warnings":[]}`,
			want:   "invalid path",
		},
		{
			name:   "invalid confidence",
			report: `{"summary":"startup","confidence":2,"files_to_read_in_order":["server/main.go"],"tests_to_read":[],"likely_chain":[],"unknowns":[],"warnings":[]}`,
			want:   "within [0,1]",
		},
		{
			name:   "empty object",
			report: `{}`,
			want:   "required field",
		},
		{
			name:   "non test path",
			report: `{"summary":"startup","confidence":0.5,"files_to_read_in_order":["server/main.go"],"tests_to_read":["server/main.go"],"likely_chain":[],"unknowns":[],"warnings":[]}`,
			want:   "not a Go test file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateFlowReport([]byte(test.report), bundle)
			if test.want == "" {
				if err != nil {
					t.Fatalf("validateFlowReport() error = %v", err)
				}
				return
			}
			if err == nil || !contains(err.Error(), test.want) {
				t.Fatalf("validateFlowReport() error = %v, want %q", err, test.want)
			}
		})
	}

	normalized, err := normalizeFlowReport([]byte(`{
		"summary":"startup",
		"confidence":0.5,
		"files_to_read_in_order":["server/main.go"],
		"tests_to_read":[],
		"likely_chain":[],
		"unknowns":[],
		"warnings":[],
		"dangerous_path":"/etc/passwd"
	}`), bundle)
	if err != nil {
		t.Fatalf("normalizeFlowReport() error = %v", err)
	}
	var normalizedFields map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &normalizedFields); err != nil {
		t.Fatal(err)
	}
	if _, retained := normalizedFields["dangerous_path"]; retained {
		t.Fatal("normalized report retained an unknown path-bearing field")
	}
	if !contains(string(normalizedFields["warnings"]), "dangerous_path") {
		t.Fatal("normalized report did not warn about ignored unknown field")
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
