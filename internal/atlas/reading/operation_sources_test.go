package reading

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// The preset exercises independent decisions, not model judgement. The actual
// Go cumulative fixture verifies the native request/response/continuation facts;
// this regression checks that those distinct facts reach the operation cube.
func TestOperationRequestPreservesReceiverArgumentsAndContinuation(t *testing.T) {
	const path = "internal/storefixture/http_registrations.go"
	anchor := func(line, column int) *sourcevalue.Anchor {
		return &sourcevalue.Anchor{Path: path, Line: line, Column: column}
	}
	parameter := func(name string, position, owner int) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "parameter", Text: name, Position: position, Owner: anchor(owner, 1)}
	}
	call := func(name string, line, column int, receiver *sourcevalue.Value) atlas.SymbolCall {
		api := &atlas.CallAPI{Package: "net/http", Receiver: name[:strings.LastIndexByte(name, '.')], Name: name[strings.LastIndexByte(name, '.')+1:]}
		return atlas.SymbolCall{Name: "http." + name, Kind: "invokes_external", Line: line, Column: column,
			API: api, ReceiverValue: receiver, Invocation: "synchronous", Resolution: "exact",
			Evidence: []atlas.EdgeEvidence{{Extractor: "compiler", Path: path, LineNo: line, Label: "original invocation"}}}
	}
	requestSet := call("Header.Set", 127, 15, &sourcevalue.Value{Kind: "field", Text: "Header", Parts: []sourcevalue.Value{*parameter("r", 2, 126)}})
	requestSet.SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "X-Scope-OrgID"}}, {Position: 2, Origin: &sourcevalue.Value{Kind: "literal", Text: "fixture-request"}}}
	continuation := call("Handler.ServeHTTP", 128, 8, parameter("next", 1, 125))
	continuation.SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: parameter("w", 1, 126)}, {Position: 2, Origin: parameter("r", 2, 126)}}
	continuation.Resolution = "unresolved"
	continuation.DispatchObservations = []atlas.DispatchObservation{{Kind: "invokes_external", Resolution: "exact", Invocation: "synchronous"}, {Kind: "invokes_unresolved", Resolution: "unresolved", Invocation: "synchronous"}}
	responseHeader := call("ResponseWriter.Header", 133, 4, parameter("w", 1, 132))
	responseSet := call("Header.Set", 133, 13, &sourcevalue.Value{Kind: "call_result", Anchor: anchor(133, 4)})
	responseSet.SourceArguments = []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "X-Scope-OrgID"}}, {Position: 2, Origin: &sourcevalue.Value{Kind: "literal", Text: "fixture-response"}}}
	byName := map[string][]atlas.SymbolCall{"withRequestScope$1": {requestSet, continuation}, "writeScopeResponse": {responseHeader, responseSet}}
	provider := &mutatedTableProvider{}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Graph.Places = nil
	r.places, r.operations = map[string]atlas.Place{}, map[string][3]string{}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	for _, name := range []string{"withRequestScope$1", "writeScopeResponse"} {
		p := atlas.Place{ID: "private-" + name, Kind: atlas.PlaceSymbol, Path: path, LineNo: 126, Parent: "file:" + path,
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, ObjectID: "private-object-" + name, Signature: "func(w http.ResponseWriter, r *http.Request)"}, Calls: byName[name]}}
		r.opts.Graph.Places = append(r.opts.Graph.Places, p)
		r.places[p.ID], r.operations[p.ID] = p, [3]string{"request", "earlier proposal", ""}
	}
	before, _ := json.Marshal(r.opts.Graph)
	inspected := 0
	provider.mutate = func(input map[string]any, answers []map[string]any) {
		for i, raw := range input["rows"].([]any) {
			row := raw.(map[string]any)
			name := row["name"].(string)
			encoded, _ := json.Marshal(row["calls"])
			var calls []struct {
				atlas.SymbolCall
				EvidenceRefs []string `json:"evidence_refs"`
			}
			if err := json.Unmarshal(encoded, &calls); err != nil {
				t.Fatal(err)
			}
			if len(calls) != len(byName[name]) {
				t.Fatalf("own original calls lost/expanded: %s", encoded)
			}
			for j, got := range calls {
				want := byName[name][j]
				want.Evidence = nil
				if !reflect.DeepEqual(got.SymbolCall, want) || len(got.EvidenceRefs) != 1 {
					t.Fatalf("%s call %d lost its receiver, values, exact site or dispatch uncertainty:\ngot %+v\nwant %+v", name, j, got, want)
				}
			}
			encoded, _ = json.Marshal(row)
			if row["source_evidence"] == nil || strings.Contains(string(encoded), "private-") {
				t.Fatalf("original source context absent or native identity exposed: %s", encoded)
			}
			answers[i]["entry"] = "none"
			if name == "writeScopeResponse" {
				answers[i]["entry"], answers[i]["activation"], answers[i]["name_kind"] = "self", "request", "label"
				answers[i]["name"], answers[i]["description"] = "Return scope response", "Sets the response header and returns no content."
			}
			inspected++
		}
	}
	if err := r.readOperations(t.Context()); err != nil {
		t.Fatal(err)
	}
	if inspected != 2 || len(r.rejected) != 0 || len(r.operations) != 1 || r.operations["private-writeScopeResponse"][0] != "request" {
		t.Fatalf("independent operation decisions changed: inspected=%d rejected=%+v operations=%+v", inspected, r.rejected, r.operations)
	}
	after, _ := json.Marshal(r.opts.Graph)
	if string(before) != string(after) {
		t.Fatal("operation evidence preparation mutated the shared graph")
	}
}
