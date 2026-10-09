package run

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
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

	Corpus          *corpus.Corpus
	RepositoryState freshness.RepositoryState
	Plan            repositoryTargetPlan
	RunID           string
	DebugDir        string
	NoCache         bool
	NoOpen          bool
	NoServe         bool
	Port            int
	StaticHost      string
	DisplayLanguage report.DisplayLanguage
	// NoModel walks the atlas without a provider: every cell is its
	// fallback line and no orientation is asked.
	NoModel bool
	// Categorizer answers the atlas's closed tables in a model run.
	Categorizer llm.Categorizer
	// Learn runs the question cascade after the atlas; off, no question is
	// generated, retrieved or answered.
	Learn bool
	// Captions asks the model for prose cells too; off, only decisions.
	Captions         bool
	Questions        []string
	Output           *runOutput
	FirstLayer       *debugdump.SemanticObserver
	DiscoverJSTSFn   jsTSProjectDiscoverer
	VerifiedRunsSink func([]report.RunReceipt)
}

// repositoryGoWorkspaceState keeps the successful fast-path workspace live
// across exact Go pages. If its initial selected-target union cannot be
// prepared, it permanently switches this run to exact per-target preparation;
// otherwise one bad sibling would poison every later healthy target.
type repositoryGoWorkspaceState struct {
	workspace        *surfacediscovery.PreparedWorkspace
	unionUnavailable bool
}

