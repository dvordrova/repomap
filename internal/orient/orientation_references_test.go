package orient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func orientationWireFromHTTPRequest(t *testing.T, request *http.Request) orientationWireBundle {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	return orientationWireFromRequestBytes(t, body)
}

func orientationWireFromRequestBytes(t *testing.T, body []byte) orientationWireBundle {
	t.Helper()
	var envelope struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Messages) == 0 {
		t.Fatal("orientation request has no messages")
	}
	const marker = "Orientation facts bundle JSON:\n"
	content := envelope.Messages[len(envelope.Messages)-1].Content
	index := strings.LastIndex(content, marker)
	if index < 0 {
		t.Fatalf("orientation wire marker is missing: %s", content)
	}
	var wire orientationWireBundle
	if err := json.Unmarshal([]byte(strings.TrimSpace(content[index+len(marker):])), &wire); err != nil {
		t.Fatalf("decode orientation wire bundle: %v", err)
	}
	return wire
}

func orientationWireFileRefs(t *testing.T, wire orientationWireBundle, path string) (string, string) {
	t.Helper()
	fileRef := ""
	for _, file := range wire.FileIndex {
		if file.Path == path {
			fileRef = file.FileRef
			break
		}
	}
	if fileRef == "" {
		t.Fatalf("wire has no file ref for %q", path)
	}
	for _, candidate := range wire.CandidateFileIndex {
		if candidate.FileRef == fileRef {
			return fileRef, candidate.EvidenceRef
		}
	}
	t.Fatalf("wire has no candidate evidence ref for %q", path)
	return "", ""
}

