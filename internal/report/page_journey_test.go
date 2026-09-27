package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
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

// Redis's benchmark, client and dump checker serve nothing: every call they
// make is reached from main. Treating all of it as wiring hid every arrow of
// those three programs.
func TestProgramThatServesNothingDrawsTheArrowsItsMainReaches(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "bench"}, Groups: []groupindex.Group{
		{ID: "client", Title: "Benchmark client", MemberSubjectIDs: []string{"main"}},
		{ID: "strings", Title: "Dynamic strings", MemberSubjectIDs: []string{"sdscat"}},
	}, Connections: []groupindex.Connection{{ID: "x1", From: groupindex.Endpoint{TargetID: "bench", GroupID: "client"}, To: groupindex.Endpoint{TargetID: "bench", GroupID: "strings"}, FromSubjectID: "main", Label: "calls", Phase: groupindex.PhaseInit}}}
	_, edges := structureEdges(t, index)
	if len(edges) != 1 || edges[0].Init {
		t.Fatalf("a program without inputs hid its main's work as wiring: %+v", edges)
	}
	index.Operations = []groupindex.Operation{{ID: "o1", SubjectID: "serve", GroupID: "client", Kind: "request", Name: "get"}}
	_, edges = structureEdges(t, index)
	if len(edges) != 1 || !edges[0].Init {
		t.Fatalf("a serving program lost its wiring distinction: %+v", edges)
	}
}

// Server runtime held main and processCommand beside core parts; its core
// mark hid where execution comes in. An area whose parts only take requests
// is not the program's entry.
func TestAreaHoldingTheEntryShowsTheEntryMarkEvenWhenCore(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "server", Seeds: []programindex.TargetSeed{{ObjectID: "main"}}}, Groups: []groupindex.Group{
		{ID: "dispatch", Title: "Command dispatch", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"main"}},
		{ID: "shared", Title: "Shared objects", Lane: groupindex.LaneCore, Core: true, MemberSubjectIDs: []string{"incr"}},
		{ID: "store", Title: "Storage", Lane: groupindex.LaneCore, Core: true, MemberSubjectIDs: []string{"save"}},
		{ID: "log", Title: "Log", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"log"}},
		// Networking only listens: its listen/bind boundary put its area in the
		// triggers lane, and "Core infrastructure" was drawn as a second entry.
		{ID: "net", Title: "Networking", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"listen"}},
		{ID: "loop", Title: "Event loop", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"poll"}},
	}, Containers: []groupindex.Container{
		{ID: "k1", Title: "Server runtime", Lane: groupindex.LaneTriggers, Core: true, GroupIDs: []string{"dispatch", "shared"}},
		{ID: "k2", Title: "Persistence", Lane: groupindex.LaneTriggers, Core: true, GroupIDs: []string{"store", "log"}},
		{ID: "k3", Title: "Core infrastructure", Lane: groupindex.LaneTriggers, GroupIDs: []string{"net", "loop"}},
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
	if lanes["Server runtime"] != "triggers" || lanes["Persistence"] != "core" || lanes["Core infrastructure"] != "" {
		t.Fatalf("area marks: %+v", lanes)
	}
	// The part keeps its own mark; only its area is not the program's entry.
	if parts["Networking"] != "triggers" {
		t.Fatalf("the listening part lost its own mark: %+v", parts)
	}
}

// Selecting GET lit fourteen parts in no order: every call among every part
// its handler reaches. The path is the trace of shortest witnesses, and the
// reading lists its parts by call depth from the handler.
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
			{ID: "lookup", Object: &groupindex.ObjectFacts{Name: "lookup", Kind: programindex.ObjectFunction}},
			{ID: "db", Object: &groupindex.ObjectFacts{Name: "db", Kind: programindex.ObjectVariable}},
		},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("getCommand", "getGeneric"),
			calls("getCommand", "addReply"),
			calls("getGeneric", "lookup"),
			calls("lookup", "zmalloc"),
			// The reply part calls back into strings: a relation, not a step.
			calls("addReply", "lookup"),
			// The handler reads the keyspace itself: one step from it, before
			// what its callees call.
			{FromSubjectID: "getCommand", ToSubjectID: "db", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationReads, Resolution: programindex.ResolutionExact},
		}}
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
	for _, step := range []string{"dispatch>strings", "dispatch>reply", "strings>memory", "dispatch>keys"} {
		if !steps[step] {
			t.Fatalf("trace step %s missing: %v", step, steps)
		}
	}
	if steps["reply>strings"] || len(steps) != 4 {
		t.Fatalf("the path is a neighbourhood again: %v", steps)
	}
}

