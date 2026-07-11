package report

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestConfidenceLabel(t *testing.T) {
	tests := []struct {
		conf float64
		want string
	}{
		{0.95, "High"},
		{0.7, "High"},
		{0.69, "Medium"},
		{0.4, "Medium"},
		{0.39, "Low"},
		{0.0, "Low"},
		{-1.0, "Low"},
		{math.NaN(), "Low"},
	}
	for _, tt := range tests {
		got := confidenceLabel(tt.conf)
		if got != tt.want {
			t.Errorf("confidenceLabel(%v) = %q, want %q", tt.conf, got, tt.want)
		}
	}
}

func TestBundleStatsLabel(t *testing.T) {
	tests := []struct {
		bs   BundleStats
		want string
	}{
		{BundleStats{SelectedFilesCount: 12, SelectedTestsCount: 5, SelectedDocsCount: 3}, "12 source files / 5 tests / 3 docs"},
		{BundleStats{SelectedFilesCount: 0, SelectedTestsCount: 0, SelectedDocsCount: 0}, "0 source files / 0 tests / 0 docs"},
	}
	for _, tt := range tests {
		got := bundleStatsLabel(tt.bs)
		if got != tt.want {
			t.Errorf("bundleStatsLabel(%+v) = %q, want %q", tt.bs, got, tt.want)
		}
	}
}

