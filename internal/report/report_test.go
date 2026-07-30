package report

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/flowexplain"
)

func TestExactDiscoveryAnchorsPreserveSavedDeclarationOrder(t *testing.T) {
	t.Parallel()

	anchors := ExactDiscoveryAnchors(
		"src/tool/service.py",
		40,
		[]string{
			"# bounded saved source",
			"def run() -> None:",
			"    print('hello')",
			"async def stop() -> None:",
		},
	)
	if len(anchors) != 2 {
		t.Fatalf("anchors = %#v, want two exact declarations", anchors)
	}
	if anchors[0].Path != "src/tool/service.py" ||
		anchors[0].Language != "python" ||
		anchors[0].Symbol != "run" ||
		anchors[0].Line != 41 ||
		anchors[1].Symbol != "stop" ||
		anchors[1].Line != 43 {
		t.Fatalf("anchors = %#v, want saved declaration order and exact lines", anchors)
	}
	for _, anchor := range anchors {
		if len(anchor.Statement) == 0 || len(anchor.ContentSHA256) != 64 {
			t.Fatalf("anchor is not bounded and content-addressed: %#v", anchor)
		}
	}
}

func TestExactDiscoveryAnchorsRejectUnboundedOrUnauthorizedInputs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		path  string
		start int
		lines []string
	}{
		{name: "absolute path", path: "/tmp/service.py", start: 1, lines: []string{"def run():"}},
		{name: "invalid start", path: "service.py", start: 0, lines: []string{"def run():"}},
		{name: "unknown language", path: "service.txt", start: 1, lines: []string{"def run():"}},
		{name: "oversized line", path: "service.py", start: 1, lines: []string{strings.Repeat("x", (64<<10)+1)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ExactDiscoveryAnchors(test.path, test.start, test.lines); len(got) != 0 {
				t.Fatalf("ExactDiscoveryAnchors() = %#v, want no anchors", got)
			}
		})
	}
}

func TestUserTopicUncertaintyExplainsSupportedIncompleteReasons(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		reason string
		want   string
	}{
		{
			reason: "core_work_fact_missing",
			want:   "The exact starting point is known, but exact source evidence does not yet establish the core behavior.",
		},
		{
			reason: "unresolved_dynamic_dispatch",
			want:   "Exact local evidence is available, but dynamic dispatch prevents proving the next target.",
		},
		{
			reason: "proof_adapter_unavailable",
			want:   "A complete proof adapter is not available for this language yet, so this remains an exact starting point rather than a claimed mechanism.",
		},
	} {
		t.Run(test.reason, func(t *testing.T) {
			got, ok := userTopicUncertainty([]string{test.reason})
			if !ok || got != test.want {
				t.Fatalf("userTopicUncertainty() = %q, %v, want %q, true", got, ok, test.want)
			}
		})
	}
}

type topicProjectionFixture struct {
	id       string
	title    string
	question string
	path     string
	symbol   string
	line     int
	reasons  []string
}

func TestProjectFreshRepoTopicsPreservesChattoRejectionReasons(t *testing.T) {
	t.Parallel()

	fixtures := []topicProjectionFixture{
		{
			id:       "presence",
			title:    "Real-time presence projection",
			question: "How are user presences aggregated for real-time delivery?",
			path:     "cli/internal/connectapi/realtime_projection.go",
			symbol:   "API.BuildRealtimeProjectionPresences",
			line:     82,
			reasons:  []string{"core_work_fact_missing", "unresolved_dynamic_dispatch"},
		},
		{
			id:       "message",
			title:    "Message creation process",
			question: "How does the server process a new chat message and persist it?",
			path:     "cli/internal/connectapi/messages.go",
			symbol:   "messageService.CreateMessage",
			line:     45,
			reasons:  []string{"observable_effect_fact_missing", "bounded_static_analysis_limit"},
		},
		{
			id:       "upload",
			title:    "Asset upload initiation",
			question: "How are file uploads initiated and validated in the asset upload service?",
			path:     "cli/internal/connectapi/asset_uploads.go",
			symbol:   "assetUploadService.CreateUpload",
			line:     17,
			reasons:  []string{"observable_effect_fact_missing", "unresolved_dynamic_dispatch"},
		},
	}
	runDir := t.TempDir()
	writeTopicProjectionArtifacts(t, runDir, fixtures)

	topics, warning := projectFreshRepoTopics(&ReportData{ArtifactsDir: runDir})
	if warning != "" {
		t.Fatalf("projectFreshRepoTopics() warning = %q", warning)
	}
	if len(topics) != len(fixtures) {
		t.Fatalf("topics = %#v, want %d", topics, len(fixtures))
	}
	for index, topic := range topics {
		fixture := fixtures[index]
		if topic.CandidateID != fixture.id || topic.Title != fixture.title ||
			topic.Question != fixture.question || len(topic.StartingSymbols) != 1 {
			t.Fatalf("topic %d = %#v, want fixture %#v", index, topic, fixture)
		}
		location := topic.StartingSymbols[0]
		if location.Path != fixture.path || location.Symbol != fixture.symbol || location.Line != fixture.line {
			t.Errorf("topic %d location = %#v, want %s:%d %s", index, location, fixture.path, fixture.line, fixture.symbol)
		}
		if strings.TrimSpace(topic.Uncertainty) == "" {
			t.Errorf("topic %d has empty uncertainty", index)
		}
	}
	encoded, err := json.Marshal(topics)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"answer"`, `"steps"`, `"effect"`, `"order"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("topic JSON contains mechanism claim field %s: %s", forbidden, encoded)
		}
	}
}

func TestProjectFreshRepoTopicsStillFailsClosedForUnknownReason(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeTopicProjectionArtifacts(t, runDir, []topicProjectionFixture{{
		id: "unknown", title: "Unknown", question: "What happens?", path: "main.go",
		symbol: "main", line: 1, reasons: []string{"future_unreviewed_reason"},
	}})

	topics, warning := projectFreshRepoTopics(&ReportData{ArtifactsDir: runDir})
	if len(topics) != 0 || !strings.Contains(warning, "rejected candidate reason is unsupported") {
		t.Fatalf("projectFreshRepoTopics() = %#v, %q, want fail-closed warning", topics, warning)
	}
}

