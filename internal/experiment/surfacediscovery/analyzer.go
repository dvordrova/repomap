package surfacediscovery

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"go/version"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/semantics/catalog"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type analyzer struct {
	ctx                       context.Context
	opts                      Options
	input                     Input
	processEntrypoints        []processEntrypoint
	catalog                   catalog.Catalog
	program                   *ssa.Program
	packages                  []*ssa.Package
	packageFacts              map[string]*packages.Package
	graph                     *callgraph.Graph
	allFunctions              map[*ssa.Function]bool
	relevant                  map[*ssa.Function]bool
	relevanceDistance         map[*ssa.Function]int
	callTargets               map[ssa.CallInstruction][]*ssa.Function
	root                      string
	modulePath                string
	scenario                  Scenario
	result                    Result
	tasks                     int
	active                    map[*ssa.Function]bool
	matchedSeeds              []string
	starts                    []dispatchStart
	assignments               map[string]Value
	valuesByAddress           map[string]Value
	summaryByID               map[string]SemanticSummary
	fileDigests               map[string]SourceDigest
	functionByID              map[string]*ssa.Function
	loopCache                 map[*ssa.Function][]loopDescriptor
	loopSeen                  map[string]bool
	compositionVisited        map[*ssa.Function]bool
	architectureAnchors       map[string]BehaviorAnchor
	architectureRelationships map[string]BehaviorRelationship
	walkedFunctions           map[*ssa.Function]bool
	callbackReferences        map[*ssa.Function]bool
	callbackReferenceIDs      map[string]bool
	entrypointPackages        map[*ssa.Function]map[string]bool
	detachedWalk              bool
}

const (
	defaultMaxDepth   = 16
	defaultMaxTasks   = 1500
	defaultMaxTargets = 8
)

type environment map[ssa.Value]Value

type dispatchStart struct {
	seed       catalog.Seed
	values     map[string]Value
	entrypoint *ssa.Function
	chain      []Wrapper
	location   Location
	ambiguous  bool
	frontiers  []Frontier
	matched    bool
	detached   bool
}

func Analyze(opts Options) (Result, error) {
	return AnalyzeContextWithInput(context.Background(), opts, Input{})
}

func AnalyzeContext(ctx context.Context, opts Options) (Result, error) {
	return AnalyzeContextWithInput(ctx, opts, Input{})
}

func AnalyzeWithInput(opts Options, input Input) (Result, error) {
	return AnalyzeContextWithInput(context.Background(), opts, input)
}

func AnalyzeContextWithInput(ctx context.Context, opts Options, input Input) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("surface discovery: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	started := time.Now()
	if strings.TrimSpace(opts.RepoPath) == "" {
		return Result{}, fmt.Errorf("surface discovery: repository path is required")
	}
	opts = normalizeOptions(opts)
	root, err := filepath.Abs(opts.RepoPath)
	if err != nil {
		return Result{}, fmt.Errorf("surface discovery: resolve repository: %w", err)
	}
	input, processEntrypoints := normalizeInput(root, input)
	builtin, err := catalog.Builtin()
	if err != nil {
		return Result{}, err
	}
	a := &analyzer{
		ctx:                       ctx,
		opts:                      opts,
		input:                     input,
		processEntrypoints:        processEntrypoints,
		catalog:                   builtin,
		root:                      root,
		active:                    map[*ssa.Function]bool{},
		assignments:               map[string]Value{},
		valuesByAddress:           map[string]Value{},
		summaryByID:               map[string]SemanticSummary{},
		fileDigests:               map[string]SourceDigest{},
		packageFacts:              map[string]*packages.Package{},
		callTargets:               map[ssa.CallInstruction][]*ssa.Function{},
		functionByID:              map[string]*ssa.Function{},
		loopCache:                 map[*ssa.Function][]loopDescriptor{},
		loopSeen:                  map[string]bool{},
		compositionVisited:        map[*ssa.Function]bool{},
		architectureAnchors:       map[string]BehaviorAnchor{},
		architectureRelationships: map[string]BehaviorRelationship{},
		walkedFunctions:           map[*ssa.Function]bool{},
		callbackReferences:        map[*ssa.Function]bool{},
		callbackReferenceIDs:      map[string]bool{},
		entrypointPackages:        map[*ssa.Function]map[string]bool{},
		scenario: Scenario{
			ID:   scenarioID(runtime.GOOS, runtime.GOARCH, opts.BuildTags),
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			Tags: append([]string{}, opts.BuildTags...),
		},
	}
	if err := a.load(); err != nil {
		return Result{}, err
	}
	a.recordProcessEntrypoints()
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	if a.program != nil {
		a.prepare()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	if a.program != nil {
		a.recordGlobalArchitectureAnchors()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	entrypoints := a.entrypoints()
	for _, entrypoint := range entrypoints {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("surface discovery: %w", err)
		}
		a.result.Coverage.EntrypointsConsidered = append(
			a.result.Coverage.EntrypointsConsidered,
			a.symbol(entrypoint),
		)
		a.entrypointPackages[entrypoint] = importedPackagePaths(entrypoint)
		entryAnchorID := a.recordArchitectureAnchor(
			"process_entry",
			"process entry "+a.functionID(entrypoint),
			a.location(entrypoint.Pos()),
			a.symbol(entrypoint),
			"Exact build-selected main declaration; process execution is not observed.",
		)
		a.walk(entrypoint, environment{}, nil, entrypoint, 0, false, entryAnchorID)
	}
	a.walkDisconnectedRelevant(entrypoints)
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	a.finish(time.Since(started))
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	a.finishArchitectureGrounding(entrypoints)
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	return a.result, nil
}

func normalizeOptions(opts Options) Options {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = defaultMaxDepth
	}
	if opts.MaxTasks <= 0 {
		opts.MaxTasks = defaultMaxTasks
	}
	if opts.MaxTargets <= 0 {
		opts.MaxTargets = defaultMaxTargets
	}
	return opts
}

func (a *analyzer) load() error {
	if err := checkSurfaceGoVersion(a.root); err != nil {
		return err
	}
	buildFlags := []string{}
	if len(a.opts.BuildTags) > 0 {
		buildFlags = append(buildFlags, "-tags="+strings.Join(a.opts.BuildTags, ","))
	}
	config := &packages.Config{
		Context: a.ctx,
		Dir:     a.root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes |
			packages.NeedModule,
		BuildFlags: buildFlags,
		Tests:      false,
	}
	loaded, err := packages.Load(config, "./...")
	if ctxErr := a.ctx.Err(); ctxErr != nil {
		return fmt.Errorf("surface discovery: %w", ctxErr)
	}
	if err != nil {
		return fmt.Errorf("surface discovery: load packages: %w", err)
	}
	if len(loaded) == 0 {
		return fmt.Errorf("surface discovery: no build-selected Go packages")
	}
	allPackages := make(map[string]*packages.Package)
	packages.Visit(loaded, func(pkg *packages.Package) bool {
		if pkg != nil && pkg.PkgPath != "" {
			allPackages[pkg.PkgPath] = pkg
			if packageSafeForSSA(pkg) {
				a.packageFacts[pkg.PkgPath] = pkg
			}
		}
		return true
	}, nil)
	a.recordPackageLoadOutcomes(allPackages)
	for _, pkg := range loaded {
		if pkg.Module != nil && pkg.Module.Main {
			a.modulePath = pkg.Module.Path
			break
		}
	}
	safeLoaded := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		if packageSafeForSSA(pkg) {
			safeLoaded = append(safeLoaded, pkg)
		} else if pkg != nil {
			a.result.Coverage.PackagesSkipped = append(
				a.result.Coverage.PackagesSkipped,
				packageIdentity(pkg),
			)
		}
	}
	if len(safeLoaded) == 0 {
		return nil
	}
	a.program, a.packages = ssautil.AllPackages(safeLoaded, ssa.InstantiateGenerics)
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	ssaPackages := a.program.AllPackages()
	sort.Slice(ssaPackages, func(i, j int) bool {
		return ssaPackagePath(ssaPackages[i]) < ssaPackagePath(ssaPackages[j])
	})
	for _, pkg := range ssaPackages {
		if err := a.ctx.Err(); err != nil {
			return fmt.Errorf("surface discovery: %w", err)
		}
		if pkg == nil {
			continue
		}
		pkg.Build()
	}
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	a.allFunctions = ssautil.AllFunctions(a.program)
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	a.graph = cha.CallGraph(a.program)
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	return nil
}

func ssaPackagePath(pkg *ssa.Package) string {
	if pkg == nil || pkg.Pkg == nil {
		return ""
	}
	return pkg.Pkg.Path()
}

func checkSurfaceGoVersion(root string) error {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("surface discovery: read go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(io.LimitReader(file, 1024*1024))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || fields[0] != "go" {
			continue
		}
		required := "go" + fields[1]
		running := runtime.Version()
		if version.IsValid(required) && version.IsValid(running) && version.Compare(running, required) < 0 {
			return fmt.Errorf(
				"surface discovery: target module requires %s but repomap runtime is %s",
				required,
				running,
			)
		}
		return nil
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("surface discovery: read go.mod: %w", err)
	}
	return nil
}

func (a *analyzer) prepare() {
	a.relevant = map[*ssa.Function]bool{}
	a.relevanceDistance = map[*ssa.Function]int{}
	orderedFunctions := a.orderedFunctions()
	for _, function := range orderedFunctions {
		if a.ctx.Err() != nil {
			return
		}
		if function == nil || function.Blocks == nil {
			continue
		}
		a.functionByID[a.functionID(function)] = function
		a.functionByID[cleanFunctionID(a.functionID(function))] = function
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if value, ok := instruction.(ssa.Value); ok {
					a.recordFunctionReference(value)
				}
				call, ok := instruction.(ssa.CallInstruction)
				if ok {
					for _, argument := range call.Common().Args {
						a.recordFunctionReference(argument)
					}
					targets := a.targets(call)
					a.callTargets[call] = targets
					for _, target := range targets {
						if seed, matched := a.callSeed(target); matched && a.terminalSeedEligible(seed, call, target) {
							a.relevant[function] = true
							a.relevanceDistance[function] = 0
						}
					}
				}
				if store, ok := instruction.(*ssa.Store); ok {
					a.recordFunctionReference(store.Val)
					if _, matched := a.fieldSeed(store); matched {
						a.relevant[function] = true
						a.relevanceDistance[function] = 0
					}
				}
			}
		}
	}

	changed := true
	for changed {
		if a.ctx.Err() != nil {
			return
		}
		changed = false
		for _, function := range orderedFunctions {
			if a.ctx.Err() != nil {
				return
			}
			// The reverse closure only needs repository wrappers. Library functions
			// remain traversable when they directly contain a configured terminal
			// seed, but unrelated dependency and stdlib call graphs must not turn
			// into repository discovery work.
			if function == nil || function.Blocks == nil || a.relevant[function] || !a.isRepositoryFunction(function) {
				continue
			}
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(ssa.CallInstruction)
					if !ok {
						continue
					}
					for _, target := range a.callTargets[call] {
						if a.relevant[target] && a.propagationTargetEligible(call, target) {
							a.relevant[function] = true
							changed = true
							break
						}
					}
				}
			}
		}
	}

	changed = true
	for changed {
		changed = false
		for function := range a.relevant {
			if function == nil || function.Blocks == nil {
				continue
			}
			if distance, exists := a.relevanceDistance[function]; exists && distance == 0 {
				continue
			}
			best := int(^uint(0) >> 1)
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(ssa.CallInstruction)
					if !ok {
						continue
					}
					for _, target := range a.callTargets[call] {
						if distance, ok := a.relevanceDistance[target]; ok && distance+1 < best {
							best = distance + 1
						}
					}
				}
			}
			previous, exists := a.relevanceDistance[function]
			if best != int(^uint(0)>>1) && (!exists || best < previous) {
				a.relevanceDistance[function] = best
				changed = true
			}
		}
	}
}

