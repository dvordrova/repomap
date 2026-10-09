package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCNativeViewsShareFilesWithoutBorrowingConditionalDeclarations(t *testing.T) {
	fixture := loadCFixture(t)
	inputs := []places.TargetInput{}
	for _, variant := range []bool{false, true} {
		if variant {
			t.Setenv("CC", "clang -DREPOMAP_NATIVE_VARIANT")
		}
		project, err := cproject.Discover(t.Context(), fixture.root, fixture.repository)
		if err != nil {
			t.Fatal(err)
		}
		selector := "c:kvd"
		if variant {
			selector = "c:kvcli"
		}
		program := cFixture{project: project}.program(t, selector)
		parsed, err := cproject.Parse(t.Context(), fixture.root, fixture.repository, program, cproject.NewStore())
		if err != nil {
			t.Fatal(err)
		}
		projected, err := cproject.Index(fixture.repository, parsed)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(projected.Input)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, places.TargetInput{Index: index, Root: "."})
	}
	indexes, err := programindex.RebindTargetSet([]programindex.Index{inputs[0].Index, inputs[1].Index})
	if err != nil {
		t.Fatal(err)
	}
	for i := range inputs {
		inputs[i].Index = indexes[i]
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: inputs})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = atlas.DecodeGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	observed := map[string][]string{}
	var conditional []atlas.SymbolCall
	for _, place := range graph.Places {
		if place.Path != "strbuf.c" {
			continue
		}
		if place.File != nil && !slices.Equal(place.TargetIDs, []string{"t1", "t2"}) {
			t.Fatalf("shared file lost owners: %v", place.TargetIDs)
		}
		if place.Symbol != nil {
			observed[place.Symbol.Decl.Name] = place.TargetIDs
			if place.Symbol.Decl.Name == "sbNativeCommon" {
				conditional = place.Symbol.Calls
			}
		}
	}
	for name, want := range map[string][]string{"sbNativeCommon": {"t1", "t2"}, "sbNativeDefault": {inputs[0].Index.Target.ID}, "sbNativeVariant": {inputs[1].Index.Target.ID}} {
		if !slices.Equal(observed[name], want) {
			t.Fatalf("%s borrowed another native view: %v want %v", name, observed[name], want)
		}
	}
	if len(conditional) != 1 || conditional[0].Resolution != "exact" || !slices.Equal(conditional[0].TargetIDs, []string{inputs[1].Index.Target.ID}) {
		t.Fatalf("shared native declarations borrowed the other build's call: %+v", conditional)
	}
}
