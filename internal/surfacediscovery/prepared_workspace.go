package surfacediscovery

import (
	"context"
	"fmt"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/gotarget"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// PreparedWorkspace owns the one packages/types/SSA lifetime shared by every
// selected Go target in an ordinary run. It is deliberately live-run-only:
// callers cannot encode it, restore it in another run, or replace an exact
// target binding after preparation.
//
// Analyze projects independently allocated target artifacts from this typed
// workspace. Sibling packages that were loaded for another selected target
// remain outside the projection's exact admitted package set.
type PreparedWorkspace struct {
	root                string
	goTarget            string
	buildTags           []string
	scenario            Scenario
	bindings            map[string]Input
	program             *ssa.Program
	packages            []*ssa.Package
	packageFacts        map[string]*packages.Package
	loadedPackages      map[string]*packages.Package
	allFunctions        map[*ssa.Function]bool
	preparationCoverage ProgramCoverage

	// These counters are intentionally private instrumentation. Focused tests
	// prove that target projections cannot accidentally repeat the expensive
	// preparation work.
	packageLoadCalls int
	ssaBuilds        int
}

// PrepareWorkspace loads the union of exact package scopes advertised by the
// selected inputs and builds one SSA program. Every target is validated before
// the workspace is returned, so a later Analyze never falls back to a new load.
func PrepareWorkspace(ctx context.Context, opts Options, inputs []Input) (*PreparedWorkspace, error) {
	normalizedOpts, root, scenario, err := normalizeWorkspaceOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("surface discovery: prepared workspace requires at least one exact target")
	}

	normalized := make([]Input, 0, len(inputs))
	bindings := make(map[string]Input, len(inputs))
	union := Input{}
	for _, input := range inputs {
		owned, normalizeErr := normalizeInput(input)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		ref := owned.AnalysisTarget.TargetRef
		if prior, duplicate := bindings[ref]; duplicate {
			if !reflect.DeepEqual(prior, owned) {
				return nil, fmt.Errorf("surface discovery: prepared target %q has conflicting exact scopes", ref)
			}
			continue
		}
		bindings[ref] = cloneInput(owned)
		normalized = append(normalized, owned)
		union.ModuleDirs = append(union.ModuleDirs, owned.ModuleDirs...)
		union.Packages = append(union.Packages, owned.Packages...)
	}
	union.ModuleDirs = normalizeModuleDirs(union.ModuleDirs)
	union.Packages = normalizePackageInputs(union.Packages)
	union.AnalysisTarget = normalized[0].AnalysisTarget

	admitted := make(map[string]bool, len(union.Packages))
	for _, pkg := range union.Packages {
		admitted[pkg.Path] = true
	}
	loader := &analyzer{
		ctx: ctx, opts: normalizedOpts, input: union, root: root,
		packageFacts:     make(map[string]*packages.Package),
		admittedPackages: admitted,
		modulePaths:      make(map[string]bool), functionIDs: make(map[*ssa.Function]string),
		scenario: scenario,
	}
	if err := loader.load(); err != nil {
		return nil, err
	}
	for _, input := range normalized[1:] {
		loader.input = input
		if err := loader.validateAnalysisTargetPackages(loader.loadedPackages); err != nil {
			return nil, err
		}
	}

	return &PreparedWorkspace{
		root: root, goTarget: normalizedOpts.GoTarget,
		buildTags: append([]string(nil), normalizedOpts.BuildTags...), scenario: scenario,
		bindings: bindings, program: loader.program,
		packages:     append([]*ssa.Package(nil), loader.packages...),
		packageFacts: loader.packageFacts, loadedPackages: loader.loadedPackages,
		allFunctions:        loader.allFunctions,
		preparationCoverage: cloneProgramCoverage(loader.result.Coverage),
		packageLoadCalls:    loader.packageLoadCalls, ssaBuilds: loader.ssaBuilds,
	}, nil
}

// Analyze projects one exact target from a prepared workspace. A mismatched
// repository, platform, build-tag set, target, or package scope is an error;
// it never triggers a hidden fresh analysis or a stale best-effort result.
func (workspace *PreparedWorkspace) Analyze(
	ctx context.Context,
	opts Options,
	input Input,
) (Result, error) {
	if workspace == nil {
		return Result{}, fmt.Errorf("surface discovery: prepared workspace is required")
	}
	normalizedOpts, root, scenario, err := normalizeWorkspaceOptions(ctx, opts)
	if err != nil {
		return Result{}, err
	}
	if root != workspace.root || normalizedOpts.GoTarget != workspace.goTarget ||
		scenario.ID != workspace.scenario.ID ||
		!slices.Equal(normalizedOpts.BuildTags, workspace.buildTags) {
		return Result{}, fmt.Errorf("surface discovery: prepared workspace options do not match this run")
	}
	input, err = normalizeInput(input)
	if err != nil {
		return Result{}, err
	}
	bound, ok := workspace.bindings[input.AnalysisTarget.TargetRef]
	if !ok || !reflect.DeepEqual(bound, input) {
		return Result{}, fmt.Errorf(
			"surface discovery: target %q is not bound to this prepared workspace",
			input.AnalysisTarget.TargetRef,
		)
	}

	a, err := workspace.analyzerFor(ctx, normalizedOpts, input)
	if err != nil {
		return Result{}, err
	}
	return analyzePreparedTarget(a)
}

