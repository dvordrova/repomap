package run

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// The documentation reduction runs beside the first target's native
// analysis. Its refusal is still the run's reported cause, and no page is
// published without the reduced documentation every page is built from.
func TestDocumentationReductionFailureStopsTheRunBesideNativeAnalysis(t *testing.T) {
	repositoryRoot := ordinaryGraphGoRepository(t)
	debugDir := t.TempDir()
	refused := errors.New("documentation preset refused")
	err := runDefaultWithDeps(repositoryRoot, []string{
		"--no-model", "--target", "example.com/common-page@.::example.com/common-page/cmd/app",
		"--no-open", "--debug-dir", debugDir,
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: io.Discard,
		llmBatchConcurrency: 1, llmBatchController: &llm.BatchController{},
		serveReport: func(context.Context, reportserver.Options) error {
			t.Fatal("a run without its documentation served a report")
			return nil
		},
		openReport: func(string) error { return nil },
		runDocumentationReduce: func(
			context.Context, llm.Executor, llm.Provider, readmetargetscout.GuidanceSnapshot,
		) (documentationreduce.Result, error) {
			return documentationreduce.Result{}, refused
		},
	})
	if !errors.Is(err, refused) {
		t.Fatalf("run error = %v, want the documentation refusal", err)
	}
	if published := ordinaryGraphRunDirs(t, debugDir); len(published) != 0 {
		t.Fatalf("published run directories without documentation: %v", published)
	}
}

func laneTestSlots(adapters ...repositoryTargetAdapter) []targetPageSlot {
	slots := make([]targetPageSlot, len(adapters))
	for position, adapter := range adapters {
		slots[position] = targetPageSlot{
			position: position,
			target:   repositoryTypedTarget{Key: repositoryTargetKey{Adapter: adapter, Ref: string(rune('a' + position))}},
		}
	}
	return slots
}

func analyzedLaneSlot(slot *targetPageSlot) {
	slot.outcome = &targetoutcome.Outcome{}
	slot.published = &targetPublishedRun{RunID: slot.target.Key.Ref}
}

// Targets of different adapters share nothing native, so their pages run
// side by side; targets of one adapter share that adapter's workspace or
// parser, so they stay one at a time in plan order.
func TestTargetPageLanesRunAdaptersSideBySideAndOneAdapterInPlanOrder(t *testing.T) {
	slots := laneTestSlots(
		repositoryTargetAdapterGo, repositoryTargetAdapterJSTS,
		repositoryTargetAdapterGo, repositoryTargetAdapterJSTS,
	)
	jstsStarted := make(chan struct{})
	var mu sync.Mutex
	running := make(map[repositoryTargetAdapter]int)
	visited := make(map[repositoryTargetAdapter][]int)
	runTargetPageLanes(t.Context(), slots, func(_ context.Context, slot *targetPageSlot) {
		adapter := slot.target.Key.Adapter
		mu.Lock()
		running[adapter]++
		if running[adapter] > 1 {
			t.Errorf("two %s pages ran at once", adapter)
		}
		visited[adapter] = append(visited[adapter], slot.position)
		mu.Unlock()
		switch slot.position {
		case 0:
			// The first Go page finishes only once a JSTS page has started.
			select {
			case <-jstsStarted:
			case <-time.After(10 * time.Second):
				t.Error("the JSTS lane did not start beside the Go lane")
			}
		case 1:
			close(jstsStarted)
		}
		mu.Lock()
		running[adapter]--
		mu.Unlock()
		analyzedLaneSlot(slot)
	})
	if got := visited[repositoryTargetAdapterGo]; len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("Go pages ran in order %v, want [0 2]", got)
	}
	if got := visited[repositoryTargetAdapterJSTS]; len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("JSTS pages ran in order %v, want [1 3]", got)
	}
	fold := foldTargetPageSlots(slots)
	if fold.stop != nil || len(fold.runs) != 4 {
		t.Fatalf("fold = %+v", fold)
	}
	for position, run := range fold.runs {
		if run.RunID != slots[position].target.Key.Ref {
			t.Fatalf("run %d is %q, want plan order", position, run.RunID)
		}
	}
}

