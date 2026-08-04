package tasklens

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

const (
	RetrievalTraceVersion  = 1
	MaxVerificationAnchors = 2
)

// SourceScopeKind describes exactly how much of an underlying source unit an
// anchor retains. It is evidence scope, not a claim about runtime behavior.
type SourceScopeKind string

const (
	SourceScopeCompleteEnclosingSymbol SourceScopeKind = "complete_enclosing_symbol"
	SourceScopeCompleteDocumentSection SourceScopeKind = "complete_document_section"
	SourceScopeCompleteFile            SourceScopeKind = "complete_file"
	SourceScopeMatchedFragments        SourceScopeKind = "matched_fragments"
	SourceScopePartialWindow           SourceScopeKind = "partial_window"
)

// NegativeEvidenceBasis identifies local authority independent of prose that
// may justify an absence claim.
type NegativeEvidenceBasis string

const (
	NegativeEvidenceNone                  NegativeEvidenceBasis = "none"
	NegativeEvidenceCompleteScope         NegativeEvidenceBasis = "complete_scope"
	NegativeEvidenceExhaustiveExactSearch NegativeEvidenceBasis = "exhaustive_bounded_exact_search"
	NegativeEvidenceDeterministicManifest NegativeEvidenceBasis = "deterministic_manifest"
)

// SourceScope is the replayable completeness contract for one retained anchor.
type SourceScope struct {
	ScopeKind                SourceScopeKind       `json:"scope_kind"`
	ScopeStart               int                   `json:"scope_start"`
	ScopeEnd                 int                   `json:"scope_end"`
	SourceTotalLines         int                   `json:"source_total_lines"`
	Truncated                bool                  `json:"truncated"`
	TruncationReason         string                `json:"truncation_reason"`
	TaskMatchesOutsideWindow bool                  `json:"task_matches_outside_window"`
	NegativeClaimsAllowed    bool                  `json:"negative_claims_allowed"`
	NegativeEvidenceBasis    NegativeEvidenceBasis `json:"negative_evidence_basis"`
}

// Validate rejects internally inconsistent scope and negative-evidence claims.
func (s SourceScope) Validate() error {
	if !validSourceScopeKind(s.ScopeKind) {
		return fmt.Errorf("task lens: invalid source scope kind")
	}
	if s.ScopeStart <= 0 || s.ScopeEnd < s.ScopeStart || s.SourceTotalLines < 0 ||
		(s.SourceTotalLines == 0 && !s.Truncated) ||
		(s.SourceTotalLines > 0 && s.SourceTotalLines < s.ScopeEnd) {
		return fmt.Errorf("task lens: invalid source scope bounds")
	}
	if !validText(s.TruncationReason, 1024, false) {
		return fmt.Errorf("task lens: invalid source scope truncation reason")
	}
	if !validNegativeEvidenceBasis(s.NegativeEvidenceBasis) {
		return fmt.Errorf("task lens: invalid negative evidence basis")
	}

	isComplete := s.isComplete()
	if isComplete && (s.Truncated || s.TruncationReason != "" || s.TaskMatchesOutsideWindow) {
		return fmt.Errorf("task lens: complete source scope cannot be truncated")
	}
	if s.ScopeKind == SourceScopeCompleteFile &&
		(s.ScopeStart != 1 || s.ScopeEnd != s.SourceTotalLines) {
		return fmt.Errorf("task lens: complete file scope must span the complete source")
	}
	if !isComplete && (!s.Truncated || s.TruncationReason == "") {
		return fmt.Errorf("task lens: incomplete source scope must expose truncation")
	}
	if s.ScopeKind == SourceScopePartialWindow && s.NegativeClaimsAllowed {
		return fmt.Errorf("task lens: partial window cannot allow negative claims")
	}
	if s.NegativeClaimsAllowed && !s.hasNegativeEvidenceAuthority() {
		return fmt.Errorf("task lens: negative claims lack complete or exhaustive authority")
	}
	if s.NegativeEvidenceBasis == NegativeEvidenceCompleteScope && !isComplete {
		return fmt.Errorf("task lens: incomplete source cannot claim complete-scope authority")
	}
	return nil
}

