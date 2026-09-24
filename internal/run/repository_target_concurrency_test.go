package run

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/reportserver"
)

// The documentation reduction runs beside the first target's native
// analysis. Its refusal is still the run's reported cause, and no page is
// published without the reduced documentation every page is built from.
func TestDocumentationReductionFailureStopsTheRunBesideNativeAnalysis(t *testing.T) {
	repositoryRoot := ordinaryGraphGoRepository(t)
	debugDir := t.TempDir()
	refused := errors.New("documentation preset refused")
	err := runDefaultWithDeps(repositoryRoot, []string{
		"--no-model", "--target", "example.com/common-page@.::example.com/common-page/cmd/app",
		"--no-open", "--debug-dir", debugDir,
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: io.Discard,
		llmBatchConcurrency: 1, llmBatchController: &llm.BatchController{},
		serveReport: func(context.Context, reportserver.Options) error {
			t.Fatal("a run without its documentation served a report")
			return nil
		},
		openReport: func(string) error { return nil },
		runDocumentationReduce: func(
			context.Context, llm.Executor, llm.Provider, readmetargetscout.GuidanceSnapshot,
		) (documentationreduce.Result, error) {
			return documentationreduce.Result{}, refused
		},
	})
	if !errors.Is(err, refused) {
		t.Fatalf("run error = %v, want the documentation refusal", err)
	}
	if published := ordinaryGraphRunDirs(t, debugDir); len(published) != 0 {
		t.Fatalf("published run directories without documentation: %v", published)
	}
}