func (a *analyzer) orderedFunctions() []*ssa.Function {
	functions := make([]*ssa.Function, 0, len(a.allFunctions))
	for function := range a.allFunctions {
		functions = append(functions, function)
	}
	sort.Slice(functions, func(i, j int) bool {
		left := a.functionID(functions[i]) + "\x00" + functions[i].Synthetic + "\x00" + strconv.Itoa(int(functions[i].Pos()))
		right := a.functionID(functions[j]) + "\x00" + functions[j].Synthetic + "\x00" + strconv.Itoa(int(functions[j].Pos()))
		return left < right
	})
	return functions
}

func (a *analyzer) recordFunctionReference(value ssa.Value) {
	if value == nil {
		return
	}
	switch current := value.(type) {
	case *ssa.Function:
		a.callbackReferences[current] = true
		a.callbackReferenceIDs[cleanFunctionID(a.functionID(current))] = true
	case *ssa.MakeClosure:
		if function, ok := current.Fn.(*ssa.Function); ok {
			a.callbackReferences[function] = true
			a.callbackReferenceIDs[cleanFunctionID(a.functionID(function))] = true
		}
	case *ssa.ChangeType:
		a.recordFunctionReference(current.X)
	case *ssa.Convert:
		a.recordFunctionReference(current.X)
	case *ssa.ChangeInterface:
		a.recordFunctionReference(current.X)
	case *ssa.MakeInterface:
		a.recordFunctionReference(current.X)
	}
}

func (a *analyzer) entrypoints() []*ssa.Function {
	result := []*ssa.Function{}
	for _, pkg := range a.packages {
		if pkg == nil || pkg.Pkg == nil || pkg.Pkg.Name() != "main" {
			continue
		}
		member := pkg.Members["main"]
		function, ok := member.(*ssa.Function)
		if !ok || function.Blocks == nil {
			continue
		}
		result = append(result, function)
	}
	sort.Slice(result, func(i, j int) bool { return a.functionID(result[i]) < a.functionID(result[j]) })
	return result
}

func (a *analyzer) walkDisconnectedRelevant(entrypoints []*ssa.Function) {
	if a.ctx.Err() != nil {
		return
	}
	candidates := make(map[*ssa.Function]*ssa.Function)
	for _, entrypoint := range entrypoints {
		reachablePackages := importedPackagePaths(entrypoint)
		for function := range a.relevant {
			if function == nil || function.Blocks == nil || function.Name() == "init" ||
				strings.EqualFold(function.Name(), "ServeHTTP") ||
				(!a.callbackReferences[function] && !a.callbackReferenceIDs[cleanFunctionID(a.functionID(function))]) ||
				!a.detachedRootEligible(function) ||
				!a.isRepositoryFunction(function) || !reachablePackages[functionPackagePath(function)] {
				continue
			}
			if _, exists := candidates[function]; !exists {
				candidates[function] = entrypoint
			}
		}
	}
	callers := make(map[*ssa.Function]map[*ssa.Function]struct{})
	for call, targets := range a.callTargets {
		caller := call.Parent()
		for _, target := range targets {
			if callers[target] == nil {
				callers[target] = make(map[*ssa.Function]struct{})
			}
			callers[target][caller] = struct{}{}
		}
	}

	for {
		if a.ctx.Err() != nil {
			return
		}
		remaining := make([]*ssa.Function, 0, len(candidates))
		for function := range candidates {
			if !a.walkedFunctions[function] {
				remaining = append(remaining, function)
			}
		}
		if len(remaining) == 0 {
			return
		}
		sort.Slice(remaining, func(i, j int) bool {
			return a.functionID(remaining[i]) < a.functionID(remaining[j])
		})
		roots := make([]*ssa.Function, 0, len(remaining))
		for _, function := range remaining {
			hasRelevantCaller := false
			for caller := range callers[function] {
				if caller != function && candidates[caller] != nil && !a.walkedFunctions[caller] {
					hasRelevantCaller = true
					break
				}
			}
			if !hasRelevantCaller {
				roots = append(roots, function)
			}
		}
		if len(roots) == 0 {
			roots = remaining[:1]
		}
		for _, root := range roots {
			if a.ctx.Err() != nil {
				return
			}
			if a.walkedFunctions[root] {
				continue
			}
			if a.tasks >= a.opts.MaxTasks {
				a.addBudget("tasks")
				return
			}
			triggerStart := len(a.result.Catalog.Triggers)
			serverStart := len(a.starts)
			a.detachedWalk = true
			a.walk(root, environment{}, nil, candidates[root], 0, false, "")
			a.detachedWalk = false
			if a.ctx.Err() != nil {
				return
			}
			if !a.walkedFunctions[root] {
				continue
			}
			location := a.location(root.Pos())
			frontier := Frontier{
				Kind:     "entrypoint_dispatch_unresolved",
				Detail:   "terminal-relevant code is import-reachable but its callback or lifecycle dispatch from the process entrypoint is unresolved",
				Location: &location,
			}
			for index := triggerStart; index < len(a.result.Catalog.Triggers); index++ {
				trigger := &a.result.Catalog.Triggers[index]
				trigger.DynamicFrontier = append(trigger.DynamicFrontier, frontier)
			}
			for index := serverStart; index < len(a.starts); index++ {
				a.starts[index].frontiers = append(a.starts[index].frontiers, frontier)
			}
			a.result.Coverage.UnsupportedDispatch = append(a.result.Coverage.UnsupportedDispatch, frontier)
		}
	}
}

func (a *analyzer) detachedRootEligible(function *ssa.Function) bool {
	if function == nil {
		return false
	}
	path := strings.ToLower(functionPackagePath(function))
	name := strings.ToLower(function.Name())
	commandPackage := strings.HasSuffix(path, "/cmd") || strings.Contains(path, "/cmd/") ||
		strings.HasSuffix(path, "/command") || strings.Contains(path, "/command/")
	if !commandPackage {
		return false
	}
	return strings.Contains(name, "run") || strings.Contains(name, "start") ||
		strings.Contains(name, "serve") || strings.Contains(name, "execute")
}

func importedPackagePaths(entrypoint *ssa.Function) map[string]bool {
	result := make(map[string]bool)
	if entrypoint == nil || entrypoint.Package() == nil || entrypoint.Package().Pkg == nil {
		return result
	}
	stack := []*types.Package{entrypoint.Package().Pkg}
	for len(stack) > 0 {
		pkg := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if pkg == nil || result[pkg.Path()] {
			continue
		}
		result[pkg.Path()] = true
		stack = append(stack, pkg.Imports()...)
	}
	return result
}

func (a *analyzer) prioritizeTargets(targets []*ssa.Function) []*ssa.Function {
	result := append([]*ssa.Function(nil), targets...)
	sort.SliceStable(result, func(i, j int) bool {
		_, leftSeed := a.callSeed(result[i])
		_, rightSeed := a.callSeed(result[j])
		if leftSeed != rightSeed {
			return leftSeed
		}
		if a.relevant[result[i]] != a.relevant[result[j]] {
			return a.relevant[result[i]]
		}
		leftDistance, leftRelevant := a.relevanceDistance[result[i]]
		rightDistance, rightRelevant := a.relevanceDistance[result[j]]
		if leftRelevant != rightRelevant {
			return leftRelevant
		}
		if leftRelevant && leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if a.isRepositoryFunction(result[i]) != a.isRepositoryFunction(result[j]) {
			return a.isRepositoryFunction(result[i])
		}
		return a.functionID(result[i]) < a.functionID(result[j])
	})
	return result
}

func (a *analyzer) walk(
	function *ssa.Function,
	env environment,
	chain []Wrapper,
	entrypoint *ssa.Function,
	depth int,
	ambiguous bool,
	parentAnchorID string,
) {
	if err := a.ctx.Err(); err != nil {
		return
	}
	if function == nil || function.Blocks == nil {
		return
	}
	if a.detachedWalk && a.walkedFunctions[function] && !a.relevant[function] {
		return
	}
	if depth > a.opts.MaxDepth {
		a.addBudget("depth")
		return
	}
	if a.tasks >= a.opts.MaxTasks {
		a.addBudget("tasks")
		return
	}
	if a.active[function] {
		frontier := Frontier{Kind: "recursive_wrapper", Detail: a.functionID(function)}
		a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontier)
		return
	}
	a.tasks++
	a.walkedFunctions[function] = true
	a.active[function] = true
	defer delete(a.active, function)
	a.result.Coverage.FunctionsInspected++

	for _, block := range function.Blocks {
		if err := a.ctx.Err(); err != nil {
			return
		}
		for _, instruction := range block.Instrs {
			if store, ok := instruction.(*ssa.Store); ok {
				a.recordAssignment(store, env)
				if seed, matched := a.fieldSeed(store); matched {
					a.recordFieldStore(seed, store, env)
				}
			}
			call, ok := instruction.(ssa.CallInstruction)
			if !ok {
				continue
			}
			targets := a.callTargets[call]
			if len(targets) == 0 {
				continue
			}
			var targetLimitFrontier *Frontier
			var dispatchFrontier *Frontier
			if len(targets) > a.opts.MaxTargets {
				a.addBudget("targets")
				targets = a.prioritizeTargets(targets)
				omitted := len(targets) - a.opts.MaxTargets
				location := a.location(call.Pos())
				frontier := Frontier{
					Kind: "call_target_limit",
					Detail: fmt.Sprintf(
						"%s omitted %d lower-priority static call target(s)",
						a.functionID(function),
						omitted,
					),
					Location: &location,
				}
				targetLimitFrontier = &frontier
				a.result.Coverage.UnsupportedDispatch = append(a.result.Coverage.UnsupportedDispatch, frontier)
				dispatch := Frontier{
					Kind:     "entrypoint_dispatch_unresolved",
					Detail:   "bounded callback candidates prevent proving dispatch from the process entrypoint",
					Location: &location,
				}
				dispatchFrontier = &dispatch
				targets = targets[:a.opts.MaxTargets]
			}
			callAmbiguous := ambiguous || (targetLimitFrontier == nil && len(targets) > 1)
			for _, target := range targets {
				if !a.callTargetEligible(call, target, entrypoint) {
					a.recordUnresolvedCallTarget(function, call, target, entrypoint)
					continue
				}
				triggerStart := len(a.result.Catalog.Triggers)
				serverStart := len(a.starts)
				architectureKind := ""
				// Detached command callbacks recover bounded surfaces only. Their
				// handoff from the process entrypoint is unresolved, so promoting
				// every visited target into architecture evidence would overstate
				// reachability and can swamp the bounded architecture map.
				if !a.detachedWalk {
					architectureKind = a.architectureCallKind(target)
				}
				nextParentAnchorID := parentAnchorID
				if architectureKind != "" {
					anchorID := a.recordArchitectureAnchor(
						architectureKind,
						architectureKind+" "+a.functionID(target),
						a.location(call.Pos()),
						a.symbol(target),
						"Kind is a bounded static classification of an exact call target; runtime reachability is not observed.",
					)
					a.recordArchitectureRelationship(parentAnchorID, anchorID, a.location(call.Pos()))
					nextParentAnchorID = anchorID
					if architectureKind == "registry_write" {
						extensionID := a.recordArchitectureAnchor(
							"extension_family",
							"extension registration via "+a.functionID(target),
							a.location(call.Pos()),
							a.symbol(target),
							"Registration proves a modular extension boundary, not creation or lifecycle execution of every implementation.",
						)
						a.recordArchitectureRelationship(anchorID, extensionID, a.location(call.Pos()))
					}
				}
				seed, matched := a.callSeed(target)
				matched = matched && a.terminalSeedEligible(seed, call, target)
				if matched {
					a.recordCall(seed, call, target, env, chain, entrypoint, callAmbiguous)
					if dispatchFrontier != nil {
						a.applyFrontierSince(triggerStart, serverStart, *dispatchFrontier)
					}
					continue
				}
				followComposition := !a.detachedWalk && a.shouldFollowComposition(target, architectureKind)
				if !a.relevant[target] && !followComposition {
					continue
				}
				if !a.relevant[target] {
					if a.compositionVisited[target] {
						continue
					}
					a.compositionVisited[target] = true
				}
				location := a.location(call.Pos())
				wrapper := Wrapper{
					Symbol:   a.symbol(target),
					Callsite: location,
					Origin:   a.wrapperOrigin(target),
				}
				nextChain := append(append([]Wrapper{}, chain...), wrapper)
				a.walk(
					target,
					a.bind(target, a.arguments(call), env, depth),
					nextChain,
					entrypoint,
					depth+1,
					callAmbiguous,
					nextParentAnchorID,
				)
				if dispatchFrontier != nil {
					a.applyFrontierSince(triggerStart, serverStart, *dispatchFrontier)
				}
			}
		}
	}
}

