package reading

import (
	"context"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const maxCoreDeclarations = 12

// readCore asks the model which parts a program exists for and which exist
// only for its tests. The code says what it observed about each part; what
// the part means to the program is the model's to say.
func (r *reader) readCore(ctx context.Context) error {
	for _, target := range r.opts.Targets {
		// The code answers what it can see: a part made of test files exists
		// for the tests, and the part where the program starts calls the core
		// rather than being it. The model is asked about the rest.
		var parts []*boxState
		for _, part := range r.boxesOfTarget(target.ID) {
			if r.onlyTestFiles(part) {
				part.forTests = true
				continue
			}
			if !part.inventory && !r.startsProgram(part, target.ID) {
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
			listed = append(listed, part.title+" — "+part.line)
		}
		rows := make([]table.Row, 0, len(parts))
		for _, part := range parts {
			fields := []table.Field{{Name: "part", Value: part.title}, {Name: "purpose", Value: part.line}}
			if names := r.partDeclarations(part); len(names) > 0 {
				fields = append(fields, table.Field{Name: "declarations", Value: names})
			}
			entries, reaches := 0, []string{}
			for _, id := range sortedKeys(r.boundaries) {
				state := r.boundaries[id]
				if state.kind == atlas.BoundaryConfig || !contains(state.place.TargetIDs, target.ID) || r.boundaryBox(target.ID, state.place) != part.id {
					continue
				}
				if state.kind == atlas.BoundaryListenAddress {
					// Where the program listens is not a request entering.
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
		r.opts.Stage(lines.StageCore, fmt.Sprintf("%s: asking which of %d parts the program exists for", target.Name, len(parts)))
		answers, err := r.runTableWith(ctx, lines.Core(), 1, []table.Field{{Name: "parts", Value: listed}}, rows, nil)
		if err != nil {
			return err
		}
		for i, part := range parts {
			answer := answers[i].answer
			if answer == nil {
				continue
			}
			part.forTests = answer["for_tests"] == "yes"
			// What exists for the tests is not what the program exists for.
			part.core = answer["core"] == "yes" && !part.forTests
		}
	}
	r.reportStage(lines.StageCore)
	return nil
}

func (r *reader) onlyTestFiles(part *boxState) bool {
	for _, fileID := range part.files {
		if file := r.places[fileID].File; file == nil || !file.Test {
			return false
		}
	}
	return len(part.files) > 0
}

func (r *reader) startsProgram(part *boxState, targetID string) bool {
	for _, fileID := range part.files {
		if contains(r.opts.Graph.Seeds, fileID) && r.boxFor(targetID, fileID) == part.id {
			return true
		}
	}
	return false
}

func (r *reader) partDeclarations(part *boxState) []string {
	ids := make([]string, 0, len(part.symbols))
	for id := range part.symbols {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	var names []string
	for _, id := range ids {
		if symbol := r.places[id].Symbol; symbol != nil && len(names) < maxCoreDeclarations {
			names = append(names, symbol.Decl.Name)
		}
	}
	return names
}
