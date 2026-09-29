package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/extractors"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
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
	Graph            atlas.Graph
	RepoPath         string
	RepositoryName   string
	Revision         string
	Corpus           *corpus.Corpus
	Sources          repositoryFactSources
	Runs             []targetPublishedRun
	CacheRoot        string
	NoCache          bool
	BatchConcurrency int
	BatchController  *llm.BatchController
	ProviderFactory  targetPortfolioProviderFactory
	Runner           orientationRunner
	Output           *runOutput
}

// repositoryFactSources are the repository-wide fact inputs no target page
// produces: the extractors' output and the unfiltered tracked-path listing.
// They start before the first target page and the facts stage waits for them.
type repositoryFactSources struct {
	extraction   *background[extractors.Result]
	trackedPaths *background[[]string]
}

func startRepositoryFactSources(ctx context.Context, repoPath string, repository *corpus.Corpus) repositoryFactSources {
	return repositoryFactSources{
		extraction: startBackground(func() (extractors.Result, error) {
			return extractors.Run(ctx, repoPath, repository)
		}),
		trackedPaths: startBackground(func() ([]string, error) {
			return repositoryTrackedPaths(ctx, repoPath), nil
		}),
	}
}

// wait joins both sources, so nothing reads the corpus after its owner.
func (sources repositoryFactSources) wait() {
	sources.extraction.wait()
	sources.trackedPaths.wait()
}

// buildFirstDayFacts derives and persists the two deterministic layers,
// facts and claims, into the repository owner directory. Claims read no fact,
// so the two are built side by side; a facts failure is still reported first.
// The atlas path stops here; the ordinary path asks for an orientation over
// them next.
func buildFirstDayFacts(ctx context.Context, options firstDayOptions) (facts.Result, claims.Result, error) {
	claimsWork := startBackground(func() (claims.Result, error) {
		return buildRepositoryClaims(ctx, options)
	})
	factsResult, err := buildRepositoryFacts(ctx, options)
	claimsResult, claimsErr := claimsWork.wait()
	if err != nil {
		return facts.Result{}, claims.Result{}, err
	}
	if claimsErr != nil {
		return facts.Result{}, claims.Result{}, claimsErr
	}
	for _, run := range options.Runs[:1] {
		if err := facts.Persist(run.RunDir, factsResult); err != nil {
			return facts.Result{}, claims.Result{}, err
		}
		if err := claims.Persist(run.RunDir, claimsResult); err != nil {
			return facts.Result{}, claims.Result{}, err
		}
	}
	return factsResult, claimsResult, nil
}

func buildRepositoryFacts(ctx context.Context, options firstDayOptions) (facts.Result, error) {
	targets := make([]facts.TargetInput, 0, len(options.Runs))
	for position := range options.Runs {
		run := &options.Runs[position]
		target := facts.TargetInput{Index: programindex.Index{Target: run.programTarget()}, ReadIndex: run.programIndex}
		catalog, err := run.dependencyCatalog()
		if err != nil {
			return facts.Result{}, err
		}
		target.Dependencies = catalog
		targets = append(targets, target)
	}
	if options.Output != nil {
		options.Output.Stage("Facts", "extracting anchored repository facts")
	}
	started := time.Now()
	extraction, extractionErr := options.Sources.extraction.wait()
	data, err := json.MarshalIndent(extraction, "", "  ")
	if err != nil {
		return facts.Result{}, err
	}
	for _, run := range options.Runs[:1] {
		if err := os.WriteFile(filepath.Join(run.RunDir, extractors.ArtifactFilename), data, 0o600); err != nil {
			return facts.Result{}, err
		}
	}
	if extractionErr != nil {
		return facts.Result{}, extractionErr
	}
	trackedPaths, _ := options.Sources.trackedPaths.wait()
	result, err := facts.Build(facts.Input{
		Revision:     options.Revision,
		Repository:   options.Corpus,
		TrackedPaths: trackedPaths,
		Targets:      targets,
		Extractions:  extraction.Extractions,
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
	for position := range options.Runs {
		run := &options.Runs[position]
		index := programindex.Index{Target: run.programTarget()}
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
	writer, err := debugdump.OpenWriter(owner)
	if err != nil {
		return orientation.Result{}, nil, fmt.Errorf("orientation: open artifact writer: %w", err)
	}
	defer writer.Close()
	observer := debugdump.NewSemanticObserver(writer)
	executor := debugdump.BindStage(llm.Executor{
		RootDir: options.CacheRoot, Enabled: !options.NoCache, Observer: timed(options.Output, observer),
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
		Graph:          options.Graph,
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
		state := "ready"
		if len(rejected) > 0 {
			details = append(details, fmt.Sprintf("ignored response items: %d; accepted overview text is kept", len(rejected)))
		}
		for _, row := range rejected {
			if row.Section == "flow_request" {
				// The overview stands; the report opens its entry's calls.
				details = append(details, "the main flow request was refused by size or context: "+row.Reason)
			}
			if row.Section == "request" {
				// The overview did not fit: the report is published without
				// a summary, roles, run recipe or main flow.
				state = "unavailable"
				details = []string{"the orientation request was refused by size or context: " + row.Reason, "the report is published without a written summary, roles, run recipe and main flow"}
				break
			}
		}
		options.Output.State("Orientation", state, details...)
	}
	return result, rejected, nil
}

func readRunProgramIndex(runDir string) (programindex.Index, error) {
	index, err := programindex.ReadFile(filepath.Join(runDir, programindex.ArtifactFilename))
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

// orientationDiagnosticRows folds the orientation stage's refused rows into
// the shared per-run log, keeping each raw model row beside its reason.
func orientationDiagnosticRows(rejected []orientation.RejectedRow) []modeldiag.Row {
	rows := make([]modeldiag.Row, 0, len(rejected))
	for _, row := range rejected {
		rows = append(rows, modeldiag.Row{
			Stage: debugdump.SemanticStageOrientation, Kind: row.Section,
			Count: 1, Reason: row.Reason, Raw: row.Raw,
		})
	}
	return rows
}