func writeTopicProjectionArtifacts(t *testing.T, runDir string, fixtures []topicProjectionFixture) {
	t.Helper()

	opportunityCandidates := make([]any, 0, len(fixtures))
	selected := make([]any, 0, len(fixtures))
	attempts := make([]any, 0, len(fixtures))
	for _, fixture := range fixtures {
		factID := fixture.id + "-fact"
		eligibility := map[string]any{
			"status":           "insufficient_primary_evidence",
			"reasons":          fixture.reasons,
			"distinct_symbols": []string{fixture.path + "\x00" + fixture.symbol},
		}
		opportunityCandidates = append(opportunityCandidates, map[string]any{
			"id": fixture.id, "title": fixture.title, "question_answered": fixture.question,
		})
		selected = append(selected, map[string]any{
			"candidate_id": fixture.id,
			"question":     fixture.question,
			"primary_path": map[string]any{
				"status":      "insufficient_primary_evidence",
				"eligibility": eligibility,
				"root_anchors": []any{map[string]any{
					"origin_fact_id": factID, "path": fixture.path, "symbol": fixture.symbol,
				}},
				"anchor_facts": []any{map[string]any{
					"id": factID,
					"source": map[string]any{
						"path": fixture.path, "start_line": fixture.line, "enclosing_symbol": fixture.symbol,
					},
				}},
			},
		})
		attempts = append(attempts, map[string]any{
			"candidate_id": fixture.id, "question": fixture.question,
			"state": "insufficient_primary_evidence", "failure_stage": "eligibility",
			"primary_eligibility": eligibility,
		})
	}
	for name, value := range map[string]any{
		freshRepoOpportunityFileForTopics: map[string]any{
			"validation_state":    "accepted",
			"normalized_proposal": map[string]any{"candidates": opportunityCandidates},
		},
		freshRepoDemoCandidatesFileForTopics: map[string]any{"selected": selected},
		freshRepoDemoStatusFileForTopics:     map[string]any{"attempts": attempts},
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseSnapshotPreservesExactPackageIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	writeTestFile(t, dir, "snapshot.json", `{
		"repo_name":"project",
		"go_facts":{
			"modules":[{"id":"module-v2","module_path":"github.com/example/project/v2","module_dir":".","display_name":"."}],
			"packages":[{
				"canonical_package_path":"github.com/example/project/v2/internal/server",
				"name":"server","owning_module_id":"module-v2","module_path":"github.com/example/project/v2",
				"package_directory":"internal/server","module_relative_path":"internal/server",
				"display_path":"internal/server","locality":"local"
			}]
		}
	}`)
	data := &ReportData{}
	if warning := parseSnapshot(path, data); warning != "" {
		t.Fatal(warning)
	}
	if data.RepositoryGraph == nil || data.RepositoryGraph.Version != 2 || len(data.RepositoryGraph.Packages) != 1 {
		t.Fatalf("repository graph = %#v", data.RepositoryGraph)
	}
	pkg := data.RepositoryGraph.Packages[0]
	if pkg.CanonicalPath != "github.com/example/project/v2/internal/server" || pkg.DisplayPath != "internal/server" || pkg.Locality != "local" {
		t.Fatalf("package identity = %#v", pkg)
	}
}

func TestConfidenceLabel(t *testing.T) {
	tests := []struct {
		conf float64
		want string
	}{
		{0.95, "strong"},
		{0.7, "strong"},
		{0.69, "medium"},
		{0.4, "medium"},
		{0.39, "weak"},
		{0.0, ""},
		{-1.0, ""},
		{math.NaN(), ""},
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
		{BundleStats{SelectedFilesCount: 12, SelectedTestsCount: 5, SelectedDocsCount: 3}, "12 source, 5 test, 3 doc"},
		{BundleStats{SelectedFilesCount: 0, SelectedTestsCount: 0, SelectedDocsCount: 0}, "0 source, 0 test, 0 doc"},
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

	if data.FormatVersion != CurrentFormatVersion {
		t.Errorf("format version = %d, want %d", data.FormatVersion, CurrentFormatVersion)
	}
	if data.FlowCount != 2 {
		t.Errorf("flow count = %d, want 2", data.FlowCount)
	}
	if data.RecommendedFlow != "a" {
		t.Errorf("recommended flow = %q, want a", data.RecommendedFlow)
	}
	if data.Flows[0].ConfidenceLabel != "strong" {
		t.Errorf("flow a confidence label = %q, want strong", data.Flows[0].ConfidenceLabel)
	}
	if data.Flows[1].ConfidenceLabel != "weak" {
		t.Errorf("flow b confidence label = %q, want weak", data.Flows[1].ConfidenceLabel)
	}
	if data.Flows[0].BundleStatsLabel != "10 source, 3 test, 2 doc" {
		t.Errorf("flow a stats label = %q", data.Flows[0].BundleStatsLabel)
	}
}

func TestMixedTopicShelfRendererIsPrimaryAndLive(t *testing.T) {
	t.Parallel()

	overviewStart := strings.Index(scriptJS, "function renderOverviewWorkspace()")
	if overviewStart < 0 {
		t.Fatal("renderOverviewWorkspace is missing")
	}
	overviewScript := scriptJS[overviewStart:]
	mixedIndex := strings.Index(overviewScript, "if (renderMixedLearningShelf(root)) return;")
	studyIndex := strings.Index(overviewScript, "if (STUDY_MAP)")
	if mixedIndex < 0 || studyIndex < 0 || mixedIndex >= studyIndex {
		t.Fatalf("mixed shelf / Study Map order = %d / %d", mixedIndex, studyIndex)
	}
	for _, liveControl := range []string{
		"card.onclick = function () {\n\t\t\tactiveOverviewTopicID = topic.candidate_id;",
		"button.onclick = function () { openSourceLocation(location); };",
		"card.onclick = function () { openUserMechanism(mechanism.artifact_id, 0); };",
	} {
		if !strings.Contains(scriptJS, liveControl) {
			t.Errorf("renderer lacks live control %q", liveControl)
		}
	}
	for _, forbidden := range []string{
		"addWorkspaceTab('Search'",
		"navigateWorkspace('search')",
		"if (!TASK_INVESTIGATION) mountSemanticSearch();",
	} {
		if strings.Contains(scriptJS, forbidden) {
			t.Errorf("normal report retains Search surface %q", forbidden)
		}
	}
	for _, removed := range []string{
		"rm-search-view",
		"mountSemanticSearch",
		"RepomapSemanticSearch",
		"DATA.semantic_search",
	} {
		if strings.Contains(scriptJS, removed) {
			t.Errorf("compiled report script retains removed Search code %q", removed)
		}
	}

	data := &ReportData{
		RepoName:      "go.etcd.io/etcd/v3",
		OpenablePaths: []string{"server/etcdserver/api/v3rpc/quota.go"},
		StudyMap:      &RepositoryStudyMap{},
		UserMechanisms: []UserMechanism{{
			ArtifactID: "semantic-artifact-003a27952d61f4735635a018",
			Title:      "Snapshot delivery",
			Question:   "How is a snapshot delivered?",
			Answer:     "The accepted path opens and sends the snapshot.",
			Steps: []UserMechanismStep{
				{Title: "Open", Locations: []UserCodeLocation{{Path: "server/etcdserver/api/v3rpc/quota.go", Line: 10}}},
				{Title: "Send", Locations: []UserCodeLocation{{Path: "server/etcdserver/api/v3rpc/quota.go", Line: 20}}},
			},
			Files: []UserCodeLocation{{Path: "server/etcdserver/api/v3rpc/quota.go", Line: 10}},
		}},
		UserTopics: []UserTopic{{
			CandidateID: "semantic-candidate-7d19808e04b2b7c7e49e02e3",
			Title:       "Storage Quota Enforcement on Writes",
			Question:    "How does the etcd server enforce storage quota?",
			StartingSymbols: []UserTopicSymbol{{
				Path: "server/etcdserver/api/v3rpc/quota.go", Symbol: "quotaKVServer.Txn", Line: 42,
			}},
			Uncertainty: "The observable result is not yet supported by exact local evidence.",
		}},
	}
	html, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{
		`"user_topics":[`,
		`"candidate_id":"semantic-candidate-7d19808e04b2b7c7e49e02e3"`,
		`"starting_symbols":[`,
		`"artifact_id":"semantic-artifact-003a27952d61f4735635a018"`,
		"Pick a path worth following.",
		"Open exact symbol",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered report lacks %q", want)
		}
	}
	for _, forbidden := range []string{"semantic_search", "rm-search-view", "RepomapSemanticSearch"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("rendered report retains removed Search code %q", forbidden)
		}
	}
}

