package componentmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	SynthesisRequestVersion = 5
	SynthesisRecordVersion  = 5
	SynthesisPromptVersion  = "architecture-grounding-v7"

	maxSynthesisRequestBytes  = 1 << 20
	maxSynthesisPromptBytes   = maxSynthesisRequestBytes + (16 << 10)
	maxSynthesisResponseBytes = modelresearch.ProviderResponseByteLimit
	maxSynthesisRecordBytes   = modelresearch.SemanticRecordByteLimit
	maxSynthesisWarnings      = 128
	maxRevisionBytes          = 256
	maxProfileBytes           = 128
	maxModelBytes             = 256
)

// SynthesisMemberRef is one short request-local typed member identity. The Ref
// is meaningful only within the exact private catalog compiled for one
// request. Canonical member IDs never enter the provider wire.
type SynthesisMemberRef struct {
	Kind MemberKind `json:"kind"`
	Ref  string     `json:"ref"`
}

// SynthesisAnchorRef is one short request-local typed grounding identity.
// Canonical anchor IDs stay in the private catalog.
type SynthesisAnchorRef struct {
	Kind BehaviorAnchorKind `json:"kind"`
	Ref  string             `json:"ref"`
}

// SynthesisFlowRef keeps local flow identity request-scoped. Flow labels may
// help grouping, but the canonical saved flow ID stays private.
type SynthesisFlowRef string

// SynthesisFact is the provider-visible semantic projection of one local
// fact. Exact locations and provenance remain backend owned.
type SynthesisFact struct {
	Kind      FactKind           `json:"kind"`
	Label     string             `json:"label"`
	Certainty evidence.Certainty `json:"certainty"`
}

type SynthesisFlowParticipation struct {
	FlowRef  SynthesisFlowRef `json:"flow_ref"`
	Evidence SynthesisFact    `json:"evidence"`
}

// SynthesisCandidate is the provider-visible candidate shape. It exposes one
// short typed ref plus bounded semantic labels, never canonical IDs, exact
// paths, locations, providers or scenarios.
type SynthesisCandidate struct {
	Ref            SynthesisMemberRef           `json:"ref"`
	ParentRef      *SynthesisMemberRef          `json:"parent_ref,omitempty"`
	Label          string                       `json:"label"`
	Participations []SynthesisFlowParticipation `json:"flow_participations,omitempty"`
	Facts          []SynthesisFact              `json:"facts"`
}

type SynthesisBehaviorAnchor struct {
	Ref        SynthesisAnchorRef   `json:"ref"`
	Label      string               `json:"label"`
	Certainty  evidence.Certainty   `json:"certainty"`
	MemberRefs []SynthesisMemberRef `json:"member_refs"`
}

// SynthesisFlow keeps a flow request-local while retaining only semantic
// labels and closed local fact classifications.
type SynthesisFlow struct {
	Ref   SynthesisFlowRef `json:"ref"`
	Label string           `json:"label"`
	Facts []SynthesisFact  `json:"facts"`
}

type SynthesisRelation struct {
	From      SynthesisMemberRef     `json:"from"`
	To        SynthesisMemberRef     `json:"to"`
	Kind      StructuralRelationKind `json:"kind"`
	Certainty evidence.Certainty     `json:"certainty"`
}

type SynthesisAnchorBinding struct {
	FlowRef   SynthesisFlowRef   `json:"flow_ref"`
	AnchorRef SynthesisAnchorRef `json:"anchor_ref"`
	MemberRef SynthesisMemberRef `json:"member_ref"`
	Certainty evidence.Certainty `json:"certainty"`
}

// SynthesisRequest is the complete model-visible payload. Version, catalog,
// canonical identity, exact source authority and provider provenance are
// deliberately absent; they are bound privately by prompt/cache/record
// identity.
type SynthesisRequest struct {
	RepositoryArchetype RepositoryArchetype       `json:"repository_archetype"`
	GroundingMode       GroundingMode             `json:"grounding_mode"`
	BehaviorAnchors     []SynthesisBehaviorAnchor `json:"behavior_anchors,omitempty"`
	Flows               []SynthesisFlow           `json:"flows,omitempty"`
	Candidates          []SynthesisCandidate      `json:"candidates"`
	Relations           []SynthesisRelation       `json:"supporting_relations,omitempty"`
	AnchorBindings      []SynthesisAnchorBinding  `json:"flow_anchor_bindings,omitempty"`
}

// SynthesisPrompt is the provider-neutral instruction plus the exact bounded
// request JSON. Transport adapters may wrap these strings in their native chat
// format but must not add repository material.
type SynthesisPrompt struct {
	Version        string `json:"version"`
	OutputLanguage string `json:"output_language"`
	System         string `json:"system"`
	User           string `json:"user"`
}

// ResponseState keeps oversized provider output replayable without storing an
// unbounded response body.
type ResponseState string

const (
	ResponseCaptured         ResponseState = "captured"
	ResponseOversize         ResponseState = "oversize_omitted"
	ResponseSensitiveOmitted ResponseState = "sensitive_omitted"
)

// SynthesisMetadata is saved beside the singular provider call. Validation
// warnings and fallback are outcomes of local Apply, never provider claims.
type SynthesisMetadata struct {
	PromptVersion          string                   `json:"prompt_version"`
	Profile                string                   `json:"profile"`
	Model                  string                   `json:"model"`
	OutputLanguage         string                   `json:"output_language"`
	InputBytes             int                      `json:"input_bytes"`
	LatencyMillis          int64                    `json:"latency_ms"`
	UsageReported          bool                     `json:"usage_reported"`
	InputTokens            int                      `json:"input_tokens,omitempty"`
	OutputTokens           int                      `json:"output_tokens,omitempty"`
	FinishReason           string                   `json:"finish_reason,omitempty"`
	TransportAttempts      int                      `json:"transport_attempts"`
	ResponseComplete       bool                     `json:"response_complete"`
	MembershipCounted      bool                     `json:"response_membership_counted"`
	MemberOccurrences      int                      `json:"response_member_occurrences,omitempty"`
	DistinctMembers        int                      `json:"response_distinct_members,omitempty"`
	ValidationWarnings     []Diagnostic             `json:"validation_warnings,omitempty"`
	ValidationOutcome      ValidationOutcome        `json:"validation_outcome"`
	ArchitectureSource     ArchitectureSource       `json:"architecture_source"`
	ArchitectureLevel      int                      `json:"architecture_level"`
	Normalizations         []NormalizationOperation `json:"normalization_operations,omitempty"`
	OriginalProposalSHA256 string                   `json:"original_proposal_sha256,omitempty"`
	FallbackReason         FallbackReason           `json:"fallback_reason,omitempty"`
}

// SynthesisCall is one already-completed provider interaction. No provider
// interface or network operation lives in this package.
type SynthesisCall struct {
	Metadata      SynthesisMetadata `json:"metadata"`
	ResponseState ResponseState     `json:"response_state"`
	ResponseBytes int               `json:"response_bytes"`
	Response      []byte            `json:"response,omitempty"`
}

// SynthesisRecord intentionally has one optional Call field rather than call
// history. This represents one call for one exact bounded synthesis request.
type SynthesisRecord struct {
	Version              int            `json:"version"`
	RepositoryRevision   string         `json:"repository_revision"`
	CacheKey             string         `json:"cache_key"`
	RequestSHA256        string         `json:"request_sha256"`
	PrivateCatalogSHA256 string         `json:"private_catalog_sha256"`
	Call                 *SynthesisCall `json:"call,omitempty"`
}

type SynthesisResult struct {
	Landscape  Landscape
	Record     SynthesisRecord
	Membership SynthesisMembershipCounts
}

// SynthesisMembershipCounts is exact response cardinality after request-local
// refs have been resolved against the private catalog. It is not inferred by
// reparsing provider bytes in a downstream owner.
type SynthesisMembershipCounts struct {
	Counted           bool
	MemberOccurrences int
	DistinctMembers   int
}

type synthesisPrivateCatalog struct {
	membersByID        map[MemberID]SynthesisMemberRef
	membersByRef       map[string]MemberID
	memberKinds        map[string]MemberKind
	anchorsByID        map[string]SynthesisAnchorRef
	anchorsByRef       map[string]string
	anchorKinds        map[string]BehaviorAnchorKind
	flowsByID          map[FlowID]SynthesisFlowRef
	canonicalOpaqueIDs map[string]struct{}
	identitySHA256     string
}

type synthesisCatalogMemberIdentity struct {
	Ref SynthesisMemberRef `json:"ref"`
	ID  MemberID           `json:"id"`
}

type synthesisCatalogAnchorIdentity struct {
	Ref SynthesisAnchorRef `json:"ref"`
	ID  string             `json:"id"`
}

