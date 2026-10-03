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

// A Main flow step whose callable a registration hands over reads where the
// walk found it registered and what runs it, as orientation saved them
// (version 3), the page deriving none: readQueryFromClient is registered
// by createClient, which acceptHandler, a callable aeProcessEvents may call
// through a value, calls; the hop through the value reads "may call" in
// words, never a count, and "registers it" links the registering line.
func TestARegistrationStepReadsItsSavedRegistration(t *testing.T) {
	builder, index := flowFixture()
	add := func(id, name string, line int) {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "redis.c", Line: line, Column: 1}}}}
	}
	add("events", "aeProcessEvents", 275)
	add("accept", "acceptHandler", 2500)
	add("create", "createClient", 2450)
	add("read", "readQueryFromClient", 2386)
	index.Groups[0].MemberSubjectIDs = append(index.Groups[0].MemberSubjectIDs, "events", "accept", "create", "read")
	builder.indexes = []groupindex.Index{index}
	builder.data.Facts = &facts.Result{Facts: []facts.Fact{{ID: "a151", Kind: facts.KindRegistration, TargetID: "t1", Symbol: "readQueryFromClient", ObjectID: "read", OwnerID: "create",
		Anchor: &facts.Anchor{Path: "redis.c", Line: 2456}, Registrar: &facts.Registrar{Name: "aeCreateFileEvent"}}}}
	builder.factsByID = builder.data.Facts.ByID()
	section := builder.byProgram["t1"]
	section.programTargetID = "t1"
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "read", Via: "one of 5", Site: "events",
		Registered: []orientation.FlowRegistration{{FactID: "a151", Chain: []orientation.FlowHop{{SubjectID: "events"}, {SubjectID: "accept", Possible: true}, {SubjectID: "create"}}}}},
		section, map[string]bool{})
	if step.Label != "readQueryFromClient" || len(step.Registers) != 1 || step.Registers[0].At == nil || step.Registers[0].At.Text != "redis.c:2456" {
		t.Fatalf("the step reads %q registered %+v", step.Label, step.Registers)
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step}}}); err != nil {
			t.Fatal(err)
		}
		chain := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(out.String()[strings.Index(out.String(), `<span class="flow-chain">`):], "")
		chain = chain[:strings.Index(chain, map[DisplayLanguage]string{English: "registers it", Russian: "регистрирует его"}[language])]
		want := map[DisplayLanguage]string{English: "aeProcessEvents may call acceptHandler → createClient ", Russian: "aeProcessEvents может вызвать acceptHandler → createClient "}[language]
		if chain != want {
			t.Fatalf("%v: the registration reads %q, want %q", language, chain, want)
		}
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
	for _, work := range builder.ownWork(section, flow, map[string]bool{"events": true}) {
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

// Where the walk decided a split, the step keeps the calls the path did not
// follow, read in words under one folded line, never "one of N" (external
// review, 2026-10-02: a reader could not see that freqtrade's process also
// calls the strategy).
func TestADecidedSplitsPassedCallsReadFoldedUnderAlsoCalls(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Via: "called",
		Passed: []orientation.FlowBranch{{SubjectID: "h1", Via: "called"}, {SubjectID: "h2", Via: "called"}}}, section, map[string]bool{})
	if step.Passed == nil || step.Fork != nil || len(step.Passed.Names) != 2 {
		t.Fatalf("passed %+v, fork %+v", step.Passed, step.Fork)
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step}}}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		label := map[DisplayLanguage]string{English: "also calls:", Russian: "также вызывает:"}[language]
		at := strings.Index(html, `<details class="flow-fork flow-passed"><summary>`+label+`</summary>`)
		if at < 0 || !strings.Contains(html[at:], "getCommand") || !strings.Contains(html[at:], "setCommand") || strings.Contains(html, "one of 2") {
			t.Fatalf("%v: the passed calls do not read folded under %q: %s", language, label, html)
		}
	}
}

