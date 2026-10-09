package contracttest

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// optionsOf names an input's options (GroupsIndex Reach.Options) as
// "name by declarer", "(also a tile)" after one that stays a tile of its
// own, and says whether the input is nested itself.
func optionsOf(t *testing.T, index groupindex.Index, name, declaredBy string) ([]string, bool) {
	t.Helper()
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	operations := map[string]groupindex.Operation{}
	for _, operation := range index.Operations {
		operations[operation.ID] = operation
	}
	for position, operation := range index.Operations {
		if operation.Name != name || names[operation.DeclaredBy] != declaredBy {
			continue
		}
		var options []string
		for _, id := range index.Reach[position].Options {
			option := operations[id]
			listed := option.Name + " by " + names[option.DeclaredBy]
			if !index.Launch.Nested[id] {
				listed += " (also a tile)"
			}
			options = append(options, listed)
		}
		return options, index.Launch.Nested[operation.ID]
	}
	t.Fatalf("no input %s declared by %s", name, declaredBy)
	return nil, false
}

// topLevelTwice is a word two of a file's inputs of one kind are named by
// among the inputs that stay tiles, or "".
func topLevelTwice(index groupindex.Index, path string) string {
	seen := map[string]bool{}
	for _, operation := range index.Operations {
		if operation.Location.Path != path || index.Launch.Nested[operation.ID] {
			continue
		}
		key := operation.Kind + " " + operation.Name
		if seen[key] {
			return key
		}
		seen[key] = true
	}
	return ""
}

// A flag belongs to its subcommand (owner, 2026-09-29: litestream's Inputs
// read -json, -timeout and -socket six to ten times each, and every tile
// carried its default and usage). An input of a subcommand's kind declared
// by code only that subcommand's case runs, or on the object its own call
// made, is its option: nested under it, no tile of its own, and a word two
// subcommands both declare is never twice among the tiles. Each is named by
// its word alone.
//
//   - Go: RunSubcommand's switch runs runServe in case serve and runCheck in
//     case check; each declares -verbose on its own flag set and hands that
//     flag set to addCommon, which declares -quiet on it: only the cases
//     run addCommon, so -quiet is an option of both. (Its facts are the
//     Python helper's too, a call on a parameter and a call_result
//     argument; no flag set is an input's own call here.)
//   - Python: argparse's --force is added to the parser add_parser("init")
//     made; -v, on the program's own parser, stays the program's.
//     add_common declares --quiet on the parser each of its calls hands it,
//     init's and status's: an option of both, no tile. add_output is also
//     handed the program's parser: --json is init's option and a tile.
//     build_serve hands build_args ARGS_SERVE with serve's parser, so the
//     OPTIONS rows it names are serve's options; build_subcommands hands
//     build_args parameters, so they stay tiles.
//   - C: kvcli's main runs bench in its bench branch, and bench compares its
//     own arguments with --requests; --raw, which main compares, stays the
//     client's.
//
// TypeScript's switch and Clojure's case form carry the same case branches
// (ProgramIndex comparisons); their fixtures declare no option inside a
// subcommand's own code yet, and neither has a helper handed a
// subcommand's object (JS declares its options on the root program alone;
// the Clojure fixture uses no option library).
func TestASubcommandsOptionsAreNestedUnderIt(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		app := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "subcommand options")
		index, err := goadapter.Build(repository, app.target, app.origins, app.direct, app.external, app.core, app.dynamic, app.tests)
		if err != nil {
			t.Fatal(err)
		}
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
		preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
			symbol, _ := item["symbol"].(string)
			compares, _ := item["compares"].(string)
			if column == "enters" && (compares == "cmd" || strings.HasSuffix(symbol, "FlagSet.Bool")) {
				return "command", true
			}
			return "", false
		}}
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
		for _, command := range []struct{ name, option string }{{"serve", "verbose by runServe"}, {"check", "verbose by runCheck"}} {
			options, nested := optionsOf(t, projected, command.name, "RunSubcommand")
			if want := []string{"quiet by addCommon", command.option}; nested || !reflect.DeepEqual(options, want) {
				t.Fatalf("%s: nested %v, options %q; want %q", command.name, nested, options, want)
			}
		}
		// ToolCommand's strict, on the flag set its own function makes, is
		// no subcommand's: no case runs ToolCommand.
		if options, nested := optionsOf(t, projected, "strict", "ToolCommand"); nested || len(options) != 0 {
			t.Fatalf("strict: nested %v, options %q", nested, options)
		}
		if word := topLevelTwice(projected, "internal/storefixture/tool_cli.go"); word != "" {
			t.Fatalf("%s is twice among the tiles", word)
		}
		for _, operation := range projected.Operations {
			if strings.Contains(operation.Name, "log each") || strings.Contains(operation.Name, ":8080") {
				t.Fatalf("an input is named by its usage or default: %q", operation.Name)
			}
		}
	})
	t.Run("python", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "python")
		index := pythonLibraryIndex(t)
		graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
		preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
			symbol, _ := item["symbol"].(string)
			switch {
			case column == "enters" && (strings.HasSuffix(symbol, ".add_argument") || strings.HasSuffix(symbol, ".add_parser")):
				return "command", true
			case column == "binds" && strings.HasSuffix(symbol, ".set_defaults"):
				return "command", true
			case column == "becomes" && item["table"] == "OPTIONS":
				return "command", true
			}
			return "", false
		}}
		projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
		for _, command := range []struct {
			name, by string
			options  []string
		}{
			{"init", "build_parser", []string{"--force by build_parser", "--json by add_output (also a tile)", "--quiet by add_common"}},
			{"status", "build_parser", []string{"--quiet by add_common"}},
			{"-v", "build_parser", nil},
			{"serve", "build_serve", []string{"force by OPTIONS (also a tile)", "verbose by OPTIONS (also a tile)"}},
		} {
			if options, nested := optionsOf(t, projected, command.name, command.by); nested || !slices.Equal(options, command.options) {
				t.Fatalf("%s: nested %v, options %q; want %q", command.name, nested, options, command.options)
			}
		}
		if word := topLevelTwice(projected, "src/fixture_app/tool_cli.py"); word != "" {
			t.Fatalf("%s is twice among the tiles", word)
		}
	})
	t.Run("c", func(t *testing.T) {
		client := readKvdPair(t).indexes["kvcli"]
		if options, nested := optionsOf(t, client, "bench", "main"); nested || !slices.Equal(options, []string{"--requests by bench"}) {
			t.Fatalf("bench: nested %v, options %q", nested, options)
		}
		if options, nested := optionsOf(t, client, "--raw", "main"); nested || len(options) != 0 {
			t.Fatalf("--raw: nested %v, options %q", nested, options)
		}
		if word := topLevelTwice(client, "kvcli.c"); word != "" {
			t.Fatalf("%s is twice among the tiles", word)
		}
	})
}
