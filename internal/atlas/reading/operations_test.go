package reading

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

// These roles are a local response preset, not a test of model judgement.
// The real regression is that a setup-only caller cannot be offered as a
// fictitious destination for its recipient's independent operation decision.
// Equivalent source observations live in the cumulative runtime_registrations
// fixtures for Go, Python and JavaScript/TypeScript.
func TestOperationOwnershipKeepsLaunchedWorkIndependentOfSetup(t *testing.T) {
	cases := []struct {
		name, path, activation, binding, invocation, doc string
	}{
		{"AddLogHook", "notifications.go", "", "", "", "Installs logging and starts the notification sender."},
		{"sendNotifications", "notifications.go", "continuous", "", "goroutine", "Consumes queued notifications until the channel closes."},
		{"formatBatch", "notifications.go", "", "", "synchronous", "Formats the current notification batch."},
		{"HandleUpdate", "metrics.go", "continuous", "", "goroutine", "Receives metric updates from the channel until shutdown."},
		{"updateContainers", "jobs.go", "scheduled", "cron.AddFunc", "", "Updates containers when the registered cron schedule fires."},
		{"notifyUpgrade", "jobs.go", "scheduled", "time.AfterFunc", "", "Sends one upgrade notice after the configured delay."},
		{"PreRun", "command.go", "", "cobra.Command.PreRun", "", "Prepares flags and logging before the command action."},
		{"serve", "server.go", "", "", "synchronous", "Configures and starts the HTTP listener."},
		{"lifespan", "server.py", "", "FastAPI.lifespan", "", "Starts background tasks, yields, then joins them at shutdown."},
		{"refreshMetrics", "metrics.ts", "scheduled", "setInterval", "", "Publishes current counters on the configured timer."},
	}
	provider := &mutatedTableProvider{}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = nil
	r.places = map[string]atlas.Place{}
	r.operations = map[string][3]string{}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	wants := map[string]string{}
	for _, test := range cases {
		p := atlas.Place{ID: test.name, Kind: atlas.PlaceSymbol, Path: test.path, LineNo: 10,
			Parent: "file:" + test.path, TargetIDs: []string{"service"}, Symbol: &atlas.SymbolFacts{
				Decl: atlas.Decl{ObjectID: "object:" + test.name, Name: test.name, Doc: test.doc},
			}}
		if test.binding != "" {
			p.Symbol.Bindings = []atlas.SymbolBinding{{From: "install", To: test.name, Detail: test.binding, Path: test.path, Line: 8}}
		}
		if test.invocation != "" {
			caller := "AddLogHook"
			if test.name == "formatBatch" {
				caller = "sendNotifications"
			}
			p.Symbol.CalledBy = []atlas.SymbolCaller{{PlaceID: caller, Name: caller, Kind: "calls", Path: "notifications.go", Line: 20, Invocation: test.invocation, Resolution: "exact"}}
		}
		if test.activation == "continuous" {
			p.Symbol.Calls = []atlas.SymbolCall{{Name: "dispatch", Kind: "calls", Line: 12, Invocation: "synchronous",
				Evidence: []atlas.EdgeEvidence{{Extractor: "control_context", Label: "range body over channel", Path: test.path, LineNo: 11}}}}
		}
		r.opts.Graph.Places = append(r.opts.Graph.Places, p)
		r.places[p.ID] = p
		// Force the negative candidates through review as if selection had
		// proposed them. Independent activation evidence supplies the workers.
		if test.activation == "" {
			r.operations[p.ID] = [3]string{"command", "Earlier hypothesis", ""}
		}
		wants[test.name] = test.activation
	}
	inspected := 0
	provider.mutate = func(input map[string]any, answers []map[string]any) {
		if input["table"] != lines.StageOperations {
			t.Fatalf("unexpected table: %v", input["table"])
		}
		for _, field := range input["fill"].([]any) {
			column := field.(map[string]any)
			if column["name"] == "entry" {
				raw, _ := json.Marshal(column["options"])
				if string(raw) != `["self","none"]` || column["options_from"] != nil {
					t.Fatalf("entry depends on a caller decision: %s", raw)
				}
			}
		}
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			name := row["name"].(string)
			inspected++
			if row["entry_options"] != nil {
				t.Fatal("caller assignment options leaked into own-operation row")
			}
			if name == "sendNotifications" {
				callers := row["observed_callers"].([]any)
				caller := callers[0].(map[string]any)
				if caller["name"] != "AddLogHook" || caller["ref"] != nil {
					t.Fatalf("launcher evidence lost or made an assignment ref: %+v", caller)
				}
				raw, _ := json.Marshal(row)
				if !strings.Contains(string(raw), "goroutine") || !strings.Contains(string(raw), "range body over channel") {
					t.Fatalf("worker source context lost: %s", raw)
				}
			}
			answers[i]["entry"] = "none"
			if activation := wants[name]; activation != "" {
				answers[i]["entry"], answers[i]["activation"] = "self", activation
				answers[i]["name_kind"], answers[i]["name"], answers[i]["description"] = "label", name, row["author_doc"]
			}
		}
	}
	if err := r.readOperations(t.Context()); err != nil {
		t.Fatal(err)
	}
	if inspected != len(cases) || len(r.rejected) != 0 {
		t.Fatalf("review coverage=%d rejected=%+v", inspected, r.rejected)
	}
	for name, activation := range wants {
		if got := r.operations[name][0]; got != activation {
			t.Errorf("%s operation=%q, want %q", name, got, activation)
		}
	}
}

