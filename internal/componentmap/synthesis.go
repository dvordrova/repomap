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
	// Decision 239: production-aware unit roles change the provider-visible
	// primary/supporting classification and the meaning of saved primary-scope
	// coverage. The prompt identity is derived automatically from its exact
	// text below. Request v19 also binds collision-safe private unit grouping:
	// command namespaces no longer merge with same-named top-level packages.
	SynthesisRequestVersion = 19
	SynthesisRecordVersion  = 15
)

// SynthesisPromptVersion is the prompt contract identity — the short SHA-256
// of the exact language-independent system text (owner directive 2026-08-07:
// short prompt SHA instead of a hand-bumped version). Any edit to the prompt
// instructions automatically changes this value, so cache keys and
// saved-record replays fail closed on their own.
var SynthesisPromptVersion = "architecture-grounding-" + shortSynthesisPromptSystemSHA()

const (
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
	// Decision 231 (Archive 9, gap closure): kind is backend-owned and may
	// be omitted by the model ({"ref":"p11"}); a supplied kind is still
	// validated. omitempty keeps a kind-less ref off the wire entirely.
	Kind MemberKind `json:"kind,omitempty"`
	Ref  string     `json:"ref"`
}

// SynthesisAnchorRef is one short request-local typed grounding identity.
// Canonical anchor IDs stay in the private catalog.
type SynthesisAnchorRef struct {
	// Decision 231 (Archive 9): anchor kind is backend-owned and may be
	// omitted by the model ({"ref":"a1"}); a supplied kind is still
	// validated. omitempty keeps a kind-less ref off the wire entirely.
	Kind BehaviorAnchorKind `json:"kind,omitempty"`
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

// SynthesisCoverageRole is backend-owned request-local quality context. It
// distinguishes the repository scope a conceptual Architecture should cover
// from bounded evidence that may ground or distinguish that scope.
type SynthesisCoverageRole string

const (
	SynthesisCoveragePrimaryScope       SynthesisCoverageRole = "primary_scope"
	SynthesisCoverageSupportingEvidence SynthesisCoverageRole = "supporting_evidence"
)

func (role SynthesisCoverageRole) valid() bool {
	return role == SynthesisCoveragePrimaryScope || role == SynthesisCoverageSupportingEvidence
}

// SynthesisCandidate is the provider-visible candidate shape. It exposes one
// short typed ref plus bounded semantic labels, never canonical IDs, exact
// paths, locations, providers or scenarios.
type SynthesisCandidate struct {
	Ref            SynthesisMemberRef           `json:"ref"`
	ParentRef      *SynthesisMemberRef          `json:"parent_ref,omitempty"`
	UnitRef        UnitWireRef                  `json:"unit_ref"`
	CoverageRole   SynthesisCoverageRole        `json:"coverage_role"`
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
	RepositoryArchetype RepositoryArchetype `json:"repository_archetype"`
	GroundingMode       GroundingMode       `json:"grounding_mode"`
	// Phase 1 prompt cleanup: required_member_refs is the exact same
	// candidate checklist as candidates[].ref (validateSynthesisRequestCoverage
	// proves the identity), so it is removed from the provider-visible wire.
	// It stays local for coverage accounting via the candidates array.
	RequiredMemberRefs []SynthesisMemberRef      `json:"-"`
	BehaviorAnchors    []SynthesisBehaviorAnchor `json:"behavior_anchors,omitempty"`
	Flows              []SynthesisFlow           `json:"flows,omitempty"`
	Candidates         []SynthesisCandidate      `json:"candidates,omitempty"`
	// Units is the bounded read-only request-local context used to understand
	// candidate unit association. Live responses still return member_refs;
	// historical unit_refs remain accepted only by the replay decoder.
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
	ResponseEmpty            ResponseState = "empty"
	ResponseOversize         ResponseState = "oversize_omitted"
	ResponseSensitiveOmitted ResponseState = "sensitive_omitted"
)

// SynthesisMetadata is saved beside the singular provider call. Validation
// warnings and fallback are outcomes of local Apply, never provider claims.
type SynthesisMetadata struct {
	PromptVersion             string                   `json:"prompt_version"`
	Profile                   string                   `json:"profile"`
	Model                     string                   `json:"model"`
	OutputLanguage            string                   `json:"output_language"`
	InputBytes                int                      `json:"input_bytes"`
	LatencyMillis             int64                    `json:"latency_ms"`
	UsageReported             bool                     `json:"usage_reported"`
	InputTokens               int                      `json:"input_tokens,omitempty"`
	OutputTokens              int                      `json:"output_tokens,omitempty"`
	FinishReason              string                   `json:"finish_reason,omitempty"`
	TransportAttempts         int                      `json:"transport_attempts"`
	ResponseComplete          bool                     `json:"response_complete"`
	MembershipCounted         bool                     `json:"response_membership_counted"`
	MemberOccurrences         int                      `json:"response_member_occurrences,omitempty"`
	DistinctMembers           int                      `json:"response_distinct_members,omitempty"`
	RequestedMemberIDs        []MemberID               `json:"requested_member_ids,omitempty"`
	CoveredMemberIDs          []MemberID               `json:"covered_member_ids,omitempty"`
	UncoveredMemberIDs        []MemberID               `json:"uncovered_member_ids,omitempty"`
	RequestedPrimaryScope     int                      `json:"requested_primary_scope,omitempty"`
	CoveredPrimaryScope       int                      `json:"covered_primary_scope,omitempty"`
	UncoveredPrimaryScope     int                      `json:"uncovered_primary_scope,omitempty"`
	CoveredSupportingEvidence int                      `json:"covered_supporting_evidence,omitempty"`
	ValidationWarnings        []Diagnostic             `json:"validation_warnings,omitempty"`
	ValidationOutcome         ValidationOutcome        `json:"validation_outcome"`
	ArchitectureSource        ArchitectureSource       `json:"architecture_source"`
	ArchitectureLevel         int                      `json:"architecture_level"`
	Normalizations            []NormalizationOperation `json:"normalization_operations,omitempty"`
	OriginalProposalSHA256    string                   `json:"original_proposal_sha256,omitempty"`
	FallbackReason            FallbackReason           `json:"fallback_reason,omitempty"`
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
	Counted                   bool
	MemberOccurrences         int
	DistinctMembers           int
	RequestedMemberIDs        []MemberID
	CoveredMemberIDs          []MemberID
	UncoveredMemberIDs        []MemberID
	RequestedPrimaryScope     int
	CoveredPrimaryScope       int
	UncoveredPrimaryScope     int
	CoveredSupportingEvidence int
}

func (counts SynthesisMembershipCounts) CoverageComplete() bool {
	return counts.Counted && len(counts.RequestedMemberIDs) > 0 && len(counts.UncoveredMemberIDs) == 0
}

type synthesisPrivateCatalog struct {
	membersByID  map[MemberID]SynthesisMemberRef
	membersByRef map[string]MemberID
	memberKinds  map[string]MemberKind
	memberRoles  map[string]CandidateRole
	// Decision 231 (Archive 9, gap closure): ref-only member lookup for
	// responses that omit the backend-owned kind ({"ref":"p11"}). The
	// request-local member ref is unique by ref alone; kind is owned by
	// the backend catalog.
	membersByRefOnly     map[string]MemberID
	memberRolesByRefOnly map[string]CandidateRole
	anchorsByID          map[string]SynthesisAnchorRef
	anchorsByRef         map[string]string
	// Decision 231 (Archive 9): ref-only anchor lookup for responses that
	// omit the backend-owned kind ({"ref":"a1"}).
	anchorsByRefOnly map[string]string
	anchorKinds      map[string]BehaviorAnchorKind
	// Decision 230 D9.7: exact members owned by each behavior anchor
	// (anchor-specific slice for repeated broad units).
	anchorMemberIDs map[string]map[MemberID]struct{}
	// memberParentIDs maps a child member (symbol/file) to its exact
	// parent package member so an anchor-owned symbol can be resolved to
	// the package member a broad unit actually contains.
	memberParentIDs    map[MemberID]MemberID
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
		membersByID:          make(map[MemberID]SynthesisMemberRef, len(bundle.Candidates)),
		membersByRef:         make(map[string]MemberID, len(bundle.Candidates)),
		membersByRefOnly:     make(map[string]MemberID, len(bundle.Candidates)),
		memberKinds:          make(map[string]MemberKind, len(bundle.Candidates)),
		memberRoles:          make(map[string]CandidateRole, len(bundle.Candidates)),
		memberRolesByRefOnly: make(map[string]CandidateRole, len(bundle.Candidates)),
		anchorsByID:          make(map[string]SynthesisAnchorRef, len(bundle.BehaviorAnchors)),
		anchorsByRef:         make(map[string]string, len(bundle.BehaviorAnchors)),
		anchorsByRefOnly:     make(map[string]string, len(bundle.BehaviorAnchors)),
		anchorKinds:          make(map[string]BehaviorAnchorKind, len(bundle.BehaviorAnchors)),
		anchorMemberIDs:      make(map[string]map[MemberID]struct{}, len(bundle.BehaviorAnchors)),
		memberParentIDs:      make(map[MemberID]MemberID, len(bundle.Candidates)),
		flowsByID:            make(map[FlowID]SynthesisFlowRef, len(bundle.Flows)),
		canonicalOpaqueIDs:   make(map[string]struct{}, len(bundle.Candidates)+len(bundle.BehaviorAnchors)+len(bundle.Flows)),
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
		// Decision 231 (Archive 9, gap closure): the request-local member ref
		// is unique by ref alone — the ref-only key is the backend-owned-kind
		// resolution for models that omit kind.
		catalog.membersByRefOnly[ref.Ref] = candidate.ID
		catalog.memberRolesByRefOnly[ref.Ref] = candidate.Role
		catalog.memberKinds[ref.Ref] = ref.Kind
		catalog.memberRoles[ref.key()] = candidate.Role
		if candidate.ParentID != nil {
			catalog.memberParentIDs[candidate.ID] = *candidate.ParentID
		}
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
		catalog.anchorsByRefOnly[ref.Ref] = anchor.ID
		catalog.anchorKinds[ref.Ref] = ref.Kind
		if len(anchor.MemberIDs) > 0 {
			memberSet := make(map[MemberID]struct{}, len(anchor.MemberIDs))
			for _, memberID := range anchor.MemberIDs {
				memberSet[memberID] = struct{}{}
			}
			catalog.anchorMemberIDs[anchor.ID] = memberSet
		}
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

type synthesisCandidateContext struct {
	ParentID     *MemberID
	UnitRef      UnitWireRef
	UnitRole     UnitRole
	CoverageRole SynthesisCoverageRole
}

// synthesisCandidateContexts restores only bounded backend-owned request
// context. Parent resolution walks the already validated finite candidate
// graph; exact unit association comes from the final private unit membership,
// never from labels or provider prose.
func synthesisCandidateContexts(
	bundle CandidateBundle,
	unitCatalog UnitCatalog,
) (map[MemberID]synthesisCandidateContext, error) {
	if len(unitCatalog.Units) != len(unitCatalog.WireUnits) {
		return nil, fmt.Errorf("componentmap: architecture unit wire catalog is inconsistent")
	}
	known := candidateIndex(bundle)
	unitRoles := make(map[UnitWireRef]UnitRole, len(unitCatalog.WireUnits))
	for index, unit := range unitCatalog.WireUnits {
		wireRef := unit.Ref
		if wireRef == "" {
			return nil, fmt.Errorf("componentmap: architecture unit wire ref is empty")
		}
		if _, duplicate := unitRoles[wireRef]; duplicate {
			return nil, fmt.Errorf("componentmap: architecture unit wire ref is duplicated")
		}
		if unit.Role != unitCatalog.Units[index].Role {
			return nil, fmt.Errorf("componentmap: architecture unit wire role is inconsistent")
		}
		if _, err := synthesisTopLevelCoverageRole(unit.Role); err != nil {
			return nil, err
		}
		unitRoles[wireRef] = unit.Role
	}

	contexts := make(map[MemberID]synthesisCandidateContext)
	for _, candidate := range bundle.Candidates {
		if candidate.Role != CandidateRoleConceptualMember {
			continue
		}
		unitRef, exists := unitCatalog.MemberToWireUnit[candidate.ID]
		if !exists {
			return nil, fmt.Errorf("componentmap: conceptual member has no architecture unit")
		}
		unitRole, advertised := unitRoles[unitRef]
		if !advertised {
			return nil, fmt.Errorf("componentmap: conceptual member has an unadvertised architecture unit")
		}
		coverageRole, err := synthesisTopLevelCoverageRole(unitRole)
		if err != nil {
			return nil, err
		}
		context := synthesisCandidateContext{
			UnitRef:      unitRef,
			UnitRole:     unitRole,
			CoverageRole: coverageRole,
		}
		if ownerID, owned := nearestConceptualPackageOwner(candidate.ID, known); owned && ownerID != candidate.ID {
			context.ParentID = &ownerID
			context.CoverageRole = SynthesisCoverageSupportingEvidence
		}
		contexts[candidate.ID] = context
	}
	return contexts, nil
}

// synthesisTopLevelCoverageRole is the single closed conversion from local
// unit purpose to provider-visible coverage priority. Package-owned children
// are downgraded to supporting evidence by synthesisCandidateContexts after
// this top-level classification is established.
func synthesisTopLevelCoverageRole(role UnitRole) (SynthesisCoverageRole, error) {
	switch role {
	case UnitRoleProduction:
		return SynthesisCoveragePrimaryScope, nil
	case UnitRoleTest, UnitRoleTooling, UnitRoleDocumentation:
		return SynthesisCoverageSupportingEvidence, nil
	default:
		return "", fmt.Errorf("componentmap: architecture unit has invalid role %q", role)
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
	contexts, err := synthesisCandidateContexts(bundle, unitCatalog)
	if err != nil {
		return SynthesisRequest{}, nil, err
	}
	for index := range request.Candidates {
		candidate := &request.Candidates[index]
		memberID, exists := catalog.membersByRef[candidate.Ref.key()]
		if !exists {
			return SynthesisRequest{}, nil, fmt.Errorf("componentmap: candidate ref is absent from the private catalog")
		}
		context, exists := contexts[memberID]
		if !exists {
			return SynthesisRequest{}, nil, fmt.Errorf("componentmap: candidate request context is missing")
		}
		candidate.UnitRef = context.UnitRef
		candidate.CoverageRole = context.CoverageRole
		if context.ParentID != nil {
			parentRef := catalog.membersByID[*context.ParentID]
			candidate.ParentRef = &parentRef
		}
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
	knownUnits := make(map[UnitWireRef]UnitRole, len(request.Units))
	for index, unit := range request.Units {
		if unit.Ref == "" {
			return fmt.Errorf("componentmap: units[%d].ref is empty", index)
		}
		if _, duplicate := knownUnits[unit.Ref]; duplicate {
			return fmt.Errorf("componentmap: units[%d].ref duplicates an earlier unit", index)
		}
		if _, err := synthesisTopLevelCoverageRole(unit.Role); err != nil {
			return fmt.Errorf("componentmap: units[%d].role is invalid", index)
		}
		knownUnits[unit.Ref] = unit.Role
	}
	seen := make(map[string]struct{}, len(request.RequiredMemberRefs))
	candidatesByRef := make(map[string]SynthesisCandidate, len(request.Candidates))
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
		candidate := request.Candidates[index]
		if !candidate.CoverageRole.valid() {
			return fmt.Errorf("componentmap: candidates[%d].coverage_role is invalid", index)
		}
		unitRole, known := knownUnits[candidate.UnitRef]
		if !known {
			return fmt.Errorf("componentmap: candidates[%d].unit_ref is not an advertised unit", index)
		}
		if candidate.ParentRef == nil {
			expectedRole, err := synthesisTopLevelCoverageRole(unitRole)
			if err != nil {
				return err
			}
			if candidate.CoverageRole != expectedRole {
				return fmt.Errorf("componentmap: candidates[%d] top-level coverage role does not match its unit role", index)
			}
			if candidate.CoverageRole == SynthesisCoverageSupportingEvidence && candidate.Ref.Kind != MemberPackage {
				return fmt.Errorf("componentmap: candidates[%d] supporting evidence has no package parent", index)
			}
		} else if candidate.CoverageRole != SynthesisCoverageSupportingEvidence {
			return fmt.Errorf("componentmap: candidates[%d] package-owned member is not supporting evidence", index)
		}
		candidatesByRef[key] = candidate
	}
	for index, candidate := range request.Candidates {
		if candidate.ParentRef == nil {
			continue
		}
		parent, known := candidatesByRef[candidate.ParentRef.key()]
		if !known || parent.Ref.Kind != MemberPackage {
			return fmt.Errorf("componentmap: candidates[%d].parent_ref is not a package candidate", index)
		}
		if candidate.Ref.Kind == MemberPackage {
			return fmt.Errorf("componentmap: candidates[%d] package candidate cannot have a package parent", index)
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
		fields = append(fields,
			synthesisWireIdentityField{
				name: fmt.Sprintf("candidates[%d].ref", index), ref: candidate.Ref.Ref,
			},
			synthesisWireIdentityField{
				name: fmt.Sprintf("candidates[%d].unit_ref", index), ref: string(candidate.UnitRef),
			},
		)
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

// synthesisPromptSystemText is the EXACT language-independent system text of
// the Architecture prompt. The prompt contract version is the short SHA-256
// of this text (owner directive 2026-08-07): editing any instruction
// automatically invalidates cache keys and saved-record replays — there is no
// version constant to bump by hand. The language suffix is appended by
// buildSynthesisPromptForLanguage and deliberately NOT part of the contract
// hash: en/ru share one contract.
func synthesisPromptSystemText() string {
	return `You create a compact conceptual architecture landscape from bounded local repository facts.

Use conceptual member, anchor, and unit refs as opaque request-local values. Copy a ref exactly as supplied; never rewrite refs, infer new refs, or mention members absent from the request. Refs under structural_context are read-only locator context and must never occur in response member_refs or anchor_refs. Candidate parent_ref, unit_ref, and coverage_role fields are read-only context and must never be returned.
Local semantic facts, compact structural relations, structural locator containment, flow participation, anchor proof_mode, and certainty are read-only grouping context. They must never be returned, upgraded, replaced, or converted into execution order. A declaration_family anchor is static declaration context and never proves runtime behavior. Canonical repository identities, exact source locations, provider provenance, scenarios, catalog identity, versions, and hashes are private and absent from this request.

Return exactly one compact JSON proposal object with this nested grammar:
{"subsystems":[{"name":"first subsystem","description":"first purpose","components":[{"name":"first component","description":"first responsibility","member_refs":["p1","s2"],"anchor_refs":["a1"]}]},{"name":"second subsystem","description":"second purpose","components":[{"name":"second component","description":"second responsibility","member_refs":["s1"],"anchor_refs":[]}]}]}

The entire response must parse as exactly one complete JSON object. Its only root field is subsystems. Each subsystem contains exactly name, description, and components. Each component contains exactly name, description, a non-empty member_refs array (p*/s*/f*), and optional anchor_refs (a*). Every ref is a plain string; do not wrap refs in objects and do not add kind fields. Do not emit response-local IDs, kind tags, parent references, unit refs, coverage roles, or any adjacency field: nesting already expresses which components belong to which subsystem. Do not nest objects inside objects or emit a second root object.

Candidates marked primary_scope form the top-level production conceptual repository surface. Top-level package candidates in test, tooling, or documentation units and package-owned child candidates are supporting_evidence. Cover defensible primary_scope across the supplied production units before selecting supporting_evidence. Supporting evidence and anchors may ground or distinguish responsibilities, but coverage of them never compensates for uncovered production primary_scope. Honest partial primary coverage is valid; never pad, invent, or exhaustively enumerate uncertain scope.

Subsystems and components are in conceptual display order. Choose representative supplied members needed to distinguish each component; exhaustive membership is not required. A member may legitimately participate in several components when it genuinely serves several distinct conceptual roles — this is shared participation, not exclusive ownership. Name what distinguishes each component from its siblings. An exact partial grouping is valid: omitted members remain in a deterministic local unclassified remainder and must not be echoed, renamed, or placed in a model-authored remainder. Fewer groups are better than padding: when evidence is weak, return fewer components honestly. A component may be anchor-backed shared participation with zero exclusive members: every one of its members is shared with sibling components, but the component still lists at least one member_refs (never only anchor_refs); anchor_refs are optional per component, not required.

Repository archetype and grounding mode are local facts. A primary pillar is one subsystem; components are nested responsibilities and are not additional primary pillars. When grounding_mode is behavior_grounded or mixed, prefer a bounded handful of distinct subsystems when the supplied evidence supports that many. Tiny, library, and package-landscape requests may honestly use fewer. hypothesis is not part of the response; the backend derives product hypothesis status exclusively from exact process_entry/call_target proof scoped to every component member. declaration_family never proves operational grounding. Separate extension families from support and tooling. Preserve unresolved frontiers. When grounding_mode is package_landscape, describe an honest static package landscape and do not imply behavioral verification.

Do not return versions, catalog identity, hashes, canonical IDs, edges, relations, flow definitions or transitions, fact payloads, repository paths, qualified symbols, test details, evidence, certainty, provenance, scenarios, source locations, coordinates, dimensions, ports, colors, styles, UI settings, markdown, or explanatory prose. Do not claim temporal or runtime behavior from static relations.`
}

// shortSynthesisPromptSystemSHA returns the first 12 hex chars of the SHA-256
// of the exact language-independent system text — the prompt contract
// identity (owner directive 2026-08-07: short prompt SHA instead of a
// hand-bumped version).
func shortSynthesisPromptSystemSHA() string {
	digest := sha256.Sum256([]byte(synthesisPromptSystemText()))
	return hex.EncodeToString(digest[:])[:12]
}

func buildSynthesisPromptForLanguage(
	bundle CandidateBundle,
	language string,
) (SynthesisPrompt, error) {
	_, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		return SynthesisPrompt{}, err
	}
	system := synthesisPromptSystemText()
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
				MembershipCounted:         membership.Counted,
				MemberOccurrences:         membership.MemberOccurrences,
				DistinctMembers:           membership.DistinctMembers,
				RequestedMemberIDs:        append([]MemberID(nil), membership.RequestedMemberIDs...),
				CoveredMemberIDs:          append([]MemberID(nil), membership.CoveredMemberIDs...),
				UncoveredMemberIDs:        append([]MemberID(nil), membership.UncoveredMemberIDs...),
				RequestedPrimaryScope:     membership.RequestedPrimaryScope,
				CoveredPrimaryScope:       membership.CoveredPrimaryScope,
				UncoveredPrimaryScope:     membership.UncoveredPrimaryScope,
				CoveredSupportingEvidence: membership.CoveredSupportingEvidence,
				ValidationWarnings:        cloneDiagnostics(landscape.Diagnostics),
				ValidationOutcome:         landscape.ValidationOutcome,
				ArchitectureSource:        landscape.Source,
				ArchitectureLevel:         landscape.Level,
				Normalizations:            append([]NormalizationOperation(nil), landscape.Normalizations...),
				OriginalProposalSHA256:    landscape.OriginalProposalSHA256,
				FallbackReason:            landscape.FallbackReason,
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
		metadata.RequestedPrimaryScope != membership.RequestedPrimaryScope ||
		metadata.CoveredPrimaryScope != membership.CoveredPrimaryScope ||
		metadata.UncoveredPrimaryScope != membership.UncoveredPrimaryScope ||
		metadata.CoveredSupportingEvidence != membership.CoveredSupportingEvidence ||
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
		metadata.DistinctMembers > metadata.MemberOccurrences ||
		metadata.RequestedPrimaryScope < 0 || metadata.CoveredPrimaryScope < 0 ||
		metadata.UncoveredPrimaryScope < 0 || metadata.CoveredSupportingEvidence < 0 {
		return fmt.Errorf("componentmap: synthesis record membership counts are inconsistent")
	}
	if !metadata.MembershipCounted {
		if metadata.MemberOccurrences != 0 || metadata.DistinctMembers != 0 ||
			len(metadata.RequestedMemberIDs) != 0 || len(metadata.CoveredMemberIDs) != 0 ||
			len(metadata.UncoveredMemberIDs) != 0 || metadata.RequestedPrimaryScope != 0 ||
			metadata.CoveredPrimaryScope != 0 || metadata.UncoveredPrimaryScope != 0 ||
			metadata.CoveredSupportingEvidence != 0 {
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
	unitCatalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		return err
	}
	contexts, err := synthesisCandidateContexts(bundle, unitCatalog)
	if err != nil {
		return err
	}
	requestedPrimary, coveredPrimary, uncoveredPrimary, coveredSupporting := synthesisCoverageCounts(contexts, covered)
	if metadata.RequestedPrimaryScope != requestedPrimary ||
		metadata.CoveredPrimaryScope != coveredPrimary ||
		metadata.UncoveredPrimaryScope != uncoveredPrimary ||
		metadata.CoveredSupportingEvidence != coveredSupporting {
		return fmt.Errorf("componentmap: synthesis record primary-scope coverage counts do not match")
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
	// Decision 236 / Archive 12 P0 (etcd): the single member_refs grammar
	// stays, but the response membership is BOUNDED. Without a ceiling a
	// large repository makes the model serialize an unbounded member list,
	// the output degenerates into repeated p* refs, hits the provider
	// output limit mid-array and dies with provider_output_limit. The
	// ceiling is deterministic, item-local and backend-owned: each
	// component keeps at most maxMemberRefsPerComponent distinct refs and
	// the whole response at most maxTotalMemberRefs; every member that was
	// NOT returned stays in the local remainder (existing accounting).
	// Never solved with prose ("never repeat") and never by raising the
	// provider ceiling.
	ceilingApplied, ceilingDiagnostics, ceilingErr := applyMemberRefsCeiling(&wireProposal)
	if ceilingErr != nil {
		return Landscape{}, SynthesisMembershipCounts{}, ceilingErr
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		return Landscape{}, SynthesisMembershipCounts{}, err
	}
	unitCatalog, unitErr := CompileUnitCatalog(bundle)
	if unitErr != nil {
		return Landscape{}, SynthesisMembershipCounts{}, unitErr
	}
	contexts, contextErr := synthesisCandidateContexts(bundle, unitCatalog)
	if contextErr != nil {
		return Landscape{}, SynthesisMembershipCounts{}, contextErr
	}
	proposal, wireDiagnostics, resolveErr := resolveSynthesisWireProposal(catalog, unitCatalog, wireProposal)
	if resolveErr != nil {
		landscape, err := synthesisResponseFallback(bundle, newDiagnostic(resolveErr.code, resolveErr.message))
		return landscape, SynthesisMembershipCounts{}, err
	}
	if ceilingApplied {
		wireDiagnostics = append(ceilingDiagnostics, wireDiagnostics...)
	}
	membership := synthesisMembershipCounts(bundle, contexts, proposal)
	landscape, err := Apply(bundle, proposal)
	if err != nil {
		return Landscape{}, SynthesisMembershipCounts{}, err
	}
	if len(wireDiagnostics) > 0 {
		// Decision 229 D7: unknown/wrong-kind member, unit and anchor
		// refs drop only the referencing component during wire
		// resolution; valid siblings publish as accepted_partial. The
		// counted recoverable findings travel with the landscape.
		landscape.Diagnostics = append(wireDiagnostics, landscape.Diagnostics...)
		if landscape.ValidationOutcome == ValidationAccepted ||
			landscape.ValidationOutcome == ValidationAcceptedNormalized {
			landscape.ValidationOutcome = ValidationAcceptedPartial
			landscape.Source = SourcePartialModel
			landscape.Level = 2
		} else if landscape.ValidationOutcome == ValidationRejected {
			// Zero independently valid items remained after item-scope
			// salvage: the exact original reason (unknown member/anchor
			// ref, ...) must surface in the fallback, never a generic
			// malformed-schema label.
			landscape.FallbackReason = fallbackReasonForDiagnostics(
				landscape.Diagnostics,
				hasAnyOperationalBehaviorAnchor(bundle.BehaviorAnchors),
			)
		}
		if err := landscape.Validate(bundle); err != nil {
			return Landscape{}, SynthesisMembershipCounts{}, err
		}
	}
	if landscape.ValidationOutcome != ValidationRejected {
		// Accepted status describes the canonical model-authored relation after
		// local readable-shape normalization, not a raw response cardinality that
		// normalization may have merged.
		membership = acceptedSynthesisMembershipCounts(bundle, contexts, landscape)
		qualityDiagnostics := synthesisPrimaryScopeDiagnostics(contexts, landscape)
		if len(qualityDiagnostics) > 0 {
			diagnostics := append(cloneDiagnostics(landscape.Diagnostics), qualityDiagnostics...)
			landscape, err = synthesisQualityFallback(bundle, proposal, diagnostics)
			if err != nil {
				return Landscape{}, SynthesisMembershipCounts{}, err
			}
		}
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

func synthesisMembershipCounts(
	bundle CandidateBundle,
	contexts map[MemberID]synthesisCandidateContext,
	proposal Proposal,
) SynthesisMembershipCounts {
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
	return synthesisMembershipCountsFromSets(bundle, contexts, len(proposal.Subsystems) > 0, occurrences, distinct)
}

func acceptedSynthesisMembershipCounts(
	bundle CandidateBundle,
	contexts map[MemberID]synthesisCandidateContext,
	landscape Landscape,
) SynthesisMembershipCounts {
	exclusiveDistinct := make(map[MemberID]struct{})
	sharedDistinct := make(map[MemberID]struct{})
	occurrences := 0
	for _, subsystem := range landscape.Subsystems {
		if subsystem.Category == SubsystemCategoryDiagnostic {
			continue
		}
		for _, component := range subsystem.Components {
			for _, member := range component.Members {
				occurrences++
				exclusiveDistinct[member.ID] = struct{}{}
			}
			for _, memberID := range component.SharedMemberIDs {
				sharedDistinct[memberID] = struct{}{}
			}
		}
	}
	covered := make(map[MemberID]struct{}, len(exclusiveDistinct)+len(sharedDistinct))
	for memberID := range exclusiveDistinct {
		covered[memberID] = struct{}{}
	}
	for memberID := range sharedDistinct {
		covered[memberID] = struct{}{}
		if _, alreadyExclusive := exclusiveDistinct[memberID]; !alreadyExclusive {
			// Shared participation may appear in several components, but it
			// claims exact scope rather than repeated exclusive ownership.
			// Count each shared-only member exactly once globally.
			occurrences++
		}
	}
	return synthesisMembershipCountsFromSets(bundle, contexts, true, occurrences, covered)
}

func synthesisMembershipCountsFromSets(
	bundle CandidateBundle,
	contexts map[MemberID]synthesisCandidateContext,
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
	requestedPrimary, coveredPrimary, uncoveredPrimary, coveredSupporting := 0, 0, 0, 0
	if counted {
		requestedPrimary, coveredPrimary, uncoveredPrimary, coveredSupporting = synthesisCoverageCounts(contexts, covered)
	}
	return SynthesisMembershipCounts{
		Counted:                   counted,
		MemberOccurrences:         occurrences,
		DistinctMembers:           len(coveredIDs),
		RequestedMemberIDs:        requestedIDs,
		CoveredMemberIDs:          coveredIDs,
		UncoveredMemberIDs:        uncoveredIDs,
		RequestedPrimaryScope:     requestedPrimary,
		CoveredPrimaryScope:       coveredPrimary,
		UncoveredPrimaryScope:     uncoveredPrimary,
		CoveredSupportingEvidence: coveredSupporting,
	}
}

func synthesisCoverageCounts(
	contexts map[MemberID]synthesisCandidateContext,
	covered map[MemberID]struct{},
) (requestedPrimary, coveredPrimary, uncoveredPrimary, coveredSupporting int) {
	for memberID, context := range contexts {
		_, isCovered := covered[memberID]
		if context.CoverageRole == SynthesisCoveragePrimaryScope {
			requestedPrimary++
			if isCovered {
				coveredPrimary++
			}
		} else if isCovered {
			coveredSupporting++
		}
	}
	uncoveredPrimary = requestedPrimary - coveredPrimary
	return requestedPrimary, coveredPrimary, uncoveredPrimary, coveredSupporting
}

func synthesisPrimaryScopeDiagnostics(
	contexts map[MemberID]synthesisCandidateContext,
	landscape Landscape,
) []Diagnostic {
	covered := make(map[MemberID]struct{})
	for _, subsystem := range landscape.Subsystems {
		if subsystem.Category == SubsystemCategoryDiagnostic {
			continue
		}
		for _, component := range subsystem.Components {
			for _, member := range component.Members {
				covered[member.ID] = struct{}{}
			}
			for _, memberID := range component.SharedMemberIDs {
				covered[memberID] = struct{}{}
			}
		}
	}
	requestedPrimary := 0
	coveredPrimary := 0
	primaryUnits := make(map[UnitWireRef]struct{})
	supportingUnits := make(map[UnitWireRef]struct{})
	for memberID, context := range contexts {
		if context.CoverageRole == SynthesisCoveragePrimaryScope {
			requestedPrimary++
		}
		if _, exists := covered[memberID]; !exists {
			continue
		}
		if context.CoverageRole == SynthesisCoveragePrimaryScope {
			coveredPrimary++
			primaryUnits[context.UnitRef] = struct{}{}
		} else if context.UnitRole == UnitRoleProduction {
			// A production child without production primary coverage in its
			// final unit remains a quality finding. All-supporting test, tooling
			// and documentation units are intentional and do not enter this gate.
			supportingUnits[context.UnitRef] = struct{}{}
		}
	}
	diagnostics := make([]Diagnostic, 0, 2)
	if requestedPrimary > 0 && coveredPrimary == 0 {
		diagnostics = append(diagnostics, newDiagnostic(
			"proposal.empty_primary_scope_coverage",
			"proposal covers no primary repository scope; publishing the exact local landscape",
		))
	}
	supportingOnlyUnits := 0
	for unitRef := range supportingUnits {
		if _, coveredByPrimary := primaryUnits[unitRef]; !coveredByPrimary {
			supportingOnlyUnits++
		}
	}
	if supportingOnlyUnits > 0 {
		diagnostics = append(diagnostics, newDiagnostic(
			"proposal.supporting_only_unit_coverage",
			fmt.Sprintf(
				"proposal covers supporting evidence in %d unit(s) without primary scope from the same unit; publishing the exact local landscape",
				supportingOnlyUnits,
			),
		))
	}
	return diagnostics
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

func synthesisQualityFallback(
	bundle CandidateBundle,
	proposal Proposal,
	diagnostics []Diagnostic,
) (Landscape, error) {
	landscape := buildDeterministicLocalLandscape(bundle, SourcePackageFallback)
	landscape.Diagnostics = cloneDiagnostics(diagnostics)
	landscape.ValidationOutcome = ValidationRejected
	landscape.FallbackReason = fallbackReasonForDiagnostics(
		landscape.Diagnostics,
		hasAnyOperationalBehaviorAnchor(bundle.BehaviorAnchors),
	)
	landscape.Fallback = true
	landscape.OriginalProposalSHA256 = proposalSHA256(proposal)
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
		// Decision 235 (v11) Gotify Option 1: deterministic normalization
		// ONLY when exactly one complete JSON object parses and every
		// trailing byte is whitespace or a bounded sequence of unmatched
		// closing ] / } delimiters (Gotify corpus response ended with a
		// redundant "]}"). Any other trailing content stays invalid; no
		// broad JSON repair and no embedded-object extraction.
		if normalized, ok := normalizeTrailingClosingDelimiters(trimmed); ok {
			diag := newDiagnostic(
				"response.trailing_closing_delimiters_normalized",
				"provider response had a bounded sequence of unmatched trailing closing delimiters; normalized deterministically",
			)
			return normalized, &diag, nil
		}
		return nil, nil, &synthesisResponseError{code: "response.invalid_proposal", message: "provider response is not exactly one complete json object"}
	}
	if trimmed[0] != '{' {
		return nil, nil, &synthesisResponseError{code: "response.invalid_proposal", message: "provider response is json but not a proposal object"}
	}
	return append([]byte(nil), trimmed...), nil, nil
}

// normalizeTrailingClosingDelimiters implements Decision 235 Gotify Option 1:
// if the raw bytes contain exactly one complete JSON object and every byte
// after its end is whitespace or an unmatched closing ] / } delimiter (at
// most maxTrailingClosingDelimiters of them), return the trimmed object.
// The returned object must itself validate as exactly one complete JSON
// object; any other trailing content (letters, digits, extra opening
// brackets, second object fragments) fails closed.
func normalizeTrailingClosingDelimiters(raw []byte) ([]byte, bool) {
	const maxTrailingClosingDelimiters = 8
	// The greedy scan: find the first index where the prefix is valid JSON.
	// Scanning from the end is unsafe (the object may end with nested ]}),
	// so scan candidate end indices by bracket depth instead.
	depth := 0
	inString := false
	escaped := false
	objectEnd := -1
	for index, b := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 && objectEnd < 0 {
				// Candidate end of the outermost JSON object. The object
				// must start with '{' and the prefix must parse as valid
				// JSON on its own.
				if index > 0 && raw[0] == '{' && json.Valid(raw[:index+1]) {
					objectEnd = index
				}
			}
			if depth < 0 {
				// Unbalanced early: the prefix before this byte may still
				// be a complete object only if depth went negative on the
				// FIRST closing delimiter beyond the object — that case is
				// handled by the trailing scan below, so stop looking for
				// more complete objects.
				if objectEnd < 0 {
					return nil, false
				}
			}
		}
		if objectEnd >= 0 && depth < 0 {
			break
		}
	}
	if objectEnd < 0 {
		return nil, false
	}
	trailing := raw[objectEnd+1:]
	trailing = bytes.TrimSpace(trailing)
	if len(trailing) == 0 {
		// Exact single complete object — normal path already handled this.
		return append([]byte(nil), raw[:objectEnd+1]...), true
	}
	if len(trailing) > maxTrailingClosingDelimiters {
		return nil, false
	}
	for _, b := range trailing {
		if b != ']' && b != '}' {
			return nil, false
		}
	}
	object := append([]byte(nil), raw[:objectEnd+1]...)
	if !json.Valid(object) || len(bytes.TrimSpace(object)) == 0 {
		return nil, false
	}
	return object, true
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
	// Decision 235 (v11): set by the decoder when the provider omitted the
	// optional anchor_refs field; the resolver counts it as a mechanical
	// normalization instead of a schema violation.
	NormalizedMissingAnchorRefs bool
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
// synthesisWireNestedSubsystem is the provider-visible nested Architecture
// response grammar (Phase 1 prompt contract cleanup). The model chooses
// meaning only: subsystem/component names, descriptions, and which supplied
// member/anchor refs distinguish each component. The model never invents
// g* IDs, never emits kind/subsystem_ref, never maintains parent/child
// adjacency foreign keys, and never counts refs/components.
type synthesisWireNestedSubsystem struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description,omitempty"`
	Components  []synthesisWireNestedComponent `json:"components"`
}

type synthesisWireNestedComponent struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	MemberRefs  []string        `json:"member_refs,omitempty"`
	UnitRefs    []string        `json:"unit_refs,omitempty"`
	AnchorRefs  json.RawMessage `json:"anchor_refs,omitempty"`
}

type synthesisWireNested struct {
	Subsystems []synthesisWireNestedSubsystem `json:"subsystems"`
}

// decodeSynthesisWireProposalJSON decodes the nested semantic Architecture
// response (Phase 1) and projects it onto the internal flat record shape:
// the backend assigns g* subsystem refs in wire order and derives
// subsystem_ref adjacency from nesting, so the model never carries foreign
// keys. Refs are plain request-local strings; the backend restores kinds.
// Historical flat tagged-record responses remain readable (Phase 6: immutable
// saved artifacts stay replayable without a live provider route); the active
// provider contract is the nested grammar only.
func decodeSynthesisWireProposalJSON(raw []byte) (synthesisWireProposal, error) {
	if !utf8.Valid(raw) {
		return synthesisWireProposal{}, fmt.Errorf("proposal is not valid utf-8")
	}
	if err := rejectDuplicateSynthesisJSONKeys(raw); err != nil {
		return synthesisWireProposal{}, err
	}
	// Active contract: nested subsystems grammar (Phase 1). Try it first;
	// a response whose root is an object without a `subsystems` array falls
	// through to the historical flat records grammar.
	if bytes.Contains(raw, []byte(`"subsystems"`)) {
		nested, err := decodeSynthesisWireNestedJSON(raw)
		if err == nil {
			return nested, nil
		}
		// Fall through to the historical grammar only when the bytes carry
		// both shapes ambiguously; a pure nested response must not be
		// re-read as flat.
		if !bytes.Contains(raw, []byte(`"records"`)) {
			return synthesisWireProposal{}, err
		}
	}
	return decodeSynthesisWireFlatJSON(raw)
}

// decodeSynthesisWireNestedJSON decodes the active nested grammar.
func decodeSynthesisWireNestedJSON(raw []byte) (synthesisWireProposal, error) {
	var nested synthesisWireNested
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&nested); err != nil {
		return synthesisWireProposal{}, fmt.Errorf("proposal is not a nested subsystems object: %w", err)
	}
	if len(nested.Subsystems) == 0 || len(nested.Subsystems) > MaxPrimarySubsystems {
		return synthesisWireProposal{}, fmt.Errorf("proposal subsystem count is outside the bounded contract")
	}
	proposal := synthesisWireProposal{Records: make([]synthesisWireRecord, 0, maxSynthesisWireRecords)}
	totalComponents := 0
	for index, subsystem := range nested.Subsystems {
		if strings.TrimSpace(subsystem.Name) == "" {
			return synthesisWireProposal{}, fmt.Errorf("proposal subsystem name is empty")
		}
		if err := validateSynthesisProposalProse("subsystem description", subsystem.Description); err != nil {
			return synthesisWireProposal{}, err
		}
		subsystemRef := fmt.Sprintf("g%d", index+1)
		proposal.Records = append(proposal.Records, synthesisWireRecord{
			Kind: synthesisWireSubsystemRecord, Ref: subsystemRef,
			Name: subsystem.Name, Description: subsystem.Description,
		})
		if len(subsystem.Components) == 0 {
			return synthesisWireProposal{}, fmt.Errorf("proposal subsystem %q has no components", subsystem.Name)
		}
		if len(subsystem.Components) > MaxComponentsPerSubsystem {
			return synthesisWireProposal{}, fmt.Errorf("proposal component count exceeds the bounded contract")
		}
		for _, component := range subsystem.Components {
			if strings.TrimSpace(component.Name) == "" {
				return synthesisWireProposal{}, fmt.Errorf("proposal component name is empty")
			}
			if err := validateSynthesisProposalProse("component description", component.Description); err != nil {
				return synthesisWireProposal{}, err
			}
			memberRefs, err := nestedMemberRefs(component.MemberRefs, "member_refs")
			if err != nil {
				return synthesisWireProposal{}, err
			}
			unitRefs, err := nestedUnitRefs(component.UnitRefs, "unit_refs")
			if err != nil {
				return synthesisWireProposal{}, err
			}
			if len(memberRefs) > 0 && len(unitRefs) > 0 {
				return synthesisWireProposal{}, fmt.Errorf("proposal component must group either member_refs or unit_refs, not both")
			}
			if len(memberRefs) == 0 && len(unitRefs) == 0 {
				return synthesisWireProposal{}, fmt.Errorf("proposal component must group at least one member_refs or unit_refs")
			}
			// Decision 237: field presence is semantically distinct from array
			// cardinality. An omitted optional field is normalized, an explicit
			// empty array is already valid, and null/non-array values fail closed.
			anchorRefsFieldExists := component.AnchorRefs != nil
			rawAnchorRefs := []string{}
			if anchorRefsFieldExists {
				if isJSONNull(component.AnchorRefs) {
					return synthesisWireProposal{}, fmt.Errorf("proposal component fields must not be null")
				}
				if err := json.Unmarshal(component.AnchorRefs, &rawAnchorRefs); err != nil {
					return synthesisWireProposal{}, fmt.Errorf("proposal anchor refs have invalid type")
				}
			}
			anchorRefs, err := nestedAnchorRefs(rawAnchorRefs, "anchor_refs")
			if err != nil {
				return synthesisWireProposal{}, err
			}
			if len(anchorRefs) > maxAnchorMembers {
				return synthesisWireProposal{}, fmt.Errorf("proposal component anchor count exceeds the bounded contract")
			}
			record := synthesisWireRecord{
				Kind: synthesisWireComponentRecord, SubsystemRef: subsystemRef,
				Name: component.Name, Description: component.Description,
				MemberRefs: memberRefs, UnitRefs: unitRefs, AnchorRefs: anchorRefs,
			}
			if !anchorRefsFieldExists {
				// Backend-owned normalization: omitted anchor_refs on a
				// component is a mechanical default, counted like the
				// flat contract's missing-anchor-refs normalization.
				record.NormalizedMissingAnchorRefs = true
			}
			proposal.Records = append(proposal.Records, record)
			totalComponents++
		}
	}
	if totalComponents == 0 || totalComponents > MaxTotalNestedComponents {
		return synthesisWireProposal{}, fmt.Errorf("proposal component count exceeds the bounded contract")
	}
	return proposal, nil
}

// decodeSynthesisWireFlatJSON decodes the historical flat tagged-record
// grammar (pre-Phase-1). It stays readable so immutable saved responses
// replay deterministically; it is not the live provider contract.
func decodeSynthesisWireFlatJSON(raw []byte) (synthesisWireProposal, error) {
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

// nestedStringRefs decodes a plain request-local string ref array. Kinds are
// backend-owned: the model returns bare refs, never {"kind":...,"ref":...}.
func nestedStringRefs(raw []string, field string) ([]SynthesisMemberRef, error) {
	refs := make([]SynthesisMemberRef, 0, len(raw))
	for index, value := range raw {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("proposal %s[%d] is empty", field, index)
		}
		refs = append(refs, SynthesisMemberRef{Ref: value})
	}
	return refs, nil
}

// nestedMemberRefs decodes plain member refs (p*/s*/f*).
func nestedMemberRefs(raw []string, field string) ([]SynthesisMemberRef, error) {
	return nestedStringRefs(raw, field)
}

// nestedUnitRefs decodes plain unit refs (u*).
func nestedUnitRefs(raw []string, field string) ([]SynthesisUnitRef, error) {
	refs := make([]SynthesisUnitRef, 0, len(raw))
	for index, value := range raw {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("proposal %s[%d] is empty", field, index)
		}
		refs = append(refs, SynthesisUnitRef{Ref: value})
	}
	return refs, nil
}

// nestedAnchorRefs decodes plain anchor refs (a*).
func nestedAnchorRefs(raw []string, field string) ([]SynthesisAnchorRef, error) {
	refs := make([]SynthesisAnchorRef, 0, len(raw))
	for index, value := range raw {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("proposal %s[%d] is empty", field, index)
		}
		refs = append(refs, SynthesisAnchorRef{Ref: value})
	}
	return refs, nil
}

// validateSynthesisProposalProse bounds a nested proposal prose value.
func validateSynthesisProposalProse(label, value string) error {
	if err := validateDisplayText(label, value, maxNameBytes, false); err != nil {
		return err
	}
	return nil
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
		// legal shapes. Decision 230 D9: `hypothesis` is advisory model
		// input (the prompt says the backend derives the product
		// hypothesis status exclusively from exact proof, and Decision
		// 228 applies it deterministically); a provider that omits it
		// does not invalidate an otherwise well-formed proposal.
		// Decision 235 (v11): `anchor_refs` is OPTIONAL — a provider that
		// omits it is normalized to [] with a counted normalization
		// (Soft Serve: 14 useful components, 0 anchor_refs). The field
		// set therefore accepts member_refs|unit_refs without anchor_refs.
		if !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "member_refs", "anchor_refs", "hypothesis",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "unit_refs", "anchor_refs", "hypothesis",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "member_refs", "anchor_refs",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "unit_refs", "anchor_refs",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "member_refs", "hypothesis",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "unit_refs", "hypothesis",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "member_refs",
		) && !hasExactSynthesisFields(
			fields, "kind", "subsystem_ref", "name", "description", "unit_refs",
		) {
			return synthesisWireRecord{}, fmt.Errorf("proposal component record fields do not match the bounded contract")
		}
		// Decision 235 (v11): missing anchor_refs normalizes to an empty
		// array — optional-field omission is a mechanical default, not a
		// schema violation.
		_, anchorRefsFieldExists := fields["anchor_refs"]
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
		if !anchorRefsFieldExists && isJSONNull(fields["hypothesis"]) {
			return synthesisWireRecord{}, fmt.Errorf("proposal component fields must not be null")
		}
		if anchorRefsFieldExists && isJSONNull(fields["anchor_refs"]) {
			return synthesisWireRecord{}, fmt.Errorf("proposal component fields must not be null")
		}
		// Decision 235 (v11): a missing anchor_refs field normalizes to an
		// empty array (recorded via NormalizedMissingAnchorRefs so the
		// resolver can count it as a mechanical normalization).
		if !anchorRefsFieldExists {
			fields["anchor_refs"] = json.RawMessage(`[]`)
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
		if rawHypothesis, exists := fields["hypothesis"]; exists && !isJSONNull(rawHypothesis) {
			if err := json.Unmarshal(rawHypothesis, &hypothesis); err != nil {
				return synthesisWireRecord{}, fmt.Errorf("proposal hypothesis has invalid type")
			}
		}
		// Decision 230 D9: absent hypothesis defaults to false; the
		// backend derives the product hypothesis deterministically from
		// exact proof (Decision 228) and overwrites this advisory input.
		record := synthesisWireRecord{
			Kind: kind, SubsystemRef: subsystemRef, Name: name, Description: description,
			MemberRefs: memberRefs, UnitRefs: unitRefs, AnchorRefs: anchorRefs, Hypothesis: hypothesis,
		}
		// Decision 235 (v11): the decoder records a missing-anchor-refs
		// normalization so the resolver can surface it as a counted
		// mechanical normalization (Soft Serve class).
		if !anchorRefsFieldExists {
			record.NormalizedMissingAnchorRefs = true
		}
		return record, nil
	default:
		return synthesisWireRecord{}, fmt.Errorf("proposal record kind is invalid")
	}
}

func decodeSynthesisMemberRef(raw json.RawMessage) (SynthesisMemberRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisMemberRef{}, fmt.Errorf("proposal member ref is not an object")
	}
	// Decision 231 (Archive 9): ref kind is backend-owned — the model may
	// omit it entirely ({"ref":"p11"}); a supplied kind is still validated.
	// This closes the D231 gap where unit_refs/anchor_refs allowed kind
	// omission but member_refs still required it, whole-rejecting otherwise
	// valid proposals (miniflux live run 20260806-223340).
	if !hasExactSynthesisFields(fields, "ref") && !hasExactSynthesisFields(fields, "kind", "ref") {
		return SynthesisMemberRef{}, fmt.Errorf("proposal member ref fields do not match the bounded contract")
	}
	ref, err := decodeRequiredProposalString(fields, "ref")
	if err != nil {
		return SynthesisMemberRef{}, err
	}
	var kind MemberKind
	if rawKind, exists := fields["kind"]; exists && !isJSONNull(rawKind) {
		decodedKind, kindErr := decodeRequiredProposalString(fields, "kind")
		if kindErr != nil {
			return SynthesisMemberRef{}, kindErr
		}
		kind = MemberKind(decodedKind)
	}
	return SynthesisMemberRef{Kind: kind, Ref: ref}, nil
}

func decodeSynthesisUnitRef(raw json.RawMessage) (SynthesisUnitRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisUnitRef{}, fmt.Errorf("proposal unit ref is not an object")
	}
	// Decision 231 (Archive 9): ref kind is backend-owned — the model may
	// omit it entirely ({"ref":"u1"}); a supplied kind is still validated.
	if !hasExactSynthesisFields(fields, "ref") && !hasExactSynthesisFields(fields, "kind", "ref") {
		return SynthesisUnitRef{}, fmt.Errorf("proposal unit ref fields do not match the bounded contract")
	}
	ref, err := decodeRequiredProposalString(fields, "ref")
	if err != nil {
		return SynthesisUnitRef{}, err
	}
	kind := MemberPackage
	if rawKind, exists := fields["kind"]; exists && !isJSONNull(rawKind) {
		decodedKind, kindErr := decodeRequiredProposalString(fields, "kind")
		if kindErr != nil {
			return SynthesisUnitRef{}, kindErr
		}
		kind = MemberKind(decodedKind)
	}
	return SynthesisUnitRef{Kind: kind, Ref: ref}, nil
}

