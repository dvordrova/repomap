package mechanismstudy

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/themestudy"
)

// Validate proves the provider-free compilation still matches its exact
// private authority and every published ceiling.
func (compilation *Compilation) Validate() error {
	if compilation == nil {
		return fmt.Errorf("mechanism study: compilation is nil")
	}
	if compilation.Version != CompilationVersion || !validSHA256(compilation.CatalogSHA256) ||
		!strings.HasPrefix(compilation.CatalogRef, "mc-") ||
		compilation.CatalogRef != "mc-"+compilation.CatalogSHA256[:16] {
		return fmt.Errorf("mechanism study: invalid compilation identity")
	}
	if err := validateBinding(compilation.Binding); err != nil {
		return err
	}
	if !validSHA256(compilation.DirectCallIndexSHA256) || compilation.Scenario.ID == "" ||
		compilation.Scenario.GOOS == "" || compilation.Scenario.GOARCH == "" ||
		!sort.StringsAreSorted(compilation.Scenario.Tags) {
		return fmt.Errorf("mechanism study: invalid direct-call binding")
	}
	if len(compilation.Cards) > MaxCards || compilation.OmittedCards < 0 ||
		len(compilation.authority) != len(compilation.Cards) {
		return fmt.Errorf("mechanism study: invalid card bounds")
	}
	if compilation.catalogAuthorityJSON == "" ||
		sha256Hex([]byte(compilation.catalogAuthorityJSON)) != compilation.CatalogSHA256 {
		return fmt.Errorf("mechanism study: catalog authority binding mismatch")
	}
	var digest compilationDigest
	if err := json.Unmarshal([]byte(compilation.catalogAuthorityJSON), &digest); err != nil {
		return fmt.Errorf("mechanism study: decode catalog authority: %w", err)
	}
	if digest.Version != CompilationVersion || digest.RequestVersion != RequestVersion ||
		digest.PromptVersion != PromptVersion || !reflect.DeepEqual(digest.Binding, compilation.Binding) ||
		digest.DirectCallIndexSHA256 != compilation.DirectCallIndexSHA256 ||
		!reflect.DeepEqual(digest.Scenario, compilation.Scenario) ||
		digest.OmittedCards != compilation.OmittedCards || len(digest.Cards) != len(compilation.Cards) {
		return fmt.Errorf("mechanism study: catalog authority does not restore compilation")
	}
	if compilation.Binding.ContextKind == ContextStudy {
		if digest.StudyThemesVersion != themestudy.StudyThemesVersion || digest.ExactContextVersion != 0 {
			return fmt.Errorf("mechanism study: invalid Study catalog contract")
		}
	} else if digest.ExactContextVersion != ExactContextVersion || digest.StudyThemesVersion != "" {
		return fmt.Errorf("mechanism study: invalid explicit-context catalog contract")
	}
	seenCards := make(map[string]struct{}, len(compilation.Cards))
	seenRefs := make(map[string]struct{})
	seenCanonical := make(map[string]struct{}, len(compilation.Cards))
	previousOrdinal := 0
	for position, card := range compilation.Cards {
		if _, duplicate := seenCards[card.Ref]; duplicate || !typedRef(card.Ref, 't') {
			return fmt.Errorf("mechanism study: invalid card ref %q", card.Ref)
		}
		seenCards[card.Ref] = struct{}{}
		authority, ok := compilation.authority[card.Ref]
		if !ok {
			return fmt.Errorf("mechanism study: missing card authority %q", card.Ref)
		}
		if authority.sourceOrdinal <= previousOrdinal || strings.TrimSpace(authority.sourceCanonical) == "" {
			return fmt.Errorf("mechanism study: invalid source card authority")
		}
		if _, duplicate := seenCanonical[authority.sourceCanonical]; duplicate {
			return fmt.Errorf("mechanism study: duplicate source card authority")
		}
		seenCanonical[authority.sourceCanonical] = struct{}{}
		previousOrdinal = authority.sourceOrdinal
		if err := validateCard(card, authority, seenRefs); err != nil {
			return fmt.Errorf("mechanism study: card %s: %w", card.Ref, err)
		}
		if err := validateDigestCard(digest.Cards[position], card, authority); err != nil {
			return fmt.Errorf("mechanism study: card %s authority digest: %w", card.Ref, err)
		}
	}
	return nil
}

