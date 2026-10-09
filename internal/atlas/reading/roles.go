package reading

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The facts of a target's units that the helper question and the grouping
// ask with (grouping.go): each unit's calls, users and registrations, from
// code structure only, never documentation. A box of our map is a
// responsibility; one file can hold the code of several boxes and one box
// can span several files.

// roleUnit is one unit of a file of the target that is neither test nor
// generated code, as the role split's requests show it: code structure only,
// never documentation.
type roleUnit struct {
	id                    string
	name, kind, signature string
	// macro says the unit is a macro (atlas Decl.Macro), a declaration kind
	// of its own for what its adapter records of its uses.
	macro bool
	// file and path are the unit's file place and its path.
	file, path string
	// lines is the unit's weight in code lines; zero is unknown and is not
	// sent.
	lines   int
	methods []string
	// calls and calledBy name units of the same file; elsewhere names what
	// it calls in other files as "path:name".
	calls, calledBy, elsewhere []string
	// callers are the distinct units of the target's other non-test files
	// that call it.
	callers map[string]bool
	// users are the units of the target's non-test, non-generated files that
	// call it, are decorated by it, read it or, for a type, take it as a
	// parameter; uses are the units it calls, is decorated by, reads or
	// takes. A hand-over is neither: a command table or a
	// route registrar hands its handlers over without using them, and a read
	// of a callable is a function value taken to be called later. Nor is the
	// other half of a hand-over, a call through the function value stored
	// there (ProgramIndex `function_value` dispatch): adlist.c's listDup
	// calls through the dup field createClient stored dupClientReplyValue
	// in, and is no user of it.
	users, uses map[string]bool
	// registered are the words of each registration handing it, or one of
	// its followers, over to be called later ("redisCommand get"): how the
	// code calls a handler no declaration calls by name.
	registered []string
	// callees, calledByAll, readers and handers are the units of the target's
	// non-test, non-generated files it calls and that call it (decorations
	// included), read it or hand it over, exactly: the helper question's
	// item. used says whether any declaration of those files uses it at all
	// (calls, decorations, hand-overs and reads, exact or among
	// alternatives, or a registration).
	callees, calledByAll, readers, handers map[string]bool
	used                                   bool
	// seed says the target's execution begins at this unit: it is never
	// asked the helper question or assigned a box, and a split file gives
	// it a row of its own.
	seed bool
}

// roleFile is one unit-bearing file of the target that is neither test nor
// generated code. A candidate of the gate holds at least two units, since one
// unit cannot go in two boxes. No size decides it.
type roleFile struct {
	file  *designFile
	units []*roleUnit
	byID  map[string]*roleUnit
}

// roleBox is one box the naming gave a file.
type roleBox struct {
	Name  string `json:"name"`
	Holds string `json:"holds"`
}

// roleFacts are the units of a target's files that are neither test nor
// generated code, by file in f* order, with what the role split's requests
// show of them.
type roleFacts struct {
	files  []*roleFile
	byFile map[string]*roleFile
	units  map[string]*roleUnit
}

