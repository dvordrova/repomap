package modelresearch

import "fmt"

const PolicyVersion = "adaptive-model-research-v2"

const maxTechnicalRequestBytes = 1 << 20

const guidedTourMaxRequestBytes = 256 << 10

const (
	coreSemanticCallLimit       = 4
	coreTotalRequestBytes       = 4 * maxTechnicalRequestBytes
	maxGuidedTourSemanticCalls  = 12
	maxGuidedTourAggregateBytes = 4 * maxTechnicalRequestBytes
)

// StageBudget combines an observed-size calibration target, a technical
// provider limit, and secondary safety ceilings. Exceeding the target does not
// reject a request; exceeding MaxRequestBytes does.
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
	GuidedTour           StageBudget `json:"guided_tour,omitempty"`
	MaxGuidedTourCalls   int         `json:"max_guided_tour_calls,omitempty"`
	MaxGuidedTourBytes   int         `json:"max_guided_tour_request_bytes,omitempty"`
	MaxSemanticCalls     int         `json:"max_semantic_calls"`
	MaxTargetedRounds    int         `json:"max_targeted_rounds"`
	MaxTotalRequestBytes int         `json:"max_total_request_bytes"`
}

func DefaultPolicy() Policy {
	return Policy{
		Version: PolicyVersion,
		Orientation: StageBudget{
			TargetRequestBytes: 80 << 10,
			MaxRequestBytes:    maxTechnicalRequestBytes,
			MaxFiles:           250,
		},
		Targeted: StageBudget{
			TargetRequestBytes: 64 << 10,
			MaxRequestBytes:    maxTechnicalRequestBytes,
			MaxFiles:           25,
			MaxEvidenceItems:   160,
			MaxSourceWindows:   25,
		},
		Architecture: StageBudget{
			TargetRequestBytes: 160 << 10,
			MaxRequestBytes:    maxTechnicalRequestBytes,
		},
		GuidedTour: StageBudget{
			TargetRequestBytes: 48 << 10,
			MaxRequestBytes:    guidedTourMaxRequestBytes,
		},
		MaxGuidedTourCalls:   1,
		MaxGuidedTourBytes:   guidedTourMaxRequestBytes,
		MaxSemanticCalls:     coreSemanticCallLimit + 1,
		MaxTargetedRounds:    2,
		MaxTotalRequestBytes: coreTotalRequestBytes + guidedTourMaxRequestBytes,
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
	legacy := p.MaxSemanticCalls == coreSemanticCallLimit && p.GuidedTour == (StageBudget{}) &&
		p.MaxGuidedTourCalls == 0 && p.MaxGuidedTourBytes == 0
	current := p.MaxGuidedTourCalls > 0 && p.MaxGuidedTourCalls <= maxGuidedTourSemanticCalls &&
		p.MaxGuidedTourBytes > 0 && p.MaxGuidedTourBytes <= maxGuidedTourAggregateBytes &&
		p.MaxSemanticCalls == coreSemanticCallLimit+p.MaxGuidedTourCalls &&
		validStageBudget(p.GuidedTour)
	if !legacy && !current {
		return fmt.Errorf("model research: semantic call policy must be legacy core-only or bounded guided-tour")
	}
	if p.MaxTargetedRounds < 0 || p.MaxTargetedRounds > 2 {
		return fmt.Errorf("model research: targeted round limit must be between 0 and 2")
	}
	minimumTotal := p.Orientation.MaxRequestBytes + p.Architecture.MaxRequestBytes
	if current {
		minimumTotal = coreTotalRequestBytes + p.MaxGuidedTourBytes
	}
	if p.MaxTotalRequestBytes < minimumTotal {
		return fmt.Errorf("model research: total request budget cannot hold the configured core and guided stages")
	}
	return nil
}

func validStageBudget(budget StageBudget) bool {
	return budget.TargetRequestBytes > 0 && budget.MaxRequestBytes >= budget.TargetRequestBytes
}

// WithGuidedTour upgrades a valid legacy four-call policy to the backward-
// compatible optional fifth editorial stage. The policy version stays stable
// because the analysis-stage contracts and their cache fingerprints do not
// change; only the new guided-tour contract consumes the added allowance.
func (p Policy) WithGuidedTour() (Policy, error) {
	return p.WithGuidedTourBudget(1, guidedTourMaxRequestBytes)
}

// WithGuidedTourBudget reserves a bounded aggregate editorial budget without
// treating request count itself as a quality objective. Callers choose a
// decomposition, then account every cache miss against these ceilings.
func (p Policy) WithGuidedTourBudget(maxCalls, maxRequestBytes int) (Policy, error) {
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	if maxCalls <= 0 || maxCalls > maxGuidedTourSemanticCalls || maxRequestBytes <= 0 ||
		maxRequestBytes > maxGuidedTourAggregateBytes {
		return Policy{}, fmt.Errorf("model research: invalid guided tour aggregate budget")
	}
	if p.MaxGuidedTourCalls >= maxCalls && p.MaxGuidedTourBytes >= maxRequestBytes {
		return p, nil
	}
	defaults := DefaultPolicy()
	p.GuidedTour = defaults.GuidedTour
	p.MaxGuidedTourCalls = max(p.MaxGuidedTourCalls, maxCalls)
	p.MaxGuidedTourBytes = max(p.MaxGuidedTourBytes, maxRequestBytes)
	p.MaxSemanticCalls = coreSemanticCallLimit + p.MaxGuidedTourCalls
	p.MaxTotalRequestBytes = coreTotalRequestBytes + p.MaxGuidedTourBytes
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
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