func TestFindBestFlow(t *testing.T) {
	t.Run("nil flows", func(t *testing.T) {
		if got := findBestFlow(nil); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
	t.Run("empty flows", func(t *testing.T) {
		if got := findBestFlow([]FlowData{}); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
	t.Run("error flow skipped", func(t *testing.T) {
		flows := []FlowData{
			{ID: "a", Confidence: 0.9, Error: "broken"},
			{ID: "b", Confidence: 0.3, Summary: "ok"},
		}
		if got := findBestFlow(flows); got != "b" {
			t.Errorf("expected b (error skipped), got %q", got)
		}
	})
	t.Run("no summary skipped", func(t *testing.T) {
		flows := []FlowData{
			{ID: "a", Confidence: 0.9, Error: ""},
			{ID: "b", Confidence: 0.3, Summary: "ok"},
		}
		if got := findBestFlow(flows); got != "b" {
			t.Errorf("expected b (no summary skipped), got %q", got)
		}
	})
	t.Run("highest confidence wins", func(t *testing.T) {
		flows := []FlowData{
			{ID: "a", Confidence: 0.5, Summary: "ok"},
			{ID: "b", Confidence: 0.8, Summary: "ok"},
			{ID: "c", Confidence: 0.3, Summary: "ok"},
		}
		if got := findBestFlow(flows); got != "b" {
			t.Errorf("expected b, got %q", got)
		}
	})
	t.Run("tiebreak by file count", func(t *testing.T) {
		flows := []FlowData{
			{ID: "a", Confidence: 0.8, Summary: "ok", FilesToRead: []FileItem{{Path: "x.go"}}},
			{ID: "b", Confidence: 0.8, Summary: "ok", FilesToRead: []FileItem{{Path: "x.go"}, {Path: "y.go"}}},
		}
		if got := findBestFlow(flows); got != "b" {
			t.Errorf("expected b (more files to read), got %q", got)
		}
	})
}

func TestEnrich(t *testing.T) {
	data := &ReportData{
		Flows: []FlowData{
			{ID: "a", Confidence: 0.85, Summary: "ok", BundleSummary: BundleStats{SelectedFilesCount: 10, SelectedTestsCount: 3, SelectedDocsCount: 2}},
			{ID: "b", Confidence: 0.35, Summary: "ok", BundleSummary: BundleStats{SelectedFilesCount: 5, SelectedTestsCount: 1, SelectedDocsCount: 1}},
		},
	}
	enrich(data)

	if data.FormatVersion != 2 {
		t.Errorf("format version = %d, want 2", data.FormatVersion)
	}
	if data.FlowCount != 2 {
		t.Errorf("flow count = %d, want 2", data.FlowCount)
	}
	if data.RecommendedFlow != "a" {
		t.Errorf("recommended flow = %q, want a", data.RecommendedFlow)
	}
	if data.Flows[0].ConfidenceLabel != "High" {
		t.Errorf("flow a confidence label = %q, want High", data.Flows[0].ConfidenceLabel)
	}
	if data.Flows[1].ConfidenceLabel != "Low" {
		t.Errorf("flow b confidence label = %q, want Low", data.Flows[1].ConfidenceLabel)
	}
	if data.Flows[0].BundleStatsLabel != "10 source files / 3 tests / 2 docs" {
		t.Errorf("flow a stats label = %q", data.Flows[0].BundleStatsLabel)
	}
}

func TestReadRunDir_Integration(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"etcd","readme":"..."}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"KV store","candidate_flows":[{"name":"gRPC Put"}],"warnings":["w1"]}`)

	flowDir := filepath.Join(dir, "flows", "grpc-put")
	mkdirAll(t, flowDir)

	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "gRPC Put Request"},
		"selected_files": [{"path":"a.go","kind":"source","score":200}],
		"selected_tests": [{"path":"a_test.go","kind":"test","score":100}],
		"selected_docs": [{"path":"doc.md","kind":"doc","score":50}],
		"selected_packages": ["pkg"],
		"related_edges": [{"from":"a","to":"b"}]
	}`)

	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "handles gRPC put",
		"confidence": 0.85,
		"flow_name": "gRPC Put Request",
		"likely_chain": [
			{"step":1,"name":"receive","what_happens":"gets request","evidence_files":["a.go"],"confidence":0.9}
		],
		"files_to_read_in_order": [
			{"path":"a.go","reason":"entrypoint","priority":1}
		],
		"tests_to_read": [
			{"path":"a_test.go","reason":"covers handler"}
		],
		"unverified_paths": [
			{"path":"legacy.go","reason":"may be unused"}
		],
		"unknowns": ["How does auth interact?"],
		"warnings": ["Low confidence in step 1"]
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}

	if data.RepoName != "etcd" {
		t.Errorf("repo_name = %q, want etcd", data.RepoName)
	}
	if data.ProjectGuess != "KV store" {
		t.Errorf("project_guess = %q, want KV store", data.ProjectGuess)
	}
	if len(data.CandidateFlows) != 1 || data.CandidateFlows[0] != "gRPC Put" {
		t.Errorf("candidate_flows = %v", data.CandidateFlows)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(data.Flows))
	}

	f := data.Flows[0]
	if f.ID != "grpc-put" {
		t.Errorf("flow ID = %q, want grpc-put", f.ID)
	}
	if f.Name != "gRPC Put Request" {
		t.Errorf("flow name = %q", f.Name)
	}
	if f.Summary != "handles gRPC put" {
		t.Errorf("summary = %q", f.Summary)
	}
	if f.Confidence != 0.85 {
		t.Errorf("confidence = %f", f.Confidence)
	}
	if f.ConfidenceLabel != "High" {
		t.Errorf("confidence label = %q, want High", f.ConfidenceLabel)
	}
	if f.BundleStatsLabel != "1 source files / 1 tests / 1 docs" {
		t.Errorf("bundle stats label = %q", f.BundleStatsLabel)
	}

	if len(f.LikelyChain) != 1 {
		t.Fatalf("expected 1 chain step, got %d", len(f.LikelyChain))
	}
	cs := f.LikelyChain[0]
	if cs.Step != 1 || cs.Name != "receive" || cs.WhatHappens != "gets request" || cs.Confidence != 0.9 {
		t.Errorf("chain step = %+v", cs)
	}
	if len(cs.EvidenceFiles) != 1 || cs.EvidenceFiles[0] != "a.go" {
		t.Errorf("evidence files = %v", cs.EvidenceFiles)
	}

	if len(f.FilesToRead) != 1 || f.FilesToRead[0].Path != "a.go" || f.FilesToRead[0].Reason != "entrypoint" || f.FilesToRead[0].Priority != 1 {
		t.Errorf("files to read = %+v", f.FilesToRead)
	}
	if len(f.TestsToRead) != 1 || f.TestsToRead[0].Path != "a_test.go" {
		t.Errorf("tests to read = %+v", f.TestsToRead)
	}
	if len(f.UnverifiedPaths) != 1 || f.UnverifiedPaths[0].Path != "legacy.go" {
		t.Errorf("unverified paths = %+v", f.UnverifiedPaths)
	}
	if len(f.Unknowns) != 1 || f.Unknowns[0] != "How does auth interact?" {
		t.Errorf("unknowns = %v", f.Unknowns)
	}
	if len(f.Warnings) != 1 || f.Warnings[0] != "Low confidence in step 1" {
		t.Errorf("warnings = %v", f.Warnings)
	}

	if f.BundleSummary.SelectedFilesCount != 1 {
		t.Errorf("selected files = %d", f.BundleSummary.SelectedFilesCount)
	}
	if f.BundleSummary.SelectedTestsCount != 1 {
		t.Errorf("selected tests = %d", f.BundleSummary.SelectedTestsCount)
	}
	if f.BundleSummary.SelectedDocsCount != 1 {
		t.Errorf("selected docs = %d", f.BundleSummary.SelectedDocsCount)
	}
	if f.BundleSummary.SelectedPkgsCount != 1 {
		t.Errorf("selected pkgs = %d", f.BundleSummary.SelectedPkgsCount)
	}
	if f.BundleSummary.RelatedEdgesCount != 1 {
		t.Errorf("related edges = %d", f.BundleSummary.RelatedEdgesCount)
	}

	if data.RecommendedFlow != "grpc-put" {
		t.Errorf("recommended flow = %q, want grpc-put", data.RecommendedFlow)
	}
	if data.FlowCount != 1 {
		t.Errorf("flow count = %d, want 1", data.FlowCount)
	}
	if data.FormatVersion != 2 {
		t.Errorf("format version = %d, want 2", data.FormatVersion)
	}
	if len(data.Warnings) < 1 || data.Warnings[0] != "w1" {
		t.Errorf("top-level warnings = %v", data.Warnings)
	}
}

