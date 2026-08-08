package mechanismstudy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// ResolveResponse restores and validates one independent provider response.
// Envelope identity is atomic; invalid mechanism siblings close item-locally
// and never discard an independently valid path on the same card.
func ResolveResponse(compilation *Compilation, batch RequestBatch, raw []byte) (Result, error) {
	if err := compilation.Validate(); err != nil {
		return Result{}, err
	}
	if err := validateBatchAgainstCompilation(compilation, batch); err != nil {
		return Result{}, err
	}
	response, err := decodeResponseEnvelope(batch, raw)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Version: ResultVersion, PromptVersion: PromptVersion,
		CatalogRef: compilation.CatalogRef, CatalogSHA256: compilation.CatalogSHA256,
		RequestRef: batch.Request.RequestRef, RequestSHA256: batch.WireSHA256,
		Cards: make([]CardResult, 0, len(batch.Request.Cards)),
	}
	resultByRef := make(map[string]int, len(batch.Request.Cards))
	requestCardByRef := make(map[string]Card, len(batch.Request.Cards))
	for position, card := range batch.Request.Cards {
		resultByRef[card.Ref] = position
		requestCardByRef[card.Ref] = card
		result.Cards = append(result.Cards, CardResult{
			CardRef: card.Ref, State: OutcomePrepared, Mechanisms: []Mechanism{},
			Frontier: append([]Frontier(nil), card.Frontier...),
		})
	}

	seenCards := make(map[string]struct{}, len(response.Cards))
	responseByRef := make(map[string]ResponseCard, len(response.Cards))
	duplicateCards := make(map[string]struct{})
	for _, card := range response.Cards {
		if _, known := requestCardByRef[card.CardRef]; !known {
			return Result{}, fmt.Errorf("mechanism study: response cites unknown card ref")
		}
		if _, duplicate := seenCards[card.CardRef]; duplicate {
			duplicateCards[card.CardRef] = struct{}{}
			continue
		}
		seenCards[card.CardRef] = struct{}{}
		responseByRef[card.CardRef] = card
	}
	for _, requestCard := range batch.Request.Cards {
		resultPosition := resultByRef[requestCard.Ref]
		cardResult := &result.Cards[resultPosition]
		if _, duplicate := duplicateCards[requestCard.Ref]; duplicate {
			cardResult.Issues = append(cardResult.Issues, Issue{Code: IssueDuplicateCard})
			cardResult.Frontier = addResponseInvalid(cardResult.Frontier, 1)
			continue
		}
		responseCard, present := responseByRef[requestCard.Ref]
		if !present {
			cardResult.Issues = append(cardResult.Issues, Issue{Code: IssueMissingCard})
			cardResult.Frontier = addResponseInvalid(cardResult.Frontier, 1)
			continue
		}
		validateCandidates(requestCard, responseCard.Mechanisms, cardResult)
	}
	return result, nil
}

func decodeResponseEnvelope(batch RequestBatch, raw []byte) (Response, error) {
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Response{}, fmt.Errorf("mechanism study: response exceeds bounded envelope")
	}
	var response Response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return Response{}, fmt.Errorf("mechanism study: decode response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Response{}, err
	}
	if response.Version != ResultVersion || response.CatalogRef != batch.Request.CatalogRef ||
		response.CatalogSHA256 != batch.Request.CatalogSHA256 || response.RequestRef != batch.Request.RequestRef {
		return Response{}, fmt.Errorf("mechanism study: response identity mismatch")
	}
	return response, nil
}

func validateBatchAgainstCompilation(compilation *Compilation, batch RequestBatch) error {
	if err := validateRequestBatchWire(batch); err != nil {
		return err
	}
	if batch.Request.CatalogRef != compilation.CatalogRef ||
		batch.Request.CatalogSHA256 != compilation.CatalogSHA256 {
		return fmt.Errorf("mechanism study: request batch binding mismatch")
	}
	compiledByRef := make(map[string]Card, len(compilation.Cards))
	for _, card := range compilation.Cards {
		compiledByRef[card.Ref] = card
	}
	for _, card := range batch.Request.Cards {
		compiled, ok := compiledByRef[card.Ref]
		if !ok || !reflect.DeepEqual(compiled, card) || !cardCanContainMechanism(card) {
			return fmt.Errorf("mechanism study: request card is not the exact compiled graph")
		}
	}
	return nil
}

