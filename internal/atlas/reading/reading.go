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
	"strconv"
	"strings"
	"sync"
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
	// Dependencies are the external packages the target imports, as the
	// dependency catalogue names them. They annotate the closed list of
	// runtime systems a boundary may name; none of them is a destination.
	Dependencies []string `json:"dependencies,omitempty"`
}

// Options is everything the reading needs.
type Options struct {
	Graph atlas.Graph
	// SealedGraph optionally holds the bytes atlas.EncodeGraph returned for
	// Graph (places.json), so the saved input does not seal the graph again.
	SealedGraph []byte
	Targets     []TargetMeta
	Repository  string
	Revision    string
	// Executor is the run's executor; the reading binds it to one debugdump
	// stage per table. Provider nil means a dry run: no call is made and
	// every cell takes its fallback.
	Executor llm.Executor
	Provider llm.Provider
	// Categorizer answers the closed tables (Definition.Classifier): keys,
	// part roles and key declarations. A live reading requires it; they
	// never fall back to Provider.
	Categorizer llm.Categorizer
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
	// Prompt needs an explicit Through stage; for a closed table it is the
	// categorizer's task. The row and byte budgets size text-model requests:
	// the categorizer's own limits pack its tables. Zero limits use the stage
	// defaults.
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
	// ReadSource returns a repository file's bytes for the outside symbols'
	// usage line; nil leaves their rows without it.
	ReadSource func(path string) ([]byte, error)
	// NoCaptions leaves every prose cell (titles, lines, sentences) on its
	// fallback and asks the model for decisions alone. A declaration's alias
	// is asked by its name either way (lines.NeedsAlias).
	NoCaptions bool
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
	opts              Options
	places            map[string]atlas.Place
	directoriesByPath map[string]string
	symbolsBySource   map[string]string
	lines             map[string]cell
	titles            map[string]cell
	openDirs          map[string]bool // directory place ID -> open, budget mode only
	openFiles         map[string]bool // file place ID -> open, budget mode only
	budget            bool

	symbolLine map[string]cell     // symbol place ID -> model line
	api        map[string]apiRole  // external symbol -> what it binds, publishes, talks to
	keys       map[string][]string // file place ID -> key symbol IDs, by rank
	// selectedKeys is every declaration the selection found worth a reader's
	// attention; partKeys is what explains the part it stands in.
	selectedKeys, partKeys, keysDecided map[string]bool

	boxOf          map[string]string    // file place ID -> box ID
	classifierGate *llm.BatchController // the decision model's own attempt gate
	// classifierResponses holds remembered decision-model responses by
	// request key, parsed once for all the rows they answer.
	classifierResponses map[string]rememberedClassifier
	designBoxOf         map[string]map[string]string // target -> declaration/file -> accepted part
	splitFiles          map[string]map[string]bool   // target -> files whose code several parts hold
	designFiles         map[string]string            // declaration -> its source file
	designSubjects      map[string]string            // native declaration -> place
	nextPart            int
	nextZone            int
	nextBoundary        int
	nextJoint           int
	boundaryIDs         map[string]string        // stable source identity -> compact boundary ID
	boxes               map[string]*boxState     // box ID -> box
	offMap              map[string][]offMapEntry // target -> what no part holds
	mapFailure          map[string]string        // target -> why it has no map of parts
	zones               map[string][]*zoneState
	arrows              map[string][]*arrowState
	boundaries          map[string]*boundaryState
	targets             map[string]*targetState
	joints              []atlas.Joint

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
	knowledgeRecords  map[knowledgeRecordKey]*Knowledge
	knowledgeSubjects map[string]*Knowledge
	symbolSelections  map[string]*Knowledge // native subject -> independent selection evidence
	responseTables    map[string]rememberedTable
	recallOnly        bool
	// shared guards knowledge and the response caches for concurrent work.
	shared *readerShared
	// knowledge.json is rewritten only when the shared record version moved
	// past the one last written, tables.md only when the printed text grew.
	knowledgeSaved, tablesSaved     int
	knowledgeWritten, tablesWritten bool
}

func (r *reader) compactID(prefix string, next *int) string {
	(*next)++
	return prefix + fmt.Sprint(*next)
}

