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

// Read through the destination question: a client whose SDK the systems
// question named "Server" is offered that system beside the repository's
// program "server", and a call answered with the system's ref keeps the
// outside system, never the program sharing its name (review 2026-10-02,
// item 1).
func TestAnOutgoingCallAnsweredWithASystemsRefKeepsTheSystem(t *testing.T) {
	get := atlas.SymbolCall{Kind: "invokes_external", Name: "Get", Line: 7, Column: 9, API: &atlas.CallAPI{Package: "net/http", Receiver: "*Client", Name: "Get"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "url", Position: 1, Anchor: &sourcevalue.Anchor{Path: "client/call.go", Line: 5, Column: 11}}}}}
	fetch := atlas.SymbolCall{Kind: "invokes_external", Name: "Fetch", Line: 8, Column: 9, API: &atlas.CallAPI{Package: "example.com/sdk", Name: "Fetch"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "key", Position: 1, Anchor: &sourcevalue.Anchor{Path: "client/call.go", Line: 5, Column: 20}}}}}
	call := atlas.Place{ID: "symbol:Call", Kind: atlas.PlaceSymbol, Path: "client/call.go", LineNo: 5, Parent: "file:call", TargetIDs: []string{"client"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:Call", Name: "Call"}, Calls: []atlas.SymbolCall{get, fetch}}}
	route := atlas.Place{ID: "fact:status", Kind: atlas.PlaceBoundary, Path: "server/api.go", LineNo: 3, Column: 2, Parent: "file:api", TargetIDs: []string{"server"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "server", FactID: "status"}}, Method: "GET", Values: []string{"/api/status"}, Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryRequest}}
	graph := []atlas.Place{call, route}
	var mu sync.Mutex
	var offered []any
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == "atlas_systems" {
			for i, source := range input["rows"].([]any) {
				rows[i]["system"] = "none"
				if strings.Contains(string(mustJSON(source)), "example.com/sdk") {
					rows[i]["system"] = "Server"
				}
			}
			return
		}
		for _, column := range input["fill"].([]any) {
			if column.(map[string]any)["name"] != "destination" {
				continue
			}
			offered, _ = input["context"].(map[string]any)["destination_catalog"].([]any)
			for i := range input["rows"].([]any) {
				rows[i]["destination"] = destinationRef(input, "Server")
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{{ID: "client", Name: "client", Root: "client"}, {ID: "server", Name: "server", Root: "server"}}
	r.opts.ReadSource = func(string) ([]byte, error) { return nil, nil }
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"net/http.Client.Get": {talks: atlas.BoundaryClientRequest}, "example.com/sdk.Fetch": {talks: atlas.BoundarySDK}}
	r.arguments = map[string]ArgumentChoice{"net/http.Client.Get": {Position: 1}, "example.com/sdk.Fetch": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if catalog := string(mustJSON(offered)); !strings.Contains(catalog, `"ref":"d1","value":"Server"`) || !strings.Contains(catalog, `"program":true,"ref":"d2","takes":["request GET /api/status"],"value":"server"`) {
		t.Fatalf("offered %s, want the system Server beside the program server", mustJSON(offered))
	}
	var found bool
	for _, state := range r.boundaries {
		if state.place.Boundary.Direction == atlas.DirectionOut && state.kind == atlas.BoundaryClientRequest {
			found = true
			t.Logf("the client's request reaches %q, program %q", state.destinationOf("client"), state.destinationTargets["client"])
			if state.destinationOf("client") != "Server" || state.destinationTargets["client"] != "" {
				t.Fatalf("the client's request answered with the system's ref reaches %q (%v), want the system Server", state.destinationOf("client"), state.destinationTargets)
			}
		}
	}
	if !found {
		t.Fatal("the client's request made no outgoing boundary")
	}
}

// A program is offered, and a program asking is called, by the one name
// the toolchain gives its executable, the name the report titles it by:
// casdoor's server, offered as github.com/casdoor/casdoor, had its web's
// calls answered "other: Casdoor" in three windows of six. That answer now
// writes the offered name and reaches the program. A program keeps its
// target name where its executable's name is another program's, or a
// system's the systems question gave any package of the run.
func TestAProgramIsOfferedByItsExecutablesName(t *testing.T) {
	targets := []TargetMeta{
		{ID: "t1", Name: "github.com/casdoor/casdoor", Executables: []string{"casdoor"}},
		{ID: "t2", Name: "web"},
		{ID: "t3", Name: "go.etcd.io/etcd/server/v3", Executables: []string{"server"}},
		{ID: "t4", Name: "go.etcd.io/etcd/etcdctl/v3", Executables: []string{"etcdctl"}},
		{ID: "t5", Name: "freqtrade", Executables: []string{"freqtrade"}},
		{ID: "t6", Name: "freqtrade-client", Executables: []string{"freqtrade-client"}},
		// Two programs whose executables are named alike keep their names.
		{ID: "t7", Name: "example.com/a/cmd/worker", Executables: []string{"worker"}},
		{ID: "t8", Name: "example.com/b/cmd/worker", Executables: []string{"worker"}},
		// An executable named as another program is keeps its target name.
		{ID: "t9", Name: "github.com/benbjohnson/litestream/cmd/litestream-vfs", Executables: []string{"litestream-vfs"}},
		{ID: "t10", Name: "litestream-vfs"},
		// An executable named as a system the run names keeps its target name.
		{ID: "t11", Name: "example.com/tools/cmd/stripe", Executables: []string{"stripe"}},
		// A program its build names twice keeps its target name.
		{ID: "t12", Name: "example.com/multi", Executables: []string{"a", "b"}},
	}
	got := programNames(targets, map[string]string{"github.com/stripe/stripe-go": "Stripe", "database/sql": ""})
	want := map[string]string{
		"t1": "casdoor", "t2": "web", "t3": "server", "t4": "etcdctl", "t5": "freqtrade", "t6": "freqtrade-client",
		"t7": "example.com/a/cmd/worker", "t8": "example.com/b/cmd/worker",
		"t9": "github.com/benbjohnson/litestream/cmd/litestream-vfs", "t10": "litestream-vfs",
		"t11": "example.com/tools/cmd/stripe", "t12": "example.com/multi",
	}
	for id, name := range want {
		if got[id] != name {
			t.Errorf("%s offered as %q, want %q", id, got[id], name)
		}
	}

	// Read through the destination question: the web's request, answered
	// "other: Casdoor", reaches the casdoor program offered by that name,
	// and the asking program is called web.
	get := atlas.SymbolCall{Kind: "invokes_external", Name: "fetch", Line: 7, Column: 9, API: &atlas.CallAPI{Package: "platform:javascript", Name: "fetch"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "url", Position: 1, Anchor: &sourcevalue.Anchor{Path: "web/src/backend.js", Line: 5, Column: 11}}}}}
	call := atlas.Place{ID: "symbol:fetchUser", Kind: atlas.PlaceSymbol, Path: "web/src/backend.js", LineNo: 5, Parent: "file:backend", TargetIDs: []string{"t2"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:fetchUser", Name: "fetchUser"}, Calls: []atlas.SymbolCall{get}}}
	route := atlas.Place{ID: "fact:user", Kind: atlas.PlaceBoundary, Path: "controllers/user.go", LineNo: 3, Column: 2, Parent: "file:user", TargetIDs: []string{"t1"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "t1", FactID: "user"}}, Method: "GET", Values: []string{"/api/get-user"}, Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryRequest}}
	graph := []atlas.Place{call, route}
	var mu sync.Mutex
	var offered, asking any
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
			context := input["context"].(map[string]any)
			offered, asking = context["destination_catalog"], context["program"]
			for i := range input["rows"].([]any) {
				rows[i]["destination"] = "other: Casdoor"
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{{ID: "t1", Name: "github.com/casdoor/casdoor", Root: ".", Language: "go", Executables: []string{"casdoor"}}, {ID: "t2", Name: "web", Root: "web", Language: "javascript"}}
	r.opts.ReadSource = func(string) ([]byte, error) { return nil, nil }
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"platform:javascript.fetch": {talks: atlas.BoundaryClientRequest}}
	r.arguments = map[string]ArgumentChoice{"platform:javascript.fetch": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if catalog := string(mustJSON(offered)); !strings.Contains(catalog, `"ref":"d1","takes":["request GET /api/get-user"],"value":"casdoor"`) || asking != "web" {
		t.Fatalf("offered %s to %v, want the program casdoor to web", catalog, asking)
	}
	var found bool
	for _, state := range r.boundaries {
		if state.place.Boundary.Direction == atlas.DirectionOut && state.kind == atlas.BoundaryClientRequest {
			found = true
			t.Logf("web's request reaches %q, program %q", state.destinationOf("t2"), state.destinationTargets["t2"])
			if state.destinationOf("t2") != "casdoor" || state.destinationTargets["t2"] != "t1" {
				t.Fatalf("web's request answered \"other: Casdoor\" reaches %q (%v), want the program casdoor", state.destinationOf("t2"), state.destinationTargets)
			}
		}
	}
	if !found {
		t.Fatal("web's request made no outgoing boundary")
	}
}

