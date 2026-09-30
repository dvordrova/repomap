package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// flowFixture is a small server: serverCron in its own part calls, in the
// order they are written, a logger in a part most parts call, a resize of
// its own part, a helper of its own part, a library's fork, a save helper
// of another part, a lookup no part holds, and one of two handlers at a
// dispatch site. The slice holds the calls out of their written order.
func flowFixture() (*pageBuilder, groupindex.Index) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "redis.c", Line: line, Column: 5}
	}
	objects := map[string]*groupindex.ObjectFacts{}
	helper := map[string]bool{"log": true, "closeClients": true, "save": true}
	builder := &pageBuilder{subjects: map[string]subjectRef{}, groupTitles: map[groupindex.Endpoint]string{}, byProgram: map[string]*pageSection{"t1": {ID: "t1", ShortLabel: "redis-server"}},
		links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"}, data: &ReportData{ProgramPortfolio: &ProgramPortfolio{}}}
	add := func(id, name string, line int) {
		objects[id] = &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: at(line)}
		subject := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: objects[id]}
		if helper[id] {
			subject.Interpretation = &groupindex.Interpretation{Helper: true}
		}
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: subject}
	}
	for id, place := range map[string]struct {
		name string
		line int
	}{"cron": {"serverCron", 1250}, "resize": {"tryResizeHashTables", 1180}, "closeClients": {"closeTimedoutClients", 1200}, "log": {"redisLog", 100},
		"save": {"rdbSaveBackground", 3000}, "lookup": {"lookupKeyRead", 900}, "h1": {"getCommand", 4000}, "h2": {"setCommand", 4100}, "x4": {"dictFind", 50},
		"fail": {"_redisAssert", 2000}} {
		add(id, place.name, place.line)
	}
	external := func(id, header, name string) {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: header + "." + name, Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{PackagePath: header, Name: name}}}}
	}
	external("fork", "unistd.h", "fork")
	external("exit", "unistd.h", "_exit")
	external("assertRtn", "assert.h", "__assert_rtn")
	external("expect", "builtin", "__builtin_expect")
	// Calls a macro's expansion makes (the C adapter's macro_expansion
	// witness, the macro as written its selector): the system header's
	// assert, and the repository's redisAssert, whose body the repository
	// spells.
	macro := func(id, name string, own bool) programindex.Relation {
		witness := programindex.Witness{Kind: "macro_expansion", Detail: name + " expands to a call"}
		if own {
			witness.Location = &programindex.Location{Path: "redis.c", Line: 238}
		}
		return programindex.Relation{ID: id, Patterns: []programindex.RelationPattern{{Selector: name}}, Witnesses: []programindex.Witness{witness}}
	}
	builder.data.ProgramPortfolio.Entries = []programindex.Index{{Target: programindex.Target{ID: "t1"}, Relations: []programindex.Relation{
		macro("assert-1", "assert", false), macro("expect-1", "assert", false), macro("assert-2", "assert", false),
		macro("redisAssert-1", "redisAssert", true), macro("exit-1", "redisAssert", true)}}}
	call := func(from, to string, line int, relation string, kind programindex.RelationKind, resolution programindex.Resolution) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationID: relation, RelationKind: kind, Resolution: resolution, Location: at(line)}
	}
	exact := func(from, to string, line int) groupindex.StructuralEdge {
		return call(from, to, line, from+"-"+to+"-"+fmt.Sprint(line), programindex.RelationCalls, programindex.ResolutionExact)
	}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{
		{ID: "g1", Title: "Server lifecycle and cron", MemberSubjectIDs: []string{"cron", "resize", "closeClients"}},
		{ID: "g2", Title: "Server core state", MemberSubjectIDs: []string{"log"}},
		{ID: "g3", Title: "Persistence", MemberSubjectIDs: []string{"save"}},
		{ID: "g4", Title: "Core data structures", MemberSubjectIDs: []string{"x4"}},
		{ID: "g5", Title: "String commands", MemberSubjectIDs: []string{"h1", "h2"}},
		{ID: "g6", Title: "Debug", MemberSubjectIDs: []string{"fail"}},
	}, StructuralEdges: []groupindex.StructuralEdge{
		exact("cron", "save", 1322), exact("cron", "log", 1288), exact("cron", "resize", 1284), exact("cron", "log", 1273),
		call("cron", "fork", 1300, "fork", programindex.RelationInvokesExternal, programindex.ResolutionExact),
		exact("cron", "closeClients", 1297), exact("cron", "lookup", 1350),
		call("cron", "h2", 1360, "dispatch", programindex.RelationCalls, programindex.ResolutionAlternatives),
		call("cron", "h1", 1360, "dispatch", programindex.RelationCalls, programindex.ResolutionAlternatives),
		call("cron", "assertRtn", 1370, "assert-1", programindex.RelationInvokesExternal, programindex.ResolutionExact),
		call("cron", "expect", 1370, "expect-1", programindex.RelationInvokesExternal, programindex.ResolutionExact),
		call("cron", "assertRtn", 1372, "assert-2", programindex.RelationInvokesExternal, programindex.ResolutionExact),
		call("cron", "expect", 1374, "builtin-1", programindex.RelationInvokesExternal, programindex.ResolutionExact),
		call("cron", "fail", 1380, "redisAssert-1", programindex.RelationCalls, programindex.ResolutionExact),
		call("cron", "exit", 1380, "exit-1", programindex.RelationInvokesExternal, programindex.ResolutionExact),
		exact("save", "log", 3010), exact("h1", "log", 4010), exact("x4", "log", 60),
		exact("lookup", "log", 905), exact("lookup", "x4", 906),
	}}
	for _, group := range index.Groups {
		builder.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: group.ID}] = group.Title
	}
	builder.indexes = []groupindex.Index{index}
	return builder, index
}

