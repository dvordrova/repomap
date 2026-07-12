package report

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	ArchitectureSynthesisStatusFile    = "architecture_synthesis_status.json"
	ArchitectureSynthesisStatusVersion = 1

	ArchitectureSynthesisSucceeded = "succeeded"
	ArchitectureSynthesisCached    = "cached"
	ArchitectureSynthesisFailed    = "failed"
)

// ArchitectureSynthesisStatus records whether the optional conceptual
// grouping request produced a usable result. It contains only bounded
// operational metadata; prompts and provider responses remain in their own
// debug artifacts.
type ArchitectureSynthesisStatus struct {
	Version              int    `json:"version"`
	State                string `json:"state"`
	PromptBytes          int    `json:"prompt_bytes,omitempty"`
	LatencyMillis        int64  `json:"latency_ms,omitempty"`
	ProviderRequestCount int    `json:"provider_request_count,omitempty"`
	ErrorCode            string `json:"error_code,omitempty"`
}

func (status ArchitectureSynthesisStatus) Validate() error {
	if status.Version != ArchitectureSynthesisStatusVersion {
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
	if status == nil || status.State != ArchitectureSynthesisFailed {
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
