package reading

import (
	"context"
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"sync"
)

func (r *reader) readDesignEagerQueueOracle(ctx context.Context) error {
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
	// The views are built for every target first; the targets below only
	// read them and the maps they fill.
	views := make([]*designView, len(targets))
	for position, target := range targets {
		views[position] = r.designView(target.ID)
		if views[position].err != nil {
			return views[position].err
		}
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
	for _, stage := range []string{lines.StageRoleHelper, lines.StageGroupEnough, lines.StageGroupAssign} {
		if _, asked := r.uses[stage]; asked {
			r.reportStage(stage)
		}
	}
	r.reportStage(lines.StageZones)
	r.reportStage(lines.StageDescribe)
	return nil
}
