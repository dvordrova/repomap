package reading

import (
	"context"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// readKeys chooses, inside each part the model drew, the declarations that
// explain it. The candidates are the declarations the selection found worth
// a reader's attention; a part with no more of them than it may show needs
// no question.
func (r *reader) readKeys(ctx context.Context) error {
	r.partKeys = make(map[string]bool)
	type asked struct {
		part *boxState
		ids  []string
	}
	var groups rowGroups
	var order []asked
	for _, target := range r.opts.Targets {
		parts := r.boxesOfTarget(target.ID)
		sort.Slice(parts, func(i, j int) bool { return compactIDLess(parts[i].id, parts[j].id) })
		title := make(map[string]string, len(parts))
		for _, part := range parts {
			title[part.id] = part.title
		}
		for _, part := range parts {
			if part.inventory || part.forTests || r.keysDecided[part.id] {
				continue
			}
			var candidates []string
			for id := range part.symbols {
				if r.selectedKeys[id] {
					candidates = append(candidates, id)
				}
			}
			sort.Slice(candidates, func(i, j int) bool { return compactIDLess(candidates[i], candidates[j]) })
			r.keysDecided[part.id] = true
			if len(candidates) <= lines.MaxKeysPerPart {
				for _, id := range candidates {
					r.partKeys[id] = true
				}
				continue
			}
			rows := make([]table.Row, 0, len(candidates))
			for _, id := range candidates {
				rows = append(rows, table.Row{ID: id, Fields: r.keyFields(target.ID, part, id, title)})
			}
			shared := []table.Field{{Name: "part", Value: part.title}, {Name: "purpose", Value: part.line}, {Name: "declarations", Value: r.partDeclarations(part)}}
			groups = append(groups, rowGroup{shared: shared, rows: rows})
			order = append(order, asked{part: part, ids: candidates})
		}
	}
	if len(groups) == 0 {
		return nil
	}
	r.opts.Stage(lines.StageKeys, fmt.Sprintf("choosing the declarations that explain %d parts", len(groups)))
	answers, err := r.runTableGroups(ctx, lines.Keys(), 1, groups, nil)
	if err != nil {
		return err
	}
	at := 0
	for _, group := range order {
		kept := 0
		for _, id := range group.ids {
			answer := answers[at].answer
			at++
			if answer != nil && answer["explains"] == "yes" && kept < lines.MaxKeysPerPart {
				r.partKeys[id] = true
				kept++
			}
		}
		// A part the model named no key for keeps the selection's own order.
		if kept == 0 {
			ranked := append([]string(nil), group.ids...)
			sort.SliceStable(ranked, func(i, j int) bool { return r.places[ranked[i]].Symbol.Rank < r.places[ranked[j]].Symbol.Rank })
			for _, id := range ranked[:lines.MaxKeysPerPart] {
				r.partKeys[id] = true
			}
		}
	}
	r.reportStage(lines.StageKeys)
	return nil
}

func (r *reader) keyFields(targetID string, part *boxState, id string, title map[string]string) []table.Field {
	place := r.places[id]
	decl := place.Symbol.Decl
	fields := []table.Field{{Name: "declaration", Value: decl.Name}, {Name: "kind", Value: decl.Kind}}
	if decl.Signature != "" {
		fields = append(fields, table.Field{Name: "signature", Value: decl.Signature})
	}
	if decl.Doc != "" {
		fields = append(fields, table.Field{Name: "author_documentation", Value: decl.Doc})
	}
	entry, reaches := false, []string{}
	for _, key := range sortedKeys(r.boundaries) {
		state := r.boundaries[key]
		b := state.place.Boundary
		if state.kind == atlas.BoundaryConfig || state.kind == atlas.BoundaryListenAddress || b.SubjectID != decl.ObjectID && b.ObjectID != decl.ObjectID {
			continue
		}
		if b.Direction == atlas.DirectionIn {
			entry = true
		} else if b.SubjectID == decl.ObjectID {
			reaches = appendUnique(reaches, state.kind)
		}
	}
	if entry {
		fields = append(fields, table.Field{Name: "entry", Value: true})
	}
	if len(reaches) > 0 {
		fields = append(fields, table.Field{Name: "reaches", Value: reaches})
	}
	var calledFrom []string
	for _, caller := range place.Symbol.CalledBy {
		if owner := r.boxFor(targetID, caller.PlaceID); owner != "" && owner != part.id && title[owner] != "" {
			calledFrom = appendUnique(calledFrom, title[owner])
		}
	}
	if len(calledFrom) > 0 {
		fields = append(fields, table.Field{Name: "called_from", Value: calledFrom})
	}
	return fields
}
