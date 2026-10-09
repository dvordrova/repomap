package contracttest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// launch is one call that starts another program, as a fixture writes it:
// the outside symbol, the call as written (what the talks question and the
// program question show), and the words the program question offers.
type launch struct {
	symbol, usage string
	words         []string
}

// launchCalls finds, in one program's graph, the calls of symbol with
// their words, and the calls made on what those calls returned, by symbol.
func launchCalls(t *testing.T, repository *corpus.Corpus, index programindex.Index, symbol string) ([][]string, []string) {
	t.Helper()
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	type site struct {
		path         string
		line, column int
	}
	launched := map[site]bool{}
	var words [][]string
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API != nil && apiSymbolName(call.API.Package, call.API.Receiver, call.API.Name) == symbol {
				launched[site{place.Path, call.Line, call.Column}] = true
				words = append(words, call.Values)
			}
		}
	}
	// A receiver is what a launch returned when it is that call's result,
	// or alternatives every one of which is (a command built on either
	// branch).
	var launchedBy func(*sourcevalue.Value) bool
	launchedBy = func(value *sourcevalue.Value) bool {
		switch {
		case value == nil:
			return false
		case value.Kind == "call_result":
			return value.Anchor != nil && launched[site{value.Anchor.Path, value.Anchor.Line, value.Anchor.Column}]
		case value.Kind == "alternatives":
			for i := range value.Parts {
				if !launchedBy(&value.Parts[i]) {
					return false
				}
			}
			return len(value.Parts) > 0
		}
		return false
	}
	var onResult []string
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API != nil && launchedBy(call.ReceiverValue) {
				onResult = append(onResult, apiSymbolName(call.API.Package, call.API.Receiver, call.API.Name))
			}
		}
	}
	return words, onResult
}

// apiSymbolName is the outside symbol's name as the atlas_api table asks it.
func apiSymbolName(pkg, receiver, name string) string {
	if receiver != "" {
		for len(receiver) > 0 && receiver[0] == '*' {
			receiver = receiver[1:]
		}
		name = receiver + "." + name
	}
	return pkg + "." + name
}

// expectLaunch checks that a fixture's launching call is asked what it does
// with other programs, shown as written, and that its call gives the words
// the program question will offer, every one of them.
func expectLaunch(t *testing.T, asked map[string]askedSymbol, repository *corpus.Corpus, index programindex.Index, want launch, question string) []string {
	t.Helper()
	expectAskedAs(t, asked, want.symbol, question, want.usage)
	words, onResult := launchCalls(t, repository, index, want.symbol)
	if !slices.ContainsFunc(words, func(values []string) bool { return slices.Equal(values, want.words) }) {
		t.Fatalf("%s is given %q, want a call given %q", want.symbol, words, want.words)
	}
	return onResult
}

// expectAskedAs is expectAsked without pinning what the call's result
// receives.
func expectAskedAs(t *testing.T, asked map[string]askedSymbol, symbol, question, usage string) {
	t.Helper()
	got, ok := asked[symbol]
	if !ok || got.set() != question || got.usage != usage {
		t.Fatalf("%s is asked %+v, want %s with usage %q", symbol, got, question, usage)
	}
}

