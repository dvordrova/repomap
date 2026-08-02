package componentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	// ContractVersion changes whenever candidate identity, proposal authority,
	// or locally validated landscape semantics change.
	ContractVersion = 6
	ProposalVersion = 6

	maxCandidates        = 512
	maxFlows             = 64
	maxRelations         = 1_024
	maxAnchorBindings    = 2_048
	maxBehaviorAnchors   = 256
	maxAnchorMembers     = 16
	maxLimitations       = 8
	maxFactsPerCandidate = 16
	maxFlowsPerCandidate = 16
	maxSubsystems        = 16
	maxComponents        = 32
	// Conceptual membership is many-to-many. Bound relation cardinality
	// independently from the number of exact local candidates so a single
	// cross-cutting member cannot consume an unbounded response budget.
	maxConceptualMemberships          = 2_048
	maxConceptualMembershipsPerMember = 8
	maxProvenanceItems                = 8
	maxScenarioContexts               = 8
	maxResearchFindings               = 64
	maxNameBytes                      = 256
	maxDescriptionBytes               = 1_024
	maxOpaqueIDBytes                  = 128
	maxFactValueBytes                 = 2_048
	maxPathBytes                      = 4_096
	maxProvenanceBytes                = 1_024
)

// MemberKind gives an opaque ID enough local type information to prevent a
// package ID and a file ID from accidentally referring to the same member.
type MemberKind string

const (
	MemberPackage    MemberKind = "package"
	MemberFile       MemberKind = "file"
	MemberSymbol     MemberKind = "symbol"
	MemberEntrypoint MemberKind = "entrypoint"
	MemberFlow       MemberKind = "flow"
)

func (kind MemberKind) valid() bool {
	switch kind {
	case MemberPackage, MemberFile, MemberSymbol, MemberEntrypoint, MemberFlow:
		return true
	default:
		return false
	}
}

// MemberID is deliberately opaque to conceptual synthesis. Callers may use
// the value only for exact equality and round-tripping; Kind is the sole
// semantic information carried by the ID itself.
type MemberID struct {
	Kind  MemberKind `json:"kind"`
	Value string     `json:"value"`
}

func (id MemberID) key() string {
	return string(id.Kind) + "\x00" + id.Value
}

// FlowID is an exact ID from a saved local flow contract.
type FlowID string

// RepositoryArchetype is a bounded, locally assessed repository shape. It is
// context for synthesis, not a model-authored architecture claim.
type RepositoryArchetype string

const (
	ArchetypeApplication           RepositoryArchetype = "application"
	ArchetypeModularPlatformServer RepositoryArchetype = "modular_platform_server"
	ArchetypeLibraryFramework      RepositoryArchetype = "library_framework"
	ArchetypeCLITool               RepositoryArchetype = "cli_tool"
	ArchetypeDaemonWorkerSystem    RepositoryArchetype = "daemon_worker_system"
	ArchetypeMonorepoMixed         RepositoryArchetype = "monorepo_mixed"
)

func (archetype RepositoryArchetype) valid() bool {
	switch archetype {
	case ArchetypeApplication, ArchetypeModularPlatformServer, ArchetypeLibraryFramework,
		ArchetypeCLITool, ArchetypeDaemonWorkerSystem, ArchetypeMonorepoMixed:
		return true
	default:
		return false
	}
}

// GroundingMode states how much of the primary architecture is supported by
// exact behavioral evidence.
type GroundingMode string

const (
	GroundingBehavior GroundingMode = "behavior_grounded"
	GroundingMixed    GroundingMode = "mixed"
	GroundingPackages GroundingMode = "package_landscape"
)

func (mode GroundingMode) valid() bool {
	return mode == GroundingBehavior || mode == GroundingMixed || mode == GroundingPackages
}

type BehaviorAnchorKind string

const (
	AnchorProcessEntry        BehaviorAnchorKind = "process_entry"
	AnchorCommandDispatch     BehaviorAnchorKind = "command_dispatch"
	AnchorConfigIngress       BehaviorAnchorKind = "config_ingress"
	AnchorConfigAdapter       BehaviorAnchorKind = "config_adapter"
	AnchorConfigApply         BehaviorAnchorKind = "config_apply"
	AnchorRegistryWrite       BehaviorAnchorKind = "registry_write"
	AnchorRegistryLookup      BehaviorAnchorKind = "registry_lookup"
	AnchorLifecycleInterface  BehaviorAnchorKind = "lifecycle_interface"
	AnchorLifecycleStart      BehaviorAnchorKind = "lifecycle_start"
	AnchorAdminControlPlane   BehaviorAnchorKind = "admin_control_plane"
	AnchorRequestDispatchRoot BehaviorAnchorKind = "request_dispatch_root"
	AnchorApplicationData     BehaviorAnchorKind = "application_data_plane"
	AnchorSecurityBoundary    BehaviorAnchorKind = "tls_or_security_boundary"
	AnchorExtensionFamily     BehaviorAnchorKind = "extension_family"
	AnchorUnresolvedFrontier  BehaviorAnchorKind = "unresolved_frontier"
)

func (kind BehaviorAnchorKind) valid() bool {
	switch kind {
	case AnchorProcessEntry, AnchorCommandDispatch, AnchorConfigIngress, AnchorConfigAdapter,
		AnchorConfigApply, AnchorRegistryWrite, AnchorRegistryLookup, AnchorLifecycleInterface,
		AnchorLifecycleStart, AnchorAdminControlPlane, AnchorRequestDispatchRoot,
		AnchorApplicationData, AnchorSecurityBoundary, AnchorExtensionFamily,
		AnchorUnresolvedFrontier:
		return true
	default:
		return false
	}
}

// BehaviorAnchor is exact local architecture evidence. The provider may group
// or name anchors by ID but cannot create or alter them.
type BehaviorAnchor struct {
	ID          string              `json:"id"`
	Kind        BehaviorAnchorKind  `json:"kind"`
	Label       string              `json:"label"`
	Location    evidence.Location   `json:"location"`
	Scenario    ScenarioContext     `json:"scenario"`
	Producer    evidence.Provenance `json:"producer"`
	Certainty   evidence.Certainty  `json:"certainty"`
	MemberIDs   []MemberID          `json:"member_ids"`
	Limitations []string            `json:"limitations"`
}

// FactKind is a small vocabulary for locally extracted candidate facts. Facts
// remain evidence; they are not component relations or temporal ordering.
type FactKind string

const (
	FactDeclaration       FactKind = "declaration"
	FactContainment       FactKind = "containment"
	FactFlowParticipation FactKind = "flow_participation"
	FactRepositoryPath    FactKind = "repository_path"
	FactExecutableRole    FactKind = "executable_role"
)

func (kind FactKind) valid() bool {
	switch kind {
	case FactDeclaration, FactContainment, FactFlowParticipation, FactRepositoryPath, FactExecutableRole:
		return true
	default:
		return false
	}
}

// LocalFact is copied unchanged into accepted components. Conceptual synthesis
// cannot strengthen its certainty or replace its provenance.
type LocalFact struct {
	Kind       FactKind              `json:"kind"`
	Value      string                `json:"value"`
	Location   *evidence.Location    `json:"location,omitempty"`
	Certainty  evidence.Certainty    `json:"certainty"`
	Provenance []evidence.Provenance `json:"provenance"`
}

// FlowParticipation ties a candidate to an exact saved flow through a local
// fact. A bare flow ID is deliberately insufficient for highlighting.
type FlowParticipation struct {
	FlowID   FlowID    `json:"flow_id"`
	Evidence LocalFact `json:"evidence"`
}

// Candidate is one exact, locally known landscape member. ParentID and
// Participations are bounded grouping inputs, not inferred architectural edges.
type Candidate struct {
	ID             MemberID            `json:"id"`
	Name           string              `json:"name"`
	ParentID       *MemberID           `json:"parent_id,omitempty"`
	Participations []FlowParticipation `json:"flow_participations,omitempty"`
	Facts          []LocalFact         `json:"facts"`
}

// Flow records the exact local flow identity used by candidate participation.
type Flow struct {
	ID    FlowID      `json:"id"`
	Name  string      `json:"name"`
	Facts []LocalFact `json:"facts"`
}

// StructuralRelationKind is intentionally smaller than the runtime relation
// vocabulary. Flow transitions remain owned by FlowProof.
type StructuralRelationKind string

const (
	StructuralRelationPackageImport   StructuralRelationKind = "package_import"
	StructuralRelationBehaviorHandoff StructuralRelationKind = "behavior_handoff"
)

func (kind StructuralRelationKind) valid() bool {
	return kind == StructuralRelationPackageImport || kind == StructuralRelationBehaviorHandoff
}

// ScenarioContext is the non-secret build context retained with a local
// structural witness. Environment values are intentionally excluded.
type ScenarioContext struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Build evidence.BuildContext `json:"build,omitempty"`
}

// LocalRelation is one component-specific structural witness between exact
// local members. Conceptual synthesis receives it but cannot modify it.
type LocalRelation struct {
	ID         string                 `json:"id"`
	From       MemberID               `json:"from"`
	To         MemberID               `json:"to"`
	Kind       StructuralRelationKind `json:"kind"`
	Location   *evidence.Location     `json:"location,omitempty"`
	Certainty  evidence.Certainty     `json:"certainty"`
	Provenance []evidence.Provenance  `json:"provenance"`
	Scenarios  []ScenarioContext      `json:"scenarios,omitempty"`
}

// FlowAnchorBinding is the exact typed join from a saved FlowProof anchor to
// one local landscape member. Presentation must not replace it with path
// coincidence.
type FlowAnchorBinding struct {
	FlowID     FlowID                `json:"flow_id"`
	AnchorID   string                `json:"anchor_id"`
	MemberID   MemberID              `json:"member_id"`
	Location   *evidence.Location    `json:"location,omitempty"`
	Certainty  evidence.Certainty    `json:"certainty"`
	Provenance []evidence.Provenance `json:"provenance"`
	Scenarios  []ScenarioContext     `json:"scenarios,omitempty"`
}

// ResearchInterpretation is a validated model interpretation over exact local
// evidence. It may guide conceptual naming/grouping but is not a LocalFact and
// cannot create members, anchors, flows, or relations.
type ResearchInterpretation struct {
	ID             string     `json:"id"`
	Question       string     `json:"question"`
	Interpretation string     `json:"interpretation"`
	EvidenceIDs    []string   `json:"evidence_ids"`
	MemberIDs      []MemberID `json:"member_ids,omitempty"`
	FlowIDs        []FlowID   `json:"flow_ids,omitempty"`
	AnchorIDs      []string   `json:"anchor_ids,omitempty"`
}

