package goadapter

import (
	"fmt"
	"path"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/programindex"
)

// ValidateTargetBinding proves that the Go-specific analysis target and the
// language-neutral ProgramTarget describe the same exact selected scope. It
// uses only sealed structural fields; display order and model output are not
// authority.
func ValidateTargetBinding(analysis analysistarget.Target, target programindex.Target) error {
	if err := analysis.Validate(); err != nil {
		return fmt.Errorf("Go program target binding: analysis target: %w", err)
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("Go program target binding: program target: %w", err)
	}
	if target.Language != "go" {
		return fmt.Errorf("Go program target binding: expected Go target, got %q", target.Language)
	}

	sourcePathByRef := make(map[string]string, len(target.Sources))
	for _, source := range target.Sources {
		sourcePathByRef[source.FileRef] = source.Path
	}
	anchorPath, ok := sourcePathByRef[target.AnchorFileRef]
	if !ok {
		return fmt.Errorf("Go program target binding: anchor source is absent")
	}

	switch analysis.Kind {
	case analysistarget.KindExecutablePackage:
		if target.Kind != "executable" || target.Name != analysis.PackagePath || target.Selector != analysis.PackagePath {
			return fmt.Errorf("Go program target binding: executable identity mismatch")
		}
		rootPaths := make(map[string]struct{}, len(analysis.Roots))
		rootLocations := make(map[string]struct{}, len(analysis.Roots))
		for _, root := range analysis.Roots {
			rootPaths[root.Path] = struct{}{}
			rootLocations[programLocationKey(root.Path, root.Line)] = struct{}{}
		}
		if len(target.Sources) != len(rootPaths) {
			return fmt.Errorf("Go program target binding: executable source boundary mismatch")
		}
		for _, source := range target.Sources {
			if _, exact := rootPaths[source.Path]; !exact {
				return fmt.Errorf("Go program target binding: executable source %q is not an exact root", source.Path)
			}
		}
		if _, exact := rootPaths[anchorPath]; !exact {
			return fmt.Errorf("Go program target binding: executable anchor is not an exact root")
		}
		if len(target.Seeds) != len(rootLocations) {
			return fmt.Errorf("Go program target binding: executable seed boundary mismatch")
		}
		seenSeeds := make(map[string]struct{}, len(target.Seeds))
		for _, seed := range target.Seeds {
			if seed.Kind != programindex.SeedCallable || seed.Location == nil || seed.Location.Column != 1 {
				return fmt.Errorf("Go program target binding: executable seed is not an exact Go callable root")
			}
			key := programLocationKey(seed.Location.Path, seed.Location.Line)
			if _, exact := rootLocations[key]; !exact {
				return fmt.Errorf("Go program target binding: executable seed is outside exact roots")
			}
			if _, duplicate := seenSeeds[key]; duplicate {
				return fmt.Errorf("Go program target binding: duplicate executable seed")
			}
			seenSeeds[key] = struct{}{}
		}

	case analysistarget.KindModuleLibrary:
		if target.Kind != "library" || target.Name != analysis.ModulePath || target.Selector != analysis.ModulePath {
			return fmt.Errorf("Go program target binding: module-library identity mismatch")
		}
		if len(target.Seeds) != 0 {
			return fmt.Errorf("Go program target binding: module library cannot contain launch seeds")
		}
		manifestPath := "go.mod"
		if analysis.ModuleDir != "." {
			manifestPath = path.Join(analysis.ModuleDir, "go.mod")
		}
		if anchorPath != manifestPath || len(target.Sources) != len(analysis.LibraryPackages)+1 {
			return fmt.Errorf("Go program target binding: module-library source boundary mismatch")
		}
		packageDirs := make(map[string]struct{}, len(analysis.LibraryPackages))
		for _, pkg := range analysis.LibraryPackages {
			packageDirs[pkg.PackageDir] = struct{}{}
		}
		seenPackageDirs := make(map[string]struct{}, len(packageDirs))
		for _, source := range target.Sources {
			if source.Path == manifestPath {
				continue
			}
			directory := path.Dir(source.Path)
			if _, exact := packageDirs[directory]; !exact {
				return fmt.Errorf("Go program target binding: library source %q is outside exact public package roots", source.Path)
			}
			if _, duplicate := seenPackageDirs[directory]; duplicate {
				return fmt.Errorf("Go program target binding: library package root %q has multiple representative sources", directory)
			}
			seenPackageDirs[directory] = struct{}{}
		}
		if len(seenPackageDirs) != len(packageDirs) {
			return fmt.Errorf("Go program target binding: library package source is missing")
		}

	default:
		return fmt.Errorf("Go program target binding: unsupported analysis target kind %q", analysis.Kind)
	}
	return nil
}

func programLocationKey(sourcePath string, line int) string {
	return fmt.Sprintf("%s\x00%d", sourcePath, line)
}
