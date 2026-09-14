package pythonprogramindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePythonFieldWritesKeepReceiverAndSource(t *testing.T) {
	const path = "src/fixture_app/models.py"
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
	// Every source-distinct write survives. Read-only methods must not acquire
	// effects merely because they belong to the same type.
	expected := map[string]string{
		"self.count = 0": "MutableCounter", "self.count += 1": "MutableCounter",
		"del self.count": "MutableCounter", "receiver.count: int = 0": "MutableCounter",
		"counter.count = 2": "MutableCounter", "self.count = 3": "",
		"self.count = 4": "", "self.count = 5": "MutableCounter",
		"inner.count = 6": "OtherCounter", "counter.count = 7": "MutableCounter",
		"counter.count = 8": "MutableCounter", "counter.count = 9": "",
		`setattr(counter, "count", 10)`: "", "counter.count = 11": "",
	}
	lines := strings.Split(files[path], "\n")
	seen := map[string]bool{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationWrites || relation.Location == nil || relation.Location.Path != path {
			continue
		}
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		owner, wanted := expected[line]
		if !wanted || seen[line] {
			t.Fatalf("unexpected or duplicate write at %q: %+v", line, relation)
		}
		seen[line] = true
		if owner == "" {
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatalf("dynamic or replaced receiver acquired a field: %q %+v", line, relation)
			}
			continue
		}
		if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 {
			t.Fatalf("write lost its possible field: %q %+v", line, relation)
		}
		field := objects[relation.ToIDs[0]]
		if field.Kind != programindex.ObjectVariable || field.Name != "count" || objects[field.OwnerID].Name != owner || relation.Location.Column < 1 {
			t.Fatalf("write gained the wrong field owner at %q: %+v", line, field)
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("missing writes: saw %v, wanted %v", seen, expected)
	}
	categorized, err := programindex.Enrich(index, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := groupindex.Build(categorized, groupindex.Proposals{})
	if err != nil {
		t.Fatal(err)
	}
	native := map[string]programindex.Relation{}
	for _, relation := range index.Relations {
		native[relation.ID] = relation
	}
	writes := 0
	for _, edge := range groups.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationKind != programindex.RelationWrites {
			continue
		}
		original := native[edge.RelationID]
		if edge.Location == nil || *edge.Location != *original.Location || edge.Resolution != original.Resolution {
			t.Fatalf("group projection lost the original write site or uncertainty: %+v", edge)
		}
		writes++
	}
	if writes != 9 {
		t.Fatalf("group projection kept %d writes, want 9 resolved candidates", writes)
	}
}