// A Main flow says what its steps are (external review, 2026-10-02: it had
// named calls and never said what they were for): each run of steps read in
// one part stands under that part once; a method's type is said with the
// type's own line once for a run of its methods; a step handling an input
// names it by kind, several folding; the flow closes saying where its path
// stops and that each step follows one call the step before may make.
func TestAMainFlowSaysEachStepsPartTypeAndInputs(t *testing.T) {
	builder, index := flowFixture()
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "server.c", Line: line, Column: 1}
	}
	builder.subjects[subjectKey("t1", "server")] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: "server", Kind: groupindex.SubjectObject,
		Object: &groupindex.ObjectFacts{Name: "Server", Kind: programindex.ObjectType, Location: at(10)}, Interpretation: &groupindex.Interpretation{Line: "The server's state and its event loop."}}}
	for id, line := range map[string]int{"process": 20, "serve": 40} {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectMethod, OwnerID: "server", Location: at(line)}}}
	}
	index.Groups[0].MemberSubjectIDs = append(index.Groups[0].MemberSubjectIDs, "server", "process", "serve")
	index.Operations = []groupindex.Operation{
		{ID: "o1", Kind: "request", Name: "set", SubjectID: "h2"},
		{ID: "o2", Kind: "request", Name: "get", SubjectID: "h1"},
		{ID: "o3", Kind: "request", Name: "mget", SubjectID: "h1"},
		{ID: "o4", Kind: "setting", Name: "maxmemory", SubjectID: "h1", HandlerUnknown: true},
	}
	builder.indexes = []groupindex.Index{index}
	section := builder.byProgram["t1"]
	section.programTargetID = "t1"
	builder.data.Orientation = &orientation.Result{MainFlow: orientation.MainFlow{Steps: []orientation.FlowStep{
		{TargetID: "t1", SubjectID: "cron"}, {TargetID: "t1", SubjectID: "process", Via: "called"}, {TargetID: "t1", SubjectID: "serve", Via: "called"},
		{TargetID: "t1", SubjectID: "h2", Via: "called"}, {TargetID: "t1", SubjectID: "h1", Via: "called"},
	}}}
	flow, _ := builder.flow(section)
	if flow == nil || len(flow.Steps) != 5 {
		t.Fatalf("flow = %+v", flow)
	}
	var said []string
	for _, step := range flow.Steps {
		line := step.Label + " [" + step.PartHead + "]"
		if step.TypeLine != "" {
			line += " " + step.TypeName + " — " + step.TypeLine
		}
		for _, handles := range step.Handles {
			var names []string
			for _, name := range handles.Names {
				names = append(names, name.Name+"@"+name.Input)
			}
			line += fmt.Sprintf(" %s %s folded=%v", handles.Words, strings.Join(names, ","), handles.Folded)
		}
		said = append(said, line)
	}
	want := []string{
		"serverCron [#t1-g1]",
		"Server.process [] Server — The server's state and its event loop.",
		"Server.serve []",
		"setCommand [#t1-g5] handles the request set@t1-o1 folded=false",
		"getCommand [] handles the requests: get@t1-o2,mget@t1-o3 folded=true",
	}
	if !slices.Equal(said, want) {
		t.Fatalf("the flow reads:\n%s\nwant\n%s", strings.Join(said, "\n"), strings.Join(want, "\n"))
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: flow}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		words := map[DisplayLanguage][]string{
			English: {"handles the request <code class=\"flow-input\" data-input=\"t1-o1\">set</code>", "<summary>handles the requests:</summary>",
				"The path stops here. At each step it follows one call the step before may make; open a step for all of its calls."},
			Russian: {"обрабатывает запрос <code class=\"flow-input\" data-input=\"t1-o1\">set</code>", "<summary>обрабатывает запросы:</summary>",
				"Путь здесь заканчивается. На каждом шаге он идёт по одному вызову, который может сделать предыдущий шаг; откройте шаг, чтобы увидеть все его вызовы."},
		}[language]
		words = append(words, `<li class="flow-part-head" data-flow-part="#t1-g1"></li><li class="flow-step"`, `<li class="flow-part-head" data-flow-part="#t1-g5"></li>`,
			`<span class="model flow-type"><code>Server</code> — <span data-display-ref="">The server&#39;s state and its event loop.</span></span>`)
		for _, word := range words {
			if !strings.Contains(html, word) {
				t.Fatalf("%v: the flow does not read %q: %s", language, word, html)
			}
		}
		if strings.Count(html, "flow-part-head") != 2 || strings.Count(html, "flow-type") != 1 || strings.Contains(html, "maxmemory") {
			t.Fatalf("%v: a part, type or input is said twice, or an unhandled input is: %s", language, html)
		}
	}
	// A way goes on from the step where the flow parts: its first step in
	// that step's part and type says neither again (othello's two ways on
	// from choose, both in AI search, had each stood under AI search).
	builder.data.Orientation.MainFlow.Steps = []orientation.FlowStep{{TargetID: "t1", SubjectID: "cron"}, {TargetID: "t1", SubjectID: "process", Via: "called",
		Paths: []orientation.FlowPath{{Steps: []orientation.FlowStep{{TargetID: "t1", SubjectID: "serve", Via: "called"}}}, {Steps: []orientation.FlowStep{{TargetID: "t1", SubjectID: "h2", Via: "called"}}}}}}
	parted, _ := builder.flow(section)
	if len(parted.Steps) != 2 || len(parted.Steps[1].Ways) != 2 {
		t.Fatalf("parted flow = %+v", parted)
	}
	same, other := parted.Steps[1].Ways[0].Head, parted.Steps[1].Ways[1].Head
	if parted.Steps[1].TypeLine == "" || same.PartHead != "" || same.TypeLine != "" || other.PartHead != "#t1-g5" {
		t.Fatalf("the ways' heads: same part %q type %q, other part %q", same.PartHead, same.TypeLine, other.PartHead)
	}
}

