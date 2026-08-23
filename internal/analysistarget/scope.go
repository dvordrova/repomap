package analysistarget

import (
	"fmt"
	"sort"

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
	packages := make(map[string]gofacts.PackageFact, len(facts.Packages))
	packagesByIdentity := make(map[string]gofacts.PackageFact, len(facts.Packages))
	for _, pkg := range facts.Packages {
		packages[pkg.CanonicalPath] = pkg
		packagesByIdentity[packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)] = pkg
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
			retained[targetPackage.PackagePath] = struct{}{}
		}
	} else {
		_, ok := packagesByIdentity[packageIdentityKey(target.ModuleID, target.PackagePath)]
		if !ok {
			return gofacts.Facts{}, fmt.Errorf("analysis target scope: selected package %q is unavailable", target.PackagePath)
		}
		retained[target.PackagePath] = struct{}{}
		growOutgoingClosure(retained, packages, facts.InternalEdges)
	}

	scoped := gofacts.Facts{
		Packages:           []gofacts.PackageFact{},
		EntrypointPackages: []gofacts.Entrypoint{},
		InternalEdges:      []gofacts.Edge{},
		ExternalImportsTop: []gofacts.ExtImport{},
		Warnings:           append([]string{}, facts.Warnings...),
	}
	retainedModules := make(map[string]struct{})
	for _, pkg := range facts.Packages {
		if _, keep := retained[pkg.CanonicalPath]; !keep {
			continue
		}
		copyPkg := pkg
		copyPkg.Files = append([]string{}, pkg.Files...)
		copyPkg.Declarations = append([]gofacts.PackageDeclaration{}, pkg.Declarations...)
		scoped.Packages = append(scoped.Packages, copyPkg)
		retainedModules[pkg.ModuleID] = struct{}{}
	}
	for _, edge := range facts.InternalEdges {
		if _, from := retained[edge.From]; !from {
			continue
		}
		if _, to := retained[edge.To]; !to {
			continue
		}
		scoped.InternalEdges = append(scoped.InternalEdges, edge)
	}
	if facts.Dependencies != nil {
		retainedImporterRefs := make(map[string]struct{})
		for _, importer := range facts.Dependencies.Importers {
			if _, keep := retained[importer.PackagePath]; keep {
				retainedImporterRefs[importer.Ref] = struct{}{}
			}
		}
		dependencyCatalog, dependencyErr := facts.Dependencies.Subset(retainedImporterRefs)
		if dependencyErr != nil {
			return gofacts.Facts{}, fmt.Errorf("analysis target scope: dependency catalog: %w", dependencyErr)
		}
		scoped.Dependencies = &dependencyCatalog
	}

	for _, entrypoint := range facts.EntrypointPackages {
		if target.Kind != KindExecutablePackage || entrypoint.ImportPath != target.PackagePath {
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

func growOutgoingClosure(retained map[string]struct{}, packages map[string]gofacts.PackageFact, edges []gofacts.Edge) {
	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			if _, from := retained[edge.From]; !from {
				continue
			}
			if _, local := packages[edge.To]; !local {
				continue
			}
			if _, exists := retained[edge.To]; exists {
				continue
			}
			retained[edge.To] = struct{}{}
			changed = true
		}
	}
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
