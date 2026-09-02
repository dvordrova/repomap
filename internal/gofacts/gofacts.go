package gofacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/reporead"
	"golang.org/x/mod/modfile"
)

const (
	advisoryGoModBytes              = 1024 * 1024
	advisoryExternalImportSummaries = 50
	scaleWarningPrefix              = "large retained Go facts: "
)

// ScaleWarnings returns only warning-only size diagnostics from an accepted
// fact set. It deliberately performs no validation and can never change
// target or publication authority.
func ScaleWarnings(facts *Facts) []string {
	if facts == nil {
		return nil
	}
	result := make([]string, 0)
	for _, warning := range facts.Warnings {
		if !strings.HasPrefix(warning, scaleWarningPrefix) {
			continue
		}
		result = append(result, strings.TrimPrefix(warning, scaleWarningPrefix))
	}
	return result
}

func scaleWarning(message string) string {
	return scaleWarningPrefix + message
}

type LoadOptions struct {
	GoTarget string
	// BuildTags is the canonical run-wide Go build selection. It must match the
	// later packages/types/SSA load so the target catalog and ProgramIndex see
	// the same source set.
	BuildTags []string
	// ModuleDir narrows deterministic collection only when ordinary main has an
	// exact typed target key. Final target resolution still validates that key
	// against the catalog built from this module; the directory is not target
	// authority by itself.
	ModuleDir string
}

type Facts struct {
	Modules               []ModuleFact          `json:"modules"`
	Packages              []PackageFact         `json:"packages"`
	PackageOrigins        []PackageOrigin       `json:"package_origins"`
	PackagesCount         int                   `json:"packages_count"`
	RetainedPackagesCount int                   `json:"retained_packages_count"`
	EntrypointPackages    []Entrypoint          `json:"entrypoint_packages"`
	InternalEdges         []Edge                `json:"internal_edges"`
	ExternalImportsTop    []ExtImport           `json:"external_imports_top"`
	Dependencies          *dependencies.Catalog `json:"dependencies,omitempty"`
	Coverage              Coverage              `json:"coverage"`
	Warnings              []string              `json:"warnings,omitempty"`
}

// PackageOrigin is the exact package namespace row returned by the canonical
// build-selected `go list -deps` invocation. PackagePath deliberately keeps
// the Go tool's raw import identity; Standard is the tool-owned distinction
// between platform and ordinary package authority.
type PackageOrigin struct {
	PackagePath string `json:"package_path"`
	Standard    bool   `json:"standard"`
}

// ValidatePackageOrigins checks the sealed canonical shape consumed by the Go
// ProgramIndex adapter. The complete producer owns sorting and deduplication;
// downstream stages never repair or reinterpret this authority.
func ValidatePackageOrigins(values []PackageOrigin) error {
	for index, value := range values {
		if value.PackagePath == "" || strings.TrimSpace(value.PackagePath) != value.PackagePath {
			return fmt.Errorf("Go package origin %d has invalid package path", index)
		}
		if index > 0 && values[index-1].PackagePath >= value.PackagePath {
			return fmt.Errorf("Go package origins are not canonical at %q", value.PackagePath)
		}
	}
	return nil
}

type CoverageState string

const (
	CoverageComplete    CoverageState = "complete"
	CoveragePartial     CoverageState = "partial"
	CoverageUnavailable CoverageState = "unavailable"
)

// Coverage describes exact local Go-fact collection, independently from any
// later model-visible projection. Discovered counts describe successful local
// enumeration; retained counts describe the bounded persisted rows.
type Coverage struct {
	State              CoverageState `json:"state"`
	ModulesDiscovered  int           `json:"modules_discovered"`
	ModulesAvailable   int           `json:"modules_available"`
	ModulesUnavailable int           `json:"modules_unavailable"`
	PackagesDiscovered int           `json:"packages_discovered"`
	PackagesRetained   int           `json:"packages_retained"`
	EdgesDiscovered    int           `json:"edges_discovered"`
	EdgesRetained      int           `json:"edges_retained"`
}

type ModuleCoverage struct {
	State              CoverageState `json:"state"`
	PackagesDiscovered int           `json:"packages_discovered"`
	PackagesRetained   int           `json:"packages_retained"`
}

