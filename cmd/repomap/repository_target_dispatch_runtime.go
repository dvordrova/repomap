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
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

type repositoryTargetDispatchOptions struct {
	Repo      string
	ExtraArgs []string
	Deps      defaultRunDeps

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
	Output          *runOutput
	FirstLayer      *debugdump.SemanticObserver
	DiscoverJSTSFn  jsTSProjectDiscoverer
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
	materializedJSTS, err := materializeSelectedJSTSProjects(ctx, options, ordered)
	if err != nil {
		return report.FailedPublicationAssessment(), "", err
	}

	goSnapshots := make(map[repositoryTargetKey]snapshot.Snapshot)
	allGoSnapshots := make([]snapshot.Snapshot, 0)
	if options.Plan.GoSource != nil {
		for _, target := range options.Plan.Targets {
			if target.Key.Adapter != repositoryTargetAdapterGo {
				continue
			}
			scoped, err := snapshot.ScopeAnalysisTarget(*options.Plan.GoSource, target.Key.Ref)
			if err != nil {
				return report.FailedPublicationAssessment(), "", fmt.Errorf(
					"repository target dispatcher: scope Go target %s: %w", target.Key.String(), err,
				)
			}
			goSnapshots[target.Key] = scoped
			allGoSnapshots = append(allGoSnapshots, scoped)
		}
	}

	runs := make([]targetPublishedRun, 0, len(ordered))
	attemptedRunDirs := make([]string, 0, len(ordered))
	pendingTargets := make([]targetPageConsoleContext, 0, len(ordered))
	failPublication := func(runErr error) (report.PublicationAssessment, string, error) {
		markTargetPagesFailed(options.Output, pendingTargets)
		return report.FailedPublicationAssessment(), "", errors.Join(
			runErr, quarantineTargetPagePublication(attemptedRunDirs),
		)
	}
	var preparedWorkspace *surfacediscovery.PreparedWorkspace
	for index := range ordered {
		target := ordered[index]
		materializedProject, hasMaterializedProject := materializedJSTS[target.Key]
		if hasMaterializedProject {
			target, err = rebindMaterializedJSTSTarget(target, materializedProject)
			if err != nil {
				return failPublication(fmt.Errorf(
					"repository target dispatcher: bind materialized JavaScript/TypeScript target: %w",
					err,
				))
			}
		}
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

		childDeps := options.Deps
		childDeps.sharedRepositoryCorpus = options.Corpus
		state := cloneRepositoryState(options.RepositoryState)
		childDeps.capturedRepositoryState = &state
		childDeps.preselectedTarget = &target
		childDeps.preselectedPythonCatalog = options.Plan.PythonCatalog
		childDeps.preselectedJSTSProject = nil
		if hasMaterializedProject {
			owned := materializedProject.Snapshot()
			childDeps.preselectedJSTSProject = &owned
		}
		childDeps.coreReadmeRoleRows = cloneReadmeRoleLog(options.Plan.Outcome.ReadmeRoles)
		childDeps.runIDOverride = runID
		childDeps.siblingTargetRun = true
		childDeps.preparedGoWorkspace = preparedWorkspace
		childDeps.preparedGoWorkspaceSink = func(workspace *surfacediscovery.PreparedWorkspace) {
			preparedWorkspace = workspace
		}
		if index == 0 {
			outcome := options.Plan.Outcome
			outcome.SelectedTargetRefs = append([]string(nil), options.Plan.Outcome.SelectedTargetRefs...)
			outcome.ReadmeRoles = cloneReadmeRoleLog(options.Plan.Outcome.ReadmeRoles)
			childDeps.preselectedOutcome = &outcome
			childDeps.firstLayerObserver = options.FirstLayer
		}
		if target.Key.Adapter == repositoryTargetAdapterGo {
			scoped, ok := goSnapshots[target.Key]
			if !ok {
				return report.FailedPublicationAssessment(), "", fmt.Errorf("repository target dispatcher: Go target snapshot is missing")
			}
			childDeps.precomputedSnapshot = &scoped
			if preparedWorkspace == nil {
				childDeps.preparedGoSnapshots = append([]snapshot.Snapshot(nil), allGoSnapshots...)
			}
		} else {
			childDeps.precomputedSnapshot = nil
			childDeps.preparedGoSnapshots = nil
		}
		var published targetPublishedRun
		childDeps.publishedTargetSink = func(value targetPublishedRun) {
			published = value
		}
		if err := runDefaultWithDeps(options.Repo, options.ExtraArgs, childDeps); err != nil {
			options.Output.TargetPage("failed", consoleTarget)
			return failPublication(
				fmt.Errorf("target page %s failed: %w", consoleTarget.DisplayPath, err),
			)
		}
		if published.RunID != runID || published.RunDir != runDir ||
			published.SelectedTargetKey != target.Key.String() {
			options.Output.TargetPage("failed", consoleTarget)
			return failPublication(
				fmt.Errorf("target page %s returned mismatched authority", consoleTarget.DisplayPath),
			)
		}
		page, err := report.LoadTargetNavigationPage(published.RunDir, published.RunID)
		if err != nil || !repositoryTypedTargetMatchesProgramTarget(target, page.ProgramTarget) {
			options.Output.TargetPage("failed", consoleTarget)
			if err == nil {
				err = fmt.Errorf("program target does not match exact adapter target")
			}
			return failPublication(
				fmt.Errorf("target page %s: %w", consoleTarget.DisplayPath, err),
			)
		}
		runs = append(runs, published)
		pendingTargets = append(pendingTargets, consoleTarget)
	}
	if len(runs) == 1 {
		assessment, err := report.AssessRunPublication(runs[0].RunDir)
		if err != nil {
			return failPublication(err)
		}
		options.Output.TargetPage("complete", pendingTargets[0])
		return assessment, filepath.Join(runs[0].RunDir, "report.html"), nil
	}

	portfolio, err := buildProgramPagePortfolio(runs, options.RunID)
	if err != nil {
		return failPublication(err)
	}
	if err := synthesizeProgramPageRuntimePortfolio(
		ctx,
		options.DebugDir,
		options.NoCache,
		options.Deps.newCubeProvider,
		options.Deps.runRuntimePortfolio,
		portfolio,
		runs,
		options.Output,
	); err != nil {
		return failPublication(err)
	}
	if err := finalizeProgramPageRuns(ctx, portfolio, runs); err != nil {
		return failPublication(err)
	}
	if options.NoServe {
		if err := publishStandaloneProgramPageBundle(portfolio, runs); err != nil {
			return failPublication(err)
		}
	}

	defaultRun := runs[0]
	assessment, err := report.AssessRunPublication(defaultRun.RunDir)
	if err != nil {
		return failPublication(err)
	}
	for _, run := range runs[1:] {
		if _, err := report.AssessRunPublication(run.RunDir); err != nil {
			return failPublication(err)
		}
	}
	for _, consoleTarget := range pendingTargets {
		options.Output.TargetPage("complete", consoleTarget)
	}
	return assessment, filepath.Join(defaultRun.RunDir, "report.html"), nil
}

