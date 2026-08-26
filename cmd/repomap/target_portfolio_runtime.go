package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

type targetPortfolioProviderFactory func() (llm.Provider, error)

type readmeRoleLogRow struct {
	FileRef         corpus.FileID                      `json:"file_ref"`
	Path            string                             `json:"path"`
	Classifications []readmetargetscout.Classification `json:"classifications"`
}

type targetPortfolioRunOutcome struct {
	SelectedRef         string
	SelectedTargets     int
	SelectedTargetRefs  []string
	SelectedFileRefs    int
	UnclassifiedFiles   int
	ReadmeRoles         []readmeRoleLogRow
	Cached              bool
	Request             []byte
	Response            []byte
	ResponseUnavailable *debugdump.SemanticUnavailable
	RequestProvenance   string
	SemanticState       string
	ValidationCode      string
	SemanticCalls       int
	TransportAttempts   int
	RequestBytes        int
	ResponseBytes       int
	LatencyMillis       int64
}

func defaultTargetPortfolioProviderFactory() (llm.Provider, error) {
	return deepseek.NewFromEnv()
}

// selectTargetsForRun runs the parallel first-layer scouts, dumb FileRef
// merge, and file-only portfolio. Existing Go target refs are restored only
// after the model decision so they never enter either provider request.
func selectTargetsForRun(
	ctx context.Context,
	repoName string,
	catalog analysistarget.TargetCatalog,
	facts gofacts.Facts,
	repository *corpus.Corpus,
	override string,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
) (snapshot.TargetRunSelection, targetPortfolioRunOutcome, error) {
	if err := catalog.Validate(); err != nil {
		return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, err
	}
	if repository == nil {
		return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, fmt.Errorf("repository corpus is unavailable")
	}
	if len(catalog.Entries) == 0 {
		return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, fmt.Errorf("no eligible Go analysis targets")
	}

	var outcome targetPortfolioRunOutcome
	override = strings.TrimSpace(override)
	var explicitEntry *analysistarget.TargetCatalogEntry
	if override != "" {
		entry, resolveErr := resolveTargetOverride(catalog, override)
		if resolveErr != nil {
			return snapshot.TargetRunSelection{}, targetPortfolioRunOutcome{}, resolveErr
		}
		explicitEntry = &entry
	}

	parallelContext, cancelParallel := context.WithCancel(ctx)
	defer cancelParallel()
	type readmeScoutResult struct {
		roles readmetargetscout.Result
		err   error
	}
	readmeResult := make(chan readmeScoutResult, 1)
	go func() {
		if !readmetargetscout.HasGuidanceFiles(repository) {
			readmeResult <- readmeScoutResult{roles: readmetargetscout.Result{}}
			return
		}
		roles, err := discoverReadmeFileRoles(
			parallelContext, repoName, repository, output, providers, executor,
		)
		readmeResult <- readmeScoutResult{roles: roles, err: err}
	}()
	var (
		goCandidates []analysistarget.FileCandidate
		resolver     analysistarget.GoFileTargetResolver
	)
	if explicitEntry == nil {
		discoveryStarted := time.Now()
		var discoveryErr error
		goCandidates, resolver, discoveryErr = analysistarget.DiscoverGoTargetFilesWithResolver(
			repository, facts, catalog,
		)
		if discoveryErr != nil {
			cancelParallel()
			<-readmeResult
			return snapshot.TargetRunSelection{}, outcome, discoveryErr
		}
		if output != nil {
			output.State(
				"Go target discovery", "ready",
				fmt.Sprintf("exact targets: %d", len(catalog.Entries)),
				fmt.Sprintf("native file hypotheses: %d", len(goCandidates)),
				formatRunOutputWallDuration(time.Since(discoveryStarted)),
			)
		}
	}

	readme := <-readmeResult
	if readme.err != nil {
		return snapshot.TargetRunSelection{}, outcome, readme.err
	}
	if explicitEntry != nil {
		defaultRef := explicitEntry.Candidate.Target.Ref
		refs := []string{defaultRef}
		outcome.SelectedRef = defaultRef
		outcome.SelectedTargets = 1
		outcome.SelectedTargetRefs = refs
		outcome.ReadmeRoles = compileReadmeRoleLog(repository, readme.roles)
		if output != nil {
			output.State(
				"Go target discovery", "not needed",
				"reason: explicit --target bypasses file-hypothesis discovery",
			)
			output.State(
				"Target hypothesis merge", "not needed",
				"reason: explicit --target bypasses candidate merging",
			)
			output.State(
				"Analysis target", "selected",
				"source: explicit --target",
				"selected: "+explicitEntry.DisplayPath,
			)
		}
		return snapshot.TargetRunSelection{
			DefaultTargetRef: defaultRef,
			TargetRefs:       refs,
		}, outcome, nil
	}

	readmeCandidates, unsupportedReadmeTargets := goResolvableReadmeTargetCandidates(
		readme.roles.TargetCandidates(), resolver,
	)
	if output != nil && unsupportedReadmeTargets > 0 {
		output.Stage(
			"Repository guidance classifier",
			fmt.Sprintf(
				"kept %d target hypotheses for the Go adapter; retained %d unsupported target roles only in diagnostics",
				len(readmeCandidates), unsupportedReadmeTargets,
			),
		)
	}
	mergeStarted := time.Now()
	merged, err := analysistarget.MergeFileCandidates(
		repository.Snapshot(), goCandidates, readmeCandidates,
	)
	if err != nil {
		return snapshot.TargetRunSelection{}, outcome, fmt.Errorf("merge target hypotheses: %w", err)
	}
	if output != nil {
		output.State(
			"Target hypothesis merge", "complete",
			fmt.Sprintf("native hypotheses: %d", len(goCandidates)),
			fmt.Sprintf("guidance hypotheses: %d", len(readmeCandidates)),
			fmt.Sprintf("merged hypotheses: %d", len(merged)),
			formatRunOutputWallDuration(time.Since(mergeStarted)),
		)
	}
	if len(merged) == 0 {
		return snapshot.TargetRunSelection{}, outcome, fmt.Errorf("target discovery returned no file hypotheses")
	}
	executableFileRefs, err := exactGoExecutableFileRefs(merged, resolver, catalog)
	if err != nil {
		return snapshot.TargetRunSelection{}, outcome, fmt.Errorf(
			"bind exact executable target authority: %w", err,
		)
	}

	selection, portfolioOutcome, err := selectTargetPortfolioForRun(
		ctx, repository.Snapshot(), merged, &executableFileRefs, nil, output, providers, executor,
	)
	outcome = portfolioOutcome
	outcome.ReadmeRoles = compileReadmeRoleLog(repository, readme.roles)
	if err != nil {
		return snapshot.TargetRunSelection{}, outcome, withTargetPortfolioChoices(
			err,
			targetPortfolioChoiceGroup{Language: "Go", Choices: targetPortfolioChoices(catalog)},
		)
	}
	selectedFileRefs := make([]corpus.FileID, 0, len(selection.Targets))
	for _, candidate := range selection.Targets {
		selectedFileRefs = append(selectedFileRefs, candidate.FileRef)
	}
	refs, err := resolver.Resolve(selectedFileRefs)
	if err != nil {
		return snapshot.TargetRunSelection{}, outcome, fmt.Errorf(
			"restore selected Go targets from file portfolio: %w; choose one exact Go target with --target TARGET",
			err,
		)
	}
	defaultRef, err := resolver.ResolveOne(selection.Default.FileRef)
	if err != nil {
		return snapshot.TargetRunSelection{}, outcome, fmt.Errorf(
			"restore default Go target from file portfolio: %w; choose one exact Go target with --target TARGET",
			err,
		)
	}
	_, ok := targetCatalogEntryByRef(catalog, defaultRef)
	if !ok {
		return snapshot.TargetRunSelection{}, outcome, fmt.Errorf("restored default Go target is outside the target catalog")
	}
	if err := validateAutomaticGoExecutableAuthority(catalog, defaultRef, refs); err != nil {
		return snapshot.TargetRunSelection{}, outcome, err
	}
	outcome.SelectedRef = defaultRef
	outcome.SelectedTargets = len(refs)
	outcome.SelectedTargetRefs = append([]string(nil), refs...)
	outcome.SelectedFileRefs = len(selection.Targets)
	outcome.UnclassifiedFiles = len(selection.Unclassified)
	return snapshot.TargetRunSelection{
		DefaultTargetRef: defaultRef,
		TargetRefs:       refs,
	}, outcome, nil
}

