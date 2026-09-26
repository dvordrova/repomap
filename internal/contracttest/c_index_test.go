package contracttest

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

// buildCIndex projects one parsed fixture program into a sealed ProgramIndex.
func buildCIndex(t *testing.T, fixture cFixture, selector string) programindex.Index {
	t.Helper()
	_, index := buildCInput(t, fixture, selector)
	return index
}

func buildCInput(t *testing.T, fixture cFixture, selector string) (programindex.Input, programindex.Index) {
	t.Helper()
	result, err := cproject.Index(fixture.repository, fixture.parsed[selector])
	if err != nil {
		t.Fatalf("project %s: %v", selector, err)
	}
	input := result.Input
	index, err := programindex.New(input)
	if err != nil {
		t.Fatalf("seal %s: %v", selector, err)
	}
	assertProgramIndexRoundTrip(t, index)
	adaptertest.AssertSharedArtifact(t, input, index)
	return input, index
}

// cObject is the one declaration of kind named name whose definition is in
// path: two definitions (a header static seen by two units, or two statics
// of one name in one file) fail.
func cObject(t *testing.T, index programindex.Index, kind programindex.ObjectKind, name, path string) programindex.Object {
	t.Helper()
	var found []programindex.Object
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name && object.Location != nil && object.Location.Path == path {
			found = append(found, object)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s %s in %s: %d objects, want one", kind, name, path, len(found))
	}
	return found[0]
}

func cObjectCount(index programindex.Index, kind programindex.ObjectKind, name string) int {
	count := 0
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name {
			count++
		}
	}
	return count
}

// cRelationsAt are the relations of kind from one object that touch line in
// path, through their own location or one of their patterns.
func cRelationsAt(index programindex.Index, kind programindex.RelationKind, fromID, path string, line int) []programindex.Relation {
	var found []programindex.Relation
	for _, relation := range index.Relations {
		if relation.Kind != kind || relation.FromID != fromID {
			continue
		}
		at := relation.Location != nil && relation.Location.Path == path && relation.Location.Line == line
		for _, pattern := range relation.Patterns {
			at = at || pattern.Location != nil && pattern.Location.Path == path && pattern.Location.Line == line
		}
		if at {
			found = append(found, relation)
		}
	}
	return found
}

// fieldLiteral reads a callable_receiver_field detail, `name = "get"`.
func fieldLiteral(detail string) (string, string) {
	field, literal, _ := strings.Cut(detail, " = ")
	return strings.TrimSpace(field), strings.Trim(strings.TrimSpace(literal), `"`)
}

var cCommandRows = []struct{ name, function string }{
	{"get", "getCommand"}, {"set", "setCommand"}, {"del", "delCommand"},
	{"keys", "keysCommand"}, {"ping", "pingCommand"}, {"bgsave", "bgsaveCommand"},
}

