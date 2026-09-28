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

// reachGraphTest builds a GroupsIndex by hand: declarations by name, the
// relations between them, parts, inputs and seeds. Derive needs no seal.
type reachGraphTest struct {
	index Index
	ids   map[string]string
	names map[string]string
}

func newReachGraphTest() *reachGraphTest {
	return &reachGraphTest{index: Index{Target: programindex.Target{ID: "t1"}}, ids: map[string]string{}, names: map[string]string{}}
}

func (g *reachGraphTest) declare(kind programindex.ObjectKind, names ...string) {
	for _, name := range names {
		id := "n" + strconv.Itoa(len(g.index.Subjects)+1)
		g.ids[name], g.names[id] = id, name
		g.index.Subjects = append(g.index.Subjects, Subject{ID: id, Kind: SubjectObject, Object: &ObjectFacts{Name: name, Kind: kind,
			Location: &programindex.Location{Path: "x.c", Line: len(g.index.Subjects) + 1, Column: 1}}})
	}
}

func (g *reachGraphTest) functions(names ...string) { g.declare(programindex.ObjectFunction, names...) }

// relate adds one relation from a declaration to its targets; its place in
// source is its line, so relations stand in the order they are added.
func (g *reachGraphTest) relate(kind programindex.RelationKind, resolution programindex.Resolution, from string, to ...string) {
	relation := "e" + strconv.Itoa(len(g.index.StructuralEdges)+1)
	for _, target := range to {
		g.index.StructuralEdges = append(g.index.StructuralEdges, StructuralEdge{FromSubjectID: g.ids[from], ToSubjectID: g.ids[target], Role: EdgeRelationTarget,
			RelationID: relation, RelationKind: kind, Resolution: resolution, Location: &programindex.Location{Path: "x.c", Line: 100 + len(g.index.StructuralEdges), Column: 2}})
	}
}

func (g *reachGraphTest) call(from string, to ...string) {
	for _, target := range to {
		g.relate(programindex.RelationCalls, programindex.ResolutionExact, from, target)
	}
}

func (g *reachGraphTest) dispatch(from string, to ...string) {
	g.relate(programindex.RelationCalls, programindex.ResolutionAlternatives, from, to...)
}

func (g *reachGraphTest) part(title string, members ...string) {
	var ids []string
	for _, member := range members {
		ids = append(ids, g.ids[member])
	}
	g.index.Groups = append(g.index.Groups, Group{ID: "g" + strconv.Itoa(len(g.index.Groups)+1), Title: title, MemberSubjectIDs: ids})
}

func (g *reachGraphTest) input(name, handler string) {
	g.index.Operations = append(g.index.Operations, Operation{ID: "o" + strconv.Itoa(len(g.index.Operations)+1), SubjectID: g.ids[handler], Kind: "command", Name: name})
}

func (g *reachGraphTest) seed(name string) {
	g.index.Target.Seeds = append(g.index.Target.Seeds, programindex.TargetSeed{ObjectID: g.ids[name]})
}

func (g *reachGraphTest) derive() *Index {
	Derive(&g.index)
	return &g.index
}

func (g *reachGraphTest) reachOf(name string) Reach {
	for position, operation := range g.index.Operations {
		if operation.Name == name {
			return g.index.Reach[position]
		}
	}
	return Reach{}
}

func (g *reachGraphTest) reached(reach Reach) map[string]int {
	result := map[string]int{}
	for _, subject := range reach.Subjects {
		result[g.names[subject.SubjectID]] = subject.Depth
	}
	return result
}

func (g *reachGraphTest) edge(position int) string {
	edge := g.index.StructuralEdges[position]
	return g.names[edge.FromSubjectID] + "→" + g.names[edge.ToSubjectID]
}

func (g *reachGraphTest) edges(positions []int) []string {
	var result []string
	for _, position := range positions {
		result = append(result, g.edge(position))
	}
	slices.Sort(result)
	return result
}