func (a *analyzer) terminalTargetEligible(call ssa.CallInstruction, target *ssa.Function) bool {
	if call == nil || target == nil {
		return false
	}
	if call.Common().StaticCallee() == target {
		return true
	}
	closure, ok := call.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	function, ok := closure.Fn.(*ssa.Function)
	return ok && function == target
}

// callTargetEligible admits only call-graph candidates that are compatible with
// the exact SSA call and the selected executable's build/import closure. CHA can
// otherwise join equal-shaped callbacks from independent main packages, which is
// not evidence that one executable reaches the other's behavior.
func (a *analyzer) callTargetEligible(
	call ssa.CallInstruction,
	target, entrypoint *ssa.Function,
) bool {
	if !a.callTargetWitness(call, target) || entrypoint == nil {
		return false
	}
	packages := a.entrypointPackages[entrypoint]
	if packages == nil {
		packages = importedPackagePaths(entrypoint)
		a.entrypointPackages[entrypoint] = packages
	}
	return packages[functionPackagePath(target)]
}

func (a *analyzer) callTargetWitness(call ssa.CallInstruction, target *ssa.Function) bool {
	if a.terminalTargetEligible(call, target) {
		return true
	}
	if call == nil || target == nil || target.Signature == nil {
		return false
	}
	if call.Common().IsInvoke() {
		return true
	}
	value := call.Common().Value
	return value != nil && types.AssignableTo(target.Signature, value.Type())
}

func (a *analyzer) recordUnresolvedCallTarget(
	function *ssa.Function,
	call ssa.CallInstruction,
	target, entrypoint *ssa.Function,
) {
	location := a.location(call.Pos())
	detail := fmt.Sprintf(
		"%s did not admit candidate %s for executable %s: no exact or type-valid call witness within its import closure",
		a.functionID(function),
		a.functionID(target),
		a.functionID(entrypoint),
	)
	a.result.Coverage.UnsupportedDispatch = append(a.result.Coverage.UnsupportedDispatch, Frontier{
		Kind:     "call_target_unresolved",
		Detail:   detail,
		Location: &location,
	})
}

func (a *analyzer) terminalSeedEligible(
	seed catalog.Seed,
	call ssa.CallInstruction,
	target *ssa.Function,
) bool {
	if seed.Effect.Kind == catalog.EffectHTTPRouteProvider {
		return a.isRepositoryFunction(target)
	}
	return a.terminalTargetEligible(call, target)
}

func (a *analyzer) propagationTargetEligible(call ssa.CallInstruction, target *ssa.Function) bool {
	return a.callTargetWitness(call, target)
}

func (a *analyzer) applyFrontierSince(triggerStart, serverStart int, frontier Frontier) {
	for index := triggerStart; index < len(a.result.Catalog.Triggers); index++ {
		trigger := &a.result.Catalog.Triggers[index]
		trigger.DynamicFrontier = append(trigger.DynamicFrontier, frontier)
	}
	for index := serverStart; index < len(a.starts); index++ {
		a.starts[index].frontiers = append(a.starts[index].frontiers, frontier)
	}
}

func (a *analyzer) recordCall(
	seed catalog.Seed,
	call ssa.CallInstruction,
	target *ssa.Function,
	env environment,
	chain []Wrapper,
	entrypoint *ssa.Function,
	ambiguous bool,
) {
	a.matchedSeeds = append(a.matchedSeeds, seed.ID)
	values := a.project(seed, a.arguments(call), target, env, 0)
	location := a.location(call.Pos())
	switch seed.Effect.Kind {
	case catalog.EffectHTTPServerStart:
		a.starts = append(a.starts, dispatchStart{
			seed: seed, values: values, entrypoint: entrypoint,
			chain: append([]Wrapper(nil), chain...), location: location, ambiguous: ambiguous,
			detached: a.detachedWalk,
		})
	case catalog.EffectHTTPRouteRegistration:
		loopSignal, inLoop := a.registrationLoop(call, seed)
		if inLoop {
			a.addLoopSignal(loopSignal)
		}
		a.recordRoute(seed, values, location, chain, entrypoint, ambiguous)
	case catalog.EffectHTTPRouteProvider:
		a.recordRouteProvider(seed, call, target, chain, entrypoint, ambiguous)
	case catalog.EffectHTTPRouteAssembly:
		a.recordRouteAssembly(seed, values, location, chain, entrypoint, ambiguous)
	case catalog.EffectAsyncTaskStart:
		a.recordAsyncTask(seed, values, location, chain, entrypoint, ambiguous)
	}
	a.recordSummary(seed, values, chain, target)
}

func (a *analyzer) recordRouteAssembly(
	seed catalog.Seed,
	values map[string]Value,
	location Location,
	chain []Wrapper,
	entrypoint *ssa.Function,
	ambiguous bool,
) {
	frontier := Frontier{
		Kind:     "configuration_assembled_route_inventory",
		Detail:   "Routes are assembled from runtime configuration; static analysis did not invent or enumerate them.",
		Location: &location,
	}
	resolution := "dynamic"
	if ambiguous {
		resolution = "ambiguous"
	}
	basis := string(catalog.OriginCatalogStatic)
	if len(chain) > 0 {
		basis = string(catalog.OriginWrapperStatic)
	}
	record := TriggerRecord{
		Kind: "http_route_frontier",
		Identity: Identity{Path: dynamicValue(
			"routes assembled from runtime configuration",
		)},
		Transport: seed.Effect.Transport, Framework: seed.Effect.Framework,
		ProcessEntrypoint: a.symbol(entrypoint), Dispatcher: values["dispatcher"],
		RegistrationSite: location, Handler: dynamicValue("configuration-selected handlers"),
		Middleware: []Value{}, WrapperChain: append([]Wrapper(nil), chain...),
		FinalSeed: seed.ID, DiscoveryBasis: basis, Certainty: "static",
		Resolution: resolution, ScenarioID: a.scenario.ID,
		Evidence: []Evidence{{
			ID: "route-assembly:" + locationKey(location), Kind: "route_assembly_call",
			Location: location, Detail: seed.ID,
		}},
		Provenance: []Provenance{{
			Provider: "go_ssa", Version: AnalyzerVersion,
			Operation: "record_configuration_route_frontier", Detail: seed.ID,
		}},
		DynamicFrontier: []Frontier{frontier},
		Status:          "configured_route_inventory_unresolved",
		ProvisionalID:   true,
	}
	record.TerminalSourceScope, record.ApplicationClass, record.PromotionBasis =
		classifyTerminalOwnership(location, chain, a.detachedWalk)
	record.ID = stableTriggerID(record)
	a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
	a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontier)
}

type providedRouteDescriptor struct {
	path     Value
	handler  Value
	location Location
}

const maxReturnedRouteDescriptors = 32

func (a *analyzer) recordRouteProvider(
	seed catalog.Seed,
	call ssa.CallInstruction,
	target *ssa.Function,
	chain []Wrapper,
	entrypoint *ssa.Function,
	ambiguous bool,
) {
	descriptors, diagnostic := a.returnedRouteDescriptors(seed, target)
	if len(descriptors) == 0 {
		location := a.location(call.Pos())
		if diagnostic == "" {
			diagnostic = a.functionID(target) + " did not yield a bounded returned route descriptor literal"
		}
		a.result.Coverage.UnsupportedDispatch = append(a.result.Coverage.UnsupportedDispatch, Frontier{
			Kind:     "route_provider_projection_unresolved",
			Detail:   diagnostic,
			Location: &location,
		})
		return
	}
	registration := a.location(call.Pos())
	for _, descriptor := range descriptors {
		frontiers := []Frontier{{
			Kind:     "route_provider_dispatch_candidate",
			Detail:   "Exact route descriptor found; runtime provider selection and consumer registration were not observed.",
			Location: &registration,
		}}
		if !descriptor.handler.Known {
			frontiers = append(frontiers, Frontier{
				Kind:     "dynamic_handler_identity",
				Detail:   "The returned descriptor handler could not be resolved to an exact function or method.",
				Location: &descriptor.location,
			})
		}
		if diagnostic != "" {
			frontiers = append(frontiers, Frontier{
				Kind: "route_provider_projection_bounded", Detail: diagnostic, Location: &registration,
			})
		}
		basis := string(catalog.OriginCatalogStatic)
		if len(chain) > 0 {
			basis = string(catalog.OriginWrapperStatic)
		}
		descriptorLocation := descriptor.location
		resolution := "exact"
		if !descriptor.handler.Known {
			resolution = "partial"
		}
		record := TriggerRecord{
			Kind:              "http_route_descriptor",
			Identity:          Identity{Path: descriptor.path},
			Transport:         seed.Effect.Transport,
			Framework:         seed.Effect.Framework,
			ProcessEntrypoint: a.symbol(entrypoint),
			Dispatcher:        dynamicValue("registry-selected route consumer"),
			RegistrationSite:  registration,
			DescriptorSite:    &descriptorLocation,
			Handler:           descriptor.handler,
			Middleware:        []Value{},
			WrapperChain:      append([]Wrapper(nil), chain...),
			FinalSeed:         seed.ID,
			DiscoveryBasis:    basis,
			Certainty:         "static",
			Resolution:        resolution,
			ScenarioID:        a.scenario.ID,
			Evidence: []Evidence{
				{ID: "route-provider:" + locationKey(registration), Kind: "route_provider_call", Location: registration, Detail: seed.ID},
				{ID: "route-descriptor:" + locationKey(descriptor.location), Kind: "returned_route_descriptor", Location: descriptor.location, Detail: a.functionID(target)},
			},
			Provenance: []Provenance{{
				Provider: "go_ssa", Version: AnalyzerVersion,
				Operation: "extract_returned_route_descriptor", Detail: seed.ID,
			}},
			DynamicFrontier: frontiers,
			Status:          "confirmed_route_descriptor",
			ProvisionalID:   !descriptor.path.Known || !descriptor.handler.Known,
		}
		record.TerminalSourceScope, record.ApplicationClass, record.PromotionBasis =
			classifyTerminalOwnership(descriptorLocation, chain, a.detachedWalk)
		record.ID = stableTriggerID(record)
		a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
		a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontiers...)
	}
}

