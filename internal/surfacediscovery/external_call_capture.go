package surfacediscovery

import (
	"fmt"
	"go/constant"
	"go/types"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/ssa"
)

// newAnalyzerExternalCallIndexBuilder seals the exact repository package
// inventory already admitted to the current SSA program. It owns no package
// load and is independent from target roots and DirectCallIndex edge bounds.
func newAnalyzerExternalCallIndexBuilder(a *analyzer) (*ExternalCallIndexBuilder, error) {
	if a == nil || a.program == nil {
		return nil, fmt.Errorf("external call index: SSA program is unavailable")
	}
	modules := make(map[string]DirectCallModule)
	packages := make(map[string]ExternalCallPackage)
	for _, pkg := range a.packages {
		if pkg == nil || pkg.Pkg == nil {
			continue
		}
		owner, module, ok := a.externalCallPackage(pkg.Pkg.Path())
		if !ok {
			continue
		}
		modules[module.ID] = module
		packages[externalCallPackageKey(owner)] = owner
	}
	moduleRows := make([]DirectCallModule, 0, len(modules))
	for _, module := range modules {
		moduleRows = append(moduleRows, module)
	}
	packageRows := make([]ExternalCallPackage, 0, len(packages))
	for _, pkg := range packages {
		packageRows = append(packageRows, pkg)
	}
	return NewExternalCallIndexBuilder(a.scenario, moduleRows, packageRows)
}

// observeExternalCallIndex observes one instruction from prepare's existing
// program-wide pass. It never consults the selected target roots.
func (a *analyzer) observeExternalCallIndex(call ssa.CallInstruction) {
	if a == nil || a.externalCallIndex == nil || a.externalCallIndexErr != nil ||
		call == nil || call.Parent() == nil || !a.isRepositoryFunction(call.Parent()) {
		return
	}
	// Compiler-generated cgo wrapper bodies share the repository package path,
	// but they are neither repository callers nor source-owned graph nodes.
	// Their authored caller-to-wrapper boundary is observed on the caller below.
	if call.Parent().Syntax() != nil && !a.repositorySourceFunction(call.Parent()) {
		return
	}
	common := call.Common()
	if common == nil {
		a.addExternalCallExclusion(call.Parent(), ExternalCallExclusion{NonStaticCallsExcluded: 1})
		return
	}
	if _, builtin := common.Value.(*ssa.Builtin); builtin {
		return
	}
	if common.IsInvoke() {
		a.observeExternalInterfaceInvoke(call, common)
		return
	}
	callee := common.StaticCallee()
	if callee == nil {
		a.addExternalCallExclusion(call.Parent(), ExternalCallExclusion{NonStaticCallsExcluded: 1})
		return
	}
	sourceCallee := externalCallCanonicalFunction(callee)
	if sourceCallee == nil {
		return
	}
	target, cgoBoundary := a.generatedCgoExternalTarget(sourceCallee)
	if !cgoBoundary && a.isRepositoryFunction(sourceCallee) {
		return
	}
	caller, owner, callerOK, synthetic := a.externalCallCaller(call.Parent())
	targetOK := cgoBoundary
	if !targetOK {
		target, targetOK = externalCallTarget(sourceCallee)
	}
	if !targetOK {
		if callerOK {
			a.addExternalCallExclusionNode(caller, ExternalCallExclusion{
				UnnamedStaticCalleesExcluded: 1,
			})
		}
		return
	}
	if !callerOK {
		if owner.PackagePath != "" {
			a.externalCallIndexErr = a.externalCallIndex.addPackageExclusion(owner, synthetic)
		}
		return
	}
	callsite := a.location(call.Pos())
	if !validRepositoryDirectCallLocation(callsite) {
		a.addExternalCallExclusionNode(caller, ExternalCallExclusion{
			InvalidCallsitesExcluded: 1,
		})
		return
	}
	a.externalCallIndexErr = a.externalCallIndex.AddWitness(ExternalCallWitness{
		Caller: caller, Target: target, Dispatch: ExternalCallStatic, Invocation: directCallInvocation(call),
		Callsite: callsite, Pattern: a.externalCallPattern(common, callsite),
	})
}

