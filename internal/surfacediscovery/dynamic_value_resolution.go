package surfacediscovery

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

// A summary is immutable after return. Unknown frontiers count paths, while
// functions and assignment locations are the same sets sealed by the index.
// witnesses are callables an interface field was given when a store under a
// branch left that field open: evidence of the frontier, never candidates.
type dynamicValueSummary struct {
	functions   map[*ssa.Function]godynamichandoff.CandidateEvidence
	assignments map[*ssa.Function]map[godynamichandoff.Location]struct{}
	witnesses   map[*ssa.Function]map[dynamicFieldWitness]struct{}
	unresolved  int
	cyclic      bool
}

// dynamicFieldWitness is one store that put a witness callable into a field,
// or one call handing it to passedTo's parameter that passedTo calls.
type dynamicFieldWitness struct {
	field       string
	location    godynamichandoff.Location
	underBranch bool
	passedTo    *ssa.Function
}

type dynamicValueKey struct {
	value       ssa.Value
	throughFlow bool
}

// Each resolver belongs to one root and one interface method, if applicable.
// Active-path results depend on their ancestors and must never enter memo.
type dynamicValueResolver struct {
	analyzer *analyzer
	method   *types.Func
	active   map[ssa.Value]bool
	memo     map[dynamicValueKey]dynamicValueSummary
	err      error
}

func newDynamicValueResolver(a *analyzer, method *types.Func) *dynamicValueResolver {
	return &dynamicValueResolver{analyzer: a, method: method,
		active: make(map[ssa.Value]bool), memo: make(map[dynamicValueKey]dynamicValueSummary)}
}

func resolveDynamicFunctionValue(value ssa.Value, analyzers ...*analyzer) (dynamicValueSummary, error) {
	var a *analyzer
	if len(analyzers) > 0 {
		a = analyzers[0]
	}
	r := newDynamicValueResolver(a, nil)
	result := r.functionValue(value, false)
	return result, r.err
}

func resolveDynamicInterfaceValue(a *analyzer, value ssa.Value, method *types.Func) (dynamicValueSummary, error) {
	r := newDynamicValueResolver(a, method)
	result := r.interfaceValue(value)
	return result, r.err
}

func (r *dynamicValueResolver) start(key dynamicValueKey) (dynamicValueSummary, bool) {
	if r.err != nil {
		return dynamicValueSummary{}, true
	}
	if key.value == nil || r.active[key.value] {
		return dynamicValueSummary{unresolved: 1, cyclic: r.active[key.value]}, true
	}
	if result, ok := r.memo[key]; ok {
		return result, true
	}
	r.active[key.value] = true
	return dynamicValueSummary{}, false
}

func (r *dynamicValueResolver) finish(key dynamicValueKey, result dynamicValueSummary) dynamicValueSummary {
	delete(r.active, key.value)
	if r.err == nil && !result.cyclic {
		r.memo[key] = result
	}
	return result
}

func (r *dynamicValueResolver) merge(result *dynamicValueSummary, child dynamicValueSummary) {
	if r.err != nil {
		return
	}
	r.err = addDynamicUnknown(&result.unresolved, child.unresolved)
	result.cyclic = result.cyclic || child.cyclic
	for function, evidence := range child.functions {
		if result.functions == nil {
			result.functions = make(map[*ssa.Function]godynamichandoff.CandidateEvidence)
		}
		// The original walker overwrites evidence in child order. Ranking it
		// here would change the authority of repeated direct/flow witnesses.
		result.functions[function] = evidence
	}
	for function, locations := range child.assignments {
		for location := range locations {
			result.assign(function, location)
		}
	}
	for function, stores := range child.witnesses {
		for store := range stores {
			result.witness(function, store)
		}
	}
}

func (result *dynamicValueSummary) assign(function *ssa.Function, location godynamichandoff.Location) {
	if result.assignments == nil {
		result.assignments = make(map[*ssa.Function]map[godynamichandoff.Location]struct{})
	}
	if result.assignments[function] == nil {
		result.assignments[function] = make(map[godynamichandoff.Location]struct{})
	}
	result.assignments[function][location] = struct{}{}
}

func (result *dynamicValueSummary) witness(function *ssa.Function, store dynamicFieldWitness) {
	if result.witnesses == nil {
		result.witnesses = make(map[*ssa.Function]map[dynamicFieldWitness]struct{})
	}
	if result.witnesses[function] == nil {
		result.witnesses[function] = make(map[dynamicFieldWitness]struct{})
	}
	result.witnesses[function][store] = struct{}{}
}

