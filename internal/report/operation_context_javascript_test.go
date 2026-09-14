package report

import "testing"

// Inputs retain their named entrance and saved attachment when a reader opens
// downstream code. Selection never manufactures or replaces the drawing.
func TestOperationContextSurvivesOpeningAPart(t *testing.T) {
	script := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "(function(){")
	runSystemJS(t, script+`
const nodes=[{id:'front',children:['area']},{id:'area',children:['a','b','c','op']},{id:'a'},{id:'b'},{id:'c'},{id:'remote'},{id:'unrelated'},{id:'op',activation:'command'}];
const edges=[{from:'op',to:'a',label:'implemented in',operations:['op']},{from:'a',to:'b',operations:['op']},{from:'b',to:'c',operations:['op']},{from:'c',to:'remote',operations:['op']},{from:'c',to:'unrelated',operations:[]}];
const p=rmSystemProjection(nodes,edges),before=JSON.stringify([p.visible,p.areas,p.representatives]);
assert.deepEqual(p.visible,['a','b','c','remote','unrelated','op']);
assert.equal(p.inputOwner.op,'a');
assert.equal(p.selection('','op').entry,'op','the named input is the path entrance');
assert.equal(p.selection('b','op').entry,'op','inspecting downstream code preserves the input entrance');
assert.equal(p.selection('b','op').active.has('unrelated'),false,'a neighbouring part outside the saved path is not highlighted');
assert.equal(p.selection('unrelated','op').active.has('unrelated'),false,'reading a part does not make it a participant of the pinned input');
for(const id of ['a','b','area']){
 const selected=p.selection(id,'op');
 assert.ok(selected.active.has('remote'));
 assert.equal(selected.path.length,4);
 assert.equal(JSON.stringify([p.visible,p.areas,p.representatives]),before,'selection must not change the drawing');
}
const single=rmSystemProjection(nodes,[edges[0]]);
assert.deepEqual([...single.selection('','op').active],['op','a']);
assert.equal(single.visible.includes('op'),true);
const unbound=rmSystemProjection(nodes,[]);
assert.equal(unbound.selection('','op').entry,'op','an unbound input keeps its own visible identity');
const componentLink=rmSystemProjection(nodes,[{from:'front',to:'remote',scope:'component'}]);
assert.ok(componentLink.selection('front').active.has('remote'),'component connections remain reachable');
`)
}

func TestInputEntrancesLeaveLayoutContainersAndKeepSemanticContext(t *testing.T) {
	script := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "(function(){")
	runSystemJS(t, script+`
const nodes=[{id:'component',children:['area','command','route']},{id:'area',children:['part','action']},
 {id:'part'},{id:'command',activation:'command',inputOwner:'part'},
 {id:'action',activation:'interaction'},{id:'route',activation:'request'}];
const edges=[{from:'command',to:'part',label:'implemented in',operations:['command']},
 {from:'action',to:'part',label:'implemented in',operations:['action']}];
const original=JSON.stringify({nodes,edges}),p=rmSystemProjection(nodes,edges);
assert.deepEqual(p.areas,[{id:'component',nodes:['area']},{id:'area',nodes:['part']}]);
assert.equal(p.parents.command,'component');assert.equal(p.parents.action,'area');assert.equal(p.parents.route,'component');
assert.equal(p.inputOwner.command,'part');assert.equal(p.inputOwner.action,'part');assert.equal(p.inputOwner.route,undefined);
for(const id of ['command','action','route']){
 assert.ok(p.visible.includes(id));assert.deepEqual(p.representatives[id],[id]);
 assert.deepEqual([...p.selection(id).selected],[id]);assert.equal(p.selection('',id).entry,id);
}
assert.deepEqual([...p.selection('','route').active],['route'],'an unmatched native route does not acquire an invented implementation');
assert.deepEqual(p.selection('','route').path,[]);
assert.equal(JSON.stringify({nodes,edges}),original,'display containment does not rewrite the saved graph');
`)
}

