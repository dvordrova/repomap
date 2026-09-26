package contracttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/atlas/reading/partstest"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// The parts request of each fixture carries code structure only and every
// declaration takes one part or an entry off the map. A Go method declared
// in another file than its type goes with its type: internal/localstore's
// Ledger.Append. Its own file, which declares nothing else, is no row of the
// parts request yet stays on the map in Ledger's part: the card does not
// list it as off the map. Python and TypeScript have no method outside its
// class; Clojure's defmethod, extend-type and extend-protocol are not
// declarations the Clojure adapter projects, so no equivalent exists there
// to check.
func TestCumulativeGoMapOfParts(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	library := analyzeGoFixture(t, root, repository, goFixtureRootPackage, "cumulative-go-map-of-parts")
	index, err := goadapter.Build(repository, library.target, library.origins, library.direct, library.external, library.core, library.dynamic, library.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	// go/scanner counts each init's own lines of code: not its doc comment,
	// the comment inside or the blank line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "internal/localstore/ledger.go", map[string][]int{
		"Ledger": {1}, "Ledger.Keys": {1}, "init": {4, 1},
	})
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "library", Name: index.Target.Name, Root: "."}, root)
	assertDescribeGolden(t, "go", checked)
	ledger := checked.Symbols[[2]string{"internal/localstore/ledger.go", "Ledger"}]
	appendMethod := checked.Symbols[[2]string{"internal/localstore/ledger_append.go", "Ledger.Append"}]
	if ledger == "" || appendMethod == "" {
		t.Fatalf("the cross-file method fixture is missing: %v %v", ledger, appendMethod)
	}
	if checked.PartOf[appendMethod] == "" || checked.PartOf[appendMethod] != checked.PartOf[ledger] {
		t.Fatalf("Ledger.Append is in %q, Ledger in %q", checked.PartOf[appendMethod], checked.PartOf[ledger])
	}
	for _, entry := range checked.Target.OffMap {
		if entry.File.Path == "internal/localstore/ledger_append.go" {
			t.Fatalf("the file of a method that follows its type is off the map: %+v", entry)
		}
	}
	// The component card lists GroupsIndex's off-map files row for row.
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, checked.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range indexes[0].OffMap {
		if file.Path == "internal/localstore/ledger_append.go" {
			t.Fatalf("the card lists ledger_append.go off the map: %+v", file)
		}
	}
	// The test declaration of a type declared in the library, root_test.go's
	// testRootReader.expected, follows that type in the same way.
	reader := checked.Symbols[[2]string{"root.go", "testRootReader"}]
	expected := checked.Symbols[[2]string{"root_test.go", "testRootReader.expected"}]
	if reader == "" || expected == "" || checked.PartOf[expected] != checked.PartOf[reader] {
		t.Fatalf("testRootReader.expected left its type: %q %q", expected, reader)
	}
	// Split, ledger.go's Ledger takes a role part and its Append, declared
	// in ledger_append.go, follows it there; its Keys too. The two inits
	// stay one unit.
	split := partstest.CheckSplit(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "library", Name: index.Target.Name, Root: "."}, root)
	ledger, appendMethod = split.Symbols[[2]string{"internal/localstore/ledger.go", "Ledger"}], split.Symbols[[2]string{"internal/localstore/ledger_append.go", "Ledger.Append"}]
	if !split.Split["internal/localstore/ledger.go"] || !split.RoleParts[split.PartOf[ledger]] || split.PartOf[appendMethod] != split.PartOf[ledger] {
		t.Fatalf("split: Ledger in %q, Ledger.Append in %q, split files %v", split.PartOf[ledger], split.PartOf[appendMethod], split.Split)
	}
	projectSplit(t, index, split)
}

// assertDescribeGolden compares every description request of a reading in
// which no file is split with the requests the code sent before the role
// split existed (testdata/describe_<language>.json, taken at f49c3304 on
// the same fixtures): listing the units a part holds changes no byte of an
// unsplit part's request, so their cache survives. A part with no member
// to describe it by, which then sent its name alone, now sends nothing.
func assertDescribeGolden(t *testing.T, language string, checked partstest.Map) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "describe_"+language+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	for part, request := range golden {
		got, asked := checked.Described[part]
		switch {
		case !strings.Contains(request, `"directories"`):
			if asked {
				t.Fatalf("%s, with no member, was described: %s", part, got)
			}
		case got != request:
			t.Fatalf("the description request of %s changed:\n%s\nwas\n%s", part, got, request)
		}
	}
	for part := range checked.Described {
		if _, known := golden[part]; !known {
			t.Fatalf("%s is described but was not before", part)
		}
	}
}

