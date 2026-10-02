package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Materializing the fixture checks its exact tracked-file inventory.
func TestCumulativeJSTSRepositoryFileInventory(t *testing.T) {
	materializeFixtureRepository(t, "jsts")
}

// A library package's entries are its API. The canvas UI's manifest exports
// canvas.mjs alone, so drawCanvas is its API and layout, which layout.mjs
// exports to its sibling, is not. The local store's manifest names no entry
// module, so every export of its own modules is.
func TestCumulativeJSTSLibraryExportsItsAPI(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "jsts")
	for _, want := range []struct {
		selector string
		basis    programindex.ExportBasis
		names    []string
	}{
		{"jsts:packages/canvas-ui/package.json", programindex.ExportsEntryModules, []string{"packages/canvas-ui/canvas.mjs drawCanvas"}},
		{"jsts:packages/local-store/package.json", programindex.ExportsVisibility, []string{"packages/local-store/src/index.ts createClient", "packages/local-store/src/index.ts get"}},
	} {
		result, err := jstsproject.DiscoverSelected(t.Context(), repository, root, want.selector)
		if err != nil {
			t.Fatal(err)
		}
		index, _, err := jstsproject.BuildFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, export := range index.Target.Exports {
			got = append(got, export.Location.Path+" "+programIndexObjectByID(index, export.ObjectID).Name)
		}
		slices.Sort(got)
		if index.Target.ExportBasis != want.basis || !slices.Equal(got, want.names) {
			t.Fatalf("%s exports %v by %q, want %v by %q", want.selector, got, index.Target.ExportBasis, want.names, want.basis)
		}
	}
}
