package reading

import (
	"context"
	"fmt"
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"sort"
	"strings"
)

// Review candidate activations with neighbouring declarations. Descriptions
// stay on symbols; this separate interpretation records whether to expose one
// as an operation, with its own cached evidence and knowledge identity.
func (r *reader) readOperations(ctx context.Context) error {
	def := lines.Operations()
	// A first model's negative answer must not suppress observed activation
	// evidence. Callbacks and asynchronous entrants are reviewed even when the
	// description table missed them. This selects candidates, never a role.
	candidates := make(map[string]bool)
	nativeRoutes := operationNativeRoutes(r.opts.Graph)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		if file := r.places[place.Parent]; file.File != nil && file.File.Generated {
			continue
		}
		_, proposed := r.operations[place.ID]
		candidates[place.ID] = proposed || observedActivation(place) || len(nativeRoutes[place.ID]) > 0
	}
	declarations := make(map[string]atlas.Place)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		if id := place.Symbol.Decl.ObjectID; id != "" {
			declarations[id] = place
		}
		declarations[place.ID] = place
	}
	var rows []table.Row
	var subjects []string
	names := make(map[string]map[string]string)
	for _, place := range r.opts.Graph.Places {
		if !candidates[place.ID] {
			continue
		}
		id := "operation:" + place.ID
		// The review is its own internal entity, attached to the source symbol.
		r.places[id] = atlas.Place{ID: id, Kind: atlas.PlaceEntity, Path: place.Path, LineNo: place.LineNo, Parent: place.ID, TargetIDs: append([]string(nil), place.TargetIDs...)}
		decl := place.Symbol.Decl
		var receivedBindings []atlas.SymbolBinding
		var suppliedCallbacks []string
		for _, binding := range place.Symbol.Bindings {
			if binding.To == decl.Name {
				receivedBindings = append(receivedBindings, binding)
			}
			if binding.From == decl.Name {
				suppliedCallbacks = append(suppliedCallbacks, binding.To)
			}
		}
		// Review source evidence, not the previous model's answer. Repeating
		// that answer encouraged confirmation instead of separating callbacks
		// from their helpers. Keep each own call's receiver and arguments: a
		// request mutation and a response mutation may use the same native API.
		var evidence lines.EvidenceCatalog
		var calls []any
		for _, call := range place.Symbol.Calls {
			calls = append(calls, evidence.CallWithOrigins(call))
		}
		callers := operationCallerEvidence(place, declarations)
		nameFields, registeredNames := operationRegisteredNames(receivedBindings, nativeRoutes[place.ID]...)
		names[place.ID] = registeredNames
		row := table.Row{ID: id, Fields: []table.Field{
			{Name: "path", Value: place.Path}, {Name: "name", Value: decl.Name},
			{Name: "signature", Value: decl.Signature}, {Name: "author_doc", Value: decl.Doc},
			{Name: "registrations_of_this_declaration", Value: evidence.Bindings(receivedBindings)},
			{Name: "registers_other_callables", Value: suppliedCallbacks},
			{Name: "calls", Value: calls}, {Name: "observed_callers", Value: callers},
		}}
		row.Fields = append(row.Fields, evidence.Fields()...)
		row.Fields = append(row.Fields, nameFields...)
		rows = append(rows, row)
		subjects = append(subjects, place.ID)
	}
	r.opts.Stage(def.Stage, fmt.Sprintf("%d candidate activations with neighbouring declarations", len(rows)))
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	for i, id := range subjects {
		answer := answers[i].answer
		if answer == nil || answer["activation"] == "none" || answer["entry"] != "self" {
			delete(r.operations, id)
			continue
		}
		operation := r.operations[id]
		operation[0], operation[1], operation[2] = answer["activation"], answer["name"], answer["description"]
		if answer["name_kind"] == "http" {
			operation[1] = answer["http_method"] + " " + names[id][answer["http_path"]]
		}
		r.operations[id] = operation
	}
	r.reportStage(def.Stage)
	return nil
}

// Native routes already identify their handler through the canonical symbol
// place. A decorator does not have to masquerade as a callback binding to make
// its source path available to operation review.
func operationNativeRoutes(graph atlas.Graph) map[string][]atlas.Place {
	result := make(map[string][]atlas.Place)
	for _, place := range graph.Places {
		b := place.Boundary
		if b == nil || b.Source != "fact" || b.SubjectID == "" || b.Direction != atlas.DirectionIn ||
			b.GivenKind != atlas.BoundaryHTTPServer || b.Method == "" || len(b.Values) == 0 {
			continue
		}
		result[b.SubjectID] = append(result[b.SubjectID], place)
	}
	return result
}

