import {test} from 'node:test';
import assert from 'node:assert/strict';
import {semanticLayout,detailedAreas,componentContents,componentTextSize,componentTextSizes,componentDetails,communicationDetails,componentViewport,communicationViewport,closedContainer,readableFocus,frameInventory,visibleRoute,overviewViewport,systemViewport,zoomMarkPosition} from './semantic.mjs';
import {prepareInteriors,layoutPrepared} from './split-layout.mjs';

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
  const prepared=await prepareInteriors([
    {id:'area',title:'Area',branch:'area',minimumWidth:400,minimumHeight:900},
    {id:'part',title:'Part',width:100,height:40},
  ],[],[{id:'area',nodes:['part']}]);
  for(const [width,height] of [[500,3000],[3000,500]]){
    const {layout,records}=await layoutPrepared(prepared,width,height);
    const area=layout.nodes.find(n=>n.id==='area');
    const scale=records.find(n=>n.id==='area').summaryScale;
    assert.ok(area.width/scale>=400&&area.height/scale>=900,`${width}×${height}: native ${area.width/scale}×${area.height/scale} must fit 400×900`);
  }
});

test('an impossible overview fit terminates with every root and relation intact',{timeout:5000},async()=>{
  const items=['left','right'].flatMap(id=>[
    {id,title:id,branch:'component',width:260,height:180,overviewMinWidth:200,overviewHeightAtWidth:()=>180},
    {id:id+'-part',title:id+' part',width:260,height:80},
  ]);
  const relations=[{from:'left-part',to:'right-part'}];
  const areas=['left','right'].map(id=>({id,nodes:[id+'-part']}));
  const {layout}=await semanticLayout(items,relations,areas,300,200);
  assert.deepEqual(layout.nodes.map(n=>n.id).sort(),items.map(n=>n.id).sort());
  assert.deepEqual(layout.edges.map(e=>[e.from,e.to]),[['left-part','right-part']]);
  assert.ok(layout.nodes.every(n=>[n.width,n.height,n.absolute.x,n.absolute.y].every(Number.isFinite)));
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

test('root summaries reserve readable width after tall area summaries are placed',async()=>{
  const items=[
    {id:'front',branch:'component',children:['ui1','ui2','ui3']},
    {id:'back',branch:'component',children:['server','domain']},
    {id:'api',branch:'communication',children:['http']},
  ],relations=[];
  for(const id of ['ui1','ui2','ui3','server','domain']){
    items.push({id,branch:'area',children:[id+'-a',id+'-b']},
      {id:id+'-a',title:id+' input',width:260,height:480},
      {id:id+'-b',title:id+' data',width:260,height:95});
    relations.push({from:id+'-a',to:id+'-b'});
  }
  items.push({id:'http',title:'POST /run',width:260,height:90});
  relations.push({from:'ui1-a',to:'http'},{from:'ui2-a',to:'http'},
    {from:'http',to:'server-a',fromSource:'client.ts:12'},
    {from:'server-a',to:'domain-a'});
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const width=1054,height=711;
  const world=await semanticLayout(items,relations,areas,width,height);
  const v=systemViewport(world.layout.nodes,width,height);
  for(const node of world.layout.nodes.filter(n=>!n.parentId)){
    assert.ok(node.width*v.zoom>=(node.id==='api'?150:200),`${node.id} must have room for its name and summary`);
    assert.ok(node.absolute.x*v.zoom+v.x>=23.99);
    assert.ok((node.absolute.x+node.width)*v.zoom+v.x<=width-23.99);
    assert.ok((node.absolute.y+node.height)*v.zoom+v.y<=height-23.99);
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
    {id:'a',title:'Input',width:260,height:200},
    {id:'b',title:'Rendering',width:260,height:90},
    {id:'worker',title:'Backend',width:260,height:90},
  ];
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const relations=[{from:'a',to:'b',operations:['click']},{from:'a',to:'worker',operations:['click'],fromSource:'ui.ts:10'}];
  const world=await semanticLayout(items,relations,areas,1000,700);
  const before=JSON.stringify(world.layout);
  const area=world.layout.nodes.find(n=>n.id==='ui');
  const summaryScale=world.records.find(n=>n.id==='ui').summaryScale;
  assert.ok(area.width/summaryScale>=400-1e-9&&area.height/summaryScale>=world.summaries.get('ui').height-1e-9,'the scaled summary must fit the very same area');
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
  assert.equal(world.summaries.get('ui').members[0].id,'a');
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
  const open=detailedAreas(scales,4);
  assert.deepEqual([...open],['small','large']);
  assert.deepEqual([...detailedAreas(scales,2.9,open)],['small','large'],'12.325px part text remains readable while zooming out');
  assert.deepEqual([...detailedAreas(scales,2.8,open)],['small'],'11.9px text returns to the area summary');
  assert.deepEqual([...detailedAreas(scales,3,new Set(['small']))],['small'],'12.75px does not open previously closed parts');
  assert.deepEqual([...detailedAreas(scales,14/17)],['small'],'parts first appear at 14px');
});

test('zooming out closes unreadable area summaries before the component summary appears',()=>{
  const component={id:'front',absolute:{x:0,y:0},width:800,height:900};
  const area={id:'area',parentId:'front',absolute:{x:32,y:128},width:400,height:480};
  const part={id:'part',parentId:'area',absolute:{x:64,y:200},width:260,height:90};
  const peer={id:'peer',parentId:'area',absolute:{x:64,y:400},width:260,height:90};
  const nodes=[component,area,part,peer],placed=new Map(nodes.map(n=>[n.id,n]));
  const records=new Map([['front',{branch:'component'}],['area',{branch:'area'}],['part',{}],['peer',{}]]);
  const route={segments:[[{x:194,y:245},{x:194,y:1100}]]};
  const original=JSON.stringify({nodes,route});
  let open=true;
  for(const zoom of [1,.8,.64,.512]){
    open=componentContents(zoom,open);
    const closed=id=>closedContainer(id,placed,records,new Set(),open);
    assert.equal(closed('front'),null,'the component stays in the same world');
    if(zoom===.512){
      assert.ok(23*zoom<12,'this captured zoom makes even the area heading unreadable');
      assert.equal(open,false,'root summary must replace the unreadable area summaries');
      assert.equal(closed('area'),component,'area summary cannot remain over the root summary');
      assert.equal(closed('part'),component,'hidden parts also use the outer component boundary');
      assert.equal(visibleRoute(route,closed('part'),closed('peer')),'','internal routes are concealed with their contents');
      assert.equal(visibleRoute(route,closed('part'),null),'M 194 900 L 194 1100','external route starts at the same component frame');
    }else{
      assert.equal(closed('area'),null,'readable area summaries remain on the map');
      assert.ok(20*zoom>=12,'visible member names remain readable');
    }
    assert.equal(JSON.stringify({nodes,route}),original,'presentation does not move the world or its routes');
  }
  assert.equal(componentContents(.6,true),true,'Back retains the open state at readable 12px member text');
  assert.equal(componentContents(.6,false),false,'Back retains the closed side of hysteresis');
  assert.equal(componentContents(.7,false),true,'area summaries first appear with 14px member text');
});

test('component detail also waits for readable direct part headings',()=>{
  const frame={id:'front',absolute:{x:0,y:0},width:1200,height:900};
  const part={id:'utility',parentId:'front',absolute:{x:32,y:128},width:260,height:90};
  {
    const records=[
      {id:'front',branch:'component',children:['area','utility']},
      {id:'area',branch:'area',children:['nested']},
      {id:'utility',branch:'part',contentScale:1},
      {id:'nested',branch:'part',contentScale:.01},
      {id:'api',branch:'communication',children:['call']},
      {id:'call',contentScale:.001},
    ];
    const textSize=componentTextSize(records);
    assert.equal(textSize,17,'only direct component parts contribute; area parts and calls have their own detail thresholds');
    assert.equal(componentContents(11.9/textSize,true,textSize),false,'an unreadable direct part closes with its component');
    assert.equal(componentContents(12.1/textSize,true,textSize),true,'readable direct parts remain on zoom-out');
    assert.equal(componentContents(13/textSize,false,textSize),false,'zoom-in waits for a clear entrance');
    assert.equal(componentContents(14/textSize,false,textSize),true,'the actual part heading appears at 14px');
    const placed=new Map([frame,part].map(n=>[n.id,n]));
    assert.equal(closedContainer('utility',placed,new Map(records.map(n=>[n.id,n])),new Set(),componentContents(.7,true,textSize)),frame,
      'the 11.9px direct part is concealed when its component summary returns');
    const viewport=componentViewport(frame,[frame,part],638);
    assert.ok(textSize*viewport.zoom>=14,'the existing component entrance reveals readable direct parts');
    assert.equal(componentContents(viewport.zoom,false,textSize),true);
  }
  assert.equal(componentTextSize([{id:'front',branch:'component',children:['area']},{id:'area',branch:'area'}]),20,'area-only components use their actual 20px member names');
});

test('independently scaled components reveal only their own readable interiors',()=>{
  const records=[
    {id:'front',branch:'component',children:['views']},
    {id:'views',branch:'area',summaryScale:.25,contentScale:.05},
    {id:'backend',branch:'component',children:['utility']},
    {id:'utility',branch:'part',contentScale:.5},
  ];
  const fonts=componentTextSizes(records);
  assert.deepEqual([...fonts],[['front',5],['backend',8.5]]);
  const opened=componentDetails(fonts,2);
  assert.deepEqual([...opened],['backend'],'10px area names stay concealed while 17px direct parts are readable');
  assert.deepEqual([...componentDetails(fonts,1.5,opened)],['backend'],'Back retains readable 12.75px direct parts');
  assert.deepEqual([...componentDetails(fonts,1.4,opened)],[],'11.9px parts close with their own component');
  const root={id:'front',absolute:{x:0,y:0},width:400,height:200};
  const area={id:'views',parentId:'front',absolute:{x:8,y:40},width:100,height:140};
  const camera=componentViewport(root,[root,area],1054,fonts.get('front')/20);
  assert.equal(camera.zoom,4);
  assert.equal(fonts.get('front')*camera.zoom,20,'component entrance compensates its actual local scale');
  const placed=new Map([root,area].map(n=>[n.id,n]));
  assert.equal(closedContainer('views',placed,new Map(records.map(n=>[n.id,n])),new Set(),true,new Set(),opened),root,
    'another open component cannot reveal this component’s unreadable interior');
});

test('external entrance makes scaled call text readable and puts the first call in view',()=>{
  const frame={id:'api',absolute:{x:0,y:0},width:300,height:300};
  const call={id:'get',parentId:'api',absolute:{x:50,y:200},width:52,height:24};
  const camera=communicationViewport(frame,[frame,call],638,578,.2);
  assert.equal(camera.zoom,5,'a 17px call heading becomes 17px on screen');
  assert.equal(17*.2*camera.zoom,17);
  assert.ok(call.absolute.x*camera.zoom+camera.x>=24);
  assert.ok(call.absolute.y*camera.zoom+camera.y>=24);
  assert.ok((call.absolute.x+call.width)*camera.zoom+camera.x<=638-24);
  assert.ok((call.absolute.y+call.height)*camera.zoom+camera.y<=578-24);
  assert.deepEqual([...communicationDetails(new Map([['api',.2]]),camera.zoom)],['api']);
});

test('communication calls replace their summary only at readable scale, clipping the same routes while closed',()=>{
  const scales=new Map([['api',1],['scaled-api',.5]]);
  assert.deepEqual([...communicationDetails(scales,.5)],[],'8.5px calls stay concealed');
  assert.deepEqual([...communicationDetails(scales,.75)],[],'zooming in waits for 14px text');
  const opened=communicationDetails(scales,.85);
  assert.deepEqual([...opened],['api']);
  assert.deepEqual([...communicationDetails(scales,.75,opened)],['api'],'zooming out retains 12.75px text');
  assert.deepEqual([...communicationDetails(scales,.7,opened)],[],'text below 12px closes again');
  assert.deepEqual([...communicationDetails(scales,1.7)],['api','scaled-api'],'content scale contributes to the threshold');
  const frame={id:'api',absolute:{x:200,y:0},width:100,height:100};
  const placed=new Map([
    ['api',frame],
    ['get',{id:'get',parentId:'api',absolute:{x:225,y:35},width:20,height:20}],
    ['post',{id:'post',parentId:'api',absolute:{x:255,y:35},width:20,height:20}],
  ]);
  const records=new Map([['api',{branch:'communication'}],['get',{}],['post',{}]]);
  const route={segments:[[{x:150,y:50},{x:250,y:50}]]};
  const original=JSON.stringify({nodes:[...placed],route});
  const hidden=id=>closedContainer(id,placed,records,new Set(),true,new Set());
  assert.equal(hidden('api'),null,'the container itself stays on the map');
  assert.equal(hidden('get'),frame,'call visibility uses the closed communication frame');
  assert.equal(visibleRoute(route,null,hidden('get')),'M 150 50 L 200 50','the external route ends at the same frame boundary');
  assert.equal(visibleRoute(route,hidden('get'),hidden('post')),'','internal routes wait for readable calls');
  assert.equal(readableFocus('get',placed,records,new Set(),true,{x:24,y:24,zoom:1},800,700,new Set()),false,'a hidden call is not a readable navigation destination');
  const restored=communicationDetails(scales,.75,new Set(JSON.parse(JSON.stringify([...opened]))));
  assert.equal(closedContainer('get',placed,records,new Set(),true,restored),null,'Back retains the open side of hysteresis');
  assert.equal(visibleRoute(route,null,null),'M 150 50 L 250 50');
  assert.equal(JSON.stringify({nodes:[...placed],route}),original,'presentation does not mutate the placed world');
});

test('distant components hide all descendants and internal arrows in the same world',async()=>{
  const items=[
    {id:'front',branch:'component',title:'Front',children:['area','utility']},
    {id:'area',branch:'area',title:'Rendering',children:['handler']},
    {id:'handler',title:'Handler',width:260,height:200},
    {id:'inputs',branch:'inputs',title:'Front',children:['run']},
    {id:'run',activation:'interaction',title:'Run',width:260,height:90},
    {id:'utility',title:'Async utilities',width:260,height:90},
    {id:'api',branch:'communication',title:'Backend API',children:['get']},
    {id:'get',title:'GET /levels',width:260,height:90},
  ];
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const world=await semanticLayout(items,[{from:'handler',to:'utility'},{from:'handler',to:'get'},{from:'run',to:'handler'}],areas,1000,700);
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
      assert.equal(closed('run').id,'inputs');
      const internal=world.layout.edges.find(e=>e.to==='utility');
      assert.equal(visibleRoute(internal,closed(internal.from),closed(internal.to)),'');
    }
    const external=world.layout.edges.find(e=>e.to==='get');
    assert.ok(visibleRoute(external,closed(external.from),closed(external.to)));
    assert.equal(JSON.stringify(world.layout),before);
  }
  assert.deepEqual(frameInventory('front',new Map(areas.map(a=>[a.id,a.nodes])),records),{groups:1,areaIDs:['area'],parts:2});
  assert.equal(componentContents(.65,true),true,'return preserves the open side at 13px member text');
  assert.equal(componentContents(.65,false),false,'return preserves the closed side of hysteresis');
});


