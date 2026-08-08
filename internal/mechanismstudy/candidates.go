package mechanismstudy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/secretscan"
)

// BatchCandidate is one strict refs-only response bound to the exact sealed v2
// request that authorized it. It contains no provider envelope or prose.
type BatchCandidate struct {
	RequestRef    string
	RequestSHA256 string
	Response      Response
}

type candidatesArtifact struct {
	Version       int                      `json:"version"`
	FactsSHA256   string                   `json:"facts_sha256"`
	CatalogRef    string                   `json:"catalog_ref"`
	CatalogSHA256 string                   `json:"catalog_sha256"`
	Batches       []candidateBatchArtifact `json:"batches"`
}

type candidateBatchArtifact struct {
	RequestRef     string          `json:"request_ref"`
	RequestSHA256  string          `json:"request_sha256"`
	ResponseSHA256 string          `json:"response_sha256"`
	Response       json.RawMessage `json:"response"`
}

type RestoredCandidates struct {
	Batches []BatchCandidate
	SHA256  string
}

// ParseBatchCandidate closes the persistence boundary around one successful
// provider response. Envelope/schema failures are returned to the caller for
// status accounting and never enter the candidates artifact.
func ParseBatchCandidate(compilation *Compilation, batch RequestBatch, raw []byte) (BatchCandidate, error) {
	if _, found := secretscan.DetectAlways(string(raw)); found {
		return BatchCandidate{}, fmt.Errorf("mechanism study candidates artifact: response contains credential-like content")
	}
	if _, err := ResolveResponse(compilation, batch, raw); err != nil {
		return BatchCandidate{}, err
	}
	response, err := decodeResponseEnvelope(batch, raw)
	if err != nil {
		return BatchCandidate{}, err
	}
	if err := validatePersistableResponse(response); err != nil {
		return BatchCandidate{}, err
	}
	return BatchCandidate{
		RequestRef:    batch.Request.RequestRef,
		RequestSHA256: batch.WireSHA256,
		Response:      canonicalResponse(response),
	}, nil
}

func validatePersistableResponse(response Response) error {
	for _, card := range response.Cards {
		if !typedRef(card.CardRef, 't') {
			return fmt.Errorf("mechanism study candidates artifact: response contains a non-ref card identity")
		}
		for _, candidate := range card.Mechanisms {
			for _, ref := range candidate.EdgeRefs {
				if !typedRef(ref, 'e') {
					return fmt.Errorf("mechanism study candidates artifact: response contains a non-ref edge identity")
				}
			}
		}
	}
	return nil
}

// EncodeCandidates stores a canonical plan-ordered subset of successful
// response batches. Missing planned batches remain prepared and are accounted
// by the status artifact rather than represented by fabricated responses.
func EncodeCandidates(factsData []byte, batches []BatchCandidate) ([]byte, error) {
	facts, err := DecodeFacts(factsData)
	if err != nil {
		return nil, err
	}
	byRef := make(map[string]BatchCandidate, len(batches))
	for _, candidate := range batches {
		if _, duplicate := byRef[candidate.RequestRef]; duplicate {
			return nil, fmt.Errorf("mechanism study candidates artifact: duplicate request response")
		}
		byRef[candidate.RequestRef] = candidate
	}
	artifact := candidatesArtifact{
		Version:       ArtifactVersion,
		FactsSHA256:   facts.SHA256,
		CatalogRef:    facts.Compilation.CatalogRef,
		CatalogSHA256: facts.Compilation.CatalogSHA256,
		Batches:       []candidateBatchArtifact{},
	}
	for _, batch := range facts.Plan.Batches {
		candidate, present := byRef[batch.Request.RequestRef]
		if !present {
			continue
		}
		delete(byRef, batch.Request.RequestRef)
		responseRaw, err := validateBatchCandidate(facts.Compilation, batch, candidate)
		if err != nil {
			return nil, err
		}
		artifact.Batches = append(artifact.Batches, candidateBatchArtifact{
			RequestRef:     candidate.RequestRef,
			RequestSHA256:  candidate.RequestSHA256,
			ResponseSHA256: sha256Hex(responseRaw),
			Response:       responseRaw,
		})
	}
	if len(byRef) != 0 {
		return nil, fmt.Errorf("mechanism study candidates artifact: response is outside exact request plan")
	}
	return encodeCanonicalArtifact("mechanism study candidates", MaxCandidatesArtifactBytes, artifact)
}

