package contracttest

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/facts"
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
// nothing; the server connects nowhere.
func TestCFixturePresetReadingKeepsSharedSocketsWithTheProgramThatRunsThem(t *testing.T) {
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
}
