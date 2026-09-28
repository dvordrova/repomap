package groupindex

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// declared adds an input whose handler is not established: an option or a
// row the declaring code's call wrote at line, of kind, declared by that
// code and, when on is not zero, on the object the call at that line made.
func (g *reachGraphTest) declared(name, kind, by string, line, on int) {
	operation := Operation{ID: "o" + strconv.Itoa(len(g.index.Operations)+1), Kind: kind, Name: name, HandlerUnknown: true, DeclaredBy: g.ids[by],
		Location: programindex.Location{Path: "x.c", Line: line, Column: 3}}
	if on > 0 {
		operation.DeclaredOn = &DeclaredOn{Location: programindex.Location{Path: "x.c", Line: on, Column: 3}, Text: "parser " + strconv.Itoa(on)}
	}
	g.index.Operations = append(g.index.Operations, operation)
}

func (g *reachGraphTest) operationNames(ids []string) []string {
	names := map[string]string{}
	for _, operation := range g.index.Operations {
		names[operation.ID] = operation.Name
	}
	var result []string
	for _, id := range ids {
		result = append(result, names[id])
	}
	return result
}

func (g *reachGraphTest) subjectNames(ids []string) []string {
	var result []string
	for _, id := range ids {
		result = append(result, g.names[id])
	}
	return result
}

// The options one function compares its arguments with are one catalogue
// of their kind, in source order: redis-cli's parseOptions. A setting the
// same function reads is another kind and another catalogue. The catalogue
// lists where the declaring code runs from and the data it reads with every
// other function reading it, never where an input takes effect.
func TestHandlerlessInputsOfOneDeclarationAreOneCatalogue(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "parseOptions", "sendCommand", "repl")
	g.declare(programindex.ObjectVariable, "config")
	g.call("main", "parseOptions", "repl")
	g.call("repl", "sendCommand")
	g.relate(programindex.RelationReads, programindex.ResolutionExact, "parseOptions", "config")
	g.relate(programindex.RelationReads, programindex.ResolutionExact, "sendCommand", "config")
	g.declared("--raw", "command", "parseOptions", 30, 0)
	g.declared("port", "setting", "parseOptions", 20, 0)
	g.declared("-h", "command", "parseOptions", 10, 0)
	g.seed("main")
	g.derive()
	if len(g.index.Catalogues) != 2 {
		t.Fatalf("catalogues: %+v", g.index.Catalogues)
	}
	commands, settings := g.index.Catalogues[0], g.index.Catalogues[1]
	if got := g.operationNames(commands.OperationIDs); commands.Kind != "command" || !reflect.DeepEqual(got, []string{"-h", "--raw"}) {
		t.Fatalf("the command catalogue holds %v (%s)", got, commands.Kind)
	}
	if got := g.operationNames(settings.OperationIDs); settings.Kind != "setting" || !reflect.DeepEqual(got, []string{"port"}) {
		t.Fatalf("the setting catalogue holds %v (%s)", got, settings.Kind)
	}
	if g.names[commands.DeclaredBy] != "parseOptions" || commands.On != nil {
		t.Fatalf("the catalogue is declared by %q on %+v", g.names[commands.DeclaredBy], commands.On)
	}
	if got := g.edges(commands.Calls); !reflect.DeepEqual(got, []string{"main→parseOptions"}) {
		t.Fatalf("the declaring code is called from %v", got)
	}
	if len(commands.Uses) != 1 || g.names[commands.Uses[0].SubjectID] != "config" || !reflect.DeepEqual(g.subjectNames(commands.Uses[0].Users), []string{"sendCommand"}) {
		t.Fatalf("the declaring code also uses %+v", commands.Uses)
	}
}

