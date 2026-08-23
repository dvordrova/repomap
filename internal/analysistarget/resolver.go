package analysistarget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
)

// Candidates derives every exact build-selected executable plus at most one
// public module-library target per complete module. A module library is not a
// package alias: it seals the complete non-main module scope and the exact
// externally importable package roots that expose at least one exported name.
func Candidates(facts gofacts.Facts) ([]Candidate, error) {
	modules := make(map[string]gofacts.ModuleFact, len(facts.Modules))
	for _, module := range facts.Modules {
		if strings.TrimSpace(module.ID) == "" || strings.TrimSpace(module.ModulePath) == "" {
			return nil, fmt.Errorf("analysis target: invalid module identity")
		}
		if _, exists := modules[module.ID]; exists {
			return nil, fmt.Errorf("analysis target: duplicate module identity %q", module.ID)
		}
		modules[module.ID] = module
	}

	packages := make(map[string]gofacts.PackageFact, len(facts.Packages))
	packagesByModule := make(map[string][]gofacts.PackageFact, len(modules))
	for _, pkg := range facts.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		if _, ok := modules[pkg.ModuleID]; !ok {
			return nil, fmt.Errorf("analysis target: package %q has unknown module %q", pkg.CanonicalPath, pkg.ModuleID)
		}
		if pkg.ModulePath != modules[pkg.ModuleID].ModulePath {
			return nil, fmt.Errorf("analysis target: package %q has inconsistent module path", pkg.CanonicalPath)
		}
		key := packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)
		if _, exists := packages[key]; exists {
			return nil, fmt.Errorf("analysis target: duplicate package identity %q", pkg.CanonicalPath)
		}
		packages[key] = pkg
		packagesByModule[pkg.ModuleID] = append(packagesByModule[pkg.ModuleID], pkg)
	}

	type executableBuild struct {
		pkg   gofacts.PackageFact
		kind  string
		roots []Root
	}
	executables := make(map[string]executableBuild)
	for _, entrypoint := range facts.EntrypointPackages {
		pkg, err := findEntrypointPackage(packages, modules, entrypoint)
		if err != nil {
			return nil, err
		}
		key := packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)
		build := executables[key]
		build.pkg = pkg
		if build.kind == "" || entrypoint.Kind < build.kind {
			build.kind = entrypoint.Kind
		}
		for _, anchor := range entrypoint.Anchors {
			if anchor.Version != gofacts.EntrypointAnchorVersion || anchor.Kind != gofacts.EntrypointAnchorGoMain {
				continue
			}
			root, err := canonicalRoot(anchor.Path, anchor.Line)
			if err != nil {
				return nil, fmt.Errorf("analysis target: entrypoint %q: %w", entrypoint.ImportPath, err)
			}
			build.roots = append(build.roots, root)
		}
		executables[key] = build
	}

	candidates := make([]Candidate, 0, len(executables)+len(modules))
	for key, pkg := range packages {
		module := modules[pkg.ModuleID]
		if executable, ok := executables[key]; ok {
			roots := canonicalRoots(executable.roots)
			if len(roots) == 0 {
				return nil, fmt.Errorf("analysis target: executable package %q has no exact main declaration", pkg.CanonicalPath)
			}
			target, err := newTarget(module, pkg, KindExecutablePackage, RootBoundaryExactPackageMains, roots)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, Candidate{
				Key: candidateKey(module.ModulePath, module.ModuleDir, pkg.CanonicalPath), Target: target,
				MainModule: rootAnalysisModule(module), EntrypointKind: executable.kind,
			})
		}
	}

	for _, module := range modules {
		target, ok, err := newModuleLibraryTarget(module, packagesByModule[module.ID])
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		candidates = append(candidates, Candidate{
			Key: moduleCandidateKey(module.ModulePath, module.ModuleDir), Target: target,
			MainModule: rootAnalysisModule(module),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Target.Kind != candidates[j].Target.Kind {
			return candidates[i].Target.Kind < candidates[j].Target.Kind
		}
		return candidates[i].Key < candidates[j].Key
	})
	return candidates, nil
}

// gofacts may inspect multiple nested modules independently; each `go list`
// invocation can consequently mark its own module as Main. In the target
// catalog, "main" means the module at the analysis root. Nested fixture and
// example modules remain valid explicit candidates.
func rootAnalysisModule(module gofacts.ModuleFact) bool {
	return module.Main && canonicalDirForMatch(module.ModuleDir) == "."
}