// dispatchRepositoryTargetPlan is the one owner-path execution. Target
// discovery and semantic selection have already completed exactly once. Each
// planned target is supplied as one typed adapter target to the ordinary
// single-page pipeline, one lane per adapter, and the results are folded in
// plan order; artifact filenames remain page-local and the shared semantic
// pipeline therefore needs no language combinations or filename prefixes.
func dispatchRepositoryTargetPlan(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ordered, err := repositoryTargetExecutionOrder(options.Plan)
	if err != nil {
		return "", err
	}
	if options.Corpus == nil {
		return "", fmt.Errorf("repository target dispatcher: repository corpus is unavailable")
	}
	if err := options.RepositoryState.Validate(); err != nil {
		return "", fmt.Errorf("repository target dispatcher: repository state: %w", err)
	}
	if options.RunID == "" || options.DebugDir == "" {
		return "", fmt.Errorf("repository target dispatcher: run identity is incomplete")
	}
	if options.Output == nil {
		options.Output = newRunOutput(options.Deps.stderr)
	}
	selectedTargets := make(map[repositoryTargetKey]targetoutcome.SelectedTarget, len(ordered))
	selectedTargetRows := make([]targetoutcome.SelectedTarget, 0, len(ordered))
	for position, target := range ordered {
		selected, selectedErr := repositorySelectedTarget(fmt.Sprintf("t%d", position+1), target)
		if selectedErr != nil {
			return "", fmt.Errorf(
				"repository target dispatcher: project selected target %s: %w",
				target.Key.String(), selectedErr,
			)
		}
		selectedTargets[target.Key] = selected
		selectedTargetRows = append(selectedTargetRows, selected)
	}
	// An unresolved default names no selected target in the outcome
	// portfolio; the first published page still owns the one physical HTML.
	defaultSelectedID := ""
	if defaultSelected, found := selectedTargets[options.Plan.Default]; found {
		defaultSelectedID = defaultSelected.ID
	}
	// The documentation reduction reads only the planned guidance, and native
	// analysis never reads the reduction, so the model call runs beside the
	// first target's compiler. It is joined before a target page needs it and
	// before the first-layer journal is flushed on every path; it is the only
	// first-layer writer after planning, so the journal keeps its order.
	documentation := startBackground(func() (documentationreduce.Result, error) {
		return reduceRepositoryDocumentationForRun(
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
	})
	defer documentation.wait()
	// Extractors and the tracked-path listing read only the repository, so
	// they run beside the target pages and the facts stage waits for them.
	factSources := startRepositoryFactSources(ctx, options.Repo, options.Corpus)
	defer factSources.wait()

	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return "", err
	}
	dispatchPlans := make(map[repositoryTargetAdapter]any)
	for _, target := range ordered {
		if _, prepared := dispatchPlans[target.Key.Adapter]; prepared {
			continue
		}
		descriptor, ok := registry.descriptor(target.Key.Adapter)
		if !ok {
			return "", fmt.Errorf(
				"repository target dispatcher: adapter %q is not registered", target.Key.Adapter,
			)
		}
		state, prepareErr := descriptor.PrepareDispatchPlan(options.Plan, ordered)
		if prepareErr != nil {
			return "", prepareErr
		}
		dispatchPlans[target.Key.Adapter] = state
	}

	// Every target keeps its position, run identity and console context from
	// the plan, whichever lane reaches it first.
	slots := make([]targetPageSlot, len(ordered))
	for position, target := range ordered {
		runID := options.RunID
		if position > 0 {
			runID = debugdump.GenerateRunID(
				repoRunLabel(options.Repo) + "-" + repositoryTypedTargetDisplay(target),
			)
		}
		role := "sibling"
		if target.Key == options.Plan.Default {
			role = "default"
		}
		selected := selectedTargets[target.Key]
		slots[position] = targetPageSlot{
			position: position, target: target, selected: selected,
			runID: runID, runDir: filepath.Join(options.DebugDir, runID),
			console: targetPageConsoleContext{
				DisplayPath: repositoryTypedTargetDisplay(target),
				Scope:       selected.ID,
				RunID:       runID,
				Role:        role,
			},
		}
	}
	pages := targetPageDispatcher{
		options: options, registry: registry, plans: dispatchPlans,
		programStore: programindex.NewArtifactStore(), documentation: documentation,
	}
	runTargetPageLanes(ctx, slots, pages.page)
	fold := foldTargetPageSlots(slots)
	runs, pendingTargets, outcomes := fold.runs, fold.pendingTargets, fold.outcomes
	failPublication := func(runErr error) (string, error) {
		// A refused documentation reduction preceded every target in the
		// serial order, so it stays the reported cause.
		if _, reduceErr := documentation.wait(); reduceErr != nil {
			return "", reduceErr
		}
		runDir := filepath.Join(options.DebugDir, options.RunID)
		if len(runs) > 0 {
			runDir = runs[0].RunDir
		}
		reportAnalyzedTargetPagePublicationFailure(options.Output, pendingTargets, runDir, runErr)
		return "", runErr
	}
	if fold.stop != nil {
		return failPublication(fold.stop)
	}
	if _, reduceErr := documentation.wait(); reduceErr != nil {
		return "", reduceErr
	}
	targetOutcomePortfolio, err := targetoutcome.Build(defaultSelectedID, outcomes)
	if err != nil {
		return failPublication(err)
	}
	if len(runs) == 0 {
		failedRunDir := filepath.Join(options.DebugDir, options.RunID)
		flushFailedFirstLayerSemanticJournal(failedRunDir, options.FirstLayer, options.Output)
		diagnosticErr := errors.Join(
			recordTargetPortfolioOutcome(failedRunDir, options.Plan.Outcome, options.Output),
			persistTargetOutcomePortfolioForRunDirs(targetOutcomePortfolio, []string{failedRunDir}),
		)
		return "", errors.Join(
			fmt.Errorf("all selected repository targets were not analyzed"),
			errors.Join(fold.targetErrors...), diagnosticErr,
		)
	}

	owner := runs[0]
	flushFirstLayerSemanticJournal(owner.RunDir, options.FirstLayer, options.Output)
	if err := recordTargetPortfolioOutcome(owner.RunDir, options.Plan.Outcome, options.Output); err != nil {
		return failPublication(err)
	}
	// The atlas is read over every target: the tables, then the boxes
	// projected into the groups the page draws, then the orientation over
	// those.
	outcome, err := readRepositoryAtlas(ctx, options, runs, factSources)
	if err != nil {
		return failPublication(err)
	}
	runs, err = projectAtlasRuns(runs, outcome, options.Output)
	if err != nil {
		return failPublication(err)
	}
	receipt, err := orientAndPublishRepositoryReport(ctx, options, runs, &outcome, targetOutcomePortfolio)
	if err != nil {
		return failPublication(err)
	}
	owner = runs[0]
	if options.VerifiedRunsSink != nil {
		options.VerifiedRunsSink([]report.RunReceipt{receipt})
	}
	for _, consoleTarget := range pendingTargets {
		options.Output.TargetPage("complete", consoleTarget)
	}
	options.Output.State(
		"Target coverage", "complete",
		fmt.Sprintf("analyzed: %d/%d", len(runs), len(ordered)),
		fmt.Sprintf("not analyzed: %d", len(ordered)-len(runs)),
	)
	return filepath.Join(owner.RunDir, receipt.HTMLFilename()), nil
}

