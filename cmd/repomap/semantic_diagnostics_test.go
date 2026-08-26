package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pipeline"
)

func TestRepositoryTargetDispatchPreflightFailureFlushesFirstLayerSemanticJournal(t *testing.T) {
	repositoryRoot := t.TempDir()
	paths := []string{"README.md", "package.json", "src/main.ts"}
	contents := map[string]string{
		"README.md":    "# Web\n\nRun the TypeScript application.\n",
		"package.json": `{"name":"web","scripts":{"start":"node src/main.ts"}}`,
		"src/main.ts":  "export const start = true\n",
	}
	for _, filePath := range paths {
		absolute := filepath.Join(repositoryRoot, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(contents[filePath]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.New(context.Background(), repositoryRoot, gitfiles.Listing{
		Paths: paths, RegularPaths: paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	manifestRef, ok := repository.ID("package.json")
	if !ok {
		t.Fatal("package manifest is absent from the test corpus")
	}
	portfolioResponse, err := json.Marshal(map[string]any{
		"default_file_ref": manifestRef,
		"target_file_refs": []corpus.FileID{manifestRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
	portfolioProvider := &targetPortfolioClientStub{response: portfolioResponse}
	providerCalls := 0
	providerFactory := func() (llm.Provider, error) {
		providerCalls++
		if providerCalls == 1 {
			return readmeProvider, nil
		}
		return portfolioProvider, nil
	}
	state := freshness.RepositoryState{
		Version: freshness.RepositoryStateVersion, Identity: repositoryRoot,
		Head: strings.Repeat("a", 40), Dirty: []freshness.DirtyFile{},
	}
	debugDir := t.TempDir()
	const runID = "jsts-preflight-failure"
	err = runDefaultWithDeps(repositoryRoot, []string{
		"--debug-dir", debugDir, "--no-cache", "--no-open",
	}, defaultRunDeps{
		ctx: t.Context(), stdout: io.Discard, stderr: io.Discard,
		sharedRepositoryCorpus: repository, capturedRepositoryState: &state,
		newTargetPortfolioProvider: providerFactory, runIDOverride: runID,
	})
	if err == nil || !strings.Contains(err.Error(), "materialize selected JavaScript/TypeScript package project") {
		t.Fatalf("selected JSTS preflight error = %v", err)
	}
	if strings.Contains(err.Error(), "semantic diagnostics: inspect metadata") ||
		strings.Contains(err.Error(), "lstat") || !strings.Contains(err.Error(), "package.json") {
		t.Fatalf("selected JSTS preflight obscured the primary manifest failure: %v", err)
	}
	if providerCalls != 2 || readmeProvider.calls != 1 || portfolioProvider.calls != 1 {
		t.Fatalf(
			"first-layer provider calls = %d / README %d / portfolio %d, want 2 / 1 / 1",
			providerCalls, readmeProvider.calls, portfolioProvider.calls,
		)
	}

	runDir := filepath.Join(debugDir, runID)
	entries, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	stages := make(map[string]int, len(entries))
	for _, entry := range entries {
		raw, readErr := os.ReadFile(filepath.Join(
			runDir, debugdump.SemanticExchangesDir, entry.Name(), debugdump.SemanticExchangeMetaFile,
		))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var record debugdump.SemanticExchangeRecord
		if decodeErr := json.Unmarshal(raw, &record); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		stages[record.Stage]++
	}
	if len(stages) != 2 || stages[debugdump.SemanticStageReadmeFileClassifier] != 1 ||
		stages[debugdump.SemanticStageTargetPortfolio] != 1 {
		t.Fatalf("persisted first-layer stages = %#v", stages)
	}
	if _, err := os.Stat(filepath.Join(runDir, "metadata.json")); !os.IsNotExist(err) {
		t.Fatalf("preflight failure unexpectedly published run metadata: %v", err)
	}
}

func TestChildAndOuterFailureRecordOneFirstLayerSemanticJournal(t *testing.T) {
	debugDir := t.TempDir()
	const runID = "child-then-outer-failure"
	writer, err := debugdump.NewWriter(debugDir, runID, false)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(debugDir, runID)
	if err := writer.WriteMetadata(debugdump.RunMeta{RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	observer := debugdump.NewSemanticObserver(nil)
	readmeRequest := []byte(`{"model":"fixture","stage":"readme"}`)
	readmeResponse := []byte(`[]`)
	if err := observer.ObserveStage(debugdump.SemanticStageReadmeFileClassifier, llm.Event{
		Kind: llm.EventLive, Source: llm.SourceLive,
		Request: readmeRequest, RequestBytes: len(readmeRequest),
		Response: readmeResponse, ResponseBytes: len(readmeResponse),
		Metrics: llm.Metrics{Attempts: 1},
	}); err != nil {
		t.Fatal(err)
	}
	portfolioRequest := []byte(`{"model":"fixture","stage":"target_portfolio"}`)
	portfolioResponse := []byte(`{"default_file_ref":"f1","target_file_refs":["f1"]}`)
	outcome := targetPortfolioRunOutcome{
		Request: portfolioRequest, RequestBytes: len(portfolioRequest),
		Response: portfolioResponse, ResponseBytes: len(portfolioResponse),
		RequestProvenance: debugdump.SemanticRequestExactSent,
		SemanticState:     debugdump.SemanticStateAccepted,
		ValidationCode:    debugdump.SemanticValidationAccepted,
		SemanticCalls:     1, TransportAttempts: 1,
	}
	var console bytes.Buffer
	output := newRunOutput(&console)

	// The default child page owns and writes the first-layer journal before
	// the outer dispatcher learns that publication failed elsewhere.
	flushFirstLayerSemanticJournal(runDir, observer, output)
	if err := recordTargetPortfolioOutcome(runDir, outcome, output); err != nil {
		t.Fatalf("child target portfolio journal: %v", err)
	}
	// The outer dispatch error retries the same lifecycle flush. This must be
	// a no-op for both the drained observer and the identical portfolio row.
	flushFailedFirstLayerSemanticJournal(runDir, observer, output)
	if err := recordTargetPortfolioOutcome(runDir, outcome, output); err != nil {
		t.Fatalf("outer failure target portfolio journal: %v", err)
	}
	if strings.Contains(console.String(), debugdump.SemanticExchangeWarningCode) {
		t.Fatalf("identical outer journal flush emitted an artifact warning:\n%s", console.String())
	}

	entries, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	stages := make(map[string]int, len(entries))
	var portfolioRecord debugdump.SemanticExchangeRecord
	for _, entry := range entries {
		encoded, err := os.ReadFile(filepath.Join(
			runDir, debugdump.SemanticExchangesDir, entry.Name(), debugdump.SemanticExchangeMetaFile,
		))
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(encoded, &record); err != nil {
			t.Fatal(err)
		}
		stages[record.Stage]++
		if record.Stage == debugdump.SemanticStageTargetPortfolio {
			portfolioRecord = record
		}
	}
	if len(entries) != 2 || stages[debugdump.SemanticStageReadmeFileClassifier] != 1 ||
		stages[debugdump.SemanticStageTargetPortfolio] != 1 {
		t.Fatalf("first-layer semantic journal stages = %#v", stages)
	}
	if portfolioRecord.RequestSHA256 != targetPortfolioPayloadSHA256(portfolioRequest) ||
		portfolioRecord.Response.OriginalSHA256 != targetPortfolioPayloadSHA256(portfolioResponse) {
		t.Fatalf("persisted target portfolio exchange = %#v", portfolioRecord)
	}

	conflicting := outcome
	conflicting.Response = []byte(`{"default_file_ref":"f2","target_file_refs":["f2"]}`)
	conflicting.ResponseBytes = len(conflicting.Response)
	if err := recordTargetPortfolioOutcome(runDir, conflicting, output); err == nil ||
		!strings.Contains(err.Error(), "conflicting semantic exchange") {
		t.Fatalf("conflicting target portfolio exchange error = %v", err)
	}
	entries, err = os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil || len(entries) != 2 {
		t.Fatalf("conflicting retry changed semantic journal: entries=%d err=%v", len(entries), err)
	}
}

func TestFailedFirstLayerSemanticJournalCreatesOnlyTheFailedRunDirectory(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "failed-first-layer")
	observer := debugdump.NewSemanticObserver(nil)
	request := []byte(`{"model":"test"}`)
	response := []byte(`[]`)
	if err := observer.ObserveStage(debugdump.SemanticStageReadmeFileClassifier, llm.Event{
		Kind: llm.EventFailure, Source: llm.SourceLive, Failure: llm.FailureValidation,
		Request: request, RequestBytes: len(request), Response: response, ResponseBytes: len(response),
		FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1},
	}); err != nil {
		t.Fatal(err)
	}

	flushFailedFirstLayerSemanticJournal(runDir, observer, nil)
	if info, err := os.Lstat(runDir); err != nil || !info.IsDir() {
		t.Fatalf("failed run directory = %#v, %v", info, err)
	}
	entries, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("semantic exchange entries = %d, want 1", len(entries))
	}
	if _, err := os.Stat(filepath.Join(runDir, "metadata.json")); !os.IsNotExist(err) {
		t.Fatalf("failed first layer unexpectedly published metadata: %v", err)
	}
}

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
