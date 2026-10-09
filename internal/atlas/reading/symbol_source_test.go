package reading

import (
	"encoding/json"
	"maps"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

func TestSymbolSourceIndexCompleteGraphAndForks(t *testing.T) {
	graph := testGraph(t)
	// A native file declaration need not have a symbol place. Keep that
	// declaration in the full source inventory while removing its place.
	missing := graphPlaceID(t, graph, atlas.PlaceSymbol, "pkg/b/gen.go", 9, "Gen")
	var places []atlas.Place
	for _, place := range graph.Places {
		if place.ID != missing {
			places = append(places, place)
		}
	}
	graph.Places = places
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	r, files := newReader(Options{Graph: graph})
	if files != 3 {
		t.Fatalf("authored files = %d, want 3", files)
	}
	before := maps.Clone(r.symbolsBySource)
	for _, current := range []*reader{r, r.view([]string{lines.StageZones}), r.view([]string{lines.StageBoundaries})} {
		legacy := &reader{places: current.places}
		for _, file := range graph.Places {
			if file.File == nil {
				continue
			}
			for _, decl := range file.File.Decls {
				got := current.symbolID(file.Path, decl.LineNo, decl.Name)
				want := legacy.symbolID(file.Path, decl.LineNo, decl.Name)
				if got != want {
					t.Fatalf("%s:%d %s = %q, original scan %q", file.Path, decl.LineNo, decl.Name, got, want)
				}
			}
		}
		if got := current.symbolID("pkg/b/gen.go", 9, "Gen"); got != "" {
			t.Fatalf("file-only declaration acquired symbol %q", got)
		}
		if got := current.symbolID("absent.go", 1, "Missing"); got != "" {
			t.Fatalf("unknown declaration acquired symbol %q", got)
		}
	}
	if !maps.Equal(before, r.symbolsBySource) {
		t.Fatal("lookups or fork construction changed the immutable source index")
	}
	current, err := atlas.EncodeGraph(r.opts.Graph)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(encoded) {
		t.Fatal("source inventory changed during lookup")
	}
}

func TestSymbolSourceIndexKeepsOriginalLastDuplicate(t *testing.T) {
	first := atlas.Place{ID: "s1", Kind: atlas.PlaceSymbol, Path: "app.go", LineNo: 8, Column: 4,
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "init"}}}
	second := first
	second.ID, second.Column = "s2", 24
	for _, ordered := range [][]atlas.Place{{first, second}, {second, first}} {
		r, _ := newReader(Options{Graph: atlas.Graph{Places: ordered}})
		want := ordered[len(ordered)-1].ID
		for _, current := range []*reader{r, r.view([]string{lines.StageZones}), r.view([]string{lines.StageBoundaries})} {
			if got := current.symbolID("app.go", 8, "init"); got != want {
				t.Fatalf("source duplicate = %q, want original last %q", got, want)
			}
		}
		if len(r.places) != 2 {
			t.Fatal("lookup erased a distinct original place")
		}
	}
}

func TestSymbolSourceIndexMissIsAuthoritative(t *testing.T) {
	place := atlas.Place{ID: "s1", Kind: atlas.PlaceSymbol, Path: "app.go", LineNo: 8,
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "run"}}}
	// A nonnil index, including the empty index, is the completed source
	// catalogue. The nil-only fallback supports hand-built internal readers.
	r, _ := newReader(Options{Graph: atlas.Graph{}})
	r.places[place.ID] = place
	if got := r.symbolID("app.go", 8, "run"); got != "" {
		t.Fatalf("complete index miss fell through to places: %q", got)
	}
	legacy := &reader{places: r.places}
	if got := legacy.symbolID("app.go", 8, "run"); got != place.ID {
		t.Fatalf("nil internal index fallback = %q, want %q", got, place.ID)
	}
	before, _ := json.Marshal(r.symbolsBySource)
	for i := 0; i < 100; i++ {
		r.symbolID("app.go", 8, "run")
	}
	after, _ := json.Marshal(r.symbolsBySource)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("misses mutated the completed source catalogue")
	}
}
