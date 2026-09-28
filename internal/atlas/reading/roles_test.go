package reading

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
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

// roleDecl is one declaration of the role split's test graph.
type roleDecl struct {
	name, kind      string
	line, end, code int
	calls           []string // "path:name" of exact callees
	uses            []string // "kind path:name" of exact reads, hand-overs and decorators
}

// roleGraph is target svc: server.go, the seed file whose code goes in
// several boxes (main, Serve, Route, the Store type with two methods, a
// helper that main and Store.Put call, and a config read with no subject);
// db.go and util.go, which the parts answer places whole; a test file and a
// generated file of two units each, which are never candidates; and target
// one, the single file one/tool.go. db.go imports server.go and its Open
// calls Store.Put. extra may change the declarations before the graph is
// sealed.
func roleGraph(t *testing.T, extra func(files map[string][]roleDecl)) atlas.Graph {
	t.Helper()
	return roleGraphWith(t, extra, nil)
}

// roleGraphWith is roleGraph with more boundary places.
func roleGraphWith(t *testing.T, extra func(files map[string][]roleDecl), boundaries []atlas.Place) atlas.Graph {
	t.Helper()
	files := map[string][]roleDecl{
		"svc/server.go": {
			{name: "main", kind: "function", line: 3, end: 8, code: 5, calls: []string{"svc/server.go:Serve", "svc/server.go:helper"}},
			{name: "Serve", kind: "function", line: 10, end: 20, code: 9, calls: []string{"svc/server.go:Route"}},
			{name: "Route", kind: "function", line: 22, end: 30, code: 7, calls: []string{"svc/server.go:Store.Get"}},
			{name: "Store", kind: "type", line: 32, end: 40, code: 8},
			{name: "Store.Get", kind: "method", line: 42, end: 50, code: 7},
			{name: "Store.Put", kind: "method", line: 52, end: 60, code: 7, calls: []string{"svc/server.go:helper"}},
			{name: "helper", kind: "function", line: 62, end: 64, code: 3},
		},
		"svc/db.go":          {{name: "Open", kind: "function", line: 3, end: 9, code: 6, calls: []string{"svc/server.go:Store.Put"}}, {name: "Close", kind: "function", line: 11, end: 13, code: 3}},
		"svc/util.go":        {{name: "Log", kind: "function", line: 3, end: 5, code: 3}},
		"svc/server_test.go": {{name: "TestServe", kind: "function", line: 3, end: 5, code: 3}, {name: "TestRoute", kind: "function", line: 7, end: 9, code: 3}},
		"svc/gen.go":         {{name: "Gen1", kind: "function", line: 3, end: 5, code: 3}, {name: "Gen2", kind: "function", line: 7, end: 9, code: 3}},
		"one/tool.go":        {{name: "Run", kind: "function", line: 3, end: 9, code: 6}, {name: "Check", kind: "function", line: 11, end: 15, code: 4}},
	}
	if extra != nil {
		extra(files)
	}
	targetOf := func(path string) []string {
		if strings.HasPrefix(path, "one/") {
			return []string{"one"}
		}
		return []string{"svc"}
	}
	dirs := map[string][]string{}
	var places []atlas.Place
	nextObject := 0
	lineOf := map[string]int{}
	for path, decls := range files {
		for _, decl := range decls {
			lineOf[path+":"+decl.name] = decl.line
		}
	}
	for path, decls := range files {
		dir := path[:strings.LastIndex(path, "/")]
		dirs[dir] = append(dirs[dir], path[len(dir)+1:])
		facts := &atlas.FileFacts{Test: strings.HasSuffix(path, "_test.go"), Generated: strings.HasSuffix(path, "gen.go")}
		var symbols []atlas.Place
		for _, spec := range decls {
			nextObject++
			decl := atlas.Decl{ObjectID: fmt.Sprintf("n%d", nextObject), Name: spec.name, Kind: spec.kind, Signature: "func()", LineNo: spec.line, EndLine: spec.end, CodeLines: spec.code, Exported: true}
			facts.Decls = append(facts.Decls, decl)
			symbol := &atlas.SymbolFacts{Decl: decl}
			for _, callee := range spec.calls {
				calleePath, name, _ := strings.Cut(callee, ":")
				symbol.Calls = append(symbol.Calls, atlas.SymbolCall{Kind: "calls", Name: name, Resolution: "exact",
					CalleeIDs: []string{atlas.SymbolID(calleePath, lineOf[callee], name)}})
			}
			for _, use := range spec.uses {
				kind, used, _ := strings.Cut(use, " ")
				usedPath, name, _ := strings.Cut(used, ":")
				symbol.Uses = append(symbol.Uses, atlas.SymbolUse{PlaceID: atlas.SymbolID(usedPath, lineOf[used], name), Kind: kind, Resolution: "exact"})
			}
			if spec.name == "Store" {
				// Store's methods, wherever they are declared.
				paths := slices.Sorted(maps.Keys(files))
				for _, methodPath := range paths {
					for _, method := range files[methodPath] {
						if strings.HasPrefix(method.name, "Store.") {
							symbol.Members = append(symbol.Members, atlas.TypeMember{Path: methodPath, Decl: atlas.Decl{Name: method.name, Kind: "method", LineNo: method.line}})
						}
					}
				}
			}
			symbols = append(symbols, atlas.Place{ID: atlas.SymbolID(path, spec.line, spec.name), Kind: atlas.PlaceSymbol, Path: path, LineNo: spec.line,
				Parent: atlas.FileID(path), TargetIDs: targetOf(path), Given: spec.name, Symbol: symbol})
		}
		places = append(places, atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, Depth: 1, Parent: atlas.DirectoryID(dir),
			TargetIDs: targetOf(path), Given: "given " + path, File: facts})
		places = append(places, symbols...)
	}
	for dir, names := range dirs {
		slices.Sort(names)
		places = append(places, atlas.Place{ID: atlas.DirectoryID(dir), Kind: atlas.PlaceDirectory, Path: dir, Depth: 0,
			TargetIDs: targetOf(dir + "/"), Given: dir + " files", Directory: &atlas.DirectoryFacts{Files: names, FileCount: len(names), TopBox: true}})
	}
	places = append(places, atlas.Place{ID: "bnd:svc/server.go:5:config", Kind: atlas.PlaceBoundary, Path: "svc/server.go", LineNo: 5,
		Parent: atlas.FileID("svc/server.go"), TargetIDs: []string{"svc"}, Given: "config PORT",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryConfig, Values: []string{"PORT"}}})
	places = append(places, boundaries...)
	graph := atlas.Graph{
		Version: atlas.GraphVersion, Revision: "abc", Places: places,
		Edges: []atlas.Edge{{From: atlas.FileID("svc/db.go"), To: atlas.FileID("svc/server.go"), Kind: "imports", Count: 1, Witnesses: []atlas.Witness{}}},
		Seeds: []string{atlas.FileID("svc/server.go")}, SeedDecls: []string{atlas.SymbolID("svc/server.go", 3, "main")},
	}
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

