package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A name in the column breaks after a dot or a slash, words at their
// spaces, never at a hyphen; a piece too long for its line also after its
// underscores, else at its humps, and is never cut with "…" (reviewer,
// 2026-09-30), the whole name on hover; a callable written inline reads as
// an anonymous function in its home.
func TestAColumnNameBreaksAtItsBoundariesAndIsNeverCut(t *testing.T) {
	runSystemJS(t, readingViewElements+nameBreaksJS(t)+`
const pieces=e=>e.children.filter(c=>c.has&&c.has('map-name-piece')).map(c=>c.textContent);
const a=rmDotBreaks(rmEl('a','','FreqtradeBot.process_open_trade_positions'));
assert.deepEqual(pieces(a),['FreqtradeBot.','process_','open_','trade_','positions'],'a break after the dot, and in a piece longer than its line after its underscores');
assert.ok(!a.children.some(c=>c.has&&c.has('map-name-long')),'no piece is cut with …');
assert.equal(a.title,'FreqtradeBot.process_open_trade_positions','the whole name on its hover');
const humps=rmDotBreaks(rmEl('a','','zunionInterBlockClientOnSwappedKeys()'));
assert.deepEqual(pieces(humps),['zunion','Inter','Block','Client','On','Swapped','Keys()'],'a long piece with no underscore breaks at its humps');
const short=rmDotBreaks(rmEl('a','','Store.process_open'));
assert.deepEqual(pieces(short),['Store.','process_open'],'a piece that fits stays whole');
const b=rmDotBreaks(rmEl('b','','build_helpers/create_command_partials.py:'));
assert.deepEqual(pieces(b),['build_helpers/','create_command_partials.','py:']);
const c=rmDotBreaks(rmEl('span','','redis-benchmark'));
assert.deepEqual(pieces(c),['redis-benchmark'],'no break at a hyphen');
// A callable written inline comes named in words (Go's inlineWords); the
// column breaks them at their spaces and never reads a name back into
// parts, in any language (review, 2026-10-03).
for(const said of ['anonymous function in InfoCommand.Run','одна из двух анонимных функций в Main loop','anonymous function in Main loop (inline, 2)']){
  const d=rmDotBreaks(rmEl('span','',said));
  assert.equal(d.textContent,said,'the name is shown as given');
  assert.ok(d.children.some(x=>!x.has&&/^\s+$/.test(x.textContent)),'words break at their spaces');
}
assert.equal(typeof rmInlineText,'undefined','the column has no name parser');
`)
}

// An Inputs reading names each kind once, however many catalogues declare
// inputs of it (litestream's had read "Incoming requests" eight times), each
// catalogue's inputs under it; an input named by a sentence is prose.
func TestAnInputsReadingNamesEachKindOnce(t *testing.T) {
	code := nameBreaksJS(t) + systemJSPiece(t, "31-reading-column.js", "var rmLanguageNames=", "// Where a catalogue's inputs are declared") + "\nvar rmPendingKind='';\n"
	runSystemJS(t, readingViewElements+code+`
const inputs={a:{dataset:{title:'/metrics'}},b:{dataset:{title:'/'}},c:{dataset:{title:'Number of months to fetch data for'}},d:{dataset:{title:'sync-interval'}}};
const node={dataset:{owner:'t1',collection:JSON.stringify({groups:[{kind:'request',inputs:['a']},{kind:'request',inputs:['b','c']},{kind:'setting',inputs:['d']}]})}};
const context={nodeById:id=>inputs[id]||null,nodeByHref:()=>null,light(){},readNode(){}};
const view=rmCollectionView(context,node,rmPage.data(node,'collection'));
const kinds=view.children.filter(c=>c.has&&c.has('map-collection-group'));
assert.deepEqual(kinds.map(k=>k.dataset.kind),['request','setting'],'each kind once');
assert.deepEqual(kinds[0].all(c=>c.has('map-collection-catalogue')).length,2,'its catalogues under it');
const buttons=view.all(c=>c.tagName==='BUTTON');
assert.deepEqual(buttons.filter(b=>b.has('map-collection-prose')).map(b=>b.textContent),['Number of months to fetch data for'],'a sentence is prose, a path or a flag is code');
`)
}

