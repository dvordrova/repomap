package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/pipeline"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythondependencies"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			printUsage()
			return
		case "--version", "-v":
			fmt.Println("repomap (dev)")
			return
		}
	}

	if len(os.Args) >= 2 && os.Args[1] == "cache" {
		if err := runCache(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	repo := "."
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repo = args[0]
		args = args[1:]
	}
	if err := runDefault(repo, args); err != nil {
		writeDefaultRunError(os.Stderr, err)
		os.Exit(defaultRunExitCode(err))
	}
}

func writeDefaultRunError(writer io.Writer, err error) {
	output := newRunOutput(writer)
	if errors.Is(err, context.Canceled) {
		output.State("Run", "canceled")
		return
	}
	output.Error("run failed", err.Error())
}

func defaultRunExitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

func linkLatest(debugDir, runDir string, stderr io.Writer) {
	latest := filepath.Join(debugDir, "latest")
	os.Remove(latest)
	if err := os.Symlink(filepath.Base(runDir), latest); err != nil {
		fmt.Fprintf(stderr, "warning: could not create latest symlink: %v\n", err)
	}
}

func runDefault(repo string, extraArgs []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runDefaultWithDeps(repo, extraArgs, defaultRunDeps{
		ctx:                        ctx,
		stdout:                     os.Stdout,
		stderr:                     os.Stderr,
		openReport:                 openReport,
		serveReport:                reportserver.Serve,
		captureRepo:                freshness.CaptureRepository,
		newTargetPortfolioProvider: defaultTargetPortfolioProviderFactory,
		newCubeProvider:            defaultTargetPortfolioProviderFactory,
	})
}

type defaultRunDeps struct {
	ctx                        context.Context
	stdout                     io.Writer
	stderr                     io.Writer
	openReport                 func(string) error
	serveReport                func(context.Context, reportserver.Options) error
	captureRepo                func(context.Context, string, *corpus.Corpus) (freshness.RepositoryState, error)
	newTargetPortfolioProvider targetPortfolioProviderFactory
	newCubeProvider            targetPortfolioProviderFactory
	resolveGoTarget            func(string, func(string) string) (gotarget.Target, error)
	precomputedSnapshot        *snapshot.Snapshot
	// preparedGoWorkspace is the one live packages/types/SSA authority shared
	// by the default Go page and every recursively published sibling page.
	preparedGoWorkspace *surfacediscovery.PreparedWorkspace
	// coreReadmeRoleRows carries the one accepted first-layer README role
	// catalog into every selected target page. Each run rebinds paths to its
	// current corpus, so run-local f* identities are never reused blindly.
	coreReadmeRoleRows  []readmeRoleLogRow
	runIDOverride       string
	siblingTargetRun    bool
	publishedTargetSink func(targetPublishedRun)
	finalizeTargetPages func(snapshot.TargetRunContainer, snapshot.TargetPagePortfolio, []targetPublishedRun) error
}

