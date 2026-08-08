package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

const (
	DirectCallIndexVersion = 1

	// The direct-call substrate is retained only in memory, but it is still
	// bounded independently from the SSA program. Crossing either ceiling closes
	// the complete index: a retained prefix must never masquerade as a complete
	// neighborhood for a later Study investigation.
	MaxDirectCallIndexNodes = 65_536
	MaxDirectCallIndexEdges = 262_144
)

type DirectCallIndexState string

const (
	DirectCallIndexReady       DirectCallIndexState = "ready"
	DirectCallIndexUnavailable DirectCallIndexState = "unavailable"
)

func (state DirectCallIndexState) Valid() bool {
	return state == DirectCallIndexReady || state == DirectCallIndexUnavailable
}

type DirectCallIndexClosedReason string

const (
	DirectCallIndexClosedSSAUnavailable DirectCallIndexClosedReason = "ssa_unavailable"
	DirectCallIndexClosedNodeLimit      DirectCallIndexClosedReason = "node_limit"
	DirectCallIndexClosedEdgeLimit      DirectCallIndexClosedReason = "edge_limit"
)

func (reason DirectCallIndexClosedReason) Valid() bool {
	switch reason {
	case DirectCallIndexClosedSSAUnavailable,
		DirectCallIndexClosedNodeLimit,
		DirectCallIndexClosedEdgeLimit:
		return true
	default:
		return false
	}
}

type DirectCallInvocation string

const (
	DirectCallSynchronous DirectCallInvocation = "synchronous"
	DirectCallGoroutine   DirectCallInvocation = "goroutine"
	DirectCallDeferred    DirectCallInvocation = "deferred"
)

func (invocation DirectCallInvocation) Valid() bool {
	switch invocation {
	case DirectCallSynchronous, DirectCallGoroutine, DirectCallDeferred:
		return true
	default:
		return false
	}
}

// DirectCallModule is one exact repository module participating in the live
// build-selected SSA program. Directory is repository-relative; Path is the Go
// module path and never an absolute host path.
type DirectCallModule struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Directory string `json:"directory"`
}

// DirectCallBodyRange lets a later exact Reading binding distinguish a
// declaration from a callsite inside the same function without fuzzy symbol
// matching or another package load.
type DirectCallBodyRange struct {
	Start Location `json:"start"`
	End   Location `json:"end"`
}

type DirectCallNode struct {
	ID          string              `json:"id"`
	Symbol      Symbol              `json:"symbol"`
	Package     string              `json:"package"`
	ModuleID    string              `json:"module_id"`
	ScenarioID  string              `json:"scenario_id"`
	Declaration Location            `json:"declaration"`
	Body        DirectCallBodyRange `json:"body"`
}

// DirectCallEdge is one exact source-level repository call relation. Calls
// with the same endpoints and invocation mode compact into one edge; the first
// exact callsite in source order is retained and WitnessCount remains complete.
type DirectCallEdge struct {
	ID                     string               `json:"id"`
	CallerID               string               `json:"caller_id"`
	CalleeID               string               `json:"callee_id"`
	ScenarioID             string               `json:"scenario_id"`
	Invocation             DirectCallInvocation `json:"invocation"`
	RepresentativeCallsite Location             `json:"representative_callsite"`
	WitnessCount           int                  `json:"witness_count"`
}

// DirectCallNodeFrontier is closed per-caller accounting for call
// instructions that cannot become exact repository-local DirectCallEdges.
// It deliberately retains neither a guessed target nor another source
// location: CallerID is already bound to one exact DirectCallNode.
type DirectCallNodeFrontier struct {
	CallerID                string `json:"caller_id"`
	DynamicInvokesExcluded  int    `json:"dynamic_invokes_excluded"`
	NonStaticCallsExcluded  int    `json:"non_static_calls_excluded"`
	ExternalCalleesExcluded int    `json:"external_callees_excluded"`
}

