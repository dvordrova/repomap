import {test} from 'node:test';
import assert from 'node:assert/strict';
import ELK from 'elkjs/lib/elk.bundled.js';
import {prepareCards,wrapText} from './cards.mjs';
import {prepareInteriors,layoutPrepared,overviewInset} from './split-layout.mjs';
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
  assert.ok(areaRecord.contentScale<areaRecord.summaryScale,'area headings and original parts keep their own scales');
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
  assert.equal(first.records,second.records,'prepared card metrics are reused');
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
  assert.ok(frame.height<prepared.summaries.get('area').height,'the removed member list no longer expands the native frame');
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

test('a fit below .44 reserves collection minima in one correction without resizing component interiors',async()=>{
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
    assert.equal(nodes.get(`${root.id}-part`).width,96);assert.equal(nodes.get(`${root.id}-part`).height,80);
    assert.deepEqual(nodes.get(`${root.id}-part`).position,{x:32,y:64},'prepared interiors retain their own coordinates');
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
  const records=cards(ordinaryRecords),prepared=await prepareInteriors(records,ordinaryRelations,ordinaryAreas,{availableHeight:548});
  const original=ELK.prototype.layout,requests=[];
  ELK.prototype.layout=function(graph,...args){requests.push(structuredClone(graph));return original.call(this,graph,...args);};
  let result;
  try{result=await layoutPrepared(prepared,1054,580);}finally{ELK.prototype.layout=original;}
  assert.equal(requests.length,9,'the existing single final correction handles all root summaries');
  const nodes=new Map(result.layout.nodes.map(node=>[node.id,node])),roots=result.layout.nodes.filter(node=>!node.parentId);
  const span=axis=>Math.max(...roots.map(node=>node.absolute[axis]+node[axis==='x'?'width':'height']))-Math.min(...roots.map(node=>node.absolute[axis]));
  const zoom=Math.min(.44,1022/span('x'),548/span('y'));
  assert.ok(zoom<.44,'the fixture exercises physical text at a smaller whole-map fit');
  for(const root of roots){
    const record=records.find(record=>record.id===root.id);
    assert.ok(root.width*zoom+1e-7>=record.overviewMinWidth,`${root.id}: no heading word is split`);
    assert.ok(root.height*zoom+1e-7>=record.overviewHeightAtWidth(root.width*zoom,{availableHeight:548}),
      `${root.id}: the complete short inventory or input types fit`);
    const interior=prepared.interiors.get(root.id);
    for(const child of interior.local.nodes.filter(node=>node.parentId)){
      const node=nodes.get(child.id);
      assert.ok(close(node.absolute.x-root.absolute.x,child.absolute.x*interior.scale));
      assert.ok(close(node.absolute.y-root.absolute.y,child.absolute.y*interior.scale));
      assert.ok(close(node.width,child.width*interior.scale));assert.ok(close(node.height,child.height*interior.scale));
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
  assert.equal(requests.length,9,'multirow sizing still needs only one correction after the eight native candidates');
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
    const interior=prepared.interiors.get(root.id);
    for(const child of interior.local.nodes.filter(node=>node.parentId)){
      const node=nodes.get(child.id);
      assert.ok(close(node.absolute.x-root.absolute.x,child.absolute.x*interior.scale));
      assert.ok(close(node.absolute.y-root.absolute.y,child.absolute.y*interior.scale));
      assert.ok(close(node.width,child.width*interior.scale));assert.ok(close(node.height,child.height*interior.scale));
    }
  }
});