// Two inputs of one kind sharing a name read apart by the words saved
// beside each (page_apart.go), shown as saved, never chosen by the page:
// the declaration declaring each, their own key words (freqtrade's version
// and version_main, both -V --version), a word each handler declares
// (etcd's POST /v3electionpb.Election/Campaign), each saying on its hover
// what it is and where it is written; inputs of two kinds are not taken for
// each other, and no input reads its code as written beside its name
// (owner's review, 2026-09-30: redis's "save strcasecmp(argv[0]").
func TestSameNamedInputsAreToldApartInWordsOnly(t *testing.T) {
	code := nameBreaksJS(t) + systemJSPiece(t, "31-reading-column.js", "var rmLanguageNames=", "// Where a catalogue's inputs are declared") + "\nvar rmPendingKind='';\n"
	runSystemJS(t, readingViewElements+code+`
const input=(title,kind,by,written)=>({dataset:{title,activation:kind,declaredBy:by,written}});
const inputs={a:input('save','request','cmdTable','{"save",saveCommand,1}'),b:input('save','setting','loadServerConfig','strcasecmp(argv[0],"save")'),
 c:input('dataformat_ohlcv','setting','SCHEMA_TRADE_REQUIRED','"dataformat_ohlcv"'),d:input('dataformat_ohlcv','setting','SCHEMA_BACKTEST_REQUIRED','"dataformat_ohlcv"'),
 e:input('-V --version','command','AVAILABLE_CLI_OPTIONS','"version": Arg("-V", "--version")'),f:input('-V --version','command','AVAILABLE_CLI_OPTIONS','"version_main": Arg("-V", "--version")')};
inputs.g=input('POST','request','','mux.Handle(http.MethodPost, pattern_Election_Campaign_0, func(…) {…})');inputs.h=input('POST','request','','mux.Handle(http.MethodPost, pattern_Lock_Lock_0, func(…) {…})');
const apart={c:[{word:'SCHEMA_TRADE_REQUIRED',of:'declared'}],d:[{word:'SCHEMA_BACKTEST_REQUIRED',of:'declared'}],e:[{word:'version',of:'key'}],f:[{word:'version_main',of:'key'}],
 g:[{word:'/v3electionpb.Election/Campaign',of:'handler',at:'gw.go:182'},{word:'RegisterElectionHandlerServer',of:'registered',at:'gw.go:173'}],h:[{word:'/v3lockpb.Lock/Lock',of:'handler',at:'lock.gw.go:90'}]};
const node={dataset:{owner:'t1',collection:JSON.stringify({groups:[{kind:'request',inputs:['a','g','h']},{kind:'setting',inputs:['b','c','d']},{kind:'command',inputs:['e','f']}],apart})}};
const context={nodeById:id=>inputs[id]||null,nodeByHref:()=>null,light(){},readNode(){}};
const view=rmCollectionView(context,node,rmPage.data(node,'collection'));
const where=view.all(c=>c.has&&c.has('map-reading-where'));
assert.deepEqual(where.map(c=>c.textContent),['/v3electionpb.Election/Campaign · RegisterElectionHandlerServer','/v3lockpb.Lock/Lock','SCHEMA_TRADE_REQUIRED','SCHEMA_BACKTEST_REQUIRED','version','version_main'],'the saved words beside each name: '+JSON.stringify(where.map(c=>c.textContent)));
assert.ok(where[0].title.includes('a word its handler declares, at gw.go:182')&&where[0].title.includes('the function registering it, at gw.go:173'),'each word says what it is and where: '+where[0].title);
assert.ok(!view.textContent.includes('strcasecmp')&&!view.textContent.includes('Arg(')&&!view.textContent.includes('pattern_'),'no code as written beside a name');
`)
}

