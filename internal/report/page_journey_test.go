package report

import (
	"bytes"
	"fmt"
	"html/template"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// structureEdges builds one target's map and returns its part-to-part arrows.
func structureEdges(t *testing.T, index groupindex.Index) (*pageMap, []pageMapEdge) {
	t.Helper()
	section := &pageSection{ID: "section", programTargetID: index.Target.ID}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{index.Target.ID: section}}
	got := builder.buildMap(section)
	var edges []pageMapEdge
	for _, edge := range got.Edges {
		if edge.Scope == "structure" {
			edges = append(edges, edge)
		}
	}
	return got, edges
}

// The canvas draws quiet what GroupsIndex marks quiet (Connection.Quiet:
// wiring and calls into helpers, with their exceptions) and nothing else.
func TestTheMapQuietsWhatGroupsIndexMarksQuiet(t *testing.T) {
	connection := func(id, to string, quiet bool) groupindex.Connection {
		return groupindex.Connection{ID: id, From: groupindex.Endpoint{TargetID: "server", GroupID: "commands"}, To: groupindex.Endpoint{TargetID: "server", GroupID: to},
			FromSubjectID: "getCommand", ToSubjectID: "x", Label: "calls " + id, Phase: groupindex.PhaseRuntime, Quiet: quiet}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "server"}, Groups: []groupindex.Group{
		{ID: "commands", Title: "String commands", MemberSubjectIDs: []string{"getCommand"}},
		{ID: "reply", Title: "Client replies", MemberSubjectIDs: []string{"addReply"}},
		{ID: "keys", Title: "Keyspace", MemberSubjectIDs: []string{"lookupKey"}},
		{ID: "memory", Title: "Memory", MemberSubjectIDs: []string{"zmalloc"}},
	}, Connections: []groupindex.Connection{connection("x1", "reply", true), connection("x2", "keys", false), connection("x3", "memory", true), connection("x4", "memory", false)}}
	_, edges := structureEdges(t, index)
	canvas := map[string]bool{}
	for _, edge := range edges {
		canvas[edge.ConnectionID[strings.LastIndex(edge.ConnectionID, "/")+1:]] = edge.Init
	}
	if want := map[string]bool{"x1": true, "x2": false, "x3": true, "x4": false}; !maps.Equal(canvas, want) {
		t.Fatalf("quiet on the canvas %v, want %v", canvas, want)
	}
}

// An area's mark is its container's: purple when any part in it is the
// domain, the entry's area included (owner, 2026-09-27); the entry mark only
// on the entry's area when no domain part stands in it; none for an area of
// plain parts, whatever they take in. Each part keeps its own mark.
func TestAnAreaHoldingADomainPartIsPurple(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "server", Seeds: []programindex.TargetSeed{{ObjectID: "main"}}}, Groups: []groupindex.Group{
		{ID: "dispatch", Title: "Command dispatch", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"main"}},
		{ID: "shared", Title: "Shared objects", Lane: groupindex.LaneCore, Core: true, MemberSubjectIDs: []string{"incr"}},
		{ID: "store", Title: "Storage", Lane: groupindex.LaneCore, Core: true, MemberSubjectIDs: []string{"save"}},
		{ID: "log", Title: "Log", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"log"}},
		{ID: "net", Title: "Networking", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"listen"}},
		{ID: "loop", Title: "Event loop", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"poll"}},
		{ID: "cli", Title: "Command line", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"args"}},
		{ID: "help", Title: "Help text", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"usage"}},
	}, Containers: []groupindex.Container{
		{ID: "k1", Title: "Server runtime", Lane: groupindex.LaneTriggers, Core: true, GroupIDs: []string{"dispatch", "shared"}},
		{ID: "k2", Title: "Persistence", Lane: groupindex.LaneCore, Core: true, GroupIDs: []string{"store", "log"}},
		{ID: "k3", Title: "Core infrastructure", Lane: groupindex.LaneCore, GroupIDs: []string{"net", "loop"}},
		{ID: "k4", Title: "Startup", Lane: groupindex.LaneTriggers, GroupIDs: []string{"cli", "help"}},
	}}
	got, _ := structureEdges(t, index)
	lanes := map[string]string{}
	parts := map[string]string{}
	for _, node := range got.Nodes {
		if node.Branch == "area" {
			lanes[node.FullTitle] = node.Lane
		} else {
			parts[node.FullTitle] = node.Lane
		}
	}
	if lanes["Server runtime"] != "core" || lanes["Persistence"] != "core" || lanes["Core infrastructure"] != "" || lanes["Startup"] != "triggers" {
		t.Fatalf("area marks: %+v", lanes)
	}
	if parts["Networking"] != "triggers" || parts["Command dispatch"] != "triggers" {
		t.Fatalf("a part lost its own mark: %+v", parts)
	}
}

