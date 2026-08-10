package report

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
)

const (
	LibraryAPIReportProjectionVersion = 1
	MaxLibraryAPIDeclarations         = 4096
	maxLibraryAPISnapshotReplayBytes  = 64 << 20
)

// LibraryAPIReportProjection is the report-owned, bounded exact exported API
// of one selected module-library target. TotalDeclarations is complete for the
// sealed target; ShownDeclarations is the globally capped declaration count.
type LibraryAPIReportProjection struct {
	Version           int                 `json:"version"`
	ModulePath        string              `json:"module_path"`
	ModuleDir         string              `json:"module_dir"`
	TotalDeclarations int                 `json:"total_declarations"`
	ShownDeclarations int                 `json:"shown_declarations"`
	Packages          []LibraryAPIPackage `json:"packages"`
}

// LibraryAPIPackage keeps exact package identity separate from its
// module-relative display path. Counts describe the complete exported package
// inventory even when Declarations is truncated by the global report cap.
type LibraryAPIPackage struct {
	PackagePath       string                  `json:"package_path"`
	PackageDir        string                  `json:"package_dir"`
	DisplayPath       string                  `json:"display_path"`
	TotalDeclarations int                     `json:"total_declarations"`
	ShownDeclarations int                     `json:"shown_declarations"`
	Counts            LibraryAPICounts        `json:"counts"`
	Declarations      []LibraryAPIDeclaration `json:"declarations"`
}

// LibraryAPICounts is the complete per-kind count for one package.
type LibraryAPICounts struct {
	Functions int `json:"functions"`
	Methods   int `json:"methods"`
	Types     int `json:"types"`
	Consts    int `json:"consts"`
	Vars      int `json:"vars"`
}

// LibraryAPIDeclaration is one exact exported Go declaration location. The
// report deliberately omits signatures, comments, bodies and source text.
type LibraryAPIDeclaration struct {
	Kind     gofacts.PackageDeclarationKind `json:"kind"`
	Name     string                         `json:"name"`
	Receiver string                         `json:"receiver,omitempty"`
	Path     string                         `json:"path"`
	Line     int                            `json:"line"`
	Column   int                            `json:"column"`
}

