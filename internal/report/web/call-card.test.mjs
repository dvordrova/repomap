import test from 'node:test';
import assert from 'node:assert/strict';
import {callCard,reach,countWords,countsHandlers,countsInputs,headingRows,briefCard} from './call-card.mjs';

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
  assert.equal(countsHandlers(card),false,'a card of calls counts calls');
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
  assert.equal(countsHandlers(card),true);
});

// Two inputs with one handler (Redis's sinter and smembers, both
// sinterCommand) had been one row naming sinter: smembers vanished and the
// card counted "94 inputs" beside 96. The row names both inputs and the
// card counts it as one handler.
test('inputs sharing a handler are one row naming each input, counted as one handler',()=>{
  const card=callCard([
    {from:'sinter',to:'sets',calls:[{label:'implemented in',name:'sinterCommand',to:'d/sinterCommand'}]},
    {from:'smembers',to:'sets',calls:[{label:'implemented in',name:'sinterCommand',to:'d/sinterCommand'}]},
    {from:'sadd',to:'sets',calls:[{label:'implemented in',name:'saddCommand',to:'d/saddCommand'}]},
  ],{nameOf,groupable:id=>false,incoming:true});
  assert.equal(card.total,2,'the card counts handlers');
  assert.equal(countsHandlers(card),true);assert.equal(countWords['implemented in'],'{0} handlers','its count says handlers');
  assert.deepEqual(card.groups[0].pairs[0].rows.map(r=>`${r.caller}>${r.callee}`),['sadd>saddCommand','sinter, smembers>sinterCommand']);
  // Each input keeps its node, so the reading column reads it.
  assert.deepEqual(card.groups[0].pairs[0].rows[1].inputRefs,[{id:'sinter',name:'sinter'},{id:'smembers',name:'smembers'}]);
});

// redis-cli's options and cmdTable rows have no established handler: their
// arrows go into the parts taking them in. The card names the inputs by where
// they are taken in and counts inputs, never handlers; an input its
// component takes in with no call of its own is named alone.
test('inputs taken in, not handled, are one line per place and counted as inputs',()=>{
  const declared=name=>({label:'declared in',name:'parseOptions',to:'d/parseOptions'});
  const looked={label:'looked up in',name:'lookupCommand',to:'d/lookupCommand'};
  const card=callCard([
    {from:'-h',to:'cli',calls:[declared()]},{from:'-p',to:'cli',calls:[declared()]},
    {from:'get',to:'cli',calls:[looked]},{from:'set',to:'cli',calls:[looked]},{from:'del',to:'cli',calls:[looked]},
    {from:'info',to:'component',calls:[],label:''},{from:'ping',to:'component',calls:[],label:''},
  ],{nameOf,groupable:()=>false,incoming:true});
  assert.deepEqual(card.kinds,[['inputs',7]]);
  assert.equal(countsHandlers(card),false,'no handler is counted');
  assert.equal(countsInputs(card),7);
  const rows=card.groups[0].pairs.flatMap(pair=>pair.rows.map(r=>[r.caller,r.kind,r.callee]));
  assert.deepEqual(rows.sort(),[['-h, -p','declared in','parseOptions'],['del, get, set','looked up in','lookupCommand'],['info, ping','input','']]);
});

test('a relation with no call of its own is read by its outside end and site',()=>{
  const card=callCard([{from:'clients',to:'tcp',label:'connects to',fromSource:'h/anet.c:158',calls:[]}],{nameOf:id=>({tcp:'TCP endpoint'})[id]||id});
  assert.deepEqual(card.groups[0].pairs[0].rows.map(r=>[r.kind,r.other,r.otherHref]),[['other','TCP endpoint','h/anet.c:158']]);
});

// litestream's Core database engine → SQLite: each statement a function
// runs is an end of its own, named by that function, and its relation has
// no call of its own. Ends of one name are one heading ("→
// checkpointWithExecutor 2", not the heading twice); in the reading column a
// row that only names its heading again is not repeated, while a call keeps
// its row.
test('ends of one name are one heading, and a row naming only its heading is not repeated',()=>{
  const sql=(at)=>({label:'runs SQL',from:`h/db.go:${at}`,at:`db.go:${at}`});
  const card=callCard([
    {from:'engine',to:'stmt-lock',calls:[sql(1186)]},
    {from:'engine',to:'stmt-checkpoint-1',calls:[sql(1240)]},
    {from:'engine',to:'stmt-checkpoint-2',calls:[sql(1252)]},
    {from:'engine',to:'begin',calls:[call('acquireReadLock calls BeginTx','db.go:1190')]},
  ],{nameOf:id=>({'stmt-lock':'acquireReadLock','stmt-checkpoint-1':'checkpointWithExecutor','stmt-checkpoint-2':'checkpointWithExecutor',begin:'BeginTx',engine:'Core database engine'})[id]||id});
  const group=card.groups[0];
  assert.deepEqual(group.pairs.map(pair=>[pair.name,pair.count]),[['checkpointWithExecutor',2],['acquireReadLock',1],['BeginTx',1]]);
  assert.deepEqual(card.into.length,4,'the ends are still counted apart');
  assert.deepEqual(headingRows(group.pairs[0],group),[],'no row repeats the heading');
  assert.deepEqual(headingRows(group.pairs[2],group).map(row=>`${row.caller}>${row.callee}`),['acquireReadLock>BeginTx'],'a call keeps its row');
});

