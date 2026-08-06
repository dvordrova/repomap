package themestudy

import (
	"strings"
	"testing"
)

// Decision 232 (Archive 9): duplicate anchor/file refs in a Scout response
// normalize deterministically (keep first) and count — they never reject a
// valid candidate.
func TestScoutDuplicateRefsNormalizeAndCount(t *testing.T) {
	t.Parallel()
	anchorRefs := map[string]struct{}{"a1": {}, "a2": {}, "a3": {}}
	fileRefs := map[string]struct{}{"f1": {}, "f2": {}}
	raw := []byte(`{"themes":[{
		"title":"Theme","question":"Q?","theme_kind":"user_journey",
		"anchor_refs":["a1","a2","a1","a3"],
		"expansion_file_refs":["f1","f2","f1"],
		"why_it_matters":"Why","expected_learning":"Learn",
		"relation_claim":"editorial_only","focused":false
	}]}`)
	candidates, status, err := ValidateScout(raw, anchorRefs, fileRefs, "catalog-digest")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("duplicate refs must not reject the candidate: %#v", status)
	}
	if len(candidates[0].AnchorRefs) != 3 {
		t.Fatalf("anchors not deduplicated: %#v", candidates[0].AnchorRefs)
	}
	if len(candidates[0].ExpansionFileRefs) != 2 {
		t.Fatalf("files not deduplicated: %#v", candidates[0].ExpansionFileRefs)
	}
	if status.Normalized["duplicate_anchor_refs"] != 1 || status.Normalized["duplicate_file_refs"] != 1 {
		t.Fatalf("duplicate counts = %#v", status.Normalized)
	}
	if status.Accepted != 1 || status.Rejected != 0 {
		t.Fatalf("status = %#v", status)
	}
}

// Decision 232: duplicate assessments in an Adjudication response normalize
// (keep first) and count — the theme survives.
func TestAdjudicationDuplicateAssessmentsNormalizeAndCount(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q", WhyItMatters: "W", ExpectedLearning: "E",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly,
		AnchorRefs: []string{"a1", "a2"},
	}
	raw := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs1"},
			{"anchor_ref":"a2","fit":"supporting","supported_observation":"obs2"},
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs1-dup"}
		],
		"reading_order":["a1","a2"],"unknowns":[]
	}]}`)
	themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 1 {
		t.Fatalf("duplicate assessment must not reject the theme: %#v", status)
	}
	if len(themes[0].AnchorAssessments) != 2 {
		t.Fatalf("assessments not deduplicated: %#v", themes[0].AnchorAssessments)
	}
	if status.Normalized["duplicate_assessment"] != 1 {
		t.Fatalf("duplicate assessment count = %#v", status.Normalized)
	}
}

// Decision 232: unassessed candidate anchors are counted as unreviewed and
// never published — the theme survives with reviewed + unreviewed totals.
func TestAdjudicationUnreviewedAnchorsAreCounted(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q", WhyItMatters: "W", ExpectedLearning: "E",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly,
		AnchorRefs: []string{"a1", "a2", "a3"},
	}
	raw := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs1"}
		],
		"reading_order":["a1"],"unknowns":[]
	}]}`)
	themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 1 {
		t.Fatalf("partial assessment must not reject the theme: %#v", status)
	}
	if status.ReviewedAnchors != 1 || status.UnreviewedAnchors != 2 {
		t.Fatalf("coverage = reviewed %d unreviewed %d", status.ReviewedAnchors, status.UnreviewedAnchors)
	}
	// The unreviewed anchor must never publish as a reading.
	if len(themes[0].AnchorAssessments) != 1 || themes[0].AnchorAssessments[0].AnchorRef != "a1" {
		t.Fatalf("unreviewed anchor leaked into assessments: %#v", themes[0].AnchorAssessments)
	}
}

// Decision 232: zero accepted themes is an honest semantic-empty result —
// the failed status returns (not an error), themes are empty, coverage is 0.
func TestAdjudicationZeroAcceptedIsSemanticEmpty(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q", WhyItMatters: "W", ExpectedLearning: "E",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly,
		AnchorRefs: []string{"a1"},
	}
	// The only theme has no direct anchor -> no direct -> rejected; zero
	// accepted themes must be a failed state with an empty theme list.
	raw := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"weak"}
		],
		"reading_order":[],"unknowns":[]
	}]}`)
	themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 0 {
		t.Fatalf("semantic-empty must carry zero themes: %#v", themes)
	}
	if status.State != "failed" {
		t.Fatalf("semantic-empty state = %q, want failed", status.State)
	}
	if status.Received != 1 || status.Rejected != 1 || status.Accepted != 0 {
		t.Fatalf("status counts = %#v", status)
	}
}

// Decision 232: a weak/irrelevant assessment may omit the observation (the
// empty-observation rejection applies only to direct/supporting).
func TestAdjudicationWeakMayOmitObservation(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q", WhyItMatters: "W", ExpectedLearning: "E",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly,
		AnchorRefs: []string{"a1", "a2"},
	}
	raw := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs"},
			{"anchor_ref":"a2","fit":"irrelevant"}
		],
		"reading_order":["a1"],"unknowns":[]
	}]}`)
	themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 1 {
		t.Fatalf("irrelevant without observation must not reject: %#v", status)
	}
	if status.Issues != nil && len(status.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", status.Issues)
	}
	for _, issue := range status.Issues {
		if strings.Contains(string(issue.Code), "empty_observation") {
			t.Fatalf("weak/irrelevant observation requirement leaked: %#v", issue)
		}
	}
}
