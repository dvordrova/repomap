package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
)

const (
	ArchitectureSynthesisStatusFile = "architecture_synthesis_status.json"
	// Decision 231 (Archive 9): shared participation changed acceptance
	// semantics (zero useful semantic components → honest local-only with
	// the exact reason code; shared_unit_slice repurposed to recoverable).
	// Decision 235 (v11): normalization/empty-item semantics (status 11).
	// Decision 237: failed-status cause, exchange reference, and bounded
	// response-shape evidence (status 12).
	// Decision 238: backend-owned primary/supporting coverage evidence
	// (status 13).
	// Decision 239: primary/supporting counters become production-aware
	// (status 14).
	// Decision 241: item-local empty/supporting-only response salvage and its
	// closed diagnostics change accepted/partial status semantics (status 15).
	// Status 16 admits an exact all-supporting non-production scope. Such a
	// request has no primary_scope candidates by construction, so its accepted
	// membership is accounted entirely as supporting evidence instead of being
	// rejected for evidence the request could not contain.
	// Status 17 keeps model membership exact while allowing backend-owned
	// parent context to satisfy primary-scope coverage. A returned nested
	// member remains the only model-authored member; its known parent may add
	// one covered primary scope without increasing DistinctMembers.
	ArchitectureSynthesisStatusVersion = 17

	ArchitectureSynthesisSucceeded   = "succeeded"
	ArchitectureSynthesisCached      = "cached"
	ArchitectureSynthesisFailed      = "failed"
	ArchitectureSynthesisUnavailable = "unavailable"

	ArchitectureSynthesisUnavailableOfflineCode             = "offline"
	ArchitectureSynthesisUnavailableExactWorkspaceGraphCode = "exact_workspace_graph_unavailable"

	// ArchitectureSynthesisErrorProviderOutputLimit is the closed failed
	// status code for an attempted Architecture provider call that exhausted
	// its output-token or response-byte budget (Decision 215). The partial
	// response is diagnostic-only; the exact local Canvas remains the visible
	// Architecture.
	ArchitectureSynthesisErrorProviderOutputLimit = "provider_output_limit"
)

// architectureStatusValidationCodes is the complete diagnostic vocabulary
// that the active Architecture response evaluator and its status evidence
// checks can persist. Keep this boundary closed: accepting merely well-formed
// opaque strings would make a current status claim validation evidence that
// the active evaluator cannot produce. Resource-limit outcomes are terminal
// errors and therefore are deliberately not represented as validation codes
// here.
var architectureStatusValidationCodes = map[string]struct{}{
	// Locally applied proposal validation and normalization diagnostics.
	"proposal.components_per_subsystem_above_preferred": {},
	"proposal.duplicate_component_identity":             {},
	"proposal.duplicate_member_id":                      {},
	"proposal.invalid_component":                        {},
	"proposal.invalid_member_id":                        {},
	"proposal.invalid_members":                          {},
	"proposal.incomplete_member_coverage":               {},
	"proposal.member_participation_limit_exceeded":      {},
	"proposal.membership_limit_exceeded":                {},
	"proposal.invalid_subsystem":                        {},
	"proposal.invalid_subsystem_count":                  {},
	"proposal.no_usable_subsystems":                     {},
	"proposal.normalized_components_per_subsystem":      {},
	"proposal.normalized_description":                   {},
	"proposal.normalized_package_only_hypothesis":       {},
	"proposal.normalized_primary_subsystems":            {},
	// Phase 1 prompt cleanup: a component without an anchor_refs field is
	// a mechanical default, counted as a normalization (nested grammar).
	"proposal.normalized_missing_anchor_refs":     {},
	"proposal.normalized_total_components":        {},
	"proposal.partial_member_coverage":            {},
	"proposal.empty_primary_scope_coverage":       {},
	"proposal.supporting_only_unit_coverage":      {},
	"proposal.primary_subsystems_above_preferred": {},
	"proposal.total_components_above_preferred":   {},
	"proposal.ungrounded_primary_component":       {},
	"proposal.unknown_anchor_id":                  {},
	"proposal.unknown_member_id":                  {},
	"proposal.unsupported_version":                {},
	// Decision 229 D7 item-scope salvage vocabulary: recoverable
	// findings emitted while dropping only the referencing component.
	"proposal.salvaged_empty_subsystem":               {},
	"proposal.unknown_unit_ref":                       {},
	"proposal.duplicate_unit_ref":                     {},
	"proposal.duplicate_anchor_id":                    {},
	"proposal.empty_member_coverage":                  {},
	"proposal.equivalent_member_set_collision":        {},
	"proposal.shared_unit_slice":                      {},
	"proposal.empty_anchor_slice":                     {},
	"proposal.empty_component":                        {},
	"proposal.supporting_only_unit_coverage_salvaged": {},
	// Decision 231 (Archive 9): zero-useful-semantic result — the exact
	// local landscape publishes with this recoverable finding instead of
	// a generic malformed-schema label.
	"proposal.zero_useful_semantic_components": {},

	// Provider response extraction and request-local reference diagnostics.
	"response.ambiguous_json":    {},
	"response.invalid_proposal":  {},
	"response.no_json":           {},
	"response.sensitive_omitted": {},

	// Exact response-evidence checks owned by the Architecture stage runner.
	"response.incomplete":             {},
	"response.membership_unavailable": {},
	"response.not_captured":           {},
	"status.invalid_evidence":         {},

	// Decision 235 (v11): bounded deterministic normalization diagnostics —
	// the run continues with the normalized response, never a rejection.
	"response.trailing_closing_delimiters_normalized": {},
	// Archive 12 P0 (etcd): bounded response membership — the ceiling is a
	// normalization (trimmed deterministically, members stay in the local
	// remainder), never a rejection.
	"response.member_refs_per_component_ceiling": {},
	"response.member_refs_total_ceiling":         {},
}

