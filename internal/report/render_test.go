package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestWriteReportHTML_Golden(t *testing.T) {
	data := ReportData{
		FormatVersion:  3,
		RepoName:       "testrepo",
		ProjectGuess:   "test project",
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
		}},
		RecommendedFlow: "flow-a",
		FlowCount:       1,
		ArtifactsDir:    "/tmp/test-run",
		Warnings:        []string{"global warning 1"},
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

func TestWriteReportHTML_OrientationOnlyIncludesCandidateDirections(t *testing.T) {
	data := ReportData{
		FormatVersion: 3,
		RepoName:      "orientation-only",
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
	if bytes.Contains(b, []byte("F:")) || bytes.Contains(b, []byte("S:")) || bytes.Contains(b, []byte("T:")) {
		t.Error("report.html contains single-letter abbreviations")
	}
}
