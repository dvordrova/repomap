package surfacediscovery

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	opts         Options
	catalog      catalog.Catalog
	program      *ssa.Program
	packages     []*ssa.Package
	graph        *callgraph.Graph
	allFunctions map[*ssa.Function]bool
	relevant     map[*ssa.Function]bool
	callTargets  map[ssa.CallInstruction][]*ssa.Function
	root         string
	modulePath   string
	scenario     Scenario
	result       Result
	tasks        int
	active       map[*ssa.Function]bool
	matchedSeeds []string
	starts       []dispatchStart
	assignments  map[string]Value
	summaryByID  map[string]SemanticSummary
	fileDigests  map[string]SourceDigest
	functionByID map[string]*ssa.Function
	loopCache    map[*ssa.Function][]loopDescriptor
	loopSeen     map[string]bool
}

type environment map[ssa.Value]Value

type dispatchStart struct {
	dispatcher Value
	server     Value
	location   Location
}

func Analyze(opts Options) (Result, error) {
	started := time.Now()
	if strings.TrimSpace(opts.RepoPath) == "" {
		return Result{}, fmt.Errorf("surface discovery: repository path is required")
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 16
	}
	if opts.MaxTasks <= 0 {
		opts.MaxTasks = 1000
	}
	if opts.MaxTargets <= 0 {
		opts.MaxTargets = 8
	}
	root, err := filepath.Abs(opts.RepoPath)
	if err != nil {
		return Result{}, fmt.Errorf("surface discovery: resolve repository: %w", err)
	}
	builtin, err := catalog.Builtin()
	if err != nil {
		return Result{}, err
	}
	a := &analyzer{
		opts:         opts,
		catalog:      builtin,
		root:         root,
		active:       map[*ssa.Function]bool{},
		assignments:  map[string]Value{},
		summaryByID:  map[string]SemanticSummary{},
		fileDigests:  map[string]SourceDigest{},
		callTargets:  map[ssa.CallInstruction][]*ssa.Function{},
		functionByID: map[string]*ssa.Function{},
		loopCache:    map[*ssa.Function][]loopDescriptor{},
		loopSeen:     map[string]bool{},
	}
	if err := a.load(); err != nil {
		return Result{}, err
	}
	a.prepare()
	entrypoints := a.entrypoints()
	for _, entrypoint := range entrypoints {
		a.result.Coverage.EntrypointsConsidered = append(
			a.result.Coverage.EntrypointsConsidered,
			a.symbol(entrypoint),
		)
		a.walk(entrypoint, environment{}, nil, entrypoint, 0, false)
	}
	a.finish(time.Since(started))
	return a.result, nil
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
		Dir: a.root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes |
			packages.NeedModule,
		BuildFlags: buildFlags,
		Tests:      false,
	}
	loaded, err := packages.Load(config, "./...")
	if err != nil {
		return fmt.Errorf("surface discovery: load packages: %w", err)
	}
	if err := surfacePackageLoadError(loaded); err != nil {
		return err
	}
	if len(loaded) == 0 {
		return fmt.Errorf("surface discovery: no build-selected Go packages")
	}
	for _, pkg := range loaded {
		if pkg.Module != nil && pkg.Module.Main {
			a.modulePath = pkg.Module.Path
			break
		}
	}
	a.program, a.packages = ssautil.AllPackages(loaded, ssa.InstantiateGenerics)
	a.program.Build()
	a.allFunctions = ssautil.AllFunctions(a.program)
	a.graph = cha.CallGraph(a.program)
	a.scenario = Scenario{
		ID:     scenarioID(runtime.GOOS, runtime.GOARCH, a.opts.BuildTags),
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		Tags:   append([]string{}, a.opts.BuildTags...),
	}
	return nil
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

func surfacePackageLoadError(loaded []*packages.Package) error {
	errorCount := 0
	firstMessage := ""
	packages.Visit(loaded, func(pkg *packages.Package) bool {
		for _, packageError := range pkg.Errors {
			errorCount++
			message := strings.Join(strings.Fields(packageError.Error()), " ")
			if firstMessage == "" || message < firstMessage {
				firstMessage = message
			}
		}
		return true
	}, nil)
	if errorCount == 0 {
		return nil
	}
	if firstMessage == "" {
		firstMessage = "details unavailable"
	}
	return fmt.Errorf(
		"surface discovery: package loading failed with %d error(s); first: %s",
		errorCount,
		firstMessage,
	)
}

func (a *analyzer) prepare() {
	a.relevant = map[*ssa.Function]bool{}
	for function := range a.allFunctions {
		if function == nil || function.Blocks == nil {
			continue
		}
		a.functionByID[a.functionID(function)] = function
		a.functionByID[cleanFunctionID(a.functionID(function))] = function
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if ok {
					targets := a.targets(call)
					a.callTargets[call] = targets
					for _, target := range targets {
						if _, matched := a.callSeed(target); matched {
							a.relevant[function] = true
						}
					}
				}
				if store, ok := instruction.(*ssa.Store); ok {
					if _, matched := a.fieldSeed(store); matched {
						a.relevant[function] = true
					}
				}
			}
		}
	}

	changed := true
	for changed {
		changed = false
		for function := range a.allFunctions {
			if function == nil || function.Blocks == nil || a.relevant[function] {
				continue
			}
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(ssa.CallInstruction)
					if !ok {
						continue
					}
					for _, target := range a.callTargets[call] {
						if a.relevant[target] {
							a.relevant[function] = true
							changed = true
							break
						}
					}
				}
			}
		}
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

