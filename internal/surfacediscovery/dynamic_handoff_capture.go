package surfacediscovery

import (
	"fmt"
	"go/types"
	"sort"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

// dynamicHandoffCapture observes the existing SSA instruction walk. It owns
// no package load and performs no call-graph construction of its own.
type dynamicHandoffCapture struct {
	handoffs []godynamichandoff.Handoff
	coverage godynamichandoff.CoverageInput
	err      error
}

func (a *analyzer) observeDynamicHandoffs(call ssa.CallInstruction) {
	if a == nil || a.dynamicHandoffCapture == nil || a.dynamicHandoffCapture.err != nil ||
		call == nil || call.Common() == nil {
		return
	}
	capture := a.dynamicHandoffCapture
	common := call.Common()
	shapes := dynamicHandoffShapeCount(common)
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

	if common.IsInvoke() {
		capture.observeInterfaceInvoke(a, call, callerID, callsite)
	} else if common.StaticCallee() == nil {
		if _, builtin := common.Value.(*ssa.Builtin); !builtin {
			capture.observeFunctionValueCall(a, call, callerID, callsite)
		}
	}

	callbackArguments := dynamicCallbackArguments(common)
	if len(callbackArguments) == 0 {
		return
	}
	staticTarget, ok := dynamicCallbackStaticTarget(a, common)
	if !ok {
		capture.coverage.UnsupportedStaticTargets += len(callbackArguments)
		return
	}
	for _, argument := range callbackArguments {
		capture.observeCallbackTransfer(a, call, callerID, callsite, staticTarget, argument)
	}
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
	candidates, complete := dynamicInterfaceCandidates(a, common.Value, common.Method)
	if !capture.acceptCandidateCount(candidates) {
		return
	}
	candidatesConsidered := len(candidates)
	resolution := dynamicResolution(candidates, complete)
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
	candidates, complete := dynamicFunctionCandidates(a, common.Value)
	if !capture.acceptCandidateCount(candidates) {
		return
	}
	candidatesConsidered := len(candidates)
	resolution := dynamicResolution(candidates, complete)
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
	argument dynamicCallbackArgument,
) {
	candidates, complete := dynamicFunctionCandidates(a, argument.value)
	if !capture.acceptCandidateCount(candidates) {
		return
	}
	candidatesConsidered := len(candidates)
	resolution := dynamicResolution(candidates, complete)
	if resolution == godynamichandoff.ResolutionUnresolved {
		candidates = []godynamichandoff.Candidate{}
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
	if len(capture.handoffs) >= godynamichandoff.MaxHandoffs {
		capture.err = fmt.Errorf("surface discovery: Go dynamic handoff: handoff limit exceeded")
		return
	}
	if len(handoff.Candidates) > godynamichandoff.MaxCandidatesPerHandoff {
		capture.err = fmt.Errorf("surface discovery: Go dynamic handoff: candidate limit exceeded")
		return
	}
	capture.handoffs = append(capture.handoffs, handoff)
}

func (capture *dynamicHandoffCapture) acceptCandidateCount(
	candidates []godynamichandoff.Candidate,
) bool {
	if len(candidates) <= godynamichandoff.MaxCandidatesPerHandoff {
		return true
	}
	capture.err = fmt.Errorf("surface discovery: Go dynamic handoff: candidate limit exceeded")
	return false
}

func (capture *dynamicHandoffCapture) finish(
	direct DirectCallIndex,
) (godynamichandoff.Index, error) {
	if capture == nil {
		return godynamichandoff.Index{}, fmt.Errorf("surface discovery: Go dynamic handoff capture is unavailable")
	}
	if capture.err != nil {
		return godynamichandoff.Index{}, capture.err
	}
	handoffs := capture.handoffs
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
) ([]godynamichandoff.Candidate, bool) {
	resolved := make(map[*ssa.Function]struct{})
	complete := resolveDynamicInterfaceValue(a, value, method, resolved, make(map[ssa.Value]bool))
	candidates := make([]godynamichandoff.Candidate, 0, len(resolved))
	evidence := godynamichandoff.EvidenceConcreteInterfaceValue
	if len(resolved) > 1 {
		evidence = godynamichandoff.EvidenceInterfaceValueAlternative
	}
	for function := range resolved {
		function = externalCallCanonicalFunction(function)
		if function == nil || !a.isRepositoryFunction(function) {
			complete = false
			continue
		}
		functionID, ok := a.directCallIndex.recordFunction(a, function)
		if !ok {
			complete = false
			continue
		}
		candidates = append(candidates, godynamichandoff.Candidate{
			FunctionID: functionID,
			Evidence:   evidence,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FunctionID < candidates[j].FunctionID
	})
	return candidates, complete
}

func resolveDynamicInterfaceValue(
	a *analyzer,
	value ssa.Value,
	method *types.Func,
	resolved map[*ssa.Function]struct{},
	active map[ssa.Value]bool,
) bool {
	if a == nil || a.program == nil || value == nil || method == nil || active[value] {
		return false
	}
	active[value] = true
	defer delete(active, value)
	switch current := value.(type) {
	case *ssa.MakeInterface:
		if current.X == nil || current.X.Type() == nil {
			return false
		}
		implementation := a.program.LookupMethod(current.X.Type(), method.Pkg(), method.Name())
		if implementation == nil {
			return false
		}
		resolved[implementation] = struct{}{}
		return true
	case *ssa.ChangeInterface:
		return resolveDynamicInterfaceValue(a, current.X, method, resolved, active)
	case *ssa.Phi:
		if len(current.Edges) == 0 {
			return false
		}
		complete := true
		for _, edge := range current.Edges {
			complete = resolveDynamicInterfaceValue(a, edge, method, resolved, active) && complete
		}
		return complete
	default:
		return false
	}
}

func dynamicFunctionCandidates(
	a *analyzer,
	value ssa.Value,
) ([]godynamichandoff.Candidate, bool) {
	resolved := make(map[*ssa.Function]godynamichandoff.CandidateEvidence)
	complete := resolveDynamicFunctionValue(value, resolved, make(map[ssa.Value]bool), false)
	if len(resolved) == 0 {
		return []godynamichandoff.Candidate{}, complete
	}
	flowEvidence := godynamichandoff.EvidenceUniqueValueFlow
	if len(resolved) > 1 {
		flowEvidence = godynamichandoff.EvidenceValueFlowAlternative
	}
	candidates := make([]godynamichandoff.Candidate, 0, len(resolved))
	for function, evidence := range resolved {
		function = externalCallCanonicalFunction(function)
		if function == nil || !a.isRepositoryFunction(function) {
			complete = false
			continue
		}
		functionID, ok := a.directCallIndex.recordFunction(a, function)
		if !ok {
			complete = false
			continue
		}
		if evidence == godynamichandoff.EvidenceUniqueValueFlow ||
			evidence == godynamichandoff.EvidenceValueFlowAlternative {
			evidence = flowEvidence
		}
		candidates = append(candidates, godynamichandoff.Candidate{
			FunctionID: functionID, Evidence: evidence,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FunctionID < candidates[j].FunctionID
	})
	return candidates, complete
}

func resolveDynamicFunctionValue(
	value ssa.Value,
	resolved map[*ssa.Function]godynamichandoff.CandidateEvidence,
	active map[ssa.Value]bool,
	throughFlow bool,
) bool {
	if value == nil || active[value] {
		return false
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
		return true
	case *ssa.MakeClosure:
		function, ok := current.Fn.(*ssa.Function)
		if !ok {
			return false
		}
		evidence := godynamichandoff.EvidenceClosureValue
		if throughFlow {
			evidence = godynamichandoff.EvidenceUniqueValueFlow
		}
		resolved[function] = evidence
		return true
	case *ssa.Phi:
		if len(current.Edges) == 0 {
			return false
		}
		complete := true
		for _, edge := range current.Edges {
			complete = resolveDynamicFunctionValue(edge, resolved, active, true) && complete
		}
		return complete
	case *ssa.ChangeType:
		return resolveDynamicFunctionValue(current.X, resolved, active, true)
	case *ssa.Convert:
		return resolveDynamicFunctionValue(current.X, resolved, active, true)
	default:
		return false
	}
}

func dynamicResolution(
	candidates []godynamichandoff.Candidate,
	complete bool,
) godynamichandoff.Resolution {
	if !complete || len(candidates) == 0 {
		return godynamichandoff.ResolutionUnresolved
	}
	if len(candidates) == 1 {
		return godynamichandoff.ResolutionExact
	}
	return godynamichandoff.ResolutionAlternatives
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