func goResolvableReadmeTargetCandidates(
	candidates []analysistarget.FileCandidate,
	resolver analysistarget.GoFileTargetResolver,
) ([]analysistarget.FileCandidate, int) {
	result := make([]analysistarget.FileCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !resolver.ResolvesOne(candidate.FileRef) {
			continue
		}
		result = append(result, candidate)
	}
	if result == nil {
		result = []analysistarget.FileCandidate{}
	}
	return result, len(candidates) - len(result)
}

// exactGoExecutableFileRefs projects only exact current catalog/resolver
// capability into the language-neutral selector. It does not infer runnable
// authority from paths or hypothesis prose, and it never removes library
// candidates from the complete request.
func exactGoExecutableFileRefs(
	candidates []analysistarget.FileCandidate,
	resolver analysistarget.GoFileTargetResolver,
	catalog analysistarget.TargetCatalog,
) ([]corpus.FileID, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	kinds := make(map[string]analysistarget.Kind, len(catalog.Entries))
	executables := make(map[string]struct{})
	for _, entry := range catalog.Entries {
		ref := entry.Candidate.Target.Ref
		kind := entry.Candidate.Target.Kind
		kinds[ref] = kind
		if kind == analysistarget.KindExecutablePackage {
			executables[ref] = struct{}{}
		}
	}

	result := make([]corpus.FileID, 0, len(executables))
	represented := make(map[string]struct{}, len(executables))
	for _, candidate := range candidates {
		refs, err := resolver.Resolve([]corpus.FileID{candidate.FileRef})
		if err != nil {
			return nil, fmt.Errorf("file_ref %q: %w", candidate.FileRef, err)
		}
		executable := false
		for _, ref := range refs {
			kind, known := kinds[ref]
			if !known {
				return nil, fmt.Errorf("file_ref %q resolves outside the exact target catalog", candidate.FileRef)
			}
			if kind == analysistarget.KindExecutablePackage {
				executable = true
				represented[ref] = struct{}{}
			}
		}
		if executable {
			result = append(result, candidate.FileRef)
		}
	}
	if len(represented) != len(executables) {
		return nil, fmt.Errorf(
			"executable target projection is incomplete: represented %d of %d exact targets",
			len(represented), len(executables),
		)
	}
	if result == nil {
		return []corpus.FileID{}, nil
	}
	return result, nil
}

