package reading

import (
	"context"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// readCore asks the model the one role each part plays. The program exists
// for its domain parts.
func (r *reader) readCore(ctx context.Context) error {
	for round, target := range r.opts.Targets {
		// The code answers what it can see: a part made only of test code is
		// the tests' (a fact of its files), and the part where the program
		// starts calls the core rather than being it. The model is asked about
		// the rest.
		var parts []*boxState
		for _, part := range r.boxesOfTarget(target.ID) {
			if !part.offCanvas() && !r.startsProgram(part, target.ID) {
				parts = append(parts, part)
			}
		}
		sort.Slice(parts, func(i, j int) bool { return compactIDLess(parts[i].id, parts[j].id) })
		if len(parts) < 2 {
			continue
		}
		title := make(map[string]string, len(parts))
		var listed []string
		for _, part := range parts {
			title[part.id] = part.title
			if part.line == "" {
				listed = append(listed, part.title)
			} else {
				listed = append(listed, part.title+" — "+part.line)
			}
		}
		rows := make([]table.Row, 0, len(parts))
		for _, part := range parts {
			fields := []table.Field{{Name: "part", Value: part.title}}
			if part.line != "" {
				fields = append(fields, table.Field{Name: "purpose", Value: part.line})
			}
			if names := r.partDeclarations(part); len(names) > 0 {
				fields = append(fields, table.Field{Name: "declarations", Value: names})
			}
			entries, reaches := 0, []string{}
			for _, id := range sortedKeys(r.boundaries) {
				state := r.boundaries[id]
				if state.kind == atlas.BoundaryConfig || !contains(state.place.TargetIDs, target.ID) || r.boundaryBox(target.ID, state.place) != part.id {
					continue
				}
				if state.kind == atlas.BoundaryListenAddress || state.handlerUnknown {
					// Where the program listens is not a request entering,
					// and an entry whose handler is not established enters
					// no part.
					continue
				}
				if state.place.Boundary.Direction == atlas.DirectionIn {
					entries++
				} else {
					reaches = appendUnique(reaches, state.kind)
				}
			}
			if entries > 0 {
				fields = append(fields, table.Field{Name: "entries", Value: entries})
			}
			if len(reaches) > 0 {
				fields = append(fields, table.Field{Name: "reaches", Value: reaches})
			}
			var calledBy, calls []string
			for _, arrow := range r.arrows[target.ID] {
				if arrow.to == part.id && title[arrow.from] != "" {
					calledBy = appendUnique(calledBy, title[arrow.from])
				}
				if arrow.from == part.id && title[arrow.to] != "" {
					calls = appendUnique(calls, title[arrow.to])
				}
			}
			if len(calledBy) > 0 {
				fields = append(fields, table.Field{Name: "called_by", Value: calledBy})
			}
			if len(calls) > 0 {
				fields = append(fields, table.Field{Name: "calls", Value: calls})
			}
			rows = append(rows, table.Row{ID: part.id, Fields: fields})
		}
		// A part is asked beside its siblings: the parts of the same area of
		// the grouping tree, or the target's parts outside every area. All
		// of a large target's parts would not fit one question's context.
		areaOf := map[string]int{}
		for i, zone := range r.treeZones[target.ID] {
			for _, id := range zone.parts {
				areaOf[id] = i + 1
			}
		}
		var groups rowGroups
		var members [][]int
		at := map[int]int{}
		for i, part := range parts {
			g, ok := at[areaOf[part.id]]
			if !ok {
				g = len(groups)
				at[areaOf[part.id]] = g
				groups, members = append(groups, rowGroup{}), append(members, nil)
			}
			groups[g].rows = append(groups[g].rows, rows[i])
			members[g] = append(members[g], i)
		}
		var order []int
		for g := range groups {
			siblings := make([]string, len(members[g]))
			for j, i := range members[g] {
				siblings[j] = listed[i]
			}
			groups[g].shared = []table.Field{{Name: "parts", Value: siblings}}
			order = append(order, members[g]...)
		}
		r.opts.Stage(lines.StageCore, fmt.Sprintf("%s: asking which of %d parts the program exists for", target.Name, len(parts)))
		answers, err := r.runTableGroups(ctx, lines.Core(), round+1, groups, nil)
		if err != nil {
			return err
		}
		for k, i := range order {
			part := parts[i]
			answer := answers[k].answer
			if answer == nil {
				continue
			}
			part.role = answer["role"]
			part.core = part.role == lines.PartDomain
		}
	}
	r.reportStage(lines.StageCore)
	return nil
}

func (r *reader) startsProgram(part *boxState, targetID string) bool {
	for _, fileID := range part.files {
		if contains(r.opts.Graph.Seeds, fileID) && contains(r.seedBoxes(targetID, fileID), part.id) {
			return true
		}
	}
	return false
}

// partDeclarations names every declaration a part holds, in ID order. The
// keys prompt calls this list everything the part holds and each core row
// carries it as the part's declarations, so nothing is cut to a count; names
// alone keep it small.
func (r *reader) partDeclarations(part *boxState) []string {
	ids := make([]string, 0, len(part.symbols))
	for id := range part.symbols {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	var names []string
	for _, id := range ids {
		if symbol := r.places[id].Symbol; symbol != nil {
			names = append(names, symbol.Decl.Name)
		}
	}
	return names
}
