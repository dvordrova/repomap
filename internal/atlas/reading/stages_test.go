package reading

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestInterpretedOperationsBindCallsWithoutFrameworkRules(t *testing.T) {
	client := atlas.Place{ID: "send", Kind: atlas.PlaceSymbol, Path: "app/send.go", LineNo: 5, Parent: "file:send", TargetIDs: []string{"client"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "sender", Name: "Submit"}}}
	handler := atlas.Place{ID: "handle", Kind: atlas.PlaceSymbol, Path: "service/handle.py", LineNo: 8, Parent: "file:handle", TargetIDs: []string{"service"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "receiver", Name: "Handler.Submit", Column: 17}}}
	r := reader{opts: Options{Graph: atlas.Graph{Places: []atlas.Place{client, handler}}}, boundaries: map[string]*boundaryState{}, operations: map[string][3]string{"handle": {"request", "submit job"}}, symbolLine: map[string]cell{"send": {value: "Submits a job to the service."}, "handle": {value: "Accepts a job."}}, outbound: map[string][]atlas.SymbolCall{"send": {{Name: "CompanySDK.Submit", Line: 11, Values: []string{"jobs"}}}}}
	r.bindInterpretedBoundaries()
	if len(r.boundaries) != 2 {
		t.Fatalf("wanted two bound endpoints, got %v", r.boundaries)
	}
	out, in := r.boundaries["out:send:0"], r.boundaries["in:handle"]
	if out.place.Boundary.ObjectID != "sender" || out.place.LineNo != 11 || in.place.Boundary.ObjectID != "receiver" || in.place.Column != 17 {
		t.Fatal("lost source identity")
	}
	if out.place.Boundary.Source != "model" || in.place.Boundary.Source != "model" {
		t.Fatal("interpretation became a source fact")
	}
	r.boxOf = map[string]string{"file:handle": "handler-group"}
	published := r.target(TargetMeta{ID: "service"})
	if len(published.Boundaries) != 1 || published.Boundaries[0].Column != 17 {
		t.Fatalf("boundary lost the declaration column before map matching: %+v", published.Boundaries)
	}
	value, _, ok := valuesJoin(out.place.Boundary, in.place.Boundary)
	if !ok || value != "Submit" {
		t.Fatalf("method candidate lost: %q %v", value, ok)
	}
}

