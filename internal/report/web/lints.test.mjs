// The reading lints' pure rules (visual/journeys.mjs), held without a
// browser: a place written as file:line is a path with a file extension
// and a line, never an address with a port.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {fileLine} from './visual/journeys.mjs';

test('a file and its line are a place; an address and its port are not',()=>{
  for(const text of ['db.go:1067','see cmd/litestream/main.go:42 here','(src/othello/ui/events.cljc:52)','redis.c:1234:5','beets/util/__init__.py:88'])
    assert.notEqual(fileLine(text),'',`${text} names a place`);
  for(const text of ['8.8.8.8:80','Google Public DNS 8.8.8.8:80','localhost:2379','example.com:443','api.github.com:443','10.0.0.1:2380','Route 53','Python 3','v3.5.0'])
    assert.equal(fileLine(text),'',`${text} names no place`);
});
