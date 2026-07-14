package componentstudy

// These are replaceable experiment-contract tests, not a schema golden. Keep
// only the bounds, grounding, and weak-response tolerance invariants; remove or
// rewrite this fixture freely when the component-study cube is refactored.

import (
	"encoding/json"
	"testing"
)

func TestBuildBoundsAndExplainsSelection(t *testing.T) {
	t.Parallel()

	seed := testSeed()
	budget := Budget{
		Version:       BudgetVersion,
		MaxAnchors:    1,
		MaxFiles:      2,
		MaxSymbols:    2,
		MaxEvidence:   1,
		MaxModelBytes: maxModelBytes,
	}
	bundle, trace, err := Build(seed, budget)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(bundle.Anchors) != 1 || bundle.Anchors[0].ID != "anchor_first" {
		t.Fatalf("anchors = %#v, want explicit rank winner", bundle.Anchors)
	}
	if len(bundle.Files) != 2 || bundle.Files[0].ID != "file_first" || bundle.Files[1].ID != "file_second" {
		t.Fatalf("files = %#v, want rank order", bundle.Files)
	}
	if len(bundle.Symbols) != 2 || bundle.Symbols[0].ID != "symbol_static" ||
		bundle.Symbols[1].ID != "symbol_navigation" {
		t.Fatalf("symbols = %#v, want rank order", bundle.Symbols)
	}
	if len(bundle.Evidence) != 1 || bundle.Evidence[0].ID != "evidence_direction" {
		t.Fatalf("evidence = %#v, want explicit rank winner", bundle.Evidence)
	}
	if decisionReason(trace, "file_third") != SelectionFileLimit {
		t.Fatalf("file_third reason = %q, want %q", decisionReason(trace, "file_third"), SelectionFileLimit)
	}
	for _, decision := range trace.Decisions {
		if decision.Rank <= 0 || decision.EstimatedBytes <= 0 {
			t.Fatalf("decision lacks rank/byte accounting: %#v", decision)
		}
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("json.Marshal(bundle) error = %v", err)
	}
	if trace.EstimatedModelBytes != len(encoded) || len(encoded) > budget.MaxModelBytes {
		t.Fatalf(
			"model bytes = %d (trace %d), budget %d",
			len(encoded),
			trace.EstimatedModelBytes,
			budget.MaxModelBytes,
		)
	}

	roomyBudget := budget
	roomyBudget.MaxAnchors = len(seed.Anchors)
	roomyBudget.MaxFiles = len(seed.Files)
	roomyBudget.MaxSymbols = len(seed.Symbols)
	roomyBudget.MaxEvidence = len(seed.Evidence)
	_, fullTrace, err := Build(seed, roomyBudget)
	if err != nil {
		t.Fatalf("Build(roomy) error = %v", err)
	}
	roomyBudget.MaxModelBytes = fullTrace.EstimatedModelBytes - 1
	bounded, boundedTrace, err := Build(seed, roomyBudget)
	if err != nil {
		t.Fatalf("Build(byte bounded) error = %v", err)
	}
	if boundedTrace.EstimatedModelBytes > roomyBudget.MaxModelBytes {
		t.Fatalf("byte-bounded bundle uses %d > %d", boundedTrace.EstimatedModelBytes, roomyBudget.MaxModelBytes)
	}
	if !hasDecisionReason(boundedTrace, SelectionModelBytesLimit) {
		t.Fatalf("trace does not explain byte omission: %#v", boundedTrace.Decisions)
	}
	if err := bounded.Validate(); err != nil {
		t.Fatalf("byte-bounded bundle is invalid: %v", err)
	}
}