// wrapperTarget returns the method a synthetic bound method wrapper or thunk
// delegates to: the closure made for a method value such as s.handleList,
// or a method expression such as (*Server).handleList. A handler registered
// that way is then the method itself, not a wrapper no repository package
// owns; before this, mux.HandleFunc("/items", s.handleList) left the route
// without a handler and the handler without a route. Other functions return
// unchanged, as does a wrapper around an interface method, whose tail call
// has no static callee.
func wrapperTarget(function *ssa.Function) *ssa.Function {
	if function == nil || !(strings.HasPrefix(function.Synthetic, "bound method wrapper") || strings.HasPrefix(function.Synthetic, "thunk")) {
		return function
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				if callee := call.Common().StaticCallee(); callee != nil {
					return callee
				}
			}
		}
	}
	return function
}

func (r *dynamicValueResolver) functionValue(value ssa.Value, throughFlow bool) dynamicValueSummary {
	key := dynamicValueKey{value, throughFlow}
	if result, done := r.start(key); done {
		return result
	}
	result := dynamicValueSummary{unresolved: 1}
	switch current := value.(type) {
	case *ssa.Function:
		evidence := godynamichandoff.EvidenceDirectFunctionValue
		if throughFlow {
			evidence = godynamichandoff.EvidenceUniqueValueFlow
		}
		result = dynamicValueSummary{functions: map[*ssa.Function]godynamichandoff.CandidateEvidence{wrapperTarget(current): evidence}}
	case *ssa.MakeClosure:
		if function, ok := current.Fn.(*ssa.Function); ok {
			evidence := godynamichandoff.EvidenceClosureValue
			if throughFlow {
				evidence = godynamichandoff.EvidenceUniqueValueFlow
			}
			result = dynamicValueSummary{functions: map[*ssa.Function]godynamichandoff.CandidateEvidence{wrapperTarget(function): evidence}}
		}
	case *ssa.Phi:
		if len(current.Edges) > 0 {
			result = dynamicValueSummary{}
			for _, edge := range current.Edges {
				r.merge(&result, r.functionValue(edge, true))
			}
		}
	case *ssa.ChangeType:
		result = r.functionValue(current.X, true)
	case *ssa.Convert:
		result = r.functionValue(current.X, true)
	case *ssa.MakeInterface:
		result = r.functionValue(current.X, true)
	case *ssa.ChangeInterface:
		result = r.functionValue(current.X, true)
	case *ssa.Call:
		result = r.functionReturns(current, 0)
	case *ssa.Extract:
		if call, ok := current.Tuple.(*ssa.Call); ok {
			result = r.functionReturns(call, current.Index)
		}
	case *ssa.UnOp:
		if current.Op == token.MUL {
			if address, ok := current.X.(*ssa.FieldAddr); ok {
				if field := dynamicSourceField(address.X, address.Field); field != nil {
					result = r.functionField(address.X, field, current, make(map[ssa.Value]bool))
				}
			}
		}
	case *ssa.Field:
		if field := dynamicSourceField(current.X, current.Field); field != nil {
			result = r.functionField(current.X, field, current, make(map[ssa.Value]bool))
		}
	}
	return r.finish(key, result)
}

func dynamicSourceField(receiver ssa.Value, index int) *types.Var {
	if receiver == nil {
		return nil
	}
	typ := receiver.Type().Underlying()
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem().Underlying()
	}
	if structure, ok := typ.(*types.Struct); ok && index >= 0 && index < structure.NumFields() {
		return structure.Field(index)
	}
	return nil
}

// Callable fields follow the actual allocated receiver and its source factory
// return. A unique initialization before the read is required; stores to other
// instances of the same type never supply a handler for this one.
func (r *dynamicValueResolver) functionField(receiver ssa.Value, field *types.Var, read ssa.Instruction, active map[ssa.Value]bool) dynamicValueSummary {
	unknown := dynamicValueSummary{unresolved: 1}
	if receiver == nil || active[receiver] || r.analyzer == nil {
		return unknown
	}
	active[receiver] = true
	defer delete(active, receiver)
	switch value := receiver.(type) {
	case *ssa.Alloc:
		var stores []*ssa.Store
		if refs := value.Referrers(); refs != nil {
			for _, ref := range *refs {
				address, ok := ref.(*ssa.FieldAddr)
				if !ok || address.X != value || dynamicSourceField(value, address.Field) != field || address.Referrers() == nil {
					continue
				}
				for _, observation := range *address.Referrers() {
					if store, ok := observation.(*ssa.Store); ok && store.Addr == address {
						stores = append(stores, store)
					}
				}
			}
		}
		if len(stores) == 1 && stores[0].Block() == value.Block() && sourceStoreBeforeRead(stores[0], read) {
			return r.functionValue(stores[0].Val, true)
		}
	case *ssa.Call:
		// A caller-side write to this returned instance makes its constructor
		// initialization insufficient. Do not substitute an old callback.
		if refs := value.Referrers(); refs != nil {
			for _, ref := range *refs {
				if address, ok := ref.(*ssa.FieldAddr); ok && dynamicSourceField(value, address.Field) == field && address.Referrers() != nil {
					for _, use := range *address.Referrers() {
						if store, ok := use.(*ssa.Store); ok && store.Addr == address {
							return unknown
						}
					}
				}
			}
		}
		callee := value.Common().StaticCallee()
		if callee == nil || !r.analyzer.isRepositoryFunction(callee) || len(callee.Blocks) == 0 {
			return unknown
		}
		result, found := dynamicValueSummary{}, false
		for _, block := range callee.Blocks {
			for _, instruction := range block.Instrs {
				if returned, ok := instruction.(*ssa.Return); ok && len(returned.Results) > 0 {
					found = true
					r.merge(&result, r.functionField(returned.Results[0], field, returned, active))
				}
			}
		}
		if found {
			return result
		}
	case *ssa.ChangeType:
		return r.functionField(value.X, field, read, active)
	case *ssa.Convert:
		return r.functionField(value.X, field, read, active)
	}
	return unknown
}