func TestEnrichSeparatesDirectionFlowSurfaceAndAnchorCounts(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		Run: &RunInfo{SurfaceDiscoveryRan: true, SurfaceDiscoveryCount: 0},
		CandidateDirections: []CandidateDirection{
			{ID: "startup", Disposition: flowexplain.DirectionAccepted, LocalProof: testSavedTraceSession("startup", true)},
			{ID: "admin", Disposition: flowexplain.DirectionAccepted, LocalProof: testSavedTraceSession("admin", false)},
			{ID: "threshold", Disposition: flowexplain.DirectionRejected, DispositionReason: "low confidence"},
		},
		Flows: []FlowData{{ID: "startup"}, {ID: "admin"}, {ID: "threshold"}},
		ArchitectureGrounding: &ArchitectureGrounding{
			BehaviorAnchors: make([]ArchitectureBehaviorAnchor, 13),
		},
	}
	enrich(data)
	if data.Run.ProposedDirectionCount != 3 || data.Run.AcceptedDirectionCount != 2 ||
		data.Run.RejectedDirectionCount != 1 || data.Run.SavedFlowCount != 2 ||
		data.Run.EvidenceBundleCount != 1 ||
		data.Run.SurfaceDiscoveryCount != 0 || data.Run.ArchitectureAnchorCount != 13 {
		t.Fatalf("run counts = %#v", data.Run)
	}
	if !data.Flows[2].EvidenceOnly {
		t.Fatal("rejected threshold direction remained eligible for a visible flow tab")
	}
}

func TestEnrichDoesNotCountEvidenceBundleAsSavedFlowOrTrace(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		Run: &RunInfo{},
		Flows: []FlowData{
			{ID: "local", EvidenceOnly: true, FlowStatus: "local_only"},
			{ID: "saved", FlowStatus: "succeeded", Summary: "saved explanation"},
		},
	}
	enrich(data)

	if data.Run.EvidenceBundleCount != 1 || data.Run.SavedFlowCount != 0 ||
		data.Run.SavedTraceCount != 0 || data.Run.FailedTraceAttemptCount != 0 {
		t.Fatalf("evidence/saved accounting = %#v", data.Run)
	}
}

