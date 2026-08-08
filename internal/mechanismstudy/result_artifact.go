package mechanismstudy

import (
	"fmt"
	"reflect"
	"strings"
)

type resultArtifact struct {
	Version          int          `json:"version"`
	ResultVersion    int          `json:"result_version"`
	PromptVersion    string       `json:"prompt_version"`
	FactsSHA256      string       `json:"facts_sha256"`
	CandidatesSHA256 string       `json:"candidates_sha256"`
	CatalogRef       string       `json:"catalog_ref"`
	CatalogSHA256    string       `json:"catalog_sha256"`
	Cards            []CardResult `json:"cards"`
}

type RestoredResult struct {
	Cards  []CardResult
	SHA256 string
}

// AggregateResults replays every successful batch against exact restored
// authority, starting from the honest prepared state for every compiled card.
// Batch, response-card, candidate, and edge-ref input order never becomes
// final result order.
func AggregateResults(compilation *Compilation, plan RequestPlan, candidates []BatchCandidate) ([]CardResult, error) {
	if err := plan.Validate(compilation); err != nil {
		return nil, err
	}
	final, err := PreparedCards(compilation)
	if err != nil {
		return nil, err
	}
	finalByRef := make(map[string]int, len(final))
	for position, card := range final {
		finalByRef[card.CardRef] = position
	}
	batchByRef := make(map[string]RequestBatch, len(plan.Batches))
	for _, batch := range plan.Batches {
		batchByRef[batch.Request.RequestRef] = batch
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		batch, present := batchByRef[candidate.RequestRef]
		if !present {
			return nil, fmt.Errorf("mechanism study: candidate batch is outside exact request plan")
		}
		if _, duplicate := seen[candidate.RequestRef]; duplicate {
			return nil, fmt.Errorf("mechanism study: duplicate candidate batch")
		}
		seen[candidate.RequestRef] = struct{}{}
		raw, err := validateBatchCandidate(compilation, batch, candidate)
		if err != nil {
			return nil, err
		}
		resolved, err := ResolveResponse(compilation, batch, raw)
		if err != nil {
			return nil, err
		}
		for _, card := range resolved.Cards {
			position, ok := finalByRef[card.CardRef]
			if !ok {
				return nil, fmt.Errorf("mechanism study: resolved card is outside compilation")
			}
			final[position] = cloneCardResult(card)
		}
	}
	if err := validateAggregateCards(compilation, final); err != nil {
		return nil, err
	}
	return final, nil
}

func EncodeResult(factsData, candidatesData []byte) ([]byte, error) {
	facts, err := DecodeFacts(factsData)
	if err != nil {
		return nil, err
	}
	candidates, err := DecodeCandidates(factsData, candidatesData)
	if err != nil {
		return nil, err
	}
	cards, err := AggregateResults(facts.Compilation, facts.Plan, candidates.Batches)
	if err != nil {
		return nil, err
	}
	artifact := resultArtifact{
		Version:          ArtifactVersion,
		ResultVersion:    ResultVersion,
		PromptVersion:    PromptVersion,
		FactsSHA256:      facts.SHA256,
		CandidatesSHA256: candidates.SHA256,
		CatalogRef:       facts.Compilation.CatalogRef,
		CatalogSHA256:    facts.Compilation.CatalogSHA256,
		Cards:            cloneCardResults(cards),
	}
	return encodeCanonicalArtifact("mechanism study result", MaxResultArtifactBytes, artifact)
}

func DecodeResult(factsData, candidatesData, data []byte) (RestoredResult, error) {
	facts, err := DecodeFacts(factsData)
	if err != nil {
		return RestoredResult{}, err
	}
	candidates, err := DecodeCandidates(factsData, candidatesData)
	if err != nil {
		return RestoredResult{}, err
	}
	var artifact resultArtifact
	if err := decodeCanonicalArtifact("mechanism study result", data, MaxResultArtifactBytes, &artifact); err != nil {
		return RestoredResult{}, err
	}
	if artifact.Version != ArtifactVersion || artifact.ResultVersion != ResultVersion ||
		artifact.PromptVersion != PromptVersion || artifact.FactsSHA256 != facts.SHA256 ||
		artifact.CandidatesSHA256 != candidates.SHA256 || artifact.CatalogRef != facts.Compilation.CatalogRef ||
		artifact.CatalogSHA256 != facts.Compilation.CatalogSHA256 {
		return RestoredResult{}, fmt.Errorf("mechanism study result artifact: binding mismatch")
	}
	want, err := AggregateResults(facts.Compilation, facts.Plan, candidates.Batches)
	if err != nil {
		return RestoredResult{}, err
	}
	if !reflect.DeepEqual(artifact.Cards, want) {
		return RestoredResult{}, fmt.Errorf("mechanism study result artifact: cards do not match canonical replay")
	}
	return RestoredResult{Cards: cloneCardResults(artifact.Cards), SHA256: sha256Hex(data)}, nil
}

