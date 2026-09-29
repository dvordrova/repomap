package contracttest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// askedSymbol is an outside symbol's row as the reading asks it: which
// question set (binds for a handed callable, talks otherwise), what the row
// shows, and the calls of it asked what their words become, as written.
type askedSymbol struct {
	question string
	usage    string
	received []string
	calls    []string
}

// askedOutsideSymbols reads one program without a model up to its outside
// symbols and returns every symbol the requests ask, failing when one is
// asked in two question sets.
func askedOutsideSymbols(t *testing.T, repository *corpus.Corpus, index programindex.Index) map[string]askedSymbol {
	t.Helper()
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Repository: "fixture", Revision: "test", Through: "atlas_api", OwnerRunDir: filepath.Join(dir, "run"),
		Targets:  []reading.TargetMeta{{ID: index.Target.ID, Language: index.Target.Language, Kind: "executable", Name: index.Target.Name, Root: "."}},
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		ReadSource: func(path string) ([]byte, error) {
			id, ok := repository.ID(path)
			if !ok {
				return nil, os.ErrNotExist
			}
			content, err := repository.ReadFileAll(id)
			return content.Bytes, err
		},
	}); err != nil {
		t.Fatal(err)
	}
	tables := filepath.Join(dir, "run", "tables")
	refs, err := filepath.Glob(filepath.Join(tables, "atlas_api-r*-w*.input.ref.json"))
	if err != nil || len(refs) == 0 {
		t.Fatalf("the outside symbols were not asked: %v %v", refs, err)
	}
	asked := map[string]askedSymbol{}
	calls := map[string][]string{}
	for _, ref := range refs {
		var pointer struct{ File string }
		raw, err := os.ReadFile(ref)
		if err == nil {
			err = json.Unmarshal(raw, &pointer)
		}
		if err == nil {
			raw, err = os.ReadFile(filepath.Join(tables, pointer.File))
		}
		var request struct {
			Fill []struct{ Name string }
			Rows []struct {
				Symbol         string   `json:"symbol"`
				Usage          string   `json:"usage"`
				Call           string   `json:"call"`
				ResultReceives []string `json:"result_receives"`
			}
		}
		if err == nil {
			err = json.Unmarshal(raw, &request)
		}
		if err != nil {
			t.Fatal(err)
		}
		question := "talks"
		for _, column := range request.Fill {
			switch column.Name {
			case "binds":
				question = "binds"
			case "enters":
				question = "enters"
			case "starts":
				question = "starts"
			}
		}
		// A starting statement is asked on its own, of no outside symbol
		// (started_test.go).
		if question == "starts" {
			continue
		}
		for _, row := range request.Rows {
			if question == "enters" {
				calls[row.Symbol] = append(calls[row.Symbol], row.Call)
				continue
			}
			if previous, twice := asked[row.Symbol]; twice {
				t.Fatalf("%s is asked in two questions: %s and %s", row.Symbol, previous.question, question)
			}
			asked[row.Symbol] = askedSymbol{question: question, usage: row.Usage, received: row.ResultReceives}
		}
	}
	for symbol, written := range calls {
		got, ok := asked[symbol]
		if !ok || got.question != "talks" {
			t.Fatalf("the calls of %s are asked what their words become beside %+v", symbol, got)
		}
		got.calls = written
		asked[symbol] = got
	}
	return asked
}

// set is the symbol's question set: binds, talks, or given (talks, with its
// word calls each asked what their words become).
func (got askedSymbol) set() string {
	if got.question == "talks" && len(got.calls) > 0 {
		return "given"
	}
	return got.question
}

// expectAsked checks one symbol's question set, its usage (for given, among
// its asked calls too) and what its call's result receives.
func expectAsked(t *testing.T, asked map[string]askedSymbol, symbol, question, usage string, received ...string) {
	t.Helper()
	got, ok := asked[symbol]
	if !ok || got.set() != question || usage != "" && (got.usage != usage || question == "given" && !slices.Contains(got.calls, usage)) || !slices.Equal(got.received, received) {
		t.Fatalf("%s is asked %+v, want %s with usage %q and result_receives %v", symbol, got, question, usage, received)
	}
}

