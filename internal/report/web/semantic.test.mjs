import {test} from 'node:test';
import assert from 'node:assert/strict';
import {semanticLayout,detailedAreas,componentContents,componentViewport,closedContainer,readableFocus,frameInventory,visibleRoute,overviewViewport,systemViewport} from './semantic.mjs';
import {arrange} from './layout.mjs';

test('a search destination must be revealed even when its hidden bounds fit the overview',()=>{
  const placed=new Map([
    ['target',{id:'target'}],['area',{id:'area',parentId:'target'}],
    ['robot',{id:'robot',parentId:'area',absolute:{x:100,y:100},width:200,height:80}],
  ]);
  const records=new Map([['target',{branch:'component'}],['area',{branch:'area'}],['robot',{contentScale:.5}]]);
  const detailed=new Set(['area']),v={x:24,y:24,zoom:2};
  const visible=(d,open,viewport=v)=>readableFocus('robot',placed,records,d,open,viewport,800,700);
  assert.equal(visible(detailed,false),false,'closed component hides the chosen part');
  assert.equal(visible(new Set(),true),false,'closed area hides the chosen part');
  assert.equal(visible(detailed,true,{...v,zoom:1}),false,'tiny text needs zoom even if the bounds fit');
  assert.equal(visible(detailed,true),true,'reading an already visible legible neighbour keeps the camera');
  assert.equal(visible(detailed,true,{...v,x:-300}),false,'cropped destination must be brought into view');
});

test('asymmetric summary minimums fit in vertical and horizontal layouts',async()=>{
  for(const [width,height] of [[500,3000],[3000,500]]){
    const layout=await arrange([
      {id:'area',minimumWidth:400,minimumHeight:900},
      {id:'part',width:100,height:40},
    ],[],[{id:'area',nodes:['part']}],width,height);
    const area=layout.nodes.find(n=>n.id==='area');
    assert.ok(area.width>=400&&area.height>=900,`${width}×${height}: ${area.width}×${area.height} must fit 400×900`);
  }
});

test('an oversized overview opens on readable content rather than empty root padding',()=>{
  const nodes=[
    {id:'root',frame:true,absolute:{x:0,y:0},width:4000,height:3000},
    {id:'a',frame:true,absolute:{x:2000,y:600},width:400,height:900},
    {id:'b',frame:true,absolute:{x:800,y:1700},width:400,height:400},
  ];
  const viewport=overviewViewport(nodes,new Set(['a','b']),600,700);
  assert.equal(nodes[1].absolute.x*viewport.zoom+viewport.x,24);
  assert.equal(nodes[1].absolute.y*viewport.zoom+viewport.y,24);
  assert.equal(viewport.zoom,.6);
  const ungrouped={id:'external',parentId:'root',absolute:{x:300,y:200},width:260,height:110};
  const externalView=overviewViewport([...nodes,ungrouped],new Set(['a','b']),600,700);
  assert.equal(ungrouped.absolute.x*externalView.zoom+externalView.x,24,'a component-owned ungrouped item is also a visible entrance');
  assert.equal(ungrouped.absolute.y*externalView.zoom+externalView.y,24);
});

test('the first visit frames all targets and communications without selecting a child',()=>{
  const roots=[
    {id:'front',absolute:{x:32,y:64},width:2084,height:1309},
    {id:'backend',absolute:{x:616,y:1846},width:1421,height:1322},
    {id:'api',absolute:{x:261,y:1457},width:924,height:184},
  ];
  for(const [width,height] of [[603,700],[895,400]]){
  const v=systemViewport(roots,width,height);
  assert.equal(componentContents(v.zoom),false);
  for(const n of roots){
    assert.ok(n.absolute.x*v.zoom+v.x>=23.99);
    assert.ok(n.absolute.y*v.zoom+v.y>=23.99);
    assert.ok((n.absolute.x+n.width)*v.zoom+v.x<=width-23.99);
    assert.ok((n.absolute.y+n.height)*v.zoom+v.y<=height-23.99);
  }
  }
});

