package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

const (
	DirectCallIndexVersion = 3

	// The direct-call substrate is retained only in memory, but it is still
	// bounded independently from the SSA program. Crossing either ceiling closes
	// the index: a retained prefix must never masquerade as the complete declared
	// or configured target-rooted neighborhood for later domain cubes.
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
	ID     string `json:"id"`
	Symbol Symbol `json:"symbol"`
	// Package and Exported are producer-owned declaration facts. Later
	// consumers may scope an exact public API without guessing from symbol
	// spelling or loading the package a second time.
	Package     string              `json:"package"`
	Exported    bool                `json:"exported"`
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
	CallerID                          string `json:"caller_id"`
	DynamicInvokesExcluded            int    `json:"dynamic_invokes_excluded"`
	NonStaticCallsExcluded            int    `json:"non_static_calls_excluded"`
	ExternalCalleesExcluded           int    `json:"external_callees_excluded"`
	DepthBoundRepositoryCallsExcluded int    `json:"depth_bound_repository_calls_excluded,omitempty"`
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
	// DepthBoundRepositoryCallsExcluded counts exact repository-local call
	// instructions whose caller was reached exactly at Scope.MaxDepth. Their
	// edges are intentionally absent, so a later consumer cannot interpret a
	// missing connector as proof of true target-unreachability.
	DepthBoundRepositoryCallsExcluded int `json:"depth_bound_repository_calls_excluded"`
	// EdgeLimitSafeDepth is populated only when a target-rooted edge ceiling
	// closes the index. It is the greatest positive CLI depth known to exclude
	// the overflowing BFS layer, so the suggested retry is causal rather than a
	// blind depth-1 guess. Zero means no positive depth is known to fit.
	EdgeLimitSafeDepth int `json:"edge_limit_safe_depth,omitempty"`
}

// DirectCallIndexScope binds the exact declaration catalog and target-rooted
// relation neighborhood to one selected analysis target and explicit bounds.
type DirectCallIndexScope struct {
	TargetRef        string   `json:"target_ref,omitempty"`
	TargetKind       string   `json:"target_kind,omitempty"`
	TargetModuleID   string   `json:"target_module_id,omitempty"`
	TargetModulePath string   `json:"target_module_path,omitempty"`
	TargetModuleDir  string   `json:"target_module_dir,omitempty"`
	TargetPackage    string   `json:"target_package,omitempty"`
	TargetPackages   []string `json:"target_packages,omitempty"`
	MaxDepth         int      `json:"max_depth,omitempty"`
	EdgeLimit        int      `json:"edge_limit,omitempty"`
}

func (scope DirectCallIndexScope) TargetScoped() bool {
	return scope.TargetRef != "" || scope.TargetKind != "" || scope.TargetModuleID != "" ||
		scope.TargetModulePath != "" || scope.TargetModuleDir != "" || scope.TargetPackage != "" ||
		len(scope.TargetPackages) != 0 || scope.MaxDepth != 0 || scope.EdgeLimit != 0
}

func (scope DirectCallIndexScope) validate() error {
	if !scope.TargetScoped() {
		return fmt.Errorf("direct call index: exact target scope is required")
	}
	if scope.TargetKind != AnalysisTargetExecutablePackage &&
		scope.TargetKind != AnalysisTargetModuleLibrary {
		return fmt.Errorf("direct call index: invalid target scope kind %q", scope.TargetKind)
	}
	if !validDirectCallTargetIdentity(scope.TargetRef) || !validDirectCallTargetIdentity(scope.TargetModuleID) ||
		!validDirectCallTargetIdentity(scope.TargetModulePath) ||
		!validDirectCallModuleDirectory(scope.TargetModuleDir) {
		return fmt.Errorf("direct call index: invalid target scope module identity")
	}
	if len(scope.TargetPackages) == 0 || len(scope.TargetPackages) > MaxDirectCallIndexNodes ||
		!sort.StringsAreSorted(scope.TargetPackages) || !uniqueStrings(scope.TargetPackages) {
		return fmt.Errorf("direct call index: invalid target scope packages")
	}
	for _, packagePath := range scope.TargetPackages {
		if !validDirectCallTargetIdentity(packagePath) {
			return fmt.Errorf("direct call index: invalid target scope packages")
		}
	}
	switch scope.TargetKind {
	case AnalysisTargetExecutablePackage:
		if !validDirectCallTargetIdentity(scope.TargetPackage) ||
			len(scope.TargetPackages) != 1 || scope.TargetPackages[0] != scope.TargetPackage {
			return fmt.Errorf("direct call index: invalid executable target scope package")
		}
	case AnalysisTargetModuleLibrary:
		if scope.TargetPackage != "" {
			return fmt.Errorf("direct call index: module library target scope retained executable package")
		}
	}
	if scope.MaxDepth < 1 {
		return fmt.Errorf("direct call index: invalid target scope depth %d", scope.MaxDepth)
	}
	if scope.EdgeLimit < 1 || scope.EdgeLimit > MaxDirectCallIndexEdges {
		return fmt.Errorf("direct call index: invalid target scope edge limit %d", scope.EdgeLimit)
	}
	return nil
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
	Scope        DirectCallIndexScope        `json:"scope,omitempty"`
	Modules      []DirectCallModule          `json:"modules"`
	Nodes        []DirectCallNode            `json:"nodes"`
	Edges        []DirectCallEdge            `json:"edges"`
	Frontiers    []DirectCallNodeFrontier    `json:"frontiers"`
	Coverage     DirectCallIndexCoverage     `json:"coverage"`
	SHA256       string                      `json:"sha256"`

	nodeLookup     map[string]int
	incomingLookup map[string][]int
	outgoingLookup map[string][]int
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
	snapshot.Scope.TargetPackages = cloneDirectCallSlice(index.Scope.TargetPackages)
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

func validDirectCallTargetIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, " \t\r\n")
}

func cloneDirectCallSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	result := make([]T, len(values))
	copy(result, values)
	return result
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
	index.incomingLookup = make(map[string][]int)
	index.outgoingLookup = make(map[string][]int)
	for position, node := range index.Nodes {
		index.nodeLookup[node.ID] = position
	}
	for position, edge := range index.Edges {
		index.incomingLookup[edge.CalleeID] = append(index.incomingLookup[edge.CalleeID], position)
		index.outgoingLookup[edge.CallerID] = append(index.outgoingLookup[edge.CallerID], position)
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
	if err := index.Scope.validate(); err != nil {
		return err
	}
	if index.Coverage.DepthBoundRepositoryCallsExcluded < 0 {
		return fmt.Errorf("direct call index: invalid depth-bound coverage")
	}
	if index.Coverage.EdgeLimitSafeDepth < 0 {
		return fmt.Errorf("direct call index: invalid edge-limit recovery depth")
	}
	if index.Coverage.EdgeLimitSafeDepth > 0 &&
		(index.State != DirectCallIndexUnavailable ||
			index.ClosedReason != DirectCallIndexClosedEdgeLimit ||
			index.Coverage.EdgeLimitSafeDepth >= index.Scope.MaxDepth) {
		return fmt.Errorf("direct call index: inconsistent edge-limit recovery depth")
	}
	if index.State == DirectCallIndexReady && index.Coverage.EdgeLimitSafeDepth != 0 {
		return fmt.Errorf("direct call index: ready index retained edge-limit recovery depth")
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
	if len(index.Edges) > index.Scope.EdgeLimit {
		return fmt.Errorf("direct call index: target graph exceeds configured edge limit")
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
	targetPackages := make(map[string]struct{}, len(index.Scope.TargetPackages))
	for _, packagePath := range index.Scope.TargetPackages {
		targetPackages[packagePath] = struct{}{}
	}
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
		if _, targetPackage := targetPackages[node.Package]; targetPackage {
			module := modules[node.ModuleID]
			if module.Path != index.Scope.TargetModulePath || module.Directory != index.Scope.TargetModuleDir {
				return fmt.Errorf("direct call index: target package node has mismatched module identity")
			}
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
	depthBoundRepositoryCalls := 0
	for _, frontier := range index.Frontiers {
		if previous != "" && frontier.CallerID <= previous {
			return fmt.Errorf("direct call index: frontiers are not unique canonical order")
		}
		previous = frontier.CallerID
		if _, ok := nodes[frontier.CallerID]; !ok {
			return fmt.Errorf("direct call index: frontier has unknown caller %q", frontier.CallerID)
		}
		if frontier.DynamicInvokesExcluded < 0 || frontier.NonStaticCallsExcluded < 0 ||
			frontier.ExternalCalleesExcluded < 0 || frontier.DepthBoundRepositoryCallsExcluded < 0 ||
			frontier.DynamicInvokesExcluded+frontier.NonStaticCallsExcluded+
				frontier.ExternalCalleesExcluded+frontier.DepthBoundRepositoryCallsExcluded == 0 {
			return fmt.Errorf("direct call index: invalid frontier for caller %q", frontier.CallerID)
		}
		dynamicInvokes += frontier.DynamicInvokesExcluded
		nonStaticCalls += frontier.NonStaticCallsExcluded
		externalCallees += frontier.ExternalCalleesExcluded
		depthBoundRepositoryCalls += frontier.DepthBoundRepositoryCallsExcluded
	}
	if dynamicInvokes > index.Coverage.DynamicInvokesExcluded ||
		nonStaticCalls > index.Coverage.NonStaticCallsExcluded ||
		externalCallees > index.Coverage.NonRepositoryCallsExcluded {
		return fmt.Errorf("direct call index: frontier exceeds global exclusion coverage")
	}
	if depthBoundRepositoryCalls != index.Coverage.DepthBoundRepositoryCallsExcluded {
		return fmt.Errorf("direct call index: per-caller depth frontier does not match global coverage")
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
	scope         DirectCallIndexScope
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
	entryCalls    *entryCallSidecar
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

func (builder *directCallIndexBuilder) setTargetScope(scope DirectCallIndexScope) {
	if builder == nil {
		return
	}
	builder.scope = scope
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
		return DirectCallIndex{}
	}
	index := DirectCallIndex{
		Version: DirectCallIndexVersion, State: builder.state, ClosedReason: builder.closedReason,
		Scenario: builder.scenario, Scope: builder.scope, Coverage: builder.coverage,
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
		Symbol: symbol, Package: packagePath, Exported: directCallFunctionExported(function),
		ModuleID: module.ID, ScenarioID: scenario.ID,
		Declaration: declaration, Body: body,
	}
	node.ID = stableDirectCallNodeID(node)
	return node, module, true
}

func directCallFunctionExported(function *ssa.Function) bool {
	if function == nil {
		return false
	}
	object := function.Object()
	return object != nil && object.Pkg() != nil && object.Exported()
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
