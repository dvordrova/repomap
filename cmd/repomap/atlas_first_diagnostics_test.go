package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

func TestAtlasStudyDiagnosticPreservesAcceptedPartial(t *testing.T) {
	diagnostic := atlasStudyAtlasFirstDiagnostic(themeStudyRunOutcome{
		State: atlasstudy.ProductStateAcceptedPartial, SemanticCalls: 1,
		TransportAttempts: 1, RequestBytes: 123,
	}, nil, true)
	if diagnostic.State != "accepted_partial" || diagnostic.SemanticCalls != 1 ||
		diagnostic.TransportAttempts != 1 || diagnostic.RequestBytes != 123 {
		t.Fatalf("partial Atlas Study diagnostic = %#v", diagnostic)
	}

	cached := atlasStudyAtlasFirstDiagnostic(themeStudyRunOutcome{
		State: atlasstudy.ProductStateAcceptedPartial, Cached: true, RequestBytes: 123,
	}, nil, true)
	if cached.State != "cache_hit" || cached.SemanticCalls != 0 || cached.TransportAttempts != 0 {
		t.Fatalf("partial cached Atlas Study diagnostic = %#v", cached)
	}
}

func TestArchitectureDiagnosticReportsExactGraphUnavailableWithoutProviderCall(t *testing.T) {
	t.Parallel()

	diagnostic := architectureAtlasFirstDiagnostic(
		architectureSynthesisOutcome{},
		&report.ExactWorkspaceGraphUnavailableError{},
		false,
	)
	if diagnostic.State != "unavailable" || diagnostic.SemanticCalls != 0 ||
		diagnostic.TransportAttempts != 0 || diagnostic.RequestBytes != 0 {
		t.Fatalf("exact-graph diagnostic = %#v", diagnostic)
	}
}

func TestArchitectureDiagnosticPreservesAcceptedPartialCoverage(t *testing.T) {
	t.Parallel()

	diagnostic := architectureAtlasFirstDiagnostic(architectureSynthesisOutcome{
		Attempted: true, InputBytes: 1200, TransportAttempts: 1,
		ValidationOutcome:        componentmap.ValidationAcceptedPartial,
		RequestedConceptualCount: 28, CoveredConceptualCount: 10,
		UncoveredConceptualCount:   18,
		RequestedPrimaryScopeCount: 18, CoveredPrimaryScopeCount: 6,
		UncoveredPrimaryScopeCount: 12, CoveredSupportingEvidenceCount: 4,
	}, nil, false)
	if diagnostic.State != "accepted_partial" || diagnostic.Outcome == nil ||
		diagnostic.Outcome.Code != "accepted_partial" {
		t.Fatalf("partial Architecture diagnostic = %#v", diagnostic)
	}
	metrics := make(map[string]int, len(diagnostic.Outcome.Metrics))
	for _, metric := range diagnostic.Outcome.Metrics {
		metrics[metric.Name] = metric.Value
	}
	if metrics["requested_conceptual_count"] != 28 ||
		metrics["covered_conceptual_count"] != 10 ||
		metrics["uncovered_conceptual_count"] != 18 ||
		metrics["requested_primary_scope_count"] != 18 ||
		metrics["covered_primary_scope_count"] != 6 ||
		metrics["uncovered_primary_scope_count"] != 12 ||
		metrics["covered_supporting_evidence_count"] != 4 {
		t.Fatalf("partial Architecture metrics = %#v", metrics)
	}
}

func TestArchitectureDiagnosticPreservesFullAndCachedPrimaryCoverage(t *testing.T) {
	t.Parallel()

	base := architectureSynthesisOutcome{
		Attempted: true, InputBytes: 1200, TransportAttempts: 1,
		ValidationOutcome:        componentmap.ValidationAccepted,
		RequestedConceptualCount: 3, CoveredConceptualCount: 3,
		RequestedPrimaryScopeCount: 2, CoveredPrimaryScopeCount: 2,
		CoveredSupportingEvidenceCount: 1,
	}
	for _, test := range []struct {
		name, state, code string
		outcome           architectureSynthesisOutcome
	}{
		{name: "live", state: "accepted", code: "accepted", outcome: base},
		{name: "cached", state: "cache_hit", code: "cache_hit", outcome: func() architectureSynthesisOutcome {
			cached := base
			cached.Cached = true
			cached.Attempted = false
			cached.TransportAttempts = 0
			return cached
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := architectureAtlasFirstDiagnostic(test.outcome, nil, false)
			if diagnostic.State != test.state || diagnostic.Outcome == nil ||
				diagnostic.Outcome.Code != test.code {
				t.Fatalf("Architecture diagnostic = %#v", diagnostic)
			}
			metrics := make(map[string]int, len(diagnostic.Outcome.Metrics))
			for _, metric := range diagnostic.Outcome.Metrics {
				metrics[metric.Name] = metric.Value
			}
			if metrics["requested_primary_scope_count"] != 2 ||
				metrics["covered_primary_scope_count"] != 2 ||
				metrics["covered_supporting_evidence_count"] != 1 {
				t.Fatalf("Architecture diagnostic metrics = %#v", metrics)
			}
		})
	}
}

