import {test} from 'node:test';
import assert from 'node:assert/strict';
import ELK from 'elkjs/lib/elk.bundled.js';
import {prepareCards,wrapText} from './cards.mjs';
import {prepareInteriors,layoutPrepared} from './split-layout.mjs';

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
    assert.ok(border(edge.segments[0][0],from),`start at visible endpoint ${from.id}`);
    assert.ok(border(edge.segments.at(-1).at(-1),to),`end at visible endpoint ${to.id}`);
    for(let i=0;i<edge.segments.length;i++){
      const segment=edge.segments[i];
      for(let j=1;j<segment.length;j++)assert.ok(close(segment[j-1].x,segment[j].x)||close(segment[j-1].y,segment[j].y),'native orthogonal segments remain orthogonal');
      if(i){const a=edge.segments[i-1].at(-1),b=segment[0];assert.ok(close(a.x,b.x)&&close(a.y,b.y),`${edge.id}: native boundary joins have no gap`);}
    }
  }
  const areaRecord=result.records.find(record=>record.id==='area'),areaNode=nodes.get('area');
  assert.ok(areaNode.width/areaRecord.summaryScale>=400-1e-7,'the complete area summary has its original width');
  assert.ok(areaNode.height/areaRecord.summaryScale>=result.summaries.get('area').height-1e-7,'the complete area summary has its original height');
  assert.ok(areaRecord.contentScale<areaRecord.summaryScale,'area summaries and original parts keep distinct zoom levels');
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
  assert.ok(interior.height*.44<availableHeight,'the overview reserves the first entrance without stretching to the entire list');
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
