package report

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An input's reading shows where it is dispatched from, never a route to
// the dispatch: the shortest static chain to Redis's call ran main → aeMain
// → beforeSleep → call, which a benchmark reader was offered as GET's path.
// Which path an input takes to the site is not established. The inputs
// whose own code reaches the site are saved with it (exec's calls call),
// and the parts the input enters are listed by depth with every call
// entering each from an earlier part.
func TestAnInputsPathNamesItsDispatchWithoutARouteAndListsItsOwnSteps(t *testing.T) {
	section := &pageSection{ID: "server", ShortLabel: "Server"}
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "redis.c", Line: line, Column: 1}
	}
	edge := func(relation, from, to string, resolution programindex.Resolution, line int) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationID: relation,
			RelationKind: programindex.RelationCalls, Resolution: resolution, Location: at(line)}
	}
	exact, alternatives := programindex.ResolutionExact, programindex.ResolutionAlternatives
	var edges []groupindex.StructuralEdge
	for i, pair := range [][2]string{{"main", "aeMain"}, {"aeMain", "events"}, {"events", "read"}, {"read", "process"}, {"process", "call"},
		{"main", "load"}, {"exec", "call"}, {"get", "getGeneric"}, {"getGeneric", "lookup"}, {"getGeneric", "reply"}} {
		edges = append(edges, edge("r"+string(rune('a'+i)), pair[0], pair[1], exact, 100+i))
	}
	for _, handler := range []string{"get", "set", "del", "exec"} {
		edges = append(edges, edge("dispatch", "call", handler, alternatives, 2054), edge("replay", "load", handler, alternatives, 7587))
	}
	// A second site of the same call reads as the same line: listed once.
	edges = append(edges, edge("rz", "getGeneric", "lookup", exact, 120))
	operation := func(id, handler string) groupindex.Operation {
		return groupindex.Operation{ID: id, SubjectID: handler, GroupID: "strings", Name: id, Kind: "request", Source: "fact", Location: *at(704)}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "server", Seeds: []programindex.TargetSeed{{ObjectID: "main", Kind: "callable", Location: at(1)}}},
		Groups: []groupindex.Group{
			{ID: "config", Title: "Server configuration", MemberSubjectIDs: []string{"main"}},
			{ID: "loop", Title: "Event loop", MemberSubjectIDs: []string{"aeMain", "events"}},
			{ID: "clients", Title: "Client connections", MemberSubjectIDs: []string{"read", "process", "call", "reply"}},
			{ID: "strings", Title: "String commands", MemberSubjectIDs: []string{"get", "getGeneric", "set"}},
			{ID: "keys", Title: "Keyspace", MemberSubjectIDs: []string{"lookup"}},
			{ID: "persist", Title: "Persistence", MemberSubjectIDs: []string{"load"}},
			{ID: "tx", Title: "Transactions", MemberSubjectIDs: []string{"exec", "del"}},
		},
		Operations:      []groupindex.Operation{operation("get", "get"), operation("set", "set"), operation("del", "del"), operation("exec", "exec")},
		StructuralEdges: edges}
	builder := pageBuilder{indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{"server": section}, subjects: map[string]subjectRef{}}
	for line, name := range []string{"main", "aeMain", "events", "read", "process", "call", "load", "exec", "get", "set", "del", "getGeneric", "lookup", "reply"} {
		subject := groupindex.Subject{ID: name, Object: &groupindex.ObjectFacts{Name: name + "Command", Kind: programindex.ObjectFunction, Location: at(line + 1)}}
		builder.subjects[subjectKey("server", name)] = subjectRef{subject: subject}
		builder.indexes[0].Subjects = append(builder.indexes[0].Subjects, subject)
	}
	groupindex.Derive(&builder.indexes[0])
	index = builder.indexes[0]
	got := builder.buildOperationMap(section, &index)
	readings := map[string]string{}
	for _, node := range got.Nodes {
		if node.Activation != "" {
			readings[node.Handler] = node.InputPath
		}
	}
	var path pageInputPath
	raw := readings["getCommand"]
	if err := json.Unmarshal([]byte(raw), &path); err != nil {
		t.Fatalf("get has no path: %q %v", raw, err)
	}
	name := func(position int) string { return path.Decls[position].Name }
	type dispatched struct {
		site                 string
		of, handlers, inputs int
		all                  bool
		reachedFrom          []string
	}
	var sites []dispatched
	for _, site := range path.Dispatched {
		sites = append(sites, dispatched{name(site.Site), site.Of, site.Handlers, site.Inputs, site.All, site.ReachedFrom})
	}
	want := []dispatched{{"callCommand", 4, 4, 4, true, []string{operationNodeID("server", "exec")}}, {"loadCommand", 4, 4, 4, true, nil}}
	if !reflect.DeepEqual(sites, want) {
		t.Fatalf("the dispatch sites of get:\n got %+v\nwant %+v", sites, want)
	}
	for _, entry := range []string{"mainCommand", "aeMainCommand", "eventsCommand", "readCommand", "processCommand", "execCommand"} {
		if strings.Contains(raw, `"`+entry+`"`) {
			t.Fatalf("get's reading names %s, which is no step of its own: %s", entry, raw)
		}
	}
	var parts []string
	for _, part := range path.Parts {
		entry := fmt.Sprintf("%s d%d", part.Title, part.Depth)
		for _, call := range part.Entered {
			entry += " " + name(call[0]) + ">" + name(call[1])
		}
		parts = append(parts, entry)
	}
	if want := []string{"String commands d0", "Keyspace d2 getGenericCommand>lookupCommand", "Client connections d2 getGenericCommand>replyCommand"}; !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts\n got %v\nwant %v", parts, want)
	}
	// The part holding the handler names it; no other part does.
	for i, part := range path.Parts {
		if (part.Handler != nil) != (i == 0) || i == 0 && name(*part.Handler) != "getCommand" {
			t.Fatalf("part %q names the handler as %v", part.Title, part.Handler)
		}
	}
	// exec's own code calls call: its reading says so, with that call.
	var exec pageInputPath
	if err := json.Unmarshal([]byte(readings["execCommand"]), &exec); err != nil || len(exec.Reaches) != 1 || exec.Decls[exec.Reaches[0].Site].Name != "callCommand" ||
		exec.Reaches[0].Inputs != 4 || len(exec.Reaches[0].Calls) != 1 || exec.Decls[exec.Reaches[0].Calls[0][0]].Name != "execCommand" {
		t.Fatalf("exec's reach of call: %+v", exec.Reaches)
	}
	renamed := remapInputPath(raw, func(id string) string { return "server-" + id })
	var moved pageInputPath
	if json.Unmarshal([]byte(renamed), &moved) != nil || moved.Parts[1].Part != "server-"+mapNodeID("keys") || moved.Decls[moved.Parts[1].Entered[0][1]].Part != "server-"+mapNodeID("keys") ||
		moved.Dispatched[0].ReachedFrom[0] != "server-"+operationNodeID("server", "exec") {
		t.Fatalf("a scoped map did not rename the path's nodes: %s", renamed)
	}
	// call's own reading, in its part, lists exec reaching it with its call.
	for _, node := range got.Nodes {
		if node.ID != mapNodeID("clients") {
			continue
		}
		var readings pageSiteReadings
		if err := json.Unmarshal([]byte(node.Dispatch), &readings); err != nil || len(readings.Sites) != 1 || readings.Decls[readings.Sites[0].Site].Name != "callCommand" ||
			len(readings.Sites[0].ReachedFrom) != 1 || readings.Sites[0].ReachedFrom[0].Input != operationNodeID("server", "exec") {
			t.Fatalf("call's reading: %s", node.Dispatch)
		}
	}
}