test('root summaries reserve readable width after tall input summaries are placed',async()=>{
  const items=[
    {id:'front',branch:'component',children:['ui1','ui2','ui3']},
    {id:'back',branch:'component',children:['server','domain']},
    {id:'api',branch:'communication',children:['http']},
  ],relations=[];
  for(const id of ['ui1','ui2','ui3','server','domain']){
    items.push({id,branch:'area',children:[id+'-a',id+'-b']},
      {id:id+'-a',title:id+' input',width:260,height:480,
        inputs:Array.from({length:6},(_,i)=>({id:id+i,title:'Input '+i,height:40}))},
      {id:id+'-b',title:id+' data',width:260,height:95});
    relations.push({from:id+'-a',to:id+'-b'});
  }
  items.push({id:'http',title:'POST /run',width:260,height:90});
  relations.push({from:'ui1-a',to:'http'},{from:'ui2-a',to:'http'},
    {from:'http',to:'server-a',fromSource:'client.ts:12'},
    {from:'server-a',to:'domain-a'});
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const world=await semanticLayout(items,relations,areas,603,620);
  const v=systemViewport(world.layout.nodes,603,620);
  for(const node of world.layout.nodes.filter(n=>!n.parentId)){
    assert.ok(node.width*v.zoom>=(node.id==='api'?150:200),`${node.id} must have room for its name and summary`);
    assert.ok(node.absolute.x*v.zoom+v.x>=23.99);
    assert.ok((node.absolute.x+node.width)*v.zoom+v.x<=579.01);
    assert.ok((node.absolute.y+node.height)*v.zoom+v.y<=596.01);
  }
  assert.equal(world.layout.nodes.length,items.length);
  assert.deepEqual(world.layout.edges.flatMap(e=>e.relations),relations);
});

test('component entrance reveals the first child even when routing puts it far inside the frame',()=>{
  const frame={id:'front',absolute:{x:32,y:64},width:2084,height:1309};
  for(const x of [64,364,1300]){
  const child={id:'area',parentId:'front',absolute:{x,y:302},width:400,height:580};
  const v=componentViewport(frame,[frame,child],603);
  assert.equal(componentContents(v.zoom),true);
  assert.equal(frame.absolute.y*v.zoom+v.y,24);
  assert.ok(child.absolute.x*v.zoom+v.x>=24);
  assert.ok((child.absolute.x+child.width)*v.zoom+v.x<=579.01);
  assert.ok(v.zoom>=.85&&v.zoom<=1);
  }
});

test('zoom detail changes contents without mutating any world coordinates or routes',async()=>{
  const items=[
    {id:'front',title:'Frontend',branch:'component',children:['ui']},
    {id:'ui',title:'Interface',branch:'area',children:['a','b']},
    {id:'a',title:'Input',width:260,height:200,inputs:[{id:'click',title:'Run',height:40}]},
    {id:'b',title:'Rendering',width:260,height:90},
    {id:'worker',title:'Backend',width:260,height:90},
  ];
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const relations=[{from:'a',to:'b',operations:['click']},{from:'a',to:'worker',operations:['click'],fromSource:'ui.ts:10'}];
  const world=await semanticLayout(items,relations,areas,1000,700);
  const before=JSON.stringify(world.layout);
  const area=world.layout.nodes.find(n=>n.id==='ui');
  assert.ok(area.width>=400&&area.height>=world.summaries.get('ui').height,'summary must fit the very same area');
  assert.ok(world.layout.nodes.every(n=>Number.isFinite(n.width)&&Number.isFinite(n.height)));
  let previous=new Set();
  for(const zoom of [.6,1,4,1,.4,2,.6]){
    previous=detailedAreas(world.scales,zoom,previous);
    for(const edge of world.layout.edges){
      const closed=id=>world.owner(id)&&!previous.has(world.owner(id))?area:null;
      const path=visibleRoute(edge,closed(edge.from),closed(edge.to));
      if(edge.to==='worker')assert.ok(path,'cross-area routes remain visible at every zoom');
      else if(!previous.has('ui'))assert.equal(path,'','internal arrows wait for their actual parts');
    }
    assert.equal(JSON.stringify(world.layout),before,'wheel gestures cannot invoke a different layout');
  }
  assert.equal(world.summaries.get('ui').members[0].inputs[0].id,'click');
  assert.equal(world.layout.edges.find(e=>e.to==='worker').relations[0].fromSource,'ui.ts:10');
});

