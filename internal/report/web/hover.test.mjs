import {test} from 'node:test';
import assert from 'node:assert/strict';
import {HoverGate} from './hover.mjs';

test('a navigation or click keeps selected reading until the pointer actually moves',()=>{
  const hover=new HoverGate();
  hover.remember(300,200);hover.pause();
  assert.equal(hover.allowed,false,'a newly revealed neighbour cannot preview itself');
  assert.equal(hover.move(300,200),false,'same-point browser events after layout do not replace reading');
  assert.equal(hover.move(301,200),false,'tiny event rounding does not replace reading');
  assert.equal(hover.move(310,200),true,'intentional movement resumes exploration');
  hover.pause();
  assert.equal(hover.move(310,200),false,'camera movement uses the same rule');
  assert.equal(hover.move(320,200),true);
});
