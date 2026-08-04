package studymap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	DirectionReferenceCatalogVersion   = "study-direction-reference-catalog-v1"
	DirectionReferenceResponseVersion  = "study-direction-reference-response-v1"
	DirectionReferenceValidatorVersion = "study-direction-reference-validator-v1"
)

type directionReferenceKind string

const (
	directionReferenceAnchor    directionReferenceKind = "anchor"
	directionReferenceDocument  directionReferenceKind = "document"
	directionReferenceArea      directionReferenceKind = "area"
	directionReferenceMechanism directionReferenceKind = "mechanism"
)

// DirectionReferenceCatalog is one private, deterministic request-local
// mapping between model-visible typed handles and backend-owned canonical
// Study identities. Per-object handles stay short; one catalog token binds the
// response to the exact bounded bundle, ordering, and contract.
type DirectionReferenceCatalog struct {
	digest        string
	catalogRef    string
	canonicalJSON []byte
	wireJSON      []byte
	byRef         map[string]directionReferenceEntry
	refByKey      map[string]string
}

type directionReferenceEntry struct {
	Kind        directionReferenceKind `json:"kind"`
	Ref         string                 `json:"ref"`
	CanonicalID string                 `json:"canonical_id"`
}

type directionReferenceWireBundle struct {
	CatalogRef         string                   `json:"catalog_ref"`
	Version            int                      `json:"version"`
	RepoName           string                   `json:"repo_name"`
	DocumentedPurpose  string                   `json:"documented_purpose,omitempty"`
	OrientationSummary string                   `json:"orientation_summary,omitempty"`
	RepositoryTypeHint RepositoryType           `json:"repository_type_hint,omitempty"`
	DomainTerms        []DomainTerm             `json:"domain_terms,omitempty"`
	Areas              []directionWireArea      `json:"areas"`
	Anchors            []directionWireAnchor    `json:"code_anchors"`
	Documents          []directionWireDocument  `json:"documents,omitempty"`
	Mechanisms         []directionWireMechanism `json:"canonical_mechanisms,omitempty"`
}

type directionWireArea struct {
	AreaRef        string `json:"area_ref"`
	Name           string `json:"name"`
	Responsibility string `json:"responsibility"`
	Path           string `json:"path,omitempty"`
	Line           int    `json:"line,omitempty"`
}

type directionWireAnchor struct {
	AnchorRef    string                         `json:"anchor_ref"`
	Path         string                         `json:"path"`
	Symbol       string                         `json:"symbol"`
	Line         int                            `json:"line"`
	Role         artifactrole.Role              `json:"role"`
	Statement    string                         `json:"bounded_fact"`
	Capabilities []semanticdiscovery.Capability `json:"capabilities,omitempty"`
	AreaRefs     []string                       `json:"area_refs,omitempty"`
}

type directionWireDocument struct {
	DocumentRef string `json:"document_ref"`
	Path        string `json:"path"`
	Label       string `json:"label"`
	Excerpt     string `json:"excerpt,omitempty"`
}

type directionWireMechanism struct {
	MechanismRef string   `json:"mechanism_ref"`
	Question     string   `json:"question"`
	Title        string   `json:"title"`
	AnchorRefs   []string `json:"anchor_refs,omitempty"`
	Paths        []string `json:"paths,omitempty"`
}

type directionReferenceProviderResponse struct {
	Version    int             `json:"version"`
	CatalogRef string          `json:"catalog_ref"`
	Directions json.RawMessage `json:"directions"`
}

type directionReferenceProviderCandidate struct {
	DirectionID     string                            `json:"direction_id,omitempty"`
	Question        string                            `json:"question"`
	WhyItMatters    string                            `json:"why_it_matters"`
	LearningOutcome string                            `json:"learning_outcome"`
	TargetJob       TargetJob                         `json:"target_user_job"`
	LearningStage   LearningStage                     `json:"learning_stage"`
	AnchorRefs      []string                          `json:"anchor_refs"`
	DocumentRefs    []string                          `json:"document_refs,omitempty"`
	AreaRefs        []string                          `json:"area_refs,omitempty"`
	MechanismRef    string                            `json:"mechanism_ref,omitempty"`
	ReadingAnchors  []directionReferenceReadingAnchor `json:"reading_anchors"`
	SearchQueries   []string                          `json:"search_queries,omitempty"`
}