// ValidateClaim rejects an absence claim when the retained scope cannot
// support it. Positive claims still need their own exact evidence.
func (s SourceScope) ValidateClaim(isAbsenceClaim bool) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if isAbsenceClaim && !s.NegativeClaimsAllowed {
		return fmt.Errorf("task lens: absence claim is not allowed by source scope")
	}
	return nil
}

func (s SourceScope) isComplete() bool {
	switch s.ScopeKind {
	case SourceScopeCompleteEnclosingSymbol,
		SourceScopeCompleteDocumentSection,
		SourceScopeCompleteFile:
		return true
	default:
		return false
	}
}

func (s SourceScope) hasNegativeEvidenceAuthority() bool {
	if s.isComplete() {
		return true
	}
	switch s.NegativeEvidenceBasis {
	case NegativeEvidenceExhaustiveExactSearch,
		NegativeEvidenceDeterministicManifest:
		return true
	default:
		return false
	}
}

func validSourceScopeKind(kind SourceScopeKind) bool {
	switch kind {
	case SourceScopeCompleteEnclosingSymbol,
		SourceScopeCompleteDocumentSection,
		SourceScopeCompleteFile,
		SourceScopeMatchedFragments,
		SourceScopePartialWindow:
		return true
	default:
		return false
	}
}

func validNegativeEvidenceBasis(basis NegativeEvidenceBasis) bool {
	switch basis {
	case NegativeEvidenceNone,
		NegativeEvidenceCompleteScope,
		NegativeEvidenceExhaustiveExactSearch,
		NegativeEvidenceDeterministicManifest:
		return true
	default:
		return false
	}
}

// TaskProfile is a generic retrieval profile. Profile names deliberately avoid
// repository- or episode-specific vocabulary.
type TaskProfile string

const (
	TaskProfileDataTagTransformation     TaskProfile = "data_tag_transformation_bug"
	TaskProfileErrorStatusMapping        TaskProfile = "error_status_mapping_bug"
	TaskProfileNilPanic                  TaskProfile = "nil_panic_bug"
	TaskProfileConfigurationPropagation  TaskProfile = "configuration_propagation_bug"
	TaskProfileErrorNormalizationPrivacy TaskProfile = "error_normalization_privacy_bug"
	TaskProfileExtensionContribution     TaskProfile = "extension_contribution_task"
	TaskProfileOperationalRelease        TaskProfile = "operational_release_task"
	TaskProfileUnknown                   TaskProfile = "unknown"
)

// Additional generic roles used by v0.1 role contracts. Existing broad roles
// remain valid and are reused where they already express the required role.
const (
	RoleTransformation                AnchorRole = "transformation_or_parsing"
	RoleUnsafeOperation               AnchorRole = "unsafe_operation"
	RoleNilHandoff                    AnchorRole = "nil_capable_handoff"
	RoleEffectiveDestination          AnchorRole = "effective_destination"
	RolePublicErrorType               AnchorRole = "public_error_type"
	RoleErrorNormalizer               AnchorRole = "error_normalizer"
	RolePublicErrorExposure           AnchorRole = "public_error_exposure"
	RoleExtensionPort                 AnchorRole = "extension_port"
	RoleWiringComposition             AnchorRole = "wiring_or_composition"
	RoleOperationalEntry              AnchorRole = "operational_entry"
	RoleProceduralBody                AnchorRole = "procedural_body"
	RoleModuleTopology                AnchorRole = "module_or_topology_source"
	RoleSafetyCheck                   AnchorRole = "safety_check"
	RoleExample                       AnchorRole = "example"
	RoleRepositoryVerificationCommand AnchorRole = "repository_verification_command"
)

// RoleRequirement allows a profile to require multiple independent anchors
// for one role, such as two representative sibling implementations.
type RoleRequirement struct {
	Role           AnchorRole `json:"role"`
	MinimumAnchors int        `json:"minimum_anchors"`
}

