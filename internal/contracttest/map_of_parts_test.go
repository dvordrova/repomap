package contracttest

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/atlas/reading/partstest"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/modeldiag"
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
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	// go/scanner counts each init's own lines of code: not its doc comment,
	// the comment inside or the blank line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "internal/localstore/ledger.go", map[string][]int{
		"Ledger": {1}, "Ledger.Keys": {1}, "init": {4, 1},
	})
	// The graph records what a declaration hands over: the command table's
	// rows hand getCommand over, and registerRouteDefinition hands the
	// closure requireRouteToken returns to http.HandleFunc. Go emits no
	// reads (GO), so no Go declaration records one.
	adaptertest.AssertDeclarationUses(t, graph,
		adaptertest.DeclarationUse{FromPath: "internal/storefixture/command_table.go", From: "commandTable", Kind: "passes_callback", ToPath: "internal/storefixture/command_table.go", To: "getCommand"},
		adaptertest.DeclarationUse{FromPath: "internal/storefixture/http_registrations.go", From: "registerRouteDefinition", Kind: "passes_callback", ToPath: "internal/storefixture/http_registrations.go", To: "requireRouteToken$1"},
	)
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "library", Name: index.Target.Name, Root: "."}, root)
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
	// A handler's assignment shows the words of the route that hands it
	// over.
	if registered := split.Registered["internal/storefixture/http_registrations.go"]; !slices.Contains(registered, "HandleFunc /v1/update") {
		t.Fatalf("http_registrations.go's handlers are asked with registrations %v", registered)
	}
	// The helper question: lookupCommand, unexported and called only by
	// DispatchCommand, is a helper and goes with it by code; DispatchCommand,
	// exported and called by nothing, is no helper by code and not asked;
	// the command table's getCommand and setCommand, which only its rows hand
	// over, have no user to follow and are asked once more. Go records no
	// reads, so no item carries read_by (GO).
	commands := "internal/storefixture/command_table.go"
	lookup, dispatch := split.Symbols[[2]string{commands, "lookupCommand"}], split.Symbols[[2]string{commands, "DispatchCommand"}]
	if !split.Helpers[[2]string{commands, "lookupCommand"}] || split.PartOf[lookup] == "" || split.PartOf[lookup] != split.PartOf[dispatch] || !recorded(split, "role_attached", "lookupCommand") {
		t.Fatalf("lookupCommand in %q, DispatchCommand in %q", split.PartOf[lookup], split.PartOf[dispatch])
	}
	if _, asked := split.HelperItems[[2]string{commands, "DispatchCommand"}]; asked {
		t.Fatal("DispatchCommand, which nothing calls, was asked the helper question")
	}
	if !recorded(split, "role_second_pass", "getCommand") || !recorded(split, "role_second_pass", "setCommand") {
		t.Fatal("the table's handlers were not asked once more")
	}
	for key, item := range split.HelperItems {
		if item["read_by"] != nil {
			t.Fatalf("the Go declaration %v carries read_by", key)
		}
	}
	projectSplit(t, index, split)
}

