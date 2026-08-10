package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/entrycall"
	"golang.org/x/tools/go/ssa"
)

const (
	maxEntryCallExternalNodes = 32_768
	maxEntryCallFamilies      = 65_536
)

// entryCallSidecar is enabled only when a live consumer requests it. It
// observes the existing SSA instruction pass and never owns a program, package
// load, or second call graph.
type entryCallSidecar struct {
	available         bool
	externalNodes     map[string]entrycall.ExactNode
	localLabels       map[string]string
	families          map[string]entrycall.ExactFamily
	capturedExternal  map[string]int
	repositoryCalls   map[*ssa.Function]map[*ssa.Function]struct{}
	surfaceCandidates map[string]rawEntrySurfaceCandidate
	surfaceCoverage   entrycall.Coverage
}

func newEntryCallSidecar() *entryCallSidecar {
	return &entryCallSidecar{
		available:         true,
		externalNodes:     make(map[string]entrycall.ExactNode),
		localLabels:       make(map[string]string),
		families:          make(map[string]entrycall.ExactFamily),
		capturedExternal:  make(map[string]int),
		repositoryCalls:   make(map[*ssa.Function]map[*ssa.Function]struct{}),
		surfaceCandidates: make(map[string]rawEntrySurfaceCandidate),
	}
}

func (builder *directCallIndexBuilder) enableEntryCallSidecar() {
	if builder != nil && builder.entryCalls == nil {
		builder.entryCalls = newEntryCallSidecar()
	}
}

func (sidecar *entryCallSidecar) close() {
	if sidecar == nil || !sidecar.available {
		return
	}
	sidecar.available = false
	sidecar.externalNodes = nil
	sidecar.localLabels = nil
	sidecar.families = nil
	sidecar.capturedExternal = nil
	sidecar.repositoryCalls = nil
	sidecar.surfaceCandidates = nil
}

func (builder *directCallIndexBuilder) recordLocalEntryCall(
	a *analyzer,
	call ssa.CallInstruction,
	callee *ssa.Function,
	edge DirectCallEdge,
) {
	if builder == nil || builder.entryCalls == nil || !builder.entryCalls.available || a == nil ||
		call == nil || callee == nil {
		return
	}
	caller := call.Parent()
	builder.entryCalls.localLabels[edge.CallerID] = entryCallFunctionLabel(caller)
	builder.entryCalls.localLabels[edge.CalleeID] = entryCallFunctionLabel(callee)
	builder.entryCalls.recordFamily(entrycall.ExactFamily{
		ID: edge.ID, CallerID: edge.CallerID, CalleeID: edge.CalleeID,
		Invocation: entryCallInvocation(edge.Invocation), WitnessCount: 1,
		Callsites: []entrycall.Location{entryCallLocation(edge.RepresentativeCallsite)},
	})
}

// recordExternalEntryCall preserves only safely named StaticCallee calls from
// an exact repository-local caller. It returns true only when that witness was
// retained, allowing the later frontier to distinguish identified external
// families from genuinely unidentified static calls.
func (builder *directCallIndexBuilder) recordExternalEntryCall(
	a *analyzer,
	call ssa.CallInstruction,
	callee *ssa.Function,
) bool {
	if builder == nil || builder.entryCalls == nil || !builder.entryCalls.available || a == nil ||
		call == nil || callee == nil || call.Parent() == nil || !a.isRepositoryFunction(call.Parent()) {
		return false
	}
	callerID, ok := builder.recordFunction(a, call.Parent())
	if !ok || builder.state != DirectCallIndexReady {
		return false
	}
	source := callee
	if origin := callee.Origin(); origin != nil {
		source = origin
	}
	object := source.Object()
	if object == nil || object.Pkg() == nil {
		return false
	}
	packagePath := object.Pkg().Path()
	name := strings.TrimSpace(object.Name())
	if packagePath == "" || name == "" || a.isRepositoryFunction(source) {
		return false
	}
	callsite := a.location(call.Pos())
	if !validRepositoryDirectCallLocation(callsite) {
		return false
	}
	receiver := receiverName(source.Signature)
	calleeID := "external:" + packagePath + "\x00" + receiver + "\x00" + name
	if _, known := builder.entryCalls.externalNodes[calleeID]; !known {
		if len(builder.entryCalls.externalNodes) >= maxEntryCallExternalNodes {
			builder.entryCalls.close()
			return false
		}
		builder.entryCalls.externalNodes[calleeID] = entrycall.ExactNode{
			ID: calleeID, Label: entrycall.PackageLabel(packagePath, receiver, name), External: true,
		}
	}
	builder.entryCalls.localLabels[callerID] = entryCallFunctionLabel(call.Parent())
	family := entrycall.ExactFamily{
		CallerID: callerID, CalleeID: calleeID,
		Invocation: entryCallInvocation(directCallInvocation(call)), WitnessCount: 1,
		Callsites: []entrycall.Location{entryCallLocation(callsite)},
	}
	family.ID = entryCallFamilyID(family)
	if !builder.entryCalls.recordFamily(family) {
		return false
	}
	builder.entryCalls.capturedExternal[callerID]++
	return true
}

