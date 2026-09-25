package groupindex

import (
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
		Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Trace: []string{},
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
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryHTTPServer, Method: "GET", Values: []string{"/loose"}, Line: "Answers the loose route.", FactID: "route"}},
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
	if len(index.OffMap) != 2 || index.OffMap[0] != want[0] || index.OffMap[1] != want[1] {
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
