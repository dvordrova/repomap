package reading

import (
	"encoding/json"
	"fmt"
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
}

// roleGraph is target svc: server.go, the seed file whose code goes in
// several boxes (main, Serve, Route, the Store type with two methods, a
// helper and a config read with no subject); db.go and util.go, which the
// parts answer places whole; a test file and a generated file of two units
// each, which are never candidates; and target one, the single file
// one/tool.go. db.go imports server.go and its Open calls Store.Put. extra
// may add places and edges before the graph is sealed.
func roleGraph(t *testing.T, extra func(files map[string][]roleDecl)) atlas.Graph {
	t.Helper()
	files := map[string][]roleDecl{
		"svc/server.go": {
			{name: "main", kind: "function", line: 3, end: 8, code: 5, calls: []string{"svc/server.go:Serve"}},
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
			if spec.name == "Store" {
				for _, method := range []string{"Store.Get", "Store.Put"} {
					symbol.Members = append(symbol.Members, atlas.TypeMember{Path: path, Decl: atlas.Decl{Name: method, Kind: "method", LineNo: lineOf[path+":"+method]}})
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
// undecided), and the other closed tables as closedDecisions does. It
// keeps every gate and assignment item it was asked.
type roleJev struct {
	typesafetest.Categorizer
	mu       sync.Mutex
	gate     map[string]llm.Verdict
	boxOf    map[string]string
	gated    []string
	assigned []string
	// asked are the items of every other closed question, by column.
	asked map[string][]string
}

func newRoleJev(gate map[string]llm.Verdict, boxOf map[string]string) *roleJev {
	jev := &roleJev{gate: gate, boxOf: boxOf}
	closed := closedDecisions().Decide
	jev.Decide = func(key string, question llm.Question) (llm.Verdict, bool) {
		jev.mu.Lock()
		defer jev.mu.Unlock()
		switch {
		case strings.HasSuffix(key, "|boxes"):
			path, _ := question.Item["path"].(string)
			jev.gated = append(jev.gated, path)
			if verdict, ok := jev.gate[path]; ok {
				return verdict, true
			}
			return typesafetest.Choose(lines.RoleOneBox), true
		case strings.HasSuffix(key, "|box"):
			name, _ := question.Item["declaration"].(string)
			jev.assigned = append(jev.assigned, name)
			box := jev.boxOf[name]
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

func defaultRoleProvider() *tableProvider {
	return &tableProvider{
		partFor: func(file map[string]any) string {
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

// The split itself: server.go's code goes in the role parts Entry, Routing
// and Storage, in naming order after the answer's parts; its part Server,
// left without a file, is not drawn and takes no ID; Unused, which holds
// nothing, is not drawn; helper, a near-tie, is off the map as undecided
// while its file stays on the map; the Store type's methods follow it. The
// test and generated files are never candidates; util.go (one unit) is not
// asked. The one-file target splits too.
func TestTheRoleSplitDrawsAFilesBoxesAsParts(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
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
	svc := targetOf(t, result, "svc")
	parts := partsByTitle(svc)
	if _, drawn := parts["Server"]; drawn || parts["Unused"].ID != "" {
		t.Fatalf("the emptied part or the empty box was drawn: %v", parts)
	}
	// The answer lists Database, Other (the test and generated files),
	// Server and Utilities.
	ids := []string{parts["Database"].ID, parts["Other"].ID, parts["Utilities"].ID, parts["Entry"].ID, parts["Routing"].ID, parts["Storage"].ID}
	if !slices.Equal(ids, []string{"p1", "p2", "p3", "p4", "p5", "p6"}) {
		t.Fatalf("IDs: answer parts in order without the emptied one, then the boxes in naming order: %v", ids)
	}
	if got := membersOf(parts["Entry"]); !slices.Equal(got, []string{"Serve", "main"}) {
		t.Fatalf("Entry holds %v", got)
	}
	if got := membersOf(parts["Storage"]); !slices.Equal(got, []string{"Store", "Store.Get", "Store.Put"}) {
		t.Fatalf("Storage holds %v: a type's methods follow it", got)
	}
	var undecided []string
	for _, entry := range svc.OffMap {
		if entry.File.Path == "svc/server.go" {
			if entry.Reason != atlas.OffMapUndecided || entry.BoxID != "" {
				t.Fatalf("server.go off the map: %+v", entry)
			}
			for _, symbol := range entry.File.Symbols {
				undecided = append(undecided, symbol.Name)
			}
		}
	}
	if !slices.Equal(undecided, []string{"helper"}) {
		t.Fatalf("undecided: %v", undecided)
	}
	one := targetOf(t, result, "one")
	if titles := partsByTitle(one); len(one.Boxes) != 2 || titles["Running"].ID == "" || titles["Checking"].ID == "" {
		t.Fatalf("the one-file target was not split: %+v", one.Boxes)
	}
	for _, kind := range []string{"role_box_empty", "role_undecided"} {
		if !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool { return row.Kind == kind }) {
			t.Fatalf("no %s record", kind)
		}
	}
}

// A split file is no part's endpoint. The import from db.go into server.go
// draws no arrow into a role part, while Open's call of Store.Put draws
// Database → Storage. The entry stays: main's part, Entry, starts the trace,
// stands in the "in" column and is not asked for a core role. A config read
// with no subject stands in the part of the declaration whose range holds
// its line (main). Each role part is described by the units it holds.
func TestASplitFileIsNoPartsEndpoint(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	parts := partsByTitle(svc)
	if parts["Entry"].ID == "" {
		t.Fatal("no split happened")
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
	if len(svc.Trace) == 0 || svc.Trace[0] != parts["Entry"].ID || parts["Entry"].Side != atlas.SideIn {
		t.Fatalf("the entry: trace %v, Entry stands %q", svc.Trace, parts["Entry"].Side)
	}
	if slices.Contains(jev.roleParts(), "Entry") {
		t.Fatal("the part holding main was asked for a core role")
	}
	for _, boundary := range svc.Boundaries {
		if boundary.Path == "svc/server.go" && boundary.BoxID != parts["Entry"].ID {
			t.Fatalf("the config read at line 5 stands in %q", boundary.BoxID)
		}
	}
	var storage describeInput
	if err := json.Unmarshal(provider.described["Storage"], &storage); err != nil {
		t.Fatal(err)
	}
	if len(storage.Directories) != 1 || len(storage.Directories[0].Files) != 1 || len(storage.Directories[0].Files[0].Members) != 1 || storage.Directories[0].Files[0].Members[0].Name != "Store" {
		t.Fatalf("Storage is described by %s", provider.described["Storage"])
	}
}

// roleParts are the parts asked for a core role, by title.
func (jev *roleJev) roleParts() []string {
	return jev.asked["role"]
}

// A file the parts answer left out is placed whole, never split: its
// naming may have been asked, but no role part is drawn from it and the
// split is recorded as not applied. A left-out file placed into a role part
// joins it whole, and the role part's own units stay exactly its own.
func TestPlacementOnlyAddsWholeFiles(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	partFor := provider.partFor
	provider.partFor = func(file map[string]any) string {
		if file["path"] == "svc/db.go" {
			return "" // a group without a name: db.go is left out
		}
		return partFor(file)
	}
	var offered []any
	storage := ""
	provider.placeFor = func(row map[string]any) string {
		offered, _ = row["calls"].([]any)
		return storage
	}
	// Storage is the last role part drawn: p5 (Other, Utilities, then the
	// boxes Entry, Routing, Storage).
	storage = "p5"
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	parts := partsByTitle(svc)
	if parts["Storage"].ID != storage {
		t.Fatalf("Storage is %s", parts["Storage"].ID)
	}
	if got := membersOf(parts["Storage"]); !slices.Equal(got, []string{"Close", "Open", "Store", "Store.Get", "Store.Put"}) {
		t.Fatalf("Storage holds %v", got)
	}
	if !slices.ContainsFunc(offered, func(call any) bool { return call == "-> "+storage+" (1)" }) {
		t.Fatalf("db.go's placement row shows its call of Store.Put as %v", offered)
	}
	for _, entry := range svc.OffMap {
		if entry.File.Path == "svc/db.go" || entry.File.Path == "svc/server.go" && (entry.Reason != atlas.OffMapUndecided || len(entry.File.Symbols) != 1) {
			t.Fatalf("off the map: %+v", entry)
		}
	}

	// Left out itself, server.go is placed whole.
	provider, jev = defaultRoleProvider(), defaultRoleJev()
	partFor = provider.partFor
	provider.partFor = func(file map[string]any) string {
		if file["path"] == "svc/server.go" {
			return ""
		}
		return partFor(file)
	}
	result, err = Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	svc = targetOf(t, result, "svc")
	if parts := partsByTitle(svc); parts["Entry"].ID != "" || !slices.ContainsFunc(result.Rejected, func(row modeldiag.Row) bool { return row.Kind == "role_not_applied" }) {
		t.Fatalf("a left-out file was split: %v", parts)
	}
	for _, entry := range svc.OffMap {
		if entry.Reason == atlas.OffMapUndecided {
			t.Fatalf("a whole file has undecided declarations: %+v", entry)
		}
	}
}

// A part that still holds a whole file after the split is drawn, described
// by that file's units alone.
func TestAPartKeepsItsOtherFiles(t *testing.T) {
	graph := roleGraph(t, nil)
	provider, jev := defaultRoleProvider(), defaultRoleJev()
	provider.partFor = func(file map[string]any) string {
		switch file["path"] {
		case "svc/server.go", "svc/util.go":
			return "Server"
		}
		return "Other"
	}
	result, err := Read(t.Context(), roleOptions(t, graph, provider, jev, ""))
	if err != nil {
		t.Fatal(err)
	}
	parts := partsByTitle(targetOf(t, result, "svc"))
	if parts["Entry"].ID == "" || parts["Server"].ID == "" {
		t.Fatalf("parts: %v", parts)
	}
	if got := membersOf(parts["Server"]); !slices.Equal(got, []string{"Log"}) {
		t.Fatalf("Server holds %v", got)
	}
	if described := string(provider.described["Server"]); !strings.Contains(described, `"Log"`) || strings.Contains(described, `"Serve"`) {
		t.Fatalf("Server is described by %s", described)
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
	whole := func(t *testing.T, result Result, kind string) {
		t.Helper()
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

	provider, jev := defaultRoleProvider(), defaultRoleJev()
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
	for _, stage := range []string{lines.StageRoleGate, lines.StageRoleBoxes, lines.StageRoleAssign} {
		if fresh, _ := live(warm, stage); fresh != 0 {
			t.Fatalf("a warm read asked %s %d times live", stage, fresh)
		}
	}
	earlier, _ := read(roleGraph(t, func(files map[string][]roleDecl) {
		files["svc/a.go"] = []roleDecl{{name: "A", kind: "function", line: 3, end: 4, code: 2}, {name: "B", kind: "function", line: 6, end: 7, code: 2}}
	}))
	if fresh, cached := live(earlier, lines.StageRoleGate); fresh != 1 || cached != 3 {
		t.Fatalf("an earlier file re-asked the gate: %d live, %d cached", fresh, cached)
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
		lines.StageRoleGate, lines.StageRoleBoxes, lines.StageRoleAssign, lines.StageCore, lines.StageKeys,
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
