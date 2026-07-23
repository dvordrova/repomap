package semanticdiscovery

import "fmt"

type AspectCoverageStatus string

const (
	AspectCoveredDirectly        AspectCoverageStatus = "covered_directly"
	AspectCoveredCompositionally AspectCoverageStatus = "covered_compositionally"
	AspectPartiallyCovered       AspectCoverageStatus = "partially_covered"
	AspectUncovered              AspectCoverageStatus = "uncovered"
)

type ClaimCoverageItem struct {
	ClaimIndex         int        `json:"claim_index"`
	Basis              ClaimBasis `json:"basis"`
	AspectIDs          []string   `json:"answer_aspect_ids,omitempty"`
	Temporal           bool       `json:"temporal"`
	SequenceSupportIDs []string   `json:"sequence_support_ids,omitempty"`
}

type AnswerAspectCoverage struct {
	AspectID     string               `json:"aspect_id"`
	Key          bool                 `json:"key,omitempty"`
	Status       AspectCoverageStatus `json:"status"`
	ClaimIndexes []int                `json:"claim_indexes,omitempty"`
	SupportIDs   []string             `json:"support_ids,omitempty"`
}

// ClaimCoverageAssessment is a local projection over a validated proposal.
// Available facts come from the validated leaf material shown to synthesis;
// they are not requirements that every artifact must cite.
type ClaimCoverageAssessment struct {
	CandidateID            string                 `json:"candidate_id"`
	AvailableFactIDs       []string               `json:"available_fact_ids"`
	UsedFactIDs            []string               `json:"used_fact_ids"`
	UnusedAvailableFactIDs []string               `json:"unused_available_fact_ids"`
	Claims                 []ClaimCoverageItem    `json:"claims"`
	AnswerAspects          []AnswerAspectCoverage `json:"answer_aspects"`
	CoveredAspectIDs       []string               `json:"covered_answer_aspects"`
	UncoveredAspectIDs     []string               `json:"uncovered_answer_aspects"`
	TemporalClaimIndexes   []int                  `json:"temporal_claim_indexes,omitempty"`
}

// AssessClaimCoverage validates the proposal through the normal fan-in path,
// then reports fact usage and claim-to-aspect coverage without treating unused
// available facts as missing answer content.
func AssessClaimCoverage(
	bundle Bundle,
	results []LeafResult,
	proposal ArtifactProposal,
) (ClaimCoverageAssessment, error) {
	_, context, err := validateLeafResults(bundle, results)
	if err != nil {
		return ClaimCoverageAssessment{}, err
	}
	if err := validateFanInProposal(context, proposal); err != nil {
		return ClaimCoverageAssessment{}, err
	}
	return assessClaimCoverage(context, proposal)
}