// A source factory can supply a callable without calling it. Only the actual
// repository callee's retained return values participate; dependency bodies,
// unresolved calls and parameter substitution remain open frontiers.
func (r *dynamicValueResolver) functionReturns(call *ssa.Call, index int) dynamicValueSummary {
	callee := call.Common().StaticCallee()
	if r.analyzer == nil || callee == nil || !r.analyzer.isRepositoryFunction(callee) || len(callee.Blocks) == 0 {
		return dynamicValueSummary{unresolved: 1}
	}
	result, found := dynamicValueSummary{}, false
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			found = true
			if index >= len(returned.Results) {
				r.merge(&result, dynamicValueSummary{unresolved: 1})
			} else {
				r.merge(&result, r.functionValue(returned.Results[index], true))
			}
		}
	}
	if !found {
		result.unresolved = 1
	}
	return result
}

func (r *dynamicValueResolver) interfaceValue(value ssa.Value) dynamicValueSummary {
	if r.analyzer == nil || r.analyzer.program == nil || r.method == nil {
		return dynamicValueSummary{unresolved: 1}
	}
	key := dynamicValueKey{value: value}
	if result, done := r.start(key); done {
		return result
	}
	result := dynamicValueSummary{unresolved: 1}
	switch current := value.(type) {
	case *ssa.MakeInterface:
		if current.X != nil && current.X.Type() != nil {
			if implementation := r.analyzer.program.LookupMethod(current.X.Type(), r.method.Pkg(), r.method.Name()); implementation != nil {
				result = dynamicValueSummary{functions: map[*ssa.Function]godynamichandoff.CandidateEvidence{implementation: ""}}
			}
		}
	case *ssa.ChangeInterface:
		result = r.interfaceValue(current.X)
	case *ssa.Phi:
		if len(current.Edges) > 0 {
			result = dynamicValueSummary{}
			for _, edge := range current.Edges {
				r.merge(&result, r.interfaceValue(edge))
			}
		}
	case *ssa.UnOp:
		result = r.interfaceField(current)
	case *ssa.Call:
		result = r.interfaceReturns(current, 0)
	case *ssa.Extract:
		if call, ok := current.Tuple.(*ssa.Call); ok {
			result = r.interfaceReturns(call, current.Index)
		}
	case *ssa.Parameter:
		result = r.interfaceParameter(current)
	}
	return r.finish(key, result)
}

// A call of a function's own func-typed parameter (the call's value is the
// parameter itself, so the function never rebinds it) calls what every
// static call of the function hands that parameter, as the Python and C
// adapters join a parameter's callers: the functions, closures and method
// values each call site passes there. A caller handing any other value, and
// a function reached otherwise than by its static calls (reachedOtherwise)
// or by none, leave the call open, each function handed a witness at the
// call handing it. Only the call through the parameter is joined: a
// parameter stored or handed on keeps its own frontier, so a register(h)
// storing h under a branch borrows no caller's handler.
//
// It is no case of functionValue, whose values it reads: a callable reaching
// a value through a parameter stays that value's frontier.
func (r *dynamicValueResolver) functionParameter(parameter *ssa.Parameter) dynamicValueSummary {
	unknown := dynamicValueSummary{unresolved: 1}
	parent := parameter.Parent()
	if r.analyzer == nil || parent == nil {
		return unknown
	}
	position := -1
	for index, candidate := range parent.Params {
		if candidate == parameter {
			position = index
		}
	}
	if position < 0 {
		return unknown
	}
	calls := r.analyzer.staticCallsTo(parent)
	open := len(calls) == 0 || r.analyzer.reachedOtherwise(parent)
	var handed []dynamicValueSummary
	var sites []godynamichandoff.Location
	for _, call := range calls {
		arguments := call.Common().Args
		if position >= len(arguments) {
			open = true
			continue
		}
		child := r.functionValue(arguments[position], true)
		open = open || child.unresolved > 0
		handed = append(handed, child)
		sites = append(sites, dynamicLocation(r.analyzer.location(call.Pos())))
	}
	result := dynamicValueSummary{}
	for i, child := range handed {
		if !open {
			r.merge(&result, child)
			continue
		}
		result.cyclic = result.cyclic || child.cyclic
		for function := range child.functions {
			result.witness(function, dynamicFieldWitness{location: sites[i], passedTo: parent})
		}
	}
	if open {
		result.unresolved = 1
	}
	return result
}

