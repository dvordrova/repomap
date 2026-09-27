import {test} from 'node:test';
import assert from 'node:assert/strict';
import ELK from 'elkjs/lib/elk.bundled.js';
import {prepareCards,wrapText} from './cards.mjs';
import {prepareInteriors,layoutPrepared,overviewInset,readableScale,pairLeads} from './split-layout.mjs';
import {systemViewport} from './semantic.mjs';
import {denseInventory,manyExternalInventory,records as ordinaryRecords,relations as ordinaryRelations,areas as ordinaryAreas} from './visual/two-systems-five-externals.mjs';

const raw=[
  {id:'app',title:'Application',branch:'component',children:['area']},
  {id:'area',title:'Requests and results',branch:'area',children:['caller','helper']},
  {id:'caller',title:'Handle a request'},
  {id:'helper',title:'Prepare a result'},
  {id:'inputs',title:'Application',branch:'inputs',children:['input']},
  {id:'input',title:'POST /jobs',activation:'request'},
  {id:'remote',title:'Job service',branch:'communication',children:['call','result']},
  {id:'call',title:'Submit a job'},
  {id:'result',title:'Read the result'},
];
const relations=[
  {from:'input',to:'caller',fromSource:'routes:12'},
  {from:'caller',to:'helper',fromSource:'handler:15'},
  {from:'caller',to:'call',fromSource:'http:20'},
  {from:'caller',to:'call',fromSource:'http:22'},
  {from:'helper',to:'result',fromSource:'http:30'},
  {from:'result',to:'caller',fromSource:'receive:8',possible:true},
];
const cards=items=>prepareCards(items,{},text=>String(text).length*8,text=>text);
const areas=items=>items.filter(item=>item.children).map(item=>({id:item.id,nodes:item.children}));
const close=(a,b)=>Math.abs(a-b)<1e-7;
function border(point,node){
  const {x,y}=node.absolute;
  return point.x>=x-1e-7&&point.x<=x+node.width+1e-7&&point.y>=y-1e-7&&point.y<=y+node.height+1e-7&&
    (close(point.x,x)||close(point.x,x+node.width)||close(point.y,y)||close(point.y,y+node.height));
}

test('short component names preserve the measured reading width of their complete area inventory',async()=>{
  const items=[
    {id:'front',title:'front',branch:'component',children:['navigation','utilities']},
    {id:'navigation',title:'Application shell and navigation',branch:'area',children:['route']},
    {id:'utilities',title:'Shared domain and utility support',branch:'area',children:['helper']},
    {id:'route',title:'Open the playground'},
    {id:'helper',title:'Prepare the level'},
  ];
  const records=cards(items),component=records.find(record=>record.id==='front');
  assert.ok(component.overviewPreferredWidth>component.overviewMinWidth,'the inventory needs more width than its short owner name');
  const prepared=await prepareInteriors(records,[],areas(items),{availableHeight:600});
  const frame=prepared.interiors.get('front'),width=frame.width*.44,height=frame.height*.44;
  for(const area of items.filter(item=>item.branch==='area')){
    assert.ok(wrapText(area.title,width-32,'500 13px system-ui',text=>text.length*8).length<=2,`${area.title}: the preferred width keeps the full entry readable`);
  }
  assert.ok(height+1e-7>=component.overviewHeightAtWidth(width,{availableHeight:600}),'the same native frame contains the complete inventory');
});

test('native outer routes stop at frames while local routes retain exact parts and sources',async()=>{
  const prepared=await prepareInteriors(cards(raw),relations,areas(raw));
  const result=await layoutPrepared(prepared,1200,800),layout=result.layout;
  assert.deepEqual(layout.nodes.map(node=>node.id).sort(),raw.map(item=>item.id).sort());
  assert.deepEqual(layout.edges.flatMap(edge=>edge.relations).map(r=>r.fromSource).sort(),relations.map(r=>r.fromSource).sort());
  assert.equal(layout.edges.find(edge=>edge.from==='caller'&&edge.to==='call').relations.length,2);
  const grouped=prepared.aggregates.find(edge=>edge.from==='app'&&edge.to==='remote');
  assert.equal(grouped.edges.length,2,'two original endpoint pairs share the native outer route');
  assert.equal(grouped.relations.length,3,'each source stays on that route');
  assert.ok(prepared.aggregates.some(edge=>edge.from==='remote'&&edge.to==='app'&&edge.possible),'reverse possible relation stays distinct');
  const nodes=new Map(layout.nodes.map(node=>[node.id,node]));
  const root=id=>{let n=nodes.get(id);while(n.parentId)n=nodes.get(n.parentId);return n;};
  for(const edge of layout.edges){
    assert.ok(edge.segments.length,'each original edge has a native route');
    const from=edge.outerSegments?root(edge.from):nodes.get(edge.from),to=edge.outerSegments?root(edge.to):nodes.get(edge.to);
    if(edge.outerSegments){assert.equal(edge.outerFrom,from.id);assert.equal(edge.outerTo,to.id);}
    assert.ok(border(edge.segments[0][0],from),`start at visible endpoint ${from.id}`);
    assert.ok(border(edge.segments.at(-1).at(-1),to),`end at visible endpoint ${to.id}`);
    for(let i=0;i<edge.segments.length;i++){
      const segment=edge.segments[i];
      for(let j=1;j<segment.length;j++)assert.ok(close(segment[j-1].x,segment[j].x)||close(segment[j-1].y,segment[j].y),'native orthogonal segments remain orthogonal');
      if(i){const a=edge.segments[i-1].at(-1),b=segment[0];assert.ok(close(a.x,b.x)&&close(a.y,b.y),`${edge.id}: native boundary joins have no gap`);}
    }
  }
  const areaRecord=result.records.find(record=>record.id==='area'),areaNode=nodes.get('area');
  assert.ok(areaNode.width/areaRecord.summaryScale>=400-1e-7,'the area retains its heading width');
  for(const child of layout.nodes.filter(node=>node.parentId==='area')){
    assert.ok(child.absolute.y>=areaNode.absolute.y+64*areaRecord.summaryScale-1e-7,'native children follow the area heading');
    assert.ok(child.absolute.y+child.height<=areaNode.absolute.y+areaNode.height+1e-7,'native children remain inside the area');
  }
  assert.ok(close(areaRecord.contentScale,areaRecord.summaryScale),'an area draws its parts at its own heading\'s size, as the parts beside it');
  assert.ok(layout.labels.length,'source-backed connection labels retain native placement');
  for(const label of layout.labels){const p=label.point||label;assert.ok(Number.isFinite(p.x)&&Number.isFinite(p.y));}
});

