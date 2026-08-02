package report

import (
	"strings"
	"testing"
)

func architectureSynthesisV4AcceptedFixture() ArchitectureSynthesisStatus {
	return ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisSucceeded,
		RequestBytes: 100, ResponseBytes: 90, ResponseContentBytes: 80,
		ProviderRequestCount: 1, TransportAttempts: 1,
		CandidateCount: 2, AnchorCount: 1,
		MembershipCounted: true, MemberOccurrences: 2, DistinctMembers: 2,
		UsageReported: true, InputTokens: 25, OutputTokens: 11,
		FinishReason: "stop", ResponseComplete: true, ResponseState: "captured",
		ProviderCallSucceeded: true, ResponseParsed: true, ProposalAccepted: true,
		ArchitectureSource: "model", ArchitectureLevel: 2,
	}
}

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
		{"request bytes", func(value *ArchitectureSynthesisStatus) { value.RequestBytes = 1 }},
		{"response bytes", func(value *ArchitectureSynthesisStatus) { value.ResponseBytes = 1 }},
		{"response content bytes", func(value *ArchitectureSynthesisStatus) { value.ResponseContentBytes = 1 }},
		{"transport attempts", func(value *ArchitectureSynthesisStatus) { value.TransportAttempts = 1 }},
		{"candidate count", func(value *ArchitectureSynthesisStatus) { value.CandidateCount = 1 }},
		{"anchor count", func(value *ArchitectureSynthesisStatus) { value.AnchorCount = 1 }},
		{"membership count", func(value *ArchitectureSynthesisStatus) {
			value.MembershipCounted = true
			value.MemberOccurrences = 1
			value.DistinctMembers = 1
		}},
		{"usage", func(value *ArchitectureSynthesisStatus) {
			value.UsageReported = true
			value.InputTokens = 1
			value.OutputTokens = 1
		}},
		{"finish reason", func(value *ArchitectureSynthesisStatus) { value.FinishReason = "stop"; value.ResponseComplete = true }},
		{"response state", func(value *ArchitectureSynthesisStatus) { value.ResponseState = "captured" }},
		{"diagnostic code", func(value *ArchitectureSynthesisStatus) {
			value.ValidationCodes = []string{"proposal.conflicting_membership"}
		}},
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

func TestArchitectureSynthesisV5SuccessRequiresAcceptedEnrichmentAndExactEvidence(t *testing.T) {
	t.Parallel()
	base := architectureSynthesisV4AcceptedFixture()
	if err := base.Validate(); err != nil {
		t.Fatalf("valid succeeded status: %v", err)
	}
	cached := base
	cached.State = ArchitectureSynthesisCached
	cached.ProviderRequestCount = 0
	cached.TransportAttempts = 0
	if err := cached.Validate(); err != nil {
		t.Fatalf("valid cached status: %v", err)
	}
	for name, mutate := range map[string]func(*ArchitectureSynthesisStatus){
		"missing acceptance":         func(value *ArchitectureSynthesisStatus) { value.ProposalAccepted = false },
		"rejected":                   func(value *ArchitectureSynthesisStatus) { value.ProposalRejected = true },
		"fallback":                   func(value *ArchitectureSynthesisStatus) { value.FallbackSelected = true; value.FallbackReason = "x" },
		"wrong succeeded count":      func(value *ArchitectureSynthesisStatus) { value.ProviderRequestCount = 0 },
		"missing request bytes":      func(value *ArchitectureSynthesisStatus) { value.RequestBytes = 0 },
		"missing candidate count":    func(value *ArchitectureSynthesisStatus) { value.CandidateCount = 0 },
		"missing transport evidence": func(value *ArchitectureSynthesisStatus) { value.TransportAttempts = 0 },
		"uncounted membership": func(value *ArchitectureSynthesisStatus) {
			value.MembershipCounted = false
			value.MemberOccurrences = 0
			value.DistinctMembers = 0
		},
		"empty membership": func(value *ArchitectureSynthesisStatus) {
			value.MemberOccurrences = 0
			value.DistinctMembers = 0
		},
		"incomplete stop": func(value *ArchitectureSynthesisStatus) { value.ResponseComplete = false },
		"completion unknown": func(value *ArchitectureSynthesisStatus) {
			value.FinishReason = ""
			value.ResponseComplete = false
		},
		"uncaptured response": func(value *ArchitectureSynthesisStatus) { value.ResponseState = "sensitive_omitted" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("accepted invalid v3 status: %#v", candidate)
			}
		})
	}
	manyToMany := base
	manyToMany.MemberOccurrences = 3
	if err := manyToMany.Validate(); err != nil {
		t.Fatalf("v5 rejected many-to-many membership evidence: %v", err)
	}
	legacyV4 := manyToMany
	legacyV4.Version = 4
	if err := legacyV4.Validate(); err == nil {
		t.Fatal("v4 status reinterpreted many-to-many membership under the v5 contract")
	}
}