func validateDigestCard(digest digestCard, card Card, authority cardAuthority) error {
	if digest.Ordinal <= 0 || digest.Ordinal != authority.sourceOrdinal ||
		strings.TrimSpace(digest.Canonical) == "" || digest.Canonical != authority.sourceCanonical ||
		!reflect.DeepEqual(digest.Card, card) ||
		len(digest.Nodes) != len(card.Nodes) || len(digest.Edges) != len(card.Edges) ||
		len(digest.Readings) != len(card.Readings) {
		return fmt.Errorf("public card mismatch")
	}
	for position, node := range digest.Nodes {
		if node.Ref != card.Nodes[position].Ref || node.Node.ID == "" ||
			authority.nodeIDByRef[node.Ref] != node.Node.ID ||
			authority.nodeRefByID[node.Node.ID] != node.Ref ||
			!reflect.DeepEqual(authority.nodeByRef[node.Ref], node.Node) ||
			!validPublicationNode(node.Node, node.Node.ScenarioID) {
			return fmt.Errorf("node restoration mismatch")
		}
	}
	for position, edge := range digest.Edges {
		exact, ok := authority.edgeByRef[edge.Ref]
		if edge.Ref != card.Edges[position].Ref || edge.Edge.ID == "" || !ok ||
			!reflect.DeepEqual(exact, edge.Edge) || !validPublicationEdge(edge.Edge, edge.Edge.ScenarioID) ||
			authority.nodeRefByID[edge.Edge.CallerID] != card.Edges[position].CallerRef ||
			authority.nodeRefByID[edge.Edge.CalleeID] != card.Edges[position].CalleeRef {
			return fmt.Errorf("edge restoration mismatch")
		}
	}
	seenOrdinals := make(map[int]struct{}, len(digest.Readings))
	for position, reading := range digest.Readings {
		if reading.Ref != card.Readings[position].Ref || reading.Ordinal <= 0 ||
			authority.readingOrdinalByRef[reading.Ref] != reading.Ordinal || reading.Path == "" ||
			reading.Line <= 0 {
			return fmt.Errorf("reading restoration mismatch")
		}
		if _, duplicate := seenOrdinals[reading.Ordinal]; duplicate {
			return fmt.Errorf("duplicate reading ordinal")
		}
		seenOrdinals[reading.Ordinal] = struct{}{}
		rootRef := authority.nodeRefByID[reading.RootID]
		if reading.RootID == "" {
			rootRef = ""
		}
		if card.Readings[position].RootNodeRef != rootRef || authority.readingRootByRef[reading.Ref] != rootRef {
			return fmt.Errorf("reading root restoration mismatch")
		}
	}
	return nil
}

