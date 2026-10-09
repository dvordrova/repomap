package orientation

import (
	"bytes"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestGraphPresentationKeepsWholeAndSplitWireAndLookupAuthority(t *testing.T) {
	f := newFixture(t)
	target := f.targetID("alpha")
	file := atlas.Place{ID: "file", Kind: atlas.PlaceFile, Path: "alpha/main.go", TargetIDs: []string{target}, File: &atlas.FileFacts{}}
	sym := func(id, path, name, object string, line int) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, Parent: "file", TargetIDs: []string{target}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, Kind: "function", ObjectID: object, LineNo: line}}}
	}
	z := sym("z", "z.go", "Z", f.objectID("alpha", "core"), 1)
	a := sym("a", "a.go", "A", "", 1)
	main := sym("main", "alpha/main.go", "Main", f.objectID("alpha", "inbound"), 2)
	main.Symbol.Calls = []atlas.SymbolCall{{Name: "dispatch", Kind: "calls", Line: 3, Column: 7, Resolution: "alternatives", CalleeIDs: []string{"z", "a"}, Values: []string{"complete original literal"}}}
	raw := atlas.Graph{Revision: "rev", Places: []atlas.Place{file, z, main, a}, Edges: []atlas.Edge{}, Seeds: []string{"file"}, SeedDecls: []string{"main"}}
	f.input.Graph = raw
	oldWire, _, err := buildOverview(f.input)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := atlas.SealGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	canonical, encoded, presentation, err := sealed.Take()
	if err != nil {
		t.Fatal(err)
	}
	f.input.Graph = canonical
	if bytes.Equal(encodeOverview(t, f.input), mustGraphWire(t, oldWire)) {
		t.Fatal("contrast failed to expose canonical alternative order change")
	}
	f.input.GraphPresentation = presentation
	got, _, err := buildOverview(f.input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGraphWire(t, got), mustGraphWire(t, oldWire)) {
		t.Fatal("whole overview wire/source order changed")
	}
	for _, target := range oldWire.Targets {
		before, after := orientationRecords(oldWire, target), orientationRecords(got, target)
		for _, cuts := range [][]int{{0, len(before)}, {0, len(before) / 2, len(before)}} {
			for i := 1; i < len(cuts); i++ {
				if cuts[i] == cuts[i-1] {
					continue
				}
				left, err := encodeContextReading(orientationReading{contextTask, target, before[cuts[i-1]:cuts[i]]})
				if err != nil {
					t.Fatal(err)
				}
				right, err := encodeContextReading(orientationReading{contextTask, target, after[cuts[i-1]:cuts[i]]})
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(left, right) {
					t.Fatal("whole/split context bytes changed")
				}
			}
		}
	}
	// Presentation is a row-owned view, never a mutation of canonical body or bytes.
	now, err := atlas.EncodeGraph(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(now, encoded) {
		t.Fatal("presentation mutated canonical artifact")
	}
	for _, place := range canonical.Places {
		if place.Symbol == nil || len(place.Symbol.Calls) == 0 {
			continue
		}
		calls := presentation.Calls(place.ID, place.Symbol.Calls)
		saved := calls[0].CalleeIDs[0]
		calls[0].CalleeIDs[0] = "mutated row order"
		again := presentation.Calls(place.ID, place.Symbol.Calls)
		if again[0].CalleeIDs[0] != saved {
			t.Fatal("row mutation changed the private producer order")
		}
	}
	// Original lookup chooses the LAST matching object and FIRST matching native key.
	for _, byObject := range []bool{true, false} {
		objects := []groupindex.Index{}
		for _, id := range []string{"t1", "t2", "t3"} {
			objects = append(objects, groupindex.Index{Target: programindex.Target{ID: id}, Subjects: []groupindex.Subject{{ID: "n1", Object: &groupindex.ObjectFacts{Name: "same", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "p.go", Line: 1, Column: 1}}}}})
		}
		first, last := sym("z-first", "p.go", "same", "t2.n1", 1), sym("a-last", "p.go", "same", "t3.n1", 1)
		first.TargetIDs, last.TargetIDs = []string{"t1"}, []string{"t1"}
		first.Symbol.Decl.Signature = "FIRST"
		last.Symbol.Decl.Signature = "LAST"
		want := "FIRST"
		if byObject {
			first.Symbol.Decl.ObjectID, last.Symbol.Decl.ObjectID = "t1.n1", "t1.n1"
			want = "LAST"
		}
		g := atlas.Graph{Revision: "rev", Places: []atlas.Place{file, first, last}, Edges: []atlas.Edge{}}
		seal, err := atlas.SealGraph(g)
		if err != nil {
			t.Fatal(err)
		}
		c, _, o, err := seal.Take()
		if err != nil {
			t.Fatal(err)
		}
		input := Input{Groups: objects, Graph: c, GraphPresentation: o}
		got := input.declarationPlaces("t1", []string{"n1"})["n1"]
		if got == nil || got.Symbol.Decl.Signature != want {
			t.Fatalf("lookup authority byObject=%v = %#v, want %s", byObject, got, want)
		}
		input.GraphPresentation = nil
		different := input.declarationPlaces("t1", []string{"n1"})["n1"]
		if different == nil || different.Symbol.Decl.Signature == want {
			t.Fatal("collision contrast did not change without original order")
		}
	}
}

func mustGraphWire(t *testing.T, value any) []byte {
	t.Helper()
	b, e := encodeWire(value)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
