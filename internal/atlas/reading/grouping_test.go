package reading

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// roleDecl is one declaration of a grouping test graph.
type roleDecl struct {
	name, kind      string
	line, end, code int
	macro, private  bool
	calls           []string // "path:name" of exact callees
	uses            []string // "kind path:name" of exact reads, hand-overs and decorators
}

// unitGraph is the graph of one target "svc" holding every file: a file
// ending in _test.go is test code, one ending in gen.go generated. A type's
// methods are the declarations named "Type.name", wherever declared. extra
// adds boundary places.
func unitGraph(t *testing.T, files map[string][]roleDecl, extra ...atlas.Place) atlas.Graph {
	t.Helper()
	lineOf := map[string]int{}
	for path, decls := range files {
		for _, decl := range decls {
			lineOf[path+":"+decl.name] = decl.line
		}
	}
	paths := slices.Sorted(maps.Keys(files))
	target := []string{"svc"}
	dirs := map[string][]string{}
	var places []atlas.Place
	next := 0
	for _, path := range paths {
		dir := filepath.Dir(path)
		dirs[dir] = append(dirs[dir], filepath.Base(path))
		facts := &atlas.FileFacts{Test: strings.HasSuffix(path, "_test.go"), Generated: strings.HasSuffix(path, "gen.go")}
		var symbols []atlas.Place
		for _, spec := range files[path] {
			next++
			decl := atlas.Decl{ObjectID: fmt.Sprintf("n%d", next), Name: spec.name, Kind: spec.kind, Signature: "func()", LineNo: spec.line, EndLine: spec.end,
				CodeLines: spec.code, Exported: !spec.private, Macro: spec.macro}
			facts.Decls = append(facts.Decls, decl)
			symbol := &atlas.SymbolFacts{Decl: decl}
			for _, callee := range spec.calls {
				calleePath, name, _ := strings.Cut(callee, ":")
				symbol.Calls = append(symbol.Calls, atlas.SymbolCall{Kind: "calls", Name: name, Resolution: "exact", CalleeIDs: []string{atlas.SymbolID(calleePath, lineOf[callee], name)}})
			}
			for _, use := range spec.uses {
				kind, used, _ := strings.Cut(use, " ")
				usedPath, name, _ := strings.Cut(used, ":")
				symbol.Uses = append(symbol.Uses, atlas.SymbolUse{PlaceID: atlas.SymbolID(usedPath, lineOf[used], name), Kind: kind, Resolution: "exact"})
			}
			if spec.kind == "type" {
				for _, methodPath := range paths {
					for _, method := range files[methodPath] {
						if strings.HasPrefix(method.name, spec.name+".") {
							symbol.Members = append(symbol.Members, atlas.TypeMember{Path: methodPath, Decl: atlas.Decl{Name: method.name, Kind: "method", LineNo: method.line}})
						}
					}
				}
			}
			symbols = append(symbols, atlas.Place{ID: atlas.SymbolID(path, spec.line, spec.name), Kind: atlas.PlaceSymbol, Path: path, LineNo: spec.line,
				Parent: atlas.FileID(path), TargetIDs: target, Given: spec.name, Symbol: symbol})
		}
		places = append(places, atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, Depth: strings.Count(path, "/"), Parent: atlas.DirectoryID(dir),
			TargetIDs: target, Given: "given " + path, File: facts})
		places = append(places, symbols...)
	}
	for dir, names := range dirs {
		places = append(places, atlas.Place{ID: atlas.DirectoryID(dir), Kind: atlas.PlaceDirectory, Path: dir, Depth: strings.Count(dir, "/"),
			TargetIDs: target, Given: dir + " files", Directory: &atlas.DirectoryFacts{Files: names, FileCount: len(names), TopBox: true}})
	}
	graph := atlas.Graph{Version: atlas.GraphVersion, Revision: "abc", Places: append(places, extra...), Edges: []atlas.Edge{}}
	atlas.SortPlaces(graph.Places)
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// registration hands a declaration of svc/api/routes.go over with words.
func registration(line int, name string, words ...string) atlas.Place {
	return atlas.Place{ID: fmt.Sprintf("bnd:svc/api/routes.go:%d:route", line), Kind: atlas.PlaceBoundary, Path: "svc/api/routes.go", LineNo: line,
		Parent: atlas.FileID("svc/api/routes.go"), TargetIDs: []string{"svc"}, Given: strings.Join(words, " "),
		Boundary: &atlas.BoundaryFacts{Source: "fact", Direction: atlas.DirectionIn, Words: words, SubjectID: atlas.SymbolID("svc/api/routes.go", lineOf(name), name)}}
}

