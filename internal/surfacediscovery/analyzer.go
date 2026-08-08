package surfacediscovery

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"go/version"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/semantics/catalog"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type analyzer struct {
	ctx                                       context.Context
	opts                                      Options
	input                                     Input
	processEntrypoints                        []processEntrypoint
	catalog                                   catalog.Catalog
	program                                   *ssa.Program
	packages                                  []*ssa.Package
	packageFacts                              map[string]*packages.Package
	graph                                     *callgraph.Graph
	allFunctions                              map[*ssa.Function]bool
	relevant                                  map[*ssa.Function]bool
	relevanceDistance                         map[*ssa.Function]int
	callTargets                               map[ssa.CallInstruction][]*ssa.Function
	root                                      string
	modulePath                                string
	modulePaths                               map[string]bool
	scenario                                  Scenario
	result                                    Result
	tasks                                     int
	active                                    map[*ssa.Function]bool
	matchedSeeds                              []string
	starts                                    []dispatchStart
	assignments                               map[string]Value
	valuesByAddress                           map[string]Value
	summaryByID                               map[string]SemanticSummary
	fileDigests                               map[string]SourceDigest
	functionByID                              map[string]*ssa.Function
	functionIDs                               map[*ssa.Function]string
	functionImplementationIDs                 map[*ssa.Function]string
	loopCache                                 map[*ssa.Function][]loopDescriptor
	loopSeen                                  map[string]bool
	compositionVisited                        map[*ssa.Function]bool
	architectureAnchors                       map[string]BehaviorAnchor
	architectureRelationships                 map[string]BehaviorRelationship
	entryHandoffs                             []EntryHandoff
	entryHandoffCoverage                      EntryHandoffCoverage
	architectureAnchorsConsidered             int
	architectureRelationshipsConsidered       int
	architectureAnchorCollectionLimited       bool
	architectureRelationshipCollectionLimited bool
	declarationFamilyMembersConsidered        int
	walkedFunctions                           map[*ssa.Function]bool
	callbackReferences                        map[*ssa.Function]bool
	callbackReferenceIDs                      map[string]bool
	entrypointPackages                        map[*ssa.Function]map[string]bool
	closureBindings                           map[*ssa.Function][]ssa.Value
	closureBindingAmbiguous                   map[*ssa.Function]bool
	freeVarBindings                           map[*ssa.FreeVar]ssa.Value
	freeVarBindingAmbiguous                   map[*ssa.FreeVar]bool
	parameterBindings                         map[*ssa.Parameter]ssa.Value
	parameterBindingAmbiguous                 map[*ssa.Parameter]bool
	uniqueStoreValues                         map[ssa.Value]ssa.Value
	storeValueAmbiguous                       map[ssa.Value]bool
	valueEvalActive                           map[ssa.Value]bool
	valueReturnActive                         map[*ssa.Function]bool
	valueEvalSteps                            int
	detachedWalk                              bool
	currentPhase                              string
	currentPhaseStarted                       time.Time
}

const (
	defaultMaxDepth          = 16
	defaultMaxTasks          = 1500
	defaultMaxTargets        = 8
	maxValueEvalSteps        = 20_000
	maxValueAlternatives     = 32
	maxValueDescriptionBytes = 4 * 1024
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
		modulePaths:               map[string]bool{},
		callTargets:               map[ssa.CallInstruction][]*ssa.Function{},
		functionByID:              map[string]*ssa.Function{},
		functionIDs:               map[*ssa.Function]string{},
		functionImplementationIDs: map[*ssa.Function]string{},
		loopCache:                 map[*ssa.Function][]loopDescriptor{},
		loopSeen:                  map[string]bool{},
		compositionVisited:        map[*ssa.Function]bool{},
		architectureAnchors:       map[string]BehaviorAnchor{},
		architectureRelationships: map[string]BehaviorRelationship{},
		walkedFunctions:           map[*ssa.Function]bool{},
		callbackReferences:        map[*ssa.Function]bool{},
		callbackReferenceIDs:      map[string]bool{},
		entrypointPackages:        map[*ssa.Function]map[string]bool{},
		closureBindings:           map[*ssa.Function][]ssa.Value{},
		closureBindingAmbiguous:   map[*ssa.Function]bool{},
		freeVarBindings:           map[*ssa.FreeVar]ssa.Value{},
		freeVarBindingAmbiguous:   map[*ssa.FreeVar]bool{},
		parameterBindings:         map[*ssa.Parameter]ssa.Value{},
		parameterBindingAmbiguous: map[*ssa.Parameter]bool{},
		uniqueStoreValues:         map[ssa.Value]ssa.Value{},
		storeValueAmbiguous:       map[ssa.Value]bool{},
		valueEvalActive:           map[ssa.Value]bool{},
		valueReturnActive:         map[*ssa.Function]bool{},
		scenario: Scenario{
			ID:   scenarioID(runtime.GOOS, runtime.GOARCH, opts.BuildTags),
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			Tags: append([]string{}, opts.BuildTags...),
		},
	}
	if err := a.load(); err != nil {
		return Result{}, err
	}
	a.result.Coverage.FrameworkMatched = map[string]int{}
	a.recordProcessEntrypoints()
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	if a.program != nil {
		finishPhase := a.startPhase("candidate_index", "indexing call targets and bounded wrapper candidates")
		a.prepare()
		finishPhase(len(a.allFunctions), len(a.allFunctions))
		limits := defaultCobraLimits()
		finishCobra := a.startPhase("cobra_inventory", "inventorying build-selected typed Cobra descriptors and direct relationships")
		before := len(a.result.Catalog.Triggers)
		a.discoverCobraCommandInventory(limits)
		discovered := len(a.result.Catalog.Triggers) - before
		finishCobra(discovered, a.result.Coverage.CobraDescriptorCount)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	if a.program != nil {
		finishPhase := a.startPhase("architecture_anchors", "projecting existing exact architecture anchors")
		a.recordGlobalArchitectureAnchors()
		finishPhase(len(a.architectureAnchors), len(a.architectureAnchors))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	entrypoints := a.entrypoints()
	a.recordEntryHandoffs(entrypoints)
	finishEntrypoints := a.startPhase("entrypoint_walk", "walking build-selected executable entrypoints")
	for _, entrypoint := range entrypoints {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("surface discovery: %w", err)
		}
		a.resetValueEvaluation()
		a.result.Coverage.EntrypointsConsidered = append(
			a.result.Coverage.EntrypointsConsidered,
			a.symbol(entrypoint),
		)
		a.entrypointPackages[entrypoint] = importedPackagePaths(entrypoint)
		entryAnchorID := a.recordArchitectureAnchor(
			componentmap.AnchorProofProcessEntry,
			"process_entry",
			"process entry "+a.functionID(entrypoint),
			a.location(entrypoint.Pos()),
			a.symbol(entrypoint),
			"Exact build-selected main declaration; process execution is not observed.",
		)
		a.walk(entrypoint, environment{}, nil, entrypoint, 0, false, entryAnchorID)
		a.emitPhaseProgress(len(a.result.Coverage.EntrypointsConsidered), len(entrypoints), "entrypoints")
	}
	finishEntrypoints(len(entrypoints), len(entrypoints))
	finishDetached := a.startPhase("detached_walk", "recovering import-reachable registrations with unresolved entry dispatch")
	a.walkDisconnectedRelevant(entrypoints)
	finishDetached(a.result.Coverage.FunctionsInspected, a.opts.MaxTasks)
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	finishCatalog := a.startPhase("catalog_finalize", "deduplicating surfaces and deriving presentation semantics")
	a.finish(time.Since(started))
	finishCatalog(len(a.result.Catalog.Triggers), len(a.result.Catalog.Triggers))
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	finishGrounding := a.startPhase("grounding_finalize", "finalizing existing architecture grounding")
	a.finishArchitectureGrounding(entrypoints)
	finishGrounding(len(a.result.Grounding.Anchors), len(a.result.Grounding.Anchors))
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

func (a *analyzer) startPhase(phase, detail string) func(int, int) {
	a.currentPhase = phase
	a.currentPhaseStarted = time.Now()
	a.emitProgress(PhaseProgress{Phase: phase, State: "started", Detail: detail})
	return func(completed, total int) {
		latency := time.Since(a.currentPhaseStarted).Milliseconds()
		a.result.Coverage.Phases = append(a.result.Coverage.Phases, PhaseMetric{
			Phase: phase, LatencyMillis: latency, Completed: completed, Total: total, Detail: detail,
		})
		a.emitProgress(PhaseProgress{
			Phase: phase, State: "completed", ElapsedMillis: latency,
			Completed: completed, Total: total, Detail: detail,
		})
		a.currentPhase = ""
		a.currentPhaseStarted = time.Time{}
	}
}

func (a *analyzer) emitPhaseProgress(completed, total int, detail string) {
	if a.currentPhase == "" || a.currentPhaseStarted.IsZero() {
		return
	}
	a.emitProgress(PhaseProgress{
		Phase: a.currentPhase, State: "progress",
		ElapsedMillis: time.Since(a.currentPhaseStarted).Milliseconds(),
		Completed:     completed, Total: total, Detail: detail,
	})
}

func (a *analyzer) emitProgress(progress PhaseProgress) {
	if a.opts.Progress != nil {
		a.opts.Progress(progress)
	}
}

func (a *analyzer) load() error {
	moduleRoots := make([]string, 0, len(a.input.ModuleDirs))
	for _, moduleDir := range a.input.ModuleDirs {
		moduleRoot := filepath.Join(a.root, filepath.FromSlash(moduleDir))
		if err := checkSurfaceGoVersion(moduleRoot, a.opts.Offline); err != nil {
			return err
		}
		moduleRoots = append(moduleRoots, moduleRoot)
	}
	buildFlags := []string{}
	if len(a.opts.BuildTags) > 0 {
		buildFlags = append(buildFlags, "-tags="+strings.Join(a.opts.BuildTags, ","))
	}
	finishLoad := a.startPhase("package_load", "loading build-selected packages and dependency type information")
	var loaded []*packages.Package
	fileSet := token.NewFileSet()
	// REPOMAP_GOTOOLCHAIN=auto is repomap's own knob (owner preference):
	// the binary may be older than the target module's go directive, and
	// the Go loader resolves the toolchain. It is translated to the go
	// command's GOTOOLCHAIN env for the loader only — repomap never
	// mutates the caller's environment. Long-horizon program Phase 1A:
	// online/default analysis also defers toolchain selection to the Go
	// loader (automatic acquisition); only offline analysis is gated on
	// the runtime version.
	loadEnv := os.Environ()
	if auto, ok := repomapGotoolchainEnv(); ok {
		loadEnv = append(loadEnv, "GOTOOLCHAIN="+auto)
	} else if !a.opts.Offline {
		loadEnv = append(loadEnv, "GOTOOLCHAIN=auto")
	}
	for _, moduleRoot := range moduleRoots {
		config := &packages.Config{
			Context: a.ctx,
			Dir:     moduleRoot,
			Env:     loadEnv,
			Fset:    fileSet,
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
				packages.NeedImports | packages.NeedExportFile | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes |
				packages.NeedModule,
			BuildFlags: buildFlags,
			Tests:      false,
		}
		modulePackages, err := packages.Load(config, localSourcePatterns(moduleRoot, moduleRoots)...)
		if ctxErr := a.ctx.Err(); ctxErr != nil {
			finishLoad(len(loaded), len(loaded))
			return fmt.Errorf("surface discovery: %w", ctxErr)
		}
		if err != nil {
			finishLoad(len(loaded), len(loaded))
			// Decision 235 (v11) 1C: with online toolchain selection the
			// loader itself may fail to acquire the required Go toolchain
			// (download/network). That is a typed failure — the generic
			// snapshot/report remains available downstream.
			if _, wantsAuto := repomapGotoolchainEnv(); wantsAuto &&
				toolchainAcquisitionError(err) {
				return &analysisToolchainUnavailableError{cause: err, module: repositoryRelativeModuleDir(a.root, moduleRoot)}
			}
			return fmt.Errorf(
				"surface discovery: load packages from %s: %w",
				repositoryRelativeModuleDir(a.root, moduleRoot),
				err,
			)
		}
		loaded = append(loaded, modulePackages...)
		a.emitPhaseProgress(len(loaded), len(loaded), "loaded package roots")
	}
	finishLoad(len(loaded), len(loaded))
	if ctxErr := a.ctx.Err(); ctxErr != nil {
		return fmt.Errorf("surface discovery: %w", ctxErr)
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
			a.modulePaths[pkg.Module.Path] = true
			if a.modulePath == "" {
				a.modulePath = pkg.Module.Path
			}
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
	finishSSA := a.startPhase("ssa_build", "building SSA for repository packages with dependency type information")
	a.program, a.packages = buildSurfaceSSAProgram(fileSet, safeLoaded)
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	ssaPackages := append([]*ssa.Package(nil), a.packages...)
	sort.Slice(ssaPackages, func(i, j int) bool {
		return ssaPackagePath(ssaPackages[i]) < ssaPackagePath(ssaPackages[j])
	})
	for index, pkg := range ssaPackages {
		if err := a.ctx.Err(); err != nil {
			return fmt.Errorf("surface discovery: %w", err)
		}
		if pkg == nil {
			continue
		}
		pkg.Build()
		if (index+1)%50 == 0 {
			a.emitPhaseProgress(index+1, len(ssaPackages), "SSA packages")
		}
	}
	finishSSA(len(ssaPackages), len(ssaPackages))
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	finishGraph := a.startPhase("call_graph", "indexing all SSA functions and constructing one CHA call graph")
	a.allFunctions = ssautil.AllFunctions(a.program)
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	a.graph = cha.CallGraph(a.program)
	finishGraph(len(a.allFunctions), len(a.allFunctions))
	if err := a.ctx.Err(); err != nil {
		return fmt.Errorf("surface discovery: %w", err)
	}
	return nil
}

func localSourcePatterns(moduleRoot string, moduleRoots []string) []string {
	patterns := []string{"./..."}
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return patterns
	}
	parsed, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return patterns
	}
	unique := make(map[string]struct{})
	for _, replacement := range parsed.Replace {
		if replacement.New.Version != "" || replacement.New.Path == "" || replacement.Old.Path == "" {
			continue
		}
		replacementRoot := filepath.FromSlash(replacement.New.Path)
		if !filepath.IsAbs(replacementRoot) {
			replacementRoot = filepath.Join(moduleRoot, replacementRoot)
		}
		replacementRoot = filepath.Clean(replacementRoot)
		alreadyLoaded := false
		for _, root := range moduleRoots {
			if filepath.Clean(root) == replacementRoot {
				alreadyLoaded = true
				break
			}
		}
		if alreadyLoaded {
			continue
		}
		unique[replacement.Old.Path+"/..."] = struct{}{}
	}
	for pattern := range unique {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns[1:])
	return patterns
}