func TestReadRunDir_Integration(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"etcd","readme":"..."}`)
	writeTestFile(t, dir, "llm_bundle.json", `{
		"allowed_paths":["server/key.go","server/shared.go","server/put.go","storage/kv.go"],
		"source_signals":[{"path":"server/put.go","line":42}],
		"go":{
			"module_summaries":[{"module_path":"example.com/project","module_dir":"."}],
			"important_edges":[{"from":"example.com/project/server","to":"example.com/project/storage"}]
		}
	}`)
	writeTestFile(t, dir, "metadata.json", `{
		"created_at":"2026-07-10T12:00:00Z",
		"model":"deepseek-v4-flash",
		"prompt_version":"orientation-json-v4",
		"compact_context_bytes":12000,
		"external_request_bytes":18000,
		"provider_request_count":1,
		"candidate_direction_count":1,
		"provider_latency_ms":432
	}`)
	writeTestFile(t, dir, "orientation_report.json", `{
		"project_guess":"KV store",
		"confidence":0.88,
		"high_level_map":[{
			"name":"API server",
			"role":"boundary",
			"evidence":["server/put.go handles Put at line 42"],
			"why_it_matters":"accepts client writes"
		}],
		"first_files_to_open":[{
			"path":"server/put.go",
			"reason":"write entrypoint",
			"priority":1
		}],
		"candidate_flows":[{
			"name":"gRPC Put",
			"flow_type":"request",
			"trigger":"a client sends Put",
			"likely_entrypoint":"server/put.go",
			"likely_files":["server/put.go","storage/kv.go"],
			"why_interesting":"shows the write path",
			"evidence":["server/put.go handles Put at line 42"],
			"confidence":0.82
		}],
		"important_domain_words":[{
			"word":"revision",
			"guess":"logical version of stored state",
			"evidence":["storage/kv.go updates revisions"]
		}],
		"questions_for_human":["Which write guarantees matter most?"],
		"unverified_paths":[{
			"path":"legacy/put.go",
			"reason":"mentioned but outside the bounded context"
		}],
		"warnings":["w1"]
	}`)

	flowDir := filepath.Join(dir, "flows", "grpc-put")
	mkdirAll(t, flowDir)

	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "gRPC Put Request", "flow_type": "request"},
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
	if data.Run == nil || data.Run.Model != "deepseek-v4-flash" || data.Run.CompactContextBytes != 12000 ||
		data.Run.ExternalRequestBytes != 18000 || data.Run.ProviderRequestCount != 1 || data.Run.CandidateDirectionCount != 1 ||
		data.Run.ProviderLatencyMillis == nil || *data.Run.ProviderLatencyMillis != 432 {
		t.Fatalf("run info = %#v", data.Run)
	}
	if data.OrientationConfidence != 0.88 {
		t.Errorf("orientation confidence = %v, want 0.88", data.OrientationConfidence)
	}
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.Modules) != 1 ||
		data.RepositoryGraph.Modules[0] != (ModuleInfo{Path: "example.com/project", Dir: ""}) ||
		len(data.RepositoryGraph.PackageEdges) != 1 {
		t.Fatalf("repository graph = %#v", data.RepositoryGraph)
	}
	if len(data.HighLevelMap) != 1 {
		t.Fatalf("high-level map items = %d, want 1", len(data.HighLevelMap))
	}
	if item := data.HighLevelMap[0]; item.Name != "API server" || item.Role != componentmap.RoleBoundary ||
		item.WhyItMatters != "accepts client writes" || len(item.Evidence) != 1 {
		t.Errorf("high-level map item = %+v", item)
	}
	if len(data.FirstFilesToOpen) != 1 {
		t.Fatalf("first files to open = %d, want 1", len(data.FirstFilesToOpen))
	}
	if file := data.FirstFilesToOpen[0]; file.Path != "server/put.go" || file.Reason != "write entrypoint" || file.Priority != 1 {
		t.Errorf("first file to open = %+v", file)
	}
	if len(data.CandidateFlows) != 1 || data.CandidateFlows[0] != "gRPC Put" {
		t.Errorf("candidate_flows = %v", data.CandidateFlows)
	}
	if len(data.CandidateDirections) != 1 {
		t.Fatalf("candidate directions = %d, want 1", len(data.CandidateDirections))
	}
	direction := data.CandidateDirections[0]
	if direction.ID != "grpc-put" || direction.Name != "gRPC Put" || direction.Trigger != "a client sends Put" {
		t.Errorf("candidate direction identity = %+v", direction)
	}
	if direction.FlowType != flowexplain.FlowTypeRequest {
		t.Errorf("candidate direction flow type = %q, want request", direction.FlowType)
	}
	if direction.LikelyEntrypoint != "server/put.go" || direction.WhyInteresting != "shows the write path" || direction.Confidence != 0.82 {
		t.Errorf("candidate direction details = %+v", direction)
	}
	if len(direction.LikelyFiles) != 2 || direction.LikelyFiles[1] != "storage/kv.go" {
		t.Errorf("candidate direction likely files = %v", direction.LikelyFiles)
	}
	if len(direction.Evidence) != 1 || direction.Evidence[0] != "server/put.go:42 handles Put" {
		t.Errorf("candidate direction evidence = %v", direction.Evidence)
	}
	if len(data.ImportantDomainWords) != 1 {
		t.Fatalf("important domain words = %d, want 1", len(data.ImportantDomainWords))
	}
	if word := data.ImportantDomainWords[0]; word.Word != "revision" || word.Guess != "logical version of stored state" || len(word.Evidence) != 1 {
		t.Errorf("important domain word = %+v", word)
	}
	if len(data.QuestionsForHuman) != 1 || data.QuestionsForHuman[0] != "Which write guarantees matter most?" {
		t.Errorf("questions for human = %v", data.QuestionsForHuman)
	}
	if len(data.OrientationUnverifiedPaths) != 1 {
		t.Fatalf("orientation unverified paths = %d, want 1", len(data.OrientationUnverifiedPaths))
	}
	if item := data.OrientationUnverifiedPaths[0]; item.Path != "legacy/put.go" || item.Reason == "" {
		t.Errorf("orientation unverified path = %+v", item)
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
	if f.FlowType != flowexplain.FlowTypeRequest {
		t.Errorf("flow type = %q, want request", f.FlowType)
	}
	if f.Summary != "handles gRPC put" {
		t.Errorf("summary = %q", f.Summary)
	}
	if f.Confidence != 0.85 {
		t.Errorf("confidence = %f", f.Confidence)
	}
	if f.ConfidenceLabel != "strong" {
		t.Errorf("confidence label = %q, want strong", f.ConfidenceLabel)
	}
	if f.BundleStatsLabel != "1 source, 1 test, 1 doc" {
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
	if data.FormatVersion != CurrentFormatVersion {
		t.Errorf("format version = %d, want %d", data.FormatVersion, CurrentFormatVersion)
	}
	for _, want := range []string{"server/key.go", "server/shared.go", "a_test.go"} {
		found := false
		for _, path := range data.OpenablePaths {
			if path == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("openable paths %v do not contain %q", data.OpenablePaths, want)
		}
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

func TestReadRunDir_LocalDirectionBundleIsNotAFlowError(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "worker-run")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed":{"id":"worker-run","name":"Worker run"},
		"selected_files":[{"path":"internal/worker/worker.go","reasons":["likely_file from candidate_flow"]}],
		"selected_tests":[{"path":"internal/worker/worker_test.go","reasons":["matched worker"]}],
		"selected_docs":[],
		"selected_packages":["example.com/test/internal/worker"],
		"related_edges":[]
	}`)
	writeTestFile(t, flowDir, "flow_status.json", `{"version":1,"mode":"local_only"}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("flows = %d, want 1", len(data.Flows))
	}
	flow := data.Flows[0]
	if !flow.EvidenceOnly || flow.Error != "" {
		t.Fatalf("local direction flow = %#v", flow)
	}
	if flow.Name != "Worker run" || len(flow.BundleFiles) != 1 || len(flow.BundleTests) != 1 {
		t.Fatalf("local direction evidence = %#v", flow)
	}
}

func TestReadRunDir_RequestedExpansionWithoutReportIsNotLocalOnly(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "worker-run")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed":{"id":"worker-run","name":"Worker run"},
		"selected_files":[{"path":"internal/worker/worker.go"}],
		"selected_tests":[],
		"selected_docs":[],
		"selected_packages":[],
		"related_edges":[]
	}`)
	writeTestFile(t, flowDir, "flow_status.json", `{"version":1,"mode":"expansion_requested"}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("flows = %d, want 1", len(data.Flows))
	}
	flow := data.Flows[0]
	if flow.EvidenceOnly || flow.Error == "" || flow.FlowStatus != "expansion_requested" {
		t.Fatalf("incomplete expansion was misclassified: %#v", flow)
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
	writeTestFile(t, flowDir, "flow_status.json", `{"version":1,"mode":"local_only"}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(data.Flows))
	}
	f := data.Flows[0]
	if !f.EvidenceOnly || f.Error != "" {
		t.Fatalf("bundle-only flow should be local evidence, got %#v", f)
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
		RepoName:              "test",
		FormatVersion:         CurrentFormatVersion,
		OrientationConfidence: 0.8,
		HighLevelMap: []Subsystem{{
			Name:         "runtime",
			Evidence:     []string{"main.go"},
			WhyItMatters: "starts the process",
		}},
		FirstFilesToOpen: []FileItem{{Path: "main.go", Reason: "entrypoint", Priority: 1}},
		ImportantDomainWords: []DomainWord{{
			Word:     "worker",
			Guess:    "background processor",
			Evidence: []string{"worker.go"},
		}},
		QuestionsForHuman:          []string{"Which path matters?"},
		OrientationUnverifiedPaths: []PathItem{{Path: "legacy.go", Reason: "not verified"}},
		Flows: []FlowData{{
			ID:               "f1",
			Confidence:       0.5,
			FilesToRead:      []FileItem{{Path: "x.go", Reason: "test", Priority: 1}},
			ConfidenceLabel:  "medium",
			BundleStatsLabel: "1 source, 0 test, 0 doc",
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
	if got.Flows[0].ConfidenceLabel != "medium" {
		t.Errorf("round-trip: confidence_label = %q", got.Flows[0].ConfidenceLabel)
	}
	if got.RecommendedFlow != "f1" {
		t.Errorf("round-trip: recommended_flow = %q", got.RecommendedFlow)
	}
	if got.OrientationConfidence != 0.8 || len(got.HighLevelMap) != 1 || len(got.FirstFilesToOpen) != 1 {
		t.Errorf("round-trip orientation overview = %+v", got)
	}
	if len(got.ImportantDomainWords) != 1 || len(got.QuestionsForHuman) != 1 || len(got.OrientationUnverifiedPaths) != 1 {
		t.Errorf("round-trip orientation guidance = %+v", got)
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

func TestParseFlowReport_StringFilesToRead(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "str-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "String Flow"},
		"selected_files": [{"path":"a.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "string based",
		"confidence": 0.5,
		"files_to_read_in_order": ["a.go", "b.go"],
		"tests_to_read": ["a_test.go"],
		"unverified_paths": ["maybe.go"]
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	if len(data.Flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(data.Flows))
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.FilesToRead) != 2 {
		t.Fatalf("expected 2 files to read, got %d", len(f.FilesToRead))
	}
	if f.FilesToRead[0].Path != "a.go" || f.FilesToRead[0].Priority != 1 {
		t.Errorf("file 0: path=%q pri=%d", f.FilesToRead[0].Path, f.FilesToRead[0].Priority)
	}
	if f.FilesToRead[1].Path != "b.go" || f.FilesToRead[1].Priority != 2 {
		t.Errorf("file 1: path=%q pri=%d", f.FilesToRead[1].Path, f.FilesToRead[1].Priority)
	}
	if len(f.TestsToRead) != 1 || f.TestsToRead[0].Path != "a_test.go" {
		t.Errorf("tests to read: %+v", f.TestsToRead)
	}
	if len(f.UnverifiedPaths) != 1 || f.UnverifiedPaths[0].Path != "maybe.go" || f.UnverifiedPaths[0].Reason != "" {
		t.Errorf("unverified paths: %+v", f.UnverifiedPaths)
	}
}

func TestParseFlowReport_StringEvidenceFiles(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "ev-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Evidence Flow"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "evidence test",
		"confidence": 0.5,
		"likely_chain": [{"step":1,"name":"start","what_happens":"begins","evidence_files":["a.go","b.go"],"confidence":0.8}],
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 {
		t.Fatalf("expected 1 chain step, got %d", len(f.LikelyChain))
	}
	cs := f.LikelyChain[0]
	if len(cs.EvidenceFiles) != 2 || cs.EvidenceFiles[0] != "a.go" || cs.EvidenceFiles[1] != "b.go" {
		t.Errorf("evidence files: %v", cs.EvidenceFiles)
	}
}

func TestParseFlowReport_ObjectEvidenceFiles(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "ev-obj")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Evidence Obj"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "object evidence",
		"confidence": 0.5,
		"likely_chain": [{"step":1,"name":"start","what_happens":"begins","evidence_files":[{"path":"a.go"},{"path":"b.go"}],"confidence":0.8}],
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 {
		t.Fatalf("expected 1 chain step")
	}
	cs := f.LikelyChain[0]
	if len(cs.EvidenceFiles) != 2 || cs.EvidenceFiles[0] != "a.go" || cs.EvidenceFiles[1] != "b.go" {
		t.Errorf("evidence files from objects: %v", cs.EvidenceFiles)
	}
}

func TestParseFlowReport_MalformedFieldReturnsError(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "bad-field")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Bad Field"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "bad field",
		"confidence": 0.5,
		"files_to_read_in_order": 42,
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error == "" {
		t.Fatal("expected error for malformed files_to_read_in_order")
	}
}

func TestParseFlowBundle_FallbackFields(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "fb-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Bundle Fallback"},
		"selected_files": [{"path":"a.go","kind":"source","score":200,"reasons":["entrypoint"]}],
		"selected_tests": [{"path":"a_test.go","kind":"test","score":100,"reasons":["covers handler"]}],
		"selected_docs": [{"path":"doc.md","kind":"doc","score":50,"reasons":[]}],
		"selected_packages": ["pkg/a", "pkg/b"],
		"related_edges": [{"from":"a","to":"b"}]
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "some flow",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]

	if len(f.BundleFiles) != 1 || f.BundleFiles[0].Path != "a.go" {
		t.Errorf("bundle files: %+v", f.BundleFiles)
	}
	if f.BundleFiles[0].Reason != "entrypoint" {
		t.Errorf("bundle file reason: %q", f.BundleFiles[0].Reason)
	}
	if len(f.BundleTests) != 1 || f.BundleTests[0].Path != "a_test.go" {
		t.Errorf("bundle tests: %+v", f.BundleTests)
	}
	if len(f.BundleDocs) != 1 || f.BundleDocs[0].Path != "doc.md" {
		t.Errorf("bundle docs: %+v", f.BundleDocs)
	}
	if len(f.BundlePackages) != 2 || f.BundlePackages[0] != "pkg/a" {
		t.Errorf("bundle packages: %v", f.BundlePackages)
	}
	if len(f.BundleEdges) != 1 || f.BundleEdges[0].From != "a" || f.BundleEdges[0].To != "b" {
		t.Errorf("bundle edges: %+v", f.BundleEdges)
	}
}

func TestRender_ErrorFlowHasFallbackContent(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "broken-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"testfail"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Broken Flow"},
		"selected_files": [{"path":"main.go","kind":"source","score":200,"reasons":["entrypoint"]}],
		"selected_tests": [{"path":"main_test.go","kind":"test","score":100,"reasons":[]}],
		"selected_docs": [],
		"selected_packages": ["main"],
		"related_edges": [{"from":"main","to":"pkg/util"}]
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{"invalid json in flow report`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	jsonData, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	content := string(jsonData)

	if !contains(content, `"error"`) {
		t.Error("error flow should have error field in report.json")
	}
	if !contains(content, `"bundle_files"`) {
		t.Error("report.json missing bundle_files for error flow")
	}
	if !contains(content, `"main.go"`) {
		t.Error("report.json missing selected file path in bundle_files")
	}
	if !contains(content, `"main_test.go"`) {
		t.Error("report.json missing selected test path in bundle_tests")
	}
	if !contains(content, `"bundle_packages"`) {
		t.Error("report.json missing bundle_packages")
	}
	if !contains(content, `"main"`) {
		t.Error("report.json missing package name")
	}
	if !contains(content, `"bundle_edges"`) {
		t.Error("report.json missing bundle_edges")
	}
	if !contains(content, `"from": "main"`) {
		t.Error("report.json missing edge from")
	}

	html, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatalf("read report.html: %v", err)
	}
	htmlStr := string(html)
	if !contains(htmlStr, "testfail") {
		t.Error("report.html missing repo name")
	}
}

