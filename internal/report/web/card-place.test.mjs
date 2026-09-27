import {test} from 'node:test';
import assert from 'node:assert/strict';
import {placeCard} from './card-place.mjs';

const canvas={left:0,top:0,right:1000,bottom:600};
const inside=(at,size)=>at.x>=canvas.left+8&&at.y>=canvas.top+8&&at.x+size.width<=canvas.right-8&&at.y+size.height<=canvas.bottom-8;

// The designer's 07c: Set commands' number in Data type commands, the area
// at the canvas's left. Its card goes out of the area on the right, level
// with the number, and covers none of the area's parts.
test('a part number\'s card stands outside its frame, level with the number',()=>{
  const size={width:360,height:280};
  const at=placeCard({handle:{left:470,top:350,right:490,bottom:366},frame:{left:40,top:100,right:520,bottom:580},canvas,size});
  assert.deepEqual(at,{x:528,y:86},"its bottom level with the number: there is no room below");
  assert.ok(inside(at,size));
});

// The designer's 08c: a label on Core infrastructure's left border. Its card
// goes out through that border.
test('a label\'s card goes out through the border its label stands on',()=>{
  const size={width:320,height:170};
  const at=placeCard({handle:{left:600,top:200,right:620,bottom:290},frame:{left:590,top:40,right:1200,bottom:900},canvas,size,side:'left'});
  assert.deepEqual(at,{x:262,y:200});
  const top=placeCard({handle:{left:300,top:95,right:360,bottom:110},frame:{left:100,top:90,right:900,bottom:500},canvas:{...canvas,top:-400},size,side:'top'});
  assert.equal(top.y,90-8-170,'a label on the top border opens upward');
});

test('with no room outside the frame the card stands beside its handle, inside the canvas',()=>{
  const size={width:400,height:300};
  const at=placeCard({handle:{left:900,top:560,right:920,bottom:576},frame:{left:0,top:0,right:1000,bottom:600},canvas,size});
  assert.equal(at.x,900-8-400,'toward the roomier side');
  assert.equal(at.y,576-300,'bottom aligned when there is no room below');
  assert.ok(inside(at,size));
  const wide=placeCard({handle:{left:10,top:10,right:30,bottom:30},frame:null,canvas,size:{width:2000,height:100}});
  assert.equal(wide.x,8,'a card wider than the canvas starts at its left edge');
});