func runDefaultWithDeps(repo string, extraArgs []string, deps defaultRunDeps) (runErr error) {
	stopAfter, err := semanticStopAfter(os.Getenv("STOP_AFTER"))
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	forcePlatform := fs.String("force-platform", "", "force Go platform as GOOS/GOARCH")
	analysisTargetFlag := fs.String("target", "", "analysis surface (unambiguous advertised path or exact target key)")
	directCallDepth := fs.Int(
		"depth", surfacediscovery.DefaultDirectCallDepth,
		"target call-graph depth",
	)
	directCallEdgeLimit := fs.Int(
		"edges-limit", surfacediscovery.DefaultDirectCallEdgeLimit,
		"maximum exact target call-graph edges",
	)
	noCache := fs.Bool("no-cache", false, "disable cross-run model response caches")
	scanSecrets := fs.Bool("scan-secrets", false, "scan repository and model material for credential-like text")
	gitLabURLFlag := fs.String("gitlab-url", "", "create a standalone report linked to this GitLab project or host")
	gitHubURLFlag := fs.String("github-url", "", "create a standalone report linked to this GitHub repository or host")
	noOpen := fs.Bool("no-open", false, "do not open the generated HTML report")
	noServe := fs.Bool("no-serve", false, "generate a static report without starting the local server")
	port := fs.Int("port", 0, "local report server port (default: random)")
	debugDir := fs.String("debug-dir", defaultDebugDir(), "directory for debug artifacts")

	if err := fs.Parse(extraArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsageTo(deps.stderr)
			return nil
		}
		return actionableFlagError(extraArgs, err)
	}
	portExplicit := false
	fs.Visit(func(option *flag.Flag) {
		if option.Name == "port" {
			portExplicit = true
		}
	})
	humanOutput := newRunOutput(deps.stderr)
	var defaultTargetConsole *targetPageConsoleContext
	defaultTargetConsoleClosed := false
	newTargetPortfolioProvider := deps.newTargetPortfolioProvider
	if newTargetPortfolioProvider == nil {
		newTargetPortfolioProvider = defaultTargetPortfolioProviderFactory
	}
	newCubeProvider := deps.newCubeProvider
	if newCubeProvider == nil {
		newCubeProvider = defaultTargetPortfolioProviderFactory
	}
	publicationStateEmitted := false
	defer func() {
		if runErr != nil && !publicationStateEmitted {
			if defaultTargetConsole != nil && !defaultTargetConsoleClosed {
				humanOutput.TargetPage("failed", *defaultTargetConsole)
				defaultTargetConsoleClosed = true
			}
			humanOutput.State(
				"Run", "failed",
				"report publication did not complete",
			)
		}
	}()
	if fs.NArg() > 0 {
		if repo != "." || fs.NArg() != 1 {
			return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
		}
		repo = fs.Arg(0)
	}
	resolveGoTarget := deps.resolveGoTarget
	if resolveGoTarget == nil {
		resolveGoTarget = gotarget.Resolve
	}
	goTarget, err := resolveGoTarget(*forcePlatform, os.Getenv)
	if err != nil {
		return fmt.Errorf("--force-platform: %w", err)
	}
	autoGoTarget := deps.precomputedSnapshot == nil && automaticGoTargetAllowed(*forcePlatform, os.Getenv)
	restoreSecretScan := secretscan.SetEnabled(*scanSecrets)
	defer restoreSecretScan()
	if *scanSecrets {
		humanOutput.State("Secret scan", "enabled", "heuristic credential detection: on")
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535")
	}
	if *directCallDepth < 1 {
		return fmt.Errorf("--depth must be at least 1")
	}
	if *directCallEdgeLimit < 1 || *directCallEdgeLimit > surfacediscovery.MaxDirectCallIndexEdges {
		return fmt.Errorf(
			"--edges-limit must be between 1 and %d",
			surfacediscovery.MaxDirectCallIndexEdges,
		)
	}
	gitLabURL := strings.TrimSpace(*gitLabURLFlag)
	gitHubURL := strings.TrimSpace(*gitHubURLFlag)
	if gitLabURL != "" && gitHubURL != "" {
		return fmt.Errorf("--gitlab-url and --github-url cannot be combined")
	}
	staticSourceHost := ""
	if gitLabURL != "" {
		staticSourceHost = "GitLab"
	}
	if gitHubURL != "" {
		staticSourceHost = "GitHub"
	}
	if staticSourceHost != "" {
		*noServe = true
	}
	if err := validateReportModeFlags(*noServe, portExplicit); err != nil {
		return err
	}
	if *noCache {
		humanOutput.State("Cache", "disabled", "cross-run model response reuse: off")
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return fmt.Errorf("resolve repository path: %w", err)
	}
	repo = absRepo
	originIdentity := snapshot.RepositoryOriginIdentity(repo)
	if *noServe && gitLabURL == "" && gitHubURL == "" {
		remoteHost, _, hasProject := strings.Cut(originIdentity, "/")
		switch {
		case hasProject && strings.EqualFold(remoteHost, "github.com"):
			gitHubURL = "https://github.com"
			staticSourceHost = "GitHub"
		case hasProject && strings.EqualFold(remoteHost, "gitlab.com"):
			gitLabURL = "https://gitlab.com"
			staticSourceHost = "GitLab"
		default:
			return fmt.Errorf("--no-serve: could not resolve a GitHub or GitLab repository from origin; specify --github-url URL or --gitlab-url URL")
		}
	}
	if gitLabURL != "" {
		gitLabURL, err = report.ResolveGitLabRepositoryURL(
			gitLabURL,
			originIdentity,
		)
		if err != nil {
			return err
		}
	}
	if gitHubURL != "" {
		gitHubURL, err = report.ResolveGitHubRepositoryURL(
			gitHubURL,
			originIdentity,
		)
		if err != nil {
			return err
		}
	}

	dDir := *debugDir
	if dDir == "" {
		return fmt.Errorf("repomap runs require a nonempty --debug-dir for report authority")
	}
	// Target selection is the first model cube and runs before the per-run
	// artifact writer. Create only the shared root here so its accepted response
	// can be cached on the first run and revalidated on the second.
	if err := os.MkdirAll(dDir, 0o700); err != nil {
		return fmt.Errorf("prepare debug directory: %w", err)
	}

	runID := strings.TrimSpace(deps.runIDOverride)
	if runID == "" {
		runID = debugdump.GenerateRunID(repoRunLabel(repo))
	}
	ctx := deps.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	repositoryName := ""
	if deps.precomputedSnapshot != nil {
		repositoryName = deps.precomputedSnapshot.RepoName
	}
	var repositoryCorpus *corpus.Corpus
	repositoryCorpus, err = corpus.Open(ctx, repo)
	if err != nil {
		return fmt.Errorf("build repository corpus: %w", err)
	}
	defer func() {
		if closeErr := repositoryCorpus.Close(); closeErr != nil && runErr == nil {
			runErr = closeErr
		}
	}()
	captureRepo := deps.captureRepo
	if captureRepo == nil {
		captureRepo = freshness.CaptureRepository
	}
	runDir := filepath.Join(dDir, runID)
	humanOutput.Artifacts(runDir)
	if err := report.RemoveRunManifest(runDir); err != nil {
		return fmt.Errorf("invalidate previous browser report authority: %w", err)
	}
	analysisRoot, err := resolveAnalysisRoot(repo)
	if err != nil {
		return err
	}
	initialState, err := captureRepo(ctx, repo, repositoryCorpus)
	if err != nil {
		return fmt.Errorf("capture repository state before orientation: %w", err)
	}
	if staticSourceHost != "" && repositoryCorpusHasWorkingTreeChanges(repositoryCorpus, initialState) {
		return fmt.Errorf(
			"--no-serve cannot create exact %s source links for tracked working-tree changes; commit or stash those changes, or remove --no-serve to open code through the local VS Code server",
			staticSourceHost,
		)
	}
	initialRepositoryStateSHA256, err := initialState.Digest()
	if err != nil {
		return fmt.Errorf("hash repository state before orientation: %w", err)
	}
	if staticSourceHost != "" && repositoryStateHasAnalyzedSubmodule(initialState) {
		return fmt.Errorf("standalone %s reports do not support analyzed submodule source because one repository URL cannot address it", staticSourceHost)
	}

	var directCallIndex *surfacediscovery.DirectCallIndex
	var externalCallIndex *surfacediscovery.ExternalCallIndex
	var dependencyCatalog *dependencies.Catalog
	var coreObjectIndex *gocoreobject.Index
	var dynamicHandoffIndex *godynamichandoff.Index
	var analysisTarget *analysistarget.Target
	var targetRunContainer *snapshot.TargetRunContainer
	var automaticGoTargetSelection *snapshot.GoTargetSelection
	preparedGoWorkspace := deps.preparedGoWorkspace
	var targetPortfolioOutcome targetPortfolioRunOutcome
	var pythonTargetSelection *pythonTargetRunSelection
	var pythonTargetCatalog *pythontarget.Catalog
	pythonProgramDefault := false
	coreReadmeRoleRows := cloneReadmeRoleLog(deps.coreReadmeRoleRows)
	var firstLayerSemanticObserver *debugdump.SemanticObserver
	analysisTargetOverride := strings.TrimSpace(*analysisTargetFlag)
	explicitGoModuleDir := ""
	if moduleDir, ok := analysistarget.ExactCandidateKeyModuleDir(analysisTargetOverride); ok {
		explicitGoModuleDir = moduleDir
	}
	languageEvidence := repositoryLanguages(repositoryCorpus)
	if deps.precomputedSnapshot == nil && languageEvidence.Python {
		if !languageEvidence.Go {
			selectionExecutor := llm.Executor{RootDir: dDir, Enabled: !*noCache}
			firstLayerSemanticObserver = debugdump.NewSemanticObserver(nil)
			selectionExecutor.Observer = firstLayerSemanticObserver
			selection, selectionErr := selectPythonTargetsForRun(
				ctx,
				repoRunLabel(repo),
				repositoryCorpus,
				analysisTargetOverride,
				humanOutput,
				newTargetPortfolioProvider,
				selectionExecutor,
			)
			if selectionErr != nil {
				flushFailedFirstLayerSemanticJournal(runDir, firstLayerSemanticObserver, humanOutput)
				return selectionErr
			}
			catalog := selection.Catalog.Snapshot()
			pythonTargetCatalog = &catalog
			pythonTargetSelection = &selection
			pythonProgramDefault = true
			targetPortfolioOutcome = selection.Outcome
			coreReadmeRoleRows = cloneReadmeRoleLog(selection.Outcome.ReadmeRoles)
		} else {
			catalog, discoveryErr := discoverPythonTargetCatalogForRun(ctx, repositoryCorpus, humanOutput)
			if discoveryErr != nil {
				return discoveryErr
			}
			ownedCatalog := catalog.Snapshot()
			pythonTargetCatalog = &ownedCatalog
			if analysisTargetOverride != "" {
				pythonTarget, selectionErr := resolveMixedPythonTargetOverride(
					catalog, repositoryCorpus, analysisTargetOverride,
				)
				if selectionErr != nil {
					return selectionErr
				}
				if pythonTarget != nil {
					selectionExecutor := llm.Executor{RootDir: dDir, Enabled: !*noCache}
					firstLayerSemanticObserver = debugdump.NewSemanticObserver(nil)
					selectionExecutor.Observer = firstLayerSemanticObserver
					selection, selectionErr := selectPythonTargetsForRun(
						ctx,
						repoRunLabel(repo),
						repositoryCorpus,
						analysisTargetOverride,
						humanOutput,
						newTargetPortfolioProvider,
						selectionExecutor,
					)
					if selectionErr != nil {
						flushFailedFirstLayerSemanticJournal(runDir, firstLayerSemanticObserver, humanOutput)
						return selectionErr
					}
					selectedCatalog := selection.Catalog.Snapshot()
					pythonTargetCatalog = &selectedCatalog
					pythonTargetSelection = &selection
					pythonProgramDefault = true
					targetPortfolioOutcome = selection.Outcome
					coreReadmeRoleRows = cloneReadmeRoleLog(selection.Outcome.ReadmeRoles)
				}
			}
		}
	}
	opts := orient.Options{
		RepoPath:            repo,
		GoTarget:            goTarget.String(),
		GoModuleDir:         explicitGoModuleDir,
		RepositoryCorpus:    repositoryCorpus,
		AutoGoTarget:        autoGoTarget,
		RunID:               runID,
		DebugDir:            dDir,
		DumpRedacted:        true,
		RequireArtifacts:    true,
		AnalyzeGoProgram:    !pythonProgramDefault,
		SkipGoFacts:         pythonProgramDefault,
		DirectCallDepth:     *directCallDepth,
		DirectCallEdgeLimit: *directCallEdgeLimit,
		PrecomputedSnapshot: deps.precomputedSnapshot,
		PreparedGoWorkspace: preparedGoWorkspace,
		PreparedGoWorkspaceSink: func(workspace *surfacediscovery.PreparedWorkspace) {
			preparedGoWorkspace = workspace
		},
		DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
			directCallIndex = &index
		},
		ExternalCallIndexSink: func(index surfacediscovery.ExternalCallIndex) {
			externalCallIndex = &index
		},
		DependencyCatalogSink: func(catalog dependencies.Catalog) {
			dependencyCatalog = &catalog
		},
		CoreObjectIndexSink: func(index gocoreobject.Index) {
			copyIndex := index.Snapshot()
			coreObjectIndex = &copyIndex
		},
		DynamicHandoffIndexSink: func(index godynamichandoff.Index) {
			copyIndex := index.Snapshot()
			dynamicHandoffIndex = &copyIndex
		},
		AnalysisTargetSink: func(target analysistarget.Target) {
			copyTarget := target.Snapshot()
			analysisTarget = &copyTarget
		},
		TargetRunContainerSink: func(container snapshot.TargetRunContainer) {
			copyContainer := container.Snapshot()
			targetRunContainer = &copyContainer
			if len(container.Targets) > 1 {
				for _, projection := range container.Targets {
					if projection.Target.Ref != container.DefaultTargetRef {
						continue
					}
					context := targetPageConsoleContext{
						DisplayPath: projection.DisplayPath,
						Scope:       analysisTargetSubject(projection.Target),
						RunID:       runID,
						Role:        "default",
					}
					defaultTargetConsole = &context
					humanOutput.TargetPage("started", context)
					break
				}
			}
		},
		GoTargetSelectionSink: func(selection snapshot.GoTargetSelection) {
			owned := selection
			owned.Examples = append([]string(nil), selection.Examples...)
			automaticGoTargetSelection = &owned
		},
		EffectiveOptions: debugdump.EffectiveOptions{
			NoCache:                *noCache,
			GoTarget:               goTarget.String(),
			AnalysisTargetOverride: analysisTargetOverride,
			DirectCallDepth:        *directCallDepth,
			DirectCallEdgeLimit:    *directCallEdgeLimit,
			ScanSecrets:            *scanSecrets,
			GitLabURL:              gitLabURL,
			GitHubURL:              gitHubURL,
			NoOpen:                 *noOpen,
			NoServe:                *noServe,
			Port:                   *port,
			DebugEnabled:           dDir != "",
		},
	}
	if deps.precomputedSnapshot == nil && !pythonProgramDefault {
		opts.AnalysisTargetSelector = func(
			selectorContext context.Context,
			repoName string,
			catalog analysistarget.TargetCatalog,
			facts gofacts.Facts,
		) (snapshot.TargetRunSelection, error) {
			repositoryName = repoName
			selectionExecutor := llm.Executor{RootDir: dDir, Enabled: !*noCache}
			if firstLayerSemanticObserver == nil {
				firstLayerSemanticObserver = debugdump.NewSemanticObserver(nil)
			}
			selectionExecutor.Observer = firstLayerSemanticObserver
			var selection snapshot.TargetRunSelection
			var outcome targetPortfolioRunOutcome
			var selectionErr error
			if analysisTargetOverride == "" && pythonTargetCatalog != nil && len(pythonTargetCatalog.Entries) > 0 {
				mixed, mixedErr := selectMixedTargetsForRun(
					selectorContext, repoName, catalog, facts, *pythonTargetCatalog,
					repositoryCorpus, humanOutput, newTargetPortfolioProvider, selectionExecutor,
				)
				selection, outcome, selectionErr = mixed.Go, mixed.Outcome, mixedErr
			} else {
				selection, outcome, selectionErr = selectTargetsForRun(
					selectorContext, repoName, catalog, facts, repositoryCorpus,
					analysisTargetOverride, humanOutput, newTargetPortfolioProvider, selectionExecutor,
				)
			}
			targetPortfolioOutcome = outcome
			coreReadmeRoleRows = cloneReadmeRoleLog(outcome.ReadmeRoles)
			if selectionErr != nil && analysisTargetOverride != "" &&
				pythonTargetCatalog != nil && len(pythonTargetCatalog.Entries) > 0 {
				return selection, fmt.Errorf("%w; Python choices: %s", selectionErr, pythonTargetChoices(*pythonTargetCatalog))
			}
			return selection, selectionErr
		}
	}
	opts.Progress = humanOutput.Progress

	err = orient.Run(ctx, opts)
	if err != nil && deps.precomputedSnapshot == nil {
		if _, metadataErr := os.Stat(filepath.Join(runDir, "metadata.json")); os.IsNotExist(metadataErr) {
			flushFailedFirstLayerSemanticJournal(runDir, firstLayerSemanticObserver, humanOutput)
		}
	}
	if automaticGoTargetSelection != nil {
		selected, parseErr := gotarget.Parse(automaticGoTargetSelection.Target)
		if parseErr != nil {
			return fmt.Errorf("restore automatic Go target selection: %w", parseErr)
		}
		goTarget = selected
	}
	metadataPath := filepath.Join(runDir, "metadata.json")
	if deps.precomputedSnapshot == nil {
		if _, metadataErr := os.Stat(metadataPath); metadataErr == nil {
			flushFirstLayerSemanticJournal(runDir, firstLayerSemanticObserver, humanOutput)
			if diagnosticErr := recordTargetPortfolioOutcome(runDir, targetPortfolioOutcome, humanOutput); diagnosticErr != nil {
				if err != nil {
					return errors.Join(err, diagnosticErr)
				}
				return diagnosticErr
			}
		}
	}
	if err != nil {
		if _, metadataErr := os.Stat(metadataPath); metadataErr == nil {
			return fmt.Errorf("%w\nrequest diagnostics: %s", err, metadataPath)
		}
		return err
	}
	boundReadmeRoles, err := restoreReadmeRoleResult(repositoryCorpus, coreReadmeRoleRows)
	if err != nil {
		return fmt.Errorf("bind accepted README file-role authority to this run: %w", err)
	}
	coreReadmeRoleRows = compileReadmeRoleLog(repositoryCorpus, boundReadmeRoles)
	if err := persistReadmeRoleAuthority(runDir, coreReadmeRoleRows); err != nil {
		return err
	}
	if deps.precomputedSnapshot == nil && targetRunContainer == nil && pythonTargetSelection == nil {
		return fmt.Errorf("no advertised Go targets; choose a Go repository or select a supported platform with --force-platform GOOS/GOARCH")
	}
	if pythonTargetSelection != nil && !pythonProgramDefault {
		return fmt.Errorf("selected Python targets require Python-default semantic authority")
	}
	if targetPortfolioOutcome.SelectedTargets > 0 {
		if pythonProgramDefault {
			if pythonTargetSelection == nil {
				return fmt.Errorf("selected Python target portfolio is unavailable")
			}
			if pythonTargetSelection.Default.Ref != targetPortfolioOutcome.SelectedRef ||
				len(pythonTargetSelection.Targets) != targetPortfolioOutcome.SelectedTargets ||
				!exactRefSet(pythonTargetRefs(pythonTargetSelection.Targets), targetPortfolioOutcome.SelectedTargetRefs) {
				return fmt.Errorf("selected Python target portfolio does not match its exact targets")
			}
		} else {
			if targetRunContainer == nil {
				return fmt.Errorf("selected target portfolio was not bound into the run container")
			}
			if err := targetRunContainer.Validate(); err != nil {
				return fmt.Errorf("validate selected target run container: %w", err)
			}
			if targetRunContainer.DefaultTargetRef != targetPortfolioOutcome.SelectedRef ||
				len(targetRunContainer.Targets) != targetPortfolioOutcome.SelectedTargets ||
				!targetRunContainerHasExactRefs(*targetRunContainer, targetPortfolioOutcome.SelectedTargetRefs) {
				return fmt.Errorf("selected target portfolio does not match the run container")
			}
		}
	}
	if analysisTarget != nil && !pythonProgramDefault {
		targetDisplay := analysisTarget.DisplayPath()
		targetSubject := analysisTargetSubject(*analysisTarget)
		if directCallIndex == nil {
			return fmt.Errorf(
				"Go call analysis is unavailable for target %s under %s; choose the correct platform with --force-platform GOOS/GOARCH (diagnostics: %s)",
				targetDisplay, goTarget.String(), filepath.Join(runDir, "metadata.json"),
			)
		}
		if err := directCallIndex.Validate(); err != nil {
			return fmt.Errorf("validate Go call analysis for target %s: %w", targetDisplay, err)
		}
		if !analysisTarget.MatchesDirectCallIndexScope(
			directCallIndex.Scope, *directCallDepth, *directCallEdgeLimit,
		) {
			return fmt.Errorf(
				"Go call analysis scope does not match target %s and requested --depth/--edges-limit",
				targetSubject,
			)
		}
		if directCallIndex.State != surfacediscovery.DirectCallIndexReady {
			switch directCallIndex.ClosedReason {
			case surfacediscovery.DirectCallIndexClosedSSAUnavailable:
				return fmt.Errorf(
					"Go SSA is unavailable for target %s under %s; choose the correct platform with --force-platform GOOS/GOARCH",
					targetDisplay, goTarget.String(),
				)
			case surfacediscovery.DirectCallIndexClosedNodeLimit:
				return fmt.Errorf(
					"Go call analysis for target %s exceeds the %d-function declaration safety bound; choose a narrower target with --target (the --depth option limits edges, not the exact symbol catalog)",
					targetDisplay, surfacediscovery.MaxDirectCallIndexNodes,
				)
			case surfacediscovery.DirectCallIndexClosedEdgeLimit:
				return directCallEdgeCeilingError(
					targetSubject, *directCallDepth, *directCallEdgeLimit,
					directCallIndex.Coverage.EdgeLimitSafeDepth,
				)
			default:
				return fmt.Errorf("Go call analysis for target %s is unavailable", targetDisplay)
			}
		}
	}
	if pythonTargetCatalog != nil {
		if err := pythontarget.PersistCatalog(runDir, *pythonTargetCatalog); err != nil {
			return err
		}
	}
	programIndexes := make([]programindex.Index, 0)
	programIndexFilenames := make([]string, 0)
	programDefaultTargetID := ""
	if pythonTargetSelection != nil {
		programIndexStarted := time.Now()
		representatives, representativeErr := pythonProgramRepresentatives(*pythonTargetSelection)
		if representativeErr != nil {
			return representativeErr
		}
		indexes, indexErr := pythonprogramindex.BuildMany(ctx, repositoryCorpus, representatives)
		if indexErr != nil {
			return fmt.Errorf("build Python program index: %w", indexErr)
		}
		indexFilenames := make([]string, len(indexes))
		for position, index := range indexes {
			isDefault := representatives[position].Ref == pythonTargetSelection.Default.Ref
			indexFilenames[position] = programindex.ArtifactFilenameForTarget(representatives[position].Ref, isDefault)
			if err := programindex.Persist(
				runDir,
				indexFilenames[position],
				index,
			); err != nil {
				return err
			}
		}
		programDefaultTargetID = indexes[0].Target.ID
		programIndexes = append(programIndexes, indexes...)
		programIndexFilenames = append(programIndexFilenames, indexFilenames...)
		programScopes, scopeErr := pythonProgramScopeCount(representatives)
		if scopeErr != nil {
			return scopeErr
		}
		programDetails := []string{
			fmt.Sprintf("Python program scopes: %d", programScopes),
			fmt.Sprintf("selected target views: %d", len(pythonTargetSelection.Targets)),
			formatRunOutputWallDuration(time.Since(programIndexStarted)),
		}
		programDetails = append(programDetails, "default artifact: program-index.json")
		humanOutput.State("Program index", "ready", programDetails...)
	}
	if !pythonProgramDefault && analysisTarget != nil && repositoryCorpus != nil && directCallIndex != nil {
		programIndexStarted := time.Now()
		if externalCallIndex == nil {
			return fmt.Errorf("Go program index requires the exact external-call index")
		}
		if coreObjectIndex == nil {
			return fmt.Errorf("Go program index requires the exact target core-object index")
		}
		if dynamicHandoffIndex == nil {
			return fmt.Errorf("Go program index requires the exact dynamic-handoff index")
		}
		index, indexErr := goadapter.Build(
			repositoryCorpus,
			*analysisTarget,
			*directCallIndex,
			*externalCallIndex,
			*coreObjectIndex,
			*dynamicHandoffIndex,
		)
		if indexErr != nil {
			return fmt.Errorf("build Go program index: %w", indexErr)
		}
		if err := programindex.Persist(runDir, "", index); err != nil {
			return err
		}
		programDefaultTargetID = index.Target.ID
		programIndexes = append(programIndexes, index)
		programIndexFilenames = append(programIndexFilenames, programindex.ArtifactFilename)
		humanOutput.State(
			"Program index", "ready",
			fmt.Sprintf("objects: %d", len(index.Objects)),
			fmt.Sprintf("relations: %d", len(index.Relations)),
			formatRunOutputWallDuration(time.Since(programIndexStarted)),
			"artifact: program-index.json",
		)
	}
	if len(programIndexes) == 0 {
		return fmt.Errorf("ordinary analysis produced no exact program index")
	}
	if strings.TrimSpace(programDefaultTargetID) == "" {
		return fmt.Errorf("ordinary analysis produced program indexes without an exact default target")
	}
	indexSet, setErr := programindex.BuildArtifactSet(
		programDefaultTargetID,
		programIndexes,
		programIndexFilenames,
	)
	if setErr != nil {
		return setErr
	}
	if err := programindex.PersistArtifactSet(runDir, indexSet); err != nil {
		return err
	}
	defaultProgramIndex, defaultIndexErr := programindex.ExactIndexByTargetID(
		programIndexes, programDefaultTargetID,
	)
	if defaultIndexErr != nil {
		return defaultIndexErr
	}
	if analysisTarget != nil && !pythonProgramDefault {
		targetDetails := []string{
			"kind: " + string(analysisTarget.Kind),
			"scope: " + analysisTargetSubject(*analysisTarget),
		}
		if targetPortfolioOutcome.SemanticState == debugdump.SemanticStateAccepted ||
			targetPortfolioOutcome.SemanticState == debugdump.SemanticStateCacheHit {
			targetDetails = append(
				targetDetails,
				fmt.Sprintf("target entry files selected: %d", targetPortfolioOutcome.SelectedFileRefs),
				fmt.Sprintf("unclassified file hypotheses dropped: %d", targetPortfolioOutcome.UnclassifiedFiles),
				fmt.Sprintf("exact analysis targets restored: %d", targetPortfolioOutcome.SelectedTargets),
			)
		}
		humanOutput.State(
			"Analysis target", analysisTarget.DisplayPath(),
			targetDetails...,
		)
	}
	if pythonTargetSelection != nil {
		targetDetails := []string{
			"language: python",
			"kind: " + string(pythonTargetSelection.Default.Kind),
			fmt.Sprintf("exact analysis targets restored: %d", len(pythonTargetSelection.Targets)),
		}
		if targetPortfolioOutcome.SelectedFileRefs > 0 {
			targetDetails = append(
				targetDetails,
				fmt.Sprintf("target entry files selected: %d", targetPortfolioOutcome.SelectedFileRefs),
				fmt.Sprintf("unclassified file hypotheses dropped: %d", targetPortfolioOutcome.UnclassifiedFiles),
			)
		}
		humanOutput.State(
			"Analysis target", pythonTargetSelection.Default.DisplayName,
			targetDetails...,
		)
	}
	if pythonProgramDefault {
		if pythonTargetSelection == nil {
			return fmt.Errorf("Python semantic cubes require the exact target selection authority")
		}
		declaredDependencies, declarationErr := runPythonDeclaredDependenciesForRun(
			ctx,
			runDir,
			repositoryCorpus,
			pythonTargetSelection.Catalog,
			pythonTargetSelection.Default,
			defaultProgramIndex,
			humanOutput,
		)
		if declarationErr != nil {
			return declarationErr
		}
		dependencyCatalogStarted := time.Now()
		catalog, catalogErr := pythondependencies.Build(defaultProgramIndex)
		if catalogErr != nil {
			return fmt.Errorf("build Python dependency catalog: %w", catalogErr)
		}
		if err := dependencies.Persist(runDir, catalog); err != nil {
			return err
		}
		dependencyCatalog = &catalog
		humanOutput.State(
			"Python dependencies", "ready",
			fmt.Sprintf("direct dependency packages: %d", len(catalog.Dependencies)),
			formatRunOutputWallDuration(time.Since(dependencyCatalogStarted)),
			"artifact: "+dependencies.ArtifactFilename,
		)
		if catalog.Coverage.State != dependencies.CoverageComplete {
			return pythonDependencyCoverageError(catalog)
		}

		semanticResult, semanticErr := runSemanticPipelineForRun(
			ctx,
			runDir,
			dDir,
			*noCache,
			stopAfter,
			"Python",
			pipeline.Authorities{
				RepositoryName: repoRunLabel(repo),
				Repository:     repositoryCorpus,
				ProgramIndex:   defaultProgramIndex,
				Dependencies:   catalog,
				Declarations:   &declaredDependencies,
				ReadmeRoles:    boundReadmeRoles,
			},
			humanOutput,
			newCubeProvider,
		)
		if semanticErr != nil {
			return semanticErr
		}
		if semanticResult.StoppedAfter != "" {
			humanOutput.State(
				"Run", "stopped",
				"requested checkpoint: STOP_AFTER=ActivityEntrypoints",
				"last artifact: "+activityentrypoint.ArtifactFilename,
			)
			return nil
		}
	}
	if !pythonProgramDefault {
		if analysisTarget == nil {
			return fmt.Errorf("analysis cubes require one exact Go analysis target")
		}
		if strings.TrimSpace(repositoryName) == "" {
			return fmt.Errorf("analysis cubes require the semantic repository name")
		}
		if repositoryCorpus == nil {
			return fmt.Errorf("analysis cubes require the repository corpus")
		}
		if dependencyCatalog == nil {
			return fmt.Errorf("analysis cubes require the target-scoped dependency catalog")
		}
		if err := dependencies.Persist(runDir, *dependencyCatalog); err != nil {
			return err
		}
		semanticResult, semanticErr := runSemanticPipelineForRun(
			ctx,
			runDir,
			dDir,
			*noCache,
			stopAfter,
			"Go",
			pipeline.Authorities{
				RepositoryName: repositoryName,
				Repository:     repositoryCorpus,
				ProgramIndex:   defaultProgramIndex,
				Dependencies:   *dependencyCatalog,
				Declarations:   nil,
				ReadmeRoles:    boundReadmeRoles,
			},
			humanOutput,
			newCubeProvider,
		)
		if semanticErr != nil {
			return semanticErr
		}
		if semanticResult.StoppedAfter != "" {
			humanOutput.State(
				"Run", "stopped",
				"requested checkpoint: STOP_AFTER=ActivityEntrypoints",
				"last artifact: "+activityentrypoint.ArtifactFilename,
			)
			return nil
		}
	}
	var reportPath string
	reconciliationStarted := time.Now()
	humanOutput.Stage("Repository authority", "reconciling captured inputs")
	reportData, err := report.ReadRunDir(runDir)
	if err != nil {
		return fmt.Errorf("read captured report inputs: %w", err)
	}
	capturedInputPaths, err := report.CapturedInputPaths(reportData)
	if err != nil {
		return fmt.Errorf("collect captured report inputs: %w", err)
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx, analysisRoot, initialState, capturedInputPaths,
	)
	if err != nil {
		return fmt.Errorf("confirm browser report authority: %w", err)
	}
	humanOutput.State(
		"Repository authority", "confirmed",
		fmt.Sprintf("captured inputs: %d", len(capturedInputPaths)),
		formatRunOutputWallDuration(time.Since(reconciliationStarted)),
	)
	generateAuthorizedReport := func() error {
		if gitLabURL != "" {
			return report.GenerateAuthorizedGitLab(runDir, authority, gitLabURL)
		}
		if gitHubURL != "" {
			return report.GenerateAuthorizedGitHub(runDir, authority, gitHubURL)
		}
		return report.GenerateAuthorized(runDir, authority)
	}
	reportStarted := time.Now()
	humanOutput.Stage("Report", "generating authorized Program report")
	if err := generateAuthorizedReport(); err != nil {
		return fmt.Errorf("generate authorized browser report: %w", err)
	}
	reportPath = filepath.Join(runDir, "report.html")
	multiTargetPublication := !deps.siblingTargetRun && targetRunContainer != nil && len(targetRunContainer.Targets) > 1
	if !deps.siblingTargetRun && !multiTargetPublication {
		humanOutput.State(
			"Report", "generated",
			formatRunOutputWallDuration(time.Since(reportStarted)),
		)
		humanOutput.Stage("Report", "path: "+reportPath)
	}
	if !deps.siblingTargetRun && !multiTargetPublication && staticSourceHost != "" {
		humanOutput.Stage(
			"Report",
			fmt.Sprintf("standalone host: %s", staticSourceHost),
			"captured revision: "+initialState.Head,
			"remote availability is not checked; ensure the captured commit is pushed before sharing",
		)
	}
	publication, err := report.AssessRunPublication(runDir)
	if err != nil {
		return errors.Join(
			fmt.Errorf("verify generated report publication: %w", err),
			quarantineTargetPagePublication([]string{runDir}),
		)
	}
	publishedTarget := targetPublishedRun{
		RunID:                 runID,
		RunDir:                runDir,
		Authority:             authority,
		RepositoryStateSHA256: initialRepositoryStateSHA256,
		SelectedRevision:      initialState.Head,
		GoTarget:              goTarget.String(),
		GitLabURL:             gitLabURL,
		GitHubURL:             gitHubURL,
	}
	if automaticGoTargetSelection != nil {
		publishedTarget.GoTargetSource = automaticGoTargetSelection.Source
		publishedTarget.GoTargetBaseline = automaticGoTargetSelection.Baseline
	}
	if analysisTarget != nil {
		publishedTarget.Target = analysisTarget.Snapshot()
	}
	if deps.publishedTargetSink != nil {
		deps.publishedTargetSink(publishedTarget)
	}
	if defaultTargetConsole != nil && !defaultTargetConsoleClosed && !multiTargetPublication {
		humanOutput.TargetPage("complete", *defaultTargetConsole)
		defaultTargetConsoleClosed = true
	}
	if !deps.siblingTargetRun && targetRunContainer != nil && len(targetRunContainer.Targets) > 1 {
		portfolioDeps := deps
		portfolioDeps.coreReadmeRoleRows = cloneReadmeRoleLog(coreReadmeRoleRows)
		portfolioDeps.preparedGoWorkspace = preparedGoWorkspace
		publication, err = publishTargetPagePortfolio(
			repo,
			extraArgs,
			portfolioDeps,
			*targetRunContainer,
			publishedTarget,
			humanOutput,
		)
		if err != nil {
			return fmt.Errorf("publish selected target pages: %w", err)
		}
		if defaultTargetConsole != nil && !defaultTargetConsoleClosed {
			humanOutput.TargetPage("complete", *defaultTargetConsole)
			defaultTargetConsoleClosed = true
		}
		humanOutput.State(
			"Report", "generated",
			formatRunOutputWallDuration(time.Since(reportStarted)),
		)
		humanOutput.Stage("Report", "path: "+reportPath)
		if staticSourceHost != "" {
			humanOutput.Stage(
				"Report",
				fmt.Sprintf("standalone host: %s", staticSourceHost),
				"captured revision: "+initialState.Head,
				"remote availability is not checked; ensure the captured commit is pushed before sharing",
			)
		}
	}
	if !deps.siblingTargetRun {
		linkLatest(dDir, runDir, runOutputWarningSink{
			output: humanOutput, summary: "could not update latest report link",
		})
	}
	if deps.siblingTargetRun {
		return nil
	}
	publicationDetails := []string{"report: " + reportPath}
	if publication.Status != report.PublicationReady {
		publicationDetails = append(publicationDetails, "report or analysis artifacts are missing or invalid")
	}
	humanOutput.State("Run", strings.ToLower(string(publication.Status)), publicationDetails...)
	publicationStateEmitted = true

	if !*noServe && deps.serveReport != nil {
		return deps.serveReport(ctx, reportserver.Options{
			RunsDir:      dDir,
			InitialRunID: runID,
			Port:         *port,
			Logf: func(format string, args ...any) {
				humanOutput.Stage("Server", fmt.Sprintf(format, args...))
			},
			OnReady: func(url string) error {
				humanOutput.State("Server", "ready", "url: "+url, "Ctrl-C to stop")
				if !*noOpen && deps.openReport != nil {
					if err := deps.openReport(url); err != nil {
						humanOutput.Warn("could not open report", err.Error())
					}
				}
				return nil
			},
		})
	}
	if !*noOpen && deps.openReport != nil {
		if err := deps.openReport(reportPath); err != nil {
			humanOutput.Warn("could not open report", err.Error())
		}
	}
	return nil
}

