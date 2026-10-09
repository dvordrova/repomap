package reading

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/programindex"
)

//go:embed prompts/design_describe.md
var designDescribePrompt string

const (
	designDescribeTask = "repomap.atlas.describe.v1"

	// designDescriptionOutputTokens is the measured allowance of one part
	// description: a sentence of at most twelve words.
	designDescriptionOutputTokens = 200
	// designOutputFloor and designOutputPerRow make the output allowance of
	// a parts or areas answer: min(128,000, max(8,192, 16 × listed rows)).
	// Measured parts answers used 1,528–1,743 tokens for 294 files and
	// 273–333 for 41; an answer that reaches the allowance is the ordinary
	// output refusal, never truncated or partly accepted.
	designOutputFloor  = 8192
	designOutputPerRow = 16
)

// designOutputTokens is the output allowance of a parts or areas request
// listing rows rows.
func designOutputTokens(rows int) int {
	return min(llm.DefaultMaxOutputTokens, max(designOutputFloor, designOutputPerRow*rows))
}

// designFile is a file of the target that holds at least one unit of its
// own: a function, a variable, a type together with its methods, or a
// module body.
type designFile struct {
	id, path, dir   string
	test, generated bool
	// units are the symbol places of the file's units in declaration order.
	units []string
}

// designView is what the code knows about one target's files before any
// part is drawn. It is built once and only read afterwards.
type designView struct {
	targetID string
	// files are the unit-bearing files in f* order; byID finds them.
	files []*designFile
	byID  map[string]*designFile
	// all is every file of the target in f* order, with or without units.
	all []string
	// decls are every declaration of the target's files by file.
	decls map[string][]string
	// follows maps a declaration that is not a unit to the declaration whose
	// membership it takes: a method to its type, a lexical child to the
	// function or method whose source range holds it.
	follows map[string]string
	// unitFile is the file a unit is declared in; kind is "types",
	// "functions", "variables" or "" for a module body, and text is its
	// name, with its signature when it is exported.
	unitFile map[string]string
	kind     map[string]string
	text     map[string]string
	// sites are the exact call sites of the target's declarations, each with
	// the units it reaches other than its own; imports are the imports
	// between unit-bearing files.
	sites   []unitSite
	imports map[[2]string]bool
	// seedName is, for each unit the target's execution begins at (a symbol
	// place whose Seeds hold the target), the declaration's own name: a
	// split file gives it a row of its own, so the entry is never left off
	// the map by the assignment.
	seedName map[string]string
	// name is every unit's declaration name: the name of the row a split
	// file gives a unit no box took.
	name map[string]string
	// Supporting observations never change the atomic membership units.
	declarations map[string]partsDeclaration
	ownedFields  map[string][]partsDeclaration
	callSites    []designCallSite
	roots        map[string]string
	err          error
}

// These projections carry source structure, never native identities or docs.
// d* refs are allocated once for this target's evidence, before packing.
type partsDeclaration struct {
	Ref       string               `json:"ref"`
	OwnerRef  string               `json:"owner_ref,omitempty"`
	Path      string               `json:"path"`
	Name      string               `json:"name"`
	Kind      string               `json:"kind"`
	Signature string               `json:"signature,omitempty"`
	Aliases   string               `json:"aliases,omitempty"`
	Line      int                  `json:"line"`
	Column    int                  `json:"column,omitempty"`
	EndLine   int                  `json:"end_line,omitempty"`
	Exported  bool                 `json:"exported"`
	Anonymous bool                 `json:"anonymous,omitempty"`
	Macro     bool                 `json:"macro,omitempty"`
	Overloads []atlas.DeclOverload `json:"overloads,omitempty"`
}

func partsDeclarationOf(ref, sourcePath string, decl atlas.Decl) partsDeclaration {
	return partsDeclaration{Ref: ref, Path: sourcePath, Name: decl.Name, Kind: decl.Kind, Signature: decl.Signature, Aliases: decl.Aliases,
		Line: decl.LineNo, Column: decl.Column, EndLine: decl.EndLine, Exported: decl.Exported, Anonymous: decl.Anonymous, Macro: decl.Macro, Overloads: slices.Clone(decl.Overloads)}
}

type partsEndpoint struct {
	Ref       string `json:"ref"`
	UnitRef   string `json:"unit_ref,omitempty"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature,omitempty"`
	Line      int    `json:"line"`
	Column    int    `json:"column,omitempty"`
}

type partsCallSite struct {
	Caller           partsEndpoint        `json:"caller"`
	Path             string               `json:"path"`
	Kind             string               `json:"kind"`
	Name             string               `json:"name"`
	Line             int                  `json:"line"`
	Column           int                  `json:"column,omitempty"`
	Invocation       string               `json:"invocation,omitempty"`
	Dispatch         string               `json:"dispatch,omitempty"`
	Detail           string               `json:"detail,omitempty"`
	Resolution       string               `json:"resolution,omitempty"`
	Values           []string             `json:"values,omitempty"`
	Arguments        []string             `json:"arguments,omitempty"`
	API              *atlas.CallAPI       `json:"api,omitempty"`
	CalleeCandidates []partsEndpoint      `json:"callee_candidates,omitempty"`
	Evidence         []atlas.EdgeEvidence `json:"evidence,omitempty"`
}
type designCallSite struct {
	caller      string
	callees     []string
	observation partsCallSite
}

