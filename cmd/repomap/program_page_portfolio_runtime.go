package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func repositoryTypedTargetDisplay(target repositoryTypedTarget) string {
	switch target.Key.Adapter {
	case repositoryTargetAdapterGo:
		if target.Go != nil {
			return target.Go.DisplayPath()
		}
	case repositoryTargetAdapterPython:
		if target.Python != nil {
			return target.Python.DisplayName
		}
	case repositoryTargetAdapterJSTS:
		if target.JSTS != nil {
			return target.JSTS.Name
		}
	}
	return target.Key.String()
}

func repositoryTypedTargetMatchesProgramTarget(
	target repositoryTypedTarget,
	programTarget programindex.Target,
) bool {
	if err := target.Validate(); err != nil || programTarget.Validate() != nil {
		return false
	}
	switch target.Key.Adapter {
	case repositoryTargetAdapterGo:
		if programTarget.Language != "go" {
			return false
		}
		name := target.Go.PackagePath
		kind := "executable"
		if target.Go.Kind == "module_library" {
			name = target.Go.ModulePath
			kind = "library"
		}
		return programTarget.Name == name && programTarget.Selector == name && programTarget.Kind == kind
	case repositoryTargetAdapterPython:
		return programTarget.Language == "python" &&
			programTarget.Selector == target.Python.Selector &&
			programTarget.Name == target.Python.DisplayName
	case repositoryTargetAdapterJSTS:
		return (programTarget.Language == "javascript" || programTarget.Language == "typescript") &&
			programTarget.Selector == target.JSTS.Selector &&
			programTarget.Name == target.JSTS.Name &&
			programTarget.AnchorFileRef == target.JSTS.ManifestFileRef
	default:
		return false
	}
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

func synthesizeProgramPageRuntimePortfolio(
	ctx context.Context,
	cacheRoot string,
	noCache bool,
	batchConcurrency int,
	batchController *llm.BatchController,
	providerFactory targetPortfolioProviderFactory,
	runner runtimePortfolioRunner,
	portfolio programpage.Portfolio,
	runs []targetPublishedRun,
	output *runOutput,
) error {
	input, owner, err := programPageRuntimePortfolioInput(portfolio, runs)
	if err != nil {
		return err
	}
	if providerFactory == nil {
		return fmt.Errorf("runtime portfolio: model provider is unavailable")
	}
	provider, err := providerFactory()
	if err != nil {
		return fmt.Errorf("runtime portfolio: configure provider: %w", err)
	}
	if provider == nil {
		return fmt.Errorf("runtime portfolio: configured model provider is unavailable")
	}
	if runner == nil {
		runner = runtimeportfolio.Run
	}
	writer, err := debugdump.OpenWriter(owner.RunDir, false)
	if err != nil {
		return fmt.Errorf("runtime portfolio: open semantic artifact writer: %w", err)
	}
	executor := llm.Executor{
		RootDir: cacheRoot, Enabled: !noCache,
		Observer:         debugdump.NewSemanticObserver(writer),
		BatchConcurrency: batchConcurrency,
		BatchController:  batchController,
	}
	if output != nil {
		output.Stage("Repository overview", "synthesizing runtime roles across completed target pages")
	}
	started := time.Now()
	outcome, runErr := runner(
		ctx,
		debugdump.BindStage(executor, debugdump.SemanticStageRuntimePortfolio),
		provider,
		input,
	)
	closeErr := writer.Close()
	if runErr != nil {
		return errors.Join(fmt.Errorf("runtime portfolio: synthesize repository roles: %w", runErr), closeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("runtime portfolio: close semantic artifact writer: %w", closeErr)
	}
	if err := outcome.Value.ValidateAgainst(input); err != nil {
		return fmt.Errorf("runtime portfolio: accepted result does not match exact repository input: %w", err)
	}
	if err := persistProgramPagePortfolioForRuns(portfolio, runs); err != nil {
		return err
	}
	if err := persistRuntimePortfolioForRuns(input, outcome.Value, runs); err != nil {
		return err
	}
	state := debugdump.SemanticStateAccepted
	semanticCalls := outcome.SemanticCalls
	transportAttempts := outcome.Metrics.Attempts
	latencyMillis := outcome.Metrics.Latency.Milliseconds()
	if outcome.Cached {
		state = debugdump.SemanticStateCacheHit
		transportAttempts = 0
		latencyMillis = 0
	}
	if err := recordSemanticStageDiagnostic(owner.RunDir, semanticStageDiagnostic{
		Stage: debugdump.SemanticStageRuntimePortfolio, State: state,
		RequestBytes: outcome.RequestBytes, SemanticCalls: semanticCalls,
		TransportAttempts: transportAttempts, LatencyMillis: latencyMillis,
	}); err != nil {
		return fmt.Errorf("runtime portfolio: record semantic diagnostics: %w", err)
	}
	if output != nil {
		source := "live provider"
		if outcome.Cached {
			source = "validated cache"
		}
		output.State(
			"Repository overview", "ready",
			"source: "+source,
			fmt.Sprintf("runtime roles: %d", len(outcome.Value.Roles)),
			fmt.Sprintf("mapped targets: %d/%d", outcome.Value.Coverage.TargetsMapped, outcome.Value.Coverage.TargetsObserved),
			formatRunOutputWallDuration(time.Since(started)),
			"artifact: "+runtimeportfolio.ArtifactFilename,
		)
	}
	return nil
}

func programPageRuntimePortfolioInput(
	portfolio programpage.Portfolio,
	runs []targetPublishedRun,
) (runtimeportfolio.Input, targetPublishedRun, error) {
	if err := portfolio.Validate(); err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, err
	}
	runsByID := make(map[string]targetPublishedRun, len(runs))
	for _, run := range runs {
		if _, duplicate := runsByID[run.RunID]; duplicate {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: duplicate published run")
		}
		runsByID[run.RunID] = run
	}
	if len(runsByID) != len(portfolio.Pages) {
		return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: completed run coverage is incomplete")
	}
	result := runtimeportfolio.Input{
		TargetPagePortfolioSHA256: portfolio.SHA256,
		Targets:                   []runtimeportfolio.TargetInput{},
		RepositoryEvidence:        []runtimeportfolio.EvidenceInput{},
	}
	var owner targetPublishedRun
	for _, binding := range portfolio.Pages {
		run, found := runsByID[binding.RunID]
		if !found {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: completed page is missing")
		}
		page := run.ProgramPage
		if page.RunID != run.RunID {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: completed page identity is invalid")
		}
		if !reflect.DeepEqual(page.ProgramTarget, binding.Target) {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: page target does not match program page authority")
		}
		data, err := report.ReadRunDir(run.RunDir)
		if err != nil {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: read completed page %s: %w", run.RunID, err)
		}
		if result.RepositoryName == "" {
			result.RepositoryName = data.RepoName
		} else if result.RepositoryName != data.RepoName {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: target pages disagree on repository identity")
		}
		if result.CapturedRevision == "" {
			result.CapturedRevision = run.SelectedRevision
		} else if result.CapturedRevision != run.SelectedRevision {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: target pages disagree on captured revision")
		}
		input, err := runtimePortfolioTargetInput(
			data, page, binding.Target.ID == portfolio.DefaultTargetID,
		)
		if err != nil {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: completed page %s: %w", run.RunID, err)
		}
		result.Targets = append(result.Targets, input)
		if binding.Target.ID == portfolio.DefaultTargetID {
			owner = run
		}
	}
	if owner.RunDir == "" {
		return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: default target page is missing")
	}
	readmeRows, err := readRuntimePortfolioReadmeRoles(owner.RunDir)
	if err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, err
	}
	result.RepositoryEvidence = runtimePortfolioRepositoryEvidence(readmeRows)
	if _, err := runtimeportfolio.Compile(result); err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: compile completed-page input: %w", err)
	}
	return result, owner, nil
}

