package atlas

import "testing"

// A call's stores name the functions it reaches by the same place IDs as its
// callees after places.json gives them their compact IDs, and a store of a
// function the call does not reach is refused.
func TestSealedGraphKeepsEachStoreWithItsCallee(t *testing.T) {
	symbol := func(name string, line int, calls ...SymbolCall) Place {
		return Place{ID: SymbolID("server.c", line, name), Kind: PlaceSymbol, Path: "server.c", LineNo: line, Parent: FileID("server.c"), TargetIDs: []string{"t1"},
			Symbol: &SymbolFacts{Decl: Decl{Name: name, Kind: "function", LineNo: line}, Calls: calls}}
	}
	get, set := SymbolID("server.c", 30, "getCommand"), SymbolID("server.c", 20, "setCommand")
	call := SymbolCall{Kind: "calls", Name: "proc", Line: 9, Column: 5, Dispatch: "function_value", CalleeIDs: []string{get, set},
		Stores: []CallStore{{CalleeID: get, Path: "server.c", LineNo: 2, Column: 5}, {CalleeID: set, Path: "server.c", LineNo: 3, Column: 5}}}
	graph := Graph{Version: GraphVersion, Edges: []Edge{}, Seeds: []string{}, Places: []Place{
		{ID: FileID("server.c"), Kind: PlaceFile, Path: "server.c", TargetIDs: []string{"t1"}, File: &FileFacts{Decls: []Decl{}, Callers: []string{}, Callees: []string{}}},
		symbol("dispatch", 8, call), symbol("setCommand", 20), symbol("getCommand", 30),
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
	var stores []CallStore
	for _, place := range sealed.Places {
		if place.Symbol == nil {
			continue
		}
		names[place.ID] = place.Symbol.Decl.Name
		for _, call := range place.Symbol.Calls {
			stores = append(stores, call.Stores...)
		}
	}
	if len(stores) != 2 || names[stores[0].CalleeID] != "getCommand" || stores[0].LineNo != 2 || names[stores[1].CalleeID] != "setCommand" || stores[1].LineNo != 3 {
		t.Fatalf("stores = %+v, names %v", stores, names)
	}
	graph.Places[1].Symbol.Calls[0].Stores = append(graph.Places[1].Symbol.Calls[0].Stores, CallStore{CalleeID: SymbolID("server.c", 8, "dispatch"), Path: "server.c", LineNo: 4})
	if _, err := EncodeGraph(graph); err == nil {
		t.Fatal("a store of a function the call does not reach was sealed")
	}
}
