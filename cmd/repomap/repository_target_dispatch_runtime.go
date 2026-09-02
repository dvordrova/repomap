package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

type repositoryTargetDispatchOptions struct {
	Repo      string
	ExtraArgs []string
	Deps      defaultRunDeps
	GoTarget  string
	// GoBuildTags and the direct-call controls are parsed once by ordinary main
	// and passed to the Go adapter before the shared ProgramIndex seam.
	GoBuildTags         []string
	DirectCallDepth     int
	DirectCallEdgeLimit int

	Corpus           *corpus.Corpus
	RepositoryState  freshness.RepositoryState
	Plan             repositoryTargetPlan
	RunID            string
	DebugDir         string
	NoCache          bool
	NoOpen           bool
	NoServe          bool
	Port             int
	StaticHost       string
	Output           *runOutput
	FirstLayer       *debugdump.SemanticObserver
	DiscoverJSTSFn   jsTSProjectDiscoverer
	VerifiedRunsSink func([]report.VerifiedRunReceipt)
}

// repositoryGoWorkspaceState keeps the successful fast-path workspace live
// across exact Go pages. If its initial selected-target union cannot be
// prepared, it permanently switches this run to exact per-target preparation;
// otherwise one bad sibling would poison every later healthy target.
type repositoryGoWorkspaceState struct {
	workspace        *surfacediscovery.PreparedWorkspace
	unionUnavailable bool
}

