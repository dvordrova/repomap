package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// An entry a declaration names again on the same object, named alike and
// answered the same kind, is that entry: etcd's proto-annotations declares
// --annotation with cmd.Flags().StringVar and names it again with
// cmd.MarkFlagRequired, both on cmd. The same name declared on another
// object (a second subcommand's), answered another kind, or on an object
// not known stays its own entry.
func TestAnEntryNamedAgainOnItsObjectIsThatEntry(t *testing.T) {
	cmd := &atlas.DeclaredOn{Path: "cmd/root.go", LineNo: 30, Column: 23, Text: "cmd := &cobra.Command{"}
	other := &atlas.DeclaredOn{Path: "cmd/root.go", LineNo: 70, Column: 2, Text: "sub := &cobra.Command{"}
	entry := func(id string, line int, kind, name string, on *atlas.DeclaredOn) *boundaryState {
		return &boundaryState{place: atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: "cmd/root.go", LineNo: line, Column: 5,
			Boundary: &atlas.BoundaryFacts{ObjectID: "t1.n2", Direction: atlas.DirectionIn, Source: "model"}},
			handlerUnknown: true, kind: kind, name: name, on: on}
	}
	r := &reader{boundaries: map[string]*boundaryState{
		"declared": entry("declared", 56, "command", "annotation", cmd),
		"required": entry("required", 57, "command", "annotation", cmd),
		"other":    entry("other", 80, "command", "annotation", other),
		"setting":  entry("setting", 58, "setting", "annotation", cmd),
		"unknown":  entry("unknown", 59, "command", "annotation", nil),
	}}
	r.foldRepeated()
	got := map[string]string{}
	for id, state := range r.boundaries {
		got[id] = state.aliasOf
	}
	want := map[string]string{"declared": "", "required": "declared", "other": "", "setting": "", "unknown": ""}
	for id, alias := range want {
		if got[id] != alias {
			t.Fatalf("aliases = %v, want %v", got, want)
		}
	}
}
