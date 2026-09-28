package contracttest

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

// cUnreachable names the functions a program's index proves it never runs,
// as file:name, sorted.
func cUnreachable(index programindex.Index) []string {
	var names []string
	for _, object := range index.Objects {
		if object.Unreachable {
			names = append(names, object.Location.Path+":"+object.Name)
		}
	}
	slices.Sort(names)
	return names
}

// buildCSet seals several fixture programs as one target set, t1..tN.
func buildCSet(t *testing.T, fixture cFixture, selectors ...string) map[string]programindex.Index {
	t.Helper()
	var indexes []programindex.Index
	for _, selector := range selectors {
		indexes = append(indexes, buildCIndex(t, fixture, selector))
	}
	rebound, err := programindex.RebindTargetSet(indexes)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]programindex.Index{}
	for position, selector := range selectors {
		result[selector] = rebound[position]
	}
	return result
}

// net.c is linked into both programs, like Redis's anet.c: the server only
// listens and the client only connects. loop.c is too, like Redis's adlist.c,
// and only the server runs it. Each program's index proves which of its
// shared functions it never runs, and what that code reads or calls out to
// stays with the program that runs it.
func TestCFixtureProvesWhatEachProgramNeverRuns(t *testing.T) {
	fixture := loadCFixture(t)
	set := buildCSet(t, fixture, "c:kvd", "c:kvcli")
	server, client := set["c:kvd"], set["c:kvcli"]
	dump := buildCIndex(t, fixture, "c:tools/dump.c")
	for _, want := range []struct {
		index programindex.Index
		names []string
	}{
		// The command functions only the table and staticsyms.h's integer
		// casts name still run: the table exists before main.
		{server, []string{"net.c:netConnect"}},
		{client, []string{"loop.c:loopCreate", "loop.c:loopCreateFileEvent", "loop.c:loopDeleteFileEvent", "loop.c:loopMain", "loop.c:loopProcessEvents",
			"loop.c:loopSetBeforeSleep", "loop.c:loopStop", "loop.c:oom", "loop_poll.c:loopApiAddEvent", "loop_poll.c:loopApiCreate", "loop_poll.c:loopApiPoll",
			"net.c:netListen", "strbuf.c:sbConsume"}},
		{dump, []string{"strbuf.c:sbConsume"}},
	} {
		if got := cUnreachable(want.index); !reflect.DeepEqual(got, want.names) {
			t.Errorf("%s never runs %v, want %v", want.index.Target.Name, got, want.names)
		}
	}

	targets := []facts.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: targets})
	if err != nil {
		t.Fatal(err)
	}
	// The backlog netListen reads is the server's setting, not the client's.
	var backlog []string
	for _, fact := range layer.OfKind(facts.KindConfigRead) {
		if fact.Key == "KVD_BACKLOG" {
			backlog = append(backlog, fact.TargetID)
		}
	}
	if !reflect.DeepEqual(backlog, []string{server.Target.ID}) {
		t.Fatalf("KVD_BACKLOG is read by %v, want only the server %s", backlog, server.Target.ID)
	}

	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	for name, unreached := range map[string][]string{"netListen": {client.Target.ID}, "netConnect": {server.Target.ID}, "sbAppend": nil} {
		path := "net.c"
		if name == "sbAppend" {
			path = "strbuf.c"
		}
		place := cSymbolPlace(t, graph, path, name)
		if !reflect.DeepEqual(place.Symbol.Unreached, unreached) || len(place.TargetIDs) != 2 {
			t.Errorf("%s is held by %v and unreached in %v, want both and %v", name, place.TargetIDs, place.Symbol.Unreached, unreached)
		}
	}
}

// kvd and kvcli read together, like redis-server and redis-cli: the socket
// symbols' roles make a listener of netListen's bind and listen and a
// client request of netConnect's connect, and each stays with the program
// that runs it, as does the setting netListen reads. The client listens on
// nothing; the server connects nowhere. The event loop both link is a part
// of the server's map and leaves the client's, which never runs it, as
// adlist.c's Linked list stays on redis-server's map and leaves redis-cli's.
// kvdPair is kvd and kvcli read together with the kvd preset, and their
// projected GroupsIndexes.
type kvdPair struct {
	server, client programindex.Index
	preset         *kvdPreset
	result         reading.Result
	indexes        map[string]groupindex.Index
}