// CandidateBundle is the bounded, versioned input to conceptual synthesis.
// It contains no coordinates and gives a proposal no authority over evidence.
type CandidateBundle struct {
	Version               int                      `json:"version"`
	RepositoryArchetype   RepositoryArchetype      `json:"repository_archetype"`
	GroundingMode         GroundingMode            `json:"grounding_mode"`
	BehaviorAnchors       []BehaviorAnchor         `json:"behavior_anchors,omitempty"`
	Candidates            []Candidate              `json:"candidates"`
	Flows                 []Flow                   `json:"flows,omitempty"`
	Relations             []LocalRelation          `json:"relations,omitempty"`
	AnchorBindings        []FlowAnchorBinding      `json:"flow_anchor_bindings,omitempty"`
	ResearchFindings      []ResearchInterpretation `json:"accepted_research_findings,omitempty"`
	ResearchPolicyVersion string                   `json:"research_policy_version,omitempty"`
}

// Proposal contains the complete provider authority: wording, membership, and
// list order. Nested slice order is the proposed conceptual ordering.
type Proposal struct {
	Version    int                 `json:"version"`
	Subsystems []ProposedSubsystem `json:"subsystems"`
}

type ProposedSubsystem struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Components  []ProposedComponent `json:"components"`
	sourceIDs   []SubsystemID
}

type ProposedComponent struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	MemberIDs   []MemberID `json:"member_ids"`
	AnchorIDs   []string   `json:"anchor_ids,omitempty"`
	Hypothesis  bool       `json:"hypothesis,omitempty"`
	sourceIDs   []ComponentID
}

type ComponentID string

type SubsystemID string

// ConceptualMembership is the canonical many-to-many relation produced by
// validated conceptual synthesis. Component.Members is a materialized
// compatibility projection of this relation over the exact local bundle.
type ConceptualMembership struct {
	ComponentID ComponentID `json:"component_id"`
	MemberID    MemberID    `json:"member_id"`
}

// Component contains exact candidates reconstructed from the local bundle.
// Its ID is independent of proposal wording and ordering.
type Component struct {
	ID          ComponentID   `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Members     []Candidate   `json:"members"`
	AnchorIDs   []string      `json:"anchor_ids,omitempty"`
	Hypothesis  bool          `json:"hypothesis,omitempty"`
	SourceIDs   []ComponentID `json:"source_component_ids,omitempty"`
}

type SubsystemCategory string

const (
	SubsystemCategoryDiagnostic SubsystemCategory = "diagnostic"
)

type Subsystem struct {
	ID          SubsystemID       `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Category    SubsystemCategory `json:"category,omitempty"`
	Components  []Component       `json:"components"`
	SourceIDs   []SubsystemID     `json:"source_subsystem_ids,omitempty"`
}

type Diagnostic struct {
	Code     string          `json:"code"`
	Severity FindingSeverity `json:"severity"`
	Message  string          `json:"message"`
	Member   *MemberID       `json:"member,omitempty"`
}

type FindingSeverity string

const (
	FindingFatal       FindingSeverity = "fatal"
	FindingRecoverable FindingSeverity = "recoverable"
	FindingAdvisory    FindingSeverity = "advisory"
)

type ValidationOutcome string

const (
	ValidationAccepted           ValidationOutcome = "accepted"
	ValidationAcceptedNormalized ValidationOutcome = "accepted_with_normalization"
	ValidationRejected           ValidationOutcome = "rejected"
)

type ArchitectureSource string

const (
	SourceValidatedModel  ArchitectureSource = "validated_model"
	SourceNormalizedModel ArchitectureSource = "normalized_model"
	SourceLocalAnchors    ArchitectureSource = "local_anchors"
	SourceLocalPackages   ArchitectureSource = "local_packages"
	SourcePackageFallback ArchitectureSource = "package_fallback"
)

type NormalizationOperation struct {
	Code               string        `json:"code"`
	Message            string        `json:"message"`
	SourceSubsystemIDs []SubsystemID `json:"source_subsystem_ids,omitempty"`
	SourceComponentIDs []ComponentID `json:"source_component_ids,omitempty"`
}

type FallbackReason string

const (
	FallbackProposalInvalid       FallbackReason = "proposal_invalid_or_empty"
	FallbackRejectedMalformed     FallbackReason = "rejected_malformed_schema"
	FallbackRejectedUnknownMember FallbackReason = "rejected_unknown_member_id"
	FallbackRejectedUnknownAnchor FallbackReason = "rejected_unknown_anchor_id"
	FallbackRejectedOwnership     FallbackReason = "rejected_conflicting_membership"
	FallbackRejectedUngrounded    FallbackReason = "rejected_ungrounded_components"
	FallbackAnchorFirst           FallbackReason = "anchor_first_fallback"
	FallbackInsufficientAnchors   FallbackReason = "insufficient_behavior_anchors"
	FallbackPackageLandscape      FallbackReason = "package_landscape_fallback"
	FallbackModelDisabled         FallbackReason = "model_disabled"
	FallbackProviderUnconfigured  FallbackReason = "provider_not_configured"
)

// Landscape is the locally validated conceptual membership result. Fallback
// is explicit so presentation never mistakes deterministic grouping for a
// provider-authored architecture claim.
type Landscape struct {
	Version                int                      `json:"version"`
	Subsystems             []Subsystem              `json:"subsystems"`
	ConceptualMemberships  []ConceptualMembership   `json:"conceptual_memberships"`
	Relations              []LocalRelation          `json:"relations,omitempty"`
	AnchorBindings         []FlowAnchorBinding      `json:"flow_anchor_bindings,omitempty"`
	Diagnostics            []Diagnostic             `json:"diagnostics,omitempty"`
	ValidationOutcome      ValidationOutcome        `json:"validation_outcome"`
	Source                 ArchitectureSource       `json:"architecture_source"`
	Level                  int                      `json:"architecture_level"`
	Normalizations         []NormalizationOperation `json:"normalization_operations,omitempty"`
	OriginalProposalSHA256 string                   `json:"original_proposal_sha256,omitempty"`
	Fallback               bool                     `json:"fallback"`
	FallbackReason         FallbackReason           `json:"fallback_reason,omitempty"`
}

// Apply validates a proposal against exact local candidates. Evidence-integrity
// failures reject the proposal; recoverable hierarchy shape is normalized
// locally before the deterministic fallback ladder is considered.
func Apply(bundle CandidateBundle, proposal Proposal) (Landscape, error) {
	if err := bundle.Validate(); err != nil {
		return Landscape{}, err
	}

	var (
		landscape   Landscape
		diagnostics []Diagnostic
		operations  []NormalizationOperation
		usable      bool
	)
	if rawDiagnostics := proposalMembershipDiagnostics(bundle, proposal); len(rawDiagnostics) > 0 {
		// Membership cardinality belongs to the exact resolved response. Check it
		// before hierarchy normalization can merge components and deduplicate a
		// cross-cutting member into an apparently bounded relation.
		diagnostics = rawDiagnostics
	} else {
		normalized, shapeOperations, shapeDiagnostics := normalizeProposalShape(bundle, proposal)
		operations = shapeOperations
		landscape, diagnostics, usable = applyProposal(bundle, normalized)
		diagnostics = append(shapeDiagnostics, diagnostics...)
	}
	if !usable && !hasFatalDiagnostics(diagnostics) {
		return Landscape{}, fmt.Errorf("componentmap: rejected proposal has no fatal diagnostic")
	}
	if !usable {
		landscape = buildDeterministicLocalLandscape(bundle, SourcePackageFallback)
		landscape.Diagnostics = diagnostics
		landscape.ValidationOutcome = ValidationRejected
		landscape.FallbackReason = fallbackReasonForDiagnostics(diagnostics, len(bundle.BehaviorAnchors) > 0)
		landscape.Fallback = true
	} else if len(operations) > 0 {
		landscape.ValidationOutcome = ValidationAcceptedNormalized
		landscape.Source = SourceNormalizedModel
		landscape.Level = 2
		landscape.Normalizations = operations
		landscape.Diagnostics = diagnostics
	} else {
		landscape.ValidationOutcome = ValidationAccepted
		landscape.Source = SourceValidatedModel
		landscape.Level = 1
		landscape.Diagnostics = diagnostics
	}
	landscape.OriginalProposalSHA256 = proposalSHA256(proposal)
	if err := landscape.Validate(bundle); err != nil {
		return Landscape{}, err
	}
	return landscape, nil
}

// Deterministic builds a usable landscape without treating an intentionally
// absent provider as malformed provider output.
func Deterministic(bundle CandidateBundle, reason FallbackReason) (Landscape, error) {
	if err := bundle.Validate(); err != nil {
		return Landscape{}, err
	}
	if reason != FallbackModelDisabled && reason != FallbackProviderUnconfigured &&
		reason != FallbackAnchorFirst && reason != FallbackInsufficientAnchors && reason != FallbackPackageLandscape {
		return Landscape{}, fmt.Errorf("componentmap: invalid deterministic fallback reason %q", reason)
	}
	landscape := buildDeterministicLocalLandscape(bundle, SourcePackageFallback)
	landscape.Fallback = true
	landscape.FallbackReason = reason
	landscape.ValidationOutcome = ValidationAccepted
	if err := landscape.Validate(bundle); err != nil {
		return Landscape{}, err
	}
	return landscape, nil
}

// Canonical builds the deterministic local landscape that exists independently
// of optional conceptual synthesis. It is primary local evidence, not a
// provider fallback.
func Canonical(bundle CandidateBundle) (Landscape, error) {
	if err := bundle.Validate(); err != nil {
		return Landscape{}, err
	}
	landscape := buildDeterministicLocalLandscape(bundle, SourceLocalPackages)
	landscape.ValidationOutcome = ValidationAccepted
	if err := landscape.Validate(bundle); err != nil {
		return Landscape{}, err
	}
	return landscape, nil
}