func lineOf(name string) int {
	for _, decls := range serviceFiles() {
		for _, decl := range decls {
			if decl.name == name {
				return decl.line
			}
		}
	}
	return 0
}

// serviceFiles is a small service: a web layer of five declarations in two
// files, a store of five (a type with two methods, one in its own file),
// a logger with a private formatter only it calls, and a test.
func serviceFiles() map[string][]roleDecl {
	return map[string][]roleDecl{
		"svc/api/routes.go": {
			{name: "Serve", kind: "function", line: 3, end: 8, code: 5, calls: []string{"svc/api/routes.go:Route"}},
			{name: "Route", kind: "function", line: 10, end: 20, code: 9, calls: []string{"svc/api/routes.go:Handle", "svc/store/db.go:Store.Get"}},
			{name: "Handle", kind: "function", line: 22, end: 30, code: 7},
		},
		"svc/api/auth.go": {
			{name: "Login", kind: "function", line: 3, end: 9, code: 6, calls: []string{"svc/store/db.go:Open"}},
			{name: "Logout", kind: "function", line: 11, end: 13, code: 2},
		},
		"svc/store/db.go": {
			{name: "Store", kind: "type", line: 3, end: 8, code: 5},
			{name: "Store.Get", kind: "method", line: 10, end: 14, code: 4},
			{name: "Open", kind: "function", line: 16, end: 20, code: 4},
			{name: "Migrate", kind: "function", line: 22, end: 30, code: 8},
		},
		"svc/store/put.go": {
			{name: "Store.Put", kind: "method", line: 3, end: 9, code: 6},
		},
		"svc/store/cache.go": {
			{name: "Cache", kind: "type", line: 3, end: 9, code: 6},
			{name: "Evict", kind: "function", line: 11, end: 15, code: 4},
		},
		"svc/util/log.go": {
			{name: "Log", kind: "function", line: 3, end: 6, code: 3, calls: []string{"svc/util/log.go:format"}},
			{name: "format", kind: "function", line: 8, end: 10, code: 2, private: true},
		},
		"svc/svc_test.go": {
			{name: "TestServe", kind: "function", line: 3, end: 5, code: 2, calls: []string{"svc/api/routes.go:Serve"}},
		},
	}
}

// groupJev decides the grouping's questions as a test sets them and keeps
// what it was asked: the boxes asked whether they need smaller boxes, the
// helper and assignment items by declaration name.
type groupJev struct {
	typesafetest.Categorizer
	mu sync.Mutex
	// helper answers by declaration name. tie leaves a declaration a
	// near-tie when the box it names is among the options. into answers
	// whole boxes by the options offered: the first listed option present
	// takes every declaration.
	helper      map[string]llm.Verdict
	tie         map[string]string
	nearTie     map[string]string
	into        []string
	enough      []string
	askedBoxes  []string
	declared    map[string][]string
	helperItems map[string]map[string]any
	boxItems    map[string][]map[string]any
}

func newGroupJev() *groupJev {
	jev := &groupJev{helper: map[string]llm.Verdict{}, tie: map[string]string{}, nearTie: map[string]string{}, declared: map[string][]string{},
		helperItems: map[string]map[string]any{}, boxItems: map[string][]map[string]any{}}
	closed := closedDecisions().Decide
	jev.Decide = func(key string, question llm.Question) (llm.Verdict, bool) {
		jev.mu.Lock()
		defer jev.mu.Unlock()
		name, _ := question.Item["name"].(string)
		switch key[strings.LastIndex(key, "|")+1:] {
		case "helper":
			jev.helperItems[name] = question.Item
			if verdict, ok := jev.helper[name]; ok {
				return verdict, true
			}
		case "grouping":
			// The item is the box itself: its name and declarations.
			box := question.Item
			name, _ = box["name"].(string)
			jev.askedBoxes = append(jev.askedBoxes, name)
			declarations, _ := box["declarations"].([]any)
			for _, declaration := range declarations {
				jev.declared[name] = append(jev.declared[name], fmt.Sprint(declaration))
			}
			if slices.Contains(jev.enough, name) {
				return typesafetest.Choose(lines.GroupEnough), true
			}
		case "box":
			jev.boxItems[name] = append(jev.boxItems[name], question.Item)
			for _, into := range jev.into {
				for _, option := range question.Options {
					if option.Name == into {
						return typesafetest.Choose(into), true
					}
				}
			}
			if among, ok := jev.tie[name]; ok && slices.ContainsFunc(question.Options, func(option llm.Option) bool { return option.Name == among }) {
				// No answer for the row: it stays undecided.
				return llm.Verdict{}, true
			}
			if lead, ok := jev.nearTie[name]; ok && slices.ContainsFunc(question.Options, func(option llm.Option) bool { return option.Name == lead }) {
				other := question.Options[0].Name
				if other == lead {
					other = question.Options[1].Name
				}
				return llm.Verdict{Choice: lead, Probabilities: map[string]float64{lead: 0.52, other: 0.48}}, true
			}
		}
		return closed(key, question)
	}
	return jev
}