test('closed area endpoints clip the existing route in each direction, without bends or invented edges',()=>{
  const left={id:'left',absolute:{x:0,y:0},width:100,height:100};
  const right={id:'right',absolute:{x:200,y:0},width:100,height:100};
  const horizontal={segments:[[{x:50,y:50},{x:250,y:50}]]};
  assert.equal(visibleRoute(horizontal,left,right),'M 100 50 L 200 50');
  assert.equal(visibleRoute({segments:[horizontal.segments[0].slice().reverse()]},right,left),'M 200 50 L 100 50');
  const bottom={id:'bottom',absolute:{x:0,y:200},width:100,height:100};
  assert.equal(visibleRoute({segments:[[{x:50,y:50},{x:50,y:250}]]},left,bottom),'M 50 100 L 50 200');
  assert.equal(visibleRoute({segments:[[{x:50,y:250},{x:50,y:50}]]},bottom,left),'M 50 200 L 50 100');
  const fractional={segments:[[{x:50,y:50},{x:50.0000000000001,y:250}]]};
  const clipped=visibleRoute(fractional,left,bottom).match(/-?[\d.]+/g).map(Number);
  assert.ok(Math.abs(clipped[0]-clipped[2])<.00001,'fractional vertical offsets cannot turn into a diagonal clip');
  assert.equal(clipped[1],100);assert.equal(clipped[3],200);
  assert.equal(visibleRoute(horizontal,left,left),'');
  assert.equal(visibleRoute(horizontal,null,null),'M 50 50 L 250 50');
});

test('semantic detail has hysteresis at the actual text scale',()=>{
  const scales=new Map([['small',1],['large',.25]]);
  assert.deepEqual([...detailedAreas(scales,.6)],[]);
  const open=detailedAreas(scales,3);
  assert.deepEqual([...open],['small','large']);
  assert.deepEqual([...detailedAreas(scales,2.4,open)],['small','large']);
  assert.deepEqual([...detailedAreas(scales,2,open)],['small']);
  assert.deepEqual([...detailedAreas(scales,2.4,new Set(['small']))],['small']);
});

test('distant components hide all descendants and internal arrows in the same world',async()=>{
  const items=[
    {id:'front',branch:'component',title:'Front',children:['area','utility']},
    {id:'area',branch:'area',title:'Rendering',children:['handler']},
    {id:'handler',title:'Handler',width:260,height:200,inputs:[{id:'run',title:'Run',height:40}]},
    {id:'utility',title:'Async utilities',width:260,height:90},
    {id:'api',branch:'communication',title:'Backend API',children:['get']},
    {id:'get',title:'GET /levels',width:260,height:90},
  ];
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const world=await semanticLayout(items,[{from:'handler',to:'utility'},{from:'handler',to:'get'}],areas,1000,700);
  const placed=new Map(world.layout.nodes.map(n=>[n.id,n])),records=new Map(world.records.map(n=>[n.id,n]));
  const before=JSON.stringify(world.layout);
  let open=true,detail=new Set();
  for(const zoom of [.7,.45,.5,.6,3,.2]){
    open=componentContents(zoom,open);detail=detailedAreas(world.scales,zoom,detail);
    const closed=id=>closedContainer(id,placed,records,detail,open);
    assert.equal(closed('front'),null);
    if(!open){
      for(const id of ['area','handler','utility'])assert.equal(closed(id).id,'front',id);
      assert.equal(closed('get').id,'api');
      const internal=world.layout.edges.find(e=>e.to==='utility');
      assert.equal(visibleRoute(internal,closed(internal.from),closed(internal.to)),'');
    }
    const external=world.layout.edges.find(e=>e.to==='get');
    assert.ok(visibleRoute(external,closed(external.from),closed(external.to)));
    assert.equal(JSON.stringify(world.layout),before);
  }
  assert.deepEqual(frameInventory('front',new Map(areas.map(a=>[a.id,a.nodes])),records),{groups:1,areaIDs:['area'],parts:2,inputs:1});
  assert.equal(componentContents(.5,true),true,'return preserves the open side of hysteresis');
  assert.equal(componentContents(.5,false),false,'return preserves the closed side of hysteresis');
});
