// Package reading walks the places of a repository the way a reader would:
// directories by depth, then files by their distance from the entry points,
// asking the model one table per round and keeping its lines. It prints
// every table it asked so the owner can read the rows, and it folds the
// answers into the atlas the page reads. Without a provider it does the same
// walk with every cell taking its fallback line.
package reading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// TargetMeta names one analyzed target of the run.
type TargetMeta struct {
	ID       string
	Language string
	Kind     string
	Name     string
	Root     string
}

// Options is everything the reading needs.
type Options struct {
	Graph      atlas.Graph
	Targets    []TargetMeta
	Repository string
	Revision   string
	// Executor is the run's executor; the reading binds it to one debugdump
	// stage per table. Provider nil means a dry run: no call is made and
	// every cell takes its fallback.
	Executor llm.Executor
	Provider llm.Provider
	// OwnerRunDir receives tables.md and tables/.
	OwnerRunDir string
	// Stage and State report progress the way the run output does.
	Stage func(name string, details ...string)
	State func(stage, state string, details ...string)
}

// Result is the reading's outcome.
type Result struct {
	Atlas      atlas.Atlas
	Uses       []atlas.StageUse
	Rejected   []modeldiag.Row
	TablesPath string
}

// cell is one model line with where it came from.
type cell struct {
	value  string
	source string
}

type reader struct {
	opts     Options
	places   map[string]atlas.Place
	lines    map[string]cell
	titles   map[string]cell
	boxes    map[string]string // file place ID -> box choice as answered
	uses     map[string]*atlas.StageUse
	rejected []modeldiag.Row
	tables   strings.Builder
	dry      bool
}

// Read performs the walk.
func Read(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(opts.Graph.Places) == 0 {
		return Result{}, fmt.Errorf("atlas reading: the graph has no places")
	}
	if opts.OwnerRunDir == "" {
		return Result{}, fmt.Errorf("atlas reading: owner run dir is required")
	}
	if opts.Stage == nil {
		opts.Stage = func(string, ...string) {}
	}
	if opts.State == nil {
		opts.State = func(string, string, ...string) {}
	}
	r := &reader{
		opts:   opts,
		places: make(map[string]atlas.Place, len(opts.Graph.Places)),
		lines:  make(map[string]cell),
		titles: make(map[string]cell),
		boxes:  make(map[string]string),
		uses:   make(map[string]*atlas.StageUse),
		dry:    opts.Provider == nil,
	}
	for _, place := range opts.Graph.Places {
		r.places[place.ID] = place
	}
	if err := os.MkdirAll(filepath.Join(opts.OwnerRunDir, atlas.TablesDir), 0o700); err != nil {
		return Result{}, fmt.Errorf("atlas reading: prepare %s: %w", atlas.TablesDir, err)
	}
	fmt.Fprintf(&r.tables, "# Atlas tables\n\nrepository: %s\nrevision: %s\nmode: %s\n\n",
		opts.Repository, opts.Revision, map[bool]string{true: "dry, every cell is its fallback", false: "live"}[r.dry])
	if err := r.readDirectories(ctx); err != nil {
		return Result{}, err
	}
	if err := r.readFiles(ctx); err != nil {
		return Result{}, err
	}
	tablesPath := filepath.Join(opts.OwnerRunDir, atlas.TablesFilename)
	if err := os.WriteFile(tablesPath, []byte(r.tables.String()), 0o600); err != nil {
		return Result{}, fmt.Errorf("atlas reading: write %s: %w", atlas.TablesFilename, err)
	}
	result := Result{Atlas: r.atlas(), Rejected: r.rejected, TablesPath: tablesPath}
	for _, use := range r.uses {
		result.Uses = append(result.Uses, *use)
	}
	sort.Slice(result.Uses, func(i, j int) bool { return result.Uses[i].Stage < result.Uses[j].Stage })
	result.Atlas.Budget.Stages = result.Uses
	return result, nil
}

// Line satisfies lines.Lines with the model's lines so far.
func (r *reader) Line(placeID string) (string, bool) {
	line, ok := r.lines[placeID]
	if !ok || line.source == atlas.SourceGiven {
		return "", false
	}
	return line.value, true
}