// Selecting GET lit fourteen parts in no order: every call among every part
// its handler reaches. The path draws every call into a part from a part
// reached earlier (none chosen by length) and no other, and the reading
// lists its parts by call depth from the handler.
func TestInputPathIsTheWitnessTraceOrderedByCallDepth(t *testing.T) {
	section := &pageSection{ID: "server", ShortLabel: "Server"}
	calls := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "server"}, Groups: []groupindex.Group{
		{ID: "dispatch", Title: "Command dispatch", MemberSubjectIDs: []string{"getCommand"}},
		{ID: "strings", Title: "String commands", MemberSubjectIDs: []string{"getGeneric", "lookup"}},
		{ID: "reply", Title: "Client replies", MemberSubjectIDs: []string{"addReply"}},
		{ID: "memory", Title: "Memory", MemberSubjectIDs: []string{"zmalloc"}},
		{ID: "keys", Title: "Keyspace", MemberSubjectIDs: []string{"db"}},
	}, Operations: []groupindex.Operation{{ID: "get", SubjectID: "getCommand", GroupID: "dispatch", Name: "get", Kind: "request", Source: "fact", Location: programindex.Location{Path: "redis.c", Line: 704, Column: 5}}},
		Subjects: []groupindex.Subject{
			{ID: "getCommand", Object: &groupindex.ObjectFacts{Name: "getCommand", Kind: programindex.ObjectFunction}},
			{ID: "getGeneric", Object: &groupindex.ObjectFacts{Name: "getGeneric", Kind: programindex.ObjectFunction}},
			{ID: "lookup", Object: &groupindex.ObjectFacts{Name: "lookup", Kind: programindex.ObjectFunction}},
			{ID: "addReply", Object: &groupindex.ObjectFacts{Name: "addReply", Kind: programindex.ObjectFunction}},
			{ID: "zmalloc", Object: &groupindex.ObjectFacts{Name: "zmalloc", Kind: programindex.ObjectFunction}},
			{ID: "db", Object: &groupindex.ObjectFacts{Name: "db", Kind: programindex.ObjectVariable}},
		},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("getCommand", "getGeneric"),
			calls("getCommand", "addReply"),
			calls("getGeneric", "lookup"),
			calls("lookup", "zmalloc"),
			// Memory is entered from two earlier parts: both calls draw.
			calls("addReply", "zmalloc"),
			// The reply part calls back into strings, reached as early: a
			// relation, not a step.
			calls("addReply", "lookup"),
			// The handler reads the keyspace itself: one step from it, before
			// what its callees call.
			{FromSubjectID: "getCommand", ToSubjectID: "db", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationReads, Resolution: programindex.ResolutionExact},
		}}
	groupindex.Derive(&index)
	builder := pageBuilder{indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{"server": section}}
	builder.subjects = make(map[string]subjectRef)
	for n, name := range []string{"getCommand", "getGeneric", "lookup", "addReply", "zmalloc", "db"} {
		builder.subjects[name] = subjectRef{subject: groupindex.Subject{ID: name, Object: &groupindex.ObjectFacts{Name: name, Location: &programindex.Location{Path: "redis.c", Line: n + 1, Column: 1}}}}
	}
	got := builder.buildOperationMap(section, &index)
	input := got.Nodes[0]
	want := strings.Join([]string{mapNodeID("dispatch"), mapNodeID("strings"), mapNodeID("reply"), mapNodeID("keys"), mapNodeID("memory")}, " ")
	if input.Trace != want {
		t.Fatalf("trace order\n got %s\nwant %s", input.Trace, want)
	}
	if input.Handler != "getCommand" {
		t.Fatalf("input lost its handler: %q", input.Handler)
	}
	steps := map[string]bool{}
	for _, edge := range got.Edges {
		if edge.Label != "implemented in" {
			steps[strings.TrimPrefix(edge.From, "n-")+">"+strings.TrimPrefix(edge.To, "n-")] = true
		}
	}
	for _, step := range []string{"dispatch>strings", "dispatch>reply", "strings>memory", "reply>memory", "dispatch>keys"} {
		if !steps[step] {
			t.Fatalf("trace step %s missing: %v", step, steps)
		}
	}
	if steps["reply>strings"] || len(steps) != 5 {
		t.Fatalf("the path is a neighbourhood again: %v", steps)
	}
}