// Inputs are grouped by the object their calls are made on when the facts
// name one: two parsers made in one function are two catalogues, and one
// parser used from two functions is one, declared by no one function. The
// input declared at the call that made the object is what its members are
// declared on.
func TestTwoObjectsInOneFunctionAreTwoCatalogues(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "buildParser", "addCommon")
	g.call("main", "buildParser")
	g.call("buildParser", "addCommon")
	g.declared("init", "command", "buildParser", 10, 0)
	g.declared("--force", "command", "buildParser", 11, 10)
	g.declared("--verbose", "command", "buildParser", 13, 12)
	g.declared("--quiet", "command", "addCommon", 20, 10)
	g.derive()
	var on10, on12 *Catalogue
	for position := range g.index.Catalogues {
		catalogue := &g.index.Catalogues[position]
		switch {
		case catalogue.On != nil && catalogue.On.Location.Line == 10:
			on10 = catalogue
		case catalogue.On != nil && catalogue.On.Location.Line == 12:
			on12 = catalogue
		}
	}
	if len(g.index.Catalogues) != 3 || on10 == nil || on12 == nil {
		t.Fatalf("catalogues: %+v", g.index.Catalogues)
	}
	if got := g.operationNames(on10.OperationIDs); !reflect.DeepEqual(got, []string{"--force", "--quiet"}) || on10.DeclaredBy != "" {
		t.Fatalf("the object made at line 10 holds %v, declared by %q", got, g.names[on10.DeclaredBy])
	}
	if got := g.operationNames([]string{on10.OnOperationID}); !reflect.DeepEqual(got, []string{"init"}) {
		t.Fatalf("the object made at line 10 is the input %v", got)
	}
	if got := g.operationNames(on12.OperationIDs); !reflect.DeepEqual(got, []string{"--verbose"}) || g.names[on12.DeclaredBy] != "buildParser" || on12.OnOperationID != "" {
		t.Fatalf("the object made at line 12 holds %v, declared by %q, on %q", got, g.names[on12.DeclaredBy], on12.OnOperationID)
	}
}

// A table's rows are one catalogue declared by the table: it is looked up
// where code reads it, and each reading function lists the calls into it.
func TestATableOfInputsIsLookedUpWhereItIsRead(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "cliSendCommand", "lookupCommand")
	g.declare(programindex.ObjectVariable, "cmdTable")
	g.call("main", "cliSendCommand", "lookupCommand")
	g.call("cliSendCommand", "lookupCommand")
	g.relate(programindex.RelationReads, programindex.ResolutionExact, "lookupCommand", "cmdTable")
	g.declared("get", "request", "cmdTable", 5, 0)
	g.declared("set", "request", "cmdTable", 6, 0)
	g.seed("main")
	g.derive()
	if len(g.index.Catalogues) != 1 {
		t.Fatalf("catalogues: %+v", g.index.Catalogues)
	}
	catalogue := g.index.Catalogues[0]
	if len(catalogue.Readers) != 1 || g.names[catalogue.Readers[0].SubjectID] != "lookupCommand" || len(catalogue.Calls) != 0 || len(catalogue.Uses) != 0 {
		t.Fatalf("the table's catalogue: %+v", catalogue)
	}
	if got := g.edges(catalogue.Readers[0].Calls); !reflect.DeepEqual(got, []string{"cliSendCommand→lookupCommand", "main→lookupCommand"}) {
		t.Fatalf("lookupCommand is called from %v", got)
	}
	// The launch finds the rows where it first reads the table.
	for _, function := range g.index.Launch.Functions {
		if found := g.operationNames(function.Found); len(found) > 0 && (g.names[function.SubjectID] != "lookupCommand" || !reflect.DeepEqual(found, []string{"get", "set"})) {
			t.Fatalf("%s holds %v", g.names[function.SubjectID], found)
		}
	}
}

