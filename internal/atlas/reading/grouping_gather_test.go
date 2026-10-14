package reading

import (
	"encoding/json"
	"path"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// A target divided into more than four boxes has them gathered into the
// larger parts a newcomer opens first: each group Jev finds one part is an
// area between the target and its boxes; a box gathered with no other, or in
// a group Jev finds only its boxes side by side, stays at the top. The gathering is asked from the boxes' names, holds and
// counts alone, inside nothing.
func TestManyBoxesAreGatheredIntoLargerParts(t *testing.T) {
	graph := unitGraph(t, serviceFiles())
	provider := &tableProvider{groupFor: func(box, file string) string {
		if box == "example.com/svc" {
			return path.Base(file)
		}
		return file
	}}
	provider.gatherFor = func(request groupGatherInput) []map[string]any {
		refs := map[string]string{}
		for _, box := range request.Boxes {
			refs[box.Name] = box.Ref
		}
		return []map[string]any{
			{"name": "Web", "holds": "Serving requests.", "boxes": []string{refs["routes.go"], refs["auth.go"]}},
			{"name": "Storage", "holds": "Keeping data.", "boxes": []string{refs["db.go"], refs["cache.go"]}},
		}
	}
	jev := newGroupJev()
	jev.apart = []string{"Storage"}
	jev.enough = append(jev.enough, "db.go", "cache.go", "routes.go", "auth.go", "log.go")
	result, err := Read(t.Context(), serviceOptions(t, graph, provider, jev))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	if len(provider.gatherRequests) != 1 {
		t.Fatalf("%d gatherings; want the target's alone", len(provider.gatherRequests))
	}
	var request groupGatherInput
	if err := json.Unmarshal(provider.gatherRequests[0], &request); err != nil {
		t.Fatal(err)
	}
	if request.Box.Name != "example.com/svc" || len(request.Box.Inside) != 0 || len(request.Boxes) <= groupLeafSize {
		t.Fatalf("gathering request %+v", request)
	}
	for _, box := range request.Boxes {
		if box.Declarations == 0 || box.Holds == "" {
			t.Fatalf("box %+v lacks its count or holds", box)
		}
	}
	if !slices.Equal(jev.keptAsked, []string{"Web", "Storage"}) {
		t.Fatalf("Jev was asked about %v; want every proposed group", jev.keptAsked)
	}
	target := result.Atlas.Targets[0]
	zones := map[string]atlas.Zone{}
	for _, zone := range target.Zones {
		zones[zone.Title] = zone
	}
	held := func(zone string) []string {
		var titles []string
		for _, box := range target.Boxes {
			if box.ZoneID == zones[zone].ID {
				titles = append(titles, box.Title)
			}
		}
		slices.Sort(titles)
		return titles
	}
	if zones["Web"].ParentID != "" || !slices.Equal(held("Web"), []string{"auth.go", "routes.go"}) {
		t.Fatalf("areas %+v; want Web at the top holding auth.go and routes.go", target.Zones)
	}
	if _, ok := zones["Storage"]; ok {
		t.Fatal("a group Jev found only its boxes side by side was gathered")
	}
	for _, box := range target.Boxes {
		if (box.Title == "log.go" || box.Title == "db.go" || box.Title == "cache.go") && box.ZoneID != "" {
			t.Fatalf("%s, in no kept group, left the top", box.Title)
		}
	}
}

// The gathering's answer is refused only where it is wrong: forms pass, an
// unknown ref is discarded, a group without a name or holds and a box put in
// two groups are refused, and a group of fewer than two boxes or of every box
// gains nothing.
func TestDecodeGatherRefusesOnlyWhatIsWrong(t *testing.T) {
	answer, err := decodeGather([]byte(`{"Result":{"GROUPS":[
		{"Name":" Web ","holds":"Serving.","boxes":["B1"," b2 ","b2","b9"]},
		{"name":"Data","holds":"Keeping.","boxes":"b3"},
		{"name":"","holds":"Nameless.","boxes":["b4","b5"]},
		{"name":"Twice","holds":"Twice.","boxes":["b5","b6"]},
		{"name":"Again","holds":"Again.","boxes":["b6","b7","b4"]}]}}`), 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []gatherGroup{{name: "Web", holds: "Serving.", boxes: []int{0, 1}}, {name: "Again", holds: "Again.", boxes: []int{6, 3}}}
	if !reflect.DeepEqual(answer.Groups, want) {
		t.Fatalf("groups %+v, want %+v", answer.Groups, want)
	}
	if !reflect.DeepEqual(answer.Refused, []string{"group 3", "b6"}) {
		t.Fatalf("refused %v", answer.Refused)
	}
	if answer, _ := decodeGather([]byte(`{"groups":[{"name":"All","holds":"All.","boxes":["b1","b2"]}]}`), 2); len(answer.Groups) != 0 {
		t.Fatal("a group of every box was kept")
	}
	if answer, err := decodeGather([]byte(`{"groups":null}`), 3); err != nil || len(answer.Groups) != 0 {
		t.Fatalf("null groups: %+v %v", answer, err)
	}
	for _, raw := range []string{`not json`, `{"boxes":[]}`} {
		if _, err := decodeGather([]byte(raw), 3); err == nil {
			t.Fatalf("%s was not refused", raw)
		}
	}
}
