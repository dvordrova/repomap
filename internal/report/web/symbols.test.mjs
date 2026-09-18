import {test} from 'node:test';
import assert from 'node:assert/strict';
import {symbolCells} from './symbols.mjs';

test('callers stand left of what they call and loose declarations fill the rest',()=>{
  // 0 calls 1, 1 calls 2; 3 and 4 are called by nobody and call nothing.
  const cells=symbolCells(5,[[0,1],[1,2]],3,3);
  assert.deepEqual(cells.slice(0,3),[{column:0,row:0},{column:1,row:0},{column:2,row:0}]);
  assert.deepEqual(cells.slice(3),[{column:0,row:1},{column:1,row:1}]);
});

test('a cycle and a chain longer than the card still place every declaration once',()=>{
  const cells=symbolCells(4,[[0,1],[1,0],[1,2],[2,3]],2,4);
  assert.equal(new Set(cells.map(cell=>`${cell.column}:${cell.row}`)).size,4);
  assert.ok(cells.every(cell=>cell.column<2&&cell.row<4));
});

test('a column too tall for the card gives the plain order back',()=>{
  const cells=symbolCells(4,[[0,1],[0,2],[0,3]],2,2);
  assert.deepEqual(cells,[{column:0,row:0},{column:1,row:0},{column:0,row:1},{column:1,row:1}]);
});
