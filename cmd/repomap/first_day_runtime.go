package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

type orientationRunner func(
	context.Context,
	llm.Executor,
	llm.Provider,
	orientation.Input,
) (orientation.Result, []orientation.RejectedRow, error)

// firstDayOptions carries what the three first-day stages read. Every value is
// already validated repository authority; the stages add no new inputs.
type firstDayOptions struct {
	RepoPath         string
	RepositoryName   string
	Revision         string
	Corpus           *corpus.Corpus
	TrackedPaths     []string
	Runs             []targetPublishedRun
	CacheRoot        string
	NoCache          bool
	BatchConcurrency int
	BatchController  *llm.BatchController
	ProviderFactory  targetPortfolioProviderFactory
	Runner           orientationRunner
	Output           *runOutput
}

// buildFirstDayLayers derives the deterministic fact and claim layers, asks
// the model for one orientation over them, and persists all three into every
// analyzed run directory. A failure here never discards a completed page: the
// caller keeps publishing and the report renders the sections it has.
func buildFirstDayLayers(ctx context.Context, options firstDayOptions) error {
	factsResult, err := buildRepositoryFacts(options)
	if err != nil {
		return err
	}
	claimsResult, err := buildRepositoryClaims(ctx, options)
	if err != nil {
		return err
	}
	orientationResult, rejected, err := runRepositoryOrientation(
		ctx, options, factsResult, claimsResult,
	)
	if err != nil {
		return err
	}
	for _, run := range options.Runs {
		if err := facts.Persist(run.RunDir, factsResult); err != nil {
			return err
		}
		if err := claims.Persist(run.RunDir, claimsResult); err != nil {
			return err
		}
		if err := orientation.Persist(run.RunDir, orientationResult); err != nil {
			return err
		}
		if err := orientation.PersistRejected(run.RunDir, rejected); err != nil {
			return err
		}
	}
	return nil
}

func buildRepositoryFacts(options firstDayOptions) (facts.Result, error) {
	targets := make([]facts.TargetInput, 0, len(options.Runs))
	for _, run := range options.Runs {
		index, err := readRunProgramIndex(run.RunDir)
		if err != nil {
			return facts.Result{}, err
		}
		target := facts.TargetInput{Index: index, RunID: run.RunID}
		if catalog, err := readRunDependencyCatalog(run.RunDir); err == nil {
			target.Dependencies = catalog
		}
		targets = append(targets, target)
	}
	if options.Output != nil {
		options.Output.Stage("Facts", "extracting anchored repository facts")
	}
	started := time.Now()
	result, err := facts.Build(facts.Input{
		Revision:     options.Revision,
		Repository:   options.Corpus,
		TrackedPaths: options.TrackedPaths,
		Targets:      targets,
	})
	if err != nil {
		return facts.Result{}, fmt.Errorf("repository facts: %w", err)
	}
	if options.Output != nil {
		options.Output.State(
			"Facts", "ready",
			fmt.Sprintf("anchored facts: %d", len(result.Facts)),
			formatRunOutputWallDuration(time.Since(started)),
		)
	}
	return result, nil
}

func buildRepositoryClaims(ctx context.Context, options firstDayOptions) (claims.Result, error) {
	roots := make([]claims.TargetRoot, 0, len(options.Runs))
	for _, run := range options.Runs {
		index, err := readRunProgramIndex(run.RunDir)
		if err != nil {
			return claims.Result{}, err
		}
		roots = append(roots, claims.TargetRoot{
			ID: index.Target.ID, Root: filepath.ToSlash(filepath.Dir(runTargetAnchorPath(index))),
		})
	}
	result, err := claims.Extract(ctx, claims.Input{
		Revision:   options.Revision,
		RepoPath:   options.RepoPath,
		Repository: options.Corpus,
		Targets:    roots,
	})
	if err != nil {
		return claims.Result{}, fmt.Errorf("repository claims: %w", err)
	}
	if options.Output != nil {
		options.Output.State("Claims", "ready", fmt.Sprintf("quoted claims: %d", len(result.Claims)))
	}
	return result, nil
}

