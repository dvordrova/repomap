package report

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/orient"
)

func readOrientationWarningDiagnostics(
	runDir string,
	orientationReportJSON []byte,
	warningOffset int,
	orientation orientationReportJSON,
) ([]runWarningDiagnostic, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, fmt.Errorf("orientation warning diagnostics: resolve run directory: %w", err)
	}
	root, err := os.OpenRoot(absDir)
	if err != nil {
		return nil, fmt.Errorf("orientation warning diagnostics: open run directory: %w", err)
	}
	defer root.Close()

	info, err := root.Lstat(orient.ConfidenceWarningDiagnosticsFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("orientation warning diagnostics: inspect sidecar: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > orient.MaxConfidenceWarningDiagnosticsBytes {
		return nil, fmt.Errorf("orientation warning diagnostics: sidecar is not a bounded regular file")
	}
	file, err := root.Open(orient.ConfidenceWarningDiagnosticsFile)
	if err != nil {
		return nil, fmt.Errorf("orientation warning diagnostics: open sidecar: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("orientation warning diagnostics: sidecar changed before open")
	}
	raw, err := io.ReadAll(io.LimitReader(
		file,
		int64(orient.MaxConfidenceWarningDiagnosticsBytes)+1,
	))
	if err != nil {
		return nil, fmt.Errorf("orientation warning diagnostics: read sidecar: %w", err)
	}
	if len(raw) == 0 || len(raw) > orient.MaxConfidenceWarningDiagnosticsBytes {
		return nil, fmt.Errorf("orientation warning diagnostics: sidecar exceeds its byte limit")
	}
	record, err := orient.DecodeConfidenceWarningDiagnostics(raw)
	if err != nil {
		return nil, err
	}
	if !record.MatchesOrientationReport(orientationReportJSON) {
		return nil, fmt.Errorf("orientation warning diagnostics: stale orientation report hash")
	}

	result := make([]runWarningDiagnostic, 0, len(record.Diagnostics))
	for _, diagnostic := range record.Diagnostics {
		if diagnostic.WarningIndex < 0 ||
			diagnostic.WarningIndex >= len(orientation.Warnings) {
			return nil, fmt.Errorf("orientation warning diagnostics: warning index is out of range")
		}
		rawWarning, ok := diagnostic.RawWarning()
		if !ok || rawWarning != orientation.Warnings[diagnostic.WarningIndex] {
			return nil, fmt.Errorf("orientation warning diagnostics: raw warning does not round trip")
		}
		switch diagnostic.Code {
		case orient.ConfidenceWarningCandidateCapped:
			if !validCandidateConfidenceWarning(orientation, diagnostic) {
				return nil, fmt.Errorf("orientation warning diagnostics: candidate capped state does not replay")
			}
		case orient.ConfidenceWarningOrientationCapped:
			if !validOrientationConfidenceWarning(orientation, diagnostic) {
				return nil, fmt.Errorf("orientation warning diagnostics: orientation capped state does not replay")
			}
		default:
			return nil, fmt.Errorf("orientation warning diagnostics: unsupported code")
		}
		result = append(result, runWarningDiagnostic{
			WarningIndex:   warningOffset + diagnostic.WarningIndex,
			Code:           diagnostic.Code,
			CandidateIndex: diagnostic.CandidateIndex,
			Proposed:       diagnostic.Proposed,
			Capped:         diagnostic.Capped,
		})
	}
	return result, nil
}

func validCandidateConfidenceWarning(
	orientation orientationReportJSON,
	diagnostic orient.ConfidenceWarningDiagnostic,
) bool {
	if diagnostic.CandidateIndex < 0 ||
		diagnostic.CandidateIndex >= len(orientation.CandidateFlows) {
		return false
	}
	candidate := orientation.CandidateFlows[diagnostic.CandidateIndex]
	return candidate.LocalVerification != nil &&
		candidate.Confidence == diagnostic.Capped &&
		candidate.LocalVerification.ConfidenceCap == diagnostic.Capped
}

func validOrientationConfidenceWarning(
	orientation orientationReportJSON,
	diagnostic orient.ConfidenceWarningDiagnostic,
) bool {
	const incompleteContextCap = 0.6
	if diagnostic.Capped != incompleteContextCap ||
		orientation.Confidence > diagnostic.Capped {
		return false
	}
	if orientation.Confidence == diagnostic.Capped {
		return true
	}
	maxCandidateConfidence := 0.0
	for _, candidate := range orientation.CandidateFlows {
		if candidate.Confidence > maxCandidateConfidence {
			maxCandidateConfidence = candidate.Confidence
		}
	}
	return maxCandidateConfidence > 0 &&
		orientation.Confidence == maxCandidateConfidence
}