func decodeSynthesisAnchorRef(raw json.RawMessage) (SynthesisAnchorRef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return SynthesisAnchorRef{}, fmt.Errorf("proposal anchor ref is not an object")
	}
	// Decision 231 (Archive 9): anchor kind is backend-owned — the model
	// may omit it ({"ref":"a1"}); a supplied kind is still validated.
	if !hasExactSynthesisFields(fields, "ref") && !hasExactSynthesisFields(fields, "kind", "ref") {
		return SynthesisAnchorRef{}, fmt.Errorf("proposal anchor ref fields do not match the bounded contract")
	}
	ref, err := decodeRequiredProposalString(fields, "ref")
	if err != nil {
		return SynthesisAnchorRef{}, err
	}
	var kind BehaviorAnchorKind
	if rawKind, exists := fields["kind"]; exists && !isJSONNull(rawKind) {
		decodedKind, kindErr := decodeRequiredProposalString(fields, "kind")
		if kindErr != nil {
			return SynthesisAnchorRef{}, kindErr
		}
		kind = BehaviorAnchorKind(decodedKind)
	}
	return SynthesisAnchorRef{Kind: kind, Ref: ref}, nil
}

// Decision 236 / Archive 12 P0 (etcd): bounded response membership. Each
// component keeps at most maxMemberRefsPerComponent distinct member refs
// and the whole proposal at most maxTotalMemberRefs total member refs.
// The bounds are generous for real repositories but stop the degenerate
// "serialize every member, repeat p* refs, blow the provider budget"
// failure mode. Members not returned stay in the existing local remainder
// accounting — the ceiling never fabricates membership and never drops a
// component.
const (
	maxMemberRefsPerComponent = 24
	maxTotalMemberRefs        = 320
)

