package surfacediscovery

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

// dynamicHandoffCapture observes the existing SSA instruction walk. It owns
// no package load and performs no call-graph construction of its own.
type dynamicHandoffCapture struct {
	enabled           bool
	handoffs          []godynamichandoff.Handoff
	callableBindings  []callableBindingFact
	callbackTraversal map[string][]*ssa.Function
	bindingsFrozen    bool
	coverage          godynamichandoff.CoverageInput
	err               error
}

// callableBindingFact is one immutable post-SSA fact shared by the bounded
// direct-call traversal and the dynamic-handoff seal. The handoff records the
// exact structural assignment. exactCandidate is populated only for a
// complete single-candidate value flow and therefore cannot turn an
// alternative or unresolved binding into traversal authority.
type callableBindingFact struct {
	handoff        godynamichandoff.Handoff
	exactCandidate *ssa.Function
}

type callableBindingSnapshot struct {
	facts    []callableBindingFact
	byCaller map[string][]*ssa.Function
}

func (snapshot callableBindingSnapshot) exactCandidates(callerID string) []*ssa.Function {
	return append([]*ssa.Function(nil), snapshot.byCaller[callerID]...)
}

func (a *analyzer) observeDynamicHandoffs(call ssa.CallInstruction) {
	if a == nil || a.dynamicHandoffCapture == nil || a.dynamicHandoffCapture.err != nil ||
		call == nil || call.Common() == nil {
		return
	}
	capture := a.dynamicHandoffCapture
	common := call.Common()
	callbackArguments := dynamicCallbackArguments(common)
	shapes := len(callbackArguments)
	if capture.enabled {
		if common.IsInvoke() {
			shapes++
		} else if common.StaticCallee() == nil {
			if _, builtin := common.Value.(*ssa.Builtin); !builtin {
				shapes++
			}
		}
	}
	if shapes == 0 || call.Parent() == nil || call.Parent().Synthetic != "" ||
		!a.isRepositoryFunction(call.Parent()) {
		return
	}
	callerID, callerOK := a.directCallIndex.recordFunction(a, call.Parent())
	if !callerOK {
		capture.coverage.UnsupportedCallers += shapes
		return
	}
	callsite := a.location(call.Pos())
	if !validRepositoryDirectCallLocation(callsite) {
		capture.coverage.InvalidCallsites += shapes
		return
	}

	if capture.enabled {
		if common.IsInvoke() {
			capture.observeInterfaceInvoke(a, call, callerID, callsite)
		} else if common.StaticCallee() == nil {
			if _, builtin := common.Value.(*ssa.Builtin); !builtin {
				capture.observeFunctionValueCall(a, call, callerID, callsite)
			}
		}
	}

	if len(callbackArguments) == 0 {
		return
	}
	staticTarget, ok := dynamicCallbackStaticTarget(a, common)
	if !ok && capture.enabled {
		capture.coverage.UnsupportedStaticTargets += len(callbackArguments)
	}
	for _, argument := range callbackArguments {
		capture.observeCallbackTransfer(a, call, callerID, callsite, staticTarget, ok, argument)
	}
}

