package mechanismstudy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	ArtifactVersion = 1

	FactsArtifactFilename      = "study_investigation_facts.v1.json"
	CandidatesArtifactFilename = "study_investigation_candidates.v1.json"
	ResultArtifactFilename     = "study_investigation_result.v1.json"
	StatusArtifactFilename     = "study_investigation_status.v1.json"

	MaxFactsArtifactBytes      = 2 << 20
	MaxCandidatesArtifactBytes = 320 << 10
	MaxResultArtifactBytes     = 256 << 10
	MaxStatusArtifactBytes     = 64 << 10
)

var ArtifactFilenames = []string{
	FactsArtifactFilename,
	CandidatesArtifactFilename,
	ResultArtifactFilename,
	StatusArtifactFilename,
}

type factsArtifact struct {
	Version                int                    `json:"version"`
	CompilationVersion     int                    `json:"compilation_version"`
	RequestVersion         int                    `json:"request_version"`
	ResultVersion          int                    `json:"result_version"`
	PromptVersion          string                 `json:"prompt_version"`
	CatalogRef             string                 `json:"catalog_ref"`
	CatalogSHA256          string                 `json:"catalog_sha256"`
	CatalogAuthoritySHA256 string                 `json:"catalog_authority_sha256"`
	CatalogAuthority       json.RawMessage        `json:"catalog_authority"`
	RequestBatches         []requestBatchArtifact `json:"request_batches"`
	UnrequestedCardRefs    []string               `json:"unrequested_card_refs"`
}

type requestBatchArtifact struct {
	RequestRef string          `json:"request_ref"`
	WireSHA256 string          `json:"wire_sha256"`
	WireJSON   json.RawMessage `json:"wire_json"`
}

// RestoredFacts is the private, hash-verified authority reconstructed from a
// canonical facts artifact. Compilation includes the unexported restoration
// maps and Plan includes newly sealed v2 request batches.
type RestoredFacts struct {
	Compilation *Compilation
	Plan        RequestPlan
	SHA256      string
}

// EncodeFacts persists the exact bounded compilation authority and the one
// deterministic four-call plan. It never persists the full DirectCallIndex.
func EncodeFacts(compilation *Compilation, plan RequestPlan) ([]byte, error) {
	if err := plan.Validate(compilation); err != nil {
		return nil, fmt.Errorf("mechanism study facts artifact: %w", err)
	}
	if compilation.catalogAuthorityJSON == "" {
		return nil, fmt.Errorf("mechanism study facts artifact: missing catalog authority")
	}
	artifact := factsArtifact{
		Version:                ArtifactVersion,
		CompilationVersion:     CompilationVersion,
		RequestVersion:         RequestVersion,
		ResultVersion:          ResultVersion,
		PromptVersion:          PromptVersion,
		CatalogRef:             compilation.CatalogRef,
		CatalogSHA256:          compilation.CatalogSHA256,
		CatalogAuthoritySHA256: sha256Hex([]byte(compilation.catalogAuthorityJSON)),
		CatalogAuthority:       json.RawMessage(compilation.catalogAuthorityJSON),
		RequestBatches:         make([]requestBatchArtifact, 0, len(plan.Batches)),
		UnrequestedCardRefs:    append([]string(nil), plan.UnrequestedCardRefs...),
	}
	for _, batch := range plan.Batches {
		if err := validateRequestBatchWire(batch); err != nil {
			return nil, err
		}
		artifact.RequestBatches = append(artifact.RequestBatches, requestBatchArtifact{
			RequestRef: batch.Request.RequestRef,
			WireSHA256: batch.WireSHA256,
			WireJSON:   json.RawMessage(batch.WireJSON),
		})
	}
	return encodeCanonicalArtifact("mechanism study facts", MaxFactsArtifactBytes, artifact)
}

// DecodeFacts restores exact private authority and request seals. Any drift in
// a nested catalog/request, even with otherwise valid JSON, fails closed.
func DecodeFacts(data []byte) (RestoredFacts, error) {
	var artifact factsArtifact
	if err := decodeCanonicalArtifact("mechanism study facts", data, MaxFactsArtifactBytes, &artifact); err != nil {
		return RestoredFacts{}, err
	}
	if artifact.Version != ArtifactVersion || artifact.CompilationVersion != CompilationVersion ||
		artifact.RequestVersion != RequestVersion || artifact.ResultVersion != ResultVersion ||
		artifact.PromptVersion != PromptVersion || artifact.CatalogRef == "" ||
		!validSHA256(artifact.CatalogSHA256) || artifact.CatalogAuthoritySHA256 != artifact.CatalogSHA256 ||
		sha256Hex(artifact.CatalogAuthority) != artifact.CatalogAuthoritySHA256 {
		return RestoredFacts{}, fmt.Errorf("mechanism study facts artifact: invalid identity")
	}
	compilation, err := restoreCompilation(artifact.CatalogRef, artifact.CatalogSHA256, artifact.CatalogAuthority)
	if err != nil {
		return RestoredFacts{}, err
	}
	plan := RequestPlan{
		Batches:             make([]RequestBatch, 0, len(artifact.RequestBatches)),
		UnrequestedCardRefs: append([]string{}, artifact.UnrequestedCardRefs...),
	}
	for _, saved := range artifact.RequestBatches {
		var request Request
		if err := decodeCanonicalJSON("mechanism study facts request wire", saved.WireJSON, &request); err != nil {
			return RestoredFacts{}, err
		}
		batch, err := makeRequestBatch(request)
		if err != nil {
			return RestoredFacts{}, err
		}
		if saved.RequestRef != request.RequestRef || saved.WireSHA256 != batch.WireSHA256 ||
			!bytes.Equal(saved.WireJSON, []byte(batch.WireJSON)) {
			return RestoredFacts{}, fmt.Errorf("mechanism study facts artifact: request binding mismatch")
		}
		plan.Batches = append(plan.Batches, batch)
	}
	if err := plan.Validate(compilation); err != nil {
		return RestoredFacts{}, fmt.Errorf("mechanism study facts artifact: %w", err)
	}
	return RestoredFacts{Compilation: compilation, Plan: plan, SHA256: sha256Hex(data)}, nil
}

