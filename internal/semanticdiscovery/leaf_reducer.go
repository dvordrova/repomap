package semanticdiscovery

import (
	"fmt"
	"strings"
)

const reducedLeafConnectionExplanation = "Validated partial evidence requires global combination"

// ReduceLeafArtifact salvages independently valid leaf items without
// rewriting model prose or support. Unknown IDs and unsupported semantics are
// dropped, never repaired. Identity and the final connection are local.
func ReduceLeafArtifact(
	task LeafTask,
	artifact LeafArtifact,
) (LeafArtifact, LeafReductionReport, error) {
	report := LeafReductionReport{}
	if err := task.Validate(); err != nil {
		return LeafArtifact{}, report, err
	}
	if artifact.Version != LeafArtifactVersion ||
		artifact.TaskID != task.ID || artifact.CandidateID != task.Candidate.ID {
		return LeafArtifact{}, report, fmt.Errorf("semantic discovery: leaf reduction envelope does not match task")
	}

	artifact = NormalizeLeafArtifact(artifact)
	known := make(map[string]Fact, len(task.Facts))
	for _, fact := range task.Facts {
		known[fact.ID] = fact
	}
	reduced := LeafArtifact{
		Version:     LeafArtifactVersion,
		TaskID:      task.ID,
		CandidateID: task.Candidate.ID,
	}
	usedSupport := make(map[string]struct{})
	for index, observation := range artifact.Observations {
		if len(reduced.Observations) == maxObservationsPerLeaf {
			report.drop("observations", index, "item_limit")
			continue
		}
		if err := validateLeafObservation(known, index, observation); err != nil {
			report.drop("observations", index, leafReductionCode(err))
			continue
		}
		reduced.Observations = append(reduced.Observations, observation)
		addIDs(usedSupport, observation.SupportIDs)
	}
	for index, contradiction := range artifact.Contradictions {
		if len(reduced.Contradictions) == maxContradictionsPerLeaf {
			report.drop("contradictions", index, "item_limit")
			continue
		}
		if err := validateLeafContradiction(known, index, contradiction); err != nil {
			report.drop("contradictions", index, leafReductionCode(err))
			continue
		}
		reduced.Contradictions = append(reduced.Contradictions, contradiction)
		addIDs(usedSupport, contradiction.SupportIDs)
	}
	for index, missing := range artifact.MissingEvidence {
		if len(reduced.MissingEvidence) == maxMissingEvidencePerLeaf {
			report.drop("missing_evidence", index, "item_limit")
			continue
		}
		if err := validateLeafMissingEvidence(known, index, missing); err != nil {
			report.drop("missing_evidence", index, leafReductionCode(err))
			continue
		}
		reduced.MissingEvidence = append(reduced.MissingEvidence, missing)
		addIDs(usedSupport, missing.SupportIDs)
	}

	report.KeptObservations = len(reduced.Observations)
	report.KeptContradictions = len(reduced.Contradictions)
	report.KeptMissingEvidence = len(reduced.MissingEvidence)
	if len(reduced.Observations) == 0 && len(reduced.MissingEvidence) == 0 {
		report.Issues = append(report.Issues, LeafReductionIssue{
			Section: "content", Index: -1, Code: "no_valid_content",
		})
		return LeafArtifact{}, report, fmt.Errorf("semantic discovery: leaf reduction has no valid observations or missing evidence")
	}
	if len(reduced.Observations) > 0 {
		reduced.Status = LeafStatusUsable
	} else {
		reduced.Status = LeafStatusInsufficientEvidence
	}
	reduced.CandidateConnection = LeafCandidateConnection{
		CandidateID: task.Candidate.ID,
		Relation:    connectionNeedsCombination,
		Explanation: reducedLeafConnectionExplanation,
		SupportIDs:  sortedSet(usedSupport),
	}
	if err := ValidateLeafArtifact(task, reduced); err != nil {
		return LeafArtifact{}, report, fmt.Errorf("semantic discovery: reduced leaf failed final validation: %w", err)
	}
	return reduced, report, nil
}

func (report *LeafReductionReport) drop(section string, index int, code string) {
	report.Issues = append(report.Issues, LeafReductionIssue{
		Section: section,
		Index:   index,
		Code:    code,
	})
	switch section {
	case "observations":
		report.DroppedObservations++
	case "contradictions":
		report.DroppedContradictions++
	case "missing_evidence":
		report.DroppedMissingEvidence++
	}
}

func leafReductionCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unknown support id"):
		return "unknown_support_id"
	case strings.Contains(message, "repository-bearing reference"):
		return "repository_reference"
	case strings.Contains(message, "duplicate"), strings.Contains(message, "repeats"):
		return "duplicate_value"
	case strings.Contains(message, "lexically unrelated"),
		strings.Contains(message, "is unrelated"),
		strings.Contains(message, "-capable support"),
		strings.Contains(message, "two source groups"),
		strings.Contains(message, "already present"):
		return "unsupported_semantics"
	default:
		return "invalid_item"
	}
}
