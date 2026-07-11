package componentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	// ContractVersion changes whenever candidate identity, proposal authority,
	// or locally validated landscape semantics change.
	ContractVersion = 2

	maxCandidates        = 512
	maxFlows             = 64
	maxRelations         = 1_024
	maxAnchorBindings    = 2_048
	maxFactsPerCandidate = 16
	maxFlowsPerCandidate = 16
	maxSubsystems        = 16
	maxComponents        = 32
	maxProvenanceItems   = 8
	maxScenarioContexts  = 8
	maxNameBytes         = 256
	maxDescriptionBytes  = 1_024
	maxOpaqueIDBytes     = 128
	maxFactValueBytes    = 2_048
	maxPathBytes         = 4_096
	maxProvenanceBytes   = 1_024
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

// FactKind is a small vocabulary for locally extracted candidate facts. Facts
// remain evidence; they are not component relations or temporal ordering.
type FactKind string

const (
	FactDeclaration       FactKind = "declaration"
	FactContainment       FactKind = "containment"
	FactFlowParticipation FactKind = "flow_participation"
	FactRepositoryPath    FactKind = "repository_path"
)

func (kind FactKind) valid() bool {
	switch kind {
	case FactDeclaration, FactContainment, FactFlowParticipation, FactRepositoryPath:
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

const StructuralRelationPackageImport StructuralRelationKind = "package_import"

func (kind StructuralRelationKind) valid() bool {
	return kind == StructuralRelationPackageImport
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

// CandidateBundle is the bounded, versioned input to conceptual synthesis.
// It contains no coordinates and gives a proposal no authority over evidence.
type CandidateBundle struct {
	Version        int                 `json:"version"`
	Candidates     []Candidate         `json:"candidates"`
	Flows          []Flow              `json:"flows,omitempty"`
	Relations      []LocalRelation     `json:"relations,omitempty"`
	AnchorBindings []FlowAnchorBinding `json:"flow_anchor_bindings,omitempty"`
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
}

type ProposedComponent struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	MemberIDs   []MemberID `json:"member_ids"`
}

type ComponentID string

type SubsystemID string

// Component contains exact candidates reconstructed from the local bundle.
// Its ID is independent of proposal wording and ordering.
type Component struct {
	ID          ComponentID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Members     []Candidate `json:"members"`
}

type Subsystem struct {
	ID          SubsystemID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Components  []Component `json:"components"`
}

type Diagnostic struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Member  *MemberID `json:"member,omitempty"`
}

type FallbackReason string

const (
	FallbackProposalInvalid      FallbackReason = "proposal_invalid_or_empty"
	FallbackModelDisabled        FallbackReason = "model_disabled"
	FallbackProviderUnconfigured FallbackReason = "provider_not_configured"
)

// Landscape is the locally validated conceptual membership result. Fallback
// is explicit so presentation never mistakes deterministic grouping for a
// provider-authored architecture claim.
type Landscape struct {
	Version        int                 `json:"version"`
	Subsystems     []Subsystem         `json:"subsystems"`
	Relations      []LocalRelation     `json:"relations,omitempty"`
	AnchorBindings []FlowAnchorBinding `json:"flow_anchor_bindings,omitempty"`
	Diagnostics    []Diagnostic        `json:"diagnostics,omitempty"`
	Fallback       bool                `json:"fallback"`
	FallbackReason FallbackReason      `json:"fallback_reason,omitempty"`
}

// Apply validates a proposal against exact local candidates. Unknown IDs are
// dropped with diagnostics. Structurally invalid or unusable proposals produce
// a deterministic fallback instead of a partially invented landscape.
func Apply(bundle CandidateBundle, proposal Proposal) (Landscape, error) {
	if err := bundle.Validate(); err != nil {
		return Landscape{}, err
	}

	landscape, diagnostics, usable := applyProposal(bundle, proposal)
	if !usable {
		landscape = deterministicFallback(bundle)
		landscape.Diagnostics = diagnostics
		landscape.Fallback = true
		landscape.FallbackReason = FallbackProposalInvalid
	}
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
	if reason != FallbackModelDisabled && reason != FallbackProviderUnconfigured {
		return Landscape{}, fmt.Errorf("componentmap: invalid deterministic fallback reason %q", reason)
	}
	landscape := deterministicFallback(bundle)
	landscape.Fallback = true
	landscape.FallbackReason = reason
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
	if landscape.Fallback && landscape.FallbackReason != FallbackProposalInvalid &&
		landscape.FallbackReason != FallbackModelDisabled &&
		landscape.FallbackReason != FallbackProviderUnconfigured {
		return fmt.Errorf("componentmap: fallback landscape has unsupported or missing reason")
	}
	if !landscape.Fallback && landscape.FallbackReason != "" {
		return fmt.Errorf("componentmap: non-fallback landscape carries a fallback reason")
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
	if len(landscape.Subsystems) == 0 || len(landscape.Subsystems) > maxSubsystems {
		return fmt.Errorf("componentmap: landscape subsystem count is out of bounds")
	}

	known := candidateIndex(bundle)
	seenMembers := make(map[MemberID]struct{}, len(bundle.Candidates))
	seenComponents := make(map[ComponentID]struct{})
	componentCount := 0
	for subsystemIndex, subsystem := range landscape.Subsystems {
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
			memberIDs := make([]MemberID, 0, len(component.Members))
			for _, member := range component.Members {
				exact, exists := known[member.ID]
				if !exists {
					return fmt.Errorf("componentmap: component references unknown member %q", member.ID.key())
				}
				if !reflect.DeepEqual(member, exact) {
					return fmt.Errorf("componentmap: component changed local member %q", member.ID.key())
				}
				if _, exists := seenMembers[member.ID]; exists {
					return fmt.Errorf("componentmap: duplicate membership for %q", member.ID.key())
				}
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
		diagnostics = append(diagnostics, Diagnostic{Code: code, Message: message})
	}
	if proposal.Version != ContractVersion {
		invalid("proposal.unsupported_version", "proposal version is missing or unsupported")
		return Landscape{}, diagnostics, false
	}
	if len(proposal.Subsystems) == 0 || len(proposal.Subsystems) > maxSubsystems {
		invalid("proposal.invalid_subsystem_count", "proposal has no subsystems or exceeds the subsystem limit")
		return Landscape{}, diagnostics, false
	}
	memberReferenceCount := 0
	componentReferenceCount := 0
	for _, subsystem := range proposal.Subsystems {
		for _, component := range subsystem.Components {
			componentReferenceCount++
			if componentReferenceCount > maxComponents {
				invalid("proposal.invalid_component_count", "proposal exceeds the component limit")
				return Landscape{}, diagnostics, false
			}
			if len(component.MemberIDs) > maxCandidates {
				invalid("proposal.invalid_members", "proposal membership exceeds the candidate limit")
				return Landscape{}, diagnostics, false
			}
			memberReferenceCount += len(component.MemberIDs)
			if memberReferenceCount > maxCandidates {
				invalid("proposal.invalid_members", "proposal membership exceeds the candidate limit")
				return Landscape{}, diagnostics, false
			}
			for _, memberID := range component.MemberIDs {
				if validateMemberID(memberID) != nil {
					invalid("proposal.invalid_member_id", "proposal contains a malformed member id")
					return Landscape{}, diagnostics, false
				}
			}
		}
	}

	known := candidateIndex(bundle)
	seenMembers := make(map[MemberID]struct{})
	landscape := Landscape{
		Version:        ContractVersion,
		Subsystems:     make([]Subsystem, 0, len(proposal.Subsystems)),
		Relations:      cloneLocalRelations(bundle.Relations),
		AnchorBindings: cloneFlowAnchorBindings(bundle.AnchorBindings),
	}
	componentCount := 0
	for _, proposedSubsystem := range proposal.Subsystems {
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
			componentCount++
			componentName := strings.TrimSpace(proposedComponent.Name)
			componentDescription := strings.TrimSpace(proposedComponent.Description)
			if componentCount > maxComponents ||
				validateDisplayText("component name", componentName, maxNameBytes, true) != nil ||
				validateDisplayText("component description", componentDescription, maxDescriptionBytes, false) != nil ||
				len(proposedComponent.MemberIDs) == 0 {
				invalid("proposal.invalid_component", "proposal contains an empty or malformed component")
				return Landscape{}, diagnostics, false
			}
			members := make([]Candidate, 0, len(proposedComponent.MemberIDs))
			for _, memberID := range proposedComponent.MemberIDs {
				candidate, exists := known[memberID]
				if !exists {
					id := memberID
					diagnostics = append(diagnostics, Diagnostic{
						Code: "proposal.unknown_member_dropped", Message: "dropped a member id absent from the local candidate bundle", Member: &id,
					})
					continue
				}
				if _, exists := seenMembers[memberID]; exists {
					invalid("proposal.duplicate_membership", "a member appears in more than one proposed membership position")
					return Landscape{}, diagnostics, false
				}
				seenMembers[memberID] = struct{}{}
				members = append(members, cloneCandidate(candidate))
			}
			if len(members) == 0 {
				invalid("proposal.empty_membership", "no known member survived in a proposed component")
				return Landscape{}, diagnostics, false
			}
			sortCandidates(members)
			subsystem.Components = append(subsystem.Components, Component{
				ID: componentID(candidateIDs(members)), Name: componentName, Description: componentDescription, Members: members,
			})
		}
		subsystem.ID = subsystemID(componentIDs(subsystem.Components))
		landscape.Subsystems = append(landscape.Subsystems, subsystem)
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
			Components:  []Component{remainder},
		})
		diagnostics = append(diagnostics, Diagnostic{
			Code:    "proposal.omitted_members_preserved",
			Message: "conceptual synthesis omitted local candidates; they remain visible in a deterministic remainder",
		})
	}
	landscape.Diagnostics = diagnostics
	return landscape, diagnostics, true
}