func validateCandidates(card Card, candidates []Candidate, result *CardResult) {
	edgeByRef := make(map[string]Edge, len(card.Edges))
	for _, edge := range card.Edges {
		edgeByRef[edge.Ref] = edge
	}
	type validatedCandidate struct {
		mechanism Mechanism
		pathKey   string
	}
	validated := make([]validatedCandidate, 0, len(candidates))
	seenPaths := make(map[string]struct{})
	for _, candidate := range candidates {
		mechanism, code := validateCandidate(candidate, edgeByRef, card.Readings)
		if code != "" {
			rejectCandidate(result, code)
			continue
		}
		pathKey := strings.Join(mechanism.EdgeRefs, "\x00")
		if _, duplicate := seenPaths[pathKey]; duplicate {
			rejectCandidate(result, IssueDuplicatePath)
			continue
		}
		seenPaths[pathKey] = struct{}{}
		validated = append(validated, validatedCandidate{mechanism: mechanism, pathKey: pathKey})
	}
	if len(validated) > MaxMechanismsPerCard {
		// Candidate order is not authority. An overfull valid set closes as one
		// card-local shape failure instead of letting its first members win.
		result.Issues = append(result.Issues, Issue{Code: IssueOverBound})
		result.Frontier = addResponseInvalid(result.Frontier, len(validated))
		sortIssues(result.Issues)
		return
	}
	sort.Slice(validated, func(i, j int) bool { return validated[i].pathKey < validated[j].pathKey })
	for _, candidate := range validated {
		result.Mechanisms = append(result.Mechanisms, candidate.mechanism)
	}
	sortIssues(result.Issues)
	if len(validated) > 0 {
		result.State = OutcomeMechanism
	}
}

func validateCandidate(
	candidate Candidate,
	edgeByRef map[string]Edge,
	readings []Reading,
) (Mechanism, IssueCode) {
	if len(candidate.EdgeRefs) < 2 {
		return Mechanism{}, IssueInvalidShape
	}
	if len(candidate.EdgeRefs) > MaxEdgesPerMechanism {
		return Mechanism{}, IssueOverBound
	}
	if hasDuplicateStrings(candidate.EdgeRefs) {
		return Mechanism{}, IssueDuplicateRef
	}

	selected := make([]Edge, 0, len(candidate.EdgeRefs))
	for _, ref := range candidate.EdgeRefs {
		edge, ok := edgeByRef[ref]
		if !ok {
			return Mechanism{}, IssueUnknownRef
		}
		selected = append(selected, edge)
	}
	nodes, edges, ok := reconstructDirectedPath(selected)
	if !ok || len(edges) < 2 || len(nodes) < 3 {
		return Mechanism{}, IssueDisconnected
	}
	pathNodes := make(map[string]struct{}, len(nodes))
	for _, ref := range nodes {
		pathNodes[ref] = struct{}{}
	}
	readingRefs := make([]string, 0, len(readings))
	for _, reading := range readings {
		if reading.RootNodeRef == "" {
			continue
		}
		if _, tied := pathNodes[reading.RootNodeRef]; tied {
			readingRefs = append(readingRefs, reading.Ref)
		}
	}
	if len(readingRefs) == 0 {
		return Mechanism{}, IssueNoReadingTie
	}
	return Mechanism{
		ReadingRefs: readingRefs, NodeRefs: nodes, EdgeRefs: edges,
	}, ""
}

func reconstructDirectedPath(selected []Edge) ([]string, []string, bool) {
	if len(selected) < 2 {
		return nil, nil, false
	}
	byCaller := make(map[string]Edge, len(selected))
	indegree := make(map[string]int)
	outdegree := make(map[string]int)
	nodes := make(map[string]struct{})
	for _, edge := range selected {
		if edge.CallerRef == edge.CalleeRef || outdegree[edge.CallerRef] > 0 || indegree[edge.CalleeRef] > 0 {
			return nil, nil, false
		}
		byCaller[edge.CallerRef] = edge
		outdegree[edge.CallerRef]++
		indegree[edge.CalleeRef]++
		nodes[edge.CallerRef] = struct{}{}
		nodes[edge.CalleeRef] = struct{}{}
	}
	if len(nodes) != len(selected)+1 {
		return nil, nil, false
	}
	starts := make([]string, 0, 1)
	for node := range nodes {
		in, out := indegree[node], outdegree[node]
		switch {
		case in == 0 && out == 1:
			starts = append(starts, node)
		case in == 1 && out == 0:
		case in == 1 && out == 1:
		default:
			return nil, nil, false
		}
	}
	if len(starts) != 1 {
		return nil, nil, false
	}
	nodeOrder := []string{starts[0]}
	edgeOrder := make([]string, 0, len(selected))
	current := starts[0]
	visited := make(map[string]struct{}, len(selected))
	for {
		edge, ok := byCaller[current]
		if !ok {
			break
		}
		if _, duplicate := visited[edge.Ref]; duplicate {
			return nil, nil, false
		}
		visited[edge.Ref] = struct{}{}
		edgeOrder = append(edgeOrder, edge.Ref)
		current = edge.CalleeRef
		nodeOrder = append(nodeOrder, current)
	}
	return nodeOrder, edgeOrder, len(edgeOrder) == len(selected)
}

func rejectCandidate(result *CardResult, code IssueCode) {
	result.Issues = append(result.Issues, Issue{Code: code})
	result.Frontier = addResponseInvalid(result.Frontier, 1)
}

func sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool { return issues[i].Code < issues[j].Code })
}

func addResponseInvalid(frontier []Frontier, count int) []Frontier {
	counts := make(map[FrontierReason]int, len(frontier)+1)
	for _, item := range frontier {
		counts[item.Reason] += item.Count
	}
	counts[FrontierResponseInvalid] += count
	return frontierFromCounts(counts)
}

func hasDuplicateStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("mechanism study: response contains trailing JSON")
		}
		return fmt.Errorf("mechanism study: decode response trailer: %w", err)
	}
	return nil
}