type ModuleFact struct {
	ID                    string         `json:"id"`
	ModulePath            string         `json:"module_path"`
	ModuleDir             string         `json:"module_dir"`
	GoMod                 string         `json:"go_mod,omitempty"`
	Main                  bool           `json:"main"`
	DisplayName           string         `json:"display_name"`
	Replacement           *ModuleSource  `json:"replacement,omitempty"`
	Replacements          []ModuleSource `json:"replacements,omitempty"`
	PackagesCount         int            `json:"packages_count"`
	RetainedPackagesCount int            `json:"retained_packages_count"`
	EntrypointPackages    []Entrypoint   `json:"entrypoint_packages"`
	Coverage              ModuleCoverage `json:"coverage"`
	Warnings              []string       `json:"warnings,omitempty"`
}

type ModuleSource struct {
	Path  string `json:"path"`
	Dir   string `json:"dir,omitempty"`
	GoMod string `json:"go_mod,omitempty"`
	Local bool   `json:"local"`
}

const PackageLoadCompletenessVersion = 1

type PackageLoadState string

const (
	PackageLoadComplete   PackageLoadState = "complete"
	PackageLoadIncomplete PackageLoadState = "incomplete"
)

// PackageLoadCompleteness is the typed, package-local authority produced by
// the exact go list invocation. Its version is persisted because absence of
// this authority must not be reinterpreted as a complete build-selected
// package when deriving aggregate module products.
type PackageLoadCompleteness struct {
	Version int              `json:"version"`
	State   PackageLoadState `json:"state"`
}

func (value *PackageLoadCompleteness) Complete() bool {
	return value != nil && value.Version == PackageLoadCompletenessVersion && value.State == PackageLoadComplete
}

type PackageFact struct {
	CanonicalPath       string                   `json:"canonical_package_path"`
	Name                string                   `json:"name"`
	ModuleID            string                   `json:"owning_module_id"`
	ModulePath          string                   `json:"module_path"`
	PackageDir          string                   `json:"package_directory"`
	ModuleRelativeDir   string                   `json:"module_relative_path"`
	DisplayPath         string                   `json:"display_path"`
	Locality            string                   `json:"locality"`
	Files               []string                 `json:"files,omitempty"`
	Declarations        []PackageDeclaration     `json:"declarations,omitempty"`
	DeclarationsScanned bool                     `json:"declarations_scanned,omitempty"`
	LoadCompleteness    *PackageLoadCompleteness `json:"load_completeness,omitempty"`
}

type Entrypoint struct {
	ModulePath        string             `json:"module_path"`
	ImportPath        string             `json:"import_path"`
	Dir               string             `json:"dir"`
	PackageDir        string             `json:"package_dir"`
	ModuleRelativeDir string             `json:"module_relative_dir"`
	ModuleDir         string             `json:"module_dir"`
	Kind              string             `json:"kind"`
	GoFiles           []string           `json:"go_files"`
	Anchors           []EntrypointAnchor `json:"anchors,omitempty"`
}

type EntrypointAnchorKind string

const (
	EntrypointAnchorVersion                      = 1
	EntrypointAnchorGoMain  EntrypointAnchorKind = "go_main_function"
	EntrypointKindGoMain                         = "go_main"
)

