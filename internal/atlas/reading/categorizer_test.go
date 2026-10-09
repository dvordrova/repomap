package reading

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// closedTableGuard is the text model, which must never be asked a closed
// table: the key declarations, the part roles or the keys.
type closedTableGuard struct {
	tableProvider
	t *testing.T
}

func (p *closedTableGuard) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Table string
		Fill  []struct{ Name string }
	}
	if json.Unmarshal(prepared.Bytes(), &request) == nil {
		for _, column := range request.Fill {
			if request.Table == lines.StageKeys || request.Table == lines.StageCore || column.Name == "key_symbol" {
				p.t.Errorf("the text model was asked the closed table %s", request.Table)
			}
		}
	}
	return p.tableProvider.Complete(ctx, prepared)
}

// The three closed tables go to the categorizer alone, and its verdicts
// decide the reading: the part it calls the domain is the core, and the
// keys are the five candidates it ranks highest, not the first five by
// name. A live reading without a categorizer is refused before it asks
// anything.
func TestTheCategorizerAloneDecidesTheClosedTables(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	provider := &closedTableGuard{t: t}
	opts := twoTargetOptions(t, graph, &provider.tableProvider)
	opts.Provider = provider
	var mu sync.Mutex
	asked := map[string]int{}
	opts.Categorizer = &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		column := key[strings.LastIndex(key, "|")+1:]
		mu.Lock()
		asked[column]++
		mu.Unlock()
		switch column {
		case "helper", "grouping", "box":
			return closedDecisions().Decide(key, question)
		case "key_symbol":
			return typesafetest.Choose("yes"), true
		case "role":
			if question.Item["part"] == "svc/core" {
				return typesafetest.Choose(lines.PartDomain), true
			}
			return typesafetest.Choose(lines.PartInterface), true
		case "explains":
			// Op01 at 0.1 up to Op08 at 0.8.
			name, _ := question.Item["declaration"].(string)
			return typesafetest.Yes(float64(name[len(name)-1]-'0') / 10), true
		}
		return llm.Verdict{}, false
	}}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if asked["key_symbol"] == 0 || asked["role"] == 0 || asked["explains"] != 8 {
		t.Fatalf("the categorizer was not asked every closed table: %v", asked)
	}
	var coreBoxes []string
	var keys []string
	for _, target := range result.Atlas.Targets {
		for _, box := range target.Boxes {
			if box.Core {
				coreBoxes = append(coreBoxes, box.Title)
			}
			for _, file := range box.Files {
				for _, symbol := range file.Symbols {
					if symbol.Key && file.Path == "svc/core/c.go" {
						keys = append(keys, symbol.Name)
					}
				}
			}
		}
	}
	slices.Sort(keys)
	if !slices.Equal(coreBoxes, []string{"svc/core"}) || !slices.Equal(keys, []string{"Op04", "Op05", "Op06", "Op07", "Op08"}) {
		t.Fatalf("the categorizer's verdicts did not decide the reading: core %v, keys %v", coreBoxes, keys)
	}
	calls := provider.calls
	opts.Categorizer, opts.OwnerRunDir = nil, t.TempDir()
	if _, err := Read(t.Context(), opts); err == nil || !strings.Contains(err.Error(), "needs a categorizer") || provider.calls != calls {
		t.Fatalf("a live reading without a categorizer: %v after %d calls", err, provider.calls-calls)
	}
	if _, err := Read(t.Context(), readOptions(t, graph, nil, "")); err != nil {
		t.Fatalf("a dry reading needs no categorizer: %v", err)
	}
}