func referenceFixtureBundle(t *testing.T) llmbundle.Bundle {
	t.Helper()
	raw := []byte(`{
  "repo_name":"fixture",
  "readme_excerpt":"A bounded command service.",
  "top_level_directory_stats":{"internal":2,"cmd":1},
  "language_hints":[{"language":"Go","files":5}],
  "go":{
    "modules_count":1,
    "packages_count":3,
    "module_summaries":[],
    "entrypoints":[{
      "kind":"command",
      "import_path":"example.com/fixture/cmd",
      "package_dir":"cmd",
      "anchors":[{"version":1,"kind":"go_main_function","path":"cmd/root.go","line":10}],
      "open_files":["cmd/root.go","cmd/main.go"]
    }],
    "command_traces":[{
      "version":2,
      "framework":"cobra",
      "entrypoint_package":"example.com/fixture/cmd",
      "command":"serve",
      "steps":[{
        "symbol":"newServeCommand",
        "relation":"registers_command",
        "callsite_location":{"path":"cmd/main.go","line":12},
        "target_location":{"path":"cmd/root.go","line":20}
      }],
      "handler_calls":[{
        "symbol":"runServer",
        "path":"internal/handler.go",
        "line":30,
        "relation":"calls",
        "resolved":true,
        "target_path":"internal/worker.go",
        "target_line":40
      }],
      "concurrency":"unknown",
      "complete":true
    }],
    "orientation_candidates":[{
      "name":"Background worker",
      "kind":"signal_flow",
      "entrypoint_package":"example.com/fixture/cmd",
      "open_files":["internal/worker.go"],
      "why":"periodic work is visible",
      "priority":10
    }],
    "important_edges":[{"from":"example.com/fixture/cmd","to":"example.com/fixture/internal/config.go"}]
  },
  "known_docs":["docs/guide.md"],
  "candidate_file_index":[{
    "id":"file-main-long-canonical-id",
    "path":"cmd/main.go",
    "kind":"entrypoint",
    "signals":["entrypoint"],
    "score":100,
    "reasons":["process entrypoint"]
  }],
  "allowed_paths":["cmd/main.go"],
  "source_signals":[{
    "path":"internal/worker.go",
    "line":40,
    "category":"background_loop",
    "match":"ticker",
    "snippet":"ticker := time.NewTicker(interval)",
    "weight":40,
    "reason":"periodic ticker created"
  }],
  "research_policy_version":"bounded-research-v1",
  "local_authorized_file_count":5
}`)
	var bundle llmbundle.Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func validReferenceResponse(t *testing.T, catalog orientationReferenceCatalog) orientationProviderResponse {
	t.Helper()
	fileRef, err := catalog.fileRef("cmd/main.go")
	if err != nil {
		t.Fatal(err)
	}
	evidenceRef, err := catalog.evidenceRef(
		"candidate_file",
		orientationCandidateFileEvidence("cmd/main.go", "entrypoint", []string{"entrypoint"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return orientationProviderResponse{
		ProjectGuess: "bounded command service",
		Confidence:   0.72,
		HighLevelMap: []orientationProviderMapItem{{
			Name: "command boundary", Role: "entry", EvidenceRefs: []string{evidenceRef},
			WhyItMatters: "starts the primary runtime",
		}},
		FirstFilesToOpen: []orientationProviderFileToOpen{{FileRef: fileRef, Reason: "process entry"}},
		CandidateFlows: []orientationProviderCandidateFlow{{
			Name: "Process startup", FlowType: "request", Trigger: "the process starts",
			LikelyEntrypointRef: fileRef, LikelyFileRefs: []string{fileRef},
			WhyInteresting: "shows runtime wiring", EvidenceRefs: []string{evidenceRef}, Confidence: 0.72,
		}},
		ImportantDomainWords: []orientationProviderDomainWord{{Word: "serve", Guess: "runtime command", EvidenceRefs: []string{evidenceRef}}},
		QuestionsForHuman:    []string{"Which runtime matters most?"},
		ResearchQuestions: []orientationProviderResearchQuestion{{
			ID: "startup", Purpose: "trace startup", Question: "How does startup reach serving?",
			CandidateFileRefs: []string{fileRef}, EvidenceCategories: []string{"declaration", "callsite"},
		}},
		Warnings: []string{},
	}
}

func TestOrientationReferenceCatalogAndWireAreDeterministicWithoutDuplicateInventory(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	reordered := referenceFixtureBundle(t)
	reordered.TopLevelDirectoryStats = map[string]int{}
	reordered.TopLevelDirectoryStats["cmd"] = 1
	reordered.TopLevelDirectoryStats["internal"] = 2

	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	reorderedCatalog, err := buildOrientationReferenceCatalog(reordered)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := buildOrientationWireBundle(bundle, catalog)
	if err != nil {
		t.Fatal(err)
	}
	reorderedWire, err := buildOrientationWireBundle(reordered, reorderedCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.digest != reorderedCatalog.digest ||
		!reflect.DeepEqual(catalog.canonicalJSON, reorderedCatalog.canonicalJSON) ||
		!reflect.DeepEqual(wire, reorderedWire) {
		t.Fatalf("catalog/wire changed under equivalent map insertion order: %q/%q", catalog.digest, reorderedCatalog.digest)
	}

	original, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	refBearingFacts := len(catalog.filesByRef) + len(bundle.CandidateFileIndex) + len(bundle.SourceSignals) +
		len(bundle.Go.Entrypoints[0].Anchors) + len(bundle.Go.CommandTraces[0].Steps)*2 +
		len(bundle.Go.CommandTraces[0].HandlerCalls) + len(bundle.Go.OrientationCandidates) + len(bundle.Go.ImportantEdges)
	if delta := len(wire) - len(original); delta > refBearingFacts*64 {
		t.Fatalf("wire projection delta is not bounded by inline refs: before=%d after=%d facts=%d", len(original), len(wire), refBearingFacts)
	}
	var projected orientationWireBundle
	if err := json.Unmarshal(wire, &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected.CandidateFileIndex) != len(bundle.CandidateFileIndex) ||
		len(projected.SourceSignals) != len(bundle.SourceSignals) ||
		len(projected.Go.Entrypoints) != len(bundle.Go.Entrypoints) ||
		len(projected.Go.CommandTraces) != len(bundle.Go.CommandTraces) ||
		len(projected.Go.OrientationCandidates) != len(bundle.Go.OrientationCandidates) ||
		len(projected.Go.ImportantEdges) != len(bundle.Go.ImportantEdges) {
		t.Fatalf("wire projection changed bounded fact counts: %#v", projected)
	}
	for _, path := range []string{"cmd/main.go", "cmd/root.go", "docs/guide.md", "internal/handler.go", "internal/worker.go"} {
		if count := strings.Count(string(wire), path); count != 1 {
			t.Fatalf("wire path %q count = %d, want exactly one file-index occurrence", path, count)
		}
	}
	if strings.Contains(string(wire), "file-main-long-canonical-id") || strings.Contains(string(wire), `"allowed_paths"`) {
		t.Fatalf("wire leaked long candidate id or duplicate allowed path inventory: %s", wire)
	}
}

func TestOrientationWireDoesNotInflateOrLoseLargeCandidateInventory(t *testing.T) {
	const candidateCount = 250
	candidates := make([]map[string]any, 0, candidateCount)
	allowed := make([]string, 0, candidateCount)
	for index := 0; index < candidateCount; index++ {
		path := fmt.Sprintf("internal/component%03d/long_descriptive_runtime_file_%03d.go", index, index)
		allowed = append(allowed, path)
		candidates = append(candidates, map[string]any{
			"id":   fmt.Sprintf("file-%016x-long-canonical-candidate-identity", index),
			"path": path, "kind": "source", "signals": []string{"source"},
			"score": candidateCount - index, "reasons": []string{"bounded source candidate"},
		})
	}
	raw, err := json.Marshal(map[string]any{
		"repo_name": "large-fixture", "readme_excerpt": "", "top_level_directory_stats": map[string]int{"internal": candidateCount},
		"language_hints": []any{}, "go": map[string]any{
			"modules_count": 0, "packages_count": 0, "module_summaries": []any{}, "entrypoints": []any{},
			"orientation_candidates": []any{}, "important_edges": []any{},
		},
		"known_docs": []any{}, "candidate_file_index": candidates, "allowed_paths": allowed,
	})
	if err != nil {
		t.Fatal(err)
	}
	var bundle llmbundle.Bundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	wireJSON, err := buildOrientationWireBundle(bundle, catalog)
	if err != nil {
		t.Fatal(err)
	}
	var wire orientationWireBundle
	if err := json.Unmarshal(wireJSON, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.CandidateFileIndex) != candidateCount || len(wire.FileIndex) != candidateCount {
		t.Fatalf("large wire lost candidates/files: candidates=%d files=%d", len(wire.CandidateFileIndex), len(wire.FileIndex))
	}
	if len(wireJSON) >= len(raw) {
		t.Fatalf("large wire would consume more context: before=%d after=%d", len(raw), len(wireJSON))
	}
	for _, path := range allowed {
		if count := strings.Count(string(wireJSON), path); count != 1 {
			t.Fatalf("large wire path %q count = %d", path, count)
		}
	}
}

func TestOrientationReferenceResolverRoundTripsCanonicalValues(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := validReferenceResponse(t, catalog)
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	report, err := parseAndResolveOrientationResponse(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	wantEvidence := orientationCandidateFileEvidence("cmd/main.go", "entrypoint", []string{"entrypoint"})
	if len(report.CandidateFlows) != 1 || report.CandidateFlows[0].LikelyEntrypoint != "cmd/main.go" ||
		!reflect.DeepEqual(report.CandidateFlows[0].LikelyFiles, []string{"cmd/main.go"}) ||
		!reflect.DeepEqual(report.CandidateFlows[0].Evidence, []string{wantEvidence}) ||
		len(report.ResearchQuestions) != 1 ||
		!reflect.DeepEqual(report.ResearchQuestions[0].CandidateIDs, []string{"file-main-long-canonical-id"}) {
		t.Fatalf("resolved report = %#v", report)
	}
}

func TestOrientationReferenceResolverKeepsProseNonAuthoritative(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := validReferenceResponse(t, catalog)
	response.ProjectGuess = "cmd/main.go may be the main runtime"
	response.Warnings = []string{"invented/path.go is not verified"}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	report, err := parseAndResolveOrientationResponse(raw, catalog)
	if err != nil {
		t.Fatalf("non-authoritative prose was lexically rejected: %v", err)
	}
	if len(report.CandidateFlows) != 1 || len(report.CandidateFlows[0].Evidence) != 1 ||
		strings.Contains(report.CandidateFlows[0].Evidence[0], "invented/path.go") {
		t.Fatalf("provider prose influenced canonical evidence: %#v", report)
	}
}

func TestOrientationReferenceResolverRejectsInvalidTypedRefs(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	valid := validReferenceResponse(t, catalog)
	fileRef := valid.FirstFilesToOpen[0].FileRef
	evidenceRef := valid.HighLevelMap[0].EvidenceRefs[0]

	tests := map[string]func(map[string]any){
		"unknown": func(value map[string]any) {
			value["first_files_to_open"].([]any)[0].(map[string]any)["file_ref"] = "f9999"
		},
		"wrong kind": func(value map[string]any) {
			value["first_files_to_open"].([]any)[0].(map[string]any)["file_ref"] = evidenceRef
		},
		"duplicate": func(value map[string]any) {
			value["candidate_flows"].([]any)[0].(map[string]any)["likely_file_refs"] = []any{fileRef, fileRef}
		},
		"prefix": func(value map[string]any) {
			value["first_files_to_open"].([]any)[0].(map[string]any)["file_ref"] = strings.TrimSuffix(fileRef, "1")
		},
		"substituted": func(value map[string]any) {
			value["first_files_to_open"].([]any)[0].(map[string]any)["file_ref"] = fileRef + "x"
		},
		"raw path": func(value map[string]any) {
			value["first_files_to_open"].([]any)[0].(map[string]any)["file_ref"] = "cmd/main.go"
		},
		"unknown field": func(value map[string]any) {
			value["candidate_flows"].([]any)[0].(map[string]any)["likely_entrypoint"] = "cmd/main.go"
		},
		"backend contract field": func(value map[string]any) {
			value["contract_version"] = orientationResponseContractVersion
		},
		"private catalog field": func(value map[string]any) {
			value["catalog_id"] = catalog.digest
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(valid)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			raw, err = json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseAndResolveOrientationResponse(raw, catalog); err == nil {
				t.Fatalf("invalid typed response was accepted: %s", raw)
			}
		})
	}
}

func TestOrientationReferenceCatalogCoversNonCandidateFactsButResearchRejectsThem(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	rootRef, err := catalog.fileRef("cmd/root.go")
	if err != nil {
		t.Fatalf("entrypoint outside candidate index has no ref: %v", err)
	}
	edgeEvidence, err := catalog.evidenceRef(
		"import_edge",
		orientationImportEdgeEvidence("example.com/fixture/cmd", "example.com/fixture/internal/config.go"),
	)
	if err != nil {
		t.Fatal(err)
	}
	response := validReferenceResponse(t, catalog)
	response.FirstFilesToOpen = []orientationProviderFileToOpen{{FileRef: rootRef, Reason: "exact entrypoint fact"}}
	response.HighLevelMap[0].EvidenceRefs = []string{edgeEvidence}
	response.CandidateFlows[0].LikelyEntrypointRef = rootRef
	response.CandidateFlows[0].LikelyFileRefs = []string{rootRef}
	response.CandidateFlows[0].EvidenceRefs = []string{edgeEvidence}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	report, err := parseAndResolveOrientationResponse(raw, catalog)
	if err != nil {
		t.Fatalf("outside-candidate navigation ref was rejected: %v", err)
	}
	if err := validateResolvedOrientation(report); err != nil {
		t.Fatalf("locally materialized import-edge evidence was reparsed: %v", err)
	}
	if report.FirstFilesToOpen[0].Path != "cmd/root.go" ||
		report.CandidateFlows[0].LikelyEntrypoint != "cmd/root.go" {
		t.Fatalf("outside-candidate navigation did not roundtrip: %#v", report)
	}

	response.ResearchQuestions[0].CandidateFileRefs = []string{rootRef}
	raw, err = json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseAndResolveOrientationResponse(raw, catalog); err == nil || !strings.Contains(err.Error(), "no candidate mapping") {
		t.Fatalf("outside-index research candidate error = %v", err)
	}
}

func TestOrientationReferenceRoundTripPreservesPlannerSelection(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := validReferenceResponse(t, catalog)
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := parseAndResolveOrientationResponse(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}

	legacyRaw := []byte(`{
  "project_guess":"bounded command service","confidence":0.72,
  "high_level_map":[],"first_files_to_open":[{"path":"cmd/main.go","reason":"process entry"}],
  "candidate_flows":[{"name":"Process startup","flow_type":"request","trigger":"the process starts","likely_entrypoint":"cmd/main.go","likely_files":["cmd/main.go"],"why_interesting":"shows runtime wiring","evidence":["cmd/main.go candidate_file entrypoint"],"confidence":0.72}],
  "important_domain_words":[],"questions_for_human":[],
  "research_questions":[{"id":"startup","purpose":"trace startup","question":"How does startup reach serving?","candidate_ids":["file-main-long-canonical-id"],"evidence_categories":["declaration","callsite"]}],
  "unverified_paths":[],"warnings":[]}`)
	legacy, err := parseOrientation(legacyRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.CandidateFlows) != len(resolved.CandidateFlows) ||
		legacy.CandidateFlows[0].Name != resolved.CandidateFlows[0].Name ||
		legacy.CandidateFlows[0].Confidence != resolved.CandidateFlows[0].Confidence ||
		!reflect.DeepEqual(legacy.CandidateFlows[0].LikelyFiles, resolved.CandidateFlows[0].LikelyFiles) ||
		!reflect.DeepEqual(legacy.ResearchQuestions, resolved.ResearchQuestions) {
		t.Fatalf("semantic roundtrip drifted: legacy=%#v resolved=%#v", legacy, resolved)
	}
	applyOrientationConfidenceGate(&legacy, bundle)
	applyOrientationConfidenceGate(&resolved, bundle)
	for index := range legacy.CandidateFlows {
		flowexplain.ClassifyCandidateFlow(&legacy.CandidateFlows[index])
	}
	for index := range resolved.CandidateFlows {
		flowexplain.ClassifyCandidateFlow(&resolved.CandidateFlows[index])
	}
	legacyAccepted, resolvedAccepted := acceptedCandidateFlows(legacy.CandidateFlows), acceptedCandidateFlows(resolved.CandidateFlows)
	if !reflect.DeepEqual(legacyAccepted, resolvedAccepted) || len(resolvedAccepted) != 1 {
		t.Fatalf("accepted candidate count/order/confidence drifted: legacy=%#v resolved=%#v", legacyAccepted, resolvedAccepted)
	}

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "cmd"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "cmd/main.go"), []byte("package main\n\nfunc main() { run() }\nfunc run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := func(questions []modelresearch.ProposedQuestion) modelresearch.PlanResult {
		result, err := modelresearch.PlanTargetedRounds(context.Background(), modelresearch.PlanningInput{
			RepoPath: repo, Questions: questions,
			Candidates:           []modelresearch.FileCandidate{{ID: "file-main-long-canonical-id", Path: "cmd/main.go", Kind: "entrypoint", Score: 100}},
			InitialProviderPaths: []string{"cmd/main.go"},
			Universe:             modelresearch.LocalRepositoryUniverse{AuthorizedPaths: []string{"cmd/main.go"}},
			Policy:               modelresearch.DefaultPolicy(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	legacyPlan, resolvedPlan := plan(legacy.ResearchQuestions), plan(resolved.ResearchQuestions)
	if !reflect.DeepEqual(legacyPlan, resolvedPlan) || len(resolvedPlan.Selected) == 0 {
		t.Fatalf("planner selection drifted or was empty: legacy=%#v resolved=%#v", legacyPlan, resolvedPlan)
	}
}

func TestOrientationValidReferenceCacheHitMakesNoProviderCall(t *testing.T) {
	bundle := referenceFixtureBundle(t)
	catalog, err := buildOrientationReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := buildOrientationWireBundle(bundle, catalog)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(validReferenceResponse(t, catalog))
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": string(response)}}},
		})
	}))
	defer server.Close()
	client := &deepseek.Client{HTTPClient: server.Client(), Model: "fixture", MaxTokens: 1000, Endpoint: server.URL, Auth: "none"}
	writer, err := debugdump.NewWriter(t.TempDir(), "valid-reference-cache", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	request, err := client.OrientPromptJSON(wire)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(raw []byte) (orientationPart, error) { return parseAndResolveOrientationResponse(raw, catalog) }
	repository := modelresearch.RepositoryContext{Identity: "fixture", Revision: "abc", Scenario: "go-default"}
	policy := modelresearch.DefaultPolicy()

	first, err := obtainOrientation(context.Background(), client, writer, policy, repository, "test", wire, catalog.digest, request, true, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolvePreparedOrientation(first, func(raw []byte) (orientationPart, string, error) {
		prepared, err := prepare(raw)
		return prepared, "", err
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveOrientationResponse(first); err != nil {
		t.Fatal(err)
	}
	exact, err := obtainOrientation(context.Background(), client, writer, policy, repository, "test", wire, catalog.digest, request, true, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if first.Metrics.CacheHit || !exact.Metrics.CacheHit || exact.Prepared == nil || providerCalls != 1 {
		t.Fatalf("valid ref cache = first %t exact %t prepared %t provider calls %d", first.Metrics.CacheHit, exact.Metrics.CacheHit, exact.Prepared != nil, providerCalls)
	}
}
