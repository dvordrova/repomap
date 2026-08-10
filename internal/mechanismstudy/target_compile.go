package mechanismstudy

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// TargetCompileInput is the single explicit target-bound Study compilation
// seam. Every authority value is supplied by its existing local producer; the
// compiler performs no package load, root discovery, or provider call.
type TargetCompileInput struct {
	Study          themestudy.StudyThemes
	Index          *surfacediscovery.DirectCallIndex
	Binding        Binding
	ReadingRoots   StudyReadingRootBindings
	AnalysisTarget analysistarget.Target
	TargetRoots    analysistarget.TargetRoots
}

type targetCompileContext struct {
	target             analysistarget.Target
	targetRootIDs      []string
	targetRootsSHA256  string
	targetRootsOmitted int
	studyRoots         StudyReadingRootBindings
}

type targetGraphSelection struct {
	graph            selectedGraph
	admittedReadings map[string]bool
	targetRootIDs    []string
	frontier         map[FrontierReason]int
}

type shortestConnector struct {
	found         bool
	distance      int
	ambiguous     bool
	graph         selectedGraph
	targetRootIDs []string
}

// CompileTargeted compiles exact target-rooted Study trails. Executable trails
// start at one of the selected package's exact main declarations and cross an
// exact D258 Study reading binding. Library trails start only at exact
// producer-owned exported declarations supplied by AnalysisTarget.
func CompileTargeted(input TargetCompileInput) (*Compilation, error) {
	target := input.AnalysisTarget.Snapshot()
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("mechanism study: validate analysis target: %w", err)
	}
	if err := analysistarget.ValidateExactRoots(target, input.Index, input.TargetRoots); err != nil {
		return nil, fmt.Errorf("mechanism study: validate exact target roots: %w", err)
	}
	if input.Study.Version != themestudy.StudyThemesVersion {
		return nil, fmt.Errorf("mechanism study: unsupported StudyThemes version %q", input.Study.Version)
	}
	binding := input.Binding
	binding.ContextKind = ContextStudy
	binding.ContextSHA256 = binding.StudyThemesSHA256
	if err := validateBinding(binding); err != nil {
		return nil, err
	}
	if input.Study.Revision == "" || input.Study.Revision != binding.RepositoryRevision {
		return nil, fmt.Errorf("mechanism study: Study revision does not match binding")
	}
	if input.Index == nil {
		return nil, fmt.Errorf("mechanism study: direct call index is nil")
	}
	if err := input.Index.Validate(); err != nil {
		return nil, fmt.Errorf("mechanism study: validate direct call index: %w", err)
	}

	sources, omittedCards := studySourceCards(input.Study)
	rootIDsBySpan, err := validateStudyReadingRootBindings(input.Study, sources, input.Index, input.ReadingRoots)
	if err != nil {
		return nil, err
	}
	targetRootIDs := make([]string, 0, len(input.TargetRoots.Roots))
	for _, root := range input.TargetRoots.Roots {
		targetRootIDs = append(targetRootIDs, root.NodeID)
	}
	context := &targetCompileContext{
		target: target, targetRootIDs: targetRootIDs,
		targetRootsSHA256:  input.TargetRoots.SHA256,
		targetRootsOmitted: input.TargetRoots.OmittedRoots,
		studyRoots:         copyStudyReadingRootBindings(input.ReadingRoots),
	}
	return compileSourcesInternal(
		sources, omittedCards, input.Index, binding, rootIDsBySpan, context,
	)
}

