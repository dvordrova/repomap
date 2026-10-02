package orientation

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
)

// A library's exports are its API: listed as `export` rows, its role's own
// evidence (Lua's liblua.a had none once claims left the request), never an
// entrypoint and never a way to run it: a run step citing only an export is
// refused, so the model cannot offer liblua.a's lua_gettop as a command.
func TestRequestListsLibraryExportsAsItsOwnEvidenceNeverARunStep(t *testing.T) {
	fixture := newFixture(t)
	input := fixture.input
	export := facts.Fact{Kind: facts.KindEntrypoint, TargetID: fixture.targetID("alpha"), Anchor: &facts.Anchor{Path: "alpha/main.go", Line: 3}, Symbol: "Apply", Key: facts.EntrypointExport}
	export.ID = facts.NewFactID(export.TargetID, export.Kind, export.Anchor.Path, "export", facts.EntrypointExport, "Apply")
	sealed, err := facts.Seal(facts.Result{Revision: input.Facts.Revision, Targets: input.Facts.Targets, Facts: append(slices.Clone(input.Facts.Facts), export)})
	if err != nil {
		t.Fatal(err)
	}
	input.Facts = sealed
	wire, cat, err := buildOverview(input)
	if err != nil {
		t.Fatal(err)
	}
	var entrypoints, exports []string
	exportRef := ""
	for _, row := range wire.Facts {
		switch row.Kind {
		case string(facts.KindEntrypoint):
			entrypoints = append(entrypoints, row.Key+" "+row.Symbol)
		case exportKind:
			exports = append(exports, row.Symbol)
			exportRef = row.Ref
		}
	}
	if !slices.Equal(entrypoints, []string{"callable Serve"}) || !slices.Equal(exports, []string{"Apply"}) {
		t.Fatalf("entrypoint rows %v, export rows %v", entrypoints, exports)
	}
	alpha := fixture.targetID("alpha")
	result, err := normalizeOverview(encodeResponse(t, map[string]any{
		"roles":      []any{map[string]any{"target": alpha, "role": "Library", "purpose": "Applies items.", "refs": []string{exportRef}}},
		"run_recipe": []any{map[string]any{"target": alpha, "command": "go run .", "cwd": "alpha", "refs": []string{exportRef}}},
	}), cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.roles) != 1 || len(result.roles[0].FactIDs) != 1 || len(result.recipe) != 0 {
		t.Fatalf("an export is its library's role evidence and no run step's: roles %+v recipe %+v", result.roles, result.recipe)
	}
}