test('viewport changes run only independent flat outer layouts and retain each prepared interior',async()=>{
  const prepared=await prepareInteriors(cards(raw),relations,areas(raw));
  const original=ELK.prototype.layout,graphs=[];
  ELK.prototype.layout=function(graph,...args){
    const input=structuredClone(graph);graphs.push(input);
    return original.call(this,graph,...args);
  };
  let first,second;
  try{first=await layoutPrepared(prepared,1200,800);second=await layoutPrepared(prepared,1500,950);}finally{ELK.prototype.layout=original;}
  assert.equal(graphs.length,16,'eight flat candidates per viewport, no interior work');
  for(const graph of graphs){
    assert.ok(graph.children.every(node=>!node.children),'outer layout receives ready participant rectangles');
    assert.ok(graph.children.every(node=>node.x===undefined&&node.y===undefined),'each candidate starts without stale positions');
    assert.ok(graph.children.every(node=>!node.ports?.length||node.layoutOptions['elk.portConstraints']==='FIXED_POS'),'only free or existing native boundary points are compared');
  }
  const normalized=layout=>{
    const nodes=new Map(layout.nodes.map(node=>[node.id,node]));
    return layout.nodes.map(node=>{let root=node;while(root.parentId)root=nodes.get(root.parentId);return {
      id:node.id,width:node.width,height:node.height,x:node.absolute.x-root.absolute.x,y:node.absolute.y-root.absolute.y,
    };}).sort((a,b)=>a.id.localeCompare(b.id));
  };
  const a=normalized(first.layout),b=normalized(second.layout);
  for(let i=0;i<a.length;i++){assert.equal(a[i].id,b[i].id);for(const field of ['width','height','x','y'])assert.ok(close(a[i][field],b[i][field]),`${a[i].id}: local ${field} survives resizing`);}
  assert.deepEqual(first.records,second.records,'the same interior fit preserves card metrics and content');
});

test('unrelated external participants cannot resize or rearrange an input collection',async()=>{
  const base=await prepareInteriors(cards(raw),relations,areas(raw));
  const extra=Array.from({length:17},(_,i)=>[
    {id:`external${i}`,title:`External participant ${i}`,branch:'communication',children:[`external-call${i}`]},
    {id:`external-call${i}`,title:`Call participant ${i}`},
  ]).flat();
  const extended=await prepareInteriors(cards([...raw,...extra]),relations,areas([...raw,...extra]));
  for(const id of ['inputs','app','remote']){
    const before=base.interiors.get(id),after=extended.interiors.get(id);
    assert.equal(after.scale,before.scale,`${id}: normalization is local`);
    assert.deepEqual(after.local,before.local,`${id}: native local geometry is independent of unrelated roots`);
  }
  const connected=await prepareInteriors(cards([...raw,...extra]),[
    ...relations,...Array.from({length:17},(_,i)=>({from:'caller',to:`external-call${i}`,fromSource:`client:${i+1}`})),
  ],areas([...raw,...extra]));
  assert.deepEqual(connected.interiors.get('inputs'),base.interiors.get('inputs'),
    'new destinations called by the implementation do not change its independent input collection');
});

test('an inventory taller than the initial canvas keeps every area behind a readable scroll entrance',async()=>{
  const areaIDs=Array.from({length:42},(_,i)=>`area${i}`);
  const items=cards([
    {id:'component',title:'Application',branch:'component',children:areaIDs},
    ...areaIDs.flatMap(id=>[{id,title:`Responsibility ${id}`,branch:'area',children:[`${id}-part`]},
      {id:`${id}-part`,title:`Implementation ${id}`}]),
  ]);
  const availableHeight=600,component=items.find(item=>item.id==='component');
  assert.ok(component.overviewHeightAtWidth(component.overviewMinWidth)>availableHeight,'complete inventory exceeds the measured canvas');
  const prepared=await prepareInteriors(items,[],areas(items),{availableHeight});
  assert.equal(prepared.records.length,items.length,'all original areas and parts survive');
  assert.equal(prepared.summaries.size,areaIDs.length);
  const interior=prepared.interiors.get('component');
  const root=interior.local.nodes.find(node=>node.id==='component');
  const children=interior.local.nodes.filter(node=>node.parentId==='component');
  const bottom=Math.max(...children.map(node=>node.absolute.y+node.height));
  assert.ok(root.height-bottom<=33,'the component ends at native child bounds plus its bottom padding');
  assert.ok(interior.height*.44>=component.overviewHeightAtWidth(component.overviewMinWidth,{availableHeight})-1e-7);
});

test('input catalogues choose native columns by their own shape without spreading a short list',async()=>{
  const small=['GET /api/level/{level_id}','GET /api/levels','POST /api/level/run'].map((title,i)=>({id:`request${i}`,title,activation:'request'}));
  const large=['Animate simulation frame','Change playground code','Change simulation slider','Change slowness setting','Display root page',
    'Load level details','Render error page','Resize simulation canvas','Run simulation on click','Toggle simulation play','Update editor code']
    .map((title,i)=>({id:`interaction${i}`,title,activation:i?'interaction':'continuous'}));
  const items=cards([
    {id:'small-inputs',title:'backend',branch:'inputs',children:small.map(n=>n.id)},...small,
    {id:'large-inputs',title:'front',branch:'inputs',children:large.map(n=>n.id)},...large,
    {id:'implementation',title:'Implementation'},
  ]);
  const relations=[...small,...large].map((n,i)=>({from:n.id,to:'implementation',fromSource:`inputs:${i+1}`}));
  const prepared=await prepareInteriors(items,relations,areas(items),{availableHeight:533});
  const columns=id=>new Set(prepared.interiors.get(id).local.nodes.filter(n=>n.parentId===id).map(n=>n.position.x)).size;
  assert.equal(columns('small-inputs'),1,'three requests retain their compact ordinary column');
  assert.equal(columns('large-inputs'),2,'the native alternative avoids a wide empty frame around eleven inputs');
  const result=await layoutPrepared(prepared,1054,581);
  assert.equal(result.layout.edges.length,relations.length,'column choice preserves all original paths');
  for(const edge of result.layout.edges)for(let i=1;i<edge.segments.length;i++){
    const a=edge.segments[i-1].at(-1),b=edge.segments[i][0];
    assert.ok(close(a.x,b.x)&&close(a.y,b.y),'column alternatives preserve native boundary joins');
  }
});