// generatedCgoExternalTarget restores the compiler-proved handoff to a cgo
// wrapper without promoting that generated wrapper into repository source
// authority or claiming execution beyond the wrapper boundary.
func (a *analyzer) generatedCgoExternalTarget(function *ssa.Function) (ExternalCallTarget, bool) {
	function = externalCallCanonicalFunction(function)
	if function == nil || !a.isRepositoryFunction(function) || a.repositorySourceFunction(function) {
		return ExternalCallTarget{}, false
	}
	name := strings.TrimPrefix(function.Name(), "_Cfunc_")
	if name == function.Name() || name == "" {
		return ExternalCallTarget{}, false
	}
	target := ExternalCallTarget{PackagePath: ExternalCallCgoPackagePath, Name: name}
	return target, validExternalCallTarget(target)
}

// observeExternalInterfaceInvoke retains the exact declared interface method
// exposed by SSA without guessing which concrete implementation runs. Invokes
// whose method identity is local or unavailable remain explicit frontiers.
func (a *analyzer) observeExternalInterfaceInvoke(call ssa.CallInstruction, common *ssa.CallCommon) {
	caller, owner, callerOK, synthetic := a.externalCallCaller(call.Parent())
	target, targetOK := externalInterfaceInvokeTarget(common)
	if !targetOK || admittedRepositoryPackage(a, target.PackagePath) {
		if callerOK {
			a.addExternalCallExclusionNode(caller, ExternalCallExclusion{DynamicInvokesExcluded: 1})
		}
		return
	}
	if !callerOK {
		if owner.PackagePath != "" {
			a.externalCallIndexErr = a.externalCallIndex.addPackageExclusion(owner, synthetic)
		}
		return
	}
	callsite := a.location(call.Pos())
	if !validRepositoryDirectCallLocation(callsite) {
		a.addExternalCallExclusionNode(caller, ExternalCallExclusion{InvalidCallsitesExcluded: 1})
		return
	}
	a.externalCallIndexErr = a.externalCallIndex.AddWitness(ExternalCallWitness{
		Caller: caller, Target: target, Dispatch: ExternalCallInterfaceInvoke,
		Invocation: directCallInvocation(call), Callsite: callsite,
		Pattern: a.externalCallPattern(common, callsite),
	})
}

// externalCallPattern projects only language syntax and compiler-resolved
// callable values already present on the SSA instruction. It neither knows nor
// classifies frameworks, protocols, routes, handlers, or bootstrap semantics.
func (a *analyzer) externalCallPattern(
	common *ssa.CallCommon,
	callsite Location,
) *ExternalCallPattern {
	if a == nil || common == nil || !validRepositoryDirectCallLocation(callsite) {
		return nil
	}
	arguments, receiver := externalCallSourceArguments(common)
	observed := len(arguments)
	pattern := &ExternalCallPattern{
		ID: externalCallPatternID(callsite), Callsite: callsite,
		ReceiverResultIDs: []string{},
		Arguments:         make([]ExternalCallPatternArgument, 0, len(arguments)),
		ArgumentsObserved: observed,
	}
	if resultID, resultType, ok := a.externalCallResult(common, callsite); ok {
		pattern.ResultID = resultID
		pattern.ResultType = resultType
	}
	if receiver != nil {
		resultIDs, unresolved := a.externalCallReceiverResults(receiver, make(map[ssa.Value]bool))
		sort.Strings(resultIDs)
		resultIDs = compactStrings(resultIDs)
		// Retain every exact call-result origin and account for each remaining
		// open value-flow frontier separately.
		pattern.ReceiverResultIDs = resultIDs
		pattern.ReceiversObserved = len(resultIDs) + unresolved
		pattern.ReceiversOmitted = unresolved
	}
	for position, argument := range arguments {
		pattern.Arguments = append(pattern.Arguments, a.externalCallPatternArgument(position+1, argument))
	}
	return pattern
}

