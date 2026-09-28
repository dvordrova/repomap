package reading

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
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
// own map, we group and abstract." A file can hold the code of several
// boxes of our map. Before the parts request, Jev decides which declarations
// are helpers (the helper question) and whether a file's code goes in several
// boxes (the gate), DeepSeek names the boxes of such a file from its
// declarations that are no helpers (the naming), and Jev puts each of those
// in one of them (the assignment). Code then places helpers with their users
// and a unit the assignment leaves open where the file's code that uses it
// went; only a helper its users share between boxes, or one nothing uses, is
// asked once more (the second pass). Each box that holds a unit is then one
// unit of the grouping, a c* row of the parts request.

//go:embed prompts/design_boxes.md
var designBoxesPrompt string

const designBoxesTask = "repomap.atlas.file_boxes.v1"

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

// roleSplit is one file whose code goes in several boxes: the boxes in
// naming order, the file's own units each holds, and the units no box took:
// undecided, those a question asked without a decision (a near-tie, an
// unanswered row, a refused window), and blocked, the helpers never asked
// because a unit that uses them never got a row.
type roleSplit struct {
	file               *designFile
	boxes              []roleBox
	holds              [][]string
	undecided, blocked []string
}

// roleFacts are the units of a target's files that are neither test nor
// generated code, by file in f* order, with what the role split's requests
// show of them.
type roleFacts struct {
	files  []*roleFile
	byFile map[string]*roleFile
	units  map[string]*roleUnit
}

