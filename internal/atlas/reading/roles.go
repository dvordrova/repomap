package reading

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The role split (owner's option "в", 2026-09-26): "what's in one file can
// have different roles, and one role can span different files. We build our
// own map, we group and abstract." A file the parts answer placed whole can
// hold the code of several boxes of our map. Jev decides whether it does
// (the gate), DeepSeek names the boxes its code goes in (the naming), and
// Jev puts each of its units in one of them (the assignment); a unit it
// leaves open goes, by code, where the file's code that uses it went. Each
// box that holds a unit becomes a part of its own.

//go:embed prompts/design_boxes.md
var designBoxesPrompt string

const designBoxesTask = "repomap.atlas.file_boxes.v1"

// roleUnit is one unit of a candidate file as the role split's requests
// show it: code structure only, never documentation.
type roleUnit struct {
	id                    string
	name, kind, signature string
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
	// call it, are decorated by it or read it; uses are the units it calls,
	// is decorated by or reads. A hand-over is neither: a command table or a
	// route registrar hands its handlers over without using them, and a read
	// of a callable is a function value taken to be called later.
	users, uses map[string]bool
	// registered are the words of each registration handing it, or one of
	// its followers, over to be called later ("redisCommand get"): how the
	// code calls a handler no declaration calls by name.
	registered []string
}

// roleFile is one candidate of the role split: a unit-bearing file of the
// target that is not test or generated code and holds at least two units,
// since one unit cannot go in two boxes. No size decides it.
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

// roleSplit is one file whose code goes in several boxes: the boxes in
// naming order, the units each holds, and the units no box took.
type roleSplit struct {
	file      *designFile
	boxes     []roleBox
	holds     [][]string
	undecided []string
}

// roleFiles lists the target's candidates in f* order with what their
// requests show.
func (r *reader) roleFiles(view *designView) []*roleFile {
	var candidates []*roleFile
	byFile := map[string]*roleFile{}
	for _, file := range view.files {
		if file.test || file.generated || len(file.units) < 2 {
			continue
		}
		candidate := &roleFile{file: file, byID: map[string]*roleUnit{}}
		for _, id := range file.units {
			decl := r.places[id].Symbol.Decl
			unit := &roleUnit{id: id, name: decl.Name, kind: decl.Kind, signature: decl.Signature, callers: map[string]bool{}, users: map[string]bool{}, uses: map[string]bool{}}
			candidate.units = append(candidate.units, unit)
			candidate.byID[id] = unit
		}
		candidates = append(candidates, candidate)
		byFile[file.id] = candidate
	}
	if len(candidates) == 0 {
		return nil
	}
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
	for _, candidate := range candidates {
		for _, unit := range candidate.units {
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
		if boundary == nil || boundary.Direction != atlas.DirectionIn || len(boundary.Words) == 0 || !contains(place.TargetIDs, view.targetID) {
			continue
		}
		subject := boundary.SubjectID
		if subject == "" {
			subject = r.designSubjects[boundary.ObjectID]
		}
		unit := unitOf(subject)
		if candidate := byFile[view.unitFile[unit]]; unit != "" && candidate != nil {
			candidate.byID[unit].registered = appendUnique(candidate.byID[unit].registered, strings.Join(boundary.Words, " "))
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
			// A unit's users and uses: its exact calls, decorations and reads
			// of what does not run, between units of files that are neither
			// test nor generated code.
			use := func(callee string) {
				to := unitOf(callee)
				toRow := view.byID[view.unitFile[to]]
				if to == "" || to == from || fromRow == nil || toRow == nil || fromRow.test || fromRow.generated || toRow.test || toRow.generated {
					return
				}
				if fromFile != nil {
					fromFile.byID[from].uses[to] = true
				}
				if toFile := byFile[view.unitFile[to]]; toFile != nil {
					toFile.byID[to].users[from] = true
				}
			}
			for _, used := range symbol.Uses {
				place, ok := r.places[used.PlaceID]
				if used.Resolution != "exact" || !ok || place.Symbol == nil || !contains(place.TargetIDs, view.targetID) {
					continue
				}
				if used.Kind == "decorates" || used.Kind == "reads" && !programindex.ObjectKind(place.Symbol.Decl.Kind).Callable() {
					use(used.PlaceID)
				}
			}
			for _, call := range symbol.Calls {
				if call.Resolution != "exact" || call.Kind != "calls" && call.Kind != "decorates" {
					continue
				}
				for _, callee := range call.CalleeIDs {
					place, ok := r.places[callee]
					if !ok || place.Symbol == nil || !contains(place.TargetIDs, view.targetID) {
						continue
					}
					use(callee)
					to := unitOf(callee)
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
	return candidates
}

// unitLines is a unit's weight: its own code lines and those of every
// follower whose source lies outside its range (a Go method, a repeated
// name), never a lexical child or a class's own methods, which its range
// already counts. A module body counts its file's code lines less those of
// the file's other top-level declarations. Zero is unknown.
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
				lines -= other.CodeLines
			}
		}
		return max(lines, 0)
	}
	lines := decl.CodeLines
	for _, id := range followers {
		follower := r.places[id]
		if follower.Path == place.Path && within(follower.Symbol.Decl, decl) {
			continue
		}
		lines += follower.Symbol.Decl.CodeLines
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

// gateRow is a candidate as the gate asks about it: its path and its
// declarations with their kind, name, signature, methods and same-file
// calls. The row is f1 in its own request.
func (file *roleFile) gateRow() table.Row {
	declarations := make([]map[string]any, 0, len(file.units))
	for _, unit := range file.units {
		item := map[string]any{"name": unit.name, "kind": unit.kind}
		if unit.signature != "" {
			item["signature"] = unit.signature
		}
		if len(unit.methods) > 0 {
			item["methods"] = unit.methods
		}
		if len(unit.calls) > 0 {
			item["calls"] = unit.calls
		}
		declarations = append(declarations, item)
	}
	return table.Row{ID: "f1", Fields: []table.Field{{Name: "path", Value: file.file.path}, {Name: "declarations", Value: declarations}}}
}

// boxesDecl is one declaration as the naming shows it; empty fields are
// left out.
type boxesDecl struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Signature        string   `json:"signature,omitempty"`
	Lines            int      `json:"lines,omitempty"`
	Methods          []string `json:"methods,omitempty"`
	Calls            []string `json:"calls,omitempty"`
	CalledBy         []string `json:"called_by,omitempty"`
	CallersElsewhere int      `json:"callers_elsewhere,omitempty"`
}

type boxesInput struct {
	Task string `json:"task"`
	File struct {
		Path         string      `json:"path"`
		Declarations []boxesDecl `json:"declarations"`
	} `json:"file"`
}

func (file *roleFile) boxesInput() boxesInput {
	input := boxesInput{Task: designBoxesTask}
	input.File.Path = file.file.path
	for _, unit := range file.units {
		input.File.Declarations = append(input.File.Declarations, boxesDecl{
			Name: unit.name, Kind: unit.kind, Signature: unit.signature, Lines: unit.lines, Methods: unit.methods,
			Calls: unit.calls, CalledBy: unit.calledBy, CallersElsewhere: len(unit.callers),
		})
	}
	return input
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

// designBoxesCall names the boxes one file's code goes in: every unit of the
// file, whole, in one request.
func designBoxesCall(input boxesInput) (llm.Call[boxesAnswer], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[boxesAnswer]{}, err
	}
	return llm.Call[boxesAnswer]{
		State: []byte(designBoxesTask),
		Prompt: llm.Prompt{System: lines.RoleMap + designBoxesPrompt, User: string(raw), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"boxes":[{"name":"Move search","holds":"The search over candidate moves and its scoring."},{"name":"Board state","holds":"The board, its pieces and applying a move to it."}]}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designOutputTokens(len(input.File.Declarations))},
		DecodeValidate: decodeBoxes,
	}, nil
}

// assignGroup is one named file's assignment: the file and its boxes as the
// context, one row per unit, d1…dn in the file's unit order.
func (file *roleFile) assignGroup(boxes []roleBox) rowGroup {
	group := rowGroup{shared: boxesContext(file.file.path, boxes)}
	for i, unit := range file.units {
		group.rows = append(group.rows, table.Row{ID: fmt.Sprintf("d%d", i+1), Fields: unit.assignItem()})
	}
	return group
}

func boxesContext(path string, boxes []roleBox) []table.Field {
	catalogue := make([]map[string]any, len(boxes))
	for i, box := range boxes {
		catalogue[i] = map[string]any{"ref": fmt.Sprintf("b%d", i+1), "title": box.Name, "holds": box.Holds}
	}
	return []table.Field{{Name: "file", Value: path}, {Name: "boxes", Value: catalogue}}
}

// assignItem is a unit as the assignment asks about it: its name, kind,
// signature, methods, same-file calls and callers, calls elsewhere and the
// words of its registrations.
func (unit *roleUnit) assignItem() []table.Field {
	item := []table.Field{{Name: "declaration", Value: unit.name}, {Name: "kind", Value: unit.kind}}
	if unit.signature != "" {
		item = append(item, table.Field{Name: "signature", Value: unit.signature})
	}
	if len(unit.methods) > 0 {
		item = append(item, table.Field{Name: "methods", Value: unit.methods})
	}
	if len(unit.calls) > 0 {
		item = append(item, table.Field{Name: "calls", Value: unit.calls})
	}
	if len(unit.calledBy) > 0 {
		item = append(item, table.Field{Name: "called_by", Value: unit.calledBy})
	}
	if len(unit.elsewhere) > 0 {
		item = append(item, table.Field{Name: "calls_elsewhere", Value: unit.elsewhere})
	}
	if len(unit.registered) > 0 {
		item = append(item, table.Field{Name: "registered", Value: unit.registered})
	}
	return item
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

// readRoles runs the role split for a target's candidates: the gate for
// each, the naming for those whose code goes in several boxes, and the
// assignment for those named. It returns, by file, each split a file ends
// with. A file keeps today's map, recorded, when the gate is not "several
// boxes" with a clear lead, the naming is refused, incomplete or gives fewer
// than two boxes, or fewer than two boxes end up holding a unit. It never
// fails the target; only a canceled context stops it.
func (r *reader) readRoles(ctx context.Context, view *designView, round int) (map[string]*roleSplit, error) {
	candidates := r.roleFiles(view)
	if len(candidates) == 0 {
		return nil, nil
	}
	target := r.opts.Targets[round-1]
	r.opts.Stage(lines.StageRoleGate, fmt.Sprintf("%s: asking whether %d files each go in one box of the map or in several", target.Name, len(candidates)))
	gates := make(rowGroups, len(candidates))
	for i, candidate := range candidates {
		gates[i] = rowGroup{rows: []table.Row{candidate.gateRow()}}
	}
	answers, err := r.runTableGroups(ctx, lines.RoleGate(), round, gates, nil)
	if err != nil {
		return nil, err
	}
	var several []*roleFile
	for i, candidate := range candidates {
		if answers[i].answer["boxes"] == lines.RoleSeveralBoxes {
			several = append(several, candidate)
		}
	}
	fmt.Fprintf(&r.tables, "role gate: %d of %d files go in several boxes\n\n", len(several), len(candidates))
	if len(several) == 0 {
		return nil, nil
	}
	named, err := r.nameBoxes(ctx, view, round, several)
	if err != nil {
		return nil, err
	}
	var files []*roleFile
	var groups rowGroups
	for _, candidate := range several {
		if boxes := named[candidate.file.id]; len(boxes) >= 2 {
			files = append(files, candidate)
			groups = append(groups, candidate.assignGroup(boxes))
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	r.opts.Stage(lines.StageRoleAssign, fmt.Sprintf("%s: putting the declarations of %d files in their boxes", target.Name, len(files)))
	assigned, err := r.runTableGroups(ctx, lines.RoleAssign(), round, groups, nil)
	if err != nil {
		return nil, err
	}
	// chosen is, by file and unit, the index of the box the unit goes in, or
	// -1 while none leads by the margin.
	chosen := make([][]int, len(files))
	at := 0
	for i, candidate := range files {
		chosen[i] = make([]int, len(candidate.units))
		for j := range candidate.units {
			chosen[i][j] = boxChoice(assigned[at].answer, named[candidate.file.id])
			at++
		}
	}
	r.settleOpen(view.targetID, files, chosen)
	splits := map[string]*roleSplit{}
	for f, candidate := range files {
		boxes := named[candidate.file.id]
		split := &roleSplit{file: candidate.file, boxes: boxes, holds: make([][]string, len(boxes))}
		for j, unit := range candidate.units {
			if chosen[f][j] < 0 {
				split.undecided = append(split.undecided, unit.id)
				continue
			}
			split.holds[chosen[f][j]] = append(split.holds[chosen[f][j]], unit.id)
		}
		holding := 0
		var empty []string
		for i, units := range split.holds {
			if len(units) > 0 {
				holding++
			} else {
				empty = append(empty, boxes[i].Name)
			}
		}
		path := candidate.file.path
		if holding < 2 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: view.targetID, Kind: "role_not_split", Count: 1, Samples: []string{path},
				Reason: fmt.Sprintf("%d of %d boxes hold a declaration; the file stays whole", holding, len(boxes))})
			fmt.Fprintf(&r.tables, "%s: %d of %d boxes hold a declaration; the file stays whole\n\n", path, holding, len(boxes))
			continue
		}
		if len(empty) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: view.targetID, Kind: "role_box_empty", Count: len(empty), Samples: empty,
				Reason: path + ": named boxes no declaration went in; not drawn"})
		}
		if len(split.undecided) > 0 {
			var names []string
			for _, id := range split.undecided {
				names = append(names, candidate.byID[id].name)
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: view.targetID, Kind: "role_undecided", Count: len(names), Samples: names,
				Reason: path + ": declarations no box took; off the map as undecided"})
		}
		fmt.Fprintf(&r.tables, "%s: split into %d boxes, %d undecided, %d named boxes empty\n", path, holding, len(split.undecided), len(empty))
		for i, box := range boxes {
			var names []string
			for _, id := range split.holds[i] {
				names = append(names, candidate.byID[id].name)
			}
			fmt.Fprintf(&r.tables, "- %s (%d): %s\n", box.Name, len(names), strings.Join(names, " "))
		}
		r.tables.WriteString("\n")
		splits[candidate.file.id] = split
	}
	return splits, nil
}