// unitSite is one exact call site: the unit whose code it is in and the
// distinct other units its callees belong to.
type unitSite struct {
	from string
	to   []string
}

// designView reads one target's files and declarations. Every declaration
// with a symbol place is either a unit of its file or follows one: a method
// its type, a lexical child the callable that encloses it.
func (r *reader) designView(targetID string) *designView {
	view := &designView{
		targetID: targetID, byID: map[string]*designFile{}, decls: map[string][]string{},
		follows: map[string]string{}, unitFile: map[string]string{}, kind: map[string]string{}, text: map[string]string{}, imports: map[[2]string]bool{},
		seedName: map[string]string{}, name: map[string]string{},
		declarations: map[string]partsDeclaration{}, ownedFields: map[string][]partsDeclaration{}, roots: map[string]string{},
	}
	var files []atlas.Place
	for _, place := range r.opts.Graph.Places {
		if place.File != nil && contains(place.TargetIDs, targetID) {
			files = append(files, place)
		}
	}
	sort.Slice(files, func(i, j int) bool { return compactIDLess(files[i].ID, files[j].ID) })
	// A method goes with its type when the type is a declaration of this
	// target, through the native owner the type's members name.
	inTarget := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.File.Decls {
			if id := r.symbolID(file.Path, decl.LineNo, decl.Name); id != "" && contains(r.places[id].TargetIDs, targetID) {
				inTarget[id] = true
			}
		}
	}
	typeOf := map[string]string{}
	for _, file := range files {
		for _, decl := range file.File.Decls {
			id := r.symbolID(file.Path, decl.LineNo, decl.Name)
			symbol := r.places[id].Symbol
			if decl.Kind != "type" || symbol == nil || !inTarget[id] {
				continue
			}
			for _, member := range symbol.Members {
				// Fields are not declarations of their own; every declaration
				// the type natively owns, its methods, goes with it.
				if member.Decl.Kind == "type" || member.Decl.Kind == "module" {
					continue
				}
				if method := r.symbolID(member.Path, member.Decl.LineNo, member.Decl.Name); method != "" && method != id && inTarget[method] {
					typeOf[method] = id
				}
			}
		}
	}
	for _, file := range files {
		view.all = append(view.all, file.ID)
		row := &designFile{id: file.ID, path: file.Path, dir: path.Dir(file.Path), test: file.File.Test, generated: file.File.Generated}
		decls := slices.DeleteFunc(slices.Clone(file.File.Decls), func(decl atlas.Decl) bool {
			return !inTarget[r.symbolID(file.Path, decl.LineNo, decl.Name)]
		})
		// named is the first unit of the file under each name: a declaration
		// that repeats it (a second Go init, a Clojure declare and its defn)
		// follows it, one unit shown with the first one's signature. Python's
		// @overload stubs and TS overload signatures are no declarations:
		// they fold into their implementation (Decl.Overloads), whose unit
		// shows the first overload's signature, the first one written.
		named := map[string]string{}
		for position, decl := range decls {
			id := r.symbolID(file.Path, decl.LineNo, decl.Name)
			if id == "" || !inTarget[id] {
				continue
			}
			view.decls[file.ID] = append(view.decls[file.ID], id)
			view.declarations[id] = partsDeclarationOf(fmt.Sprintf("d%d", len(view.declarations)+1), file.Path, decl)
			if decl.ObjectID != "" {
				r.designSubjects[decl.ObjectID] = id
			}
			if owner := typeOf[id]; owner != "" {
				view.follows[id] = owner
				continue
			}
			if parent := enclosingCallable(decls, position); parent >= 0 {
				if parentID := r.symbolID(file.Path, decls[parent].LineNo, decls[parent].Name); parentID != "" {
					view.follows[id] = parentID
					continue
				}
			}
			if first := named[decl.Name]; first != "" {
				view.follows[id] = first
				continue
			}
			named[decl.Name] = id
			row.units = append(row.units, id)
			view.unitFile[id] = file.ID
			if symbol := r.places[id].Symbol; symbol != nil && slices.Contains(symbol.Seeds, targetID) {
				view.seedName[id] = decl.Name
			}
			view.name[id] = decl.Name
			view.text[id] = decl.Name
			signature := decl.Signature
			if len(decl.Overloads) > 0 && decl.Overloads[0].Signature != "" {
				signature = decl.Overloads[0].Signature
			}
			if decl.Exported && signature != "" {
				view.text[id] += " " + signature
			}
			switch decl.Kind {
			case "type":
				view.kind[id] = "types"
			case "variable":
				view.kind[id] = "variables"
			case "module":
				// A module body counts as a unit; its name repeats the path.
			default:
				view.kind[id] = "functions"
			}
		}
		if len(row.units) > 0 {
			view.files = append(view.files, row)
			view.byID[file.ID] = row
		}
	}
	// Resolve every native ancestry before any interpretation request. A cycle
	// or missing owner is a source-format error, never a partial membership.
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			view.root(id)
			if view.err != nil {
				return view
			}
		}
	}
	// Followers retain their full headers and immediate owner without becoming
	// assignment rows. Fields retain their exact native observer scope.
	nextRef := len(view.declarations)
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			decl := view.declarations[id]
			if owner := view.follows[id]; owner != "" {
				decl.OwnerRef = view.declarations[owner].Ref
				view.declarations[id] = decl
			}
			if r.places[id].Symbol == nil {
				continue
			}
			for _, member := range r.places[id].Symbol.Members {
				if !slices.Contains(member.TargetIDs, targetID) {
					continue
				}
				if memberID := r.symbolID(member.Path, member.Decl.LineNo, member.Decl.Name); memberID != "" {
					continue // A lifted declaration occurs only through its eligible source row.
				}
				if member.Decl.Kind != "variable" && member.Decl.Kind != "field" {
					continue // Only native unlifted fields supplement source declarations.
				}
				nextRef++
				field := partsDeclarationOf(fmt.Sprintf("d%d", nextRef), member.Path, member.Decl)
				field.OwnerRef = decl.Ref
				view.ownedFields[id] = append(view.ownedFields[id], field)
			}
		}
	}
	// A call site counts from the unit whose code holds it: a method's from
	// its type's unit, wherever the method is declared.
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			symbol := r.places[id].Symbol
			from := view.root(id)
			if symbol == nil || view.unitFile[from] == "" {
				continue
			}
			for _, call := range symbol.Calls {
				if !contains(callTargets(r.places[id], call), targetID) {
					continue
				}
				site := designCallSite{caller: id, observation: partsCallSite{Path: r.places[id].Path, Kind: call.Kind, Name: call.Name,
					Line: call.Line, Column: call.Column, Invocation: call.Invocation, Dispatch: call.Dispatch, Detail: call.Detail, Resolution: call.Resolution,
					Values: slices.Clone(call.Values), Arguments: slices.Clone(call.Arguments), API: call.API, Evidence: slices.Clone(call.Evidence)}}
				for _, callee := range call.CalleeIDs {
					if inTarget[callee] {
						site.callees = append(site.callees, callee)
					}
				}
				view.callSites = append(view.callSites, site)
				if call.Kind != "calls" || call.Resolution != "exact" {
					continue
				}
				var reached []string
				for _, callee := range call.CalleeIDs {
					if !inTarget[callee] {
						continue
					}
					if to := view.root(callee); view.unitFile[to] != "" && to != from && !slices.Contains(reached, to) {
						reached = append(reached, to)
					}
				}
				if len(reached) > 0 {
					view.sites = append(view.sites, unitSite{from: from, to: reached})
				}
			}
		}
	}
	for _, edge := range r.opts.Graph.Edges {
		if edge.Kind == "imports" && edge.From != edge.To && view.byID[edge.From] != nil && view.byID[edge.To] != nil {
			view.imports[[2]string{edge.From, edge.To}] = true
		}
	}
	return view
}

