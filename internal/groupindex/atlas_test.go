package groupindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func atlasTestProgram(t *testing.T, name string, files ...string) programindex.Index {
	t.Helper()
	objects := make([]programindex.ObjectInput, 0, len(files))
	for i, file := range files {
		objects = append(objects, programindex.ObjectInput{
			SourceRef: "o" + string(rune('a'+i)), Kind: programindex.ObjectFunction, Name: "F" + string(rune('A'+i)),
			Visibility: programindex.VisibilityPublic,
			Location:   &programindex.Location{Path: file, Line: 3, Column: 1},
		})
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: name, Selector: name,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: files[0]}}, AnchorFileRef: "f1",
		},
		Objects:   objects,
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func TestProjectAtlasMakesGroupsContainersAndConnections(t *testing.T) {
	svc := atlasTestProgram(t, "svc", "svc/api/h.go", "svc/core/c.go")
	web := atlasTestProgram(t, "web", "web/src/app.ts")
	value := atlas.Atlas{
		Version: atlas.Version, Repository: "x", Revision: "abc",
		Targets: []atlas.Target{
			{
				ID: svc.Target.ID, Language: "go", Kind: "executable", Name: "svc", Root: "svc",
				Zones: []atlas.Zone{{ID: "serving", Title: "Serving", Line: "Serves things.", BoxIDs: []string{"svc/api", "svc/core"}}},
				Boxes: []atlas.Box{
					{ID: "svc/api", Dir: "svc/api", Title: "HTTP handlers", Line: "Answers requests.", ZoneID: "serving", Side: atlas.SideIn, Open: true,
						Files: []atlas.File{{Path: "svc/api/h.go", Line: "Handler file.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}, Keys: []atlas.Key{}},
					{ID: "svc/core", Dir: "svc/core", Title: "Domain", Line: "Does the work.", ZoneID: "serving", Side: atlas.SideMid, Open: true,
						Files: []atlas.File{{Path: "svc/core/c.go", Line: "Core file.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}, Keys: []atlas.Key{}},
				},
				Arrows:     []atlas.Arrow{{From: "svc/api", To: "svc/core", Calls: 3, Witnesses: []atlas.Witness{}, Sentence: "The handlers hand requests to the domain."}},
				Boundaries: []atlas.Boundary{{ID: "b-in", BoxID: "svc/api", Path: "svc/api/h.go", LineNo: 10, Caller: "FA", Direction: atlas.DirectionIn, Kind: atlas.BoundaryHTTPServer, Values: []string{"/api/levels"}, Line: "Serves levels."}},
				Trace:      []string{"svc/api", "svc/core"},
			},
			{
				ID: web.Target.ID, Language: "typescript", Kind: "package", Name: "web", Root: "web",
				Zones: []atlas.Zone{},
				Boxes: []atlas.Box{{ID: "web/src", Dir: "web/src", Title: "Client", Line: "Calls the API.", Side: atlas.SideOut, Open: true,
					Files: []atlas.File{{Path: "web/src/app.ts", Line: "App.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}, Keys: []atlas.Key{}}},
				Arrows:     []atlas.Arrow{},
				Boundaries: []atlas.Boundary{{ID: "b-out", BoxID: "web/src", Path: "web/src/app.ts", LineNo: 5, Caller: "FA", Direction: atlas.DirectionOut, Kind: atlas.BoundaryHTTPClient, Values: []string{"/api/levels"}, Line: "Fetches levels."}},
				Trace:      []string{},
			},
		},
		Joints: []atlas.Joint{{
			ID: "j1", From: atlas.Endpoint{TargetID: web.Target.ID, BoundaryID: "b-out"}, To: atlas.Endpoint{TargetID: svc.Target.ID, BoundaryID: "b-in"},
			Value: "GET /api/levels", Same: true, Label: "reads levels over HTTP",
		}},
		Diagnostics: []atlas.Diagnostic{},
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{svc.Target.ID: svc, web.Target.ID: web}, value)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 2 {
		t.Fatalf("indexes: %d", len(indexes))
	}
	byTarget := make(map[string]Index)
	for _, index := range indexes {
		byTarget[index.Target.Name] = index
	}
	svcIndex := byTarget["svc"]
	if len(svcIndex.Groups) != 2 || len(svcIndex.Containers) != 1 || len(svcIndex.Connections) != 1 {
		t.Fatalf("svc: groups %d containers %d connections %d", len(svcIndex.Groups), len(svcIndex.Containers), len(svcIndex.Connections))
	}
	lanes := make(map[string]Lane)
	for _, group := range svcIndex.Groups {
		lanes[group.Title] = group.Lane
		if len(group.MemberSubjectIDs) != 1 {
			t.Fatalf("group %s members: %v", group.Title, group.MemberSubjectIDs)
		}
	}
	if lanes["HTTP handlers"] != LaneTriggers || lanes["Domain"] != LaneCore {
		t.Fatalf("lanes: %v", lanes)
	}
	if svcIndex.Containers[0].Title != "Serving" || len(svcIndex.Containers[0].GroupIDs) != 2 {
		t.Fatalf("container: %+v", svcIndex.Containers[0])
	}
	if svcIndex.Connections[0].Label != "The handlers hand requests to the domain." {
		t.Fatalf("arrow: %+v", svcIndex.Connections[0])
	}
	webIndex := byTarget["web"]
	if len(webIndex.Connections) != 1 || webIndex.Connections[0].To.TargetID != svc.Target.ID ||
		webIndex.Connections[0].SemanticKind != "reads_levels_over_http" || webIndex.Groups[0].Lane != LaneDependencies {
		t.Fatalf("web: %+v", webIndex.Connections)
	}
	// The projection is stable: the same atlas seals to the same digests.
	again, err := ProjectAtlas(map[string]programindex.Index{svc.Target.ID: svc, web.Target.ID: web}, value)
	if err != nil {
		t.Fatal(err)
	}
	for i := range indexes {
		if indexes[i].SHA256 != again[i].SHA256 {
			t.Fatal("projection is not deterministic")
		}
	}
}
