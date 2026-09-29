package orientation

import (
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// flowScope is the code the Main flow request shows of one program: every
// callable the program runs from where it starts (groupindex Launch), every
// callable an input's handler reaches (groupindex Reach), and every
// repository callable one of those hands over as a callback or registers,
// with what that one runs in turn. Reach never follows a hand-over (Worker
// hands self._process_running to _throttle, which calls func(*args)); only
// this evidence scope does, and GroupsIndex keeps its own rule.
//
// Members are in reading order: the walk from the seeds, breadth first; then
// the rest of the launch walk, from the load-time roots; then each input's
// reach in operation order. A handed-over callable, and what it runs, follow
// the callable that hands it over. HandedOver counts the members only a
// hand-over brings in.
type flowScope struct {
	Members    []string
	HandedOver int
}

func scopeOf(index groupindex.Index, registrations []facts.Fact) flowScope {
	position := make(map[string]int, len(index.Subjects))
	executing := make([]bool, len(index.Subjects))
	for at, subject := range index.Subjects {
		position[subject.ID] = at
		if subject.Object != nil {
			switch subject.Object.Kind {
			case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
				executing[at] = true
			}
		}
	}
	handler := make(map[string]bool)
	for _, operation := range index.Operations {
		if operation.SubjectID != "" {
			handler[operation.SubjectID] = true
		}
	}
	calls := make(map[string][]string)
	hands := make(map[string][]string)
	for _, edge := range index.StructuralEdges {
		from, fromKnown := position[edge.FromSubjectID]
		to, toKnown := position[edge.ToSubjectID]
		if !fromKnown || !toKnown || edge.Role != groupindex.EdgeRelationTarget ||
			edge.Resolution == programindex.ResolutionUnresolved || !executing[from] || !executing[to] {
			continue
		}
		switch edge.RelationKind {
		case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal:
			// The launch and reach rule: a call resolved as alternatives
			// does not run another input's handler from here.
			if edge.Resolution == programindex.ResolutionAlternatives && handler[edge.ToSubjectID] {
				continue
			}
			calls[edge.FromSubjectID] = append(calls[edge.FromSubjectID], edge.ToSubjectID)
		case programindex.RelationPassesCallback:
			hands[edge.FromSubjectID] = append(hands[edge.FromSubjectID], edge.ToSubjectID)
		}
	}
	for _, fact := range registrations {
		if fact.Kind != facts.KindRegistration || fact.TargetID != index.Target.ID || fact.OwnerID == "" || fact.ObjectID == "" {
			continue
		}
		from, fromKnown := position[fact.OwnerID]
		to, toKnown := position[fact.ObjectID]
		if fromKnown && toKnown && executing[from] && executing[to] {
			hands[fact.OwnerID] = append(hands[fact.OwnerID], fact.ObjectID)
		}
	}
	callable := func(id string) bool {
		at, known := position[id]
		return known && executing[at]
	}

	var base []string
	inBase := make(map[string]bool)
	add := func(id string) {
		if callable(id) && !inBase[id] {
			inBase[id] = true
			base = append(base, id)
		}
	}
	// The seeds' own walk first: Launch starts its walk from the seeds and
	// the load-time roots together, and a Python program's hundreds of
	// module bodies would otherwise come before main's callees.
	var walk []string
	walked := make(map[string]bool)
	for _, seed := range index.Target.Seeds {
		if callable(seed.ObjectID) && !walked[seed.ObjectID] {
			walked[seed.ObjectID] = true
			walk = append(walk, seed.ObjectID)
		}
	}
	for next := 0; next < len(walk); next++ {
		for _, to := range calls[walk[next]] {
			if !walked[to] {
				walked[to] = true
				walk = append(walk, to)
			}
		}
	}
	for _, id := range walk {
		add(id)
	}
	for _, function := range index.Launch.Functions {
		add(function.SubjectID)
	}
	for _, reach := range index.Reach {
		for _, reached := range reach.Subjects {
			add(reached.SubjectID)
		}
	}

	scope := flowScope{Members: make([]string, 0, len(base))}
	placed := make(map[string]bool, len(base))
	for _, id := range base {
		placed[id] = true
		scope.Members = append(scope.Members, id)
		// What this member hands over, and what that runs and hands over in
		// turn, breadth first, when nothing earlier holds it.
		var queue []string
		for _, to := range hands[id] {
			if !inBase[to] && !placed[to] {
				placed[to] = true
				queue = append(queue, to)
			}
		}
		for next := 0; next < len(queue); next++ {
			current := queue[next]
			scope.Members = append(scope.Members, current)
			scope.HandedOver++
			for _, to := range append(append([]string(nil), hands[current]...), calls[current]...) {
				if !inBase[to] && !placed[to] {
					placed[to] = true
					queue = append(queue, to)
				}
			}
		}
	}
	return scope
}