func (view *designView) partsEndpoint(id string, rowOf map[string]string) partsEndpoint {
	decl := view.declarations[id]
	return partsEndpoint{Ref: decl.Ref, UnitRef: rowOf[view.root(id)], Path: decl.Path, Name: decl.Name, Kind: decl.Kind, Signature: decl.Signature, Line: decl.Line, Column: decl.Column}
}

// root is the unit a declaration's membership comes from.
func (view *designView) root(id string) string {
	if view.roots == nil {
		view.roots = map[string]string{}
	}
	if root := view.roots[id]; root != "" {
		return root
	}
	var ancestry []string
	seen := map[string]bool{}
	current := id
	for {
		if seen[current] {
			view.err = fmt.Errorf("parts source ownership has a cycle at %s", current)
			return ""
		}
		seen[current] = true
		ancestry = append(ancestry, current)
		if root := view.roots[current]; root != "" {
			current = root
			break
		}
		next, ok := view.follows[current]
		if !ok {
			break
		}
		if next == "" || view.declarations[next].Ref == "" {
			view.err = fmt.Errorf("parts source ownership of %s has no known declaration %s", current, next)
			return ""
		}
		current = next
	}
	for _, declaration := range ancestry {
		view.roots[declaration] = current
	}
	return current
}

// enclosingCallable is the innermost function or method of a file whose
// source range holds declaration at: the adapter's own positions and end
// lines make it the declaration's lexical parent. -1 when none does.
func enclosingCallable(decls []atlas.Decl, at int) int {
	child := decls[at]
	best := -1
	for position, decl := range decls {
		if position == at || decl.EndLine <= 0 || decl.Kind != "function" && decl.Kind != "method" {
			continue
		}
		starts := decl.LineNo < child.LineNo || decl.LineNo == child.LineNo && decl.Column < child.Column
		if !starts || child.LineNo > decl.EndLine {
			continue
		}
		if best < 0 || decls[best].LineNo < decl.LineNo || decls[best].LineNo == decl.LineNo && decls[best].Column < decl.Column {
			best = position
		}
	}
	return best
}

