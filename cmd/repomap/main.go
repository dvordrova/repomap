package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/targetoutcome"
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
	repositoryArgumentOmitted := true
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repo = args[0]
		args = args[1:]
		repositoryArgumentOmitted = false
	}
	if err := runDefault(repo, args, repositoryArgumentOmitted); err != nil {
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

func runDefault(repo string, extraArgs []string, repositoryArgumentOmitted bool) error {
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
		llmBatchConcurrency:        llm.DefaultBatchConcurrency,
		llmBatchController:         &llm.BatchController{},
		repositoryArgumentOmitted:  repositoryArgumentOmitted,
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
	runDocumentationReduce     documentationReduceRunner
	runProgramCategorization   programCategorizationRunner
	runProgramGrouping         programGroupingRunner
	runGroupMatching           groupMatchingRunner
	runOrientation             orientationRunner
	// One controller follows the complete repository run, including selected
	// child targets and the repository overview. DeepSeek concurrency limits are
	// account-scoped, so a transient HTTP 429 must serialize later attempts even
	// when a later stage creates a fresh provider adapter instance.
	llmBatchConcurrency int
	llmBatchController  *llm.BatchController
	// preselectedTarget is one exact page from the outer repository target
	// plan. It bypasses every first-layer scout and portfolio model call.
	preselectedTarget      *repositoryTypedTarget
	preselectedProgramPage *repositoryProgramPageAuthority
	reducedDocumentation   *documentationreduce.Result
	resolveGoTarget        func(string, func(string) string) (gotarget.Target, error)
	// goBuildTags is parsed once by the outer ordinary run and copied into every
	// selected child page. A bound empty slice is distinct from an unbound value
	// so sibling pages never reread a changing process environment.
	goBuildTags      []string
	goBuildTagsBound bool
	// sharedRepositoryCorpus and capturedRepositoryState let an outer
	// multi-language target dispatcher keep one tracked-file namespace and one
	// repository authority across every exact target page. Child pages borrow
	// the corpus and never close it.
	sharedRepositoryCorpus  *corpus.Corpus
	capturedRepositoryState *freshness.RepositoryState
	// coreReadmeRoleRows carries the one accepted first-layer README role
	// catalog into every selected target page. Each run rebinds paths to its
	// current corpus, so run-local f* identities are never reused blindly.
	coreReadmeRoleRows []readmeRoleLogRow
	runIDOverride      string
	siblingTargetRun   bool
	// deferredPortfolioHTML marks a target-local backing run in the repository
	// ProgramPagePortfolio. It leaves report generation and the sole browser HTML
	// to the complete portfolio transaction, including for a one-target run.
	deferredPortfolioHTML bool
	publishedTargetSink   func(targetPublishedRun)
	// targetOutcomeStageSink exposes only the current closed per-target
	// boundary to the outer dispatcher. It never changes execution semantics;
	// it lets a contained failure be reported without matching error strings.
	targetOutcomeStageSink func(targetoutcome.Stage)
	// repositoryArgumentOmitted is true only when the user left [repo] out of
	// the command. Internal child pages retain the outer invocation choice.
	repositoryArgumentOmitted bool
}

func runDefaultWithDeps(repo string, extraArgs []string, deps defaultRunDeps) (runErr error) {
	if deps.llmBatchConcurrency < 1 {
		// Keep direct test/dependency injection sequential unless it explicitly
		// opts into the product pool. runDefault sets the ordinary product limit.
		deps.llmBatchConcurrency = 1
	}
	if deps.llmBatchController == nil {
		deps.llmBatchController = &llm.BatchController{}
	}
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	forcePlatform := fs.String("force-platform", "", "force Go platform as GOOS/GOARCH")
	analysisTargetFlag := fs.String("target", "", "analysis surface (unambiguous advertised path or exact target key)")
	directCallDepth := fs.Int(
		"depth", surfacediscovery.DefaultDirectCallDepth,
		"target call-graph depth (0 keeps all reachable calls)",
	)
	directCallEdgeLimit := fs.Int(
		"edges-limit", surfacediscovery.DefaultDirectCallEdgeLimit,
		"maximum exact target call-graph edges (0 keeps all edges)",
	)
	noCache := fs.Bool("no-cache", false, "disable cross-run model response caches")
	scanSecrets := fs.Bool("scan-secrets", false, "scan repository and model material for credential-like text")
	gitLabURLFlag := fs.String("gitlab-url", "", "create a standalone report with GitLab source links; does not select a repository")
	gitHubURLFlag := fs.String("github-url", "", "create a standalone report with GitHub source links; does not select a repository")
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
	newTargetPortfolioProvider := deps.newTargetPortfolioProvider
	if newTargetPortfolioProvider == nil {
		newTargetPortfolioProvider = defaultTargetPortfolioProviderFactory
	}
	if deps.newCubeProvider == nil {
		deps.newCubeProvider = defaultTargetPortfolioProviderFactory
	}
	newCubeProvider := deps.newCubeProvider
	publicationStateEmitted := false
	defer func() {
		if runErr != nil && !publicationStateEmitted && !deps.siblingTargetRun {
			humanOutput.State(
				"Run", "failed",
				"report publication did not complete",
			)
		}
	}()
	var err error
	repo, deps.repositoryArgumentOmitted, err = bindParsedRepositoryArgument(
		repo,
		deps.repositoryArgumentOmitted,
		fs.Args(),
	)
	if err != nil {
		return err
	}
	resolveGoTarget := deps.resolveGoTarget
	if resolveGoTarget == nil {
		resolveGoTarget = gotarget.Resolve
	}
	goTarget, err := resolveGoTarget(*forcePlatform, os.Getenv)
	if err != nil {
		return fmt.Errorf("--force-platform: %w", err)
	}
	autoGoTarget := automaticGoTargetAllowed(*forcePlatform, os.Getenv)
	restoreSecretScan := secretscan.SetEnabled(*scanSecrets)
	defer restoreSecretScan()
	if *scanSecrets {
		humanOutput.State("Secret scan", "enabled", "heuristic credential detection: on")
	}
	if *port < 0 || *port > reportserver.MaxTCPPort {
		return fmt.Errorf("--port must be between 0 and %d", reportserver.MaxTCPPort)
	}
	if err := validateDirectCallControls(*directCallDepth, *directCallEdgeLimit); err != nil {
		return err
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
		if err := validateImplicitRepositorySourceLink(
			"--gitlab-url",
			gitLabURL,
			originIdentity,
			deps.repositoryArgumentOmitted,
		); err != nil {
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
		if err := validateImplicitRepositorySourceLink(
			"--github-url",
			gitHubURL,
			originIdentity,
			deps.repositoryArgumentOmitted,
		); err != nil {
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
	var repositoryCorpus *corpus.Corpus
	ownsRepositoryCorpus := deps.sharedRepositoryCorpus == nil
	if deps.sharedRepositoryCorpus != nil {
		repositoryCorpus = deps.sharedRepositoryCorpus
		if err := repositoryCorpus.Snapshot().Validate(); err != nil {
			return fmt.Errorf("bind shared repository corpus: %w", err)
		}
	} else {
		repositoryCorpus, err = corpus.Open(ctx, repo)
		if err != nil {
			return fmt.Errorf("build repository corpus: %w", err)
		}
		defer func() {
			if closeErr := repositoryCorpus.Close(); closeErr != nil && runErr == nil {
				runErr = closeErr
			}
		}()
	}
	if ownsRepositoryCorpus {
		defer reportCorpusScaleWarnings(humanOutput, repositoryCorpus)
	}
	languageEvidence := repositoryLanguages(repositoryCorpus)
	targetOverride := strings.TrimSpace(*analysisTargetFlag)
	goBuildTags := append([]string(nil), deps.goBuildTags...)
	var deferredGoBuildTagsErr error
	if !deps.goBuildTagsBound {
		goBuildTags, deferredGoBuildTagsErr, err = prepareGoBuildTags(
			languageEvidence.Go,
			os.Getenv("GOTAGS"),
			targetOverride,
		)
		if err != nil {
			return fmt.Errorf("GOTAGS: %w", err)
		}
		deps.goBuildTags = append([]string(nil), goBuildTags...)
		deps.goBuildTagsBound = true
	}
	if deps.preselectedTarget == nil {
		reportGoBuildTagScaleWarnings(humanOutput, goBuildTags)
	}
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
	var initialState freshness.RepositoryState
	if deps.capturedRepositoryState != nil {
		initialState = cloneRepositoryState(*deps.capturedRepositoryState)
		if err := initialState.Validate(); err != nil {
			return fmt.Errorf("bind captured repository state: %w", err)
		}
	} else {
		initialState, err = captureRepo(ctx, repo, repositoryCorpus)
		if err != nil {
			return fmt.Errorf("capture repository state before orientation: %w", err)
		}
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
	if deps.preselectedTarget == nil && !deps.siblingTargetRun {
		goSource, prepareErr := prepareRepositoryPlanningGoSource(
			languageEvidence.Go,
			targetOverride,
			func() (snapshot.Snapshot, error) {
				moduleDir := ""
				if exactModuleDir, ok := analysistarget.ExactCandidateKeyModuleDir(targetOverride); ok {
					moduleDir = exactModuleDir
				}
				return snapshot.BuildContext(ctx, snapshot.Options{
					RepoPath: repo, GoTarget: goTarget.String(), GoModuleDir: moduleDir,
					BuildTags:        goBuildTags,
					RepositoryCorpus: repositoryCorpus, AutoGoTarget: autoGoTarget,
				})
			},
		)
		if prepareErr != nil {
			return prepareErr
		}
		if goSource != nil && (goSource.GoFacts == nil || goSource.TargetCatalog == nil ||
			len(goSource.TargetCatalog.Entries) == 0) {
			return fmt.Errorf("repository target planning found Go project evidence without an exact Go target catalog")
		}
		if goSource != nil {
			reportRepositorySnapshotScaleWarnings(humanOutput, *goSource)
			reportGoFactScaleWarnings(humanOutput, goSource.GoFacts)
		}
		firstLayer := debugdump.NewSemanticObserver(nil)
		selectionExecutor := llm.Executor{
			RootDir: dDir, Enabled: !*noCache, Observer: firstLayer,
			BatchConcurrency: deps.llmBatchConcurrency,
			BatchController:  deps.llmBatchController,
		}
		repositoryName := repoRunLabel(repo)
		if goSource != nil && strings.TrimSpace(goSource.RepoName) != "" {
			repositoryName = goSource.RepoName
		}
		plan, selectionErr := selectRepositoryTargetPlanForRun(ctx, repositoryTargetRuntimeOptions{
			RepoName: repositoryName, Repository: repositoryCorpus,
			GoSnapshot: goSource, DiscoverPython: languageEvidence.Python,
			DiscoverJSTS:   languageEvidence.JavaScriptTypeScript,
			TargetOverride: targetOverride,
			Output:         humanOutput, Providers: newTargetPortfolioProvider,
			Executor: selectionExecutor, ScoutJSTSFn: jstsproject.ScoutTargets,
		})
		reportSemanticOrdinalScaleWarnings(
			humanOutput, "Repository selection",
			[]string{"repository: " + repositoryName},
			firstLayer.OrdinalScaleWarnings(),
		)
		if selectionErr != nil {
			flushFailedFirstLayerSemanticJournal(runDir, firstLayer, humanOutput)
			return errors.Join(
				selectionErr,
				recordTargetPortfolioOutcome(runDir, plan.Outcome, humanOutput),
			)
		}
		if deferredGoBuildTagsErr != nil && repositoryTargetPlanContainsGo(plan) {
			flushFailedFirstLayerSemanticJournal(runDir, firstLayer, humanOutput)
			return errors.Join(
				fmt.Errorf("GOTAGS: %w", deferredGoBuildTagsErr),
				recordTargetPortfolioOutcome(runDir, plan.Outcome, humanOutput),
			)
		}
		var verifiedRuns []report.VerifiedRunReceipt
		publication, reportPath, dispatchErr := dispatchRepositoryTargetPlan(
			ctx,
			repositoryTargetDispatchOptions{
				Repo: repo, ExtraArgs: append([]string(nil), extraArgs...), Deps: deps,
				GoTarget: goTarget.String(), GoBuildTags: append([]string(nil), goBuildTags...),
				DirectCallDepth: *directCallDepth, DirectCallEdgeLimit: *directCallEdgeLimit,
				Corpus: repositoryCorpus, RepositoryState: initialState, Plan: plan,
				RunID: runID, DebugDir: dDir, NoCache: *noCache, NoOpen: *noOpen,
				NoServe: *noServe, Port: *port, StaticHost: staticSourceHost,
				Output: humanOutput, FirstLayer: firstLayer,
				DiscoverJSTSFn: jstsproject.DiscoverSelected,
				VerifiedRunsSink: func(receipts []report.VerifiedRunReceipt) {
					verifiedRuns = append([]report.VerifiedRunReceipt(nil), receipts...)
				},
			},
		)
		if dispatchErr != nil {
			flushFailedFirstLayerSemanticJournal(runDir, firstLayer, humanOutput)
			return errors.Join(
				dispatchErr,
				recordTargetPortfolioOutcome(runDir, plan.Outcome, humanOutput),
			)
		}
		publicationStateEmitted = true
		return finishRepositoryTargetDispatch(
			ctx, deps, dDir, filepath.Dir(reportPath), reportPath, publication,
			*noServe, *noOpen, *port, staticSourceHost, verifiedRuns, humanOutput,
		)
	}

	var dependencyCatalog *dependencies.Catalog
	coreReadmeRoleRows := cloneReadmeRoleLog(deps.coreReadmeRoleRows)
	analysisTargetOverride := strings.TrimSpace(*analysisTargetFlag)
	if deps.preselectedTarget == nil {
		return fmt.Errorf("repository target dispatcher did not provide an exact page target")
	}
	selected := *deps.preselectedTarget
	if err := selected.Validate(); err != nil {
		return fmt.Errorf("bind preselected repository target: %w", err)
	}
	if deps.preselectedProgramPage == nil {
		return fmt.Errorf("repository target dispatcher did not provide exact ProgramIndex authority")
	}
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return fmt.Errorf("bind preselected repository target: %w", err)
	}
	genericProgramPage, err := ownRepositoryProgramPageAuthority(
		registry, selected, *deps.preselectedProgramPage,
	)
	if err != nil {
		return fmt.Errorf("bind preselected repository target: %w", err)
	}
	if deps.targetOutcomeStageSink != nil {
		deps.targetOutcomeStageSink(targetoutcome.StageProgramAnalysis)
	}
	opts := orient.Options{
		RepoPath:         repo,
		GoTarget:         goTarget.String(),
		BuildTags:        append([]string(nil), goBuildTags...),
		RepositoryCorpus: repositoryCorpus,
		AutoGoTarget:     autoGoTarget,
		RunID:            runID,
		DebugDir:         dDir,
		DumpRedacted:     true,
		RequireArtifacts: true,
		SkipGoFacts:      true,
		EffectiveOptions: debugdump.EffectiveOptions{
			NoCache:                *noCache,
			GoTarget:               goTarget.String(),
			BuildTags:              append([]string(nil), goBuildTags...),
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
	opts.Progress = humanOutput.Progress

	err = orient.Run(ctx, opts)
	metadataPath := filepath.Join(runDir, "metadata.json")
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
	index := genericProgramPage.ProgramIndex.Snapshot()
	reportProgramIndexScaleWarnings(humanOutput, []programindex.Index{index})
	reportProgramViewScaleWarnings(humanOutput, []programindex.Index{index})
	catalog, catalogErr := dependencies.BuildWithOmissions(
		genericProgramPage.Dependencies.Importers,
		genericProgramPage.Dependencies.Dependencies,
		genericProgramPage.Dependencies.Coverage.Omissions,
	)
	if catalogErr != nil {
		return fmt.Errorf("own generic dependency authority: %w", catalogErr)
	}
	dependencyCatalog = &catalog
	humanOutput.State(
		"Base program index", "ready",
		"language: "+index.Target.Language,
		fmt.Sprintf("objects: %d", len(index.Objects)),
		fmt.Sprintf("relations: %d", len(index.Relations)),
	)
	if strings.TrimSpace(index.Target.ID) == "" {
		return fmt.Errorf("ordinary analysis produced program indexes without an exact default target")
	}
	if deps.reducedDocumentation == nil {
		return fmt.Errorf("repository target dispatcher did not provide reduced documentation authority")
	}
	if deps.targetOutcomeStageSink != nil {
		deps.targetOutcomeStageSink(targetoutcome.StageSemanticAnalysis)
	}
	ownedDocumentation, documentationErr := deps.reducedDocumentation.Snapshot()
	if documentationErr != nil {
		return fmt.Errorf("bind reduced documentation authority: %w", documentationErr)
	}
	index, err = enrichProgramIndexForRun(
		ctx,
		runDir,
		dDir,
		*noCache,
		deps.llmBatchConcurrency,
		deps.llmBatchController,
		newCubeProvider,
		deps.runProgramCategorization,
		ownedDocumentation,
		index,
		humanOutput,
	)
	if err != nil {
		return err
	}
	indexSet, setErr := programindex.BuildArtifactSet(index)
	if setErr != nil {
		return setErr
	}
	reportProgramIndexSetScaleWarnings(humanOutput, indexSet, []programindex.Index{index})
	if err := programindex.Persist(runDir, programindex.ArtifactFilename, index); err != nil {
		return err
	}
	if err := programindex.PersistArtifactSet(runDir, indexSet); err != nil {
		return err
	}
	humanOutput.State(
		"Program index", "ready",
		"enriched targets: 1",
		"artifact set: "+programindex.ArtifactSetFilename,
	)
	defaultProgramIndex := index.Snapshot()
	defaultGroupIndex, err := groupProgramIndexForRun(
		ctx,
		runDir,
		dDir,
		*noCache,
		deps.llmBatchConcurrency,
		deps.llmBatchController,
		newCubeProvider,
		deps.runProgramGrouping,
		index,
		humanOutput,
	)
	if err != nil {
		return err
	}
	if deps.targetOutcomeStageSink != nil {
		deps.targetOutcomeStageSink(targetoutcome.StageDependencyAnalysis)
	}
	index = genericProgramPage.ProgramIndex
	humanOutput.State(
		"Analysis target", index.Target.Name,
		"language: "+index.Target.Language,
		"kind: "+index.Target.Kind,
		"selector: "+index.Target.Selector,
	)
	if dependencyCatalog == nil {
		return fmt.Errorf("selected target requires exact dependency authority")
	}
	reportDependencyCatalogScaleWarnings(humanOutput, defaultProgramIndex.Target, *dependencyCatalog)
	if err := dependencies.Persist(runDir, *dependencyCatalog); err != nil {
		return err
	}
	if deps.targetOutcomeStageSink != nil {
		deps.targetOutcomeStageSink(targetoutcome.StageTargetPage)
	}
	var reportPath string
	reportData, err := report.ReadRunDir(runDir)
	if err != nil {
		return fmt.Errorf("read captured report inputs: %w", err)
	}
	reportTarget := reportDefaultProgramTarget(reportData)
	targetReportScaleWarnings := report.ReportInputScaleWarnings(reportData)
	targetReportScaleWarnings = append(
		targetReportScaleWarnings,
		report.CapturedReportInputFileScaleWarnings(runDir)...,
	)
	if manifest, manifestErr := report.ReadRunManifest(runDir); manifestErr == nil {
		targetReportScaleWarnings = append(
			targetReportScaleWarnings,
			report.RunManifestScaleWarnings(manifest)...,
		)
	}
	reportInputScaleWarnings(humanOutput, targetReportScaleWarnings, reportTarget)
	reconciliationStarted := time.Now()
	humanOutput.Stage("Repository authority", "reconciling captured inputs")
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
	generateAuthorizedReport := func() (report.GenerationDiagnostics, error) {
		if gitLabURL != "" {
			return report.GenerateAuthorizedGitLabWithDiagnostics(runDir, authority, gitLabURL)
		}
		if gitHubURL != "" {
			return report.GenerateAuthorizedGitHubWithDiagnostics(runDir, authority, gitHubURL)
		}
		return report.GenerateAuthorizedWithDiagnostics(runDir, authority)
	}
	reportStarted := time.Now()
	if !deps.deferredPortfolioHTML {
		humanOutput.Stage("Report", "generating authorized Program report")
		generationDiagnostics, generationErr := generateAuthorizedReport()
		alreadyReported := reportScaleWarningKeySet(targetReportScaleWarnings)
		generationScaleWarnings := excludeReportScaleWarnings(
			generationDiagnostics.ScaleWarnings(), alreadyReported,
		)
		reportInputScaleWarnings(humanOutput, generationScaleWarnings, reportTarget)
		generationTargetScaleWarnings := reportTargetBoundInputScaleWarnings(
			humanOutput,
			excludeTargetReportScaleWarnings(
				generationDiagnostics.TargetScaleWarnings(), alreadyReported,
			),
			reportTarget,
		)
		targetReportScaleWarnings = append(
			targetReportScaleWarnings,
			generationScaleWarnings...,
		)
		targetReportScaleWarnings = append(
			targetReportScaleWarnings,
			generationTargetScaleWarnings...,
		)
		if generationErr != nil {
			return fmt.Errorf("generate authorized browser report: %w", generationErr)
		}
		reportScaleWarnings := make([]report.ReportInputScaleWarning, 0)
		if manifest, manifestErr := report.ReadRunManifest(runDir); manifestErr == nil {
			reportScaleWarnings = append(
				reportScaleWarnings,
				report.RunManifestScaleWarnings(manifest)...,
			)
		}
		reportScaleWarnings = append(
			reportScaleWarnings,
			report.PublishedReportScaleWarnings(runDir)...,
		)
		reportScaleWarnings = excludeReportScaleWarnings(
			reportScaleWarnings,
			reportScaleWarningKeySet(targetReportScaleWarnings),
		)
		reportInputScaleWarnings(humanOutput, reportScaleWarnings, reportTarget)
	}
	reportPath = filepath.Join(runDir, "report.html")
	if !deps.siblingTargetRun {
		humanOutput.State(
			"Report", "generated",
			formatRunOutputWallDuration(time.Since(reportStarted)),
		)
		humanOutput.Stage("Report", "path: "+reportPath)
	}
	if !deps.siblingTargetRun && staticSourceHost != "" {
		humanOutput.Stage(
			"Report",
			fmt.Sprintf("standalone host: %s", staticSourceHost),
			"captured revision: "+initialState.Head,
			"remote availability is not checked; ensure the captured commit is pushed before sharing",
		)
	}
	publication := report.PublicationAssessment{Status: report.PublicationReady}
	backingPage, err := preparePublishedTargetAuthority(
		func() (report.TargetNavigationPage, error) {
			return report.PreparedTargetNavigationPage(runDir, reportData)
		},
	)
	if err != nil {
		return errors.Join(err, quarantineTargetPagePublication([]string{runDir}))
	}
	if !deps.deferredPortfolioHTML {
		publication, err = report.AssessRunPublication(runDir)
		if err != nil {
			return errors.Join(
				fmt.Errorf("verify generated report publication: %w", err),
				quarantineTargetPagePublication([]string{runDir}),
			)
		}
	}
	publishedTarget := targetPublishedRun{
		RunID:                 runID,
		RunDir:                runDir,
		ProgramPage:           backingPage,
		GroupIndex:            defaultGroupIndex,
		ReportScaleWarnings:   append([]report.ReportInputScaleWarning(nil), targetReportScaleWarnings...),
		Authority:             authority,
		RepositoryStateSHA256: initialRepositoryStateSHA256,
		SelectedRevision:      initialState.Head,
		GitLabURL:             gitLabURL,
		GitHubURL:             gitHubURL,
	}
	if deps.preselectedTarget != nil {
		publishedTarget.SelectedTargetKey = deps.preselectedTarget.Key.String()
		publishedTarget.SelectedTargetDisplay = repositoryTypedTargetDisplay(*deps.preselectedTarget)
	}
	if deps.publishedTargetSink != nil {
		deps.publishedTargetSink(publishedTarget)
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

func bindParsedRepositoryArgument(
	repo string,
	repositoryArgumentOmitted bool,
	positional []string,
) (string, bool, error) {
	if len(positional) == 0 {
		return repo, repositoryArgumentOmitted, nil
	}
	if !repositoryArgumentOmitted || repo != "." || len(positional) != 1 {
		return "", repositoryArgumentOmitted, fmt.Errorf(
			"unexpected positional arguments: %s",
			strings.Join(positional, " "),
		)
	}
	return positional[0], false, nil
}

func validateImplicitRepositorySourceLink(
	flagName string,
	repositoryURL string,
	originIdentity string,
	repositoryArgumentOmitted bool,
) error {
	if !repositoryArgumentOmitted || strings.TrimSpace(originIdentity) == "" || repositoryURL == "" {
		return nil
	}
	parsed, err := url.Parse(repositoryURL)
	if err == nil {
		originHost, originProject, found := strings.Cut(strings.TrimSpace(originIdentity), "/")
		sourceProject := strings.TrimPrefix(parsed.Path, "/")
		if found && originHost != "" && originProject != "" &&
			strings.EqualFold(parsed.Hostname(), originHost) &&
			strings.EqualFold(sourceProject, originProject) {
			return nil
		}
	}
	return fmt.Errorf(
		"%s does not match the analyzed current-directory origin; this source-link flag does not clone or select a repository; pass [repo] explicitly to analyze another checkout, or use a URL matching the current directory's origin",
		flagName,
	)
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

func cloneRepositoryState(value freshness.RepositoryState) freshness.RepositoryState {
	result := value
	result.Dirty = append([]freshness.DirtyFile(nil), value.Dirty...)
	result.Submodules = append([]freshness.SubmoduleState(nil), value.Submodules...)
	return result
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

func resolveGoBuildTagsForRepository(hasGoEvidence bool, raw string) ([]string, error) {
	if !hasGoEvidence {
		return []string{}, nil
	}
	return gotarget.ParseBuildTags(raw)
}

// prepareGoBuildTags may defer an invalid Go-only environment value only long
// enough to resolve one explicit language-neutral target. Default selection
// still fails before its model request, while an exact Python or JavaScript /
// TypeScript target is not blocked by irrelevant Go configuration.
func prepareGoBuildTags(
	hasGoEvidence bool,
	raw string,
	targetOverride string,
) (tags []string, deferredErr error, immediateErr error) {
	tags, err := resolveGoBuildTagsForRepository(hasGoEvidence, raw)
	if err == nil {
		return tags, nil, nil
	}
	if strings.TrimSpace(targetOverride) != "" {
		return []string{}, err, nil
	}
	return nil, nil, err
}

func repositoryTargetPlanContainsGo(plan repositoryTargetPlan) bool {
	for _, target := range plan.Targets {
		if target.Key.Adapter == repositoryTargetAdapterGo {
			return true
		}
	}
	return false
}

func directCallEdgeCeilingError(target string, depth, edgeLimit, safeDepth int) error {
	depthLabel := "all reachable calls"
	if depth > 0 {
		depthLabel = strconv.Itoa(depth)
	}
	lines := []string{fmt.Sprintf(
		"Go call analysis for target %s exceeded the explicit --edges-limit=%d at depth %s",
		target, edgeLimit, depthLabel,
	)}
	if safeDepth >= 1 && (depth == 0 || safeDepth < depth) {
		lines = append(lines, fmt.Sprintf(
			"you can opt into a narrower traversal with --depth %d; this depth is known to fit the current edge ceiling",
			safeDepth,
		))
	}
	if edgeLimit > 0 {
		next := edgeLimit * 2
		if next > edgeLimit {
			lines = append(lines, fmt.Sprintf(
				"to preserve depth, try --edges-limit %d or use --edges-limit 0 to retain every edge",
				next,
			))
		}
	}
	lines = append(lines,
		"the retry rebuilds local SSA and the call index; the standard Go build cache remains reusable, and no provider call was made",
	)
	return errors.New(strings.Join(lines, "\n"))
}

func validateDirectCallControls(depth, edgeLimit int) error {
	if depth < 0 {
		return fmt.Errorf("--depth must be non-negative (0 keeps all reachable calls)")
	}
	if edgeLimit < 0 {
		return fmt.Errorf("--edges-limit must be non-negative (0 keeps all exact edges)")
	}
	return nil
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
	fmt.Fprintf(writer, "  --depth N                   target call-graph depth (0 = all; default: 0)\n")
	fmt.Fprintf(writer, "  --edges-limit N             maximum exact target call-graph edges (0 = all; default: 0)\n")
	fmt.Fprintf(writer, "  --github-url URL            static GitHub source links; does not select a repository\n")
	fmt.Fprintf(writer, "  --gitlab-url URL            static GitLab source links; does not select a repository\n")
	fmt.Fprintf(writer, "  --no-open                   do not open the report\n")
	fmt.Fprintf(writer, "  --no-serve                  write static HTML with remote source links\n")
	fmt.Fprintf(writer, "  --port PORT                 local report server port (default: random)\n")
	fmt.Fprintf(writer, "  --debug-dir DIR             report and cache directory\n")
	fmt.Fprintf(writer, "  --no-cache                  bypass persistent model-response caches\n")
	fmt.Fprintf(writer, "  --scan-secrets              enable heuristic credential scanning\n")
	fmt.Fprintf(writer, "  --help, -h                  show this help\n")
	fmt.Fprintf(writer, "  --version                   show version\n")
	fmt.Fprintf(writer, "\nEnvironment:\n")
	fmt.Fprintf(writer, "  GOTAGS               additional Go build tags (comma or whitespace separated)\n")
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
