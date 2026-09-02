package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programcategorization"
	"github.com/dvordrova/repomap/internal/programindex"
)

type programCategorizationRunner func(
	context.Context,
	llm.Executor,
	llm.Provider,
	programindex.Index,
	documentationreduce.Result,
) (programcategorization.Result, error)

// enrichProgramIndexForRun executes semantic categorization for the one exact
// ProgramIndex owned by this target run. The exact repository reduction is
// persisted first as the inspectable input layer.
func enrichProgramIndexForRun(
	ctx context.Context,
	runDir string,
	cacheRoot string,
	noCache bool,
	batchConcurrency int,
	batchController *llm.BatchController,
	providerFactory targetPortfolioProviderFactory,
	runner programCategorizationRunner,
	documentation documentationreduce.Result,
	base programindex.Index,
	output *runOutput,
) (programindex.Index, error) {
	if err := base.Validate(); err != nil {
		return programindex.Index{}, fmt.Errorf("program categorization: validate base ProgramIndex: %w", err)
	}
	if base.Categorization != nil {
		return programindex.Index{}, fmt.Errorf(
			"program categorization: target %s is already enriched", base.Target.ID,
		)
	}
	ownedDocumentation, err := documentation.Snapshot()
	if err != nil {
		return programindex.Index{}, fmt.Errorf("program categorization: own reduced documentation: %w", err)
	}
	if err := documentationreduce.Persist(runDir, ownedDocumentation); err != nil {
		return programindex.Index{}, fmt.Errorf("program categorization: persist reduced documentation: %w", err)
	}
	providerRequired := runner == nil
	if runner == nil {
		runner = programcategorization.Run
	}
	var provider llm.Provider
	if providerFactory != nil {
		provider, err = providerFactory()
		if err != nil {
			return programindex.Index{}, fmt.Errorf("program categorization: configure provider: %w", err)
		}
	}
	if providerRequired && provider == nil {
		return programindex.Index{}, fmt.Errorf("program categorization: configured model provider is unavailable")
	}

	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return programindex.Index{}, fmt.Errorf("program categorization: open artifact writer: %w", err)
	}
	observer := debugdump.NewSemanticObserver(writer)
	executor := debugdump.BindStage(llm.Executor{
		RootDir:          cacheRoot,
		Enabled:          !noCache,
		Observer:         observer,
		BatchConcurrency: batchConcurrency,
		BatchController:  batchController,
	}, debugdump.SemanticStageProgramCategorization)

	if output != nil {
		output.Stage("Program categorization", base.Target.Name)
	}
	started := time.Now()
	result, runErr := runner(ctx, executor, provider, base, ownedDocumentation)
	if runErr != nil {
		return programindex.Index{}, errors.Join(fmt.Errorf(
			"program categorization: target %s: %w", base.Target.ID, runErr,
		), writer.Close())
	}
	index, enrichErr := result.Enrich(base, ownedDocumentation)
	if enrichErr != nil {
		return programindex.Index{}, errors.Join(fmt.Errorf(
			"program categorization: enrich target %s: %w", base.Target.ID, enrichErr,
		), writer.Close())
	}
	if output != nil {
		discarded := 0
		for _, diagnostic := range result.Diagnostics {
			discarded += diagnostic.Count
		}
		output.State(
			"Program categorization", "ready",
			"target: "+base.Target.Name,
			fmt.Sprintf("categorized subjects: %d", len(result.Assignments)),
			fmt.Sprintf("discarded response rows: %d", discarded),
			formatRunOutputWallDuration(time.Since(started)),
		)
	}
	closeErr := writer.Close()
	reportSemanticOrdinalScaleWarnings(
		output,
		"Program categorization",
		nil,
		observer.OrdinalScaleWarnings(),
	)
	if closeErr != nil {
		return programindex.Index{}, fmt.Errorf("program categorization: close artifact writer: %w", closeErr)
	}
	return index, nil
}