func validateReportModeFlags(noServe bool, portExplicit bool) error {
	if noServe && portExplicit {
		return errors.New(
			"--port configures the local report server and cannot be combined with static report mode; remove --port, or remove --no-serve/--github-url/--gitlab-url to serve locally",
		)
	}
	return nil
}

func semanticStopAfter(value string) (pipeline.Stage, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "":
		return "", nil
	case "ActivityEntrypoints":
		return pipeline.StageActivityEntrypoints, nil
	default:
		return "", fmt.Errorf(
			"invalid STOP_AFTER=%q: supported checkpoint is ActivityEntrypoints; unset STOP_AFTER to run the complete analysis",
			value,
		)
	}
}

func actionableFlagError(args []string, parseErr error) error {
	guidance := map[string]string{
		"offline":         "repomap is online-only; remove --offline",
		"all-targets":     "every eligible target is analyzed by default; remove --all-targets",
		"go-target":       "use --force-platform GOOS/GOARCH",
		"strict-snapshot": "repository changes are allowed during analysis; remove --strict-snapshot",
		"source-episode":  "source episodes were removed; remove --source-episode",
		"no-secrets":      "heuristic scanning is off by default; remove --no-secrets or use --scan-secrets to enable it",
		"lang":            "reports are currently canonical English; remove --lang",
	}
	for _, argument := range args {
		name := strings.TrimLeft(strings.SplitN(argument, "=", 2)[0], "-")
		if hint, ok := guidance[name]; ok {
			return fmt.Errorf("invalid flag --%s: %s", name, hint)
		}
	}
	return fmt.Errorf("%w; run 'repomap --help' for supported flags", parseErr)
}

