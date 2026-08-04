package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/orient"
)

const orientationCandidateCapWarning = "local confidence gate capped candidate_flows[0] from 0.90 to 0.30"

func TestOrientationWarningSidecarHydratesCanonicalReportCopy(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	orientationJSON := warningSidecarOrientationJSON(t, 0.3)
	writeWarningSidecarOrientation(t, runDir, orientationJSON)
	writeOrientationWarningSidecar(t, runDir, orientationJSON, []orient.ConfidenceWarningDiagnostic{{
		WarningIndex:   1,
		Code:           orient.ConfidenceWarningCandidateCapped,
		CandidateIndex: 0,
		Proposed:       0.9,
		Capped:         0.3,
	}})

	replayed, canonical := warningSidecarCanonicalClone(t, runDir)
	if replayed.presentationMetadataErr != nil ||
		len(replayed.runWarningDiagnostics) != 1 {
		t.Fatalf(
			"valid warning sidecar replay = diagnostics %#v, error %v",
			replayed.runWarningDiagnostics,
			replayed.presentationMetadataErr,
		)
	}
	if err := HydrateRunPresentationMetadata(runDir, canonical); err != nil {
		t.Fatal(err)
	}
	rendered := reportDataForRendering(canonical)
	if len(rendered.PresentationWarningMessages) != 1 ||
		rendered.PresentationWarningMessages[0].WarningIndex !=
			replayed.runWarningDiagnostics[0].WarningIndex ||
		rendered.PresentationWarningMessages[0].MessageID !=
			runWarningMessageCandidateConfidenceCapped {
		t.Fatalf(
			"hydrated warning presentation = %#v",
			rendered.PresentationWarningMessages,
		)
	}
}

func TestMissingOrientationWarningSidecarMeansNoTypedDiagnostics(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeWarningSidecarOrientation(
		t,
		runDir,
		warningSidecarOrientationJSON(t, 0.3),
	)
	replayed, canonical := warningSidecarCanonicalClone(t, runDir)
	if replayed.presentationMetadataErr != nil ||
		len(replayed.runWarningDiagnostics) != 0 {
		t.Fatalf(
			"missing sidecar replay = diagnostics %#v, error %v",
			replayed.runWarningDiagnostics,
			replayed.presentationMetadataErr,
		)
	}
	if err := HydrateRunPresentationMetadata(runDir, canonical); err != nil {
		t.Fatal(err)
	}
	if len(canonical.runWarningDiagnostics) != 0 {
		t.Fatalf("missing sidecar hydrated diagnostics %#v", canonical.runWarningDiagnostics)
	}
}

func TestInvalidOrientationWarningSidecarsDoNotChangeCanonicalEnglish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		confidence float64
		sidecar    func(t *testing.T, orientationJSON []byte) []byte
	}{
		{
			name:       "corrupt",
			confidence: 0.3,
			sidecar: func(_ *testing.T, _ []byte) []byte {
				return []byte("{not-json")
			},
		},
		{
			name:       "stale hash",
			confidence: 0.3,
			sidecar: func(t *testing.T, _ []byte) []byte {
				return warningSidecarBytes(t, []byte("different orientation bytes"), warningSidecarDiagnostic())
			},
		},
		{
			name:       "warning index",
			confidence: 0.3,
			sidecar: func(t *testing.T, orientationJSON []byte) []byte {
				diagnostic := warningSidecarDiagnostic()
				diagnostic.WarningIndex = 99
				return warningSidecarBytes(t, orientationJSON, diagnostic)
			},
		},
		{
			name:       "raw warning round trip",
			confidence: 0.3,
			sidecar: func(t *testing.T, orientationJSON []byte) []byte {
				diagnostic := warningSidecarDiagnostic()
				diagnostic.Proposed = 0.8
				return warningSidecarBytes(t, orientationJSON, diagnostic)
			},
		},
		{
			name:       "local capped state",
			confidence: 0.2,
			sidecar: func(t *testing.T, orientationJSON []byte) []byte {
				return warningSidecarBytes(t, orientationJSON, warningSidecarDiagnostic())
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := t.TempDir()
			orientationJSON := warningSidecarOrientationJSON(t, test.confidence)
			writeWarningSidecarOrientation(t, runDir, orientationJSON)
			if err := os.WriteFile(
				filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
				test.sidecar(t, orientationJSON),
				0o600,
			); err != nil {
				t.Fatal(err)
			}

			replayed, canonical := warningSidecarCanonicalClone(t, runDir)
			if replayed.presentationMetadataErr == nil ||
				len(replayed.runWarningDiagnostics) != 0 {
				t.Fatalf(
					"invalid sidecar replay = diagnostics %#v, error %v",
					replayed.runWarningDiagnostics,
					replayed.presentationMetadataErr,
				)
			}
			if err := HydrateRunPresentationMetadata(runDir, canonical); err == nil {
				t.Fatal("shared presentation hydration accepted invalid sidecar")
			}
			if canonical.presentationMetadataErr == nil {
				t.Fatal("hydration did not carry the transient sidecar failure")
			}

			withInvalid := warningSidecarCanonicalJSON(t, replayed)
			if err := os.Remove(filepath.Join(
				runDir,
				orient.ConfidenceWarningDiagnosticsFile,
			)); err != nil {
				t.Fatal(err)
			}
			withoutSidecar, err := ReadRunDir(runDir)
			if err != nil {
				t.Fatal(err)
			}
			withoutInvalid := warningSidecarCanonicalJSON(t, withoutSidecar)
			if !bytes.Equal(withInvalid, withoutInvalid) {
				t.Fatalf(
					"optional sidecar changed canonical EN report\nwith: %s\nwithout: %s",
					withInvalid,
					withoutInvalid,
				)
			}
		})
	}
}