type synthesisCatalogFlowIdentity struct {
	Ref SynthesisFlowRef `json:"ref"`
	ID  FlowID           `json:"id"`
}

type synthesisCatalogIdentity struct {
	Members []synthesisCatalogMemberIdentity `json:"members"`
	Anchors []synthesisCatalogAnchorIdentity `json:"anchors,omitempty"`
	Flows   []synthesisCatalogFlowIdentity   `json:"flows,omitempty"`
}

func buildSynthesisPrivateCatalog(bundle CandidateBundle) (synthesisPrivateCatalog, error) {
	if err := bundle.Validate(); err != nil {
		return synthesisPrivateCatalog{}, err
	}
	catalog := synthesisPrivateCatalog{
		membersByID:        make(map[MemberID]SynthesisMemberRef, len(bundle.Candidates)),
		membersByRef:       make(map[string]MemberID, len(bundle.Candidates)),
		memberKinds:        make(map[string]MemberKind, len(bundle.Candidates)),
		anchorsByID:        make(map[string]SynthesisAnchorRef, len(bundle.BehaviorAnchors)),
		anchorsByRef:       make(map[string]string, len(bundle.BehaviorAnchors)),
		anchorKinds:        make(map[string]BehaviorAnchorKind, len(bundle.BehaviorAnchors)),
		flowsByID:          make(map[FlowID]SynthesisFlowRef, len(bundle.Flows)),
		canonicalOpaqueIDs: make(map[string]struct{}, len(bundle.Candidates)+len(bundle.BehaviorAnchors)+len(bundle.Flows)),
	}
	identity := synthesisCatalogIdentity{
		Members: make([]synthesisCatalogMemberIdentity, 0, len(bundle.Candidates)),
		Anchors: make([]synthesisCatalogAnchorIdentity, 0, len(bundle.BehaviorAnchors)),
		Flows:   make([]synthesisCatalogFlowIdentity, 0, len(bundle.Flows)),
	}
	// Reserve every backend-owned identity before allocating any model-visible
	// ref. Canonical IDs are opaque, so a perfectly valid local ID may itself be
	// "p1", "a1", or another value from the short-ref namespace.
	for _, candidate := range bundle.Candidates {
		catalog.canonicalOpaqueIDs[candidate.ID.Value] = struct{}{}
	}
	for _, anchor := range bundle.BehaviorAnchors {
		catalog.canonicalOpaqueIDs[anchor.ID] = struct{}{}
	}
	for _, flow := range bundle.Flows {
		catalog.canonicalOpaqueIDs[string(flow.ID)] = struct{}{}
	}
	reservedRefs := make(map[string]struct{}, len(catalog.canonicalOpaqueIDs)+len(bundle.Candidates)+len(bundle.BehaviorAnchors)+len(bundle.Flows))
	for canonicalID := range catalog.canonicalOpaqueIDs {
		reservedRefs[canonicalID] = struct{}{}
	}
	candidates := append([]Candidate(nil), bundle.Candidates...)
	sortCandidates(candidates)
	memberOrdinals := make(map[MemberKind]int)
	for _, candidate := range candidates {
		refValue, nextOrdinal := allocateSynthesisRequestRef(
			synthesisMemberRefPrefix(candidate.ID.Kind),
			memberOrdinals[candidate.ID.Kind],
			reservedRefs,
		)
		memberOrdinals[candidate.ID.Kind] = nextOrdinal
		ref := SynthesisMemberRef{
			Kind: candidate.ID.Kind,
			Ref:  refValue,
		}
		catalog.membersByID[candidate.ID] = ref
		catalog.membersByRef[ref.key()] = candidate.ID
		catalog.memberKinds[ref.Ref] = ref.Kind
		identity.Members = append(identity.Members, synthesisCatalogMemberIdentity{Ref: ref, ID: candidate.ID})
	}
	anchors := append([]BehaviorAnchor(nil), bundle.BehaviorAnchors...)
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].ID < anchors[j].ID })
	anchorOrdinal := 0
	for _, anchor := range anchors {
		refValue, nextOrdinal := allocateSynthesisRequestRef("a", anchorOrdinal, reservedRefs)
		anchorOrdinal = nextOrdinal
		ref := SynthesisAnchorRef{Kind: anchor.Kind, Ref: refValue}
		catalog.anchorsByID[anchor.ID] = ref
		catalog.anchorsByRef[ref.key()] = anchor.ID
		catalog.anchorKinds[ref.Ref] = ref.Kind
		identity.Anchors = append(identity.Anchors, synthesisCatalogAnchorIdentity{Ref: ref, ID: anchor.ID})
	}
	flows := append([]Flow(nil), bundle.Flows...)
	sort.Slice(flows, func(i, j int) bool { return flows[i].ID < flows[j].ID })
	flowOrdinal := 0
	for _, flow := range flows {
		refValue, nextOrdinal := allocateSynthesisRequestRef("q", flowOrdinal, reservedRefs)
		flowOrdinal = nextOrdinal
		ref := SynthesisFlowRef(refValue)
		catalog.flowsByID[flow.ID] = ref
		identity.Flows = append(identity.Flows, synthesisCatalogFlowIdentity{Ref: ref, ID: flow.ID})
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return synthesisPrivateCatalog{}, fmt.Errorf("componentmap: encode private synthesis catalog: %w", err)
	}
	catalog.identitySHA256 = sha256String(encoded)
	return catalog, nil
}

func allocateSynthesisRequestRef(prefix string, ordinal int, reserved map[string]struct{}) (string, int) {
	for {
		ordinal++
		ref := fmt.Sprintf("%s%d", prefix, ordinal)
		if _, blocked := reserved[ref]; blocked {
			continue
		}
		reserved[ref] = struct{}{}
		return ref, ordinal
	}
}

func (ref SynthesisMemberRef) key() string {
	return string(ref.Kind) + "\x00" + ref.Ref
}

func (ref SynthesisAnchorRef) key() string {
	return string(ref.Kind) + "\x00" + ref.Ref
}

func (catalog synthesisPrivateCatalog) containsCanonicalOpaqueID(value string) bool {
	_, exists := catalog.canonicalOpaqueIDs[strings.TrimSpace(value)]
	return exists
}

func (catalog synthesisPrivateCatalog) sanitizeCanonicalOpaqueTokens(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	containsCanonical := false
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		if catalog.containsCanonicalOpaqueID(field) {
			containsCanonical = true
			continue
		}
		kept = append(kept, field)
	}
	if !containsCanonical {
		return trimmed
	}
	return strings.Join(kept, " ")
}

func (catalog synthesisPrivateCatalog) synthesisLabelOrFallback(value, fallback string) string {
	if label := catalog.sanitizeCanonicalOpaqueTokens(value); label != "" {
		return label
	}
	if label := catalog.sanitizeCanonicalOpaqueTokens(fallback); label != "" {
		return label
	}
	for ordinal := 1; ; ordinal++ {
		label := fmt.Sprintf("local-label-%d", ordinal)
		if !catalog.containsCanonicalOpaqueID(label) {
			return label
		}
	}
}

func synthesisMemberRefPrefix(kind MemberKind) string {
	switch kind {
	case MemberPackage:
		return "p"
	case MemberFile:
		return "f"
	case MemberSymbol:
		return "s"
	case MemberEntrypoint:
		return "e"
	case MemberFlow:
		return "w"
	default:
		return "m"
	}
}