func (sidecar *entryCallSidecar) recordFamily(family entrycall.ExactFamily) bool {
	if sidecar == nil || !sidecar.available {
		return false
	}
	if existing, known := sidecar.families[family.ID]; known {
		existing.WitnessCount++
		existing.Callsites = appendEntryCallCallsite(existing.Callsites, family.Callsites[0])
		sidecar.families[family.ID] = existing
		return true
	}
	if len(sidecar.families) >= maxEntryCallFamilies {
		sidecar.close()
		return false
	}
	family.Callsites = appendEntryCallCallsite(nil, family.Callsites[0])
	sidecar.families[family.ID] = family
	return true
}

func appendEntryCallCallsite(locations []entrycall.Location, candidate entrycall.Location) []entrycall.Location {
	for _, location := range locations {
		if location == candidate {
			return locations
		}
	}
	locations = append(locations, candidate)
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].Path != locations[j].Path {
			return locations[i].Path < locations[j].Path
		}
		if locations[i].Line != locations[j].Line {
			return locations[i].Line < locations[j].Line
		}
		return locations[i].Column < locations[j].Column
	})
	if len(locations) > entrycall.MaxRepresentativeCallsites {
		locations = locations[:entrycall.MaxRepresentativeCallsites]
	}
	return locations
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
	if !builder.entryCalls.available {
		return entrycall.Unavailable(entrycall.ClosedSidecarLimit)
	}
	if len(entrypoints) == 0 {
		return entrycall.Unavailable(entrycall.ClosedNoEntrypoints)
	}
	substrate := entrycall.Substrate{
		Version: entrycall.SubstrateVersion, State: entrycall.StateReady,
		Roots: []entrycall.ExactRoot{}, Nodes: []entrycall.ExactNode{},
		Families: []entrycall.ExactFamily{}, Frontiers: []entrycall.ExactFrontier{},
		SurfaceCandidates: []entrycall.ExactSurfaceCandidate{},
	}
	substrate.SurfaceCandidates, substrate.Coverage =
		builder.entryCalls.projectSurfaceCandidates(a, builder, entrypoints)
	for _, node := range index.Nodes {
		label := builder.entryCalls.localLabels[node.ID]
		if label == "" {
			label = entrycall.PackageLabel(node.Package, "", node.Symbol.Name)
		}
		substrate.Nodes = append(substrate.Nodes, entrycall.ExactNode{
			ID: node.ID, Label: label, Declaration: entryCallLocation(node.Declaration),
		})
	}
	for _, node := range builder.entryCalls.externalNodes {
		substrate.Nodes = append(substrate.Nodes, node)
	}
	for _, family := range builder.entryCalls.families {
		family.Callsites = append([]entrycall.Location(nil), family.Callsites...)
		substrate.Families = append(substrate.Families, family)
		substrate.Coverage.WitnessesIndexed += family.WitnessCount
	}
	for _, frontier := range index.Frontiers {
		unidentified := frontier.ExternalCalleesExcluded - builder.entryCalls.capturedExternal[frontier.CallerID]
		if unidentified < 0 {
			unidentified = 0
		}
		if frontier.DynamicInvokesExcluded+frontier.NonStaticCallsExcluded+unidentified == 0 {
			continue
		}
		substrate.Frontiers = append(substrate.Frontiers, entrycall.ExactFrontier{
			CallerID:                  frontier.CallerID,
			DynamicInvokesExcluded:    frontier.DynamicInvokesExcluded,
			NonStaticCallsExcluded:    frontier.NonStaticCallsExcluded,
			UnidentifiedCallsExcluded: unidentified,
		})
	}
	for _, entrypoint := range entrypoints {
		if nodeID, ok := builder.functionNode[entrypoint]; ok && nodeID != "" {
			substrate.Roots = append(substrate.Roots, entrycall.ExactRoot{NodeID: nodeID})
		}
	}
	if len(substrate.Roots) == 0 {
		return entrycall.Unavailable(entrycall.ClosedNoEntrypoints)
	}
	sort.Slice(substrate.Roots, func(i, j int) bool { return substrate.Roots[i].NodeID < substrate.Roots[j].NodeID })
	sort.Slice(substrate.Nodes, func(i, j int) bool { return substrate.Nodes[i].ID < substrate.Nodes[j].ID })
	sort.Slice(substrate.Families, func(i, j int) bool { return substrate.Families[i].ID < substrate.Families[j].ID })
	sort.Slice(substrate.Frontiers, func(i, j int) bool { return substrate.Frontiers[i].CallerID < substrate.Frontiers[j].CallerID })
	substrate.Coverage.RootsConsidered = len(substrate.Roots)
	substrate.Coverage.NodesConsidered = len(substrate.Nodes)
	substrate.Coverage.FamiliesConsidered = len(substrate.Families)
	return substrate
}

func entryCallFunctionLabel(function *ssa.Function) string {
	if function == nil {
		return "symbol"
	}
	return entrycall.PackageLabel(functionPackagePath(function), receiverName(function.Signature), function.Name())
}

func entryCallLocation(location Location) entrycall.Location {
	return entrycall.Location{Path: location.Path, Line: location.Line, Column: location.Column}
}

func entryCallInvocation(invocation DirectCallInvocation) entrycall.Invocation {
	switch invocation {
	case DirectCallGoroutine:
		return entrycall.InvocationGoroutine
	case DirectCallDeferred:
		return entrycall.InvocationDeferred
	default:
		return entrycall.InvocationSynchronous
	}
}

func entryCallFamilyID(family entrycall.ExactFamily) string {
	digest := sha256.Sum256([]byte(
		family.CallerID + "\x00" + family.CalleeID + "\x00" + string(family.Invocation),
	))
	return "entry-family-" + hex.EncodeToString(digest[:])
}