// ProjectLibraryAPI derives the exact public declaration inventory from the
// selected target's build-scoped Go facts. The global cap is allocated in
// canonical package round-robin order so a large root package cannot starve
// smaller public packages; declarations within every package remain a
// canonical prefix.
func ProjectLibraryAPI(
	facts gofacts.Facts,
	target analysistarget.Target,
) (*LibraryAPIReportProjection, error) {
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("library API projection: %w", err)
	}
	if target.Kind != analysistarget.KindModuleLibrary {
		return nil, nil
	}
	matchingModules := 0
	for _, module := range facts.Modules {
		if module.ID == target.ModuleID && module.ModulePath == target.ModulePath &&
			module.ModuleDir == target.ModuleDir {
			matchingModules++
		}
	}
	if matchingModules != 1 {
		return nil, fmt.Errorf("library API projection: selected module authority is unavailable")
	}

	factsByPackage := make(map[string]gofacts.PackageFact, len(facts.Packages))
	for _, pkg := range facts.Packages {
		key := libraryAPIPackageKey(pkg.ModuleID, pkg.CanonicalPath)
		if _, duplicate := factsByPackage[key]; duplicate {
			return nil, fmt.Errorf("library API projection: duplicate package facts for %q", pkg.CanonicalPath)
		}
		factsByPackage[key] = pkg
	}

	projection := &LibraryAPIReportProjection{
		Version:    LibraryAPIReportProjectionVersion,
		ModulePath: target.ModulePath,
		ModuleDir:  target.ModuleDir,
		Packages:   make([]LibraryAPIPackage, 0, len(target.LibraryPackages)),
	}
	complete := make([][]LibraryAPIDeclaration, 0, len(target.LibraryPackages))
	for _, targetPackage := range target.LibraryPackages {
		pkg, ok := factsByPackage[libraryAPIPackageKey(target.ModuleID, targetPackage.PackagePath)]
		if !ok || pkg.Name == "main" || pkg.ModulePath != target.ModulePath ||
			pkg.PackageDir != targetPackage.PackageDir ||
			!pkg.DeclarationsScanned {
			return nil, fmt.Errorf(
				"library API projection: selected package %q declaration authority is unavailable",
				targetPackage.PackagePath,
			)
		}
		canonical, err := gofacts.CanonicalPackageDeclarations(pkg.Declarations)
		if err != nil {
			return nil, fmt.Errorf("library API projection: package %q: %w", targetPackage.PackagePath, err)
		}
		files := make(map[string]struct{}, len(pkg.Files))
		for _, sourcePath := range pkg.Files {
			files[sourcePath] = struct{}{}
		}
		declarations := make([]LibraryAPIDeclaration, 0, len(canonical))
		counts := LibraryAPICounts{}
		for _, declaration := range canonical {
			if !declaration.ExportedAPI() {
				continue
			}
			if _, selected := files[declaration.Path]; !selected {
				return nil, fmt.Errorf(
					"library API projection: package %q declaration %q is outside its build-selected files",
					targetPackage.PackagePath,
					declaration.Label(),
				)
			}
			counts.add(declaration.Kind)
			declarations = append(declarations, LibraryAPIDeclaration{
				Kind: declaration.Kind, Name: declaration.Name, Receiver: declaration.Receiver,
				Path: declaration.Path, Line: declaration.Line, Column: declaration.Column,
			})
		}
		if len(declarations) == 0 {
			return nil, fmt.Errorf(
				"library API projection: selected package %q has no exported declaration",
				targetPackage.PackagePath,
			)
		}
		displayPath, err := libraryAPIDisplayPath(target.ModuleDir, targetPackage.PackageDir)
		if err != nil {
			return nil, fmt.Errorf("library API projection: package %q: %w", targetPackage.PackagePath, err)
		}
		projection.Packages = append(projection.Packages, LibraryAPIPackage{
			PackagePath:       targetPackage.PackagePath,
			PackageDir:        targetPackage.PackageDir,
			DisplayPath:       displayPath,
			TotalDeclarations: len(declarations),
			Counts:            counts,
			Declarations:      []LibraryAPIDeclaration{},
		})
		complete = append(complete, declarations)
		projection.TotalDeclarations += len(declarations)
	}

	remaining := min(projection.TotalDeclarations, MaxLibraryAPIDeclarations)
	for declarationIndex := 0; remaining > 0; declarationIndex++ {
		for packageIndex := range projection.Packages {
			if declarationIndex >= len(complete[packageIndex]) {
				continue
			}
			projection.Packages[packageIndex].Declarations = append(
				projection.Packages[packageIndex].Declarations,
				complete[packageIndex][declarationIndex],
			)
			projection.Packages[packageIndex].ShownDeclarations++
			projection.ShownDeclarations++
			remaining--
			if remaining == 0 {
				break
			}
		}
	}
	return projection, nil
}

func (counts *LibraryAPICounts) add(kind gofacts.PackageDeclarationKind) {
	switch kind {
	case gofacts.PackageDeclarationFunc:
		counts.Functions++
	case gofacts.PackageDeclarationMethod:
		counts.Methods++
	case gofacts.PackageDeclarationType:
		counts.Types++
	case gofacts.PackageDeclarationConst:
		counts.Consts++
	case gofacts.PackageDeclarationVar:
		counts.Vars++
	}
}

func (counts LibraryAPICounts) total() int {
	return counts.Functions + counts.Methods + counts.Types + counts.Consts + counts.Vars
}

func libraryAPIPackageKey(moduleID, packagePath string) string {
	return moduleID + "\x00" + packagePath
}

func libraryAPIDisplayPath(moduleDir, packageDir string) (string, error) {
	switch {
	case moduleDir == ".":
		return packageDir, nil
	case packageDir == moduleDir:
		return ".", nil
	case strings.HasPrefix(packageDir, moduleDir+"/"):
		return strings.TrimPrefix(packageDir, moduleDir+"/"), nil
	default:
		return "", fmt.Errorf("package directory %q is outside module directory %q", packageDir, moduleDir)
	}
}

func rehydrateLibraryAPIProjection(
	root *os.Root,
	data *ReportData,
	requirePersisted bool,
) error {
	if data == nil {
		return fmt.Errorf("library API projection: report data is required")
	}
	if data.AnalysisTarget == nil || data.AnalysisTarget.Kind != analysistarget.KindModuleLibrary {
		if data.LibraryAPI != nil {
			return fmt.Errorf("library API projection: non-module target carries a library API")
		}
		return nil
	}
	if data.repositoryGoFacts == nil {
		if root == nil {
			return fmt.Errorf("library API projection: snapshot authority is unavailable")
		}
		snapshotRaw, err := readManifestFile(root, "snapshot.json", maxLibraryAPISnapshotReplayBytes)
		if err != nil {
			return fmt.Errorf("library API projection: read declaration authority: %w", err)
		}
		facts, err := decodeSnapshotExactGoFacts(snapshotRaw)
		if err != nil {
			return fmt.Errorf("library API projection: decode declaration authority: %w", err)
		}
		data.repositoryGoFacts = &facts
	}
	expected, err := ProjectLibraryAPI(*data.repositoryGoFacts, *data.AnalysisTarget)
	if err != nil {
		return err
	}
	if requirePersisted {
		if !reflect.DeepEqual(data.LibraryAPI, expected) {
			return fmt.Errorf("library API projection: saved projection does not match snapshot authority")
		}
		return validateLibraryAPIProjection(data.AnalysisTarget, data.LibraryAPI, data.OpenablePaths)
	}
	data.LibraryAPI = expected
	return nil
}