func TestRender_HealthyFlowNoFallbackHeading(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "good-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"testgood"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Good Flow"},
		"selected_files": [{"path":"g.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "works fine",
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

	jsonData, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}

	if contains(string(jsonData), `"error"`) {
		t.Error("healthy flow should not have error field in report.json")
	}
	if !contains(string(jsonData), `"summary": "works fine"`) {
		t.Error("report.json missing flow summary")
	}
	if !contains(string(jsonData), `"g.go"`) {
		t.Error("report.json missing file path")
	}
}

func TestReportJSON_HasFileReasons(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "reason-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"reasontest"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Reason Flow"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "reasons included",
		"confidence": 0.8,
		"files_to_read_in_order": [
			{"path":"server/main.go","reason":"Entrypoint of the etcd binary; initiates startup","priority":1},
			{"path":"server/etcdmain/main.go","reason":"Main function; orchestrates startup","priority":2}
		],
		"tests_to_read": [{"path":"main_test.go","reason":"integration test"}],
		"likely_chain": [
			{"step":1,"name":"startup","what_happens":"","evidence_files":["server/main.go"],"confidence":0.8},
			{"step":2,"name":"","what_happens":"","evidence_files":["server/etcdmain/main.go"],"confidence":0.7}
		],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	jsonData, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	content := string(jsonData)

	if !contains(content, `"Entrypoint of the etcd binary; initiates startup"`) {
		t.Error("report.json missing file reason in files_to_read_in_order")
	}
	if !contains(content, `"Main function; orchestrates startup"`) {
		t.Error("report.json missing second file reason")
	}
	if !contains(content, `"integration test"`) {
		t.Error("report.json missing test reason")
	}
	if !contains(content, `"server/main.go"`) {
		t.Error("report.json missing evidence file path")
	}
}