func (bundle CandidateBundle) Validate() error {
	if bundle.Version != ContractVersion {
		return fmt.Errorf("componentmap: unsupported candidate bundle version %d", bundle.Version)
	}
	if len(bundle.Candidates) == 0 {
		return fmt.Errorf("componentmap: candidate bundle is empty")
	}
	if len(bundle.Candidates) > maxCandidates {
		return fmt.Errorf("componentmap: candidate bundle exceeds %d candidates", maxCandidates)
	}
	if len(bundle.Flows) > maxFlows {
		return fmt.Errorf("componentmap: candidate bundle exceeds %d flows", maxFlows)
	}
	if len(bundle.Relations) > maxRelations {
		return fmt.Errorf("componentmap: candidate bundle exceeds %d structural relations", maxRelations)
	}
	if len(bundle.AnchorBindings) > maxAnchorBindings {
		return fmt.Errorf("componentmap: candidate bundle exceeds %d flow-anchor bindings", maxAnchorBindings)
	}
	if len(bundle.ResearchFindings) > maxResearchFindings {
		return fmt.Errorf("componentmap: candidate bundle exceeds %d research findings", maxResearchFindings)
	}
	if len(bundle.ResearchFindings) > 0 && strings.TrimSpace(bundle.ResearchPolicyVersion) == "" {
		return fmt.Errorf("componentmap: research findings require a policy version")
	}
	if !bundle.RepositoryArchetype.valid() {
		return fmt.Errorf("componentmap: invalid repository archetype %q", bundle.RepositoryArchetype)
	}
	if !bundle.GroundingMode.valid() {
		return fmt.Errorf("componentmap: invalid grounding mode %q", bundle.GroundingMode)
	}
	if len(bundle.BehaviorAnchors) > maxBehaviorAnchors {
		return fmt.Errorf("componentmap: candidate bundle exceeds %d behavior anchors", maxBehaviorAnchors)
	}
	if bundle.GroundingMode != GroundingPackages && len(bundle.BehaviorAnchors) == 0 {
		return fmt.Errorf("componentmap: grounded architecture has no behavior anchors")
	}

	flowIDs := make(map[FlowID]struct{}, len(bundle.Flows))
	for index, flow := range bundle.Flows {
		if err := validateFlow(flow); err != nil {
			return fmt.Errorf("componentmap: flows[%d]: %w", index, err)
		}
		if _, exists := flowIDs[flow.ID]; exists {
			return fmt.Errorf("componentmap: duplicate flow id %q", flow.ID)
		}
		flowIDs[flow.ID] = struct{}{}
	}

	members := make(map[MemberID]Candidate, len(bundle.Candidates))
	for index, candidate := range bundle.Candidates {
		if err := validateCandidate(candidate); err != nil {
			return fmt.Errorf("componentmap: candidates[%d]: %w", index, err)
		}
		if _, exists := members[candidate.ID]; exists {
			return fmt.Errorf("componentmap: duplicate member id %q", candidate.ID.key())
		}
		members[candidate.ID] = candidate
	}
	for _, candidate := range bundle.Candidates {
		if candidate.ParentID != nil {
			if *candidate.ParentID == candidate.ID {
				return fmt.Errorf("componentmap: member %q is its own parent", candidate.ID.key())
			}
			if _, exists := members[*candidate.ParentID]; !exists {
				return fmt.Errorf("componentmap: member %q has unknown parent", candidate.ID.key())
			}
		}
		for _, participation := range candidate.Participations {
			if _, exists := flowIDs[participation.FlowID]; !exists {
				return fmt.Errorf("componentmap: member %q references unknown flow %q", candidate.ID.key(), participation.FlowID)
			}
		}
		if candidate.ID.Kind == MemberFlow && len(candidate.Participations) != 1 {
			return fmt.Errorf("componentmap: flow member %q must reference exactly one flow", candidate.ID.key())
		}
	}
	anchorIDs := make(map[string]struct{}, len(bundle.BehaviorAnchors))
	for index, anchor := range bundle.BehaviorAnchors {
		if err := validateBehaviorAnchor(anchor, members); err != nil {
			return fmt.Errorf("componentmap: behavior_anchors[%d]: %w", index, err)
		}
		if _, duplicate := anchorIDs[anchor.ID]; duplicate {
			return fmt.Errorf("componentmap: duplicate behavior anchor id %q", anchor.ID)
		}
		anchorIDs[anchor.ID] = struct{}{}
	}
	if err := validateParentCycles(bundle.Candidates, members); err != nil {
		return err
	}
	relationIDs := make(map[string]struct{}, len(bundle.Relations))
	relationWitnesses := make(map[string]struct{}, len(bundle.Relations))
	scenarioDefinitions := make(map[string]ScenarioContext)
	for index, relation := range bundle.Relations {
		if err := validateLocalRelation(relation, members); err != nil {
			return fmt.Errorf("componentmap: relations[%d]: %w", index, err)
		}
		if _, duplicate := relationIDs[relation.ID]; duplicate {
			return fmt.Errorf("componentmap: duplicate structural relation id %q", relation.ID)
		}
		relationIDs[relation.ID] = struct{}{}
		witnessKey := relation.From.key() + "\x00" + relation.To.key() + "\x00" + string(relation.Kind)
		if _, duplicate := relationWitnesses[witnessKey]; duplicate {
			return fmt.Errorf("componentmap: duplicate structural relation witness")
		}
		relationWitnesses[witnessKey] = struct{}{}
		for _, scenario := range relation.Scenarios {
			if previous, exists := scenarioDefinitions[scenario.ID]; exists && !reflect.DeepEqual(previous, scenario) {
				return fmt.Errorf("componentmap: scenario %q has conflicting definitions", scenario.ID)
			}
			scenarioDefinitions[scenario.ID] = scenario
		}
	}
	anchorBindings := make(map[string]struct{}, len(bundle.AnchorBindings))
	for index, binding := range bundle.AnchorBindings {
		if err := validateFlowAnchorBinding(binding, members, flowIDs); err != nil {
			return fmt.Errorf("componentmap: flow_anchor_bindings[%d]: %w", index, err)
		}
		key := string(binding.FlowID) + "\x00" + binding.AnchorID
		if _, duplicate := anchorBindings[key]; duplicate {
			return fmt.Errorf("componentmap: duplicate binding for flow anchor")
		}
		anchorBindings[key] = struct{}{}
		for _, scenario := range binding.Scenarios {
			if previous, exists := scenarioDefinitions[scenario.ID]; exists && !reflect.DeepEqual(previous, scenario) {
				return fmt.Errorf("componentmap: scenario %q has conflicting definitions", scenario.ID)
			}
			scenarioDefinitions[scenario.ID] = scenario
		}
	}
	researchIDs := make(map[string]struct{}, len(bundle.ResearchFindings))
	for index, finding := range bundle.ResearchFindings {
		if err := validateResearchInterpretation(finding, members, flowIDs, anchorIDs); err != nil {
			return fmt.Errorf("componentmap: accepted_research_findings[%d]: %w", index, err)
		}
		if _, duplicate := researchIDs[finding.ID]; duplicate {
			return fmt.Errorf("componentmap: duplicate research finding id %q", finding.ID)
		}
		researchIDs[finding.ID] = struct{}{}
	}
	return nil
}

func validateResearchInterpretation(
	finding ResearchInterpretation,
	members map[MemberID]Candidate,
	flows map[FlowID]struct{},
	anchors map[string]struct{},
) error {
	if strings.TrimSpace(finding.ID) == "" || strings.TrimSpace(finding.Question) == "" ||
		strings.TrimSpace(finding.Interpretation) == "" || len(finding.EvidenceIDs) == 0 {
		return fmt.Errorf("research interpretation is incomplete")
	}
	if len(finding.MemberIDs) == 0 && len(finding.FlowIDs) == 0 && len(finding.AnchorIDs) == 0 {
		return fmt.Errorf("research interpretation has no exact local binding")
	}
	for _, memberID := range finding.MemberIDs {
		if _, ok := members[memberID]; !ok {
			return fmt.Errorf("research interpretation references unknown member")
		}
	}
	for _, flowID := range finding.FlowIDs {
		if _, ok := flows[flowID]; !ok {
			return fmt.Errorf("research interpretation references unknown flow %q", flowID)
		}
	}
	for _, anchorID := range finding.AnchorIDs {
		if _, ok := anchors[anchorID]; !ok {
			return fmt.Errorf("research interpretation references unknown anchor %q", anchorID)
		}
	}
	return nil
}

