package orient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// ConfidenceWarningDiagnosticsFile is a presentation-only debug artifact.
	// It is intentionally outside report.json and the run manifest.
	ConfidenceWarningDiagnosticsFile     = "orientation_warning_diagnostics.v1.json"
	ConfidenceWarningDiagnosticsVersion  = 1
	MaxConfidenceWarningDiagnosticsBytes = 64 << 10
	maxConfidenceWarningDiagnostics      = 256
)

// ConfidenceWarningDiagnostics binds the producer's typed warning addresses
// to the exact canonical orientation_report.json bytes.
type ConfidenceWarningDiagnostics struct {
	Version                 int                           `json:"version"`
	OrientationReportSHA256 string                        `json:"orientation_report_sha256"`
	Diagnostics             []ConfidenceWarningDiagnostic `json:"diagnostics"`
}

// EncodeConfidenceWarningDiagnostics creates the bounded sidecar written next
// to orientation_report.json. The report hash covers the exact bytes passed to
// debugdump, including their whitespace and final-newline choice.
func EncodeConfidenceWarningDiagnostics(
	orientationReportJSON []byte,
	diagnostics []ConfidenceWarningDiagnostic,
) ([]byte, error) {
	copiedDiagnostics := make(
		[]ConfidenceWarningDiagnostic,
		len(diagnostics),
	)
	copy(copiedDiagnostics, diagnostics)
	record := ConfidenceWarningDiagnostics{
		Version:                 ConfidenceWarningDiagnosticsVersion,
		OrientationReportSHA256: ConfidenceWarningReportSHA256(orientationReportJSON),
		Diagnostics:             copiedDiagnostics,
	}
	if err := record.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("orientation warning diagnostics: encode: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > MaxConfidenceWarningDiagnosticsBytes {
		return nil, fmt.Errorf(
			"orientation warning diagnostics: exceeds %d bytes",
			MaxConfidenceWarningDiagnosticsBytes,
		)
	}
	return encoded, nil
}

// DecodeConfidenceWarningDiagnostics strictly decodes one bounded sidecar.
func DecodeConfidenceWarningDiagnostics(
	data []byte,
) (ConfidenceWarningDiagnostics, error) {
	if len(data) == 0 || len(data) > MaxConfidenceWarningDiagnosticsBytes {
		return ConfidenceWarningDiagnostics{}, fmt.Errorf(
			"orientation warning diagnostics: size must be between 1 and %d bytes",
			MaxConfidenceWarningDiagnosticsBytes,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record ConfidenceWarningDiagnostics
	if err := decoder.Decode(&record); err != nil {
		return ConfidenceWarningDiagnostics{}, fmt.Errorf(
			"orientation warning diagnostics: decode: %w",
			err,
		)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ConfidenceWarningDiagnostics{}, fmt.Errorf(
				"orientation warning diagnostics: multiple json values",
			)
		}
		return ConfidenceWarningDiagnostics{}, fmt.Errorf(
			"orientation warning diagnostics: trailing data: %w",
			err,
		)
	}
	if err := record.Validate(); err != nil {
		return ConfidenceWarningDiagnostics{}, err
	}
	return record, nil
}

func (record ConfidenceWarningDiagnostics) Validate() error {
	if record.Version != ConfidenceWarningDiagnosticsVersion {
		return fmt.Errorf("orientation warning diagnostics: unsupported version")
	}
	if !validConfidenceWarningSHA256(record.OrientationReportSHA256) {
		return fmt.Errorf("orientation warning diagnostics: invalid orientation report hash")
	}
	if record.Diagnostics == nil {
		return fmt.Errorf("orientation warning diagnostics: diagnostics array is required")
	}
	if len(record.Diagnostics) > maxConfidenceWarningDiagnostics {
		return fmt.Errorf("orientation warning diagnostics: too many diagnostics")
	}
	lastWarningIndex := -1
	for _, diagnostic := range record.Diagnostics {
		if diagnostic.WarningIndex <= lastWarningIndex {
			return fmt.Errorf("orientation warning diagnostics: warning indices are not strictly increasing")
		}
		if _, ok := diagnostic.RawWarning(); !ok {
			return fmt.Errorf("orientation warning diagnostics: invalid diagnostic")
		}
		switch diagnostic.Code {
		case ConfidenceWarningCandidateCapped:
			if diagnostic.CandidateIndex < 0 {
				return fmt.Errorf("orientation warning diagnostics: invalid candidate index")
			}
		case ConfidenceWarningOrientationCapped:
			if diagnostic.CandidateIndex != -1 {
				return fmt.Errorf("orientation warning diagnostics: orientation diagnostic has a candidate index")
			}
		default:
			return fmt.Errorf("orientation warning diagnostics: unknown code")
		}
		lastWarningIndex = diagnostic.WarningIndex
	}
	return nil
}

func (record ConfidenceWarningDiagnostics) MatchesOrientationReport(
	orientationReportJSON []byte,
) bool {
	return record.OrientationReportSHA256 ==
		ConfidenceWarningReportSHA256(orientationReportJSON)
}

func ConfidenceWarningReportSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validConfidenceWarningSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
