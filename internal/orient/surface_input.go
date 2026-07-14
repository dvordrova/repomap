package orient

import (
	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func surfaceDiscoveryInput(repositoryName string, facts *gofacts.Facts) surfacediscovery.Input {
	input := surfacediscovery.Input{RepositoryName: repositoryName}
	if facts == nil {
		return input
	}
	input.Entrypoints = make([]surfacediscovery.EntrypointInput, 0, len(facts.EntrypointPackages))
	for _, entrypoint := range facts.EntrypointPackages {
		converted := surfacediscovery.EntrypointInput{
			Package: entrypoint.ImportPath, PackageDir: entrypoint.PackageDir, Kind: entrypoint.Kind,
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