// A walked flow says how a step is reached as its code does, and a
// dispatch site by the function holding it, a name read as a step's is,
// never a file and line (owner: no line numbers in the column; redis's
// flow had read "one of 94 at redis.c:2054"), and one of the callables a
// call through a value may run in words, never a count: "one of the calls
// serverCron may make" (owner: no digits in the column). A named fork is
// one folded line of its candidates, read the same way, or "one of the
// calls it may make" when they share no site.
func TestAFlowsViaAndForkNameTheSitesFunctionNotItsLine(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "h2", Via: "one of 2", Site: "cron"}, section, map[string]bool{})
	fork := builder.flowFork(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Branches: []orientation.FlowBranch{
		{SubjectID: "h1", Via: "one of 2", Site: "cron"}, {SubjectID: "h2", Via: "one of 2", Site: "cron"}}})
	var names []string
	for _, name := range fork.Names {
		names = append(names, name.Name+" "+name.Part)
	}
	if step.ViaFrom == nil || step.ViaFrom.Name != "serverCron" || !step.OneOf || step.Via != "" || fork.From == nil || fork.From.Name != "serverCron" || !fork.OneOf ||
		!slices.Equal(names, []string{"getCommand #t1-g5", "setCommand #t1-g5"}) {
		t.Fatalf("step via %q from %+v; fork %+v, %q", step.Via, step.ViaFrom, fork, names)
	}
	step.Fork = fork
	mixed := builder.flowFork(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Branches: []orientation.FlowBranch{
		{SubjectID: "h1", Via: "called"}, {SubjectID: "save", Via: "handed to aeCreateTimeEvent"}}})
	if !mixed.OneOf || mixed.From != nil || len(mixed.Names) != 2 {
		t.Fatalf("mixed fork = %+v", mixed)
	}
	unsited := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "h1", Via: "one of 2"}, section, map[string]bool{})
	unsited.Fork = mixed
	words := map[DisplayLanguage][3]string{
		English: {"one of the calls serverCron may make", "one of the calls the step before may make", "one of the calls it may make"},
		Russian: {"один из вызовов, которые может сделать serverCron", "один из вызовов, которые может сделать предыдущий шаг", "один из вызовов, которые он может сделать"},
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step, unsited}}}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		read := func(from, to string) []string {
			var said []string
			for _, part := range strings.Split(html, from)[1:] {
				end := strings.Index(part, to)
				if end < 0 {
					t.Fatalf("%s is not closed: %s", from, part)
				}
				said = append(said, regexp.MustCompile(`<[^>]+>`).ReplaceAllString(part[:end], ""))
			}
			return said
		}
		vias, summaries := read(`<span class="flow-via">`, `</span></span>`), read(`<details class="flow-fork"><summary>`, `</summary>`)
		want := words[language]
		if len(vias) != 2 || !strings.HasPrefix(vias[0], want[0]) || !strings.HasPrefix(vias[1], want[1]) ||
			!slices.Equal(summaries, []string{want[0], want[2]}) {
			t.Fatalf("%v: via %q, forks %q, want %q", language, vias, summaries, want)
		}
		for _, text := range append(vias, summaries...) {
			if regexp.MustCompile(`\d`).MatchString(text) {
				t.Fatalf("%v: a flow row prints a digit: %q", language, text)
			}
		}
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