func buildSurfaceSSAProgram(
	fileSet *token.FileSet,
	loaded []*packages.Package,
) (*ssa.Program, []*ssa.Package) {
	program := ssa.NewProgram(fileSet, ssa.InstantiateGenerics)
	sourceByTypes := make(map[*types.Package]*packages.Package, len(loaded))
	for _, pkg := range loaded {
		if packageSafeForSSA(pkg) {
			sourceByTypes[pkg.Types] = pkg
		}
	}
	created := make(map[*types.Package]*ssa.Package)
	active := make(map[*types.Package]bool)
	var create func(*types.Package) *ssa.Package
	create = func(typePackage *types.Package) *ssa.Package {
		if typePackage == nil {
			return nil
		}
		if pkg, exists := created[typePackage]; exists {
			return pkg
		}
		if active[typePackage] {
			return nil
		}
		active[typePackage] = true
		imports := append([]*types.Package(nil), typePackage.Imports()...)
		sort.Slice(imports, func(i, j int) bool { return imports[i].Path() < imports[j].Path() })
		for _, imported := range imports {
			create(imported)
		}
		delete(active, typePackage)
		source := sourceByTypes[typePackage]
		var files []*ast.File
		var info *types.Info
		if source != nil {
			files = source.Syntax
			info = source.TypesInfo
		}
		created[typePackage] = program.CreatePackage(typePackage, files, info, true)
		return created[typePackage]
	}
	result := make([]*ssa.Package, 0, len(loaded))
	for _, pkg := range loaded {
		if !packageSafeForSSA(pkg) {
			continue
		}
		result = append(result, create(pkg.Types))
	}
	return program, result
}

func repositoryRelativeModuleDir(root, moduleRoot string) string {
	relative, err := filepath.Rel(root, moduleRoot)
	if err != nil || relative == "" {
		return "."
	}
	return filepath.ToSlash(relative)
}

func ssaPackagePath(pkg *ssa.Package) string {
	if pkg == nil || pkg.Pkg == nil {
		return ""
	}
	return pkg.Pkg.Path()
}

// repomapGotoolchainEnv returns the GOTOOLCHAIN value repomap's own
// REPOMAP_GOTOOLCHAIN knob requests ("auto" or "local+auto"), or false when
// the knob is unset. repomap owns this variable — a plain GOTOOLCHAIN value
// from the caller's environment must not change repomap behavior.
func repomapGotoolchainEnv() (string, bool) {
	value := os.Getenv("REPOMAP_GOTOOLCHAIN")
	if value == "auto" || value == "local+auto" {
		return value, true
	}
	return "", false
}