// applyMemberRefsCeiling trims each component's member_refs to the bounded
// maximum, then the whole proposal to the total maximum. It returns true
// and exact diagnostics when any trimming happened; the diagnostics carry
// the precise counts so the report can state what was bounded.
func applyMemberRefsCeiling(proposal *synthesisWireProposal) (bool, []Diagnostic, error) {
	if proposal == nil {
		return false, nil, fmt.Errorf("componentmap: member refs ceiling requires a proposal")
	}
	applied := false
	perComponentTrimmed := 0
	for recordIndex := range proposal.Records {
		record := &proposal.Records[recordIndex]
		if record.Kind != synthesisWireComponentRecord {
			continue
		}
		if len(record.MemberRefs) > maxMemberRefsPerComponent {
			trimmed := len(record.MemberRefs) - maxMemberRefsPerComponent
			// Keep the FIRST refs in wire order (the model's stated
			// priority); the rest stay in the local remainder.
			record.MemberRefs = record.MemberRefs[:maxMemberRefsPerComponent]
			perComponentTrimmed += trimmed
			applied = true
		}
	}
	var diagnostics []Diagnostic
	total := 0
	for _, record := range proposal.Records {
		if record.Kind != synthesisWireComponentRecord {
			continue
		}
		total += len(record.MemberRefs)
	}
	if total > maxTotalMemberRefs {
		// Total bound: drop from the END of the last component that still
		// has refs, then backwards, so the earliest components (the
		// model's stated priority) keep their membership.
		toDrop := total - maxTotalMemberRefs
		for recordIndex := len(proposal.Records) - 1; recordIndex >= 0 && toDrop > 0; recordIndex-- {
			record := &proposal.Records[recordIndex]
			if record.Kind != synthesisWireComponentRecord {
				continue
			}
			if len(record.MemberRefs) == 0 {
				continue
			}
			drop := len(record.MemberRefs)
			if drop > toDrop {
				drop = toDrop
			}
			record.MemberRefs = record.MemberRefs[:len(record.MemberRefs)-drop]
			toDrop -= drop
		}
		diagnostics = append(diagnostics, newDiagnostic(
			"response.member_refs_total_ceiling",
			fmt.Sprintf(
				"provider response membership exceeded the bounded total (%d); trimmed deterministically to %d refs; remaining members stay in the local remainder",
				maxTotalMemberRefs, maxTotalMemberRefs,
			),
		))
		applied = true
	}
	if perComponentTrimmed > 0 {
		diagnostics = append(diagnostics, newDiagnostic(
			"response.member_refs_per_component_ceiling",
			fmt.Sprintf(
				"provider response membership exceeded the bounded per-component maximum (%d distinct refs); trimmed %d refs across components; remaining members stay in the local remainder",
				maxMemberRefsPerComponent, perComponentTrimmed,
			),
		))
	}
	return applied, diagnostics, nil
}

