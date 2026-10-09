package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The scene's facts are read off the system map as the canvas reads the
// page: an input takes effect in its handler's part, else where its code
// takes it in, else in its program; a call tile stands for its system; a
// part reaching another program's input, and a call through an outside
// system another program's input serves, are runtime pairs of programs; a
// code use alone is not. An input reaches the parts and systems its saved
// path's relations start or end at.
func TestTheSceneSaysWhereEachInputTakesEffectAndWhoCallsEachSystem(t *testing.T) {
	handler := pageAnchor{Path: "api/routes.go", Line: 10, Open: sceneKey("api/routes.go", 10, 6)}
	view := &pageMap{Nodes: []pageMapNode{
		{ID: "system-component-t1", Owner: "t1", Branch: "component", ItemKind: "Component", Children: "t1-area-k1"},
		{ID: "t1-area-k1", Owner: "t1", Branch: "area", Children: "n-t1-g1 n-t1-g2 t1-o1"},
		{ID: "n-t1-g1", Owner: "t1"},
		{ID: "n-t1-g2", Owner: "t1"},
		{ID: "system-component-t2", Owner: "t2", Branch: "component", ItemKind: "Component", Children: "n-t2-g1"},
		{ID: "n-t2-g1", Owner: "t2"},
		{ID: "t1-o1", Owner: "t1", Activation: "request", InputOwner: "n-t1-g1", Handler: "serveUsers", HandlerSource: handler},
		{ID: "t1-o2", Owner: "t1", Activation: "setting"},
		{ID: "t1-o3", Owner: "t1", Activation: "background"},
		{ID: "t1-o4", Owner: "t1", Activation: "queue_consumer"},
		{ID: "t2-o1", Owner: "t2", Activation: "request"},
		{ID: "system-inputs-t1", Owner: "t1", Branch: "inputs", Children: "t1-o1 t1-o2 t1-o3 t1-o4"},
		{ID: "system-inputs-t2", Owner: "t2", Branch: "inputs", Children: "t2-o1"},
		{ID: "system-t1-out-b1", Owner: "t1", ItemKind: "External communication"},
		{ID: "system-t1-out-b1-destination", Owner: "t1", Branch: "communication", ItemKind: "External communication", Children: "system-t1-out-b1", DestinationKind: "database"},
		{ID: "system-t1-out-b2", Owner: "t1", ItemKind: "External communication"},
		{ID: "system-t1-out-b2-destination", Owner: "t1", Branch: "communication", ItemKind: "External communication", Children: "system-t1-out-b2", DestinationKind: "queue_producer"},
		{ID: "system-outside-t1", Owner: "t1", Branch: "outside", ItemKind: "External communication", Children: "system-t1-out-b1-destination system-t1-out-b2-destination"},
	}, Edges: []pageMapEdge{
		/* 0 */ {From: "t1-o2", To: "n-t1-g2", Scope: "operation", Label: "declared in", Operations: "t1-o2"},
		/* 1 */ {From: "t1-o3", To: "n-t1-g2", Scope: "operation", Label: "loop calls tick", FromSource: pageAnchor{Href: "x"}, Operations: "t1-o3 t1-o1"},
		/* 2 */ {From: "n-t1-g1", To: "system-t1-out-b1", Scope: "structure", Label: "Database", Operations: "t1-o1",
			Calls: []pageEdgeCall{{Label: "save calls Exec", Caller: sceneKey("store/db.go", 3, 1)}, {Label: "load calls Query", Caller: sceneKey("store/db.go", 9, 1)}}},
		/* 3 */ {From: "n-t1-g2", To: "system-t1-out-b2", Scope: "structure", Label: "Queue"},
		/* 4 */ {From: "system-t1-out-b2-destination", To: "t2-o1", Scope: "structure", Label: "connects_to", Operations: "t2-o1"},
		/* 5 */ {From: "n-t2-g1", To: "n-t1-g1", Scope: "structure", Label: "imports"},
		/* 6 */ {From: "t2-o1", To: "n-t2-g1", Scope: "operation", Label: "implemented in"},
		/* 7 */ {From: "n-t1-g1", To: "system-t1-out-b1", Scope: "structure", Label: "Database", Possible: true},
	}}
	got := sceneOf(view, sceneSourceOf)
	program1, program2 := "system-component-t1", "system-component-t2"
	want := &SceneFacts{
		Inputs: map[string]SceneInput{
			"t1-o1": {Kind: "request", Program: program1, Parts: []string{"n-t1-g1"}, Handled: true, Handler: &SceneSource{Path: "api/routes.go", Line: 10}},
			"t1-o2": {Kind: "setting", Program: program1, Parts: []string{"n-t1-g2"}},
			"t1-o3": {Kind: "continuous", Program: program1, Parts: []string{"n-t1-g2"}, Handled: true},
			"t1-o4": {Kind: "consumer", Program: program1, Parts: []string{}},
			"t2-o1": {Kind: "request", Program: program2, Parts: []string{"n-t2-g1"}, Handled: true},
		},
		Systems: map[string]SceneSystem{
			"system-t1-out-b1-destination": {Kind: "database", Parts: []string{"n-t1-g1"}, Programs: []string{program1}},
			"system-t1-out-b2-destination": {Kind: "other", Parts: []string{"n-t1-g2"}, Programs: []string{program1}},
		},
		Calls: map[string][]SceneCall{
			"n-t1-g1": {{System: "system-t1-out-b1-destination", Caller: &SceneSource{Path: "store/db.go", Line: 3}}, {System: "system-t1-out-b1-destination", Caller: &SceneSource{Path: "store/db.go", Line: 9}},
				// The possible arrow's call names no declaration making it.
				{System: "system-t1-out-b1-destination"}},
			"n-t1-g2": {{System: "system-t1-out-b2-destination"}},
		},
		ProgramPairs: []SceneProgramPair{
			{From: program1, To: program2, Runtime: true, Relations: []int{4}},
			{From: program2, To: program1, Runtime: false, Relations: []int{5}},
		},
		// In the page's order, whatever order a relation lists them in; a
		// system by its call record or itself; an input's own end is no
		// part.
		Reaching: map[string][]string{
			"n-t1-g1":                      {"t1-o1"},
			"n-t1-g2":                      {"t1-o1", "t1-o2", "t1-o3"},
			"system-t1-out-b1-destination": {"t1-o1"},
			"system-t1-out-b2-destination": {"t2-o1"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", " ")
		wantJSON, _ := json.MarshalIndent(want, "", " ")
		t.Fatalf("scene\n%s\nwant\n%s", gotJSON, wantJSON)
	}
}

// sceneFixture is one program: routes.go's handler takes GET /users in,
// db.go's save calls PostgreSQL.
func sceneFixture(t *testing.T) ReportData {
	t.Helper()
	paths := []string{"api/routes.go", "store/db.go"}
	objects := []programindex.ObjectInput{
		{SourceRef: "p1", Kind: programindex.ObjectFunction, Name: "serveUsers", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: paths[0], Line: 3, Column: 6}},
		{SourceRef: "p2", Kind: programindex.ObjectFunction, Name: "save", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: paths[1], Line: 5, Column: 6}},
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{ID: "t1", Language: "go", Kind: "executable", Name: "users", Selector: "users",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: paths[0]}, {FileRef: "f2", Path: paths[1]}}, AnchorFileRef: "f1"},
		Objects: objects, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects)},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := atlas.Target{ID: program.Target.ID, Language: "go", Kind: "executable", Name: "users", Root: ".",
		Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boundaries: []atlas.Boundary{
			{ID: "in1", ObjectID: program.Objects[0].ID, BoxID: "p1", Path: paths[0], LineNo: 8, Column: 2, Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest,
				Method: "GET", Values: []string{"/users"}, Name: "GET /users", Line: "Lists users.", FactID: "fact-in1"},
			{ID: "out1", ObjectID: program.Objects[1].ID, BoxID: "p2", Path: paths[1], LineNo: 7, Column: 3, Direction: atlas.DirectionOut, Kind: atlas.BoundaryDB,
				External: "Exec", Destination: "PostgreSQL", Values: []string{}, Line: "Saves a user.", FactID: "fact-out1"},
		}}
	for i, title := range []string{"Routes", "Storage"} {
		id := fmt.Sprintf("p%d", i+1)
		target.Boxes = append(target.Boxes, atlas.Box{ID: id, Dir: filepath.Dir(paths[i]), Title: title, Line: "Does " + title + ".", Side: atlas.SideMid, Open: true,
			MemberIDs: []string{program.Objects[i].ID}, Files: []atlas.File{{Path: paths[i], Line: title + ".", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}})
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{program.Target.ID: program},
		atlas.Atlas{Version: atlas.Version, Repository: "users", Revision: "abc", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := NewProgramPortfolio(program.Target.ID, []programindex.Index{program})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGroupGraphView(indexes, program.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := ReportData{FormatVersion: CurrentFormatVersion, RepoName: "users", CapturedRevision: strings.Repeat("a", 40),
		ProgramPortfolio: portfolio, GroupGraph: graph,
		TargetOutcomePortfolio: reportTargetOutcomeViewFixture(t, []TargetNavigationPage{{
			RunID: "20261001-120000-page-a1b2c3", ProgramTarget: program.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename,
		}}, program.Target.ID)}
	if err := collectOpenablePaths(&data); err != nil {
		t.Fatal(err)
	}
	return data
}

// The page shows the scene saved with the report as it was saved, and it
// names the page's own records and declarations whatever links the page is
// rendered with: none, a static host's, a host missing the files, a local
// server's. An input's handler and an outside call's caller are the place
// a declaration of their part carries on the page (its path and line), and
// read off the rendered page itself the facts are the saved ones.
func TestThePageShowsTheSavedSceneWhateverItsLinks(t *testing.T) {
	data := sceneFixture(t)
	scene, err := deriveScene(&data)
	if err != nil {
		t.Fatal(err)
	}
	data.Scene = scene
	links := map[string]func(*ReportData){
		"none": func(*ReportData) {},
		"GitHub": func(data *ReportData) {
			data.GitHubSourceLinks = &GitHubSourceLinks{RepositoryURL: "https://github.com/example/users", Revision: data.CapturedRevision}
		},
		"GitHub without the files": func(data *ReportData) {
			data.GitHubSourceLinks = &GitHubSourceLinks{RepositoryURL: "https://github.com/example/users", Revision: data.CapturedRevision}
			data.UnavailableSourcePaths = append([]string(nil), data.OpenablePaths...)
		},
		"served": func(data *ReportData) {
			data.SourceIDs = map[string]string{}
			for i, path := range data.OpenablePaths {
				data.SourceIDs[path] = fmt.Sprintf("%043d", i+1)
			}
		},
	}
	for name, link := range links {
		t.Run(name, func(t *testing.T) {
			rendered := data
			link(&rendered)
			options := reportSingleTargetRenderOptionsFixture(t, &rendered)
			page, err := RenderHTMLWithOptions(&rendered, options)
			if err != nil {
				t.Fatal(err)
			}
			raw := readEmbeddedJSON(t, page, "rm-scene")
			var shown scenePage
			if err := json.Unmarshal(raw, &shown); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(shown, scenePage(*scene)) {
				t.Fatalf("the page shows\n%s\nnot the saved scene %+v", raw, scene)
			}
			prepared, err := PreparePage(&rendered, options)
			if err != nil {
				t.Fatal(err)
			}
			systemMap := prepared.view.SystemMap()
			places := map[string]map[SceneSource]string{}
			titles := map[string]string{}
			for _, node := range systemMap.Nodes {
				titles[node.FullTitle] = node.ID
				if node.Symbols == "" {
					continue
				}
				var symbols []pageNodeSymbol
				if err := json.Unmarshal([]byte(node.Symbols), &symbols); err != nil {
					t.Fatal(err)
				}
				places[node.ID] = map[SceneSource]string{}
				for _, symbol := range symbols {
					places[node.ID][SceneSource{Path: symbol.Path, Line: symbol.Line}] = symbol.Name
				}
			}
			routes, storage := titles["Routes"], titles["Storage"]
			if len(shown.Inputs) != 1 || len(shown.Systems) != 1 {
				t.Fatalf("inputs %+v, systems %+v", shown.Inputs, shown.Systems)
			}
			for _, input := range shown.Inputs {
				if !input.Handled || !reflect.DeepEqual(input.Parts, []string{routes}) || input.Handler == nil || places[routes][*input.Handler] != "serveUsers" {
					t.Fatalf("GET /users takes effect in %+v, not serveUsers of %q %v", input, routes, places[routes])
				}
			}
			calls := shown.Calls[storage]
			if len(calls) != 1 || calls[0].Caller == nil || places[storage][*calls[0].Caller] != "save" ||
				shown.Systems[calls[0].System].Kind != "database" || !reflect.DeepEqual(shown.Systems[calls[0].System].Parts, []string{storage}) {
				t.Fatalf("Storage's calls %+v, systems %+v, declarations %v", calls, shown.Systems, places[storage])
			}
			// The page drawn with these links says the same where it says it.
			read := sceneOf(systemMap, func(key string) *SceneSource { return &SceneSource{Path: key} })
			for id, input := range read.Inputs {
				input.Handler = shown.Inputs[id].Handler
				read.Inputs[id] = input
			}
			for part, calls := range read.Calls {
				for i := range calls {
					calls[i].Caller = shown.Calls[part][0].Caller
				}
			}
			if !reflect.DeepEqual(scenePage(*read), shown) {
				t.Fatalf("read off the page %+v, saved %+v", read, shown)
			}
		})
	}
}

// Publication saves the scene in report.json, read when the report is
// assembled; rendering the saved run reads it back unchanged.
func TestPublicationSavesTheScene(t *testing.T) {
	runDir := t.TempDir()
	data := sceneFixture(t)
	data.ArtifactsDir, data.defaultProgramIndexArtifactFilename = runDir, "program-index.json"
	manifest := validRunManifestFixture(t)
	source, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(runDir, source, GenerateOptions{Data: &data, PublishHTML: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Scene *SceneFacts `json:"scene"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	want, err := deriveScene(&data)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Scene == nil || len(saved.Scene.Inputs) != 1 || len(saved.Scene.Systems) != 1 || !reflect.DeepEqual(saved.Scene, want) {
		t.Fatalf("saved scene %+v, want %+v", saved.Scene, want)
	}
	for _, input := range saved.Scene.Inputs {
		if input.Handler == nil || *input.Handler != (SceneSource{Path: "api/routes.go", Line: 3}) {
			t.Fatalf("the saved handler is not its declaration's place: %+v", input.Handler)
		}
	}
}