// Every language's calls that give an outside symbol words are each asked
// what their words become, beside what a call to the symbol does with
// other programs; a symbol handed a callable is asked what it becomes
// instead, and no symbol is asked in two sets. The symbol's row shows its
// first word call as written and what the code calls on the call's result.
// Missing equivalents are held as they are, recorded in each language's
// contract: Go's package-level variable initializer calls no outside
// symbol the index records, and an npm package without its declarations
// names no symbol, so neither is asked.
func TestEveryLanguageAsksItsWordGivenCallsTheEntryQuestion(t *testing.T) {
	t.Run("c", func(t *testing.T) {
		fixture := loadCFixture(t)
		asked := askedOutsideSymbols(t, fixture.repository, buildCIndex(t, fixture, "c:kvd"))
		expectAsked(t, asked, "string.h.strcmp", "given", `strcmp(argv[1], "--symbols")`)
		expectAsked(t, asked, "stdio.h.fprintf", "given", "")
		expectAsked(t, asked, "pthread.h.pthread_create", "binds", "pthread_create(&stats, NULL, statsWorker, NULL)")
		expectAsked(t, asked, "sys/socket.h.accept", "talks", "")
	})
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		authorities := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "word-given")
		input, err := goadapter.BuildInput(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		asked := askedOutsideSymbols(t, repository, index)
		expectAsked(t, asked, "flag.String", "given", `flag.String("price-endpoint", "", "price service")`)
		expectAsked(t, asked, "net/http.HandleFunc", "binds", "")
		// var verbose = flag.Bool("verbose", …) at package level: a recorded
		// gap (GO.md), not an answer.
		if _, ok := asked["flag.Bool"]; ok {
			t.Fatal("the package-level flag.Bool is asked: record the gap as closed in GO.md")
		}
	})
	t.Run("python", func(t *testing.T) {
		_, repository := materializeFixtureRepository(t, "python")
		catalog, err := pythontarget.Discover(t.Context(), repository)
		if err != nil {
			t.Fatal(err)
		}
		input, err := pythonprogramindex.BuildInput(t.Context(), repository, pythonFixtureTarget(t, catalog))
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		asked := askedOutsideSymbols(t, repository, index)
		// ServiceCommands' parser and subcommands are fields stored once from
		// argparse's calls, so the calls on them count beside the locals';
		// RebuiltParser's add_argument, on a parser stored twice, does not.
		expectAsked(t, asked, "argparse.ArgumentParser", "given", `argparse.ArgumentParser("tool")`, "add_argument ×1", "add_subparsers ×2")
		expectAsked(t, asked, "argparse.ArgumentParser.add_argument", "given", `parser.add_argument("-v", "--verbose", action="store_true")`)
		expectAsked(t, asked, "argparse.ArgumentParser.add_subparsers", "given", `parser.add_subparsers(dest="cmd")`, "add_parser ×2")
		expectAsked(t, asked, "argparse.ArgumentParser.add_subparsers.add_parser", "given", `commands.add_parser("init")`, "add_argument ×1", "set_defaults ×2")
		// The subcommand's handler is handed over by another call: a second
		// question; when both are accepted, one input (J1,
		// TestCumulativePythonInputsJoinAndCatalogue).
		expectAsked(t, asked, "argparse.ArgumentParser.add_subparsers.add_parser.set_defaults", "binds", "init.set_defaults(func=run_init)")
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := jstsproject.Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		asked := askedOutsideSymbols(t, repository, index)
		expectAsked(t, asked, "platform:javascript.Console.log", "given", "")
		expectAsked(t, asked, "platform:javascript.Worker", "given", `new Worker("./market-worker.js", {type: "module"})`)
		// commander is declared in package.json but not installed: its
		// calls name no symbol (JSTS.md), and .option("-p, --port <n>") is
		// a registration with no symbol to ask about.
		for symbol := range asked {
			if strings.Contains(symbol, "commander") {
				t.Fatalf("%s is asked without its declarations", symbol)
			}
		}
	})
	t.Run("clojure", func(t *testing.T) {
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
		asked := askedOutsideSymbols(t, repository, index)
		expectAsked(t, asked, "clojure.core.format", "given", `(format "create %s dir" dir)`)
		expectAsked(t, asked, "clojure.string.replace", "given", "")
	})
}
