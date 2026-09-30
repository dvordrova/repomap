package reading

import (
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// Example code is no core: the core question offers `example`, with its
// criteria, beside the four roles, and a part answered example carries no
// core mark (freqtrade's sample strategies had carried it as domain).
func TestAPartAnsweredExampleIsNoCore(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	provider := &tableProvider{}
	opts := twoTargetOptions(t, graph, provider)
	var mu sync.Mutex
	offered := map[string]*llm.Criteria{}
	opts.Categorizer = &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		switch key[strings.LastIndex(key, "|")+1:] {
		case "key_symbol":
			return typesafetest.Choose("yes"), true
		case "explains":
			return typesafetest.Yes(0.5), true
		case "role":
			mu.Lock()
			for _, option := range question.Options {
				offered[option.Name] = option.Criteria
			}
			mu.Unlock()
			if question.Item["part"] == "svc/core" {
				return typesafetest.Choose(lines.PartExample), true
			}
			return typesafetest.Choose(lines.PartInterface), true
		}
		return llm.Verdict{}, false
	}}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if criteria, ok := offered[lines.PartExample]; !ok || criteria == nil || criteria.What == "" || len(offered) != 5 {
		t.Fatalf("the core question does not offer example with its criteria beside the four roles: %v", offered)
	}
	for _, target := range result.Atlas.Targets {
		for _, box := range target.Boxes {
			if box.Core {
				t.Fatalf("part %s answered %s or interface is core", box.Title, lines.PartExample)
			}
		}
	}
}
