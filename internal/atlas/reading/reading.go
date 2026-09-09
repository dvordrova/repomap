// Package reading walks the places of a repository the way a reader would:
// directories by depth, then independent files with direct caller facts,
// then the boundaries, the parts, the arrows, the portfolio and the joints,
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

// BudgetThreshold is the number of code files above which the model is asked
// where to dig: directories and files get an open cell, and what it leaves
// closed keeps its fallback line.
const BudgetThreshold = 2000

// TargetMeta names one analyzed target of the run.
type TargetMeta struct {
	ID           string   `json:"id"`
	Language     string   `json:"language"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Root         string   `json:"root"`
	SelectedRole string   `json:"selected_role,omitempty"`
	SharedCode   []string `json:"shared_code,omitempty"`
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
	// Budget forces the budget mode regardless of the file count; tests use
	// it. Zero keeps the threshold.
	Budget bool
	// Through stops after this table stage; empty runs the complete reading.
	Through string
	// Table controls apply to Through, or to all stages when Through is empty.
	// Prompt needs an explicit Through stage. Zero limits use the stage defaults.
	Prompt     string
	WindowRows int
	InputBytes int
	// Questions ask independent reading questions over the deterministic
	// graph. Through=atlas_question stops at candidates; atlas_answer answers
	// from those original sources and returns their reading order.
	// These isolated stages do not run the map wording stages.
	Questions []string
	// Learn adapts the base learning intents after the ordinary atlas.
	Learn bool
}

// Result is the reading's outcome.
type Result struct {
	Complete   bool
	Through    string
	Atlas      atlas.Atlas
	Uses       []atlas.StageUse
	Rejected   []modeldiag.Row
	TablesPath string
	Questions  []atlas.QuestionRoute
	Learning   *atlas.LearningPlan
}

// cell is one model line with where it came from.
type cell struct {
	value  string
	source string
}

type reader struct {
	opts      Options
	places    map[string]atlas.Place
	lines     map[string]cell
	titles    map[string]cell
	boxChoice map[string]string // file place ID -> box choice as answered
	openDirs  map[string]bool   // directory place ID -> open, budget mode only
	openFiles map[string]bool   // file place ID -> open, budget mode only
	budget    bool

	symbolLine map[string]cell               // symbol place ID -> model line
	operations map[string][3]string          // symbol ID -> activation, operation name and description
	outbound   map[string][]atlas.SymbolCall // model-selected calls, still anchored in source
	keys       map[string][]string           // file place ID -> key symbol IDs, by rank

	boxOf      map[string]string    // file place ID -> box ID
	boxes      map[string]*boxState // box ID -> box
	zones      map[string][]*zoneState
	arrows     map[string][]*arrowState
	boundaries map[string]*boundaryState
	targets    map[string]*targetState
	joints     []atlas.Joint

	uses              map[string]*atlas.StageUse
	started           map[string]time.Time
	rejected          []modeldiag.Row
	tables            strings.Builder
	dry               bool
	question          *atlas.QuestionRoute
	questions         []atlas.QuestionRoute
	questionText      string
	questionKey       string
	learning          *atlas.LearningPlan
	learningRound     int
	knowledge         map[string]*Knowledge
	knowledgeSubjects map[string]*Knowledge
	responseTables    map[string]rememberedTable
	recallOnly        bool
}

// Read performs the walk.
func Read(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateControls(opts); err != nil {
		return Result{}, err
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
	if err := os.MkdirAll(filepath.Join(opts.OwnerRunDir, atlas.TablesDir), 0o700); err != nil {
		return Result{}, fmt.Errorf("atlas reading: prepare %s: %w", atlas.TablesDir, err)
	}
	graph, err := SaveInput(opts)
	if err != nil {
		return Result{}, err
	}
	opts.Graph = graph
	r := &reader{
		opts:              opts,
		places:            make(map[string]atlas.Place, len(opts.Graph.Places)),
		lines:             make(map[string]cell),
		titles:            make(map[string]cell),
		boxChoice:         make(map[string]string),
		openDirs:          make(map[string]bool),
		openFiles:         make(map[string]bool),
		symbolLine:        make(map[string]cell),
		operations:        make(map[string][3]string),
		outbound:          make(map[string][]atlas.SymbolCall),
		keys:              make(map[string][]string),
		uses:              make(map[string]*atlas.StageUse),
		started:           make(map[string]time.Time),
		dry:               opts.Provider == nil,
		knowledge:         make(map[string]*Knowledge),
		knowledgeSubjects: make(map[string]*Knowledge),
		responseTables:    make(map[string]rememberedTable),
	}
	files := 0
	for _, place := range opts.Graph.Places {
		r.places[place.ID] = place
		if place.Kind == atlas.PlaceFile && !place.File.Generated {
			files++
		}
	}
	r.budget = opts.Budget || files > BudgetThreshold
	mode := "live"
	if r.dry {
		mode = "dry, every cell is its fallback"
	}
	if r.budget {
		mode += ", budget: the model says where to dig"
	}
	fmt.Fprintf(&r.tables, "# Atlas tables\n\nrepository: %s\nrevision: %s\nmode: %s\n\n", opts.Repository, opts.Revision, mode)
	steps := []struct {
		name string
		run  func(context.Context) error
	}{
		{lines.StageDirectories, r.readDirectories},
		{lines.StageFiles, r.readFiles},
		{lines.StageSymbols, r.readSymbols},
		{lines.StageOperations, r.readOperations},
		{lines.StageBoundaries, r.readBoundaries},
		{lines.StageZones, r.readZones},
		{lines.StageArrows, r.readArrows},
		{lines.StageTargets, r.readTargets},
		{lines.StageJoints, r.readJoints},
	}
	questionOnly := opts.Through == lines.StageQuestion || opts.Through == lines.StageAnswer
	if questionOnly {
		// Restore only descriptions whose exact current basis is remembered.
		// Reuse the ordinary row builders; this prelude never calls a model.
		if opts.Executor.Enabled && !r.dry {
			r.recallOnly = true
			stage, state := r.opts.Stage, r.opts.State
			r.opts.Stage = func(string, ...string) {}
			r.opts.State = func(string, string, ...string) {}
			for _, step := range steps[:5] {
				if err := step.run(ctx); err != nil {
					return Result{}, err
				}
				if r.use(step.name).Reused == 0 {
					delete(r.uses, step.name)
				}
			}
			r.opts.Stage, r.opts.State = stage, state
			r.recallOnly = false
			if len(r.knowledge) > 0 {
				r.opts.State("Knowledge", "ready", fmt.Sprintf("reused %d entity descriptions; no description requests made", len(r.knowledge)))
			}
		}
		steps = nil
	}
	through := ""
	for _, step := range steps {
		if err := step.run(ctx); err != nil {
			return Result{}, err
		}
		through = step.name
		if through == lines.StageSymbols {
			r.assignBoxes()
		}
		if err := os.WriteFile(filepath.Join(opts.OwnerRunDir, atlas.TablesFilename), []byte(r.tables.String()), 0o600); err != nil {
			return Result{}, err
		}
		if err := r.persistKnowledge(); err != nil {
			return Result{}, err
		}
		if through == opts.Through {
			break
		}
	}
	if through == lines.StageJoints && (opts.Learn || opts.Through == stageLearn) {
		if err := r.readLearning(ctx); err != nil {
			return Result{}, err
		}
		if opts.Through == stageLearn {
			through = stageLearn
		}
		if err := os.WriteFile(filepath.Join(opts.OwnerRunDir, atlas.TablesFilename), []byte(r.tables.String()), 0o600); err != nil {
			return Result{}, err
		}
	}
	if (through == lines.StageJoints || questionOnly) && (len(opts.Questions) > 0 || r.learning != nil && len(r.learning.Questions) > 0) {
		if err := r.readQuestions(ctx); err != nil {
			return Result{}, err
		}
		through = lines.StageAnswer
		if opts.Through == lines.StageQuestion {
			through = lines.StageQuestion
		}
		if err := os.WriteFile(filepath.Join(opts.OwnerRunDir, atlas.TablesFilename), []byte(r.tables.String()), 0o600); err != nil {
			return Result{}, err
		}
		if err := r.persistKnowledge(); err != nil {
			return Result{}, err
		}
	}
	tablesPath := filepath.Join(opts.OwnerRunDir, atlas.TablesFilename)
	result := Result{Complete: !questionOnly && (through == lines.StageJoints || through == lines.StageAnswer), Through: through, Rejected: r.rejected, TablesPath: tablesPath, Questions: r.questions, Learning: r.learning}
	if result.Complete {
		result.Atlas = r.atlas(files)
	}
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
	if !ok || line.source == atlas.SourceGiven || line.source == atlas.SourceUnused {
		return "", false
	}
	return line.value, true
}

func (r *reader) readDirectories(ctx context.Context) error {
	def := lines.Directories()
	if r.budget {
		def = lines.WithOpen(def)
	}
	byDepth := make(map[int][]atlas.Place)
	for _, place := range r.opts.Graph.Places {
		if place.Kind == atlas.PlaceDirectory {
			byDepth[place.Depth] = append(byDepth[place.Depth], place)
		}
	}
	depths := sortedInts(byDepth)
	r.opts.Stage(def.Stage, fmt.Sprintf("%d directories in %d rounds by depth", countPlaces(byDepth), len(depths)))
	for round, depth := range depths {
		dirs := byDepth[depth]
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].Path < dirs[j].Path })
		rows := make([]table.Row, 0, len(dirs))
		var asked []atlas.Place
		for _, place := range dirs {
			if r.budget && r.closedAbove(place) {
				r.titles[place.ID] = cell{value: directoryTitle(place.Path), source: atlas.SourceUnused}
				r.lines[place.ID] = cell{value: place.Given, source: atlas.SourceUnused}
				r.openDirs[place.ID] = false
				continue
			}
			var parent *atlas.Place
			if place.Parent != "" {
				if p, ok := r.places[place.Parent]; ok {
					parent = &p
				}
			}
			rows = append(rows, lines.DirectoryRow(place, parent))
			asked = append(asked, place)
		}
		answers, err := r.runTable(ctx, def, round+1, rows)
		if err != nil {
			return err
		}
		for i, place := range asked {
			r.openDirs[place.ID] = true
			if answer := answers[i]; answer.answer != nil {
				r.titles[place.ID] = cell{value: answer.answer["title"], source: answer.source}
				r.lines[place.ID] = cell{value: answer.answer["line"], source: answer.source}
				if r.budget {
					r.openDirs[place.ID] = answer.answer["open"] == "yes"
				}
			} else {
				r.titles[place.ID] = cell{value: directoryTitle(place.Path), source: atlas.SourceGiven}
				r.lines[place.ID] = cell{value: place.Given, source: answer.source}
			}
		}
	}
	r.reportStage(def.Stage)
	return nil
}

// closedAbove says whether a place sits under a directory the model closed.
func (r *reader) closedAbove(place atlas.Place) bool {
	parent := place.Parent
	for parent != "" {
		if open, decided := r.openDirs[parent]; decided && !open {
			return true
		}
		parent = r.places[parent].Parent
	}
	return false
}

func (r *reader) readFiles(ctx context.Context) error {
	def := lines.Files()
	if r.budget {
		def = lines.WithOpen(def)
	}
	var files []atlas.Place
	generated, closed := 0, 0
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceFile {
			continue
		}
		switch {
		case place.File.Generated:
			generated++
			r.lines[place.ID] = cell{value: place.Given, source: atlas.SourceUnused}
		case r.budget && r.closedAbove(place):
			closed++
			r.lines[place.ID] = cell{value: place.Given, source: atlas.SourceUnused}
			r.openFiles[place.ID] = false
		default:
			files = append(files, place)
		}
	}
	details := []string{fmt.Sprintf("%d files in independent windows with direct caller facts", len(files))}
	if generated > 0 {
		details = append(details, fmt.Sprintf("generated files left unasked: %d", generated))
	}
	if closed > 0 {
		details = append(details, fmt.Sprintf("files under closed directories left unasked: %d", closed))
	}
	r.opts.Stage(def.Stage, details...)
	siblings := r.siblingBoxes()
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	rows := make([]table.Row, 0, len(files))
	for _, place := range files {
		directory := r.places[place.Parent]
		rows = append(rows, lines.FileRow(place, directory, r.rankedSiblings(place, siblings[directory.Path]), r, r.places))
	}
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	for i, row := range rows {
		place := r.places[row.ID]
		r.openFiles[row.ID] = true
		if answer := answers[i]; answer.answer != nil {
			r.lines[row.ID] = cell{value: answer.answer["line"], source: answer.source}
			r.boxChoice[row.ID] = answer.answer["box"]
			if r.budget {
				r.openFiles[row.ID] = answer.answer["open"] == "yes"
			}
		} else {
			r.lines[row.ID] = cell{value: place.Given, source: answer.source}
			r.boxChoice[row.ID] = lines.BoxHere
		}
	}
	r.cancelLonelyBoxes()
	r.reportStage(def.Stage)
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
		children[parentDir(place.Path)] = append(children[parentDir(place.Path)], place.Path)
	}
	result := make(map[string][]string)
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceDirectory || len(place.Directory.Files) == 0 {
			continue
		}
		parent := parentDir(place.Path)
		if place.Path == "." {
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
	column := lines.Files().Columns[1]
	count := make(map[string]int)
	for fileID, choice := range r.boxChoice {
		if title, ok := table.IsFree(column, choice); ok {
			count[parentDir(r.places[fileID].Path)+"#"+atlas.Slug(title)]++
		}
	}
	for fileID, choice := range r.boxChoice {
		if title, ok := table.IsFree(column, choice); ok {
			if count[parentDir(r.places[fileID].Path)+"#"+atlas.Slug(title)] < 2 {
				r.boxChoice[fileID] = lines.BoxHere
			}
		}
	}
}

// rowAnswer is what one row ends with: the model's cells, or none with the
// source saying why.
type rowAnswer struct {
	answer      table.Answer
	source      string
	requestSHA  string
	responseSHA string
	requestKey  string
	rowKey      string
}

func (r *reader) runTable(ctx context.Context, def table.Definition, round int, rows []table.Row) ([]rowAnswer, error) {
	return r.runTableWith(ctx, def, round, nil, rows, nil)
}

// runTableWith asks one round of one table: windows in parallel, each window
// its own request, a rejected window falling back on its own. The optional
// check runs over an accepted window's answers and may refuse it.
func (r *reader) runTableWith(
	ctx context.Context, def table.Definition, round int, shared []table.Field,
	rows []table.Row, check func(table.Answers) error,
) ([]rowAnswer, error) {
	answers := make([]rowAnswer, len(rows))
	if len(rows) == 0 {
		return answers, nil
	}
	if _, started := r.started[def.Stage]; !started {
		r.started[def.Stage] = time.Now()
	}
	if r.opts.Through == "" || r.opts.Through == def.Stage {
		if r.opts.Prompt != "" {
			def.System = r.opts.Prompt
		}
		if r.opts.WindowRows > 0 {
			def.Window = r.opts.WindowRows
		}
		if r.opts.InputBytes > 0 {
			def.MaxInputBytes = r.opts.InputBytes
		}
	}
	if def.Independent && !r.dry {
		if check != nil {
			return nil, fmt.Errorf("table %s: a whole-window check cannot validate independent knowledge", def.Stage)
		}
		return r.runIndependent(ctx, def, round, shared, rows)
	}
	return r.runPreparedTable(ctx, def, round, shared, rows, check)
}

func (r *reader) runPreparedTable(ctx context.Context, def table.Definition, round int, shared []table.Field, rows []table.Row, check func(table.Answers) error) ([]rowAnswer, error) {
	answers := make([]rowAnswer, len(rows))
	windows, err := table.WindowsWithContext(def, round, shared, rows)
	if err != nil {
		return nil, err
	}
	use := r.use(def.Stage)
	use.Rows += len(rows)
	use.Windows += len(windows)
	for _, window := range windows {
		if err := r.writeWindowFile(window, "prompt.md", []byte(def.System)); err != nil {
			return nil, err
		}
		if err := r.writeWindowFile(window, "input.json", window.Request); err != nil {
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
			if err := r.writeWindowResult(window, nil, atlas.SourceGiven, "no provider"); err != nil {
				return nil, err
			}
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
		if check != nil {
			decode := call.DecodeValidate
			call.DecodeValidate = func(raw []byte) (table.Answers, error) {
				value, err := decode(raw)
				if err != nil {
					return nil, err
				}
				if err := check(value); err != nil {
					return nil, fmt.Errorf("table %s: %w", def.Stage, err)
				}
				return value, nil
			}
		}
		calls[i] = call
	}
	executor := debugdump.BindStage(r.opts.Executor, def.Stage)
	results := llm.ExecuteJSONEach(ctx, executor, r.opts.Provider, calls)
	for i, window := range windows {
		result := results[i]
		if len(result.Outcome.Request) > 0 {
			if err := r.writeWindowFile(window, "request.json", result.Outcome.Request); err != nil {
				return nil, err
			}
		}
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
				answers[offsets[i]+j] = rowAnswer{answer: result.Outcome.Value[j], source: source, requestSHA: result.Outcome.RequestSHA256, responseSHA: result.Outcome.ResponseSHA256, requestKey: result.Outcome.CacheKey, rowKey: table.Key(j)}
			}
			if err := r.writeWindowResult(window, result.Outcome.Value, source, ""); err != nil {
				return nil, err
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
		responseRef := ""
		if len(result.Outcome.Response) > 0 {
			responseRef = filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json")))
		}
		r.rejected = append(r.rejected, modeldiag.Row{
			Stage: def.Stage, Kind: "window_rejected", Count: len(window.Rows),
			Reason:      reason,
			ResponseRef: responseRef,
			Samples:     []string{fmt.Sprintf("round %d window %d", window.Round, window.Index)},
		})
		if err := r.writeWindowResult(window, nil, atlas.SourceGiven, reason); err != nil {
			return nil, err
		}
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

func (r *reader) reportStage(stage string) {
	use := r.use(stage)
	reused := fmt.Sprintf("reused entity descriptions: %d", use.Reused)
	if stage == lines.StageQuestion {
		reused = fmt.Sprintf("reused question/evidence decisions: %d", use.Reused)
	}
	details := []string{
		fmt.Sprintf("rows: %d", use.Rows),
		fmt.Sprintf("windows: %d (live %d, cached %d, rejected %d)", use.Windows, use.Live, use.Cached, use.Rejected),
		reused,
		fmt.Sprintf("rows without a model answer: %d", use.Given),
	}
	if started, ok := r.started[stage]; ok {
		details = append(details, fmt.Sprintf("duration: %s", time.Since(started).Round(time.Millisecond)))
	}
	r.opts.State(stage, "ready", details...)
}

func (r *reader) windowFileName(window table.Window, suffix string) string {
	prefix := window.Stage
	if r.questionKey != "" {
		prefix += "-" + r.questionKey
	}
	return fmt.Sprintf("%s-r%d-w%d.%s", prefix, window.Round, window.Index, suffix)
}

func (r *reader) writeWindowFile(window table.Window, suffix string, data []byte) error {
	if suffix != "result.json" {
		cacheRoot := r.opts.Executor.RootDir
		if cacheRoot == "" {
			cacheRoot = filepath.Dir(r.opts.OwnerRunDir)
		}
		filename, err := llm.SavePayload(cacheRoot, data)
		if err != nil {
			return err
		}
		tablesDir, err := filepath.Abs(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(tablesDir, filename)
		if err != nil {
			return err
		}
		data, err = json.Marshal(struct {
			File string `json:"file"`
		}{filepath.ToSlash(relative)})
		if err != nil {
			return err
		}
		suffix = strings.TrimSuffix(suffix, filepath.Ext(suffix)) + ".ref.json"
	}
	name := filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, r.windowFileName(window, suffix))
	if err := os.WriteFile(name, data, 0o600); err != nil {
		return fmt.Errorf("atlas reading: write %s: %w", filepath.Base(name), err)
	}
	return nil
}

// printWindow adds one window to tables.md: every row with its key, its
// inputs, its fallback and, after a live run, the model's cells beside it.
func (r *reader) printWindow(def table.Definition, window table.Window, answers table.Answers, source string, latency time.Duration) {
	fmt.Fprintf(&r.tables, "## %s · round %d · window %d · %s\n\n",
		def.Stage, window.Round, window.Index, path.Join(atlas.TablesDir, r.windowFileName(window, "request.ref.json")))
	fmt.Fprintf(&r.tables, "source: %s", source)
	if latency > 0 {
		fmt.Fprintf(&r.tables, " · %s", latency.Round(time.Millisecond))
	}
	r.tables.WriteString("\n")
	for _, field := range window.Context {
		encoded, _ := json.Marshal(field.Value)
		fmt.Fprintf(&r.tables, "context %s: %s\n", field.Name, string(encoded))
	}
	r.tables.WriteString("\n")
	for i, row := range window.Rows {
		fmt.Fprintf(&r.tables, "- %s · %s\n", table.Key(i), row.ID)
		for _, field := range row.Fields {
			encoded, _ := json.Marshal(field.Value)
			fmt.Fprintf(&r.tables, "  - %s: %s\n", field.Name, string(encoded))
		}
		if place, ok := r.places[row.ID]; ok {
			fmt.Fprintf(&r.tables, "  - given: %s\n", place.Given)
		}
		if answers != nil {
			for _, column := range def.Columns {
				fmt.Fprintf(&r.tables, "  - %s → %s\n", column.Name, answers[i][column.Name])
			}
		}
	}
	r.tables.WriteString("\n")
}

// atlas folds everything into the artifact the page reads.
func (r *reader) atlas(files int) atlas.Atlas {
	result := atlas.Atlas{
		Version: atlas.Version, Repository: r.opts.Repository, Revision: r.opts.Revision,
		Targets: []atlas.Target{}, Joints: append([]atlas.Joint{}, r.joints...), Diagnostics: []atlas.Diagnostic{},
		Budget: atlas.Budget{Files: files, OpenAsked: r.budget},
	}
	for _, open := range r.openDirs {
		if open {
			result.Budget.DirsOpened++
		}
	}
	for _, open := range r.openFiles {
		if open {
			result.Budget.FilesOpened++
		}
	}
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
		SharedCode: append([]string(nil), meta.SharedCode...),
		Zones:      []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{},
		Boundaries: []atlas.Boundary{}, Trace: []string{},
	}
	if state, ok := r.targets[meta.ID]; ok {
		target.Line, target.Role = state.line, state.role
	}
	for _, owner := range r.boxesOfTarget(meta.ID) {
		box := atlas.Box{
			ID: owner.id, Dir: owner.dir, Title: owner.title, Line: owner.line,
			ZoneID: owner.zoneID[meta.ID], Side: r.side(owner, meta.ID), Open: owner.open,
			Files: []atlas.File{}, Keys: []atlas.Key{},
		}
		for _, fileID := range owner.files {
			place := r.places[fileID]
			if !contains(place.TargetIDs, meta.ID) {
				continue
			}
			line := r.lines[fileID]
			file := atlas.File{
				Path: place.Path, Line: line.value, Source: line.source,
				Open:    r.openFiles[fileID] || !r.budget && !place.File.Generated,
				Asked:   line.source != atlas.SourceUnused,
				Symbols: []atlas.Symbol{},
				Callers: len(place.File.Callers), Callees: len(place.File.Callees),
			}
			for _, decl := range place.File.Decls {
				symbol := atlas.Symbol{
					ObjectID: decl.ObjectID,
					ID:       atlas.SymbolID(place.Path, decl.LineNo, decl.Name), Name: decl.Name, Kind: decl.Kind,
					Signature: decl.Signature, Doc: decl.Doc, LineNo: decl.LineNo, Column: decl.Column,
				}
				if line, ok := r.symbolLine[symbol.ID]; ok {
					symbol.Line = line.value
				}
				if knowledge := r.knowledge[symbol.ID]; knowledge != nil && knowledge.Cells["alias"] != "none" {
					symbol.Alias = knowledge.Cells["alias"]
				}
				symbol.Key = contains(r.keys[fileID], symbol.ID)
				if operation, ok := r.operations[symbol.ID]; ok {
					symbol.Activation, symbol.Operation, symbol.OperationSummary = operation[0], operation[1], operation[2]
				}
				file.Symbols = append(file.Symbols, symbol)
			}
			box.Files = append(box.Files, file)
			target.Files++
			target.Symbols += len(file.Symbols)
		}
		box.Keys = modelKeys(box.Files)
		if len(box.Keys) == 0 {
			box.Keys = rankedKeys(box.Files)
		}
		target.Boxes = append(target.Boxes, box)
	}
	for _, zone := range r.zones[meta.ID] {
		boxIDs := append([]string{}, zone.boxes...)
		sort.Strings(boxIDs)
		target.Zones = append(target.Zones, atlas.Zone{ID: zone.id, Title: zone.title, Line: zone.line, BoxIDs: boxIDs})
	}
	for _, arrow := range r.arrows[meta.ID] {
		if !arrow.drawn {
			continue
		}
		target.Arrows = append(target.Arrows, atlas.Arrow{
			From: arrow.from, To: arrow.to, Calls: arrow.calls, Witnesses: arrow.topWitnesses(), Sentence: arrow.sentence,
		})
	}
	boundaryIDs := make([]string, 0, len(r.boundaries))
	for id := range r.boundaries {
		boundaryIDs = append(boundaryIDs, id)
	}
	sort.Strings(boundaryIDs)
	for _, id := range boundaryIDs {
		state := r.boundaries[id]
		if !contains(state.place.TargetIDs, meta.ID) {
			continue
		}
		boxID, ok := r.boxOf[state.place.Parent]
		if !ok {
			continue
		}
		facts := state.place.Boundary
		target.Boundaries = append(target.Boundaries, atlas.Boundary{
			ObjectID: facts.ObjectID,
			ID:       state.place.ID, BoxID: boxID, Path: state.place.Path, LineNo: state.place.LineNo, Column: state.place.Column,
			Caller: facts.Caller, Direction: facts.Direction, Kind: state.kind, External: facts.External, Method: facts.Method,
			Values: append([]string{}, facts.Values...), Line: state.line, FactID: facts.FactID,
		})
	}
	target.Trace = r.trace(meta.ID)
	return target
}

// modelKeys lists the symbols the model marked as key, three per box, the
// documented ones first.
func modelKeys(files []atlas.File) []atlas.Key {
	var keys []atlas.Key
	for _, file := range files {
		for _, symbol := range file.Symbols {
			if !symbol.Key {
				continue
			}
			doc := symbol.Line
			if doc == "" {
				doc = symbol.Doc
			}
			keys = append(keys, atlas.Key{SymbolID: symbol.ID, Name: symbol.Name, Path: file.Path, Doc: doc, LineNo: symbol.LineNo})
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if (keys[i].Doc != "") != (keys[j].Doc != "") {
			return keys[i].Doc != ""
		}
		return keys[i].Name < keys[j].Name
	})
	if len(keys) > 3 {
		keys = keys[:3]
	}
	return keys
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
			if symbol.Doc == "" {
				continue
			}
			exported := symbol.Name != "" && strings.ToUpper(symbol.Name[:1]) == symbol.Name[:1]
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
