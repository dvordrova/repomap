package groupindex

import "github.com/dvordrova/repomap/internal/programindex"

// Phases of a program: initialization is what the target's seeds reach by
// ordinary calls — the wiring before anything serves; runtime is what an
// operation reaches on its chains. A subject or a connection has one phase,
// or both when the same declaration serves in each. Derived from the
// program index, the operations and the chains; never persisted.
const (
	PhaseInit    = "init"
	PhaseRuntime = "runtime"
	PhaseBoth    = "both"
)

func applyPhases(index *Index, program programindex.Index) {
	callees := make(map[string][]string)
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Resolution == programindex.ResolutionUnresolved {
			continue
		}
		callees[relation.FromID] = append(callees[relation.FromID], relation.ToIDs...)
	}
	init := make(map[string]bool)
	var queue []string
	for _, seed := range program.Target.Seeds {
		if !init[seed.ObjectID] {
			init[seed.ObjectID] = true
			queue = append(queue, seed.ObjectID)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range callees[current] {
			if !init[next] {
				init[next] = true
				queue = append(queue, next)
			}
		}
	}
	runtime := make(map[string]bool)
	for _, operation := range index.Operations {
		if operation.SubjectID != "" {
			runtime[operation.SubjectID] = true
		}
	}
	for _, chain := range index.Chains {
		for _, id := range chain.SubjectIDs {
			runtime[id] = true
		}
	}
	phase := func(id string) string {
		switch {
		case init[id] && runtime[id]:
			return PhaseBoth
		case init[id]:
			return PhaseInit
		case runtime[id]:
			return PhaseRuntime
		}
		return ""
	}
	for position := range index.Subjects {
		index.Subjects[position].Phase = phase(index.Subjects[position].ID)
	}
	for position := range index.Connections {
		index.Connections[position].Phase = phase(index.Connections[position].FromSubjectID)
	}
}