func (workspace *PreparedWorkspace) analyzerFor(
	ctx context.Context,
	opts Options,
	input Input,
) (*analyzer, error) {
	admitted := make(map[string]bool, len(input.Packages))
	modulePaths := make(map[string]bool)
	for _, pkg := range input.Packages {
		admitted[pkg.Path] = true
		facts := workspace.packageFacts[pkg.Path]
		if !packageSafeForSSA(facts) {
			return nil, fmt.Errorf("surface discovery: admitted package %q is unavailable in prepared workspace", pkg.Path)
		}
		if facts.Module != nil && facts.Module.Main && facts.Module.Path != "" {
			modulePaths[facts.Module.Path] = true
		}
	}

	ssaPackages := make([]*ssa.Package, 0, len(input.Packages))
	seenPackages := make(map[*types.Package]struct{}, len(input.Packages))
	for _, pkg := range workspace.packages {
		if pkg == nil || pkg.Pkg == nil || !admitted[pkg.Pkg.Path()] {
			continue
		}
		facts := workspace.packageFacts[pkg.Pkg.Path()]
		if facts == nil || facts.Types != pkg.Pkg {
			continue
		}
		if _, duplicate := seenPackages[pkg.Pkg]; duplicate {
			continue
		}
		seenPackages[pkg.Pkg] = struct{}{}
		ssaPackages = append(ssaPackages, pkg)
	}
	sort.Slice(ssaPackages, func(i, j int) bool {
		return ssaPackagePath(ssaPackages[i]) < ssaPackagePath(ssaPackages[j])
	})
	if len(ssaPackages) != len(input.Packages) {
		return nil, fmt.Errorf("surface discovery: prepared workspace package projection is incomplete")
	}

	functions := make(map[*ssa.Function]bool)
	for function := range workspace.allFunctions {
		if function == nil {
			continue
		}
		packagePath := functionPackagePath(function)
		if !admitted[packagePath] || !functionBelongsToFacts(function, workspace.packageFacts[packagePath]) {
			continue
		}
		functions[function] = true
	}
	a := &analyzer{
		ctx: ctx, opts: opts, input: input, root: workspace.root,
		program: workspace.program, packages: ssaPackages,
		packageFacts: workspace.packageFacts, loadedPackages: workspace.loadedPackages,
		admittedPackages: admitted, allFunctions: functions,
		modulePaths: modulePaths, functionIDs: make(map[*ssa.Function]string),
		scenario: workspace.scenario,
		result:   Result{Coverage: cloneProgramCoverage(workspace.preparationCoverage)},
	}
	a.recordPackageLoadOutcomes(workspace.packageClosure(input))
	for _, function := range a.materializeSelectedLibraryCallables() {
		if function != nil && functionBelongsToFacts(function, workspace.packageFacts[functionPackagePath(function)]) {
			a.allFunctions[function] = true
		}
	}
	a.directCallIndex = newDirectCallIndexBuilderWithLimits(
		a.scenario, MaxDirectCallIndexNodes, opts.DirectCallEdgeLimit,
	)
	a.directCallIndex.setTargetScope(DirectCallIndexScope{
		TargetRef: input.AnalysisTarget.TargetRef, TargetKind: input.AnalysisTarget.Kind,
		TargetModuleID: input.AnalysisTarget.ModuleID, TargetModulePath: input.AnalysisTarget.ModulePath,
		TargetModuleDir: input.AnalysisTarget.ModuleDir, TargetPackage: input.AnalysisTarget.PackagePath,
		TargetPackages: append([]string(nil), input.AnalysisTarget.TargetPackages...),
		MaxDepth:       opts.DirectCallDepth, EdgeLimit: opts.DirectCallEdgeLimit,
	})
	if opts.CaptureEntryCallSubstrate {
		a.directCallIndex.enableEntryCallSidecar()
	}
	return a, nil
}

