package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/report"
)

const maxAtlasFirstMetadataBytes = 4 << 20

type atlasFirstStageDiagnostic struct {
	Stage             string
	State             string
	RequestBytes      int
	SemanticCalls     int
	TransportAttempts int
	LatencyMillis     int64
	Outcome           *debugdump.SemanticOutcome
}

func architectureAtlasFirstDiagnostic(
	outcome architectureSynthesisOutcome,
	stageErr error,
	offline bool,
) atlasFirstStageDiagnostic {
	state := "accepted"
	switch {
	case offline:
		state = "unavailable"
	case report.IsExactWorkspaceGraphUnavailable(stageErr):
		state = "unavailable"
	case errors.Is(stageErr, context.Canceled), errors.Is(stageErr, context.DeadlineExceeded):
		state = "canceled"
	case isSemanticResourceLimit(stageErr):
		state = "resource_exhausted"
	case stageErr != nil && outcome.ValidationOutcome == componentmap.ValidationRejected:
		state = "rejected"
	case stageErr != nil:
		state = "failed"
	case outcome.Cached:
		state = "cache_hit"
	case outcome.ValidationOutcome == componentmap.ValidationAcceptedPartial:
		state = "accepted_partial"
	}
	semanticCalls := 0
	if outcome.Attempted && !outcome.Cached {
		semanticCalls = 1
	}
	diagnostic := atlasFirstStageDiagnostic{
		Stage: debugdump.SemanticStageArchitecture, State: state,
		RequestBytes: outcome.InputBytes, SemanticCalls: semanticCalls,
		TransportAttempts: outcome.TransportAttempts, LatencyMillis: outcome.LatencyMillis,
	}
	failure := outcome.Failure
	if failure == nil && stageErr != nil && state != "canceled" && state != "resource_exhausted" && state != "unavailable" {
		failure = architectureSynthesisFailureDiagnostic(outcome, stageErr)
	}
	if failure != nil {
		diagnostic.Outcome = &debugdump.SemanticOutcome{
			Phase: failure.Stage, Code: failure.Code,
			Detail:  failure.Detail,
			Metrics: architectureSynthesisFailureMetrics(outcome),
		}
	} else if state == "accepted_partial" {
		diagnostic.Outcome = &debugdump.SemanticOutcome{
			Phase: "complete", Code: "accepted_partial",
			Metrics: architectureSynthesisOutcomeMetrics(outcome),
		}
	} else if state == "accepted" {
		diagnostic.Outcome = &debugdump.SemanticOutcome{
			Phase: "complete", Code: "accepted",
			Metrics: architectureSynthesisOutcomeMetrics(outcome),
		}
	} else if state == "cache_hit" {
		diagnostic.Outcome = &debugdump.SemanticOutcome{
			Phase: "cache", Code: "cache_hit",
			Metrics: architectureSynthesisOutcomeMetrics(outcome),
		}
	}
	return diagnostic
}

func atlasStudyAtlasFirstDiagnostic(
	outcome themeStudyRunOutcome,
	stageErr error,
	called bool,
) atlasFirstStageDiagnostic {
	state := "not_called"
	switch {
	case errors.Is(stageErr, context.Canceled), errors.Is(stageErr, context.DeadlineExceeded):
		state = "canceled"
	case isSemanticResourceLimit(stageErr):
		state = "resource_exhausted"
	case stageErr != nil:
		state = "failed"
	case !called:
		state = "not_called"
	case outcome.ProviderSkipped || outcome.State == atlasstudy.ProductStateUnavailable:
		state = "unavailable"
	case outcome.State == atlasstudy.ProductStateFailed:
		state = "failed"
	case outcome.Cached:
		state = "cache_hit"
	case outcome.State == atlasstudy.ProductStateAcceptedPartial:
		state = "accepted_partial"
	case outcome.State == atlasstudy.ProductStateAccepted:
		state = "accepted"
	}
	return atlasFirstStageDiagnostic{
		Stage: debugdump.SemanticStageAtlasStudy, State: state,
		RequestBytes: outcome.RequestBytes, SemanticCalls: outcome.SemanticCalls,
		TransportAttempts: outcome.TransportAttempts, LatencyMillis: outcome.LatencyMillis,
	}
}

