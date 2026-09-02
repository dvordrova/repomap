package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"

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
		writer, writerErr := debugdump.OpenWriter(run.RunDir, true)
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

// finalizeProgramPageRuns installs the one complete matched GroupsIndex set
// into every successful backing page. No repository-level semantic authority
// is rebuilt here: ProgramPagePortfolio and TargetOutcomePortfolio retain
// orchestration identity, while GroupsIndex is the sole repository graph.
func finalizeProgramPageRuns(
	ctx context.Context,
	portfolio programpage.Portfolio,
	targetOutcomes targetoutcome.Portfolio,
	runs []targetPublishedRun,
	output *runOutput,
) error {
	if err := portfolio.Validate(); err != nil {
		return err
	}
	if len(runs) != len(portfolio.Pages) {
		return fmt.Errorf("program page portfolio: completed run coverage is incomplete")
	}
	if _, err := report.NewTargetOutcomePortfolioView(targetOutcomes, portfolio); err != nil {
		return err
	}
	if err := persistProgramPagePortfolioForRuns(portfolio, runs); err != nil {
		return err
	}

	groupIndexes := make([]groupindex.Index, len(runs))
	for position, run := range runs {
		if err := run.GroupIndex.Validate(); err != nil {
			return fmt.Errorf("program page portfolio: run %s GroupsIndex: %w", run.RunID, err)
		}
		if run.GroupIndex.Target.ID != run.ProgramPage.ProgramTarget.ID {
			return fmt.Errorf("program page portfolio: run %s graph target mismatch", run.RunID)
		}
		groupIndexes[position] = run.GroupIndex.Snapshot()
	}
	groupGraph, err := report.NewGroupGraphView(groupIndexes, portfolio.DefaultTargetID)
	if err != nil {
		return fmt.Errorf("program page portfolio: validate matched group graph: %w", err)
	}
	groupGraphPaths, err := groupGraph.SourcePaths()
	if err != nil {
		return fmt.Errorf("program page portfolio: collect group graph sources: %w", err)
	}
	for index := range runs {
		reportCurrentCapturedInputScaleWarnings(output, &runs[index])
		extended, err := report.ExtendRunAuthority(ctx, runs[index].Authority, groupGraphPaths)
		if err != nil {
			return fmt.Errorf("program page portfolio: authorize graph sources for run %s: %w", runs[index].RunID, err)
		}
		bound, err := report.BindRunAuthorityGroupGraph(extended, groupIndexes)
		if err != nil {
			return fmt.Errorf("program page portfolio: bind group graph for run %s: %w", runs[index].RunID, err)
		}
		runs[index].Authority = bound
	}

	runIndexByID := make(map[string]int, len(runs))
	for index, run := range runs {
		if _, duplicate := runIndexByID[run.RunID]; duplicate {
			return fmt.Errorf("program page portfolio: duplicate completed run")
		}
		runIndexByID[run.RunID] = index
	}
	for _, binding := range portfolio.Pages {
		runIndex, found := runIndexByID[binding.RunID]
		if !found {
			return fmt.Errorf("program page portfolio: completed run is missing")
		}
		run := &runs[runIndex]
		page := run.ProgramPage
		if page.RunID != run.RunID {
			return fmt.Errorf("program page portfolio: completed page identity is invalid")
		}
		if !reflect.DeepEqual(page.ProgramTarget, binding.Target) {
			return fmt.Errorf("program page portfolio: completed page target mismatch")
		}
		receipt, diagnostics, err := run.generateBackingPageData()
		newWarnings := excludeReportScaleWarnings(
			diagnostics.ScaleWarnings(),
			reportScaleWarningKeySet(run.ReportScaleWarnings),
		)
		reportInputScaleWarnings(output, newWarnings, run.ProgramPage.ProgramTarget)
		run.ReportScaleWarnings = append(run.ReportScaleWarnings, newWarnings...)
		if err != nil {
			return fmt.Errorf("program page portfolio: finalize backing run %s: %w", run.RunID, err)
		}
		if err := receipt.ValidateRunIdentity(run.RunDir); err != nil {
			return fmt.Errorf("program page portfolio: verify run receipt %s: %w", run.RunID, err)
		}
		if receiptPage := receipt.ProgramPage(); !reflect.DeepEqual(receiptPage, page) {
			return fmt.Errorf("program page portfolio: run %s receipt page mismatch", run.RunID)
		}
		manifest := receipt.Manifest()
		if manifest.RepositoryStateSHA256 != run.RepositoryStateSHA256 ||
			manifest.MaterialInputs.SelectedRevision != run.SelectedRevision ||
			manifest.MaterialInputs.ProgramTargetID != binding.Target.ID ||
			manifest.MaterialInputs.ProgramPagePortfolioSHA256 == "" {
			return fmt.Errorf("program page portfolio: run %s authority mismatch", run.RunID)
		}
		run.Receipt = receipt
		reportCurrentTargetPageScaleWarnings(output, run)
	}
	return nil
}

func publishProgramPageBundle(
	portfolio programpage.Portfolio,
	runs []targetPublishedRun,
) (report.PublicationAssessment, error) {
	if err := portfolio.Validate(); err != nil {
		return report.FailedPublicationAssessment(), err
	}
	runsByID := make(map[string]targetPublishedRun, len(runs))
	for _, run := range runs {
		if run.RunID == "" || run.RunDir == "" || filepath.Base(run.RunDir) != run.RunID {
			return report.FailedPublicationAssessment(), fmt.Errorf("program page bundle: completed run identity is invalid")
		}
		if _, duplicate := runsByID[run.RunID]; duplicate {
			return report.FailedPublicationAssessment(), fmt.Errorf("program page bundle: duplicate completed run")
		}
		runsByID[run.RunID] = run
	}
	if len(runsByID) != len(portfolio.Pages) {
		return report.FailedPublicationAssessment(), fmt.Errorf("program page bundle: completed run coverage is incomplete")
	}
	defaultRunDir := ""
	receipts := make([]report.VerifiedRunReceipt, 0, len(portfolio.Pages))
	for _, page := range portfolio.Pages {
		run, found := runsByID[page.RunID]
		if !found {
			return report.FailedPublicationAssessment(), fmt.Errorf("program page bundle: portfolio run is missing")
		}
		if page.Target.ID == portfolio.DefaultTargetID {
			defaultRunDir = run.RunDir
		}
		receipts = append(receipts, run.Receipt)
	}
	if defaultRunDir == "" {
		return report.FailedPublicationAssessment(), fmt.Errorf("program page bundle: default run is missing")
	}
	assessment, err := report.PublishProgramPageBundleFromVerifiedRunsAtomic(
		defaultRunDir, portfolio, receipts,
	)
	if err != nil {
		return assessment, fmt.Errorf("program page bundle: publish: %w", err)
	}
	return assessment, nil
}
