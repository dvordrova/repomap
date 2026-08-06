package themestudy

import (
	"strings"
	"testing"
)

// Decision 233 (Archive 9): semantic-equivalent themes (same normalized
// question+title) CO-PROJECT into one card with alternates instead of being
// dropped — distinct readings are retained, nothing silently vanishes.
func TestReduceSemanticEquivalentThemesCoProject(t *testing.T) {
	t.Parallel()
	anchors := map[string]AnchorInfo{
		"a1": {Path: "pkg/alpha.go", Line: 1, Symbol: "Alpha"},
		"a2": {Path: "pkg/beta.go", Line: 2, Symbol: "Beta"},
		"a3": {Path: "pkg/gamma.go", Line: 3, Symbol: "Gamma"},
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", ThemeKind: KindUserJourney, AnchorRefs: []string{"a1", "a2"}},
		"t2": {Ref: "t2", ThemeKind: KindUserJourney, AnchorRefs: []string{"a3"}},
	}
	themes := []AdjudicatedTheme{
		{
			CandidateRef: "t1", FinalTitle: "Same title", FinalQuestion: "Same question?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "obs1"},
				{AnchorRef: "a2", Fit: FitDirect, SupportedObservation: "obs2"},
			},
			ReadingOrder: []string{"a1", "a2"},
		},
		{
			// Semantic equivalent (same normalized question+title) with a
			// DISTINCT reading — must co-project, never drop.
			CandidateRef: "t2", FinalTitle: "Same title", FinalQuestion: "Same question?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: "a3", Fit: FitDirect, SupportedObservation: "obs3"},
			},
			ReadingOrder: []string{"a3"},
		},
	}
	reduction, err := Reduce(ReducerInput{Themes: themes, Candidates: candidates, Anchors: anchors})
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("semantic equivalents must co-project into 1 card, got %d (omitted=%d)", len(reduction.Cards), reduction.Omitted)
	}
	card := reduction.Cards[0]
	if len(card.Readings) != 2 || len(card.AlternateReadings) != 1 {
		t.Fatalf("co-project lost readings: primary=%d alternates=%d", len(card.Readings), len(card.AlternateReadings))
	}
	if len(card.AlternateTitles) != 1 || len(card.AlternateQuestions) != 1 {
		t.Fatalf("alternate provenance missing: titles=%v questions=%v", card.AlternateTitles, card.AlternateQuestions)
	}
	if card.AlternateReadings[0].Symbol != "Gamma" {
		t.Fatalf("alternate reading wrong: %#v", card.AlternateReadings[0])
	}
	if reduction.Omitted != 0 {
		t.Fatalf("co-projected theme must not count as omitted: %d", reduction.Omitted)
	}
}

// Decision 233: the portfolio-concentration diagnostic is GENERIC — a
// synthetic logging-heavy shelf (no TLS string anywhere in the rule) must
// trigger the same control as any other overrepresented family.
func TestReducePortfolioConcentrationIsGenericNonTLS(t *testing.T) {
	t.Parallel()
	anchors := map[string]AnchorInfo{
		"a1": {Path: "logging/sink.go", Line: 1, Symbol: "Sink1"},
		"a2": {Path: "logging/sink2.go", Line: 2, Symbol: "Sink2"},
		"a3": {Path: "logging/sink3.go", Line: 3, Symbol: "Sink3"},
		"a4": {Path: "core/run.go", Line: 4, Symbol: "Run"},
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", ThemeKind: KindSiblingImplementationFamily, AnchorRefs: []string{"a1"}},
		"t2": {Ref: "t2", ThemeKind: KindSiblingImplementationFamily, AnchorRefs: []string{"a2"}},
		"t3": {Ref: "t3", ThemeKind: KindSiblingImplementationFamily, AnchorRefs: []string{"a3"}},
		"t4": {Ref: "t4", ThemeKind: KindUserJourney, AnchorRefs: []string{"a4"}},
	}
	themes := make([]AdjudicatedTheme, 0, 4)
	for _, candidate := range candidates {
		ref := candidate.AnchorRefs[0]
		themes = append(themes, AdjudicatedTheme{
			CandidateRef: candidate.Ref,
			FinalTitle:   "Title " + candidate.Ref, FinalQuestion: "Question " + candidate.Ref + "?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: ref, Fit: FitDirect, SupportedObservation: "obs"},
			},
			ReadingOrder: []string{ref},
		})
	}
	reduction, err := Reduce(ReducerInput{Themes: themes, Candidates: candidates, Anchors: anchors})
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Cards) != 4 {
		t.Fatalf("concentration must not drop cards: got %d", len(reduction.Cards))
	}
	if reduction.Diagnostics["concentrated_family"] != 3 ||
		reduction.Diagnostics["portfolio_total"] != 4 ||
		reduction.Diagnostics["concentration_other"] != 1 {
		t.Fatalf("concentration counts = %#v", reduction.Diagnostics)
	}
	marked := 0
	for _, card := range reduction.Cards {
		if card.ConcentrationDiagnostic == "" {
			continue
		}
		marked++
		if !strings.HasPrefix(card.ConcentrationDiagnostic, "logging:3/4") {
			t.Fatalf("marker = %q, want logging:3/4", card.ConcentrationDiagnostic)
		}
	}
	if marked != 3 {
		t.Fatalf("concentration marked %d cards, want 3", marked)
	}
	// All anchors remain counted and reachable (nothing deleted).
	if reduction.Omitted != 0 {
		t.Fatalf("concentration must not omit: %d", reduction.Omitted)
	}
}

// Decision 233: a balanced shelf (no single family > half) publishes NO
// concentration marker and NO diagnostic.
func TestReduceBalancedShelfHasNoConcentration(t *testing.T) {
	t.Parallel()
	anchors := map[string]AnchorInfo{
		"a1": {Path: "core/a.go", Line: 1, Symbol: "A"},
		"a2": {Path: "api/b.go", Line: 2, Symbol: "B"},
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", ThemeKind: KindUserJourney, AnchorRefs: []string{"a1"}},
		"t2": {Ref: "t2", ThemeKind: KindIntegrationFamily, AnchorRefs: []string{"a2"}},
	}
	themes := []AdjudicatedTheme{
		{CandidateRef: "t1", FinalTitle: "T1", FinalQuestion: "Q1?",
			AnchorAssessments: []AnchorAssessment{{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o1"}}, ReadingOrder: []string{"a1"}},
		{CandidateRef: "t2", FinalTitle: "T2", FinalQuestion: "Q2?",
			AnchorAssessments: []AnchorAssessment{{AnchorRef: "a2", Fit: FitDirect, SupportedObservation: "o2"}}, ReadingOrder: []string{"a2"}},
	}
	reduction, err := Reduce(ReducerInput{Themes: themes, Candidates: candidates, Anchors: anchors})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reduction.Diagnostics["concentrated_family"]; ok {
		t.Fatalf("balanced shelf must not report concentration: %#v", reduction.Diagnostics)
	}
	for _, card := range reduction.Cards {
		if card.ConcentrationDiagnostic != "" {
			t.Fatalf("balanced shelf card marked: %#v", card)
		}
	}
}