// A function's flow is its calls in the order they are written, under each
// part once, each callee once with every place it is called; a dispatch
// site is one call;
// a library's call is named; a call a macro's expansion makes is the
// macro as written, once, and a compiler builtin no call of its own
// (owner, 2026-09-29: handleClientsWaitingListPush's assert had read
// __assert_rtn and __builtin_expect); a helper folds when its part is one most
// parts call into, and stays when its part says what it is for or is the
// caller's own (its own work); a declaration no part holds keeps its call
// and its own flow.
func TestAFunctionsFlowIsItsCallsInWrittenOrder(t *testing.T) {
	builder, index := flowFixture()
	raw := builder.groupReading(index, index.Groups[0], pageGroup{ID: "t1-g1", Title: index.Groups[0].Title})
	var reading pageGroupReading
	if err := json.Unmarshal([]byte(raw), &reading); err != nil {
		t.Fatal(err)
	}
	flowOf := func(name string) []string {
		for _, own := range reading.Own {
			if reading.Decls[own.Decl].Name != name {
				continue
			}
			var said []string
			for _, call := range own.Flow {
				line := ""
				switch {
				case call.Decl != nil:
					line = reading.Decls[*call.Decl].Name
					if reading.Decls[*call.Decl].Part == "" {
						line += " (no part)"
					}
				case call.One != nil:
					var names []string
					for _, one := range call.One {
						names = append(names, reading.Decls[one].Name)
					}
					line = "one of " + strings.Join(names, ", ")
				case call.Macro != "":
					line = "macro " + call.Macro + " from " + call.Lib
				default:
					line = call.Name + " from " + call.Lib
				}
				if call.Macro != "" && call.Decl != nil {
					line = "macro " + call.Macro + " calling " + line
				}
				if call.Helper {
					line += " [helper]"
				}
				var sites []string
				for _, site := range call.Sites {
					sites = append(sites, strings.TrimPrefix(site.At, "redis.c:"))
				}
				said = append(said, line+" @"+strings.Join(sites, ","))
			}
			return said
		}
		return nil
	}
	want := []string{
		"redisLog [helper] @1273,1288", "tryResizeHashTables @1284", "closeTimedoutClients @1297", "fork from unistd.h @1300",
		"macro assert from assert.h @1370,1372", "rdbSaveBackground @1322", "lookupKeyRead (no part) @1350", "one of setCommand, getCommand @1360",
		"macro redisAssert calling _redisAssert @1380",
	}
	if got := flowOf("serverCron"); !slices.Equal(got, want) {
		t.Fatalf("serverCron's flow = %q\nwant %q", got, want)
	}
	// The declaration no part holds is read where it is called: no call is
	// dropped (lookupKeyRead → lookupKey had been lost with it).
	if got := flowOf("lookupKeyRead"); !slices.Equal(got, []string{"redisLog [helper] @905", "dictFind @906"}) {
		t.Fatalf("lookupKeyRead's flow = %q", got)
	}
}