// EntrypointAnchor is a deterministic source declaration selected under the
// active Go build configuration. Path is repository-relative and Line points
// at the declaration name.
type EntrypointAnchor struct {
	Version int                  `json:"version"`
	Kind    EntrypointAnchorKind `json:"kind"`
	Path    string               `json:"path"`
	Line    int                  `json:"line"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ExtImport struct {
	ImportPath  string `json:"import_path"`
	UsedByCount int    `json:"used_by_count"`
}

type goListPackage struct {
	ImportPath      string
	Dir             string
	Name            string
	DepOnly         bool
	Standard        bool
	Incomplete      bool
	GoFiles         []string
	CompiledGoFiles []string
	CgoFiles        []string
	CFiles          []string
	CXXFiles        []string
	MFiles          []string
	HFiles          []string
	FFiles          []string
	SFiles          []string
	SwigFiles       []string
	SwigCXXFiles    []string
	SysoFiles       []string
	Imports         []string
	Module          *goListModule
	Error           *goListError
	DepsErrors      []goListError
}

type goListModule struct {
	Path    string
	Version string
	Dir     string
	GoMod   string
	Main    bool
	Replace *goListModule
}

type goListError struct {
	Err string
}

type dependencyPackageLoad struct {
	packages []goListPackage
}

func DiscoverGoModules(fileList []string, repoPath string) []string {
	seen := make(map[string]struct{})

	for _, f := range fileList {
		if filepath.Base(f) == "go.mod" {
			dir := filepath.Dir(f)
			if dir == "." {
				seen["."] = struct{}{}
			} else {
				seen[dir] = struct{}{}
			}
		}
	}

	if _, ok := seen["."]; !ok {
		if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
			seen["."] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for d := range seen {
		result = append(result, d)
	}
	sort.Strings(result)
	return result
}

func LoadWithOptions(
	ctx context.Context,
	repoPath string,
	fileList []string,
	opts LoadOptions,
) (*Facts, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.GoTarget) == "" {
		return nil, fmt.Errorf("load Go facts: resolved Go target is required")
	}
	target, err := gotarget.Parse(opts.GoTarget)
	if err != nil {
		return nil, fmt.Errorf("load Go facts: %w", err)
	}
	buildTags, err := gotarget.CanonicalBuildTags(opts.BuildTags)
	if err != nil {
		return nil, fmt.Errorf("load Go facts: %w", err)
	}

	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("load Go facts: resolve repository path: %w", err)
	}
	resolvedRepoPath, err := filepath.EvalSymlinks(absRepoPath)
	if err != nil {
		return nil, fmt.Errorf("load Go facts: resolve repository symlinks: %w", err)
	}

	moduleDirs := DiscoverGoModules(fileList, resolvedRepoPath)
	moduleDirs, err = selectModuleDirs(moduleDirs, opts.ModuleDir)
	if err != nil {
		return nil, fmt.Errorf("load Go facts: %w", err)
	}
	if len(moduleDirs) == 0 {
		emptyDependencies := dependencies.Empty()
		return &Facts{Dependencies: &emptyDependencies, Coverage: Coverage{State: CoverageComplete}}, nil
	}
	repoReader, err := reporead.New(resolvedRepoPath)
	if err != nil {
		return nil, fmt.Errorf("prepare bounded go.mod reads: %w", err)
	}
	defer repoReader.Close()

	var allPkgs []goListPackage
	var packageFacts []PackageFact
	var allEntrypoints []Entrypoint
	var topWarnings []string
	modules := make([]ModuleFact, 0, len(moduleDirs))
	availableModules := 0
	dependencyLoads := make([]dependencyPackageLoad, 0, len(moduleDirs))

	for _, modRelDir := range moduleDirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		goModBytes, err := verifyModuleGoMod(repoReader, modRelDir)
		if err != nil {
			return nil, fmt.Errorf("load Go facts for module %s: validate go.mod: %w", modRelDir, err)
		}
		if goModBytes > advisoryGoModBytes {
			topWarnings = append(topWarnings, scaleWarning(fmt.Sprintf(
				"module %s: retained the complete %d-byte go.mod; usual size is %d bytes",
				modRelDir, goModBytes, advisoryGoModBytes,
			)))
		}

		absDir := resolvedRepoPath
		if modRelDir != "." {
			absDir = filepath.Join(resolvedRepoPath, modRelDir)
		}

		moduleInfo, err := runGoModule(ctx, absDir, target)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, fmt.Errorf("load Go facts for module %s: %w", modRelDir, err)
		}
		modulePath := moduleInfo.Path
		moduleID := moduleFactID(modulePath, modRelDir)
		listedPkgs, modWarnings, err := runGoList(ctx, absDir, target, buildTags)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, fmt.Errorf("load Go facts for module %s: %w", modRelDir, err)
		}
		if len(modWarnings) > 0 {
			return nil, fmt.Errorf(
				"load Go facts for module %s: incomplete go list authority: %s",
				modRelDir, summarizeCollectionFailures(modWarnings),
			)
		}
		pkgs := rootGoListPackages(listedPkgs)
		availableModules++
		allPkgs = append(allPkgs, pkgs...)
		dependencyLoads = append(dependencyLoads, dependencyPackageLoad{packages: listedPkgs})
		for _, pkg := range pkgs {
			_, moduleRelativeDir, packageDir, normalizeErr := normalizePackagePaths(resolvedRepoPath, absDir, pkg.Dir)
			if normalizeErr != nil || pkg.ImportPath == "" {
				if normalizeErr != nil {
					return nil, fmt.Errorf(
						"load Go facts for module %s: package %q has unavailable repository ownership: %w",
						modRelDir, pkg.ImportPath, normalizeErr,
					)
				}
				return nil, fmt.Errorf(
					"load Go facts for module %s: package has no import path",
					modRelDir,
				)
			}
			displayPath := filepath.ToSlash(moduleRelativeDir)
			if displayPath == "." || displayPath == "" {
				displayPath = pkg.Name
			}
			declarations, declarationWarnings, declarationScaleWarnings := extractPackageDeclarations(repoReader, packageDir, pkg)
			if len(declarationWarnings) > 0 {
				return nil, fmt.Errorf(
					"load Go facts for module %s: incomplete declaration authority: %s",
					modRelDir, summarizeCollectionFailures(declarationWarnings),
				)
			}
			topWarnings = append(topWarnings, declarationScaleWarnings...)
			packageFacts = append(packageFacts, PackageFact{
				CanonicalPath: pkg.ImportPath, Name: pkg.Name, ModuleID: moduleID, ModulePath: modulePath,
				PackageDir: filepath.ToSlash(packageDir), ModuleRelativeDir: filepath.ToSlash(moduleRelativeDir),
				DisplayPath: displayPath, Locality: "local", Files: packageInputFiles(packageDir, pkg),
				Declarations: declarations, DeclarationsScanned: len(declarationWarnings) == 0,
				LoadCompleteness: packageLoadCompleteness(pkg),
			})
		}

		entrypointCandidates := buildEntrypointCandidates(pkgs, resolvedRepoPath, absDir, modRelDir, modulePath)
		modEntrypoints, entrypointWarnings, entrypointScaleWarnings := resolveMainEntrypoints(repoReader, entrypointCandidates)
		if len(entrypointWarnings) > 0 {
			return nil, fmt.Errorf(
				"load Go facts for module %s: incomplete entrypoint authority: %s",
				modRelDir, summarizeCollectionFailures(entrypointWarnings),
			)
		}
		topWarnings = append(topWarnings, entrypointScaleWarnings...)
		allEntrypoints = append(allEntrypoints, modEntrypoints...)
		replacements, replacementErr := readModuleReplacements(repoReader, resolvedRepoPath, modRelDir)
		if replacementErr != nil {
			return nil, fmt.Errorf(
				"load Go facts for module %s: read replacements: %w",
				modRelDir, replacementErr,
			)
		}
		modules = append(modules, ModuleFact{
			ID:                 moduleID,
			ModulePath:         modulePath,
			ModuleDir:          modRelDir,
			GoMod:              repositoryRelativeMetadataPath(resolvedRepoPath, moduleInfo.GoMod),
			Main:               moduleInfo.Main,
			DisplayName:        moduleDisplayName(modulePath, modRelDir),
			Replacement:        moduleSource(resolvedRepoPath, moduleInfo.Replace),
			Replacements:       replacements,
			PackagesCount:      len(pkgs),
			EntrypointPackages: modEntrypoints,
			Coverage: ModuleCoverage{
				State:              coverageStateForWarnings(modWarnings),
				PackagesDiscovered: len(pkgs),
			},
			Warnings: modWarnings,
		})

	}

	dependencyCatalog, dependencyWarnings, err := buildDependencyCatalog(resolvedRepoPath, dependencyLoads)
	if err != nil {
		return nil, fmt.Errorf("build Go dependency catalog: %w", err)
	}
	topWarnings = append(topWarnings, dependencyWarnings...)
	packageOrigins, err := buildPackageOrigins(dependencyLoads)
	if err != nil {
		return nil, fmt.Errorf("build Go package-origin authority: %w", err)
	}

	allEdges := buildInternalEdges(dependencyCatalog)
	discoveredEdges := len(allEdges)

	extImports := buildExternalImports(dependencyCatalog)
	if len(extImports) > advisoryExternalImportSummaries {
		topWarnings = append(topWarnings, scaleWarning(fmt.Sprintf(
			"retained all %d external import summaries; usual size is %d",
			len(extImports), advisoryExternalImportSummaries,
		)))
	}
	sort.Slice(packageFacts, func(i, j int) bool {
		return packageFactLess(packageFacts[i], packageFacts[j])
	})
	edges := allEdges
	retainedByModule := make(map[string]int, len(modules))
	for _, pkg := range packageFacts {
		retainedByModule[pkg.ModuleID]++
	}
	for index := range modules {
		module := &modules[index]
		if module.Coverage.State == CoverageUnavailable {
			continue
		}
		module.RetainedPackagesCount = retainedByModule[module.ID]
		module.Coverage.PackagesRetained = module.RetainedPackagesCount
		if module.RetainedPackagesCount < module.PackagesCount {
			module.Coverage.State = CoveragePartial
		}
	}
	coverage := Coverage{
		State:              CoverageComplete,
		ModulesDiscovered:  len(moduleDirs),
		ModulesAvailable:   availableModules,
		ModulesUnavailable: len(moduleDirs) - availableModules,
		PackagesDiscovered: len(allPkgs),
		PackagesRetained:   len(packageFacts),
		EdgesDiscovered:    discoveredEdges,
		EdgesRetained:      len(edges),
	}
	if availableModules == 0 {
		coverage.State = CoverageUnavailable
	} else if coverage.ModulesUnavailable > 0 || hasPartialModuleCoverage(modules) ||
		dependencyCatalog.Coverage.State == dependencies.CoveragePartial {
		coverage.State = CoveragePartial
	}

	return &Facts{
		Modules:               modules,
		Packages:              packageFacts,
		PackageOrigins:        packageOrigins,
		PackagesCount:         len(allPkgs),
		RetainedPackagesCount: len(packageFacts),
		EntrypointPackages:    allEntrypoints,
		InternalEdges:         edges,
		ExternalImportsTop:    extImports,
		Dependencies:          &dependencyCatalog,
		Coverage:              coverage,
		Warnings:              canonicalWarnings(topWarnings),
	}, nil
}

func buildPackageOrigins(loads []dependencyPackageLoad) ([]PackageOrigin, error) {
	byPath := make(map[string]bool)
	for _, load := range loads {
		for _, pkg := range load.packages {
			if pkg.ImportPath == "" || strings.TrimSpace(pkg.ImportPath) != pkg.ImportPath {
				return nil, fmt.Errorf("go-list package has invalid import path")
			}
			if previous, exists := byPath[pkg.ImportPath]; exists && previous != pkg.Standard {
				return nil, fmt.Errorf("conflicting standard authority for package %q", pkg.ImportPath)
			}
			byPath[pkg.ImportPath] = pkg.Standard
		}
	}
	result := make([]PackageOrigin, 0, len(byPath))
	for packagePath, standard := range byPath {
		result = append(result, PackageOrigin{PackagePath: packagePath, Standard: standard})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PackagePath < result[j].PackagePath
	})
	if err := ValidatePackageOrigins(result); err != nil {
		return nil, err
	}
	return result, nil
}

func selectModuleDirs(discovered []string, explicit string) ([]string, error) {
	if explicit == "" {
		return discovered, nil
	}
	if strings.TrimSpace(explicit) != explicit || filepath.IsAbs(explicit) {
		return nil, fmt.Errorf("explicit target module directory must be canonical and repository-relative")
	}
	canonical := filepath.ToSlash(filepath.Clean(explicit))
	if canonical != explicit || strings.HasPrefix(canonical, "../") {
		return nil, fmt.Errorf("explicit target module directory must be canonical and repository-relative")
	}
	for _, moduleDir := range discovered {
		if moduleDir == canonical {
			return []string{canonical}, nil
		}
	}
	return nil, fmt.Errorf("explicit target module directory %q is not a discovered Go module", explicit)
}

func packageInputFiles(packageDir string, pkg goListPackage) []string {
	fileNames := make([]string, 0, len(pkg.GoFiles)+len(pkg.CgoFiles)+len(pkg.CFiles)+len(pkg.HFiles)+len(pkg.SFiles))
	fileNames = append(fileNames, pkg.GoFiles...)
	fileNames = append(fileNames, pkg.CgoFiles...)
	fileNames = append(fileNames, pkg.CFiles...)
	fileNames = append(fileNames, pkg.CXXFiles...)
	fileNames = append(fileNames, pkg.MFiles...)
	fileNames = append(fileNames, pkg.HFiles...)
	fileNames = append(fileNames, pkg.FFiles...)
	fileNames = append(fileNames, pkg.SFiles...)
	fileNames = append(fileNames, pkg.SwigFiles...)
	fileNames = append(fileNames, pkg.SwigCXXFiles...)
	fileNames = append(fileNames, pkg.SysoFiles...)
	result := make([]string, 0, len(fileNames))
	for _, name := range fileNames {
		name = filepath.ToSlash(filepath.Clean(name))
		if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/") {
			continue
		}
		if packageDir == "." || packageDir == "" {
			result = append(result, name)
		} else {
			result = append(result, filepath.ToSlash(filepath.Join(packageDir, name)))
		}
	}
	sort.Strings(result)
	return compactStrings(result)
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func summarizeCollectionFailures(values []string) string {
	if len(values) == 0 {
		return "unknown collection failure"
	}
	first := strings.TrimSpace(values[0])
	if first == "" {
		first = "unknown collection failure"
	}
	if len(values) == 1 {
		return first
	}
	return fmt.Sprintf("%s (and %d more)", first, len(values)-1)
}

func readModuleReplacements(reader *reporead.Reader, repoRoot, moduleDir string) ([]ModuleSource, error) {
	goModPath := "go.mod"
	if moduleDir != "." {
		goModPath = filepath.ToSlash(filepath.Join(moduleDir, goModPath))
	}
	content, err := reader.ReadFileAll(goModPath)
	if err != nil {
		return nil, err
	}
	parsed, err := modfile.Parse(goModPath, content.Bytes, nil)
	if err != nil {
		return nil, err
	}
	result := make([]ModuleSource, 0, len(parsed.Replace))
	moduleRoot := repoRoot
	if moduleDir != "." {
		moduleRoot = filepath.Join(repoRoot, filepath.FromSlash(moduleDir))
	}
	for _, replacement := range parsed.Replace {
		source := ModuleSource{Path: replacement.Old.Path}
		if replacement.New.Version == "" && replacement.New.Path != "" {
			target := replacement.New.Path
			if !filepath.IsAbs(target) {
				target = filepath.Join(moduleRoot, filepath.FromSlash(target))
			}
			if relative := repositoryRelativeMetadataPath(repoRoot, target); relative != "" {
				source.Dir = relative
				source.GoMod = filepath.ToSlash(filepath.Join(relative, "go.mod"))
				source.Local = true
			}
		}
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func runGoModule(ctx context.Context, repoDir string, target gotarget.Target) (goListModule, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json")
	cmd.Dir = repoDir
	cmd.Env = append(target.ApplyEnv(os.Environ()), "GOWORK=off")
	output, err := cmd.Output()
	if err != nil {
		return goListModule{}, fmt.Errorf("go list -m: %w", err)
	}
	var module goListModule
	if err := json.Unmarshal(output, &module); err != nil {
		return goListModule{}, fmt.Errorf("decode go list module: %w", err)
	}
	if module.Path == "" {
		return goListModule{}, fmt.Errorf("go list module has no declared path")
	}
	return module, nil
}

func moduleFactID(modulePath, moduleDir string) string {
	digest := sha256.Sum256([]byte(modulePath + "\x00" + filepath.ToSlash(moduleDir)))
	return fmt.Sprintf("module-%x", digest[:12])
}

func moduleDisplayName(_ string, moduleDir string) string {
	if moduleDir != "" && moduleDir != "." {
		return filepath.ToSlash(moduleDir)
	}
	return "."
}

func repositoryRelativeMetadataPath(repoRoot, value string) string {
	if value == "" {
		return ""
	}
	relative, err := filepath.Rel(repoRoot, value)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(relative)
}

func moduleSource(repoRoot string, module *goListModule) *ModuleSource {
	if module == nil {
		return nil
	}
	dir := repositoryRelativeMetadataPath(repoRoot, module.Dir)
	goMod := repositoryRelativeMetadataPath(repoRoot, module.GoMod)
	return &ModuleSource{Path: module.Path, Dir: dir, GoMod: goMod, Local: dir != ""}
}

func verifyModuleGoMod(reader *reporead.Reader, moduleDir string) (int, error) {
	goModPath := "go.mod"
	if moduleDir != "." {
		goModPath = filepath.Join(moduleDir, goModPath)
	}

	content, err := reader.ReadFileAll(goModPath)
	if err != nil {
		return 0, fmt.Errorf("verify %q: %w", goModPath, err)
	}
	return len(content.Bytes), nil
}

func runGoList(
	ctx context.Context,
	repoDir string,
	target gotarget.Target,
	buildTags []string,
) ([]goListPackage, []string, error) {
	// Always pass an explicit value, including empty, so ambient
	// GOFLAGS=-tags=... cannot silently change the run's recorded source set.
	args := []string{
		"list", "-deps", "-e", "-json", "-tags=" + strings.Join(buildTags, ","),
	}
	args = append(args, "./...")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoDir
	cmd.Env = append(target.ApplyEnv(os.Environ()), "GOWORK=off")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("go list failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	return parseGoListOutput(&stdout)
}

func parseGoListOutput(r io.Reader) ([]goListPackage, []string, error) {
	dec := json.NewDecoder(r)
	var pkgs []goListPackage
	var warnings []string

	for {
		var pkg goListPackage
		if err := dec.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return nil, nil, fmt.Errorf("decode package %d: %w", len(pkgs), err)
		}
		// Dependency-only objects exist to provide exact dependency metadata.
		// Their diagnostics are already reflected by the importing root
		// package's Incomplete/DepsErrors authority, so repeating them here
		// would change the established workspace-package warning contract.
		if pkg.DepOnly {
			pkgs = append(pkgs, pkg)
			continue
		}
		if pkg.Incomplete {
			warnings = append(warnings, fmt.Sprintf("package %s: go list facts are incomplete", pkg.ImportPath))
		}
		if pkg.Error != nil {
			warnings = append(warnings, fmt.Sprintf("package %s: %s", pkg.ImportPath, pkg.Error.Err))
		}
		for _, dependencyError := range pkg.DepsErrors {
			warnings = append(warnings, fmt.Sprintf(
				"package %s dependency: %s",
				pkg.ImportPath,
				dependencyError.Err,
			))
		}
		pkgs = append(pkgs, pkg)
	}

	return pkgs, warnings, nil
}

func rootGoListPackages(values []goListPackage) []goListPackage {
	result := make([]goListPackage, 0, len(values))
	for _, value := range values {
		// `go list -deps -json ./...` emits a root row for a directory that has
		// only external *_test.go files, even though Tests=false has no ordinary
		// package syntax for it. Keep that row in the raw dependency load, but do
		// not promote it into repository PackageFacts or target identity.
		if value.DepOnly || len(value.GoFiles)+len(value.CgoFiles) == 0 {
			continue
		}
		result = append(result, value)
	}
	return result
}

func packageLoadCompleteness(pkg goListPackage) *PackageLoadCompleteness {
	state := PackageLoadComplete
	if pkg.Incomplete || pkg.Error != nil || len(pkg.DepsErrors) > 0 {
		state = PackageLoadIncomplete
	}
	return &PackageLoadCompleteness{Version: PackageLoadCompletenessVersion, State: state}
}

func normalizePackagePaths(repoRoot, moduleRoot, pkgDir string) (moduleDir string, moduleRelativeDir string, packageDir string, err error) {
	repoRoot = filepath.Clean(repoRoot)
	moduleRoot = filepath.Clean(moduleRoot)
	pkgDir = filepath.Clean(pkgDir)

	packageDir, err = filepath.Rel(repoRoot, pkgDir)
	if err != nil {
		resolvedRepoRoot, repoErr := filepath.EvalSymlinks(repoRoot)
		if repoErr != nil {
			return "", "", "", fmt.Errorf("resolve repository root %q after relative-path failure: %w", repoRoot, repoErr)
		}
		resolvedPackageDir, packageErr := filepath.EvalSymlinks(pkgDir)
		if packageErr != nil {
			return "", "", "", fmt.Errorf("resolve package directory %q after relative-path failure: %w", pkgDir, packageErr)
		}
		repoRoot, pkgDir = resolvedRepoRoot, resolvedPackageDir
		packageDir, err = filepath.Rel(repoRoot, pkgDir)
		if err != nil {
			return "", "", "", fmt.Errorf("package dir %q not under repo root %q: %w", pkgDir, repoRoot, err)
		}
	}
	if packageDir == "" {
		packageDir = "."
	}
	if packageDir == ".." || strings.HasPrefix(packageDir, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("package dir %q is outside repo root %q", pkgDir, repoRoot)
	}

	moduleDir, err = filepath.Rel(repoRoot, moduleRoot)
	if err != nil {
		resolvedRepoRoot, repoErr := filepath.EvalSymlinks(repoRoot)
		if repoErr != nil {
			return "", "", "", fmt.Errorf("resolve repository root %q after relative-path failure: %w", repoRoot, repoErr)
		}
		resolvedModuleRoot, moduleErr := filepath.EvalSymlinks(moduleRoot)
		if moduleErr != nil {
			return "", "", "", fmt.Errorf("resolve module root %q after relative-path failure: %w", moduleRoot, moduleErr)
		}
		repoRoot, moduleRoot = resolvedRepoRoot, resolvedModuleRoot
		moduleDir, err = filepath.Rel(repoRoot, moduleRoot)
		if err != nil {
			return "", "", "", fmt.Errorf("module root %q not under repo root %q: %w", moduleRoot, repoRoot, err)
		}
	}
	if moduleDir == "" {
		moduleDir = "."
	}

	moduleRelativeDir, err = filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		resolvedModuleRoot, moduleErr := filepath.EvalSymlinks(moduleRoot)
		if moduleErr != nil {
			return "", "", "", fmt.Errorf("resolve module root %q after relative-path failure: %w", moduleRoot, moduleErr)
		}
		resolvedPackageDir, packageErr := filepath.EvalSymlinks(pkgDir)
		if packageErr != nil {
			return "", "", "", fmt.Errorf("resolve package directory %q after relative-path failure: %w", pkgDir, packageErr)
		}
		moduleRoot, pkgDir = resolvedModuleRoot, resolvedPackageDir
		moduleRelativeDir, err = filepath.Rel(moduleRoot, pkgDir)
		if err != nil {
			return "", "", "", fmt.Errorf("package dir %q not under module root %q: %w", pkgDir, moduleRoot, err)
		}
	}
	if moduleRelativeDir == "" {
		moduleRelativeDir = "."
	}
	if moduleRelativeDir == ".." || strings.HasPrefix(moduleRelativeDir, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("package dir %q is outside module root %q", pkgDir, moduleRoot)
	}

	return moduleDir, moduleRelativeDir, packageDir, nil
}

func coverageStateForWarnings(warnings []string) CoverageState {
	if len(warnings) > 0 {
		return CoveragePartial
	}
	return CoverageComplete
}

func hasPartialModuleCoverage(modules []ModuleFact) bool {
	for _, module := range modules {
		if module.Coverage.State == CoveragePartial {
			return true
		}
	}
	return false
}

func packageFactLess(left, right PackageFact) bool {
	if left.PackageDir != right.PackageDir {
		return left.PackageDir < right.PackageDir
	}
	if left.ModuleID != right.ModuleID {
		return left.ModuleID < right.ModuleID
	}
	return left.CanonicalPath < right.CanonicalPath
}

func buildEntrypointCandidates(pkgs []goListPackage, repoRoot string, moduleRoot string, modRelDir string, modulePath string) []Entrypoint {
	eps := make([]Entrypoint, 0)
	for _, p := range pkgs {
		if p.Name != "main" || p.Error != nil {
			continue
		}

		md, mrd, pd, err := normalizePackagePaths(repoRoot, moduleRoot, p.Dir)
		if err != nil {
			md = modRelDir
			pd = filepath.Base(p.Dir)
			mrd = pd
		}

		eps = append(eps, Entrypoint{
			ModulePath:        modulePath,
			ImportPath:        p.ImportPath,
			Dir:               pd,
			PackageDir:        pd,
			ModuleRelativeDir: mrd,
			ModuleDir:         md,
			Kind:              EntrypointKindGoMain,
			GoFiles:           buildSelectedOriginalGoFiles(p),
		})
	}
	return eps
}

// buildSelectedOriginalGoFiles owns the exact repository sources that may
// contain a process entrypoint. CompiledGoFiles is deliberately excluded: for
// cgo packages it includes generated objdir syntax that has no repository
// source identity.
func buildSelectedOriginalGoFiles(pkg goListPackage) []string {
	files := append([]string(nil), pkg.GoFiles...)
	files = append(files, pkg.CgoFiles...)
	sort.Strings(files)
	return compactStrings(files)
}

func buildInternalEdges(catalog dependencies.Catalog) []Edge {
	importers := make(map[string]string, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		importers[importer.Ref] = importer.PackagePath
	}
	edges := make([]Edge, 0)
	seen := make(map[Edge]struct{})
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind != dependencies.KindWorkspace {
			continue
		}
		for _, importerRef := range dependency.ImporterRefs {
			from := importers[importerRef]
			edge := Edge{From: from, To: dependency.PackagePath}
			if edge.From == "" || edge.To == "" {
				continue
			}
			if _, duplicate := seen[edge]; duplicate {
				continue
			}
			seen[edge] = struct{}{}
			edges = append(edges, edge)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	return edges
}

func buildExternalImports(catalog dependencies.Catalog) []ExtImport {
	uses := make(map[string]map[string]struct{})
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind == dependencies.KindWorkspace {
			continue
		}
		refs := uses[dependency.PackagePath]
		if refs == nil {
			refs = make(map[string]struct{})
			uses[dependency.PackagePath] = refs
		}
		for _, importerRef := range dependency.ImporterRefs {
			if importerRef != "" {
				refs[importerRef] = struct{}{}
			}
		}
	}

	extImports := make([]ExtImport, 0, len(uses))
	for packagePath, refs := range uses {
		if packagePath == "" || len(refs) == 0 {
			continue
		}
		extImports = append(extImports, ExtImport{ImportPath: packagePath, UsedByCount: len(refs)})
	}
	sort.Slice(extImports, func(i, j int) bool {
		if extImports[i].UsedByCount == extImports[j].UsedByCount {
			return extImports[i].ImportPath < extImports[j].ImportPath
		}
		return extImports[i].UsedByCount > extImports[j].UsedByCount
	})
	return extImports
}
