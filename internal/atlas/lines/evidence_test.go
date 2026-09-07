package lines

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
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

func TestCallContextKeysStayLocalWithoutChangingProviderInput(t *testing.T) {
	call := atlas.SymbolCall{Name: "Client.Submit", Kind: "calls", Line: 12, Column: 31, CalleeIDs: []string{"sym:private-source-key"}}
	var catalog EvidenceCatalog
	raw, err := json.Marshal(catalog.Call(call))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "callee_ids") || strings.Contains(string(raw), "private-source-key") || strings.Contains(string(raw), "column") || len(call.CalleeIDs) != 1 || call.Column != 31 {
		t.Fatalf("local traversal keys leaked or were mutated: %s %+v", raw, call)
	}
	call.CalleeIDs = nil
	call.Column = 0
	want, _ := json.Marshal(call)
	if string(raw) != string(want) {
		t.Fatalf("adding local identity changed description input: %s != %s", raw, want)
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
	raw, _ = json.Marshal(map[string]any{"rows": []map[string]string{{"key": "r1", "line": explanation, "key_symbol": "yes"}}})
	answer, err := table.Decode(Types(), windows[0], raw)
	if err != nil || len(answer) != 1 || answer[0]["line"] != explanation {
		t.Fatalf("type explanation lost its full qualification or paragraph break: %v, %v", answer, err)
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