func collectTargetRootedGraph(
	index *surfacediscovery.DirectCallIndex,
	targetRootIDs []string,
	omittedTargetRoots int,
	readingRootIDs []string,
) targetGraphSelection {
	result := targetGraphSelection{
		graph: selectedGraph{
			nodes:   make(map[string]surfacediscovery.DirectCallNode),
			edges:   make(map[string]surfacediscovery.DirectCallEdge),
			omitted: make(map[string]FrontierReason),
		},
		admittedReadings: make(map[string]bool),
		frontier:         make(map[FrontierReason]int),
	}
	if omittedTargetRoots > 0 {
		result.frontier[FrontierShallowBound] += omittedTargetRoots
	}
	if index == nil || index.State != surfacediscovery.DirectCallIndexReady {
		return result
	}
	usedTargetRoots := make(map[string]struct{})
	seenReadings := make(map[string]struct{})
	for _, readingRootID := range orderedNodeIDs(index, readingRootIDs) {
		if readingRootID == "" {
			continue
		}
		if _, duplicate := seenReadings[readingRootID]; duplicate {
			continue
		}
		seenReadings[readingRootID] = struct{}{}

		connector := buildShortestConnector(index, targetRootIDs, readingRootID)
		if !connector.found {
			result.frontier[missingTargetConnectorReason(index)]++
			continue
		}
		if connector.distance > MaxEdgesPerMechanism {
			result.frontier[FrontierDepthBound]++
			continue
		}
		if len(connector.graph.nodes) > MaxNodesPerCard || len(connector.graph.edges) > MaxEdgesPerCard {
			if connector.ambiguous {
				result.frontier[FrontierAmbiguousConnector]++
			} else {
				result.frontier[FrontierShallowBound]++
			}
			continue
		}

		trail := addReadingContinuations(index, connector, readingRootID)
		if mergedNodeCount(result.graph.nodes, trail.nodes) > MaxNodesPerCard ||
			mergedEdgeCount(result.graph.edges, trail.edges) > MaxEdgesPerCard {
			result.frontier[FrontierShallowBound] += max(1, len(trail.edges))
			continue
		}
		mergeSelectedGraph(&result.graph, trail)
		result.admittedReadings[readingRootID] = true
		for _, rootID := range connector.targetRootIDs {
			usedTargetRoots[rootID] = struct{}{}
		}
	}
	for edgeID := range result.graph.edges {
		delete(result.graph.omitted, edgeID)
	}
	rootIDs := make([]string, 0, len(usedTargetRoots))
	for rootID := range usedTargetRoots {
		rootIDs = append(rootIDs, rootID)
	}
	result.targetRootIDs = orderedNodeIDs(index, rootIDs)
	return result
}

func missingTargetConnectorReason(index *surfacediscovery.DirectCallIndex) FrontierReason {
	if index != nil && index.Scope.TargetScoped() &&
		index.Coverage.DepthBoundRepositoryCallsExcluded > 0 {
		return FrontierDepthBound
	}
	return FrontierTargetUnreachable
}

