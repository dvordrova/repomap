package report

import (
	"testing"
)

// Phase 1 prompt cleanup regression: a nested-grammar Architecture response
// whose components omit anchor_refs normalizes with the counted
// proposal.normalized_missing_anchor_refs code. The saved status must accept
// that code (live miniflux corpus run failed on the status validator before
// the vocabulary was extended).
func TestArchitectureStatusAcceptsNormalizedMissingAnchorRefs(t *testing.T) {
	t.Parallel()
	status := ArchitectureSynthesisStatus{
		Version: ArchitectureSynthesisStatusVersion,
		State:   ArchitectureSynthesisSucceeded,
		// Exact completion + membership evidence for an accepted live call.
		ResponseState:            "captured",
		ResponseComplete:         true,
		FinishReason:             "stop",
		MembershipCounted:        true,
		MemberOccurrences:        6,
		DistinctMembers:          6,
		RequestedConceptualCount: 6,
		LocalCandidateCount:      6,
		ProviderRequestCount:     1,
		TransportAttempts:        1,
		RequestBytes:             500,
		ProviderCallSucceeded:    true,
		ResponseParsed:           true,
		ProposalAccepted:         true,
		ResponseBytes:            1000,
		ResponseContentBytes:     1000,
		CoveredConceptualCount:   6,
		ProposalNormalized:       true,
		NormalizationCount:       1,
		ArchitectureSource:       "model",
		ArchitectureLevel:        1,
		ValidationCodes:          []string{"proposal.normalized_missing_anchor_refs"},
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("status with normalized_missing_anchor_refs rejected: %v", err)
	}
}

// The full set of diagnostics emitted by the current Architecture resolver
// must all be accepted by the status vocabulary (live corpus regression).
func TestArchitectureStatusVocabularyCoversResolverDiagnostics(t *testing.T) {
	t.Parallel()
	for _, code := range []string{
		"proposal.unknown_member_id",
		"proposal.unknown_anchor_id",
		"proposal.unknown_unit_ref",
		"proposal.duplicate_member_id",
		"proposal.duplicate_anchor_id",
		"proposal.duplicate_unit_ref",
		"proposal.normalized_missing_anchor_refs",
	} {
		if !validArchitectureStatusCodeForVersion(ArchitectureSynthesisStatusVersion, code) {
			t.Fatalf("resolver diagnostic %q is not accepted by the current status vocabulary", code)
		}
	}
}
