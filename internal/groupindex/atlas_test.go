package groupindex

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestAtlasInterpretationRebindsTheSameDeclarationAcrossTargets(t *testing.T) {
	a := atlasTestProgram(t, "library", "pkg/work.go")
	b := atlasTestProgram(t, "executable", "pkg/work.go")
	rebound := rebindTestTargets(t, a, b)
	a, b = rebound[0], rebound[1]
	if a.Objects[0].ID != b.Objects[0].ID {
		t.Fatal("the same target-local ordinal should be reusable across qualified targets")
	}
	makeTarget := func(p programindex.Index) atlas.Target {
		return atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: "pkg", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}, Boxes: []atlas.Box{{ID: "pkg", Dir: "pkg", Title: "Work", Line: "Does work.", Side: atlas.SideMid, Keys: []atlas.Key{}, Files: []atlas.File{{Path: "pkg/work.go", Line: "Work.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{{ID: "symbol", ObjectID: p.Target.ID + "." + p.Objects[0].ID, Name: "FA", Kind: "function", LineNo: 3, Column: 1, Line: "Restores a snapshot.", Alias: "snapshot restorer", Activation: "command", Operation: "snapshot restore", OperationSummary: "Restores a data directory from a saved snapshot.", Key: true}}}}}}}
	}
	result, err := ProjectAtlas(map[string]programindex.Index{a.Target.ID: a, b.Target.ID: b}, atlas.Atlas{Version: atlas.Version, Repository: "x", Revision: "abc", Targets: []atlas.Target{makeTarget(a), makeTarget(b)}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	for i, index := range result {
		if len(index.Operations) != 1 || index.Operations[0].Name != "snapshot restore" {
			t.Fatalf("operation lost in %s: %+v", index.Target.Name, index.Operations)
		}
		want := []programindex.Index{a, b}[i].Objects[0].ID
		if index.Operations[0].SubjectID != want || index.Subjects[0].Interpretation == nil {
			t.Fatalf("operation was not rebound to %s", want)
		}
		if index.Subjects[0].Interpretation.Alias != "snapshot restorer" || index.Subjects[0].Object.Name != "FA" || index.Subjects[0].Object.Location.Path != "pkg/work.go" || index.Subjects[0].Object.Location.Line != 3 || index.Subjects[0].Object.Location.Column != 1 {
			t.Fatalf("alias changed native identity or source location: %+v", index.Subjects[0])
		}
		copy := index.Snapshot()
		copy.Subjects[0].Interpretation.Line = "changed"
		copy.Subjects[0].Interpretation.Alias = "changed alias"
		if index.Subjects[0].Interpretation.Line == "changed" || index.Subjects[0].Interpretation.Alias == "changed alias" {
			t.Fatal("snapshot shares interpretation")
		}
	}
}

// A declaration linked into two programs runs in one of them: the program
// whose index proves it never runs the declaration publishes no work of it.
func TestAtlasOperationStaysWithTheProgramThatRunsItsDeclaration(t *testing.T) {
	never, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "c", Kind: "executable", Name: "client", Selector: "client",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "pkg/work.go"}}, AnchorFileRef: "f1"},
		Objects: []programindex.ObjectInput{{SourceRef: "oa", Kind: programindex.ObjectFunction, Name: "FA", Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: "pkg/work.go", Line: 3, Column: 1}, Unreachable: true}},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebound := rebindTestTargets(t, atlasTestProgram(t, "server", "pkg/work.go"), never)
	programs := map[string]programindex.Index{}
	var targets []atlas.Target
	for _, p := range rebound {
		programs[p.Target.ID] = p
		targets = append(targets, atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "c", Kind: "executable", Root: "pkg", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{},
			Boxes: []atlas.Box{{ID: "pkg", Dir: "pkg", Title: "Work", Line: "Does work.", Side: atlas.SideMid, Keys: []atlas.Key{}, Files: []atlas.File{{Path: "pkg/work.go", Line: "Work.", Source: atlas.SourceModel,
				Symbols: []atlas.Symbol{{ID: "symbol", ObjectID: p.Target.ID + "." + p.Objects[0].ID, Name: "FA", Kind: "function", LineNo: 3, Column: 1, Activation: "continuous", Operation: "flush", OperationSummary: "Flushes the log every second."}}}}}}})
	}
	result, err := ProjectAtlas(programs, atlas.Atlas{Version: atlas.Version, Repository: "x", Revision: "abc", Targets: targets, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	operations := map[string]int{}
	for _, index := range result {
		operations[index.Target.Name] = len(index.Operations)
	}
	if want := map[string]int{"server": 1, "client": 0}; !reflect.DeepEqual(operations, want) {
		t.Fatalf("operations by program = %v, want %v", operations, want)
	}
}

func TestObservedRoutesReplaceTheDeclarationOperationAndKeepAliases(t *testing.T) {
	p := atlasTestProgram(t, "server", "api/handler.go")
	target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: "api", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boxes: []atlas.Box{{ID: "api", Dir: "api", Title: "API", Line: "Answers requests.", Side: atlas.SideIn, Keys: []atlas.Key{}, MemberIDs: []string{p.Objects[0].ID}, Files: []atlas.File{{Path: "api/handler.go", Line: "Handles requests.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{{ID: "handler", ObjectID: p.Objects[0].ID, Name: "FA", Kind: "function", LineNo: 3, Column: 1, Line: "Returns status.", Activation: "request", Operation: "get status"}}}}}}}
	for i, route := range []string{"/status", "/health"} {
		target.Boundaries = append(target.Boundaries, atlas.Boundary{ID: fmt.Sprintf("route%d", i), ObjectID: p.Objects[0].ID, BoxID: "api", Path: "api/handler.go", LineNo: 2 + i, Column: 1, Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest, Method: "GET", Values: []string{route}, Name: "GET " + route, Line: "Returns status.", FactID: "fact"})
	}
	target.Boundaries = append(target.Boundaries, atlas.Boundary{ID: "listener", ObjectID: p.Objects[0].ID, BoxID: "api", Path: "api/handler.go", LineNo: 9, Column: 1,
		Direction: atlas.DirectionIn, Kind: atlas.BoundaryListenAddress, Values: []string{":8080"}, Line: "Listens for HTTP connections.", FactID: "listen-fact"})

	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	operations := indexes[0].Operations
	if len(operations) != 2 {
		t.Fatalf("routes were duplicated or aliases lost: %+v", operations)
	}
	for _, operation := range operations {
		if operation.Source != "fact" || operation.FactID != "fact" || operation.SubjectID != p.Objects[0].ID || !strings.HasPrefix(operation.Name, "GET /") {
			t.Fatalf("route lost native binding: %+v", operation)
		}
	}
}

func TestAtlasProjectsIndependentInterpretationWithoutCaption(t *testing.T) {
	p := atlasTestProgram(t, "app", "app/work.go")
	for _, test := range []struct {
		name string
		want Interpretation
	}{
		{name: "uncaptioned non-key action", want: Interpretation{Activation: "interaction", Operation: "Edit code", OperationSummary: "Updates the program from the editor."}},
		{name: "uncaptioned background work", want: Interpretation{Activation: "continuous", Operation: "Animate field", OperationSummary: "Advances the displayed simulation."}},
		{name: "uncaptioned key", want: Interpretation{Key: true}},
		{name: "alias", want: Interpretation{Alias: "work runner"}},
		{name: "caption", want: Interpretation{Line: "Runs the work."}},
		{name: "no interpretation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := test.want
			symbol := atlas.Symbol{ID: "work", ObjectID: p.Objects[0].ID, Name: "FA", Kind: "function", LineNo: 3, Column: 1,
				Line: want.Line, Alias: want.Alias, Key: want.Key, Activation: want.Activation, Operation: want.Operation, OperationSummary: want.OperationSummary}
			target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: "app",
				Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{},
				Boxes: []atlas.Box{{ID: "app", Dir: "app", Title: "Work", Line: "Does work.", Side: atlas.SideMid, Keys: []atlas.Key{},
					Files: []atlas.File{{Path: "app/work.go", Line: "Work.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{symbol}}}}}}
			indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
			if err != nil {
				t.Fatal(err)
			}
			index := indexes[0]
			got := index.Subjects[0].Interpretation
			if want == (Interpretation{}) {
				if got != nil {
					t.Fatalf("invented an interpretation: %+v", got)
				}
			} else if got == nil || *got != want {
				t.Fatalf("independent interpretation lost: got %+v want %+v", got, want)
			}
			if want.Activation == "" {
				if len(index.Operations) != 0 {
					t.Fatalf("invented an operation: %+v", index.Operations)
				}
				return
			}
			if len(index.Operations) != 1 {
				t.Fatalf("uncaptioned operation lost: %+v", index.Operations)
			}
			op := index.Operations[0]
			if op.SubjectID != p.Objects[0].ID || op.Kind != want.Activation || op.Name != want.Operation || op.Summary != want.OperationSummary || op.Source != "model" || op.Location != *p.Objects[0].Location {
				t.Fatalf("operation changed its meaning or source: %+v", op)
			}
		})
	}
}

func TestSharedCodeLinksRemainBoundToTheirCompleteTarget(t *testing.T) {
	app := atlasTestProgram(t, "app", "cmd/app.go")
	shared := atlasTestProgram(t, "shared", "pkg/shared.go")
	rebound := rebindTestTargets(t, app, shared)
	app, shared = rebound[0], rebound[1]
	makeTarget := func(p programindex.Index, role string) atlas.Target {
		return atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Role: role, Root: ".", Zones: []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}}
	}
	value := atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{makeTarget(app, atlas.RoleProduct), makeTarget(shared, atlas.RoleSharedCode)}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	value.Targets[0].SharedCode = []string{shared.Target.ID}
	indexes, err := ProjectAtlas(map[string]programindex.Index{app.Target.ID: app, shared.Target.ID: shared}, value)
	if err != nil {
		t.Fatal(err)
	}
	var consumer Index
	for _, index := range indexes {
		if index.Target.ID == app.Target.ID {
			consumer = index
			copy := index.Snapshot()
			copy.SharedCode[0] = "changed"
			if index.SharedCode[0] != shared.Target.ID {
				t.Fatal("snapshot aliases shared code ownership")
			}
		}
	}
	if err := ValidateSet([]Index{consumer}); err == nil {
		t.Fatal("missing shared analysis accepted")
	}
	for _, ids := range [][]string{{"unknown"}, {app.Target.ID}, {shared.Target.ID, shared.Target.ID}} {
		value.Targets[0].SharedCode = ids
		if atlas.Validate(value) == nil {
			t.Fatalf("invalid shared code link accepted: %v", ids)
		}
	}
	value.Targets[0].SharedCode = []string{shared.Target.ID}
	value.Targets[1].Role = atlas.RoleTool
	if atlas.Validate(value) == nil {
		t.Fatal("tool promoted into shared code")
	}
}

func atlasTestProgram(t *testing.T, name string, files ...string) programindex.Index {
	t.Helper()
	return atlasTestProgramWith(t, name, nil, files...)
}

// atlasTestProgramWith is atlasTestProgram with native relations between its
// objects oa, ob, ... (one per file, in file order).
func atlasTestProgramWith(t *testing.T, name string, relations []programindex.RelationInput, files ...string) programindex.Index {
	t.Helper()
	if relations == nil {
		relations = []programindex.RelationInput{}
	}
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
		Relations: relations,
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func TestProjectAtlasMakesGroupsContainersAndConnections(t *testing.T) {
	// The handler calls the domain: a native relation the arrow's sentence
	// describes between the two parts.
	svc := atlasTestProgramWith(t, "svc", []programindex.RelationInput{{
		SourceRef: "handler-calls-domain", Kind: programindex.RelationCalls, FromRef: "oa", ToRefs: []string{"ob"},
		Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: &programindex.Location{Path: "svc/api/h.go", Line: 4, Column: 2},
		Witnesses: []programindex.Witness{{Kind: "call", Location: &programindex.Location{Path: "svc/api/h.go", Line: 4, Column: 2}}}, WitnessesObserved: 1,
	}}, "svc/api/h.go", "svc/core/c.go")
	web := atlasTestProgram(t, "web", "web/src/app.ts")
	rebound := rebindTestTargets(t, svc, web)
	svc, web = rebound[0], rebound[1]
	objectIn := func(p programindex.Index, path string) []string {
		for _, object := range p.Objects {
			if object.Location != nil && object.Location.Path == path {
				return []string{object.ID}
			}
		}
		t.Fatalf("no object in %s", path)
		return nil
	}
	value := atlas.Atlas{
		Version: atlas.Version, Repository: "x", Revision: "abc",
		Targets: []atlas.Target{
			{
				ID: svc.Target.ID, Language: "go", Kind: "executable", Name: "svc", Root: "svc",
				Zones: []atlas.Zone{{ID: "serving", Title: "Serving", Line: "Serves things.", BoxIDs: []string{"svc/api", "svc/core"}}},
				Boxes: []atlas.Box{
					{ID: "svc/api", Dir: "svc/api", Title: "HTTP handlers", Line: "Answers requests.", ZoneID: "serving", Side: atlas.SideIn, Open: true, MemberIDs: objectIn(svc, "svc/api/h.go"),
						Files: []atlas.File{{Path: "svc/api/h.go", Line: "Handler file.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}, Keys: []atlas.Key{}},
					{ID: "svc/core", Dir: "svc/core", Title: "Domain", Line: "Does the work.", ZoneID: "serving", Side: atlas.SideMid, Open: true, MemberIDs: objectIn(svc, "svc/core/c.go"),
						Files: []atlas.File{{Path: "svc/core/c.go", Line: "Core file.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}, Keys: []atlas.Key{}},
				},
				Arrows:     []atlas.Arrow{{ID: "x1", From: "svc/api", To: "svc/core", Calls: 3, Witnesses: []atlas.Witness{}, Sentence: "The handlers hand requests to the domain."}},
				Boundaries: []atlas.Boundary{{ID: "b-in", BoxID: "svc/api", Path: "svc/api/h.go", LineNo: 10, Caller: "FA", Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest, Values: []string{"/api/levels"}, Line: "Serves levels."}},
			},
			{
				ID: web.Target.ID, Language: "typescript", Kind: "package", Name: "web", Root: "web",
				Zones: []atlas.Zone{},
				Boxes: []atlas.Box{{ID: "web/src", Dir: "web/src", Title: "Client", Line: "Calls the API.", Side: atlas.SideOut, Open: true, MemberIDs: objectIn(web, "web/src/app.ts"),
					Files: []atlas.File{{Path: "web/src/app.ts", Line: "App.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}, Keys: []atlas.Key{}}},
				Arrows:     []atlas.Arrow{},
				Boundaries: []atlas.Boundary{{ID: "b-out", BoxID: "web/src", Path: "web/src/app.ts", LineNo: 5, Caller: "FA", Direction: atlas.DirectionOut, Kind: atlas.BoundaryClientRequest, Values: []string{"/api/levels"}, Line: "Fetches levels."}},
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
	loaded, err := ProjectAtlasFrom(value, func(id string) (programindex.Index, error) {
		if id == svc.Target.ID {
			return svc, nil
		}
		if id == web.Target.ID {
			return web, nil
		}
		return programindex.Index{}, fmt.Errorf("unknown target %s", id)
	})
	if err != nil || !reflect.DeepEqual(indexes, loaded) {
		t.Fatalf("individual target reads changed groups or cross-target connections: %v", err)
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
	if svcIndex.Connections[0].Summary != "The handlers hand requests to the domain." || svcIndex.Connections[0].FromSubjectID == "" {
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
	// Two inferred calls at one source line may name the same peer and label.
	// Their original boundary identities must survive instead of conflicting.
	other := value.Joints[0]
	other.ID = "j2"
	other.Value = "GET /api/other"
	value.Joints = append(value.Joints, other)
	withAliases, err := ProjectAtlas(map[string]programindex.Index{svc.Target.ID: svc, web.Target.ID: web}, value)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range withAliases {
		if index.Target.ID != web.Target.ID {
			continue
		}
		if len(index.Connections) != 2 || index.Connections[0].SourceID == index.Connections[1].SourceID {
			t.Fatalf("distinct inferred calls collapsed: %+v", index.Connections)
		}
	}
}

// An entry's operation is named by the words the model chose, as the atlas
// restored them, or by its handler: never by composing the fact's method and
// values, which would give every protocol HTTP's shape.
func TestEntryOperationsTakeTheChosenNameOrTheHandler(t *testing.T) {
	p := atlasTestProgram(t, "server", "api/handler.go")
	target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: "api", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boxes: []atlas.Box{{ID: "api", Dir: "api", Title: "API", Line: "Answers requests.", Side: atlas.SideIn, Keys: []atlas.Key{}, MemberIDs: []string{p.Objects[0].ID},
			Files: []atlas.File{{Path: "api/handler.go", Line: "Handles requests.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}}}}
	entry := func(id string, line int, name string, kind string, values ...string) atlas.Boundary {
		return atlas.Boundary{ID: id, ObjectID: p.Objects[0].ID, BoxID: "api", Path: "api/handler.go", LineNo: line, Column: 1, Caller: "FA",
			Direction: atlas.DirectionIn, Kind: kind, Method: "GET", Values: values, Name: name, Line: "Answers.", FactID: "fact-" + id}
	}
	target.Boundaries = []atlas.Boundary{
		entry("chosen", 4, "get", atlas.BoundaryRequest, "get", "kvCommand"),
		entry("unchosen", 5, "", atlas.BoundaryRequest, "/users"),
		entry("worker", 6, "", atlas.BoundaryContinuous, []string{}...),
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, operation := range indexes[0].Operations {
		got = append(got, operation.Kind+" "+operation.Name+" @"+strconv.Itoa(operation.Location.Line))
	}
	sort.Strings(got)
	if want := []string{"continuous FA @6", "request FA @5", "request get @4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operations = %v, want %v", got, want)
	}
}