// One "TCP endpoint" box took arrows from all three Redis programs, though
// for redis-cli that endpoint is redis-server and for redis-server its
// master: equal destination text proves no identity. Each program keeps its
// own destination frame, tile and arrow; frames naming the same destination
// only share a display group. One call written once is one tile (owner's
// decision a): Redis's three programs each drew a "DNS resolver" frame with
// gethostbyname, called at anet.c:146 by all three and at anet.c:115 by
// redis-benchmark and redis-cli; it stands once, with an arrow from each
// program.
func TestSystemMapKeepsEachProgramsOutsideDestinationItsOwn(t *testing.T) {
	part := func(target string) *pageMap {
		m := &pageMap{Nodes: []pageMapNode{{ID: "n-g1", FullTitle: "Networking"}}}
		scopeTargetMapIDs(m, target)
		return m
	}
	resolve := func(target, path string, line int) pageOutbound {
		return pageOutbound{ID: target + "-out-b108", Destination: "DNS resolver", External: "netdb.h.gethostbyname", MapGroup: "g1", Source: "model",
			Anchor: pageAnchor{Path: path, Line: line, Text: fmt.Sprintf("%s:%d", path, line)}}
	}
	view := func(server pageOutbound, benchmark, cli []pageOutbound) pageView {
		return pageView{Sections: []*pageSection{
			{ID: "t1", programTargetID: "t1", ShortLabel: "redis-server", Map: part("t1"), Outbound: []pageOutbound{server,
				{ID: "t1-out-b120", Destination: "Master", External: "sys/socket.h.connect", MapGroup: "g1", Source: "model", Anchor: pageAnchor{Text: "replication.c:40"}}}},
			{ID: "t2", programTargetID: "t2", ShortLabel: "redis-benchmark", Map: part("t2"), Outbound: benchmark},
			{ID: "t4", programTargetID: "t4", ShortLabel: "redis-cli", Map: part("t4"), Outbound: cli},
		}}
	}
	both := func(target string) []pageOutbound {
		second := resolve(target, "anet.c", 146)
		second.ID = target + "-out-b109"
		return []pageOutbound{resolve(target, "anet.c", 115), second}
	}
	drawn := func(m *pageMap) (map[string]pageMapNode, map[string]pageMapNode) {
		frames, tiles := map[string]pageMapNode{}, map[string]pageMapNode{}
		for _, node := range m.Nodes {
			switch {
			case node.Branch == "communication":
				frames[node.ID] = node
			case node.ItemKind == "External communication":
				tiles[node.ID] = node
			}
		}
		return frames, tiles
	}
	callers := func(m *pageMap, tile string) map[string]bool {
		from := map[string]bool{}
		for _, edge := range m.Edges {
			if edge.To == tile {
				from[edge.From] = true
			}
		}
		return from
	}

	// A call site in common: one frame, one tile, an arrow from each program.
	shared := view(resolve("t1", "anet.c", 146), both("t2"), both("t4"))
	got := shared.SystemMap()
	frames, tiles := drawn(got)
	frame, tile := frames["system-t1-out-b108-destination"], tiles["system-t1-out-b108"]
	if frame.Owner != "t1" || frame.Children != tile.ID || frame.DisplayGroup != "" || len(frames) != 2 || len(tiles) != 2 {
		t.Fatalf("one call written once is not one tile: frames %v tiles %v", frames, tiles)
	}
	if want := []string{"system-t2-out-b108", "system-t2-out-b109", "system-t4-out-b108", "system-t4-out-b109"}; !slices.Equal(tile.Aliases, want) {
		t.Fatalf("the other programs' records do not lead to the tile: %v", tile.Aliases)
	}
	if from := callers(got, tile.ID); len(from) != 3 || !from[targetMapNodeID("t2", "n-g1")] || !from[targetMapNodeID("t4", "n-g1")] {
		t.Fatalf("the arrows to the shared tile come from %v", from)
	}

	// Different call sites: each program keeps its own frame, tile and arrow.
	apart := view(resolve("t1", "anet.c", 115), []pageOutbound{resolve("t2", "benchmark.c", 20)}, []pageOutbound{resolve("t4", "cli.c", 30)})
	got = apart.SystemMap()
	frames, tiles = drawn(got)
	groups := map[string]bool{}
	for _, target := range []string{"t1", "t2", "t4"} {
		frame, tile := frames["system-"+target+"-out-b108-destination"], tiles["system-"+target+"-out-b108"]
		if frame.Owner != target || tile.Owner != target || frame.Children != tile.ID || frame.FullTitle != "DNS resolver" {
			t.Fatalf("program %s lost its own destination: frame %+v tile %+v", target, frame, tile)
		}
		groups[frame.DisplayGroup] = true
		if from := callers(got, tile.ID); len(from) != 1 || !from[targetMapNodeID(target, "n-g1")] {
			t.Fatalf("the arrows to %s's tile come from %v", target, from)
		}
	}
	if len(groups) != 1 || groups[""] {
		t.Fatalf("frames naming one destination stand in one display group: %v", groups)
	}
	if lone := frames["system-t1-out-b120-destination"]; lone.ID == "" || lone.DisplayGroup != "" || lone.DisplayGroupTitle != "" {
		t.Fatalf("a destination only one program names needs no group: %+v", lone)
	}
	if len(frames) != 4 || len(tiles) != 4 {
		t.Fatalf("frames %d, tiles %d", len(frames), len(tiles))
	}
	// The first screen showed "DNS resolver" three times side by side: the
	// page gives the group that text once, for its frame to carry, and each
	// frame keeps its own title for its reading.
	var page bytes.Buffer
	parsed, err := template.New("map").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/map.html", "templates/html/partials.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := parsed.ExecuteTemplate(&page, "map.html", got); err != nil {
		t.Fatal(err)
	}
	if carried := strings.Count(page.String(), `data-display-group-title="DNS resolver"`); carried != 3 {
		t.Fatalf("%d frames tell the page their group carries their destination text", carried)
	}
	for _, target := range []string{"t1", "t2", "t4"} {
		if frame := frames["system-"+target+"-out-b108-destination"]; frame.DisplayGroupTitle != "DNS resolver" || frame.FullTitle != "DNS resolver" {
			t.Fatalf("program %s's frame: %+v", target, frame)
		}
	}
	// Grouped without regard to letter case, "DNS Resolver" and "DNS resolver"
	// stand together but are not one text: each frame keeps its own heading.
	apart.Sections[2].Outbound[0].Destination = "DNS Resolver"
	together := 0
	for _, node := range apart.SystemMap().Nodes {
		if node.Branch == "communication" && node.DisplayGroup != "" {
			together++
			if node.DisplayGroupTitle != "" {
				t.Fatalf("differently spelled frames lost their own headings: %+v", node)
			}
		}
	}
	if together != 3 {
		t.Fatalf("%d differently spelled frames stand together", together)
	}
}