// dispatchRepositoryTargetPlan is the one owner-path execution loop. Target
// discovery and semantic selection have already completed exactly once. Each
// iteration supplies one typed adapter target to the ordinary single-page
// pipeline; artifact filenames remain page-local and the shared semantic
// pipeline therefore needs no language combinations or filename prefixes.
func dispatchRepositoryTargetPlan(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
) (report.PublicationAssessment, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ordered, err := repositoryTargetExecutionOrder(options.Plan)
	if err != nil {
		return report.FailedPublicationAssessment(), "", err
	}
	if options.Corpus == nil {
		return report.FailedPublicationAssessment(), "", fmt.Errorf("repository target dispatcher: repository corpus is unavailable")
	}
	if err := options.RepositoryState.Validate(); err != nil {
		return report.FailedPublicationAssessment(), "", fmt.Errorf("repository target dispatcher: repository state: %w", err)
	}
	if options.RunID == "" || options.DebugDir == "" {
		return report.FailedPublicationAssessment(), "", fmt.Errorf("repository target dispatcher: run identity is incomplete")
	}
	if options.Output == nil {
		options.Output = newRunOutput(options.Deps.stderr)
	}
	multiTarget := len(ordered) > 1
	selectedTargets := make(map[repositoryTargetKey]targetoutcome.SelectedTarget, len(ordered))
	selectedTargetRows := make([]targetoutcome.SelectedTarget, 0, len(ordered))
	for _, target := range ordered {
		selected, selectedErr := repositorySelectedTarget(target)
		if selectedErr != nil {
			return report.FailedPublicationAssessment(), "", fmt.Errorf(
				"repository target dispatcher: project selected target %s: %w",
				target.Key.String(), selectedErr,
			)
		}
		selectedTargets[target.Key] = selected
		selectedTargetRows = append(selectedTargetRows, selected)
	}
	reportSelectedTargetOutcomeScaleWarnings(options.Output, selectedTargetRows)
	defaultSelected, found := selectedTargets[options.Plan.Default]
	if !found {
		return report.FailedPublicationAssessment(), "", fmt.Errorf(
			"repository target dispatcher: selected default identity is absent",
		)
	}
	reducedDocumentation, err := reduceRepositoryDocumentationForRun(
		ctx,
		options.DebugDir,
		options.NoCache,
		options.Deps.llmBatchConcurrency,
		options.Deps.llmBatchController,
		options.Deps.newCubeProvider,
		options.Deps.runDocumentationReduce,
		options.FirstLayer,
		options.Plan.guidance,
		options.Output,
	)
	if err != nil {
		return report.FailedPublicationAssessment(), "", err
	}

	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return report.FailedPublicationAssessment(), "", err
	}
	dispatchPlans := make(map[repositoryTargetAdapter]any)
	for _, target := range ordered {
		if _, prepared := dispatchPlans[target.Key.Adapter]; prepared {
			continue
		}
		descriptor, ok := registry.descriptor(target.Key.Adapter)
		if !ok {
			return report.FailedPublicationAssessment(), "", fmt.Errorf(
				"repository target dispatcher: adapter %q is not registered", target.Key.Adapter,
			)
		}
		state, prepareErr := descriptor.PrepareDispatchPlan(options.Plan, ordered)
		if prepareErr != nil {
			return report.FailedPublicationAssessment(), "", prepareErr
		}
		dispatchPlans[target.Key.Adapter] = state
	}

	runs := make([]targetPublishedRun, 0, len(ordered))
	attemptedRunDirs := make([]string, 0, len(ordered))
	pendingTargets := make([]targetPageConsoleContext, 0, len(ordered))
	outcomes := make([]targetoutcome.Outcome, 0, len(ordered))
	targetErrors := make([]error, 0, len(ordered))
	failPublication := func(runErr error) (report.PublicationAssessment, string, error) {
		reportAnalyzedTargetPagePublicationFailure(options.Output, pendingTargets)
		return report.FailedPublicationAssessment(), "", errors.Join(
			runErr, quarantineTargetPagePublication(attemptedRunDirs),
		)
	}
	recordFailure := func(
		selected targetoutcome.SelectedTarget,
		consoleTarget targetPageConsoleContext,
		stage targetoutcome.Stage,
		reason targetoutcome.Reason,
		targetErr error,
	) error {
		outcome, outcomeErr := targetoutcome.NewNotAnalyzed(selected, stage, reason)
		if outcomeErr != nil {
			return outcomeErr
		}
		if quarantineErr := quarantineTargetPagePublication([]string{
			filepath.Join(options.DebugDir, consoleTarget.RunID),
		}); quarantineErr != nil {
			return quarantineErr
		}
		outcomes = append(outcomes, outcome)
		wrapped := fmt.Errorf("target page %s failed: %w", consoleTarget.DisplayPath, targetErr)
		targetErrors = append(targetErrors, wrapped)
		options.Output.TargetPage("failed", consoleTarget)
		options.Output.Warn(
			"Target not analyzed",
			"target: "+consoleTarget.DisplayPath,
			"scope: "+consoleTarget.Scope,
			"stage: "+string(stage),
			"reason: "+string(reason),
			targetErr.Error(),
		)
		return nil
	}
	for index := range ordered {
		target := ordered[index]
		runID := options.RunID
		role := "default"
		if index > 0 {
			runID = debugdump.GenerateRunID(
				repoRunLabel(options.Repo) + "-" + repositoryTypedTargetDisplay(target),
			)
			role = "sibling"
		}
		runDir := filepath.Join(options.DebugDir, runID)
		attemptedRunDirs = append(attemptedRunDirs, runDir)
		consoleTarget := targetPageConsoleContext{
			DisplayPath: repositoryTypedTargetDisplay(target),
			Scope:       target.Key.String(),
			RunID:       runID,
			Role:        role,
		}
		options.Output.TargetPage("started", consoleTarget)
		selected := selectedTargets[target.Key]
		currentStage := targetoutcome.StageTargetPreparation

		descriptor, ok := registry.descriptor(target.Key.Adapter)
		if !ok {
			return failPublication(fmt.Errorf(
				"repository target dispatcher: adapter %q is not registered", target.Key.Adapter,
			))
		}
		dispatchBinding, prepareErr := descriptor.PrepareDispatchTarget(
			ctx, options, target, dispatchPlans[target.Key.Adapter],
		)
		if prepareErr != nil {
			if ctx.Err() != nil {
				return failPublication(prepareErr)
			}
			stage, reason := classifyRepositoryTargetFailure(currentStage, prepareErr)
			if failureErr := recordFailure(selected, consoleTarget, stage, reason, prepareErr); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}
		if err := dispatchBinding.Target.validateWith(registry); err != nil ||
			!sameRepositoryPlannedTarget(target, dispatchBinding.Target) {
			if err == nil {
				err = fmt.Errorf("prepared target changed its planned identity")
			}
			stage, reason := classifyRepositoryTargetFailure(currentStage, err)
			if failureErr := recordFailure(
				selected, consoleTarget, stage, reason, err,
			); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}
		target = dispatchBinding.Target
		currentStage = targetoutcome.StageProgramAnalysis
		var programPage repositoryProgramPageAuthority
		if !dispatchBinding.ProgramFactsBound {
			prepareErr = fmt.Errorf(
				"repository target adapter %q did not bind one compiler fact snapshot",
				target.Key.Adapter,
			)
		} else {
			programPage, prepareErr = buildRepositoryProgramPageAuthority(
				registry,
				repositoryProgramBuildRequest{
					Context: ctx, Corpus: options.Corpus,
					Target: target, Facts: dispatchBinding.ProgramFacts,
				},
			)
		}
		// Adapter-native compiler/parser facts are live only across the atomic
		// ProgramIndex + dependency projection. Release them before any semantic
		// or report work begins, including when the projection fails.
		dispatchBinding.ProgramFacts = nil
		if prepareErr != nil {
			stage, reason := classifyRepositoryTargetFailure(currentStage, prepareErr)
			if failureErr := recordFailure(selected, consoleTarget, stage, reason, prepareErr); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}

		childDeps := options.Deps
		childDeps.sharedRepositoryCorpus = options.Corpus
		state := cloneRepositoryState(options.RepositoryState)
		childDeps.capturedRepositoryState = &state
		childDeps.preselectedTarget = &target
		childDeps.preselectedProgramPage = &programPage
		childDeps.coreReadmeRoleRows = cloneReadmeRoleLog(options.Plan.Outcome.ReadmeRoles)
		childDeps.runIDOverride = runID
		childDeps.siblingTargetRun = true
		childDeps.deferredPortfolioHTML = true
		ownedDocumentation, documentationErr := reducedDocumentation.Snapshot()
		if documentationErr != nil {
			return failPublication(fmt.Errorf(
				"repository target dispatcher: own reduced documentation for %s: %w",
				consoleTarget.DisplayPath,
				documentationErr,
			))
		}
		childDeps.reducedDocumentation = &ownedDocumentation
		childDeps.targetOutcomeStageSink = func(stage targetoutcome.Stage) {
			currentStage = stage
		}
		var published targetPublishedRun
		childDeps.publishedTargetSink = func(value targetPublishedRun) {
			published = value
		}
		if err := runDefaultWithDeps(options.Repo, options.ExtraArgs, childDeps); err != nil {
			if ctx.Err() != nil {
				return failPublication(fmt.Errorf("target page %s failed: %w", consoleTarget.DisplayPath, err))
			}
			stage, reason := classifyRepositoryTargetFailure(currentStage, err)
			if failureErr := recordFailure(selected, consoleTarget, stage, reason, err); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}
		if published.RunID != runID || published.RunDir != runDir ||
			published.SelectedTargetKey != target.Key.String() {
			authorityErr := fmt.Errorf("target page returned mismatched authority")
			if failureErr := recordFailure(
				selected, consoleTarget, targetoutcome.StageTargetPage,
				targetoutcome.ReasonTargetOutputInvalid, authorityErr,
			); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}
		page := published.ProgramPage
		if page.RunID != published.RunID ||
			!repositoryTypedTargetMatchesProgramTarget(target, page.ProgramTarget) {
			err = fmt.Errorf("program target does not match exact adapter target")
			if failureErr := recordFailure(
				selected, consoleTarget, targetoutcome.StageTargetPage,
				targetoutcome.ReasonTargetOutputInvalid, err,
			); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}
		if err := published.GroupIndex.Validate(); err != nil ||
			published.GroupIndex.Target.ID != page.ProgramTarget.ID {
			if err == nil {
				err = fmt.Errorf("GroupsIndex target does not match exact adapter target")
			}
			if failureErr := recordFailure(
				selected, consoleTarget, targetoutcome.StageSemanticAnalysis,
				targetoutcome.ReasonTargetOutputInvalid, err,
			); failureErr != nil {
				return failPublication(failureErr)
			}
			continue
		}
		published.ProgramPage = page
		analyzed, analyzedErr := targetoutcome.NewAnalyzed(selected, page.ProgramTarget, runID)
		if analyzedErr != nil {
			return failPublication(analyzedErr)
		}
		outcomes = append(outcomes, analyzed)
		runs = append(runs, published)
		pendingTargets = append(pendingTargets, consoleTarget)
	}
	targetOutcomePortfolio, err := targetoutcome.Build(defaultSelected.ID, outcomes)
	if err != nil {
		return failPublication(err)
	}
	reportTargetOutcomeScaleWarnings(options.Output, targetOutcomePortfolio)
	if len(runs) == 0 {
		failedRunDir := filepath.Join(options.DebugDir, options.RunID)
		flushFailedFirstLayerSemanticJournal(failedRunDir, options.FirstLayer, options.Output)
		diagnosticErr := errors.Join(
			recordTargetPortfolioOutcome(failedRunDir, options.Plan.Outcome, options.Output),
			persistTargetOutcomePortfolioForRunDirs(targetOutcomePortfolio, []string{failedRunDir}),
		)
		return report.FailedPublicationAssessment(), "", errors.Join(
			fmt.Errorf("all selected repository targets were not analyzed"),
			errors.Join(targetErrors...), diagnosticErr,
		)
	}

	owner := runs[0]
	flushFirstLayerSemanticJournal(owner.RunDir, options.FirstLayer, options.Output)
	if err := recordTargetPortfolioOutcome(owner.RunDir, options.Plan.Outcome, options.Output); err != nil {
		return failPublication(err)
	}
	if multiTarget {
		runs, err = matchPublishedRunGroups(
			ctx,
			options.DebugDir,
			options.NoCache,
			options.Deps.llmBatchConcurrency,
			options.Deps.llmBatchController,
			options.Deps.newCubeProvider,
			options.Deps.runGroupMatching,
			runs,
			options.Output,
		)
		if err != nil {
			return failPublication(err)
		}
	}
	owner = runs[0]
	// The first-day layers read the completed graph and the repository corpus,
	// then land in every backing run before its report data is generated.
	if err := buildFirstDayLayers(ctx, firstDayOptions{
		RepoPath:         options.Repo,
		RepositoryName:   repoRunLabel(options.Repo),
		Revision:         options.RepositoryState.Head,
		Corpus:           options.Corpus,
		TrackedPaths:     repositoryTrackedPaths(ctx, options.Repo),
		Runs:             runs,
		CacheRoot:        options.DebugDir,
		NoCache:          options.NoCache,
		BatchConcurrency: options.Deps.llmBatchConcurrency,
		BatchController:  options.Deps.llmBatchController,
		ProviderFactory:  options.Deps.newCubeProvider,
		Runner:           options.Deps.runOrientation,
		Output:           options.Output,
	}); err != nil {
		return failPublication(err)
	}
	portfolio, err := buildProgramPagePortfolio(runs, owner.RunID)
	if err != nil {
		return failPublication(err)
	}
	reportProgramPagePortfolioScaleWarnings(options.Output, portfolio)
	if err := persistTargetOutcomePortfolioForRuns(targetOutcomePortfolio, runs); err != nil {
		return failPublication(err)
	}
	if err := finalizeProgramPageRuns(
		ctx, portfolio, targetOutcomePortfolio, runs, options.Output,
	); err != nil {
		return failPublication(err)
	}
	assessment, err := publishProgramPageBundle(portfolio, runs)
	reportTargetPageRunScaleWarnings(
		options.Output, runs, assessment.ScaleWarnings(), assessment.TargetScaleWarnings(),
	)
	if err != nil {
		return failPublication(err)
	}
	if options.VerifiedRunsSink != nil {
		receipts := make([]report.VerifiedRunReceipt, 0, len(runs))
		for _, run := range runs {
			receipts = append(receipts, run.Receipt)
		}
		options.VerifiedRunsSink(receipts)
	}
	for _, consoleTarget := range pendingTargets {
		options.Output.TargetPage("complete", consoleTarget)
	}
	options.Output.State(
		"Target coverage", "complete",
		fmt.Sprintf("analyzed: %d/%d", len(runs), len(ordered)),
		fmt.Sprintf("not analyzed: %d", len(ordered)-len(runs)),
	)
	return assessment, filepath.Join(owner.RunDir, "report.html"), nil
}