func TestReportJSON_ZeroConfidenceNotEstimated(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "zero-conf")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"zerotest"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Zero Conf"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "zero confidence flow",
		"confidence": 0,
		"files_to_read_in_order": [{"path":"z.go","reason":"zero"}],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	jsonData, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}

	if contains(string(jsonData), `"confidence_label": "Low"`) {
		t.Error("zero confidence should not produce 'Low' label")
	}
	if contains(string(jsonData), `"confidence_label": "weak"`) {
		t.Error("zero confidence should not produce 'weak' label")
	}
	if contains(string(jsonData), `"0%"`) {
		t.Error("report.json should not contain misleading 0%")
	}
}

func TestReportJSON_BundleStatsLabelReadable(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "bs-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"bstest"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "BS Flow"},
		"selected_files": [{"path":"a.go","kind":"source","score":200}],
		"selected_tests": [{"path":"a_test.go","kind":"test","score":100}],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "bundle stats",
		"confidence": 0.5,
		"files_to_read_in_order": [{"path":"a.go","reason":"entry"}],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	jsonData, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	content := string(jsonData)

	if !contains(content, `"bundle_stats_label": "1 source, 1 test, 0 doc"`) {
		t.Error("report.json missing readable bundle stats label")
	}
}

func TestGenerate_HTMLContainsFileReason(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "ft-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"fttest"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "FT Flow"},
		"selected_files": [{"path":"server/main.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "file test",
		"confidence": 0.8,
		"files_to_read_in_order": [
			{"path":"server/main.go","reason":"Entrypoint of the etcd binary; initiates startup","priority":1}
		],
		"tests_to_read": [],
		"likely_chain": [
			{"step":1,"name":"startup","what_happens":"","evidence_files":["server/main.go"],"confidence":0.8}
		],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	jsonData, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	content := string(jsonData)

	if !contains(content, "Entrypoint of the etcd binary; initiates startup") {
		t.Error("report.json missing file reason from files_to_read_in_order")
	}
	if !contains(content, "server/main.go") {
		t.Error("report.json missing file path")
	}

	html, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatalf("read report.html: %v", err)
	}
	htmlStr := string(html)

	if !contains(htmlStr, "fttest") {
		t.Error("report.html missing repo name")
	}
	if !contains(htmlStr, "server/main.go") {
		t.Error("report.html missing file path")
	}
	if containsLegacySingleLetterLabel(html) {
		t.Error("report.html contains single-letter abbreviations")
	}
}

func TestChainStep_IntStep(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "int-step")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Int Step"},
		"selected_files": [{"path":"a.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "int step",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [{"step":1,"name":"start","what_happens":"init","evidence_files":[],"confidence":0.8}],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 || f.LikelyChain[0].Step != 1 {
		t.Errorf("expected step 1, got %d", f.LikelyChain[0].Step)
	}
}

func TestChainStep_StringStep(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "str-step")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Str Step"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "str step",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [{"step":"2","name":"second","what_happens":"runs","evidence_files":[],"confidence":0.8}],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 || f.LikelyChain[0].Step != 2 {
		t.Errorf("expected step 2, got %d", f.LikelyChain[0].Step)
	}
}

func TestChainStep_StepPrefix(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "step-prefix")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Step Prefix"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "step prefix",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [{"step":"Step 3","name":"third","what_happens":"finishes","evidence_files":[],"confidence":0.8}],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 || f.LikelyChain[0].Step != 3 {
		t.Errorf("expected step 3, got %d", f.LikelyChain[0].Step)
	}
}

func TestChainStep_MissingStep(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "missing-step")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Missing Step"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "missing step",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [
			{"name":"first","what_happens":"starts","evidence_files":[],"confidence":0.8},
			{"name":"second","what_happens":"ends","evidence_files":[],"confidence":0.8}
		],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(f.LikelyChain))
	}
	if f.LikelyChain[0].Step != 1 {
		t.Errorf("expected auto step 1, got %d", f.LikelyChain[0].Step)
	}
	if f.LikelyChain[1].Step != 2 {
		t.Errorf("expected auto step 2, got %d", f.LikelyChain[1].Step)
	}
}

func TestChainStep_ZeroStepBecomesIndex(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "zero-step")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Zero Step"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "zero step",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [
			{"step":0,"name":"one","what_happens":"first","evidence_files":[],"confidence":0.8},
			{"step":0,"name":"two","what_happens":"second","evidence_files":[],"confidence":0.8}
		],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 2 {
		t.Fatalf("expected 2 steps")
	}
	if f.LikelyChain[0].Step != 1 {
		t.Errorf("expected zero→1, got %d", f.LikelyChain[0].Step)
	}
	if f.LikelyChain[1].Step != 2 {
		t.Errorf("expected zero→2, got %d", f.LikelyChain[1].Step)
	}
}

