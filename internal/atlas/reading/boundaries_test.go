package reading

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

func TestBoundaryNativeFactSurvivesRefusedProseAndSameLineCallsKeepColumns(t *testing.T) {
	native := atlas.Place{ID: "native", Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 20, Column: 9, Parent: "file:main", TargetIDs: []string{"service"}, Given: "GET https://native.example", Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "fact"}}, ObjectID: "caller", Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryHTTPClient, Method: "GET", Values: []string{"https://native.example"}}}
	first := atlas.SymbolCall{Name: "http.Get", Kind: "invokes_external", Line: 20, Column: 9, API: &atlas.CallAPI{Package: "net/http", Name: "Get"}}
	second := atlas.SymbolCall{Name: "http.Get", Kind: "invokes_external", Line: 20, Column: 42, API: &atlas.CallAPI{Package: "net/http", Name: "Get"}}
	symbol := atlas.Place{ID: "owner", Kind: atlas.PlaceSymbol, Path: "main.go", LineNo: 10, Parent: "file:main", TargetIDs: []string{"service"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "caller", Name: "send"}, Calls: []atlas.SymbolCall{first, second}}}
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		sources := input["rows"].([]any)
		for i, row := range rows {
			row["decision"] = "none"
			if sources[i].(map[string]any)["kind_given"] != nil {
				row["line"] = 42 // Refused prose must not remove the fixed fact.
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = []atlas.Place{native, symbol}
	r.places = map[string]atlas.Place{native.ID: native, symbol.ID: symbol}
	r.knowledge = map[string]*Knowledge{}
	r.knowledgeSubjects = map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"net/http.Get": {talks: atlas.BoundaryHTTPClient}}
	r.boundaries = map[string]*boundaryState{native.ID: {place: native, kind: atlas.BoundaryHTTPClient}}
	r.bindInterpretedBoundaries()
	if len(r.boundaries) != 2 {
		t.Fatalf("same-line call collapsed or exact native call duplicated: %+v", r.boundaries)
	}
	for id, b := range r.boundaries {
		if id != native.ID && (b.place.Column != 42 || b.kind != atlas.BoundaryHTTPClient) {
			t.Fatalf("call column or its symbol's kind lost: %+v", b.place)
		}
	}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(r.boundaries) != 2 || !reflect.DeepEqual(r.boundaries[native.ID].place, native) || r.boundaries[native.ID].kind != atlas.BoundaryHTTPClient || r.boundaries[native.ID].line != native.Given || r.boundaries[native.ID].address != native.Boundary.Values[0] || r.boundaries[native.ID].basis != "dispatch" {
		t.Fatalf("refused prose deleted source fact: %+v", r.boundaries)
	}
	// Each outbound row loses only its refused line; its destination and
	// address decisions stand beside it.
	if len(r.rejected) != 2 || r.rejected[0].Kind != "cell_rejected" || !strings.Contains(r.rejected[0].Reason, `"line"`) || r.rejected[1].Kind != "cell_rejected" {
		raw, _ := json.Marshal(r.rejected)
		t.Fatalf("fixed fact prose was not locally validated: %s", raw)
	}
}