func targetRunContainerHasExactRefs(container snapshot.TargetRunContainer, expected []string) bool {
	actual := make([]string, len(container.Targets))
	for index, projection := range container.Targets {
		actual[index] = projection.Target.Ref
	}
	return exactRefSet(actual, expected)
}

func exactRefSet(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	remaining := make(map[string]struct{}, len(expected))
	for _, ref := range expected {
		if strings.TrimSpace(ref) == "" {
			return false
		}
		if _, duplicate := remaining[ref]; duplicate {
			return false
		}
		remaining[ref] = struct{}{}
	}
	for _, ref := range actual {
		if _, ok := remaining[ref]; !ok {
			return false
		}
		delete(remaining, ref)
	}
	return len(remaining) == 0
}

func automaticGoTargetAllowed(explicit string, getenv func(string) string) bool {
	if explicit != "" {
		return false
	}
	if getenv == nil {
		return true
	}
	return getenv("GOOS") == "" && getenv("GOARCH") == ""
}

func directCallEdgeCeilingError(target string, depth, edgeLimit, safeDepth int) error {
	lines := []string{fmt.Sprintf(
		"Go call analysis for target %s exceeded --edges-limit=%d at --depth=%d",
		target, edgeLimit, depth,
	)}
	if safeDepth >= 1 && safeDepth < depth {
		lines = append(lines, fmt.Sprintf(
			"you can decrease depth via --depth %d (default %d; this depth is known to fit the current edge ceiling)",
			safeDepth, surfacediscovery.DefaultDirectCallDepth,
		))
	}
	if edgeLimit < surfacediscovery.MaxDirectCallIndexEdges {
		next := edgeLimit * 2
		if next <= edgeLimit || next > surfacediscovery.MaxDirectCallIndexEdges {
			next = surfacediscovery.MaxDirectCallIndexEdges
		}
		lines = append(lines, fmt.Sprintf(
			"to preserve depth, try --edges-limit %d (default %d; maximum %d; the full edge count is not computed after the safety stop)",
			next, surfacediscovery.DefaultDirectCallEdgeLimit, surfacediscovery.MaxDirectCallIndexEdges,
		))
	}
	lines = append(lines,
		"the retry rebuilds local SSA and the call index; the standard Go build cache remains reusable, and no provider call was made",
	)
	return errors.New(strings.Join(lines, "\n"))
}

