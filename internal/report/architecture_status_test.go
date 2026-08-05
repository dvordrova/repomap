package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

func architectureSynthesisV4AcceptedFixture() ArchitectureSynthesisStatus {
	return ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisSucceeded,
		RequestBytes: 100, ResponseBytes: 90, ResponseContentBytes: 80,
		ProviderRequestCount: 1, TransportAttempts: 1,
		LocalCandidateCount: 3, RequestedConceptualCount: 2, StructuralLocatorCount: 1,
		AnchorCount:       1,
		MembershipCounted: true, MemberOccurrences: 2, DistinctMembers: 2,
		CoveredConceptualCount: 2,
		UsageReported:          true, InputTokens: 25, OutputTokens: 11,
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
		UnavailableCode: ArchitectureSynthesisUnavailableOfflineCode,
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("offline Architecture status: %v", err)
	}
	exactGraphUnavailable := status
	exactGraphUnavailable.UnavailableCode = ArchitectureSynthesisUnavailableExactWorkspaceGraphCode
	if err := exactGraphUnavailable.Validate(); err != nil {
		t.Fatalf("exact-graph Architecture status: %v", err)
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
		{"local candidate count", func(value *ArchitectureSynthesisStatus) { value.LocalCandidateCount = 1 }},
		{"requested conceptual count", func(value *ArchitectureSynthesisStatus) { value.RequestedConceptualCount = 1 }},
		{"structural locator count", func(value *ArchitectureSynthesisStatus) { value.StructuralLocatorCount = 1 }},
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

func TestArchitectureSynthesisV7SuccessRequiresExactConceptualCoverageAndTruthfulLocalRoles(t *testing.T) {
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
		"missing conceptual count":   func(value *ArchitectureSynthesisStatus) { value.RequestedConceptualCount = 0 },
		"wrong local role sum":       func(value *ArchitectureSynthesisStatus) { value.LocalCandidateCount++ },
		"legacy candidate count":     func(value *ArchitectureSynthesisStatus) { value.CandidateCount = value.LocalCandidateCount },
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
		"incomplete distinct coverage": func(value *ArchitectureSynthesisStatus) {
			value.DistinctMembers = value.RequestedConceptualCount - 1
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
	legacyV4.CandidateCount = legacyV4.RequestedConceptualCount
	legacyV4.LocalCandidateCount = 0
	legacyV4.RequestedConceptualCount = 0
	legacyV4.StructuralLocatorCount = 0
	legacyV4.CoveredConceptualCount = 0
	if err := legacyV4.Validate(); err == nil {
		t.Fatal("v4 status reinterpreted many-to-many membership under the v5 contract")
	}
}

func TestArchitectureSynthesisV8AcceptsExactPartialCoverage(t *testing.T) {
	t.Parallel()

	status := architectureSynthesisV4AcceptedFixture()
	status.LocalCandidateCount = 4
	status.RequestedConceptualCount = 3
	status.StructuralLocatorCount = 1
	status.MemberOccurrences = 2
	status.DistinctMembers = 2
	status.CoveredConceptualCount = 2
	status.UncoveredConceptualCount = 1
	status.UncoveredConceptualIDs = []componentmap.MemberID{{
		Kind: componentmap.MemberPackage, Value: "member-package-service",
	}}
	status.ProposalPartial = true
	status.ArchitectureSource = "partial_model"
	if err := status.Validate(); err != nil {
		t.Fatalf("valid partial Architecture status: %v", err)
	}

	cached := status
	cached.State = ArchitectureSynthesisCached
	cached.ProviderRequestCount = 0
	cached.TransportAttempts = 0
	if err := cached.Validate(); err != nil {
		t.Fatalf("valid cached partial Architecture status: %v", err)
	}

	for name, mutate := range map[string]func(*ArchitectureSynthesisStatus){
		"missing partial flag": func(value *ArchitectureSynthesisStatus) { value.ProposalPartial = false },
		"covered mismatch":     func(value *ArchitectureSynthesisStatus) { value.CoveredConceptualCount++ },
		"uncovered mismatch":   func(value *ArchitectureSynthesisStatus) { value.UncoveredConceptualCount++ },
		"missing exact id":     func(value *ArchitectureSynthesisStatus) { value.UncoveredConceptualIDs = nil },
		"duplicate exact id": func(value *ArchitectureSynthesisStatus) {
			value.UncoveredConceptualCount = 2
			value.RequestedConceptualCount = 4
			value.LocalCandidateCount = 5
			value.UncoveredConceptualIDs = append(value.UncoveredConceptualIDs, value.UncoveredConceptualIDs[0])
		},
		"invalid exact id": func(value *ArchitectureSynthesisStatus) {
			value.UncoveredConceptualIDs[0].Kind = "unknown"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := status
			candidate.UncoveredConceptualIDs = append([]componentmap.MemberID(nil), status.UncoveredConceptualIDs...)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("partial Architecture status accepted %s", name)
			}
		})
	}
}