func validateProgramPageRuntimeArtifacts(
	portfolio programpage.Portfolio,
	targetOutcomes targetoutcome.Portfolio,
	runs []targetPublishedRun,
) error {
	input, _, err := programPageRuntimePortfolioInput(portfolio, runs)
	if err != nil {
		return err
	}
	portfolioBytes, err := portfolio.CanonicalJSON()
	if err != nil {
		return err
	}
	targetOutcomeBytes, err := targetOutcomes.CanonicalJSON()
	if err != nil {
		return err
	}
	runtimeValidator := runtimePortfolioArtifactSetValidator{}
	for _, run := range runs {
		pageBytes, found, err := readTargetPageRunFile(
			run.RunDir, programpage.ArtifactFilename, programpage.MaxArtifactBytes,
		)
		if err != nil {
			return err
		}
		if !found || !bytes.Equal(pageBytes, portfolioBytes) {
			return fmt.Errorf("program page portfolio: run %s has stale page authority", run.RunID)
		}
		outcomeBytes, found, err := readTargetPageRunFile(
			run.RunDir, targetoutcome.ArtifactFilename, targetoutcome.MaxArtifactBytes,
		)
		if err != nil {
			return err
		}
		if !found || !bytes.Equal(outcomeBytes, targetOutcomeBytes) {
			return fmt.Errorf("target outcome portfolio: run %s has stale selected-target authority", run.RunID)
		}
		raw, found, err := readTargetPageRunFile(
			run.RunDir, runtimeportfolio.ArtifactFilename, runtimeportfolio.MaxArtifactBytes,
		)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("runtime portfolio: run %s is missing the repository artifact", run.RunID)
		}
		if err := runtimeValidator.validate(raw, func(candidate []byte) error {
			return fullyValidateRuntimePortfolioArtifact(candidate, input)
		}); err != nil {
			return fmt.Errorf("runtime portfolio: run %s: %w", run.RunID, err)
		}
	}
	return nil
}