func (r *reader) readDirectories(ctx context.Context) error {
	def := lines.Directories()
	byDepth := make(map[int][]atlas.Place)
	for _, place := range r.opts.Graph.Places {
		if place.Kind == atlas.PlaceDirectory {
			byDepth[place.Depth] = append(byDepth[place.Depth], place)
		}
	}
	depths := sortedInts(byDepth)
	r.opts.Stage(def.Stage, fmt.Sprintf("%d directories in %d rounds by depth", countPlaces(byDepth), len(depths)))
	started := time.Now()
	for round, depth := range depths {
		dirs := byDepth[depth]
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].Path < dirs[j].Path })
		rows := make([]table.Row, 0, len(dirs))
		for _, place := range dirs {
			var parent *atlas.Place
			if place.Parent != "" {
				if p, ok := r.places[place.Parent]; ok {
					parent = &p
				}
			}
			rows = append(rows, lines.DirectoryRow(place, parent))
		}
		answers, err := r.runTable(ctx, def, round+1, rows)
		if err != nil {
			return err
		}
		for i, row := range rows {
			place := r.places[row.ID]
			if answer := answers[i]; answer.answer != nil {
				r.titles[row.ID] = cell{value: answer.answer["title"], source: answer.source}
				r.lines[row.ID] = cell{value: answer.answer["line"], source: answer.source}
			} else {
				r.titles[row.ID] = cell{value: directoryTitle(place.Path), source: atlas.SourceGiven}
				r.lines[row.ID] = cell{value: place.Given, source: answer.source}
			}
		}
	}
	r.report(def.Stage, started)
	return nil
}

func (r *reader) readFiles(ctx context.Context) error {
	def := lines.Files()
	byDepth := make(map[int][]atlas.Place)
	generated := 0
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceFile {
			continue
		}
		if place.File.Generated {
			generated++
			r.lines[place.ID] = cell{value: place.Given, source: atlas.SourceUnused}
			continue
		}
		byDepth[place.Depth] = append(byDepth[place.Depth], place)
	}
	depths := sortedInts(byDepth)
	details := []string{fmt.Sprintf("%d files in %d rounds by distance from the entry points", countPlaces(byDepth), len(depths))}
	if generated > 0 {
		details = append(details, fmt.Sprintf("generated files left unasked: %d", generated))
	}
	r.opts.Stage(def.Stage, details...)
	started := time.Now()
	siblings := r.siblingBoxes()
	for round, depth := range depths {
		files := byDepth[depth]
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		rows := make([]table.Row, 0, len(files))
		for _, place := range files {
			directory := r.places[place.Parent]
			rows = append(rows, lines.FileRow(place, directory, r.rankedSiblings(place, siblings[directory.Path]), r, r.places))
		}
		answers, err := r.runTable(ctx, def, round+1, rows)
		if err != nil {
			return err
		}
		for i, row := range rows {
			place := r.places[row.ID]
			if answer := answers[i]; answer.answer != nil {
				r.lines[row.ID] = cell{value: answer.answer["line"], source: answer.source}
				r.boxes[row.ID] = answer.answer["box"]
			} else {
				r.lines[row.ID] = cell{value: place.Given, source: answer.source}
				r.boxes[row.ID] = lines.BoxHere
			}
		}
	}
	r.cancelLonelyBoxes()
	r.report(def.Stage, started)
	return nil
}

// siblingBoxes lists, per directory, the paths of its sibling directories
// that hold files: the boxes a file may move to.
func (r *reader) siblingBoxes() map[string][]string {
	children := make(map[string][]string)
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceDirectory || len(place.Directory.Files) == 0 || place.Path == "." {
			continue
		}
		parent := path.Dir(place.Path)
		if parent == "" || parent == "/" {
			parent = "."
		}
		children[parent] = append(children[parent], place.Path)
	}
	result := make(map[string][]string)
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceDirectory || len(place.Directory.Files) == 0 {
			continue
		}
		parent := path.Dir(place.Path)
		if parent == "" || parent == "/" || place.Path == "." {
			parent = "."
		}
		var siblings []string
		for _, sibling := range children[parent] {
			if sibling != place.Path {
				siblings = append(siblings, sibling)
			}
		}
		sort.Strings(siblings)
		result[place.Path] = siblings
	}
	return result
}

// rankedSiblings orders a directory's sibling boxes by how many of the
// file's callers and callees live in them, so the few the row can carry are
// the ones the file might belong with.
func (r *reader) rankedSiblings(place atlas.Place, siblings []string) []string {
	if len(siblings) == 0 {
		return siblings
	}
	weight := make(map[string]int, len(siblings))
	for _, neighbourID := range append(append([]string{}, place.File.Callers...), place.File.Callees...) {
		neighbour, ok := r.places[neighbourID]
		if !ok {
			continue
		}
		weight[path.Dir(neighbour.Path)]++
	}
	ranked := append([]string(nil), siblings...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if weight[ranked[i]] != weight[ranked[j]] {
			return weight[ranked[i]] > weight[ranked[j]]
		}
		return ranked[i] < ranked[j]
	})
	return ranked
}

