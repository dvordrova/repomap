package audit

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// memoryTree is a repository held in memory: the fixture's source.
type memoryTree map[string]string

func (tree memoryTree) isPath(word string) bool {
	for path := range tree {
		if path == word || strings.HasPrefix(path, word+"/") || strings.HasSuffix(path, "/"+word) {
			return true
		}
	}
	return false
}

func (tree memoryTree) grep(literal string, paths []string) (bool, error) {
	for path, text := range tree {
		if (len(paths) == 0 || slices.Contains(paths, path)) && strings.Contains(text, literal) {
			return true, nil
		}
	}
	return false, nil
}

func (tree memoryTree) read(path string) (string, error) { return tree[path], nil }

const fixtureInventory = `{
 "repository": "kvd", "revision": "0123456789abcdef",
 "items": [
  {"component": "kvd", "inventory": "requests", "kind": "protocol-command", "name": "get", "anchor": "kvd.c:100", "handler": "getCommand", "must": true, "why": "read"},
  {"component": "kvd", "inventory": "requests", "kind": "protocol-command", "name": "del", "anchor": "kvd.c:101", "handler": "delCommand", "must": true, "why": "delete"},
  {"component": "kvd", "inventory": "commands", "kind": "setting", "name": "port", "anchor": "kvd.c:120", "handler": null, "must": true, "why": "listen port"},
  {"component": "kvd", "inventory": "commands", "kind": "entrypoint", "name": "kvd binary", "anchor": "kvd.c:9", "handler": "main", "must": true, "why": "way in"},
  {"component": "kvd", "inventory": "external", "kind": "tcp", "name": "upstream", "anchor": "kvd.c:200", "handler": "syncUpstream", "must": true, "why": "connects from syncUpstream"},
  {"component": "kvd", "inventory": "external", "kind": "dns", "name": "name lookup", "anchor": "net.c:10", "handler": null, "must": false, "why": "resolves the upstream host"},
  {"component": "kvd", "inventory": "data", "kind": "sqlite-table", "name": "user rows (users)", "anchor": "store.c:9", "handler": null, "must": true, "why": "accounts"},
  {"component": "kvd", "inventory": "workers", "kind": "main-loop", "name": "event loop", "anchor": "kvd.c:71", "handler": "loop", "must": false, "why": "foreground", "flow": true}
 ],
 "not": [
  {"component": "kvd", "name": "port", "anchor": "kvd.c:120", "reason": "a tie goes to the item"},
  {"component": "kvd", "name": "appendfsync values", "anchor": "kvd.c:122", "reason": "values of appendfsync"},
  {"component": "kvd", "name": "debug", "anchor": "kvd.c:140", "reason": "a log level, not a setting"},
  {"component": "kvd", "name": "cache probe", "anchor": "kvd.c:300", "reason": "traps never match a caller site"}
 ],
 "errata": [{"date": "2026-09-29", "item": "fixture", "change": "none", "reason": "shape", "citation": "kvd.c:1"}]
}`

func fixtureRun() *auditRun {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 1}
	}
	program := programindex.Index{
		Target: programindex.Target{ID: "t1", Name: "kvd", Language: "c"},
		Objects: []programindex.Object{
			{ID: "n1", Kind: programindex.ObjectFunction, Name: "main", Location: at("kvd.c", 10)},
			{ID: "n2", Kind: programindex.ObjectFunction, Name: "initServer", Location: at("kvd.c", 30)},
			{ID: "n3", Kind: programindex.ObjectFunction, Name: "acceptHandler", Location: at("kvd.c", 50)},
			{ID: "n4", Kind: programindex.ObjectFunction, Name: "loop", Location: at("kvd.c", 70)},
			{ID: "n5", Kind: programindex.ObjectFunction, Name: "lonely", Location: at("kvd.c", 90)},
		},
		Relations: []programindex.Relation{
			{ID: "r1", Kind: programindex.RelationCalls, FromID: "n1", ToIDs: []string{"n2"}},
			{ID: "r2", Kind: programindex.RelationPassesCallback, FromID: "n2", ToIDs: []string{"n3"}},
			{ID: "r3", Kind: programindex.RelationCalls, FromID: "n1", ToIDs: []string{"n4"}},
		},
	}
	operation := func(id, kind, name string, line int) groupindex.Operation {
		return groupindex.Operation{ID: id, Kind: kind, Name: name, Source: "model", Location: programindex.Location{Path: "kvd.c", Line: line, Column: 1}}
	}
	always := operation("o3", "setting", "always", 122)
	always.ValueOf = "o2"
	index := groupindex.Index{
		Target: program.Target,
		Operations: []groupindex.Operation{
			operation("o1", "request", "get", 100),
			operation("o2", "setting", "port", 120),
			always,
			operation("o4", "setting", "debug", 140),
			operation("o5", "command", "-v", 160),
		},
		Outbound: []groupindex.OutboundCall{
			{ID: "b1", Kind: "client_request", Destination: "upstream", Location: programindex.Location{Path: "net.c", Line: 20},
				ReachedFrom: []groupindex.OutboundCaller{{SubjectID: "n2", Location: at("kvd.c", 200)}}},
			{ID: "b2", Kind: "client_request", Destination: "upstream (retry)", Location: programindex.Location{Path: "net.c", Line: 40}},
			{ID: "b4", Kind: "sdk", Destination: "DNS resolver", Location: programindex.Location{Path: "net.c", Line: 11},
				ReachedFrom: []groupindex.OutboundCaller{{SubjectID: "n2", Location: at("kvd.c", 200)}}},
			{ID: "b3", Kind: "client_request", Destination: "cache", Location: programindex.Location{Path: "net.c", Line: 60},
				ReachedFrom: []groupindex.OutboundCaller{{SubjectID: "n2", Location: at("kvd.c", 300)}}},
		},
		Data: []groupindex.DataRecord{{DataRecord: atlas.DataRecord{ID: "d1", Path: "schema.sql", Line: 5, Data: &facts.DataObject{Kind: "table", Name: "users"}}}},
		// What groupindex.Derive gives a value of another input's words.
		Launch: groupindex.Launch{Nested: map[string]bool{"o3": true}},
	}
	return &auditRun{
		dir: "fixture", revision: "0123456789abcdef", targets: []auditTarget{newAuditTarget("t1", "kvd", program, index)},
		facts: facts.Result{Facts: []facts.Fact{
			{ID: "a1", Kind: facts.KindEntrypoint, TargetID: "t1", Symbol: "main", ObjectID: "n1", Anchor: &facts.Anchor{Path: "kvd.c", Line: 10}},
			{ID: "a2", Kind: facts.KindConfigRead, TargetID: "t1", Key: "KVD_PORT", Anchor: &facts.Anchor{Path: "kvd.c", Line: 12}},
		}},
		orientation: &orientation.Result{
			Summary: "kvd serves keys.",
			RunRecipe: []orientation.RecipeStep{{TargetID: "t1", Command: "./kvd --port 6379", FactIDs: []string{"a1"},
				Note: `It prints "Usage: kvd [--port N]", then "listening on 6379"; "ready for work" is invented.`}},
			MainFlow: orientation.MainFlow{Steps: []orientation.FlowStep{
				{TargetID: "t1", FactID: "a1", Explanation: "The program starts at main."},
				{TargetID: "t1", SubjectID: "n2", Explanation: "initServer calls acceptHandler on each connection."},
				{TargetID: "t1", SubjectID: "n3", Explanation: "acceptHandler accepts one client."},
				{TargetID: "t1", SubjectID: "n4", Explanation: "main enters loop."},
				{TargetID: "t1", SubjectID: "n5", Explanation: "lonely hands work to inventedHelper."},
			}},
		},
	}
}

