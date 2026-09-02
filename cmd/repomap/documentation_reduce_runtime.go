package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

type documentationReduceRunner func(
	context.Context,
	llm.Executor,
	llm.Provider,
	readmetargetscout.GuidanceSnapshot,
) (documentationreduce.Result, error)

// reduceRepositoryDocumentationForRun owns the repository-wide documentation
// reduction. It executes before any selected target page starts and returns one
// immutable handoff that every child page snapshots. The first-layer observer
// is deliberately pending here because the target run directory does not exist
// until the ordinary deterministic analyzer starts.
func reduceRepositoryDocumentationForRun(
	ctx context.Context,
	cacheRoot string,
	noCache bool,
	batchConcurrency int,
	batchController *llm.BatchController,
	providerFactory targetPortfolioProviderFactory,
	runner documentationReduceRunner,
	observer *debugdump.SemanticObserver,
	guidance readmetargetscout.GuidanceSnapshot,
	output *runOutput,
) (documentationreduce.Result, error) {
	ownedGuidance, err := guidance.Snapshot()
	if err != nil {
		return documentationreduce.Result{}, fmt.Errorf("repository documentation: own guidance: %w", err)
	}
	if runner == nil {
		runner = documentationreduce.Run
	}
	var provider llm.Provider
	if len(ownedGuidance.Documents) > 0 {
		if providerFactory == nil {
			return documentationreduce.Result{}, fmt.Errorf("repository documentation: model provider is unavailable")
		}
		provider, err = providerFactory()
		if err != nil {
			return documentationreduce.Result{}, fmt.Errorf("repository documentation: configure provider: %w", err)
		}
		if provider == nil {
			return documentationreduce.Result{}, fmt.Errorf("repository documentation: configured model provider is unavailable")
		}
	}
	if output != nil {
		output.Stage("Documentation", "reducing repository guidance")
	}
	started := time.Now()
	executor := debugdump.BindStage(llm.Executor{
		RootDir:          cacheRoot,
		Enabled:          !noCache,
		Observer:         observer,
		BatchConcurrency: batchConcurrency,
		BatchController:  batchController,
	}, debugdump.SemanticStageDocumentationReduce)
	result, err := runner(ctx, executor, provider, ownedGuidance)
	if err != nil {
		return documentationreduce.Result{}, fmt.Errorf("repository documentation: reduce guidance: %w", err)
	}
	if err := result.ValidateAgainst(ownedGuidance); err != nil {
		return documentationreduce.Result{}, fmt.Errorf("repository documentation: validate reduction: %w", err)
	}
	ownedResult, err := result.Snapshot()
	if err != nil {
		return documentationreduce.Result{}, fmt.Errorf("repository documentation: own reduction: %w", err)
	}
	if output != nil {
		output.State(
			"Documentation", "ready",
			fmt.Sprintf("guidance documents: %d", len(ownedGuidance.Documents)),
			fmt.Sprintf("reduced sources: %d", len(ownedResult.Sources)),
			formatRunOutputWallDuration(time.Since(started)),
		)
	}
	return ownedResult, nil
}