// A function handling inputs a case of its comparison declares keeps, beside
// its flow, what each case's lines call (critic, 2026-09-30: litestream's
// replicate had read all of Main.Run): only the calls written in the case,
// once for every input of it.
func TestACaseFlowIsWhatItsLinesCall(t *testing.T) {
	builder, index := flowFixture()
	index.Operations = []groupindex.Operation{
		{ID: "o1", SubjectID: "cron", Branch: &programindex.LineRange{Line: 1284, EndLine: 1300}},
		{ID: "o2", SubjectID: "cron", Branch: &programindex.LineRange{Line: 1284, EndLine: 1300}},
		{ID: "o3", SubjectID: "cron", Branch: &programindex.LineRange{Line: 1350, EndLine: 1360}},
		{ID: "o4", SubjectID: "cron"},
	}
	builder.indexes = []groupindex.Index{index}
	raw := builder.groupReading(index, index.Groups[0], pageGroup{ID: "t1-g1", Title: index.Groups[0].Title})
	var reading pageGroupReading
	if err := json.Unmarshal([]byte(raw), &reading); err != nil {
		t.Fatal(err)
	}
	var said []string
	for _, own := range reading.Own {
		if reading.Decls[own.Decl].Name != "serverCron" {
			continue
		}
		for _, inCase := range own.Cases {
			var calls []string
			for _, call := range inCase.Flow {
				var sites []string
				for _, site := range call.Sites {
					sites = append(sites, strings.TrimPrefix(site.At, "redis.c:"))
				}
				name := call.Name
				switch {
				case call.Decl != nil:
					name = reading.Decls[*call.Decl].Name
				case call.One != nil:
					name = "one of " + fmt.Sprint(len(call.One))
				}
				calls = append(calls, name+" @"+strings.Join(sites, ","))
			}
			said = append(said, fmt.Sprintf("%d: %s", inCase.Line, strings.Join(calls, " | ")))
		}
	}
	want := []string{
		"1284: tryResizeHashTables @1284 | closeTimedoutClients @1297 | redisLog @1288 | fork @1300",
		"1350: lookupKeyRead @1350 | one of 2 @1360",
	}
	if !slices.Equal(said, want) {
		t.Fatalf("serverCron's cases = %q\nwant %q", said, want)
	}
}

