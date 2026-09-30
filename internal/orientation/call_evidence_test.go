package orientation

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestOrientationUsesOriginalMemberCallsWithoutImportingNeighbourBehavior(t *testing.T) {
	fixture := newFixture(t)
	programTargetID := fixture.input.Groups[0].Target.ID
	main := atlas.Place{ID: "local-place-main", Kind: atlas.PlaceSymbol, Path: "alpha/main.go", LineNo: 1, TargetIDs: []string{programTargetID},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: fixture.objectID("alpha", "inbound"), Name: "Serve", Signature: "func Serve()"}, Calls: []atlas.SymbolCall{
			{Name: "Apply", Kind: "calls", Line: 5, Column: 9, Resolution: "alternatives", CalleeIDs: []string{"local-place-core"},
				SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "field", Text: "issueTrackerClient", Anchor: &sourcevalue.Anchor{Path: "alpha/main.go", Line: 5, Column: 15}}}}},
			{Name: "Apply", Kind: "calls", Line: 5, Column: 35, Invocation: "goroutine", Resolution: "exact", CalleeIDs: []string{"local-place-core"}},
		}}}
	core := atlas.Place{ID: "local-place-core", Kind: atlas.PlaceSymbol, Path: "alpha/core.go", LineNo: 8, TargetIDs: []string{programTargetID},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: fixture.objectID("alpha", "core"), Name: "Apply", Signature: "func Apply()"}}}
	unselected := atlas.Place{ID: "local-place-unselected", Kind: atlas.PlaceSymbol, Path: "alpha/other.go", LineNo: 2, TargetIDs: []string{programTargetID},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "local-object-unselected", Name: "Unrelated"}, Calls: []atlas.SymbolCall{{Name: "unselected-neighbour-exchange"}}}}
	fixture.input.Graph = atlas.Graph{Places: []atlas.Place{main, core, unselected}}
	overview, catalog, err := buildOverview(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range overview.Seeds {
		if _, ok := catalog.subjects[row.Ref]; !ok {
			t.Fatalf("uncitable seed row: %s", row.Ref)
		}
	}
	seeds, _ := encodeWire(overview.Seeds)
	// A seed's callee outside the request is named by its declaration.
	for raw, callee := range map[string]string{string(seeds): "Apply (alpha/core.go:8)"} {
		for _, want := range []string{"alternatives", "goroutine", "Apply@5 -> " + callee} {
			if !strings.Contains(raw, want) {
				t.Fatalf("original observation %q missing: %s", want, raw)
			}
		}
		// An argument's origin stays local (rows.go).
		for _, forbidden := range append(fixture.canonicalIDs(), "local-place-", "local-object-", "unselected-neighbour-exchange", "callee_ids", `"column"`, "issueTrackerClient") {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("unadvertised identity/neighbor leaked: %s", forbidden)
			}
		}
	}
}