// validateAutomaticGoExecutableAuthority rebinds the accepted file decision
// to the current exact catalog. The selector must itself return an executable
// default when one exists; this check rejects stale or incompatible authority
// and never promotes an omitted executable. Explicit --target bypasses it.
func validateAutomaticGoExecutableAuthority(
	catalog analysistarget.TargetCatalog,
	defaultRef string,
	selectedRefs []string,
) error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	selected := make(map[string]struct{}, len(selectedRefs))
	for _, ref := range selectedRefs {
		if _, duplicate := selected[ref]; duplicate {
			return fmt.Errorf("executable target authority contains duplicate selected target %q", ref)
		}
		selected[ref] = struct{}{}
	}
	if _, ok := selected[defaultRef]; !ok {
		return fmt.Errorf("executable target authority default is outside the selected target set")
	}

	executableCatalog := 0
	executableSelected := 0
	defaultKind := analysistarget.Kind("")
	knownSelected := 0
	for _, entry := range catalog.Entries {
		target := entry.Candidate.Target
		if target.Kind == analysistarget.KindExecutablePackage {
			executableCatalog++
		}
		if target.Ref == defaultRef {
			defaultKind = target.Kind
		}
		if _, ok := selected[target.Ref]; ok {
			knownSelected++
			if target.Kind == analysistarget.KindExecutablePackage {
				executableSelected++
			}
		}
	}
	if knownSelected != len(selected) || defaultKind == "" {
		return fmt.Errorf("executable target authority is outside the current exact catalog")
	}
	if executableCatalog == 0 {
		return nil
	}
	if executableSelected == 0 {
		return fmt.Errorf(
			"executable target authority was lost: exact catalog has %d executable target(s), selection has none",
			executableCatalog,
		)
	}
	if defaultKind != analysistarget.KindExecutablePackage {
		return fmt.Errorf(
			"executable target authority requires an executable default, got %q", defaultKind,
		)
	}
	return nil
}

func selectTargetPortfolioForRun(
	ctx context.Context,
	corpusSnapshot corpus.Snapshot,
	candidates []analysistarget.FileCandidate,
	executableFileRefs *[]corpus.FileID,
	requiredTargetFileRefs *[]corpus.FileID,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
) (targetportfolio.Selection, targetPortfolioRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	if err := ctx.Err(); err != nil {
		return targetportfolio.Selection{}, targetPortfolioRunOutcome{}, err
	}

	started := time.Now()
	outcome := targetPortfolioRunOutcome{}
	var compilation targetportfolio.Compilation
	var err error
	if executableFileRefs != nil && requiredTargetFileRefs != nil {
		err = fmt.Errorf("target portfolio cannot bind executable and required target authority together")
	} else if executableFileRefs != nil {
		compilation, err = targetportfolio.CompileWithExecutableAuthority(
			corpusSnapshot, candidates, *executableFileRefs,
		)
	} else if requiredTargetFileRefs != nil {
		compilation, err = targetportfolio.CompileWithRequiredTargetAuthority(
			corpusSnapshot, candidates, *requiredTargetFileRefs,
		)
	} else {
		compilation, err = targetportfolio.Compile(corpusSnapshot, candidates)
	}
	if err != nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "request_build_failed",
			"could not compile its exact candidate request", err,
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	prompt, err := targetportfolio.BuildPrompt(compilation)
	if err != nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "request_build_failed",
			"could not build its model prompt", err,
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	providerBundle, err := targetportfolio.ProviderVisibleJSON(compilation)
	if err != nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "request_build_failed",
			"could not seal its provider-visible request", err,
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	outcome.Request = providerBundle
	outcome.RequestBytes = len(providerBundle)
	outcome.RequestProvenance = debugdump.SemanticRequestPrepared

	if providers == nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "provider_configuration_failed",
			"has no configured model provider", errors.New("provider factory is unavailable"),
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	provider, err := providers()
	if err != nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "provider_configuration_failed",
			"could not configure its model provider", err,
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	if provider == nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "provider_configuration_failed",
			"could not configure its model provider", errors.New("provider is unavailable"),
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	if err := ctx.Err(); err != nil {
		return targetportfolio.Selection{}, targetPortfolioCanceled(outcome), err
	}

	output.Stage(
		"Analysis target",
		fmt.Sprintf("asking the model to choose the default and filter %d merged file hypotheses", len(compilation.Request.Candidates)),
	)
	executionState, err := targetportfolio.ExecutionState(compilation)
	if err != nil {
		failed, failErr := failTargetPortfolioSelection(
			outcome, "request_build_failed", "could not bind its execution state", err,
		)
		return targetportfolio.Selection{}, failed, failErr
	}
	modelOutcome, callErr := llm.ExecuteJSON(
		ctx,
		executor,
		provider,
		llm.Call[targetportfolio.Selection]{
			State: executionState,
			Prompt: llm.Prompt{
				System: prompt.System, User: prompt.User, ResponseFormatJSON: true,
			},
			Limits: llm.Limits{
				MaxRequestBytes:  targetportfolio.MaxProviderRequestBytes,
				MaxResponseBytes: targetportfolio.MaxResponseBytes,
				MaxOutputTokens:  targetportfolio.MaxOutputTokens,
			},
			DecodeValidate: func(raw []byte) (targetportfolio.Selection, error) {
				return targetportfolio.ResolveResponse(compilation, raw)
			},
		},
	)
	applyTargetPortfolioLLMOutcome(&outcome, modelOutcome)
	if callErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			outcome = targetPortfolioCanceled(outcome)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return targetportfolio.Selection{}, outcome, ctxErr
			}
			return targetportfolio.Selection{}, outcome, callErr
		}
		failureCode := targetPortfolioLLMFailureCode(callErr)
		if modelOutcome.ResponseRedacted {
			failureCode = "response_secret_scan"
		}
		failed, failErr := failTargetPortfolioSelection(
			outcome, failureCode,
			"did not complete", callErr,
		)
		return targetportfolio.Selection{}, failed, failErr
	}

	selection := modelOutcome.Value
	outcome.SelectedFileRefs = len(selection.Targets)
	outcome.UnclassifiedFiles = len(selection.Unclassified)
	if outcome.Cached {
		outcome.SemanticState = debugdump.SemanticStateCacheHit
		outcome.ValidationCode = debugdump.SemanticValidationCache
	} else {
		outcome.SemanticState = debugdump.SemanticStateAccepted
		outcome.ValidationCode = debugdump.SemanticValidationAccepted
	}
	if len(selection.Targets) == 0 {
		return selection, outcome, errors.New(
			"target portfolio found no positively supported target entry; choose one exact target with --target TARGET",
		)
	}
	if selection.Default == nil {
		return targetportfolio.Selection{}, outcome, fmt.Errorf("target portfolio accepted targets without a default")
	}
	source := "live provider"
	if outcome.Cached {
		source = "validated cache"
	}
	output.State(
		"Analysis target", "selected",
		"source: "+source,
		fmt.Sprintf("selected file hypotheses: %d", len(selection.Targets)),
		fmt.Sprintf("unclassified file hypotheses: %d", len(selection.Unclassified)),
		formatRunOutputWallDuration(time.Since(started)),
	)
	return selection, outcome, nil
}