func operationRegisteredNames(bindings []atlas.SymbolBinding, routes ...atlas.Place) ([]table.Field, map[string]string) {
	seen := make(map[atlas.RegistrationArgument]bool)
	var arguments []atlas.RegistrationArgument
	for _, binding := range bindings {
		for _, argument := range binding.Arguments {
			if argument.Kind == "literal_string" && !seen[argument] {
				seen[argument] = true
				arguments = append(arguments, argument)
			}
		}
	}
	sort.Slice(arguments, func(i, j int) bool {
		a, b := arguments[i], arguments[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		if a.Keyword != b.Keyword {
			return a.Keyword < b.Keyword
		}
		return a.Value < b.Value
	})
	var catalogue []map[string]any
	var refs []string
	names := make(map[string]string)
	// Prefer the complete native path, including source-observed router mounts.
	// Raw callback literals remain useful when no native HTTP fact is available.
	sort.Slice(routes, func(i, j int) bool { return routes[i].ID < routes[j].ID })
	for _, route := range routes {
		for _, path := range route.Boundary.Values {
			ref := fmt.Sprintf("p%d", len(refs)+1)
			refs = append(refs, ref)
			names[ref] = path
			catalogue = append(catalogue, map[string]any{"ref": ref, "http_route": map[string]any{
				"method": route.Boundary.Method, "path": path, "source_path": route.Path,
				"line": route.LineNo, "column": route.Column,
			}})
		}
	}
	if len(routes) > 0 {
		arguments = nil
	}
	for i, argument := range arguments {
		ref := fmt.Sprintf("p%d", i+1)
		refs = append(refs, ref)
		names[ref] = argument.Value
		catalogue = append(catalogue, map[string]any{"ref": ref, "argument": argument})
	}
	kinds := []string{"label"}
	if len(refs) > 0 {
		kinds = append(kinds, "http")
	}
	return []table.Field{{Name: "registered_names", Value: catalogue}, {Name: "registered_name_options", Value: refs},
		{Name: "name_kind_options", Value: kinds}}, names
}

// Supply immediate native callers only. Each declaration appears once with
// every distinct call site; neither its outgoing calls nor its own callers
// are recursively expanded. Missing symbol places retain their native
// signature and locations. Names never substitute for identity.
func operationCallerEvidence(place atlas.Place, declarations map[string]atlas.Place) []map[string]any {
	byID := make(map[string]map[string]any)
	for i, caller := range place.Symbol.CalledBy {
		id := caller.ObjectID
		if caller.PlaceID != "" {
			id = caller.PlaceID
		}
		if id == "" {
			id = fmt.Sprintf("observation:%d", i)
		}
		row := byID[id]
		if row == nil {
			row = map[string]any{"name": caller.Name, "signature": caller.Signature, "path": caller.Path}
			if declaration, ok := declarations[id]; ok && declaration.Symbol != nil {
				row["author_doc"] = declaration.Symbol.Decl.Doc
				row["declaration_line"] = declaration.LineNo
				// This row needs to know how its caller is activated, not all
				// help/metadata fields of that caller's registration object.
				// Those literal observations belong in the caller's own review.
				var bindings []atlas.SymbolBinding
				for _, binding := range declaration.Symbol.Bindings {
					if binding.To != declaration.Symbol.Decl.Name {
						continue
					}
					binding.Evidence = nil
					binding.Arguments = nil
					bindings = append(bindings, binding)
				}
				var evidence lines.EvidenceCatalog
				row["callable_bindings"] = evidence.Bindings(bindings)
			}
			byID[id] = row
		}
		sites, _ := row["call_sites"].([]map[string]any)
		row["call_sites"] = append(sites, map[string]any{"line": caller.Line, "kind": caller.Kind, "invocation": caller.Invocation, "resolution": caller.Resolution})
	}
	keys := make([]string, 0, len(byID))
	for id := range byID {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	var rows []map[string]any
	for _, id := range keys {
		rows = append(rows, byID[id])
	}
	return rows
}

func observedActivation(place atlas.Place) bool {
	for _, binding := range place.Symbol.Bindings {
		if binding.To == place.Symbol.Decl.Name {
			return true
		}
	}
	for _, caller := range place.Symbol.CalledBy {
		if caller.Kind == "executes" || strings.Contains(caller.Invocation, "goroutine") || strings.Contains(caller.Invocation, "asynchronous") || strings.HasPrefix(caller.Invocation, "coroutine_result_argument:") {
			return true
		}
	}
	return false
}
