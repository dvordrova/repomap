package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/orient"
)

func TestConfidenceGateWarningsUseTypedCatalogOutsideTranslationInventory(t *testing.T) {
	t.Parallel()

	const (
		productWarning = "local confidence gate capped candidate_flows[0] from 0.90 to 0.30"
		modelWarning   = "The provider could not confirm the remote acknowledgement."
	)
	runDir := t.TempDir()
	orientation := map[string]any{
		"project_guess":       "fixture",
		"confidence":          0.3,
		"high_level_map":      []any{},
		"first_files_to_open": []any{},
		"candidate_flows": []any{map[string]any{
			"name":       "Fixture direction",
			"confidence": 0.3,
			"local_verification": map[string]any{
				"status":         "partial",
				"confidence_cap": 0.3,
			},
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []any{},
		"unverified_paths":       []any{},
		// The first byte-identical warning is provider-authored. The local gate
		// appends its own validated warning only at the end.
		"warnings": []string{productWarning, modelWarning, productWarning},
	}
	encoded, err := json.Marshal(orientation)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "orientation_report.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	writeOrientationWarningSidecar(t, runDir, encoded, []orient.ConfidenceWarningDiagnostic{{
		WarningIndex:   2,
		Code:           orient.ConfidenceWarningCandidateCapped,
		CandidateIndex: 0,
		Proposed:       0.9,
		Capped:         0.3,
	}})
	data := &ReportData{}
	if warning := parseOrientationReport(path, data); warning != "" {
		t.Fatal(warning)
	}

	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	foundModelWarning := false
	foundProductLookalike := false
	for _, field := range prepared.Canonical.Fields {
		switch field.Text {
		case productWarning:
			foundProductLookalike = true
		case modelWarning:
			foundModelWarning = true
		}
	}
	if !foundModelWarning {
		t.Fatal("model-authored warning was removed from translation inventory")
	}
	if !foundProductLookalike {
		t.Fatal("provider-authored confidence-warning lookalike was incorrectly claimed by the product catalog")
	}

	rendered := reportDataForRendering(data)
	if len(rendered.PresentationWarningMessages) != 1 {
		t.Fatalf("typed warning messages = %#v", rendered.PresentationWarningMessages)
	}
	presentation := rendered.PresentationWarningMessages[0]
	if presentation.WarningIndex != 2 ||
		presentation.MessageID != runWarningMessageCandidateConfidenceCapped ||
		presentation.CandidateIndex != 0 ||
		presentation.Proposed != "0.90" || presentation.Capped != "0.30" {
		t.Fatalf("typed warning presentation = %#v", presentation)
	}

	withDiagnostics, err := json.Marshal(reportDataForPersistence(data))
	if err != nil {
		t.Fatal(err)
	}
	withoutDiagnostics := *data
	withoutDiagnostics.runWarningDiagnostics = nil
	withoutDiagnostics.PresentationWarningMessages = nil
	canonical, err := json.Marshal(reportDataForPersistence(&withoutDiagnostics))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withDiagnostics, canonical) {
		t.Fatalf("typed presentation metadata changed canonical report JSON\nwith: %s\nwant: %s", withDiagnostics, canonical)
	}
}

func TestProviderConfidenceWarningLookalikeWithoutCappedStateRemainsProse(t *testing.T) {
	t.Parallel()

	const raw = "local confidence gate capped candidate_flows[0] from 0.90 to 0.30"
	runDir := t.TempDir()
	orientation := map[string]any{"warnings": []string{raw}}
	encoded, err := json.Marshal(orientation)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "orientation_report.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	data := &ReportData{}
	if warning := parseOrientationReport(path, data); warning != "" {
		t.Fatal(warning)
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, field := range prepared.Canonical.Fields {
		if field.Text == raw {
			found = true
		}
	}
	if !found {
		t.Fatal("provider lookalike without matching capped state was incorrectly claimed")
	}
}

func TestOrientationConfidenceWarningUsesValidatedProducerSidecar(t *testing.T) {
	t.Parallel()

	const raw = "local confidence gate capped orientation from 0.90 to 0.60 because focused retrieval is incomplete"
	runDir := t.TempDir()
	orientation := map[string]any{
		"confidence": 0.3,
		"candidate_flows": []any{map[string]any{
			"name":       "Already conservative direction",
			"confidence": 0.3,
			"local_verification": map[string]any{
				"status":         "partial",
				"confidence_cap": 0.6,
			},
		}},
		"warnings": []string{raw},
	}
	encoded, err := json.Marshal(orientation)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "orientation_report.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	writeOrientationWarningSidecar(t, runDir, encoded, []orient.ConfidenceWarningDiagnostic{{
		WarningIndex:   0,
		Code:           orient.ConfidenceWarningOrientationCapped,
		CandidateIndex: -1,
		Proposed:       0.9,
		Capped:         0.6,
	}})
	data := &ReportData{}
	if warning := parseOrientationReport(path, data); warning != "" {
		t.Fatal(warning)
	}
	if len(data.runWarningDiagnostics) != 1 ||
		data.runWarningDiagnostics[0].Code != orient.ConfidenceWarningOrientationCapped {
		t.Fatalf("orientation confidence diagnostics = %#v", data.runWarningDiagnostics)
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range prepared.Canonical.Fields {
		if field.Text == raw {
			t.Fatal("validated orientation confidence warning entered translation inventory")
		}
	}
}

func writeOrientationWarningSidecar(
	t *testing.T,
	runDir string,
	orientationJSON []byte,
	diagnostics []orient.ConfidenceWarningDiagnostic,
) {
	t.Helper()
	encoded, err := orient.EncodeConfidenceWarningDiagnostics(
		orientationJSON,
		diagnostics,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
		encoded,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func TestConfidenceWarningDiagnosticsHydrateOnlyAfterCanonicalWarningReplay(t *testing.T) {
	t.Parallel()

	const raw = "local confidence gate capped orientation from 0.90 to 0.60 because focused retrieval is incomplete"
	replayed := &ReportData{
		Warnings: []string{raw},
		runWarningDiagnostics: []runWarningDiagnostic{{
			WarningIndex: 0,
			Code:         orient.ConfidenceWarningOrientationCapped,
			Proposed:     0.9,
			Capped:       0.6,
		}},
	}
	target := &ReportData{Warnings: []string{raw}}
	if err := hydratePresentationMetadataFromReplay(target, replayed); err != nil {
		t.Fatal(err)
	}
	if len(target.runWarningDiagnostics) != 1 ||
		target.runWarningDiagnostics[0].Code != orient.ConfidenceWarningOrientationCapped {
		t.Fatalf("hydrated diagnostics = %#v", target.runWarningDiagnostics)
	}

	target = &ReportData{Warnings: []string{"different canonical warning"}}
	if err := hydratePresentationMetadataFromReplay(target, replayed); err == nil {
		t.Fatal("mismatched canonical warning vector accepted typed metadata")
	}
}