// materializeSelectedJSTSProjects is the selected-target execution boundary
// for the TypeScript compiler. The dispatcher invokes it independently for
// each JSTS page so one package's missing owner-prepared compiler does not
// suppress unrelated targets. Exact Go/Python targets make no compiler call
// even though the JSTS scout participated in repository planning.
func materializeSelectedJSTSProjects(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	ordered []repositoryTypedTarget,
) (map[repositoryTargetKey]jstsproject.Result, error) {
	result := make(map[repositoryTargetKey]jstsproject.Result)
	discover := options.DiscoverJSTSFn
	if discover == nil {
		discover = jstsproject.DiscoverSelected
	}
	for _, target := range ordered {
		if target.Key.Adapter != repositoryTargetAdapterJSTS {
			continue
		}
		jstsTarget, ok := repositoryJSTSTarget(target)
		if !ok {
			return nil, fmt.Errorf("repository target dispatcher: selected JavaScript/TypeScript target lacks scout authority")
		}
		started := time.Now()
		if options.Output != nil {
			options.Output.State(
				"JavaScript/TypeScript project materialization", "started",
				"language adapter: JavaScript/TypeScript",
				"target: "+jstsTarget.Name,
				"manifest: "+jstsTarget.ManifestPath,
				"selector: "+jstsTarget.Selector,
			)
		}
		project, err := discover(ctx, options.Corpus, options.Repo, jstsTarget.Selector)
		if err != nil {
			if jsTSOwnerPreparationError(err) {
				return nil, fmt.Errorf(
					"materialize selected JavaScript/TypeScript package project %s (manifest %s): %w; the owner must prepare the manifest-declared TypeScript compiler in repository-local node_modules with the project's normal install command before running repomap; repomap never installs packages",
					jstsTarget.Selector, jstsTarget.ManifestPath, err,
				)
			}
			return nil, fmt.Errorf(
				"materialize selected JavaScript/TypeScript package project %s (manifest %s): %w",
				jstsTarget.Selector, jstsTarget.ManifestPath, err,
			)
		}
		if err := validateJSTSTargetMaterialization(options.Corpus, jstsTarget, project); err != nil {
			return nil, fmt.Errorf(
				"materialize selected JavaScript/TypeScript package project %s: %w",
				jstsTarget.Selector,
				err,
			)
		}
		// The validated selected project is complete authority at this
		// boundary. Report its advisory scale measurements now, before later
		// rebinding, child-run setup, ProgramIndex projection, or persistence
		// can fail and hide them.
		reportJSTSProjectScaleWarnings(options.Output, programindex.Target{
			ID: project.ProgramTargetID, Language: project.Project.Language,
			Name: project.Project.Name, Selector: project.Project.Selector,
		}, project)
		result[target.Key] = project.Snapshot()
		if options.Output != nil {
			options.Output.State(
				"JavaScript/TypeScript project materialization", "ready",
				"manifest: "+project.Project.ManifestPath,
				"language: "+project.Project.Language,
				"target kind: "+jstsproject.TargetKind(project),
				fmt.Sprintf("repository source files: %d", len(project.Files)),
				fmt.Sprintf("product surfaces: %d", jsTSProductSurfaceCount(project)),
				fmt.Sprintf("all classified surfaces: %d", len(project.Surfaces)),
				formatRunOutputWallDuration(time.Since(started)),
			)
		}
	}
	return result, nil
}