func TestCFixtureIndexesTheServer(t *testing.T) {
	fixture := loadCFixture(t)
	index := buildCIndex(t, fixture, "c:kvd")
	main := cObject(t, index, programindex.ObjectFunction, "main", "kvd.c")
	if index.Target.Language != "c" || index.Target.Selector != "c:kvd" || len(index.Target.Seeds) != 1 ||
		index.Target.Seeds[0].ObjectID != main.ID || index.Target.Seeds[0].Kind != programindex.SeedCallable {
		t.Fatalf("target: %+v", index.Target)
	}

	// Identity: a static is internal and named by its file, so the two oom
	// functions stay apart; a header's static inline is one declaration
	// however many units include it; the backend's functions live in the
	// file that defines them, and the other backend is not in this build.
	if getCommand := cObject(t, index, programindex.ObjectFunction, "getCommand", "kvd.c"); getCommand.Visibility != programindex.VisibilityInternal {
		t.Fatalf("getCommand: %+v", getCommand)
	}
	if fail := cObject(t, index, programindex.ObjectFunction, "kvAssertFail", "kvd.c"); fail.Visibility != programindex.VisibilityPublic {
		t.Fatalf("kvAssertFail: %+v", fail)
	}
	cObject(t, index, programindex.ObjectFunction, "oom", "loop.c")
	cObject(t, index, programindex.ObjectFunction, "oom", "strbuf.c")
	cObject(t, index, programindex.ObjectFunction, "sbAvail", "strbuf.h")
	cObject(t, index, programindex.ObjectFunction, "loopApiPoll", "loop_poll.c")
	if cObjectCount(index, programindex.ObjectFunction, "oom") != 2 || cObjectCount(index, programindex.ObjectFunction, "sbAvail") != 1 ||
		cObjectCount(index, programindex.ObjectFunction, "loopApiPoll") != 1 {
		t.Fatal("a definition was duplicated or merged across files")
	}
	for _, object := range index.Objects {
		if object.Location != nil && object.Location.Path == "loop_epoll.c" {
			t.Fatalf("the backend outside this build was indexed: %+v", object)
		}
	}
	// typedef struct {...} strbuf is the type strbuf; typedef struct
	// kvCommand {...} kvCommand is one type, whose fields it contains.
	cObject(t, index, programindex.ObjectType, "strbuf", "strbuf.h")
	cObject(t, index, programindex.ObjectType, "loopFired", "loop.h")
	command := cObject(t, index, programindex.ObjectType, "kvCommand", "kvd.h")
	if cObjectCount(index, programindex.ObjectType, "kvCommand") != 1 {
		t.Fatal("typedef struct kvCommand {...} kvCommand became two types")
	}
	if proc := cObject(t, index, programindex.ObjectVariable, "proc", "kvd.h"); proc.ContainerID != command.ID {
		t.Fatalf("kvCommand.proc: %+v", proc)
	}

	// Every command table row hands its function over under the row's name.
	table := map[string]programindex.Object{}
	for _, row := range cCommandRows {
		function := cObject(t, index, programindex.ObjectFunction, row.function, "kvd.c")
		table[row.function] = function
		line, _ := fixture.at(t, "kvd.c", `{"`+row.name+`", `+row.function, "")
		var stores []programindex.Relation
		for _, relation := range index.Relations {
			if relation.Kind == programindex.RelationPassesCallback && slices.Equal(relation.ToIDs, []string{function.ID}) {
				stores = append(stores, relation)
			}
		}
		if len(stores) != 1 || stores[0].Resolution != programindex.ResolutionExact || len(stores[0].Witnesses) < 2 || stores[0].Witnesses[0].Kind != "c_function_pointer_store" {
			t.Fatalf("%s row: %+v", row.name, stores)
		}
		named := false
		for _, witness := range stores[0].Witnesses {
			if witness.Kind != "callable_receiver_field" {
				continue
			}
			field, literal := fieldLiteral(witness.Detail)
			if field != "name" || literal != row.name || witness.Location == nil || witness.Location.Path != "kvd.c" || witness.Location.Line != line {
				t.Fatalf("%s row names %q at %+v", row.name, witness.Detail, witness.Location)
			}
			named = true
		}
		if !named {
			t.Fatalf("%s row has no name witness: %+v", row.name, stores[0].Witnesses)
		}
	}
	// Functions cast to integers in staticsyms.h are data: no row there
	// hands a callable over.
	for _, relation := range index.Relations {
		for _, witness := range append([]programindex.Witness{{Location: relation.Location}}, relation.Witnesses...) {
			if witness.Location != nil && witness.Location.Path == "staticsyms.h" {
				t.Fatalf("an address cast to an integer became a relation: %+v", relation)
			}
		}
	}

	// cmd->proc(c) runs one of the functions the table stores.
	processCommand := cObject(t, index, programindex.ObjectFunction, "processCommand", "kvd.c")
	procLine, _ := fixture.at(t, "kvd.c", "cmd->proc(c);", "")
	dispatch := cRelationsAt(index, programindex.RelationCalls, processCommand.ID, "kvd.c", procLine)
	var commands []string
	for _, function := range table {
		commands = append(commands, function.ID)
	}
	sort.Strings(commands)
	if len(dispatch) != 1 || dispatch[0].Dispatch != programindex.DispatchFunctionValue || dispatch[0].Resolution != programindex.ResolutionAlternatives ||
		!slices.Equal(sortedIDs(dispatch[0].ToIDs), commands) {
		t.Fatalf("cmd->proc(c): %+v", dispatch)
	}

	// fe->rfileProc and fe->wfileProc are stored under a branch on mask:
	// which handler each field holds is not known, so both calls stay
	// unresolved and name every handler passed to loopCreateFileEvent.
	processEvents := cObject(t, index, programindex.ObjectFunction, "loopProcessEvents", "loop.c")
	for _, field := range []string{"rfileProc", "wfileProc"} {
		line, _ := fixture.at(t, "loop.c", "fe->"+field+"(l, fd, fe->data, mask)", "")
		calls := cRelationsAt(index, programindex.RelationCalls, processEvents.ID, "loop.c", line)
		if len(calls) != 1 || calls[0].Dispatch != programindex.DispatchFunctionValue || calls[0].Resolution != programindex.ResolutionUnresolved || len(calls[0].ToIDs) != 0 {
			t.Fatalf("fe->%s: %+v", field, calls)
		}
		var details []string
		for _, witness := range calls[0].Witnesses {
			details = append(details, witness.Detail)
		}
		for _, candidate := range []string{"acceptHandler", "readQueryFromClient", "sendReplyToClient"} {
			if !strings.Contains(strings.Join(details, "\n"), candidate) {
				t.Fatalf("fe->%s does not name the candidate %s: %q", field, candidate, details)
			}
		}
	}
	// l->beforeSleep has one unconditional store: the call is exact.
	loopMain := cObject(t, index, programindex.ObjectFunction, "loopMain", "loop.c")
	beforeSleep := cObject(t, index, programindex.ObjectFunction, "beforeSleep", "kvd.c")
	sleepLine, _ := fixture.at(t, "loop.c", "l->beforeSleep(l);", "")
	if calls := cRelationsAt(index, programindex.RelationCalls, loopMain.ID, "loop.c", sleepLine); len(calls) != 1 ||
		calls[0].Dispatch != programindex.DispatchFunctionValue || calls[0].Resolution != programindex.ResolutionExact || !slices.Equal(calls[0].ToIDs, []string{beforeSleep.ID}) {
		t.Fatalf("l->beforeSleep(l): %+v", calls)
	}

	// Handlers handed to the repository's own event loop are callbacks at
	// their registration argument.
	acceptHandler := cObject(t, index, programindex.ObjectFunction, "acceptHandler", "kvd.c")
	acceptLine, _ := fixture.at(t, "kvd.c", "LOOP_READABLE, acceptHandler, NULL)", "")
	assertCCallback(t, index, main, acceptHandler, "kvd.c", acceptLine, "loopCreateFileEvent")
	readQuery := cObject(t, index, programindex.ObjectFunction, "readQueryFromClient", "kvd.c")
	readLine, _ := fixture.at(t, "kvd.c", "LOOP_READABLE, readQueryFromClient, c)", "")
	assertCCallback(t, index, acceptHandler, readQuery, "kvd.c", readLine, "loopCreateFileEvent")
	stats := cObject(t, index, programindex.ObjectFunction, "statsWorker", "kvd.c")
	threadLine, _ := fixture.at(t, "kvd.c", "pthread_create(&stats, NULL, statsWorker, NULL)", "")
	assertCCallback(t, index, main, stats, "kvd.c", threadLine, "pthread_create")
	keys := cObject(t, index, programindex.ObjectFunction, "keysCommand", "kvd.c")
	compare := cObject(t, index, programindex.ObjectFunction, "compareKeys", "kvd.c")
	qsortLine, _ := fixture.at(t, "kvd.c", "qsort(keys,", "")
	assertCCallback(t, index, keys, compare, "kvd.c", qsortLine, "qsort")

	// act.sa_handler = onSignal hands onSignal to struct sigaction under the
	// field as the fixture writes it, not the platform's internal union.
	setupSignals := cObject(t, index, programindex.ObjectFunction, "setupSignals", "kvd.c")
	onSignal := cObject(t, index, programindex.ObjectFunction, "onSignal", "kvd.c")
	handlerLine, _ := fixture.at(t, "kvd.c", "act.sa_handler = onSignal;", "")
	var sigactionArgument string
	for _, relation := range cRelationsAt(index, programindex.RelationInvokesExternal, setupSignals.ID, "kvd.c", handlerLine) {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if argument.Keyword == "sa_handler" && slices.Equal(argument.ObjectIDs, []string{onSignal.ID}) {
					sigactionArgument = argument.ID
					symbol := cExternal(t, index, relation.ToIDs)
					if symbol.AuthorityKind != programindex.ExternalAuthorityPlatform || symbol.PackagePath != "signal.h" || strings.TrimPrefix(symbol.Name, "struct ") != "sigaction" {
						t.Fatalf("sa_handler store names %+v", symbol)
					}
				}
			}
		}
	}
	if sigactionArgument == "" {
		t.Fatal("act.sa_handler = onSignal is not a store into struct sigaction")
	}
	var handed bool
	for _, relation := range index.Relations {
		handed = handed || relation.Kind == programindex.RelationPassesCallback && relation.FromID == setupSignals.ID &&
			slices.Equal(relation.ToIDs, []string{onSignal.ID}) && relation.SourceArgumentID == sigactionArgument
	}
	if !handed {
		t.Fatal("onSignal is not handed over at its sa_handler store")
	}
	for _, object := range index.Objects {
		if object.External != nil && strings.Contains(object.External.Name, "__sigaction_u") {
			t.Fatalf("the platform's internal union leaked: %+v", object)
		}
	}

	// A call written in a macro argument is an ordinary call at its own
	// place; the call the macro body writes is at the macro name, with the
	// macro's definition as its witness.
	nonBlocking := cObject(t, index, programindex.ObjectFunction, "setNonBlocking", "kvd.c")
	assertFail := cObject(t, index, programindex.ObjectFunction, "kvAssertFail", "kvd.c")
	macroLine, nonBlockingColumn := fixture.at(t, "kvd.c", "kvAssert(setNonBlocking(cfd) == 0)", "setNonBlocking")
	_, macroColumn := fixture.at(t, "kvd.c", "kvAssert(setNonBlocking(cfd) == 0)", "")
	defineLine, _ := fixture.at(t, "kvd.h", "#define kvAssert(e)", "")
	assertCCallSite(t, index, acceptHandler, nonBlocking, "setNonBlocking", "kvd.c", macroLine, nonBlockingColumn)
	failure := assertCCallSite(t, index, acceptHandler, assertFail, "kvAssert", "kvd.c", macroLine, macroColumn)
	witnesses := slices.Clone(failure.Witnesses)
	for _, pattern := range failure.Patterns {
		witnesses = append(witnesses, pattern.Context...)
	}
	expansion := false
	for _, witness := range witnesses {
		expansion = expansion || witness.Kind == "macro_expansion" && witness.Location != nil && witness.Location.Path == "kvd.h" && witness.Location.Line == defineLine
	}
	if !expansion {
		t.Fatalf("kvAssert has no macro_expansion witness at kvd.h:%d: %+v", defineLine, failure.Witnesses)
	}

	// Platform calls name the header the fixture includes.
	for _, call := range []struct{ from, name, header, needle string }{
		{"main", "getenv", "stdlib.h", `getenv("KVD_PORT")`},
		{"main", "system", "stdlib.h", "system(hook)"},
		{"keysCommand", "qsort", "stdlib.h", "qsort(keys,"},
		{"addReplyBulk", "snprintf", "stdio.h", `snprintf(header, sizeof header`},
		{"bgsaveCommand", "fork", "unistd.h", "child = fork();"},
		{"listenOn", "socket", "sys/socket.h", "socket(AF_INET, SOCK_STREAM, 0)"},
	} {
		from := cObject(t, index, programindex.ObjectFunction, call.from, "kvd.c")
		line, _ := fixture.at(t, "kvd.c", call.needle, "")
		found := false
		for _, relation := range cRelationsAt(index, programindex.RelationInvokesExternal, from.ID, "kvd.c", line) {
			symbol := cExternal(t, index, relation.ToIDs)
			found = found || symbol.Name == call.name && symbol.PackagePath == call.header && symbol.AuthorityKind == programindex.ExternalAuthorityPlatform
		}
		if !found {
			t.Errorf("%s does not call %s from %s at kvd.c:%d", call.from, call.name, call.header, line)
		}
	}

	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	// A loop that never ends and a loop that runs until stopped.
	statsLoop, _ := fixture.at(t, "kvd.c", "while (1)", "")
	statsCall, _ := fixture.at(t, "kvd.c", "        reportStats();", "")
	adaptertest.AssertCallControls(t, index, graph, "kvd.c", "reportStats", map[int][]adaptertest.Control{
		statsCall: {{Line: statsLoop, Kind: "while body with constant true condition"}},
	})
	mainLoop, _ := fixture.at(t, "loop.c", "while (!l->stop)", "")
	eventsCall, _ := fixture.at(t, "loop.c", "loopProcessEvents(l);", "")
	adaptertest.AssertCallControls(t, index, graph, "loop.c", "loopProcessEvents", map[int][]adaptertest.Control{
		eventsCall: {{Line: mainLoop, Kind: "while body"}},
	})
	assertCTableBindings(t, fixture, graph)
	assertCHandedBindings(t, fixture, graph)
	assertCDispatchCalls(t, fixture, graph)
}