// graphWithFacts is the places graph of one target with its fact layer, as
// an ordinary run builds it: its registrations are boundary places.
func graphWithFacts(t *testing.T, repository *corpus.Corpus, target places.TargetInput) atlas.Graph {
	t.Helper()
	root := target.Root
	if root == "" {
		root = "."
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: target.Index, Root: root}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{target}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

// recorded says whether the reading recorded a row of this kind naming the
// sample.
func recorded(split partstest.Map, kind, sample string) bool {
	return slices.ContainsFunc(split.Rejected, func(row modeldiag.Row) bool { return row.Kind == kind && slices.Contains(row.Samples, sample) })
}

// projectSplit checks that GroupsIndex accepts a split atlas and lists a
// split file's undecided declarations by name without calling the file off
// the map.
func projectSplit(t *testing.T, index programindex.Index, split partstest.Map) groupindex.Index {
	t.Helper()
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, split.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	undecided := 0
	subjects := map[string]groupindex.Subject{}
	for _, subject := range indexes[0].Subjects {
		subjects[subject.ID] = subject
	}
	checkUnreachedParts(t, index, indexes[0])
	for _, file := range indexes[0].OffMap {
		// A split file is on the map through its role parts; only its
		// undecided declarations, or those of a role part the program never
		// runs, are listed off it.
		if split.Split[file.Path] && (file.Reason != groupindex.OffMapUndecided && file.Reason != groupindex.OffMapUnreachable || len(file.SubjectIDs) == 0) {
			t.Fatalf("the split file %s is listed off the map: %+v", file.Path, file)
		}
		if file.Reason != groupindex.OffMapUndecided {
			continue
		}
		undecided++
		// Each undecided declaration is named by its subject in that file,
		// so the card can link it to its source.
		var want int
		for _, entry := range split.Target.OffMap {
			if entry.Reason == atlas.OffMapUndecided && entry.File.Path == file.Path {
				want += len(entry.File.Symbols)
			}
		}
		if len(file.SubjectIDs) != want {
			t.Fatalf("%s lists %d undecided subjects for %d declarations", file.Path, len(file.SubjectIDs), want)
		}
		for _, id := range file.SubjectIDs {
			if object := subjects[id].Object; object == nil || object.Location == nil || object.Location.Path != file.Path {
				t.Fatalf("the undecided subject %s of %s is not a declaration of that file", id, file.Path)
			}
		}
	}
	for _, entry := range split.Target.OffMap {
		if entry.Reason == atlas.OffMapUndecided && undecided == 0 {
			t.Fatalf("undecided declarations of %s are not listed", entry.File.Path)
		}
	}
	return indexes[0]
}

// checkUnreachedParts holds a program's parts to its adapter's reachability:
// a part listed off its map as unreachable lists declarations of that file
// that no group holds, and every one of them that runs is one the program's
// index proves unreachable; a group holds at least one declaration that may
// run, or none that runs at all.
func checkUnreachedParts(t *testing.T, program programindex.Index, index groupindex.Index) {
	t.Helper()
	objects := map[string]programindex.Object{}
	for _, object := range program.Objects {
		objects[object.ID] = object
	}
	grouped := map[string]bool{}
	for _, group := range index.Groups {
		runs, reached := false, false
		for _, id := range group.MemberSubjectIDs {
			grouped[id] = true
			if object := objects[id]; object.Kind.Callable() {
				runs, reached = true, reached || !object.Unreachable
			}
		}
		if runs && !reached {
			t.Fatalf("the part %q, which its program never runs, is on its map", group.Title)
		}
	}
	for _, file := range index.OffMap {
		if file.Reason != groupindex.OffMapUnreachable {
			continue
		}
		for _, id := range file.SubjectIDs {
			object := objects[id]
			if object.Location == nil || object.Location.Path != file.Path || grouped[id] || object.Kind.Callable() && !object.Unreachable {
				t.Fatalf("%s lists %s (%s) as a part its program never runs", file.Path, object.Name, id)
			}
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
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
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
	// The graph records what a declaration reads, across files, and what
	// decorates it, a bare decorator name included.
	adaptertest.AssertDeclarationUses(t, graph,
		adaptertest.DeclarationUse{FromPath: "src/fixture_app/models.py", From: "read_level_data", Kind: "reads", ToPath: "src/fixture_app/levels.py", To: "READ_VALUES"},
		adaptertest.DeclarationUse{FromPath: "src/fixture_app/models.py", From: "read_level_data", Kind: "reads", ToPath: "src/fixture_app/levels.py", To: "READ_LIMIT"},
		adaptertest.DeclarationUse{FromPath: "src/fixture_app/models.py", From: "traced_level", Kind: "decorates", ToPath: "src/fixture_app/models.py", To: "traced"},
	)
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root)
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
	if registered := split.Registered["src/fixture_app/http_registrations.py"]; !slices.Contains(registered, "get /health") {
		t.Fatalf("http_registrations.py's handlers are asked with registrations %v", registered)
	}
	// The helper question: format_score, which __all__ leaves out and only
	// render_level calls, is a helper, so exports.py keeps one declaration
	// that is none and stays whole with it. levels.py's constants are asked
	// with the function of models.py that reads them.
	exports := "src/fixture_app/exports.py"
	format, render := split.Symbols[[2]string{exports, "format_score"}], split.Symbols[[2]string{exports, "render_level"}]
	if !split.Helpers[[2]string{exports, "format_score"}] || split.Helpers[[2]string{exports, "render_level"}] || split.PartOf[format] == "" || split.PartOf[format] != split.PartOf[render] || !recorded(split, "role_not_split", exports) {
		t.Fatalf("format_score in %q, render_level in %q", split.PartOf[format], split.PartOf[render])
	}
	for _, name := range []string{"READ_VALUES", "READ_LIMIT"} {
		if got := split.HelperItems[[2]string{"src/fixture_app/levels.py", name}]["read_by"]; !slices.Contains(anyStrings(got), "src/fixture_app/models.py:read_level_data") {
			t.Fatalf("%s is asked with read_by %v", name, got)
		}
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
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index})
	// The reader counts code lines outside the docstring, the ;; comment and
	// the blank line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "src/example/core.clj", map[string][]int{
		"example.core/shout": {1, 4}, "example.core/loud-greeting": {2},
	})
	// The graph records what a function reads in another namespace and the
	// function it hands to clojure.core/map.
	adaptertest.AssertDeclarationUses(t, graph,
		adaptertest.DeclarationUse{FromPath: "src/example/core.clj", From: "example.core/read-limit", Kind: "reads", ToPath: "src/example/service.cljc", To: "example.service/source-limit"},
		adaptertest.DeclarationUse{FromPath: "src/example/core.clj", From: "example.core/greet-many", Kind: "passes_callback", ToPath: "src/example/service.cljc", To: "example.service/greet"},
	)
	meta := reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "executable", Name: index.Target.Name, Root: "."}
	partstest.Check(t, graph, meta, root)
	// Split, core.clj is the seed file: its -main keeps the entry.
	split := partstest.CheckSplit(t, graph, meta, root)
	if !split.Split["src/example/core.clj"] {
		t.Fatal("split: core.clj was not split")
	}
	// The fixture registers no route or command in a split file; its one
	// registration there hands a function to clojure.core/map.
	if registered := split.Registered["src/example/service.cljc"]; !slices.Contains(registered, "clojure.core/map") {
		t.Fatalf("service.cljc's declarations are asked with registrations %v", registered)
	}
	// The helper question: the private exclaim, which only cheer calls, is a
	// helper and goes with cheer by code; cheer, public and called by
	// nothing, is not asked. The items carry what reads and what hands over:
	// read-limit reads source-limit, greet-many hands greet to map. A macro's
	// uses leave no relation (CLOJURE), so a macro nothing else uses is no
	// helper by code.
	core := "src/example/core.clj"
	exclaim, cheer := split.Symbols[[2]string{core, "example.core/exclaim"}], split.Symbols[[2]string{core, "example.core/cheer"}]
	if !split.Helpers[[2]string{core, "example.core/exclaim"}] || split.PartOf[exclaim] == "" || split.PartOf[exclaim] != split.PartOf[cheer] || !recorded(split, "role_attached", "example.core/exclaim") {
		t.Fatalf("exclaim in %q, cheer in %q", split.PartOf[exclaim], split.PartOf[cheer])
	}
	if _, asked := split.HelperItems[[2]string{core, "example.core/cheer"}]; asked {
		t.Fatal("cheer, which nothing calls, was asked the helper question")
	}
	service := "src/example/service.cljc"
	if got := split.HelperItems[[2]string{service, "example.service/source-limit"}]["read_by"]; !slices.Contains(anyStrings(got), core+":example.core/read-limit") {
		t.Fatalf("source-limit is asked with read_by %v", got)
	}
	if got := split.HelperItems[[2]string{service, "example.service/greet"}]["handed_over_by"]; !slices.Contains(anyStrings(got), core+":example.core/greet-many") {
		t.Fatalf("greet is asked with handed_over_by %v", got)
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
	graph := graphWithFacts(t, fixture.repository, places.TargetInput{Index: index, Root: "."})
	// The lexer skips the comment inside bgsaveCommand; strings holding
	// "//" or "/*" stay code.
	adaptertest.AssertDeclarationCodeLines(t, graph, "kvd.c", map[string][]int{
		"bgsaveCommand": {17}, "processCommand": {13},
	})
	// The graph records what a declaration reads or hands over: printSymbols
	// reads staticsyms.h's symsTable, as redis.c's findFuncName does;
	// keysCommand hands compareKeys to qsort; the command table's rows hand
	// getCommand over.
	adaptertest.AssertDeclarationUses(t, graph,
		adaptertest.DeclarationUse{FromPath: "kvd.c", From: "printSymbols", Kind: "reads", ToPath: "staticsyms.h", To: "symsTable"},
		adaptertest.DeclarationUse{FromPath: "kvd.c", From: "keysCommand", Kind: "passes_callback", ToPath: "kvd.c", To: "compareKeys"},
		adaptertest.DeclarationUse{FromPath: "kvd.c", From: "cmdTable", Kind: "passes_callback", ToPath: "kvd.c", To: "getCommand"},
	)
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}, fixture.root)
	for _, declaration := range [][2]string{{"kvd.c", "main"}, {"loop_poll.c", "loopApiPoll"}, {"strbuf.h", "sbAvail"}, {"kvd.h", "kvClient"}} {
		if checked.Symbols[declaration] == "" {
			t.Fatalf("%s %s is not on the map", declaration[0], declaration[1])
		}
	}
	// Split, kvd.c is the seed file, as redis.c is redis-server's: the part
	// holding main stands in the "in" column (CheckSplit).
	split := partstest.CheckSplit(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}, fixture.root)
	main := split.Symbols[[2]string{"kvd.c", "main"}]
	if !split.Split["kvd.c"] || split.PartOf[main] == "" {
		t.Fatalf("split: main in %q", split.PartOf[main])
	}
	// A command handler's assignment shows its command table row's words:
	// getCommand is "kvCommand get", not a command lookup.
	if registered := split.Registered["kvd.c"]; !slices.Contains(registered, "kvCommand get") {
		t.Fatalf("kvd.c's handlers are asked with registrations %v", registered)
	}
	// The helper question: saveSnapshot, static and called only by
	// bgsaveCommand, goes with it by code. staticsyms.h's symsTable, which
	// only printSymbols reads, is a helper, so the header keeps one
	// declaration that is none and stays whole. That one is its type
	// kvSymbol, which the check does not take for a helper (a type has no
	// use facts), so the header is no file of helpers and keeps a part of
	// its own, although all it shows other files is a helper used from
	// printSymbols's box alone: rule B joins a whole file only when every
	// declaration of it is a helper (redis's staticsymbols.h holds only its
	// table, its struct being declared in redis.c). addReplyBulk and
	// addReplyLong, whose callers stand in both boxes, are asked once more.
	snapshot, bgsave := split.Symbols[[2]string{"kvd.c", "saveSnapshot"}], split.Symbols[[2]string{"kvd.c", "bgsaveCommand"}]
	if split.PartOf[snapshot] == "" || split.PartOf[snapshot] != split.PartOf[bgsave] || !recorded(split, "role_attached", "saveSnapshot") {
		t.Fatalf("saveSnapshot in %q, bgsaveCommand in %q", split.PartOf[snapshot], split.PartOf[bgsave])
	}
	printer := split.PartOf[split.Symbols[[2]string{"kvd.c", "printSymbols"}]]
	header := split.PartOf[split.Symbols[[2]string{"staticsyms.h", "kvSymbol"}]]
	if table := split.PartOf[split.Symbols[[2]string{"staticsyms.h", "symsTable"}]]; !split.Helpers[[2]string{"staticsyms.h", "symsTable"}] || header == "" || table != header || header == printer {
		t.Fatalf("staticsyms.h's symsTable is in %q, kvSymbol in %q, printSymbols in %q", table, header, printer)
	}
	if recorded(split, "role_attached", "staticsyms.h") {
		t.Fatal("the header, which declares a type that is no helper, is recorded as joined")
	}
	if got := split.HelperItems[[2]string{"staticsyms.h", "symsTable"}]["read_by"]; !reflect.DeepEqual(got, []any{"kvd.c:printSymbols"}) {
		t.Fatalf("symsTable is asked with read_by %v", got)
	}
	for _, name := range []string{"addReplyBulk", "addReplyLong"} {
		if !recorded(split, "role_second_pass", name) {
			t.Fatalf("%s was not asked once more", name)
		}
	}
	// The split puts netConnect, which the server never runs, alone in a
	// role part of net.c: that part leaves the server's map and is listed
	// by its declaration.
	netConnect := cObject(t, index, programindex.ObjectFunction, "netConnect", "net.c")
	listed := false
	for _, file := range projectSplit(t, index, split).OffMap {
		listed = listed || file.Path == "net.c" && file.Reason == groupindex.OffMapUnreachable && slices.Equal(file.SubjectIDs, []string{netConnect.ID})
	}
	if !listed {
		t.Fatal("the role part of net.c the server never runs is not listed off its map")
	}
}