func newTarget(module gofacts.ModuleFact, pkg gofacts.PackageFact, kind Kind, boundary RootBoundary, roots []Root) (Target, error) {
	if strings.TrimSpace(pkg.ModuleID) == "" || strings.TrimSpace(pkg.ModulePath) == "" ||
		strings.TrimSpace(pkg.CanonicalPath) == "" {
		return Target{}, fmt.Errorf("analysis target: invalid package identity")
	}
	dir, err := canonicalPackageDir(pkg.PackageDir)
	if err != nil {
		return Target{}, fmt.Errorf("analysis target: package %q: %w", pkg.CanonicalPath, err)
	}
	moduleDir, err := canonicalPackageDir(module.ModuleDir)
	if err != nil {
		return Target{}, fmt.Errorf("analysis target: module %q: %w", module.ModulePath, err)
	}
	target := Target{
		Version: Version, Kind: kind, ModuleID: pkg.ModuleID, ModulePath: pkg.ModulePath,
		ModuleDir: moduleDir, PackagePath: pkg.CanonicalPath, PackageDir: dir, RootBoundary: boundary,
		Roots: append([]Root{}, roots...),
	}
	target.Ref, err = targetRef(target)
	if err != nil {
		return Target{}, err
	}
	if err := target.Validate(); err != nil {
		return Target{}, err
	}
	return target, nil
}

func newModuleLibraryTarget(
	module gofacts.ModuleFact,
	packages []gofacts.PackageFact,
) (Target, bool, error) {
	if !modulePackageInventoryComplete(module, packages) {
		return Target{}, false, nil
	}

	modulePackages := make([]TargetPackage, 0, len(packages))
	libraryPackages := make([]TargetPackage, 0, len(packages))
	for _, pkg := range packages {
		if pkg.Name == "main" {
			continue
		}
		targetPackage, err := targetPackageFromFact(module, pkg)
		if err != nil {
			return Target{}, false, err
		}
		modulePackages = append(modulePackages, targetPackage)
		if internalModulePackage(pkg.ModuleRelativeDir) {
			continue
		}
		if !pkg.LoadCompleteness.Complete() {
			// Aggregate public-library authority requires an exact build-selected
			// package. Missing or incomplete package-local go-list authority fails
			// closed, while an internal-only package remains analysis context.
			return Target{}, false, nil
		}
		if !pkg.DeclarationsScanned {
			// Without a complete scan, absence of exported declarations is not
			// evidence. Keep exact executables but omit this aggregate target.
			return Target{}, false, nil
		}
		declarations, err := gofacts.CanonicalPackageDeclarations(pkg.Declarations)
		if err != nil {
			return Target{}, false, fmt.Errorf("analysis target: package %q declarations: %w", pkg.CanonicalPath, err)
		}
		hasExport := false
		for _, declaration := range declarations {
			if declaration.ExportedAPI() {
				hasExport = true
				break
			}
		}
		if hasExport {
			libraryPackages = append(libraryPackages, targetPackage)
		}
	}
	modulePackages = canonicalTargetPackages(modulePackages)
	libraryPackages = canonicalTargetPackages(libraryPackages)
	if len(libraryPackages) == 0 {
		return Target{}, false, nil
	}

	moduleDir, err := canonicalPackageDir(module.ModuleDir)
	if err != nil {
		return Target{}, false, fmt.Errorf("analysis target: module %q: %w", module.ModulePath, err)
	}
	target := Target{
		Version: Version, Kind: KindModuleLibrary, ModuleID: module.ID,
		ModulePath: module.ModulePath, ModuleDir: moduleDir,
		ModulePackages: modulePackages, LibraryPackages: libraryPackages,
		RootBoundary: RootBoundaryExactModuleAPI, Roots: []Root{},
	}
	target.Ref, err = targetRef(target)
	if err != nil {
		return Target{}, false, err
	}
	if err := target.Validate(); err != nil {
		return Target{}, false, err
	}
	return target, true, nil
}

func modulePackageInventoryComplete(module gofacts.ModuleFact, packages []gofacts.PackageFact) bool {
	count := len(packages)
	return count > 0 && exactOrUnspecifiedCount(module.PackagesCount, count) &&
		exactOrUnspecifiedCount(module.RetainedPackagesCount, count) &&
		(module.Coverage.PackagesDiscovered == 0 || module.Coverage.PackagesDiscovered == count) &&
		(module.Coverage.PackagesRetained == 0 || module.Coverage.PackagesRetained == count)
}

func exactOrUnspecifiedCount(value, exact int) bool {
	return value == 0 || value == exact
}

func targetPackageFromFact(module gofacts.ModuleFact, pkg gofacts.PackageFact) (TargetPackage, error) {
	if pkg.ModuleID != module.ID || pkg.ModulePath != module.ModulePath {
		return TargetPackage{}, fmt.Errorf("analysis target: package %q has inconsistent module identity", pkg.CanonicalPath)
	}
	packageDir, err := canonicalPackageDir(pkg.PackageDir)
	if err != nil {
		return TargetPackage{}, fmt.Errorf("analysis target: package %q: %w", pkg.CanonicalPath, err)
	}
	result := TargetPackage{PackagePath: pkg.CanonicalPath, PackageDir: packageDir}
	if !packageBelongsToModule(module.ModulePath, result.PackagePath) ||
		!directoryBelongsToModule(canonicalDirForMatch(module.ModuleDir), result.PackageDir) {
		return TargetPackage{}, fmt.Errorf("analysis target: package %q is outside module %q", pkg.CanonicalPath, module.ModulePath)
	}
	return result, nil
}