// designUnit is one row of a target's parts request: a whole file (its f*
// ref), or one box of a file the role split splits (a request-local c* ref,
// with the box's name). Its units are the declarations whose membership the
// row carries; they take the row's part.
type designUnit struct {
	ref, path, dir, box string
	// file is the f* place of the file the row's units are declared in.
	file            string
	test, generated bool
	units           []string
	// types, functions and variables are the units' names, exported ones
	// with their signature, each once.
	types, functions, variables []string
}

// designUnitRow is one unit as the parts and placement requests show it.
type designUnitRow struct {
	Ref       string   `json:"ref"`
	Path      string   `json:"path"`
	Box       string   `json:"box,omitempty"`
	Units     int      `json:"units"`
	Types     []string `json:"types,omitempty"`
	Functions []string `json:"functions,omitempty"`
	Variables []string `json:"variables,omitempty"`
}

func (unit *designUnit) row() designUnitRow {
	return designUnitRow{Ref: unit.ref, Path: unit.path, Box: unit.box, Units: len(unit.units), Types: unit.types, Functions: unit.functions, Variables: unit.variables}
}

// designUnitOf is a row of the given units of one file.
func (view *designView) designUnitOf(ref string, file *designFile, box string, units []string) *designUnit {
	unit := &designUnit{ref: ref, path: file.path, dir: file.dir, box: box, file: file.id, test: file.test, generated: file.generated, units: units}
	for _, id := range units {
		switch view.kind[id] {
		case "types":
			unit.types = appendUnique(unit.types, view.text[id])
		case "variables":
			unit.variables = appendUnique(unit.variables, view.text[id])
		case "functions":
			unit.functions = appendUnique(unit.functions, view.text[id])
		}
	}
	return unit
}

func cleanText(text string) string {
	space := func(r rune) bool { return unicode.IsSpace(r) || r < 0x20 || r == 0x7f }
	return strings.Join(strings.FieldsFunc(text, space), " ")
}

// refList reads a list of refs: a JSON array of strings, or one string of
// refs separated by spaces or commas, the form a table sequence cell takes.
// Every ref is still checked by the caller. Anything else is not a list.
func refList(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	var refs []string
	if json.Unmarshal(raw, &refs) == nil {
		return refs, true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.Fields(strings.ReplaceAll(text, ",", " ")), true
	}
	return nil, false
}

// inputRefused says whether the provider refused a request for its input or
// context size, the one refusal a smaller window of complete files answers.
func inputRefused(err error) bool {
	var resource *llm.ResourceLimitError
	return errors.As(err, &resource) && (resource.Kind == llm.ResourceLimitContextTokens || resource.Kind == llm.ResourceLimitRequestBytes)
}

// designPart is one drawn part of a target before it is a box: the units
// of the rows its refs name.
type designPart struct {
	id, name string
	holds    string
	refs     []string
}

// designOutcome is a target's map after closed original-unit assignment.
type designOutcome struct {
	parts  []*designPart
	partOf map[string]string // unit row ref → part ID
	// unitPart is each placed unit's part; unitReason why a unit is off the
	// map: undecided (no box of its split file took it), left_out or
	// conflict (its row's).
	unitPart, unitReason map[string]string
	failure              string
	boxes                []*boxState
	membership           map[string]string // declaration or file place → part ID
	refusedParts         []atlas.RefusedPart
}

