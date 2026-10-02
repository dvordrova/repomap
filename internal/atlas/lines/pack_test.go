package lines

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// routerPlace is casdoor's InitAPI in small: one registration call per
// route, the same outside API each time with its own route words, every
// handler handed over at its own line, and calls that stay single.
func routerPlace(routes int) atlas.Place {
	api := &atlas.CallAPI{Package: "github.com/beego/beego/v2/server/web", Name: "Router", Signature: "func(rootpath string, c web.ControllerInterface, mappingMethods ...string) *web.HttpServer"}
	place := atlas.Place{ID: "symbol:init-api", Path: "routers/router.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "InitAPI", Kind: "function", Signature: "func()"}}}
	witness := []atlas.EdgeEvidence{{Extractor: "callback_registration", Path: "routers/router.go", LineNo: 40, Label: "receiving call: web.Router"}}
	symbol := place.Symbol
	symbol.Calls = append(symbol.Calls, atlas.SymbolCall{Kind: "invokes_external", Name: "web.NSNamespace", Line: 34, Values: []string{"/api"},
		API: &atlas.CallAPI{Package: api.Package, Name: "NSNamespace"}})
	for i := range routes {
		line := 100 + i
		symbol.Calls = append(symbol.Calls, atlas.SymbolCall{Kind: "invokes_external", Name: "web.Router", Line: line, API: api,
			Values: []string{fmt.Sprintf("/api/get-thing-%d", i), fmt.Sprintf("GET:GetThing%d", i)}})
		// Bindings arrive in their native order, not by line.
		symbol.Bindings = append(symbol.Bindings, atlas.SymbolBinding{From: "InitAPI", To: "ApiController.Finish", Kind: "binds_implementation",
			Detail: "parameter 2 -> web.Router; interface web.ControllerInterface method Finish func()", Path: "routers/router.go", Line: 100 + (i*7)%routes, Resolution: "exact"})
	}
	// The same call twice on one line, and a call of the same callee with
	// its own evidence and dispatch: nothing that tells them apart is lost.
	symbol.Calls = append(symbol.Calls,
		atlas.SymbolCall{Kind: "invokes_external", Name: "web.Router", Line: 100, API: api, Values: []string{"/api/get-thing-0", "GET:GetThing0"}},
		atlas.SymbolCall{Kind: "invokes_external", Name: "web.Router", Line: 90, API: api, Dispatch: "function_value", Evidence: witness},
		atlas.SymbolCall{Kind: "invokes_external", Name: "web.AddNamespace", Line: 45, API: &atlas.CallAPI{Package: api.Package, Name: "AddNamespace"}},
	)
	symbol.Bindings = append(symbol.Bindings,
		atlas.SymbolBinding{From: "InitAPI", To: "ApiController.Finish", Kind: "binds_implementation",
			Detail: "parameter 2 -> web.Router; interface web.ControllerInterface method Finish func()", Path: "routers/router.go", Line: 100, Resolution: "exact"},
		atlas.SymbolBinding{From: "InitAPI", To: "McpController.Prepare", Kind: "binds_implementation",
			Detail: "parameter 2 -> web.Router; interface web.ControllerInterface method Prepare func()", Path: "routers/router.go", Line: 77, Resolution: "exact", Evidence: witness},
	)
	return place
}

// fieldJSON is a row's fields as JSON values by name.
func fieldJSON(t *testing.T, fields map[string]any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	return values
}

// unpackedCalls reads a packed calls list back into one object per call,
// each with exactly the fields it had: a table row is its shared fields and
// its non-null columns.
func unpackedCalls(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, entry := range entries {
		if entry["rows"] == nil {
			call, _ := json.Marshal(entry)
			calls = append(calls, string(call))
			continue
		}
		var shared map[string]json.RawMessage
		var columns []string
		var rows [][]json.RawMessage
		if json.Unmarshal(entry["shared"], &shared) != nil || json.Unmarshal(entry["columns"], &columns) != nil || json.Unmarshal(entry["rows"], &rows) != nil {
			t.Fatalf("call table %s", entry)
		}
		if string(entry["note"]) == "" {
			t.Fatal("a call table does not explain itself")
		}
		for _, row := range rows {
			call := make(map[string]json.RawMessage, len(shared)+len(columns))
			for name, value := range shared {
				call[name] = value
			}
			for i, name := range columns {
				if string(row[i]) != "null" {
					call[name] = row[i]
				}
			}
			encoded, _ := json.Marshal(call)
			calls = append(calls, string(encoded))
		}
	}
	slices.Sort(calls)
	return calls
}

// unpackedBindings reads a binding table back into one row per binding,
// as "to|detail|...|line" with every cell, so repeats count.
func unpackedBindings(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var bindings BindingTable
	if err := json.Unmarshal(raw, &bindings); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, row := range bindings.Rows {
		cells := map[string]any{}
		for name, value := range bindings.Shared {
			cells[name] = value
		}
		var lines []any
		for i, name := range bindings.Columns {
			if name == "lines" {
				lines = row[i].([]any)
				continue
			}
			cells[name] = row[i]
		}
		if lines == nil {
			lines = []any{cells["line"]}
		}
		for _, line := range lines {
			cells["line"] = line
			encoded, _ := json.Marshal(cells)
			out = append(out, string(encoded))
		}
	}
	slices.Sort(out)
	return out
}

func TestPackedSymbolRowKeepsEveryCallBindingValueAndLine(t *testing.T) {
	place := routerPlace(40)
	row := SymbolRow(place, "")
	packed := PackSymbolRow(row)
	original, compact := map[string]any{}, map[string]any{}
	for _, field := range row.Fields {
		original[field.Name] = field.Value
	}
	for _, field := range packed.Fields {
		compact[field.Name] = field.Value
	}
	before, after := fieldJSON(t, original), fieldJSON(t, compact)
	for name, value := range before {
		if name == "calls" || name == "callable_bindings" {
			continue
		}
		if string(after[name]) != string(value) {
			t.Fatalf("packing changed field %s", name)
		}
	}
	if len(after) != len(before) {
		t.Fatalf("packing changed the row's fields: %d, want %d", len(after), len(before))
	}
	if got, want := unpackedCalls(t, after["calls"]), unpackedCalls(t, before["calls"]); !slices.Equal(got, want) {
		t.Fatalf("packed calls lost or changed a call:\n%v\nwant\n%v", got, want)
	}
	if got, want := unpackedBindings(t, after["callable_bindings"]), unpackedBindings(t, before["callable_bindings"]); !slices.Equal(got, want) {
		t.Fatalf("packed bindings lost or changed a binding:\n%v\nwant\n%v", got, want)
	}
	if len(after["calls"])*3 > len(before["calls"]) || len(after["callable_bindings"])*3 > len(before["callable_bindings"]) {
		t.Fatalf("packing did not shrink the repeated evidence: calls %d -> %d, bindings %d -> %d",
			len(before["calls"]), len(after["calls"]), len(before["callable_bindings"]), len(after["callable_bindings"]))
	}

	// The shape a reader meets: one table per callee called more than once,
	// in place of its first call, its sites in their columns.
	var calls []map[string]json.RawMessage
	if err := json.Unmarshal(after["calls"], &calls); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0]["name"] == nil || calls[1]["rows"] == nil || calls[2]["name"] == nil {
		t.Fatalf("packed calls are not NSNamespace, one web.Router table, AddNamespace: %s", after["calls"])
	}
	var router callTable
	if err := json.Unmarshal(mustJSON(t, calls[1]), &router); err != nil {
		t.Fatal(err)
	}
	if router.Note != packedCallsNote || string(router.Shared["name"]) != `"web.Router"` || router.Shared["api"] == nil ||
		!slices.Equal(router.Columns, []string{"dispatch", "values", "evidence_refs", "lines"}) || len(router.Rows) != 41 {
		t.Fatalf("router table: note %q shared %v columns %v rows %d", router.Note, router.Shared, router.Columns, len(router.Rows))
	}
	var bindings BindingTable
	if err := json.Unmarshal(after["callable_bindings"], &bindings); err != nil {
		t.Fatal(err)
	}
	if bindings.Note != packedBindingsNote || !slices.Contains(bindings.Columns, "lines") || slices.Contains(bindings.Columns, "line") || len(bindings.Rows) != 2 {
		t.Fatalf("binding table: %s", after["callable_bindings"])
	}
	lines := bindings.Rows[0][slices.Index(bindings.Columns, "lines")].([]any)
	if len(lines) != 41 || lines[0] != float64(100) || lines[1] != float64(100) || lines[40] != float64(139) {
		t.Fatalf("merged lines are not ascending with the repeated line kept: %v", lines)
	}

	// Packing is deterministic, and a row with nothing repeated keeps its form.
	if again, _ := json.Marshal(PackSymbolRow(SymbolRow(place, "")).Fields); string(again) != string(mustJSON(t, packed.Fields)) {
		t.Fatal("packing the same row twice wrote different bytes")
	}
	plain := atlas.Place{ID: "symbol:plain", Path: "a.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "run", Kind: "function"},
		Calls: []atlas.SymbolCall{{Kind: "invokes_external", Name: "fmt.Println", Line: 3}}}}
	if got, want := mustJSON(t, PackSymbolRow(SymbolRow(plain, "")).Fields), mustJSON(t, SymbolRow(plain, "").Fields); string(got) != string(want) {
		t.Fatalf("a row with nothing repeated changed: %s", got)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