func (a *analyzer) returnedRouteDescriptors(
	seed catalog.Seed,
	target *ssa.Function,
) ([]providedRouteDescriptor, string) {
	pathField := seed.Projections["path"].Field
	handlerField := seed.Projections["handler"].Field
	if target == nil || pathField == "" || handlerField == "" {
		return nil, "route provider is missing exact returned path/handler field semantics"
	}
	if !routeProviderResultHasFields(target.Signature, pathField, handlerField) {
		return nil, "route provider result type does not contain the configured descriptor fields"
	}
	targetPosition := a.program.Fset.PositionFor(target.Pos(), true)
	if targetPosition.Filename == "" {
		return nil, "route provider source location is unavailable"
	}
	facts := a.packageFacts[functionPackagePath(target)]
	if facts == nil || facts.TypesInfo == nil {
		return nil, "route provider typed syntax is unavailable"
	}
	var file *ast.File
	for _, candidate := range facts.Syntax {
		if a.program.Fset.PositionFor(candidate.Pos(), true).Filename == targetPosition.Filename {
			file = candidate
			break
		}
	}
	if file == nil {
		return nil, "route provider build-selected syntax is unavailable"
	}
	fileSet := a.program.Fset
	result := []providedRouteDescriptor{}
	diagnostic := ""
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != target.Name() || function.Body == nil ||
			fileSet.Position(function.Pos()).Line != targetPosition.Line {
			continue
		}
		for _, statement := range function.Body.List {
			returned, ok := statement.(*ast.ReturnStmt)
			if !ok {
				continue
			}
			for _, expression := range returned.Results {
				literal, ok := unwrappedCompositeLiteral(expression)
				if !ok {
					diagnostic = "route provider return value is not a direct bounded descriptor literal"
					continue
				}
				for _, element := range literal.Elts {
					if len(result) >= maxReturnedRouteDescriptors {
						diagnostic = "route provider descriptor limit reached"
						break
					}
					descriptor, ok := element.(*ast.CompositeLit)
					if !ok {
						diagnostic = "route provider contains a non-literal descriptor"
						continue
					}
					fields := keyedCompositeFields(descriptor)
					pathExpression, pathOK := fields[pathField]
					handlerExpression, handlerOK := fields[handlerField]
					if !pathOK || !handlerOK {
						continue
					}
					path, known := typedStringValue(pathExpression, facts.TypesInfo)
					if !known {
						diagnostic = "route provider contains a non-constant route path"
						continue
					}
					position := fileSet.Position(descriptor.Pos())
					result = append(result, providedRouteDescriptor{
						path:     knownValue("returned_field", path),
						handler:  routeProviderHandler(handlerExpression, facts.TypesInfo),
						location: a.sourceLocation(position.Filename, position.Line, position.Column),
					})
				}
			}
		}
	}
	return result, diagnostic
}

func routeProviderResultHasFields(signature *types.Signature, pathField, handlerField string) bool {
	if signature == nil || signature.Results() == nil || signature.Results().Len() != 1 {
		return false
	}
	resultType := signature.Results().At(0).Type()
	var element types.Type
	switch result := resultType.Underlying().(type) {
	case *types.Slice:
		element = result.Elem()
	case *types.Array:
		element = result.Elem()
	default:
		return false
	}
	if pointer, ok := element.(*types.Pointer); ok {
		element = pointer.Elem()
	}
	structure, ok := element.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	foundPath := false
	foundHandler := false
	for index := 0; index < structure.NumFields(); index++ {
		switch structure.Field(index).Name() {
		case pathField:
			foundPath = true
		case handlerField:
			foundHandler = true
		}
	}
	return foundPath && foundHandler
}

func unwrappedCompositeLiteral(expression ast.Expr) (*ast.CompositeLit, bool) {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parenthesized.X
	}
	literal, ok := expression.(*ast.CompositeLit)
	return literal, ok
}

func keyedCompositeFields(literal *ast.CompositeLit) map[string]ast.Expr {
	result := make(map[string]ast.Expr)
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := field.Key.(*ast.Ident)
		if ok {
			result[name.Name] = field.Value
		}
	}
	return result
}

func typedStringValue(expression ast.Expr, info *types.Info) (string, bool) {
	if expression == nil || info == nil {
		return "", false
	}
	value := info.Types[expression].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

func routeProviderHandler(expression ast.Expr, info *types.Info) Value {
	if expression == nil || info == nil {
		return dynamicValue("unresolved returned descriptor handler")
	}
	if call, ok := expression.(*ast.CallExpr); ok {
		if len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
			return dynamicValue("unresolved returned descriptor handler")
		}
		expression = call.Args[0]
	}
	var object types.Object
	switch current := expression.(type) {
	case *ast.Ident:
		object = info.Uses[current]
	case *ast.SelectorExpr:
		if selection := info.Selections[current]; selection != nil {
			object = selection.Obj()
		} else {
			object = info.Uses[current.Sel]
		}
	}
	function, ok := object.(*types.Func)
	if !ok || function.Pkg() == nil {
		return dynamicValue("unresolved returned descriptor handler")
	}
	return knownValue("function", typesFunctionID(function))
}

func typesFunctionID(function *types.Func) string {
	path := function.Pkg().Path()
	signature, _ := function.Type().(*types.Signature)
	if signature != nil && signature.Recv() != nil {
		return path + ".(" + receiverName(signature) + ")." + function.Name()
	}
	return path + "." + function.Name()
}

func (a *analyzer) recordRoute(
	seed catalog.Seed,
	values map[string]Value,
	location Location,
	chain []Wrapper,
	entrypoint *ssa.Function,
	ambiguous bool,
) {
	pathValue := values["path"]
	if prefix, exists := values["path_prefix"]; exists {
		if prefix.Known && pathValue.Known {
			pathValue = knownValue("concatenation", prefix.Text+pathValue.Text)
		} else {
			pathValue = dynamicValue(strings.TrimSpace(prefix.Text + " + " + pathValue.Text))
		}
	}
	if !pathValue.Known && len(pathValue.Candidates) > 0 {
		concrete, unresolved := partitionRouteCandidates(pathValue.Candidates)
		for _, candidate := range concrete {
			candidateValues := cloneValues(values)
			candidateValues["path"] = knownValue("constant_alternative", candidate)
			delete(candidateValues, "path_prefix")
			a.recordRoute(seed, candidateValues, location, chain, entrypoint, true)
		}
		if len(unresolved) == 0 {
			return
		}
		pathValue = dynamicValue("unresolved route alternative")
		pathValue.Candidates = unresolved
	}
	handler := values["handler"]
	dispatcher := values["dispatcher"]
	method := values["method"].Text
	middleware := []Value{}
	if handler.Kind == "middleware_result" && len(handler.Candidates) > 1 {
		for _, candidate := range handler.Candidates[1:] {
			middleware = append(middleware, knownValue("function", candidate))
		}
		handler.Candidates = handler.Candidates[:1]
		if len(handler.Candidates) == 1 {
			handler.Text = handler.Candidates[0]
			handler.Known = true
		}
	}
	frontiers := []Frontier{}
	if !pathValue.Known {
		frontiers = append(frontiers, Frontier{Kind: "dynamic_route_identity", Detail: pathValue.Text, Location: &location})
	}
	if !handler.Known {
		frontiers = append(frontiers, Frontier{Kind: "dynamic_handler_identity", Detail: handler.Text, Location: &location})
	}
	status := "confirmed_direct_registration"
	basis := string(catalog.OriginCatalogStatic)
	if len(chain) > 0 {
		status = "confirmed_through_library_wrapper"
		basis = string(catalog.OriginWrapperStatic)
		for _, wrapper := range chain {
			if wrapper.Origin == "repository" {
				status = "confirmed_through_repository_wrapper"
				break
			}
		}
	}
	resolution := "exact"
	if ambiguous {
		resolution = "ambiguous"
	}
	if len(frontiers) > 0 {
		resolution = "dynamic"
		status = "dynamic_unknown"
	}
	record := TriggerRecord{
		Kind:              "http_route",
		Identity:          Identity{Method: method, Path: pathValue},
		Transport:         seed.Effect.Transport,
		Framework:         seed.Effect.Framework,
		ProcessEntrypoint: a.symbol(entrypoint),
		Dispatcher:        dispatcher,
		RegistrationSite:  location,
		Handler:           handler,
		Middleware:        middleware,
		WrapperChain:      append([]Wrapper{}, chain...),
		FinalSeed:         seed.ID,
		DiscoveryBasis:    basis,
		Certainty:         "static",
		Resolution:        resolution,
		ScenarioID:        a.scenario.ID,
		Evidence: []Evidence{{
			ID: "registration:" + locationKey(location), Kind: "registration_call",
			Location: location, Detail: seed.ID,
		}},
		Provenance: []Provenance{{
			Provider: "go_ssa", Version: AnalyzerVersion,
			Operation: "propagate_terminal_semantics", Detail: seed.ID,
		}},
		DynamicFrontier: frontiers,
		Status:          status,
	}
	record.TerminalSourceScope, record.ApplicationClass, record.PromotionBasis =
		classifyTerminalOwnership(location, chain, a.detachedWalk)
	record.ProvisionalID = !pathValue.Known || !handler.Known || !dispatcher.Known
	record.ID = stableTriggerID(record)
	a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
	a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontiers...)
}

func partitionRouteCandidates(candidates []string) (concrete, unresolved []string) {
	seenConcrete := make(map[string]struct{})
	seenUnresolved := make(map[string]struct{})
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if isConcreteRoutePattern(candidate) {
			if _, exists := seenConcrete[candidate]; !exists {
				seenConcrete[candidate] = struct{}{}
				concrete = append(concrete, candidate)
			}
			continue
		}
		if _, exists := seenUnresolved[candidate]; !exists {
			seenUnresolved[candidate] = struct{}{}
			unresolved = append(unresolved, candidate)
		}
	}
	sort.Strings(concrete)
	sort.Strings(unresolved)
	return concrete, unresolved
}

func isConcreteRoutePattern(value string) bool {
	if strings.Contains(value, " | ") || strings.Contains(value, " + ") {
		return false
	}
	if strings.HasPrefix(value, "/") {
		return true
	}
	fields := strings.Fields(value)
	return len(fields) == 2 && fields[0] == strings.ToUpper(fields[0]) && strings.HasPrefix(fields[1], "/")
}

func hasRepositoryWrapper(chain []Wrapper) bool {
	for _, wrapper := range chain {
		if wrapper.Origin == "repository" ||
			(wrapper.Callsite.Path != "" && !filepath.IsAbs(wrapper.Callsite.Path) &&
				!strings.Contains(wrapper.Symbol.Name, "$") &&
				!strings.Contains(wrapper.Symbol.Name, "#") &&
				!strings.EqualFold(wrapper.Symbol.Name, "init")) {
			return true
		}
	}
	return false
}

func classifyTerminalOwnership(location Location, chain []Wrapper, detached bool) (string, string, string) {
	repositoryWrapper := hasRepositoryWrapper(chain)
	if !filepath.IsAbs(location.Path) {
		// An unresolved dispatch does not turn repository-local registrations
		// into dependency behavior. It limits reachability, which is recorded
		// separately as a frontier on the surface.
		return "repository", ApplicationSurface, PromotionRepositoryRegistration
	}
	if detached {
		basis := PromotionNone
		if repositoryWrapper {
			basis = PromotionRepositoryWrapper
		}
		return terminalSourceScope(location), SupportingDependencyBehavior, basis
	}
	if repositoryWrapper {
		return "dependency", ApplicationSurface, PromotionRepositoryWrapper
	}
	return terminalSourceScope(location), DependencyOnly, PromotionNone
}

func terminalSourceScope(location Location) string {
	if !filepath.IsAbs(location.Path) {
		return "repository"
	}
	return "dependency"
}

func cloneValues(values map[string]Value) map[string]Value {
	result := make(map[string]Value, len(values))
	for name, value := range values {
		result[name] = value
	}
	return result
}

func (a *analyzer) recordFieldStore(seed catalog.Seed, store *ssa.Store, env environment) {
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return
	}
	server := a.eval(field.X, env, 0)
	dispatcher := a.eval(store.Val, env, 0)
	if server.Text != "" {
		a.assignments[valueAddressKey(server)] = dispatcher
	}
	a.matchedSeeds = append(a.matchedSeeds, seed.ID)
}

func (a *analyzer) recordAssignment(store *ssa.Store, env environment) {
	if store == nil || !simpleAssignmentValue(store.Val, 0) {
		return
	}
	key := a.assignmentKey(store.Addr, env, 0)
	value := a.eval(store.Val, env, 0)
	if key == "" || value.Text == "" {
		return
	}
	if previous, exists := a.valuesByAddress[key]; exists {
		a.valuesByAddress[key] = mergeValues([]Value{previous, value})
		return
	}
	a.valuesByAddress[key] = value
}

func simpleAssignmentValue(value ssa.Value, depth int) bool {
	if value == nil || depth > 8 {
		return false
	}
	switch current := value.(type) {
	case *ssa.Const, *ssa.Function, *ssa.Parameter, *ssa.Global, *ssa.Alloc, *ssa.MakeClosure:
		return true
	case *ssa.ChangeType:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.Convert:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.ChangeInterface:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.MakeInterface:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.TypeAssert:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.UnOp:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.FieldAddr:
		return simpleAssignmentValue(current.X, depth+1)
	case *ssa.Field:
		return simpleAssignmentValue(current.X, depth+1)
	default:
		return false
	}
}

