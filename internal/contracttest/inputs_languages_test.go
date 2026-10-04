package contracttest

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/llm"
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
	// A reader of LooksLikeFlag's strings.HasPrefix(arg, "-") answers none
	// when the command option is not for a prefix a word is tested to start
	// with, and command, as litestream's reading did, when it is.
	preset.read = func(column string, item map[string]any, options []llm.Option) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "enters" && strings.HasSuffix(symbol, "strings.HasPrefix") {
			if strings.Contains(notFor(options, "command"), "a character or prefix a word is tested to start with") {
				return "none", true
			}
			return "command", true
		}
		return "", false
	}
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
	// LooksLikeFlag's "-" is asked with its call as written and is no input.
	if !slices.ContainsFunc(preset.asked["enters"], func(item map[string]any) bool { return callText(item) == `strings.HasPrefix(arg, "-")` }) {
		t.Fatal(`strings.HasPrefix(arg, "-") was not asked`)
	}
	for _, row := range inputRows(projected, "internal/storefixture/tool_cli.go") {
		if row.name == "-" {
			t.Fatalf("a prefix a word is tested to start with is an input: %+v", row)
		}
	}
	got := inputRows(projected, "internal/storefixture/tool_cli.go", "internal/storefixture/destinations.go")
	want := []inputRow{
		{kind: "command", name: "price-endpoint", declaredBy: "DestinationApplication", at: "internal/storefixture/destinations.go:28"},
		{kind: "command", name: "verbose", declaredBy: "runServe", on: `flag.NewFlagSet("serve", flag.ContinueOnError)`, at: "internal/storefixture/tool_cli.go:100"},
		{kind: "command", name: "verbose", declaredBy: "runCheck", on: `flag.NewFlagSet("check", flag.ContinueOnError)`, at: "internal/storefixture/tool_cli.go:112"},
		// addCommon declares quiet on the flag set it is handed: a
		// parameter is no object its own call made.
		{kind: "command", name: "quiet", declaredBy: "addCommon", at: "internal/storefixture/tool_cli.go:126"},
		{kind: "setting", name: "listen", declaredBy: "ServerConfig", on: "json.Unmarshal(raw, &config)", at: "internal/storefixture/tool_cli.go:15"},
		{kind: "setting", name: "data_dir", declaredBy: "ServerConfig", on: "json.Unmarshal(raw, &config)", at: "internal/storefixture/tool_cli.go:16"},
		{kind: "command", name: "port", declaredBy: "ToolCommand", on: `flag.NewFlagSet("serve", flag.ContinueOnError)`, at: "internal/storefixture/tool_cli.go:36"},
		{kind: "command", name: "strict", declaredBy: "ToolCommand", on: `flag.NewFlagSet("check", flag.ContinueOnError)`, at: "internal/storefixture/tool_cli.go:38"},
		{kind: "command", name: "check", declaredBy: "ToolCommand", at: "internal/storefixture/tool_cli.go:39"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs:\n%+v\nwant\n%+v", got, want)
	}
	// Each input keeps its registration as the code wrote it.
	if written := writtenRows(projected, "internal/storefixture/tool_cli.go"); written["check"] != `strings.EqualFold(os.Args[1], "check")` || !strings.Contains(written["port"], `"port"`) {
		t.Fatalf("registrations as written: %q", written)
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
	// net/http is asked once for the whole run, with each call of each
	// symbol given different values (B7): Get's literal path, a setting
	// getter's result, a field's value and a concatenation are four
	// observations, and Client.Do's
	// request req.WithContext makes is another than the variable req.
	systems := preset.items("atlas_systems.system")
	if len(systems) != 1 || !strings.Contains(systems[0], `"package":"net/http"`) ||
		!strings.Contains(systems[0], `{"calls":["http.DefaultClient.Do(req)","http.DefaultClient.Do(req.WithContext(ctx))"],"symbol":"Client.Do"}`) ||
		!strings.Contains(systems[0], `{"calls":["http.Get(\"/api/levels\")","http.Get(settingOrDefault(\"staticBaseUrl\"))","http.Get(client.Base)","http.Get(base + \"/items\")"],"symbol":"Get"}`) {
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
// the per-call comparison has no Python equivalent; fnmatch.fnmatch in
// run_init compares a field of what init's handler was handed with a word,
// init's sub-argument, never asked (K3). subprocess.run's list of words
// gives no call word, so its program stays not established.
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
			// add_subparsers(dest=…) and a pop of a key are answered as
			// freqtrade's reading answered them.
			if strings.HasSuffix(symbol, ".add_argument") || strings.HasSuffix(symbol, ".add_parser") ||
				strings.HasSuffix(symbol, ".add_subparsers") || strings.HasSuffix(symbol, ".pop") {
				return "command", true
			}
			// bare_variant's fnmatch, a helper init's handler calls, is
			// asked and answered a command.
			if symbol == "fnmatch.fnmatch" {
				return "command", true
			}
		case "binds":
			if strings.HasSuffix(symbol, ".set_defaults") {
				return "command", true
			}
		}
		return "", false
	}}
	// A reader of a Query call answers none when its only words are given
	// as a description, help or usage and the request option is not for
	// one, else request, as freqtrade's reading did.
	preset.read = func(column string, item map[string]any, options []llm.Option) (string, bool) {
		if symbol, _ := item["symbol"].(string); column != "enters" || !strings.HasSuffix(symbol, ".Query") {
			return "", false
		}
		arguments, _ := item["arguments"].([]any)
		literals, _ := item["literals"].([]any)
		described := false
		for _, literal := range literals {
			word, _ := literal.(string)
			for _, argument := range arguments {
				text, _ := argument.(string)
				if given, value, _ := strings.Cut(text, ": "); value == strconv.Quote(word) {
					if given != "description" && given != "help" && given != "usage" {
						return "request", true
					}
					described = true
				}
			}
		}
		if described && strings.Contains(notFor(options, "request"), "a parameter's description, help or usage text") {
			return "none", true
		}
		return "request", true
	}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	// A pop out of a copy of the code's own table row (dispatch.py's
	// option_help, freqtrade's options.pop("help")) is not asked.
	for _, item := range preset.asked["enters"] {
		if symbol, _ := item["symbol"].(string); strings.HasSuffix(symbol, ".pop") {
			t.Fatalf("a call on the code's own table row was asked: %v", item)
		}
	}
	for _, row := range inputRows(projected, "src/fixture_app/dispatch.py") {
		if row.name == "help" {
			t.Fatalf("dispatch.py's input %+v", row)
		}
	}
	// A query parameter's description names no input; its alias does, and
	// the name column offers each word with the parameter it is given as.
	offered := false
	for _, item := range preset.asked["atlas_boundaries.name"] {
		words, _ := item["words"].([]any)
		given := map[any]bool{}
		for _, word := range words {
			offered, _ := word.(map[string]any)
			given[offered["given"]] = true
		}
		offered = offered || given["description"] && given["alias"]
	}
	if !offered {
		t.Fatalf("no name row offers a word with its parameter: %v", preset.items("atlas_boundaries.name"))
	}
	var queries []string
	for _, row := range inputRows(projected, "src/fixture_app/cli.py") {
		if row.kind == "request" && row.declaredBy != "" && !strings.HasPrefix(row.name, "/") {
			queries = append(queries, row.name)
		}
	}
	if !slices.Equal(queries, []string{"token"}) {
		t.Fatalf("cli.py's query inputs %q, want [token]", queries)
	}
	// The dest of add_subparsers, given only as dest=, on which subcommands
	// with their own handlers are declared (init, serve), is none of their
	// tiles.
	got := inputRows(projected, "src/fixture_app/tool_cli.py")
	want := []inputRow{
		// build_remote's group: remote, the word a person types before add,
		// stays an input; each add_subparsers(dest=…) is none.
		{kind: "command", name: "remote", declaredBy: "build_remote", on: `parser.add_subparsers(dest="cmd")`, at: "src/fixture_app/tool_cli.py:100"},
		{kind: "command", name: "add", declaredBy: "build_remote", on: `remote.add_subparsers(dest="remote_cmd")`, handler: "run_remote_add", at: "src/fixture_app/tool_cli.py:102"},
		// A word a helper init's handler calls checks: an input of its own,
		// no word init's handler checks.
		{kind: "command", name: "init-bare", declaredBy: "bare_variant", at: "src/fixture_app/tool_cli.py:114"},
		// run_init, init's handler, compares its argument's cmd with
		// init-*: a word only it checks, init's sub-argument (K3), never
		// asked what it becomes.
		{kind: "command", name: "init-*", declaredBy: "run_init", at: "src/fixture_app/tool_cli.py:19"},
		{kind: "command", name: "-v", declaredBy: "build_parser", on: `argparse.ArgumentParser("tool")`, at: "src/fixture_app/tool_cli.py:26"},
		{kind: "command", name: "init", declaredBy: "build_parser", on: `parser.add_subparsers(dest="cmd")`, handler: "run_init", at: "src/fixture_app/tool_cli.py:28"},
		{kind: "command", name: "--force", declaredBy: "build_parser", on: `commands.add_parser("init")`, at: "src/fixture_app/tool_cli.py:30"},
		{kind: "command", name: "status", declaredBy: "build_parser", on: `parser.add_subparsers(dest="cmd")`, at: "src/fixture_app/tool_cli.py:33"},
		// ServiceCommands keeps its parser and subcommands in fields, each
		// stored once from argparse's call: serve joins its handler as init
		// does. RebuiltParser stores its parser twice, so --again, on either
		// parser, is no input.
		{kind: "command", name: "serve", declaredBy: "build", on: `self.parser.add_subparsers(dest="cmd")`, handler: "run_serve", at: "src/fixture_app/tool_cli.py:63"},
		// A helper declares on the parser it is handed: a parameter is no
		// object its own call made (subcommand_options_test.go nests them).
		{kind: "command", name: "--quiet", declaredBy: "add_common", at: "src/fixture_app/tool_cli.py:84"},
		{kind: "command", name: "--json", declaredBy: "add_output", at: "src/fixture_app/tool_cli.py:90"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool_cli.py's inputs:\n%+v\nwant\n%+v", got, want)
	}
	if written := writtenRows(projected, "src/fixture_app/tool_cli.py"); !strings.Contains(written["--force"], `"--force"`) || !strings.Contains(written["init"], `"init"`) {
		t.Fatalf("registrations as written: %q", written)
	}
	for _, item := range preset.asked["enters"] {
		if symbol, _ := item["symbol"].(string); symbol == "fnmatch.fnmatch" && strings.HasPrefix(stringOf(item["in"]), "run_init") {
			t.Fatalf("a word init's handler compares with what it was handed was asked: %v", item)
		}
	}
	for position, operation := range projected.Operations {
		if operation.Name == "init-*" && !projected.Launch.Nested[operation.ID] {
			t.Fatal("init-* is not nested under init")
		}
		if operation.Name == "init" {
			var subArguments []string
			for _, id := range projected.Reach[position].SubArguments {
				for _, other := range projected.Operations {
					if other.ID == id {
						subArguments = append(subArguments, other.Name)
					}
				}
			}
			if !slices.Equal(subArguments, []string{"init-*"}) {
				t.Fatalf("init's sub-arguments: %v", subArguments)
			}
		}
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
	// A reader of the sketch's :draw hand-over answers extension when the
	// extension option includes a callback a framework calls every frame,
	// and none, as othello's near-tie fell, when it does not.
	preset.read = func(column string, item map[string]any, options []llm.Option) (string, bool) {
		symbol, _ := item["symbol"].(string)
		if column != "binds" || symbol != "quil.core.sketch.draw" {
			return "", false
		}
		for _, option := range options {
			if option.Name == "extension" && option.Criteria != nil && strings.Contains(option.Criteria.Includes, "calls every frame") {
				return "extension", true
			}
		}
		return "none", true
	}
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
	var shout, draw bool
	for _, row := range got {
		shout = shout || row.kind == "command" && row.name == "--shout" && row.declaredBy == "example.core/shouted?"
		draw = draw || row.kind == "extension" && row.handler == "example.core/draw-greeting"
	}
	if len(got) != 2 || !shout || !draw {
		t.Fatalf("core.clj's inputs: %+v", got)
	}
	if written := writtenRows(projected, "src/example/core.clj"); written["--shout"] != `(= (first args) "--shout")` {
		t.Fatalf("registration as written: %q", written)
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

// stringOf is an item's field as text, or "".
func stringOf(value any) string {
	text, _ := value.(string)
	return text
}
