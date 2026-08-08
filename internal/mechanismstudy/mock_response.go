package mechanismstudy

import (
	"encoding/json"
	"fmt"
	"sort"
)

// MockResponse builds one deterministic provider-free contract fixture from
// the exact advertised graph. It is not a production selection fallback and
// never invents a root or requests more graph.
func MockResponse(batch RequestBatch) ([]byte, error) {
	if err := batch.Request.Validate(); err != nil {
		return nil, err
	}
	response := Response{
		Version: ResultVersion, CatalogRef: batch.Request.CatalogRef,
		CatalogSHA256: batch.Request.CatalogSHA256, RequestRef: batch.Request.RequestRef,
		Cards: make([]ResponseCard, 0, len(batch.Request.Cards)),
	}
	for _, card := range batch.Request.Cards {
		response.Cards = append(response.Cards, ResponseCard{
			CardRef: card.Ref, Mechanisms: mockCardCandidates(card),
		})
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("mechanism study: encode mock response: %w", err)
	}
	if len(encoded) > MaxResponseBytes {
		return nil, fmt.Errorf("mechanism study: mock response exceeds bounded envelope")
	}
	return encoded, nil
}

func mockCardCandidates(card Card) []Candidate {
	roots := make(map[string]struct{})
	for _, reading := range card.Readings {
		if reading.RootNodeRef != "" {
			roots[reading.RootNodeRef] = struct{}{}
		}
	}
	edges := append([]Edge(nil), card.Edges...)
	sort.Slice(edges, func(i, j int) bool { return edges[i].Ref < edges[j].Ref })
	for _, first := range edges {
		for _, second := range edges {
			if first.Ref == second.Ref || first.CalleeRef != second.CallerRef ||
				first.CallerRef == first.CalleeRef || second.CallerRef == second.CalleeRef ||
				first.CallerRef == second.CalleeRef {
				continue
			}
			tied := false
			for _, nodeRef := range []string{first.CallerRef, first.CalleeRef, second.CalleeRef} {
				if _, ok := roots[nodeRef]; ok {
					tied = true
					break
				}
			}
			if !tied {
				continue
			}
			// Deliberately reverse the wire order: response order cannot become
			// backend path authority.
			return []Candidate{{EdgeRefs: []string{second.Ref, first.Ref}}}
		}
	}
	return []Candidate{}
}