// serviceBoxes divides the target by layer, the web layer by file, and the
// store by file too.
func serviceBoxes(box, file string) string {
	switch {
	case box == "example.com/svc" && strings.HasPrefix(file, "svc/api/"):
		return "Web"
	case box == "example.com/svc" && strings.HasPrefix(file, "svc/store/"):
		return "Storage"
	case box == "example.com/svc":
		return "Logging"
	case strings.HasSuffix(file, "routes.go"):
		return "Routing"
	case strings.HasSuffix(file, "auth.go"):
		return "Auth"
	case strings.HasSuffix(file, "cache.go"):
		return "Caching"
	}
	return "Database"
}

func serviceOptions(t *testing.T, graph atlas.Graph, provider *tableProvider, jev *groupJev) Options {
	t.Helper()
	opts := readOptions(t, graph, provider, "")
	opts.Targets = []TargetMeta{{ID: "svc", Language: "go", Kind: "executable", Name: "example.com/svc", Root: "svc"}}
	opts.Categorizer = jev
	return opts
}

func partOf(target atlas.Target) map[string]atlas.Box {
	parts := map[string]atlas.Box{}
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				parts[symbol.Name] = box
			}
		}
	}
	return parts
}

func membersOf(box atlas.Box) []string {
	var names []string
	for _, file := range box.Files {
		for _, symbol := range file.Symbols {
			names = append(names, symbol.Name)
		}
	}
	slices.Sort(names)
	return names
}

// The grouping divides the target into boxes, and each box that needs it
// again: the web layer becomes an area of its files' parts. A box of at most
// four declarations is not asked (the store's Caching and the web's Auth
// never are); a box whose declarations all went in one smaller box is read
// as it is (Storage, which Jev put wholly in Database). A near-tie takes
// Jev's leading box (Login, led into Routing). A declaration with no answer
// goes where its decided call neighbours went (Handle, called by Route), and
// one with none is a box of its own, recorded (Logout). The
// helper format is never grouped and goes with Log, its one user. Methods
// follow their type across files; the test is one part off the canvas.
func TestTheGroupingRecursesIntoAreasAndParts(t *testing.T) {
	graph := unitGraph(t, serviceFiles())
	provider := &tableProvider{groupFor: serviceBoxes}
	jev := newGroupJev()
	jev.helper["format"] = typesafetest.Choose(lines.RoleHelperHelper)
	jev.tie["Handle"], jev.tie["Logout"] = "Routing", "Routing"
	jev.nearTie["Login"] = "Routing"
	jev.into = []string{"Database"}
	result, err := Read(t.Context(), serviceOptions(t, graph, provider, jev))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	target := result.Atlas.Targets[0]
	if !slices.Equal(jev.askedBoxes, []string{"example.com/svc", "Web", "Storage"}) {
		t.Fatalf("asked boxes %v; want the target, Web and Storage, the boxes of more than four declarations", jev.askedBoxes)
	}
	for _, declaration := range jev.declared["example.com/svc"] {
		if strings.HasSuffix(declaration, ": format") || strings.Contains(declaration, "TestServe") || strings.Contains(declaration, "Store.") {
			t.Fatalf("the target's box lists %s: no helper, test code or method is an element", declaration)
		}
	}
	if len(jev.boxItems["format"]) != 0 || len(jev.boxItems["TestServe"]) != 0 {
		t.Fatal("a helper or test code was assigned a box")
	}
	parts := partOf(target)
	want := map[string][]string{
		"Routing": {"Handle", "Login", "Route", "Serve"},
		"Logout":  {"Logout"},
		"Storage": {"Cache", "Evict", "Migrate", "Open", "Store", "Store.Get", "Store.Put"},
		"Logging": {"Log", "format"},
		"Tests":   {"TestServe"},
	}
	got := map[string][]string{}
	for _, box := range target.Boxes {
		got[box.Title] = membersOf(box)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parts %v, want %v", got, want)
	}
	if !parts["TestServe"].ForTests || parts["Route"].ForTests {
		t.Fatal("the test part is not the off-canvas one")
	}
	if len(target.Zones) != 1 || target.Zones[0].Title != "Web" || target.Zones[0].ParentID != "" {
		t.Fatalf("areas %+v; want Web alone", target.Zones)
	}
	var inWeb []string
	for _, id := range target.Zones[0].BoxIDs {
		for _, box := range target.Boxes {
			if box.ID == id {
				inWeb = append(inWeb, box.Title)
			}
			if box.ZoneID == target.Zones[0].ID && !slices.Contains(target.Zones[0].BoxIDs, box.ID) {
				t.Fatalf("%s names the area that does not list it", box.Title)
			}
		}
	}
	slices.Sort(inWeb)
	if !slices.Equal(inWeb, []string{"Logout", "Routing"}) {
		t.Fatalf("the Web area holds %v", inWeb)
	}
	// A proposed box keeps the proposal's holds; the undecided box of its own
	// has none and is described.
	if !strings.HasPrefix(parts["Route"].Line, "Holds ") || parts["Logout"].Line != "About Logout." {
		t.Fatalf("lines: Routing %q, Logout %q", parts["Route"].Line, parts["Logout"].Line)
	}
	if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool {
		return row.Kind == "group_undecided" && slices.Equal(row.Samples, []string{"svc/api/auth.go:Logout"})
	}) {
		t.Fatalf("the undecided Logout is not recorded: %+v", result.Rejected)
	}
	var marks []string
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				if symbol.Helper {
					marks = append(marks, symbol.Name)
				}
			}
		}
	}
	if !slices.Equal(marks, []string{"format"}) {
		t.Fatalf("helper marks %v", marks)
	}
}