func (a *analyzer) assignmentKey(value ssa.Value, env environment, depth int) string {
	switch current := value.(type) {
	case *ssa.Global:
		return packagePath(current.Pkg) + "." + current.Name()
	case *ssa.FieldAddr:
		base := a.eval(current.X, env, depth+1)
		if base.Text == "" {
			return ""
		}
		return valueAddressKey(base) + "." + fieldName(current.X.Type(), current.Field)
	}
	resolved := a.eval(value, env, depth+1)
	return valueAddressKey(resolved)
}

func (a *analyzer) assignment(key string) Value {
	if key == "" {
		return Value{}
	}
	return a.valuesByAddress[key]
}

func (a *analyzer) project(
	seed catalog.Seed,
	args []ssa.Value,
	target *ssa.Function,
	env environment,
	depth int,
) map[string]Value {
	result := make(map[string]Value, len(seed.Projections))
	receiverOffset := 0
	if target.Signature.Recv() != nil {
		receiverOffset = 1
	}
	for name, projection := range seed.Projections {
		var value Value
		switch projection.Source {
		case catalog.ProjectionConstant:
			value = knownValue("constant", projection.Value)
		case catalog.ProjectionReceiver:
			if len(args) > 0 {
				value = a.eval(args[0], env, depth+1)
			}
		case catalog.ProjectionReceiverField:
			if len(args) > 0 {
				receiver := a.eval(args[0], env, depth+1)
				value = a.assignment(valueAddressKey(receiver) + "." + projection.Field)
			}
		case catalog.ProjectionReturnField:
			value = dynamicValue("returned field " + projection.Field)
		case catalog.ProjectionArgument:
			if projection.Index != nil {
				index := receiverOffset + *projection.Index
				if index >= 0 && index < len(args) {
					value = a.eval(args[index], env, depth+1)
				}
			}
		}
		if projection.Default != "" && (!value.Known || value.Text == "nil") {
			value = knownValue("default", projection.Default)
		}
		if value.Text == "" {
			value = dynamicValue("unknown " + name)
		}
		result[name] = value
	}
	return result
}

func (a *analyzer) bind(
	target *ssa.Function,
	args []ssa.Value,
	caller environment,
	depth int,
) environment {
	result := environment{}
	for index, parameter := range target.Params {
		if index >= len(args) {
			result[parameter] = dynamicValue("parameter " + parameter.Name())
			continue
		}
		result[parameter] = a.eval(args[index], caller, depth+1)
	}
	return result
}

func (a *analyzer) eval(value ssa.Value, env environment, depth int) Value {
	if value == nil {
		return dynamicValue("nil")
	}
	if depth > a.opts.MaxDepth {
		return dynamicValue("value depth budget")
	}
	if resolved, ok := env[value]; ok {
		return resolved
	}
	switch current := value.(type) {
	case *ssa.Const:
		if current.Value == nil || current.IsNil() {
			return dynamicValue("nil")
		}
		if current.Value.Kind() == constant.String {
			return knownValue("constant", constant.StringVal(current.Value))
		}
		return knownValue("constant", current.Value.ExactString())
	case *ssa.Function:
		return knownValue("function", a.functionID(current))
	case *ssa.Parameter:
		return dynamicValue("parameter " + current.Name())
	case *ssa.Global:
		return knownValue("global", packagePath(current.Pkg)+"."+current.Name())
	case *ssa.Alloc:
		value := knownValue("allocation", a.valueIdentity(current))
		value.addressKey = contextualAddressKey(value.Text, env)
		return value
	case *ssa.MakeClosure:
		if function, ok := current.Fn.(*ssa.Function); ok {
			return knownValue("method_value", cleanFunctionID(a.functionID(function)))
		}
	case *ssa.BinOp:
		if current.Op == token.ADD {
			left := a.eval(current.X, env, depth+1)
			right := a.eval(current.Y, env, depth+1)
			if left.Known && right.Known {
				return knownValue("concatenation", left.Text+right.Text)
			}
			return dynamicValue(strings.TrimSpace(left.Text + " + " + right.Text))
		}
	case *ssa.ChangeType:
		return a.eval(current.X, env, depth+1)
	case *ssa.Convert:
		return a.eval(current.X, env, depth+1)
	case *ssa.ChangeInterface:
		return a.eval(current.X, env, depth+1)
	case *ssa.MakeInterface:
		return a.eval(current.X, env, depth+1)
	case *ssa.TypeAssert:
		return a.eval(current.X, env, depth+1)
	case *ssa.Extract:
		return a.eval(current.Tuple, env, depth+1)
	case *ssa.UnOp:
		if current.Op == token.MUL {
			if assigned := a.assignment(a.assignmentKey(current.X, env, depth+1)); assigned.Text != "" {
				return assigned
			}
			if field, ok := current.X.(*ssa.FieldAddr); ok {
				base := a.eval(field.X, env, depth+1)
				return dynamicValue(base.Text + "." + fieldName(field.X.Type(), field.Field))
			}
		}
		return a.eval(current.X, env, depth+1)
	case *ssa.FieldAddr:
		base := a.eval(current.X, env, depth+1)
		value := knownValue("field", base.Text+"."+fieldName(current.X.Type(), current.Field))
		value.addressKey = valueAddressKey(base) + "." + fieldName(current.X.Type(), current.Field)
		return value
	case *ssa.Field:
		base := a.eval(current.X, env, depth+1)
		if assigned := a.assignment(valueAddressKey(base) + "." + fieldName(current.X.Type(), current.Field)); assigned.Text != "" {
			return assigned
		}
		return dynamicValue(base.Text + "." + fieldName(current.X.Type(), current.Field))
	case *ssa.Phi:
		values := []Value{}
		for _, edge := range current.Edges {
			values = append(values, a.eval(edge, env, depth+1))
		}
		return mergeValues(values)
	case *ssa.Call:
		return a.evalCall(current, env, depth+1)
	}
	return dynamicValue(value.String())
}

func (a *analyzer) evalCall(call *ssa.Call, env environment, depth int) Value {
	targets := a.callTargets[call]
	if len(targets) == 0 {
		targets = a.targets(call)
	}
	if len(targets) != 1 {
		candidates := make([]string, 0, len(targets))
		for _, target := range targets {
			candidates = append(candidates, a.functionID(target))
		}
		sort.Strings(candidates)
		return Value{Kind: "call", Text: "ambiguous call", Candidates: candidates}
	}
	target := targets[0]
	if packagePath(target.Pkg) == "net/http" && target.Name() == "NewServeMux" {
		value := knownValue("dispatcher", "net/http.NewServeMux@"+locationKey(a.location(call.Pos())))
		value.addressKey = contextualAddressKey(value.Text, env)
		return value
	}
	underlying := []string{}
	for _, argument := range call.Common().Args {
		candidate := a.eval(argument, env, depth+1)
		if candidate.Kind == "function" || candidate.Kind == "method_value" {
			underlying = append(underlying, candidate.Text)
		}
	}
	if len(underlying) > 0 {
		if returned, ok := a.evalReturn(target, call.Common().Args, env, depth+1); ok &&
			(returned.Kind == "function" || returned.Kind == "method_value") {
			return Value{
				Kind: "middleware_result", Text: returned.Text, Known: returned.Known,
				Candidates: []string{returned.Text, a.functionID(target)},
			}
		}
	}
	if returned, ok := a.evalReturn(target, a.arguments(call), env, depth+1); ok {
		return returned
	}
	if len(underlying) > 0 {
		return Value{
			Kind: "middleware_result", Text: underlying[0], Known: true,
			Candidates: append(underlying, a.functionID(target)),
		}
	}
	return dynamicValue("result of " + a.functionID(target))
}

func (a *analyzer) evalReturn(
	target *ssa.Function,
	args []ssa.Value,
	caller environment,
	depth int,
) (Value, bool) {
	if target == nil || target.Blocks == nil || depth > a.opts.MaxDepth || a.active[target] {
		return Value{}, false
	}
	env := a.bind(target, args, caller, depth+1)
	values := []Value{}
	for _, block := range target.Blocks {
		for _, instruction := range block.Instrs {
			if store, ok := instruction.(*ssa.Store); ok {
				a.recordAssignment(store, env)
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) == 0 {
				continue
			}
			values = append(values, a.eval(returned.Results[0], env, depth+1))
		}
	}
	if len(values) == 0 {
		return Value{}, false
	}
	return mergeValues(values), true
}

func (a *analyzer) targets(call ssa.CallInstruction) []*ssa.Function {
	result := []*ssa.Function{}
	seen := map[*ssa.Function]bool{}
	if static := call.Common().StaticCallee(); static != nil {
		result = append(result, static)
		seen[static] = true
	}
	if closure, ok := call.Common().Value.(*ssa.MakeClosure); ok {
		if function, ok := closure.Fn.(*ssa.Function); ok && !seen[function] {
			result = append(result, function)
			seen[function] = true
		}
	}
	if node := a.graph.Nodes[call.Parent()]; node != nil {
		for _, edge := range node.Out {
			if edge.Site != call || edge.Callee == nil || edge.Callee.Func == nil || seen[edge.Callee.Func] {
				continue
			}
			result = append(result, edge.Callee.Func)
			seen[edge.Callee.Func] = true
		}
	}
	byImplementation := map[string]*ssa.Function{}
	for _, function := range result {
		key := functionPackagePath(function) + "\x00" + function.Name() + "\x00" + strconv.Itoa(int(function.Pos()))
		previous, exists := byImplementation[key]
		if !exists || (previous.Synthetic != "" && function.Synthetic == "") {
			byImplementation[key] = function
		}
	}
	result = result[:0]
	for _, function := range byImplementation {
		result = append(result, function)
	}
	sort.Slice(result, func(i, j int) bool { return a.functionID(result[i]) < a.functionID(result[j]) })
	return result
}

func (a *analyzer) arguments(call ssa.CallInstruction) []ssa.Value {
	args := call.Common().Args
	if !call.Common().IsInvoke() {
		return args
	}
	result := make([]ssa.Value, 0, len(args)+1)
	result = append(result, call.Common().Value)
	result = append(result, args...)
	return result
}

func (a *analyzer) callSeed(function *ssa.Function) (catalog.Seed, bool) {
	if function == nil || function.Signature == nil {
		return catalog.Seed{}, false
	}
	path := functionPackagePath(function)
	receiver := receiverName(function.Signature)
	argumentCount := function.Signature.Params().Len()
	for _, seed := range a.catalog.Seeds {
		if seed.Operation != catalog.OperationCall || seed.Symbol.PackagePath != path ||
			seed.Symbol.Name != function.Name() || seed.Symbol.Receiver != receiver {
			continue
		}
		if argumentCount < seed.Symbol.MinArgs {
			continue
		}
		if seed.Symbol.Variadic != nil && function.Signature.Variadic() != *seed.Symbol.Variadic {
			continue
		}
		return seed, true
	}
	return catalog.Seed{}, false
}

func (a *analyzer) fieldSeed(store *ssa.Store) (catalog.Seed, bool) {
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return catalog.Seed{}, false
	}
	name := fieldName(field.X.Type(), field.Field)
	path, receiver := receiverTypeIdentity(field.X.Type())
	for _, seed := range a.catalog.Seeds {
		if seed.Operation == catalog.OperationFieldStore && seed.Symbol.PackagePath == path &&
			seed.Symbol.Receiver == receiver && seed.Symbol.Name == name {
			return seed, true
		}
	}
	return catalog.Seed{}, false
}

