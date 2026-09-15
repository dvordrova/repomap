// An area around one existing part adds no visual level. Keep its saved
// identity in the host reading; only the drawing uses the part's rectangle.
// Participant/input/destination frames still express a distinct boundary.
export function singlePartAreas(records, areas) {
  const byID=new Map(records.map(n=>[n.id,n]));
  const children=new Map(areas.map(a=>[a.id,a.nodes]));
  const aliases=new Map();
  for(const area of records.filter(n=>n.branch==='area')){
    const ids=children.get(area.id)||[],part=byID.get(ids[0]);
    if(ids.length===1&&part&&!part.branch&&!part.activation&&!children.get(part.id)?.length)
      aliases.set(area.id,part.id);
  }
  const displayed=id=>aliases.get(id)||id;
  const captions=new Map([...aliases].map(([area,part])=>[part,byID.get(area).title]));
  return {
    records:records.filter(n=>!aliases.has(n.id)).map(n=>n.children?{...n,children:n.children.map(displayed)}
      :captions.has(n.id)?{...n,overviewTitle:captions.get(n.id)}:n),
    areas:areas.filter(a=>!aliases.has(a.id)).map(a=>({...a,nodes:a.nodes.map(displayed)})),
    aliases,
  };
}

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
    // before ELK routes around the cards. Inputs retain their original cards.
    cards.get(id).members.push(n);
  }
  for(const card of cards.values()){
    if(card.members.length===1&&card.members[0].id===card.id){card.height=card.members[0].height;card.single=true;continue;}
    card.height=74+String(card.title).split('\n').length*28+new Set(card.members.map(n=>n.kindLabel).filter(Boolean)).size*32+card.members.reduce((h,n)=>h+
      Math.max(48,24+String(n.title).split('\n').length*24),0);
  }
  return {records:[...cards.values()]};
}