// externalCallSourceArguments restores source-level variadic elements when
// SSA materialized a complete fixed array for them. It otherwise keeps the
// aggregate slice as one dynamic argument. This is generic syntax recovery;
// it does not inspect package, selector, framework, or protocol names.
func externalCallSourceArguments(common *ssa.CallCommon) ([]ssa.Value, ssa.Value) {
	if common == nil {
		return nil, nil
	}
	arguments := append([]ssa.Value(nil), common.Args...)
	var receiver ssa.Value
	if common.IsInvoke() {
		receiver = common.Value
	} else if callee := common.StaticCallee(); callee != nil && callee.Signature != nil && callee.Signature.Recv() != nil {
		if len(arguments) == 0 {
			return nil, nil
		}
		receiver = arguments[0]
		arguments = arguments[1:]
	}
	if common.Signature() == nil || !common.Signature().Variadic() || len(arguments) == 0 {
		return arguments, receiver
	}
	if values, ok := externalCallVariadicValues(arguments[len(arguments)-1]); ok {
		arguments = append(arguments[:len(arguments)-1], values...)
	}
	return arguments, receiver
}

func externalCallVariadicValues(value ssa.Value) ([]ssa.Value, bool) {
	for {
		switch current := value.(type) {
		case *ssa.ChangeType:
			value = current.X
		case *ssa.Convert:
			value = current.X
		case *ssa.Slice:
			allocation, ok := current.X.(*ssa.Alloc)
			if !ok {
				return nil, false
			}
			pointer, ok := allocation.Type().Underlying().(*types.Pointer)
			if !ok {
				return nil, false
			}
			array, ok := pointer.Elem().Underlying().(*types.Array)
			if !ok || array.Len() < 0 {
				return nil, false
			}
			length := array.Len()
			valuesByIndex := make(map[int64]ssa.Value)
			referrers := allocation.Referrers()
			if referrers == nil {
				return nil, length == 0
			}
			for _, reference := range *referrers {
				address, addressOK := reference.(*ssa.IndexAddr)
				if !addressOK {
					if _, sliceOK := reference.(*ssa.Slice); sliceOK {
						continue
					}
					return nil, false
				}
				index, indexOK := externalCallConstantIndex(address.Index)
				if !indexOK || index < 0 || index >= length {
					return nil, false
				}
				addressRefs := address.Referrers()
				if addressRefs == nil {
					return nil, false
				}
				var stored ssa.Value
				for _, addressRef := range *addressRefs {
					store, storeOK := addressRef.(*ssa.Store)
					if !storeOK || store.Addr != address || stored != nil {
						return nil, false
					}
					stored = store.Val
				}
				if stored == nil {
					return nil, false
				}
				if _, duplicate := valuesByIndex[index]; duplicate {
					return nil, false
				}
				valuesByIndex[index] = stored
			}
			if int64(len(valuesByIndex)) != length || uint64(length) > uint64(^uint(0)>>1) {
				return nil, false
			}
			values := make([]ssa.Value, int(length))
			for index, stored := range valuesByIndex {
				values[int(index)] = stored
			}
			return values, true
		default:
			return nil, false
		}
	}
}

func externalCallConstantIndex(value ssa.Value) (int64, bool) {
	constantValue, ok := value.(*ssa.Const)
	if !ok || constantValue.Value == nil || constantValue.Value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Int64Val(constantValue.Value)
	if !exact || integer < 0 {
		return 0, false
	}
	return integer, true
}

func (a *analyzer) externalCallResult(
	common *ssa.CallCommon,
	callsite Location,
) (string, string, bool) {
	if a == nil || common == nil || !validRepositoryDirectCallLocation(callsite) || common.Signature() == nil ||
		common.Signature().Results() == nil || common.Signature().Results().Len() == 0 {
		return "", "", false
	}
	identity := externalCallCommonIdentity(a, common)
	if identity == "" {
		return "", "", false
	}
	resultType := types.TypeString(common.Signature().Results(), packageQualifier)
	if !utf8.ValidString(resultType) {
		resultType = ""
	}
	return stableDirectCallID("call-result", externalCallPatternID(callsite), identity), resultType, true
}

func externalCallCommonIdentity(a *analyzer, common *ssa.CallCommon) string {
	if common == nil {
		return ""
	}
	if callee := externalCallCanonicalFunction(common.StaticCallee()); callee != nil {
		if object := callee.Object(); object != nil && object.Pkg() != nil {
			return object.Pkg().Path() + "\x00" + receiverName(callee.Signature) + "\x00" + object.Name()
		}
		if a != nil {
			return a.functionID(callee)
		}
	}
	if common.Method != nil && common.Method.Pkg() != nil {
		return common.Method.Pkg().Path() + "\x00" + common.Method.Name()
	}
	return ""
}