// twoTargetGraph is a repository of two targets: a backend "svc" with five
// top boxes, a route and a config read, and a "web" client that calls the
// route and one URL nothing serves.
func twoTargetGraph(t *testing.T) atlas.Graph {
	t.Helper()
	dir := func(path string, depth int, parent string, targets []string, dirs, files []string, count int, top bool) atlas.Place {
		return atlas.Place{
			ID: atlas.DirectoryID(path), Kind: atlas.PlaceDirectory, Path: path, Depth: depth, Parent: parent,
			TargetIDs: targets, Given: path + " files",
			Directory: &atlas.DirectoryFacts{Dirs: dirs, Files: files, FileCount: count, TopBox: top},
		}
	}
	file := func(path string, depth int, targets []string, callers, callees []string) atlas.Place {
		return atlas.Place{
			ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, Depth: depth,
			Parent: atlas.DirectoryID(filepath.Dir(path)), TargetIDs: targets, Given: "given " + path,
			File: &atlas.FileFacts{
				Decls:   []atlas.Decl{{Name: "F", Kind: "function", LineNo: 3, Exported: true, Doc: "F does."}},
				Callers: callers, Callees: callees,
			},
		}
	}
	boundary := func(path string, line int, targets []string, direction, kind, method string, values []string) atlas.Place {
		return atlas.Place{
			ID: "bnd:" + path + ":" + itoa(line) + ":" + kind, Kind: atlas.PlaceBoundary, Path: path, LineNo: line,
			Parent: atlas.FileID(path), TargetIDs: targets, Given: kind + " " + strings.Join(values, ","),
			Boundary: &atlas.BoundaryFacts{Source: "fact", Caller: "F", Direction: direction, GivenKind: kind, Method: method, Values: values},
		}
	}
	svc, web := []string{"svc"}, []string{"web"}
	places := []atlas.Place{
		dir(".", 0, "", []string{"svc", "web"}, []string{"svc", "web"}, nil, 7, false),
		dir("svc", 1, atlas.DirectoryID("."), svc, []string{"api", "core", "db", "jobs", "util"}, nil, 5, false),
		dir("svc/api", 2, atlas.DirectoryID("svc"), svc, nil, []string{"h.go"}, 1, true),
		dir("svc/core", 2, atlas.DirectoryID("svc"), svc, nil, []string{"c.go"}, 1, true),
		dir("svc/db", 2, atlas.DirectoryID("svc"), svc, nil, []string{"d.go"}, 1, true),
		dir("svc/jobs", 2, atlas.DirectoryID("svc"), svc, nil, []string{"j.go"}, 1, true),
		dir("svc/util", 2, atlas.DirectoryID("svc"), svc, nil, []string{"u.go"}, 1, true),
		dir("web", 1, atlas.DirectoryID("."), web, []string{"src"}, nil, 2, false),
		dir("web/src", 2, atlas.DirectoryID("web"), web, nil, []string{"app.ts", "client.ts"}, 2, true),
		file("svc/api/h.go", 0, svc, nil, []string{atlas.FileID("svc/core/c.go")}),
		file("svc/core/c.go", 1, svc, []string{atlas.FileID("svc/api/h.go")}, []string{atlas.FileID("svc/db/d.go"), atlas.FileID("svc/util/u.go")}),
		file("svc/db/d.go", 2, svc, []string{atlas.FileID("svc/core/c.go")}, nil),
		file("svc/jobs/j.go", 3, svc, nil, []string{atlas.FileID("svc/core/c.go")}),
		file("svc/util/u.go", 2, svc, []string{atlas.FileID("svc/core/c.go")}, nil),
		file("web/src/app.ts", 0, web, nil, []string{atlas.FileID("web/src/client.ts")}),
		file("web/src/client.ts", 1, web, []string{atlas.FileID("web/src/app.ts")}, nil),
		boundary("svc/api/h.go", 10, svc, atlas.DirectionIn, atlas.BoundaryHTTPServer, "GET", []string{"/api/levels/{id}"}),
		boundary("svc/db/d.go", 20, svc, atlas.DirectionOut, atlas.BoundaryConfig, "", []string{"DATABASE_URL"}),
		boundary("web/src/client.ts", 5, web, atlas.DirectionOut, atlas.BoundaryHTTPClient, "GET", []string{"/api/levels/{param}"}),
		boundary("web/src/client.ts", 9, web, atlas.DirectionOut, atlas.BoundaryHTTPClient, "POST", []string{"/api/nothing"}),
		boundary("web/src/app.ts", 2, web, atlas.DirectionOut, atlas.BoundaryConfig, "", []string{"DATABASE_URL"}),
	}
	edge := func(from, to string, count int) atlas.Edge {
		return atlas.Edge{From: atlas.FileID(from), To: atlas.FileID(to), Kind: "calls", Count: count,
			Witnesses: []atlas.Witness{{Caller: "F", Callee: "F", Path: from, LineNo: 4}}}
	}
	graph := atlas.Graph{
		Version: atlas.GraphVersion, Revision: "abc", Places: places,
		Edges: []atlas.Edge{
			edge("svc/api/h.go", "svc/core/c.go", 3), edge("svc/core/c.go", "svc/db/d.go", 5),
			edge("svc/core/c.go", "svc/util/u.go", 1), edge("svc/jobs/j.go", "svc/core/c.go", 2),
			edge("web/src/app.ts", "web/src/client.ts", 4),
		},
		Seeds: []string{atlas.FileID("svc/api/h.go"), atlas.FileID("web/src/app.ts")},
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

// withSymbols adds eight candidate symbols to svc/core/c.go, ranked 1..8.
func withSymbols(t *testing.T, graph atlas.Graph) atlas.Graph {
	t.Helper()
	core := -1
	for i, place := range graph.Places {
		if place.ID == atlas.FileID("svc/core/c.go") {
			core = i
		}
	}
	for i := 1; i <= 8; i++ {
		name := "Op" + itoa(i)
		graph.Places[core].File.Decls = append(graph.Places[core].File.Decls, atlas.Decl{
			Name: name, Kind: "function", Signature: "func()", LineNo: 10 + i, Exported: true,
		})
		graph.Places = append(graph.Places, atlas.Place{
			ID: atlas.SymbolID("svc/core/c.go", 10+i, name), Kind: atlas.PlaceSymbol, Path: "svc/core/c.go",
			LineNo: 10 + i, Depth: 1, TargetIDs: []string{"svc"}, Parent: atlas.FileID("svc/core/c.go"),
			Given:  "func " + name,
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, Kind: "function", Signature: "func()", LineNo: 10 + i, Exported: true}, Candidate: true, Rank: i},
		})
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

func TestSymbolsGetLinesAndAtMostFiveKeysPerFile(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	// The fake answers "yes" (the first option) for every key_symbol cell.
	provider := &tableProvider{}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	var core atlas.File
	var box atlas.Box
	for _, target := range result.Atlas.Targets {
		for _, candidate := range target.Boxes {
			for _, file := range candidate.Files {
				if file.Path == "svc/core/c.go" {
					core, box = file, candidate
				}
			}
		}
	}
	keys, lined := 0, 0
	for _, symbol := range core.Symbols {
		if symbol.Key {
			keys++
		}
		if strings.HasPrefix(symbol.Line, "Text for") {
			lined++
		}
	}
	// c.go declares F (not a candidate) plus eight candidates: eight lines,
	// five keys by rank.
	if lined != 8 || keys != lines.MaxKeysPerFile {
		t.Fatalf("lined %d keys %d: %+v", lined, keys, core.Symbols)
	}
	for _, symbol := range core.Symbols {
		if symbol.Key && !strings.HasPrefix(symbol.Name, "Op") {
			t.Fatalf("a non-candidate became a key: %+v", symbol)
		}
	}
	if len(box.Keys) != 3 || box.Keys[0].Doc == "" {
		t.Fatalf("box keys: %+v", box.Keys)
	}
	var use atlas.StageUse
	for _, candidate := range result.Uses {
		if candidate.Stage == lines.StageSymbols {
			use = candidate
		}
	}
	if use.Rows != 8 || use.Windows != 1 {
		t.Fatalf("symbol use: %+v", use)
	}
}

func itoa(value int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+value/10)) + string(rune('0'+value%10)))
}

