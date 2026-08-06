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
	SynthesisRequestVersion = 14
	SynthesisRecordVersion  = 11
	SynthesisPromptVersion  = "architecture-grounding-v17"

	maxSynthesisRequestBytes  = 1 << 20
	maxSynthesisPromptBytes   = maxSynthesisRequestBytes + (16 << 10)
	maxSynthesisResponseBytes = modelresearch.ProviderResponseByteLimit
	maxSynthesisRecordBytes   = modelresearch.SemanticRecordByteLimit
	maxSynthesisWarnings      = 128
	maxRevisionBytes          = 256
	maxProfileBytes           = 128
	maxModelBytes             = 256
	maxSynthesisWireRecords   = MaxPrimarySubsystems + MaxTotalNestedComponents
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
	ProofMode  AnchorProofMode      `json:"proof_mode"`
	Label      string               `json:"label"`
	Certainty  evidence.Certainty   `json:"certainty"`
	MemberRefs []SynthesisMemberRef `json:"member_refs"`
}

// SynthesisStructuralLocator is complete read-only containment context. Its
// ref is never a conceptual member ref and therefore cannot be returned in a
// provider component. ParentRefs and ChildRefs name only exact conceptual
// candidates from this same request.
type SynthesisStructuralLocator struct {
	Ref        SynthesisMemberRef   `json:"ref"`
	Label      string               `json:"label"`
	ParentRefs []SynthesisMemberRef `json:"parent_refs"`
	ChildRefs  []SynthesisMemberRef `json:"child_refs"`
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
	RequiredMemberRefs  []SynthesisMemberRef      `json:"required_member_refs,omitempty"`
	BehaviorAnchors     []SynthesisBehaviorAnchor `json:"behavior_anchors,omitempty"`
	Flows               []SynthesisFlow           `json:"flows,omitempty"`
	Candidates          []SynthesisCandidate      `json:"candidates,omitempty"`
	// Units is the Decision 216 bounded local unit catalog. When present,
	// the model groups request-local unit refs (u*) instead of raw
	// package/symbol candidates; backend expansion restores exact
	// membership. Empty for legacy callers that still send raw candidates.
	Units             []SynthesisUnit              `json:"units,omitempty"`
	StructuralContext []SynthesisStructuralLocator `json:"structural_context,omitempty"`
	Relations         []SynthesisRelation          `json:"supporting_relations,omitempty"`
	AnchorBindings    []SynthesisAnchorBinding     `json:"flow_anchor_bindings,omitempty"`
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
	RequestedMemberIDs     []MemberID               `json:"requested_member_ids,omitempty"`
	CoveredMemberIDs       []MemberID               `json:"covered_member_ids,omitempty"`
	UncoveredMemberIDs     []MemberID               `json:"uncovered_member_ids,omitempty"`
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
	Version                int            `json:"version"`
	RepositoryRevision     string         `json:"repository_revision"`
	CacheKey               string         `json:"cache_key"`
	RequestSHA256          string         `json:"request_sha256"`
	PrivateCatalogSHA256   string         `json:"private_catalog_sha256"`
	ProviderRequestSHA256  string         `json:"provider_request_sha256"`
	ProviderEndpointSHA256 string         `json:"provider_endpoint_sha256"`
	Call                   *SynthesisCall `json:"call,omitempty"`
}

// SynthesisProviderIdentity binds a saved semantic result to the exact
// external request body and provider endpoint identity used for the call. The
// endpoint digest must be computed from non-secret canonical endpoint identity;
// Authorization and credentials never belong in this record.
type SynthesisProviderIdentity struct {
	RequestSHA256  string
	EndpointSHA256 string
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
	Counted            bool
	MemberOccurrences  int
	DistinctMembers    int
	RequestedMemberIDs []MemberID
	CoveredMemberIDs   []MemberID
	UncoveredMemberIDs []MemberID
}

func (counts SynthesisMembershipCounts) CoverageComplete() bool {
	return counts.Counted && len(counts.RequestedMemberIDs) > 0 && len(counts.UncoveredMemberIDs) == 0
}

type synthesisPrivateCatalog struct {
	membersByID        map[MemberID]SynthesisMemberRef
	membersByRef       map[string]MemberID
	memberKinds        map[string]MemberKind
	memberRoles        map[string]CandidateRole
	anchorsByID        map[string]SynthesisAnchorRef
	anchorsByRef       map[string]string
	anchorKinds        map[string]BehaviorAnchorKind
	flowsByID          map[FlowID]SynthesisFlowRef
	canonicalOpaqueIDs map[string]struct{}
	identitySHA256     string
}

type synthesisCatalogMemberIdentity struct {
	Ref      SynthesisMemberRef `json:"ref"`
	ID       MemberID           `json:"id"`
	Role     CandidateRole      `json:"role"`
	ParentID *MemberID          `json:"parent_id,omitempty"`
}