func validateCard(card Card, authority cardAuthority, globalRefs map[string]struct{}) error {
	if !publicLabelValid(card.Label, maxCardLabelRunes) || !publicLabelValid(card.Question, maxCardQuestionRunes) ||
		len(card.Readings) > MaxDirectReadingsPerCard ||
		len(card.Nodes) > MaxNodesPerCard || len(card.Edges) > MaxEdgesPerCard ||
		len(card.Frontier) > MaxFrontierRecordsPerCard {
		return fmt.Errorf("invalid public bounds")
	}
	if len(authority.nodeIDByRef) != len(card.Nodes) || len(authority.nodeRefByID) != len(card.Nodes) ||
		len(authority.nodeByRef) != len(card.Nodes) || len(authority.edgeByRef) != len(card.Edges) ||
		len(authority.readingOrdinalByRef) != len(card.Readings) {
		return fmt.Errorf("invalid restoration authority bounds")
	}
	nodes := make(map[string]struct{}, len(card.Nodes))
	for _, node := range card.Nodes {
		if !typedRef(node.Ref, 'n') || !publicLabelValid(node.Label, maxNodeLabelRunes) {
			return fmt.Errorf("invalid node")
		}
		if _, duplicate := globalRefs[node.Ref]; duplicate {
			return fmt.Errorf("duplicate request-local ref %q", node.Ref)
		}
		globalRefs[node.Ref] = struct{}{}
		nodes[node.Ref] = struct{}{}
		if authority.nodeIDByRef[node.Ref] == "" || authority.nodeRefByID[authority.nodeIDByRef[node.Ref]] != node.Ref {
			return fmt.Errorf("node authority mismatch")
		}
		exact, ok := authority.nodeByRef[node.Ref]
		if !ok || exact.ID != authority.nodeIDByRef[node.Ref] {
			return fmt.Errorf("node restoration authority mismatch")
		}
	}
	for _, reading := range card.Readings {
		if !typedRef(reading.Ref, 'r') || !publicLabelValid(reading.Label, maxReadingLabelRunes) {
			return fmt.Errorf("invalid reading")
		}
		if _, duplicate := globalRefs[reading.Ref]; duplicate {
			return fmt.Errorf("duplicate request-local ref %q", reading.Ref)
		}
		globalRefs[reading.Ref] = struct{}{}
		if reading.RootNodeRef != "" {
			if _, ok := nodes[reading.RootNodeRef]; !ok || authority.readingRootByRef[reading.Ref] != reading.RootNodeRef {
				return fmt.Errorf("reading root authority mismatch")
			}
		} else if authority.readingRootByRef[reading.Ref] != "" {
			return fmt.Errorf("hidden reading root authority")
		}
		if authority.readingOrdinalByRef[reading.Ref] <= 0 {
			return fmt.Errorf("missing reading ordinal authority")
		}
	}
	seenEdges := make(map[string]struct{}, len(card.Edges))
	for _, edge := range card.Edges {
		if !typedRef(edge.Ref, 'e') || !edge.Invocation.Valid() || edge.WitnessCount <= 0 {
			return fmt.Errorf("invalid edge")
		}
		if _, duplicate := globalRefs[edge.Ref]; duplicate {
			return fmt.Errorf("duplicate request-local ref %q", edge.Ref)
		}
		globalRefs[edge.Ref] = struct{}{}
		if _, ok := nodes[edge.CallerRef]; !ok {
			return fmt.Errorf("unknown edge caller")
		}
		if _, ok := nodes[edge.CalleeRef]; !ok {
			return fmt.Errorf("unknown edge callee")
		}
		if _, duplicate := seenEdges[edge.Ref]; duplicate {
			return fmt.Errorf("duplicate edge")
		}
		seenEdges[edge.Ref] = struct{}{}
		exact, ok := authority.edgeByRef[edge.Ref]
		if !ok || authority.nodeRefByID[exact.CallerID] != edge.CallerRef ||
			authority.nodeRefByID[exact.CalleeID] != edge.CalleeRef ||
			exact.Invocation != edge.Invocation || exact.WitnessCount != edge.WitnessCount {
			return fmt.Errorf("edge authority mismatch")
		}
	}
	previous := ""
	for _, frontier := range card.Frontier {
		if !frontier.Reason.valid() || frontier.Count <= 0 || (previous != "" && string(frontier.Reason) <= previous) {
			return fmt.Errorf("invalid frontier")
		}
		previous = string(frontier.Reason)
	}
	return nil
}

func typedRef(value string, prefix byte) bool {
	if len(value) < 2 || value[0] != prefix {
		return false
	}
	for position := 1; position < len(value); position++ {
		if value[position] < '0' || value[position] > '9' {
			return false
		}
	}
	return value[1] != '0'
}