// BuildSynthesisRequest validates and canonically orders the bounded local
// inputs before encoding the exact bytes intended for a provider.
func BuildSynthesisRequest(bundle CandidateBundle) (SynthesisRequest, []byte, error) {
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		return SynthesisRequest{}, nil, err
	}
	for index, binding := range bundle.AnchorBindings {
		if _, exists := catalog.anchorsByID[binding.AnchorID]; !exists {
			return SynthesisRequest{}, nil, fmt.Errorf(
				"componentmap: flow_anchor_bindings[%d]: binding references unknown behavior anchor",
				index,
			)
		}
	}
	request := SynthesisRequest{
		RepositoryArchetype: bundle.RepositoryArchetype,
		GroundingMode:       bundle.GroundingMode,
		BehaviorAnchors:     make([]SynthesisBehaviorAnchor, 0, len(bundle.BehaviorAnchors)),
		Candidates:          make([]SynthesisCandidate, 0, len(bundle.Candidates)),
		Flows:               make([]SynthesisFlow, 0, len(bundle.Flows)),
		Relations:           make([]SynthesisRelation, 0, len(bundle.Relations)),
		AnchorBindings:      make([]SynthesisAnchorBinding, 0, len(bundle.AnchorBindings)),
	}
	anchors := append([]BehaviorAnchor(nil), bundle.BehaviorAnchors...)
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].ID < anchors[j].ID })
	for _, anchor := range anchors {
		memberRefs := make([]SynthesisMemberRef, 0, len(anchor.MemberIDs))
		for _, memberID := range anchor.MemberIDs {
			memberRefs = append(memberRefs, catalog.membersByID[memberID])
		}
		sort.Slice(memberRefs, func(i, j int) bool { return memberRefs[i].key() < memberRefs[j].key() })
		request.BehaviorAnchors = append(request.BehaviorAnchors, SynthesisBehaviorAnchor{
			Ref: catalog.anchorsByID[anchor.ID], Label: synthesisAnchorLabel(catalog, anchor.Kind),
			Certainty: anchor.Certainty, MemberRefs: memberRefs,
		})
	}
	candidates := append([]Candidate(nil), bundle.Candidates...)
	sortCandidates(candidates)
	for _, candidate := range candidates {
		projected := SynthesisCandidate{
			Ref: catalog.membersByID[candidate.ID], Label: synthesisCandidateLabel(catalog, candidate),
			Participations: make([]SynthesisFlowParticipation, 0, len(candidate.Participations)),
			Facts:          make([]SynthesisFact, 0, len(candidate.Facts)),
		}
		if candidate.ParentID != nil {
			parent := catalog.membersByID[*candidate.ParentID]
			projected.ParentRef = &parent
		}
		for _, participation := range candidate.Participations {
			projected.Participations = append(projected.Participations, SynthesisFlowParticipation{
				FlowRef:  catalog.flowsByID[participation.FlowID],
				Evidence: projectSynthesisFact(catalog, participation.Evidence),
			})
		}
		for _, fact := range candidate.Facts {
			projected.Facts = append(projected.Facts, projectSynthesisFact(catalog, fact))
		}
		request.Candidates = append(request.Candidates, projected)
	}
	flows := append([]Flow(nil), bundle.Flows...)
	sort.Slice(flows, func(i, j int) bool { return flows[i].ID < flows[j].ID })
	for _, flow := range flows {
		facts := make([]SynthesisFact, 0, len(flow.Facts))
		for _, fact := range flow.Facts {
			facts = append(facts, projectSynthesisFact(catalog, fact))
		}
		label := synthesisSemanticLabel(catalog, MemberFlow, flow.Name)
		label = catalog.synthesisLabelOrFallback(label, "flow")
		request.Flows = append(request.Flows, SynthesisFlow{
			Ref: catalog.flowsByID[flow.ID], Label: label, Facts: facts,
		})
	}
	relations := append([]LocalRelation(nil), bundle.Relations...)
	sort.Slice(relations, func(i, j int) bool { return relations[i].ID < relations[j].ID })
	for _, relation := range relations {
		request.Relations = append(request.Relations, SynthesisRelation{
			From: catalog.membersByID[relation.From], To: catalog.membersByID[relation.To],
			Kind: relation.Kind, Certainty: relation.Certainty,
		})
	}
	bindings := append([]FlowAnchorBinding(nil), bundle.AnchorBindings...)
	sort.Slice(bindings, func(i, j int) bool {
		left := string(bindings[i].FlowID) + "\x00" + bindings[i].AnchorID + "\x00" + bindings[i].MemberID.key()
		right := string(bindings[j].FlowID) + "\x00" + bindings[j].AnchorID + "\x00" + bindings[j].MemberID.key()
		return left < right
	})
	for _, binding := range bindings {
		request.AnchorBindings = append(request.AnchorBindings, SynthesisAnchorBinding{
			FlowRef: catalog.flowsByID[binding.FlowID], AnchorRef: catalog.anchorsByID[binding.AnchorID],
			MemberRef: catalog.membersByID[binding.MemberID], Certainty: binding.Certainty,
		})
	}
	if err := validateSynthesisRequestIdentityFields(catalog, request); err != nil {
		return SynthesisRequest{}, nil, err
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		return SynthesisRequest{}, nil, fmt.Errorf("componentmap: encode synthesis request: %w", err)
	}
	if len(encoded) > maxSynthesisRequestBytes {
		return SynthesisRequest{}, nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: maxSynthesisRequestBytes, Observed: len(encoded), ObservedKnown: true,
		}, nil)
	}
	if synthesisJSONContainsCredential(encoded) {
		return SynthesisRequest{}, nil, fmt.Errorf("componentmap: synthesis request contains an obvious credential")
	}
	return request, encoded, nil
}

type synthesisWireIdentityField struct {
	name string
	ref  string
}

func synthesisRequestIdentityFields(request SynthesisRequest) []synthesisWireIdentityField {
	fields := make([]synthesisWireIdentityField, 0)
	for index, anchor := range request.BehaviorAnchors {
		fields = append(fields, synthesisWireIdentityField{
			name: fmt.Sprintf("behavior_anchors[%d].ref", index), ref: anchor.Ref.Ref,
		})
		for memberIndex, memberRef := range anchor.MemberRefs {
			fields = append(fields, synthesisWireIdentityField{
				name: fmt.Sprintf("behavior_anchors[%d].member_refs[%d]", index, memberIndex), ref: memberRef.Ref,
			})
		}
	}
	for index, flow := range request.Flows {
		fields = append(fields, synthesisWireIdentityField{
			name: fmt.Sprintf("flows[%d].ref", index), ref: string(flow.Ref),
		})
	}
	for index, candidate := range request.Candidates {
		fields = append(fields, synthesisWireIdentityField{
			name: fmt.Sprintf("candidates[%d].ref", index), ref: candidate.Ref.Ref,
		})
		if candidate.ParentRef != nil {
			fields = append(fields, synthesisWireIdentityField{
				name: fmt.Sprintf("candidates[%d].parent_ref", index), ref: candidate.ParentRef.Ref,
			})
		}
		for participationIndex, participation := range candidate.Participations {
			fields = append(fields, synthesisWireIdentityField{
				name: fmt.Sprintf("candidates[%d].flow_participations[%d].flow_ref", index, participationIndex),
				ref:  string(participation.FlowRef),
			})
		}
	}
	for index, relation := range request.Relations {
		fields = append(fields,
			synthesisWireIdentityField{name: fmt.Sprintf("supporting_relations[%d].from", index), ref: relation.From.Ref},
			synthesisWireIdentityField{name: fmt.Sprintf("supporting_relations[%d].to", index), ref: relation.To.Ref},
		)
	}
	for index, binding := range request.AnchorBindings {
		fields = append(fields,
			synthesisWireIdentityField{name: fmt.Sprintf("flow_anchor_bindings[%d].flow_ref", index), ref: string(binding.FlowRef)},
			synthesisWireIdentityField{name: fmt.Sprintf("flow_anchor_bindings[%d].anchor_ref", index), ref: binding.AnchorRef.Ref},
			synthesisWireIdentityField{name: fmt.Sprintf("flow_anchor_bindings[%d].member_ref", index), ref: binding.MemberRef.Ref},
		)
	}
	return fields
}

func validateSynthesisRequestIdentityFields(catalog synthesisPrivateCatalog, request SynthesisRequest) error {
	for _, field := range synthesisRequestIdentityFields(request) {
		if field.ref == "" {
			return fmt.Errorf("componentmap: synthesis request %s is empty", field.name)
		}
		if catalog.containsCanonicalOpaqueID(field.ref) {
			return fmt.Errorf("componentmap: synthesis request %s collides with a private canonical identity", field.name)
		}
	}
	return nil
}

func projectSynthesisFact(catalog synthesisPrivateCatalog, fact LocalFact) SynthesisFact {
	return SynthesisFact{
		Kind: fact.Kind, Label: synthesisFactLabel(catalog, fact), Certainty: fact.Certainty,
	}
}

func synthesisFactLabel(catalog synthesisPrivateCatalog, fact LocalFact) string {
	fallback := synthesisFactFallbackLabel(fact.Kind)
	value := catalog.sanitizeCanonicalOpaqueTokens(fact.Value)
	if value == "" {
		return catalog.synthesisLabelOrFallback("", fallback)
	}
	var label string
	switch fact.Kind {
	case FactRepositoryPath:
		label = path.Base(value)
		if label == "." || label == "/" || label == "" {
			return catalog.synthesisLabelOrFallback("", fallback)
		}
	case FactExecutableRole:
		label = value
	case FactFlowParticipation:
		// The request-local flow ref already binds this evidence to the exact
		// backend-owned flow. Production LocalFact.Value is the canonical FlowID
		// and must not cross the provider boundary as a display label.
		return catalog.synthesisLabelOrFallback("", fallback)
	default:
		label = synthesisSemanticLabel(catalog, MemberSymbol, value)
	}
	label = catalog.sanitizeCanonicalOpaqueTokens(label)
	return catalog.synthesisLabelOrFallback(label, fallback)
}