func TestFixedConfigurationAndIncomingFactsNeedOnlyTheirExplanation(t *testing.T) {
	config := atlas.Place{ID: "config", Kind: atlas.PlaceBoundary, Path: "config.py", LineNo: 12, Parent: "file:config",
		Given: "SOURCE_CONFIG", Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "source:config"}}, Direction: atlas.DirectionOut,
			GivenKind: atlas.BoundaryConfig, Values: []string{"SOURCE_CONFIG"}}}
	route := atlas.Place{ID: "route", Kind: atlas.PlaceBoundary, Path: "routes.ts", LineNo: 21, Parent: "file:routes",
		Given: "GET /metrics", Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "source:route"}}, Direction: atlas.DirectionIn,
			GivenKind: atlas.BoundaryHTTPServer, Method: "GET", Values: []string{"/metrics"}}}
	provider := &mutatedTableProvider{}
	inspected := 0
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		fill := input["fill"].([]any)
		if len(fill) != 1 || fill[0].(map[string]any)["name"] != "line" {
			t.Fatalf("fixed native facts asked to establish an external exchange: %+v", fill)
		}
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			if row["decision_options"] != nil || row["kind_options"] != nil {
				t.Fatalf("fixed native choices reached provider: %+v", row)
			}
			inspected++
			rows[i]["line"] = "Explains the supplied observation."
			// Unrequested extra cells cannot override the native properties.
			rows[i]["decision"], rows[i]["kind"] = "none", "sdk"
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = []atlas.Place{config, route}
	r.places = map[string]atlas.Place{config.ID: config, route.ID: route}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if inspected != 2 || len(r.boundaries) != 2 || len(r.rejected) != 0 {
		t.Fatalf("native prose coverage lost: inspected=%d boundaries=%+v rejected=%+v", inspected, r.boundaries, r.rejected)
	}
	for _, original := range []atlas.Place{config, route} {
		state := r.boundaries[original.ID]
		if !reflect.DeepEqual(state.place, original) || state.kind != original.Boundary.GivenKind || state.line != "Explains the supplied observation." {
			t.Fatalf("model classified a fixed fact: %+v", state)
		}
	}
}

