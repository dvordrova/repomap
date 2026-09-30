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
