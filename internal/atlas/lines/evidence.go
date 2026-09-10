package lines

import (
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// EvidenceCatalog shares repeated source observations inside one provider row.
// The native facts remain untouched. Refs are local to this row, allocated in
// encounter order, and retain every source-distinct observation and association.
type EvidenceCatalog struct {
	refs  map[atlas.EdgeEvidence]string
	byRef map[string]atlas.EdgeEvidence
}

func (c *EvidenceCatalog) references(evidence []atlas.EdgeEvidence) []string {
	if c.refs == nil {
		c.refs = make(map[atlas.EdgeEvidence]string)
		c.byRef = make(map[string]atlas.EdgeEvidence)
	}
	var refs []string
	for _, observation := range evidence {
		ref, ok := c.refs[observation]
		if !ok {
			ref = fmt.Sprintf("e%d", len(c.refs)+1)
			c.refs[observation] = ref
			c.byRef[ref] = observation
		}
		refs = append(refs, ref)
	}
	return refs
}

func (c *EvidenceCatalog) Call(call atlas.SymbolCall) any {
	// This only establishes that at least one observed callee candidate is
	// indexed in the repository. Resolution still distinguishes exact and
	// possible dispatch; other candidates may remain external or unresolved.
	hasRepositoryCallee := len(call.CalleeIDs) > 0
	call.CalleeIDs = nil
	call.SourceArguments = nil // Read by destination traversal with source anchors.
	call.API = nil // Exact package authority is used by local mechanism matching.
	call.ReceiverValue, call.ResultValue = nil, nil
	call.Column = 0 // The exact native identity stays local.
	refs := c.references(call.Evidence)
	call.Evidence = nil
	return struct {
		atlas.SymbolCall
		HasRepositoryCalleeCandidate bool     `json:"has_repository_callee_candidate,omitempty"`
		EvidenceRefs                 []string `json:"evidence_refs,omitempty"`
	}{call, hasRepositoryCallee, refs}
}

// BindingTable factors shared fields out of a registration list. A row still
// names its own candidate callable, source site and resolution; common fields
// are repeated logically, not discarded or combined into a new observation.
type BindingTable struct {
	Note    string         `json:"note"`
	Shared  map[string]any `json:"shared"`
	Columns []string       `json:"columns"`
	Rows    [][]any        `json:"rows"`
}

func (c *EvidenceCatalog) Bindings(bindings []atlas.SymbolBinding) any {
	if len(bindings) == 0 {
		return nil
	}
	columns := []string{"from", "to", "detail", "invocation", "resolution", "path", "line", "arguments", "evidence_refs"}
	var rows [][]any
	for _, binding := range bindings {
		refs := c.references(binding.Evidence)
		rows = append(rows, []any{binding.From, binding.To, binding.Detail, binding.Invocation, binding.Resolution, binding.Path, binding.Line, binding.Arguments, refs})
	}
	if len(rows) == 1 {
		binding := make(map[string]any, len(columns))
		for i, name := range columns {
			binding[name] = rows[0][i]
		}
		return binding
	}
	result := BindingTable{Note: "Each row is one binding. Read values in columns order; shared fields apply to every row. Row numbers start at 1.", Shared: make(map[string]any), Rows: make([][]any, len(rows))}
	for j, name := range columns {
		shared := true
		for _, row := range rows[1:] {
			if !reflect.DeepEqual(row[j], rows[0][j]) {
				shared = false
				break
			}
		}
		if shared {
			result.Shared[name] = rows[0][j]
			continue
		}
		result.Columns = append(result.Columns, name)
		for i, row := range rows {
			result.Rows[i] = append(result.Rows[i], row[j])
		}
	}
	return result
}

func (c *EvidenceCatalog) Fields() []table.Field {
	if len(c.byRef) == 0 {
		return nil
	}
	return []table.Field{{Name: "source_evidence", Value: struct {
		Note  string                        `json:"note"`
		ByRef map[string]atlas.EdgeEvidence `json:"by_ref"`
	}{"Every evidence_refs list in this row refers to these source observations. Shared refs preserve each call or binding's own evidence; they do not establish extra calls or runtime identity.", c.byRef}}}
}
