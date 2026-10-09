package report

import (
	"os/exec"
	"strings"
	"testing"
)

func systemJSPiece(t *testing.T, name, start, end string) string {
	t.Helper()
	raw, err := reportTemplateFS.ReadFile("templates/js/" + name)
	if err != nil {
		t.Fatal(err)
	}
	_, tail, ok := strings.Cut(string(raw), start)
	if !ok {
		t.Fatalf("missing %s", start)
	}
	body, _, ok := strings.Cut(tail, end)
	if !ok {
		t.Fatalf("missing %s", end)
	}
	return start + body
}

// jsPageDataStandIn reads a value a harness writes into an element's
// dataset as JSON, where the page refers to it in its page data
// (10-ui.js rmPage, page_data_table.go).
const jsPageDataStandIn = `var rmPage={data:function(element,name){var value=element&&element.dataset?element.dataset[name]:undefined;return value===undefined||value===''?null:JSON.parse(value);},link:function(value){return value;}};
`

func runSystemJS(t *testing.T, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	out, err := exec.CommandContext(t.Context(), node, "-e", "const assert=require('node:assert/strict');\n"+jsPageDataStandIn+script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// A new selection records its destination immediately. An unfinished initial
// layout cannot overwrite a later Back/Forward visit or its source selection.
func TestMapRevealRecordsDestinationBeforeLayout(t *testing.T) {
	selectCode := systemJSPiece(t, "29-operation-view.js", "async function select(", "  function reset(")
	stateCode := systemJSPiece(t, "29-operation-view.js", "map.readingState=function(){", "  map.revealNode=")
	resumeCode := systemJSPiece(t, "29-operation-view.js", "map.resumeExploration=function(){", "  map.readingState=")
	entranceCode := systemJSPiece(t, "29-operation-view.js", "function rmInputPath(", "(function(){")
	runSystemJS(t, entranceCode+`
let surface=null,scope='a',operation=null,inputAway=false,beforeInput=null,selectionRevision=0,searchValue='',filterValue='',savedAddress;
const a={id:'a',dataset:{}},b={id:'b',dataset:{}},op={id:'op',dataset:{activation:'command'}},byID={a,b,op};
let finish;const ready=new Promise(resolve=>finish=resolve),search={},filter={};
const map={explorerMember:{owner:'a',key:'source-a'},captureViewport(){return {scale:1,left:20,top:40};},
 showNode(n){this.shown=n.id;},clearInspection(){this.shown=null;},explainSource(s){this.explorerMember=s;},inspectConcept(){this.explorerMember=null;},restoreViewport(v){this.viewport=v;}};
let emphasisCount=0;function emphasize(){emphasisCount++;}function updateResults(){}function emit(){}function focusNode(){}
function address(n){savedAddress={id:n.id,state:map.readingState()};}
`+selectCode+resumeCode+stateCode+`
(async()=>{
 search.value=searchValue='input search';filter.value=filterValue='input';
 const pending=select(b,true,{key:'source-b'},true);
 assert.equal(savedAddress.id,'b');assert.equal(savedAddress.state.scope,'b');assert.equal(savedAddress.state.source,null);
 assert.equal(savedAddress.state.search,'');assert.equal(savedAddress.state.filter,'','a chosen destination closes the catalogue and its unrelated highlights');
 assert.equal(search.value,'');assert.equal(filter.value,'');
 const restored=map.restoreReadingState({scope:'a',operation:'op',source:{key:'source-a'},viewport:{scale:1.2,left:32,top:65}});
 finish();await Promise.all([pending,restored]);
 assert.equal(map.shown,'a');assert.equal(map.explorerMember.key,'source-a');assert.equal(map.readingState().operation,'op');assert.equal(map.viewport.left,32);
 const beforeClick=emphasisCount;await select(b,true);assert.equal(map.readingState().operation,'','reading a part leaves the input path (owner, 2026-09-28)');
 assert.equal(emphasisCount-beforeClick,1,'one click updates selection once');
 await map.restoreReadingState({scope:'',operation:'',viewport:{overview:true,x:16,y:16,zoom:.6}});
 assert.equal(map.shown,null,'Back to the first visit clears the previous part reading');
 assert.equal(map.viewport.overview,true,'the unselected overview and its camera also return');
})();
`)
}

func TestMapInputRevealPreservesItsExactCardIdentity(t *testing.T) {
	focusCode := systemJSPiece(t, "29-operation-view.js", "function focusNode(", "  async function select(")
	runSystemJS(t, `
const projection={inputOwner:{input:'handler'}};
const calls=[],surface={focus(id,center){calls.push({id,center});}};
`+focusCode+`
focusNode({id:'input'},true);
assert.deepEqual(calls,[{id:'input',center:true}],'the camera receives the exact named input entrance');
focusNode({id:'handler'},false);assert.equal(calls[1].center,false,'visible part selection preserves the camera');

`)
}

func TestInputCatalogueSelectionKeepsItsActualComponent(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "map.componentSelection=function(){", "  map.selectComponent=")
	runSystemJS(t, `
const nodes=[{id:'component-a',dataset:{branch:'component',owner:'a',title:'Same title'}},
 {id:'component-b',dataset:{branch:'component',owner:'b',title:'Same title'}},
 {id:'catalogue',dataset:{branch:'inputs',owner:'a',title:'Same title'}},
 {id:'input',dataset:{activation:'command',owner:'a'}},
 {id:'unknown',dataset:{activation:'request',owner:'missing',title:'Same title'}},
 {id:'not-a-component',dataset:{branch:'communication',owner:'missing'}},
 {id:'part',dataset:{owner:'b'}}];
const byID=Object.fromEntries(nodes.map(n=>[n.id,n])),parents={input:'catalogue',part:'component-b'};
const before=JSON.stringify(parents),viewport={componentsOpen:false},map={captureViewport:()=>viewport};
let scope='',operation=null;
function path(id){return parents[id]?[parents[id],id]:[id];}
`+code+`
for(const id of ['input','catalogue']){scope=id;assert.equal(map.componentSelection(),'a','an input catalogue identifies its actual target even while the component contents are closed');}
scope='';operation=byID.input;assert.equal(map.componentSelection(),'a','a pinned input retains its target');
scope='unknown';assert.equal(map.componentSelection(),'','a matching title or non-component owner cannot establish target identity');
scope='part';assert.equal(map.componentSelection(),'','ordinary overview selection still shows All');
viewport.componentsOpen=true;assert.equal(map.componentSelection(),'b','an inspected part keeps its own component rather than borrowing the pinned input owner');
assert.equal(JSON.stringify(parents),before,'selection never creates a fake containment parent');
`)
}

func TestMapClosingDetailsKeepsCameraAndPinnedInput(t *testing.T) {
	closeCode := systemJSPiece(t, "29-operation-view.js", "map.closeDetails=function(){", "  clear.addEventListener")
	runSystemJS(t, `
let selectionRevision=0,scope='part',operation={id:'input'},addressed;
const viewport={x:-123.5,y:222,zoom:.5,componentsOpen:false};
const map={viewport,clearInspection(){this.closed=true;}};
const surface={clearHover(){}};
function emphasize(){} function emit(){} function address(n){addressed=n;}
`+closeCode+`
map.closeDetails();
assert.equal(scope,'');assert.equal(operation.id,'input');assert.equal(addressed,operation);
assert.equal(map.closed,true);assert.equal(map.viewport,viewport);
assert.equal(viewport.componentsOpen,false);assert.equal(selectionRevision,1);
operation=null;map.closeDetails();assert.equal(addressed,null);
`)
}

// Back emits popstate followed by hashchange. Once the complete visit was
// restored, the second event must not open the detail layout and recenter it.
// A fresh deep link still needs its destination revealed.
func TestMapHashChangeKeepsRestoredVisit(t *testing.T) {
	hashCode := systemJSPiece(t, "29-operation-view.js", "function hashChanged(){", "  document.addEventListener('click'")
	runSystemJS(t, `
const part={id:'part'},input={id:'input'},home={id:'overview'},repo={id:'repository-map'};
const nodes={part,input,overview:home,'repository-map':repo};
const document={getElementById:id=>nodes[id]},location={hash:'#part'},history={state:null};
const map={readingRestoring:false,closest:()=>home};
let selected=[],resets=0;
function mapped(n){return n===part||n===input?n:null;}
function select(...args){selected.push(args);}
function reset(){resets++;}
`+hashCode+`
history.state={repomapReading:{map:{page:'overview',value:{scope:'part',viewport:{overview:true,x:24.625,y:.689453,zoom:.75}}}}};
hashChanged();assert.equal(selected.length,0,'restored overview and panned camera must remain intact');
location.hash='#input';history.state.repomapReading.map.value={scope:'part',operation:'input'};
hashChanged();assert.equal(selected.length,0,'input visit preserves its separately inspected part');
history.state=null;hashChanged();assert.equal(selected.length,1,'fresh input deep link must open');
assert.equal(selected[0][0],input);assert.equal(selected[0][3],true);
history.state={repomapReading:{map:{page:'other-page',value:{scope:'input'}}}};
hashChanged();assert.equal(selected.length,2,'another map cannot claim this visit');
map.readingRestoring=true;hashChanged();assert.equal(selected.length,2,'pending restoration owns the destination');
map.readingRestoring=false;location.hash='#overview';hashChanged();assert.equal(resets,1);
`)
}

func TestWholeMapClearsReadingAndFiltersButKeepsHistoryVisit(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "map.showWholeMap=async function(){", "  map.componentSelection=")
	runSystemJS(t, `
const map={},search={},filter={};let searchValue='old',filterValue='part',resetCount=0,emitted=0;
const ready=Promise.resolve(),events=[],surface={overview:async()=>events.push('camera')};
const document={querySelector:()=>({restoreSearch:()=>events.push('clear global search')})};
function updateResults(){events.push('results');}function reset(){resetCount++;}function address(n,visit){events.push(['address',n,visit]);}function emit(){emitted++;}
`+code+`
(async()=>{await map.showWholeMap();assert.equal(resetCount,1);assert.equal(searchValue,'');assert.equal(filterValue,'');assert.deepEqual(events,['results',['address',null,true],'clear global search','camera']);assert.equal(emitted,1);})();
`)
}