// DirectCallIndexCoverage accounts for deliberately excluded non-exact call
// shapes. These counts are local diagnostics; none of their prose or source
// details is serialized into the report product.
type DirectCallIndexCoverage struct {
	FunctionsConsidered          int `json:"functions_considered"`
	CallInstructionsConsidered   int `json:"call_instructions_considered"`
	ModulesIndexed               int `json:"modules_indexed"`
	NodesConsidered              int `json:"nodes_considered"`
	NodesIndexed                 int `json:"nodes_indexed"`
	UniqueEdgesConsidered        int `json:"unique_edges_considered"`
	EdgesIndexed                 int `json:"edges_indexed"`
	DirectStaticWitnessesIndexed int `json:"direct_static_witnesses_indexed"`
	SyntheticFunctionsExcluded   int `json:"synthetic_functions_excluded"`
	InvalidFunctionsExcluded     int `json:"invalid_functions_excluded"`
	DynamicInvokesExcluded       int `json:"dynamic_invokes_excluded"`
	NonStaticCallsExcluded       int `json:"non_static_calls_excluded"`
	NonRepositoryCallsExcluded   int `json:"non_repository_calls_excluded"`
	InvalidEndpointCallsExcluded int `json:"invalid_endpoint_calls_excluded"`
	InvalidCallsitesExcluded     int `json:"invalid_callsites_excluded"`
}

// DirectCallIndex is a deterministic, bounded, non-persisted local substrate.
// SHA256 binds the complete canonical index (or its closed unavailable state)
// so later bounded requests and final artifacts can name the exact producer
// input without retaining the SSA program.
type DirectCallIndex struct {
	Version      int                         `json:"version"`
	State        DirectCallIndexState        `json:"state"`
	ClosedReason DirectCallIndexClosedReason `json:"closed_reason,omitempty"`
	Scenario     Scenario                    `json:"scenario"`
	Modules      []DirectCallModule          `json:"modules"`
	Nodes        []DirectCallNode            `json:"nodes"`
	Edges        []DirectCallEdge            `json:"edges"`
	Frontiers    []DirectCallNodeFrontier    `json:"frontiers"`
	Coverage     DirectCallIndexCoverage     `json:"coverage"`
	SHA256       string                      `json:"sha256"`

	nodeLookup     map[string]int
	moduleLookup   map[string]int
	incomingLookup map[string][]int
	outgoingLookup map[string][]int
	frontierLookup map[string]int
}

// UnavailableDirectCallIndex returns the canonical closed substrate used when
// an ordinary run has no Go surface SSA handoff. It performs no package load
// or analysis and retains no partial graph; later Study cards can therefore
// publish an honest prepared investigation instead of fabricating a graph or
// omitting the current artifact family.
func UnavailableDirectCallIndex() DirectCallIndex {
	builder := newDirectCallIndexBuilder(Scenario{
		ID:   scenarioID(runtime.GOOS, runtime.GOARCH, nil),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Tags: []string{},
	})
	builder.close(DirectCallIndexClosedSSAUnavailable)
	return builder.finish()
}

// Snapshot returns an independently owned in-memory copy of the complete
// direct-call index. The public slices, nested symbol aliases, scenario tags,
// and private lookup tables share no backing storage with index. This is the
// handoff boundary between surface discovery and a later live-run consumer:
// either side may retain or query its copy without mutating the producer's
// result. No serialization, package loading, or SSA work is performed.
func (index DirectCallIndex) Snapshot() DirectCallIndex {
	snapshot := index
	snapshot.Scenario.Tags = cloneDirectCallSlice(index.Scenario.Tags)
	snapshot.Modules = cloneDirectCallSlice(index.Modules)
	snapshot.Nodes = cloneDirectCallSlice(index.Nodes)
	for position := range snapshot.Nodes {
		snapshot.Nodes[position] = copyDirectCallNode(snapshot.Nodes[position])
	}
	snapshot.Edges = cloneDirectCallSlice(index.Edges)
	snapshot.Frontiers = cloneDirectCallSlice(index.Frontiers)
	snapshot.initializeLookups()
	return snapshot
}

func cloneDirectCallSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	result := make([]T, len(values))
	copy(result, values)
	return result
}

type DirectCallRootState string

const (
	DirectCallRootResolved   DirectCallRootState = "resolved"
	DirectCallRootAmbiguous  DirectCallRootState = "ambiguous"
	DirectCallRootUnresolved DirectCallRootState = "unresolved"
)

type DirectCallRootBinding string

const (
	DirectCallRootDeclaration        DirectCallRootBinding = "declaration"
	DirectCallRootContainingFunction DirectCallRootBinding = "containing_function"
)

// DirectCallRootResolution is an exact local lookup result for a final Study
// Reading. Resolution first requires a declaration locator match; a callsite
// may bind only to the exact same symbol whose saved body range contains the
// line. Multiple matches remain ambiguous rather than picking a representative.
type DirectCallRootResolution struct {
	State   DirectCallRootState   `json:"state"`
	Binding DirectCallRootBinding `json:"binding,omitempty"`
	Node    DirectCallNode        `json:"node,omitempty"`
}

