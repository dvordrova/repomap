package contracttest

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// flowSplit is one split a fixture's Main flow walk asked about: the step
// and its options by name.
type flowSplit struct {
	step    string
	options []string
	// meanings are the options' criteria, as the categorizer reads them.
	meanings []string
}

// flowPreset decides each split of a fixture's Main flow: the option to go
// on through for a step, or a near-tie. A split it does not know fails the
// request, so no split is answered by accident.
type flowPreset struct {
	choose map[string]string
	tie    map[string]bool
	// torn names, for a step, the two options a near-tie is between, where
	// they are not its first two.
	torn  map[string][2]string
	mu    sync.Mutex
	asked []flowSplit
}

func (preset *flowPreset) categorizer() *typesafetest.Categorizer {
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		step, _ := question.Item["step"].(string)
		var options, meanings []string
		for _, option := range question.Options {
			options, meanings = append(options, option.Name), append(meanings, option.Meaning)
		}
		preset.mu.Lock()
		preset.asked = append(preset.asked, flowSplit{step: step, options: options, meanings: meanings})
		preset.mu.Unlock()
		if preset.tie[step] && len(options) > 1 {
			return llm.Verdict{Choice: options[0], Probabilities: map[string]float64{options[0]: 0.5, options[1]: 0.45}}, true
		}
		if pair, torn := preset.torn[step]; torn && slices.Contains(options, pair[0]) && slices.Contains(options, pair[1]) {
			return llm.Verdict{Choice: pair[0], Probabilities: map[string]float64{pair[0]: 0.5, pair[1]: 0.45}}, true
		}
		chosen, known := preset.choose[step]
		if !known || !slices.Contains(options, chosen) {
			return llm.Verdict{}, false
		}
		return typesafetest.Choose(chosen), true
	}}
}

// walkFixtureFlow walks a fixture program's Main flow as an ordinary run
// does: the overview names the program, the preset decides each split.
func walkFixtureFlow(t *testing.T, layer facts.Result, indexes []groupindex.Index, graph atlas.Graph, targetID string, preset *flowPreset) orientation.Result {
	t.Helper()
	noClaims, err := claims.Seal(claims.Result{Revision: "test", Claims: []claims.Claim{}})
	if err != nil {
		t.Fatal(err)
	}
	result, rejected, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, &capturedOrientation{flowTarget: targetID},
		orientation.Input{RepositoryName: "fixture", Facts: layer, Claims: noClaims, Groups: indexes, Graph: graph, Categorizer: preset.categorizer()})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rejected {
		if row.Section != "flow_fork" {
			t.Fatalf("the orientation refused a row: %+v", row)
		}
	}
	return result
}

// flowPath reads a walked flow's steps by name with how each is reached,
// then its fork's branches.
func flowPath(indexes []groupindex.Index, flow orientation.MainFlow) []string {
	names := map[string]string{}
	for _, index := range indexes {
		for _, subject := range index.Subjects {
			if subject.Object != nil {
				names[index.Target.ID+"."+subject.ID] = subject.Object.Name
			}
		}
	}
	// A dispatch site's alternative is said with the declaration holding
	// the site: "one of 3 from loopProcessEvents".
	reached := func(targetID, via, site string) string {
		if site != "" {
			return via + " from " + names[targetID+"."+site]
		}
		return via
	}
	// Where the flow parts, each way's steps are numbered by the way ("2:
	// setCommand"), then come the fork's unfollowed candidates ("? ...").
	var path []string
	var walk func(prefix string, steps []orientation.FlowStep)
	walk = func(prefix string, steps []orientation.FlowStep) {
		for _, step := range steps {
			path = append(path, fmt.Sprintf("%s%s (%s)", prefix, names[step.TargetID+"."+step.SubjectID], reached(step.TargetID, step.Via, step.Site)))
			for number, way := range step.Paths {
				walk(fmt.Sprintf("%s%d: ", prefix, number+1), way.Steps)
			}
			for _, branch := range step.Branches {
				path = append(path, fmt.Sprintf("%s? %s (%s)", prefix, names[step.TargetID+"."+branch.SubjectID], reached(step.TargetID, branch.Via, branch.Site)))
			}
		}
	}
	walk("", flow.Steps)
	return path
}