func discoverReadmeFileRoles(
	ctx context.Context,
	repoName string,
	repository *corpus.Corpus,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
) (readmetargetscout.Result, error) {
	started := time.Now()
	compilation, err := readmetargetscout.Compile(targetPortfolioRepoName(repoName), repository)
	if err != nil {
		return nil, fmt.Errorf("repository guidance classifier: %w", err)
	}
	if compilation.State == readmetargetscout.StateNotApplicable {
		return readmetargetscout.Result{}, nil
	}
	prompt, err := readmetargetscout.BuildPrompt(compilation)
	if err != nil {
		return nil, fmt.Errorf("repository guidance classifier: build prompt: %w", err)
	}
	if providers == nil {
		return nil, fmt.Errorf("repository guidance classifier: model provider is unavailable; configure the provider and retry")
	}
	provider, err := providers()
	if err != nil {
		return nil, fmt.Errorf("repository guidance classifier: configure model provider: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("repository guidance classifier: configured model provider is unavailable")
	}
	if output != nil {
		output.Stage("Repository guidance classifier", fmt.Sprintf(
			"asking the model to classify sparse repository file roles across %d tracked files",
			compilation.Request.FileCount,
		))
	}
	modelOutcome, err := llm.ExecuteJSON(
		ctx,
		debugdump.BindStage(executor, debugdump.SemanticStageReadmeFileClassifier),
		provider,
		llm.Call[readmetargetscout.Result]{
			State: readmetargetscout.ExecutionState(),
			Prompt: llm.Prompt{
				System: prompt.System, User: prompt.User, ResponseFormatJSON: false,
			},
			Limits: llm.Limits{
				MaxRequestBytes:  readmetargetscout.MaxProviderRequestBytes,
				MaxResponseBytes: readmetargetscout.MaxResponseBytes,
				MaxOutputTokens:  readmetargetscout.MaxOutputTokens,
			},
			DecodeValidate: func(raw []byte) (readmetargetscout.Result, error) {
				return readmetargetscout.ResolveResponse(compilation, raw)
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"repository guidance classifier did not complete: %w; fix provider access and retry",
			err,
		)
	}
	result := modelOutcome.Value
	if output != nil {
		counts := readmeRoleCounts(result)
		source := "live"
		latency := modelOutcome.Metrics.Latency.Milliseconds()
		if modelOutcome.Cached {
			source = "cache"
			latency = 0
		}
		output.Stage(
			"Repository guidance classifier",
			fmt.Sprintf("classified files: %d", len(result)),
			formatRunOutputWallDuration(time.Since(started)),
			fmt.Sprintf(
				"%s result: request %d bytes, response %d bytes, %d ms",
				source, modelOutcome.RequestBytes, modelOutcome.ResponseBytes, latency,
			),
			fmt.Sprintf(
				"roles: target %d, example %d, test %d, support tool %d, config %d, database %d, client %d, docs %d, deployment %d, contract %d",
				counts[readmetargetscout.ClassTargetEntry], counts[readmetargetscout.ClassExampleEntry],
				counts[readmetargetscout.ClassTestEntry], counts[readmetargetscout.ClassSupportToolEntry],
				counts[readmetargetscout.ClassConfiguration], counts[readmetargetscout.ClassDatabaseAsset],
				counts[readmetargetscout.ClassClientEntry], counts[readmetargetscout.ClassDocumentation],
				counts[readmetargetscout.ClassDeployment], counts[readmetargetscout.ClassInterfaceContract],
			),
		)
	}
	return result, nil
}

func readmeRoleCounts(result readmetargetscout.Result) map[readmetargetscout.FileClass]int {
	counts := make(map[readmetargetscout.FileClass]int)
	for _, file := range result {
		for _, classification := range file.Classifications {
			counts[classification.Class]++
		}
	}
	return counts
}

func compileReadmeRoleLog(
	repository *corpus.Corpus,
	result readmetargetscout.Result,
) []readmeRoleLogRow {
	rows := make([]readmeRoleLogRow, 0, len(result))
	for _, file := range result {
		info, known := repository.Info(file.FileRef)
		if !known {
			continue
		}
		row := readmeRoleLogRow{
			FileRef:         file.FileRef,
			Path:            info.Entry.Path,
			Classifications: make([]readmetargetscout.Classification, len(file.Classifications)),
		}
		for classIndex, classification := range file.Classifications {
			row.Classifications[classIndex] = readmetargetscout.Classification{
				Class:      classification.Class,
				Hypotheses: append([]string(nil), classification.Hypotheses...),
			}
		}
		rows = append(rows, row)
	}
	if rows == nil {
		return []readmeRoleLogRow{}
	}
	return rows
}

func cloneReadmeRoleLog(rows []readmeRoleLogRow) []readmeRoleLogRow {
	cloned := make([]readmeRoleLogRow, len(rows))
	for rowIndex, row := range rows {
		cloned[rowIndex] = readmeRoleLogRow{
			FileRef: row.FileRef,
			Path:    row.Path,
			Classifications: make(
				[]readmetargetscout.Classification, len(row.Classifications),
			),
		}
		for classIndex, classification := range row.Classifications {
			cloned[rowIndex].Classifications[classIndex] = readmetargetscout.Classification{
				Class: classification.Class,
				Hypotheses: append(
					[]string(nil), classification.Hypotheses...,
				),
			}
		}
	}
	if cloned == nil {
		return []readmeRoleLogRow{}
	}
	return cloned
}

// restoreReadmeRoleResult rebinds accepted path-backed role facts to the
// current run-local corpus namespace. Repository changes are allowed during a
// multi-target run: removed files simply cease to be evidence, while surviving
// paths receive their current f* ref.
func restoreReadmeRoleResult(
	repository *corpus.Corpus,
	rows []readmeRoleLogRow,
) (readmetargetscout.Result, error) {
	if repository == nil {
		return nil, fmt.Errorf("repository corpus is unavailable")
	}
	result := make(readmetargetscout.Result, 0, len(rows))
	for _, row := range rows {
		fileRef, known := repository.ID(row.Path)
		if !known {
			continue
		}
		classifications := make([]readmetargetscout.Classification, len(row.Classifications))
		for classIndex, classification := range row.Classifications {
			classifications[classIndex] = readmetargetscout.Classification{
				Class:      classification.Class,
				Hypotheses: append([]string(nil), classification.Hypotheses...),
			}
		}
		result = append(result, readmetargetscout.ClassifiedFile{
			FileRef:         fileRef,
			Classifications: classifications,
		})
	}
	return result.SnapshotAgainstCorpus(repository)
}

func targetCatalogEntryByRef(
	catalog analysistarget.TargetCatalog,
	targetRef string,
) (analysistarget.TargetCatalogEntry, bool) {
	for _, entry := range catalog.Entries {
		if entry.Candidate.Target.Ref == targetRef {
			return entry, true
		}
	}
	return analysistarget.TargetCatalogEntry{}, false
}

func applyTargetPortfolioLLMOutcome(
	outcome *targetPortfolioRunOutcome,
	modelOutcome llm.Outcome[targetportfolio.Selection],
) {
	if outcome == nil {
		return
	}
	if modelOutcome.RequestBytes > 0 {
		outcome.Request = append([]byte(nil), modelOutcome.Request...)
		outcome.RequestBytes = modelOutcome.RequestBytes
	}
	outcome.Response = append([]byte(nil), modelOutcome.Response...)
	outcome.ResponseBytes = max(
		modelOutcome.ResponseBytes,
		modelOutcome.Metrics.ProviderResponseBytes,
	)
	outcome.Cached = modelOutcome.Cached
	if modelOutcome.ResponseRedacted {
		outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableOmitted, OriginalBytes: modelOutcome.ResponseBytes,
		}
	} else if len(modelOutcome.Response) == 0 && outcome.ResponseBytes > 0 {
		outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableNoContent, OriginalBytes: outcome.ResponseBytes,
		}
	}
	if modelOutcome.RequestRedacted {
		outcome.Request = nil
	}
	if modelOutcome.Cached {
		outcome.RequestProvenance = debugdump.SemanticRequestPrepared
		outcome.SemanticCalls = 0
		outcome.TransportAttempts = 0
		outcome.LatencyMillis = 0
		return
	}
	if modelOutcome.Metrics.Attempts > 0 {
		outcome.RequestProvenance = debugdump.SemanticRequestExactSent
		outcome.SemanticCalls = 1
		outcome.TransportAttempts = modelOutcome.Metrics.Attempts
		outcome.LatencyMillis = modelOutcome.Metrics.Latency.Milliseconds()
	}
}