func buildShortestConnector(
	index *surfacediscovery.DirectCallIndex,
	targetRootIDs []string,
	readingRootID string,
) shortestConnector {
	result := shortestConnector{graph: selectedGraph{
		nodes:   make(map[string]surfacediscovery.DirectCallNode),
		edges:   make(map[string]surfacediscovery.DirectCallEdge),
		omitted: make(map[string]FrontierReason),
	}}
	distance := make(map[string]int)
	predecessors := make(map[string][]surfacediscovery.DirectCallEdge)
	queue := make([]string, 0, len(targetRootIDs))
	for _, rootID := range orderedNodeIDs(index, targetRootIDs) {
		if _, exists := distance[rootID]; exists {
			continue
		}
		distance[rootID] = 0
		queue = append(queue, rootID)
	}
	foundDistance := -1
	for len(queue) > 0 {
		callerID := queue[0]
		queue = queue[1:]
		callerDistance := distance[callerID]
		if foundDistance >= 0 && callerDistance >= foundDistance {
			continue
		}
		for _, edge := range orderedAdjacentEdges(index, callerID) {
			calleeDistance := callerDistance + 1
			knownDistance, known := distance[edge.CalleeID]
			switch {
			case !known:
				distance[edge.CalleeID] = calleeDistance
				predecessors[edge.CalleeID] = []surfacediscovery.DirectCallEdge{edge}
				queue = append(queue, edge.CalleeID)
				if edge.CalleeID == readingRootID {
					foundDistance = calleeDistance
				}
			case knownDistance == calleeDistance:
				predecessors[edge.CalleeID] = append(predecessors[edge.CalleeID], edge)
			}
		}
	}
	readingDistance, found := distance[readingRootID]
	if !found {
		return result
	}
	result.found = true
	result.distance = readingDistance
	needed := map[string]struct{}{readingRootID: {}}
	stack := []string{readingRootID}
	for len(stack) > 0 {
		nodeID := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		preds := append([]surfacediscovery.DirectCallEdge(nil), predecessors[nodeID]...)
		sort.Slice(preds, func(i, j int) bool { return directCallEdgeSourceLess(index, preds[i], preds[j]) })
		if len(preds) > 1 {
			result.ambiguous = true
		}
		for _, edge := range preds {
			if distance[edge.CallerID]+1 != distance[nodeID] {
				continue
			}
			result.graph.edges[edge.ID] = edge
			needed[edge.CallerID] = struct{}{}
			if distance[edge.CallerID] > 0 {
				stack = appendUnique(stack, edge.CallerID)
			}
		}
	}
	for nodeID := range needed {
		if node, ok := index.Node(nodeID); ok {
			result.graph.nodes[nodeID] = node
		}
		if distance[nodeID] == 0 {
			result.targetRootIDs = append(result.targetRootIDs, nodeID)
		}
	}
	result.targetRootIDs = orderedNodeIDs(index, result.targetRootIDs)
	if len(result.targetRootIDs) > 1 {
		result.ambiguous = true
	}
	return result
}

func addReadingContinuations(
	index *surfacediscovery.DirectCallIndex,
	connector shortestConnector,
	readingRootID string,
) selectedGraph {
	graph := cloneSelectedGraph(connector.graph)
	remainingDepth := MaxEdgesPerMechanism - connector.distance
	if remainingDepth <= 0 {
		for _, edge := range orderedAdjacentEdges(index, readingRootID) {
			markOmitted(graph, edge.ID, FrontierDepthBound)
		}
		return graph
	}
	connectorNodes := make(map[string]struct{}, len(connector.graph.nodes))
	for nodeID := range connector.graph.nodes {
		connectorNodes[nodeID] = struct{}{}
	}
	// A breadth-first prefix can spend the complete continuation budget on a
	// wide reading root before reaching any second edge. Reserve only the
	// shortest source-ordered simple spine needed to make a two-edge mechanism
	// possible; if no complete spine exists, the ordinary bounded breadth stays
	// honest and the request planner will keep the card provider-free.
	requiredSpineEdges := max(0, 2-connector.distance)
	reservedSpine := shortestQualifyingContinuationSpine(
		index, readingRootID, connectorNodes, requiredSpineEdges, remainingDepth,
	)
	reservedEdgeIDs := make(map[string]struct{}, len(reservedSpine))
	for _, edge := range reservedSpine {
		node, _ := index.Node(edge.CalleeID) // proved present by the helper
		graph.nodes[edge.CalleeID] = node
		graph.edges[edge.ID] = edge
		delete(graph.omitted, edge.ID)
		reservedEdgeIDs[edge.ID] = struct{}{}
	}
	distance := map[string]int{readingRootID: 0}
	queue := []string{readingRootID}
	selectedContinuations := len(reservedEdgeIDs)
	for len(queue) > 0 {
		callerID := queue[0]
		queue = queue[1:]
		callerDistance := distance[callerID]
		for _, edge := range orderedAdjacentEdges(index, callerID) {
			if _, selected := graph.edges[edge.ID]; selected {
				if _, reserved := reservedEdgeIDs[edge.ID]; reserved {
					if _, seen := distance[edge.CalleeID]; !seen {
						distance[edge.CalleeID] = callerDistance + 1
						queue = append(queue, edge.CalleeID)
					}
				}
				continue
			}
			if callerDistance >= remainingDepth {
				markOmitted(graph, edge.ID, FrontierDepthBound)
				continue
			}
			if edge.CallerID == edge.CalleeID {
				markOmitted(graph, edge.ID, FrontierShallowBound)
				continue
			}
			if _, connectorAncestor := connectorNodes[edge.CalleeID]; connectorAncestor {
				markOmitted(graph, edge.ID, FrontierDepthBound)
				continue
			}
			if selectedContinuations >= MaxContinuationEdgesPerReading {
				markOmitted(graph, edge.ID, FrontierShallowBound)
				continue
			}
			node, ok := index.Node(edge.CalleeID)
			if !ok {
				markOmitted(graph, edge.ID, FrontierShallowBound)
				continue
			}
			graph.nodes[edge.CalleeID] = node
			graph.edges[edge.ID] = edge
			delete(graph.omitted, edge.ID)
			selectedContinuations++
			if _, seen := distance[edge.CalleeID]; !seen {
				distance[edge.CalleeID] = callerDistance + 1
				queue = append(queue, edge.CalleeID)
			}
		}
	}
	return graph
}