// cancelLonelyBoxes moves a file back into its directory when it was the only
// one to start a new box: one file is not a box.
func (r *reader) cancelLonelyBoxes() {
	count := make(map[string]int)
	for fileID, choice := range r.boxes {
		if title, ok := table.IsFree(lines.Files().Columns[1], choice); ok {
			place := r.places[fileID]
			count[path.Dir(place.Path)+"#"+atlas.Slug(title)]++
		}
	}
	for fileID, choice := range r.boxes {
		if title, ok := table.IsFree(lines.Files().Columns[1], choice); ok {
			place := r.places[fileID]
			if count[path.Dir(place.Path)+"#"+atlas.Slug(title)] < 2 {
				r.boxes[fileID] = lines.BoxHere
			}
		}
	}
}

// rowAnswer is what one row ends with: the model's cells, or none with the
// source saying why.
type rowAnswer struct {
	answer table.Answer
	source string
}

// runTable asks one round of one table: windows in parallel, each window
// its own request, a rejected window falling back on its own.
func (r *reader) runTable(ctx context.Context, def table.Definition, round int, rows []table.Row) ([]rowAnswer, error) {
	answers := make([]rowAnswer, len(rows))
	windows, err := table.Windows(def, round, rows)
	if err != nil {
		return nil, err
	}
	use := r.use(def.Stage)
	use.Rows += len(rows)
	use.Windows += len(windows)
	for _, window := range windows {
		if err := r.writeWindowFile(window, "request.json", window.Request); err != nil {
			return nil, err
		}
	}
	offsets := make([]int, len(windows))
	offset := 0
	for i, window := range windows {
		offsets[i] = offset
		offset += len(window.Rows)
	}
	if r.dry {
		for i, window := range windows {
			for j := range window.Rows {
				answers[offsets[i]+j] = rowAnswer{source: atlas.SourceGiven}
			}
			use.Given += len(window.Rows)
			r.printWindow(def, window, nil, "dry: fallback lines", 0)
		}
		return answers, nil
	}
	calls := make([]llm.Call[table.Answers], len(windows))
	for i, window := range windows {
		call, err := table.Call(def, window)
		if err != nil {
			return nil, err
		}
		calls[i] = call
	}
	executor := debugdump.BindStage(r.opts.Executor, def.Stage)
	results := llm.ExecuteJSONEach(ctx, executor, r.opts.Provider, calls)
	for i, window := range windows {
		result := results[i]
		if len(result.Outcome.Response) > 0 {
			if err := r.writeWindowFile(window, "response.json", result.Outcome.Response); err != nil {
				return nil, err
			}
		}
		if result.Err == nil {
			source := atlas.SourceModel
			if result.Outcome.Cached {
				source = atlas.SourceCache
				use.Cached++
			} else {
				use.Live++
			}
			for j := range window.Rows {
				answers[offsets[i]+j] = rowAnswer{answer: result.Outcome.Value[j], source: source}
			}
			r.printWindow(def, window, result.Outcome.Value, source, result.Outcome.Metrics.Latency)
			continue
		}
		if errors.Is(result.Err, context.Canceled) || ctx.Err() != nil {
			return nil, result.Err
		}
		use.Rejected++
		use.Given += len(window.Rows)
		if !result.Outcome.Cached {
			use.Live++
		}
		for j := range window.Rows {
			answers[offsets[i]+j] = rowAnswer{source: atlas.SourceGiven}
		}
		reason := result.Err.Error()
		r.rejected = append(r.rejected, modeldiag.Row{
			Stage: def.Stage, Kind: "window_rejected", Count: len(window.Rows),
			Reason: reason, Raw: rawJSON(result.Outcome.Response),
			Samples: []string{fmt.Sprintf("round %d window %d", window.Round, window.Index)},
		})
		r.printWindow(def, window, nil, "rejected: "+reason, result.Outcome.Metrics.Latency)
	}
	return answers, nil
}

func (r *reader) use(stage string) *atlas.StageUse {
	use, ok := r.uses[stage]
	if !ok {
		use = &atlas.StageUse{Stage: stage}
		r.uses[stage] = use
	}
	return use
}

func (r *reader) report(stage string, started time.Time) {
	use := r.use(stage)
	r.opts.State(stage, "ready",
		fmt.Sprintf("rows: %d", use.Rows),
		fmt.Sprintf("windows: %d (live %d, cached %d, rejected %d)", use.Windows, use.Live, use.Cached, use.Rejected),
		fmt.Sprintf("rows on their fallback line: %d", use.Given),
		fmt.Sprintf("duration: %s", time.Since(started).Round(time.Millisecond)),
	)
}