func finalizeProgramPageRuns(
	ctx context.Context,
	portfolio programpage.Portfolio,
	targetOutcomes targetoutcome.Portfolio,
	runs []targetPublishedRun,
) error {
	if err := validateProgramPageRuntimeArtifacts(portfolio, targetOutcomes, runs); err != nil {
		return err
	}
	for index := range runs {
		data, err := report.ReadRunDir(runs[index].RunDir)
		if err != nil {
			return fmt.Errorf("program page portfolio: restore runtime evidence for run %s: %w", runs[index].RunID, err)
		}
		paths, err := report.CapturedInputPaths(data)
		if err != nil {
			return fmt.Errorf("program page portfolio: collect runtime evidence for run %s: %w", runs[index].RunID, err)
		}
		extended, err := report.ExtendRunAuthority(ctx, runs[index].Authority, paths)
		if err != nil {
			return fmt.Errorf("program page portfolio: authorize runtime evidence for run %s: %w", runs[index].RunID, err)
		}
		runs[index].Authority = extended
	}

	runByID := make(map[string]targetPublishedRun, len(runs))
	for _, run := range runs {
		runByID[run.RunID] = run
	}
	for _, binding := range portfolio.Pages {
		run, found := runByID[binding.RunID]
		if !found {
			return fmt.Errorf("program page portfolio: completed run is missing")
		}
		page := run.ProgramPage
		if page.RunID != run.RunID {
			return fmt.Errorf("program page portfolio: completed page identity is invalid")
		}
		if !reflect.DeepEqual(page.ProgramTarget, binding.Target) {
			return fmt.Errorf("program page portfolio: completed page target mismatch")
		}
		if err := run.generateBackingPageData(); err != nil {
			return fmt.Errorf("program page portfolio: finalize backing run %s: %w", run.RunID, err)
		}
		manifest, err := report.ReadRunManifest(run.RunDir)
		if err != nil {
			return fmt.Errorf("program page portfolio: verify run %s: %w", run.RunID, err)
		}
		if manifest.RepositoryStateSHA256 != run.RepositoryStateSHA256 ||
			manifest.MaterialInputs.SelectedRevision != run.SelectedRevision ||
			manifest.MaterialInputs.ProgramTargetID != binding.Target.ID ||
			manifest.MaterialInputs.ProgramPagePortfolioSHA256 == "" {
			return fmt.Errorf("program page portfolio: run %s authority mismatch", run.RunID)
		}
	}
	return nil
}

func publishProgramPageBundle(
	portfolio programpage.Portfolio,
	runs []targetPublishedRun,
) error {
	if err := portfolio.Validate(); err != nil {
		return err
	}
	runsByID := make(map[string]targetPublishedRun, len(runs))
	for _, run := range runs {
		if run.RunID == "" || run.RunDir == "" || filepath.Base(run.RunDir) != run.RunID {
			return fmt.Errorf("program page bundle: completed run identity is invalid")
		}
		if _, duplicate := runsByID[run.RunID]; duplicate {
			return fmt.Errorf("program page bundle: duplicate completed run")
		}
		runsByID[run.RunID] = run
	}
	if len(runsByID) != len(portfolio.Pages) {
		return fmt.Errorf("program page bundle: completed run coverage is incomplete")
	}
	defaultRunDir := ""
	for _, page := range portfolio.Pages {
		run, found := runsByID[page.RunID]
		if !found {
			return fmt.Errorf("program page bundle: portfolio run is missing")
		}
		if page.Target.ID == portfolio.DefaultTargetID {
			defaultRunDir = run.RunDir
		}
	}
	if defaultRunDir == "" {
		return fmt.Errorf("program page bundle: default run is missing")
	}
	if err := report.WriteProgramPageBundleFromArtifactsAtomic(defaultRunDir, portfolio); err != nil {
		return fmt.Errorf("program page bundle: publish: %w", err)
	}
	return nil
}
