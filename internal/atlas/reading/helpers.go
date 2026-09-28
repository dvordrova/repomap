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

// groupKey is a row of the parts request code can place a unit in: a box of
// a split file, a whole file's row (box -1), or the row of a split file's
// seed (box -1 and the seed's unit).
type groupKey struct {
	file string
	box  int
	seed string
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
	// attached are units of split files placed in a row that is no box of
	// their own file (a row of another file, or a seed's row); attachedFiles
	// are whole files that joined a box of a split file.
	attached      map[string]groupKey
	attachedFiles map[string]groupKey
	// What each rule placed, by the file of what it placed: rule A's
	// helpers, rule B's files (by the file whose box they joined), rule C's
	// open units by their users or by what they use, and the second pass.
	byHelperUsers, byUsers, byUses map[string][]string
	joined                         map[string][]string
	secondAsked, secondPlaced      map[string][]string
	// asked are the helpers the second pass asked, in any round, and rounds
	// what each of its rounds asked and placed, in round order.
	asked  map[string]bool
	rounds []passRound
	// settledOut is set once the second pass has nothing left to ask: a
	// unit still open then can never get a row, and the rules stop waiting
	// on it as a user (keysOf). One near-tie (lookupKeyRead) had kept
	// every helper it uses (expireIfNeeded, lookupKey and three VM
	// functions after them) off the map, blocked.
	settledOut bool
}

// passRound is one round of the second pass: the helpers it asked and how
// many of them its answers placed.
type passRound struct {
	asked, placed int
}

func newPlacement(facts *roleFacts, helpers map[string]bool) *placement {
	return &placement{facts: facts, helpers: helpers, split: map[string]*splitFile{},
		attached: map[string]groupKey{}, attachedFiles: map[string]groupKey{},
		byHelperUsers: map[string][]string{}, byUsers: map[string][]string{}, byUses: map[string][]string{},
		joined: map[string][]string{}, secondAsked: map[string][]string{}, secondPlaced: map[string][]string{}, asked: map[string]bool{}}
}

// resolve follows a whole file's row to the box the file joined.
func (p *placement) resolve(key groupKey) groupKey {
	if key.box < 0 && key.seed == "" {
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
		if unit.seed {
			return groupKey{file: unit.file, box: -1, seed: id}, true
		}
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
			// Once nothing more can be asked, an open unit is absent.
			open = open || !p.settledOut
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
// its command table or registrar, and neither does a call through the
// function value a hand-over stored, so a callback never follows the code
// that runs what was stored (listDup, dupClientReplyValue).
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
//     left open takes the row of its file every unit of it that uses it
//     stands in, a box or a seed's own row; with none using it, the row of
//     every unit of its file it uses that is no helper, or, once nothing
//     more is asked, of the helpers it uses when it uses nothing else.
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
				if key := onlyKey(keys); key.file == file.file.id && key.seed == "" {
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
			// oneRow is the row of this file every listed unit of it stands
			// in: one of its boxes, or the row of one of its seeds.
			oneRow := func(ids []string) (groupKey, bool) {
				var row groupKey
				for i, id := range ids {
					key, ok := p.keyOf(id)
					if !ok || key.file != file.file.id || key.box < 0 && key.seed == "" || i > 0 && key != row {
						return groupKey{}, false
					}
					row = key
				}
				return row, len(ids) > 0
			}
			place := func(unit *roleUnit, row groupKey) {
				if row.seed == "" {
					split.box[unit.id] = row.box
				} else {
					p.attached[unit.id] = row
				}
				changed = true
			}
			for _, unit := range file.units {
				if p.helpers[unit.id] || unit.seed || !p.open(unit.id) {
					continue
				}
				var users, uses []string
				for id := range unit.users {
					if file.byID[id] != nil {
						users = append(users, id)
					}
				}
				if len(users) > 0 {
					if row, ok := oneRow(users); ok {
						place(unit, row)
						p.byUsers[file.file.id] = append(p.byUsers[file.file.id], unit.name)
					}
					continue
				}
				for id := range unit.uses {
					if file.byID[id] != nil && !p.helpers[id] {
						uses = append(uses, id)
					}
				}
				// Once nothing more is asked, a unit that uses only helpers
				// of its file takes the row they got: the fixture's
				// RunEventLoop runs eventLoop, a helper only it uses.
				if len(uses) == 0 && p.settledOut {
					for id := range unit.uses {
						if file.byID[id] != nil {
							uses = append(uses, id)
						}
					}
				}
				if row, ok := oneRow(uses); ok {
					place(unit, row)
					p.byUses[file.file.id] = append(p.byUses[file.file.id], unit.name)
				}
			}
		}
	}
}