// One "TCP endpoint" box took arrows from all three Redis programs, though
// for redis-cli that endpoint is redis-server and for redis-server its
// master: equal destination text proves no identity. Each program keeps its
// own destination frame, tile and arrow; frames naming the same destination
// only share a display group.
func TestSystemMapKeepsEachProgramsOutsideDestinationItsOwn(t *testing.T) {
	part := func(target string) *pageMap {
		m := &pageMap{Nodes: []pageMapNode{{ID: "n-g1", FullTitle: "Networking"}}}
		scopeTargetMapIDs(m, target)
		return m
	}
	resolve := func(target string) pageOutbound {
		return pageOutbound{ID: target + "-out-b108", Destination: "DNS resolver", External: "netdb.h.gethostbyname", MapGroup: "g1", Source: "model", Anchor: pageAnchor{Text: "anet.c:115"}}
	}
	view := pageView{Sections: []*pageSection{
		{ID: "t1", programTargetID: "t1", ShortLabel: "redis-server", Map: part("t1"), Outbound: []pageOutbound{resolve("t1"),
			{ID: "t1-out-b120", Destination: "Master", External: "sys/socket.h.connect", MapGroup: "g1", Source: "model", Anchor: pageAnchor{Text: "replication.c:40"}}}},
		{ID: "t2", programTargetID: "t2", ShortLabel: "redis-benchmark", Map: part("t2"), Outbound: []pageOutbound{resolve("t2")}},
		{ID: "t4", programTargetID: "t4", ShortLabel: "redis-cli", Map: part("t4"), Outbound: []pageOutbound{resolve("t4")}},
	}}
	got := view.SystemMap()
	frames := map[string]pageMapNode{}
	tiles := map[string]pageMapNode{}
	for _, node := range got.Nodes {
		switch {
		case node.Branch == "communication":
			frames[node.ID] = node
		case node.ItemKind == "External communication":
			tiles[node.ID] = node
		}
	}
	groups := map[string]bool{}
	for _, target := range []string{"t1", "t2", "t4"} {
		frame, tile := frames["system-"+target+"-out-b108-destination"], tiles["system-"+target+"-out-b108"]
		if frame.Owner != target || tile.Owner != target || frame.Children != tile.ID || frame.FullTitle != "DNS resolver" {
			t.Fatalf("program %s lost its own destination: frame %+v tile %+v", target, frame, tile)
		}
		groups[frame.DisplayGroup] = true
		callers := map[string]bool{}
		for _, edge := range got.Edges {
			if edge.To == tile.ID {
				callers[edge.From] = true
			}
		}
		if len(callers) != 1 || !callers[targetMapNodeID(target, "n-g1")] {
			t.Fatalf("the arrows to %s's tile come from %v", target, callers)
		}
	}
	if len(groups) != 1 || groups[""] {
		t.Fatalf("frames naming one destination stand in one display group: %v", groups)
	}
	if lone := frames["system-t1-out-b120-destination"]; lone.ID == "" || lone.DisplayGroup != "" {
		t.Fatalf("a destination only one program names needs no group: %+v", lone)
	}
	if len(frames) != 4 || len(tiles) != 4 {
		t.Fatalf("frames %d, tiles %d", len(frames), len(tiles))
	}
}