func targetPortfolioLLMFailureCode(err error) string {
	switch {
	case errors.Is(err, llm.ErrSensitivePreparedRequest):
		return "request_secret_scan"
	case errors.Is(err, llm.ErrSensitiveResponse):
		return "response_secret_scan"
	}
	var providerErr *llm.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Operation == "prepare" {
			return "request_build_failed"
		}
		return "provider_failed"
	}
	return "response_validation"
}

func targetPortfolioRepoName(repositoryIdentity string) string {
	trimmed := strings.Trim(strings.TrimSpace(repositoryIdentity), "/")
	separator := strings.LastIndexByte(trimmed, '/')
	if separator < 0 {
		return trimmed
	}
	last := trimmed[separator+1:]
	if isGoMajorVersionPathSegment(last) {
		trimmed = strings.TrimSuffix(trimmed[:separator], "/")
		if previous := strings.LastIndexByte(trimmed, '/'); previous >= 0 {
			return trimmed[previous+1:]
		}
		return trimmed
	}
	return last
}

func isGoMajorVersionPathSegment(value string) bool {
	if len(value) < 2 || value[0] != 'v' || value[1] == '0' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return len(value) > 2 || value[1] >= '2'
}

func failTargetPortfolioSelection(
	outcome targetPortfolioRunOutcome,
	code string,
	operation string,
	cause error,
) (targetPortfolioRunOutcome, error) {
	outcome.SemanticState, outcome.ValidationCode = targetPortfolioFailureSemantics(code)
	guidance := "fix the reported model-selection failure and retry, or bypass this required cube by analyzing exactly one target with --target TARGET"
	return outcome, fmt.Errorf(
		"required target portfolio selection %s: %w; %s",
		operation, cause, guidance,
	)
}

