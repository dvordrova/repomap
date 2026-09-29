import {test} from 'node:test';
import assert from 'node:assert/strict';
import {connections, borderCrossing, stubEnds} from './layout.mjs';
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
  assert.equal(inputs.length,8,'a bounded comparison of native ports, orientation and layer packing');
  assert.deepEqual([...new Set(inputs.map(input=>JSON.stringify([input.layoutOptions['elk.direction'],input.layoutOptions['elk.layered.layerUnzipping.strategy']||'NONE'])))].map(s=>JSON.parse(s)).sort(),[
    ['DOWN','ALTERNATING'],['DOWN','NONE'],['RIGHT','ALTERNATING'],['RIGHT','NONE'],
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

test('a candidate ELK cannot place is left out, and the map fails only when none is placed',async()=>{
  const prepared=await prepareInteriors(items,relations,areas);
  const original=ELK.prototype.layout;
  const unzipped=graph=>graph.id==='world'&&graph.layoutOptions['elk.layered.layerUnzipping.strategy']==='ALTERNATING'&&graph.layoutOptions['elk.direction']==='RIGHT';
  ELK.prototype.layout=function(graph,...args){
    if(unzipped(graph))return Promise.reject(new TypeError('java.lang.NullPointerException'));
    return original.call(this,graph,...args);
  };
  let placed;
  try{placed=await layoutPrepared(prepared,1200,700);}finally{ELK.prototype.layout=original;}
  assert.ok(placed.layout.nodes.length>0,'the other candidates still place the world');
  ELK.prototype.layout=function(graph,...args){
    if(graph.id==='world')return Promise.reject(new TypeError('java.lang.NullPointerException'));
    return original.call(this,graph,...args);
  };
  try{await assert.rejects(layoutPrepared(prepared,1200,700),/NullPointerException/);}finally{ELK.prototype.layout=original;}
});

test('the ordinary composed layout routes the real endpoints, retaining every original source',async()=>{
  const {layout:result}=await semanticLayout(items,relations,areas,1200,700);
  assert.deepEqual(result.nodes.map(n=>n.id).sort(),items.map(n=>n.id).sort());
  assert.equal(result.edges.flatMap(e=>e.relations).length,relations.length);
  assert.ok(result.nodes.every(n=>[n.width,n.height,n.absolute.x,n.absolute.y].every(Number.isFinite)));
  const seen=new Set();
  for(const n of result.nodes){assert.ok(!n.parentId||seen.has(n.parentId),'parents precede their children for React Flow');seen.add(n.id);}
  for(const edge of result.edges){
    assert.ok(edge.path,'every edge has a route');
    const drawnNode=id=>{let n=result.nodes.find(n=>n.id===id);if(edge.outerSegments)while(n.parentId)n=result.nodes.find(p=>p.id===n.parentId);return n;};
    const from=drawnNode(edge.from),to=drawnNode(edge.to);
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

test('grouped destinations keep direction, the actual inner parts and all sources',async()=>{
  const {layout:result}=await semanticLayout(items,relations,areas,1200,700);
  const groups=connections('right',['handler','data'],result.edges);
  const incoming=groups.find(g=>g.outside==='caller'&&g.incoming);
  assert.deepEqual(incoming.insides.sort(),['data','handler']);assert.equal(incoming.relations.length,3);
  const outgoing=groups.find(g=>g.outside==='caller'&&!g.incoming);
  assert.deepEqual(outgoing.insides,['data']);assert.equal(outgoing.relations[0].possible,true);
  assert.equal(result.labels.length,4,'one boundary label per participant and direction on each side');
  for(const label of result.labels)assert.ok(Number.isFinite(label.point.x)&&Number.isFinite(label.point.y),'labels use the native boundary endpoint');
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

// An arrow's card stands where the drawn arrow crosses the frame's border,
// not where it ends on a part inside (redis-cli's had stood on Command line
// client and over Dynamic strings).
test('an arrow meets a frame\'s border where it crosses it',()=>{
  const frame={x:0,y:0,width:100,height:80};
  assert.deepEqual(borderCrossing([{x:150,y:40},{x:60,y:40},{x:60,y:20}],frame),{x:100,y:40},'an arrow running on to a part inside meets the border once');
  assert.deepEqual(borderCrossing([{x:20,y:30},{x:20,y:-40}],frame),{x:20,y:0},'leaving through the top');
  assert.deepEqual(borderCrossing([{x:-50,y:10},{x:0,y:10}],frame),{x:0,y:10},'an end on the border is its crossing');
  assert.equal(borderCrossing([{x:-50,y:10},{x:-10,y:10}],frame),null,'an arrow that never meets it');
});

// A part read by itself inside an area had no arrow: the route to the part
// or area beside it is drawn from its area's border.
test('a part looked at gets one short arrow per connection out of the side facing its neighbour',()=>{
  const part={x:0,y:0,width:100,height:50};
  const ends=stubEnds(part,[{key:'in:left',box:{x:-300,y:0,width:50,height:50},incoming:true},{key:'out:right',box:{x:300,y:-10,width:50,height:50},incoming:false},
    {key:'out:right2',box:{x:300,y:40,width:50,height:50},incoming:false},{key:'out:below',box:{x:0,y:300,width:100,height:50},incoming:false}],10);
  assert.deepEqual(ends.get('in:left'),{side:'left',point:{x:0,y:25},points:[{x:-10,y:25},{x:0,y:25}]},'an incoming one points in');
  assert.equal(ends.get('out:right').point.x,100);assert.ok(Math.abs(ends.get('out:right').point.y-50/3)<1e-9,'two on one side spread along it, in their neighbours\' order');
  assert.deepEqual(ends.get('out:right2').points.map(p=>[p.x,Math.round(p.y*1e6)/1e6]),[[100,33.333333],[110,33.333333]],'an outgoing one points out');
  assert.equal(ends.get('out:below').side,'bottom');
});
