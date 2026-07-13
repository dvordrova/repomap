package modelresearch

import "fmt"

const PolicyVersion = "adaptive-model-research-v1"

// StageBudget combines a byte-oriented provider limit with secondary safety
// ceilings. Bytes, not item counts, are the primary context boundary.
type StageBudget struct {
	TargetRequestBytes int `json:"target_request_bytes"`
	MaxRequestBytes    int `json:"max_request_bytes"`
	MaxFiles           int `json:"max_files,omitempty"`
	MaxEvidenceItems   int `json:"max_evidence_items,omitempty"`
	MaxSourceWindows   int `json:"max_source_windows,omitempty"`
}

// Policy is the complete bounded normal-run model policy. A run may consume
// less than these limits and usually should.
type Policy struct {
	Version              string      `json:"version"`
	Orientation          StageBudget `json:"orientation"`
	Targeted             StageBudget `json:"targeted_research"`
	Architecture         StageBudget `json:"architecture_synthesis"`
	MaxSemanticCalls     int         `json:"max_semantic_calls"`
	MaxTargetedRounds    int         `json:"max_targeted_rounds"`
	MaxTotalRequestBytes int         `json:"max_total_request_bytes"`
}

func DefaultPolicy() Policy {
	return Policy{
		Version: PolicyVersion,
		Orientation: StageBudget{
			TargetRequestBytes: 80 << 10,
			MaxRequestBytes:    96 << 10,
			MaxFiles:           250,
		},
		Targeted: StageBudget{
			TargetRequestBytes: 64 << 10,
			MaxRequestBytes:    80 << 10,
			MaxFiles:           25,
			MaxEvidenceItems:   160,
			MaxSourceWindows:   25,
		},
		Architecture: StageBudget{
			TargetRequestBytes: 80 << 10,
			MaxRequestBytes:    96 << 10,
		},
		MaxSemanticCalls:     4,
		MaxTargetedRounds:    2,
		MaxTotalRequestBytes: 320 << 10,
	}
}

func (p Policy) Validate() error {
	if p.Version != PolicyVersion {
		return fmt.Errorf("model research: unsupported policy version %q", p.Version)
	}
	for name, budget := range map[string]StageBudget{
		"orientation": p.Orientation, "targeted": p.Targeted, "architecture": p.Architecture,
	} {
		if budget.TargetRequestBytes <= 0 || budget.MaxRequestBytes < budget.TargetRequestBytes {
			return fmt.Errorf("model research: invalid %s request-byte budget", name)
		}
	}
	if p.MaxSemanticCalls != 4 {
		return fmt.Errorf("model research: normal semantic call limit must be 4")
	}
	if p.MaxTargetedRounds < 0 || p.MaxTargetedRounds > 2 {
		return fmt.Errorf("model research: targeted round limit must be between 0 and 2")
	}
	if p.MaxTotalRequestBytes < p.Orientation.MaxRequestBytes+p.Architecture.MaxRequestBytes {
		return fmt.Errorf("model research: total request budget cannot hold orientation and architecture")
	}
	return nil
}

type Usage struct {
	SemanticCalls int `json:"semantic_calls"`
	RequestBytes  int `json:"request_bytes"`
}

// Allows reports whether one more semantic stage fits the hard normal-run
// call, stage-byte, and total-byte budgets.
func (p Policy) Allows(stage StageBudget, usage Usage, requestBytes int) (bool, string) {
	if requestBytes <= 0 {
		return false, "empty_request"
	}
	if requestBytes > stage.MaxRequestBytes {
		return false, "stage_byte_budget_exhausted"
	}
	if usage.SemanticCalls >= p.MaxSemanticCalls {
		return false, "call_budget_exhausted"
	}
	if usage.RequestBytes+requestBytes > p.MaxTotalRequestBytes {
		return false, "total_byte_budget_exhausted"
	}
	return true, "within_budget"
}
