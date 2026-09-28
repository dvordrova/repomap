package atlas

import (
	"slices"
	"testing"
)

// A declaration's uses name what it reads or hands over by the same compact
// place IDs as every other graph reference once places.json is sealed, each
// once; a use of a place that is not a declaration is refused.
func TestSealedGraphKeepsEachUseOnce(t *testing.T) {
	symbol := func(name string, line int, uses ...SymbolUse) Place {
		return Place{ID: SymbolID("server.c", line, name), Kind: PlaceSymbol, Path: "server.c", LineNo: line, Parent: FileID("server.c"), TargetIDs: []string{"t1"},
			Symbol: &SymbolFacts{Decl: Decl{Name: name, Kind: "function", LineNo: line}, Uses: uses}}
	}
	table, get := SymbolID("server.c", 2, "cmdTable"), SymbolID("server.c", 30, "getCommand")
	graph := Graph{Version: GraphVersion, Edges: []Edge{}, Seeds: []string{}, Places: []Place{
		{ID: FileID("server.c"), Kind: PlaceFile, Path: "server.c", TargetIDs: []string{"t1"}, File: &FileFacts{Decls: []Decl{}, Callers: []string{}, Callees: []string{}}},
		symbol("cmdTable", 2, SymbolUse{PlaceID: get, Kind: "passes_callback", Resolution: "exact"}),
		symbol("lookupCommand", 8, SymbolUse{PlaceID: table, Kind: "reads", Resolution: "exact"}, SymbolUse{PlaceID: table, Kind: "reads", Resolution: "exact"}),
		symbol("getCommand", 30),
	}}
	encoded, err := EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, place := range sealed.Places {
		if place.Symbol != nil {
			names[place.ID] = place.Symbol.Decl.Name
		}
	}
	var got []string
	for _, place := range sealed.Places {
		if place.Symbol == nil {
			continue
		}
		for _, use := range place.Symbol.Uses {
			got = append(got, place.Symbol.Decl.Name+" "+use.Kind+" "+names[use.PlaceID])
		}
	}
	if want := []string{"cmdTable passes_callback getCommand", "lookupCommand reads cmdTable"}; !slices.Equal(got, want) {
		t.Fatalf("uses = %v, want %v", got, want)
	}
	graph.Places[2].Symbol.Uses = append(graph.Places[2].Symbol.Uses, SymbolUse{PlaceID: FileID("server.c"), Kind: "reads", Resolution: "exact"})
	if _, err := EncodeGraph(graph); err == nil {
		t.Fatal("a use of a file place was sealed")
	}
}
