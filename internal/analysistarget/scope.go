package analysistarget

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/gofacts"
)

// ScopeGoFacts projects exact repository facts to one selected product before
// any downstream semantic or cardinality budget. Executables retain their
// repository-local outgoing import closure. A root library retains its owning
// module; an explicitly selected library subpackage retains its outgoing import
// closure. The input is never mutated.
func ScopeGoFacts(facts gofacts.Facts, target Target) (gofacts.Facts, error) {
	if err := target.Validate(); err != nil {
		return gofacts.Facts{}, fmt.Errorf("analysis target scope: %w", err)
	}
	packages := make(map[string]gofacts.PackageFact, len(facts.Packages))
	for _, pkg := range facts.Packages {
		packages[pkg.CanonicalPath] = pkg
	}
	selected, ok := packages[target.PackagePath]
	if !ok || selected.ModuleID != target.ModuleID {
		return gofacts.Facts{}, fmt.Errorf("analysis target scope: selected package %q is unavailable", target.PackagePath)
	}

	retained := map[string]struct{}{target.PackagePath: {}}
	if target.Kind == KindLibraryPackage && target.PackageDir == "." {
		for _, pkg := range facts.Packages {
			if pkg.ModuleID == target.ModuleID {
				retained[pkg.CanonicalPath] = struct{}{}
			}
		}
	} else {
		growOutgoingClosure(retained, packages, facts.InternalEdges)
	}

	scoped := gofacts.Facts{
		Packages:              []gofacts.PackageFact{},
		EntrypointPackages:    []gofacts.Entrypoint{},
		CommandTraces:         []gofacts.CommandTrace{},
		ModuleSummaries:       []gofacts.ModuleSummary{},
		OrientationCandidates: []gofacts.OrientationCandidate{},
		InternalEdges:         []gofacts.Edge{},
		ExternalImportsTop:    []gofacts.ExtImport{},
		Warnings:              append([]string{}, facts.Warnings...),
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

	for _, entrypoint := range facts.EntrypointPackages {
		if target.Kind != KindExecutablePackage || entrypoint.ImportPath != target.PackagePath {
			continue
		}
		copyEntrypoint := entrypoint
		copyEntrypoint.GoFiles = append([]string{}, entrypoint.GoFiles...)
		copyEntrypoint.Anchors = append([]gofacts.EntrypointAnchor{}, entrypoint.Anchors...)
		scoped.EntrypointPackages = append(scoped.EntrypointPackages, copyEntrypoint)
	}
	for _, trace := range facts.CommandTraces {
		if target.Kind == KindExecutablePackage && trace.EntrypointPackage == target.PackagePath {
			scoped.CommandTraces = append(scoped.CommandTraces, trace)
		}
	}
	for _, candidate := range facts.OrientationCandidates {
		if target.Kind == KindExecutablePackage && candidate.EntrypointPackage == target.PackagePath {
			copyCandidate := candidate
			copyCandidate.OpenFiles = append([]string{}, candidate.OpenFiles...)
			scoped.OrientationCandidates = append(scoped.OrientationCandidates, copyCandidate)
		}
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
	for _, summary := range facts.ModuleSummaries {
		moduleID := moduleIDForSummary(facts.Modules, summary)
		if _, keep := retainedModules[moduleID]; !keep {
			continue
		}
		copySummary := summary
		copySummary.PackagesCount = packagesByModule[moduleID]
		copySummary.EntrypointsCount = len(entrypointsByModule[moduleID])
		copySummary.TopImportedInternalPkgs = filterPackagePaths(summary.TopImportedInternalPkgs, retained)
		copySummary.TopExternalImports = []gofacts.ExtImport{}
		scoped.ModuleSummaries = append(scoped.ModuleSummaries, copySummary)
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

func moduleIDForSummary(modules []gofacts.ModuleFact, summary gofacts.ModuleSummary) string {
	for _, module := range modules {
		if module.ModulePath == summary.ModulePath && canonicalDirForMatch(module.ModuleDir) == canonicalDirForMatch(summary.ModuleDir) {
			return module.ID
		}
	}
	return ""
}

func filterPackagePaths(values []string, retained map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, keep := retained[value]; keep {
			result = append(result, value)
		}
	}
	return result
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
