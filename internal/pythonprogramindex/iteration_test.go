package pythonprogramindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePythonTypedIterationKeepsPossibleMethodsAndWrites(t *testing.T) {
	const path = "src/fixture_app/iteration.py"
	files := cumulativePythonSources(t, path)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	wanted := map[string]bool{
		"typed field through sorted": true, "typed parameter": true,
		"local annotation": true, "homogeneous tuple": true,
		"replaced receiver": false, "replaced item": false,
		"zero iterations possible": false, "shadowed sorted": false,
		"replaced collection": false, "unknown collection": false,
		"zero iterations in else": false, "heterogeneous tuple": false,
		"union item": false, "async iteration": false,
		"annotated underscore parameter": true, "annotated underscore call result": true,
		"untyped underscore parameter": false,
	}
	lines := strings.Split(files[path], "\n")
	seen := map[string]bool{}
	writes := 0
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path {
			continue
		}
		line := lines[relation.Location.Line-1]
		if relation.Kind == programindex.RelationWrites && strings.Contains(line, "typed iteration write") {
			if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 || objects[relation.ToIDs[0]].Name != "position" || objects[objects[relation.ToIDs[0]].OwnerID].Name != "MovingItem" {
				t.Fatalf("iteration write lost its original possible field: %+v", relation)
			}
			writes++
		}
		if relation.Kind != programindex.RelationCalls || !(strings.Contains(line, "item.advance()") || strings.Contains(line, "_.advance()")) {
			continue
		}
		_, marker, _ := strings.Cut(line, "# ")
		known, ok := wanted[marker]
		if !ok || seen[marker] {
			t.Fatalf("unexpected iteration call %q: %+v", marker, relation)
		}
		seen[marker] = true
		if !known {
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatalf("%s invented a target: %+v", marker, relation)
			}
			continue
		}
		if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 || objects[relation.ToIDs[0]].Name != "advance" || objects[objects[relation.ToIDs[0]].OwnerID].Name != "MovingItem" {
			t.Fatalf("%s lost the possible original method: %+v", marker, relation)
		}
	}
	if len(seen) != len(wanted) || writes != 1 {
		t.Fatalf("iteration coverage: calls %v, writes %d", seen, writes)
	}
}
