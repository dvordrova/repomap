package componentteach

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentprobe"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

// Experiment contract: replace this test when the teacher cube graduates. It
// protects only the current split between provider input and local locators.
func TestBuildKeepsGroundedLocatorsOutOfModelBundle(t *testing.T) {
	t.Parallel()
	round := minimalProbeRound()

	bundle, index, trace, err := Build(round, nil, DefaultBudget())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := trace.Validate(); err != nil {
		t.Fatalf("SelectionTrace.Validate() error = %v", err)
	}
	if len(bundle.Evidence) < 2 {
		t.Fatalf("Build().Evidence = %d, want source and relation", len(bundle.Evidence))
	}
	if bundle.Component.SupportBasis != SupportOrientationHypothesis {
		t.Fatal("component purpose was promoted beyond hypothesis")
	}
	foundRelation := false
	for _, item := range bundle.Evidence {
		if item.Kind == EvidenceStaticRelation {
			foundRelation = item.Caller == "A" && item.Callee == "B" && item.ActiveBuildCaveat != ""
		}
	}
	if !foundRelation {
		t.Fatal("static relation lost caller/callee names or active-build caveat")
	}
	for _, item := range bundle.Evidence {
		if item.Kind == EvidenceOrientationNote {
			t.Fatal("component-purpose hypothesis was fabricated into citable repository evidence")
		}
	}
	modelJSON, err := MarshalModelBundle(bundle)
	if err != nil {
		t.Fatalf("MarshalModelBundle() error = %v", err)
	}
	if strings.Contains(string(modelJSON), `"path"`) || strings.Contains(string(modelJSON), `"start_line"`) {
		t.Fatalf("model bundle leaked locator metadata: %s", modelJSON)
	}
	if len(index.Entries) != len(bundle.Evidence) {
		t.Fatalf("Index entries = %d, evidence = %d", len(index.Entries), len(bundle.Evidence))
	}
	if len(bundle.UnresolvedFrontierIDs) != 0 || len(bundle.UnresolvedFrontiers) != 0 {
		t.Fatal("frontier pointing back to an already probed exact symbol survived")
	}
	lines := make([]numberedLine, 65)
	for index := range lines {
		lines[index] = numberedLine{number: index + 1, text: "ordinary source line"}
	}
	if chunks := chunkLines(lines); len(chunks) != 1 || len(chunks[0]) != 65 {
		t.Fatalf("65-line ordinary function was split into %d chunks", len(chunks))
	}

	sensitive := minimalProbeRound()
	sensitive.SymbolProbes[0].Source.Lines[0].Text = `func A() {} // api_key := "company-secret-value"`
	safeBundle, _, safeTrace, err := Build(sensitive, nil, DefaultBudget())
	if err != nil {
		t.Fatalf("Build(sensitive) error = %v", err)
	}
	safeJSON, err := MarshalModelBundle(safeBundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(safeJSON), "company-secret-value") || !hasSelectionReason(safeTrace, SelectionRemoteValidationFail) {
		t.Fatal("outbound source validation did not omit and trace the source card")
	}
}

