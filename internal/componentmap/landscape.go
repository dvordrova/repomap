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
	ContractVersion = 1

	maxCandidates          = 512
	maxFlows               = 64
	maxFactsPerCandidate   = 16
	maxFlowIDsPerCandidate = 16
	maxSubsystems          = 16
	maxComponents          = 32
	maxNameBytes           = 256
	maxDescriptionBytes    = 1_024
	maxOpaqueIDBytes       = 128
	maxFactValueBytes      = 2_048
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

// Candidate is one exact, locally known landscape member. ParentID and FlowIDs
// are bounded grouping inputs, not inferred architectural edges.
type Candidate struct {
	ID       MemberID    `json:"id"`
	Name     string      `json:"name"`
	ParentID *MemberID   `json:"parent_id,omitempty"`
	FlowIDs  []FlowID    `json:"flow_ids,omitempty"`
	Facts    []LocalFact `json:"facts"`
}

// Flow records the exact local flow identity used by candidate participation.
type Flow struct {
	ID    FlowID      `json:"id"`
	Name  string      `json:"name"`
	Facts []LocalFact `json:"facts"`
}

// CandidateBundle is the bounded, versioned input to conceptual synthesis.
// It contains no coordinates and gives a proposal no authority over evidence.
type CandidateBundle struct {
	Version    int         `json:"version"`
	Candidates []Candidate `json:"candidates"`
	Flows      []Flow      `json:"flows,omitempty"`
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

// Landscape is the locally validated conceptual membership result. Fallback
// is explicit so presentation never mistakes deterministic grouping for a
// provider-authored architecture claim.
type Landscape struct {
	Version        int          `json:"version"`
	Subsystems     []Subsystem  `json:"subsystems"`
	Diagnostics    []Diagnostic `json:"diagnostics,omitempty"`
	Fallback       bool         `json:"fallback"`
	FallbackReason string       `json:"fallback_reason,omitempty"`
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
		landscape.FallbackReason = "proposal_invalid_or_empty"
	}
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
		for _, flowID := range candidate.FlowIDs {
			if _, exists := flowIDs[flowID]; !exists {
				return fmt.Errorf("componentmap: member %q references unknown flow %q", candidate.ID.key(), flowID)
			}
		}
		if candidate.ID.Kind == MemberFlow && len(candidate.FlowIDs) != 1 {
			return fmt.Errorf("componentmap: flow member %q must reference exactly one flow", candidate.ID.key())
		}
	}
	if err := validateParentCycles(bundle.Candidates, members); err != nil {
		return err
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

	known := candidateIndex(bundle)
	seenMembers := make(map[MemberID]struct{})
	landscape := Landscape{Version: ContractVersion, Subsystems: make([]Subsystem, 0, len(proposal.Subsystems))}
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
	return Landscape{Version: ContractVersion, Subsystems: subsystems}
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
	if len(candidate.FlowIDs) > 0 {
		flowIDs := append([]FlowID(nil), candidate.FlowIDs...)
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
	if len(candidate.FlowIDs) > maxFlowIDsPerCandidate {
		return fmt.Errorf("candidate flow id count exceeds %d", maxFlowIDsPerCandidate)
	}
	seenFlows := make(map[FlowID]struct{}, len(candidate.FlowIDs))
	for _, flowID := range candidate.FlowIDs {
		if err := validateOpaqueText("flow id", string(flowID), maxOpaqueIDBytes); err != nil {
			return err
		}
		if _, exists := seenFlows[flowID]; exists {
			return fmt.Errorf("candidate repeats flow id %q", flowID)
		}
		seenFlows[flowID] = struct{}{}
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
	if len(fact.Provenance) == 0 {
		return fmt.Errorf("fact has no provenance")
	}
	for index, provenance := range fact.Provenance {
		if strings.TrimSpace(provenance.Provider) == "" || strings.TrimSpace(provenance.Operation) == "" {
			return fmt.Errorf("fact provenance[%d] is incomplete", index)
		}
	}
	if fact.Location != nil && strings.TrimSpace(fact.Location.Path) == "" {
		return fmt.Errorf("fact location has no path")
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
	cloned.FlowIDs = append([]FlowID(nil), candidate.FlowIDs...)
	cloned.Facts = make([]LocalFact, len(candidate.Facts))
	for index, fact := range candidate.Facts {
		cloned.Facts[index] = fact
		if fact.Location != nil {
			location := *fact.Location
			cloned.Facts[index].Location = &location
		}
		cloned.Facts[index].Provenance = append([]evidence.Provenance(nil), fact.Provenance...)
		for provenanceIndex, provenance := range cloned.Facts[index].Provenance {
			if provenance.Location == nil {
				continue
			}
			location := *provenance.Location
			cloned.Facts[index].Provenance[provenanceIndex].Location = &location
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
