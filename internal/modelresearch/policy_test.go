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
	if policy.MaxTotalRequestBytes != 320<<10 {
		t.Fatalf("total request budget = %d, want %d", policy.MaxTotalRequestBytes, 320<<10)
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