func (capture *dynamicHandoffCapture) observeCallableBinding(a *analyzer, store *ssa.Store) {
	if capture == nil || capture.err != nil || capture.bindingsFrozen || a == nil || store == nil ||
		store.Parent() == nil || store.Parent().Synthetic != "" || !a.isRepositoryFunction(store.Parent()) {
		return
	}
	fieldAddress, ok := store.Addr.(*ssa.FieldAddr)
	if !ok || fieldAddress.X == nil || store.Val == nil {
		return
	}
	containerPointer, ok := fieldAddress.X.Type().Underlying().(*types.Pointer)
	if !ok || containerPointer.Elem() == nil {
		return
	}
	container, ok := containerPointer.Elem().Underlying().(*types.Struct)
	if !ok || fieldAddress.Field < 0 || fieldAddress.Field >= container.NumFields() {
		return
	}
	field := container.Field(fieldAddress.Field)
	if field == nil || !dynamicCallableFieldBinding(field.Type(), store.Val, make(map[ssa.Value]bool)) {
		return
	}

	candidateFacts, unresolved := dynamicFunctionCandidateFacts(a, store.Val)
	if len(candidateFacts) == 0 {
		if _, callable := dynamicCallableSignature(store.Val.Type()); !callable {
			return
		}
	}
	candidates := make([]godynamichandoff.Candidate, 0, len(candidateFacts))
	for _, candidate := range candidateFacts {
		candidates = append(candidates, candidate.candidate)
	}
	callerID, ok := a.directCallIndex.recordFunction(a, store.Parent())
	if !ok {
		capture.coverage.UnsupportedCallers++
		return
	}
	callsite := a.location(store.Pos())
	if !validRepositoryDirectCallLocation(callsite) || callsite.Column <= 0 {
		capture.coverage.InvalidCallsites++
		return
	}
	resolution := dynamicResolution(candidates, unresolved)
	candidatesConsidered := dynamicCandidatesConsidered(candidates, unresolved)
	var exactCandidate *ssa.Function
	if resolution == godynamichandoff.ResolutionExact {
		exactCandidate = candidateFacts[0].function
	} else if resolution == godynamichandoff.ResolutionUnresolved {
		candidates = []godynamichandoff.Candidate{}
	}
	signature := dynamicFunctionValueSignature(store.Val, make(map[ssa.Value]bool))
	capture.callableBindings = append(capture.callableBindings, callableBindingFact{
		handoff: godynamichandoff.Handoff{
			Kind:       godynamichandoff.CallableBinding,
			CallerID:   callerID,
			Invocation: godynamichandoff.InvocationBinding,
			Callsite:   dynamicLocation(callsite),
			Slot: godynamichandoff.Slot{
				ContainerType: types.TypeString(containerPointer.Elem(), packageQualifier),
				DeclaredType:  types.TypeString(field.Type(), packageQualifier),
				Field:         field.Name(),
				Signature:     signature,
			},
			Resolution:           resolution,
			Candidates:           candidates,
			CandidatesConsidered: candidatesConsidered,
		},
		exactCandidate: exactCandidate,
	})
}

func dynamicCallableFieldBinding(
	declaredType types.Type,
	value ssa.Value,
	active map[ssa.Value]bool,
) bool {
	if _, callable := dynamicCallableSignature(declaredType); callable {
		return true
	}
	if declaredType == nil || value == nil || active[value] {
		return false
	}
	declaredInterface, ok := types.Unalias(declaredType).Underlying().(*types.Interface)
	if !ok {
		return false
	}
	declaredInterface = declaredInterface.Complete()
	if declaredInterface.NumMethods() == 0 {
		return false
	}
	active[value] = true
	defer delete(active, value)
	switch current := value.(type) {
	case *ssa.MakeInterface:
		if current.X == nil || current.X.Type() == nil {
			return false
		}
		if _, callable := dynamicCallableSignature(current.X.Type()); !callable {
			return false
		}
		return types.Implements(current.X.Type(), declaredInterface)
	case *ssa.ChangeInterface:
		return dynamicCallableFieldBinding(declaredType, current.X, active)
	case *ssa.Phi:
		if len(current.Edges) == 0 {
			return false
		}
		for _, edge := range current.Edges {
			if !dynamicCallableFieldBinding(declaredType, edge, active) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (capture *dynamicHandoffCapture) freezeCallableBindings() callableBindingSnapshot {
	if capture == nil {
		return callableBindingSnapshot{facts: []callableBindingFact{}, byCaller: map[string][]*ssa.Function{}}
	}
	capture.bindingsFrozen = true
	facts := make([]callableBindingFact, len(capture.callableBindings))
	for position, fact := range capture.callableBindings {
		fact.handoff.Candidates = append([]godynamichandoff.Candidate(nil), fact.handoff.Candidates...)
		facts[position] = fact
	}
	sort.Slice(facts, func(i, j int) bool {
		left, right := facts[i].handoff, facts[j].handoff
		if left.CallerID != right.CallerID {
			return left.CallerID < right.CallerID
		}
		if left.Callsite != right.Callsite {
			if left.Callsite.Path != right.Callsite.Path {
				return left.Callsite.Path < right.Callsite.Path
			}
			if left.Callsite.Line != right.Callsite.Line {
				return left.Callsite.Line < right.Callsite.Line
			}
			return left.Callsite.Column < right.Callsite.Column
		}
		if left.Slot.ContainerType != right.Slot.ContainerType {
			return left.Slot.ContainerType < right.Slot.ContainerType
		}
		if left.Slot.Field != right.Slot.Field {
			return left.Slot.Field < right.Slot.Field
		}
		return callableBindingCandidateKey(left) < callableBindingCandidateKey(right)
	})
	snapshot := callableBindingSnapshot{
		facts: facts, byCaller: make(map[string][]*ssa.Function),
	}
	for _, fact := range facts {
		if fact.handoff.Resolution == godynamichandoff.ResolutionExact && fact.exactCandidate != nil {
			snapshot.byCaller[fact.handoff.CallerID] = append(
				snapshot.byCaller[fact.handoff.CallerID], fact.exactCandidate,
			)
		}
	}
	for callerID, candidates := range capture.callbackTraversal {
		for _, candidate := range candidates {
			if candidate == nil {
				continue
			}
			snapshot.byCaller[callerID] = append(snapshot.byCaller[callerID], candidate)
		}
	}
	for callerID, candidates := range snapshot.byCaller {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].String() < candidates[j].String()
		})
		compacted := candidates[:0]
		for _, candidate := range candidates {
			if len(compacted) == 0 || compacted[len(compacted)-1] != candidate {
				compacted = append(compacted, candidate)
			}
		}
		snapshot.byCaller[callerID] = compacted
	}
	return snapshot
}

