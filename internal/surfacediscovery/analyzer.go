package surfacediscovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/gotarget"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type analyzer struct {
	ctx            context.Context
	opts           Options
	input          Input
	program        *ssa.Program
	packages       []*ssa.Package
	packageFacts   map[string]*packages.Package
	loadedPackages map[string]*packages.Package
	// admittedPackages is the exact target-scoped repository package set. A
	// prepared workspace may contain sibling target packages in the same SSA
	// program; no projection is allowed to observe those siblings.
	admittedPackages map[string]bool
	allFunctions     map[*ssa.Function]bool
	root             string
	modulePaths      map[string]bool
	scenario         Scenario
	result           Result

	functionIDs map[*ssa.Function]string

	currentPhase        string
	currentPhaseStarted time.Time

	directCallIndex       *directCallIndexBuilder
	externalCallIndex     *ExternalCallIndexBuilder
	externalCallIndexErr  error
	dynamicHandoffCapture *dynamicHandoffCapture
	callableBindings      callableBindingSnapshot

	packageLoadCalls int
	ssaBuilds        int
}

// AnalyzeContextWithInput loads one sealed target boundary and projects all
// exact local artifacts in one typed-program pass.
func AnalyzeContextWithInput(ctx context.Context, opts Options, input Input) (Result, error) {
	workspace, err := PrepareWorkspace(ctx, opts, []Input{input})
	if err != nil {
		return Result{}, err
	}
	return workspace.Analyze(ctx, opts, input)
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
	if a.input.AnalysisTarget == nil || a.input.AnalysisTarget.ModuleDir == "" {
		return fmt.Errorf("surface discovery: target module directory is required")
	}
	moduleRoot := filepath.Join(
		a.root, filepath.FromSlash(a.input.AnalysisTarget.ModuleDir),
	)
	patterns := make([]string, 0, len(a.input.Packages))
	for _, pkg := range a.input.Packages {
		patterns = append(patterns, pkg.Path)
	}
	sort.Strings(patterns)
	if len(patterns) == 0 {
		return fmt.Errorf("surface discovery: no target-selected Go packages")
	}
	// An explicit empty value is required too: packages.Load otherwise inherits
	// GOFLAGS=-tags=... while the persisted scenario truthfully records no tags.
	buildFlags := []string{"-tags=" + strings.Join(a.opts.BuildTags, ",")}
	finishLoad := a.startPhase("package_load", "loading target-selected packages and dependency types")
	fileSet := token.NewFileSet()
	loadEnv := gotarget.Target{GOOS: a.scenario.GOOS, GOARCH: a.scenario.GOARCH}.ApplyEnv(os.Environ())
	loadEnv = append(loadEnv, "GOTOOLCHAIN=auto")
	config := &packages.Config{
		Context: a.ctx, Dir: moduleRoot, Env: loadEnv, Fset: fileSet,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedExportFile | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes |
			packages.NeedModule,
		BuildFlags: buildFlags, Tests: false,
	}
	loaded, err := packages.Load(config, patterns...)
	a.packageLoadCalls++
	if ctxErr := a.ctx.Err(); ctxErr != nil {
		finishLoad(len(loaded), len(patterns))
		return fmt.Errorf("surface discovery: %w", ctxErr)
	}
	if err != nil {
		finishLoad(len(loaded), len(patterns))
		return fmt.Errorf(
			"surface discovery: load target packages from %s: %w",
			repositoryRelativeModuleDir(a.root, moduleRoot), err,
		)
	}
	a.emitPhaseProgress(len(loaded), len(patterns), "loaded package roots")
	finishLoad(len(loaded), len(patterns))
	if len(loaded) == 0 {
		return fmt.Errorf("surface discovery: no target-selected Go packages")
	}

	allPackages := make(map[string]*packages.Package)
	var identityErr error
	packages.Visit(loaded, func(pkg *packages.Package) bool {
		if identityErr != nil {
			return false
		}
		if pkg != nil && pkg.PkgPath != "" {
			if prior, exists := allPackages[pkg.PkgPath]; exists && prior != pkg {
				identityErr = fmt.Errorf(
					"surface discovery: one Go load universe produced distinct package identities for path %q; internal repomap package/type invariant failed; repository changes are not required",
					pkg.PkgPath,
				)
				return false
			}
			allPackages[pkg.PkgPath] = pkg
			if packageSafeForSSA(pkg) {
				if prior, exists := a.packageFacts[pkg.PkgPath]; exists && prior != pkg {
					identityErr = fmt.Errorf(
						"surface discovery: one Go load universe produced distinct typed facts for path %q; internal repomap package/type invariant failed; repository changes are not required",
						pkg.PkgPath,
					)
					return false
				}
				a.packageFacts[pkg.PkgPath] = pkg
			}
		}
		return true
	}, nil)
	if identityErr != nil {
		return identityErr
	}
	a.loadedPackages = allPackages
	// Package-load diagnostics are complete load authority, including when the
	// selected target is not SSA-safe and validation returns immediately below.
	// Recording them after validation would erase the concrete failure evidence
	// from precisely the runs that need it most.
	a.recordPackageLoadOutcomes(allPackages)
	if err := a.validateAnalysisTargetPackages(allPackages); err != nil {
		return err
	}
	for _, pkg := range allPackages {
		if pkg != nil && pkg.Module != nil && pkg.Module.Main {
			a.modulePaths[pkg.Module.Path] = true
		}
	}
	safeLoaded := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		if packageSafeForSSA(pkg) {
			safeLoaded = append(safeLoaded, pkg)
		}
	}
	if len(safeLoaded) == 0 {
		return fmt.Errorf("surface discovery: selected target has no SSA-safe packages")
	}

	finishSSA := a.startPhase("ssa_build", "building one typed target program")
	a.program, a.packages = buildSurfaceSSAProgram(fileSet, safeLoaded)
	a.ssaBuilds++
	ssaPackages := append([]*ssa.Package(nil), a.packages...)
	sort.Slice(ssaPackages, func(i, j int) bool { return ssaPackagePath(ssaPackages[i]) < ssaPackagePath(ssaPackages[j]) })
	for index, pkg := range ssaPackages {
		if err := a.ctx.Err(); err != nil {
			return fmt.Errorf("surface discovery: %w", err)
		}
		if pkg != nil {
			pkg.Build()
		}
		if (index+1)%50 == 0 {
			a.emitPhaseProgress(index+1, len(ssaPackages), "SSA packages")
		}
	}
	finishSSA(len(ssaPackages), len(ssaPackages))
	a.allFunctions = ssautil.AllFunctions(a.program)
	return nil
}