func TestParsePlanKeepsUsableGroundedParts(t *testing.T) {
	t.Parallel()

	seed := testSeed()
	bundle, _, err := Build(seed, Budget{
		Version:       BudgetVersion,
		MaxAnchors:    len(seed.Anchors),
		MaxFiles:      len(seed.Files),
		MaxSymbols:    len(seed.Symbols),
		MaxEvidence:   len(seed.Evidence),
		MaxModelBytes: maxModelBytes,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	raw := []byte("```json\n" + `{
  "version": 1,
  "framing": " Learn the write path\nfrom its boundary inward. ",
  "questions": [
    {
      "id": "q1",
      "question": "Where does the component accept work?",
      "why": "This establishes the boundary before internals.",
      "evidence_ids": ["evidence_direction", "file_first", "component_batch", "unknown_evidence"]
    },
    7,
    {
      "id": "q2",
      "question": "What is magic?",
      "why": "It might matter.",
      "evidence_ids": ["unknown_evidence"]
    }
  ],
  "selected_files": [{"id":"file_first"}, "unknown_file", "file_second", "file_third"],
  "selected_symbols": ["symbol_static", "symbol_navigation", "symbol_third", "symbol_fourth"],
  "unknowns": "Runtime ordering is not shown.",
  "warnings": ["Static calls are not runtime traces.", 9],
  "certainty": "verified"
}` + "\n```")
	result, err := ParsePlan(bundle, raw)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	if result.Plan.Framing != "Learn the write path from its boundary inward." {
		t.Fatalf("framing = %q", result.Plan.Framing)
	}
	if len(result.Plan.Questions) != 1 || result.Plan.Questions[0].ID != "q1" {
		t.Fatalf("questions = %#v, want only grounded q1", result.Plan.Questions)
	}
	if got := result.Plan.Questions[0].EvidenceIDs; len(got) != 2 || got[0] != "evidence_direction" || got[1] != "file_first" {
		t.Fatalf("q1 evidence = %#v", got)
	}
	if result.Plan.PrimaryQuestionID != "q1" {
		t.Fatalf("primary question = %q, want repaired q1", result.Plan.PrimaryQuestionID)
	}
	if len(result.Plan.SelectedFiles) != maxSelectedFileCount ||
		result.Plan.SelectedFiles[0].ID != "file_first" ||
		result.Plan.SelectedFiles[1].ID != "file_second" {
		t.Fatalf("selected files = %#v", result.Plan.SelectedFiles)
	}
	if len(result.Plan.SelectedSymbols) != maxSelectedSymbolCount {
		t.Fatalf("selected symbols = %#v", result.Plan.SelectedSymbols)
	}
	if result.Plan.SelectedSymbols[0].Certainty != CertaintyStatic ||
		result.Plan.SelectedSymbols[1].Certainty != CertaintyNavigation {
		t.Fatalf("model selection promoted local certainty: %#v", result.Plan.SelectedSymbols)
	}
	for _, code := range []string{
		"response.fenced_json_accepted",
		"field.unknown_ignored",
		"selection.unknown_id_dropped",
		"selection.object_id_accepted",
		"question.invalid_dropped",
		"question.ungrounded_dropped",
		"questions.below_schema_minimum",
		"primary_question.repaired",
		"selection.limit_dropped",
	} {
		if !hasDiagnostic(result.Diagnostics, code) {
			t.Errorf("missing diagnostic %q in %#v", code, result.Diagnostics)
		}
	}
}

func testSeed() Seed {
	return Seed{
		Version:  SeedVersion,
		RepoName: "pebble",
		Goal: Goal{
			ID:        "goal_onboarding",
			Kind:      GoalOnboarding,
			Objective: "Understand one component before following its implementation details.",
		},
		Component: Component{
			ID:      "component_batch",
			Name:    "Batch operations",
			Purpose: "Groups writes before committing them.",
		},
		Anchors: []AnchorCandidate{
			testAnchor("anchor_second", 2, "commit.go", 30, CertaintyNavigation),
			testAnchor("anchor_first", 1, "batch.go", 395, CertaintyStatic),
		},
		Files: []FileCandidate{
			testFile("file_third", 3, "wal/wal.go", CertaintyNavigation),
			testFile("file_first", 1, "batch.go", CertaintyStatic),
			testFile("file_second", 2, "commit.go", CertaintyNavigation),
		},
		Symbols: []SymbolCandidate{
			testSymbol("symbol_fourth", 4, "applyInternal", "commit.go", 140, CertaintyPossible),
			testSymbol("symbol_static", 1, "Batch.Commit", "batch.go", 395, CertaintyStatic),
			testSymbol("symbol_navigation", 2, "commitPipeline", "commit.go", 80, CertaintyNavigation),
			testSymbol("symbol_third", 3, "writeWAL", "commit.go", 120, CertaintyStatic),
		},
		Evidence: []EvidenceCandidate{
			testEvidence(
				"evidence_relation",
				2,
				EvidenceRelation,
				CertaintyStatic,
				"component_batch",
				"symbol_static",
			),
			testEvidence(
				"evidence_direction",
				1,
				EvidenceDirection,
				CertaintyHypothesis,
				"component_batch",
			),
		},
	}
}

func testAnchor(id string, rank int, path string, line int, certainty Certainty) AnchorCandidate {
	return AnchorCandidate{
		ID: id, Rank: rank, Path: path, Line: line,
		Reason: "fixture anchor", Provenance: testProvenance(), Certainty: certainty,
	}
}

func testFile(id string, rank int, path string, certainty Certainty) FileCandidate {
	return FileCandidate{
		ID: id, Rank: rank, Path: path,
		Reason: "fixture file", Provenance: testProvenance(), Certainty: certainty,
	}
}

func testSymbol(id string, rank int, name, path string, line int, certainty Certainty) SymbolCandidate {
	return SymbolCandidate{
		ID: id, Rank: rank, Name: name, Kind: "function", Path: path, Line: line, Column: 1,
		Reason: "fixture symbol", Provenance: testProvenance(), Certainty: certainty,
	}
}

func testEvidence(
	id string,
	rank int,
	kind EvidenceKind,
	certainty Certainty,
	relatedIDs ...string,
) EvidenceCandidate {
	return EvidenceCandidate{
		ID: id, Rank: rank, Kind: kind, Statement: "Bounded fixture evidence.",
		RelatedIDs: relatedIDs, Reason: "fixture evidence",
		Provenance: testProvenance(), Certainty: certainty,
	}
}

func testProvenance() Provenance {
	return Provenance{Source: "local-facts", Operation: "component-study-fixture"}
}

func decisionReason(trace SelectionTrace, id string) SelectionReason {
	for _, decision := range trace.Decisions {
		if decision.ID == id {
			return decision.Reason
		}
	}
	return ""
}

func hasDecisionReason(trace SelectionTrace, reason SelectionReason) bool {
	for _, decision := range trace.Decisions {
		if decision.Reason == reason {
			return true
		}
	}
	return false
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, item := range diagnostics {
		if item.Code == code {
			return true
		}
	}
	return false
}
