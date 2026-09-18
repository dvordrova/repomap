import {test} from 'node:test';
import assert from 'node:assert/strict';
import {symbolBlocks,symbolRow} from './symbols.mjs';

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