// A box the grouping answers is grouped enough, and one whose proposal
// names fewer than two boxes, are read as they are: one part, no area.
func TestABoxReadAsItIsIsOnePart(t *testing.T) {
	for name, setup := range map[string]func(*tableProvider, *groupJev){
		"grouped enough": func(_ *tableProvider, jev *groupJev) { jev.enough = []string{"example.com/svc"} },
		"one proposed box": func(provider *tableProvider, _ *groupJev) {
			provider.groupFor = func(string, string) string { return "Everything" }
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider, jev := &tableProvider{groupFor: serviceBoxes}, newGroupJev()
			setup(provider, jev)
			result, err := Read(t.Context(), serviceOptions(t, unitGraph(t, serviceFiles()), provider, jev))
			if err != nil {
				t.Fatal(err)
			}
			target := result.Atlas.Targets[0]
			var titles []string
			for _, box := range target.Boxes {
				titles = append(titles, box.Title)
			}
			if !slices.Equal(titles, []string{"example.com/svc", "Tests"}) || len(target.Zones) != 0 || len(jev.boxItems) != 0 {
				t.Fatalf("parts %v, areas %+v, assigned %d", titles, target.Zones, len(jev.boxItems))
			}
		})
	}
}

// The assignment item is the helper question's: it carries a declaration's
// registrations, so a handler goes with the work its route names.
func TestTheAssignmentShowsTheRegistrationsOfADeclaration(t *testing.T) {
	graph := unitGraph(t, serviceFiles(), registration(12, "Handle", "HandleFunc", "/orders"))
	provider, jev := &tableProvider{groupFor: serviceBoxes}, newGroupJev()
	if _, err := Read(t.Context(), serviceOptions(t, graph, provider, jev)); err != nil {
		t.Fatal(err)
	}
	if len(jev.boxItems["Handle"]) == 0 {
		t.Fatal("Handle was never assigned")
	}
	for _, item := range jev.boxItems["Handle"] {
		if !reflect.DeepEqual(item["registered"], []any{"HandleFunc /orders"}) {
			t.Fatalf("Handle is assigned as %v", item)
		}
	}
}