// A dispatch site's "one of N" counts its alternatives, and its inputs can
// outnumber them: Redis's call is one of 94 handlers and 95 inputs are
// dispatched there, because sinterCommand handles sinter and smembers. The
// page data says how many alternatives are handlers and which handler
// several of its inputs share, so each count can say what it counts.
func TestADispatchSiteCountsItsHandlersApartFromItsInputs(t *testing.T) {
	index := groupindex.Index{Operations: []groupindex.Operation{
		{ID: "sinter", SubjectID: "sinterCommand"}, {ID: "sadd", SubjectID: "saddCommand"}, {ID: "smembers", SubjectID: "sinterCommand"}, {ID: "ping", SubjectID: "pingCommand"},
	}}
	site := groupindex.DispatchSite{FromSubjectID: "call", Alternatives: []string{"sinterCommand", "saddCommand", "freeClient"}, OperationIDs: []string{"sinter", "sadd", "smembers"}}
	builder := pageBuilder{subjects: map[string]subjectRef{}}
	decls := builder.pathDecls("server", func(string) string { return "" })
	handlers, shared := siteHandlers(&index, site, decls, func(id string) string { return "node-" + id })
	if handlers != 2 {
		t.Fatalf("handlers among the alternatives = %d, want 2 (freeClient handles no input)", handlers)
	}
	if len(shared) != 1 || decls.list[shared[0].Handler].Name != "sinterCommand" || !equalStrings(shared[0].Inputs, []string{"node-sinter", "node-smembers"}) {
		t.Fatalf("shared handlers = %+v (decls %+v)", shared, decls.list)
	}
	raw, err := json.Marshal(pageInputPath{Dispatched: []pageDispatched{{Shared: shared}}, Decls: decls.list})
	if err != nil {
		t.Fatal(err)
	}
	var moved pageInputPath
	if json.Unmarshal([]byte(remapInputPath(string(raw), func(id string) string { return "t1-" + id })), &moved) != nil || moved.Dispatched[0].Shared[0].Inputs[1] != "t1-node-smembers" {
		t.Fatalf("a scoped map did not rename the shared handler's inputs: %+v", moved)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// The ways a request arrives at a dispatch site stand requests first, then
// scheduled work, then continuous work (owner, 2026-09-28): GET's reading
// had opened with "arrives at call from serverCron" before acceptHandler.
func TestOuterInputsStandRequestsBeforeScheduledBeforeContinuous(t *testing.T) {
	at := &programindex.Location{Path: "redis.c", Line: 1, Column: 1}
	index := groupindex.Index{Target: programindex.Target{ID: "server"},
		Operations: []groupindex.Operation{{ID: "get", Kind: "request", SubjectID: "get"}, {ID: "loop", Kind: "continuous"}, {ID: "cron", Kind: "scheduled"}, {ID: "accept", Kind: "request"}},
		Dispatch: []groupindex.DispatchSite{{FromSubjectID: "call", Location: at, Alternatives: []string{"get"}, OperationIDs: []string{"get"},
			Outer: []groupindex.OuterInput{{OperationID: "loop"}, {OperationID: "cron"}, {OperationID: "accept"}}}}}
	builder := &pageBuilder{indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{}}
	decls := builder.pathDecls("server", func(string) string { return "" })
	raw := builder.inputPath(&builder.indexes[0], index.Operations[0], groupindex.Reach{}, decls, func(string) string { return "" }, func(id string) string { return "n-" + id }, nil)
	var path struct {
		Dispatched []struct {
			Outer []struct{ Input string } `json:"outer"`
		} `json:"dispatched"`
	}
	if err := json.Unmarshal([]byte(raw), &path); err != nil || len(path.Dispatched) != 1 {
		t.Fatalf("%s %v", raw, err)
	}
	var order []string
	for _, outer := range path.Dispatched[0].Outer {
		order = append(order, outer.Input)
	}
	if strings.Join(order, " ") != "n-accept n-cron n-loop" {
		t.Fatalf("outer inputs: %v", order)
	}
}

// A dispatcher's reading lists the inputs dispatched there by name, each
// with its handler (owner, 2026-09-28: "one of 94 handlers" opened nothing).
func TestADispatchersReadingListsItsInputsByName(t *testing.T) {
	at := &programindex.Location{Path: "redis.c", Line: 1, Column: 1}
	index := groupindex.Index{Target: programindex.Target{ID: "server"},
		Operations: []groupindex.Operation{{ID: "set", Name: "set", Kind: "request", SubjectID: "setCommand"}, {ID: "get", Name: "get", Kind: "request", SubjectID: "getCommand"}, {ID: "DEL", Name: "DEL", Kind: "request", SubjectID: "delCommand"}},
		Dispatch:   []groupindex.DispatchSite{{FromSubjectID: "call", Location: at, Alternatives: []string{"setCommand", "getCommand", "delCommand"}, OperationIDs: []string{"set", "get", "DEL"}}}}
	builder := &pageBuilder{indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{}}
	for _, name := range []string{"call", "setCommand", "getCommand", "delCommand"} {
		builder.subjects[subjectKey("server", name)] = subjectRef{subject: groupindex.Subject{ID: name, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: at}}}
	}
	raw := builder.siteReadings(&builder.indexes[0], groupindex.Group{MemberSubjectIDs: []string{"call"}}, builder.pathDecls("server", func(string) string { return "" }), func(id string) string { return "n-" + id })
	var readings pageSiteReadings
	if err := json.Unmarshal([]byte(raw), &readings); err != nil || len(readings.Sites) != 1 {
		t.Fatalf("%s %v", raw, err)
	}
	var listed []string
	for _, entry := range readings.Sites[0].Dispatched {
		listed = append(listed, entry.Input+" "+readings.Decls[entry.Handler].Name)
	}
	if strings.Join(listed, ", ") != "n-DEL delCommand, n-get getCommand, n-set setCommand" {
		t.Fatalf("dispatched: %v", listed)
	}
}

// The keys of a table an input's handler looks up stand in the order the
// table writes them, each with what it names (othello's key-pressed had
// read "1 2 h n u", bare and sorted, for key->command's n → new-game, u →
// undo, h → hints, 1 → play-black, 2 → play-white).
func TestAnInputsKeysStandAsWrittenWithWhatEachNames(t *testing.T) {
	row := func(id, name, named string, line int) groupindex.Operation {
		return groupindex.Operation{ID: id, Kind: "interaction", Name: name, Names: []string{named}, HandlerUnknown: true, ValueOf: "key", Location: programindex.Location{Path: "events.cljc", Line: line, Column: 4}}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Operations: []groupindex.Operation{
		row("o1", "1", "play-black", 214), row("o2", "2", "play-white", 215), row("o3", "h", "hints", 213), row("o4", "n", "new-game", 211), row("o5", "u", "undo", 212),
		{ID: "key", Kind: "interaction", Name: "key-pressed", SubjectID: "on-key", Location: programindex.Location{Path: "sketch.clj", Line: 45, Column: 18}},
	}}
	builder := &pageBuilder{indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{}}
	decls := builder.pathDecls("t1", func(string) string { return "" })
	raw := builder.inputPath(&builder.indexes[0], index.Operations[5], groupindex.Reach{OperationID: "key", SubArguments: []string{"o1", "o2", "o3", "o4", "o5"}}, decls, func(string) string { return "" }, func(id string) string { return "n-" + id }, nil)
	var path struct {
		Checks []pageDecl `json:"checks"`
	}
	if err := json.Unmarshal([]byte(raw), &path); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, check := range path.Checks {
		keys = append(keys, check.Name+" → "+check.Named)
	}
	if want := "n → new-game, u → undo, h → hints, 1 → play-black, 2 → play-white"; strings.Join(keys, ", ") != want {
		t.Fatalf("key-pressed's keys: %q, want %q", strings.Join(keys, ", "), want)
	}
}