type directionReferenceReadingAnchor struct {
	AnchorRef     string `json:"anchor_ref"`
	Label         string `json:"label"`
	WhatToLookFor string `json:"what_to_look_for"`
}

// BuildDirectionReferenceCatalog builds the exact private identity and the
// model-visible wire projection together. Slice order is intentionally part of
// the identity because it is part of the exact provider request.
func BuildDirectionReferenceCatalog(bundle Bundle) (DirectionReferenceCatalog, error) {
	if err := bundle.Validate(); err != nil {
		return DirectionReferenceCatalog{}, err
	}
	// The candidate request never contains retained source bodies. Bind the
	// request token to the exact model-visible candidate input plus its private
	// canonical mappings, so an unrelated review source-fragment change does
	// not invalidate every per-direction review cache entry.
	bundleJSON, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		return DirectionReferenceCatalog{}, fmt.Errorf("study direction refs: encode bundle identity: %w", err)
	}
	privateIdentity := struct {
		CatalogContract   string          `json:"catalog_contract"`
		ResponseContract  string          `json:"response_contract"`
		ValidatorContract string          `json:"validator_contract"`
		PromptContract    string          `json:"prompt_contract"`
		Bundle            json.RawMessage `json:"bundle"`
	}{
		CatalogContract:   DirectionReferenceCatalogVersion,
		ResponseContract:  DirectionReferenceResponseVersion,
		ValidatorContract: DirectionReferenceValidatorVersion,
		PromptContract:    semanticdiscovery.StudyCandidatesPromptVersion,
		Bundle:            append(json.RawMessage(nil), bundleJSON...),
	}
	privateJSON, err := json.Marshal(privateIdentity)
	if err != nil {
		return DirectionReferenceCatalog{}, fmt.Errorf("study direction refs: encode private identity: %w", err)
	}
	privateDigest := sha256.Sum256(privateJSON)
	catalogRef := "c_" + hex.EncodeToString(privateDigest[:12])

	entries := make([]directionReferenceEntry, 0,
		len(bundle.Anchors)+len(bundle.Documents)+len(bundle.Areas)+len(bundle.Mechanisms))
	appendEntries := func(kind directionReferenceKind, ids []string) error {
		for ordinal, id := range ids {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("study direction refs: %s identity is empty", kind)
			}
			entries = append(entries, directionReferenceEntry{
				Kind:        kind,
				Ref:         directionReferenceHandle(kind, ordinal+1),
				CanonicalID: id,
			})
		}
		return nil
	}
	anchorIDs := make([]string, 0, len(bundle.Anchors))
	for _, item := range bundle.Anchors {
		anchorIDs = append(anchorIDs, item.ID)
	}
	documentIDs := make([]string, 0, len(bundle.Documents))
	for _, item := range bundle.Documents {
		documentIDs = append(documentIDs, item.ID)
	}
	areaIDs := make([]string, 0, len(bundle.Areas))
	for _, item := range bundle.Areas {
		areaIDs = append(areaIDs, item.ID)
	}
	mechanismIDs := make([]string, 0, len(bundle.Mechanisms))
	for _, item := range bundle.Mechanisms {
		mechanismIDs = append(mechanismIDs, item.ID)
	}
	for _, group := range []struct {
		kind directionReferenceKind
		ids  []string
	}{
		{directionReferenceAnchor, anchorIDs},
		{directionReferenceDocument, documentIDs},
		{directionReferenceArea, areaIDs},
		{directionReferenceMechanism, mechanismIDs},
	} {
		if err := appendEntries(group.kind, group.ids); err != nil {
			return DirectionReferenceCatalog{}, err
		}
	}
	identity := struct {
		CatalogContract   string                    `json:"catalog_contract"`
		ResponseContract  string                    `json:"response_contract"`
		ValidatorContract string                    `json:"validator_contract"`
		PromptContract    string                    `json:"prompt_contract"`
		CatalogRef        string                    `json:"catalog_ref"`
		PrivateSHA256     string                    `json:"private_sha256"`
		Entries           []directionReferenceEntry `json:"entries"`
	}{
		CatalogContract:   DirectionReferenceCatalogVersion,
		ResponseContract:  DirectionReferenceResponseVersion,
		ValidatorContract: DirectionReferenceValidatorVersion,
		PromptContract:    semanticdiscovery.StudyCandidatesPromptVersion,
		CatalogRef:        catalogRef,
		PrivateSHA256:     hex.EncodeToString(privateDigest[:]),
		Entries:           entries,
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		return DirectionReferenceCatalog{}, fmt.Errorf("study direction refs: encode catalog identity: %w", err)
	}
	digest := sha256.Sum256(identityJSON)
	catalog := DirectionReferenceCatalog{
		digest:        hex.EncodeToString(digest[:]),
		catalogRef:    catalogRef,
		canonicalJSON: append([]byte(nil), identityJSON...),
		byRef:         make(map[string]directionReferenceEntry, len(entries)),
		refByKey:      make(map[string]string, len(entries)),
	}
	for _, entry := range entries {
		if _, duplicate := catalog.byRef[entry.Ref]; duplicate {
			return DirectionReferenceCatalog{}, fmt.Errorf("study direction refs: handle collision")
		}
		key := directionReferenceKey(entry.Kind, entry.CanonicalID)
		if _, duplicate := catalog.refByKey[key]; duplicate {
			return DirectionReferenceCatalog{}, fmt.Errorf("study direction refs: duplicate %s identity", entry.Kind)
		}
		catalog.byRef[entry.Ref] = entry
		catalog.refByKey[key] = entry.Ref
	}
	wire, err := catalog.buildWireBundle(bundle)
	if err != nil {
		return DirectionReferenceCatalog{}, err
	}
	catalog.wireJSON = wire
	return catalog, nil
}

