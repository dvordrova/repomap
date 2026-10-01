// The display model (PLAN B): Outside frames grouped by part (B′), inputs by
// kind, and every box's markers, on synthetic graphs and the real reports
// named by REPOMAP_SCENE_PAGES (scene-pages.mjs).
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {buildModel,markersPerSide,inputKinds} from './model.mjs';
import {syntheticPages,realPages,measure} from './scene-pages.mjs';

for(const [name,page] of [...syntheticPages,...realPages()]){
  test(`${name}: Outside frames hold shared systems first, then a bucket per part, then the rest`,()=>{
    const model=buildModel(page,{measure});
    for(const frame of [...model.nodes.values()].filter(node=>node.kind==='outside')){
      const callers=id=>model.callers.get(id)||new Set();
      const items=frame.children.map(id=>model.nodes.get(id));
      const rank=item=>item.kind==='bucket'?1:callers(item.id).size>=2&&!item.unestablished?0:2;
      assert.deepEqual(items.map(rank),[...items.map(rank)].sort((a,b)=>a-b),`${frame.name}: shared, buckets, rest`);
      const shared=items.filter(item=>rank(item)===0).map(item=>callers(item.id).size);
      assert.deepEqual(shared,[...shared].sort((a,b)=>b-a),`${frame.name}: the most shared system first`);
      for(const bucket of items.filter(item=>item.kind==='bucket')){
        assert.ok(bucket.children.length>=2,`${bucket.name}: a bucket holds two systems or more`);
        for(const id of bucket.children)assert.deepEqual([...callers(id)],[bucket.part],`${model.nodes.get(id).name} is called by ${bucket.name} alone`);
      }
      const held=items.flatMap(item=>item.kind==='bucket'?item.children:[item.id]);
      assert.equal(new Set(held).size,held.length,`${frame.name}: each system once`);
      const original=(model.record.get(frame.id).children||[]).filter(id=>model.nodes.get(id)?.kind==='system');
      assert.deepEqual([...held].sort(),[...original].sort(),`${frame.name}: every system of the frame`);
      // Every part calling two systems or more of its own has its bucket.
      const own=new Map();
      for(const item of items.filter(item=>rank(item)===2&&!item.unestablished&&callers(item.id).size===1)){const [part]=callers(item.id);own.set(part,(own.get(part)||0)+1);}
      for(const [part,count] of own)assert.ok(count<2,`${model.nodes.get(part)?.name}: ${count} systems of its own outside its bucket`);
      // The skeptic's check for casdoor (PLAN B: 94 systems, 33 items), no
      // cap elsewhere.
      if(name==='casdoor')assert.ok(items.length<=44,`${frame.name}: ${items.length} items at the top`);
    }
  });
  test(`${name}: inputs stand by kind, each in one group`,()=>{
    const model=buildModel(page,{measure});
    for(const collection of [...model.nodes.values()].filter(node=>node.kind==='inputs')){
      const kinds=collection.children.map(id=>model.nodes.get(id).inputKind);
      assert.deepEqual(kinds,inputKinds.filter(kind=>kinds.includes(kind)),'kinds in the column\'s order');
      for(const id of collection.children)for(const input of model.nodes.get(id).children)assert.equal(model.nodes.get(input).inputKind,model.nodes.get(id).inputKind);
    }
  });
  test(`${name}: a box's markers say what is inside it, at most three a side`,()=>{
    const model=buildModel(page,{measure});
    for(const node of model.nodes.values()){
      if(!['part','area','program'].includes(node.kind))continue;
      const markers=model.markersOf(node.id);
      assert.ok(markers.in.length<=markersPerSide&&markers.out.length<=markersPerSide,`${node.name}: ${markers.in.length}+${markers.out.length} markers`);
      const inside=new Set([node.id,...model.leaves(node.id)]);
      for(const marker of markers.in)for(const input of marker.members){
        const anchor=model.anchors.get(input);
        assert.ok(anchor.parts.some(part=>inside.has(part))||anchor.program===node.id,`${input} takes effect in ${node.name}`);
        // An input with no known handler is never said to be handled here.
        if(marker.handled.includes(input))assert.ok(anchor.handled,`${input} said handled in ${node.name}`);
      }
      for(const marker of markers.out)for(const system of marker.systems)
        assert.ok(marker.members.some(part=>model.calls.get(part)?.has(system)),`${system} is called from ${node.name}`);
    }
  });
}

for(const [name,page] of [...syntheticPages,...realPages()]){
  test(`${name}: inside a part, a marker stands on a declaration only for an input it handles or a call it makes`,()=>{
    const model=buildModel(page,{measure});
    for(const node of model.nodes.values()){
      if(node.kind!=='part'||!node.item?.symbols?.length)continue;
      const {members}=model.memberMarkersOf(node.id);
      for(const sides of members.values())for(const marker of sides.in)
        for(const input of marker.members)assert.ok(model.anchors.get(input)?.handled,`${input}, with no known handler, stands on a declaration of ${node.name}`);
    }
  });
}