test('an area reserves its native contents and heading without a second member-list height',async()=>{
  const records=cards(raw.filter(item=>['app','area','caller','helper'].includes(item.id)));
  const prepared=await prepareInteriors(records,[{from:'caller',to:'helper'}],areas(records));
  const interior=prepared.interiors.get('app'),frame=interior.local.nodes.find(node=>node.id==='area');
  const children=interior.local.nodes.filter(node=>node.parentId==='area');
  const bottom=Math.max(...children.map(node=>node.absolute.y+node.height));
  const record=prepared.records.find(item=>item.id==='area'),areaScale=record.contentScale/record.summaryScale;
  assert.ok(close(frame.absolute.y+frame.height-bottom,32*areaScale),
    'only native bottom padding remains below the actual objects');
});

test('connected dense component inventories keep All readable without stretching the component frame',async()=>{
  const fixture=denseInventory(),records=cards(fixture.records),width=1054,height=581;
  const prepared=await prepareInteriors(records,fixture.relations,fixture.areas,{availableHeight:height-2*overviewInset});
  for(const id of ['front','backend']){
    const {local}=prepared.interiors.get(id),children=local.nodes.filter(node=>node.parentId===id);
    const root=local.nodes.find(node=>node.id===id);
    const bends=[...local.edges.values()].flat(2).filter(point=>!border(point,root));
    assert.equal(children.length,42,'every connected area remains in the component');
    assert.ok(local.height-Math.max(...children.map(node=>node.absolute.y+node.height),...bends.map(point=>point.y))<=33,
      'the frame ends at its native children/routes and bottom padding');
  }
  const {layout}=await layoutPrepared(prepared,width,height),roots=layout.nodes.filter(node=>!node.parentId);
  const span=axis=>Math.max(...roots.map(node=>node.absolute[axis]+node[axis==='x'?'width':'height']))-Math.min(...roots.map(node=>node.absolute[axis]));
  const zoom=Math.min(.44,(width-2*overviewInset)/span('x'),(height-2*overviewInset)/span('y'));
  for(const node of roots){
    const record=records.find(record=>record.id===node.id),physicalWidth=node.width*zoom,physicalHeight=node.height*zoom;
    assert.ok(physicalWidth+1e-7>=record.overviewMinWidth,`${node.id}: the complete heading fits at All`);
    assert.ok(physicalHeight+1e-7>=record.overviewHeightAtWidth(physicalWidth,{availableHeight:height-2*overviewInset}),
      `${node.id}: the heading and inventory entrance fit at All`);
  }
  assert.equal(layout.nodes.length,fixture.records.length);
  assert.equal(layout.edges.flatMap(edge=>edge.relations).length,fixture.relations.length);
});

test('component placement moves ready area interiors intact and bundles only matching boundary relations',async()=>{
  const records=cards([
    {id:'component',title:'Application',branch:'component',children:['first','second','direct']},
    {id:'first',title:'Accept work',branch:'area',children:['request','authorize']},
    {id:'second',title:'Execute work',branch:'area',children:['schedule','run']},
    {id:'request',title:'Accept a request'},{id:'authorize',title:'Check permissions'},
    {id:'schedule',title:'Schedule execution'},{id:'run',title:'Execute the work'},
    {id:'direct',title:'Shared helper'},
  ]);
  const relations=[
    {from:'request',to:'authorize',fromSource:'request:1'},
    {from:'schedule',to:'run',fromSource:'schedule:2'},
    {from:'request',to:'schedule',fromSource:'request:3'},
    {from:'authorize',to:'run',fromSource:'authorize:4'},
    {from:'request',to:'run',possible:true,fromSource:'request:5'},
    {from:'run',to:'request',fromSource:'run:6'},
    {from:'run',to:'direct',fromSource:'run:7'},
  ];
  const original=ELK.prototype.layout;
  let ready;
  ELK.prototype.layout=function(graph,...args){
    return original.call(this,graph,...args).then(placed=>{if(graph.id==='interior:component')ready=structuredClone(placed.children[0]);return placed;});
  };
  let prepared;
  try{prepared=await prepareInteriors(records,relations,areas(records));}finally{ELK.prototype.layout=original;}
  const {local}=prepared.interiors.get('component'),nodes=new Map(local.nodes.map(node=>[node.id,node]));
  const offsets=new Map([[ready.id,{x:0,y:0}]]),nativeEdges=new Map();
  function index(node){
    const offset=offsets.get(node.id);
    for(const child of node.children||[]){offsets.set(child.id,{x:offset.x+child.x,y:offset.y+child.y});index(child);}
  }
  index(ready);
  function nativeRoutes(node){
    for(const edge of node.edges||[]){
      const offset=offsets.get(edge.container||node.id);
      nativeEdges.set(edge.id,(edge.sections||[]).map(section=>[section.startPoint,...section.bendPoints||[],section.endPoint]
        .map(point=>({x:point.x+offset.x,y:point.y+offset.y}))));
    }
    for(const child of node.children||[])nativeRoutes(child);
  }
  nativeRoutes(ready);
  for(const area of ready.children.filter(node=>node.children)){
    const placed=nodes.get(area.id);
    assert.ok(placed.frame,'a ready area remains a frame after the flat placement');
    assert.equal(placed.width,area.width);assert.equal(placed.height,area.height);
    for(const child of area.children){
      const moved=nodes.get(child.id);
      assert.deepEqual(moved.position,{x:child.x,y:child.y},'only the area offset changes');
      assert.equal(moved.width,child.width);assert.equal(moved.height,child.height);
    }
  }
  const routes=Object.fromEntries(prepared.edges.map(edge=>[edge.relations[0].fromSource,local.edges.get(edge.id)]));
  for(const [source,area] of [['request:1','first'],['schedule:2','second']]){
    const edge=prepared.edges.find(edge=>edge.relations[0].fromSource===source),a=offsets.get(area),b=nodes.get(area).absolute;
    const expected=nativeEdges.get(edge.id).map(segment=>segment.map(point=>({x:point.x-a.x+b.x,y:point.y-a.y+b.y})));
    assert.deepEqual(routes[source],expected,'every original area route receives only the area translation');
  }
  assert.deepEqual(routes['request:3'],routes['authorize:4'],'distinct original calls share the same directed area boundary route');
  assert.deepEqual(routes['request:3'],routes['request:5'],'possible and definite source relations share one native boundary corridor');
  assert.notDeepEqual(routes['request:3'],routes['run:6'],'the reverse direction remains separate');
  for(const [source,from,to] of [['request:1','request','authorize'],['schedule:2','schedule','run'],
    ['request:3','first','second'],['authorize:4','first','second'],['request:5','first','second'],
    ['run:6','second','first'],['run:7','second','direct']]){
    assert.ok(border(routes[source][0][0],nodes.get(from)),`${source}: native start remains on its visible boundary`);
    assert.ok(border(routes[source].at(-1).at(-1),nodes.get(to)),`${source}: native end remains on its visible boundary`);
  }
  assert.deepEqual(prepared.edges.flatMap(edge=>edge.relations),relations,'the boundary grouping preserves every original source and endpoint');
});

