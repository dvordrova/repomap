package gofacts

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
)

type workspaceDependencyPackage struct {
	name           string
	modulePath     string
	packagePath    string
	repositoryPath string
}

// buildDependencyCatalog converts the dependency objects returned by the
// existing per-module go-list call into the language-neutral dependency core.
// It executes no command and performs no path-spelling classification.
func buildDependencyCatalog(repoRoot string, loads []dependencyPackageLoad) (dependencies.Catalog, []string, error) {
	workspace := make(map[string]workspaceDependencyPackage)
	for _, load := range loads {
		for _, pkg := range load.packages {
			if pkg.DepOnly {
				continue
			}
			value, ok := workspacePackage(repoRoot, pkg)
			if !ok {
				continue
			}
			key := dependencyPackageKey(value.packagePath, value.repositoryPath)
			if previous, exists := workspace[key]; exists && previous != value {
				return dependencies.Catalog{}, nil, fmt.Errorf("conflicting workspace package metadata for %q", value.packagePath)
			}
			workspace[key] = value
		}
	}

	var importerRows []dependencies.Importer
	var dependencyRows []dependencies.Dependency
	var omissions []dependencies.Omission
	var warnings []string
	for _, load := range loads {
		metadata, err := dependencyMetadata(load.packages)
		if err != nil {
			return dependencies.Catalog{}, nil, err
		}
		for _, pkg := range load.packages {
			if pkg.DepOnly {
				continue
			}
			imports := canonicalImportPaths(pkg.Imports)
			if len(imports) == 0 {
				continue
			}
			effect := effectImports(pkg)
			importer, ok := dependencyImporter(repoRoot, pkg)
			if ok {
				importer, err = dependencies.SealImporter(importer)
				ok = err == nil
			}
			if ok {
				importerRows = append(importerRows, importer)
			} else {
				warnings = append(warnings, fmt.Sprintf("package %q has unavailable dependency importer identity", pkg.ImportPath))
			}
			for _, importedPath := range imports {
				if !ok {
					omissions = append(omissions, dependencyOmission(
						dependencies.Importer{}, pkg.ImportPath, importedPath,
						dependencies.OmissionImporterIdentityUnavailable,
					))
					continue
				}
				loaded, exists := metadata[importedPath]
				if !exists {
					warnings = append(warnings, fmt.Sprintf(
						"package %q import %q has no exact go-list dependency object",
						pkg.ImportPath, importedPath,
					))
					omissions = append(omissions, dependencyOmission(
						importer, pkg.ImportPath, importedPath,
						dependencies.OmissionDependencyMetadataMissing,
					))
					continue
				}
				value, reason, valueErr := goDependency(repoRoot, loaded, workspace)
				if valueErr != nil {
					warnings = append(warnings, fmt.Sprintf(
						"package %q import %q: %v", pkg.ImportPath, importedPath, valueErr,
					))
					omissions = append(omissions, dependencyOmission(importer, pkg.ImportPath, importedPath, reason))
					continue
				}
				value.ImporterRefs = []string{importer.Ref}
				if effect[importedPath] {
					value.EffectImporterRefs = []string{importer.Ref}
				}
				dependencyRows = append(dependencyRows, value)
			}
		}
	}
	catalog, err := dependencies.BuildWithOmissions(importerRows, dependencyRows, omissions)
	if err != nil {
		return dependencies.Catalog{}, nil, err
	}
	return catalog, canonicalWarnings(warnings), nil
}

func dependencyMetadata(packages []goListPackage) (map[string]goListPackage, error) {
	result := make(map[string]goListPackage, len(packages))
	for _, pkg := range packages {
		if pkg.ImportPath == "" {
			continue
		}
		if previous, exists := result[pkg.ImportPath]; exists {
			if !sameDependencyPackageMetadata(previous, pkg) {
				return nil, fmt.Errorf("conflicting go-list dependency objects for %q", pkg.ImportPath)
			}
			continue
		}
		result[pkg.ImportPath] = pkg
	}
	return result, nil
}

func sameDependencyPackageMetadata(left, right goListPackage) bool {
	if left.ImportPath != right.ImportPath || left.Dir != right.Dir || left.Name != right.Name ||
		left.Standard != right.Standard || left.Error == nil != (right.Error == nil) {
		return false
	}
	return sameGoListModule(left.Module, right.Module)
}

func sameGoListModule(left, right *goListModule) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left.Path != right.Path || left.Version != right.Version || left.Dir != right.Dir || left.Main != right.Main {
		return false
	}
	return sameGoListModule(left.Replace, right.Replace)
}

func dependencyImporter(repoRoot string, pkg goListPackage) (dependencies.Importer, bool) {
	workspace, ok := workspacePackage(repoRoot, pkg)
	if !ok {
		return dependencies.Importer{}, false
	}
	return dependencies.Importer{
		Language:       "go",
		Name:           workspace.name,
		ModulePath:     workspace.modulePath,
		PackagePath:    workspace.packagePath,
		RepositoryPath: workspace.repositoryPath,
	}, true
}