// A name in the reading column reads its declaration: each end of a call
// carries the part it is read in and the declaration's own key from the
// page data, never the call site (redis-cli's joint is written at
// anet.c:158 and lands at anet.c:256, inside anetAccept declared at 248).
test('a call names the declaration at each end and the part it is read in',()=>{
  const joint={label:'anetTcpGenericConnect connects_to anetAccept',from:'h/anet.c:158',to:'h/anet.c:256',at:'anet.c:158',caller:'d/anet.c:128',callee:'d/anet.c:248'};
  const card=callCard([
    {from:'sockets',to:'networking',calls:[joint,call('anetTcpConnect calls connect','anet.c:170')]},
    {from:'get',to:'strings',calls:[{label:'implemented in',name:'getCommand',to:'d/getCommand',callee:'d/getCommand'}]},
    {from:'clients',to:'generic',calls:[call('call calls delCommand','redis.c:2054',{fold:'t1/f6',of:94,one:true,caller:'d/call',callee:'d/delCommand'})]},
  ],{nameOf,groupable:id=>id!=='get'});
  const rows=card.groups.flatMap(group=>group.pairs.flatMap(pair=>pair.rows));
  const joined=rows.find(row=>row.callee==='anetAccept');
  assert.deepEqual([joined.callerAt,joined.calleeAt],[{part:'sockets',key:'d/anet.c:128'},{part:'networking',key:'d/anet.c:248'}]);
  assert.equal(joined.site,'h/anet.c:158','the row\'s code is still where the call is written');
  const unnamed=rows.find(row=>row.callee==='connect');
  assert.deepEqual([unnamed.callerAt,unnamed.calleeAt],[null,null],'a call without declaration keys names nothing to read');
  assert.deepEqual(rows.find(row=>row.callee==='getCommand').calleeAt,{part:'strings',key:'d/getCommand'},'an input\'s handler is read in its part');
  const fold=card.groups.find(group=>group.id==='clients').folds[0];
  assert.deepEqual([fold.callerAt,fold.parts[0].rows[0].calleeAt],[{part:'clients',key:'d/call'},{part:'generic',key:'d/delCommand'}]);
});

test('reach says all, some of, or nothing for a single part',()=>{
  assert.deepEqual(reach(8,8),{all:true,count:8});
  assert.deepEqual(reach(8,9),{all:false,count:8,of:9});
  assert.equal(reach(1,1),null);
});

// Litestream's CLI → Core database engine card had listed 174 caller and
// callee rows: on the canvas each part it goes into names what it reaches
// there once.
test('an arrow card on the canvas names each callee once under the part it goes into',()=>{
  const relations=[{from:'persist',to:'data',calls:[call('rdbSave calls dictNext','rdb.c:10'),call('rdbLoad calls dictNext','rdb.c:40'),call('rdbSave calls sdsnew','rdb.c:12')]},
    {from:'clients',to:'strings',calls:[call('readQuery calls sdsnew','net.c:5')]}];
  const brief=briefCard(callCard(relations,{nameOf}));
  assert.deepEqual(brief.map(part=>part.name),['Data structures','Strings']);
  assert.deepEqual(brief[0].names.map(entry=>entry.name),['dictNext','sdsnew']);
  assert.deepEqual(brief[1].names.map(entry=>entry.name),['sdsnew']);
});

// redis-cli → redis-server: the exchange, each side's own function.
test('an arrow card names a call leaving its program by the functions on each side',()=>{
  const relations=[{from:'clients',to:'data',calls:[call('anetTcpGenericConnect connects_to anetAccept','anet.c:158',{sides:[
    {program:'redis-cli',path:[{name:'cliConnect'},{name:'anetTcpConnect'}]},{program:'redis-server',path:[{name:'acceptHandler'},{name:'anetAccept'}]}]})]}];
  assert.deepEqual(briefCard(callCard(relations,{nameOf})).map(part=>part.names.map(entry=>entry.name)),[['cliConnect ⇢ acceptHandler']]);
});