func repositoryStateHasAnalyzedSubmodule(state freshness.RepositoryState) bool {
	for _, submodule := range state.Submodules {
		if submodule.IncludedInAnalysis {
			return true
		}
	}
	return false
}

func repositoryCorpusHasWorkingTreeChanges(repository *corpus.Corpus, state freshness.RepositoryState) bool {
	if repository == nil {
		return false
	}
	for _, dirty := range state.Dirty {
		if _, ok := repository.ID(filepath.ToSlash(dirty.Path)); ok {
			return true
		}
		if dirty.FromPath != "" {
			if _, ok := repository.ID(filepath.ToSlash(dirty.FromPath)); ok {
				return true
			}
		}
	}
	return false
}

func defaultDebugDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheDir) == "" {
		return ""
	}
	return filepath.Join(cacheDir, "repomap", "runs")
}

func resolveAnalysisRoot(repositoryPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(repositoryPath)
	if err != nil {
		return "", fmt.Errorf("resolve analysis root: %w", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve analysis root: %w", err)
	}
	root := filepath.Clean(absolute)
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect analysis root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("analysis root is not a directory: %s", root)
	}
	return root, nil
}

func repoRunLabel(repo string) string {
	absPath, err := filepath.Abs(repo)
	if err == nil {
		return filepath.Base(absPath)
	}
	return filepath.Base(filepath.Clean(repo))
}