// targetPortfolioChoiceGroup belongs to a language adapter, not to the shared
// portfolio cube. The cube selects only closed FileRefs; exact --target keys
// become relevant solely when the CLI explains how to correct a failed run.
type targetPortfolioChoiceGroup struct {
	Language string
	Choices  string
}

func withTargetPortfolioChoices(err error, groups ...targetPortfolioChoiceGroup) error {
	if err == nil {
		return nil
	}
	summaries := make([]string, 0, len(groups))
	for _, group := range groups {
		language := strings.TrimSpace(group.Language)
		choices := strings.TrimSpace(group.Choices)
		if language == "" || choices == "" {
			return fmt.Errorf("%w; exact --target choice guidance is incomplete", err)
		}
		summaries = append(summaries, language+": "+choices)
	}
	if len(summaries) == 0 {
		return fmt.Errorf("%w; no exact --target choices were advertised", err)
	}
	return fmt.Errorf("%w; exact --target choices: %s", err, strings.Join(summaries, "; "))
}

func targetPortfolioCanceled(outcome targetPortfolioRunOutcome) targetPortfolioRunOutcome {
	outcome.SemanticState = debugdump.SemanticStateCanceled
	outcome.ValidationCode = debugdump.SemanticValidationCanceled
	if len(outcome.Response) == 0 {
		outcome.ResponseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableCanceled, OriginalBytes: outcome.ResponseBytes,
		}
	}
	return outcome
}

func targetPortfolioFailureSemantics(code string) (string, string) {
	switch code {
	case "provider_configuration_failed", "provider_failed", "request_build_failed":
		return debugdump.SemanticStateProviderFailed, debugdump.SemanticValidationProvider
	case "request_secret_scan", "response_secret_scan":
		return debugdump.SemanticStateRejected, debugdump.SemanticValidationSecret
	default:
		return debugdump.SemanticStateRejected, debugdump.SemanticValidationResponse
	}
}

func targetPortfolioChoices(catalog analysistarget.TargetCatalog) string {
	const limit = 12
	choices := make([]string, 0, min(len(catalog.Entries), limit)+1)
	available := 0
	for _, entry := range catalog.Entries {
		available++
		if len(choices) == limit {
			continue
		}
		choices = append(choices, fmt.Sprintf(
			"%s (%s; %s)", entry.DisplayPath, entry.Candidate.Target.Kind, entry.Candidate.Key,
		))
	}
	if available > len(choices) {
		choices = append(choices, fmt.Sprintf("... and %d more", available-len(choices)))
	}
	return strings.Join(choices, ", ")
}