func TestInputLayoutPreservesEndpointsAndActualComponentNames(t *testing.T) {
	projection := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "(function(){")
	layout := systemJSPiece(t, "29-operation-view.js", "async function layout(){", "  map.captureViewport=")
	runSystemJS(t, projection+`
const node=(id,dataset)=>({id,dataset});
const nodes=[node('web',{branch:'component',owner:'web-section',title:'Browser app',children:'part action route'}),
 node('input-catalogue',{branch:'inputs',owner:'web-section',title:'Browser app',children:'action route'}),
 node('part',{owner:'web-section',title:'Submit handler'}),node('action',{owner:'web-section',title:'Submit',activation:'interaction'}),
 node('route',{owner:'web-section',title:'POST /jobs',activation:'request'}),
 node('other',{branch:'component',owner:'other-section',title:'Other system'}),
 node('unknown',{owner:'missing-section',title:'Unmatched owner',activation:'command'})];
const byID=Object.fromEntries(nodes.map(n=>[n.id,n]));
const rawEdges=[{from:'action',to:'part',label:'implemented in',operations:['action'],fromSource:'app.ts:12'}];
const model=nodes.map(n=>({id:n.id,branch:n.dataset.branch,children:(n.dataset.children||'').split(/\s+/).filter(Boolean),activation:n.dataset.activation}));
const projection=rmSystemProjection(model,rawEdges),map={setAttribute(){}},stage={},caption={};let surface,captured;
const kind=n=>n.dataset.activation?'Input':'Part',category=n=>n.dataset.activation?'input':'part';
function emphasize(){}function select(){}function renderCaption(){}function rmT(s){return s;}
async function rmCreateFlow(...args){captured=args;return {layout:{edges:args[3]}};}
`+layout+`
(async()=>{
 await layout();const [, ,items,relations,areas,inputOwner]=captured;
 assert.deepEqual(items.map(n=>n.id),nodes.map(n=>n.id),'each original entrance stays a selectable node');
 assert.deepEqual(relations,rawEdges,'attachment endpoints and original provenance are unchanged');
 assert.equal(relations[0].displayFrom,undefined);assert.equal(relations[0].displayTo,undefined);
 assert.deepEqual(areas,[{id:'web',nodes:['part']},{id:'input-catalogue',nodes:['action','route']}]);assert.deepEqual(items.find(n=>n.id==='web').children,['part']);
 assert.deepEqual(items.find(n=>n.id==='input-catalogue').children,['action','route']);
 for(const id of ['action','route','input-catalogue']){const input=items.find(n=>n.id===id);assert.equal(input.componentOwner,'web');assert.equal(input.componentName,'Browser app');}
 const unknown=items.find(n=>n.id==='unknown');assert.equal(unknown.componentOwner,'');assert.equal(unknown.componentName,'','an unknown target is not guessed from a nearby frame');
 assert.equal(inputOwner.action,'part');assert.equal(inputOwner.route,undefined);
 assert.equal(projection.parents.route,'input-catalogue','the catalogue breadcrumb remains available');
})();
`)
}

func TestInputSearchHighlightsTheExactEntrance(t *testing.T) {
	projection := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "(function(){")
	emphasize := systemJSPiece(t, "29-operation-view.js", "function emphasize(){", "  function renderCaption(){")
	runSystemJS(t, projection+`
const nodes=[{id:'part'},{id:'input',activation:'command',inputOwner:'part'}];
const projection=rmSystemProjection(nodes,[{from:'input',to:'part',label:'implemented in',operations:['input']}]);
const scope='',operation=null,searchValue='run',filterValue='',numbered=true;
let update;const surface={update:value=>update=value},map={classList:{toggle(){}},dataset:{}},clear={};
const matches=n=>n.id==='input';function renderCaption(){}
`+emphasize+`
emphasize();assert.deepEqual([...update.matched],['input']);
`)
}
