package report

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	ArchitectureSynthesisStatusFile    = "architecture_synthesis_status.json"
	ArchitectureSynthesisStatusVersion = 4

	ArchitectureSynthesisSucceeded   = "succeeded"
	ArchitectureSynthesisCached      = "cached"
	ArchitectureSynthesisFailed      = "failed"
	ArchitectureSynthesisUnavailable = "unavailable"
)

// architectureStatusValidationCodes is the complete diagnostic vocabulary
// that the active Architecture response evaluator and its status evidence
// checks can persist. Keep this boundary closed: accepting merely well-formed
// opaque strings would make a v4 status claim validation evidence that this
// reader cannot interpret. Resource-limit outcomes are terminal errors and
// therefore are deliberately not represented as validation codes here.
var architectureStatusValidationCodes = map[string]struct{}{
	// Locally applied proposal validation and normalization diagnostics.
	"proposal.components_per_subsystem_above_preferred": {},
	"proposal.conflicting_membership":                   {},
	"proposal.invalid_component":                        {},
	"proposal.invalid_member_id":                        {},
	"proposal.invalid_members":                          {},
	"proposal.invalid_subsystem":                        {},
	"proposal.invalid_subsystem_count":                  {},
	"proposal.no_usable_subsystems":                     {},
	"proposal.normalized_components_per_subsystem":      {},
	"proposal.normalized_description":                   {},
	"proposal.normalized_package_only_hypothesis":       {},
	"proposal.normalized_primary_subsystems":            {},
	"proposal.normalized_total_components":              {},
	"proposal.omitted_members_exceed_bounds":            {},
	"proposal.omitted_members_preserved":                {},
	"proposal.omitted_process_entry_member":             {},
	"proposal.primary_subsystems_above_preferred":       {},
	"proposal.total_components_above_preferred":         {},
	"proposal.ungrounded_primary_component":             {},
	"proposal.unknown_anchor_id":                        {},
	"proposal.unknown_member_id":                        {},
	"proposal.unsupported_version":                      {},

	// Provider response extraction and request-local reference diagnostics.
	"response.ambiguous_json":          {},
	"response.embedded_json_extracted": {},
	"response.fenced_json_extracted":   {},
	"response.invalid_proposal":        {},
	"response.no_json":                 {},
	"response.sensitive_omitted":       {},
	"response.unknown_fields_ignored":  {},

	// Exact response-evidence checks owned by the Architecture stage runner.
	"response.incomplete":             {},
	"response.membership_unavailable": {},
	"response.not_captured":           {},
	"status.invalid_evidence":         {},
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
	PromptBytes           int      `json:"prompt_bytes,omitempty"`
	RequestBytes          int      `json:"request_bytes,omitempty"`
	ResponseBytes         int      `json:"response_bytes,omitempty"`
	ResponseContentBytes  int      `json:"response_content_bytes,omitempty"`
	LatencyMillis         int64    `json:"latency_ms,omitempty"`
	ProviderRequestCount  int      `json:"provider_request_count,omitempty"`
	TransportAttempts     int      `json:"transport_attempts,omitempty"`
	CandidateCount        int      `json:"candidate_count,omitempty"`
	AnchorCount           int      `json:"anchor_count,omitempty"`
	MembershipCounted     bool     `json:"response_membership_counted,omitempty"`
	MemberOccurrences     int      `json:"response_member_occurrences,omitempty"`
	DistinctMembers       int      `json:"response_distinct_members,omitempty"`
	UsageReported         bool     `json:"usage_reported,omitempty"`
	InputTokens           int      `json:"input_tokens,omitempty"`
	OutputTokens          int      `json:"output_tokens,omitempty"`
	FinishReason          string   `json:"finish_reason,omitempty"`
	ResponseComplete      bool     `json:"response_complete,omitempty"`
	ResponseState         string   `json:"response_state,omitempty"`
	ValidationCodes       []string `json:"validation_diagnostic_codes,omitempty"`
	ProviderCallSucceeded bool     `json:"provider_call_succeeded,omitempty"`
	ResponseParsed        bool     `json:"response_parsed,omitempty"`
	ProposalAccepted      bool     `json:"proposal_accepted,omitempty"`
	ProposalNormalized    bool     `json:"proposal_normalized,omitempty"`
	ProposalRejected      bool     `json:"proposal_rejected,omitempty"`
	FallbackSelected      bool     `json:"fallback_selected,omitempty"`
	ArchitectureSource    string   `json:"architecture_source,omitempty"`
	ArchitectureLevel     int      `json:"architecture_level,omitempty"`
	NormalizationCount    int      `json:"normalization_count,omitempty"`
	FallbackReason        string   `json:"fallback_reason,omitempty"`
	ErrorCode             string   `json:"error_code,omitempty"`
	UnavailableCode       string   `json:"unavailable_code,omitempty"`
}

