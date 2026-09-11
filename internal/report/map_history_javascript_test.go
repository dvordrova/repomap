package report

import (
	"os/exec"
	"strings"
	"testing"
)

// Execute the owning navigation and history functions. Only DOM storage and
// layout completion are simulated: Back/Forward may happen before layout ends.
func TestMapRevealRecordsDestinationBeforeLayout(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the report's JavaScript regression")
	}
	read := func(name string) string {
		t.Helper()
		raw, err := reportTemplateFS.ReadFile("templates/js/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	part := func(script, start, end string) string {
		t.Helper()
		_, tail, found := strings.Cut(script, start)
		if !found {
			t.Fatalf("missing UI function %q", start)
		}
		body, _, found := strings.Cut(tail, end)
		if !found {
			t.Fatalf("missing UI function boundary %q", end)
		}
		return start + body
	}
	operations, modes, finder := read("29-operation-view.js"), read("45-modes.js"), read("40-find.js")
	harness := `
const assert=require('node:assert/strict');
// Layout is exercised in browser acceptance; this harness checks destination history.
function rmScrollToReading(node){node.scrollIntoView({block:'start'});}
const clone=value=>value==null?value:JSON.parse(JSON.stringify(value));
const page={id:'backend',querySelector(){return map;}},home=page;
function node(id,dataset){return {id,dataset,closest(selector){return selector==='[data-report-page]'?page:selector==='[data-map-explorer]'?map:null;}};}
const area=node('area',{branch:'area',title:'Backend entry'}),leaf=node('part',{title:'Application core'});
const directArea=node('direct-area',{branch:'area',title:'Application core'});
const operationNode=node('request',{activation:'request',title:'POST /api/level/run'});
const elements={area,part:leaf,'direct-area':directArea,request:operationNode,backend:page},listeners={};
const document={getElementById(id){return elements[id]||null;},
  addEventListener(name,fn){listeners[name]=fn;},dispatchEvent(e){listeners[e.type]?.(e);}};
const window={addEventListener(){}};
class CustomEvent{constructor(type,{detail}){this.type=type;this.detail=detail;}}
class Event{constructor(type){this.type=type;}}
let url=new URL('https://report.invalid/?mode=work#part'),entries=[{url:url.href,state:null}],position=0;
const location={get href(){return url.href;},get hash(){return url.hash;}};
const history={get state(){return clone(entries[position].state);},
  replaceState(state,unused,next){url=new URL(next);entries[position]={url:url.href,state:clone(state)};},
  pushState(state,unused,next){url=new URL(next);entries.splice(++position);entries.push({url:url.href,state:clone(state)});}};
let current=page,mode='work',question=null,term=null,searchIntent='',restoring=false;
const globalSearch=null;
function enclosing(n){return n===page?page:n?.closest('[data-report-page]');}
function locate(){return elements[location.hash.slice(1)];}
function showEntrance(){}
function setPage(n){current=enclosing(n);}
function savedElement(){return null;}function showReturn(){}function showLocation(){}
function close(){}
let layouts=[];
const map={explorerScope:'part',explorerMember:null,readingRestoring:false,visible:'part',
  classList:{contains(){return map.visible==='part';}},
  captureViewport(){return {scale:1,left:0,top:0};},restoreViewport(){},scrollIntoView(){},
  dispatchEvent(e){if(e.type==='repomap:reading')remember();}};
const requestAnimationFrame=fn=>setImmediate(fn);
` + part(modes, "function readingState(){", "  function savedElement(") +
		part(modes, "function address(node,replace){", "  // The former Learn/Work switch") +
		part(modes, "document.addEventListener('repomap:navigate'", "  // Layout commits") +
		part(modes, "function restore(){", "  // Restore the visit") +
		part(finder, "async function go(entry){", "  function action(") +
		part(finder, "document.addEventListener('click',function(e){var a=e.target.closest('a[data-question-map]')", "\n})();") + `
function install(){
  const byID={area,part:leaf,'direct-area':directArea,request:operationNode},nodes=[area,leaf,directArea,operationNode];
  let scope='part',operation=operationNode,pinned=true,mode='operations',search={value:'run_level'},trail=[],visit=null,scopeVisits=new Map();
  let revision=0;
  let historyRestoreHash='',historyRestoreKey='',historyRestorePromise=null,historyRevision=0;
  function abandonHistoryRestore(){historyRestoreHash='';historyRestoreKey='';historyRevision++;map.readingRestoring=false;}
  function displayed(id){return id==='direct-area'?'part':id;}function setScope(id){scope=displayed(id);}
  function followForeign(){return false;}function snapshot(){return {};}
  function readingDisclosures(){return null;}function restoreDisclosures(){}function revealChoice(){}function orient(){}
  function show(n){map.shown=n.id;}function resetScope(){scope='';}
  function address(n){document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:n}}));}
  const picker={value:'',dispatchEvent(){if(!this.value)map.explorerMember=null;map.dispatchEvent(new Event('repomap:reading'));}};
  map.querySelector=()=>picker;map.inspectConcept=()=>{map.explorerMember=null;};
  map.explainSource=source=>{map.explorerMember={owner:scope,...source};picker.value='0';map.dispatchEvent(new Event('repomap:reading'));};
  async function render(){
    const ticket=++revision,wanted=scope;
    if(wanted==='area')await new Promise(resolve=>layouts.push(resolve));
    if(ticket!==revision)return;
    map.visible=wanted;map.explorerScope=scope;
    if(scope)show(byID[scope]);
    map.dispatchEvent(new Event('repomap:reading'));
    return ticket;
  }
` + part(operations, "function scopeKey(id){", "    function showReturnPath(") +
		part(operations, "async function reveal(n,allUses,source){", "    map.exploreNode=") +
		part(operations, "map.readingState=function(){", "    map.displayedNode=") +
		part(operations, "map.findNode=function(n,source){", "    map.operationChoices=") +
		part(operations, "function hashChanged(){", "    document.addEventListener('click'") + `
  map.revealNode=reveal;map.openNode=open;map.hashChanged=hashChanged;
}
const settle=async()=>{await new Promise(setImmediate);await new Promise(setImmediate);};
async function traverse(step){
  position+=step;url=new URL(entries[position].url);
  restore(); // popstate capture
  restore(); // hashchange capture
  map.hashChanged(); // explorer hashchange, after the history owner
  await settle();
}
(async()=>{
  for(const caller of ['reveal-clear','reveal-keep','search','question-map']){
    url=new URL('https://report.invalid/?mode=work#part');entries=[{url:url.href,state:null}];position=0;layouts=[];install();
    const source={href:'https://example.invalid/blob/rev/backend/app.py#L75',open:'',key:'backend/app.py:75'};
    map.explainSource(source);remember();const original=clone(history.state.repomapReading.map.value);
    const allUses=caller!=='reveal-keep';
    const opening=caller==='search'?go({node:area,map}):caller==='question-map'?
      listeners.click({target:{closest(){return {dataset:{questionMap:'area'}};}},preventDefault(){}}):map.revealNode(area,allUses);
    assert.equal(location.hash,'#area');assert.equal(layouts.length,1,'area layout still pending');
    const saved=history.state.repomapReading.map.value;
    assert.equal(saved.scope,'area',caller+': new address must store its destination, not the previous part');
    assert.equal(saved.search,'','cleared search is part of the destination snapshot');
    assert.equal(saved.source,null,'previous part source cannot belong to the area');
    assert.equal(saved.operation,allUses?'':'request');assert.equal(saved.pinned,!allUses);
    assert.equal(saved.mode,allUses?'structure':'operations');
    await traverse(-1);
    assert.equal(map.readingState().scope,'part');assert.deepEqual(map.readingState().source,original.source);
    assert.equal(map.readingState().search,original.search,'Back retains the previous search');
    await traverse(1);
    layouts.splice(0).forEach(done=>done());await opening;await settle();
    assert.equal(location.hash,'#area');assert.equal(map.visible,'area');assert.equal(map.readingState().scope,'area');
    assert.equal(map.readingState().source,null);assert.equal(map.readingRestoring,false);
    assert.equal(history.state.repomapReading.map.value.scope,'area');
  }
  // A layout abandoned by Back must not replace the restored inspector or
  // declaration when its delayed completion eventually arrives.
  for(const caller of ['reveal','search']){
    url=new URL('https://report.invalid/?mode=work#part');entries=[{url:url.href,state:null}];position=0;layouts=[];install();
    const original={href:'https://example.invalid/blob/rev/backend/app.py#L75',open:'',key:'backend/app.py:75'};
    const obsolete={href:'https://example.invalid/blob/rev/backend/other.py#L9',open:'',key:'backend/other.py:9'};
    map.explainSource(original);remember();
    const opening=caller==='search'?go({node:area,map,codeSource:obsolete}):map.revealNode(area,true,obsolete);
    await traverse(-1);assert.equal(map.shown,'part');assert.deepEqual(map.readingState().source,original);
    layouts.splice(0).forEach(done=>done());await opening;await settle();
    assert.equal(location.hash,'#part');assert.equal(map.visible,'part');
    assert.equal(map.shown,'part',caller+': abandoned reveal cannot replace the inspector');
    assert.deepEqual(map.readingState().source,original,caller+': abandoned reveal cannot replace its declaration');
    assert.deepEqual(history.state.repomapReading.map.value.source,original);
  }
  // Ordinary direct-node opening follows the same destination-first contract.
  url=new URL('https://report.invalid/?mode=work#part');entries=[{url:url.href,state:null}];position=0;layouts=[];install();remember();
  const opening=map.openNode('area');assert.equal(history.state.repomapReading.map.value.scope,'area');
  layouts.splice(0).forEach(done=>done());await opening;assert.equal(map.visible,'area');
  // Source-bearing search keeps its exact declaration after its map is ready.
  const source={href:'https://example.invalid/blob/rev/backend/app.py#L75',open:'',key:'backend/app.py:75'};
  await go({node:leaf,map,codeSource:source});assert.deepEqual(map.readingState().source,source);
  assert.deepEqual(history.state.repomapReading.map.value.source,source);
  // Display normalization must not replace the original navigation address.
  await map.revealNode(directArea,true);assert.equal(location.hash,'#direct-area');
  assert.equal(history.state.repomapReading.map.value.scope,'part');
  // Non-map question/term results still use their ordinary exact destination.
  const textDestination=node('question',{});textDestination.scrollIntoView=()=>{textDestination.scrolled=true;};
  await go({destination:textDestination});assert.equal(location.hash,'#question');assert.equal(textDestination.scrolled,true);
})().catch(error=>{console.error(error);process.exitCode=1;});
`
	command := exec.CommandContext(t.Context(), node, "--eval", harness)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("map destination history: %v\n%s", err, output)
	}
}