func (r *reader) readDesign(ctx context.Context) error {
	r.opts.Stage(lines.StageZones, "reading complete original code, choosing responsibilities, and assigning every original unit to their accepted purposes")
	r.boxes = map[string]*boxState{}
	r.designBoxOf = map[string]map[string]string{}
	r.helperOf = map[string]map[string]bool{}
	r.treeZones = map[string][]zoneDraft{}
	r.offMap = map[string][]offMapEntry{}
	r.mapFailure = map[string]string{}
	r.refusedParts = map[string][]atlas.RefusedPart{}
	if r.designSubjects == nil {
		r.designSubjects = map[string]string{}
	}
	targets := r.opts.Targets
	// Validate every complete target view before any model work, as in the
	// eager walk. Keep only its source-subject mapping; workers read this
	// finalized mapping without any concurrent writer.
	for _, target := range targets {
		view := r.designView(target.ID)
		if view.err != nil {
			return view.err
		}
	}
	// Allocate the small result records up front. Each target acquires a
	// preparation slot before rebuilding its complete native view.
	readers := make([]*reader, len(targets))
	for position, target := range targets {
		r.designBoxOf[target.ID] = map[string]string{}
		readers[position] = r.view(nil)
	}
	order := &designOrder{parts: newDesignTurns(len(targets))}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	concurrency := r.opts.Executor.BatchConcurrency
	if concurrency <= 0 {
		concurrency = llm.DefaultBatchConcurrency
	}
	slots := make(chan struct{}, min(concurrency, len(targets)))
	var failed sync.Once
	var failure error
	fail := func(err error) {
		failed.Do(func() {
			failure = err
			cancel()
		})
	}
	// Freeze preparation's reader snapshot before workers can mutate owner
	// counters. Subsequent builds copy this immutable base only.
	preparationBase := *r
	var wg sync.WaitGroup
dispatch:
	for position, target := range targets {
		if err := ctx.Err(); err != nil {
			fail(err)
			break
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			fail(ctx.Err())
			break dispatch
		}
		// Rebuilding has the same complete source input, but its temporary
		// subject writes belong to this preparation, not the shared map.
		preparation := preparationBase
		preparation.designSubjects = map[string]string{}
		view := preparation.designView(target.ID)
		if view.err != nil {
			fail(view.err)
			<-slots
			break
		}
		wg.Add(1)
		go func(position int, view *designView) {
			defer wg.Done()
			defer func() { <-slots }()
			if err := readers[position].designTarget(ctx, r, order, position, view); err != nil {
				fail(err)
			}
		}(position, view)
	}
	wg.Wait()
	if failure != nil {
		return failure
	}
	for _, view := range readers {
		r.joinView(view)
	}
	for _, stage := range []string{lines.StageRoleHelper, lines.StageGroupEnough, lines.StageGroupAssign} {
		if _, asked := r.uses[stage]; asked {
			r.reportStage(stage)
		}
	}
	r.reportStage(lines.StageZones)
	r.reportStage(lines.StageDescribe)
	return nil
}

// designOrder hands out the compact part IDs in target order. Drawing takes
// the owner's lock as well, so a canceled target that passes its turn early
// never draws beside another.
type designOrder struct {
	parts   *designTurns
	drawing sync.Mutex
}

// designTurns lets target p go once every target before it has passed.
type designTurns struct {
	ready  []chan struct{}
	passed []sync.Once
}

func newDesignTurns(targets int) *designTurns {
	turns := &designTurns{ready: make([]chan struct{}, targets+1), passed: make([]sync.Once, targets+1)}
	for i := range turns.ready {
		turns.ready[i] = make(chan struct{})
	}
	turns.passed[0].Do(func() { close(turns.ready[0]) })
	return turns
}

func (turns *designTurns) wait(ctx context.Context, position int) error {
	select {
	case <-turns.ready[position]:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (turns *designTurns) pass(position int) {
	turns.passed[position+1].Do(func() { close(turns.ready[position+1]) })
}

// offMapEntry is one file, or the declarations of a file, no drawn part
// holds. boxID is the part that holds the file itself when only the listed
// declarations are off the map.
type offMapEntry struct {
	fileID  string
	reason  string
	symbols []string
	boxID   string
}

// designTarget is one target's map of parts, read on the view r and drawn on
// owner, the reader the views share. The role split of its files runs
// first; the parts request then groups the files and the boxes of the split
// files, and its parts take their IDs in answer order.
func (r *reader) designTarget(ctx context.Context, owner *reader, order *designOrder, position int, view *designView) error {
	defer order.parts.pass(position)
	target := r.opts.Targets[position]
	round := position + 1
	outcome := &designOutcome{partOf: map[string]string{}, unitPart: map[string]string{}, unitReason: map[string]string{}, membership: map[string]string{}}
	units, drafts, tree, err := r.groupedUnits(ctx, view, round, target, outcome)
	if err != nil {
		return err
	}
	// Every declaration whose unit Jev decided is a helper carries the mark.
	helpers := map[string]bool{}
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			if tree.helpers[view.root(id)] {
				helpers[id] = true
			}
		}
	}
	// The parts take their compact IDs in target order, each target's in
	// answer order.
	if err := order.parts.wait(ctx, position); err != nil {
		return err
	}
	order.drawing.Lock()
	for i := range drafts {
		drafts[i].id = owner.compactID("p", &owner.nextPart)
	}
	order.drawing.Unlock()
	order.parts.pass(position)
	for i := range drafts {
		part := &drafts[i]
		outcome.parts = append(outcome.parts, part)
		for _, ref := range part.refs {
			outcome.partOf[ref] = part.id
		}
	}
	r.drawParts(view, outcome, units)
	zones := tree.zones(drafts)
	if err := r.describeParts(ctx, view, round, outcome); err != nil {
		return err
	}
	order.drawing.Lock()
	defer order.drawing.Unlock()
	for _, box := range outcome.boxes {
		owner.boxes[box.id] = box
	}
	membership := owner.designBoxOf[target.ID]
	for place, part := range outcome.membership {
		membership[place] = part
	}
	owner.helperOf[target.ID] = helpers
	owner.treeZones[target.ID] = zones
	owner.offMap[target.ID] = r.offMapEntries(view, outcome)
	owner.refusedParts[target.ID] = outcome.refusedParts
	if outcome.failure != "" {
		owner.mapFailure[target.ID] = outcome.failure
	}
	return nil
}

