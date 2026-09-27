import test from 'node:test';
import assert from 'node:assert/strict';
import {routeDrawing} from './route-drawing.mjs';
import {emphasis} from './emphasis.mjs';

const point=(x,y)=>({x,y});
const shared=[point(100,50),point(200,50)];
const edges=()=>[
  {id:'first',from:'input-a',to:'part-a',relations:[{id:'relation-a',operations:['operation-a']}],segments:[
    [point(20,20),point(80,20),point(80,50),point(100,50)],shared,
    [point(200,50),point(220,50),point(220,20),point(250,20)],
  ]},
  {id:'second',from:'input-b',to:'part-b',relations:[{id:'relation-b',operations:['operation-b']}],segments:[
    [point(20,80),point(80,80),point(80,50),point(100,50)],shared,
    [point(200,50),point(220,50),point(220,80),point(250,80)],
  ]},
];

test('one shared route is drawn once while operation branches keep their exact selection',()=>{
  const original=edges(),before=structuredClone(original);
  const state=emphasis({operation:'operation-a',entry:'input-a'},'',()=>[],original);
  const drawing=routeDrawing(original,()=>null,state.activeEdges,true);
  assert.equal(drawing.length,5);
  const common=drawing.find(route=>route.edgeIDs.length===2);
  assert.deepEqual(common.edgeIDs,['first','second']);
  assert.equal(common.on,true);
  assert.equal(common.arrow,false,'a route continuing inside the target has no boundary arrowhead');
  assert.deepEqual(drawing.filter(route=>route.edgeIDs.length===1&&route.edgeIDs[0]==='second').map(route=>[route.on,route.dim]),[[false,true],[false,true]]);
  assert.equal(drawing.filter(route=>route.arrow).length,2,'only actual targets receive arrowheads');
  assert.deepEqual([...state.activeEdges],['first']);
  assert.deepEqual([...state.participants],['input-a','part-a']);
  assert.deepEqual(original,before,'logical relations and their source identities remain unchanged');
});

test('closed frames expose one shared outer arrow and hide both sets of inner routes',()=>{
  const source={id:'inputs',absolute:{x:0,y:0},width:100,height:100};
  const target={id:'component',absolute:{x:200,y:0},width:100,height:100};
  const drawing=routeDrawing(edges(),id=>id.startsWith('input')?source:target,new Set(['first']),true);
  assert.equal(drawing.length,1);
  assert.equal(drawing[0].path,'M 100 50 L 200 50');
  assert.deepEqual(drawing[0].edgeIDs,['first','second']);
  assert.equal(drawing[0].arrow,true);
  assert.equal(drawing[0].on,true);
  assert.equal(drawing[0].dim,false);
});

test('opening a participant never extends a cross-system arrow into its interior',()=>{
  const original=edges().map(edge=>({...edge,outerSegments:[shared]}));
  const result=routeDrawing(original,()=>null,new Set(['first']),true);
  assert.equal(result.length,1);
  assert.equal(result[0].path,'M 100 50 L 200 50');
  assert.equal(result[0].arrow,true);
  assert.deepEqual(result[0].edgeIDs,['first','second']);
  assert.equal(result[0].on,true);
});

test('a shared outer route is not painted again for every original relation',()=>{
  const original=Array.from({length:102},(_,index)=>({id:`edge-${index}`,from:`input-${index}`,to:`part-${index}`,
    relations:[{id:`relation-${index}`}],segments:[shared]}));
  const drawing=routeDrawing(original,()=>null,new Set(['edge-101']),true);
  assert.equal(drawing.length,1);
  assert.equal(drawing[0].edgeIDs.length,102);
  assert.equal(drawing[0].on,true);
  assert.equal(drawing[0].arrow,true);
});

