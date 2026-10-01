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
      const callers=id=>model.callers?.get?.(id)||new Set(model.edges.filter(edge=>edge.to===id&&model.nodes.get(edge.from)?.kind==='part').map(edge=>edge.from));
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
      // About forty at most (PLAN B: casdoor 94 systems, 33 items).
      assert.ok(items.length<=44,`${frame.name}: ${items.length} items at the top`);
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