// A target that is not analyzed is an explicit outcome: it never cancels a
// sibling, in its own lane or another.
func TestTargetPageLanesKeepSiblingsOfATargetThatIsNotAnalyzed(t *testing.T) {
	slots := laneTestSlots(
		repositoryTargetAdapterGo, repositoryTargetAdapterJSTS, repositoryTargetAdapterGo,
	)
	notAnalyzed := errors.New("compiler refused the target")
	runTargetPageLanes(t.Context(), slots, func(ctx context.Context, slot *targetPageSlot) {
		if slot.position == 0 {
			slot.outcome = &targetoutcome.Outcome{}
			slot.failure = notAnalyzed
			return
		}
		if err := ctx.Err(); err != nil {
			t.Errorf("target %d saw its sibling's failure as cancellation: %v", slot.position, err)
		}
		analyzedLaneSlot(slot)
	})
	fold := foldTargetPageSlots(slots)
	if fold.stop != nil || len(fold.outcomes) != 3 || len(fold.targetErrors) != 1 ||
		!errors.Is(fold.targetErrors[0], notAnalyzed) || len(fold.runs) != 2 ||
		fold.runs[0].RunID != "b" || fold.runs[1].RunID != "c" {
		t.Fatalf("fold = %+v", fold)
	}
}

// A failure that ends the publication cancels the other lanes, and the run
// reports that failure, not the cancellation an earlier target saw.
func TestTargetPageLanesReportTheFailureNotTheCancellationItCaused(t *testing.T) {
	slots := laneTestSlots(
		repositoryTargetAdapterGo, repositoryTargetAdapterJSTS,
		repositoryTargetAdapterJSTS, repositoryTargetAdapterGo,
	)
	refused := errors.New("target outcome could not be recorded")
	var mu sync.Mutex
	ran := make(map[int]bool)
	runTargetPageLanes(t.Context(), slots, func(ctx context.Context, slot *targetPageSlot) {
		mu.Lock()
		ran[slot.position] = true
		mu.Unlock()
		switch slot.position {
		case 0:
			select {
			case <-ctx.Done():
				slot.stop, slot.stopCanceled = ctx.Err(), true
			case <-time.After(10 * time.Second):
				t.Error("the Go lane was not canceled by its sibling's failure")
				analyzedLaneSlot(slot)
			}
		case 1:
			slot.stop = refused
		default:
			analyzedLaneSlot(slot)
		}
	})
	if ran[2] || ran[3] {
		t.Fatalf("stopped lanes started later targets: %v", ran)
	}
	fold := foldTargetPageSlots(slots)
	if !errors.Is(fold.stop, refused) {
		t.Fatalf("reported failure = %v, want the sibling's refusal", fold.stop)
	}
}

// Python and JavaScript/TypeScript pages run in their own lanes through the
// real native analyzers, and still publish one report whose targets keep
// their plan positions.
func TestPythonAndJSTSPagesPublishOneReportInPlanOrder(t *testing.T) {
	repositoryRoot := ordinaryGraphCumulativeRepository(t)
	debugDir := t.TempDir()
	var served []report.RunReceipt
	err := runDefaultWithDeps(repositoryRoot, []string{
		"--no-model", "--target", "python:.:guard:src/acme/api,jsts:package.json",
		"--no-open", "--debug-dir", debugDir,
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: io.Discard,
		llmBatchConcurrency: 1, llmBatchController: &llm.BatchController{},
		serveReport: func(_ context.Context, options reportserver.Options) error {
			served = options.Runs
			return nil
		},
		openReport: func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("two-adapter run: %v", err)
	}
	published := ordinaryGraphRunDirs(t, debugDir)
	if len(published) != 1 || len(served) != 1 {
		t.Fatalf("published %v, served %d receipts; want one repository report", published, len(served))
	}
	data := served[0].Data()
	if data == nil || data.TargetOutcomePortfolio == nil || data.ProgramPortfolio == nil {
		t.Fatal("the served repository report carries no target inventory")
	}
	outcomes := data.TargetOutcomePortfolio.Outcomes
	if len(outcomes) != 2 || outcomes[0].SelectedTargetID != "t1" || outcomes[1].SelectedTargetID != "t2" ||
		outcomes[0].Language != "python" || outcomes[0].State != targetoutcome.StateAnalyzed ||
		outcomes[1].State != targetoutcome.StateAnalyzed {
		t.Fatalf("target outcomes = %+v", outcomes)
	}
	var entries []programindex.Index
	if err := data.ProgramPortfolio.ReadProgramIndexes(func(index programindex.Index) error {
		entries = append(entries, index)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Target.ID != "t1" || entries[0].Target.Language != "python" ||
		entries[1].Target.ID != "t2" || entries[1].Target.Language == "python" {
		t.Fatalf("program portfolio targets = %+v", entries)
	}
}
