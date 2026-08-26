package surfacediscovery

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/gotarget"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// PreparedWorkspace owns the compatible packages/types/SSA lifetimes shared
// by selected Go targets in an ordinary run. Targets rooted in the same module
// resolution context share one load universe. Targets rooted in different
// module contexts retain separate universes so equal package paths can never
// join unrelated *types.Package or *ssa.Package identities. It is deliberately
// live-run-only: callers cannot encode it, restore it in another run, or
// replace an exact target binding after preparation.
//
// Analyze projects independently allocated target artifacts from this typed
// workspace. Sibling packages that were loaded for another selected target
// remain outside the projection's exact admitted package set.
type PreparedWorkspace struct {
	root      string
	goTarget  string
	buildTags []string
	scenario  Scenario
	bindings  map[string]preparedTargetBinding
	universes map[string]*preparedLoadUniverse

	// These counters are intentionally private instrumentation. Focused tests
	// prove that target projections cannot accidentally repeat the expensive
	// preparation work.
	packageLoadCalls int
	ssaBuilds        int
}

type preparedTargetBinding struct {
	input       Input
	universeKey string
}

// preparedLoadUniverse is one coherent packages.Load result and the SSA
// program built exclusively from the *types.Package values owned by that
// result. Maps keyed by package path are populated only after uniqueness in
// this universe has been proved; they are never last-wins indexes.
type preparedLoadUniverse struct {
	program             *ssa.Program
	ssaPackages         map[string]*ssa.Package
	packageFacts        map[string]*packages.Package
	loadedPackages      map[string]*packages.Package
	allFunctions        map[*ssa.Function]bool
	preparationCoverage ProgramCoverage
}

type preparedLoadGroup struct {
	key    string
	inputs []Input
}

const maxPreparedProjectionDiagnosticKeys = 8

func preparedUniverseKey(target *AnalysisTargetInput) string {
	if target == nil {
		return ""
	}
	return target.ModuleDir + "\x00" + target.ModulePath
}

func preparedPackageKey(pkg PackageInput) string {
	return pkg.ModuleDir + ":" + pkg.Path
}

// validatePreparedPackageKeys rejects a package path that claims two local
// module owners inside one Go resolution context. A Go load can resolve only
// one package for an import path; choosing either owner would be arbitrary.
func validatePreparedPackageKeys(values []PackageInput) error {
	owners := make(map[string]string, len(values))
	for _, value := range values {
		key := preparedPackageKey(value)
		if prior, exists := owners[value.Path]; exists && prior != key {
			return fmt.Errorf(
				"surface discovery: prepared load universe has conflicting exact package keys %s; internal repomap package-scope invariant failed; repository changes are not required",
				boundedPreparedKeys([]string{prior, key}),
			)
		}
		owners[value.Path] = key
	}
	return nil
}

func indexPreparedSSAPackages(values []*ssa.Package) (map[string]*ssa.Package, error) {
	result := make(map[string]*ssa.Package, len(values))
	for _, pkg := range values {
		if pkg == nil || pkg.Pkg == nil || pkg.Pkg.Path() == "" {
			continue
		}
		packagePath := pkg.Pkg.Path()
		if prior, exists := result[packagePath]; exists {
			if prior == pkg {
				continue
			}
			return nil, fmt.Errorf(
				"surface discovery: prepared load universe produced distinct SSA packages for exact path %q; internal repomap package/type/SSA identity invariant failed; repository changes are not required",
				packagePath,
			)
		}
		result[packagePath] = pkg
	}
	return result, nil
}

func (universe *preparedLoadUniverse) projectPackages(
	root string,
	expected []PackageInput,
) ([]*ssa.Package, map[string]bool, error) {
	ssaPackages := make([]*ssa.Package, 0, len(expected))
	modulePaths := make(map[string]bool)
	expectedKeys := make([]string, 0, len(expected))
	resolvedKeys := make([]string, 0, len(expected))
	issues := make([]string, 0)
	for _, value := range expected {
		key := preparedPackageKey(value)
		expectedKeys = append(expectedKeys, key)
		facts := universe.packageFacts[value.Path]
		ssaPackage := universe.ssaPackages[value.Path]
		switch {
		case !packageSafeForSSA(facts):
			issues = append(issues, key+"=typed_facts_unavailable")
			continue
		case ssaPackage == nil || ssaPackage.Pkg == nil:
			issues = append(issues, key+"=ssa_package_unavailable")
			continue
		case facts.Types != ssaPackage.Pkg:
			issues = append(issues, key+"=types_ssa_identity_mismatch")
			continue
		}
		moduleDir, ok := repositoryPackageModuleDirectory(root, facts)
		if !ok || moduleDir != value.ModuleDir {
			actual := "outside_repository"
			if ok {
				actual = moduleDir
			}
			issues = append(issues, key+"=module_directory:"+actual)
			continue
		}
		resolvedKeys = append(resolvedKeys, key)
		ssaPackages = append(ssaPackages, ssaPackage)
		if facts.Module != nil && facts.Module.Path != "" {
			modulePaths[facts.Module.Path] = true
		}
	}
	if len(resolvedKeys) != len(expectedKeys) {
		return nil, nil, fmt.Errorf(
			"surface discovery: prepared workspace package projection is incomplete: expected=%d resolved=%d expected_keys=%s resolved_keys=%s issues=%s; internal repomap package/type/SSA identity invariant failed; repository changes are not required; retry with an updated repomap build and include this diagnostic when reporting the bug",
			len(expectedKeys), len(resolvedKeys), boundedPreparedKeys(expectedKeys),
			boundedPreparedKeys(resolvedKeys), boundedPreparedKeys(issues),
		)
	}
	sort.Slice(ssaPackages, func(i, j int) bool {
		return ssaPackagePath(ssaPackages[i]) < ssaPackagePath(ssaPackages[j])
	})
	return ssaPackages, modulePaths, nil
}

