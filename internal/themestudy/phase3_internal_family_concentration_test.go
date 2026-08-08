package themestudy

import (
	"testing"
)

// Long-horizon program Phase 3 (Miniflux live run): every Go reading sits
// under internal/, so the old top-level family rule bucketed the whole
// shelf as one trivial `internal` family and the concentration diagnostic
// never fired — even when one domain (config) held more than half the
// shelf. The structural prefix is skipped and the first domain-bearing
// segment is the family, so a config-heavy portfolio triggers the same
// generic control as a logging-heavy one.
func TestReducePortfolioConcentrationSkipsInternalStructuralPrefix(t *testing.T) {
	t.Parallel()
	anchors := map[string]AnchorInfo{
		"a1": {Path: "internal/config/parser.go", Line: 1, Symbol: "Parse1"},
		"a2": {Path: "internal/config/parser.go", Line: 2, Symbol: "Parse2"},
		"a3": {Path: "internal/config/loader.go", Line: 3, Symbol: "Load"},
		"a4": {Path: "internal/reader/fetcher.go", Line: 4, Symbol: "Fetch"},
		"a5": {Path: "internal/reader/parser.go", Line: 5, Symbol: "FeedParse"},
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", ThemeKind: KindSharedDomainResponsibility, AnchorRefs: []string{"a1"}},
		"t2": {Ref: "t2", ThemeKind: KindSharedDomainResponsibility, AnchorRefs: []string{"a2"}},
		"t3": {Ref: "t3", ThemeKind: KindSharedDomainResponsibility, AnchorRefs: []string{"a3"}},
		"t4": {Ref: "t4", ThemeKind: KindUserJourney, AnchorRefs: []string{"a4"}},
		"t5": {Ref: "t5", ThemeKind: KindUserJourney, AnchorRefs: []string{"a5"}},
	}
	themes := make([]AdjudicatedTheme, 0, 5)
	for _, candidate := range candidates {
		ref := candidate.AnchorRefs[0]
		themes = append(themes, AdjudicatedTheme{
			CandidateRef: candidate.Ref,
			FinalTitle:   "Title " + candidate.Ref, FinalQuestion: "Question " + candidate.Ref + "?",
			WhyItMatters: "The final question matters.", ExpectedLearning: "Learn from the retained reading.",
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
	// config family = 3 of 5 (> half) — the diagnostic must fire with the
	// domain family, not the structural `internal` bucket.
	if reduction.Diagnostics["concentrated_family"] != 3 ||
		reduction.Diagnostics["portfolio_total"] != 5 ||
		reduction.Diagnostics["concentration_other"] != 2 {
		t.Fatalf("concentration counts = %#v, want config:3/5 (other=2)", reduction.Diagnostics)
	}
	marked := 0
	for _, card := range reduction.Cards {
		if card.ConcentrationDiagnostic != "" {
			marked++
		}
	}
	if marked != 3 {
		t.Fatalf("concentration-marked cards = %d, want 3 (the config family)", marked)
	}
}
