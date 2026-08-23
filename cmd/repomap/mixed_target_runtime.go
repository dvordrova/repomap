package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/lexicalhints"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/snapshot"
)

// mixedTargetRunSelection is one language-neutral file-portfolio decision
// restored through both exact language adapters. The current report can own
// only a Go-only positive selection; any selected Python target ends the run
// before ProgramIndex construction instead of becoming a structural sibling.
type mixedTargetRunSelection struct {
	Go      snapshot.TargetRunSelection
	Outcome targetPortfolioRunOutcome
}

func selectMixedTargetsForRun(
	ctx context.Context,
	repoName string,
	goCatalog analysistarget.TargetCatalog,
	facts gofacts.Facts,
	pythonCatalog pythontarget.Catalog,
	repository *corpus.Corpus,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
) (mixedTargetRunSelection, error) {
	if err := goCatalog.Validate(); err != nil {
		return mixedTargetRunSelection{}, err
	}
	if err := pythonCatalog.Validate(); err != nil {
		return mixedTargetRunSelection{}, fmt.Errorf("validate Python target catalog: %w", err)
	}
	if repository == nil {
		return mixedTargetRunSelection{}, fmt.Errorf("mixed target selection: repository corpus is unavailable")
	}
	if len(goCatalog.Entries) == 0 {
		return mixedTargetRunSelection{}, fmt.Errorf("mixed target selection: no eligible Go analysis targets")
	}

	parallelContext, cancelParallel := context.WithCancel(ctx)
	defer cancelParallel()
	type readmeScoutResult struct {
		roles readmetargetscout.Result
		err   error
	}
	readmeResult := make(chan readmeScoutResult, 1)
	go func() {
		if !readmetargetscout.HasReadmeFiles(repository) {
			readmeResult <- readmeScoutResult{roles: readmetargetscout.Result{}}
			return
		}
		started := time.Now()
		lexical, err := lexicalhints.Scan(parallelContext, repository)
		if err != nil {
			readmeResult <- readmeScoutResult{err: fmt.Errorf("local lexical hints: %w", err)}
			return
		}
		if output != nil {
			output.State(
				"Local lexical hints", "ready",
				fmt.Sprintf(
					"scanned tracked files: %d/%d",
					lexical.Coverage.ScannedFiles, lexical.Coverage.TrackedFiles,
				),
				fmt.Sprintf("files with positive counts: %d", len(lexical.Model.ByFile)),
				formatRunOutputWallDuration(time.Since(started)),
			)
		}
		roles, err := discoverReadmeFileRoles(
			parallelContext, repoName, repository, lexical, output, providers, executor,
		)
		readmeResult <- readmeScoutResult{roles: roles, err: err}
	}()

	goDiscoveryStarted := time.Now()
	goCandidates, goResolver, goErr := analysistarget.DiscoverGoTargetFilesWithResolver(
		repository, facts, goCatalog,
	)
	goDiscoveryDuration := time.Since(goDiscoveryStarted)
	pythonProjectionStarted := time.Now()
	pythonCandidates, pythonResolver, pythonErr := pythontarget.FileCandidatesWithResolver(
		repository, pythonCatalog,
	)
	pythonProjectionDuration := time.Since(pythonProjectionStarted)
	if err := errors.Join(goErr, pythonErr); err != nil {
		cancelParallel()
		<-readmeResult
		return mixedTargetRunSelection{}, err
	}
	if output != nil {
		output.State(
			"Go target discovery", "ready",
			fmt.Sprintf("exact targets: %d", len(goCatalog.Entries)),
			fmt.Sprintf("native file hypotheses: %d", len(goCandidates)),
			formatRunOutputWallDuration(goDiscoveryDuration),
		)
		output.State(
			"Python target projection", "ready",
			fmt.Sprintf("exact targets: %d", len(pythonCatalog.Entries)),
			fmt.Sprintf("native file hypotheses: %d", len(pythonCandidates)),
			formatRunOutputWallDuration(pythonProjectionDuration),
		)
	}
	readme := <-readmeResult
	if readme.err != nil {
		return mixedTargetRunSelection{}, readme.err
	}
	readmeCandidates, unsupported := mixedResolvableReadmeTargetCandidates(
		readme.roles.TargetCandidates(), goResolver, pythonResolver,
	)
	if output != nil && unsupported > 0 {
		output.Stage(
			"README file classifier",
			fmt.Sprintf(
				"kept %d target hypotheses resolvable by Go or Python; retained %d unsupported target roles only in diagnostics",
				len(readmeCandidates), unsupported,
			),
		)
	}
	mergeStarted := time.Now()
	merged, err := analysistarget.MergeFileCandidates(
		repository.Snapshot(), goCandidates, pythonCandidates, readmeCandidates,
	)
	if err != nil {
		return mixedTargetRunSelection{}, fmt.Errorf("merge mixed target hypotheses: %w", err)
	}
	if output != nil {
		output.State(
			"Target hypothesis merge", "complete",
			fmt.Sprintf("Go hypotheses: %d", len(goCandidates)),
			fmt.Sprintf("Python hypotheses: %d", len(pythonCandidates)),
			fmt.Sprintf("README hypotheses: %d", len(readmeCandidates)),
			fmt.Sprintf("merged hypotheses: %d", len(merged)),
			formatRunOutputWallDuration(time.Since(mergeStarted)),
		)
	}
	if len(merged) == 0 {
		return mixedTargetRunSelection{}, fmt.Errorf("mixed target discovery returned no file hypotheses")
	}

	portfolio, outcome, err := selectTargetPortfolioForRun(
		ctx, repository.Snapshot(), merged, output, providers, executor,
	)
	outcome.ReadmeRoles = compileReadmeRoleLog(repository, readme.roles)
	if err != nil {
		return mixedTargetRunSelection{Outcome: outcome}, withTargetPortfolioChoices(
			err,
			targetPortfolioChoiceGroup{Language: "Go", Choices: targetPortfolioChoices(goCatalog)},
			targetPortfolioChoiceGroup{Language: "Python", Choices: pythonTargetChoices(pythonCatalog)},
		)
	}
	if portfolio.Default == nil {
		return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf("mixed target portfolio accepted targets without a default")
	}

	goFiles := make([]corpus.FileID, 0, len(portfolio.Targets))
	pythonFiles := make([]corpus.FileID, 0, len(portfolio.Targets))
	for _, candidate := range portfolio.Targets {
		goMatch := goResolver.ResolvesOne(candidate.FileRef)
		pythonMatch := pythonResolver.Resolves(candidate.FileRef)
		if !goMatch && !pythonMatch {
			return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf(
				"model-selected file %q is outside both language adapters", candidate.Path,
			)
		}
		if goMatch {
			goFiles = append(goFiles, candidate.FileRef)
		}
		if pythonMatch {
			pythonFiles = append(pythonFiles, candidate.FileRef)
		}
	}
	outcome.SelectedFileRefs = len(portfolio.Targets)
	outcome.UnclassifiedFiles = len(portfolio.Unclassified)
	goRefs := []string{}
	if len(goFiles) > 0 {
		goRefs, err = goResolver.Resolve(goFiles)
		if err != nil {
			return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf("restore selected Go targets: %w", err)
		}
	}
	pythonTargets := []pythontarget.Target{}
	if len(pythonFiles) > 0 {
		var resolveErr error
		pythonTargets, resolveErr = pythonResolver.Resolve(pythonFiles)
		if resolveErr != nil {
			return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf("restore selected Python targets: %w", resolveErr)
		}
		if len(pythonTargets) == 0 {
			return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf("selected Python file set restored no exact targets")
		}
	}
	outcome.SelectedTargets = len(goRefs) + len(pythonTargets)
	outcome.SelectedTargetRefs = append(append([]string(nil), goRefs...), pythonTargetRefs(pythonTargets)...)

	if !goResolver.ResolvesOne(portfolio.Default.FileRef) {
		if pythonResolver.Resolves(portfolio.Default.FileRef) {
			return mixedTargetRunSelection{Outcome: outcome}, mixedPythonSelectionError(
				portfolio.Default.Path, true, len(pythonFiles), len(pythonTargets), goCatalog,
			)
		}
		return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf(
			"target portfolio default %q has no unambiguous exact target in either language adapter",
			portfolio.Default.Path,
		)
	}
	defaultGoRef, err := goResolver.ResolveOne(portfolio.Default.FileRef)
	if err != nil {
		return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf("restore default Go target: %w", err)
	}
	_, ok := targetCatalogEntryByRef(goCatalog, defaultGoRef)
	if !ok {
		return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf("restored default Go target is outside the target catalog")
	}
	outcome.SelectedRef = defaultGoRef
	if len(pythonTargets) > 0 {
		return mixedTargetRunSelection{Outcome: outcome}, mixedPythonSelectionError(
			portfolio.Default.Path, false, len(pythonFiles), len(pythonTargets), goCatalog,
		)
	}
	if len(goRefs) == 0 {
		return mixedTargetRunSelection{Outcome: outcome}, fmt.Errorf(
			"target portfolio selected no Go target that the current mixed-repository report can own",
		)
	}
	return mixedTargetRunSelection{
		Go:      snapshot.TargetRunSelection{DefaultTargetRef: defaultGoRef, TargetRefs: goRefs},
		Outcome: outcome,
	}, nil
}