// A destination ref the model chose stands: a system's ref is that system
// and a program's that program, whatever the cell's name matches. Only a
// free name may choose a program, and only by the program's offered name
// written as offered (case aside), when no system of the catalogue has that
// name too. A free name is never read through a program's import path:
// "Casdoor" is not github.com/casdoor/casdoor, whose tail proves no
// runtime endpoint (review 2026-10-02, item 1).
func TestAChosenDestinationRefStandsAndOnlyAFreeNameMayNameAProgram(t *testing.T) {
	cases := []struct {
		name    string
		catalog []lines.Destination
		ref     string
		cell    string
		want    string
	}{
		// Negative: an explicitly chosen system ref stays the system.
		{"an explicitly chosen system ref", []lines.Destination{{Ref: "d1", Value: "Casdoor"}, {Ref: "d2", Value: "github.com/casdoor/casdoor", Program: true, Target: "server"}}, "d1", "Casdoor", ""},
		// Negative: a system and a program with the same name.
		{"a system's ref named as a program is", []lines.Destination{{Ref: "d1", Value: "Server"}, {Ref: "d2", Value: "server", Program: true, Target: "server"}}, "d1", "Server", ""},
		{"a free name a system and a program share", []lines.Destination{{Ref: "d1", Value: "Server"}, {Ref: "d2", Value: "server", Program: true, Target: "server"}}, "other: server", "server", ""},
		// Negative: two programs with the same path tail.
		{"a free name two programs' paths end in", []lines.Destination{{Ref: "d1", Value: "example.com/a/server", Program: true, Target: "t1"}, {Ref: "d2", Value: "example.com/b/server", Program: true, Target: "t2"}}, "other: server", "server", ""},
		// Negative: a free name is no import path's tail.
		{"a free name ending one program's path", []lines.Destination{{Ref: "d1", Value: "github.com/casdoor/casdoor", Program: true, Target: "t1"}}, "other: Casdoor", "Casdoor", ""},
		// Positive: a program's ref, and its offered name written free.
		{"a program's ref", []lines.Destination{{Ref: "d1", Value: "Server"}, {Ref: "d2", Value: "server", Program: true, Target: "server"}}, "d2", "server", "server"},
		{"a program's offered name written free", []lines.Destination{{Ref: "d1", Value: "Telegram"}, {Ref: "d2", Value: "github.com/casdoor/casdoor", Program: true, Target: "t1"}}, "other: GitHub.com/casdoor/casdoor", "GitHub.com/casdoor/casdoor", "t1"},
	}
	for _, c := range cases {
		target, _ := lines.DestinationTarget(c.catalog, c.ref, c.cell)
		t.Logf("%s: %q chose %q", c.name, c.ref, target)
		if target != c.want {
			t.Errorf("%s: %q chose the program %q, want %q", c.name, c.ref, target, c.want)
		}
	}
}