func twoTargetOptions(t *testing.T, graph atlas.Graph, provider *tableProvider) Options {
	t.Helper()
	opts := readOptions(t, graph, provider, "")
	opts.Targets = []TargetMeta{
		{ID: "svc", Language: "go", Kind: "executable", Name: "example.com/svc", Root: "svc"},
		{ID: "web", Language: "typescript", Kind: "package", Name: "web", Root: "web"},
	}
	return opts
}

func TestZonesAreNamedAssignedAndInherited(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{
		partNames: []string{"Serving", "Domain", "Storage", "Spare"},
		partFor: map[string]string{
			"Title api": "Serving", "Title core": "Domain", "Title db": "Storage", "Title jobs": "Domain", "Title util": "Storage",
		},
	}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	var svc atlas.Target
	for _, target := range result.Atlas.Targets {
		if target.ID == "svc" {
			svc = target
		}
	}
	// Four names were asked; "Spare" held no box and vanished.
	if len(svc.Zones) != 3 {
		t.Fatalf("zones: %+v", svc.Zones)
	}
	zones := make(map[string]atlas.Zone)
	for _, zone := range svc.Zones {
		zones[zone.ID] = zone
		if zone.Line == "" || !strings.HasPrefix(zone.Line, "Text for") {
			t.Errorf("zone %s has no model line: %q", zone.ID, zone.Line)
		}
	}
	if len(zones["domain"].BoxIDs) != 2 || len(zones["storage"].BoxIDs) != 2 || len(zones["serving"].BoxIDs) != 1 {
		t.Fatalf("zone boxes: %+v", zones)
	}
	for _, box := range svc.Boxes {
		if box.ZoneID == "" {
			t.Errorf("box %s has no zone", box.ID)
		}
	}
	sides := make(map[string]string)
	for _, box := range svc.Boxes {
		sides[box.ID] = box.Side
	}
	if sides["svc/api"] != atlas.SideIn || sides["svc/db"] != atlas.SideMid || sides["svc/core"] != atlas.SideMid {
		t.Fatalf("sides: %v", sides)
	}
	if strings.Join(svc.Trace, " ") != "svc/api svc/core svc/db" {
		t.Fatalf("trace: %v", svc.Trace)
	}
	if len(svc.Arrows) != 4 {
		t.Fatalf("arrows: %+v", svc.Arrows)
	}
	for _, arrow := range svc.Arrows {
		if !strings.HasPrefix(arrow.Sentence, "Text for") {
			t.Errorf("arrow %s -> %s has no model sentence: %q", arrow.From, arrow.To, arrow.Sentence)
		}
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestConfigReadDoesNotTurnCoreIntoDependencyOrShareASeed(t *testing.T) {
	owner := &boxState{id: "tool/cmd", files: []string{"main", "commands"}}
	r := &reader{
		opts: Options{Graph: atlas.Graph{Seeds: []string{"main"}}},
		places: map[string]atlas.Place{
			"main":     {TargetIDs: []string{"exe"}},
			"commands": {TargetIDs: []string{"exe", "lib"}},
		},
		boxOf: map[string]string{"commands": owner.id},
		boundaries: map[string]*boundaryState{"token": {
			kind: "config", place: atlas.Place{Parent: "commands", TargetIDs: []string{"exe", "lib"},
				Boundary: &atlas.BoundaryFacts{Direction: atlas.DirectionOut}},
		}},
	}
	if got := r.side(owner, "lib"); got != atlas.SideMid {
		t.Fatalf("config or another target's seed classified library as %s", got)
	}
	if got := r.side(owner, "exe"); got != atlas.SideIn {
		t.Fatalf("executable lost its entrypoint: %s", got)
	}
	r.boundaries["token"].kind = "http"
	if got := r.side(owner, "lib"); got != atlas.SideOut {
		t.Fatalf("outbound integration lost its direction: %s", got)
	}
}

func TestZonesFallBackWhenTheNamesWindowIsRefused(t *testing.T) {
	graph := twoTargetGraph(t)
	// The names come back all the same: fewer distinct names than asked, so
	// the window is refused and the largest boxes name the parts; the assign
	// window then answers a name outside the list and is refused too.
	provider := &tableProvider{partNames: []string{"All", "All", "All", "All"}, partFor: map[string]string{
		"Title api": "Odd", "Title core": "Odd", "Title db": "Odd", "Title jobs": "Odd", "Title util": "Odd",
	}}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	var svc atlas.Target
	for _, target := range result.Atlas.Targets {
		if target.ID == "svc" {
			svc = target
		}
	}
	if len(svc.Zones) != lines.WantZones(5) {
		t.Fatalf("fallback zones: %d, want %d: %+v", len(svc.Zones), lines.WantZones(5), svc.Zones)
	}
	rejected := 0
	for _, row := range result.Rejected {
		if row.Stage == lines.StageZones {
			rejected++
		}
	}
	// The names window is refused; the box left over goes to an assign
	// window the fake answers with the same bad name, refused again.
	if rejected < 1 {
		t.Fatalf("zone rejections: %+v", result.Rejected)
	}
}

func TestJointsMatchValuesAcrossTargetsAndAskPeersForTheRest(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{sameFor: map[string]string{"GET /api/levels/{id}": "yes"}}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	var byValue []string
	for _, joint := range result.Atlas.Joints {
		byValue = append(byValue, joint.Value)
		if joint.From.TargetID != "web" || joint.To.TargetID != "svc" {
			t.Errorf("joint direction: %+v", joint)
		}
	}
	// The route is joined through its parameter (possible); the config keys
	// shared by both targets are not a joint; the unmatched POST chose the
	// first peer, blind.
	if len(result.Atlas.Joints) != 2 {
		t.Fatalf("joints: %v", byValue)
	}
	exact, blind := result.Atlas.Joints[0], result.Atlas.Joints[1]
	if strings.Contains(exact.ID, "client.ts:9") {
		exact, blind = blind, exact
	}
	if exact.Value != "GET /api/levels/{id}" || !exact.Possible || exact.Blind || exact.Label != "reads GET /api/levels/{id}" {
		t.Fatalf("value joint: %+v", exact)
	}
	if !blind.Blind || !blind.Possible || blind.To.BoundaryID != "bnd:svc/api/h.go:10:http_server" {
		t.Fatalf("blind joint: %+v", blind)
	}
	var web atlas.Target
	for _, target := range result.Atlas.Targets {
		if target.ID == "web" {
			web = target
		}
	}
	if web.Role == "" || web.Line != "Text for r2" && web.Line != "Text for r1" {
		t.Fatalf("portfolio cells: role %q line %q", web.Role, web.Line)
	}
	if len(web.Boundaries) != 3 {
		t.Fatalf("web boundaries: %+v", web.Boundaries)
	}
	for _, boundary := range web.Boundaries {
		if boundary.Kind == "" || !strings.HasPrefix(boundary.Line, "Text for") {
			t.Errorf("boundary %s: kind %q line %q", boundary.ID, boundary.Kind, boundary.Line)
		}
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestPeerContextsContainOnlyEligibleCounterparts(t *testing.T) {
	r := reader{targets: map[string]*targetState{"a": {role: atlas.RoleProduct}, "b": {role: atlas.RoleProduct}, "f": {role: atlas.RoleFixture}}}
	targets := map[string]TargetMeta{"a": {ID: "a"}, "b": {ID: "b"}, "f": {ID: "f", Root: "fixtures/one"}}
	boundary := func(id, object, target string) *boundaryState {
		return &boundaryState{place: atlas.Place{ID: id, TargetIDs: []string{target}, Boundary: &atlas.BoundaryFacts{ObjectID: object}}}
	}
	outs := []*boundaryState{boundary("out1", "shared", "a"), boundary("out2", "other", "a"), boundary("out3", "third", "a")}
	ins := []*boundaryState{boundary("self", "shared", "b"), boundary("remote", "remote", "b"), boundary("local", "local", "a"), boundary("fixture", "fixture", "f")}
	batches := r.peerBatches(outs, ins, targets)
	if len(batches) != 2 || len(batches[0].outs) != 1 || len(batches[1].outs) != 2 {
		t.Fatalf("different eligible dictionaries were not separated: %+v", batches)
	}
	if len(batches[0].ins) != 1 || batches[0].ins[0].place.ID != "remote" {
		t.Fatalf("same-object, local or fixture peer exposed: %+v", batches[0].ins)
	}
	if len(batches[1].ins) != 2 || batches[1].ins[0].place.ID != "self" || batches[1].ins[1].place.ID != "remote" {
		t.Fatalf("eligible peer lost: %+v", batches[1].ins)
	}
}

func TestPeerWindowWinnersCompeteBeforePublication(t *testing.T) {
	graph := twoTargetGraph(t)
	// More than one peer window. The provider picks the first candidate in
	// each window; only their final comparison can identify one counterpart.
	for i := 0; i < 55; i++ {
		graph.Places = append(graph.Places, atlas.Place{
			ID: fmt.Sprintf("bnd:svc/extra:%03d", i), Kind: atlas.PlaceBoundary,
			Path: "svc/api/h.go", LineNo: 100 + i, Parent: atlas.FileID("svc/api/h.go"), TargetIDs: []string{"svc"},
			Boundary: &atlas.BoundaryFacts{Source: "fact", Caller: "Extra", Direction: atlas.DirectionIn, GivenKind: atlas.BoundaryHTTPServer, Method: "GET", Values: []string{fmt.Sprintf("/extra/%d", i)}},
		})
	}
	atlas.SortPlaces(graph.Places)
	provider := &tableProvider{sameFor: map[string]string{"GET /api/levels/{id}": "yes"}}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	var blind, literal int
	for _, joint := range result.Atlas.Joints {
		if joint.Blind {
			blind++
			if joint.To.BoundaryID != "bnd:svc/api/h.go:10:http_server" || !joint.Possible {
				t.Fatalf("final source identity or uncertainty lost: %+v", joint)
			}
		} else {
			literal++
		}
	}
	if blind != 1 || literal != 1 {
		t.Fatalf("published intermediate window choices: blind=%d literal=%d", blind, literal)
	}
}

func TestMatchingSignatureBelongsToExactBoundaryObject(t *testing.T) {
	r := reader{places: map[string]atlas.Place{
		"file:service.go": {File: &atlas.FileFacts{Decls: []atlas.Decl{
			{Name: "Read", ObjectID: "unary", Signature: "func(Request) Response"},
			{Name: "Read", ObjectID: "stream", Signature: "func(Request, ResponseStream) error"},
		}}},
	}}
	state := &boundaryState{place: atlas.Place{Parent: "file:service.go", Boundary: &atlas.BoundaryFacts{ObjectID: "stream", Caller: "Read", CallerDoc: "Streams results."}}}
	side := r.sideOf(state, nil)
	if side.Signature != "func(Request, ResponseStream) error" || side.CallerDoc != "Streams results." {
		t.Fatalf("matching lost the exact protocol declaration: %+v", side)
	}
	state.place.Boundary.ObjectID = "unknown"
	if side := r.sideOf(state, nil); side.Signature != "" {
		t.Fatal("a same-named declaration supplied a guessed signature")
	}
}

func TestPeerEligibilityDifferencesDoNotSplitUnrelatedWindows(t *testing.T) {
	graph := twoTargetGraph(t)
	for i := 0; i < 64; i++ {
		object := fmt.Sprintf("shared-object-%d", i)
		for _, side := range []struct{ target, path, direction, kind string }{
			{"web", "web/src/client.ts", atlas.DirectionOut, atlas.BoundarySDK},
			{"svc", "svc/api/h.go", atlas.DirectionIn, atlas.BoundaryOther},
		} {
			graph.Places = append(graph.Places, atlas.Place{
				ID: fmt.Sprintf("bnd:%s:peer-%03d", side.target, i), Kind: atlas.PlaceBoundary, Path: side.path, LineNo: 100 + i,
				Parent: atlas.FileID(side.path), TargetIDs: []string{side.target},
				Boundary: &atlas.BoundaryFacts{Source: "fact", ObjectID: object, Caller: "F", Direction: side.direction, GivenKind: side.kind},
			})
		}
	}
	atlas.SortPlaces(graph.Places)
	provider := &tableProvider{sameFor: map[string]string{"GET /api/levels/{id}": "yes"}}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	for _, use := range result.Uses {
		if use.Stage == lines.StageJoints && use.Windows >= 100 {
			t.Fatalf("a self-reference split every peer window into single-row calls: %+v", use)
		}
	}
}

func TestCrossTargetCallsBecomeLinkJoints(t *testing.T) {
	graph := twoTargetGraph(t)
	// web's client calls svc's handler file directly: a seam between targets.
	graph.Edges = append(graph.Edges, atlas.Edge{
		From: atlas.FileID("web/src/client.ts"), To: atlas.FileID("svc/api/h.go"), Kind: "calls", Count: 1,
		Witnesses: []atlas.Witness{{Caller: "F", Callee: "F", Path: "web/src/client.ts", LineNo: 7}},
	})
	// A shared package under neither root that only svc happened to index is
	// not svc's: a call into it from web is no seam between the two.
	graph.Places = append(graph.Places,
		atlas.Place{
			ID: atlas.DirectoryID("shared/pb"), Kind: atlas.PlaceDirectory, Path: "shared/pb", Depth: 2,
			Parent: atlas.DirectoryID("."), TargetIDs: []string{"svc"}, Given: "shared/pb files",
			Directory: &atlas.DirectoryFacts{Dirs: []string{}, Files: []string{"p.go"}, FileCount: 1, TopBox: true},
		},
		atlas.Place{
			ID: atlas.FileID("shared/pb/p.go"), Kind: atlas.PlaceFile, Path: "shared/pb/p.go", Depth: 2,
			Parent: atlas.DirectoryID("shared/pb"), TargetIDs: []string{"svc"}, Given: "given shared/pb/p.go",
			File: &atlas.FileFacts{Callers: []string{atlas.FileID("web/src/client.ts")}, Callees: []string{}, Decls: []atlas.Decl{}},
		})
	graph.Edges = append(graph.Edges, atlas.Edge{
		From: atlas.FileID("web/src/client.ts"), To: atlas.FileID("shared/pb/p.go"), Kind: "imports", Count: 9,
		Witnesses: []atlas.Witness{{Caller: "F", Callee: "F", Path: "web/src/client.ts", LineNo: 8}},
	})
	sort.Slice(graph.Places, func(i, j int) bool { return graph.Places[i].ID < graph.Places[j].ID })
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Read(context.Background(), twoTargetOptions(t, graph, &tableProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	links := 0
	for _, joint := range result.Atlas.Joints {
		if joint.From.BoxID == "" {
			continue
		}
		links++
		if joint.From.TargetID != "web" || joint.From.BoxID != "web/src" || joint.To.TargetID != "svc" || joint.To.BoxID != "svc/api" || !joint.Same || joint.Possible {
			t.Fatalf("link joint: %+v", joint)
		}
	}
	if links != 1 {
		t.Fatalf("link joints: %d", links)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestFixturesJoinOnlyWithinTheirRoot(t *testing.T) {
	if fixtureRoot("testdata/acceptance/python-tutorial-game/backend") != "testdata/acceptance" {
		t.Fatal(fixtureRoot("testdata/acceptance/python-tutorial-game/backend"))
	}
	if fixtureRoot("services/billing") != "services" {
		t.Fatal(fixtureRoot("services/billing"))
	}
	if lines.FallbackRole("testdata/repositories/go", "module_library") != atlas.RoleFixture {
		t.Fatal("testdata is not a fixture")
	}
	if lines.FallbackRole("cmd/repomap", "executable") != atlas.RoleProduct {
		t.Fatal("cmd is not a product")
	}
}

func TestBudgetClosesDirectories(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{}
	opts := twoTargetOptions(t, graph, provider)
	opts.Budget = true
	result, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Atlas.Budget.OpenAsked {
		t.Fatal("budget mode did not ask open")
	}
	// The fake provider answers the first option, "yes": everything opens.
	if result.Atlas.Budget.DirsOpened == 0 || result.Atlas.Budget.FilesOpened != 7 {
		t.Fatalf("budget: %+v", result.Atlas.Budget)
	}
}

func TestClosedScopesSkipDescriptionsButRemainQuestionSources(t *testing.T) {
	for _, budget := range []bool{true, false} {
		t.Run(fmt.Sprintf("budget=%v", budget), func(t *testing.T) {
			graph := twoTargetGraph(t)
			var symbols []atlas.Place
			for i := range graph.Places {
				file := &graph.Places[i]
				if file.File == nil {
					continue
				}
				for rank, kind := range []string{"function", "type"} {
					decl := atlas.Decl{ObjectID: file.ID + ":" + kind, Name: "Example" + kind, Kind: kind, LineNo: 10 + rank, Doc: "Original declaration documentation."}
					file.File.Decls = append(file.File.Decls, decl)
					symbols = append(symbols, atlas.Place{ID: atlas.SymbolID(file.Path, decl.LineNo, decl.Name), Kind: atlas.PlaceSymbol,
						Path: file.Path, LineNo: decl.LineNo, Parent: file.ID, TargetIDs: file.TargetIDs,
						Symbol: &atlas.SymbolFacts{Decl: decl, Candidate: true, Rank: rank}})
				}
			}
			graph.Places = append(graph.Places, symbols...)
			atlas.SortPlaces(graph.Places)
			before, err := atlas.EncodeGraph(graph)
			if err != nil {
				t.Fatal(err)
			}
			provider := &tableProvider{openFor: map[string]string{
				"web": "no", "svc/core/c.go": "no", // Closed ancestor and closed file.
				"svc/db/d.go": "", "svc/jobs/j.go": "invalid", "svc/util": "", // Refusals must not close children.
			}, questionFor: make(map[string]table.Answer)}
			opts := twoTargetOptions(t, graph, provider)
			opts.Budget, opts.Through = budget, lines.StageSymbols
			opts.Executor.Enabled, opts.Executor.RootDir = true, t.TempDir()
			result, err := Read(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			closed := func(path string) bool {
				return budget && (strings.HasPrefix(path, "web/") || path == "svc/core/c.go")
			}
			for _, target := range result.Atlas.Targets {
				for _, box := range target.Boxes {
					for _, file := range box.Files {
						wantRequests := 3 // One file row, one callable row, one type row.
						if closed(file.Path) {
							wantRequests = 1
							if strings.HasPrefix(file.Path, "web/") {
								wantRequests = 0
							}
						}
						if got := provider.answers[file.Path]; got != wantRequests {
							t.Errorf("%s: sent %d rows, want %d", file.Path, got, wantRequests)
						}
						for _, symbol := range file.Symbols {
							if symbol.ObjectID == "" {
								continue
							}
							if described := strings.HasPrefix(symbol.Line, "Text for"); described == closed(file.Path) {
								t.Errorf("%s %s: closed=%v, description=%q", file.Path, symbol.Name, closed(file.Path), symbol.Line)
							}
						}
					}
				}
			}
			// Question-only reading recalls the same accepted decisions, yet its
			// original evidence still includes every closed callable and type.
			for _, chunk := range lines.QuestionRows(graph) {
				var refs []string
				for ref := range chunk.Anchors {
					refs = append(refs, ref)
				}
				sort.Strings(refs)
				provider.questionFor[chunk.Place.Path] = table.Answer{"relevance": "direct", "anchors": strings.Join(refs, " "), "why": "Inspect these original declarations."}
			}
			asked := make(map[string]int)
			for path, count := range provider.answers {
				asked[path] = count
			}
			opts.OwnerRunDir, opts.Through, opts.Questions = t.TempDir(), lines.StageQuestion, []string{"What declarations are available?"}
			questions, err := Read(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(provider.answers, asked) {
				t.Fatal("question-only reading made new description requests")
			}
			selected := make(map[string]bool)
			for _, question := range questions.Questions {
				for _, stop := range question.Stops {
					selected[stop.SubjectID] = true
				}
			}
			for _, symbol := range symbols {
				if !selected[symbol.Symbol.Decl.ObjectID] {
					t.Errorf("question lost original source: %s", symbol.ID)
				}
			}
			after, err := atlas.EncodeGraph(graph)
			if err != nil || string(before) != string(after) {
				t.Fatalf("reading changed original graph: %v", err)
			}
		})
	}
}