type synthesisCatalogAnchorIdentity struct {
	Ref       SynthesisAnchorRef `json:"ref"`
	ID        string             `json:"id"`
	ProofMode AnchorProofMode    `json:"proof_mode"`
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
		memberRoles:        make(map[string]CandidateRole, len(bundle.Candidates)),
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
		catalog.memberRoles[ref.key()] = candidate.Role
		identity.Members = append(identity.Members, synthesisCatalogMemberIdentity{
			Ref: ref, ID: candidate.ID, Role: candidate.Role, ParentID: cloneMemberID(candidate.ParentID),
		})
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
		identity.Anchors = append(identity.Anchors, synthesisCatalogAnchorIdentity{
			Ref: ref, ID: anchor.ID, ProofMode: anchor.ProofMode,
		})
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

func cloneMemberID(id *MemberID) *MemberID {
	if id == nil {
		return nil
	}
	cloned := *id
	return &cloned
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
		RequiredMemberRefs:  make([]SynthesisMemberRef, 0, len(bundle.Candidates)),
		BehaviorAnchors:     make([]SynthesisBehaviorAnchor, 0, len(bundle.BehaviorAnchors)),
		Candidates:          make([]SynthesisCandidate, 0, len(bundle.Candidates)),
		StructuralContext:   make([]SynthesisStructuralLocator, 0),
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
			Ref: catalog.anchorsByID[anchor.ID], ProofMode: anchor.ProofMode,
			Label:     synthesisAnchorLabel(catalog, anchor.Kind),
			Certainty: anchor.Certainty, MemberRefs: memberRefs,
		})
	}
	candidates := append([]Candidate(nil), bundle.Candidates...)
	sortCandidates(candidates)
	candidatesByID := make(map[MemberID]Candidate, len(candidates))
	childrenByParent := make(map[MemberID][]Candidate)
	for _, candidate := range candidates {
		candidatesByID[candidate.ID] = candidate
		if candidate.ParentID != nil {
			childrenByParent[*candidate.ParentID] = append(childrenByParent[*candidate.ParentID], candidate)
		}
	}
	for _, candidate := range candidates {
		if candidate.Role != CandidateRoleConceptualMember {
			continue
		}
		projected := SynthesisCandidate{
			Ref: catalog.membersByID[candidate.ID], Label: synthesisCandidateLabel(catalog, candidate),
			Participations: make([]SynthesisFlowParticipation, 0, len(candidate.Participations)),
			Facts:          make([]SynthesisFact, 0, len(candidate.Facts)),
		}
		if candidate.ParentID != nil && candidatesByID[*candidate.ParentID].Role == CandidateRoleConceptualMember {
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
		request.RequiredMemberRefs = append(request.RequiredMemberRefs, projected.Ref)
	}
	for _, candidate := range candidates {
		if candidate.Role != CandidateRoleStructuralLocator {
			continue
		}
		locator := SynthesisStructuralLocator{
			Ref: catalog.membersByID[candidate.ID], Label: synthesisCandidateLabel(catalog, candidate),
			ParentRefs: make([]SynthesisMemberRef, 0, 1),
			ChildRefs:  make([]SynthesisMemberRef, 0, len(childrenByParent[candidate.ID])),
		}
		if candidate.ParentID != nil {
			locator.ParentRefs = append(locator.ParentRefs, catalog.membersByID[*candidate.ParentID])
		}
		for _, child := range childrenByParent[candidate.ID] {
			locator.ChildRefs = append(locator.ChildRefs, catalog.membersByID[child.ID])
		}
		sort.Slice(locator.ParentRefs, func(i, j int) bool { return locator.ParentRefs[i].key() < locator.ParentRefs[j].key() })
		sort.Slice(locator.ChildRefs, func(i, j int) bool { return locator.ChildRefs[i].key() < locator.ChildRefs[j].key() })
		request.StructuralContext = append(request.StructuralContext, locator)
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
	// Decision 216: compile the bounded local unit catalog alongside the raw
	// candidate projection. The unit refs (u*) let the model group bounded
	// units; backend expansion restores exact membership. Raw candidates
	// remain in the wire until the response contract is unit-only.
	unitCatalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		return SynthesisRequest{}, nil, fmt.Errorf("componentmap: compile architecture units: %w", err)
	}
	request.Units = append([]SynthesisUnit(nil), unitCatalog.WireUnits...)
	for _, relation := range relations {
		if candidatesByID[relation.From].Role != CandidateRoleConceptualMember ||
			candidatesByID[relation.To].Role != CandidateRoleConceptualMember {
			continue
		}
		// Decision 223 (completing Decision 216): when a unit catalog is
		// present the model groups u* unit refs and cannot act on raw
		// p*-level package-import edges. Those edges are represented by
		// the per-unit RelationOutCount aggregate; behavior_handoff
		// relations stay as exact read-only grouping context.
		if len(unitCatalog.WireUnits) > 0 && relation.Kind == StructuralRelationPackageImport {
			continue
		}
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
	if err := validateSynthesisRequestCoverage(request); err != nil {
		return SynthesisRequest{}, nil, err
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

// validateSynthesisRequestCoverage keeps the provider-visible checklist
// mechanically identical to the complete ordered candidate ref set. Parent
// refs are deliberately irrelevant: they are context, not independent
// evidence that a candidate was assigned to a conceptual component.
func validateSynthesisRequestCoverage(request SynthesisRequest) error {
	if len(request.RequiredMemberRefs) != len(request.Candidates) {
		return fmt.Errorf(
			"componentmap: required_member_refs count %d does not match candidate count %d",
			len(request.RequiredMemberRefs), len(request.Candidates),
		)
	}
	seen := make(map[string]struct{}, len(request.RequiredMemberRefs))
	for index, required := range request.RequiredMemberRefs {
		if required.Ref == "" {
			return fmt.Errorf("componentmap: required_member_refs[%d] is empty", index)
		}
		key := required.key()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("componentmap: required_member_refs[%d] duplicates an earlier checklist ref", index)
		}
		seen[key] = struct{}{}
		if required != request.Candidates[index].Ref {
			return fmt.Errorf(
				"componentmap: required_member_refs[%d] does not match candidates[%d].ref",
				index, index,
			)
		}
	}
	seenLocatorRefs := make(map[string]struct{}, len(request.StructuralContext))
	allRefs := make(map[string]struct{}, len(seen)+len(request.StructuralContext))
	for key := range seen {
		allRefs[key] = struct{}{}
	}
	for index, locator := range request.StructuralContext {
		if locator.Ref.Ref == "" {
			return fmt.Errorf("componentmap: structural_context[%d].ref is empty", index)
		}
		key := locator.Ref.key()
		if _, conceptual := seen[key]; conceptual {
			return fmt.Errorf("componentmap: structural_context[%d].ref is also a conceptual candidate", index)
		}
		if _, duplicate := allRefs[key]; duplicate {
			return fmt.Errorf("componentmap: structural_context[%d].ref duplicates an earlier locator", index)
		}
		allRefs[key] = struct{}{}
	}
	for index, locator := range request.StructuralContext {
		if err := validateDisplayText("structural locator label", locator.Label, maxNameBytes, true); err != nil {
			return fmt.Errorf("componentmap: structural_context[%d]: %w", index, err)
		}
		key := locator.Ref.key()
		if _, duplicate := seenLocatorRefs[key]; duplicate {
			return fmt.Errorf("componentmap: structural_context[%d].ref duplicates an earlier locator", index)
		}
		seenLocatorRefs[key] = struct{}{}
		seenParents := make(map[string]struct{}, len(locator.ParentRefs))
		for parentIndex, parentRef := range locator.ParentRefs {
			parentKey := parentRef.key()
			if _, known := allRefs[parentKey]; !known {
				return fmt.Errorf("componentmap: structural_context[%d].parent_refs[%d] is not a request member", index, parentIndex)
			}
			if _, duplicate := seenParents[parentKey]; duplicate {
				return fmt.Errorf("componentmap: structural_context[%d] repeats a parent ref", index)
			}
			seenParents[parentKey] = struct{}{}
		}
		seenChildren := make(map[string]struct{}, len(locator.ChildRefs))
		for childIndex, childRef := range locator.ChildRefs {
			childKey := childRef.key()
			if _, known := allRefs[childKey]; !known {
				return fmt.Errorf("componentmap: structural_context[%d].child_refs[%d] is not a request member", index, childIndex)
			}
			if _, duplicate := seenChildren[childKey]; duplicate {
				return fmt.Errorf("componentmap: structural_context[%d] repeats a child ref", index)
			}
			seenChildren[childKey] = struct{}{}
		}
	}
	return nil
}

type synthesisWireIdentityField struct {
	name string
	ref  string
}

func synthesisRequestIdentityFields(request SynthesisRequest) []synthesisWireIdentityField {
	fields := make([]synthesisWireIdentityField, 0)
	for index, memberRef := range request.RequiredMemberRefs {
		fields = append(fields, synthesisWireIdentityField{
			name: fmt.Sprintf("required_member_refs[%d]", index), ref: memberRef.Ref,
		})
	}
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
	for index, locator := range request.StructuralContext {
		fields = append(fields, synthesisWireIdentityField{
			name: fmt.Sprintf("structural_context[%d].ref", index), ref: locator.Ref.Ref,
		})
		for parentIndex, parentRef := range locator.ParentRefs {
			fields = append(fields, synthesisWireIdentityField{
				name: fmt.Sprintf("structural_context[%d].parent_refs[%d]", index, parentIndex), ref: parentRef.Ref,
			})
		}
		for childIndex, childRef := range locator.ChildRefs {
			fields = append(fields, synthesisWireIdentityField{
				name: fmt.Sprintf("structural_context[%d].child_refs[%d]", index, childIndex), ref: childRef.Ref,
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
		if !strings.Contains(value, "/") {
			// A root-level repository path is already its own basename. Exposing
			// it would violate the path-free Architecture wire contract.
			return catalog.synthesisLabelOrFallback("", fallback)
		}
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
	if kind == MemberFile && !strings.Contains(value, "/") {
		// File producers commonly use the repository-relative path as Name.
		// For a root-level file there is no safe basename-only projection.
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

Use conceptual member, anchor, and flow refs as opaque request-local typed values. Copy a ref only into a response field of the same kind. Do not rewrite refs, infer new refs, or mention members absent from the request. Refs under structural_context are read-only locator context and must never occur in response member_refs.
Local semantic facts, compact structural relations, structural locator containment, flow participation, anchor proof_mode, and certainty are read-only grouping context. They must never be returned, upgraded, replaced, or converted into execution order. A declaration_family anchor is static declaration context and never proves runtime behavior. Canonical repository identities, exact source locations, provider provenance, scenarios, catalog identity, versions, and hashes are private and absent from this request.

Return exactly one compact JSON proposal object with one ordered records array. Use this exact tagged-record grammar:
{"records":[{"kind":"subsystem","ref":"g1","name":"first subsystem","description":"first purpose"},{"kind":"component","subsystem_ref":"g1","name":"first component","description":"first responsibility","unit_refs":[{"kind":"package","ref":"u1"}],"anchor_refs":[{"kind":"process_entry","ref":"a1"}],"hypothesis":false},{"kind":"subsystem","ref":"g2","name":"second subsystem","description":"second purpose"},{"kind":"component","subsystem_ref":"g2","name":"second component","description":"second responsibility","unit_refs":[{"kind":"package","ref":"u2"}],"anchor_refs":[],"hypothesis":true}]}

The entire response must parse as exactly one complete JSON object. Its only root field is records. A subsystem record contains exactly kind, ref, name, and description. Its ref is a unique response-local value g1, g2, and so on; it is not a supplied request ref. A component record contains exactly kind, subsystem_ref, name, description, either unit_refs or member_refs, anchor_refs, and hypothesis. When the request supplies a units catalog, group unit_refs (u*): copy each unit ref exactly as supplied and never split one unit across components. When the request has no units catalog, group member_refs (p*/s*/f*) instead; never mix unit_refs and member_refs in one component. Copy subsystem_ref exactly from one subsystem record. Do not nest records or emit a second root object. Before returning, silently validate the complete JSON syntax, every record kind, every unique subsystem ref, and every exact subsystem_ref, then return only that one object.

Records are in conceptual display order. Emit each subsystem record followed by its component records. Never repeat a unit ref within one component. Never repeat an anchor ref within one component. units is the exhaustive bounded local unit catalog available for grouping: group units coherently, do not invent, rename, or rewrite unit refs. A unit may legitimately participate in several components when it genuinely serves several conceptual roles; this expresses participation, not exclusive ownership, and is accepted. An exact partial grouping is valid: omitted units remain in a deterministic local unclassified remainder and must not be echoed, renamed, or placed in a model-authored remainder. At least one supplied unit ref must be returned. Before returning, collect the distinct unit_refs from every component and self-check them: every returned unit ref must be an exact supplied unit ref, with no unknown or wrong-kind ref. Cross-cutting repeats count once for this identity self-check. Every component must contain at least one supplied unit ref (or, under the legacy member contract, at least one supplied conceptual member ref).

Repository archetype and grounding mode are local facts. A primary pillar is one subsystem record; component records are nested responsibilities and are not additional primary pillars. When grounding_mode is behavior_grounded or mixed, prefer four to seven distinct subsystem records when the supplied evidence supports that many, never more than twelve. Tiny, library, and package-landscape requests may honestly use one to three. Prefer one to four component records per subsystem and no more than forty-eight component records in total. hypothesis is required wire syntax but only advisory model input: the backend derives the product hypothesis status exclusively from exact process_entry/call_target proof scoped to every component member. declaration_family never proves operational grounding. Separate extension families from support and tooling. Preserve unresolved frontiers. When grounding_mode is package_landscape, describe an honest static package landscape and do not imply behavioral verification.

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
	return recordSynthesisResponseForLanguage(
		bundle,
		repositoryRevision,
		profile,
		model,
		outputLanguage,
		latency,
		rawResponse,
	)
}

// RecordSynthesisResponseForLanguageAndProvider evaluates an already-received
// response and binds the resulting persistable record to the exact external
// provider request body and non-secret endpoint identity. Runtime/cache owners
// should use this entrypoint; RecordSynthesisResponseForLanguage remains useful
// for provider-neutral evaluation before an explicit binding is available.
func RecordSynthesisResponseForLanguageAndProvider(
	bundle CandidateBundle,
	repositoryRevision string,
	profile string,
	model string,
	outputLanguage string,
	providerIdentity SynthesisProviderIdentity,
	latency time.Duration,
	rawResponse []byte,
) (SynthesisResult, error) {
	result, err := recordSynthesisResponseForLanguage(
		bundle,
		repositoryRevision,
		profile,
		model,
		outputLanguage,
		latency,
		rawResponse,
	)
	if err != nil {
		return SynthesisResult{}, err
	}
	return BindSynthesisProviderIdentity(bundle, repositoryRevision, result, providerIdentity)
}

func recordSynthesisResponseForLanguage(
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
				RequestedMemberIDs:     append([]MemberID(nil), membership.RequestedMemberIDs...),
				CoveredMemberIDs:       append([]MemberID(nil), membership.CoveredMemberIDs...),
				UncoveredMemberIDs:     append([]MemberID(nil), membership.UncoveredMemberIDs...),
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
	if err := validateSynthesisRecord(bundle, repositoryRevision, record, false); err != nil {
		return SynthesisResult{}, err
	}
	return SynthesisResult{Landscape: landscape, Record: record, Membership: membership}, nil
}

// BindSynthesisProviderIdentity returns an independently validated copy of a
// provider-neutral result whose record is safe to persist or replay. It does
// not retain endpoint text, request bytes, credentials, or Authorization.
func BindSynthesisProviderIdentity(
	bundle CandidateBundle,
	repositoryRevision string,
	result SynthesisResult,
	identity SynthesisProviderIdentity,
) (SynthesisResult, error) {
	if err := validateSynthesisProviderIdentity(identity); err != nil {
		return SynthesisResult{}, err
	}
	result.Record.ProviderRequestSHA256 = identity.RequestSHA256
	result.Record.ProviderEndpointSHA256 = identity.EndpointSHA256
	if err := validateSynthesisRecord(bundle, repositoryRevision, result.Record, true); err != nil {
		return SynthesisResult{}, err
	}
	return result, nil
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
	return replaySynthesisResult(bundle, repositoryRevision, SynthesisProviderIdentity{}, false, saved)
}

// ReplaySynthesisResultForProvider additionally proves that a persisted record
// belongs to the exact external request body and endpoint identity selected by
// the active cache owner. A digest mismatch is a closed cache miss/rejection.
func ReplaySynthesisResultForProvider(
	bundle CandidateBundle,
	repositoryRevision string,
	providerIdentity SynthesisProviderIdentity,
	saved []byte,
) (SynthesisResult, error) {
	if err := validateSynthesisProviderIdentity(providerIdentity); err != nil {
		return SynthesisResult{}, err
	}
	return replaySynthesisResult(bundle, repositoryRevision, providerIdentity, true, saved)
}

func replaySynthesisResult(
	bundle CandidateBundle,
	repositoryRevision string,
	providerIdentity SynthesisProviderIdentity,
	requireExpectedProvider bool,
	saved []byte,
) (SynthesisResult, error) {
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
	if err := validateSynthesisRecord(bundle, repositoryRevision, record, true); err != nil {
		return SynthesisResult{}, err
	}
	if requireExpectedProvider && (record.ProviderRequestSHA256 != providerIdentity.RequestSHA256 ||
		record.ProviderEndpointSHA256 != providerIdentity.EndpointSHA256) {
		return SynthesisResult{}, fmt.Errorf("componentmap: synthesis record external provider identity does not match")
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
		metadata.DistinctMembers != membership.DistinctMembers ||
		!reflect.DeepEqual(metadata.RequestedMemberIDs, membership.RequestedMemberIDs) ||
		!reflect.DeepEqual(metadata.CoveredMemberIDs, membership.CoveredMemberIDs) ||
		!reflect.DeepEqual(metadata.UncoveredMemberIDs, membership.UncoveredMemberIDs) {
		return SynthesisResult{}, fmt.Errorf("componentmap: saved synthesis membership counts do not replay")
	}
	return SynthesisResult{Landscape: landscape, Record: record, Membership: membership}, nil
}

func validateSynthesisRecord(
	bundle CandidateBundle,
	repositoryRevision string,
	record SynthesisRecord,
	requireProviderIdentity bool,
) error {
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
	if requireProviderIdentity {
		if err := validateSynthesisProviderIdentity(SynthesisProviderIdentity{
			RequestSHA256: record.ProviderRequestSHA256, EndpointSHA256: record.ProviderEndpointSHA256,
		}); err != nil {
			return err
		}
	} else if record.ProviderRequestSHA256 != "" || record.ProviderEndpointSHA256 != "" {
		if err := validateSynthesisProviderIdentity(SynthesisProviderIdentity{
			RequestSHA256: record.ProviderRequestSHA256, EndpointSHA256: record.ProviderEndpointSHA256,
		}); err != nil {
			return err
		}
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
	if err := validateSynthesisMembershipMetadata(bundle, metadata); err != nil {
		return err
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

func validateSynthesisMembershipMetadata(bundle CandidateBundle, metadata SynthesisMetadata) error {
	if metadata.MemberOccurrences < 0 || metadata.DistinctMembers < 0 ||
		metadata.DistinctMembers > metadata.MemberOccurrences {
		return fmt.Errorf("componentmap: synthesis record membership counts are inconsistent")
	}
	if !metadata.MembershipCounted {
		if metadata.MemberOccurrences != 0 || metadata.DistinctMembers != 0 ||
			len(metadata.RequestedMemberIDs) != 0 || len(metadata.CoveredMemberIDs) != 0 ||
			len(metadata.UncoveredMemberIDs) != 0 {
			return fmt.Errorf("componentmap: uncounted synthesis record carries membership coverage")
		}
		return nil
	}
	requested := make([]MemberID, 0)
	for _, candidate := range bundle.Candidates {
		if candidate.Role == CandidateRoleConceptualMember {
			requested = append(requested, candidate.ID)
		}
	}
	sortMemberIDs(requested)
	if !reflect.DeepEqual(metadata.RequestedMemberIDs, requested) {
		return fmt.Errorf("componentmap: synthesis record requested member identities do not match")
	}
	if metadata.DistinctMembers != len(metadata.CoveredMemberIDs) {
		return fmt.Errorf("componentmap: synthesis record covered member identities do not match counts")
	}
	covered := make(map[MemberID]struct{}, len(metadata.CoveredMemberIDs))
	for index, memberID := range metadata.CoveredMemberIDs {
		if index > 0 && metadata.CoveredMemberIDs[index-1].key() >= memberID.key() {
			return fmt.Errorf("componentmap: synthesis record covered member identities are not strictly sorted")
		}
		covered[memberID] = struct{}{}
	}
	uncovered := make(map[MemberID]struct{}, len(metadata.UncoveredMemberIDs))
	for index, memberID := range metadata.UncoveredMemberIDs {
		if index > 0 && metadata.UncoveredMemberIDs[index-1].key() >= memberID.key() {
			return fmt.Errorf("componentmap: synthesis record uncovered member identities are not strictly sorted")
		}
		uncovered[memberID] = struct{}{}
	}
	if len(covered)+len(uncovered) != len(requested) {
		return fmt.Errorf("componentmap: synthesis record membership partition cardinality does not match")
	}
	for _, memberID := range requested {
		_, isCovered := covered[memberID]
		_, isUncovered := uncovered[memberID]
		if isCovered == isUncovered {
			return fmt.Errorf("componentmap: synthesis record membership partition is not exact")
		}
	}
	return nil
}

func validateSynthesisProviderIdentity(identity SynthesisProviderIdentity) error {
	if !modelresearch.IsSHA256(identity.RequestSHA256) {
		return fmt.Errorf("componentmap: synthesis provider request digest is missing or malformed")
	}
	if !modelresearch.IsSHA256(identity.EndpointSHA256) {
		return fmt.Errorf("componentmap: synthesis provider endpoint digest is missing or malformed")
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
	wireProposal, err := decodeSynthesisWireProposalJSON(object)
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
	unitCatalog, unitErr := CompileUnitCatalog(bundle)
	if unitErr != nil {
		return Landscape{}, SynthesisMembershipCounts{}, unitErr
	}
	proposal, resolveErr := resolveSynthesisWireProposal(catalog, unitCatalog, wireProposal)
	if resolveErr != nil {
		landscape, err := synthesisResponseFallback(bundle, newDiagnostic(resolveErr.code, resolveErr.message))
		return landscape, SynthesisMembershipCounts{}, err
	}
	membership := synthesisMembershipCounts(bundle, proposal)
	landscape, err := Apply(bundle, proposal)
	if err != nil {
		return Landscape{}, SynthesisMembershipCounts{}, err
	}
	if landscape.ValidationOutcome != ValidationRejected {
		// Accepted status describes the canonical model-authored relation after
		// local readable-shape normalization, not a raw response cardinality that
		// normalization may have merged.
		membership = acceptedSynthesisMembershipCounts(bundle, landscape)
	}
	warnings := make([]Diagnostic, 0, 1)
	if normalization != nil {
		warnings = append(warnings, *normalization)
	}
	if len(warnings) > 0 {
		landscape.Diagnostics = append(warnings, landscape.Diagnostics...)
		if err := landscape.Validate(bundle); err != nil {
			return Landscape{}, SynthesisMembershipCounts{}, err
		}
	}
	return landscape, membership, nil
}

func synthesisMembershipCounts(bundle CandidateBundle, proposal Proposal) SynthesisMembershipCounts {
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
	return synthesisMembershipCountsFromSets(bundle, len(proposal.Subsystems) > 0, occurrences, distinct)
}

func acceptedSynthesisMembershipCounts(bundle CandidateBundle, landscape Landscape) SynthesisMembershipCounts {
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
	return synthesisMembershipCountsFromSets(bundle, true, occurrences, distinct)
}

func synthesisMembershipCountsFromSets(
	bundle CandidateBundle,
	counted bool,
	occurrences int,
	covered map[MemberID]struct{},
) SynthesisMembershipCounts {
	requestedIDs := make([]MemberID, 0)
	for _, candidate := range bundle.Candidates {
		if candidate.Role == CandidateRoleConceptualMember {
			requestedIDs = append(requestedIDs, candidate.ID)
		}
	}
	sortMemberIDs(requestedIDs)
	var coveredIDs []MemberID
	var uncoveredIDs []MemberID
	for _, memberID := range requestedIDs {
		if _, exists := covered[memberID]; exists {
			coveredIDs = append(coveredIDs, memberID)
		} else {
			uncoveredIDs = append(uncoveredIDs, memberID)
		}
	}
	return SynthesisMembershipCounts{
		Counted:            counted,
		MemberOccurrences:  occurrences,
		DistinctMembers:    len(coveredIDs),
		RequestedMemberIDs: requestedIDs,
		CoveredMemberIDs:   coveredIDs,
		UncoveredMemberIDs: uncoveredIDs,
	}
}

func synthesisResponseFallback(bundle CandidateBundle, warning Diagnostic) (Landscape, error) {
	landscape := buildDeterministicLocalLandscape(bundle, SourcePackageFallback)
	landscape.Diagnostics = []Diagnostic{warning}
	landscape.ValidationOutcome = ValidationRejected
	landscape.FallbackReason = fallbackReasonForDiagnostics(landscape.Diagnostics, len(bundle.BehaviorAnchors) > 0)
	landscape.Fallback = true
	landscape.OriginalProposalSHA256 = proposalSHA256(Proposal{})
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
	if !json.Valid(trimmed) {
		if len(jsonObjectCandidates(trimmed, 2)) > 1 {
			return nil, nil, &synthesisResponseError{code: "response.ambiguous_json", message: "provider response contains several json objects"}
		}
		if !bytes.Contains(trimmed, []byte("{")) {
			return nil, nil, &synthesisResponseError{code: "response.no_json", message: "provider response contains no json object"}
		}
		return nil, nil, &synthesisResponseError{code: "response.invalid_proposal", message: "provider response is not exactly one complete json object"}
	}
	if trimmed[0] != '{' {
		return nil, nil, &synthesisResponseError{code: "response.invalid_proposal", message: "provider response is json but not a proposal object"}
	}
	return append([]byte(nil), trimmed...), nil, nil
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

type synthesisWireRecordKind string

const (
	synthesisWireSubsystemRecord synthesisWireRecordKind = "subsystem"
	synthesisWireComponentRecord synthesisWireRecordKind = "component"
)

type synthesisWireProposal struct {
	Records []synthesisWireRecord `json:"records"`
}

// synthesisWireRecord is an in-memory tagged union. MarshalJSON emits only the
// fields owned by its exact kind so tests exercise the same one-grammar wire
// accepted from providers.
type synthesisWireRecord struct {
	Kind         synthesisWireRecordKind
	Ref          string
	SubsystemRef string
	Name         string
	Description  string
	MemberRefs   []SynthesisMemberRef
	UnitRefs     []SynthesisUnitRef
	AnchorRefs   []SynthesisAnchorRef
	Hypothesis   bool
}

// SynthesisUnitRef is a request-local unit reference (Decision 216).
type SynthesisUnitRef struct {
	Kind MemberKind `json:"kind"`
	Ref  string     `json:"ref"`
}

func (record synthesisWireRecord) MarshalJSON() ([]byte, error) {
	switch record.Kind {
	case synthesisWireSubsystemRecord:
		return json.Marshal(struct {
			Kind        synthesisWireRecordKind `json:"kind"`
			Ref         string                  `json:"ref"`
			Name        string                  `json:"name"`
			Description string                  `json:"description"`
		}{record.Kind, record.Ref, record.Name, record.Description})
	case synthesisWireComponentRecord:
		// member_refs XOR unit_refs stays exact on the wire: the used field
		// serializes even when empty ([]), the unused field is omitted.
		memberRefs := record.MemberRefs
		if memberRefs == nil {
			memberRefs = []SynthesisMemberRef{}
		}
		var unitRefs []SynthesisUnitRef
		if len(record.UnitRefs) > 0 {
			unitRefs = record.UnitRefs
		}
		anchorRefs := record.AnchorRefs
		if anchorRefs == nil {
			anchorRefs = []SynthesisAnchorRef{}
		}
		return json.Marshal(struct {
			Kind         synthesisWireRecordKind `json:"kind"`
			SubsystemRef string                  `json:"subsystem_ref"`
			Name         string                  `json:"name"`
			Description  string                  `json:"description"`
			MemberRefs   []SynthesisMemberRef    `json:"member_refs,omitempty"`
			UnitRefs     []SynthesisUnitRef      `json:"unit_refs,omitempty"`
			AnchorRefs   []SynthesisAnchorRef    `json:"anchor_refs"`
			Hypothesis   bool                    `json:"hypothesis"`
		}{record.Kind, record.SubsystemRef, record.Name, record.Description, memberRefs, unitRefs, anchorRefs, record.Hypothesis})
	default:
		return nil, fmt.Errorf("componentmap: unsupported synthesis wire record kind")
	}
}

// decodeSynthesisWireProposalJSON rejects every field outside the exact
// tagged-record contract. Canonical IDs have no field in the active response
// shape and are never copied from provider bytes.
func decodeSynthesisWireProposalJSON(raw []byte) (synthesisWireProposal, error) {
	if !utf8.Valid(raw) {
		return synthesisWireProposal{}, fmt.Errorf("proposal is not valid utf-8")
	}
	if err := rejectDuplicateSynthesisJSONKeys(raw); err != nil {
		return synthesisWireProposal{}, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return synthesisWireProposal{}, fmt.Errorf("proposal is not an object")
	}
	if !hasExactSynthesisFields(root, "records") {
		return synthesisWireProposal{}, fmt.Errorf("proposal root fields do not match the records contract")
	}
	var rawRecords []json.RawMessage
	if err := json.Unmarshal(root["records"], &rawRecords); err != nil {
		return synthesisWireProposal{}, fmt.Errorf("proposal records have invalid type")
	}
	if len(rawRecords) == 0 || len(rawRecords) > maxSynthesisWireRecords {
		return synthesisWireProposal{}, fmt.Errorf("proposal record count is outside the bounded contract")
	}
	proposal := synthesisWireProposal{Records: make([]synthesisWireRecord, 0, len(rawRecords))}
	for _, rawRecord := range rawRecords {
		record, err := decodeSynthesisWireRecord(rawRecord)
		if err != nil {
			return synthesisWireProposal{}, err
		}
		proposal.Records = append(proposal.Records, record)
	}
	return proposal, nil
}

// SynthesisResponseMembershipCounts returns raw conceptual-membership
// cardinality only when raw uses the exact current flat response grammar. It
// deliberately performs no catalog resolution, normalization, or repair, so a
// rejected current response can retain diagnostic counts without accepting
// retired or malformed provider bytes.
func SynthesisResponseMembershipCounts(raw []byte) (bool, int, int) {
	proposal, err := decodeSynthesisWireProposalJSON(raw)
	if err != nil {
		return false, 0, 0
	}
	seenComponent := false
	occurrences := 0
	distinct := make(map[string]struct{})
	for _, record := range proposal.Records {
		if record.Kind != synthesisWireComponentRecord {
			continue
		}
		seenComponent = true
		for _, memberRef := range record.MemberRefs {
			occurrences++
			distinct[memberRef.key()] = struct{}{}
		}
		for _, unitRef := range record.UnitRefs {
			occurrences++
			distinct["unit\x00"+unitRef.Ref] = struct{}{}
		}
	}
	if !seenComponent {
		return false, 0, 0
	}
	return true, occurrences, len(distinct)
}

func decodeSynthesisWireRecord(raw json.RawMessage) (synthesisWireRecord, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return synthesisWireRecord{}, fmt.Errorf("proposal record is not an object")
	}
	kindValue, err := decodeRequiredProposalString(fields, "kind")
	if err != nil {
		return synthesisWireRecord{}, err
	}
	kind := synthesisWireRecordKind(kindValue)
	switch kind {
	case synthesisWireSubsystemRecord:
		if !hasExactSynthesisFields(fields, "kind", "ref", "name", "description") {
			return synthesisWireRecord{}, fmt.Errorf("proposal subsystem record fields do not match the bounded contract")
		}
		ref, err := decodeRequiredProposalString(fields, "ref")
		if err != nil {
			return synthesisWireRecord{}, err
		}
		name, err := decodeRequiredProposalString(fields, "name")
		if err != nil {
			return synthesisWireRecord{}, err
		}
		description, err := decodeRequiredProposalDescription(fields, "description")
		if err != nil {
			return synthesisWireRecord{}, err
		}
		return synthesisWireRecord{
			Kind: kind, Ref: ref, Name: name, Description: description,
		}, nil
	case synthesisWireComponentRecord:
		// Decision 216: a component groups either raw member refs (legacy
		// flat contract) or request-local unit refs (u*, bounded unit
		// contract) — never both. The exact field set therefore has two
		// legal shapes.
		if !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "member_refs", "anchor_refs", "hypothesis",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "unit_refs", "anchor_refs", "hypothesis",
		) {
			return synthesisWireRecord{}, fmt.Errorf("proposal component record fields do not match the bounded contract")
		}
		subsystemRef, err := decodeRequiredProposalString(fields, "subsystem_ref")
		if err != nil {
			return synthesisWireRecord{}, err
		}
		name, err := decodeRequiredProposalString(fields, "name")
		if err != nil {
			return synthesisWireRecord{}, err
		}
		description, err := decodeRequiredProposalDescription(fields, "description")
		if err != nil {
			return synthesisWireRecord{}, err
		}
		if isJSONNull(fields["anchor_refs"]) || isJSONNull(fields["hypothesis"]) {
			return synthesisWireRecord{}, fmt.Errorf("proposal component fields must not be null")
		}
		_, memberRefsFieldExists := fields["member_refs"]
		_, unitRefsFieldExists := fields["unit_refs"]
		hasMemberRefs := memberRefsFieldExists && !isJSONNull(fields["member_refs"])
		hasUnitRefs := unitRefsFieldExists && !isJSONNull(fields["unit_refs"])
		if hasMemberRefs == hasUnitRefs {
			return synthesisWireRecord{}, fmt.Errorf("proposal component must group either member_refs or unit_refs, not both and not neither")
		}
		memberRefs := []SynthesisMemberRef{}
		unitRefs := []SynthesisUnitRef{}
		if hasMemberRefs {
			var rawMemberRefs []json.RawMessage
			if err := json.Unmarshal(fields["member_refs"], &rawMemberRefs); err != nil {
				return synthesisWireRecord{}, fmt.Errorf("proposal member refs have invalid type")
			}
			memberRefs = make([]SynthesisMemberRef, 0, len(rawMemberRefs))
			for _, rawMemberRef := range rawMemberRefs {
				memberRef, err := decodeSynthesisMemberRef(rawMemberRef)
				if err != nil {
					return synthesisWireRecord{}, err
				}
				memberRefs = append(memberRefs, memberRef)
			}
		} else {
			var rawUnitRefs []json.RawMessage
			if err := json.Unmarshal(fields["unit_refs"], &rawUnitRefs); err != nil {
				return synthesisWireRecord{}, fmt.Errorf("proposal unit refs have invalid type")
			}
			unitRefs = make([]SynthesisUnitRef, 0, len(rawUnitRefs))
			for _, rawUnitRef := range rawUnitRefs {
				unitRef, err := decodeSynthesisUnitRef(rawUnitRef)
				if err != nil {
					return synthesisWireRecord{}, err
				}
				unitRefs = append(unitRefs, unitRef)
			}
		}
		var rawAnchorRefs []json.RawMessage
		if err := json.Unmarshal(fields["anchor_refs"], &rawAnchorRefs); err != nil {
			return synthesisWireRecord{}, fmt.Errorf("proposal anchor refs have invalid type")
		}
		anchorRefs := make([]SynthesisAnchorRef, 0, len(rawAnchorRefs))
		for _, rawAnchorRef := range rawAnchorRefs {
			anchorRef, err := decodeSynthesisAnchorRef(rawAnchorRef)
			if err != nil {
				return synthesisWireRecord{}, err
			}
			anchorRefs = append(anchorRefs, anchorRef)
		}
		var hypothesis bool
		if err := json.Unmarshal(fields["hypothesis"], &hypothesis); err != nil {
			return synthesisWireRecord{}, fmt.Errorf("proposal hypothesis has invalid type")
		}
		return synthesisWireRecord{
			Kind: kind, SubsystemRef: subsystemRef, Name: name, Description: description,
			MemberRefs: memberRefs, UnitRefs: unitRefs, AnchorRefs: anchorRefs, Hypothesis: hypothesis,
		}, nil
	default:
		return synthesisWireRecord{}, fmt.Errorf("proposal record kind is invalid")
	}
}

func decodeSynthesisMemberRef(raw json.RawMessage) (SynthesisMemberRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisMemberRef{}, fmt.Errorf("proposal member ref is not an object")
	}
	if !hasExactSynthesisFields(fields, "kind", "ref") {
		return SynthesisMemberRef{}, fmt.Errorf("proposal member ref fields do not match the bounded contract")
	}
	kind, err := decodeRequiredProposalString(fields, "kind")
	if err != nil {
		return SynthesisMemberRef{}, err
	}
	ref, err := decodeRequiredProposalString(fields, "ref")
	if err != nil {
		return SynthesisMemberRef{}, err
	}
	return SynthesisMemberRef{Kind: MemberKind(kind), Ref: ref}, nil
}

func decodeSynthesisUnitRef(raw json.RawMessage) (SynthesisUnitRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisUnitRef{}, fmt.Errorf("proposal unit ref is not an object")
	}
	if !hasExactSynthesisFields(fields, "kind", "ref") {
		return SynthesisUnitRef{}, fmt.Errorf("proposal unit ref fields do not match the bounded contract")
	}
	kind, err := decodeRequiredProposalString(fields, "kind")
	if err != nil {
		return SynthesisUnitRef{}, err
	}
	ref, err := decodeRequiredProposalString(fields, "ref")
	if err != nil {
		return SynthesisUnitRef{}, err
	}
	return SynthesisUnitRef{Kind: MemberKind(kind), Ref: ref}, nil
}

func decodeSynthesisAnchorRef(raw json.RawMessage) (SynthesisAnchorRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisAnchorRef{}, fmt.Errorf("proposal anchor ref is not an object")
	}
	if !hasExactSynthesisFields(fields, "kind", "ref") {
		return SynthesisAnchorRef{}, fmt.Errorf("proposal anchor ref fields do not match the bounded contract")
	}
	kind, err := decodeRequiredProposalString(fields, "kind")
	if err != nil {
		return SynthesisAnchorRef{}, err
	}
	ref, err := decodeRequiredProposalString(fields, "ref")
	if err != nil {
		return SynthesisAnchorRef{}, err
	}
	return SynthesisAnchorRef{Kind: BehaviorAnchorKind(kind), Ref: ref}, nil
}

func resolveSynthesisWireProposal(
	catalog synthesisPrivateCatalog,
	unitCatalog UnitCatalog,
	wire synthesisWireProposal,
) (Proposal, *synthesisResponseError) {
	proposal := Proposal{Version: ProposalVersion}
	subsystemIndexes := make(map[string]int)
	componentCounts := make(map[string]int)
	totalComponents := 0
	for _, record := range wire.Records {
		if err := validateSynthesisResponseDisplay(record); err != nil {
			return Proposal{}, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal display text is outside the bounded contract",
			}
		}
		switch record.Kind {
		case synthesisWireSubsystemRecord:
			if !validSynthesisSubsystemRef(record.Ref) {
				return Proposal{}, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal subsystem ref is malformed",
				}
			}
			if _, duplicate := subsystemIndexes[record.Ref]; duplicate {
				return Proposal{}, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal repeats a response-local subsystem ref",
				}
			}
			subsystemIndexes[record.Ref] = len(proposal.Subsystems)
			proposal.Subsystems = append(proposal.Subsystems, ProposedSubsystem{
				Name: record.Name, Description: record.Description,
			})
		case synthesisWireComponentRecord:
			if !validSynthesisSubsystemRef(record.SubsystemRef) {
				return Proposal{}, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal component subsystem ref is malformed",
				}
			}
			if len(record.AnchorRefs) > maxAnchorMembers {
				return Proposal{}, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal component anchor count exceeds the bounded contract",
				}
			}
			componentCounts[record.SubsystemRef]++
			totalComponents++
		}
	}
	if len(proposal.Subsystems) == 0 || len(proposal.Subsystems) > MaxPrimarySubsystems {
		return Proposal{}, &synthesisResponseError{
			code: "response.invalid_proposal", message: "proposal subsystem count is outside the bounded contract",
		}
	}
	if totalComponents == 0 || totalComponents > MaxTotalNestedComponents {
		return Proposal{}, &synthesisResponseError{
			code: "response.invalid_proposal", message: "proposal component count exceeds the bounded contract",
		}
	}
	for subsystemRef, componentCount := range componentCounts {
		if _, exists := subsystemIndexes[subsystemRef]; !exists {
			return Proposal{}, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component references an unknown response-local subsystem ref",
			}
		}
		if componentCount > MaxComponentsPerSubsystem {
			return Proposal{}, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component count exceeds the bounded contract",
			}
		}
	}
	for _, record := range wire.Records {
		if record.Kind != synthesisWireComponentRecord {
			continue
		}
		if !validSynthesisSubsystemRef(record.SubsystemRef) {
			return Proposal{}, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component subsystem ref is malformed",
			}
		}
		subsystemIndex, exists := subsystemIndexes[record.SubsystemRef]
		if !exists {
			return Proposal{}, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component references an unknown response-local subsystem ref",
			}
		}
		component := ProposedComponent{
			Name: record.Name, Description: record.Description,
			Hypothesis: record.Hypothesis,
			MemberIDs:  make([]MemberID, 0, len(record.MemberRefs)+len(record.UnitRefs)),
			AnchorIDs:  make([]string, 0, len(record.AnchorRefs)),
		}
		for _, memberRef := range record.MemberRefs {
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
			if catalog.memberRoles[memberRef.key()] != CandidateRoleConceptualMember {
				return Proposal{}, &synthesisResponseError{
					code: "proposal.unknown_member_id", message: "proposal returned a structural locator as conceptual membership",
				}
			}
			component.MemberIDs = append(component.MemberIDs, memberID)
		}
		// Decision 216: a component grouping unit refs (u*) expands locally
		// to the exact unit members. Unknown, duplicate, or wrong-kind unit
		// refs fail closed — never repaired, never guessed.
		if len(record.UnitRefs) > 0 {
			unitMembersByRef := unitCatalogUnitMembersByWireRef(unitCatalog)
			seenUnits := make(map[string]struct{}, len(record.UnitRefs))
			for _, unitRef := range record.UnitRefs {
				if unitRef.Kind != MemberPackage {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.unknown_unit_ref", message: "proposal unit ref has the wrong request-local kind",
					}
				}
				members, exists := unitMembersByRef[unitRef.Ref]
				if !exists {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.unknown_unit_ref", message: "proposal references an unknown request-local unit ref",
					}
				}
				if _, duplicate := seenUnits[unitRef.Ref]; duplicate {
					return Proposal{}, &synthesisResponseError{
						code: "proposal.duplicate_unit_ref", message: "proposal repeats a unit ref within one component",
					}
				}
				seenUnits[unitRef.Ref] = struct{}{}
				component.MemberIDs = append(component.MemberIDs, members...)
			}
		}
		seenAnchors := make(map[string]struct{}, len(record.AnchorRefs))
		for _, anchorRef := range record.AnchorRefs {
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
		proposal.Subsystems[subsystemIndex].Components = append(
			proposal.Subsystems[subsystemIndex].Components, component,
		)
	}
	for _, subsystem := range proposal.Subsystems {
		if len(subsystem.Components) == 0 {
			return Proposal{}, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal subsystem has no component records",
			}
		}
	}
	return proposal, nil
}