func TestBoundaryKnownHTTPAddressSurvivesAcceptedUnknownAndPreservesSourceAnchor(t *testing.T) {
	const address = "https://실제.example/경로/%2F"
	native := atlas.Place{ID: "native", Kind: atlas.PlaceBoundary, Path: "client.go", LineNo: 27, Column: 19,
		Parent: "file:client", TargetIDs: []string{"service"}, Given: "GET " + address,
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "original-http"}}, Direction: atlas.DirectionOut,
			GivenKind: atlas.BoundaryHTTPClient, Method: "GET", Values: []string{address}}}
	provider := &mutatedTableProvider{}
	provider.mutate = func(_ map[string]any, rows []map[string]any) {
		for _, row := range rows {
			row["decision"], row["kind"], row["line"] = "boundary", "http_client", "Requests the configured peer."
			row["destination"], row["basis"], row["address"] = "other: Peer service", "remote_client_instance", "unknown"
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = []atlas.Place{native}
	r.places = map[string]atlas.Place{native.ID: native}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.boxOf = map[string]string{native.Parent: "client"}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := r.target(TargetMeta{ID: "service"}).Boundaries
	if len(got) != 1 || got[0].Address != address || got[0].Basis != "dispatch" || got[0].Path != native.Path || got[0].LineNo != 27 || got[0].Column != 19 || got[0].FactID != "original-http" {
		t.Fatalf("model uncertainty overwrote known source facts: %+v", got)
	}
}

func TestBoundaryPurposeReadsOnlyNativeImmediateCallersAndOwnAncestorDocuments(t *testing.T) {
	root := atlas.Place{ID: "dir:root", Kind: atlas.PlaceDirectory, Path: ".", Directory: &atlas.DirectoryFacts{Readme: "A service that proxies partner data and refreshes snapshots."}}
	directory := atlas.Place{ID: "dir:utils", Kind: atlas.PlaceDirectory, Path: "utils", Parent: root.ID, Directory: &atlas.DirectoryFacts{Doc: "Shared request helpers."}}
	file := atlas.Place{ID: "file:requests", Kind: atlas.PlaceFile, Path: "utils/requests.go", Parent: directory.ID, TargetIDs: []string{"service"}, File: &atlas.FileFacts{Doc: "Preserve cancellation when forwarding a request."}}
	call := atlas.SymbolCall{Kind: "invokes_external", Name: "http.Client.Do", Line: 31, Column: 19, API: &atlas.CallAPI{Package: "net/http", Receiver: "*Client", Name: "Do"}}
	owner := atlas.Place{ID: "symbol:send", Kind: atlas.PlaceSymbol, Path: file.Path, Parent: file.ID, LineNo: 10, TargetIDs: []string{"service"}, Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{ObjectID: "object:send", Name: "SendRequest", Signature: "func SendRequest(ctx Context, endpoint string)"}, Calls: []atlas.SymbolCall{call},
		CalledBy: []atlas.SymbolCaller{
			{PlaceID: "symbol:proxy", ObjectID: "object:proxy", Name: "Proxy", Signature: "func Proxy(ctx Context)", Path: "server/gen_server.go", Line: 32, Kind: "calls", Resolution: "resolved"},
			{PlaceID: "symbol:proxy", ObjectID: "object:proxy", Name: "Proxy", Signature: "func Proxy(ctx Context)", Path: "server/gen_server.go", Line: 41, Kind: "calls", Resolution: "resolved"},
			{ObjectID: "object:refresh", Name: "Refresh", Signature: "func Refresh()", Path: "jobs/refresh.go", Line: 22, Kind: "calls", Resolution: "alternatives"},
			{Name: "Proxy", Signature: "func Proxy()", Path: "another/server.go", Line: 18, Kind: "calls", Resolution: "unresolved"},
		}}}
	binding := atlas.SymbolBinding{From: "Routes", To: "Proxy", Path: "server/routes.go", Line: 9, Kind: "passes_callback", Resolution: "resolved",
		Arguments: []atlas.RegistrationArgument{{Kind: "literal_string", Value: "/proxy", Path: "server/routes.go", Line: 9, Position: 1}}}
	proxy := atlas.Place{ID: "symbol:proxy", Kind: atlas.PlaceSymbol, Path: "server/gen_server.go", LineNo: 20, Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{ObjectID: "object:proxy", Name: "Proxy", Doc: "Forwards the caller's request to the configured partner."}, Bindings: []atlas.SymbolBinding{binding},
		Calls: []atlas.SymbolCall{{Name: "unexpanded-callee-marker"}}, CalledBy: []atlas.SymbolCaller{{PlaceID: "symbol:grandparent", Name: "grandparent-marker"}}}}
	refresh := atlas.Place{ID: "symbol:refresh", Kind: atlas.PlaceSymbol, Path: "jobs/refresh.go", LineNo: 12, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:refresh", Name: "Refresh", Doc: "Refreshes the snapshot from the upstream source."}}}
	grandparent := atlas.Place{ID: "symbol:grandparent", Kind: atlas.PlaceSymbol, Path: "main.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:grandparent", Doc: "grandparent-document-marker"}}}
	otherDirectory := atlas.Place{ID: "dir:other", Kind: atlas.PlaceDirectory, Path: "utils-other", Parent: root.ID, Directory: &atlas.DirectoryFacts{Readme: "unrelated-readme-marker"}}
	graph := []atlas.Place{root, directory, file, owner, proxy, refresh, grandparent, otherDirectory}
	before, _ := json.Marshal(graph)
	provider := &mutatedTableProvider{}
	inspected := 0
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		inspected += len(rows)
		if len(rows) != 1 {
			t.Fatalf("caller contexts created extra boundaries: %d", len(rows))
		}
		source := input["rows"].([]any)[0].(map[string]any)
		context := windowOwner(input)["source_context"].(map[string]any)
		ownFile := context["file"].(map[string]any)
		if ownFile["path"] != file.Path || ownFile["author_doc"] != file.File.Doc {
			t.Fatalf("own file evidence lost: %+v", ownFile)
		}
		ancestors := context["ancestor_directories"].([]any)
		if len(ancestors) != 2 || ancestors[0].(map[string]any)["path"] != directory.Path || ancestors[1].(map[string]any)["readme_claim"] != root.Directory.Readme {
			t.Fatalf("actual ancestor documentation lost: %+v", ancestors)
		}
		callers := context["immediate_callers"].([]any)
		if len(callers) != 3 {
			t.Fatalf("native callers were merged by name or duplicated by site: %+v", callers)
		}
		for _, item := range callers {
			caller := item.(map[string]any)
			sites := caller["call_sites"].([]any)
			switch caller["path"] {
			case proxy.Path:
				if caller["author_doc"] != proxy.Symbol.Decl.Doc || len(sites) != 2 || caller["callable_bindings"] == nil {
					t.Fatalf("proxy purpose/registration/sites lost: %+v", caller)
				}
				encoded, _ := json.Marshal(caller["callable_bindings"])
				if !strings.Contains(string(encoded), `"value":"/proxy"`) || !strings.Contains(string(encoded), `"path":"server/routes.go"`) {
					t.Fatalf("native registration literal lost: %s", encoded)
				}
			case refresh.Path:
				if caller["author_doc"] != refresh.Symbol.Decl.Doc || sites[0].(map[string]any)["resolution"] != "alternatives" {
					t.Fatalf("object identity or possible dispatch changed: %+v", caller)
				}
			case "another/server.go":
				if caller["author_doc"] != nil || caller["callable_bindings"] != nil {
					t.Fatalf("matching name borrowed another declaration: %+v", caller)
				}
			default:
				t.Fatalf("unexpected caller: %+v", caller)
			}
		}
		encoded, _ := json.Marshal(input)
		for _, forbidden := range []string{"grandparent-marker", "grandparent-document-marker", "unexpanded-callee-marker", "unrelated-readme-marker", "symbol:proxy", "object:refresh"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Errorf("unrelated source or native identity leaked: %s", forbidden)
			}
		}
		if source["address_options"] != nil || source["address_catalog"] != nil {
			t.Fatalf("caller/README clues became address choices: %+v", source)
		}
		if strings.Contains(string(mustJSON(context["immediate_callers"])), "result_value") {
			t.Fatal("caller call sites carried result value trees")
		}
		rows[0]["decision"], rows[0]["kind"], rows[0]["line"] = "boundary", "http_client", "forwards requests and refreshes snapshots · not established: destination"
		rows[0]["destination"], rows[0]["basis"], rows[0]["address"] = "other: Remote endpoints", "dispatch", "unknown"
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"net/http.Client.Do": {talks: atlas.BoundaryHTTPClient}}
	r.boxOf = map[string]string{file.ID: "requests"}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := r.target(TargetMeta{ID: "service"}).Boundaries
	after, _ := json.Marshal(r.opts.Graph.Places)
	if inspected != 1 || len(got) != 1 || got[0].Path != file.Path || got[0].LineNo != 31 || got[0].Column != 19 || got[0].Address != "" || string(before) != string(after) {
		t.Fatalf("source graph, boundary count or original call anchor changed: %+v", got)
	}
}

