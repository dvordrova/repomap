package run

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// readRepositoryAtlas is the atlas path after every target page has its
// program index: facts and claims, then the places graph, then the tables.
// places.json, tables.md and tables/ land in the owner run; atlas.json in
// every run. It returns the path of tables.md, the thing the owner reads.
func readRepositoryAtlas(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	runs []targetPublishedRun,
) (string, error) {
	owner := runs[0]
	_, claimsResult, err := buildFirstDayFacts(ctx, firstDayOptions{
		RepoPath:       options.Repo,
		RepositoryName: repoRunLabel(options.Repo),
		Revision:       options.RepositoryState.Head,
		Corpus:         options.Corpus,
		TrackedPaths:   repositoryTrackedPaths(ctx, options.Repo),
		Runs:           runs,
		Output:         options.Output,
	})
	if err != nil {
		return "", err
	}

	targets := make([]places.TargetInput, 0, len(runs))
	metas := make([]reading.TargetMeta, 0, len(runs))
	for _, run := range runs {
		index, err := readRunProgramIndex(run.RunDir)
		if err != nil {
			return "", err
		}
		root := filepath.ToSlash(filepath.Dir(runTargetAnchorPath(index)))
		target := places.TargetInput{Index: index, Root: root}
		if catalog, err := readRunDependencyCatalog(run.RunDir); err == nil {
			target.Dependencies = catalog
		}
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
		Targets: targets, Claims: claimsResult,
	})
	if err != nil {
		return "", err
	}
	if err := atlas.PersistGraph(owner.RunDir, graph); err != nil {
		return "", err
	}
	dirs, files := 0, 0
	for _, place := range graph.Places {
		switch place.Kind {
		case atlas.PlaceDirectory:
			dirs++
		case atlas.PlaceFile:
			files++
		}
	}
	options.Output.State("Atlas places", "ready",
		fmt.Sprintf("directories: %d", dirs), fmt.Sprintf("files: %d", files),
		fmt.Sprintf("edges: %d", len(graph.Edges)), fmt.Sprintf("seeds: %d", len(graph.Seeds)),
		formatRunOutputWallDuration(time.Since(started)),
	)

	var provider llm.Provider
	if !options.NoModel {
		if options.Deps.newCubeProvider == nil {
			return "", fmt.Errorf("atlas: model provider is unavailable; pass --no-model to read the tables without it")
		}
		provider, err = options.Deps.newCubeProvider()
		if err != nil {
			return "", fmt.Errorf("atlas: configure provider: %w", err)
		}
	}
	writer, err := debugdump.OpenWriter(owner.RunDir)
	if err != nil {
		return "", fmt.Errorf("atlas: open artifact writer: %w", err)
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
		Stage: options.Output.Stage, State: options.Output.State,
	})
	if err != nil {
		return "", err
	}
	if err := modeldiag.Append(owner.RunDir, result.Rejected); err != nil {
		options.Output.Warn("could not record atlas diagnostics", err.Error())
	}
	for _, run := range runs {
		if err := atlas.Persist(run.RunDir, result.Atlas); err != nil {
			return "", err
		}
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
	return result.TablesPath, nil
}

// finishAtlasDispatch closes an atlas run: the latest link, the time, and
// where the tables are. No report is served.
func finishAtlasDispatch(debugDir, tablesPath string, output *runOutput) error {
	linkLatest(debugDir, filepath.Dir(tablesPath), runOutputWarningSink{
		output: output, summary: "could not update latest report link",
	})
	output.Timing()
	output.State("Run", "ready", "atlas tables: "+tablesPath)
	return nil
}
