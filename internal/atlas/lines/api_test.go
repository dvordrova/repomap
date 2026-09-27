package lines

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// The talks question has no "other" to fall into.
func TestTalksOffersNoOther(t *testing.T) {
	for _, def := range []struct{ handed bool }{{true}, {false}} {
		for _, column := range API(def.handed).Columns {
			if column.Name == "talks" && slices.Contains(column.Options, "other") {
				t.Fatalf("talks offers other: %v", column.Options)
			}
		}
	}
}

// Every api question is a closed choice the categorizer answers, and every
// option, the explicit none among them, carries its criteria: an option
// without them is the question the model was left to guess, which gave
// inet_aton "talks sdk" and accept "talks client_request" in some draws and
// not in others.
func TestEveryAPIOptionHasCriteriaAndEveryQuestionANone(t *testing.T) {
	for _, handed := range []bool{true, false} {
		def := API(handed)
		if !def.Classifier || !table.Closed(def) {
			t.Fatalf("%s is not a closed question for the categorizer", def.Contract)
		}
		for _, column := range def.Columns {
			if column.Optional || !slices.Contains(column.Options, APINone) {
				t.Fatalf("%s %s has no explicit none: %v", def.Contract, column.Name, column.Options)
			}
			for _, option := range column.Options {
				criteria, ok := column.Criteria[option]
				if !ok || criteria.What == "" || criteria.Includes == "" || criteria.NotFor == "" || len(criteria.Examples) == 0 {
					t.Fatalf("%s %s option %s has no criteria: %+v", def.Contract, column.Name, option, criteria)
				}
			}
		}
	}
}

// The request Jev reads carries every option's criteria, never null, and
// the task that says what the map wants.
func TestAPIRequestSendsEveryCriterion(t *testing.T) {
	window := table.Window{Rows: []table.Row{{ID: "sym1", Fields: []table.Field{{Name: "symbol", Value: "sys/socket.h.accept"}, {Name: "usage", Value: "fd = accept(s, &sa, &len);"}}}}}
	for _, handed := range []bool{true, false} {
		def := table.ForClassifier(API(handed))
		recorder := &criteriaRecorder{}
		if _, err := table.ClassifierCall(recorder, def, window); err != nil {
			t.Fatal(err)
		}
		if recorder.task != apiPrompt || len(recorder.questions) != len(def.Columns) {
			t.Fatalf("%s: task or questions: %d", def.Contract, len(recorder.questions))
		}
		for key, question := range recorder.questions {
			if question.Name != "outside_symbol" || question.Item["symbol"] != "sys/socket.h.accept" {
				t.Fatalf("%s: the question does not hold the symbol: %+v", key, question)
			}
			for _, option := range question.Options {
				if option.Criteria == nil {
					t.Fatalf("%s: option %s is sent without criteria", key, option.Name)
				}
			}
		}
	}
}

type criteriaRecorder struct {
	llm.Categorizer
	task      string
	questions map[string]llm.Question
}

func (r *criteriaRecorder) Prompt(task string, _ map[string]any, questions map[string]llm.Question) (llm.Prompt, error) {
	r.task, r.questions = task, questions
	return llm.Prompt{User: "{}"}, nil
}

// A handed symbol's two decisions are independent: a near-tie on whether
// the call serves leaves what the callable becomes standing.
func TestHandedAPIDecisionsFailAlone(t *testing.T) {
	def := API(true)
	window := table.Window{Rows: []table.Row{{ID: "sym1"}}}
	result, err := table.DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"sym1|binds":     {Choice: "extension", Probabilities: map[string]float64{"extension": 0.97, "none": 0.03}},
		"sym1|publishes": {Choice: APINone, Probabilities: map[string]float64{APINone: 0.52, APIServes: 0.48}},
	})
	if err != nil || result.Answers[0]["binds"] != "extension" || len(result.Rejections) != 1 || result.Rejections[0].Cell != "publishes" {
		t.Fatalf("a near-tie on publishes cost the symbol its binds: %+v %v", result, err)
	}
}