test('a fit below .44 reserves collection minima and fits the existing interiors once',async()=>{
  // A wide ready frontend and its peer fit below .44. The smaller catalogues
  // were prepared exactly at .44, so resizing from the old fit alone would
  // still leave their headings too small after the new outer placement.
  const roots=[
    {id:'front',branch:'component',width:1391,height:814,overviewMinWidth:107,overviewHeightAtWidth:()=>268},
    {id:'backend',branch:'component',width:749,height:645,overviewMinWidth:138,overviewHeightAtWidth:()=>212},
    {id:'front-inputs',branch:'inputs',width:91/.44,height:149/.44,overviewMinWidth:91,overviewHeightAtWidth:()=>149},
    {id:'backend-inputs',branch:'inputs',width:73/.44,height:107/.44,overviewMinWidth:73,overviewHeightAtWidth:()=>107},
  ].map(root=>({...root,children:[`${root.id}-part`]}));
  const records=roots.flatMap(root=>[root,{id:`${root.id}-part`,width:96,height:80,contentScale:1}]);
  const interiors=new Map(roots.map(root=>{
    const ports=[['out','EAST',root.width],['in','WEST',0]].map(([id,side,x])=>({id:`${root.id}-${id}`,
      width:0,height:0,x,y:90,layoutOptions:{'elk.port.side':side}}));
    const local={nodes:[{id:root.id,position:{x:0,y:0},absolute:{x:0,y:0},width:root.width,height:root.height,frame:true},
      {id:`${root.id}-part`,parentId:root.id,position:{x:32,y:64},absolute:{x:32,y:64},width:96,height:80,frame:false}],
      edges:new Map(),labels:[],ports,width:root.width,height:root.height};
    return [root.id,{id:root.id,local,ports,scale:1,width:root.width,height:root.height}];
  }));
  const aggregates=[['front-inputs','front'],['front','backend'],['backend-inputs','backend']].map(([from,to],i)=>({
    id:`outer:${i}`,from,to,sourcePort:`${from}-out`,targetPort:`${to}-in`,edges:[`e${i}`],relations:[],
  }));
  const prepared={roots,records,interiors,aggregates,labels:new Map(),scales:new Map(),owner:()=>'',summaries:new Map(),
    edges:aggregates.map((edge,i)=>({id:`e${i}`,from:`${edge.from}-part`,to:`${edge.to}-part`,aggregate:String(i),relations:[]}))};
  const original=ELK.prototype.layout,requests=[];
  ELK.prototype.layout=function(graph,...args){requests.push(structuredClone(graph));return original.call(this,graph,...args);};
  let result;
  try{result=await layoutPrepared(prepared,1054,580);}finally{ELK.prototype.layout=original;}
  assert.equal(requests.length,9,'one measured correction follows the same eight native candidates');
  const nodes=new Map(result.layout.nodes.map(node=>[node.id,node])),frames=roots.map(root=>nodes.get(root.id));
  const span=axis=>Math.max(...frames.map(node=>node.absolute[axis]+node[axis==='x'?'width':'height']))-Math.min(...frames.map(node=>node.absolute[axis]));
  const zoom=Math.min(.44,1022/span('x'),548/span('y'));
  assert.ok(zoom<.44,'this is the sub-.44 fit that previously clipped the smaller catalogues');
  for(const root of roots){
    const frame=nodes.get(root.id);
    assert.ok(frame.width*zoom+1e-7>=root.overviewMinWidth,`${root.id}: the final fit preserves heading width`);
    assert.ok(frame.height*zoom+1e-7>=root.overviewHeightAtWidth(frame.width*zoom),`${root.id}: the final fit preserves the full input types`);
    assert.ok(frame.width>=root.width&&frame.height>=root.height,'a measured root reserve never shrinks its native contents');
    const scale=Math.min(frame.width/root.width,frame.height/root.height),part=nodes.get(`${root.id}-part`);
    assert.ok(close(part.width,96*scale)&&close(part.height,80*scale),'the original drawing uses its enlarged frame');
    assert.ok(close(part.position.x,32*scale)&&close(part.position.y,64*scale),'one uniform transform preserves relative geometry');
    assert.ok(close(result.records.find(record=>record.id===part.id).contentScale,scale),'text uses the same scale as its card');
  }
  for(const node of requests.at(-1).children)for(const port of node.ports||[]){
    if(port.layoutOptions['elk.port.side']==='EAST')assert.equal(port.x,node.width,'the fixed native port follows the enlarged frame');
  }
  for(const edge of result.layout.edges){
    const aggregate=aggregates.find(item=>item.edges.includes(edge.id));
    assert.ok(border(edge.segments[0][0],nodes.get(aggregate.from)));
    assert.ok(border(edge.segments.at(-1).at(-1),nodes.get(aggregate.to)));
  }
});

test('the ordinary map reserves a short component inventory as well as collection headings',async()=>{
  const records=cards(ordinaryRecords),prepared=await prepareInteriors(records,ordinaryRelations,ordinaryAreas,{availableHeight:508});
  const original=ELK.prototype.layout,requests=[];
  ELK.prototype.layout=function(graph,...args){requests.push(structuredClone(graph));return original.call(this,graph,...args);};
  let result;
  try{result=await layoutPrepared(prepared,1000,540);}finally{ELK.prototype.layout=original;}
  assert.ok(requests.length<=10,'eight native candidates and at most two corrections handle all root summaries');
  const nodes=new Map(result.layout.nodes.map(node=>[node.id,node])),roots=result.layout.nodes.filter(node=>!node.parentId);
  const span=axis=>Math.max(...roots.map(node=>node.absolute[axis]+node[axis==='x'?'width':'height']))-Math.min(...roots.map(node=>node.absolute[axis]));
  const zoom=Math.min(.44,968/span('x'),508/span('y'));
  assert.ok(zoom<.44,'the fixture exercises physical text at a smaller whole-map fit');
  for(const root of roots){
    const record=records.find(record=>record.id===root.id);
    assert.ok(root.width*zoom+1e-7>=record.overviewMinWidth,`${root.id}: no heading word is split`);
    assert.ok(root.height*zoom+1e-7>=record.overviewHeightAtWidth(root.width*zoom,{availableHeight:508}),
      `${root.id}: the complete short inventory or input types fit`);
    const interior=prepared.interiors.get(root.id),scale=Math.min(root.width/interior.local.width,root.height/interior.local.height);
    for(const child of interior.local.nodes.filter(node=>node.parentId)){
      const node=nodes.get(child.id);
      assert.ok(close(node.absolute.x-root.absolute.x,child.absolute.x*scale));
      assert.ok(close(node.absolute.y-root.absolute.y,child.absolute.y*scale));
      assert.ok(close(node.width,child.width*scale));assert.ok(close(node.height,child.height*scale));
    }
  }
});


