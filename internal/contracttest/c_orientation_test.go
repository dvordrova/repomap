package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The orientation reads kvd's main by its own calls, in the order main writes
// them, from the graph an ordinary run builds: its places name each
// declaration by the object its program indexed (t1.n1), and a seed of the
// group index (n1 of t1) finds its place there. Looked up by the bare member
// ID, no member found one and every request went without member evidence;
// listed in the graph's order, main's external calls came first. Redis's main
// flow then put loadServerConfig before the initServerConfig main calls first.
// A seed's row is complete: the recipe needs every one of main's settings.
func TestCFixtureOrientationReadsMainsCallsInWrittenOrder(t *testing.T) {
	run := runKvdOrientation(t)
	var overview struct {
		Seeds []struct {
			Ref, Name, Anchor string
			Calls             []any
		}
	}
	if err := json.Unmarshal(run.asked.bodies(t)[0], &overview); err != nil {
		t.Fatal(err)
	}
	mainLine, _ := run.fixture.at(t, "kvd.c", "int main(int argc, char **argv) {", "")
	for _, seed := range overview.Seeds {
		if seed.Name != "main" || seed.Anchor != fmt.Sprintf("kvd.c:%d", mainLine) {
			continue
		}
		names, lines := callHeads(seed.Calls)
		// main's seventeen calls, as written: its three settings, the
		// --symbols branch, then the port.
		if want := []string{"stdlib.h.getenv", "stdlib.h.getenv", "stdlib.h.getenv", "string.h.strcmp", "printSymbols", "stdlib.h.atoi"}; len(names) != 17 || !slices.Equal(names[:6], want) || !slices.IsSorted(lines) {
			t.Fatalf("main's calls read %v at %v, want all 17 as written, starting %v", names, lines, want)
		}
		return
	}
	t.Fatalf("kvd's main is not a seed row: %+v", overview.Seeds)
}

// kvd's Main flow is walked by code from main (design skeptic, 2026-09-30):
// main hands acceptHandler to the event loop's loopCreateFileEvent, and the
// loop's one call through a file event's rfileProc, which the index leaves
// open, reaches each handler a file event is given (acceptHandler,
// readQueryFromClient) as one of them: a split, one question. Past it
// readQueryFromClient runs processInputBuffer and processCommand with no
// question; processCommand's dispatch over the command table is the next
// split. No declaration is on the path twice.
func TestCFixtureMainFlowWalksWhatMainHandsOver(t *testing.T) {
	run := runKvdOrientation(t)
	if bodies := run.asked.bodies(t); len(bodies) != 1 {
		t.Fatalf("the orientation asked the model %d times, want the overview alone", len(bodies))
	}
	// With no categorizer the flow ends at main's split, a named fork of
	// its candidates, acceptHandler among them as main hands it over.
	if path := flowPath(run.indexes, run.result.MainFlow); len(path) < 2 || path[0] != "main ()" ||
		!slices.Contains(path, "? acceptHandler (handed to loopCreateFileEvent)") || !slices.Contains(path, "? loopMain (called)") {
		t.Fatalf("the walk with no categorizer = %q", path)
	}
	preset := &flowPreset{choose: map[string]string{"main": "loopMain", "loopMain": "loopProcessEvents", "loopProcessEvents": "readQueryFromClient",
		"readQueryFromClient": "processInputBuffer", "processInputBuffer": "processCommand"}, tie: map[string]bool{"processCommand": true}}
	result := walkFixtureFlow(t, run.layer, run.indexes, run.graph, run.server, preset)
	want := []string{"main ()", "loopMain (called)", "loopProcessEvents (called)", "readQueryFromClient (one of 3 from loopProcessEvents)",
		"processInputBuffer (called)", "processCommand (called)",
		// The tie parts the flow: addReply and bgsaveCommand are ways of
		// their own, the other commands the fork's folded candidates.
		"1: addReply (called)", "1: ? sendReplyToClient (handed to loopCreateFileEvent)", "1: ? loopCreateFileEvent (called)", "1: ? sbAppend (called)",
		"2: bgsaveCommand (one of 6 from processCommand)",
		"? delCommand (one of 6 from processCommand)", "? keysCommand (one of 6 from processCommand)", "? pingCommand (one of 6 from processCommand)",
		"? getCommand (one of 6 from processCommand)", "? setCommand (one of 6 from processCommand)"}
	if got := flowPath(run.indexes, result.MainFlow); !slices.Equal(got, want) {
		t.Fatalf("kvd's flow = %q\nwant %q", got, want)
	}
	// One question per split, the loop's rfileProc among them, and the way
	// through addReply asks its own. The
	// categorizer reads the site's file and line; the flow keeps its
	// function, the reader's column printing no line.
	var steps []string
	for _, split := range preset.asked {
		steps = append(steps, split.step)
	}
	if !slices.Equal(steps, []string{"main", "loopMain", "loopProcessEvents", "readQueryFromClient", "processInputBuffer", "processCommand", "addReply"}) ||
		!slices.Equal(preset.asked[2].options, []string{"acceptHandler", "loopApiPoll", "readQueryFromClient", "sendReplyToClient"}) ||
		!strings.Contains(preset.asked[2].meanings[2], "reached: one of 3 at loop.c:58") {
		t.Fatalf("splits asked: %+v", preset.asked)
	}
	assertNoRepeats(t, result.MainFlow)
}