// secondPass asks the assignment once more, after code has settled, about
// the helpers of split files code left open that their users share between
// rows or that nothing uses, with every named box of their file as the
// options, and code then settles again. A helper with a user still open is
// not asked yet: once its users have rows, code places it (rule A, when they
// share one) or a further round asks it (when they stand in two or more).
// Rounds repeat until none qualifies, which ends, since each unit is asked
// at most once. Each round is a round of windows of its own, the k-th at
// len(targets)·k + round, after every target's first pass and each other's,
// so no window overwrites another. A near-tie leaves the helper undecided; a
// helper never asked because a unit that uses it never got a row is
// blocked.
//
// When a round asks nothing, the units still open can never get a row: the
// rules settle once more without waiting on them as users, and the rounds
// go on asking the helpers whose users with rows share two or more rows,
// or have none, until again none qualifies.
func (r *reader) secondPass(ctx context.Context, round int, p *placement) error {
	for pass := 1; ; pass++ {
		asked, err := r.passRound(ctx, round, pass, p)
		if err != nil {
			return err
		}
		if !asked {
			if p.settledOut {
				return nil
			}
			p.settledOut = true
			p.settle()
		}
	}
}

// passRound asks the pass-th round of the second pass and settles; it says
// whether any helper qualified.
func (r *reader) passRound(ctx context.Context, round, pass int, p *placement) (bool, error) {
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
			if !p.helpers[unit.id] || !p.open(unit.id) || p.asked[unit.id] {
				continue
			}
			if keys, _ := p.keysOf(unit.users); len(unit.users) == 0 || len(keys) >= 2 || p.settledOut && len(keys) == 0 {
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
		return false, nil
	}
	target := r.opts.Targets[round-1]
	if pass == 1 {
		r.opts.Stage(lines.StageRoleAssign, fmt.Sprintf("%s: asking once more where %d helpers go that their users share or nothing uses", target.Name, groups.count()))
	} else {
		r.opts.Stage(lines.StageRoleAssign, fmt.Sprintf("%s: asking where %d helpers go whose users got their boxes in round %d", target.Name, groups.count(), pass-1))
	}
	answers, err := r.runTableGroups(ctx, lines.RoleAssign(), len(r.opts.Targets)*pass+round, groups, nil)
	if err != nil {
		return false, err
	}
	at, placed := 0, 0
	for f, split := range files {
		id := split.file.file.id
		for _, unit := range asked[f] {
			p.asked[unit.id] = true
			p.secondAsked[id] = append(p.secondAsked[id], unit.name)
			if box := boxChoice(answers[at].answer, split.boxes); box >= 0 {
				split.box[unit.id] = box
				p.secondPlaced[id] = append(p.secondPlaced[id], unit.name)
				placed++
			}
			at++
		}
	}
	p.rounds = append(p.rounds, passRound{asked: at, placed: placed})
	p.settle()
	return true, nil
}

// blocked says whether a unit is a helper code left open that the second
// pass never asked: a unit that uses it never got a row.
func (p *placement) blocked(id string) bool {
	return p.helpers[id] && p.open(id) && !p.asked[id]
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
	if len(p.rounds) > 0 {
		r.tables.WriteString("role second pass:")
		for i, round := range p.rounds {
			fmt.Fprintf(&r.tables, " round %d asked %d, placed %d;", i+1, round.asked, round.placed)
		}
		r.tables.WriteString("\n\n")
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
