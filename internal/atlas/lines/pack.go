package lines

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

// Notes of the packed forms. Each packed field explains itself: the
// prompts are not changed, so every request that fits keeps its bytes.
const (
	packedBindingsNote = "Each row is one binding at every line in `lines`; a line listed twice holds two such bindings. Read values in columns order; shared fields apply to every row."
	packedCallsNote    = "Each row is one call of this callee with every line it occurs on; a line listed twice holds two such calls. Read values in columns order; shared fields apply to every row; null means not observed."
)

// PackSymbolRow is the lossless compact form of a symbol row whose
// categorizer question is over the envelope (table.Definition.Pack).
// Bindings that differ only in their line are one table row listing every
// line, in ascending order, a line repeated for each binding on it. The
// calls of one callee (the same kind, name and api) are one table in
// place of its first call: what every call shares once, what differs in
// columns, one row per call in order. Every binding, call, value, line and
// evidence ref stays, attributable to its own call or binding. Casdoor's
// InitAPI, 322 web.Router registrations, went from 150,741 to about 26,000
// bytes.
func PackSymbolRow(row table.Row) table.Row {
	packed := table.Row{ID: row.ID, Fields: slices.Clone(row.Fields)}
	for i, field := range packed.Fields {
		switch value := field.Value.(type) {
		case BindingTable:
			packed.Fields[i].Value = packBindings(value)
		case []*siteCall:
			packed.Fields[i].Value = packCalls(value)
		}
	}
	return packed
}

// packBindings merges the rows of a binding table that differ only in
// their line. A table whose line is shared has no such rows.
func packBindings(bindings BindingTable) BindingTable {
	at := slices.Index(bindings.Columns, "line")
	if at < 0 {
		return bindings
	}
	packed := BindingTable{Note: packedBindingsNote, Shared: bindings.Shared, Columns: slices.Clone(bindings.Columns)}
	packed.Columns[at] = "lines"
	merged := make(map[string]int)
	for _, row := range bindings.Rows {
		// Bindings writes plain values: names, an int line, strings, refs.
		identity, _ := json.Marshal(slices.Concat(row[:at], row[at+1:]))
		line := row[at].(int)
		if i, found := merged[string(identity)]; found {
			packed.Rows[i][at] = append(packed.Rows[i][at].([]int), line)
			continue
		}
		cells := slices.Clone(row)
		cells[at] = []int{line}
		merged[string(identity)] = len(packed.Rows)
		packed.Rows = append(packed.Rows, cells)
	}
	for _, row := range packed.Rows {
		slices.Sort(row[at].([]int))
	}
	return packed
}

// callTable is the calls of one callee in a packed row: the fields every
// call shares, and one row per call of the fields that differ.
type callTable struct {
	Note    string                     `json:"note"`
	Shared  map[string]json.RawMessage `json:"shared"`
	Columns []string                   `json:"columns"`
	Rows    [][]json.RawMessage        `json:"rows"`
}

// packCalls writes the calls of each callee called more than once as one
// table, in place of its first call; a callee called once keeps its call.
func packCalls(calls []*siteCall) []any {
	var order []string
	groups := make(map[string][]*siteCall)
	for _, call := range calls {
		// A callee is its kind, name and exact external symbol: strings.
		identity, _ := json.Marshal([]any{call.Kind, call.Name, call.API})
		if groups[string(identity)] == nil {
			order = append(order, string(identity))
		}
		groups[string(identity)] = append(groups[string(identity)], call)
	}
	packed := make([]any, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if len(group) == 1 {
			packed = append(packed, group[0])
			continue
		}
		factored, ok := callsTable(group)
		if !ok {
			for _, call := range group {
				packed = append(packed, call)
			}
			continue
		}
		packed = append(packed, factored)
	}
	return packed
}

// callsTable factors the calls of one callee like a binding table: a field
// equal on every call is shared, any other is a column in the order a call
// writes its fields, null where a call has none.
func callsTable(calls []*siteCall) (callTable, bool) {
	fields := make([]map[string]json.RawMessage, len(calls))
	for i, call := range calls {
		raw, err := json.Marshal(call)
		if err != nil || json.Unmarshal(raw, &fields[i]) != nil {
			return callTable{}, false
		}
	}
	names := callFieldOrder()
	for _, call := range fields {
		for name := range call {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	factored := callTable{Note: packedCallsNote, Shared: make(map[string]json.RawMessage)}
	for _, name := range names {
		first, present := fields[0][name]
		shared := present
		for _, call := range fields[1:] {
			if value, found := call[name]; !found || !bytes.Equal(value, first) {
				shared = false
				break
			}
		}
		if shared {
			factored.Shared[name] = first
			continue
		}
		used := false
		for _, call := range fields {
			if _, found := call[name]; found {
				used = true
				break
			}
		}
		if used {
			factored.Columns = append(factored.Columns, name)
		}
	}
	for _, call := range fields {
		row := make([]json.RawMessage, len(factored.Columns))
		for i, name := range factored.Columns {
			row[i] = json.RawMessage("null")
			if value, found := call[name]; found {
				row[i] = value
			}
		}
		factored.Rows = append(factored.Rows, row)
	}
	return factored, true
}

// callFieldOrder lists a call's JSON field names in the order the encoder
// writes them, embedded fields in their place; a later field of the same
// name, which the encoder writes in the first one's stead, is not repeated.
func callFieldOrder() []string {
	var names []string
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := range t.NumField() {
			field := t.Field(i)
			tag := field.Tag.Get("json")
			if tag == "-" || !field.IsExported() && !field.Anonymous {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if field.Anonymous && name == "" && field.Type.Kind() == reflect.Struct {
				walk(field.Type)
				continue
			}
			if name == "" {
				name = field.Name
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	walk(reflect.TypeFor[siteCall]())
	return names
}
