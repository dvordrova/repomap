package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
)

var update = flag.Bool("update", false, "update golden files")

func TestWriteReportHTML_Golden(t *testing.T) {
	latency := int64(432)
	localProof := &flowproof.Session{
		Version: flowproof.SessionVersion,
		Proof: flowproof.Proof{
			Version: flowproof.Version, ID: "flow-a", Archetype: flowproof.ArchetypeCLI, Goal: "Test Flow", Command: "serve",
			Slots: []flowproof.Slot{
				{Kind: flowproof.SlotTrigger, Status: flowproof.SlotVerified, Summary: "registered serve command"},
				{Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified, Summary: "exact main"},
				{Kind: flowproof.SlotTermination, Status: flowproof.SlotPartial, Missing: "shutdown path"},
			},
			Anchors: []flowproof.Anchor{
				{ID: "main", Kind: flowproof.AnchorFunction, Label: "main", Location: &evidence.Location{Path: "main.go", Line: 1}},
				{ID: "serve", Kind: flowproof.AnchorFunction, Label: "serve", Location: &evidence.Location{Path: "server/server.go", Line: 20}},
			},
			Transitions: []flowproof.Transition{{
				ID: "dispatch-serve", From: "main", To: "serve", Relation: evidence.RelationCalls,
				Resolution: evidence.ResolutionStatic, Invocation: evidence.InvocationSynchronous,
				Certainty: evidence.CertaintyStatic, Evidence: evidence.Location{Path: "main.go", Line: 8}, Provider: "go_syntax",
			}},
		},
		Budget: flowproof.DefaultBudget(),
		Stats:  flowproof.Stats{TasksCompleted: 1, Files: []string{"main.go", "server/server.go"}, Symbols: []string{"serve"}, WallMillis: 42},
		Stop:   &flowproof.Stop{Reason: flowproof.StopNoTask, Message: "shutdown executor is not configured"},
	}
	data := ReportData{
		FormatVersion:         CurrentFormatVersion,
		RepoName:              "testrepo",
		ProjectGuess:          "test project",
		OrientationConfidence: 0.82,
		HighLevelMap: []Subsystem{{
			Name:         "command",
			Role:         componentmap.RoleEntry,
			Evidence:     []string{"main.go wires the server"},
			WhyItMatters: "owns process startup",
		}},
		FirstFilesToOpen: []FileItem{{
			Path:     "main.go",
			Reason:   "process entrypoint",
			Priority: 1,
		}},
		CandidateFlows: []string{"flow-a"},
		CandidateDirections: []CandidateDirection{{
			ID:               "flow-a",
			Name:             "Test Flow",
			Trigger:          "a request arrives",
			LikelyEntrypoint: "main.go",
			LikelyFiles:      []string{"main.go", "server/server.go"},
			WhyInteresting:   "shows the primary request path",
			Evidence:         []string{"main.go wires the server"},
			Confidence:       0.75,
			LocalVerification: &flowexplain.FlowVerification{
				Status:        "partial",
				ConfidenceCap: 0.75,
				Verified:      []string{"main.go:1 exact entrypoint"},
				Missing:       []string{"first domain-level call"},
			},
			LocalProof: localProof,
		}},
		ImportantDomainWords: []DomainWord{{
			Word:     "worker",
			Guess:    "background request processor",
			Evidence: []string{"server/server.go starts workers"},
		}},
		QuestionsForHuman: []string{"Which runtime path matters most?"},
		OrientationUnverifiedPaths: []PathItem{{
			Path:   "legacy/worker.go",
			Reason: "mentioned but not included in the bounded context",
		}},
		RecommendedFlow: "flow-a",
		FlowCount:       1,
		ArtifactsDir:    "/tmp/test-run",
		Warnings:        []string{"global warning 1"},
		Run: &RunInfo{
			CreatedAt:               "2026-07-10T12:00:00Z",
			Model:                   "deepseek-v4-flash",
			PromptVersion:           "orientation-json-v3",
			CompactContextBytes:     12000,
			ExternalRequestBytes:    18000,
			ProviderRequestCount:    1,
			CandidateDirectionCount: 1,
			ProviderLatencyMillis:   &latency,
		},
		Flows: []FlowData{{
			ID:               "flow-a",
			Name:             "Test Flow",
			Summary:          "does something interesting",
			Confidence:       0.75,
			ConfidenceLabel:  "strong",
			BundleStatsLabel: "5 source, 2 test, 1 doc",
			LikelyChain: []ChainStep{{
				Step:          1,
				Name:          "start",
				WhatHappens:   "begins execution",
				EvidenceFiles: []string{"main.go"},
				Confidence:    0.8,
			}},
			FilesToRead: []FileItem{
				{Path: "main.go", Reason: "entrypoint", Priority: 1},
				{Path: "server/server.go", Reason: "request routing", Priority: 2},
			},
			TestsToRead: []FileItem{
				{Path: "main_test.go", Reason: "integration test"},
			},
			UnverifiedPaths: []PathItem{
				{Path: "legacy.go", Reason: "may be unused"},
			},
			Unknowns: []string{"How does auth interact?"},
			Warnings: []string{"Low confidence in step 1"},
			BundleSummary: BundleStats{
				SelectedFilesCount: 5,
				SelectedTestsCount: 2,
				SelectedDocsCount:  1,
				SelectedPkgsCount:  3,
				RelatedEdgesCount:  4,
			},
		}},
	}

	html, err := buildHTML(&data)
	if err != nil {
		t.Fatalf("buildHTML: %v", err)
	}

	// Replaceable presentation snapshot: it catches accidental loss of the
	// self-contained report, but its markup is not a stable product contract.
	// Intentional UI refactors should regenerate it instead of preserving DOM
	// details solely for this test.
	golden := filepath.Join("testdata", "report.golden.html")
	if *update {
		if err := os.WriteFile(golden, html, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create)", err)
	}

	if !bytes.Equal(html, want) {
		t.Errorf("HTML output differs from golden file.\nRun 'go test -run TestWriteReportHTML_Golden -update' to regenerate.")
		t.Errorf("Got %d bytes, want %d bytes", len(html), len(want))
	}
}