func checkSurfaceGoVersion(root string, offline bool) error {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("surface discovery: read go.mod: %w", err)
	}
	defer file.Close()

	// Long-horizon program Phase 1A: online/default analysis always defers
	// toolchain selection to the Go loader (automatic acquisition); the
	// runtime version check is an honest admission gate ONLY for offline
	// analysis, which cannot acquire a toolchain. REPOMAP_GOTOOLCHAIN=auto
	// (owner preference) keeps deferring for both modes. A plain GOTOOLCHAIN
	// value is deliberately NOT honored — that variable belongs to the Go
	// toolchain; repomap owns its own knob.
	if _, auto := repomapGotoolchainEnv(); auto {
		return nil
	}
	if !offline {
		return nil
	}

	reader := bufio.NewReader(io.LimitReader(file, 1024*1024))
	for {
		raw, err := reader.ReadString('\n')
		if len(raw) == 0 {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		fields := strings.Fields(raw)
		if len(fields) != 2 || fields[0] != "go" {
			if errors.Is(err, io.EOF) {
				break
			}
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
	return nil
}

// analysisToolchainUnavailableError is the Decision 235 1C typed failure:
// online toolchain selection could not acquire the required Go toolchain.
// The generic snapshot/report remains available downstream.
type analysisToolchainUnavailableError struct {
	cause  error
	module string
}

func (e *analysisToolchainUnavailableError) Error() string {
	return fmt.Sprintf("analysis_toolchain_unavailable: acquire Go toolchain for module %s: %v", e.module, e.cause)
}

func (e *analysisToolchainUnavailableError) Unwrap() error { return e.cause }

// IsAnalysisToolchainUnavailable reports whether the error is the typed
// toolchain-acquisition failure (Decision 235 1C).
func IsAnalysisToolchainUnavailable(err error) bool {
	var target *analysisToolchainUnavailableError
	return errors.As(err, &target)
}

// toolchainAcquisitionError detects Go toolchain acquisition failures in a
// packages.Load error — the go command reports them with a distinctive
// message about downloading the go toolchain or a missing go.mod go line.
func toolchainAcquisitionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "go toolchain") ||
		strings.Contains(message, "downloading go") ||
		strings.Contains(message, "module go@") ||
		strings.Contains(message, "toolchain switch")
}

