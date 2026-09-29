package reading

import (
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
)

func (r *reader) dataForTarget(id string) []atlas.DataRecord {
	var rows []atlas.DataRecord
	known := map[string]bool{}
	for _, place := range r.opts.Graph.Places {
		// A data object written in a test file is the test's, not the
		// program's (testPath, the rule boundaries follow).
		if place.Entity == nil || place.Entity.Data == nil || !contains(place.TargetIDs, id) || r.testPath(place.Path) {
			continue
		}
		known[place.ID] = true
		rows = append(rows, atlas.DataRecord{ID: place.ID, Path: place.Path, Line: place.LineNo, Data: facts.CloneData(place.Entity.Data)})
	}
	refs := map[string][]string{}
	for _, edge := range r.opts.Graph.Edges {
		if edge.Kind == "observation" && known[edge.From] && known[edge.To] {
			refs[edge.From] = append(refs[edge.From], edge.To)
		}
	}
	for i := range rows {
		rows[i].References = refs[rows[i].ID]
		sort.Slice(rows[i].References, func(a, b int) bool { return compactIDLess(rows[i].References[a], rows[i].References[b]) })
	}
	sort.Slice(rows, func(i, j int) bool { return compactIDLess(rows[i].ID, rows[j].ID) })
	return rows
}