// flowJoins reads where each way of a walked flow ends going on as another
// way ("runSession joins acceptJob").
func flowJoins(indexes []groupindex.Index, flow orientation.MainFlow) []string {
	names := map[string]string{}
	for _, index := range indexes {
		for _, subject := range index.Subjects {
			if subject.Object != nil {
				names[index.Target.ID+"."+subject.ID] = subject.Object.Name
			}
		}
	}
	var joins []string
	var walk func(steps []orientation.FlowStep)
	walk = func(steps []orientation.FlowStep) {
		for _, step := range steps {
			for _, joined := range step.Joins {
				joins = append(joins, names[step.TargetID+"."+step.SubjectID]+" joins "+names[step.TargetID+"."+joined.SubjectID])
			}
			for _, way := range step.Paths {
				walk(way.Steps)
			}
		}
	}
	walk(flow.Steps)
	return joins
}

// assertPassedHanded walks a fixture's flow from start, which calls runner
// and hands it handed: the step goes on through runner and keeps handed
// beside it as handed to runner, never as a call, which the page reads
// under "also hands over:" (control review, 2026-10-03: Lua 5.1.5's
// handle_script had listed laction, which docall hands to signal, among
// its calls).
func assertPassedHanded(t *testing.T, index groupindex.Index, layer facts.Result, graph atlas.Graph, start, path, runner, handed string) {
	t.Helper()
	preset := &flowPreset{choose: map[string]string{start: runner}}
	flow := walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, start, path), preset)
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	var passed []string
	for _, branch := range flow.Steps[0].Passed {
		passed = append(passed, names[branch.SubjectID]+" "+branch.Via)
	}
	if len(flow.Steps) < 2 || names[flow.Steps[1].SubjectID] != runner || !slices.Equal(passed, []string{handed + " handed to " + runner}) {
		t.Fatalf("%s goes on to %v passing %q, want %s passing %s handed to %s", start, flowPath([]groupindex.Index{index}, flow), passed, runner, handed, runner)
	}
}

// assertWaysJoin walks a fixture's flow from start, its split torn, where
// second calls first, the other way's start, and other: second's way is
// asked between both, joins first and is never walked into it (control
// review, 2026-10-03: two ways from one start may meet, as Lua 5.1.5's
// script way reaches lua_pcall, the prompt way's start, through docall).
func assertWaysJoin(t *testing.T, index groupindex.Index, layer facts.Result, graph atlas.Graph, start, path, second, first, other string) {
	t.Helper()
	preset := &flowPreset{tie: map[string]bool{start: true}, choose: map[string]string{second: first}}
	flow := walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, start, path), preset)
	// The ways come in the options' order, by name.
	want := []string{start + " ()", "1: " + first + " (called)", "2: " + second + " (called)"}
	if second < first {
		want = []string{start + " ()", "1: " + second + " (called)", "2: " + first + " (called)"}
	}
	if got := flowPath([]groupindex.Index{index}, flow); !slices.Equal(got, want) {
		t.Fatalf("%s's flow = %q\nwant %q", start, got, want)
	}
	if got, want := flowJoins([]groupindex.Index{index}, flow), []string{second + " joins " + first}; !slices.Equal(got, want) {
		t.Fatalf("the ways' joins = %q, want %q", got, want)
	}
	if len(preset.asked) != 2 || preset.asked[1].step != second || !slices.Equal(preset.asked[1].options, []string{first, other}) && !slices.Equal(preset.asked[1].options, []string{other, first}) {
		t.Fatalf("splits asked %+v, want %s between %s and %s", preset.asked, second, first, other)
	}
}

