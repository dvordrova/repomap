package reading

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// learningResponseWith replaces the reviews of one intent in the ordinary
// reply with raw review objects.
func learningResponseWith(t *testing.T, intent string, reviews ...string) []byte {
	t.Helper()
	var wire []json.RawMessage
	for _, review := range learningReply().Reviews {
		if review.Intent == intent {
			continue
		}
		raw, err := json.Marshal(review)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, raw)
	}
	for _, review := range reviews {
		wire = append(wire, json.RawMessage(review))
	}
	raw, err := json.Marshal(map[string]any{"reviews": wire})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Each review is read field by field: a harmless form difference is the same
// review, a mistyped or unlisted member costs only itself, and the closed
// state is kept as written. A review without a readable state, in a state
// outside the closed set, or answered twice differently is refused.
func TestLearningReviewFormsAreReadAndWrongOnesRefused(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}, {Ref: "e2"}, {Ref: "e3"}}, Intents: learningIntents()}
	lease := `{"question":"What does a lease control?","why":"It bounds stored data.","sources":["e1"]}`
	for name, test := range map[string]struct {
		reviews   []string
		state     string
		reason    string
		sources   []string
		questions []string
		dropped   int
	}{
		"string sources and one malformed question": {
			reviews: []string{`{"intent":"purpose","state":"questions","reason":"State matters.","sources":"e1, e3","questions":[` + lease + `,7,{"question":"What does a revision name?","why":"Versions.","sources":"e3"}]}`},
			state:   "questions", reason: "State matters.", sources: []string{"e1", "e3"}, questions: []string{"What does a lease control?", "What does a revision name?"}, dropped: 1},
		"identical repeat": {
			reviews: []string{`{"intent":"purpose","state":"questions","reason":"State matters.","questions":[` + lease + `]}`, `{"intent":"purpose","state":"questions","reason":"State matters.","questions":[` + lease + `]}`},
			state:   "questions", reason: "State matters.", questions: []string{"What does a lease control?"}},
		"spelled state and intent": {
			reviews: []string{`{"intent":" PURPOSE ","state":"Not applicable","reason":"A library has no purpose of its own.","sources":["e2"]}`},
			state:   "not_applicable", reason: "A library has no purpose of its own.", sources: []string{"e2"}},
		"unknown without a reason": {
			reviews: []string{`{"intent":"purpose","state":"unknown","reason":""}`},
			state:   "unknown"},
		"unknown carrying questions": {
			reviews: []string{`{"intent":"purpose","state":"unknown","reason":"Nothing here.","questions":[` + lease + `]}`},
			state:   "unknown", reason: "Nothing here.", dropped: 1},
		"question without why": {
			reviews: []string{`{"intent":"purpose","state":"questions","reason":"","questions":[{"question":"What does a lease control?","sources":["e1"]}]}`},
			state:   "questions", questions: []string{"What does a lease control?"}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeLearning(learningResponseWith(t, "purpose", test.reviews...), pool)
			if err != nil || len(got.Reviews) != len(learningIntents()) || len(got.Rejections) != 0 || len(got.QuestionRejections) != test.dropped {
				t.Fatalf("a readable review was refused: %+v / %v", got, err)
			}
			index := slices.IndexFunc(got.Reviews, func(review learningReview) bool { return review.Intent == "purpose" })
			review := got.Reviews[index]
			var questions []string
			for _, question := range review.Questions {
				questions = append(questions, question.Question)
			}
			if review.State != test.state || review.Reason != test.reason || !slices.Equal(review.Sources, test.sources) || !slices.Equal(questions, test.questions) {
				t.Fatalf("the review changed: %+v", review)
			}
		})
	}
	for name, reviews := range map[string][]string{
		"no state":                 {`{"intent":"purpose","questions":7}`},
		"state outside the set":    {`{"intent":"purpose","state":"maybe","reason":"Unsure."}`},
		"answered differently":     {`{"intent":"purpose","state":"unknown","reason":"One."}`, `{"intent":"purpose","state":"unknown","reason":"Two."}`},
		"inapplicable unsupported": {`{"intent":"purpose","state":"not_applicable","reason":"No."}`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeLearning(learningResponseWith(t, "purpose", reviews...), pool)
			if err != nil || len(got.Reviews) != len(learningIntents())-1 || len(got.Rejections) != 1 || got.Rejections[0].Intent != "purpose" {
				t.Fatalf("a wrong review was read or refused its neighbours: %+v / %v", got, err)
			}
		})
	}
	// A bare array of reviews is the reviews array; no reviews at all is not.
	bare, err := json.Marshal(learningReply().Reviews)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := decodeLearning(bare, pool); err != nil || len(got.Reviews) != len(learningIntents()) {
		t.Fatalf("a bare reviews array was refused: %+v / %v", got, err)
	}
	for _, raw := range []string{`{"reviews":null}`, `{"notes":[]}`, `null`} {
		if _, err := decodeLearning([]byte(raw), pool); err == nil || !strings.Contains(err.Error(), "reviews array") {
			t.Fatalf("a response without reviews was read: %s / %v", raw, err)
		}
	}
}

// A menu without its rationale keeps its choices, and a menu naming only
// questions it was not offered selects none of them.
func TestLearningMenuWithoutAReasonKeepsItsChoices(t *testing.T) {
	provider := &learningProvider{menuEdit: func(row map[string]string) {
		switch row["key"] {
		case "purpose":
			delete(row, "reason")
		case "data":
			row["questions"] = "q99 q100"
		}
	}}
	r := isolatedLearningReader(t, t.TempDir(), provider)
	purpose := atlas.LearningQuestion{Question: "How does it start?", Origins: []atlas.LearningOrigin{{Intent: "purpose", Title: "Purpose", Question: "How does it start?", Why: "Startup."}}}
	data := atlas.LearningQuestion{Question: "What does a lease control?", Origins: []atlas.LearningOrigin{{Intent: "data", Title: "Data", Question: "What does a lease control?", Why: "Stored data."}}}
	r.learning = &atlas.LearningPlan{State: "ready", Questions: []atlas.LearningQuestion{purpose, data}}
	if err := r.selectLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	audiences := map[string]string{}
	for _, selection := range r.learning.Selections {
		audiences[selection.Intent] = fmt.Sprintf("%s/%q", selection.Audience, selection.Reason)
	}
	if audiences["purpose"] != `first_day/""` || audiences["data"] != `not_selected/"These questions provide a complementary introduction to the topic."` {
		t.Fatalf("a menu without a reason lost its choices or unoffered refs selected a question: %v", audiences)
	}
	if r.learning.State != "ready" || len(r.learning.Questions) != 1 || r.learning.Questions[0].Question != purpose.Question || len(r.rejected) != 0 {
		t.Fatalf("plan changed: %+v / %+v", r.learning, r.rejected)
	}
}
