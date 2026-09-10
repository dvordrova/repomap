package reading

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

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