type registeredHTTPProvider struct{ tableProvider }

func (p *registeredHTTPProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	response, err := p.tableProvider.Complete(ctx, prepared)
	if err != nil {
		return response, err
	}
	var request struct {
		Table string
		Rows  []map[string]any
	}
	if err = json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return response, err
	}
	if request.Table != lines.StageOperations {
		return response, nil
	}
	var output struct{ Rows []map[string]string }
	if err = json.Unmarshal(response.Response, &output); err != nil {
		return response, err
	}
	for i, row := range request.Rows {
		if row["name"] == "Op01" {
			output.Rows[i]["entry"] = "self"
			output.Rows[i]["activation"] = "request"
			output.Rows[i]["name_kind"] = "http"
			output.Rows[i]["http_method"] = "GET"
			output.Rows[i]["http_path"] = "p1"
			output.Rows[i]["name"] = "GET /invented"
		}
	}
	response.Response, err = json.Marshal(output)
	return response, err
}

func TestOperationRestoresOriginalHTTPPathWithoutTrimmingOrTranslation(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	path := "/고객/" + strings.Repeat("long-segment/", 8) + "%20status"
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.Symbol != nil && place.Symbol.Decl.Name == "Op01" {
			place.Symbol.Bindings = []atlas.SymbolBinding{{From: "Install", To: "Op01", Detail: "Router.Get", Path: place.Path, Line: 5,
				Arguments: []atlas.RegistrationArgument{{Position: 1, Kind: "literal_string", Value: path, Path: place.Path, Line: 5}}}}
		}
	}
	provider := &registeredHTTPProvider{}
	opts := twoTargetOptions(t, graph, &provider.tableProvider)
	opts.Provider, opts.Through = provider, lines.StageJoints
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range result.Atlas.Targets {
		for _, box := range target.Boxes {
			for _, file := range box.Files {
				for _, symbol := range file.Symbols {
					if symbol.Name == "Op01" {
						if symbol.Operation != "GET "+path {
							t.Fatalf("HTTP path was generated or changed: %q", symbol.Operation)
						}
						return
					}
				}
			}
		}
	}
	t.Fatal("HTTP operation disappeared from atlas")
}

