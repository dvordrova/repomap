package report

import (
	"os/exec"
	"strings"
	"testing"
)

// Run both cross-target click handlers, their common explorer path and the
// ordinary history owner. Only browser element/history storage is simulated.
func TestCrossTargetOperationKeepsQuestionAndBackHistory(t *testing.T) {
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
	operations, modes := read("29-operation-view.js"), read("45-modes.js")
	harness := `
const assert=require('node:assert/strict');
function element(id,page){return {id,dataset:{},children:[],events:{},hidden:false,
  classList:{contains(){return false;},toggle(){}},
  appendChild(n){this.children.push(n);return n;},
  addEventListener(name,fn){this.events[name]=fn;},
  closest(selector){return selector==='[data-report-page]'?page||this:null;},
  querySelector(selector){return selector==='[data-map-explorer]'?this.map:null;},
  getAttribute(name){return name==='href'?this.href:null;},scrollIntoView(){}};}
const home=element('overview'),front=element('front'),backend=element('backend'),questionPage=element('questions');
front.dataset.componentName='Front';backend.dataset.componentName='Backend';
const guide=element('question5',questionPage);guide.matches=selector=>selector==='.reading-guide';
guide.querySelector=selector=>selector==='.reading-question'?{textContent:'How does running a level reach the backend?'}:null;
const partNode=element('front-http',front),destination=element('backend-post',backend),remote=element('remote-post',front);
destination.dataset.activation='request';destination.dataset.title='POST /api/level/run';
remote.dataset={remote:'true',title:destination.dataset.title};remote.href='#'+destination.id;
const elements=new Map([home,front,backend,questionPage,guide,partNode,destination,remote].map(n=>[n.id,n]));
const listeners={};
const document={body:{dataset:{}},getElementById(id){return elements.get(id)||null;},createElement(){return element('');},
  querySelectorAll(){return [];},addEventListener(name,fn){listeners[name]=fn;},
  dispatchEvent(event){listeners[event.type]?.(event);}};
class CustomEvent{constructor(type,options){this.type=type;this.detail=options.detail;}}
const clone=value=>value==null?value:JSON.parse(JSON.stringify(value));
let entries=[],position=0,activeURL=new URL('https://report.invalid/?mode=work#front-http'),hashListeners=[];
const window={addEventListener(name,fn){if(name==='hashchange')hashListeners.push(fn);}};
const location={get href(){return activeURL.href;},get hash(){return activeURL.hash;},
  set hash(value){activeURL.hash=value;entries.splice(++position);entries.push({url:activeURL.href,state:null});hashListeners.forEach(fn=>fn());}};
const history={get state(){return entries[position]?.state;},
  replaceState(state,unused,url){activeURL=new URL(url);entries[position]={url:activeURL.href,state:clone(state)};},
  pushState(state,unused,url){activeURL=new URL(url);entries.splice(++position);entries.push({url:activeURL.href,state:clone(state)});},
  back(){assert.ok(position>0);activeURL=new URL(entries[--position].url);restore();}};
let current=front,mode='work',question=guide,term=null,searchIntent='',restoring=false,detailGroup=null;
const body=document.body,pages=[home,front,backend,questionPage],guides=[guide],globalSearch=null;
const returnLink={},termLink={},mapLink={},searchLink={},returnLinks={},readingIntent={},locationName={},mapContext={};
function rmT(text,...values){return text.replace(/\{(\d+)\}/g,(_,n)=>values[n]);}
function setMode(next){mode=next==='work'?'work':'learn';}
function selectQuestion(){} function revealConcept(){} function placeSearch(){} function measureToolbar(){}
` + part(modes, "function enclosing(node){", "  function selectQuestion(") +
		part(modes, "function showReturn(){", "  function placeSearch(") +
		part(modes, "function setPage(node){", "  function setMode(") +
		part(modes, "document.addEventListener('repomap:navigate'", "  // Layout commits") +
		part(modes, "function restore(){", "  // Restore the visit") + `
document.addEventListener('repomap:reading',remember);
front.map={state:null,readingState(){return clone(this.state);},restoreReadingState(value){this.state=clone(value);},
  explorationLabel(){return 'Front HTTP service';}};
backend.map={state:null,readingState(){return clone(this.state);},restoreReadingState(value){this.state=clone(value);},
  explorationLabel(){return 'Backend operation';},querySelectorAll(){return [destination];},
  closest(){return backend;},classList:{contains(){return false;}},scrollIntoView(){}};
function installBackend(){
  const map=backend.map,nodes=[destination],byID={[destination.id]:destination};
  let visit,focusHistory,focusOrigin,mode='structure',operation=null,pinned=false,scope='',trail=[],search={value:''},historyRestoreHash='',revision=0;
  function followForeign(){return false;}function abandonHistoryRestore(){}function snapshot(){return {};}
  function displayed(id){return id;}function setScope(id){scope=id;}function revealChoice(){}
  function show(node){map.shown=node;}
  async function render(){var ticket=++revision;map.state={scope,operation:operation?.id||'',pinned,mode};document.dispatchEvent({type:'repomap:reading'});return ticket;}
` + part(operations, "async function reveal(n,allUses,source){", "    map.exploreNode=") +
		part(operations, "function resetScope(){", "    document.addEventListener('click'") + `
  map.revealNode=reveal;
}
function frontHandlers(){
  const map={closest(){return front;}},byID={[remote.id]:remote},scope=partNode.id,currentEdges=[],incoming=false,groups=[remote];
  function displayed(id){return id;}
  function button(text,fn){const result=element('');result.textContent=text;result.addEventListener('click',fn);return result;}
  function focusRelations(){}
` + part(operations, "function followForeign(node,allUses,source){", "    ops.forEach(") +
		part(operations, "async function open(id,neighbor){", "    function updateFocusSelection(") +
		part(operations, "function neighbor(relations,id){", "        peers.forEach(") +
		part(operations, "groups.forEach(function(n){", "    async function reveal(") + `
  return {relation:neighbor([],remote.id).children[1].events.click,svg:remote.events.click};
}
(async function(){
  for(const handlerName of ['relation','svg']){
    current=front;question=guide;term=null;searchIntent='';mode='work';restoring=false;
    activeURL=new URL('https://report.invalid/?mode=work#front-http');entries=[{url:activeURL.href,state:null}];position=0;
    const original={scope:partNode.id,operation:'front-request',pinned:true,mode:'operations',
      source:{key:'src/client.ts:8',href:'',open:'src/client.ts:8'},disclosures:{outgoing:true}};
    front.map.state=clone(original);backend.map.state=null;hashListeners=[restore];installBackend();remember();
    const click=frontHandlers()[handlerName];click({preventDefault(){},stopImmediatePropagation(){}});
    await new Promise(setImmediate);
    assert.equal(current,backend,handlerName+': exact owning page');
    assert.equal(backend.map.shown,destination,handlerName+': exact owning operation');
    assert.equal(question,guide,handlerName+': preserved question origin');
    assert.equal(mode,'work',handlerName+': preserved reading mode');
    assert.equal(returnLink.hidden,false);assert.equal(returnLink.href,'#question5');
    assert.equal(readingIntent.textContent,guide.querySelector('.reading-question').textContent);
    assert.equal(history.state.repomapReading.question,'question5');
    assert.equal(history.state.repomapReading.map.value.operation,destination.id);
    assert.equal(entries.length,2,handlerName+': one navigation entry');
    assert.deepEqual(entries[0].state.repomapReading.map.value,original,handlerName+': source visit not overwritten');
    front.map.state=null;history.back();
    assert.equal(current,front);assert.equal(question,guide);assert.equal(mode,'work');
    assert.deepEqual(front.map.state,original,handlerName+': Back restores exact selected cube and open disclosure');
  }
})().catch(error=>{console.error(error);process.exitCode=1;});
`
	command := exec.CommandContext(t.Context(), node, "--eval", harness)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("cross-target operation navigation: %v\n%s", err, output)
	}
}