// A flow's calls stand under each part once (reviewer, 2026-09-30: redis
// main had shown "Server lifecycle and cron" four times): the parts in the
// order of their first call, each part's calls in written order, a
// dispatch site by its first declaration's part, and the calls the report
// names no declaration for in their written order among themselves.
func TestAFlowStandsUnderEachPartOnce(t *testing.T) {
	parts := []string{"#life", "#persist", "#core", "#life", "#loop", "#persist", "#life"}
	callee := func(position int) *int { return &position }
	calls := []pageFlowCall{
		{Decl: callee(0)}, {Decl: callee(1)}, {Name: "fprintf"}, {Decl: callee(2)}, {Decl: callee(3)},
		{One: []int{4, 0}}, {Name: "exit"}, {Decl: callee(5)}, {Decl: callee(6)},
	}
	var said []string
	for _, call := range groupFlowByPart(calls, func(position int) string { return parts[position] }) {
		switch {
		case call.Decl != nil:
			said = append(said, fmt.Sprint(*call.Decl))
		case call.One != nil:
			said = append(said, fmt.Sprint("one of ", call.One))
		default:
			said = append(said, call.Name)
		}
	}
	want := []string{"0", "3", "6", "1", "5", "fprintf", "exit", "2", "one of [4 0]"}
	if !slices.Equal(said, want) {
		t.Fatalf("grouped flow = %q, want %q", said, want)
	}
}

// How a request reaches a dispatch site reads as chains in call order, one
// per outer input by its shortest route through the callable it hands over,
// requests first; the callable is marked with how it is handed over, and
// another way names where it leaves the first: GET arrives from
// acceptHandler through readQueryFromClient, and from serverCron through
// syncWithMaster's createClient.
func TestARequestsWaysInAreChainsInCallOrder(t *testing.T) {
	names := []string{"acceptHandler", "serverCron", "syncWithMaster", "createClient", "readQueryFromClient", "freeClient", "processInputBuffer", "processCommand", "call"}
	builder := &pageBuilder{subjects: map[string]subjectRef{}}
	for _, name := range names {
		builder.subjects[subjectKey("t1", name)] = subjectRef{subject: groupindex.Subject{ID: name, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction}}}
	}
	var edges []groupindex.StructuralEdge
	edge := func(from, to string) int {
		edges = append(edges, groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls})
		return len(edges) - 1
	}
	route := []int{edge("readQueryFromClient", "freeClient"), edge("readQueryFromClient", "processInputBuffer"), edge("processInputBuffer", "processCommand"),
		edge("freeClient", "processInputBuffer"), edge("processCommand", "call")}
	handOver := edge("createClient", "readQueryFromClient")
	index := &groupindex.Index{Target: programindex.Target{ID: "t1"}, Operations: []groupindex.Operation{
		{ID: "cron", Kind: "scheduled", SubjectID: "serverCron"}, {ID: "accept", Kind: "request", SubjectID: "acceptHandler"}}}
	site := groupindex.DispatchSite{FromSubjectID: "call", Outer: []groupindex.OuterInput{
		{OperationID: "cron", Registered: "readQueryFromClient", HandOver: handOver, Registering: []int{edge("serverCron", "syncWithMaster"), edge("syncWithMaster", "createClient")}, Edges: route},
		{OperationID: "accept", Registered: "readQueryFromClient", HandOver: handOver, Registering: []int{edge("acceptHandler", "createClient")}, Edges: route},
	}}
	index.StructuralEdges = edges
	decls := builder.pathDecls("t1", func(string) string { return "" })
	ways := builder.waysIn(index, site, decls, func(id string) string { return "t1-" + id }, map[string]string{"t1-cron": "scheduled", "t1-accept": "request"})
	markWaysFrom(ways)
	said := func(list []int) string {
		var out []string
		for _, at := range list {
			out = append(out, decls.list[at].Name)
		}
		return strings.Join(out, " → ")
	}
	if len(ways) != 2 || ways[0].Input != "t1-accept" || said(ways[0].Chain) != "acceptHandler → readQueryFromClient → processInputBuffer → processCommand → call" ||
		ways[0].Hop != 1 || said(ways[0].By) != "acceptHandler → createClient" || ways[0].From != nil {
		t.Fatalf("the first way is not the request's chain: %+v", ways)
	}
	if ways[1].Input != "t1-cron" || said(ways[1].By) != "serverCron → syncWithMaster → createClient" || ways[1].From == nil || decls.list[*ways[1].From].Name != "syncWithMaster" {
		t.Fatalf("the other way does not name where it leaves the first: %+v", ways[1])
	}
}

