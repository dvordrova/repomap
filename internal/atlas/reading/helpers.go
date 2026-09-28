package reading

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// The helper question (owner, 2026-09-28): Jev decides once per unit whether
// it is a helper, code that serves the work of other declarations. A helper
// is never named or assigned; code places it with its users. Only a decided
// "helper" is one: responsibility, none of these, a near-tie, an unanswered
// row and a refused window all leave the unit named and assigned as before.

// askedHelper says whether the helper question asks about a unit. A
// function, method, lambda or variable nothing in the program uses (no
// call, decoration, hand-over or read, exact or among alternatives, and no
// registration; test and generated code left out) is no helper by code: an
// entry point or an operation a library offers is used by no declaration of
// its own. A type and a module body are always asked, since no fact names
// where a type is used: their users are unknown, not none.
func (unit *roleUnit) askedHelper() bool {
	switch unit.kind {
	case "function", "method", "lambda", "variable":
		return unit.used
	}
	return true
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
	var groups rowGroups
	var asked []*roleUnit
	var unused []string
	for _, file := range facts.files {
		var group rowGroup
		for i, unit := range file.units {
			if !unit.askedHelper() {
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
	target := r.opts.Targets[round-1]
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
	r.tables.WriteString("\n\n")
	return helpers, nil
}

// groupKey is a row of the parts request code can place a unit in: a box of
// a split file, or a whole file's row (box -1).
type groupKey struct {
	file string
	box  int
}

// splitFile is a file the assignment splits: its boxes and, by unit, the
// index of the box it goes in, -1 while it has none.
type splitFile struct {
	file  *roleFile
	boxes []roleBox
	box   map[string]int
}

// placement is where the role split's code rules put a target's units, and
// what they did.
type placement struct {
	facts   *roleFacts
	helpers map[string]bool
	split   map[string]*splitFile
	// attached are helpers of split files placed in a row of another file;
	// attachedFiles are whole files that joined a box of a split file.
	attached      map[string]groupKey
	attachedFiles map[string]groupKey
	// What each rule placed, by the file of what it placed: rule A's
	// helpers, rule B's files (by the file whose box they joined), rule C's
	// open units by their users or by what they use, and the second pass.
	byHelperUsers, byUsers, byUses map[string][]string
	joined                         map[string][]string
	secondAsked, secondPlaced      map[string][]string
}

func newPlacement(facts *roleFacts, helpers map[string]bool) *placement {
	return &placement{facts: facts, helpers: helpers, split: map[string]*splitFile{},
		attached: map[string]groupKey{}, attachedFiles: map[string]groupKey{},
		byHelperUsers: map[string][]string{}, byUsers: map[string][]string{}, byUses: map[string][]string{},
		joined: map[string][]string{}, secondAsked: map[string][]string{}, secondPlaced: map[string][]string{}}
}

// resolve follows a whole file's row to the box the file joined.
func (p *placement) resolve(key groupKey) groupKey {
	if key.box < 0 {
		if to, ok := p.attachedFiles[key.file]; ok {
			return to
		}
	}
	return key
}

// keyOf is the row a unit stands in, or false while it has none.
func (p *placement) keyOf(id string) (groupKey, bool) {
	unit := p.facts.units[id]
	if unit == nil {
		return groupKey{}, false
	}
	if split := p.split[unit.file]; split != nil {
		if box := split.box[id]; box >= 0 {
			return groupKey{file: unit.file, box: box}, true
		}
		if key, ok := p.attached[id]; ok {
			return p.resolve(key), true
		}
		return groupKey{}, false
	}
	return p.resolve(groupKey{file: unit.file, box: -1}), true
}

// open says whether a unit of a split file has no row yet.
func (p *placement) open(id string) bool {
	_, placed := p.keyOf(id)
	return !placed
}

// keysOf are the rows the given units stand in, and whether one has none.
func (p *placement) keysOf(ids map[string]bool) (map[groupKey]bool, bool) {
	keys := map[groupKey]bool{}
	open := false
	for id := range ids {
		key, ok := p.keyOf(id)
		if !ok {
			open = true
			continue
		}
		keys[key] = true
	}
	return keys, open
}

// onlyKey is the one key of a set of one.
func onlyKey(keys map[groupKey]bool) groupKey {
	for key := range keys {
		return key
	}
	return groupKey{}
}

// settle runs the code rules to a fixed point, since what one places may
// settle another. Users are callers, decorated declarations and readers of
// what does not run; a hand-over never counts, so a handler never follows
// its command table or registrar.
//
//   - A: a helper of a split file whose users, in any file, all stand in one
//     row joins that row: a box of its file, a box of another split file or
//     a whole file's row.
//   - B: a whole file of helpers (owner's map model: a library file is a
//     file of helpers), every unit of which, its types included, is a
//     decided helper, and all of whose outside users stand in one box of a
//     split file, joins that box. One unit that is no helper (a type
//     answered responsibility, an entry nothing uses, a near-tie) keeps the
//     file out. A whole file never joins a whole file.
//   - C: a unit of a split file that is no helper and that the assignment
//     left open takes the box every unit of its file that uses it has; with
//     none using it, the box of every unit of its file it uses that is no
//     helper.
func (p *placement) settle() {
	for changed := true; changed; {
		changed = false
		for _, file := range p.facts.files {
			split := p.split[file.file.id]
			if split == nil {
				continue
			}
			for _, unit := range file.units {
				if !p.helpers[unit.id] || !p.open(unit.id) || len(unit.users) == 0 {
					continue
				}
				keys, open := p.keysOf(unit.users)
				if open || len(keys) != 1 {
					continue
				}
				if key := onlyKey(keys); key.file == file.file.id {
					split.box[unit.id] = key.box
				} else {
					p.attached[unit.id] = key
				}
				p.byHelperUsers[file.file.id] = append(p.byHelperUsers[file.file.id], unit.name)
				changed = true
			}
		}
		for _, file := range p.facts.files {
			id := file.file.id
			if p.split[id] != nil {
				continue
			}
			if _, joined := p.attachedFiles[id]; joined {
				continue
			}
			outside := map[string]bool{}
			helpersOnly := len(file.units) > 0
			for _, unit := range file.units {
				if !p.helpers[unit.id] {
					helpersOnly = false
					break
				}
				for user := range unit.users {
					if p.facts.units[user].file != id {
						outside[user] = true
					}
				}
			}
			if !helpersOnly || len(outside) == 0 {
				continue
			}
			keys, open := p.keysOf(outside)
			if open || len(keys) != 1 {
				continue
			}
			key := onlyKey(keys)
			if key.box < 0 {
				continue
			}
			p.attachedFiles[id] = key
			p.joined[key.file] = append(p.joined[key.file], file.file.path)
			changed = true
		}
		for _, file := range p.facts.files {
			split := p.split[file.file.id]
			if split == nil {
				continue
			}
			// oneBox is the box of this file every listed unit of it stands in.
			oneBox := func(ids []string) int {
				box := -1
				for _, id := range ids {
					key, ok := p.keyOf(id)
					if !ok || key.file != file.file.id || key.box < 0 || box >= 0 && key.box != box {
						return -1
					}
					box = key.box
				}
				return box
			}
			for _, unit := range file.units {
				if p.helpers[unit.id] || !p.open(unit.id) {
					continue
				}
				var users, uses []string
				for id := range unit.users {
					if file.byID[id] != nil {
						users = append(users, id)
					}
				}
				if len(users) > 0 {
					if box := oneBox(users); box >= 0 {
						split.box[unit.id] = box
						p.byUsers[file.file.id] = append(p.byUsers[file.file.id], unit.name)
						changed = true
					}
					continue
				}
				for id := range unit.uses {
					if file.byID[id] != nil && !p.helpers[id] {
						uses = append(uses, id)
					}
				}
				if box := oneBox(uses); len(uses) > 0 && box >= 0 {
					split.box[unit.id] = box
					p.byUses[file.file.id] = append(p.byUses[file.file.id], unit.name)
					changed = true
				}
			}
		}
	}
}

// secondPass asks the assignment once more, in a round of its own, about
// the helpers of split files code left open that their users share between
// rows or that nothing uses, with every named box of their file as the
// options; a helper whose users are still open is not asked. Code then
// settles again. A near-tie leaves the helper undecided.
func (r *reader) secondPass(ctx context.Context, round int, p *placement) error {
	var files []*splitFile
	var asked [][]*roleUnit
	var groups rowGroups
	for _, file := range p.facts.files {
		split := p.split[file.file.id]
		if split == nil {
			continue
		}
		var rest []*roleUnit
		for _, unit := range file.units {
			if !p.helpers[unit.id] || !p.open(unit.id) {
				continue
			}
			if keys, _ := p.keysOf(unit.users); len(unit.users) == 0 || len(keys) >= 2 {
				rest = append(rest, unit)
			}
		}
		if len(rest) == 0 {
			continue
		}
		chosen := map[string]bool{}
		for _, unit := range rest {
			chosen[unit.id] = true
		}
		files = append(files, split)
		asked = append(asked, rest)
		groups = append(groups, file.assignGroup(split.boxes, func(unit *roleUnit) bool { return chosen[unit.id] }))
	}
	if len(groups) == 0 {
		return nil
	}
	target := r.opts.Targets[round-1]
	r.opts.Stage(lines.StageRoleAssign, fmt.Sprintf("%s: asking once more where %d helpers go that their users share or nothing uses", target.Name, groups.count()))
	// The second pass has a round of its own after every target's first, so
	// its windows never overwrite the first pass's.
	answers, err := r.runTableGroups(ctx, lines.RoleAssign(), len(r.opts.Targets)+round, groups, nil)
	if err != nil {
		return err
	}
	at := 0
	for f, split := range files {
		id := split.file.file.id
		for _, unit := range asked[f] {
			p.secondAsked[id] = append(p.secondAsked[id], unit.name)
			if box := boxChoice(answers[at].answer, split.boxes); box >= 0 {
				split.box[unit.id] = box
				p.secondPlaced[id] = append(p.secondPlaced[id], unit.name)
			}
			at++
		}
	}
	p.settle()
	return nil
}

// recordPlacement records, by file, what each code rule and the second pass
// placed, in rejected.jsonl and tables.md.
func (r *reader) recordPlacement(targetID string, p *placement) {
	for _, file := range p.facts.files {
		id, path := file.file.id, file.file.path
		note := func(kind string, names []string, reason, printed string) {
			if len(names) == 0 {
				return
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: targetID, Kind: kind, Count: len(names), Samples: names, Reason: path + ": " + reason})
			fmt.Fprintf(&r.tables, "%s: %s: %s\n\n", path, printed, strings.Join(names, " "))
		}
		note("role_attached", p.byHelperUsers[id], "helpers whose users all stand in one box or file, placed there by code", "helpers placed with their users")
		note("role_attached", p.joined[id], "whole files of helpers only, used from other files by one box of this file alone, joined to it by code", "files joined to a box")
		note("role_placed_by_users", p.byUsers[id], "declarations the assignment left open, placed in the one box of the file's declarations that use them", "placed by their users")
		note("role_placed_by_uses", p.byUses[id], "declarations the assignment left open that no declaration of the file uses, placed in the one box of what they use that is no helper", "placed by what they use")
		if asked := p.secondAsked[id]; len(asked) > 0 {
			note("role_second_pass", asked, fmt.Sprintf("helpers their users share between boxes or nothing uses, asked once more: %d placed", len(p.secondPlaced[id])), "asked once more")
		}
	}
}

// resolved are the attachments with every whole file's row followed to the
// box the file joined.
func (p *placement) resolved() (map[string]groupKey, map[string]groupKey) {
	attached := make(map[string]groupKey, len(p.attached))
	for id, key := range p.attached {
		attached[id] = p.resolve(key)
	}
	return attached, p.attachedFiles
}

// into lists, by row, the units code placed there from other files: the
// helpers of split files attached there and the units of whole files that
// joined a box, in f* and unit order.
func (p *placement) into() map[groupKey][]string {
	into := map[groupKey][]string{}
	for _, file := range p.facts.files {
		if key, joined := p.attachedFiles[file.file.id]; joined {
			for _, unit := range file.units {
				into[key] = append(into[key], unit.id)
			}
			continue
		}
		for _, unit := range file.units {
			if key, ok := p.attached[unit.id]; ok {
				key = p.resolve(key)
				into[key] = append(into[key], unit.id)
			}
		}
	}
	return into
}
