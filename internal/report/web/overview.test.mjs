import {test} from 'node:test';
import assert from 'node:assert/strict';
import {semanticLayout,visibleRoute} from './semantic.mjs';

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
