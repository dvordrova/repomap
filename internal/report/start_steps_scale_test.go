package report

import (
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A library's every export is an entrypoint: headscale's library had 1,788
// of them in one part. Each entrypoint reads its part's connections, and
// each connection names the input its far end is written at. The page read
// every operation's place again for every connection of every entrypoint,
// and headscale's render took 300 s. The part's connections are now read
// once however many entrypoints it holds, and places once a call. The
// allocations grow with the entrypoints, not with entrypoints times
// connections times operations.
func TestStartStepsReadAPartsConnectionsOnceForAllItsEntrypoints(t *testing.T) {
	const exports, connections, operations = 400, 40, 400
	library := groupindex.Index{Target: programindex.Target{ID: "lib"}, Groups: []groupindex.Group{{ID: "api", Title: "API"}, {ID: "store", Title: "Store"}}}
	server := groupindex.Index{Target: programindex.Target{ID: "srv"}, Groups: []groupindex.Group{{ID: "handlers", Title: "Handlers"}}}
	b := pageBuilder{
		data:        &ReportData{},
		subjects:    map[string]subjectRef{},
		subjectAt:   map[string]string{},
		groupTitles: map[groupindex.Endpoint]string{{TargetID: "srv", GroupID: "handlers"}: "Handlers", {TargetID: "lib", GroupID: "store"}: "Store"},
		links:       pageLinks{sourceIDs: map[string]string{"lib.go": "l", "srv.go": "s"}},
	}
	section := &pageSection{ID: "library"}
	for i := range exports {
		id := fmt.Sprintf("export%d", i)
		library.Groups[0].MemberSubjectIDs = append(library.Groups[0].MemberSubjectIDs, id)
		location := &programindex.Location{Path: "lib.go", Line: 10 + i, Column: 1}
		b.subjects[subjectKey("lib", id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Location: location}}}
		b.subjectAt[subjectLocationKey("lib", "lib.go", location.Line)] = id
		section.Entrypoints = append(section.Entrypoints, pageEntrypoint{Symbol: id, Anchor: &pageAnchor{Path: "lib.go", Line: location.Line}})
	}
	for i := range operations {
		server.Operations = append(server.Operations, groupindex.Operation{ID: fmt.Sprintf("op%d", i), Name: fmt.Sprintf("GET /op%d", i), Kind: "request", Location: programindex.Location{Path: "srv.go", Line: 100 + i, Column: 1}})
	}
	// The same place twice: the first operation written there is the input.
	server.Operations = append(server.Operations, groupindex.Operation{ID: "late", Name: "GET /late", Kind: "request", Location: programindex.Location{Path: "srv.go", Line: 100 + operations - 1, Column: 1}})
	for i := range connections {
		library.Connections = append(library.Connections, groupindex.Connection{
			From: groupindex.Endpoint{TargetID: "lib", GroupID: "api"}, To: groupindex.Endpoint{TargetID: "srv", GroupID: "handlers"},
			SourceID: fmt.Sprintf("c%d", i), Label: fmt.Sprintf("calls op%d", operations-1-i), FromSubjectID: "export0",
			FromLocation: &programindex.Location{Path: "lib.go", Line: 10 + i, Column: 2},
			ToLocation:   &programindex.Location{Path: "srv.go", Line: 100 + operations - 1 - i, Column: 1},
		})
	}
	library.Connections = append(library.Connections,
		groupindex.Connection{From: groupindex.Endpoint{TargetID: "lib", GroupID: "api"}, To: groupindex.Endpoint{TargetID: "lib", GroupID: "store"}, SourceID: "inner", Label: "keeps", FromSubjectID: "export0", FromLocation: &programindex.Location{Path: "lib.go", Line: 9, Column: 1}},
		groupindex.Connection{From: groupindex.Endpoint{TargetID: "lib", GroupID: "store"}, To: groupindex.Endpoint{TargetID: "srv", GroupID: "handlers"}, SourceID: "out", SourceKind: "integration", Label: "serves"},
	)
	b.indexes = []groupindex.Index{library, server}
	b.byProgram = map[string]*pageSection{"lib": section, "srv": {ID: "server", ShortLabel: "server"}}

	steps, _ := b.startSteps(section, library)
	if len(steps) != exports {
		t.Fatalf("%d steps, want one per export", len(steps))
	}
	first := steps[0].Reaches
	if len(first) != maxStartReaches || first[0].Label != "keeps" || first[1].Title != "GET /op399" {
		t.Fatalf("export0's first reaches = %+v", first)
	}
	for _, row := range first[1:] {
		if !row.input || row.Href != "#"+operationNodeID("server", fmt.Sprintf("op%d", operations-1-(row.FromSource.Line-10))) {
			t.Fatalf("a connection did not name the first input written at its far end: %+v", row)
		}
	}
	if len(first[0].Continues) != 1 || first[0].Continues[0].Name != "server" {
		t.Fatalf("an inner connection lost where its part goes on to: %+v", first[0].Continues)
	}
	// The other exports make none of them: export0's are never their way.
	for _, step := range steps[1:] {
		if len(step.Reaches) != 0 {
			t.Fatalf("export %s reads %d of export0's connections as its own", step.Symbol, len(step.Reaches))
		}
	}
	// Per export a handful of allocations; read again per connection and
	// operation it was millions.
	allocations := testing.AllocsPerRun(1, func() { b.startSteps(section, library) })
	if bound := float64(40*exports + 20*(connections+operations)); allocations > bound {
		t.Fatalf("startSteps made %.0f allocations for %d exports, %d connections and %d operations, over %.0f: its work grows faster than its entrypoints", allocations, exports, connections, operations, bound)
	}
}

