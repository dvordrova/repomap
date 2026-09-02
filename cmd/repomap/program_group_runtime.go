package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/groupmatching"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/programgrouping"
	"github.com/dvordrova/repomap/internal/programindex"
)

type programGroupingRunner func(
	context.Context,
	llm.Executor,
	llm.Provider,
	programindex.Index,
) (groupindex.Index, []groupindex.Diagnostic, error)

type groupMatchingRunner func(
	context.Context,
	llm.Executor,
	llm.Provider,
	[]groupindex.Index,
) ([]groupindex.Index, []groupindex.Diagnostic, error)

// groupProgramIndexForRun builds and persists the one GroupsIndex owned by
// this target run.
func groupProgramIndexForRun(
	ctx context.Context,
	runDir string,
	cacheRoot string,
	noCache bool,
	batchConcurrency int,
	batchController *llm.BatchController,
	providerFactory targetPortfolioProviderFactory,
	runner programGroupingRunner,
	program programindex.Index,
	output *runOutput,
) (groupindex.Index, error) {
	if err := program.Validate(); err != nil {
		return groupindex.Index{}, fmt.Errorf("program grouping: validate ProgramIndex: %w", err)
	}
	if program.Categorization == nil {
		return groupindex.Index{}, fmt.Errorf("program grouping: target %s is not categorized", program.Target.ID)
	}
	providerRequired := runner == nil && programIndexNeedsGroupingProvider(program)
	if runner == nil {
		runner = programgrouping.Run
	}
	var provider llm.Provider
	var err error
	if providerFactory != nil {
		provider, err = providerFactory()
		if err != nil {
			return groupindex.Index{}, fmt.Errorf("program grouping: configure provider: %w", err)
		}
	}
	if providerRequired && provider == nil {
		return groupindex.Index{}, fmt.Errorf("program grouping: configured model provider is unavailable")
	}

	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return groupindex.Index{}, fmt.Errorf("program grouping: open artifact writer: %w", err)
	}
	observer := debugdump.NewSemanticObserver(writer)
	executor := debugdump.BindStage(llm.Executor{
		RootDir: cacheRoot, Enabled: !noCache, Observer: observer,
		BatchConcurrency: batchConcurrency, BatchController: batchController,
	}, debugdump.SemanticStageProgramGrouping)

	if output != nil {
		output.Stage("Program grouping", program.Target.Name)
	}
	started := time.Now()
	index, diagnostics, runErr := runner(ctx, executor, provider, program)
	if runErr != nil {
		return groupindex.Index{}, errors.Join(
			fmt.Errorf("program grouping: target %s: %w", program.Target.ID, runErr),
			writer.Close(),
		)
	}
	if err := index.Validate(); err != nil {
		return groupindex.Index{}, errors.Join(
			fmt.Errorf("program grouping: validate target %s result: %w", program.Target.ID, err),
			writer.Close(),
		)
	}
	if index.Target.ID != program.Target.ID || index.ProgramIndexSHA256 != program.SHA256 {
		return groupindex.Index{}, errors.Join(
			fmt.Errorf("program grouping: target %s result changed producer authority", program.Target.ID),
			writer.Close(),
		)
	}
	groupRows := groupDiagnosticRows(
		debugdump.SemanticStageProgramGrouping, program.Target.Name, diagnostics,
	)
	if err := modeldiag.Append(runDir, groupRows); err != nil && output != nil {
		output.Warn("could not record grouping diagnostics", err.Error())
	}
	if output != nil {
		details := []string{
			"target: " + program.Target.Name,
			fmt.Sprintf("groups: %d", len(index.Groups)),
			fmt.Sprintf("local connections: %d", len(index.Connections)),
			fmt.Sprintf("discarded response rows: %d", len(diagnostics)),
		}
		details = append(details, modeldiag.Summary(groupRows)...)
		details = append(details, formatRunOutputWallDuration(time.Since(started)))
		output.State("Program grouping", "ready", details...)
	}
	closeErr := writer.Close()
	reportSemanticOrdinalScaleWarnings(output, "Program grouping", nil, observer.OrdinalScaleWarnings())
	if closeErr != nil {
		return groupindex.Index{}, fmt.Errorf("program grouping: close artifact writer: %w", closeErr)
	}
	if err := groupindex.Persist(runDir, index); err != nil {
		return groupindex.Index{}, err
	}
	return index.Snapshot(), nil
}

