package modelresearch

import "testing"

func TestDefaultPolicyCapsNormalRun(t *testing.T) {
	policy := DefaultPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if policy.MaxSemanticCalls != 4 || policy.MaxTargetedRounds != 2 {
		t.Fatalf("call limits = %d/%d, want 4/2", policy.MaxSemanticCalls, policy.MaxTargetedRounds)
	}
	if policy.MaxTotalRequestBytes != 4<<20 {
		t.Fatalf("total request budget = %d, want %d", policy.MaxTotalRequestBytes, 4<<20)
	}
	if policy.Architecture.MaxRequestBytes != 1<<20 {
		t.Fatalf("architecture technical ceiling = %d, want %d", policy.Architecture.MaxRequestBytes, 1<<20)
	}
}

func TestPolicyRejectsFifthSemanticCall(t *testing.T) {
	policy := DefaultPolicy()
	allowed, reason := policy.Allows(policy.Targeted, Usage{
		SemanticCalls: policy.MaxSemanticCalls,
		RequestBytes:  100 << 10,
	}, 10<<10)
	if allowed || reason != "call_budget_exhausted" {
		t.Fatalf("Allows() = %t, %q, want false call_budget_exhausted", allowed, reason)
	}
}

func TestPolicyTreatsStageTargetsAsSoft(t *testing.T) {
	policy := DefaultPolicy()
	for name, stage := range map[string]StageBudget{
		"orientation":  policy.Orientation,
		"targeted":     policy.Targeted,
		"architecture": policy.Architecture,
	} {
		requestBytes := stage.TargetRequestBytes + 1
		allowed, reason := policy.Allows(stage, Usage{}, requestBytes)
		if !allowed || reason != "within_budget" {
			t.Errorf("%s Allows(%d) = %t, %q, want true within_budget", name, requestBytes, allowed, reason)
		}
	}
}

func TestPolicyRejectsTotalRequestOverflow(t *testing.T) {
	policy := DefaultPolicy()
	allowed, reason := policy.Allows(policy.Architecture, Usage{
		SemanticCalls: 3,
		RequestBytes:  policy.MaxTotalRequestBytes - 1024,
	}, 2048)
	if allowed || reason != "total_byte_budget_exhausted" {
		t.Fatalf("Allows() = %t, %q, want false total_byte_budget_exhausted", allowed, reason)
	}
}
