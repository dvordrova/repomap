package themestudy

import (
	"testing"
)

// Archive 12 P0 (Casdoor 2 cards / Telebot 3 pairs): two accepted themes
// whose published reading sets are EXACTLY identical — even when they name
// different anchor refs that resolve to the same exact source identity —
// must publish as ONE card with the second theme's title/question retained
// as alternate provenance. Before this fix the ref-based canonical identity
// let duplicate reading sets publish as separate cards.
func TestReduceIdenticalReadingSetThroughDifferentAnchorRefsCoProjects(t *testing.T) {
	t.Parallel()
	anchors := map[string]AnchorInfo{
		"a1": {Path: "pkg/tls.go", Line: 40, Symbol: "loadTLS"},
		"a2": {Path: "pkg/tls.go", Line: 40, Symbol: "loadTLS"}, // same exact identity, different ref
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", ThemeKind: KindLifecycleConcern, AnchorRefs: []string{"a1"}},
		"t2": {Ref: "t2", ThemeKind: KindLifecycleConcern, AnchorRefs: []string{"a2"}},
	}
	themes := []AdjudicatedTheme{
		{
			CandidateRef: "t1", FinalTitle: "Certificate loading", FinalQuestion: "How are certificates loaded?",
			WhyItMatters: "Certificate loading matters.", ExpectedLearning: "Learn how certificates are loaded.",
			AnchorAssessments: []AnchorAssessment{{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "obs1"}},
			ReadingOrder:      []string{"a1"},
		},
		{
			CandidateRef: "t2", FinalTitle: "TLS bootstrap", FinalQuestion: "When does TLS configuration start?",
			WhyItMatters: "TLS bootstrap matters.", ExpectedLearning: "Learn where TLS configuration starts.",
			AnchorAssessments: []AnchorAssessment{{AnchorRef: "a2", Fit: FitDirect, SupportedObservation: "obs1"}},
			ReadingOrder:      []string{"a2"},
		},
	}
	reduction, err := Reduce(ReducerInput{Themes: themes, Candidates: candidates, Anchors: anchors})
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("identical reading set via different refs must co-project into 1 card, got %d (omitted=%d)", len(reduction.Cards), reduction.Omitted)
	}
	card := reduction.Cards[0]
	if len(card.Readings) != 1 {
		t.Fatalf("primary readings = %d, want 1", len(card.Readings))
	}
	foundAlternate := false
	for _, alternate := range card.AlternateTitles {
		if alternate == "TLS bootstrap" {
			foundAlternate = true
		}
	}
	if !foundAlternate {
		t.Fatalf("second theme title missing from alternates: %#v", card.AlternateTitles)
	}
	if reduction.CoProjected != 1 {
		t.Fatalf("CoProjected = %d, want 1", reduction.CoProjected)
	}
}
