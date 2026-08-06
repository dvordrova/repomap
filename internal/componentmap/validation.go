package componentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	PreferredPrimarySubsystems   = 7
	MaxPrimarySubsystems         = 12
	PreferredComponentsPerSystem = 4
	MaxComponentsPerSubsystem    = 24
	PreferredTotalComponents     = 18
	MaxTotalNestedComponents     = 48
)

func newDiagnostic(code, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: diagnosticSeverity(code), Message: message}
}

func diagnosticSeverity(code string) FindingSeverity {
	switch code {
	case "proposal.unsupported_version",
		"proposal.invalid_subsystem_count",
		"proposal.invalid_component_count",
		"proposal.invalid_members",
		"proposal.invalid_member_id",
		"proposal.membership_limit_exceeded",
		"proposal.member_participation_limit_exceeded",
		"proposal.incomplete_member_coverage",
		"proposal.empty_member_coverage",
		"proposal.invalid_subsystem",
		"proposal.invalid_component",
		"proposal.no_usable_subsystems",
		"proposal.omitted_members_exceed_bounds",
		"proposal.ungrounded_primary_component",
		"response.no_json",
		"response.ambiguous_json",
		"response.invalid_proposal",
		"response.too_large",
		"response.sensitive_omitted":
		return FindingFatal
	case "proposal.normalized_primary_subsystems",
		"proposal.normalized_components_per_subsystem",
		"proposal.normalized_total_components",
		"proposal.normalized_package_only_hypothesis",
		"proposal.normalized_description",
		// Decision 229 D7 item-scope salvage classes: the referencing
		// component is dropped or locally normalized, valid siblings
		// publish as accepted_partial — never a whole-stage rejection.
		"proposal.unknown_member_id",
		"proposal.unknown_anchor_id",
		"proposal.duplicate_member_id",
		"proposal.duplicate_component_identity",
		// Decision 229 D7: ref-resolution failures during wire
		// resolution drop only the referencing component.
		"proposal.unknown_unit_ref",
		"proposal.duplicate_unit_ref",
		"proposal.duplicate_anchor_id",
		// Decision 229 D7: a subsystem emptied by item-scope salvage is
		// skipped with a counted recoverable finding, never a fatal.
		"proposal.salvaged_empty_subsystem",
		// Decision 230 D9.7: repeated broad unit reduced to its
		// anchor-specific slice is a recoverable normalization; an empty
		// slice drops only the referencing component.
		"proposal.shared_unit_slice",
		"proposal.empty_anchor_slice":
		return FindingRecoverable
	default:
		return FindingAdvisory
	}
}