func restoreCompilation(catalogRef, catalogSHA string, raw []byte) (*Compilation, error) {
	var digest compilationDigest
	if err := decodeCanonicalJSON("mechanism study catalog authority", raw, &digest); err != nil {
		return nil, err
	}
	if sha256Hex(raw) != catalogSHA || catalogRef != "mc-"+catalogSHA[:16] {
		return nil, fmt.Errorf("mechanism study facts artifact: catalog binding mismatch")
	}
	compilation := &Compilation{
		Version:               digest.Version,
		TargetTrailVersion:    digest.TargetTrailVersion,
		CatalogRef:            catalogRef,
		CatalogSHA256:         catalogSHA,
		Binding:               digest.Binding,
		DirectCallIndexSHA256: digest.DirectCallIndexSHA256,
		Scenario:              copyScenario(digest.Scenario),
		Cards:                 make([]Card, 0, len(digest.Cards)),
		OmittedCards:          digest.OmittedCards,
		authority:             make(map[string]cardAuthority, len(digest.Cards)),
		catalogAuthorityJSON:  string(raw),
	}
	if digest.AnalysisTarget != nil {
		compilation.AnalysisTargetRef = digest.AnalysisTarget.Ref
		compilation.TargetRootsSHA256 = digest.TargetRootsSHA256
	}
	for _, saved := range digest.Cards {
		card := copyCard(saved.Card)
		authority := cardAuthority{
			sourceOrdinal:       saved.Ordinal,
			sourceCanonical:     saved.Canonical,
			nodeIDByRef:         make(map[string]string, len(saved.Nodes)),
			nodeRefByID:         make(map[string]string, len(saved.Nodes)),
			nodeByRef:           make(map[string]surfacediscovery.DirectCallNode, len(saved.Nodes)),
			edgeByRef:           make(map[string]surfacediscovery.DirectCallEdge, len(saved.Edges)),
			readingRootByRef:    make(map[string]string, len(saved.Readings)),
			readingOrdinalByRef: make(map[string]int, len(saved.Readings)),
			targetRootRefs:      make(map[string]struct{}, len(saved.TargetRootIDs)),
		}
		for _, savedNode := range saved.Nodes {
			node := copyExactNode(savedNode.Node)
			authority.nodeIDByRef[savedNode.Ref] = node.ID
			authority.nodeRefByID[node.ID] = savedNode.Ref
			authority.nodeByRef[savedNode.Ref] = node
		}
		for _, savedEdge := range saved.Edges {
			authority.edgeByRef[savedEdge.Ref] = savedEdge.Edge
		}
		for _, targetRootID := range saved.TargetRootIDs {
			rootRef := authority.nodeRefByID[targetRootID]
			if rootRef == "" {
				return nil, fmt.Errorf("mechanism study facts artifact: target root cannot be restored")
			}
			authority.targetRootRefs[rootRef] = struct{}{}
		}
		for _, savedReading := range saved.Readings {
			rootRef := authority.nodeRefByID[savedReading.RootID]
			if savedReading.RootID != "" && rootRef == "" {
				return nil, fmt.Errorf("mechanism study facts artifact: reading root cannot be restored")
			}
			if rootRef != "" {
				authority.readingRootByRef[savedReading.Ref] = rootRef
			}
			authority.readingOrdinalByRef[savedReading.Ref] = savedReading.Ordinal
		}
		compilation.Cards = append(compilation.Cards, card)
		compilation.authority[card.Ref] = authority
	}
	if err := compilation.Validate(); err != nil {
		return nil, fmt.Errorf("mechanism study facts artifact: restore compilation: %w", err)
	}
	return compilation, nil
}

func copyExactNode(node surfacediscovery.DirectCallNode) surfacediscovery.DirectCallNode {
	node.Symbol.EquivalentIDs = append([]string(nil), node.Symbol.EquivalentIDs...)
	return node
}

func encodeCanonicalArtifact(name string, limit int, value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s artifact: encode: %w", name, err)
	}
	if len(encoded) == 0 || len(encoded) > limit {
		return nil, fmt.Errorf("%s artifact exceeds %d bytes", name, limit)
	}
	if _, found := secretscan.DetectAlways(string(encoded)); found {
		return nil, fmt.Errorf("%s artifact contains credential-like content", name)
	}
	return encoded, nil
}

func decodeCanonicalArtifact(name string, data []byte, limit int, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("%s artifact: empty", name)
	}
	if len(data) > limit {
		return fmt.Errorf("%s artifact exceeds %d bytes", name, limit)
	}
	if _, found := secretscan.DetectAlways(string(data)); found {
		return fmt.Errorf("%s artifact contains credential-like content", name)
	}
	if err := decodeCanonicalJSON(name+" artifact", data, target); err != nil {
		return err
	}
	return nil
}

func decodeCanonicalJSON(name string, data []byte, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("%s: empty", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s: decode: %w", name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: trailing data", name)
	}
	canonical, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("%s: encode canonical: %w", name, err)
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("%s: not canonical", name)
	}
	return nil
}

func sortedUniqueTypedRefs(values []string, prefix byte) bool {
	if !sort.StringsAreSorted(values) {
		return false
	}
	previous := ""
	for _, value := range values {
		if !typedRef(value, prefix) || value == previous {
			return false
		}
		previous = value
	}
	return true
}
