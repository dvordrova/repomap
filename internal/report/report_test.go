package report

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseRunMetadataRetainsContentBeyondFormerLocalByteThreshold(t *testing.T) {
	metadata := runMetadataJSON{
		RepoName: "project",
		Warnings: []string{strings.Repeat("x", advisoryReportTargetMetadataBytes+1)},
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= advisoryReportTargetMetadataBytes {
		t.Fatal("fixture did not cross the former metadata threshold")
	}
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	data := &ReportData{RepoName: "project"}
	if err := parseRunMetadata(path, data); err != nil {
		t.Fatalf("metadata above former local threshold: %v", err)
	}
	if !reflect.DeepEqual(data.Warnings, metadata.Warnings) {
		t.Fatal("metadata warnings were not retained")
	}
	warnings := ReportInputScaleWarnings(data)
	if len(warnings) != 1 || warnings[0].Kind != ReportScaleWarningTargetMetadataBytes {
		t.Fatalf("metadata scale warnings = %#v", warnings)
	}
}

func TestPublishedReportScaleWarningsAreStatOnlyAndNonFatal(t *testing.T) {
	runDir := t.TempDir()
	for _, file := range []struct {
		name string
		size int64
	}{
		{name: "report.json", size: int64(MaxReportJSONBytes) + 1},
		// A sparse report larger than the former raw-bundle threshold proves
		// publication metadata is not misreported as raw browser payload.
		{name: "report.html", size: int64(MaxOrdinaryReportHTMLBytes) + 1},
	} {
		path := filepath.Join(runDir, file.name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(path, file.size); err != nil {
			t.Fatal(err)
		}
	}
	warnings := PublishedReportScaleWarnings(runDir)
	if len(warnings) != 2 || warnings[0].Kind != ReportScaleWarningReportJSONBytes ||
		warnings[1].Kind != ReportScaleWarningReportHTMLBytes {
		t.Fatalf("published report warnings = %#v", warnings)
	}
	for _, warning := range warnings {
		if warning.Kind == ReportScaleWarningBundleBytes {
			t.Fatalf("report file metadata became raw bundle accounting: %#v", warnings)
		}
	}
	if warnings := PublishedReportScaleWarnings(filepath.Join(runDir, "missing")); len(warnings) != 0 {
		t.Fatalf("missing diagnostic path became an error-like warning: %#v", warnings)
	}
}

func TestStandaloneBundlePayloadScaleWarningsUseExactRawPayloadMeasurement(t *testing.T) {
	if warnings := standaloneBundlePayloadScaleWarnings(AdvisoryStandaloneTargetBundlePayloadBytes); len(warnings) != 0 {
		t.Fatalf("at-threshold raw payload warnings = %#v", warnings)
	}
	retained := AdvisoryStandaloneTargetBundlePayloadBytes + 1
	warnings := standaloneBundlePayloadScaleWarnings(retained)
	if len(warnings) != 1 || warnings[0] != (ReportInputScaleWarning{
		Kind:         ReportScaleWarningBundleBytes,
		Retained:     int(retained),
		AdvisorySize: int(AdvisoryStandaloneTargetBundlePayloadBytes),
	}) {
		t.Fatalf("raw payload warnings = %#v", warnings)
	}
}

func TestFailedPublicationAssessmentRetainsCompletedScaleWarnings(t *testing.T) {
	want := []ReportInputScaleWarning{{
		Kind:         ReportScaleWarningBundleBytes,
		Retained:     int(AdvisoryStandaloneTargetBundlePayloadBytes) + 1,
		AdvisorySize: int(AdvisoryStandaloneTargetBundlePayloadBytes),
	}}
	assessment := completedPublicationAssessment(errors.New("late atomic publication failure"), want, nil)
	if assessment.Status != PublicationFailed || !reflect.DeepEqual(assessment.ScaleWarnings(), want) {
		t.Fatalf("failed publication diagnostics = %#v / %#v", assessment, assessment.ScaleWarnings())
	}
	want[0].Retained = 0
	if assessment.ScaleWarnings()[0].Retained == 0 {
		t.Fatal("failed assessment retained caller-owned warning storage")
	}
}

func TestFailedPublicationAssessmentRetainsTargetScaleWarnings(t *testing.T) {
	want := []TargetReportScaleWarning{{
		SelectedTargetID: "selected-worker", ProgramTargetID: "program-worker",
		Warning: ReportInputScaleWarning{
			Kind:         ReportScaleWarningTargetBundleRawBytes,
			Retained:     int(AdvisoryStandaloneTargetPayloadBytes) + 1,
			AdvisorySize: int(AdvisoryStandaloneTargetPayloadBytes),
		},
	}}
	assessment := completedPublicationAssessment(
		errors.New("late sibling failure"), nil, want,
	)
	if assessment.Status != PublicationFailed ||
		!reflect.DeepEqual(assessment.TargetScaleWarnings(), want) {
		t.Fatalf("failed target diagnostics = %#v", assessment.TargetScaleWarnings())
	}
	want[0].Warning.Retained = 0
	if assessment.TargetScaleWarnings()[0].Warning.Retained == 0 {
		t.Fatal("failed assessment retained caller-owned target warning storage")
	}
}
