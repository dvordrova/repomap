package contracttest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// A plain assignment to a record's field keeps the value it stores, as any
// source value is recorded: kvd's main stores the literal "dump.kv" in
// server.dbfile, its configuration reader what strdup returns; ++ and a
// compound assignment store no value of their own (C only: Go records its
// writes without a value, Python sets no path, JS/TS and Clojure record no
// field write).
func TestCFixtureFieldWritesKeepTheValueTheyStore(t *testing.T) {
	fixture := loadCFixture(t)
	server := buildCIndex(t, fixture, "c:kvd")
	names := map[string]string{}
	for _, object := range server.Objects {
		names[object.ID] = object.Name
	}
	written := map[string]string{}
	for _, relation := range server.Relations {
		if relation.Kind != programindex.RelationWrites || relation.FieldPath == "" {
			continue
		}
		key := fmt.Sprintf("%s by %s@%d", relation.FieldPath, names[relation.FromID], relation.Location.Line)
		switch value := relation.Value; {
		case value == nil:
			written[key] = "none"
		default:
			written[key] = value.Kind + " " + value.Text
		}
	}
	line := func(needle string) int {
		at, _ := fixture.at(t, "kvd.c", needle, needle[:1])
		return at
	}
	for key, want := range map[string]string{
		fmt.Sprintf("server.dbfile by main@%d", line(`server.dbfile = "dump.kv";`)):                   "literal dump.kv",
		fmt.Sprintf("server.dbfile by loadConfig@%d", line("server.dbfile = strdup(argv[1])")):        "call_result strdup(argv[1])",
		fmt.Sprintf("server.dirty by setCommand@%d", line("server.dirty++;")):                         "none",
		fmt.Sprintf("server.dirty by delCommand@%d", line("server.dirty += deleted;")):                "none",
		fmt.Sprintf("server.saveChild by beforeSleep@%d", line("server.saveChild = -1;")):             "unknown -1",
		fmt.Sprintf("server.shutdown by onSignal@%d", line("server.shutdown = 1;")):                   "literal 1",
		fmt.Sprintf("server.port by main@%d", line("server.port = port != NULL ? atoi(port) : KVD_")): "unknown port != NULL ? atoi(port) : KVD_DEFAULT_PORT",
	} {
		if written[key] != want {
			t.Errorf("%s stores %q, want %q", key, written[key], want)
		}
	}
}