func TestSearchLocatorIsCompactAndExactCodeWinsOverAnswerBody(t *testing.T) {
	code := systemJSPiece(t, "40-find.js", "  function rank(", "  function render(")
	runSystemJS(t, `
const shown=[];function appendText(parent,tag,text,cls){shown.push({tag,text,cls});}
`+code+`
assert.ok(rank({title:'Config:10',kind:'code'},'config')<rank({title:'How do I run it?',kind:'question'},'config'));
description({summary:'Intro '.repeat(100)+'config setting '+'After '.repeat(100)}, {}, ['config']);
assert.equal(shown.length,1);assert.equal(shown[0].tag,'p');assert.ok(shown[0].text.length<160);assert.ok(shown[0].text.includes('config'));assert.equal(shown[0].cls,'find-result-summary');
`)
}

// "Leave input path" returns to what was read before the input and to its
// camera (owner, 2026-09-28): it had gone to the repository's summary, and
// a reader needed the Components list to get back to redis-server.
func TestLeavingAnInputPathReturnsToThePreviousReading(t *testing.T) {
	selectCode := systemJSPiece(t, "29-operation-view.js", "async function select(", "  function reset(")
	stateCode := systemJSPiece(t, "29-operation-view.js", "map.readingState=function(){", "  map.revealNode=")
	resumeCode := systemJSPiece(t, "29-operation-view.js", "map.resumeExploration=function(){", "  map.readingState=")
	leaveCode := systemJSPiece(t, "29-operation-view.js", "  clear.addEventListener('click',function(){", "  function filterChanged(){")
	entranceCode := systemJSPiece(t, "29-operation-view.js", "function rmInputPath(", "(function(){")
	runSystemJS(t, entranceCode+`
let surface=null,scope='',operation=null,inputAway=false,beforeInput=null,selectionRevision=0,searchValue='',filterValue='',addresses=[],camera={x:1,y:2,zoom:3};
const server={id:'server',dataset:{}},get={id:'get',dataset:{activation:'request'}},set={id:'set',dataset:{activation:'request'}},byID={server,get,set};
const ready=Promise.resolve(),search={},filter={};let listener;const clear={addEventListener(kind,f){listener=f;}};
const map={captureViewport(){return camera;},showNode(n){this.shown=n.id;},clearInspection(){this.shown=null;},explainSource(){},inspectConcept(){},restoreViewport(v){camera=v;}};
function emphasize(){}function updateResults(){}function emit(){}function focusNode(){}function reset(){scope='';operation=null;map.shown=null;}
function address(n,visit){addresses.push([n?n.id:'home',!!visit]);}
`+selectCode+resumeCode+stateCode+leaveCode+`
(async()=>{
 await select(server,true);
 await select(get,true,null,true);camera={x:9,y:9,zoom:9};
 await select(set,true,null,true);
 assert.equal(operation,set);
 listener();await new Promise(resolve=>setTimeout(resolve,0));
 assert.equal(operation,null,'the input path is left');
 assert.equal(scope,'server','the reading before the first input returns');assert.equal(map.shown,'server');
 assert.deepEqual(camera,{x:1,y:2,zoom:3},'and its camera');
 assert.deepEqual(addresses.at(-1),['server',true],'a new visit, so Back returns to the input');
})().catch(error=>{console.error(error);process.exit(1);});
`)
}

