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
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// callText is the call a per-call item shows, as written.
func callText(item map[string]any) string {
	call, _ := item["call"].(string)
	return call
}

// The Go fixture's inputs, read end to end (internal/storefixture):
//
//   - per call: strings.EqualFold(os.Args[1], "check") compares the
//     program's argument with a subcommand's name, a command, while
//     strings.EqualFold(level, "default") in the same function compares
//     data; each call is asked on its own;
//   - K2: ToolCommand declares port on the flag set NewFlagSet("serve")
//     makes and strict on the one NewFlagSet("check") makes: two objects in
//     one function, two catalogues;
//   - K1: DestinationApplication's price-endpoint, on no object, is the
//     catalogue of its declaring function;
//   - settings: ServerConfig's tagged fields are asked with the one call
//     decoding the structure, json.Unmarshal(raw, &config), and are the
//     settings of that call; a response structure's fields, given to no
//     decoder, are none;
//   - runs_program: Revision's exec.CommandContext offers its words and the
//     program is named git;
//   - systems: net/http is asked once, with each symbol the program calls.
//
// J1 has no Go equivalent: the standard library has no call that names an
// entry and another that hands its handler over on the first one's result.
func TestCumulativeGoInputsAreAskedPerCallAndCatalogued(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	app := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "cumulative-go-inputs")
	index, err := goadapter.Build(repository, app.target, app.origins, app.direct, app.external, app.core, app.dynamic, app.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, options []string) (string, bool) {
		symbol, _ := item["symbol"].(string)
		switch column {
		case "talks":
			switch {
			case strings.HasPrefix(symbol, "os/exec."):
				return "runs_program", true
			case symbol == "net/http.Client.Do" || symbol == "net/http.Get":
				return "client_request", true
			}
		case "enters":
			switch {
			case strings.HasSuffix(symbol, "flag.String") || strings.HasSuffix(symbol, "FlagSet.Int") || strings.HasSuffix(symbol, "FlagSet.Bool"):
				return "command", true
			case strings.HasSuffix(symbol, "strings.EqualFold") && strings.Contains(callText(item), "os.Args"):
				return "command", true
			}
		case "program":
			if slices.Contains(options, "git") {
				return "git", true
			}
		case "becomes":
			if uses, _ := jsonText(item["structure_use"]); strings.Contains(uses, "encoding/json.Unmarshal") {
				return "setting", true
			}
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	// Each EqualFold call was asked on its own.
	var equalFolds []string
	for _, item := range preset.asked["enters"] {
		if symbol, _ := item["symbol"].(string); strings.HasSuffix(symbol, "strings.EqualFold") {
			equalFolds = append(equalFolds, callText(item))
		}
	}
	slices.Sort(equalFolds)
	if want := []string{`strings.EqualFold(level, "default")`, `strings.EqualFold(os.Args[1], "check")`}; !slices.Equal(equalFolds, want) {
		t.Fatalf("EqualFold calls asked: %q, want %q", equalFolds, want)
	}
	got := inputRows(projected, "internal/storefixture/tool_cli.go", "internal/storefixture/destinations.go")
	want := []inputRow{
		{kind: "command", name: "price-endpoint", declaredBy: "DestinationApplication", at: "internal/storefixture/destinations.go:27"},
		{kind: "setting", name: "listen", declaredBy: "ServerConfig", on: "json.Unmarshal(raw, &config)", at: "internal/storefixture/tool_cli.go:15"},
		{kind: "setting", name: "data_dir", declaredBy: "ServerConfig", on: "json.Unmarshal(raw, &config)", at: "internal/storefixture/tool_cli.go:16"},
		{kind: "command", name: "port", declaredBy: "ToolCommand", on: `flag.NewFlagSet("serve", flag.ContinueOnError)`, at: "internal/storefixture/tool_cli.go:36"},
		{kind: "command", name: "strict", declaredBy: "ToolCommand", on: `flag.NewFlagSet("check", flag.ContinueOnError)`, at: "internal/storefixture/tool_cli.go:38"},
		{kind: "command", name: "check", declaredBy: "ToolCommand", at: "internal/storefixture/tool_cli.go:39"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs:\n%+v\nwant\n%+v", got, want)
	}
	catalogues := catalogueRows(projected)
	for _, row := range []string{
		`command on flag.NewFlagSet("serve", flag.ContinueOnError): port`,
		`command on flag.NewFlagSet("check", flag.ContinueOnError): strict`,
		"command by DestinationApplication: price-endpoint",
		"command by ToolCommand: check",
		"setting on json.Unmarshal(raw, &config): listen data_dir",
	} {
		if !slices.Contains(catalogues, row) {
			t.Fatalf("no catalogue %q in %q", row, catalogues)
		}
	}
	// Every tagged field was asked; only ServerConfig's are settings.
	fields := map[string]bool{}
	for _, item := range preset.asked["becomes"] {
		if field, ok := item["field"].(string); ok {
			fields[strings.Fields(field)[0]] = true
		}
	}
	for _, name := range []string{"Listen", "DataDir", "Count"} {
		if !fields[name] {
			t.Fatalf("the tagged field %s was not asked: %v", name, fields)
		}
	}
	// The launch names its program by the word the answer chose.
	var programs []string
	for _, boundary := range projected.Outbound {
		if boundary.Kind == "runs_program" {
			programs = append(programs, boundary.Destination)
		}
	}
	if !slices.Contains(programs, "git") {
		t.Fatalf("programs started: %v", programs)
	}
	// net/http is asked once for the whole run, with each symbol called.
	systems := preset.items("atlas_systems.system")
	if len(systems) != 1 || !strings.Contains(systems[0], `"package":"net/http"`) || !strings.Contains(systems[0], `{"call":"http.DefaultClient.Do(req)","symbol":"Client.Do"}`) || !strings.Contains(systems[0], `"symbol":"Get"`) {
		t.Fatalf("systems asked: %v", systems)
	}
}

// jsonText is a value as JSON text, for a substring check of an item.
func jsonText(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	raw, err := json.Marshal(value)
	return string(raw), err == nil
}

// The Python fixture's inputs (src/fixture_app/tool_cli.py): argparse's
// add_parser names init and set_defaults hands its handler over on that
// call's own result, one input with its handler (J1); --verbose is declared
// on the parser ArgumentParser("tool") makes and --force on init's own
// parser, whose catalogue names init as what its members are declared on
// (K2). Python compares an argument with an operator, which is no call, so
// the per-call comparison has no Python equivalent; subprocess.run's list
// of words gives no call word, so its program stays not established.
func TestCumulativePythonInputsJoinAndCatalogue(t *testing.T) {
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
		symbol, _ := item["symbol"].(string)
		switch column {
		case "talks":
			if symbol == "httpx.get" {
				return "client_request", true
			}
		case "enters":
			if strings.HasSuffix(symbol, ".add_argument") || strings.HasSuffix(symbol, ".add_parser") {
				return "command", true
			}
		case "binds":
			if strings.HasSuffix(symbol, ".set_defaults") {
				return "command", true
			}
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	got := inputRows(projected, "src/fixture_app/tool_cli.py")
	want := []inputRow{
		{kind: "command", name: "-v", declaredBy: "build_parser", on: `argparse.ArgumentParser("tool")`, at: "src/fixture_app/tool_cli.py:22"},
		{kind: "command", name: "init", declaredBy: "build_parser", on: `parser.add_subparsers(dest="cmd")`, handler: "run_init", at: "src/fixture_app/tool_cli.py:24"},
		{kind: "command", name: "--force", declaredBy: "build_parser", on: `commands.add_parser("init")`, at: "src/fixture_app/tool_cli.py:26"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool_cli.py's inputs:\n%+v\nwant\n%+v", got, want)
	}
	catalogues := catalogueRows(projected)
	for _, row := range []string{
		`command on argparse.ArgumentParser("tool"): -v`,
		`command on commands.add_parser("init") (init): --force`,
	} {
		if !slices.Contains(catalogues, row) {
			t.Fatalf("no catalogue %q in %q", row, catalogues)
		}
	}
	for _, item := range preset.asked["program"] {
		t.Fatalf("a program was asked although no call word names it: %v", item)
	}
	// httpx, the package levels.py's outgoing call goes through, is asked
	// its system once, with the call as written (requests is the fixture's
	// own module, src/requests.py, and no outside package).
	systems := preset.items("atlas_systems.system")
	if len(systems) != 1 || !strings.Contains(systems[0], `"package":"httpx"`) || !strings.Contains(systems[0], `"symbol":"get"`) {
		t.Fatalf("systems asked: %v", systems)
	}
}

// The Clojure fixture's inputs (src/example/core.clj): clojure.core/= is
// asked per call, so shouted?'s comparison of the first argument with
// --shout is a command while default-row?'s comparison of a row's name is
// not; revision's shell/sh names git. tools.cli option vectors are no
// literals given to a call, so a catalogue of options has no Clojure
// equivalent yet (CLOJURE).
func TestCumulativeClojureInputsAreAskedPerCall(t *testing.T) {
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
	preset := &inputsPreset{decide: func(column string, item map[string]any, options []string) (string, bool) {
		symbol, _ := item["symbol"].(string)
		switch column {
		case "talks":
			if symbol == "clojure.java.shell.sh" {
				return "runs_program", true
			}
		case "enters":
			if symbol == "clojure.core.=" && strings.Contains(callText(item), "(first args)") {
				return "command", true
			}
		case "program":
			if slices.Contains(options, "git") {
				return "git", true
			}
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	var compared []string
	for _, item := range preset.asked["enters"] {
		if symbol, _ := item["symbol"].(string); symbol == "clojure.core.=" {
			compared = append(compared, callText(item))
		}
	}
	slices.Sort(compared)
	if want := []string{`(= (:name row) "default")`, `(= (first args) "--shout")`}; !slices.Equal(compared, want) {
		t.Fatalf("= calls asked: %q, want %q", compared, want)
	}
	got := inputRows(projected, "src/example/core.clj")
	if len(got) != 1 || got[0].kind != "command" || got[0].name != "--shout" || got[0].declaredBy != "example.core/shouted?" {
		t.Fatalf("core.clj's inputs: %+v", got)
	}
	var programs []string
	for _, boundary := range projected.Outbound {
		if boundary.Kind == "runs_program" {
			programs = append(programs, boundary.Destination)
		}
	}
	if !slices.Contains(programs, "git") {
		t.Fatalf("programs started: %v", programs)
	}
}