func resolveSynthesisWireProposal(
	catalog synthesisPrivateCatalog,
	unitCatalog UnitCatalog,
	wire synthesisWireProposal,
) (Proposal, []Diagnostic, *synthesisResponseError) {
	proposal := Proposal{Version: ProposalVersion}
	// Decision 229 D7: recoverable findings collected while dropping
	// components with unknown/wrong-kind refs item-scope during wire
	// resolution. Structural contract violations stay whole-response
	// errors; ref resolution failures drop only the referencing
	// component so valid siblings publish.
	var wireDiagnostics []Diagnostic
	salvageComponent := func(code, message string) {
		wireDiagnostics = append(wireDiagnostics, newDiagnostic(code, message))
	}
	subsystemIndexes := make(map[string]int)
	componentCounts := make(map[string]int)
	totalComponents := 0
	for _, record := range wire.Records {
		if err := validateSynthesisResponseDisplay(record); err != nil {
			return Proposal{}, wireDiagnostics, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal display text is outside the bounded contract",
			}
		}
		switch record.Kind {
		case synthesisWireSubsystemRecord:
			if !validSynthesisSubsystemRef(record.Ref) {
				return Proposal{}, wireDiagnostics, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal subsystem ref is malformed",
				}
			}
			if _, duplicate := subsystemIndexes[record.Ref]; duplicate {
				return Proposal{}, wireDiagnostics, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal repeats a response-local subsystem ref",
				}
			}
			subsystemIndexes[record.Ref] = len(proposal.Subsystems)
			proposal.Subsystems = append(proposal.Subsystems, ProposedSubsystem{
				Name: record.Name, Description: record.Description,
			})
		case synthesisWireComponentRecord:
			if !validSynthesisSubsystemRef(record.SubsystemRef) {
				return Proposal{}, wireDiagnostics, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal component subsystem ref is malformed",
				}
			}
			if len(record.AnchorRefs) > maxAnchorMembers {
				return Proposal{}, wireDiagnostics, &synthesisResponseError{
					code: "response.invalid_proposal", message: "proposal component anchor count exceeds the bounded contract",
				}
			}
			componentCounts[record.SubsystemRef]++
			totalComponents++
		}
	}
	if len(proposal.Subsystems) == 0 || len(proposal.Subsystems) > MaxPrimarySubsystems {
		return Proposal{}, wireDiagnostics, &synthesisResponseError{
			code: "response.invalid_proposal", message: "proposal subsystem count is outside the bounded contract",
		}
	}
	if totalComponents == 0 || totalComponents > MaxTotalNestedComponents {
		return Proposal{}, wireDiagnostics, &synthesisResponseError{
			code: "response.invalid_proposal", message: "proposal component count exceeds the bounded contract",
		}
	}
	// Decision 230 D9.7 (salvage contract v6 "repeated broad unit"): when
	// several components reference the SAME unit ref, that unit is shared
	// scope, not independent ownership — each referencing component keeps
	// only its anchor-specific slice (the exact members its anchors own)
	// instead of the full broad unit. A unit referenced exactly once keeps
	// its full expansion.
	unitUsage := make(map[string]int)
	for _, record := range wire.Records {
		if record.Kind != synthesisWireComponentRecord {
			continue
		}
		for _, unitRef := range record.UnitRefs {
			unitUsage[unitRef.Ref]++
		}
	}
	// Decision 235 (v11): a provider that omits the optional anchor_refs
	// field is normalized to [] — a counted mechanical normalization, never
	// a rejection (Soft Serve class: 14 useful components, 0 anchor_refs).
	normalizedAnchorCount := 0
	for _, record := range wire.Records {
		if record.Kind == synthesisWireComponentRecord && record.NormalizedMissingAnchorRefs {
			normalizedAnchorCount++
		}
	}
	if normalizedAnchorCount > 0 {
		wireDiagnostics = append(wireDiagnostics, newDiagnostic(
			"proposal.normalized_missing_anchor_refs",
			fmt.Sprintf("proposal omitted optional anchor_refs on %d component(s); normalized to empty arrays", normalizedAnchorCount),
		))
	}
	for subsystemRef, componentCount := range componentCounts {
		if _, exists := subsystemIndexes[subsystemRef]; !exists {
			return Proposal{}, wireDiagnostics, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component references an unknown response-local subsystem ref",
			}
		}
		if componentCount > MaxComponentsPerSubsystem {
			return Proposal{}, wireDiagnostics, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component count exceeds the bounded contract",
			}
		}
	}
	for _, record := range wire.Records {
		if record.Kind != synthesisWireComponentRecord {
			continue
		}
		if !validSynthesisSubsystemRef(record.SubsystemRef) {
			return Proposal{}, wireDiagnostics, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component subsystem ref is malformed",
			}
		}
		subsystemIndex, exists := subsystemIndexes[record.SubsystemRef]
		if !exists {
			return Proposal{}, wireDiagnostics, &synthesisResponseError{
				code: "response.invalid_proposal", message: "proposal component references an unknown response-local subsystem ref",
			}
		}
		component := ProposedComponent{
			Name: record.Name, Description: record.Description,
			Hypothesis: record.Hypothesis,
			MemberIDs:  make([]MemberID, 0, len(record.MemberRefs)+len(record.UnitRefs)),
			AnchorIDs:  make([]string, 0, len(record.AnchorRefs)),
		}
		// Decision 229 D7: ref-resolution failures are item-scope — the
		// referencing component is dropped with a counted recoverable
		// finding; valid sibling components still publish. Structural
		// contract violations (malformed refs, unknown subsystem refs,
		// count bounds) remain whole-response errors above.
		componentSalvaged := false
		for _, memberRef := range record.MemberRefs {
			// Decision 231 (Archive 9, gap closure): a model may omit the
			// member kind ({"ref":"p11"}); the catalog owns kinds. A
			// supplied wrong kind still fails item-scope. With an omitted
			// kind the ref is resolved by the ref-only catalog key.
			if memberRef.Kind != "" {
				if expectedKind, exists := catalog.memberKinds[memberRef.Ref]; exists && expectedKind != memberRef.Kind {
					salvageComponent("proposal.unknown_member_id", "proposal member ref has the wrong request-local kind")
					componentSalvaged = true
					break
				}
			}
			var memberID MemberID
			var exists bool
			if memberRef.Kind == "" {
				memberID, exists = catalog.membersByRefOnly[memberRef.Ref]
			} else {
				memberID, exists = catalog.membersByRef[memberRef.key()]
			}
			if !exists {
				salvageComponent("proposal.unknown_member_id", "proposal references an unknown request-local member ref")
				componentSalvaged = true
				break
			}
			var memberRole CandidateRole
			if memberRef.Kind == "" {
				memberRole = catalog.memberRolesByRefOnly[memberRef.Ref]
			} else {
				memberRole = catalog.memberRoles[memberRef.key()]
			}
			if memberRole != CandidateRoleConceptualMember {
				salvageComponent("proposal.unknown_member_id", "proposal returned a structural locator as conceptual membership")
				componentSalvaged = true
				break
			}
			component.MemberIDs = append(component.MemberIDs, memberID)
		}
		// Decision 230 D9.7 / 231: anchors resolve with exact catalog
		// kinds; anchor membership stays attached for grounding and UI
		// (shared participation needs no exclusive member slice).
		seenAnchors := make(map[string]struct{}, len(record.AnchorRefs))
		for _, anchorRef := range record.AnchorRefs {
			// Decision 231 (Archive 9): a model may omit the anchor kind
			// ({"ref":"a1"}); the catalog owns kinds. A supplied wrong
			// kind still fails item-scope. With an omitted kind the ref is
			// resolved by the ref-only catalog key.
			if anchorRef.Kind != "" {
				if expectedKind, exists := catalog.anchorKinds[anchorRef.Ref]; exists && expectedKind != anchorRef.Kind {
					salvageComponent("proposal.unknown_anchor_id", "proposal anchor ref has the wrong request-local kind")
					componentSalvaged = true
					break
				}
			}
			var anchorID string
			if anchorRef.Kind == "" {
				anchorID, exists = catalog.anchorsByRefOnly[anchorRef.Ref]
			} else {
				anchorID, exists = catalog.anchorsByRef[anchorRef.key()]
			}
			if !exists {
				salvageComponent("proposal.unknown_anchor_id", "proposal references an unknown request-local anchor ref")
				componentSalvaged = true
				break
			}
			if _, duplicate := seenAnchors[anchorID]; duplicate {
				salvageComponent("proposal.duplicate_anchor_id", "proposal repeats an anchor ref within one component")
				componentSalvaged = true
				break
			}
			seenAnchors[anchorID] = struct{}{}
			component.AnchorIDs = append(component.AnchorIDs, anchorID)
		}
		if !componentSalvaged && len(record.UnitRefs) > 0 {
			// Decision 216: a component grouping unit refs (u*) expands locally
			// to the exact unit members. Unknown, duplicate, or wrong-kind unit
			// refs fail closed — never repaired, never guessed.
			unitMembersByRef := unitCatalogUnitMembersByWireRef(unitCatalog)
			seenUnits := make(map[string]struct{}, len(record.UnitRefs))
			for _, unitRef := range record.UnitRefs {
				// Decision 231 (Archive 9): the model may omit the unit
				// kind ({"ref":"u1"}); units are package scope by
				// contract. A supplied wrong kind still fails item-scope.
				if unitRef.Kind != "" && unitRef.Kind != MemberPackage {
					salvageComponent("proposal.unknown_unit_ref", "proposal unit ref has the wrong request-local kind")
					componentSalvaged = true
					break
				}
				members, exists := unitMembersByRef[unitRef.Ref]
				if !exists {
					salvageComponent("proposal.unknown_unit_ref", "proposal references an unknown request-local unit ref")
					componentSalvaged = true
					break
				}
				if _, duplicate := seenUnits[unitRef.Ref]; duplicate {
					salvageComponent("proposal.duplicate_unit_ref", "proposal repeats a unit ref within one component")
					componentSalvaged = true
					break
				}
				seenUnits[unitRef.Ref] = struct{}{}
				if unitUsage[unitRef.Ref] > 1 {
					// Decision 231 (Archive 9): repeated broad unit = shared
					// participation, never exclusive ownership. The component
					// keeps the unit ref in SharedUnitRefs and its exact
					// anchors; it does NOT need an exclusive member slice to
					// survive. Package units and symbol anchors legitimately
					// differ in kind — the old package∩symbol slice was empty
					// by type and deleted useful roles (Telebot/Chatto whole
					// reject, Restic dispatcher loss). Anchors stay attached
					// for grounding and UI; Apply emits the counted
					// shared_unit_slice finding when the participation
					// publishes.
					component.SharedUnitRefs = append(component.SharedUnitRefs, unitRef.Ref)
				} else {
					component.MemberIDs = append(component.MemberIDs, members...)
				}
			}
		}
		if componentSalvaged {
			// Item-scope drop: the component is skipped; valid sibling
			// components still publish (Decision 229 D7).
			continue
		}
		proposal.Subsystems[subsystemIndex].Components = append(
			proposal.Subsystems[subsystemIndex].Components, component,
		)
	}
	for _, subsystem := range proposal.Subsystems {
		if len(subsystem.Components) == 0 {
			// Decision 229 D7: a subsystem whose components were all
			// dropped item-scope during wire resolution is left empty —
			// Apply decides: valid sibling subsystems publish, and a
			// proposal where every subsystem ends empty rejects
			// whole-stage with the exact original reason.
			continue
		}
	}
	return proposal, wireDiagnostics, nil
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