func TestReportDoesNotRenderStaticFactsAsRuntimeSequence(t *testing.T) {
	// Replaceable presentation contract: this test protects the previously
	// misleading runtime sequence claim, not the current DOM structure.
	data := ReportData{
		RepoName: "static-proof",
		CandidateDirections: []CandidateDirection{{
			ID:   "backup",
			Name: "Backup",
			LocalProof: &flowproof.Session{
				Proof: flowproof.Proof{
					Slots: []flowproof.Slot{{
						Kind:        flowproof.SlotApplicationCallable,
						Status:      flowproof.SlotVerified,
						EvidenceIDs: []string{"dispatch-handler"},
					}},
					Anchors: []flowproof.Anchor{
						{ID: "main", Kind: flowproof.AnchorFunction, Label: "main", Location: &evidence.Location{Path: "main.go", Line: 10}},
						{ID: "handler", Kind: flowproof.AnchorFunction, Label: "runBackup", Location: &evidence.Location{Path: "backup.go", Line: 40}},
						{ID: "scanner-task", Kind: flowproof.AnchorTask, Label: "scanner task", Location: &evidence.Location{Path: "backup.go", Line: 60}},
						{ID: "scan", Kind: flowproof.AnchorMethod, Label: "Scan", Location: &evidence.Location{Path: "scanner.go", Line: 20}},
					},
					Transitions: []flowproof.Transition{
						{
							ID: "dispatch-handler", From: "main", To: "handler", Relation: evidence.RelationDispatches,
							Resolution: evidence.ResolutionFrameworkRule, Invocation: evidence.InvocationCallback,
							Evidence: evidence.Location{Path: "main.go", Line: 25},
						},
						{
							ID: "start-scanner", From: "handler", To: "scanner-task", Relation: evidence.RelationStartsGoroutine,
							Resolution: evidence.ResolutionStatic, Invocation: evidence.InvocationGoroutine,
							Evidence: evidence.Location{Path: "backup.go", Line: 60},
						},
						{
							ID: "scan-body", From: "scanner-task", To: "scan", Relation: evidence.RelationCallback,
							Resolution: evidence.ResolutionStatic, Invocation: evidence.InvocationGoroutine,
							Condition: &evidence.Condition{
								Expression: "opts.Scan",
								Location:   evidence.Location{Path: "backup.go", Line: 59},
							},
							Evidence: evidence.Location{Path: "backup.go", Line: 61},
						},
					},
				},
			},
		}},
	}

	html, err := buildHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte("Static relation groups"),
		[]byte("Grouped by static scope. Runtime order is not inferred."),
		[]byte("Command setup"),
		[]byte("Main handler branch"),
		[]byte("Task · "),
		[]byte("Other static relations"),
		[]byte("→ ' + relation + ' / ' + invocation + ' →"),
		[]byte(`"expression":"opts.Scan"`),
	} {
		if !bytes.Contains(html, want) {
			t.Errorf("report HTML missing static proof presentation %q", want)
		}
	}
	for _, unwanted := range [][]byte{
		[]byte("Guided symbol path"),
		[]byte("var sequence = []"),
		[]byte("el('ol', 'rm-proof-path')"),
	} {
		if bytes.Contains(html, unwanted) {
			t.Errorf("report HTML still presents static facts as an ordered runtime path: %q", unwanted)
		}
	}
}

