package reading

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestBoundaryReviewOwnsRuntimeRelationshipsAndKeepsIndependentRows(t *testing.T) {
	const address = "  https://거래.example/시세/%2F  "
	const purpose = "Sends requests to the peer service with a destination resolved from runtime configuration, preserving the request context and payload.\n\nThe supplied declaration does not establish the final host or deployment topology."
	owner := func(id string, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: id + ".go", Parent: "file:" + id, LineNo: 10, TargetIDs: []string{"service"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:" + id, Name: id, Kind: "function", Signature: "func()", Doc: "Original author documentation."}, Calls: calls}}
	}
	call := func(name string, line, column int, values ...string) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: column, Values: values}
	}
	send := owner("send", call("http.NewRequestWithContext", 20, 9, address), call("http.Client.Do", 21, 11), call("time.Sleep", 21, 44))
	exporter := owner("exporter", call("otlptracehttp.New", 12, 4), call("otlptracehttp.WithEndpoint", 12, 30, "collector.internal:4318"))
	send.Symbol.Calls[0].API = &atlas.CallAPI{Package: "net/http", Name: "NewRequestWithContext"}
	send.Symbol.Calls[0].SourceArguments = []atlas.SourceArgument{{Position: 3, Origin: &sourcevalue.Value{Kind: "literal", Text: address}}}
	send.Symbol.Calls[1].API = &atlas.CallAPI{Package: "net/http", Receiver: "Client", Name: "Do"}
	send.Symbol.Calls[1].SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "send.go", Line: 20, Column: 9}}}}
	const otlp = "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	exporter.Symbol.Calls[0].API = &atlas.CallAPI{Package: otlp, Name: "New"}
	exporter.Symbol.Calls[0].SourceArguments = []atlas.SourceArgument{{Position: 2, Origin: &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "exporter.go", Line: 12, Column: 30}}}}
	exporter.Symbol.Calls[1].API = &atlas.CallAPI{Package: otlp, Name: "WithEndpoint"}
	exporter.Symbol.Calls[1].SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "collector.internal:4318"}}}
	setup := owner("setup", call("http.NewRequest", 12, 7, "https://unused.example"))
	unknown := owner("unknown", call("Opaque.Apply", 12, 5))
	bad := owner("bad", call("http.Client.Do", 12, 8))
	dynamic := owner("dynamic", call("http.Client.Do", 12, 9))
	provider := &mutatedTableProvider{}
	var inspected int
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		sourceRows := input["rows"].([]any)
		for i, row := range rows {
			source := sourceRows[i].(map[string]any)
			original := source["owner"].(map[string]any)
			if original["author_doc"] != "Original author documentation." {
				t.Error("author evidence lost")
			}
			if _, present := source["file_hypothesis"]; present {
				t.Error("boundary depends on a caption")
			}
			inspected++
			row["decision"], row["kind"], row["line"] = "boundary", "http_client", purpose
			row["destination"], row["basis"], row["address"] = "Peer service", "dispatch", "unknown"
			switch source["caller"] {
			case "send":
				if len(original["calls"].([]any)) != 3 {
					t.Error("owning callable calls truncated")
				}
				if source["external"] == "time.Sleep" {
					row["decision"] = "none"
					row["kind"] = 42
					row["address"] = false
				} else {
					row["address"] = "a1"
				}
			case "exporter":
				if len(original["calls"].([]any)) != 2 {
					t.Error("exporter lost its configuration observation")
				}
				row["destination"], row["basis"], row["line"], row["address"] = "Trace collector", "remote_client_instance", "Configures trace export to the collector.", "a1"
			case "setup":
				row["decision"] = "none"
				row["destination"] = false
			case "unknown":
				row["decision"] = "unassessed"
				row["line"] = nil
			case "bad":
				row["address"] = "a999"
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = []atlas.Place{send, exporter, setup, unknown, bad, dynamic}
	r.places = make(map[string]atlas.Place)
	r.knowledge = make(map[string]*Knowledge)
	r.knowledgeSubjects = make(map[string]*Knowledge)
	r.responseTables = make(map[string]rememberedTable)
	r.outbound = map[string][]atlas.SymbolCall{"send": send.Symbol.Calls[1:], "exporter": exporter.Symbol.Calls[:1], "setup": setup.Symbol.Calls, "unknown": unknown.Symbol.Calls, "bad": bad.Symbol.Calls, "dynamic": dynamic.Symbol.Calls}
	r.boxOf = make(map[string]string)
	for _, p := range r.opts.Graph.Places {
		r.places[p.ID] = p
		r.boxOf[p.Parent] = "box"
	}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if inspected != 7 || len(r.boundaries) != 3 || len(r.rejected) != 1 {
		t.Fatalf("rows=%d boundaries=%+v rejected=%+v", inspected, r.boundaries, r.rejected)
	}
	projected := r.target(TargetMeta{ID: "service"})
	byCaller := make(map[string]atlas.Boundary)
	for _, b := range projected.Boundaries {
		byCaller[b.Caller] = b
	}
	if b := byCaller["send"]; b.Address != address || b.Column != 11 || b.Source != "model" || b.Kind != "http_client" || b.Destination != "Peer service" || b.Basis != "dispatch" || b.Line != purpose || len(b.Values) != 0 {
		t.Fatalf("source or positive claim lost: %+v", b)
	}
	if b := byCaller["exporter"]; b.Address != "collector.internal:4318" || b.Basis != "configuration" || b.Destination != "Trace collector" {
		t.Fatalf("configuration became dispatch: %+v", b)
	}
	if b := byCaller["dynamic"]; b.Address != "" || b.Basis != "dispatch" {
		t.Fatalf("unknown runtime address invented: %+v", b)
	}
	decisions := map[string]string{}
	for _, k := range r.knowledge {
		decisions[k.Path] = k.Cells["decision"]
	}
	if decisions["setup.go"] != "none" || decisions["unknown.go"] != "unassessed" {
		t.Fatalf("negative assessments not retained: %v", decisions)
	}
	// Every refused/negative source remains available in the original graph.
	if len(r.opts.Graph.Places) != 6 || len(r.opts.Graph.Places[0].Symbol.Calls) != 3 {
		t.Fatal("review rewrote source observations")
	}
}

func TestBoundaryNativeFactSurvivesRefusedProseAndSameLineCallsKeepColumns(t *testing.T) {
	native := atlas.Place{ID: "native", Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 20, Column: 9, Parent: "file:main", TargetIDs: []string{"service"}, Given: "GET https://native.example", Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "fact"}}, ObjectID: "caller", Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryHTTPClient, Method: "GET", Values: []string{"https://native.example"}}}
	first := atlas.SymbolCall{Name: "http.Get", Kind: "invokes_external", Line: 20, Column: 9}
	second := atlas.SymbolCall{Name: "http.Get", Kind: "invokes_external", Line: 20, Column: 42}
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
	r.outbound = map[string][]atlas.SymbolCall{symbol.ID: {first, second}}
	r.boundaries = map[string]*boundaryState{native.ID: {place: native, kind: atlas.BoundaryHTTPClient}}
	r.bindInterpretedBoundaries()
	if len(r.boundaries) != 2 {
		t.Fatalf("same-line call collapsed or exact native call duplicated: %+v", r.boundaries)
	}
	var candidateID string
	for id, b := range r.boundaries {
		if id != native.ID {
			candidateID = id
			if b.place.Column != 42 {
				t.Fatalf("call column lost: %+v", b.place)
			}
		}
	}
	// An unrelated earlier selection cannot shift the source call's identity.
	r.outbound[symbol.ID] = []atlas.SymbolCall{second}
	r.boundaries = map[string]*boundaryState{native.ID: {place: native, kind: atlas.BoundaryHTTPClient}}
	r.bindInterpretedBoundaries()
	if r.boundaries[candidateID] == nil {
		t.Fatal("call identity depends on selected-list order")
	}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(r.boundaries) != 1 || !reflect.DeepEqual(r.boundaries[native.ID].place, native) || r.boundaries[native.ID].kind != atlas.BoundaryHTTPClient || r.boundaries[native.ID].line != native.Given || r.boundaries[native.ID].address != native.Boundary.Values[0] || r.boundaries[native.ID].basis != "dispatch" {
		t.Fatalf("refused prose deleted source fact: %+v", r.boundaries)
	}
	if len(r.rejected) != 1 || !strings.Contains(r.rejected[0].Reason, "line") {
		raw, _ := json.Marshal(r.rejected)
		t.Fatalf("fixed fact prose was not locally validated: %s", raw)
	}
}

