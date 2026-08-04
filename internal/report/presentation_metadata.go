package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/tasklens"
)

// HydrateRunPresentationMetadata restores render-only typed metadata that is
// deliberately omitted from canonical report.json. The canonical report
// remains authoritative: metadata is copied only after a fresh artifact replay
// reproduces the complete warning vector and the persisted Task Lens
// projection byte-for-byte.
func HydrateRunPresentationMetadata(runDir string, data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report presentation metadata: data is required")
	}
	if data.TaskInvestigation == nil &&
		studyPublicationWarningMessageID(data.StudyPublication) == "" &&
		!orientationWarningSidecarExists(runDir) {
		data.PresentationWarningKinds = nil
		data.PresentationWarnings = nil
		data.PresentationWarningMessages = nil
		data.runWarningDiagnostics = nil
		data.presentationMetadataErr = nil
		return nil
	}
	replayed, err := ReadRunDir(runDir)
	if err != nil {
		return fmt.Errorf("report presentation metadata: replay run: %w", err)
	}
	return hydratePresentationMetadataFromReplay(data, replayed)
}

func orientationWarningSidecarExists(runDir string) bool {
	_, err := os.Lstat(filepath.Join(
		runDir,
		orient.ConfidenceWarningDiagnosticsFile,
	))
	return !os.IsNotExist(err)
}

func hydratePresentationMetadataFromReplay(
	target,
	replayed *ReportData,
) error {
	if target == nil || replayed == nil {
		return fmt.Errorf("report presentation metadata: report is required")
	}
	if replayed.presentationMetadataErr != nil {
		// Carry the failure on the canonical render value as transient state.
		// The server deliberately continues loading EN after this method
		// returns; LoadPresentationLocalization must still be able to degrade
		// an otherwise hash-compatible RU projection.
		target.presentationMetadataErr = replayed.presentationMetadataErr
		return fmt.Errorf(
			"report presentation metadata: orientation warning sidecar: %w",
			replayed.presentationMetadataErr,
		)
	}
	if !slices.Equal(target.Warnings, replayed.Warnings) {
		return fmt.Errorf("report presentation metadata: warning vector does not replay")
	}
	if !sameStudyPublicationStatus(
		target.StudyPublication,
		replayed.StudyPublication,
	) {
		return fmt.Errorf("report presentation metadata: Study publication status does not replay")
	}

	studyWarningKinds := studyPresentationWarningKinds(replayed)
	var taskWarningDiagnostics []tasklens.WarningDiagnostic

	switch {
	case target.TaskInvestigation == nil && replayed.TaskInvestigation == nil:
	case target.TaskInvestigation == nil || replayed.TaskInvestigation == nil:
		return fmt.Errorf("report presentation metadata: Task Lens projection does not replay")
	default:
		if !samePersistedTaskInvestigation(
			target.TaskInvestigation,
			replayed.TaskInvestigation,
		) {
			return fmt.Errorf("report presentation metadata: Task Lens projection does not replay")
		}
		taskWarningDiagnostics = append(
			[]tasklens.WarningDiagnostic(nil),
			replayed.TaskInvestigation.warningDiagnostics...,
		)
	}
	target.PresentationWarningKinds = studyWarningKinds
	target.PresentationWarnings = nil
	target.PresentationWarningMessages = nil
	target.runWarningDiagnostics = append(
		[]runWarningDiagnostic(nil),
		replayed.runWarningDiagnostics...,
	)
	target.presentationMetadataErr = nil
	if target.TaskInvestigation != nil {
		target.TaskInvestigation.warningDiagnostics = taskWarningDiagnostics
		target.TaskInvestigation.PresentationWarnings = nil
	}
	return nil
}

// prepareReplayedPresentationMetadata enables locally reconstructed Study and
// Task Lens catalog metadata on an ordinary artifact replay. An optional
// orientation sidecar error remains quarantined in presentationMetadataErr:
// canonical English replay and persistence proceed, while shared hydration
// will surface the error before an RU projection can be claimed complete.
func prepareReplayedPresentationMetadata(data *ReportData) {
	if data == nil {
		return
	}
	data.PresentationWarningKinds = studyPresentationWarningKinds(data)
	data.PresentationWarnings = nil
	data.PresentationWarningMessages = nil
	if data.TaskInvestigation != nil {
		data.TaskInvestigation.PresentationWarnings = nil
	}
}

func sameStudyPublicationStatus(left, right *StudyPublicationStatus) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func studyPresentationWarningKinds(data *ReportData) []string {
	if data == nil {
		return nil
	}
	warning := studyPublicationUserWarning(data.StudyPublication)
	messageID := studyPublicationWarningMessageID(data.StudyPublication)
	if warning == "" || messageID == "" {
		return nil
	}
	for index := len(data.Warnings) - 1; index >= 0; index-- {
		if data.Warnings[index] != warning {
			continue
		}
		kinds := make([]string, len(data.Warnings))
		kinds[index] = messageID
		return kinds
	}
	return nil
}

func samePersistedTaskInvestigation(
	left,
	right *TaskInvestigationWorkspace,
) bool {
	leftJSON, leftErr := json.Marshal(taskInvestigationForPersistence(left))
	rightJSON, rightErr := json.Marshal(taskInvestigationForPersistence(right))
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func taskInvestigationForPersistence(
	workspace *TaskInvestigationWorkspace,
) *TaskInvestigationWorkspace {
	if workspace == nil {
		return nil
	}
	cloned := *workspace
	cloned.PresentationWarnings = nil
	return &cloned
}
