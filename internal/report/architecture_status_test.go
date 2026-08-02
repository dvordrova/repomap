package report

import (
	"strings"
	"testing"
)

func TestArchitectureSynthesisUnavailableIsExplicitAndProviderFree(t *testing.T) {
	t.Parallel()

	status := ArchitectureSynthesisStatus{
		Version:         ArchitectureSynthesisStatusVersion,
		State:           ArchitectureSynthesisUnavailable,
		UnavailableCode: "offline",
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("offline Architecture status: %v", err)
	}

	invalid := status
	invalid.ProviderRequestCount = 1
	if err := invalid.Validate(); err == nil {
		t.Fatal("offline Architecture status accepted a provider request")
	}
	invalid = status
	invalid.UnavailableCode = "provider_error"
	if err := invalid.Validate(); err == nil {
		t.Fatal("offline Architecture status accepted an open unavailable code")
	}
	mutations := []struct {
		name   string
		mutate func(*ArchitectureSynthesisStatus)
	}{
		{"prompt bytes", func(value *ArchitectureSynthesisStatus) { value.PromptBytes = 1 }},
		{"latency", func(value *ArchitectureSynthesisStatus) { value.LatencyMillis = 1 }},
		{"call succeeded", func(value *ArchitectureSynthesisStatus) { value.ProviderCallSucceeded = true }},
		{"response parsed", func(value *ArchitectureSynthesisStatus) { value.ResponseParsed = true }},
		{"accepted", func(value *ArchitectureSynthesisStatus) { value.ProposalAccepted = true }},
		{"normalized", func(value *ArchitectureSynthesisStatus) { value.ProposalNormalized = true }},
		{"rejected", func(value *ArchitectureSynthesisStatus) { value.ProposalRejected = true }},
		{"fallback", func(value *ArchitectureSynthesisStatus) { value.FallbackSelected = true }},
		{"source", func(value *ArchitectureSynthesisStatus) { value.ArchitectureSource = "model" }},
		{"level", func(value *ArchitectureSynthesisStatus) { value.ArchitectureLevel = 1 }},
		{"normalizations", func(value *ArchitectureSynthesisStatus) { value.NormalizationCount = 1 }},
		{"fallback reason", func(value *ArchitectureSynthesisStatus) { value.FallbackReason = "x" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := status
			mutation.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("offline Architecture status accepted %s", mutation.name)
			}
		})
	}
}

func TestArchitectureSynthesisV3SuccessRequiresAcceptedEnrichment(t *testing.T) {
	t.Parallel()
	base := ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisSucceeded,
		PromptBytes: 100, ProviderRequestCount: 1, ProviderCallSucceeded: true,
		ResponseParsed: true, ProposalAccepted: true, ArchitectureSource: "model",
		ArchitectureLevel: 2,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid succeeded status: %v", err)
	}
	cached := base
	cached.State = ArchitectureSynthesisCached
	cached.ProviderRequestCount = 0
	if err := cached.Validate(); err != nil {
		t.Fatalf("valid cached status: %v", err)
	}
	for name, mutate := range map[string]func(*ArchitectureSynthesisStatus){
		"missing acceptance":    func(value *ArchitectureSynthesisStatus) { value.ProposalAccepted = false },
		"rejected":              func(value *ArchitectureSynthesisStatus) { value.ProposalRejected = true },
		"fallback":              func(value *ArchitectureSynthesisStatus) { value.FallbackSelected = true; value.FallbackReason = "x" },
		"wrong succeeded count": func(value *ArchitectureSynthesisStatus) { value.ProviderRequestCount = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("accepted invalid v3 status: %#v", candidate)
			}
		})
	}
}

func TestArchitectureFailureWarningKeepsLocalCanvasAuthoritative(t *testing.T) {
	t.Parallel()
	status := &ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisFailed,
		ErrorCode: "invalid_response", ProposalRejected: true,
	}
	warning := architectureSynthesisUserWarning(status)
	if !strings.Contains(warning, "exact local Architecture Canvas is shown") ||
		strings.Contains(strings.ToLower(warning), "fallback") {
		t.Fatalf("failure warning = %q", warning)
	}
}