func (g *reachGraphTest) phases() map[string]string {
	result := map[string]string{}
	for _, subject := range g.index.Subjects {
		result[g.names[subject.ID]] = subject.Phase
	}
	return result
}

// An input reaches what its handler calls, every alternative of a dispatch
// among them, and the data that code reads. It does not reach what it
// imports, hands over as a callback, decorates or writes, nor what an
// unresolved call's stores name; a table it reads is reached and never
// walked from, so the callback stored in it is not.
func TestAnInputReachesWhatItsHandlerCallsAndReadsNothingMore(t *testing.T) {
	g := newReachGraphTest()
	g.functions("handler", "helper", "either", "or", "imported", "callback", "decorator", "stored", "guessed", "written", "init")
	g.declare(programindex.ObjectVariable, "table")
	g.call("handler", "helper")
	g.dispatch("helper", "either", "or")
	g.relate(programindex.RelationImports, programindex.ResolutionExact, "handler", "imported")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "handler", "callback")
	g.relate(programindex.RelationDecorates, programindex.ResolutionExact, "decorator", "handler")
	g.relate(programindex.RelationDecorates, programindex.ResolutionExact, "handler", "decorator")
	g.relate(programindex.RelationCalls, programindex.ResolutionUnresolved, "handler", "guessed")
	g.relate(programindex.RelationWrites, programindex.ResolutionExact, "handler", "written")
	g.relate(programindex.RelationReads, programindex.ResolutionExact, "helper", "table")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "table", "stored")
	g.call("table", "init")
	g.input("run", "handler")
	g.derive()
	got := g.reached(g.reachOf("run"))
	want := map[string]int{"handler": 0, "helper": 1, "either": 2, "or": 2, "table": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reached %v, want %v", got, want)
	}
	if first := g.reachOf("run").Subjects[0]; g.names[first.SubjectID] != "handler" {
		t.Fatalf("the reach does not start at the handler: %+v", g.reachOf("run").Subjects)
	}
	if edges := g.edges(g.reachOf("run").Edges); !reflect.DeepEqual(edges, []string{"handler→helper", "helper→either", "helper→or", "helper→table"}) {
		t.Fatalf("followed %v", edges)
	}
}

// Every followed relation into a part from a part reached earlier enters it,
// none chosen by length; one from a part reached no earlier is counted. A
// caller off the map stands for every earlier part that reaches it through
// code off the map, and a handler off the map enters parts itself.
func TestEveryCallIntoAPartIsAWitness(t *testing.T) {
	g := newReachGraphTest()
	g.functions("handler", "a", "a2", "b", "hidden", "c", "offHandler")
	g.part("Entry", "handler")
	g.part("A", "a", "a2")
	g.part("B", "b")
	g.part("C", "c")
	g.call("handler", "a", "a2", "hidden")
	g.call("a", "b", "hidden")
	g.call("b", "a2")
	g.call("hidden", "c")
	g.call("offHandler", "c")
	g.input("run", "handler")
	g.input("off", "offHandler")
	g.derive()
	parts := map[string]ReachedGroup{}
	titles := map[string]string{}
	for _, group := range g.index.Groups {
		titles[group.ID] = group.Title
	}
	for _, group := range g.reachOf("run").Groups {
		parts[titles[group.GroupID]] = group
	}
	describe := func(group ReachedGroup) []string {
		var result []string
		for _, witness := range group.Entered {
			var from []string
			for _, id := range witness.From {
				from = append(from, titles[id])
			}
			result = append(result, g.edge(witness.Edge)+" from "+strings.Join(from, "+"))
		}
		return result
	}
	if got := describe(parts["A"]); !reflect.DeepEqual(got, []string{"handler→a from Entry", "handler→a2 from Entry"}) || parts["A"].Others != 1 || parts["A"].Depth != 1 {
		t.Fatalf("A entered by %v, others %d", got, parts["A"].Others)
	}
	if got := describe(parts["B"]); !reflect.DeepEqual(got, []string{"a→b from A"}) || parts["B"].Others != 0 {
		t.Fatalf("B entered by %v", got)
	}
	if got := describe(parts["C"]); !reflect.DeepEqual(got, []string{"hidden→c from Entry+A"}) {
		t.Fatalf("C entered by %v: the caller off the map stands for the parts reaching it", got)
	}
	var order []string
	for _, group := range g.reachOf("run").Groups {
		order = append(order, titles[group.GroupID])
	}
	if !reflect.DeepEqual(order, []string{"Entry", "A", "B", "C"}) {
		t.Fatalf("parts by depth: %v", order)
	}
	off := g.reachOf("off").Groups
	if len(off) != 1 || len(off[0].Entered) != 1 || len(off[0].Entered[0].From) != 0 {
		t.Fatalf("a handler off the map enters its first part itself: %+v", off)
	}
}

