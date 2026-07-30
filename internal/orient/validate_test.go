package orient

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
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
			name: "colliding direction ids",
			mutate: func(report *orientationPart) {
				duplicate := report.CandidateFlows[0]
				duplicate.Name = "server-startup"
				report.CandidateFlows = append(report.CandidateFlows, duplicate)
			},
			want: "collides",
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
		"high_level_map":[{"name":"runtime","role":"made_up","evidence":["internal/runtime/*"],"why_it_matters":"core"}],
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
	for _, want := range []string{"first_files_to_open", "unverified_paths", "extra_provider_field", "wildcard evidence", "high_level_map[0].role"} {
		if !strings.Contains(joinedWarnings, want) {
			t.Fatalf("warnings = %q, want %q", report.Warnings, want)
		}
	}
	if report.HighLevelMap[0].Role != componentmap.RoleUnknown {
		t.Fatalf("role = %q, want unknown", report.HighLevelMap[0].Role)
	}
}

func TestNormalizeOrientationGroundingDropsProseDriftAndRepairsEntrypoint(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "metrics service",
		Confidence:   0.8,
		FirstFilesToOpen: []fileToOpen{
			{Path: "cmd/server/main.go", Reason: "entrypoint"},
			{Path: "internal/deepseek/deepseek.go", Reason: "invented provider path"},
		},
		HighLevelMap: []orientationMapItem{{
			Name: "configuration",
			Evidence: []string{
				"main.go imports config",
				"config/config.go and config/reload.go handle configuration",
			},
			WhyItMatters: "controls runtime behavior",
		}},
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "configuration reload",
			Trigger:          "SIGHUP",
			LikelyEntrypoint: "config package (not in allowed_paths)",
			LikelyFiles:      []string{"cmd/server/main.go", "config/reload.go"},
			WhyInteresting:   "runtime reconfiguration",
			Evidence: []string{
				"main.go starts the reload loop",
				"config/reload.go handles reloads",
			},
			Confidence: 0.7,
		}},
		ImportantDomainWords: []orientationDomainWord{{
			Word:     "reload",
			Guess:    "configuration reload",
			Evidence: []string{"main.go", "config/reload.go"},
		}},
		UnverifiedPaths: unverifiedPathList{
			{Path: "a/b/c/", Reason: "model returned a directory-like path"},
			{Path: "../secret", Reason: "unsafe"},
			{Path: "a/b/c", Reason: "duplicate canonical path"},
		},
	}
	allowed := []string{"cmd/server/main.go", "config/config.go", "config/reload.go"}

	normalizeOrientationGrounding(&report, allowed, nil, nil)

	if got := report.CandidateFlows[0].LikelyEntrypoint; got != "cmd/server/main.go" {
		t.Fatalf("likely_entrypoint = %q, want allowed fallback", got)
	}
	if len(report.FirstFilesToOpen) != 1 || report.FirstFilesToOpen[0].Path != "cmd/server/main.go" {
		t.Fatalf("first files = %#v, want only grounded path", report.FirstFilesToOpen)
	}
	if len(report.HighLevelMap[0].Evidence) != 1 || len(report.CandidateFlows[0].Evidence) != 1 ||
		len(report.ImportantDomainWords[0].Evidence) != 1 {
		t.Fatalf("normalized evidence = high:%q flow:%q domain:%q",
			report.HighLevelMap[0].Evidence,
			report.CandidateFlows[0].Evidence,
			report.ImportantDomainWords[0].Evidence,
		)
	}
	if len(report.UnverifiedPaths) != 1 || report.UnverifiedPaths[0].Path != "a/b/c" {
		t.Fatalf("normalized unverified paths = %#v, want one canonical path", report.UnverifiedPaths)
	}
	warnings := strings.Join(report.Warnings, "\n")
	if !strings.Contains(warnings, "dropped ungrounded path-like evidence") ||
		!strings.Contains(warnings, "replaced ungrounded") ||
		!strings.Contains(warnings, `dropped first_files_to_open[1] outside allowed_paths: "internal/deepseek/deepseek.go"`) ||
		!strings.Contains(warnings, `normalized unverified_paths[0] to "a/b/c"`) ||
		!strings.Contains(warnings, `dropped unverified_paths[1] with invalid path: "../secret"`) {
		t.Fatalf("warnings = %q, want evidence, entrypoint, and first-file warnings", report.Warnings)
	}
	if err := validateOrientation(report, allowed, nil); err != nil {
		t.Fatalf("normalized orientation should validate: %v", err)
	}
}

