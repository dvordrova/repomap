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

test('possible relations and the reverse direction retain distinct route styling',()=>{
  const original=edges();original[1].possible=true;
  original.push({id:'reverse',from:'part-a',to:'input-a',relations:[],segments:[[...shared].reverse()]});
  const drawing=routeDrawing(original,()=>null,new Set());
  const common=drawing.filter(route=>route.path==='M 100 50 L 200 50');
  assert.equal(common.length,2);
  assert.deepEqual(common.map(route=>route.possible),[false,true]);
  assert.equal(drawing.filter(route=>route.path==='M 200 50 L 100 50').length,1);
});
