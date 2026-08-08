package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestD242RunManifestRejectsHistoricalReportFormat(t *testing.T) {
	t.Parallel()

	manifest := validRunManifestFixture(t)
	manifest.ReportFormatVersion = CurrentFormatVersion - 1
	manifest.MaterialInputs.ReportContract = manifest.ReportFormatVersion
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported report format version") {
		t.Fatalf("historical report format error = %v", err)
	}
}

func TestD242RunManifestRejectsMechanismFragmentDrift(t *testing.T) {
	t.Parallel()

	data := mechanismFragmentFixture()
	data.FormatVersion = CurrentFormatVersion
	data.OpenablePaths = []string{"ldap/server.go", "main.go", "service.go"}
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatalf("ensure mechanism fragments: %v", err)
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
		manifest.OpenablePaths = append([]string(nil), data.OpenablePaths...)
		manifest.Components = nil
		manifest.ReportSHA256 = manifestSHA256(reportJSON)
		return manifest.VerifyReportJSON(reportJSON)
	}
	if err := verify(t, data); err != nil {
		t.Fatalf("valid exact mechanism fragments rejected: %v", err)
	}

	drifted := mechanismFragmentFixture()
	drifted.FormatVersion = CurrentFormatVersion
	drifted.OpenablePaths = append([]string(nil), data.OpenablePaths...)
	if err := ensureMechanismFragments(drifted); err != nil {
		t.Fatal(err)
	}
	if err := ensureArchitectureComponentNavigation(drifted); err != nil {
		t.Fatal(err)
	}
	drifted.ArchitectureCanvas.MechanismFragments[0].Handoffs[0].Label = "drifted handoff"
	if err := verify(t, drifted); err == nil ||
		!strings.Contains(err.Error(), "persisted projection does not match exact local evidence") {
		t.Fatalf("drifted mechanism fragment error = %v", err)
	}
}