func validateAggregateCards(compilation *Compilation, cards []CardResult) error {
	if len(cards) != len(compilation.Cards) {
		return fmt.Errorf("mechanism study: aggregate result card count mismatch")
	}
	for position, result := range cards {
		card := compilation.Cards[position]
		if result.CardRef != card.Ref || (result.State != OutcomePrepared && result.State != OutcomeMechanism) ||
			result.Mechanisms == nil {
			return fmt.Errorf("mechanism study: invalid aggregate card identity")
		}
		if len(result.Mechanisms) > MaxMechanismsPerCard || len(result.Frontier) > MaxFrontierRecordsPerCard {
			return fmt.Errorf("mechanism study: aggregate card exceeds bounds")
		}
		edges := make(map[string]Edge, len(card.Edges))
		for _, edge := range card.Edges {
			edges[edge.Ref] = edge
		}
		previousPath := ""
		for _, mechanism := range result.Mechanisms {
			resolved, code := validateCandidate(Candidate{EdgeRefs: mechanism.EdgeRefs}, edges, card.Readings)
			if code != "" || !reflect.DeepEqual(resolved, mechanism) {
				return fmt.Errorf("mechanism study: aggregate mechanism is not exact")
			}
			path := strings.Join(mechanism.EdgeRefs, "\x00")
			if previousPath != "" && path <= previousPath {
				return fmt.Errorf("mechanism study: aggregate mechanisms are not canonical")
			}
			previousPath = path
		}
		if (len(result.Mechanisms) > 0) != (result.State == OutcomeMechanism) {
			return fmt.Errorf("mechanism study: aggregate outcome does not match exact mechanisms")
		}
		previousReason := ""
		for _, frontier := range result.Frontier {
			if !frontier.Reason.valid() || frontier.Count <= 0 ||
				(previousReason != "" && string(frontier.Reason) <= previousReason) {
				return fmt.Errorf("mechanism study: invalid aggregate frontier")
			}
			previousReason = string(frontier.Reason)
		}
		previousIssue := IssueCode("")
		for _, issue := range result.Issues {
			if !issue.Code.valid() || (previousIssue != "" && issue.Code < previousIssue) {
				return fmt.Errorf("mechanism study: invalid aggregate issue")
			}
			previousIssue = issue.Code
		}
	}
	return nil
}

func (code IssueCode) valid() bool {
	switch code {
	case IssueInvalidShape, IssueUnknownRef, IssueDisconnected, IssueDuplicateRef,
		IssueDuplicatePath, IssueOverBound, IssueNoReadingTie, IssueMissingCard, IssueDuplicateCard:
		return true
	default:
		return false
	}
}

func cloneCardResult(card CardResult) CardResult {
	clone := card
	clone.Frontier = append([]Frontier(nil), card.Frontier...)
	clone.Issues = append([]Issue(nil), card.Issues...)
	clone.Mechanisms = make([]Mechanism, 0, len(card.Mechanisms))
	for _, mechanism := range card.Mechanisms {
		clone.Mechanisms = append(clone.Mechanisms, Mechanism{
			ReadingRefs: append([]string(nil), mechanism.ReadingRefs...),
			NodeRefs:    append([]string(nil), mechanism.NodeRefs...),
			EdgeRefs:    append([]string(nil), mechanism.EdgeRefs...),
		})
	}
	return clone
}

func cloneCardResults(cards []CardResult) []CardResult {
	result := make([]CardResult, 0, len(cards))
	for _, card := range cards {
		result = append(result, cloneCardResult(card))
	}
	return result
}