// callHeads reads each call tuple's called name and line.
func callHeads(calls []any) ([]string, []int) {
	var names []string
	var lines []int
	for _, call := range calls {
		head, _ := call.(string)
		if tuple, isList := call.([]any); isList {
			head, _ = tuple[0].(string)
		}
		head, _, _ = strings.Cut(head, " -> ")
		at := strings.LastIndex(head, "@")
		line, _ := strconv.Atoi(head[at+1:])
		names, lines = append(names, head[:at]), append(lines, line)
	}
	return names, lines
}

// The Makefile is kvd's manifest, as a go.mod or a package.json is: the
// recipe question reads what `make` builds by default, the rules a reader
// runs (`make test`, with its commands) and the CFLAGS its compiles use, each
// at its line, so a run recipe can cite the build instead of saying there
// is none.
func TestKvdOrientationReadsTheMakefileAsTheBuildsManifest(t *testing.T) {
	run := runKvdOrientation(t)
	var overview struct {
		Targets []struct {
			Name, Manifest string
		}
		Facts []struct {
			Kind, Anchor, Key, Value string
		}
	}
	if err := json.Unmarshal(run.asked.bodies(t)[0], &overview); err != nil {
		t.Fatal(err)
	}
	for _, target := range overview.Targets {
		if target.Manifest != "Makefile" {
			t.Fatalf("%s's manifest is %q, want Makefile", target.Name, target.Manifest)
		}
	}
	rows := map[string]string{}
	for _, fact := range overview.Facts {
		if fact.Kind == "manifest" {
			rows[fact.Key] = fact.Anchor + " " + fact.Value
		}
	}
	goal, _ := run.fixture.at(t, "Makefile", "all: kvd kvcli", "")
	test, _ := run.fixture.at(t, "Makefile", "test: kvd kvcli", "")
	flags, _ := run.fixture.at(t, "Makefile", "CFLAGS = ", "")
	for key, want := range map[string]string{
		"default_goal":    fmt.Sprintf("Makefile:%d all: kvd kvcli", goal),
		"variable.CFLAGS": fmt.Sprintf("Makefile:%d -std=c99 -O2 -g -Wall -D_DEFAULT_SOURCE -DLOOP_POLL", flags),
	} {
		if rows[key] != want {
			t.Fatalf("manifest row %s = %q, want %q (rows %v)", key, rows[key], want, rows)
		}
	}
	if row := rows["rule.test"]; !strings.HasPrefix(row, fmt.Sprintf("Makefile:%d kvd kvcli — runs: ./kvd &", test)) {
		t.Fatalf("make test reads %q", row)
	}
	if _, object := rows["rule.kvd.o"]; object {
		t.Fatal("an object's rule is no command a reader runs")
	}
}

type kvdOrientation struct {
	fixture cFixture
	graph   atlas.Graph
	indexes []groupindex.Index
	asked   *capturedOrientation
	layer   facts.Result
	server  string
	result  orientation.Result
}

// runKvdOrientation runs the orientation over kvd and kvcli as an ordinary
// run builds them, the overview choosing kvd's flow.
func runKvdOrientation(t *testing.T) kvdOrientation {
	t.Helper()
	fixture := loadCFixture(t)
	set := buildCSet(t, fixture, "c:kvd", "c:kvcli")
	server, client := set["c:kvd"], set["c:kvcli"]
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}})
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
	read, err := reading.Read(t.Context(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: true, Targets: metas,
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: preset.categorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{server.Target.ID: server, client.Target.ID: client}, read.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	noClaims, err := claims.Seal(claims.Result{Revision: "test", Claims: []claims.Claim{}})
	if err != nil {
		t.Fatal(err)
	}
	asked := &capturedOrientation{flowTarget: server.Target.ID}
	result, _, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, asked,
		orientation.Input{RepositoryName: "kvd", Facts: layer, Claims: noClaims, Groups: indexes, Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	return kvdOrientation{fixture: fixture, graph: graph, indexes: indexes, asked: asked, layer: layer, server: server.Target.ID, result: result}
}

// capturedOrientation keeps the orientation's one provider request, the
// overview, answering it by choosing flowTarget's Main flow when one is set;
// the flow is walked by code.
type capturedOrientation struct {
	flowTarget string
	mu         sync.Mutex
	users      [][]byte
}

func (*capturedOrientation) State() []byte { return []byte(`{"provider":"captured-orientation"}`) }

func (*capturedOrientation) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (asked *capturedOrientation) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	asked.mu.Lock()
	defer asked.mu.Unlock()
	if len(asked.users) == 1 {
		return llm.Completion{}, fmt.Errorf("the orientation was asked a second time")
	}
	asked.users = append(asked.users, slices.Clone(prepared.Bytes()))
	response := []byte(`{}`)
	if len(asked.users) == 1 && asked.flowTarget != "" {
		response = []byte(`{"main_flow_target":"` + asked.flowTarget + `"}`)
	}
	return llm.Completion{Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (asked *capturedOrientation) bodies(t *testing.T) [][]byte {
	t.Helper()
	asked.mu.Lock()
	defer asked.mu.Unlock()
	if len(asked.users) == 0 {
		t.Fatal("the orientation was not asked")
	}
	return asked.users
}
