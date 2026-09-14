package groupindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestSelectedModuleBodyKeepsCallsWithoutAssigningTheWholeFile(t *testing.T) {
	loc := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app.py", Line: line, Column: 1}
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "python", Kind: "executable", Name: "app", Selector: "app", Sources: []programindex.TargetSource{{FileRef: "app", Path: "app.py"}}, AnchorFileRef: "app"},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Name: "app", Kind: programindex.ObjectModule, Visibility: programindex.VisibilityPublic, Location: loc(1)},
			{SourceRef: "work", Name: "work", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, OwnerRef: "module", ContainerRef: "module", Location: loc(3)},
			{SourceRef: "omitted", Name: "unassigned", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, OwnerRef: "module", ContainerRef: "module", Location: loc(8)},
			{SourceRef: "local", Name: "result", Kind: programindex.ObjectVariable, Visibility: programindex.VisibilityInternal, OwnerRef: "work", ContainerRef: "work", Location: loc(4)},
		},
		Relations: []programindex.RelationInput{{SourceRef: "call", Kind: programindex.RelationCalls, FromRef: "module", ToRefs: []string{"work"}, Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: loc(12), WitnessesObserved: 1, Witnesses: []programindex.Witness{{Kind: "direct_call", Location: loc(12)}}}},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, object := range program.Objects {
		ids[object.Name] = object.ID
	}
	file := atlas.File{Path: "app.py", Source: atlas.SourceModel, Line: "Starts the work.", Symbols: []atlas.Symbol{}}
	value := atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{{ID: program.Target.ID, Name: "app", Root: ".", Boxes: []atlas.Box{
		{ID: "start", Dir: ".", Title: "Startup", Line: "Calls work during startup.", Side: atlas.SideIn, MemberIDs: []string{ids["app"]}, Files: []atlas.File{file}, Keys: []atlas.Key{}},
		{ID: "work", Dir: ".", Title: "Work", Line: "Does the work.", Side: atlas.SideMid, MemberIDs: []string{ids["work"]}, Files: []atlas.File{file}, Keys: []atlas.Key{}},
	}, Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}, Trace: []string{}}}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value)
	if err != nil {
		t.Fatal(err)
	}
	index := indexes[0]
	for _, group := range index.Groups {
		want := 1
		if group.Title == "Work" {
			want = 2
		}
		if len(group.MemberSubjectIDs) != want {
			t.Fatalf("module assigned other declarations: %+v", group)
		}
		for _, id := range group.MemberSubjectIDs {
			if id == ids["unassigned"] {
				t.Fatal("a refused/unassigned declaration borrowed its module's part")
			}
		}
	}
	if len(index.Connections) != 1 {
		t.Fatalf("module call disappeared: %+v", index.Connections)
	}
	call := index.Connections[0]
	if call.FromSubjectID != ids["app"] || call.ToSubjectID != ids["work"] || call.FromLocation.Line != 12 || call.ToLocation.Line != 3 {
		t.Fatalf("module call lost original endpoints: %+v", call)
	}
}
