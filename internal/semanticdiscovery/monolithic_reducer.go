package semanticdiscovery

import "fmt"

// ReduceMonolithicArtifact preserves only whole proposals whose semantic
// content passes the existing validators. It never rewrites claims, support,
// or prose; the model-proposed verdict is replaced by local authority.
func ReduceMonolithicArtifact(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
) (FanInArtifact, FanInReductionReport, error) {
	report := FanInReductionReport{}
	context, err := validateMonolithicCandidates(bundle, candidates)
	if err != nil {
		return FanInArtifact{}, report, err
	}
	if artifact.Version != FanInArtifactVersion {
		return FanInArtifact{}, report, fmt.Errorf(
			"semantic discovery: unsupported monolithic artifact version %d",
			artifact.Version,
		)
	}

	reduced := FanInArtifact{
		Version:   FanInArtifactVersion,
		Artifacts: make([]ArtifactProposal, 0, len(context.candidates)),
	}
	keptCandidates := make(map[string]struct{}, len(context.candidates))
	for index, original := range artifact.Artifacts {
		proposal := original
		normalizeFanInProposal(&proposal)
		if _, known := context.candidates[proposal.CandidateID]; !known {
			report.drop(index, "unknown_candidate")
			continue
		}
		if _, duplicate := keptCandidates[proposal.CandidateID]; duplicate {
			report.drop(index, "duplicate_candidate")
			continue
		}
		input, err := validateMonolithicProposalContent(context, proposal)
		if err != nil {
			report.drop(index, "invalid_proposal")
			continue
		}
		if !validModelVerdict(proposal.Verdict) {
			report.drop(index, "invalid_proposal")
			continue
		}
		modelVerdict := proposal.Verdict
		derivedVerdict, reasons := deriveVerdict(input)
		proposal.Verdict = derivedVerdict
		if modelVerdict != derivedVerdict {
			report.VerdictDiagnostics = append(
				report.VerdictDiagnostics,
				VerdictDiagnostic{
					Code:           "model_verdict_mismatch",
					ArtifactIndex:  index,
					CandidateID:    proposal.CandidateID,
					ModelVerdict:   modelVerdict,
					DerivedVerdict: derivedVerdict,
					Reasons:        append([]VerdictReason(nil), reasons...),
				},
			)
		}
		keptCandidates[proposal.CandidateID] = struct{}{}
		reduced.Artifacts = append(reduced.Artifacts, proposal)
	}
	reduced = NormalizeMonolithicArtifact(reduced)
	report.KeptArtifacts = len(reduced.Artifacts)
	if len(reduced.Artifacts) == 0 {
		report.addIssue(-1, "no_valid_artifacts")
		return FanInArtifact{}, report, fmt.Errorf(
			"semantic discovery: monolithic reduction has no valid artifacts",
		)
	}
	if err := validateMonolithicArtifact(context, reduced, false); err != nil {
		return FanInArtifact{}, report, fmt.Errorf(
			"semantic discovery: reduced monolithic artifact failed final validation: %w",
			err,
		)
	}
	return reduced, report, nil
}