// shortestQualifyingContinuationSpine returns one complete source-ordered
// simple continuation of exactly needed edges. A nil result means that no such
// exact continuation exists; it never retains a partial prefix.
func shortestQualifyingContinuationSpine(
	index *surfacediscovery.DirectCallIndex,
	readingRootID string,
	connectorNodes map[string]struct{},
	needed int,
	remainingDepth int,
) []surfacediscovery.DirectCallEdge {
	if needed <= 0 {
		return []surfacediscovery.DirectCallEdge{}
	}
	if index == nil || needed > remainingDepth || needed > MaxContinuationEdgesPerReading {
		return nil
	}
	if _, ok := index.Node(readingRootID); !ok {
		return nil
	}
	visited := make(map[string]struct{}, len(connectorNodes)+needed)
	for nodeID := range connectorNodes {
		visited[nodeID] = struct{}{}
	}
	visited[readingRootID] = struct{}{}
	path := make([]surfacediscovery.DirectCallEdge, 0, needed)
	var walk func(string) bool
	walk = func(callerID string) bool {
		if len(path) == needed {
			return true
		}
		for _, edge := range orderedAdjacentEdges(index, callerID) {
			if _, cycle := visited[edge.CalleeID]; cycle {
				continue
			}
			if _, ok := index.Node(edge.CalleeID); !ok {
				continue
			}
			visited[edge.CalleeID] = struct{}{}
			path = append(path, edge)
			if walk(edge.CalleeID) {
				return true
			}
			path = path[:len(path)-1]
			delete(visited, edge.CalleeID)
		}
		return false
	}
	if !walk(readingRootID) {
		return nil
	}
	return append([]surfacediscovery.DirectCallEdge(nil), path...)
}

func orderedAdjacentEdges(index *surfacediscovery.DirectCallIndex, callerID string) []surfacediscovery.DirectCallEdge {
	edges := index.Outgoing(callerID)
	sort.Slice(edges, func(i, j int) bool { return directCallEdgeSourceLess(index, edges[i], edges[j]) })
	return edges
}

func orderedSelectedNodeIDs(
	index *surfacediscovery.DirectCallIndex,
	nodes map[string]surfacediscovery.DirectCallNode,
	targeted bool,
) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	if targeted {
		return orderedNodeIDs(index, ids)
	}
	sort.Strings(ids)
	return ids
}

func orderedSelectedEdgeIDs(
	index *surfacediscovery.DirectCallIndex,
	edges map[string]surfacediscovery.DirectCallEdge,
	targeted bool,
) []string {
	ids := make([]string, 0, len(edges))
	for id := range edges {
		ids = append(ids, id)
	}
	if !targeted {
		sort.Strings(ids)
		return ids
	}
	sort.Slice(ids, func(i, j int) bool {
		return directCallEdgeSourceLess(index, edges[ids[i]], edges[ids[j]])
	})
	return ids
}