func (index *DirectCallIndex) ResolveRoot(path string, line int, symbol string) DirectCallRootResolution {
	if index == nil || index.State != DirectCallIndexReady ||
		!validRepositoryDirectCallLocation(Location{Path: path, Line: line}) || strings.TrimSpace(symbol) == "" {
		return DirectCallRootResolution{State: DirectCallRootUnresolved}
	}
	declarations := make([]DirectCallNode, 0, 1)
	for _, node := range index.Nodes {
		if node.Declaration.Path == path && node.Declaration.Line == line &&
			directCallSymbolMatches(node.Symbol, symbol) {
			declarations = append(declarations, copyDirectCallNode(node))
		}
	}
	if len(declarations) == 1 {
		return DirectCallRootResolution{
			State: DirectCallRootResolved, Binding: DirectCallRootDeclaration, Node: declarations[0],
		}
	}
	if len(declarations) > 1 {
		return DirectCallRootResolution{State: DirectCallRootAmbiguous}
	}

	containing := make([]DirectCallNode, 0, 1)
	for _, node := range index.Nodes {
		if directCallSymbolMatches(node.Symbol, symbol) && directCallBodyContains(node.Body, path, line) {
			containing = append(containing, copyDirectCallNode(node))
		}
	}
	if len(containing) == 1 {
		return DirectCallRootResolution{
			State: DirectCallRootResolved, Binding: DirectCallRootContainingFunction, Node: containing[0],
		}
	}
	if len(containing) > 1 {
		return DirectCallRootResolution{State: DirectCallRootAmbiguous}
	}
	return DirectCallRootResolution{State: DirectCallRootUnresolved}
}

func (index *DirectCallIndex) Node(id string) (DirectCallNode, bool) {
	if index == nil || index.State != DirectCallIndexReady {
		return DirectCallNode{}, false
	}
	if index.nodeLookup != nil {
		position, ok := index.nodeLookup[id]
		if !ok {
			return DirectCallNode{}, false
		}
		return copyDirectCallNode(index.Nodes[position]), true
	}
	for _, node := range index.Nodes {
		if node.ID == id {
			return copyDirectCallNode(node), true
		}
	}
	return DirectCallNode{}, false
}

func (index *DirectCallIndex) Module(id string) (DirectCallModule, bool) {
	if index == nil || index.State != DirectCallIndexReady {
		return DirectCallModule{}, false
	}
	if index.moduleLookup != nil {
		position, ok := index.moduleLookup[id]
		if !ok {
			return DirectCallModule{}, false
		}
		return index.Modules[position], true
	}
	for _, module := range index.Modules {
		if module.ID == id {
			return module, true
		}
	}
	return DirectCallModule{}, false
}

func (index *DirectCallIndex) Incoming(nodeID string) []DirectCallEdge {
	if index == nil || index.State != DirectCallIndexReady {
		return []DirectCallEdge{}
	}
	if index.incomingLookup != nil {
		return index.edgesAt(index.incomingLookup[nodeID])
	}
	result := make([]DirectCallEdge, 0)
	for _, edge := range index.Edges {
		if edge.CalleeID == nodeID {
			result = append(result, edge)
		}
	}
	return result
}

func (index *DirectCallIndex) Outgoing(nodeID string) []DirectCallEdge {
	if index == nil || index.State != DirectCallIndexReady {
		return []DirectCallEdge{}
	}
	if index.outgoingLookup != nil {
		return index.edgesAt(index.outgoingLookup[nodeID])
	}
	result := make([]DirectCallEdge, 0)
	for _, edge := range index.Edges {
		if edge.CallerID == nodeID {
			result = append(result, edge)
		}
	}
	return result
}

// Frontier returns immutable closed exclusion counts for one exact caller.
// False means that the node has no excluded call shape recorded (or that the
// index/node is unavailable); callers can use Node when they need to
// distinguish those cases.
func (index *DirectCallIndex) Frontier(nodeID string) (DirectCallNodeFrontier, bool) {
	if index == nil || index.State != DirectCallIndexReady {
		return DirectCallNodeFrontier{}, false
	}
	if index.frontierLookup != nil {
		position, ok := index.frontierLookup[nodeID]
		if !ok || position < 0 || position >= len(index.Frontiers) {
			return DirectCallNodeFrontier{}, false
		}
		return index.Frontiers[position], true
	}
	for _, frontier := range index.Frontiers {
		if frontier.CallerID == nodeID {
			return frontier, true
		}
	}
	return DirectCallNodeFrontier{}, false
}