// RoleContract is derived before synthesis and divided by sufficiency impact.
type RoleContract struct {
	Profile    TaskProfile       `json:"profile"`
	Key        []RoleRequirement `json:"key"`
	Supporting []RoleRequirement `json:"supporting"`
	Optional   []RoleRequirement `json:"optional"`
}

// DefaultRoleContract returns the bounded generic contract for a profile.
func DefaultRoleContract(profile TaskProfile) (RoleContract, error) {
	contract := RoleContract{
		Profile:    profile,
		Key:        []RoleRequirement{},
		Supporting: []RoleRequirement{},
		Optional:   []RoleRequirement{},
	}
	switch profile {
	case TaskProfileDataTagTransformation:
		contract.Key = roleRequirements(
			RolePublicOrCLIEntry,
			RoleTransformation,
			RoleGeneratedOutput,
		)
		contract.Supporting = roleRequirements(RoleVerificationAnchor)
		contract.Optional = roleRequirements(RoleReproductionAnchor, RoleDocumentationContract)
	case TaskProfileErrorStatusMapping:
		contract.Key = roleRequirements(RoleSymptomSite, RoleErrorCreation, RoleErrorMapping)
		contract.Supporting = roleRequirements(RoleVerificationAnchor)
		contract.Optional = roleRequirements(RolePublicOrCLIEntry, RoleDocumentationContract)
	case TaskProfileNilPanic:
		contract.Key = roleRequirements(RolePublicOrCLIEntry, RoleUnsafeOperation, RoleNilHandoff)
		contract.Supporting = roleRequirements(RoleVerificationAnchor)
		contract.Optional = roleRequirements(RoleReproductionAnchor)
	case TaskProfileConfigurationPropagation:
		contract.Key = roleRequirements(
			RoleConfigurationSource,
			RoleConfigurationCopy,
			RoleEffectiveDestination,
		)
		contract.Supporting = roleRequirements(RoleVerificationAnchor)
		contract.Optional = roleRequirements(RoleDocumentationContract)
	case TaskProfileErrorNormalizationPrivacy:
		contract.Key = roleRequirements(
			RolePublicErrorType,
			RoleErrorNormalizer,
			RolePublicErrorExposure,
		)
		contract.Supporting = roleRequirements(RoleVerificationAnchor)
		contract.Optional = roleRequirements(RoleErrorCreation, RoleDocumentationContract)
	case TaskProfileExtensionContribution:
		contract.Key = []RoleRequirement{
			{Role: RoleExtensionPort, MinimumAnchors: 1},
			{Role: RoleRepresentativeImplementation, MinimumAnchors: 2},
			{Role: RoleWiringComposition, MinimumAnchors: 1},
		}
		contract.Supporting = roleRequirements(RoleExample, RoleRepositoryVerificationCommand)
		contract.Optional = roleRequirements(RoleDocumentationContract, RoleVerificationAnchor)
	case TaskProfileOperationalRelease:
		contract.Key = roleRequirements(
			RoleDocumentationContract,
			RoleOperationalEntry,
			RoleProceduralBody,
		)
		contract.Supporting = roleRequirements(RoleModuleTopology, RoleSafetyCheck)
		contract.Optional = roleRequirements(RoleRepositoryVerificationCommand, RoleVerificationAnchor)
	case TaskProfileUnknown:
		contract.Supporting = roleRequirements(RoleRepresentativeImplementation)
		contract.Optional = roleRequirements(RoleVerificationAnchor, RoleDocumentationContract)
	default:
		return RoleContract{}, fmt.Errorf("task lens: invalid task profile")
	}
	return contract, contract.Validate()
}

