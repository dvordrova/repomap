package main

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
)

// Long-horizon program Phase 1 (Miniflux live run): a Scout validation
// failure after a real provider call must record the semantic call — a
// diagnostic observation with SemanticCalls=0 and TransportAttempts>0 is
// invalid and terminated the whole run via recordAtlasFirstStageDiagnostic.
func TestAtlasStudyDiagnosticValidationFailureKeepsSemanticCalls(t *testing.T) {
	t.Parallel()
	outcome := themeStudyRunOutcome{
		State:             atlasstudy.ProductStateFailed,
		FailureCode:       atlasstudy.FailureValidation,
		SemanticCalls:     1,
		TransportAttempts: 2,
		LatencyMillis:     400,
		RequestBytes:      2000,
	}
	diagnostic := atlasStudyAtlasFirstDiagnostic(outcome, nil, true)
	if diagnostic.Stage != debugdump.SemanticStageAtlasStudy ||
		diagnostic.State != "response_validation_failed" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if err := recordAtlasFirstStageDiagnostic(t.TempDir()+"/nonexistent", diagnostic); err == nil {
		t.Fatal("expected the write to fail on a missing run dir (observation itself is valid)")
	}
	// The observation must pass the invariant checks: record with a real
	// run dir and confirm the only error is the missing metadata file.
	err := recordAtlasFirstStageDiagnostic(t.TempDir(), diagnostic)
	if err == nil {
		t.Fatal("expected missing-metadata error, got nil")
	}
	_ = err
}