func windowFileName(window table.Window, suffix string) string {
	return fmt.Sprintf("%s-r%d-w%d.%s", window.Stage, window.Round, window.Index, suffix)
}

func (r *reader) writeWindowFile(window table.Window, suffix string, data []byte) error {
	name := filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, windowFileName(window, suffix))
	if err := os.WriteFile(name, data, 0o600); err != nil {
		return fmt.Errorf("atlas reading: write %s: %w", filepath.Base(name), err)
	}
	return nil
}

// printWindow adds one window to tables.md: every row with its key, its
// inputs, its fallback and, after a live run, the model's cells beside it.
func (r *reader) printWindow(def table.Definition, window table.Window, answers table.Answers, source string, latency time.Duration) {
	fmt.Fprintf(&r.tables, "## %s · round %d · window %d · %s\n\n",
		def.Stage, window.Round, window.Index, path.Join(atlas.TablesDir, windowFileName(window, "request.json")))
	fmt.Fprintf(&r.tables, "source: %s", source)
	if latency > 0 {
		fmt.Fprintf(&r.tables, " · %s", latency.Round(time.Millisecond))
	}
	r.tables.WriteString("\n\n")
	for i, row := range window.Rows {
		place := r.places[row.ID]
		fmt.Fprintf(&r.tables, "- %s · %s\n", table.Key(i), row.ID)
		for _, field := range row.Fields {
			encoded, _ := json.Marshal(field.Value)
			fmt.Fprintf(&r.tables, "  - %s: %s\n", field.Name, string(encoded))
		}
		fmt.Fprintf(&r.tables, "  - given: %s\n", place.Given)
		if answers != nil {
			for _, column := range def.Columns {
				fmt.Fprintf(&r.tables, "  - %s → %s\n", column.Name, answers[i][column.Name])
			}
		}
	}
	r.tables.WriteString("\n")
}

func rawJSON(response []byte) json.RawMessage {
	if len(response) == 0 {
		return nil
	}
	if json.Valid(response) {
		return json.RawMessage(response)
	}
	encoded, err := json.Marshal(string(response))
	if err != nil {
		return nil
	}
	return json.RawMessage(encoded)
}

// atlas folds the lines into boxes per target.
func (r *reader) atlas() atlas.Atlas {
	result := atlas.Atlas{
		Version: atlas.Version, Repository: r.opts.Repository, Revision: r.opts.Revision,
		Targets: []atlas.Target{}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{},
	}
	files := 0
	for _, place := range r.opts.Graph.Places {
		if place.Kind == atlas.PlaceFile && !place.File.Generated {
			files++
		}
	}
	result.Budget = atlas.Budget{Files: files, DirsOpened: 0, FilesOpened: files}
	for _, target := range r.opts.Targets {
		result.Targets = append(result.Targets, r.target(target))
	}
	sort.Slice(result.Targets, func(i, j int) bool { return result.Targets[i].Name < result.Targets[j].Name })
	byKind := make(map[string]*atlas.Diagnostic)
	for _, row := range r.rejected {
		key := row.Stage + "/" + row.Kind
		diagnostic, ok := byKind[key]
		if !ok {
			diagnostic = &atlas.Diagnostic{Stage: row.Stage, Kind: row.Kind}
			byKind[key] = diagnostic
		}
		diagnostic.Count += row.Count
		if len(diagnostic.Samples) < modeldiag.MaxSamples {
			diagnostic.Samples = append(diagnostic.Samples, row.Samples...)
		}
	}
	for _, diagnostic := range byKind {
		result.Diagnostics = append(result.Diagnostics, *diagnostic)
	}
	sort.Slice(result.Diagnostics, func(i, j int) bool {
		return result.Diagnostics[i].Stage+result.Diagnostics[i].Kind < result.Diagnostics[j].Stage+result.Diagnostics[j].Kind
	})
	return result
}

