package reading

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// readingStages are the table stages of the ordinary walk.
var readingStages = map[string]func(*reader, context.Context) error{
	lines.StageDirectories: (*reader).readDirectories,
	lines.StageFiles:       (*reader).readFiles,
	lines.StageSymbols:     (*reader).readSymbols,
	lines.StageAPI:         (*reader).readAPI,
	lines.StageBoundaries:  (*reader).readBoundaries,
	lines.StageLayers:      (*reader).readLayers,
	lines.StageZones:       (*reader).readDesign,
	lines.StageArrows:      (*reader).readArrows,
	lines.StageCore:        (*reader).readCore,
	lines.StageKeys:        (*reader).readKeys,
	lines.StageTargets:     (*reader).readTargets,
	lines.StageJoints:      (*reader).readJoints,
}

// readingPhases orders the walk. A phase is one or more chains of stages;
// the chains of one phase read nothing another writes, so they run at once
// and join before the next phase. After the files, the symbols; the outside
// symbols, the boundaries their roles make and the layers between them; and
// the zones are such chains. The arrows read all three. Flattened, the
// phases are the step order: tables.md, rejected rows, compact IDs and
// every request stay those of the serial walk.
var readingPhases = [][][]string{
	{{lines.StageDirectories}},
	{{lines.StageFiles}},
	{{lines.StageSymbols}, {lines.StageAPI, lines.StageBoundaries, lines.StageLayers}, {lines.StageZones}},
	{{lines.StageArrows}},
	{{lines.StageCore}},
	{{lines.StageKeys}},
	{{lines.StageTargets}},
	{{lines.StageJoints}},
}

// recallStages restore remembered descriptions before question-only reading,
// one after another and without a model.
var recallStages = []string{lines.StageDirectories, lines.StageFiles, lines.StageSymbols, lines.StageAPI, lines.StageBoundaries, lines.StageLayers}

// walk runs the stages up to Through, or all of them, and returns the last
// one it ran. tables.md and knowledge.json are saved after every serial
// stage and after every join.
func (r *reader) walk(ctx context.Context) (string, error) {
	included := make(map[string]bool)
	stop := false
	for _, phase := range readingPhases {
		for _, chain := range phase {
			for _, name := range chain {
				if !stop {
					included[name] = true
					stop = name == r.opts.Through
				}
			}
		}
	}
	through := ""
	for _, phase := range readingPhases {
		var chains [][]string
		for _, chain := range phase {
			var kept []string
			for _, name := range chain {
				if included[name] {
					kept = append(kept, name)
				}
			}
			if len(kept) > 0 {
				chains = append(chains, kept)
			}
		}
		switch len(chains) {
		case 0:
			continue
		case 1:
			for _, name := range chains[0] {
				if err := readingStages[name](r, ctx); err != nil {
					return "", err
				}
				through = name
				if err := r.saveProgress(); err != nil {
					return "", err
				}
			}
		default:
			if err := r.fork(ctx, chains); err != nil {
				return "", err
			}
			last := chains[len(chains)-1]
			through = last[len(last)-1]
			if err := r.saveProgress(); err != nil {
				return "", err
			}
		}
	}
	return through, nil
}

func (r *reader) saveProgress() error {
	if err := r.saveTables(); err != nil {
		return err
	}
	return r.persistKnowledge()
}

// fork runs chains of stages at once, each on its own view of the reader,
// and joins them all. The first failure cancels the other chains and is the
// error returned; the cancellations it causes are not. Nothing a chain did
// reaches the reader unless every chain succeeded.
func (r *reader) fork(ctx context.Context, chains [][]string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	views := make([]*reader, len(chains))
	var failed sync.Once
	var failure error
	var wg sync.WaitGroup
	for i, chain := range chains {
		views[i] = r.view(chain)
		wg.Add(1)
		go func(view *reader, chain []string) {
			defer wg.Done()
			for _, name := range chain {
				if err := readingStages[name](view, ctx); err != nil {
					failed.Do(func() {
						failure = err
						cancel()
					})
					return
				}
			}
		}(views[i], chain)
	}
	wg.Wait()
	if failure != nil {
		return failure
	}
	for i, chain := range chains {
		r.join(views[i], chain)
	}
	return nil
}

// view is the reader one chain runs on. It shares the graph, the lines of
// the stages before it, the file inventory and the locked knowledge and
// response caches; it prints, counts and rejects into its own sinks. The
// boundaries rewrite the places of the boundaries they read, so their chain
// works on its own copy of the places.
func (r *reader) view(chain []string) *reader {
	view := *r
	view.tables = strings.Builder{}
	view.rejected = nil
	view.uses = make(map[string]*atlas.StageUse)
	view.started = make(map[string]time.Time)
	if slices.Contains(chain, lines.StageBoundaries) {
		view.places = maps.Clone(r.places)
	}
	return &view
}

// join takes back what the chain's stages own and adds what the chain
// printed, counted and rejected after everything before it. Maps a stage
// only fills (lines of symbols, selected keys) are shared with the view
// already; what a stage replaces comes back here, stage by stage, with the
// ID counters it owns, so the reader stays whole for the stages after.
func (r *reader) join(view *reader, chain []string) {
	r.tables.WriteString(view.tables.String())
	r.rejected = append(r.rejected, view.rejected...)
	maps.Copy(r.uses, view.uses)
	maps.Copy(r.started, view.started)
	for _, name := range chain {
		switch name {
		case lines.StageSymbols:
			r.symbolSelections = view.symbolSelections
		case lines.StageAPI:
			r.api = view.api
		case lines.StageBoundaries:
			r.places, r.boundaries = view.places, view.boundaries
			r.nextBoundary, r.boundaryIDs = view.nextBoundary, view.boundaryIDs
		case lines.StageLayers:
			r.roles = view.roles
		case lines.StageZones:
			r.boxes, r.designBoxOf, r.zones = view.boxes, view.designBoxOf, view.zones
			r.designFiles, r.designSubjects = view.designFiles, view.designSubjects
			r.nextPart, r.nextZone = view.nextPart, view.nextZone
		}
	}
}

// joinView adds what a view of one stage's own work printed, counted and
// rejected after everything before it; its stages keep their earliest start.
func (r *reader) joinView(view *reader) {
	r.tables.WriteString(view.tables.String())
	r.rejected = append(r.rejected, view.rejected...)
	for stage, use := range view.uses {
		total := r.use(stage)
		total.Rows += use.Rows
		total.Windows += use.Windows
		total.Live += use.Live
		total.Cached += use.Cached
		total.Reused += use.Reused
		total.Rejected += use.Rejected
		total.Given += use.Given
	}
	for stage, at := range view.started {
		if first, ok := r.started[stage]; !ok || at.Before(first) {
			r.started[stage] = at
		}
	}
}