func directionReferenceHandle(kind directionReferenceKind, ordinal int) string {
	prefix := map[directionReferenceKind]string{
		directionReferenceAnchor:    "a",
		directionReferenceDocument:  "d",
		directionReferenceArea:      "r",
		directionReferenceMechanism: "m",
	}[kind]
	return fmt.Sprintf("%s%d", prefix, ordinal)
}

func directionReferenceKey(kind directionReferenceKind, id string) string {
	return string(kind) + "\x00" + id
}

func (catalog DirectionReferenceCatalog) Digest() string { return catalog.digest }

func (catalog DirectionReferenceCatalog) CatalogRef() string { return catalog.catalogRef }

func (catalog DirectionReferenceCatalog) IdentityJSON() []byte {
	return append([]byte(nil), catalog.canonicalJSON...)
}

func (catalog DirectionReferenceCatalog) PromptBundleJSON() []byte {
	return append([]byte(nil), catalog.wireJSON...)
}

func (catalog DirectionReferenceCatalog) buildWireBundle(bundle Bundle) ([]byte, error) {
	wire := directionReferenceWireBundle{
		CatalogRef: catalog.catalogRef, Version: bundle.Version, RepoName: bundle.RepoName,
		DocumentedPurpose:  bundle.DocumentedPurpose,
		OrientationSummary: bundle.OrientationSummary,
		RepositoryTypeHint: bundle.RepositoryTypeHint,
		DomainTerms:        append([]DomainTerm(nil), bundle.DomainTerms...),
	}
	for _, area := range bundle.Areas {
		ref, err := catalog.ref(directionReferenceArea, area.ID)
		if err != nil {
			return nil, err
		}
		wire.Areas = append(wire.Areas, directionWireArea{
			AreaRef: ref, Name: area.Name, Responsibility: area.Responsibility,
			Path: area.Path, Line: area.Line,
		})
	}
	for _, anchor := range bundle.Anchors {
		ref, err := catalog.ref(directionReferenceAnchor, anchor.ID)
		if err != nil {
			return nil, err
		}
		areaRefs := make([]string, 0, len(anchor.AreaIDs))
		for _, areaID := range anchor.AreaIDs {
			areaRef, err := catalog.ref(directionReferenceArea, areaID)
			if err != nil {
				return nil, err
			}
			areaRefs = append(areaRefs, areaRef)
		}
		wire.Anchors = append(wire.Anchors, directionWireAnchor{
			AnchorRef: ref, Path: anchor.Path, Symbol: anchor.Symbol, Line: anchor.Line,
			Role: anchor.Role, Statement: anchor.Statement,
			Capabilities: append([]semanticdiscovery.Capability(nil), anchor.Capabilities...),
			AreaRefs:     areaRefs,
		})
	}
	for _, document := range bundle.Documents {
		ref, err := catalog.ref(directionReferenceDocument, document.ID)
		if err != nil {
			return nil, err
		}
		wire.Documents = append(wire.Documents, directionWireDocument{
			DocumentRef: ref, Path: document.Path, Label: document.Label, Excerpt: document.Excerpt,
		})
	}
	for _, mechanism := range bundle.Mechanisms {
		ref, err := catalog.ref(directionReferenceMechanism, mechanism.ID)
		if err != nil {
			return nil, err
		}
		anchorRefs := make([]string, 0, len(mechanism.AnchorIDs))
		for _, anchorID := range mechanism.AnchorIDs {
			anchorRef, err := catalog.ref(directionReferenceAnchor, anchorID)
			if err != nil {
				return nil, err
			}
			anchorRefs = append(anchorRefs, anchorRef)
		}
		wire.Mechanisms = append(wire.Mechanisms, directionWireMechanism{
			MechanismRef: ref, Question: mechanism.Question, Title: mechanism.Title,
			AnchorRefs: anchorRefs, Paths: append([]string(nil), mechanism.Paths...),
		})
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("study direction refs: encode wire bundle: %w", err)
	}
	return encoded, nil
}

