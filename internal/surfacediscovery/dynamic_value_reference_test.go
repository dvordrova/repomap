package surfacediscovery

// Original path-local walkers retained only as a bounded equivalence oracle.
import (
	"go/types"
	"sort"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

func referenceDynamicInterfaceCandidates(
	a *analyzer,
	value ssa.Value,
	method *types.Func,
) ([]godynamichandoff.Candidate, int) {
	resolved := make(map[*ssa.Function]struct{})
	assignments := make(map[*ssa.Function][]godynamichandoff.Location)
	unresolved := referenceResolveDynamicInterfaceValue(a, value, method, resolved, assignments, make(map[ssa.Value]bool))
	functionIDs := make(map[string]struct{}, len(resolved))
	assignmentsByID := make(map[string][]godynamichandoff.Location)
	for function := range resolved {
		locations := assignments[function]
		function = externalCallCanonicalFunction(function)
		if function == nil || !a.isRepositoryFunction(function) {
			unresolved++
			continue
		}
		functionID, ok := a.directCallIndex.recordFunction(a, function)
		if !ok {
			unresolved++
			continue
		}
		functionIDs[functionID] = struct{}{}
		assignmentsByID[functionID] = append(assignmentsByID[functionID], locations...)
	}
	evidence := godynamichandoff.EvidenceConcreteInterfaceValue
	if len(functionIDs) > 1 {
		evidence = godynamichandoff.EvidenceInterfaceValueAlternative
	}
	candidates := make([]godynamichandoff.Candidate, 0, len(functionIDs))
	for functionID := range functionIDs {
		candidateEvidence := evidence
		if len(assignmentsByID[functionID]) > 0 {
			candidateEvidence = godynamichandoff.EvidenceInterfaceFieldAssignment
		}
		candidates = append(candidates, godynamichandoff.Candidate{
			FunctionID:  functionID,
			Evidence:    candidateEvidence,
			Assignments: assignmentsByID[functionID],
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FunctionID < candidates[j].FunctionID
	})
	return candidates, unresolved
}

func referenceResolveDynamicInterfaceValue(
	a *analyzer,
	value ssa.Value,
	method *types.Func,
	resolved map[*ssa.Function]struct{},
	assignments map[*ssa.Function][]godynamichandoff.Location,
	active map[ssa.Value]bool,
) int {
	if a == nil || a.program == nil || value == nil || method == nil || active[value] {
		return 1
	}
	active[value] = true
	defer delete(active, value)
	switch current := value.(type) {
	case *ssa.MakeInterface:
		if current.X == nil || current.X.Type() == nil {
			return 1
		}
		implementation := a.program.LookupMethod(current.X.Type(), method.Pkg(), method.Name())
		if implementation == nil {
			return 1
		}
		resolved[implementation] = struct{}{}
		return 0
	case *ssa.ChangeInterface:
		return referenceResolveDynamicInterfaceValue(a, current.X, method, resolved, assignments, active)
	case *ssa.Phi:
		if len(current.Edges) == 0 {
			return 1
		}
		unresolved := 0
		for _, edge := range current.Edges {
			unresolved += referenceResolveDynamicInterfaceValue(a, edge, method, resolved, assignments, active)
		}
		return unresolved
	case *ssa.UnOp:
		return referenceResolveInterfaceField(a, current, method, resolved, assignments, active)
	case *ssa.Call:
		return referenceResolveInterfaceReturns(a, current, 0, method, resolved, assignments, active)
	case *ssa.Extract:
		if call, ok := current.Tuple.(*ssa.Call); ok {
			return referenceResolveInterfaceReturns(a, call, current.Index, method, resolved, assignments, active)
		}
		return 1
	default:
		return 1
	}
}

func referenceDynamicFunctionCandidates(
	a *analyzer,
	value ssa.Value,
) ([]godynamichandoff.Candidate, int) {
	facts, unresolved := referenceDynamicFunctionCandidateFacts(a, value)
	candidates := make([]godynamichandoff.Candidate, 0, len(facts))
	for _, fact := range facts {
		candidates = append(candidates, fact.candidate)
	}
	return candidates, unresolved
}

func referenceDynamicFunctionCandidateFacts(
	a *analyzer,
	value ssa.Value,
) ([]dynamicFunctionCandidateFact, int) {
	resolved := make(map[*ssa.Function]godynamichandoff.CandidateEvidence)
	unresolved := referenceResolveDynamicFunctionValue(value, resolved, make(map[ssa.Value]bool), false)
	if len(resolved) == 0 {
		return []dynamicFunctionCandidateFact{}, unresolved
	}
	byFunctionID := make(map[string]dynamicFunctionCandidateFact, len(resolved))
	for function, evidence := range resolved {
		function = externalCallCanonicalFunction(function)
		if function == nil || !a.isRepositoryFunction(function) {
			unresolved++
			continue
		}
		functionID, ok := a.directCallIndex.recordFunction(a, function)
		if !ok {
			unresolved++
			continue
		}
		candidate := dynamicFunctionCandidateFact{
			function: function,
			candidate: godynamichandoff.Candidate{
				FunctionID: functionID, Evidence: evidence,
			},
		}
		if previous, exists := byFunctionID[functionID]; !exists ||
			dynamicCandidateEvidenceRank(evidence) < dynamicCandidateEvidenceRank(previous.candidate.Evidence) {
			byFunctionID[functionID] = candidate
		}
	}
	flowEvidence := godynamichandoff.EvidenceUniqueValueFlow
	if len(byFunctionID) > 1 {
		flowEvidence = godynamichandoff.EvidenceValueFlowAlternative
	}
	for functionID, candidate := range byFunctionID {
		if candidate.candidate.Evidence == godynamichandoff.EvidenceUniqueValueFlow ||
			candidate.candidate.Evidence == godynamichandoff.EvidenceValueFlowAlternative {
			candidate.candidate.Evidence = flowEvidence
			byFunctionID[functionID] = candidate
		}
	}
	candidates := make([]dynamicFunctionCandidateFact, 0, len(byFunctionID))
	for _, candidate := range byFunctionID {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].candidate.FunctionID < candidates[j].candidate.FunctionID
	})
	return candidates, unresolved
}

