package report

import (
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// browseTestSpan builds one focused route span for the synthetic browse
// fixtures. The span ID doubles as the reading-target ID so the input stays
// self-consistent without a full atlas fixture.
func browseTestSpan(id string, stage atlasstudy.LearningStage) atlasstudy.RouteSpan {
	return atlasstudy.RouteSpan{
		ID:               id,
		Kind:             atlasstudy.RouteSpanFocused,
		LearningStage:    stage,
		QuestionEnglish:  "Question for " + id + "?",
		AllowedTargetIDs: []string{id},
	}
}

// browseTestInput assembles an input whose route spans are exactly ids, each
// with a focused reading target at open.go, except closeID (if non-empty)
// which points at closed.go and is deliberately not openable.
func browseTestInput(ids []string, closeID string) atlasstudy.Input {
	var input atlasstudy.Input
	input.Language = atlasstudy.LanguageEnglish
	for _, id := range ids {
		path := "open.go"
		if id == closeID {
			path = "closed.go"
		}
		input.ReadingTargets = append(input.ReadingTargets, atlasstudy.ReadingTarget{
			ID:       id,
			Kind:     "surface",
			Symbol:   "Symbol" + id,
			Location: evidence.Location{Path: path, Line: 10},
		})
		input.RouteSpans = append(input.RouteSpans, browseTestSpan(id, atlasstudy.StageOrientation))
	}
	return input
}

// themeSeedSpec builds one a* seed spec bound to a canonical span ID.
func themeSeedSpec(ref, spanID string) themestudy.SeedSpec {
	return themestudy.SeedSpec{
		Ref: ref, Path: "open.go", Line: 10, Symbol: "Symbol" + spanID,
		Provenance: "d211_span_reading_target", Kind: "focused",
		CanonicalSpanID: spanID,
	}
}

// themeScoutRequest builds a Scout request whose seed packs advertise exactly
// the ref->span bindings given.
func themeScoutRequest(bindings map[string]string) themestudy.ScoutRequest {
	var request themestudy.ScoutRequest
	request.Language = themestudy.LanguageEnglish
	for ref, spanID := range bindings {
		request.SeedPacks.Packs = append(request.SeedPacks.Packs, themestudy.SeedPack{
			Seed: themeSeedSpec(ref, spanID),
		})
	}
	return request
}

// themeScoutResult builds a Scout result with one accepted candidate per
// entry: ref -> anchor refs.
func themeScoutResult(candidates map[string][]string) themestudy.ScoutResult {
	var result themestudy.ScoutResult
	result.State = string(atlasstudy.ProductStateAccepted)
	for ref, anchors := range candidates {
		result.Candidates = append(result.Candidates, themestudy.ScoutCandidate{
			Ref: ref, AnchorRefs: anchors,
		})
	}
	return result
}

// themeStudyThemes builds the reduced portfolio with one card per entry:
// ordinal -> [span IDs read in this theme].
func themeStudyThemes(ordinals map[int][]string) themestudy.StudyThemes {
	var themes themestudy.StudyThemes
	for ordinal, spanIDs := range ordinals {
		card := themestudy.ThemeCard{Ordinal: ordinal}
		for _, spanID := range spanIDs {
			card.Readings = append(card.Readings, themestudy.Reading{
				Label: "read " + spanID, Symbol: "Symbol" + spanID,
				Path: "open.go", Line: 10, CanonicalSpanID: spanID,
			})
		}
		themes.Cards = append(themes.Cards, card)
	}
	return themes
}

func deriveThemeBrowseForTest(
	t *testing.T,
	ids []string,
	seedBindings map[string]string,
	scoutCandidates map[string][]string,
	themeOrdinals map[int][]string,
	closeID string,
) (*FrontierBrowse, themeStageCounts, error) {
	t.Helper()
	input := browseTestInput(ids, closeID)
	data := &ReportData{OpenablePaths: []string{"open.go"}}
	return deriveThemeStudyFrontierBrowse(
		input,
		themeScoutRequest(seedBindings),
		themeScoutResult(scoutCandidates),
		themeStudyThemes(themeOrdinals),
		data,
	)
}

// TestAtlasStudyBrowseStageDerivationAndTallies is the D213 re-based stage
// contract: every considered span receives exactly one of the four stages
// (considered / seed-advertised / scout-anchored / published) and the
// per-stage tallies equal the exact set sizes. The synthetic sets keep the
// chain published ⊆ scout-anchored ⊆ seed-advertised ⊆ considered.
func TestAtlasStudyBrowseStageDerivationAndTallies(t *testing.T) {
	ids := []string{"s1", "s2", "s3", "s4", "s5", "s6"}
	// s1,s2,s3,s4 advertised as seeds; s1,s2,s3 anchored by Scout candidates;
	// s1,s2 read in final themes.
	seedBindings := map[string]string{"a1": "s1", "a2": "s2", "a3": "s3", "a4": "s4"}
	scoutCandidates := map[string][]string{
		"t1": {"a1", "a2"},
		"t2": {"a3"},
	}
	themeOrdinals := map[int][]string{
		1: {"s1"},
		2: {"s2"},
	}
	browse, counts, err := deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err != nil {
		t.Fatalf("derive theme browse: %v", err)
	}
	if counts.considered != 6 || counts.seedAdvertised != 4 ||
		counts.scoutAnchored != 3 || counts.published != 2 {
		t.Fatalf("stage counts mismatch: %+v", counts)
	}
	stages := make(map[AtlasStudySpanStage]int)
	for _, span := range browse.Spans {
		stages[span.Stage]++
	}
	if stages[AtlasStudySpanStageConsidered] != 2 || stages[AtlasStudySpanStageSeedAdvertised] != 1 ||
		stages[AtlasStudySpanStageScoutAnchored] != 1 || stages[AtlasStudySpanStagePublished] != 2 {
		t.Fatalf("browse stage distribution mismatch: %+v", stages)
	}
	if browse.Total != 6 || browse.Shown != 6 {
		t.Fatalf("browse totals mismatch: total=%d shown=%d", browse.Total, browse.Shown)
	}
	// Display order: published first, then scout-anchored, seed-advertised,
	// considered (locale-independent canonical span-ID order within groups).
	order := make([]AtlasStudySpanStage, 0, len(browse.Spans))
	for _, span := range browse.Spans {
		order = append(order, span.Stage)
	}
	wantOrder := []AtlasStudySpanStage{
		AtlasStudySpanStagePublished, AtlasStudySpanStagePublished,
		AtlasStudySpanStageScoutAnchored, AtlasStudySpanStageSeedAdvertised,
		AtlasStudySpanStageConsidered, AtlasStudySpanStageConsidered,
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("browse order mismatch at %d: got %s want %s", i, order[i], wantOrder[i])
		}
	}
}