// B12: the work an input's handler reaches is runtime, not initialization,
// even where no chain leads to an outside call; what the launch and a
// handler both reach is both; what only the launch reaches is
// initialization. A connection has its source's phase.
func TestWorkAnInputReachesIsNotInitialization(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "setup", "register", "handler", "internal", "shared")
	g.call("main", "setup", "register", "shared")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "register", "handler")
	g.call("handler", "internal", "shared")
	g.part("Launch", "main", "setup", "register")
	g.part("Work", "handler", "internal")
	g.part("Common", "shared")
	g.seed("main")
	g.input("run", "handler")
	g.index.Connections = []Connection{
		{From: Endpoint{TargetID: "t1", GroupID: "g2"}, To: Endpoint{TargetID: "t1", GroupID: "g3"}, FromSubjectID: g.ids["handler"], ToSubjectID: g.ids["shared"]},
		{From: Endpoint{TargetID: "t1", GroupID: "g1"}, To: Endpoint{TargetID: "t1", GroupID: "g3"}, FromSubjectID: g.ids["main"], ToSubjectID: g.ids["shared"]},
	}
	g.derive()
	want := map[string]string{"main": PhaseInit, "setup": PhaseInit, "register": PhaseInit, "handler": PhaseRuntime, "internal": PhaseRuntime, "shared": PhaseBoth}
	if got := g.phases(); !reflect.DeepEqual(got, want) {
		t.Fatalf("phases %v, want %v", got, want)
	}
	if g.index.Connections[0].Phase != PhaseRuntime || g.index.Connections[1].Phase != PhaseInit {
		t.Fatalf("connection phases %q %q", g.index.Connections[0].Phase, g.index.Connections[1].Phase)
	}
}

// A program with no input handled by a declaration (Redis's client, its
// benchmark and its dump checker) does all its work from its launch: none
// of it is initialization, so nothing of it is drawn quiet as wiring.
func TestAProgramThatServesNothingHasNoInitialization(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "connect", "send")
	g.call("main", "connect")
	g.call("connect", "send")
	g.seed("main")
	g.index.Operations = []Operation{{ID: "o1", Kind: "entry", Name: "unbound"}}
	g.derive()
	for name, phase := range g.phases() {
		if phase != "" {
			t.Fatalf("%s has phase %q in a program that serves nothing", name, phase)
		}
	}
}