// A Main flow step citing a registration reads the callable it registers,
// registered where the step's own path reaches: readQueryFromClient is
// registered in createClient, which acceptHandler (the step before) calls,
// and in beforeSleep's resume path, which it does not; the step names the
// first and links its line, never an arbitrary first site (owner,
// 2026-09-29: it had linked redis.c:1414). Unreached, every site is named.
func TestARegistrationStepIsRegisteredOnItsOwnPath(t *testing.T) {
	builder, index := flowFixture()
	add := func(id, name string, line int) {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "redis.c", Line: line, Column: 1}}}}
	}
	add("accept", "acceptHandler", 2500)
	add("create", "createClient", 2450)
	add("sleep", "beforeSleep", 1400)
	add("read", "readQueryFromClient", 2386)
	index.Groups[0].MemberSubjectIDs = append(index.Groups[0].MemberSubjectIDs, "accept", "create", "sleep", "read")
	index.StructuralEdges = append(index.StructuralEdges, groupindex.StructuralEdge{FromSubjectID: "accept", ToSubjectID: "create", Role: groupindex.EdgeRelationTarget,
		RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "redis.c", Line: 2510}})
	builder.indexes = []groupindex.Index{index}
	registration := func(id, owner string, line int) facts.Fact {
		return facts.Fact{ID: id, Kind: facts.KindRegistration, TargetID: "t1", Symbol: "readQueryFromClient", ObjectID: "read", OwnerID: owner,
			Anchor: &facts.Anchor{Path: "redis.c", Line: line}, Registrar: &facts.Registrar{Name: "aeCreateFileEvent"}}
	}
	builder.data.Facts = &facts.Result{Facts: []facts.Fact{registration("a147", "sleep", 1414), registration("a151", "create", 2456)}}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	said := func(path *pageStepPath) []string {
		var row pageFlowStep
		if !builder.registeredStep(&row, builder.data.Facts.Facts[0], section, path) {
			t.Fatal("the registration step was not read")
		}
		var out []string
		for _, site := range row.Registers {
			var by []string
			for _, name := range site.By {
				by = append(by, name.Name)
			}
			out = append(out, strings.Join(by, " → ")+" @"+site.At.Text)
		}
		if row.Label != "readQueryFromClient" {
			t.Fatalf("the step reads %q", row.Label)
		}
		return out
	}
	if got := said(&pageStepPath{shown: []string{"accept"}, runners: map[string]bool{}}); !slices.Equal(got, []string{"acceptHandler → createClient @redis.c:2456"}) {
		t.Fatalf("after acceptHandler the step is registered at %q", got)
	}
	if got := said(&pageStepPath{shown: []string{"h1"}, runners: map[string]bool{}}); !slices.Equal(got, []string{"beforeSleep @redis.c:1414", "createClient @redis.c:2456"}) {
		t.Fatalf("off every path the step names %q", got)
	}
}

