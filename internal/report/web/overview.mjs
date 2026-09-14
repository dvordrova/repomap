import {inputGroupsHeight} from './cards.mjs';

// Summary content for the same fixed areas, without a second graph or layout.
export function overviewRecords(items, areas) {
  const byID=new Map(items.map(n=>[n.id,n]));
  const parent=new Map();areas.forEach(a=>a.nodes.forEach(id=>parent.set(id,a.id)));
  function group(id){let result=id;for(let at=id;at;at=parent.get(at))if(byID.get(at)?.branch==='area')result=at;return result;}
  const cards=new Map();
  for(const n of items.filter(n=>!n.children?.length)){
    const id=group(n.id);
    if(!cards.has(id))cards.set(id,{...byID.get(id),children:[],members:[],width:400});
    // Wrapped labels are already complete; reserve their larger overview type
    // before ELK routes around the cards, including every input row.
    cards.get(id).members.push({...n,inputs:(n.inputs||[]).map(input=>({...input,
      height:22+String(input.displayTitle||input.title).split('\n').length*22}))});
  }
  for(const card of cards.values()){
    if(card.members.length===1&&card.members[0].id===card.id){card.height=card.members[0].height;card.single=true;continue;}
    card.height=74+String(card.title).split('\n').length*28+new Set(card.members.map(n=>n.kindLabel).filter(Boolean)).size*32+card.members.reduce((h,n)=>h+
      Math.max(48,24+String(n.title).split('\n').length*24)+
      (n.inputs.length?16+inputGroupsHeight(n.inputs,36):0),0);
  }
  return {records:[...cards.values()]};
}
