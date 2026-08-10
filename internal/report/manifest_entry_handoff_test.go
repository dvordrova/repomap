package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRunManifestRejectsHistoricalEntrypointHandoffReportFormat(t *testing.T) {
	t.Parallel()

	manifest := validRunManifestFixture(t)
	manifest.ReportFormatVersion = CurrentFormatVersion - 1
	manifest.MaterialInputs.ReportContract = manifest.ReportFormatVersion
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported report format version") {
		t.Fatalf("historical report format error = %v", err)
	}
}

func TestRunManifestRejectsEntrypointHandoffGroupDrift(t *testing.T) {
	t.Parallel()

	data := entrypointHandoffGroupFixture()
	data.FormatVersion = CurrentFormatVersion
	data.AnalysisTarget = reportAnalysisTargetFixture(t)
	data.OpenablePaths = []string{"ldap/server.go", "main.go", "service.go"}
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatalf("ensure entry handoff groups: %v", err)
	}
	if err := ensureArchitectureComponentNavigation(data); err != nil {
		t.Fatalf("ensure architecture navigation: %v", err)
	}

	verify := func(t *testing.T, data *ReportData) error {
		t.Helper()
		reportJSON, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		manifest := validRunManifestFixture(t)
		bindRunManifestAnalysisTarget(t, &manifest, data)
		manifest.OpenablePaths = append([]string(nil), data.OpenablePaths...)
		manifest.Components = nil
		manifest.ReportSHA256 = manifestSHA256(reportJSON)
		return manifest.VerifyReportJSON(reportJSON)
	}
	if err := verify(t, data); err != nil {
		t.Fatalf("valid exact entry handoff groups rejected: %v", err)
	}

	drifted := entrypointHandoffGroupFixture()
	drifted.FormatVersion = CurrentFormatVersion
	drifted.AnalysisTarget = reportAnalysisTargetFixture(t)
	drifted.OpenablePaths = append([]string(nil), data.OpenablePaths...)
	if err := ensureEntrypointHandoffGroups(drifted); err != nil {
		t.Fatal(err)
	}
	if err := ensureArchitectureComponentNavigation(drifted); err != nil {
		t.Fatal(err)
	}
	drifted.ArchitectureCanvas.EntryHandoffGroups[0].EntryHandoffs[0].EvidenceRef.ID = "entry-handoff-drifted"
	if err := verify(t, drifted); err == nil ||
		!strings.Contains(err.Error(), "persisted projection does not match exact local evidence") {
		t.Fatalf("drifted entry handoff group error = %v", err)
	}
}
