package surfacediscovery

import (
	"sort"

	"github.com/dvordrova/repomap/internal/entrycall"
	"golang.org/x/tools/go/ssa"
)

// entryCallSidecar retains only the repository-local adjacency needed to bind
// generic syntax candidates to exact process roots. It observes the existing
// SSA pass and never owns a program, package load, or second call graph.
type entryCallSidecar struct {
	repositoryCalls   map[*ssa.Function]map[*ssa.Function]struct{}
	surfaceCandidates map[string]rawEntrySurfaceCandidate
	surfaceCoverage   entrycall.Coverage
}

func newEntryCallSidecar() *entryCallSidecar {
	return &entryCallSidecar{
		repositoryCalls:   make(map[*ssa.Function]map[*ssa.Function]struct{}),
		surfaceCandidates: make(map[string]rawEntrySurfaceCandidate),
	}
}

func (builder *directCallIndexBuilder) enableEntryCallSidecar() {
	if builder != nil && builder.entryCalls == nil {
		builder.entryCalls = newEntryCallSidecar()
	}
}

func (builder *directCallIndexBuilder) entryCallSubstrate(
	a *analyzer,
	index DirectCallIndex,
	entrypoints []*ssa.Function,
) entrycall.Substrate {
	if builder == nil || builder.entryCalls == nil {
		return entrycall.Unavailable(entrycall.ClosedSSAUnavailable)
	}
	if index.State != DirectCallIndexReady {
		return entrycall.Unavailable(entrycall.ClosedIndexLimit)
	}
	roots := entrySurfaceProcessRoots(a, entrypoints)
	if len(roots) == 0 {
		return entrycall.Unavailable(entrycall.ClosedNoEntrypoints)
	}
	substrate := entrycall.Substrate{
		Version: entrycall.SubstrateVersion, State: entrycall.StateReady,
		Roots: []entrycall.ExactRoot{}, SurfaceCandidates: []entrycall.ExactSurfaceCandidate{},
	}
	substrate.SurfaceCandidates, substrate.Coverage =
		builder.entryCalls.projectSurfaceCandidates(a, builder, roots)
	for _, root := range roots {
		if nodeID, ok := builder.functionNode[root]; ok && nodeID != "" {
			substrate.Roots = append(substrate.Roots, entrycall.ExactRoot{NodeID: nodeID})
		}
	}
	if len(substrate.Roots) == 0 {
		return entrycall.Unavailable(entrycall.ClosedNoEntrypoints)
	}
	sort.Slice(substrate.Roots, func(i, j int) bool { return substrate.Roots[i].NodeID < substrate.Roots[j].NodeID })
	substrate.Coverage.RootsConsidered = len(substrate.Roots)
	return substrate
}

func entryCallLocation(location Location) entrycall.Location {
	return entrycall.Location{Path: location.Path, Line: location.Line, Column: location.Column}
}
