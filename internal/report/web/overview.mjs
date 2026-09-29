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

// Inside a component's input collection the inputs stand together by the
// part their handler is in, each group framed and named by that part: GET
// was one of 98 tiles in no order. An input whose handler is not
// established stands with the part its one Inputs arrow goes into, where
// its code takes it in (page_operations.go "declared in", "looked up in"):
// freqtrade's 127 options had stood as a wall of loose tiles under six
// group frames. This is display containment over saved relations, not
// another architectural group, and it adds no relation; an input with
// neither stays loose after the groups, and a collection whose inputs all
// share one part keeps them loose.
const takenInto=new Set(['declared in','looked up in']);
export function inputGroupsByPart(records, areas, inputOwner={}, relations=[]) {
  const byID=new Map(records.map(n=>[n.id,n]));
  const takenIn=new Map();
  for(const relation of relations)if(takenInto.has(relation.label)&&byID.get(relation.from)?.activation&&byID.has(relation.to)&&!byID.get(relation.to).activation){
    if(!takenIn.has(relation.from))takenIn.set(relation.from,new Set());
    takenIn.get(relation.from).add(relation.to);
  }
  const partOf=id=>inputOwner[id]||(takenIn.get(id)?.size===1?[...takenIn.get(id)][0]:'');
  const added=[],replaced=new Map();
  for(const collection of records.filter(n=>n.branch==='inputs')){
    const inputs=(collection.children||[]).filter(id=>byID.get(id)?.activation);
    const groups=new Map(),loose=[];
    for(const id of inputs){
      const owner=partOf(id);
      if(!owner||!byID.has(owner)){loose.push(id);continue;}
      if(!groups.has(owner))groups.set(owner,[]);
      groups.get(owner).push(id);
    }
    if(groups.size<2)continue;
    const frames=[...groups].map(([owner,children])=>({id:`${collection.id}~${owner}`,title:byID.get(owner).title,branch:'inputs-part',
      owner,componentOwner:collection.componentOwner,componentName:collection.componentName,children,category:'input'}))
      .sort((a,b)=>String(a.title).localeCompare(String(b.title))||a.id.localeCompare(b.id));
    added.push(...frames);
    replaced.set(collection.id,[...frames.map(frame=>frame.id),...loose]);
  }
  if(!added.length)return {records,areas};
  return {
    records:[...records.map(n=>replaced.has(n.id)?{...n,children:replaced.get(n.id)}:n),...added],
    areas:[...areas.map(a=>replaced.has(a.id)?{...a,nodes:replaced.get(a.id)}:a),...added.map(frame=>({id:frame.id,nodes:frame.children}))],
  };
}

// A program's Outside frame (page_system_map.go) holds its destinations,
// each drawn as a chip that names it: an outside system is one thing the
// program talks to, and the calls it makes there are read in the column.
// The calls' tiles are not drawn: an arrow into one goes into its chip, and
// a tile the reading names stands for its chip (`chipOf`). Litestream's
// twelve destination frames of empty call tiles had stood in one row under
// a comb of their arrows (owner, 2026-09-29). Display only: the page's
// records, relations and readings are unchanged.
export function outsideChips(records, areas, relations) {
  const byID=new Map(records.map(n=>[n.id,n])),chipOf=new Map(),chips=new Set();
  for(const frame of records.filter(n=>n.branch==='outside'))
    for(const id of frame.children||[]){
      if(!byID.has(id))continue;
      chips.add(id);
      for(const tile of byID.get(id).children||[])chipOf.set(tile,id);
    }
  if(!chips.size)return {records,areas,relations,chipOf};
  const shown=id=>chipOf.get(id)||id;
  return {
    records:records.filter(n=>!chipOf.has(n.id)).map(n=>chips.has(n.id)?{...n,branch:'chip',children:[],category:'external'}:n),
    areas:areas.filter(a=>!chips.has(a.id)),
    relations:relations.map(r=>chipOf.has(r.from)||chipOf.has(r.to)?{...r,from:shown(r.from),to:shown(r.to)}:r),
    chipOf,
  };
}