test('parallel rows share one measured reserve while every participant remains readable',async()=>{
  const fixture=manyExternalInventory({inputs:true}),records=cards(fixture.records);
  const width=1054,height=711,available={width:width-2*overviewInset,height:height-2*overviewInset};
  const prepared=await prepareInteriors(records,fixture.relations,fixture.areas,{availableHeight:available.height});
  const original=ELK.prototype.layout,requests=[];
  ELK.prototype.layout=function(graph,...args){requests.push(structuredClone(graph));return original.call(this,graph,...args);};
  let result;
  try{result=await layoutPrepared(prepared,width,height);}finally{ELK.prototype.layout=original;}
  assert.ok(requests.length<=10,'multirow sizing stays within the two corrections after the eight native candidates');
  const nodes=new Map(result.layout.nodes.map(node=>[node.id,node])),roots=result.layout.nodes.filter(node=>!node.parentId);
  assert.equal(roots.length,21,'both targets, both input collections and all seventeen destinations remain');
  const span=axis=>Math.max(...roots.map(node=>node.absolute[axis]+node[axis==='x'?'width':'height']))-Math.min(...roots.map(node=>node.absolute[axis]));
  const zoom=Math.min(.44,available.width/span('x'),available.height/span('y'));
  assert.ok(zoom<.44,'the regression reaches a whole-map fit below the preferred camera');
  for(const root of roots){
    const record=records.find(record=>record.id===root.id);
    assert.ok(root.width*zoom+1e-7>=record.overviewMinWidth,`${root.id}: its heading keeps complete words`);
    assert.ok(root.height*zoom+1e-7>=record.overviewHeightAtWidth(root.width*zoom,{availableHeight:available.height}),
      `${root.id}: its measured heading and complete short inventory fit`);
    const interior=prepared.interiors.get(root.id),scale=Math.min(root.width/interior.local.width,root.height/interior.local.height);
    for(const child of interior.local.nodes.filter(node=>node.parentId)){
      const node=nodes.get(child.id);
      assert.ok(close(node.absolute.x-root.absolute.x,child.absolute.x*scale));
      assert.ok(close(node.absolute.y-root.absolute.y,child.absolute.y*scale));
      assert.ok(close(node.width,child.width*scale));assert.ok(close(node.height,child.height*scale));
    }
  }
});

// Laid out with the whole component and its 98 inputs, Redis's Server
// runtime put each of its seven parts in a row of its own: a staircase of
// postage stamps, shrunk to a peer's width, with arrowheads larger than them.
test('an area lays out its parts from its own arrows, at their own size, not as a staircase of the component',async()=>{
  const runtime=['r1','r2','r3','r4','r5','r6','r7'],types=['t1','t2','t3','t4'],inputs=Array.from({length:12},(_,i)=>`i${i}`);
  const items=[{id:'server',title:'Server',branch:'component'},
    {id:'runtime',title:'Server runtime',branch:'area'},{id:'types',title:'Data type commands',branch:'area'},
    ...runtime.map(id=>({id,title:`Runtime ${id}`})),...types.map(id=>({id,title:`Types ${id}`})),
    {id:'inputs',title:'Server',branch:'inputs'},...inputs.map(id=>({id,title:id,activation:'request'})),
    {id:'dns',title:'DNS',branch:'communication'},{id:'resolve',title:'gethostbyname',category:'external'}];
  const relations=[['r1','r2'],['r1','r3'],['r2','r4'],['r3','r5'],['r4','r6'],['r5','r7'],['r2','t1'],['t1','r5'],['r6','t2'],['t3','r1'],['t4','r7'],['r7','resolve'],
    ...inputs.map((id,i)=>[id,[...runtime,...types][i%11]])].map(([from,to])=>({from,to}));
  const areaList=[{id:'server',nodes:['runtime','types']},{id:'runtime',nodes:runtime},{id:'types',nodes:types},{id:'inputs',nodes:inputs},{id:'dns',nodes:['resolve']}];
  const prepared=await prepareInteriors(cards(items),relations,areaList);
  const {local}=prepared.interiors.get('server'),nodes=new Map(local.nodes.map(node=>[node.id,node]));
  const boxes=runtime.map(id=>nodes.get(id)),rows=new Set(boxes.map(box=>Math.round(box.absolute.y))).size;
  assert.ok(rows<boxes.length,`the runtime parts share rows instead of a staircase of ${rows} rows`);
  const record=prepared.records.find(item=>item.id==='runtime'),frame=nodes.get('runtime');
  assert.ok(close(record.contentScale,record.summaryScale),'parts keep their size beside the area heading and the loose parts');
  for(const box of boxes)assert.ok(box.absolute.x>=frame.absolute.x&&box.absolute.x+box.width<=frame.absolute.x+frame.width+1e-7,'every part stands inside its area');
});