func workspacePackage(repoRoot string, pkg goListPackage) (workspaceDependencyPackage, bool) {
	if pkg.ImportPath == "" || pkg.Name == "" || pkg.Module == nil || pkg.Module.Path == "" {
		return workspaceDependencyPackage{}, false
	}
	repositoryPath := repositoryRelativeMetadataPath(repoRoot, pkg.Dir)
	if repositoryPath == "" {
		return workspaceDependencyPackage{}, false
	}
	return workspaceDependencyPackage{
		name:           pkg.Name,
		modulePath:     pkg.Module.Path,
		packagePath:    pkg.ImportPath,
		repositoryPath: repositoryPath,
	}, true
}

func goDependency(
	repoRoot string,
	pkg goListPackage,
	workspace map[string]workspaceDependencyPackage,
) (dependencies.Dependency, dependencies.OmissionReason, error) {
	if pkg.ImportPath == "" || pkg.Name == "" {
		return dependencies.Dependency{}, dependencies.OmissionDependencyIdentityMissing,
			fmt.Errorf("go-list package identity is incomplete")
	}
	if pkg.Error != nil {
		return dependencies.Dependency{}, dependencies.OmissionDependencyLoadUnavailable,
			fmt.Errorf("go-list dependency is unavailable")
	}
	if pkg.Standard {
		return dependencies.Dependency{
			Language: "go", Kind: dependencies.KindStdlib,
			Name: pkg.Name, PackagePath: pkg.ImportPath,
		}, "", nil
	}
	if repositoryPath := repositoryRelativeMetadataPath(repoRoot, pkg.Dir); repositoryPath != "" {
		if owned, ok := workspace[dependencyPackageKey(pkg.ImportPath, repositoryPath)]; ok {
			return dependencies.Dependency{
				Language: "go", Kind: dependencies.KindWorkspace,
				Name: owned.name, ModulePath: owned.modulePath,
				PackagePath: owned.packagePath, RepositoryPath: owned.repositoryPath,
			}, "", nil
		}
	}
	if pkg.Module == nil || pkg.Module.Path == "" {
		return dependencies.Dependency{}, dependencies.OmissionModuleAuthorityMissing,
			fmt.Errorf("non-standard dependency has no exact module authority")
	}
	return dependencies.Dependency{
		Language: "go", Kind: dependencies.KindExternal,
		Name: pkg.Name, ModulePath: pkg.Module.Path, ModuleVersion: pkg.Module.Version,
		PackagePath: pkg.ImportPath, Replacement: dependencyReplacement(repoRoot, pkg.Module.Replace),
	}, "", nil
}

func dependencyOmission(
	importer dependencies.Importer,
	importerPackagePath string,
	packagePath string,
	reason dependencies.OmissionReason,
) dependencies.Omission {
	return dependencies.Omission{
		ImporterRef: importer.Ref, ImporterPackagePath: importerPackagePath,
		PackagePath: packagePath, Reason: reason,
	}
}

func dependencyReplacement(repoRoot string, module *goListModule) *dependencies.Replacement {
	if module == nil {
		return nil
	}
	if module.Version != "" {
		return &dependencies.Replacement{ModulePath: module.Path, ModuleVersion: module.Version}
	}
	return &dependencies.Replacement{
		Local:          true,
		RepositoryPath: repositoryRelativeMetadataPath(repoRoot, module.Dir),
	}
}

func dependencyPackageKey(packagePath, repositoryPath string) string {
	return packagePath + "\x00" + repositoryPath
}

func canonicalImportPaths(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		// C is the cgo pseudo-package, not a Go package dependency and has
		// no go-list dependency object.
		if value == "" || value == "C" || strings.TrimSpace(value) != value ||
			(write > 0 && result[write-1] == value) {
			continue
		}
		result[write] = value
		write++
	}
	return result[:write]
}

func canonicalWarnings(values []string) []string {
	sort.Strings(values)
	write := 0
	for _, value := range values {
		if write > 0 && values[write-1] == value {
			continue
		}
		values[write] = value
		write++
	}
	return values[:write]
}

// effectImports are the paths a package imports for the effect of
// importing them: an import spec of the path in one of the package's own
// source files is named `_` (`import _ "github.com/go-sql-driver/mysql"`),
// the author's statement that the file needs the package's init, which
// registers a driver or a plugin. Go runs that init once for the program
// however else the package imports the path, so a named import of it in
// another file (casdoor's syncer_database.go beside ormer.go's `_`) takes
// nothing away. Only the files the build selected (GoFiles, CgoFiles)
// count; a file that cannot be read makes no claim.
func effectImports(pkg goListPackage) map[string]bool {
	blank := make(map[string]bool)
	files := token.NewFileSet()
	for _, name := range append(append([]string(nil), pkg.GoFiles...), pkg.CgoFiles...) {
		file, err := parser.ParseFile(files, filepath.Join(pkg.Dir, name), nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil
			}
			if spec.Name != nil && spec.Name.Name == "_" {
				blank[path] = true
			}
		}
	}
	return blank
}
