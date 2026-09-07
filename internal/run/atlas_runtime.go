package run

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// atlasOutcome is what the atlas path hands to publication: the atlas, the
// facts and claims it was read over, and where the owner's tables are.
type atlasOutcome struct {
	TablesPath  string
	Atlas       atlas.Atlas
	Facts       facts.Result
	Claims      claims.Result
	Orientation *orientation.Result
	Questions   []atlas.QuestionRoute
	Learning    *atlas.LearningPlan
}

// readRepositoryAtlas is the atlas path after every target page has its
// program index: facts and claims, then the places graph, then the tables.
// Repository-wide artifacts land once in the owner run.
func readRepositoryAtlas(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	runs []targetPublishedRun,
) (atlasOutcome, error) {
	owner := runs[0]
	factsResult, claimsResult, err := buildFirstDayFacts(ctx, firstDayOptions{
		RepoPath:       options.Repo,
		RepositoryName: repoRunLabel(options.Repo),
		Revision:       options.RepositoryState.Head,
		Corpus:         options.Corpus,
		TrackedPaths:   repositoryTrackedPaths(ctx, options.Repo),
		Runs:           runs,
		Output:         options.Output,
	})
	if err != nil {
		return atlasOutcome{}, err
	}

	targets := make([]places.TargetInput, 0, len(runs))
	metas := make([]reading.TargetMeta, 0, len(runs))
	for position := range runs {
		run := &runs[position]
		index, err := run.programIndex()
		if err != nil {
			return atlasOutcome{}, err
		}
		root := filepath.ToSlash(filepath.Dir(runTargetAnchorPath(index)))
		target := places.TargetInput{Index: index, Root: root}
		catalog, err := run.dependencyCatalog()
		if err != nil {
			return atlasOutcome{}, err
		}
		target.Dependencies = catalog
		targets = append(targets, target)
		metas = append(metas, reading.TargetMeta{
			ID: index.Target.ID, Language: index.Target.Language, Kind: index.Target.Kind,
			Name: index.Target.Name, Root: root,
		})
	}
	options.Output.Stage("Atlas places", "building the places graph from the program indexes, claims and corpus")
	started := time.Now()
	graph, err := places.Build(places.Input{
		Revision: options.RepositoryState.Head, Repository: options.Corpus,
		Targets: targets, Claims: claimsResult, Facts: factsResult,
	})
	if err != nil {
		return atlasOutcome{}, err
	}
	if err := atlas.PersistGraph(owner.RunDir, graph); err != nil {
		return atlasOutcome{}, err
	}
	dirs, files, boundaries := 0, 0, 0
	for _, place := range graph.Places {
		switch place.Kind {
		case atlas.PlaceDirectory:
			dirs++
		case atlas.PlaceFile:
			files++
		case atlas.PlaceBoundary:
			boundaries++
		}
	}
	options.Output.State("Atlas places", "ready",
		fmt.Sprintf("directories: %d", dirs), fmt.Sprintf("files: %d", files), fmt.Sprintf("boundaries: %d", boundaries),
		fmt.Sprintf("edges: %d", len(graph.Edges)), fmt.Sprintf("seeds: %d", len(graph.Seeds)),
		formatRunOutputWallDuration(time.Since(started)),
	)

	var provider llm.Provider
	if !options.NoModel {
		if options.Deps.newCubeProvider == nil {
			return atlasOutcome{}, fmt.Errorf("atlas: model provider is unavailable; pass --no-model to read the tables without it")
		}
		provider, err = options.Deps.newCubeProvider()
		if err != nil {
			return atlasOutcome{}, fmt.Errorf("atlas: configure provider: %w", err)
		}
	}
	writer, err := debugdump.OpenWriter(owner.RunDir)
	if err != nil {
		return atlasOutcome{}, fmt.Errorf("atlas: open artifact writer: %w", err)
	}
	defer writer.Close()
	executor := llm.Executor{
		RootDir: options.DebugDir, Enabled: !options.NoCache,
		Observer:         timed(options.Output, debugdump.NewSemanticObserver(writer)),
		BatchConcurrency: options.Deps.llmBatchConcurrency,
		BatchController:  options.Deps.llmBatchController,
	}
	result, err := reading.Read(ctx, reading.Options{
		Graph: graph, Targets: metas,
		Repository: repoRunLabel(options.Repo), Revision: options.RepositoryState.Head,
		Executor: executor, Provider: provider, OwnerRunDir: owner.RunDir,
		Questions: options.Questions, Learn: true,
		Stage: options.Output.Stage, State: options.Output.State,
	})
	if err != nil {
		return atlasOutcome{}, err
	}
	if err := modeldiag.Append(owner.RunDir, result.Rejected); err != nil {
		options.Output.Warn("could not record atlas diagnostics", err.Error())
	}
	if err := atlas.Persist(owner.RunDir, result.Atlas); err != nil {
		return atlasOutcome{}, err
	}
	details := []string{
		"tables: " + result.TablesPath,
		"places: " + filepath.Join(owner.RunDir, atlas.GraphFilename),
		"atlas: " + filepath.Join(owner.RunDir, atlas.ArtifactFilename),
	}
	for _, use := range result.Uses {
		details = append(details, fmt.Sprintf(
			"%s: %d rows, %d windows, live %d, cached %d, rejected %d, on fallback %d",
			use.Stage, use.Rows, use.Windows, use.Live, use.Cached, use.Rejected, use.Given,
		))
	}
	options.Output.State("Atlas", "ready", details...)
	return atlasOutcome{TablesPath: result.TablesPath, Atlas: result.Atlas, Facts: factsResult, Claims: claimsResult, Questions: result.Questions, Learning: result.Learning}, nil
}

