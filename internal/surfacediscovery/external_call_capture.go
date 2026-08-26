package surfacediscovery

import (
	"fmt"
	"go/types"

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
	if sourceCallee == nil || a.isRepositoryFunction(sourceCallee) {
		return
	}
	caller, owner, callerOK, synthetic := a.externalCallCaller(call.Parent())
	target, targetOK := externalCallTarget(sourceCallee)
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
		Callsite: callsite,
	})
}

// observeExternalInterfaceInvoke retains the exact declared interface method
// exposed by SSA without guessing which concrete implementation runs. Invokes
// whose method identity is local or unavailable remain explicit frontiers.
func (a *analyzer) observeExternalInterfaceInvoke(call ssa.CallInstruction, common *ssa.CallCommon) {
	caller, owner, callerOK, synthetic := a.externalCallCaller(call.Parent())
	target, targetOK := externalInterfaceInvokeTarget(common)
	if !targetOK || entrySurfaceRepositoryPackage(a, target.PackagePath) {
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
	})
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
	if a == nil || packagePath == "" || !entrySurfaceRepositoryPackage(a, packagePath) {
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