func decodeRequiredProposalString(fields map[string]json.RawMessage, name string) (string, error) {
	return decodeRequiredProposalText(fields, name, true)
}

func decodeRequiredProposalDescription(fields map[string]json.RawMessage, name string) (string, error) {
	return decodeRequiredProposalText(fields, name, false)
}

func decodeRequiredProposalText(fields map[string]json.RawMessage, name string, nonEmpty bool) (string, error) {
	value, exists := fields[name]
	if !exists || isJSONNull(value) {
		return "", fmt.Errorf("proposal is missing a required string field")
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", fmt.Errorf("proposal field has invalid string type")
	}
	if strings.ContainsRune(result, utf8.RuneError) {
		return "", fmt.Errorf("proposal field contains invalid unicode")
	}
	if nonEmpty && result == "" {
		return "", fmt.Errorf("proposal has an empty required string field")
	}
	return result, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func rejectDuplicateSynthesisJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := consumeUniqueSynthesisJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("proposal contains trailing json")
		}
		return fmt.Errorf("proposal contains invalid trailing json: %w", err)
	}
	return nil
}

func consumeUniqueSynthesisJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("proposal contains invalid json: %w", err)
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("proposal contains invalid object key: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("proposal contains a non-string object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("proposal repeats json field %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeUniqueSynthesisJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("proposal contains an unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueSynthesisJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("proposal contains an unterminated array")
		}
	default:
		return fmt.Errorf("proposal contains an unexpected json delimiter")
	}
	return nil
}

func validateSynthesisResponseDisplay(record synthesisWireRecord) error {
	if strings.TrimSpace(record.Name) != record.Name || strings.TrimSpace(record.Description) != record.Description {
		return fmt.Errorf("proposal display text has surrounding whitespace")
	}
	if err := validateDisplayText("proposal record name", record.Name, maxNameBytes, true); err != nil {
		return err
	}
	return validateDisplayText("proposal record description", record.Description, maxDescriptionBytes, false)
}

func hasExactSynthesisFields(fields map[string]json.RawMessage, expected ...string) bool {
	if len(fields) != len(expected) {
		return false
	}
	for _, field := range expected {
		if _, exists := fields[field]; !exists {
			return false
		}
	}
	return true
}

func validSynthesisSubsystemRef(ref string) bool {
	if len(ref) < 2 || len(ref) > maxOpaqueIDBytes || ref[0] != 'g' || ref[1] == '0' {
		return false
	}
	for _, char := range ref[1:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
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
	if _, sensitive := secretscan.DetectAlways(string(encoded)); sensitive {
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
			_, sensitive := secretscan.DetectAlways(typed)
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
	if _, sensitive := secretscan.DetectAlways(string(response)); sensitive {
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
