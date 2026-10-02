package report

import "testing"

// Find listed adlist.c's listCreate three times, once per program compiled
// from it, with nothing telling the results apart, and a declaration no
// part holds was a result of its own. One declaration (its file and line)
// is one result: every program that holds it is one "In program / part"
// link, a program that leaves it off its map links to the list saying so
// (Not on the map, or Not reachable from the entrypoints), and the result
// shows the declaration as its tile does, a type with its fields.
func TestFindListsOneResultPerDeclarationWithItsPrograms(t *testing.T) {
	code := systemJSPiece(t, "40-find.js", "  // One entry per declaration", "  function sectionsFor(")
	runSystemJS(t, `
const entries=[],codeEntries=new Map(),components={server:'redis-server',bench:'redis-benchmark',cli:'redis-cli'},groupNodes={};
function add(entry){entries.push(entry);}
function rmT(text){return text;}
function modelText(){return '';}
const sections={server:{id:'server'},bench:{id:'bench'},cli:{id:'cli'}};
function el(tag){return {tagName:tag.toUpperCase(),children:[],dataset:{},className:'',appendChild(c){this.children.push(c);return c;},
  get textContent(){return this.own!==undefined?this.own:this.children.map(c=>c.textContent).join('');},set textContent(v){this.own=v;},
  getAttribute(name){return name==='href'?this.href||null:null;},querySelector(selector){return selector==='.ln'?this.children.find(c=>c.className==='ln')||null:null;}};}
const document={documentElement:{lang:'en'},getElementById:id=>sections[id]||null,createElement:el,createTextNode:text=>({textContent:text})};
const map={};
const symbols=JSON.stringify([{name:'listCreate',kind:'function',href:'h/adlist.c#L41',text:'(): list *'},{name:'list',kind:'type',href:'h/adlist.h#L47'},{name:'head',kind:'field',owner:2,text:': listNode *'},{name:'tail',kind:'field',owner:2,text:': listNode *'}]);
const node=(title,owner)=>({dataset:{title,symbols,owner},closest:()=>map});
const item=(name,path,line)=>({name,source:{Href:'h/'+path+'#L'+line,Path:path,Line:line},fields:[]});
groupNodes.gs=node('Data structures','server');groupNodes.gb=node('Linked list','bench');
const members=new Map([[groupNodes.gs,[item('listCreate','adlist.c',41),item('list','adlist.h',47)]],[groupNodes.gb,[item('listCreate','adlist.c',41)]]]);
const repomapMembers={items:n=>members.get(n)||[]};
const chip=(text,href)=>({textContent:text,tagName:'A',getAttribute:name=>name==='href'?href:null,dataset:{},querySelector:()=>null});
const section=id=>sections[id];
function offMap(sectionID,path,text,list){
  const row={dataset:{path},querySelectorAll:()=>[]},s=section(sectionID),c=chip(text,'h/'+path);
  // What a program never reaches stands on the "What is missing" page,
  // marked with the program it is missing from.
  const unreached=list==='unreached';
  c.closest=selector=>selector==='[data-off-map-file]'?row:selector==='[data-gaps-of]'?(unreached?{dataset:{gapsOf:sectionID}}:null):selector==='[data-report-page]'?(unreached?{id:'repository-reference'}:s):selector==='.unreached-parts'?(unreached?{}:null):null;
  return c;
}
const off=[offMap('cli','adlist.c','listCreate:41','unreached'),offMap('server','redis.c','saveparam:301','catalog')];
document.querySelectorAll=selector=>off;
`+code+`
assert.equal(entries.length,3,'one result per declaration, not per program');
const create=entries.find(e=>e.title==='listCreate:41');
assert.deepEqual(create.memberships.map(m=>m.title),['redis-benchmark / Linked list','redis-cli / Not reachable from the entrypoints','redis-server / Data structures']);
assert.ok(create.memberships[0].node&&create.memberships[2].node&&!create.memberships[1].node,'a part opens on the map, the off-map list opens its row');
assert.deepEqual(create.sections.sort(),['bench','cli','server']);assert.equal(create.section,'');
assert.equal(create.component,'redis-benchmark, redis-cli, redis-server');
assert.ok(create.haystack.includes('redis-cli')&&create.haystack.includes('adlist.c'));
assert.deepEqual(tileRows(create),{head:'listCreate(): list *',fields:[]},'a function reads as its tile');
assert.deepEqual(tileRows(entries.find(e=>e.title==='list:47')),{head:'list',fields:['head: listNode *','tail: listNode *']},'a type reads with its fields');
const undecided=entries.find(e=>e.title==='saveparam:301');
assert.deepEqual(undecided.memberships.map(m=>m.title),['redis-server / Not on the map']);assert.equal(undecided.section,'server');
assert.deepEqual(tileRows(undecided),{head:'saveparam',fields:[]});
`)
}

// Find "Campaign" as an operation found nothing on etcd: its inputs were
// indexed by their titles alone ("POST"), the words telling them apart and
// their registration as written unread (control review, 2026-10-02). An
// input is found by the name its list gives it and by its words, and it
// opens itself.
func TestFindIndexesAnInputByItsToldApartName(t *testing.T) {
	code := systemJSPiece(t, "40-find.js", "  function nodeKind(d){", "  // A declaration off the map stays findable") +
		systemJSPiece(t, "40-find.js", "  document.querySelectorAll('[data-map-explorer] [data-node]')", "  // One entry per declaration")
	runSystemJS(t, `
const entries=[],components={server:'server (executable)'},groupNodes={};
function add(entry){entry.haystack=(entry.title+' '+(entry.additionalText||'')).toLowerCase();entries.push(entry);}
function rmT(text){return text;}
const map={displayedNode:n=>n,areaDescriptions:()=>[]};
const input={dataset:{title:'POST',activation:'request',owner:'server',handler:'RegisterElectionHandlerServer',written:'mux.Handle(http.MethodPost, pattern_Election_Campaign_0, func(…) {…})'},closest:()=>map,getAttribute:()=>null};
rmPage.data=((n,key)=>key==='apart'?[{word:'/v3electionpb.Election/Campaign'},{word:'RegisterElectionHandlerServer'}]:key==='inputPath'?{checks:[{name:'/v3/election/campaign'}]}:null);
const document={getElementById:id=>id==='server'?{id:'server',querySelector:()=>null}:null,querySelectorAll:()=>[input]};
`+code+`
assert.equal(entries.length,1);
assert.equal(entries[0].title,'POST /v3electionpb.Election/Campaign · RegisterElectionHandlerServer','named as its list names it');
assert.ok(['campaign','/v3/election/campaign','pattern_election_campaign_0'].every(word=>entries[0].haystack.includes(word)),'found by its words and its registration as written: '+entries[0].haystack);
assert.equal(entries[0].node,input,'it opens the input itself');
`)
}