// roleJev answers the role split's questions: the gate by file path, the
// assignment by declaration name to a box title ("" is a near-tie, left
// open), and the other closed tables as closedDecisions does. It keeps every
// gate and assignment item it was asked.
type roleJev struct {
	typesafetest.Categorizer
	mu       sync.Mutex
	gate     map[string]llm.Verdict
	boxOf    map[string]string
	gated    []string
	assigned []string
	// items are the declarations asked by the assignment, by name.
	items map[string]map[string]any
	// helper answers the helper question by declaration name
	// (responsibility when absent); helperAsked keeps every name it was
	// asked about and helperItems their items.
	helper      map[string]llm.Verdict
	helperAsked []string
	helperItems map[string]map[string]any
	// asked are the items of every other closed question, by column.
	asked map[string][]string
}

func newRoleJev(gate map[string]llm.Verdict, boxOf map[string]string) *roleJev {
	jev := &roleJev{gate: gate, boxOf: boxOf, items: map[string]map[string]any{}, helper: map[string]llm.Verdict{}, helperItems: map[string]map[string]any{}}
	closed := closedDecisions().Decide
	jev.Decide = func(key string, question llm.Question) (llm.Verdict, bool) {
		jev.mu.Lock()
		defer jev.mu.Unlock()
		switch {
		case strings.HasSuffix(key, "|helper"):
			name, _ := question.Item["name"].(string)
			jev.helperAsked = append(jev.helperAsked, name)
			jev.helperItems[name] = question.Item
			if verdict, ok := jev.helper[name]; ok {
				return verdict, true
			}
			return typesafetest.Choose(lines.RoleHelperOwnJob), true
		case strings.HasSuffix(key, "|boxes"):
			path, _ := question.Item["path"].(string)
			jev.gated = append(jev.gated, path)
			if verdict, ok := jev.gate[path]; ok {
				return verdict, true
			}
			return typesafetest.Choose(lines.RoleOneBox), true
		case strings.HasSuffix(key, "|box"):
			name, _ := question.Item["declaration"].(string)
			box := jev.boxOf[name]
			jev.assigned = append(jev.assigned, name)
			jev.items[name] = question.Item
			if box == "" {
				first, second := question.Options[0].Name, question.Options[1].Name
				return llm.Verdict{Choice: first, Probabilities: map[string]float64{first: 0.5, second: 0.5}}, true
			}
			return typesafetest.Choose(box), true
		}
		column := key[strings.LastIndex(key, "|")+1:]
		if jev.asked == nil {
			jev.asked = map[string][]string{}
		}
		if part, ok := question.Item["part"].(string); ok {
			jev.asked[column] = append(jev.asked[column], part)
		}
		return closed(key, question)
	}
	return jev
}

func several() llm.Verdict {
	return llm.Verdict{Choice: lines.RoleSeveralBoxes, Probabilities: map[string]float64{lines.RoleSeveralBoxes: 0.9, lines.RoleOneBox: 0.1}}
}

// serverBoxes names server.go's boxes: Entry, Routing, Storage and an
// Unused box no declaration goes in.
const serverBoxes = `{"boxes":[{"name":"Entry","holds":"Starting and serving."},{"name":"Routing","holds":"Choosing a handler."},{"name":"Storage","holds":"The store and its helper."},{"name":"Unused","holds":"Nothing here."}]}`

func roleOptions(t *testing.T, graph atlas.Graph, provider *tableProvider, jev *roleJev, cache string) Options {
	t.Helper()
	opts := readOptions(t, graph, provider, cache)
	opts.Targets = []TargetMeta{
		{ID: "svc", Language: "go", Kind: "executable", Name: "example.com/svc", Root: "svc"},
		{ID: "one", Language: "go", Kind: "executable", Name: "example.com/one", Root: "one"},
	}
	opts.Categorizer = jev
	return opts
}

// defaultRoleProvider names the naming's boxes after serverBoxes (tool.go's
// Running and Checking) and answers the parts request with one part per box
// of a split file, named after the box, and one per whole file.
func defaultRoleProvider() *tableProvider {
	return &tableProvider{
		partFor: func(file map[string]any) string {
			if box, ok := file["box"].(string); ok {
				return box
			}
			switch file["path"] {
			case "svc/server.go":
				return "Server"
			case "svc/db.go":
				return "Database"
			case "svc/util.go":
				return "Utilities"
			}
			return "Other"
		},
		boxesFor: func(path string) string {
			if path == "one/tool.go" {
				return `{"boxes":[{"name":"Running","holds":"Run."},{"name":"Checking","holds":"Check."}]}`
			}
			return serverBoxes
		},
	}
}

func defaultRoleJev() *roleJev {
	return newRoleJev(map[string]llm.Verdict{"svc/server.go": several(), "one/tool.go": several()},
		map[string]string{"main": "Entry", "Serve": "Entry", "Route": "Routing", "Store": "Storage", "helper": "", "Run": "Running", "Check": "Checking"})
}