func (catalog DirectionReferenceCatalog) ref(kind directionReferenceKind, id string) (string, error) {
	ref, ok := catalog.refByKey[directionReferenceKey(kind, id)]
	if !ok {
		return "", fmt.Errorf("study direction refs: no %s ref for local identity", kind)
	}
	return ref, nil
}

type directionReferenceResolutionError struct {
	code string
}

func (err *directionReferenceResolutionError) Error() string {
	return "study direction refs: candidate reference rejected"
}

func newDirectionReferenceResolutionError(code string) error {
	return &directionReferenceResolutionError{code: code}
}

func directionReferenceResolutionCode(err error) string {
	if typed, ok := err.(*directionReferenceResolutionError); ok {
		return typed.code
	}
	return "invalid_typed_reference"
}

func (catalog DirectionReferenceCatalog) resolve(_ string, kind directionReferenceKind, ref string) (string, error) {
	entry, ok := catalog.byRef[ref]
	if !ok {
		return "", newDirectionReferenceResolutionError("unknown_" + string(kind) + "_ref")
	}
	if entry.Kind != kind {
		return "", newDirectionReferenceResolutionError("wrong_kind_" + string(kind) + "_ref")
	}
	return entry.CanonicalID, nil
}

func (catalog DirectionReferenceCatalog) resolveList(field string, kind directionReferenceKind, refs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(refs))
	ids := make([]string, 0, len(refs))
	for index, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, newDirectionReferenceResolutionError("duplicate_" + string(kind) + "_refs")
		}
		seen[ref] = struct{}{}
		id, err := catalog.resolve(fmt.Sprintf("%s[%d]", field, index), kind, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// RecoverDirectionReferenceProviderJSON extracts only the typed direction
// envelope. It never normalizes a handle or accepts the canonical saved DTO.
func RecoverDirectionReferenceProviderJSON(raw []byte) ([]byte, error) {
	return recoverEditingProviderJSON(
		raw,
		"typed direction proposal",
		func(candidate []byte) error {
			var response directionReferenceProviderResponse
			return decodeEditingJSON(
				candidate,
				maxEditingArtifactBytes,
				"typed direction proposal",
				&response,
			)
		},
	)
}

// DecodeAndResolveDirectionProposalWithDiagnostics accepts only the typed
// provider DTO. Exact handles are restored to canonical IDs before the
// existing candidate normalization and validation assign local direction IDs.
func DecodeAndResolveDirectionProposalWithDiagnostics(
	raw []byte,
	catalog DirectionReferenceCatalog,
) (DirectionProposal, DirectionProposalDiagnostics, error) {
	var envelope directionReferenceProviderResponse
	if err := decodeEditingJSON(raw, maxEditingArtifactBytes, "typed direction proposal", &envelope); err != nil {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, err
	}
	if envelope.Version != DirectionProposalVersion {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, fmt.Errorf(
			"study map: unsupported direction proposal version %d", envelope.Version,
		)
	}
	if envelope.CatalogRef == "" || envelope.CatalogRef != catalog.catalogRef {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, fmt.Errorf(
			"study direction refs: response catalog does not match the exact request",
		)
	}
	items, err := decodeBoundedDirectionItems(envelope.Directions)
	if err != nil {
		return DirectionProposal{}, DirectionProposalDiagnostics{}, err
	}
	diagnostics := DirectionProposalDiagnostics{Received: len(items)}
	directions := make([]DirectionCandidate, 0, len(items))
	seenDirections := make(map[string]struct{}, len(items))
	reject := func(position int, code string) {
		diagnostics.Issues = append(diagnostics.Issues, DirectionProposalIssue{Position: position, Code: code})
	}
	for position, item := range items {
		var provider directionReferenceProviderCandidate
		if err := decodeEditingJSON(item, maxEditingArtifactBytes, "typed direction candidate", &provider); err != nil {
			reject(position, "decode_candidate")
			continue
		}
		if strings.TrimSpace(provider.DirectionID) != "" {
			reject(position, "model_direction_id")
			continue
		}
		resolved, err := catalog.resolveProviderCandidate(position, provider)
		if err != nil {
			reject(position, directionReferenceResolutionCode(err))
			continue
		}
		normalized, err := normalizeDirectionCandidate(resolved)
		if err != nil {
			reject(position, directionCandidateValidationCode(err))
			continue
		}
		if _, duplicate := seenDirections[normalized.DirectionID]; duplicate {
			reject(position, "duplicate_direction_id")
			continue
		}
		seenDirections[normalized.DirectionID] = struct{}{}
		directions = append(directions, normalized)
	}
	diagnostics.Accepted = len(directions)
	diagnostics.Rejected = len(diagnostics.Issues)
	if diagnostics.Accepted == 0 {
		return DirectionProposal{}, diagnostics, fmt.Errorf("study map: direction proposal has no valid candidates")
	}
	return DirectionProposal{Version: envelope.Version, Directions: directions}, diagnostics, nil
}

// DecodeIncompleteDirectionReferences preserves the existing weak Study
// projection from a transport-valid typed response. It resolves only the
// reading anchors that can be published as local navigation; it does not
// weaken the complete candidate contract or infer missing selections.
func DecodeIncompleteDirectionReferences(
	raw []byte,
	bundle Bundle,
) ([]IncompleteDirection, DirectionProposalDiagnostics, error) {
	catalog, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		return nil, DirectionProposalDiagnostics{}, err
	}
	var envelope directionReferenceProviderResponse
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"typed incomplete direction projection",
		&envelope,
	); err != nil {
		return nil, DirectionProposalDiagnostics{}, err
	}
	if envelope.Version != DirectionProposalVersion {
		return nil, DirectionProposalDiagnostics{}, fmt.Errorf(
			"study map: unsupported direction proposal version %d",
			envelope.Version,
		)
	}
	if envelope.CatalogRef == "" || envelope.CatalogRef != catalog.catalogRef {
		return nil, DirectionProposalDiagnostics{}, fmt.Errorf(
			"study direction refs: response catalog does not match the exact request",
		)
	}
	items, err := decodeBoundedDirectionItems(envelope.Directions)
	if err != nil {
		return nil, DirectionProposalDiagnostics{}, err
	}
	diagnostics := DirectionProposalDiagnostics{Received: len(items)}
	result := make([]IncompleteDirection, 0, len(items))
	seenDirections := make(map[string]struct{}, len(items))
	reject := func(position int, code string) {
		diagnostics.Issues = append(diagnostics.Issues, DirectionProposalIssue{
			Position: position,
			Code:     code,
		})
	}
	for position, item := range items {
		var provider directionReferenceProviderCandidate
		if err := decodeEditingJSON(
			item,
			maxEditingArtifactBytes,
			"typed incomplete direction candidate",
			&provider,
		); err != nil {
			reject(position, "decode_candidate")
			continue
		}
		if len(provider.DirectionID) > 256 || strings.TrimSpace(provider.DirectionID) != "" {
			reject(position, "model_direction_id")
			continue
		}
		if !validIncompleteDirectionReferenceScalars(provider) {
			reject(position, "invalid_candidate")
			continue
		}
		reading, err := catalog.resolveIncompleteReadingAnchors(position, provider.ReadingAnchors)
		if err != nil {
			reject(position, "invalid_reading_anchors")
			continue
		}
		anchorIDs := make([]string, 0, len(reading))
		for _, readingAnchor := range reading {
			anchorIDs = append(anchorIDs, readingAnchor.AnchorID)
		}
		directionID := localDirectionID(DirectionCandidate{
			Question:  provider.Question,
			AnchorIDs: anchorIDs,
		})
		if _, duplicate := seenDirections[directionID]; duplicate {
			reject(position, "duplicate_direction_id")
			continue
		}
		seenDirections[directionID] = struct{}{}
		result = append(result, IncompleteDirection{
			DirectionID:     directionID,
			Question:        strings.TrimSpace(provider.Question),
			WhyItMatters:    strings.TrimSpace(provider.WhyItMatters),
			LearningOutcome: strings.TrimSpace(provider.LearningOutcome),
			TargetJob:       provider.TargetJob,
			LearningStage:   provider.LearningStage,
			ReadingAnchors:  reading,
		})
	}
	diagnostics.Accepted = len(result)
	diagnostics.Rejected = len(diagnostics.Issues)
	return result, diagnostics, nil
}