func orderedNodeIDs(index *surfacediscovery.DirectCallIndex, ids []string) []string {
	result := append([]string(nil), ids...)
	sort.Slice(result, func(i, j int) bool {
		left, leftOK := index.Node(result[i])
		right, rightOK := index.Node(result[j])
		if !leftOK || !rightOK {
			return result[i] < result[j]
		}
		if locationLess(left.Declaration, right.Declaration) {
			return true
		}
		if locationLess(right.Declaration, left.Declaration) {
			return false
		}
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Symbol.Name != right.Symbol.Name {
			return left.Symbol.Name < right.Symbol.Name
		}
		return left.ID < right.ID
	})
	return result
}

func directCallEdgeSourceLess(
	index *surfacediscovery.DirectCallIndex,
	left, right surfacediscovery.DirectCallEdge,
) bool {
	if locationLess(left.RepresentativeCallsite, right.RepresentativeCallsite) {
		return true
	}
	if locationLess(right.RepresentativeCallsite, left.RepresentativeCallsite) {
		return false
	}
	leftCaller, _ := index.Node(left.CallerID)
	rightCaller, _ := index.Node(right.CallerID)
	if locationLess(leftCaller.Declaration, rightCaller.Declaration) {
		return true
	}
	if locationLess(rightCaller.Declaration, leftCaller.Declaration) {
		return false
	}
	leftCallee, _ := index.Node(left.CalleeID)
	rightCallee, _ := index.Node(right.CalleeID)
	if locationLess(leftCallee.Declaration, rightCallee.Declaration) {
		return true
	}
	if locationLess(rightCallee.Declaration, leftCallee.Declaration) {
		return false
	}
	if left.Invocation != right.Invocation {
		return left.Invocation < right.Invocation
	}
	return left.ID < right.ID
}

func locationLess(left, right surfacediscovery.Location) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Column < right.Column
}

func cloneSelectedGraph(source selectedGraph) selectedGraph {
	result := selectedGraph{
		nodes:   make(map[string]surfacediscovery.DirectCallNode, len(source.nodes)),
		edges:   make(map[string]surfacediscovery.DirectCallEdge, len(source.edges)),
		omitted: make(map[string]FrontierReason, len(source.omitted)),
	}
	mergeSelectedGraph(&result, source)
	return result
}

func mergeSelectedGraph(target *selectedGraph, source selectedGraph) {
	for id, node := range source.nodes {
		target.nodes[id] = node
	}
	for id, edge := range source.edges {
		target.edges[id] = edge
		delete(target.omitted, id)
	}
	for id, reason := range source.omitted {
		markOmitted(*target, id, reason)
	}
}

func mergedNodeCount(left, right map[string]surfacediscovery.DirectCallNode) int {
	count := len(left)
	for id := range right {
		if _, exists := left[id]; !exists {
			count++
		}
	}
	return count
}

func mergedEdgeCount(left, right map[string]surfacediscovery.DirectCallEdge) int {
	count := len(left)
	for id := range right {
		if _, exists := left[id]; !exists {
			count++
		}
	}
	return count
}

func copyStudyReadingRootBindings(source StudyReadingRootBindings) StudyReadingRootBindings {
	result := source
	result.Scenario = copyScenario(source.Scenario)
	result.Readings = append([]StudyReadingRootBinding(nil), source.Readings...)
	return result
}

