package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
)

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
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return debugdump.RunMeta{}, fmt.Errorf("semantic diagnostics: metadata is not a non-empty regular file")
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