// A flow parting where the model is torn reads as its trunk, then, under
// the step where it parts, each way a short path of its own: a way of a few
// steps shows them, a long one its first step with the rest folded under
// one line; the candidates no way follows stay one folded line of names,
// one per line (owner, 2026-09-30).
func TestAPartedFlowReadsAsItsTrunkThenEachWay(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	step := func(id string) orientation.FlowStep {
		return orientation.FlowStep{TargetID: "t1", SubjectID: id, Via: "called"}
	}
	fork := orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Via: "called",
		Paths: []orientation.FlowPath{
			{Steps: []orientation.FlowStep{step("h1")}},
			{Steps: []orientation.FlowStep{step("h2"), step("lookup"), step("resize"), step("x4")}},
		},
		Branches: []orientation.FlowBranch{{SubjectID: "save", Via: "called"}, {SubjectID: "log", Via: "called"}}}
	row := builder.flowStep(fork, section, map[string]bool{})
	row.Ways = builder.flowWays(fork, section, map[string]bool{})
	row.Fork = builder.flowFork(fork)
	if len(row.Ways) != 2 || row.Ways[0].Head.Label != "getCommand" || row.Ways[0].Folded ||
		row.Ways[1].Head.Label != "setCommand" || !row.Ways[1].Folded || len(row.Ways[1].Rest) != 3 {
		t.Fatalf("ways = %+v", row.Ways)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{},
		Flow: &pageFlow{Steps: []pageFlowStep{{Label: "main"}, row}}}); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Count(html, `<ol class="flow flow-way">`) != 2 || strings.Count(html, `<details class="flow-way-rest" data-folded>`) != 1 ||
		!strings.Contains(html, "<summary>the rest of this way</summary>") || strings.Count(html, `<span class="flow-fork-name">`) != 2 {
		t.Fatalf("the parted flow does not read as its trunk and ways:\n%s", html)
	}
	// The trunk's steps come before the ways, which come before the fold.
	if !(strings.Index(html, ">serverCron<") < strings.Index(html, `class="flow-ways"`) && strings.Index(html, `class="flow-ways"`) < strings.Index(html, `class="flow-fork"`)) {
		t.Fatalf("the parted flow's order is wrong:\n%s", html)
	}
}

// How the step before reaches a Main flow step reads in the page's
// language, through the vocabulary: on a Russian page "called" and "handed
// to quil.core.sketch.setup" had stood in English, the walk's saved words.
func TestAFlowsViaReadsInThePagesLanguage(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	var steps []pageFlowStep
	for _, via := range []string{"called", "handed to quil.core.sketch.setup", "handed over"} {
		steps = append(steps, builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "h1", Via: via}, section, map[string]bool{}))
	}
	want := map[DisplayLanguage][]string{
		English: {"called", "handed to quil.core.sketch.setup", "handed over"},
		Russian: {"вызывается", "передаётся в quil.core.sketch.setup", "передаётся"},
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: &pageFlow{Steps: steps}}); err != nil {
			t.Fatal(err)
		}
		var said []string
		for _, match := range regexp.MustCompile(`<span class="flow-via">([^<]*)</span>`).FindAllStringSubmatch(out.String(), -1) {
			said = append(said, match[1])
		}
		if !slices.Equal(said, want[language]) {
			t.Fatalf("%v: the steps are reached %q, want %q", language, said, want[language])
		}
	}
}