// A program calling one outside symbol from two places draws one tile. An
// input whose path reaches only the second call kept its entry under the
// second record's id, a tile no map draws: reading the drawn tile on echo's
// GET /users/:id lost "Why it appears".
func TestInputWitnessToAFoldedOutsideCallLeadsToItsTile(t *testing.T) {
	m := &pageMap{Nodes: []pageMapNode{{ID: "n-g1", FullTitle: "Repository"}}}
	scopeTargetMapIDs(m, "t1")
	reading := `{"parts":[{"part":"system-t1-out-b2","depth":2,"entered":[[0,1,0]]}],"decls":[{"name":"GetUser","source":"handler.go:20"},{"name":"Scan","source":"postgres.go:40","part":"system-t1-out-b2"}]}`
	m.Nodes = append(m.Nodes, pageMapNode{ID: "t1-o1", FullTitle: "GET /users/:id", Activation: "request", InputPath: reading})
	row := func(id, anchor string) pageOutbound {
		return pageOutbound{ID: id, Destination: "PostgreSQL", External: "database/sql.Row.Scan", MapGroup: "g1", Source: "model", Anchor: pageAnchor{Text: anchor}}
	}
	view := pageView{Sections: []*pageSection{{ID: "t1", programTargetID: "t1", ShortLabel: "api", Map: m,
		Outbound: []pageOutbound{row("t1-out-b1", "postgres.go:30"), row("t1-out-b2", "postgres.go:40")}}}}
	got := view.SystemMap()
	var input pageMapNode
	drawn := map[string]bool{}
	for _, node := range got.Nodes {
		drawn[node.ID] = true
		if node.ID == "t1-o1" {
			input = node
		}
	}
	if drawn["system-t1-out-b2"] || !drawn["system-t1-out-b1"] {
		t.Fatalf("one outside symbol is one tile: %v", drawn)
	}
	if !strings.Contains(input.InputPath, `"part":"system-t1-out-b1"`) || strings.Contains(input.InputPath, "system-t1-out-b2") {
		t.Fatalf("the path names a tile the map does not draw: %s", input.InputPath)
	}
}