// PlanRequestBatches returns the smallest deterministic production API seam.
// It never plans more than MaxProviderCalls independent semantic requests.
// Any otherwise eligible suffix remains explicit and prepared instead of
// turning the planner ceiling into a terminal run error.
func PlanRequestBatches(compilation *Compilation) (RequestPlan, error) {
	if err := compilation.Validate(); err != nil {
		return RequestPlan{}, err
	}
	return planRequestBatchesUnchecked(compilation)
}

func planRequestBatchesUnchecked(compilation *Compilation) (RequestPlan, error) {
	return planRequestBatchesWithCallLimit(compilation, MaxProviderCalls)
}

func planRequestBatchesWithCallLimit(compilation *Compilation, callLimit int) (RequestPlan, error) {
	if callLimit < 0 || callLimit > MaxProviderCalls {
		return RequestPlan{}, fmt.Errorf("mechanism study: invalid provider call limit")
	}
	eligible := make([]Card, 0, len(compilation.Cards))
	for _, card := range compilation.Cards {
		if cardCanContainMechanism(card) {
			eligible = append(eligible, copyCard(card))
		}
	}
	if len(eligible) == 0 {
		return RequestPlan{Batches: []RequestBatch{}, UnrequestedCardRefs: []string{}}, nil
	}

	plan := RequestPlan{Batches: []RequestBatch{}, UnrequestedCardRefs: []string{}}
	for len(eligible) > 0 && len(plan.Batches) < callLimit {
		batchNumber := len(plan.Batches) + 1
		request := Request{
			Version: RequestVersion, PromptVersion: PromptVersion,
			CatalogRef: compilation.CatalogRef, CatalogSHA256: compilation.CatalogSHA256,
			RequestRef: fmt.Sprintf("q%d", batchNumber), Cards: []Card{},
		}
		nodes, edges := 0, 0
		for len(eligible) > 0 && len(request.Cards) < MaxCardsPerRequest {
			candidate := eligible[0]
			if nodes+len(candidate.Nodes) > MaxNodesPerRequest || edges+len(candidate.Edges) > MaxEdgesPerRequest {
				break
			}
			trial := request
			trial.Cards = append(append([]Card(nil), request.Cards...), candidate)
			encoded, err := json.Marshal(trial)
			if err != nil {
				return RequestPlan{}, fmt.Errorf("mechanism study: encode request: %w", err)
			}
			if len(encoded) > MaxRequestBytes {
				break
			}
			request = trial
			nodes += len(candidate.Nodes)
			edges += len(candidate.Edges)
			eligible = eligible[1:]
		}
		if len(request.Cards) == 0 {
			return RequestPlan{}, fmt.Errorf("mechanism study: one bounded card exceeds request envelope")
		}
		batch, err := makeRequestBatch(request)
		if err != nil {
			return RequestPlan{}, err
		}
		plan.Batches = append(plan.Batches, batch)
	}
	for _, card := range eligible {
		plan.UnrequestedCardRefs = append(plan.UnrequestedCardRefs, card.Ref)
	}
	return plan, nil
}

// BuildRequestBatches preserves the experiment-facing v2 API. Production
// callers use PlanRequestBatches so the prepared suffix is visible.
func BuildRequestBatches(compilation *Compilation) ([]RequestBatch, error) {
	plan, err := PlanRequestBatches(compilation)
	if err != nil {
		return nil, err
	}
	return append([]RequestBatch(nil), plan.Batches...), nil
}

