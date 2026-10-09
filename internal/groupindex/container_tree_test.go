package groupindex

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestAreaTreeProjectionSealHydrateAndDerivedViewsKeepEmptyDirectParent(t *testing.T) {
	p := helperTestProgram(t)
	a := helperTestAtlas(p)
	a.Targets[0].Zones = append([]atlas.Zone{{ID: "root", Title: "Runtime", BoxIDs: []string{}}}, a.Targets[0].Zones...)
	a.Targets[0].Zones[1].ParentID = "root"
	a.Targets[0].Zones[2].ParentID = "root"
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, a)
	if err != nil {
		t.Fatal(err)
	}
	index := indexes[0]
	if index.Containers[0].ParentID != "" || len(index.Containers[0].GroupIDs) != 0 || index.Containers[1].ParentID != "k1" || len(ContainerGroups(index.Containers, "k1")) != 4 || index.Containers[0].Lane != LaneTriggers || !index.Containers[0].Core {
		t.Fatalf("wrong tree %v", index.Containers)
	}
	sealed := OverlayFromIndex(index)
	raw, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeOverlay(raw)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := decoded.Hydrate(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Containers, index.Containers) || !reflect.DeepEqual(restored.Connections, index.Connections) {
		t.Fatal("roundtrip changed tree or original connections")
	}
	views := TestFreeViews([]Index{index}, map[string]bool{})
	if len(views[0].Containers) != 3 {
		t.Fatalf("TestFree dropped ancestor %v", views[0].Containers)
	}
	canonical := map[Endpoint]Endpoint{}
	for _, g := range index.Groups {
		at := Endpoint{TargetID: index.Target.ID, GroupID: g.ID}
		canonical[at] = at
	}
	if !reflect.DeepEqual(foldContainers(index, canonical), index.Containers) {
		t.Fatal("fold dropped tree")
	}
	// Filtering a complete sibling leaves a unary ancestor in the derived
	// view. Keep that original navigation ancestry, without claiming a new
	// model decision over the smaller scope.
	testPaths := map[string]bool{}
	for _, group := range index.Groups {
		if !slices.Contains(index.Containers[2].GroupIDs, group.ID) {
			continue
		}
		for _, subject := range index.Subjects {
			if slices.Contains(group.MemberSubjectIDs, subject.ID) && subject.Object != nil && subject.Object.Location != nil {
				testPaths[subject.Object.Location.Path] = true
			}
		}
	}
	filtered := TestFreeViews([]Index{index}, testPaths)[0]
	if len(filtered.Groups) != 2 || len(filtered.Containers) != 2 || filtered.Containers[0].ID != "k1" || filtered.Containers[1].ParentID != "k1" || len(filtered.Containers[0].GroupIDs) != 0 {
		t.Fatalf("filtered view lost original unary ancestry: %v", filtered.Containers)
	}
	for _, mutate := range []func(*Index){
		func(i *Index) { i.Containers[0].ParentID = "k3" },
		func(i *Index) { i.Containers[2].ParentID = "unknown" },
		func(i *Index) { i.Containers[0].GroupIDs = slices.Clone(i.Containers[1].GroupIDs) },
		func(i *Index) {
			i.Containers = []Container{
				{ID: "k1", GroupIDs: []string{}},
				{ID: "k2", ParentID: "k1", GroupIDs: slices.Clone(ContainerGroups(index.Containers, "k1"))},
			}
		},
	} {
		copy := index.Snapshot()
		mutate(&copy)
		if err := copy.Validate(); err == nil {
			t.Fatal("invalid tree accepted")
		}
	}
}
