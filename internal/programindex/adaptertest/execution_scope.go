package adaptertest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// AssertExecutionScope checks a real fixture call at its written location.
// A call in a module body must remain selectable as that body, rather than
// disappearing or borrowing a declaration elsewhere in the same source file.
func AssertExecutionScope(t *testing.T, index programindex.Index, graph atlas.Graph, path string, line int, kind programindex.ObjectKind) {
	t.Helper()
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	seen := 0
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path || relation.Location.Line != line ||
			(relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal && relation.Kind != programindex.RelationExecutes) {
			continue
		}
		owner := objects[relation.FromID]
		if owner.Kind != kind || owner.Location == nil {
			t.Fatalf("%s:%d native call owner = %+v, want %s", path, line, owner, kind)
		}
		id := atlas.SymbolID(owner.Location.Path, owner.Location.Line, owner.Name)
		var place *atlas.Place
		for i := range graph.Places {
			if graph.Places[i].ID == id {
				place = &graph.Places[i]
			}
		}
		if place == nil || place.Symbol == nil || !place.Symbol.Candidate || place.Symbol.Decl.ObjectID != owner.ID || place.Symbol.Decl.Kind != string(kind) {
			t.Fatalf("%s:%d lost its native %s grouping candidate: owner=%+v, place=%+v", path, line, kind, owner, place)
		}
		found := false
		columns := map[int]bool{relation.Location.Column: true}
		for _, pattern := range relation.Patterns {
			if pattern.Location != nil && pattern.Location.Path == path && pattern.Location.Line == line {
				columns[pattern.Location.Column] = true
			}
		}
		for _, call := range place.Symbol.Calls {
			if call.Line == line && columns[call.Column] && call.Kind == string(relation.Kind) && call.Resolution == string(relation.Resolution) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s:%d candidate lost the original call: %+v", path, line, place.Symbol.Calls)
		}
		seen++
	}
	if seen == 0 {
		t.Fatalf("no native call at fixture %s:%d", path, line)
	}
}
