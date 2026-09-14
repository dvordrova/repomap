import {test} from 'node:test';
import assert from 'node:assert/strict';
import {connections} from './layout.mjs';
import {prepareInteriors,layoutPrepared} from './split-layout.mjs';
import {semanticLayout} from './semantic.mjs';
import ELK from 'elkjs/lib/elk.bundled.js';

const part=id=>({id,title:id,width:180,height:95,labelWidth:140,labelHeight:16});
const area=id=>({id,title:id,branch:'area'});
const items=[area('left'),area('right'),part('caller'),part('helper'),part('handler'),part('data')];
const areas=[{id:'left',nodes:['caller','helper']},{id:'right',nodes:['handler','data']}];
const relation=(from,to,source,possible=false)=>({from,to,possible,fromSource:source,operations:['request']});
const relations=[relation('caller','handler','one:1'),relation('caller','handler','one:2'),relation('caller','data','one:3'),
  relation('handler','data','two:1'),relation('helper','caller','one:4'),relation('data','caller','two:2',true)];

test('outer orientation candidates never feed a previous layout back into ELK',async()=>{
  const prepared=await prepareInteriors(items,relations,areas);
  const interiors=structuredClone(prepared.interiors);
  const original=ELK.prototype.layout,inputs=[];
  ELK.prototype.layout=function(graph,...args){inputs.push(structuredClone(graph));return original.call(this,graph,...args);};
  try{await layoutPrepared(prepared,1200,700);}finally{ELK.prototype.layout=original;}
  assert.deepEqual(inputs.map(input=>[input.layoutOptions['elk.direction'],input.layoutOptions['elk.layered.layerUnzipping.strategy']||'NONE']),[
    ['DOWN','NONE'],['RIGHT','NONE'],['DOWN','ALTERNATING'],['RIGHT','ALTERNATING'],
  ],'compare native orientations and outer-layer alternatives with the real library');
  for(const input of inputs){
    for(const child of input.children){
      assert.equal(child.children,undefined,'outer candidates reuse ready participant rectangles');
      assert.equal(child.layoutOptions?.['elk.layered.layerUnzipping.strategy'],undefined,'unzipping belongs only to the outer graph');
      assert.equal(child.x,undefined,'candidate nodes have no positions from an earlier result');
      assert.equal(child.y,undefined,'candidate nodes have no positions from an earlier result');
    }
  }
  for(const input of inputs)for(const edge of input.edges){
    assert.equal(edge.sections,undefined,'a previously bent edge may become straight; old bend points must not survive');
    for(const label of edge.labels||[])assert.equal(label.x,undefined,'candidate positions are independent');
  }
  assert.deepEqual(prepared.interiors,interiors,'candidate layouts cannot mutate prepared interior nodes, ports or routes');
});
function segmentHits(a,b,box) {
  const x=box.absolute.x,y=box.absolute.y,eps=.01;
  return a.x===b.x ? a.x>x+eps&&a.x<x+box.width-eps&&Math.max(a.y,b.y)>y+eps&&Math.min(a.y,b.y)<y+box.height-eps
    : a.y>y+eps&&a.y<y+box.height-eps&&Math.max(a.x,b.x)>x+eps&&Math.min(a.x,b.x)<x+box.width-eps;
}

test('the ordinary composed layout routes the real endpoints, retaining every original source',async()=>{
  const {layout:result}=await semanticLayout(items,relations,areas,1200,700);
  assert.deepEqual(result.nodes.map(n=>n.id).sort(),items.map(n=>n.id).sort());
  assert.equal(result.edges.flatMap(e=>e.relations).length,relations.length);
  assert.ok(result.nodes.every(n=>[n.width,n.height,n.absolute.x,n.absolute.y].every(Number.isFinite)));
  const seen=new Set();
  for(const n of result.nodes){assert.ok(!n.parentId||seen.has(n.parentId),'parents precede their children for React Flow');seen.add(n.id);}
  for(const edge of result.edges){
    assert.ok(edge.path,'every edge has a route');
    const from=result.nodes.find(n=>n.id===edge.from),to=result.nodes.find(n=>n.id===edge.to);
    function onBorder(p,n){const {x,y}=n.absolute;return p.x>=x-.01&&p.x<=x+n.width+.01&&p.y>=y-.01&&p.y<=y+n.height+.01&&(Math.abs(p.x-x)<.01||Math.abs(p.x-x-n.width)<.01||Math.abs(p.y-y)<.01||Math.abs(p.y-y-n.height)<.01);}
    assert.ok(onBorder(edge.segments[0][0],from),`start at ${edge.from}`);
    assert.ok(onBorder(edge.segments.at(-1).at(-1),to),`end at ${edge.to}`);
    for(const segment of edge.segments)for(let i=1;i<segment.length;i++){
      const a=segment[i-1],b=segment[i];assert.ok(a.x===b.x||a.y===b.y,'orthogonal route');
      for(const n of result.nodes.filter(n=>!n.frame))assert.ok(!segmentHits(a,b,n),`${edge.from} → ${edge.to} crosses ${n.id}`);
    }
  }
  assert.equal(result.edges.find(e=>e.from==='caller'&&e.to==='handler').relations.length,2,'repeated calls keep both sources');
});

test('grouped destinations keep direction, actual inner numbers and all sources',async()=>{
  const {layout:result}=await semanticLayout(items,relations,areas,1200,700);
  const groups=connections('right',['handler','data'],result.edges);
  const incoming=groups.find(g=>g.outside==='caller'&&g.incoming);
  assert.deepEqual(incoming.numbers,[1,2]);assert.equal(incoming.relations.length,3);
  const outgoing=groups.find(g=>g.outside==='caller'&&!g.incoming);
  assert.deepEqual(outgoing.numbers,[2]);assert.equal(outgoing.relations[0].possible,true);
  assert.equal(result.labels.length,5,'three outside/direction groups on the left, two on the right');
  for(const label of result.labels)assert.ok(Number.isFinite(label.x)&&Number.isFinite(label.y),'ELK reserves label positions');
});


test('a root leaf reserves its measured readable label dimensions before routing',async()=>{
  const items=[{id:'unowned',activation:'request',title:'A named input',width:260,height:90,minimumWidth:420,minimumHeight:180},part('handler')];
  const original={from:'unowned',to:'handler',fromSource:'app:12'};
  const prepared=await prepareInteriors(items,[original],[]);
  const {layout:result,records}=await layoutPrepared(prepared,1100,700);
  const local=prepared.interiors.get('unowned').local;
  assert.ok(local.width>=420&&local.height>=180,'the native interior reserves supplied minimum dimensions');
  const input=result.nodes.find(n=>n.id==='unowned');
  const scale=records.find(n=>n.id==='unowned').contentScale;
  assert.ok(input.width/scale>=420-1e-7&&input.height/scale>=180-1e-7,'composition preserves the complete minimum after uniform scaling');
  assert.equal(input.parentId,undefined);
  assert.deepEqual(result.edges[0].relations,[original]);
});