// TestAuditFixture scores a hand-built run: the matching rules, fix 0, the
// ReachedFrom exception, the Main flow items and the three falsities.
func TestAuditFixture(t *testing.T) {
	inv, err := decodeInventory([]byte(fixtureInventory))
	if err != nil {
		t.Fatal(err)
	}
	source := memoryTree{
		"kvd.c": "int main(void) {\n  printf(\"Usage: kvd [--port N]\\n\");\n  printf(\"listening on %d\\n\", port);\n  if (!strcmp(argv[1], \"--port\")) {}\n}\n",
	}
	report, err := runAudit("kvd", fixtureRun(), inv, source)
	if err != nil {
		t.Fatal(err)
	}

	status := map[string]string{}
	for _, res := range report.score.results {
		status[res.item.Name] = res.status
	}
	for name, want := range map[string]string{
		"get":               "found",  // exact line
		"del":               "missed", // the get row keeps its exact line, never its neighbour
		"port":              "found",  // an item wins a tie with a trap
		"kvd binary":        "found",  // any entrypoint of the component
		"upstream":          "found",  // the call's ReachedFrom caller site
		"user rows (users)": "found",  // a table name across files
		"name lookup":       "found",  // its row's own location outranks the row's caller site
	} {
		if status[name] != want {
			t.Errorf("item %q: %s, want %s", name, status[name], want)
		}
	}
	if _, scored := status["event loop"]; scored || len(report.score.flow) != 1 || report.score.flow[0].status != "found" || !slices.Equal(report.score.flow[0].steps, []int{4}) {
		t.Errorf("the flow item is scored against Main flow step 4 only, not the workers: %v %+v", status, report.score.flow)
	}

	rowStatus := map[string]string{}
	for _, row := range report.score.rows {
		rowStatus[row.id] = row.status
	}
	for id, want := range map[string]string{
		"t1/o3": "nested",           // fix 0: a value of port is no trap hit
		"t1/o4": "trap",             // a trap on its own line
		"t1/o5": "extra",            // in no inventory
		"t1/b2": "same_destination", // the matched destination's other call
		"t1/b3": "extra",            // a trap never matches a caller site
	} {
		if rowStatus[id] != want {
			t.Errorf("row %s: %s, want %s", id, rowStatus[id], want)
		}
	}

	falsities := report.falsities()
	for _, want := range []string{"step 5 (t1 lonely): BROKEN", `invented name "inventedHelper"`, `quotes "ready for work"`} {
		if !slices.ContainsFunc(falsities, func(f string) bool { return strings.Contains(f, want) }) {
			t.Errorf("falsities %q miss %q", falsities, want)
		}
	}
	if len(falsities) != 3 {
		t.Errorf("falsities: %q, want the broken link, the invented name and the missing quote only", falsities)
	}
	if !slices.ContainsFunc(report.flow.wording, func(f wordingFlag) bool { return f.says == "calls acceptHandler" && f.code == "passes_callback" }) {
		t.Errorf("wording flags %+v miss step 2's call that is a callback", report.flow.wording)
	}
	if err := report.write(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