// A dispatch site names every input whose handler is one of its
// alternatives, and lists the inputs that reach it by calls with every
// call on any route to it. An input reaching it only through another
// input's handler by an alternative is that input's route, not its own; the
// launch is no input.
func TestTheDispatchSiteListsEveryInputThatReachesIt(t *testing.T) {
	g := newReachGraphTest()
	g.functions("main", "aeMain", "processCommand", "call", "loadAppendOnlyFile",
		"getCommand", "execCommand", "lpushCommand", "debugCommand", "pushGeneric", "wakeReader", "wakeWriter", "signalKey")
	g.call("main", "aeMain", "loadAppendOnlyFile")
	g.call("aeMain", "processCommand")
	g.call("processCommand", "call")
	g.dispatch("call", "getCommand", "execCommand", "lpushCommand", "debugCommand")
	g.call("execCommand", "call")
	g.call("lpushCommand", "pushGeneric")
	g.call("pushGeneric", "wakeReader", "wakeWriter", "signalKey")
	g.call("wakeReader", "call")
	g.call("wakeWriter", "call")
	g.call("debugCommand", "loadAppendOnlyFile")
	g.dispatch("loadAppendOnlyFile", "getCommand", "execCommand", "lpushCommand", "debugCommand")
	g.seed("main")
	g.input("get", "getCommand")
	g.input("exec", "execCommand")
	g.input("lpush", "lpushCommand")
	g.input("debug", "debugCommand")
	g.derive()
	sites := map[string]DispatchSite{}
	var order []string
	for _, site := range g.index.Dispatch {
		sites[g.names[site.FromSubjectID]] = site
		order = append(order, g.names[site.FromSubjectID])
	}
	if !reflect.DeepEqual(order, []string{"call", "loadAppendOnlyFile"}) {
		t.Fatalf("dispatch sites in source order: %v", order)
	}
	operations := map[string]string{}
	for _, operation := range g.index.Operations {
		operations[operation.ID] = operation.Name
	}
	reachedFrom := func(site DispatchSite) map[string][]string {
		result := map[string][]string{}
		for _, reach := range site.ReachedFrom {
			result[operations[reach.OperationID]] = g.edges(reach.Edges)
		}
		return result
	}
	if got := sites["call"].OperationIDs; len(got) != 4 || len(sites["call"].Alternatives) != 4 {
		t.Fatalf("call dispatches %v of %v", got, sites["call"].Alternatives)
	}
	want := map[string][]string{
		"exec":  {"execCommand→call"},
		"lpush": {"lpushCommand→pushGeneric", "pushGeneric→wakeReader", "pushGeneric→wakeWriter", "wakeReader→call", "wakeWriter→call"},
	}
	if got := reachedFrom(sites["call"]); !reflect.DeepEqual(got, want) {
		t.Fatalf("call reached from %v, want %v", got, want)
	}
	if got := reachedFrom(sites["loadAppendOnlyFile"]); !reflect.DeepEqual(got, map[string][]string{"debug": {"debugCommand→loadAppendOnlyFile"}}) {
		t.Fatalf("loadAppendOnlyFile reached from %v", got)
	}
	// C1: the dispatch is where the other handlers begin; exec's reach
	// stops there.
	if got := g.reached(g.reachOf("exec")); !reflect.DeepEqual(got, map[string]int{"execCommand": 0, "call": 1}) {
		t.Fatalf("exec reaches %v", got)
	}
}

// C1: an exact call from one input's handler into another's is the first
// handler's own code, so its reach goes on through the second's callees.
func TestAnExactCallIntoAnotherHandlerKeepsReachingItsCode(t *testing.T) {
	g := newReachGraphTest()
	g.functions("listUsers", "getUser", "loadUser")
	g.call("listUsers", "getUser")
	g.call("getUser", "loadUser")
	g.input("list", "listUsers")
	g.input("get", "getUser")
	g.derive()
	if got := g.reached(g.reachOf("list")); !reflect.DeepEqual(got, map[string]int{"listUsers": 0, "getUser": 1, "loadUser": 2}) {
		t.Fatalf("list reaches %v", got)
	}
}

