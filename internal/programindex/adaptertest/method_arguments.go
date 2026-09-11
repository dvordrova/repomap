package adaptertest

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// AssertMethodArgumentPositions checks the native counterparts of Go's
// synthetic bound-method case, without asserting identical binding semantics.
func AssertMethodArgumentPositions(t *testing.T, graph atlas.Graph, path, caller, receiver string) {
	t.Helper()
	for _, place := range graph.Places {
		if place.Path != path || place.Symbol == nil || place.Symbol.Decl.Name != caller {
			continue
		}
		// The graph keeps each expression's anchor for the local destination
		// pass; the consuming request carries the expression without it.
		anchored := 0
		for _, call := range place.Symbol.Calls {
			if call.Name != receiver || len(call.SourceArguments) != 4 {
				continue
			}
			for _, argument := range call.SourceArguments {
				v := argument.Origin
				if v == nil || v.Anchor == nil || v.Anchor.Path != path || v.Anchor.Line < 1 || v.Anchor.Column < 1 {
					t.Fatalf("method-value expression lost its source anchor in the graph: %+v", argument)
				}
			}
			anchored++
		}
		if anchored != 1 {
			t.Fatalf("graph calls of %s with four anchored arguments = %d, want 1", receiver, anchored)
		}
		evidence := lines.CallableEvidence(graph, map[string]bool{place.Symbol.Decl.ObjectID: true})[place.Symbol.Decl.ObjectID]
		raw, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Calls []atlas.SymbolCall `json:"calls"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		for _, call := range wire.Calls {
			if call.Name != receiver {
				continue
			}
			if len(call.SourceArguments) != 4 {
				t.Fatalf("method-value arguments lost original positions: %s", raw)
			}
			for i, argument := range call.SourceArguments {
				want := []string{"first", "second", "first", "second"}[i]
				v := argument.Origin
				if argument.Position != i+1 || v == nil || v.Kind != "field" || v.Text != want || v.Anchor != nil || len(v.Parts) != 1 || v.Parts[0].Text != "app" {
					t.Fatalf("method-value expression changed before consuming request: %+v; %s", argument, raw)
				}
			}
			return
		}
		t.Fatalf("method argument receiver absent from consuming request: %s", raw)
	}
	t.Fatalf("method argument fixture %s:%s absent from graph", path, caller)
}