func rehydrateLibraryAPIProjectionFromRunDir(runDir string, data *ReportData) error {
	if data == nil || data.AnalysisTarget == nil || data.AnalysisTarget.Kind != analysistarget.KindModuleLibrary {
		return rehydrateLibraryAPIProjection(nil, data, false)
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("library API projection: open run directory: %w", err)
	}
	defer root.Close()
	return rehydrateLibraryAPIProjection(root, data, false)
}

func validateLibraryAPIProjection(
	target *analysistarget.Target,
	projection *LibraryAPIReportProjection,
	openablePaths []string,
) error {
	if target == nil || target.Kind != analysistarget.KindModuleLibrary {
		if projection != nil {
			return fmt.Errorf("library API projection: non-module target carries a library API")
		}
		return nil
	}
	if projection == nil {
		return fmt.Errorf("library API projection: selected module library has no exact public API")
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("library API projection: %w", err)
	}
	if projection.Version != LibraryAPIReportProjectionVersion ||
		projection.ModulePath != target.ModulePath || projection.ModuleDir != target.ModuleDir ||
		len(projection.Packages) != len(target.LibraryPackages) ||
		projection.TotalDeclarations <= 0 || projection.ShownDeclarations <= 0 ||
		projection.ShownDeclarations > min(projection.TotalDeclarations, MaxLibraryAPIDeclarations) {
		return fmt.Errorf("library API projection: invalid identity or totals")
	}
	openable := make(map[string]struct{}, len(openablePaths))
	for _, sourcePath := range openablePaths {
		openable[sourcePath] = struct{}{}
	}
	total, shown := 0, 0
	for packageIndex := range projection.Packages {
		pkg := projection.Packages[packageIndex]
		wantPackage := target.LibraryPackages[packageIndex]
		displayPath, err := libraryAPIDisplayPath(target.ModuleDir, wantPackage.PackageDir)
		if err != nil || pkg.PackagePath != wantPackage.PackagePath ||
			pkg.PackageDir != wantPackage.PackageDir || pkg.DisplayPath != displayPath ||
			pkg.TotalDeclarations <= 0 || pkg.Counts.total() != pkg.TotalDeclarations ||
			pkg.ShownDeclarations != len(pkg.Declarations) ||
			pkg.ShownDeclarations > pkg.TotalDeclarations {
			return fmt.Errorf("library API projection: package %d is invalid", packageIndex)
		}
		canonical := make([]gofacts.PackageDeclaration, len(pkg.Declarations))
		for declarationIndex, declaration := range pkg.Declarations {
			canonical[declarationIndex] = gofacts.PackageDeclaration{
				Kind: declaration.Kind, Name: declaration.Name, Receiver: declaration.Receiver,
				Path: declaration.Path, Line: declaration.Line, Column: declaration.Column,
			}
			if !canonical[declarationIndex].ExportedAPI() {
				return fmt.Errorf("library API projection: package %d declaration %d is not exported", packageIndex, declarationIndex)
			}
			if _, authorized := openable[declaration.Path]; !authorized {
				return fmt.Errorf("library API projection: source %q is not authorized", declaration.Path)
			}
		}
		canonicalSorted, err := gofacts.CanonicalPackageDeclarations(canonical)
		if err != nil || !reflect.DeepEqual(canonical, canonicalSorted) {
			return fmt.Errorf("library API projection: package %d declarations are not canonical", packageIndex)
		}
		total += pkg.TotalDeclarations
		shown += pkg.ShownDeclarations
	}
	if total != projection.TotalDeclarations || shown != projection.ShownDeclarations ||
		shown != min(total, MaxLibraryAPIDeclarations) {
		return fmt.Errorf("library API projection: aggregate counts are not truthful")
	}
	return nil
}