// The route fact is the handler's operation; the reading adds no model
// operation beside it, so the group index shows the route once.
func TestNativeHTTPRouteHandlerKeepsItsRouteAndGetsNoModelOperation(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	want := "/api/v1/고객/%20status"
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Name != "Op01" {
			continue
		}
		graph.Places = append(graph.Places, atlas.Place{
			ID: "bnd:customer", Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: 9, Column: 3,
			Parent: place.Parent, TargetIDs: place.TargetIDs,
			Boundary: &atlas.BoundaryFacts{SubjectID: place.ID, Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: place.TargetIDs[0], FactID: "native:customer"}},
				GivenKind: atlas.BoundaryHTTPServer, Direction: atlas.DirectionIn, Method: "GET", Values: []string{want}},
		})
		break
	}
	atlas.SortPlaces(graph.Places)
	provider := &registeredHTTPProvider{}
	opts := twoTargetOptions(t, graph, &provider.tableProvider)
	opts.Provider, opts.Through = provider, lines.StageJoints
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var handler *atlas.Symbol
	routes := 0
	for _, target := range result.Atlas.Targets {
		for _, boundary := range target.Boundaries {
			if boundary.ID == "bnd:customer" && len(boundary.Values) == 1 && boundary.Values[0] == want {
				routes++
			}
		}
		for _, box := range target.Boxes {
			for _, file := range box.Files {
				for i := range file.Symbols {
					if file.Symbols[i].Name == "Op01" {
						handler = &file.Symbols[i]
					}
				}
			}
		}
	}
	if handler == nil || routes != 1 {
		t.Fatalf("native route or its handler disappeared: handler=%+v routes=%d", handler, routes)
	}
	if handler.Activation != "" || handler.Operation != "" || handler.OperationSummary != "" {
		t.Fatalf("route handler received a model operation beside its route: %+v", handler)
	}
}

func TestNativeHTTPRouteCatalogueUsesSubjectIdentityAndRetainsMounts(t *testing.T) {
	first := atlas.Place{ID: "route:first", Path: "routes.py", LineNo: 11, Column: 2,
		Boundary: &atlas.BoundaryFacts{Source: "fact", SubjectID: "handler:first", Caller: "same_name", GivenKind: atlas.BoundaryHTTPServer,
			Direction: atlas.DirectionIn, Method: "GET", Values: []string{"/api/one", "/api/two"}}}
	second := first
	second.ID, second.Boundary = "route:second", &atlas.BoundaryFacts{Source: "fact", SubjectID: "handler:second", Caller: "same_name",
		GivenKind: atlas.BoundaryHTTPServer, Direction: atlas.DirectionIn, Method: "POST", Values: []string{"/other"}}
	listener := first
	listener.ID, listener.Boundary = "listener", &atlas.BoundaryFacts{Source: "fact", SubjectID: "handler:first", GivenKind: atlas.BoundaryHTTPServer,
		Direction: atlas.DirectionIn, Values: []string{":8080"}}
	routes := operationNativeRoutes(atlas.Graph{Places: []atlas.Place{first, second, listener}})
	if len(routes["handler:first"]) != 1 || len(routes["handler:second"]) != 1 {
		t.Fatalf("route identity confused with name/listener: %+v", routes)
	}
	fields, names := operationRegisteredNames([]atlas.SymbolBinding{{Arguments: []atlas.RegistrationArgument{{Kind: "literal_string", Value: "/suffix"}}}}, routes["handler:first"]...)
	if len(names) != 2 || names["p1"] != "/api/one" || names["p2"] != "/api/two" {
		t.Fatalf("mounts were replaced by callback suffix: %+v", names)
	}
	encoded, _ := json.Marshal(fields)
	if !strings.Contains(string(encoded), `"column":2`) || strings.Contains(string(encoded), "/other") || strings.Contains(string(encoded), "/suffix") {
		t.Fatalf("native catalogue lost source or mixed owners: %s", encoded)
	}
}

