package analysistarget

import (
	"slices"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// MatchesDirectCallIndexScope reports whether scope is bound to every exact
// target identity field and to the requested traversal bounds.
func (target Target) MatchesDirectCallIndexScope(
	scope surfacediscovery.DirectCallIndexScope,
	maxDepth int,
	edgeLimit int,
) bool {
	return target.matchesDirectCallIndexTargetScope(scope) &&
		scope.MaxDepth == maxDepth && scope.EdgeLimit == edgeLimit
}

func (target Target) matchesDirectCallIndexTargetScope(
	scope surfacediscovery.DirectCallIndexScope,
) bool {
	targetPackage := ""
	if target.Kind == KindExecutablePackage {
		targetPackage = target.PackagePath
	}
	return scope.TargetScoped() &&
		scope.TargetRef == target.Ref &&
		scope.TargetKind == directCallIndexTargetKind(target) &&
		scope.TargetModuleID == target.ModuleID &&
		scope.TargetModulePath == target.ModulePath &&
		scope.TargetModuleDir == target.ModuleDir &&
		scope.TargetPackage == targetPackage &&
		slices.Equal(scope.TargetPackages, directCallIndexTargetPackagePaths(target))
}

func directCallIndexTargetKind(target Target) string {
	if target.Kind == KindModuleLibrary {
		return surfacediscovery.AnalysisTargetModuleLibrary
	}
	return surfacediscovery.AnalysisTargetExecutablePackage
}

func directCallIndexTargetPackagePaths(target Target) []string {
	packages := target.RootPackages()
	result := make([]string, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, pkg.PackagePath)
	}
	return result
}
