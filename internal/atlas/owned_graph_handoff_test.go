package atlas

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func originalEncodeGraphOracle(graph Graph) ([]byte, error) {
	// Seal a canonical, independent copy of local native bindings. Caller
	// iteration order and duplicate observations do not change provider input.
	graph.Places = append([]Place(nil), graph.Places...)
	for i := range graph.Places {
		if graph.Places[i].Boundary != nil {
			boundary := *graph.Places[i].Boundary
			boundary.Origins = CanonicalBoundaryOrigins(boundary.Origins)
			graph.Places[i].Boundary = &boundary
		}
	}
	graph.Version = GraphVersion
	graph.SHA256 = ""
	var err error
	graph, err = compactGraphPlaceIDs(graph)
	if err != nil {
		return nil, err
	}
	if err := validateGraph(graph); err != nil {
		return nil, err
	}
	digest, err := digestOf("repomap-atlas-graph-v3\x00", graph)
	if err != nil {
		return nil, err
	}
	graph.SHA256 = digest
	return json.Marshal(graph)
}

func TestOwnedSealedGraphCanonicalBytesAndSingleTransfer(t *testing.T) {
	graph := Graph{Version: GraphVersion, Revision: "rev", Places: []Place{
		{ID: FileID("a.go"), Kind: PlaceFile, Path: "a.go", TargetIDs: []string{"t1"}, File: &FileFacts{Decls: []Decl{}, Callers: []string{}, Callees: []string{}}},
		{ID: SymbolID("a.go", 3, "Run"), Kind: PlaceSymbol, Path: "a.go", LineNo: 3, Parent: FileID("a.go"), TargetIDs: []string{"t1"}, Symbol: &SymbolFacts{Decl: Decl{Name: "Run", Kind: "function", LineNo: 3, Signature: "func()"}}},
	}, Edges: []Edge{}, Seeds: []string{FileID("a.go")}, SeedDecls: []string{SymbolID("a.go", 3, "Run")}}
	before, _ := json.Marshal(graph)
	for _, reverse := range []bool{false, true} {
		if reverse {
			graph.Places[0], graph.Places[1] = graph.Places[1], graph.Places[0]
		}
		want, err := originalEncodeGraphOracle(graph)
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := SealGraph(graph)
		if err != nil {
			t.Fatal(err)
		}
		alias := *sealed
		// The producer's nested pointers and slices do not alias the canonical owner.
		for i := range graph.Places {
			if graph.Places[i].Symbol != nil {
				graph.Places[i].Symbol.Decl.Signature = "changed caller"
				graph.Places[i].TargetIDs[0] = "changed"
			}
		}
		owned, got, _, err := sealed.Take()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatal("original complete bytes differ")
		}
		decoded, err := DecodeGraph(want)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(owned, decoded) {
			t.Fatal("canonical owned value differs from strict saved read")
		}
		if _, _, _, err := alias.Take(); err == nil {
			t.Fatal("copied handle transferred twice")
		}
		if err := sealed.WriteGraph(t.TempDir()); err == nil {
			t.Fatal("consumed handle kept encoded body")
		}
		for i := range graph.Places {
			if graph.Places[i].Symbol != nil {
				graph.Places[i].Symbol.Decl.Signature = "func()"
				graph.Places[i].TargetIDs[0] = "t1"
			}
		}
	}
	graph.Places[0], graph.Places[1] = graph.Places[1], graph.Places[0]
	after, _ := json.Marshal(graph)
	if !bytes.Equal(before, after) {
		t.Fatal("sealing changed original source graph")
	}
	invalid := graph
	invalid.Places = append([]Place(nil), graph.Places...)
	invalid.Places = append(invalid.Places, invalid.Places[0])
	if _, err := SealGraph(invalid); err == nil {
		t.Fatal("invalid duplicate input sealed")
	}
}