func partsByTitle(target atlas.Target) map[string]atlas.Box {
	parts := map[string]atlas.Box{}
	for _, box := range target.Boxes {
		parts[box.Title] = box
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

// partsRequest is the parts request whose rows lie under prefix.
func partsRequest(t *testing.T, provider *tableProvider, prefix string) (request struct {
	Units   []map[string]any `json:"units"`
	Calls   []string         `json:"calls"`
	Imports []string         `json:"imports"`
}) {
	t.Helper()
	for _, body := range provider.partsBodies {
		request.Units, request.Calls, request.Imports = nil, nil, nil
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if len(request.Units) > 0 && strings.HasPrefix(fmt.Sprint(request.Units[0]["path"]), prefix) {
			return request
		}
	}
	t.Fatalf("no parts request lists %s", prefix)
	return request
}

// Split files are grouped as units: the role split runs before the parts
// request, which lists each box of server.go that holds a unit as a c* row
// with the box's name, in naming order (the empty Unused box is no row),
// and every other file, db.go and util.go included, as its f* row;
// server.go has no whole row, calls count per site between rows and an
// import into server.go counts for no row. The parts take the answer's
// names and IDs in its order, a part may hold two boxes of one file, and a
// type's methods follow it. helper, whose callers sit in two boxes, is off
// the map as undecided while server.go stays on it. Test, generated and
// one-unit files are no gate candidates; the one-file target whose file
// splits now sends a parts request.
func TestSplitFilesAreGroupedAsUnits(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	provider.partsResponse = func(rows []map[string]any) string {
		ref := map[string]string{}
		for _, row := range rows {
			name := fmt.Sprint(row["path"])
			if box, ok := row["box"].(string); ok {
				name = box
			}
			ref[name] = fmt.Sprint(row["ref"])
		}
		if _, ok := ref["Running"]; ok {
			return fmt.Sprintf(`{"groups":[{"name":"Checking","units":[%q]},{"name":"Running","units":[%q]}]}`, ref["Checking"], ref["Running"])
		}
		return fmt.Sprintf(`{"groups":[{"name":"Storage and data","units":[%q,%q]},{"name":"Serving","units":[%q,%q]},{"name":"Tooling","units":[%q,%q,%q]}]}`,
			ref["Storage"], ref["svc/db.go"], ref["Entry"], ref["Routing"], ref["svc/gen.go"], ref["svc/server_test.go"], ref["svc/util.go"])
	}
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	slices.Sort(jev.gated)
	if !slices.Equal(jev.gated, []string{"one/tool.go", "svc/db.go", "svc/server.go"}) {
		t.Fatalf("gate asked about %v; test, generated and one-unit files are no candidates", jev.gated)
	}
	request := partsRequest(t, provider, "svc/")
	var rows []string
	for _, row := range request.Units {
		rows = append(rows, fmt.Sprintf("%v %v %v", row["ref"], row["path"], row["box"]))
	}
	if want := []string{"f2 svc/db.go <nil>", "f3 svc/gen.go <nil>", "c1 svc/server.go Entry", "c2 svc/server.go Routing", "c3 svc/server.go Storage",
		"f5 svc/server_test.go <nil>", "f6 svc/util.go <nil>"}; !slices.Equal(rows, want) {
		t.Fatalf("svc's parts request rows:\n%v\nwant\n%v", rows, want)
	}
	if !slices.Equal(request.Calls, []string{"c1 -> c2 (1)", "c2 -> c3 (1)", "f2 -> c3 (1)"}) || len(request.Imports) != 0 {
		t.Fatalf("calls %v imports %v: a site counts per row it reaches, an import into a split file counts for none", request.Calls, request.Imports)
	}
	svc := targetOf(t, result, "svc")
	var titles []string
	for _, box := range svc.Boxes {
		titles = append(titles, box.ID+" "+box.Title)
	}
	if want := []string{"p1 Storage and data", "p2 Serving", "p3 Tooling"}; !slices.Equal(titles, want) {
		t.Fatalf("parts %v, want the answer's names in its order", titles)
	}
	parts := partsByTitle(svc)
	if got := membersOf(parts["Storage and data"]); !slices.Equal(got, []string{"Close", "Open", "Store", "Store.Get", "Store.Put"}) {
		t.Fatalf("Storage and data holds %v: a type's methods follow it", got)
	}
	if got := membersOf(parts["Serving"]); !slices.Equal(got, []string{"Route", "Serve", "main"}) {
		t.Fatalf("Serving holds %v", got)
	}
	var undecided []string
	for _, entry := range svc.OffMap {
		if entry.File.Path != "svc/server.go" {
			continue
		}
		if entry.Reason != atlas.OffMapUndecided || entry.BoxID != "" {
			t.Fatalf("server.go off the map: %+v", entry)
		}
		for _, symbol := range entry.File.Symbols {
			undecided = append(undecided, symbol.Name)
		}
	}
	if !slices.Equal(undecided, []string{"helper"}) {
		t.Fatalf("undecided: %v", undecided)
	}
	one := targetOf(t, result, "one")
	if titles := partsByTitle(one); len(one.Boxes) != 2 || titles["Running"].ID == "" || titles["Checking"].ID == "" || len(partsRequest(t, provider, "one/").Units) != 2 {
		t.Fatalf("the one-file target was not grouped by its boxes: %+v", one.Boxes)
	}
	for _, kind := range []string{"role_box_empty", "role_undecided"} {
		if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool { return row.Kind == kind }) {
			t.Fatalf("no %s record", kind)
		}
	}
}

// registration is a request registered in server.go at line, within main's
// source range (3-8), handing the declaration name over with the words the
// code wrote there.
func registration(line int, name string, words ...string) atlas.Place {
	return atlas.Place{ID: fmt.Sprintf("bnd:svc/server.go:%d:register", line), Kind: atlas.PlaceBoundary, Path: "svc/server.go", LineNo: line,
		Parent: atlas.FileID("svc/server.go"), TargetIDs: []string{"svc"}, Given: "registers " + name,
		Boundary: &atlas.BoundaryFacts{Source: "fact", Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryRequest, External: "net/http.HandleFunc", Values: words[1:], Words: words,
			SubjectID: roleSymbol(name)}}
}

// roleSymbol is the symbol place of a declaration of server.go.
func roleSymbol(name string) string {
	lines := map[string]int{"main": 3, "Serve": 10, "Route": 22, "Store": 32, "Store.Get": 42, "Store.Put": 52, "helper": 62}
	return atlas.SymbolID("svc/server.go", lines[name], name)
}

// The assignment shows how the code calls a declaration no declaration
// calls by name: the words of each registration that hands it, or one of
// its methods, over. Route is registered once; Store through its method
// Store.Get, twice with the same words, which it carries once.
func TestTheAssignmentShowsTheRegistrationsOfADeclaration(t *testing.T) {
	graph := roleGraphWith(t, nil, []atlas.Place{
		registration(6, "Route", "HandleFunc", "/route"),
		registration(7, "Store.Get", "HandleFunc", "/get"),
		registration(8, "Store.Get", "HandleFunc", "/get"),
	})
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	if _, err := Read(t.Context(), roleOptions(t, graph, provider, jev, "")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]any{"Route": {"HandleFunc /route"}, "Store": {"HandleFunc /get"}, "main": nil} {
		got, _ := jev.items[name]["registered"].([]any)
		if !slices.Equal(got, want) {
			t.Fatalf("%s is asked with registrations %v, want %v", name, got, want)
		}
	}
}

// withoutMainCallingHelper is roleGraph's server.go where helper's only
// caller is Store.Put.
func withoutMainCallingHelper(files map[string][]roleDecl) {
	files["svc/server.go"][0].calls = []string{"svc/server.go:Serve"}
}

// A unit the assignment leaves open goes, by code and with no second
// question, to the box every unit of its file that uses it went in: its
// callers, what it decorates and what reads it. helper, called only by
// Store.Put, and limits, read only by Store.Get, go in Storage. A unit its
// file does not use goes where everything it uses went: main, which only
// calls Serve, goes in Entry. A hand-over is no use: handlePing, which the
// routes table hands over and Route reads as a function value, stays
// undecided although both sit in Routing. So does a unit whose users sit in
// two boxes: helper, when main calls it too. Each placement is recorded.
func TestAnOpenUnitGoesWhereItsSameFileUsersAre(t *testing.T) {
	graph := roleGraph(t, func(files map[string][]roleDecl) {
		withoutMainCallingHelper(files)
		server := files["svc/server.go"]
		for i := range server {
			switch server[i].name {
			case "Store.Get":
				server[i].uses = []string{"reads svc/server.go:limits"}
			case "Route":
				server[i].uses = []string{"reads svc/server.go:handlePing"}
			}
		}
		files["svc/server.go"] = append(server,
			roleDecl{name: "limits", kind: "variable", line: 66, end: 66, code: 1},
			roleDecl{name: "routes", kind: "variable", line: 68, end: 68, code: 1, uses: []string{"passes_callback svc/server.go:handlePing"}},
			roleDecl{name: "handlePing", kind: "function", line: 70, end: 72, code: 3},
		)
	})
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	jev.boxOf["main"], jev.boxOf["limits"], jev.boxOf["routes"], jev.boxOf["handlePing"] = "", "", "Routing", ""
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	parts := partsByTitle(svc)
	if got := membersOf(parts["Storage"]); !slices.Equal(got, []string{"Store", "Store.Get", "Store.Put", "helper", "limits"}) {
		t.Fatalf("Storage holds %v", got)
	}
	if got := membersOf(parts["Entry"]); !slices.Equal(got, []string{"Serve", "main"}) {
		t.Fatalf("Entry holds %v", got)
	}
	var undecided []string
	for _, entry := range svc.OffMap {
		if entry.Reason == atlas.OffMapUndecided {
			for _, symbol := range entry.File.Symbols {
				undecided = append(undecided, symbol.Name)
			}
		}
	}
	if !slices.Equal(undecided, []string{"handlePing"}) {
		t.Fatalf("undecided: %v; a hand-over or a function value read placed a unit", undecided)
	}
	for _, name := range []string{"helper", "limits", "main", "handlePing"} {
		if asked := slices.Index(jev.assigned, name); asked < 0 || slices.Contains(jev.assigned[asked+1:], name) {
			t.Fatalf("%s was asked %v, want once", name, jev.assigned)
		}
	}
	recorded := map[string][]string{}
	for _, row := range result.Rejected {
		if row.Kind == "role_placed_by_users" || row.Kind == "role_placed_by_uses" {
			recorded[row.Kind] = append(recorded[row.Kind], row.Samples...)
		}
	}
	if !slices.Equal(recorded["role_placed_by_users"], []string{"helper", "limits"}) || !slices.Equal(recorded["role_placed_by_uses"], []string{"main"}) {
		t.Fatalf("recorded placements: %v", recorded)
	}

	provider, jev = defaultRoleProvider(), defaultRoleJev()
	result, err = Read(t.Context(), roleOptions(t, roleGraph(t, nil), provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(targetOf(t, result, "svc").OffMap, func(entry atlas.OffMapFile) bool {
		return entry.Reason == atlas.OffMapUndecided && len(entry.File.Symbols) == 1 && entry.File.Symbols[0].Name == "helper"
	}) {
		t.Fatal("helper, whose callers sit in Entry and Storage, was placed")
	}
}

// helperJev is the default role fake with the named declarations decided
// helpers.
func helperJev(names ...string) *roleJev {
	jev := defaultRoleJev()
	for _, name := range names {
		jev.helper[name] = typesafetest.Choose(lines.RoleHelperHelper)
	}
	return jev
}

// helperMarks are the atlas symbols of a target that carry the helper mark,
// by name.
func helperMarks(target atlas.Target) []string {
	var names []string
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				if symbol.Helper {
					names = append(names, symbol.Name)
				}
			}
		}
	}
	for _, entry := range target.OffMap {
		for _, symbol := range entry.File.Symbols {
			if symbol.Helper {
				names = append(names, symbol.Name)
			}
		}
	}
	slices.Sort(names)
	return names
}

