package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	AnalysisTargetExecutablePackage = "executable_package"
	AnalysisTargetModuleLibrary     = "module_library"

	// MaxPackageDiagnostics is the former ordinary collection size. It remains
	// only as a scale-warning threshold; every distinct diagnostic is retained.
	MaxPackageDiagnostics   = 128
	advisoryDiagnosticBytes = 512
)

// Input is an exact, already-selected Go analysis boundary. The package does
// not discover targets and no repository-wide compatibility mode exists.
type Input struct {
	ModuleDirs     []string
	Packages       []PackageInput
	AnalysisTarget *AnalysisTargetInput
}

type AnalysisTargetInput struct {
	TargetRef      string
	Kind           string
	ModuleID       string
	ModulePath     string
	ModuleDir      string
	PackagePath    string
	TargetPackages []string
	Roots          []AnalysisTargetRootInput
}

type AnalysisTargetRootInput struct {
	Path string
	Line int
}

type PackageInput struct {
	Path      string
	ModuleDir string
}

func normalizeInput(input Input) (Input, error) {
	input.ModuleDirs = normalizeModuleDirs(input.ModuleDirs)
	input.Packages = normalizePackageInputs(input.Packages)
	target, err := normalizeAnalysisTargetInput(input.AnalysisTarget, input.Packages)
	if err != nil {
		return Input{}, err
	}
	input.AnalysisTarget = target
	return input, nil
}

func normalizeAnalysisTargetInput(
	target *AnalysisTargetInput,
	packages []PackageInput,
) (*AnalysisTargetInput, error) {
	if target == nil {
		return nil, fmt.Errorf("surface discovery: exact analysis target is required")
	}
	result := &AnalysisTargetInput{
		TargetRef: target.TargetRef, Kind: target.Kind,
		ModuleID: target.ModuleID, ModulePath: target.ModulePath,
		ModuleDir: cleanRepositoryPath(target.ModuleDir), PackagePath: target.PackagePath,
		TargetPackages: append([]string(nil), target.TargetPackages...),
		Roots:          append([]AnalysisTargetRootInput(nil), target.Roots...),
	}
	if !validDirectCallTargetIdentity(result.TargetRef) || !validDirectCallTargetIdentity(result.ModuleID) ||
		!validDirectCallTargetIdentity(result.ModulePath) || result.ModuleDir == "" ||
		result.ModuleDir != target.ModuleDir || strings.TrimSpace(result.Kind) != result.Kind {
		return nil, fmt.Errorf("surface discovery: analysis target module identity is required")
	}
	for _, packagePath := range result.TargetPackages {
		if !validDirectCallTargetIdentity(packagePath) {
			return nil, fmt.Errorf("surface discovery: analysis target has an invalid target package")
		}
	}
	if len(result.TargetPackages) == 0 ||
		!sort.StringsAreSorted(result.TargetPackages) || !uniqueStrings(result.TargetPackages) {
		return nil, fmt.Errorf("surface discovery: analysis target packages are not canonical sorted unique order")
	}
	admitted := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		admitted[pkg.ModuleDir+"\x00"+pkg.Path] = struct{}{}
	}
	for _, packagePath := range result.TargetPackages {
		if _, available := admitted[result.ModuleDir+"\x00"+packagePath]; !available {
			return nil, fmt.Errorf(
				"surface discovery: analysis target package %q is outside the admitted module package scope",
				packagePath,
			)
		}
	}
	for index := range result.Roots {
		rootPath := cleanRepositoryPath(result.Roots[index].Path)
		if rootPath == "" || path.Ext(rootPath) != ".go" || result.Roots[index].Line <= 0 {
			return nil, fmt.Errorf("surface discovery: analysis target has an invalid exact root")
		}
		result.Roots[index].Path = rootPath
	}
	sort.Slice(result.Roots, func(i, j int) bool {
		if result.Roots[i].Path != result.Roots[j].Path {
			return result.Roots[i].Path < result.Roots[j].Path
		}
		return result.Roots[i].Line < result.Roots[j].Line
	})
	for index := 1; index < len(result.Roots); index++ {
		if result.Roots[index] == result.Roots[index-1] {
			return nil, fmt.Errorf("surface discovery: analysis target has duplicate exact roots")
		}
	}
	switch result.Kind {
	case AnalysisTargetExecutablePackage:
		if result.PackagePath == "" || len(result.TargetPackages) != 1 ||
			result.TargetPackages[0] != result.PackagePath || len(result.Roots) == 0 {
			return nil, fmt.Errorf("surface discovery: executable analysis target requires one exact package and roots")
		}
	case AnalysisTargetModuleLibrary:
		if result.PackagePath != "" || len(result.Roots) != 0 {
			return nil, fmt.Errorf("surface discovery: module library analysis target cannot declare process roots")
		}
	default:
		return nil, fmt.Errorf("surface discovery: invalid analysis target kind %q", result.Kind)
	}
	return result, nil
}