// The callers of a destination's calls read as one "Called from", and where
// they are written as one "Made in": each record's reach joined by program
// and part, each declaration once.
func TestADestinationsCallersAreJoinedByPart(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "// Where several outside calls are made and reached from", "// An outside call's record as the column reads it")
	runSystemJS(t, readingViewElements+code+`
const d=(name,part)=>({name,href:'h#'+name,part});
const one={decls:[d('rdbSave','#p'),d('syncWithMaster','#r')],groups:[{part:'#p',title:'Persistence',decls:[{decl:0,kind:'calls'}]},{part:'#r',title:'Replication',decls:[{decl:1,kind:'calls'}]}]};
const two={decls:[d('rdbSave','#p'),d('rdbLoad','#p')],groups:[{part:'#p',title:'Persistence',decls:[{decl:0,kind:'calls'},{decl:1,kind:'calls'}]}]};
const joined=rmMergeReached([one,null,two]);
assert.deepEqual(joined.groups.map(g=>[g.title,g.decls.map(e=>joined.decls[e.decl].name)]),[['Persistence',['rdbSave','rdbLoad']],['Replication',['syncWithMaster']]]);
assert.equal(rmMergeReached([null,{decls:[],groups:[]}]),null,'no callers, no list');
assert.equal(joined.made,undefined,'no call said where it is made, no "Made in"');
one.made=[{part:'#n',title:'Networking',decls:[{decl:0,kind:'calls'}]}];two.made=[{part:'#n',title:'Networking',decls:[{decl:1,kind:'calls'}]}];
const made=rmMergeReached([one,two]);
assert.deepEqual(made.made.map(g=>[g.title,g.decls.map(e=>made.decls[e.decl].name)]),[['Networking',['rdbSave','rdbLoad']]],'where the calls are made, joined by part');
assert.deepEqual(rmMergeReached([{decls:[d('connect','#n')],groups:[],made:[{part:'#n',title:'Networking',decls:[{decl:0,kind:'calls'}]}]}]).groups,[],'made alone still reads');
`)
}

// A part written only in its program's tests, or a frame of such parts,
// reads its connections last (29-operation-view.js rmTestOnly).
func TestAFrameOfTestPartsIsTestOnly(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "function rmTestOnly(", "function rmInputPath(")
	runSystemJS(t, code+`
const node=(id,test,children)=>({id,dataset:{test:test?'true':'',children:children||''}});
const byID={a:node('a',true),b:node('b',true),c:node('c',false),tests:node('tests',false,'a b'),mixed:node('mixed',false,'a c')};
assert.equal(rmTestOnly(byID.a,byID),true);
assert.equal(rmTestOnly(byID.tests,byID),true,'an area all of whose parts are tests');
assert.equal(rmTestOnly(byID.mixed,byID),false);
assert.equal(rmTestOnly(byID.c,byID),false);
assert.equal(rmTestOnly(undefined,byID),false);
`)
}

// A part is test-only when every declaration with a place is written in
// its program's test sources (ProgramTarget TestSources).
func TestAPartOfTestSourcesIsTestOnly(t *testing.T) {
	at := func(path string) *programindex.Location {
		return &programindex.Location{Path: path, Line: 1, Column: 1}
	}
	builder := &pageBuilder{subjects: map[string]subjectRef{}, data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{
		{Target: programindex.Target{ID: "t1", TestSources: []string{"tests/conftest.py", "tests/test_bot.py"}}}}}}}
	for id, path := range map[string]string{"n1": "tests/conftest.py", "n2": "tests/test_bot.py", "n3": "freqtrade/bot.py"} {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: at(path)}}}
	}
	if !builder.testOnly("t1", groupindex.Group{MemberSubjectIDs: []string{"n1", "n2"}}) {
		t.Fatal("a part of test sources is not test-only")
	}
	if builder.testOnly("t1", groupindex.Group{MemberSubjectIDs: []string{"n1", "n3"}}) {
		t.Fatal("a part with product code is test-only")
	}
	if builder.testOnly("t1", groupindex.Group{MemberSubjectIDs: []string{"missing"}}) {
		t.Fatal("a part with no placed declaration is test-only")
	}
}