func (r *reader) target(meta TargetMeta) atlas.Target {
	target := atlas.Target{
		ID: meta.ID, Language: meta.Language, Kind: meta.Kind, Name: meta.Name, Root: meta.Root,
		Zones: []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{},
		Boundaries: []atlas.Boundary{}, Trace: []string{},
	}
	if line, ok := r.lines[atlas.DirectoryID(meta.Root)]; ok {
		target.Line = line.value
	}
	boxes := make(map[string]*atlas.Box)
	box := func(id, dir, title string) *atlas.Box {
		existing, ok := boxes[id]
		if ok {
			return existing
		}
		created := &atlas.Box{ID: id, Dir: dir, Title: title, Side: atlas.SideMid, Open: true, Files: []atlas.File{}, Keys: []atlas.Key{}}
		if line, ok := r.lines[atlas.DirectoryID(dir)]; ok && id == dir {
			created.Line = line.value
		} else if place, ok := r.places[atlas.DirectoryID(dir)]; ok {
			created.Line = place.Given
		}
		if title == "" {
			if t, ok := r.titles[atlas.DirectoryID(dir)]; ok {
				created.Title = t.value
			} else {
				created.Title = directoryTitle(dir)
			}
		}
		boxes[id] = created
		return created
	}
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceFile || !contains(place.TargetIDs, meta.ID) {
			continue
		}
		dir := path.Dir(place.Path)
		if dir == "" || dir == "/" {
			dir = "."
		}
		boxID, boxDir, title := dir, dir, ""
		switch choice := r.boxes[place.ID]; {
		case choice == "" || choice == lines.BoxHere:
		default:
			if newTitle, ok := table.IsFree(lines.Files().Columns[1], choice); ok {
				boxID, title = dir+"#"+atlas.Slug(newTitle), newTitle
			} else if _, ok := r.places[atlas.DirectoryID(choice)]; ok {
				boxID, boxDir = choice, choice
			}
		}
		owner := box(boxID, boxDir, title)
		if title != "" && owner.Line == "" {
			owner.Line = r.lines[place.ID].value
		}
		line := r.lines[place.ID]
		file := atlas.File{
			Path: place.Path, Line: line.value, Source: line.source, Open: true,
			Asked:   line.source == atlas.SourceModel || line.source == atlas.SourceCache || line.source == atlas.SourceGiven && !place.File.Generated,
			Symbols: []atlas.Symbol{},
			Callers: len(place.File.Callers), Callees: len(place.File.Callees),
		}
		if place.File.Generated {
			file.Asked = false
		}
		for _, decl := range place.File.Decls {
			file.Symbols = append(file.Symbols, atlas.Symbol{
				ID: atlas.SymbolID(place.Path, decl.LineNo, decl.Name), Name: decl.Name, Kind: decl.Kind,
				Signature: decl.Signature, Doc: decl.Doc, LineNo: decl.LineNo, Column: decl.Column,
			})
		}
		owner.Files = append(owner.Files, file)
		target.Files++
		target.Symbols += len(file.Symbols)
	}
	for _, owner := range boxes {
		sort.Slice(owner.Files, func(i, j int) bool { return owner.Files[i].Path < owner.Files[j].Path })
		owner.Keys = rankedKeys(owner.Files)
		target.Boxes = append(target.Boxes, *owner)
	}
	sort.Slice(target.Boxes, func(i, j int) bool { return target.Boxes[i].ID < target.Boxes[j].ID })
	return target
}

// rankedKeys picks a box's key symbols by code: exported and documented
// first, then by name, three per box.
func rankedKeys(files []atlas.File) []atlas.Key {
	type candidate struct {
		key      atlas.Key
		exported bool
	}
	var candidates []candidate
	for _, file := range files {
		for _, symbol := range file.Symbols {
			exported := symbol.Name != "" && strings.ToUpper(symbol.Name[:1]) == symbol.Name[:1]
			if symbol.Doc == "" {
				continue
			}
			candidates = append(candidates, candidate{
				key:      atlas.Key{SymbolID: symbol.ID, Name: symbol.Name, Path: file.Path, Doc: symbol.Doc, LineNo: symbol.LineNo},
				exported: exported,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].exported != candidates[j].exported {
			return candidates[i].exported
		}
		return candidates[i].key.Name < candidates[j].key.Name
	})
	keys := make([]atlas.Key, 0, 3)
	for _, c := range candidates {
		keys = append(keys, c.key)
		if len(keys) == 3 {
			break
		}
	}
	return keys
}

func directoryTitle(dir string) string {
	if dir == "." {
		return "repository root"
	}
	return path.Base(dir)
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func sortedInts(byDepth map[int][]atlas.Place) []int {
	depths := make([]int, 0, len(byDepth))
	for depth := range byDepth {
		depths = append(depths, depth)
	}
	sort.Ints(depths)
	return depths
}

func countPlaces(byDepth map[int][]atlas.Place) int {
	total := 0
	for _, places := range byDepth {
		total += len(places)
	}
	return total
}
