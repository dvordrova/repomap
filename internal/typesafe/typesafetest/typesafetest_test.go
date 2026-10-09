package typesafetest

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestChooseWritesOnlyTheExactPreparedQuestionInventory(t *testing.T) {
	certain := Choose("A")
	client := &Categorizer{Decide: ByColumn(map[string]llm.Verdict{"pick": certain})}
	for _, names := range [][]string{{"A", "B"}, {"A", "C", "D"}} {
		var options []llm.Option
		for _, name := range names {
			options = append(options, llm.Option{Name: name})
		}
		prompt, err := client.Prompt("choose", nil, map[string]llm.Question{"row|pick": {Options: options}})
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := client.Prepare(prompt, llm.Limits{MaxOutputTokens: 1})
		if err != nil {
			t.Fatal(err)
		}
		completion, err := client.Complete(t.Context(), prepared)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Answers map[string]struct{ Probabilities map[string]float64 }
		}
		if err := json.Unmarshal(completion.Response, &body); err != nil {
			t.Fatal(err)
		}
		probabilities := body.Answers["row|pick"].Probabilities
		if len(probabilities) != len(names) {
			t.Fatalf("invented or omitted an option: %v", probabilities)
		}
		for _, name := range names {
			value, present := probabilities[name]
			want := 0.0
			if name == "A" {
				want = 1
			}
			if !present || value != want {
				t.Fatalf("option %s: got %v/present=%v want %v", name, value, present, want)
			}
		}
	}
	if len(certain.Probabilities) != 1 {
		t.Fatal("request-specific scores mutated the reusable certainty preset")
	}
}