// drawParts gives every declaration its unit's part (a method its type's, a
// lexical child its parent's) and builds the boxes. A part's sources are the
// files of its units. A file's own part is the one part holding every
// placed unit declared in it, or, for a file that declares only what
// follows units of other files, the one part holding its placed
// declarations; a file whose units sit in two parts, as a split file's
// usually do, has none. A part whose every file is test code is a fact,
// kept in the atlas and off the canvas.
func (r *reader) drawParts(view *designView, outcome *designOutcome, units []*designUnit) {
	byRef := map[string]*designUnit{}
	for _, unit := range units {
		byRef[unit.ref] = unit
	}
	boxes := map[string]*boxState{}
	for _, part := range outcome.parts {
		box := &boxState{id: part.id, targetID: view.targetID, title: part.name, line: part.holds, open: true, dir: ".", symbols: map[string]bool{}, test: true}
		for _, ref := range part.refs {
			unit := byRef[ref]
			box.unitIDs = append(box.unitIDs, unit.units...)
			box.sources = appendUnique(box.sources, unit.file)
			// A unit code placed in this row from another file brings its file.
			for _, id := range unit.units {
				box.sources = appendUnique(box.sources, view.unitFile[id])
			}
			box.test = box.test && unit.test
			for _, id := range unit.units {
				outcome.unitPart[id] = part.id
			}
		}
		box.units = len(box.unitIDs)
		boxes[part.id] = box
	}
	for _, ref := range view.all {
		for _, id := range view.decls[ref] {
			part := outcome.unitPart[view.root(id)]
			if part == "" {
				continue
			}
			outcome.membership[id] = part
			box := boxes[part]
			box.symbols[id] = true
			box.files = appendUnique(box.files, ref)
		}
	}
	for _, ref := range view.all {
		if outcome.failure != "" {
			break
		}
		own := view.byID[ref] != nil
		shared, last := map[string]bool{}, ""
		for _, id := range view.decls[ref] {
			part := outcome.membership[id]
			if part == "" || own && view.unitFile[view.root(id)] != ref {
				continue
			}
			shared[part], last = true, part
		}
		if len(shared) == 1 {
			outcome.membership[ref] = last
		}
	}
	for _, part := range outcome.parts {
		box := boxes[part.id]
		sort.Slice(box.files, func(i, j int) bool { return compactIDLess(box.files[i], box.files[j]) })
		sort.Slice(box.sources, func(i, j int) bool { return compactIDLess(box.sources[i], box.sources[j]) })
		if len(box.sources) > 0 {
			box.dir = path.Dir(r.places[box.sources[0]].Path)
		}
		box.forTests = box.test && len(box.sources) > 0
		box.unreached = !box.forTests && r.runsNothing(box)
		outcome.boxes = append(outcome.boxes, box)
	}
}

// runsNothing reports a part its program never runs: it holds declarations
// that run, and the program's adapter proved every one of them unreachable
// there (ProgramIndex `unreachable`, which only the C adapter proves). Its
// types, fields and variables run nothing of their own and follow it. Like a
// part made only of test code, it leaves the canvas: redis-cli links
// adlist.c and never calls one of its thirteen functions, and "Linked list"
// stood on its map with an arrow to Memory allocation.
func (r *reader) runsNothing(box *boxState) bool {
	runs := false
	for id := range box.symbols {
		place := r.places[id]
		if place.Symbol == nil || !programindex.ObjectKind(place.Symbol.Decl.Kind).Callable() {
			continue
		}
		if !slices.Contains(place.Symbol.Unreached, box.targetID) {
			return false
		}
		runs = true
	}
	return runs
}

// offMapEntries lists every declaration of the target no part holds, under
// its file, by its unit's reason (undecided, left_out, conflict), or
// map_failure when the target has no map; a file that declares nothing is
// listed whole as no_units. An entry names the part that holds its file
// when there is one: the file itself stays on the map then. A file every
// declaration of which a part holds has no entry.
func (r *reader) offMapEntries(view *designView, outcome *designOutcome) []offMapEntry {
	entries := []offMapEntry{}
	for _, ref := range view.all {
		byReason := map[string][]string{}
		var reasons []string
		for _, id := range view.decls[ref] {
			if outcome.membership[id] != "" {
				continue
			}
			why := outcome.unitReason[view.root(id)]
			switch {
			case outcome.failure != "":
				why = atlas.OffMapFailure
			case why == "":
				why = atlas.OffMapNoUnits
			}
			if byReason[why] == nil {
				reasons = append(reasons, why)
			}
			byReason[why] = append(byReason[why], id)
		}
		if len(view.decls[ref]) == 0 {
			reasons = []string{atlas.OffMapNoUnits}
			if outcome.failure != "" {
				reasons = []string{atlas.OffMapFailure}
			}
		}
		for _, why := range reasons {
			entries = append(entries, offMapEntry{fileID: ref, reason: why, symbols: byReason[why], boxID: outcome.membership[ref]})
		}
	}
	return entries
}

