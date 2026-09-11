package lines

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Interface field alternatives can cite the same assignments at many calls.
// A description row must fit without dropping candidate implementations or
// confusing the evidence belonging to one call with that of another.
func TestSharedEvidenceKeepsEveryCallAndBindingAssociation(t *testing.T) {
	var observations []atlas.EdgeEvidence
	for i := 0; i < 18; i++ {
		observations = append(observations, atlas.EdgeEvidence{
			Extractor: "interface_field_assignment", Path: "tree/interval.go", LineNo: 100 + i,
			Label: fmt.Sprintf("candidate %d: %s", i, strings.Repeat("qualified.interface.method ", 6)),
		})
	}
	place := atlas.Place{ID: "symbol:local-only", Path: "tree/interval.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Node.Update", Kind: "method", ObjectID: "native-id-not-for-provider"}}}
	for i := 0; i < 12; i++ {
		// Distinct associations, including order and a repeated observation.
		evidence := append([]atlas.EdgeEvidence(nil), observations...)
		evidence = append(evidence, observations[i])
		place.Symbol.Calls = append(place.Symbol.Calls, atlas.SymbolCall{Name: "Comparable.Compare", Line: 200 + i, Resolution: "alternatives", Evidence: evidence})
		place.Symbol.Bindings = append(place.Symbol.Bindings, atlas.SymbolBinding{From: "Node.Update", To: "Value.Compare", Line: 200 + i, Resolution: "alternatives", Evidence: evidence})
	}
	before, _ := json.Marshal(place)
	if len(before) <= table.DefaultInputBytes {
		t.Fatal("fixture does not reproduce repeated evidence overflow")
	}
	row := SymbolRow(place, "Updates an interval tree.")
	windows, err := table.Windows(Symbols(), 1, []table.Row{row})
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 || strings.Contains(string(windows[0].Request), place.Symbol.Decl.ObjectID) {
		t.Fatal("provider preparation lost the row or exposed a native identity")
	}
	var catalog EvidenceCatalog
	for _, call := range place.Symbol.Calls {
		raw, _ := json.Marshal(catalog.Call(call))
		var projected struct {
			atlas.SymbolCall
			EvidenceRefs []string `json:"evidence_refs"`
		}
		if err := json.Unmarshal(raw, &projected); err != nil {
			t.Fatal(err)
		}
		for _, ref := range projected.EvidenceRefs {
			projected.Evidence = append(projected.Evidence, catalog.byRef[ref])
		}
		if !reflect.DeepEqual(projected.SymbolCall, call) {
			t.Fatal("call evidence or uncertainty changed")
		}
	}
	bindings := catalog.Bindings(place.Symbol.Bindings).(BindingTable)
	for i, row := range bindings.Rows {
		projected := make(map[string]any)
		for key, value := range bindings.Shared {
			projected[key] = value
		}
		for j, value := range row {
			projected[bindings.Columns[j]] = value
		}
		raw, _ := json.Marshal(projected)
		var binding struct {
			atlas.SymbolBinding
			EvidenceRefs []string `json:"evidence_refs"`
		}
		if err := json.Unmarshal(raw, &binding); err != nil {
			t.Fatal(err)
		}
		for _, ref := range binding.EvidenceRefs {
			binding.Evidence = append(binding.Evidence, catalog.byRef[ref])
		}
		if !reflect.DeepEqual(binding.SymbolBinding, place.Symbol.Bindings[i]) {
			t.Fatal("binding evidence or uncertainty changed")
		}
	}
	if len(catalog.byRef) != len(observations) {
		t.Fatal("source observations were not shared across calls and bindings")
	}
	after, _ := json.Marshal(place)
	if string(before) != string(after) {
		t.Fatal("provider preparation changed reusable native facts")
	}
	first, _ := json.Marshal(row)
	second, _ := json.Marshal(SymbolRow(place, "Updates an interval tree."))
	if string(first) != string(second) {
		t.Fatal("local refs are not deterministic")
	}
}

func TestCallContextExposesRepositoryOriginWithoutLeakingNativeKeys(t *testing.T) {
	for _, resolution := range []string{"exact", "alternatives", "unresolved"} {
		t.Run(resolution, func(t *testing.T) {
			call := atlas.SymbolCall{Name: "Client.Submit", Kind: "calls", Line: 12, Column: 31,
				Resolution: resolution, CalleeIDs: []string{"sym:private-source-key"}, API: &atlas.CallAPI{Package: "example.com/vendor-client", Receiver: "Client", Name: "Submit"}}
			var catalog EvidenceCatalog
			raw, err := json.Marshal(catalog.Call(call))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "callee_ids") || strings.Contains(string(raw), "private-source-key") || strings.Contains(string(raw), "column") || len(call.CalleeIDs) != 1 || call.Column != 31 {
				t.Fatalf("local traversal keys leaked or were mutated: %s %+v", raw, call)
			}
			var projected map[string]any
			if json.Unmarshal(raw, &projected) != nil || projected["has_repository_callee_candidate"] != true || projected["resolution"] != resolution {
				t.Fatalf("origin observation lost or dispatch certainty changed: %s", raw)
			}
			if !strings.Contains(string(raw), `"package":"example.com/vendor-client"`) || !strings.Contains(string(raw), `"receiver":"Client"`) {
				t.Fatalf("selection lost its native API identity: %s", raw)
			}
			call.CalleeIDs = []string{"sym:renamed-key", "sym:second-possible-key"}
			changedIDs, _ := json.Marshal(catalog.Call(call))
			if string(raw) != string(changedIDs) {
				t.Fatal("native ID/count changes altered the existential origin observation")
			}
			call.CalleeIDs = nil
			withoutOrigin, _ := json.Marshal(catalog.Call(call))
			if strings.Contains(string(withoutOrigin), "has_repository_callee_candidate") {
				t.Fatal("missing native callee evidence became a claim about origin")
			}
		})
	}
}