func (a *analyzer) validateAnalysisTargetPackages(allPackages map[string]*packages.Package) error {
	target := a.input.AnalysisTarget
	for _, packagePath := range target.TargetPackages {
		pkg := allPackages[packagePath]
		if !packageSafeForSSA(pkg) {
			targetErr := &AnalysisTargetSSAUnavailableError{
				Reason: AnalysisTargetPackageNotSSASafe, Package: packagePath,
				Diagnostic: a.analysisTargetSSADiagnostic(allPackages, packagePath),
			}
			targetErr.bindProgramCoverage(a.result.Coverage)
			return targetErr
		}
		if pkg.Module == nil || pkg.Module.Path != target.ModulePath {
			return fmt.Errorf(
				"surface discovery: target package %q does not belong to sealed module %q",
				packagePath, target.ModulePath,
			)
		}
		moduleDir, ok := repositoryPackageModuleDirectory(a.root, pkg)
		if !ok || moduleDir != target.ModuleDir {
			return fmt.Errorf("surface discovery: target package %q has inconsistent sealed module directory", packagePath)
		}
		if target.Kind == AnalysisTargetModuleLibrary && pkg.Name == "main" {
			return fmt.Errorf("surface discovery: module library target package %q is executable", packagePath)
		}
	}
	return nil
}