func TestArchitectureSynthesisV4UncalledFailureRejectsProviderResponseEvidence(t *testing.T) {
	base := ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion,
		State:   ArchitectureSynthesisFailed, ErrorCode: "provider_error",
		RequestBytes: 100, CandidateCount: 2,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid pre-provider failure: %v", err)
	}
	for name, mutate := range map[string]func(*ArchitectureSynthesisStatus){
		"response": func(value *ArchitectureSynthesisStatus) {
			value.ResponseBytes = 10
			value.ResponseContentBytes = 10
			value.ResponseState = "captured"
		},
		"membership": func(value *ArchitectureSynthesisStatus) {
			value.MembershipCounted = true
			value.MemberOccurrences = 1
			value.DistinctMembers = 1
		},
		"usage": func(value *ArchitectureSynthesisStatus) {
			value.UsageReported = true
			value.InputTokens = 1
		},
		"completion": func(value *ArchitectureSynthesisStatus) {
			value.FinishReason = "stop"
			value.ResponseComplete = true
		},
		"validation": func(value *ArchitectureSynthesisStatus) {
			value.ValidationCodes = []string{"response.no_json"}
			value.ResponseParsed = true
			value.ProposalRejected = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("accepted uncalled provider evidence: %#v", candidate)
			}
		})
	}
}

func TestArchitectureSynthesisV5AcceptsExactClosedProducerDiagnosticRegistry(t *testing.T) {
	producerCodes := []string{
		"proposal.components_per_subsystem_above_preferred",
		"proposal.conflicting_membership",
		"proposal.duplicate_component_identity",
		"proposal.duplicate_member_id",
		"proposal.invalid_component",
		"proposal.invalid_member_id",
		"proposal.invalid_members",
		"proposal.member_participation_limit_exceeded",
		"proposal.membership_limit_exceeded",
		"proposal.invalid_subsystem",
		"proposal.invalid_subsystem_count",
		"proposal.no_usable_subsystems",
		"proposal.normalized_components_per_subsystem",
		"proposal.normalized_description",
		"proposal.normalized_package_only_hypothesis",
		"proposal.normalized_primary_subsystems",
		"proposal.normalized_total_components",
		"proposal.omitted_members_exceed_bounds",
		"proposal.omitted_members_preserved",
		"proposal.omitted_process_entry_member",
		"proposal.primary_subsystems_above_preferred",
		"proposal.total_components_above_preferred",
		"proposal.ungrounded_primary_component",
		"proposal.unknown_anchor_id",
		"proposal.unknown_member_id",
		"proposal.unsupported_version",
		"response.ambiguous_json",
		"response.embedded_json_extracted",
		"response.fenced_json_extracted",
		"response.invalid_proposal",
		"response.no_json",
		"response.sensitive_omitted",
		"response.unknown_fields_ignored",
		"response.incomplete",
		"response.membership_unavailable",
		"response.not_captured",
		"status.invalid_evidence",
	}
	if len(architectureStatusValidationCodes) != len(producerCodes) {
		t.Fatalf(
			"Architecture diagnostic registry has %d codes, producer inventory has %d",
			len(architectureStatusValidationCodes),
			len(producerCodes),
		)
	}
	for _, code := range producerCodes {
		if !validArchitectureStatusCode(code) {
			t.Errorf("producer diagnostic code %q is absent from the closed status registry", code)
		}
	}

	base := architectureSynthesisV4AcceptedFixture()
	base.State = ArchitectureSynthesisFailed
	base.ErrorCode = "invalid_response"
	base.ProposalAccepted = false
	base.ProposalRejected = true
	base.ArchitectureSource = ""
	base.ArchitectureLevel = 0
	base.ValidationCodes = producerCodes
	if err := base.Validate(); err != nil {
		t.Fatalf("complete closed producer diagnostics: %v", err)
	}
}

func TestArchitectureSynthesisV4RejectsUnknownHistoricalAndDuplicateDiagnosticCodes(t *testing.T) {
	for _, code := range []string{
		"proposal.future_code",
		"proposal.excess_primary_pillars",
		"response.too_large",
		"response.INVALID",
		"",
	} {
		t.Run(code, func(t *testing.T) {
			candidate := architectureSynthesisV4AcceptedFixture()
			candidate.ValidationCodes = []string{code}
			if err := candidate.Validate(); err == nil {
				t.Fatalf("Architecture status accepted non-current diagnostic code %q", code)
			}
		})
	}

	duplicate := architectureSynthesisV4AcceptedFixture()
	duplicate.ValidationCodes = []string{
		"proposal.omitted_members_preserved",
		"proposal.omitted_members_preserved",
	}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("Architecture status accepted duplicate validation diagnostic codes")
	}
}

func TestArchitectureFailureWarningKeepsLocalCanvasAuthoritative(t *testing.T) {
	t.Parallel()
	status := &ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisFailed,
		ErrorCode: "invalid_response", ProposalRejected: true,
		RequestBytes: 100, ResponseBytes: 90, ResponseContentBytes: 80,
		ProviderRequestCount: 1, TransportAttempts: 1, CandidateCount: 2,
		MembershipCounted: true, MemberOccurrences: 3, DistinctMembers: 2,
		UsageReported: true, InputTokens: 25, OutputTokens: 11,
		FinishReason: "stop", ResponseComplete: true, ResponseState: "captured",
		ProviderCallSucceeded: true, ResponseParsed: true,
		ValidationCodes: []string{"proposal.conflicting_membership"},
	}
	warning := architectureSynthesisUserWarning(status)
	if !strings.Contains(warning, "exact local Architecture Canvas is shown") ||
		strings.Contains(strings.ToLower(warning), "fallback") {
		t.Fatalf("failure warning = %q", warning)
	}
}
