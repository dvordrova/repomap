package debugdump

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func failureNoticeEvent() llm.Event {
	return llm.Event{
		Kind: llm.EventFailure, Source: llm.SourceLive, Failure: llm.FailureValidation,
		Request: []byte(`{"request":true}`), Response: []byte(`{"wrong":true}`),
		Metrics:            llm.Metrics{Attempts: 1},
		HTTPResponse:       &llm.HTTPResponse{StatusCode: 200, Headers: map[string][]string{"X-Request-Id": {"original-id"}}},
		ResponseRejections: []llm.ResponseRejection{{Kind: "response_validation", Count: 1, Reason: "required row omitted"}},
	}
}

func TestFailureNoticeWaitsForJournalAndIgnoresAcceptedMetadata(t *testing.T) {
	observer := NewSemanticObserver(nil)
	var notices []SemanticFailureReceipt
	observer.SetFailureNotice(func(receipt SemanticFailureReceipt) { notices = append(notices, receipt) })
	event := failureNoticeEvent()
	if err := observer.ObserveStage(SemanticStageReportTranslation, event); err != nil || len(notices) != 0 {
		t.Fatalf("unwritten failure advertised payloads: %+v / %v", notices, err)
	}
	event.HTTPResponse.Headers["X-Request-Id"][0] = "changed-after-event"
	writer, err := NewWriter(t.TempDir(), "buffered")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	observer.Flush(writer)
	observer.Flush(writer)
	if len(notices) != 1 || notices[0].Reason != "required row omitted" {
		t.Fatalf("buffered failure receipt = %+v", notices)
	}
	if notices[0].HTTPResponse.Headers["X-Request-Id"][0] != "original-id" || notices[0].TransportAttempts != 1 {
		t.Fatal("buffered response diagnostics changed after the original event")
	}
	for _, path := range []string{notices[0].RequestPath, notices[0].ResponsePath, notices[0].JournalPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []llm.EventKind{llm.EventLive, llm.EventCacheHit} {
		event := failureNoticeEvent()
		event.Kind, event.Failure = kind, llm.FailureNone
		if kind == llm.EventCacheHit {
			event.Source = llm.SourceCache
		}
		event.ResponseRejections = []llm.ResponseRejection{{Kind: "terminology_metadata_rejected", Count: 1, Reason: "unused term"}}
		if err := observer.ObserveStage(SemanticStageReportTranslation, event); err != nil {
			t.Fatal(err)
		}
	}
	observer.Flush(writer)
	if len(notices) != 1 {
		t.Fatal("accepted main results with optional metadata rejection spammed failure notices")
	}
}

func TestFailureNoticeConcurrentReceiptsAndUnwrittenJournal(t *testing.T) {
	writer, err := NewWriter(t.TempDir(), "concurrent")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	observer := NewSemanticObserver(writer)
	var mu sync.Mutex
	paths := make(map[string]bool)
	observer.SetFailureNotice(func(receipt SemanticFailureReceipt) {
		mu.Lock()
		defer mu.Unlock()
		if paths[receipt.JournalPath] {
			t.Error("two failures shared one journal")
		}
		paths[receipt.JournalPath] = true
	})
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			if err := observer.ObserveStage(SemanticStageReportTranslation, failureNoticeEvent()); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if len(paths) != 4 {
		t.Fatalf("concurrent failures recorded %d receipts", len(paths))
	}
	var warnings bytes.Buffer
	writer.SetWarningWriter(&warnings)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_ = observer.ObserveStage(SemanticStageReportTranslation, failureNoticeEvent())
	if len(paths) != 4 || warnings.Len() == 0 {
		t.Fatal("failed journal publication advertised nonexistent payloads or hid its warning")
	}
}