func assessClaimCoverage(
	context fanInContext,
	proposal ArtifactProposal,
) (ClaimCoverageAssessment, error) {
	candidate := canonicalOpportunityCandidate(
		context.candidates[proposal.CandidateID],
	)
	result := resultForCandidate(context, proposal.CandidateID)
	if result == nil {
		return ClaimCoverageAssessment{}, fmt.Errorf(
			"semantic discovery: claim coverage candidate has no validated leaf",
		)
	}
	assessment := ClaimCoverageAssessment{
		CandidateID: proposal.CandidateID,
		AvailableFactIDs: append(
			[]string(nil),
			result.Artifact.CandidateConnection.SupportIDs...,
		),
	}
	used := make(map[string]struct{})
	for claimIndex, claim := range proposal.Claims {
		addIDs(used, claim.SupportIDs)
		aspectIDs, err := claimAnswerAspectIDs(context, candidate, claim)
		if err != nil {
			return ClaimCoverageAssessment{}, err
		}
		sequenceSupport := make([]string, 0, len(claim.SupportIDs))
		for _, fact := range factsForKnownIDs(claim.SupportIDs, context.bundleFacts) {
			if factSupportsCapability(fact, CapabilitySequence) {
				sequenceSupport = append(sequenceSupport, fact.ID)
			}
		}
		item := ClaimCoverageItem{
			ClaimIndex:         claimIndex,
			Basis:              claim.Basis,
			AspectIDs:          aspectIDs,
			Temporal:           sequencePattern.MatchString(claim.Text),
			SequenceSupportIDs: sortedUnique(sequenceSupport),
		}
		assessment.Claims = append(assessment.Claims, item)
		if item.Temporal {
			assessment.TemporalClaimIndexes = append(
				assessment.TemporalClaimIndexes,
				claimIndex,
			)
		}
	}
	assessment.AvailableFactIDs = sortedUnique(assessment.AvailableFactIDs)
	assessment.UsedFactIDs = sortedSet(used)
	for _, id := range assessment.AvailableFactIDs {
		if _, exists := used[id]; !exists {
			assessment.UnusedAvailableFactIDs = append(
				assessment.UnusedAvailableFactIDs,
				id,
			)
		}
	}
	assessment.AnswerAspects = assessAnswerAspects(candidate, proposal, assessment.Claims)
	for _, aspect := range assessment.AnswerAspects {
		if aspect.Status == AspectUncovered {
			assessment.UncoveredAspectIDs = append(
				assessment.UncoveredAspectIDs,
				aspect.AspectID,
			)
			continue
		}
		assessment.CoveredAspectIDs = append(
			assessment.CoveredAspectIDs,
			aspect.AspectID,
		)
	}
	return assessment, nil
}

func claimAnswerAspectIDs(
	context fanInContext,
	candidate OpportunityCandidate,
	claim ProposedClaim,
) ([]string, error) {
	if candidate.IntentContract == nil {
		return nil, nil
	}
	if claim.Basis != ClaimUnresolved {
		ids := make([]string, 0, len(candidate.IntentContract.RequiredAnswerAspects))
		for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
			if claimCoversAnswerAspect(claim, context.bundleFacts, aspect) {
				ids = append(ids, aspect.ID)
			}
		}
		return sortedUnique(ids), nil
	}

	_, missingCapabilities, err := resolveMissingRefs(context, claim.MissingRefs)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(candidate.IntentContract.RequiredAnswerAspects))
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		if capabilitiesContainAll(missingCapabilities, aspect.RequiredCapabilities) {
			ids = append(ids, aspect.ID)
		}
	}
	return sortedUnique(ids), nil
}

func capabilitiesContainAll(
	available map[Capability]struct{},
	required []Capability,
) bool {
	if len(required) == 0 {
		return false
	}
	for _, capability := range required {
		if _, exists := available[capability]; !exists {
			return false
		}
	}
	return true
}

func assessAnswerAspects(
	candidate OpportunityCandidate,
	proposal ArtifactProposal,
	claims []ClaimCoverageItem,
) []AnswerAspectCoverage {
	if candidate.IntentContract == nil {
		return nil
	}
	result := make([]AnswerAspectCoverage, 0, len(candidate.IntentContract.RequiredAnswerAspects))
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		coverage := AnswerAspectCoverage{
			AspectID: aspect.ID,
			Key:      aspect.Key,
			Status:   AspectUncovered,
		}
		bestRank := 0
		support := make(map[string]struct{})
		for _, claim := range claims {
			if !containsID(claim.AspectIDs, aspect.ID) {
				continue
			}
			coverage.ClaimIndexes = append(coverage.ClaimIndexes, claim.ClaimIndex)
			addIDs(support, proposal.Claims[claim.ClaimIndex].SupportIDs)
			rank, status := aspectCoverageRank(claim.Basis)
			if rank > bestRank {
				bestRank = rank
				coverage.Status = status
			}
		}
		coverage.SupportIDs = sortedSet(support)
		result = append(result, coverage)
	}
	return result
}

func aspectCoverageRank(basis ClaimBasis) (int, AspectCoverageStatus) {
	switch basis {
	case ClaimDirect:
		return 3, AspectCoveredDirectly
	case ClaimCompositional:
		return 2, AspectCoveredCompositionally
	case ClaimInterpretive:
		return 1, AspectPartiallyCovered
	default:
		return 0, AspectUncovered
	}
}
