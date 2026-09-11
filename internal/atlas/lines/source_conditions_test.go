package lines

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestEmptyCallCatalogueDoesNotAskOutboundOrRemoveKeyAndCandidate(t *testing.T) {
	def := SymbolSelection(false)
	rows := []table.Row{{Fields: []table.Field{{Name: "call_options", Value: []string{}}}}, {Fields: []table.Field{{Name: "call_options", Value: []string{"c1"}}}}}
	window := table.Window{Rows: rows}
	raw, err := table.Request(def, window)
	if err != nil || !strings.Contains(string(raw), `"when_options_nonempty":"call_options"`) || strings.Contains(string(raw), "limit_from") {
		t.Fatalf("missing input condition, or a count the options already bound: %s %v", raw, err)
	}
	// A dropped candidate cell reads no; a written yes is the proposal.
	result, err := table.DecodeResult(def, window, []byte(`{"rows":[{"key":"r1","key_symbol":"yes","outbound":42},{"key":"r2","key_symbol":"no","operation_candidate":"yes","outbound":"c1"}]}`))
	if err != nil || len(result.Rejections) != 0 || result.Answers[0]["key_symbol"] != "yes" || result.Answers[0]["operation_candidate"] != "no" || len(result.Answers[0]) != 2 ||
		result.Answers[1]["outbound"] != "c1" || result.Answers[1]["operation_candidate"] != "yes" {
		t.Fatalf("input condition changed supported decisions: %+v %v", result, err)
	}
	missing, err := table.DecodeResult(def, window, []byte(`{"rows":[{"key":"r1","key_symbol":"yes"},{"key":"r2","key_symbol":"no","operation_candidate":"no"}]}`))
	if err != nil || missing.Answers[0] == nil || missing.Answers[1] != nil {
		t.Fatalf("empty calls requested outbound or real calls stopped requiring it: %+v %v", missing, err)
	}
	// A written value outside yes/no is still a refused cell, not a default.
	written, err := table.DecodeResult(def, window, []byte(`{"rows":[{"key":"r1","key_symbol":"yes","operation_candidate":"unassessed"},{"key":"r2","key_symbol":"no","operation_candidate":"no","outbound":"none"}]}`))
	if err != nil || written.Answers[0] != nil || written.Answers[1] == nil || len(written.Rejections) != 1 {
		t.Fatalf("a value outside yes/no was accepted or refused its neighbour: %+v %v", written, err)
	}
}

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
	window := table.Window{Rows: []table.Row{row}}
	result, err := table.DecodeResult(SymbolSelection(false), window, []byte(`{"rows":[{"key":"r1","key_symbol":"yes","operation_candidate":"no","outbound":"c1 c5 c2"}]}`))
	if err != nil || result.Answers[0]["outbound"] != "c5 c2" {
		t.Fatalf("unsupported local choice displaced accepted choices: %+v %v", result, err)
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
		{Name: "time.Sleep", Kind: "invokes_external", Line: 14, Column: 4, Invocation: "synchronous", Resolution: "exact", API: &atlas.CallAPI{Package: "time", Name: "Sleep"}, Evidence: []atlas.EdgeEvidence{loop}},
		{Name: "Store.Save", Kind: "calls", Line: 15, Column: 4, Invocation: "interface_invoke:synchronous", Resolution: "alternatives", CalleeIDs: []string{"private-a", "private-b"}},
	}}}
	raw, err := json.Marshal(SymbolRow(place, "").Fields)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"local_calls"`, `"processPendingJobs@13 goroutine (for body without condition; select without default)"`, `"ref":"c2"`, `"ref":"c3"`,
		`"invocation":"interface_invoke:synchronous"`, `"resolution":"alternatives"`, `"source_evidence"`, `"for body without condition"`, `"has_repository_callee_candidate":true`} {
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
	binding := atlas.SymbolBinding{From: "Routes", To: "Proxy", Path: "server/routes.go", Line: 19, Invocation: "callback_transfer", Resolution: "alternatives",
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