// The launch walks from the seeds and from the code the language runs at
// load, by the reach rule, and says what each function it reaches holds:
// the inputs it declares, the calls the reading left unsure, the calls the
// code cannot follow, or nothing. It never walks into another input's
// handler through alternatives and hides no input: one declared by code it
// does not reach is still an input and a catalogue.
func TestTheWalkNeverHidesAnInput(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "parseOptions", "dispatch", "getCommand", "setCommand", "closed", "unsure", "unreached")
	g.call("main", "parseOptions", "dispatch", "closed", "unsure")
	g.dispatch("dispatch", "getCommand", "setCommand")
	g.index.Unresolved = []UnresolvedCall{{RelationID: "e99", FromSubjectID: g.ids["closed"], Location: &programindex.Location{Path: "x.c", Line: 70, Column: 2}}}
	g.input("get", "getCommand")
	g.input("set", "setCommand")
	g.declared("--raw", "command", "parseOptions", 40, 0)
	g.declared("--late", "command", "unreached", 41, 0)
	g.index.Unsure = []UnsureCall{{SubjectID: g.ids["unsure"], Location: programindex.Location{Path: "x.c", Line: 50, Column: 2}, Symbol: "strcmp", Reason: "undecided"}}
	g.seed("main")
	g.derive()
	outcomes := map[string]string{}
	for _, function := range g.index.Launch.Functions {
		outcomes[g.names[function.SubjectID]] = function.Outcome()
	}
	want := map[string]string{"main": "nothing", "parseOptions": "found", "dispatch": "nothing", "closed": "closed", "unsure": "unsure"}
	if !reflect.DeepEqual(outcomes, want) {
		t.Fatalf("the launch reaches %v, want %v", outcomes, want)
	}
	if !reflect.DeepEqual(g.subjectNames(g.index.Launch.Roots), []string{"main"}) {
		t.Fatalf("roots: %v", g.subjectNames(g.index.Launch.Roots))
	}
	late := false
	for _, catalogue := range g.index.Catalogues {
		late = late || slices.Equal(g.operationNames(catalogue.OperationIDs), []string{"--late"})
	}
	if !late || len(g.index.Launch.Nested) != 0 {
		t.Fatalf("an input the launch does not reach is hidden: catalogues %+v, nested %v", g.index.Catalogues, g.index.Launch.Nested)
	}
}

// The code a language runs at load is a root beside the seeds: a Go init
// function and a package variable whose initializer calls, a module body
// in Python, JS and Clojure. A C file scope runs nothing at load.
func TestLoadTimeCodeIsALaunchRoot(t *testing.T) {
	for _, test := range []struct {
		language string
		want     []string
	}{
		{"go", []string{"main", "init", "module", "registry"}},
		{"python", []string{"main", "module"}},
		{"c", []string{"main"}},
	} {
		g := newReachGraphTest()
		g.index.Target.Language = test.language
		g.functions("main", "init", "register", "idle")
		g.declare(programindex.ObjectModule, "module")
		g.declare(programindex.ObjectVariable, "registry", "table")
		g.call("registry", "register")
		g.call("module", "register")
		g.seed("main")
		g.derive()
		if got := g.subjectNames(g.index.Launch.Roots); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s roots: %v, want %v", test.language, got, test.want)
		}
	}
}

// Words only an input's handler checks are that input's sub-arguments
// (SORT's asc): listed in its reach, never tiles. Words the launch reaches
// too stay inputs of their own.
func TestAnEntryInAHandlersReachIsItsSubArgument(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "parseOptions", "sortCommand", "parseSortArgs")
	g.call("main", "parseOptions")
	g.call("sortCommand", "parseSortArgs")
	g.call("parseOptions", "parseSortArgs")
	g.functions("limitCommand", "parseLimit")
	g.call("limitCommand", "parseLimit")
	g.input("sort", "sortCommand")
	g.input("limit", "limitCommand")
	g.declared("asc", "command", "parseSortArgs", 60, 0)
	g.declared("count", "command", "parseLimit", 61, 0)
	g.seed("main")
	g.derive()
	if got := g.operationNames(g.reachOf("sort").SubArguments); len(got) != 0 {
		t.Fatalf("a word the launch reaches is sort's sub-argument: %v", got)
	}
	if got := g.operationNames(g.reachOf("limit").SubArguments); !reflect.DeepEqual(got, []string{"count"}) {
		t.Fatalf("limit's sub-arguments: %v", got)
	}
	nested := []string{}
	for id := range g.index.Launch.Nested {
		nested = append(nested, id)
	}
	if got := g.operationNames(nested); !reflect.DeepEqual(got, []string{"count"}) {
		t.Fatalf("nested: %v", got)
	}
}