test('input groups each hold their tiles under their title, side by side without overlap, inside the collection',async()=>{
  const inputs=Array.from({length:9},(_,i)=>`i${i}`);
  const items=[{id:'server',title:'Server',branch:'component'},{id:'a',title:'String commands'},{id:'b',title:'List commands'},
    {id:'inputs',title:'Server',branch:'inputs',children:['inputs~a','inputs~b']},
    {id:'inputs~a',title:'String commands',branch:'inputs-part',children:inputs.slice(0,6)},{id:'inputs~b',title:'List commands',branch:'inputs-part',children:inputs.slice(6)},
    ...inputs.map(id=>({id,title:id,activation:'request'}))];
  const areaList=[{id:'server',nodes:['a','b']},{id:'inputs',nodes:['inputs~a','inputs~b']},{id:'inputs~a',nodes:inputs.slice(0,6)},{id:'inputs~b',nodes:inputs.slice(6)}];
  const relations=inputs.map((id,i)=>({from:id,to:i<6?'a':'b'}));
  const prepared=await prepareInteriors(cards(items),relations,areaList);
  const {local}=prepared.interiors.get('inputs'),nodes=new Map(local.nodes.map(node=>[node.id,node]));
  const inside=(child,frame)=>child.absolute.x>=frame.absolute.x-1e-7&&child.absolute.y>=frame.absolute.y-1e-7&&
    child.absolute.x+child.width<=frame.absolute.x+frame.width+1e-7&&child.absolute.y+child.height<=frame.absolute.y+frame.height+1e-7;
  const [a,b,root]=['inputs~a','inputs~b','inputs'].map(id=>nodes.get(id));
  assert.ok(inside(a,root)&&inside(b,root),'both groups stand inside the collection');
  const apart=a.absolute.x+a.width<=b.absolute.x+1e-7||b.absolute.x+b.width<=a.absolute.x+1e-7||a.absolute.y+a.height<=b.absolute.y+1e-7||b.absolute.y+b.height<=a.absolute.y+1e-7;
  assert.ok(apart,'the groups do not overlap');
  for(const id of inputs.slice(0,6))assert.ok(inside(nodes.get(id),a),`${id} stands in its group`);
  for(const id of inputs.slice(6))assert.ok(inside(nodes.get(id),b),`${id} stands in its group`);
  const header=prepared.records.find(record=>record.id==='inputs~a').headerHeight;
  for(const id of inputs.slice(0,6))assert.ok(nodes.get(id).absolute.y>=a.absolute.y+header-1e-7,'tiles stand under the group title');
  const before=(p,q)=>p.absolute.y<q.absolute.y-1e-7||Math.abs(p.absolute.y-q.absolute.y)<1e-7&&p.absolute.x<q.absolute.x;
  assert.ok(before(a,b),'groups keep their reading order');
  for(let i=1;i<6;i++)assert.ok(before(nodes.get(inputs[i-1]),nodes.get(inputs[i])),'tiles keep their reading order');
});

// Redis's Server runtime: six parts and 21 arrows, most of them pairs with
// both directions. Laid out twice, each pair made ELK reverse one arrow into
// a wrap-around, and the area stood 1300 px wide in a 1214 px canvas.
test('a dense area lays out one route per pair of ends and fits the canvas at a readable scale',async()=>{
  const parts=['clients','store','replication','config','blocking','memory'];
  const arrows=[['store','config'],['store','memory'],['store','clients'],['replication','config'],['replication','clients'],['config','clients'],
    ['blocking','clients'],['memory','config'],['clients','store'],['clients','replication'],['clients','config'],['clients','blocking'],
    ['clients','memory'],['replication','store'],['config','store'],['config','replication'],['config','blocking'],['config','memory'],
    ['blocking','store'],['blocking','memory'],['memory','store']];
  const items=[{id:'server',title:'Server',branch:'component'},{id:'runtime',title:'Server runtime',branch:'area'},
    ...parts.map(id=>({id,title:`Runtime part ${id}`,summary:'Keeps one responsibility of the running server in one place.',category:'part'}))];
  const canvas={width:1214,height:680};
  const prepared=await prepareInteriors(cards(items),arrows.map(([from,to])=>({from,to})),
    [{id:'server',nodes:['runtime']},{id:'runtime',nodes:parts}],{canvas,availableHeight:canvas.height-32});
  const {local}=prepared.interiors.get('server'),area=local.nodes.find(node=>node.id==='runtime');
  assert.ok(area.width*readableScale<=canvas.width-48&&area.height*readableScale<=canvas.height-48,
    `the area (${Math.round(area.width)}x${Math.round(area.height)}) fits the canvas while its parts stay readable`);
  const route=(from,to)=>JSON.stringify(local.edges.get(prepared.edges.find(edge=>edge.from===from&&edge.to===to).id));
  const flip=segments=>JSON.stringify(JSON.parse(segments).slice().reverse().map(points=>points.slice().reverse()));
  for(const [from,to] of arrows)if(arrows.some(([a,b])=>a===to&&b===from))
    assert.equal(route(to,from),flip(route(from,to)),`${from} and ${to} share one route in both directions`);
});

// Persistence's one arrow, wrapped into two rows, ran around the area.
test('an area with one arrow draws it straight between its two parts, not around them',async()=>{
  const items=[{id:'server',title:'Server',branch:'component'},{id:'persistence',title:'Persistence',branch:'area'},
    {id:'aof',title:'AOF persistence',category:'part'},{id:'rdb',title:'RDB persistence',category:'part'}];
  const prepared=await prepareInteriors(cards(items),[{from:'aof',to:'rdb',possible:true}],
    [{id:'server',nodes:['persistence']},{id:'persistence',nodes:['aof','rdb']}],{canvas:{width:1214,height:680},availableHeight:648});
  const {local}=prepared.interiors.get('server'),at=new Map(local.nodes.map(node=>[node.id,node]));
  const centre=id=>({x:at.get(id).absolute.x+at.get(id).width/2,y:at.get(id).absolute.y+at.get(id).height/2});
  const [points]=local.edges.get(prepared.edges[0].id);
  const length=points.slice(1).reduce((sum,point,i)=>sum+Math.abs(point.x-points[i].x)+Math.abs(point.y-points[i].y),0);
  const a=centre('aof'),b=centre('rdb');
  assert.ok(length<=Math.abs(a.x-b.x)+Math.abs(a.y-b.y),`the arrow runs ${Math.round(length)} between parts whose centres are ${Math.round(Math.abs(a.x-b.x)+Math.abs(a.y-b.y))} apart`);
});