func analyzePreparedTarget(a *analyzer) (Result, error) {
	if a == nil {
		return Result{}, fmt.Errorf("surface discovery: prepared analyzer is unavailable")
	}
	if a.opts.CaptureCoreObjectIndex {
		index, err := a.captureCoreObjectIndex()
		if err != nil {
			return Result{}, err
		}
		a.result.CoreObjectIndex = &index
	}
	if a.opts.CaptureExternalCallIndex {
		index, err := newAnalyzerExternalCallIndexBuilder(a)
		if err != nil {
			return Result{}, err
		}
		a.externalCallIndex = index
	}
	if a.opts.CaptureDynamicHandoffIndex {
		a.dynamicHandoffCapture = &dynamicHandoffCapture{}
	}

	finishIndex := a.startPhase("program_index", "indexing exact calls and generic activity candidates")
	a.prepareTargetProgram()
	if err := a.ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("surface discovery: %w", err)
	}
	if a.externalCallIndexErr != nil {
		return Result{}, a.externalCallIndexErr
	}
	if a.dynamicHandoffCapture != nil && a.dynamicHandoffCapture.err != nil {
		return Result{}, a.dynamicHandoffCapture.err
	}
	if a.externalCallIndex != nil {
		index, err := a.externalCallIndex.Finish()
		if err != nil {
			return Result{}, err
		}
		a.result.ExternalCallIndex = &index
	}
	if err := a.recordTargetDirectCallEdges(); err != nil {
		return Result{}, err
	}
	finishIndex(len(a.allFunctions), len(a.allFunctions))

	direct := a.directCallIndex.finish()
	a.result.DirectCallIndex = &direct
	if a.dynamicHandoffCapture != nil {
		index, err := a.dynamicHandoffCapture.finish(direct)
		if err != nil {
			return Result{}, err
		}
		a.result.DynamicHandoffIndex = &index
	}
	if a.opts.CaptureEntryCallSubstrate {
		substrate := a.directCallIndex.entryCallSubstrate(a, direct, a.entrypoints())
		a.result.EntryCallSubstrate = &substrate
	}
	a.result.normalize()
	return a.result, nil
}

func normalizeWorkspaceOptions(
	ctx context.Context,
	opts Options,
) (Options, string, Scenario, error) {
	if ctx == nil {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: %w", err)
	}
	if strings.TrimSpace(opts.RepoPath) == "" {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: repository path is required")
	}
	if strings.TrimSpace(opts.GoTarget) == "" {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: resolved Go target is required")
	}
	target, err := gotarget.Parse(opts.GoTarget)
	if err != nil {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: %w", err)
	}
	opts.GoTarget = target.String()
	if opts.DirectCallDepth < 1 {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: direct call depth must be at least 1")
	}
	if opts.DirectCallEdgeLimit < 1 {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: direct call edge limit must be at least 1")
	}
	if opts.DirectCallEdgeLimit > MaxDirectCallIndexEdges {
		return Options{}, "", Scenario{}, fmt.Errorf(
			"surface discovery: direct call edge limit %d exceeds maximum %d",
			opts.DirectCallEdgeLimit, MaxDirectCallIndexEdges,
		)
	}
	root, err := filepath.Abs(opts.RepoPath)
	if err != nil {
		return Options{}, "", Scenario{}, fmt.Errorf("surface discovery: resolve repository: %w", err)
	}
	opts.BuildTags = compactStrings(opts.BuildTags)
	scenario := Scenario{
		ID:   scenarioID(target.GOOS, target.GOARCH, opts.BuildTags),
		GOOS: target.GOOS, GOARCH: target.GOARCH,
		Tags: append([]string(nil), opts.BuildTags...),
	}
	return opts, root, scenario, nil
}

func (workspace *PreparedWorkspace) packageClosure(input Input) map[string]*packages.Package {
	result := make(map[string]*packages.Package)
	queue := make([]*packages.Package, 0, len(input.Packages))
	for _, admitted := range input.Packages {
		if pkg := workspace.loadedPackages[admitted.Path]; pkg != nil {
			queue = append(queue, pkg)
		}
	}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if pkg == nil || pkg.PkgPath == "" {
			continue
		}
		if _, seen := result[pkg.PkgPath]; seen {
			continue
		}
		result[pkg.PkgPath] = pkg
		paths := make([]string, 0, len(pkg.Imports))
		for importPath := range pkg.Imports {
			paths = append(paths, importPath)
		}
		sort.Strings(paths)
		for _, importPath := range paths {
			queue = append(queue, pkg.Imports[importPath])
		}
	}
	return result
}

func functionBelongsToFacts(function *ssa.Function, facts *packages.Package) bool {
	if function == nil || facts == nil || facts.Types == nil {
		return false
	}
	if object := function.Object(); object != nil && object.Pkg() != nil {
		return object.Pkg() == facts.Types
	}
	return function.Pkg != nil && function.Pkg.Pkg == facts.Types
}

func cloneInput(input Input) Input {
	result := input
	result.ModuleDirs = append([]string(nil), input.ModuleDirs...)
	result.Packages = append([]PackageInput(nil), input.Packages...)
	if input.AnalysisTarget != nil {
		target := *input.AnalysisTarget
		target.TargetPackages = append([]string(nil), input.AnalysisTarget.TargetPackages...)
		target.Roots = append([]AnalysisTargetRootInput(nil), input.AnalysisTarget.Roots...)
		result.AnalysisTarget = &target
	}
	return result
}

func cloneProgramCoverage(coverage ProgramCoverage) ProgramCoverage {
	result := coverage
	result.PackageDiagnostics = append([]PackageDiagnostic(nil), coverage.PackageDiagnostics...)
	for index := range result.PackageDiagnostics {
		if coverage.PackageDiagnostics[index].Location != nil {
			location := *coverage.PackageDiagnostics[index].Location
			result.PackageDiagnostics[index].Location = &location
		}
	}
	result.Phases = append([]PhaseMetric(nil), coverage.Phases...)
	return result
}