// Lua 5.1.5's etc/noparser.c leaves the parser out: luaX_init is empty and
// luaY_parser only reports "parser not loaded" through lua_error. The start
// list had read both as "uses Parser", "uses Lexer": its part's header uses
// and its other members' calls, under each entry (control review,
// 2026-10-02). An entry reads only its own way: an empty one says it calls
// nothing, one whose calls reach no other part names them, and the part's
// other connections stand apart with their owners, its header and type uses
// on their own.
func TestAnEntryReadsOnlyItsOwnWayAndItsPartsOtherConnectionsApart(t *testing.T) {
	stubs := groupindex.Index{Target: programindex.Target{ID: "etc"}, Groups: []groupindex.Group{{ID: "stubs", Title: "Parser stubs", MemberSubjectIDs: []string{"init", "parser", "helper"}}, {ID: "io", Title: "IO"}}}
	stubs.Connections = []groupindex.Connection{
		{From: groupindex.Endpoint{TargetID: "etc", GroupID: "stubs"}, To: groupindex.Endpoint{TargetID: "etc", GroupID: "io"}, SourceID: "h", SourceKind: "imports", Label: "uses IO"},
		{From: groupindex.Endpoint{TargetID: "etc", GroupID: "stubs"}, To: groupindex.Endpoint{TargetID: "etc", GroupID: "io"}, SourceID: "w", SourceKind: "native_calls", Label: "helper calls fwrite_stub", FromSubjectID: "helper", FromLocation: &programindex.Location{Path: "etc/noparser.c", Line: 40, Column: 3}},
	}
	b := pageBuilder{
		data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{{Target: programindex.Target{ID: "etc"},
			Objects: []programindex.Object{{ID: "init", Name: "luaX_init"}, {ID: "parser", Name: "luaY_parser"}},
			// As saved: calls no unit of etc defines, with no target.
			Relations: []programindex.Relation{
				{ID: "e4", Kind: programindex.RelationCalls, FromID: "parser", Resolution: programindex.ResolutionUnresolved,
					Location: &programindex.Location{Path: "etc/noparser.c", Line: 30, Column: 3}, Patterns: []programindex.RelationPattern{{Selector: "lua_error"}}},
				{ID: "e3", Kind: programindex.RelationCalls, FromID: "parser", Resolution: programindex.ResolutionUnresolved,
					Location: &programindex.Location{Path: "etc/noparser.c", Line: 29, Column: 3}, Patterns: []programindex.RelationPattern{{Selector: "lua_pushliteral"}}},
			}}}}},
		subjects:    map[string]subjectRef{},
		subjectAt:   map[string]string{},
		groupTitles: map[groupindex.Endpoint]string{{TargetID: "etc", GroupID: "io"}: "IO"},
	}
	section := &pageSection{ID: "etc"}
	b.byProgram = map[string]*pageSection{"etc": section}
	b.indexes = []groupindex.Index{stubs}
	for line, id := range map[int]string{21: "init", 25: "parser"} {
		location := &programindex.Location{Path: "etc/noparser.c", Line: line, Column: 1}
		b.subjects[subjectKey("etc", id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Location: location}}}
		b.subjectAt[subjectLocationKey("etc", "etc/noparser.c", line)] = id
	}
	section.Entrypoints = []pageEntrypoint{{Symbol: "luaX_init", Anchor: &pageAnchor{Path: "etc/noparser.c", Line: 21}}, {Symbol: "luaY_parser", Anchor: &pageAnchor{Path: "etc/noparser.c", Line: 25}}}
	steps, elsewhere := b.startSteps(section, stubs)
	if len(steps) != 2 || len(steps[0].Reaches)+len(steps[1].Reaches) != 0 {
		t.Fatalf("an entry read its part's connections as its way: %+v", steps)
	}
	if !steps[0].Silent || len(steps[0].Calls) != 0 {
		t.Fatalf("the empty luaX_init: silent %v, calls %v", steps[0].Silent, steps[0].Calls)
	}
	// Its calls named as written, where they are written, in that order,
	// their implementation not established.
	var calls []string
	for _, call := range steps[1].Calls {
		calls = append(calls, fmt.Sprintf("%s:%d:%v", call.Anchor.Text, call.Anchor.Line, call.Unresolved))
	}
	if steps[1].Silent || fmt.Sprint(calls) != "[lua_pushliteral:29:true lua_error:30:true]" {
		t.Fatalf("luaY_parser: silent %v, calls %v", steps[1].Silent, calls)
	}
	if len(elsewhere) != 1 || elsewhere[0].Group != "Parser stubs" || len(elsewhere[0].Rows) != 1 || elsewhere[0].Rows[0].Label != "helper calls fwrite_stub" ||
		len(elsewhere[0].Uses) != 1 || elsewhere[0].Uses[0].Title != "IO" {
		t.Fatalf("the part's other connections: %+v", elsewhere)
	}
}
