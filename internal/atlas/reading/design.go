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
)

//go:embed prompts/design_parts.md
var designPartsPrompt string

//go:embed prompts/design_describe.md
var designDescribePrompt string

//go:embed prompts/design_areas.md
var designAreasPrompt string

const (
	designPartsTask    = "repomap.atlas.parts.v1"
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

// designFile is one row of a target's parts request: a file of the target
// that holds at least one unit of its own.
type designFile struct {
	id, path, dir   string
	test, generated bool
	// units are the symbol places of the file's units: a function, a
	// variable, a type together with its methods, or a module body. Their
	// part is the file's part.
	units                       []string
	types, functions, variables []string
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
	// unitFile is the file whose part a unit takes.
	unitFile map[string]string
	// calls counts exact call sites between listed files, once per distinct
	// pair of files; imports are the imports between listed files.
	calls   map[[2]string]int
	imports map[[2]string]bool
}

// designView reads one target's files and declarations. Every declaration
// with a symbol place is either a unit of its file or follows one: a method
// its type, a lexical child the callable that encloses it.
func (r *reader) designView(targetID string) *designView {
	view := &designView{
		targetID: targetID, byID: map[string]*designFile{}, decls: map[string][]string{},
		follows: map[string]string{}, unitFile: map[string]string{}, calls: map[[2]string]int{}, imports: map[[2]string]bool{},
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
			r.designFiles[id] = file.ID
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
			text := decl.Name
			if decl.Exported && decl.Signature != "" {
				text += " " + decl.Signature
			}
			switch decl.Kind {
			case "type":
				row.types = appendUnique(row.types, text)
			case "variable":
				row.variables = appendUnique(row.variables, text)
			case "module":
				// A module body counts as a unit; its name repeats the path.
			default:
				row.functions = appendUnique(row.functions, text)
			}
		}
		if len(row.units) > 0 {
			view.files = append(view.files, row)
			view.byID[file.ID] = row
		}
	}
	// A declaration counts from its own file when that file is listed, else
	// from the file of the unit it follows.
	listedFile := func(id string) string {
		if file := r.designFiles[id]; view.byID[file] != nil {
			return file
		}
		return view.unitFile[view.root(id)]
	}
	for _, file := range view.all {
		for _, id := range view.decls[file] {
			symbol := r.places[id].Symbol
			if symbol == nil {
				continue
			}
			from := listedFile(id)
			if from == "" {
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
					if to := listedFile(callee); to != "" && to != from && !slices.Contains(reached, to) {
						reached = append(reached, to)
					}
				}
				for _, to := range reached {
					view.calls[[2]string{from, to}]++
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

// designFileRow is one file as the parts and placement requests show it.
type designFileRow struct {
	Ref       string   `json:"ref"`
	Path      string   `json:"path"`
	Units     int      `json:"units"`
	Types     []string `json:"types,omitempty"`
	Functions []string `json:"functions,omitempty"`
	Variables []string `json:"variables,omitempty"`
}

func (file *designFile) row() designFileRow {
	return designFileRow{Ref: file.id, Path: file.path, Units: len(file.units), Types: file.types, Functions: file.functions, Variables: file.variables}
}

// designPartsInput is the parts request: the listed files with the calls and
// imports among them, aggregated over refs the request advertises.
type designPartsInput struct {
	Task    string          `json:"task"`
	Files   []designFileRow `json:"files"`
	Calls   []string        `json:"calls,omitempty"`
	Imports []string        `json:"imports,omitempty"`
}

func (view *designView) partsInput(files []*designFile) designPartsInput {
	listed := map[string]bool{}
	input := designPartsInput{Task: designPartsTask}
	for _, file := range files {
		listed[file.id] = true
		input.Files = append(input.Files, file.row())
	}
	input.Calls = pairCounts(view.calls, listed)
	for _, pair := range sortedPairs(view.imports) {
		if listed[pair[0]] && listed[pair[1]] {
			input.Imports = append(input.Imports, pair[0]+" -> "+pair[1])
		}
	}
	return input
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
	Name  string
	Files []string
	// Malformed says the element was not a name with a list of files.
	Malformed bool
}

type partsAnswer struct {
	Groups []partsGroup
}

// MarshalJSON writes the answer in its wire shape.
func (answer partsAnswer) MarshalJSON() ([]byte, error) {
	type group struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
	}
	groups := make([]group, 0, len(answer.Groups))
	for _, g := range answer.Groups {
		groups = append(groups, group{Name: g.Name, Files: g.Files})
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

// decodeParts reads {"groups":[{"name":…,"files":[…]}]} over the file refs
// one request listed. Every group is read on its own, and extra fields such
// as an "about" are ignored; "files" may also be one string of refs. Only an
// answer that draws no part is refused whole: it is not JSON, holds no
// groups, or no group holds a listed file of its own (every ref unknown, such
// as a path, every group without a name, or every file listed in two
// different groups).
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
	for _, element := range elements {
		var group struct {
			Name  string          `json:"name"`
			Files json.RawMessage `json:"files"`
		}
		if err := json.Unmarshal(element, &group); err != nil {
			answer.Groups = append(answer.Groups, partsGroup{Malformed: true})
			continue
		}
		files, ok := refList(group.Files)
		if !ok {
			answer.Groups = append(answer.Groups, partsGroup{Malformed: true})
			continue
		}
		answer.Groups = append(answer.Groups, partsGroup{Name: cleanText(group.Name), Files: files})
	}
	for _, files := range validatePartition(answer, listed).files {
		if len(files) > 0 {
			return answer, nil
		}
	}
	return partsAnswer{}, fmt.Errorf("parts: no group holds a listed file of its own")
}

// partition is a validated parts answer: independent file → part rows over
// the files one request listed. Validation annotates; it refuses only the
// memberships that are actually wrong and keeps every good neighbour.
type partition struct {
	// names and files are the accepted groups in answer order, each with the
	// files it alone holds. A group every file of which conflicted keeps no
	// file and is not drawn.
	names []string
	files [][]string
	// leftOut are listed files no accepted group holds.
	leftOut []string
	// conflicts are listed files two or more accepted groups hold, with
	// those groups; both memberships are refused, never the first kept.
	conflicts map[string][]int
	// unknown are refs the request did not list; they are discarded.
	unknown []string
	// refused says why a group was not accepted: unreadable, no name or
	// no listed file. Its files are left out unless another group holds them.
	refused []string
	// repeated are names given to more than one accepted group, ignoring
	// case; each group keeps its own files.
	repeated []string
	// repeatedGroups are groups that restate an earlier accepted group: the
	// same name, ignoring case, over the same set of listed files. The same
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
		case group.Malformed:
			result.refused = append(result.refused, fmt.Sprintf("group %d is not a name with a list of files", position+1))
			continue
		case group.Name == "":
			result.refused = append(result.refused, fmt.Sprintf("group %d has no name", position+1))
			continue
		}
		var files []string
		for _, ref := range group.Files {
			ref = strings.TrimSpace(ref)
			switch {
			case !known[ref]:
				if !unknown[ref] {
					unknown[ref] = true
					result.unknown = append(result.unknown, ref)
				}
			case !slices.Contains(files, ref):
				files = append(files, ref)
			}
		}
		if len(files) == 0 {
			result.refused = append(result.refused, fmt.Sprintf("group %q holds no listed file", group.Name))
			continue
		}
		set := slices.Clone(files)
		sort.Strings(set)
		if earlier := sameGroup(result.names, sets, group.Name, set); earlier >= 0 {
			result.repeatedGroups = append(result.repeatedGroups, fmt.Sprintf("group %d repeats group %q", position+1, result.names[earlier]))
			continue
		}
		index := len(result.names)
		result.names = append(result.names, group.Name)
		result.files = append(result.files, nil)
		sets = append(sets, set)
		for _, ref := range files {
			holders[ref] = append(holders[ref], index)
		}
	}
	for _, ref := range listed {
		switch len(holders[ref]) {
		case 0:
			result.leftOut = append(result.leftOut, ref)
		case 1:
			part := holders[ref][0]
			result.files[part] = append(result.files[part], ref)
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

// designPartsCall is the parts request of one window of a target's files. An
// answer refused whole leaves the window refused; it is not asked again.
func designPartsCall(input designPartsInput) (llm.Call[partsAnswer], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[partsAnswer]{}, err
	}
	listed := make([]string, len(input.Files))
	for i, file := range input.Files {
		listed[i] = file.Ref
	}
	return llm.Call[partsAnswer]{
		State: []byte(designPartsTask),
		Prompt: llm.Prompt{System: designPartsPrompt, User: string(raw), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"groups":[{"name":"Move search","files":["f4","f9"]},{"name":"Board state","files":["f2"]}]}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designOutputTokens(len(input.Files))},
		DecodeValidate: func(raw []byte) (partsAnswer, error) { return decodeParts(raw, listed) },
	}, nil
}

// splitWindow halves a window of files along whole directory subtrees: the
// items are the subtrees and the loose files directly under the deepest
// directory all the files share, halved by file count in path order. A
// single flat directory thus splits into contiguous halves. One file does
// not split.
func splitWindow(files []*designFile) ([]*designFile, []*designFile, bool) {
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
	item := func(file *designFile) string {
		rest := strings.Split(file.path, "/")[len(common):]
		if len(rest) > 1 {
			return rest[0] + "/"
		}
		return file.path
	}
	var items [][]*designFile
	for _, file := range sorted {
		if len(items) > 0 && item(items[len(items)-1][0]) == item(file) {
			items[len(items)-1] = append(items[len(items)-1], file)
			continue
		}
		items = append(items, []*designFile{file})
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
	var left, right []*designFile
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
	files     []*designFile
	partition partition
	err       error
}

// askParts sends a target's files, split into directory-subtree windows only
// when the request does not fit the provider. Parts never cross windows and
// no file is sampled or left unasked.
func (r *reader) askParts(ctx context.Context, view *designView, round int) ([]partsWindow, error) {
	executor := debugdump.BindStage(r.opts.Executor, lines.StageZones)
	provider := r.opts.Provider
	use := r.use(lines.StageZones)
	queue := [][]*designFile{view.files}
	var answered []partsWindow
	index := 0
	for len(queue) > 0 {
		var windows [][]*designFile
		var calls []llm.Call[partsAnswer]
		var next [][]*designFile
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
			answer := partsWindow{files: windows[i], err: result.Err}
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
				for j, file := range windows[i] {
					listed[j] = file.id
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
				fmt.Fprintf(&r.tables, "%d files · parts answer refused: %s\n\n", len(windows[i]), result.Err)
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
		if len(result.files[i]) == 0 {
			continue
		}
		drawn++
		fmt.Fprintf(&r.tables, "- %s: %s\n", name, strings.Join(result.files[i], " "))
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
		fmt.Fprintf(&r.tables, "- one part holds every placed file; accepted as returned\n")
	case drawn > 1 && drawn == listed:
		fmt.Fprintf(&r.tables, "- one part per file; accepted as returned\n")
	}
	r.tables.WriteString("\n")
}

// designPart is one drawn part of a target before it is a box.
type designPart struct {
	id    string
	name  string
	files []string // row files placed in it
}

// designOutcome is a target's map of parts once the answer, the follow-up
// and the membership rules have been applied.
type designOutcome struct {
	parts      []*designPart
	partOf     map[string]string // row file → part ID
	offReason  map[string]string // file → why it is off the map
	failure    string
	boxes      []*boxState
	membership map[string]string // declaration or file place → part ID
}

func (r *reader) readDesign(ctx context.Context) error {
	r.started[lines.StageZones] = time.Now()
	r.opts.Stage(lines.StageZones, "grouping each target's files into parts, placing what the answer left out, then describing the parts")
	r.boxes = map[string]*boxState{}
	r.designBoxOf = map[string]map[string]string{}
	r.offMap = map[string][]offMapEntry{}
	r.mapFailure = map[string]string{}
	if r.designFiles == nil {
		r.designFiles = map[string]string{}
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
// owner, the reader the views share.
func (r *reader) designTarget(ctx context.Context, owner *reader, order *designOrder, position int, view *designView) error {
	defer order.parts.pass(position)
	target := r.opts.Targets[position]
	round := position + 1
	outcome := &designOutcome{partOf: map[string]string{}, offReason: map[string]string{}, membership: map[string]string{}}
	var drafts []designPart
	conflicts := map[string][]int{}
	var leftOut []string
	switch {
	case len(view.files) == 0:
		// A target without code has a legitimate empty map.
	case len(view.files) == 1:
		// Splitting one file is not a decision: the one part takes the
		// target's name.
		drafts = []designPart{{name: target.Name, files: []string{view.files[0].id}}}
	case r.dry:
		outcome.failure = atlas.MapFailureNoModel
	default:
		windows, err := r.askParts(ctx, view, round)
		if err != nil {
			return err
		}
		refused := 0
		for _, window := range windows {
			if window.err != nil {
				refused++
				for _, file := range window.files {
					leftOut = append(leftOut, file.id)
				}
				continue
			}
			drawnIndex := map[int]int{}
			for i, name := range window.partition.names {
				if len(window.partition.files[i]) == 0 {
					continue
				}
				drawnIndex[i] = len(drafts)
				drafts = append(drafts, designPart{name: name, files: window.partition.files[i]})
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
	// The parts take their compact IDs in target order.
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
		for _, file := range part.files {
			outcome.partOf[file] = part.id
		}
	}
	if err := r.placeFiles(ctx, view, round, outcome, leftOut, conflicts); err != nil {
		return err
	}
	r.drawParts(view, outcome)
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

// placeFiles asks one closed-choice follow-up for the files the answer left
// out or listed in two parts, when at least one part was drawn. An unknown,
// missing or refused choice leaves the file off the map with its reason.
func (r *reader) placeFiles(ctx context.Context, view *designView, round int, outcome *designOutcome, leftOut []string, conflicts map[string][]int) error {
	reason := map[string]string{}
	for _, ref := range leftOut {
		reason[ref] = atlas.OffMapLeftOut
	}
	for ref := range conflicts {
		reason[ref] = atlas.OffMapConflict
	}
	var asked []string
	for _, file := range view.files {
		if reason[file.id] != "" {
			asked = append(asked, file.id)
		}
	}
	defer func() {
		for _, ref := range asked {
			if outcome.partOf[ref] == "" {
				outcome.offReason[ref] = reason[ref]
			}
		}
	}()
	if len(asked) == 0 || len(outcome.parts) == 0 {
		return nil
	}
	if _, started := r.started[lines.StagePlacement]; !started {
		r.started[lines.StagePlacement] = time.Now()
	}
	partDirs := map[string][]string{}
	var catalogue []map[string]any
	var all []string
	for _, part := range outcome.parts {
		var dirs []string
		for _, file := range part.files {
			dirs = appendUnique(dirs, view.byID[file].dir)
		}
		sort.Strings(dirs)
		partDirs[part.id] = dirs
		catalogue = append(catalogue, map[string]any{"ref": part.id, "name": part.name, "dirs": dirs})
		all = append(all, part.id)
	}
	rows := make([]table.Row, 0, len(asked))
	offered := make([][]string, 0, len(asked))
	for _, ref := range asked {
		file := view.byID[ref]
		options := all
		if holders, ok := conflicts[ref]; ok {
			options = nil
			for _, holder := range holders {
				options = append(options, outcome.parts[holder].id)
			}
		}
		fields := []table.Field{{Name: "path", Value: file.path}, {Name: "units", Value: len(file.units)}}
		if len(file.types) > 0 {
			fields = append(fields, table.Field{Name: "types", Value: file.types})
		}
		if len(file.functions) > 0 {
			fields = append(fields, table.Field{Name: "functions", Value: file.functions})
		}
		if len(file.variables) > 0 {
			fields = append(fields, table.Field{Name: "variables", Value: file.variables})
		}
		out, in := map[string]int{}, map[string]int{}
		for pair, count := range view.calls {
			if pair[0] == ref && outcome.partOf[pair[1]] != "" {
				out[outcome.partOf[pair[1]]] += count
			}
			if pair[1] == ref && outcome.partOf[pair[0]] != "" {
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
		imports, importedBy := map[string]bool{}, map[string]bool{}
		for pair := range view.imports {
			if pair[0] == ref && outcome.partOf[pair[1]] != "" {
				imports[outcome.partOf[pair[1]]] = true
			}
			if pair[1] == ref && outcome.partOf[pair[0]] != "" {
				importedBy[outcome.partOf[pair[0]]] = true
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
		rows = append(rows, table.Row{ID: ref, Fields: fields})
		offered = append(offered, options)
	}
	r.opts.Stage(lines.StagePlacement, fmt.Sprintf("%s: placing %d files the parts answer left out or listed twice", r.opts.Targets[round-1].Name, len(rows)))
	answers, err := r.runTableWith(ctx, lines.Placement(), round, []table.Field{{Name: "parts", Value: catalogue}}, rows, nil)
	if err != nil {
		return err
	}
	for i, ref := range asked {
		choice := ""
		if answers[i].answer != nil {
			choice = answers[i].answer["part"]
		}
		if !slices.Contains(offered[i], choice) {
			continue
		}
		outcome.partOf[ref] = choice
		for _, part := range outcome.parts {
			if part.id == choice {
				part.files = append(part.files, ref)
			}
		}
	}
	for _, part := range outcome.parts {
		sort.Slice(part.files, func(i, j int) bool { return compactIDLess(part.files[i], part.files[j]) })
	}
	return nil
}

// drawParts gives every declaration of a placed file its file's part, a
// method its type's part and a lexical child its parent's part, and builds
// the boxes. A part whose every file is test code is a fact, kept in the
// atlas and off the canvas.
func (r *reader) drawParts(view *designView, outcome *designOutcome) {
	boxes := map[string]*boxState{}
	for _, part := range outcome.parts {
		box := &boxState{id: part.id, targetID: view.targetID, title: part.name, open: true, dir: ".", symbols: map[string]bool{}, test: true}
		for _, ref := range part.files {
			box.units += len(view.byID[ref].units)
			box.rows = append(box.rows, ref)
			box.test = box.test && view.byID[ref].test
			outcome.membership[ref] = part.id
		}
		boxes[part.id] = box
	}
	for _, ref := range view.all {
		for _, id := range view.decls[ref] {
			part := outcome.partOf[view.unitFile[view.root(id)]]
			if part == "" {
				continue
			}
			outcome.membership[id] = part
			box := boxes[part]
			box.symbols[id] = true
			if !slices.Contains(box.files, ref) {
				box.files = append(box.files, ref)
			}
		}
	}
	// A file that is not a row declares only what follows a unit of another
	// file, such as a Go method declared outside its type's file. It is on
	// the map through those declarations, and its endpoint is the one part
	// they share. Only a file that declares nothing has no units to place;
	// under a map failure no file is placed.
	for _, ref := range view.all {
		switch {
		case outcome.failure != "":
			outcome.offReason[ref] = atlas.OffMapFailure
		case view.byID[ref] != nil:
		case len(view.decls[ref]) == 0:
			outcome.offReason[ref] = atlas.OffMapNoUnits
		default:
			shared, last := map[string]bool{}, ""
			for _, id := range view.decls[ref] {
				if part := outcome.membership[id]; part != "" {
					shared[part], last = true, part
				}
			}
			if len(shared) == 1 {
				outcome.membership[ref] = last
			}
		}
	}
	for _, part := range outcome.parts {
		box := boxes[part.id]
		for _, ref := range part.files {
			if !slices.Contains(box.files, ref) {
				box.files = append(box.files, ref)
			}
		}
		sort.Slice(box.files, func(i, j int) bool { return compactIDLess(box.files[i], box.files[j]) })
		if len(box.rows) > 0 {
			box.dir = path.Dir(r.places[box.rows[0]].Path)
		}
		box.forTests = box.test && len(box.rows) > 0
		outcome.boxes = append(outcome.boxes, box)
	}
}

// offMapEntries lists every declaration of the target no part holds, under
// its file: an off-map file with its reason, or, in a file a part holds, a
// method whose type is off the map, with that type's reason and the file's
// part. A file every declaration of which a part holds has no entry.
func (r *reader) offMapEntries(view *designView, outcome *designOutcome) []offMapEntry {
	entries := []offMapEntry{}
	for _, ref := range view.all {
		reason := outcome.offReason[ref]
		byReason := map[string][]string{}
		var reasons []string
		for _, id := range view.decls[ref] {
			if outcome.membership[id] != "" {
				continue
			}
			why := reason
			if why == "" {
				why = outcome.offReason[view.unitFile[view.root(id)]]
			}
			if why == "" {
				why = atlas.OffMapNoUnits
			}
			if byReason[why] == nil {
				reasons = append(reasons, why)
			}
			byReason[why] = append(byReason[why], id)
		}
		if reason != "" && byReason[reason] == nil {
			reasons = append([]string{reason}, reasons...)
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

// partDescribeInput lists every unit of a part's files by directory and
// file, with its name and its signature when it has one. No documentation
// is sent.
func (r *reader) partDescribeInput(view *designView, box *boxState) describeInput {
	input := describeInput{Task: designDescribeTask, Part: box.title}
	byDir := map[string]*describeDirectory{}
	var dirs []string
	rows := slices.Clone(box.rows)
	sort.Slice(rows, func(i, j int) bool { return view.byID[rows[i]].path < view.byID[rows[j]].path })
	for _, ref := range rows {
		file := view.byID[ref]
		var members []describeMember
		for _, id := range file.units {
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
		if box.forTests {
			continue
		}
		call, err := describeCall(r.partDescribeInput(view, box))
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

// boundaryBox is the part a boundary stands in: its declaration's part, else
// its file's part. A boundary in a file off the map has none.
func (r *reader) boundaryBox(targetID string, place atlas.Place) string {
	if place.Boundary != nil {
		if id := r.boxFor(targetID, place.Boundary.SubjectID); id != "" {
			return id
		}
		if symbol := r.designSubjects[place.Boundary.ObjectID]; symbol != "" {
			if id := r.boxFor(targetID, symbol); id != "" {
				return id
			}
		}
	}
	return r.boxFor(targetID, place.Parent)
}