// resolveTargetOverride applies fail-closed target resolution. Exact refs and
// typed candidate keys win first. Human
// path aliases are accepted only when they identify one surface: a module-root
// executable and that module's Library API deliberately share a display path,
// so an untyped alias such as "." or "server" must not silently pick one.
func resolveTargetOverride(
	catalog analysistarget.TargetCatalog,
	override string,
) (analysistarget.TargetCatalogEntry, error) {
	exact := make([]analysistarget.TargetCatalogEntry, 0, 1)
	for _, entry := range catalog.Entries {
		if override == entry.Candidate.Target.Ref || override == entry.Candidate.Key {
			exact = append(exact, entry)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return analysistarget.TargetCatalogEntry{}, fmt.Errorf(
			"--target %q matches more than one exact target", override,
		)
	}

	aliases := make([]analysistarget.TargetCatalogEntry, 0, 1)
	for _, entry := range catalog.Entries {
		target := entry.Candidate.Target
		match := override == entry.DisplayPath
		if target.Kind == analysistarget.KindModuleLibrary {
			match = match || override == target.ModulePath
		} else {
			match = match || override == target.PackagePath
		}
		if match {
			aliases = append(aliases, entry)
		}
	}
	switch len(aliases) {
	case 1:
		return aliases[0], nil
	case 0:
		return analysistarget.TargetCatalogEntry{}, fmt.Errorf(
			"--target %q is not an eligible module surface; choose one of: %s",
			override, targetPortfolioChoices(catalog),
		)
	default:
		keys := make([]string, 0, len(aliases))
		for _, entry := range aliases {
			keys = append(keys, entry.Candidate.Key)
		}
		return analysistarget.TargetCatalogEntry{}, fmt.Errorf(
			"--target %q is ambiguous; use one exact target key: %s",
			override, strings.Join(keys, ", "),
		)
	}
}

func recordTargetPortfolioOutcome(
	runDir string,
	outcome targetPortfolioRunOutcome,
	output *runOutput,
) error {
	if outcome.SemanticState == "" {
		return nil
	}
	var exchange *debugdump.SemanticExchange
	alreadyRecorded := false
	responseUnavailable := outcome.ResponseUnavailable
	if len(outcome.Response) == 0 && responseUnavailable == nil {
		responseUnavailable = &debugdump.SemanticUnavailable{
			Code: debugdump.SemanticUnavailableNoContent, OriginalBytes: outcome.ResponseBytes,
		}
	}
	if outcome.RequestBytes > 0 && len(outcome.Request) > 0 {
		value := debugdump.SemanticExchange{
			Stage:           debugdump.SemanticStageTargetPortfolio,
			InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
			RequestProvenance: outcome.RequestProvenance,
			State:             outcome.SemanticState, ValidationCode: outcome.ValidationCode,
			SemanticCalls: outcome.SemanticCalls, TransportAttempts: outcome.TransportAttempts,
			Request: outcome.Request, Response: outcome.Response,
			ResponseUnavailable: responseUnavailable,
		}
		var err error
		alreadyRecorded, err = targetPortfolioSemanticExchangeRecorded(runDir, value)
		if err != nil {
			return err
		}
		exchange = &value
	}

	var diagnosticErr error
	metadataPath := filepath.Join(runDir, "metadata.json")
	if _, err := os.Lstat(metadataPath); err == nil {
		diagnosticErr = recordSemanticStageDiagnostic(runDir, targetPortfolioDiagnostic(outcome))
	} else if !os.IsNotExist(err) {
		diagnosticErr = fmt.Errorf("target portfolio: inspect metadata authority: %w", err)
	}
	if exchange == nil || alreadyRecorded {
		return diagnosticErr
	}
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return errors.Join(
			diagnosticErr,
			fmt.Errorf("target portfolio: open semantic exchange writer: %w", err),
		)
	}
	defer writer.Close()
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "Target portfolio semantic exchange journal unavailable",
	})
	writer.RecordSemanticExchange(*exchange)
	return diagnosticErr
}

func targetPortfolioSemanticExchangeRecorded(
	runDir string,
	exchange debugdump.SemanticExchange,
) (bool, error) {
	directory := filepath.Join(runDir, debugdump.SemanticExchangesDir)
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("target portfolio: inspect semantic exchange journal: %w", err)
	}
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			return false, fmt.Errorf("target portfolio: semantic exchange journal contains a non-directory entry")
		}
		metadataPath := filepath.Join(directory, entry.Name(), debugdump.SemanticExchangeMetaFile)
		info, err := os.Lstat(metadataPath)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSemanticMetadataBytes {
			return false, fmt.Errorf("target portfolio: semantic exchange journal contains invalid metadata")
		}
		encoded, err := os.ReadFile(metadataPath)
		if err != nil {
			return false, fmt.Errorf("target portfolio: read semantic exchange metadata: %w", err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(encoded, &record); err != nil {
			return false, fmt.Errorf("target portfolio: decode semantic exchange metadata: %w", err)
		}
		if record.Stage != debugdump.SemanticStageTargetPortfolio {
			continue
		}
		if found || !sameTargetPortfolioSemanticExchange(record, exchange) {
			return false, fmt.Errorf("target portfolio: conflicting semantic exchange is already recorded")
		}
		found = true
	}
	return found, nil
}