type fallbackGroup struct {
	key         string
	category    string
	name        string
	description string
	members     []Candidate
}

func deterministicFallback(bundle CandidateBundle) Landscape {
	known := candidateIndex(bundle)
	flowNames := make(map[FlowID]string, len(bundle.Flows))
	for _, flow := range bundle.Flows {
		flowNames[flow.ID] = flow.Name
	}

	groupsByKey := make(map[string]*fallbackGroup)
	candidates := append([]Candidate(nil), bundle.Candidates...)
	sortCandidates(candidates)
	for _, candidate := range candidates {
		key, category, name := fallbackBasis(candidate, known, flowNames)
		group := groupsByKey[key]
		if group == nil {
			group = &fallbackGroup{
				key: key, category: category, name: name,
				description: "Deterministic grouping from exact local " + category + " candidates.",
			}
			groupsByKey[key] = group
		}
		group.members = append(group.members, cloneCandidate(candidate))
	}

	groups := make([]fallbackGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		sortCandidates(group.members)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].key < groups[j].key })
	if len(groups) > maxComponents {
		kept := append([]fallbackGroup(nil), groups[:maxComponents-1]...)
		remainder := fallbackGroup{
			key: "zz:other", category: "repository", name: "Other repository members",
			description: "Deterministic bounded remainder of exact local candidates.",
		}
		for _, group := range groups[maxComponents-1:] {
			remainder.members = append(remainder.members, group.members...)
		}
		sortCandidates(remainder.members)
		groups = append(kept, remainder)
	}

	byCategory := make(map[string][]Component)
	for _, group := range groups {
		byCategory[group.category] = append(byCategory[group.category], Component{
			ID: componentID(candidateIDs(group.members)), Name: group.name, Description: group.description, Members: group.members,
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
		name := fallbackSubsystemName(category)
		subsystems = append(subsystems, Subsystem{
			ID: subsystemID(componentIDs(components)), Name: name,
			Description: "Deterministic local " + category + " landscape.", Components: components,
		})
	}
	return Landscape{
		Version: ContractVersion, Subsystems: subsystems,
		Relations: cloneLocalRelations(bundle.Relations), AnchorBindings: cloneFlowAnchorBindings(bundle.AnchorBindings),
	}
}

func fallbackBasis(candidate Candidate, known map[MemberID]Candidate, flowNames map[FlowID]string) (string, string, string) {
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

func fallbackSubsystemName(category string) string {
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
