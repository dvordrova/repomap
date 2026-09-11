package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// A selection reason that names candidates by their request-local keys
// reaches the reader with the questions spelled out; keys outside the pool
// stay as written.
func TestSelectionReasonSpellsCandidateKeys(t *testing.T) {
	questions := []atlas.LearningQuestion{{Question: "How is the server started?"}, {Question: "Where is PG_URL read?"}, {Question: "What does the outbox relay do?"}}
	pool := []int{2, 0}
	got := spellCandidateRefs("q1 and q2 overlap; q7 is unknown, and eq1 is not a key", pool, questions)
	want := "«What does the outbox relay do?» and «How is the server started?» overlap; q7 is unknown, and eq1 is not a key"
	if got != want {
		t.Fatalf("spellCandidateRefs = %q, want %q", got, want)
	}
}

// Reviews reach the decoder under the intent's title, another field name,
// as an object keyed by intent, or without keys in the asked order; only
// the advertised id form was accepted before.
func TestLearningReviewsAreMatchedByTitleFieldNameOrOrder(t *testing.T) {
	intents := learningIntents()
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}}
	review := func(key, value string) string {
		return `{"` + key + `":"` + value + `","state":"unknown","reason":"nothing established yet","sources":[]}`
	}
	cases := map[string]string{
		"title":     `{"reviews":[` + review("intent", intents[0].Title) + `]}`,
		"intent_id": `{"reviews":[` + review("intent_id", intents[1].ID) + `]}`,
		"heading":   `{"reviews":[` + review("intent", "## "+intents[2].ID+" | "+intents[2].Title) + `]}`,
		"object":    `{"reviews":{"` + intents[3].ID + `":{"state":"unknown","reason":"nothing established yet","sources":[]}}}`,
	}
	for name, raw := range cases {
		result, err := decodeLearning([]byte(raw), pool)
		if err != nil || len(result.Reviews) != 1 {
			t.Fatalf("%s: %v / %d reviews (%v)", name, err, len(result.Reviews), result.Rejections)
		}
	}
	var keyless []string
	for range intents {
		keyless = append(keyless, `{"state":"unknown","reason":"nothing established yet","sources":[]}`)
	}
	result, err := decodeLearning([]byte(`{"reviews":[`+joinJSON(keyless)+`]}`), pool)
	if err != nil || len(result.Reviews) != len(intents) {
		t.Fatalf("keyless complete response in asked order: %v / %d of %d", err, len(result.Reviews), len(intents))
	}
	if _, err := decodeLearning([]byte(`{"reviews":[`+review("intent", "no such intent")+`]}`), pool); err == nil {
		t.Fatal("an unknown intent name was accepted")
	}
}

func joinJSON(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ","
		}
		out += item
	}
	return out
}

// Six windows after a split each skip some intents; the plan is complete
// when every intent has an accepted review somewhere, and shows one
// "unavailable" review only for an intent no window reviewed.
func TestLearningReviewsConsolidatePerIntentAcrossWindows(t *testing.T) {
	intents := learningIntents()
	var reviews []atlas.LearningReview
	for i, intent := range intents {
		other := "unavailable"
		if i == 0 {
			other = "unknown"
		}
		reviews = append(reviews,
			atlas.LearningReview{Intent: intent.ID, Window: 1, State: map[bool]string{true: "questions", false: "unavailable"}[i != 0]},
			atlas.LearningReview{Intent: intent.ID, Window: 2, State: other})
	}
	kept, state := consolidateLearningReviews(reviews, "partial")
	if state != "ready" || len(kept) != len(intents) {
		t.Fatalf("every intent was reviewed in some window: state %q, %d reviews kept (%+v)", state, len(kept), kept)
	}
	for _, review := range kept {
		if review.State == "unavailable" {
			t.Fatalf("a covered intent kept an unavailable review: %+v", review)
		}
	}
	reviews = reviews[:2*len(intents)-2]
	reviews = append(reviews, atlas.LearningReview{Intent: intents[len(intents)-1].ID, Window: 1, State: "unavailable"}, atlas.LearningReview{Intent: intents[len(intents)-1].ID, Window: 2, State: "unavailable"})
	kept, state = consolidateLearningReviews(reviews, "partial")
	unavailable := 0
	for _, review := range kept {
		if review.State == "unavailable" {
			unavailable++
		}
	}
	if state != "partial" || unavailable != 1 {
		t.Fatalf("an intent no window reviewed: state %q, %d unavailable reviews", state, unavailable)
	}
}
