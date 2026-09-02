package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// goRepositoryProgramFacts is the Go adapter's one immutable handoff. The
// packages/types/SSA lifetime has ended before this value crosses the shared
// ProgramIndex seam, and every retained producer index is independently owned.
type goRepositoryProgramFacts struct {
	Target            analysistarget.Target
	EffectiveGoTarget string
	Direct            surfacediscovery.DirectCallIndex
	External          surfacediscovery.ExternalCallIndex
	Core              gocoreobject.Index
	Dynamic           godynamichandoff.Index
	PackageOrigins    []gofacts.PackageOrigin
	Dependencies      dependencies.Catalog
}

func prepareGoRepositoryProgramFacts(
	ctx context.Context,
	options repositoryTargetDispatchOptions,
	target repositoryTypedTarget,
	state *goRepositoryDispatchPlan,
) (goRepositoryProgramFacts, error) {
	if state == nil {
		return goRepositoryProgramFacts{}, fmt.Errorf("repository target dispatcher: invalid Go plan state")
	}
	if err := state.errors[target.Key]; err != nil {
		return goRepositoryProgramFacts{}, err
	}
	scoped, ok := state.snapshots[target.Key]
	if !ok || scoped.AnalysisTarget == nil || scoped.GoFacts == nil {
		return goRepositoryProgramFacts{}, fmt.Errorf("repository target dispatcher: Go target snapshot is missing")
	}
	selected, selectedOK := repositoryGoTarget(target)
	if !selectedOK || selected.Ref != scoped.AnalysisTarget.Ref {
		return goRepositoryProgramFacts{}, fmt.Errorf("repository target dispatcher: Go target snapshot identity changed")
	}

	goTarget, err := effectiveScopedGoTarget(options.GoTarget, scoped)
	if err != nil {
		return goRepositoryProgramFacts{}, err
	}
	surfaceOptions := surfacediscovery.DefaultOptions(options.Repo, goTarget)
	surfaceOptions.BuildTags = append([]string(nil), options.GoBuildTags...)
	if options.DirectCallDepth > 0 {
		surfaceOptions.DirectCallDepth = options.DirectCallDepth
	}
	if options.DirectCallEdgeLimit > 0 {
		surfaceOptions.DirectCallEdgeLimit = options.DirectCallEdgeLimit
	}
	surfaceOptions.CaptureExternalCallIndex = true
	surfaceOptions.CaptureCoreObjectIndex = true
	surfaceOptions.CaptureDynamicHandoffIndex = true

	result, err := state.workspace.analyze(
		ctx, surfaceOptions, scoped, state.all,
	)
	targetLabel := repositoryTypedTargetDisplay(target) + " (" + target.Key.String() + ")"
	if options.Output != nil {
		coverage := result.Coverage
		if err != nil {
			var unavailable *surfacediscovery.AnalysisTargetSSAUnavailableError
			if errors.As(err, &unavailable) {
				if failedCoverage, ok := unavailable.ProgramCoverageSnapshot(); ok {
					coverage = failedCoverage
				}
			}
		}
		reportPackageDiagnosticScaleWarnings(options.Output, targetLabel, coverage)
	}
	if err != nil {
		return goRepositoryProgramFacts{}, err
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil ||
		result.CoreObjectIndex == nil || result.DynamicHandoffIndex == nil {
		return goRepositoryProgramFacts{}, fmt.Errorf("Go adapter returned incomplete exact producer facts")
	}
	if err := result.DirectCallIndex.Validate(); err != nil {
		return goRepositoryProgramFacts{}, fmt.Errorf(
			"validate Go call analysis for target %s: %w", selected.DisplayPath(), err,
		)
	}
	if !selected.MatchesDirectCallIndexScope(
		result.DirectCallIndex.Scope,
		options.DirectCallDepth,
		options.DirectCallEdgeLimit,
	) {
		return goRepositoryProgramFacts{}, fmt.Errorf(
			"Go call analysis scope does not match target %s and requested --depth/--edges-limit",
			analysisTargetSubject(selected),
		)
	}
	if result.DirectCallIndex.State != surfacediscovery.DirectCallIndexReady {
		switch result.DirectCallIndex.ClosedReason {
		case surfacediscovery.DirectCallIndexClosedSSAUnavailable:
			return goRepositoryProgramFacts{}, fmt.Errorf(
				"Go SSA is unavailable for target %s under %s; choose the correct platform with --force-platform GOOS/GOARCH",
				selected.DisplayPath(), goTarget,
			)
		case surfacediscovery.DirectCallIndexClosedEdgeLimit:
			return goRepositoryProgramFacts{}, directCallEdgeCeilingError(
				analysisTargetSubject(selected),
				options.DirectCallDepth,
				options.DirectCallEdgeLimit,
				result.DirectCallIndex.Coverage.EdgeLimitSafeDepth,
			)
		default:
			return goRepositoryProgramFacts{}, fmt.Errorf(
				"Go call analysis for target %s is unavailable", selected.DisplayPath(),
			)
		}
	}
	if options.Output != nil {
		reportDirectCallIndexScaleWarnings(options.Output, targetLabel, *result.DirectCallIndex)
	}
	if scoped.GoFacts.Dependencies == nil {
		return goRepositoryProgramFacts{}, fmt.Errorf("Go adapter requires target-scoped dependency authority")
	}
	ownedDependencies, err := ownRepositoryDependencies(*scoped.GoFacts.Dependencies)
	if err != nil {
		return goRepositoryProgramFacts{}, fmt.Errorf("own Go dependency authority: %w", err)
	}
	if options.Output != nil {
		reportDependencyCatalogScaleWarningsForTarget(options.Output, targetLabel, ownedDependencies)
	}
	return goRepositoryProgramFacts{
		Target:            scoped.AnalysisTarget.Snapshot(),
		EffectiveGoTarget: goTarget,
		Direct:            result.DirectCallIndex.Snapshot(),
		External:          result.ExternalCallIndex.Snapshot(),
		Core:              result.CoreObjectIndex.Snapshot(),
		Dynamic:           result.DynamicHandoffIndex.Snapshot(),
		PackageOrigins:    append([]gofacts.PackageOrigin(nil), scoped.GoFacts.PackageOrigins...),
		Dependencies:      ownedDependencies,
	}, nil
}

func effectiveScopedGoTarget(baseline string, scoped snapshot.Snapshot) (string, error) {
	baseline = strings.TrimSpace(baseline)
	if baseline == "" {
		return "", fmt.Errorf("repository target dispatcher: resolved Go target is missing")
	}
	if scoped.GoTargetSelection == nil {
		return baseline, nil
	}
	selection := *scoped.GoTargetSelection
	if err := selection.ValidateAgainstAdvisory(scoped.GoTargetAdvisory); err != nil {
		return "", fmt.Errorf("repository target dispatcher: automatic Go target selection: %w", err)
	}
	if selection.Baseline != baseline {
		return "", fmt.Errorf(
			"repository target dispatcher: automatic Go target baseline %q does not match resolved target %q",
			selection.Baseline, baseline,
		)
	}
	return selection.Target, nil
}

func (state *repositoryGoWorkspaceState) analyze(
	ctx context.Context,
	options surfacediscovery.Options,
	scoped snapshot.Snapshot,
	selected []snapshot.Snapshot,
) (surfacediscovery.Result, error) {
	current, err := goSurfaceDiscoveryInput(scoped)
	if err != nil {
		return surfacediscovery.Result{}, err
	}
	workspace := state.workspace
	if workspace == nil {
		inputs := []surfacediscovery.Input{current}
		useUnion := !state.unionUnavailable && len(selected) > 1
		if useUnion {
			inputs = make([]surfacediscovery.Input, 0, len(selected))
			seen := make(map[string]struct{}, len(selected))
			currentIncluded := false
			for index := range selected {
				owned, ownErr := snapshot.OwnSnapshot(selected[index])
				if ownErr != nil {
					return surfacediscovery.Result{}, fmt.Errorf(
						"prepare selected Go target workspace: own projection %d: %w", index, ownErr,
					)
				}
				input, inputErr := goSurfaceDiscoveryInput(owned)
				if inputErr != nil {
					return surfacediscovery.Result{}, fmt.Errorf(
						"prepare selected Go target workspace: projection %d: %w", index, inputErr,
					)
				}
				ref := input.AnalysisTarget.TargetRef
				if _, duplicate := seen[ref]; duplicate {
					return surfacediscovery.Result{}, fmt.Errorf(
						"prepare selected Go target workspace: duplicate target projection %q", ref,
					)
				}
				seen[ref] = struct{}{}
				currentIncluded = currentIncluded || ref == current.AnalysisTarget.TargetRef
				inputs = append(inputs, input)
			}
			if !currentIncluded {
				return surfacediscovery.Result{}, fmt.Errorf(
					"prepare selected Go target workspace: current target projection is absent",
				)
			}
		}

		workspace, err = surfacediscovery.PrepareWorkspace(ctx, options, inputs)
		if err != nil && useUnion && !errors.Is(err, context.Canceled) &&
			!errors.Is(err, context.DeadlineExceeded) {
			state.workspace = nil
			state.unionUnavailable = true
			workspace, err = surfacediscovery.PrepareWorkspace(
				ctx, options, []surfacediscovery.Input{current},
			)
			useUnion = false
		}
		if err != nil {
			return surfacediscovery.Result{}, err
		}
		if useUnion || len(selected) <= 1 {
			state.workspace = workspace
		}
	}
	return workspace.Analyze(ctx, options, current)
}

func goSurfaceDiscoveryInput(scoped snapshot.Snapshot) (surfacediscovery.Input, error) {
	if scoped.AnalysisTarget == nil || scoped.GoFacts == nil || scoped.TargetCatalog != nil {
		return surfacediscovery.Input{}, fmt.Errorf("projection is not an exact scoped Go target")
	}
	return goadapter.AnalysisInput(scoped.GoFacts, scoped.AnalysisTarget)
}

func buildGoRepositoryProgramInput(
	request repositoryProgramBuildRequest,
) (programindex.Input, error) {
	facts, ok := request.Facts.(goRepositoryProgramFacts)
	selected, selectedOK := repositoryGoTarget(request.Target)
	if !ok || !selectedOK || facts.Target.Ref != selected.Ref {
		return programindex.Input{}, fmt.Errorf("invalid Go compiler fact snapshot")
	}
	return goadapter.BuildInput(
		request.Corpus,
		facts.Target,
		facts.PackageOrigins,
		facts.Direct,
		facts.External,
		facts.Core,
		facts.Dynamic,
	)
}

func buildGoRepositoryDependencies(
	request repositoryDependencyBuildRequest,
) (dependencies.Catalog, error) {
	facts, ok := request.Facts.(goRepositoryProgramFacts)
	selected, selectedOK := repositoryGoTarget(request.Target)
	if !ok || !selectedOK || facts.Target.Ref != selected.Ref {
		return dependencies.Catalog{}, fmt.Errorf("invalid Go compiler fact snapshot")
	}
	return ownRepositoryDependencies(facts.Dependencies)
}

func ownRepositoryDependencies(source dependencies.Catalog) (dependencies.Catalog, error) {
	return dependencies.BuildWithOmissions(
		source.Importers,
		source.Dependencies,
		source.Coverage.Omissions,
	)
}