func repositoryTargetExecutionOrder(
	plan repositoryTargetPlan,
) ([]repositoryTypedTarget, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	defaultTarget, found := plan.DefaultTarget()
	if !found {
		return nil, fmt.Errorf("repository target dispatcher: default target is absent")
	}
	ordered := make([]repositoryTypedTarget, 0, len(plan.Targets))
	ordered = append(ordered, defaultTarget)
	for _, target := range plan.Targets {
		if target.Key != defaultTarget.Key {
			ordered = append(ordered, target)
		}
	}
	return ordered, nil
}

func finishRepositoryTargetDispatch(
	ctx context.Context,
	deps defaultRunDeps,
	debugDir string,
	runDir string,
	reportPath string,
	publication report.PublicationAssessment,
	noServe bool,
	noOpen bool,
	port int,
	staticHost string,
	verifiedRuns []report.VerifiedRunReceipt,
	output *runOutput,
) error {
	linkLatest(debugDir, runDir, runOutputWarningSink{
		output: output, summary: "could not update latest report link",
	})
	output.State("Report", "generated")
	output.Stage("Report", "path: "+reportPath)
	if staticHost != "" {
		output.Stage("Report", "standalone host: "+staticHost)
	}
	publicationDetails := []string{"report: " + reportPath}
	if publication.Status != report.PublicationReady {
		publicationDetails = append(publicationDetails, "report or analysis artifacts are missing or invalid")
	}
	output.State("Run", strings.ToLower(string(publication.Status)), publicationDetails...)
	if !noServe && deps.serveReport != nil {
		return deps.serveReport(ctx, reportserver.Options{
			RunsDir: debugDir, InitialRunID: filepath.Base(runDir), Port: port,
			VerifiedRuns: verifiedRuns,
			Logf: func(format string, args ...any) {
				output.Stage("Server", fmt.Sprintf(format, args...))
			},
			OnReady: func(url string) error {
				output.State("Server", "ready", "url: "+url, "Ctrl-C to stop")
				if !noOpen && deps.openReport != nil {
					if err := deps.openReport(url); err != nil {
						output.Warn("could not open report", err.Error())
					}
				}
				return nil
			},
		})
	}
	if !noOpen && deps.openReport != nil {
		if err := deps.openReport(reportPath); err != nil {
			output.Warn("could not open report", err.Error())
		}
	}
	return nil
}