// repositoryPackageModuleDirectory resolves the repository-local module owner
// exactly as the Go command did. A local replacement is authoritative over the
// original module cache directory; neither absolute path is persisted.
func repositoryPackageModuleDirectory(root string, pkg *packages.Package) (string, bool) {
	if pkg == nil || pkg.Module == nil {
		return "", false
	}
	if pkg.Module.Replace != nil && pkg.Module.Replace.Dir != "" {
		if directory, ok := containedModuleDirectory(root, pkg.Module.Replace.Dir); ok {
			return directory, true
		}
	}
	return containedModuleDirectory(root, pkg.Module.Dir)
}

func (a *analyzer) materializeSelectedLibraryCallables() []*ssa.Function {
	if a.program == nil || a.input.AnalysisTarget.Kind != AnalysisTargetModuleLibrary {
		return nil
	}
	packagesByPath := make(map[string]*ssa.Package, len(a.packages))
	for _, pkg := range a.packages {
		if pkg != nil && pkg.Pkg != nil {
			packagesByPath[pkg.Pkg.Path()] = pkg
		}
	}
	var functions []*ssa.Function
	seen := make(map[*ssa.Function]struct{})
	appendFunction := func(function *ssa.Function) {
		if function == nil {
			return
		}
		if origin := function.Origin(); origin != nil {
			function = origin
		}
		if function.Blocks == nil {
			return
		}
		if _, duplicate := seen[function]; duplicate {
			return
		}
		seen[function] = struct{}{}
		functions = append(functions, function)
	}
	for _, packagePath := range a.input.AnalysisTarget.TargetPackages {
		pkg := packagesByPath[packagePath]
		if pkg == nil || pkg.Pkg == nil || pkg.Pkg.Name() == "main" {
			continue
		}
		scope := pkg.Pkg.Scope()
		for _, name := range scope.Names() {
			switch object := scope.Lookup(name).(type) {
			case *types.Func:
				if object.Exported() && object.Pkg() != nil && object.Pkg().Path() == packagePath {
					appendFunction(a.program.FuncValue(object))
				}
			case *types.TypeName:
				named, ok := object.Type().(*types.Named)
				if !ok {
					continue
				}
				for index := 0; index < named.NumMethods(); index++ {
					method := named.Method(index)
					if method != nil && method.Exported() && method.Pkg() != nil && method.Pkg().Path() == packagePath {
						appendFunction(a.program.FuncValue(method))
					}
				}
			}
		}
	}
	sort.Slice(functions, func(i, j int) bool {
		left, right := a.location(functions[i].Pos()), a.location(functions[j].Pos())
		if directCallLocationLess(left, right) {
			return true
		}
		if directCallLocationLess(right, left) {
			return false
		}
		return a.functionID(functions[i]) < a.functionID(functions[j])
	})
	return functions
}

func buildSurfaceSSAProgram(fileSet *token.FileSet, loaded []*packages.Package) (*ssa.Program, []*ssa.Package) {
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
			files, info = source.Syntax, source.TypesInfo
		}
		created[typePackage] = program.CreatePackage(typePackage, files, info, true)
		return created[typePackage]
	}
	result := make([]*ssa.Package, 0, len(loaded))
	for _, pkg := range loaded {
		if packageSafeForSSA(pkg) {
			result = append(result, create(pkg.Types))
		}
	}
	return program, result
}

