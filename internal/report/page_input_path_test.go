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
		site        string
		of, inputs  int
		all         bool
		reachedFrom []string
	}
	var sites []dispatched
	for _, site := range path.Dispatched {
		sites = append(sites, dispatched{name(site.Site), site.Of, site.Inputs, site.All, site.ReachedFrom})
	}
	want := []dispatched{{"callCommand", 4, 4, true, []string{operationNodeID("server", "exec")}}, {"loadCommand", 4, 4, true, nil}}
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