func (index *DirectCallIndex) edgesAt(positions []int) []DirectCallEdge {
	result := make([]DirectCallEdge, 0, len(positions))
	for _, position := range positions {
		if position >= 0 && position < len(index.Edges) {
			result = append(result, index.Edges[position])
		}
	}
	return result
}

func (index *DirectCallIndex) initializeLookups() {
	index.nodeLookup = make(map[string]int, len(index.Nodes))
	index.moduleLookup = make(map[string]int, len(index.Modules))
	index.incomingLookup = make(map[string][]int)
	index.outgoingLookup = make(map[string][]int)
	index.frontierLookup = make(map[string]int, len(index.Frontiers))
	for position, module := range index.Modules {
		index.moduleLookup[module.ID] = position
	}
	for position, node := range index.Nodes {
		index.nodeLookup[node.ID] = position
	}
	for position, edge := range index.Edges {
		index.incomingLookup[edge.CalleeID] = append(index.incomingLookup[edge.CalleeID], position)
		index.outgoingLookup[edge.CallerID] = append(index.outgoingLookup[edge.CallerID], position)
	}
	for position, frontier := range index.Frontiers {
		index.frontierLookup[frontier.CallerID] = position
	}
}

func (index DirectCallIndex) Validate() error {
	if index.Version != DirectCallIndexVersion {
		return fmt.Errorf("direct call index: unsupported version %d", index.Version)
	}
	if !index.State.Valid() {
		return fmt.Errorf("direct call index: invalid state %q", index.State)
	}
	if index.Scenario.ID == "" || index.Scenario.GOOS == "" || index.Scenario.GOARCH == "" ||
		!sort.StringsAreSorted(index.Scenario.Tags) || !uniqueStrings(index.Scenario.Tags) {
		return fmt.Errorf("direct call index: invalid canonical scenario")
	}
	if index.State == DirectCallIndexUnavailable {
		if !index.ClosedReason.Valid() {
			return fmt.Errorf("direct call index: unavailable index has invalid closed reason %q", index.ClosedReason)
		}
		if len(index.Modules) != 0 || len(index.Nodes) != 0 || len(index.Edges) != 0 || len(index.Frontiers) != 0 ||
			index.Coverage.ModulesIndexed != 0 || index.Coverage.NodesIndexed != 0 ||
			index.Coverage.EdgesIndexed != 0 {
			return fmt.Errorf("direct call index: unavailable index retained a partial graph")
		}
	} else if index.ClosedReason != "" {
		return fmt.Errorf("direct call index: ready index has closed reason %q", index.ClosedReason)
	}
	if len(index.Nodes) > MaxDirectCallIndexNodes || len(index.Edges) > MaxDirectCallIndexEdges {
		return fmt.Errorf("direct call index: graph exceeds production bounds")
	}
	if index.Coverage.ModulesIndexed != len(index.Modules) ||
		index.Coverage.NodesIndexed != len(index.Nodes) ||
		index.Coverage.EdgesIndexed != len(index.Edges) {
		return fmt.Errorf("direct call index: coverage does not match graph")
	}

	modules := make(map[string]DirectCallModule, len(index.Modules))
	previous := ""
	for _, module := range index.Modules {
		key := directCallModuleKey(module)
		if module.ID == "" || module.Path == "" || !validDirectCallModuleDirectory(module.Directory) ||
			module.ID != stableDirectCallID("direct-module", module.Path, module.Directory) {
			return fmt.Errorf("direct call index: invalid module %q", module.ID)
		}
		if previous != "" && key <= previous {
			return fmt.Errorf("direct call index: modules are not unique canonical order")
		}
		previous = key
		modules[module.ID] = module
	}

	nodes := make(map[string]DirectCallNode, len(index.Nodes))
	previous = ""
	for _, node := range index.Nodes {
		key := directCallNodeKey(node)
		if previous != "" && key <= previous {
			return fmt.Errorf("direct call index: nodes are not unique canonical order")
		}
		previous = key
		if _, ok := modules[node.ModuleID]; !ok || node.ScenarioID != index.Scenario.ID ||
			node.Package == "" || node.Symbol.ID == "" || node.Symbol.Package != node.Package ||
			!validDirectCallEquivalentIDs(node.Symbol.EquivalentIDs) ||
			node.Symbol.Location.Path != node.Declaration.Path ||
			node.Symbol.Location.Line != node.Declaration.Line ||
			node.Symbol.Location.Column != node.Declaration.Column ||
			!validRepositoryDirectCallLocation(node.Declaration) ||
			!validDirectCallBody(node.Declaration, node.Body) ||
			node.ID != stableDirectCallNodeID(node) {
			return fmt.Errorf("direct call index: invalid node %q", node.ID)
		}
		nodes[node.ID] = node
	}

	edges := make(map[string]struct{}, len(index.Edges))
	previous = ""
	for _, edge := range index.Edges {
		key := directCallEdgeKey(edge)
		if previous != "" && key <= previous {
			return fmt.Errorf("direct call index: edges are not unique canonical order")
		}
		previous = key
		if _, ok := nodes[edge.CallerID]; !ok {
			return fmt.Errorf("direct call index: edge %q has unknown caller", edge.ID)
		}
		if _, ok := nodes[edge.CalleeID]; !ok {
			return fmt.Errorf("direct call index: edge %q has unknown callee", edge.ID)
		}
		if edge.ScenarioID != index.Scenario.ID || !edge.Invocation.Valid() || edge.WitnessCount <= 0 ||
			!validRepositoryDirectCallLocation(edge.RepresentativeCallsite) ||
			edge.ID != stableDirectCallEdgeID(edge) {
			return fmt.Errorf("direct call index: invalid edge %q", edge.ID)
		}
		if _, duplicate := edges[edge.ID]; duplicate {
			return fmt.Errorf("direct call index: duplicate edge %q", edge.ID)
		}
		edges[edge.ID] = struct{}{}
	}

	previous = ""
	dynamicInvokes := 0
	nonStaticCalls := 0
	externalCallees := 0
	for _, frontier := range index.Frontiers {
		if previous != "" && frontier.CallerID <= previous {
			return fmt.Errorf("direct call index: frontiers are not unique canonical order")
		}
		previous = frontier.CallerID
		if _, ok := nodes[frontier.CallerID]; !ok {
			return fmt.Errorf("direct call index: frontier has unknown caller %q", frontier.CallerID)
		}
		if frontier.DynamicInvokesExcluded < 0 || frontier.NonStaticCallsExcluded < 0 ||
			frontier.ExternalCalleesExcluded < 0 ||
			frontier.DynamicInvokesExcluded+frontier.NonStaticCallsExcluded+frontier.ExternalCalleesExcluded == 0 {
			return fmt.Errorf("direct call index: invalid frontier for caller %q", frontier.CallerID)
		}
		dynamicInvokes += frontier.DynamicInvokesExcluded
		nonStaticCalls += frontier.NonStaticCallsExcluded
		externalCallees += frontier.ExternalCalleesExcluded
	}
	if dynamicInvokes > index.Coverage.DynamicInvokesExcluded ||
		nonStaticCalls > index.Coverage.NonStaticCallsExcluded ||
		externalCallees > index.Coverage.NonRepositoryCallsExcluded {
		return fmt.Errorf("direct call index: frontier exceeds global exclusion coverage")
	}

	digest, err := directCallIndexSHA256(index)
	if err != nil {
		return err
	}
	if len(index.SHA256) != sha256.Size*2 || index.SHA256 != digest {
		return fmt.Errorf("direct call index: sha256 mismatch")
	}
	if _, err := hex.DecodeString(index.SHA256); err != nil {
		return fmt.Errorf("direct call index: invalid sha256: %w", err)
	}
	return nil
}