// A helper is never named or assigned: the naming of server.go lists
// neither helper nor its name among Store's calls, and the assignment never
// asks about it. Called only by Store.Put, it joins Storage by code with no
// question (rule A), is recorded as attached and carries the helper mark.
func TestHelpersAreNotNamedAndGoWithTheirUsers(t *testing.T) {
	graph := roleGraph(t, withoutMainCallingHelper)
	provider, jev := defaultRoleProvider(), helperJev("helper")
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	var naming struct {
		File struct {
			Declarations []boxesDecl `json:"declarations"`
		} `json:"file"`
	}
	if err := json.Unmarshal(provider.named["svc/server.go"], &naming); err != nil {
		t.Fatal(err)
	}
	for _, decl := range naming.File.Declarations {
		if decl.Name == "helper" || slices.Contains(decl.Calls, "helper") || slices.Contains(decl.CalledBy, "helper") {
			t.Fatalf("the naming shows the helper: %+v", decl)
		}
	}
	if len(naming.File.Declarations) != 4 {
		t.Fatalf("the naming lists %d declarations, want the 4 that are no helpers", len(naming.File.Declarations))
	}
	if slices.Contains(jev.assigned, "helper") {
		t.Fatalf("the helper was assigned: %v", jev.assigned)
	}
	svc := targetOf(t, result, "svc")
	if got := membersOf(partsByTitle(svc)["Storage"]); !slices.Equal(got, []string{"Store", "Store.Get", "Store.Put", "helper"}) {
		t.Fatalf("Storage holds %v", got)
	}
	if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool {
		return row.Kind == "role_attached" && slices.Equal(row.Samples, []string{"helper"})
	}) {
		t.Fatal("the helper's placement is not recorded")
	}
	if marks := helperMarks(svc); !slices.Equal(marks, []string{"helper"}) {
		t.Fatalf("helper marks: %v", marks)
	}
}

