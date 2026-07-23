package semanticdiscovery

// VerdictInput contains only locally validated semantic state. Model prose and
// a model-proposed verdict are intentionally absent: neither is authority for
// the canonical result.
type VerdictInput struct {
	ClaimBases     []ClaimBasis
	Contradictions []LeafContradiction
	// CentralClaimContradicted is set only by a local validator that can bind
	// a contradiction to the mechanism's central claim. Generic retained leaf
	// contradictions remain gaps and therefore contribute to mixed instead.
	CentralClaimContradicted bool
	RequiredAspectIDs        []string
	CoveredAspectIDs         []string
	RetainedMissingEvidence  int
}

type VerdictReason string

const (
	VerdictReasonCentralClaimContradicted VerdictReason = "central_claim_contradicted"
	VerdictReasonContradictionRetained    VerdictReason = "contradiction_retained"
	VerdictReasonNoSupportedClaim         VerdictReason = "no_supported_claim"
	VerdictReasonUnresolvedClaimPresent   VerdictReason = "unresolved_claim_present"
	VerdictReasonMissingEvidenceRetained  VerdictReason = "missing_evidence_retained"
	VerdictReasonRequiredAspectUncovered  VerdictReason = "required_aspect_uncovered"
	VerdictReasonResolvedClaimPresent     VerdictReason = "resolved_claim_present"
	VerdictReasonRequiredAspectsCovered   VerdictReason = "required_aspects_covered"
)

// DeriveVerdict is the canonical local verdict reducer. Its caller must first
// validate claim support, opaque IDs, temporal/scope semantics, and intent.
func DeriveVerdict(input VerdictInput) Verdict {
	verdict, _ := deriveVerdict(input)
	return verdict
}

func validModelVerdict(verdict Verdict) bool {
	switch verdict {
	case VerdictSupported, VerdictMixed, VerdictInsufficientEvidence:
		return true
	default:
		return false
	}
}

func validCanonicalVerdict(verdict Verdict) bool {
	return validModelVerdict(verdict) || verdict == VerdictUnsupported
}

func deriveVerdict(input VerdictInput) (Verdict, []VerdictReason) {
	if input.CentralClaimContradicted {
		return VerdictUnsupported, []VerdictReason{VerdictReasonCentralClaimContradicted}
	}

	hasResolved := false
	hasUnresolved := false
	for _, basis := range input.ClaimBases {
		switch basis {
		case ClaimDirect, ClaimCompositional, ClaimInterpretive:
			hasResolved = true
		case ClaimUnresolved:
			hasUnresolved = true
		}
	}
	if !hasResolved {
		return VerdictInsufficientEvidence, []VerdictReason{VerdictReasonNoSupportedClaim}
	}

	reasons := make([]VerdictReason, 0, 3)
	if hasUnresolved {
		reasons = append(reasons, VerdictReasonUnresolvedClaimPresent)
	}
	if input.RetainedMissingEvidence > 0 {
		reasons = append(reasons, VerdictReasonMissingEvidenceRetained)
	}
	if len(input.Contradictions) > 0 {
		reasons = append(reasons, VerdictReasonContradictionRetained)
	}
	if hasUncoveredRequiredAspect(input.RequiredAspectIDs, input.CoveredAspectIDs) {
		reasons = append(reasons, VerdictReasonRequiredAspectUncovered)
	}
	if len(reasons) > 0 {
		return VerdictMixed, reasons
	}
	return VerdictSupported, []VerdictReason{
		VerdictReasonResolvedClaimPresent,
		VerdictReasonRequiredAspectsCovered,
	}
}

func hasUncoveredRequiredAspect(required, covered []string) bool {
	coveredSet := make(map[string]struct{}, len(covered))
	for _, id := range covered {
		coveredSet[id] = struct{}{}
	}
	for _, id := range required {
		if _, exists := coveredSet[id]; !exists {
			return true
		}
	}
	return false
}

func verdictInputForProposal(
	proposal ArtifactProposal,
	candidate OpportunityCandidate,
	contradictions []LeafContradiction,
	retainedMissingEvidence int,
	known map[string]Fact,
) VerdictInput {
	input := VerdictInput{
		ClaimBases:              make([]ClaimBasis, 0, len(proposal.Claims)),
		Contradictions:          append([]LeafContradiction(nil), contradictions...),
		RetainedMissingEvidence: retainedMissingEvidence,
	}
	for _, claim := range proposal.Claims {
		input.ClaimBases = append(input.ClaimBases, claim.Basis)
	}
	if candidate.IntentContract == nil {
		return input
	}
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		input.RequiredAspectIDs = append(input.RequiredAspectIDs, aspect.ID)
	}
	input.CoveredAspectIDs, _, _ = deriveIntentCoverage(candidate, proposal.Claims, known)
	return input
}
