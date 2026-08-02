package orient

import (
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func surfaceDiscoveryInput(repositoryName string, facts *gofacts.Facts) surfacediscovery.Input {
	input := surfacediscovery.Input{RepositoryName: repositoryName}
	if facts == nil {
		return input
	}
	input.ModuleDirs = make([]string, 0, len(facts.Modules))
	for _, module := range facts.Modules {
		input.ModuleDirs = append(input.ModuleDirs, module.ModuleDir)
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