func TestPathValidation_UnverifiedPathWarns(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "unver-path")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Unverified Path"},
		"selected_files": [{"path":"real.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "unverified path test",
		"confidence": 0.5,
		"files_to_read_in_order": [{"path":"hallucinated.go","reason":"made up","priority":1}],
		"tests_to_read": [],
		"likely_chain": [{"step":1,"name":"start","what_happens":"begins","evidence_files":["fake.go"],"confidence":0.8}],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": ["llm warning"]
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.FilesToRead) != 1 || f.FilesToRead[0].Path != "hallucinated.go" {
		t.Errorf("hallucinated path should still be present: %+v", f.FilesToRead)
	}
	if len(f.LikelyChain) != 1 {
		t.Fatalf("expected 1 chain step")
	}
	foundFileWarn := false
	foundEvidenceWarn := false
	foundLLMWarn := false
	for _, w := range f.Warnings {
		if contains(w, "unverified path in files_to_read_in_order: hallucinated.go") {
			foundFileWarn = true
		}
		if contains(w, "unverified path in likely_chain evidence: fake.go") {
			foundEvidenceWarn = true
		}
		if contains(w, "llm warning") {
			foundLLMWarn = true
		}
	}
	if !foundFileWarn {
		t.Error("expected warning for hallucinated.go in files_to_read_in_order")
	}
	if !foundEvidenceWarn {
		t.Error("expected warning for fake.go in evidence_files")
	}
	if !foundLLMWarn {
		t.Error("LLM warning should be preserved")
	}
}

func TestPathValidation_VerifiedPathNoWarn(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "ver-path")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Verified Path"},
		"selected_files": [{"path":"real.go","kind":"source","score":200}, {"path":"other.go","kind":"source","score":100}],
		"selected_tests": [{"path":"real_test.go","kind":"test","score":100}],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "verified paths",
		"confidence": 0.5,
		"files_to_read_in_order": [{"path":"real.go","reason":"entrypoint","priority":1}],
		"tests_to_read": [{"path":"real_test.go","reason":"covers"}],
		"likely_chain": [{"step":1,"name":"start","what_happens":"begins","evidence_files":["other.go"],"confidence":0.8}],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	for _, w := range f.Warnings {
		if contains(w, "unverified path") {
			t.Errorf("verified paths should not produce unverified warnings: %s", w)
		}
	}
}

func TestParseFlowReport_UnknownsAsObjects(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "unk-obj")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Unknowns Obj"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "object unknowns",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [
			{"uncertainty": "Exact order of calls", "reason": "Not explicitly shown"},
			{"description": "Another unknown", "reason": "Details are unclear"}
		],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s (unknowns as objects should not fail)", f.Error)
	}
	if len(f.Unknowns) != 2 {
		t.Fatalf("expected 2 unknowns, got %d: %v", len(f.Unknowns), f.Unknowns)
	}
	if f.Unknowns[0] != "Exact order of calls — Not explicitly shown" {
		t.Errorf("unknowns[0] = %q", f.Unknowns[0])
	}
}

func TestParseFlowReport_WarningsAsObjects(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "warn-obj")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Warnings Obj"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "object warnings",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": [
			{"warning": "The flow assumes leader", "reason": "Not explicit"},
			{"message": "Tests may be incomplete", "reason": "Coverage unknown"}
		]
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s (warnings as objects should not fail)", f.Error)
	}
	if len(f.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(f.Warnings), f.Warnings)
	}
	if f.Warnings[0] != "The flow assumes leader — Not explicit" {
		t.Errorf("warnings[0] = %q", f.Warnings[0])
	}
}

func TestParseFlowReport_UnknownsAsMap(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "unk-map")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Unknowns Map"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "map unknowns",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": {
			"performance": ["Latency under load is unknown"],
			"correctness": ["Raft consensus details unclear"]
		},
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s (unknowns as map should not fail)", f.Error)
	}
	if len(f.Unknowns) != 2 {
		t.Fatalf("expected 2 unknowns, got %d: %v", len(f.Unknowns), f.Unknowns)
	}
}

func TestChainStep_DescriptionFallback(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "desc-fb")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Desc Fallback"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "description fallback",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [
			{"description": "1. server/main.go parses config and calls start().", "evidence_files": ["server/main.go"], "confidence": 0.8}
		],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 {
		t.Fatalf("expected 1 chain step, got %d", len(f.LikelyChain))
	}
	cs := f.LikelyChain[0]
	if cs.WhatHappens != "1. server/main.go parses config and calls start()." {
		t.Errorf("what_happens from description = %q", cs.WhatHappens)
	}
	if cs.Name != "1. server/main.go parses config and calls start()." {
		t.Errorf("name from description = %q", cs.Name)
	}
}

func TestChainStep_ReasonFallback(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "reason-fb")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Reason Fallback"},
		"selected_files": [{"path":"key.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "reason fallback",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [
			{"step": "gRPC request arrives", "evidence_files": ["key.go"], "reason": "key.go implements the Put handler"}
		],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 1 {
		t.Fatalf("expected 1 chain step, got %d", len(f.LikelyChain))
	}
	cs := f.LikelyChain[0]
	if cs.WhatHappens != "key.go implements the Put handler" {
		t.Errorf("what_happens from reason = %q", cs.WhatHappens)
	}
	if cs.Step != 1 {
		t.Errorf("expected step 1 (fallback from non-numeric), got %d", cs.Step)
	}
}

func TestParseFlowReport_UnknownsEmptyStrings(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "unk-empty")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Empty Unknowns"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "empty unknowns",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": null
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if f.Unknowns != nil && len(f.Unknowns) > 0 {
		t.Errorf("expected empty/nil unknowns, got %v", f.Unknowns)
	}
	if f.Warnings != nil {
		t.Errorf("expected nil warnings for null, got %v", f.Warnings)
	}
}