// A helper whose users stand in two boxes (main in Entry, Store.Put in
// Storage) is shared: code cannot place it, so the assignment asks about it
// once, after code has settled, in a round of its own whose windows keep the
// first pass's. Placed, it goes where the answer says; a near-tie leaves it
// undecided. No unit is asked twice.
func TestASharedHelperIsAskedOnceInASecondPass(t *testing.T) {
	read := func(box string) (Result, *roleJev, Options) {
		t.Helper()
		provider, jev := defaultRoleProvider(), helperJev("helper")
		jev.boxOf["helper"] = box
		opts := roleOptions(t, roleGraph(t, nil), provider, jev, "")
		result, err := Read(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if err := atlas.Validate(result.Atlas); err != nil {
			t.Fatal(err)
		}
		for i, name := range jev.assigned {
			if slices.Contains(jev.assigned[i+1:], name) {
				t.Fatalf("%s was assigned twice: %v", name, jev.assigned)
			}
		}
		return result, jev, opts
	}
	result, jev, opts := read("Routing")
	if !slices.Contains(jev.assigned, "helper") {
		t.Fatalf("the shared helper was not asked again: %v", jev.assigned)
	}
	if got := membersOf(partsByTitle(targetOf(t, result, "svc"))["Routing"]); !slices.Equal(got, []string{"Route", "helper"}) {
		t.Fatalf("Routing holds %v", got)
	}
	if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool {
		return row.Kind == "role_second_pass" && slices.Equal(row.Samples, []string{"helper"})
	}) {
		t.Fatal("the second pass is not recorded")
	}
	// Two targets: the first pass of svc is round 1, its second pass round 3.
	for _, name := range []string{"atlas_role_assign-r1-w0.request.ref.json", "atlas_role_assign-r3-w0.request.ref.json"} {
		if _, err := os.Stat(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, name)); err != nil {
			t.Fatalf("no %s: %v", name, err)
		}
	}
	result, _, _ = read("")
	if !slices.ContainsFunc(targetOf(t, result, "svc").OffMap, func(entry atlas.OffMapFile) bool {
		return entry.Reason == atlas.OffMapUndecided && len(entry.File.Symbols) == 1 && entry.File.Symbols[0].Name == "helper"
	}) {
		t.Fatal("a near-tie in the second pass placed the helper")
	}
}

// pool.go's getBuf, a helper Store.Get alone calls, is its face; bufPool,
// which getBuf reads, is used only in pool.go. The whole file joins Storage,
// the box of Store (rule B): it is no row of the parts request and Storage
// holds its units. When util.go's Log, a whole file, calls getBuf too, the
// file's outside users stand in two rows and it keeps a row of its own.
func TestAHelperFileJoinsTheBoxOfItsUsers(t *testing.T) {
	pool := func(logUses bool) func(files map[string][]roleDecl) {
		return func(files map[string][]roleDecl) {
			files["svc/pool.go"] = []roleDecl{
				{name: "bufPool", kind: "variable", line: 3, end: 3, code: 1},
				{name: "getBuf", kind: "function", line: 5, end: 9, code: 4, uses: []string{"reads svc/pool.go:bufPool"}},
			}
			server := files["svc/server.go"]
			for i := range server {
				if server[i].name == "Store.Get" {
					server[i].calls = []string{"svc/pool.go:getBuf"}
				}
			}
			if logUses {
				files["svc/util.go"][0].calls = []string{"svc/pool.go:getBuf"}
			}
		}
	}
	read := func(logUses bool) (atlas.Target, *tableProvider, Result) {
		t.Helper()
		provider, jev := defaultRoleProvider(), helperJev("getBuf")
		result, err := Read(t.Context(), roleOptions(t, roleGraph(t, pool(logUses)), provider, jev, ""))
		if err != nil {
			t.Fatal(err)
		}
		if err := atlas.Validate(result.Atlas); err != nil {
			t.Fatal(err)
		}
		return targetOf(t, result, "svc"), provider, result
	}
	svc, provider, result := read(false)
	for _, row := range partsRequest(t, provider, "svc/").Units {
		if row["path"] == "svc/pool.go" && row["box"] == nil {
			t.Fatalf("pool.go joined no box: %v", row)
		}
	}
	if got := membersOf(partsByTitle(svc)["Storage"]); !slices.Equal(got, []string{"Store", "Store.Get", "Store.Put", "bufPool", "getBuf"}) {
		t.Fatalf("Storage holds %v", got)
	}
	if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool {
		return row.Kind == "role_attached" && slices.Equal(row.Samples, []string{"svc/pool.go"})
	}) {
		t.Fatal("the file's joining is not recorded")
	}
	svc, provider, _ = read(true)
	rows := 0
	for _, row := range partsRequest(t, provider, "svc/").Units {
		if row["path"] == "svc/pool.go" && row["box"] == nil {
			rows++
		}
	}
	if rows != 1 || slices.Contains(membersOf(partsByTitle(svc)["Storage"]), "getBuf") {
		t.Fatalf("pool.go, which a whole file also uses, joined Storage: %d rows", rows)
	}
}