func runRepositoryOrientation(
	ctx context.Context,
	options firstDayOptions,
	factsResult facts.Result,
	claimsResult claims.Result,
) (orientation.Result, []orientation.RejectedRow, error) {
	indexes := make([]groupindex.Index, 0, len(options.Runs))
	for _, run := range options.Runs {
		indexes = append(indexes, run.GroupIndex.Snapshot())
	}
	runner := options.Runner
	if runner == nil {
		runner = orientation.Run
	}
	var provider llm.Provider
	if options.ProviderFactory != nil {
		created, err := options.ProviderFactory()
		if err != nil {
			return orientation.Result{}, nil, fmt.Errorf("orientation: configure provider: %w", err)
		}
		provider = created
	}
	owner := options.Runs[0].RunDir
	writer, err := debugdump.OpenWriter(owner, false)
	if err != nil {
		return orientation.Result{}, nil, fmt.Errorf("orientation: open artifact writer: %w", err)
	}
	defer writer.Close()
	observer := debugdump.NewSemanticObserver(writer)
	executor := debugdump.BindStage(llm.Executor{
		RootDir: options.CacheRoot, Enabled: !options.NoCache, Observer: observer,
		BatchConcurrency: options.BatchConcurrency, BatchController: options.BatchController,
	}, debugdump.SemanticStageOrientation)

	if options.Output != nil {
		options.Output.Stage("Orientation", "asking for roles, a run recipe and the main flow")
	}
	started := time.Now()
	result, rejected, err := runner(ctx, executor, provider, orientation.Input{
		RepositoryName: options.RepositoryName,
		Facts:          factsResult,
		Claims:         claimsResult,
		Groups:         indexes,
	})
	if err != nil {
		return orientation.Result{}, nil, fmt.Errorf("orientation: %w", err)
	}
	if options.Output != nil {
		details := []string{
			fmt.Sprintf("roles: %d", len(result.Roles)),
			fmt.Sprintf("run steps: %d", len(result.RunRecipe)),
			fmt.Sprintf("flow steps: %d", len(result.MainFlow.Steps)),
			formatRunOutputWallDuration(time.Since(started)),
		}
		if len(rejected) > 0 {
			details = append(details, fmt.Sprintf("discarded response rows: %d", len(rejected)))
		}
		options.Output.State("Orientation", "ready", details...)
	}
	return result, rejected, nil
}

func readRunProgramIndex(runDir string) (programindex.Index, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, programindex.ArtifactFilename))
	if err != nil {
		return programindex.Index{}, fmt.Errorf("first-day layers: read program index: %w", err)
	}
	index, err := programindex.Decode(raw)
	if err != nil {
		return programindex.Index{}, fmt.Errorf("first-day layers: decode program index: %w", err)
	}
	return index, nil
}

func readRunDependencyCatalog(runDir string) (*dependencies.Catalog, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, dependencies.ArtifactFilename))
	if err != nil {
		return nil, err
	}
	catalog, err := dependencies.Decode(raw)
	if err != nil {
		return nil, err
	}
	return &catalog, nil
}

// runTargetAnchorPath is the repository-relative file a target is anchored to.
func runTargetAnchorPath(index programindex.Index) string {
	for _, source := range index.Target.Sources {
		if source.FileRef == index.Target.AnchorFileRef {
			return source.Path
		}
	}
	if len(index.Target.Sources) > 0 {
		return index.Target.Sources[0].Path
	}
	return "."
}

// repositoryTrackedPaths lists the unfiltered tracked paths. It exists so a
// committed environment file can be reported as present by path alone; its
// contents are never read. A listing failure yields no paths rather than
// failing the run.
func repositoryTrackedPaths(ctx context.Context, repoPath string) []string {
	listing, err := gitfiles.ListWithModesContext(ctx, repoPath)
	if err != nil {
		return nil
	}
	return listing.Paths
}