// A declaration that runs and hands another input's handler over registers
// that input; the hand-over is not a call, so the handed handler's code is
// not reached. A table the reach only reads registers nothing (B1): Redis's
// cmdTable, read by six commands, would have made every command
// "registered by" them.
func TestAHandlerHandingOverAnotherInputsHandlerRegistersIt(t *testing.T) {
	g := newReachGraphTest()
	g.functions("acceptHandler", "createClient", "readQueryFromClient", "processInputBuffer", "lookupCommand", "getCommand", "execCommand")
	g.declare(programindex.ObjectVariable, "cmdTable")
	g.call("acceptHandler", "createClient")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "createClient", "readQueryFromClient")
	g.call("readQueryFromClient", "processInputBuffer")
	g.call("execCommand", "lookupCommand")
	g.relate(programindex.RelationReads, programindex.ResolutionExact, "lookupCommand", "cmdTable")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "cmdTable", "getCommand")
	g.input("accept", "acceptHandler")
	g.input("read", "readQueryFromClient")
	g.input("get", "getCommand")
	g.input("exec", "execCommand")
	g.derive()
	if got := g.edges(g.reachOf("accept").HandsOver); !reflect.DeepEqual(got, []string{"createClient→readQueryFromClient"}) {
		t.Fatalf("accept hands over %v", got)
	}
	if got := g.reachOf("read").HandedOverBy; !reflect.DeepEqual(got, []string{"o1"}) {
		t.Fatalf("read is handed over by %v", got)
	}
	if _, reached := g.reached(g.reachOf("accept"))["readQueryFromClient"]; reached {
		t.Fatal("a hand-over was followed as a call")
	}
	if got := g.reachOf("exec").HandsOver; len(got) != 0 || len(g.reachOf("get").HandedOverBy) != 0 {
		t.Fatalf("a table read registers its callbacks: %v", g.edges(got))
	}
}