func synthesisFactFallbackLabel(kind FactKind) string {
	switch kind {
	case FactRepositoryPath:
		return "source file"
	case FactExecutableRole:
		return "executable role"
	case FactFlowParticipation:
		return "flow participation"
	case FactContainment:
		return "containment"
	default:
		return "declaration"
	}
}

func synthesisCandidateLabel(catalog synthesisPrivateCatalog, candidate Candidate) string {
	label := synthesisSemanticLabel(catalog, candidate.ID.Kind, candidate.Name)
	if label == "" {
		for _, fact := range candidate.Facts {
			if fact.Kind == FactDeclaration || fact.Kind == FactRepositoryPath {
				label = synthesisFactLabel(catalog, fact)
				break
			}
		}
	}
	return catalog.synthesisLabelOrFallback(label, strings.ReplaceAll(string(candidate.ID.Kind), "_", " "))
}

func synthesisSemanticLabel(catalog synthesisPrivateCatalog, kind MemberKind, value string) string {
	value = catalog.sanitizeCanonicalOpaqueTokens(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "/") {
		value = path.Base(value)
	}
	if kind == MemberSymbol || kind == MemberEntrypoint {
		if index := strings.IndexByte(value, '.'); index >= 0 && index+1 < len(value) {
			value = value[index+1:]
		}
	}
	value = catalog.sanitizeCanonicalOpaqueTokens(value)
	if value == "" {
		return ""
	}
	if len(value) > maxNameBytes {
		value = truncateDisplayText(value, maxNameBytes)
	}
	return catalog.sanitizeCanonicalOpaqueTokens(value)
}

func synthesisAnchorLabel(catalog synthesisPrivateCatalog, kind BehaviorAnchorKind) string {
	label := strings.ReplaceAll(string(kind), "_", " ")
	return catalog.synthesisLabelOrFallback(label, "architecture anchor")
}

// BuildSynthesisPrompt exposes the actual versioned synthesis instruction used
// by provider adapters. The output schema is intentionally smaller than the
// local Landscape: evidence, relations, certainty, layout, and styling remain
// local authority and cannot be returned by the model.
func BuildSynthesisPrompt(bundle CandidateBundle) (SynthesisPrompt, error) {
	return buildSynthesisPromptForLanguage(bundle, "en")
}

// BuildSynthesisPromptForLanguage keeps the facts wire identical while making
// one explicit stage-owned language contract for the four model-authored
// display fields. Refs, facts, relations and closed labels remain language
// independent.
func BuildSynthesisPromptForLanguage(
	bundle CandidateBundle,
	outputLanguage string,
) (SynthesisPrompt, error) {
	language, err := normalizeSynthesisOutputLanguage(outputLanguage)
	if err != nil {
		return SynthesisPrompt{}, err
	}
	return buildSynthesisPromptForLanguage(bundle, language)
}

func buildSynthesisPromptForLanguage(
	bundle CandidateBundle,
	language string,
) (SynthesisPrompt, error) {
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return SynthesisPrompt{}, err
	}
	system := `You create a compact conceptual architecture landscape from bounded local repository facts.

Use member, anchor, and flow refs as opaque request-local typed values. Copy a ref only into a response field of the same kind. Do not rewrite refs, infer new refs, or mention members absent from the request.
Local semantic facts, compact structural relations, flow participation, and certainty are read-only grouping context. They must never be returned, upgraded, replaced, or converted into execution order. Canonical repository identities, exact source locations, provider provenance, scenarios, catalog identity, versions, and hashes are private and absent from this request.

Return exactly one compact JSON proposal object with this shape:
{"subsystems":[{"name":"short name","description":"short purpose","components":[{"name":"short name","description":"short purpose","member_refs":[{"kind":"package","ref":"p1"}],"anchor_refs":[{"kind":"process_entry","ref":"a1"}],"hypothesis":false}]}]}

The only allowed proposal fields are subsystems, subsystem name/description/components, component name/description/member_refs/anchor_refs/hypothesis, and ref kind/ref. Array order is the conceptual display order. Never repeat a member ref within one component. A genuinely cross-cutting member may appear in several different conceptual components; this expresses participation, not ownership. Never repeat an anchor ref within one component. Omit an uncertain member because local validation retains omissions separately. Every component must contain at least one supplied member ref.

Repository archetype and grounding mode are local facts. A primary pillar is one top-level subsystem; components are nested responsibilities and are not additional primary pillars. When grounding_mode is behavior_grounded or mixed, choose four to seven top-level primary subsystems when the supplied evidence supports that many, never more than eight. Prefer one to four nested components per subsystem and no more than eighteen in total. Every non-hypothesis nested component must cite at least one supplied behavior anchor ref. Set hypothesis true only when a component is explicitly conceptual or package-derived; do not use it merely to avoid available anchors. Separate extension families from support and tooling. Preserve unresolved frontiers. When grounding_mode is package_landscape, describe an honest static package landscape and do not imply behavioral verification.

Do not return versions, catalog identity, hashes, canonical IDs, edges, relations, flow definitions or transitions, fact payloads, repository paths, qualified symbols, test details, evidence, certainty, provenance, scenarios, source locations, coordinates, dimensions, ports, colors, styles, UI settings, markdown, or explanatory prose. Do not claim temporal or runtime behavior from static relations.`
	if language == "ru" {
		system += `

Write only subsystem and component name and description prose in Russian. Preserve technical identifiers, product and library names, protocols, acronyms, supplied refs, JSON keys, and every closed schema value exactly as supplied.`
	} else {
		system += `

Write only subsystem and component name and description prose in English. Preserve technical identifiers, product and library names, protocols, acronyms, supplied refs, JSON keys, and every closed schema value exactly as supplied.`
	}
	user := "Bounded candidate request:\n" + string(requestJSON)
	promptBytes := len(system) + len(user)
	if promptBytes > maxSynthesisPromptBytes {
		return SynthesisPrompt{}, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: maxSynthesisPromptBytes, Observed: promptBytes, ObservedKnown: true,
		}, nil)
	}
	return SynthesisPrompt{
		Version: SynthesisPromptVersion, OutputLanguage: language,
		System: system, User: user,
	}, nil
}

// SynthesisCacheKey binds one conceptual synthesis to the exact bounded local
// request as well as the repository revision and prompt contract.
func SynthesisCacheKey(repositoryRevision string, bundle CandidateBundle) (string, error) {
	return SynthesisCacheKeyForProvider(repositoryRevision, bundle, "", "")
}

// SynthesisCacheKeyForProvider additionally binds the cache to the configured
// provider profile, model and private canonical ref catalog.
func SynthesisCacheKeyForProvider(repositoryRevision string, bundle CandidateBundle, profile, model string) (string, error) {
	return synthesisCacheKeyForProvider(repositoryRevision, bundle, profile, model, "")
}

// SynthesisCacheKeyForProviderAndLanguage additionally isolates provider
// responses whose human-readable prose was requested in another language.
// English intentionally retains the pre-language cache key so current English
// records remain path-compatible.
func SynthesisCacheKeyForProviderAndLanguage(
	repositoryRevision string,
	bundle CandidateBundle,
	profile string,
	model string,
	outputLanguage string,
) (string, error) {
	language, err := normalizeSynthesisOutputLanguage(outputLanguage)
	if err != nil {
		return "", err
	}
	if language == "en" {
		language = ""
	}
	return synthesisCacheKeyForProvider(repositoryRevision, bundle, profile, model, language)
}

func synthesisCacheKeyForProvider(
	repositoryRevision string,
	bundle CandidateBundle,
	profile string,
	model string,
	outputLanguage string,
) (string, error) {
	if err := validateSynthesisRevision(repositoryRevision); err != nil {
		return "", err
	}
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return "", err
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "componentmap-synthesis\nrevision=%s\nrequest_contract=%d\nproposal_contract=%d\nprompt=%s\nprofile=%s\nmodel=%s\n",
		repositoryRevision, SynthesisRequestVersion, ProposalVersion, SynthesisPromptVersion, profile, model)
	if outputLanguage != "" {
		fmt.Fprintf(hash, "language=%s\n", outputLanguage)
	}
	fmt.Fprintf(hash, "request=%s\nprivate_catalog=%s\n", sha256String(requestJSON), catalog.identitySHA256)
	return "component-synthesis-" + hex.EncodeToString(hash.Sum(nil)), nil
}

