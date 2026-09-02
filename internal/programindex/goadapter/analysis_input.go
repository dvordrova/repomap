package goadapter

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// AnalysisInput binds one already-scoped Go fact set to the exact typed target
// consumed by the Go packages/types/SSA producer. It performs no I/O.
func AnalysisInput(
	facts *gofacts.Facts,
	target *analysistarget.Target,
) (surfacediscovery.Input, error) {
	if facts == nil || target == nil {
		return surfacediscovery.Input{}, fmt.Errorf("Go program index adapter: scoped facts and target are required")
	}
	targetKind := string(target.Kind)
	targetPackages := []string{target.PackagePath}
	packagePath := target.PackagePath
	if target.Kind == analysistarget.KindModuleLibrary {
		targetKind = surfacediscovery.AnalysisTargetModuleLibrary
		packagePath = ""
		targetPackages = make([]string, 0, len(target.LibraryPackages))
		for _, pkg := range target.LibraryPackages {
			targetPackages = append(targetPackages, pkg.PackagePath)
		}
	}
	input := surfacediscovery.Input{
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: targetKind,
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			PackagePath: packagePath, TargetPackages: targetPackages,
			Roots: make([]surfacediscovery.AnalysisTargetRootInput, 0, len(target.Roots)),
		},
		ModuleDirs: make([]string, 0, len(facts.Modules)),
		Packages:   make([]surfacediscovery.PackageInput, 0, len(facts.Packages)),
	}
	for _, root := range target.Roots {
		input.AnalysisTarget.Roots = append(input.AnalysisTarget.Roots, surfacediscovery.AnalysisTargetRootInput{
			Path: root.Path, Line: root.Line,
		})
	}
	moduleDirs := make(map[string]string, len(facts.Modules))
	for _, module := range facts.Modules {
		input.ModuleDirs = append(input.ModuleDirs, module.ModuleDir)
		moduleDirs[module.ID] = module.ModuleDir
	}
	for _, pkg := range facts.Packages {
		input.Packages = append(input.Packages, surfacediscovery.PackageInput{
			Path: pkg.CanonicalPath, ModuleDir: moduleDirs[pkg.ModuleID],
		})
	}
	return input, nil
}