// matchPublishedRunGroups performs the one repository-level LLM matching pass
// over target-local graphs, then replaces every run's GroupsIndex only after a
// complete validated set has been returned.
func matchPublishedRunGroups(
	ctx context.Context,
	cacheRoot string,
	noCache bool,
	batchConcurrency int,
	batchController *llm.BatchController,
	providerFactory targetPortfolioProviderFactory,
	runner groupMatchingRunner,
	runs []targetPublishedRun,
	output *runOutput,
) ([]targetPublishedRun, error) {
	if len(runs) == 0 {
		return nil, fmt.Errorf("group matching: completed target run set is empty")
	}
	indexes := make([]groupindex.Index, len(runs))
	for position, run := range runs {
		if err := run.GroupIndex.Validate(); err != nil {
			return nil, fmt.Errorf("group matching: run %s GroupsIndex: %w", run.RunID, err)
		}
		if run.GroupIndex.Target.ID != run.ProgramPage.ProgramTarget.ID {
			return nil, fmt.Errorf("group matching: run %s graph target does not match page target", run.RunID)
		}
		indexes[position] = run.GroupIndex.Snapshot()
	}
	if err := groupindex.ValidateSet(indexes); err != nil {
		return nil, fmt.Errorf("group matching: input set: %w", err)
	}
	defaultRunner := runner == nil
	providerRequired := false
	if defaultRunner {
		var needsErr error
		providerRequired, needsErr = groupmatching.NeedsProvider(indexes)
		if needsErr != nil {
			return nil, fmt.Errorf("group matching: determine provider requirement: %w", needsErr)
		}
		runner = groupmatching.Run
	}
	var provider llm.Provider
	var err error
	if providerFactory != nil && (!defaultRunner || providerRequired) {
		provider, err = providerFactory()
		if err != nil {
			return nil, fmt.Errorf("group matching: configure provider: %w", err)
		}
	}
	if providerRequired && provider == nil {
		return nil, fmt.Errorf("group matching: configured model provider is unavailable")
	}
	writer, err := debugdump.OpenWriter(runs[0].RunDir, false)
	if err != nil {
		return nil, fmt.Errorf("group matching: open artifact writer: %w", err)
	}
	observer := debugdump.NewSemanticObserver(writer)
	executor := debugdump.BindStage(llm.Executor{
		RootDir: cacheRoot, Enabled: !noCache, Observer: observer,
		BatchConcurrency: batchConcurrency, BatchController: batchController,
	}, debugdump.SemanticStageGroupMatching)
	if output != nil {
		output.Stage("Group matching", "connecting completed target graphs")
	}
	started := time.Now()
	matched, diagnostics, runErr := runner(ctx, executor, provider, indexes)
	closeErr := writer.Close()
	reportSemanticOrdinalScaleWarnings(output, "Group matching", nil, observer.OrdinalScaleWarnings())
	if runErr != nil {
		return nil, errors.Join(fmt.Errorf("group matching: match target graphs: %w", runErr), closeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("group matching: close artifact writer: %w", closeErr)
	}
	if err := groupindex.ValidateSet(matched); err != nil {
		return nil, fmt.Errorf("group matching: result set: %w", err)
	}
	matchedByTarget := make(map[string]groupindex.Index, len(matched))
	for _, index := range matched {
		matchedByTarget[index.Target.ID] = index.Snapshot()
	}
	result := make([]targetPublishedRun, len(runs))
	connectionCount := 0
	for position, run := range runs {
		index, ok := matchedByTarget[run.GroupIndex.Target.ID]
		if !ok || index.ProgramIndexSHA256 != run.GroupIndex.ProgramIndexSHA256 {
			return nil, fmt.Errorf("group matching: result omitted or changed target %s", run.GroupIndex.Target.ID)
		}
		result[position] = run
		result[position].GroupIndex = index.Snapshot()
		connectionCount += len(index.Connections)
	}
	if len(matchedByTarget) != len(result) {
		return nil, fmt.Errorf("group matching: result target inventory changed")
	}
	for _, run := range result {
		if err := groupindex.Persist(run.RunDir, run.GroupIndex); err != nil {
			return nil, fmt.Errorf("group matching: persist run %s: %w", run.RunID, err)
		}
	}
	matchRows := groupDiagnosticRows(debugdump.SemanticStageGroupMatching, "", diagnostics)
	if err := modeldiag.Append(runs[0].RunDir, matchRows); err != nil && output != nil {
		output.Warn("could not record matching diagnostics", err.Error())
	}
	if output != nil {
		details := []string{
			fmt.Sprintf("target graphs: %d", len(result)),
			fmt.Sprintf("connections: %d", connectionCount),
			fmt.Sprintf("discarded response rows: %d", len(diagnostics)),
		}
		details = append(details, modeldiag.Summary(matchRows)...)
		details = append(details, formatRunOutputWallDuration(time.Since(started)))
		output.State("Group matching", "ready", details...)
	}
	return result, nil
}

func programIndexNeedsGroupingProvider(index programindex.Index) bool {
	return index.Categorization != nil && len(index.Categorization.Assignments) > 0
}

// groupDiagnosticRows folds per-proposal grouping and matching diagnostics
// into one counted row per reason, naming a few of the refused proposals.
func groupDiagnosticRows(
	stage string,
	target string,
	diagnostics []groupindex.Diagnostic,
) []modeldiag.Row {
	if len(diagnostics) == 0 {
		return nil
	}
	counts := map[string]int{}
	samples := map[string][]string{}
	order := []string{}
	for _, diagnostic := range diagnostics {
		kind := string(diagnostic.Kind)
		if _, seen := counts[kind]; !seen {
			order = append(order, kind)
		}
		counts[kind]++
		if diagnostic.ProposalKey != "" && len(samples[kind]) < modeldiag.MaxSamples {
			samples[kind] = append(samples[kind], diagnostic.ProposalKey)
		}
	}
	rows := make([]modeldiag.Row, 0, len(order))
	for _, kind := range order {
		rows = append(rows, modeldiag.Row{
			Stage: stage, Target: target, Kind: kind,
			Count: counts[kind], Samples: samples[kind],
		})
	}
	return rows
}