// Validate rejects duplicate roles and invalid minimum coverage.
func (c RoleContract) Validate() error {
	if !validTaskProfile(c.Profile) {
		return fmt.Errorf("task lens: invalid task profile")
	}
	seen := make(map[AnchorRole]struct{})
	groups := [][]RoleRequirement{c.Key, c.Supporting, c.Optional}
	for _, group := range groups {
		for _, requirement := range group {
			if !validContractAnchorRole(requirement.Role) || requirement.MinimumAnchors <= 0 {
				return fmt.Errorf("task lens: invalid role requirement")
			}
			if _, duplicate := seen[requirement.Role]; duplicate {
				return fmt.Errorf("task lens: duplicate role requirement")
			}
			seen[requirement.Role] = struct{}{}
		}
	}
	return nil
}

// RoleCoverageItem binds one immutable requirement to the anchors that cover it.
type RoleCoverageItem struct {
	Role           AnchorRole `json:"role"`
	MinimumAnchors int        `json:"minimum_anchors"`
	AnchorIDs      []string   `json:"anchor_ids"`
	Represented    bool       `json:"represented"`
}

// RoleCoverage records coverage without allowing synthesis to edit the contract.
type RoleCoverage struct {
	Profile    TaskProfile        `json:"profile"`
	Key        []RoleCoverageItem `json:"key"`
	Supporting []RoleCoverageItem `json:"supporting"`
	Optional   []RoleCoverageItem `json:"optional"`
}

// EvaluateRoleCoverage evaluates exact role hints on retained anchors.
func EvaluateRoleCoverage(contract RoleContract, anchors []Anchor) (RoleCoverage, error) {
	if err := contract.Validate(); err != nil {
		return RoleCoverage{}, err
	}
	roleAnchors := make(map[AnchorRole][]string)
	for _, anchor := range anchors {
		for _, role := range anchor.RoleHints {
			if !validContractAnchorRole(role) {
				return RoleCoverage{}, fmt.Errorf("task lens: anchor has invalid contract role")
			}
			roleAnchors[role] = append(roleAnchors[role], anchor.ID)
		}
	}
	coverage := RoleCoverage{
		Profile:    contract.Profile,
		Key:        evaluateRoleRequirements(contract.Key, roleAnchors),
		Supporting: evaluateRoleRequirements(contract.Supporting, roleAnchors),
		Optional:   evaluateRoleRequirements(contract.Optional, roleAnchors),
	}
	return coverage, coverage.ValidateAgainst(contract)
}

// Validate checks that represented is a deterministic projection of exact IDs.
func (c RoleCoverage) Validate() error {
	if !validTaskProfile(c.Profile) {
		return fmt.Errorf("task lens: invalid role coverage profile")
	}
	seen := make(map[AnchorRole]struct{})
	groups := [][]RoleCoverageItem{c.Key, c.Supporting, c.Optional}
	for _, group := range groups {
		for _, item := range group {
			if !validContractAnchorRole(item.Role) || item.MinimumAnchors <= 0 {
				return fmt.Errorf("task lens: invalid role coverage item")
			}
			if _, duplicate := seen[item.Role]; duplicate {
				return fmt.Errorf("task lens: duplicate role coverage item")
			}
			seen[item.Role] = struct{}{}
			if !isUniqueSortedOpaque(item.AnchorIDs) ||
				item.Represented != (len(item.AnchorIDs) >= item.MinimumAnchors) {
				return fmt.Errorf("task lens: inconsistent role coverage")
			}
		}
	}
	return nil
}

// ValidateAgainst binds coverage to the exact pre-synthesis role contract.
func (c RoleCoverage) ValidateAgainst(contract RoleContract) error {
	if err := contract.Validate(); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Profile != contract.Profile ||
		!coverageMatchesRequirements(c.Key, contract.Key) ||
		!coverageMatchesRequirements(c.Supporting, contract.Supporting) ||
		!coverageMatchesRequirements(c.Optional, contract.Optional) {
		return fmt.Errorf("task lens: role coverage does not match its pre-synthesis contract")
	}
	return nil
}

