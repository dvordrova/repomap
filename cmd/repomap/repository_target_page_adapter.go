package main

import (
	"context"
	"fmt"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythondependencies"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

type repositoryTargetDispatchBinding struct {
	Target            repositoryTypedTarget
	ProgramFacts      any
	ProgramFactsBound bool
}

// pythonRepositoryProgramFacts is the one immutable Python adapter handoff.
// It retains only the exact catalog and selected target. The isolated parser
// runs later inside BuildProgramInput, after orchestration enters the program
// analysis stage.
type pythonRepositoryProgramFacts struct {
	Catalog pythontarget.Catalog
	Target  pythontarget.Target
}

type goRepositoryDispatchPlan struct {
	snapshots map[repositoryTargetKey]snapshot.Snapshot
	errors    map[repositoryTargetKey]error
	all       []snapshot.Snapshot
	workspace repositoryGoWorkspaceState
}

func prepareGoRepositoryDispatchPlan(
	plan repositoryTargetPlan,
	ordered []repositoryTypedTarget,
) (any, error) {
	state := &goRepositoryDispatchPlan{
		snapshots: make(map[repositoryTargetKey]snapshot.Snapshot),
		errors:    make(map[repositoryTargetKey]error),
	}
	source, ok := repositoryPlanGoSource(plan)
	if !ok {
		return nil, fmt.Errorf("repository target dispatcher: Go plan source is missing")
	}
	for _, target := range ordered {
		if target.Key.Adapter != repositoryTargetAdapterGo {
			continue
		}
		scoped, err := snapshot.ScopeAnalysisTarget(*source, target.Key.Ref)
		if err != nil {
			state.errors[target.Key] = fmt.Errorf(
				"repository target dispatcher: scope Go target %s: %w", target.Key.String(), err,
			)
			continue
		}
		state.snapshots[target.Key] = scoped
		state.all = append(state.all, scoped)
	}
	return state, nil
}

func prepareGoRepositoryDispatchTarget(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	target repositoryTypedTarget,
	planState any,
) (repositoryTargetDispatchBinding, error) {
	state, ok := planState.(*goRepositoryDispatchPlan)
	if !ok {
		return repositoryTargetDispatchBinding{}, fmt.Errorf("repository target dispatcher: invalid Go plan state")
	}
	facts, err := prepareGoRepositoryProgramFacts(ctx, options, target, state)
	if err != nil {
		return repositoryTargetDispatchBinding{}, err
	}
	return repositoryTargetDispatchBinding{
		Target: target, ProgramFacts: facts, ProgramFactsBound: true,
	}, nil
}

func preparePythonRepositoryDispatchPlan(
	plan repositoryTargetPlan,
	_ []repositoryTypedTarget,
) (any, error) {
	catalog, ok := repositoryPlanPythonCatalog(plan)
	if !ok {
		return nil, fmt.Errorf("repository target dispatcher: Python plan catalog is missing")
	}
	return catalog.Snapshot(), nil
}

func preparePythonRepositoryDispatchTarget(
	_ context.Context,
	_ repositoryTargetDispatchOptions,
	target repositoryTypedTarget,
	planState any,
) (repositoryTargetDispatchBinding, error) {
	catalog, ok := planState.(pythontarget.Catalog)
	if !ok {
		return repositoryTargetDispatchBinding{}, fmt.Errorf("repository target dispatcher: invalid Python plan state")
	}
	selected, ok := repositoryPythonTarget(target)
	if !ok || !catalog.OwnsTarget(selected) {
		return repositoryTargetDispatchBinding{}, fmt.Errorf(
			"repository target dispatcher: Python target is outside its exact catalog authority",
		)
	}
	ownedCatalog := catalog.Snapshot()
	ownedTarget := selected.Snapshot()
	return repositoryTargetDispatchBinding{
		Target: target,
		ProgramFacts: pythonRepositoryProgramFacts{
			Catalog: ownedCatalog, Target: ownedTarget,
		},
		ProgramFactsBound: true,
	}, nil
}

func buildPythonRepositoryProgramInput(
	request repositoryProgramBuildRequest,
) (programindex.Input, error) {
	facts, ok := request.Facts.(pythonRepositoryProgramFacts)
	selected, selectedOK := repositoryPythonTarget(request.Target)
	if !ok || !selectedOK || facts.Target.Ref != selected.Ref ||
		facts.Target.Selector != selected.Selector || !facts.Catalog.OwnsTarget(facts.Target) {
		return programindex.Input{}, fmt.Errorf("invalid Python parser fact snapshot")
	}
	input, err := pythonprogramindex.BuildInput(request.Context, request.Corpus, facts.Target)
	if err != nil {
		return programindex.Input{}, fmt.Errorf("isolated parser: %w", err)
	}
	return input, nil
}

func buildPythonRepositoryDependencies(
	request repositoryDependencyBuildRequest,
) (dependencies.Catalog, error) {
	facts, ok := request.Facts.(pythonRepositoryProgramFacts)
	selected, selectedOK := repositoryPythonTarget(request.Target)
	if !ok || !selectedOK || facts.Target.Ref != selected.Ref ||
		facts.Target.Selector != selected.Selector || !facts.Catalog.OwnsTarget(facts.Target) {
		return dependencies.Catalog{}, fmt.Errorf("invalid Python parser fact snapshot")
	}
	catalog, err := pythondependencies.Build(request.ProgramIndex)
	if err != nil {
		return dependencies.Catalog{}, err
	}
	if err := pythonDependencyCoverageError(catalog); err != nil {
		return dependencies.Catalog{}, err
	}
	return catalog, nil
}

func prepareJSTSRepositoryDispatchPlan(
	_ repositoryTargetPlan,
	_ []repositoryTypedTarget,
) (any, error) {
	return struct{}{}, nil
}

func prepareJSTSRepositoryDispatchTarget(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	target repositoryTypedTarget,
	_ any,
) (repositoryTargetDispatchBinding, error) {
	materialized, err := materializeSelectedJSTSProjects(
		ctx, options, []repositoryTypedTarget{target},
	)
	if err != nil {
		return repositoryTargetDispatchBinding{}, err
	}
	project, ok := materialized[target.Key]
	if !ok {
		return repositoryTargetDispatchBinding{}, fmt.Errorf(
			"repository target dispatcher: materialized JavaScript/TypeScript target is missing",
		)
	}
	rebound, err := rebindMaterializedJSTSTarget(target, project)
	if err != nil {
		return repositoryTargetDispatchBinding{}, err
	}
	return repositoryTargetDispatchBinding{
		Target: rebound, ProgramFacts: project.Snapshot(), ProgramFactsBound: true,
	}, nil
}