func boundedPreparedKeys(values []string) string {
	ordered := append([]string(nil), values...)
	sort.Strings(ordered)
	omitted := 0
	if len(ordered) > maxPreparedProjectionDiagnosticKeys {
		omitted = len(ordered) - maxPreparedProjectionDiagnosticKeys
		ordered = ordered[:maxPreparedProjectionDiagnosticKeys]
	}
	quoted := make([]string, len(ordered))
	for index, value := range ordered {
		quoted[index] = strconv.Quote(value)
	}
	result := "[" + strings.Join(quoted, ", ")
	if omitted > 0 {
		if len(quoted) > 0 {
			result += ", "
		}
		result += fmt.Sprintf("+%d more", omitted)
	}
	return result + "]"
}

// PrepareWorkspace loads the union of exact package scopes advertised inside
// each compatible module-resolution context and builds one SSA program per
// context. Every target is validated before the workspace is returned, so a
// later Analyze never falls back to a new load.
func PrepareWorkspace(ctx context.Context, opts Options, inputs []Input) (*PreparedWorkspace, error) {
	normalizedOpts, root, scenario, err := normalizeWorkspaceOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("surface discovery: prepared workspace requires at least one exact target")
	}

	bindings := make(map[string]preparedTargetBinding, len(inputs))
	groups := make(map[string]*preparedLoadGroup)
	for _, input := range inputs {
		owned, normalizeErr := normalizeInput(input)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		ref := owned.AnalysisTarget.TargetRef
		if prior, duplicate := bindings[ref]; duplicate {
			if !reflect.DeepEqual(prior.input, owned) {
				return nil, fmt.Errorf("surface discovery: prepared target %q has conflicting exact scopes", ref)
			}
			continue
		}
		key := preparedUniverseKey(owned.AnalysisTarget)
		bindings[ref] = preparedTargetBinding{input: cloneInput(owned), universeKey: key}
		group := groups[key]
		if group == nil {
			group = &preparedLoadGroup{key: key}
			groups[key] = group
		}
		group.inputs = append(group.inputs, owned)
	}

	workspace := &PreparedWorkspace{
		root: root, goTarget: normalizedOpts.GoTarget,
		buildTags: append([]string(nil), normalizedOpts.BuildTags...), scenario: scenario,
		bindings: bindings, universes: make(map[string]*preparedLoadUniverse, len(groups)),
	}
	orderedGroups := make([]*preparedLoadGroup, 0, len(groups))
	for _, group := range groups {
		orderedGroups = append(orderedGroups, group)
	}
	sort.Slice(orderedGroups, func(i, j int) bool { return orderedGroups[i].key < orderedGroups[j].key })
	for _, group := range orderedGroups {
		universe, loadCalls, ssaBuilds, prepareErr := prepareLoadUniverse(
			ctx, normalizedOpts, root, scenario, group,
		)
		workspace.packageLoadCalls += loadCalls
		workspace.ssaBuilds += ssaBuilds
		if prepareErr != nil {
			return nil, prepareErr
		}
		workspace.universes[group.key] = universe
	}
	return workspace, nil
}

