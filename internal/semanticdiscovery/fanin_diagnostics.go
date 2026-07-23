package semanticdiscovery

import "strings"

const MaxFanInReductionReasons = 16

const (
	FanInReasonUnknownRepositoryReference = "unknown_repository_reference"
	FanInReasonUnsupportedSequence        = "unsupported_sequence_language"
	FanInReasonLimitationNotExplicit      = "limitation_not_explicit"
	FanInReasonUnknownOpaqueID            = "unknown_opaque_id"
	FanInReasonCrossCandidateSupport      = "cross_candidate_support"
	FanInReasonIntentRetention            = "intent_retention_failure"
	FanInReasonProposalContract           = "proposal_contract_rejected"
	FanInReasonLocalSequenceScope         = "local_sequence_scope_violation"
)

func diagnoseFanInProposal(
	context fanInContext,
	proposal ArtifactProposal,
	validationErr error,
) []FanInReductionReason {
	reasons := make([]FanInReductionReason, 0, 4)
	add := func(reason FanInReductionReason) {
		if len(reasons) >= MaxFanInReductionReasons {
			return
		}
		for _, existing := range reasons {
			if sameFanInReductionReason(existing, reason) {
				return
			}
		}
		reason.SupportIDs = boundedKnownSupportIDs(reason.SupportIDs, context.bundleFacts)
		reasons = append(reasons, reason)
	}

	for _, field := range proposalRepositoryTextFields(proposal) {
		if repositoryReferencePattern.MatchString(field.value) {
			add(FanInReductionReason{
				Code:       FanInReasonUnknownRepositoryReference,
				Field:      field.name,
				ClaimIndex: field.claimIndex,
			})
		}
	}

	for claimIndex, claim := range proposal.Claims {
		facts := factsForKnownIDs(claim.SupportIDs, context.bundleFacts)
		_, missingCapabilities, missingErr := resolveMissingRefs(context, claim.MissingRefs)
		sequenceExplicitlyMissing := missingErr == nil &&
			missingSupportsCapability(missingCapabilities, CapabilitySequence)
		if sequencePattern.MatchString(claim.Text) &&
			(!factsSupportCapability(facts, CapabilitySequence) ||
				!hasCapabilityOverlap(claim.Text, facts, CapabilitySequence)) &&
			!sequenceExplicitlyMissing {
			add(FanInReductionReason{
				Code:       FanInReasonUnsupportedSequence,
				Field:      "claim.text",
				ClaimIndex: fanInDiagnosticIndex(claimIndex),
				SupportIDs: claim.SupportIDs,
			})
		}
		if claim.Basis == ClaimUnresolved &&
			(len(claim.MissingRefs) == 0 || !hasExplicitLimitation(claim.Text)) {
			add(FanInReductionReason{
				Code:       FanInReasonLimitationNotExplicit,
				Field:      "claim.text",
				ClaimIndex: fanInDiagnosticIndex(claimIndex),
				SupportIDs: claim.SupportIDs,
			})
		}
		for _, id := range claim.SupportIDs {
			if _, exists := context.bundleFacts[id]; !exists {
				add(FanInReductionReason{
					Code:       FanInReasonUnknownOpaqueID,
					Field:      "claim.support_ids",
					ClaimIndex: fanInDiagnosticIndex(claimIndex),
				})
				break
			}
		}
		if err := validateFanInClaimCandidateLineage(context, proposal.CandidateID, claim); err != nil {
			message := err.Error()
			switch {
			case strings.Contains(message, "unknown"):
				add(FanInReductionReason{
					Code:       FanInReasonUnknownOpaqueID,
					Field:      "claim.refs",
					ClaimIndex: fanInDiagnosticIndex(claimIndex),
				})
			case strings.Contains(message, "borrow") || strings.Contains(message, "candidate leaf"):
				add(FanInReductionReason{
					Code:       FanInReasonCrossCandidateSupport,
					Field:      "claim.refs",
					ClaimIndex: fanInDiagnosticIndex(claimIndex),
					SupportIDs: claim.SupportIDs,
				})
			}
		}
	}

	if candidate, exists := context.candidates[proposal.CandidateID]; exists {
		if err := validateIntentProposal(candidate, proposal, context.bundleFacts); err != nil {
			add(FanInReductionReason{Code: FanInReasonIntentRetention, Field: "artifact.intent"})
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, classifyFanInValidationError(validationErr))
	}
	return reasons
}

type proposalRepositoryTextField struct {
	name       string
	value      string
	claimIndex *int
}

func proposalRepositoryTextFields(proposal ArtifactProposal) []proposalRepositoryTextField {
	fields := []proposalRepositoryTextField{
		{name: "artifact.title", value: proposal.Title},
		{name: "artifact.summary", value: proposal.Summary},
	}
	for _, alias := range proposal.Aliases {
		fields = append(fields, proposalRepositoryTextField{name: "artifact.alias", value: alias})
	}
	for _, question := range proposal.LikelyQuestions {
		fields = append(fields, proposalRepositoryTextField{name: "artifact.likely_question", value: question})
	}
	for claimIndex, claim := range proposal.Claims {
		index := fanInDiagnosticIndex(claimIndex)
		fields = append(fields,
			proposalRepositoryTextField{name: "claim.title", value: claim.Title, claimIndex: index},
			proposalRepositoryTextField{name: "claim.text", value: claim.Text, claimIndex: index},
		)
	}
	return fields
}

func classifyFanInValidationError(err error) FanInReductionReason {
	reason := FanInReductionReason{Code: FanInReasonProposalContract}
	if err == nil {
		return reason
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "repository-bearing"):
		reason.Code = FanInReasonUnknownRepositoryReference
	case strings.Contains(message, "sequence-capable") || strings.Contains(message, "ordering"):
		reason.Code = FanInReasonUnsupportedSequence
	case strings.Contains(message, "explicit limitation"):
		reason.Code = FanInReasonLimitationNotExplicit
	case strings.Contains(message, "unknown"):
		reason.Code = FanInReasonUnknownOpaqueID
	case strings.Contains(message, "borrow") || strings.Contains(message, "candidate fact scope"):
		reason.Code = FanInReasonCrossCandidateSupport
	case strings.Contains(message, "aspect") || strings.Contains(message, "original question") ||
		strings.Contains(message, "aliases"):
		reason.Code = FanInReasonIntentRetention
	}
	return reason
}

func boundedKnownSupportIDs(ids []string, known map[string]Fact) []string {
	result := make([]string, 0, len(ids))
	for _, id := range sortedUnique(ids) {
		if _, exists := known[id]; !exists {
			continue
		}
		result = append(result, id)
		if len(result) == maxKeywordsPerFact {
			break
		}
	}
	return result
}

func fanInDiagnosticIndex(index int) *int {
	value := index
	return &value
}

func sameFanInReductionReason(left, right FanInReductionReason) bool {
	if left.Code != right.Code || left.Field != right.Field {
		return false
	}
	if left.ClaimIndex == nil || right.ClaimIndex == nil {
		return left.ClaimIndex == nil && right.ClaimIndex == nil
	}
	return *left.ClaimIndex == *right.ClaimIndex
}
