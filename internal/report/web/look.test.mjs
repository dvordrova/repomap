import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createLook} from './look.mjs';

test('leaving lingers, so the pointer can cross to what the look opened',()=>{
  const look=createLook();
  look.enter('label:1');
  const ask=look.leave('label:1',1000);
  assert.equal(look.tick(ask-1),false);
  look.enter('label:1');
  assert.equal(look.tick(ask+1),false,'coming back in time keeps the look');
  assert.equal(look.key,'label:1');
});

test('a look that ended begins again on the next entry',()=>{
  const look=createLook();
  look.enter('label:1');
  assert.equal(look.tick(look.leave('label:1',0)),true);
  assert.equal(look.key,'');
  assert.equal(look.enter('label:1'),true);
  assert.equal(look.key,'label:1');
});

test('the thing sliding from under a still pointer during the camera move is not leaving',()=>{
  const look=createLook();
  look.enter('badge:p1');
  look.cameraMoves(100,100,500);
  assert.equal(look.leave('badge:p1',200),0);
  assert.equal(look.move(140,100,300,false),false,'the pointer during the move resets where the look is held from');
  assert.equal(look.tick(5000),false);
  assert.equal(look.move(145,100,600,false),false,'a small movement after the move keeps it');
  assert.equal(look.move(200,100,700,false),true,'a real movement away ends it');
  assert.equal(look.key,'');
});

test('moving on the thing itself after the camera move keeps the look',()=>{
  const look=createLook();
  look.enter('badge:p1');look.cameraMoves(0,0,100);
  assert.equal(look.move(50,0,200,true),false);
  assert.equal(look.key,'badge:p1');
  assert.ok(look.leave('badge:p1',300)>0,'and it can be left as any other look');
});

test('another thing takes the look and drops the hold',()=>{
  const look=createLook();
  look.enter('badge:p1');look.cameraMoves(0,0,100);
  look.enter('label:7');
  assert.equal(look.held,false);
  assert.ok(look.leave('label:7',50)>0);
});