// etcd's local_request_Election_Campaign_0 calls server.Campaign at
// gw/v3election.pb.gw.go:62: an interface's method, saved unresolved with
// no callee and its witness naming ElectionServer.Campaign. The flow had
// dropped it, its reading listing only NewDecoder, Decode, Is and Errorf.
// It stands named as written, where it is written, its implementation not
// established; none is invented.
func TestAFlowNamesACallNoImplementationIsEstablishedFor(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{{ID: "g1", Title: "Election", MemberSubjectIDs: []string{"n1"}}}}
	at := &programindex.Location{Path: "gw/v3election.pb.gw.go", Line: 62, Column: 29}
	b := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{{Target: programindex.Target{ID: "t1"},
		Relations: []programindex.Relation{{ID: "e26194", Kind: programindex.RelationCalls, FromID: "n1", Resolution: programindex.ResolutionUnresolved, Dispatch: "interface", Location: at,
			Witnesses: []programindex.Witness{{Kind: "go_ssa_dynamic_handoff", Detail: "go.etcd.io/etcd/server/v3/etcdserver/api/v3election/v3electionpb.ElectionServer.Campaign func(context.Context) error"}}}}}}}},
		subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://example.test/etcd", blobPrefix: "/blob/", revision: "r"}}
	calls := b.flowOf(&index, "n1", func(string) int { return -1 }, nil)
	if len(calls) != 1 || calls[0].Name != "v3electionpb.ElectionServer.Campaign" || !calls[0].Unresolved || len(calls[0].Sites) != 1 ||
		calls[0].Sites[0].Href != "https://example.test/etcd/blob/r/gw/v3election.pb.gw.go#L62" {
		t.Fatalf("the unresolved call: %+v", calls)
	}
}

// Lua's Main flow reaches lua_pcallk from handle_script through docall, a
// helper never a step of its own (orientation FlowStep.Through). The column
// had read "lua_pcallk (called)" without saying so. The step says the
// helper it is reached through, by name, read as a step's name is; a split's
// candidate the same way.
func TestAMainFlowStepSaysTheHelperItIsReachedThrough(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Via: "called", Through: []string{"h1"},
		Passed: []orientation.FlowBranch{{SubjectID: "h2", Via: "called", Through: []string{"h1"}}}}, section, map[string]bool{})
	if len(step.Through) != 1 || step.Through[0].Name != "getCommand" || step.Passed == nil || len(step.Passed.Names[0].Through) != 1 {
		t.Fatalf("through: step %+v, passed %+v", step.Through, step.Passed)
	}
	for language, word := range map[DisplayLanguage]string{English: "through", Russian: "через"} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step}}}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		via := html[strings.Index(html, `<span class="flow-via">`):]
		via = via[:strings.Index(via, "</span>")+7]
		if !strings.Contains(via, " "+word+" ") || !strings.Contains(via, `class="flow-chain-name"`) || !strings.Contains(via, "getCommand") {
			t.Fatalf("%v: the step does not say it is reached through getCommand: %s", language, via)
		}
		if strings.Count(html, " "+word+" ") != 2 {
			t.Fatalf("%v: the passed candidate does not say it either: %s", language, html)
		}
	}
}

