package themestudy

import (
	"testing"
)

// Long-horizon program Phase 3 (syn live run): four accepted themes over the
// SAME exact reading but different kind labels (model classification) must
// publish as ONE card — kind is never evidence identity. The extras
// co-project as alternate provenance (titles/questions/readings).
func TestReduceIdenticalReadingSetAcrossDifferentKindsCoProjects(t *testing.T) {
	t.Parallel()
	anchors := map[string]AnchorInfo{
		"a1": {Path: "cmd/web/main.go", Line: 10, Symbol: "main"},
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", ThemeKind: KindUserJourney, AnchorRefs: []string{"a1"}},
		"t2": {Ref: "t2", ThemeKind: KindLifecycleConcern, AnchorRefs: []string{"a1"}},
		"t3": {Ref: "t3", ThemeKind: KindCrossCuttingPolicy, AnchorRefs: []string{"a1"}},
		"t4": {Ref: "t4", ThemeKind: KindIntegrationFamily, AnchorRefs: []string{"a1"}},
	}
	themes := []AdjudicatedTheme{
		{CandidateRef: "t1", FinalTitle: "Entry point", FinalQuestion: "Where does the service start?", WhyItMatters: "Service startup matters.", ExpectedLearning: "Learn where the service starts.", AnchorAssessments: []AnchorAssessment{{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o1"}}, ReadingOrder: []string{"a1"}},
		{CandidateRef: "t2", FinalTitle: "Startup config", FinalQuestion: "When is configuration loaded?", WhyItMatters: "Startup configuration matters.", ExpectedLearning: "Learn when configuration is loaded.", AnchorAssessments: []AnchorAssessment{{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o2"}}, ReadingOrder: []string{"a1"}},
		{CandidateRef: "t3", FinalTitle: "Error handling", FinalQuestion: "How are startup errors surfaced?", WhyItMatters: "Startup errors matter.", ExpectedLearning: "Learn how startup errors are surfaced.", AnchorAssessments: []AnchorAssessment{{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o3"}}, ReadingOrder: []string{"a1"}},
		{CandidateRef: "t4", FinalTitle: "Server wiring", FinalQuestion: "How is the server wired?", WhyItMatters: "Server wiring matters.", ExpectedLearning: "Learn how the server is wired.", AnchorAssessments: []AnchorAssessment{{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o4"}}, ReadingOrder: []string{"a1"}},
	}
	reduction, err := Reduce(ReducerInput{Themes: themes, Candidates: candidates, Anchors: anchors})
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("identical reading set across kinds must co-project into 1 card, got %d (omitted=%d)", len(reduction.Cards), reduction.Omitted)
	}
	card := reduction.Cards[0]
	if len(card.Readings) != 1 || card.Readings[0].Path != "cmd/web/main.go" {
		t.Fatalf("primary readings = %#v, want the single exact reading", card.Readings)
	}
	if reduction.CoProjected != 3 {
		t.Fatalf("co_projected = %d, want 3 (three extras folded in)", reduction.CoProjected)
	}
	wantAlternates := map[string]bool{"Startup config": true, "Error handling": true, "Server wiring": true}
	for _, alternate := range card.AlternateTitles {
		delete(wantAlternates, alternate)
	}
	if len(wantAlternates) != 0 {
		t.Fatalf("alternate titles missing: %#v", wantAlternates)
	}
}
