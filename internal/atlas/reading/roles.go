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
)

// The role split (owner's option "в", 2026-09-26): "what's in one file can
// have different roles, and one role can span different files. We build our
// own map, we group and abstract." A file the parts answer placed whole can
// hold the code of several boxes of our map. Jev decides whether it does
// (the gate), DeepSeek names the boxes its code goes in (the naming), and
// Jev puts each of its units in one of them (the assignment). Each box that
// holds a unit becomes a part of its own.

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
			unit := &roleUnit{id: id, name: decl.Name, kind: decl.Kind, signature: decl.Signature, callers: map[string]bool{}}
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
			for _, call := range symbol.Calls {
				if call.Resolution != "exact" || call.Kind != "calls" && call.Kind != "decorates" {
					continue
				}
				for _, callee := range call.CalleeIDs {
					place, ok := r.places[callee]
					if !ok || place.Symbol == nil || !contains(place.TargetIDs, view.targetID) {
						continue
					}
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
	catalogue := make([]map[string]any, len(boxes))
	for i, box := range boxes {
		catalogue[i] = map[string]any{"ref": fmt.Sprintf("b%d", i+1), "title": box.Name, "holds": box.Holds}
	}
	group := rowGroup{shared: []table.Field{{Name: "file", Value: file.file.path}, {Name: "boxes", Value: catalogue}}}
	for i, unit := range file.units {
		item := []table.Field{{Name: "declaration", Value: unit.name}, {Name: "kind", Value: unit.kind}}
		if unit.signature != "" {
			item = append(item, table.Field{Name: "signature", Value: unit.signature})
		}
		for _, list := range []struct {
			name  string
			value []string
		}{{"methods", unit.methods}, {"calls", unit.calls}, {"called_by", unit.calledBy}, {"calls_elsewhere", unit.elsewhere}} {
			if len(list.value) > 0 {
				item = append(item, table.Field{Name: list.name, Value: list.value})
			}
		}
		group.rows = append(group.rows, table.Row{ID: fmt.Sprintf("d%d", i+1), Fields: item})
	}
	return group
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
	splits := map[string]*roleSplit{}
	at := 0
	for _, candidate := range files {
		boxes := named[candidate.file.id]
		split := &roleSplit{file: candidate.file, boxes: boxes, holds: make([][]string, len(boxes))}
		for _, unit := range candidate.units {
			choice := ""
			if answer := assigned[at].answer; answer != nil {
				choice = answer["box"]
			}
			at++
			var index int
			if _, err := fmt.Sscanf(choice, "b%d", &index); err != nil || index < 1 || index > len(boxes) || choice != fmt.Sprintf("b%d", index) {
				split.undecided = append(split.undecided, unit.id)
				continue
			}
			split.holds[index-1] = append(split.holds[index-1], unit.id)
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
