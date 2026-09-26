package lines

import (
	"encoding/json"
	"maps"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// symbolRowCalls decodes the calls a symbol row sends, as the model reads them.
func symbolRowCalls(t *testing.T, row table.Row) (map[string]any, []map[string]any) {
	t.Helper()
	fields := make(map[string]any)
	for _, field := range row.Fields {
		fields[field.Name] = field.Value
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Calls []map[string]any `json:"calls"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return fields, decoded.Calls
}

// Only a complete exact repository callee becomes a local_calls line; a
// possible, unresolved or external call keeps its full evidence and its line.
func TestSymbolRowRetainsUncertainCallsAndTheirLines(t *testing.T) {
	calls := []atlas.SymbolCall{
		{Name: "local", Kind: "calls", Line: 10, Resolution: "exact", CalleeIDs: []string{"private-local"}},
		{Name: "possible", Kind: "calls", Line: 11, Resolution: "alternatives", CalleeIDs: []string{"private-local"}},
		{Name: "unresolved", Kind: "calls", Line: 12, Resolution: "unresolved", CalleeIDs: []string{"private-local"}},
		{Name: "missing resolution", Kind: "calls", Line: 13, CalleeIDs: []string{"private-local"}},
		{Name: "external", Kind: "invokes_external", Line: 14, Resolution: "exact", API: &atlas.CallAPI{Package: "net/http", Name: "Get"}},
		{Name: "declared API", Kind: "calls", Line: 15, Resolution: "exact", CalleeIDs: []string{"private-local"}, API: &atlas.CallAPI{Name: "Send"}},
		{Name: "unknown target", Kind: "calls", Line: 16, Resolution: "exact"},
	}
	fields, rendered := symbolRowCalls(t, SymbolRow(atlas.Place{Symbol: &atlas.SymbolFacts{Calls: calls}}, ""))
	var names []string
	for i, call := range rendered {
		name, _ := call["name"].(string)
		names = append(names, name)
		if !reflect.DeepEqual(call["lines"], []any{float64(12 + i - 1)}) {
			t.Fatalf("call %s lost its line: %+v", call["name"], call)
		}
	}
	if !reflect.DeepEqual(names, []string{"possible", "unresolved", "missing resolution", "external", "declared API", "unknown target"}) {
		t.Fatalf("lost uncertainty or reordered calls: %v", names)
	}
	if !reflect.DeepEqual(fields["local_calls"], []string{"local@10"}) {
		t.Fatalf("local delegation is not one context line: %+v", fields["local_calls"])
	}
	for _, gone := range []string{"call_count", "call_options"} {
		if _, rendered := fields[gone]; rendered {
			t.Fatalf("the row still renders %s, which no column reads", gone)
		}
	}
}

// Identical calls are written once with the list of lines they occur on:
// the canvas.spec.mjs selection row (550 calls, 71 KB of calls) was refused
// for its size on every run, and 245 entries of 31 KB carry the same facts.
// Expanding each entry back over its lines gives every call the row used to
// carry, with its own evidence, and nothing else.
func TestSymbolRowWritesIdenticalCallsOnceWithEveryLine(t *testing.T) {
	loop := atlas.EdgeEvidence{Extractor: "control_context", Label: "for body", Path: "spec.mjs", LineNo: 30}
	click := func(line, column int, values ...string) atlas.SymbolCall {
		return atlas.SymbolCall{Name: "page.click", Kind: "invokes_external", Line: line, Column: column, Values: values,
			API: &atlas.CallAPI{Package: "playwright", Receiver: "Page", Name: "click"}, SourceArguments: []atlas.SourceArgument{{Position: 1}}}
	}
	looped := click(31, 5, "#next")
	looped.Evidence = []atlas.EdgeEvidence{loop}
	possible := atlas.SymbolCall{Name: "Store.Save", Kind: "calls", Line: 40, Column: 3, Dispatch: "interface", Resolution: "alternatives", CalleeIDs: []string{"private-a", "private-b"}}
	calls := []atlas.SymbolCall{
		click(10, 5, "#next"), click(12, 5, "#next"), click(12, 30, "#next"), click(11, 5, "#back"),
		{Name: "helper", Kind: "calls", Line: 13, Resolution: "exact", CalleeIDs: []string{"private-helper"}},
		looped, click(20, 5, "#next"), possible, possible,
	}
	calls[len(calls)-1].Line = 44
	place := atlas.Place{ID: "s1", Path: "spec.mjs", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "visual/canvas.spec", Kind: "function"}, Calls: calls}}
	before, _ := json.Marshal(place)
	row := SymbolRow(place, "")
	fields, rendered := symbolRowCalls(t, row)
	lines := make([][]any, len(rendered))
	for i, call := range rendered {
		lines[i], _ = call["lines"].([]any)
		if _, single := call["line"]; single {
			t.Fatalf("an entry kept a single line beside its lines: %+v", call)
		}
		if _, ref := call["ref"]; ref {
			t.Fatalf("an entry kept a ref no column selects: %+v", call)
		}
	}
	want := [][]any{{10.0, 12.0, 12.0, 20.0}, {11.0}, {31.0}, {40.0, 44.0}}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("identical calls were not written once in call order, or different ones were merged: %v", lines)
	}
	// Expand every entry back over its lines and resolve its evidence refs:
	// the result is each original call as a single rendering shows it.
	byRef := fields["source_evidence"]
	raw, _ := json.Marshal(byRef)
	var catalogue struct {
		ByRef map[string]atlas.EdgeEvidence `json:"by_ref"`
	}
	if err := json.Unmarshal(raw, &catalogue); err != nil {
		t.Fatal(err)
	}
	type shown struct {
		atlas.SymbolCall
		HasRepositoryCalleeCandidate bool `json:"has_repository_callee_candidate,omitempty"`
	}
	var expanded []shown
	for _, call := range rendered {
		for _, line := range call["lines"].([]any) {
			single := maps.Clone(call)
			delete(single, "lines")
			single["line"] = line
			raw, _ := json.Marshal(single)
			var restored struct {
				shown
				EvidenceRefs []string `json:"evidence_refs"`
			}
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			for _, ref := range restored.EvidenceRefs {
				restored.Evidence = append(restored.Evidence, catalogue.ByRef[ref])
			}
			expanded = append(expanded, restored.shown)
		}
	}
	var originals []shown
	for _, call := range calls {
		if call.Name == "helper" {
			continue
		}
		original := shown{SymbolCall: call, HasRepositoryCalleeCandidate: len(call.CalleeIDs) > 0}
		original.Column, original.CalleeIDs, original.SourceArguments = 0, nil, nil
		originals = append(originals, original)
	}
	order := func(calls []shown) {
		sort.SliceStable(calls, func(i, j int) bool { return calls[i].Line < calls[j].Line })
	}
	order(expanded)
	order(originals)
	if !reflect.DeepEqual(expanded, originals) {
		t.Fatalf("writing identical calls once lost or changed a call:\n%+v\nwant\n%+v", expanded, originals)
	}
	if after, _ := json.Marshal(place); string(before) != string(after) {
		t.Fatal("rendering changed the native calls")
	}
	// Both prompts that receive these rows say what an entry's lines are,
	// including a line listed twice: the evidence vocabulary contract test
	// checks enumerated values, not field names.
	for _, def := range []table.Definition{SymbolSelection(false), Symbols()} {
		if !strings.Contains(def.System, "`lines` of a call") || !strings.Contains(def.System, "line listed twice") {
			t.Fatalf("%s receives call entries with lines, and its prompt does not define them", def.Contract)
		}
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
	for _, want := range []string{`"local_calls"`, `"processPendingJobs@13 goroutine (for body without condition; select without default)"`, `"lines":[14]`, `"lines":[15]`,
		`"dispatch":"interface"`, `"resolution":"alternatives"`, `"source_evidence"`, `"for body without condition"`, `"has_repository_callee_candidate":true`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("symbol row lost %s: %s", want, raw)
		}
	}
	for _, forbidden := range []string{`"invocation":"synchronous"`, `"resolution":"exact"`, `"lines":[13]`, "private-", "call_count"} {
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