func TestSymlinkedOrientationWarningSidecarFailsSafely(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	orientationJSON := warningSidecarOrientationJSON(t, 0.3)
	writeWarningSidecarOrientation(t, runDir, orientationJSON)
	victim := filepath.Join(t.TempDir(), "victim.json")
	victimBytes := warningSidecarBytes(t, orientationJSON, warningSidecarDiagnostic())
	if err := os.WriteFile(victim, victimBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		victim,
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
	); err != nil {
		t.Fatal(err)
	}

	replayed, canonical := warningSidecarCanonicalClone(t, runDir)
	if replayed.presentationMetadataErr == nil ||
		!strings.Contains(replayed.presentationMetadataErr.Error(), "regular file") {
		t.Fatalf("symlinked sidecar replay error = %v", replayed.presentationMetadataErr)
	}
	if err := HydrateRunPresentationMetadata(runDir, canonical); err == nil {
		t.Fatal("shared hydration accepted a symlinked sidecar")
	}
	after, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, victimBytes) {
		t.Fatal("sidecar reader changed the symlink target")
	}
}

func TestHydrationFailureForcesRussianProjectionDegradation(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	orientationJSON := warningSidecarOrientationJSON(t, 0.3)
	writeWarningSidecarOrientation(t, runDir, orientationJSON)
	// This corrupt sidecar declares no usable diagnostics. Without the
	// transient hydration error, the complete localization inventory would be
	// byte-identical to the missing-sidecar case and an old RU success could be
	// accepted silently.
	if err := os.WriteFile(
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
		[]byte(`{"version":99,"orientation_report_sha256":"","diagnostics":[]}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	_, canonical := warningSidecarCanonicalClone(t, runDir)
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatalf("%v; warnings=%#v", err, canonical.Warnings)
	}
	translations := make(map[string]string, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		parts := []string{"Русский текст"}
		for _, placeholder := range field.Placeholders {
			for count := 0; count < placeholder.Count; count++ {
				parts = append(parts, placeholder.Token)
			}
		}
		translations[field.ID] = strings.Join(parts, " ")
	}
	projection := localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		projection,
		true,
		"request-sha",
		"cache-key",
	); err != nil {
		t.Fatal(err)
	}
	if err := HydrateRunPresentationMetadata(runDir, canonical); err == nil {
		t.Fatal("shared hydration accepted corrupt sidecar")
	}
	projected, status := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if status.State != PresentationLocalizationFailed ||
		status.ReasonCode != LocalizationFailureSavedProjection ||
		projected.ReportLanguage != localization.LocaleRussian ||
		projected.presentationLocalizationState != PresentationLocalizationFailed {
		t.Fatalf("corrupt-sidecar RU projection/status = %#v / %#v", projected, status)
	}
	english, englishStatus := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleEnglish,
	)
	if englishStatus.State != "" || english.ReportLanguage != "" ||
		!bytes.Equal(
			warningSidecarCanonicalJSON(t, english),
			warningSidecarCanonicalJSON(t, canonical),
		) {
		t.Fatalf("corrupt optional sidecar changed canonical EN: %#v / %#v", english, englishStatus)
	}
}

func warningSidecarOrientationJSON(t *testing.T, candidateConfidence float64) []byte {
	t.Helper()
	orientation := map[string]any{
		"project_guess":       "Fixture repository",
		"confidence":          candidateConfidence,
		"high_level_map":      []any{},
		"first_files_to_open": []any{},
		"candidate_flows": []any{map[string]any{
			"name":       "Fixture direction",
			"confidence": candidateConfidence,
			"local_verification": map[string]any{
				"status":         "partial",
				"confidence_cap": 0.3,
			},
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []any{},
		"unverified_paths":       []any{},
		"warnings": []string{
			orientationCandidateCapWarning,
			orientationCandidateCapWarning,
		},
	}
	encoded, err := json.Marshal(orientation)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func warningSidecarDiagnostic() orient.ConfidenceWarningDiagnostic {
	return orient.ConfidenceWarningDiagnostic{
		WarningIndex:   1,
		Code:           orient.ConfidenceWarningCandidateCapped,
		CandidateIndex: 0,
		Proposed:       0.9,
		Capped:         0.3,
	}
}

func warningSidecarBytes(
	t *testing.T,
	orientationJSON []byte,
	diagnostic orient.ConfidenceWarningDiagnostic,
) []byte {
	t.Helper()
	encoded, err := orient.EncodeConfidenceWarningDiagnostics(
		orientationJSON,
		[]orient.ConfidenceWarningDiagnostic{diagnostic},
	)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func writeWarningSidecarOrientation(
	t *testing.T,
	runDir string,
	orientationJSON []byte,
) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(runDir, "snapshot.json"),
		[]byte("{}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "orientation_report.json"),
		orientationJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func warningSidecarCanonicalClone(
	t *testing.T,
	runDir string,
) (*ReportData, *ReportData) {
	t.Helper()
	replayed, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	raw := warningSidecarCanonicalJSON(t, replayed)
	var canonical ReportData
	if err := json.Unmarshal(raw, &canonical); err != nil {
		t.Fatal(err)
	}
	return replayed, &canonical
}

func warningSidecarCanonicalJSON(t *testing.T, data *ReportData) []byte {
	t.Helper()
	raw, err := json.Marshal(reportDataForPersistence(data))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