func callableBindingCandidateKey(handoff godynamichandoff.Handoff) string {
	var result strings.Builder
	result.WriteString(string(handoff.Resolution))
	for _, candidate := range handoff.Candidates {
		result.WriteByte(0)
		result.WriteString(candidate.FunctionID)
		result.WriteByte(0)
		result.WriteString(string(candidate.Evidence))
	}
	return result.String()
}

func (capture *dynamicHandoffCapture) observeInterfaceInvoke(
	a *analyzer,
	call ssa.CallInstruction,
	callerID string,
	callsite Location,
) {
	common := call.Common()
	if common.Method == nil || common.Value == nil || common.Signature() == nil {
		capture.err = fmt.Errorf("surface discovery: Go dynamic handoff: malformed SSA interface invoke")
		return
	}
	candidates, unresolved := dynamicInterfaceCandidates(a, common.Value, common.Method)
	candidatesConsidered := dynamicCandidatesConsidered(candidates, unresolved)
	resolution := dynamicResolution(candidates, unresolved)
	if resolution == godynamichandoff.ResolutionUnresolved {
		candidates = []godynamichandoff.Candidate{}
	}
	capture.append(godynamichandoff.Handoff{
		Kind:       godynamichandoff.InterfaceInvoke,
		CallerID:   callerID,
		Invocation: dynamicInvocation(call),
		Callsite:   dynamicLocation(callsite),
		Slot: godynamichandoff.Slot{
			DeclaredType: types.TypeString(common.Value.Type(), packageQualifier),
			Method:       common.Method.Name(),
			Signature:    types.TypeString(common.Signature(), packageQualifier),
		},
		Resolution:           resolution,
		Candidates:           candidates,
		CandidatesConsidered: candidatesConsidered,
	})
}

func (capture *dynamicHandoffCapture) observeFunctionValueCall(
	a *analyzer,
	call ssa.CallInstruction,
	callerID string,
	callsite Location,
) {
	common := call.Common()
	candidates, unresolved := dynamicFunctionCandidates(a, common.Value)
	candidatesConsidered := dynamicCandidatesConsidered(candidates, unresolved)
	resolution := dynamicResolution(candidates, unresolved)
	if resolution == godynamichandoff.ResolutionUnresolved {
		candidates = []godynamichandoff.Candidate{}
	}
	capture.append(godynamichandoff.Handoff{
		Kind:       godynamichandoff.FunctionValueCall,
		CallerID:   callerID,
		Invocation: dynamicInvocation(call),
		Callsite:   dynamicLocation(callsite),
		Slot: godynamichandoff.Slot{
			Signature: types.TypeString(common.Signature(), packageQualifier),
		},
		Resolution:           resolution,
		Candidates:           candidates,
		CandidatesConsidered: candidatesConsidered,
	})
}

