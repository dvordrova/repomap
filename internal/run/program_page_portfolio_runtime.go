package run

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
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

// orientAndPublishRepositoryReport asks the orientation over the projected
// groups while the report is assembled from the targets, groups, facts and
// claims; only the glossary, the translation and the publication wait for
// the orientation. An orientation failure is reported before any assembly
// failure, and nothing of the report is written before the orientation is
// accepted.
func orientAndPublishRepositoryReport(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	runs []targetPublishedRun,
	outcome *atlasOutcome,
	targetOutcomes targetoutcome.Portfolio,
) (report.RunReceipt, error) {
	// The assembly owns a copy: the orientation later records its result and
	// drops the graph in outcome itself.
	unoriented := *outcome
	assembly := startBackground(func() (repositoryReportAssembly, error) {
		return assembleRepositoryReport(ctx, targetOutcomes, runs, unoriented)
	})
	if !options.NoModel {
		if err := orientAtlasRuns(ctx, options, runs, outcome); err != nil {
			assembly.wait()
			return report.RunReceipt{}, err
		}
	}
	assembled, err := assembly.wait()
	if err != nil {
		return report.RunReceipt{}, err
	}
	options.Output.Stage("Report publication", "assembling one repository report from memory")
	started := time.Now()
	receipt, err := publishRepositoryReport(ctx, assembled, outcome.Orientation, options)
	if err != nil {
		return report.RunReceipt{}, err
	}
	options.Output.Wall(publicationStage, time.Since(started))
	options.Output.State("Report publication", "ready", formatRunOutputWallDuration(time.Since(started)))
	return receipt, nil
}

// repositoryReportAssembly is the repository report before its orientation:
// every target's ProgramIndex and projected groups bound into one ReportData
// with the facts, claims and learning plan. Nothing in it reads the
// orientation, so it is assembled while the orientation is asked.
type repositoryReportAssembly struct {
	portfolio      programpage.Portfolio
	targetOutcomes targetoutcome.Portfolio
	runs           []targetPublishedRun
	data           *report.ReportData
	programIndexes []programindex.Index
}

// assembleRepositoryReport projects shared results once, in memory, without
// writing anything: a run whose orientation fails keeps the artifacts it
// kept before.
func assembleRepositoryReport(
	ctx context.Context,
	targetOutcomes targetoutcome.Portfolio,
	runs []targetPublishedRun,
	outcome atlasOutcome,
) (repositoryReportAssembly, error) {
	if err := ctx.Err(); err != nil {
		return repositoryReportAssembly{}, err
	}
	if len(runs) == 0 {
		return repositoryReportAssembly{}, fmt.Errorf("repository report: completed target coverage is incomplete")
	}
	portfolio, err := buildProgramPagePortfolio(runs, runs[0].RunID)
	if err != nil {
		return repositoryReportAssembly{}, err
	}
	if err := portfolio.Validate(); err != nil {
		return repositoryReportAssembly{}, err
	}
	if len(runs) != len(portfolio.Pages) {
		return repositoryReportAssembly{}, fmt.Errorf("repository report: completed target coverage is incomplete")
	}
	owner := &runs[0]
	if owner.ProgramPage.ProgramTarget.ID != portfolio.DefaultTargetID {
		return repositoryReportAssembly{}, fmt.Errorf("repository report: owner is not the default target")
	}
	inventory, err := report.NewTargetOutcomePortfolioView(targetOutcomes, portfolio)
	if err != nil {
		return repositoryReportAssembly{}, err
	}
	groupIndexes := make([]groupindex.Index, len(runs))
	programIndexes := make([]programindex.Index, len(runs))
	for position, run := range runs {
		if run.GroupIndex.Target.ID != run.ProgramPage.ProgramTarget.ID {
			return repositoryReportAssembly{}, fmt.Errorf("repository report: run %s graph target mismatch", run.RunID)
		}
		groupIndexes[position] = run.GroupIndex
		programIndex, programErr := run.programIndex()
		if programErr != nil {
			return repositoryReportAssembly{}, programErr
		}
		programIndexes[position] = programIndex
	}
	index := programIndexes[0]
	if owner.Documentation == nil {
		return repositoryReportAssembly{}, fmt.Errorf("repository report: documentation is missing from memory")
	}
	data, err := report.NewData(owner.RunDir, owner.RepoName, index, *owner.Documentation)
	if err != nil {
		return repositoryReportAssembly{}, err
	}
	if err := report.BindProgramPortfolio(data, portfolio.DefaultTargetID, programIndexes); err != nil {
		return repositoryReportAssembly{}, err
	}
	if err := report.BindGroupGraphView(data, groupIndexes); err != nil {
		return repositoryReportAssembly{}, err
	}
	data.TargetOutcomePortfolio = inventory
	data.Facts, data.Claims = &outcome.Facts, &outcome.Claims
	data.Questions = outcome.Questions
	data.Learning = outcome.Learning
	data.CapturedRevision = owner.Source.Repository.Head
	return repositoryReportAssembly{
		portfolio: portfolio, targetOutcomes: targetOutcomes, runs: runs,
		data: data, programIndexes: programIndexes,
	}, nil
}

// publishRepositoryReport completes the assembled report with its
// orientation, glossary and display translation and publishes it from
// memory. Target directories keep their own analysis artifacts, never
// repository reports.
func publishRepositoryReport(
	ctx context.Context,
	assembled repositoryReportAssembly,
	oriented *orientation.Result,
	options repositoryTargetDispatchOptions,
) (report.RunReceipt, error) {
	output := options.Output
	if err := ctx.Err(); err != nil {
		return report.RunReceipt{}, err
	}
	runs, data, programIndexes := assembled.runs, assembled.data, assembled.programIndexes
	owner := &runs[0]
	if err := persistProgramPagePortfolioForRuns(assembled.portfolio, runs[:1]); err != nil {
		return report.RunReceipt{}, err
	}
	if err := persistTargetOutcomePortfolioForRuns(assembled.targetOutcomes, runs[:1]); err != nil {
		return report.RunReceipt{}, err
	}
	data.Orientation = oriented
	if err := reduceReportGlossary(ctx, options, owner.RunDir, data, programIndexes); err != nil {
		return report.RunReceipt{}, err
	}
	renderOptions, err := translateReportDisplay(ctx, options, owner.RunDir, data)
	if err != nil {
		return report.RunReceipt{}, err
	}
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
	generateOptions := report.GenerateOptions{
		Data: data, GitLabURL: owner.GitLabURL, GitHubURL: owner.GitHubURL, PublishHTML: true, Render: renderOptions,
	}
	if !options.NoServe && options.Deps.serveReport != nil {
		// The page the local server will serve is rendered beside report.html,
		// so serving does not render it again after publication.
		generateOptions.ServedSourceID = reportserver.SourceID
	}
	return report.Generate(owner.RunDir, owner.Source, generateOptions)
}