func (a *analyzer) recordSummary(
	seed catalog.Seed,
	values map[string]Value,
	chain []Wrapper,
	target *ssa.Function,
) {
	path := make([]string, 0, len(chain))
	for _, wrapper := range chain {
		path = append(path, wrapper.Symbol.ID)
	}
	functionID := a.functionID(target)
	if len(path) > 0 {
		functionID = path[len(path)-1]
	}
	summary := SemanticSummary{
		FunctionID:  functionID,
		Effect:      string(seed.Effect.Kind),
		FinalSeed:   seed.ID,
		WrapperPath: path,
		Projections: values,
		Certainty:   "static",
		ScenarioID:  a.scenario.ID,
		Provenance: []Provenance{{
			Provider: "go_ssa", Version: AnalyzerVersion,
			Operation: "derive_wrapper_summary", Detail: seed.ID,
		}},
	}
	for _, wrapper := range chain {
		if digest, ok := a.sourceDigest(wrapper.Symbol.Location.Path); ok {
			summary.SourceDependency = append(summary.SourceDependency, digest)
		}
	}
	a.summaryByID[summary.FunctionID+"\x00"+seed.ID] = summary
}

func (a *analyzer) recordGlobalArchitectureAnchors() {
	functions := make([]*ssa.Function, 0)
	for _, function := range a.orderedFunctions() {
		if a.ctx.Err() != nil {
			return
		}
		if function != nil && function.Blocks != nil && a.isRepositoryFunction(function) {
			functions = append(functions, function)
		}
	}
	families := make(map[string][]Symbol)
	for _, function := range functions {
		if a.ctx.Err() != nil {
			return
		}
		name := strings.ToLower(function.Name())
		kind := a.architectureCallKind(function)
		if function.Signature.Recv() != nil && (name == "provision" || name == "validate" || name == "cleanup") {
			kind = "lifecycle_interface"
		}
		if kind != "" && kind != "command_dispatch" && kind != "registry_write" && len(families[kind]) < 32 {
			families[kind] = append(families[kind], a.symbol(function))
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if !ok {
					continue
				}
				for _, target := range a.callTargets[call] {
					if a.architectureCallKind(target) != "registry_write" {
						continue
					}
					location := a.location(call.Pos())
					registryID := a.recordArchitectureAnchor(
						"registry_write",
						"registry write "+a.functionID(target),
						location,
						a.symbol(target),
						"Exact initialization or repository call to a registry-shaped target; later lookup and construction remain separate evidence.",
					)
					extensionID := a.recordArchitectureAnchor(
						"extension_family",
						"extension registration via "+a.functionID(target),
						location,
						a.symbol(target),
						"Registration establishes an extension boundary, not execution or complete implementation coverage.",
					)
					a.recordArchitectureRelationship(registryID, extensionID, location)
				}
			}
		}
	}
	kinds := make([]string, 0, len(families))
	for kind := range families {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		members := deduplicateArchitectureSymbols(families[kind])
		if len(members) > 8 {
			members = members[:8]
		}
		if len(members) == 0 {
			continue
		}
		a.recordArchitectureAnchorMembers(
			kind,
			"discovered "+kind+" family",
			members[0].Location,
			members,
			"Exact build-selected declarations share a bounded architecture-shaped signature; invocation and complete family coverage are not implied.",
		)
	}
}

func (a *analyzer) architectureCallKind(target *ssa.Function) string {
	if target == nil || !a.isRepositoryFunction(target) {
		return ""
	}
	name := strings.ToLower(target.Name())
	qualified := strings.ToLower(a.functionID(target))
	configContext := isConfigurationPackage(functionPackagePath(target))
	registryContext := strings.Contains(qualified, "registr") || strings.Contains(name, "module") ||
		strings.Contains(name, "plugin") || strings.Contains(name, "extension")
	switch {
	case registryContext && strings.HasPrefix(name, "register"):
		return "registry_write"
	case registryContext && (strings.Contains(name, "lookup") || strings.Contains(name, "loadmodule") || strings.Contains(name, "getmodule")):
		return "registry_lookup"
	case configContext && (strings.Contains(name, "adapt") || strings.Contains(name, "convert")):
		return "config_adapter"
	case configContext && (strings.HasPrefix(name, "load") || strings.HasPrefix(name, "read") || strings.HasPrefix(name, "parse") || strings.HasPrefix(name, "decode")):
		return "config_ingress"
	case configContext && (strings.Contains(name, "apply") || strings.Contains(name, "provision")):
		return "config_apply"
	case strings.Contains(qualified, "admin") && (strings.Contains(name, "start") || strings.Contains(name, "serve") || strings.Contains(name, "route") || strings.Contains(name, "handler")):
		return "admin_control_plane"
	case name == "servehttp" || strings.Contains(name, "dispatchrequest"):
		return "request_dispatch_root"
	case strings.Contains(qualified, "tls") || strings.Contains(qualified, "pki") || strings.Contains(qualified, "certificate"):
		return "tls_or_security_boundary"
	case name == "start" || name == "provision" || name == "cleanup":
		return "lifecycle_start"
	case (strings.HasSuffix(functionPackagePath(target), "/cmd") || strings.Contains(functionPackagePath(target), "/cmd/") ||
		strings.HasSuffix(functionPackagePath(target), "/command") || strings.Contains(functionPackagePath(target), "/command/")) &&
		(name == "main" || name == "execute" || strings.HasPrefix(name, "run")):
		return "command_dispatch"
	default:
		return ""
	}
}

func (a *analyzer) shouldFollowComposition(target *ssa.Function, architectureKind string) bool {
	if target == nil || target.Blocks == nil || !a.isRepositoryFunction(target) || target.Name() == "init" {
		return false
	}
	return architectureKind != "request_dispatch_root" && architectureKind != "application_data_plane"
}

func (a *analyzer) recordArchitectureAnchor(
	kind, label string,
	location Location,
	member Symbol,
	limitation string,
) string {
	return a.recordArchitectureAnchorMembersWithProvenance(
		kind, label, location, []Symbol{member}, limitation,
		Provenance{Provider: "go_ssa", Version: AnalyzerVersion, Operation: "classify_architecture_anchor"},
	)
}

func (a *analyzer) recordArchitectureAnchorMembers(
	kind, label string,
	location Location,
	members []Symbol,
	limitation string,
) string {
	return a.recordArchitectureAnchorMembersWithProvenance(
		kind, label, location, members, limitation,
		Provenance{Provider: "go_ssa", Version: AnalyzerVersion, Operation: "classify_architecture_anchor"},
	)
}

func (a *analyzer) recordArchitectureAnchorMembersWithProvenance(
	kind, label string,
	location Location,
	members []Symbol,
	limitation string,
	producer Provenance,
) string {
	if location.Path == "" || location.Line <= 0 {
		return ""
	}
	if len(members) == 0 {
		return ""
	}
	member := members[0]
	identity := locationKey(location)
	if kind == "process_entry" {
		identity = fmt.Sprintf("%s:%d", location.Path, location.Line)
	} else if kind == "registry_write" || kind == "extension_family" {
		identity = member.ID
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"architecture-anchor-v1", kind, identity, member.ID,
	}, "\x00")))
	id := "anchor-" + hex.EncodeToString(digest[:12])
	if _, exists := a.architectureAnchors[id]; exists {
		return id
	}
	if len(a.architectureAnchors) >= maxCollectedArchitectureAnchors {
		a.addBudget("architecture_anchor_collection")
		return ""
	}
	a.architectureAnchors[id] = BehaviorAnchor{
		ID: id, Kind: kind, Label: label, Location: location, Scenario: a.scenario,
		Producer:  producer,
		Certainty: "static", AssociatedMembers: append([]Symbol(nil), members...), Limitations: []string{limitation},
	}
	return id
}

func (a *analyzer) recordArchitectureRelationship(from, to string, location Location) {
	if from == "" || to == "" || from == to {
		return
	}
	kind := a.semanticArchitectureRelationshipKind(from, to)
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"architecture-relationship-v2", from, to, kind,
	}, "\x00")))
	id := "handoff-" + hex.EncodeToString(digest[:12])
	relationship, exists := a.architectureRelationships[id]
	if !exists {
		if len(a.architectureRelationships) >= maxCollectedArchitectureRelationships {
			a.addBudget("architecture_relationship_collection")
			return
		}
		relationship = BehaviorRelationship{
			ID: id, From: from, To: to, Kind: kind, EvidenceKind: "bounded_direct_call", Location: location,
			Certainty: "static", witnessPackages: make(map[string]struct{}),
			Producer: Provenance{Provider: "go_ssa", Version: AnalyzerVersion, Operation: "connect_architecture_anchors"},
		}
	}
	witnessDigest := sha256.Sum256([]byte(strings.Join([]string{
		"architecture-relationship-witness-v1", from, to, locationKey(location),
	}, "\x00")))
	witnessID := "witness-" + hex.EncodeToString(witnessDigest[:12])
	if len(relationship.WitnessIDs) < maxArchitectureRelationshipWitnesses &&
		!stringSliceContains(relationship.WitnessIDs, witnessID) {
		relationship.WitnessIDs = append(relationship.WitnessIDs, witnessID)
		relationship.WitnessCount = len(relationship.WitnessIDs)
	}
	if len(relationship.RepresentativeLocations) < 8 {
		relationship.RepresentativeLocations = append(relationship.RepresentativeLocations, location)
	}
	relationship.witnessPackages[filepath.Dir(location.Path)] = struct{}{}
	relationship.PackageCount = len(relationship.witnessPackages)
	a.architectureRelationships[id] = relationship
}

func (a *analyzer) semanticArchitectureRelationshipKind(from, to string) string {
	fromAnchor, fromExists := a.architectureAnchors[from]
	toAnchor, toExists := a.architectureAnchors[to]
	if !fromExists || !toExists {
		return "static_call_supporting_relation"
	}
	switch {
	case fromAnchor.Kind == "process_entry" && toAnchor.Kind == "command_dispatch":
		return "dispatches_to"
	case fromAnchor.Kind == "registry_write" && toAnchor.Kind == "extension_family":
		return "registers_extension_family"
	case isConfigAnchorKind(fromAnchor.Kind) && isConfigAnchorKind(toAnchor.Kind):
		return "loads_or_adapts_config"
	case fromAnchor.Kind == "lifecycle_interface" && toAnchor.Kind == "lifecycle_start":
		return "starts_lifecycle"
	case toAnchor.Kind == "admin_control_plane":
		return "exposes_admin_control_plane"
	case toAnchor.Kind == "request_dispatch_root":
		return "dispatches_http_request"
	case toAnchor.Kind == "tls_or_security_boundary":
		return "configures_security_boundary"
	default:
		return "static_call_supporting_relation"
	}
}

func isConfigAnchorKind(kind string) bool {
	return kind == "config_ingress" || kind == "config_adapter" || kind == "config_apply"
}

func isConfigurationPackage(packagePath string) bool {
	segments := strings.FieldsFunc(strings.ToLower(packagePath), func(char rune) bool { return char == '/' || char == '.' })
	for _, segment := range segments {
		if segment == "config" || segment == "configuration" || segment == "caddyfile" || strings.HasSuffix(segment, "config") {
			return true
		}
	}
	return false
}