// The ordinary run renders from the projected indexes, a saved rendering
// from their hydrated overlays. Both derive after the joints are in, so
// they carry the same reach, dispatch sites, phases and helper marks.
func TestRenderDerivesWhatTheRunDerived(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 2}
	}
	call := func(ref, from, to string, line int) programindex.RelationInput {
		return programindex.RelationInput{SourceRef: ref, Kind: programindex.RelationCalls, FromRef: from, ToRefs: []string{to},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: at("svc/api/h.go", line),
			Witnesses: []programindex.Witness{{Kind: "call", Location: at("svc/api/h.go", line)}}, WitnessesObserved: 1}
	}
	program := func(name string, seeded bool, relations []programindex.RelationInput, files ...string) programindex.Index {
		var objects []programindex.ObjectInput
		for i, file := range files {
			objects = append(objects, programindex.ObjectInput{SourceRef: "o" + string(rune('a'+i)), Kind: programindex.ObjectFunction, Name: "F" + string(rune('A'+i)),
				Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: file, Line: 3, Column: 1}})
		}
		input := programindex.Input{ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
			Target: programindex.TargetInput{Language: "go", Kind: "executable", Name: name, Selector: name,
				Sources: []programindex.TargetSource{{FileRef: "f1", Path: files[0]}}, AnchorFileRef: "f1"},
			Objects: objects, Relations: relations, Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)}}
		if seeded {
			input.Target.Seeds = []programindex.TargetSeedInput{{ObjectRef: "oa", Kind: programindex.SeedCallable, Location: &programindex.Location{Path: files[0], Line: 3, Column: 1}}}
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		return index
	}
	svc := program("svc", true, []programindex.RelationInput{call("main-calls-handler", "oa", "ob", 4), call("handler-calls-client", "ob", "oc", 5)},
		"svc/main.go", "svc/api/h.go", "svc/client/c.go")
	peer := program("peer", false, nil, "peer/server.go")
	rebound := rebindTestTargets(t, svc, peer)
	svc, peer = rebound[0], rebound[1]
	box := func(p programindex.Index, id, path, title, side string, object int, symbol atlas.Symbol) atlas.Box {
		symbol.ID, symbol.ObjectID, symbol.Name, symbol.Kind, symbol.LineNo, symbol.Column = "s"+id, p.Target.ID+"."+p.Objects[object].ID, p.Objects[object].Name, "function", 3, 1
		return atlas.Box{ID: id, Dir: id, Title: title, Line: title + ".", Side: side, Open: true, MemberIDs: []string{p.Objects[object].ID},
			Files: []atlas.File{{Path: path, Line: "File.", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{symbol}}}}
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "x", Revision: "abc", Diagnostics: []atlas.Diagnostic{},
		Targets: []atlas.Target{
			{ID: svc.Target.ID, Language: "go", Kind: "executable", Name: "svc", Root: "svc", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
				Boxes: []atlas.Box{
					box(svc, "main", "svc/main.go", "Launch", atlas.SideMid, 0, atlas.Symbol{}),
					box(svc, "api", "svc/api/h.go", "Handlers", atlas.SideIn, 1, atlas.Symbol{Activation: "request", Operation: "get level", OperationSummary: "Returns a level."}),
					box(svc, "client", "svc/client/c.go", "Client", atlas.SideOut, 2, atlas.Symbol{Helper: true}),
				},
				Boundaries: []atlas.Boundary{{ID: "b-out", BoxID: "client", ObjectID: svc.Target.ID + "." + svc.Objects[2].ID, Path: "svc/client/c.go", LineNo: 5, Caller: "FC",
					Direction: atlas.DirectionOut, Kind: atlas.BoundaryClientRequest, Values: []string{"/peer"}, Line: "Asks the peer."}}},
			{ID: peer.Target.ID, Language: "go", Kind: "executable", Name: "peer", Root: "peer", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
				Boxes: []atlas.Box{box(peer, "server", "peer/server.go", "Server", atlas.SideIn, 0, atlas.Symbol{})},
				Boundaries: []atlas.Boundary{{ID: "b-in", BoxID: "server", ObjectID: peer.Target.ID + "." + peer.Objects[0].ID, Path: "peer/server.go", LineNo: 7, Caller: "FA",
					Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest, Values: []string{"/peer"}, Line: "Serves the peer."}}},
		},
		Joints: []atlas.Joint{{ID: "j1", From: atlas.Endpoint{TargetID: svc.Target.ID, BoundaryID: "b-out"}, To: atlas.Endpoint{TargetID: peer.Target.ID, BoundaryID: "b-in"},
			Value: "GET /peer", Same: true, Label: "asks the peer"}},
	}
	programs := map[string]programindex.Index{svc.Target.ID: svc, peer.Target.ID: peer}
	indexes, err := ProjectAtlas(programs, value)
	if err != nil {
		t.Fatal(err)
	}
	joint := false
	for _, index := range indexes {
		hydrated, err := OverlayFromIndex(index).Hydrate(programs[index.Target.ID])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(index.Reach, hydrated.Reach) || !reflect.DeepEqual(index.Dispatch, hydrated.Dispatch) {
			t.Fatalf("%s: the rendering derives another reach", index.Target.Name)
		}
		for position, subject := range index.Subjects {
			if subject.Phase != hydrated.Subjects[position].Phase {
				t.Fatalf("%s: %s is %q in the run and %q rendered", index.Target.Name, subject.ID, subject.Phase, hydrated.Subjects[position].Phase)
			}
		}
		for position, connection := range index.Connections {
			other := hydrated.Connections[position]
			if connection.Phase != other.Phase || connection.ToHelper != other.ToHelper {
				t.Fatalf("%s: connection %s is %q/%v in the run and %q/%v rendered", index.Target.Name, connection.ID, connection.Phase, connection.ToHelper, other.Phase, other.ToHelper)
			}
			if connection.To.TargetID != index.Target.ID {
				joint = true
				if connection.Phase != PhaseBoth {
					t.Fatalf("the joint from the handler's client has phase %q, want both", connection.Phase)
				}
			}
		}
	}
	if !joint {
		t.Fatal("no joint was projected")
	}
}