// unitFacts reads every unit of the target's files that are neither test nor
// generated code, in f* order, with its calls, users and registrations.
func (r *reader) unitFacts(view *designView) *roleFacts {
	facts := &roleFacts{byFile: map[string]*roleFile{}, units: map[string]*roleUnit{}}
	for _, file := range view.files {
		if file.test || file.generated {
			continue
		}
		roles := &roleFile{file: file, byID: map[string]*roleUnit{}}
		for _, id := range file.units {
			decl := r.places[id].Symbol.Decl
			_, seed := view.seedName[id]
			unit := &roleUnit{id: id, name: decl.Name, kind: decl.Kind, macro: decl.Macro, signature: decl.Signature, file: file.id, path: file.path, seed: seed,
				callers: map[string]bool{}, users: map[string]bool{}, uses: map[string]bool{},
				callees: map[string]bool{}, calledByAll: map[string]bool{}, readers: map[string]bool{}, handers: map[string]bool{}}
			roles.units = append(roles.units, unit)
			roles.byID[id] = unit
			facts.units[id] = unit
		}
		facts.files = append(facts.files, roles)
		facts.byFile[file.id] = roles
	}
	if len(facts.files) == 0 {
		return facts
	}
	byFile := facts.byFile
	// unitOf is the unit a declaration's membership comes from.
	unitOf := func(id string) string {
		if root := view.root(id); view.unitFile[root] != "" {
			return root
		}
		return ""
	}
	followers := map[string][]string{}
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			if unit := unitOf(id); unit != "" && unit != id {
				followers[unit] = append(followers[unit], id)
			}
		}
	}
	for _, file := range facts.files {
		for _, unit := range file.units {
			unit.lines = r.unitLines(unit.id, followers[unit.id])
			for _, follower := range followers[unit.id] {
				decl := r.places[follower].Symbol.Decl
				if decl.Kind != "method" {
					continue
				}
				short := decl.Name
				if _, after, ok := strings.Cut(short, "."); ok {
					short = after
				}
				unit.methods = appendUnique(unit.methods, strings.TrimSpace(short+" "+decl.Signature))
			}
		}
	}
	// A registration that hands a unit or one of its followers over, such as
	// a command table row or a route, carries the words the code wrote there.
	for _, place := range r.opts.Graph.Places {
		boundary := place.Boundary
		if boundary == nil || boundary.Registrar != nil || boundary.Direction != atlas.DirectionIn || len(boundary.Words) == 0 || !contains(place.TargetIDs, view.targetID) {
			continue
		}
		subject := boundary.SubjectID
		if subject == "" {
			subject = r.designSubjects[boundary.ObjectID]
		}
		if unit := facts.units[unitOf(subject)]; unit != nil {
			unit.registered = appendUnique(unit.registered, strings.Join(boundary.Words, " "))
			unit.used = true
		}
	}
	// Calls are exact call sites; a decoration applies its decorator, so it
	// counts as a caller of it.
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			symbol := r.places[id].Symbol
			from := unitOf(id)
			if symbol == nil || from == "" {
				continue
			}
			fromFile := byFile[view.unitFile[from]]
			fromRow := view.byID[view.unitFile[from]]
			fromUnit := facts.units[from]
			// A unit's users and uses: its exact calls, decorations and reads
			// of what does not run, between units of files that are neither
			// test nor generated code.
			use := func(callee string) {
				to := unitOf(callee)
				if to == "" || to == from || fromUnit == nil || facts.units[to] == nil {
					return
				}
				fromUnit.uses[to] = true
				facts.units[to].users[from] = true
			}
			for _, used := range symbol.Uses {
				place, ok := r.places[used.PlaceID]
				if !ok || place.Symbol == nil || !contains(place.TargetIDs, view.targetID) {
					continue
				}
				// A callable taking a type as a parameter uses that type: the
				// one use of a type the facts record.
				if used.Resolution == "exact" && (used.Kind == "decorates" || used.Kind == atlas.UseTakes || used.Kind == "reads" && !programindex.ObjectKind(place.Symbol.Decl.Kind).Callable()) {
					use(used.PlaceID)
				}
				to := unitOf(used.PlaceID)
				toUnit := facts.units[to]
				if fromUnit == nil || toUnit == nil || to == from || used.Resolution != "exact" && used.Resolution != "alternatives" {
					continue
				}
				switch used.Kind {
				case "decorates", "passes_callback", "reads":
					toUnit.used = true
				}
				if used.Resolution != "exact" {
					continue
				}
				switch used.Kind {
				case "reads":
					toUnit.readers[from] = true
				case "passes_callback":
					toUnit.handers[from] = true
				}
			}
			for _, call := range symbol.Calls {
				if !contains(callTargets(r.places[id], call), view.targetID) {
					continue
				}
				if call.Kind != "calls" && call.Kind != "decorates" {
					continue
				}
				if call.Resolution == "alternatives" {
					// A call that may reach any of several declarations makes each
					// of them possibly used.
					for _, callee := range call.CalleeIDs {
						if place, ok := r.places[callee]; ok && place.Symbol != nil && contains(place.TargetIDs, view.targetID) {
							if to := unitOf(callee); fromUnit != nil && facts.units[to] != nil && to != from {
								facts.units[to].used = true
							}
						}
					}
					continue
				}
				if call.Resolution != "exact" {
					continue
				}
				for _, callee := range call.CalleeIDs {
					place, ok := r.places[callee]
					if !ok || place.Symbol == nil || !contains(place.TargetIDs, view.targetID) {
						continue
					}
					// A call through a stored function value uses the function,
					// but its caller is no user: the call is the other half of
					// the hand-over that stored it.
					if call.Dispatch != programindex.DispatchFunctionValue {
						use(callee)
					}
					to := unitOf(callee)
					if toUnit := facts.units[to]; fromUnit != nil && toUnit != nil && to != from {
						fromUnit.callees[to] = true
						toUnit.calledByAll[from] = true
						toUnit.used = true
					}
					var toFile *roleFile
					if to != "" {
						toFile = byFile[view.unitFile[to]]
					}
					if fromFile != nil && toFile == fromFile {
						if to != from {
							if call.Kind == "calls" {
								fromFile.byID[from].calls = appendUnique(fromFile.byID[from].calls, toFile.byID[to].name)
							}
							toFile.byID[to].calledBy = appendUnique(toFile.byID[to].calledBy, fromFile.byID[from].name)
						}
						continue
					}
					if fromFile != nil && call.Kind == "calls" {
						fromFile.byID[from].elsewhere = appendUnique(fromFile.byID[from].elsewhere, place.Path+":"+place.Symbol.Decl.Name)
					}
					if toFile != nil && fromRow != nil && !fromRow.test {
						toFile.byID[to].callers[from] = true
					}
				}
			}
		}
	}
	return facts
}