// The helper question's item is code structure only: its name, kind, file,
// signature and lines, the declarations of the program it calls and that
// call it, read it or hand it over, as "path:name", with test code left out,
// and its registrations. A function or variable nothing uses is not asked;
// a type always is.
func TestTheHelperItemCarriesItsUsers(t *testing.T) {
	graph := roleGraphWith(t, func(files map[string][]roleDecl) {
		server := files["svc/server.go"]
		for i := range server {
			if server[i].name == "Store.Get" {
				server[i].uses = []string{"reads svc/server.go:limits"}
			}
		}
		files["svc/server.go"] = append(server,
			roleDecl{name: "limits", kind: "variable", line: 66, end: 66, code: 1},
			roleDecl{name: "routes", kind: "variable", line: 68, end: 68, code: 1, uses: []string{"passes_callback svc/server.go:handlePing"}},
			roleDecl{name: "handlePing", kind: "function", line: 70, end: 72, code: 3},
		)
		files["svc/server_test.go"][0].calls = []string{"svc/server.go:Route"}
	}, []atlas.Place{registration(6, "Route", "HandleFunc", "/route")})
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	if _, err := Read(t.Context(), roleOptions(t, graph, provider, jev, "")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]map[string]any{
		"limits":     {"name": "limits", "kind": "variable", "file": "svc/server.go", "signature": "func()", "lines": 1.0, "read_by": []any{"svc/server.go:Store"}},
		"handlePing": {"name": "handlePing", "kind": "function", "file": "svc/server.go", "signature": "func()", "lines": 3.0, "handed_over_by": []any{"svc/server.go:routes"}},
		"Route": {"name": "Route", "kind": "function", "file": "svc/server.go", "signature": "func()", "lines": 7.0,
			"calls": []any{"svc/server.go:Store"}, "called_by": []any{"svc/server.go:Serve"}, "registered": []any{"HandleFunc /route"}},
		"Store": {"name": "Store", "kind": "type", "file": "svc/server.go", "signature": "func()", "lines": 22.0, "methods": []any{"Get func()", "Put func()"},
			"calls": []any{"svc/server.go:helper"}, "called_by": []any{"svc/db.go:Open", "svc/server.go:Route"}},
	} {
		if got := jev.helperItems[name]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s is asked as %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{"main", "Close", "Log", "TestServe", "Gen1", "routes"} {
		if slices.Contains(jev.helperAsked, name) {
			t.Fatalf("%s, which nothing in the program uses or which is test or generated code, was asked", name)
		}
	}
}

// Only a decided "helper" is a helper: a near-tie (0.52 against 0.48) leaves
// the declaration named and assigned as before, with no mark.
func TestAnUncertainHelperAnswerIsNoHelper(t *testing.T) {
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	jev.helper["helper"] = llm.Verdict{Choice: lines.RoleHelperHelper, Probabilities: map[string]float64{lines.RoleHelperHelper: 0.52, lines.RoleHelperOwnJob: 0.48}}
	result, err := Read(t.Context(), roleOptions(t, roleGraph(t, nil), provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(jev.helperAsked, "helper") || !slices.Contains(jev.assigned, "helper") || !strings.Contains(string(provider.named["svc/server.go"]), `"name":"helper"`) {
		t.Fatalf("an uncertain helper was not named and assigned: %v", jev.assigned)
	}
	if marks := helperMarks(targetOf(t, result, "svc")); len(marks) != 0 {
		t.Fatalf("helper marks: %v", marks)
	}
}

// An input stands where its handler is. helper, registered at line 6 inside
// main's source range, is undecided while main and Store.Put call it: its
// input names no part, not main's Entry. Called only by Store.Put, it goes
// in Storage by code and its input stands there.
func TestAnInputWithAnUndecidedHandlerNamesNoPart(t *testing.T) {
	registered := []atlas.Place{registration(6, "helper", "HandleFunc", "/helper")}
	graph := roleGraphWith(t, nil, registered)
	input := func(result Result) atlas.Boundary {
		t.Helper()
		for _, boundary := range targetOf(t, result, "svc").Boundaries {
			if boundary.LineNo == 6 {
				return boundary
			}
		}
		t.Fatal("the registration of helper is no boundary")
		return atlas.Boundary{}
	}
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if box := input(result).BoxID; box != "" {
		t.Fatalf("the input of the undecided helper stands in %q (Entry is %q)", box, partsByTitle(targetOf(t, result, "svc"))["Entry"].ID)
	}
	provider, jev = defaultRoleProvider(), defaultRoleJev()
	result, err = Read(t.Context(), roleOptions(t, roleGraphWith(t, withoutMainCallingHelper, registered), provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if box, storage := input(result).BoxID, partsByTitle(targetOf(t, result, "svc"))["Storage"].ID; box != storage {
		t.Fatalf("the input of helper stands in %q, not Storage %q", box, storage)
	}
}

// One rule for every file: a place takes the part of the declaration around
// it, and a file's own part is the one part holding all its placed units.
// server.go's units sit in three parts, so it has no part of its own: the
// import from db.go into it draws no arrow into its parts, while Open's call
// of Store.Put draws Database → Storage. The config read at line 5 inside
// main stands in main's part, Entry, which starts the trace, stands in the
// "in" column and is not asked for a core role. A config read inside
// Store.Flush, a method of Store declared in db.go, stands in Store's part,
// Storage, not in Database, the part of db.go's own units.
func TestOneRuleForEveryFile(t *testing.T) {
	graph := roleGraphWith(t, func(files map[string][]roleDecl) {
		files["svc/db.go"] = append(files["svc/db.go"], roleDecl{name: "Store.Flush", kind: "method", line: 15, end: 19, code: 4})
	}, []atlas.Place{{ID: "bnd:svc/db.go:17:config", Kind: atlas.PlaceBoundary, Path: "svc/db.go", LineNo: 17,
		Parent: atlas.FileID("svc/db.go"), TargetIDs: []string{"svc"}, Given: "config FLUSH_EVERY",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryConfig, Values: []string{"FLUSH_EVERY"}}}})
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	parts := partsByTitle(svc)
	if parts["Entry"].ID == "" || !slices.Contains(membersOf(parts["Storage"]), "Store.Flush") {
		t.Fatalf("no split, or Store.Flush left its type: %v", parts)
	}
	roleParts := map[string]bool{parts["Entry"].ID: true, parts["Routing"].ID: true, parts["Storage"].ID: true}
	calls := false
	for _, arrow := range svc.Arrows {
		if len(arrow.Witnesses) == 0 && (roleParts[arrow.From] || roleParts[arrow.To]) {
			t.Fatalf("the import into server.go drew an arrow %s -> %s", arrow.From, arrow.To)
		}
		calls = calls || arrow.From == parts["Database"].ID && arrow.To == parts["Storage"].ID
	}
	if !calls {
		t.Fatalf("Open's call of Store.Put drew no arrow: %+v", svc.Arrows)
	}
	if parts["Entry"].Side != atlas.SideIn {
		t.Fatalf("the entry: Entry stands %q", parts["Entry"].Side)
	}
	if slices.Contains(jev.roleParts(), "Entry") {
		t.Fatal("the part holding main was asked for a core role")
	}
	stands := map[string]string{}
	for _, boundary := range svc.Boundaries {
		stands[fmt.Sprintf("%s:%d", boundary.Path, boundary.LineNo)] = boundary.BoxID
	}
	if stands["svc/server.go:5"] != parts["Entry"].ID {
		t.Fatalf("the config read inside main stands in %q, not Entry %q", stands["svc/server.go:5"], parts["Entry"].ID)
	}
	if stands["svc/db.go:17"] != parts["Storage"].ID {
		t.Fatalf("the config read inside Store.Flush stands in %q, not Storage %q (Database is %q)", stands["svc/db.go:17"], parts["Storage"].ID, parts["Database"].ID)
	}
}

// roleParts are the parts asked for a core role, by title.
func (jev *roleJev) roleParts() []string {
	return jev.asked["role"]
}

// A box the parts answer leaves out goes to the follow-up as its own row,
// with its file's path, its box's name and its calls to and from the drawn
// parts; its file stays on the map through its other boxes. Refused, its
// declarations are off the map as left out while the other boxes keep their
// parts; placed, it joins the part chosen.
func TestALeftOutBoxKeepsItsFileOnTheMap(t *testing.T) {
	graph := roleGraph(t, nil)
	read := func(choice func(options []any) string) (Result, map[string]any) {
		t.Helper()
		provider, jev := defaultRoleProvider(), defaultRoleJev()
		partFor := provider.partFor
		provider.partFor = func(unit map[string]any) string {
			if unit["box"] == "Routing" {
				return "" // a group without a name: the Routing box is left out
			}
			return partFor(unit)
		}
		var asked map[string]any
		provider.placeFor = func(row map[string]any) string {
			asked = row
			options, _ := row["part_options"].([]any)
			return choice(options)
		}
		result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
		if err != nil {
			t.Fatal(err)
		}
		if err := atlas.Validate(result.Atlas); err != nil {
			t.Fatal(err)
		}
		return result, asked
	}
	result, asked := read(func([]any) string { return "p99" })
	svc := targetOf(t, result, "svc")
	parts := partsByTitle(svc)
	if asked["path"] != "svc/server.go" || asked["box"] != "Routing" || fmt.Sprint(asked["calls"]) != fmt.Sprintf("[-> %s (1) %s -> (1)]", parts["Storage"].ID, parts["Entry"].ID) {
		t.Fatalf("the left-out box was asked as %v", asked)
	}
	offMap := map[string][]string{}
	for _, entry := range svc.OffMap {
		for _, symbol := range entry.File.Symbols {
			offMap[entry.File.Path+" "+entry.Reason] = append(offMap[entry.File.Path+" "+entry.Reason], symbol.Name)
		}
	}
	if !slices.Equal(offMap["svc/server.go left_out"], []string{"Route"}) || parts["Entry"].ID == "" || parts["Storage"].ID == "" {
		t.Fatalf("off the map %v, parts %v", offMap, parts)
	}
	result, _ = read(func(options []any) string { return fmt.Sprint(options[0]) })
	svc = targetOf(t, result, "svc")
	if got := membersOf(partsByTitle(svc)["Database"]); !slices.Equal(got, []string{"Close", "Open", "Route"}) {
		t.Fatalf("the placed box joined %v", got)
	}
}

// A near-tie at the gate (0.54 against 0.46, under the 0.10 lead) keeps the
// file whole: no naming is asked.
func TestAnUncertainGateAsksNoNaming(t *testing.T) {
	graph := roleGraph(t, nil)
	provider := defaultRoleProvider()
	tie := llm.Verdict{Choice: lines.RoleSeveralBoxes, Probabilities: map[string]float64{lines.RoleSeveralBoxes: 0.54, lines.RoleOneBox: 0.46}}
	jev := newRoleJev(map[string]llm.Verdict{"svc/server.go": tie, "one/tool.go": tie}, nil)
	if _, err := Read(t.Context(), roleOptions(t, graph, provider, jev, "")); err != nil {
		t.Fatal(err)
	}
	if len(jev.gated) == 0 || provider.designRequests[designBoxesTask] != 0 || len(jev.assigned) != 0 {
		t.Fatalf("gate %v, naming %d, assignment %v", jev.gated, provider.designRequests[designBoxesTask], jev.assigned)
	}
}

// The naming's list is one partition decision: a box without holds keeps
// the file whole. Two boxes sharing a name are both offered, each by its
// ref with its own holds, and both are drawn.
func TestNamingFormsAndRefusals(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	provider.boxesFor = func(path string) string {
		return `{"boxes":[{"name":"Entry","holds":"Starting."},{"name":"Routing"}]}`
	}
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if parts := partsByTitle(targetOf(t, result, "svc")); parts["Server"].ID == "" || len(jev.assigned) != 0 ||
		!slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool { return row.Kind == "role_boxes_incomplete" }) {
		t.Fatalf("an incomplete box list split the file: %v, assigned %v", parts, jev.assigned)
	}

	provider, jev = defaultRoleProvider(), defaultRoleJev()
	provider.boxesFor = func(path string) string {
		return `{"boxes":[{"name":"Entry","holds":"Starting and serving."},{"name":"Store","holds":"The store type."},{"name":"store","holds":"Its helper."}]}`
	}
	jev.boxOf = map[string]string{"main": "Entry", "Serve": "Entry", "Route": "Entry", "Store": "b2", "helper": "b3"}
	result, err = Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	var titles []string
	for _, box := range svc.Boxes {
		titles = append(titles, box.Title)
	}
	if !slices.Contains(titles, "Store") || !slices.Contains(titles, "store") {
		t.Fatalf("two boxes sharing a name were not both drawn: %v", titles)
	}
}

// A file is split only into two or more boxes. When the naming gives one box,
// nothing is assigned; when the assignment puts every decided unit in one
// box, the file stays whole in the answer's part with every unit, the
// undecided one included, and nothing of it goes off the map.
func TestAFileInFewerThanTwoBoxesStaysWhole(t *testing.T) {
	graph := roleGraph(t, nil)
	var provider *tableProvider
	whole := func(t *testing.T, result Result, kind string) {
		t.Helper()
		var rows []string
		for _, row := range partsRequest(t, provider, "svc/").Units {
			if row["path"] == "svc/server.go" {
				rows = append(rows, fmt.Sprintf("%v %v", row["ref"], row["box"]))
			}
		}
		if !slices.Equal(rows, []string{"f4 <nil>"}) {
			t.Fatalf("server.go's rows: %v, want one whole row", rows)
		}
		svc := targetOf(t, result, "svc")
		parts := partsByTitle(svc)
		if got := membersOf(parts["Server"]); !slices.Equal(got, []string{"Route", "Serve", "Store", "Store.Get", "Store.Put", "helper", "main"}) {
			t.Fatalf("Server holds %v; the whole file stays in its part", got)
		}
		if parts["Entry"].ID != "" {
			t.Fatalf("a box was drawn for a file that stays whole: %v", parts)
		}
		for _, entry := range svc.OffMap {
			if entry.File.Path == "svc/server.go" {
				t.Fatalf("a unit of a whole file went off the map: %+v", entry)
			}
		}
		if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool {
			return row.Kind == kind && slices.Contains(row.Samples, "svc/server.go") || row.Kind == kind && strings.HasPrefix(row.Reason, "svc/server.go")
		}) {
			t.Fatalf("no %s record for server.go", kind)
		}
	}

	provider = defaultRoleProvider()
	jev := defaultRoleJev()
	provider.boxesFor = func(path string) string { return `{"boxes":[{"name":"Entry","holds":"All of it."}]}` }
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(jev.assigned, "main") {
		t.Fatalf("one named box was assigned: %v", jev.assigned)
	}
	whole(t, result, "role_one_box")

	provider, jev = defaultRoleProvider(), defaultRoleJev()
	jev.boxOf = map[string]string{"main": "Entry", "Serve": "Entry", "Route": "Entry", "Store": "Entry", "helper": "", "Run": "Running", "Check": "Checking"}
	result, err = Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	whole(t, result, "role_not_split")
}

// Without a model nothing is asked, though the same reading with one splits.
func TestWithoutAModelNothingIsSplit(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if partsByTitle(targetOf(t, result, "svc"))["Entry"].ID == "" {
		t.Fatal("no split happened with a model")
	}
	opts := roleOptions(t, graph, nil, defaultRoleJev(), "")
	opts.Provider, opts.Categorizer = nil, nil
	if _, err := Read(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
}

// The role split's requests are keyed by request-local refs: a warm read
// asks nothing, a file added earlier in path order (which renumbers every
// later place) is the only new gate, and a call into server.go from
// another file changes server.go's naming alone.
func TestTheRoleSplitCacheIsLocalToEachFile(t *testing.T) {
	cache := t.TempDir()
	live := func(result Result, stage string) (int, int) {
		for _, use := range result.Uses {
			if use.Stage == stage {
				return use.Live, use.Cached
			}
		}
		return 0, 0
	}
	read := func(graph atlas.Graph) (Result, *roleJev) {
		provider, jev := defaultRoleProvider(), defaultRoleJev()
		result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, cache))
		if err != nil {
			t.Fatal(err)
		}
		if partsByTitle(targetOf(t, result, "svc"))["Entry"].ID == "" {
			t.Fatal("no split happened")
		}
		return result, jev
	}
	read(roleGraph(t, nil))
	warm, jev := read(roleGraph(t, nil))
	if jev.Calls() != 0 {
		t.Fatalf("a warm read asked Jev %d times", jev.Calls())
	}
	for _, stage := range []string{lines.StageRoleHelper, lines.StageRoleGate, lines.StageRoleBoxes, lines.StageRoleAssign} {
		if fresh, _ := live(warm, stage); fresh != 0 {
			t.Fatalf("a warm read asked %s %d times live", stage, fresh)
		}
	}
	earlier, _ := read(roleGraph(t, func(files map[string][]roleDecl) {
		files["svc/a.go"] = []roleDecl{{name: "A", kind: "function", line: 3, end: 4, code: 2}, {name: "B", kind: "function", line: 6, end: 7, code: 2, calls: []string{"svc/a.go:A"}}}
	}))
	if fresh, cached := live(earlier, lines.StageRoleGate); fresh != 1 || cached != 3 {
		t.Fatalf("an earlier file re-asked the gate: %d live, %d cached", fresh, cached)
	}
	// The helper question asks one group per file: a.go's is the one new.
	if fresh, cached := live(earlier, lines.StageRoleHelper); fresh != 1 || cached != 1 {
		t.Fatalf("an earlier file re-asked the helper question: %d live, %d cached", fresh, cached)
	}
	for _, stage := range []string{lines.StageRoleBoxes, lines.StageRoleAssign} {
		if fresh, _ := live(earlier, stage); fresh != 0 {
			t.Fatalf("an earlier file re-asked %s %d times", stage, fresh)
		}
	}
	called, _ := read(roleGraph(t, func(files map[string][]roleDecl) {
		files["svc/util.go"][0].calls = []string{"svc/server.go:Route"}
	}))
	if fresh, _ := live(called, lines.StageRoleGate); fresh != 0 {
		t.Fatalf("a call from another file re-asked the gate %d times", fresh)
	}
	if fresh, cached := live(called, lines.StageRoleBoxes); fresh != 1 || cached != 1 {
		t.Fatalf("a call into server.go re-asked the naming %d times (%d cached)", fresh, cached)
	}
	// Route's item names its callers across files, so server.go's helper
	// group is asked again.
	if fresh, _ := live(called, lines.StageRoleHelper); fresh != 1 {
		t.Fatalf("a call into server.go re-asked the helper question %d times", fresh)
	}
}

// decodeBoxes reads forms, not refusals: a wrapper, keys in another case,
// whitespace, an identical repeat once, null as no boxes; the same name
// with other holds is kept twice and noted, a box without holds is noted
// as incomplete. Only an answer that is not JSON or has no list is refused.
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
// or latency of it (the role split's stages were missing in the first real
// run).
func TestTheMapOfPartsStagesAreJournaled(t *testing.T) {
	writer, err := debugdump.NewWriter(t.TempDir(), "run")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, stage := range []string{
		lines.StageZones, lines.StagePlacement, lines.StageDescribe, lines.StageAreas,
		lines.StageRoleHelper, lines.StageRoleGate, lines.StageRoleBoxes, lines.StageRoleAssign, lines.StageCore, lines.StageKeys,
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
