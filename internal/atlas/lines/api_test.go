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
	for _, def := range []table.Definition{API(true), API(false), APIGiven()} {
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
	for _, def := range []table.Definition{API(true), API(false), APIGiven()} {
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
	for _, asked := range []table.Definition{API(true), API(false), APIGiven()} {
		def := table.ForClassifier(asked)
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
	}{{API(true), "binds"}, {APIGiven(), "enters"}}
}

// No outcome is decided in two columns of one row: a symbol whose calls
// give it words is asked what they do with other programs and what the
// words become, and the two share only none. Taking messages from a queue
// is talks's to say; enters never offers it, nor middleware.
func TestNoOutcomeIsOfferedInTwoColumns(t *testing.T) {
	def := APIGiven()
	if len(def.Columns) != 2 || def.Columns[0].Name != "talks" || def.Columns[1].Name != "enters" || !def.Columns[0].Alone || !def.Columns[1].Alone {
		t.Fatalf("the word-given question is %+v", def.Columns)
	}
	for _, option := range def.Columns[1].Options {
		if option != APINone && slices.Contains(def.Columns[0].Options, option) {
			t.Fatalf("%s is offered by talks and enters", option)
		}
	}
	if slices.Contains(def.Columns[1].Options, APIMiddleware) || !slices.Contains(def.Columns[1].Options, "command") {
		t.Fatalf("enters offers %v", def.Columns[1].Options)
	}
}