type dynamicCallbackArgument struct {
	value     ssa.Value
	parameter int
	signature string
}

func (capture *dynamicHandoffCapture) observeCallbackTransfer(
	a *analyzer,
	call ssa.CallInstruction,
	callerID string,
	callsite Location,
	staticTarget godynamichandoff.StaticTarget,
	staticTargetOK bool,
	argument dynamicCallbackArgument,
) {
	candidateFacts, unresolved := dynamicFunctionCandidateFacts(a, argument.value)
	candidates := make([]godynamichandoff.Candidate, 0, len(candidateFacts))
	for _, candidate := range candidateFacts {
		candidates = append(candidates, candidate.candidate)
	}
	candidatesConsidered := dynamicCandidatesConsidered(candidates, unresolved)
	resolution := dynamicResolution(candidates, unresolved)
	if resolution == godynamichandoff.ResolutionExact && len(candidateFacts) == 1 {
		if capture.callbackTraversal == nil {
			capture.callbackTraversal = make(map[string][]*ssa.Function)
		}
		capture.callbackTraversal[callerID] = append(capture.callbackTraversal[callerID], candidateFacts[0].function)
	}
	if resolution == godynamichandoff.ResolutionUnresolved {
		candidates = []godynamichandoff.Candidate{}
	}
	if !capture.enabled || !staticTargetOK {
		return
	}
	capture.append(godynamichandoff.Handoff{
		Kind:         godynamichandoff.CallbackTransfer,
		CallerID:     callerID,
		Invocation:   dynamicInvocation(call),
		Callsite:     dynamicLocation(callsite),
		StaticTarget: staticTarget,
		Slot: godynamichandoff.Slot{
			Signature: argument.signature,
			Parameter: argument.parameter,
		},
		Resolution:           resolution,
		Candidates:           candidates,
		CandidatesConsidered: candidatesConsidered,
	})
}

func (capture *dynamicHandoffCapture) append(handoff godynamichandoff.Handoff) {
	if capture.err != nil {
		return
	}
	capture.handoffs = append(capture.handoffs, handoff)
}

func (capture *dynamicHandoffCapture) finish(
	direct DirectCallIndex,
	bindings callableBindingSnapshot,
) (godynamichandoff.Index, error) {
	if capture == nil {
		return godynamichandoff.Index{}, fmt.Errorf("surface discovery: Go dynamic handoff capture is unavailable")
	}
	if capture.err != nil {
		return godynamichandoff.Index{}, capture.err
	}
	handoffs := append([]godynamichandoff.Handoff(nil), capture.handoffs...)
	for _, fact := range bindings.facts {
		handoff := fact.handoff
		handoff.Candidates = append([]godynamichandoff.Candidate(nil), fact.handoff.Candidates...)
		handoffs = append(handoffs, handoff)
	}
	coverage := capture.coverage
	if direct.State != DirectCallIndexReady {
		coverage.UnsupportedCallers += len(handoffs)
		handoffs = nil
	}
	functions := make([]godynamichandoff.Function, 0, len(direct.Nodes))
	for _, node := range direct.Nodes {
		functions = append(functions, godynamichandoff.Function{
			ID: node.ID, Package: node.Package, Symbol: node.Symbol.ID,
			Location: dynamicLocation(node.Declaration),
		})
	}
	index, err := godynamichandoff.New(godynamichandoff.Input{
		Scenario: godynamichandoff.Scenario{
			ID: direct.Scenario.ID, GOOS: direct.Scenario.GOOS, GOARCH: direct.Scenario.GOARCH,
			Tags: append([]string(nil), direct.Scenario.Tags...),
		},
		SourceDirectCallSHA256: direct.SHA256,
		Functions:              functions,
		Handoffs:               handoffs,
		Coverage:               coverage,
	})
	if err != nil {
		return godynamichandoff.Index{}, fmt.Errorf("surface discovery: seal Go dynamic handoff index: %w", err)
	}
	return index, nil
}