func TestArchitectureSynthesisV8KeepsV7FullCoverageReadable(t *testing.T) {
	t.Parallel()

	historical := architectureSynthesisV4AcceptedFixture()
	historical.Version = 7
	historical.CoveredConceptualCount = 0
	if err := historical.Validate(); err != nil {
		t.Fatalf("historical v7 full Architecture status is unreadable: %v", err)
	}
}

func TestArchitectureSynthesisV7RejectsD206PartialCoverageDiagnostic(t *testing.T) {
	t.Parallel()

	historical := architectureSynthesisV4AcceptedFixture()
	historical.Version = 7
	historical.CoveredConceptualCount = 0
	historical.ValidationCodes = []string{"proposal.partial_member_coverage"}
	if err := historical.Validate(); err == nil {
		t.Fatal("historical v7 status accepted the D206 partial-coverage diagnostic")
	}
}

func TestArchitectureSynthesisV7DoesNotReinterpretHistoricalCandidateCoverage(t *testing.T) {
	t.Parallel()

	historical := architectureSynthesisV4AcceptedFixture()
	historical.Version = 6
	historical.CandidateCount = historical.RequestedConceptualCount
	historical.LocalCandidateCount = 0
	historical.RequestedConceptualCount = 0
	historical.StructuralLocatorCount = 0
	historical.CoveredConceptualCount = 0
	if err := historical.Validate(); err != nil {
		t.Fatalf("historical v6 Architecture status is unreadable: %v", err)
	}

	current := historical
	current.Version = ArchitectureSynthesisStatusVersion
	if err := current.Validate(); err == nil {
		t.Fatal("v7 status reinterpreted the historical candidate count as conceptual coverage")
	}
}