// windowOwner is the one declaration a boundary window shares.
func windowOwner(input map[string]any) map[string]any {
	context, _ := input["context"].(map[string]any)
	owners, _ := context["owners"].([]any)
	if len(owners) != 1 {
		return nil
	}
	owner, _ := owners[0].(map[string]any)
	return owner
}

// destinationRef finds the window's ref of a listed runtime system.
func destinationRef(input map[string]any, system string) string {
	context, _ := input["context"].(map[string]any)
	catalog, _ := context["destination_catalog"].([]any)
	for _, item := range catalog {
		entry, _ := item.(map[string]any)
		if entry["value"] == system {
			return entry["ref"].(string)
		}
	}
	return ""
}

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func TestColumnlessNativeFactClaimsItsLineAndColumnedFactKeepsOtherCalls(t *testing.T) {
	// An SDK boundary from an external_call observation carries no column.
	// Morfeu 20260911-152759 reviewed internal/broker/client.go:166 twice,
	// as bnd:…:166:sdk and as out:…PublicarComConfirm:166:42, and the
	// outbound page listed the call twice.
	call := atlas.SymbolCall{Name: "amqp091.Channel.PublishWithDeferredConfirm", Kind: "invokes_external", Line: 166, Column: 42, API: &atlas.CallAPI{Package: "github.com/rabbitmq/amqp091-go", Receiver: "*Channel", Name: "PublishWithDeferredConfirm"}}
	symbol := atlas.Place{ID: "owner", Kind: atlas.PlaceSymbol, Path: "client.go", LineNo: 143, Parent: "file:client", TargetIDs: []string{"service"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "caller", Name: "Client.PublicarComConfirm"}, Calls: []atlas.SymbolCall{call}}}
	// The SDK observation itself arrives with source "external_call", not
	// "fact" (places.go); run 20260911-171727 kept all four duplicates while
	// the claim matched "fact" alone.
	fact := func(column int) atlas.Place {
		return atlas.Place{ID: "native", Kind: atlas.PlaceBoundary, Path: "client.go", LineNo: 166, Column: column, Parent: "file:client", TargetIDs: []string{"service"},
			Given: "RabbitMQ broker", Boundary: &atlas.BoundaryFacts{Source: "external_call", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "sdk"}},
				ObjectID: "caller", Direction: atlas.DirectionOut, GivenKind: atlas.BoundarySDK}}
	}
	for _, test := range []struct {
		name       string
		column     int
		boundaries int
	}{
		{"no column claims every call on the line", 0, 1},
		{"the call's own column claims it", 42, 1},
		{"another column leaves the call its own review", 9, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			native := fact(test.column)
			r := &reader{
				opts:       Options{Graph: atlas.Graph{Places: []atlas.Place{native, symbol}}},
				places:     map[string]atlas.Place{native.ID: native, symbol.ID: symbol},
				api:        map[string]apiRole{"github.com/rabbitmq/amqp091-go.Channel.PublishWithDeferredConfirm": {talks: atlas.BoundarySDK}},
				boundaries: map[string]*boundaryState{native.ID: {place: native, kind: atlas.BoundarySDK}},
			}
			r.bindInterpretedBoundaries()
			if len(r.boundaries) != test.boundaries {
				t.Fatalf("boundaries = %d, want %d: %+v", len(r.boundaries), test.boundaries, r.boundaries)
			}
		})
	}
}