func proposalSHA256(proposal Proposal) string {
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func normalizeProposalShape(bundle CandidateBundle, proposal Proposal) (Proposal, []NormalizationOperation, []Diagnostic) {
	normalized := cloneProposal(proposal)
	annotateProposalSources(&normalized)
	operations := make([]NormalizationOperation, 0)
	findings := make([]Diagnostic, 0)

	for index := range normalized.Subsystems {
		subsystem := &normalized.Subsystems[index]
		if len(subsystem.Description) > maxDescriptionBytes {
			subsystem.Description = truncateDisplayText(subsystem.Description, maxDescriptionBytes)
			operations = append(operations, NormalizationOperation{
				Code: "normalized_verbose_description", Message: "trimmed an overlong subsystem description",
				SourceSubsystemIDs: append([]SubsystemID(nil), subsystem.sourceIDs...),
			})
			findings = append(findings, newDiagnostic("proposal.normalized_description", "trimmed an overlong subsystem description"))
		}
		for componentIndex := range subsystem.Components {
			component := &subsystem.Components[componentIndex]
			if len(component.Description) > maxDescriptionBytes {
				component.Description = truncateDisplayText(component.Description, maxDescriptionBytes)
				operations = append(operations, NormalizationOperation{
					Code: "normalized_verbose_description", Message: "trimmed an overlong component description",
					SourceComponentIDs: append([]ComponentID(nil), component.sourceIDs...),
				})
				findings = append(findings, newDiagnostic("proposal.normalized_description", "trimmed an overlong component description"))
			}
		}
	}

	if len(normalized.Subsystems) > PreferredPrimarySubsystems {
		findings = append(findings, newDiagnostic(
			"proposal.primary_subsystems_above_preferred",
			"proposal has more than seven primary subsystems but remains within the readable hard bound",
		))
	}
	if len(normalized.Subsystems) > MaxPrimarySubsystems {
		extra := append([]ProposedSubsystem(nil), normalized.Subsystems[MaxPrimarySubsystems-1:]...)
		merged := mergeProposedSubsystems(extra)
		normalized.Subsystems = append(normalized.Subsystems[:MaxPrimarySubsystems-1], merged)
		operations = append(operations, NormalizationOperation{
			Code: "normalized_excess_primary_subsystems", Message: "merged excess primary subsystems into one additional-responsibilities pillar",
			SourceSubsystemIDs: append([]SubsystemID(nil), merged.sourceIDs...),
		})
		findings = append(findings, newDiagnostic(
			"proposal.normalized_primary_subsystems",
			"merged primary subsystems above the hard maximum of eight",
		))
	}

	for index := range normalized.Subsystems {
		subsystem := &normalized.Subsystems[index]
		if len(subsystem.Components) > PreferredComponentsPerSystem {
			findings = append(findings, newDiagnostic(
				"proposal.components_per_subsystem_above_preferred",
				"a subsystem has more than four nested responsibilities but remains within the readable hard bound",
			))
		}
		if len(subsystem.Components) <= MaxComponentsPerSubsystem {
			continue
		}
		extra := append([]ProposedComponent(nil), subsystem.Components[MaxComponentsPerSubsystem-1:]...)
		merged := mergeProposedComponents(extra)
		subsystem.Components = append(subsystem.Components[:MaxComponentsPerSubsystem-1], merged)
		operations = append(operations, NormalizationOperation{
			Code:               "normalized_excess_components_per_subsystem",
			Message:            "merged excess nested responsibilities within one subsystem",
			SourceSubsystemIDs: append([]SubsystemID(nil), subsystem.sourceIDs...),
			SourceComponentIDs: append([]ComponentID(nil), merged.sourceIDs...),
		})
		findings = append(findings, newDiagnostic(
			"proposal.normalized_components_per_subsystem",
			"merged nested responsibilities above the per-subsystem hard maximum",
		))
	}

	total := proposalComponentCount(normalized)
	if total > PreferredTotalComponents {
		findings = append(findings, newDiagnostic(
			"proposal.total_components_above_preferred",
			"proposal has more than eighteen nested responsibilities",
		))
	}
	for total > MaxTotalNestedComponents {
		subsystemIndex := subsystemWithMostComponents(normalized.Subsystems)
		if subsystemIndex < 0 {
			break
		}
		subsystem := &normalized.Subsystems[subsystemIndex]
		last := len(subsystem.Components) - 1
		merged := mergeProposedComponents(subsystem.Components[last-1:])
		subsystem.Components = append(subsystem.Components[:last-1], merged)
		operations = append(operations, NormalizationOperation{
			Code:               "normalized_excess_total_components",
			Message:            "merged low-priority nested responsibilities to satisfy the total component bound",
			SourceSubsystemIDs: append([]SubsystemID(nil), subsystem.sourceIDs...),
			SourceComponentIDs: append([]ComponentID(nil), merged.sourceIDs...),
		})
		total--
	}
	if proposalComponentCount(proposal) > MaxTotalNestedComponents {
		findings = append(findings, newDiagnostic(
			"proposal.normalized_total_components",
			"merged nested responsibilities above the total hard maximum",
		))
	}
	return normalized, operations, findings
}

func knownPackageOnlyMembers(known map[MemberID]Candidate, memberIDs []MemberID) bool {
	if len(memberIDs) == 0 {
		return false
	}
	for _, memberID := range memberIDs {
		if memberID.Kind != MemberPackage {
			return false
		}
		if _, exists := known[memberID]; !exists {
			return false
		}
	}
	return true
}

func cloneProposal(proposal Proposal) Proposal {
	result := Proposal{Version: proposal.Version, Subsystems: make([]ProposedSubsystem, len(proposal.Subsystems))}
	for subsystemIndex, subsystem := range proposal.Subsystems {
		result.Subsystems[subsystemIndex] = ProposedSubsystem{
			Name: subsystem.Name, Description: subsystem.Description,
			Components: make([]ProposedComponent, len(subsystem.Components)),
			sourceIDs:  append([]SubsystemID(nil), subsystem.sourceIDs...),
		}
		for componentIndex, component := range subsystem.Components {
			result.Subsystems[subsystemIndex].Components[componentIndex] = ProposedComponent{
				Name: component.Name, Description: component.Description,
				MemberIDs:  append([]MemberID(nil), component.MemberIDs...),
				AnchorIDs:  append([]string(nil), component.AnchorIDs...),
				Hypothesis: component.Hypothesis,
				sourceIDs:  append([]ComponentID(nil), component.sourceIDs...),
			}
		}
	}
	return result
}

func annotateProposalSources(proposal *Proposal) {
	for subsystemIndex := range proposal.Subsystems {
		subsystem := &proposal.Subsystems[subsystemIndex]
		componentIDs := make([]ComponentID, 0, len(subsystem.Components))
		for componentIndex := range subsystem.Components {
			component := &subsystem.Components[componentIndex]
			id := componentID(component.MemberIDs)
			component.sourceIDs = []ComponentID{id}
			componentIDs = append(componentIDs, id)
		}
		subsystem.sourceIDs = []SubsystemID{subsystemID(componentIDs)}
	}
}

func mergeProposedSubsystems(subsystems []ProposedSubsystem) ProposedSubsystem {
	result := ProposedSubsystem{
		Name:        "Additional responsibilities",
		Description: "Additional grounded responsibilities retained after local hierarchy normalization.",
	}
	for _, subsystem := range subsystems {
		result.Components = append(result.Components, subsystem.Components...)
		result.sourceIDs = append(result.sourceIDs, subsystem.sourceIDs...)
	}
	return result
}

func mergeProposedComponents(components []ProposedComponent) ProposedComponent {
	result := ProposedComponent{
		Name:        "Additional responsibilities",
		Description: "Grounded nested responsibilities combined by deterministic local normalization.",
		Hypothesis:  true,
	}
	memberSet := make(map[MemberID]struct{})
	anchorSet := make(map[string]struct{})
	for _, component := range components {
		if !component.Hypothesis {
			result.Hypothesis = false
		}
		for _, memberID := range component.MemberIDs {
			memberSet[memberID] = struct{}{}
		}
		for _, anchorID := range component.AnchorIDs {
			anchorSet[anchorID] = struct{}{}
		}
		result.sourceIDs = append(result.sourceIDs, component.sourceIDs...)
	}
	for memberID := range memberSet {
		result.MemberIDs = append(result.MemberIDs, memberID)
	}
	sort.Slice(result.MemberIDs, func(i, j int) bool { return result.MemberIDs[i].key() < result.MemberIDs[j].key() })
	for anchorID := range anchorSet {
		result.AnchorIDs = append(result.AnchorIDs, anchorID)
	}
	sort.Strings(result.AnchorIDs)
	sort.Slice(result.sourceIDs, func(i, j int) bool { return result.sourceIDs[i] < result.sourceIDs[j] })
	return result
}

func proposalComponentCount(proposal Proposal) int {
	total := 0
	for _, subsystem := range proposal.Subsystems {
		total += len(subsystem.Components)
	}
	return total
}

func subsystemWithMostComponents(subsystems []ProposedSubsystem) int {
	selected := -1
	for index, subsystem := range subsystems {
		if len(subsystem.Components) < 2 {
			continue
		}
		if selected < 0 || len(subsystem.Components) > len(subsystems[selected].Components) {
			selected = index
		}
	}
	return selected
}

func truncateDisplayText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	cut := limit - len("...")
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.TrimSpace(value[:cut]) + "..."
}