// A reading's folds stay open when the reader comes back to it by a click
// (owner, 2026-09-28: Persistence's "Called from" tree closed again on every
// return), each known by its words, whatever the reading shows around it.
func TestAReadingsFoldsAreRememberedByTheirWords(t *testing.T) {
	code := systemJSPiece(t, "30-map.js", "function rmFolds(card){", "// \"Expand all\" opens")
	runSystemJS(t, code+`
const fold=(words,open)=>({open,closest(){return null;},querySelector(){return {textContent:words};}});
const card=list=>({querySelector(){return null;},querySelectorAll(){return list;}});
const first=[fold('Called from · 3',true),fold('+2',false),fold('+2',true),fold('Source details',false)];
const keys=rmOpenFolds(card(first));
assert.deepEqual(keys,['Called from · 3#1','+2#2']);
// Shown again with a declaration's fold above them.
const again=[fold('Calls',false),fold('Called from · 3',false),fold('+2',false),fold('+2',false),fold('Source details',true)];
rmRestoreFolds(card(again),keys);
assert.deepEqual(again.map(f=>f.open),[false,true,false,true,false]);
`)
}

func TestMainFlowCallsAndTheirEvidenceSurviveReturn(t *testing.T) {
	code := systemJSPiece(t, "30-map.js", "function rmFolds(card){", "// \"Expand all\" opens")
	runSystemJS(t, code+`
const fold=(words,open)=>({open,querySelector(){return {textContent:words};}});
function reading(states){
 const evidence=fold('Source details',false),flowFold=fold('Main flow',true);
 const steps=states.map((expanded,i)=>{
   const twist={expanded,calls:null,getAttribute(){return String(this.expanded);},captureCalls(){return this.calls;},setExpanded(open,saved){this.expanded=open;if(saved!==undefined)this.calls=saved.slice();if(open&&i===0&&!details.includes(evidence))details.push(evidence);}};
   return {dataset:{stepPart:'#t16-g23',stepKey:i===2?'another':'bootstrap-main'},closest(){return null;},querySelector(){return twist;},twist};
 });
 const details=[flowFold];if(states[0])details.push(evidence);
 const flow={querySelectorAll(){return steps;}};
 return {steps,evidence,querySelector(){return flow;},querySelectorAll(){return details;}};
}

const before=reading([true,false,true]);before.evidence.open=true;
before.steps[0].twist.calls=['>calls:first','>calls:first>calls:second','\u0000helpers'];
before.steps[1].twist.calls=['\u0000helpers'];
const saved=rmOpenFolds(before);
const after=reading([false,false,false]);rmRestoreFolds(after,saved);
assert.deepEqual(after.steps.map(s=>s.twist.expanded),[true,false,true],'same declaration repeated independently');
assert.equal(after.evidence.open,true,'calls are mounted before restoring their evidence');
assert.deepEqual(after.steps[0].twist.calls,before.steps[0].twist.calls,'serialized nested-call and helper state returns to its own root');
assert.deepEqual(after.steps[1].twist.calls,['\u0000helpers'],'a closed repeated root keeps its own helper state');
rmRestoreFolds(after,saved);
assert.equal(after.querySelectorAll().filter(d=>d===after.evidence).length,1,'return mounts calls once');
`)
}