test('reverse relations share one bidirectional physical route',()=>{
  const original=edges();original[1].possible=true;
  original.push({id:'reverse',from:'part-a',to:'input-a',relations:[],segments:[[...shared].reverse()]});
  const drawing=routeDrawing(original,()=>null,new Set());
  const common=drawing.filter(route=>route.path==='M 100 50 L 200 50');
  assert.equal(common.length,1);
  assert.equal(common[0].possible,false);
  assert.equal(drawing.filter(route=>route.path==='M 200 50 L 100 50').length,0);
  assert.equal(drawing.filter(route=>route.reverseArrow).length,1);
  assert.deepEqual([...common[0].edgeIDs].sort(),['first','reverse','second']);
});

test('grouped container routes expose the selected native boundary points',()=>{
  const source={id:'requests',absolute:{x:0,y:0},width:100,height:100};
  const target={id:'execution',absolute:{x:200,y:0},width:100,height:100};
  const drawing=routeDrawing(edges(),()=>null,new Set(),false,id=>id.startsWith('input')?source:target);
  assert.equal(drawing.length,1);
  assert.deepEqual(drawing[0].edgeIDs,['first','second']);
  assert.deepEqual(drawing[0].points,shared);
  assert.deepEqual(drawing[0].start,point(100,50));
  assert.deepEqual(drawing[0].end,point(200,50));
});

test('opened and closed area pairs share one bidirectional native route',()=>{
  const source={id:'requests',absolute:{x:0,y:0},width:100,height:100};
  const target={id:'execution',absolute:{x:200,y:0},width:100,height:100};
  const original=edges();original[1].possible=true;
  original[1].segments=[[point(20,80),point(100,80),point(200,80),point(250,80)]];
  original.push({id:'reverse',from:'part-a',to:'input-a',segments:[[...shared].reverse()]});
  const boundary=id=>id.startsWith('input')?source:target;
  for(const opened of [false,true]){
    const drawing=routeDrawing(original,opened?()=>null:boundary,new Set(['second']),false,opened?boundary:()=>null);
    assert.equal(drawing.length,1);
    assert.deepEqual(drawing.map(route=>[route.path,route.possible,route.reverseArrow]),[
      ['M 100 50 L 200 50',false,true],
    ]);
    assert.deepEqual(drawing[0].edgeIDs,['first','second','reverse']);
    assert.ok(drawing[0].on,'selecting a possible relation highlights the shared visible route');
  }
});

// An arrowhead is its connection's handle, so a drawn route names the boxes
// it runs between: the frames an area pair is clipped to, and the outer
// participants of a cross-system arrow.
test('a drawn route names the boxes at its two ends',()=>{
  const source={id:'requests',absolute:{x:0,y:0},width:100,height:100};
  const target={id:'execution',absolute:{x:200,y:0},width:100,height:100};
  const boundary=id=>id.startsWith('input')?source:target;
  assert.deepEqual(routeDrawing(edges(),()=>null,new Set(),false,boundary)[0].boxes,['requests','execution']);
  const outer=edges().map(edge=>({...edge,outerSegments:[shared],outerFrom:'inputs',outerTo:'component'}));
  assert.deepEqual(routeDrawing(outer,()=>null,new Set())[0].boxes,['inputs','component']);
});

test('an empty first clipped route cannot hide a later nonempty route for the same pair',()=>{
  const source={id:'requests',absolute:{x:0,y:0},width:100,height:100};
  const target={id:'execution',absolute:{x:200,y:0},width:100,height:100};
  const original=edges();original[0].segments=[[point(20,20),point(80,20)]];
  const before=structuredClone(original);
  const drawing=routeDrawing(original,id=>id.startsWith('input')?source:target,new Set(['second']));
  assert.equal(drawing.length,1);
  assert.equal(drawing[0].path,'M 100 50 L 200 50');
  assert.deepEqual(drawing[0].edgeIDs,['second']);
  assert.equal(drawing[0].on,true);assert.equal(drawing[0].arrow,true);
  assert.deepEqual(original,before);
});

