package themestudy

import (
	"strings"
	"testing"
)

// Phase 3 prompt cleanup (owner canonical wording): the Theme Adjudication
// response uses one `readings` array whose position IS the reading order —
// no separate selection/order serialization and no weak/irrelevant rows.
// The model returns only anchors that materially help answer the candidate
// question; unassessed candidate anchors are locally accounted as
// unreviewed by the backend.
func TestValidateAdjudicationReadingsWire(t *testing.T) {
	t.Parallel()
	candidateByRef := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", Title: "t1", Question: "t1?", ThemeKind: KindUserJourney,
			AnchorRefs: []string{"a1", "a2", "a3"}, WhyItMatters: "w", ExpectedLearning: "l",
			RelationClaim: RelationClaimEditorialOnly},
	}
	raw := []byte(`{"themes":[{"candidate_ref":"t1","final_title":"Theme one","final_question":"Question one?",` +
		`"readings":[` +
		`{"anchor_ref":"a2","support":"direct","observation":"second anchor directly supports"},` +
		`{"anchor_ref":"a1","support":"supporting","observation":"first anchor supports context"}` +
		`],"unknowns":["runtime behavior not observed"]}]}`)
	accepted, status, err := ValidateAdjudication(raw, candidateByRef)
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted = %d, want 1", len(accepted))
	}
	theme := accepted[0]
	// readings position IS the reading order: a2 before a1.
	if len(theme.ReadingOrder) != 2 || theme.ReadingOrder[0] != "a2" || theme.ReadingOrder[1] != "a1" {
		t.Fatalf("reading order = %v, want [a2 a1]", theme.ReadingOrder)
	}
	if len(theme.AnchorAssessments) != 2 {
		t.Fatalf("assessments = %d, want 2", len(theme.AnchorAssessments))
	}
	if theme.AnchorAssessments[0].Fit != FitDirect || theme.AnchorAssessments[1].Fit != FitSupporting {
		t.Fatalf("fits = %v/%v, want direct/supporting",
			theme.AnchorAssessments[0].Fit, theme.AnchorAssessments[1].Fit)
	}
	// a3 was never assessed: counted as unreviewed, never fatal.
	if status.UnreviewedAnchors != 1 {
		t.Fatalf("unreviewed anchors = %d, want 1 (a3)", status.UnreviewedAnchors)
	}
	if status.Rejected != 0 || status.State != "accepted" {
		t.Fatalf("status = %#v", status)
	}
}

// Phase 3: a returned theme keeps only retained anchors; an anchor omitted
// from readings is locally unreviewed, and the readings array order survives
// into publication.
func TestValidateAdjudicationReadingsOrderSurvives(t *testing.T) {
	t.Parallel()
	candidateByRef := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", Title: "t1", Question: "t1?", ThemeKind: KindUserJourney,
			AnchorRefs: []string{"a1", "a2", "a3", "a4"}, WhyItMatters: "w", ExpectedLearning: "l",
			RelationClaim: RelationClaimEditorialOnly},
	}
	raw := []byte(`{"themes":[{"candidate_ref":"t1","final_title":"T","final_question":"Q?",` +
		`"readings":[` +
		`{"anchor_ref":"a4","support":"direct","observation":"o4"},` +
		`{"anchor_ref":"a2","support":"direct","observation":"o2"}` +
		`],"unknowns":[]}]}`)
	accepted, _, err := ValidateAdjudication(raw, candidateByRef)
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if len(accepted) != 1 || len(accepted[0].ReadingOrder) != 2 {
		t.Fatalf("accepted = %#v", accepted)
	}
	if accepted[0].ReadingOrder[0] != "a4" || accepted[0].ReadingOrder[1] != "a2" {
		t.Fatalf("readings order not preserved: %v", accepted[0].ReadingOrder)
	}
}

// Phase 3: the active adjudication prompt names the wire `sources` (never
// `expanded_sources`) and asks for readings ordered for a developer, with at
// least one direct reading and no placeholders.
func TestAdjudicationPromptReadingsGrammar(t *testing.T) {
	t.Parallel()
	request := buildTestAdjudicationRequest(t)
	prompt := BuildAdjudicationPrompt(request)
	user := prompt.User
	for _, required := range []string{
		"Review each candidate independently",
		"Return every candidate that remains a useful source-backed theme after review",
		"omit unsupported candidates",
		"order readings in the order you recommend a developer inspect them",
		"Include at least one direct reading",
		`"readings":[{"anchor_ref":"a1","support":"direct","observation":"..."}]`,
	} {
		if !strings.Contains(user, required) {
			t.Errorf("adjudication prompt misses readings instruction %q", required)
		}
	}
	for _, forbidden := range []string{
		"expanded_sources", "anchor_assessments", "reading_order", "supported_observation",
		"weak", "irrelevant", "fit is one of",
	} {
		if strings.Contains(user, forbidden) {
			t.Errorf("adjudication prompt still carries legacy wire language %q", forbidden)
		}
	}
	if !strings.Contains(prompt.System, "sources contains additional context requested during local expansion") {
		t.Errorf("adjudication system prompt must name the wire `sources`")
	}
	if strings.Contains(prompt.System, "expanded_sources") {
		t.Errorf("adjudication system prompt still calls the wire expanded_sources")
	}
}

// buildTestAdjudicationRequest is a minimal valid adjudication request
// derived from the shared Scout fixture.
func buildTestAdjudicationRequest(t *testing.T) AdjudicationRequest {
	t.Helper()
	candidates := []ScoutCandidate{{
		Ref: "t1", Title: "t1", Question: "t1?", ThemeKind: KindUserJourney,
		AnchorRefs: []string{"a1"}, WhyItMatters: "w", ExpectedLearning: "l",
		RelationClaim: RelationClaimEditorialOnly,
	}}
	anchors := map[string]AnchorInfo{"a1": {Path: "main.go", Symbol: "main", Line: 2}}
	expansion := SourceExpansion{Version: "v1", CandidateSHA256: "test-expansion-sha"}
	request, err := CompileAdjudication(LanguageEnglish, candidates, expansion, anchors, nil)
	if err != nil {
		t.Fatalf("CompileAdjudication: %v", err)
	}
	return request
}