// Experiment contract: intentionally disposable. It checks weak JSON repair
// without pinning prose, prompt wording, or section cardinality.
func TestParseReportRepairsEnvelopeAndDropsOnlyBadSiblings(t *testing.T) {
	t.Parallel()
	bundle, _, _, err := Build(minimalProbeRound(), nil, DefaultBudget())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	known := bundle.Evidence[0].ID
	raw, err := json.Marshal(map[string]any{
		"version":             99,
		"primary_question_id": "invented-question",
		"mental_model": map[string]any{
			"id": "model-owned", "text": "  A is the entry.  ",
			"evidence_ids": known,
		},
		"boundaries": []any{
			map[string]any{"text": "A is only used by B.", "evidence_ids": []string{known}},
		},
		"unknowns": []any{
			map[string]any{"text": "Still unknown", "evidence_ids": []string{"unknown-id"}},
			map[string]any{"text": "Grounded uncertainty", "evidence_ids": []string{known, "unknown-id"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseReport(bundle, []byte("preface\n```json\n"+string(raw)+"\n```"))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if len(result.Report.MentalModel) != 1 || result.Report.MentalModel[0].ID != "item-mental-model-001" {
		t.Fatal("singleton item was not retained with a local id")
	}
	if result.Report.PrimaryQuestionID != bundle.PrimaryQuestion.ID || len(result.Report.Unknowns) != 1 {
		t.Fatal("question repair or sibling-preserving id filtering failed")
	}
	if len(result.Report.Boundaries) != 0 || !hasDiagnostic(result.Diagnostics, "claim.closed_world_dropped") {
		t.Fatal("unqualified closed-world claim survived bounded static evidence")
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("repairs were not surfaced as diagnostics")
	}
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func minimalProbeRound() componentprobe.Bundle {
	location := evidence.Location{Path: "sample.go", Line: 1, Column: 1}
	calleeLocation := evidence.Location{Path: "sample.go", Line: 2, Column: 1}
	selected := componentstudy.SymbolCandidate{
		ID: "symbol-a", Rank: 1, Name: "A", Kind: string(evidence.EntityFunction),
		Path: "sample.go", Line: 1, Column: 1, Reason: "primary entry",
		Provenance: componentstudy.Provenance{Source: "test", Operation: "fixture"},
		Certainty:  componentstudy.CertaintyStatic,
	}
	probeID := testStableID("probe", selected.ID, selected.Path, fmt.Sprint(selected.Line), selected.Name)
	provenance := []evidence.Provenance{{Provider: "test", Version: "1", Operation: "exact", Location: &location}}
	target := evidence.Entity{ID: "target-a", Kind: evidence.EntityFunction, Name: "A", Language: "go", Location: &location}
	callee := evidence.Entity{ID: "target-b", Kind: evidence.EntityFunction, Name: "B", Language: "go", Location: &calleeLocation}
	structural := symbol.Bundle{
		Version: symbol.BundleVersion, RepoName: "sample", Query: "A",
		Target:     symbol.Fact{EvidenceID: "resolution-001", Entity: target, Certainty: evidence.CertaintyStatic, Provenance: provenance},
		Candidates: []symbol.Fact{},
		OutgoingCalls: []symbol.CallFact{{
			EvidenceID: "call-out-001", Caller: target, Callee: callee, Callsite: &location,
			Certainty: evidence.CertaintyStatic, Provenance: provenance,
		}},
		IncomingCalls: []symbol.CallFact{}, Scenarios: []symbol.Scenario{},
		AllowedPaths: []string{"sample.go"}, Warnings: []string{}, Truncated: map[string]int{},
	}
	card := sourcecard.Card{
		Version: sourcecard.Version, Language: "go", RepoName: "sample",
		Target:     sourcecard.Target{EvidenceID: "resolution-001", EntityID: target.ID, Name: "A", Kind: target.Kind, Path: "sample.go", Line: 1, Column: 1},
		FileSHA256: strings.Repeat("0", 64),
		Window:     sourcecard.Window{StartLine: 1, EndLine: 1, IncludedBytes: 11, StopReason: sourcecard.StopEndOfFile},
		Lines:      []sourcecard.Line{{EvidenceID: "source-1", Line: 1, Text: "func A() {}"}}, Warnings: []sourcecard.Warning{},
	}
	tests := testevidence.Bundle{
		Version: testevidence.BundleVersion, TargetName: "A",
		Searches:   []testevidence.Search{{AnchorEvidenceID: "resolution-001", SymbolName: "A", Location: location, SourceEvidenceIDs: []string{}}},
		References: []testevidence.Reference{}, Scenarios: []evidence.Scenario{}, Warnings: []testevidence.Warning{},
	}
	refs := []componentprobe.EvidenceRef{
		{
			ID:   testStableID("ev", probeID, string(componentprobe.ArtifactStructural), "resolution-001"),
			Kind: componentprobe.EvidenceResolution, LocalID: "resolution-001",
			Origin: componentprobe.EvidenceOrigin{ProbeID: probeID, Artifact: componentprobe.ArtifactStructural, LocalID: "resolution-001"},
			Basis:  componentprobe.SupportStaticNavigation,
		},
		{
			ID:   testStableID("ev", probeID, string(componentprobe.ArtifactStructural), "call-out-001"),
			Kind: componentprobe.EvidenceOutgoingCall, LocalID: "call-out-001",
			Origin: componentprobe.EvidenceOrigin{ProbeID: probeID, Artifact: componentprobe.ArtifactStructural, LocalID: "call-out-001"},
			Basis:  componentprobe.SupportStaticNavigation,
		},
		{
			ID:   testStableID("ev", probeID, string(componentprobe.ArtifactSource), "source-1"),
			Kind: componentprobe.EvidenceSourceLine, LocalID: "source-1",
			Origin: componentprobe.EvidenceOrigin{ProbeID: probeID, Artifact: componentprobe.ArtifactSource, LocalID: "source-1"},
			Basis:  componentprobe.SupportSource,
		},
	}
	question := componentstudy.Question{ID: "question-a", Question: "How does A work?", Why: "It is the first research step.", EvidenceIDs: []string{selected.ID}}
	selectedFrontier := componentprobe.Frontier{
		Kind: componentprobe.FrontierCallEndpoint, Direction: componentprobe.DirectionOutgoing,
		Name: "A", EntityKind: evidence.EntityFunction, Location: location,
		Certainty: evidence.CertaintyStatic, Provenance: provenance,
		Origins: []componentprobe.EvidenceOrigin{{ProbeID: probeID, Artifact: componentprobe.ArtifactStructural, LocalID: "call-out-001"}},
		Basis:   componentprobe.SupportStaticNavigation, NavigationOnly: false, RuntimeProof: false,
	}
	selectedFrontier.ID = testFrontierID(selectedFrontier)
	return componentprobe.Bundle{
		Version: componentprobe.BundleVersion, Round: componentprobe.RoundInitial, Status: componentprobe.StatusFrontier,
		Focus: componentprobe.Focus{
			Goal:            componentstudy.Goal{ID: "goal-a", Kind: componentstudy.GoalOnboarding, Objective: "Understand the sample lifecycle."},
			Component:       componentstudy.Component{ID: "component-a", Name: "Sample", Purpose: "Coordinates the sample lifecycle."},
			PrimaryQuestion: question, SelectedFiles: []componentstudy.FileCandidate{}, SupportBasis: componentprobe.SupportOrientationHypothesis,
		},
		SymbolProbes: []componentprobe.SymbolProbe{{
			ID: probeID, SelectedSymbol: selected, Structural: structural, Source: card, Tests: tests, EvidenceIndex: refs,
		}},
		CallsiteWindows: []componentprobe.CallsiteWindow{}, Frontier: []componentprobe.Frontier{selectedFrontier}, Warnings: []componentprobe.Warning{},
	}
}

func testStableID(prefix string, parts ...string) string {
	hash := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return fmt.Sprintf("%s-%x", prefix, hash.Sum(nil)[:10])
}

func testFrontierID(frontier componentprobe.Frontier) string {
	name := frontier.Name
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		name = name[index+1:]
	}
	key := fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s",
		frontier.Kind,
		frontier.Direction,
		frontier.EntityKind,
		frontier.Location.Path,
		frontier.Location.Line,
		frontier.Location.Column,
		strings.TrimSpace(name),
	)
	return testStableID("frontier", key)
}

func hasSelectionReason(trace SelectionTrace, reason SelectionReason) bool {
	for _, decision := range trace.Decisions {
		if decision.Reason == reason {
			return true
		}
	}
	return false
}