func validateTargetCompilationIdentity(compilation *Compilation, digest compilationDigest) error {
	if compilation.TargetTrailVersion == 0 && compilation.AnalysisTargetRef == "" {
		if compilation.TargetRootsSHA256 != "" || digest.TargetTrailVersion != 0 ||
			digest.AnalysisTarget != nil || digest.TargetRootsSHA256 != "" ||
			digest.TargetRootsOmitted != 0 || digest.StudyReadingRoots != nil {
			return fmt.Errorf("mechanism study: untargeted compilation carries target authority")
		}
		return nil
	}
	if compilation.TargetTrailVersion != TargetTrailVersion ||
		digest.TargetTrailVersion != TargetTrailVersion || digest.AnalysisTarget == nil ||
		digest.StudyReadingRoots == nil || !validSHA256(compilation.TargetRootsSHA256) ||
		digest.TargetRootsSHA256 != compilation.TargetRootsSHA256 || digest.TargetRootsOmitted < 0 {
		return fmt.Errorf("mechanism study: incomplete analysis target authority")
	}
	if err := digest.AnalysisTarget.Validate(); err != nil ||
		compilation.AnalysisTargetRef != digest.AnalysisTarget.Ref {
		return fmt.Errorf("mechanism study: invalid analysis target authority")
	}
	roots := digest.StudyReadingRoots
	if roots.Version != StudyRootBindingsVersion ||
		roots.RepositoryRevision != compilation.Binding.RepositoryRevision ||
		roots.DirectCallIndexSHA256 != compilation.DirectCallIndexSHA256 ||
		!sameScenario(roots.Scenario, compilation.Scenario) {
		return fmt.Errorf("mechanism study: target compilation reading binding mismatch")
	}
	previous := ""
	for _, root := range roots.Readings {
		if strings.TrimSpace(root.CanonicalSpanID) == "" || strings.TrimSpace(root.NodeID) == "" ||
			(previous != "" && root.CanonicalSpanID <= previous) {
			return fmt.Errorf("mechanism study: invalid target compilation reading roots")
		}
		previous = root.CanonicalSpanID
	}
	canonicalTarget := digest.AnalysisTarget.Snapshot()
	canonicalRoots := copyStudyReadingRootBindings(*roots)
	if !reflect.DeepEqual(*digest.AnalysisTarget, canonicalTarget) || !reflect.DeepEqual(*roots, canonicalRoots) {
		return fmt.Errorf("mechanism study: mutable target compilation authority")
	}
	return nil
}

func validateDigestTargetRoots(targetRootIDs []string, authority cardAuthority) error {
	if len(targetRootIDs) != len(authority.targetRootRefs) {
		return fmt.Errorf("target root restoration count mismatch")
	}
	want := make([]string, 0, len(authority.targetRootRefs))
	for ref := range authority.targetRootRefs {
		nodeID := authority.nodeIDByRef[ref]
		if nodeID == "" {
			return fmt.Errorf("target root restoration mismatch")
		}
		want = append(want, nodeID)
	}
	sort.Slice(want, func(i, j int) bool {
		left := authority.nodeByRef[authority.nodeRefByID[want[i]]]
		right := authority.nodeByRef[authority.nodeRefByID[want[j]]]
		if locationLess(left.Declaration, right.Declaration) {
			return true
		}
		if locationLess(right.Declaration, left.Declaration) {
			return false
		}
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Symbol.Name != right.Symbol.Name {
			return left.Symbol.Name < right.Symbol.Name
		}
		return left.ID < right.ID
	})
	for position := range want {
		if targetRootIDs[position] != want[position] {
			return fmt.Errorf("target roots are not exact source order")
		}
	}
	return nil
}