func TestReportHTML_NoLow0Percent(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"nope"}`)
	writeTestFile(t, dir, "orientation_report.json", `{"project_guess":"","candidate_flows":[],"warnings":[]}`)

	flowDir := filepath.Join(dir, "flows", "zero-flow")
	mkdirAll(t, flowDir)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Zero Flow"},
		"selected_files": [{"path":"z.go","kind":"source","score":200}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "zero confidence flow",
		"confidence": 0,
		"files_to_read_in_order": [{"path":"z.go","reason":"entrypoint"}],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	if err := Generate(dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	html, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatalf("read report.html: %v", err)
	}
	htmlStr := string(html)

	if contains(htmlStr, "Low 0%") {
		t.Error("report.html contains 'Low 0%'")
	}
	if contains(htmlStr, "cannot unmarshal") {
		t.Error("report.html contains unmarshal error")
	}
	if !contains(htmlStr, "Model confidence: not estimated") {
		t.Error("report.html should show 'Model confidence: not estimated' for zero confidence")
	}
	if !contains(htmlStr, "z.go") {
		t.Error("report.html missing file path from zero-confidence flow")
	}
}

func TestParseFlowReport_FullDriftScenario(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "full-drift")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"drift"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Full Drift"},
		"selected_files": [{"path":"a.go","kind":"source","score":200}, {"path":"b.go","kind":"source","score":150}],
		"selected_tests": [{"path":"a_test.go","kind":"test","score":100}, {"path":"b_test.go","kind":"test","score":50}],
		"selected_docs": [],
		"selected_packages": ["pkg"],
		"related_edges": []
	}`)
	// Simulate every drift: object unknowns, object warnings, description/reason on chain,
	// non-numeric step, string-only tests_to_read, object evidence_files
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "full drift test",
		"confidence": 0.6,
		"files_to_read_in_order": [
			{"path":"a.go","reason":"entrypoint","priority":1},
			{"path":"b.go","reason":"handler"}
		],
		"tests_to_read": ["a_test.go", "b_test.go"],
		"likely_chain": [
			{
				"step": "init",
				"description": "Phase 1: initialize the server and load configuration.",
				"evidence_files": [{"path":"a.go"}, {"path":"b.go"}],
				"confidence": 0.7
			},
			{
				"step": "execute",
				"reason": "Phase 2: handle the actual request logic.",
				"evidence_files": ["a.go"],
				"confidence": 0.5
			}
		],
		"unverified_paths": ["legacy.go"],
		"unknowns": [
			{"uncertainty": "Exact order of calls", "reason": "Not shown"},
			{"description": "Role of interceptor", "reason": "Unclear"}
		],
		"warnings": [
			{"warning": "Assumes leader", "reason": "Not explicit"},
			{"message": "Tests incomplete", "reason": "Coverage unknown"}
		]
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s (all drifts should be tolerated)", f.Error)
	}

	// Check unknowns normalized
	if len(f.Unknowns) != 2 {
		t.Fatalf("expected 2 unknowns, got %d", len(f.Unknowns))
	}
	// Check warnings normalized
	if len(f.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(f.Warnings))
	}
	// Check chain: 2 steps
	if len(f.LikelyChain) != 2 {
		t.Fatalf("expected 2 chain steps, got %d", len(f.LikelyChain))
	}
	// Step 1: description → what_happens, name from description
	if f.LikelyChain[0].WhatHappens != "Phase 1: initialize the server and load configuration." {
		t.Errorf("step 1 what_happens = %q", f.LikelyChain[0].WhatHappens)
	}
	// Step 2: reason → what_happens
	if f.LikelyChain[1].WhatHappens != "Phase 2: handle the actual request logic." {
		t.Errorf("step 2 what_happens = %q", f.LikelyChain[1].WhatHappens)
	}
	// Evidence files from objects
	if len(f.LikelyChain[0].EvidenceFiles) != 2 {
		t.Errorf("step 1 evidence files = %v", f.LikelyChain[0].EvidenceFiles)
	}
	// files_to_read_in_order
	if len(f.FilesToRead) != 2 {
		t.Fatalf("expected 2 files to read, got %d", len(f.FilesToRead))
	}
	if f.FilesToRead[0].Priority != 1 || f.FilesToRead[1].Priority != 0 {
		t.Errorf("priorities: file0=%d file1=%d", f.FilesToRead[0].Priority, f.FilesToRead[1].Priority)
	}
	// tests_to_read as strings
	if len(f.TestsToRead) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(f.TestsToRead))
	}
	if f.TestsToRead[0].Path != "a_test.go" || f.TestsToRead[1].Path != "b_test.go" {
		t.Errorf("tests: %+v", f.TestsToRead)
	}
	// unverified_paths as strings
	if len(f.UnverifiedPaths) != 1 || f.UnverifiedPaths[0].Path != "legacy.go" {
		t.Errorf("unverified paths: %+v", f.UnverifiedPaths)
	}
}

func TestParseFlowReport_LikelyChainAsObjectWithSteps(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "lc-obj")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "LC Obj"},
		"selected_files": [{"path":"grpc.go","kind":"source","score":200}, {"path":"quota.go","kind":"source","score":150}],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "object likely_chain",
		"confidence": 0.7,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": {
			"summary": "Registration -> Check -> Handler",
			"steps": [
				{
					"role": "gRPC registration",
					"file": "grpc.go",
					"function": "RegisterKVServer",
					"evidence_files": ["grpc.go"]
				},
				{
					"role": "quota check",
					"file": "quota.go",
					"function": "quotaKVServer.Put",
					"evidence_files": ["quota.go"]
				}
			]
		},
		"unverified_paths": [],
		"unknowns": [],
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.LikelyChain) != 2 {
		t.Fatalf("expected 2 chain steps, got %d", len(f.LikelyChain))
	}
	if f.LikelyChain[0].Name != "gRPC registration" {
		t.Errorf("step 0 name = %q", f.LikelyChain[0].Name)
	}
	if f.LikelyChain[0].WhatHappens != "gRPC registration: RegisterKVServer" {
		t.Errorf("step 0 what_happens = %q", f.LikelyChain[0].WhatHappens)
	}
	if f.LikelyChain[1].Name != "quota check" {
		t.Errorf("step 1 name = %q", f.LikelyChain[1].Name)
	}
}

func TestParseFlowReport_UnknownsAsBareString(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "unk-str")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Unknowns Str"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "bare string unknowns",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": "Only one thing is unclear here.",
		"warnings": []
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.Unknowns) != 1 {
		t.Fatalf("expected 1 unknown, got %d: %v", len(f.Unknowns), f.Unknowns)
	}
	if f.Unknowns[0] != "Only one thing is unclear here." {
		t.Errorf("unknowns[0] = %q", f.Unknowns[0])
	}
}

func TestParseFlowReport_WarningsAsBareString(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flows", "warn-str")
	mkdirAll(t, flowDir)
	writeTestFile(t, dir, "snapshot.json", `{"repo_name":"test"}`)
	writeTestFile(t, flowDir, "flow_bundle.json", `{
		"flow_seed": {"name": "Warnings Str"},
		"selected_files": [],
		"selected_tests": [],
		"selected_docs": [],
		"selected_packages": [],
		"related_edges": []
	}`)
	writeTestFile(t, flowDir, "flow_report.json", `{
		"summary": "bare string warning",
		"confidence": 0.5,
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"likely_chain": [],
		"unverified_paths": [],
		"unknowns": [],
		"warnings": "Single warning as a string."
	}`)

	data, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	f := data.Flows[0]
	if f.Error != "" {
		t.Fatalf("unexpected error: %s", f.Error)
	}
	if len(f.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(f.Warnings), f.Warnings)
	}
	if f.Warnings[0] != "Single warning as a string." {
		t.Errorf("warnings[0] = %q", f.Warnings[0])
	}
}

func TestBoundedDocumentedPurposeSkipsReadmeChrome(t *testing.T) {
	readme := "Project\n![status](badge.svg)\n==========\n\n" +
		"Project safely copies database changes to durable storage.\n" +
		"It runs beside the application.\n\n" +
		"## Installation\nRun the installer.\n"
	want := "Project safely copies database changes to durable storage. It runs beside the application."
	if got := boundedDocumentedPurpose(readme); got != want {
		t.Fatalf("documented purpose = %q, want %q", got, want)
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
