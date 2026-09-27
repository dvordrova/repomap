package report

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An input's reading shows its path: the chain from the program's entry to
// each dispatch site whose alternatives hold its handler, shared by every
// input those alternatives handle, then the handler's own steps, a callee
// under its caller. The chain passes through none of the handlers the site
// chooses between: Redis's shortest chain to call ran main →
// loadAppendOnlyFile → execCommand → call, through a command the dispatch
// itself calls.
func TestAnInputsPathSharesTheChainToItsDispatchAndListsItsOwnSteps(t *testing.T) {
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
	if len(path.Shared) != 2 {
		t.Fatalf("want the chains to both dispatch sites, got %+v", path.Shared)
	}
	network, replay := path.Shared[0], path.Shared[1]
	if want := []string{"mainCommand", "aeMainCommand", "eventsCommand", "readCommand", "processCommand", "callCommand"}; !equalStrings(names(network.Steps), want) {
		t.Fatalf("the chain to call went through a handler it dispatches to:\n got %v\nwant %v", names(network.Steps), want)
	}
	if network.Inputs != 4 || !network.All || network.Of != 4 || network.Through != "callCommand" {
		t.Fatalf("the shared chain does not say whom it is shared by: %+v", network)
	}
	if network.Steps[1].PartTitle != "Event loop" || network.Steps[1].Part != mapNodeID("loop") || network.Steps[0].Href == "" && network.Steps[0].Open == "" && network.Steps[0].Source == "" {
		t.Fatalf("a step lost its part or its code: %+v", network.Steps[1])
	}
	if want := []string{"mainCommand", "loadCommand"}; !equalStrings(names(replay.Steps), want) || replay.Through != "loadCommand" {
		t.Fatalf("the replay chain: %v", names(replay.Steps))
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
	if json.Unmarshal([]byte(renamed), &moved) != nil || moved.Own[2].Part != "server-"+mapNodeID("keys") || moved.Shared[0].Steps[0].Part != "server-"+mapNodeID("config") {
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
