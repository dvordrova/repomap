import {test} from 'node:test';
import assert from 'node:assert/strict';
import {semanticLayout,visibleRoute} from './semantic.mjs';
import {inputGroupsByPart} from './overview.mjs';

test('overview folds only saved areas and preserves every part, input, external identity and directed relation',async()=>{
  const items=[
    {id:'front',branch:'component',children:['ui'],title:'Frontend'},
    {id:'ui',branch:'area',children:['a','b'],title:'Interface'},
    {id:'back',branch:'component',children:['worker','remote','unread'],title:'Backend'},
    {id:'a',title:'Input handling',height:180},
    {id:'inputs',branch:'inputs',children:['input'],title:'Frontend'},
    {id:'input',activation:'interaction',title:'Submit',height:90},
    {id:'b',title:'Rendering',height:90},
    {id:'worker',title:'Worker',height:90},
    {id:'remote',title:'Remote API',category:'external',height:90},
    {id:'unread',title:'Unread component',category:'component',summary:'Analysis failed',height:90},
  ];
  const areas=items.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const detail={edges:[
    {id:'implemented',relations:[{from:'input',to:'a',label:'implemented in',fromSource:'ui:4'}]},
    {id:'internal',from:'a',to:'b',relations:[{from:'a',to:'b',operations:['input']}]},
    {id:'across',from:'a',to:'worker',relations:[{from:'a',to:'worker',operations:['input'],fromSource:'code:12'}]},
    {id:'return',from:'worker',to:'b',relations:[{from:'worker',to:'b',possible:true}]},
    {id:'external',from:'worker',to:'remote',relations:[{from:'worker',to:'remote',fromSource:'code:40'}]},
  ]};
  const result=await semanticLayout(items.map(n=>({...n,width:260})),detail.edges.flatMap(e=>e.relations),areas,1300,900);
  assert.deepEqual(result.summaries.get('ui').members.map(n=>n.id),['a','b']);
  assert.equal(result.summaries.get('ui').members[0].inputs,undefined);
  assert.equal(result.layout.nodes.find(n=>n.id==='input').parentId,'inputs');
  assert.equal(result.layout.nodes.find(n=>n.id==='inputs').parentId,undefined);
  assert.equal(result.summaries.get('remote').category,'external');
  assert.equal(result.summaries.get('unread').summary,'Analysis failed');
  assert.equal(result.layout.edges.length,5,'all original relations survive beneath the summary');
  const across=result.layout.edges.find(e=>e.from==='a'&&e.to==='worker');
  assert.equal(across.relations[0].fromSource,'code:12');
  assert.deepEqual(across.relations[0].operations,['input']);
  assert.ok(result.layout.edges.some(e=>e.from==='worker'&&e.to==='b'));
  const area=result.layout.nodes.find(n=>n.id==='ui');
  assert.equal(visibleRoute(result.layout.edges.find(e=>e.from==='a'&&e.to==='b'),area,area),'');
  assert.ok(visibleRoute(across,area,null));
  assert.ok(result.layout.nodes.every(n=>Number.isFinite(n.width)&&Number.isFinite(n.height)));
});

// GET was one of 98 input tiles in no order. The collection stands its inputs
// together by the part their handler is in, framed and named by that part.
test('an input collection groups its inputs by their handler part, and keeps an unowned input loose',()=>{
  const records=[{id:'server-inputs',branch:'inputs',children:['get','set','lpush','thread','orphan']},
    {id:'get',title:'get',activation:'request'},{id:'set',title:'set',activation:'request'},{id:'lpush',title:'lpush',activation:'request'},
    {id:'thread',title:'IOThreadEntryPoint',activation:'continuous'},{id:'orphan',title:'ping',activation:'request'},
    {id:'strings',title:'String commands'},{id:'lists',title:'List commands'},{id:'vm',title:'Virtual memory'}];
  const areas=[{id:'server-inputs',nodes:['get','set','lpush','thread','orphan']}];
  const owner={get:'strings',set:'strings',lpush:'lists',thread:'vm'};
  const original=JSON.stringify({records,areas});
  const grouped=inputGroupsByPart(records,areas,owner);
  const collection=grouped.records.find(n=>n.id==='server-inputs');
  assert.deepEqual(collection.children,['server-inputs~lists','server-inputs~strings','server-inputs~vm','orphan'],'groups by part name, then the loose input');
  const strings=grouped.records.find(n=>n.id==='server-inputs~strings');
  assert.equal(strings.branch,'inputs-part');assert.equal(strings.title,'String commands');assert.equal(strings.owner,'strings');
  assert.deepEqual(strings.children,['get','set']);
  assert.deepEqual(grouped.areas.find(a=>a.id==='server-inputs~strings').nodes,['get','set']);
  assert.equal(JSON.stringify({records,areas}),original,'the saved records are not changed');
  const single=inputGroupsByPart(records,areas,{get:'strings',set:'strings'});
  assert.equal(single.records,records,'a collection with one owning part keeps its inputs loose');
});

// freqtrade's 127 options whose handler is not established stood as a wall
// of loose tiles under six group frames: each stands with the one part its
// Inputs arrow goes into, where its code takes it in.
test('an input without a handler stands in the group of the part it is taken in',()=>{
  const records=[{id:'inputs',branch:'inputs',children:['trade','--verbose','--config','orphan']},
    {id:'trade',activation:'command'},{id:'--verbose',activation:'command'},{id:'--config',activation:'command'},{id:'orphan',activation:'command'},
    {id:'cli',title:'CLI entry and commands'},{id:'engine',title:'Trading engine'},{id:'other',title:'Configuration'}];
  const relations=[{from:'trade',to:'engine',label:'implemented in'},{from:'--verbose',to:'cli',label:'looked up in'},
    {from:'--config',to:'cli',label:'declared in'},{from:'orphan',to:'cli',label:'looked up in'},{from:'orphan',to:'other',label:'looked up in'}];
  const {records:shown}=inputGroupsByPart(records,records.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children})),{trade:'engine'},relations);
  const group=owner=>shown.find(n=>n.branch==='inputs-part'&&n.owner===owner)?.children;
  assert.deepEqual(group('engine'),['trade']);
  assert.deepEqual(group('cli'),['--verbose','--config'],'taken in by one part: with it');
  assert.ok(shown.find(n=>n.id==='inputs').children.includes('orphan'),'taken in by two parts: loose, after the groups');
});