// targetPageSlot is one planned target's place in the ordered plan. A lane
// writes only its own slots; the dispatcher folds them in target order.
type targetPageSlot struct {
	position int
	target   repositoryTypedTarget
	selected targetoutcome.SelectedTarget
	runID    string
	runDir   string
	console  targetPageConsoleContext

	// outcome is set once the target finished: analyzed with published, or
	// not analyzed with failure. stop is a failure that ends the publication;
	// stopCanceled marks one caused by cancellation.
	outcome      *targetoutcome.Outcome
	failure      error
	published    *targetPublishedRun
	stop         error
	stopCanceled bool
}

// targetPageDispatcher runs the ordinary single-page pipeline for planned
// targets. Its values are shared read-only by every lane; adapter plan state
// is used only by that adapter's lane.
type targetPageDispatcher struct {
	options       repositoryTargetDispatchOptions
	registry      repositoryTargetAdapterRegistry
	plans         map[repositoryTargetAdapter]any
	programStore  *programindex.ArtifactStore
	documentation *background[documentationreduce.Result]
}

// runTargetPageLanes gives every adapter one lane. Lanes run side by side
// because no target page reads another; targets of one adapter stay serial
// in plan order because they share that adapter's native state (the Go
// workspace, the Python parser groups). A target that is not analyzed never
// stops its siblings; a failure that ends the publication cancels the other
// lanes, whose unstarted targets stay unrun.
func runTargetPageLanes(
	ctx context.Context,
	slots []targetPageSlot,
	page func(context.Context, *targetPageSlot),
) {
	laneCtx, cancelLanes := context.WithCancel(ctx)
	defer cancelLanes()
	var lanes [][]int
	laneOf := make(map[repositoryTargetAdapter]int)
	for position := range slots {
		adapter := slots[position].target.Key.Adapter
		lane, found := laneOf[adapter]
		if !found {
			lane = len(lanes)
			laneOf[adapter] = lane
			lanes = append(lanes, nil)
		}
		lanes[lane] = append(lanes[lane], position)
	}
	var running sync.WaitGroup
	for _, lane := range lanes {
		running.Add(1)
		go func(positions []int) {
			defer running.Done()
			for _, position := range positions {
				slot := &slots[position]
				page(laneCtx, slot)
				if slot.stop != nil {
					cancelLanes()
					return
				}
			}
		}(lane)
	}
	running.Wait()
}

// targetPageFold is the lanes' result in target order, as the serial loop
// appended it.
type targetPageFold struct {
	runs           []targetPublishedRun
	pendingTargets []targetPageConsoleContext
	outcomes       []targetoutcome.Outcome
	targetErrors   []error
	// stop is the failure that ends the publication: the lowest-position one
	// that is not a cancellation, or else the lowest-position cancellation.
	stop error
}

func foldTargetPageSlots(slots []targetPageSlot) targetPageFold {
	var fold targetPageFold
	var stopped *targetPageSlot
	for position := range slots {
		slot := &slots[position]
		if slot.stop != nil {
			if stopped == nil || stopped.stopCanceled && !slot.stopCanceled {
				stopped = slot
			}
			continue
		}
		if slot.outcome == nil {
			continue
		}
		fold.outcomes = append(fold.outcomes, *slot.outcome)
		if slot.failure != nil {
			fold.targetErrors = append(fold.targetErrors, slot.failure)
			continue
		}
		fold.runs = append(fold.runs, *slot.published)
		fold.pendingTargets = append(fold.pendingTargets, slot.console)
	}
	if stopped != nil {
		fold.stop = stopped.stop
	}
	return fold
}

