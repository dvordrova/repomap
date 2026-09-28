package groupindex

import (
	"slices"
)

// OuterInput is one way a request dispatched at a site arrives there
// (u6): an input whose reach holds the site's declaration and that is not
// itself dispatched there, or an input whose reach hands over a callable
// that is no input's handler (Registered, through the HandOver edge) and
// whose own walk, by the reach rule, holds the declaration. Registering are
// the input's calls on a route to the code handing the callable over;
// Edges the calls on a route to the declaration, from the input's handler
// or from the handed callable. Every such input is listed; none is chosen.
// A hand-over into an input's handler is that input's own reach, listed
// through it, never through the hop.
type OuterInput struct {
	OperationID string
	HandOver    int
	Registered  string
	Registering []int
	Edges       []int
}

// outerInputs fills each dispatch site's Outer and Unexplained. The
// program's start is no input: a call into the site from code only the
// launch reaches (Redis's AOF replay at start) leaves other ways not
// established, never read as the input's route.
func (graph *reachGraph) outerInputs(sites []DispatchSite, reaches []Reach) {
	index := graph.index
	walks := map[int]Reach{}
	walk := func(position int) Reach {
		if reach, done := walks[position]; done {
			return reach
		}
		reach := graph.reach(-1, Operation{SubjectID: index.Subjects[position].ID})
		walks[position] = reach
		return reach
	}
	// What each input hands over that no input handles.
	type handed struct {
		edge, callable int
	}
	handsOf := make([][]handed, len(reaches))
	for position, reach := range reaches {
		for _, subject := range reach.Subjects {
			for _, edge := range graph.hands[graph.position[subject.SubjectID]] {
				to := graph.to[edge]
				if len(graph.handlers[to]) > 0 || !graph.executing[to] {
					continue
				}
				handsOf[position] = append(handsOf[position], handed{edge: edge, callable: to})
			}
		}
	}
	into := map[int][]int{}
	for from := range graph.exec {
		for _, edge := range graph.exec[from] {
			into[graph.to[edge]] = append(into[graph.to[edge]], edge)
		}
	}
	held := map[string]bool{}
	for _, reach := range reaches {
		for _, subject := range reach.Subjects {
			held[subject.SubjectID] = true
		}
	}
	for position := range reaches {
		for _, hand := range handsOf[position] {
			for _, subject := range walk(hand.callable).Subjects {
				held[subject.SubjectID] = true
			}
		}
	}
	for position := range sites {
		site := &sites[position]
		if len(site.OperationIDs) == 0 {
			continue
		}
		declaration := graph.position[site.FromSubjectID]
		for _, reached := range site.ReachedFrom {
			if !slices.Contains(site.OperationIDs, reached.OperationID) {
				site.Outer = append(site.Outer, OuterInput{OperationID: reached.OperationID, HandOver: -1, Edges: reached.Edges})
			}
		}
		for operation, reach := range reaches {
			if slices.Contains(site.OperationIDs, reach.OperationID) {
				continue
			}
			for _, hand := range handsOf[operation] {
				edges, reaches := graph.routesTo(walk(hand.callable), declaration)
				if !reaches {
					continue
				}
				registering, _ := graph.routesTo(reach, graph.from[hand.edge])
				site.Outer = append(site.Outer, OuterInput{OperationID: reach.OperationID, HandOver: hand.edge,
					Registered: index.Subjects[hand.callable].ID, Registering: registering, Edges: edges})
			}
		}
		for _, edge := range into[declaration] {
			if !held[index.Subjects[graph.from[edge]].ID] {
				site.Unexplained = true
				break
			}
		}
	}
}
