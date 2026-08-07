package themestudy

import (
	"testing"
)

// Archive 12 P0 (owner directive): reading_order may contain only anchors
// assessed direct or supporting in the returned theme. A reading order that
// lists a weak/irrelevant or unassessed anchor would silently change meaning
// when the reducer publishes readings — the theme must be rejected
// item-locally.
func TestAdjudicationReadingOrderRequiresDirectOrSupporting(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q", WhyItMatters: "W", ExpectedLearning: "E",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly,
		AnchorRefs: []string{"a1", "a2", "a3"},
	}

	// weak anchor in reading_order -> reject
	rawWeak := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs1"},
			{"anchor_ref":"a2","fit":"weak","supported_observation":""}
		],
		"reading_order":["a1","a2"],"unknowns":[]
	}]}`)
	_, status, err := ValidateAdjudication(rawWeak, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if status.Accepted != 0 || status.Rejected != 1 {
		t.Fatalf("weak-anchor reading order must be rejected, got %#v", status)
	}

	// unassessed anchor in reading_order -> reject
	rawUnassessed := []byte(`{"themes":[{
		"candidate_ref":"t1","final_title":"T","final_question":"Q",
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs1"}
		],
		"reading_order":["a1","a3"],"unknowns":[]
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
		"anchor_assessments":[
			{"anchor_ref":"a1","fit":"direct","supported_observation":"obs1"},
			{"anchor_ref":"a2","fit":"supporting","supported_observation":"obs2"}
		],
		"reading_order":["a2","a1"],"unknowns":[]
	}]}`)
	themes, status, err := ValidateAdjudication(rawGood, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatal(err)
	}
	if status.Accepted != 1 || len(themes) != 1 {
		t.Fatalf("direct/supporting reading order must be accepted, got %#v", status)
	}
}
