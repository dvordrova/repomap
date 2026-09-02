package debugdump

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestSemanticObserverPersistsRawInvalidResponse(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "cube-invalid-json", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	request := []byte(`{"model":"fixture","messages":[]}`)
	response := []byte("leading words {not-json")
	observer := NewSemanticObserver(writer)
	if err := observer.ObserveStage(SemanticStageProgramGrouping, llm.Event{
		Kind: llm.EventFailure, Source: llm.SourceLive, Failure: llm.FailureValidation,
		Request: request, RequestBytes: len(request),
		Response: response, ResponseBytes: len(response),
		Metrics: llm.Metrics{Attempts: 1},
	}); err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	directory, record := onlyObserverRecord(t, runDir)
	if record.Stage != SemanticStageProgramGrouping ||
		record.State != SemanticStateRejected ||
		record.ValidationCode != SemanticValidationDecode ||
		record.Response.Storage != "raw_content" || record.Response.File != "response.txt" {
		t.Fatalf("semantic exchange record = %#v", record)
	}
	saved, err := os.ReadFile(filepath.Join(directory, record.Response.File))
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != string(response) {
		t.Fatalf("saved response = %q, want %q", saved, response)
	}
}

func TestBoundSemanticObserverFlushesAcceptedExchange(t *testing.T) {
	t.Parallel()

	base := NewSemanticObserver(nil)
	executor := BindStage(
		llm.Executor{Observer: base},
		SemanticStageReadmeFileClassifier,
	)
	request := []byte(`{"model":"fixture","messages":[]}`)
	response := []byte(`[]`)
	if err := executor.Observer.Observe(llm.Event{
		Kind: llm.EventLive, Source: llm.SourceLive,
		Request: request, Response: response,
		Metrics: llm.Metrics{Attempts: 1},
	}); err != nil {
		t.Fatal(err)
	}
	writer, err := NewWriter(t.TempDir(), "readme-classifier", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	base.Flush(writer)
	_, record := onlyObserverRecord(t, filepath.Join(writer.BaseDir, writer.RunID))
	if record.Stage != SemanticStageReadmeFileClassifier ||
		record.State != SemanticStateAccepted {
		t.Fatalf("README semantic exchange record = %#v", record)
	}
}

func TestSemanticObserverRetainsExchangeBeyondFormerOrdinalThreshold(t *testing.T) {
	request := []byte(`{"model":"fixture","messages":[]}`)
	response := []byte(`[]`)
	exchange, retained := semanticExchangeForStageEvent(
		SemanticStageTargetPortfolio,
		MaxSemanticAttemptOrdinal+1,
		llm.Event{
			Kind: llm.EventLive, Source: llm.SourceLive,
			Request: request, Response: response, Metrics: llm.Metrics{Attempts: 1},
		},
	)
	if !retained || exchange.SemanticAttemptOrdinal != MaxSemanticAttemptOrdinal+1 {
		t.Fatalf("exchange beyond former ordinal threshold = %#v, retained %t", exchange, retained)
	}
}

func TestSemanticObserverReportsFormerOrdinalThresholdsAsAggregateWarnings(t *testing.T) {
	observer := NewSemanticObserver(nil)
	event := llm.Event{
		Kind: llm.EventLive, Source: llm.SourceLive,
		Request: []byte(`{"model":"fixture","messages":[]}`), Response: []byte(`[]`),
		Metrics: llm.Metrics{Attempts: 1},
	}
	for ordinal := 0; ordinal < MaxSemanticExchangeInstanceOrdinal+1; ordinal++ {
		if err := observer.ObserveStage(SemanticStageTargetPortfolio, event); err != nil {
			t.Fatal(err)
		}
	}
	want := []SemanticOrdinalScaleWarning{
		{
			Kind:         SemanticScaleWarningAttemptOrdinal,
			Retained:     MaxSemanticExchangeInstanceOrdinal + 1,
			AdvisorySize: MaxSemanticAttemptOrdinal,
		},
		{
			Kind:         SemanticScaleWarningInstanceOrdinal,
			Retained:     MaxSemanticExchangeInstanceOrdinal + 1,
			AdvisorySize: MaxSemanticExchangeInstanceOrdinal,
		},
	}
	if got := observer.OrdinalScaleWarnings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("OrdinalScaleWarnings = %#v, want %#v", got, want)
	}
	if got := observer.pending[len(observer.pending)-1]; got.InstanceOrdinal != MaxSemanticExchangeInstanceOrdinal+1 ||
		got.SemanticAttemptOrdinal != MaxSemanticExchangeInstanceOrdinal+1 {
		t.Fatalf("last retained exchange ordinals = %#v", got)
	}

	redacted := NewSemanticObserver(nil)
	event.RequestRedacted = true
	if err := redacted.ObserveStage(SemanticStageTargetPortfolio, event); err != nil {
		t.Fatal(err)
	}
	if len(redacted.OrdinalScaleWarnings()) != 0 || redacted.instanceOrdinal != 0 {
		t.Fatalf("redacted event entered ordinal accounting: %#v", redacted)
	}
}

func TestSemanticObserverOmitsRedactedAuthorizationRequest(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "redacted-request", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	observer := NewSemanticObserver(writer)
	request := []byte(`{"message":"Authorization: Bearer must-not-persist"}`)
	if err := observer.ObserveStage(SemanticStageReadmeFileClassifier, llm.Event{
		Kind: llm.EventLive, Source: llm.SourceLive,
		Request: request, RequestBytes: len(request), RequestRedacted: true,
		Response: []byte(`[]`), Metrics: llm.Metrics{Attempts: 1},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = os.Lstat(filepath.Join(writer.BaseDir, writer.RunID, SemanticExchangesDir))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("redacted request created a semantic journal: %v", err)
	}
}

func TestSemanticObserverMarksRedactedResponseUnavailable(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "redacted-response", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	observer := NewSemanticObserver(writer)
	request := []byte(`{"model":"fixture","messages":[]}`)
	if err := observer.ObserveStage(SemanticStageProgramGrouping, llm.Event{
		Kind: llm.EventFailure, Source: llm.SourceLive, Failure: llm.FailureResponse,
		Request: request, RequestBytes: len(request),
		ResponseRedacted: true, ResponseSHA256: strings.Repeat("a", 64), ResponseBytes: 48,
		Metrics: llm.Metrics{Attempts: 1},
	}); err != nil {
		t.Fatal(err)
	}
	_, record := onlyObserverRecord(t, filepath.Join(writer.BaseDir, writer.RunID))
	if record.State != SemanticStateRejected ||
		record.ValidationCode != SemanticValidationSecret ||
		record.Response.Storage != "raw_unavailable" ||
		record.Response.UnavailableCode != SemanticUnavailableOmitted ||
		record.Response.OriginalBytes != 48 {
		t.Fatalf("redacted response record = %#v", record)
	}
}

func onlyObserverRecord(t *testing.T, runDir string) (string, SemanticExchangeRecord) {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, SemanticExchangesDir))
	if err != nil || len(directories) != 1 {
		t.Fatalf("semantic exchange directories = %d, err = %v", len(directories), err)
	}
	directory := filepath.Join(runDir, SemanticExchangesDir, directories[0].Name())
	metadata, err := os.ReadFile(filepath.Join(directory, SemanticExchangeMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var record SemanticExchangeRecord
	if err := json.Unmarshal(metadata, &record); err != nil {
		t.Fatal(err)
	}
	return directory, record
}