func TestArchitectureSynthesisV4UncalledFailureRejectsProviderResponseEvidence(t *testing.T) {
	base := ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion,
		State:   ArchitectureSynthesisFailed, ErrorCode: "provider_error",
		RequestBytes: 100, LocalCandidateCount: 3, RequestedConceptualCount: 2,
		StructuralLocatorCount: 1,
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

func TestArchitectureSynthesisV6AcceptsExactClosedProducerDiagnosticRegistry(t *testing.T) {
	producerCodes := []string{
		"proposal.components_per_subsystem_above_preferred",
		"proposal.duplicate_component_identity",
		"proposal.duplicate_member_id",
		"proposal.invalid_component",
		"proposal.invalid_member_id",
		"proposal.invalid_members",
		"proposal.incomplete_member_coverage",
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
		"proposal.partial_member_coverage",
		"proposal.primary_subsystems_above_preferred",
		"proposal.total_components_above_preferred",
		"proposal.ungrounded_primary_component",
		"proposal.unknown_anchor_id",
		"proposal.unknown_member_id",
		"proposal.unsupported_version",
		"response.ambiguous_json",
		"response.invalid_proposal",
		"response.no_json",
		"response.sensitive_omitted",
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
	base.CoveredConceptualCount = 0
	base.ProposalRejected = true
	base.ArchitectureSource = ""
	base.ArchitectureLevel = 0
	base.ValidationCodes = producerCodes
	if err := base.Validate(); err != nil {
		t.Fatalf("complete closed producer diagnostics: %v", err)
	}
}

func TestArchitectureSynthesisV6RejectsRetiredDiagnosticsWhileV5RemainsReadable(t *testing.T) {
	for code := range architectureStatusHistoricalValidationCodes {
		t.Run(code, func(t *testing.T) {
			historical := architectureSynthesisV4AcceptedFixture()
			historical.Version = 5
			historical.CandidateCount = historical.RequestedConceptualCount
			historical.LocalCandidateCount = 0
			historical.RequestedConceptualCount = 0
			historical.StructuralLocatorCount = 0
			historical.CoveredConceptualCount = 0
			historical.ValidationCodes = []string{code}
			if err := historical.Validate(); err != nil {
				t.Fatalf("historical v5 diagnostic %q is unreadable: %v", code, err)
			}

			current := historical
			current.Version = ArchitectureSynthesisStatusVersion
			if err := current.Validate(); err == nil {
				t.Fatalf("current status accepted retired diagnostic %q", code)
			}
		})
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
		"proposal.invalid_members",
		"proposal.invalid_members",
	}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("Architecture status accepted duplicate validation diagnostic codes")
	}
}

func TestArchitectureSynthesisV9ProviderOutputLimitStatus(t *testing.T) {
	t.Parallel()
	base := func() ArchitectureSynthesisStatus {
		return ArchitectureSynthesisStatus{
			Version: 9, State: ArchitectureSynthesisFailed,
			ErrorCode:    ArchitectureSynthesisErrorProviderOutputLimit,
			RequestBytes: 5904, ResponseBytes: 201396, ResponseContentBytes: 201396,
			ProviderRequestCount: 1, TransportAttempts: 1,
			LocalCandidateCount: 3, RequestedConceptualCount: 2, StructuralLocatorCount: 1,
			AnchorCount: 4, UsageReported: true, InputTokens: 42197, OutputTokens: 64000,
			ConfiguredMaxTokens: 64000, ObservedOutputTokens: 64000,
			FinishReason: "length", ResponseComplete: false,
		}
	}
	t.Run("valid length-ended status", func(t *testing.T) {
		if err := base().Validate(); err != nil {
			t.Fatalf("valid v9 provider output limit status rejected: %v", err)
		}
	})
	t.Run("requires exact attempted request evidence", func(t *testing.T) {
		for _, mutate := range []func(*ArchitectureSynthesisStatus){
			func(status *ArchitectureSynthesisStatus) { status.ProviderRequestCount = 0 },
			func(status *ArchitectureSynthesisStatus) { status.RequestBytes = 0 },
			func(status *ArchitectureSynthesisStatus) { status.TransportAttempts = 0 },
		} {
			status := base()
			mutate(&status)
			if err := status.Validate(); err == nil {
				t.Fatalf("accepted incomplete attempted evidence: %#v", status)
			}
		}
	})
	t.Run("rejects partial response publication", func(t *testing.T) {
		for _, mutate := range []func(*ArchitectureSynthesisStatus){
			func(status *ArchitectureSynthesisStatus) { status.ProposalAccepted = true },
			func(status *ArchitectureSynthesisStatus) { status.MembershipCounted = true },
			func(status *ArchitectureSynthesisStatus) { status.ProviderCallSucceeded = true },
			func(status *ArchitectureSynthesisStatus) { status.ResponseParsed = true },
			func(status *ArchitectureSynthesisStatus) { status.ArchitectureSource = "model" },
			func(status *ArchitectureSynthesisStatus) { status.ValidationCodes = []string{"response.incomplete"} },
		} {
			status := base()
			mutate(&status)
			if err := status.Validate(); err == nil {
				t.Fatalf("accepted partial-response publication: %#v", status)
			}
		}
	})
	t.Run("rejects inconsistent token evidence", func(t *testing.T) {
		status := base()
		status.OutputTokens = 32000
		if err := status.Validate(); err == nil {
			t.Fatal("accepted mismatched observed tokens")
		}
		status = base()
		status.UsageReported = false
		if err := status.Validate(); err == nil {
			t.Fatal("accepted tokens without reported usage")
		}
	})
	t.Run("response byte overflow shape", func(t *testing.T) {
		status := base()
		status.FinishReason = "stop"
		status.ResponseComplete = true
		status.UsageReported = false
		status.InputTokens = 0
		status.OutputTokens = 0
		status.ObservedOutputTokens = 0
		if err := status.Validate(); err != nil {
			t.Fatalf("valid response-byte overflow status rejected: %v", err)
		}
	})
	t.Run("length claim rejected outside failed state", func(t *testing.T) {
		status := base()
		status.State = ArchitectureSynthesisSucceeded
		status.ErrorCode = ""
		status.ResponseComplete = true
		if err := status.Validate(); err == nil {
			t.Fatal("accepted length-ended claim in succeeded state")
		}
	})
	t.Run("v8 rejects v9 evidence fields", func(t *testing.T) {
		status := base()
		status.Version = 8
		if err := status.Validate(); err == nil {
			t.Fatal("v8 accepted v9 provider output limit evidence")
		}
	})
}

func TestArchitectureFailureWarningKeepsLocalCanvasAuthoritative(t *testing.T) {
	t.Parallel()
	status := &ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisFailed,
		ErrorCode: "invalid_response", ProposalRejected: true,
		RequestBytes: 100, ResponseBytes: 90, ResponseContentBytes: 80,
		ProviderRequestCount: 1, TransportAttempts: 1,
		LocalCandidateCount: 3, RequestedConceptualCount: 2, StructuralLocatorCount: 1,
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
