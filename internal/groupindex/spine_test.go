package groupindex

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// An input's spine follows its handler's flow while a step's work is one
// call into the next: a constructor call and its class are one step
// (Worker(args), worker.run(), worker.exit()), a callable handed over is a
// registration and no step (signal.signal(term_handler)), and a call into
// the outside is no work. Where the work splits, its branches are named
// with the members of each the step calls, a helper marked (critic,
// 2026-09-30: freqtrade's trade had hidden the bot loop behind "Reaches 65
// more parts deeper").
func TestAnInputsSpineFollowsOneCallAtATime(t *testing.T) {
	g := newReachGraphTest()
	g.functions("start_trading", "term_handler")
	g.declare(programindex.ObjectType, "Worker", "FreqtradeBot", "Configuration", "State")
	g.declare(programindex.ObjectMethod, "Worker.__init__", "Worker.run", "Worker.exit", "Worker._worker", "FreqtradeBot.__init__", "FreqtradeBot.process", "Configuration.get_config")
	g.declare(programindex.ObjectExternalSymbol, "signal.signal")
	owner := map[string]string{"Worker.__init__": "Worker", "Worker.run": "Worker", "Worker.exit": "Worker", "Worker._worker": "Worker",
		"FreqtradeBot.__init__": "FreqtradeBot", "FreqtradeBot.process": "FreqtradeBot", "Configuration.get_config": "Configuration"}
	for position := range g.index.Subjects {
		subject := &g.index.Subjects[position]
		if class, ok := owner[g.names[subject.ID]]; ok {
			subject.Object.OwnerID = g.ids[class]
		}
	}
	g.call("start_trading", "signal.signal", "Worker", "Worker.__init__", "Worker.run", "Worker.exit")
	g.relate(programindex.RelationPassesCallback, programindex.ResolutionExact, "start_trading", "term_handler")
	g.call("Worker.__init__", "FreqtradeBot", "FreqtradeBot.__init__", "Configuration", "Configuration.get_config")
	g.call("Worker.run", "Worker._worker")
	g.call("Worker._worker", "FreqtradeBot.process", "State")
	for position := range g.index.Subjects {
		if g.names[g.index.Subjects[position].ID] == "State" {
			g.index.Subjects[position].Interpretation = &Interpretation{Helper: true}
		}
	}
	g.input("trade", "start_trading")
	g.derive()
	spine := g.reachOf("trade").Spine
	step := func(s SpineStep) []string {
		row := []string{g.names[s.SubjectID]}
		for _, member := range s.Members {
			row = append(row, g.names[member])
		}
		if s.Helper {
			row = append(row, "(helper)")
		}
		return row
	}
	var steps, branches [][]string
	for _, s := range spine.Steps {
		steps = append(steps, step(s))
	}
	for _, s := range spine.Branches {
		branches = append(branches, step(s))
	}
	if want := [][]string{{"start_trading", "start_trading"}, {"Worker", "Worker", "Worker.__init__", "Worker.run", "Worker.exit"}}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps %q, want %q", steps, want)
	}
	if want := [][]string{{"FreqtradeBot", "FreqtradeBot", "FreqtradeBot.__init__", "FreqtradeBot.process"}, {"Configuration", "Configuration", "Configuration.get_config"}, {"State", "State", "(helper)"}}; !reflect.DeepEqual(branches, want) {
		t.Fatalf("branches %q, want %q", branches, want)
	}
}

