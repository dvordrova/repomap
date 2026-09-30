package contracttest

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// dispatchPreset answers a comparison (an item with `compares`) comparing
// value command, and nothing else but the neutral answers.
func dispatchPreset(value string) *inputsPreset {
	return &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if compares, _ := item["compares"].(string); column == "enters" && compares == value {
			return "command", true
		}
		return "", false
	}}
}

// comparisonsAsked are the comparison items asked, as "value: cases".
func comparisonsAsked(preset *inputsPreset) []string {
	var result []string
	for _, item := range preset.asked["enters"] {
		compares, ok := item["compares"].(string)
		if !ok {
			continue
		}
		cases, _ := json.Marshal(item["cases"])
		result = append(result, compares+": "+string(cases))
	}
	slices.Sort(result)
	return result
}

// expectDispatchInputs checks one comparison asked once with every case's
// words, and one command input per case, named by its first word and
// declared by the comparing declaration: handled by it, within its case's
// lines, when the case calls into the program's own code (handled), its
// handler not established otherwise.
func expectDispatchInputs(t *testing.T, preset *inputsPreset, projected groupindex.Index, path, value, cases, declaredBy string, names []string, handled ...string) {
	t.Helper()
	var asked []string
	for _, item := range comparisonsAsked(preset) {
		if strings.HasPrefix(item, value+": ") {
			asked = append(asked, item)
		}
	}
	if want := []string{value + ": " + cases}; !reflect.DeepEqual(asked, want) {
		t.Fatalf("comparisons asked: %q, want %q", asked, want)
	}
	var got []string
	for _, row := range inputRows(projected, path) {
		if row.declaredBy == declaredBy {
			if want := map[bool]string{true: declaredBy}[slices.Contains(handled, row.name)]; row.kind != "command" || row.handler != want {
				t.Fatalf("a case's input is %+v, want its handler %q", row, want)
			}
			got = append(got, row.name)
		}
	}
	slices.Sort(got)
	names = slices.Sorted(slices.Values(names))
	if !reflect.DeepEqual(got, names) {
		t.Fatalf("%s's inputs: %q, want %q", declaredBy, got, names)
	}
}

// Each language's comparison is asked once with all its cases, in the
// inputs step, and an entry answer makes one input per case, declared by the
// comparing function (its catalogue): Go's RunSubcommand (the switch and its
// default's help words), Python's if/elif chain, TypeScript's switch on
// process.argv[2], Clojure's case form and C's switch on a letter. A case
// whose lines call into the program's own code is handled by the comparing
// function there, and its reach starts from those calls: Go's serve and
// check (runServe, runCheck), Python's init (run_init), TypeScript's build
// (runBuild), Clojure's serve (shout), C's V (printVersion), as
// litestream's case "replicate" runs its command; a case returning a word
// or printing usage has no handler established. Python's tables of names are asked with
// the table question: OPTIONS and REQUIRED, never FORMATS, which nothing
// reads.
func TestEveryLanguageAsksAComparisonOnceAndMakesAnInputPerCase(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		app := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "dispatch inputs")
		index, err := goadapter.Build(repository, app.target, app.origins, app.direct, app.external, app.core, app.dynamic, app.tests)
		if err != nil {
			t.Fatal(err)
		}
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
		preset := dispatchPreset("cmd")
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
		expectDispatchInputs(t, preset, projected, "internal/storefixture/tool_cli.go", "cmd", `[["serve"],["check","verify"],["help","-h"]]`, "RunSubcommand", []string{"serve", "check", "help"}, "serve", "check")
		if reached := reachedNames(projected, "serve"); !slices.Contains(reached, "runServe") || slices.Contains(reached, "runCheck") {
			t.Fatalf("serve's reach %q, want runServe and never check's runCheck", reached)
		}
		for _, item := range preset.asked["enters"] {
			if item["compares"] == "cmd" && !strings.Contains(item["from"].(string), `element "0" of parameter #1 args of RunSubcommand`) {
				t.Fatalf("cmd's origin as asked: %v", item["from"])
			}
		}
		// The cases handled in their lines leave the catalogue of the words
		// RunSubcommand declares whose handler is not established.
		if catalogues := catalogueRows(projected); !slices.Contains(catalogues, "command by RunSubcommand: help") {
			t.Fatalf("no catalogue of RunSubcommand's unhandled case in %q", catalogues)
		}
	})
	t.Run("python", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "python")
		index := pythonLibraryIndex(t)
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
		preset := dispatchPreset("command")
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
		expectDispatchInputs(t, preset, projected, "src/fixture_app/dispatch.py", "command", `[["init"],["serve","run"],["help"]]`, "dispatch", []string{"init", "serve", "help"}, "init")
		var tables []string
		for _, item := range preset.asked["becomes"] {
			if name, ok := item["table"].(string); ok {
				tables = append(tables, name)
			}
		}
		for _, name := range []string{"OPTIONS", "REQUIRED"} {
			if !slices.Contains(tables, name) {
				t.Fatalf("the table %s was not asked: %q", name, tables)
			}
		}
		if slices.Contains(tables, "FORMATS") {
			t.Fatalf("FORMATS, which nothing reads, was asked: %q", tables)
		}
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := jstsproject.Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
		preset := dispatchPreset("process.argv[2]")
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "typescript", Kind: index.Target.Kind, Name: index.Target.Name, Root: "."}, root, preset)
		expectDispatchInputs(t, preset, projected, "src/dispatch.ts", "process.argv[2]", `[["build"],["check","verify"],["help","-h"]]`, "dispatch", []string{"build", "check", "help"}, "build")
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
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index})
		preset := dispatchPreset("(first args)")
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
		expectDispatchInputs(t, preset, projected, "src/example/core.clj", "(first args)", `[["serve"],["check","verify"]]`, "example.core/run-command", []string{"serve", "check"}, "serve")
	})
	t.Run("c", func(t *testing.T) {
		fixture := loadCFixture(t)
		index := buildCIndex(t, fixture, "c:kvcli")
		layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
		if err != nil {
			t.Fatal(err)
		}
		graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
		if err != nil {
			t.Fatal(err)
		}
		preset := dispatchPreset("arg[1]")
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}, fixture.root, preset)
		expectDispatchInputs(t, preset, projected, "kvcli.c", "arg[1]", `[["h","?"],["V"]]`, "shortOption", []string{"h", "V"}, "V")
	})
}

// reachedNames are the declarations the input named name reaches, by name.
func reachedNames(index groupindex.Index, name string) []string {
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	for position, operation := range index.Operations {
		if operation.Name != name || position >= len(index.Reach) {
			continue
		}
		var reached []string
		for _, subject := range index.Reach[position].Subjects {
			reached = append(reached, names[subject.SubjectID])
		}
		return reached
	}
	return nil
}