func repositoryRelativeModuleDir(root, moduleRoot string) string {
	relative, err := filepath.Rel(root, moduleRoot)
	if err != nil || relative == "" || relative == "." {
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

func (a *analyzer) prepareTargetProgram() {
	functions := a.orderedFunctions()
	for index, function := range functions {
		if a.ctx.Err() != nil {
			return
		}
		if function == nil || function.Blocks == nil {
			continue
		}
		a.directCallIndex.recordFunction(a, function)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if store, ok := instruction.(*ssa.Store); ok {
					a.dynamicHandoffCapture.observeCallableBinding(a, store)
				}
				call, ok := instruction.(ssa.CallInstruction)
				if !ok {
					continue
				}
				a.observeExternalCallIndex(call)
				a.observeDynamicHandoffs(call)
			}
		}
		if (index+1)%1000 == 0 {
			a.emitPhaseProgress(index+1, len(functions), "SSA functions indexed")
		}
	}
	a.callableBindings = a.dynamicHandoffCapture.freezeCallableBindings()
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

func (a *analyzer) entrypoints() []*ssa.Function {
	result := []*ssa.Function{}
	for _, pkg := range a.packages {
		if pkg == nil || pkg.Pkg == nil || pkg.Pkg.Name() != "main" {
			continue
		}
		function, ok := pkg.Members["main"].(*ssa.Function)
		if ok && function.Blocks != nil {
			result = append(result, function)
		}
	}
	sort.Slice(result, func(i, j int) bool { return a.functionID(result[i]) < a.functionID(result[j]) })
	return result
}

func importedPackagePaths(entrypoint *ssa.Function) map[string]bool {
	result := make(map[string]bool)
	if entrypoint == nil || entrypoint.Pkg == nil || entrypoint.Pkg.Pkg == nil {
		return result
	}
	var visit func(*types.Package)
	visit = func(pkg *types.Package) {
		if pkg == nil || result[pkg.Path()] {
			return
		}
		result[pkg.Path()] = true
		for _, imported := range pkg.Imports() {
			visit(imported)
		}
	}
	visit(entrypoint.Pkg.Pkg)
	return result
}

func (a *analyzer) repositoryDirectStaticCall(call ssa.CallInstruction, target *ssa.Function) bool {
	return call != nil && target != nil && call.Parent() != nil &&
		call.Common() != nil && call.Common().StaticCallee() == target &&
		a.isRepositoryFunction(call.Parent()) && a.repositorySourceFunction(target)
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

func validEntryHandoffSymbol(symbol Symbol) bool {
	return symbol.ID != "" && symbol.Package != "" && symbol.Name != "" &&
		validEntryHandoffLocation(symbol.Location)
}

func validEntryHandoffLocation(location Location) bool {
	return location.Path != "" && location.Path != "." && location.Line > 0 && location.Column >= 0 &&
		fs.ValidPath(location.Path) && !strings.ContainsRune(location.Path, '\\')
}

func (a *analyzer) symbol(function *ssa.Function) Symbol {
	return Symbol{
		ID: a.functionID(function), Package: functionPackagePath(function),
		Name: function.Name(), Location: a.location(function.Pos()),
	}
}

func (a *analyzer) functionID(function *ssa.Function) string {
	if function == nil {
		return "unknown"
	}
	if id, found := a.functionIDs[function]; found {
		return id
	}
	packagePath := functionPackagePath(function)
	if packagePath == "" {
		packagePath = "synthetic"
	}
	receiver := receiverName(function.Signature)
	id := packagePath + "." + function.Name()
	if receiver != "" {
		id = packagePath + ".(" + receiver + ")." + function.Name()
	}
	a.functionIDs[function] = id
	return id
}

func (a *analyzer) location(position token.Pos) Location {
	resolved := a.program.Fset.PositionFor(position, true)
	path := resolved.Filename
	if relative, err := filepath.Rel(a.root, path); err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		path = filepath.ToSlash(relative)
	} else if path != "" {
		path = "<external>/" + filepath.Base(path)
	}
	return Location{Path: path, Line: resolved.Line, Column: resolved.Column}
}

func (a *analyzer) isRepositoryFunction(function *ssa.Function) bool {
	packagePath := functionPackagePath(function)
	return packagePath != "" && a.admittedPackages[packagePath]
}

// repositorySourceFunction is stricter than package ownership: compiler
// helpers for cgo share the repository package path but have no repository
// declaration location and therefore cannot be repository graph nodes.
func (a *analyzer) repositorySourceFunction(function *ssa.Function) bool {
	if !a.isRepositoryFunction(function) {
		return false
	}
	location := a.location(function.Pos())
	return validRepositoryDirectCallLocation(location) && location.Column > 0
}

func scenarioID(goos, goarch string, tags []string) string {
	copyTags := append([]string(nil), tags...)
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

func locationKey(location Location) string {
	return location.Path + ":" + strconv.Itoa(location.Line) + ":" + strconv.Itoa(location.Column)
}