// materializeSelectedJSTSProjects is the selected-target execution boundary
// for the TypeScript compiler. It runs before any page starts so a missing
// owner-prepared compiler fails without publishing or doing unrelated heavy
// target work. Exact Go/Python plans contain no JSTS target and therefore make
// no compiler call even though the JSTS scout participated in planning.
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
		if target.JSTS == nil {
			return nil, fmt.Errorf("repository target dispatcher: selected JavaScript/TypeScript target lacks scout authority")
		}
		started := time.Now()
		if options.Output != nil {
			options.Output.State(
				"JavaScript/TypeScript project materialization", "started",
				"language adapter: JavaScript/TypeScript",
				"target: "+target.JSTS.Name,
				"manifest: "+target.JSTS.ManifestPath,
				"selector: "+target.JSTS.Selector,
			)
		}
		project, err := discover(ctx, options.Corpus, options.Repo, target.JSTS.Selector)
		if err != nil {
			if jsTSOwnerPreparationError(err) {
				return nil, fmt.Errorf(
					"materialize selected JavaScript/TypeScript package project %s (manifest %s): %w; the owner must prepare the manifest-declared TypeScript compiler in repository-local node_modules with the project's normal install command before running repomap; repomap never installs packages",
					target.JSTS.Selector, target.JSTS.ManifestPath, err,
				)
			}
			return nil, fmt.Errorf(
				"materialize selected JavaScript/TypeScript package project %s (manifest %s): %w",
				target.JSTS.Selector, target.JSTS.ManifestPath, err,
			)
		}
		if err := validateJSTSTargetMaterialization(options.Corpus, *target.JSTS, project); err != nil {
			return nil, fmt.Errorf(
				"materialize selected JavaScript/TypeScript package project %s: %w",
				target.JSTS.Selector,
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
