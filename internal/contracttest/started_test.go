package contracttest

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// startedItems are the starting statements a preset was asked, as
// "statement | starts | in".
func startedItems(preset *inputsPreset) []string {
	var items []string
	for _, item := range preset.asked["starts"] {
		statement, _ := item["statement"].(string)
		starts, _ := item["starts"].(string)
		in, _ := item["in"].(string)
		items = append(items, statement+" | "+strings.Fields(starts + " ?")[0]+" | "+strings.Fields(in + " ?")[0])
	}
	slices.Sort(items)
	return items
}

// The Go fixture's worker service starts four goroutines beside its
// polling loop (internal/storefixture/runtime_registrations.go): the commit
// worker directly (go RunCommitWorker(stop)), the compactor inside a
// closure that marks a wait group done, a closure loading the cache once
// and a sweeper closure calling only outside code, named "StartSweeper
// (inline)". Each `go` statement is a registration handing its function over
// and is asked on its own, never through one symbol's answer: the closure
// is named and handled by the one function it calls itself (RunCompactor,
// loadCache), not by its own Open$1-like name. A preset reader answers the
// ticker loops continuous and scheduled and the one-shot load none; the
// answers become two entries whose handlers are the started functions, and
// the one-shot load no entry. time.AfterFunc's one-shot delay and the
// finite retry are no starting statements. The worker's status route's
// handler compares the request it was handed with two words, the route's
// sub-arguments (owner's rule K3), never asked what they become.
func TestCumulativeGoStartsAreAskedPerStatement(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	worker := sharedGoFixtureAuthorities(t, root, repository, goFixtureRootPackage+"/cmd/worker", "cumulative-go-worker-starts")
	index, err := goadapter.Build(repository, worker.target, worker.origins, worker.direct, worker.external, worker.core, worker.dynamic, worker.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, options []string) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "binds" && symbol == "net/http.HandleFunc" {
			return "request", true
		}
		if column != "starts" {
			return "", false
		}
		switch starts, _ := item["starts"].(string); {
		case strings.HasPrefix(starts, "RunCommitWorker"):
			return "continuous", true
		case strings.HasPrefix(starts, "RunCompactor"):
			return "scheduled", true
		case strings.HasPrefix(starts, "StartSweeper$1"):
			return "continuous", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	const path = "internal/storefixture/runtime_registrations.go"
	want := []string{
		"go RunCommitWorker(stop) | RunCommitWorker | StartCommitWorker",
		"go func() { done <- loadCache() }() | loadCache | WarmCache",
		"go func() {defer wg.Done() RunCompactor(stop)}() | RunCompactor | StartCompactor",
		"go func() {ticker := time.NewTicker(time.Hour) defer ticker.Stop() for {select {case <-ticker.C: runtime.GC() case <-stop: return}}}() | StartSweeper$1 | StartSweeper",
	}
	if got := startedItems(preset); !reflect.DeepEqual(got, want) {
		t.Fatalf("starting statements asked:\n%q\nwant\n%q", got, want)
	}
	// The started function's own calls, by name, are its item's evidence.
	for _, item := range preset.asked["starts"] {
		if starts, _ := item["starts"].(string); strings.HasPrefix(starts, "RunCompactor") {
			calls := stringsOf(item["calls"])
			if !slices.Contains(calls, "time.NewTicker") || !slices.ContainsFunc(calls, func(name string) bool { return strings.HasSuffix(name, "commitPendingBatches") }) {
				t.Fatalf("RunCompactor's calls: %q", calls)
			}
		}
	}
	const status = "cmd/worker/status.go"
	got := inputRows(projected, path, status)
	wantInputs := []inputRow{
		// serveStatus, the handler of the status route, compares the
		// request it was handed with HEAD and X-Verbose: sub-arguments of
		// its route (owner's rule K3), never asked. What it writes to w is
		// asked, and none. The preset names the route by its call's first
		// word.
		{kind: "request", name: "HEAD", declaredBy: "serveStatus", at: status + ":14"},
		{kind: "request", name: "X-Verbose", declaredBy: "serveStatus", at: status + ":17"},
		{kind: "request", name: "HandleFunc", declaredBy: "init", handler: "serveStatus", at: status + ":21"},
		{kind: "continuous", name: "RunCommitWorker", declaredBy: "StartCommitWorker", handler: "RunCommitWorker", at: path + ":25"},
		{kind: "scheduled", name: "RunCompactor", declaredBy: "StartCompactor", handler: "RunCompactor", at: path + ":54"},
		// A closure calling only outside code is handled by itself and
		// named by the function it is written in, never StartSweeper$1.
		{kind: "continuous", name: "StartSweeper (inline)", declaredBy: "StartSweeper", handler: "StartSweeper$1", at: path + ":74"},
	}
	if !reflect.DeepEqual(got, wantInputs) {
		t.Fatalf("started inputs:\n%+v\nwant\n%+v", got, wantInputs)
	}
	var entered []string
	for _, item := range preset.asked["enters"] {
		if in, _ := item["in"].(string); strings.HasPrefix(in, "serveStatus") {
			entered = append(entered, callText(item))
		}
	}
	if want := []string{`fmt.Fprintf(w, "pending jobs (verbose %t)\n", verbose)`}; !slices.Equal(entered, want) {
		t.Fatalf("serveStatus's calls asked what their words become: %q, want %q", entered, want)
	}
	for position, operation := range projected.Operations {
		if operation.Location.Path == status && operation.SubjectID != "" {
			var subArguments []string
			for _, id := range projected.Reach[position].SubArguments {
				for _, other := range projected.Operations {
					if other.ID == id {
						subArguments = append(subArguments, other.Name)
					}
				}
			}
			if slices.Sort(subArguments); !slices.Equal(subArguments, []string{"HEAD", "X-Verbose"}) {
				t.Fatalf("the status route's sub-arguments: %v", subArguments)
			}
		}
	}
}

// The Python fixture starts coroutines with asyncio.create_task
// (src/fixture_app/runtime_registrations.py): poll_prices loops for as
// long as the program runs, announce_start says once that the bot started,
// and submit_candles hands refresh_candles a stream. Each create_task is a
// registration handing its coroutine over, its word the call it is handed
// to, asked on its own with that call as written. A preset reader answers
// the polling loop continuous and the rest none: one entry, whose handler
// is poll_prices.
func TestCumulativePythonStartsAreAskedPerStatement(t *testing.T) {
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
	input, err := sharedPythonFixtureInput(t, repository, target)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, options []string) (string, bool) {
		if starts, _ := item["starts"].(string); column == "starts" && strings.HasPrefix(starts, "poll_prices") {
			return "continuous", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	const path = "src/fixture_app/runtime_registrations.py"
	want := []string{
		"asyncio.create_task(announce_start()) | announce_start | start_background_tasks",
		"asyncio.create_task(poll_prices()) | poll_prices | start_background_tasks",
		"asyncio.create_task(refresh_candles(stream)) | refresh_candles | submit_candles",
	}
	if got := startedItems(preset); !reflect.DeepEqual(got, want) {
		t.Fatalf("starting statements asked:\n%q\nwant\n%q", got, want)
	}
	var started []inputRow
	for _, row := range inputRows(projected, path) {
		if row.handler == "poll_prices" || row.handler == "announce_start" || row.handler == "refresh_candles" {
			started = append(started, row)
		}
	}
	if len(started) != 1 || started[0].kind != "continuous" || started[0].handler != "poll_prices" {
		t.Fatalf("started inputs: %+v", started)
	}
}