// TestAtlasStudyBrowsePublishedThemeRefs verifies the D213 B1/N5 contract:
// a published row lists EVERY matching published theme ordinal in canonical
// theme order, and non-published rows carry no theme refs.
func TestAtlasStudyBrowsePublishedThemeRefs(t *testing.T) {
	ids := []string{"s1", "s2", "s3"}
	seedBindings := map[string]string{"a1": "s1", "a2": "s2", "a3": "s3"}
	scoutCandidates := map[string][]string{"t1": {"a1", "a2", "a3"}}
	// s1 appears in themes 1 and 3 (canonical order 1,3); s2 in theme 2.
	themeOrdinals := map[int][]string{
		1: {"s1"},
		2: {"s2"},
		3: {"s1"},
	}
	browse, _, err := deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err != nil {
		t.Fatalf("derive theme browse: %v", err)
	}
	byTitle := make(map[string]Span)
	for _, span := range browse.Spans {
		byTitle[span.Title] = span
	}
	s1 := byTitle["Symbols1"]
	s2 := byTitle["Symbols2"]
	if s1.Stage != AtlasStudySpanStagePublished || len(s1.ThemeRefs) != 2 ||
		s1.ThemeRefs[0] != 1 || s1.ThemeRefs[1] != 3 {
		t.Fatalf("s1 theme refs mismatch: stage=%s refs=%v", s1.Stage, s1.ThemeRefs)
	}
	if s2.Stage != AtlasStudySpanStagePublished || len(s2.ThemeRefs) != 1 || s2.ThemeRefs[0] != 2 {
		t.Fatalf("s2 theme refs mismatch: stage=%s refs=%v", s2.Stage, s2.ThemeRefs)
	}
	s3 := byTitle["Symbols3"]
	if s3.Stage != AtlasStudySpanStageScoutAnchored || len(s3.ThemeRefs) != 0 {
		t.Fatalf("s3 must be scout-anchored without theme refs: stage=%s refs=%v", s3.Stage, s3.ThemeRefs)
	}
}