// assertPreparedThenExecuted walks a fixture's run that prepares, then
// executes only on preparing's result, as Lua's luaD_call runs luaV_execute
// only if luaD_precall says so: through prepare, the route goes on into
// collect and what it calls; where it ends, the path is back in run, at
// execute, still under its condition, and on into what execute calls.
// Nothing is asked of the way back: execute is run's one later call. A
// language whose adapter records no guards (guards false: JS/TS, Clojure)
// cannot say execute runs under a condition: the path ends where the route
// does, execute among run's passed calls, a missing equivalent.
func assertPreparedThenExecuted(t *testing.T, index groupindex.Index, layer facts.Result, graph atlas.Graph, run, path, prepare, collect, collected, execute, executed string, guards bool) {
	t.Helper()
	preset := &flowPreset{choose: map[string]string{run: prepare}}
	flow := walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, run, path), preset)
	want := []string{run + " ()", prepare + " (called)", collect + " (called)", collected + " (called)"}
	if guards {
		want = append(want, execute+" (called)", executed+" (called)")
	}
	if got := flowPath([]groupindex.Index{index}, flow); !slices.Equal(got, want) {
		t.Fatalf("%s's flow = %q\nwant %q", run, got, want)
	}
	if len(preset.asked) != 1 || preset.asked[0].step != run {
		t.Fatalf("splits asked: %+v, want one at %s", preset.asked, run)
	}
	if !guards {
		if passed := flow.Steps[0].Passed; len(passed) != 1 || passed[0].SubjectID != subjectNamed(t, index, execute, path) {
			t.Fatalf("%s passing %+v, want %s", run, passed, execute)
		}
		return
	}
	back := flow.Steps[4]
	if back.Resumes != flow.Steps[0].SubjectID || back.Guard == nil || back.Guard.Kind != programindex.GuardBranch || len(flow.Steps[0].Passed) != 0 {
		t.Fatalf("%s read back in %q under %+v, %s passing %+v; want back in %s under its condition", execute, back.Resumes, back.Guard, run, flow.Steps[0].Passed, run)
	}
}

// assertNoRepeats says a walked flow names no declaration twice.
func assertNoRepeats(t *testing.T, flow orientation.MainFlow) {
	t.Helper()
	seen := map[string]bool{}
	for _, step := range flow.Steps {
		if seen[step.SubjectID] {
			t.Fatalf("%s is on the flow twice: %+v", step.SubjectID, flow.Steps)
		}
		seen[step.SubjectID] = true
	}
}

// assertNoTestCode says no split offered a declaration of a test source.
func assertNoTestCode(t *testing.T, preset *flowPreset, testNames ...string) {
	t.Helper()
	for _, split := range preset.asked {
		for _, option := range split.options {
			for _, name := range testNames {
				if strings.Contains(option, name) {
					t.Fatalf("test code %q was a candidate at %s", option, split.step)
				}
			}
		}
	}
}

// pythonFlowFixture is the cumulative Python fixture's library as an
// ordinary run reads it, with its facts, for a flow walked from one of its
// functions.
func pythonFlowFixture(t *testing.T) (groupindex.Index, facts.Result, atlas.Graph) {
	t.Helper()
	root, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var target pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.Kind == pythontarget.KindLibrary && candidate.ProjectDir == "." {
			target = candidate
			break
		}
	}
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, target)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(string, map[string]any, []string) (string, bool) { return "", false }}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	return projected, layer, graph
}

// subjectNamed is the subject of an index a function of that name, written
// in that file, is.
func subjectNamed(t *testing.T, index groupindex.Index, name, path string) string {
	t.Helper()
	for _, subject := range index.Subjects {
		if subject.Object != nil && subject.Object.Name == name && subject.Object.Location != nil && subject.Object.Location.Path == path {
			return subject.ID
		}
	}
	t.Fatalf("no %s in %s", name, path)
	return ""
}

// subjectAt is the subject a declaration of that name at that line is.
func subjectAt(t *testing.T, index groupindex.Index, name, path string, line int) string {
	t.Helper()
	for _, subject := range index.Subjects {
		if subject.Object != nil && subject.Object.Name == name && subject.Object.Location != nil && subject.Object.Location.Path == path && subject.Object.Location.Line == line {
			return subject.ID
		}
	}
	t.Fatalf("no %s at %s:%d", name, path, line)
	return ""
}

