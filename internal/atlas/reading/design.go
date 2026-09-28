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

//go:embed prompts/design_parts.md
var designPartsPrompt string

//go:embed prompts/design_describe.md
var designDescribePrompt string

//go:embed prompts/design_areas.md
var designAreasPrompt string

const (
	designPartsTask    = "repomap.atlas.parts.v2"
	designDescribeTask = "repomap.atlas.describe.v1"
	designAreasTask    = "repomap.atlas.areas.v1"

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
			if id := r.symbolID(file.Path, decl.LineNo, decl.Name); id != "" {
				inTarget[id] = true
			}
		}
	}
	typeOf := map[string]string{}
	for _, file := range files {
		for _, decl := range file.File.Decls {
			id := r.symbolID(file.Path, decl.LineNo, decl.Name)
			symbol := r.places[id].Symbol
			if decl.Kind != "type" || symbol == nil {
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
		decls := file.File.Decls
		// named is the first unit of the file under each name: a declaration
		// that repeats it (a second Go init, Python @overload stubs and their
		// implementation, TS overload signatures, a Clojure declare and its
		// defn) follows it, one unit shown with the first one's signature.
		named := map[string]string{}
		for position, decl := range decls {
			id := r.symbolID(file.Path, decl.LineNo, decl.Name)
			if id == "" {
				continue
			}
			view.decls[file.ID] = append(view.decls[file.ID], id)
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
			view.text[id] = decl.Name
			if decl.Exported && decl.Signature != "" {
				view.text[id] += " " + decl.Signature
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

// root is the unit a declaration's membership comes from.
func (view *designView) root(id string) string {
	for seen := 0; seen < 64; seen++ {
		next, ok := view.follows[id]
		if !ok {
			return id
		}
		id = next
	}
	return id
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

// groupingUnits are the rows of a target's parts request, in f* order: a
// file the role split splits gives one c* row per box that holds a unit, in
// naming order (c1, c2… across the target); any other file gives one whole
// row under its f* ref. The units no box of a split file took stay off the
// map as undecided.
func (view *designView) groupingUnits(splits map[string]*roleSplit, outcome *designOutcome) []*designUnit {
	var units []*designUnit
	boxes := 0
	for _, file := range view.files {
		split := splits[file.id]
		if split == nil {
			units = append(units, view.designUnitOf(file.id, file, "", file.units))
			continue
		}
		for i, box := range split.boxes {
			if len(split.holds[i]) == 0 {
				continue
			}
			boxes++
			units = append(units, view.designUnitOf(fmt.Sprintf("c%d", boxes), file, box.Name, split.holds[i]))
		}
		for _, id := range split.undecided {
			outcome.unitReason[id] = atlas.OffMapUndecided
		}
	}
	return units
}

// designPartsInput is the parts request: the listed units with the calls
// among them and the imports among whole files, aggregated over refs the
// request advertises.
type designPartsInput struct {
	Task    string          `json:"task"`
	Units   []designUnitRow `json:"units"`
	Calls   []string        `json:"calls,omitempty"`
	Imports []string        `json:"imports,omitempty"`
}

func (view *designView) partsInput(units []*designUnit) designPartsInput {
	listed := map[string]bool{}
	rowOf := map[string]string{}
	whole := map[string]string{}
	input := designPartsInput{Task: designPartsTask}
	for _, unit := range units {
		listed[unit.ref] = true
		input.Units = append(input.Units, unit.row())
		for _, id := range unit.units {
			rowOf[id] = unit.ref
		}
		if unit.box == "" {
			whole[unit.file] = unit.ref
		}
	}
	input.Calls = pairCounts(view.siteCounts(rowOf), listed)
	for _, pair := range sortedPairs(view.imports) {
		if from, to := whole[pair[0]], whole[pair[1]]; from != "" && to != "" {
			input.Imports = append(input.Imports, from+" -> "+to)
		}
	}
	return input
}

// siteCounts counts, per exact call site, each distinct row other than its
// own that it reaches, by pair of rows; rowOf is each unit's row.
func (view *designView) siteCounts(rowOf map[string]string) map[[2]string]int {
	counts := map[[2]string]int{}
	for _, site := range view.sites {
		from := rowOf[site.from]
		if from == "" {
			continue
		}
		var reached []string
		for _, unit := range site.to {
			if to := rowOf[unit]; to != "" && to != from && !slices.Contains(reached, to) {
				reached = append(reached, to)
				counts[[2]string{from, to}]++
			}
		}
	}
	return counts
}

// pairCounts prints "a -> b (n)" for every pair both of whose ends are kept.
func pairCounts(counts map[[2]string]int, kept map[string]bool) []string {
	pairs := make(map[[2]string]bool, len(counts))
	for pair := range counts {
		pairs[pair] = true
	}
	var result []string
	for _, pair := range sortedPairs(pairs) {
		if kept[pair[0]] && kept[pair[1]] {
			result = append(result, fmt.Sprintf("%s -> %s (%d)", pair[0], pair[1], counts[pair]))
		}
	}
	return result
}

func sortedPairs(pairs map[[2]string]bool) [][2]string {
	result := make([][2]string, 0, len(pairs))
	for pair := range pairs {
		result = append(result, pair)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i][0] != result[j][0] {
			return compactIDLess(result[i][0], result[j][0])
		}
		return compactIDLess(result[i][1], result[j][1])
	})
	return result
}

// partsGroup is one element of a parts answer, read on its own.
type partsGroup struct {
	Name string
	Refs []string
	// Malformed says why the element is not a name with one list of units.
	Malformed string
}

type partsAnswer struct {
	Groups []partsGroup
}

// MarshalJSON writes the answer in its wire shape.
func (answer partsAnswer) MarshalJSON() ([]byte, error) {
	type group struct {
		Name  string   `json:"name"`
		Units []string `json:"units"`
	}
	groups := make([]group, 0, len(answer.Groups))
	for _, g := range answer.Groups {
		groups = append(groups, group{Name: g.Name, Units: g.Refs})
	}
	return json.Marshal(struct {
		Groups []group `json:"groups"`
	}{groups})
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

// decodeParts reads {"groups":[{"name":…,"units":[…]}]} over the unit refs
// one request listed. Every group is read on its own, and extra fields such
// as an "about" are ignored; "files" is the same list as "units" (a form, as
// the answers to the file-only request wrote it), the two given alike are
// one list and given differently refuse that group alone; the list may also
// be one string of refs. Only an answer that draws no part is refused
// whole: it is not JSON, holds no groups, or no group holds a listed unit of
// its own (every ref unknown, such as a path, every group without a name,
// or every unit listed in two different groups).
func decodeParts(raw []byte, listed []string) (partsAnswer, error) {
	var envelope struct {
		Groups json.RawMessage `json:"groups"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return partsAnswer{}, fmt.Errorf("parts: the answer is not a JSON object")
	}
	var elements []json.RawMessage
	if len(envelope.Groups) == 0 || json.Unmarshal(envelope.Groups, &elements) != nil || len(elements) == 0 {
		return partsAnswer{}, fmt.Errorf("parts: the answer has no groups")
	}
	answer := partsAnswer{Groups: make([]partsGroup, 0, len(elements))}
	const notAList = "is not a name with a list of files or units"
	for _, element := range elements {
		var group struct {
			Name  string          `json:"name"`
			Units json.RawMessage `json:"units"`
			Files json.RawMessage `json:"files"`
		}
		if err := json.Unmarshal(element, &group); err != nil {
			answer.Groups = append(answer.Groups, partsGroup{Malformed: notAList})
			continue
		}
		units, unitsOK := refList(group.Units)
		files, filesOK := refList(group.Files)
		switch {
		case !unitsOK || !filesOK:
			answer.Groups = append(answer.Groups, partsGroup{Malformed: notAList})
			continue
		case len(group.Units) > 0 && len(group.Files) > 0 && !sameRefs(units, files):
			answer.Groups = append(answer.Groups, partsGroup{Malformed: "gives units and files that differ"})
			continue
		case len(group.Units) == 0:
			units = files
		}
		answer.Groups = append(answer.Groups, partsGroup{Name: cleanText(group.Name), Refs: units})
	}
	for _, refs := range validatePartition(answer, listed).refs {
		if len(refs) > 0 {
			return answer, nil
		}
	}
	return partsAnswer{}, fmt.Errorf("parts: no group holds a listed unit of its own")
}

// sameRefs says whether two lists hold the same refs, ignoring order,
// repeats and surrounding space.
func sameRefs(left, right []string) bool {
	set := func(refs []string) []string {
		result := make([]string, 0, len(refs))
		for _, ref := range refs {
			result = append(result, strings.TrimSpace(ref))
		}
		sort.Strings(result)
		return slices.Compact(result)
	}
	return slices.Equal(set(left), set(right))
}

// partition is a validated parts answer: independent unit → part rows over
// the units one request listed. Validation annotates; it refuses only the
// memberships that are actually wrong and keeps every good neighbour.
type partition struct {
	// names and refs are the accepted groups in answer order, each with the
	// units it alone holds. A group every unit of which conflicted keeps no
	// unit and is not drawn.
	names []string
	refs  [][]string
	// leftOut are listed units no accepted group holds.
	leftOut []string
	// conflicts are listed units two or more accepted groups hold, with
	// those groups; both memberships are refused, never the first kept.
	conflicts map[string][]int
	// unknown are refs the request did not list; they are discarded.
	unknown []string
	// refused says why a group was not accepted: unreadable, no name or
	// no listed unit. Its units are left out unless another group holds them.
	refused []string
	// repeated are names given to more than one accepted group, ignoring
	// case; each group keeps its own units.
	repeated []string
	// repeatedGroups are groups that restate an earlier accepted group: the
	// same name, ignoring case, over the same set of listed units. The same
	// statement twice is one answer, so the repeat is not drawn again.
	repeatedGroups []string
}

func validatePartition(answer partsAnswer, listed []string) partition {
	known := make(map[string]bool, len(listed))
	for _, ref := range listed {
		known[ref] = true
	}
	result := partition{conflicts: map[string][]int{}}
	holders := map[string][]int{}
	unknown := map[string]bool{}
	var sets [][]string // each accepted group's listed files, sorted
	for position, group := range answer.Groups {
		switch {
		case group.Malformed != "":
			result.refused = append(result.refused, fmt.Sprintf("group %d %s", position+1, group.Malformed))
			continue
		case group.Name == "":
			result.refused = append(result.refused, fmt.Sprintf("group %d has no name", position+1))
			continue
		}
		var refs []string
		for _, ref := range group.Refs {
			ref = strings.TrimSpace(ref)
			switch {
			case !known[ref]:
				if !unknown[ref] {
					unknown[ref] = true
					result.unknown = append(result.unknown, ref)
				}
			case !slices.Contains(refs, ref):
				refs = append(refs, ref)
			}
		}
		if len(refs) == 0 {
			result.refused = append(result.refused, fmt.Sprintf("group %q holds no listed unit", group.Name))
			continue
		}
		set := slices.Clone(refs)
		sort.Strings(set)
		if earlier := sameGroup(result.names, sets, group.Name, set); earlier >= 0 {
			result.repeatedGroups = append(result.repeatedGroups, fmt.Sprintf("group %d repeats group %q", position+1, result.names[earlier]))
			continue
		}
		index := len(result.names)
		result.names = append(result.names, group.Name)
		result.refs = append(result.refs, nil)
		sets = append(sets, set)
		for _, ref := range refs {
			holders[ref] = append(holders[ref], index)
		}
	}
	for _, ref := range listed {
		switch len(holders[ref]) {
		case 0:
			result.leftOut = append(result.leftOut, ref)
		case 1:
			part := holders[ref][0]
			result.refs[part] = append(result.refs[part], ref)
		default:
			result.conflicts[ref] = holders[ref]
		}
	}
	seen := map[string]int{}
	for _, name := range result.names {
		seen[strings.ToLower(name)]++
	}
	for _, name := range result.names {
		if seen[strings.ToLower(name)] > 1 && !slices.Contains(result.repeated, name) {
			result.repeated = append(result.repeated, name)
		}
	}
	return result
}

// sameGroup returns the earlier accepted group with the same name, ignoring
// case, and the same set of listed refs, or -1. Equal names alone never make
// two groups one.
func sameGroup(names []string, sets [][]string, name string, set []string) int {
	for i := range names {
		if strings.EqualFold(names[i], name) && slices.Equal(sets[i], set) {
			return i
		}
	}
	return -1
}

// designPartsCall is the parts request of one window of a target's units.
// An answer refused whole leaves the window refused; it is not asked again.
func designPartsCall(input designPartsInput) (llm.Call[partsAnswer], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[partsAnswer]{}, err
	}
	listed := make([]string, len(input.Units))
	for i, unit := range input.Units {
		listed[i] = unit.Ref
	}
	return llm.Call[partsAnswer]{
		State: []byte(designPartsTask),
		Prompt: llm.Prompt{System: designPartsPrompt, User: string(raw), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"groups":[{"name":"Move search","units":["f4","c2"]},{"name":"Board state","units":["f2"]}]}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designOutputTokens(len(input.Units))},
		DecodeValidate: func(raw []byte) (partsAnswer, error) { return decodeParts(raw, listed) },
	}, nil
}

// splitWindow halves a window of units along whole directory subtrees: the
// items are the subtrees and the loose files directly under the deepest
// directory all the units share, halved by unit count in path order, so the
// boxes of one file stay in one window. A single flat directory thus splits
// into contiguous halves. One file does not split.
func splitWindow(files []*designUnit) ([]*designUnit, []*designUnit, bool) {
	if len(files) < 2 {
		return nil, nil, false
	}
	common := strings.Split(files[0].dir, "/")
	if files[0].dir == "." {
		common = nil
	}
	for _, file := range files[1:] {
		parts := strings.Split(file.dir, "/")
		if file.dir == "." {
			parts = nil
		}
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	sorted := slices.Clone(files)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	item := func(file *designUnit) string {
		rest := strings.Split(file.path, "/")[len(common):]
		if len(rest) > 1 {
			return rest[0] + "/"
		}
		return file.path
	}
	var items [][]*designUnit
	for _, file := range sorted {
		if len(items) > 0 && item(items[len(items)-1][0]) == item(file) {
			items[len(items)-1] = append(items[len(items)-1], file)
			continue
		}
		items = append(items, []*designUnit{file})
	}
	if len(items) < 2 {
		return nil, nil, false
	}
	half, best := 0, len(files)
	count := 0
	for i := 0; i < len(items)-1; i++ {
		count += len(items[i])
		if gap := max(count, len(files)-count) - min(count, len(files)-count); gap < best {
			half, best = i+1, gap
		}
	}
	var left, right []*designUnit
	for i, group := range items {
		if i < half {
			left = append(left, group...)
		} else {
			right = append(right, group...)
		}
	}
	return left, right, true
}

// windowTooLarge says whether a prepared parts request cannot be sent whole:
// the provider refused to prepare it or it passes the request envelope.
func windowTooLarge(provider llm.Provider, call llm.Call[partsAnswer]) bool {
	prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
	var resource *llm.ResourceLimitError
	if errors.As(err, &resource) {
		return true
	}
	return err == nil && prepared.Len() > call.Limits.MaxRequestBytes
}

// inputRefused says whether the provider refused a request for its input or
// context size, the one refusal a smaller window of complete files answers.
func inputRefused(err error) bool {
	var resource *llm.ResourceLimitError
	return errors.As(err, &resource) && (resource.Kind == llm.ResourceLimitContextTokens || resource.Kind == llm.ResourceLimitRequestBytes)
}

// partsWindow is one answered window of a target's parts request.
type partsWindow struct {
	units     []*designUnit
	partition partition
	err       error
}

// askParts sends a target's units, split into directory-subtree windows
// only when the request does not fit the provider. Parts never cross
// windows and no unit is sampled or left unasked.
func (r *reader) askParts(ctx context.Context, view *designView, round int, units []*designUnit) ([]partsWindow, error) {
	if _, started := r.started[lines.StageZones]; !started {
		r.started[lines.StageZones] = time.Now()
	}
	executor := debugdump.BindStage(r.opts.Executor, lines.StageZones)
	provider := r.opts.Provider
	use := r.use(lines.StageZones)
	queue := [][]*designUnit{units}
	var answered []partsWindow
	index := 0
	for len(queue) > 0 {
		var windows [][]*designUnit
		var calls []llm.Call[partsAnswer]
		var next [][]*designUnit
		for _, files := range queue {
			call, err := designPartsCall(view.partsInput(files))
			if err != nil {
				return nil, err
			}
			if left, right, ok := splitWindow(files); ok {
				recalled, err := llm.RecallAdaptiveSplit(executor, provider, call)
				if err != nil {
					return nil, err
				}
				if recalled || windowTooLarge(provider, call) {
					next = append(next, left, right)
					continue
				}
			}
			windows = append(windows, files)
			calls = append(calls, call)
		}
		results := llm.ExecuteJSONEach(ctx, executor, provider, calls)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for i, result := range results {
			if result.Err != nil && inputRefused(result.Err) {
				if left, right, ok := splitWindow(windows[i]); ok {
					if _, err := llm.RememberAdaptiveSplit(executor, provider, calls[i], result.Outcome, result.Err); err != nil {
						return nil, err
					}
					next = append(next, left, right)
					continue
				}
			}
			window := table.Window{Stage: lines.StageZones, Round: round, Index: index}
			index++
			answer := partsWindow{units: windows[i], err: result.Err}
			use.Windows++
			use.Rows += len(windows[i])
			if result.Outcome.Cached {
				use.Cached++
			} else {
				use.Live++
			}
			responseRef := path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
			fmt.Fprintf(&r.tables, "## %s · round %d · window %d · %s\n\n", lines.StageZones, round, window.Index, path.Join(atlas.TablesDir, r.windowFileName(window, "request.ref.json")))
			// Validation records each annotation of an accepted answer as a
			// rejected row that points at this window; an answer so annotated,
			// like a refused one, keeps its payloads in the run.
			recorded := len(r.rejected)
			if result.Err == nil {
				listed := make([]string, len(windows[i]))
				for j, unit := range windows[i] {
					listed[j] = unit.ref
				}
				answer.partition = validatePartition(result.Outcome.Value, listed)
				r.recordPartition(view.targetID, answer.partition, responseRef, len(windows[i]))
			}
			if err := r.writeWindowExchange(window, []byte(calls[i].Prompt.System), []byte(calls[i].Prompt.User), result.Outcome.Request, result.Outcome.Response, result.Err != nil || len(result.Outcome.ResponseRejections) > 0 || len(r.rejected) > recorded); err != nil {
				return nil, err
			}
			if result.Err != nil {
				use.Rejected++
				// A refusal that left no response has no response ref to name.
				if len(result.Outcome.Response) == 0 {
					responseRef = ""
				}
				r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Target: view.targetID, Kind: "window_rejected", Count: len(windows[i]), Reason: result.Err.Error(), ResponseRef: responseRef})
				fmt.Fprintf(&r.tables, "%d units · parts answer refused: %s\n\n", len(windows[i]), result.Err)
				answered = append(answered, answer)
				continue
			}
			raw, err := json.MarshalIndent(result.Outcome.Value, "", "  ")
			if err != nil {
				return nil, err
			}
			if err := r.writeWindowFile(window, "result.json", raw); err != nil {
				return nil, err
			}
			answered = append(answered, answer)
		}
		queue = next
	}
	return answered, nil
}

// recordPartition prints what one window's validation kept and annotated,
// and records every annotation without refusing the answer.
func (r *reader) recordPartition(targetID string, result partition, responseRef string, listed int) {
	drawn := 0
	for i, name := range result.names {
		if len(result.refs[i]) == 0 {
			continue
		}
		drawn++
		fmt.Fprintf(&r.tables, "- %s: %s\n", name, strings.Join(result.refs[i], " "))
	}
	note := func(kind string, samples []string, reason string) {
		if len(samples) == 0 {
			return
		}
		r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Target: targetID, Kind: kind, Count: len(samples), Samples: samples, Reason: reason, ResponseRef: responseRef})
		fmt.Fprintf(&r.tables, "- %s: %s\n", reason, strings.Join(samples, " "))
	}
	var conflicts []string
	for ref := range result.conflicts {
		conflicts = append(conflicts, ref)
	}
	sort.Slice(conflicts, func(i, j int) bool { return compactIDLess(conflicts[i], conflicts[j]) })
	note("part_conflict", conflicts, "listed in two parts; asked again")
	note("part_left_out", result.leftOut, "in no part; asked again")
	note("part_unknown_ref", result.unknown, "refs the request did not list, discarded")
	note("part_refused_group", result.refused, "groups not drawn")
	note("part_repeated_name", result.repeated, "names given to two parts, each kept")
	note("part_repeated_group", result.repeatedGroups, "groups given twice, drawn once")
	switch {
	case drawn == 1 && listed > 1:
		fmt.Fprintf(&r.tables, "- one part holds every placed unit; accepted as returned\n")
	case drawn > 1 && drawn == listed:
		fmt.Fprintf(&r.tables, "- one part per unit; accepted as returned\n")
	}
	r.tables.WriteString("\n")
}

// designPart is one drawn part of a target before it is a box: the units
// of the rows its refs name.
type designPart struct {
	id, name string
	refs     []string
}

// designOutcome is a target's map of parts once the answer, the follow-up
// and the membership rules have been applied.
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
}

func (r *reader) readDesign(ctx context.Context) error {
	r.opts.Stage(lines.StageZones, "splitting each target's files into boxes, grouping the files and boxes into parts, placing what the answer left out, then describing the parts")
	r.boxes = map[string]*boxState{}
	r.designBoxOf = map[string]map[string]string{}
	r.offMap = map[string][]offMapEntry{}
	r.mapFailure = map[string]string{}
	if r.designSubjects == nil {
		r.designSubjects = map[string]string{}
	}
	targets := r.opts.Targets
	// The views are built for every target first; the targets below only
	// read them and the maps they fill.
	views := make([]*designView, len(targets))
	for position, target := range targets {
		views[position] = r.designView(target.ID)
		r.designBoxOf[target.ID] = map[string]string{}
	}
	// Every target is read on its own view at once. Its parts take their
	// compact IDs in target order, the one place a target waits for the ones
	// before it. The first failure cancels the other targets and is the
	// error returned; nothing a view did reaches the reader then.
	order := &designOrder{parts: newDesignTurns(len(targets))}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	readers := make([]*reader, len(targets))
	var failed sync.Once
	var failure error
	var wg sync.WaitGroup
	for position := range targets {
		readers[position] = r.view(nil)
		wg.Add(1)
		go func(position int) {
			defer wg.Done()
			if err := readers[position].designTarget(ctx, r, order, position, views[position]); err != nil {
				failed.Do(func() {
					failure = err
					cancel()
				})
			}
		}(position)
	}
	wg.Wait()
	if failure != nil {
		return failure
	}
	for _, view := range readers {
		r.joinView(view)
	}
	for _, stage := range []string{lines.StageRoleGate, lines.StageRoleBoxes, lines.StageRoleAssign} {
		if _, asked := r.uses[stage]; asked {
			r.reportStage(stage)
		}
	}
	r.reportStage(lines.StageZones)
	r.reportStage(lines.StagePlacement)
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
	var splits map[string]*roleSplit
	if !r.dry {
		var err error
		if splits, err = r.readRoles(ctx, view, round); err != nil {
			return err
		}
	}
	units := view.groupingUnits(splits, outcome)
	var drafts []designPart
	conflicts := map[string][]int{}
	var leftOut []string
	switch {
	case len(units) == 0:
		// A target without code has a legitimate empty map.
	case len(units) == 1:
		// Splitting one unit among parts is not a decision: the one part
		// takes the target's name.
		drafts = []designPart{{name: target.Name, refs: []string{units[0].ref}}}
	case r.dry:
		outcome.failure = atlas.MapFailureNoModel
	default:
		windows, err := r.askParts(ctx, view, round, units)
		if err != nil {
			return err
		}
		refused := 0
		for _, window := range windows {
			if window.err != nil {
				refused++
				for _, unit := range window.units {
					leftOut = append(leftOut, unit.ref)
				}
				continue
			}
			drawnIndex := map[int]int{}
			for i, name := range window.partition.names {
				if len(window.partition.refs[i]) == 0 {
					continue
				}
				drawnIndex[i] = len(drafts)
				drafts = append(drafts, designPart{name: name, refs: window.partition.refs[i]})
			}
			leftOut = append(leftOut, window.partition.leftOut...)
			for ref, holders := range window.partition.conflicts {
				var drawn []int
				for _, holder := range holders {
					if at, ok := drawnIndex[holder]; ok {
						drawn = append(drawn, at)
					}
				}
				if len(drawn) == 0 {
					leftOut = append(leftOut, ref)
					continue
				}
				conflicts[ref] = drawn
			}
		}
		if refused == len(windows) {
			outcome.failure = atlas.MapFailureRefused
			drafts, leftOut, conflicts = nil, nil, map[string][]int{}
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
	// A conflict is offered only the drawn parts that listed it.
	offered := map[string][]string{}
	for ref, holders := range conflicts {
		for _, holder := range holders {
			offered[ref] = append(offered[ref], drafts[holder].id)
		}
	}
	if err := r.placeUnits(ctx, view, round, outcome, units, leftOut, offered); err != nil {
		return err
	}
	r.drawParts(view, outcome, units)
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
	owner.offMap[target.ID] = r.offMapEntries(view, outcome)
	if outcome.failure != "" {
		owner.mapFailure[target.ID] = outcome.failure
	}
	return nil
}

// placeUnits asks one closed-choice follow-up for the units the answer left
// out or listed in two parts, when at least one part was drawn. An unknown,
// missing or refused choice leaves the unit's declarations off the map with
// its reason; a box left out leaves its file on the map through its other
// boxes.
func (r *reader) placeUnits(ctx context.Context, view *designView, round int, outcome *designOutcome, units []*designUnit, leftOut []string, conflicts map[string][]string) error {
	reason := map[string]string{}
	for _, ref := range leftOut {
		reason[ref] = atlas.OffMapLeftOut
	}
	for ref := range conflicts {
		reason[ref] = atlas.OffMapConflict
	}
	var asked []*designUnit
	for _, unit := range units {
		if reason[unit.ref] != "" {
			asked = append(asked, unit)
		}
	}
	defer func() {
		for _, unit := range asked {
			if outcome.partOf[unit.ref] == "" {
				for _, id := range unit.units {
					outcome.unitReason[id] = reason[unit.ref]
				}
			}
		}
	}()
	if len(asked) == 0 || len(outcome.parts) == 0 {
		return nil
	}
	if _, started := r.started[lines.StagePlacement]; !started {
		r.started[lines.StagePlacement] = time.Now()
	}
	byRef := map[string]*designUnit{}
	rowOf := map[string]string{}
	whole := map[string]string{}
	for _, unit := range units {
		byRef[unit.ref] = unit
		for _, id := range unit.units {
			rowOf[id] = unit.ref
		}
		if unit.box == "" {
			whole[unit.file] = unit.ref
		}
	}
	var catalogue []map[string]any
	var all []string
	for _, part := range outcome.parts {
		var dirs []string
		for _, ref := range part.refs {
			dirs = appendUnique(dirs, byRef[ref].dir)
		}
		sort.Strings(dirs)
		catalogue = append(catalogue, map[string]any{"ref": part.id, "name": part.name, "dirs": dirs})
		all = append(all, part.id)
	}
	// A call site reaches a part through the rows it reaches there.
	counts := view.siteCounts(rowOf)
	rows := make([]table.Row, 0, len(asked))
	offered := make([][]string, 0, len(asked))
	for _, unit := range asked {
		options := all
		if holders, ok := conflicts[unit.ref]; ok {
			options = holders
		}
		fields := []table.Field{{Name: "path", Value: unit.path}}
		if unit.box != "" {
			fields = append(fields, table.Field{Name: "box", Value: unit.box})
		}
		fields = append(fields, table.Field{Name: "units", Value: len(unit.units)})
		if len(unit.types) > 0 {
			fields = append(fields, table.Field{Name: "types", Value: unit.types})
		}
		if len(unit.functions) > 0 {
			fields = append(fields, table.Field{Name: "functions", Value: unit.functions})
		}
		if len(unit.variables) > 0 {
			fields = append(fields, table.Field{Name: "variables", Value: unit.variables})
		}
		out, in := map[string]int{}, map[string]int{}
		for pair, count := range counts {
			if pair[0] == unit.ref && outcome.partOf[pair[1]] != "" {
				out[outcome.partOf[pair[1]]] += count
			}
			if pair[1] == unit.ref && outcome.partOf[pair[0]] != "" {
				in[outcome.partOf[pair[0]]] += count
			}
		}
		var calls []string
		for _, part := range all {
			if out[part] > 0 {
				calls = append(calls, fmt.Sprintf("-> %s (%d)", part, out[part]))
			}
		}
		for _, part := range all {
			if in[part] > 0 {
				calls = append(calls, fmt.Sprintf("%s -> (%d)", part, in[part]))
			}
		}
		if len(calls) > 0 {
			fields = append(fields, table.Field{Name: "calls", Value: calls})
		}
		// Imports are between whole files: a box has none of its own.
		imports, importedBy := map[string]bool{}, map[string]bool{}
		if unit.box == "" {
			for pair := range view.imports {
				if pair[0] == unit.file && outcome.partOf[whole[pair[1]]] != "" {
					imports[outcome.partOf[whole[pair[1]]]] = true
				}
				if pair[1] == unit.file && outcome.partOf[whole[pair[0]]] != "" {
					importedBy[outcome.partOf[whole[pair[0]]]] = true
				}
			}
		}
		var importLines []string
		for _, part := range all {
			if imports[part] {
				importLines = append(importLines, "-> "+part)
			}
		}
		for _, part := range all {
			if importedBy[part] {
				importLines = append(importLines, part+" ->")
			}
		}
		if len(importLines) > 0 {
			fields = append(fields, table.Field{Name: "imports", Value: importLines})
		}
		fields = append(fields, table.Field{Name: "part_options", Value: options})
		rows = append(rows, table.Row{ID: unit.ref, Fields: fields})
		offered = append(offered, options)
	}
	r.opts.Stage(lines.StagePlacement, fmt.Sprintf("%s: placing %d files and boxes the parts answer left out or listed twice", r.opts.Targets[round-1].Name, len(rows)))
	answers, err := r.runTableWith(ctx, lines.Placement(), round, []table.Field{{Name: "parts", Value: catalogue}}, rows, nil)
	if err != nil {
		return err
	}
	position := map[string]int{}
	for i, unit := range units {
		position[unit.ref] = i
	}
	for i, unit := range asked {
		choice := ""
		if answers[i].answer != nil {
			choice = answers[i].answer["part"]
		}
		if !slices.Contains(offered[i], choice) {
			continue
		}
		outcome.partOf[unit.ref] = choice
		for _, part := range outcome.parts {
			if part.id == choice {
				part.refs = append(part.refs, unit.ref)
			}
		}
	}
	for _, part := range outcome.parts {
		sort.Slice(part.refs, func(i, j int) bool { return position[part.refs[i]] < position[part.refs[j]] })
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
		box := &boxState{id: part.id, targetID: view.targetID, title: part.name, open: true, dir: ".", symbols: map[string]bool{}, test: true}
		for _, ref := range part.refs {
			unit := byRef[ref]
			box.unitIDs = append(box.unitIDs, unit.units...)
			box.sources = appendUnique(box.sources, unit.file)
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

// describeParts asks one description per drawn part that is not made only of
// test code, all at once. A refused description leaves the part in the
// explicit no-description state; nothing fills it in.
func (r *reader) describeParts(ctx context.Context, view *designView, round int, outcome *designOutcome) error {
	if r.dry {
		return nil
	}
	var boxes []*boxState
	var calls []llm.Call[description]
	for _, box := range outcome.boxes {
		if box.offCanvas() {
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
