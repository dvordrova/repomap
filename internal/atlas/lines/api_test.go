package lines

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// The talks question has no "other" to fall into.
func TestTalksOffersNoOther(t *testing.T) {
	for _, def := range []table.Definition{API(true, true), API(false, true), API(true, false), APICall()} {
		for _, column := range def.Columns {
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
	for _, def := range []table.Definition{API(true, true), API(false, true), API(true, false), APICall(), APIArgument()} {
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
	for _, asked := range []table.Definition{API(true, true), API(false, true), API(true, false), APICall()} {
		def := table.ForClassifier(asked)
		recorder := &criteriaRecorder{}
		if _, err := table.ClassifierCall(recorder, def, window); err != nil {
			t.Fatal(err)
		}
		task, item := apiPrompt, "outside_symbol"
		if def.Contract == APICall().Contract {
			task, item = apiCallPrompt, "outside_call"
		}
		if recorder.task != task || len(recorder.questions) != len(def.Columns) {
			t.Fatalf("%s: task or questions: %d", def.Contract, len(recorder.questions))
		}
		for key, question := range recorder.questions {
			if question.Name != item || question.Item["symbol"] != "sys/socket.h.accept" {
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

// A handed symbol's two decisions are independent: a near-tie on what its
// call does with other programs leaves what the callable becomes standing.
// It is asked the same talks question as every other symbol, so a dial
// handed a configuration with a callback in it is asked whom it talks to.
func TestHandedAPIDecisionsFailAlone(t *testing.T) {
	def := API(true, true)
	if talks := def.Columns[1]; talks.Name != "talks" || !slices.Equal(talks.Options, API(false, true).Columns[0].Options) {
		t.Fatalf("a handed symbol is not asked the talks question: %+v", def.Columns)
	}
	window := table.Window{Rows: []table.Row{{ID: "sym1"}}}
	result, err := table.DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"sym1|binds": {Choice: "extension", Probabilities: map[string]float64{"extension": 0.97, "none": 0.03}},
		"sym1|talks": {Choice: APINone, Probabilities: map[string]float64{APINone: 0.52, APIServes: 0.48}},
	})
	if err != nil || result.Answers[0]["binds"] != "extension" || len(result.Rejections) != 1 || result.Rejections[0].Cell != "talks" {
		t.Fatalf("a near-tie on talks cost the symbol its binds: %+v %v", result, err)
	}
}

// Every question that asks what something of the repository becomes on our
// map reads one criteria file: an option such as command or none means the
// same wherever it is offered, and no question keeps a copy of its own.
func TestEveryEntryQuestionReadsOneCriteria(t *testing.T) {
	asked := 0
	for _, question := range entryQuestions() {
		for _, column := range question.def.Columns {
			if column.Name != question.column {
				continue
			}
			for _, option := range column.Options {
				asked++
				want, ok := entryOptions[option]
				if got := column.Criteria[option]; !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("%s %s asks %s with its own criteria:\n got %+v\nwant %+v", question.def.Contract, column.Name, option, got, want)
				}
			}
		}
	}
	if asked == 0 {
		t.Fatal("no entry question was found")
	}
}

// The criteria describe every repository in generic words: an example
// taken from a repository the questions were tuned on would teach the model
// that repository's answers.
func TestEntryCriteriaNameNoRepositoryItem(t *testing.T) {
	text := strings.ToLower(entryOptionsText)
	for _, name := range []string{"accepthandler", "servercron", "readqueryfromclient", "beforesleep", "cmdtable", "getcommand", "kvd", "redis", "symstable", "litestream"} {
		if strings.Contains(text, name) {
			t.Fatalf("prompts/entry_options.md names %s", name)
		}
	}
	for _, name := range entryOptionNames() {
		if _, ok := entryOptions[name]; !ok {
			t.Fatalf("prompts/entry_options.md has no option %s", name)
		}
	}
}

// entryQuestions are the columns that ask what something of the
// repository becomes on our map.
func entryQuestions() []struct {
	def    table.Definition
	column string
} {
	return []struct {
		def    table.Definition
		column string
	}{{API(true, true), "binds"}, {API(true, false), "binds"}, {APICall(), "enters"}}
}

// No outcome is decided by two questions: a call's words are asked what
// they become beside its symbol's talks answer, and the two share only
// none. Taking messages from a queue is talks's to say; enters never
// offers it, nor middleware.
func TestNoOutcomeIsOfferedInTwoQuestions(t *testing.T) {
	talks, enters := API(false, true).Columns[0], APICall().Columns[0]
	if talks.Name != "talks" || enters.Name != "enters" {
		t.Fatalf("the questions are %+v and %+v", talks, enters)
	}
	for _, option := range enters.Options {
		if option != APINone && slices.Contains(talks.Options, option) {
			t.Fatalf("%s is offered by talks and enters", option)
		}
	}
	if slices.Contains(enters.Options, APIMiddleware) || !slices.Contains(enters.Options, "command") {
		t.Fatalf("enters offers %v", enters.Options)
	}
}