// Validate proves a plan is the exact deterministic prefix/suffix partition
// for one current compilation, including the private request seals.
func (plan RequestPlan) Validate(compilation *Compilation) error {
	if err := compilation.Validate(); err != nil {
		return err
	}
	want, err := planRequestBatchesUnchecked(compilation)
	if err != nil {
		return err
	}
	if len(plan.Batches) != len(want.Batches) || len(plan.UnrequestedCardRefs) != len(want.UnrequestedCardRefs) {
		return fmt.Errorf("mechanism study: request plan shape does not match exact compilation")
	}
	for position := range plan.Batches {
		if !reflect.DeepEqual(plan.Batches[position], want.Batches[position]) {
			return fmt.Errorf("mechanism study: request plan batch %d does not match exact compilation", position+1)
		}
	}
	if !reflect.DeepEqual(plan.UnrequestedCardRefs, want.UnrequestedCardRefs) {
		return fmt.Errorf("mechanism study: request plan suffix does not match exact compilation")
	}
	return nil
}

func makeRequestBatch(request Request) (RequestBatch, error) {
	if err := request.Validate(); err != nil {
		return RequestBatch{}, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return RequestBatch{}, fmt.Errorf("mechanism study: encode request: %w", err)
	}
	if len(encoded) > MaxRequestBytes {
		return RequestBatch{}, fmt.Errorf("mechanism study: request exceeds byte bound")
	}
	wireSHA := sha256Hex(encoded)
	return RequestBatch{
		Request: request, WireJSON: string(encoded), WireSHA256: wireSHA,
		sealed: requestBatchSeal(wireSHA),
	}, nil
}

func requestBatchSeal(wireSHA string) string {
	return fmt.Sprintf("mechanism-request-v%d:%s", RequestVersion, wireSHA)
}

func (request Request) Validate() error {
	if request.Version != RequestVersion || request.PromptVersion != PromptVersion ||
		!strings.HasPrefix(request.CatalogRef, "mc-") || !validSHA256(request.CatalogSHA256) ||
		request.CatalogRef != "mc-"+request.CatalogSHA256[:16] || !typedRef(request.RequestRef, 'q') ||
		len(request.Cards) == 0 || len(request.Cards) > MaxCardsPerRequest {
		return fmt.Errorf("mechanism study: invalid request envelope")
	}
	nodes, edges := 0, 0
	seenCards := make(map[string]struct{}, len(request.Cards))
	seenRefs := make(map[string]struct{})
	for _, card := range request.Cards {
		if _, duplicate := seenCards[card.Ref]; duplicate || !typedRef(card.Ref, 't') || !cardCanContainMechanism(card) {
			return fmt.Errorf("mechanism study: invalid request card")
		}
		seenCards[card.Ref] = struct{}{}
		if err := validatePublicCard(card, seenRefs); err != nil {
			return err
		}
		nodes += len(card.Nodes)
		edges += len(card.Edges)
	}
	if nodes > MaxNodesPerRequest || edges > MaxEdgesPerRequest {
		return fmt.Errorf("mechanism study: request graph exceeds aggregate bounds")
	}
	return nil
}