// projectAtlasRuns gives every run the GroupsIndex the page reads, built
// from the atlas: boxes as groups, zones as containers, arrows and joints as
// connections. The projected index replaces the empty one the child wrote.
func projectAtlasRuns(runs []targetPublishedRun, outcome atlasOutcome, output *runOutput) ([]targetPublishedRun, error) {
	programs := make(map[string]programindex.Index, len(runs))
	for position := range runs {
		run := &runs[position]
		index, err := run.programIndex()
		if err != nil {
			return nil, err
		}
		programs[index.Target.ID] = index
	}
	projected, err := groupindex.ProjectAtlas(programs, outcome.Atlas)
	if err != nil {
		return nil, err
	}
	byTarget := make(map[string]groupindex.Index, len(projected))
	for _, index := range projected {
		byTarget[index.Target.ID] = index
	}
	result := make([]targetPublishedRun, len(runs))
	groups, connections := 0, 0
	for position, run := range runs {
		index, ok := byTarget[run.ProgramPage.ProgramTarget.ID]
		if !ok {
			return nil, fmt.Errorf("atlas: target %s has no projected groups", run.ProgramPage.ProgramTarget.Name)
		}
		result[position] = run
		result[position].GroupIndex = index.Snapshot()
		if err := groupindex.Persist(run.RunDir, index); err != nil {
			return nil, fmt.Errorf("atlas: persist projected groups for %s: %w", run.RunID, err)
		}
		groups += len(index.Groups)
		connections += len(index.Connections)
	}
	output.State("Atlas groups", "ready",
		fmt.Sprintf("targets: %d", len(result)), fmt.Sprintf("boxes as groups: %d", groups),
		fmt.Sprintf("arrows and joints as connections: %d", connections),
	)
	return result, nil
}

// orientAtlasRuns asks the orientation over the projected groups and lands
// it in the repository owner run.
func orientAtlasRuns(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	runs []targetPublishedRun,
	outcome *atlasOutcome,
) error {
	firstDay := firstDayOptions{
		RepoPath:         options.Repo,
		RepositoryName:   repoRunLabel(options.Repo),
		Revision:         options.RepositoryState.Head,
		Corpus:           options.Corpus,
		Runs:             runs,
		CacheRoot:        options.DebugDir,
		NoCache:          options.NoCache,
		BatchConcurrency: options.Deps.llmBatchConcurrency,
		BatchController:  options.Deps.llmBatchController,
		ProviderFactory:  options.Deps.newCubeProvider,
		Runner:           options.Deps.runOrientation,
		Output:           options.Output,
	}
	orientationResult, rejected, err := runRepositoryOrientation(ctx, firstDay, outcome.Facts, outcome.Claims)
	if err != nil {
		return err
	}
	outcome.Orientation = &orientationResult
	for _, run := range runs[:1] {
		if err := orientation.Persist(run.RunDir, orientationResult); err != nil {
			return err
		}
		if err := modeldiag.Append(run.RunDir, orientationDiagnosticRows(rejected)); err != nil {
			return err
		}
	}
	return nil
}