func internalModulePackage(moduleRelativeDir string) bool {
	for _, segment := range strings.Split(canonicalDirForMatch(moduleRelativeDir), "/") {
		if segment == "internal" {
			return true
		}
	}
	return false
}

func targetRef(target Target) (string, error) {
	wire, err := canonicalTargetJSON(target)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return "at-" + hex.EncodeToString(digest[:12]), nil
}

func canonicalTargetJSON(target Target) ([]byte, error) {
	identity := target
	identity.Ref = ""
	wire, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("analysis target: encode identity: %w", err)
	}
	return wire, nil
}

func findEntrypointPackage(
	packages map[string]gofacts.PackageFact,
	modules map[string]gofacts.ModuleFact,
	entrypoint gofacts.Entrypoint,
) (gofacts.PackageFact, error) {
	var matches []gofacts.PackageFact
	for _, pkg := range packages {
		module := modules[pkg.ModuleID]
		if module.ModulePath == entrypoint.ModulePath &&
			canonicalDirForMatch(module.ModuleDir) == canonicalDirForMatch(entrypoint.ModuleDir) &&
			pkg.CanonicalPath == entrypoint.ImportPath &&
			canonicalDirForMatch(pkg.PackageDir) == canonicalDirForMatch(entrypoint.PackageDir) {
			matches = append(matches, pkg)
		}
	}
	if len(matches) == 0 {
		return gofacts.PackageFact{}, fmt.Errorf(
			"analysis target: entrypoint package %q is absent from exact package facts",
			entrypoint.ImportPath,
		)
	}
	if len(matches) != 1 {
		return gofacts.PackageFact{}, fmt.Errorf(
			"analysis target: entrypoint package %q has %d exact package matches",
			entrypoint.ImportPath, len(matches),
		)
	}
	return matches[0], nil
}

func canonicalRoot(rootPath string, line int) (Root, error) {
	clean := canonicalDirForMatch(rootPath)
	if clean == "." || strings.HasPrefix(clean, "../") || path.IsAbs(rootPath) || line <= 0 {
		return Root{}, fmt.Errorf("invalid exact main declaration %q:%d", rootPath, line)
	}
	return Root{Path: clean, Line: line}, nil
}

func canonicalRoots(roots []Root) []Root {
	result := append([]Root{}, roots...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Line < result[j].Line
	})
	deduplicated := result[:0]
	for _, root := range result {
		if len(deduplicated) == 0 || deduplicated[len(deduplicated)-1] != root {
			deduplicated = append(deduplicated, root)
		}
	}
	return deduplicated
}

func canonicalPackageDir(value string) (string, error) {
	clean := canonicalDirForMatch(value)
	if path.IsAbs(value) || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid repository-relative package directory %q", value)
	}
	return clean, nil
}

func canonicalDirForMatch(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "."
	}
	return path.Clean(value)
}

func packageIdentityKey(moduleID, packagePath string) string {
	return moduleID + "\x00" + packagePath
}

func candidateKey(modulePath, moduleDir, packagePath string) string {
	return modulePath + "@" + canonicalDirForMatch(moduleDir) + "::" + packagePath
}

func moduleCandidateKey(modulePath, moduleDir string) string {
	return modulePath + "@" + canonicalDirForMatch(moduleDir) + "::module_library"
}

func targetCandidateKey(target Target) string {
	if target.Kind == KindModuleLibrary {
		return moduleCandidateKey(target.ModulePath, target.ModuleDir)
	}
	return candidateKey(target.ModulePath, target.ModuleDir, target.PackagePath)
}

// ExactCandidateKeyModuleDir extracts only the canonical module-directory
// routing hint from a typed candidate key. The hint may narrow deterministic
// Go-fact loading for an explicit --target, but it is never target authority:
// the resulting catalog must still contain and resolve the complete key.
func ExactCandidateKeyModuleDir(value string) (string, bool) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", false
	}
	separator := strings.Index(value, "::")
	if separator <= 0 || separator+2 >= len(value) || strings.Contains(value[separator+2:], "::") {
		return "", false
	}
	prefix := value[:separator]
	at := strings.LastIndex(prefix, "@")
	if at <= 0 || at+1 >= len(prefix) {
		return "", false
	}
	moduleDir := prefix[at+1:]
	canonical := canonicalDirForMatch(moduleDir)
	if moduleDir != canonical || path.IsAbs(moduleDir) || strings.HasPrefix(canonical, "../") {
		return "", false
	}
	return canonical, true
}