func validatePublicCard(card Card, globalRefs map[string]struct{}) error {
	if card.Label == "" || card.Question == "" || len(card.Readings) == 0 ||
		len(card.Readings) > MaxDirectReadingsPerCard || len(card.Nodes) > MaxNodesPerCard ||
		len(card.Edges) > MaxEdgesPerCard || len(card.Frontier) > MaxFrontierRecordsPerCard ||
		!publicLabelValid(card.Label, maxCardLabelRunes) || !publicLabelValid(card.Question, maxCardQuestionRunes) {
		return fmt.Errorf("mechanism study: invalid public card bounds")
	}
	nodes := make(map[string]struct{}, len(card.Nodes))
	for _, node := range card.Nodes {
		if !typedRef(node.Ref, 'n') || !publicLabelValid(node.Label, maxNodeLabelRunes) {
			return fmt.Errorf("mechanism study: invalid public node")
		}
		if _, duplicate := globalRefs[node.Ref]; duplicate {
			return fmt.Errorf("mechanism study: duplicate public ref")
		}
		globalRefs[node.Ref] = struct{}{}
		nodes[node.Ref] = struct{}{}
	}
	rooted := false
	for _, reading := range card.Readings {
		if !typedRef(reading.Ref, 'r') || !publicLabelValid(reading.Label, maxReadingLabelRunes) {
			return fmt.Errorf("mechanism study: invalid public reading")
		}
		if _, duplicate := globalRefs[reading.Ref]; duplicate {
			return fmt.Errorf("mechanism study: duplicate public ref")
		}
		globalRefs[reading.Ref] = struct{}{}
		if reading.RootNodeRef != "" {
			if _, ok := nodes[reading.RootNodeRef]; !ok {
				return fmt.Errorf("mechanism study: unknown public reading root")
			}
			rooted = true
		}
	}
	if !rooted {
		return fmt.Errorf("mechanism study: request card has no exact root")
	}
	for _, edge := range card.Edges {
		if !typedRef(edge.Ref, 'e') || !edge.Invocation.Valid() || edge.WitnessCount <= 0 {
			return fmt.Errorf("mechanism study: invalid public edge")
		}
		if _, duplicate := globalRefs[edge.Ref]; duplicate {
			return fmt.Errorf("mechanism study: duplicate public ref")
		}
		globalRefs[edge.Ref] = struct{}{}
		if _, ok := nodes[edge.CallerRef]; !ok {
			return fmt.Errorf("mechanism study: unknown public caller")
		}
		if _, ok := nodes[edge.CalleeRef]; !ok {
			return fmt.Errorf("mechanism study: unknown public callee")
		}
	}
	previous := ""
	for _, frontier := range card.Frontier {
		if !frontier.Reason.valid() || frontier.Count <= 0 ||
			(previous != "" && string(frontier.Reason) <= previous) {
			return fmt.Errorf("mechanism study: invalid public frontier")
		}
		previous = string(frontier.Reason)
	}
	return nil
}

func publicLabelValid(value string, limit int) bool {
	return value != "" && safeLabel(value, limit, "") == value
}

func cardCanContainMechanism(card Card) bool {
	if len(card.Edges) < 2 {
		return false
	}
	roots := make(map[string]struct{})
	for _, reading := range card.Readings {
		if reading.RootNodeRef != "" {
			roots[reading.RootNodeRef] = struct{}{}
		}
	}
	if len(roots) == 0 {
		return false
	}
	for _, first := range card.Edges {
		for _, second := range card.Edges {
			if first.Ref == second.Ref || first.CalleeRef != second.CallerRef ||
				first.CallerRef == first.CalleeRef || second.CallerRef == second.CalleeRef ||
				first.CallerRef == second.CalleeRef {
				continue
			}
			if _, ok := roots[first.CallerRef]; ok {
				return true
			}
			if _, ok := roots[first.CalleeRef]; ok {
				return true
			}
			if _, ok := roots[second.CalleeRef]; ok {
				return true
			}
		}
	}
	return false
}

func copyCard(card Card) Card {
	if card.Readings != nil {
		card.Readings = append([]Reading{}, card.Readings...)
	}
	if card.Nodes != nil {
		card.Nodes = append([]Node{}, card.Nodes...)
	}
	if card.Edges != nil {
		card.Edges = append([]Edge{}, card.Edges...)
	}
	if card.Frontier != nil {
		card.Frontier = append([]Frontier{}, card.Frontier...)
	}
	return card
}

// PreparedCards returns the honest provider-free state for every compiled
// card. Validated mechanisms replace this state only for their own card.
func PreparedCards(compilation *Compilation) ([]CardResult, error) {
	if err := compilation.Validate(); err != nil {
		return nil, err
	}
	results := make([]CardResult, 0, len(compilation.Cards))
	for _, card := range compilation.Cards {
		results = append(results, CardResult{
			CardRef: card.Ref, State: OutcomePrepared,
			Mechanisms: []Mechanism{}, Frontier: append([]Frontier(nil), card.Frontier...),
		})
	}
	return results, nil
}
