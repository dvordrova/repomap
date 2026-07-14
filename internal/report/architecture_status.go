package report

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	ArchitectureSynthesisStatusFile    = "architecture_synthesis_status.json"
	ArchitectureSynthesisStatusVersion = 2

	ArchitectureSynthesisSucceeded = "succeeded"
	ArchitectureSynthesisCached    = "cached"
	ArchitectureSynthesisFailed    = "failed"
)

// ArchitectureSynthesisStatus records whether the optional conceptual
// grouping request produced a usable result. It contains only bounded
// operational metadata; prompts and provider responses remain in their own
// debug artifacts.
type ArchitectureSynthesisStatus struct {
	Version               int    `json:"version"`
	State                 string `json:"state"`
	PromptBytes           int    `json:"prompt_bytes,omitempty"`
	LatencyMillis         int64  `json:"latency_ms,omitempty"`
	ProviderRequestCount  int    `json:"provider_request_count,omitempty"`
	ProviderCallSucceeded bool   `json:"provider_call_succeeded,omitempty"`
	ResponseParsed        bool   `json:"response_parsed,omitempty"`
	ProposalAccepted      bool   `json:"proposal_accepted,omitempty"`
	ProposalNormalized    bool   `json:"proposal_normalized,omitempty"`
	ProposalRejected      bool   `json:"proposal_rejected,omitempty"`
	FallbackSelected      bool   `json:"fallback_selected,omitempty"`
	ArchitectureSource    string `json:"architecture_source,omitempty"`
	ArchitectureLevel     int    `json:"architecture_level,omitempty"`
	NormalizationCount    int    `json:"normalization_count,omitempty"`
	FallbackReason        string `json:"fallback_reason,omitempty"`
	ErrorCode             string `json:"error_code,omitempty"`
}

func (status ArchitectureSynthesisStatus) Validate() error {
	if status.Version != 1 && status.Version != ArchitectureSynthesisStatusVersion {
		return fmt.Errorf("unsupported architecture synthesis status version %d", status.Version)
	}
	switch status.State {
	case ArchitectureSynthesisSucceeded, ArchitectureSynthesisCached:
		if status.ErrorCode != "" {
			return fmt.Errorf("successful architecture synthesis status cannot contain an error code")
		}
	case ArchitectureSynthesisFailed:
		if status.ErrorCode == "" {
			return fmt.Errorf("failed architecture synthesis status requires an error code")
		}
	default:
		return fmt.Errorf("unsupported architecture synthesis state %q", status.State)
	}
	if status.PromptBytes < 0 || status.LatencyMillis < 0 || status.ProviderRequestCount < 0 || status.ProviderRequestCount > 1 {
		return fmt.Errorf("architecture synthesis status contains invalid metrics")
	}
	if status.Version == 1 {
		return nil
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
		return nil
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
		return "The model architecture proposal was rejected by local validation; a deterministic local fallback is shown."
	}
	if status.State != ArchitectureSynthesisFailed {
		return ""
	}
	switch status.ErrorCode {
	case "empty_response":
		return "Architecture map was not generated because the grouping request returned no content."
	case "invalid_response":
		return "Architecture map was not generated because the grouping response could not be validated."
	default:
		return "Architecture map was not generated because the grouping request failed."
	}
}
