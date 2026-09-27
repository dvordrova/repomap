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
const document={documentElement:{lang:'en'}};
const map={};
const symbols=JSON.stringify([{name:'listCreate',kind:'function',href:'h/adlist.c#L41',text:'(): list *'},{name:'list',kind:'type',href:'h/adlist.h#L47'},{name:'head',kind:'field',owner:2,text:': listNode *'},{name:'tail',kind:'field',owner:2,text:': listNode *'}]);
const node=(title)=>({dataset:{title,symbols},closest:()=>map});
const chip=(text,href)=>({textContent:text,tagName:'A',getAttribute:name=>name==='href'?href:null,dataset:{},querySelector:()=>null});
const section=id=>({id});
function inventory(sectionID,groupID,path,chips){
  const s=section(sectionID),group={id:groupID,closest:()=>s};
  return {closest:()=>group,querySelector:()=>({textContent:path+' · '+chips.length+' symbols'}),
    querySelectorAll:()=>chips.map(c=>({querySelector:selector=>selector==='.chip'?c:null,querySelectorAll:()=>[]}))};
}
groupNodes.gs=node('Data structures');groupNodes.gb=node('Linked list');
const files=[inventory('server','gs','adlist.c',[chip('listCreate:41','h/adlist.c#L41')]),inventory('bench','gb','adlist.c',[chip('listCreate:41','h/adlist.c#L41')]),
  inventory('server','gs','adlist.h',[chip('list:47','h/adlist.h#L47')])];
function offMap(sectionID,path,text,list){
  const row={dataset:{path},querySelectorAll:()=>[]},s=section(sectionID),c=chip(text,'h/'+path);
  c.closest=selector=>selector==='[data-off-map-file]'?row:selector==='[data-report-page]'?s:selector==='.unreached-parts'?(list==='unreached'?{}:null):null;
  return c;
}
const off=[offMap('cli','adlist.c','listCreate:41','unreached'),offMap('server','redis.c','saveparam:301','catalog')];
document.querySelectorAll=selector=>selector==='.group .inventory-file'?files:off;
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