func compactIDLess(left, right string) bool {
	parts := func(value string) (string, int, bool) {
		cut := 0
		for cut < len(value) && value[cut] >= 'a' && value[cut] <= 'z' {
			cut++
		}
		if cut == 0 || cut == len(value) {
			return "", 0, false
		}
		number, err := strconv.Atoi(value[cut:])
		return value[:cut], number, err == nil && number > 0
	}
	leftPrefix, leftNumber, leftOK := parts(left)
	rightPrefix, rightNumber, rightOK := parts(right)
	if leftOK && rightOK && leftPrefix == rightPrefix {
		return leftNumber < rightNumber
	}
	return left < right
}

func symbolSourceKey(path string, line int, name string) string {
	return path + "\x00" + fmt.Sprintf("%d", line) + "\x00" + name
}

func (r *reader) symbolID(path string, line int, name string) string {
	if id := r.symbolsBySource[symbolSourceKey(path, line, name)]; id != "" {
		return id
	}
	for id, place := range r.places {
		if place.Kind == atlas.PlaceSymbol && place.Path == path && place.LineNo == line && place.Symbol != nil && place.Symbol.Decl.Name == name {
			return id
		}
	}
	return ""
}

func (r *reader) directoryID(path string) string {
	if id := r.directoriesByPath[path]; id != "" {
		return id
	}
	for id, place := range r.places {
		if place.Kind == atlas.PlaceDirectory && place.Path == path {
			return id
		}
	}
	return ""
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
	if opts.Provider != nil && opts.Categorizer == nil {
		return Result{}, fmt.Errorf("atlas reading: a live reading needs a categorizer for its closed tables")
	}
	if opts.Stage == nil {
		opts.Stage = func(string, ...string) {}
	}
	if opts.State == nil {
		opts.State = func(string, string, ...string) {}
	}
	// Stages that run at once report one event at a time.
	var reporting sync.Mutex
	stage, state := opts.Stage, opts.State
	opts.Stage = func(name string, details ...string) {
		reporting.Lock()
		defer reporting.Unlock()
		stage(name, details...)
	}
	opts.State = func(name, value string, details ...string) {
		reporting.Lock()
		defer reporting.Unlock()
		state(name, value, details...)
	}
	if err := os.MkdirAll(filepath.Join(opts.OwnerRunDir, atlas.TablesDir), 0o700); err != nil {
		return Result{}, fmt.Errorf("atlas reading: prepare %s: %w", atlas.TablesDir, err)
	}
	graph, err := SaveInput(opts)
	if err != nil {
		return Result{}, err
	}
	opts.Graph, opts.SealedGraph = graph, nil
	r := &reader{
		opts:              opts,
		classifierGate:    &llm.BatchController{},
		places:            make(map[string]atlas.Place, len(opts.Graph.Places)),
		directoriesByPath: make(map[string]string),
		symbolsBySource:   make(map[string]string),
		lines:             make(map[string]cell),
		titles:            make(map[string]cell),
		openDirs:          make(map[string]bool),
		openFiles:         make(map[string]bool),
		symbolLine:        make(map[string]cell),
		api:               make(map[string]apiRole),
		keys:              make(map[string][]string),
		selectedKeys:      make(map[string]bool),
		keysDecided:       make(map[string]bool),
		uses:              make(map[string]*atlas.StageUse),
		started:           make(map[string]time.Time),
		dry:               opts.Provider == nil,
		knowledge:         make(map[string]*Knowledge),
		knowledgeRecords:  make(map[knowledgeRecordKey]*Knowledge),
		knowledgeSubjects: make(map[string]*Knowledge),
		responseTables:    make(map[string]rememberedTable),
		boundaryIDs:       make(map[string]string),
		boxOf:             make(map[string]string),
		shared:            &readerShared{},

		classifierResponses: make(map[string]rememberedClassifier),
	}
	files := 0
	for _, place := range opts.Graph.Places {
		r.places[place.ID] = place
		if place.Kind == atlas.PlaceDirectory {
			r.directoriesByPath[place.Path] = place.ID
		}
		if place.Kind == atlas.PlaceFile {
			// Before the parts are drawn, a file stands for itself.
			r.boxOf[place.ID] = place.Path
		}
		if place.Kind == atlas.PlaceSymbol && place.Symbol != nil {
			r.symbolsBySource[symbolSourceKey(place.Path, place.LineNo, place.Symbol.Decl.Name)] = place.ID
		}
		if place.Kind == atlas.PlaceBoundary {
			r.nextBoundary++
		}
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
	questionOnly := opts.Through == lines.StageQuestion || opts.Through == lines.StageAnswer
	through := ""
	if questionOnly {
		// Restore only descriptions whose exact current basis is remembered.
		// Reuse the ordinary row builders; this prelude never calls a model.
		if opts.Executor.Enabled && !r.dry {
			r.recallOnly = true
			stage, state := r.opts.Stage, r.opts.State
			r.opts.Stage = func(string, ...string) {}
			r.opts.State = func(string, string, ...string) {}
			for _, name := range recallStages {
				if err := readingStages[name](r, ctx); err != nil {
					return Result{}, err
				}
				if r.use(name).Reused == 0 {
					delete(r.uses, name)
				}
			}
			r.opts.Stage, r.opts.State = stage, state
			r.recallOnly = false
			if len(r.knowledge) > 0 {
				r.opts.State("Knowledge", "ready", fmt.Sprintf("reused %d entity descriptions; no description requests made", len(r.knowledge)))
			}
		}
	} else if through, err = r.walk(ctx); err != nil {
		return Result{}, err
	}
	if through == lines.StageJoints && (opts.Learn || opts.Through == stageLearn) {
		if err := r.readLearning(ctx); err != nil {
			return Result{}, err
		}
		if opts.Through == stageLearn {
			through = stageLearn
		}
		if err := r.saveTables(); err != nil {
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
		if err := r.saveTables(); err != nil {
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

// saveTables writes tables.md when the reading printed more since the last
// write. The printed text only grows, so an equal length is the same text.
func (r *reader) saveTables() error {
	if r.tablesWritten && r.tablesSaved == r.tables.Len() {
		return nil
	}
	if err := os.WriteFile(filepath.Join(r.opts.OwnerRunDir, atlas.TablesFilename), []byte(r.tables.String()), 0o600); err != nil {
		return err
	}
	r.tablesWritten, r.tablesSaved = true, r.tables.Len()
	return nil
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
		// The children of one parent share a window; the parent is its
		// context, sent once. Rows keep their path order across groups.
		var groups rowGroups
		byParent := make(map[string]int)
		var asked []atlas.Place
		for _, place := range dirs {
			if r.budget && r.closedAbove(place) {
				r.titles[place.ID] = cell{value: directoryTitle(place.Path), source: atlas.SourceUnused}
				r.lines[place.ID] = cell{value: place.Given, source: atlas.SourceUnused}
				r.openDirs[place.ID] = false
				continue
			}
			at, known := byParent[place.Parent]
			if !known {
				var parent *atlas.Place
				if p, ok := r.places[place.Parent]; ok && place.Parent != "" {
					parent = &p
				}
				at = len(groups)
				byParent[place.Parent] = at
				groups = append(groups, rowGroup{shared: lines.DirectoryContext(parent)})
			}
			groups[at].rows = append(groups[at].rows, lines.DirectoryRow(place))
			asked = append(asked, place)
		}
		// asked follows path order; answers follow group order.
		sort.SliceStable(asked, func(i, j int) bool {
			if byParent[asked[i].Parent] != byParent[asked[j].Parent] {
				return byParent[asked[i].Parent] < byParent[asked[j].Parent]
			}
			return asked[i].Path < asked[j].Path
		})
		answers, err := r.runTableGroups(ctx, def, round+1, groups, nil)
		if err != nil {
			return err
		}
		for i, place := range asked {
			r.openDirs[place.ID] = true
			answer := answers[i]
			// A cell refused alone takes the fallback the whole row takes: the
			// given title or line, and an open that closes nothing.
			r.titles[place.ID] = cell{value: directoryTitle(place.Path), source: atlas.SourceGiven}
			r.lines[place.ID] = cell{value: place.Given, source: answer.source}
			if answer.answer == nil {
				continue
			}
			if title, ok := answer.answer["title"]; ok {
				r.titles[place.ID] = cell{value: title, source: answer.source}
			}
			if line, ok := answer.answer["line"]; ok {
				r.lines[place.ID] = cell{value: line, source: answer.source}
			} else {
				r.lines[place.ID] = cell{value: place.Given, source: atlas.SourceGiven}
			}
			if r.budget {
				r.openDirs[place.ID] = answer.answer["open"] != "no"
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
	calling := r.fileCallers()
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	rows := make([]table.Row, 0, len(files))
	for _, place := range files {
		directory := r.places[place.Parent]
		rows = append(rows, lines.FileRow(place, directory, r, r.places, calling[place.ID]))
	}
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	for i, row := range rows {
		place := r.places[row.ID]
		r.openFiles[row.ID] = true
		answer := answers[i]
		// A refused line keeps the given one; a refused open closes nothing.
		r.lines[row.ID] = cell{value: place.Given, source: answer.source}
		if answer.answer == nil {
			continue
		}
		if line, ok := answer.answer["line"]; ok {
			r.lines[row.ID] = cell{value: line, source: answer.source}
		} else {
			r.lines[row.ID] = cell{value: place.Given, source: atlas.SourceGiven}
		}
		if r.budget {
			r.openFiles[row.ID] = answer.answer["open"] != "no"
		}
	}
	r.reportStage(def.Stage)
	return nil
}

// fileCallers reads, per file, which declarations of each calling file the
// graph saw calling into it: the witnesses of the file-to-file edges, each
// name once, in edge order.
func (r *reader) fileCallers() map[string]lines.FileCallers {
	result := make(map[string]lines.FileCallers)
	for _, edge := range r.opts.Graph.Edges {
		if r.places[edge.From].File == nil || r.places[edge.To].File == nil {
			continue
		}
		for _, witness := range edge.Witnesses {
			if witness.Caller == "" {
				continue
			}
			callers := result[edge.To]
			if callers == nil {
				callers = make(lines.FileCallers)
				result[edge.To] = callers
			}
			if !contains(callers[edge.From], witness.Caller) {
				callers[edge.From] = append(callers[edge.From], witness.Caller)
			}
		}
	}
	return result
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
	// partial marks a recalled answer that lost a cell: its text is not
	// accepted as glossary prose.
	partial bool
}

func (r *reader) runTable(ctx context.Context, def table.Definition, round int, rows []table.Row) ([]rowAnswer, error) {
	return r.runTableWith(ctx, def, round, nil, rows, nil)
}

// rowGroup is a run of rows that share one window context: the boundary
// candidates of one declaration with that declaration sent once, the
// directories of one parent with the parent's line sent once. The groups of
// one round still go to the provider together.
type rowGroup struct {
	shared []table.Field
	rows   []table.Row
}

type rowGroups []rowGroup

func (groups rowGroups) count() int {
	total := 0
	for _, group := range groups {
		total += len(group.rows)
	}
	return total
}

// runTableWith asks one round of one table: windows in parallel, each window
// its own request, a rejected window falling back on its own. The optional
// check runs over an accepted window's answers and may refuse it.
func (r *reader) runTableWith(
	ctx context.Context, def table.Definition, round int, shared []table.Field,
	rows []table.Row, check func(table.Answers) error,
) ([]rowAnswer, error) {
	return r.runTableGroups(ctx, def, round, rowGroups{{shared: shared, rows: rows}}, check)
}

// runTableGroups is runTableWith over several groups, each with its own
// shared context; the answers come back in the groups' row order.
func (r *reader) runTableGroups(ctx context.Context, def table.Definition, round int, groups rowGroups, check func(table.Answers) error) ([]rowAnswer, error) {
	answers := make([]rowAnswer, groups.count())
	if len(answers) == 0 {
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
	if r.opts.NoCaptions {
		def = withoutCaptions(def)
		if len(def.Columns) == 0 {
			for i := range answers {
				answers[i] = rowAnswer{source: atlas.SourceGiven}
			}
			r.use(def.Stage).Rows += len(answers)
			r.use(def.Stage).Given += len(answers)
			return answers, nil
		}
	}
	if def.Memoize && !r.dry {
		if check != nil {
			return nil, fmt.Errorf("table %s: a whole-window check cannot validate independent knowledge", def.Stage)
		}
		return r.runIndependent(ctx, def, round, groups)
	}
	return r.runPreparedGroups(ctx, def, round, groups, check)
}

// withoutCaptions keeps the decisions of a table and drops its prose cells;
// a table of prose alone asks nothing. An alias is not a caption: a table
// asked of names that need one keeps it with or without captions (owner
// decision 2026-09-26), and the other names never take it (withoutAlias).
func withoutCaptions(def table.Definition) table.Definition {
	return narrowed(def, func(column table.Column) bool {
		return column.Kind != table.Text || column.Name == lines.AliasColumn
	})
}

// withoutAlias is a Symbols or Types table asked of names that need no alias
// (lines.NeedsAlias): its other cells, without the alias. A type then asks
// its line alone with or without captions, in one request shape and memo.
func withoutAlias(def table.Definition) table.Definition {
	return narrowed(def, func(column table.Column) bool { return column.Name != lines.AliasColumn })
}

// narrowedContract marks a table asked with fewer cells than it defines.
const narrowedContract = ".decisions"

// narrowed keeps the columns keep accepts. A table that lost any is marked
// once, however many narrowings took them.
func narrowed(def table.Definition, keep func(table.Column) bool) table.Definition {
	columns := make([]table.Column, 0, len(def.Columns))
	for _, column := range def.Columns {
		if keep(column) {
			columns = append(columns, column)
		}
	}
	if len(columns) == len(def.Columns) {
		return def
	}
	def.Columns = columns
	if !strings.HasSuffix(def.Contract, narrowedContract) {
		def.Contract += narrowedContract
	}
	return def
}

func (r *reader) runPreparedTable(ctx context.Context, def table.Definition, round int, shared []table.Field, rows []table.Row, check func(table.Answers) error) ([]rowAnswer, error) {
	return r.runPreparedGroups(ctx, def, round, rowGroups{{shared: shared, rows: rows}}, check)
}

// runPreparedGroups packs every group into its own windows and sends all of
// them as one batch; window indexes run across the groups.
func (r *reader) runPreparedGroups(ctx context.Context, def table.Definition, round int, groups rowGroups, check func(table.Answers) error) ([]rowAnswer, error) {
	answers := make([]rowAnswer, groups.count())
	provider := r.providerFor(def)
	classifier := r.classifies(def)
	if classifier {
		def = table.ForClassifier(def)
	}
	var windows []table.Window
	for _, group := range groups {
		if len(group.rows) == 0 {
			continue
		}
		packed, err := table.WindowsWithContext(def, round, group.shared, group.rows)
		if err != nil {
			return nil, err
		}
		if classifier {
			if packed, err = table.FitClassifierWindows(r.opts.Categorizer, def, packed); err != nil {
				return nil, err
			}
		}
		for _, window := range packed {
			window.Index = len(windows)
			windows = append(windows, window)
		}
	}
	use := r.use(def.Stage)
	use.Rows += len(answers)
	use.Windows += len(windows)
	offsets := make([]int, len(windows))
	offset := 0
	for i, window := range windows {
		offsets[i] = offset
		offset += len(window.Rows)
	}
	if r.dry {
		for i, window := range windows {
			if err := r.writeWindowExchange(window, []byte(def.System), window.Request, nil, nil, false); err != nil {
				return nil, err
			}
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
	calls := make([]llm.Call[table.Result], len(windows))
	for i, window := range windows {
		if classifier {
			call, err := table.ClassifierCall(r.opts.Categorizer, def, window)
			if err != nil {
				return nil, err
			}
			decode := call.DecodeValidate
			call.DecodeValidate = func(raw []byte) (table.Result, error) {
				value, err := decode(raw)
				if err == nil && check != nil {
					if err := check(value.Answers); err != nil {
						return table.Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
					}
				}
				return value, err
			}
			calls[i] = call
			continue
		}
		call, err := table.Call(def, window)
		if err != nil {
			return nil, err
		}
		calls[i] = llm.Call[table.Result]{
			State: call.State, Prompt: call.Prompt, Limits: call.Limits,
			DecodeValidate: func(raw []byte) (table.Result, error) {
				value, err := table.DecodeResult(def, window, raw)
				if err != nil {
					return table.Result{}, err
				}
				if check != nil {
					if err := check(value.Answers); err != nil {
						return table.Result{}, fmt.Errorf("table %s: %w", def.Stage, err)
					}
				}
				return value, nil
			},
		}
	}
	executor := debugdump.BindStage(r.opts.Executor, def.Stage)
	if classifier {
		// The categorizer has its own rate limits and its own gate.
		executor.BatchConcurrency, executor.BatchController = table.ClassifierConcurrency, r.classifierGate
	}
	results := llm.ExecuteJSONEach(ctx, executor, provider, calls)
	for i, window := range windows {
		result := results[i]
		// The prompt and input follow the exchange: a window refused whole or
		// in a row or cell keeps all its payloads in the run.
		if err := r.writeWindowExchange(window, []byte(def.System), window.Request, result.Outcome.Request, result.Outcome.Response, result.Err != nil || len(result.Outcome.ResponseRejections) > 0); err != nil {
			return nil, err
		}
		if result.Err == nil {
			value := result.Outcome.Value
			source := atlas.SourceModel
			if result.Outcome.Cached {
				source = atlas.SourceCache
				use.Cached++
			} else {
				use.Live++
			}
			rejectedRows := 0
			for j := range window.Rows {
				if value.Answers[j] == nil {
					answers[offsets[i]+j] = rowAnswer{source: atlas.SourceGiven}
					rejectedRows++
					continue
				}
				answers[offsets[i]+j] = rowAnswer{answer: value.Answers[j], source: source, requestSHA: result.Outcome.RequestSHA256, responseSHA: result.Outcome.ResponseSHA256, requestKey: result.Outcome.CacheKey, rowKey: window.Rows[j].ID}
			}
			use.Given += rejectedRows
			reason := ""
			if rejectedRows > 0 {
				use.Rejected++
				reason = fmt.Sprintf("%d rows rejected, %d accepted in this response", rejectedRows, len(window.Rows)-rejectedRows)
				if r.opts.State != nil {
					r.opts.State(def.Stage, "ready", reason)
				}
			}
			r.journalRowRejections(def, window, value.Rejections)
			if err := r.writeWindowResult(window, value.Answers, source, reason); err != nil {
				return nil, err
			}
			r.printWindow(def, window, value.Answers, source, result.Outcome.Metrics.Latency)
			for _, rejection := range value.Rejections {
				fmt.Fprintf(&r.tables, "- Rejected %s: %s\n", rejection.Key, rejection.Reason)
			}
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
		// A response with no accepted row is not cached; each row's own
		// reason still reaches the journal, not only the first one.
		var refused *table.NoRowsAccepted
		if errors.As(result.Err, &refused) {
			r.journalRowRejections(def, window, refused.Rejections)
		}
		if err := r.writeWindowResult(window, nil, atlas.SourceGiven, reason); err != nil {
			return nil, err
		}
		r.printWindow(def, window, nil, "rejected: "+reason, result.Outcome.Metrics.Latency)
	}
	return answers, nil
}

// journalRowRejections writes one journal row per refused row or cell of a
// window's response.
func (r *reader) journalRowRejections(def table.Definition, window table.Window, rejections []table.RowRejection) {
	for _, rejection := range rejections {
		samples := []string{rejection.Key}
		for _, row := range window.Rows {
			if rejection.Key == row.ID {
				samples = append(samples, row.ID)
				break
			}
		}
		r.rejected = append(r.rejected, modeldiag.Row{
			Stage: def.Stage, Kind: rejectionKind(rejection), Count: 1, Reason: rejection.Reason, Samples: samples,
			ResponseRef: filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))),
		})
	}
}

// rejectionKind names a refused row, or one refused cell of a kept row.
func rejectionKind(rejection table.RowRejection) string {
	if rejection.Cell != "" {
		return "cell_rejected"
	}
	return "row_rejected"
}

// classifies reports whether the categorizer answers this table: the table
// opted in and the reading is live. A dry reading has no categorizer and
// packs the table as it prints it; ClassifierCall refuses one not closed.
func (r *reader) classifies(def table.Definition) bool {
	return r.opts.Categorizer != nil && def.Classifier
}

// providerFor is the provider that answers this table.
func (r *reader) providerFor(def table.Definition) llm.Provider {
	if r.classifies(def) {
		return r.opts.Categorizer
	}
	return r.opts.Provider
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

// writeWindowFile writes one run-local file of a window: its normalized
// result or a payload ref.
func (r *reader) writeWindowFile(window table.Window, suffix string, data []byte) error {
	name := filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, r.windowFileName(window, suffix))
	if err := os.WriteFile(name, data, 0o600); err != nil {
		return fmt.Errorf("atlas reading: write %s: %w", filepath.Base(name), err)
	}
	return nil
}

// writeWindowExchange references every payload of one window's model
// exchange: its prompt, its table input, the exact request and the response;
// an absent one is nil. A wholly accepted answer's payloads are linked from
// the shared store beside its cache record's. refused is an answer refused
// whole or in any part: its call failed, its decoder refused a row, cell or
// member, or the stage recorded a rejected row for it. Its payloads are what
// those rows point at, so they are saved in this run, beside its journal,
// where cache clear leaves them and no later run reads them; an accepted
// record keeps its own copy in the store.
func (r *reader) writeWindowExchange(window table.Window, prompt, input, request, response []byte, refused bool) error {
	cacheRoot := r.opts.Executor.RootDir
	if cacheRoot == "" {
		cacheRoot = filepath.Dir(r.opts.OwnerRunDir)
	}
	save := func(raw []byte) (string, error) { return llm.SavePayload(cacheRoot, raw) }
	if refused {
		save = func(raw []byte) (string, error) { return llm.SaveRunPayload(r.opts.OwnerRunDir, raw) }
	}
	tablesDir, err := filepath.Abs(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir))
	if err != nil {
		return err
	}
	for _, item := range []struct {
		name string
		data []byte
	}{{"prompt", prompt}, {"input", input}, {"request", request}, {"response", response}} {
		if len(item.data) == 0 {
			continue
		}
		filename, err := save(item.data)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(tablesDir, filename)
		if err != nil {
			return err
		}
		ref, err := json.Marshal(struct {
			File string `json:"file"`
		}{filepath.ToSlash(relative)})
		if err != nil {
			return err
		}
		if err := r.writeWindowFile(window, item.name+".ref.json", ref); err != nil {
			return err
		}
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
		fmt.Fprintf(&r.tables, "- %s\n", row.ID)
		for _, field := range row.Fields {
			encoded, _ := json.Marshal(field.Value)
			fmt.Fprintf(&r.tables, "  - %s: %s\n", field.Name, string(encoded))
		}
		if place, ok := r.places[row.ID]; ok && !localRows(def) {
			fmt.Fprintf(&r.tables, "  - given: %s\n", place.Given)
		}
		if answers != nil && answers[i] != nil {
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
		Targets: []atlas.Target{}, Joints: append([]atlas.Joint{}, r.joints...), API: r.apiRoles(), Diagnostics: []atlas.Diagnostic{},
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
		projected := r.target(target)
		projected.Data = r.dataForTarget(target.ID)
		result.Targets = append(result.Targets, projected)
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
	countedFiles := map[string]bool{}
	target := atlas.Target{
		ID: meta.ID, Language: meta.Language, Kind: meta.Kind, Name: meta.Name, Root: meta.Root,
		SharedCode: append([]string(nil), meta.SharedCode...),
		Zones:      []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{},
		Boundaries: []atlas.Boundary{}, OffMap: []atlas.OffMapFile{},
		MapFailure: r.mapFailure[meta.ID],
	}
	if state, ok := r.targets[meta.ID]; ok {
		target.Line, target.Role = state.line, state.role
	}
	zoneOf := map[string]string{}
	for _, zone := range r.zones[meta.ID] {
		boxIDs := append([]string{}, zone.boxes...)
		sort.Slice(boxIDs, func(i, j int) bool { return compactIDLess(boxIDs[i], boxIDs[j]) })
		target.Zones = append(target.Zones, atlas.Zone{ID: zone.id, Title: zone.title, Line: zone.line, BoxIDs: boxIDs})
		for _, id := range boxIDs {
			zoneOf[id] = zone.id
		}
	}
	count := func(fileID string, file atlas.File) {
		if !countedFiles[fileID] {
			target.Files++
			countedFiles[fileID] = true
		}
		target.Symbols += len(file.Symbols)
	}
	for _, owner := range r.boxesOfTarget(meta.ID) {
		box := atlas.Box{
			ID: owner.id, Dir: owner.dir, Title: owner.title, Line: owner.line,
			ZoneID: zoneOf[owner.id], Side: r.side(owner, meta.ID), Open: owner.open,
			Core: owner.core, ForTests: owner.forTests, Unreached: owner.unreached,
			MemberIDs: []string{}, Files: []atlas.File{}, Keys: []atlas.Key{},
		}
		for _, fileID := range owner.files {
			if !contains(r.places[fileID].TargetIDs, meta.ID) {
				continue
			}
			file := r.projectFile(fileID, owner.symbols, owner.id)
			for _, symbol := range file.Symbols {
				if symbol.ObjectID != "" {
					box.MemberIDs = append(box.MemberIDs, symbol.ObjectID)
				}
			}
			box.Files = append(box.Files, file)
			count(fileID, file)
		}
		box.Keys = modelKeys(box.Files)
		if len(box.Keys) == 0 {
			box.Keys = rankedKeys(box.Files)
		}
		target.Boxes = append(target.Boxes, box)
	}
	for _, entry := range r.offMap[meta.ID] {
		symbols := make(map[string]bool, len(entry.symbols))
		for _, id := range entry.symbols {
			symbols[id] = true
		}
		file := r.projectFile(entry.fileID, symbols, "")
		target.OffMap = append(target.OffMap, atlas.OffMapFile{ID: entry.fileID, Reason: entry.reason, BoxID: entry.boxID, File: file})
		count(entry.fileID, file)
	}
	for _, arrow := range r.arrows[meta.ID] {
		if !arrow.drawn {
			continue
		}
		target.Arrows = append(target.Arrows, atlas.Arrow{
			ID: arrow.id, From: arrow.from, To: arrow.to, Calls: arrow.calls, Witnesses: arrow.topWitnesses(), Sentence: arrow.sentence,
		})
	}
	boundaryIDs := make([]string, 0, len(r.boundaries))
	for id := range r.boundaries {
		boundaryIDs = append(boundaryIDs, id)
	}
	sort.Slice(boundaryIDs, func(i, j int) bool { return compactIDLess(boundaryIDs[i], boundaryIDs[j]) })
	for _, id := range boundaryIDs {
		state := r.boundaries[id]
		if !contains(state.place.TargetIDs, meta.ID) {
			continue
		}
		// The target shows only the boxes holding its files; a boundary whose
		// file this target does not hold has no box here to name. One in a
		// file of this target that no part holds names no box and stays.
		boxID := r.boundaryBox(meta.ID, state.place)
		file, known := r.places[state.place.Parent]
		if boxID == "" && !known || known && !contains(file.TargetIDs, meta.ID) {
			continue
		}
		facts := state.place.Boundary
		objectID, factID := facts.ObjectID, ""
		if len(facts.Origins) > 0 {
			for _, origin := range facts.Origins {
				if origin.TargetID == meta.ID {
					objectID, factID = origin.ObjectID, origin.FactID
					break
				}
			}
			if factID == "" {
				continue
			}
		}
		var uses []atlas.DestinationUse
		for _, use := range state.uses {
			if contains(use.TargetIDs, meta.ID) {
				uses = append(uses, cloneDestinationUse(use))
			}
		}
		target.Boundaries = append(target.Boundaries, atlas.Boundary{
			Uses:     uses,
			ObjectID: objectID,
			ID:       state.place.ID, BoxID: boxID, Path: state.place.Path, LineNo: state.place.LineNo, Column: state.place.Column,
			Caller: facts.Caller, Direction: facts.Direction, Kind: state.kind, External: facts.External, Method: facts.Method,
			Values: append([]string{}, facts.Values...), Name: state.name, Line: state.line, FactID: factID,
			Source: facts.Source, Destination: state.destination, Address: state.address, Basis: state.basis,
		})
	}
	return target
}

// projectFile is one file as a part or the off-map record shows it: its line
// and the listed declarations with their captions, aliases and keys. Inside a
// part whose keys were chosen, that choice stands; elsewhere a file keeps the
// selection by file.
func (r *reader) projectFile(fileID string, listed map[string]bool, boxID string) atlas.File {
	place := r.places[fileID]
	line := r.lines[fileID]
	file := atlas.File{
		Path: place.Path, Line: line.value, Source: line.source,
		Open:    r.openFiles[fileID] || !r.budget && !place.File.Generated,
		Asked:   line.source != atlas.SourceUnused,
		Symbols: []atlas.Symbol{},
		Callers: len(place.File.Callers), Callees: len(place.File.Callees),
	}
	for _, decl := range place.File.Decls {
		symbolID := r.symbolID(place.Path, decl.LineNo, decl.Name)
		if !listed[symbolID] {
			continue
		}
		symbol := atlas.Symbol{
			ObjectID: decl.ObjectID,
			ID:       symbolID, Name: decl.Name, Kind: decl.Kind,
			Signature: decl.Signature, Doc: decl.Doc, LineNo: decl.LineNo, Column: decl.Column,
		}
		if line, ok := r.symbolLine[symbol.ID]; ok {
			symbol.Line = line.value
		}
		if knowledge := r.knowledge[symbol.ID]; knowledge != nil && knowledge.Cells["alias"] != "none" {
			symbol.Alias = knowledge.Cells["alias"]
		}
		if boxID != "" && r.keysDecided[boxID] {
			symbol.Key = r.partKeys[symbol.ID]
		} else {
			symbol.Key = contains(r.keys[fileID], symbol.ID)
		}
		file.Symbols = append(file.Symbols, symbol)
	}
	return file
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
