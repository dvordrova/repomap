package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/sourceexplain"
)

type failingSourceExplainer struct {
	raw []byte
}

func TestExperimentMetadataRecordsSourceTiming(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.July, 10, 12, 34, 56, 0, time.FixedZone("offset", 4*60*60))
	finished := started.Add(1250 * time.Millisecond)
	var metadata experimentMetadata
	metadata.recordSourceTiming(started, finished)

	if metadata.SourceCapturedAt != "2026-07-10T08:34:56Z" {
		t.Fatalf("source captured at = %q", metadata.SourceCapturedAt)
	}
	if metadata.SourceLatencyMillis == nil || *metadata.SourceLatencyMillis != 1250 {
		t.Fatalf("source latency = %v", metadata.SourceLatencyMillis)
	}
}

func (f failingSourceExplainer) Explain(context.Context, sourceexplain.Bundle) (sourceexplain.Explanation, error) {
	return sourceexplain.Explanation{Raw: append([]byte{}, f.raw...)}, errors.New("malformed response")
}

func TestRunSourceExplanationPersistsRawResponseOnParseFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := []byte(`{"assessments":`)
	_, err := runSourceExplanation(context.Background(), failingSourceExplainer{raw: raw}, sourceexplain.Bundle{}, dir)
	if err == nil {
		t.Fatal("runSourceExplanation() error = nil")
	}
	written, readErr := os.ReadFile(filepath.Join(dir, "deepseek_source_response.raw.txt"))
	if readErr != nil {
		t.Fatalf("read raw response: %v", readErr)
	}
	want := append(append([]byte{}, raw...), '\n')
	if string(written) != string(want) {
		t.Fatalf("raw artifact = %q, want %q", written, want)
	}
}
