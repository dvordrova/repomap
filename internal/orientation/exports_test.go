package orientation

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
)

// A library's exports are its API, never a way to run it: the request counts
// them and lists no row a run recipe could cite, so the model cannot offer
// liblua.a's lua_gettop as a command while the program's own entrypoint
// stays citable.
func TestRequestCountsLibraryExportsAndListsNone(t *testing.T) {
	fixture := newFixture(t)
	input := fixture.input
	export := facts.Fact{Kind: facts.KindEntrypoint, TargetID: fixture.targetID("alpha"), Anchor: &facts.Anchor{Path: "alpha/main.go", Line: 3}, Symbol: "Apply", Key: facts.EntrypointExport}
	export.ID = facts.NewFactID(export.TargetID, export.Kind, export.Anchor.Path, "export", facts.EntrypointExport, "Apply")
	sealed, err := facts.Seal(facts.Result{Revision: input.Facts.Revision, Targets: input.Facts.Targets, Facts: append(slices.Clone(input.Facts.Facts), export)})
	if err != nil {
		t.Fatal(err)
	}
	input.Facts = sealed
	wire, _, err := buildOverview(input)
	if err != nil {
		t.Fatal(err)
	}
	if wire.OmittedFactCounts["entrypoint_export"] != 1 {
		t.Fatalf("omitted counts %v", wire.OmittedFactCounts)
	}
	var entrypoints []string
	for _, row := range wire.Facts {
		if row.Kind == string(facts.KindEntrypoint) {
			entrypoints = append(entrypoints, row.Key+" "+row.Symbol)
		}
	}
	if !slices.Equal(entrypoints, []string{"callable Serve"}) {
		t.Fatalf("entrypoint rows offered to the recipe: %v", entrypoints)
	}
}