// Find listed the outside call gethostbyname as a Part, three times, and
// could not find an undecided declaration.
func TestFindNamesOutsideCallsAndOffMapCodeByWhatTheyAre(t *testing.T) {
	script := systemJSPiece(t, "40-find.js", "  function nodeKind(", "  document.querySelectorAll('[data-system-map] [data-branch=\"component\"]')")
	runSystemJS(t, "function rmT(s){return s;}\n"+script+`
assert.deepEqual(nodeKind({itemKind:'External communication',title:'gethostbyname'}),{kind:'external',type:'External communication'});
assert.deepEqual(nodeKind({branch:'communication',itemKind:'External communication'}),{kind:'external',type:'External communication'});
assert.deepEqual(nodeKind({branch:'inputs',itemKind:'Inputs'}),{kind:'operation',type:'Inputs'});
assert.equal(nodeKind({branch:'component'}),null,'a component is listed once');
assert.deepEqual(nodeKind({}),{kind:'part',type:'Part'});
assert.deepEqual(nodeKind({branch:'area'}),{kind:'part',type:'Area'});
assert.deepEqual(nodeKind({activation:'request'}),{kind:'operation',type:'request'});
assert.ok(nodeSummary({title:'get',handler:'getCommand',summary:''}).includes('getCommand'),'with no summary an input is described by its handler');
assert.equal(nodeSummary({title:'serve',handler:'serve',summary:''}),'');
assert.equal(nodeSummary({title:'get',handler:'getCommand',summary:'Reads a key.'}),'Reads a key.');
`)
}

// Client connection handling listed redisClient and then fd, db, dictid,
// querybuf… as peers of its functions.
func TestTypeFieldsStayInsideTheirTypeInTheCodeList(t *testing.T) {
	builder := pageBuilder{subjects: map[string]subjectRef{}}
	object := func(id, name string, kind programindex.ObjectKind, owner string, line int) {
		builder.subjects[id] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: owner, Location: &programindex.Location{Path: "redis.c", Line: line, Column: 1}}}}
	}
	object("client", "redisClient", programindex.ObjectType, "", 303)
	object("fd", "fd", programindex.ObjectVariable, "client", 304)
	object("db", "db", programindex.ObjectVariable, "client", 305)
	object("create", "createClient", programindex.ObjectFunction, "", 2440)
	object("local", "c", programindex.ObjectVariable, "create", 2441)
	rows, _ := builder.memberChips("", []string{"client", "fd", "db", "create", "local"})
	if len(rows) != 1 {
		t.Fatalf("rows: %+v", rows)
	}
	var names []string
	for _, chip := range rows[0].Members {
		names = append(names, chip.Name)
	}
	if strings.Join(names, " ") != "redisClient createClient c" {
		t.Fatalf("fields listed as peers of the functions: %v", names)
	}
	fields := rows[0].Members[0].Fields
	if len(fields) != 2 || fields[0].Name != "fd" || fields[1].Name != "db" || rows[0].SymbolCount() != 5 {
		t.Fatalf("the type lost its fields: %+v", fields)
	}
}