// The helper item carries what calls, reads and hands a declaration over,
// and its registrations; a declaration nothing in the program uses, test
// and generated code are not asked.
func TestTheHelperItemCarriesItsUsers(t *testing.T) {
	files := serviceFiles()
	files["svc/store/db.go"][1].uses = []string{"reads svc/store/db.go:limits"}
	files["svc/store/db.go"] = append(files["svc/store/db.go"],
		roleDecl{name: "limits", kind: "variable", line: 32, end: 32, code: 1},
		roleDecl{name: "routes", kind: "variable", line: 34, end: 34, code: 1, uses: []string{"passes_callback svc/store/db.go:handlePing"}},
		roleDecl{name: "handlePing", kind: "function", line: 36, end: 38, code: 3},
	)
	files["svc/gen.go"] = []roleDecl{{name: "Gen1", kind: "function", line: 3, end: 5, code: 2, calls: []string{"svc/api/routes.go:Route"}}}
	graph := unitGraph(t, files, registration(12, "Route", "HandleFunc", "/route"))
	provider, jev := &tableProvider{groupFor: serviceBoxes}, newGroupJev()
	opts := serviceOptions(t, graph, provider, jev)
	opts.Targets[0].Language = "c"
	if _, err := Read(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]map[string]any{
		"limits":     {"name": "limits", "kind": "variable", "file": "svc/store/db.go", "signature": "func()", "lines": 1.0, "read_by": []any{"svc/store/db.go:Store"}},
		"handlePing": {"name": "handlePing", "kind": "function", "file": "svc/store/db.go", "signature": "func()", "lines": 3.0, "handed_over_by": []any{"svc/store/db.go:routes"}},
		"Route": {"name": "Route", "kind": "function", "file": "svc/api/routes.go", "signature": "func()", "lines": 9.0,
			"calls": []any{"svc/api/routes.go:Handle", "svc/store/db.go:Store"}, "called_by": []any{"svc/api/routes.go:Serve"}, "registered": []any{"HandleFunc /route"}},
		"Store": {"name": "Store", "kind": "type", "file": "svc/store/db.go", "signature": "func()", "lines": 15.0, "methods": []any{"Get func()", "Put func()"},
			"called_by": []any{"svc/api/routes.go:Route"}},
	} {
		if got := jev.helperItems[name]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s is asked as %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{"Serve", "Migrate", "TestServe", "Gen1", "routes"} {
		if _, asked := jev.helperItems[name]; asked {
			t.Fatalf("%s, which nothing in the program uses or which is test or generated code, was asked", name)
		}
	}
}

// No recorded use is no proof of none where the adapter records no use of
// such a declaration (recordedUses): a Go variable nothing reads is asked,
// since Go records no reads (GO), and so is a Clojure macro nothing uses,
// since Clojure records no use of a macro (CLOJURE). C records a
// variable's reads, so there a variable nothing reads is no helper by code;
// it has no macro declarations, so it states no uses of one. A function
// nothing calls is asked in none of them.
func TestAKindWithNoRecordedUsesIsAsked(t *testing.T) {
	files := serviceFiles()
	files["svc/store/db.go"] = append(files["svc/store/db.go"],
		roleDecl{name: "routes", kind: "variable", line: 32, end: 32, code: 1},
		roleDecl{name: "ensure", kind: "function", line: 34, end: 36, code: 2, macro: true},
	)
	graph := unitGraph(t, files)
	for language, want := range map[string][]string{"go": {"routes", "ensure"}, "clojure": {"ensure"}, "c": {"ensure"}} {
		provider, jev := &tableProvider{groupFor: serviceBoxes}, newGroupJev()
		opts := serviceOptions(t, graph, provider, jev)
		opts.Targets[0].Language = language
		if _, err := Read(t.Context(), opts); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"routes", "ensure"} {
			if _, asked := jev.helperItems[name]; asked != slices.Contains(want, name) {
				t.Fatalf("%s: %s asked %v; want only %v, the kinds with no recorded uses, asked", language, name, asked, want)
			}
		}
		for _, name := range []string{"Serve", "Migrate", "Evict"} {
			if _, asked := jev.helperItems[name]; asked {
				t.Fatalf("%s: %s, a function nothing calls, was asked", language, name)
			}
		}
	}
}