// projectSplit checks that GroupsIndex accepts a split atlas and lists a
// split file's undecided declarations by name without calling the file off
// the map.
func projectSplit(t *testing.T, index programindex.Index, split partstest.Map) {
	t.Helper()
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, split.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	undecided := 0
	for _, file := range indexes[0].OffMap {
		if split.Split[file.Path] && (file.Reason != groupindex.OffMapUndecided || len(file.Declarations) == 0) {
			t.Fatalf("the split file %s is listed off the map: %+v", file.Path, file)
		}
		if file.Reason == groupindex.OffMapUndecided {
			undecided++
		}
	}
	for _, entry := range split.Target.OffMap {
		if entry.Reason == atlas.OffMapUndecided && undecided == 0 {
			t.Fatalf("undecided declarations of %s are not listed", entry.File.Path)
		}
	}
}

func TestCumulativePythonMapOfParts(t *testing.T) {
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
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	// A module declaring __all__ exports exactly what it lists: the parts
	// request shows the signature of render_level, not of format_score.
	seen := 0
	for _, object := range index.Objects {
		if object.Location == nil || object.Location.Path != "src/fixture_app/exports.py" {
			continue
		}
		want := map[string]programindex.Visibility{"render_level": programindex.VisibilityPublic, "format_score": programindex.VisibilityInternal}[object.Name]
		if want == "" {
			continue
		}
		seen++
		if object.Visibility != want {
			t.Fatalf("%s visibility %s, want %s", object.Name, object.Visibility, want)
		}
	}
	if seen != 2 {
		t.Fatalf("exports.py declarations seen: %d", seen)
	}
	// tokenize counts code lines outside the docstring, the comment and the
	// blank line; an overload stub is its one line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "src/fixture_app/generic_types.py", map[string][]int{
		"pick": {1, 1, 3}, "first": {2},
	})
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root)
	assertDescribeGolden(t, "python", checked)
	// A function nested in another takes its parent's part.
	outer := checked.Symbols[[2]string{"src/fixture_app/http_registrations.py", "register_route"}]
	nested := checked.Symbols[[2]string{"src/fixture_app/http_registrations.py", "empty_registered_handler"}]
	if nested == "" || outer == "" || checked.PartOf[nested] != checked.PartOf[outer] {
		t.Fatalf("a nested function left its parent's part: %q %q", nested, outer)
	}
	// Split, the nested function still follows its parent into a role part.
	split := partstest.CheckSplit(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root)
	outer, nested = split.Symbols[[2]string{"src/fixture_app/http_registrations.py", "register_route"}], split.Symbols[[2]string{"src/fixture_app/http_registrations.py", "empty_registered_handler"}]
	if !split.Split["src/fixture_app/http_registrations.py"] || split.PartOf[nested] != split.PartOf[outer] {
		t.Fatalf("split: the nested function in %q, its parent in %q", split.PartOf[nested], split.PartOf[outer])
	}
	projectSplit(t, index, split)
}

func TestCumulativeClojureMapOfParts(t *testing.T) {
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
	// The reader counts code lines outside the docstring, the ;; comment and
	// the blank line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "src/example/core.clj", map[string][]int{
		"example.core/shout": {1, 4}, "example.core/loud-greeting": {2},
	})
	meta := reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "executable", Name: index.Target.Name, Root: "."}
	assertDescribeGolden(t, "clojure", partstest.Check(t, graph, meta, root))
	// Split, core.clj is the seed file: its -main keeps the entry.
	split := partstest.CheckSplit(t, graph, meta, root)
	if !split.Split["src/example/core.clj"] || len(split.Target.Trace) == 0 {
		t.Fatalf("split: core.clj split %v, trace %v", split.Split["src/example/core.clj"], split.Target.Trace)
	}
	projectSplit(t, index, split)
}

// The C server's parts request carries code structure only, and every
// declaration of its files takes one part or an entry off the map: the
// backend loop.c includes (loop_poll.c) with its includer's declarations,
// and the headers' types, prototypes and static inline functions.
func TestCumulativeCMapOfParts(t *testing.T) {
	fixture := loadCFixture(t)
	index := buildCIndex(t, fixture, "c:kvd")
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	// The lexer skips the comment inside bgsaveCommand; strings holding
	// "//" or "/*" stay code.
	adaptertest.AssertDeclarationCodeLines(t, graph, "kvd.c", map[string][]int{
		"bgsaveCommand": {17}, "processCommand": {12},
	})
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}, fixture.root)
	assertDescribeGolden(t, "c", checked)
	for _, declaration := range [][2]string{{"kvd.c", "main"}, {"loop_poll.c", "loopApiPoll"}, {"strbuf.h", "sbAvail"}, {"kvd.h", "kvClient"}} {
		if checked.Symbols[declaration] == "" {
			t.Fatalf("%s %s is not on the map", declaration[0], declaration[1])
		}
	}
	// Split, kvd.c is the seed file, as redis.c is redis-server's: the part
	// holding main starts the trace and stands in the "in" column.
	split := partstest.CheckSplit(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}, fixture.root)
	main := split.Symbols[[2]string{"kvd.c", "main"}]
	if !split.Split["kvd.c"] || split.PartOf[main] == "" || len(split.Target.Trace) == 0 || split.Target.Trace[0] != split.PartOf[main] {
		t.Fatalf("split: main in %q, trace %v", split.PartOf[main], split.Target.Trace)
	}
	projectSplit(t, index, split)
}