// WalkPathsThen reads a step's later calls where the route through its
// chosen call ends, innermost step first, as a human reads luaD_call:
// "first luaD_precall, then, only if its result says so, luaV_execute".
// The test's later says which calls are later and when a chosen call
// hands control back (a guarded one does not: it may sit in an arm the
// later calls are alternatives of, or return).
func TestAWalkGoesBackToAStepsLaterCallsWhereARouteEnds(t *testing.T) {
	type graph struct {
		calls   map[string][]string
		picks   map[string][]string
		guarded map[string]bool
		// again is a step's answer when it is asked a second time, among
		// its later calls.
		again map[string][]string
	}
	walk := func(g graph) SpinePath {
		asked := map[string]int{}
		next := func(step SpineStep) []SpineStep {
			var steps []SpineStep
			for _, id := range g.calls[step.SubjectID] {
				steps = append(steps, SpineStep{SubjectID: id})
			}
			return steps
		}
		pick := func(step SpineStep, candidates []SpineStep) []int {
			var chosen []int
			answer := g.picks[step.SubjectID]
			if asked[step.SubjectID]++; asked[step.SubjectID] > 1 {
				answer = g.again[step.SubjectID]
			}
			for _, want := range answer {
				for at, candidate := range candidates {
					if candidate.SubjectID == want {
						chosen = append(chosen, at)
					}
				}
			}
			return chosen
		}
		later := func(step, chosen SpineStep, candidates []SpineStep) []SpineStep {
			if g.guarded[chosen.SubjectID] {
				return nil
			}
			for at, candidate := range candidates {
				if candidate.SubjectID == chosen.SubjectID {
					return candidates[at+1:]
				}
			}
			return nil
		}
		return WalkPathsThen(SpineStep{SubjectID: "main"}, next, pick, later)
	}
	read := func(path SpinePath) []string {
		var said []string
		for path := &path; path != nil; path = path.Then {
			for _, step := range path.Steps {
				line := step.SubjectID
				if step.Resumes != "" {
					line = step.Resumes + " then " + line
				}
				said = append(said, line)
			}
			for _, way := range path.Paths {
				said = append(said, "way "+way.Steps[0].SubjectID)
			}
			for _, candidate := range path.Rest {
				said = append(said, "fork "+candidate.SubjectID)
			}
		}
		return said
	}
	passed := func(path SpinePath, at int) []string {
		var ids []string
		for _, step := range path.Passed[at] {
			ids = append(ids, step.SubjectID)
		}
		return ids
	}
	for name, test := range map[string]struct {
		graph  graph
		want   []string
		passed []string
	}{
		// call: first precall, then execute; precall's route ends.
		"back after the route ends": {graph{calls: map[string][]string{"main": {"precall", "execute"}}, picks: map[string][]string{"main": {"precall"}}},
			[]string{"main", "precall", "main then execute"}, nil},
		// A guarded chosen call hands nothing back: execute stays passed.
		"no way back from a guarded call": {graph{calls: map[string][]string{"main": {"precall", "execute"}}, picks: map[string][]string{"main": {"precall"}}, guarded: map[string]bool{"precall": true}},
			[]string{"main", "precall"}, []string{"execute"}},
		// Whichever of precall's torn ways runs, it returns to main.
		"back after a torn split's ways": {graph{calls: map[string][]string{"main": {"precall", "execute"}, "precall": {"gc", "read"}}, picks: map[string][]string{"main": {"precall"}, "precall": {"gc", "read"}}},
			[]string{"main", "precall", "way gc", "way read", "main then execute"}, nil},
		// An unanswered split goes back as a torn one does.
		"back after an unanswered split": {graph{calls: map[string][]string{"main": {"precall", "execute"}, "precall": {"gc", "read"}}, picks: map[string][]string{"main": {"precall"}}},
			[]string{"main", "precall", "fork gc", "fork read", "main then execute"}, nil},
		// Several later calls are a split; torn between them, the path
		// ends and they stay main's passed calls.
		"a torn choice among later calls ends the path": {graph{calls: map[string][]string{"main": {"precall", "execute", "close"}}, picks: map[string][]string{"main": {"precall"}}, again: map[string][]string{"main": {"execute", "close"}}},
			[]string{"main", "precall"}, []string{"execute", "close"}},
		// One of several later calls chosen goes on; the rest stay passed.
		"a decided choice among later calls": {graph{calls: map[string][]string{"main": {"precall", "execute", "close"}}, picks: map[string][]string{"main": {"precall"}}, again: map[string][]string{"main": {"close"}}},
			[]string{"main", "precall", "main then close"}, []string{"execute"}},
		// A later call already on the path is not read again.
		"a later call already walked": {graph{calls: map[string][]string{"main": {"precall", "execute"}, "precall": {"execute"}}, picks: map[string][]string{"main": {"precall"}}},
			[]string{"main", "precall", "execute"}, []string{"execute"}},
		// The innermost step is read first, then its caller.
		"innermost first": {graph{calls: map[string][]string{"main": {"call", "close"}, "call": {"precall", "execute"}}, picks: map[string][]string{"main": {"call"}, "call": {"precall"}}},
			[]string{"main", "call", "precall", "call then execute", "main then close"}, nil},
	} {
		path := walk(test.graph)
		if got := read(path); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: path = %q, want %q", name, got, test.want)
		}
		if got := passed(path, 0); !reflect.DeepEqual(got, test.passed) {
			t.Errorf("%s: main's passed calls = %q, want %q", name, got, test.passed)
		}
	}
}
