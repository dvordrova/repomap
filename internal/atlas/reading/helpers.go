package reading

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// The helper question (owner, 2026-09-28): Jev decides once per unit whether
// it is a helper, code that serves the work of other declarations. A helper
// is never named or assigned; code places it with its users. Only a decided
// "helper" is one: responsibility, none of these, a near-tie, an unanswered
// row and a refused window all leave the unit named and assigned as before.

// recordedUses states, by target language and declaration kind, which uses
// of a declaration its language adapter records as facts, as the language
// contracts say: calls (decorations included), hand-overs, reads and
// registrations. A kind absent here has none: no fact names where a type is
// used as a type, a module body is used by no declaration, Go records no
// read of a variable (GO) and Clojure records no use of a macro (CLOJURE).
// What an adapter records only in part stays listed, and its gap recorded
// in its contract: Go's function values kept in a slice, a map or a
// package variable (GO), Python's unresolved module-attribute calls
// (PYTHON).
var recordedUses = map[string]map[string][]string{
	"c": {
		"function": {"calls", "hand-overs", "registrations"},
		"variable": {"reads"},
	},
	"go": {
		"function": {"calls", "hand-overs", "registrations"},
		"method":   {"calls", "hand-overs", "registrations"},
	},
	"python": {
		"function": {"calls", "decorations", "hand-overs", "reads", "registrations"},
		"method":   {"calls", "decorations", "hand-overs", "reads", "registrations"},
		"lambda":   {"calls", "hand-overs", "reads"},
		"variable": {"reads"},
	},
	"typescript": {
		"function": {"calls", "decorations", "hand-overs", "reads", "registrations"},
		"method":   {"calls", "decorations", "hand-overs", "reads", "registrations"},
		"lambda":   {"calls", "hand-overs", "reads"},
		"variable": {"reads"},
	},
	"javascript": {
		"function": {"calls", "decorations", "hand-overs", "reads", "registrations"},
		"method":   {"calls", "decorations", "hand-overs", "reads", "registrations"},
		"lambda":   {"calls", "hand-overs", "reads"},
		"variable": {"reads"},
	},
	"clojure": {
		"function": {"calls", "hand-overs", "reads"},
		"variable": {"reads"},
	},
}

// useKind is the unit's declaration kind as recordedUses knows it: a macro
// is a kind of its own, whatever kind its index gives it.
func (unit *roleUnit) useKind() string {
	if unit.macro {
		return "macro"
	}
	return unit.kind
}

// askedHelper says whether the helper question asks about a unit of a
// target in language. A unit of a kind whose uses the adapter records
// (recordedUses) that nothing in the program uses (no call, decoration,
// hand-over or read, exact or among alternatives, and no registration; test
// and generated code left out) is no helper by code: an entry point or an
// operation a library offers is used by no declaration of its own. Any
// other unit is asked, since no recorded use says nothing of its users:
// they are unknown, not none.
func (unit *roleUnit) askedHelper(language string) bool {
	if len(recordedUses[language][unit.useKind()]) == 0 {
		return true
	}
	return unit.used
}

// helperItem is a unit as the helper question asks about it: its name,
// kind, file, signature, code lines and methods, the declarations of the
// program it calls and that call it, read it or hand it over ("path:name",
// test and generated code left out), and the words of its registrations.
func (unit *roleUnit) helperItem(facts *roleFacts) []table.Field {
	item := []table.Field{{Name: "name", Value: unit.name}, {Name: "kind", Value: unit.kind}, {Name: "file", Value: unit.path}}
	if unit.signature != "" {
		item = append(item, table.Field{Name: "signature", Value: unit.signature})
	}
	if unit.lines > 0 {
		item = append(item, table.Field{Name: "lines", Value: unit.lines})
	}
	if len(unit.methods) > 0 {
		item = append(item, table.Field{Name: "methods", Value: unit.methods})
	}
	for _, list := range []struct {
		name string
		ids  map[string]bool
	}{{"calls", unit.callees}, {"called_by", unit.calledByAll}, {"read_by", unit.readers}, {"handed_over_by", unit.handers}} {
		if labels := facts.labels(list.ids); len(labels) > 0 {
			item = append(item, table.Field{Name: list.name, Value: labels})
		}
	}
	if len(unit.registered) > 0 {
		item = append(item, table.Field{Name: "registered", Value: unit.registered})
	}
	return item
}

// labels names units as "path:name", ordered by path and name.
func (facts *roleFacts) labels(ids map[string]bool) []string {
	units := make([]*roleUnit, 0, len(ids))
	for id := range ids {
		if unit := facts.units[id]; unit != nil {
			units = append(units, unit)
		}
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].path != units[j].path {
			return units[i].path < units[j].path
		}
		return units[i].name < units[j].name
	})
	labels := make([]string, len(units))
	for i, unit := range units {
		labels[i] = unit.path + ":" + unit.name
	}
	return labels
}

// askHelpers asks the helper question about every asked unit, one row group
// per file in f* order with rows keyed dN by the unit's place N in its file,
// and returns the units decided "helper". A declaration nothing uses is not
// asked; tables.md names it.
func (r *reader) askHelpers(ctx context.Context, round int, facts *roleFacts) (map[string]bool, error) {
	helpers := map[string]bool{}
	target := r.opts.Targets[round-1]
	var groups rowGroups
	var asked []*roleUnit
	var unused, seeds []string
	for _, file := range facts.files {
		var group rowGroup
		for i, unit := range file.units {
			if unit.seed {
				seeds = append(seeds, unit.path+":"+unit.name)
				continue
			}
			if !unit.askedHelper(target.Language) {
				unused = append(unused, unit.path+":"+unit.name)
				continue
			}
			group.rows = append(group.rows, table.Row{ID: fmt.Sprintf("d%d", i+1), Fields: unit.helperItem(facts)})
			asked = append(asked, unit)
		}
		if len(group.rows) > 0 {
			groups = append(groups, group)
		}
	}
	if len(asked) > 0 {
		r.opts.Stage(lines.StageRoleHelper, fmt.Sprintf("%s: asking whether %d declarations of %d files are helpers", target.Name, len(asked), len(groups)))
		answers, err := r.runTableGroups(ctx, lines.RoleHelper(), round, groups, nil)
		if err != nil {
			return nil, err
		}
		for i, unit := range asked {
			if answers[i].answer["helper"] == lines.RoleHelperHelper {
				helpers[unit.id] = true
			}
		}
	}
	fmt.Fprintf(&r.tables, "role helper: %d of %d asked declarations are helpers; %d that nothing uses are no helpers by code", len(helpers), len(asked), len(unused))
	if len(unused) > 0 {
		fmt.Fprintf(&r.tables, ": %s", strings.Join(unused, " "))
	}
	if len(seeds) > 0 {
		fmt.Fprintf(&r.tables, "; %d seeds are not asked: %s", len(seeds), strings.Join(seeds, " "))
	}
	r.tables.WriteString("\n\n")
	return helpers, nil
}