func sameTargetPortfolioSemanticExchange(
	record debugdump.SemanticExchangeRecord,
	exchange debugdump.SemanticExchange,
) bool {
	requestSHA256 := targetPortfolioPayloadSHA256(exchange.Request)
	if record.Version <= 0 || record.Stage != exchange.Stage ||
		record.InstanceOrdinal != exchange.InstanceOrdinal ||
		record.SemanticAttemptOrdinal != exchange.SemanticAttemptOrdinal ||
		record.RequestSHA256 != requestSHA256 ||
		record.RequestProvenance != exchange.RequestProvenance ||
		record.State != exchange.State || record.ValidationCode != exchange.ValidationCode ||
		record.SemanticCalls != exchange.SemanticCalls ||
		record.TransportAttempts != exchange.TransportAttempts ||
		!sameTargetPortfolioPayloadIdentity(record.Request, exchange.Request, nil) ||
		!sameTargetPortfolioPayloadIdentity(record.Response, exchange.Response, exchange.ResponseUnavailable) {
		return false
	}
	return true
}

func sameTargetPortfolioPayloadIdentity(
	record debugdump.SemanticPayloadRecord,
	raw []byte,
	unavailable *debugdump.SemanticUnavailable,
) bool {
	if unavailable != nil {
		return len(raw) == 0 && record.Storage == "raw_unavailable" &&
			record.OriginalSHA256 == unavailable.OriginalSHA256 &&
			record.OriginalBytes == unavailable.OriginalBytes &&
			record.UnavailableCode == unavailable.Code
	}
	return record.OriginalSHA256 == targetPortfolioPayloadSHA256(raw) &&
		record.OriginalBytes == len(raw) && record.UnavailableCode == ""
}

func targetPortfolioPayloadSHA256(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("%x", digest)
}

// persistReadmeRoleAuthority writes the exact accepted first-layer role
// catalog that later cubes consume. Empty input has exact absent authority;
// any failure for non-empty input is terminal rather than a logging warning.
func persistReadmeRoleAuthority(
	runDir string,
	rows []readmeRoleLogRow,
) error {
	if len(rows) == 0 {
		if err := os.Remove(filepath.Join(runDir, readmetargetscout.ArtifactFilename)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("README file-role authority: remove stale empty artifact: %w", err)
		}
		return nil
	}
	payload, err := json.MarshalIndent(struct {
		Version int                `json:"version"`
		Files   []readmeRoleLogRow `json:"files"`
	}{Version: 1, Files: rows}, "", "  ")
	if err != nil {
		return fmt.Errorf("README file-role authority: encode: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		return fmt.Errorf("README file-role authority: open writer: %w", err)
	}
	writeErr := writer.WriteValidatedFile(
		readmetargetscout.ArtifactFilename,
		payload,
		func(saved []byte) error {
			if !bytes.Equal(saved, payload) {
				return fmt.Errorf("README file-role authority differs from the accepted catalog")
			}
			return validateReadmeRoleLog(saved)
		},
	)
	closeErr := writer.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("README file-role authority: persist: %w", err)
	}
	return nil
}

func validateReadmeRoleLog(raw []byte) error {
	var payload struct {
		Version int                `json:"version"`
		Files   []readmeRoleLogRow `json:"files"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || payload.Version != 1 || payload.Files == nil {
		return fmt.Errorf("invalid README file-role log")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("invalid trailing README file-role log data")
	}
	seenFiles := make(map[corpus.FileID]struct{}, len(payload.Files))
	for _, row := range payload.Files {
		if row.FileRef == "" || row.Path == "" || len(row.Classifications) == 0 {
			return fmt.Errorf("invalid README file-role log row")
		}
		if _, duplicate := seenFiles[row.FileRef]; duplicate {
			return fmt.Errorf("duplicate README file-role log row")
		}
		seenFiles[row.FileRef] = struct{}{}
		seenClasses := make(map[readmetargetscout.FileClass]struct{}, len(row.Classifications))
		for _, classification := range row.Classifications {
			if !validReadmeRoleClass(classification.Class) || len(classification.Hypotheses) == 0 {
				return fmt.Errorf("invalid README file-role log classification")
			}
			if _, duplicate := seenClasses[classification.Class]; duplicate {
				return fmt.Errorf("duplicate README file-role log classification")
			}
			seenClasses[classification.Class] = struct{}{}
		}
	}
	return nil
}

func validReadmeRoleClass(value readmetargetscout.FileClass) bool {
	switch value {
	case readmetargetscout.ClassTargetEntry,
		readmetargetscout.ClassExampleEntry,
		readmetargetscout.ClassTestEntry,
		readmetargetscout.ClassSupportToolEntry,
		readmetargetscout.ClassConfiguration,
		readmetargetscout.ClassDatabaseAsset,
		readmetargetscout.ClassClientEntry,
		readmetargetscout.ClassDocumentation,
		readmetargetscout.ClassDeployment,
		readmetargetscout.ClassInterfaceContract:
		return true
	default:
		return false
	}
}

func targetPortfolioDiagnostic(outcome targetPortfolioRunOutcome) semanticStageDiagnostic {
	return semanticStageDiagnostic{
		Stage: debugdump.SemanticStageTargetPortfolio, State: outcome.SemanticState,
		RequestBytes: outcome.RequestBytes, SemanticCalls: outcome.SemanticCalls,
		TransportAttempts: outcome.TransportAttempts, LatencyMillis: outcome.LatencyMillis,
	}
}
