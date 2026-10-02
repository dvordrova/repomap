package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Redis's connect is written once, in anet.c, and all three programs make
// it: its one tile is read with where each program reaches it from, not
// with the wrapper one hop inside anet.c. The callers stand by part, the
// tile's own program first and each other program's part named with its
// program; a caller is its name, once however many places it calls from,
// and no line number is written (owner, 2026-09-29): the page data keeps
// the place for the name's hover only.
func TestAnOutsideTileIsReadWithTheCallersEachProgramReachesItFrom(t *testing.T) {
	builder := &pageBuilder{subjects: map[string]subjectRef{}, groupTitles: map[groupindex.Endpoint]string{}, links: pageLinks{sourceIDs: map[string]string{"redis.c": "f1", "redis-benchmark.c": "f2", "redis-cli.c": "f3", "anet.c": "f4"}}}
	programs := []struct{ target, label string }{{"t1", "redis-server"}, {"t2", "redis-benchmark"}, {"t4", "redis-cli"}}
	var sections []*pageSection
	for _, program := range programs {
		sections = append(sections, &pageSection{ID: program.target, programTargetID: program.target, ShortLabel: program.label, Map: &pageMap{Nodes: []pageMapNode{{ID: targetMapNodeID(program.target, "n-g1"), FullTitle: "Networking"}}}})
	}
	builder.sections = sections
	declare := func(target, id, name, path string, line int) {
		builder.subjects[subjectKey(target, id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Kind: programindex.ObjectFunction, Name: name, Location: &programindex.Location{Path: path, Line: line, Column: 1}}}}
	}
	site := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 5}
	}
	declare("t1", "n1", "syncWithMaster", "redis.c", 7190)
	declare("t1", "n2", "main", "redis.c", 9000)
	declare("t2", "n1", "createClient", "redis-benchmark.c", 330)
	declare("t4", "n1", "cliConnect", "redis-cli.c", 170)
	declare("t1", "n9", "anetTcpGenericConnect", "anet.c", 140)
	builder.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: "g1"}] = "Networking"
	builder.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: "g2"}] = "Replication"
	builder.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: "g3"}] = "Server startup"
	builder.groupTitles[groupindex.Endpoint{TargetID: "t2", GroupID: "g2"}] = "Benchmark clients"
	builder.groupTitles[groupindex.Endpoint{TargetID: "t4", GroupID: "g2"}] = "Command line client"
	reached := map[string][]groupindex.OutboundCaller{
		// syncWithMaster calls into anet.c from two places: one name.
		"t1": {{SubjectID: "n1", GroupID: "g2", Location: site("redis.c", 7219)}, {SubjectID: "n1", GroupID: "g2", Location: site("redis.c", 7250)}, {SubjectID: "n2", GroupID: "g3", Location: site("redis.c", 9010)}},
		"t2": {{SubjectID: "n1", GroupID: "g2", Location: site("redis-benchmark.c", 343)}},
		"t4": {{SubjectID: "n1", GroupID: "g2", Location: site("redis-cli.c", 179)}},
	}
	for _, section := range sections {
		call := groupindex.OutboundCall{ID: "b121", Destination: "TCP endpoint", External: "sys/socket.h.connect", ReachedFrom: reached[section.ID]}
		row := pageOutbound{ID: section.ID + "-out-b121", Destination: call.Destination, External: call.External, MapGroup: "g1", Source: "model",
			Anchor: pageAnchor{Path: "anet.c", Line: 158, Text: "anet.c:158"}}
		row.reached, row.program = builder.outboundReached(&groupindex.Index{Target: programindex.Target{ID: section.ID}}, section, call), componentTitle(section, sections)
		// Where the call is written: anet.c's anetTcpGenericConnect, in
		// Networking (redis-server's record only, here).
		if made, ok := builder.reachedName(&groupindex.Index{Target: programindex.Target{ID: section.ID}}, section, "n9", "g1", site("anet.c", 158), false); ok && section.ID == "t1" {
			row.made = &made
		}
		section.Outbound = []pageOutbound{row}
	}
	view := pageView{Sections: sections}
	var tile pageMapNode
	for _, node := range view.SystemMap().Nodes {
		if node.ItemKind == "External communication" && node.Branch == "" {
			if tile.ID != "" {
				t.Fatalf("one call written once is two tiles: %s and %s", tile.ID, node.ID)
			}
			tile = node
		}
	}
	var reading pageReached
	if err := json.Unmarshal([]byte(tile.Reached), &reading); err != nil {
		t.Fatalf("the tile carries no reading of its callers: %q, %v", tile.Reached, err)
	}
	var got []string
	for _, group := range reading.Groups {
		var names []string
		for _, end := range group.Decls {
			names = append(names, reading.Decls[end.Decl].Name)
		}
		got = append(got, fmt.Sprintf("%s[%s %s] %s", strings.TrimSuffix(group.Program+": ", ": "), group.Title, group.Part, strings.Join(names, ", ")))
	}
	want := []string{"[Replication #t1-g2] syncWithMaster", "[Server startup #t1-g3] main", "redis-benchmark[Benchmark clients #t2-g2] createClient", "redis-cli[Command line client #t4-g2] cliConnect"}
	if !slices.Equal(got, want) {
		t.Fatalf("the tile is reached from %q, want %q", got, want)
	}
	if len(reading.Made) != 1 || reading.Made[0].Title != "Networking" || reading.Made[0].Part != "#t1-g1" || len(reading.Made[0].Decls) != 1 ||
		reading.Decls[reading.Made[0].Decls[0].Decl].Name != "anetTcpGenericConnect" {
		t.Fatalf("the tile does not say where its call is made: %+v", reading.Made)
	}
	if sync := reading.Groups[0].Decls[0]; sync.Site == nil || sync.Site.At != "redis.c:7219 · 7250" || reading.Decls[sync.Decl].Part != "#t1-g2" {
		t.Fatalf("a caller's places are for its hover, its part for its reading: %+v %+v", sync, reading.Decls[sync.Decl])
	}

	// The column: each part in its box with its callers by name, another
	// program's part named with its program, no line numbers, and a name
	// reads its function.
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	raw, err := json.Marshal(reading)
	if err != nil {
		t.Fatal(err)
	}
	runSystemJS(t, readingViewElements+code+`
nodes['#t1-g2']={dataset:{title:'Replication',lane:'core'},getAttribute:()=>'#t1-g2'};
nodes['#t4-g2']={dataset:{title:'Command line client',lane:'triggers'},getAttribute:()=>'#t4-g2'};
const reached=`+string(raw)+`;
const made=reached.made;delete reached.made;
const section=rmReachedFrom(ctx,reached);
const parts=section.all(c=>c.has('map-reading-peer'));
assert.deepEqual(parts.map(p=>[p.all(c=>c.has('map-reading-program')).map(c=>c.textContent).join(''),p.all(c=>c.has('map-part-box')).map(c=>c.textContent).join(''),names(p)]),
 [['','Replication',['syncWithMaster()']],['','Server startup',['main()']],['redis-benchmark:','Benchmark clients',['createClient()']],['redis-cli:','Command line client',['cliConnect()']]],'callers by part box, other programs named');
assert.equal(section.tagName,'SECTION','a short list stands open');
assert.ok(!/\.c:\d|:\d{2,}/.test(section.textContent),'no line numbers: '+section.textContent);
parts[0].all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(-1),['syncWithMaster'],'a name reads its function');
// freqtrade's exchange calls are reached from 39 functions: the list folds,
// counting nothing (owner, 2026-09-29: no digits in the column).
const many={decls:[],groups:[{part:'#t1-g2',title:'Replication',decls:[]}]};
for(let i=0;i<39;i++){many.decls.push({name:'caller'+i,kind:'function',part:'#t1-g2',href:'h#c'+i});many.groups[0].decls.push({decl:i,kind:'calls'});}
const folded=rmReachedFrom(ctx,many);
assert.equal(folded.tagName,'DETAILS','a long list folds');
assert.ok(!/\d/.test(folded.children[0].textContent),'its heading counts nothing: '+folded.children[0].textContent);
assert.equal(rmReachedFrom(ctx,{decls:[],groups:[]}),null,'no callers, no list');
// Where the call is written stands first, "Made in", the same way; with no
// callers it stands alone and says no "Called from" stands.
reached.made=made;
const both=rmReachedFrom(ctx,reached);
assert.deepEqual(both.children.map(c=>c.className),['map-reading-side map-reading-in outbound-made','map-reading-side map-reading-in outbound-reached']);
const madeIn=both.children[0];
assert.equal(madeIn.children[0].textContent,'Made in');
assert.deepEqual(madeIn.all(c=>c.has('map-reading-peer')).map(p=>[p.all(c=>c.has('map-part-box')).map(c=>c.textContent).join(''),names(p)]),[['Networking',['anetTcpGenericConnect()']]]);
assert.equal(both.calledFrom,true);
const alone=rmReachedFrom(ctx,{decls:reached.decls,groups:[],made});
assert.deepEqual([alone.children.length,alone.calledFrom],[1,false],'made alone, no "Called from"');
`)
	if regexp.MustCompile(`"(line|column)"`).MatchString(tile.Reached) {
		t.Fatalf("the page data writes a caller's line: %s", tile.Reached)
	}
}

