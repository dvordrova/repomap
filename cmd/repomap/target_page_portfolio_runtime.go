package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	Receipt     report.VerifiedRunReceipt

	ReportScaleWarnings []report.ReportInputScaleWarning

	SelectedTargetKey     string
	SelectedTargetDisplay string
	Authority             report.RunAuthority
	RepositoryStateSHA256 string
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
func quarantineTargetPagePublication(runDirs []string) error {
	seen := make(map[string]struct{}, len(runDirs))
	var result error
	for _, runDir := range runDirs {
		if runDir == "" {
			continue
		}
		clean := filepath.Clean(runDir)
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		if err := report.RemoveRunManifest(clean); err != nil {
			result = errors.Join(result, err)
		}
		for _, name := range []string{"report.html", "report.json"} {
			from := filepath.Join(clean, name)
			to := filepath.Join(clean, name+".failed")
			if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
				result = errors.Join(result, fmt.Errorf("quarantine %s: %w", from, err))
			}
		}
	}
	return result
}

func (run targetPublishedRun) generateBackingPageData() (
	report.VerifiedRunReceipt,
	report.GenerationDiagnostics,
	error,
) {
	switch {
	case run.GitLabURL != "":
		return report.GenerateAuthorizedGitLabPageDataVerifiedWithDiagnostics(
			run.RunDir, run.Authority, run.GitLabURL,
		)
	case run.GitHubURL != "":
		return report.GenerateAuthorizedGitHubPageDataVerifiedWithDiagnostics(
			run.RunDir, run.Authority, run.GitHubURL,
		)
	default:
		return report.GenerateAuthorizedPageDataVerifiedWithDiagnostics(run.RunDir, run.Authority)
	}
}
