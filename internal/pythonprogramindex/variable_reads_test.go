package pythonprogramindex

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativeVariableReadsKeepOriginalDeclarationAndLexicalScope(t *testing.T) {
	const path = "src/fixture_app/models.py"
	repository := pythonCorpus(t, cumulativePythonSources(t, path, "src/fixture_app/levels.py"))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	want := map[string][]string{
		"read_level_data":    {"fixture_app.levels.READ_VALUES", "fixture_app.levels.READ_LIMIT", "fixture_app.levels.READ_LIMIT", "fixture_app.levels.READ_LIMIT"},
		"shadow_level_data":  {"shadow_level_data.values", "shadow_level_data.READ_LIMIT"},
		"comprehension_data": {"fixture_app.levels.READ_VALUES", "fixture_app.levels.READ_LIMIT"},
		"ReadScope":          {"ReadScope.READ_LIMIT"},
		"method":             {"fixture_app.levels.READ_LIMIT"},
		"read_global_data":   {"fixture_app.levels.READ_LIMIT"},
		"inner_data":         {"outer_data.READ_LIMIT"},
		"with_shadow":        nil, "except_shadow": nil, "match_shadow": nil,
		"read_counter": {"MutableCounter.count"}, "read_replaced_counter": nil,
		"store_level_data": {"fixture_app.levels.READ_VALUES", "fixture_app.levels.READ_LIMIT"},
		"read":             {"MutableCounter.count"}, "increment": {"MutableCounter.count"}, "clear": nil,
	}
	got := map[string][]string{}
	sites := map[string]bool{}
	reads := map[string]programindex.Relation{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationReads || relation.Location == nil || relation.Location.Path != path {
			continue
		}
		caller := objects[relation.FromID].Name
		if _, checked := want[caller]; !checked {
			continue
		}
		for _, id := range relation.ToIDs {
			target := objects[id]
			if target.Name != "READ_LIMIT" && target.Name != "READ_VALUES" && target.Name != "values" && target.Name != "count" {
				continue
			}
			if target.Kind != programindex.ObjectVariable || relation.Resolution != programindex.ResolutionAlternatives || relation.Location.Column < 1 {
				t.Fatalf("read lost declaration, source or possible authority: %+v", relation)
			}
			if sites[relation.ID] {
				t.Fatalf("duplicate read site: %+v", relation)
			}
			sites[relation.ID] = true
			reads[relation.ID] = relation
			owner := target.OwnerID
			if owner == "" {
				owner = target.ContainerID
			}
			got[caller] = append(got[caller], objects[owner].Name+"."+target.Name)
		}
	}
	for caller, expected := range want {
		sort.Strings(expected)
		sort.Strings(got[caller])
		if !reflect.DeepEqual(got[caller], expected) {
			t.Errorf("%s reads %v, want %v", caller, got[caller], expected)
		}
	}
	categorized, err := programindex.Enrich(index, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	groups, _, err := groupindex.Build(categorized, groupindex.Proposals{})
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range groups.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationKind != programindex.RelationReads {
			continue
		}
		original, checked := reads[edge.RelationID]
		if !checked {
			continue
		}
		if edge.Location == nil || *edge.Location != *original.Location || edge.Resolution != original.Resolution {
			t.Fatalf("projection lost original read: %+v", edge)
		}
		delete(reads, edge.RelationID)
	}
	if len(reads) != 0 {
		t.Fatalf("projection lost %d read relations", len(reads))
	}
}
