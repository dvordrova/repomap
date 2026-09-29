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
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
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
		items = append(items, statement+" | "+strings.Fields(starts+" ?")[0]+" | "+strings.Fields(in+" ?")[0])
	}
	slices.Sort(items)
	return items
}

// The Go fixture's worker service starts three goroutines beside its
// polling loop (internal/storefixture/runtime_registrations.go): the commit
// worker directly (go RunCommitWorker(stop)), the compactor inside a
// closure that marks a wait group done, and a closure loading the cache
// once. Each `go` statement is a registration handing its function over
// and is asked on its own, never through one symbol's answer: the closure
// is named and handled by the one function it calls itself (RunCompactor,
// loadCache), not by its own Open$1-like name. A preset reader answers the
// ticker loops continuous and scheduled and the one-shot load none; the
// answers become two entries whose handlers are the started functions, and
// the one-shot load no entry. time.AfterFunc's one-shot delay and the
// finite retry are no starting statements.
func TestCumulativeGoStartsAreAskedPerStatement(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	worker := analyzeGoFixture(t, root, repository, goFixtureRootPackage+"/cmd/worker", "cumulative-go-worker-starts")
	index, err := goadapter.Build(repository, worker.target, worker.origins, worker.direct, worker.external, worker.core, worker.dynamic, worker.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, options []string) (string, bool) {
		if column != "starts" {
			return "", false
		}
		switch starts, _ := item["starts"].(string); {
		case strings.HasPrefix(starts, "RunCommitWorker"):
			return "continuous", true
		case strings.HasPrefix(starts, "RunCompactor"):
			return "scheduled", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	const path = "internal/storefixture/runtime_registrations.go"
	want := []string{
		"go RunCommitWorker(stop) | RunCommitWorker | StartCommitWorker",
		"go func() { done <- loadCache() }() | loadCache | WarmCache",
		"go func() {defer wg.Done() RunCompactor(stop)}() | RunCompactor | StartCompactor",
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
	got := inputRows(projected, path)
	wantInputs := []inputRow{
		{kind: "continuous", name: "RunCommitWorker", declaredBy: "StartCommitWorker", handler: "RunCommitWorker", at: path + ":24"},
		{kind: "scheduled", name: "RunCompactor", declaredBy: "StartCompactor", handler: "RunCompactor", at: path + ":53"},
	}
	if !reflect.DeepEqual(got, wantInputs) {
		t.Fatalf("started inputs:\n%+v\nwant\n%+v", got, wantInputs)
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
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, target)
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
