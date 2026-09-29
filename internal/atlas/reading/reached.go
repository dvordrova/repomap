package reading

import (
	"slices"

	"github.com/dvordrova/repomap/internal/atlas"
)

// reachedFrom are the declarations an outgoing call is reached from in the
// programs its row runs in, each by name, for the question naming the
// call's destination (destination_groups.go): Redis's connect is written
// in anet.c's anetTcpGenericConnect, and redis-server reaches it from syncWithMaster,
// redis-cli from cliConnect, which says what is at the other end. It is
// GroupsIndex's ReachedFrom (outbound_reached.go) before the parts are
// drawn, when a file stands for its part: from the declaration making the
// call, exact calls are followed backwards only through callers in its own
// file; each path ends at the first caller in another file, or, where the
// callers run out inside the file, at a seed or an input's handler, and is
// otherwise dropped. Callers in test files and callers none of the row's
// programs runs are skipped; a cycle stops where it closes, and there is no
// depth cap. A code fact: no model decides it.
func (r *reader) reachedFrom(state *boundaryState, owner atlas.Place, handlers map[string]bool) []string {
	if owner.Symbol == nil {
		return nil
	}
	targets := rowTargets(state)
	name := func(place atlas.Place) string { return place.Symbol.Decl.Name }
	var result []string
	seen := map[string]bool{owner.ID: true}
	stack := []atlas.Place{owner}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		callers := 0
		for _, caller := range current.Symbol.CalledBy {
			if caller.Kind != "calls" || caller.Resolution != "exact" || caller.PlaceID == "" || caller.PlaceID == current.ID || r.testPath(caller.Path) {
				continue
			}
			place := r.places[caller.PlaceID]
			if place.Symbol == nil || r.testFile(place.Parent) || len(intersectTargets(targets, runningTargets(place))) == 0 {
				continue
			}
			callers++
			if place.Path != owner.Path {
				result = append(result, name(place))
				continue
			}
			if !seen[place.ID] {
				seen[place.ID] = true
				stack = append(stack, place)
			}
		}
		if callers == 0 && current.ID != owner.ID && (len(current.Symbol.Seeds) > 0 || handlers[current.ID]) {
			result = append(result, name(current))
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

// entryHandlers are the declarations handling the program's entries, by
// place: the callables the incoming boundaries hand over.
func (r *reader) entryHandlers() map[string]bool {
	handlers := map[string]bool{}
	for _, state := range r.boundaries {
		if b := state.place.Boundary; b != nil && b.Direction == atlas.DirectionIn && b.SubjectID != "" && !state.handlerUnknown {
			handlers[b.SubjectID] = true
		}
	}
	return handlers
}