func referenceResolveDynamicFunctionValue(
	value ssa.Value,
	resolved map[*ssa.Function]godynamichandoff.CandidateEvidence,
	active map[ssa.Value]bool,
	throughFlow bool,
) int {
	if value == nil || active[value] {
		return 1
	}
	active[value] = true
	defer delete(active, value)
	switch current := value.(type) {
	case *ssa.Function:
		evidence := godynamichandoff.EvidenceDirectFunctionValue
		if throughFlow {
			evidence = godynamichandoff.EvidenceUniqueValueFlow
		}
		resolved[current] = evidence
		return 0
	case *ssa.MakeClosure:
		function, ok := current.Fn.(*ssa.Function)
		if !ok {
			return 1
		}
		evidence := godynamichandoff.EvidenceClosureValue
		if throughFlow {
			evidence = godynamichandoff.EvidenceUniqueValueFlow
		}
		resolved[function] = evidence
		return 0
	case *ssa.Phi:
		if len(current.Edges) == 0 {
			return 1
		}
		unresolved := 0
		for _, edge := range current.Edges {
			unresolved += referenceResolveDynamicFunctionValue(edge, resolved, active, true)
		}
		return unresolved
	case *ssa.ChangeType:
		return referenceResolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.Convert:
		return referenceResolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.MakeInterface:
		return referenceResolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.ChangeInterface:
		return referenceResolveDynamicFunctionValue(current.X, resolved, active, true)
	default:
		return 1
	}
}

func referenceResolveInterfaceField(a *analyzer, value *ssa.UnOp, method *types.Func, resolved map[*ssa.Function]struct{}, assignments map[*ssa.Function][]godynamichandoff.Location, active map[ssa.Value]bool) int {
	field, _ := interfaceReceiverField(value)
	if field == nil || a.dynamicHandoffCapture == nil {
		return 1
	}
	// A known store is not proof that this instance was created there. The
	// remaining receiver is deliberately unresolved, even for one observed type.
	unresolved := 1
	for _, store := range a.dynamicHandoffCapture.interfaceFields[field] {
		candidates := make(map[*ssa.Function]struct{})
		origins := make(map[*ssa.Function][]godynamichandoff.Location)
		unresolved += referenceResolveDynamicInterfaceValue(a, store.Val, method, candidates, origins, active)
		location := dynamicLocation(a.location(store.Pos()))
		for function := range candidates {
			resolved[function] = struct{}{}
			assignments[function] = append(assignments[function], origins[function]...)
			assignments[function] = append(assignments[function], location)
		}
	}
	return unresolved
}

// A factory's concrete return expression identifies a possible value. Interface
// parameters, external factories and recursive unresolved returns stay open.
// This does not instantiate or execute the factory.

func referenceResolveInterfaceReturns(a *analyzer, call *ssa.Call, result int, method *types.Func, resolved map[*ssa.Function]struct{}, assignments map[*ssa.Function][]godynamichandoff.Location, active map[ssa.Value]bool) int {
	callee := call.Common().StaticCallee()
	if callee == nil || !a.isRepositoryFunction(callee) || len(callee.Blocks) == 0 {
		return 1
	}
	unresolved, returns := 0, 0
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			returns++
			if result >= len(returned.Results) {
				unresolved++
				continue
			}
			unresolved += referenceResolveDynamicInterfaceValue(a, returned.Results[result], method, resolved, assignments, active)
		}
	}
	if returns == 0 {
		unresolved++
	}
	return unresolved
}
