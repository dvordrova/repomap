package main

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestAnalysisTargetScopeMatchBindsCompleteExecutableAndModuleLibraryAuthority(t *testing.T) {
	catalog := targetPortfolioRuntimeCatalog(t, targetPortfolioRuntimeFacts())
	for _, entry := range catalog.Entries {
		target := entry.Candidate.Target
		if target.Kind != analysistarget.KindExecutablePackage &&
			target.Kind != analysistarget.KindModuleLibrary {
			continue
		}
		t.Run(string(target.Kind)+"/"+entry.DisplayPath, func(t *testing.T) {
			targetPackage := ""
			if target.Kind == analysistarget.KindExecutablePackage {
				targetPackage = target.PackagePath
			}
			scope := surfacediscovery.DirectCallIndexScope{
				TargetRef: target.Ref, TargetKind: analysisTargetSurfaceKind(target),
				TargetModuleID: target.ModuleID, TargetModulePath: target.ModulePath,
				TargetModuleDir: target.ModuleDir, TargetPackage: targetPackage,
				TargetPackages: analysisTargetRootPackagePaths(target),
				MaxDepth:       7, EdgeLimit: 123,
			}
			if !analysisTargetScopeMatches(target, scope, 7, 123) {
				t.Fatalf("exact scope did not match target: %#v / %#v", target, scope)
			}

			mutations := []func(*surfacediscovery.DirectCallIndexScope){
				func(value *surfacediscovery.DirectCallIndexScope) { value.TargetRef += "-drift" },
				func(value *surfacediscovery.DirectCallIndexScope) { value.TargetKind += "-drift" },
				func(value *surfacediscovery.DirectCallIndexScope) { value.TargetModuleID += "-drift" },
				func(value *surfacediscovery.DirectCallIndexScope) { value.TargetModulePath += "/drift" },
				func(value *surfacediscovery.DirectCallIndexScope) { value.TargetModuleDir += "/drift" },
				func(value *surfacediscovery.DirectCallIndexScope) { value.TargetPackage += "/drift" },
				func(value *surfacediscovery.DirectCallIndexScope) {
					value.TargetPackages = append(slices.Clone(value.TargetPackages), "example.com/drift")
				},
				func(value *surfacediscovery.DirectCallIndexScope) { value.MaxDepth++ },
				func(value *surfacediscovery.DirectCallIndexScope) { value.EdgeLimit++ },
			}
			for index, mutate := range mutations {
				drifted := scope
				drifted.TargetPackages = slices.Clone(scope.TargetPackages)
				mutate(&drifted)
				if analysisTargetScopeMatches(target, drifted, 7, 123) {
					t.Fatalf("accepted scope drift %d: %#v", index, drifted)
				}
			}
		})
	}
}
