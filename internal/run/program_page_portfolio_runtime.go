package run

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// preparePublishedTargetAuthority retains the exact ProgramTarget page for
// both publication modes. The outer language-neutral dispatcher uses this
// identity to prove that the completed run matches the selected adapter
// target; repository semantic authority stays in the final GroupsIndex set.
func preparePublishedTargetAuthority(
	preparePage func() (report.TargetNavigationPage, error),
) (report.TargetNavigationPage, error) {
	if preparePage == nil {
		return report.TargetNavigationPage{},
			fmt.Errorf("retain prepared report page identity: page projector is missing")
	}
	page, err := preparePage()
	if err != nil {
		return report.TargetNavigationPage{},
			fmt.Errorf("retain prepared report page identity: %w", err)
	}
	return page, nil
}

// buildProgramPagePortfolio restores the public language-neutral identity of
// every completed target page from its exact ProgramPortfolio/ArtifactSet.
// Adapter refs remain orchestration-only; the durable cross-page contract is
// keyed by the sealed ProgramTarget each page actually analyzed.
func buildProgramPagePortfolio(
	runs []targetPublishedRun,
	defaultRunID string,
) (programpage.Portfolio, error) {
	if len(runs) == 0 {
		return programpage.Portfolio{}, fmt.Errorf("program page portfolio: completed run set is empty")
	}
	pages := make([]programpage.Page, 0, len(runs))
	defaultTargetID := ""
	seenRunIDs := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		if run.RunID == "" || run.RunDir == "" || filepath.Base(run.RunDir) != run.RunID {
			return programpage.Portfolio{}, fmt.Errorf("program page portfolio: completed run identity is invalid")
		}
		if _, duplicate := seenRunIDs[run.RunID]; duplicate {
			return programpage.Portfolio{}, fmt.Errorf("program page portfolio: duplicate completed run")
		}
		seenRunIDs[run.RunID] = struct{}{}
		page := run.ProgramPage
		if page.RunID != run.RunID {
			return programpage.Portfolio{}, fmt.Errorf("program page portfolio: completed page identity is invalid")
		}
		pages = append(pages, programpage.Page{Target: page.ProgramTarget, RunID: run.RunID})
		if run.RunID == defaultRunID {
			defaultTargetID = page.ProgramTarget.ID
		}
	}
	if defaultTargetID == "" {
		return programpage.Portfolio{}, fmt.Errorf("program page portfolio: default completed run is absent")
	}
	return programpage.Build(defaultTargetID, pages)
}

func persistProgramPagePortfolioForRuns(
	portfolio programpage.Portfolio,
	runs []targetPublishedRun,
) error {
	encoded, err := portfolio.CanonicalJSON()
	if err != nil {
		return err
	}
	for _, run := range runs {
		writer, writerErr := debugdump.OpenWriter(run.RunDir)
		if writerErr != nil {
			return fmt.Errorf("program page portfolio: open run %s: %w", run.RunID, writerErr)
		}
		writeErr := writer.WriteValidatedFile(
			programpage.ArtifactFilename,
			encoded,
			func(saved []byte) error {
				if !bytes.Equal(saved, encoded) {
					return fmt.Errorf("program page portfolio: persisted bytes changed")
				}
				decoded, decodeErr := programpage.Decode(saved)
				if decodeErr != nil {
					return decodeErr
				}
				if decoded.SHA256 != portfolio.SHA256 {
					return fmt.Errorf("program page portfolio: persisted authority changed")
				}
				return nil
			},
		)
		closeErr := writer.Close()
		if writeErr != nil {
			return fmt.Errorf("program page portfolio: persist run %s: %w", run.RunID, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("program page portfolio: close run %s: %w", run.RunID, closeErr)
		}
	}
	return nil
}

// publishRepositoryReport projects shared results once and publishes from memory.
// Target directories keep their own analysis artifacts, never repository reports.
func publishRepositoryReport(
	ctx context.Context,
	portfolio programpage.Portfolio,
	targetOutcomes targetoutcome.Portfolio,
	runs []targetPublishedRun,
	outcome atlasOutcome,
	output *runOutput,
) (report.RunReceipt, error) {
	if err := ctx.Err(); err != nil {
		return report.RunReceipt{}, err
	}
	if err := portfolio.Validate(); err != nil {
		return report.RunReceipt{}, err
	}
	if len(runs) == 0 || len(runs) != len(portfolio.Pages) {
		return report.RunReceipt{}, fmt.Errorf("repository report: completed target coverage is incomplete")
	}
	owner := &runs[0]
	if owner.ProgramPage.ProgramTarget.ID != portfolio.DefaultTargetID {
		return report.RunReceipt{}, fmt.Errorf("repository report: owner is not the default target")
	}
	inventory, err := report.NewTargetOutcomePortfolioView(targetOutcomes, portfolio)
	if err != nil {
		return report.RunReceipt{}, err
	}
	if err := persistProgramPagePortfolioForRuns(portfolio, runs[:1]); err != nil {
		return report.RunReceipt{}, err
	}
	if err := persistTargetOutcomePortfolioForRuns(targetOutcomes, runs[:1]); err != nil {
		return report.RunReceipt{}, err
	}
	groupIndexes := make([]groupindex.Index, len(runs))
	for position, run := range runs {
		if run.GroupIndex.Target.ID != run.ProgramPage.ProgramTarget.ID {
			return report.RunReceipt{}, fmt.Errorf("repository report: run %s graph target mismatch", run.RunID)
		}
		groupIndexes[position] = run.GroupIndex
	}
	index, err := owner.programIndex()
	if err != nil {
		return report.RunReceipt{}, err
	}
	if owner.Documentation == nil {
		return report.RunReceipt{}, fmt.Errorf("repository report: documentation is missing from memory")
	}
	data, err := report.NewData(owner.RunDir, owner.RepoName, index, *owner.Documentation)
	if err != nil {
		return report.RunReceipt{}, err
	}
	if err := report.BindGroupGraphView(data, groupIndexes); err != nil {
		return report.RunReceipt{}, err
	}
	data.TargetOutcomePortfolio = inventory
	data.Facts, data.Claims, data.Orientation = &outcome.Facts, &outcome.Claims, outcome.Orientation
	data.Questions = outcome.Questions
	data.Learning = outcome.Learning
	timing := wholeRunTiming(output, runs)
	data.Timing = &report.RunTiming{WallMS: timing.WallMS}
	for _, stage := range timing.Stages {
		data.Timing.Stages = append(data.Timing.Stages, report.StageTiming{
			Stage: stage.Stage, Live: stage.Live, Cached: stage.Cached,
			ProviderMS: stage.ProviderMS, SlowestMS: stage.SlowestMS,
		})
	}
	if err := writeRunTiming(owner.RunDir, timing); err != nil {
		return report.RunReceipt{}, err
	}
	return report.Generate(owner.RunDir, owner.Source, report.GenerateOptions{
		Data: data, GitLabURL: owner.GitLabURL, GitHubURL: owner.GitHubURL, PublishHTML: true,
	})
}