func normalizePackageInputs(values []PackageInput) []PackageInput {
	unique := make(map[string]PackageInput, len(values))
	for _, value := range values {
		value.Path = strings.TrimSpace(value.Path)
		value.ModuleDir = cleanRepositoryPath(value.ModuleDir)
		if value.Path == "" || value.ModuleDir == "" {
			continue
		}
		unique[value.ModuleDir+"\x00"+value.Path] = value
	}
	result := make([]PackageInput, 0, len(unique))
	for _, value := range unique {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ModuleDir != result[j].ModuleDir {
			return result[i].ModuleDir < result[j].ModuleDir
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func normalizeModuleDirs(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = cleanRepositoryPath(value); value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func cleanRepositoryPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || path.IsAbs(value) {
		return ""
	}
	value = path.Clean(value)
	if value == ".." || strings.HasPrefix(value, "../") || !fs.ValidPath(value) {
		return ""
	}
	return value
}

func packageSafeForSSA(pkg *packages.Package) bool {
	return pkg != nil && pkg.PkgPath != "" && pkg.Types != nil && !pkg.IllTyped && len(pkg.Errors) == 0
}

func (a *analyzer) recordPackageLoadOutcomes(allPackages map[string]*packages.Package) {
	ordered := make([]*packages.Package, 0, len(allPackages))
	for _, pkg := range allPackages {
		if pkg != nil {
			ordered = append(ordered, pkg)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return packageIdentity(ordered[i]) < packageIdentity(ordered[j]) })
	seen := make(map[string]struct{})
	for _, pkg := range ordered {
		errors := append([]packages.Error(nil), pkg.Errors...)
		sort.Slice(errors, func(i, j int) bool { return errors[i].Error() < errors[j].Error() })
		for _, packageError := range errors {
			location := a.packageErrorLocation(pkg, packageError.Pos)
			message := boundedDiagnosticMessage(packageError.Msg, a.root)
			id := stableDiagnosticID(packageIdentity(pkg), packageErrorKind(packageError.Kind), message, location)
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			a.result.Coverage.PackageDiagnostics = append(a.result.Coverage.PackageDiagnostics, PackageDiagnostic{
				ID: id, Kind: packageErrorKind(packageError.Kind), Message: message,
				Package: packageIdentity(pkg), Location: location,
			})
		}
	}
}

func (a *analyzer) analysisTargetSSADiagnostic(
	allPackages map[string]*packages.Package,
	targetPackage string,
) *PackageDiagnostic {
	target := allPackages[targetPackage]
	if target == nil {
		return nil
	}
	distance := map[string]int{packageIdentity(target): 0}
	queue := []*packages.Package{target}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentDistance := distance[packageIdentity(current)]
		imports := make([]string, 0, len(current.Imports))
		for importPath := range current.Imports {
			imports = append(imports, importPath)
		}
		sort.Strings(imports)
		for _, importPath := range imports {
			imported := current.Imports[importPath]
			identity := packageIdentity(imported)
			if identity == "" {
				continue
			}
			if _, seen := distance[identity]; seen {
				continue
			}
			distance[identity] = currentDistance + 1
			queue = append(queue, imported)
		}
	}
	best, bestDistance := -1, int(^uint(0)>>1)
	for index := range a.result.Coverage.PackageDiagnostics {
		diagnostic := &a.result.Coverage.PackageDiagnostics[index]
		diagnosticDistance, reachable := distance[diagnostic.Package]
		if !reachable || diagnostic.Location == nil || diagnostic.Location.Path == "" || diagnostic.Location.Line <= 0 {
			continue
		}
		if best < 0 || diagnosticDistance < bestDistance ||
			diagnosticDistance == bestDistance && diagnostic.ID < a.result.Coverage.PackageDiagnostics[best].ID {
			best, bestDistance = index, diagnosticDistance
		}
	}
	if best < 0 {
		return nil
	}
	result := a.result.Coverage.PackageDiagnostics[best]
	location := *result.Location
	result.Location = &location
	return &result
}

func packageIdentity(pkg *packages.Package) string {
	if pkg == nil {
		return ""
	}
	if pkg.PkgPath != "" {
		return pkg.PkgPath
	}
	return pkg.ID
}

func (a *analyzer) packageErrorLocation(pkg *packages.Package, position string) *Location {
	filename, line, column := parsePackageErrorPosition(position)
	if filename == "" || line <= 0 {
		return nil
	}
	if !filepath.IsAbs(filename) {
		filename = loadedPackageFilename(pkg, filename)
		if filename == "" {
			return nil
		}
	}
	relative, err := filepath.Rel(a.root, filename)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	relative = filepath.ToSlash(relative)
	if cleanRepositoryPath(relative) == "" {
		return nil
	}
	return &Location{Path: relative, Line: line, Column: column}
}

func loadedPackageFilename(pkg *packages.Package, filename string) string {
	if pkg == nil {
		return ""
	}
	filename = filepath.Clean(filename)
	files := append(append(append([]string(nil), pkg.GoFiles...), pkg.CompiledGoFiles...), pkg.OtherFiles...)
	sort.Strings(files)
	for _, candidate := range files {
		candidate = filepath.Clean(candidate)
		if !filepath.IsAbs(candidate) && pkg.Dir != "" {
			candidate = filepath.Join(pkg.Dir, candidate)
		}
		for _, root := range []string{pkg.Dir, packageModuleDir(pkg)} {
			if root == "" {
				continue
			}
			if relative, err := filepath.Rel(root, candidate); err == nil && filepath.Clean(relative) == filename {
				return candidate
			}
		}
	}
	return ""
}

func packageModuleDir(pkg *packages.Package) string {
	if pkg == nil || pkg.Module == nil {
		return ""
	}
	return pkg.Module.Dir
}

func parsePackageErrorPosition(position string) (string, int, int) {
	position = strings.TrimSpace(position)
	if position == "" || position == "-" {
		return "", 0, 0
	}
	last := strings.LastIndex(position, ":")
	if last < 0 {
		return "", 0, 0
	}
	lastValue, err := strconv.Atoi(position[last+1:])
	if err != nil {
		return "", 0, 0
	}
	prefix := position[:last]
	second := strings.LastIndex(prefix, ":")
	if second < 0 {
		return prefix, lastValue, 0
	}
	line, err := strconv.Atoi(prefix[second+1:])
	if err != nil {
		return prefix, lastValue, 0
	}
	return prefix[:second], line, lastValue
}

func packageErrorKind(kind packages.ErrorKind) string {
	switch kind {
	case packages.ListError:
		return "list"
	case packages.ParseError:
		return "parse"
	case packages.TypeError:
		return "type"
	default:
		return "unknown"
	}
}

func boundedDiagnosticMessage(message, root string) string {
	message = strings.Join(strings.Fields(message), " ")
	root = filepath.Clean(root)
	message = strings.ReplaceAll(message, root, ".")
	message = strings.ReplaceAll(message, filepath.ToSlash(root), ".")
	return message
}

func stableDiagnosticID(packagePath, kind, message string, location *Location) string {
	locationKey := ""
	if location != nil {
		locationKey = fmt.Sprintf("%s:%d:%d", location.Path, location.Line, location.Column)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{packagePath, kind, message, locationKey}, "\x00")))
	return "package-diagnostic-" + hex.EncodeToString(digest[:12])
}