// The files a program keeps are gathered from its file calls, walked along
// the decided path argument to where the path ends (reading.FileReader). A
// preset stands for the talks and argument answers; the walk and the
// grouping are code. Each language's fixture reuses what it already has:
//
//   - C: kvd's saveSnapshot opens the snapshot through its parameter, which
//     bgsaveCommand passes from server.dbfile, whose default main stores
//     ("dump.kv") and whose configuration override loadConfig stores from
//     the line it read (not established); loadConfig opens the file
//     KVD_CONFIG names.
//   - Go: createFixtureState creates "fixture-state.db"; LoadServerConfig
//     reads the path it is given, which no call gives: not established. Go
//     records a field's writes without their values, so no field is one.
//     ReadFixtureJournal reads store.path + "-journal", whose path no call
//     gives: the template {store.path}-journal.
//   - Python: read_settings opens Path(name), whose name no call gives: not
//     established. A field stored twice (MutableAdapter's url, default and
//     replacement) is unknown to the Python adapter, so its file would be
//     not established too; the fixture has no file read from a field.
//   - Clojure: deliver! spits to destination, its own parameter, which
//     core.clj's -main passes as "greeting.txt": the file. A Clojure value
//     is no field.
//   - JS/TS: the fixture's node:fs calls (prepare-readme.mjs's readFileSync
//     through prepareReadme("README.md")) name no symbol without
//     @types/node, so no talks answer makes them file calls (JSTS).
func TestEveryLanguageKeepsTheFilesItsCodeReaches(t *testing.T) {
	// files are the records as "name: call function@line …" lines, with a
	// field's values "= value by function@line".
	files := func(graph atlas.Graph, index programindex.Index, symbols []string, choices map[string]reading.ArgumentChoice) []string {
		t.Helper()
		names := map[string]string{}
		for _, object := range index.Objects {
			names[index.Target.ID+"."+object.ID] = object.Name
			names[object.ID] = object.Name
		}
		var result []string
		for _, record := range reading.NewFileReader(graph.Places, symbols, reading.DestinationChoices{Arguments: choices}, nil).Files(index.Target.ID) {
			data := record.Data
			if err := data.Validate(); err != nil {
				t.Fatalf("%s: %v", record.ID, err)
			}
			name := data.Name
			if name == "" {
				name = "(not established)"
			}
			var parts []string
			for _, call := range data.File.Calls {
				parts = append(parts, fmt.Sprintf("%s %s@%d", call.Symbol, names[call.ObjectID], call.Anchor.Line))
			}
			for _, value := range data.File.Values {
				written := value.Value
				if written == "" {
					written = "?"
				}
				parts = append(parts, fmt.Sprintf("= %s by %s@%d", written, names[value.ObjectID], value.Anchor.Line))
			}
			result = append(result, name+": "+strings.Join(parts, " "))
		}
		return result
	}
	t.Run("c", func(t *testing.T) {
		fixture := loadCFixture(t)
		index := buildCIndex(t, fixture, "c:kvd")
		layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
		if err != nil {
			t.Fatal(err)
		}
		graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
		if err != nil {
			t.Fatal(err)
		}
		line := func(needle string) int {
			at, _ := fixture.at(t, "kvd.c", needle, needle[:1])
			return at
		}
		got := files(graph, index, []string{"stdio.h.fopen"}, map[string]reading.ArgumentChoice{"stdio.h.fopen": {Position: 1}})
		want := []string{
			fmt.Sprintf("{server.dbfile}: stdio.h.fopen saveSnapshot@%d = ? by loadConfig@%d = dump.kv by main@%d",
				line(`fopen(filename, "w")`), line("server.dbfile = strdup(argv[1])"), line(`server.dbfile = "dump.kv";`)),
			fmt.Sprintf("{env:KVD_CONFIG}: stdio.h.fopen loadConfig@%d", line(`fopen(filename, "r")`)),
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("kvd's files = %q\nwant %q", got, want)
		}
		// Without a decided argument a file call's path is not established:
		// one record per function, never a path.
		got = files(graph, index, []string{"stdio.h.fopen"}, nil)
		want = []string{
			fmt.Sprintf("(not established): stdio.h.fopen saveSnapshot@%d", line(`fopen(filename, "w")`)),
			fmt.Sprintf("(not established): stdio.h.fopen loadConfig@%d", line(`fopen(filename, "r")`)),
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("kvd's files without an argument = %q\nwant %q", got, want)
		}
	})
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		authorities := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "files")
		index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
		if err != nil {
			t.Fatal(err)
		}
		got := files(graph, index, []string{"os.Create", "os.ReadFile"}, map[string]reading.ArgumentChoice{"os.Create": {Position: 1}, "os.ReadFile": {Position: 1}})
		// settingFile reads a file per key its callers hand it
		// (cmd/app/setting_lookup.go), each walked to its own key.
		settingFile := fixtureLine(t, "go", "cmd/app/setting_lookup.go", `os.ReadFile(key + ".conf")`)
		want := []string{
			fmt.Sprintf("dataSourceName.conf: os.ReadFile settingFile@%d", settingFile),
			fmt.Sprintf("staticBaseUrl.conf: os.ReadFile settingFile@%d", settingFile),
			fmt.Sprintf("fixture-state.db: os.Create createFixtureState@%d", fixtureLine(t, "go", "internal/storefixture/fixtures.go", `os.Create("fixture-state.db")`)),
			fmt.Sprintf("{path}-journal: os.ReadFile ReadFixtureJournal@%d", fixtureLine(t, "go", "internal/storefixture/fixtures.go", "os.ReadFile(store.journalPath())")),
			fmt.Sprintf("{path}.tmp: os.ReadFile ReadFirstStagedJournal@%d", fixtureLine(t, "go", "internal/storefixture/fixtures.go", `os.ReadFile(stagedJournals()[0].path + ".tmp")`)),
			fmt.Sprintf("(not established): os.ReadFile LoadServerConfig@%d", fixtureLine(t, "go", "internal/storefixture/tool_cli.go", "os.ReadFile(path)")),
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("the Go fixture's files = %q\nwant %q", got, want)
		}
	})
	t.Run("python", func(t *testing.T) {
		_, repository := materializeFixtureRepository(t, "python")
		index := pythonLibraryIndex(t)
		graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
		if err != nil {
			t.Fatal(err)
		}
		// open is made on what Path returns, which carries Path's first
		// argument (the argument decision's next round).
		got := files(graph, index, []string{"pathlib.Path.open"}, map[string]reading.ArgumentChoice{"pathlib.Path.open": {Receiver: true}, "pathlib.Path": {Position: 1}})
		if want := []string{fmt.Sprintf("(not established): pathlib.Path.open read_settings@%d", fixtureLine(t, "python", "src/fixture_app/outside_results.py", "with Path(name).open()"))}; !reflect.DeepEqual(got, want) {
			t.Fatalf("the Python fixture's files = %q\nwant %q", got, want)
		}
		// create_datadir's datadir=None names no file, and its f-string reads
		// by the key it looks config up by.
		got = files(graph, index, []string{"pathlib.Path"}, map[string]reading.ArgumentChoice{"pathlib.Path": {Position: 1}})
		datadir := fixtureLine(t, "python", "src/fixture_app/outside_results.py", "return Path(datadir) if datadir")
		if want := fmt.Sprintf("{user_data_dir}/data: pathlib.Path create_datadir@%d", datadir); !slices.Contains(got, want) || slices.ContainsFunc(got, func(file string) bool { return strings.HasPrefix(file, "None:") }) {
			t.Fatalf("create_datadir's files = %q, want %q and no None", got, want)
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
		graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
		if err != nil {
			t.Fatal(err)
		}
		got := files(graph, index, []string{"clojure.core.spit"}, map[string]reading.ArgumentChoice{"clojure.core.spit": {Position: 1}})
		if want := []string{fmt.Sprintf("greeting.txt: clojure.core.spit example.service/deliver!@%d", fixtureLine(t, "clojure", "src/example/service.cljc", "(spit destination message)"))}; !reflect.DeepEqual(got, want) {
			t.Fatalf("the Clojure fixture's files = %q\nwant %q", got, want)
		}
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		result, err := jstsproject.DiscoverSelected(t.Context(), repository, root, "jsts:packages/documentation-tools/package.json")
		if err != nil {
			t.Fatal(err)
		}
		index, _, err := jstsproject.BuildFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
		if err != nil {
			t.Fatal(err)
		}
		for _, place := range graph.Places {
			if place.Symbol == nil {
				continue
			}
			for _, call := range place.Symbol.Calls {
				if call.Name == "readFileSync" && (call.API != nil || call.Kind == string(programindex.RelationInvokesExternal)) {
					t.Fatalf("readFileSync names an outside symbol: %+v", call)
				}
			}
		}
	})
}

// fixtureLine is the line of a language fixture's file where needle is
// written first.
func fixtureLine(t *testing.T, language, path, needle string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "testdata", "repositories", language, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	offset := strings.Index(string(raw), needle)
	if offset < 0 {
		t.Fatalf("%s has no %q", path, needle)
	}
	return strings.Count(string(raw[:offset]), "\n") + 1
}
