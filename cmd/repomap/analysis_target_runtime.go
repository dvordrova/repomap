package main

import (
	"slices"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// analysisTargetSurfaceKind translates the retained one-package library
// reader into D280's module-library runtime shape. Fresh target catalogs emit
// only executable_package and module_library.
func analysisTargetSurfaceKind(target analysistarget.Target) string {
	if target.Kind == analysistarget.KindModuleLibrary || target.Kind == analysistarget.KindLibraryPackage {
		return surfacediscovery.AnalysisTargetModuleLibrary
	}
	return surfacediscovery.AnalysisTargetExecutablePackage
}

func analysisTargetRootPackagePaths(target analysistarget.Target) []string {
	packages := target.RootPackages()
	result := make([]string, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, pkg.PackagePath)
	}
	return result
}

func analysisTargetScopeMatches(
	target analysistarget.Target,
	scope surfacediscovery.DirectCallIndexScope,
	depth int,
	edgeLimit int,
) bool {
	targetPackage := ""
	if target.Kind == analysistarget.KindExecutablePackage {
		targetPackage = target.PackagePath
	}
	return scope.TargetScoped() &&
		scope.TargetRef == target.Ref &&
		scope.TargetKind == analysisTargetSurfaceKind(target) &&
		scope.TargetModuleID == target.ModuleID &&
		scope.TargetModulePath == target.ModulePath &&
		scope.TargetModuleDir == target.ModuleDir &&
		scope.TargetPackage == targetPackage &&
		slices.Equal(scope.TargetPackages, analysisTargetRootPackagePaths(target)) &&
		scope.MaxDepth == depth && scope.EdgeLimit == edgeLimit
}

func analysisTargetSubject(target analysistarget.Target) string {
	if target.Kind == analysistarget.KindModuleLibrary {
		return target.ModulePath + " library API"
	}
	return target.PackagePath
}