// describeMember is one unit of a part as the description request shows it.
type describeMember struct {
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	// Line is the description of a part named as a member of an area.
	Line string `json:"line,omitempty"`
}

type describeFile struct {
	File    string           `json:"file"`
	Members []describeMember `json:"members"`
}

type describeDirectory struct {
	Dir   string         `json:"dir"`
	Files []describeFile `json:"files"`
}

// describeInput is one description request: a part and its members grouped
// dir → file, or an area and its parts.
type describeInput struct {
	Task        string              `json:"task"`
	Part        string              `json:"part"`
	Directories []describeDirectory `json:"directories,omitempty"`
	Members     []describeMember    `json:"members,omitempty"`
}

type description struct {
	Description string `json:"description"`
}

// decodeDescription reads {"description":"…"}; an empty or undecodable one
// is refused. A long one is kept as returned.
func decodeDescription(raw []byte) (description, error) {
	var value description
	if err := json.Unmarshal(raw, &value); err != nil {
		return description{}, fmt.Errorf("description: the answer is not a description object")
	}
	value.Description = cleanText(value.Description)
	if value.Description == "" {
		return description{}, fmt.Errorf("description: the answer is empty")
	}
	return value, nil
}

func describeCall(input describeInput) (llm.Call[description], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[description]{}, err
	}
	return llm.Call[description]{
		State: []byte(designDescribeTask),
		Prompt: llm.Prompt{System: designDescribePrompt, User: string(raw), ResponseFormatJSON: true,
			ResponseExample: `{"description":"Chooses a move by exploring legal continuations."}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designDescriptionOutputTokens},
		DecodeValidate: decodeDescription,
	}, nil
}

// partDescribeInput lists every unit a part holds by directory and file,
// with its name and its signature when it has one: all units of a file it
// holds whole, only its own of a split file. No documentation is sent.
func (r *reader) partDescribeInput(view *designView, box *boxState) describeInput {
	input := describeInput{Task: designDescribeTask, Part: box.title}
	byDir := map[string]*describeDirectory{}
	var dirs []string
	held := make(map[string]bool, len(box.unitIDs))
	for _, id := range box.unitIDs {
		held[id] = true
	}
	rows := slices.Clone(box.sources)
	sort.Slice(rows, func(i, j int) bool { return view.byID[rows[i]].path < view.byID[rows[j]].path })
	for _, ref := range rows {
		file := view.byID[ref]
		var members []describeMember
		for _, id := range file.units {
			if !held[id] {
				continue
			}
			decl := r.places[id].Symbol.Decl
			if decl.Kind == "module" {
				continue
			}
			members = append(members, describeMember{Name: decl.Name, Signature: decl.Signature})
		}
		sort.SliceStable(members, func(i, j int) bool { return members[i].Name < members[j].Name })
		if len(members) == 0 {
			continue
		}
		dir := byDir[file.dir]
		if dir == nil {
			dir = &describeDirectory{Dir: file.dir}
			byDir[file.dir] = dir
			dirs = append(dirs, file.dir)
		}
		dir.Files = append(dir.Files, describeFile{File: file.path, Members: members})
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		input.Directories = append(input.Directories, *byDir[dir])
	}
	return input
}

// describeParts describes only an automatic native part with no catalogue
// purpose. Accepted responsibility purposes stay unchanged. A refused native
// description leaves the explicit no-description state.
func (r *reader) describeParts(ctx context.Context, view *designView, round int, outcome *designOutcome) error {
	if r.dry {
		return nil
	}
	var boxes []*boxState
	var calls []llm.Call[description]
	for _, box := range outcome.boxes {
		if box.offCanvas() || box.line != "" {
			continue
		}
		input := r.partDescribeInput(view, box)
		if len(input.Directories) == 0 {
			// A part whose only units are module bodies has no member to
			// describe it by; a request of its name alone would invent one.
			fmt.Fprintf(&r.tables, "%s: no member to describe it by; no description\n\n", box.title)
			continue
		}
		call, err := describeCall(input)
		if err != nil {
			return err
		}
		boxes = append(boxes, box)
		calls = append(calls, call)
	}
	if len(calls) == 0 {
		return nil
	}
	if _, started := r.started[lines.StageDescribe]; !started {
		r.started[lines.StageDescribe] = time.Now()
	}
	described, err := r.describe(ctx, lines.StageDescribe, round, calls)
	if err != nil {
		return err
	}
	for i, box := range boxes {
		box.line = described[i]
	}
	return nil
}

// describe runs description requests at once and returns each accepted
// sentence, or "" for a refused one, which it records.
func (r *reader) describe(ctx context.Context, stage string, round int, calls []llm.Call[description]) ([]string, error) {
	executor := debugdump.BindStage(r.opts.Executor, stage)
	results := llm.ExecuteJSONEach(ctx, executor, r.opts.Provider, calls)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	use := r.use(stage)
	described := make([]string, len(calls))
	for i, result := range results {
		window := table.Window{Stage: stage, Round: round, Index: i}
		use.Windows++
		use.Rows++
		if result.Outcome.Cached {
			use.Cached++
		} else {
			use.Live++
		}
		if err := r.writeWindowExchange(window, nil, []byte(calls[i].Prompt.User), result.Outcome.Request, result.Outcome.Response, result.Err != nil || len(result.Outcome.ResponseRejections) > 0); err != nil {
			return nil, err
		}
		var input describeInput
		_ = json.Unmarshal([]byte(calls[i].Prompt.User), &input)
		if result.Err != nil {
			if errors.Is(result.Err, context.Canceled) {
				return nil, result.Err
			}
			use.Rejected++
			use.Given++
			responseRef := ""
			if len(result.Outcome.Response) > 0 {
				responseRef = path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: stage, Kind: "description_refused", Count: 1, Samples: []string{input.Part}, Reason: result.Err.Error(),
				ResponseRef: responseRef})
			fmt.Fprintf(&r.tables, "## %s · round %d · window %d\n\n%s: no description (%s)\n\n", stage, round, i, input.Part, result.Err)
			continue
		}
		described[i] = result.Outcome.Value.Description
		fmt.Fprintf(&r.tables, "## %s · round %d · window %d\n\n%s: %s\n\n", stage, round, i, input.Part, described[i])
	}
	return described, nil
}

func (r *reader) boxFor(targetID, placeID string) string {
	if owners, ok := r.designBoxOf[targetID]; ok {
		return owners[placeID]
	}
	return r.boxOfPlace(placeID)
}

// boundaryBox is the part a boundary stands in, by one rule for every file:
// its subject's part; an input that hands a subject over stands only where
// that handler is, so with the handler on no part (undecided, or in a file
// off the map) it names none rather than the part of the code that
// registers it; else the part of the innermost declaration whose source
// range holds its line, none when that declaration is off the map; else,
// with no declaration around it, the part of its file's module body, else
// its file's part.
func (r *reader) boundaryBox(targetID string, place atlas.Place) string {
	if place.Boundary != nil {
		subjects := []string{place.Boundary.SubjectID, r.designSubjects[place.Boundary.ObjectID]}
		for _, subject := range subjects {
			if id := r.boxFor(targetID, subject); subject != "" && id != "" {
				return id
			}
		}
		if place.Boundary.Direction == atlas.DirectionIn && (subjects[0] != "" || subjects[1] != "") {
			return ""
		}
	}
	decl, module := r.enclosingDecl(place.Parent, place.LineNo)
	if decl != "" {
		return r.boxFor(targetID, decl)
	}
	if box := r.boxFor(targetID, module); module != "" && box != "" {
		return box
	}
	return r.boxFor(targetID, place.Parent)
}

// enclosingDecl is the symbol place of the innermost declaration of a file
// whose source range holds a line, or "" when none does; module is the
// file's module body.
func (r *reader) enclosingDecl(fileID string, line int) (decl, module string) {
	file := r.places[fileID]
	if file.File == nil {
		return "", ""
	}
	best := -1
	for i, candidate := range file.File.Decls {
		id := r.symbolID(file.Path, candidate.LineNo, candidate.Name)
		if candidate.Kind == "module" {
			module = id
			continue
		}
		if id == "" || candidate.EndLine <= 0 || candidate.LineNo > line || line > candidate.EndLine {
			continue
		}
		if best < 0 || candidate.LineNo >= file.File.Decls[best].LineNo {
			best = i
		}
	}
	if best >= 0 {
		decl = r.symbolID(file.Path, file.File.Decls[best].LineNo, file.File.Decls[best].Name)
	}
	return decl, module
}

// seedBoxes are the parts a seed file enters its target through: the parts
// holding its seed declarations (places `seed_decls`), else its file's part.
func (r *reader) seedBoxes(targetID, seed string) []string {
	var boxes []string
	for _, decl := range r.opts.Graph.SeedDecls {
		if r.places[decl].Parent != seed {
			continue
		}
		if box := r.boxFor(targetID, decl); box != "" {
			boxes = appendUnique(boxes, box)
		}
	}
	if len(boxes) > 0 {
		return boxes
	}
	if box := r.boxFor(targetID, seed); box != "" {
		return []string{box}
	}
	return nil
}
