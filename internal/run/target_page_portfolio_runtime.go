package run

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
)

// targetPublishedRun is the transaction-local handoff from one exact language
// adapter page to the repository dispatcher. ProgramPagePortfolio and
// TargetOutcomePortfolio retain orchestration identity; GroupIndex is the
// page's semantic graph and is replaced by its matched snapshot before the
// final shared graph is bound.
type targetPublishedRun struct {
	RunID         string
	RunDir        string
	ProgramPage   report.TargetNavigationPage
	GroupIndex    groupindex.Index
	ProgramIndex  *programindex.Index
	Dependencies  *dependencies.Catalog
	Documentation *documentationreduce.Result
	RepoName      string
	Timing        debugdump.RunTiming

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

// Restored stage inputs are loaded once. Ordinary runs already own these values.
func (run *targetPublishedRun) programIndex() (programindex.Index, error) {
	if run.ProgramIndex == nil {
		index, err := readRunProgramIndex(run.RunDir)
		if err != nil {
			return programindex.Index{}, err
		}
		run.ProgramIndex = &index
	}
	return *run.ProgramIndex, nil
}

func (run *targetPublishedRun) dependencyCatalog() (*dependencies.Catalog, error) {
	if run.Dependencies == nil {
		catalog, err := readRunDependencyCatalog(run.RunDir)
		if err != nil {
			return nil, err
		}
		run.Dependencies = catalog
	}
	return run.Dependencies, nil
}