func TestChosenInputIsEnteredAsItsPath(t *testing.T) {
	entrance := systemJSPiece(t, "29-operation-view.js", "function rmInputPath(", "(function(){")
	selectCode := systemJSPiece(t, "29-operation-view.js", "async function select(", "  function reset(")
	captionCode := systemJSPiece(t, "29-operation-view.js", "function renderCaption(", "  function focusNode(")
	runSystemJS(t, entrance+`
function rmT(s){return s;}
function rmEl(tag,className,text){return {tag,className,textContent:text||'',children:[],hidden:false,dataset:{},listeners:{},
  append(...c){this.children.push(...c);},appendChild(c){this.children.push(c);return c;},prepend(c){this.children.unshift(c);},
  replaceChildren(){this.children=[];},addEventListener(k,f){this.listeners[k]=f;},remove(){}};}
const node=(id,dataset)=>({id,dataset});
const get=node('get',{activation:'request',title:'get',inputTrace:'gone dispatch reply'}),dispatch=node('dispatch',{title:'Command dispatch'}),
 reply=node('reply',{title:'Client replies'}),ping=node('ping',{activation:'request',title:'ping'});
const byID={get,dispatch,reply,ping};
assert.deepEqual(rmInputPath(get,byID),['dispatch','reply'],'the path is the drawn parts of the saved trace');
assert.deepEqual(rmInputPath(dispatch,byID),[]);
const inspector=rmEl('div','map-inspector'),controls=rmEl('div','map-input-context'),caption=rmEl('div'),clear=rmEl('button'),colorKey=rmEl('span');
const map={querySelector(s){return s==='.map-inspector'?inspector:s==='.map-input-context'?controls:null;},showNode(n){this.shown=n.id;},clearMapPreview(){},
 readingState(){return {scope,operation:operation?.id||''};}};
const tiles=[];
let scope='',operation=null,inputAway=false,beforeInput=null,selectionRevision=0,searchValue='',filterValue='',visual=null,surface={clearHover(){},showInput(id){tiles.push(id);}};
const search={},filter={},ready=Promise.resolve(),focused=[];
function updateResults(){}function emit(){}function address(){}function emphasize(){renderCaption();}
function focusNode(n,center){focused.push({id:n.id,center});}
const showInput=()=>controls.children.flatMap(c=>c.children).find(c=>c.className==='system-input-start');
`+selectCode+captionCode+`
(async()=>{
 await select(get,true,null,true);
 assert.deepEqual(focused.at(-1),{id:'get',center:true},'the canvas frames the chosen input: its path');
 assert.equal(map.shown,'get','the reading stays on the chosen input');assert.equal(operation,get);
 assert.ok(showInput(),'the tile is one Show input away');
 await showInput().listeners.click();
 assert.deepEqual(tiles,['get'],'Show input frames the tile among its group, not the path or the wall');
 assert.equal(showInput(),undefined,'once its tile is framed there is nowhere to go');
 const before=focused.length;await select(get,true);
 assert.deepEqual(focused.slice(before),[],'its tile clicked on the canvas reads it with its path, the camera staying (owner, 2026-09-29)');
 assert.equal(operation,get,'the path stays pinned');
 assert.ok(showInput());
 await select(dispatch,true,null,true);
 assert.equal(operation,null,'reading a part leaves the input path (owner, 2026-09-28)');
 assert.equal(showInput(),undefined);
 assert.equal(controls.hidden,true,'and its "Leave input path" with it');
 await select(ping,true,null,true);
 assert.deepEqual(focused.at(-1),{id:'ping',center:true},'an input without a trace is entered as its tile');
 assert.equal(showInput(),undefined);
 const still=focused.length;await select(ping,true);
 assert.equal(focused.length,still,'a trace-less tile clicked on the canvas keeps the camera');
})().catch(error=>{console.error(error);process.exit(1);});
`)
}

// Find → pingCommand opened the "Not on the map" row under the sticky
// toolbar: the declaration was found and still not seen.
func TestCodeRowsFindOpensStandBelowTheToolbar(t *testing.T) {
	raw, err := reportTemplateFS.ReadFile("templates/css/39-modes.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range strings.Split(string(raw), "}") {
		selectors, body, ok := strings.Cut(rule, "{")
		if !ok || !strings.Contains(body, "scroll-margin-top") {
			continue
		}
		if strings.Contains(selectors, "[data-off-map-file]") && strings.Contains(selectors, ".symbol-index li") {
			return
		}
	}
	t.Fatal("an off-map declaration or source index line Find opens has no scroll margin below the toolbar")
}
