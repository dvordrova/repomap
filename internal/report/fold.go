package report

import (
	"github.com/dvordrova/repomap/internal/groupindex"
)

// foldIndexes is the page's view of the group graph: one group per title in
// a target (groupindex.Fold), the one part of a script program named by its
// file (scriptGroups).
func foldIndexes(indexes []groupindex.Index) []groupindex.Index {
	folded := groupindex.Fold(indexes)
	for position := range folded {
		scriptGroups(indexes[position], folded[position].Groups)
	}
	return folded
}