func fallbackReasonForDiagnostics(diagnostics []Diagnostic, hasAnchors bool) FallbackReason {
	for _, diagnostic := range diagnostics {
		// Decision 229 D7: item-scope salvage classes are recoverable, but
		// when EVERY component was dropped (zero independently valid items
		// remain) the exact original reason must surface in the fallback —
		// never a generic malformed-schema label.
		if diagnostic.Severity == FindingFatal || diagnostic.Severity == FindingRecoverable {
			switch diagnostic.Code {
			case "proposal.unknown_member_id":
				return FallbackRejectedUnknownMember
			case "proposal.unknown_anchor_id":
				return FallbackRejectedUnknownAnchor
			case "proposal.ungrounded_primary_component":
				return FallbackRejectedUngrounded
			}
		}
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != FindingFatal {
			continue
		}
		switch diagnostic.Code {
		case "proposal.unknown_member_id":
			return FallbackRejectedUnknownMember
		case "proposal.unknown_anchor_id":
			return FallbackRejectedUnknownAnchor
		case "proposal.ungrounded_primary_component":
			return FallbackRejectedUngrounded
		default:
			return FallbackRejectedMalformed
		}
	}
	if hasAnchors {
		return FallbackAnchorFirst
	}
	return FallbackInsufficientAnchors
}