// The registrations of one holder: a route put into a router, the router
// started on an address, a callback handed to a sorter, a driver opened.
// The symbol roles say what each is; the holder gives the route its address.
func TestSymbolRolesTurnRegistrationsIntoBoundariesAndHoldersCarryAddresses(t *testing.T) {
	registration := func(id, external string, direction string, holder string, handler string, values ...string) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 20, Column: len(id), Parent: "file:main", TargetIDs: []string{"api"}, Given: external,
			Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "api", FactID: "fact:" + id}}, ObjectID: handler, Caller: "main", External: external, Holder: holder, Values: values, Direction: direction}}
	}
	route := registration("b1", "echo.Echo.GET", atlas.DirectionIn, "main.go:19:20", "handler", "/users/:id")
	route.Boundary.Method = "GET"
	start := registration("b2", "echo.Echo.Start", atlas.DirectionOut, "main.go:19:20", "", ":8080")
	comparator := registration("b3", "sort.Slice", atlas.DirectionIn, "", "less")
	open := registration("b4", "database/sql.Open", atlas.DirectionOut, "", "", "postgres")
	r := answerTestReader(t, nil, nil)
	r.dry, r.opts.Through = true, ""
	r.opts.Graph.Places = []atlas.Place{route, start, comparator, open}
	r.places = map[string]atlas.Place{}
	for _, place := range r.opts.Graph.Places {
		r.places[place.ID] = place
	}
	r.api = map[string]apiRole{
		"echo.Echo.GET":     {binds: atlas.BoundaryHTTPServer},
		"echo.Echo.Start":   {publishes: true},
		"database/sql.Open": {talks: atlas.BoundaryDB},
	}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(r.boundaries) != 3 || r.boundaries["b3"] != nil {
		t.Fatalf("a callback handed to a symbol that binds nothing survived: %+v", r.boundaries)
	}
	if got := r.boundaries["b1"]; got.kind != atlas.BoundaryHTTPServer || got.address != ":8080" {
		t.Fatalf("route did not take its symbol's kind and its holder's address: %+v", got)
	}
	if got := r.boundaries["b2"]; got.kind != atlas.BoundaryListenAddress || got.place.Boundary.Direction != atlas.DirectionIn || got.address != ":8080" {
		t.Fatalf("publishing call is not the listener: %+v", got)
	}
	if got := r.boundaries["b4"]; got.kind != atlas.BoundaryDB || got.place.Boundary.Direction != atlas.DirectionOut {
		t.Fatalf("driver open is not the database boundary: %+v", got)
	}
	if !reflect.DeepEqual(r.apiRoles(), []atlas.APIRole{{Symbol: "database/sql.Open", Talks: "db"}, {Symbol: "echo.Echo.GET", Binds: "http_server"}, {Symbol: "echo.Echo.Start", Publishes: true}}) {
		t.Fatalf("roles recorded differently: %+v", r.apiRoles())
	}
}