// The column reads an outside call's record (owner, 2026-09-29) from the
// row the component's page writes: no place is printed, the call is named
// once, a link to the line making it, and the names its address passes
// through are links too. The run from the program's own code stands only
// when no "Called from" says it by part.
func TestAnOutsideCallsRecordInTheColumnPrintsNoPlace(t *testing.T) {
	call := pageAnchor{Path: "anet.c", Line: 146, Href: "https://src/anet.c#L146", Text: "anet.c:146"}
	row := pageOutbound{ID: "t1-out-b120", KindLabel: "SDK", External: "netdb.h.gethostbyname", Source: "model", Anchor: call,
		Summary: "Resolves the master's host name. It blocks.",
		Side:    &pageCallSide{Program: "redis-server", Path: []pageSideStep{{Name: "syncWithMaster"}, {Name: "anetTcpConnect"}, {Name: "anetTcpGenericConnect"}}},
		Uses: []pageOutboundUse{{Frontier: "addr", Steps: []pageOutboundStep{{Name: "anetTcpGenericConnect", Anchor: call},
			{Name: "anetTcpConnect", Anchor: pageAnchor{Path: "anet.c", Line: 170, Href: "https://src/anet.c#L170", Text: "anet.c:170"}}}}}}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := parsed.ExecuteTemplate(&html, "outbound-row", row); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(html.String())
	if err != nil {
		t.Fatal(err)
	}
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
El.prototype.querySelector=function(s){return this.querySelectorAll(s)[0]||null;};
El.prototype.getAttribute=function(k){return this.attrs&&k in this.attrs?this.attrs[k]:null;};
// The row as the page writes it, in the harness's elements.
function parse(html){
  const root=new El('div'),entity=s=>s.replace(/&(amp|lt|gt|quot|#39|#34);/g,(_,e)=>({amp:'&',lt:'<',gt:'>',quot:'"','#39':"'",'#34':'"'})[e]);
  let at=root;
  for(const m of html.matchAll(/<(\/?)([a-zA-Z0-9]+)((?:\s+[^\s=>]+(?:="[^"]*")?)*)\s*>|([^<]+)/g)){
    if(m[4]!==undefined){at.appendChild(text(entity(m[4])));continue;}
    if(m[1]){at=at.parent||root;continue;}
    const el=new El(m[2]);el.attrs={};
    for(const a of m[3].matchAll(/([^\s=]+)(?:="([^"]*)")?/g)){
      const key=a[1],value=entity(a[2]||'');
      if(key==='class')el.className=value;else if(key.startsWith('data-'))el.dataset[key.slice(5).replace(/-(.)/g,(_,c)=>c.toUpperCase())]=value;else el.attrs[key]=value;
    }
    at.appendChild(el);if(!/^(br|wbr)$/i.test(m[2]))at=el;
  }
  return root.children.find(c=>c instanceof El);
}
const row=`+string(raw)+`;
const reached={decls:[{name:'syncWithMaster',kind:'function',part:'#core',href:'h#sync'}],groups:[{part:'#core',title:'Replication',decls:[{decl:0,kind:'calls'}]}]};
const place=/\.c:\d/;
const record=parse(row);
assert.ok(place.test(record.textContent),'the page\'s row prints its places');
rmOutboundRecord(record,rmReachedFrom(ctx,reached),'gethostbyname',true);
assert.ok(!place.test(record.textContent),'the column prints no place: '+record.textContent);
const named=record.all(c=>c.tagName==='A'&&c.textContent==='netdb.h.gethostbyname');
assert.equal(record.textContent.split('netdb.h.gethostbyname').length,2,'the call is named once: '+record.textContent);
assert.deepEqual(named.map(c=>[c.href,c.title]),[['https://src/anet.c#L146','anet.c:146']],'a link to its line, its place on hover');
assert.equal(record.all(c=>c.has('outbound-side')).length,0,'"Called from" says what the run from the program\'s code did');
assert.equal(record.all(c=>c.has('outbound-reached')).length,1,'where the program reaches it from');
assert.equal(record.all(c=>c.tagName==='DETAILS'||c.tagName==='SUMMARY').length,0,'the record stands open, no line of its own above it');
assert.ok(!record.textContent.includes('Resolves the master'),'the card\'s intro says the model\'s note');
assert.deepEqual(record.all(c=>c.has('outbound-chain'))[0].all(c=>c.tagName==='A').map(c=>[c.textContent,c.href]),
 [['anetTcpGenericConnect','https://src/anet.c#L146'],['anetTcpConnect','https://src/anet.c#L170']],'the names its address passes through are links');
// A record whose call is only said where it is made keeps the run from the
// program's code: no "Called from" says it by part.
const made=parse(row);
rmOutboundRecord(made,rmReachedFrom(ctx,{decls:reached.decls,groups:[],made:reached.groups}),'gethostbyname',true);
assert.equal(made.all(c=>c.has('outbound-side')).length,1,'the run stands beside "Made in"');
assert.equal(made.all(c=>c.has('outbound-made')).length,1);
const alone=parse(row);
rmOutboundRecord(alone,null,'gethostbyname',false);
assert.ok(!place.test(alone.textContent),'no place with no callers either');
assert.equal(alone.all(c=>c.has('outbound-side')).length,1,'with no "Called from", the run from the program\'s code stands');
assert.ok(alone.textContent.includes('Resolves the master'),'a note the card does not say stays');
`)
}
