import test from 'node:test';
import assert from 'node:assert/strict';
import {callCard,reach} from './call-card.mjs';

const names={persist:'Persistence',clients:'Client connections',data:'Data structures',strings:'Strings',
  generic:'Generic key commands',lists:'List commands',get:'get',inputs:'Inputs'};
const nameOf=id=>names[id]||id;
const call=(label,at,extra={})=>({label,from:`h/${at}`,to:`d/${label.split(' ')[2]}`,at,...extra});

// Server runtime → Core infrastructure: calls grouped by the part they are
// made from, then the part they go into, the bigger first, each group's
// calls in the order they are written.
test('calls stand under their caller part and the part they go into, in source order',()=>{
  const card=callCard([
    {from:'persist',to:'data',calls:[call('rdbSave calls dictNext','redis.c:3292'),call('rdbSaveObject calls listRewind','redis.c:3170'),call('rdbSaveObject calls listNext','redis.c:3171')]},
    {from:'persist',to:'strings',calls:[call('rdbSaveObject calls sdslen','redis.c:3180')]},
    {from:'clients',to:'data',calls:[call('freeClient calls listRelease','redis.c:1807'),call('freeClient calls listRelease','redis.c:1810')]},
  ],{nameOf});
  assert.equal(card.total,5,'a caller calling one callee at two sites is one call');
  assert.deepEqual(card.kinds,[['calls',5]]);
  assert.deepEqual(card.from.map(p=>[p.name,p.count]),[['Persistence',4],['Client connections',1]]);
  assert.deepEqual(card.into.map(p=>[p.name,p.count]),[['Data structures',4],['Strings',1]]);
  const persist=card.groups[0];
  assert.equal(persist.name,'Persistence');
  assert.deepEqual(persist.pairs.map(p=>[p.name,p.count]),[['Data structures',3],['Strings',1]]);
  assert.deepEqual(persist.pairs[0].rows.map(r=>`${r.caller}>${r.callee}`),['rdbSaveObject>listRewind','rdbSaveObject>listNext','rdbSave>dictNext'],'rows follow their call sites');
  assert.equal(persist.pairs[0].rows[0].site,'h/redis.c:3170','the caller leads to where the call is written');
  assert.equal(persist.pairs[0].rows[0].calleeHref,'d/listRewind','the callee leads to its declaration');
});

// "call → xCommand" and "cmdTable passes callback xCommand", 77 rows each,
// fold into one line each, the callees by part under it.
test('a dispatch site and a set handed over whole are one line each',()=>{
  const dispatch=(callee,at)=>call(`call calls ${callee}`,'redis.c:2054',{fold:'t1/f6',of:94,one:true});
  const handed=(callee,line)=>call(`cmdTable passes_callback ${callee}`,`redis.c:${line}`,{fold:'t1/f6',of:94,same:'call'});
  const card=callCard([
    {from:'clients',to:'generic',calls:[dispatch('delCommand'),dispatch('existsCommand'),handed('delCommand',709),handed('existsCommand',710)]},
    {from:'clients',to:'lists',calls:[dispatch('lpushCommand'),handed('lpushCommand',720),call('addReply calls listAddNodeTail','redis.c:2480')]},
  ],{nameOf});
  assert.equal(card.total,7);
  assert.deepEqual(card.kinds,[['calls',4],['passes_callback',3]]);
  const clients=card.groups[0];
  assert.equal(clients.count,7);
  assert.deepEqual(clients.folds.map(f=>[f.caller,f.count,f.of,f.one,f.same]),[['call',3,94,true,''],['cmdTable',3,94,false,'call']]);
  assert.deepEqual(clients.folds[0].parts.map(p=>[p.name,p.count,p.rows.map(r=>r.callee)]),[['Generic key commands',2,['delCommand','existsCommand']],['List commands',1,['lpushCommand']]]);
  assert.equal(clients.folds[0].site,'h/redis.c:2054');
  assert.deepEqual(clients.pairs.map(p=>[p.name,p.rows.map(r=>r.callee)]),[['List commands',['listAddNodeTail']]],'a call outside the fold stays its own row');
});

// An input is not a part calls are made from: "get → getCommand" rows stand
// under the part they reach, not in a group of one per input.
test('calls from inputs are grouped by the part they reach only',()=>{
  const card=callCard([
    {from:'get',to:'strings',calls:[{label:'implemented in',name:'getCommand',to:'d/getCommand'}]},
    {from:'set',to:'strings',calls:[{label:'implemented in',name:'setCommand',to:'d/setCommand'}]},
  ],{nameOf,groupable:id=>!['get','set'].includes(id),incoming:true});
  assert.equal(card.groups.length,1);
  assert.equal(card.groups[0].id,'');
  assert.deepEqual(card.groups[0].pairs[0].rows.map(r=>`${r.caller}>${r.callee}`),['get>getCommand','set>setCommand']);
  assert.deepEqual(card.kinds,[['implemented in',2]]);
  assert.deepEqual(card.from,[]);
});

test('a relation with no call of its own is read by its outside end and site',()=>{
  const card=callCard([{from:'clients',to:'tcp',label:'connects to',fromSource:'h/anet.c:158',calls:[]}],{nameOf:id=>({tcp:'TCP endpoint'})[id]||id});
  assert.deepEqual(card.groups[0].pairs[0].rows.map(r=>[r.kind,r.other,r.otherHref]),[['other','TCP endpoint','h/anet.c:158']]);
});

test('reach says all, some of, or nothing for a single part',()=>{
  assert.deepEqual(reach(8,8),{all:true,count:8});
  assert.deepEqual(reach(8,9),{all:false,count:8,of:9});
  assert.equal(reach(1,1),null);
});
