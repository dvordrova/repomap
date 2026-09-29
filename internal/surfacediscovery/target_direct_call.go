package surfacediscovery

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// recordTargetDirectCallEdges builds the exact edge neighborhood only after
// the complete declaration catalog exists. The ordinary zero-valued controls
// retain calls from every repository declaration in the loaded target scope;
// discovery of declarations is independent of runtime entrypoint reachability.
// Positive explicit depth
// and edge values remain opt-in narrowing controls.
func (a *analyzer) recordTargetDirectCallEdges() error {
	if a == nil || a.directCallIndex == nil || a.input.AnalysisTarget == nil ||
		a.directCallIndex.state != DirectCallIndexReady {
		return nil
	}
	if err := a.ctx.Err(); err != nil {
		return err
	}
	roots := a.targetDirectCallRoots()
	if target := a.input.AnalysisTarget; target.Kind == AnalysisTargetExecutablePackage &&
		len(roots) != len(target.Roots) {
		return &AnalysisTargetSSAUnavailableError{
			Reason: AnalysisTargetExactRootsUnavailable, Package: target.PackagePath,
			ExpectedRoots: len(target.Roots), ResolvedRoots: len(roots),
		}
	}
	if a.opts.DirectCallDepth == 0 && a.opts.DirectCallEdgeLimit == 0 {
		roots = nil
		seen := make(map[*ssa.Function]bool)
		for _, function := range a.orderedFunctions() {
			if function != nil && function.Origin() != nil {
				function = function.Origin()
			}
			if function == nil || function.Blocks == nil || !a.repositorySourceFunction(function) || seen[function] {
				continue
			}
			seen[function] = true
			roots = append(roots, function)
		}
	}
	initialized := a.recordInitializerCalls()
	if a.directCallIndex.state != DirectCallIndexReady {
		return nil
	}
	if len(roots) == 0 && len(initialized) == 0 {
		return nil
	}
	type queuedFunction struct {
		function *ssa.Function
		depth    int
	}
	queue := make([]queuedFunction, 0, len(roots))
	distance := make(map[*ssa.Function]int, len(roots))
	for _, root := range roots {
		distance[root] = 0
		queue = append(queue, queuedFunction{function: root})
	}
	// What a package-level variable's initializer calls runs when the
	// package loads, one call away from nothing the program declares.
	for _, callee := range initialized {
		if _, found := distance[callee]; !found {
			distance[callee] = 1
			queue = append(queue, queuedFunction{function: callee, depth: 1})
		}
	}
	for len(queue) > 0 && a.directCallIndex.state == DirectCallIndexReady {
		if err := a.ctx.Err(); err != nil {
			return err
		}
		current := queue[0]
		queue = queue[1:]
		if current.depth > a.directCallIndex.coverage.TraversalDepthReached {
			a.directCallIndex.coverage.TraversalDepthReached = current.depth
		}
		if current.function == nil || current.function.Blocks == nil {
			continue
		}
		calls := targetDirectCalls(a, current.function)
		if a.opts.DirectCallDepth > 0 && current.depth >= a.opts.DirectCallDepth {
			a.recordTargetDepthFrontier(current.function, calls)
			continue
		}
		for _, call := range calls {
			a.directCallIndex.recordCall(a, call)
			if a.directCallIndex.state != DirectCallIndexReady {
				if a.directCallIndex.closedReason == DirectCallIndexClosedEdgeLimit && current.depth > 0 {
					a.directCallIndex.coverage.EdgeLimitSafeDepth = current.depth
				}
				return nil
			}
			common := call.Common()
			if common == nil || common.IsInvoke() {
				continue
			}
			callee := common.StaticCallee()
			if callee == nil || callee.Blocks == nil || !a.repositoryDirectStaticCall(call, callee) {
				continue
			}
			if origin := callee.Origin(); origin != nil {
				callee = origin
			}
			nextDepth := current.depth + 1
			if previous, found := distance[callee]; found && previous <= nextDepth {
				continue
			}
			distance[callee] = nextDepth
			queue = append(queue, queuedFunction{function: callee, depth: nextDepth})
		}
		callerID, ok := a.directCallIndex.recordFunction(a, current.function)
		if !ok || a.directCallIndex.state != DirectCallIndexReady {
			continue
		}
		// The fields a body reads and writes are recorded where its calls
		// are: an explicit depth narrows both alike.
		a.recordFieldAccesses(current.function, callerID)
		for _, candidate := range a.callableBindings.exactCandidates(callerID) {
			if candidate != nil && candidate.Origin() != nil {
				candidate = candidate.Origin()
			}
			if candidate == nil || candidate.Blocks == nil || !a.repositorySourceFunction(candidate) {
				continue
			}
			nextDepth := current.depth + 1
			if previous, found := distance[candidate]; found && previous <= nextDepth {
				continue
			}
			distance[candidate] = nextDepth
			queue = append(queue, queuedFunction{function: candidate, depth: nextDepth})
		}
	}
	return a.ctx.Err()
}