// Validate proves that every accepted member is an unchanged local candidate,
// membership is unique, and IDs were derived from the current contract.
func (landscape Landscape) Validate(bundle CandidateBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if landscape.Version != ContractVersion {
		return fmt.Errorf("componentmap: unsupported landscape version %d", landscape.Version)
	}
	if landscape.Fallback && !validFallbackReason(landscape.FallbackReason) {
		return fmt.Errorf("componentmap: fallback landscape has unsupported or missing reason")
	}
	if !landscape.Fallback && landscape.FallbackReason != "" {
		return fmt.Errorf("componentmap: non-fallback landscape carries a fallback reason")
	}
	if !validValidationOutcome(landscape.ValidationOutcome) {
		return fmt.Errorf("componentmap: invalid validation outcome %q", landscape.ValidationOutcome)
	}
	if !validArchitectureSource(landscape.Source) || landscape.Level < 1 || landscape.Level > 4 {
		return fmt.Errorf("componentmap: invalid architecture source or level")
	}
	if landscape.Source == SourceValidatedModel && (landscape.Level != 1 || landscape.Fallback || len(landscape.Normalizations) != 0) {
		return fmt.Errorf("componentmap: validated model source has inconsistent state")
	}
	if landscape.Source == SourceNormalizedModel && (landscape.Level != 2 || landscape.Fallback || len(landscape.Normalizations) == 0) {
		return fmt.Errorf("componentmap: normalized model source has inconsistent state")
	}
	if landscape.Source == SourceLocalAnchors && landscape.Level != 3 {
		return fmt.Errorf("componentmap: local-anchor source has inconsistent level")
	}
	if landscape.Source == SourceLocalPackages && landscape.Level != 4 {
		return fmt.Errorf("componentmap: local-package source has inconsistent level")
	}
	if landscape.Source == SourcePackageFallback && landscape.Level != 4 {
		return fmt.Errorf("componentmap: package source has inconsistent level")
	}
	if landscape.OriginalProposalSHA256 != "" && len(landscape.OriginalProposalSHA256) != 64 {
		return fmt.Errorf("componentmap: original proposal digest is malformed")
	}
	if !reflect.DeepEqual(landscape.Relations, bundle.Relations) {
		return fmt.Errorf("componentmap: landscape changed local structural relations")
	}
	if !reflect.DeepEqual(landscape.AnchorBindings, bundle.AnchorBindings) {
		return fmt.Errorf("componentmap: landscape changed local flow-anchor bindings")
	}
	for index, diagnostic := range landscape.Diagnostics {
		if err := validateDiagnostic(diagnostic); err != nil {
			return fmt.Errorf("componentmap: diagnostics[%d]: %w", index, err)
		}
	}
	for index, operation := range landscape.Normalizations {
		if err := validateNormalizationOperation(operation); err != nil {
			return fmt.Errorf("componentmap: normalization_operations[%d]: %w", index, err)
		}
	}
	if len(landscape.Subsystems) == 0 || len(landscape.Subsystems) > maxSubsystems {
		return fmt.Errorf("componentmap: landscape subsystem count is out of bounds")
	}

	known := candidateIndex(bundle)
	knownAnchors := make(map[string]struct{}, len(bundle.BehaviorAnchors))
	for _, anchor := range bundle.BehaviorAnchors {
		knownAnchors[anchor.ID] = struct{}{}
	}
	seenMembers := make(map[MemberID]struct{}, len(bundle.Candidates))
	seenComponents := make(map[ComponentID]struct{})
	componentCount := 0
	for subsystemIndex, subsystem := range landscape.Subsystems {
		if subsystem.Category != "" && subsystem.Category != SubsystemCategoryDiagnostic {
			return fmt.Errorf("componentmap: subsystem[%d] has unsupported category %q", subsystemIndex, subsystem.Category)
		}
		if err := validateDisplayText("subsystem name", subsystem.Name, maxNameBytes, true); err != nil {
			return err
		}
		if err := validateDisplayText("subsystem description", subsystem.Description, maxDescriptionBytes, false); err != nil {
			return err
		}
		if len(subsystem.Components) == 0 {
			return fmt.Errorf("componentmap: subsystem[%d] has no components", subsystemIndex)
		}
		componentIDs := make([]ComponentID, 0, len(subsystem.Components))
		for componentIndex, component := range subsystem.Components {
			componentCount++
			if err := validateDisplayText("component name", component.Name, maxNameBytes, true); err != nil {
				return err
			}
			if err := validateDisplayText("component description", component.Description, maxDescriptionBytes, false); err != nil {
				return err
			}
			if len(component.Members) == 0 {
				return fmt.Errorf("componentmap: subsystem[%d].components[%d] has no members", subsystemIndex, componentIndex)
			}
			if len(component.AnchorIDs) > maxAnchorMembers {
				return fmt.Errorf("componentmap: component has too many behavior anchors")
			}
			seenAnchorIDs := make(map[string]struct{}, len(component.AnchorIDs))
			for _, anchorID := range component.AnchorIDs {
				if _, exists := knownAnchors[anchorID]; !exists {
					return fmt.Errorf("componentmap: component references unknown behavior anchor %q", anchorID)
				}
				if _, duplicate := seenAnchorIDs[anchorID]; duplicate {
					return fmt.Errorf("componentmap: component repeats behavior anchor %q", anchorID)
				}
				seenAnchorIDs[anchorID] = struct{}{}
			}
			if bundle.GroundingMode != GroundingPackages && subsystem.Category != SubsystemCategoryDiagnostic &&
				len(component.AnchorIDs) == 0 && !component.Hypothesis {
				return fmt.Errorf("componentmap: grounded component lacks an anchor or explicit hypothesis")
			}
			memberIDs := make([]MemberID, 0, len(component.Members))
			seenComponentMembers := make(map[MemberID]struct{}, len(component.Members))
			for _, member := range component.Members {
				exact, exists := known[member.ID]
				if !exists {
					return fmt.Errorf("componentmap: component references unknown member %q", member.ID.key())
				}
				if !reflect.DeepEqual(member, exact) {
					return fmt.Errorf("componentmap: component changed local member %q", member.ID.key())
				}
				if _, exists := seenComponentMembers[member.ID]; exists {
					return fmt.Errorf("componentmap: component repeats membership for %q", member.ID.key())
				}
				seenComponentMembers[member.ID] = struct{}{}
				seenMembers[member.ID] = struct{}{}
				memberIDs = append(memberIDs, member.ID)
			}
			if expected := componentID(memberIDs); component.ID != expected {
				return fmt.Errorf("componentmap: component id %q does not match exact membership", component.ID)
			}
			if _, exists := seenComponents[component.ID]; exists {
				return fmt.Errorf("componentmap: duplicate component id %q", component.ID)
			}
			seenComponents[component.ID] = struct{}{}
			componentIDs = append(componentIDs, component.ID)
		}
		if expected := subsystemID(componentIDs); subsystem.ID != expected {
			return fmt.Errorf("componentmap: subsystem id %q does not match exact components", subsystem.ID)
		}
	}
	if componentCount > maxComponents {
		return fmt.Errorf("componentmap: landscape exceeds %d components", maxComponents)
	}
	expectedMemberships := conceptualMembershipsFromSubsystems(landscape.Subsystems)
	if len(expectedMemberships) > maxConceptualMemberships {
		return fmt.Errorf("componentmap: landscape exceeds %d conceptual memberships", maxConceptualMemberships)
	}
	perMember := make(map[MemberID]int, len(seenMembers))
	for _, membership := range expectedMemberships {
		perMember[membership.MemberID]++
		if perMember[membership.MemberID] > maxConceptualMembershipsPerMember {
			return fmt.Errorf(
				"componentmap: member %q exceeds %d conceptual memberships",
				membership.MemberID.key(), maxConceptualMembershipsPerMember,
			)
		}
	}
	if !reflect.DeepEqual(landscape.ConceptualMemberships, expectedMemberships) {
		return fmt.Errorf("componentmap: conceptual membership relation does not match component projection")
	}
	if len(seenMembers) != len(bundle.Candidates) {
		return fmt.Errorf(
			"componentmap: landscape covers %d of %d local candidates",
			len(seenMembers), len(bundle.Candidates),
		)
	}
	return nil
}

func applyProposal(bundle CandidateBundle, proposal Proposal) (Landscape, []Diagnostic, bool) {
	diagnostics := make([]Diagnostic, 0)
	invalid := func(code, message string) {
		diagnostics = append(diagnostics, newDiagnostic(code, message))
	}
	if proposal.Version != ProposalVersion {
		invalid("proposal.unsupported_version", "proposal version is missing or unsupported")
		return Landscape{}, diagnostics, false
	}
	if len(proposal.Subsystems) == 0 {
		invalid("proposal.invalid_subsystem_count", "proposal has no subsystems")
		return Landscape{}, diagnostics, false
	}
	if membershipDiagnostics := proposalMembershipDiagnostics(bundle, proposal); len(membershipDiagnostics) > 0 {
		diagnostics = append(diagnostics, membershipDiagnostics...)
		return Landscape{}, diagnostics, false
	}
	proposedSubsystems := proposal.Subsystems

	known := candidateIndex(bundle)
	knownAnchors := make(map[string]struct{}, len(bundle.BehaviorAnchors))
	for _, anchor := range bundle.BehaviorAnchors {
		knownAnchors[anchor.ID] = struct{}{}
	}
	seenMembers := make(map[MemberID]struct{})
	seenComponentIDs := make(map[ComponentID]struct{})
	landscape := Landscape{
		Version:        ContractVersion,
		Subsystems:     make([]Subsystem, 0, len(proposal.Subsystems)),
		Relations:      cloneLocalRelations(bundle.Relations),
		AnchorBindings: cloneFlowAnchorBindings(bundle.AnchorBindings),
	}
	componentCount := 0
	componentBudget := maxComponents - 1
	for _, proposedSubsystem := range proposedSubsystems {
		if componentBudget == 0 {
			break
		}
		name := strings.TrimSpace(proposedSubsystem.Name)
		description := strings.TrimSpace(proposedSubsystem.Description)
		if validateDisplayText("subsystem name", name, maxNameBytes, true) != nil ||
			validateDisplayText("subsystem description", description, maxDescriptionBytes, false) != nil ||
			len(proposedSubsystem.Components) == 0 {
			invalid("proposal.invalid_subsystem", "proposal contains an empty or malformed subsystem")
			return Landscape{}, diagnostics, false
		}
		subsystem := Subsystem{Name: name, Description: description, Components: make([]Component, 0, len(proposedSubsystem.Components))}
		for _, proposedComponent := range proposedSubsystem.Components {
			if componentBudget == 0 {
				break
			}
			componentBudget--
			componentName := strings.TrimSpace(proposedComponent.Name)
			componentDescription := strings.TrimSpace(proposedComponent.Description)
			if validateDisplayText("component name", componentName, maxNameBytes, true) != nil ||
				validateDisplayText("component description", componentDescription, maxDescriptionBytes, false) != nil ||
				len(proposedComponent.MemberIDs) == 0 {
				invalid("proposal.invalid_component", "proposal contains an empty or malformed component")
				return Landscape{}, diagnostics, false
			}
			members := make([]Candidate, 0, len(proposedComponent.MemberIDs))
			for _, memberID := range proposedComponent.MemberIDs {
				candidate, exists := known[memberID]
				if !exists {
					invalid("proposal.unknown_member_id", "proposal references a member id absent from the local candidate bundle")
					return Landscape{}, diagnostics, false
				}
				seenMembers[memberID] = struct{}{}
				members = append(members, cloneCandidate(candidate))
			}
			if len(members) == 0 {
				invalid("proposal.invalid_component", "proposal component has no usable exact members")
				return Landscape{}, diagnostics, false
			}
			anchorIDs := make([]string, 0, len(proposedComponent.AnchorIDs))
			seenAnchorIDs := make(map[string]struct{}, len(proposedComponent.AnchorIDs))
			for _, anchorID := range proposedComponent.AnchorIDs {
				if _, exists := knownAnchors[anchorID]; !exists {
					invalid("proposal.unknown_anchor_id", "proposal references an anchor id absent from the local grounding bundle")
					return Landscape{}, diagnostics, false
				}
				if _, duplicate := seenAnchorIDs[anchorID]; duplicate {
					continue
				}
				seenAnchorIDs[anchorID] = struct{}{}
				anchorIDs = append(anchorIDs, anchorID)
			}
			sort.Strings(anchorIDs)
			if bundle.GroundingMode != GroundingPackages && len(anchorIDs) == 0 && !proposedComponent.Hypothesis {
				invalid("proposal.ungrounded_primary_component", "a primary component lacks a supplied behavior anchor or explicit hypothesis")
				return Landscape{}, diagnostics, false
			}
			sortCandidates(members)
			id := componentID(candidateIDs(members))
			if _, duplicate := seenComponentIDs[id]; duplicate {
				invalid("proposal.duplicate_component_identity", "proposal contains two components with the same exact member set")
				return Landscape{}, diagnostics, false
			}
			seenComponentIDs[id] = struct{}{}
			componentCount++
			subsystem.Components = append(subsystem.Components, Component{
				ID: id, Name: componentName, Description: componentDescription,
				Members: members, AnchorIDs: anchorIDs, Hypothesis: proposedComponent.Hypothesis,
				SourceIDs: append([]ComponentID(nil), proposedComponent.sourceIDs...),
			})
		}
		if len(subsystem.Components) == 0 {
			invalid("proposal.invalid_subsystem", "proposal subsystem has no usable nested components")
			return Landscape{}, diagnostics, false
		}
		subsystem.ID = subsystemID(componentIDs(subsystem.Components))
		subsystem.SourceIDs = append([]SubsystemID(nil), proposedSubsystem.sourceIDs...)
		landscape.Subsystems = append(landscape.Subsystems, subsystem)
	}
	if len(landscape.Subsystems) == 0 {
		invalid("proposal.no_usable_subsystems", "no proposed subsystem retained a unique known member")
		return Landscape{}, diagnostics, false
	}
	for _, anchor := range bundle.BehaviorAnchors {
		if anchor.Kind != AnchorProcessEntry {
			continue
		}
		for _, memberID := range anchor.MemberIDs {
			if _, included := seenMembers[memberID]; included {
				continue
			}
			invalid(
				"proposal.omitted_process_entry_member",
				"proposal omitted an exact process-entry member from the conceptual architecture",
			)
		}
	}
	if len(seenMembers) != len(bundle.Candidates) {
		missing := make([]Candidate, 0, len(bundle.Candidates)-len(seenMembers))
		for _, candidate := range bundle.Candidates {
			if _, included := seenMembers[candidate.ID]; included {
				continue
			}
			missing = append(missing, cloneCandidate(candidate))
		}
		sortCandidates(missing)
		if len(landscape.Subsystems) == maxSubsystems || componentCount == maxComponents {
			invalid("proposal.omitted_members_exceed_bounds", "proposal omitted local members and no bounded remainder fits")
			return Landscape{}, diagnostics, false
		}
		remainder := Component{
			ID:          componentID(candidateIDs(missing)),
			Name:        "Other locally known members",
			Description: "Exact local candidates omitted by conceptual synthesis; retained without inferred grouping.",
			Members:     missing,
		}
		landscape.Subsystems = append(landscape.Subsystems, Subsystem{
			ID:          subsystemID([]ComponentID{remainder.ID}),
			Name:        "Unassigned local evidence",
			Description: "Local evidence intentionally preserved outside the proposed conceptual groups.",
			Category:    SubsystemCategoryDiagnostic,
			Components:  []Component{remainder},
		})
		diagnostics = append(diagnostics, newDiagnostic(
			"proposal.omitted_members_preserved",
			"conceptual synthesis omitted local candidates; they remain visible in a deterministic remainder",
		))
	}
	landscape.Diagnostics = diagnostics
	landscape.ConceptualMemberships = conceptualMembershipsFromSubsystems(landscape.Subsystems)
	return landscape, diagnostics, true
}

