package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/sourceexplain"
)

type failingSourceExplainer struct {
	raw []byte
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
