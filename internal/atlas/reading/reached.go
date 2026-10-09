package reading

import (
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
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
// otherwise dropped. A declaration no exact call reaches is reached through
// the calls resolved to alternatives among which it stands (freqtrade's
// Webhook.send_msg, called only by RPCManager.send_msg's loop over its
// registered handlers). Callers in test files and callers none of the
// row's programs runs are skipped; a cycle stops where it closes, and there
// is no depth cap. A code fact: no model decides it. targets are the
// programs asked about, each destination being one program's
// (destinationMember).
func (r *reader) reachedFrom(targets []string, owner atlas.Place, handlers map[string]bool) []string {
	if owner.Symbol == nil {
		return nil
	}
	name := func(place atlas.Place) string { return place.Symbol.Decl.Name }
	var result []string
	seen := map[string]bool{owner.ID: true}
	stack := []atlas.Place{owner}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		callers := r.runningCallers(targets, current, "exact")
		if len(callers) == 0 {
			callers = r.runningCallers(targets, current, "alternatives")
		}
		for _, place := range callers {
			if place.Path != owner.Path {
				result = append(result, name(place))
				continue
			}
			if !seen[place.ID] {
				seen[place.ID] = true
				stack = append(stack, place)
			}
		}
		if len(callers) == 0 && current.ID != owner.ID && (len(current.Symbol.Seeds) > 0 || handlers[current.ID]) {
			result = append(result, name(current))
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

// runningCallers are the declarations calling current by calls of one
// resolution, outside tests, that one of targets runs.
func (r *reader) runningCallers(targets []string, current atlas.Place, resolution string) []atlas.Place {
	var result []atlas.Place
	for _, caller := range current.Symbol.CalledBy {
		if target, _, scoped := strings.Cut(caller.ObjectID, "."); scoped && programindex.ValidTargetID(target) && !slices.Contains(targets, target) {
			continue
		}
		if caller.Kind != "calls" || caller.Resolution != resolution || caller.PlaceID == "" || caller.PlaceID == current.ID || r.testPath(caller.Path) {
			continue
		}
		place := r.places[caller.PlaceID]
		if place.Symbol == nil || r.testFile(place.Parent) || len(intersectTargets(targets, runningTargets(place))) == 0 {
			continue
		}
		result = append(result, place)
	}
	return result
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