// "redis.c.redisCommand.proc get in getCommand" was the get input's whole
// description: the registration's own words. The reading names the handler.
func TestInputIsReadByItsHandlerNotItsRegistrationWords(t *testing.T) {
	builder := pageBuilder{factsByID: map[string]facts.Fact{
		"a1": {ID: "a1", Kind: facts.KindRegistration, Text: "redis.c.redisCommand.proc", Values: []string{"get"}, Symbol: "getCommand"},
	}}
	given := groupindex.Operation{FactID: "a1", Summary: "redis.c.redisCommand.proc get in getCommand"}
	if got := builder.operationSummary(given); got != "" {
		t.Fatalf("raw registration words shown: %q", got)
	}
	explained := groupindex.Operation{FactID: "a1", Summary: "Returns the string value stored at a key."}
	if got := builder.operationSummary(explained); got != explained.Summary {
		t.Fatalf("a model explanation was dropped: %q", got)
	}
	interpreted := groupindex.Operation{Summary: "redis.c.redisCommand.proc get in getCommand"}
	if got := builder.operationSummary(interpreted); got != interpreted.Summary {
		t.Fatalf("an operation without its registration fact lost its line: %q", got)
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
assert.equal(nodeSummary({title:'get',handler:'getCommand',summary:''}),'handled by getCommand');
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

func TestPartReadingPutsItsCodeBeforeItsConnections(t *testing.T) {
	script := systemJSPiece(t, "29-operation-view.js", "function rmReadingOrder(", "(function(){")
	runSystemJS(t, script+`
function element(name){return {name,after(other){const list=card.children;list.splice(list.indexOf(other),1);list.splice(list.indexOf(this)+1,0,other);}};}
const card={children:['map-card-intro','group-connections','call-path','system-reaching-inputs','map-concepts','map-all-members','group-internal-connections'].map(element),
 querySelector(selector){const name=selector.replace(':scope>.','');return this.children.find(child=>child.name===name)||null;}};
rmReadingOrder(card);
assert.deepEqual(card.children.map(child=>child.name),['map-card-intro','call-path','map-concepts','map-all-members','group-connections','system-reaching-inputs','group-internal-connections']);
`)
}

// A call an input's code makes between two parts is work, even when the
// connection first names a relation main reaches. Drawing only the trace
// must not turn those arrows into hidden wiring.
func TestCallsOnAnInputsPathAreWorkNotWiring(t *testing.T) {
	calls := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	connection := func(id, from, to, subject string) groupindex.Connection {
		return groupindex.Connection{ID: id, From: groupindex.Endpoint{TargetID: "server", GroupID: from}, To: groupindex.Endpoint{TargetID: "server", GroupID: to}, FromSubjectID: subject, Label: "calls", Phase: groupindex.PhaseInit}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "server"}, Groups: []groupindex.Group{
		{ID: "dispatch", Title: "Command dispatch", MemberSubjectIDs: []string{"call", "init"}},
		{ID: "reply", Title: "Client replies", MemberSubjectIDs: []string{"addReply", "reset"}},
		{ID: "config", Title: "Configuration", MemberSubjectIDs: []string{"load"}},
		{ID: "memory", Title: "Memory", MemberSubjectIDs: []string{"zmalloc"}},
	}, Operations: []groupindex.Operation{{ID: "get", SubjectID: "call", GroupID: "dispatch", Name: "get", Kind: "request"}},
		StructuralEdges: []groupindex.StructuralEdge{calls("call", "addReply"), calls("addReply", "reset"), calls("load", "zmalloc")},
		Connections:     []groupindex.Connection{connection("x1", "dispatch", "reply", "init"), connection("x2", "config", "memory", "load")}}
	_, edges := structureEdges(t, index)
	init := map[string]bool{}
	for _, edge := range edges {
		init[edge.ConnectionID] = edge.Init
	}
	if init["x1"] || !init["x2"] {
		t.Fatalf("work on an input's path hidden as wiring, or wiring shown as work: %v", init)
	}
}

// Find → get framed get's tile in a wall of Redis's 98 inputs, and no arrow
// of its path was in sight: the collection stands outside its component and
// its tiles draw no arrow of their own. A chosen input, even its tile clicked
// on the canvas, is entered as its path, the reading on the input; Show input
// frames the tile among the inputs its handler's part takes.
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
const map={querySelector(s){return s==='.map-inspector'?inspector:s==='.map-input-context'?controls:null;},showNode(n){this.shown=n.id;},clearMapPreview(){}};
const tiles=[];
let scope='',operation=null,inputAway=false,selectionRevision=0,searchValue='',filterValue='',visual=null,surface={clearHover(){},showInput(id){tiles.push(id);}};
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
 assert.deepEqual(focused.slice(before),[{id:'get',center:true}],'its tile clicked on the canvas moves to its path');
 assert.ok(showInput());
 await select(dispatch,true,null,true);
 assert.ok(showInput(),'reading a part on the path offers the tile');
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