// A request dispatched at a site arrives from every input whose reach holds
// the site's declaration and is not dispatched there, and from every input
// that hands over a callable no input handles whose walk holds it
// (acceptHandler registers readQueryFromClient, which reaches call). A
// hand-over into an input's handler is that input's, no hop: cron hands
// timerHandler over, and the timer input is listed through its own reach. A call into
// the declaration from code only the launch reaches leaves other ways not
// established.
func TestADispatchSiteNamesTheOuterInputsReachingIt(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "loadAppendOnlyFile", "acceptHandler", "createClient", "readQueryFromClient", "processInputBuffer", "call",
		"getCommand", "setCommand", "cronHandler", "flushSlaves", "timerHandler")
	g.call("main", "loadAppendOnlyFile")
	g.call("loadAppendOnlyFile", "call")
	g.call("acceptHandler", "createClient")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "createClient", "readQueryFromClient")
	g.call("readQueryFromClient", "processInputBuffer")
	g.call("processInputBuffer", "call")
	g.dispatch("call", "getCommand", "setCommand")
	g.call("cronHandler", "flushSlaves")
	g.call("flushSlaves", "call")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "cronHandler", "timerHandler")
	g.call("timerHandler", "flushSlaves")
	g.input("accept", "acceptHandler")
	g.input("cron", "cronHandler")
	g.input("timer", "timerHandler")
	g.input("get", "getCommand")
	g.input("set", "setCommand")
	g.seed("main")
	g.derive()
	var site *DispatchSite
	for position := range g.index.Dispatch {
		if g.names[g.index.Dispatch[position].FromSubjectID] == "call" {
			site = &g.index.Dispatch[position]
		}
	}
	if site == nil {
		t.Fatalf("no dispatch site at call: %+v", g.index.Dispatch)
	}
	type outer struct {
		input, registered string
		registering, edges []string
	}
	var got []outer
	for _, input := range site.Outer {
		row := outer{input: g.operationNames([]string{input.OperationID})[0], registered: g.names[input.Registered], registering: g.edges(input.Registering), edges: g.edges(input.Edges)}
		if input.HandOver >= 0 {
			row.registered += " via " + g.edge(input.HandOver)
		}
		got = append(got, row)
	}
	want := []outer{
		{input: "cron", edges: []string{"cronHandler→flushSlaves", "flushSlaves→call"}},
		{input: "timer", edges: []string{"flushSlaves→call", "timerHandler→flushSlaves"}},
		{input: "accept", registered: "readQueryFromClient via createClient→readQueryFromClient", registering: []string{"acceptHandler→createClient"},
			edges: []string{"processInputBuffer→call", "readQueryFromClient→processInputBuffer"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outer inputs of call:\n%+v\nwant\n%+v", got, want)
	}
	if !site.Unexplained {
		t.Fatal("the launch-only call from loadAppendOnlyFile is read as explained")
	}
}

// A row of a client's table of commands keeps the server's input its peer
// joint names, even when that boundary's operation was folded into another:
// the server registers its handler twice under one name, and the second
// site's boundary stands for the first's operation. A row names no input
// the joint does not name, and no row draws a connection.
func TestARowKeepsItsPeerThroughAFoldedBoundary(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 1}
	}
	program := func(name string, objects ...programindex.ObjectInput) programindex.Index {
		input := programindex.Input{ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
			Target: programindex.TargetInput{Language: "c", Kind: "executable", Name: name, Selector: name,
				Sources: []programindex.TargetSource{{FileRef: "f1", Path: objects[0].Location.Path}}, AnchorFileRef: "f1"},
			Objects: objects, Relations: []programindex.RelationInput{}, Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects)}}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		return index
	}
	cli := program("cli", programindex.ObjectInput{SourceRef: "table", Kind: programindex.ObjectVariable, Name: "cmdTable", Visibility: programindex.VisibilityInternal, Location: at("cli.c", 3)})
	srv := program("srv", programindex.ObjectInput{SourceRef: "get", Kind: programindex.ObjectFunction, Name: "getCommand", Visibility: programindex.VisibilityInternal, Location: at("srv.c", 9)})
	rebound := rebindTestTargets(t, cli, srv)
	cli, srv = rebound[0], rebound[1]
	box := func(p programindex.Index, path string) atlas.Box {
		return atlas.Box{ID: "p1", Dir: "p1", Title: path, Line: path + ".", Side: atlas.SideMid, Open: true, MemberIDs: []string{p.Objects[0].ID},
			Files: []atlas.File{{Path: path, Line: "File.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{{ID: "s1", ObjectID: p.Target.ID + "." + p.Objects[0].ID, Name: p.Objects[0].Name, Kind: "function", LineNo: p.Objects[0].Location.Line, Column: 1}}}}}
	}
	row := func(id, word string, line int) atlas.Boundary {
		return atlas.Boundary{ID: id, BoxID: "p1", ObjectID: cli.Target.ID + "." + cli.Objects[0].ID, Path: "cli.c", LineNo: line, Column: 5, Caller: "cmdTable",
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryCommand, Values: []string{word}, Name: word, Source: "model", HandlerUnknown: true}
	}
	handler := func(id string, line int) atlas.Boundary {
		return atlas.Boundary{ID: id, BoxID: "p1", ObjectID: srv.Target.ID + "." + srv.Objects[0].ID, Path: "srv.c", LineNo: line, Column: 5, Caller: "main",
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest, Values: []string{"get"}, Name: "get", Source: "fact"}
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "x", Revision: "abc", Diagnostics: []atlas.Diagnostic{},
		Targets: []atlas.Target{
			{ID: cli.Target.ID, Language: "c", Kind: "executable", Name: "cli", Root: ".", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
				Boxes: []atlas.Box{box(cli, "cli.c")}, Boundaries: []atlas.Boundary{row("b-get", "get", 4), row("b-del", "del", 5)}},
			{ID: srv.Target.ID, Language: "c", Kind: "executable", Name: "srv", Root: ".", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
				Boxes: []atlas.Box{box(srv, "srv.c")}, Boundaries: []atlas.Boundary{handler("b-first", 20), handler("b-second", 30)}},
		},
		// The peers answer named the second registration for get and
		// nothing for del.
		Joints: []atlas.Joint{{ID: "j1", From: atlas.Endpoint{TargetID: cli.Target.ID, BoundaryID: "b-get"}, To: atlas.Endpoint{TargetID: srv.Target.ID, BoundaryID: "b-second"},
			Value: "get", Same: true, Label: "sends get", Possible: true, SourceKind: "catalogue"}},
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{cli.Target.ID: cli, srv.Target.ID: srv}, value)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Index{}
	for _, index := range indexes {
		byName[index.Target.Name] = index
	}
	server := byName["srv"]
	if len(server.Operations) != 1 {
		t.Fatalf("the server's get is %d operations: %+v", len(server.Operations), server.Operations)
	}
	sends := map[string][]PeerInput{}
	for _, operation := range byName["cli"].Operations {
		sends[operation.Name] = operation.Sends
	}
	if want := []PeerInput{{TargetID: srv.Target.ID, OperationID: server.Operations[0].ID, Label: "sends get"}}; !reflect.DeepEqual(sends["get"], want) || len(sends["del"]) != 0 {
		t.Fatalf("rows send %+v", sends)
	}
	for _, index := range indexes {
		for _, connection := range index.Connections {
			t.Fatalf("a row draws a connection: %+v", connection)
		}
	}
}