type directCallIndexBuilder struct {
	scenario      Scenario
	maxNodes      int
	maxEdges      int
	state         DirectCallIndexState
	closedReason  DirectCallIndexClosedReason
	modules       map[string]DirectCallModule
	nodes         map[string]DirectCallNode
	edges         map[string]DirectCallEdge
	frontiers     map[string]DirectCallNodeFrontier
	functionNode  map[*ssa.Function]string
	functionsSeen map[*ssa.Function]struct{}
	coverage      DirectCallIndexCoverage
}

func newDirectCallIndexBuilder(scenario Scenario) *directCallIndexBuilder {
	return newDirectCallIndexBuilderWithLimits(scenario, MaxDirectCallIndexNodes, MaxDirectCallIndexEdges)
}

func newDirectCallIndexBuilderWithLimits(scenario Scenario, maxNodes, maxEdges int) *directCallIndexBuilder {
	scenario.Tags = append([]string(nil), scenario.Tags...)
	sort.Strings(scenario.Tags)
	scenario.Tags = compactStrings(scenario.Tags)
	return &directCallIndexBuilder{
		scenario: scenario, maxNodes: maxNodes, maxEdges: maxEdges,
		state:   DirectCallIndexReady,
		modules: make(map[string]DirectCallModule), nodes: make(map[string]DirectCallNode),
		edges: make(map[string]DirectCallEdge), functionNode: make(map[*ssa.Function]string),
		frontiers:     make(map[string]DirectCallNodeFrontier),
		functionsSeen: make(map[*ssa.Function]struct{}),
	}
}

