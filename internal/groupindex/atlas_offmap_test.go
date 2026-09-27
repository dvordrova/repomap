package groupindex

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A file off the map keeps its declarations' interpretations and its
// boundaries: an incoming route there is an operation of no group. A part
// made only of test code is listed off the map as tests. A declaration off
// the map in a file a part holds keeps its interpretation, and its file is
// not listed. The record survives the persisted overlay.
func TestOffMapFilesKeepTheirBoundariesAndInterpretations(t *testing.T) {
	p := atlasTestProgram(t, "server", "api/a.go", "loose/b.go", "api/a_test.go", "api/a.go")
	var stray string
	for _, object := range p.Objects {
		if object.Name == "FD" {
			stray = object.ID
		}
	}
	objectIn := func(path string) string {
		for _, object := range p.Objects {
			if object.Location != nil && object.Location.Path == path {
				return object.ID
			}
		}
		t.Fatalf("no object in %s", path)
		return ""
	}
	file := func(path, line string, symbols ...atlas.Symbol) atlas.File {
		return atlas.File{Path: path, Line: line, Source: atlas.SourceModel, Symbols: append([]atlas.Symbol{}, symbols...)}
	}
	target := atlas.Target{
		ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: ".",
		Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boxes: []atlas.Box{
			{ID: "p1", Dir: "api", Title: "API", Side: atlas.SideIn, MemberIDs: []string{objectIn("api/a.go")}, Keys: []atlas.Key{},
				Files: []atlas.File{file("api/a.go", "Serves.", atlas.Symbol{ID: "s1", ObjectID: objectIn("api/a.go"), Name: "FA", Kind: "function", LineNo: 3})}},
			{ID: "p2", Dir: "api", Title: "API checks", Side: atlas.SideMid, ForTests: true, MemberIDs: []string{objectIn("api/a_test.go")}, Keys: []atlas.Key{},
				Files: []atlas.File{file("api/a_test.go", "Checks.", atlas.Symbol{ID: "s3", ObjectID: objectIn("api/a_test.go"), Name: "FC", Kind: "function", LineNo: 3})}},
		},
		OffMap: []atlas.OffMapFile{
			{ID: "f1", Reason: atlas.OffMapLeftOut, BoxID: "p1",
				File: file("api/a.go", "Serves.", atlas.Symbol{ID: "s4", ObjectID: stray, Name: "FD", Kind: "method", LineNo: 3, Line: "Runs a job."})},
			{ID: "f2", Reason: atlas.OffMapLeftOut,
				File: file("loose/b.go", "Handles a loose route.", atlas.Symbol{ID: "s2", ObjectID: objectIn("loose/b.go"), Name: "FB", Kind: "function", LineNo: 3, Line: "Answers the loose route.", Key: true})},
		},
		Boundaries: []atlas.Boundary{{ID: "b1", ObjectID: objectIn("loose/b.go"), Path: "loose/b.go", LineNo: 3, Column: 1,
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest, Method: "GET", Values: []string{"/loose"}, Name: "GET /loose", Line: "Answers the loose route.", FactID: "route"}},
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	if err := atlas.Validate(value); err != nil {
		t.Fatal(err)
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, value)
	if err != nil {
		t.Fatal(err)
	}
	index := indexes[0]
	if len(index.Groups) != 1 || index.Groups[0].Title != "API" || index.Groups[0].Summary != "" {
		t.Fatalf("groups: %+v", index.Groups)
	}
	if len(index.Operations) != 1 || index.Operations[0].GroupID != "" || index.Operations[0].SubjectID != objectIn("loose/b.go") || index.Operations[0].Name != "GET /loose" {
		t.Fatalf("the route off the map was lost or given a group: %+v", index.Operations)
	}
	want := []OffMapFile{{Path: "api/a_test.go", Reason: OffMapTests, Part: "API checks"}, {Path: "loose/b.go", Reason: atlas.OffMapLeftOut}}
	if !reflect.DeepEqual(index.OffMap, want) {
		t.Fatalf("off the map: %+v", index.OffMap)
	}
	for _, subject := range index.Subjects {
		if subject.ID == objectIn("loose/b.go") && (subject.Interpretation == nil || !subject.Interpretation.Key || len(subject.Categories) != 0) {
			t.Fatalf("an off-map declaration lost its interpretation or gained a lane: %+v", subject)
		}
		if subject.ID == stray && (subject.Interpretation == nil || subject.Interpretation.Line != "Runs a job." || len(subject.Categories) != 0) {
			t.Fatalf("a declaration off the map in a placed file: %+v", subject)
		}
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.OffMap) != 2 || decoded.Operations[0].GroupID != "" {
		t.Fatalf("the overlay lost the off-map record: %+v", decoded.OffMap)
	}
}

// A part its program never runs leaves that program's map as a part made
// only of test code does: redis-cli links adlist.c and calls none of its
// functions, and "Linked list" stood on its canvas with an arrow to Memory
// allocation. It is no group and draws no connection; its declarations,
// types included, are listed off the map by file with its name.
func TestAPartItsProgramNeverRunsIsListedOffTheMapByItsDeclarations(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 1}
	}
	call := func(ref, from, to string, location *programindex.Location) programindex.RelationInput {
		return programindex.RelationInput{SourceRef: ref, Kind: programindex.RelationCalls, FromRef: from, ToRefs: []string{to},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: location,
			Witnesses: []programindex.Witness{{Kind: "call", Location: location}}, WitnessesObserved: 1}
	}
	function := func(ref, name, path string, line int, unreachable bool) programindex.ObjectInput {
		return programindex.ObjectInput{SourceRef: ref, Kind: programindex.ObjectFunction, Name: name, Visibility: programindex.VisibilityPublic,
			Location: at(path, line), Unreachable: unreachable}
	}
	objects := []programindex.ObjectInput{
		function("main", "main", "cli.c", 3, false),
		function("zmalloc", "zmalloc", "zmalloc.c", 3, false),
		function("listCreate", "listCreate", "list.c", 3, true),
		function("listRelease", "listRelease", "list.c", 9, true),
		{SourceRef: "list", Kind: programindex.ObjectType, Name: "list", Visibility: programindex.VisibilityPublic, Location: at("list.h", 3)},
	}
	relations := []programindex.RelationInput{call("main-zmalloc", "main", "zmalloc", at("cli.c", 4)), call("listCreate-zmalloc", "listCreate", "zmalloc", at("list.c", 4))}
	p, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "c", Kind: "executable", Name: "cli", Selector: "cli",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "cli.c"}}, AnchorFileRef: "f1"},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatal(err)
	}
	p = rebindTestTargets(t, p)[0]
	id := map[string]string{}
	for _, object := range p.Objects {
		id[object.Name] = object.ID
	}
	symbol := func(name, kind string, line int) atlas.Symbol {
		return atlas.Symbol{ID: "s-" + name, ObjectID: p.Target.ID + "." + id[name], Name: name, Kind: kind, LineNo: line, Column: 1}
	}
	file := func(path string, symbols ...atlas.Symbol) atlas.File {
		return atlas.File{Path: path, Source: atlas.SourceModel, Symbols: symbols}
	}
	target := atlas.Target{
		ID: p.Target.ID, Name: p.Target.Name, Language: "c", Kind: "executable", Root: ".",
		Zones: []atlas.Zone{}, Boundaries: []atlas.Boundary{},
		Arrows: []atlas.Arrow{{ID: "a1", From: "p1", To: "p3", Calls: 1, Witnesses: []atlas.Witness{}, Sentence: "Client calls Memory: zmalloc."},
			{ID: "a2", From: "p2", To: "p3", Calls: 1, Witnesses: []atlas.Witness{}, Sentence: "Linked list calls Memory: zmalloc."}},
		Boxes: []atlas.Box{
			{ID: "p1", Dir: ".", Title: "Client", Side: atlas.SideIn, Keys: []atlas.Key{}, MemberIDs: []string{id["main"]},
				Files: []atlas.File{file("cli.c", symbol("main", "function", 3))}},
			{ID: "p2", Dir: ".", Title: "Linked list", Side: atlas.SideMid, Unreached: true, Keys: []atlas.Key{}, MemberIDs: []string{id["listCreate"], id["listRelease"], id["list"]},
				Files: []atlas.File{file("list.c", symbol("listCreate", "function", 3), symbol("listRelease", "function", 9)), file("list.h", symbol("list", "type", 3))}},
			{ID: "p3", Dir: ".", Title: "Memory", Side: atlas.SideOut, Keys: []atlas.Key{}, MemberIDs: []string{id["zmalloc"]},
				Files: []atlas.File{file("zmalloc.c", symbol("zmalloc", "function", 3))}},
		},
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	if err := atlas.Validate(value); err != nil {
		t.Fatal(err)
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, value)
	if err != nil {
		t.Fatal(err)
	}
	index := indexes[0]
	var titles []string
	for _, group := range index.Groups {
		titles = append(titles, group.Title)
	}
	if slices.Sort(titles); !reflect.DeepEqual(titles, []string{"Client", "Memory"}) {
		t.Fatalf("groups = %v, want the parts the program runs", titles)
	}
	var drawn []string
	for _, connection := range index.Connections {
		drawn = append(drawn, connection.Label)
	}
	if !reflect.DeepEqual(drawn, []string{"main calls zmalloc"}) {
		t.Fatalf("connections = %v, want only main's", drawn)
	}
	want := []OffMapFile{
		{Path: "list.c", Reason: OffMapUnreachable, Part: "Linked list", SubjectIDs: []string{id["listCreate"], id["listRelease"]}},
		{Path: "list.h", Reason: OffMapUnreachable, Part: "Linked list", SubjectIDs: []string{id["list"]}},
	}
	if !reflect.DeepEqual(index.OffMap, want) {
		t.Fatalf("off the map: %+v\nwant %+v", index.OffMap, want)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := Decode(encoded, p); err != nil || !reflect.DeepEqual(decoded.OffMap, want) {
		t.Fatalf("the overlay lost the unreachable part: %v %+v", err, decoded.OffMap)
	}
}
