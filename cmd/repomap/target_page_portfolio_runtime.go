package main

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/report"
)

// targetPublishedRun is the transaction-local handoff from one exact language
// adapter page to the repository dispatcher. ProgramPagePortfolio and
// TargetOutcomePortfolio retain orchestration identity; GroupIndex is the
// page's semantic graph and is replaced by its matched snapshot before the
// final shared graph is bound.
type targetPublishedRun struct {
	RunID       string
	RunDir      string
	ProgramPage report.TargetNavigationPage
	GroupIndex  groupindex.Index
	Receipt     report.RunReceipt

	SelectedTargetKey     string
	SelectedTargetDisplay string
	Source                report.RunSource
	SelectedRevision      string
	GitLabURL             string
	GitHubURL             string
}

// reportAnalyzedTargetPagePublicationFailure keeps target-local analysis state
// distinct from the shared publication boundary. These pages completed their
// validated pipelines; a later repository graph, persistence, manifest, or
// bundle failure prevents publication without turning them into target-local
// failures.
func reportAnalyzedTargetPagePublicationFailure(output *runOutput, targets []targetPageConsoleContext) {
	if output == nil {
		return
	}
	for _, target := range targets {
		output.TargetPage("analyzed", target)
	}
	output.State(
		"Report publication", "failed",
		fmt.Sprintf("analyzed target pages: %d", len(targets)),
		"final report was not published",
	)
}

// quarantineTargetPagePublication removes browser authority and gives any
// already-rendered files an explicit failed suffix. Raw analysis artifacts
// remain available for diagnostics, but no partial multi-target publication
// keeps a product-looking report.html/report.json pair.

func (run targetPublishedRun) generateBackingPageData() (report.RunReceipt, error) {
	return report.Generate(run.RunDir, run.Source, report.GenerateOptions{
		GitLabURL: run.GitLabURL, GitHubURL: run.GitHubURL, PublishHTML: false,
	})
}
