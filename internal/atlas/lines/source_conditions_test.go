package lines

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

func TestOutgoingOptionsRetainUncertainCallsAndOriginalPositions(t *testing.T) {
	calls := []atlas.SymbolCall{
		{Name: "local", Kind: "calls", Line: 10, Resolution: "exact", CalleeIDs: []string{"private-local"}},
		{Name: "possible", Kind: "calls", Line: 11, Resolution: "alternatives", CalleeIDs: []string{"private-local"}},
		{Name: "unresolved", Kind: "calls", Line: 12, Resolution: "unresolved", CalleeIDs: []string{"private-local"}},
		{Name: "missing resolution", Kind: "calls", Line: 13, CalleeIDs: []string{"private-local"}},
		{Name: "external", Kind: "invokes_external", Line: 14, Resolution: "exact", API: &atlas.CallAPI{Package: "net/http", Name: "Get"}},
		{Name: "declared API", Kind: "calls", Line: 15, Resolution: "exact", CalleeIDs: []string{"private-local"}, API: &atlas.CallAPI{Name: "Send"}},
		{Name: "unknown target", Kind: "calls", Line: 16, Resolution: "exact"},
	}
	row := SymbolRow(atlas.Place{Symbol: &atlas.SymbolFacts{Calls: calls}}, "")
	row.ID = "s1"
	fields := make(map[string]any)
	for _, field := range row.Fields {
		fields[field.Name] = field.Value
	}
	if !reflect.DeepEqual(fields["call_options"], []string{"c2", "c3", "c4", "c5", "c6", "c7"}) {
		t.Fatalf("lost uncertainty or renumbered calls: %+v", fields["call_options"])
	}
	if rendered := fields["calls"].([]map[string]any); len(rendered) != 6 || rendered[0]["ref"] != "c2" {
		t.Fatalf("selectable calls lost their original refs: %+v", rendered)
	}
	if !reflect.DeepEqual(fields["local_calls"], []string{"local@10"}) {
		t.Fatalf("local delegation is not one context line: %+v", fields["local_calls"])
	}
	if _, counted := fields["call_count"]; counted {
		t.Fatal("a count the options already bound is still rendered")
	}
}

// A rendered call leaves out the default invocation and resolution, which the
// prompt defines; a local callee is one line with what the call says about
// this declaration: its site, a non-default invocation and its control
// statements. Morfeu's symbol windows spent 14-18% of their bytes on the two
// defaults and 18% on full context objects for local delegation.
func TestSymbolRowOmitsDefaultsAndKeepsLocalCallsBrief(t *testing.T) {
	loop := atlas.EdgeEvidence{Extractor: "control_context", Label: "for body without condition", Path: "worker.go", LineNo: 11}
	choice := atlas.EdgeEvidence{Extractor: "control_context", Label: "select without default", Path: "worker.go", LineNo: 12}
	place := atlas.Place{ID: "sym:run", Path: "worker.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "run", Kind: "function"}, Calls: []atlas.SymbolCall{
		{Name: "processPendingJobs", Kind: "calls", Line: 13, Column: 4, Invocation: "goroutine", Resolution: "exact", CalleeIDs: []string{"private-callee"}, Evidence: []atlas.EdgeEvidence{loop, choice}},
		{Name: "time.Sleep", Kind: "invokes_external", Line: 14, Column: 4, Resolution: "exact", API: &atlas.CallAPI{Package: "time", Name: "Sleep"}, Evidence: []atlas.EdgeEvidence{loop}},
		{Name: "Store.Save", Kind: "calls", Line: 15, Column: 4, Dispatch: "interface", Resolution: "alternatives", CalleeIDs: []string{"private-a", "private-b"}},
	}}}
	raw, err := json.Marshal(SymbolRow(place, "").Fields)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"local_calls"`, `"processPendingJobs@13 goroutine (for body without condition; select without default)"`, `"ref":"c2"`, `"ref":"c3"`,
		`"dispatch":"interface"`, `"resolution":"alternatives"`, `"source_evidence"`, `"for body without condition"`, `"has_repository_callee_candidate":true`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("symbol row lost %s: %s", want, raw)
		}
	}
	for _, forbidden := range []string{`"invocation":"synchronous"`, `"resolution":"exact"`, `"ref":"c1"`, "private-", "call_count"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("symbol row carries %s: %s", forbidden, raw)
		}
	}
	// The local line names its control statements itself; only the selectable
	// call's loop observation enters the shared catalogue.
	if strings.Count(string(raw), "control_context") != 1 {
		t.Fatalf("local delegation registered its evidence in the catalogue: %s", raw)
	}
}

func TestSingletonBindingIsOneCompleteObject(t *testing.T) {
	binding := atlas.SymbolBinding{From: "Routes", To: "Proxy", Path: "server/routes.go", Line: 19, Kind: "passes_callback", Resolution: "alternatives",
		Arguments: []atlas.RegistrationArgument{{Kind: "literal_string", Value: "/proxy", Path: "server/routes.go", Line: 19}}, Evidence: []atlas.EdgeEvidence{{Path: "server/routes.go", LineNo: 19, Extractor: "binding"}}}
	before, _ := json.Marshal(binding)
	var catalogue EvidenceCatalog
	projected := catalogue.Bindings([]atlas.SymbolBinding{binding})
	raw, err := json.Marshal(projected)
	if err != nil || strings.Contains(string(raw), `"rows"`) || strings.Contains(string(raw), `"columns"`) {
		t.Fatalf("singleton retained tabular shell: %s %v", raw, err)
	}
	var restored struct {
		atlas.SymbolBinding
		EvidenceRefs []string `json:"evidence_refs"`
	}
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for _, ref := range restored.EvidenceRefs {
		restored.Evidence = append(restored.Evidence, catalogue.byRef[ref])
	}
	after, _ := json.Marshal(binding)
	if !reflect.DeepEqual(restored.SymbolBinding, binding) || string(before) != string(after) {
		t.Fatal("singleton lost evidence or changed source")
	}
}
