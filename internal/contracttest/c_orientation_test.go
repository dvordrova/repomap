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

// kvd's Main flow is asked over what kvd runs: main and what it calls, the
// load-time code, each command's reach, and what a callable hands over:
// main hands acceptHandler to the event loop, which hands
// readQueryFromClient on, which runs processCommand. No call reaches them,
// so without the hand-over the flow could not go past loopMain. Every one
// is listed with all of its calls, in reading order.
func TestCFixtureMainFlowScopeFollowsWhatMainHandsOver(t *testing.T) {
	run := runKvdOrientation(t)
	bodies := run.asked.bodies(t)
	if len(bodies) != 2 {
		t.Fatalf("the orientation asked %d times, want the overview then the flow", len(bodies))
	}
	var flow struct {
		Target  struct{ Ref, Name string }
		Members []struct {
			Ref, Name, Anchor string
			Calls             []any
		}
	}
	if err := json.Unmarshal(bodies[1], &flow); err != nil {
		t.Fatal(err)
	}
	if flow.Target.Name != "kvd" {
		t.Fatalf("the flow was asked for %+v, not kvd", flow.Target)
	}
	at := map[string]int{}
	for position, member := range flow.Members {
		if _, repeated := at[member.Ref]; repeated {
			t.Fatalf("%s is listed twice", member.Ref)
		}
		at[member.Ref] = position
	}
	var index groupindex.Index
	for _, candidate := range run.indexes {
		if candidate.Target.Name == "kvd" {
			index = candidate
		}
	}
	subjects := map[string]groupindex.Subject{}
	byName := map[string]string{}
	for _, subject := range index.Subjects {
		subjects[subject.ID] = subject
		if subject.Object != nil {
			byName[subject.Object.Name] = index.Target.ID + "." + subject.ID
		}
	}
	runs := func(id string) bool {
		object := subjects[id].Object
		return object != nil && slices.Contains([]programindex.ObjectKind{programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule}, object.Kind)
	}
	for _, function := range index.Launch.Functions {
		if _, listed := at[index.Target.ID+"."+function.SubjectID]; runs(function.SubjectID) && !listed {
			t.Fatalf("launch function %s is not listed", function.SubjectID)
		}
	}
	for _, reach := range index.Reach {
		for _, reached := range reach.Subjects {
			if _, listed := at[index.Target.ID+"."+reached.SubjectID]; runs(reached.SubjectID) && !listed {
				t.Fatalf("%s, reached by %s, is not listed", reached.SubjectID, reach.OperationID)
			}
		}
	}
	order := []string{"main", "acceptHandler", "readQueryFromClient", "processInputBuffer", "processCommand"}
	for i, name := range order {
		position, listed := at[byName[name]]
		if !listed {
			t.Fatalf("%s is not in kvd's flow scope", name)
		}
		if i == 0 && position != 0 {
			t.Fatalf("main is member %d, not the first", position)
		}
		if i > 0 && position < at[byName[order[i-1]]] {
			t.Fatalf("%s (member %d) comes before %s, which hands it over or calls it", name, position, order[i-1])
		}
	}
	// Every listed member has every call its declaration makes.
	calls := map[string]int{}
	for _, place := range run.graph.Places {
		if place.Symbol != nil {
			calls[fmt.Sprintf("%s %s:%d", place.Symbol.Decl.Name, place.Path, place.LineNo)] = len(place.Symbol.Calls)
		}
	}
	for _, member := range flow.Members {
		if want, placed := calls[member.Name+" "+member.Anchor]; placed && len(member.Calls) != want {
			t.Fatalf("%s lists %d of its %d calls", member.Name, len(member.Calls), want)
		}
	}
	for _, body := range bodies {
		for _, cut := range []string{"calls_omitted", "called_by", "callee_id"} {
			if strings.Contains(string(body), cut) {
				t.Fatalf("a request carries %q", cut)
			}
		}
	}
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

type kvdOrientation struct {
	fixture cFixture
	graph   atlas.Graph
	indexes []groupindex.Index
	asked   *capturedOrientation
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
	if _, _, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, asked,
		orientation.Input{RepositoryName: "kvd", Facts: layer, Claims: noClaims, Groups: indexes, Graph: graph}); err != nil {
		t.Fatal(err)
	}
	return kvdOrientation{fixture: fixture, graph: graph, indexes: indexes, asked: asked}
}

// capturedOrientation keeps the orientation's requests. It answers the
// overview by choosing flowTarget's Main flow, when one is set, and the flow
// with the legitimate empty answer.
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
	if len(asked.users) == 2 {
		return llm.Completion{}, fmt.Errorf("the orientation was asked a third time")
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