func TestWriteReportHTML_WritesFile(t *testing.T) {
	data := ReportData{
		RepoName:  "test",
		FlowCount: 0,
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")

	if err := WriteReportHTML(&data, path); err != nil {
		t.Fatalf("WriteReportHTML: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report.html: %v", err)
	}
	if len(b) == 0 {
		t.Error("report.html is empty")
	}
	if !bytes.Contains(b, []byte("test")) {
		t.Error("report.html does not contain repo name")
	}
	if !bytes.Contains(b, []byte("rm-report-data")) {
		t.Error("report.html missing report-data script tag")
	}
	if bytes.Contains(b, []byte("F:")) {
		t.Error("report.html contains single-letter abbreviation F:")
	}
	if bytes.Contains(b, []byte("S:")) {
		t.Error("report.html contains single-letter abbreviation S:")
	}
}

func TestWriteReportHTML_EscapesModelControlledTitle(t *testing.T) {
	data := ReportData{
		RepoName:     "safe-repo",
		ProjectGuess: `</title><script>globalThis.pwned = true</script>`,
	}
	html, err := buildHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(html, []byte(`</title><script>globalThis.pwned`)) {
		t.Fatalf("model-controlled title was embedded as executable HTML: %s", html)
	}
	if !bytes.Contains(html, []byte(`&lt;/title&gt;&lt;script&gt;globalThis.pwned`)) {
		t.Fatalf("escaped title is missing: %s", html)
	}
	if !bytes.Contains(html, []byte(`\u003c/title\u003e\u003cscript\u003e`)) {
		t.Fatalf("embedded report JSON did not retain safe JSON escaping: %s", html)
	}
}

func TestWriteReportHTML_OrientationOnlyIncludesCandidateDirections(t *testing.T) {
	data := ReportData{
		FormatVersion:         CurrentFormatVersion,
		RepoName:              "orientation-only",
		ProjectGuess:          "metrics service",
		OrientationConfidence: 0.86,
		HighLevelMap: []Subsystem{{
			Name:         "metrics API",
			Evidence:     []string{"api/server.go registers metrics routes"},
			WhyItMatters: "serves collected measurements",
		}},
		FirstFilesToOpen: []FileItem{{
			Path:   "api/server.go",
			Reason: "public API entrypoint",
		}},
		CandidateFlows: []string{
			"HTTP request",
		},
		CandidateDirections: []CandidateDirection{{
			ID:               "http-request",
			Name:             "HTTP request",
			Trigger:          "GET /metrics",
			LikelyEntrypoint: "api/server.go",
			LikelyFiles:      []string{"api/server.go", "metrics/registry.go"},
			WhyInteresting:   "connects the public API to metric collection",
			Evidence:         []string{"api/server.go registers the route"},
			Confidence:       0.8,
		}},
		ImportantDomainWords: []DomainWord{{
			Word:     "scrape",
			Guess:    "collect metrics from a target",
			Evidence: []string{"metrics/registry.go stores collectors"},
		}},
		QuestionsForHuman: []string{"Which collector is representative?"},
		OrientationUnverifiedPaths: []PathItem{{
			Path:   "legacy/scrape.go",
			Reason: "not present in the bounded context",
		}},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")
	if err := WriteReportHTML(&data, path); err != nil {
		t.Fatalf("WriteReportHTML: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report.html: %v", err)
	}
	for _, want := range [][]byte{
		[]byte("Purpose"),
		[]byte("Components"),
		[]byte("Start here"),
		[]byte("Important terms"),
		[]byte("Questions for a teammate"),
		[]byte("metrics service"),
		[]byte("metrics API"),
		[]byte("serves collected measurements"),
		[]byte("public API entrypoint"),
		[]byte("scrape"),
		[]byte("collect metrics from a target"),
		[]byte("Which collector is representative?"),
		[]byte("legacy/scrape.go"),
		[]byte("candidate_directions"),
		[]byte("HTTP request"),
		[]byte("GET /metrics"),
		[]byte("api/server.go"),
		[]byte("connects the public API to metric collection"),
		[]byte("api/server.go registers the route"),
		[]byte("Directions to explore"),
	} {
		if !bytes.Contains(b, want) {
			t.Errorf("report.html does not contain %q", want)
		}
	}
	if bytes.Contains(b, []byte("No flows identified")) {
		t.Error("orientation-only report contains the misleading old empty-state text")
	}
}

func TestWriteReportHTML_DirectionCanOpenSavedLocalEvidence(t *testing.T) {
	data := ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "friend-project",
		CandidateDirections: []CandidateDirection{{
			ID:               "worker-run",
			Name:             "Worker run",
			Trigger:          "the worker starts",
			LikelyEntrypoint: "internal/worker/worker.go",
			LikelyFiles:      []string{"internal/worker/worker.go"},
			Evidence:         []string{"internal/worker/worker.go"},
			Confidence:       0.8,
		}},
		Flows: []FlowData{{
			ID:           "worker-run",
			Name:         "Worker run",
			EvidenceOnly: true,
			FlowStatus:   "local_only",
			BundleFiles: []FileItem{{
				Path:   "internal/worker/worker.go",
				Reason: "likely_file from candidate_flow",
			}},
		}},
	}

	html, err := buildHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte(`"evidence_only":true`),
		[]byte("Explore this direction"),
		[]byte("Suggested files are selected from repository facts"),
		[]byte("Suggested files to inspect"),
		[]byte("Run details"),
		[]byte("rm-candidate-direction--clickable"),
	} {
		if !bytes.Contains(html, want) {
			t.Errorf("report HTML missing %q", want)
		}
	}
	for _, unwanted := range [][]byte{
		[]byte("· local evidence"),
		[]byte(">Local evidence<"),
		[]byte("No second model call was made"),
	} {
		if bytes.Contains(html, unwanted) {
			t.Errorf("report HTML still contains noisy implementation label %q", unwanted)
		}
	}
}