func (builder *directCallIndexBuilder) close(reason DirectCallIndexClosedReason) {
	if builder == nil || builder.state == DirectCallIndexUnavailable {
		return
	}
	builder.state = DirectCallIndexUnavailable
	builder.closedReason = reason
	// Fail closed and release the partial graph immediately. Coverage counters
	// remain available to explain the bounded local outcome.
	builder.modules = nil
	builder.nodes = nil
	builder.edges = nil
	builder.frontiers = nil
	builder.functionNode = nil
	return
}

func (builder *directCallIndexBuilder) recordFunction(a *analyzer, function *ssa.Function) (string, bool) {
	if builder == nil || function == nil || builder.state != DirectCallIndexReady {
		return "", false
	}
	if id, found := builder.functionNode[function]; found {
		return id, id != ""
	}
	if _, seen := builder.functionsSeen[function]; !seen {
		builder.functionsSeen[function] = struct{}{}
		builder.coverage.FunctionsConsidered++
	}
	sourceFunction := function
	if origin := function.Origin(); origin != nil {
		sourceFunction = origin
		if id, found := builder.functionNode[sourceFunction]; found {
			builder.functionNode[function] = id
			return id, id != ""
		}
	}
	if sourceFunction.Synthetic != "" {
		builder.coverage.SyntheticFunctionsExcluded++
		builder.functionNode[function] = ""
		return "", false
	}
	node, module, ok := a.directCallNode(sourceFunction, builder.scenario)
	if !ok {
		builder.coverage.InvalidFunctionsExcluded++
		builder.functionNode[function] = ""
		builder.functionNode[sourceFunction] = ""
		return "", false
	}
	builder.coverage.NodesConsidered++
	if _, exists := builder.nodes[node.ID]; !exists {
		if len(builder.nodes) >= builder.maxNodes {
			builder.close(DirectCallIndexClosedNodeLimit)
			return "", false
		}
		builder.nodes[node.ID] = node
		builder.modules[module.ID] = module
	}
	builder.functionNode[sourceFunction] = node.ID
	builder.functionNode[function] = node.ID
	return node.ID, true
}

func (builder *directCallIndexBuilder) recordCall(a *analyzer, call ssa.CallInstruction) {
	if builder == nil || call == nil || builder.state != DirectCallIndexReady {
		return
	}
	builder.coverage.CallInstructionsConsidered++
	common := call.Common()
	if common == nil {
		builder.coverage.NonStaticCallsExcluded++
		builder.recordCallerFrontier(a, call.Parent(), directCallFrontierNonStatic)
		return
	}
	if common.IsInvoke() {
		builder.coverage.DynamicInvokesExcluded++
		builder.recordCallerFrontier(a, call.Parent(), directCallFrontierDynamicInvoke)
		return
	}
	callee := common.StaticCallee()
	if callee == nil {
		builder.coverage.NonStaticCallsExcluded++
		builder.recordCallerFrontier(a, call.Parent(), directCallFrontierNonStatic)
		return
	}
	if !a.repositoryDirectStaticCall(call, callee) {
		builder.coverage.NonRepositoryCallsExcluded++
		builder.recordCallerFrontier(a, call.Parent(), directCallFrontierExternalCallee)
		return
	}
	callerID, callerOK := builder.recordFunction(a, call.Parent())
	calleeID, calleeOK := builder.recordFunction(a, callee)
	if builder.state != DirectCallIndexReady {
		return
	}
	if !callerOK || !calleeOK {
		builder.coverage.InvalidEndpointCallsExcluded++
		return
	}
	callsite := a.location(call.Pos())
	if !validRepositoryDirectCallLocation(callsite) {
		builder.coverage.InvalidCallsitesExcluded++
		return
	}
	edge := DirectCallEdge{
		CallerID: callerID, CalleeID: calleeID, ScenarioID: builder.scenario.ID,
		Invocation: directCallInvocation(call), RepresentativeCallsite: callsite,
		WitnessCount: 1,
	}
	edge.ID = stableDirectCallEdgeID(edge)
	if existing, found := builder.edges[edge.ID]; found {
		existing.WitnessCount++
		if directCallLocationLess(callsite, existing.RepresentativeCallsite) {
			existing.RepresentativeCallsite = callsite
		}
		builder.edges[edge.ID] = existing
		builder.coverage.DirectStaticWitnessesIndexed++
		return
	}
	builder.coverage.UniqueEdgesConsidered++
	if len(builder.edges) >= builder.maxEdges {
		builder.close(DirectCallIndexClosedEdgeLimit)
		return
	}
	builder.edges[edge.ID] = edge
	builder.coverage.DirectStaticWitnessesIndexed++
}

