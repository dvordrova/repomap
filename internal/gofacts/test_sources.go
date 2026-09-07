package gofacts

import (
	"fmt"
	"go/parser"
	"go/token"
	"sort"
	"strconv"

	"github.com/dvordrova/repomap/internal/reporead"
)

// TestSource is a build-selected source file reported by go list, independent
// of the ordinary package/API inventory. PackagePath names the package being
// tested even for an external test package or a test-only directory.
// Declarations are parsed syntax, not type-checked functions, test execution,
// coverage, or a claim about which production declaration a test exercises.
type TestSource struct {
	ModuleID            string               `json:"module_id"`
	ModulePath          string               `json:"module_path"`
	ModuleDir           string               `json:"module_dir"`
	PackagePath         string               `json:"package_path"`
	PackageName         string               `json:"package_name,omitempty"`
	Path                string               `json:"path"`
	External            bool                 `json:"external"`
	DeclarationsScanned bool                 `json:"declarations_scanned"`
	Declarations        []PackageDeclaration `json:"declarations"`
	Imports             []string             `json:"imports"`
}

// collectTestSources reuses the existing go-list rows; it does not enable
// Tests in the production packages/SSA load, create targets or load another
// compiler universe. Test-only root rows remain useful here even though they
// are ineligible for the ordinary package catalog.
func collectTestSources(reader *reporead.Reader, root, moduleRoot, moduleID, modulePath string, packages []goListPackage, corpusPaths []string) ([]TestSource, []string, error) {
	allowed := make(map[string]bool, len(corpusPaths))
	for _, path := range corpusPaths {
		allowed[path] = true
	}
	var result []TestSource
	var warnings []string
	seen := make(map[string]bool)
	for _, pkg := range packages {
		if pkg.DepOnly || len(pkg.TestGoFiles)+len(pkg.XTestGoFiles) == 0 {
			continue
		}
		moduleDir, _, packageDir, err := normalizePackagePaths(root, moduleRoot, pkg.Dir)
		if err != nil {
			return nil, nil, err
		}
		for _, group := range []struct {
			files    []string
			external bool
		}{{pkg.TestGoFiles, false}, {pkg.XTestGoFiles, true}} {
			for _, name := range group.files {
				path, err := entrypointSourcePath(packageDir, name)
				if err != nil {
					return nil, nil, err
				}
				if !allowed[path] {
					continue
				}
				if seen[path] {
					return nil, nil, fmt.Errorf("test source %s has more than one package owner", path)
				}
				seen[path] = true
				source := TestSource{ModuleID: moduleID, ModulePath: modulePath, ModuleDir: moduleDir,
					PackagePath: pkg.ImportPath, Path: path, External: group.external,
					Declarations: []PackageDeclaration{}, Imports: []string{}}
				content, err := reader.ReadFileAll(path)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("test source %s: declarations unavailable: %v", path, err))
					result = append(result, source)
					continue
				}
				fileSet := token.NewFileSet()
				file, err := parser.ParseFile(fileSet, path, content.Bytes, parser.SkipObjectResolution)
				if err != nil {
					// A broken test does not erase a valid application. Retain its
					// inventory row, with explicitly unavailable declarations.
					warnings = append(warnings, fmt.Sprintf("test source %s: declarations unavailable: %v", path, err))
					result = append(result, source)
					continue
				}
				source.PackageName = file.Name.Name
				source.Declarations, err = CanonicalPackageDeclarations(declarationsFromFile(fileSet, file, path))
				if err != nil {
					return nil, nil, err
				}
				for _, imported := range file.Imports {
					value, err := strconv.Unquote(imported.Path.Value)
					if err != nil {
						return nil, nil, fmt.Errorf("test source %s: import path: %w", path, err)
					}
					source.Imports = append(source.Imports, value)
				}
				sort.Strings(source.Imports)
				source.Imports = compactStrings(source.Imports)
				source.DeclarationsScanned = true
				result = append(result, source)
			}
		}
	}
	return canonicalTestSources(result), canonicalWarnings(warnings), nil
}

func canonicalTestSources(values []TestSource) []TestSource {
	result := append([]TestSource{}, values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

// CloneTestSources returns independently owned source evidence. Target
// scoping decides which component owns it; this function does not assign it.
func CloneTestSources(values []TestSource) []TestSource {
	result := append([]TestSource{}, values...)
	for i := range result {
		result[i].Declarations = append([]PackageDeclaration{}, values[i].Declarations...)
		result[i].Imports = append([]string{}, values[i].Imports...)
	}
	return result
}