func sortedIDs(ids []string) []string {
	ids = slices.Clone(ids)
	sort.Strings(ids)
	return ids
}

func cExternal(t *testing.T, index programindex.Index, ids []string) programindex.ExternalSymbol {
	t.Helper()
	if len(ids) != 1 {
		t.Fatalf("external call targets: %v", ids)
	}
	for _, object := range index.Objects {
		if object.ID == ids[0] && object.External != nil {
			return *object.External
		}
	}
	t.Fatalf("%s is not an external symbol", ids[0])
	return programindex.ExternalSymbol{}
}

// assertCCallback checks that from hands callback over at one call of
// selector on line, through that call's own argument.
func assertCCallback(t *testing.T, index programindex.Index, from, callback programindex.Object, path string, line int, selector string) {
	t.Helper()
	arguments := map[string]bool{}
	for _, relation := range index.Relations {
		if relation.FromID != from.ID {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector != selector || pattern.Location == nil || pattern.Location.Path != path || pattern.Location.Line != line {
				continue
			}
			for _, argument := range pattern.Arguments {
				if slices.Contains(argument.ObjectIDs, callback.ID) {
					arguments[argument.ID] = true
				}
			}
		}
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.FromID == from.ID && slices.Equal(relation.ToIDs, []string{callback.ID}) && arguments[relation.SourceArgumentID] {
			return
		}
	}
	t.Fatalf("%s does not hand %s to %s at %s:%d", from.Name, callback.Name, selector, path, line)
}

