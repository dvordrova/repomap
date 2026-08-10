package mechanismstudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

type direction uint8

const (
	directionOutgoing direction = iota
	directionIncoming
)

type graphLane struct {
	rootID    string
	direction direction
	levelOne  []string
	levelTwo  []string
}

type selectedGraph struct {
	nodes   map[string]surfacediscovery.DirectCallNode
	edges   map[string]surfacediscovery.DirectCallEdge
	omitted map[string]FrontierReason
}

type digestCard struct {
	Card          Card            `json:"card"`
	Ordinal       int             `json:"ordinal"`
	Canonical     string          `json:"canonical"`
	TargetRootIDs []string        `json:"target_root_ids,omitempty"`
	Nodes         []digestNode    `json:"nodes"`
	Edges         []digestEdge    `json:"edges"`
	Readings      []digestReading `json:"readings"`
}

type digestNode struct {
	Ref  string                          `json:"ref"`
	Node surfacediscovery.DirectCallNode `json:"node"`
}

type digestEdge struct {
	Ref  string                          `json:"ref"`
	Edge surfacediscovery.DirectCallEdge `json:"edge"`
}

type digestReading struct {
	Ref     string `json:"ref"`
	Ordinal int    `json:"ordinal"`
	RootID  string `json:"root_id,omitempty"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Symbol  string `json:"symbol"`
}

type compilationDigest struct {
	Version               int                       `json:"version"`
	RequestVersion        int                       `json:"request_version"`
	PromptVersion         string                    `json:"prompt_version"`
	StudyThemesVersion    string                    `json:"study_themes_version,omitempty"`
	ExactContextVersion   int                       `json:"exact_context_version,omitempty"`
	Binding               Binding                   `json:"binding"`
	DirectCallIndexSHA256 string                    `json:"direct_call_index_sha256"`
	Scenario              surfacediscovery.Scenario `json:"scenario"`
	TargetTrailVersion    int                       `json:"target_trail_version,omitempty"`
	AnalysisTarget        *analysistarget.Target    `json:"analysis_target,omitempty"`
	TargetRootsSHA256     string                    `json:"target_roots_sha256,omitempty"`
	TargetRootsOmitted    int                       `json:"target_roots_omitted,omitempty"`
	StudyReadingRoots     *StudyReadingRootBindings `json:"study_reading_roots,omitempty"`
	Cards                 []digestCard              `json:"cards"`
	OmittedCards          int                       `json:"omitted_cards"`
}

type sourceReading struct {
	Ordinal         int
	Label           string
	Path            string
	Line            int
	Symbol          string
	CanonicalSpanID string
}

type sourceCard struct {
	Ordinal            int
	Canonical          string
	Label              string
	Question           string
	Readings           []sourceReading
	UnsupportedReading int
}

type studyRootLocator struct {
	path string
	line int
}

// Compile binds final primary/direct Study readings to exact DirectCallIndex
// functions and collects the complete advertised depth-two graph. It performs
// no provider call and never loads packages or builds SSA.
func Compile(study themestudy.StudyThemes, index *surfacediscovery.DirectCallIndex, binding Binding) (*Compilation, error) {
	roots, err := BindStudyReadingRoots(study, index)
	if err != nil {
		return nil, err
	}
	return CompileWithStudyReadingRoots(study, index, binding, roots)
}

// BindStudyReadingRoots restores final direct Study readings with a
// backend-owned CanonicalSpanID to unique exact DirectCallIndex nodes. Symbol
// spelling is deliberately absent from this lookup: exact declaration
// location wins, otherwise exactly one containing body may bind. Zero or
// multiple locator matches remain unbound.
func BindStudyReadingRoots(
	study themestudy.StudyThemes,
	index *surfacediscovery.DirectCallIndex,
) (StudyReadingRootBindings, error) {
	if study.Version != themestudy.StudyThemesVersion {
		return StudyReadingRootBindings{}, fmt.Errorf("mechanism study: unsupported StudyThemes version %q", study.Version)
	}
	if strings.TrimSpace(study.Revision) == "" {
		return StudyReadingRootBindings{}, fmt.Errorf("mechanism study: missing Study revision")
	}
	if index == nil {
		return StudyReadingRootBindings{}, fmt.Errorf("mechanism study: direct call index is nil")
	}
	if err := index.Validate(); err != nil {
		return StudyReadingRootBindings{}, fmt.Errorf("mechanism study: validate direct call index: %w", err)
	}

	sources, _ := studySourceCards(study)
	locators, conflicted := studyReadingRootLocators(sources)

	result := StudyReadingRootBindings{
		Version: StudyRootBindingsVersion, RepositoryRevision: study.Revision,
		DirectCallIndexSHA256: index.SHA256, Scenario: copyScenario(index.Scenario),
		Readings: []StudyReadingRootBinding{},
	}
	spanIDs := make([]string, 0, len(locators))
	for spanID := range locators {
		spanIDs = append(spanIDs, spanID)
	}
	sort.Strings(spanIDs)
	for _, spanID := range spanIDs {
		if _, conflict := conflicted[spanID]; conflict {
			continue
		}
		location := locators[spanID]
		nodeID, resolved := uniqueDirectCallNodeAt(index, location.path, location.line)
		if !resolved {
			continue
		}
		result.Readings = append(result.Readings, StudyReadingRootBinding{
			CanonicalSpanID: spanID, NodeID: nodeID,
		})
	}
	return result, nil
}

// CompileWithStudyReadingRoots is the explicit producer/root-wiring seam. It
// accepts only a binding envelope produced for the same Study revision,
// DirectCallIndex digest and complete build scenario, and revalidates every
// supplied span-to-node locator before compiling the existing bounded graph.
func CompileWithStudyReadingRoots(
	study themestudy.StudyThemes,
	index *surfacediscovery.DirectCallIndex,
	binding Binding,
	roots StudyReadingRootBindings,
) (*Compilation, error) {
	if study.Version != themestudy.StudyThemesVersion {
		return nil, fmt.Errorf("mechanism study: unsupported StudyThemes version %q", study.Version)
	}
	binding.ContextKind = ContextStudy
	binding.ContextSHA256 = binding.StudyThemesSHA256
	if err := validateBinding(binding); err != nil {
		return nil, err
	}
	if study.Revision == "" || study.Revision != binding.RepositoryRevision {
		return nil, fmt.Errorf("mechanism study: Study revision does not match binding")
	}
	if index == nil {
		return nil, fmt.Errorf("mechanism study: direct call index is nil")
	}
	if err := index.Validate(); err != nil {
		return nil, fmt.Errorf("mechanism study: validate direct call index: %w", err)
	}

	sources, omittedCards := studySourceCards(study)
	rootIDsBySpan, err := validateStudyReadingRootBindings(study, sources, index, roots)
	if err != nil {
		return nil, err
	}
	return compileSources(sources, omittedCards, index, binding, rootIDsBySpan)
}

func studySourceCards(study themestudy.StudyThemes) ([]sourceCard, int) {
	cards := append([]themestudy.ThemeCard(nil), study.Cards...)
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].Ordinal != cards[j].Ordinal {
			return cards[i].Ordinal < cards[j].Ordinal
		}
		return cards[i].CanonicalID < cards[j].CanonicalID
	})
	omittedCards := 0
	if len(cards) > MaxCards {
		omittedCards = len(cards) - MaxCards
		cards = cards[:MaxCards]
	}

	sources := make([]sourceCard, 0, len(cards))
	for _, card := range cards {
		source := sourceCard{
			Ordinal: card.Ordinal, Canonical: card.CanonicalID,
			Label: card.FinalTitle, Question: card.FinalQuestion,
			UnsupportedReading: len(card.AlternateReadings),
		}
		for position, reading := range card.Readings {
			if reading.Fit != themestudy.FitDirect {
				source.UnsupportedReading++
				continue
			}
			source.Readings = append(source.Readings, sourceReading{
				Ordinal: position + 1,
				Label:   reading.Label, Path: reading.Path, Line: reading.Line, Symbol: reading.Symbol,
				CanonicalSpanID: reading.CanonicalSpanID,
			})
		}
		sources = append(sources, source)
	}
	return sources, omittedCards
}

// CompileContexts is the context-neutral experiment seam. The caller supplies
// exact roots directly, so no Scout, Adjudication, selector, or ranking stage
// is needed. Input order is the explicit context order and carries no graph
// authority.
func CompileContexts(contexts []ExactContext, index *surfacediscovery.DirectCallIndex, repository RepositoryBinding) (*Compilation, error) {
	if strings.TrimSpace(repository.RepositoryRevision) == "" || !validSHA256(repository.RepositoryFreshnessSHA256) {
		return nil, fmt.Errorf("mechanism study: invalid repository binding")
	}
	contextBytes, err := json.Marshal(contexts)
	if err != nil {
		return nil, fmt.Errorf("mechanism study: encode exact contexts: %w", err)
	}
	binding := Binding{
		ContextKind: ContextExplicit, ContextSHA256: sha256Hex(contextBytes),
		RepositoryRevision:        repository.RepositoryRevision,
		RepositoryFreshnessSHA256: repository.RepositoryFreshnessSHA256,
	}
	omittedCards := 0
	if len(contexts) > MaxCards {
		omittedCards = len(contexts) - MaxCards
		contexts = contexts[:MaxCards]
	}
	sources := make([]sourceCard, 0, len(contexts))
	for position, context := range contexts {
		source := sourceCard{
			Ordinal:   position + 1,
			Canonical: fmt.Sprintf("explicit-context-%d-%s", position+1, binding.ContextSHA256),
			Label:     context.Label, Question: context.Question,
		}
		for readingPosition, reading := range context.Readings {
			source.Readings = append(source.Readings, sourceReading{
				Ordinal: readingPosition + 1, Label: reading.Label, Path: reading.Path,
				Line: reading.Line, Symbol: reading.Symbol,
			})
		}
		sources = append(sources, source)
	}
	return compileSources(sources, omittedCards, index, binding, nil)
}

func compileSources(
	sources []sourceCard,
	omittedCards int,
	index *surfacediscovery.DirectCallIndex,
	binding Binding,
	rootIDsBySpan map[string]string,
) (*Compilation, error) {
	return compileSourcesInternal(sources, omittedCards, index, binding, rootIDsBySpan, nil)
}

func compileSourcesInternal(
	sources []sourceCard,
	omittedCards int,
	index *surfacediscovery.DirectCallIndex,
	binding Binding,
	rootIDsBySpan map[string]string,
	targetContext *targetCompileContext,
) (*Compilation, error) {
	if err := validateBinding(binding); err != nil {
		return nil, err
	}
	if index == nil {
		return nil, fmt.Errorf("mechanism study: direct call index is nil")
	}
	if err := index.Validate(); err != nil {
		return nil, fmt.Errorf("mechanism study: validate direct call index: %w", err)
	}
	compilation := &Compilation{
		Version: CompilationVersion, Binding: binding,
		DirectCallIndexSHA256: index.SHA256,
		Scenario:              copyScenario(index.Scenario),
		Cards:                 make([]Card, 0, len(sources)),
		OmittedCards:          omittedCards,
		authority:             make(map[string]cardAuthority, len(sources)),
	}
	digest := compilationDigest{
		Version: CompilationVersion, RequestVersion: RequestVersion,
		PromptVersion: PromptVersion,
		Binding:       binding, DirectCallIndexSHA256: index.SHA256,
		Scenario: copyScenario(index.Scenario), OmittedCards: omittedCards,
	}
	if targetContext != nil {
		target := targetContext.target.Snapshot()
		roots := copyStudyReadingRootBindings(targetContext.studyRoots)
		compilation.TargetTrailVersion = TargetTrailVersion
		compilation.AnalysisTargetRef = target.Ref
		compilation.TargetRootsSHA256 = targetContext.targetRootsSHA256
		digest.TargetTrailVersion = TargetTrailVersion
		digest.AnalysisTarget = &target
		digest.TargetRootsSHA256 = targetContext.targetRootsSHA256
		digest.TargetRootsOmitted = targetContext.targetRootsOmitted
		digest.StudyReadingRoots = &roots
	}
	if binding.ContextKind == ContextStudy {
		digest.StudyThemesVersion = themestudy.StudyThemesVersion
	} else {
		digest.ExactContextVersion = ExactContextVersion
	}
	nextReadingRef, nextNodeRef, nextEdgeRef := 1, 1, 1
	for cardPosition, sourceCard := range sources {
		cardRef := fmt.Sprintf("t%d", cardPosition+1)
		card, authority, digestEntry, nextRead, nextNode, nextEdge, err := compileCard(
			cardRef, sourceCard, index, rootIDsBySpan, targetContext,
			nextReadingRef, nextNodeRef, nextEdgeRef,
		)
		if err != nil {
			return nil, err
		}
		nextReadingRef, nextNodeRef, nextEdgeRef = nextRead, nextNode, nextEdge
		compilation.Cards = append(compilation.Cards, card)
		compilation.authority[cardRef] = authority
		digest.Cards = append(digest.Cards, digestEntry)
	}

	encoded, err := json.Marshal(digest)
	if err != nil {
		return nil, fmt.Errorf("mechanism study: encode catalog authority: %w", err)
	}
	compilation.catalogAuthorityJSON = string(encoded)
	compilation.CatalogSHA256 = sha256Hex(encoded)
	compilation.CatalogRef = "mc-" + compilation.CatalogSHA256[:16]
	if err := compilation.Validate(); err != nil {
		return nil, err
	}
	return compilation, nil
}

func compileCard(
	cardRef string,
	source sourceCard,
	index *surfacediscovery.DirectCallIndex,
	rootIDsBySpan map[string]string,
	targetContext *targetCompileContext,
	nextReadingRef, nextNodeRef, nextEdgeRef int,
) (Card, cardAuthority, digestCard, int, int, int, error) {
	if source.Ordinal <= 0 || strings.TrimSpace(source.Canonical) == "" {
		return Card{}, cardAuthority{}, digestCard{}, 0, 0, 0,
			fmt.Errorf("mechanism study: invalid context card identity")
	}
	frontierCounts := make(map[FrontierReason]int)
	selectedReadings := make([]sourceReading, 0, MaxDirectReadingsPerCard)
	for _, reading := range source.Readings {
		if len(selectedReadings) >= MaxDirectReadingsPerCard {
			frontierCounts[FrontierShallowBound]++
			continue
		}
		selectedReadings = append(selectedReadings, reading)
	}
	frontierCounts[FrontierUnsupportedReading] += source.UnsupportedReading

	type readingBinding struct {
		reading sourceReading
		ref     string
		rootID  string
	}
	bindings := make([]readingBinding, 0, len(selectedReadings))
	rootIDs := make([]string, 0, len(selectedReadings))
	seenRoots := make(map[string]struct{})
	for _, reading := range selectedReadings {
		ref := fmt.Sprintf("r%d", nextReadingRef)
		nextReadingRef++
		binding := readingBinding{reading: reading, ref: ref}
		if index.State != surfacediscovery.DirectCallIndexReady {
			frontierCounts[FrontierIndexUnavailable]++
		} else {
			resolution := surfacediscovery.DirectCallRootResolution{State: surfacediscovery.DirectCallRootUnresolved}
			if reading.CanonicalSpanID != "" {
				if nodeID := rootIDsBySpan[reading.CanonicalSpanID]; nodeID != "" {
					if node, ok := index.Node(nodeID); ok {
						resolution = index.ResolveRoot(reading.Path, reading.Line, node.Symbol.ID)
						if resolution.State == surfacediscovery.DirectCallRootResolved && resolution.Node.ID != nodeID {
							resolution = surfacediscovery.DirectCallRootResolution{State: surfacediscovery.DirectCallRootUnresolved}
						}
					}
				}
			} else {
				resolution = index.ResolveRoot(reading.Path, reading.Line, reading.Symbol)
			}
			if resolution.State != surfacediscovery.DirectCallRootResolved {
				frontierCounts[FrontierNoExactFunction]++
			} else {
				binding.rootID = resolution.Node.ID
				if _, duplicate := seenRoots[binding.rootID]; !duplicate {
					seenRoots[binding.rootID] = struct{}{}
					rootIDs = append(rootIDs, binding.rootID)
				}
			}
		}
		bindings = append(bindings, binding)
	}

	graph := selectedGraph{}
	targetRootIDs := []string{}
	if targetContext == nil {
		graph = collectGraph(index, rootIDs)
	} else {
		selection := collectTargetRootedGraph(
			index, targetContext.targetRootIDs, targetContext.targetRootsOmitted, rootIDs,
		)
		graph = selection.graph
		targetRootIDs = append(targetRootIDs, selection.targetRootIDs...)
		for reason, count := range selection.frontier {
			frontierCounts[reason] += count
		}
		for position := range bindings {
			if bindings[position].rootID == "" || selection.admittedReadings[bindings[position].rootID] {
				continue
			}
			bindings[position].rootID = ""
		}
	}
	for _, reason := range graph.omitted {
		frontierCounts[reason]++
	}
	for nodeID := range graph.nodes {
		frontier, ok := index.Frontier(nodeID)
		if !ok {
			continue
		}
		frontierCounts[FrontierDynamicInvoke] +=
			frontier.DynamicInvokesExcluded + frontier.NonStaticCallsExcluded
		frontierCounts[FrontierExternalCallee] += frontier.ExternalCalleesExcluded
		frontierCounts[FrontierDepthBound] += frontier.DepthBoundRepositoryCallsExcluded
	}

	nodeIDs := orderedSelectedNodeIDs(index, graph.nodes, targetContext != nil)
	nodeRefByID := make(map[string]string, len(nodeIDs))
	nodeIDByRef := make(map[string]string, len(nodeIDs))
	card := Card{
		Ref:      cardRef,
		Label:    safeLabel(source.Label, maxCardLabelRunes, "Context card "+cardRef[1:]),
		Question: safeLabel(source.Question, maxCardQuestionRunes, "Trace the exact direct-call structure."),
		Readings: make([]Reading, 0, len(bindings)),
		Nodes:    make([]Node, 0, len(nodeIDs)),
		Edges:    []Edge{},
	}
	digestEntry := digestCard{
		Card: card, Ordinal: source.Ordinal, Canonical: source.Canonical,
		TargetRootIDs: append([]string(nil), targetRootIDs...),
	}
	nodeByRef := make(map[string]surfacediscovery.DirectCallNode, len(nodeIDs))
	for _, id := range nodeIDs {
		node := graph.nodes[id]
		ref := fmt.Sprintf("n%d", nextNodeRef)
		nextNodeRef++
		nodeRefByID[id], nodeIDByRef[ref] = ref, id
		card.Nodes = append(card.Nodes, Node{
			Ref: ref, Label: nodeLabel(node),
		})
		nodeByRef[ref] = node
		digestEntry.Nodes = append(digestEntry.Nodes, digestNode{Ref: ref, Node: node})
	}
	if targetContext != nil && len(targetRootIDs) > 0 {
		card.TargetRootRefs = make([]string, 0, len(targetRootIDs))
		for _, nodeID := range targetRootIDs {
			ref := nodeRefByID[nodeID]
			if ref == "" {
				return Card{}, cardAuthority{}, digestCard{}, 0, 0, 0,
					fmt.Errorf("mechanism study: selected target root is absent from the card")
			}
			card.TargetRootRefs = append(card.TargetRootRefs, ref)
		}
	}
	readingRootByRef := make(map[string]string, len(bindings))
	readingOrdinalByRef := make(map[string]int, len(bindings))
	for _, binding := range bindings {
		rootRef := nodeRefByID[binding.rootID]
		card.Readings = append(card.Readings, Reading{
			Ref:         binding.ref,
			Label:       safeLabel(binding.reading.Label, maxReadingLabelRunes, "Direct reading"),
			RootNodeRef: rootRef,
		})
		if rootRef != "" {
			readingRootByRef[binding.ref] = rootRef
		}
		readingOrdinalByRef[binding.ref] = binding.reading.Ordinal
		digestEntry.Readings = append(digestEntry.Readings, digestReading{
			Ref: binding.ref, Ordinal: binding.reading.Ordinal, RootID: binding.rootID,
			Path: binding.reading.Path, Line: binding.reading.Line, Symbol: binding.reading.Symbol,
		})
	}

	edgeIDs := orderedSelectedEdgeIDs(index, graph.edges, targetContext != nil)
	edgeByRef := make(map[string]surfacediscovery.DirectCallEdge, len(edgeIDs))
	for _, id := range edgeIDs {
		edge := graph.edges[id]
		callerRef, callerOK := nodeRefByID[edge.CallerID]
		calleeRef, calleeOK := nodeRefByID[edge.CalleeID]
		if !callerOK || !calleeOK {
			return Card{}, cardAuthority{}, digestCard{}, 0, 0, 0,
				fmt.Errorf("mechanism study: selected edge has an unselected endpoint")
		}
		ref := fmt.Sprintf("e%d", nextEdgeRef)
		nextEdgeRef++
		card.Edges = append(card.Edges, Edge{
			Ref: ref, CallerRef: callerRef, CalleeRef: calleeRef,
			Invocation: edge.Invocation, WitnessCount: edge.WitnessCount,
		})
		edgeByRef[ref] = edge
		digestEntry.Edges = append(digestEntry.Edges, digestEdge{Ref: ref, Edge: edge})
	}
	card.Frontier = frontierFromCounts(frontierCounts)
	digestEntry.Card = card
	authority := cardAuthority{
		sourceOrdinal: source.Ordinal, sourceCanonical: source.Canonical,
		nodeIDByRef: nodeIDByRef, nodeRefByID: nodeRefByID, nodeByRef: nodeByRef,
		edgeByRef: edgeByRef, readingRootByRef: readingRootByRef,
		readingOrdinalByRef: readingOrdinalByRef,
		targetRootRefs:      make(map[string]struct{}, len(targetRootIDs)),
	}
	for _, ref := range card.TargetRootRefs {
		authority.targetRootRefs[ref] = struct{}{}
	}
	return card, authority, digestEntry, nextReadingRef, nextNodeRef, nextEdgeRef, nil
}

func collectGraph(index *surfacediscovery.DirectCallIndex, roots []string) selectedGraph {
	graph := selectedGraph{
		nodes:   make(map[string]surfacediscovery.DirectCallNode),
		edges:   make(map[string]surfacediscovery.DirectCallEdge),
		omitted: make(map[string]FrontierReason),
	}
	if index == nil || index.State != surfacediscovery.DirectCallIndexReady {
		return graph
	}
	for _, root := range roots {
		if node, ok := index.Node(root); ok {
			graph.nodes[root] = node
		}
	}
	lanes := make([]*graphLane, 0, len(roots)*2)
	for _, root := range roots {
		lanes = append(lanes,
			&graphLane{rootID: root, direction: directionOutgoing},
			&graphLane{rootID: root, direction: directionIncoming},
		)
	}

	// Depth one: one neighbor per reading/direction per round prevents a
	// high-degree root from consuming the complete card before siblings.
	for neighborPosition := 0; neighborPosition < MaxRootNeighborsPerDirection; neighborPosition++ {
		for _, lane := range lanes {
			edges := adjacent(index, lane.rootID, lane.direction)
			markBeyondNeighborLimit(graph, edges, MaxRootNeighborsPerDirection)
			if neighborPosition >= len(edges) {
				continue
			}
			other, selected := selectEdge(index, graph, edges[neighborPosition], lane.direction)
			if selected {
				lane.levelOne = appendUnique(lane.levelOne, other)
			}
		}
	}

	// Depth two is balanced first across each parent slot, then across lanes,
	// then across each parent's neighbor slot.
	for neighborPosition := 0; neighborPosition < MaxContinuationNeighborsPerDirection; neighborPosition++ {
		for parentPosition := 0; parentPosition < MaxRootNeighborsPerDirection; parentPosition++ {
			for _, lane := range lanes {
				if parentPosition >= len(lane.levelOne) {
					continue
				}
				parent := lane.levelOne[parentPosition]
				edges := adjacent(index, parent, lane.direction)
				markBeyondNeighborLimit(graph, edges, MaxContinuationNeighborsPerDirection)
				if neighborPosition >= len(edges) {
					continue
				}
				other, selected := selectEdge(index, graph, edges[neighborPosition], lane.direction)
				if selected {
					lane.levelTwo = appendUnique(lane.levelTwo, other)
				}
			}
		}
	}

	// Anything beyond the selected absolute depth-two frontier stays typed and
	// counted. Exact edges selected through another reading are never omitted.
	for _, lane := range lanes {
		for _, nodeID := range lane.levelTwo {
			for _, edge := range adjacent(index, nodeID, lane.direction) {
				markOmitted(graph, edge.ID, FrontierDepthBound)
			}
		}
	}
	for id := range graph.edges {
		delete(graph.omitted, id)
	}
	return graph
}

func adjacent(index *surfacediscovery.DirectCallIndex, nodeID string, lane direction) []surfacediscovery.DirectCallEdge {
	if lane == directionIncoming {
		return index.Incoming(nodeID)
	}
	return index.Outgoing(nodeID)
}

func markBeyondNeighborLimit(graph selectedGraph, edges []surfacediscovery.DirectCallEdge, limit int) {
	for position := limit; position < len(edges); position++ {
		markOmitted(graph, edges[position].ID, FrontierShallowBound)
	}
}

func selectEdge(
	index *surfacediscovery.DirectCallIndex,
	graph selectedGraph,
	edge surfacediscovery.DirectCallEdge,
	lane direction,
) (string, bool) {
	if _, selected := graph.edges[edge.ID]; selected {
		if lane == directionIncoming {
			return edge.CallerID, true
		}
		return edge.CalleeID, true
	}
	other := edge.CalleeID
	if lane == directionIncoming {
		other = edge.CallerID
	}
	if _, selected := graph.nodes[other]; !selected && len(graph.nodes) >= MaxNodesPerCard {
		markOmitted(graph, edge.ID, FrontierShallowBound)
		return other, false
	}
	if len(graph.edges) >= MaxEdgesPerCard {
		markOmitted(graph, edge.ID, FrontierShallowBound)
		return other, false
	}
	node, ok := index.Node(other)
	if !ok {
		markOmitted(graph, edge.ID, FrontierShallowBound)
		return other, false
	}
	graph.nodes[other] = node
	graph.edges[edge.ID] = edge
	delete(graph.omitted, edge.ID)
	return other, true
}

func markOmitted(graph selectedGraph, edgeID string, reason FrontierReason) {
	if edgeID == "" {
		return
	}
	if _, selected := graph.edges[edgeID]; selected {
		return
	}
	if _, recorded := graph.omitted[edgeID]; !recorded {
		graph.omitted[edgeID] = reason
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func frontierFromCounts(counts map[FrontierReason]int) []Frontier {
	reasons := make([]string, 0, len(counts))
	for reason, count := range counts {
		if count > 0 {
			reasons = append(reasons, string(reason))
		}
	}
	sort.Strings(reasons)
	if len(reasons) == 0 {
		return nil
	}
	if len(reasons) > MaxFrontierRecordsPerCard {
		reasons = reasons[:MaxFrontierRecordsPerCard]
	}
	frontier := make([]Frontier, 0, len(reasons))
	for _, raw := range reasons {
		reason := FrontierReason(raw)
		frontier = append(frontier, Frontier{Reason: reason, Count: counts[reason]})
	}
	return frontier
}

func validateBinding(binding Binding) error {
	if !validSHA256(binding.ContextSHA256) || !validSHA256(binding.RepositoryFreshnessSHA256) ||
		strings.TrimSpace(binding.RepositoryRevision) == "" {
		return fmt.Errorf("mechanism study: invalid exact binding")
	}
	switch binding.ContextKind {
	case ContextStudy:
		if !validSHA256(binding.StudyThemesSHA256) || !validSHA256(binding.AtlasStudyCatalogSHA256) ||
			binding.ContextSHA256 != binding.StudyThemesSHA256 {
			return fmt.Errorf("mechanism study: invalid Study binding")
		}
	case ContextExplicit:
		if binding.StudyThemesSHA256 != "" || binding.AtlasStudyCatalogSHA256 != "" {
			return fmt.Errorf("mechanism study: explicit context carries Study binding")
		}
	default:
		return fmt.Errorf("mechanism study: invalid context kind %q", binding.ContextKind)
	}
	return nil
}

func validateStudyReadingRootBindings(
	study themestudy.StudyThemes,
	sources []sourceCard,
	index *surfacediscovery.DirectCallIndex,
	roots StudyReadingRootBindings,
) (map[string]string, error) {
	if roots.Version != StudyRootBindingsVersion || roots.RepositoryRevision != study.Revision ||
		roots.DirectCallIndexSHA256 != index.SHA256 || !sameScenario(roots.Scenario, index.Scenario) {
		return nil, fmt.Errorf("mechanism study: Study reading root binding identity mismatch")
	}
	allowed, conflicted := studyReadingRootLocators(sources)
	result := make(map[string]string, len(roots.Readings))
	previous := ""
	for _, root := range roots.Readings {
		if strings.TrimSpace(root.CanonicalSpanID) == "" || strings.TrimSpace(root.NodeID) == "" ||
			(previous != "" && root.CanonicalSpanID <= previous) {
			return nil, fmt.Errorf("mechanism study: invalid Study reading root binding order")
		}
		previous = root.CanonicalSpanID
		location, advertised := allowed[root.CanonicalSpanID]
		_, conflict := conflicted[root.CanonicalSpanID]
		if !advertised || conflict {
			return nil, fmt.Errorf("mechanism study: Study reading root binding is not an exact advertised locator")
		}
		nodeID, resolved := uniqueDirectCallNodeAt(index, location.path, location.line)
		if !resolved || nodeID != root.NodeID {
			return nil, fmt.Errorf("mechanism study: Study reading root binding does not match the unique exact locator")
		}
		result[root.CanonicalSpanID] = root.NodeID
	}
	return result, nil
}

func studyReadingRootLocators(sources []sourceCard) (map[string]studyRootLocator, map[string]struct{}) {
	locators := make(map[string]studyRootLocator)
	conflicted := make(map[string]struct{})
	for _, card := range sources {
		for position, reading := range card.Readings {
			if position >= MaxDirectReadingsPerCard {
				break
			}
			spanID := strings.TrimSpace(reading.CanonicalSpanID)
			if spanID == "" || spanID != reading.CanonicalSpanID ||
				!validStudyRootLocator(reading.Path, reading.Line) {
				continue
			}
			current := studyRootLocator{path: reading.Path, line: reading.Line}
			if previous, exists := locators[spanID]; exists && previous != current {
				conflicted[spanID] = struct{}{}
				continue
			}
			locators[spanID] = current
		}
	}
	return locators, conflicted
}

func uniqueDirectCallNodeAt(index *surfacediscovery.DirectCallIndex, path string, line int) (string, bool) {
	if index == nil || index.State != surfacediscovery.DirectCallIndexReady || !validStudyRootLocator(path, line) {
		return "", false
	}
	declarations := make([]string, 0, 1)
	for _, node := range index.Nodes {
		if node.Declaration.Path == path && node.Declaration.Line == line {
			declarations = append(declarations, node.ID)
		}
	}
	if len(declarations) == 1 {
		return declarations[0], true
	}
	if len(declarations) > 1 {
		return "", false
	}
	containing := make([]string, 0, 1)
	for _, node := range index.Nodes {
		if node.Body.Start.Path == path && node.Body.End.Path == path &&
			line >= node.Body.Start.Line && line <= node.Body.End.Line {
			containing = append(containing, node.ID)
		}
	}
	if len(containing) == 1 {
		return containing[0], true
	}
	return "", false
}

func validStudyRootLocator(path string, line int) bool {
	return line > 0 && fs.ValidPath(path)
}

func sameScenario(left, right surfacediscovery.Scenario) bool {
	return left.ID == right.ID && left.GOOS == right.GOOS && left.GOARCH == right.GOARCH &&
		left.GoFlags == right.GoFlags && slices.Equal(left.Tags, right.Tags)
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func copyScenario(scenario surfacediscovery.Scenario) surfacediscovery.Scenario {
	scenario.Tags = append([]string(nil), scenario.Tags...)
	return scenario
}

// safeLabel keeps model-visible context useful without copying locators,
// canonical identities, control characters, or arbitrarily long prose.
func safeLabel(value string, limit int, fallback string) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if value == "" || containsPathLikeToken(value) || looksLikeCanonicalIdentity(value) {
		value = fallback
	}
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(r)
	}
	value = strings.TrimSpace(strings.Join(strings.Fields(builder.String()), " "))
	if value == "" {
		value = fallback
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func containsPathLikeToken(value string) bool {
	if strings.ContainsAny(value, "/\\") {
		return true
	}
	extensions := []string{
		".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".rs", ".java", ".kt",
		".c", ".h", ".cpp", ".cs", ".rb", ".php", ".md", ".yaml", ".yml",
		".json", ".toml", ".xml", ".proto", ".sql", ".sh",
	}
	for _, field := range strings.Fields(value) {
		field = strings.ToLower(strings.Trim(field, "`'\"()[]{}<>,;.!?"))
		for _, extension := range extensions {
			if strings.HasSuffix(field, extension) {
				return true
			}
		}
		if colon := strings.LastIndexByte(field, ':'); colon > 0 && colon < len(field)-1 {
			prefixHasLetter := false
			for _, r := range field[:colon] {
				if unicode.IsLetter(r) {
					prefixHasLetter = true
					break
				}
			}
			line := field[colon+1:]
			allDigits := true
			for _, r := range line {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if prefixHasLetter && allDigits {
				return true
			}
		}
	}
	return false
}

func looksLikeCanonicalIdentity(value string) bool {
	compact := strings.ReplaceAll(strings.ReplaceAll(value, "-", ""), "_", "")
	if len(compact) < 32 {
		return false
	}
	for _, r := range compact {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func nodeLabel(node surfacediscovery.DirectCallNode) string {
	packageLeaf := node.Package
	if separator := strings.LastIndex(packageLeaf, "/"); separator >= 0 {
		packageLeaf = packageLeaf[separator+1:]
	}
	packageLeaf = safeLabel(packageLeaf, 32, "package")
	name := safeLabel(node.Symbol.Name, 56, "function")
	return safeLabel(packageLeaf+" · "+name, maxNodeLabelRunes, name)
}