// settleOpen places, by code, each unit the assignment left open: it takes
// box k when every unit of its file that uses it (calls it, is decorated by
// it or reads it; never one that only hands it over) has a box and that box
// is k; a unit no unit of its file uses takes k when everything of its file
// it uses has box k. Anything else stays open, and so undecided. It runs to
// a fixed point: a unit placed may settle its neighbours. Where a unit's
// users are is a code fact, so the question is not asked again (owner,
// 2026-09-28). Every unit it places is recorded.
func (r *reader) settleOpen(targetID string, files []*roleFile, chosen [][]int) {
	byUsers, byUses := map[int][]string{}, map[int][]string{}
	// common is the one box every unit of ids has, or -1 when one has none,
	// two differ or ids names none of the file's units.
	common := func(f int, ids map[string]bool) int {
		box, seen := -1, false
		for j, unit := range files[f].units {
			if !ids[unit.id] {
				continue
			}
			if chosen[f][j] < 0 || seen && chosen[f][j] != box {
				return -1
			}
			box, seen = chosen[f][j], true
		}
		return box
	}
	sameFile := func(f int, ids map[string]bool) bool {
		for id := range ids {
			if files[f].byID[id] != nil {
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for f, file := range files {
			for j, unit := range file.units {
				if chosen[f][j] >= 0 {
					continue
				}
				if sameFile(f, unit.users) {
					if box := common(f, unit.users); box >= 0 {
						chosen[f][j], changed = box, true
						byUsers[f] = append(byUsers[f], unit.name)
					}
					continue
				}
				if box := common(f, unit.uses); box >= 0 {
					chosen[f][j], changed = box, true
					byUses[f] = append(byUses[f], unit.name)
				}
			}
		}
	}
	for f, file := range files {
		path := file.file.path
		if names := byUsers[f]; len(names) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: targetID, Kind: "role_placed_by_users", Count: len(names), Samples: names,
				Reason: path + ": declarations the assignment left open, placed in the one box of the file's declarations that use them"})
			fmt.Fprintf(&r.tables, "%s: placed by their users: %s\n\n", path, strings.Join(names, " "))
		}
		if names := byUses[f]; len(names) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: targetID, Kind: "role_placed_by_uses", Count: len(names), Samples: names,
				Reason: path + ": declarations the assignment left open that no declaration of the file uses, placed in the one box of what they use"})
			fmt.Fprintf(&r.tables, "%s: placed by what they use: %s\n\n", path, strings.Join(names, " "))
		}
	}
}

