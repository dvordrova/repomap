import React from 'react';
// A trackpad pinch is a ctrl+wheel event. Let it reach the map even when
// ordinary scrolling belongs to an overflowing inventory.
export function scrollInventory(event){
  if(!event.ctrlKey&&event.currentTarget.scrollHeight>event.currentTarget.clientHeight)event.stopPropagation();
}
export function InputTypes({groups}){
  return <ul className="flow-input-types" onWheelCapture={scrollInventory}>{groups.map(group=><li key={group.kind} data-input-group-kind={group.kind}>{group.title}</li>)}</ul>;
}

export function OverviewMembers({members,reading,operation,open}){
  const kinds=new Map();
  for(const member of members){const kind=member.kindLabel||'';if(!kinds.has(kind))kinds.set(kind,[]);kinds.get(kind).push(member);}
  return <div className="flow-overview-members">{[...kinds].map(([kind,items])=><section key={kind} className="flow-member-kind-group">
    {kind&&<div className={`flow-member-kind ${items[0].lane==='core'?'flow-member-kind-core':''}`}>{kind}</div>}
    {items.map(n=><div key={n.id} className={`flow-overview-member flow-member-${n.category||'part'} ${n.lane==='core'?'flow-member-core':''}`}>
    <button type="button" data-overview-member={n.id} className={`flow-member-title nopan ${reading===n.id?'flow-overview-reading':''}`}
      onClick={event=>{event.stopPropagation();open(n.id,event);}}>{n.title}</button>
  </div>)}</section>)}</div>;
}