// What a program runs on its own reads after its Main flow (owner,
// 2026-09-29: serverCron was not findable from a flow of client commands):
// its scheduled inputs, then its continuous ones, each the callable its
// registration hands over, registered by its registering function alone
// (no run of calls from the entries is chosen: litestream's Replica.monitor
// had read "… ReplicateCommand.Run → Store.Close → … → Replica.Start
// registers it", through its shutdown) and run by the function calling it
// through a value, that function's run from the last runner the flow
// already showed; a callable no registration hands over is its name alone,
// one the Main flow names is not repeated, and a request is no work of its
// own.
func TestWorkARunsOnItsOwnReadsAfterTheMainFlow(t *testing.T) {
	builder, index := flowFixture()
	add := func(id, name string, line int) {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "redis.c", Line: line, Column: 1}}}}
	}
	add("main", "main", 9124)
	add("init", "initServer", 1532)
	add("events", "aeProcessEvents", 275)
	add("timers", "processTimeEvents", 212)
	add("spawn", "spawnIOThread", 8719)
	add("thread", "IOThreadEntryPoint", 8665)
	add("tick", "tickTimer", 700)
	index.Groups[0].MemberSubjectIDs = append(index.Groups[0].MemberSubjectIDs, "main", "init", "events", "timers", "spawn", "thread", "tick")
	exact := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget,
			RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "redis.c", Line: 1}}
	}
	index.StructuralEdges = append(index.StructuralEdges, exact("main", "init"), exact("main", "events"), exact("events", "timers"))
	index.Entries = []groupindex.Entry{{SubjectID: "main"}}
	index.Operations = []groupindex.Operation{
		{ID: "o1", Kind: "request", SubjectID: "h1"},
		{ID: "o2", Kind: "continuous", SubjectID: "thread", FactID: "a2"},
		{ID: "o3", Kind: "scheduled", SubjectID: "cron", FactID: "a1"},
		{ID: "o4", Kind: "scheduled", SubjectID: "tick"},
		{ID: "o5", Kind: "scheduled", SubjectID: "resize"},
	}
	builder.indexes = []groupindex.Index{index}
	registration := func(id, object, owner string, line int) facts.Fact {
		return facts.Fact{ID: id, Kind: facts.KindRegistration, TargetID: "t1", ObjectID: object, OwnerID: owner, Anchor: &facts.Anchor{Path: "redis.c", Line: line}}
	}
	builder.data.Facts = &facts.Result{Facts: []facts.Fact{registration("a1", "cron", "init", 1575), registration("a2", "thread", "spawn", 8728)}}
	builder.factsByID = builder.data.Facts.ByID()
	// processTimeEvents calls te->timeProc, which only serverCron is
	// stored in: a call through a function value resolved to it alone.
	builder.data.ProgramPortfolio.Entries[0].Relations = append(builder.data.ProgramPortfolio.Entries[0].Relations,
		programindex.Relation{ID: "time-proc", Kind: programindex.RelationCalls, FromID: "timers", ToIDs: []string{"cron"}, Resolution: programindex.ResolutionExact, Dispatch: programindex.DispatchFunctionValue})
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	resize, _ := builder.subject("t1", "resize")
	_, resizeAnchor := builder.subjectDisplay(resize.subject)
	flow := &pageFlow{Steps: []pageFlowStep{{Label: "tryResizeHashTables", Key: declarationKey(resizeAnchor)}}}
	var said []string
	for _, work := range builder.ownWork(section, flow, &pageStepPath{runners: map[string]bool{"events": true}}) {
		var how []string
		for _, site := range work.Registers {
			var by []string
			for _, name := range site.By {
				by = append(by, name.Name)
			}
			how = append(how, strings.Join(by, " → ")+" registers it @"+site.At.Text)
		}
		for _, chain := range work.RunBy {
			var by []string
			for _, name := range chain {
				by = append(by, name.Name)
			}
			how = append(how, strings.Join(by, " → ")+" runs it")
		}
		line := work.Input + " " + work.Label + " @" + work.Anchor.Text
		if len(how) > 0 {
			line += " — " + strings.Join(how, "; ")
		}
		said = append(said, line)
	}
	want := []string{
		"t1-o3 serverCron @redis.c:1250 — initServer registers it @redis.c:1575; aeProcessEvents → processTimeEvents runs it",
		"t1-o4 tickTimer @redis.c:700",
		"t1-o2 IOThreadEntryPoint @redis.c:8665 — spawnIOThread registers it @redis.c:8728",
	}
	if !slices.Equal(said, want) {
		t.Fatalf("it runs on its own:\n%s\nwant\n%s", strings.Join(said, "\n"), strings.Join(want, "\n"))
	}
}