// Only a decided helper is one: a near-tie leaves the declaration an
// element, grouped and assigned like any other, with no helper mark.
func TestAnUncertainHelperAnswerIsNoHelper(t *testing.T) {
	provider, jev := &tableProvider{groupFor: serviceBoxes}, newGroupJev()
	jev.helper["format"] = llm.Verdict{Choice: lines.RoleHelperHelper, Probabilities: map[string]float64{lines.RoleHelperHelper: 0.52, lines.RoleHelperOwnJob: 0.48}}
	result, err := Read(t.Context(), serviceOptions(t, unitGraph(t, serviceFiles()), provider, jev))
	if err != nil {
		t.Fatal(err)
	}
	if _, asked := jev.helperItems["format"]; !asked || len(jev.boxItems["format"]) == 0 {
		t.Fatal("an uncertain helper was not asked, or not assigned a box")
	}
	for _, box := range result.Atlas.Targets[0].Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				if symbol.Helper {
					t.Fatalf("%s carries the helper mark", symbol.Name)
				}
			}
		}
	}
}

// The proposal request sends each file with the names of its elements, and
// its box with the boxes it sits in; no helper, test code or documentation.
func TestTheProposalSendsNamesByFile(t *testing.T) {
	provider, jev := &tableProvider{groupFor: serviceBoxes}, newGroupJev()
	jev.helper["format"] = typesafetest.Choose(lines.RoleHelperHelper)
	if _, err := Read(t.Context(), serviceOptions(t, unitGraph(t, serviceFiles()), provider, jev)); err != nil {
		t.Fatal(err)
	}
	var web groupProposeInput
	for _, body := range provider.proposed {
		var input groupProposeInput
		if err := json.Unmarshal(body, &input); err != nil {
			t.Fatal(err)
		}
		if input.Task != groupProposeTask {
			t.Fatalf("task %q", input.Task)
		}
		if input.Box.Name == "example.com/svc" {
			for _, file := range input.Files {
				if slices.Contains(file.Names, "format") || strings.HasSuffix(file.Path, "_test.go") {
					t.Fatalf("the target's proposal lists %v", file)
				}
			}
		}
		if input.Box.Name == "Web" {
			web = input
		}
	}
	want := groupProposeInput{Task: groupProposeTask, Box: groupProposeBox{Name: "Web", Holds: "Holds svc/api/auth.go, svc/api/routes.go."},
		Files: []groupFileNames{{Path: "svc/api/auth.go", Names: []string{"Login", "Logout"}}, {Path: "svc/api/routes.go", Names: []string{"Serve", "Route", "Handle"}}}}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("the Web proposal is %+v, want %+v", web, want)
	}
}

func TestDecodeBoxesRefusesOnlyWhatIsWrong(t *testing.T) {
	answer, err := decodeBoxes([]byte(`{"result":{"Boxes":[{"Name":" Entry ","holds":"Start."},{"name":"Entry","holds":"Start."},{"name":"entry","holds":"Serve."},{"name":"Store"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Boxes) != 2 || answer.Boxes[0].Name != "Entry" || answer.Boxes[1].Holds != "Serve." || !slices.Equal(answer.Repeated, []string{"entry"}) || !slices.Equal(answer.Incomplete, []string{"box 4"}) {
		t.Fatalf("decoded %+v", answer)
	}
	if answer, err := decodeBoxes([]byte(`{"boxes":null}`)); err != nil || len(answer.Boxes) != 0 {
		t.Fatalf("null: %+v %v", answer, err)
	}
	for _, raw := range []string{`not json`, `{"groups":[]}`, `{"boxes":"Entry, Store"}`} {
		if _, err := decodeBoxes([]byte(raw)); err == nil {
			t.Fatalf("%s was accepted", raw)
		}
	}
}

// Every request stage of the map of parts is a journal stage: an exchange of
// any other stage is refused as invalid, so the run keeps no record, tokens
// or latency of it.
func TestTheMapOfPartsStagesAreJournaled(t *testing.T) {
	writer, err := debugdump.NewWriter(t.TempDir(), "run")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, stage := range []string{
		lines.StageZones, lines.StagePlacement, lines.StageDescribe, lines.StageAreas,
		lines.StageRoleHelper, lines.StageGroupEnough, lines.StageGroupAssign, lines.StageCore, lines.StageKeys,
	} {
		reference := writer.RecordSemanticExchange(debugdump.SemanticExchange{
			Stage: stage, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1, RequestProvenance: debugdump.SemanticRequestExactSent,
			State: debugdump.SemanticStateAccepted, ValidationCode: debugdump.SemanticValidationAccepted,
			SemanticCalls: 1, TransportAttempts: 1, Request: []byte(`{"stage":"` + stage + `"}`), Response: []byte(`{}`),
		})
		if reference == "" {
			t.Errorf("%s is not a semantic journal stage", stage)
		}
	}
}