func (a *analyzer) externalCallReceiverResults(
	value ssa.Value,
	active map[ssa.Value]bool,
) ([]string, int) {
	if a == nil || value == nil || active[value] {
		return nil, 1
	}
	active[value] = true
	defer delete(active, value)
	switch current := value.(type) {
	case *ssa.Call:
		callsite := a.location(current.Pos())
		resultID, _, ok := a.externalCallResult(current.Common(), callsite)
		if !ok {
			return nil, 1
		}
		return []string{resultID}, 0
	case *ssa.ChangeType:
		return a.externalCallReceiverResults(current.X, active)
	case *ssa.Convert:
		return a.externalCallReceiverResults(current.X, active)
	case *ssa.MakeInterface:
		return a.externalCallReceiverResults(current.X, active)
	case *ssa.ChangeInterface:
		return a.externalCallReceiverResults(current.X, active)
	case *ssa.Phi:
		result := make([]string, 0, len(current.Edges))
		if len(current.Edges) == 0 {
			return nil, 1
		}
		unresolved := 0
		for _, edge := range current.Edges {
			values, edgeUnresolved := a.externalCallReceiverResults(edge, active)
			result = append(result, values...)
			unresolved += edgeUnresolved
		}
		return result, unresolved
	default:
		// A parameter, field, allocation, or other non-call value has no
		// call-result provenance; that is a complete empty answer, not an
		// unresolved call-result candidate.
		return nil, 0
	}
}

func (a *analyzer) externalCallPatternArgument(
	position int,
	value ssa.Value,
) ExternalCallPatternArgument {
	argument := ExternalCallPatternArgument{
		Position: position, Kind: ExternalCallPatternDynamic, ObjectIDs: []string{},
	}
	if literal, ok := externalCallPatternString(value, make(map[ssa.Value]bool)); ok && utf8.ValidString(literal) {
		argument.Kind = ExternalCallPatternLiteralString
		argument.Value = literal
	}
	if !externalCallPatternMayBeCallable(value, make(map[ssa.Value]bool)) {
		return argument
	}
	candidates, unresolved := dynamicFunctionCandidateFacts(a, value)
	objectIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.candidate.FunctionID != "" {
			objectIDs = append(objectIDs, candidate.candidate.FunctionID)
		}
	}
	sort.Strings(objectIDs)
	objectIDs = compactStrings(objectIDs)
	// Known candidates remain exact local facts even when other branches are
	// open. Open branches contribute omitted frontiers, not omissions of the
	// known IDs.
	argument.ObjectsObserved = len(objectIDs) + unresolved
	argument.ObjectIDs = objectIDs
	argument.ObjectsOmitted = unresolved
	return argument
}

func externalCallPatternString(value ssa.Value, active map[ssa.Value]bool) (string, bool) {
	if value == nil || active[value] {
		return "", false
	}
	active[value] = true
	defer delete(active, value)
	switch current := value.(type) {
	case *ssa.Const:
		if current.Value != nil && current.Value.Kind() == constant.String {
			return constant.StringVal(current.Value), true
		}
	case *ssa.ChangeType:
		return externalCallPatternString(current.X, active)
	case *ssa.Convert:
		return externalCallPatternString(current.X, active)
	case *ssa.MakeInterface:
		return externalCallPatternString(current.X, active)
	case *ssa.ChangeInterface:
		return externalCallPatternString(current.X, active)
	}
	return "", false
}

func externalCallPatternMayBeCallable(value ssa.Value, active map[ssa.Value]bool) bool {
	if value == nil || active[value] {
		return false
	}
	active[value] = true
	defer delete(active, value)
	if _, ok := dynamicCallableSignature(value.Type()); ok {
		return true
	}
	switch current := value.(type) {
	case *ssa.ChangeType:
		return externalCallPatternMayBeCallable(current.X, active)
	case *ssa.Convert:
		return externalCallPatternMayBeCallable(current.X, active)
	case *ssa.MakeInterface:
		return externalCallPatternMayBeCallable(current.X, active)
	case *ssa.ChangeInterface:
		return externalCallPatternMayBeCallable(current.X, active)
	case *ssa.Phi:
		for _, edge := range current.Edges {
			if externalCallPatternMayBeCallable(edge, active) {
				return true
			}
		}
	}
	return false
}

