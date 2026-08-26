package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

type jsTSProjectDiscoverer func(
	context.Context,
	*corpus.Corpus,
	string,
	string,
) (jstsproject.Result, error)

// jsTSTargetScout establishes every exact package-target authority from the
// shared corpus without invoking the TypeScript compiler. Full project
// discovery is execution work and runs only after each target is selected.
type jsTSTargetScout func(
	context.Context,
	*corpus.Corpus,
	string,
) ([]jstsproject.Target, error)

// jsTSTargetRunSelection is the page-local bridge from one exact package target
// to its compiler-backed project. Browser, server, command-line,
// shared-contract, and tool surfaces remain within that package target; they
// are not sibling target pages or RuntimePortfolio process roles.
type jsTSTargetRunSelection struct {
	Project jstsproject.Result
	Outcome targetPortfolioRunOutcome
}

// selectJSTSTargetForRun is the compatibility single-page selector. The
// ordinary multi-target path uses ScoutTargets plus repository-wide planning,
// then materializes every retained exact selector. This helper merges one
// package.json candidate with guidance for that same file; the model never
// receives the project ref or selector.
func selectJSTSTargetForRun(
	ctx context.Context,
	repoName string,
	repositoryRoot string,
	repository *corpus.Corpus,
	override string,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
	discover jsTSProjectDiscoverer,
) (jsTSTargetRunSelection, error) {
	if repository == nil {
		return jsTSTargetRunSelection{}, fmt.Errorf("JavaScript/TypeScript target selection: repository corpus is unavailable")
	}
	if discover == nil {
		return jsTSTargetRunSelection{}, fmt.Errorf("JavaScript/TypeScript target selection: project discoverer is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	parallelContext, cancelParallel := context.WithCancel(ctx)
	defer cancelParallel()

	type discoveryResult struct {
		project jstsproject.Result
		err     error
	}
	discovery := make(chan discoveryResult, 1)
	go func() {
		started := time.Now()
		project, err := discover(
			parallelContext, repository, repositoryRoot, exactJSTSManifestSelector(override),
		)
		if err == nil && output != nil {
			output.State(
				"JavaScript/TypeScript project discovery", "ready",
				"language: "+project.Project.Language,
				"target kind: "+jstsproject.TargetKind(project),
				fmt.Sprintf("repository source files: %d", len(project.Files)),
				fmt.Sprintf("product surfaces: %d", jsTSProductSurfaceCount(project)),
				fmt.Sprintf("all classified surfaces: %d", len(project.Surfaces)),
				formatRunOutputWallDuration(time.Since(started)),
			)
		}
		discovery <- discoveryResult{project: project, err: err}
	}()

	readmeResult := make(chan struct {
		roles readmetargetscout.Result
		err   error
	}, 1)
	go func() {
		if !readmetargetscout.HasGuidanceFiles(repository) {
			readmeResult <- struct {
				roles readmetargetscout.Result
				err   error
			}{roles: readmetargetscout.Result{}}
			return
		}
		roles, err := discoverReadmeFileRoles(
			parallelContext, repoName, repository, output, providers, executor,
		)
		readmeResult <- struct {
			roles readmetargetscout.Result
			err   error
		}{roles: roles, err: err}
	}()

	discovered := <-discovery
	if discovered.err != nil {
		cancelParallel()
		<-readmeResult
		if jsTSOwnerPreparationError(discovered.err) {
			return jsTSTargetRunSelection{}, fmt.Errorf(
				"discover JavaScript/TypeScript package project: %w; the owner must prepare repository-local dependencies with the project's normal install command (for example npm ci) before running repomap; repomap never installs packages",
				discovered.err,
			)
		}
		return jsTSTargetRunSelection{}, fmt.Errorf(
			"discover JavaScript/TypeScript package project: %w", discovered.err,
		)
	}
	if err := discovered.project.Validate(); err != nil {
		cancelParallel()
		<-readmeResult
		return jsTSTargetRunSelection{}, fmt.Errorf(
			"validate JavaScript/TypeScript package project: %w", err,
		)
	}
	manifestRef, err := validateJSTSProjectCorpusBinding(repository, discovered.project)
	if err != nil {
		cancelParallel()
		<-readmeResult
		return jsTSTargetRunSelection{}, fmt.Errorf(
			"bind JavaScript/TypeScript package project to the current repository: %w", err,
		)
	}

	readme := <-readmeResult
	if readme.err != nil {
		return jsTSTargetRunSelection{}, readme.err
	}
	readmeRows := compileReadmeRoleLog(repository, readme.roles)
	override = strings.TrimSpace(override)
	if override != "" {
		if override != discovered.project.Project.Ref && override != discovered.project.Project.Selector {
			return jsTSTargetRunSelection{}, fmt.Errorf(
				"--target %q is not the exact JavaScript/TypeScript package project; use %s",
				override, discovered.project.Project.Selector,
			)
		}
		outcome := jsTSTargetSelectionOutcome(discovered.project, readmeRows)
		if output != nil {
			output.State(
				"Target hypothesis merge", "not needed",
				"reason: explicit --target bypasses candidate merging",
			)
			output.State(
				"Analysis target", "selected",
				"source: explicit --target",
				"selected: "+discovered.project.Project.Name,
			)
		}
		return jsTSTargetRunSelection{Project: discovered.project.Snapshot(), Outcome: outcome}, nil
	}

	native := []analysistarget.FileCandidate{{
		FileRef: manifestRef,
		Hypotheses: []string{
			"JavaScript/TypeScript package project with exact manifest, configuration, source, and producer-classified surface evidence",
		},
	}}
	readmeCandidates, unsupported := jsTSResolvableReadmeTargetCandidates(
		readme.roles.TargetCandidates(), manifestRef,
	)
	if output != nil && unsupported > 0 {
		output.Stage(
			"Repository guidance classifier",
			fmt.Sprintf(
				"kept %d target hypotheses for the JavaScript/TypeScript adapter; retained %d unsupported target roles only in diagnostics",
				len(readmeCandidates), unsupported,
			),
		)
	}
	mergeStarted := time.Now()
	merged, err := analysistarget.MergeFileCandidates(
		repository.Snapshot(), native, readmeCandidates,
	)
	if err != nil {
		return jsTSTargetRunSelection{}, fmt.Errorf(
			"merge JavaScript/TypeScript target hypotheses: %w", err,
		)
	}
	if output != nil {
		output.State(
			"Target hypothesis merge", "complete",
			fmt.Sprintf("native hypotheses: %d", len(native)),
			fmt.Sprintf("guidance hypotheses: %d", len(readmeCandidates)),
			fmt.Sprintf("merged hypotheses: %d", len(merged)),
			formatRunOutputWallDuration(time.Since(mergeStarted)),
		)
	}
	portfolio, outcome, err := selectTargetPortfolioForRun(
		ctx, repository.Snapshot(), merged, nil, nil, output, providers, executor,
	)
	outcome.ReadmeRoles = readmeRows
	if err != nil {
		return jsTSTargetRunSelection{}, withTargetPortfolioChoices(
			err,
			targetPortfolioChoiceGroup{
				Language: "JavaScript/TypeScript",
				Choices:  discovered.project.Project.Selector + " (" + discovered.project.Project.ManifestPath + ")",
			},
		)
	}
	if portfolio.Default == nil || portfolio.Default.FileRef != manifestRef ||
		len(portfolio.Targets) != 1 || portfolio.Targets[0].FileRef != manifestRef {
		return jsTSTargetRunSelection{}, fmt.Errorf(
			"restore JavaScript/TypeScript target: TargetPortfolio did not retain the one exact package project",
		)
	}
	outcome.SelectedRef = discovered.project.Project.Ref
	outcome.SelectedTargets = 1
	outcome.SelectedTargetRefs = []string{discovered.project.Project.Ref}
	outcome.SelectedFileRefs = 1
	outcome.UnclassifiedFiles = len(portfolio.Unclassified)
	return jsTSTargetRunSelection{
		Project: discovered.project.Snapshot(), Outcome: outcome,
	}, nil
}

func exactJSTSManifestSelector(override string) string {
	override = strings.TrimSpace(override)
	if strings.HasPrefix(override, "jsts:") {
		return override
	}
	return ""
}

func jsTSOwnerPreparationError(err error) bool {
	return errors.Is(err, jstsproject.ErrTypeScriptCompilerUnavailable)
}

func jsTSProductSurfaceCount(result jstsproject.Result) int {
	count := 0
	for _, surface := range result.Surfaces {
		if surface.Role == jstsproject.SurfaceProduct &&
			(surface.Kind == jstsproject.SurfaceBrowser || surface.Kind == jstsproject.SurfaceServer || surface.Kind == jstsproject.SurfaceCLI) {
			count++
		}
	}
	return count
}

func validateJSTSProjectCorpusBinding(
	repository *corpus.Corpus,
	result jstsproject.Result,
) (corpus.FileID, error) {
	if repository == nil || result.CorpusSHA256 != repository.SHA256() {
		return "", fmt.Errorf("project corpus identity does not match")
	}
	exactFileRef := func(filePath, fileRef string) error {
		ref, ok := repository.ID(filePath)
		if !ok || string(ref) != fileRef {
			return fmt.Errorf("project file %q is not bound to its exact current FileRef", filePath)
		}
		return nil
	}
	if path.Base(result.Project.ManifestPath) != "package.json" || result.Project.Selector != "jsts:"+result.Project.ManifestPath {
		return "", fmt.Errorf("project manifest/selector identity is invalid")
	}
	if err := exactFileRef(result.Project.ManifestPath, result.Project.ManifestFileRef); err != nil {
		return "", err
	}
	if result.Project.ConfigPath != "" {
		if err := exactFileRef(result.Project.ConfigPath, result.Project.ConfigFileRef); err != nil {
			return "", err
		}
	}
	if result.Project.LockfilePath != "" {
		if err := exactFileRef(result.Project.LockfilePath, result.Project.LockfileFileRef); err != nil {
			return "", err
		}
	}
	for _, file := range result.Project.ToolConfigs {
		if err := exactFileRef(file.Path, file.FileRef); err != nil {
			return "", err
		}
	}
	for _, binary := range result.Project.Binaries {
		if err := exactFileRef(binary.Path, binary.FileRef); err != nil {
			return "", err
		}
	}
	for _, file := range result.Files {
		if err := exactFileRef(file.Path, file.FileRef); err != nil {
			return "", err
		}
	}
	manifestRef, _ := repository.ID(result.Project.ManifestPath)
	return manifestRef, nil
}

func validateJSTSTargetMaterialization(
	repository *corpus.Corpus,
	target jstsproject.Target,
	result jstsproject.Result,
) error {
	if err := target.ValidateAgainst(repository); err != nil {
		return fmt.Errorf("scout target: %w", err)
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("project result: %w", err)
	}
	if _, err := validateJSTSProjectCorpusBinding(repository, result); err != nil {
		return fmt.Errorf("project corpus binding: %w", err)
	}
	materialized, err := jstsproject.TargetFromResult(result)
	if err != nil {
		return fmt.Errorf("restore materialized target: %w", err)
	}
	if err := target.ValidateMaterialization(materialized); err != nil {
		return err
	}
	return nil
}

func rebindMaterializedJSTSTarget(
	target repositoryTypedTarget,
	result jstsproject.Result,
) (repositoryTypedTarget, error) {
	if target.Key.Adapter != repositoryTargetAdapterJSTS || target.JSTS == nil {
		return repositoryTypedTarget{}, fmt.Errorf("selected target is not JavaScript/TypeScript")
	}
	materialized, err := jstsproject.TargetFromResult(result)
	if err != nil {
		return repositoryTypedTarget{}, fmt.Errorf("restore materialized target: %w", err)
	}
	if err := target.JSTS.ValidateMaterialization(materialized); err != nil {
		return repositoryTypedTarget{}, err
	}
	rebound := target
	rebound.JSTS = &materialized
	if err := rebound.Validate(); err != nil {
		return repositoryTypedTarget{}, fmt.Errorf("rebind materialized target: %w", err)
	}
	return rebound, nil
}

func jsTSResolvableReadmeTargetCandidates(
	candidates []analysistarget.FileCandidate,
	manifestRef corpus.FileID,
) ([]analysistarget.FileCandidate, int) {
	result := make([]analysistarget.FileCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.FileRef == manifestRef {
			result = append(result, candidate)
		}
	}
	if result == nil {
		result = []analysistarget.FileCandidate{}
	}
	return result, len(candidates) - len(result)
}

func jsTSTargetSelectionOutcome(
	project jstsproject.Result,
	readmeRows []readmeRoleLogRow,
) targetPortfolioRunOutcome {
	return targetPortfolioRunOutcome{
		SelectedRef: project.Project.Ref, SelectedTargets: 1,
		SelectedTargetRefs: []string{project.Project.Ref},
		ReadmeRoles:        cloneReadmeRoleLog(readmeRows),
	}
}
