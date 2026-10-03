package table

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// A question without a verdict leaves only its row unanswered; every other
// row keeps its decision.
func TestAMissingVerdictRefusesOnlyItsRow(t *testing.T) {
	result, err := DecodeClassifierAnswers(closedDefinition(), closedWindow(), map[string]llm.Verdict{
		"s1|part": chose("Serving", map[string]float64{"Serving": 0.9}),
	})
	if err != nil || result.Answers[0]["part"] != "c1" || result.Answers[1] != nil || len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, "not answered") {
		t.Fatalf("one missing verdict refused its neighbours: %+v / %v", result, err)
	}
}

// A window whose every row the model answered, if uncertainly, is an
// explicit answer: it is accepted and cached with no row decided, so the
// same request is not bought again. A window with rows left unanswered, or
// answered outside their options, and nothing accepted is still refused.
func TestAnUncertainWindowIsAnExplicitAnswer(t *testing.T) {
	def, window := closedDefinition(), closedWindow()
	result, err := DecodeClassifierAnswers(def, window, map[string]llm.Verdict{
		"s1|part": chose("Serving", map[string]float64{"Serving": 0.4, "none": 0.35}),
		"s2|part": chose("none", map[string]float64{"Serving": 0.45, "none": 0.5}),
	})
	if err != nil || result.Answers[0] != nil || result.Answers[1] != nil || len(result.Rejections) != 2 || len(result.AcceptedRowKeys()) != 0 {
		t.Fatalf("an all-uncertain window was refused or decided: %+v / %v", result, err)
	}
	for name, verdicts := range map[string]map[string]llm.Verdict{
		"every row missing": {},
		"one row missing":   {"s1|part": chose("Serving", map[string]float64{"Serving": 0.4, "none": 0.35})},
		"one unlisted, one unsure": {
			"s1|part": chose("c9", map[string]float64{"c9": 1}),
			"s2|part": chose("none", map[string]float64{"none": 0.3, "Serving": 0.28}),
		},
	} {
		if _, err := DecodeClassifierAnswers(def, window, verdicts); err == nil || !strings.Contains(err.Error(), "no rows accepted") {
			t.Fatalf("%s: a window without an explicit answer was accepted: %v", name, err)
		}
	}
	provider := &fixedClassifierProvider{response: []byte(`{"answers":{"s1|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.4,"none":0.35}},"s2|part":{"type":"choice","choice":"none","probabilities":{"Serving":0.45,"none":0.5}}}}`)}
	call, err := ClassifierCall(&typesafe.Client{}, def, window)
	if err != nil {
		t.Fatal(err)
	}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	if _, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err != nil {
		t.Fatal(err)
	}
	recalled, err := llm.RecallJSON(t.Context(), executor, provider, call)
	if err != nil || !recalled.Cached || provider.calls != 1 {
		t.Fatalf("the uncertain window was not cached: %+v / %v", recalled, err)
	}
}

type fixedClassifierProvider struct {
	response []byte
	calls    int
}

func (*fixedClassifierProvider) State() []byte { return []byte(`{"model":"classifier-test"}`) }
func (*fixedClassifierProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (provider *fixedClassifierProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.calls++
	return llm.Completion{Response: provider.response, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, nil
}

// A question Jev answered twice differently refuses only its cell, or its
// row when the cell is not alone; an identical repeat is one answer and the
// neighbours keep their decisions (review B2).
func TestAConflictingVerdictRefusesOnlyItsQuestion(t *testing.T) {
	def, window := closedDefinition(), closedWindow()
	response := []byte(`{"answers":{
		"s1|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.9,"none":0.1}},
		"s1|part":{"type":"choice","choice":"none","probabilities":{"Serving":0.1,"none":0.9}},
		"s2|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.9,"none":0.1}},
		"s2|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.9,"none":0.1}}}}`)
	verdicts, err := (&typesafe.Client{}).Verdicts(response)
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeClassifierAnswers(def, window, verdicts)
	if err != nil || result.Answers[0] != nil || result.Answers[1]["part"] != "c1" || len(result.Rejections) != 1 ||
		!strings.Contains(result.Rejections[0].Reason, "answered twice differently") {
		t.Fatalf("a conflicting answer: %+v / %v", result, err)
	}
}

// Copies of an answer that differ only in letter case and whitespace (yes
// and " YES ", identical probabilities, in either order) are one answer:
// the row is decided, not refused as answered twice differently (review
// B2's residual). Both the YesAt cutoff and the margin rule read the form.
func TestCopiesDifferingInFormAreOneAnswer(t *testing.T) {
	yes := `{"type":"choice","choice":"yes","probabilities":{"yes":0.99,"no":0.01}}`
	spaced := `{"type":"choice","choice":" YES ","probabilities":{"yes":0.99,"no":0.01}}`
	neighbour := `{"type":"choice","choice":"no","probabilities":{"no":0.95,"yes":0.05}}`
	window := Window{Rows: []Row{{ID: "n1", Fields: []Field{{Name: "name", Value: "a"}}}, {ID: "n2", Fields: []Field{{Name: "name", Value: "b"}}}}}
	for _, yesAt := range []float64{0, 0.8} {
		def := Definition{Stage: "atlas_open", Contract: "b2-form", System: "open?", Classifier: true, YesAt: yesAt,
			Columns: []Column{{Name: "open", Kind: Choice, Options: []string{"yes", "no"}}}}
		for _, order := range [][2]string{{yes, spaced}, {spaced, yes}} {
			response := `{"answers":{"n1|open":` + order[0] + `,"n1|open":` + order[1] + `,"n2|open":` + neighbour + `}}`
			verdicts, err := (&typesafe.Client{}).Verdicts([]byte(response))
			if err != nil || verdicts["n1|open"].Conflict {
				t.Fatalf("yes at %v: copies differing in form were a conflict: %+v / %v", yesAt, verdicts, err)
			}
			result, err := DecodeClassifierAnswers(def, window, verdicts)
			if err != nil || result.Answers[0]["open"] != "yes" || len(result.Rejections) != 0 {
				t.Fatalf("yes at %v, order %q first: %+v / %v", yesAt, order[0], result, err)
			}
			if got := result.Answers[1]["open"]; got != "no" {
				t.Fatalf("yes at %v: the neighbour lost its answer: %+v", yesAt, result.Answers[1])
			}
		}
	}
	// A different decision written in another form is still a conflict.
	verdicts, _ := (&typesafe.Client{}).Verdicts([]byte(`{"answers":{"n1|open":` + yes + `,"n1|open":{"type":"choice","choice":" NO ","probabilities":{"yes":0.01,"no":0.99}}}}`))
	if !verdicts["n1|open"].Conflict {
		t.Fatalf("a different decision in another form was one answer: %+v", verdicts)
	}
}