func validateTargetRootProjection(card Card, authority cardAuthority) error {
	if len(card.TargetRootRefs) != len(authority.targetRootRefs) {
		return fmt.Errorf("target root projection count mismatch")
	}
	if len(card.TargetRootRefs) == 0 {
		if card.TargetRootRefs != nil {
			return fmt.Errorf("target root projection must be absent when empty")
		}
		return nil
	}
	seen := make(map[string]struct{}, len(card.TargetRootRefs))
	for _, ref := range card.TargetRootRefs {
		if !typedRef(ref, 'n') {
			return fmt.Errorf("invalid target root projection ref")
		}
		if _, exact := authority.targetRootRefs[ref]; !exact {
			return fmt.Errorf("target root projection authority mismatch")
		}
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("duplicate target root projection ref")
		}
		seen[ref] = struct{}{}
	}
	position := 0
	for _, node := range card.Nodes {
		if _, exact := authority.targetRootRefs[node.Ref]; !exact {
			continue
		}
		if card.TargetRootRefs[position] != node.Ref {
			return fmt.Errorf("target root projection is not in exact node order")
		}
		position++
	}
	if position != len(card.TargetRootRefs) {
		return fmt.Errorf("target root projection is incomplete")
	}
	return nil
}

func validateTargetCardShape(card Card, authority cardAuthority) error {
	nodes := make(map[string]struct{}, len(card.Nodes))
	for _, node := range card.Nodes {
		nodes[node.Ref] = struct{}{}
	}
	for rootRef := range authority.targetRootRefs {
		if _, exists := nodes[rootRef]; !exists {
			return fmt.Errorf("target card root is not an advertised node")
		}
	}
	readingRoots := make(map[string]struct{})
	for _, reading := range card.Readings {
		if reading.RootNodeRef != "" {
			readingRoots[reading.RootNodeRef] = struct{}{}
		}
	}
	if len(readingRoots) == 0 {
		if len(authority.targetRootRefs) != 0 || len(card.Nodes) != 0 || len(card.Edges) != 0 {
			return fmt.Errorf("prepared target card retained an unbound graph")
		}
		return nil
	}
	if len(authority.targetRootRefs) == 0 {
		return fmt.Errorf("target card has a reading without a target root")
	}
	adjacency := make(map[string][]string)
	reverse := make(map[string][]string)
	for _, edge := range card.Edges {
		adjacency[edge.CallerRef] = append(adjacency[edge.CallerRef], edge.CalleeRef)
		reverse[edge.CalleeRef] = append(reverse[edge.CalleeRef], edge.CallerRef)
	}
	fromTargets := boundedDistances(authority.targetRootRefs, adjacency, MaxEdgesPerMechanism)
	fromReadings := boundedDistances(readingRoots, adjacency, MaxEdgesPerMechanism)
	toReadings := boundedDistances(readingRoots, reverse, MaxEdgesPerMechanism)
	for readingRoot := range readingRoots {
		if _, reachable := fromTargets[readingRoot]; !reachable {
			return fmt.Errorf("target card reading is unreachable from its exact target")
		}
	}
	for targetRoot := range authority.targetRootRefs {
		if _, reachesReading := toReadings[targetRoot]; !reachesReading {
			return fmt.Errorf("target card advertises an unused target root")
		}
	}
	for _, node := range card.Nodes {
		if _, reachable := fromTargets[node.Ref]; !reachable {
			return fmt.Errorf("target card node is not reachable from its exact target")
		}
		_, beforeReading := toReadings[node.Ref]
		_, afterReading := fromReadings[node.Ref]
		if !beforeReading && !afterReading {
			return fmt.Errorf("target card node is outside every Study-crossing trail")
		}
	}
	for _, edge := range card.Edges {
		_, beforeReading := toReadings[edge.CalleeRef]
		_, afterReading := fromReadings[edge.CallerRef]
		if !beforeReading && !afterReading {
			return fmt.Errorf("target card edge is outside every Study-crossing trail")
		}
	}
	return nil
}

func boundedDistances(
	roots map[string]struct{},
	adjacency map[string][]string,
	maxDepth int,
) map[string]int {
	distance := make(map[string]int, len(roots))
	queue := make([]string, 0, len(roots))
	for root := range roots {
		distance[root] = 0
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if distance[current] >= maxDepth {
			continue
		}
		for _, next := range adjacency[current] {
			if _, seen := distance[next]; seen {
				continue
			}
			distance[next] = distance[current] + 1
			queue = append(queue, next)
		}
	}
	return distance
}