func TestNormalizeOrientationGroundingCanonicalizesVerifiedSourceLines(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "storage engine",
		Confidence:   0.8,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "batch commit",
			Trigger:          "Batch.Commit",
			LikelyEntrypoint: "batch.go",
			LikelyFiles:      []string{"batch.go"},
			Evidence: []string{
				"batch.go contains fsync call at line 395",
				"batch.go defines DeleteRange at line 999",
			},
			Confidence: 0.7,
		}},
	}
	normalizeOrientationGrounding(
		&report,
		[]string{"batch.go"},
		nil,
		[]evidence.Location{{Path: "batch.go", Line: 395}},
	)

	want := []string{"batch.go:395 contains fsync call", "batch.go defines DeleteRange"}
	if got := report.CandidateFlows[0].Evidence; !slices.Equal(got, want) {
		t.Fatalf("canonical evidence = %q, want %q", got, want)
	}
	if !strings.Contains(strings.Join(report.Warnings, "\n"), "removed ungrounded line claim") {
		t.Fatalf("warnings = %q", report.Warnings)
	}
}

func TestNormalizeOrientationGroundingDropsDirectoryFromLikelyFilesWithoutFailingRun(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "storage engine",
		Confidence:   0.8,
		CandidateFlows: []flowexplain.CandidateFlow{
			{
				Name:             "batch commit",
				Trigger:          "Batch.Commit",
				LikelyEntrypoint: "batch.go",
				LikelyFiles:      []string{"batch.go", "internal/compact"},
				Evidence:         []string{"batch.go"},
				Confidence:       0.7,
			},
			{
				Name:             "compaction",
				Trigger:          "background scheduler",
				LikelyEntrypoint: "internal/compact",
				LikelyFiles:      []string{"internal/compact"},
				Evidence:         []string{"internal/compact.go"},
				Confidence:       0.6,
			},
			{
				Name:             "ungrounded",
				Trigger:          "unknown",
				LikelyEntrypoint: "invented/package",
				LikelyFiles:      []string{"invented/package"},
				Evidence:         []string{"model guess without a file"},
				Confidence:       0.2,
			},
		},
	}
	allowed := []string{"batch.go", "internal/compact.go"}
	normalizeOrientationGrounding(&report, allowed, nil, nil)

	if len(report.CandidateFlows) != 2 {
		t.Fatalf("candidate flows = %#v", report.CandidateFlows)
	}
	if got := report.CandidateFlows[0].LikelyFiles; !slices.Equal(got, []string{"batch.go"}) {
		t.Fatalf("batch likely files = %q", got)
	}
	if flow := report.CandidateFlows[1]; flow.LikelyEntrypoint != "internal/compact.go" ||
		!slices.Equal(flow.LikelyFiles, []string{"internal/compact.go"}) {
		t.Fatalf("recovered compaction flow = %#v", flow)
	}
	warnings := strings.Join(report.Warnings, "\n")
	if !strings.Contains(warnings, "dropped ungrounded candidate_flows[0].likely_files[1]") ||
		!strings.Contains(warnings, "dropped candidate_flows[2]") {
		t.Fatalf("warnings = %q", report.Warnings)
	}
	if err := validateOrientation(report, allowed, nil); err != nil {
		t.Fatalf("normalized report should remain usable: %v", err)
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