// A small page of three programs: a client (t1) calling an outside system
// a server's (t2) input serves, a server handling its input, and a tool
// (t3) using the server's code.
const item=(id,fields={})=>({id,title:id,branch:'',activation:'',lane:'',summary:'',symbols:[],symbolCalls:[],children:[],category:'part',...fields});
const system='system-t1-out-b1-destination';
function smallPage(scene){
  return {items:[
    item('system-component-t1',{branch:'component',category:'component',children:['n-t1-g1','n-t1-g2']}),item('n-t1-g1'),item('n-t1-g2'),
    item('system-component-t2',{branch:'component',category:'component',children:['n-t2-g1','n-t2-g2']}),item('n-t2-g1'),
    item('n-t2-g2',{symbols:[{name:'serve',kind:'function',path:'a.go',line:7},{name:'other',kind:'function',path:'a.go',line:20}]}),
    item('system-component-t3',{branch:'component',category:'component',children:['n-t3-g1']}),item('n-t3-g1'),
    item('system-inputs-t2',{branch:'inputs',componentOwner:'system-component-t2',children:['t2-o1']}),
    item('t2-o1',{activation:'request',componentOwner:'system-component-t2',category:'input'}),
    item('system-outside-t1',{branch:'outside',children:[system]}),
    item(system,{branch:'communication',destinationKind:'request',children:['system-t1-out-b1']}),
    item('system-t1-out-b1',{category:'external'}),
  ],relations:[
    {from:'n-t1-g1',to:'system-t1-out-b1',scope:'operation',calls:[{caller:'#n1',callee:''}],label:''},
    {from:'system-t1-out-b1',to:'t2-o1',scope:'operation',calls:[],label:'connects to'},
    {from:'t2-o1',to:'n-t2-g1',scope:'operation',calls:[],label:'implemented in'},
    {from:'n-t3-g1',to:'n-t2-g1',scope:'structure',calls:[],label:''},
  ],areas:[],inputOwner:{'t2-o1':'n-t2-g1'},scene};
}
// The facts a report saves for it (scene.go).
const savedFacts={
  inputs:{'t2-o1':{kind:'request',program:'system-component-t2',parts:['n-t2-g1'],handled:true}},
  systems:{[system]:{kind:'request',parts:['n-t1-g1'],programs:['system-component-t1']}},
  calls:{'n-t1-g1':[{system}]},
  programPairs:[{from:'system-component-t1',to:'system-component-t2',runtime:true,relations:[1]},
    {from:'system-component-t3',to:'system-component-t2',runtime:false,relations:[3]}],
};
const pairOf=model=>(a,b)=>model.homePairs.find(p=>[p.from,p.to].sort().join('|')===[a,b].sort().join('|'));

// The whole map's arrows (owner, 2026-10-01, on the skeptic's verdict).
test('the whole map joins two programs once, at run time through an outside system, and by code use apart',()=>{
  const model=buildModel(smallPage(savedFacts),{measure}),pair=pairOf(model);
  assert.ok(pair('system-component-t1','system-component-t2')?.operation,'the client reaches the server through the system it calls');
  assert.equal(pair('system-component-t1','system-component-t2').uses,false);
  assert.ok(pair('system-component-t3','system-component-t2')?.uses,'a code use alone is drawn on demand');
  assert.ok(pair('system-inputs-t2','system-component-t2'),'a program\'s Inputs go into it');
  assert.ok(pair('system-component-t1','system-outside-t1'),'a program goes into its Outside frame');
});

// The page shows the saved facts (owner, 2026-10-01: "у html должна быть
// простая задача — вот данные, показываю"): where the page's relations
// say otherwise, the model follows the saved scene; with none saved it
// draws none, however much the relations hold.
test('buildModel draws the facts saved with the report and derives none of its own',()=>{
  const told={
    inputs:{'t2-o1':{kind:'command',program:'system-component-t2',parts:['n-t2-g2'],handled:true,handler:{path:'a.go',line:7}}},
    systems:{[system]:{kind:'database',parts:['n-t1-g2'],programs:['system-component-t1']}},
    calls:{'n-t1-g2':[{system,caller:{path:'b.go',line:3}}]},
    programPairs:[{from:'system-component-t1',to:'system-component-t2',runtime:false,relations:[1]}],
  };
  const model=buildModel(smallPage(told),{measure}),pair=pairOf(model);
  assert.deepEqual(model.anchors.get('t2-o1'),{parts:['n-t2-g2'],handled:true,handler:{path:'a.go',line:7}},'the input takes effect where the scene says');
  assert.equal(model.nodes.get('t2-o1').inputKind,'command','its kind is the saved one');
  assert.deepEqual([...model.callers.get(system)],['n-t1-g2'],'the system\'s callers are the saved ones');
  assert.equal(model.nodes.get(system).systemKind,'database','its kind is the saved one');
  assert.deepEqual([...model.calls.keys()],['n-t1-g2'],'the parts calling out are the saved ones');
  assert.deepEqual(model.markersOf('n-t2-g2').in.map(marker=>[marker.kind,marker.members]),[['command',['t2-o1']]]);
  assert.deepEqual(model.markersOf('n-t2-g1').in,[],'no marker where the relations alone put the input');
  assert.deepEqual([...model.memberMarkersOf('n-t2-g2').members.keys()],[0],'the handler\'s declaration by its saved place');
  assert.deepEqual(model.markersOf('n-t1-g1').out,[],'no marker where the relations alone put a call');
  assert.ok(pair('system-component-t1','system-component-t2')?.uses,'the programs are joined as saved, by code use');
  assert.equal(pair('system-component-t3','system-component-t2'),undefined,'no pair of programs the scene does not save');

  const bare=buildModel(smallPage(undefined),{measure});
  assert.deepEqual([...bare.anchors.values()],[{parts:[],handled:false,program:''}],'no input placed');
  assert.equal(bare.nodes.get('t2-o1').inputKind,'entry','an input of no saved kind is of a kind not established');
  assert.equal(bare.callers.size+bare.callingPrograms.size+bare.calls.size,0,'no caller, no call');
  for(const id of ['system-component-t1','system-component-t2','n-t1-g1','n-t2-g1'])assert.deepEqual(bare.markersOf(id),{in:[],out:[]},`${id}: no marker`);
  assert.deepEqual(bare.homePairs.filter(p=>bare.nodes.get(p.from).kind==='program'&&bare.nodes.get(p.to).kind==='program'),[],'no pair of programs');
});
