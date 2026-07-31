package orient

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
)

func TestConfidenceWarningDiagnosticsCodecIsStrictAndReportBound(t *testing.T) {
	t.Parallel()

	reportJSON := []byte(`{"warnings":["local confidence gate capped candidate_flows[0] from 0.90 to 0.30"]}`)
	diagnostic := ConfidenceWarningDiagnostic{
		WarningIndex:   0,
		Code:           ConfidenceWarningCandidateCapped,
		CandidateIndex: 0,
		Proposed:       0.9,
		Capped:         0.3,
	}
	encoded, err := EncodeConfidenceWarningDiagnostics(
		reportJSON,
		[]ConfidenceWarningDiagnostic{diagnostic},
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := DecodeConfidenceWarningDiagnostics(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !record.MatchesOrientationReport(reportJSON) ||
		record.MatchesOrientationReport(append(append([]byte(nil), reportJSON...), '\n')) ||
		len(record.Diagnostics) != 1 || record.Diagnostics[0] != diagnostic {
		t.Fatalf("decoded sidecar = %#v", record)
	}

	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "version",
			mutate: func(value map[string]any) {
				value["version"] = float64(ConfidenceWarningDiagnosticsVersion + 1)
			},
		},
		{
			name: "unknown field",
			mutate: func(value map[string]any) {
				value["unexpected"] = true
			},
		},
		{
			name: "code",
			mutate: func(value map[string]any) {
				diagnostics := value["diagnostics"].([]any)
				diagnostics[0].(map[string]any)["code"] = "provider_warning"
			},
		},
		{
			name: "orientation candidate index",
			mutate: func(value map[string]any) {
				diagnostics := value["diagnostics"].([]any)
				entry := diagnostics[0].(map[string]any)
				entry["code"] = string(ConfidenceWarningOrientationCapped)
				entry["candidate_index"] = float64(0)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clonedJSON, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var cloned map[string]any
			if err := json.Unmarshal(clonedJSON, &cloned); err != nil {
				t.Fatal(err)
			}
			test.mutate(cloned)
			mutated, err := json.Marshal(cloned)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeConfidenceWarningDiagnostics(mutated); err == nil {
				t.Fatal("invalid sidecar decoded successfully")
			}
		})
	}
}

func TestDebugdumpBindsSidecarToExactSavedOrientationBytes(t *testing.T) {
	t.Parallel()

	writer, err := debugdump.NewWriter(t.TempDir(), "warning-sidecar", true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	reportJSON := []byte(`{"token":"must-redact","warnings":[]}`)
	if err := writer.WriteOrientationReportWithSidecar(
		reportJSON,
		ConfidenceWarningDiagnosticsFile,
		func(saved []byte) ([]byte, error) {
			return EncodeConfidenceWarningDiagnostics(saved, nil)
		},
	); err != nil {
		t.Fatal(err)
	}
	savedReport, err := os.ReadFile(filepath.Join(writer.RunDir(), "orientation_report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(savedReport, reportJSON) ||
		strings.Contains(string(savedReport), "must-redact") {
		t.Fatalf("orientation report was not redacted: %s", savedReport)
	}
	sidecarJSON, err := os.ReadFile(filepath.Join(
		writer.RunDir(),
		ConfidenceWarningDiagnosticsFile,
	))
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := DecodeConfidenceWarningDiagnostics(sidecarJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !sidecar.MatchesOrientationReport(savedReport) {
		t.Fatal("sidecar does not bind the exact saved orientation bytes")
	}
}