func dynamicHandoffShapeCount(common *ssa.CallCommon) int {
	if common == nil {
		return 0
	}
	count := 0
	if common.IsInvoke() {
		count++
	} else if common.StaticCallee() == nil {
		if _, builtin := common.Value.(*ssa.Builtin); !builtin {
			count++
		}
	}
	count += len(dynamicCallbackArguments(common))
	return count
}

func dynamicCallbackArguments(common *ssa.CallCommon) []dynamicCallbackArgument {
	if common == nil || common.Signature() == nil {
		return nil
	}
	if _, builtin := common.Value.(*ssa.Builtin); builtin {
		return nil
	}
	receiverOffset := 0
	// Call-mode method Args include the receiver at position zero. Invoke-mode
	// Args do not: common.Value is the receiver and Args starts with parameter
	// one. Keep the declared parameter number stable across both forms.
	if !common.IsInvoke() && common.Signature().Recv() != nil {
		receiverOffset = 1
	}
	result := make([]dynamicCallbackArgument, 0)
	for index, argument := range common.Args {
		if argument == nil || index < receiverOffset {
			continue
		}
		signature, ok := dynamicCallableSignature(argument.Type())
		if !ok {
			continue
		}
		result = append(result, dynamicCallbackArgument{
			value: argument, parameter: index - receiverOffset + 1,
			signature: types.TypeString(signature, packageQualifier),
		})
	}
	return result
}

func dynamicCallbackStaticTarget(
	a *analyzer,
	common *ssa.CallCommon,
) (godynamichandoff.StaticTarget, bool) {
	if common == nil {
		return godynamichandoff.StaticTarget{}, false
	}
	if common.IsInvoke() {
		target, ok := externalInterfaceInvokeTarget(common)
		if !ok {
			return godynamichandoff.StaticTarget{}, false
		}
		return godynamichandoff.StaticTarget{
			Package: target.PackagePath, Receiver: target.Receiver, Name: target.Name,
		}, true
	}
	return dynamicStaticTarget(a, common.StaticCallee())
}

func dynamicCallableSignature(value types.Type) (*types.Signature, bool) {
	if value == nil {
		return nil, false
	}
	value = types.Unalias(value)
	signature, ok := value.Underlying().(*types.Signature)
	return signature, ok && signature != nil
}

func dynamicStaticTarget(a *analyzer, callee *ssa.Function) (godynamichandoff.StaticTarget, bool) {
	callee = externalCallCanonicalFunction(callee)
	if callee == nil {
		return godynamichandoff.StaticTarget{}, false
	}
	if a.isRepositoryFunction(callee) {
		if functionID, ok := a.directCallIndex.recordFunction(a, callee); ok {
			return godynamichandoff.StaticTarget{FunctionID: functionID}, true
		}
	}
	target, ok := externalCallTarget(callee)
	if !ok {
		return godynamichandoff.StaticTarget{}, false
	}
	return godynamichandoff.StaticTarget{
		Package: target.PackagePath, Receiver: target.Receiver, Name: target.Name,
	}, true
}

func dynamicInterfaceCandidates(
	a *analyzer,
	value ssa.Value,
	method *types.Func,
) ([]godynamichandoff.Candidate, int) {
	resolved := make(map[*ssa.Function]struct{})
	unresolved := resolveDynamicInterfaceValue(a, value, method, resolved, make(map[ssa.Value]bool))
	functionIDs := make(map[string]struct{}, len(resolved))
	for function := range resolved {
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
	}
	evidence := godynamichandoff.EvidenceConcreteInterfaceValue
	if len(functionIDs) > 1 {
		evidence = godynamichandoff.EvidenceInterfaceValueAlternative
	}
	candidates := make([]godynamichandoff.Candidate, 0, len(functionIDs))
	for functionID := range functionIDs {
		candidates = append(candidates, godynamichandoff.Candidate{
			FunctionID: functionID,
			Evidence:   evidence,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FunctionID < candidates[j].FunctionID
	})
	return candidates, unresolved
}

func resolveDynamicInterfaceValue(
	a *analyzer,
	value ssa.Value,
	method *types.Func,
	resolved map[*ssa.Function]struct{},
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
		return resolveDynamicInterfaceValue(a, current.X, method, resolved, active)
	case *ssa.Phi:
		if len(current.Edges) == 0 {
			return 1
		}
		unresolved := 0
		for _, edge := range current.Edges {
			unresolved += resolveDynamicInterfaceValue(a, edge, method, resolved, active)
		}
		return unresolved
	default:
		return 1
	}
}