// MissingKeyRoles returns a stable list of uncovered key roles.
func (c RoleCoverage) MissingKeyRoles() []AnchorRole {
	missing := make([]AnchorRole, 0, len(c.Key))
	for _, item := range c.Key {
		if !item.Represented {
			missing = append(missing, item.Role)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return missing
}

func roleRequirements(roles ...AnchorRole) []RoleRequirement {
	result := make([]RoleRequirement, 0, len(roles))
	for _, role := range roles {
		result = append(result, RoleRequirement{Role: role, MinimumAnchors: 1})
	}
	return result
}

func evaluateRoleRequirements(
	requirements []RoleRequirement,
	roleAnchors map[AnchorRole][]string,
) []RoleCoverageItem {
	result := make([]RoleCoverageItem, 0, len(requirements))
	for _, requirement := range requirements {
		anchorIDs := uniqueSorted(roleAnchors[requirement.Role])
		if anchorIDs == nil {
			anchorIDs = []string{}
		}
		result = append(result, RoleCoverageItem{
			Role:           requirement.Role,
			MinimumAnchors: requirement.MinimumAnchors,
			AnchorIDs:      anchorIDs,
			Represented:    len(anchorIDs) >= requirement.MinimumAnchors,
		})
	}
	return result
}

func coverageMatchesRequirements(
	coverage []RoleCoverageItem,
	requirements []RoleRequirement,
) bool {
	if len(coverage) != len(requirements) {
		return false
	}
	expected := make(map[AnchorRole]int, len(requirements))
	for _, requirement := range requirements {
		expected[requirement.Role] = requirement.MinimumAnchors
	}
	for _, item := range coverage {
		minimum, ok := expected[item.Role]
		if !ok || minimum != item.MinimumAnchors {
			return false
		}
	}
	return true
}

func validTaskProfile(profile TaskProfile) bool {
	switch profile {
	case TaskProfileDataTagTransformation,
		TaskProfileErrorStatusMapping,
		TaskProfileNilPanic,
		TaskProfileConfigurationPropagation,
		TaskProfileErrorNormalizationPrivacy,
		TaskProfileExtensionContribution,
		TaskProfileOperationalRelease,
		TaskProfileUnknown:
		return true
	default:
		return false
	}
}

func validContractAnchorRole(role AnchorRole) bool {
	if validAnchorRole(role) {
		return true
	}
	switch role {
	case RoleTransformation,
		RoleUnsafeOperation,
		RoleNilHandoff,
		RoleEffectiveDestination,
		RolePublicErrorType,
		RoleErrorNormalizer,
		RolePublicErrorExposure,
		RoleExtensionPort,
		RoleWiringComposition,
		RoleOperationalEntry,
		RoleProceduralBody,
		RoleModuleTopology,
		RoleSafetyCheck,
		RoleExample,
		RoleRepositoryVerificationCommand:
		return true
	default:
		return false
	}
}

// VerificationAuthority preserves the exact authority labels required by the
// verification frontier contract.
type VerificationAuthority string

const (
	VerificationExactExistingTest     VerificationAuthority = "exact_existing_test"
	VerificationExactGeneratedFixture VerificationAuthority = "exact_generated_fixture"
	VerificationExactExample          VerificationAuthority = "exact_example"
	VerificationDocumentedCommand     VerificationAuthority = "documented_command"
	VerificationProposedTestLocation  VerificationAuthority = "proposed_test_location"
	VerificationMissingEvidence       VerificationAuthority = "missing_evidence"
)

// VerificationItem is one bounded candidate or exact verification authority.
type VerificationItem struct {
	ID          string                `json:"id"`
	Authority   VerificationAuthority `json:"authority"`
	AnchorID    string                `json:"anchor_id,omitempty"`
	Path        string                `json:"path,omitempty"`
	Symbol      string                `json:"symbol,omitempty"`
	Text        string                `json:"text"`
	EvidenceIDs []string              `json:"evidence_ids"`
}

// VerificationFrontier keeps each bounded output class separately so its
// cardinality cannot be obscured in prose.
type VerificationFrontier struct {
	DecisiveAnchorID string             `json:"decisive_anchor_id,omitempty"`
	Anchors          []VerificationItem `json:"anchors"`
	Fixture          *VerificationItem  `json:"fixture,omitempty"`
	CommandOrEffect  *VerificationItem  `json:"command_or_effect,omitempty"`
}

// Validate enforces frontier bounds and authority-specific grounding.
func (f VerificationFrontier) Validate() error {
	if f.DecisiveAnchorID != "" && !validOpaque(f.DecisiveAnchorID) {
		return fmt.Errorf("task lens: invalid decisive anchor id")
	}
	if len(f.Anchors) > MaxVerificationAnchors {
		return fmt.Errorf("task lens: verification frontier exceeds anchor bound")
	}
	seen := make(map[string]struct{})
	for _, item := range f.Anchors {
		if item.Authority == VerificationExactGeneratedFixture ||
			item.Authority == VerificationDocumentedCommand {
			return fmt.Errorf("task lens: verification item is in the wrong frontier slot")
		}
		if err := validateVerificationItem(item, seen); err != nil {
			return err
		}
	}
	if f.Fixture != nil {
		if f.Fixture.Authority != VerificationExactGeneratedFixture {
			return fmt.Errorf("task lens: fixture has invalid verification authority")
		}
		if err := validateVerificationItem(*f.Fixture, seen); err != nil {
			return err
		}
	}
	if f.CommandOrEffect != nil {
		if f.CommandOrEffect.Authority != VerificationDocumentedCommand {
			return fmt.Errorf("task lens: command or effect has invalid verification authority")
		}
		if err := validateVerificationItem(*f.CommandOrEffect, seen); err != nil {
			return err
		}
	}
	return nil
}

// HasExactAnchorOrEffect reports whether a bounded, repository-grounded
// verification item exists. Proposed locations and missing evidence do not pass.
func (f VerificationFrontier) HasExactAnchorOrEffect() bool {
	if err := f.Validate(); err != nil || f.DecisiveAnchorID == "" {
		return false
	}
	for _, item := range f.allItems() {
		switch item.Authority {
		case VerificationExactExistingTest,
			VerificationExactGeneratedFixture,
			VerificationExactExample,
			VerificationDocumentedCommand:
			return true
		default:
			continue
		}
	}
	return false
}

// DefaultVerificationAuthorityOrder returns the required bounded search order.
func DefaultVerificationAuthorityOrder() []VerificationAuthority {
	return []VerificationAuthority{
		VerificationExactExistingTest,
		VerificationExactGeneratedFixture,
		VerificationExactExample,
		VerificationDocumentedCommand,
		VerificationProposedTestLocation,
		VerificationMissingEvidence,
	}
}

func (f VerificationFrontier) allItems() []VerificationItem {
	items := append([]VerificationItem{}, f.Anchors...)
	if f.Fixture != nil {
		items = append(items, *f.Fixture)
	}
	if f.CommandOrEffect != nil {
		items = append(items, *f.CommandOrEffect)
	}
	return items
}

func validateVerificationItem(item VerificationItem, seen map[string]struct{}) error {
	if !validOpaque(item.ID) || !validVerificationAuthority(item.Authority) ||
		!validText(item.Text, 2048, true) ||
		!validText(item.Symbol, 256, false) ||
		(item.Path != "" && !validPath(item.Path)) {
		return fmt.Errorf("task lens: invalid verification item")
	}
	if _, duplicate := seen[item.ID]; duplicate {
		return fmt.Errorf("task lens: duplicate verification item")
	}
	seen[item.ID] = struct{}{}
	if item.AnchorID != "" && !validOpaque(item.AnchorID) {
		return fmt.Errorf("task lens: invalid verification anchor id")
	}
	if !isUniqueOpaque(item.EvidenceIDs) {
		return fmt.Errorf("task lens: invalid verification evidence ids")
	}
	switch item.Authority {
	case VerificationExactExistingTest,
		VerificationExactGeneratedFixture,
		VerificationExactExample:
		if item.AnchorID == "" || len(item.EvidenceIDs) == 0 {
			return fmt.Errorf("task lens: exact verification item lacks anchor evidence")
		}
	case VerificationDocumentedCommand:
		if item.AnchorID == "" || len(item.EvidenceIDs) == 0 {
			return fmt.Errorf("task lens: documented verification lacks evidence")
		}
	case VerificationMissingEvidence:
		if item.AnchorID != "" || item.Path != "" || item.Symbol != "" || len(item.EvidenceIDs) != 0 {
			return fmt.Errorf("task lens: missing verification cannot claim repository evidence")
		}
	case VerificationProposedTestLocation:
		// A proposed location may cite a package path, but its authority remains
		// explicitly non-historical.
	}
	return nil
}

func validVerificationAuthority(authority VerificationAuthority) bool {
	switch authority {
	case VerificationExactExistingTest,
		VerificationExactGeneratedFixture,
		VerificationExactExample,
		VerificationDocumentedCommand,
		VerificationProposedTestLocation,
		VerificationMissingEvidence:
		return true
	default:
		return false
	}
}

// RelationKind is a bounded local relation type. None of these values alone
// establishes runtime reachability, order, or causality.
type RelationKind string

const (
	RelationDirectCall        RelationKind = "direct_call"
	RelationFieldCopy         RelationKind = "field_copy"
	RelationFieldRead         RelationKind = "field_read"
	RelationFieldWrite        RelationKind = "field_write"
	RelationErrorCreated      RelationKind = "error_created"
	RelationErrorMapped       RelationKind = "error_mapped"
	RelationErrorExposed      RelationKind = "error_exposed"
	RelationValueTransformed  RelationKind = "value_transformed"
	RelationTypeNameGenerated RelationKind = "type_name_generated"
	RelationConfigApplied     RelationKind = "config_applied"
	RelationScriptInvokes     RelationKind = "script_invokes"
	RelationTestExercises     RelationKind = "test_exercises"
	RelationFixtureRecords    RelationKind = "fixture_records"
	RelationDocumentedUses    RelationKind = "documented_uses"
	RelationSharedStateAlias  RelationKind = "shared_state_alias"
	RelationScopeUnknown      RelationKind = "scope_unknown"
)

func validRelationKind(kind RelationKind) bool {
	switch kind {
	case RelationDirectCall,
		RelationFieldCopy,
		RelationFieldRead,
		RelationFieldWrite,
		RelationErrorCreated,
		RelationErrorMapped,
		RelationErrorExposed,
		RelationValueTransformed,
		RelationTypeNameGenerated,
		RelationConfigApplied,
		RelationScriptInvokes,
		RelationTestExercises,
		RelationFixtureRecords,
		RelationDocumentedUses,
		RelationSharedStateAlias,
		RelationScopeUnknown:
		return true
	default:
		return false
	}
}

// CheapExitInput contains only deterministic local gate inputs.
type CheapExitInput struct {
	AreaIDs                       []string             `json:"area_ids"`
	MissingKeyRoles               []AnchorRole         `json:"missing_key_roles"`
	DecisiveRelationKind          RelationKind         `json:"decisive_relation_kind"`
	DecisiveRelationSupport       SupportType          `json:"decisive_relation_support"`
	Verification                  VerificationFrontier `json:"verification"`
	UnresolvedCompetingHypotheses int                  `json:"unresolved_competing_hypotheses"`
}

type CheapExitGate string

const (
	CheapExitGateUnambiguousArea       CheapExitGate = "unambiguous_area"
	CheapExitGateAllKeyRoles           CheapExitGate = "all_key_roles"
	CheapExitGateDecisiveLocalRelation CheapExitGate = "decisive_locally_observed_relation"
	CheapExitGateExactVerification     CheapExitGate = "exact_verification_anchor_or_effect"
	CheapExitGateNoCompetingHypothesis CheapExitGate = "no_unresolved_competing_hypothesis"
)

type CheapExitRoute string

const (
	CheapExitRouteZeroCall      CheapExitRoute = "zero_call"
	CheapExitRouteSynthesisCall CheapExitRoute = "single_synthesis_call"
)

type CheapExitGateResult struct {
	Gate   CheapExitGate `json:"gate"`
	Passed bool          `json:"passed"`
	Reason string        `json:"reason"`
}

// CheapExitDecision records every gate, not only the first failure.
type CheapExitDecision struct {
	Eligible bool                  `json:"eligible"`
	Route    CheapExitRoute        `json:"route"`
	Gates    []CheapExitGateResult `json:"gates"`
	Reasons  []string              `json:"reasons"`
}

// EvaluateCheapExit applies the five required zero-call gates in fixed order.
func EvaluateCheapExit(input CheapExitInput) CheapExitDecision {
	areaIDs := uniqueNonEmptySorted(input.AreaIDs)
	missingRoles := append([]AnchorRole(nil), input.MissingKeyRoles...)
	sort.Slice(missingRoles, func(i, j int) bool { return missingRoles[i] < missingRoles[j] })
	missingRoles = slices.Compact(missingRoles)

	areaPassed := len(areaIDs) == 1
	areaReason := fmt.Sprintf("found %d distinct candidate areas; exactly 1 is required", len(areaIDs))
	if areaPassed {
		areaReason = "exactly one candidate area is locally supported"
	}
	rolesPassed := len(missingRoles) == 0
	rolesReason := "all key roles are represented"
	if !rolesPassed {
		rolesReason = "missing key roles: " + joinAnchorRoles(missingRoles)
	}
	relationPassed := validRelationKind(input.DecisiveRelationKind) &&
		input.DecisiveRelationKind != RelationScopeUnknown &&
		input.DecisiveRelationSupport == SupportLocallyObserved
	relationReason := "one typed decisive relation is locally observed"
	if !relationPassed {
		relationReason = "no typed decisive locally observed relation is available"
	}
	verificationPassed := input.Verification.HasExactAnchorOrEffect()
	verificationReason := "one exact grounded verification anchor or effect is available"
	if !verificationPassed {
		verificationReason = "no exact grounded verification anchor or effect is available"
	}
	hypothesisPassed := input.UnresolvedCompetingHypotheses == 0
	hypothesisReason := "no unresolved competing hypothesis remains"
	if !hypothesisPassed {
		hypothesisReason = fmt.Sprintf(
			"found %d unresolved competing hypotheses; exactly 0 are required",
			input.UnresolvedCompetingHypotheses,
		)
	}

	gates := []CheapExitGateResult{
		{Gate: CheapExitGateUnambiguousArea, Passed: areaPassed, Reason: areaReason},
		{Gate: CheapExitGateAllKeyRoles, Passed: rolesPassed, Reason: rolesReason},
		{Gate: CheapExitGateDecisiveLocalRelation, Passed: relationPassed, Reason: relationReason},
		{Gate: CheapExitGateExactVerification, Passed: verificationPassed, Reason: verificationReason},
		{Gate: CheapExitGateNoCompetingHypothesis, Passed: hypothesisPassed, Reason: hypothesisReason},
	}
	decision := CheapExitDecision{
		Eligible: true,
		Route:    CheapExitRouteZeroCall,
		Gates:    gates,
		Reasons:  []string{},
	}
	for _, gate := range gates {
		if gate.Passed {
			continue
		}
		decision.Eligible = false
		decision.Reasons = append(decision.Reasons, gate.Reason)
	}
	if decision.Eligible {
		decision.Reasons = append(decision.Reasons, "all deterministic cheap-exit gates passed")
		return decision
	}
	decision.Route = CheapExitRouteSynthesisCall
	return decision
}

func joinAnchorRoles(roles []AnchorRole) string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	return strings.Join(values, ", ")
}

func uniqueNonEmptySorted(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return slices.Compact(result)
}

func isUniqueOpaque(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validOpaque(value) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func isUniqueSortedOpaque(values []string) bool {
	return isUniqueOpaque(values) && slices.IsSorted(values)
}