func writeTestFileFullPath(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestFile(%s): %v", path, err)
	}
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"generatetest"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"gen project","candidate_flows":[],"warnings":[]}`)

	flowDir := filepath.Join(dir, "flows", "gen-flow")
	mkdirAll(t, flowDir)
	writeTestFileFullPath(t, filepath.Join(flowDir, "flow_bundle.json"), `{
		"flow_seed": {"name": "Generate Flow"},
		"selected_files": [{"path":"g.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": ["pkg"],
		"related_edges": []
	}`)
	writeTestFileFullPath(t, filepath.Join(flowDir, "flow_report.json"), `{
		"summary": "generated flow",
		"confidence": 0.8,
		"files_to_read_in_order": [{"path":"g.go","reason":"entrypoint"}],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify report.json exists and is valid
	jsonPath := filepath.Join(dir, "report.json")
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if len(b) == 0 {
		t.Error("report.json is empty")
	}

	// Verify report.html exists
	htmlPath := filepath.Join(dir, "report.html")
	b, err = os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read report.html: %v", err)
	}
	if len(b) == 0 {
		t.Error("report.html is empty")
	}
	if !bytes.Contains(b, []byte("generatetest")) {
		t.Error("report.html does not contain repo name")
	}
	if containsLegacySingleLetterLabel(b) {
		t.Error("report.html contains single-letter abbreviations")
	}

	feedbackPath := filepath.Join(dir, "onboarding-feedback.md")
	feedback, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatalf("read onboarding feedback template: %v", err)
	}
	for _, want := range [][]byte{[]byte("Repository: generatetest"), []byte("## Correct"), []byte("## Missing"), []byte("## Misleading")} {
		if !bytes.Contains(feedback, want) {
			t.Errorf("feedback template missing %q", want)
		}
	}
	if !bytes.Contains(b, []byte("onboarding-feedback.md")) {
		t.Error("report HTML does not expose the feedback path")
	}

	const friendNotes = "friend notes must survive report regeneration\n"
	if err := os.WriteFile(feedbackPath, []byte(friendNotes), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Generate(dir); err != nil {
		t.Fatalf("regenerate report: %v", err)
	}
	feedback, err = os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(feedback) != friendNotes {
		t.Fatalf("report regeneration overwrote feedback: %q", feedback)
	}
}

// containsLegacySingleLetterLabel intentionally looks only for an HTML text
// node shaped like the retired one-letter labels. Searching the complete
// self-contained HTML for "D:" also matches harmless JavaScript identifiers
// such as candidateID and makes this presentation test expensive to maintain.
func containsLegacySingleLetterLabel(html []byte) bool {
	for _, label := range [][]byte{
		[]byte(">F:<"),
		[]byte(">S:<"),
		[]byte(">T:<"),
		[]byte(">D:<"),
		[]byte(">P:<"),
		[]byte(">E:<"),
	} {
		if bytes.Contains(html, label) {
			return true
		}
	}
	return false
}
