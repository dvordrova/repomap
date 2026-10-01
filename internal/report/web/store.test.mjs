// The scene canvas's store (store.mjs): the level changes by an action or
// a zoom, never a pan; one pinch crosses one level boundary (scene.mjs
// pinchLimit).
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createStore,sceneReducer,initialState,zoomAction,createCamera} from './store.mjs';
import {pinchLimit} from './scene.mjs';

test('a pan asks for no level; a zoom asks with its direction',()=>{
  assert.equal(zoomAction(1,{x:40,y:-20,zoom:1},{x:0,y:0}),null);
  assert.deepEqual(zoomAction(1,{zoom:2},{x:3,y:4}),{type:'zoom',zoom:2,aim:{x:3,y:4},zoomingIn:true});
  assert.equal(zoomAction(2,{zoom:1},{x:0,y:0}).zoomingIn,false);
});

test('the store keeps its state when an action changes nothing, and tells its readers once when it does',()=>{
  const model={nodes:new Map([['p',{kind:'program'}]]),ancestors:()=>[]};
  const store=createStore(sceneReducer({model,geometry:()=>null,scene:()=>null}),initialState);
  let told=0;store.subscribe(()=>told++);
  store.dispatch({type:'enter',level:['p']});
  store.dispatch({type:'enter',level:['p']});
  store.dispatch({type:'point',target:{type:'box',id:'p'}});
  store.dispatch({type:'point',target:{type:'box',id:'p'}});
  store.dispatch({type:'unknown'});
  assert.deepEqual(store.getState().level,['p']);
  assert.equal(told,2);
});

test('the camera tells its readers only of a move',()=>{
  const camera=createCamera({x:0,y:0,zoom:1});
  let told=0;camera.subscribe(()=>told++);
  camera.set({x:0,y:0,zoom:1});camera.set({x:5,y:0,zoom:1});
  assert.equal(told,1);
});

test('one pinch crosses one level boundary and stops short of the next',()=>{
  // Depths 0 below zoom 2, 1 from 2, 2 from 4.
  const depthAt=zoom=>zoom>=4?2:zoom>=2?1:0;
  const gesture={depth:0};
  assert.equal(pinchLimit(1,1.5,depthAt,gesture),1.5,'within a level the pinch goes on');
  const held=pinchLimit(1.5,8,depthAt,gesture);
  assert.ok(held>=2&&held<4,`held at ${held}, past the first boundary and short of the second`);
  assert.equal(pinchLimit(held,1,depthAt,gesture),1,'and back across it');
});