// Two calls publish one holder on different addresses. The route on that
// holder takes the address of the first publishing call in boundary order,
// on every reading: map iteration never chooses the address.
func TestTwoPublishesOnOneHolderGiveTheFirstAddress(t *testing.T) {
	registration := func(id, external, direction, handler string, values ...string) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 20, Column: len(id), Parent: "file:main", TargetIDs: []string{"api"}, Given: external,
			Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "api", FactID: "fact:" + id}}, ObjectID: handler, Caller: "main", External: external, Holder: "main.go:19:20", Values: values, Direction: direction}}
	}
	places := []atlas.Place{
		registration("b1", "echo.Echo.GET", atlas.DirectionIn, "handler", "/users/:id"),
		registration("b2", "echo.Echo.Start", atlas.DirectionOut, "", ":8080"),
		registration("b3", "echo.Echo.StartTLS", atlas.DirectionOut, "", ":8443"),
		registration("b4", "echo.Echo.POST", atlas.DirectionIn, "create", "/users"),
	}
	for attempt := 0; attempt < 32; attempt++ {
		r := answerTestReader(t, nil, nil)
		r.dry, r.opts.Through = true, ""
		r.opts.Graph.Places = places
		r.places = map[string]atlas.Place{}
		for _, place := range places {
			r.places[place.ID] = place
		}
		r.api = map[string]apiRole{
			"echo.Echo.GET": {binds: atlas.BoundaryHTTPServer}, "echo.Echo.POST": {binds: atlas.BoundaryHTTPServer},
			"echo.Echo.Start": {publishes: true}, "echo.Echo.StartTLS": {publishes: true},
		}
		if err := r.readBoundaries(t.Context()); err != nil {
			t.Fatal(err)
		}
		if r.boundaries["b1"].address != ":8080" || r.boundaries["b4"].address != ":8080" {
			t.Fatalf("attempt %d: routes took %q and %q, want the first publish's :8080", attempt, r.boundaries["b1"].address, r.boundaries["b4"].address)
		}
	}
}

// A server started elsewhere on a value the code could not follow back to
// the router: the model is shown the holders and picks one.
func TestPublishWithoutFollowedHolderAsksWhichHolderItServes(t *testing.T) {
	route := atlas.Place{ID: "b1", Kind: atlas.PlaceBoundary, Path: "routes.go", LineNo: 12, Column: 4, Parent: "file:routes", TargetIDs: []string{"api"}, Given: "GET /users/:id",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "api", FactID: "fact:route"}}, ObjectID: "handler", Caller: "routes", External: "echo.Echo.GET", Holder: "routes.go:10:8", Method: "GET", Values: []string{"/users/:id"}, Direction: atlas.DirectionIn}}
	start := atlas.Place{ID: "b2", Kind: atlas.PlaceBoundary, Path: "server.go", LineNo: 40, Column: 3, Parent: "file:server", TargetIDs: []string{"api"}, Given: "Start",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "api", FactID: "fact:start"}}, Caller: "Server.Run", External: "echo.Echo.Start", Values: []string{":8080"}, Direction: atlas.DirectionOut}}
	provider := &mutatedTableProvider{}
	asked := 0
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		if input["table"] != lines.StagePublish {
			return
		}
		asked++
		holders := input["context"].(map[string]any)["holders"].([]any)
		if len(holders) != 1 || holders[0].(map[string]any)["holder"] != "routes.go:10:8" || !strings.Contains(string(mustJSON(holders)), "GET /users/:id → routes") {
			t.Fatalf("holders shown differently: %+v", holders)
		}
		rows[0]["holder"] = "h1"
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = []atlas.Place{route, start}
	r.places = map[string]atlas.Place{route.ID: route, start.ID: start}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"echo.Echo.GET": {binds: atlas.BoundaryHTTPServer}, "echo.Echo.Start": {publishes: true}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if asked != 1 || r.boundaries["b1"].address != ":8080" {
		t.Fatalf("asked %d times, route address %q", asked, r.boundaries["b1"].address)
	}
}