// candidates are the files the gate asks about: those of two units or more.
func (facts *roleFacts) candidates() []*roleFile {
	var candidates []*roleFile
	for _, file := range facts.files {
		if len(file.units) >= 2 {
			candidates = append(candidates, file)
		}
	}
	return candidates
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

// boxesInput is the naming's request over the file's units that are no
// helpers: a helper is never named, and its name is left out of the calls
// and callers of the others.
func (file *roleFile) boxesInput(helpers map[string]bool) boxesInput {
	input := boxesInput{Task: designBoxesTask}
	input.File.Path = file.file.path
	helperNames := map[string]bool{}
	for _, unit := range file.units {
		if helpers[unit.id] {
			helperNames[unit.name] = true
		}
	}
	without := func(names []string) []string {
		var kept []string
		for _, name := range names {
			if !helperNames[name] {
				kept = append(kept, name)
			}
		}
		return kept
	}
	for _, unit := range file.units {
		if helpers[unit.id] {
			continue
		}
		input.File.Declarations = append(input.File.Declarations, boxesDecl{
			Name: unit.name, Kind: unit.kind, Signature: unit.signature, Lines: unit.lines, Methods: unit.methods,
			Calls: without(unit.calls), CalledBy: without(unit.calledBy), CallersElsewhere: len(unit.callers),
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

// assignGroup is one named file's assignment of the asked units: the file
// and its boxes as the context, one row per asked unit keyed dN by its place
// N in the file's unit order, so a row keeps its key whichever others are
// asked.
func (file *roleFile) assignGroup(boxes []roleBox, asked func(*roleUnit) bool) rowGroup {
	group := rowGroup{shared: boxesContext(file.file.path, boxes)}
	for i, unit := range file.units {
		if asked(unit) {
			group.rows = append(group.rows, table.Row{ID: fmt.Sprintf("d%d", i+1), Fields: unit.assignItem()})
		}
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

// unitSplit is what the role split decided for a target: the files that
// split, by file; the units of split files code placed in a row of another
// file (attached) and the whole files that joined a box (attachedFiles),
// each by its grouping key; and the declarations Jev decided are helpers.
type unitSplit struct {
	splits        map[string]*roleSplit
	attached      map[string]groupKey
	attachedFiles map[string]groupKey
	helpers       map[string]bool
	// into are, by row, the units code placed there from other files.
	into map[groupKey][]string
}

// readRoles runs the role split for a target: the helper question for every
// unit and the gate for each candidate at once, the naming for those whose
// code goes in several boxes and that hold at least two units that are no
// helpers, the assignment of those units, then code: helpers go with their
// users, open units where the file's code that uses them went, and the
// helpers only their users' boxes share or nothing uses are asked once more.
// A file keeps today's map, recorded, when the gate is not "several boxes"
// with a clear lead, the naming is refused, incomplete or gives fewer than
// two boxes, or fewer than two boxes hold a unit that is no helper. It never
// fails the target; only a canceled context stops it.
func (r *reader) readRoles(ctx context.Context, view *designView, round int) (*unitSplit, error) {
	result := &unitSplit{splits: map[string]*roleSplit{}, attached: map[string]groupKey{}, attachedFiles: map[string]groupKey{}, helpers: map[string]bool{}}
	facts := r.unitFacts(view)
	if len(facts.files) == 0 {
		return result, nil
	}
	candidates := facts.candidates()
	// The helper question and the gate do not wait for each other; each is
	// read on its own view and joined helper first.
	helperView, gateView := r.view(nil), r.view(nil)
	var helpers map[string]bool
	var several []bool
	var helperErr, gateErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		helpers, helperErr = helperView.askHelpers(ctx, round, facts)
	}()
	go func() {
		defer wg.Done()
		several, gateErr = gateView.askGate(ctx, round, candidates)
	}()
	wg.Wait()
	r.joinView(helperView)
	r.joinView(gateView)
	if err := errors.Join(helperErr, gateErr); err != nil {
		return nil, err
	}
	result.helpers = helpers
	var named []*roleFile
	for i, candidate := range candidates {
		if !several[i] {
			continue
		}
		kept := 0
		for _, unit := range candidate.units {
			if !helpers[unit.id] && !unit.seed {
				kept++
			}
		}
		if kept < 2 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleBoxes, Target: view.targetID, Kind: "role_not_split", Count: 1, Samples: []string{candidate.file.path},
				Reason: fmt.Sprintf("%d of %d declarations are no helpers; the file stays whole", kept, len(candidate.units))})
			fmt.Fprintf(&r.tables, "%s: %d of %d declarations are no helpers; the file stays whole\n\n", candidate.file.path, kept, len(candidate.units))
			continue
		}
		named = append(named, candidate)
	}
	if len(named) == 0 {
		return result, nil
	}
	boxesOf, err := r.nameBoxes(ctx, view, round, named, helpers)
	if err != nil {
		return nil, err
	}
	notHelper := func(unit *roleUnit) bool { return !helpers[unit.id] && !unit.seed }
	var files []*roleFile
	var groups rowGroups
	for _, candidate := range named {
		if boxes := boxesOf[candidate.file.id]; len(boxes) >= 2 {
			files = append(files, candidate)
			groups = append(groups, candidate.assignGroup(boxes, notHelper))
		}
	}
	if len(files) == 0 {
		return result, nil
	}
	target := r.opts.Targets[round-1]
	r.opts.Stage(lines.StageRoleAssign, fmt.Sprintf("%s: putting the declarations of %d files that are no helpers in their boxes", target.Name, len(files)))
	assigned, err := r.runTableGroups(ctx, lines.RoleAssign(), round, groups, nil)
	if err != nil {
		return nil, err
	}
	place := newPlacement(facts, helpers)
	at := 0
	for _, candidate := range files {
		boxes := boxesOf[candidate.file.id]
		split := &splitFile{file: candidate, boxes: boxes, box: map[string]int{}}
		holding := map[int]bool{}
		for _, unit := range candidate.units {
			split.box[unit.id] = -1
			if helpers[unit.id] || unit.seed {
				continue
			}
			if box := boxChoice(assigned[at].answer, boxes); box >= 0 {
				split.box[unit.id] = box
				holding[box] = true
			}
			at++
		}
		// Code places an open unit only in a box that already holds one, so
		// the boxes the assignment filled decide whether the file splits.
		if len(holding) < 2 {
			path := candidate.file.path
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: view.targetID, Kind: "role_not_split", Count: 1, Samples: []string{path},
				Reason: fmt.Sprintf("%d of %d boxes hold a declaration that is no helper; the file stays whole", len(holding), len(boxes))})
			fmt.Fprintf(&r.tables, "%s: %d of %d boxes hold a declaration that is no helper; the file stays whole\n\n", path, len(holding), len(boxes))
			continue
		}
		place.split[candidate.file.id] = split
	}
	if len(place.split) == 0 {
		return result, nil
	}
	place.settle()
	if err := r.secondPass(ctx, round, place); err != nil {
		return nil, err
	}
	r.recordPlacement(view.targetID, place)
	result.attached, result.attachedFiles = place.resolved()
	into := place.into()
	result.into = into
	for _, file := range facts.files {
		state := place.split[file.file.id]
		if state == nil {
			continue
		}
		split := &roleSplit{file: file.file, boxes: state.boxes, holds: make([][]string, len(state.boxes))}
		for _, unit := range file.units {
			if unit.seed {
				continue
			}
			if box := state.box[unit.id]; box >= 0 {
				split.holds[box] = append(split.holds[box], unit.id)
				continue
			}
			if _, placed := place.attached[unit.id]; placed {
				continue
			}
			if place.blocked(unit.id) {
				split.blocked = append(split.blocked, unit.id)
			} else {
				split.undecided = append(split.undecided, unit.id)
			}
		}
		path := file.file.path
		var empty []string
		holding, helperOnly := 0, 0
		for i, box := range state.boxes {
			key := groupKey{file: file.file.id, box: i}
			if len(split.holds[i]) == 0 && len(into[key]) == 0 {
				empty = append(empty, box.Name)
				continue
			}
			holding++
			only := true
			for _, id := range split.holds[i] {
				only = only && helpers[id]
			}
			for _, id := range into[key] {
				only = only && helpers[id]
			}
			if only {
				helperOnly++
			}
		}
		if len(empty) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: view.targetID, Kind: "role_box_empty", Count: len(empty), Samples: empty,
				Reason: path + ": named boxes no declaration went in; not drawn"})
		}
		offMap := func(kind string, ids []string, reason string) {
			if len(ids) == 0 {
				return
			}
			var names []string
			for _, id := range ids {
				names = append(names, file.byID[id].name)
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageRoleAssign, Target: view.targetID, Kind: kind, Count: len(names), Samples: names, Reason: path + ": " + reason})
			fmt.Fprintf(&r.tables, "%s: %s: %s\n", path, reason, strings.Join(names, " "))
		}
		offMap("role_undecided", split.undecided, "declarations no box took; each is a row of its own in the parts request")
		offMap("role_blocked", split.blocked, "helpers never asked, since a declaration that uses them never got a box; off the map as blocked")
		fmt.Fprintf(&r.tables, "%s: split into %d boxes, %d undecided, %d blocked, %d named boxes empty, %d holding only helpers\n", path, holding, len(split.undecided), len(split.blocked), len(empty), helperOnly)
		for i, box := range state.boxes {
			var names []string
			for _, id := range split.holds[i] {
				names = append(names, file.byID[id].name)
			}
			for _, id := range into[groupKey{file: file.file.id, box: i}] {
				names = append(names, facts.units[id].path+":"+facts.units[id].name)
			}
			fmt.Fprintf(&r.tables, "- %s (%d): %s\n", box.Name, len(names), strings.Join(names, " "))
		}
		r.tables.WriteString("\n")
		result.splits[file.file.id] = split
	}
	return result, nil
}