// proposalMembershipDiagnostics validates the exact resolved response before
// any readable-shape normalization. It deliberately returns at most one fatal
// diagnostic so rejection remains closed and bounded.
func proposalMembershipDiagnostics(bundle CandidateBundle, proposal Proposal) []Diagnostic {
	if proposal.Version != ProposalVersion || len(proposal.Subsystems) == 0 {
		return nil
	}
	memberReferenceCount := 0
	memberReferenceCounts := make(map[MemberID]int)
	distinctReferencedMembers := make(map[MemberID]struct{})
	for _, subsystem := range proposal.Subsystems {
		for _, component := range subsystem.Components {
			if len(component.MemberIDs) > maxCandidates {
				return []Diagnostic{newDiagnostic(
					"proposal.invalid_members",
					"proposal membership exceeds the candidate limit",
				)}
			}
			memberReferenceCount += len(component.MemberIDs)
			if memberReferenceCount > maxConceptualMemberships {
				return []Diagnostic{newDiagnostic(
					"proposal.membership_limit_exceeded",
					"proposal exceeds the conceptual membership limit",
				)}
			}
			seenComponentMembers := make(map[MemberID]struct{}, len(component.MemberIDs))
			for _, memberID := range component.MemberIDs {
				if validateMemberID(memberID) != nil {
					return []Diagnostic{newDiagnostic(
						"proposal.invalid_member_id",
						"proposal contains a malformed member id",
					)}
				}
				if _, duplicate := seenComponentMembers[memberID]; duplicate {
					return []Diagnostic{newDiagnostic(
						"proposal.duplicate_member_id",
						"proposal repeats one exact member within a component",
					)}
				}
				seenComponentMembers[memberID] = struct{}{}
				distinctReferencedMembers[memberID] = struct{}{}
				memberReferenceCounts[memberID]++
				if memberReferenceCounts[memberID] > maxConceptualMembershipsPerMember {
					return []Diagnostic{newDiagnostic(
						"proposal.member_participation_limit_exceeded",
						"one exact member exceeds the conceptual participation limit",
					)}
				}
			}
		}
	}
	omittedMembers := 0
	for _, candidate := range bundle.Candidates {
		if _, included := distinctReferencedMembers[candidate.ID]; !included {
			omittedMembers++
		}
	}
	if memberReferenceCount+omittedMembers > maxConceptualMemberships {
		return []Diagnostic{newDiagnostic(
			"proposal.membership_limit_exceeded",
			"proposal plus the deterministic omitted-member remainder exceeds the conceptual membership limit",
		)}
	}
	return nil
}

type localGroup struct {
	key         string
	category    string
	name        string
	description string
	members     []Candidate
}

func buildDeterministicLocalLandscape(
	bundle CandidateBundle,
	packageSource ArchitectureSource,
) Landscape {
	var landscape Landscape
	if useAnchorFirstLocalGrouping(bundle) {
		landscape = anchorFirstLocalLandscape(bundle)
	} else {
		landscape = packageLocalLandscape(bundle, packageSource)
	}
	landscape.ConceptualMemberships = conceptualMembershipsFromSubsystems(landscape.Subsystems)
	return landscape
}

func packageLocalLandscape(bundle CandidateBundle, source ArchitectureSource) Landscape {
	known := candidateIndex(bundle)
	flowNames := make(map[FlowID]string, len(bundle.Flows))
	for _, flow := range bundle.Flows {
		flowNames[flow.ID] = flow.Name
	}

	groupsByKey := make(map[string]*localGroup)
	candidates := append([]Candidate(nil), bundle.Candidates...)
	sortCandidates(candidates)
	for _, candidate := range candidates {
		key, category, name := localGroupingBasis(candidate, known, flowNames)
		group := groupsByKey[key]
		if group == nil {
			group = &localGroup{
				key: key, category: category, name: name,
				description: "Deterministic grouping from exact local " + category + " candidates.",
			}
			groupsByKey[key] = group
		}
		group.members = append(group.members, cloneCandidate(candidate))
	}

	groups := make([]localGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		sortCandidates(group.members)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].key < groups[j].key })
	groupLimit := maxComponents
	if bundle.GroundingMode != GroundingPackages {
		groupLimit = 8
	}
	if len(groups) > groupLimit {
		kept := append([]localGroup(nil), groups[:groupLimit-1]...)
		remainder := localGroup{
			key: "zz:other", category: "diagnostic", name: "Other repository members",
			description: "Deterministic bounded remainder kept outside the primary architecture.",
		}
		for _, group := range groups[groupLimit-1:] {
			remainder.members = append(remainder.members, group.members...)
		}
		sortCandidates(remainder.members)
		groups = append(kept, remainder)
	}

	byCategory := make(map[string][]Component)
	for _, group := range groups {
		id := componentID(candidateIDs(group.members))
		byCategory[group.category] = append(byCategory[group.category], Component{
			ID: id, Name: group.name, Description: group.description,
			Members: group.members, AnchorIDs: behaviorAnchorsForMembers(bundle.BehaviorAnchors, group.members),
			Hypothesis: bundle.GroundingMode != GroundingPackages && len(behaviorAnchorsForMembers(bundle.BehaviorAnchors, group.members)) == 0,
			SourceIDs:  []ComponentID{id},
		})
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	subsystems := make([]Subsystem, 0, len(categories))
	for _, category := range categories {
		components := byCategory[category]
		sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
		name := localSubsystemName(category)
		subsystemCategory := SubsystemCategory("")
		if category == "diagnostic" {
			subsystemCategory = SubsystemCategoryDiagnostic
		}
		id := subsystemID(componentIDs(components))
		subsystems = append(subsystems, Subsystem{
			ID: id, Name: name,
			Description: "Deterministic local " + category + " landscape.",
			Category:    subsystemCategory, Components: components, SourceIDs: []SubsystemID{id},
		})
	}
	return Landscape{
		Version: ContractVersion, Subsystems: subsystems,
		Relations: cloneLocalRelations(bundle.Relations), AnchorBindings: cloneFlowAnchorBindings(bundle.AnchorBindings),
		Source: source, Level: 4,
	}
}

type anchorLocalGroup struct {
	name        string
	description string
	kinds       []BehaviorAnchorKind
}

func anchorFirstLocalLandscape(bundle CandidateBundle) Landscape {
	groups := []anchorLocalGroup{
		{name: "Entry and dispatch", description: "Process entry and command dispatch anchors.", kinds: []BehaviorAnchorKind{AnchorProcessEntry, AnchorCommandDispatch}},
		{name: "Configuration", description: "Configuration ingress, adaptation, and application anchors.", kinds: []BehaviorAnchorKind{AnchorConfigIngress, AnchorConfigAdapter, AnchorConfigApply}},
		{name: "Runtime and extensions", description: "Registry, extension, and lifecycle anchors.", kinds: []BehaviorAnchorKind{AnchorRegistryWrite, AnchorRegistryLookup, AnchorExtensionFamily, AnchorLifecycleInterface, AnchorLifecycleStart}},
		{name: "Control plane", description: "Administrative control-plane anchors.", kinds: []BehaviorAnchorKind{AnchorAdminControlPlane}},
		{name: "Request and data plane", description: "Request dispatch and application data-plane anchors.", kinds: []BehaviorAnchorKind{AnchorRequestDispatchRoot, AnchorApplicationData}},
		{name: "Security", description: "TLS and security-boundary anchors.", kinds: []BehaviorAnchorKind{AnchorSecurityBoundary}},
	}
	known := candidateIndex(bundle)
	anchorsByKind := make(map[BehaviorAnchorKind][]BehaviorAnchor)
	for _, anchor := range bundle.BehaviorAnchors {
		anchorsByKind[anchor.Kind] = append(anchorsByKind[anchor.Kind], anchor)
	}
	for kind := range anchorsByKind {
		sort.Slice(anchorsByKind[kind], func(i, j int) bool { return anchorsByKind[kind][i].ID < anchorsByKind[kind][j].ID })
	}

	owned := make(map[MemberID]struct{}, len(bundle.Candidates))
	subsystems := make([]Subsystem, 0, len(groups)+1)
	for _, group := range groups {
		components := make([]Component, 0, len(group.kinds))
		for _, kind := range group.kinds {
			anchors := anchorsByKind[kind]
			if len(anchors) == 0 {
				continue
			}
			if kind == AnchorProcessEntry {
				components = append(components, processEntryLocalComponents(anchors, known, owned)...)
				continue
			}
			memberSet := make(map[MemberID]struct{})
			anchorIDs := make([]string, 0, len(anchors))
			for _, anchor := range anchors {
				anchorIDs = append(anchorIDs, anchor.ID)
				for _, memberID := range anchor.MemberIDs {
					addAnchorLocalMember(memberID, known, owned, memberSet)
				}
			}
			members := candidatesFromIDSet(memberSet, known)
			if len(members) == 0 {
				continue
			}
			id := componentID(candidateIDs(members))
			components = append(components, Component{
				ID: id, Name: anchorLocalComponentName(kind),
				Description: "Deterministic grouping from exact " + string(kind) + " anchors.",
				Members:     members, AnchorIDs: anchorIDs, SourceIDs: []ComponentID{id},
			})
		}
		if len(components) == 0 {
			continue
		}
		id := subsystemID(componentIDs(components))
		subsystems = append(subsystems, Subsystem{
			ID: id, Name: group.name, Description: group.description,
			Components: components, SourceIDs: []SubsystemID{id},
		})
	}

	remainder := make([]Candidate, 0)
	for _, candidate := range bundle.Candidates {
		if _, exists := owned[candidate.ID]; !exists {
			remainder = append(remainder, cloneCandidate(candidate))
		}
	}
	if len(remainder) > 0 {
		sortCandidates(remainder)
		componentID := componentID(candidateIDs(remainder))
		component := Component{
			ID: componentID, Name: "Supporting repository evidence",
			Description: "Exact local members not assigned by the bounded anchor-kind mapping.",
			Members:     remainder, Hypothesis: true, SourceIDs: []ComponentID{componentID},
		}
		subsystemID := subsystemID([]ComponentID{componentID})
		subsystems = append(subsystems, Subsystem{
			ID: subsystemID, Name: "Supporting evidence",
			Description: "Package, file, symbol, and flow evidence retained outside the primary architecture.",
			Category:    SubsystemCategoryDiagnostic, Components: []Component{component},
			SourceIDs: []SubsystemID{subsystemID},
		})
	}
	return Landscape{
		Version: ContractVersion, Subsystems: subsystems,
		Relations: cloneLocalRelations(bundle.Relations), AnchorBindings: cloneFlowAnchorBindings(bundle.AnchorBindings),
		Source: SourceLocalAnchors, Level: 3,
	}
}

