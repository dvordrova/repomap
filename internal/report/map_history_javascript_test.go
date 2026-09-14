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
func runSystemJS(t *testing.T, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	out, err := exec.CommandContext(t.Context(), node, "-e", "const assert=require('node:assert/strict');\n"+script).CombinedOutput()
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
	runSystemJS(t, `
let surface=null,scope='a',operation=null,selectionRevision=0,searchValue='',filterValue='',numbered=true,savedAddress;
const a={id:'a',dataset:{}},b={id:'b',dataset:{}},op={id:'op',dataset:{activation:'command'}},byID={a,b,op};
let finish;const ready=new Promise(resolve=>finish=resolve),search={},filter={};
const map={explorerMember:{owner:'a',key:'source-a'},captureViewport(){return {scale:1,left:20,top:40};},
 showNode(n){this.shown=n.id;},clearInspection(){this.shown=null;},explainSource(s){this.explorerMember=s;},inspectConcept(){this.explorerMember=null;},restoreViewport(v){this.viewport=v;}};
let emphasisCount=0;function emphasize(){emphasisCount++;}function updateResults(){}function emit(){}function focusNode(){}function syncStyle(){}
function address(n){savedAddress={id:n.id,state:map.readingState()};}
`+selectCode+resumeCode+stateCode+`
(async()=>{
 search.value=searchValue='input search';filter.value=filterValue='input';
 const pending=select(b,true,{key:'source-b'},true);
 assert.equal(savedAddress.id,'b');assert.equal(savedAddress.state.scope,'b');assert.equal(savedAddress.state.source,null);
 assert.equal(savedAddress.state.search,'');assert.equal(savedAddress.state.filter,'','a chosen destination closes the catalogue and its unrelated highlights');
 assert.equal(search.value,'');assert.equal(filter.value,'');
 const restored=map.restoreReadingState({scope:'a',operation:'op',numbered:false,source:{key:'source-a'},viewport:{scale:1.2,left:32,top:65}});
 finish();await Promise.all([pending,restored]);
 assert.equal(map.shown,'a');assert.equal(map.explorerMember.key,'source-a');assert.equal(map.readingState().operation,'op');assert.equal(map.viewport.left,32);
 assert.equal(map.readingState().numbered,false,'connection style survives return');
 const beforeClick=emphasisCount;await select(b,true);assert.equal(map.readingState().operation,'op','part selection preserves operation');
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