func mixedPythonSelectionError(
	defaultPath string,
	pythonDefault bool,
	selectedPythonFiles int,
	selectedPythonTargets int,
	goCatalog analysistarget.TargetCatalog,
) error {
	defaultFact := "the default remains an exact Go target"
	if pythonDefault {
		defaultFact = fmt.Sprintf("the default file %q resolves to Python", defaultPath)
	}
	return fmt.Errorf(
		"mixed Go/Python target portfolio selected %d Python file(s) restoring to %d exact Python target view(s), and %s; the current report cannot publish complete semantics for that mixed positive selection; rerun with --target using one exact Go choice (%s), or analyze a Python-only project root",
		selectedPythonFiles,
		selectedPythonTargets,
		defaultFact,
		targetPortfolioChoices(goCatalog),
	)
}

func mixedResolvableReadmeTargetCandidates(
	candidates []analysistarget.FileCandidate,
	goResolver analysistarget.GoFileTargetResolver,
	pythonResolver pythontarget.FileTargetResolver,
) ([]analysistarget.FileCandidate, int) {
	result := make([]analysistarget.FileCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if goResolver.ResolvesOne(candidate.FileRef) || pythonResolver.Resolves(candidate.FileRef) {
			result = append(result, candidate)
		}
	}
	if result == nil {
		result = []analysistarget.FileCandidate{}
	}
	return result, len(candidates) - len(result)
}
