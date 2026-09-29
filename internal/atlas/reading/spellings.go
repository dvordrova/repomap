package reading

import (
	"cmp"
	"slices"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// foldSpellings makes the spellings of one value one entry, as a case
// listing several words is one input named by its first spelling. A call
// that reads the same value as an earlier call (ProgramIndex SameValueAs:
// litestream's `query.Get("storage-class")` in the else-if arm after
// `query.Get("storageClass")`, or `query.Get("force-path-style")` joined by
// || to `query.Get("forcePathStyle")`) and whose own answer made it an
// entry of the same kind as another call of that read is that entry's
// alias: the call written first stands, named by its own words, and the
// others are its aliases (atlas Boundary.AliasOf). Each call keeps its own
// answer: a call answered another kind, or none, is not folded, and
// nothing is asked. Only entries a call's words made take part; a value of
// another entry (values.go) stays that entry's.
func (r *reader) foldSpellings() {
	type key struct {
		caller, kind string
		root         sourcevalue.Anchor
	}
	groups := map[key][]*boundaryState{}
	for _, state := range r.boundaries {
		facts := state.place.Boundary
		if !state.handlerUnknown || state.tableRow || state.valueOf != "" || facts == nil || facts.Source != "model" || facts.Direction != atlas.DirectionIn || state.kind == "" {
			continue
		}
		root := sourcevalue.Anchor{Path: state.place.Path, Line: state.place.LineNo, Column: state.place.Column}
		if state.sameValueAs != nil {
			root = *state.sameValueAs
		}
		at := key{caller: facts.ObjectID, kind: state.kind, root: root}
		groups[at] = append(groups[at], state)
	}
	for _, states := range groups {
		if len(states) < 2 {
			continue
		}
		slices.SortFunc(states, func(a, b *boundaryState) int {
			return cmp.Or(cmp.Compare(a.place.Path, b.place.Path), cmp.Compare(a.place.LineNo, b.place.LineNo), cmp.Compare(a.place.Column, b.place.Column), cmp.Compare(a.place.ID, b.place.ID))
		})
		for _, state := range states[1:] {
			state.aliasOf = states[0].place.ID
		}
	}
}

func cloneAnchor(value *sourcevalue.Anchor) *sourcevalue.Anchor {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
