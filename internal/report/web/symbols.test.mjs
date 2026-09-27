import {test} from 'node:test';
import assert from 'node:assert/strict';
import {symbolBlocks,symbolRow,tileGrid} from './symbols.mjs';

const type=name=>({name}),method=(name,owner)=>({name,owner}),fn=name=>({name});

test('a method is a row of its type and a returned type stands to the right',()=>{
  // 0 UserSerializer, 1 its Response method, 2 UserResponse; Response returns UserResponse.
  const {blocks,rows,hidden}=symbolBlocks([type('UserSerializer'),method('Response',1),type('UserResponse')],[[1,2,'returns']],3,400);
  assert.equal(hidden,0);
  assert.deepEqual(blocks.map(block=>[block.head,block.rows,block.column]),[[0,[1],0],[2,[],1]]);
  assert.equal(rows[1].y,symbolRow.header);
  assert.equal(rows[1].column,0);
  assert.equal(rows[1].block,rows[0].block);
  assert.notEqual(rows[2].block,rows[0].block);
});

test('a method whose type is not in the part stands alone',()=>{
  const {blocks}=symbolBlocks([method('Orphan.run',7),fn('main')],[[1,0,'calls']],2,200);
  assert.deepEqual(blocks.map(block=>[block.head,block.column]).sort(),[[0,1],[1,0]]);
});

test('a column too tall spills into the next and what finds no room is counted',()=>{
  const symbols=[fn('a'),fn('b'),fn('c'),fn('d'),fn('e')];
  const {blocks,hidden}=symbolBlocks(symbols,[],2,2*symbolRow.header+symbolRow.gap);
  assert.equal(blocks.length,4);
  assert.equal(hidden,1);
  assert.deepEqual(blocks.map(block=>block.column),[0,0,1,1]);
});

test('a cycle of calls still places every declaration once',()=>{
  const {blocks,hidden}=symbolBlocks([fn('a'),fn('b')],[[0,1,'calls'],[1,0,'calls']],2,200);
  assert.equal(blocks.length,2);assert.equal(hidden,0);
});

test('fields stand above methods and a skipped declaration takes no room',()=>{
  const symbols=[type('User'),method('save',1),{name:'name',owner:1,kind:'field'},{name:'loose',kind:'skip'}];
  const {blocks,rows}=symbolBlocks(symbols,[],2,400);
  assert.deepEqual(blocks.map(block=>block.rows),[[2,1]]);
  assert.equal(rows[3],null);
});

test('a type taller than the card shows the rows that fit and counts the rest',()=>{
  const symbols=[type('Service'),...Array.from({length:12},(_,i)=>method('m'+i,1))];
  const height=symbolRow.header+symbolRow.pad+5*symbolRow.row;
  const {blocks,rows,hidden}=symbolBlocks(symbols,[],2,height);
  assert.equal(blocks[0].rows.length,4);
  assert.equal(blocks[0].more,8);
  assert.ok(blocks[0].height<=height);
  assert.equal(rows[12],null,'a row that is not drawn anchors no link');
  assert.equal(hidden,0);
});

// Placed by link column first, Redis's Data structures drew ten zipmap helpers
// and counted list, a key's return type, in its "+59".
test('tiles are placed in the listed order, keys first, each in its own link column',()=>{
  const key=(name,extra={})=>({name,key:true,...extra});
  // 0 listCreate returns 1 list; six helpers follow in the page's order.
  const symbols=[key('listCreate'),key('list',{kind:'type'}),...Array.from({length:6},(_,i)=>fn('helper'+i))];
  const height=3*symbolRow.header+2*symbolRow.gap;
  const {blocks,hidden}=symbolBlocks(symbols,[[0,1,'returns']],2,height);
  const drawn=new Set(blocks.map(block=>block.head));
  assert.ok(drawn.has(0)&&drawn.has(1),'every key is drawn');
  assert.equal(blocks.find(block=>block.head===1).column,1,'list keeps its link column right of what returns it');
  assert.deepEqual(blocks.filter(block=>block.column===0).map(block=>block.head),[0,2,3],'a column stacks its tiles in list order');
  assert.equal(hidden,2);
  assert.ok(!drawn.has(7)&&!drawn.has(6),'what finds no room is the end of the list');
});

test('a key is never counted while a declaration after the keys is drawn',()=>{
  // A key type too tall for the card, and a small function that would fit.
  const symbols=[{name:'start',key:true},{name:'Config',key:true},...['a','b','c'].map(name=>({name,owner:2,kind:'field'})),fn('helper')];
  const height=2*symbolRow.header+symbolRow.gap;
  const {blocks,hidden}=symbolBlocks(symbols,[],1,height);
  assert.deepEqual(blocks.map(block=>block.head),[0],'the helper does not take the place the key had no room in');
  assert.equal(hidden,5,'the type with its three fields and the helper are counted');
  // A type whose key is one of its methods stands among the keys.
  const typed=[{name:'run',owner:4,key:true},fn('first'),fn('second'),type('Worker')];
  const one=symbolBlocks(typed,[],1,2*symbolRow.header+symbolRow.row+symbolRow.pad+symbolRow.gap);
  assert.deepEqual(one.blocks.map(block=>block.head),[3,1]);
  assert.equal(one.hidden,1);
});

// A monospace measure: every character 8px wide.
const measure=text=>text.length*8;
test('a tile is as wide as the longest name and every tile fits whole',()=>{
  const names=['_dictStringCopyHTKeyCompare','listCreate','dictAdd',...Array.from({length:60},(_,i)=>'helper'+i)];
  const grid=tileGrid(names.map(name=>({name})),[],{width:260,height:88},measure);
  assert.equal(grid.hidden,0,'nothing is counted away');
  assert.ok(grid.tileWidth>=27*8+22,'the longest name is not cut');
  assert.ok(grid.divisor>4,'a part with more tiles than its card holds at a quarter is drawn smaller');
  assert.equal(tileGrid(names.slice(0,3).map(name=>({name})),[],{width:260,height:88},measure).divisor,4,'a small part keeps the quarter scale');
});

test('tiles stand by file, files in the order their first declaration is listed',()=>{
  const symbols=[{name:'dictAdd',key:true,path:'dict.c'},{name:'listCreate',key:true,path:'adlist.c'},{name:'dictFind',path:'dict.c'},
    {name:'listAddNodeTail',path:'adlist.c'},{name:'zipmapNew',path:'zipmap.c'},{name:'dictNext',path:'dict.c'}];
  const grid=tileGrid(symbols,[],{width:260,height:200},measure);
  assert.equal(grid.hidden,0);
  const column=grid.blocks.filter(block=>block.column===0).sort((a,b)=>a.y-b.y).map(block=>symbols[block.head].name);
  assert.deepEqual(column,['dictAdd','dictFind','dictNext','listCreate','listAddNodeTail','zipmapNew']);
});

// However many declarations a part holds, none is counted away: the search
// for the scale that holds them has no ceiling of its own. Stopped at a
// divisor of 64, a small card of 600 declarations counted many of them away.
test('a part of any size draws every declaration whole',()=>{
  const symbols=Array.from({length:600},(_,i)=>({name:'handler'+i}));
  const grid=tileGrid(symbols,[],{width:20,height:30},measure);
  assert.equal(grid.hidden,0,'nothing is counted away');
  assert.equal(grid.blocks.length,600);
});