func validIncompleteDirectionReferenceScalars(candidate directionReferenceProviderCandidate) bool {
	if len(candidate.Question) > 512 ||
		len(candidate.WhyItMatters) > 1024 ||
		len(candidate.LearningOutcome) > 1024 {
		return false
	}
	return naturalQuestion(candidate.Question) &&
		validText(candidate.WhyItMatters, 1024, true) &&
		!impliesRuntimeOrder(candidate.WhyItMatters) &&
		validText(candidate.LearningOutcome, 1024, true) &&
		!impliesRuntimeOrder(candidate.LearningOutcome) &&
		validTargetJob(candidate.TargetJob) &&
		validLearningStage(candidate.LearningStage)
}

func (catalog DirectionReferenceCatalog) resolveIncompleteReadingAnchors(
	position int,
	provider []directionReferenceReadingAnchor,
) ([]ReadingAnchor, error) {
	if len(provider) == 0 || len(provider) > 5 {
		return nil, fmt.Errorf("study direction refs: incomplete reading anchor count is outside bounds")
	}
	result := make([]ReadingAnchor, 0, len(provider))
	seen := make(map[string]struct{}, len(provider))
	for index, reading := range provider {
		if len(reading.AnchorRef) > 64 || len(reading.Label) > 64 ||
			len(reading.WhatToLookFor) > 768 ||
			!validText(reading.WhatToLookFor, 768, true) ||
			impliesRuntimeOrder(reading.WhatToLookFor) {
			return nil, fmt.Errorf("study direction refs: invalid incomplete reading anchor")
		}
		label, ok := canonicalProviderReadingLabel(reading.Label)
		if !ok {
			return nil, fmt.Errorf("study direction refs: invalid incomplete reading anchor")
		}
		if _, duplicate := seen[reading.AnchorRef]; duplicate {
			return nil, fmt.Errorf("study direction refs: duplicate incomplete reading anchor")
		}
		seen[reading.AnchorRef] = struct{}{}
		anchorID, err := catalog.resolve(
			fmt.Sprintf("directions[%d].reading_anchors[%d].anchor_ref", position, index),
			directionReferenceAnchor,
			reading.AnchorRef,
		)
		if err != nil {
			return nil, fmt.Errorf("study direction refs: unresolved incomplete reading anchor")
		}
		result = append(result, ReadingAnchor{
			AnchorID: anchorID, Label: label,
			WhatToLookFor: strings.TrimSpace(reading.WhatToLookFor),
		})
	}
	return result, nil
}