type directCallFrontierKind uint8

const (
	directCallFrontierDynamicInvoke directCallFrontierKind = iota + 1
	directCallFrontierNonStatic
	directCallFrontierExternalCallee
)

func (builder *directCallIndexBuilder) recordCallerFrontier(
	a *analyzer,
	caller *ssa.Function,
	kind directCallFrontierKind,
) {
	if builder == nil || caller == nil || builder.state != DirectCallIndexReady {
		return
	}
	callerID, ok := builder.recordFunction(a, caller)
	if !ok || builder.state != DirectCallIndexReady {
		return
	}
	frontier := builder.frontiers[callerID]
	frontier.CallerID = callerID
	switch kind {
	case directCallFrontierDynamicInvoke:
		frontier.DynamicInvokesExcluded++
	case directCallFrontierNonStatic:
		frontier.NonStaticCallsExcluded++
	case directCallFrontierExternalCallee:
		frontier.ExternalCalleesExcluded++
	default:
		return
	}
	builder.frontiers[callerID] = frontier
}

func (builder *directCallIndexBuilder) finish() DirectCallIndex {
	if builder == nil {
		builder = newDirectCallIndexBuilder(Scenario{})
		builder.close(DirectCallIndexClosedSSAUnavailable)
	}
	index := DirectCallIndex{
		Version: DirectCallIndexVersion, State: builder.state, ClosedReason: builder.closedReason,
		Scenario: builder.scenario, Coverage: builder.coverage,
		Modules: []DirectCallModule{}, Nodes: []DirectCallNode{}, Edges: []DirectCallEdge{},
		Frontiers: []DirectCallNodeFrontier{},
	}
	if builder.state == DirectCallIndexReady {
		for _, module := range builder.modules {
			index.Modules = append(index.Modules, module)
		}
		for _, node := range builder.nodes {
			index.Nodes = append(index.Nodes, node)
		}
		for _, edge := range builder.edges {
			index.Edges = append(index.Edges, edge)
		}
		for _, frontier := range builder.frontiers {
			index.Frontiers = append(index.Frontiers, frontier)
		}
		sort.Slice(index.Modules, func(i, j int) bool {
			return directCallModuleKey(index.Modules[i]) < directCallModuleKey(index.Modules[j])
		})
		sort.Slice(index.Nodes, func(i, j int) bool {
			return directCallNodeKey(index.Nodes[i]) < directCallNodeKey(index.Nodes[j])
		})
		sort.Slice(index.Edges, func(i, j int) bool {
			return directCallEdgeKey(index.Edges[i]) < directCallEdgeKey(index.Edges[j])
		})
		sort.Slice(index.Frontiers, func(i, j int) bool {
			return index.Frontiers[i].CallerID < index.Frontiers[j].CallerID
		})
		index.Coverage.ModulesIndexed = len(index.Modules)
		index.Coverage.NodesIndexed = len(index.Nodes)
		index.Coverage.EdgesIndexed = len(index.Edges)
	} else {
		index.Coverage.ModulesIndexed = 0
		index.Coverage.NodesIndexed = 0
		index.Coverage.EdgesIndexed = 0
	}
	index.SHA256, _ = directCallIndexSHA256(index)
	index.initializeLookups()
	return index
}

func (a *analyzer) directCallNode(function *ssa.Function, scenario Scenario) (DirectCallNode, DirectCallModule, bool) {
	if a == nil || function == nil || function.Blocks == nil || function.Syntax() == nil ||
		!a.isRepositoryFunction(function) {
		return DirectCallNode{}, DirectCallModule{}, false
	}
	packagePath := functionPackagePath(function)
	facts := a.packageFacts[packagePath]
	if facts == nil || facts.Module == nil || facts.Module.Path == "" || facts.Module.Dir == "" ||
		!a.modulePaths[facts.Module.Path] {
		return DirectCallNode{}, DirectCallModule{}, false
	}
	moduleDirectory, ok := containedModuleDirectory(a.root, facts.Module.Dir)
	if !ok {
		return DirectCallNode{}, DirectCallModule{}, false
	}
	module := DirectCallModule{
		Path: facts.Module.Path, Directory: moduleDirectory,
	}
	module.ID = stableDirectCallID("direct-module", module.Path, module.Directory)
	symbol := a.symbol(function)
	declaration := symbol.Location
	body := DirectCallBodyRange{
		Start: a.location(function.Syntax().Pos()),
		End:   a.location(function.Syntax().End()),
	}
	if !validEntryHandoffSymbol(symbol) || !validRepositoryDirectCallLocation(declaration) ||
		!validDirectCallBody(declaration, body) {
		return DirectCallNode{}, DirectCallModule{}, false
	}
	node := DirectCallNode{
		Symbol: symbol, Package: packagePath, ModuleID: module.ID, ScenarioID: scenario.ID,
		Declaration: declaration, Body: body,
	}
	node.ID = stableDirectCallNodeID(node)
	return node, module, true
}