// etcd's gateway calls server.Campaign on a value no observed flow gives: its
// targets are the methods the repository's types implementing
// ElectionServer declare (ProgramIndex Relation.Basis "implements"). The
// page reads the call as the interface's method implemented in this
// repository by them, by method set, never as a traced call: in the Calls
// column, among an entry's own calls and on a Main flow step and its
// passed candidates, a single implementation included, in both languages.
func TestACallKnownByItsInterfacesImplementationsSaysSo(t *testing.T) {
	at := &programindex.Location{Path: "gw/v3election.pb.gw.go", Line: 62, Column: 29}
	witness := []programindex.Witness{{Kind: "go_ssa_dynamic_handoff", Detail: "go.etcd.io/etcd/server/v3/etcdserver/api/v3election/v3electionpb.ElectionServer.Campaign func(context.Context) error"}}
	relation := programindex.Relation{ID: "e27300", Kind: programindex.RelationCalls, FromID: "n1", ToIDs: []string{"n2", "n3"}, Resolution: programindex.ResolutionAlternatives,
		Dispatch: programindex.DispatchInterface, Basis: programindex.BasisImplements, Location: at, Witnesses: witness}
	edge := func(to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: "n1", ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationID: "e27300", RelationKind: programindex.RelationCalls,
			Resolution: programindex.ResolutionAlternatives, Location: at, Basis: programindex.BasisImplements}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{{ID: "g1", Title: "Election", MemberSubjectIDs: []string{"n1", "n2", "n3"}}},
		StructuralEdges: []groupindex.StructuralEdge{edge("n2"), edge("n3")}}
	b := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{{Target: programindex.Target{ID: "t1"},
		Objects: []programindex.Object{{ID: "n1", Name: "local_request_Election_Campaign_0"}, {ID: "n2", Name: "Campaign", OwnerID: "n4"}, {ID: "n3", Name: "Campaign", OwnerID: "n5"},
			{ID: "n4", Name: "electionServer"}, {ID: "n5", Name: "electionProxy"}},
		Relations: []programindex.Relation{relation}}}}},
		subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://example.test/etcd", blobPrefix: "/blob/", revision: "r"}}
	declared := map[string]int{"n2": 0, "n3": 1}
	calls := b.flowOf(&index, "n1", func(id string) int {
		if position, ok := declared[id]; ok {
			return position
		}
		return -1
	}, nil)
	if len(calls) != 1 || calls[0].Implements != "v3electionpb.ElectionServer.Campaign" || !slices.Equal(calls[0].One, []int{0, 1}) || calls[0].Unresolved {
		t.Fatalf("the implemented call: %+v", calls)
	}
	own, _ := b.ownCalls("t1", "n1")
	if len(own) != 1 || own[0].Anchor.Text != "v3electionpb.ElectionServer.Campaign" || !slices.Equal(own[0].Implementers, []string{"electionServer.Campaign", "electionProxy.Campaign"}) || own[0].Unresolved {
		t.Fatalf("the entry's own call: %+v", own)
	}

	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Via: "one of 2", Basis: programindex.BasisImplements,
		Passed: []orientation.FlowBranch{{SubjectID: "h2", Via: "one of 2", Basis: programindex.BasisImplements}}}, section, map[string]bool{})
	if !step.Implemented || step.Passed == nil || !step.Passed.Names[0].Implemented {
		t.Fatalf("the step's basis: %+v, passed %+v", step, step.Passed)
	}
	for language, words := range map[DisplayLanguage][2]string{
		English: {"an implementation in this repository, by method set, not a traced call", "(by method set)"},
		Russian: {"реализация в этом репозитории, по набору методов, а не прослеженный вызов", "(по набору методов)"},
	} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "etcd", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step}}}); err != nil {
			t.Fatal(err)
		}
		if html := out.String(); !strings.Contains(html, words[0]) || !strings.Contains(html, words[1]) {
			t.Fatalf("%v: the step or its passed candidate does not say how it is known: %s", language, html)
		}
	}
}

// One callee reached once by a traced call and once as the one
// implementation of an interface no observed value fills stands twice, each
// row with its own site and how it is known, whichever the code lists first.
func TestATracedCallAndAnImplementationOfTheSameCalleeKeepTheirBasis(t *testing.T) {
	traced := &programindex.Location{Path: "server.go", Line: 10, Column: 2}
	implemented := &programindex.Location{Path: "server.go", Line: 20, Column: 2}
	witness := []programindex.Witness{{Kind: "go_ssa_dynamic_handoff", Detail: "example.com/app.Store.Put func(string)"}}
	relations := []programindex.Relation{
		{ID: "e1", Kind: programindex.RelationCalls, FromID: "n1", ToIDs: []string{"n2"}, Resolution: programindex.ResolutionExact, Location: traced, Witnesses: witness},
		{ID: "e2", Kind: programindex.RelationCalls, FromID: "n1", ToIDs: []string{"n2"}, Resolution: programindex.ResolutionExact, Dispatch: programindex.DispatchInterface,
			Basis: programindex.BasisImplements, Location: implemented, Witnesses: witness},
	}
	edge := func(relation programindex.Relation) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: "n1", ToSubjectID: "n2", Role: groupindex.EdgeRelationTarget, RelationID: relation.ID, RelationKind: programindex.RelationCalls,
			Resolution: relation.Resolution, Location: relation.Location, Basis: relation.Basis}
	}
	for _, order := range [][]int{{0, 1}, {1, 0}} {
		index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{{ID: "g1", Title: "Store", MemberSubjectIDs: []string{"n1", "n2"}}},
			StructuralEdges: []groupindex.StructuralEdge{edge(relations[order[0]]), edge(relations[order[1]])}}
		b := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{{Target: programindex.Target{ID: "t1"},
			Objects: []programindex.Object{{ID: "n1", Name: "save"}, {ID: "n2", Name: "Put"}}, Relations: relations}}}},
			subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://example.test/app", blobPrefix: "/blob/", revision: "r"}}
		calls := b.flowOf(&index, "n1", func(id string) int {
			if id == "n2" {
				return 0
			}
			return -1
		}, nil)
		said := map[string]string{}
		for _, call := range calls {
			if call.Decl == nil || *call.Decl != 0 || len(call.Sites) != 1 {
				t.Fatalf("order %v: a row is not the callee at one site: %+v", order, calls)
			}
			said[call.Sites[0].At] = call.Implements
		}
		if len(calls) != 2 || said["server.go:10"] != "" || said["server.go:20"] != "app.Store.Put" {
			t.Fatalf("order %v: rows %+v read %v", order, calls, said)
		}
	}
}