// An interface constructor parameter is resolved from the actual values at
// every SSA use of that exact repository function. Direct static calls supply
// their corresponding argument; any other use keeps the frontier open. This
// preserves distinct alternatives and never substitutes a merely compatible
// implementation by type name.
func (r *dynamicValueResolver) interfaceParameter(parameter *ssa.Parameter) dynamicValueSummary {
	unknown := dynamicValueSummary{unresolved: 1}
	if r.analyzer == nil || parameter == nil || parameter.Parent() == nil {
		return unknown
	}
	parent := parameter.Parent()
	position := -1
	for index, candidate := range parent.Params {
		if candidate == parameter {
			position = index
			break
		}
	}
	if position < 0 {
		return unknown
	}

	result := dynamicValueSummary{}
	found := false
	for _, call := range r.analyzer.staticCallsTo(parent) {
		found = true
		if position >= len(call.Common().Args) {
			r.merge(&result, unknown)
			continue
		}
		r.merge(&result, r.interfaceValue(call.Common().Args[position]))
	}
	if !found {
		return unknown
	}
	return result
}

func (r *dynamicValueResolver) interfaceField(value *ssa.UnOp) dynamicValueSummary {
	result := dynamicValueSummary{unresolved: 1}
	field, container := interfaceReceiverField(value)
	if field == nil || r.analyzer.dynamicHandoffCapture == nil {
		return result
	}
	stores := r.analyzer.dynamicHandoffCapture.interfaceFields[field]
	if len(stores) == 0 {
		return result
	}
	// Every store of this field in the analyzed program is observed. Their
	// values are the field's values; a store whose value cannot be followed
	// keeps its own unknown path.
	open := false
	for _, store := range stores {
		open = open || store.underBranch
	}
	result = dynamicValueSummary{}
	for _, store := range stores {
		child := r.interfaceValue(store.store.Val)
		location := dynamicLocation(r.analyzer.location(store.store.Pos()))
		if !open {
			r.merge(&result, child)
			for function := range child.functions {
				// Attach the store to this parent, never to the cached value.
				result.assign(function, location)
			}
			continue
		}
		// A store under a branch gives the field its value on some paths
		// only, and a parameter stored there joins every caller's argument,
		// whichever field the branch chose for it. The field's value is then
		// unknown: each stored callable is a witness, as in the C adapter.
		r.merge(&result, dynamicValueSummary{witnesses: child.witnesses, cyclic: child.cyclic})
		stored := dynamicFieldWitness{field: types.TypeString(container, packageQualifier) + "." + field.Name(),
			location: location, underBranch: store.underBranch}
		for function := range child.functions {
			result.witness(function, stored)
		}
	}
	if open {
		result.unresolved = 1
	}
	return result
}

// A factory return is a possible value, not an execution or an instantiation.
func (r *dynamicValueResolver) interfaceReturns(call *ssa.Call, index int) dynamicValueSummary {
	callee := call.Common().StaticCallee()
	if callee == nil || !r.analyzer.isRepositoryFunction(callee) || len(callee.Blocks) == 0 {
		return dynamicValueSummary{unresolved: 1}
	}
	result, found := dynamicValueSummary{}, false
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			found = true
			if index >= len(returned.Results) {
				r.merge(&result, dynamicValueSummary{unresolved: 1})
			} else {
				r.merge(&result, r.interfaceValue(returned.Results[index]))
			}
		}
	}
	if !found {
		result.unresolved = 1
	}
	return result
}

func addDynamicUnknown(count *int, delta int) error {
	if delta > int(^uint(0)>>1)-*count {
		return fmt.Errorf("surface discovery: Go dynamic handoff candidate count overflows int")
	}
	*count += delta
	return nil
}

func checkDynamicCandidateCount(candidates, unresolved int) error {
	return addDynamicUnknown(&unresolved, candidates)
}