test('input collections reveal original inputs at readable scale and clip only their real implementation routes',()=>{
  const frame={id:'inputs',absolute:{x:0,y:0},width:400,height:360};
  const input={id:'submit',parentId:'inputs',absolute:{x:32,y:128},width:260,height:90};
  const implementation={id:'handler',absolute:{x:600,y:0},width:260,height:90};
  const nodes=[frame,input,implementation],placed=new Map(nodes.map(n=>[n.id,n]));
  const records=new Map([['inputs',{branch:'inputs'}],['submit',{activation:'command'}],['handler',{}]]);
  const route={from:'submit',to:'handler',segments:[[{x:292,y:170},{x:600,y:170}]],relations:[{from:'submit',to:'handler',label:'implemented in',fromSource:'cli:12'}]};
  const original=JSON.stringify({nodes,route});
  const scales=new Map([['inputs',1]]);
  let open=new Set();
  for(const zoom of [.44,1,.75,.7]){
    open=communicationDetails(scales,zoom,open);
    const closed=closedContainer('submit',placed,records,new Set(),componentContents(zoom,true),open);
    if(zoom===.44||zoom===.7){
      assert.equal(closed,frame,'unreadable input cards are replaced by their type catalogue');
      assert.equal(visibleRoute(route,closed,null),'M 400 170 L 600 170');
    }else{
      assert.equal(closed,null);
      assert.ok(17*zoom>=12,'visible input names remain readable');
      assert.equal(visibleRoute(route,null,null),'M 292 170 L 600 170');
    }
    assert.equal(JSON.stringify({nodes,route}),original,'zoom neither reparents the input nor changes its source edge');
  }
  const entrance=communicationViewport(frame,nodes,1054,578,1);
  assert.ok(17*entrance.zoom>=14);
  assert.deepEqual([...communicationDetails(scales,entrance.zoom)],['inputs']);
  assert.ok(input.absolute.x*entrance.zoom+entrance.x>=24);
  assert.ok(input.absolute.y*entrance.zoom+entrance.y>=24);
});

test('the component zoom mark follows the visible corner without moving the world',()=>{
  const node={absolute:{x:0,y:0},width:4000,height:3000};
  const original=JSON.stringify(node);
  for(const viewport of [{x:24,y:24,zoom:.4},{x:-200,y:-400,zoom:.7}]){
    const mark=zoomMarkPosition(node,viewport,1054,578);
    assert.equal(mark.x*viewport.zoom+viewport.x,1014,'right edge of the icon remains 12px inside the visible frame');
    assert.ok(Math.abs(mark.y*viewport.zoom+viewport.y-Math.max(0,viewport.y)-12)<1e-9);
    assert.equal(JSON.stringify(node),original);
  }
  assert.equal(zoomMarkPosition(node,{x:1050,y:24,zoom:1},1054,578),null,'do not draw an icon beyond a nearly offscreen frame');
  const collection={absolute:{x:0,y:0},width:160,height:44};
  assert.deepEqual(zoomMarkPosition(collection,{x:24,y:24,zoom:1},1054,578,8),{x:124,y:8},'the 28px entrance fits the collection’s measured 44px height and 8px insets');
});