func deduplicateArchitectureSymbols(symbols []Symbol) []Symbol {
	byDeclaration := make(map[string][]Symbol)
	for _, symbol := range symbols {
		key := strings.Join([]string{
			symbol.Package, symbol.Name, symbol.Location.Path,
			strconv.Itoa(symbol.Location.Line), strconv.Itoa(symbol.Location.Column),
		}, "\x00")
		byDeclaration[key] = append(byDeclaration[key], symbol)
	}
	result := make([]Symbol, 0, len(byDeclaration))
	for _, equivalents := range byDeclaration {
		sort.Slice(equivalents, func(i, j int) bool { return equivalents[i].ID < equivalents[j].ID })
		canonical := equivalents[0]
		seen := make(map[string]struct{}, len(equivalents))
		for _, equivalent := range equivalents {
			if _, duplicate := seen[equivalent.ID]; duplicate {
				continue
			}
			seen[equivalent.ID] = struct{}{}
			canonical.EquivalentIDs = append(canonical.EquivalentIDs, equivalent.ID)
		}
		result = append(result, canonical)
	}
	sort.Slice(result, func(i, j int) bool {
		if locationKey(result[i].Location) != locationKey(result[j].Location) {
			return locationKey(result[i].Location) < locationKey(result[j].Location)
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func (a *analyzer) finishArchitectureGrounding(entrypoints []*ssa.Function) {
	anchors := make([]BehaviorAnchor, 0, len(a.architectureAnchors))
	for _, anchor := range a.architectureAnchors {
		if a.ctx.Err() != nil {
			return
		}
		anchors = append(anchors, anchor)
	}
	relationships := make([]BehaviorRelationship, 0, len(a.architectureRelationships))
	for _, relationship := range a.architectureRelationships {
		if a.ctx.Err() != nil {
			return
		}
		sort.Strings(relationship.WitnessIDs)
		sort.Slice(relationship.RepresentativeLocations, func(i, j int) bool {
			return locationKey(relationship.RepresentativeLocations[i]) < locationKey(relationship.RepresentativeLocations[j])
		})
		relationship.witnessPackages = nil
		relationships = append(relationships, relationship)
	}
	var groundingBounded bool
	anchors, relationships, groundingBounded = boundArchitectureGrounding(anchors, relationships)
	if groundingBounded {
		a.addBudget("architecture_anchors")
	}
	kinds := make(map[string]bool)
	for _, anchor := range anchors {
		kinds[anchor.Kind] = true
	}
	anchorKindByID := make(map[string]string, len(anchors))
	reachable := make(map[string]bool)
	queue := make([]string, 0)
	for _, anchor := range anchors {
		if a.ctx.Err() != nil {
			return
		}
		anchorKindByID[anchor.ID] = anchor.Kind
		if anchor.Kind == "process_entry" {
			reachable[anchor.ID] = true
			queue = append(queue, anchor.ID)
		}
	}
	for len(queue) > 0 {
		if a.ctx.Err() != nil {
			return
		}
		current := queue[0]
		queue = queue[1:]
		for _, relationship := range relationships {
			if relationship.From != current || reachable[relationship.To] {
				continue
			}
			reachable[relationship.To] = true
			queue = append(queue, relationship.To)
		}
	}
	reachableKinds := make(map[string]bool)
	for anchorID := range reachable {
		if a.ctx.Err() != nil {
			return
		}
		reachableKinds[anchorKindByID[anchorID]] = true
	}

	pillars := 0
	reachablePillars := 0
	groups := [][]string{
		{"command_dispatch"},
		{"config_ingress", "config_adapter", "config_apply"},
		{"registry_write", "registry_lookup", "extension_family"},
		{"lifecycle_interface", "lifecycle_start"},
		{"admin_control_plane"},
		{"request_dispatch_root", "application_data_plane"},
		{"tls_or_security_boundary"},
	}
	for _, group := range groups {
		if a.ctx.Err() != nil {
			return
		}
		for _, kind := range group {
			if kinds[kind] {
				pillars++
				break
			}
		}
		for _, kind := range group {
			if reachableKinds[kind] {
				reachablePillars++
				break
			}
		}
	}
	mode := "package_landscape"
	if kinds["process_entry"] && reachablePillars >= 4 {
		mode = "behavior_grounded"
	} else if kinds["process_entry"] && pillars >= 2 {
		mode = "mixed"
	}

	processEntryCount := 0
	for _, anchor := range anchors {
		if anchor.Kind == "process_entry" {
			processEntryCount++
		}
	}
	archetype := "application"
	evidenceItems := []string{fmt.Sprintf(
		"%d exact build-selected process entrypoint(s); %d available to typed analysis",
		processEntryCount,
		len(entrypoints),
	)}
	alternatives := []string{}
	switch {
	case (kinds["registry_write"] || kinds["extension_family"]) &&
		(kinds["request_dispatch_root"] || kinds["admin_control_plane"]):
		archetype = "modular_platform_server"
		evidenceItems = append(evidenceItems, "exact registry/extension and server/control-plane anchors")
		alternatives = append(alternatives, "application")
	case a.result.Coverage.Workers > 0 || a.result.Coverage.AsyncTasks > 1:
		archetype = "daemon_worker_system"
		evidenceItems = append(evidenceItems, "bounded worker or asynchronous task registrations")
		alternatives = append(alternatives, "application")
	case kinds["command_dispatch"] && !kinds["request_dispatch_root"]:
		archetype = "cli_tool"
		evidenceItems = append(evidenceItems, "exact process and command-dispatch anchors")
		alternatives = append(alternatives, "application")
	case processEntryCount == 0:
		archetype = "library_framework"
		evidenceItems = append(evidenceItems, "no build-selected process entrypoint")
	case processEntryCount > 3:
		archetype = "monorepo_mixed"
		evidenceItems = append(evidenceItems, "several build-selected executable entrypoints")
		alternatives = append(alternatives, "application")
	default:
		alternatives = append(alternatives, "cli_tool", "daemon_worker_system")
	}
	a.result.Grounding = ArchitectureGrounding{
		Version:             ArchitectureGroundingVersion,
		RepositoryArchetype: ArchetypeAssessment{Selected: archetype, Evidence: evidenceItems, Alternatives: alternatives},
		GroundingMode:       mode, Anchors: anchors, Relationships: relationships,
	}
	if a.ctx.Err() != nil {
		return
	}
	a.result.normalize()
}

const (
	maxCollectedArchitectureAnchors       = 1024
	maxCollectedArchitectureRelationships = 4096
	maxPersistedArchitectureAnchors       = 256
	maxPersistedArchitectureRelationships = 512
	maxArchitectureAnchorsPerKind         = 16
	maxProcessEntryArchitectureAnchors    = 64
	maxArchitectureRelationshipWitnesses  = 64
)

func boundArchitectureGrounding(
	anchors []BehaviorAnchor,
	relationships []BehaviorRelationship,
) ([]BehaviorAnchor, []BehaviorRelationship, bool) {
	reachable := architectureReachableAnchorIDs(anchors, relationships)
	sort.Slice(anchors, func(i, j int) bool {
		leftPriority := architectureAnchorRetentionPriority(anchors[i], reachable)
		rightPriority := architectureAnchorRetentionPriority(anchors[j], reachable)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if anchors[i].Kind != anchors[j].Kind {
			return anchors[i].Kind < anchors[j].Kind
		}
		leftLocation := locationKey(anchors[i].Location)
		rightLocation := locationKey(anchors[j].Location)
		if leftLocation != rightLocation {
			return leftLocation < rightLocation
		}
		return anchors[i].ID < anchors[j].ID
	})
	counts := make(map[string]int)
	retainedIDs := make(map[string]struct{})
	retained := make([]BehaviorAnchor, 0, len(anchors))
	bounded := false
	for _, anchor := range anchors {
		limit := maxArchitectureAnchorsPerKind
		if anchor.Kind == "process_entry" {
			limit = maxProcessEntryArchitectureAnchors
		}
		if len(retained) >= maxPersistedArchitectureAnchors || counts[anchor.Kind] >= limit {
			bounded = true
			continue
		}
		counts[anchor.Kind]++
		retainedIDs[anchor.ID] = struct{}{}
		retained = append(retained, anchor)
	}
	filteredRelationships := make([]BehaviorRelationship, 0, len(relationships))
	for _, relationship := range relationships {
		_, fromRetained := retainedIDs[relationship.From]
		_, toRetained := retainedIDs[relationship.To]
		if !fromRetained || !toRetained {
			bounded = true
			continue
		}
		filteredRelationships = append(filteredRelationships, relationship)
	}
	sort.Slice(filteredRelationships, func(i, j int) bool {
		leftReachable := reachable[filteredRelationships[i].From] && reachable[filteredRelationships[i].To]
		rightReachable := reachable[filteredRelationships[j].From] && reachable[filteredRelationships[j].To]
		if leftReachable != rightReachable {
			return leftReachable
		}
		return filteredRelationships[i].ID < filteredRelationships[j].ID
	})
	if len(filteredRelationships) > maxPersistedArchitectureRelationships {
		filteredRelationships = filteredRelationships[:maxPersistedArchitectureRelationships]
		bounded = true
	}
	return retained, filteredRelationships, bounded
}

func architectureReachableAnchorIDs(
	anchors []BehaviorAnchor,
	relationships []BehaviorRelationship,
) map[string]bool {
	known := make(map[string]struct{}, len(anchors))
	reachable := make(map[string]bool)
	queue := make([]string, 0)
	for _, anchor := range anchors {
		known[anchor.ID] = struct{}{}
		if anchor.Kind == "process_entry" {
			reachable[anchor.ID] = true
			queue = append(queue, anchor.ID)
		}
	}
	outgoing := make(map[string][]string)
	for _, relationship := range relationships {
		if _, fromExists := known[relationship.From]; !fromExists {
			continue
		}
		if _, toExists := known[relationship.To]; !toExists {
			continue
		}
		outgoing[relationship.From] = append(outgoing[relationship.From], relationship.To)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range outgoing[current] {
			if reachable[next] {
				continue
			}
			reachable[next] = true
			queue = append(queue, next)
		}
	}
	return reachable
}

func architectureAnchorRetentionPriority(anchor BehaviorAnchor, reachable map[string]bool) int {
	if anchor.Kind == "process_entry" {
		return 0
	}
	if reachable[anchor.ID] {
		return 1
	}
	return 2
}

func stringSliceContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (a *analyzer) finish(latency time.Duration) {
	for index := range a.result.Catalog.Triggers {
		trigger := &a.result.Catalog.Triggers[index]
		for startIndex := range a.starts {
			start := &a.starts[startIndex]
			dispatcher := a.startDispatcher(*start)
			if dispatcher.Text != "" && valueAddressKey(dispatcher) == valueAddressKey(trigger.Dispatcher) {
				location := start.location
				trigger.ServerStartSite = &location
				trigger.Evidence = append(trigger.Evidence, Evidence{
					ID: "server-start:" + locationKey(location), Kind: "server_start_call",
					Location: location, Detail: dispatcher.Text,
				})
				start.matched = true
				break
			}
		}
	}
	for _, start := range a.starts {
		a.recordServerStart(start)
	}
	a.result.Catalog.Triggers = deduplicateTriggerRecords(a.result.Catalog.Triggers)
	for index := range a.result.Catalog.Triggers {
		trigger := &a.result.Catalog.Triggers[index]
		a.annotateTriggerOwnership(trigger)
		deriveSurfaceSemantics(trigger)
	}
	for _, summary := range a.summaryByID {
		a.result.Summaries = append(a.result.Summaries, summary)
	}
	repository := Repository{Root: a.root, ModulePath: a.modulePath}
	a.result.Catalog.Version = TriggerCatalogVersion
	a.result.Catalog.AnalyzerVersion = AnalyzerVersion
	a.result.Catalog.CatalogVersion = CatalogVersion
	a.result.Catalog.Repository = repository
	a.result.Catalog.Scenario = a.scenario
	a.result.Coverage.Version = CoverageVersion
	a.result.Coverage.Repository = repository
	a.result.Coverage.Scenario = a.scenario
	a.result.Coverage.ConfiguredSeedsMatched = append([]string{}, a.matchedSeeds...)
	a.result.Coverage.PackagesInspected = len(a.packages)
	a.result.Coverage.DispatchRootsFound = len(a.starts)
	a.result.Coverage.ColdLatencyMillis = latency.Milliseconds()
	a.result.Coverage.BuildConstraints = append([]string{}, a.opts.BuildTags...)
	a.result.Coverage.ScopeStatement = "exact build-selected process entries plus runtime registrations and starts found through safe typed package closures and bounded wrapper propagation under the recorded build scenario, subject to listed diagnostics and frontiers"
	for _, trigger := range a.result.Catalog.Triggers {
		if len(trigger.WrapperChain) == 0 {
			a.result.Coverage.DirectTriggers++
		} else {
			a.result.Coverage.WrapperDerivedTriggers++
		}
		if trigger.Kind == "process_entry" {
			a.result.Coverage.ProcessEntries++
			if trigger.Availability == AvailabilityUnavailable {
				a.result.Coverage.UnavailableProcessEntries++
			} else {
				a.result.Coverage.AvailableProcessEntries++
			}
		}
		if trigger.Kind != "process_entry" && !trigger.Handler.Known {
			a.result.Coverage.UnresolvedHandlers++
		}
		switch trigger.Kind {
		case "worker":
			a.result.Coverage.Workers++
		case "async_task":
			a.result.Coverage.AsyncTasks++
		}
		if trigger.Kind == "http_route" && trigger.Resolution != "exact" {
			a.result.Coverage.PossibleRegistrations++
		}
	}
	a.result.normalize()
}

func deduplicateTriggerRecords(records []TriggerRecord) []TriggerRecord {
	result := make([]TriggerRecord, 0, len(records))
	indexByID := make(map[string]int, len(records))
	for _, record := range records {
		index, duplicate := indexByID[record.ID]
		if !duplicate {
			indexByID[record.ID] = len(result)
			result = append(result, record)
			continue
		}
		existing := &result[index]
		if preferTriggerRecord(record, *existing) {
			record.Evidence = append(record.Evidence, existing.Evidence...)
			record.Provenance = append(record.Provenance, existing.Provenance...)
			*existing = record
		} else {
			existing.Evidence = append(existing.Evidence, record.Evidence...)
			existing.Provenance = append(existing.Provenance, record.Provenance...)
		}
		existing.Evidence = compactEvidence(existing.Evidence)
		existing.Provenance = compactProvenance(existing.Provenance)
	}
	return result
}

func preferTriggerRecord(candidate, existing TriggerRecord) bool {
	rank := func(resolution string) int {
		switch resolution {
		case "exact":
			return 0
		case "partial":
			return 1
		case "ambiguous":
			return 2
		default:
			return 3
		}
	}
	if rank(candidate.Resolution) != rank(existing.Resolution) {
		return rank(candidate.Resolution) < rank(existing.Resolution)
	}
	candidateKnown := boolInt(candidate.Identity.Path.Known) + boolInt(candidate.Dispatcher.Known) + boolInt(candidate.Handler.Known)
	existingKnown := boolInt(existing.Identity.Path.Known) + boolInt(existing.Dispatcher.Known) + boolInt(existing.Handler.Known)
	if candidateKnown != existingKnown {
		return candidateKnown > existingKnown
	}
	if candidate.ProvisionalID != existing.ProvisionalID {
		return !candidate.ProvisionalID
	}
	if len(candidate.WrapperChain) != len(existing.WrapperChain) {
		return len(candidate.WrapperChain) < len(existing.WrapperChain)
	}
	return triggerRecordOrderKey(candidate) < triggerRecordOrderKey(existing)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func triggerRecordOrderKey(record TriggerRecord) string {
	wrappers := make([]string, 0, len(record.WrapperChain))
	for _, wrapper := range record.WrapperChain {
		wrappers = append(wrappers, wrapper.Symbol.ID+"@"+locationKey(wrapper.Callsite))
	}
	frontiers := make([]string, 0, len(record.DynamicFrontier))
	for _, frontier := range record.DynamicFrontier {
		frontiers = append(frontiers, frontier.Kind+"="+frontier.Detail)
	}
	return strings.Join([]string{
		record.Status,
		record.Dispatcher.Text,
		record.Handler.Text,
		strings.Join(wrappers, "|"),
		strings.Join(frontiers, "|"),
	}, "\x00")
}

func compactEvidence(input []Evidence) []Evidence {
	result := make([]Evidence, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		key := item.ID + "\x00" + item.Kind + "\x00" + locationKey(item.Location) + "\x00" + item.Detail
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func compactProvenance(input []Provenance) []Provenance {
	result := make([]Provenance, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		key := item.Provider + "\x00" + item.Version + "\x00" + item.Operation + "\x00" + item.Detail
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func (a *analyzer) startDispatcher(start dispatchStart) Value {
	dispatcher := start.values["dispatcher"]
	server := start.values["server"]
	if (!dispatcher.Known || dispatcher.Text == "") && server.Text != "" {
		if assigned := a.assignments[valueAddressKey(server)]; assigned.Text != "" {
			dispatcher = assigned
		}
	}
	return dispatcher
}

func (a *analyzer) recordServerStart(start dispatchStart) {
	dispatcher := a.startDispatcher(start)
	identity := firstKnownValue(
		start.values["address"],
		start.values["listener"],
		start.values["server"],
	)
	frontiers := append([]Frontier(nil), start.frontiers...)
	if !start.matched {
		frontiers = append(frontiers, Frontier{
			Kind:     "unresolved_dispatch_inventory",
			Detail:   "No supported route registration was correlated with this static server start call.",
			Location: &start.location,
		})
	}
	status := "confirmed_server_start_call"
	resolution := "exact"
	if len(frontiers) > 0 || start.ambiguous || !dispatcher.Known || !identity.Known {
		resolution = "partial"
	}
	basis := string(catalog.OriginCatalogStatic)
	if len(start.chain) > 0 {
		basis = string(catalog.OriginWrapperStatic)
	}
	location := start.location
	record := TriggerRecord{
		Kind: "http_server", Identity: Identity{Name: "HTTP server", Path: identity},
		Transport: start.seed.Effect.Transport, Framework: start.seed.Effect.Framework,
		ProcessEntrypoint: a.symbol(start.entrypoint), Dispatcher: dispatcher,
		RegistrationSite: location, ServerStartSite: &location, Handler: dispatcher,
		Middleware: []Value{}, WrapperChain: append([]Wrapper(nil), start.chain...),
		FinalSeed: start.seed.ID, DiscoveryBasis: basis, Certainty: "static",
		Resolution: resolution, ScenarioID: a.scenario.ID,
		Evidence: []Evidence{{
			ID: "server-start:" + locationKey(location), Kind: "server_start_call",
			Location: location, Detail: start.seed.ID,
		}},
		Provenance: []Provenance{{
			Provider: "go_ssa", Version: AnalyzerVersion,
			Operation: "propagate_terminal_semantics", Detail: start.seed.ID,
		}},
		DynamicFrontier: frontiers, Status: status,
	}
	record.TerminalSourceScope, record.ApplicationClass, record.PromotionBasis =
		classifyTerminalOwnership(location, start.chain, start.detached)
	// A server-start record is identified by its exact static call site. Handler
	// and listener resolution are supporting facts and may remain bounded without
	// making that call-site identity provisional.
	record.ProvisionalID = false
	record.ID = stableTriggerID(record)
	a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
	a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontiers...)
}

func firstKnownValue(values ...Value) Value {
	for _, value := range values {
		if value.Known && value.Text != "" {
			return value
		}
	}
	for _, value := range values {
		if value.Text != "" {
			return value
		}
	}
	return dynamicValue("unknown server identity")
}

func (a *analyzer) symbol(function *ssa.Function) Symbol {
	return Symbol{
		ID:       a.functionID(function),
		Package:  functionPackagePath(function),
		Name:     function.Name(),
		Location: a.location(function.Pos()),
	}
}

func (a *analyzer) functionID(function *ssa.Function) string {
	if function == nil {
		return "unknown"
	}
	path := functionPackagePath(function)
	if path == "" {
		path = "synthetic"
	}
	receiver := receiverName(function.Signature)
	if receiver != "" {
		return path + ".(" + receiver + ")." + function.Name()
	}
	return path + "." + function.Name()
}

func (a *analyzer) location(position token.Pos) Location {
	resolved := a.program.Fset.PositionFor(position, true)
	path := resolved.Filename
	if relative, err := filepath.Rel(a.root, path); err == nil && !strings.HasPrefix(relative, "..") {
		path = filepath.ToSlash(relative)
	}
	return Location{Path: path, Line: resolved.Line, Column: resolved.Column}
}

func (a *analyzer) sourceLocation(filename string, line, column int) Location {
	path := filename
	if relative, err := filepath.Rel(a.root, filename); err == nil && !strings.HasPrefix(relative, "..") {
		path = filepath.ToSlash(relative)
	}
	return Location{Path: path, Line: line, Column: column}
}

func (a *analyzer) valueIdentity(value ssa.Value) string {
	location := a.location(value.Pos())
	if filepath.IsAbs(location.Path) {
		location.Path = "<external>/" + filepath.Base(location.Path)
	}
	return types.TypeString(value.Type(), packageQualifier) + "@" + locationKey(location)
}

func (a *analyzer) wrapperOrigin(function *ssa.Function) string {
	if a.isRepositoryFunction(function) {
		return "repository"
	}
	return "library"
}

func (a *analyzer) isRepositoryFunction(function *ssa.Function) bool {
	path := functionPackagePath(function)
	return a.modulePath != "" && (path == a.modulePath || strings.HasPrefix(path, a.modulePath+"/"))
}

func (a *analyzer) addBudget(name string) {
	a.result.Coverage.BudgetsReached = append(a.result.Coverage.BudgetsReached, name)
}

func (a *analyzer) sourceDigest(path string) (SourceDigest, bool) {
	if cached, ok := a.fileDigests[path]; ok {
		return cached, true
	}
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(a.root, filepath.FromSlash(path))
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return SourceDigest{}, false
	}
	digest := sha256.Sum256(data)
	result := SourceDigest{Path: path, SHA256: hex.EncodeToString(digest[:])}
	a.fileDigests[path] = result
	return result, true
}

func knownValue(kind, text string) Value {
	return Value{Kind: kind, Text: text, Known: text != "", Candidates: []string{}}
}

func valueAddressKey(value Value) string {
	if value.addressKey != "" {
		return value.addressKey
	}
	return value.Text
}

func contextualAddressKey(base string, env environment) string {
	if base == "" || len(env) == 0 {
		return base
	}
	parts := make([]string, 0, len(env))
	for variable, value := range env {
		parts = append(parts, variable.Name()+"="+value.Text+"#"+value.addressKey)
	}
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return base + "#" + hex.EncodeToString(digest[:8])
}

func dynamicValue(text string) Value {
	return Value{Kind: "unknown", Text: text, Known: false, Candidates: []string{}}
}

func mergeValues(values []Value) Value {
	if len(values) == 0 {
		return dynamicValue("no values")
	}
	first := values[0]
	candidates := []string{}
	for _, value := range values {
		if value.Text != "" {
			candidates = append(candidates, value.Text)
		}
		if value.Text != first.Text || value.Known != first.Known {
			first.Known = false
			first.Kind = "alternatives"
		}
		if value.addressKey != first.addressKey {
			first.addressKey = ""
		}
	}
	sort.Strings(candidates)
	first.Candidates = compactStrings(candidates)
	if len(first.Candidates) > 1 {
		first.Text = strings.Join(first.Candidates, " | ")
	}
	return first
}

func scenarioID(goos, goarch string, tags []string) string {
	copyTags := append([]string{}, tags...)
	sort.Strings(copyTags)
	return "go:" + goos + "/" + goarch + ":tags=" + strings.Join(copyTags, ",")
}

func packageQualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Path()
}

func packagePath(pkg *ssa.Package) string {
	if pkg == nil || pkg.Pkg == nil {
		return ""
	}
	return pkg.Pkg.Path()
}

func functionPackagePath(function *ssa.Function) string {
	if function == nil {
		return ""
	}
	if object := function.Object(); object != nil && object.Pkg() != nil {
		return object.Pkg().Path()
	}
	return packagePath(function.Pkg)
}

func receiverName(signature *types.Signature) string {
	if signature == nil || signature.Recv() == nil {
		return ""
	}
	_, receiver := receiverTypeIdentity(signature.Recv().Type())
	return receiver
}

func receiverTypeIdentity(value types.Type) (string, string) {
	pointer := false
	if typed, ok := value.(*types.Pointer); ok {
		pointer = true
		value = typed.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return "", ""
	}
	name := named.Obj().Name()
	if pointer {
		name = "*" + name
	}
	return named.Obj().Pkg().Path(), name
}

func fieldName(value types.Type, index int) string {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if ok {
		value = named.Underlying()
	}
	structure, ok := value.(*types.Struct)
	if !ok || index < 0 || index >= structure.NumFields() {
		return ""
	}
	return structure.Field(index).Name()
}

func cleanFunctionID(id string) string {
	return strings.TrimSuffix(strings.TrimSuffix(id, "$bound"), "$thunk")
}

func locationKey(location Location) string {
	return location.Path + ":" + strconv.Itoa(location.Line) + ":" + strconv.Itoa(location.Column)
}
