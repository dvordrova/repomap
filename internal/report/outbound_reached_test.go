package report

import (
	"encoding/json"
	"fmt"
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
const section=rmReachedFrom(ctx,reached);
const parts=section.all(c=>c.has('map-reading-peer'));
assert.deepEqual(parts.map(p=>[p.all(c=>c.has('map-reading-program')).map(c=>c.textContent).join(''),p.all(c=>c.has('map-part-box')).map(c=>c.textContent).join(''),names(p)]),
 [['','Replication',['syncWithMaster()']],['','Server startup',['main()']],['redis-benchmark:','Benchmark clients',['createClient()']],['redis-cli:','Command line client',['cliConnect()']]],'callers by part box, other programs named');
assert.equal(section.tagName,'SECTION','a short list stands open');
assert.ok(!/\.c:\d|:\d{2,}/.test(section.textContent),'no line numbers: '+section.textContent);
parts[0].all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(-1),['syncWithMaster'],'a name reads its function');
// freqtrade's exchange calls are reached from 39 functions: the list folds
// under its count.
const many={decls:[],groups:[{part:'#t1-g2',title:'Replication',decls:[]}]};
for(let i=0;i<39;i++){many.decls.push({name:'caller'+i,kind:'function',part:'#t1-g2',href:'h#c'+i});many.groups[0].decls.push({decl:i,kind:'calls'});}
const folded=rmReachedFrom(ctx,many);
assert.equal(folded.tagName,'DETAILS','a long list folds');
assert.equal(folded.children[0].all(c=>c.has('map-reading-peer-count'))[0].textContent,'39','under its count');
assert.equal(rmReachedFrom(ctx,{decls:[],groups:[]}),null,'no callers, no list');
`)
	if regexp.MustCompile(`"(line|column)"`).MatchString(tile.Reached) {
		t.Fatalf("the page data writes a caller's line: %s", tile.Reached)
	}
}
