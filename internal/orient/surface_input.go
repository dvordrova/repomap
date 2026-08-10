package orient

import (
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func surfaceDiscoveryInput(
	repositoryName string,
	facts *gofacts.Facts,
	target *analysistarget.Target,
) surfacediscovery.Input {
	input := surfacediscovery.Input{RepositoryName: repositoryName}
	if target != nil {
		input.AnalysisTarget = &surfacediscovery.AnalysisTargetInput{
			Kind: string(target.Kind), PackagePath: target.PackagePath,
			Roots: make([]surfacediscovery.AnalysisTargetRootInput, 0, len(target.Roots)),
		}
		for _, root := range target.Roots {
			input.AnalysisTarget.Roots = append(input.AnalysisTarget.Roots, surfacediscovery.AnalysisTargetRootInput{
				Path: root.Path, Line: root.Line,
			})
		}
	}
	if facts == nil {
		return input
	}
	input.ModuleDirs = make([]string, 0, len(facts.Modules))
	for _, module := range facts.Modules {
		input.ModuleDirs = append(input.ModuleDirs, module.ModuleDir)
	}
	input.Packages = make([]surfacediscovery.PackageInput, 0, len(facts.Packages))
	moduleDirs := make(map[string]string, len(facts.Modules))
	for _, module := range facts.Modules {
		moduleDirs[module.ID] = module.ModuleDir
	}
	for _, pkg := range facts.Packages {
		input.Packages = append(input.Packages, surfacediscovery.PackageInput{
			Path: pkg.CanonicalPath, ModuleDir: moduleDirs[pkg.ModuleID],
		})
	}
	input.Entrypoints = make([]surfacediscovery.EntrypointInput, 0, len(facts.EntrypointPackages))
	for _, entrypoint := range facts.EntrypointPackages {
		converted := surfacediscovery.EntrypointInput{
			Package: entrypoint.ImportPath, PackageDir: entrypoint.PackageDir,
			ModuleDir: entrypoint.ModuleDir, Kind: entrypoint.Kind,
			Anchors: make([]surfacediscovery.EntrypointAnchorInput, 0, len(entrypoint.Anchors)),
		}
		for _, anchor := range entrypoint.Anchors {
			converted.Anchors = append(converted.Anchors, surfacediscovery.EntrypointAnchorInput{
				Kind: string(anchor.Kind), Path: anchor.Path, Line: anchor.Line,
			})
		}
		input.Entrypoints = append(input.Entrypoints, converted)
	}
	return input
}
