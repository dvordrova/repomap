package lines

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

func TestCallableEvidenceWithinLimitsCutsListsAndCountsTheRest(t *testing.T) {
	place := atlas.Place{ID: "sym:serve", Kind: atlas.PlaceSymbol, Path: "main.go", LineNo: 1, Symbol: &atlas.SymbolFacts{
		Decl:     atlas.Decl{ObjectID: "obj:serve", Name: "Serve"},
		Calls:    []atlas.SymbolCall{{Name: "A", Line: 2}, {Name: "B", Line: 3}, {Name: "C", Line: 4}},
		CalledBy: []atlas.SymbolCaller{{Path: "x.go", Line: 1}, {Path: "y.go", Line: 2}, {Path: "z.go", Line: 3}},
	}}
	graph := atlas.Graph{Places: []atlas.Place{place}}
	subjects := map[string]bool{"obj:serve": true}
	bounded := CallableEvidenceWithin(graph, subjects, EvidenceLimits{Calls: 2, Callers: 2})["obj:serve"]
	if calls, _ := bounded["calls"].([]questionCall); len(calls) != 2 || bounded["calls_omitted"] != 1 || calls[1].Name != "B" {
		t.Fatalf("calls were not cut in source order with an honest count: %+v", bounded)
	}
	if callers, _ := bounded["called_by"].([]questionCaller); len(callers) != 2 || bounded["called_by_omitted"] != 1 || callers[1].Path != "y.go" {
		t.Fatalf("callers were not cut in source order with an honest count: %+v", bounded)
	}
	complete := CallableEvidence(graph, subjects)["obj:serve"]
	if calls, _ := complete["calls"].([]questionCall); len(calls) != 3 {
		t.Fatalf("unbounded evidence lost calls: %+v", complete)
	}
	if _, cut := complete["calls_omitted"]; cut {
		t.Fatal("unbounded evidence claims a cut")
	}
	if _, cut := complete["called_by_omitted"]; cut {
		t.Fatal("unbounded evidence claims a caller cut")
	}
}