// RecordSynthesisResponse evaluates an already-received response, records one
// bounded call, and returns the locally authoritative landscape. It performs
// no network I/O.
func RecordSynthesisResponse(
	bundle CandidateBundle,
	repositoryRevision string,
	profile string,
	model string,
	latency time.Duration,
	rawResponse []byte,
) (SynthesisResult, error) {
	return RecordSynthesisResponseForLanguage(
		bundle,
		repositoryRevision,
		profile,
		model,
		"en",
		latency,
		rawResponse,
	)
}

// RecordSynthesisResponseForLanguage records the language identity that shaped
// provider prose. Historical records without this field remain replayable, but
// active cache selection can distinguish them from explicitly authored output.
func RecordSynthesisResponseForLanguage(
	bundle CandidateBundle,
	repositoryRevision string,
	profile string,
	model string,
	outputLanguage string,
	latency time.Duration,
	rawResponse []byte,
) (SynthesisResult, error) {
	if latency < 0 {
		return SynthesisResult{}, fmt.Errorf("componentmap: synthesis latency cannot be negative")
	}
	if err := validateSynthesisLabel("profile", profile, maxProfileBytes); err != nil {
		return SynthesisResult{}, err
	}
	if err := validateSynthesisLabel("model", model, maxModelBytes); err != nil {
		return SynthesisResult{}, err
	}
	language, err := normalizeSynthesisOutputLanguage(outputLanguage)
	if err != nil {
		return SynthesisResult{}, err
	}
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return SynthesisResult{}, err
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		return SynthesisResult{}, err
	}
	prompt, err := BuildSynthesisPromptForLanguage(bundle, language)
	if err != nil {
		return SynthesisResult{}, err
	}
	cacheKey, err := SynthesisCacheKeyForProviderAndLanguage(repositoryRevision, bundle, profile, model, language)
	if err != nil {
		return SynthesisResult{}, err
	}
	if len(rawResponse) > maxSynthesisResponseBytes {
		return SynthesisResult{}, synthesisResourceLimit(
			modelresearch.ResourceLimitResponseBytes,
			maxSynthesisResponseBytes,
			len(rawResponse),
		)
	}

	state := ResponseCaptured
	response := append([]byte(nil), rawResponse...)
	if synthesisResponseContainsCredential(rawResponse) {
		state = ResponseSensitiveOmitted
		response = nil
	}
	landscape, membership, err := evaluateSynthesisResponse(bundle, state, response)
	if err != nil {
		return SynthesisResult{}, err
	}
	record := SynthesisRecord{
		Version:              SynthesisRecordVersion,
		RepositoryRevision:   repositoryRevision,
		CacheKey:             cacheKey,
		RequestSHA256:        sha256String(requestJSON),
		PrivateCatalogSHA256: catalog.identitySHA256,
		Call: &SynthesisCall{
			Metadata: SynthesisMetadata{
				PromptVersion: SynthesisPromptVersion,
				Profile:       profile, Model: model,
				OutputLanguage: language,
				InputBytes:     synthesisPromptSize(prompt), LatencyMillis: latency.Milliseconds(),
				MembershipCounted:      membership.Counted,
				MemberOccurrences:      membership.MemberOccurrences,
				DistinctMembers:        membership.DistinctMembers,
				ValidationWarnings:     cloneDiagnostics(landscape.Diagnostics),
				ValidationOutcome:      landscape.ValidationOutcome,
				ArchitectureSource:     landscape.Source,
				ArchitectureLevel:      landscape.Level,
				Normalizations:         append([]NormalizationOperation(nil), landscape.Normalizations...),
				OriginalProposalSHA256: landscape.OriginalProposalSHA256,
				FallbackReason:         landscape.FallbackReason,
			},
			ResponseState: state, ResponseBytes: len(rawResponse), Response: response,
		},
	}
	if err := validateSynthesisRecord(bundle, repositoryRevision, record); err != nil {
		return SynthesisResult{}, err
	}
	return SynthesisResult{Landscape: landscape, Record: record, Membership: membership}, nil
}

// ReplaySynthesis strictly decodes one saved record, rebuilds the exact local
// request, and re-applies the saved provider response without a provider call.
func ReplaySynthesis(bundle CandidateBundle, repositoryRevision string, saved []byte) (Landscape, error) {
	result, err := ReplaySynthesisResult(bundle, repositoryRevision, saved)
	if err != nil {
		return Landscape{}, err
	}
	return result.Landscape, nil
}

// ReplaySynthesisResult revalidates the saved response and returns the same
// authoritative resolved membership counts that were recorded originally.
func ReplaySynthesisResult(bundle CandidateBundle, repositoryRevision string, saved []byte) (SynthesisResult, error) {
	if len(saved) == 0 {
		return SynthesisResult{}, fmt.Errorf("componentmap: saved synthesis record is empty or too large")
	}
	if len(saved) > maxSynthesisRecordBytes {
		return SynthesisResult{}, synthesisResourceLimit(
			modelresearch.ResourceLimitRecordBytes,
			maxSynthesisRecordBytes,
			len(saved),
		)
	}
	var record SynthesisRecord
	if err := decodeStrictJSON(saved, &record); err != nil {
		return SynthesisResult{}, fmt.Errorf("componentmap: decode synthesis record: %w", err)
	}
	if err := validateSynthesisRecord(bundle, repositoryRevision, record); err != nil {
		return SynthesisResult{}, err
	}

	landscape, membership, err := evaluateSynthesisResponse(bundle, record.Call.ResponseState, record.Call.Response)
	if err != nil {
		return SynthesisResult{}, err
	}
	if !diagnosticsEqual(record.Call.Metadata.ValidationWarnings, landscape.Diagnostics) {
		return SynthesisResult{}, fmt.Errorf("componentmap: saved synthesis validation warnings do not replay")
	}
	if record.Call.Metadata.FallbackReason != landscape.FallbackReason {
		return SynthesisResult{}, fmt.Errorf("componentmap: saved synthesis fallback reason does not replay")
	}
	metadata := record.Call.Metadata
	if metadata.ValidationOutcome != landscape.ValidationOutcome || metadata.ArchitectureSource != landscape.Source ||
		metadata.ArchitectureLevel != landscape.Level || !reflect.DeepEqual(metadata.Normalizations, landscape.Normalizations) ||
		metadata.OriginalProposalSHA256 != landscape.OriginalProposalSHA256 {
		return SynthesisResult{}, fmt.Errorf("componentmap: saved synthesis validation outcome does not replay")
	}
	if metadata.MembershipCounted != membership.Counted ||
		metadata.MemberOccurrences != membership.MemberOccurrences ||
		metadata.DistinctMembers != membership.DistinctMembers {
		return SynthesisResult{}, fmt.Errorf("componentmap: saved synthesis membership counts do not replay")
	}
	return SynthesisResult{Landscape: landscape, Record: record, Membership: membership}, nil
}

