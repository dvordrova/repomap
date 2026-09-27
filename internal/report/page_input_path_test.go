package report

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An input's reading shows its path: each dispatch site whose alternatives
// hold its handler, named by the declaration it dispatches through and how
// many it chooses between, then the handler's own steps, a callee under its
// caller. No route from the program's entry to the site is chosen: the
// shortest static chain to Redis's call ran main → aeMain → beforeSleep →
// call, which a benchmark reader was offered as GET's path. Which path an
// input takes to the site is not established, so none is shown.
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
		builder.subjects[subjectKey("server", name)] = subjectRef{subject: groupindex.Subject{ID: name,
			Object: &groupindex.ObjectFacts{Name: name + "Command", Kind: programindex.ObjectFunction, Location: at(line + 1)}}}
	}
	got := builder.buildOperationMap(section, &index)
	var raw string
	for _, node := range got.Nodes {
		if node.Activation != "" && node.Handler == "getCommand" {
			raw = node.InputPath
		}
	}
	var path pageInputPath
	if err := json.Unmarshal([]byte(raw), &path); err != nil {
		t.Fatalf("get has no path: %q %v", raw, err)
	}
	names := func(steps []pagePathStep) (out []string) {
		for _, step := range steps {
			out = append(out, step.Name)
		}
		return out
	}
	want := []pageSharedPath{{Inputs: 4, All: true, Through: "callCommand", Of: 4}, {Inputs: 4, All: true, Through: "loadCommand", Of: 4}}
	if !reflect.DeepEqual(path.Shared, want) {
		t.Fatalf("the dispatch sites the input shares:\n got %+v\nwant %+v", path.Shared, want)
	}
	for _, entry := range []string{"mainCommand", "aeMainCommand", "eventsCommand", "readCommand", "processCommand"} {
		if strings.Contains(raw, `"`+entry+`"`) {
			t.Fatalf("the path chose a route from the entry through %s: %s", entry, raw)
		}
	}
	own := names(path.Own)
	if want := []string{"getCommand", "getGenericCommand", "lookupCommand", "replyCommand"}; !equalStrings(own, want) {
		t.Fatalf("own steps\n got %v\nwant %v", own, want)
	}
	if depths := []int{path.Own[0].Depth, path.Own[1].Depth, path.Own[2].Depth, path.Own[3].Depth}; !equalInts(depths, []int{0, 1, 2, 2}) {
		t.Fatalf("a callee is not under its caller: %v", depths)
	}
	renamed := remapInputPath(raw, func(id string) string { return "server-" + id })
	var moved pageInputPath
	if json.Unmarshal([]byte(renamed), &moved) != nil || moved.Own[2].Part != "server-"+mapNodeID("keys") || !reflect.DeepEqual(moved.Shared, want) {
		t.Fatalf("a scoped map did not rename the path's parts: %s", renamed)
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
