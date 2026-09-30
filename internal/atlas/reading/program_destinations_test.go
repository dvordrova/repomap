package reading

import (
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A destination can be one of this repository's other programs: the
// destination question offers each program that takes requests or listens
// while it runs, with what it takes, never the calling program itself nor
// one that takes nothing a call reaches; the chosen one is recorded as the
// boundary's destination target. freqtrade-client's one generic request,
// whose path the code computes, had stood as "Freqtrade Server" outside
// freqtrade. Only a destination with a request sent or a connection opened
// is offered a program: a host lookup through a library is answered by the
// resolver (redis-cli's gethostbyname had been drawn into redis-server).
func TestADestinationCanBeAnotherProgramOfTheRepository(t *testing.T) {
	get := atlas.SymbolCall{Kind: "invokes_external", Name: "Get", Line: 7, Column: 9, API: &atlas.CallAPI{Package: "net/http", Receiver: "*Client", Name: "Get"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "url", Position: 1, Anchor: &sourcevalue.Anchor{Path: "client/call.go", Line: 5, Column: 11}}}}}
	call := atlas.Place{ID: "symbol:Call", Kind: atlas.PlaceSymbol, Path: "client/call.go", LineNo: 5, Parent: "file:call", TargetIDs: []string{"client"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:Call", Name: "Call"}, Calls: []atlas.SymbolCall{get}}}
	route := atlas.Place{ID: "fact:status", Kind: atlas.PlaceBoundary, Path: "server/api.go", LineNo: 3, Column: 2, Parent: "file:api", TargetIDs: []string{"server"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "server", FactID: "status"}}, Method: "GET", Values: []string{"/api/status"}, Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryRequest}}
	option := atlas.Place{ID: "fact:verbose", Kind: atlas.PlaceBoundary, Path: "tool/main.go", LineNo: 4, Column: 2, Parent: "file:tool", TargetIDs: []string{"tool"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "tool", FactID: "verbose"}}, Values: []string{"-verbose"}, Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryCommand}}
	lookup := atlas.SymbolCall{Kind: "invokes_external", Name: "LookupHost", Line: 12, Column: 9, API: &atlas.CallAPI{Package: "net", Name: "LookupHost"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "host", Position: 1, Anchor: &sourcevalue.Anchor{Path: "client/resolve.go", Line: 10, Column: 14}}}}}
	resolve := atlas.Place{ID: "symbol:Resolve", Kind: atlas.PlaceSymbol, Path: "client/resolve.go", LineNo: 10, Parent: "file:resolve", TargetIDs: []string{"client"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:Resolve", Name: "Resolve"}, Calls: []atlas.SymbolCall{lookup}}}
	graph := []atlas.Place{call, resolve, route, option}
	var mu sync.Mutex
	var offered []any
	lookupOffered := -1
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == "atlas_systems" {
			for i := range rows {
				rows[i]["system"] = "none"
			}
			return
		}
		for _, column := range input["fill"].([]any) {
			if column.(map[string]any)["name"] != "destination" {
				continue
			}
			catalog, _ := input["context"].(map[string]any)["destination_catalog"].([]any)
			for i, source := range input["rows"].([]any) {
				if strings.Contains(string(mustJSON(source)), "LookupHost") {
					lookupOffered = len(catalog)
					rows[i]["destination"] = "other: DNS Resolver"
					continue
				}
				offered = catalog
				rows[i]["destination"] = destinationRef(input, "server")
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{{ID: "client", Name: "client", Root: "client"}, {ID: "server", Name: "server", Root: "server"}, {ID: "tool", Name: "tool", Root: "tool"}}
	r.opts.ReadSource = func(string) ([]byte, error) { return nil, nil }
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"net/http.Client.Get": {talks: atlas.BoundaryClientRequest}, "net.LookupHost": {talks: atlas.BoundarySDK}}
	r.arguments = map[string]ArgumentChoice{"net/http.Client.Get": {Position: 1}, "net.LookupHost": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(offered) != 1 {
		t.Fatalf("offered %s, want the server alone", mustJSON(offered))
	}
	entry := offered[0].(map[string]any)
	takes, _ := entry["takes"].([]any)
	if entry["value"] != "server" || entry["program"] != true || len(takes) != 1 || !strings.Contains(takes[0].(string), "request GET") {
		t.Fatalf("the server's entry %s, want the program with what it takes", mustJSON(entry))
	}
	if lookupOffered != 0 {
		t.Fatalf("the host lookup was offered %d entries, want none", lookupOffered)
	}
	var found bool
	for _, state := range r.boundaries {
		if state.place.Boundary.Direction == atlas.DirectionOut && state.kind == atlas.BoundaryClientRequest {
			found = true
			if state.destinationOf("client") != "server" || state.destinationTargets["client"] != "server" {
				t.Fatalf("the client's call reaches %q (%v), want the server program", state.destinationOf("client"), state.destinationTargets)
			}
		}
	}
	if !found {
		t.Fatal("the client's call made no outgoing boundary")
	}
	catalog := []lines.Destination{{Ref: "d1", Value: "Telegram"}, {Ref: "d2", Value: "server", Program: true, Target: "server"}}
	for _, cell := range [][2]string{{"d1", "Telegram"}, {"other: Server farm", "Server farm"}} {
		if target, _ := lines.DestinationTarget(catalog, cell[0], cell[1]); target != "" {
			t.Fatalf("%q named the program %s", cell[0], target)
		}
	}
	// The offered program named after the free prefix is that program.
	for _, cell := range [][2]string{{"d2", "server"}, {"other: Server", "Server"}} {
		if target, name := lines.DestinationTarget(catalog, cell[0], cell[1]); target != "server" || name != "server" {
			t.Fatalf("%q named %q %q, want the server program", cell[0], target, name)
		}
	}
}
