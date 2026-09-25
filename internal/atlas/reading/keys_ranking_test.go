package reading

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// A refused candidate is no key, and the others are still ranked by their
// probability. An accepted answer without a probability, or a flat ranking,
// still keeps the selection's own order.
func TestKeyRankingSkipsARefusedCandidate(t *testing.T) {
	cell := table.ProbabilityCell("explains")
	var ids []string
	var answers []rowAnswer
	for i, p := range []string{"0.10", "0.95", "0.20", "0.90", "0.85", "0.80", "0.75"} {
		ids = append(ids, fmt.Sprintf("s%d", i+1))
		answers = append(answers, rowAnswer{answer: table.Answer{cell: p}})
	}
	answers[2] = rowAnswer{}
	ranked, ok := rankedByProbability(ids, answers)
	if !ok || !slices.Equal(ranked, []string{"s2", "s4", "s5", "s6", "s7"}) || len(ranked) != lines.MaxKeysPerPart {
		t.Fatalf("one refused candidate declined the ranking or became a key: %v / %t", ranked, ok)
	}
	withoutProbability := slices.Clone(answers)
	withoutProbability[0] = rowAnswer{answer: table.Answer{"explains": "yes"}}
	if _, ok := rankedByProbability(ids, withoutProbability); ok {
		t.Fatal("an answer without a probability was ranked")
	}
	flat := slices.Clone(answers)
	for i := range flat {
		if flat[i].answer != nil {
			flat[i] = rowAnswer{answer: table.Answer{cell: "0.5"}}
		}
	}
	if _, ok := rankedByProbability(ids, flat); ok {
		t.Fatal("a flat ranking decided the keys")
	}
}