func hasFatalDiagnostics(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == FindingFatal {
			return true
		}
	}
	return false
}

// landscapeHasItemScopeSalvage reports whether the diagnostics carry any
// Decision 229 D7 item-scope salvage class — a recoverable finding that
// dropped or normalized a specific component while valid siblings publish.
func landscapeHasItemScopeSalvage(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case "proposal.unknown_member_id",
			"proposal.unknown_anchor_id",
			"proposal.duplicate_member_id",
			"proposal.duplicate_component_identity",
			"proposal.unknown_unit_ref",
			"proposal.duplicate_unit_ref",
			"proposal.duplicate_anchor_id",
			"proposal.salvaged_empty_subsystem":
			if diagnostic.Severity == FindingRecoverable {
				return true
			}
		}
	}
	return false
}

func validFallbackReason(reason FallbackReason) bool {
	switch reason {
	case FallbackProposalInvalid, FallbackRejectedMalformed, FallbackRejectedUnknownMember,
		FallbackRejectedUnknownAnchor, FallbackRejectedOwnership, FallbackRejectedUngrounded,
		FallbackAnchorFirst, FallbackInsufficientAnchors, FallbackPackageLandscape,
		FallbackModelDisabled, FallbackProviderUnconfigured:
		return true
	default:
		return false
	}
}

func validValidationOutcome(outcome ValidationOutcome) bool {
	return outcome == ValidationAccepted || outcome == ValidationAcceptedPartial ||
		outcome == ValidationAcceptedNormalized || outcome == ValidationRejected
}

func validArchitectureSource(source ArchitectureSource) bool {
	return source == SourceValidatedModel || source == SourcePartialModel || source == SourceNormalizedModel ||
		source == SourceLocalAnchors || source == SourceLocalPackages ||
		source == SourcePackageFallback
}

func validateNormalizationOperation(operation NormalizationOperation) error {
	if err := validateOpaqueText("normalization code", operation.Code, maxOpaqueIDBytes); err != nil {
		return err
	}
	if err := validateDisplayText("normalization message", operation.Message, maxDescriptionBytes, true); err != nil {
		return err
	}
	if len(operation.SourceSubsystemIDs) == 0 && len(operation.SourceComponentIDs) == 0 {
		return fmt.Errorf("normalization operation has no source identities")
	}
	return nil
}

func useAnchorFirstLocalGrouping(bundle CandidateBundle) bool {
	if len(bundle.BehaviorAnchors) == 0 || bundle.GroundingMode == GroundingPackages {
		return false
	}
	kinds := make(map[BehaviorAnchorKind]struct{}, len(bundle.BehaviorAnchors))
	for _, anchor := range bundle.BehaviorAnchors {
		if anchor.ProofMode != AnchorProofDeclarationFamily && anchor.Kind != AnchorUnresolvedFrontier {
			kinds[anchor.Kind] = struct{}{}
		}
	}
	return len(kinds) >= 2
}
