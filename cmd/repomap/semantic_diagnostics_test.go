package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pipeline"
)

func TestRecordSemanticPipelineAccountingUpdatesMetadataExactlyOnce(t *testing.T) {
	writer, err := debugdump.NewWriter(t.TempDir(), "pipeline-accounting", false)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	if err := writer.WriteMetadata(debugdump.RunMeta{
		RunID: writer.RunID,
		RequestAttempts: []debugdump.RequestAttempt{{
			Stage: debugdump.SemanticStageTargetPortfolio,
			State: debugdump.SemanticStateCacheHit,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	events := []pipeline.AccountingEvent{
		{
			Stage: debugdump.SemanticStageActivityEntrypoints, Ordinal: 1,
			State: pipeline.AccountingAccepted, RequestBytes: 100,
			SemanticCalls: 1, TransportAttempts: 2,
			Metrics: llm.Metrics{Latency: 7 * time.Millisecond, Attempts: 2},
		},
		{
			Stage: debugdump.SemanticStageActivityEntrypoints, Ordinal: 2,
			State: pipeline.AccountingCacheHit, RequestBytes: 80,
			Metrics: llm.Metrics{Latency: 7 * time.Millisecond, Attempts: 2},
		},
		{
			Stage: debugdump.SemanticStageCoreMapRefined, Ordinal: 1,
			State: pipeline.AccountingAccepted, RequestBytes: 200,
			SemanticCalls: 1, TransportAttempts: 1,
			Metrics: llm.Metrics{Latency: 3 * time.Millisecond, Attempts: 1},
		},
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := recordSemanticPipelineAccounting(runDir, events); err != nil {
			t.Fatal(err)
		}
	}
	metadata, err := readSemanticMetadata(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.RequestAttempts) != 4 || metadata.ProviderRequestCount != 2 ||
		metadata.ExternalRequestBytes != 300 || metadata.ProviderLatencyMillis == nil ||
		*metadata.ProviderLatencyMillis != 10 || metadata.ProviderAccountingComplete {
		t.Fatalf("pipeline accounting metadata = %#v", metadata)
	}
	for _, attempt := range metadata.RequestAttempts {
		if attempt.Outcome == nil {
			t.Fatalf("request attempt has no closed outcome: %#v", attempt)
		}
	}
}
