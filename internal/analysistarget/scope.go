package analysistarget

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gofacts"
)

// ScopeGoFacts projects exact repository facts to one selected product before
// any downstream semantic or cardinality budget. Executables retain their
// repository-local outgoing import closure. A module library retains exactly
// its sealed owning-module non-main package inventory, including supporting
// internal packages; main packages are separate executable products. The
// input is never mutated.
func ScopeGoFacts(facts gofacts.Facts, target Target) (gofacts.Facts, error) {
	if err := target.Validate(); err != nil {
		return gofacts.Facts{}, fmt.Errorf("analysis target scope: %w", err)
	}
	packagesByIdentity := make(map[string]gofacts.PackageFact, len(facts.Packages))
	for _, pkg := range facts.Packages {
		packagesByIdentity[packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)] = pkg
	}
	exactDependencies, err := newExactDependencyScope(facts)
	if err != nil {
		return gofacts.Facts{}, fmt.Errorf("analysis target scope: dependency catalog: %w", err)
	}

	retained := make(map[string]struct{})
	if target.Kind == KindModuleLibrary {
		current := make([]TargetPackage, 0, len(target.ModulePackages))
		for _, pkg := range facts.Packages {
			if pkg.ModuleID != target.ModuleID || pkg.Name == "main" {
				continue
			}
			current = append(current, TargetPackage{PackagePath: pkg.CanonicalPath, PackageDir: pkg.PackageDir})
		}
		current = canonicalTargetPackages(current)
		if !sameTargetPackages(current, target.ModulePackages) {
			return gofacts.Facts{}, fmt.Errorf("analysis target scope: module package inventory mismatch")
		}
		for _, targetPackage := range target.ModulePackages {
			pkg, ok := packagesByIdentity[packageIdentityKey(target.ModuleID, targetPackage.PackagePath)]
			if !ok || pkg.Name == "main" || pkg.PackageDir != targetPackage.PackageDir {
				return gofacts.Facts{}, fmt.Errorf("analysis target scope: module package %q is unavailable", targetPackage.PackagePath)
			}
			retained[packageFactIdentity(pkg)] = struct{}{}
		}
	} else {
		targetPackage, ok := packagesByIdentity[packageIdentityKey(target.ModuleID, target.PackagePath)]
		if !ok {
			return gofacts.Facts{}, fmt.Errorf("analysis target scope: selected package %q is unavailable", target.PackagePath)
		}
		retained[packageFactIdentity(targetPackage)] = struct{}{}
		if exactDependencies != nil {
			if err := exactDependencies.growOutgoingClosure(retained, *facts.Dependencies); err != nil {
				return gofacts.Facts{}, fmt.Errorf("analysis target scope: dependency catalog: %w", err)
			}
		} else {
			growLegacyOutgoingClosure(retained, facts.Packages, facts.InternalEdges, target.PackagePath)
		}
	}

	scoped := gofacts.Facts{
		Packages:           []gofacts.PackageFact{},
		PackageOrigins:     append([]gofacts.PackageOrigin(nil), facts.PackageOrigins...),
		EntrypointPackages: []gofacts.Entrypoint{},
		InternalEdges:      []gofacts.Edge{},
		ExternalImportsTop: []gofacts.ExtImport{},
		Warnings:           append([]string{}, facts.Warnings...),
	}
	retainedModules := make(map[string]struct{})
	for _, pkg := range facts.Packages {
		if _, keep := retained[packageFactIdentity(pkg)]; !keep {
			continue
		}
		copyPkg := pkg
		copyPkg.Files = append([]string{}, pkg.Files...)
		copyPkg.Declarations = append([]gofacts.PackageDeclaration{}, pkg.Declarations...)
		scoped.Packages = append(scoped.Packages, copyPkg)
		retainedModules[pkg.ModuleID] = struct{}{}
	}
	if facts.Dependencies != nil {
		retainedImporterRefs := make(map[string]struct{})
		for importerRef, pkg := range exactDependencies.importerPackages {
			if _, keep := retained[pkg.identity]; keep {
				retainedImporterRefs[importerRef] = struct{}{}
			}
		}
		dependencyCatalog, dependencyErr := facts.Dependencies.Subset(retainedImporterRefs)
		if dependencyErr != nil {
			return gofacts.Facts{}, fmt.Errorf("analysis target scope: dependency catalog: %w", dependencyErr)
		}
		scoped.Dependencies = &dependencyCatalog
		scoped.InternalEdges = exactDependencies.internalEdges(dependencyCatalog, retained)
	} else {
		retainedPaths := retainedPackagePaths(facts.Packages, retained)
		for _, edge := range facts.InternalEdges {
			if _, from := retainedPaths[edge.From]; !from {
				continue
			}
			if _, to := retainedPaths[edge.To]; !to {
				continue
			}
			scoped.InternalEdges = append(scoped.InternalEdges, edge)
		}
	}

	for _, entrypoint := range facts.EntrypointPackages {
		if target.Kind != KindExecutablePackage || entrypoint.ImportPath != target.PackagePath ||
			entrypoint.ModulePath != target.ModulePath || entrypoint.ModuleDir != target.ModuleDir {
			continue
		}
		copyEntrypoint := entrypoint
		copyEntrypoint.GoFiles = append([]string{}, entrypoint.GoFiles...)
		copyEntrypoint.Anchors = append([]gofacts.EntrypointAnchor{}, entrypoint.Anchors...)
		scoped.EntrypointPackages = append(scoped.EntrypointPackages, copyEntrypoint)
	}
	entrypointsByModule := make(map[string][]gofacts.Entrypoint)
	for _, entrypoint := range scoped.EntrypointPackages {
		entrypointsByModule[target.ModuleID] = append(entrypointsByModule[target.ModuleID], entrypoint)
	}
	packagesByModule := make(map[string]int)
	for _, pkg := range scoped.Packages {
		packagesByModule[pkg.ModuleID]++
	}
	for _, module := range facts.Modules {
		if _, keep := retainedModules[module.ID]; !keep {
			continue
		}
		copyModule := module
		copyModule.EntrypointPackages = append([]gofacts.Entrypoint{}, entrypointsByModule[module.ID]...)
		copyModule.PackagesCount = packagesByModule[module.ID]
		copyModule.RetainedPackagesCount = packagesByModule[module.ID]
		copyModule.Coverage.PackagesDiscovered = packagesByModule[module.ID]
		copyModule.Coverage.PackagesRetained = packagesByModule[module.ID]
		copyModule.Warnings = append([]string{}, module.Warnings...)
		scoped.Modules = append(scoped.Modules, copyModule)
	}
	sortFacts(&scoped)
	scoped.PackagesCount = len(scoped.Packages)
	scoped.RetainedPackagesCount = len(scoped.Packages)
	scoped.Coverage = facts.Coverage
	scoped.Coverage.ModulesDiscovered = len(scoped.Modules)
	scoped.Coverage.ModulesAvailable = len(scoped.Modules)
	scoped.Coverage.ModulesUnavailable = 0
	scoped.Coverage.PackagesDiscovered = len(scoped.Packages)
	scoped.Coverage.PackagesRetained = len(scoped.Packages)
	scoped.Coverage.EdgesDiscovered = len(scoped.InternalEdges)
	scoped.Coverage.EdgesRetained = len(scoped.InternalEdges)
	scoped.Warnings = append(scoped.Warnings, fmt.Sprintf(
		"analysis target %s retained %d package(s)", target.Ref, len(scoped.Packages),
	))
	return scoped, nil
}

