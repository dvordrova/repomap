import {test} from 'node:test';
import assert from 'node:assert/strict';
import {emphasis,focusAncestors} from './emphasis.mjs';

const leaves=id=>id==='area'?['handler','paint']:id==='other'?['test']: [id];
const edges=[
  {id:'hp',from:'handler',to:'paint',relations:[{operations:['input']}]},
  {id:'ht',from:'handler',to:'test',relations:[]},
  {id:'pt',from:'paint',to:'test',relations:[]},
  {id:'elsewhere',from:'test',to:'unrelated',relations:[]},
];
const empty={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false};
test('hovering a child retains its component frame without borrowing sibling arrows',()=>{
  const placed=new Map([['front',{}],['utility',{parentId:'front'}],['area',{parentId:'front'}],['handler',{parentId:'area'}]]);
  const result=emphasis(empty,'utility',id=>[id],[...edges,{id:'own',from:'utility',to:'test',relations:[]}]);
  assert.deepEqual([...focusAncestors(result.focus,placed)],['front']);
  assert.deepEqual([...result.activeEdges],['own']);
  assert.deepEqual([...focusAncestors(new Set(['handler']),placed)],['area','front']);
  assert.deepEqual([...focusAncestors(new Set(),placed)],[]);
});
test('hover names and marks the whole area, with exactly the endpoints of its arrows',()=>{
  const result=emphasis(empty,'area',leaves,edges);
  assert.equal(result.mode,'hover');assert.equal(result.subject,'area');
  assert.deepEqual([...result.focus],['area','handler','paint']);
  assert.deepEqual([...result.activeEdges],['hp','ht','pt']);
  assert.ok(result.participants.has('test'));
  assert.ok(!result.participants.has('unrelated'));
});
test('hover replaces the drawing emphasis without mixing in a clicked part; leaving restores it',()=>{
  const view={...empty,scope:'paint',selected:new Set(['paint'])};
  const before=emphasis(view,'',leaves,edges),after=emphasis(view,'other',leaves,edges);
  assert.equal(after.mode,'hover');assert.equal(after.subject,'other');
  assert.deepEqual([...after.activeEdges],['ht','pt','elsewhere']);
  assert.ok(!after.activeEdges.has('hp'),'old selected links are not mixed into hover');
  assert.deepEqual([...before.activeEdges],['hp','pt']);
  assert.deepEqual(emphasis(view,'',leaves,edges),before);
});
test('reading an off-path part never colours it as an input participant',()=>{
  const view={...empty,operation:'input',entry:'handler',scope:'test',selected:new Set(['test'])};
  const result=emphasis(view,'',leaves,edges);
  assert.deepEqual([...result.activeEdges],['hp']);
  assert.deepEqual([...result.participants],['handler','paint']);
  assert.equal(result.readingOutside,true);
  assert.equal(emphasis({...view,scope:'paint'},'',leaves,edges).readingOutside,false);
  const hovered=emphasis(view,'other',leaves,edges);
  assert.equal(hovered.mode,'hover','a pinned input never disables exploring other areas');
  assert.ok(hovered.activeEdges.has('elsewhere'));
  assert.ok(!hovered.activeEdges.has('hp'),'input path is suspended, not combined with hover');
});
test('search matches are not combined with an old input or hover path, including zero matches',()=>{
  const view={...empty,searching:true,operation:'input',entry:'handler',matched:new Set(['test'])};
  const result=emphasis(view,'area',leaves,edges);
  assert.equal(result.mode,'search');
  assert.deepEqual([...result.participants],['test']);assert.equal(result.activeEdges.size,0);
  assert.equal(emphasis({...view,matched:new Set()},'area',leaves,edges).participants.size,0);
});