// Versions through v5 used exclusive membership, deterministic remainder,
// and permissive JSON extraction diagnostics. Historical artifacts remain
// readable, but a v6 writer cannot claim these retired outcomes.
var architectureStatusHistoricalValidationCodes = map[string]struct{}{
	"proposal.conflicting_membership":        {},
	"proposal.omitted_members_exceed_bounds": {},
	"proposal.omitted_members_preserved":     {},
	"proposal.omitted_process_entry_member":  {},
	"response.embedded_json_extracted":       {},
	"response.fenced_json_extracted":         {},
	"response.unknown_fields_ignored":        {},
}

// ArchitectureSynthesisStatus records whether the optional conceptual
// grouping request produced a usable result. It contains only bounded
// operational metadata; prompts and provider responses remain in their own
// debug artifacts.
type ArchitectureSynthesisStatus struct {
	Version int    `json:"version"`
	State   string `json:"state"`
	// PromptBytes is retained only for reading status versions 1-3. Version 4
	// names the exact provider body truthfully as RequestBytes.
	PromptBytes          int   `json:"prompt_bytes,omitempty"`
	RequestBytes         int   `json:"request_bytes,omitempty"`
	ResponseBytes        int   `json:"response_bytes,omitempty"`
	ResponseContentBytes int   `json:"response_content_bytes,omitempty"`
	LatencyMillis        int64 `json:"latency_ms,omitempty"`
	ProviderRequestCount int   `json:"provider_request_count,omitempty"`
	TransportAttempts    int   `json:"transport_attempts,omitempty"`
	// CandidateCount is retained only for reading status versions 1-6. Version
	// 7 separates the complete local candidate set from the smaller exact set
	// of conceptual members requested from the provider.
	CandidateCount                 int                     `json:"candidate_count,omitempty"`
	LocalCandidateCount            int                     `json:"local_candidate_count,omitempty"`
	RequestedConceptualCount       int                     `json:"requested_conceptual_count,omitempty"`
	StructuralLocatorCount         int                     `json:"structural_locator_count,omitempty"`
	AnchorCount                    int                     `json:"anchor_count,omitempty"`
	MembershipCounted              bool                    `json:"response_membership_counted,omitempty"`
	MemberOccurrences              int                     `json:"response_member_occurrences,omitempty"`
	DistinctMembers                int                     `json:"response_distinct_members,omitempty"`
	CoveredConceptualCount         int                     `json:"covered_conceptual_count,omitempty"`
	UncoveredConceptualCount       int                     `json:"uncovered_conceptual_count,omitempty"`
	UncoveredConceptualIDs         []componentmap.MemberID `json:"uncovered_conceptual_member_ids,omitempty"`
	RequestedPrimaryScopeCount     int                     `json:"requested_primary_scope_count,omitempty"`
	CoveredPrimaryScopeCount       int                     `json:"covered_primary_scope_count,omitempty"`
	UncoveredPrimaryScopeCount     int                     `json:"uncovered_primary_scope_count,omitempty"`
	CoveredSupportingEvidenceCount int                     `json:"covered_supporting_evidence_count,omitempty"`
	UsageReported                  bool                    `json:"usage_reported,omitempty"`
	InputTokens                    int                     `json:"input_tokens,omitempty"`
	OutputTokens                   int                     `json:"output_tokens,omitempty"`
	// ConfiguredMaxTokens and ObservedOutputTokens carry the bounded output
	// envelope evidence for the failed provider_output_limit state (Decision
	// 215): the exact configured global ceiling and the observed completion
	// token count that exhausted it. They are never populated for accepted,
	// cached, unavailable, or other failed states.
	ConfiguredMaxTokens   int                                 `json:"configured_max_tokens,omitempty"`
	ObservedOutputTokens  int                                 `json:"observed_output_tokens,omitempty"`
	FinishReason          string                              `json:"finish_reason,omitempty"`
	ResponseComplete      bool                                `json:"response_complete,omitempty"`
	ResponseState         string                              `json:"response_state,omitempty"`
	ValidationCodes       []string                            `json:"validation_diagnostic_codes,omitempty"`
	ProviderCallSucceeded bool                                `json:"provider_call_succeeded,omitempty"`
	ResponseParsed        bool                                `json:"response_parsed,omitempty"`
	ProposalAccepted      bool                                `json:"proposal_accepted,omitempty"`
	ProposalPartial       bool                                `json:"proposal_partial,omitempty"`
	ProposalNormalized    bool                                `json:"proposal_normalized,omitempty"`
	ProposalRejected      bool                                `json:"proposal_rejected,omitempty"`
	FallbackSelected      bool                                `json:"fallback_selected,omitempty"`
	ArchitectureSource    string                              `json:"architecture_source,omitempty"`
	ArchitectureLevel     int                                 `json:"architecture_level,omitempty"`
	NormalizationCount    int                                 `json:"normalization_count,omitempty"`
	FallbackReason        string                              `json:"fallback_reason,omitempty"`
	ErrorCode             string                              `json:"error_code,omitempty"`
	UnavailableCode       string                              `json:"unavailable_code,omitempty"`
	Failure               *ArchitectureSynthesisFailure       `json:"failure,omitempty"`
	ResponseShape         *ArchitectureSynthesisResponseShape `json:"response_shape,omitempty"`
	SemanticExchangePath  string                              `json:"semantic_exchange_path,omitempty"`
}

