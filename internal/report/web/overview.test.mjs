import {test} from 'node:test';
import assert from 'node:assert/strict';
import {semanticLayout,visibleRoute} from './semantic.mjs';
import {singlePartAreas} from './overview.mjs';

test('one-part areas share their existing part while participants, sources and multi-part areas remain',async()=>{
  const records=[
    {id:'app',title:'Application',branch:'component',children:['api-area','domain']},
    {id:'api-area',title:'HTTP API surface',summary:'Coordinates incoming requests.',branch:'area',children:['api']},
    {id:'api',title:'HTTP API surface',lane:'triggers',source:'app.py:12'},
    {id:'domain',title:'Simulation',branch:'area',children:['field','robot']},
    {id:'field',title:'Field'},{id:'robot',title:'Robot'},
    {id:'inputs',title:'Application',branch:'inputs',children:['request']},
    {id:'request',title:'Run',activation:'request'},
    {id:'remote',title:'Queue',branch:'communication',children:['publish']},
    {id:'publish',title:'Publish message',category:'external'},
  ];
  const areas=records.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
  const before=JSON.stringify({records,areas});
  const display=singlePartAreas(records,areas);
  assert.deepEqual([...display.aliases],[['api-area','api']]);
  assert.deepEqual(display.records.find(n=>n.id==='app').children,['api','domain']);
  assert.deepEqual(display.areas.find(n=>n.id==='app').nodes,['api','domain']);
  assert.deepEqual(display.records.find(n=>n.id==='api'),{...records.find(n=>n.id==='api'),overviewTitle:'HTTP API surface'});
  assert.equal(JSON.stringify({records,areas}),before,'saved area description and membership are untouched');
  const relations=[{from:'request',to:'api',operations:['request'],fromSource:'app.py:4'},
    {from:'api',to:'field',operations:['request'],fromSource:'app.py:16'},
    {from:'robot',to:'publish',possible:true}];
  const world=await semanticLayout(display.records,relations,display.areas,1200,800);
  const nodes=new Map(world.layout.nodes.map(n=>[n.id,n]));
  assert.equal(nodes.has('api-area'),false,'no redundant frame or additional zoom level');
  assert.equal(nodes.get('api').parentId,'app');
  assert.equal(nodes.get('field').parentId,'domain');
  assert.equal(nodes.get('publish').parentId,'remote');
  assert.equal(nodes.get('request').parentId,'inputs');
  assert.deepEqual(world.layout.edges.flatMap(e=>e.relations),relations,'all native endpoints, sources and paths survive');
});

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