// One "TCP endpoint" box took arrows from all three Redis programs. Each
// program's destination frame stays a participant of its own, with its own
// arrow; frames naming the same destination stand in one display group.
test('frames naming one destination stand in a display group, each keeping its own arrow',async()=>{
  const items=[
    {id:'server',title:'redis-server',branch:'component'},{id:'cli',title:'redis-cli',branch:'component'},
    {id:'net-s',title:'Networking',category:'part'},{id:'net-c',title:'Network client',category:'part'},
    {id:'tcp-s',title:'TCP endpoint',branch:'communication',category:'external',displayGroup:'tcp'},
    {id:'tcp-c',title:'TCP endpoint',branch:'communication',category:'external',displayGroup:'tcp'},
    {id:'connect-s',title:'connect',category:'external'},{id:'connect-c',title:'connect',category:'external'},
  ];
  const areaList=[{id:'server',nodes:['net-s']},{id:'cli',nodes:['net-c']},{id:'tcp-s',nodes:['connect-s']},{id:'tcp-c',nodes:['connect-c']}];
  const prepared=await prepareInteriors(cards(items),[{from:'net-s',to:'connect-s'},{from:'net-c',to:'connect-c'}],areaList);
  const {layout,records}=await layoutPrepared(prepared,1200,700);
  const at=new Map(layout.nodes.map(node=>[node.id,node]));
  const group=layout.nodes.find(node=>node.display);
  assert.ok(group&&records.find(record=>record.id===group.id)?.branch==='communication-group','one display group is drawn');
  for(const id of ['tcp-s','tcp-c']){
    const frame=at.get(id);
    assert.equal(frame.parentId,undefined,`${id} stays a participant of its own`);
    assert.ok(frame.absolute.x>=group.absolute.x-1e-6&&frame.absolute.y>=group.absolute.y-1e-6&&
      frame.absolute.x+frame.width<=group.absolute.x+group.width+1e-6&&frame.absolute.y+frame.height<=group.absolute.y+group.height+1e-6,`${id} stands inside the group`);
  }
  for(const [from,to] of [['net-s','tcp-s'],['net-c','tcp-c']]){
    const edge=layout.edges.find(edge=>edge.from===from),end=edge.segments.at(-1).at(-1),frame=at.get(to);
    assert.equal(edge.outerTo,to,`${from}'s arrow ends at its own frame`);
    assert.ok(border(end,frame),`${from}'s arrow reaches ${to}'s border`);
  }
  assert.ok(!layout.edges.some(edge=>edge.outerTo===group.id||edge.to===group.id),'the group ends no arrow');
});

// Redis's three "DNS resolver" frames stood in a group at the bottom of the
// map. The fit that reserved every heading measured the frames alone; the
// camera frames the group too, 0.85% smaller, and each heading reserved to
// the pixel lost its last letter.
test('the whole-map camera that frames a display group still gives every heading its reserved room',async()=>{
  const items=structuredClone(ordinaryRecords),relations=structuredClone(ordinaryRelations),areaList=structuredClone(ordinaryAreas);
  for(const [id,caller] of [['dns-front','submission'],['dns-backend','worker']]){
    items.push({id,title:'DNS resolver',category:'external',branch:'communication',children:[`${id}-call`],displayGroup:'dns',displayGroupTitle:'DNS resolver'},
      {id:`${id}-call`,title:'gethostbyname',category:'external'});
    areaList.push({id,nodes:[`${id}-call`]});relations.push({from:caller,to:`${id}-call`});
  }
  // Plain tiles leave Redis's 1054×580 canvas at the preferred camera; a
  // smaller one keeps the fit below it.
  const records=cards(items),width=1000,height=540;
  const prepared=await prepareInteriors(records,relations,areaList,{availableHeight:height-2*overviewInset});
  const result=await layoutPrepared(prepared,width,height);
  const {zoom}=systemViewport(result.layout.nodes,width,height);
  const group=result.layout.nodes.find(node=>node.display),heading=result.records.find(record=>record.id===group.id);
  assert.ok(zoom<.44,'the camera fits below the preferred scale, where the reserve matters');
  for(const node of result.layout.nodes.filter(node=>!node.parentId&&!node.display)){
    const record=records.find(record=>record.id===node.id);
    if(!record.overviewMinWidth)continue;
    assert.ok(node.width*zoom+1e-6>=record.overviewMinWidth,`${node.id}: ${node.width*zoom} of ${record.overviewMinWidth}px for its heading`);
    assert.ok(node.height*zoom+1e-6>=record.overviewHeightAtWidth(node.width*zoom,{availableHeight:height-2*overviewInset}),`${node.id}: its summary fits`);
  }
  const need=heading.side==='right'?heading.headingAt(Infinity).extent:heading.headingAt(group.width*zoom).height;
  assert.ok(heading.band*zoom+1e-6>=need,`the group's heading has ${heading.band*zoom} of ${need}px`);
});

// Above the tiles, Redis's three arrows ran through "DNS resolver".
test('a display group carries its frames\' shared text once, where no arrow runs',async()=>{
  const items=[
    {id:'server',title:'redis-server',branch:'component'},{id:'cli',title:'redis-cli',branch:'component'},{id:'bench',title:'redis-benchmark',branch:'component'},
    ...['server','cli','bench'].flatMap(owner=>[{id:`net-${owner}`,title:'Networking',category:'part'},
      {id:`dns-${owner}`,title:'DNS resolver',branch:'communication',category:'external',displayGroup:'dns',displayGroupTitle:'DNS resolver'},
      {id:`resolve-${owner}`,title:'gethostbyname',category:'external'}]),
  ];
  const areaList=['server','cli','bench'].flatMap(owner=>[{id:owner,nodes:[`net-${owner}`]},{id:`dns-${owner}`,nodes:[`resolve-${owner}`]}]);
  const prepared=await prepareInteriors(cards(items),['server','cli','bench'].map(owner=>({from:`net-${owner}`,to:`resolve-${owner}`})),areaList);
  const {layout,records}=await layoutPrepared(prepared,1200,700);
  const groups=layout.nodes.filter(node=>node.display);
  assert.equal(groups.length,1);
  const group=groups[0],heading=records.find(record=>record.id===group.id);
  assert.equal(heading.title,'DNS resolver','the group says it');
  assert.deepEqual(heading.tiles.sort(),['dns-bench','dns-cli','dns-server']);
  // The heading's band is the group's widest strip beside its tiles.
  const tiles=heading.tiles.map(id=>layout.nodes.find(node=>node.id===id));
  const box={left:Math.min(...tiles.map(n=>n.absolute.x)),top:Math.min(...tiles.map(n=>n.absolute.y)),
    right:Math.max(...tiles.map(n=>n.absolute.x+n.width)),bottom:Math.max(...tiles.map(n=>n.absolute.y+n.height))};
  const {x,y,width,height}={...group.absolute,width:group.width,height:group.height};
  const band=[{x,y,width,height:box.top-y},{x,y:box.bottom,width,height:y+height-box.bottom},
    {x,y,width:box.left-x,height},{x:box.right,y,width:x+width-box.right,height}].sort((a,b)=>b.width*b.height-a.width*a.height)[0];
  assert.ok(Math.min(band.width,band.height)>=heading.band-1e-6,'the heading has its band');
  const inside=point=>point.x>band.x+1e-6&&point.x<band.x+band.width-1e-6&&point.y>band.y+1e-6&&point.y<band.y+band.height-1e-6;
  for(const edge of layout.edges){
    assert.equal(edge.outerTo,edge.to.replace('resolve-','dns-'),'each program\'s arrow ends at its own tile');
    for(const segment of edge.segments)for(let i=1;i<segment.length;i++)for(let t=0;t<=1;t+=1/64){
      const point={x:segment[i-1].x+(segment[i].x-segment[i-1].x)*t,y:segment[i-1].y+(segment[i].y-segment[i-1].y)*t};
      assert.ok(!inside(point),`${edge.from}'s arrow crosses the heading at ${JSON.stringify(point)}`);
    }
  }
});