func containedModuleDirectory(root, moduleRoot string) (string, bool) {
	relative, err := filepath.Rel(root, moduleRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	if relative == "" || relative == "." {
		return ".", true
	}
	relative = filepath.ToSlash(relative)
	return relative, fs.ValidPath(relative)
}

func directCallInvocation(call ssa.CallInstruction) DirectCallInvocation {
	switch call.(type) {
	case *ssa.Go:
		return DirectCallGoroutine
	case *ssa.Defer:
		return DirectCallDeferred
	default:
		return DirectCallSynchronous
	}
}

func validRepositoryDirectCallLocation(location Location) bool {
	return validEntryHandoffLocation(location) && !strings.HasPrefix(location.Path, "<external>/")
}

func validDirectCallModuleDirectory(directory string) bool {
	return directory == "." || fs.ValidPath(directory)
}

func validDirectCallBody(declaration Location, body DirectCallBodyRange) bool {
	if !validRepositoryDirectCallLocation(body.Start) || !validRepositoryDirectCallLocation(body.End) ||
		body.Start.Path != declaration.Path || body.End.Path != declaration.Path {
		return false
	}
	if body.Start.Line > body.End.Line {
		return false
	}
	return body.Start.Line != body.End.Line || body.Start.Column <= body.End.Column
}

func directCallBodyContains(body DirectCallBodyRange, path string, line int) bool {
	return path != "" && line > 0 && body.Start.Path == path && body.End.Path == path &&
		line >= body.Start.Line && line <= body.End.Line
}

func directCallSymbolMatches(symbol Symbol, candidate string) bool {
	if candidate == symbol.ID {
		return true
	}
	position := sort.SearchStrings(symbol.EquivalentIDs, candidate)
	return position < len(symbol.EquivalentIDs) && symbol.EquivalentIDs[position] == candidate
}

func validDirectCallEquivalentIDs(ids []string) bool {
	if !sort.StringsAreSorted(ids) || !uniqueStrings(ids) {
		return false
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return false
		}
	}
	return true
}

func copyDirectCallNode(node DirectCallNode) DirectCallNode {
	node.Symbol.EquivalentIDs = append([]string(nil), node.Symbol.EquivalentIDs...)
	return node
}

func stableDirectCallNodeID(node DirectCallNode) string {
	return stableDirectCallID(
		"direct-node", node.ModuleID, node.ScenarioID, node.Symbol.ID,
		locationKey(node.Declaration),
	)
}

func stableDirectCallEdgeID(edge DirectCallEdge) string {
	return stableDirectCallID(
		"direct-edge", edge.ScenarioID, edge.CallerID, edge.CalleeID, string(edge.Invocation),
	)
}

func stableDirectCallID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		var length [8]byte
		for index := range length {
			length[index] = byte(uint64(len(field)) >> uint(56-index*8))
		}
		digest.Write(length[:])
		digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func directCallModuleKey(module DirectCallModule) string {
	return strings.Join([]string{module.Directory, module.Path, module.ID}, "\x00")
}

func directCallNodeKey(node DirectCallNode) string {
	return strings.Join([]string{
		node.ModuleID, node.Package, node.Symbol.ID, locationKey(node.Declaration), node.ID,
	}, "\x00")
}

func directCallEdgeKey(edge DirectCallEdge) string {
	return strings.Join([]string{
		edge.CallerID, edge.CalleeID, string(edge.Invocation),
		locationKey(edge.RepresentativeCallsite), edge.ID,
	}, "\x00")
}

func directCallLocationLess(left, right Location) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Column < right.Column
}

func directCallIndexSHA256(index DirectCallIndex) (string, error) {
	index.SHA256 = ""
	encoded, err := json.Marshal(index)
	if err != nil {
		return "", fmt.Errorf("direct call index: encode digest material: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func uniqueStrings(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return false
		}
	}
	return true
}
