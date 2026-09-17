package reading

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	layerDepth       = 8
	layerSourceLines = 60
)

// readLayers walks from every bound entry along the calls of the graph to
// every declaration that makes an outgoing call, and asks the model what
// each declaration on the way does — from its source, the first table that
// reads code.
func (r *reader) readLayers(ctx context.Context) error {
	r.roles = make(map[string]string)
	owners := make(map[string]atlas.Place)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol != nil {
			owners[place.ID] = place
			if place.Symbol.Decl.ObjectID != "" {
				owners[place.Symbol.Decl.ObjectID] = place
			}
		}
	}
	var entries []string
	leaving := make(map[string]bool)
	for _, id := range sortedKeys(r.boundaries) {
		state := r.boundaries[id]
		b := state.place.Boundary
		owner := boundaryOwner(b, owners)
		if owner.Symbol == nil {
			continue
		}
		switch {
		case b.Direction == atlas.DirectionIn && b.ObjectID != "" && state.kind != atlas.BoundaryListenAddress:
			// A bound callable is an entry; the declaration that starts the
			// listener is not.
			entries = appendUnique(entries, owner.ID)
		case b.Direction == atlas.DirectionOut && state.kind != atlas.BoundaryConfig:
			leaving[owner.ID] = true
		}
	}
	// An activation the operations table accepted is an entry as well.
	for id, operation := range r.operations {
		if operation[0] != "" && owners[id].Symbol != nil {
			entries = appendUnique(entries, id)
		}
	}
	sort.Strings(entries)
	type node struct {
		place  atlas.Place
		before []string
		after  []string
	}
	nodes := make(map[string]*node)
	var order []string
	touch := func(path []string) {
		for i, id := range path {
			current := nodes[id]
			if current == nil {
				current = &node{place: owners[id]}
				nodes[id] = current
				order = append(order, id)
			}
			if i > 0 {
				current.before = appendUnique(current.before, owners[path[i-1]].Symbol.Decl.Name)
			}
			if i < len(path)-1 {
				current.after = appendUnique(current.after, owners[path[i+1]].Symbol.Decl.Name)
			}
		}
	}
	for _, entry := range entries {
		onPath := map[string]bool{entry: true}
		var walk func(path []string)
		walk = func(path []string) {
			current := path[len(path)-1]
			if leaving[current] {
				touch(path)
			}
			if len(path) >= layerDepth {
				return
			}
			for _, call := range owners[current].Symbol.Calls {
				for _, next := range call.CalleeIDs {
					if onPath[next] || owners[next].Symbol == nil {
						continue
					}
					onPath[next] = true
					walk(append(path, next))
					delete(onPath, next)
				}
			}
		}
		walk([]string{entry})
	}
	var rows []table.Row
	for _, id := range order {
		current := nodes[id]
		decl := current.place.Symbol.Decl
		fields := []table.Field{{Name: "name", Value: decl.Name}, {Name: "path", Value: current.place.Path}, {Name: "line", Value: decl.LineNo}}
		if decl.Signature != "" {
			fields = append(fields, table.Field{Name: "signature", Value: decl.Signature})
		}
		if source := r.source(current.place.Path, decl.LineNo, decl.EndLine); source != "" {
			fields = append(fields, table.Field{Name: "source", Value: source})
		}
		if len(current.before) > 0 {
			sort.Strings(current.before)
			fields = append(fields, table.Field{Name: "before", Value: current.before})
		}
		if len(current.after) > 0 {
			sort.Strings(current.after)
			fields = append(fields, table.Field{Name: "after", Value: current.after})
		}
		rows = append(rows, table.Row{ID: id, Fields: fields})
	}
	r.opts.Stage(lines.StageLayers, fmt.Sprintf("reading %d declarations on the ways from %d entries to outgoing calls", len(rows), len(entries)))
	answers, err := r.runTable(ctx, lines.Layers(), 1, rows)
	if err != nil {
		return err
	}
	for i, id := range order {
		if answer := answers[i].answer; answer != nil && answer["role"] != "" {
			r.roles[id] = answer["role"]
		}
	}
	r.reportStage(lines.StageLayers)
	return nil
}

// source is a declaration's lines as written, numbered, from its first line
// to its last known line, the middle elided beyond the budget.
func (r *reader) source(path string, from, to int) string {
	if r.opts.ReadSource == nil || from < 1 || to < from {
		return ""
	}
	content, err := r.opts.ReadSource(path)
	if err != nil {
		return ""
	}
	text := strings.Split(string(content), "\n")
	if to > len(text) {
		to = len(text)
	}
	var out []string
	for line := from; line <= to; line++ {
		if to-from+1 > layerSourceLines && line == from+layerSourceLines/2 {
			out = append(out, "…")
			line = to - layerSourceLines/2
		}
		out = append(out, fmt.Sprintf("%d  %s", line, text[line-1]))
	}
	return strings.Join(out, "\n")
}
