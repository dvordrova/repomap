package surfacediscovery

import (
	"fmt"
	"go/types"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

// A summary is immutable after return. Unknown frontiers count paths, while
// functions and assignment locations are the same sets sealed by the index.
type dynamicValueSummary struct {
	functions   map[*ssa.Function]godynamichandoff.CandidateEvidence
	assignments map[*ssa.Function]map[godynamichandoff.Location]struct{}
	unresolved  int
	cyclic      bool
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

func resolveDynamicFunctionValue(value ssa.Value) (dynamicValueSummary, error) {
	r := newDynamicValueResolver(nil, nil)
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
		result = dynamicValueSummary{functions: map[*ssa.Function]godynamichandoff.CandidateEvidence{current: evidence}}
	case *ssa.MakeClosure:
		if function, ok := current.Fn.(*ssa.Function); ok {
			evidence := godynamichandoff.EvidenceClosureValue
			if throughFlow {
				evidence = godynamichandoff.EvidenceUniqueValueFlow
			}
			result = dynamicValueSummary{functions: map[*ssa.Function]godynamichandoff.CandidateEvidence{function: evidence}}
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
	}
	return r.finish(key, result)
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
	}
	return r.finish(key, result)
}

func (r *dynamicValueResolver) interfaceField(value *ssa.UnOp) dynamicValueSummary {
	result := dynamicValueSummary{unresolved: 1}
	field, _ := interfaceReceiverField(value)
	if field == nil || r.analyzer.dynamicHandoffCapture == nil {
		return result
	}
	// Stores remain possible alternatives; none closes this instance's frontier.
	for _, store := range r.analyzer.dynamicHandoffCapture.interfaceFields[field] {
		child := r.interfaceValue(store.Val)
		r.merge(&result, child)
		for function := range child.functions {
			// Attach the store to this parent, never to the cached value.
			result.assign(function, dynamicLocation(r.analyzer.location(store.Pos())))
		}
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
