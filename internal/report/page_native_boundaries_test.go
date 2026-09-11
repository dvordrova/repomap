package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestSharedNativeRoutesReachEachCatalogueWithoutListenerOrForeignFactIDs(t *testing.T) {
	programs := map[string]programindex.Index{}
	var targets []reading.TargetMeta
	var origins []atlas.BoundaryOrigin
	layer := &facts.Result{}
	var targetIDs []string
	var representative programindex.Object
	for i, name := range []string{"main", "shared"} {
		program, err := programindex.New(programindex.Input{ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
			Target:  programindex.TargetInput{Language: "go", Kind: "executable", Name: name, Selector: name, AnchorFileRef: "file", Sources: []programindex.TargetSource{{FileRef: "file", Path: "api.go"}}},
			Objects: []programindex.ObjectInput{{SourceRef: "handler", Kind: programindex.ObjectFunction, Name: "Handler", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "api.go", Line: 3, Column: 1}}}, Relations: []programindex.RelationInput{}, Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 1}})
		if err != nil {
			t.Fatal(err)
		}
		programs[program.Target.ID] = program
		targetIDs = append(targetIDs, program.Target.ID)
		targets = append(targets, reading.TargetMeta{ID: program.Target.ID, Language: "go", Kind: "executable", Name: name, Root: ".", SelectedRole: atlas.RoleProduct})
		origins = append(origins, atlas.BoundaryOrigin{TargetID: program.Target.ID, FactID: fmt.Sprintf("route-%d", i), ObjectID: program.Objects[0].ID})
		layer.Facts = append(layer.Facts, facts.Fact{ID: fmt.Sprintf("route-%d", i), Kind: facts.KindHTTPRoute, TargetID: name, Method: "ANY", Path: "/v1/update", Anchor: &facts.Anchor{Path: "api.go", Line: 10, Column: 4}})
		if i == 0 {
			representative = program.Objects[0]
		}
	}
	graph := atlas.Graph{Version: atlas.GraphVersion, Revision: "native-test", Seeds: []string{atlas.FileID("api.go")}, Edges: []atlas.Edge{}, Places: []atlas.Place{
		{ID: atlas.DirectoryID("."), Kind: atlas.PlaceDirectory, Path: ".", TargetIDs: targetIDs, Given: "API", Directory: &atlas.DirectoryFacts{Files: []string{"api.go"}, Dirs: []string{}, FileCount: 1, TopBox: true}},
		{ID: atlas.FileID("api.go"), Kind: atlas.PlaceFile, Path: "api.go", Parent: atlas.DirectoryID("."), TargetIDs: targetIDs, Given: "API handler", File: &atlas.FileFacts{Decls: []atlas.Decl{{ObjectID: representative.ID, Name: "Handler", Kind: "function", LineNo: 3, Column: 1}}}},
		{ID: atlas.SymbolID("api.go", 3, "Handler"), Kind: atlas.PlaceSymbol, Path: "api.go", LineNo: 3, Column: 1, Parent: atlas.FileID("api.go"), TargetIDs: targetIDs, Given: "Handler", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: representative.ID, Name: "Handler", Kind: "function", LineNo: 3, Column: 1}}},
		{ID: "bnd:route", Kind: atlas.PlaceBoundary, Path: "api.go", LineNo: 10, Column: 4, Parent: atlas.FileID("api.go"), TargetIDs: targetIDs, Given: "ANY /v1/update", Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: origins, ObjectID: representative.ID, SubjectID: atlas.SymbolID("api.go", 3, "Handler"), Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryHTTPServer, Method: "ANY", Values: []string{"/v1/update"}}},
	}}
	listener := graph.Places[3]
	listener.ID = "bnd:listener"
	listener.LineNo = 20
	listener.Given = "listen :8080"
	b := *listener.Boundary
	listener.Boundary = &b
	b.GivenKind = atlas.BoundaryListenAddress
	b.Method = ""
	b.Values = []string{":8080"}
	b.Origins = append([]atlas.BoundaryOrigin(nil), origins...)
	for i := range b.Origins {
		b.Origins[i].FactID = fmt.Sprintf("listener-%d", i)
	}
	graph.Places = append(graph.Places, listener)
	atlas.SortPlaces(graph.Places)
	raw, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = atlas.DecodeGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reading.Read(t.Context(), reading.Options{Graph: graph, Targets: targets, Repository: "fixture", Revision: graph.Revision, OwnerRunDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(programs, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	builder := pageBuilder{data: &ReportData{Facts: layer}, indexes: indexes, links: pageLinks{sourceIDs: map[string]string{"api.go": "source"}}}
	for _, target := range targets {
		section := &pageSection{ID: target.Name, programTargetID: target.ID, factsTargetID: target.Name, FactsAvailable: true}
		builder.fillSectionOperations(section)
		if len(section.Requests) != 0 || len(section.Activities) != 0 || len(section.RouteGroups) != 1 || section.RouteGroups[0].Paths != 1 {
			t.Fatalf("listener or foreign fact inflated %s catalogue: %+v", target.Name, section)
		}
		paths := section.RouteGroups[0].Rows[0].Paths
		if len(paths) != 1 || len(paths[0].OperationHrefs) != 1 {
			t.Fatalf("native operation lost its exact route membership for %s: %+v", target.Name, paths)
		}
	}
	for _, target := range result.Atlas.Targets {
		if len(target.Boundaries) != 2 {
			t.Fatalf("listener was removed from atlas facts: %+v", target.Boundaries)
		}
	}
}
