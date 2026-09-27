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
// was one of 98 tiles in no order. This is display containment over the
// saved implementation owner, not another architectural group; an input
// with no owner stays loose after the groups, and a collection whose inputs
// all share one part keeps them loose.
export function inputGroupsByPart(records, areas, inputOwner={}) {
  const byID=new Map(records.map(n=>[n.id,n]));
  const added=[],replaced=new Map();
  for(const collection of records.filter(n=>n.branch==='inputs')){
    const inputs=(collection.children||[]).filter(id=>byID.get(id)?.activation);
    const groups=new Map(),loose=[];
    for(const id of inputs){
      const owner=inputOwner[id];
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
