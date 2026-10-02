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

	steps := b.startSteps(section, library)
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
	for _, step := range steps[1:] {
		if len(step.Reaches) != maxStartReaches {
			t.Fatalf("export %s reads %d connections, want %d", step.Symbol, len(step.Reaches), maxStartReaches)
		}
	}
	// Per export a handful of allocations; read again per connection and
	// operation it was millions.
	allocations := testing.AllocsPerRun(1, func() { b.startSteps(section, library) })
	if bound := float64(40*exports + 20*(connections+operations)); allocations > bound {
		t.Fatalf("startSteps made %.0f allocations for %d exports, %d connections and %d operations, over %.0f: its work grows faster than its entrypoints", allocations, exports, connections, operations, bound)
	}
}
