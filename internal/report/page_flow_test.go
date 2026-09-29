package report

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
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

// A function's flow is its calls in the order they are written, each
// callee once with every place it is called; a dispatch site is one call;
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
		"rdbSaveBackground @1322", "lookupKeyRead (no part) @1350", "one of setCommand, getCommand @1360",
		"macro assert from assert.h @1370,1372", "macro redisAssert calling _redisAssert @1380",
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
