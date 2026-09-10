package lines

import (
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
)

// A declaration anchor is source context, not a new selectable source or a
// canonical symbol identity on the wire.
type questionDeclaration struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

func questionDeclarationOf(place atlas.Place) *questionDeclaration {
	if place.Symbol == nil {
		return nil
	}
	return &questionDeclaration{Name: place.Symbol.Decl.Name, Path: place.Path,
		Line: place.LineNo, Column: place.Symbol.Decl.Column}
}

type questionCall struct {
	callEvidence
	CalleeCandidates []questionDeclaration `json:"callee_candidates,omitempty"`
}

type questionCaller struct {
	atlas.SymbolCaller
	Column      int                  `json:"column,omitempty"`
	Declaration *questionDeclaration `json:"caller_declaration,omitempty"`
}

// Attach only this declaration's observations. In particular a selected type
// does not recursively pull in its methods' calls, and an incoming caller does
// not donate its other calls to the selected declaration.
func questionCallableEvidence(facts map[string]any, place atlas.Place, places, symbols map[string]atlas.Place) {
	var evidence EvidenceCatalog
	var calls []questionCall
	for _, call := range place.Symbol.Calls {
		projected := questionCall{callEvidence: evidence.call(call)}
		// Question reading needs exact source sites and native API identity.
		// Other tables keep their existing compact Call projection.
		projected.Column, projected.API = call.Column, call.API
		for _, id := range call.CalleeIDs {
			if declaration := questionDeclarationOf(places[id]); declaration != nil {
				projected.CalleeCandidates = append(projected.CalleeCandidates, *declaration)
			}
		}
		calls = append(calls, projected)
	}
	if len(calls) > 0 {
		facts["calls"] = calls
	}
	var callers []questionCaller
	for _, caller := range place.Symbol.CalledBy {
		original := places[caller.PlaceID]
		if original.Symbol == nil && caller.ObjectID != "" {
			original = symbols[caller.ObjectID]
		}
		projected := questionCaller{SymbolCaller: caller, Declaration: questionDeclarationOf(original)}
		projected.PlaceID, projected.ObjectID = "", ""
		// SymbolCaller owns the native relationship and its certainty. Its
		// exact columns come only from matching native outgoing call records;
		// multiple calls on one line remain multiple source sites.
		columns := make(map[int]bool)
		if original.Symbol != nil && original.Path == caller.Path {
			for _, call := range original.Symbol.Calls {
				if call.Line == caller.Line && call.Kind == caller.Kind &&
					call.Invocation == caller.Invocation && call.Resolution == caller.Resolution &&
					slices.Contains(call.CalleeIDs, place.ID) {
					columns[call.Column] = true
				}
			}
		}
		if len(columns) == 0 {
			callers = append(callers, projected)
			continue
		}
		ordered := make([]int, 0, len(columns))
		for column := range columns {
			ordered = append(ordered, column)
		}
		sort.Ints(ordered)
		for _, column := range ordered {
			one := projected
			one.Column = column
			callers = append(callers, one)
		}
	}
	if len(callers) > 0 {
		facts["called_by"] = callers
	}
	if len(evidence.byRef) > 0 {
		facts["source_evidence"] = map[string]any{
			"note":   "Each evidence_refs list refers only to this declaration's by_ref catalogue. Another declaration may reuse a ref spelling. Observations retain each call's original uncertainty and establish no additional calls.",
			"by_ref": evidence.byRef,
		}
	}
}