func TestBoundaryDefinitionsKeepPositiveAndNegativeCellsConditional(t *testing.T) {
	for _, out := range []bool{false, true} {
		def := lines.Boundaries(out)
		for _, column := range def.Columns[1:] {
			if column.When["decision"] != "boundary" {
				t.Fatalf("%s required on a negative result", column.Name)
			}
		}
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
			row["destination"], row["basis"], row["address"] = "Peer service", "remote_client_instance", "unknown"
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
	file := atlas.Place{ID: "file:requests", Kind: atlas.PlaceFile, Path: "utils/requests.go", Parent: directory.ID, File: &atlas.FileFacts{Doc: "Preserve cancellation when forwarding a request."}}
	call := atlas.SymbolCall{Kind: "invokes_external", Name: "http.Client.Do", Line: 31, Column: 19}
	owner := atlas.Place{ID: "symbol:send", Kind: atlas.PlaceSymbol, Path: file.Path, Parent: file.ID, LineNo: 10, TargetIDs: []string{"service"}, Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{ObjectID: "object:send", Name: "SendRequest", Signature: "func SendRequest(ctx Context, endpoint string)"}, Calls: []atlas.SymbolCall{call},
		CalledBy: []atlas.SymbolCaller{
			{PlaceID: "symbol:proxy", ObjectID: "object:proxy", Name: "Proxy", Signature: "func Proxy(ctx Context)", Path: "server/gen_server.go", Line: 32, Kind: "calls", Resolution: "resolved"},
			{PlaceID: "symbol:proxy", ObjectID: "object:proxy", Name: "Proxy", Signature: "func Proxy(ctx Context)", Path: "server/gen_server.go", Line: 41, Kind: "calls", Resolution: "resolved"},
			{ObjectID: "object:refresh", Name: "Refresh", Signature: "func Refresh()", Path: "jobs/refresh.go", Line: 22, Kind: "calls", Resolution: "alternatives"},
			{Name: "Proxy", Signature: "func Proxy()", Path: "another/server.go", Line: 18, Kind: "calls", Resolution: "unresolved"},
		}}}
	binding := atlas.SymbolBinding{From: "Routes", To: "Proxy", Path: "server/routes.go", Line: 9, Invocation: "callback_transfer", Resolution: "resolved",
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
		context := source["source_context"].(map[string]any)
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
		if options := source["address_options"].([]any); len(options) != 1 || options[0] != "unknown" {
			t.Fatalf("caller/README clues became address choices: %+v", options)
		}
		rows[0]["decision"], rows[0]["kind"], rows[0]["line"] = "boundary", "http_client", "The caller documentation suggests forwarding requests and refreshing snapshots; the final destination remains unknown."
		rows[0]["destination"], rows[0]["basis"], rows[0]["address"] = "Remote endpoints", "dispatch", "unknown"
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.outbound = map[string][]atlas.SymbolCall{owner.ID: {call}}
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
