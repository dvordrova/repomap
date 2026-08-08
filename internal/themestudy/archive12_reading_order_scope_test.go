package themestudy

import (
	"testing"
)

// Archive 12 P0 (owner directive): the current readings array derives the
// internal reading order and may contain only direct/supporting anchors. A
// weak/irrelevant row would silently change meaning when the reducer publishes
// readings, so the theme must be rejected item-locally.
func TestAdjudicationReadingOrderRequiresDirectOrSupporting(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q", WhyItMatters: "W", ExpectedLearning: "E",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly,
		AnchorRefs: []string{"a1", "a2", "a3"},
	}

	// weak row in readings -> reject
	rawWeak := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"why_it_matters":"Q matters.","expected_learning":"Learn Q.",
		"readings":[
			{"anchor_ref":"a1","support":"direct","observation":"obs1"},
			{"anchor_ref":"a2","support":"weak","observation":""}
		],"unknowns":[]
	}]}`)
	_, status, err := ValidateAdjudication(rawWeak, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if status.Accepted != 0 || status.Rejected != 1 {
		t.Fatalf("weak-anchor reading order must be rejected, got %#v", status)
	}

	// irrelevant row in the current readings array -> reject
	rawUnassessed := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"why_it_matters":"Q matters.","expected_learning":"Learn Q.",
		"readings":[
			{"anchor_ref":"a1","support":"direct","observation":"obs1"},
			{"anchor_ref":"a3","support":"irrelevant","observation":""}
		],"unknowns":[]
	}]}`)
	_, status, err = ValidateAdjudication(rawUnassessed, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if status.Accepted != 0 || status.Rejected != 1 {
		t.Fatalf("unassessed-anchor reading order must be rejected, got %#v", status)
	}

	// direct+supporting only -> accepted
	rawGood := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"why_it_matters":"Q matters.","expected_learning":"Learn Q.",
		"readings":[
			{"anchor_ref":"a2","support":"supporting","observation":"obs2"},
			{"anchor_ref":"a1","support":"direct","observation":"obs1"}
		],"unknowns":[]
	}]}`)
	themes, status, err := ValidateAdjudication(rawGood, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if status.Accepted != 1 || len(themes) != 1 {
		t.Fatalf("direct/supporting reading order must be accepted, got %#v", status)
	}
}