func (a *analyzer) addExternalCallExclusion(
	function *ssa.Function,
	exclusion ExternalCallExclusion,
) {
	caller, _, ok, _ := a.externalCallCaller(function)
	if !ok {
		return
	}
	a.addExternalCallExclusionNode(caller, exclusion)
}

func (a *analyzer) addExternalCallExclusionNode(
	caller DirectCallNode,
	exclusion ExternalCallExclusion,
) {
	if a == nil || a.externalCallIndex == nil || a.externalCallIndexErr != nil {
		return
	}
	exclusion.Caller = caller
	a.externalCallIndexErr = a.externalCallIndex.AddExclusion(exclusion)
}

func (a *analyzer) externalCallCaller(
	function *ssa.Function,
) (DirectCallNode, ExternalCallPackage, bool, bool) {
	if a == nil || function == nil {
		return DirectCallNode{}, ExternalCallPackage{}, false, false
	}
	source := externalCallCanonicalFunction(function)
	if source == nil {
		return DirectCallNode{}, ExternalCallPackage{}, false, false
	}
	owner, _, ownerOK := a.externalCallPackage(functionPackagePath(source))
	// DirectCallIndex deliberately excludes synthetic SSA functions from its
	// exact caller authority. Keep the external-call producer on the same
	// boundary so a wrapper or nested synthetic body cannot cite a caller that
	// the shared ProgramIndex will never own.
	if source.Synthetic != "" {
		return DirectCallNode{}, owner, false, ownerOK
	}
	node, _, nodeOK := a.directCallNode(source, a.scenario)
	return node, owner, nodeOK, false
}

func (a *analyzer) externalCallPackage(
	packagePath string,
) (ExternalCallPackage, DirectCallModule, bool) {
	if a == nil || packagePath == "" || !admittedRepositoryPackage(a, packagePath) {
		return ExternalCallPackage{}, DirectCallModule{}, false
	}
	facts := a.packageFacts[packagePath]
	if facts == nil || facts.Module == nil || facts.Module.Path == "" ||
		!a.modulePaths[facts.Module.Path] {
		return ExternalCallPackage{}, DirectCallModule{}, false
	}
	directory, ok := repositoryPackageModuleDirectory(a.root, facts)
	if !ok {
		return ExternalCallPackage{}, DirectCallModule{}, false
	}
	module := DirectCallModule{Path: facts.Module.Path, Directory: directory}
	module.ID = stableDirectCallID("direct-module", module.Path, module.Directory)
	return ExternalCallPackage{ModuleID: module.ID, PackagePath: packagePath}, module, true
}

func admittedRepositoryPackage(a *analyzer, packagePath string) bool {
	return a != nil && packagePath != "" && a.admittedPackages[packagePath]
}

func externalCallCanonicalFunction(function *ssa.Function) *ssa.Function {
	if function == nil {
		return nil
	}
	if origin := function.Origin(); origin != nil {
		return origin
	}
	return function
}

func externalCallTarget(function *ssa.Function) (ExternalCallTarget, bool) {
	function = externalCallCanonicalFunction(function)
	if function == nil {
		return ExternalCallTarget{}, false
	}
	object := function.Object()
	if object == nil || object.Pkg() == nil {
		return ExternalCallTarget{}, false
	}
	target := ExternalCallTarget{
		PackagePath: object.Pkg().Path(), Receiver: receiverName(function.Signature),
		Name: object.Name(),
	}
	return target, validExternalCallTarget(target)
}

func externalInterfaceInvokeTarget(common *ssa.CallCommon) (ExternalCallTarget, bool) {
	if common == nil || !common.IsInvoke() || common.Method == nil || common.Method.Pkg() == nil {
		return ExternalCallTarget{}, false
	}
	signature, _ := common.Method.Type().(*types.Signature)
	target := ExternalCallTarget{
		PackagePath: common.Method.Pkg().Path(), Receiver: receiverName(signature),
		Name: common.Method.Name(),
	}
	return target, validExternalCallTarget(target)
}
