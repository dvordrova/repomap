package report

import "testing"

// The whole drawing is independent of selection. A one-part input does not
// manufacture a two-box graph, and a longer path retains its remote endpoint.
func TestOperationContextSurvivesOpeningAPart(t *testing.T) {
	script := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "(function(){")
	runSystemJS(t, script+`
const nodes=[{id:'front',children:['area']},{id:'area',children:['a','b','c','op']},{id:'a'},{id:'b'},{id:'c'},{id:'remote'},{id:'unrelated'},{id:'op',activation:'command'}];
const edges=[{from:'op',to:'a',label:'implemented in',operations:['op']},{from:'a',to:'b',operations:['op']},{from:'b',to:'c',operations:['op']},{from:'c',to:'remote',operations:['op']},{from:'c',to:'unrelated',operations:[]}];
const p=rmSystemProjection(nodes,edges),before=JSON.stringify([p.visible,p.areas,p.representatives]);
assert.deepEqual(p.visible,['a','b','c','remote','unrelated']);
assert.equal(p.inputOwner.op,'a');
assert.equal(p.selection('','op').entry,'a','the visible handler identifies the selected input');
assert.equal(p.selection('b','op').entry,'a','inspecting downstream code preserves the entry');
assert.equal(p.selection('b','op').active.has('unrelated'),false,'a neighbouring part outside the saved path is not highlighted');
assert.equal(p.selection('unrelated','op').active.has('unrelated'),false,'reading a part does not make it a participant of the pinned input');
for(const id of ['a','b','area']){
 const selected=p.selection(id,'op');
 assert.ok(selected.active.has('remote'));
 assert.equal(selected.path.length,4);
 assert.equal(JSON.stringify([p.visible,p.areas,p.representatives]),before,'selection must not change the drawing');
}
const single=rmSystemProjection(nodes,[edges[0]]);
assert.deepEqual([...single.selection('','op').active],['a']);
assert.equal(single.visible.includes('op'),false);
const unbound=rmSystemProjection(nodes,[]);
assert.equal(unbound.selection('','op').entry,'op','an unbound input keeps its own visible identity');
const componentLink=rmSystemProjection(nodes,[{from:'front',to:'remote',scope:'component'}]);
assert.ok(componentLink.selection('front').active.has('remote'),'component connections remain reachable');
`)
}
