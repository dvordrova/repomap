package adaptertest

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// CallSite is one registration fact a call writes at its own position.
// Symbol is the name of the handed callable, empty when none is handed.
type CallSite struct {
	Line, Column int
	Key, Text    string
	Path         string
	Symbol       string
}

// AssertCallSiteBoundaries reads one real native input as two targets that
// share its files, the way two selected targets see one shared source file.
// On the lines of want, each target must hold exactly the wanted facts:
// every call is its own fact at its own column, so no chained or nested call
// is lost to or merged with its neighbour. Each call must then be one
// boundary place observed once by each target with that target's own fact,
// and the sealed graph must encode, which refuses two facts of one target in
// one place.
func AssertCallSiteBoundaries(t *testing.T, repository *corpus.Corpus, input programindex.Input, source string, want []CallSite) {
	t.Helper()
	var indexes []programindex.Index
	for _, view := range []string{"first", "second"} {
		viewInput := input
		viewInput.Target.Selector += "/call-site-view-" + view
		index, err := programindex.New(viewInput)
		if err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, index)
	}
	indexes, err := programindex.RebindTargetSet(indexes)
	if err != nil {
		t.Fatal(err)
	}
	var factTargets []facts.TargetInput
	var placeTargets []places.TargetInput
	for _, index := range indexes {
		factTargets = append(factTargets, facts.TargetInput{Index: index, Root: "."})
		placeTargets = append(placeTargets, places.TargetInput{Index: index})
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: factTargets})
	if err != nil {
		t.Fatal(err)
	}
	lines := map[int]bool{}
	for _, site := range want {
		lines[site.Line] = true
	}
	want = slices.Clone(want)
	slices.SortFunc(want, compareCallSites)
	factTarget := map[string]string{}
	got := map[string][]CallSite{}
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		if fact.Anchor == nil || fact.Anchor.Path != source || !lines[fact.Anchor.Line] {
			continue
		}
		factTarget[fact.ID] = fact.TargetID
		got[fact.TargetID] = append(got[fact.TargetID], CallSite{Line: fact.Anchor.Line, Column: fact.Anchor.Column,
			Key: fact.Key, Text: fact.Text, Path: fact.Path, Symbol: fact.Symbol})
	}
	for _, index := range indexes {
		have := got[index.Target.ID]
		slices.SortFunc(have, compareCallSites)
		if !reflect.DeepEqual(have, want) {
			t.Fatalf("registration facts of %s on lines %v in %s:\n have %s\n want %s", index.Target.ID, slices.Sorted(maps.Keys(lines)), source, formatCallSites(have), formatCallSites(want))
		}
	}
	graph, err := places.Build(places.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: placeTargets, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := atlas.EncodeGraph(graph); err != nil {
		t.Fatalf("sealed graph with chained calls in %s: %v", source, err)
	}
	var boundaries []CallSite
	for _, place := range graph.Places {
		if place.Boundary == nil || place.Path != source || !lines[place.LineNo] {
			continue
		}
		boundaries = append(boundaries, CallSite{Line: place.LineNo, Column: place.Column})
		observed := map[string]bool{}
		for _, origin := range place.Boundary.Origins {
			if factTarget[origin.FactID] != origin.TargetID || observed[origin.TargetID] {
				t.Fatalf("boundary at %d:%d holds a foreign or second fact of %s: %+v", place.LineNo, place.Column, origin.TargetID, place.Boundary.Origins)
			}
			observed[origin.TargetID] = true
		}
		if len(observed) != len(indexes) {
			t.Fatalf("boundary at %d:%d lost a target's own fact: %+v", place.LineNo, place.Column, place.Boundary.Origins)
		}
	}
	if len(boundaries) != len(want) {
		t.Fatalf("calls in %s became %d boundary places, want one per call (%d): %v", source, len(boundaries), len(want), boundaries)
	}
}

func compareCallSites(a, b CallSite) int {
	return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column), cmp.Compare(a.Key, b.Key),
		cmp.Compare(a.Text, b.Text), cmp.Compare(a.Path, b.Path), cmp.Compare(a.Symbol, b.Symbol))
}

func formatCallSites(sites []CallSite) string {
	var text []string
	for _, site := range sites {
		text = append(text, fmt.Sprintf("%d:%d %s %s %q %s", site.Line, site.Column, site.Key, site.Text, site.Path, site.Symbol))
	}
	return "[" + strings.Join(text, "; ") + "]"
}
