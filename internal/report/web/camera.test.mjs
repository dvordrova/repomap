import {test} from 'node:test';
import assert from 'node:assert/strict';
import {union,cameraContaining,readableTogether} from './camera.mjs';

const size={width:1000,height:600};
const inside=(box,v,pad=24)=>box.x*v.zoom+v.x>=pad-1e-6&&(box.x+box.width)*v.zoom+v.x<=size.width-pad+1e-6
  &&box.y*v.zoom+v.y>=pad-1e-6&&(box.y+box.height)*v.zoom+v.y<=size.height-pad+1e-6;

test('the union of rectangles is the rectangle that holds them all',()=>{
  assert.deepEqual(union([{x:0,y:0,width:10,height:10},null,{x:20,y:-5,width:5,height:40}]),{x:0,y:-5,width:25,height:40});
  assert.equal(union([]),null);
});

test('what already fits leaves the camera alone',()=>{
  const v={x:0,y:0,zoom:1};
  assert.deepEqual(cameraContaining({x:100,y:100,width:200,height:100},v,size),v);
});

test('a box too large for the zoom is shown whole and no smaller than needed',()=>{
  const box={x:0,y:0,width:2000,height:400},v=cameraContaining(box,{x:0,y:0,zoom:1},size);
  assert.ok(Math.abs(v.zoom-952/2000)<1e-9);
  assert.ok(inside(box,v));
});

test('the camera never zooms in',()=>{
  const v=cameraContaining({x:0,y:0,width:10,height:10},{x:50,y:50,zoom:.5},size);
  assert.equal(v.zoom,.5);
});

test('the anchor stays under the pointer when the box allows, and yields when it does not',()=>{
  const start={x:-400,y:-100,zoom:2},anchor={x:300,y:150};
  const near=cameraContaining({x:250,y:100,width:400,height:200},start,size,{anchor});
  assert.ok(Math.abs(anchor.x*near.zoom+near.x-(anchor.x*start.zoom+start.x))<1e-9);
  const far={x:250,y:100,width:1500,height:200},moved=cameraContaining(far,start,size,{anchor});
  assert.ok(inside(far,moved));
});

test('a box that cannot fit above the smallest zoom shows its beginning',()=>{
  const v=cameraContaining({x:0,y:0,width:10000,height:100},{x:0,y:0,zoom:1},size,{minZoom:.5});
  assert.equal(v.zoom,.5);assert.equal(v.x,24);
});

test('a far card that would make the near ones unreadable is left where it is',()=>{
  const part={x:400,y:250,width:100,height:40},view={x:0,y:0,zoom:1};
  const near={x:520,y:240,width:150,height:60,minZoom:.8},far={x:400,y:3000,width:150,height:60,minZoom:.8};
  assert.deepEqual(readableTogether([part],[far,near],view,size,{anchor:{x:450,y:270}}),[near]);
  const close={x:400,y:420,width:150,height:60,minZoom:.8};
  assert.deepEqual(readableTogether([part],[close,near],view,size,{anchor:{x:450,y:270}}),[near,close]);
});