// kvcli links the server's event loop, as redis-cli links adlist.o, and
// never runs it. Drawn one part per file, loop.c and the poll backend it
// includes hold nothing the client runs: those parts leave its map and are
// listed off it by their declarations, as a part made only of test code is.
// loop.h's types run nothing of their own and keep their part; net.c, whose
// netListen the client never runs, keeps its part for netConnect.
func TestCFixtureClientMapLeavesTheLoopItNeverRuns(t *testing.T) {
	fixture := loadCFixture(t)
	index := buildCIndex(t, fixture, "c:kvcli")
	graph := graphWithFacts(t, fixture.repository, places.TargetInput{Index: index, Root: "."})
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}, fixture.root)
	var unreached, drawn []string
	for _, box := range checked.Target.Boxes {
		if box.Unreached {
			unreached = append(unreached, box.Title)
		} else {
			drawn = append(drawn, box.Title)
		}
		if box.Unreached && box.Line != "" {
			t.Fatalf("the part %q the client never runs was described: %q", box.Title, box.Line)
		}
	}
	if want := []string{"loop.c", "loop_poll.c"}; !slices.Equal(unreached, want) {
		t.Fatalf("parts the client never runs = %v, want %v (drawn: %v)", unreached, want, drawn)
	}
	for _, title := range []string{"kvcli.c", "loop.h", "net.c", "strbuf.c"} {
		if !slices.Contains(drawn, title) {
			t.Fatalf("%s is not drawn on the client's map: %v", title, drawn)
		}
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, checked.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	checkUnreachedParts(t, index, indexes[0])
	listed := map[string]string{}
	for _, file := range indexes[0].OffMap {
		if file.Reason == groupindex.OffMapUnreachable {
			listed[file.Path] = file.Part
			if file.Path == "loop.c" && !slices.Contains(file.SubjectIDs, cObject(t, index, programindex.ObjectFunction, "loopMain", "loop.c").ID) {
				t.Fatalf("loop.c's row does not list loopMain: %+v", file)
			}
		}
	}
	if want := map[string]string{"loop.c": "loop.c", "loop_poll.c": "loop_poll.c"}; !maps.Equal(listed, want) {
		t.Fatalf("listed as never run = %v, want %v", listed, want)
	}
}

// anyStrings reads a JSON list of strings.
func anyStrings(value any) []string {
	list, _ := value.([]any)
	var result []string
	for _, item := range list {
		result = append(result, fmt.Sprint(item))
	}
	return result
}