// Redis's Server configuration and lifecycle is its program's entry side,
// yet each of its pairs came first as a redisLog call into it: laid out that
// way, ELK made it a sink and wrapped its routes around Server runtime.
test('a pair of ends is laid out from the program\'s entry side toward the other',async()=>{
  const items=[{id:'server',title:'Server',branch:'component'},{id:'runtime',title:'Server runtime',branch:'area'},
    {id:'config',title:'Server configuration',category:'part',lane:'triggers'},{id:'memory',title:'Virtual memory',category:'part',lane:'core'}];
  const relations=[{from:'memory',to:'config',fromSource:'vm:10'},{from:'config',to:'memory',fromSource:'config:20'}];
  const prepared=await prepareInteriors(cards(items),relations,[{id:'server',nodes:['runtime']},{id:'runtime',nodes:['config','memory']}],
    {canvas:{width:1214,height:680},availableHeight:648});
  const {local}=prepared.interiors.get('server'),at=new Map(local.nodes.map(node=>[node.id,node]));
  const config=at.get('config'),memory=at.get('memory');
  assert.ok(config.absolute.x+config.width<=memory.absolute.x+1e-6||config.absolute.y+config.height<=memory.absolute.y+1e-6,
    'the entry part stands first in the layout, its pair laid out leaving it');
  const route=from=>JSON.stringify(local.edges.get(prepared.edges.find(edge=>edge.from===from).id));
  assert.equal(route('memory'),JSON.stringify(JSON.parse(route('config')).slice().reverse().map(points=>points.slice().reverse())),
    'both directions keep their arrow on the one route');
});

// Between areas the same pair rule holds: the entry area leads.
test('a pair of areas is laid out from the program\'s entry area toward the other',async()=>{
  const items=[{id:'server',title:'Server',branch:'component'},
    {id:'core',title:'Core infrastructure',branch:'area'},{id:'runtime',title:'Server runtime',branch:'area',lane:'triggers'},
    {id:'log',title:'Logging',category:'part'},{id:'config',title:'Server configuration',category:'part',lane:'triggers'}];
  const relations=[{from:'log',to:'config'},{from:'config',to:'log'}];
  const prepared=await prepareInteriors(cards(items),relations,[{id:'server',nodes:['core','runtime']},{id:'core',nodes:['log']},{id:'runtime',nodes:['config']}],
    {canvas:{width:1214,height:680},availableHeight:648});
  const {local}=prepared.interiors.get('server'),at=new Map(local.nodes.map(node=>[node.id,node]));
  const runtime=at.get('runtime'),core=at.get('core');
  assert.ok(runtime.absolute.x+runtime.width<=core.absolute.x+1e-6||runtime.absolute.y+runtime.height<=core.absolute.y+1e-6,
    'the entry area stands first in the component, the pair laid out leaving it');
});

test('a pair leads from an entry end to one that is not, and otherwise keeps its first edge',()=>{
  const entry=id=>id.startsWith('in');
  const leads=ends=>[...pairLeads(ends,entry).values()].map(end=>`${end.source}>${end.target}`);
  assert.deepEqual(leads([{source:'b',target:'in'},{source:'in',target:'b'}]),['in>b'],'the entry end leads though it came second');
  assert.deepEqual(leads([{source:'in2',target:'in'},{source:'in',target:'in2'}]),['in2>in'],'two entry ends keep the first');
  assert.deepEqual(leads([{source:'b',target:'c'},{source:'c',target:'b'}]),['b>c'],'no entry end keeps the first');
  assert.deepEqual(leads([{source:'b',target:'in'}]),['b>in'],'a lone direction is laid out as it is');
});

// A loose part beside areas is drawn filling its box once the areas open,
// at their parts' scale. Its box is its own card's, whatever the areas
// beside it hold: grown to fit its closed heading at its smallest area's
// heading scale, a loose part beside two areas of fourteen parts filled
// 1036 by 739 px beside 260 by 88 px parts (400 by 200 px as a card).
test('a loose part keeps its own card box however large the areas beside it',async()=>{
  const loose=async size=>{
    const runtime=Array.from({length:size},(_,i)=>`r${i}`),types=Array.from({length:size},(_,i)=>`t${i}`);
    const items=[{id:'server',title:'Server',branch:'component'},
      {id:'runtime',title:'Server runtime',branch:'area'},{id:'types',title:'Data type commands',branch:'area'},
      ...runtime.map(id=>({id,title:`Runtime ${id}`})),...types.map(id=>({id,title:`Types ${id}`})),
      {id:'symbols',title:'Debug symbols'}];
    const chain=ids=>ids.slice(1).map((id,i)=>({from:ids[i],to:id}));
    const relations=[...chain(runtime),...chain(types),{from:runtime.at(-1),to:types[0]},{from:'symbols',to:runtime[0]}];
    const areaList=[{id:'server',nodes:['runtime','types','symbols']},{id:'runtime',nodes:runtime},{id:'types',nodes:types}];
    const prepared=await prepareInteriors(cards(items),relations,areaList,{canvas:{width:1214,height:620}});
    const node=prepared.interiors.get('server').local.nodes.find(node=>node.id==='symbols');
    return {width:node.width,height:node.height};
  };
  const small=await loose(2),large=await loose(14);
  assert.ok(close(small.width,large.width)&&close(small.height,large.height),
    `the loose part is ${large.width.toFixed(0)} by ${large.height.toFixed(0)} beside areas of fourteen parts, ${small.width.toFixed(0)} by ${small.height.toFixed(0)} beside areas of two`);
});