func TestArchitectureDiagnosticPreservesOutputLimitCode(t *testing.T) {
	t.Parallel()

	limitErr := &architectureOutputResourceExhausted{cause: &modelresearch.ResourceLimitError{
		Stage: "architecture_synthesis", Kind: modelresearch.ResourceLimitOutputTokens,
		Limit: 64_000,
	}}
	diagnostic := architectureAtlasFirstDiagnostic(architectureSynthesisOutcome{
		Attempted: true, InputBytes: 1200, TransportAttempts: 1,
		Failure: &report.ArchitectureSynthesisFailure{
			Stage: "provider_call", Code: "architecture.provider_output_limit",
		},
	}, limitErr, false)
	if diagnostic.State != "resource_exhausted" || diagnostic.Outcome == nil ||
		diagnostic.Outcome.Phase != "provider_call" ||
		diagnostic.Outcome.Code != "architecture.provider_output_limit" {
		t.Fatalf("output-limit Architecture diagnostic = %#v", diagnostic)
	}
}

func TestAtlasFirstStageDiagnosticsAreExactIdempotentAndExplicitlyComplete(t *testing.T) {
	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteMetadata(debugdump.RunMeta{
		RunID: "diagnostic-run", Command: "atlas-first",
	}); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	observations := []atlasFirstStageDiagnostic{
		{Stage: debugdump.SemanticStageArchitecture, State: "cached", RequestBytes: 202, LatencyMillis: 22},
		{Stage: debugdump.SemanticStageAtlasStudy, State: "accepted", RequestBytes: 303, SemanticCalls: 2, TransportAttempts: 2, LatencyMillis: 33},
	}
	for _, observation := range observations {
		if err := recordAtlasFirstStageDiagnostic(runDir, observation); err != nil {
			t.Fatal(err)
		}
	}
	// Re-recording one owner replaces that observation instead of double-counting it.
	observations[0].State = "cache_hit"
	if err := recordAtlasFirstStageDiagnostic(runDir, observations[0]); err != nil {
		t.Fatal(err)
	}
	metadata := readAtlasFirstMetadataFixture(t, runDir)
	if metadata.ProviderAccountingComplete || metadata.ProviderRequestCount != 2 ||
		metadata.ExternalRequestBytes != 303 || len(metadata.RequestAttempts) != 2 ||
		metadata.ProviderLatencyMillis == nil || *metadata.ProviderLatencyMillis != 33 {
		t.Fatalf("in-progress metadata = %#v", metadata)
	}
	for _, attempt := range metadata.RequestAttempts {
		if attempt.Stage == debugdump.SemanticStageArchitecture && attempt.LatencyMillis != nil {
			t.Fatalf("cache-hit Architecture exposes original call latency = %#v", attempt)
		}
	}
	if err := finalizeAtlasFirstStageDiagnostics(runDir); err != nil {
		t.Fatal(err)
	}
	metadata = readAtlasFirstMetadataFixture(t, runDir)
	if !metadata.ProviderAccountingComplete || metadata.ProviderRequestCount != 2 ||
		metadata.ExternalRequestBytes != 303 {
		t.Fatalf("complete metadata = %#v", metadata)
	}
}

func TestAtlasFirstStageDiagnosticsRejectImpossibleCallAccounting(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := recordAtlasFirstStageDiagnostic(runDir, atlasFirstStageDiagnostic{
		Stage: debugdump.SemanticStageArchitecture, State: "invalid",
		SemanticCalls: 0, TransportAttempts: 1,
	})
	if err == nil {
		t.Fatal("impossible transport attempt was accepted")
	}
}

func TestAtlasFirstStageDiagnosticsAcceptMultipleCallsAsObservations(t *testing.T) {
	runDir := t.TempDir()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteMetadata(debugdump.RunMeta{
		RunID: "multiple-call-diagnostic", Command: "atlas-first",
	}); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	if err := recordAtlasFirstStageDiagnostic(runDir, atlasFirstStageDiagnostic{
		Stage: debugdump.SemanticStageAtlasStudy, State: "accepted",
		RequestBytes: 303, SemanticCalls: 2, TransportAttempts: 3, LatencyMillis: 33,
	}); err != nil {
		t.Fatalf("record multiple-call diagnostic: %v", err)
	}
	metadata := readAtlasFirstMetadataFixture(t, runDir)
	if metadata.ProviderRequestCount != 2 || metadata.ExternalRequestBytes != 303 ||
		len(metadata.RequestAttempts) != 1 ||
		metadata.RequestAttempts[0].ProviderCallCount != 2 ||
		metadata.RequestAttempts[0].TransportAttemptCount != 3 {
		t.Fatalf("multiple-call metadata = %#v", metadata)
	}
}

func readAtlasFirstMetadataFixture(t *testing.T, runDir string) debugdump.RunMeta {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}
