package surfacediscovery

import (
	"github.com/dvordrova/repomap/internal/semantics/catalog"
	"golang.org/x/tools/go/ssa"
)

func (a *analyzer) recordAsyncTask(
	seed catalog.Seed,
	values map[string]Value,
	location Location,
	chain []Wrapper,
	entrypoint *ssa.Function,
	ambiguous bool,
) {
	callback := values["callback"]
	dispatcher := values["dispatcher"]
	frontiers := []Frontier{}
	if !callback.Known {
		frontiers = append(frontiers, Frontier{
			Kind: "dynamic_callback_identity", Detail: callback.Text, Location: &location,
		})
	}

	kind := "async_task"
	status := "confirmed_async_task_start"
	evidence := []Evidence{{
		ID: "async-start:" + locationKey(location), Kind: "async_task_start",
		Location: location, Detail: seed.ID,
	}}
	if callbackFunction := a.functionByID[cleanFunctionID(callback.Text)]; callbackFunction != nil {
		loops := a.loops(callbackFunction)
		if len(loops) > 0 {
			kind = "worker"
			status = "possible_worker_loop"
			for _, loop := range loops {
				signal := loop.signal
				signal.TerminalSeed = seed.ID
				a.addLoopSignal(signal)
				evidence = append(evidence, Evidence{
					ID: "loop:" + locationKey(signal.Location), Kind: signal.Kind,
					Location: signal.Location, Detail: signal.Detail,
				})
				if signal.Kind == "channel_receive_loop" || signal.Kind == "select_event_loop" {
					status = "confirmed_worker_registration"
				}
			}
		}
	}

	basis := string(catalog.OriginCatalogStatic)
	if len(chain) > 0 {
		basis = string(catalog.OriginWrapperStatic)
	}
	resolution := "exact"
	if ambiguous {
		resolution = "ambiguous"
	}
	if !callback.Known {
		resolution = "dynamic"
	}
	record := TriggerRecord{
		Kind: kind,
		Identity: Identity{
			Name: callback.Text,
			Path: Value{Kind: "not_applicable", Known: true, Candidates: []string{}},
		},
		Transport:         seed.Effect.Transport,
		Framework:         seed.Effect.Framework,
		ProcessEntrypoint: a.symbol(entrypoint),
		Dispatcher:        dispatcher,
		RegistrationSite:  location,
		Handler:           callback,
		Middleware:        []Value{},
		WrapperChain:      append([]Wrapper{}, chain...),
		FinalSeed:         seed.ID,
		DiscoveryBasis:    basis,
		Certainty:         "static",
		Resolution:        resolution,
		ScenarioID:        a.scenario.ID,
		Evidence:          evidence,
		Provenance: []Provenance{{
			Provider: "go_ssa", Version: AnalyzerVersion,
			Operation: "classify_async_callback", Detail: seed.ID,
		}},
		DynamicFrontier: frontiers,
		Status:          status,
	}
	record.ProvisionalID = !callback.Known || !dispatcher.Known
	record.ID = stableTriggerID(record)
	a.result.Catalog.Triggers = append(a.result.Catalog.Triggers, record)
	a.result.Coverage.DynamicFrontiers = append(a.result.Coverage.DynamicFrontiers, frontiers...)
}