// askGate asks the gate for each candidate at once and says, by candidate,
// whether its code goes in several boxes with a clear lead.
func (r *reader) askGate(ctx context.Context, round int, candidates []*roleFile) ([]bool, error) {
	several := make([]bool, len(candidates))
	if len(candidates) == 0 {
		return several, nil
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
	count := 0
	for i := range candidates {
		if answers[i].answer["boxes"] == lines.RoleSeveralBoxes {
			several[i] = true
			count++
		}
	}
	fmt.Fprintf(&r.tables, "role gate: %d of %d files go in several boxes\n\n", count, len(candidates))
	return several, nil
}

// nameBoxes asks the naming for each file at once, over its units that are
// no helpers, and returns, by file, the boxes of every answer that names at
// least two complete boxes.
func (r *reader) nameBoxes(ctx context.Context, view *designView, round int, files []*roleFile, helpers map[string]bool) (map[string][]roleBox, error) {
	r.started[lines.StageRoleBoxes] = time.Now()
	target := r.opts.Targets[round-1]
	r.opts.Stage(lines.StageRoleBoxes, fmt.Sprintf("%s: naming the boxes the code of %d files goes in", target.Name, len(files)))
	calls := make([]llm.Call[boxesAnswer], len(files))
	for i, file := range files {
		call, err := designBoxesCall(file.boxesInput(helpers))
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
	return def.Stage == lines.StageRoleHelper || def.Stage == lines.StageRoleGate || def.Stage == lines.StageRoleAssign
}