func sameTargetPackages(left, right []TargetPackage) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type exactDependencyScope struct {
	importerPackages  map[string]exactDependencyPackage
	workspacePackages map[string]exactDependencyPackage
}

type exactDependencyPackage struct {
	identity    string
	packagePath string
}

type exactDependencyPackageKey struct {
	modulePath     string
	packagePath    string
	repositoryPath string
}

func newExactDependencyScope(facts gofacts.Facts) (*exactDependencyScope, error) {
	if facts.Dependencies == nil {
		return nil, nil
	}
	if err := facts.Dependencies.Validate(); err != nil {
		return nil, err
	}
	packageIdentities := make(map[exactDependencyPackageKey]exactDependencyPackage, len(facts.Packages))
	for _, pkg := range facts.Packages {
		key := dependencyPackageKey(pkg.ModulePath, pkg.CanonicalPath, pkg.PackageDir)
		value := exactDependencyPackage{identity: packageFactIdentity(pkg), packagePath: pkg.CanonicalPath}
		if previous, exists := packageIdentities[key]; exists && previous.identity != value.identity {
			return nil, fmt.Errorf("exact repository package identity %q is ambiguous", pkg.CanonicalPath)
		}
		packageIdentities[key] = value
	}
	scope := &exactDependencyScope{
		importerPackages:  make(map[string]exactDependencyPackage, len(facts.Dependencies.Importers)),
		workspacePackages: make(map[string]exactDependencyPackage),
	}
	for _, importer := range facts.Dependencies.Importers {
		pkg, ok := packageIdentities[dependencyPackageKey(
			importer.ModulePath, importer.PackagePath, importer.RepositoryPath,
		)]
		if !ok {
			// The dependency catalog may retain exact raw non-DepOnly importer
			// authority for a row that the build-selected package inventory
			// deliberately excludes. It cannot admit a package into this target.
			continue
		}
		scope.importerPackages[importer.Ref] = pkg
	}
	for _, dependency := range facts.Dependencies.Dependencies {
		if dependency.Kind != dependencies.KindWorkspace {
			continue
		}
		pkg, ok := packageIdentities[dependencyPackageKey(
			dependency.ModulePath, dependency.PackagePath, dependency.RepositoryPath,
		)]
		if !ok {
			// Unselected raw package rows may likewise appear as workspace
			// dependency metadata. A retained executable importer that reaches
			// one is rejected below; unrelated targets remain analyzable.
			continue
		}
		scope.workspacePackages[dependency.ID] = pkg
	}
	return scope, nil
}

