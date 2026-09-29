package run

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
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
	// DeclarationKeys waits for the lookup pass of the group projection,
	// read while the tables wait on the models.
	DeclarationKeys func() (groupindex.DeclarationKeys, error)
	Graph           atlas.Graph
	TablesPath      string
	Atlas           atlas.Atlas
	Facts           facts.Result
	Claims          claims.Result
	Orientation     *orientation.Result
	Questions       []atlas.QuestionRoute
	Learning        *atlas.LearningPlan
}

// readRepositoryAtlas is the atlas path after every target page has its
// program index: facts and claims, then the places graph, then the tables.
// Repository-wide artifacts land once in the owner run.
func readRepositoryAtlas(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	runs []targetPublishedRun,
	sources repositoryFactSources,
) (atlasOutcome, error) {
	owner := runs[0]
	factsResult, claimsResult, err := buildFirstDayFacts(ctx, firstDayOptions{
		RepoPath:       options.Repo,
		RepositoryName: repoRunLabel(options.Repo),
		Revision:       options.RepositoryState.Head,
		Corpus:         options.Corpus,
		Sources:        sources,
		Runs:           runs,
		Output:         options.Output,
	})
	if err != nil {
		return atlasOutcome{}, err
	}

	targets := make([]places.TargetInput, 0, len(runs))
	metas := make([]reading.TargetMeta, 0, len(runs))
	targetIDs := make(map[string]string)
	for i := range runs {
		targetIDs[runs[i].SelectedTargetKey] = runs[i].programTarget().ID
	}
	planned := make(map[string]repositoryTypedTarget)
	var indexReader programindex.FileReader
	defer indexReader.Release()
	for _, target := range options.Plan.Targets {
		planned[target.Key.String()] = target
	}
	for position := range runs {
		run := &runs[position]
		index := programindex.Index{Target: run.programTarget()}
		claimed, root := atlasTargetRoots(index, planned[run.SelectedTargetKey])
		readIndex := run.validProgramIndex
		if run.ProgramIndex == nil {
			filename := filepath.Join(run.RunDir, programindex.ArtifactFilename)
			readIndex = func() (programindex.Index, error) { return indexReader.ReadFile(filename) }
		}
		target := places.TargetInput{Index: index, Root: claimed, AbsorbedRoot: planned[run.SelectedTargetKey].AbsorbedRoot, ReadIndex: readIndex}
		catalog, err := run.dependencyCatalog()
		if err != nil {
			return atlasOutcome{}, err
		}
		target.Dependencies = catalog
		targets = append(targets, target)
		meta := reading.TargetMeta{
			ID: index.Target.ID, Language: index.Target.Language, Kind: index.Target.Kind,
			Name: index.Target.Name, Root: root, Dependencies: externalDependencies(catalog),
		}
		if target, ok := planned[run.SelectedTargetKey]; ok {
			meta.SelectedRole = target.Placement
			if meta.SelectedRole == "standalone" {
				meta.SelectedRole = atlas.RoleProduct
				if strings.Contains(index.Target.Kind, "library") {
					meta.SelectedRole = atlas.RoleLibrary
				}
			}
			for _, shared := range target.SharedCode {
				if id := targetIDs[shared.String()]; id != "" {
					meta.SharedCode = append(meta.SharedCode, id)
				}
			}
		}
		metas = append(metas, meta)
	}
	options.Output.Stage("Atlas places", "building the places graph from the program indexes, claims and corpus")
	started := time.Now()
	graph, err := places.Build(places.Input{
		Revision: options.RepositoryState.Head, Repository: options.Corpus,
		Targets: targets, Claims: claimsResult, Facts: factsResult,
	})
	indexReader.Release()
	if err != nil {
		return atlasOutcome{}, err
	}
	// Seal once: places.json and the reading's saved input share these bytes.
	sealed, err := atlas.EncodeGraph(graph)
	if err != nil {
		return atlasOutcome{}, err
	}
	if err := atlas.WriteGraph(owner.RunDir, sealed); err != nil {
		return atlasOutcome{}, err
	}
	// The group projection looks up the declaration keys of what the atlas
	// cites. With places done, the programs are read for them now, one at a
	// time, beside the tables, in the atlas target order (by name).
	byName := slices.Clone(metas)
	sort.Slice(byName, func(i, j int) bool { return byName[i].Name < byName[j].Name })
	keyTargets := make([]string, len(byName))
	for i, meta := range byName {
		keyTargets[i] = meta.ID
	}
	declarationKeys := startDeclarationKeys(keyTargets, runs)
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
	if options.Categorizer != nil {
		options.Output.State("Categorizer", "ready", "closed tables: "+string(options.Categorizer.State()))
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
	questions := options.Questions
	if !options.Learn {
		questions = nil
		options.Output.State("Questions", "off", "no --learn: skipping generation, retrieval and answers")
	}
	result, err := reading.Read(ctx, reading.Options{
		Graph: graph, SealedGraph: sealed, Targets: metas,
		Repository: repoRunLabel(options.Repo), Revision: options.RepositoryState.Head,
		Executor: executor, Provider: provider, Categorizer: options.Categorizer, OwnerRunDir: owner.RunDir,
		Questions: questions, Learn: options.Learn, NoCaptions: !options.Captions,
		ReadSource: func(path string) ([]byte, error) {
			id, ok := options.Corpus.ID(path)
			if !ok {
				return nil, fmt.Errorf("%s is not in the corpus", path)
			}
			content, err := options.Corpus.ReadFileAll(id)
			if err != nil {
				return nil, err
			}
			return content.Bytes, nil
		},
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
	return atlasOutcome{DeclarationKeys: declarationKeys, Graph: graph, TablesPath: result.TablesPath, Atlas: result.Atlas, Facts: factsResult, Claims: claimsResult, Questions: result.Questions, Learning: result.Learning}, nil
}

// startDeclarationKeys reads the runs' declaration keys in the background
// and returns the wait.
func startDeclarationKeys(targetIDs []string, runs []targetPublishedRun) func() (groupindex.DeclarationKeys, error) {
	programs := make(map[string]*targetPublishedRun, len(runs))
	for position := range runs {
		programs[runs[position].programTarget().ID] = &runs[position]
	}
	type read struct {
		keys groupindex.DeclarationKeys
		err  error
	}
	done := make(chan read, 1)
	go func() {
		keys, err := groupindex.ReadDeclarationKeys(targetIDs, func(id string) (programindex.Index, error) {
			run, ok := programs[id]
			if !ok {
				return programindex.Index{}, fmt.Errorf("atlas: target %s has no program index", id)
			}
			return run.validProgramIndex()
		})
		done <- read{keys: keys, err: err}
	}()
	return sync.OnceValues(func() (groupindex.DeclarationKeys, error) {
		result := <-done
		return result.keys, result.err
	})
}

// projectAtlasRuns gives every run the GroupsIndex the page reads, built
// from the atlas: boxes as groups, zones as containers, arrows and joints as
// connections. The projected index replaces the empty one the child wrote.
func projectAtlasRuns(runs []targetPublishedRun, outcome atlasOutcome, output *runOutput) ([]targetPublishedRun, error) {
	programs := make(map[string]*targetPublishedRun, len(runs))
	for position := range runs {
		run := &runs[position]
		programs[run.programTarget().ID] = run
	}
	var keys *groupindex.DeclarationKeys
	if outcome.DeclarationKeys != nil {
		read, err := outcome.DeclarationKeys()
		if err != nil {
			return nil, err
		}
		keys = &read
	}
	projected, err := groupindex.ProjectAtlasWithKeys(outcome.Atlas, keys, func(id string) (programindex.Index, error) {
		run, ok := programs[id]
		if !ok {
			return programindex.Index{}, fmt.Errorf("atlas: target %s has no program index", id)
		}
		return run.validProgramIndex()
	})
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
		result[position].GroupIndex = index
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
		Graph:            outcome.Graph,
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
	outcome.Graph = atlas.Graph{}
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

// externalDependencies lists a target's external dependencies once each,
// by package path: the record the systems question shows beside an
// outside package's calls.
func externalDependencies(catalog *dependencies.Catalog) []reading.Dependency {
	if catalog == nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []reading.Dependency
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind != dependencies.KindExternal || dependency.PackagePath == "" || seen[dependency.PackagePath] {
			continue
		}
		seen[dependency.PackagePath] = true
		result = append(result, reading.Dependency{Package: dependency.PackagePath, Module: dependency.ModulePath, Version: dependency.ModuleVersion})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Package < result[j].Package })
	return result
}

// atlasTargetRoots are a page's roots in the atlas: its own, the directory
// of its anchor file, which places claims at its own depth, and the root it
// reads as, which is the root of a library folded into it when there is one
// (the executable absorbed its library, so internal and pkg packages, or a
// Python distribution's tests, are its own boxes). A folded library's root
// is claimed beside the program's own, never instead of it: a guard tool
// beside a console script's file keeps sharing that directory with it.
func atlasTargetRoots(index programindex.Index, planned repositoryTypedTarget) (claimed, root string) {
	claimed = filepath.ToSlash(filepath.Dir(runTargetAnchorPath(index)))
	if planned.AbsorbedRoot != "" {
		return claimed, planned.AbsorbedRoot
	}
	return claimed, claimed
}