func readKvdPair(t *testing.T) kvdPair {
	t.Helper()
	fixture := loadCFixture(t)
	set := buildCSet(t, fixture, "c:kvd", "c:kvcli")
	server, client := set["c:kvd"], set["c:kvcli"]
	targets := []facts.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: targets})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	preset := &kvdPreset{roles: map[string]map[string]string{"sys/socket.h.connect": {"talks": "client_request"}}}
	var metas []reading.TargetMeta
	for _, index := range []programindex.Index{server, client} {
		metas = append(metas, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."})
	}
	result, err := reading.Read(t.Context(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: true, Targets: metas,
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: preset.categorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Rejected {
		if row.Kind == "window_rejected" {
			t.Fatalf("a question was left unanswered: %+v", row)
		}
	}
	projected, err := groupindex.ProjectAtlas(map[string]programindex.Index{server.Target.ID: server, client.Target.ID: client}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	indexes := map[string]groupindex.Index{}
	for _, index := range projected {
		indexes[index.Target.Name] = index
	}
	return kvdPair{server: server, client: client, preset: preset, result: result, indexes: indexes}
}

func TestCFixturePresetReadingKeepsSharedSocketsWithTheProgramThatRunsThem(t *testing.T) {
	pair := readKvdPair(t)
	server, client, result := pair.server, pair.client, pair.result
	calls := map[string][]string{}
	for _, target := range result.Atlas.Targets {
		for _, boundary := range target.Boundaries {
			if boundary.Path == "net.c" {
				calls[target.Name] = append(calls[target.Name], strings.Join([]string{boundary.Caller, boundary.External, boundary.Direction, boundary.Kind}, " | "))
			}
		}
		slices.Sort(calls[target.Name])
	}
	want := map[string][]string{
		"kvd": {
			"netListen |  | out | config", // KVD_BACKLOG
			"netListen | socket.h.bind | in | listen_address",
			"netListen | socket.h.listen | in | listen_address",
		},
		"kvcli": {"netConnect | socket.h.connect | out | client_request"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("net.c's boundaries by program = %v, want %v", calls, want)
	}
	unreached := map[string][]string{}
	for _, target := range result.Atlas.Targets {
		for _, box := range target.Boxes {
			if box.Unreached {
				unreached[target.Name] = append(unreached[target.Name], box.Title)
			}
		}
	}
	if want := map[string][]string{"kvcli": {"loop.c", "loop_poll.c"}}; !reflect.DeepEqual(unreached, want) {
		t.Fatalf("parts off each program's map as never run = %v, want %v", unreached, want)
	}
	for _, index := range pair.indexes {
		program := map[string]programindex.Index{server.Target.ID: server, client.Target.ID: client}[index.Target.ID]
		checkUnreachedParts(t, program, index)
		grouped := slices.ContainsFunc(index.Groups, func(group groupindex.Group) bool { return group.Title == "loop.c" })
		if grouped != (index.Target.ID == server.Target.ID) {
			t.Fatalf("%s draws loop.c: %v", program.Target.Name, grouped)
		}
	}
}

// kvcli's inputs: --raw, an option main compares an element of its argument
// vector with, and the rows of its table of command names, asked once as a
// table read by lookupCommand, one catalogue looked up there. The same
// symbol's comparison of a command's own name ("bgsave") is no input: each
// call is asked on its own with where its arguments come from. The client
// connects to the server, so each row is asked which of the server's inputs
// it sends (K5); a row the reader leaves unmatched names none although the
// server has an input of the same word, and no row draws an arrow.
func TestCFixturePresetReadingNamesTheClientsOptionsAndCommands(t *testing.T) {
	pair := readKvdPair(t)
	client, server := pair.indexes["kvcli"], pair.indexes["kvd"]
	pair.preset.mu.Lock()
	entered, tables, peers, systems := slices.Clone(pair.preset.entered), slices.Clone(pair.preset.tables), slices.Clone(pair.preset.peers), slices.Clone(pair.preset.systems)
	pair.preset.mu.Unlock()
	// The package kvcli's connect goes through is asked its system once.
	if !slices.Equal(systems, []string{"sys/socket.h"}) {
		t.Fatalf("outside packages asked their system: %v", systems)
	}
	// Its counterpart is chosen among kvd's inputs whose code is known: an
	// input whose handler is not established (--symbols, the settings) is
	// no endpoint another program's call reaches.
	pair.preset.mu.Lock()
	offered := slices.Clone(pair.preset.connectOffered)
	pair.preset.mu.Unlock()
	if len(offered) == 0 {
		t.Fatal("the client's connect was offered no counterpart")
	}
	for _, values := range offered {
		if slices.Contains([]string{"--symbols", "port", "dbfilename"}, values) {
			t.Fatalf("the connect was offered the handler-less input %q among %q", values, offered)
		}
	}
	for _, call := range []string{
		`string.h.strcasecmp in main: 1: element "1" of parameter #2 argv of main; 2: "--raw"`,
		`string.h.strcasecmp in main: 1: field name of result of calling lookupCommand(argv[first]); 2: "bgsave"`,
	} {
		if !slices.Contains(entered, call) {
			t.Fatalf("%s was not asked on its own: %v", call, entered)
		}
	}
	if !slices.Contains(tables, "cmdTable read by [lookupCommand]") {
		t.Fatalf("kvcli's table of names was not asked with its reader: %v", tables)
	}
	names := map[string]string{}
	for _, subject := range client.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	serverOperations := map[string]string{}
	for _, operation := range server.Operations {
		serverOperations[operation.ID] = operation.Name
	}
	type input struct{ kind, name, by, sends string }
	var got []input
	for _, operation := range client.Operations {
		var sends []string
		for _, peer := range operation.Sends {
			if peer.TargetID != server.Target.ID {
				t.Fatalf("%s sends to %s", operation.Name, peer.TargetID)
			}
			sends = append(sends, serverOperations[peer.OperationID])
		}
		if !operation.HandlerUnknown || operation.SubjectID != "" {
			t.Fatalf("kvcli's %s has a handler", operation.Name)
		}
		got = append(got, input{kind: operation.Kind, name: operation.Name, by: names[operation.DeclaredBy], sends: strings.Join(sends, " ")})
	}
	slices.SortFunc(got, func(a, b input) int { return strings.Compare(a.name, b.name) })
	want := []input{
		{kind: "command", name: "--raw", by: "main"},
		{kind: "command", name: "bgsave", by: "cmdTable", sends: "bgsave"},
		{kind: "command", name: "del", by: "cmdTable"},
		{kind: "command", name: "get", by: "cmdTable", sends: "get"},
		{kind: "command", name: "keys", by: "cmdTable", sends: "keys"},
		{kind: "command", name: "ping", by: "cmdTable", sends: "ping"},
		{kind: "command", name: "set", by: "cmdTable", sends: "set"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kvcli's inputs = %+v\nwant %+v", got, want)
	}
	if !slices.Contains(peers, "del -> none") {
		t.Fatalf("the rows were asked %v", peers)
	}
	var table *groupindex.Catalogue
	for position := range client.Catalogues {
		if names[client.Catalogues[position].DeclaredBy] == "cmdTable" {
			table = &client.Catalogues[position]
		}
	}
	if table == nil || len(table.OperationIDs) != 6 || len(table.Readers) != 1 || names[table.Readers[0].SubjectID] != "lookupCommand" {
		t.Fatalf("kvcli's table catalogue: %+v", table)
	}
	for _, connection := range client.Connections {
		if connection.SourceKind == "catalogue" {
			t.Fatalf("a row of the table draws an arrow: %+v", connection)
		}
	}
}