// Same names are common across transports, client adapters and stores. Only
// the native caller identity may select a declaration, and expansion must not
// recursively copy the entire program into every operation row.
func TestOperationContextUsesNativeCallersWithoutNamePeers(t *testing.T) {
	decl := func(id, path, name string) atlas.Place {
		return atlas.Place{ID: "sym:" + id, Path: path, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: id, Name: name, LineNo: 10}}}
	}
	store := decl("object-store", "store.go", "Store.Add")
	handler := decl("object-handler", "handler.go", "Handler.Add")
	outer := decl("object-outer", "outer.go", "Quota.Add")
	unrelated := decl("object-unrelated", "client.go", "Client.Add")
	handler.Symbol.CalledBy = []atlas.SymbolCaller{{ObjectID: outer.Symbol.Decl.ObjectID, Name: "Quota.Add", Path: outer.Path, Line: 12, Resolution: "alternatives"}}
	handler.Symbol.Calls = []atlas.SymbolCall{{Name: "Log.LargeUnrelatedPayload", Values: []string{"not operation context"}}}
	handler.Symbol.Bindings = []atlas.SymbolBinding{{From: "Install", To: "Handler.Add", Detail: "Action.Run", Evidence: []atlas.EdgeEvidence{{Label: "CallerHelpMustStayOnCaller"}}}}
	store.Symbol.CalledBy = []atlas.SymbolCaller{
		{ObjectID: handler.Symbol.Decl.ObjectID, Name: "Handler.Add", Path: handler.Path, Line: 14, Resolution: "alternatives"},
		{ObjectID: "another-target-handler-id", PlaceID: handler.Symbol.Decl.ObjectID, Name: "Handler.Add", Path: handler.Path, Line: 18, Resolution: "alternatives"},
		{ObjectID: "generated-object", Name: "Generated.Add", Path: "generated.go", Line: 12, Resolution: "exact"},
		{ObjectID: store.Symbol.Decl.ObjectID, Name: "Store.Add", Path: store.Path, Line: 20, Resolution: "exact"},
	}
	declarations := map[string]atlas.Place{store.Symbol.Decl.ObjectID: store, handler.Symbol.Decl.ObjectID: handler, outer.Symbol.Decl.ObjectID: outer, unrelated.Symbol.Decl.ObjectID: unrelated}
	rows := operationCallerEvidence(store, declarations)
	if len(rows) != 3 {
		t.Fatalf("caller expansion duplicated or dropped a generated/recursive call: %+v", rows)
	}
	for _, row := range rows {
		if row["name"] == "Handler.Add" {
			sites := row["call_sites"].([]map[string]any)
			if len(sites) != 2 || sites[0]["line"] != 14 || sites[1]["line"] != 18 || sites[0]["resolution"] != "alternatives" {
				t.Fatalf("distinct call site or uncertainty lost: %+v", sites)
			}
		}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"object_id", "object-handler", "Client.Add", "Quota.Add", "LargeUnrelatedPayload", "CallerHelpMustStayOnCaller"} {
		if strings.Contains(string(raw), absent) {
			t.Fatalf("provider context leaked %q: %s", absent, raw)
		}
	}
	if store.Symbol.CalledBy[0].ObjectID == "" || handler.Symbol.CalledBy[0].ObjectID == "" {
		t.Fatal("provider projection mutated reusable native identities")
	}
	if len(handler.Symbol.Bindings[0].Evidence) != 1 || !strings.Contains(string(raw), "Action.Run") {
		t.Fatal("caller activation was lost or source metadata mutated")
	}
}

// A negative description-table decision must not hide native registrations
// from the operation review. Neither a registration nor a goroutine alone
// determines its semantic role; the reviewer can still return none.
func TestOperationReviewIncludesMissedRegisteredCallback(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	var id string
	for i := range graph.Places {
		p := &graph.Places[i]
		if p.Symbol != nil && p.Symbol.Decl.Name == "Op01" {
			id = p.ID
			p.Symbol.Bindings = []atlas.SymbolBinding{{From: "Install", To: p.Symbol.Decl.Name, Detail: "company.Action.Execute", Path: p.Path, Line: p.LineNo}}
		}
	}
	if id == "" {
		t.Fatal("test symbol missing")
	}
	provider := &tableProvider{}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	for _, use := range result.Uses {
		if use.Stage == lines.StageOperations {
			if use.Rows != 1 {
				t.Fatalf("native callback not reviewed independently: %+v", use)
			}
			return
		}
	}
	t.Fatal("no operation review")
}

func TestGeneratedContextDoesNotCreateDescriptionOrOperationRequests(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	for i := range graph.Places {
		p := &graph.Places[i]
		if p.Path != "svc/core/c.go" {
			continue
		}
		if p.File != nil {
			p.File.Generated = true
		}
		if p.Symbol != nil {
			p.Symbol.Candidate = false
			p.Symbol.Bindings = []atlas.SymbolBinding{{From: "Install", To: p.Symbol.Decl.Name, Detail: "Action.Run", Path: p.Path, Line: p.LineNo}}
		}
	}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, &tableProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, use := range result.Uses {
		if (use.Stage == lines.StageSymbols || use.Stage == lines.StageOperations) && use.Rows != 0 {
			t.Fatalf("context-only declarations produced model requests: %+v", use)
		}
	}
}