// walkFixtureFrom walks a fixture program's flow from one of its functions.
func walkFixtureFrom(t *testing.T, index groupindex.Index, layer facts.Result, graph atlas.Graph, entry string, preset *flowPreset) orientation.MainFlow {
	t.Helper()
	flow, rejected, err := orientation.WalkFlow(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, preset.categorizer(),
		orientation.Input{Facts: layer, Groups: []groupindex.Index{index}, Graph: graph}, index.Target.ID, entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rejected {
		if row.Section != "flow_fork" {
			t.Fatalf("the walk refused a row: %+v", row)
		}
	}
	return flow
}

// The Python fixture's flows, walked from its functions (design skeptic,
// 2026-09-30): tool_cli's main calls build_parser, its one candidate, with
// no question; build_parser's work splits between the parser helpers it
// calls and run_init, which it hands to argparse's set_defaults. A class's
// member handing a private helper of its class to another that calls it is
// one step: Loop.run's _throttle(func=self._process) goes on to _process's
// accept_client with no question. Throttle.run's public members are steps
// of their own: its work splits between throttle and the two it hands
// throttle, and the test's lambda it is also handed is no candidate.
func TestPythonFixtureMainFlowsWalkByCode(t *testing.T) {
	index, layer, graph := pythonFlowFixture(t)
	tool := &flowPreset{choose: map[string]string{"build_parser": "run_init"}}
	flow := walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, "main", "src/fixture_app/tool_cli.py"), tool)
	if got, want := flowPath([]groupindex.Index{index}, flow), []string{"main ()", "build_parser (called)",
		"run_init (handed to argparse.ArgumentParser.add_subparsers.add_parser.set_defaults)", "bare_variant (called)"}; !slices.Equal(got, want) {
		t.Fatalf("tool_cli's flow = %q\nwant %q", got, want)
	}
	if len(tool.asked) != 1 || !slices.Equal(tool.asked[0].options, []string{"add_common", "add_output", "run_init"}) {
		t.Fatalf("splits asked: %+v", tool.asked)
	}
	assertNoRepeats(t, flow)
	loop := &flowPreset{}
	if got := flowPath([]groupindex.Index{index}, walkFixtureFrom(t, index, layer, graph, subjectAt(t, index, "run", "src/fixture_app/stored_callbacks.py", 97), loop)); len(loop.asked) != 0 ||
		!slices.Equal(got[len(got)-1:], []string{"accept_client (called)"}) {
		t.Fatalf("Loop.run's flow = %q after %d questions, want accept_client with none", got, len(loop.asked))
	}
	throttle := &flowPreset{tie: map[string]bool{"run": true}}
	if got := flowPath([]groupindex.Index{index}, walkFixtureFrom(t, index, layer, graph, subjectAt(t, index, "run", "src/fixture_app/stored_callbacks.py", 55), throttle)); !slices.Equal(got, []string{"run ()", "1: process_running (handed to throttle)", "1: accept_client (called)",
		"2: process_stopped (handed to throttle)", "2: flush_replies (called)", "? throttle (called)"}) {
		t.Fatalf("Throttle.run's flow = %q", got)
	}
	assertNoTestCode(t, throttle, "lambda", "throttle_a_lambda")
	assertWaysJoin(t, index, layer, graph, "start_session", "src/fixture_app/stored_callbacks.py", "run_session", "accept_client", "flush_replies")
	assertPassedHanded(t, index, layer, graph, "start_once", "src/fixture_app/stored_callbacks.py", "run_once", "accept_client")
	assertPreparedThenExecuted(t, index, layer, graph, "watch_run", "src/fixture_app/stored_callbacks.py", "watch_prepare", "watch_collect", "accept_client", "watch_execute", "flush_replies", true)
}

// goFlowFixture is one command of the cumulative Go fixture as an ordinary
// run reads it, with its facts.
func goFlowFixture(t *testing.T, pkg string) (groupindex.Index, facts.Result, atlas.Graph) {
	t.Helper()
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	app := analyzeGoFixture(t, root, repository, pkg, "flow walk")
	index, err := goadapter.Build(repository, app.target, app.origins, app.direct, app.external, app.core, app.dynamic, app.tests)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	// Every part is one the program exists for: the filter by core parts
	// keeps the fixture's small handlers.
	preset := &inputsPreset{decide: func(column string, _ map[string]any, _ []string) (string, bool) { return "domain", column == "role" }}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	return projected, layer, graph
}