func processEntryLocalComponents(
	anchors []BehaviorAnchor,
	known map[MemberID]Candidate,
	owned map[MemberID]struct{},
) []Component {
	modulePrefix := processEntryModulePrefix(anchors, known)
	groups := []struct {
		name        string
		description string
		class       string
	}{
		{name: "Primary application", description: "Repository-named process entrypoint backed by an exact main declaration.", class: "primary"},
		{name: "Secondary services", description: "Repository service entrypoints distinct from the primary application.", class: "secondary_service"},
		{name: "Tool entrypoints", description: "Developer, build, release, and maintenance tool entrypoints.", class: "tooling"},
		{name: "Test and helper entrypoints", description: "Test, example, and helper process entrypoints.", class: "test_or_helper"},
		{name: "Other process entrypoints", description: "Exact process entrypoints whose product role remains unresolved.", class: "unknown"},
	}
	components := make([]Component, 0, len(groups))
	for _, group := range groups {
		matching := make([]BehaviorAnchor, 0)
		for _, anchor := range anchors {
			if processEntryLocalClass(anchor, known, modulePrefix) != group.class {
				continue
			}
			matching = append(matching, anchor)
		}
		for start := 0; start < len(matching); start += maxAnchorMembers {
			end := min(start+maxAnchorMembers, len(matching))
			memberSet := make(map[MemberID]struct{})
			anchorIDs := make([]string, 0, end-start)
			for _, anchor := range matching[start:end] {
				anchorIDs = append(anchorIDs, anchor.ID)
				for _, memberID := range anchor.MemberIDs {
					addAnchorLocalMember(memberID, known, owned, memberSet)
				}
			}
			members := candidatesFromIDSet(memberSet, known)
			if len(members) == 0 {
				continue
			}
			id := componentID(candidateIDs(members))
			name := group.name
			if len(matching) > maxAnchorMembers {
				name += fmt.Sprintf(" %d", start/maxAnchorMembers+1)
			}
			components = append(components, Component{
				ID: id, Name: name, Description: group.description,
				Members: members, AnchorIDs: anchorIDs, SourceIDs: []ComponentID{id},
			})
		}
	}
	return components
}

func processEntryLocalClass(anchor BehaviorAnchor, known map[MemberID]Candidate, modulePrefix string) string {
	if role := processEntryExecutableRole(anchor, known); role != "" {
		return role
	}
	packagePath := processEntryPackagePath(anchor, known)
	segments := strings.Split(strings.ToLower(packagePath), "/")
	if containsAnySegment(segments, "test", "tests", "testing", "testutil", "testdata", "helper", "helpers", "example", "examples") {
		return "test_or_helper"
	}
	if containsAnySegment(segments, "dev", "tool", "tools", "hack", "script", "scripts", "build", "release", "generator", "generators") {
		return "tooling"
	}
	if modulePrefix != "" && (packagePath == modulePrefix || path.Base(packagePath) == moduleBaseName(modulePrefix)) {
		return "primary"
	}
	if packagePath != "" {
		return "secondary_service"
	}
	return "unknown"
}

func processEntryExecutableRole(anchor BehaviorAnchor, known map[MemberID]Candidate) string {
	for _, memberID := range anchor.MemberIDs {
		currentID := memberID
		for {
			candidate, exists := known[currentID]
			if !exists {
				break
			}
			for _, fact := range candidate.Facts {
				if fact.Kind != FactExecutableRole {
					continue
				}
				switch fact.Value {
				case "primary_application":
					return "primary"
				case "secondary_service":
					return "secondary_service"
				case "tooling", "secondary_tooling":
					return "tooling"
				case "test_or_helper":
					return "test_or_helper"
				}
			}
			if candidate.ParentID == nil {
				break
			}
			currentID = *candidate.ParentID
		}
	}
	return ""
}

func moduleBaseName(modulePath string) string {
	base := path.Base(modulePath)
	if len(base) > 1 && base[0] == 'v' {
		allDigits := true
		for _, char := range base[1:] {
			if char < '0' || char > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return path.Base(path.Dir(modulePath))
		}
	}
	return base
}

func processEntryModulePrefix(anchors []BehaviorAnchor, known map[MemberID]Candidate) string {
	counts := make(map[string]int)
	for _, anchor := range anchors {
		packagePath := processEntryPackagePath(anchor, known)
		if index := strings.Index(packagePath, "/cmd/"); index > 0 {
			counts[packagePath[:index]]++
		}
	}
	selected := ""
	selectedCount := 0
	for prefix, count := range counts {
		if count > selectedCount || count == selectedCount && prefix < selected {
			selected = prefix
			selectedCount = count
		}
	}
	if selected != "" {
		return selected
	}
	if len(anchors) == 1 {
		return processEntryPackagePath(anchors[0], known)
	}
	return ""
}

func processEntryPackagePath(anchor BehaviorAnchor, known map[MemberID]Candidate) string {
	for _, memberID := range anchor.MemberIDs {
		currentID := memberID
		for {
			candidate, exists := known[currentID]
			if !exists {
				break
			}
			for _, fact := range candidate.Facts {
				if fact.Kind == FactDeclaration && strings.HasSuffix(fact.Value, ".main") {
					return strings.TrimSuffix(fact.Value, ".main")
				}
			}
			if strings.HasSuffix(candidate.Name, ".main") {
				return strings.TrimSuffix(candidate.Name, ".main")
			}
			if candidate.ParentID == nil {
				break
			}
			currentID = *candidate.ParentID
		}
	}
	return ""
}

func containsAnySegment(segments []string, candidates ...string) bool {
	for _, segment := range segments {
		for _, candidate := range candidates {
			if segment == candidate {
				return true
			}
		}
	}
	return false
}

func anchorHasFlowParticipation(anchor BehaviorAnchor, known map[MemberID]Candidate) bool {
	for _, memberID := range anchor.MemberIDs {
		currentID := memberID
		for {
			candidate, exists := known[currentID]
			if !exists {
				break
			}
			if len(candidate.Participations) > 0 {
				return true
			}
			if candidate.ParentID == nil {
				break
			}
			currentID = *candidate.ParentID
		}
	}
	return false
}

func addAnchorLocalMember(
	memberID MemberID,
	known map[MemberID]Candidate,
	owned map[MemberID]struct{},
	selected map[MemberID]struct{},
) {
	currentID := memberID
	for {
		candidate, exists := known[currentID]
		if !exists {
			return
		}
		if _, alreadyOwned := owned[currentID]; !alreadyOwned {
			owned[currentID] = struct{}{}
			selected[currentID] = struct{}{}
		}
		if candidate.ParentID == nil {
			return
		}
		currentID = *candidate.ParentID
	}
}

func candidatesFromIDSet(ids map[MemberID]struct{}, known map[MemberID]Candidate) []Candidate {
	result := make([]Candidate, 0, len(ids))
	for id := range ids {
		result = append(result, cloneCandidate(known[id]))
	}
	sortCandidates(result)
	return result
}

func anchorLocalComponentName(kind BehaviorAnchorKind) string {
	switch kind {
	case AnchorProcessEntry:
		return "Process entry"
	case AnchorCommandDispatch:
		return "Command dispatch"
	case AnchorConfigIngress:
		return "Configuration ingress"
	case AnchorConfigAdapter:
		return "Configuration adapters"
	case AnchorConfigApply:
		return "Configuration application"
	case AnchorRegistryWrite:
		return "Extension registration"
	case AnchorRegistryLookup:
		return "Extension lookup"
	case AnchorExtensionFamily:
		return "Extension families"
	case AnchorLifecycleInterface:
		return "Lifecycle contracts"
	case AnchorLifecycleStart:
		return "Lifecycle startup"
	case AnchorAdminControlPlane:
		return "Administrative control plane"
	case AnchorRequestDispatchRoot:
		return "Request dispatch"
	case AnchorApplicationData:
		return "Application data plane"
	case AnchorSecurityBoundary:
		return "TLS and security boundary"
	default:
		return "Unresolved frontier"
	}
}

func localGroupingBasis(candidate Candidate, known map[MemberID]Candidate, flowNames map[FlowID]string) (string, string, string) {
	root := candidate
	seen := make(map[MemberID]struct{})
	for root.ParentID != nil {
		if _, exists := seen[root.ID]; exists {
			break
		}
		seen[root.ID] = struct{}{}
		parent, exists := known[*root.ParentID]
		if !exists {
			break
		}
		root = parent
	}
	if root.ID.Kind == MemberPackage || root.ID.Kind == MemberEntrypoint || root.ID.Kind == MemberFlow {
		category := string(root.ID.Kind)
		return category + ":" + root.ID.key(), category, root.Name
	}
	if len(candidate.Participations) > 0 {
		flowIDs := make([]FlowID, len(candidate.Participations))
		for index, participation := range candidate.Participations {
			flowIDs[index] = participation.FlowID
		}
		sort.Slice(flowIDs, func(i, j int) bool { return flowIDs[i] < flowIDs[j] })
		flowID := flowIDs[0]
		name := flowNames[flowID]
		if name == "" {
			name = string(flowID)
		}
		return "flow-participation:" + string(flowID), "flow", name
	}
	category := string(candidate.ID.Kind)
	return "kind:" + category, category, "Repository " + pluralKind(candidate.ID.Kind)
}

func localSubsystemName(category string) string {
	switch category {
	case "entrypoint":
		return "Entrypoints"
	case "flow":
		return "Flows"
	case "package":
		return "Packages"
	case "file":
		return "Files"
	case "symbol":
		return "Symbols"
	case "diagnostic":
		return "Supporting evidence"
	default:
		return "Repository"
	}
}