test('the same leaf pair retains all sources on one deterministic native route',()=>{
  const original=[
    {id:'a-possible',from:'launch',to:'settings',possible:true,segments:[[point(100,70),point(200,70)]],
      relations:[{label:'reads',possible:true,fromSource:'main.py:17',toSource:'settings.py:15'}]},
    {id:'z-exact',from:'launch',to:'settings',possible:false,segments:[shared],
      relations:[{label:'imports',possible:false,fromSource:'main.py:6',toSource:'settings.py:15'}]},
  ];
  const before=structuredClone(original);
  for(const edges of [original,[...original].reverse()]){
    const drawing=routeDrawing(edges,()=>null,new Set(['a-possible']));
    assert.equal(drawing.length,1);assert.deepEqual(drawing[0].points,shared);
    assert.equal(drawing[0].possible,false);assert.equal(drawing[0].on,true);
    assert.deepEqual(drawing[0].edgeIDs.sort(),['a-possible','z-exact']);
  }
  const possibleOnly=original.map(edge=>({...edge,possible:true}));
  for(const edges of [possibleOnly,[...possibleOnly].reverse()]){
    const drawing=routeDrawing(edges,()=>null,new Set());
    assert.equal(drawing.length,1);assert.equal(drawing[0].id,'a-possible');
    assert.equal(drawing[0].possible,true,'a bundle with no exact relation remains dashed');
  }
  assert.deepEqual(original,before,'source locations and individual certainty never change');
});

test('outer endpoint identities combine different native paths across opened participants',()=>{
  const original=edges().map((edge,index)=>({...edge,possible:!!index,outerFrom:'inputs',outerTo:'component',
    outerSegments:[index?[point(100,80),point(200,80)]:shared]}));
  const drawing=routeDrawing(original,()=>null,new Set(['second']));
  assert.equal(drawing.length,1);assert.deepEqual(drawing[0].points,shared);
  assert.deepEqual(drawing[0].edgeIDs,['first','second']);
  assert.equal(drawing[0].possible,false);assert.equal(drawing[0].on,true);
});

// Initialization wiring is dashed and appears only while a box or an area at
// one of its ends is what the reader looks at; selecting the whole component
// looks at nothing in particular and keeps the runtime arrows alone.
test('init routes are drawn only for a looked-at end',()=>{
  const wiring=[
    {id:'boot',from:'bootstrap',to:'domain',init:true,relations:[{id:'r1',init:true}],segments:[[point(0,0),point(50,0)]]},
    {id:'flow',from:'handler',to:'domain',relations:[{id:'r2'}],segments:[[point(0,20),point(50,20)]]},
  ];
  const none=routeDrawing(wiring,()=>null,new Set(),false,()=>null,false);
  assert.deepEqual(none.map(route=>route.id),['flow']);
  const whole=routeDrawing(wiring,()=>null,new Set(['boot','flow']),true,()=>null,false);
  assert.deepEqual(whole.map(route=>route.id),['flow']);
  const looked=routeDrawing(wiring,()=>null,new Set(['boot']),true,()=>null,true);
  assert.deepEqual(looked.map(route=>[route.id,route.init,route.dim]),[['boot',true,false],['flow',false,true]]);  // Zoomed into the area that holds bootstrap, with nothing hovered.
  const zoomed=routeDrawing(wiring,()=>null,new Set(),false,()=>null,false,new Set(['bootstrap']));
  assert.deepEqual(zoomed.map(route=>route.id),['boot','flow']);
  const elsewhere=routeDrawing(wiring,()=>null,new Set(),false,()=>null,false,new Set(['other']));
  assert.deepEqual(elsewhere.map(route=>route.id),['flow']);
});

test('an arrow between the looked-at frame\'s own parts stays while the rest recedes',()=>{
  const edges=[{id:'inner',from:'a',to:'b',segments:[[{x:0,y:0},{x:10,y:0}]]},{id:'far',from:'c',to:'d',segments:[[{x:0,y:10},{x:10,y:10}]]}];
  const routes=routeDrawing(edges,()=>null,new Set(),true,()=>null,false,null,new Set(['area','a','b']));
  assert.equal(routes.find(route=>route.id==='inner').dim,false);
  assert.equal(routes.find(route=>route.id==='far').dim,true);
});