func TestNativeCallTreeRestoresTwoLazyLevelsFromOriginalPaths(t *testing.T) {
	code := systemJSPiece(t, "32-flow.js", "var rmFlowHelpers=false;", "// The one quiet toggle")
	runSystemJS(t, fakeElements+`
El.prototype.querySelectorAll=function(selector){const cls=selector.slice(1);return this.all(e=>e!==this&&e.classList.contains(cls));};
function rmDotBreaks(e){return e;}
function rmCallableName(d){return d.name;}
const rmEndWords={out:{}};
const data={decls:['entry','first','second','leaf'].map(name=>({key:name,name})),own:[
 {decl:0,flow:[{decl:1}]},{decl:1,flow:[{decl:2}]},{decl:2,flow:[{decl:3}]}]};
const ctx={};
`+code+`
const tree=rmFlowTree(ctx,data,data.own[0]);
const rows=()=>tree.all(e=>e.tagName==='DETAILS');
assert.equal(rows().length,1,'closed call has no fabricated descendants');
rows()[0].open=true;rows()[0].listeners.toggle();
assert.equal(rows().length,2,'first native toggle mounts the next call');
rows()[1].open=true;rows()[1].listeners.toggle();
const state=tree.captureCalls();
assert.deepEqual(state,['>calls:first','>calls:first>calls:second']);
const returned=rmFlowTree(ctx,data,data.own[0],0,state);
const restored=returned.all(e=>e.tagName==='DETAILS');
assert.deepEqual(restored.map(r=>r.open),[true,true],'both lazy levels exist open before asynchronous toggle events');
assert.ok(returned.textContent.includes('leaf'),'deepest call is reachable on return');
restored.forEach(r=>r.listeners.toggle());
assert.equal(returned.all(e=>e.tagName==='DETAILS').length,2,'queued toggle events do not duplicate descendants');
returned.restoreCalls(state);
assert.deepEqual(returned.captureCalls(),state,'original path state survives redraw');
assert.equal(returned.all(e=>e.tagName==='DETAILS').length,2,'redraw replaces the original tree once');
`)
}