func pluralKind(kind MemberKind) string {
	switch kind {
	case MemberEntrypoint:
		return "entrypoints"
	default:
		return string(kind) + "s"
	}
}

func componentID(memberIDs []MemberID) ComponentID {
	keys := make([]string, len(memberIDs))
	for index, id := range memberIDs {
		keys[index] = id.key()
	}
	sort.Strings(keys)
	return ComponentID("component-" + stableDigest("component", keys))
}

func subsystemID(componentIDs []ComponentID) SubsystemID {
	keys := make([]string, len(componentIDs))
	for index, id := range componentIDs {
		keys[index] = string(id)
	}
	sort.Strings(keys)
	return SubsystemID("subsystem-" + stableDigest("subsystem", keys))
}

func stableDigest(kind string, values []string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "componentmap/%d/%s\n", ContractVersion, kind)
	for _, value := range values {
		fmt.Fprintf(hash, "%d:%s\n", len(value), value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validateCandidate(candidate Candidate) error {
	if err := validateMemberID(candidate.ID); err != nil {
		return err
	}
	if err := validateDisplayText("candidate name", candidate.Name, maxNameBytes, true); err != nil {
		return err
	}
	if len(candidate.Facts) == 0 || len(candidate.Facts) > maxFactsPerCandidate {
		return fmt.Errorf("candidate fact count is out of bounds")
	}
	for index, fact := range candidate.Facts {
		if err := validateFact(fact); err != nil {
			return fmt.Errorf("facts[%d]: %w", index, err)
		}
	}
	if len(candidate.Participations) > maxFlowsPerCandidate {
		return fmt.Errorf("candidate flow participation count exceeds %d", maxFlowsPerCandidate)
	}
	seenFlows := make(map[FlowID]struct{}, len(candidate.Participations))
	for index, participation := range candidate.Participations {
		if err := validateOpaqueText("flow id", string(participation.FlowID), maxOpaqueIDBytes); err != nil {
			return err
		}
		if _, exists := seenFlows[participation.FlowID]; exists {
			return fmt.Errorf("candidate repeats flow id %q", participation.FlowID)
		}
		seenFlows[participation.FlowID] = struct{}{}
		if participation.Evidence.Kind != FactFlowParticipation {
			return fmt.Errorf("flow participation[%d] is not backed by a flow-participation fact", index)
		}
		if participation.Evidence.Value != string(participation.FlowID) {
			return fmt.Errorf("flow participation[%d] evidence does not identify its typed flow", index)
		}
		if err := validateFact(participation.Evidence); err != nil {
			return fmt.Errorf("flow participation[%d]: %w", index, err)
		}
		if participation.Evidence.Certainty != evidence.CertaintyStatic &&
			participation.Evidence.Certainty != evidence.CertaintyObserved &&
			participation.Evidence.Certainty != evidence.CertaintyVerified {
			return fmt.Errorf("flow participation[%d] is not locally grounded", index)
		}
	}
	return nil
}

func validateFlow(flow Flow) error {
	if err := validateOpaqueText("flow id", string(flow.ID), maxOpaqueIDBytes); err != nil {
		return err
	}
	if err := validateDisplayText("flow name", flow.Name, maxNameBytes, true); err != nil {
		return err
	}
	if len(flow.Facts) == 0 || len(flow.Facts) > maxFactsPerCandidate {
		return fmt.Errorf("flow fact count is out of bounds")
	}
	for index, fact := range flow.Facts {
		if err := validateFact(fact); err != nil {
			return fmt.Errorf("facts[%d]: %w", index, err)
		}
	}
	return nil
}

func validateFact(fact LocalFact) error {
	if !fact.Kind.valid() {
		return fmt.Errorf("invalid fact kind %q", fact.Kind)
	}
	if err := validateDisplayText("fact value", fact.Value, maxFactValueBytes, true); err != nil {
		return err
	}
	if !fact.Certainty.Valid() {
		return fmt.Errorf("invalid fact certainty %q", fact.Certainty)
	}
	if len(fact.Provenance) == 0 || len(fact.Provenance) > maxProvenanceItems {
		return fmt.Errorf("fact provenance count is out of bounds")
	}
	for index, provenance := range fact.Provenance {
		if err := validateProvenance(provenance); err != nil {
			return fmt.Errorf("fact provenance[%d]: %w", index, err)
		}
	}
	if fact.Location != nil {
		if err := validateLocation(*fact.Location); err != nil {
			return fmt.Errorf("fact location: %w", err)
		}
	}
	return nil
}

func validateBehaviorAnchor(anchor BehaviorAnchor, known map[MemberID]Candidate) error {
	if err := validateOpaqueText("behavior anchor id", anchor.ID, maxOpaqueIDBytes); err != nil {
		return err
	}
	if !anchor.Kind.valid() {
		return fmt.Errorf("invalid behavior anchor kind %q", anchor.Kind)
	}
	if err := validateDisplayText("behavior anchor label", anchor.Label, maxNameBytes, true); err != nil {
		return err
	}
	if err := validateLocation(anchor.Location); err != nil || anchor.Location.Line == 0 {
		return fmt.Errorf("behavior anchor location must be exact")
	}
	if err := validateScenarioContext(anchor.Scenario); err != nil {
		return fmt.Errorf("scenario: %w", err)
	}
	if err := validateProvenance(anchor.Producer); err != nil {
		return fmt.Errorf("producer: %w", err)
	}
	if anchor.Certainty != evidence.CertaintyStatic && anchor.Certainty != evidence.CertaintyObserved &&
		anchor.Certainty != evidence.CertaintyVerified {
		return fmt.Errorf("behavior anchor certainty %q is not locally grounded", anchor.Certainty)
	}
	if len(anchor.MemberIDs) == 0 || len(anchor.MemberIDs) > maxAnchorMembers {
		return fmt.Errorf("behavior anchor member count is out of bounds")
	}
	seenMembers := make(map[MemberID]struct{}, len(anchor.MemberIDs))
	for _, memberID := range anchor.MemberIDs {
		if _, exists := known[memberID]; !exists {
			return fmt.Errorf("behavior anchor references unknown member")
		}
		if _, duplicate := seenMembers[memberID]; duplicate {
			return fmt.Errorf("behavior anchor repeats a member")
		}
		seenMembers[memberID] = struct{}{}
	}
	if len(anchor.Limitations) == 0 || len(anchor.Limitations) > maxLimitations {
		return fmt.Errorf("behavior anchor limitation count is out of bounds")
	}
	for _, limitation := range anchor.Limitations {
		if err := validateDisplayText("behavior anchor limitation", limitation, maxDescriptionBytes, true); err != nil {
			return err
		}
	}
	return nil
}

func behaviorAnchorsForMembers(anchors []BehaviorAnchor, members []Candidate) []string {
	memberSet := make(map[MemberID]struct{}, len(members))
	for _, member := range members {
		memberSet[member.ID] = struct{}{}
	}
	result := make([]string, 0)
	for _, anchor := range anchors {
		for _, memberID := range anchor.MemberIDs {
			if _, exists := memberSet[memberID]; exists {
				result = append(result, anchor.ID)
				break
			}
		}
	}
	sort.Strings(result)
	return result
}

func validateLocalRelation(relation LocalRelation, known map[MemberID]Candidate) error {
	if err := validateOpaqueText("relation id", relation.ID, maxOpaqueIDBytes); err != nil {
		return err
	}
	if err := validateMemberID(relation.From); err != nil {
		return fmt.Errorf("source member: %w", err)
	}
	if err := validateMemberID(relation.To); err != nil {
		return fmt.Errorf("target member: %w", err)
	}
	if relation.From == relation.To {
		return fmt.Errorf("structural relation is self-referential")
	}
	if _, exists := known[relation.From]; !exists {
		return fmt.Errorf("structural relation has unknown source member")
	}
	if _, exists := known[relation.To]; !exists {
		return fmt.Errorf("structural relation has unknown target member")
	}
	if !relation.Kind.valid() {
		return fmt.Errorf("invalid structural relation kind %q", relation.Kind)
	}
	if relation.Kind == StructuralRelationPackageImport &&
		(relation.From.Kind != MemberPackage || relation.To.Kind != MemberPackage) {
		return fmt.Errorf("package-import relation endpoints must be package members")
	}
	if relation.Certainty != evidence.CertaintyStatic &&
		relation.Certainty != evidence.CertaintyObserved &&
		relation.Certainty != evidence.CertaintyVerified {
		return fmt.Errorf("structural relation certainty %q is not locally grounded", relation.Certainty)
	}
	if len(relation.Provenance) == 0 || len(relation.Provenance) > maxProvenanceItems {
		return fmt.Errorf("structural relation provenance count is out of bounds")
	}
	for index, provenance := range relation.Provenance {
		if err := validateProvenance(provenance); err != nil {
			return fmt.Errorf("provenance[%d]: %w", index, err)
		}
	}
	if relation.Location != nil {
		if err := validateLocation(*relation.Location); err != nil {
			return fmt.Errorf("location: %w", err)
		}
	}
	if len(relation.Scenarios) == 0 || len(relation.Scenarios) > maxScenarioContexts {
		return fmt.Errorf("structural relation scenario count is out of bounds")
	}
	seenScenarios := make(map[string]struct{}, len(relation.Scenarios))
	for index, scenario := range relation.Scenarios {
		if err := validateScenarioContext(scenario); err != nil {
			return fmt.Errorf("scenarios[%d]: %w", index, err)
		}
		if _, duplicate := seenScenarios[scenario.ID]; duplicate {
			return fmt.Errorf("duplicate scenario %q", scenario.ID)
		}
		seenScenarios[scenario.ID] = struct{}{}
	}
	return nil
}

func validateFlowAnchorBinding(
	binding FlowAnchorBinding,
	known map[MemberID]Candidate,
	flowIDs map[FlowID]struct{},
) error {
	if err := validateOpaqueText("flow id", string(binding.FlowID), maxOpaqueIDBytes); err != nil {
		return err
	}
	if _, exists := flowIDs[binding.FlowID]; !exists {
		return fmt.Errorf("binding references unknown flow")
	}
	if err := validateOpaqueText("anchor id", binding.AnchorID, maxOpaqueIDBytes); err != nil {
		return err
	}
	if err := validateMemberID(binding.MemberID); err != nil {
		return fmt.Errorf("member: %w", err)
	}
	candidate, exists := known[binding.MemberID]
	if !exists {
		return fmt.Errorf("binding references unknown member")
	}
	if !candidateParticipatesIn(candidate, binding.FlowID) {
		return fmt.Errorf("bound member has no witnessed participation in the flow")
	}
	if binding.Certainty != evidence.CertaintyStatic &&
		binding.Certainty != evidence.CertaintyObserved &&
		binding.Certainty != evidence.CertaintyVerified {
		return fmt.Errorf("binding certainty %q is not locally grounded", binding.Certainty)
	}
	if len(binding.Provenance) == 0 || len(binding.Provenance) > maxProvenanceItems {
		return fmt.Errorf("binding provenance count is out of bounds")
	}
	for index, provenance := range binding.Provenance {
		if err := validateProvenance(provenance); err != nil {
			return fmt.Errorf("provenance[%d]: %w", index, err)
		}
	}
	if binding.Location != nil {
		if err := validateLocation(*binding.Location); err != nil {
			return fmt.Errorf("location: %w", err)
		}
	}
	if len(binding.Scenarios) > maxScenarioContexts {
		return fmt.Errorf("binding scenario count exceeds %d", maxScenarioContexts)
	}
	for index, scenario := range binding.Scenarios {
		if err := validateScenarioContext(scenario); err != nil {
			return fmt.Errorf("scenarios[%d]: %w", index, err)
		}
	}
	return nil
}

func candidateParticipatesIn(candidate Candidate, flowID FlowID) bool {
	for _, participation := range candidate.Participations {
		if participation.FlowID == flowID {
			return true
		}
	}
	return false
}

func validateProvenance(provenance evidence.Provenance) error {
	if err := validateDisplayText("provider", provenance.Provider, maxProvenanceBytes, true); err != nil {
		return err
	}
	if err := validateDisplayText("version", provenance.Version, maxProvenanceBytes, false); err != nil {
		return err
	}
	if err := validateDisplayText("operation", provenance.Operation, maxProvenanceBytes, true); err != nil {
		return err
	}
	if err := validateDisplayText("detail", provenance.Detail, maxProvenanceBytes, false); err != nil {
		return err
	}
	if provenance.Location != nil {
		if err := validateLocation(*provenance.Location); err != nil {
			return fmt.Errorf("location: %w", err)
		}
	}
	return nil
}

func validateScenarioContext(scenario ScenarioContext) error {
	if err := validateOpaqueText("scenario id", scenario.ID, maxOpaqueIDBytes); err != nil {
		return err
	}
	if err := validateDisplayText("scenario name", scenario.Name, maxNameBytes, true); err != nil {
		return err
	}
	if scenario.Build.GOOS != "" {
		if err := validateOpaqueText("scenario GOOS", scenario.Build.GOOS, maxOpaqueIDBytes); err != nil {
			return err
		}
	}
	if scenario.Build.GOARCH != "" {
		if err := validateOpaqueText("scenario GOARCH", scenario.Build.GOARCH, maxOpaqueIDBytes); err != nil {
			return err
		}
	}
	if len(scenario.Build.BuildTags) > 32 {
		return fmt.Errorf("scenario has too many build tags")
	}
	for _, tag := range scenario.Build.BuildTags {
		if err := validateOpaqueText("build tag", tag, maxOpaqueIDBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateLocation(location evidence.Location) error {
	if err := validateDisplayText("path", location.Path, maxPathBytes, true); err != nil {
		return err
	}
	if location.Line < 0 || location.Column < 0 || location.EndLine < 0 || location.EndColumn < 0 {
		return fmt.Errorf("source coordinates are invalid")
	}
	if location.Line == 0 {
		if location.Column != 0 || location.EndLine != 0 || location.EndColumn != 0 {
			return fmt.Errorf("path-only evidence cannot carry partial coordinates")
		}
		return nil
	}
	if location.EndLine > 0 && location.EndLine < location.Line {
		return fmt.Errorf("source range ends before it starts")
	}
	if location.EndLine == 0 && location.EndColumn != 0 {
		return fmt.Errorf("source range has an end column without an end line")
	}
	return nil
}

func validateDiagnostic(diagnostic Diagnostic) error {
	if err := validateOpaqueText("diagnostic code", diagnostic.Code, maxOpaqueIDBytes); err != nil {
		return err
	}
	if err := validateDisplayText("diagnostic message", diagnostic.Message, maxDescriptionBytes, true); err != nil {
		return err
	}
	if diagnostic.Severity != FindingFatal && diagnostic.Severity != FindingRecoverable && diagnostic.Severity != FindingAdvisory {
		return fmt.Errorf("invalid diagnostic severity %q", diagnostic.Severity)
	}
	if diagnostic.Member != nil {
		if err := validateMemberID(*diagnostic.Member); err != nil {
			return fmt.Errorf("diagnostic member: %w", err)
		}
	}
	return nil
}

func validateMemberID(id MemberID) error {
	if !id.Kind.valid() {
		return fmt.Errorf("invalid member kind %q", id.Kind)
	}
	return validateOpaqueText("member id", id.Value, maxOpaqueIDBytes)
}

func validateOpaqueText(field, value string, limit int) error {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || len(value) > limit {
		return fmt.Errorf("%s is empty, malformed, or too long", field)
	}
	for _, char := range value {
		if char < 0x21 || char == 0x7f {
			return fmt.Errorf("%s contains control or whitespace characters", field)
		}
	}
	return nil
}

func validateDisplayText(field, value string, limit int, required bool) error {
	if !utf8.ValidString(value) || len(value) > limit || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is malformed or too long", field)
	}
	if required && value == "" {
		return fmt.Errorf("%s is empty", field)
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return fmt.Errorf("%s contains control characters", field)
		}
	}
	return nil
}

func validateParentCycles(candidates []Candidate, known map[MemberID]Candidate) error {
	for _, candidate := range candidates {
		seen := make(map[MemberID]struct{})
		current := candidate
		for current.ParentID != nil {
			if _, exists := seen[current.ID]; exists {
				return fmt.Errorf("componentmap: parent cycle includes %q", current.ID.key())
			}
			seen[current.ID] = struct{}{}
			current = known[*current.ParentID]
		}
	}
	return nil
}

func candidateIndex(bundle CandidateBundle) map[MemberID]Candidate {
	result := make(map[MemberID]Candidate, len(bundle.Candidates))
	for _, candidate := range bundle.Candidates {
		result[candidate.ID] = candidate
	}
	return result
}

func cloneCandidate(candidate Candidate) Candidate {
	cloned := candidate
	if candidate.ParentID != nil {
		parentID := *candidate.ParentID
		cloned.ParentID = &parentID
	}
	if len(candidate.Participations) > 0 {
		cloned.Participations = make([]FlowParticipation, len(candidate.Participations))
		for index, participation := range candidate.Participations {
			cloned.Participations[index] = FlowParticipation{
				FlowID:   participation.FlowID,
				Evidence: cloneLocalFact(participation.Evidence),
			}
		}
	}
	cloned.Facts = make([]LocalFact, len(candidate.Facts))
	for index, fact := range candidate.Facts {
		cloned.Facts[index] = cloneLocalFact(fact)
	}
	return cloned
}

func cloneLocalFact(fact LocalFact) LocalFact {
	cloned := fact
	if fact.Location != nil {
		location := *fact.Location
		cloned.Location = &location
	}
	cloned.Provenance = append([]evidence.Provenance(nil), fact.Provenance...)
	for index, provenance := range cloned.Provenance {
		if provenance.Location == nil {
			continue
		}
		location := *provenance.Location
		cloned.Provenance[index].Location = &location
	}
	return cloned
}

func cloneLocalRelations(relations []LocalRelation) []LocalRelation {
	if relations == nil {
		return nil
	}
	cloned := make([]LocalRelation, len(relations))
	for index, relation := range relations {
		cloned[index] = relation
		if relation.Location != nil {
			location := *relation.Location
			cloned[index].Location = &location
		}
		cloned[index].Provenance = append([]evidence.Provenance(nil), relation.Provenance...)
		for provenanceIndex, provenance := range cloned[index].Provenance {
			if provenance.Location == nil {
				continue
			}
			location := *provenance.Location
			cloned[index].Provenance[provenanceIndex].Location = &location
		}
		cloned[index].Scenarios = append([]ScenarioContext(nil), relation.Scenarios...)
		for scenarioIndex := range cloned[index].Scenarios {
			cloned[index].Scenarios[scenarioIndex].Build.BuildTags = append(
				[]string(nil), relation.Scenarios[scenarioIndex].Build.BuildTags...,
			)
		}
	}
	return cloned
}

func cloneFlowAnchorBindings(bindings []FlowAnchorBinding) []FlowAnchorBinding {
	if bindings == nil {
		return nil
	}
	cloned := make([]FlowAnchorBinding, len(bindings))
	for index, binding := range bindings {
		cloned[index] = binding
		if binding.Location != nil {
			location := *binding.Location
			cloned[index].Location = &location
		}
		cloned[index].Provenance = append([]evidence.Provenance(nil), binding.Provenance...)
		for provenanceIndex, provenance := range cloned[index].Provenance {
			if provenance.Location == nil {
				continue
			}
			location := *provenance.Location
			cloned[index].Provenance[provenanceIndex].Location = &location
		}
		cloned[index].Scenarios = append([]ScenarioContext(nil), binding.Scenarios...)
		for scenarioIndex := range cloned[index].Scenarios {
			cloned[index].Scenarios[scenarioIndex].Build.BuildTags = append(
				[]string(nil), binding.Scenarios[scenarioIndex].Build.BuildTags...,
			)
		}
	}
	return cloned
}

func cloneResearchInterpretations(findings []ResearchInterpretation) []ResearchInterpretation {
	if findings == nil {
		return nil
	}
	result := make([]ResearchInterpretation, len(findings))
	for index, finding := range findings {
		result[index] = finding
		result[index].EvidenceIDs = append([]string(nil), finding.EvidenceIDs...)
		result[index].MemberIDs = append([]MemberID(nil), finding.MemberIDs...)
		result[index].FlowIDs = append([]FlowID(nil), finding.FlowIDs...)
		result[index].AnchorIDs = append([]string(nil), finding.AnchorIDs...)
	}
	return result
}

func sortCandidates(candidates []Candidate) {
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID.key() < candidates[j].ID.key() })
}

func candidateIDs(candidates []Candidate) []MemberID {
	result := make([]MemberID, len(candidates))
	for index, candidate := range candidates {
		result[index] = candidate.ID
	}
	return result
}

func componentIDs(components []Component) []ComponentID {
	result := make([]ComponentID, len(components))
	for index, component := range components {
		result[index] = component.ID
	}
	return result
}

func conceptualMembershipsFromSubsystems(subsystems []Subsystem) []ConceptualMembership {
	memberships := make([]ConceptualMembership, 0)
	for _, subsystem := range subsystems {
		for _, component := range subsystem.Components {
			for _, member := range component.Members {
				memberships = append(memberships, ConceptualMembership{
					ComponentID: component.ID,
					MemberID:    member.ID,
				})
			}
		}
	}
	sort.Slice(memberships, func(i, j int) bool {
		if memberships[i].ComponentID != memberships[j].ComponentID {
			return memberships[i].ComponentID < memberships[j].ComponentID
		}
		return memberships[i].MemberID.key() < memberships[j].MemberID.key()
	})
	return memberships
}