// A walked flow says how a step is reached as its code does, and a
// dispatch site by the function holding it, a name read as a step's is,
// never a file and line (owner: no line numbers in the column; redis's
// flow had read "one of 94 at redis.c:2054"). A named fork is one folded
// line of its candidates.
func TestAFlowsViaAndForkNameTheSitesFunctionNotItsLine(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "h2", Via: "one of 2", Site: "cron"}, section, &pageStepPath{runners: map[string]bool{}})
	fork := builder.flowFork(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Branches: []orientation.FlowBranch{
		{SubjectID: "h1", Via: "one of 2", Site: "cron"}, {SubjectID: "h2", Via: "one of 2", Site: "cron"}}})
	var names []string
	for _, name := range fork.Names {
		names = append(names, name.Name+" "+name.Part)
	}
	if step.ViaFrom == nil || step.ViaFrom.Name != "serverCron" || fork.From == nil || fork.From.Name != "serverCron" || fork.Label != "one of 2" ||
		!slices.Equal(names, []string{"getCommand #t1-g5", "setCommand #t1-g5"}) {
		t.Fatalf("step via %q from %+v; fork %q from %+v, %q", step.Via, step.ViaFrom, fork.Label, fork.From, names)
	}
	step.Fork = fork
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step}}}); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	read := func(from, to string) string {
		at := strings.Index(html, from)
		if at < 0 {
			t.Fatalf("the step lost %s: %s", from, html)
		}
		end := strings.Index(html[at:], to)
		if end < 0 {
			t.Fatalf("%s is not closed: %s", from, html[at:])
		}
		return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html[at:at+end], "")
	}
	via, summary := read(`<span class="flow-via">`, `</span>`), read(`<summary>`, `</summary>`)
	if !strings.Contains(via, "one of 2 from serverCron") || !strings.Contains(summary, "one of 2 from serverCron") ||
		!strings.Contains(html, `<details class="flow-fork">`) {
		t.Fatalf("via %q, fork %q", via, summary)
	}
	for _, text := range []string{via, summary} {
		if regexp.MustCompile(`\S+\.\w+:\d+`).MatchString(text) {
			t.Fatalf("a flow row prints a file and line: %q", text)
		}
	}
	mixed := builder.flowFork(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Branches: []orientation.FlowBranch{
		{SubjectID: "h1", Via: "called"}, {SubjectID: "save", Via: "handed to aeCreateTimeEvent"}}})
	if mixed.Label != "one of 2" || mixed.From != nil || len(mixed.Names) != 2 {
		t.Fatalf("mixed fork = %+v", mixed)
	}
	if builder.flowFork(orientation.FlowStep{TargetID: "t1", SubjectID: "cron"}) != nil {
		t.Fatal("a step with no branches has a fork")
	}
}

// A fork's candidates sharing a name are told apart as the categorizer
// read them, by where they stand: litestream's Sync fork had listed
// ReplicaClient eight times.
func TestAForksSameNamedCandidatesAreToldApart(t *testing.T) {
	builder, _ := flowFixture()
	for id, file := range map[string]string{"c1": "s3/replica_client.go", "c2": "gs/replica_client.go"} {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: "ReplicaClient", Kind: programindex.ObjectType, Location: &programindex.Location{Path: file, Line: 20, Column: 6}}}}
	}
	fork := builder.flowFork(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Branches: []orientation.FlowBranch{
		{SubjectID: "lookup", Via: "called"}, {SubjectID: "c1", Via: "called"}, {SubjectID: "c2", Via: "called"}}})
	var names []string
	for _, name := range fork.Names {
		names = append(names, name.Name)
	}
	if !slices.Equal(names, []string{"lookupKeyRead", "s3.ReplicaClient", "gs.ReplicaClient"}) {
		t.Fatalf("fork names = %q", names)
	}
}