// Every language's call that starts another program is asked what it does
// with other programs and shows the call as written; read without a model,
// where that answer is not decided, each of its word calls is also asked
// what its words become (given); the call gives the program question every word it writes, the
// program's word among them. A call on what a launching call returned is
// recorded as made on that call's result, which is how the reading keeps
// one boundary per launch; in Go a command built on either branch of an
// if/else is the alternatives of both calls' results, in edge order, so
// the call on it is both launches'. A launch whose program a variable
// holds gives no word. Two gaps are recorded, not answered: Python's words inside a
// list literal are no call words (PYTHON.md), and JavaScript's
// node:child_process has no declarations in the fixture (no @types/node),
// so spawn names no symbol and is not asked (JSTS.md).
func TestEveryLanguageAsksItsLaunchingCallsWithTheirWords(t *testing.T) {
	t.Run("c", func(t *testing.T) {
		fixture := sharedCFixture(t)
		dump := buildCIndex(t, fixture, "c:tools/dump.c")
		expectLaunch(t, askedOutsideSymbols(t, fixture.repository, dump), fixture.repository, dump,
			launch{"stdio.h.popen", `popen("sort -u", "w")`, []string{"sort -u", "w"}}, "given")
		kvd := buildCIndex(t, fixture, "c:kvd")
		// system(hook): the program comes from the environment.
		expectLaunch(t, askedOutsideSymbols(t, fixture.repository, kvd), fixture.repository, kvd,
			launch{"stdlib.h.system", "system(hook)", nil}, "talks")
	})
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		authorities := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "launches")
		input, err := goadapter.BuildInput(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		asked := askedOutsideSymbols(t, repository, index)
		onResult := expectLaunch(t, asked, repository, index,
			launch{"os/exec.CommandContext", `exec.CommandContext(ctx, "git", "rev-parse", "HEAD")`, []string{"git", "rev-parse", "HEAD"}}, "given")
		slices.Sort(onResult)
		if !slices.Equal(onResult, []string{"os/exec.Cmd.CombinedOutput", "os/exec.Cmd.Output"}) {
			t.Fatalf("calls on the started command = %q, want CombinedOutput (RevisionOf) and Output (Revision)", onResult)
		}
		expectEitherBranchLaunch(t, repository, index)
		onResult = expectLaunch(t, asked, repository, index, launch{"os/exec.Command", "exec.Command(hook, args...)", nil}, "talks")
		if !slices.Equal(onResult, []string{"os/exec.Cmd.Run"}) {
			t.Fatalf("calls on the hook's command = %q, want Run", onResult)
		}
	})
	t.Run("python", func(t *testing.T) {
		_, repository := materializeFixtureRepository(t, "python")
		catalog, err := pythontarget.Discover(t.Context(), repository)
		if err != nil {
			t.Fatal(err)
		}
		input, err := sharedPythonFixtureInput(t, repository, pythonFixtureTarget(t, catalog))
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		// The program's words are inside a list: the index records a call's
		// literal arguments only, so the call gives no word and its program
		// stays not established (a recorded gap, PYTHON.md).
		expectLaunch(t, askedOutsideSymbols(t, repository, index), repository, index,
			launch{"subprocess.run", `subprocess.run(["git", "rev-parse", "HEAD"], check=True, capture_output=True)`, nil}, "talks")
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := sharedJSTSFixture(t, repository, root)
		if err != nil {
			t.Fatal(err)
		}
		for symbol := range askedOutsideSymbols(t, repository, index) {
			if symbol == "child_process.spawn" || symbol == "node:child_process.spawn" {
				t.Fatalf("%s is asked without its declarations: record the gap as closed in JSTS.md", symbol)
			}
		}
	})
	t.Run("clojure", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "clojure")
		targets, err := clojureproject.Scout(repository, "clojure")
		if err != nil || len(targets) != 1 {
			t.Fatalf("Clojure discovery: %v %v", targets, err)
		}
		result, err := sharedClojureFixture(t, root, repository, targets[0])
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(result.Input)
		if err != nil {
			t.Fatal(err)
		}
		expectLaunch(t, askedOutsideSymbols(t, repository, index), repository, index,
			launch{"clojure.java.shell.sh", `(shell/sh "git" "rev-parse" "HEAD")`, []string{"git", "rev-parse", "HEAD"}}, "given")
	})
}

// expectEitherBranchLaunch checks RevisionOf in the Go fixture: cmd is built
// by exec.CommandContext on either branch, both calls give git as a word,
// and the receiver of cmd.CombinedOutput() is the alternatives of the two
// calls' results in edge order (then, else), neither picked.
func expectEitherBranchLaunch(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	var launches []string
	var receiver *sourcevalue.Value
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Name != "RevisionOf" {
			continue
		}
		for _, call := range place.Symbol.Calls {
			switch call.Name {
			case "exec.CommandContext":
				if !slices.Contains(call.Values, "git") {
					t.Fatalf("RevisionOf's launch at line %d gives %q, no git", call.Line, call.Values)
				}
				launches = append(launches, fmt.Sprintf("call_result %s:%d:%d", place.Path, call.Line, call.Column))
			case "exec.Cmd.CombinedOutput":
				receiver = call.ReceiverValue
			}
		}
	}
	var parts []string
	if receiver != nil && receiver.Kind == "alternatives" {
		for _, part := range receiver.Parts {
			if part.Anchor != nil {
				parts = append(parts, fmt.Sprintf("%s %s:%d:%d", part.Kind, part.Anchor.Path, part.Anchor.Line, part.Anchor.Column))
			}
		}
	}
	slices.Sort(launches)
	if len(launches) != 2 || !slices.Equal(parts, launches) {
		t.Fatalf("CombinedOutput's receiver = %+v, want the alternatives %q", receiver, launches)
	}
}