func validateSynthesisRecord(bundle CandidateBundle, repositoryRevision string, record SynthesisRecord) error {
	if record.Version != SynthesisRecordVersion {
		return fmt.Errorf("componentmap: unsupported synthesis record version %d", record.Version)
	}
	if err := validateSynthesisRevision(repositoryRevision); err != nil {
		return err
	}
	if record.RepositoryRevision != repositoryRevision {
		return fmt.Errorf("componentmap: synthesis record repository revision does not match")
	}
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return err
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		return err
	}
	if record.RequestSHA256 != sha256String(requestJSON) {
		return fmt.Errorf("componentmap: synthesis record request digest does not match")
	}
	if record.PrivateCatalogSHA256 != catalog.identitySHA256 {
		return fmt.Errorf("componentmap: synthesis record private catalog digest does not match")
	}
	if record.Call == nil {
		return fmt.Errorf("componentmap: synthesis record has no represented call")
	}
	metadata := record.Call.Metadata
	var (
		expectedCacheKey string
		promptLanguage   = "en"
	)
	if metadata.OutputLanguage == "" {
		// Records written before output-language identity existed are still
		// valid historical report artifacts. After bounded replay validation,
		// active cache selection rejects their unknown language.
		expectedCacheKey, err = SynthesisCacheKeyForProvider(repositoryRevision, bundle, metadata.Profile, metadata.Model)
	} else {
		language, languageErr := normalizeSynthesisOutputLanguage(metadata.OutputLanguage)
		if languageErr != nil || language != metadata.OutputLanguage {
			return fmt.Errorf("componentmap: synthesis record output language is invalid")
		}
		expectedCacheKey, err = SynthesisCacheKeyForProviderAndLanguage(
			repositoryRevision,
			bundle,
			metadata.Profile,
			metadata.Model,
			language,
		)
		promptLanguage = language
	}
	if err != nil {
		return err
	}
	prompt, err := BuildSynthesisPromptForLanguage(bundle, promptLanguage)
	if err != nil {
		return err
	}
	if record.CacheKey != expectedCacheKey {
		return fmt.Errorf("componentmap: synthesis record cache key does not match")
	}
	if metadata.PromptVersion != SynthesisPromptVersion {
		return fmt.Errorf("componentmap: synthesis record prompt version does not match")
	}
	if err := validateSynthesisLabel("profile", metadata.Profile, maxProfileBytes); err != nil {
		return err
	}
	if err := validateSynthesisLabel("model", metadata.Model, maxModelBytes); err != nil {
		return err
	}
	if metadata.InputBytes != synthesisPromptSize(prompt) {
		return fmt.Errorf("componentmap: synthesis record input byte count does not match")
	}
	if metadata.LatencyMillis < 0 {
		return fmt.Errorf("componentmap: synthesis record latency cannot be negative")
	}
	if metadata.InputTokens < 0 || metadata.OutputTokens < 0 {
		return fmt.Errorf("componentmap: synthesis record token counts cannot be negative")
	}
	if metadata.MemberOccurrences < 0 || metadata.DistinctMembers < 0 ||
		metadata.DistinctMembers > metadata.MemberOccurrences ||
		(metadata.MemberOccurrences > 0 && !metadata.MembershipCounted) ||
		(metadata.MembershipCounted && metadata.MemberOccurrences > 0 && metadata.DistinctMembers == 0) ||
		(!metadata.MembershipCounted && (metadata.MemberOccurrences != 0 || metadata.DistinctMembers != 0)) {
		return fmt.Errorf("componentmap: synthesis record membership counts are inconsistent")
	}
	if metadata.TransportAttempts < 0 {
		return fmt.Errorf("componentmap: synthesis record transport attempts cannot be negative")
	}
	if len(metadata.FinishReason) > maxProfileBytes || strings.TrimSpace(metadata.FinishReason) != metadata.FinishReason ||
		strings.ContainsAny(metadata.FinishReason, "\r\n\t") {
		return fmt.Errorf("componentmap: synthesis record finish reason is malformed")
	}
	if len(metadata.ValidationWarnings) > maxSynthesisWarnings {
		return fmt.Errorf("componentmap: synthesis record has too many validation warnings")
	}
	for index, warning := range metadata.ValidationWarnings {
		if err := validateDiagnostic(warning); err != nil {
			return fmt.Errorf("componentmap: synthesis validation warning[%d]: %w", index, err)
		}
	}
	if metadata.FallbackReason != "" && !validFallbackReason(metadata.FallbackReason) {
		return fmt.Errorf("componentmap: model-assisted synthesis has invalid fallback reason %q", metadata.FallbackReason)
	}
	if !validValidationOutcome(metadata.ValidationOutcome) || !validArchitectureSource(metadata.ArchitectureSource) ||
		metadata.ArchitectureLevel < 1 || metadata.ArchitectureLevel > 4 {
		return fmt.Errorf("componentmap: synthesis validation outcome metadata is invalid")
	}
	for index, operation := range metadata.Normalizations {
		if err := validateNormalizationOperation(operation); err != nil {
			return fmt.Errorf("componentmap: synthesis normalization[%d]: %w", index, err)
		}
	}
	if metadata.OriginalProposalSHA256 != "" && len(metadata.OriginalProposalSHA256) != 64 {
		return fmt.Errorf("componentmap: synthesis original proposal digest is malformed")
	}
	switch record.Call.ResponseState {
	case ResponseCaptured:
		if len(record.Call.Response) > maxSynthesisResponseBytes {
			return synthesisResourceLimit(
				modelresearch.ResourceLimitResponseBytes,
				maxSynthesisResponseBytes,
				len(record.Call.Response),
			)
		}
		if record.Call.ResponseBytes != len(record.Call.Response) {
			return fmt.Errorf("componentmap: captured synthesis response byte count is invalid")
		}
		if synthesisResponseContainsCredential(record.Call.Response) {
			return fmt.Errorf("componentmap: captured synthesis response violates the obvious credential policy")
		}
	case ResponseOversize:
		if record.Call.ResponseBytes <= maxSynthesisResponseBytes || len(record.Call.Response) != 0 {
			return fmt.Errorf("componentmap: oversized synthesis response record is invalid")
		}
	case ResponseSensitiveOmitted:
		if record.Call.ResponseBytes < 1 || record.Call.ResponseBytes > maxSynthesisResponseBytes || len(record.Call.Response) != 0 {
			return fmt.Errorf("componentmap: sensitive synthesis response record is invalid")
		}
	default:
		return fmt.Errorf("componentmap: invalid synthesis response state %q", record.Call.ResponseState)
	}
	return nil
}

func normalizeSynthesisOutputLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en":
		return "en", nil
	case "ru":
		return "ru", nil
	default:
		return "", fmt.Errorf("componentmap: synthesis output language must be \"en\" or \"ru\"")
	}
}

func evaluateSynthesisResponse(
	bundle CandidateBundle,
	state ResponseState,
	response []byte,
) (Landscape, SynthesisMembershipCounts, error) {
	if state == ResponseOversize {
		return Landscape{}, SynthesisMembershipCounts{}, synthesisResourceLimit(
			modelresearch.ResourceLimitResponseBytes,
			maxSynthesisResponseBytes,
			maxSynthesisResponseBytes+1,
		)
	}
	if state == ResponseSensitiveOmitted {
		landscape, err := synthesisResponseFallback(bundle, newDiagnostic(
			"response.sensitive_omitted",
			"provider response matched the obvious credential policy and was not retained",
		))
		return landscape, SynthesisMembershipCounts{}, err
	}
	object, normalization, responseErr := extractProposalObject(response)
	if responseErr != nil {
		landscape, err := synthesisResponseFallback(bundle, newDiagnostic(responseErr.code, responseErr.message))
		return landscape, SynthesisMembershipCounts{}, err
	}
	wireProposal, unknownFields, err := decodeSynthesisWireProposalJSON(object)
	if err != nil {
		landscape, fallbackErr := synthesisResponseFallback(bundle, newDiagnostic(
			"response.invalid_proposal",
			"recovered json does not satisfy the bounded proposal schema",
		))
		return landscape, SynthesisMembershipCounts{}, fallbackErr
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		return Landscape{}, SynthesisMembershipCounts{}, err
	}
	proposal, resolveErr := resolveSynthesisWireProposal(catalog, wireProposal)
	if resolveErr != nil {
		landscape, err := synthesisResponseFallback(bundle, newDiagnostic(resolveErr.code, resolveErr.message))
		return landscape, SynthesisMembershipCounts{}, err
	}
	membership := synthesisMembershipCounts(proposal)
	landscape, err := Apply(bundle, proposal)
	if err != nil {
		return Landscape{}, SynthesisMembershipCounts{}, err
	}
	if landscape.ValidationOutcome != ValidationRejected {
		// Accepted status describes the canonical model-authored relation after
		// local readable-shape normalization, not a raw response cardinality that
		// normalization may have merged. The deterministic remainder is excluded.
		membership = acceptedSynthesisMembershipCounts(landscape)
	}
	warnings := make([]Diagnostic, 0, 2)
	if normalization != nil {
		warnings = append(warnings, *normalization)
	}
	if unknownFields {
		warnings = append(warnings, newDiagnostic(
			"response.unknown_fields_ignored",
			"ignored bounded response fields outside the conceptual proposal contract",
		))
	}
	if len(warnings) > 0 {
		landscape.Diagnostics = append(warnings, landscape.Diagnostics...)
		if err := landscape.Validate(bundle); err != nil {
			return Landscape{}, SynthesisMembershipCounts{}, err
		}
	}
	return landscape, membership, nil
}

func synthesisMembershipCounts(proposal Proposal) SynthesisMembershipCounts {
	distinct := make(map[MemberID]struct{})
	occurrences := 0
	for _, subsystem := range proposal.Subsystems {
		for _, component := range subsystem.Components {
			for _, memberID := range component.MemberIDs {
				occurrences++
				distinct[memberID] = struct{}{}
			}
		}
	}
	return SynthesisMembershipCounts{
		Counted:           len(proposal.Subsystems) > 0,
		MemberOccurrences: occurrences,
		DistinctMembers:   len(distinct),
	}
}

func acceptedSynthesisMembershipCounts(landscape Landscape) SynthesisMembershipCounts {
	distinct := make(map[MemberID]struct{})
	occurrences := 0
	for _, subsystem := range landscape.Subsystems {
		if subsystem.Category == SubsystemCategoryDiagnostic {
			continue
		}
		for _, component := range subsystem.Components {
			for _, member := range component.Members {
				occurrences++
				distinct[member.ID] = struct{}{}
			}
		}
	}
	return SynthesisMembershipCounts{
		Counted:           true,
		MemberOccurrences: occurrences,
		DistinctMembers:   len(distinct),
	}
}