func dynamicFunctionCandidates(
	a *analyzer,
	value ssa.Value,
) ([]godynamichandoff.Candidate, int) {
	facts, unresolved := dynamicFunctionCandidateFacts(a, value)
	candidates := make([]godynamichandoff.Candidate, 0, len(facts))
	for _, fact := range facts {
		candidates = append(candidates, fact.candidate)
	}
	return candidates, unresolved
}

type dynamicFunctionCandidateFact struct {
	function  *ssa.Function
	candidate godynamichandoff.Candidate
}

func dynamicFunctionCandidateFacts(
	a *analyzer,
	value ssa.Value,
) ([]dynamicFunctionCandidateFact, int) {
	resolved := make(map[*ssa.Function]godynamichandoff.CandidateEvidence)
	unresolved := resolveDynamicFunctionValue(value, resolved, make(map[ssa.Value]bool), false)
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

func dynamicCandidateEvidenceRank(value godynamichandoff.CandidateEvidence) int {
	switch value {
	case godynamichandoff.EvidenceDirectFunctionValue:
		return 0
	case godynamichandoff.EvidenceClosureValue:
		return 1
	case godynamichandoff.EvidenceUniqueValueFlow:
		return 2
	case godynamichandoff.EvidenceValueFlowAlternative:
		return 3
	default:
		return 99
	}
}

func resolveDynamicFunctionValue(
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
			unresolved += resolveDynamicFunctionValue(edge, resolved, active, true)
		}
		return unresolved
	case *ssa.ChangeType:
		return resolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.Convert:
		return resolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.MakeInterface:
		return resolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.ChangeInterface:
		return resolveDynamicFunctionValue(current.X, resolved, active, true)
	default:
		return 1
	}
}

func dynamicFunctionValueSignature(value ssa.Value, active map[ssa.Value]bool) string {
	if value == nil || active[value] {
		return ""
	}
	active[value] = true
	defer delete(active, value)
	if signature, ok := dynamicCallableSignature(value.Type()); ok {
		return types.TypeString(signature, packageQualifier)
	}
	switch current := value.(type) {
	case *ssa.MakeInterface:
		return dynamicFunctionValueSignature(current.X, active)
	case *ssa.ChangeInterface:
		return dynamicFunctionValueSignature(current.X, active)
	case *ssa.ChangeType:
		return dynamicFunctionValueSignature(current.X, active)
	case *ssa.Convert:
		return dynamicFunctionValueSignature(current.X, active)
	case *ssa.Phi:
		result := ""
		for _, edge := range current.Edges {
			candidate := dynamicFunctionValueSignature(edge, active)
			if candidate == "" {
				return ""
			}
			if result != "" && result != candidate {
				return ""
			}
			result = candidate
		}
		return result
	default:
		return ""
	}
}

func dynamicResolution(
	candidates []godynamichandoff.Candidate,
	unresolved int,
) godynamichandoff.Resolution {
	if len(candidates) == 0 {
		return godynamichandoff.ResolutionUnresolved
	}
	if unresolved == 0 && len(candidates) == 1 {
		return godynamichandoff.ResolutionExact
	}
	return godynamichandoff.ResolutionAlternatives
}

func dynamicCandidatesConsidered(
	candidates []godynamichandoff.Candidate,
	unresolved int,
) int {
	return len(candidates) + unresolved
}

func dynamicInvocation(call ssa.CallInstruction) godynamichandoff.Invocation {
	switch directCallInvocation(call) {
	case DirectCallGoroutine:
		return godynamichandoff.InvocationGoroutine
	case DirectCallDeferred:
		return godynamichandoff.InvocationDeferred
	default:
		return godynamichandoff.InvocationSynchronous
	}
}

func dynamicLocation(value Location) godynamichandoff.Location {
	return godynamichandoff.Location{Path: value.Path, Line: value.Line, Column: value.Column}
}