func recordAtlasFirstStageDiagnostic(runDir string, diagnostic atlasFirstStageDiagnostic) error {
	if diagnostic.Stage == "" || diagnostic.State == "" || diagnostic.RequestBytes < 0 ||
		diagnostic.SemanticCalls < 0 || diagnostic.TransportAttempts < 0 ||
		(diagnostic.SemanticCalls == 0 && diagnostic.TransportAttempts != 0) ||
		diagnostic.LatencyMillis < 0 {
		return fmt.Errorf("Atlas-first diagnostics: invalid stage observation")
	}
	metadata, err := readAtlasFirstMetadata(runDir)
	if err != nil {
		return err
	}
	var latency *int64
	if diagnostic.State != "cache_hit" {
		observed := diagnostic.LatencyMillis
		latency = &observed
	}
	attempt := debugdump.RequestAttempt{
		Stage: diagnostic.Stage, State: diagnostic.State,
		RequestBytes:          diagnostic.RequestBytes,
		ProviderCallCount:     diagnostic.SemanticCalls,
		TransportAttemptCount: diagnostic.TransportAttempts,
		LatencyMillis:         latency,
		Outcome:               diagnostic.Outcome,
	}
	updated := make([]debugdump.RequestAttempt, 0, len(metadata.RequestAttempts)+1)
	for _, existing := range metadata.RequestAttempts {
		if existing.Stage != diagnostic.Stage {
			updated = append(updated, existing)
		}
	}
	metadata.RequestAttempts = append(updated, attempt)
	metadata.ProviderAccountingComplete = false
	recomputeAtlasFirstProviderTotals(&metadata)
	return writeAtlasFirstMetadata(runDir, metadata)
}

func finalizeAtlasFirstStageDiagnostics(runDir string) error {
	metadata, err := readAtlasFirstMetadata(runDir)
	if err != nil {
		return err
	}
	metadata.ProviderAccountingComplete = true
	recomputeAtlasFirstProviderTotals(&metadata)
	return writeAtlasFirstMetadata(runDir, metadata)
}

func recomputeAtlasFirstProviderTotals(metadata *debugdump.RunMeta) {
	metadata.ProviderRequestCount = 0
	metadata.ExternalRequestBytes = 0
	var totalLatency int64
	latencyKnown := false
	for _, attempt := range metadata.RequestAttempts {
		metadata.ProviderRequestCount += attempt.ProviderCallCount
		if attempt.ProviderCallCount > 0 {
			metadata.ExternalRequestBytes += attempt.RequestBytes
		}
		if attempt.LatencyMillis != nil {
			totalLatency += *attempt.LatencyMillis
			latencyKnown = true
		}
	}
	if latencyKnown {
		metadata.ProviderLatencyMillis = &totalLatency
	} else {
		metadata.ProviderLatencyMillis = nil
	}
}

func readAtlasFirstMetadata(runDir string) (debugdump.RunMeta, error) {
	path := filepath.Join(runDir, "metadata.json")
	info, err := os.Lstat(path)
	if err != nil {
		return debugdump.RunMeta{}, fmt.Errorf("Atlas-first diagnostics: inspect metadata: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxAtlasFirstMetadataBytes {
		return debugdump.RunMeta{}, fmt.Errorf("Atlas-first diagnostics: metadata is not a bounded regular file")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return debugdump.RunMeta{}, fmt.Errorf("Atlas-first diagnostics: read metadata: %w", err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return debugdump.RunMeta{}, fmt.Errorf("Atlas-first diagnostics: decode metadata: %w", err)
	}
	return metadata, nil
}

func writeAtlasFirstMetadata(runDir string, metadata debugdump.RunMeta) error {
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return fmt.Errorf("Atlas-first diagnostics: open confined writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteMetadata(metadata); err != nil {
		return fmt.Errorf("Atlas-first diagnostics: write metadata: %w", err)
	}
	return nil
}