// nameBoxes asks the naming for each file at once and returns, by file,
// the boxes of every answer that names at least two complete boxes.
func (r *reader) nameBoxes(ctx context.Context, view *designView, round int, files []*roleFile) (map[string][]roleBox, error) {
	r.started[lines.StageRoleBoxes] = time.Now()
	target := r.opts.Targets[round-1]
	r.opts.Stage(lines.StageRoleBoxes, fmt.Sprintf("%s: naming the boxes the code of %d files goes in", target.Name, len(files)))
	calls := make([]llm.Call[boxesAnswer], len(files))
	for i, file := range files {
		call, err := designBoxesCall(file.boxesInput())
		if err != nil {
			return nil, err
		}
		calls[i] = call
	}
	results := llm.ExecuteJSONEach(ctx, debugdump.BindStage(r.opts.Executor, lines.StageRoleBoxes), r.opts.Provider, calls)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	use := r.use(lines.StageRoleBoxes)
	named := map[string][]roleBox{}
	for i, result := range results {
		file := files[i].file
		window := table.Window{Stage: lines.StageRoleBoxes, Round: round, Index: i}
		use.Windows++
		use.Rows++
		if result.Outcome.Cached {
			use.Cached++
		} else {
			use.Live++
		}
		responseRef := path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
		fmt.Fprintf(&r.tables, "## %s · round %d · window %d · %s\n\n", lines.StageRoleBoxes, round, i, path.Join(atlas.TablesDir, r.windowFileName(window, "request.ref.json")))
		note := func(kind, reason string, samples []string) {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleBoxes, Target: view.targetID, Kind: kind, Count: max(1, len(samples)), Samples: samples, Reason: file.path + ": " + reason, ResponseRef: responseRef})
			fmt.Fprintf(&r.tables, "%s: %s %s\n", file.path, reason, strings.Join(samples, ", "))
		}
		answer := result.Outcome.Value
		recorded := len(r.rejected)
		switch {
		case result.Err != nil:
			if len(result.Outcome.Response) == 0 {
				responseRef = ""
			}
			use.Rejected++
			use.Given++
			note("window_rejected", "boxes answer refused, the file stays whole: "+result.Err.Error(), nil)
		case len(answer.Incomplete) > 0:
			note("role_boxes_incomplete", "boxes without a name or holds; the file stays whole", answer.Incomplete)
		case len(answer.Boxes) < 2:
			note("role_one_box", fmt.Sprintf("%d boxes named; the file stays whole", len(answer.Boxes)), nil)
		default:
			if len(answer.Repeated) > 0 {
				note("role_repeated_name", "names given to two boxes, each kept", answer.Repeated)
			}
			named[file.id] = answer.Boxes
			for _, box := range answer.Boxes {
				fmt.Fprintf(&r.tables, "- %s: %s\n", box.Name, box.Holds)
			}
		}
		r.tables.WriteString("\n")
		if err := r.writeWindowExchange(window, []byte(calls[i].Prompt.System), []byte(calls[i].Prompt.User), result.Outcome.Request, result.Outcome.Response, result.Err != nil || len(r.rejected) > recorded); err != nil {
			return nil, err
		}
		if result.Err == nil {
			raw, err := json.MarshalIndent(answer, "", "  ")
			if err != nil {
				return nil, err
			}
			if err := r.writeWindowFile(window, "result.json", raw); err != nil {
				return nil, err
			}
		}
	}
	return named, nil
}

// localRows says whether a table's row IDs are request-local refs (f1,
// d1…dn) rather than places, as in the role split: a warm cache keeps them
// when a file is added or edited earlier in path order.
func localRows(def table.Definition) bool {
	return def.Stage == lines.StageRoleGate || def.Stage == lines.StageRoleAssign
}