func TestReadRunDir_EmptyFlowsDir(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, "flows"))
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 0 {
		t.Errorf("expected 0 flows, got %d", len(data.Flows))
	}
	if data.RepoName != "test" {
		t.Errorf("repo_name = %q", data.RepoName)
	}
}

func TestReadRunDir_FlowWithError(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "bad-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(data.Flows))
	}
	if data.Flows[0].Error == "" {
		t.Error("expected error field for missing flow report")
	}
	if data.Flows[0].ID != "bad-flow" {
		t.Errorf("flow ID = %q", data.Flows[0].ID)
	}
}

func TestReadRunDir_MissingSnapshot(t *testing.T) {
	dir := t.TempDir()
	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if data.RepoName != "" {
		t.Error("expected empty repo name when snapshot missing")
	}
}

func TestReadRunDir_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "bad-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_report.json", `{not valid json}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(data.Flows))
	}
	if data.Flows[0].Error == "" {
		t.Error("expected error for malformed JSON")
	}
}

func TestReadRunDir_FlowWithoutFlowsDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 0 {
		t.Errorf("expected 0 flows, got %d", len(data.Flows))
	}
}

func TestReadRunDir_BundleOnlyNoReport(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "bundle-only")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Test Flow"},
		"selected_files": [{"path":"a.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": ["pkg"],
		"related_edges": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(data.Flows))
	}
	f := data.Flows[0]
	if f.Error == "" {
		t.Error("expected error since flow_report.json is missing")
	}
	if f.Name != "Test Flow" {
		t.Errorf("flow name = %q", f.Name)
	}
	if f.BundleSummary.SelectedFilesCount != 1 {
		t.Errorf("selected files = %d", f.BundleSummary.SelectedFilesCount)
	}
}

func TestWriteReportJSON_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"testrepo"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"test guess","candidate_flows":[{"name":"flow1"}]}`)

	flowDir := filepath.Join(dir, "flows", "flow1")
	mkdirAll(t, flowDir)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Flow One"},
		"selected_files": [{"path":"main.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "flows nicely",
		"confidence": 0.75,
		"files_to_read_in_order": [{"path":"main.go","reason":"entrypoint"}],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}

	jsonPath := filepath.Join(dir, "report.json")
	if err := WriteReportJSON(data, jsonPath); err != nil {
		t.Fatalf("WriteReportJSON: %v", err)
	}

	// Read back and verify
	data2, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir (roundtrip): %v", err)
	}
	if data2.RepoName != data.RepoName {
		t.Errorf("round-trip repo_name: %q vs %q", data2.RepoName, data.RepoName)
	}
	if len(data2.Flows) != len(data.Flows) {
		t.Errorf("round-trip flow count: %d vs %d", len(data2.Flows), len(data.Flows))
	}
	if data2.RecommendedFlow != data.RecommendedFlow {
		t.Errorf("round-trip recommended flow: %q vs %q", data2.RecommendedFlow, data.RecommendedFlow)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	data := ReportData{
		RepoName:     "test",
		FormatVersion: 2,
		Flows: []FlowData{{
			ID:         "f1",
			Confidence: 0.5,
			FilesToRead: []FileItem{{Path: "x.go", Reason: "test", Priority: 1}},
			ConfidenceLabel: "Medium",
			BundleStatsLabel: "1 source files / 0 tests / 0 docs",
		}},
		RecommendedFlow: "f1",
		FlowCount:       1,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	var got ReportData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	if got.Flows[0].FilesToRead[0].Path != "x.go" {
		t.Errorf("round-trip broken: path = %q", got.Flows[0].FilesToRead[0].Path)
	}
	if got.Flows[0].ConfidenceLabel != "Medium" {
		t.Errorf("round-trip: confidence_label = %q", got.Flows[0].ConfidenceLabel)
	}
	if got.RecommendedFlow != "f1" {
		t.Errorf("round-trip: recommended_flow = %q", got.RecommendedFlow)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestFile(%s): %v", name, err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdirAll(%s): %v", path, err)
	}
}