func (a *analyzer) recordTargetDepthFrontier(caller *ssa.Function, calls []ssa.CallInstruction) {
	if a == nil || a.directCallIndex == nil || a.directCallIndex.state != DirectCallIndexReady {
		return
	}
	omitted := 0
	for _, call := range calls {
		common := call.Common()
		if common == nil || common.IsInvoke() {
			continue
		}
		callee := common.StaticCallee()
		if callee != nil && a.repositoryDirectStaticCall(call, callee) {
			omitted++
		}
	}
	if omitted == 0 {
		return
	}
	callerID, ok := a.directCallIndex.recordFunction(a, caller)
	if !ok || a.directCallIndex.state != DirectCallIndexReady {
		return
	}
	frontier := a.directCallIndex.frontiers[callerID]
	frontier.CallerID = callerID
	frontier.DepthBoundRepositoryCallsExcluded += omitted
	a.directCallIndex.frontiers[callerID] = frontier
	a.directCallIndex.coverage.DepthBoundRepositoryCallsExcluded += omitted
}

func (a *analyzer) targetDirectCallRoots() []*ssa.Function {
	if a == nil || a.input.AnalysisTarget == nil {
		return nil
	}
	target := a.input.AnalysisTarget
	targetPackages := make(map[string]struct{}, len(target.TargetPackages))
	for _, packagePath := range target.TargetPackages {
		targetPackages[packagePath] = struct{}{}
	}
	rootLocations := make(map[string]struct{}, len(target.Roots))
	for _, root := range target.Roots {
		rootLocations[targetDirectCallRootKey(root.Path, root.Line)] = struct{}{}
	}
	roots := make([]*ssa.Function, 0)
	seenRoots := make(map[*ssa.Function]struct{})
	for _, function := range a.orderedFunctions() {
		if function == nil {
			continue
		}
		if origin := function.Origin(); origin != nil {
			function = origin
		}
		if function.Blocks == nil {
			continue
		}
		packagePath := functionPackagePath(function)
		if _, duplicate := seenRoots[function]; duplicate {
			continue
		}
		switch target.Kind {
		case AnalysisTargetExecutablePackage:
			if packagePath != target.PackagePath {
				continue
			}
			location := a.location(function.Pos())
			if function.Name() != "main" {
				continue
			}
			if _, found := rootLocations[targetDirectCallRootKey(location.Path, location.Line)]; !found {
				continue
			}
		case AnalysisTargetModuleLibrary:
			if _, included := targetPackages[packagePath]; !included ||
				!directCallFunctionExported(function) || function.Package() == nil ||
				function.Package().Pkg == nil || function.Package().Pkg.Name() == "main" {
				continue
			}
		default:
			continue
		}
		seenRoots[function] = struct{}{}
		roots = append(roots, function)
	}
	sort.Slice(roots, func(i, j int) bool {
		left := a.location(roots[i].Pos())
		right := a.location(roots[j].Pos())
		if directCallLocationLess(left, right) {
			return true
		}
		if directCallLocationLess(right, left) {
			return false
		}
		return a.functionID(roots[i]) < a.functionID(roots[j])
	})
	return roots
}

func targetDirectCallRootKey(path string, line int) string {
	return path + "\x00" + strconv.Itoa(line)
}

func targetDirectCalls(a *analyzer, function *ssa.Function) []ssa.CallInstruction {
	result := make([]ssa.CallInstruction, 0)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(ssa.CallInstruction); ok {
				result = append(result, call)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := a.location(result[i].Pos())
		right := a.location(result[j].Pos())
		if directCallLocationLess(left, right) {
			return true
		}
		if directCallLocationLess(right, left) {
			return false
		}
		leftTarget := ""
		if common := result[i].Common(); common != nil && common.StaticCallee() != nil {
			leftTarget = a.functionID(common.StaticCallee())
		}
		rightTarget := ""
		if common := result[j].Common(); common != nil && common.StaticCallee() != nil {
			rightTarget = a.functionID(common.StaticCallee())
		}
		return leftTarget < rightTarget
	})
	return result
}

// initializerSpec is one value of a package-level variable specification:
// the source range of its initializer expression and the variable it
// initializes.
type initializerSpec struct {
	start, end token.Pos
	name       *ast.Ident
	spec       *ast.ValueSpec
}