func DecodeCandidates(factsData, data []byte) (RestoredCandidates, error) {
	facts, err := DecodeFacts(factsData)
	if err != nil {
		return RestoredCandidates{}, err
	}
	var artifact candidatesArtifact
	if err := decodeCanonicalArtifact("mechanism study candidates", data, MaxCandidatesArtifactBytes, &artifact); err != nil {
		return RestoredCandidates{}, err
	}
	if artifact.Version != ArtifactVersion || artifact.FactsSHA256 != facts.SHA256 ||
		artifact.CatalogRef != facts.Compilation.CatalogRef ||
		artifact.CatalogSHA256 != facts.Compilation.CatalogSHA256 {
		return RestoredCandidates{}, fmt.Errorf("mechanism study candidates artifact: binding mismatch")
	}
	batchByRef := make(map[string]RequestBatch, len(facts.Plan.Batches))
	planPosition := make(map[string]int, len(facts.Plan.Batches))
	for position, batch := range facts.Plan.Batches {
		batchByRef[batch.Request.RequestRef] = batch
		planPosition[batch.Request.RequestRef] = position
	}
	result := RestoredCandidates{Batches: make([]BatchCandidate, 0, len(artifact.Batches)), SHA256: sha256Hex(data)}
	seen := make(map[string]struct{}, len(artifact.Batches))
	previousPosition := -1
	for _, saved := range artifact.Batches {
		batch, present := batchByRef[saved.RequestRef]
		position := planPosition[saved.RequestRef]
		if !present || position <= previousPosition {
			return RestoredCandidates{}, fmt.Errorf("mechanism study candidates artifact: requests are not canonical plan order")
		}
		if _, duplicate := seen[saved.RequestRef]; duplicate {
			return RestoredCandidates{}, fmt.Errorf("mechanism study candidates artifact: duplicate request response")
		}
		seen[saved.RequestRef] = struct{}{}
		previousPosition = position
		if saved.RequestSHA256 != batch.WireSHA256 || !validSHA256(saved.ResponseSHA256) ||
			sha256Hex(saved.Response) != saved.ResponseSHA256 {
			return RestoredCandidates{}, fmt.Errorf("mechanism study candidates artifact: response digest mismatch")
		}
		var response Response
		if err := decodeCanonicalJSON("mechanism study candidate response", saved.Response, &response); err != nil {
			return RestoredCandidates{}, err
		}
		candidate := BatchCandidate{RequestRef: saved.RequestRef, RequestSHA256: saved.RequestSHA256, Response: response}
		canonicalRaw, err := validateBatchCandidate(facts.Compilation, batch, candidate)
		if err != nil {
			return RestoredCandidates{}, err
		}
		if !bytes.Equal(canonicalRaw, saved.Response) {
			return RestoredCandidates{}, fmt.Errorf("mechanism study candidates artifact: response set is not canonical")
		}
		candidate.Response = canonicalResponse(response)
		result.Batches = append(result.Batches, candidate)
	}
	return result, nil
}

func validateBatchCandidate(compilation *Compilation, batch RequestBatch, candidate BatchCandidate) ([]byte, error) {
	if candidate.RequestRef != batch.Request.RequestRef || candidate.RequestSHA256 != batch.WireSHA256 {
		return nil, fmt.Errorf("mechanism study candidates artifact: request binding mismatch")
	}
	if err := validatePersistableResponse(candidate.Response); err != nil {
		return nil, err
	}
	response := canonicalResponse(candidate.Response)
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("mechanism study candidates artifact: encode response: %w", err)
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return nil, fmt.Errorf("mechanism study candidates artifact: response exceeds bounded envelope")
	}
	if _, found := secretscan.DetectAlways(string(raw)); found {
		return nil, fmt.Errorf("mechanism study candidates artifact: response contains credential-like content")
	}
	if _, err := ResolveResponse(compilation, batch, raw); err != nil {
		return nil, fmt.Errorf("mechanism study candidates artifact: %w", err)
	}
	return raw, nil
}

func cloneResponse(response Response) Response {
	clone := response
	clone.Cards = make([]ResponseCard, 0, len(response.Cards))
	for _, card := range response.Cards {
		copied := ResponseCard{CardRef: card.CardRef, Mechanisms: make([]Candidate, 0, len(card.Mechanisms))}
		for _, candidate := range card.Mechanisms {
			copied.Mechanisms = append(copied.Mechanisms, Candidate{EdgeRefs: append([]string(nil), candidate.EdgeRefs...)})
		}
		clone.Cards = append(clone.Cards, copied)
	}
	return clone
}

func canonicalResponse(response Response) Response {
	clone := cloneResponse(response)
	if clone.Cards == nil {
		clone.Cards = []ResponseCard{}
	}
	for cardPosition := range clone.Cards {
		card := &clone.Cards[cardPosition]
		if card.Mechanisms == nil {
			card.Mechanisms = []Candidate{}
		}
		for candidatePosition := range card.Mechanisms {
			sort.Strings(card.Mechanisms[candidatePosition].EdgeRefs)
			if card.Mechanisms[candidatePosition].EdgeRefs == nil {
				card.Mechanisms[candidatePosition].EdgeRefs = []string{}
			}
		}
		sort.Slice(card.Mechanisms, func(i, j int) bool {
			return strings.Join(card.Mechanisms[i].EdgeRefs, "\x00") <
				strings.Join(card.Mechanisms[j].EdgeRefs, "\x00")
		})
	}
	sort.Slice(clone.Cards, func(i, j int) bool {
		if clone.Cards[i].CardRef != clone.Cards[j].CardRef {
			return clone.Cards[i].CardRef < clone.Cards[j].CardRef
		}
		left, _ := json.Marshal(clone.Cards[i].Mechanisms)
		right, _ := json.Marshal(clone.Cards[j].Mechanisms)
		return string(left) < string(right)
	})
	return clone
}
