package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// The entries one function's calls make by comparing literal elements of one
// value: the lowest element's are entries, each higher element's a value of
// the entry written last before it (Redis's loadServerConfig: "appendfsync:
// always | everysec | no"). A value with no entry before it stays an entry.
func TestAValueComparedAfterItsDirectiveIsItsSubArgument(t *testing.T) {
	base := sourcevalue.Anchor{Path: "redis.c", Line: 1642, Column: 16}
	element := func(index int) []atlas.SourceArgument {
		return []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "index", Parts: []sourcevalue.Value{
			{Kind: "call_result", Text: "sdssplitlen(...)", Anchor: &base}, {Kind: "literal", Text: string(rune('0' + index))}}}}}
	}
	r := &reader{boundaries: map[string]*boundaryState{}}
	add := func(id string, line, index int) {
		r.boundaries[id] = &boundaryState{handlerUnknown: true, element: comparedElement(element(index)), place: atlas.Place{ID: id, Path: "redis.c", LineNo: line,
			Boundary: &atlas.BoundaryFacts{ObjectID: "loadServerConfig"}}}
	}
	add("stray", 1650, 1)
	add("loglevel", 1660, 0)
	add("debug", 1661, 1)
	add("verbose", 1662, 1)
	add("appendfsync", 1740, 0)
	add("always", 1741, 1)
	r.foldValues()
	got := map[string]string{}
	for id, state := range r.boundaries {
		got[id] = state.valueOf
	}
	want := map[string]string{"stray": "", "loglevel": "", "debug": "loglevel", "verbose": "loglevel", "appendfsync": "", "always": "appendfsync"}
	for id, of := range want {
		if got[id] != of {
			t.Fatalf("values: %v", got)
		}
	}
	if comparedElement([]atlas.SourceArgument{{Origin: &sourcevalue.Value{Kind: "literal", Text: "always"}}}) != nil {
		t.Fatal("a literal argument compares no element")
	}
}