// unitLines is a unit's weight: its own code lines, its overloads' (each
// written before it, outside its range), and those of every follower whose
// source lies outside its range (a Go method, a repeated name), never a
// lexical child or a class's own methods, which its range already counts.
// A module body counts its file's code lines less those of the file's other
// top-level declarations and their overloads. Zero is unknown.
func (r *reader) unitLines(unit string, followers []string) int {
	place := r.places[unit]
	decl := place.Symbol.Decl
	if decl.CodeLines == 0 {
		return 0
	}
	if decl.Kind == "module" {
		file := r.places[place.Parent]
		if file.File == nil {
			return 0
		}
		lines := decl.CodeLines
		for i, other := range file.File.Decls {
			if other.Kind != "module" && !insideAnother(file.File.Decls, i) {
				lines -= other.CodeLines + overloadLines(other)
			}
		}
		return max(lines, 0)
	}
	lines := decl.CodeLines + overloadLines(decl)
	for _, id := range followers {
		follower := r.places[id]
		if follower.Path == place.Path && within(follower.Symbol.Decl, decl) {
			continue
		}
		lines += follower.Symbol.Decl.CodeLines
	}
	return lines
}

// overloadLines are the code lines of a declaration's overloads.
func overloadLines(decl atlas.Decl) int {
	lines := 0
	for _, overload := range decl.Overloads {
		lines += overload.CodeLines
	}
	return lines
}

// within says whether inner's source lies inside outer's known range.
func within(inner, outer atlas.Decl) bool {
	end := max(inner.EndLine, inner.LineNo)
	return outer.EndLine > 0 && outer.LineNo <= inner.LineNo && end <= outer.EndLine
}

// insideAnother says whether a declaration's source lies inside another
// declaration of its file; of two with the same range the earlier holds the
// later.
func insideAnother(decls []atlas.Decl, at int) bool {
	for i, other := range decls {
		if i == at || other.Kind == "module" || !within(decls[at], other) {
			continue
		}
		if other.LineNo != decls[at].LineNo || other.EndLine != decls[at].EndLine || i < at {
			return true
		}
	}
	return false
}

// boxesAnswer is a decoded naming answer: the boxes, each once, and what
// the decoder noted without refusing the answer.
type boxesAnswer struct {
	Boxes []roleBox `json:"boxes"`
	// Incomplete names the boxes given without a name or without holds;
	// the file then stays whole, since the list is one partition decision.
	Incomplete []string `json:"-"`
	// Repeated names boxes given twice under one name with different holds;
	// both are kept.
	Repeated []string `json:"-"`
}

// decodeBoxes reads {"boxes":[{"name","holds"}]}. A wrapper object around
// it, keys in another case, whitespace and an identical repeated box are
// forms, not refusals; null is no boxes. An answer that is not JSON or holds
// no list of boxes is refused. Nothing bounds the count.
func decodeBoxes(raw []byte) (boxesAnswer, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return boxesAnswer{}, fmt.Errorf("boxes: the answer is not JSON")
	}
	list, ok := boxesList(value, 2)
	if !ok {
		return boxesAnswer{}, fmt.Errorf("boxes: the answer has no list of boxes")
	}
	var answer boxesAnswer
	seen := map[string]string{} // lower-cased name -> holds of the first box
	for position, element := range list {
		fields, _ := element.(map[string]any)
		name, holds := cleanText(textField(fields, "name")), cleanText(textField(fields, "holds"))
		if name == "" || holds == "" {
			answer.Incomplete = append(answer.Incomplete, fmt.Sprintf("box %d", position+1))
			continue
		}
		key := strings.ToLower(name)
		if first, known := seen[key]; known {
			if first == holds {
				continue
			}
			answer.Repeated = appendUnique(answer.Repeated, name)
		} else {
			seen[key] = holds
		}
		answer.Boxes = append(answer.Boxes, roleBox{Name: name, Holds: holds})
	}
	return answer, nil
}

// boxesList finds the boxes list: the value itself, its "boxes" field in
// any case, or the same inside one wrapper object.
func boxesList(value any, depth int) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case map[string]any:
		for key, field := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "boxes") {
				if field == nil {
					return nil, true
				}
				list, ok := field.([]any)
				return list, ok
			}
		}
		if len(typed) == 1 && depth > 0 {
			for _, field := range typed {
				if wrapped, ok := field.(map[string]any); ok {
					return boxesList(wrapped, depth-1)
				}
			}
		}
	}
	return nil, false
}

func textField(fields map[string]any, name string) string {
	for key, value := range fields {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			text, _ := value.(string)
			return text
		}
	}
	return ""
}

// boxChoice reads an answer's box ref: its index among boxes, or -1.
func boxChoice(answer table.Answer, boxes []roleBox) int {
	if answer == nil {
		return -1
	}
	choice := answer["box"]
	var index int
	if _, err := fmt.Sscanf(choice, "b%d", &index); err != nil || index < 1 || index > len(boxes) || choice != fmt.Sprintf("b%d", index) {
		return -1
	}
	return index - 1
}

// localRows says whether a table's row IDs are request-local refs (f1,
// d1…dn) rather than places, as in the role split: a warm cache keeps them
// when a file is added or edited earlier in path order.
func localRows(def table.Definition) bool {
	return def.Stage == lines.StageRoleHelper || def.Stage == lines.StageGroupEnough || def.Stage == lines.StageGroupAssign
}
