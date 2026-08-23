package cubemap

import "github.com/dvordrova/repomap/internal/llm"

// Stage is the closed semantic owner of one cubemap model call.
type Stage string

const (
	StageActivitySurfaces        Stage = "cubemap_activity_surfaces"
	StageEntrypoints             Stage = "cubemap_entrypoints"
	StageIntegrationDependencies Stage = "cubemap_integration_dependencies"
	StageIntegrationSymbols      Stage = "cubemap_integration_symbols"
	StageSurfaceCoreEffects      Stage = "cubemap_surface_core_effects"
)

// StageObserver lets an ordinary-run observer retain the domain owner without
// adding domain fields to the provider-neutral llm.Event contract.
type StageObserver interface {
	ObserveCubemap(Stage, llm.Event) error
}

func executorForStage(executor llm.Executor, stage Stage) llm.Executor {
	observer, ok := executor.Observer.(StageObserver)
	if !ok {
		return executor
	}
	executor.Observer = llm.ObserverFunc(func(event llm.Event) error {
		return observer.ObserveCubemap(stage, event)
	})
	return executor
}
