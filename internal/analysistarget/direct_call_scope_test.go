package analysistarget

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestTargetMatchesDirectCallIndexScopeBindsCompleteTargetAndBounds(t *testing.T) {
	executable := requireCandidateTarget(t, syntheticFacts(
		"module-root", "example.com/product",
		[]syntheticPackage{{
			path: "example.com/product/cmd/app", dir: "cmd/app", executable: true, line: 7,
		}},
	), KindExecutablePackage, "cmd/app")
	library := requireCandidateTarget(t, syntheticFacts(
		"module-root", "example.com/product",
		[]syntheticPackage{
			{path: "example.com/product", dir: "."},
			{path: "example.com/product/client", dir: "client"},
		},
	), KindModuleLibrary, "")

	for _, target := range []Target{executable, library} {
		t.Run(string(target.Kind), func(t *testing.T) {
			scope := surfacediscovery.DirectCallIndexScope{
				TargetRef: target.Ref, TargetKind: string(target.Kind),
				TargetModuleID: target.ModuleID, TargetModulePath: target.ModulePath,
				TargetModuleDir: target.ModuleDir, TargetPackage: target.PackagePath,
				TargetPackages: directCallIndexTargetPackagePaths(target),
				MaxDepth:       7, EdgeLimit: 123,
			}
			if !target.MatchesDirectCallIndexScope(scope, 7, 123) {
				t.Fatalf("exact scope did not match target: %#v / %#v", target, scope)
			}

			mutations := []struct {
				name   string
				mutate func(*surfacediscovery.DirectCallIndexScope)
			}{
				{"target ref", func(value *surfacediscovery.DirectCallIndexScope) { value.TargetRef += "-drift" }},
				{"target kind", func(value *surfacediscovery.DirectCallIndexScope) { value.TargetKind += "-drift" }},
				{"module ID", func(value *surfacediscovery.DirectCallIndexScope) { value.TargetModuleID += "-drift" }},
				{"module path", func(value *surfacediscovery.DirectCallIndexScope) { value.TargetModulePath += "/drift" }},
				{"module directory", func(value *surfacediscovery.DirectCallIndexScope) { value.TargetModuleDir += "/drift" }},
				{"package", func(value *surfacediscovery.DirectCallIndexScope) { value.TargetPackage += "/drift" }},
				{"target packages", func(value *surfacediscovery.DirectCallIndexScope) {
					value.TargetPackages = append(value.TargetPackages, "example.com/drift")
				}},
				{"maximum depth", func(value *surfacediscovery.DirectCallIndexScope) { value.MaxDepth++ }},
				{"edge limit", func(value *surfacediscovery.DirectCallIndexScope) { value.EdgeLimit++ }},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					drifted := scope
					drifted.TargetPackages = slices.Clone(scope.TargetPackages)
					mutation.mutate(&drifted)
					if target.MatchesDirectCallIndexScope(drifted, 7, 123) {
						t.Fatalf("accepted scope drift: %#v", drifted)
					}
				})
			}
		})
	}
}