func printUsage() {
	printUsageTo(os.Stderr)
}

func printUsageTo(writer io.Writer) {
	fmt.Fprintf(writer, "Usage: repomap [repo] [flags]\n")
	fmt.Fprintf(writer, "       repomap cache clear [--debug-dir DIR]\n")
	fmt.Fprintf(writer, "\nFlags:\n")
	fmt.Fprintf(writer, "  --target TARGET             analyze exactly one explicit target\n")
	fmt.Fprintf(writer, "  --force-platform GOOS/GOARCH override normal Go platform selection\n")
	fmt.Fprintf(writer, "  --depth N                   target call-graph depth (default: %d)\n", surfacediscovery.DefaultDirectCallDepth)
	fmt.Fprintf(writer, "  --edges-limit N             maximum exact target call-graph edges (default: %d)\n", surfacediscovery.DefaultDirectCallEdgeLimit)
	fmt.Fprintf(writer, "  --github-url URL            static report source host\n")
	fmt.Fprintf(writer, "  --gitlab-url URL            static report source host\n")
	fmt.Fprintf(writer, "  --no-open                   do not open the report\n")
	fmt.Fprintf(writer, "  --no-serve                  write static HTML with remote source links\n")
	fmt.Fprintf(writer, "  --port PORT                 local report server port (default: random)\n")
	fmt.Fprintf(writer, "  --debug-dir DIR             report and cache directory\n")
	fmt.Fprintf(writer, "  --no-cache                  bypass persistent model-response caches\n")
	fmt.Fprintf(writer, "  --scan-secrets              enable heuristic credential scanning\n")
	fmt.Fprintf(writer, "  --help, -h                  show this help\n")
	fmt.Fprintf(writer, "  --version                   show version\n")
	fmt.Fprintf(writer, "\nEnvironment:\n")
	fmt.Fprintf(writer, "  REPOMAP_LLM_ENDPOINT full OpenAI-compatible chat/completions URL\n")
	fmt.Fprintf(writer, "  REPOMAP_LLM_MODEL\n")
	fmt.Fprintf(writer, "  REPOMAP_LLM_API_KEY (for bearer auth)\n")
	fmt.Fprintf(writer, "  REPOMAP_LLM_AUTH    bearer (default) or none\n")
	fmt.Fprintf(writer, "  REPOMAP_LLM_TIMEOUT (default 10m)\n")
	fmt.Fprintf(writer, "  DEEPSEEK_API_KEY    quick setup; defaults to deepseek-v4-flash\n")
	fmt.Fprintf(writer, "  DEEPSEEK_*          compatibility configuration aliases\n")
	fmt.Fprintf(writer, "\nExamples:\n")
	fmt.Fprintf(writer, "  repomap\n")
	fmt.Fprintf(writer, "  repomap ../etcd\n")
	fmt.Fprintf(writer, "  repomap ../etcd --no-serve\n")
	fmt.Fprintf(writer, "  repomap ../etcd --no-serve --github-url https://github.com/etcd-io/etcd\n")
	fmt.Fprintf(writer, "  repomap cache clear\n")
}
