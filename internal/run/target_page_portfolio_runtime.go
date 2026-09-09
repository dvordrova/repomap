package run

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/modeldiag"
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
func reportAnalyzedTargetPagePublicationFailure(output *runOutput, targets []targetPageConsoleContext, runDir string, runErr error) {
	if output == nil {
		return
	}
	for _, target := range targets {
		output.TargetPage("analyzed", target)
	}
	details := []string{fmt.Sprintf("analyzed target pages: %d", len(targets))}
	if runErr != nil {
		details = append(details, "reason: "+runErr.Error())
	}
	details = append(details, "final report was not published")
	if runDir != "" {
		for _, artifact := range []struct{ label, name string }{
			{"artifacts", "."},
			{"rejections", modeldiag.Filename},
			{"model request/response journals", debugdump.SemanticExchangesDir},
		} {
			path := filepath.Join(runDir, artifact.name)
			if _, err := os.Stat(path); err == nil {
				details = append(details, artifact.label+": "+path)
			}
		}
	}
	output.State("Report publication", "failed", details...)
}

// Deferred portfolio stages read one saved index at a time. Do not attach it
// back to the run: that would retain every child's index until publication.
func (run *targetPublishedRun) programIndex() (programindex.Index, error) {
	if run.ProgramIndex == nil {
		return readRunProgramIndex(run.RunDir)
	}
	return *run.ProgramIndex, nil
}

func (run *targetPublishedRun) programTarget() programindex.Target {
	if run.ProgramIndex != nil {
		return run.ProgramIndex.Target
	}
	return run.ProgramPage.ProgramTarget
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