func synthesisResponseFallback(bundle CandidateBundle, warning Diagnostic) (Landscape, error) {
	landscape, err := Apply(bundle, Proposal{})
	if err != nil {
		return Landscape{}, err
	}
	landscape.Diagnostics = append([]Diagnostic{warning}, landscape.Diagnostics...)
	switch warning.Code {
	case "proposal.unknown_member_id":
		landscape.FallbackReason = FallbackRejectedUnknownMember
	case "proposal.unknown_anchor_id":
		landscape.FallbackReason = FallbackRejectedUnknownAnchor
	}
	if err := landscape.Validate(bundle); err != nil {
		return Landscape{}, err
	}
	return landscape, nil
}

type synthesisResponseError struct {
	code    string
	message string
}

func extractProposalObject(raw []byte) ([]byte, *Diagnostic, *synthesisResponseError) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, &synthesisResponseError{code: "response.no_json", message: "provider response contains no json object"}
	}
	if json.Valid(trimmed) {
		if trimmed[0] != '{' {
			return nil, nil, &synthesisResponseError{code: "response.invalid_proposal", message: "provider response is json but not a proposal object"}
		}
		return append([]byte(nil), trimmed...), nil, nil
	}

	fenced := fencedJSONObjectCandidates(trimmed)
	switch len(fenced) {
	case 1:
		if len(jsonObjectCandidates(trimmed, 2)) > 1 {
			return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several json objects"}
		}
		diagnostic := newDiagnostic("response.fenced_json_extracted", "accepted one bounded proposal object from a markdown fence")
		return fenced[0], &diagnostic, nil
	case 0:
	default:
		return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several fenced json objects"}
	}

	embedded := jsonObjectCandidates(trimmed, 2)
	switch len(embedded) {
	case 1:
		diagnostic := newDiagnostic("response.embedded_json_extracted", "accepted one bounded proposal object embedded in provider prose")
		return embedded[0], &diagnostic, nil
	case 0:
		return nil, nil, &synthesisResponseError{code: "response.no_json", message: "provider response contains no recoverable json object"}
	default:
		return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several json objects"}
	}
}

func synthesisResourceLimit(
	kind modelresearch.ResourceLimitKind,
	limit int,
	observed int,
) *modelresearch.ResourceLimitError {
	return &modelresearch.ResourceLimitError{
		Stage: "architecture_synthesis", Kind: kind,
		Limit: limit, Observed: observed, ObservedKnown: true,
	}
}

type synthesisWireProposal struct {
	Subsystems []synthesisWireSubsystem `json:"subsystems"`
}

type synthesisWireSubsystem struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Components  []synthesisWireComponent `json:"components"`
}

type synthesisWireComponent struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	MemberRefs  []SynthesisMemberRef `json:"member_refs"`
	AnchorRefs  []SynthesisAnchorRef `json:"anchor_refs,omitempty"`
	Hypothesis  bool                 `json:"hypothesis,omitempty"`
}

// decodeSynthesisWireProposalJSON is strict about every known field type while
// tolerating harmless commentary fields. Canonical IDs have no field in the
// active response shape and are never copied from provider bytes.
func decodeSynthesisWireProposalJSON(raw []byte) (synthesisWireProposal, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return synthesisWireProposal{}, false, fmt.Errorf("proposal is not an object")
	}
	if hasForbiddenSynthesisFields(root,
		"version", "catalog", "catalog_ref", "catalog_hash", "private_catalog_sha256",
		"request_sha256", "allowed_paths",
	) {
		return synthesisWireProposal{}, false, fmt.Errorf("proposal contains backend-owned identity fields")
	}
	unknown := hasUnknownFields(root, "subsystems")

	var proposal synthesisWireProposal
	if value, exists := root["subsystems"]; exists {
		var rawSubsystems []json.RawMessage
		if err := json.Unmarshal(value, &rawSubsystems); err != nil {
			return synthesisWireProposal{}, unknown, fmt.Errorf("proposal subsystems have invalid type")
		}
		proposal.Subsystems = make([]synthesisWireSubsystem, 0, len(rawSubsystems))
		for _, rawSubsystem := range rawSubsystems {
			subsystem, itemUnknown, err := decodeSynthesisWireSubsystem(rawSubsystem)
			if err != nil {
				return synthesisWireProposal{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			proposal.Subsystems = append(proposal.Subsystems, subsystem)
		}
	}
	return proposal, unknown, nil
}

func decodeSynthesisWireSubsystem(raw json.RawMessage) (synthesisWireSubsystem, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return synthesisWireSubsystem{}, false, fmt.Errorf("proposal subsystem is not an object")
	}
	if hasForbiddenSynthesisFields(fields, "id", "version", "catalog_ref", "hash") {
		return synthesisWireSubsystem{}, false, fmt.Errorf("proposal subsystem contains backend-owned identity fields")
	}
	unknown := hasUnknownFields(fields, "name", "description", "components")
	name, err := decodeProposalString(fields, "name")
	if err != nil {
		return synthesisWireSubsystem{}, unknown, err
	}
	description, err := decodeProposalString(fields, "description")
	if err != nil {
		return synthesisWireSubsystem{}, unknown, err
	}
	result := synthesisWireSubsystem{Name: name, Description: description}
	if value, exists := fields["components"]; exists {
		var rawComponents []json.RawMessage
		if err := json.Unmarshal(value, &rawComponents); err != nil {
			return synthesisWireSubsystem{}, unknown, fmt.Errorf("proposal components have invalid type")
		}
		result.Components = make([]synthesisWireComponent, 0, len(rawComponents))
		for _, rawComponent := range rawComponents {
			component, itemUnknown, err := decodeSynthesisWireComponent(rawComponent)
			if err != nil {
				return synthesisWireSubsystem{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			result.Components = append(result.Components, component)
		}
	}
	return result, unknown, nil
}

func decodeSynthesisWireComponent(raw json.RawMessage) (synthesisWireComponent, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return synthesisWireComponent{}, false, fmt.Errorf("proposal component is not an object")
	}
	if hasForbiddenSynthesisFields(fields,
		"id", "member_ids", "anchor_ids", "catalog_ref", "hash", "location", "provenance",
	) {
		return synthesisWireComponent{}, false, fmt.Errorf("proposal component contains backend-owned identity fields")
	}
	unknown := hasUnknownFields(fields, "name", "description", "member_refs", "anchor_refs", "hypothesis")
	name, err := decodeProposalString(fields, "name")
	if err != nil {
		return synthesisWireComponent{}, unknown, err
	}
	description, err := decodeProposalString(fields, "description")
	if err != nil {
		return synthesisWireComponent{}, unknown, err
	}
	result := synthesisWireComponent{Name: name, Description: description}
	if value, exists := fields["member_refs"]; exists {
		var rawMemberRefs []json.RawMessage
		if err := json.Unmarshal(value, &rawMemberRefs); err != nil {
			return synthesisWireComponent{}, unknown, fmt.Errorf("proposal member refs have invalid type")
		}
		result.MemberRefs = make([]SynthesisMemberRef, 0, len(rawMemberRefs))
		for _, rawMemberRef := range rawMemberRefs {
			memberRef, itemUnknown, err := decodeSynthesisMemberRef(rawMemberRef)
			if err != nil {
				return synthesisWireComponent{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			result.MemberRefs = append(result.MemberRefs, memberRef)
		}
	}
	if value, exists := fields["anchor_refs"]; exists {
		var rawAnchorRefs []json.RawMessage
		if err := json.Unmarshal(value, &rawAnchorRefs); err != nil {
			return synthesisWireComponent{}, unknown, fmt.Errorf("proposal anchor refs have invalid type")
		}
		result.AnchorRefs = make([]SynthesisAnchorRef, 0, len(rawAnchorRefs))
		for _, rawAnchorRef := range rawAnchorRefs {
			anchorRef, itemUnknown, err := decodeSynthesisAnchorRef(rawAnchorRef)
			if err != nil {
				return synthesisWireComponent{}, unknown || itemUnknown, err
			}
			unknown = unknown || itemUnknown
			result.AnchorRefs = append(result.AnchorRefs, anchorRef)
		}
	}
	if value, exists := fields["hypothesis"]; exists {
		if err := json.Unmarshal(value, &result.Hypothesis); err != nil {
			return synthesisWireComponent{}, unknown, fmt.Errorf("proposal hypothesis has invalid type")
		}
	}
	return result, unknown, nil
}

func decodeSynthesisMemberRef(raw json.RawMessage) (SynthesisMemberRef, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisMemberRef{}, false, fmt.Errorf("proposal member ref is not an object")
	}
	if hasForbiddenSynthesisFields(fields, "id", "value", "member_id", "canonical_id") {
		return SynthesisMemberRef{}, false, fmt.Errorf("proposal member ref contains backend-owned identity fields")
	}
	unknown := hasUnknownFields(fields, "kind", "ref")
	kind, err := decodeProposalString(fields, "kind")
	if err != nil {
		return SynthesisMemberRef{}, unknown, err
	}
	ref, err := decodeProposalString(fields, "ref")
	if err != nil {
		return SynthesisMemberRef{}, unknown, err
	}
	return SynthesisMemberRef{Kind: MemberKind(kind), Ref: ref}, unknown, nil
}

func decodeSynthesisAnchorRef(raw json.RawMessage) (SynthesisAnchorRef, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisAnchorRef{}, false, fmt.Errorf("proposal anchor ref is not an object")
	}
	if hasForbiddenSynthesisFields(fields, "id", "value", "anchor_id", "canonical_id") {
		return SynthesisAnchorRef{}, false, fmt.Errorf("proposal anchor ref contains backend-owned identity fields")
	}
	unknown := hasUnknownFields(fields, "kind", "ref")
	kind, err := decodeProposalString(fields, "kind")
	if err != nil {
		return SynthesisAnchorRef{}, unknown, err
	}
	ref, err := decodeProposalString(fields, "ref")
	if err != nil {
		return SynthesisAnchorRef{}, unknown, err
	}
	return SynthesisAnchorRef{Kind: BehaviorAnchorKind(kind), Ref: ref}, unknown, nil
}

func resolveSynthesisWireProposal(
	catalog synthesisPrivateCatalog,
	wire synthesisWireProposal,
) (Proposal, *synthesisResponseError) {
	proposal := Proposal{Version: ProposalVersion, Subsystems: make([]ProposedSubsystem, 0, len(wire.Subsystems))}
	for _, wireSubsystem := range wire.Subsystems {
		subsystem := ProposedSubsystem{
			Name: wireSubsystem.Name, Description: wireSubsystem.Description,
			Components: make([]ProposedComponent, 0, len(wireSubsystem.Components)),
		}
		for _, wireComponent := range wireSubsystem.Components {
			component := ProposedComponent{
				Name: wireComponent.Name, Description: wireComponent.Description,
				Hypothesis: wireComponent.Hypothesis,
				MemberIDs:  make([]MemberID, 0, len(wireComponent.MemberRefs)),
				AnchorIDs:  make([]string, 0, len(wireComponent.AnchorRefs)),
			}
			for _, memberRef := range wireComponent.MemberRefs {
				if expectedKind, exists := catalog.memberKinds[memberRef.Ref]; exists && expectedKind != memberRef.Kind {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.unknown_member_id", message: "proposal member ref has the wrong request-local kind",
					}
				}
				memberID, exists := catalog.membersByRef[memberRef.key()]
				if !exists {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.unknown_member_id", message: "proposal references an unknown request-local member ref",
					}
				}
				component.MemberIDs = append(component.MemberIDs, memberID)
			}
			seenAnchors := make(map[string]struct{}, len(wireComponent.AnchorRefs))
			for _, anchorRef := range wireComponent.AnchorRefs {
				if expectedKind, exists := catalog.anchorKinds[anchorRef.Ref]; exists && expectedKind != anchorRef.Kind {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.unknown_anchor_id", message: "proposal anchor ref has the wrong request-local kind",
					}
				}
				anchorID, exists := catalog.anchorsByRef[anchorRef.key()]
				if !exists {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.unknown_anchor_id", message: "proposal references an unknown request-local anchor ref",
					}
				}
				if _, duplicate := seenAnchors[anchorID]; duplicate {
					return Proposal{}, &synthesisResponseError{
						code: "response.invalid_proposal", message: "proposal repeats an anchor ref within one component",
					}
				}
				seenAnchors[anchorID] = struct{}{}
				component.AnchorIDs = append(component.AnchorIDs, anchorID)
			}
			subsystem.Components = append(subsystem.Components, component)
		}
		proposal.Subsystems = append(proposal.Subsystems, subsystem)
	}
	return proposal, nil
}

