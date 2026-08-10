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

// Resolve enumerates exact package candidates and either applies one explicit
// override or performs conservative deterministic auto-selection. Ambiguity
// is a closed state with no Selected target.
func Resolve(facts gofacts.Facts, options Options) (Resolution, error) {
	candidates, err := Candidates(facts)
	if err != nil {
		return Resolution{}, err
	}
	resolution := Resolution{Candidates: candidates}
	if len(candidates) == 0 {
		resolution.State = ResolutionUnavailable
		resolution.Reason = "no exact Go package target candidates"
		if strings.TrimSpace(options.Override) != "" {
			return Resolution{}, fmt.Errorf("%w: %q", ErrOverrideNotFound, options.Override)
		}
		return resolution, nil
	}

	if override := strings.TrimSpace(options.Override); override != "" {
		matches := matchingCandidates(candidates, override)
		switch len(matches) {
		case 0:
			return Resolution{}, fmt.Errorf("%w: %q", ErrOverrideNotFound, override)
		case 1:
			return selectedResolution(candidates, matches[0].Target, "explicit override"), nil
		default:
			return Resolution{}, fmt.Errorf("%w: %q matches %d candidates", ErrOverrideAmbiguous, override, len(matches))
		}
	}

	target, reason, ok := autoSelect(candidates)
	if !ok {
		resolution.State = ResolutionAmbiguous
		resolution.Reason = "multiple plausible analysis target packages require an explicit override"
		return resolution, nil
	}
	return selectedResolution(candidates, target, reason), nil
}

// Candidates derives executable candidates from exact build-selected main
// anchors and library candidates from the remaining exact local packages.
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

	candidates := make([]Candidate, 0, len(packages))
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
			continue
		}

		target, err := newTarget(module, pkg, KindLibraryPackage, RootBoundaryExactPublicAPI, []Root{})
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, Candidate{
			Key: candidateKey(module.ModulePath, module.ModuleDir, pkg.CanonicalPath), Target: target, MainModule: rootAnalysisModule(module),
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
// invocation can consequently mark its own module as Main. For target
// auto-selection, "main" means the module at the analysis root. Nested fixture
// and example modules remain valid explicit targets but never compete with the
// ordinary root product.
func rootAnalysisModule(module gofacts.ModuleFact) bool {
	return module.Main && canonicalDirForMatch(module.ModuleDir) == "."
}

func autoSelect(candidates []Candidate) (Target, string, bool) {
	plausibleExecutables := make([]Candidate, 0)
	for _, candidate := range candidates {
		if candidate.MainModule && candidate.Target.Kind == KindExecutablePackage &&
			!auxiliaryEntrypointKind(candidate.EntrypointKind) {
			plausibleExecutables = append(plausibleExecutables, candidate)
		}
	}

	primary := filterCandidates(plausibleExecutables, func(candidate Candidate) bool {
		return candidate.EntrypointKind == "primary_binary" || candidate.EntrypointKind == "primary_application"
	})
	if len(primary) == 1 {
		return primary[0].Target, "unique primary executable package", true
	}
	if len(primary) > 1 {
		return Target{}, "", false
	}

	moduleMatches := filterCandidates(plausibleExecutables, func(candidate Candidate) bool {
		return packageBase(candidate.Target.PackageDir) == moduleBase(candidate.Target.ModulePath)
	})
	if len(moduleMatches) == 1 {
		return moduleMatches[0].Target, "unique executable matching the main module name", true
	}
	if len(moduleMatches) > 1 {
		return Target{}, "", false
	}

	rootLibraries := filterCandidates(candidates, func(candidate Candidate) bool {
		return candidate.MainModule && candidate.Target.Kind == KindLibraryPackage && candidate.Target.PackageDir == "."
	})
	if len(plausibleExecutables) == 0 && len(rootLibraries) == 1 {
		return rootLibraries[0].Target, "sole root library package", true
	}
	if len(plausibleExecutables) == 1 && len(rootLibraries) == 0 {
		return plausibleExecutables[0].Target, "sole plausible executable package", true
	}
	return Target{}, "", false
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

func matchingCandidates(candidates []Candidate, override string) []Candidate {
	matches := make([]Candidate, 0, 1)
	cleanDir := canonicalDirForMatch(override)
	for _, candidate := range candidates {
		if override == candidate.Target.Ref || override == candidate.Key || override == candidate.Target.PackagePath ||
			cleanDir == canonicalDirForMatch(candidate.Target.PackageDir) {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func selectedResolution(candidates []Candidate, target Target, reason string) Resolution {
	selected := target
	return Resolution{State: ResolutionSelected, Reason: reason, Selected: &selected, Candidates: candidates}
}

func filterCandidates(candidates []Candidate, keep func(Candidate) bool) []Candidate {
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if keep(candidate) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func auxiliaryEntrypointKind(kind string) bool {
	switch kind {
	case "tool", "example", "test_binary":
		return true
	default:
		return false
	}
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

func packageBase(packageDir string) string {
	if packageDir == "." {
		return ""
	}
	return path.Base(packageDir)
}

func moduleBase(modulePath string) string {
	clean := strings.TrimSuffix(path.Clean(modulePath), "/")
	base := path.Base(clean)
	if len(base) > 1 && base[0] == 'v' {
		for index := 1; index < len(base); index++ {
			if base[index] < '0' || base[index] > '9' {
				return base
			}
		}
		clean = path.Dir(clean)
		base = path.Base(clean)
	}
	return base
}