func (a *analyzer) prepare() {
	a.relevant = map[*ssa.Function]bool{}
	a.relevanceDistance = map[*ssa.Function]int{}
	orderedFunctions := a.orderedFunctions()
	for functionIndex, function := range orderedFunctions {
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
				if closure, ok := instruction.(*ssa.MakeClosure); ok {
					a.recordClosureBindings(closure)
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
						// Decision 220 B: repository-local typed registration
						// shapes (path string, handler) also make the calling
						// function relevant for the walk. Convenience methods
						// that wrap a catalog seed are handled by wrapper
						// propagation and are not claimed here.
						if a.typedRegistrationShape(target) && !a.callsCatalogSeed(target) &&
							a.terminalSeedEligible(catalog.Seed{}, call, target) {
							a.relevant[function] = true
							a.relevanceDistance[function] = 0
						}
					}
				}
				if store, ok := instruction.(*ssa.Store); ok {
					a.recordUniqueStore(store)
					a.recordFunctionReference(store.Val)
					if _, matched := a.fieldSeed(store); matched {
						a.relevant[function] = true
						a.relevanceDistance[function] = 0
					}
				}
			}
		}
		if (functionIndex+1)%1000 == 0 {
			a.emitPhaseProgress(functionIndex+1, len(orderedFunctions), "SSA functions indexed")
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

func (a *analyzer) recordUniqueStore(store *ssa.Store) {
	if store == nil || store.Addr == nil || store.Val == nil || a.storeValueAmbiguous[store.Addr] {
		return
	}
	if previous, exists := a.uniqueStoreValues[store.Addr]; exists && previous != store.Val {
		a.storeValueAmbiguous[store.Addr] = true
		delete(a.uniqueStoreValues, store.Addr)
		return
	}
	a.uniqueStoreValues[store.Addr] = store.Val
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
			_, capturedClosure := a.closureBindings[function]
			if !hasRelevantCaller || capturedClosure {
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
			a.resetValueEvaluation()
			a.walk(root, a.closureEnvironment(root), nil, candidates[root], 0, false, "")
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

func (a *analyzer) recordClosureBindings(closure *ssa.MakeClosure) {
	if closure == nil {
		return
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || a.closureBindingAmbiguous[function] {
		return
	}
	bindings := append([]ssa.Value(nil), closure.Bindings...)
	if previous, exists := a.closureBindings[function]; exists {
		if len(previous) != len(bindings) {
			a.closureBindingAmbiguous[function] = true
			delete(a.closureBindings, function)
			return
		}
		for index := range previous {
			if previous[index] != bindings[index] {
				a.closureBindingAmbiguous[function] = true
				delete(a.closureBindings, function)
				return
			}
		}
		return
	}
	a.closureBindings[function] = bindings
	for index, freeVar := range function.FreeVars {
		if index >= len(bindings) || a.freeVarBindingAmbiguous[freeVar] {
			continue
		}
		if previous, exists := a.freeVarBindings[freeVar]; exists && previous != bindings[index] {
			a.freeVarBindingAmbiguous[freeVar] = true
			delete(a.freeVarBindings, freeVar)
			continue
		}
		a.freeVarBindings[freeVar] = bindings[index]
	}
}

func (a *analyzer) closureEnvironment(function *ssa.Function) environment {
	bindings, exists := a.closureBindings[function]
	if !exists || len(bindings) != len(function.FreeVars) {
		return environment{}
	}
	env := environment{}
	for index, freeVar := range function.FreeVars {
		env[freeVar] = a.eval(bindings[index], environment{}, 0)
	}
	return env
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
	if a.tasks%100 == 0 {
		a.emitPhaseProgress(a.tasks, a.opts.MaxTasks, "bounded functions walked")
	}
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
			targets = a.addBoundValueTarget(targets, call, env)
			if len(targets) == 0 {
				continue
			}
			admitted := make([]*ssa.Function, 0, len(targets))
			var rejected *ssa.Function
			for _, target := range targets {
				if a.callTargetEligible(call, target, entrypoint, env) {
					admitted = append(admitted, target)
					continue
				}
				if rejected == nil {
					rejected = target
				}
			}
			if rejected != nil {
				a.recordUnresolvedCallTarget(function, call, rejected, entrypoint)
			}
			targets = admitted
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
						componentmap.AnchorProofCallTarget,
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
							componentmap.AnchorProofCallTarget,
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
				// Decision 220 B: generic typed registration detector. A
				// repository-local call to a method with the closed typed
				// registration shape (path string, handler) is recorded as a
				// route with the detector producer; exact when the path and
				// handler resolve, otherwise dynamic with a bounded frontier.
				// Catalog seeds and their convenience wrappers always win;
				// the detector never double-reports.
				if a.typedRegistrationShape(target) && !a.callsCatalogSeed(target) &&
					a.terminalSeedEligible(catalog.Seed{}, call, target) {
					a.recordTypedRegistration(call, target, env, chain, entrypoint, callAmbiguous)
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
					a.bindWithCall(target, a.arguments(call), env, depth, call),
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

func (a *analyzer) addBoundValueTarget(
	targets []*ssa.Function,
	call ssa.CallInstruction,
	env environment,
) []*ssa.Function {
	if call == nil || call.Common().Value == nil {
		return targets
	}
	value := a.eval(call.Common().Value, env, 0)
	ids := append([]string{value.Text}, value.Candidates...)
	if function := a.uniqueOriginFunction(call.Common().Value, 0); function != nil {
		ids = append(ids, a.functionID(function))
	}
	if freeVar, ok := call.Common().Value.(*ssa.FreeVar); ok && !a.freeVarBindingAmbiguous[freeVar] {
		if function := a.boundFunction(a.freeVarBindings[freeVar], 0); function != nil {
			ids = append(ids, a.functionID(function))
		}
	}
	seen := make(map[*ssa.Function]struct{}, len(targets))
	for _, target := range targets {
		seen[target] = struct{}{}
	}
	for _, id := range ids {
		target := a.functionByID[id]
		if target == nil {
			target = a.functionByID[cleanFunctionID(id)]
		}
		if target != nil {
			if _, exists := seen[target]; !exists {
				seen[target] = struct{}{}
				targets = append(targets, target)
			}
		}
	}
	return targets
}

// uniqueOriginFunction follows only identity-preserving, single-origin SSA
// values. It intentionally rejects Phi, maps, reflection, invokes, and every
// ambiguous store/parameter/free-variable binding.
func (a *analyzer) uniqueOriginFunction(value ssa.Value, depth int) *ssa.Function {
	if value == nil || depth > 8 {
		return nil
	}
	switch current := value.(type) {
	case *ssa.Function:
		return current
	case *ssa.MakeClosure:
		function, _ := current.Fn.(*ssa.Function)
		return function
	case *ssa.Parameter:
		if a.parameterBindingAmbiguous[current] {
			return nil
		}
		return a.uniqueOriginFunction(a.parameterBindings[current], depth+1)
	case *ssa.FreeVar:
		if a.freeVarBindingAmbiguous[current] {
			return nil
		}
		return a.uniqueOriginFunction(a.freeVarBindings[current], depth+1)
	case *ssa.ChangeType:
		return a.uniqueOriginFunction(current.X, depth+1)
	case *ssa.Convert:
		return a.uniqueOriginFunction(current.X, depth+1)
	case *ssa.MakeInterface:
		return a.uniqueOriginFunction(current.X, depth+1)
	case *ssa.UnOp:
		if a.storeValueAmbiguous[current.X] {
			return nil
		}
		if stored := a.uniqueStoreValues[current.X]; stored != nil {
			return a.uniqueOriginFunction(stored, depth+1)
		}
		return nil
	case *ssa.Alloc:
		if a.storeValueAmbiguous[current] {
			return nil
		}
		return a.uniqueOriginFunction(a.uniqueStoreValues[current], depth+1)
	case *ssa.Extract:
		call, ok := current.Tuple.(*ssa.Call)
		if !ok || call.Common().StaticCallee() == nil {
			return nil
		}
		return a.uniqueReturnFunction(call.Common().StaticCallee(), current.Index, depth+1)
	case *ssa.Call:
		if current.Common().StaticCallee() == nil {
			return nil
		}
		return a.uniqueReturnFunction(current.Common().StaticCallee(), 0, depth+1)
	}
	return nil
}

func (a *analyzer) uniqueReturnFunction(function *ssa.Function, index, depth int) *ssa.Function {
	if function == nil || function.Blocks == nil || index < 0 {
		return nil
	}
	var result *ssa.Function
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || index >= len(returned.Results) {
				continue
			}
			candidate := a.uniqueOriginFunction(returned.Results[index], depth+1)
			if candidate == nil {
				return nil
			}
			if result != nil && result != candidate {
				return nil
			}
			result = candidate
		}
	}
	return result
}

func (a *analyzer) boundFunction(value ssa.Value, depth int) *ssa.Function {
	if value == nil || depth > 4 {
		return nil
	}
	switch current := value.(type) {
	case *ssa.Function:
		return current
	case *ssa.MakeClosure:
		function, _ := current.Fn.(*ssa.Function)
		return function
	case *ssa.ChangeType:
		return a.boundFunction(current.X, depth+1)
	case *ssa.Convert:
		return a.boundFunction(current.X, depth+1)
	case *ssa.MakeInterface:
		return a.boundFunction(current.X, depth+1)
	case *ssa.UnOp:
		return a.boundFunction(current.X, depth+1)
	case *ssa.Alloc:
		var resolved *ssa.Function
		for function := range a.allFunctions {
			if function == nil {
				continue
			}
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					store, ok := instruction.(*ssa.Store)
					if !ok || store.Addr != current {
						continue
					}
					candidate := a.boundFunction(store.Val, depth+1)
					if candidate == nil {
						continue
					}
					if resolved != nil && resolved != candidate {
						return nil
					}
					resolved = candidate
				}
			}
		}
		return resolved
	case *ssa.FreeVar:
		if a.freeVarBindingAmbiguous[current] {
			return nil
		}
		return a.boundFunction(a.freeVarBindings[current], depth+1)
	}
	return nil
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
// the exact SSA call. Executable walks additionally constrain targets to the
// selected executable's build/import closure. A library has no process entry,
// but an exact repository-local direct static call still proves a call boundary;
// it does not prove runtime reachability.
func (a *analyzer) callTargetEligible(
	call ssa.CallInstruction,
	target, entrypoint *ssa.Function,
	env environment,
) bool {
	if !a.callTargetWitness(call, target, env) {
		return false
	}
	if entrypoint == nil {
		return a.repositoryDirectStaticCall(call, target)
	}
	packages := a.entrypointPackages[entrypoint]
	if packages == nil {
		packages = importedPackagePaths(entrypoint)
		a.entrypointPackages[entrypoint] = packages
	}
	return packages[functionPackagePath(target)]
}

func (a *analyzer) repositoryDirectStaticCall(call ssa.CallInstruction, target *ssa.Function) bool {
	return call != nil && target != nil && call.Parent() != nil &&
		call.Common().StaticCallee() == target &&
		a.isRepositoryFunction(call.Parent()) && a.isRepositoryFunction(target)
}

func (a *analyzer) callTargetWitness(call ssa.CallInstruction, target *ssa.Function, env environment) bool {
	if a.terminalTargetEligible(call, target) {
		return true
	}
	if call == nil || target == nil || call.Common().Value == nil {
		return false
	}
	if origin := a.uniqueOriginFunction(call.Common().Value, 0); origin == target {
		return true
	}
	value := a.eval(call.Common().Value, env, 0)
	targetID := a.functionID(target)
	if value.Text == targetID || cleanFunctionID(value.Text) == cleanFunctionID(targetID) {
		return true
	}
	for _, candidate := range value.Candidates {
		if candidate == targetID || cleanFunctionID(candidate) == cleanFunctionID(targetID) {
			return true
		}
	}
	if _, captured := call.Common().Value.(*ssa.FreeVar); captured && !a.closureBindingAmbiguous[target] {
		_, witnessed := a.closureBindings[target]
		return witnessed
	}
	return false
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
	if a.terminalTargetEligible(call, target) {
		return true
	}
	_, captured := call.Common().Value.(*ssa.FreeVar)
	return captured
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

// httpVerbFromMethodName returns the closed HTTP verb for a registration
// method name, or "" when the name is not a verb (Decision 220 A/B). Only
// the standard HTTP methods plus "Any" are recognized; a name like "Add" or
// "Register" never infers a verb.
func httpVerbFromMethodName(name string) string {
	switch name {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return name
	case "Any":
		return "ANY"
	default:
		return ""
	}
}

// recordTypedRegistration records a route from the generic typed
// registration detector (Decision 220 B): a repository-local call to a
// method whose signature is (path string, handler). Resolution is exact when
// the path argument is a known constant and the handler argument resolves to
// a repository-local symbol; otherwise dynamic with a bounded frontier. The
// detector never invents a verb and never double-reports catalog seeds.
func (a *analyzer) recordTypedRegistration(
	call ssa.CallInstruction,
	target *ssa.Function,
	env environment,
	chain []Wrapper,
	entrypoint *ssa.Function,
	ambiguous bool,
) {
	location := a.location(call.Pos())
	args := a.arguments(call)
	// Both static and invoke method calls place the receiver first (static:
	// receiver is Args[0]; invoke: arguments() prepends the receiver Value).
	// The path and handler are the first and second explicit parameters.
	receiverOffset := 1
	pathValue := dynamicValue("unknown path")
	if len(args) > receiverOffset {
		pathValue = a.eval(args[receiverOffset], env, 0)
	}
	handlerValue := dynamicValue("unknown handler")
	if len(args) > receiverOffset+1 {
		handlerValue = a.eval(args[receiverOffset+1], env, 0)
	}
	frontiers := []Frontier{}
	if !pathValue.Known {
		frontiers = append(frontiers, Frontier{Kind: "dynamic_route_identity", Detail: pathValue.Text, Location: &location})
	}
	if !handlerValue.Known {
		frontiers = append(frontiers, Frontier{Kind: "dynamic_handler_identity", Detail: handlerValue.Text, Location: &location})
	}
	resolution := "exact"
	status := "confirmed_typed_registration"
	if ambiguous {
		resolution = "ambiguous"
	}
	if len(frontiers) > 0 {
		resolution = "dynamic"
		status = "dynamic_typed_registration"
	}
	basis := string(catalog.OriginCatalogStatic)
	if len(chain) > 0 {
		basis = string(catalog.OriginWrapperStatic)
	}
	// A closed HTTP verb method name (GET/POST/PUT/DELETE/PATCH/HEAD/
	// OPTIONS/Any) establishes the route method exactly (Decision 220 A/B);
	// other names leave the method unset rather than guessing.
	method := ""
	if verb := httpVerbFromMethodName(target.Name()); verb != "" {
		method = verb
	}
	record := TriggerRecord{
		Kind:              "http_route",
		Producer:          "typed_registration_detector",
		Identity:          Identity{Method: method, Path: pathValue},
		Transport:         "http",
		Framework:         "typed",
		ProcessEntrypoint: a.symbol(entrypoint),
		Dispatcher:        dynamicValue("typed registration receiver"),
		RegistrationSite:  location,
		Handler:           handlerValue,
		Middleware:        []Value{},
		WrapperChain:      append([]Wrapper{}, chain...),
		FinalSeed:         "typed_registration_detector",
		DiscoveryBasis:    basis,
		Certainty:         "static",
		Resolution:        resolution,
		ScenarioID:        a.scenario.ID,
		Evidence: []Evidence{{
			ID: "typed-registration:" + locationKey(location), Kind: "typed_registration_call",
			Location: location, Detail: a.functionID(target),
		}},
		Provenance: []Provenance{{
			Provider: "go_ssa", Version: AnalyzerVersion,
			Operation: "detect_typed_registration_shape", Detail: a.functionID(target),
		}},
		DynamicFrontier: frontiers,
		Status:          status,
	}
	record.TerminalSourceScope, record.ApplicationClass, record.PromotionBasis =
		classifyTerminalOwnership(location, chain, a.detachedWalk)
	record.ProvisionalID = !pathValue.Known || !handlerValue.Known
	record.ID = stableTriggerID(record)
	a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
	a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontiers...)
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
		a.valuesByAddress[key] = a.mergeValues([]Value{previous, value})
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
		a.recordParameterBinding(parameter, args[index])
		result[parameter] = a.eval(args[index], caller, depth+1)
	}
	return result
}

func (a *analyzer) recordParameterBinding(parameter *ssa.Parameter, value ssa.Value) {
	if parameter == nil || value == nil || a.parameterBindingAmbiguous[parameter] {
		return
	}
	if previous, exists := a.parameterBindings[parameter]; exists && previous != value {
		a.parameterBindingAmbiguous[parameter] = true
		delete(a.parameterBindings, parameter)
		return
	}
	a.parameterBindings[parameter] = value
}

func (a *analyzer) bindWithCall(
	target *ssa.Function,
	args []ssa.Value,
	caller environment,
	depth int,
	call ssa.CallInstruction,
) environment {
	result := a.bind(target, args, caller, depth)
	closure, ok := call.Common().Value.(*ssa.MakeClosure)
	if !ok || closure == nil || closure.Fn != target {
		if _, captured := call.Common().Value.(*ssa.FreeVar); captured {
			for freeVar, value := range a.closureEnvironment(target) {
				result[freeVar] = value
			}
		}
		return result
	}
	for index, freeVar := range target.FreeVars {
		if index >= len(closure.Bindings) {
			break
		}
		result[freeVar] = a.eval(closure.Bindings[index], caller, depth+1)
	}
	return result
}

func (a *analyzer) eval(value ssa.Value, env environment, depth int) Value {
	if value == nil {
		return dynamicValue("nil")
	}
	if a.ctx != nil && a.ctx.Err() != nil {
		return dynamicValue("value evaluation canceled")
	}
	if depth > a.opts.MaxDepth {
		a.addBudget("value_depth")
		return dynamicValue("unresolved value")
	}
	if resolved, ok := env[value]; ok {
		return resolved
	}
	if a.valueEvalSteps >= maxValueEvalSteps {
		a.addBudget("value_evaluation")
		return dynamicValue("unresolved value")
	}
	a.valueEvalSteps++
	if a.valueEvalActive == nil {
		a.valueEvalActive = map[ssa.Value]bool{}
	}
	if a.valueEvalActive[value] {
		a.addBudget("value_evaluation_cycle")
		return dynamicValue("unresolved value")
	}
	a.valueEvalActive[value] = true
	defer delete(a.valueEvalActive, value)
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
				if len(left.Text) > maxValueDescriptionBytes ||
					len(right.Text) > maxValueDescriptionBytes ||
					len(left.Text)+len(right.Text) > maxValueDescriptionBytes {
					a.addBudget("value_description")
					return dynamicValue("unresolved value")
				}
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
		edgeCount := len(current.Edges)
		if edgeCount > maxValueAlternatives {
			a.addBudget("value_alternatives")
			edgeCount = maxValueAlternatives
		}
		values := make([]Value, 0, edgeCount)
		for _, edge := range current.Edges[:edgeCount] {
			values = append(values, a.eval(edge, env, depth+1))
		}
		return a.mergeValues(values)
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
		targetCount := len(targets)
		if targetCount > maxValueAlternatives {
			a.addBudget("value_alternatives")
			targetCount = maxValueAlternatives
		}
		candidates := make([]string, 0, targetCount)
		for _, target := range targets[:targetCount] {
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

func (a *analyzer) resetValueEvaluation() {
	a.valueEvalSteps = 0
	a.valueEvalActive = map[ssa.Value]bool{}
	a.valueReturnActive = map[*ssa.Function]bool{}
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
	if a.valueReturnActive == nil {
		a.valueReturnActive = map[*ssa.Function]bool{}
	}
	if a.valueReturnActive[target] {
		a.addBudget("value_evaluation_cycle")
		return Value{}, false
	}
	a.valueReturnActive[target] = true
	defer delete(a.valueReturnActive, target)
	env := a.bind(target, args, caller, depth+1)
	values := make([]Value, 0, maxValueAlternatives)
	for _, block := range target.Blocks {
		for _, instruction := range block.Instrs {
			if store, ok := instruction.(*ssa.Store); ok {
				a.recordAssignment(store, env)
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) == 0 {
				continue
			}
			if len(values) >= maxValueAlternatives {
				a.addBudget("value_alternatives")
				return a.mergeValues(values), true
			}
			values = append(values, a.eval(returned.Results[0], env, depth+1))
		}
	}
	if len(values) == 0 {
		return Value{}, false
	}
	return a.mergeValues(values), true
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
	graphLimit := a.opts.MaxTargets + 1 - len(result)
	if graphLimit < 0 {
		graphLimit = 0
	}
	graphTargets := make(map[string]boundedCallTarget, graphLimit)
	if a.graph != nil {
		if node := a.graph.Nodes[call.Parent()]; node != nil {
			for edgeIndex, edge := range node.Out {
				if edgeIndex%1024 == 0 && a.ctx.Err() != nil {
					break
				}
				if edge.Site != call || edge.Callee == nil || edge.Callee.Func == nil || seen[edge.Callee.Func] {
					continue
				}
				a.retainBoundedCallTarget(graphTargets, a.newBoundedCallTarget(call, edge.Callee.Func), graphLimit)
			}
		}
	}
	for _, target := range graphTargets {
		if !seen[target.function] {
			result = append(result, target.function)
			seen[target.function] = true
		}
	}
	byImplementation := make(map[string]*ssa.Function, len(result))
	for _, function := range result {
		key := a.functionImplementationID(function)
		previous, exists := byImplementation[key]
		if !exists || (previous.Synthetic != "" && function.Synthetic == "") {
			byImplementation[key] = function
		}
	}
	result = result[:0]
	for _, function := range byImplementation {
		result = append(result, function)
	}
	sort.Slice(result, func(i, j int) bool {
		leftID := a.functionID(result[i])
		rightID := a.functionID(result[j])
		if leftID != rightID {
			return leftID < rightID
		}
		if result[i].Synthetic != result[j].Synthetic {
			return result[i].Synthetic < result[j].Synthetic
		}
		return result[i].Pos() < result[j].Pos()
	})
	return result
}

type boundedCallTarget struct {
	function       *ssa.Function
	implementation string
	terminalSeed   bool
	repository     bool
	concrete       bool
	order          string
}

func (a *analyzer) newBoundedCallTarget(
	call ssa.CallInstruction,
	function *ssa.Function,
) boundedCallTarget {
	seed, matched := a.callSeed(function)
	return boundedCallTarget{
		function:       function,
		implementation: a.functionImplementationID(function),
		terminalSeed:   matched && a.terminalSeedEligible(seed, call, function),
		repository:     a.isRepositoryFunction(function),
		concrete:       function != nil && function.Synthetic == "",
		order:          a.functionID(function),
	}
}

func (a *analyzer) retainBoundedCallTarget(
	retained map[string]boundedCallTarget,
	candidate boundedCallTarget,
	limit int,
) {
	if candidate.function == nil || limit <= 0 {
		return
	}
	if previous, exists := retained[candidate.implementation]; exists {
		if boundedCallTargetLess(candidate, previous) {
			retained[candidate.implementation] = candidate
		}
		return
	}
	if len(retained) < limit {
		retained[candidate.implementation] = candidate
		return
	}
	var worstKey string
	var worst boundedCallTarget
	first := true
	for key, current := range retained {
		if first || boundedCallTargetLess(worst, current) {
			worstKey = key
			worst = current
			first = false
		}
	}
	if boundedCallTargetLess(candidate, worst) {
		delete(retained, worstKey)
		retained[candidate.implementation] = candidate
	}
}

func boundedCallTargetLess(left, right boundedCallTarget) bool {
	if left.terminalSeed != right.terminalSeed {
		return left.terminalSeed
	}
	if left.repository != right.repository {
		return left.repository
	}
	if left.concrete != right.concrete {
		return left.concrete
	}
	if left.order != right.order {
		return left.order < right.order
	}
	return left.implementation < right.implementation
}

func (a *analyzer) functionImplementationID(function *ssa.Function) string {
	if function == nil {
		return "unknown"
	}
	if a.functionImplementationIDs == nil {
		a.functionImplementationIDs = map[*ssa.Function]string{}
	}
	if id, found := a.functionImplementationIDs[function]; found {
		return id
	}
	id := functionPackagePath(function) + "\x00" + function.Name() + "\x00" + strconv.Itoa(int(function.Pos()))
	a.functionImplementationIDs[function] = id
	return id
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

// typedRegistrationShape reports whether a repository-local method has the
// closed typed registration shape (Decision 220 B): a first string parameter
// (route path) and a second handler parameter of a supported handler kind
// (func, func(http.ResponseWriter,*http.Request), http.Handler, or a
// context-handler interface). The detector is deliberately conservative: it
// never infers a verb from the name and never claims a shape when the
// signature does not establish it exactly.
func (a *analyzer) typedRegistrationShape(function *ssa.Function) bool {
	if function == nil || function.Signature == nil {
		return false
	}
	signature := function.Signature
	params := signature.Params()
	if params.Len() < 2 {
		return false
	}
	pathType, ok := params.At(0).Type().(*types.Basic)
	if !ok || pathType.Kind() != types.String {
		return false
	}
	handlerType := params.At(1).Type()
	return typedHandlerKind(handlerType) != ""
}

// callsCatalogSeed reports whether the target's body directly calls a
// catalog seed (Decision 220 B). A framework convenience method that wraps a
// catalog seed (e.g. echo GET → Add, gin GET → Handle) is therefore handled
// by the existing wrapper propagation, which preserves the exact method
// constant and group prefix; the generic detector must not double-report it.
func (a *analyzer) callsCatalogSeed(target *ssa.Function) bool {
	if target == nil || target.Blocks == nil {
		return false
	}
	for _, block := range target.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(ssa.CallInstruction)
			if !ok {
				continue
			}
			for _, candidate := range a.callTargets[call] {
				if _, matched := a.callSeed(candidate); matched {
					return true
				}
			}
		}
	}
	return false
}

// typedHandlerKind classifies a handler parameter type into a closed kind, or
// returns "" when the type is not an established handler shape (Decision
// 220 B). It never widens: an arbitrary interface without an http.Handler
// method set is not claimed as a handler.
func typedHandlerKind(handlerType types.Type) string {
	if handlerType == nil {
		return ""
	}
	if signature, ok := handlerType.(*types.Signature); ok {
		if signature.Recv() == nil {
			// func(...) — accept any plain function (its parameter shape is
			// validated by the analyzer walk, not guessed here).
			return "func"
		}
		return ""
	}
	named, ok := handlerType.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return ""
	}
	path := named.Obj().Pkg().Path()
	name := named.Obj().Name()
	if path == "net/http" && name == "Handler" {
		return "http_handler"
	}
	if path == "net/http" && name == "HandlerFunc" {
		return "http_handler_func"
	}
	// context handler interfaces from supported server frameworks are
	// recognized only when the framework is already cataloged; the generic
	// detector stays conservative and does not guess unknown interfaces.
	if path == "github.com/gin-gonic/gin" && name == "HandlerFunc" {
		return "context_handler"
	}
	if path == "github.com/labstack/echo/v4" && name == "HandlerFunc" {
		return "context_handler"
	}
	if path == "github.com/go-chi/chi/v5" && name == "HandlerFunc" {
		return "context_handler"
	}
	return ""
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
		if kind != "" && kind != "command_dispatch" && kind != "registry_write" {
			families[kind] = append(families[kind], a.symbol(function))
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if !ok {
					continue
				}
				target := call.Common().StaticCallee()
				if !a.repositoryDirectStaticCall(call, target) {
					continue
				}
				kind := a.architectureCallKind(target)
				if kind == "" {
					continue
				}
				location := a.location(call.Pos())
				anchorID := a.recordArchitectureAnchor(
					componentmap.AnchorProofCallTarget,
					kind,
					kind+" "+a.functionID(target),
					location,
					a.symbol(target),
					"Exact repository-local direct static call boundary; runtime reachability is not implied.",
				)
				if kind == "registry_write" {
					extensionID := a.recordArchitectureAnchor(
						componentmap.AnchorProofCallTarget,
						"extension_family",
						"extension registration via "+a.functionID(target),
						location,
						a.symbol(target),
						"Registration establishes an extension boundary, not execution or complete implementation coverage.",
					)
					a.recordArchitectureRelationship(anchorID, extensionID, location)
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
		a.recordDeclarationFamilyAnchors(kind, families[kind])
	}
}

func (a *analyzer) recordDeclarationFamilyAnchors(kind string, candidates []Symbol) {
	members := deduplicateArchitectureSymbols(publishableArchitectureDeclarationSymbols(candidates))
	if len(members) == 0 {
		return
	}
	a.declarationFamilyMembersConsidered += len(members)
	for start := 0; start < len(members); start += MaxArchitectureAnchorMembers {
		end := min(start+MaxArchitectureAnchorMembers, len(members))
		chunk := members[start:end]
		label := "discovered " + kind + " declaration family"
		if len(members) > MaxArchitectureAnchorMembers {
			label += fmt.Sprintf(" %d", start/MaxArchitectureAnchorMembers+1)
		}
		a.recordArchitectureAnchorMembers(
			componentmap.AnchorProofDeclarationFamily,
			kind,
			label,
			chunk[0].Location,
			chunk,
			"Exact build-selected declarations share a local architecture classification; invocation and shared function signature are not implied.",
		)
	}
}

func publishableArchitectureDeclarationSymbols(candidates []Symbol) []Symbol {
	result := make([]Symbol, 0, len(candidates))
	for _, candidate := range candidates {
		path := candidate.Location.Path
		if candidate.ID == "" || path == "" || path == "." || candidate.Location.Line <= 0 ||
			candidate.Location.Column < 0 || !fs.ValidPath(path) || strings.ContainsRune(path, '\\') {
			continue
		}
		result = append(result, candidate)
	}
	return result
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
	proofMode componentmap.AnchorProofMode,
	kind, label string,
	location Location,
	member Symbol,
	limitation string,
) string {
	return a.recordArchitectureAnchorMembersWithProvenance(
		proofMode, kind, label, location, []Symbol{member}, limitation,
		Provenance{Provider: "go_ssa", Version: AnalyzerVersion, Operation: "classify_architecture_anchor"},
	)
}

func (a *analyzer) recordArchitectureAnchorMembers(
	proofMode componentmap.AnchorProofMode,
	kind, label string,
	location Location,
	members []Symbol,
	limitation string,
) string {
	return a.recordArchitectureAnchorMembersWithProvenance(
		proofMode, kind, label, location, members, limitation,
		Provenance{Provider: "go_ssa", Version: AnalyzerVersion, Operation: "classify_architecture_anchor"},
	)
}

func (a *analyzer) recordArchitectureAnchorMembersWithProvenance(
	proofMode componentmap.AnchorProofMode,
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
	canonicalMembers := canonicalArchitectureAnchorMembers(members)
	member := canonicalMembers[0]
	identity := locationKey(location)
	if kind == "process_entry" {
		identity = fmt.Sprintf("%s:%d", location.Path, location.Line)
	} else if kind == "registry_write" || kind == "extension_family" {
		identity = member.ID
	}
	digest := sha256.New()
	writeArchitectureAnchorIdentityField(digest, "architecture-anchor-v3")
	writeArchitectureAnchorIdentityField(digest, string(proofMode))
	writeArchitectureAnchorIdentityField(digest, kind)
	writeArchitectureAnchorIdentityField(digest, identity)
	var memberCount [8]byte
	binary.BigEndian.PutUint64(memberCount[:], uint64(len(canonicalMembers)))
	_, _ = digest.Write(memberCount[:])
	for _, canonicalMember := range canonicalMembers {
		writeArchitectureAnchorIdentityField(digest, canonicalMember.ID)
		writeArchitectureAnchorIdentityField(digest, canonicalMember.Package)
		writeArchitectureAnchorIdentityField(digest, canonicalMember.Name)
		writeArchitectureAnchorIdentityField(digest, canonicalMember.Location.Path)
		writeArchitectureAnchorIdentityField(digest, strconv.Itoa(canonicalMember.Location.Line))
		memberColumn := canonicalMember.Location.Column
		if proofMode == componentmap.AnchorProofProcessEntry {
			// The syntax producer reports declaration columns as zero while SSA
			// reports the identifier column. Path+line is the shared exact local
			// process-entry identity used to deduplicate those two typed proofs.
			memberColumn = 0
		}
		writeArchitectureAnchorIdentityField(digest, strconv.Itoa(memberColumn))
		var equivalentCount [8]byte
		binary.BigEndian.PutUint64(equivalentCount[:], uint64(len(canonicalMember.EquivalentIDs)))
		_, _ = digest.Write(equivalentCount[:])
		for _, equivalentID := range canonicalMember.EquivalentIDs {
			writeArchitectureAnchorIdentityField(digest, equivalentID)
		}
	}
	digestBytes := digest.Sum(nil)
	id := "anchor-" + hex.EncodeToString(digestBytes[:12])
	if _, exists := a.architectureAnchors[id]; exists {
		return id
	}
	a.architectureAnchorsConsidered++
	if len(a.architectureAnchors) >= maxCollectedArchitectureAnchors {
		a.architectureAnchorCollectionLimited = true
		a.addBudget("architecture_anchor_collection")
		return ""
	}
	a.architectureAnchors[id] = BehaviorAnchor{
		ID: id, Kind: kind, ProofMode: proofMode, Label: label, Location: location, Scenario: a.scenario,
		Producer:  producer,
		Certainty: "static", AssociatedMembers: canonicalMembers, Limitations: []string{limitation},
	}
	return id
}

func canonicalArchitectureAnchorMembers(members []Symbol) []Symbol {
	result := append([]Symbol(nil), members...)
	for index := range result {
		result[index].EquivalentIDs = append([]string(nil), result[index].EquivalentIDs...)
		sort.Strings(result[index].EquivalentIDs)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i]
		right := result[j]
		leftKey := strings.Join([]string{
			left.ID, left.Package, left.Name, left.Location.Path,
			strconv.Itoa(left.Location.Line), strconv.Itoa(left.Location.Column),
			strings.Join(left.EquivalentIDs, "\x00"),
		}, "\x00")
		rightKey := strings.Join([]string{
			right.ID, right.Package, right.Name, right.Location.Path,
			strconv.Itoa(right.Location.Line), strconv.Itoa(right.Location.Column),
			strings.Join(right.EquivalentIDs, "\x00"),
		}, "\x00")
		return leftKey < rightKey
	})
	return result
}

func writeArchitectureAnchorIdentityField(writer io.Writer, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = io.WriteString(writer, value)
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
		a.architectureRelationshipsConsidered++
		if len(a.architectureRelationships) >= maxCollectedArchitectureRelationships {
			a.architectureRelationshipCollectionLimited = true
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

type entryHandoffWitness struct {
	processEntrypoint Symbol
	callee            Symbol
	callsite          Location
}

// recordEntryHandoffs inspects only the bodies of already built, exact
// production process-entry SSA functions. It neither traverses the call graph
// nor creates Architecture anchors or relationships.
func (a *analyzer) recordEntryHandoffs(entrypoints []*ssa.Function) {
	var witnesses []entryHandoffWitness
	for _, entrypoint := range entrypoints {
		if a.ctx.Err() != nil || !a.productionProcessEntrypoint(entrypoint) {
			continue
		}
		entrySymbol := a.symbol(entrypoint)
		for _, block := range entrypoint.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if !ok || call.Common() == nil || call.Common().IsInvoke() {
					continue
				}
				callee := call.Common().StaticCallee()
				if callee == nil || callee == entrypoint || callee.Name() == "init" ||
					!a.repositoryDirectStaticCall(call, callee) {
					continue
				}
				calleeSymbol := a.symbol(callee)
				callsite := a.location(call.Pos())
				if !validEntryHandoffSymbol(entrySymbol) || !validEntryHandoffSymbol(calleeSymbol) ||
					!validEntryHandoffLocation(callsite) {
					continue
				}
				witnesses = append(witnesses, entryHandoffWitness{
					processEntrypoint: entrySymbol,
					callee:            calleeSymbol,
					callsite:          callsite,
				})
			}
		}
	}

	sort.Slice(witnesses, func(i, j int) bool {
		left := entryHandoffWitnessKey(witnesses[i])
		right := entryHandoffWitnessKey(witnesses[j])
		return left < right
	})

	all := make([]EntryHandoff, 0, len(witnesses))
	for start := 0; start < len(witnesses); {
		end := start + 1
		edgeKey := entryHandoffEdgeKey(witnesses[start])
		for end < len(witnesses) && entryHandoffEdgeKey(witnesses[end]) == edgeKey {
			end++
		}
		witness := witnesses[start]
		handoff := EntryHandoff{
			ProcessEntrypoint:      witness.processEntrypoint,
			Callee:                 witness.callee,
			RepresentativeCallsite: witness.callsite,
			WitnessCount:           end - start,
			TargetPackage:          witness.callee.Package,
			Scenario:               a.scenario,
			Certainty:              "static",
			Producer: Provenance{
				Provider:  "go_ssa",
				Version:   AnalyzerVersion,
				Operation: "collect_entry_direct_static_handoff",
			},
			Limitations: []string{
				"Exact repository-local direct static call from a build-selected production process entry; runtime order, successful execution, ownership, and transitive reachability are not observed.",
			},
		}
		handoff.ID = stableEntryHandoffID(handoff)
		all = append(all, handoff)
		start = end
	}

	var coverage EntryHandoffCoverage
	var collectionLimited bool
	all, coverage, collectionLimited = boundCollectedEntryHandoffs(
		all,
		len(witnesses),
		maxCollectedEntryHandoffs,
	)
	if collectionLimited {
		a.addBudget("entry_handoff_collection")
	}
	a.entryHandoffs = all
	a.entryHandoffCoverage = coverage
}

func boundCollectedEntryHandoffs(
	handoffs []EntryHandoff,
	witnessesConsidered int,
	limit int,
) ([]EntryHandoff, EntryHandoffCoverage, bool) {
	result := append([]EntryHandoff(nil), handoffs...)
	coverage := EntryHandoffCoverage{
		Complete:             true,
		Reasons:              []GroundingCoverageReason{},
		CandidateSetSHA256:   entryHandoffCandidateSetSHA256(result),
		CandidatesConsidered: len(result),
		CandidatesCollected:  len(result),
		WitnessesConsidered:  witnessesConsidered,
	}
	limited := limit >= 0 && len(result) > limit
	if limited {
		result = result[:limit]
		coverage.CandidatesCollected = len(result)
		coverage.Complete = false
		coverage.Reasons = append(coverage.Reasons, GroundingCoverageCollectionLimit)
	}
	return result, coverage, limited
}

func (a *analyzer) productionProcessEntrypoint(function *ssa.Function) bool {
	if function == nil || function.Blocks == nil || function.Name() != "main" {
		return false
	}
	symbol := a.symbol(function)
	for _, entrypoint := range a.processEntrypoints {
		if entrypoint.availability != AvailabilityAvailable ||
			(entrypoint.role != ExecutableRolePrimaryApplication &&
				entrypoint.role != ExecutableRoleSecondaryService) {
			continue
		}
		if entrypoint.packagePath == symbol.Package && entrypoint.anchor.Path == symbol.Location.Path &&
			entrypoint.anchor.Line == symbol.Location.Line {
			return true
		}
	}
	return false
}

func validEntryHandoffSymbol(symbol Symbol) bool {
	return symbol.ID != "" && symbol.Package != "" && symbol.Name != "" &&
		validEntryHandoffLocation(symbol.Location)
}

func validEntryHandoffLocation(location Location) bool {
	return location.Path != "" && location.Path != "." && location.Line > 0 && location.Column >= 0 &&
		fs.ValidPath(location.Path) && !strings.ContainsRune(location.Path, '\\')
}

func entryHandoffEdgeKey(witness entryHandoffWitness) string {
	return strings.Join([]string{
		witness.processEntrypoint.ID,
		locationKey(witness.processEntrypoint.Location),
		witness.callee.ID,
		locationKey(witness.callee.Location),
	}, "\x00")
}

func entryHandoffWitnessKey(witness entryHandoffWitness) string {
	return entryHandoffEdgeKey(witness) + "\x00" + locationKey(witness.callsite)
}

func stableEntryHandoffID(handoff EntryHandoff) string {
	digest := sha256.New()
	for _, field := range []string{
		"entry-handoff-v1",
		handoff.ProcessEntrypoint.ID,
		locationKey(handoff.ProcessEntrypoint.Location),
		handoff.Callee.ID,
		locationKey(handoff.Callee.Location),
		handoff.Scenario.ID,
	} {
		writeArchitectureAnchorIdentityField(digest, field)
	}
	return "entry-handoff-" + hex.EncodeToString(digest.Sum(nil)[:12])
}

func entryHandoffCandidateSetSHA256(handoffs []EntryHandoff) string {
	digest := sha256.New()
	for _, handoff := range handoffs {
		for _, field := range []string{
			handoff.ID,
			handoff.ProcessEntrypoint.ID,
			locationKey(handoff.ProcessEntrypoint.Location),
			handoff.Callee.ID,
			locationKey(handoff.Callee.Location),
			locationKey(handoff.RepresentativeCallsite),
			strconv.Itoa(handoff.WitnessCount),
			handoff.TargetPackage,
			handoff.Scenario.ID,
		} {
			writeArchitectureAnchorIdentityField(digest, field)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
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
	declarationFamilyMembersPublished := 0
	for _, anchor := range anchors {
		if anchor.ProofMode == componentmap.AnchorProofDeclarationFamily {
			declarationFamilyMembersPublished += len(anchor.AssociatedMembers)
		}
	}
	groundingCoverage := a.architectureGroundingCoverage(
		a.architectureAnchorsConsidered,
		len(anchors),
		a.architectureRelationshipsConsidered,
		len(relationships),
		declarationFamilyMembersPublished,
		groundingBounded,
	)
	entryHandoffs := append([]EntryHandoff(nil), a.entryHandoffs...)
	entryHandoffCoverage := a.entryHandoffCoverage
	var handoffPersistenceLimited bool
	entryHandoffs, entryHandoffCoverage, handoffPersistenceLimited = boundEntryHandoffs(
		entryHandoffs,
		entryHandoffCoverage,
		maxPersistedEntryHandoffs,
	)
	if handoffPersistenceLimited {
		a.addBudget("entry_handoff_persistence")
	}
	groundingCoverage.EntryHandoffs = entryHandoffCoverage
	if !entryHandoffCoverage.Complete {
		groundingCoverage.Complete = false
		groundingCoverage.Reasons = append(groundingCoverage.Reasons, entryHandoffCoverage.Reasons...)
		sort.Slice(groundingCoverage.Reasons, func(i, j int) bool {
			return groundingCoverage.Reasons[i] < groundingCoverage.Reasons[j]
		})
		groundingCoverage.Reasons = compactGroundingCoverageReasons(groundingCoverage.Reasons)
	}
	operationalAnchorIDs := make(map[string]bool, len(anchors))
	reachable := make(map[string]bool)
	queue := make([]string, 0)
	for _, anchor := range anchors {
		if a.ctx.Err() != nil {
			return
		}
		if !architectureOperationalProofMode(anchor.ProofMode) {
			continue
		}
		operationalAnchorIDs[anchor.ID] = true
		if anchor.Kind == "process_entry" && anchor.ProofMode == componentmap.AnchorProofProcessEntry {
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
			if relationship.From != current || !operationalAnchorIDs[relationship.To] || reachable[relationship.To] {
				continue
			}
			reachable[relationship.To] = true
			queue = append(queue, relationship.To)
		}
	}
	kinds, reachableKinds := architectureOperationalKindSets(anchors, reachable)
	mode := architectureGroundingMode(kinds, reachableKinds)

	processEntryCount := 0
	for _, anchor := range anchors {
		if anchor.Kind == "process_entry" && anchor.ProofMode == componentmap.AnchorProofProcessEntry {
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
	case a.hasOnlyAuxiliaryProcessEntrypoints(processEntryCount) && a.hasBuildSelectedLibraryPackage():
		archetype = "library_framework"
		evidenceItems = append(evidenceItems, "all exact process entrypoints are examples or test helpers; a non-main library package is build-selected")
		alternatives = append(alternatives, "application")
	case architectureHasModularPlatformServerShape(kinds):
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
		GroundingMode:       mode, Anchors: anchors, Relationships: relationships, EntryHandoffs: entryHandoffs,
		Coverage: groundingCoverage,
	}
	if a.ctx.Err() != nil {
		return
	}
	a.result.normalize()
}

func boundEntryHandoffs(
	handoffs []EntryHandoff,
	coverage EntryHandoffCoverage,
	limit int,
) ([]EntryHandoff, EntryHandoffCoverage, bool) {
	result := append([]EntryHandoff(nil), handoffs...)
	if coverage.CandidateSetSHA256 == "" {
		coverage.CandidateSetSHA256 = entryHandoffCandidateSetSHA256(result)
		coverage.CandidatesConsidered = len(result)
		coverage.CandidatesCollected = len(result)
		for _, handoff := range result {
			coverage.WitnessesConsidered += handoff.WitnessCount
		}
	}
	limited := limit >= 0 && len(result) > limit
	if limited {
		result = result[:limit]
		coverage.Reasons = append(coverage.Reasons, GroundingCoveragePersistenceLimit)
	}
	coverage.CandidatesPublished = len(result)
	sort.Slice(coverage.Reasons, func(i, j int) bool {
		return coverage.Reasons[i] < coverage.Reasons[j]
	})
	coverage.Reasons = compactGroundingCoverageReasons(coverage.Reasons)
	coverage.Complete = len(coverage.Reasons) == 0
	return result, coverage, limited
}

func architectureOperationalKindSets(
	anchors []BehaviorAnchor,
	reachable map[string]bool,
) (map[string]bool, map[string]bool) {
	kinds := make(map[string]bool)
	reachableKinds := make(map[string]bool)
	for _, anchor := range anchors {
		if !architectureOperationalProofMode(anchor.ProofMode) {
			continue
		}
		kinds[anchor.Kind] = true
		if reachable[anchor.ID] {
			reachableKinds[anchor.Kind] = true
		}
	}
	return kinds, reachableKinds
}

func architectureOperationalProofMode(proofMode componentmap.AnchorProofMode) bool {
	return proofMode == componentmap.AnchorProofProcessEntry ||
		proofMode == componentmap.AnchorProofCallTarget
}

func architectureGroundingMode(kinds, reachableKinds map[string]bool) string {
	pillars := architecturePillarCount(kinds)
	reachablePillars := architecturePillarCount(reachableKinds)
	if kinds["process_entry"] && reachablePillars >= 4 {
		return "behavior_grounded"
	}
	if kinds["process_entry"] && pillars >= 2 {
		return "mixed"
	}
	return "package_landscape"
}

func architecturePillarCount(kinds map[string]bool) int {
	groups := [][]string{
		{"command_dispatch"},
		{"config_ingress", "config_adapter", "config_apply"},
		{"registry_write", "registry_lookup", "extension_family"},
		{"lifecycle_interface", "lifecycle_start"},
		{"admin_control_plane"},
		{"request_dispatch_root", "application_data_plane"},
		{"tls_or_security_boundary"},
	}
	pillars := 0
	for _, group := range groups {
		for _, kind := range group {
			if kinds[kind] {
				pillars++
				break
			}
		}
	}
	return pillars
}

func architectureHasModularPlatformServerShape(kinds map[string]bool) bool {
	return (kinds["registry_write"] || kinds["extension_family"]) &&
		(kinds["request_dispatch_root"] || kinds["admin_control_plane"])
}

func (a *analyzer) architectureGroundingCoverage(
	anchorsConsidered int,
	anchorsPublished int,
	relationshipsConsidered int,
	relationshipsPublished int,
	declarationFamilyMembersPublished int,
	persistenceLimited bool,
) GroundingCoverage {
	coverageReasons := make([]GroundingCoverageReason, 0, 2)
	collectionLimited := a.architectureAnchorCollectionLimited ||
		a.architectureRelationshipCollectionLimited
	if collectionLimited {
		coverageReasons = append(coverageReasons, GroundingCoverageCollectionLimit)
	}
	if persistenceLimited {
		coverageReasons = append(coverageReasons, GroundingCoveragePersistenceLimit)
	}
	return GroundingCoverage{
		Complete:                           !collectionLimited && !persistenceLimited,
		Reasons:                            coverageReasons,
		AnchorsConsidered:                  anchorsConsidered,
		AnchorsPublished:                   anchorsPublished,
		RelationshipsConsidered:            relationshipsConsidered,
		RelationshipsPublished:             relationshipsPublished,
		DeclarationFamilyMembersConsidered: a.declarationFamilyMembersConsidered,
		DeclarationFamilyMembersPublished:  declarationFamilyMembersPublished,
	}
}

func (a *analyzer) hasOnlyAuxiliaryProcessEntrypoints(processEntryCount int) bool {
	if processEntryCount == 0 || len(a.processEntrypoints) != processEntryCount {
		return false
	}
	for _, entrypoint := range a.processEntrypoints {
		if entrypoint.role != ExecutableRoleTestOrHelper {
			return false
		}
	}
	return true
}

func (a *analyzer) hasBuildSelectedLibraryPackage() bool {
	for _, pkg := range a.packages {
		if pkg != nil && pkg.Pkg != nil && pkg.Pkg.Name() != "main" {
			return true
		}
	}
	return false
}

const (
	maxCollectedArchitectureAnchors       = 1024
	maxCollectedArchitectureRelationships = 4096
	maxPersistedArchitectureAnchors       = 256
	maxPersistedArchitectureRelationships = 512
	maxArchitectureAnchorsPerKind         = 16
	maxProcessEntryArchitectureAnchors    = 64
	maxArchitectureRelationshipWitnesses  = 64
	maxCollectedEntryHandoffs             = 4096
	maxPersistedEntryHandoffs             = 256
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
	a.result.Coverage.ScopeStatement = "exact build-selected process entries, typed Cobra commands, runtime registrations and starts found through safe typed package closures, bounded wrapper propagation, and the generic typed registration detector (path string + handler) under the recorded build scenario, subject to listed diagnostics and frontiers"
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
		// Decision 220 D: per-framework matched counts so coverage shows
		// exactly which adapters produced records (catalog vs detector).
		frameworkKey := trigger.Framework
		if frameworkKey == "" {
			frameworkKey = "unknown"
		}
		if trigger.Producer == "typed_registration_detector" {
			a.result.Coverage.TypedRegistrationDetectorMatches++
		}
		a.result.Coverage.FrameworkMatched[frameworkKey]++
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
	if a.functionIDs == nil {
		a.functionIDs = map[*ssa.Function]string{}
	}
	if id, found := a.functionIDs[function]; found {
		return id
	}
	path := functionPackagePath(function)
	if path == "" {
		path = "synthetic"
	}
	receiver := receiverName(function.Signature)
	var id string
	if receiver != "" {
		id = path + ".(" + receiver + ")." + function.Name()
	} else {
		id = path + "." + function.Name()
	}
	a.functionIDs[function] = id
	return id
}

func (a *analyzer) location(position token.Pos) Location {
	resolved := a.program.Fset.PositionFor(position, true)
	path := resolved.Filename
	if relative, err := filepath.Rel(a.root, path); err == nil && !strings.HasPrefix(relative, "..") {
		path = filepath.ToSlash(relative)
	} else if path != "" {
		// Decision 235 (v11) 1D container-registry: out-of-root files
		// ($GOROOT/stdlib, module cache) are marked external instead of
		// leaking an absolute host path that later becomes a required
		// repository source action. Local callsite evidence is retained.
		path = "<external>/" + filepath.Base(path)
	}
	return Location{Path: path, Line: resolved.Line, Column: resolved.Column}
}

func (a *analyzer) sourceLocation(filename string, line, column int) Location {
	path := filename
	if relative, err := filepath.Rel(a.root, filename); err == nil && !strings.HasPrefix(relative, "..") {
		path = filepath.ToSlash(relative)
	} else if path != "" {
		path = "<external>/" + filepath.Base(path)
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
	packagePath := functionPackagePath(function)
	for modulePath := range a.modulePaths {
		if packagePath == modulePath || strings.HasPrefix(packagePath, modulePath+"/") {
			return true
		}
	}
	return false
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

func (a *analyzer) mergeValues(values []Value) Value {
	if len(values) == 0 {
		return dynamicValue("no values")
	}
	if len(values) > maxValueAlternatives {
		a.addBudget("value_alternatives")
		values = values[:maxValueAlternatives]
	}
	first := values[0]
	candidates := make([]string, 0, len(values))
	for _, value := range values {
		if len(value.Text) > maxValueDescriptionBytes {
			a.addBudget("value_description")
			return dynamicValue("unresolved value")
		}
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
		joined := strings.Join(first.Candidates, " | ")
		if len(joined) > maxValueDescriptionBytes {
			a.addBudget("value_description")
			return dynamicValue("unresolved value")
		}
		first.Text = joined
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
