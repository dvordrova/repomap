package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/pipeline"
)

const maxSemanticMetadataBytes = 4 << 20

type semanticStageDiagnostic struct {
	Stage             string
	State             string
	RequestBytes      int
	SemanticCalls     int
	TransportAttempts int
	LatencyMillis     int64
	Outcome           *debugdump.SemanticOutcome
}

func recordSemanticStageDiagnostic(runDir string, diagnostic semanticStageDiagnostic) error {
	if diagnostic.Stage == "" || diagnostic.State == "" || diagnostic.RequestBytes < 0 ||
		diagnostic.SemanticCalls < 0 || diagnostic.TransportAttempts < 0 ||
		(diagnostic.SemanticCalls == 0 && diagnostic.TransportAttempts != 0) ||
		diagnostic.LatencyMillis < 0 {
		return fmt.Errorf("semantic diagnostics: invalid stage observation")
	}
	metadata, err := readSemanticMetadata(runDir)
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
	recomputeSemanticProviderTotals(&metadata)
	return writeSemanticMetadata(runDir, metadata)
}

// recordSemanticPipelineAccounting projects the pipeline's payload-free,
// per-call accounting events into ordinary run metadata in one confined
// update. Repeated stage rows are retained because bounded cube batches are
// distinct provider calls; Ordinal remains available on the pipeline result.
func recordSemanticPipelineAccounting(
	runDir string,
	events []pipeline.AccountingEvent,
) error {
	if len(events) == 0 {
		return nil
	}
	attempts := make([]debugdump.RequestAttempt, 0, len(events))
	nextOrdinal := make(map[string]int)
	for _, event := range events {
		if !semanticPipelineAccountingStage(event.Stage) ||
			event.Ordinal != nextOrdinal[event.Stage]+1 ||
			event.RequestBytes < 0 || event.SemanticCalls < 0 || event.SemanticCalls > 1 ||
			event.TransportAttempts < 0 || event.Metrics.Latency < 0 ||
			(event.SemanticCalls == 0 && event.TransportAttempts != 0) {
			return fmt.Errorf("semantic diagnostics: invalid pipeline accounting event")
		}
		nextOrdinal[event.Stage] = event.Ordinal
		state, err := semanticPipelineAccountingState(event.State)
		if err != nil {
			return err
		}
		var latency *int64
		if event.SemanticCalls > 0 {
			value := event.Metrics.Latency.Milliseconds()
			latency = &value
		}
		attempts = append(attempts, debugdump.RequestAttempt{
			Stage: event.Stage, State: state, RequestBytes: event.RequestBytes,
			ProviderCallCount: event.SemanticCalls, TransportAttemptCount: event.TransportAttempts,
			LatencyMillis: latency,
		})
	}

	metadata, err := readSemanticMetadata(runDir)
	if err != nil {
		return err
	}
	retained := make([]debugdump.RequestAttempt, 0, len(metadata.RequestAttempts)+len(attempts))
	for _, attempt := range metadata.RequestAttempts {
		if !semanticPipelineAccountingStage(attempt.Stage) {
			retained = append(retained, attempt)
		}
	}
	metadata.RequestAttempts = append(retained, attempts...)
	// This function proves the complete ProgramIndex semantic chain only. The
	// run may contain earlier target/README calls with separate owners.
	metadata.ProviderAccountingComplete = false
	recomputeSemanticProviderTotals(&metadata)
	return writeSemanticMetadata(runDir, metadata)
}

func semanticPipelineAccountingStage(stage string) bool {
	switch stage {
	case debugdump.SemanticStageActivityEntrypoints,
		debugdump.SemanticStageIntegrationDependencies,
		debugdump.SemanticStageIntegrationUsage,
		debugdump.SemanticStageCoreMapBaseline,
		debugdump.SemanticStageCoreMapRefined:
		return true
	default:
		return false
	}
}

func semanticPipelineAccountingState(state pipeline.AccountingState) (string, error) {
	switch state {
	case pipeline.AccountingAccepted:
		return debugdump.SemanticStateAccepted, nil
	case pipeline.AccountingCacheHit:
		return debugdump.SemanticStateCacheHit, nil
	case pipeline.AccountingRejected:
		return debugdump.SemanticStateRejected, nil
	case pipeline.AccountingProviderFailed:
		return debugdump.SemanticStateProviderFailed, nil
	default:
		return "", fmt.Errorf("semantic diagnostics: invalid pipeline accounting state")
	}
}

func recomputeSemanticProviderTotals(metadata *debugdump.RunMeta) {
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

func readSemanticMetadata(runDir string) (debugdump.RunMeta, error) {
	path := filepath.Join(runDir, "metadata.json")
	info, err := os.Lstat(path)
	if err != nil {
		return debugdump.RunMeta{}, fmt.Errorf("semantic diagnostics: inspect metadata: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSemanticMetadataBytes {
		return debugdump.RunMeta{}, fmt.Errorf("semantic diagnostics: metadata is not a bounded regular file")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return debugdump.RunMeta{}, fmt.Errorf("semantic diagnostics: read metadata: %w", err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return debugdump.RunMeta{}, fmt.Errorf("semantic diagnostics: decode metadata: %w", err)
	}
	return metadata, nil
}

func writeSemanticMetadata(runDir string, metadata debugdump.RunMeta) error {
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return fmt.Errorf("semantic diagnostics: open confined writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteMetadata(metadata); err != nil {
		return fmt.Errorf("semantic diagnostics: write metadata: %w", err)
	}
	return nil
}

func flushFirstLayerSemanticJournal(
	runDir string,
	observer *debugdump.SemanticObserver,
	output *runOutput,
) {
	if observer == nil {
		return
	}
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		if output != nil {
			output.Warn("First-layer semantic exchange journal unavailable", err.Error())
		}
		return
	}
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "First-layer semantic exchange journal unavailable",
	})
	observer.Flush(writer)
	if err := writer.Close(); err != nil && output != nil {
		output.Warn("First-layer semantic exchange journal unavailable", err.Error())
	}
}

// flushFailedFirstLayerSemanticJournal materializes the already-announced run
// directory only after an early first-layer failure. Successful runs still let
// the ordinary artifact writer create the directory and metadata authority.
func flushFailedFirstLayerSemanticJournal(
	runDir string,
	observer *debugdump.SemanticObserver,
	output *runOutput,
) {
	if observer == nil {
		return
	}
	if err := os.Mkdir(runDir, 0o700); err != nil && !os.IsExist(err) {
		if output != nil {
			output.Warn("First-layer semantic exchange journal unavailable", err.Error())
		}
		return
	}
	flushFirstLayerSemanticJournal(runDir, observer, output)
}
