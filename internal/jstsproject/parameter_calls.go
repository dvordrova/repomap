package jstsproject

import (
	"slices"

	"github.com/dvordrova/repomap/internal/programindex"
)

// handParameterCalls makes a call of a function's own parameter, which the
// function never assigns (Call.CalleeParameter), a call of what the
// program's calls into the function hand that parameter, as the Python, Go
// and C adapters join a parameter's callers: the one callable each call's
// argument there names, one exact, several alternatives, with the
// function_value dispatch and a function_value_store witness at each call
// handing one. A call handing any other value, one leaving the parameter to
// its default or to a spread, and a function the program reaches otherwise
// than by an exact call of it (a read of it as a value, a callable handed
// over, a construction, one of a call's alternatives) leave the call
// unresolved, the callables handed still its witnesses. A test's call hands
// a function outside the tests nothing.
func handParameterCalls(result Result, relations []programindex.RelationInput, callRelations map[string]int, declarations map[string]Declaration) {
	owners := map[string]bool{}
	for _, call := range result.Calls {
		if call.CalleeParameter > 0 {
			owners[call.CallerRef] = true
		}
	}
	if len(owners) == 0 {
		return
	}
	tested := map[string]bool{}
	for _, file := range result.Files {
		if file.Test {
			tested[file.FileRef] = true
		}
	}
	testedRef := func(ref string) bool {
		declaration, ok := declarations[ref]
		return ok && tested[declaration.Location.FileRef]
	}
	counts := func(from, owner string) bool { return testedRef(owner) || !testedRef(from) }
	entering := map[string][]Call{}
	otherwise := map[string]bool{}
	for _, call := range result.Calls {
		for _, callee := range call.CalleeRefs {
			if !owners[callee] || !counts(call.CallerRef, callee) {
				continue
			}
			if call.Invocation == "call" && call.Resolution == "exact" && len(call.CalleeRefs) == 1 && call.ExternalPackage == "" {
				entering[callee] = append(entering[callee], call)
			} else {
				otherwise[callee] = true
			}
		}
		if call.Pattern == nil {
			continue
		}
		for _, argument := range call.Pattern.Arguments {
			for _, ref := range argument.ObjectRefs {
				if owners[ref] && counts(call.CallerRef, ref) {
					otherwise[ref] = true
				}
			}
		}
	}
	for _, read := range result.Reads {
		for _, ref := range read.ToRefs {
			if owners[ref] && counts(read.FromRef, ref) {
				otherwise[ref] = true
			}
		}
	}
	for _, binding := range result.Bindings {
		for _, ref := range binding.ToRefs {
			if owners[ref] && counts(binding.FromRef, ref) {
				otherwise[ref] = true
			}
		}
	}
	name := func(ref string) string { return declarationDisplayName(declarations[ref], declarations) }
	for _, call := range result.Calls {
		position, owner := call.CalleeParameter, call.CallerRef
		index, ok := callRelations[call.Ref]
		if position == 0 || !ok {
			continue
		}
		relation := &relations[index]
		relation.Dispatch = programindex.DispatchFunctionValue
		known := !otherwise[owner] && len(entering[owner]) > 0
		var targets []string
		for _, into := range entering[owner] {
			handed := ""
			if into.Pattern != nil && (into.SpreadFrom == 0 || into.SpreadFrom > position) {
				for _, argument := range into.Pattern.Arguments {
					if argument.Position != position || len(argument.ObjectRefs) != 1 || argument.Resolution != "exact" {
						continue
					}
					if declaration, ok := declarations[argument.ObjectRefs[0]]; ok && (declaration.Kind == "function" || declaration.Kind == "method" || declaration.Kind == "lambda") {
						handed = argument.ObjectRefs[0]
					}
				}
			}
			if handed == "" {
				known = false
				continue
			}
			if !slices.Contains(targets, handed) {
				targets = append(targets, handed)
			}
			relation.Witnesses = append(relation.Witnesses, programindex.Witness{Kind: "function_value_store", Detail: name(handed) + " passed to " + name(owner),
				Location: programLocation(into.Location), ObjectRef: handed})
			relation.WitnessesObserved++
		}
		if known && len(targets) > 0 {
			slices.Sort(targets)
			relation.ToRefs, relation.TargetsObserved = targets, len(targets)
			relation.Resolution = programindex.ResolutionExact
			if len(targets) > 1 {
				relation.Resolution = programindex.ResolutionAlternatives
			}
		}
	}
}