// recordInitializerCalls records every exact repository call written in a
// package-level variable's initializer, with that variable as its caller
// (GO): Go evaluates those initializers in the package's synthetic
// initializer, which declares nothing, so the call had no caller the index
// could name. A call the synthetic initializer makes outside any variable's
// initializer (an init function, an imported package's initializer) is the
// runtime's own order, not code written there, and stays out. It returns
// the callees, in order.
func (a *analyzer) recordInitializerCalls() []*ssa.Function {
	var callees []*ssa.Function
	for _, function := range a.orderedFunctions() {
		if function == nil || function.Synthetic != "package initializer" || function.Blocks == nil || !a.isRepositoryFunction(function) {
			continue
		}
		packagePath := functionPackagePath(function)
		facts := a.packageFacts[packagePath]
		if facts == nil || facts.Module == nil || facts.Module.Path == "" || !a.modulePaths[facts.Module.Path] || facts.TypesInfo == nil {
			continue
		}
		moduleDirectory, ok := repositoryPackageModuleDirectory(a.root, facts)
		if !ok {
			continue
		}
		module := DirectCallModule{Path: facts.Module.Path, Directory: moduleDirectory}
		module.ID = stableDirectCallID("direct-module", module.Path, module.Directory)
		specs := initializerSpecs(facts.Syntax)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if !ok || call.Common() == nil || call.Common().IsInvoke() {
					continue
				}
				callee := call.Common().StaticCallee()
				if callee == nil || !a.repositoryDirectStaticCall(call, callee) {
					continue
				}
				spec, ok := specAt(specs, call.Pos())
				if !ok {
					continue
				}
				variable, ok := a.initializerVariable(packagePath, facts.TypesInfo, spec, module.ID)
				if !ok {
					continue
				}
				if a.directCallIndex.recordInitializerCall(a, call, variable, module) {
					if origin := callee.Origin(); origin != nil {
						callee = origin
					}
					callees = append(callees, callee)
				}
				if a.directCallIndex.state != DirectCallIndexReady {
					return callees
				}
			}
		}
	}
	return callees
}

// initializerSpecs lists the initializer values of every package-level
// variable of files. A specification of several names with one value (a
// call returning several results) gives it to its first name.
func initializerSpecs(files []*ast.File) []initializerSpec {
	var specs []initializerSpec
	for _, file := range files {
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, raw := range general.Specs {
				spec, ok := raw.(*ast.ValueSpec)
				if !ok || len(spec.Names) == 0 {
					continue
				}
				for i, value := range spec.Values {
					name := spec.Names[0]
					if len(spec.Values) == len(spec.Names) {
						name = spec.Names[i]
					}
					specs = append(specs, initializerSpec{start: value.Pos(), end: value.End(), name: name, spec: spec})
				}
			}
		}
	}
	return specs
}

// specAt is the initializer value whose source range holds position.
func specAt(specs []initializerSpec, position token.Pos) (initializerSpec, bool) {
	if !position.IsValid() {
		return initializerSpec{}, false
	}
	for _, spec := range specs {
		if spec.start <= position && position < spec.end {
			return spec, true
		}
	}
	return initializerSpec{}, false
}

// initializerVariable is the variable an initializer value initializes, as
// the index names a caller: its name, type, exported state and the source
// range of its whole specification.
func (a *analyzer) initializerVariable(packagePath string, info *types.Info, spec initializerSpec, moduleID string) (DirectCallVariable, bool) {
	declaration := a.location(spec.name.Pos())
	variable := DirectCallVariable{
		Symbol:  Symbol{ID: packagePath + "." + spec.name.Name, Package: packagePath, Name: spec.name.Name, Location: declaration},
		Package: packagePath, Exported: ast.IsExported(spec.name.Name), ModuleID: moduleID, ScenarioID: a.directCallIndex.scenario.ID,
		Declaration: declaration,
		Body:        DirectCallBodyRange{Start: a.location(spec.spec.Pos()), End: a.location(spec.spec.End())},
		CodeLines:   a.codeLines(spec.spec.Pos(), spec.spec.End()),
	}
	if object := info.Defs[spec.name]; object != nil {
		variable.Signature = types.TypeString(object.Type(), packageQualifier)
	}
	if !validRepositoryDirectCallLocation(declaration) || declaration.Column <= 0 || !validDirectCallBody(declaration, variable.Body) {
		return DirectCallVariable{}, false
	}
	variable.ID = stableDirectCallVariableID(variable)
	return variable, true
}