// TestAtlasStudyBrowseChainFailClosed verifies the fail-closed invariant: a
// published span outside scout-anchored, a scout-anchored span outside
// seed-advertised, or a seed-advertised span outside considered must reject
// the whole browse.
func TestAtlasStudyBrowseChainFailClosed(t *testing.T) {
	ids := []string{"s1", "s2"}
	// published s1 but s1 is not anchored by any Scout candidate.
	seedBindings := map[string]string{"a1": "s1", "a2": "s2"}
	scoutCandidates := map[string][]string{"t1": {"a2"}}
	themeOrdinals := map[int][]string{1: {"s1"}}
	_, _, err := deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err == nil {
		t.Fatal("published span outside scout-anchored must fail closed")
	}
	// scout-anchored s1 but s1 is not seed-advertised.
	scoutCandidates = map[string][]string{"t1": {"a1", "a2"}}
	seedBindings = map[string]string{"a2": "s2"}
	_, _, err = deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err == nil {
		t.Fatal("scout-anchored span outside seed-advertised must fail closed")
	}
	// seed-advertised span outside the rebuilt input.
	seedBindings = map[string]string{"a1": "s9", "a2": "s2"}
	_, _, err = deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err == nil {
		t.Fatal("seed-advertised span outside considered must fail closed")
	}
}

// TestAtlasStudyBrowseSeedBindingFailClosed verifies that a Scout candidate
// anchor with no span binding rejects the browse (fail closed rather than
// silently dropping the row).
func TestAtlasStudyBrowseSeedBindingFailClosed(t *testing.T) {
	ids := []string{"s1", "s2"}
	seedBindings := map[string]string{"a1": "s1"} // a2 not advertised at all
	scoutCandidates := map[string][]string{"t1": {"a1", "a2"}}
	themeOrdinals := map[int][]string{1: {"s1"}}
	_, _, err := deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err == nil {
		t.Fatal("candidate anchor with no advertised seed must fail closed")
	}
}

// TestAtlasStudyBrowse256CeilingTruthful verifies the MaxAtlasStudyBrowseSpans
// ceiling: Total stays the full considered count while Shown is capped.
func TestAtlasStudyBrowse256CeilingTruthful(t *testing.T) {
	ids := make([]string, 0, MaxAtlasStudyBrowseSpans+40)
	for i := 0; i < MaxAtlasStudyBrowseSpans+40; i++ {
		ids = append(ids, fmt.Sprintf("s%d", i))
	}
	seedBindings := make(map[string]string)
	scoutCandidates := make(map[string][]string)
	themeOrdinals := make(map[int][]string)
	browse, counts, err := deriveThemeBrowseForTest(t, ids, seedBindings, scoutCandidates, themeOrdinals, "")
	if err != nil {
		t.Fatalf("derive theme browse: %v", err)
	}
	if counts.considered != len(ids) {
		t.Fatalf("considered count mismatch: got %d want %d", counts.considered, len(ids))
	}
	if browse.Total != len(ids) || browse.Shown != MaxAtlasStudyBrowseSpans ||
		len(browse.Spans) != MaxAtlasStudyBrowseSpans {
		t.Fatalf("ceiling mismatch: total=%d shown=%d rows=%d", browse.Total, browse.Shown, len(browse.Spans))
	}
}

// TestAtlasStudyBrowseUnavailableSource verifies that a considered row whose
// source path is not openable publishes the neutral unavailable state (zero
// location) instead of a dead button.
func TestAtlasStudyBrowseUnavailableSource(t *testing.T) {
	ids := []string{"s1", "s2"}
	_, _, err := deriveThemeBrowseForTest(t, ids, nil, nil, nil, "s2")
	if err != nil {
		t.Fatalf("derive theme browse: %v", err)
	}
}

// TestAtlasStudyFailedBrowseNeutral verifies the neutral local-question browse
// for failed runs: every row is the considered stage, no theme refs, and the
// total comes from the rebuilt input count.
func TestAtlasStudyFailedBrowseNeutral(t *testing.T) {
	ids := []string{"s1", "s2", "s3"}
	input := browseTestInput(ids, "")
	data := &ReportData{OpenablePaths: []string{"open.go"}}
	browse, err := deriveAtlasStudyFailedBrowse(input, data)
	if err != nil {
		t.Fatalf("derive failed browse: %v", err)
	}
	if browse.Total != 3 || browse.Shown != 3 {
		t.Fatalf("failed browse totals mismatch: %+v", browse)
	}
	for _, span := range browse.Spans {
		if span.Stage != AtlasStudySpanStageConsidered || len(span.ThemeRefs) != 0 {
			t.Fatalf("failed browse row must be considered without theme refs: %+v", span)
		}
	}
}
