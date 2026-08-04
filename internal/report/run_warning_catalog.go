package report

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/orient"
)

const (
	runWarningMessageCandidateConfidenceCapped   = "main.warning.confidence_candidate_capped"
	runWarningMessageOrientationConfidenceCapped = "main.warning.confidence_orientation_capped_incomplete"
)

func runPresentationWarnings(data *ReportData) []RunPresentationWarning {
	if data == nil || len(data.runWarningDiagnostics) == 0 {
		return nil
	}
	result := make([]RunPresentationWarning, 0, len(data.runWarningDiagnostics))
	for _, diagnostic := range data.runWarningDiagnostics {
		presentation, ok := runPresentationWarning(diagnostic)
		if !ok {
			continue
		}
		result = append(result, presentation)
	}
	return result
}

func runPresentationWarningForIndex(
	data *ReportData,
	warningIndex int,
) (RunPresentationWarning, bool) {
	if data == nil || warningIndex < 0 {
		return RunPresentationWarning{}, false
	}
	for _, diagnostic := range data.runWarningDiagnostics {
		if diagnostic.WarningIndex != warningIndex {
			continue
		}
		return runPresentationWarning(diagnostic)
	}
	return RunPresentationWarning{}, false
}

func runPresentationWarning(
	diagnostic runWarningDiagnostic,
) (RunPresentationWarning, bool) {
	messageID := ""
	switch diagnostic.Code {
	case orient.ConfidenceWarningCandidateCapped:
		messageID = runWarningMessageCandidateConfidenceCapped
	case orient.ConfidenceWarningOrientationCapped:
		messageID = runWarningMessageOrientationConfidenceCapped
	default:
		return RunPresentationWarning{}, false
	}
	return RunPresentationWarning{
		WarningIndex:   diagnostic.WarningIndex,
		MessageID:      messageID,
		CandidateIndex: diagnostic.CandidateIndex,
		Proposed:       fmt.Sprintf("%.2f", diagnostic.Proposed),
		Capped:         fmt.Sprintf("%.2f", diagnostic.Capped),
	}, true
}
