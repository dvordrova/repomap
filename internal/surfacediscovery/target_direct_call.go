package surfacediscovery

import (
	"sort"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// recordTargetDirectCallEdges builds the exact edge neighborhood only after
// the complete declaration catalog exists. This keeps every exact exported
// library root available while making graph depth and edge limits actually
// reduce the expensive target-reachable relation set.
func (a *analyzer) recordTargetDirectCallEdges() error {
	if a == nil || a.directCallIndex == nil || a.input.AnalysisTarget == nil ||
		a.directCallIndex.state != DirectCallIndexReady {
		return nil
	}
	roots := a.targetDirectCallRoots()
	if target := a.input.AnalysisTarget; target.Kind == AnalysisTargetExecutablePackage &&
		len(roots) != len(target.Roots) {
		return &AnalysisTargetSSAUnavailableError{
			Reason: AnalysisTargetExactRootsUnavailable, Package: target.PackagePath,
			ExpectedRoots: len(target.Roots), ResolvedRoots: len(roots),
		}
	}
	if len(roots) == 0 {
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
	for len(queue) > 0 && a.directCallIndex.state == DirectCallIndexReady {
		if a.ctx.Err() != nil {
			return nil
		}
		current := queue[0]
		queue = queue[1:]
		if current.function == nil || current.function.Blocks == nil {
			continue
		}
		calls := targetDirectCalls(a, current.function)
		if current.depth >= a.opts.DirectCallDepth {
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
			nextDepth := current.depth + 1
			if previous, found := distance[callee]; found && previous <= nextDepth {
				continue
			}
			distance[callee] = nextDepth
			queue = append(queue, queuedFunction{function: callee, depth: nextDepth})
		}
	}
	return nil
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
		if function.Blocks == nil || functionPackagePath(function) != target.PackagePath {
			continue
		}
		if _, duplicate := seenRoots[function]; duplicate {
			continue
		}
		switch target.Kind {
		case AnalysisTargetExecutablePackage:
			location := a.location(function.Pos())
			if function.Name() != "main" {
				continue
			}
			if _, found := rootLocations[targetDirectCallRootKey(location.Path, location.Line)]; !found {
				continue
			}
		case AnalysisTargetLibraryPackage:
			if !directCallFunctionExported(function) {
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
