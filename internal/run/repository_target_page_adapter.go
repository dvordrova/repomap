package run

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
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
// The shared parser group is loaded lazily inside BuildProgramInput, after
// orchestration enters the selected target's program analysis stage.
type pythonRepositoryProgramFacts struct {
	Catalog pythontarget.Catalog
	Target  pythontarget.Target
	Group   *pythonRepositoryParserGroup
}

type pythonRepositoryDispatchPlan struct {
	catalog pythontarget.Catalog
	groups  map[string]*pythonRepositoryParserGroup
}

type pythonRepositoryParserGroup struct {
	targets   []pythontarget.Target
	inputs    map[string]pythonprogramindex.InputResult
	attempted bool
	output    *runOutput
}

func (group *pythonRepositoryParserGroup) take(ctx context.Context, repository *corpus.Corpus, target pythontarget.Target) (programindex.Input, error) {
	if !group.attempted {
		group.attempted = true
		if group.output != nil {
			group.output.State("Python project", "parsing shared sources",
				"root: "+target.ProjectDir,
				fmt.Sprintf("targets: %d; source files: %d", len(group.targets), len(target.Modules)))
		}
		inputs, err := pythonprogramindex.BuildInputResults(ctx, repository, group.targets)
		if err != nil {
			if ctx.Err() != nil {
				return programindex.Input{}, err
			}
			// A failed shared parse/projection cannot refuse an independently
			// valid target. Keep the existing exact-target path for this group.
			if group.output != nil {
				group.output.State("Python project", "parsing targets separately", "root: "+target.ProjectDir, "shared preparation failed: "+err.Error())
			}
		} else {
			group.inputs = make(map[string]pythonprogramindex.InputResult, len(inputs))
			for i, input := range inputs {
				group.inputs[group.targets[i].Ref] = input
			}
		}
		group.targets = nil
	}
	if input, ok := group.inputs[target.Ref]; ok {
		delete(group.inputs, target.Ref)
		return input.Input, input.Err
	}
	return pythonprogramindex.BuildInput(ctx, repository, target)
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
	ordered []repositoryTypedTarget,
) (any, error) {
	catalog, ok := repositoryPlanPythonCatalog(plan)
	if !ok {
		return nil, fmt.Errorf("repository target dispatcher: Python plan catalog is missing")
	}
	checked, err := catalog.Check()
	if err != nil {
		return nil, err
	}
	state := &pythonRepositoryDispatchPlan{catalog: checked, groups: make(map[string]*pythonRepositoryParserGroup)}
	groups := make(map[[32]byte]*pythonRepositoryParserGroup)
	for _, selected := range ordered {
		target, ok := repositoryPythonTarget(selected)
		if !ok {
			continue
		}
		encoded, err := json.Marshal(struct {
			Root    string
			Modules []pythontarget.Module
		}{target.ProjectDir, target.Modules})
		if err != nil {
			return nil, err
		}
		key := sha256.Sum256(encoded)
		group := groups[key]
		if group == nil {
			group = &pythonRepositoryParserGroup{}
			groups[key] = group
		}
		group.targets = append(group.targets, target)
		state.groups[target.Ref] = group
	}
	return state, nil
}

func preparePythonRepositoryDispatchTarget(
	_ context.Context,
	options repositoryTargetDispatchOptions,
	target repositoryTypedTarget,
	planState any,
) (repositoryTargetDispatchBinding, error) {
	state, ok := planState.(*pythonRepositoryDispatchPlan)
	if !ok {
		return repositoryTargetDispatchBinding{}, fmt.Errorf("repository target dispatcher: invalid Python plan state")
	}
	selected, ok := repositoryPythonTarget(target)
	if !ok || !state.catalog.OwnsTarget(selected) {
		return repositoryTargetDispatchBinding{}, fmt.Errorf(
			"repository target dispatcher: Python target is outside its exact catalog authority",
		)
	}
	group := state.groups[selected.Ref]
	if group == nil {
		return repositoryTargetDispatchBinding{}, fmt.Errorf("repository target dispatcher: Python target has no parser group")
	}
	group.output = options.Output
	return repositoryTargetDispatchBinding{
		Target: target,
		ProgramFacts: pythonRepositoryProgramFacts{
			Catalog: state.catalog, Target: selected, Group: group,
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
	var seeds []pythontarget.Target
	for _, seed := range request.Target.Seeds {
		native, ok := repositoryPythonTarget(seed)
		if !ok || !facts.Catalog.OwnsTarget(native) {
			return programindex.Input{}, fmt.Errorf("seed outside Python target catalogue")
		}
		seeds = append(seeds, native)
	}
	var input programindex.Input
	var err error
	if facts.Group != nil {
		input, err = facts.Group.take(request.Context, request.Corpus, facts.Target)
	} else {
		input, err = pythonprogramindex.BuildInput(request.Context, request.Corpus, facts.Target)
	}
	if err != nil {
		return programindex.Input{}, fmt.Errorf("isolated parser: %w", err)
	}
	return pythonprogramindex.WithSeeds(request.Corpus, input, facts.Target, seeds...)
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