// A Main flow step says what the call reaching it runs under, with the
// construct's place, a candidate beside it the same way, and the last step
// why the path ends there, in both languages (control review, 2026-10-03:
// Lua's forprep reaches the collector only through luaG_runerror).
func TestAMainFlowStepSaysWhatItRunsUnderAndWhyThePathStops(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "lvm.c", Line: line, Column: 5}
	}
	step := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "cron", Via: "called",
		Guard: &programindex.Guard{Kind: programindex.GuardBranch, Location: at(218)}, Loop: at(1180), Stop: orientation.StopFailureOnly, OpenAt: at(462),
		Passed: []orientation.FlowBranch{{SubjectID: "h2", Via: "called", Guard: &programindex.Guard{Kind: programindex.GuardNoReturn, Location: at(223)}}}}, section, map[string]bool{})
	if step.Guard == nil || step.Loop == nil || step.Stop == "" || step.Passed == nil || step.Passed.Names[0].Guard == nil {
		t.Fatalf("the step's guard, loop or stop is lost: %+v", step)
	}
	for language, words := range map[DisplayLanguage][]string{
		English: {"only under a condition", "in a loop", "never returns", "This route ends here: the next steps identified for this route are on failure paths.",
			"it calls through a value whose target is not established"},
		Russian: {"только при условии", "в цикле", "не возвращает управление", "найденные следующие шаги этого пути идут по путям отказа",
			"вызывает через значение, цель которого не установлена"},
	} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "lua", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{step}}}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		for _, word := range words {
			if !strings.Contains(html, word) {
				t.Fatalf("%v: the step does not say %q: %s", language, word, html)
			}
		}
		if !strings.Contains(html, "lvm.c:223") || !strings.Contains(html, "lvm.c:1180") || !strings.Contains(html, "lvm.c:462") {
			t.Fatalf("%v: a guard or loop lost its place: %s", language, html)
		}
	}
}

// A saved flow's code title ("From main to <where the walk stopped>") is
// never shown: the parts line is the flow's only title, in both languages,
// and an orientation saved with one still renders (control review and
// skeptic, 2026-10-03: "From main to propagatemark or lua_getfenv" read as
// where the program goes).
func TestAMainFlowIsTitledByItsPartsAlone(t *testing.T) {
	builder, index := flowFixture()
	builder.indexes = []groupindex.Index{index}
	section := builder.byProgram["t1"]
	section.programTargetID = "t1"
	builder.data.Orientation = &orientation.Result{MainFlow: orientation.MainFlow{Title: "From serverCron to setCommand", Steps: []orientation.FlowStep{
		{TargetID: "t1", SubjectID: "cron"}, {TargetID: "t1", SubjectID: "h2", Via: "called"}}}}
	flow, _ := builder.flow(section)
	if flow == nil || len(flow.Parts) != 2 {
		t.Fatalf("flow = %+v", flow)
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "redis-server", Map: &pageMap{}, Flow: flow}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		if !strings.Contains(html, "Server lifecycle and cron</a> → <a") || !strings.Contains(html, ">String commands</a>") || strings.Contains(html, "From serverCron") ||
			strings.Count(html, "flow-title") != 1 {
			t.Fatalf("%v: the flow is not titled by its parts alone: %s", language, html)
		}
	}
}