func (a *analyzer) walk(
	function *ssa.Function,
	env environment,
	chain []Wrapper,
	entrypoint *ssa.Function,
	depth int,
	ambiguous bool,
) {
	if function == nil || function.Blocks == nil {
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
	a.active[function] = true
	defer delete(a.active, function)
	a.result.Coverage.FunctionsInspected++

	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if store, ok := instruction.(*ssa.Store); ok {
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
			if len(targets) > a.opts.MaxTargets {
				a.addBudget("targets")
				targets = targets[:a.opts.MaxTargets]
				ambiguous = true
			}
			callAmbiguous := ambiguous || len(targets) > 1
			for _, target := range targets {
				seed, matched := a.callSeed(target)
				if matched {
					a.recordCall(seed, call, target, env, chain, entrypoint, callAmbiguous)
					continue
				}
				if !a.relevant[target] {
					continue
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
				)
			}
		}
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
			dispatcher: values["dispatcher"],
			server:     values["server"],
			location:   location,
		})
	case catalog.EffectHTTPRouteRegistration:
		loopSignal, inLoop := a.registrationLoop(call, seed)
		if inLoop {
			a.addLoopSignal(loopSignal)
		}
		a.recordRoute(seed, values, location, chain, entrypoint, ambiguous)
	case catalog.EffectAsyncTaskStart:
		a.recordAsyncTask(seed, values, location, chain, entrypoint, ambiguous)
	}
	a.recordSummary(seed, values, chain, target)
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
	handler := values["handler"]
	dispatcher := values["dispatcher"]
	method := values["method"].Text
	middleware := []Value{}
	if len(handler.Candidates) > 1 {
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
	record.ProvisionalID = !pathValue.Known || !handler.Known || !dispatcher.Known
	record.ID = stableTriggerID(record)
	a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
	a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontiers...)
}

func (a *analyzer) recordFieldStore(seed catalog.Seed, store *ssa.Store, env environment) {
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return
	}
	server := a.eval(field.X, env, 0)
	dispatcher := a.eval(store.Val, env, 0)
	if server.Text != "" {
		a.assignments[server.Text] = dispatcher
	}
	a.matchedSeeds = append(a.matchedSeeds, seed.ID)
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
		return knownValue("allocation", a.valueIdentity(current))
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
			if field, ok := current.X.(*ssa.FieldAddr); ok {
				base := a.eval(field.X, env, depth+1)
				return dynamicValue(base.Text + "." + fieldName(field.X.Type(), field.Field))
			}
		}
		return a.eval(current.X, env, depth+1)
	case *ssa.FieldAddr:
		base := a.eval(current.X, env, depth+1)
		return knownValue("field", base.Text+"."+fieldName(current.X.Type(), current.Field))
	case *ssa.Field:
		base := a.eval(current.X, env, depth+1)
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
		return knownValue("dispatcher", "net/http.NewServeMux@"+locationKey(a.location(call.Pos())))
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

func (a *analyzer) finish(latency time.Duration) {
	for index := range a.result.Catalog.Triggers {
		trigger := &a.result.Catalog.Triggers[index]
		for _, start := range a.starts {
			dispatcher := start.dispatcher
			if dispatcher.Text == "" && start.server.Text != "" {
				dispatcher = a.assignments[start.server.Text]
			}
			if dispatcher.Text != "" && dispatcher.Text == trigger.Dispatcher.Text {
				location := start.location
				trigger.ServerStartSite = &location
				trigger.Evidence = append(trigger.Evidence, Evidence{
					ID: "server-start:" + locationKey(location), Kind: "server_start_call",
					Location: location, Detail: dispatcher.Text,
				})
				break
			}
		}
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
	a.result.Coverage.ScopeStatement = "runtime registrations and starts found through configured terminal seeds and bounded wrapper propagation under the recorded build scenario, subject to listed frontiers"
	for _, trigger := range a.result.Catalog.Triggers {
		if len(trigger.WrapperChain) == 0 {
			a.result.Coverage.DirectTriggers++
		} else {
			a.result.Coverage.WrapperDerivedTriggers++
		}
		if !trigger.Handler.Known {
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

func (a *analyzer) valueIdentity(value ssa.Value) string {
	return types.TypeString(value.Type(), packageQualifier) + "@" + locationKey(a.location(value.Pos()))
}

func (a *analyzer) wrapperOrigin(function *ssa.Function) string {
	path := functionPackagePath(function)
	if a.modulePath != "" && (path == a.modulePath || strings.HasPrefix(path, a.modulePath+"/")) {
		return "repository"
	}
	return "library"
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
