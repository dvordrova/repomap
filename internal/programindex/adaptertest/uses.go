package adaptertest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// DeclarationUse is one expected use of the places graph: the declaration
// From of file FromPath reads, hands over or is decorated by (Kind) the
// declaration To of file ToPath.
type DeclarationUse struct {
	FromPath, From, Kind, ToPath, To string
}

// AssertDeclarationUses checks that the places graph carries each use as an
// exact `uses` entry of its declaration: the reading layer's own record of
// what a declaration reads, hands over or is decorated by, which it cannot
// see in the calls it lifts for context.
func AssertDeclarationUses(t testing.TB, graph atlas.Graph, uses ...DeclarationUse) {
	t.Helper()
	symbols := map[[2]string][]atlas.Place{}
	byID := map[string]atlas.Place{}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		key := [2]string{place.Path, place.Symbol.Decl.Name}
		symbols[key] = append(symbols[key], place)
		byID[place.ID] = place
	}
	for _, want := range uses {
		from := symbols[[2]string{want.FromPath, want.From}]
		if len(from) == 0 {
			t.Errorf("%s %s is no declaration of the graph", want.FromPath, want.From)
			continue
		}
		found := false
		var got []string
		for _, place := range from {
			for _, use := range place.Symbol.Uses {
				to := byID[use.PlaceID]
				got = append(got, use.Kind+" "+to.Path+":"+to.Symbol.Decl.Name)
				found = found || use.Kind == want.Kind && use.Resolution == "exact" && to.Path == want.ToPath && to.Symbol.Decl.Name == want.To
			}
		}
		if !found {
			t.Errorf("%s %s does not %s %s %s exactly; its uses: %v", want.FromPath, want.From, want.Kind, want.ToPath, want.To, got)
		}
	}
}
