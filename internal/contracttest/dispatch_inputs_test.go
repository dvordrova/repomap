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
// declared by the comparing declaration.
func expectDispatchInputs(t *testing.T, preset *inputsPreset, projected groupindex.Index, path, value, cases, declaredBy string, names []string) {
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
			if row.kind != "command" || row.handler != "" {
				t.Fatalf("a case's input is %+v", row)
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
// inputs step, and an entry answer makes one input per case, whose handler
// is not established, declared by the comparing function (its catalogue):
// Go's RunSubcommand (the switch and its default's help words), Python's
// if/elif chain, TypeScript's switch on process.argv[2], Clojure's case
// form and C's switch on a letter. Python's tables of names are asked with
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
		expectDispatchInputs(t, preset, projected, "internal/storefixture/tool_cli.go", "cmd", `[["serve"],["check","verify"],["help","-h"]]`, "RunSubcommand", []string{"serve", "check", "help"})
		for _, item := range preset.asked["enters"] {
			if item["compares"] == "cmd" && !strings.Contains(item["from"].(string), `element "0" of parameter #1 args of RunSubcommand`) {
				t.Fatalf("cmd's origin as asked: %v", item["from"])
			}
		}
		if catalogues := catalogueRows(projected); !slices.Contains(catalogues, "command by RunSubcommand: serve check help") {
			t.Fatalf("no catalogue of RunSubcommand's cases in %q", catalogues)
		}
	})
	t.Run("python", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "python")
		index := pythonLibraryIndex(t)
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
		preset := dispatchPreset("command")
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
		expectDispatchInputs(t, preset, projected, "src/fixture_app/dispatch.py", "command", `[["init"],["serve","run"],["help"]]`, "dispatch", []string{"init", "serve", "help"})
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
		expectDispatchInputs(t, preset, projected, "src/dispatch.ts", "process.argv[2]", `[["build"],["check","verify"],["help","-h"]]`, "dispatch", []string{"build", "check", "help"})
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
		expectDispatchInputs(t, preset, projected, "src/example/core.clj", "(first args)", `[["serve"],["check","verify"]]`, "example.core/run-command", []string{"serve", "check"})
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
		expectDispatchInputs(t, preset, projected, "kvcli.c", "arg[1]", `[["h","?"],["V"]]`, "shortOption", []string{"h", "V"})
	})
}