// assertCCallSite returns the one call from caller to callee written as
// selector at line and column.
func assertCCallSite(t *testing.T, index programindex.Index, caller, callee programindex.Object, selector, path string, line, column int) programindex.Relation {
	t.Helper()
	var found []programindex.Relation
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != caller.ID || !slices.Equal(relation.ToIDs, []string{callee.ID}) {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector == selector && pattern.Location != nil && *pattern.Location == (programindex.Location{Path: path, Line: line, Column: column}) {
				found = append(found, relation)
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s calls %s as %s at %s:%d:%d %d times", caller.Name, callee.Name, selector, path, line, column, len(found))
	}
	return found[0]
}

func cSymbolPlace(t *testing.T, graph atlas.Graph, path, name string) atlas.Place {
	t.Helper()
	for _, place := range graph.Places {
		if place.Symbol != nil && place.Path == path && place.Symbol.Decl.Name == name {
			return place
		}
	}
	t.Fatalf("no symbol place for %s in %s", name, path)
	return atlas.Place{}
}

// Each command function's reading carries the table row that names it, with
// its own name only.
func assertCTableBindings(t *testing.T, fixture cFixture, graph atlas.Graph) {
	t.Helper()
	for _, row := range cCommandRows {
		line, _ := fixture.at(t, "kvd.c", `{"`+row.name+`", `+row.function, "")
		place := cSymbolPlace(t, graph, "kvd.c", row.function)
		var names []string
		for _, binding := range place.Symbol.Bindings {
			if binding.To != row.function || binding.Kind != string(programindex.RelationPassesCallback) {
				continue
			}
			for _, evidence := range binding.Evidence {
				if evidence.Extractor == "callable_receiver_field" && evidence.Path == "kvd.c" && evidence.LineNo == line {
					_, literal := fieldLiteral(evidence.Label)
					names = append(names, literal)
				}
			}
		}
		if !slices.Equal(names, []string{row.name}) {
			t.Fatalf("%s bindings name %q, want %q: %+v", row.function, names, row.name, place.Symbol.Bindings)
		}
	}
}

// Every function handed over at a call or a store keeps that binding in its
// own reading, at the place it is handed over: a store's own witness makes
// the row, so no handler depends on a field-name witness to appear.
func assertCHandedBindings(t *testing.T, fixture cFixture, graph atlas.Graph) {
	t.Helper()
	for _, handed := range []struct{ from, callback, needle string }{
		{"main", "acceptHandler", "LOOP_READABLE, acceptHandler, NULL)"},
		{"acceptHandler", "readQueryFromClient", "LOOP_READABLE, readQueryFromClient, c)"},
		{"addReply", "sendReplyToClient", "LOOP_WRITABLE, sendReplyToClient, c)"},
		{"main", "beforeSleep", "loopSetBeforeSleep(server.el, beforeSleep)"},
		{"main", "statsWorker", "pthread_create(&stats, NULL, statsWorker, NULL)"},
		{"keysCommand", "compareKeys", "qsort(keys,"},
		{"setupSignals", "onSignal", "act.sa_handler = onSignal;"},
	} {
		line, _ := fixture.at(t, "kvd.c", handed.needle, "")
		found := false
		for _, binding := range cSymbolPlace(t, graph, "kvd.c", handed.callback).Symbol.Bindings {
			if binding.From != handed.from || binding.To != handed.callback || binding.Kind != string(programindex.RelationPassesCallback) ||
				binding.Path != "kvd.c" || binding.Line != line {
				continue
			}
			for _, evidence := range binding.Evidence {
				found = found || evidence.Extractor == "callback_registration" && evidence.Path == "kvd.c" && evidence.LineNo == line
			}
		}
		if !found {
			t.Fatalf("%s's reading does not show %s handing it over at kvd.c:%d", handed.callback, handed.from, line)
		}
	}
}

// The readings of the two dispatching functions keep what the index knows:
// alternatives for the table, nothing chosen for the branch stores.
func assertCDispatchCalls(t *testing.T, fixture cFixture, graph atlas.Graph) {
	t.Helper()
	procLine, _ := fixture.at(t, "kvd.c", "cmd->proc(c);", "")
	var proc []atlas.SymbolCall
	for _, call := range cSymbolPlace(t, graph, "kvd.c", "processCommand").Symbol.Calls {
		if call.Line == procLine {
			proc = append(proc, call)
		}
	}
	if len(proc) != 1 || proc[0].Dispatch != programindex.DispatchFunctionValue || proc[0].Resolution != string(programindex.ResolutionAlternatives) || len(proc[0].CalleeIDs) != len(cCommandRows) {
		t.Fatalf("processCommand's dispatch: %+v", proc)
	}
	for _, field := range []string{"rfileProc", "wfileProc"} {
		line, _ := fixture.at(t, "loop.c", "fe->"+field+"(l, fd, fe->data, mask)", "")
		var calls []atlas.SymbolCall
		for _, call := range cSymbolPlace(t, graph, "loop.c", "loopProcessEvents").Symbol.Calls {
			if call.Line == line {
				calls = append(calls, call)
			}
		}
		if len(calls) != 1 || calls[0].Dispatch != programindex.DispatchFunctionValue || calls[0].Resolution != string(programindex.ResolutionUnresolved) || len(calls[0].CalleeIDs) != 0 {
			t.Fatalf("loopProcessEvents' %s dispatch: %+v", field, calls)
		}
	}
}

func TestCFixtureKeepsTheClientApart(t *testing.T) {
	fixture := loadCFixture(t)
	input, index := buildCInput(t, fixture, "c:kvcli")
	main := cObject(t, index, programindex.ObjectFunction, "main", "kvcli.c")
	if len(index.Target.Seeds) != 1 || index.Target.Seeds[0].ObjectID != main.ID {
		t.Fatalf("kvcli seeds: %+v", index.Target.Seeds)
	}
	// The client's own static table and lookup, of the same names as the
	// server's; its rows name no function, so nothing is handed over.
	cObject(t, index, programindex.ObjectFunction, "lookupCommand", "kvcli.c")
	cObject(t, index, programindex.ObjectVariable, "cmdTable", "kvcli.c")
	for _, object := range index.Objects {
		if object.Location != nil && (object.Location.Path == "kvd.c" || object.Location.Path == "loop.c") {
			t.Fatalf("the client indexed the server's %s", object.Name)
		}
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback {
			t.Fatalf("the client hands a callable over: %+v", relation)
		}
	}
	// strncmp(host, "kvd://", strlen("kvd://")) ... host + strlen("kvd://"):
	// different calls with the same address and the same call twice, nested
	// on one line. Each call is its own fact at its called name.
	const scheme = `return strncmp(host, "kvd://", strlen("kvd://")) == 0 ? host + strlen("kvd://") : host;`
	line, compare := fixture.at(t, "kvcli.c", scheme, "strncmp")
	_, measure := fixture.at(t, "kvcli.c", scheme, `strlen("kvd://")) == 0`)
	_, skip := fixture.at(t, "kvcli.c", scheme, `strlen("kvd://") : host`)
	adaptertest.AssertCallSiteBoundaries(t, fixture.repository, input, "kvcli.c", []adaptertest.CallSite{
		{Line: line, Column: compare, Key: "strncmp", Text: "string.h.strncmp", Path: "kvd://"},
		{Line: line, Column: measure, Key: "strlen", Text: "string.h.strlen", Path: "kvd://"},
		{Line: line, Column: skip, Key: "strlen", Text: "string.h.strlen", Path: "kvd://"},
	})
	// The dump tool takes what its closure links: strbuf.c's functions.
	dump := buildCIndex(t, fixture, "c:tools/dump.c")
	sbAppend := cObject(t, dump, programindex.ObjectFunction, "sbAppend", "strbuf.c")
	dumpMain := cObject(t, dump, programindex.ObjectFunction, "main", "tools/dump.c")
	var linked bool
	for _, relation := range dump.Relations {
		linked = linked || relation.Kind == programindex.RelationCalls && relation.FromID == dumpMain.ID && slices.Equal(relation.ToIDs, []string{sbAppend.ID}) && relation.Resolution == programindex.ResolutionExact
	}
	if !linked {
		t.Fatal("the dump tool's call to sbAppend does not reach strbuf.c")
	}
}

// Owner decision D1: a function stored in a row of the repository's own
// table that names it by a string literal is a registration, which the
// reading stage classifies like any other.
func TestCFixtureCommandRowsAreRegistrations(t *testing.T) {
	fixture := loadCFixture(t)
	index := buildCIndex(t, fixture, "c:kvd")
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string][]facts.Fact{}
	for _, fact := range result.OfKind(facts.KindRegistration) {
		registered[fact.Symbol] = append(registered[fact.Symbol], fact)
	}
	for _, row := range cCommandRows {
		line, _ := fixture.at(t, "kvd.c", `{"`+row.name+`", `+row.function, "")
		rows := registered[row.function]
		if len(rows) != 1 || rows[0].Anchor == nil || rows[0].Anchor.Path != "kvd.c" || rows[0].Anchor.Line != line || !slices.Contains(rows[0].Values, row.name) {
			t.Fatalf("%s registrations: %+v", row.function, rows)
		}
		// A command named get or del is no HTTP method: the row has no
		// address for a verb to qualify.
		if rows[0].Method != "" || rows[0].Path != "" {
			t.Fatalf("%s row states an HTTP request: %+v", row.function, rows[0])
		}
	}
	// The platform receives the thread body, the comparator and the signal
	// handler.
	for _, handed := range []string{"statsWorker", "compareKeys", "onSignal"} {
		if len(registered[handed]) != 1 {
			t.Fatalf("%s registrations: %+v", handed, registered[handed])
		}
	}
}