type ArchitectureSynthesisFailure struct {
	Stage  string `json:"stage"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type ArchitectureSynthesisResponseShape struct {
	JSONValid              bool   `json:"json_valid"`
	Grammar                string `json:"grammar,omitempty"`
	SubsystemCount         int    `json:"subsystem_count,omitempty"`
	ComponentCount         int    `json:"component_count,omitempty"`
	MemberRefCount         int    `json:"member_ref_count,omitempty"`
	UnitRefCount           int    `json:"unit_ref_count,omitempty"`
	AnchorRefCount         int    `json:"anchor_ref_count,omitempty"`
	MissingAnchorRefsCount int    `json:"missing_anchor_refs_count,omitempty"`
	EmptyAnchorRefsCount   int    `json:"explicit_empty_anchor_refs_count,omitempty"`
	NullAnchorRefsCount    int    `json:"null_anchor_refs_count,omitempty"`
}

func (status ArchitectureSynthesisStatus) Validate() error {
	if status.Version < 1 || status.Version > ArchitectureSynthesisStatusVersion {
		return fmt.Errorf("unsupported architecture synthesis status version %d", status.Version)
	}
	switch status.State {
	case ArchitectureSynthesisSucceeded, ArchitectureSynthesisCached:
		if status.ErrorCode != "" || status.UnavailableCode != "" || status.Failure != nil {
			return fmt.Errorf("successful architecture synthesis status cannot contain an error code")
		}
	case ArchitectureSynthesisFailed:
		if status.ErrorCode == "" || status.UnavailableCode != "" {
			return fmt.Errorf("failed architecture synthesis status requires an error code")
		}
		if status.ErrorCode == ArchitectureSynthesisErrorProviderOutputLimit {
			if err := status.validateProviderOutputLimitEvidence(); err != nil {
				return err
			}
		}
		if status.Version >= 12 && status.Failure == nil {
			return fmt.Errorf("failed architecture synthesis status requires a failure diagnostic")
		}
	case ArchitectureSynthesisUnavailable:
		if status.Version < 3 || !validArchitectureUnavailableCode(status.UnavailableCode) || status.ErrorCode != "" ||
			status.PromptBytes != 0 || status.LatencyMillis != 0 || status.ProviderRequestCount != 0 ||
			status.ProviderCallSucceeded || status.ResponseParsed || status.ProposalAccepted ||
			status.ProposalPartial || status.ProposalNormalized || status.ProposalRejected || status.FallbackSelected ||
			status.ArchitectureSource != "" || status.ArchitectureLevel != 0 ||
			status.NormalizationCount != 0 || status.FallbackReason != "" ||
			status.RequestBytes != 0 || status.ResponseBytes != 0 ||
			status.ResponseContentBytes != 0 || status.TransportAttempts != 0 ||
			status.CandidateCount != 0 || status.LocalCandidateCount != 0 ||
			status.RequestedConceptualCount != 0 || status.StructuralLocatorCount != 0 ||
			status.AnchorCount != 0 ||
			status.MembershipCounted || status.MemberOccurrences != 0 ||
			status.DistinctMembers != 0 || status.CoveredConceptualCount != 0 ||
			status.UncoveredConceptualCount != 0 || len(status.UncoveredConceptualIDs) != 0 ||
			status.RequestedPrimaryScopeCount != 0 || status.CoveredPrimaryScopeCount != 0 ||
			status.UncoveredPrimaryScopeCount != 0 || status.CoveredSupportingEvidenceCount != 0 ||
			status.UsageReported ||
			status.InputTokens != 0 || status.OutputTokens != 0 ||
			status.ConfiguredMaxTokens != 0 || status.ObservedOutputTokens != 0 ||
			status.FinishReason != "" || status.ResponseComplete ||
			status.ResponseState != "" || len(status.ValidationCodes) != 0 ||
			status.Failure != nil || status.ResponseShape != nil || status.SemanticExchangePath != "" {
			return fmt.Errorf("unavailable architecture synthesis status is inconsistent")
		}
		return nil
	default:
		return fmt.Errorf("unsupported architecture synthesis state %q", status.State)
	}
	if status.Version < 12 && (status.Failure != nil || status.ResponseShape != nil || status.SemanticExchangePath != "") {
		return fmt.Errorf("historical architecture synthesis status cannot use v12 diagnostics")
	}
	if status.Failure != nil {
		if err := validateArchitectureSynthesisFailure(*status.Failure); err != nil {
			return err
		}
	}
	if status.ResponseShape != nil {
		if err := status.ResponseShape.Validate(); err != nil {
			return err
		}
		if status.ResponseBytes == 0 {
			return fmt.Errorf("architecture response shape requires response byte evidence")
		}
	}
	if status.SemanticExchangePath != "" && !validArchitectureSemanticExchangePath(status.SemanticExchangePath) {
		return fmt.Errorf("architecture semantic exchange path is invalid")
	}
	if status.PromptBytes < 0 || status.RequestBytes < 0 || status.ResponseBytes < 0 ||
		status.ResponseContentBytes < 0 || status.LatencyMillis < 0 ||
		status.ProviderRequestCount < 0 || status.ProviderRequestCount > 1 ||
		status.TransportAttempts < 0 || status.CandidateCount < 0 ||
		status.LocalCandidateCount < 0 || status.RequestedConceptualCount < 0 ||
		status.StructuralLocatorCount < 0 ||
		status.AnchorCount < 0 || status.MemberOccurrences < 0 ||
		status.DistinctMembers < 0 || status.CoveredConceptualCount < 0 ||
		status.UncoveredConceptualCount < 0 || status.RequestedPrimaryScopeCount < 0 ||
		status.CoveredPrimaryScopeCount < 0 || status.UncoveredPrimaryScopeCount < 0 ||
		status.CoveredSupportingEvidenceCount < 0 || status.InputTokens < 0 || status.OutputTokens < 0 {
		return fmt.Errorf("architecture synthesis status contains invalid metrics")
	}
	if err := status.validatePrimaryScopeCoverage(); err != nil {
		return err
	}
	if status.Version == 1 {
		return nil
	}
	if status.Version >= 4 {
		if err := status.validateResolvedMembershipEvidence(); err != nil {
			return err
		}
	}
	if status.ArchitectureLevel < 0 || status.ArchitectureLevel > 4 || status.NormalizationCount < 0 {
		return fmt.Errorf("architecture synthesis status contains invalid outcome metrics")
	}
	if status.ProposalNormalized && !status.ProposalAccepted {
		return fmt.Errorf("normalized architecture proposal must also be accepted")
	}
	if status.ProposalPartial && !status.ProposalAccepted {
		return fmt.Errorf("partial architecture proposal must also be accepted")
	}
	if status.ProposalAccepted && status.ProposalRejected {
		return fmt.Errorf("architecture proposal cannot be accepted and rejected")
	}
	if status.State == ArchitectureSynthesisFailed {
		if status.Version >= 3 && (status.ProposalAccepted || status.ProposalPartial || status.ProposalNormalized ||
			status.FallbackSelected || status.FallbackReason != "" ||
			status.ArchitectureSource != "" || status.ArchitectureLevel != 0 ||
			status.NormalizationCount != 0) {
			return fmt.Errorf("failed architecture synthesis status cannot publish enrichment")
		}
		if status.Version >= 12 {
			if err := status.validateFailureEvidence(); err != nil {
				return err
			}
		}
		return nil
	}
	if status.Version >= 3 {
		expectedRequests := 1
		if status.State == ArchitectureSynthesisCached {
			expectedRequests = 0
		}
		if status.ProviderRequestCount != expectedRequests || !status.ProposalAccepted ||
			status.ProposalRejected || status.FallbackSelected || status.FallbackReason != "" {
			return fmt.Errorf("accepted architecture synthesis status is inconsistent")
		}
	}
	if !status.ProviderCallSucceeded {
		return fmt.Errorf("completed architecture synthesis requires a successful provider response")
	}
	if !status.ProposalAccepted && !status.ProposalRejected {
		return fmt.Errorf("completed architecture synthesis requires a proposal outcome")
	}
	if status.ProposalNormalized != (status.NormalizationCount > 0) {
		return fmt.Errorf("architecture normalization metrics are inconsistent")
	}
	if status.ArchitectureSource == "" || status.ArchitectureLevel == 0 {
		return fmt.Errorf("completed architecture synthesis requires an architecture source")
	}
	if status.FallbackSelected != (status.FallbackReason != "") {
		return fmt.Errorf("architecture fallback metrics are inconsistent")
	}
	return nil
}

func validateArchitectureSynthesisFailure(failure ArchitectureSynthesisFailure) error {
	valid := false
	switch failure.Stage + "\x00" + failure.Code {
	case "input_preparation\x00architecture.preparation_failed",
		"provider_configuration\x00architecture.provider_configuration_failed",
		"provider_call\x00architecture.provider_call_failed",
		"provider_call\x00architecture.provider_output_limit",
		"response_decode\x00architecture.empty_response",
		"response_validation\x00architecture.proposal_rejected",
		"response_validation\x00componentmap.response_evaluation_failed",
		"landscape_validation\x00componentmap.partial_model_inconsistent":
		valid = true
	}
	if !valid {
		return fmt.Errorf("architecture synthesis failure diagnostic is invalid")
	}
	wantDetail := ""
	if failure.Code == componentmap.SynthesisFailureCodePartialModelInconsistent {
		wantDetail = componentmap.DiagnoseSynthesisFailure(
			componentmap.ErrPartialModelStateInconsistent,
		).Detail
	}
	if failure.Detail != wantDetail {
		return fmt.Errorf("architecture synthesis failure detail is invalid")
	}
	return nil
}

func (shape ArchitectureSynthesisResponseShape) Validate() error {
	if shape.Grammar != "" && shape.Grammar != "nested" && shape.Grammar != "flat" {
		return fmt.Errorf("architecture synthesis response shape grammar is invalid")
	}
	metrics := []int{
		shape.SubsystemCount, shape.ComponentCount, shape.MemberRefCount,
		shape.UnitRefCount, shape.AnchorRefCount, shape.MissingAnchorRefsCount,
		shape.EmptyAnchorRefsCount, shape.NullAnchorRefsCount,
	}
	for _, metric := range metrics {
		if metric < 0 {
			return fmt.Errorf("architecture synthesis response shape contains invalid metrics")
		}
	}
	if !shape.JSONValid && (shape.Grammar != "" || shape.SubsystemCount != 0 ||
		shape.ComponentCount != 0 || shape.MemberRefCount != 0 || shape.UnitRefCount != 0 ||
		shape.AnchorRefCount != 0 || shape.MissingAnchorRefsCount != 0 ||
		shape.EmptyAnchorRefsCount != 0 || shape.NullAnchorRefsCount != 0) {
		return fmt.Errorf("invalid JSON cannot carry decoded architecture response shape")
	}
	if shape.MissingAnchorRefsCount+shape.EmptyAnchorRefsCount+shape.NullAnchorRefsCount > shape.ComponentCount {
		return fmt.Errorf("architecture synthesis anchor field counts exceed components")
	}
	return nil
}

func validArchitectureSemanticExchangePath(value string) bool {
	local := filepath.FromSlash(value)
	if value == "" || !filepath.IsLocal(local) || filepath.ToSlash(filepath.Clean(local)) != value {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 3 || parts[0] != "semantic_exchanges" || len(parts[1]) != 64 ||
		parts[2] != "exchange.v2.json" {
		return false
	}
	for _, character := range parts[1] {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func validArchitectureUnavailableCode(code string) bool {
	return code == ArchitectureSynthesisUnavailableOfflineCode ||
		code == ArchitectureSynthesisUnavailableExactWorkspaceGraphCode
}

func (status ArchitectureSynthesisStatus) validateResolvedMembershipEvidence() error {
	if status.PromptBytes != 0 {
		return fmt.Errorf("architecture synthesis status v4+ cannot use legacy prompt bytes")
	}
	if status.Version >= 7 {
		if status.CandidateCount != 0 {
			return fmt.Errorf("architecture synthesis status v7+ cannot use legacy candidate count")
		}
		if status.LocalCandidateCount != status.RequestedConceptualCount+status.StructuralLocatorCount {
			return fmt.Errorf("architecture synthesis local candidate role counts are inconsistent")
		}
	} else if status.LocalCandidateCount != 0 || status.RequestedConceptualCount != 0 ||
		status.StructuralLocatorCount != 0 {
		return fmt.Errorf("historical architecture synthesis status cannot use v7 candidate role counts")
	}
	if status.Version < 8 && (status.CoveredConceptualCount != 0 ||
		status.UncoveredConceptualCount != 0 || len(status.UncoveredConceptualIDs) != 0 ||
		status.ProposalPartial) {
		return fmt.Errorf("historical architecture synthesis status cannot use v8 coverage evidence")
	}
	if status.ProviderRequestCount == 1 {
		candidateEvidence := status.CandidateCount
		if status.Version >= 7 {
			candidateEvidence = status.RequestedConceptualCount
		}
		if status.RequestBytes == 0 || candidateEvidence == 0 || status.TransportAttempts == 0 {
			return fmt.Errorf("live architecture synthesis status requires exact request evidence")
		}
	} else if status.TransportAttempts != 0 {
		return fmt.Errorf("provider-free architecture status cannot contain transport attempts")
	}
	if status.ProviderRequestCount == 0 && status.State != ArchitectureSynthesisCached &&
		(status.ResponseBytes != 0 || status.ResponseContentBytes != 0 ||
			status.MembershipCounted || status.MemberOccurrences != 0 || status.DistinctMembers != 0 ||
			status.UsageReported || status.InputTokens != 0 || status.OutputTokens != 0 ||
			status.FinishReason != "" || status.ResponseComplete || status.ResponseState != "" ||
			len(status.ValidationCodes) != 0 || status.ProviderCallSucceeded || status.ResponseParsed ||
			status.ProposalAccepted || status.ProposalPartial || status.ProposalNormalized || status.ProposalRejected ||
			status.CoveredConceptualCount != 0 || status.UncoveredConceptualCount != 0 ||
			len(status.UncoveredConceptualIDs) != 0) {
		return fmt.Errorf("uncalled architecture synthesis cannot contain provider response evidence")
	}
	if status.ResponseContentBytes > status.ResponseBytes {
		return fmt.Errorf("architecture synthesis response byte evidence is inconsistent")
	}
	if status.MembershipCounted {
		if status.DistinctMembers > status.MemberOccurrences ||
			(status.MemberOccurrences > 0 && status.DistinctMembers == 0) {
			return fmt.Errorf("architecture synthesis response membership counts are inconsistent")
		}
	} else if status.MemberOccurrences != 0 || status.DistinctMembers != 0 {
		return fmt.Errorf("uncounted architecture response cannot contain membership counts")
	}
	// Decision 215: an attempted Architecture provider call that exhausted its
	// output budget carries exact bounded evidence and nothing else. The
	// partial response is diagnostic-only and the local Canvas is the visible
	// Architecture.
	if status.ErrorCode == ArchitectureSynthesisErrorProviderOutputLimit {
		if err := status.validateProviderOutputLimitEvidence(); err != nil {
			return err
		}
	}
	if err := status.validateConceptualCoverage(); err != nil {
		return err
	}
	if !status.UsageReported && (status.InputTokens != 0 || status.OutputTokens != 0) {
		return fmt.Errorf("architecture token counts require reported provider usage")
	}
	switch status.FinishReason {
	case "", "stop", "content_filter", "tool_calls", "insufficient_system_resource":
	case "length":
		// Decision 215: a v9 failed provider_output_limit status truthfully
		// records finish_reason=length with response_complete=false. Every
		// other state rejects a length-ended claim below.
		if status.State != ArchitectureSynthesisFailed ||
			status.ErrorCode != ArchitectureSynthesisErrorProviderOutputLimit {
			return fmt.Errorf("architecture synthesis status has unsupported finish reason %q", status.FinishReason)
		}
	default:
		return fmt.Errorf("architecture synthesis status has unsupported finish reason %q", status.FinishReason)
	}
	if status.ResponseComplete != (status.FinishReason == "stop") {
		return fmt.Errorf("architecture response completion evidence is inconsistent")
	}
	switch status.ResponseState {
	case "", "captured", "empty", "oversize_omitted", "sensitive_omitted":
	default:
		return fmt.Errorf("architecture synthesis status has unsupported response state %q", status.ResponseState)
	}
	if status.ProviderCallSucceeded {
		if status.ResponseBytes == 0 || status.ResponseState == "" {
			return fmt.Errorf("successful provider call requires exact response evidence")
		}
		if status.ResponseState == "captured" && status.ResponseContentBytes == 0 {
			return fmt.Errorf("captured architecture response requires content bytes")
		}
		if status.ResponseState != "captured" && status.ResponseContentBytes != 0 {
			return fmt.Errorf("omitted architecture response cannot claim captured content bytes")
		}
	}
	if (status.State == ArchitectureSynthesisSucceeded || status.State == ArchitectureSynthesisCached) &&
		(!status.ResponseComplete || !status.MembershipCounted || status.MemberOccurrences == 0 ||
			(status.Version == 4 && status.MemberOccurrences != status.DistinctMembers) ||
			(status.Version == 6 && status.DistinctMembers != status.CandidateCount) ||
			(status.Version == 7 && status.DistinctMembers != status.RequestedConceptualCount) ||
			status.ResponseState != "captured") {
		return fmt.Errorf("accepted live Architecture requires complete exact membership evidence")
	}
	if len(status.ValidationCodes) > len(architectureStatusValidationCodes) {
		return fmt.Errorf("architecture synthesis status has too many validation diagnostic codes")
	}
	seen := make(map[string]struct{}, len(status.ValidationCodes))
	for _, code := range status.ValidationCodes {
		if !validArchitectureStatusCodeForVersion(status.Version, code) {
			return fmt.Errorf("architecture synthesis status contains invalid validation diagnostic code")
		}
		if _, duplicate := seen[code]; duplicate {
			return fmt.Errorf("architecture synthesis status contains duplicate validation diagnostic code")
		}
		seen[code] = struct{}{}
	}
	return nil
}

func (status ArchitectureSynthesisStatus) validateFailureEvidence() error {
	if status.Failure == nil {
		return fmt.Errorf("failed architecture synthesis status requires a failure diagnostic")
	}
	code := status.Failure.Code
	switch code {
	case "architecture.preparation_failed", "architecture.provider_configuration_failed":
		if status.ErrorCode != "provider_error" || status.ProviderRequestCount != 0 ||
			status.ProviderCallSucceeded || status.ResponseBytes != 0 || status.ResponseState != "" ||
			status.ResponseShape != nil || status.SemanticExchangePath != "" {
			return fmt.Errorf("pre-call architecture failure evidence is inconsistent")
		}
	case "architecture.provider_call_failed":
		if status.ErrorCode != "provider_error" || status.ProviderRequestCount != 1 ||
			status.ProviderCallSucceeded {
			return fmt.Errorf("architecture provider-call failure evidence is inconsistent")
		}
	case "architecture.provider_output_limit":
		if status.ErrorCode != ArchitectureSynthesisErrorProviderOutputLimit {
			return fmt.Errorf("architecture output-limit failure code is inconsistent")
		}
	case "architecture.empty_response":
		if status.ErrorCode != "empty_response" || status.ProviderRequestCount != 1 ||
			!status.ProviderCallSucceeded || status.ResponseParsed || status.ResponseBytes == 0 ||
			status.ResponseContentBytes != 0 || status.ResponseState != "empty" {
			return fmt.Errorf("empty architecture response evidence is inconsistent")
		}
	case "architecture.proposal_rejected", "componentmap.response_evaluation_failed":
		if status.ErrorCode != "invalid_response" || status.ProviderRequestCount != 1 ||
			!status.ProviderCallSucceeded || status.ResponseBytes == 0 {
			return fmt.Errorf("architecture response rejection evidence is inconsistent")
		}
	case "componentmap.partial_model_inconsistent":
		if status.ErrorCode != "invalid_response" || status.ProviderRequestCount != 1 ||
			!status.ProviderCallSucceeded || !status.ResponseParsed || status.ResponseBytes == 0 {
			return fmt.Errorf("architecture landscape failure evidence is inconsistent")
		}
	default:
		return fmt.Errorf("architecture synthesis failure diagnostic is invalid")
	}
	return nil
}

func (status ArchitectureSynthesisStatus) validateProviderOutputLimitEvidence() error {
	if status.Version < 9 {
		return fmt.Errorf("provider output limit status requires version 9")
	}
	if status.State != ArchitectureSynthesisFailed {
		return fmt.Errorf("provider output limit evidence requires a failed state")
	}
	if status.ProviderRequestCount != 1 || status.RequestBytes == 0 || status.TransportAttempts == 0 {
		return fmt.Errorf("provider output limit status requires exact attempted request evidence")
	}
	if status.ResponseBytes == 0 {
		return fmt.Errorf("provider output limit status requires known partial response bytes")
	}
	if status.ConfiguredMaxTokens <= 0 {
		return fmt.Errorf("provider output limit status requires configured output token evidence")
	}
	// Output-token exhaustion (finish_reason=length) and local response-byte
	// overflow are both Decision 215 attempted output exhaustion, but only the
	// former carries provider finish/usage evidence.
	if status.FinishReason == "length" {
		if status.ResponseComplete {
			return fmt.Errorf("provider output limit status requires response_complete=false")
		}
		if status.ObservedOutputTokens <= 0 {
			return fmt.Errorf("provider output limit status requires observed output token evidence")
		}
		if status.OutputTokens != status.ObservedOutputTokens {
			return fmt.Errorf("provider output limit observed tokens do not match usage")
		}
		if status.UsageReported != (status.InputTokens != 0 || status.OutputTokens != 0) {
			return fmt.Errorf("provider output limit token counts are inconsistent with reported usage")
		}
	} else if status.FinishReason != "" && status.FinishReason != "stop" {
		return fmt.Errorf("provider output limit status has unsupported finish reason %q", status.FinishReason)
	}
	if status.ProviderCallSucceeded || status.ResponseParsed || status.MembershipCounted ||
		status.MemberOccurrences != 0 || status.DistinctMembers != 0 ||
		status.CoveredConceptualCount != 0 || status.UncoveredConceptualCount != 0 ||
		len(status.UncoveredConceptualIDs) != 0 || len(status.ValidationCodes) != 0 {
		return fmt.Errorf("provider output limit status cannot publish partial response evidence")
	}
	return nil
}

func (status ArchitectureSynthesisStatus) validatePrimaryScopeCoverage() error {
	if status.Version < 13 {
		if status.RequestedPrimaryScopeCount != 0 || status.CoveredPrimaryScopeCount != 0 ||
			status.UncoveredPrimaryScopeCount != 0 || status.CoveredSupportingEvidenceCount != 0 {
			return fmt.Errorf("historical architecture synthesis status cannot use primary-scope coverage")
		}
		return nil
	}
	if status.RequestedPrimaryScopeCount == 0 {
		if status.CoveredPrimaryScopeCount != 0 || status.UncoveredPrimaryScopeCount != 0 {
			return fmt.Errorf("architecture primary-scope coverage lacks a requested scope")
		}
		if status.hasValidationCode("proposal.empty_primary_scope_coverage") ||
			status.hasValidationCode("proposal.supporting_only_unit_coverage") {
			return fmt.Errorf("architecture primary-scope diagnostic lacks a requested scope")
		}
		// Versions 13-15 required every accepted result to carry primary scope.
		// Preserve that historical contract while status 16 records the producer's
		// already-valid all-supporting test/tooling/documentation landscape.
		if status.Version < 16 {
			if status.CoveredSupportingEvidenceCount != 0 {
				return fmt.Errorf("architecture primary-scope coverage lacks a requested scope")
			}
			if status.State == ArchitectureSynthesisSucceeded || status.State == ArchitectureSynthesisCached {
				return fmt.Errorf("accepted architecture synthesis requires primary-scope evidence")
			}
			return nil
		}
		if status.CoveredSupportingEvidenceCount > status.RequestedConceptualCount {
			return fmt.Errorf("architecture supporting coverage exceeds the requested conceptual scope")
		}
		if status.CoveredSupportingEvidenceCount > 0 &&
			(!status.ProviderCallSucceeded || !status.ResponseParsed || !status.MembershipCounted) {
			return fmt.Errorf("architecture supporting coverage lacks parsed membership evidence")
		}
		if status.MembershipCounted {
			if status.CoveredSupportingEvidenceCount != status.DistinctMembers {
				return fmt.Errorf("architecture supporting coverage does not match resolved membership")
			}
		} else if status.CoveredSupportingEvidenceCount != 0 {
			return fmt.Errorf("uncounted architecture response cannot contain supporting coverage")
		}
		return nil
	}
	if status.RequestedPrimaryScopeCount > status.RequestedConceptualCount ||
		status.CoveredPrimaryScopeCount > status.RequestedPrimaryScopeCount ||
		status.UncoveredPrimaryScopeCount > status.RequestedPrimaryScopeCount ||
		status.CoveredSupportingEvidenceCount > status.RequestedConceptualCount-status.RequestedPrimaryScopeCount {
		return fmt.Errorf("architecture primary-scope coverage exceeds the requested conceptual scope")
	}
	if !status.ProviderCallSucceeded || !status.ResponseParsed || !status.MembershipCounted {
		return fmt.Errorf("architecture primary-scope coverage lacks parsed membership evidence")
	}
	if status.CoveredPrimaryScopeCount+status.UncoveredPrimaryScopeCount != status.RequestedPrimaryScopeCount {
		return fmt.Errorf("architecture primary-scope coverage partition is inconsistent")
	}
	if status.Version < 17 {
		if status.CoveredPrimaryScopeCount+status.CoveredSupportingEvidenceCount != status.DistinctMembers {
			return fmt.Errorf("architecture primary-scope coverage does not match resolved membership")
		}
	} else {
		// Model membership contains only the refs the model returned. A selected
		// nested supporting ref can additionally cover its exact backend-owned
		// primary parent for scope accounting, but that parent is not inserted
		// into membership. The status has aggregate evidence only; the synthesis
		// record validates the exact ref-to-parent mapping.
		explicitPrimary := status.DistinctMembers - status.CoveredSupportingEvidenceCount
		if explicitPrimary < 0 || status.CoveredPrimaryScopeCount < explicitPrimary ||
			status.CoveredPrimaryScopeCount-explicitPrimary > status.CoveredSupportingEvidenceCount {
			return fmt.Errorf("architecture derived primary-scope coverage does not match resolved membership")
		}
	}
	if status.State == ArchitectureSynthesisFailed && !status.ProposalRejected {
		return fmt.Errorf("failed architecture primary-scope coverage requires a rejected proposal")
	}
	emptyPrimary := status.hasValidationCode("proposal.empty_primary_scope_coverage")
	supportingOnly := status.hasValidationCode("proposal.supporting_only_unit_coverage")
	qualityRejected := emptyPrimary || supportingOnly
	if qualityRejected {
		if status.State != ArchitectureSynthesisFailed || !status.ProposalRejected {
			return fmt.Errorf("architecture primary-scope quality rejection lacks parsed membership evidence")
		}
		if status.Failure == nil || status.Failure.Stage != "response_validation" ||
			status.Failure.Code != "architecture.proposal_rejected" {
			return fmt.Errorf("architecture primary-scope quality diagnostic has the wrong failure cause")
		}
	}
	if status.State == ArchitectureSynthesisSucceeded || status.State == ArchitectureSynthesisCached {
		if status.CoveredPrimaryScopeCount == 0 || (status.Version < 17 &&
			status.CoveredPrimaryScopeCount+status.CoveredSupportingEvidenceCount != status.CoveredConceptualCount) {
			return fmt.Errorf("accepted architecture primary-scope coverage is inconsistent")
		}
	}
	if emptyPrimary && status.CoveredPrimaryScopeCount != 0 {
		return fmt.Errorf("empty primary-scope diagnostic contradicts coverage evidence")
	}
	if supportingOnly && status.CoveredSupportingEvidenceCount == 0 {
		return fmt.Errorf("supporting-only diagnostic requires supporting coverage evidence")
	}
	return nil
}

func (status ArchitectureSynthesisStatus) hasValidationCode(want string) bool {
	for _, code := range status.ValidationCodes {
		if code == want {
			return true
		}
	}
	return false
}

func (status ArchitectureSynthesisStatus) validateConceptualCoverage() error {
	if status.Version < 8 {
		return nil
	}
	accepted := status.State == ArchitectureSynthesisSucceeded || status.State == ArchitectureSynthesisCached
	if !accepted {
		if status.CoveredConceptualCount != 0 || status.UncoveredConceptualCount != 0 ||
			len(status.UncoveredConceptualIDs) != 0 || status.ProposalPartial {
			return fmt.Errorf("unaccepted architecture synthesis cannot contain accepted coverage evidence")
		}
		return nil
	}
	if !status.ProposalAccepted || !status.MembershipCounted || status.CoveredConceptualCount == 0 {
		return fmt.Errorf("accepted architecture synthesis requires non-empty resolved conceptual coverage")
	}
	if status.CoveredConceptualCount != status.DistinctMembers {
		return fmt.Errorf("architecture synthesis covered count does not match resolved distinct membership")
	}
	if status.CoveredConceptualCount+status.UncoveredConceptualCount != status.RequestedConceptualCount {
		return fmt.Errorf("architecture synthesis conceptual coverage counts are inconsistent")
	}
	if len(status.UncoveredConceptualIDs) != status.UncoveredConceptualCount {
		return fmt.Errorf("architecture synthesis uncovered member identities are incomplete")
	}
	if status.ProposalPartial != (status.UncoveredConceptualCount > 0) {
		return fmt.Errorf("architecture synthesis partial outcome does not match uncovered members")
	}
	for index, id := range status.UncoveredConceptualIDs {
		if !validArchitectureConceptualMemberID(id) {
			return fmt.Errorf("architecture synthesis uncovered member identity is invalid")
		}
		if index > 0 && !architectureMemberIDBefore(status.UncoveredConceptualIDs[index-1], id) {
			return fmt.Errorf("architecture synthesis uncovered member identities are not unique and sorted")
		}
	}
	return nil
}

func validArchitectureConceptualMemberID(id componentmap.MemberID) bool {
	if strings.TrimSpace(id.Value) == "" || id.Value != strings.TrimSpace(id.Value) {
		return false
	}
	switch id.Kind {
	case componentmap.MemberPackage, componentmap.MemberFile, componentmap.MemberSymbol,
		componentmap.MemberEntrypoint, componentmap.MemberFlow:
		return true
	default:
		return false
	}
}

func architectureMemberIDBefore(left, right componentmap.MemberID) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Value < right.Value
}

func validArchitectureStatusCode(code string) bool {
	_, valid := architectureStatusValidationCodes[code]
	return valid
}

func validArchitectureStatusCodeForVersion(version int, code string) bool {
	if code == "proposal.partial_member_coverage" && version < 8 {
		return false
	}
	if (code == "proposal.empty_primary_scope_coverage" ||
		code == "proposal.supporting_only_unit_coverage") && version < 13 {
		return false
	}
	if (code == "proposal.empty_component" ||
		code == "proposal.supporting_only_unit_coverage_salvaged") && version < 15 {
		return false
	}
	if validArchitectureStatusCode(code) {
		return true
	}
	if version > 5 {
		return false
	}
	_, valid := architectureStatusHistoricalValidationCodes[code]
	return valid
}

func readArchitectureSynthesisStatus(path string) (*ArchitectureSynthesisStatus, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, fmt.Sprintf("architecture synthesis status: %v", err)
	}
	var status ArchitectureSynthesisStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Sprintf("architecture synthesis status: invalid json")
	}
	if err := status.Validate(); err != nil {
		return nil, fmt.Sprintf("architecture synthesis status: %v", err)
	}
	return &status, ""
}

func architectureSynthesisUserWarning(status *ArchitectureSynthesisStatus) string {
	if status == nil {
		return ""
	}
	if status.Version >= 2 && status.FallbackSelected && status.ProposalRejected {
		return "The model architecture proposal was rejected by local validation; the exact local Architecture Canvas is shown."
	}
	if status.State != ArchitectureSynthesisFailed {
		return ""
	}
	switch status.ErrorCode {
	case "empty_response":
		return "Model-assisted Architecture grouping was not generated because the request returned no content; the exact local Architecture Canvas is shown."
	case "invalid_response":
		return "Model-assisted Architecture grouping was not accepted because the response could not be validated; the exact local Architecture Canvas is shown."
	default:
		return "Model-assisted Architecture grouping was not generated because the request failed; the exact local Architecture Canvas is shown."
	}
}