func decodeProposalString(fields map[string]json.RawMessage, name string) (string, error) {
	value, exists := fields[name]
	if !exists {
		return "", nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", fmt.Errorf("proposal field has invalid string type")
	}
	return result, nil
}

func hasUnknownFields(fields map[string]json.RawMessage, allowed ...string) bool {
	known := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		known[field] = struct{}{}
	}
	for field := range fields {
		if _, exists := known[field]; !exists {
			return true
		}
	}
	return false
}

func hasForbiddenSynthesisFields(fields map[string]json.RawMessage, forbidden ...string) bool {
	for _, field := range forbidden {
		if _, exists := fields[field]; exists {
			return true
		}
	}
	return false
}

func fencedJSONObjectCandidates(raw []byte) [][]byte {
	result := make([][]byte, 0, 2)
	for cursor := 0; cursor < len(raw) && len(result) < 2; {
		openOffset := bytes.Index(raw[cursor:], []byte("```"))
		if openOffset < 0 {
			break
		}
		contentStart := cursor + openOffset + 3
		closeOffset := bytes.Index(raw[contentStart:], []byte("```"))
		if closeOffset < 0 {
			break
		}
		contentEnd := contentStart + closeOffset
		content := bytes.TrimSpace(raw[contentStart:contentEnd])
		if newline := bytes.IndexByte(content, '\n'); newline >= 0 && strings.EqualFold(strings.TrimSpace(string(content[:newline])), "json") {
			content = bytes.TrimSpace(content[newline+1:])
		}
		result = append(result, jsonObjectCandidates(content, 2-len(result))...)
		cursor = contentEnd + 3
	}
	return result
}

func jsonObjectCandidates(raw []byte, limit int) [][]byte {
	result := make([][]byte, 0, limit)
	for index := 0; index < len(raw) && len(result) < limit; {
		relative := bytes.IndexByte(raw[index:], '{')
		if relative < 0 {
			break
		}
		start := index + relative
		decoder := json.NewDecoder(bytes.NewReader(raw[start:]))
		var candidate json.RawMessage
		if err := decoder.Decode(&candidate); err != nil || len(candidate) == 0 || candidate[0] != '{' {
			index = start + 1
			continue
		}
		result = append(result, append([]byte(nil), candidate...))
		consumed := int(decoder.InputOffset())
		if consumed <= 0 {
			consumed = 1
		}
		index = start + consumed
	}
	return result
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing json value")
		}
		return err
	}
	return nil
}

func validateSynthesisRevision(revision string) error {
	if len(revision) == 0 || len(revision) > maxRevisionBytes || strings.TrimSpace(revision) != revision || !utf8.ValidString(revision) {
		return fmt.Errorf("componentmap: repository revision is empty, malformed, or too long")
	}
	for _, char := range revision {
		if char <= 0x20 || char == 0x7f {
			return fmt.Errorf("componentmap: repository revision contains whitespace or control characters")
		}
	}
	return nil
}

func validateSynthesisLabel(field, value string, limit int) error {
	if err := validateDisplayText(field, value, limit, true); err != nil {
		return fmt.Errorf("componentmap: synthesis %w", err)
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("componentmap: synthesis %s contains control whitespace", field)
	}
	return nil
}

func sha256String(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func synthesisPromptSize(prompt SynthesisPrompt) int {
	return len(prompt.System) + len(prompt.User)
}

func synthesisJSONContainsCredential(encoded []byte) bool {
	if _, sensitive := secretscan.Detect(string(encoded)); sensitive {
		return true
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return true
	}
	var inspect func(any) bool
	inspect = func(current any) bool {
		switch typed := current.(type) {
		case string:
			_, sensitive := secretscan.Detect(typed)
			return sensitive
		case []any:
			for _, item := range typed {
				if inspect(item) {
					return true
				}
			}
		case map[string]any:
			for key, item := range typed {
				if inspect(key) || inspect(item) {
					return true
				}
			}
		}
		return false
	}
	return inspect(value)
}

func synthesisResponseContainsCredential(response []byte) bool {
	if _, sensitive := secretscan.Detect(string(response)); sensitive {
		return true
	}
	for _, object := range jsonObjectCandidates(response, 2) {
		if synthesisJSONContainsCredential(object) {
			return true
		}
	}
	return false
}

func cloneDiagnostics(values []Diagnostic) []Diagnostic {
	if values == nil {
		return nil
	}
	result := make([]Diagnostic, len(values))
	for index, value := range values {
		result[index] = value
		if value.Member != nil {
			member := *value.Member
			result[index].Member = &member
		}
	}
	return result
}

func diagnosticsEqual(left, right []Diagnostic) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !reflect.DeepEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}
