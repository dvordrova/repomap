package jstsproject

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func assertCumulativeJSTSValueReads(t *testing.T, result Result, index programindex.Index) {
	t.Helper()
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	want := map[string][]string{
		"valueReferences": {"paintColor", "paintColor", "paintColor", "paintColor", "paintColor", "paintColor"},
		"localReadShadow": {"localReadShadow.READ_COLOR"}, "parameterReadShadow": nil, "typeOnlyRead": nil,
		"writeOnlyReferences": nil, "readModifyReferences": {"readModifyReferences.counter", "readModifyReferences.counter", "readModifyReferences.counter"},
		"ElementReferences": {"ReadTile", "ReadTile", "paintColor"}, "TagShadow": nil,
		"directTileCall": nil, "deferredRead": nil, "jsValueReferences": {"paintColor"}, "jsReadShadow": nil,
	}
	got := map[string][]string{}
	reads := map[string]programindex.Relation{}
	var colorLines []int
	jsx := 0
	var propertyReads []programindex.Relation
	for _, relation := range index.Relations {
		caller := objects[relation.FromID].Name
		if caller == "propertyRead" && relation.Kind == programindex.RelationReads {
			propertyReads = append(propertyReads, relation)
			reads[relation.ID] = relation
		}
		if _, checked := want[caller]; !checked || relation.Kind != programindex.RelationReads {
			continue
		}
		for _, id := range relation.ToIDs {
			target := objects[id]
			name := target.Name[strings.LastIndex(target.Name, ".")+1:]
			if name != "paintColor" && name != "READ_COLOR" && name != "counter" && name != "ReadTile" {
				continue
			}
			got[caller] = append(got[caller], target.Name)
			if relation.Location == nil || relation.Location.Column < 1 || target.Location == nil {
				t.Fatalf("read lost original locations: %+v", relation)
			}
			resolution := programindex.ResolutionExact
			if caller == "jsValueReferences" {
				resolution = programindex.ResolutionAlternatives
			}
			if relation.Resolution != resolution {
				t.Fatalf("read changed compiler authority: %+v", relation)
			}
			if target.Name == "paintColor" && (target.Location.Path != "shared/contracts.ts" || target.Location.Line != 5) {
				t.Fatalf("alias/barrel read used a different declaration: %+v", target)
			}
			if caller == "valueReferences" {
				colorLines = append(colorLines, relation.Location.Line)
			}
			if caller == "localReadShadow" && (target.Location.Path != "src/ambiguity.tsx" || target.Location.Line != 92) {
				t.Fatalf("shadowed value borrowed the imported declaration: %+v", target)
			}
			if target.Name == "ReadTile" {
				if relation.Location.Line != 114 || target.Location.Line != 6 || target.Location.Path != "shared/contracts.ts" || len(relation.Witnesses) != 1 || relation.Witnesses[0].Kind != "jsx_tag_reference" {
					t.Fatalf("JSX tag lost compiler binding/site: %+v", relation)
				}
				jsx++
			}
			reads[relation.ID] = relation
		}
	}
	for caller, expected := range want {
		sort.Strings(expected)
		sort.Strings(got[caller])
		if !reflect.DeepEqual(got[caller], expected) {
			t.Errorf("%s reads %v, want %v", caller, got[caller], expected)
		}
	}
	sort.Ints(colorLines)
	if !reflect.DeepEqual(colorLines, []int{86, 87, 88, 88, 89, 89}) || jsx != 2 {
		t.Errorf("read sites collapsed, moved to import/declaration, or counted closing JSX tags: %v / %d", colorLines, jsx)
	}
	if len(propertyReads) != 2 || propertyReads[0].ID == propertyReads[1].ID || propertyReads[0].Location == nil || propertyReads[1].Location == nil || *propertyReads[0].Location != *propertyReads[1].Location {
		t.Fatalf("property and receiver reads sharing a source position were collapsed: %+v", propertyReads)
	}
	var propertyTargets []string
	for _, read := range propertyReads {
		for _, id := range read.ToIDs {
			propertyTargets = append(propertyTargets, objects[id].Name)
		}
	}
	sort.Strings(propertyTargets)
	if !reflect.DeepEqual(propertyTargets, []string{"ReadPalette.color", "currentPalette"}) {
		t.Fatalf("property read borrowed different declarations: %v", propertyTargets)
	}
	for _, relation := range index.Relations {
		if objects[relation.FromID].Name == "ElementReferences" && relation.Kind == programindex.RelationCalls {
			t.Fatalf("JSX tag fabricated execution: %+v", relation)
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
		original, checked := reads[edge.RelationID]
		if !checked || edge.Role != groupindex.EdgeRelationTarget {
			continue
		}
		if edge.RelationKind != programindex.RelationReads || edge.Location == nil || *edge.Location != *original.Location || edge.Resolution != original.Resolution {
			t.Fatalf("GroupsIndex altered the original use: %+v", edge)
		}
		delete(reads, edge.RelationID)
	}
	if len(reads) != 0 {
		t.Fatalf("GroupsIndex lost %d reads", len(reads))
	}
	if len(result.Reads) == 0 {
		t.Fatal("native read handoff empty")
	}
	bad := result.Snapshot()
	bad.Reads[0].ToRefs = []string{"unknown"}
	if _, err := Seal(bad); err == nil {
		t.Fatal("unknown read declaration accepted")
	}
}