// A request carries each origin as kind and text with parts to OriginDepth;
// anchors, owners, positions and deeper parts stay in the graph for the
// destination traversal. Morfeu's orientation request spent 90 KB of 280 KB
// on origin trees up to nine levels deep, and the answer used five nodes.
func TestCallOriginsAreCompactInRequestsAndCompleteInTheGraph(t *testing.T) {
	anchor := func(line, column int) *sourcevalue.Anchor {
		return &sourcevalue.Anchor{Path: "client.go", Line: line, Column: column}
	}
	level3 := sourcevalue.Value{Kind: "unknown", Text: "level3-expression", Anchor: anchor(9, 6)}
	level2 := sourcevalue.Value{Kind: "field", Text: "level2", Anchor: anchor(9, 5), Parts: []sourcevalue.Value{level3}}
	level1 := sourcevalue.Value{Kind: "field", Text: "level1", Anchor: anchor(9, 4), Owner: anchor(1, 1), Parts: []sourcevalue.Value{level2},
		Initializer: &sourcevalue.Value{Kind: "literal", Text: "init-value", Anchor: anchor(2, 2)}}
	root := &sourcevalue.Value{Kind: "concat", Anchor: anchor(9, 3), Parts: []sourcevalue.Value{{Kind: "literal", Text: "https://", Anchor: anchor(9, 3)}, level1}}
	call := atlas.SymbolCall{Kind: "invokes_external", Name: "http.Get", Line: 9, Column: 2, Resolution: "exact",
		API:             &atlas.CallAPI{Package: "net/http", Name: "Get"},
		ReceiverValue:   &sourcevalue.Value{Kind: "parameter", Text: "client", Position: 1, Owner: anchor(1, 1)},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: root}},
		ResultValue:     &sourcevalue.Value{Kind: "call_result", Anchor: anchor(9, 2)}}
	before, _ := json.Marshal(call)
	var catalog EvidenceCatalog
	compact, err := json.Marshal(catalog.CallWithOrigins(call))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"receiver_value":{"kind":"parameter","text":"client"}`, `"result_value":{"kind":"call_result"}`, `"kind":"concat"`, `"text":"https://"`,
		`"kind":"field","text":"level1"`, `"initializer":{"kind":"literal","text":"init-value"}`, `"parts":[{"kind":"field","text":"level2"}]`, `"column":2`, `"position":1,"origin"`} {
		if !strings.Contains(string(compact), want) {
			t.Fatalf("request origin lost %s: %s", want, compact)
		}
	}
	for _, forbidden := range []string{`"anchor"`, `"owner"`, "level3-expression", `"position":1,"anchor"`} {
		if strings.Contains(string(compact), forbidden) {
			t.Fatalf("request origin carries %s: %s", forbidden, compact)
		}
	}
	if len(compact) >= len(before)/2 {
		t.Fatalf("request form is not compact: %d bytes of %d", len(compact), len(before))
	}
	place := atlas.Place{ID: "sym:send", Kind: atlas.PlaceSymbol, Path: "client.go", LineNo: 1, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "obj:send", Name: "send"}, Calls: []atlas.SymbolCall{call}}}
	question, err := json.Marshal(CallableEvidence(atlas.Graph{Places: []atlas.Place{place}}, map[string]bool{"obj:send": true})["obj:send"])
	if err != nil || strings.Contains(string(question), `"anchor"`) || !strings.Contains(string(question), `"text":"level2"`) {
		t.Fatalf("declaration evidence for questions and orientation is not the compact form: %s %v", question, err)
	}
	if after, _ := json.Marshal(call); string(before) != string(after) {
		t.Fatal("request preparation changed the graph's complete origins")
	}
}

func TestTypeContextKeepsOwnedDeclarationsWithoutNativeIDs(t *testing.T) {
	place := atlas.Place{ID: "type-local", Path: "queue/type.go", Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{Name: "Ticket", Kind: "type", ObjectID: "native-type-id", Doc: "Tracks a pending job."},
		Members: []atlas.TypeMember{
			{Path: "queue/renew.go", Decl: atlas.Decl{Name: "Ticket.Renew", Kind: "method", ObjectID: "native-method-id", LineNo: 9, Signature: "func() error", Doc: "Renew extends the ticket's validity."}},
			{Path: "queue/type.go", Decl: atlas.Decl{Name: "Ticket.Done", Kind: "method", LineNo: 24}},
			{Path: "queue/type.go", Decl: atlas.Decl{Name: "items", Kind: "variable", LineNo: 3, Signature: "items: list[Point]", ObjectID: "native-field-id"}},
		},
	}}
	before, _ := json.Marshal(place)
	windows, err := table.Windows(Types(), 1, []table.Row{TypeRow(place)})
	if err != nil || len(windows) != 1 {
		t.Fatalf("type context: %v", err)
	}
	request := string(windows[0].Request)
	for _, want := range []string{"Tracks a pending job.", "queue/renew.go", "Renew extends the ticket's validity.", "func() error", "Ticket.Done", "items: list[Point]"} {
		if !strings.Contains(request, want) {
			t.Fatalf("lost owned context %q: %s", want, request)
		}
	}
	for _, forbidden := range []string{"native-type-id", "native-method-id", "native-field-id", "type-local", "activation", "outbound", "file_hypothesis"} {
		if strings.Contains(request, forbidden) {
			t.Fatalf("type table exposed unrelated field %q", forbidden)
		}
	}
	file := atlas.Place{ID: "file-local", Kind: atlas.PlaceFile, Path: place.Path, File: &atlas.FileFacts{Decls: []atlas.Decl{place.Symbol.Decl}}}
	chunks := QuestionRows(atlas.Graph{Places: []atlas.Place{file, place}})
	if len(chunks) != 1 {
		t.Fatalf("type question chunks: %d", len(chunks))
	}
	evidence := AnchorEvidence(chunks[0], "a1")["evidence"].([]map[string]any)[0]
	members, ok := evidence["owned_declarations"].([]map[string]any)
	if !ok || len(members) != len(place.Symbol.Members) || members[2]["signature"] != "items: list[Point]" || members[2]["line"] != 3 {
		t.Fatalf("selecting a type lost its original owned declarations: %+v", evidence)
	}
	raw, err := json.Marshal(evidence)
	if err != nil || strings.Contains(string(raw), "native-") {
		t.Fatalf("type question evidence leaked identity: %s (%v)", raw, err)
	}
	after, _ := json.Marshal(place)
	if string(before) != string(after) {
		t.Fatal("type preparation mutated the native context")
	}
	explanation := "A ticket represents a pending job and keeps the points associated with that job together. Its Renew operation extends the ticket's validity; the declaration also exposes a Done operation.\n\nThese names describe the interface, without establishing what happens to the points when the job ends."
	raw, _ = json.Marshal(map[string]any{"rows": []map[string]string{{"key": "r1", "line": explanation, "alias": "none", "key_symbol": "yes"}}})
	answer, err := table.Decode(Types(), windows[0], raw)
	if err != nil || len(answer) != 1 || answer[0]["line"] != explanation {
		t.Fatalf("type explanation lost its full qualification or paragraph break: %v, %v", answer, err)
	}
}

func TestOperationPromptNamesInteractionsInEnglishAndPreservesCommandSyntax(t *testing.T) {
	def := Operations()
	if def.Contract != "repomap.atlas.operations.v20" {
		t.Fatalf("unexpected operation contract: %s", def.Contract)
	}
	for _, instruction := range []string{"short English name", "rather than copying an unexplained", "Do not translate observed command/path syntax"} {
		if !strings.Contains(def.System, instruction) {
			t.Fatalf("ordinary operation prompt omitted %q", instruction)
		}
	}
}

func TestIndependentTablesPackCompleteRowsWithoutCountCaps(t *testing.T) {
	var rows []table.Row
	for i := 0; i < 41; i++ {
		place := atlas.Place{ID: fmt.Sprintf("type-%d", i), Path: fmt.Sprintf("queue/type%d.go", i), Symbol: &atlas.SymbolFacts{
			Decl: atlas.Decl{Name: fmt.Sprintf("Ticket%d", i), Kind: "type", Doc: "Tracks a pending job."},
			Members: []atlas.TypeMember{{Path: "queue/renew.go", Decl: atlas.Decl{
				Name: "Renew", Kind: "method", LineNo: 9, Signature: "func() error", Doc: "Renew extends the ticket's validity.",
			}}},
		}}
		rows = append(rows, TypeRow(place))
	}
	for _, def := range []table.Definition{Directories(), Files(), Symbols(), Types(), Boundaries(), Operations()} {
		t.Run(def.Contract, func(t *testing.T) {
			windows, err := table.Windows(def, 0, rows)
			if err != nil || len(windows) != 1 || !reflect.DeepEqual(windows[0].Rows, rows) {
				t.Fatalf("complete evidence that fits one request was split or changed: %d windows, %v", len(windows), err)
			}
			if len(def.System)+len(windows[0].Request) > table.DefaultInputBytes {
				t.Fatal("independent table exceeded the unchanged default byte budget")
			}
			def.Window = 8
			limited, err := table.Windows(def, 0, rows)
			if err != nil || len(limited) != 6 || len(limited[0].Rows) != 8 || len(limited[5].Rows) != 1 {
				t.Fatalf("explicit row budget was ignored: %d windows, %v", len(limited), err)
			}
		})
	}
}