// A call into a package this repository builds is no outside system: the
// systems question never names it, and its destination is offered the
// repository's programs, a db call included (etcd's tools call client/v3's
// KV.Get, which the systems question had named "etcd" beside the server
// program it reaches).
func TestACallIntoTheRepositorysOwnPackageIsOfferedItsPrograms(t *testing.T) {
	get := atlas.SymbolCall{Kind: "invokes_external", Name: "Get", Line: 7, Column: 9, API: &atlas.CallAPI{Package: "example.com/kv/client", Receiver: "KV", Name: "Get"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "key", Position: 1, Anchor: &sourcevalue.Anchor{Path: "tool/main.go", Line: 5, Column: 11}}}}}
	read := atlas.Place{ID: "symbol:Read", Kind: atlas.PlaceSymbol, Path: "tool/main.go", LineNo: 5, Parent: "file:main", TargetIDs: []string{"tool"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:Read", Name: "Read"}, Calls: []atlas.SymbolCall{get}}}
	route := atlas.Place{ID: "fact:range", Kind: atlas.PlaceBoundary, Path: "server/kv.go", LineNo: 3, Column: 2, Parent: "file:kv", TargetIDs: []string{"server"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "server", FactID: "range"}}, Values: []string{"/etcdserverpb.KV/Range"}, Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryRequest}}
	graph := []atlas.Place{read, route}
	var mu sync.Mutex
	var asked []string
	var offered []any
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == "atlas_systems" {
			for i, source := range input["rows"].([]any) {
				asked = append(asked, source.(map[string]any)["package"].(string))
				rows[i]["system"] = "none"
			}
			return
		}
		for _, column := range input["fill"].([]any) {
			if column.(map[string]any)["name"] != "destination" {
				continue
			}
			offered, _ = input["context"].(map[string]any)["destination_catalog"].([]any)
			for i := range input["rows"].([]any) {
				rows[i]["destination"] = destinationRef(input, "example.com/kv/server")
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{{ID: "client", Language: "go", Kind: "library", Name: "example.com/kv/client", Root: "client"},
		{ID: "server", Language: "go", Kind: "executable", Name: "example.com/kv/server", Root: "server"}, {ID: "tool", Language: "go", Kind: "executable", Name: "example.com/kv/tool", Root: "tool"}}
	r.opts.ReadSource = func(string) ([]byte, error) { return nil, nil }
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"example.com/kv/client.KV.Get": {talks: atlas.BoundaryDB}}
	r.arguments = map[string]ArgumentChoice{"example.com/kv/client.KV.Get": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(asked) > 0 {
		t.Fatalf("the systems question named the repository's own package: %q", asked)
	}
	if !strings.Contains(string(mustJSON(offered)), `"program":true`) {
		t.Fatalf("the call into the repository's client was offered %s, want the server program", mustJSON(offered))
	}
	for _, state := range r.boundaries {
		if state.place.Boundary.Direction == atlas.DirectionOut && state.destinationTargets["tool"] != "server" {
			t.Fatalf("the tool's KV.Get reaches %q (%v), want the server program", state.destinationOf("tool"), state.destinationTargets)
		}
	}
}