func prepareLoadUniverse(
	ctx context.Context,
	opts Options,
	root string,
	scenario Scenario,
	group *preparedLoadGroup,
) (*preparedLoadUniverse, int, int, error) {
	if group == nil || len(group.inputs) == 0 {
		return nil, 0, 0, fmt.Errorf("surface discovery: prepared load universe is empty")
	}
	union := Input{AnalysisTarget: group.inputs[0].AnalysisTarget}
	for _, input := range group.inputs {
		union.ModuleDirs = append(union.ModuleDirs, input.ModuleDirs...)
		union.Packages = append(union.Packages, input.Packages...)
	}
	union.ModuleDirs = normalizeModuleDirs(union.ModuleDirs)
	union.Packages = normalizePackageInputs(union.Packages)
	if err := validatePreparedPackageKeys(union.Packages); err != nil {
		return nil, 0, 0, err
	}

	admitted := make(map[string]bool, len(union.Packages))
	for _, pkg := range union.Packages {
		admitted[pkg.Path] = true
	}
	loader := &analyzer{
		ctx: ctx, opts: opts, input: union, root: root,
		packageFacts:     make(map[string]*packages.Package),
		admittedPackages: admitted,
		modulePaths:      make(map[string]bool), functionIDs: make(map[*ssa.Function]string),
		scenario: scenario,
	}
	if err := loader.load(); err != nil {
		return nil, loader.packageLoadCalls, loader.ssaBuilds, err
	}
	for _, input := range group.inputs {
		loader.input = input
		if err := loader.validateAnalysisTargetPackages(loader.loadedPackages); err != nil {
			return nil, loader.packageLoadCalls, loader.ssaBuilds, err
		}
	}
	ssaPackages, err := indexPreparedSSAPackages(loader.packages)
	if err != nil {
		return nil, loader.packageLoadCalls, loader.ssaBuilds, err
	}
	return &preparedLoadUniverse{
		program: loader.program, ssaPackages: ssaPackages,
		packageFacts: loader.packageFacts, loadedPackages: loader.loadedPackages,
		allFunctions:        loader.allFunctions,
		preparationCoverage: cloneProgramCoverage(loader.result.Coverage),
	}, loader.packageLoadCalls, loader.ssaBuilds, nil
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
	if !ok || !reflect.DeepEqual(bound.input, input) {
		return Result{}, fmt.Errorf(
			"surface discovery: target %q is not bound to this prepared workspace",
			input.AnalysisTarget.TargetRef,
		)
	}
	universe := workspace.universes[bound.universeKey]
	if universe == nil {
		return Result{}, fmt.Errorf(
			"surface discovery: target %q has no prepared load universe; internal repomap invariant failed; repository changes are not required",
			input.AnalysisTarget.TargetRef,
		)
	}

	a, err := workspace.analyzerFor(ctx, normalizedOpts, input, universe)
	if err != nil {
		return Result{}, err
	}
	return analyzePreparedTarget(a)
}

func (workspace *PreparedWorkspace) analyzerFor(
	ctx context.Context,
	opts Options,
	input Input,
	universe *preparedLoadUniverse,
) (*analyzer, error) {
	if universe == nil {
		return nil, fmt.Errorf("surface discovery: prepared load universe is required")
	}
	admitted := make(map[string]bool, len(input.Packages))
	modulePaths := make(map[string]bool)
	ssaPackages, resolvedModulePaths, err := universe.projectPackages(workspace.root, input.Packages)
	if err != nil {
		return nil, err
	}
	for _, pkg := range input.Packages {
		admitted[pkg.Path] = true
	}
	for modulePath := range resolvedModulePaths {
		modulePaths[modulePath] = true
	}

	functions := make(map[*ssa.Function]bool)
	for function := range universe.allFunctions {
		if function == nil {
			continue
		}
		packagePath := functionPackagePath(function)
		if !admitted[packagePath] || !functionBelongsToFacts(function, universe.packageFacts[packagePath]) {
			continue
		}
		functions[function] = true
	}
	a := &analyzer{
		ctx: ctx, opts: opts, input: input, root: workspace.root,
		program: universe.program, packages: ssaPackages,
		packageFacts: universe.packageFacts, loadedPackages: universe.loadedPackages,
		admittedPackages: admitted, allFunctions: functions,
		modulePaths: modulePaths, functionIDs: make(map[*ssa.Function]string),
		scenario: workspace.scenario,
		result:   Result{Coverage: cloneProgramCoverage(universe.preparationCoverage)},
	}
	a.recordPackageLoadOutcomes(universe.packageClosure(input))
	for _, function := range a.materializeSelectedLibraryCallables() {
		if function != nil && functionBelongsToFacts(function, universe.packageFacts[functionPackagePath(function)]) {
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
	if a.opts.CaptureCoreObjectIndex {
		index, err := a.captureCoreObjectIndex(direct)
		if err != nil {
			return Result{}, err
		}
		a.result.CoreObjectIndex = &index
	}
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

func (universe *preparedLoadUniverse) packageClosure(input Input) map[string]*packages.Package {
	result := make(map[string]*packages.Package)
	queue := make([]*packages.Package, 0, len(input.Packages))
	for _, admitted := range input.Packages {
		if pkg := universe.loadedPackages[admitted.Path]; pkg != nil {
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