// A way going on as others says each, with how the step reaches it: two
// joins end a way as either, and a join beside a way of the step's own is
// read after that way, in both languages (control review, 2026-10-03: H
// torn between two ways' starts kept only one).
func TestAMainFlowWaySaysEveryWayItGoesOnAs(t *testing.T) {
	builder, _ := flowFixture()
	section := builder.byProgram["t1"]
	guard := &programindex.Guard{Kind: programindex.GuardBranch, Location: &programindex.Location{Path: "ldo.c", Line: 377, Column: 3}}
	joins := []orientation.FlowBranch{{SubjectID: "cron", Via: "called", Guard: guard}, {SubjectID: "h2", Via: "called", Through: []string{"h1"}}}
	ended := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "h3", Via: "called", Stop: orientation.StopJoins, Joins: joins}, section, map[string]bool{})
	torn := builder.flowStep(orientation.FlowStep{TargetID: "t1", SubjectID: "h3", Via: "called", Stop: orientation.StopTorn, Joins: joins[:1]}, section, map[string]bool{})
	if len(ended.Joins) != 2 || ended.Joins[0].Guard == nil || len(ended.Joins[1].Through) != 1 || len(torn.Joins) != 1 || torn.Stop == "" {
		t.Fatalf("a join or its reach is lost: %+v / %+v", ended, torn)
	}
	for language, words := range map[DisplayLanguage][]string{
		English: {"From here this route goes on as the way from", " or ", "only under a condition", "From here this route also goes on as the way from"},
		Russian: {"Дальше этот путь идёт как путь от", " или ", "только при условии", "Отсюда этот путь идёт и как путь от"},
	} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "target.html", &pageSection{ID: "t1", ShortLabel: "lua", Map: &pageMap{}, Flow: &pageFlow{Steps: []pageFlowStep{ended, torn}}}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		for _, word := range words {
			if !strings.Contains(html, word) {
				t.Fatalf("%v: the joins do not say %q: %s", language, word, html)
			}
		}
		if !strings.Contains(html, "ldo.c:377") || strings.Count(html, ">"+ended.Joins[1].Name+"<") < 1 {
			t.Fatalf("%v: a join lost its place or its name: %s", language, html)
		}
	}
}

// A Main flow's parts line names the parts its steps stand in, each once in
// the order the path enters them; one way alone entering new parts reads as
// the path, a way going on as another adds nothing, and ways going on apart
// read as alternatives, never as a sequence (control review, 2026-10-03:
// "From main to forprep or luaD_hook" asked a newcomer to know Lua's
// internals; Lua 5.1.5's collector and libraries ways are not one after the
// other).
func TestAMainFlowNamesThePartsItPassesThrough(t *testing.T) {
	groups := []groupindex.Group{{ID: "g1", Title: "Standalone interpreter"}, {ID: "g2", Title: "Core API"}, {ID: "g3", Title: "Virtual machine"},
		{ID: "g4", Title: "Collector"}, {ID: "g5", Title: "Libraries"}}
	builder := &pageBuilder{groupTitles: map[groupindex.Endpoint]string{}, indexes: []groupindex.Index{{Target: programindex.Target{ID: "t1"}, Groups: groups}}}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	part := func(group string) string { return "#" + groupAnchorID("t1", group) }
	step := func(group string) pageFlowStep { return pageFlowStep{Part: part(group)} }
	read := func(steps []pageFlowStep) string {
		var line strings.Builder
		for at, said := range builder.flowParts(section, steps) {
			switch {
			case said.Or:
				line.WriteString(" or ")
			case at > 0:
				line.WriteString(" → ")
			}
			line.WriteString(said.Title)
		}
		return line.String()
	}
	joined := step("g4")
	joined.Joins = []pageStepName{{Name: "lua_pcall"}}
	steps := []pageFlowStep{step("g1"), step("g1"), step("g2")}
	steps[2].Ways = []pageFlowWay{{Head: step("g3"), Rest: []pageFlowStep{step("g3")}}, {Head: step("g2"), Rest: []pageFlowStep{step("g2")}}, {Head: step("g1"), Rest: []pageFlowStep{joined}}}
	if got, want := read(steps), "Standalone interpreter → Core API → Virtual machine"; got != want {
		t.Fatalf("parts %q, want %q", got, want)
	}
	apart := []pageFlowStep{step("g1"), step("g2"), step("g3")}
	apart[2].Ways = []pageFlowWay{{Head: step("g4"), Rest: []pageFlowStep{step("g5")}}, {Head: step("g5")}, {Head: step("g4")}}
	if got, want := read(apart), "Standalone interpreter → Core API → Virtual machine → Collector or Libraries"; got != want {
		t.Fatalf("ways going on apart read %q, want %q", got, want)
	}
	if parts := builder.flowParts(section, []pageFlowStep{step("g1"), step("g1")}); parts != nil {
		t.Fatalf("one part names no parts line: %+v", parts)
	}
}
