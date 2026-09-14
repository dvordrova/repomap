package groupindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestAtlasExplicitMembershipKeepsSameFileRelationsAndLexicalChildren(t *testing.T) {
	location := func(file string, line int) *programindex.Location {
		return &programindex.Location{Path: file, Line: line, Column: 1}
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "go", Kind: "executable", Name: "app", Selector: "app", Sources: []programindex.TargetSource{{FileRef: "main", Path: "app/main.go"}}, AnchorFileRef: "main"},
		Objects: []programindex.ObjectInput{
			{SourceRef: "start", Name: "Start", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, Location: location("app/main.go", 3)},
			{SourceRef: "load", Name: "Load", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, Location: location("app/main.go", 20)},
			{SourceRef: "save", Name: "Save", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, Location: location("other/store.go", 5)},
			{SourceRef: "local", Name: "result", Kind: programindex.ObjectVariable, Visibility: programindex.VisibilityInternal, OwnerRef: "load", ContainerRef: "load", Location: location("app/main.go", 21)},
		},
		Relations: []programindex.RelationInput{{SourceRef: "call", Kind: programindex.RelationCalls, FromRef: "start", ToRefs: []string{"load"}, Resolution: programindex.ResolutionExact, TargetsObserved: 1, WitnessesObserved: 1, Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location("app/main.go", 4)}}, Location: location("app/main.go", 4)}},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, object := range program.Objects {
		ids[object.Name] = object.ID
	}
	target := atlas.Target{ID: program.Target.ID, Name: "app", Root: "app", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}, Trace: []string{}, Boxes: []atlas.Box{
		{ID: "entry", Dir: "app", Title: "Application", Line: "Starts the work.", Side: atlas.SideIn, MemberIDs: []string{ids["Start"]}, Keys: []atlas.Key{}, Files: []atlas.File{{Path: "app/main.go", Line: "Mixed responsibilities.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}},
		{ID: "storage", Dir: "other", Title: "Storage", Line: "Loads and saves state.", Side: atlas.SideMid, MemberIDs: []string{ids["Load"], ids["Save"]}, Keys: []atlas.Key{}, Files: []atlas.File{{Path: "app/main.go", Line: "Mixed responsibilities.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}, {Path: "other/store.go", Line: "Saves state.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}},
	}}
	value := atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value)
	if err != nil {
		t.Fatal(err)
	}
	index := indexes[0]
	if len(index.Subjects) != 4 || len(index.Groups) != 2 || len(index.StructuralEdges) == 0 || len(index.Connections) != 1 {
		t.Fatalf("lost facts or collaboration: subjects=%d groups=%d edges=%d connections=%d", len(index.Subjects), len(index.Groups), len(index.StructuralEdges), len(index.Connections))
	}
	for _, group := range index.Groups {
		want := 1
		if group.Title == "Storage" {
			want = 3
		}
		if len(group.MemberSubjectIDs) != want {
			t.Fatalf("file path overrode explicit/native lexical membership: %+v", group)
		}
	}
	connection := index.Connections[0]
	if connection.FromSubjectID != ids["Start"] || connection.ToSubjectID != ids["Load"] || connection.FromLocation.Line != 4 || connection.ToLocation.Line != 20 || connection.SourceKind != "native_calls" || len(connection.Evidence) != 2 {
		t.Fatalf("relation lost exact evidence: %+v", connection)
	}
	value.Targets[0].Boxes[0].MemberIDs = append(value.Targets[0].Boxes[0].MemberIDs, ids["Load"])
	if _, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value); err == nil {
		t.Fatal("conflicting explicit membership accepted")
	}
}
