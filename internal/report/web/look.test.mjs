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
  look.enter('label:p1');
  look.cameraMoves(100,100,500);
  assert.equal(look.leave('label:p1',200),0);
  assert.equal(look.move(140,100,300,false),false,'the pointer during the move resets where the look is held from');
  assert.equal(look.tick(5000),false);
  assert.equal(look.move(145,100,600,false),false,'a small movement after the move keeps it');
  assert.equal(look.move(200,100,700,false),true,'a real movement away ends it');
  assert.equal(look.key,'');
});

test('moving on the thing itself after the camera move keeps the look',()=>{
  const look=createLook();
  look.enter('label:p1');look.cameraMoves(0,0,100);
  assert.equal(look.move(50,0,200,true),false);
  assert.equal(look.key,'label:p1');
  assert.ok(look.leave('label:p1',300)>0,'and it can be left as any other look');
});

test('another thing takes the look and drops the hold',()=>{
  const look=createLook();
  look.enter('label:p1');look.cameraMoves(0,0,100);
  look.enter('label:7');
  assert.equal(look.held,false);
  assert.ok(look.leave('label:7',50)>0);
});

test('a look opens on intent: resting on the handle opens it, crossing it does not',()=>{
  const look=createLook({dwell:120});
  const ask=look.aim('label:p1',1000);
  assert.equal(ask,1120);
  assert.equal(look.tick(1100),false,'not yet');
  assert.equal(look.key,'');
  assert.equal(look.tick(1121),true,'the pointer stayed: the card opens');
  assert.equal(look.key,'label:p1');
  look.aim('label:p2',2000);look.abandon('label:p2');
  assert.equal(look.tick(3000),false,'a handle crossed on the way opens nothing');
  assert.equal(look.key,'label:p1');
});

// The owner's 19.png and the designer's paths 07c→07d and 08c→08d: the card
// stands beside its handle and the way to it crosses other handles and
// empty canvas. The look survives that way and nothing else takes it.
test('on the way from the handle to its card the look lasts and no other handle takes it',()=>{
  const look=createLook({linger:260,dwell:120});
  look.enter('label:p1');
  const card={left:300,top:100,right:600,bottom:400};
  look.leave('label:p1',0,{x:100,y:250},card);
  // Slowly on the way: long past the linger, still inside the triangle.
  for(let t=50,x=110;x<290;t+=50,x+=20){assert.equal(look.move(x,250,t,false),false);assert.equal(look.tick(t),false,`at ${x}`);}
  assert.equal(look.aim('label:boundary',500),0,'another handle on the way does not take the look');
  assert.equal(look.tick(600),false,'the pointer is still on its way');
  assert.equal(look.key,'label:p1');
  look.enter('label:p1');
  assert.equal(look.key,'label:p1','reaching the card keeps it');
});

test('leaving the triangle is leaving',()=>{
  const look=createLook({linger:260});
  look.enter('label:1');
  const card={left:300,top:100,right:600,bottom:400};
  look.leave('label:1',0,{x:100,y:250},card);
  look.move(120,250,20,false);
  look.move(120,40,40,false);
  assert.equal(look.safe,false);
  assert.equal(look.tick(200),false,'the ordinary linger still runs');
  assert.equal(look.tick(301),true);
  assert.equal(look.key,'');
  assert.ok(look.aim('label:2',400)>0,'another handle takes the pointer again');
});
