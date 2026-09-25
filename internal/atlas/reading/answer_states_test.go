package reading

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// The answer state is the model's closed decision and is never promoted. An
// unanswered row keeps its state and gap, and the answer, basis and sources
// it carries anyway are discarded and recorded; its gap may be unnamed. A
// partial answer may leave its gap unnamed and a sourced answer its basis.
// A substantive answer without sources, or a settled one with a gap, is
// still refused.
func TestAnswerStatesKeepTheDecisionAndDiscardOnlyWhatItCannotCarry(t *testing.T) {
	cases := []struct {
		cells    map[string]any
		state    string
		text     string
		basis    string
		gap      string
		steps    int
		refused  bool
		accepted bool
	}{
		{cells: map[string]any{"state": "unanswered", "answer": "It probably retries.", "basis": "A guess.", "sources": "c1", "remaining": "The retry policy was not supplied."},
			state: "unanswered", gap: "The retry policy was not supplied."},
		{cells: map[string]any{"state": "unanswered", "answer": "None.", "basis": "none", "remaining": "NONE"},
			state: "unanswered", accepted: true},
		{cells: map[string]any{"state": "partial", "answer": "Run starts the worker.", "basis": "The signature.", "sources": "c1", "remaining": "none"},
			state: "partial", text: "Run starts the worker.", basis: "The signature.", steps: 1, accepted: true},
		{cells: map[string]any{"state": "answered", "answer": "Run starts the worker.", "basis": "none", "sources": []string{"c1"}, "remaining": "none"},
			state: "answered", text: "Run starts the worker.", steps: 1, accepted: true},
		{cells: map[string]any{"state": "answered", "answer": "Run starts the worker.", "basis": "The signature.", "sources": "", "remaining": "none"},
			refused: true},
		{cells: map[string]any{"state": "answered", "answer": "Run starts the worker.", "basis": "The signature.", "sources": "c1", "remaining": "Shutdown was not supplied."},
			refused: true},
	}
	provider := &answerRowsAdapter{answerTestProvider: &answerTestProvider{tableProvider: &tableProvider{}}}
	provider.completeAnswer = func(request answerTestRequest) ([]byte, error) {
		var rows []map[string]any
		for i, input := range request.Rows {
			row := map[string]any{"key": input["key"]}
			candidate := input["candidate_options"].([]any)[0]
			for name, value := range cases[i].cells {
				// The question's own candidate ref stands for c1.
				if value == "c1" {
					value = candidate
				}
				if list, ok := value.([]string); ok && len(list) == 1 && list[0] == "c1" {
					value = []any{candidate}
				}
				row[name] = value
			}
			rows = append(rows, row)
		}
		return json.Marshal(map[string]any{"rows": rows})
	}
	r := answerTestReader(t, answerTestRoutes(len(cases)), provider)
	if err := r.readAnswers(t.Context()); err != nil {
		t.Fatal(err)
	}
	var acceptedKeys []string
	for i, want := range cases {
		answer := r.questions[i].Answer
		part := answer.Parts[0]
		if want.refused {
			if answer.State != "unavailable" || part.Source != atlas.SourceGiven || part.Text != "" {
				t.Fatalf("case %d: a wrong answer was accepted: %+v", i, answer)
			}
			continue
		}
		if answer.State != want.state || part.Text != want.text || part.Basis != want.basis || part.Remaining != want.gap || len(part.Steps) != want.steps || part.Source != atlas.SourceModel {
			t.Fatalf("case %d: the answer changed: %+v", i, part)
		}
		if want.accepted {
			acceptedKeys = append(acceptedKeys, r.questions[i].ID)
		}
	}
	kinds := map[string]int{}
	for _, row := range r.rejected {
		kinds[row.Kind]++
		if row.Kind == "cell_rejected" && row.Samples[0] != r.questions[0].ID {
			t.Fatalf("a discarded cell was recorded for the wrong question: %+v", row)
		}
	}
	if kinds["cell_rejected"] != 3 || kinds["row_rejected"] != 2 {
		t.Fatalf("discarded cells and refused answers were not journaled apart: %+v", r.rejected)
	}
	// The discarded text of the unanswered question authorizes no glossary.
	if len(provider.accepted) != 1 || !reflect.DeepEqual(provider.accepted[0], acceptedKeys) {
		t.Fatalf("glossary rows = %v, want %v", provider.accepted, acceptedKeys)
	}
}
