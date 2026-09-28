package reading

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// valueElement is the element of another value a call compares: the value
// by where it was made, and the element by its literal index.
type valueElement struct {
	base  sourcevalue.Anchor
	index int
}

// comparedElement is the first argument of a call that is a literal element
// of a value made at a known site (argv[1] of what sdssplitlen returned),
// or nil.
func comparedElement(arguments []atlas.SourceArgument) *valueElement {
	for _, argument := range arguments {
		origin := argument.Origin
		if origin == nil || origin.Kind != "index" || len(origin.Parts) != 2 || origin.Parts[0].Anchor == nil || origin.Parts[1].Kind != "literal" {
			continue
		}
		index, err := strconv.Atoi(origin.Parts[1].Text)
		if err != nil || index < 0 {
			continue
		}
		return &valueElement{base: *origin.Parts[0].Anchor, index: index}
	}
	return nil
}

// foldValues makes the values of an entry its sub-arguments (owner's rule
// K3: the words compared inside an input's handling are its
// sub-arguments). Among the entries one function's calls make whose calls
// compare literal elements of one value, the lowest element's are entries
// (Redis's loadServerConfig compares argv[0] with "appendfsync"), and each
// entry comparing a higher element (argv[1] with "always") is a value of
// the entry written last before it: "appendfsync: always | everysec | no".
// redis-server had listed always, everysec, no, debug, verbose, notice and
// warning among its 37 settings.
func (r *reader) foldValues() {
	type key struct {
		caller string
		base   sourcevalue.Anchor
	}
	groups := map[key][]*boundaryState{}
	for _, state := range r.boundaries {
		if state.element == nil || !state.handlerUnknown || state.place.Boundary == nil {
			continue
		}
		at := key{caller: state.place.Boundary.ObjectID, base: state.element.base}
		groups[at] = append(groups[at], state)
	}
	for _, states := range groups {
		slices.SortFunc(states, func(a, b *boundaryState) int {
			return cmp.Or(cmp.Compare(a.place.Path, b.place.Path), cmp.Compare(a.place.LineNo, b.place.LineNo), cmp.Compare(a.place.Column, b.place.Column), cmp.Compare(a.place.ID, b.place.ID))
		})
		lowest := states[0].element.index
		for _, state := range states {
			lowest = min(lowest, state.element.index)
		}
		parent := ""
		for _, state := range states {
			if state.element.index == lowest {
				parent = state.place.ID
				continue
			}
			state.valueOf = parent
		}
	}
}