// Metadata on an object a factory creates must not describe the factory's own
// activation. The callable receiving that object keeps the original evidence.
func TestOperationReviewKeepsRegistrationMetadataOnItsRecipient(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	var first, second string
	transfer := atlas.SymbolBinding{From: "Op01", To: "Op02", Detail: "Action.Run", Path: "svc/core/c.go", Line: 15,
		Evidence: []atlas.EdgeEvidence{{Label: "second-action-help"}}}
	for i := range graph.Places {
		p := &graph.Places[i]
		if p.Symbol == nil {
			continue
		}
		switch p.Symbol.Decl.Name {
		case "Op01":
			first = "operation:" + p.ID
			p.Symbol.Bindings = []atlas.SymbolBinding{
				{From: "Install", To: "Op01", Detail: "Action.Run", Path: p.Path, Line: p.LineNo,
					Evidence: []atlas.EdgeEvidence{{Label: "first-action-help"}}}, transfer,
			}
		case "Op02":
			second = "operation:" + p.ID
			p.Symbol.Bindings = []atlas.SymbolBinding{transfer}
		}
	}
	opts := twoTargetOptions(t, graph, &tableProvider{})
	opts.Through = lines.StageOperations
	if _, err := Read(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, KnowledgeFilename))
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Records []Knowledge `json:"records"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatal(err)
	}
	inputs := make(map[string]string)
	for _, record := range artifact.Records {
		inputs[record.PlaceID] = string(record.Input)
	}
	if !strings.Contains(inputs[first], "first-action-help") || strings.Contains(inputs[first], "second-action-help") || !strings.Contains(inputs[first], "Op02") {
		t.Fatalf("factory inherited supplied action metadata or lost its own evidence: %s", inputs[first])
	}
	if !strings.Contains(inputs[second], "second-action-help") || strings.Contains(inputs[second], "first-action-help") {
		t.Fatalf("callback lost or borrowed its registration evidence: %s", inputs[second])
	}
}

// A declaration with an observed route is not reviewed even when the
// selection proposed it and a registration binds it: the route fact is its
// operation, and the group index would drop the model's request on the
// same declaration as a second copy of that route.
func TestNativeRouteHandlersAreNotReviewedWhileOtherHandlersAre(t *testing.T) {
	provider := &mutatedTableProvider{}
	var asked []string
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		if input["table"] != lines.StageOperations {
			return
		}
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			asked = append(asked, row["name"].(string))
			rows[i]["entry"], rows[i]["activation"], rows[i]["name_kind"] = "self", "request", "label"
			rows[i]["name"], rows[i]["description"] = row["name"], "Handles the request."
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	handler := func(name string) atlas.Place {
		return atlas.Place{ID: name, Kind: atlas.PlaceSymbol, Path: "api.py", LineNo: 10, Parent: "file:api", TargetIDs: []string{"service"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:" + name, Name: name},
				Bindings: []atlas.SymbolBinding{{From: "install", To: name, Detail: "app.get", Path: "api.py", Line: 8}}}}
	}
	routed, plain := handler("routed"), handler("plain")
	route := atlas.Place{ID: "bnd:api.py:8:http_server", Kind: atlas.PlaceBoundary, Path: "api.py", LineNo: 8, Parent: "file:api", TargetIDs: []string{"service"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", SubjectID: routed.ID, GivenKind: atlas.BoundaryHTTPServer, Direction: atlas.DirectionIn, Method: "GET", Values: []string{"/items"}}}
	r.opts.Graph.Places = []atlas.Place{routed, plain, route}
	r.places = map[string]atlas.Place{routed.ID: routed, plain.ID: plain, route.ID: route}
	r.operations = map[string][3]string{routed.ID: {"request"}, plain.ID: {"request"}}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	if err := r.readOperations(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, []string{"plain"}) {
		t.Fatalf("reviewed rows = %v, want only the handler without a route", asked)
	}
	if _, proposed := r.operations[routed.ID]; proposed {
		t.Fatalf("route handler kept a hypothesis without its review: %v", r.operations[routed.ID])
	}
	if operation := r.operations[plain.ID]; operation[0] != "request" || operation[1] != "plain" {
		t.Fatalf("handler without a route lost its review: %v", operation)
	}
}