// notAnalyzed records the explicit outcome of a target whose page failed.
func (pages targetPageDispatcher) notAnalyzed(
	slot *targetPageSlot,
	stage targetoutcome.Stage,
	reason targetoutcome.Reason,
	targetErr error,
) {
	outcome, err := targetoutcome.NewNotAnalyzed(slot.selected, stage, reason, failureDetail(targetErr, pages.options.Repo))
	if err != nil {
		slot.stop = err
		return
	}
	slot.outcome = &outcome
	slot.failure = fmt.Errorf("target page %s failed: %w", slot.console.DisplayPath, targetErr)
	output := pages.options.Output
	output.TargetPage("failed", slot.console)
	output.Warn(
		"Target not analyzed",
		"target: "+slot.console.DisplayPath,
		"scope: "+slot.console.Scope,
		"stage: "+string(stage),
		"reason: "+string(reason),
		targetErr.Error(),
	)
}

// page prepares one target's native facts, projects its ProgramIndex and
// runs its target page, leaving the result in its slot.
func (pages targetPageDispatcher) page(ctx context.Context, slot *targetPageSlot) {
	options := pages.options
	registry := pages.registry
	if err := ctx.Err(); err != nil {
		slot.stop, slot.stopCanceled = err, true
		return
	}
	target := slot.target
	consoleTarget := slot.console
	options.Output.TargetPage("started", consoleTarget)
	currentStage := targetoutcome.StageTargetPreparation

	descriptor, ok := registry.descriptor(target.Key.Adapter)
	if !ok {
		slot.stop = fmt.Errorf(
			"repository target dispatcher: adapter %q is not registered", target.Key.Adapter,
		)
		return
	}
	prepareStarted := time.Now()
	dispatchBinding, prepareErr := descriptor.PrepareDispatchTarget(
		ctx, options, target, pages.plans[target.Key.Adapter],
	)
	options.Output.Wall("target native analysis", time.Since(prepareStarted))
	if prepareErr != nil {
		if ctx.Err() != nil {
			slot.stop, slot.stopCanceled = prepareErr, true
			return
		}
		stage, reason := classifyRepositoryTargetFailure(currentStage, prepareErr)
		pages.notAnalyzed(slot, stage, reason, prepareErr)
		return
	}
	if err := dispatchBinding.Target.validateWith(registry); err != nil ||
		!sameRepositoryPlannedTarget(target, dispatchBinding.Target) {
		if err == nil {
			err = fmt.Errorf("prepared target changed its planned identity")
		}
		stage, reason := classifyRepositoryTargetFailure(currentStage, err)
		pages.notAnalyzed(slot, stage, reason, err)
		return
	}
	target = dispatchBinding.Target
	currentStage = targetoutcome.StageProgramAnalysis
	projectionStarted := time.Now()
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
				Target: target, TargetID: fmt.Sprintf("t%d", slot.position+1), Facts: dispatchBinding.ProgramFacts,
			},
		)
	}
	// Adapter-native compiler/parser facts are live only across the atomic
	// ProgramIndex + dependency projection. Release them before any semantic
	// or report work begins, including when the projection fails. The
	// platform view they were read in stays, for the run's metadata.
	dispatchBinding.ProgramFacts = nil
	programPage.CPlatform = dispatchBinding.CPlatform
	options.Output.Wall("target program projection", time.Since(projectionStarted))
	if prepareErr != nil {
		stage, reason := classifyRepositoryTargetFailure(currentStage, prepareErr)
		pages.notAnalyzed(slot, stage, reason, prepareErr)
		return
	}
	childDeps := options.Deps
	childDeps.ctx = ctx
	childDeps.sharedRepositoryCorpus = options.Corpus
	state := cloneRepositoryState(options.RepositoryState)
	childDeps.capturedRepositoryState = &state
	childDeps.preselectedTarget = &target
	childDeps.preselectedProgramPage = &programPage
	childDeps.programIndexStore = pages.programStore
	childDeps.coreReadmeRoleRows = cloneReadmeRoleLog(options.Plan.Outcome.ReadmeRoles)
	childDeps.runIDOverride = slot.runID
	childDeps.siblingTargetRun = true
	childDeps.deferredPortfolioHTML = true
	reducedDocumentation, reduceErr := pages.documentation.wait()
	if reduceErr != nil {
		slot.stop = reduceErr
		return
	}
	ownedDocumentation, documentationErr := reducedDocumentation.Snapshot()
	if documentationErr != nil {
		slot.stop = fmt.Errorf(
			"repository target dispatcher: own reduced documentation for %s: %w",
			consoleTarget.DisplayPath,
			documentationErr,
		)
		return
	}
	childDeps.reducedDocumentation = &ownedDocumentation
	childDeps.targetOutcomeStageSink = func(stage targetoutcome.Stage) {
		currentStage = stage
	}
	var published targetPublishedRun
	childDeps.publishedTargetSink = func(value targetPublishedRun) {
		published = value
	}
	artifactStarted := time.Now()
	childErr := runDefaultWithDeps(options.Repo, options.ExtraArgs, childDeps)
	options.Output.Wall("target artifact preparation", time.Since(artifactStarted))
	if err := childErr; err != nil {
		if ctx.Err() != nil {
			slot.stop = fmt.Errorf("target page %s failed: %w", consoleTarget.DisplayPath, err)
			slot.stopCanceled = true
			return
		}
		stage, reason := classifyRepositoryTargetFailure(currentStage, err)
		pages.notAnalyzed(slot, stage, reason, err)
		return
	}
	if published.RunID != slot.runID || published.RunDir != slot.runDir ||
		published.SelectedTargetKey != target.Key.String() {
		pages.notAnalyzed(
			slot, targetoutcome.StageTargetPage, targetoutcome.ReasonTargetOutputInvalid,
			fmt.Errorf("target page returned mismatched authority"),
		)
		return
	}
	page := published.ProgramPage
	if page.RunID != published.RunID ||
		!repositoryTypedTargetMatchesProgramTarget(target, page.ProgramTarget) {
		pages.notAnalyzed(
			slot, targetoutcome.StageTargetPage, targetoutcome.ReasonTargetOutputInvalid,
			fmt.Errorf("program target does not match exact adapter target"),
		)
		return
	}
	if err := published.GroupIndex.Validate(); err != nil ||
		published.GroupIndex.Target.ID != page.ProgramTarget.ID {
		if err == nil {
			err = fmt.Errorf("GroupsIndex target does not match exact adapter target")
		}
		pages.notAnalyzed(
			slot, targetoutcome.StageSemanticAnalysis, targetoutcome.ReasonTargetOutputInvalid, err,
		)
		return
	}
	published.ProgramPage = page
	analyzed, analyzedErr := targetoutcome.NewAnalyzed(slot.selected, page.ProgramTarget, slot.runID)
	if analyzedErr != nil {
		slot.stop = analyzedErr
		return
	}
	// The saved child artifacts are complete. Subsequent consumers read the
	// sealed program index one target at a time, so completed pages do not
	// accumulate all native graphs while the remaining targets are parsed.
	// The empty child group index is replaced by the atlas projection.
	published.releaseProgramIndex()
	published.GroupIndex = groupindex.Index{}
	slot.outcome = &analyzed
	slot.published = &published
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
					"materialize selected JavaScript/TypeScript package project %s (manifest %s): %w; make a TypeScript compiler available in project node_modules, the selected Node installation, or through tsc on PATH; repomap never installs packages",
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
		// An unresolved default promotes no target: pages keep plan order.
		return append([]repositoryTypedTarget(nil), plan.Targets...), nil
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
	noServe bool,
	noOpen bool,
	port int,
	staticHost string,
	verifiedRuns []report.RunReceipt,
	output *runOutput,
) error {
	linkLatest(debugDir, runDir, runOutputWarningSink{
		output: output, summary: "could not update latest report link",
	})
	output.State("Report", "generated")
	output.Stage("Report", "path: "+reportPath)
	if err := recordCommandTiming(runDir, output); err != nil {
		output.Warn("could not record the command's timing", err.Error())
	}
	output.Timing()
	if staticHost != "" {
		output.Stage("Report", "standalone host: "+staticHost)
	}
	output.State("Run", "ready", "report: "+reportPath)
	if !noServe && deps.serveReport != nil {
		return deps.serveReport(ctx, reportserver.Options{
			Config:  deps.repositoryConfig,
			RunsDir: debugDir, InitialRunID: filepath.Base(runDir), Port: port,
			Runs: verifiedRuns,
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