// The Go fixture's flows (design skeptic, 2026-09-30). The worker's main
// splits between the background start and its poll interval; past
// StartBackground's split, StartCommitWorker's goroutine is its one
// candidate, RunCommitWorker handed to go, which calls commitPendingBatches:
// a chain asked nothing. The app's main calls Exercise, its one candidate
// whose closure enters a core part, and a near-tie at Exercise's split ends
// the flow there, a named fork. registerLevelRoute hands getLevel to
// net/http.HandleFunc, but getLevel does nothing and its part is no core
// part: no candidate, so its flow is itself.
func TestGoFixtureMainFlowsWalkByCode(t *testing.T) {
	worker, layer, graph := goFlowFixture(t, goFixtureRootPackage+"/cmd/worker")
	preset := &flowPreset{choose: map[string]string{"main": "StartBackground", "StartBackground": "StartCommitWorker"}}
	flow := walkFixtureFrom(t, worker, layer, graph, subjectNamed(t, worker, "main", "cmd/worker/main.go"), preset)
	if got, want := flowPath([]groupindex.Index{worker}, flow), []string{"main ()", "StartBackground (called)", "StartCommitWorker (called)",
		"RunCommitWorker (handed to go)", "commitPendingBatches (called)"}; !slices.Equal(got, want) {
		t.Fatalf("the worker's flow = %q\nwant %q", got, want)
	}
	// Where RunCommitWorker's route ends, the path is back in
	// StartBackground among the calls it writes after StartCommitWorker:
	// one question, left unanswered here, so the path ends.
	if len(preset.asked) != 3 || !slices.Equal(preset.asked[1].options, []string{"RetryCommit", "ScheduleCommit", "StartCommitWorker", "StartCompactor", "StartSweeper", "WarmCache"}) ||
		preset.asked[2].step != "StartBackground" || !slices.Equal(preset.asked[2].options, []string{"RetryCommit", "ScheduleCommit", "StartCompactor", "StartSweeper", "WarmCache"}) {
		t.Fatalf("splits asked: %+v", preset.asked)
	}
	assertNoRepeats(t, flow)
	app, layer, graph := goFlowFixture(t, goFixtureAppPackage)
	tie := &flowPreset{tie: map[string]bool{"Exercise": true}}
	if got := flowPath([]groupindex.Index{app}, walkFixtureFrom(t, app, layer, graph, subjectNamed(t, app, "main", "cmd/app/main.go"), tie)); !slices.Equal(got, []string{"main ()", "Exercise (called)",
		"1: Name (called)", "2: OpenUserRows (called)", "2: newUserRows (called)", "? registerSignalConsumer (called)", "? createFixtureState (called)"}) || len(tie.asked) != 1 {
		t.Fatalf("the app's flow = %q after %d questions", got, len(tie.asked))
	}
	route := &flowPreset{}
	if got := flowPath([]groupindex.Index{app}, walkFixtureFrom(t, app, layer, graph, subjectNamed(t, app, "registerLevelRoute", "cmd/app/main.go"), route)); !slices.Equal(got, []string{"registerLevelRoute ()"}) || len(route.asked) != 0 {
		t.Fatalf("registerLevelRoute's flow = %q", got)
	}
	assertWaysJoin(t, app, layer, graph, "StartSession", "internal/storefixture/command_table.go", "runSession", "acceptJob", "flushJob")
	assertPassedHanded(t, app, layer, graph, "StartOnce", "internal/storefixture/command_table.go", "runOnce", "acceptJob")
	assertPreparedThenExecuted(t, app, layer, graph, "WatchRun", "internal/storefixture/command_table.go", "watchPrepare", "watchCollect", "acceptJob", "watchExecute", "flushJob", true)
}

// jstsFlowFixture is the cumulative JS/TS fixture as an ordinary run reads
// it, with its facts.
func jstsFlowFixture(t *testing.T) (groupindex.Index, facts.Result, atlas.Graph) {
	t.Helper()
	root, repository := materializeFixtureRepository(t, "jsts")
	_, index, _, err := jstsproject.Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, _ map[string]any, _ []string) (string, bool) { return "domain", column == "role" }}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "typescript", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	return projected, layer, graph
}