func (catalog DirectionReferenceCatalog) resolveProviderCandidate(
	position int,
	provider directionReferenceProviderCandidate,
) (DirectionCandidate, error) {
	anchors, err := catalog.resolveList(
		fmt.Sprintf("directions[%d].anchor_refs", position),
		directionReferenceAnchor,
		provider.AnchorRefs,
	)
	if err != nil {
		return DirectionCandidate{}, err
	}
	documents, err := catalog.resolveList(
		fmt.Sprintf("directions[%d].document_refs", position),
		directionReferenceDocument,
		provider.DocumentRefs,
	)
	if err != nil {
		return DirectionCandidate{}, err
	}
	areas, err := catalog.resolveList(
		fmt.Sprintf("directions[%d].area_refs", position),
		directionReferenceArea,
		provider.AreaRefs,
	)
	if err != nil {
		return DirectionCandidate{}, err
	}
	mechanism := ""
	if provider.MechanismRef != "" {
		mechanism, err = catalog.resolve(
			fmt.Sprintf("directions[%d].mechanism_ref", position),
			directionReferenceMechanism,
			provider.MechanismRef,
		)
		if err != nil {
			return DirectionCandidate{}, err
		}
	}
	reading := make([]ReadingAnchor, 0, len(provider.ReadingAnchors))
	seenReading := make(map[string]struct{}, len(provider.ReadingAnchors))
	for index, item := range provider.ReadingAnchors {
		if _, duplicate := seenReading[item.AnchorRef]; duplicate {
			return DirectionCandidate{}, newDirectionReferenceResolutionError(
				"duplicate_reading_anchor_refs",
			)
		}
		seenReading[item.AnchorRef] = struct{}{}
		anchorID, err := catalog.resolve(
			fmt.Sprintf("directions[%d].reading_anchors[%d].anchor_ref", position, index),
			directionReferenceAnchor,
			item.AnchorRef,
		)
		if err != nil {
			return DirectionCandidate{}, err
		}
		reading = append(reading, ReadingAnchor{
			AnchorID: anchorID, Label: item.Label, WhatToLookFor: item.WhatToLookFor,
		})
	}
	return DirectionCandidate{
		Question: provider.Question, WhyItMatters: provider.WhyItMatters,
		LearningOutcome: provider.LearningOutcome, TargetJob: provider.TargetJob,
		LearningStage: provider.LearningStage, AnchorIDs: anchors,
		DocumentIDs: documents, AreaIDs: areas, MechanismID: mechanism,
		ReadingAnchors: reading, SearchQueries: append([]string(nil), provider.SearchQueries...),
	}, nil
}