func (scope *exactDependencyScope) growOutgoingClosure(
	retained map[string]struct{},
	catalog dependencies.Catalog,
) error {
	changed := true
	for changed {
		changed = false
		for _, dependency := range catalog.Dependencies {
			if dependency.Kind != dependencies.KindWorkspace {
				continue
			}
			to, targetAvailable := scope.workspacePackages[dependency.ID]
			for _, importerRef := range dependency.ImporterRefs {
				from, importerAvailable := scope.importerPackages[importerRef]
				if !importerAvailable {
					continue
				}
				if _, admitted := retained[from.identity]; !admitted {
					continue
				}
				if !targetAvailable {
					return fmt.Errorf(
						"retained importer %q reaches workspace dependency %q without an exact build-selected package",
						importerRef, dependency.ID,
					)
				}
				if _, exists := retained[to.identity]; exists {
					continue
				}
				retained[to.identity] = struct{}{}
				changed = true
			}
		}
	}
	return nil
}

func (scope *exactDependencyScope) internalEdges(
	catalog dependencies.Catalog,
	retained map[string]struct{},
) []gofacts.Edge {
	edges := make([]gofacts.Edge, 0)
	seen := make(map[gofacts.Edge]struct{})
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind != dependencies.KindWorkspace {
			continue
		}
		to, targetAvailable := scope.workspacePackages[dependency.ID]
		if !targetAvailable {
			continue
		}
		if _, admitted := retained[to.identity]; !admitted {
			continue
		}
		for _, importerRef := range dependency.ImporterRefs {
			from, importerAvailable := scope.importerPackages[importerRef]
			if !importerAvailable {
				continue
			}
			if _, admitted := retained[from.identity]; !admitted {
				continue
			}
			edge := gofacts.Edge{
				From: from.packagePath,
				To:   to.packagePath,
			}
			if _, duplicate := seen[edge]; duplicate {
				continue
			}
			seen[edge] = struct{}{}
			edges = append(edges, edge)
		}
	}
	return edges
}

func growLegacyOutgoingClosure(
	retained map[string]struct{},
	packageFacts []gofacts.PackageFact,
	edges []gofacts.Edge,
	targetPackagePath string,
) {
	packages := make(map[string]gofacts.PackageFact, len(packageFacts))
	for _, pkg := range packageFacts {
		packages[pkg.CanonicalPath] = pkg
	}
	retainedPaths := map[string]struct{}{targetPackagePath: {}}
	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			if _, from := retainedPaths[edge.From]; !from {
				continue
			}
			if _, local := packages[edge.To]; !local {
				continue
			}
			if _, exists := retainedPaths[edge.To]; exists {
				continue
			}
			retainedPaths[edge.To] = struct{}{}
			changed = true
		}
	}
	for _, pkg := range packageFacts {
		if _, keep := retainedPaths[pkg.CanonicalPath]; keep {
			retained[packageFactIdentity(pkg)] = struct{}{}
		}
	}
}

func retainedPackagePaths(
	packages []gofacts.PackageFact,
	retained map[string]struct{},
) map[string]struct{} {
	result := make(map[string]struct{}, len(retained))
	for _, pkg := range packages {
		if _, keep := retained[packageFactIdentity(pkg)]; keep {
			result[pkg.CanonicalPath] = struct{}{}
		}
	}
	return result
}

func dependencyPackageKey(modulePath, packagePath, repositoryPath string) exactDependencyPackageKey {
	return exactDependencyPackageKey{
		modulePath: modulePath, packagePath: packagePath, repositoryPath: repositoryPath,
	}
}

func packageFactIdentity(pkg gofacts.PackageFact) string {
	return packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)
}

func sortFacts(facts *gofacts.Facts) {
	sort.Slice(facts.Modules, func(i, j int) bool {
		if facts.Modules[i].ModuleDir != facts.Modules[j].ModuleDir {
			return facts.Modules[i].ModuleDir < facts.Modules[j].ModuleDir
		}
		return facts.Modules[i].ModulePath < facts.Modules[j].ModulePath
	})
	sort.Slice(facts.Packages, func(i, j int) bool {
		return facts.Packages[i].CanonicalPath < facts.Packages[j].CanonicalPath
	})
	sort.Slice(facts.EntrypointPackages, func(i, j int) bool {
		return facts.EntrypointPackages[i].ImportPath < facts.EntrypointPackages[j].ImportPath
	})
	sort.Slice(facts.InternalEdges, func(i, j int) bool {
		if facts.InternalEdges[i].From != facts.InternalEdges[j].From {
			return facts.InternalEdges[i].From < facts.InternalEdges[j].From
		}
		return facts.InternalEdges[i].To < facts.InternalEdges[j].To
	})
}
