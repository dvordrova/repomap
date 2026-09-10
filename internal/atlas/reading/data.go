package reading

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"sort"
)

func (r *reader) dataForTarget(id string) []atlas.DataRecord {
	var rows []atlas.DataRecord
	known := map[string]bool{}
	for _, place := range r.opts.Graph.Places {
		if place.Entity == nil || place.Entity.Data == nil || !contains(place.TargetIDs, id) {
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
		sort.Strings(rows[i].References)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}