func (status ArchitectureSynthesisStatus) Validate() error {
	if status.Version < 1 || status.Version > ArchitectureSynthesisStatusVersion {
		return fmt.Errorf("unsupported architecture synthesis status version %d", status.Version)
	}
	switch status.State {
	case ArchitectureSynthesisSucceeded, ArchitectureSynthesisCached:
		if status.ErrorCode != "" || status.UnavailableCode != "" {
			return fmt.Errorf("successful architecture synthesis status cannot contain an error code")
		}
	case ArchitectureSynthesisFailed:
		if status.ErrorCode == "" || status.UnavailableCode != "" {
			return fmt.Errorf("failed architecture synthesis status requires an error code")
		}
	case ArchitectureSynthesisUnavailable:
		if status.Version < 3 || status.UnavailableCode != "offline" || status.ErrorCode != "" ||
			status.PromptBytes != 0 || status.LatencyMillis != 0 || status.ProviderRequestCount != 0 ||
			status.ProviderCallSucceeded || status.ResponseParsed || status.ProposalAccepted ||
			status.ProposalNormalized || status.ProposalRejected || status.FallbackSelected ||
			status.ArchitectureSource != "" || status.ArchitectureLevel != 0 ||
			status.NormalizationCount != 0 || status.FallbackReason != "" ||
			status.RequestBytes != 0 || status.ResponseBytes != 0 ||
			status.ResponseContentBytes != 0 || status.TransportAttempts != 0 ||
			status.CandidateCount != 0 || status.AnchorCount != 0 ||
			status.MembershipCounted || status.MemberOccurrences != 0 ||
			status.DistinctMembers != 0 || status.UsageReported ||
			status.InputTokens != 0 || status.OutputTokens != 0 ||
			status.FinishReason != "" || status.ResponseComplete ||
			status.ResponseState != "" || len(status.ValidationCodes) != 0 {
			return fmt.Errorf("unavailable architecture synthesis status is inconsistent")
		}
		return nil
	default:
		return fmt.Errorf("unsupported architecture synthesis state %q", status.State)
	}
	if status.PromptBytes < 0 || status.RequestBytes < 0 || status.ResponseBytes < 0 ||
		status.ResponseContentBytes < 0 || status.LatencyMillis < 0 ||
		status.ProviderRequestCount < 0 || status.ProviderRequestCount > 1 ||
		status.TransportAttempts < 0 || status.CandidateCount < 0 ||
		status.AnchorCount < 0 || status.MemberOccurrences < 0 ||
		status.DistinctMembers < 0 || status.InputTokens < 0 || status.OutputTokens < 0 {
		return fmt.Errorf("architecture synthesis status contains invalid metrics")
	}
	if status.Version == 1 {
		return nil
	}
	if status.Version >= 4 {
		if err := status.validateV4Evidence(); err != nil {
			return err
		}
	}
	if status.ArchitectureLevel < 0 || status.ArchitectureLevel > 4 || status.NormalizationCount < 0 {
		return fmt.Errorf("architecture synthesis status contains invalid outcome metrics")
	}
	if status.ProposalNormalized && !status.ProposalAccepted {
		return fmt.Errorf("normalized architecture proposal must also be accepted")
	}
	if status.ProposalAccepted && status.ProposalRejected {
		return fmt.Errorf("architecture proposal cannot be accepted and rejected")
	}
	if status.State == ArchitectureSynthesisFailed {
		if status.Version >= 3 && (status.ProposalAccepted || status.ProposalNormalized ||
			status.FallbackSelected || status.FallbackReason != "" ||
			status.ArchitectureSource != "" || status.ArchitectureLevel != 0 ||
			status.NormalizationCount != 0) {
			return fmt.Errorf("failed architecture synthesis status cannot publish enrichment")
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

func (status ArchitectureSynthesisStatus) validateV4Evidence() error {
	if status.PromptBytes != 0 {
		return fmt.Errorf("architecture synthesis status v4 cannot use legacy prompt bytes")
	}
	if status.ProviderRequestCount == 1 {
		if status.RequestBytes == 0 || status.CandidateCount == 0 || status.TransportAttempts == 0 {
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
			status.ProposalAccepted || status.ProposalNormalized || status.ProposalRejected) {
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
	if !status.UsageReported && (status.InputTokens != 0 || status.OutputTokens != 0) {
		return fmt.Errorf("architecture token counts require reported provider usage")
	}
	switch status.FinishReason {
	case "", "stop", "content_filter", "tool_calls", "insufficient_system_resource":
	default:
		return fmt.Errorf("architecture synthesis status has unsupported finish reason %q", status.FinishReason)
	}
	if status.ResponseComplete != (status.FinishReason == "stop") {
		return fmt.Errorf("architecture response completion evidence is inconsistent")
	}
	switch status.ResponseState {
	case "", "captured", "oversize_omitted", "sensitive_omitted":
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
			status.MemberOccurrences != status.DistinctMembers ||
			status.ResponseState != "captured") {
		return fmt.Errorf("accepted live Architecture requires complete exact membership evidence")
	}
	if len(status.ValidationCodes) > len(architectureStatusValidationCodes) {
		return fmt.Errorf("architecture synthesis status has too many validation diagnostic codes")
	}
	seen := make(map[string]struct{}, len(status.ValidationCodes))
	for _, code := range status.ValidationCodes {
		if !validArchitectureStatusCode(code) {
			return fmt.Errorf("architecture synthesis status contains invalid validation diagnostic code")
		}
		if _, duplicate := seen[code]; duplicate {
			return fmt.Errorf("architecture synthesis status contains duplicate validation diagnostic code")
		}
		seen[code] = struct{}{}
	}
	return nil
}

func validArchitectureStatusCode(code string) bool {
	_, valid := architectureStatusValidationCodes[code]
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
