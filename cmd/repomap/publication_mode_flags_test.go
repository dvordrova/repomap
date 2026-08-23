package main

import (
	"strings"
	"testing"
)

func TestExplicitPortIsRejectedOnlyForStaticReportMode(t *testing.T) {
	if err := validateReportModeFlags(false, true); err != nil {
		t.Fatalf("served explicit port: %v", err)
	}
	if err := validateReportModeFlags(true, false); err != nil {
		t.Fatalf("static report without explicit port: %v", err)
	}
	err := validateReportModeFlags(true, true)
	if err == nil || !strings.Contains(err.Error(), "remove --port") ||
		!strings.Contains(err.Error(), "serve locally") {
		t.Fatalf("static explicit port error = %v", err)
	}
}

func TestSemanticStopAfterAcceptsOnlyActivityEntrypoints(t *testing.T) {
	if stage, err := semanticStopAfter(""); err != nil || stage != "" {
		t.Fatalf("empty checkpoint = %q, %v", stage, err)
	}
	if stage, err := semanticStopAfter(" ActivityEntrypoints "); err != nil || stage != "activity_entrypoints" {
		t.Fatalf("activity checkpoint = %q, %v", stage, err)
	}
	if _, err := semanticStopAfter("CoreMap"); err == nil ||
		!strings.Contains(err.Error(), "supported checkpoint is ActivityEntrypoints") {
		t.Fatalf("unsupported checkpoint error = %v", err)
	}
}
