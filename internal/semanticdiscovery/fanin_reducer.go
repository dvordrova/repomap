package semanticdiscovery

import "fmt"

const MaxFanInReductionIssues = 16

// ReduceFanInArtifact preserves only whole proposals whose semantic content
// passes the existing validators. It never rewrites claims, support, or prose;
// the model-proposed verdict is replaced by the locally derived authority.
func ReduceFanInArtifact(
	bundle Bundle,
	results []LeafResult,
	artifact FanInArtifact,
) (FanInArtifact, FanInReductionReport, error) {
	report := FanInReductionReport{}
	_, context, err := validateLeafResults(bundle, results)
	if err != nil {
		return FanInArtifact{}, report, err
	}
	if artifact.Version != FanInArtifactVersion {
		return FanInArtifact{}, report, fmt.Errorf(
			"semantic discovery: unsupported fan-in artifact version %d",
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
		input, err := validateFanInProposalContent(context, proposal)
		if err != nil {
			report.dropWithReasons(
				index,
				"invalid_proposal",
				diagnoseFanInProposal(context, proposal, err),
			)
			continue
		}
		if !validModelVerdict(proposal.Verdict) {
			validationErr := fmt.Errorf("unsupported model-proposed verdict %q", proposal.Verdict)
			report.dropWithReasons(
				index,
				"invalid_proposal",
				diagnoseFanInProposal(context, proposal, validationErr),
			)
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
	reduced = NormalizeFanInArtifact(reduced)
	report.KeptArtifacts = len(reduced.Artifacts)
	if len(reduced.Artifacts) == 0 {
		report.addIssue(-1, "no_valid_artifacts")
		return FanInArtifact{}, report, fmt.Errorf(
			"semantic discovery: fan-in reduction has no valid artifacts",
		)
	}
	if err := validateFanInArtifact(context, reduced, false); err != nil {
		return FanInArtifact{}, report, fmt.Errorf(
			"semantic discovery: reduced fan-in failed final validation: %w",
			err,
		)
	}
	return reduced, report, nil
}

func (report *FanInReductionReport) drop(index int, code string) {
	report.dropWithReasons(index, code, nil)
}

func (report *FanInReductionReport) dropWithReasons(
	index int,
	code string,
	reasons []FanInReductionReason,
) {
	report.DroppedArtifacts++
	report.addIssueWithReasons(index, code, reasons)
}

func (report *FanInReductionReport) addIssue(index int, code string) {
	report.addIssueWithReasons(index, code, nil)
}

func (report *FanInReductionReport) addIssueWithReasons(
	index int,
	code string,
	reasons []FanInReductionReason,
) {
	if len(report.Issues) == MaxFanInReductionIssues {
		return
	}
	report.Issues = append(report.Issues, FanInReductionIssue{
		ArtifactIndex: index,
		Code:          code,
		Reasons:       append([]FanInReductionReason(nil), reasons...),
	})
}