// The JS/TS fixture's flows (design skeptic, 2026-09-30):
// registerChainedCallbacks hands handleOrder and recordOrder to its own
// CallbackChain's map, which it calls: a split of three, one question, and
// the chosen handler goes on to what it calls. The market worker's module
// body hands receiveMarketUpdate to addEventListener, its one candidate,
// asked nothing.
func TestJSTSFixtureMainFlowsWalkByCode(t *testing.T) {
	index, layer, graph := jstsFlowFixture(t)
	preset := &flowPreset{choose: map[string]string{"registerChainedCallbacks": "handleOrder"}}
	flow := walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, "registerChainedCallbacks", "src/server.ts"), preset)
	if got, want := flowPath([]groupindex.Index{index}, flow), []string{"registerChainedCallbacks ()", "handleOrder (handed to CallbackChain.map)", "recordOrder (called)"}; !slices.Equal(got, want) {
		t.Fatalf("registerChainedCallbacks' flow = %q\nwant %q", got, want)
	}
	if len(preset.asked) != 1 || !slices.Equal(preset.asked[0].options, []string{"CallbackChain.map", "handleOrder", "recordOrder"}) {
		t.Fatalf("splits asked: %+v", preset.asked)
	}
	assertNoRepeats(t, flow)
	worker := &flowPreset{}
	if got := flowPath([]groupindex.Index{index}, walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, "src/market-worker", "src/market-worker.js"), worker)); !slices.Equal(got,
		[]string{"src/market-worker ()", "receiveMarketUpdate (handed to platform:javascript.addEventListener)"}) || len(worker.asked) != 0 {
		t.Fatalf("the market worker's flow = %q after %d questions", got, len(worker.asked))
	}
	assertNoTestCode(t, preset, "exerciseMarket", "throttleAnArrow")
	assertWaysJoin(t, index, layer, graph, "startSession", "src/stored-callbacks.ts", "runSession", "acceptClient", "flushReplies")
	assertPassedHanded(t, index, layer, graph, "startOnce", "src/stored-callbacks.ts", "runOnce", "acceptClient")
	assertPreparedThenExecuted(t, index, layer, graph, "watchRun", "src/stored-callbacks.ts", "watchPrepare", "watchCollect", "acceptClient", "watchExecute", "flushReplies", false)
}

// clojureFlowFixture is the cumulative Clojure fixture as an ordinary run
// reads it, with its facts.
func clojureFlowFixture(t *testing.T) (groupindex.Index, facts.Result, atlas.Graph) {
	t.Helper()
	root, repository := materializeFixtureRepository(t, "clojure")
	targets, err := clojureproject.Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("Clojure discovery: %v %v", targets, err)
	}
	result, err := clojureproject.Build(t.Context(), root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, _ map[string]any, _ []string) (string, bool) { return "domain", column == "role" }}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	return projected, layer, graph
}

// The Clojure fixture's flow (design skeptic, 2026-09-30): start-sketch!
// hands draw-greeting and on-key to quil's sketch; draw-greeting only
// formats a string, so its closure enters no core part and on-key is the
// one candidate. on-key calls service/command-for: a chain asked nothing.
func TestClojureFixtureMainFlowWalksByCode(t *testing.T) {
	index, layer, graph := clojureFlowFixture(t)
	preset := &flowPreset{}
	flow := walkFixtureFrom(t, index, layer, graph, subjectNamed(t, index, "example.core/start-sketch!", "src/example/core.clj"), preset)
	if got, want := flowPath([]groupindex.Index{index}, flow), []string{"example.core/start-sketch! ()",
		"example.core/on-key (handed to quil.core.sketch.key-pressed)", "example.service/command-for (called)"}; !slices.Equal(got, want) || len(preset.asked) != 0 {
		t.Fatalf("start-sketch!'s flow = %q after %d questions\nwant %q", got, len(preset.asked), want)
	}
	assertNoRepeats(t, flow)
	assertWaysJoin(t, index, layer, graph, "example.core/open-greeting", "src/example/core.clj", "example.core/greet-command", "example.service/command-for", "example.core/loud-greeting")
	assertPreparedThenExecuted(t, index, layer, graph, "example.core/watch-run", "src/example/core.clj", "example.core/watch-prepare", "example.core/watch-collect", "example.service/command-for", "example.core/watch-execute", "example.core/loud-greeting", false)
}
