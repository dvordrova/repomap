package lines

import (
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// EvidenceCatalog shares repeated source observations inside one provider row.
// The native facts remain untouched. Refs are local to this row, allocated in
// encounter order, and retain every source-distinct observation and association.
type EvidenceCatalog struct {
	// OmitDefaults drops a call's invocation when it is synchronous and its
	// resolution when it is exact. The evidence vocabulary attached to the
	// symbols and operations prompts defines both defaults; rows of other
	// tables keep every value until their prompts carry it too.
	OmitDefaults bool
	refs         map[atlas.EdgeEvidence]string
	byRef        map[string]atlas.EdgeEvidence
}

// DefaultInvocation and DefaultResolution are the values a rendered call
// leaves out under OmitDefaults.
const (
	DefaultInvocation = "synchronous"
	DefaultResolution = "exact"
)

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

type callEvidence struct {
	atlas.SymbolCall
	HasRepositoryCalleeCandidate bool     `json:"has_repository_callee_candidate,omitempty"`
	EvidenceRefs                 []string `json:"evidence_refs,omitempty"`
}

func (c *EvidenceCatalog) Call(call atlas.SymbolCall) any {
	return c.call(call)
}

// CallWithOrigins retains the receiver, arguments and result origins needed
// to distinguish uses of the same API, in their request form: kind and text
// per node, parts nested at most OriginDepth levels. These are syntax
// observations, including unknowns and alternatives, not resolved runtime
// values. The complete origins, with their anchors and owners, stay in the
// atlas structures where the destination traversal reads them.
func (c *EvidenceCatalog) CallWithOrigins(call atlas.SymbolCall) any {
	return c.callWithOrigins(call)
}

func (c *EvidenceCatalog) callWithOrigins(call atlas.SymbolCall) callEvidence {
	projected := c.call(call)
	if len(call.SourceArguments) > 0 {
		projected.SourceArguments = make([]atlas.SourceArgument, len(call.SourceArguments))
		for i, argument := range call.SourceArguments {
			projected.SourceArguments[i] = atlas.SourceArgument{Position: argument.Position, Keyword: argument.Keyword, Origin: compactOrigin(argument.Origin, 0)}
		}
	}
	projected.ReceiverValue, projected.ResultValue = compactOrigin(call.ReceiverValue, 0), compactOrigin(call.ResultValue, 0)
	projected.API, projected.Column = call.API, call.Column
	return projected
}

// OriginDepth is how many levels of parts a request origin keeps below its
// root node. Morfeu's orientation request carried origin trees nine levels
// deep with an anchor and owner on every node, 90 KB of a 280 KB request,
// and the answer used five nodes of 118.
const OriginDepth = 2

func compactOrigin(value *sourcevalue.Value, level int) *sourcevalue.Value {
	if value == nil {
		return nil
	}
	result := &sourcevalue.Value{Kind: value.Kind, Text: value.Text}
	if level >= OriginDepth {
		return result
	}
	result.Initializer = compactOrigin(value.Initializer, level+1)
	for i := range value.Parts {
		result.Parts = append(result.Parts, *compactOrigin(&value.Parts[i], level+1))
	}
	return result
}

func (c *EvidenceCatalog) call(call atlas.SymbolCall) callEvidence {
	// This only establishes that at least one observed callee candidate is
	// indexed in the repository. Resolution still distinguishes exact and
	// possible dispatch; other candidates may remain external or unresolved.
	hasRepositoryCallee := len(call.CalleeIDs) > 0
	call.CalleeIDs = nil
	call.SourceArguments = nil // Read by destination traversal with source anchors.
	call.ReceiverValue, call.ResultValue = nil, nil
	call.Column = 0 // The exact native identity stays local.
	refs := c.references(call.Evidence)
	call.Evidence = nil
	if c.OmitDefaults {
		if call.Invocation == DefaultInvocation {
			call.Invocation = ""
		}
		if call.Resolution == DefaultResolution {
			call.Resolution = ""
		}
	}
	return callEvidence{call, hasRepositoryCallee, refs}
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
	// A binding without arguments or evidence has no such field, not a null.
	if len(rows) == 1 {
		binding := make(map[string]any, len(columns))
		for i, name := range columns {
			if !absent(rows[0][i]) {
				binding[name] = rows[0][i]
			}
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
			if !absent(rows[0][j]) {
				result.Shared[name] = rows[0][j]
			}
			continue
		}
		result.Columns = append(result.Columns, name)
		for i, row := range rows {
			result.Rows[i] = append(result.Rows[i], row[j])
		}
	}
	return result
}

// absent reports a value that would render as null: a nil slice of
// arguments or refs. An empty string or zero line is still a value.
func absent(value any) bool {
	if value == nil {
		return true
	}
	switch reflected := reflect.ValueOf(value); reflected.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
		return reflected.IsNil()
	}
	return false
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
