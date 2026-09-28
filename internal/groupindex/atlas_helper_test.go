package groupindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// helperTestProgram is target svc with one function per file, FA in a.go
// through FD in d.go; FA is the target's seed and calls FB and FC.
func helperTestProgram(t *testing.T) programindex.Index {
	t.Helper()
	files := []string{"svc/a.go", "svc/b.go", "svc/c.go", "svc/d.go"}
	var objects []programindex.ObjectInput
	for i, file := range files {
		objects = append(objects, programindex.ObjectInput{
			SourceRef: "o" + string(rune('a'+i)), Kind: programindex.ObjectFunction, Name: "F" + string(rune('A'+i)),
			Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: file, Line: 3, Column: 1},
		})
	}
	call := func(to string, line int) programindex.RelationInput {
		at := &programindex.Location{Path: "svc/a.go", Line: line, Column: 2}
		return programindex.RelationInput{SourceRef: "a-calls-" + to, Kind: programindex.RelationCalls, FromRef: "oa", ToRefs: []string{to},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: at,
			Witnesses: []programindex.Witness{{Kind: "call", Location: at}}, WitnessesObserved: 1}
	}
	relations := []programindex.RelationInput{call("ob", 4), call("oc", 5)}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: "svc", Selector: "svc",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: files[0]}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "oa", Kind: programindex.SeedCallable, Location: &programindex.Location{Path: files[0], Line: 3, Column: 1}}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

// helperTestAtlas draws one part per file of helperTestProgram: FB is a
// helper, D is the domain, c.go takes requests. Areas: "Start" holds a.go
// (the entry) and b.go; "Requests" holds c.go and d.go.
func helperTestAtlas(p programindex.Index) atlas.Atlas {
	box := func(id, path, name string, index int, side string, core, helper bool) atlas.Box {
		object := p.Objects[index]
		return atlas.Box{ID: id, Dir: "svc", Title: name, Line: name + " line.", Side: side, Core: core, Open: true, MemberIDs: []string{object.ID},
			Files: []atlas.File{{Path: path, Line: "File.", Source: atlas.SourceModel, Open: true, Asked: true,
				Symbols: []atlas.Symbol{{ID: "s" + id, ObjectID: object.ID, Name: object.Name, Kind: "function", LineNo: 3, Column: 1, Helper: helper}}}}}
	}
	return atlas.Atlas{Version: atlas.Version, Repository: "x", Revision: "abc", Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{},
		Targets: []atlas.Target{{
			ID: p.Target.ID, Language: "go", Kind: "executable", Name: "svc", Root: "svc",
			Zones: []atlas.Zone{
				{ID: "z1", Title: "Start", Line: "Starts.", BoxIDs: []string{"pa", "pb"}},
				{ID: "z2", Title: "Requests", Line: "Serves.", BoxIDs: []string{"pc", "pd"}},
			},
			Boxes: []atlas.Box{
				box("pa", "svc/a.go", "Main", 0, atlas.SideIn, false, false),
				box("pb", "svc/b.go", "Formatting", 1, atlas.SideMid, false, true),
				box("pc", "svc/c.go", "Handlers", 2, atlas.SideIn, false, false),
				box("pd", "svc/d.go", "Domain", 3, atlas.SideMid, true, false),
			},
			Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{},
		}},
	}
}

// A relation into a declaration the helper question marked is a connection
// to a helper, and no other is. The mark is saved as the subject's
// interpretation and derived again on decoding, so a report rendered from
// the saved index quiets the same arrows as the ordinary run.
func TestAConnectionIntoAHelperIsMarkedAfterDecoding(t *testing.T) {
	p := helperTestProgram(t)
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, helperTestAtlas(p))
	if err != nil {
		t.Fatal(err)
	}
	check := func(index Index, when string) {
		t.Helper()
		marked := map[string]bool{}
		for _, connection := range index.Connections {
			marked[connection.ToSubjectID] = connection.ToHelper
		}
		if len(marked) != 2 || !marked[p.Objects[1].ID] || marked[p.Objects[2].ID] {
			t.Fatalf("%s: connections into helpers %v (FB %s is the helper)", when, marked, p.Objects[1].ID)
		}
	}
	check(indexes[0], "projected")
	encoded, err := Encode(indexes[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"helper":true`) || strings.Contains(string(encoded), "to_helper") {
		t.Fatalf("the saved index does not carry the helper interpretation alone:\n%s", encoded)
	}
	decoded, err := Decode(encoded, p)
	if err != nil {
		t.Fatal(err)
	}
	check(decoded, "decoded")
}

// An area's marks come from data. Only an area holding the program's entry
// (a target seed) stands in the triggers lane; an area whose part takes
// requests without holding the entry does not. An area is core when any part
// in it is.
func TestAnAreaIsTheEntryOnlyByItsSeedAndCoreByAnyPart(t *testing.T) {
	p := helperTestProgram(t)
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, helperTestAtlas(p))
	if err != nil {
		t.Fatal(err)
	}
	areas := map[string]Container{}
	for _, container := range indexes[0].Containers {
		areas[container.Title] = container
	}
	if start := areas["Start"]; start.Lane != LaneTriggers || start.Core {
		t.Fatalf("the entry's area: %+v", start)
	}
	if requests := areas["Requests"]; requests.Lane == LaneTriggers || !requests.Core {
		t.Fatalf("the area taking requests with a domain part: %+v", requests)
	}
}
