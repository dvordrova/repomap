import React from 'react';
import {groupInputs} from './cards.mjs';

// Both zoom levels use the same input cards. A count is never a substitute for
// the actual request, command or activity that the reader came here to find.
export function InputCards({inputs=[],operation,select}){
  if(!inputs.length)return null;
  return <div className="flow-inputs">{groupInputs(inputs).map(group=><section key={group.kind} className="flow-input-group" data-input-kind={group.kind}>
    <div className="flow-input-kind">{group.title}</div>
    {group.inputs.map(input=><button key={input.id} type="button" data-input-id={input.id}
    aria-pressed={operation===input.id} className="flow-input-card nopan" style={{height:input.height}}
    onClick={event=>{event.stopPropagation();select(input.id,event);}}>
    <strong>{input.displayTitle||input.title}</strong>
  </button>)}</section>)}</div>;
}

export function OverviewMembers({members,reading,operation,open}){
  const kinds=new Map();
  for(const member of members){const kind=member.kindLabel||'';if(!kinds.has(kind))kinds.set(kind,[]);kinds.get(kind).push(member);}
  return <div className="flow-overview-members">{[...kinds].map(([kind,items])=><section key={kind} className="flow-member-kind-group">
    {kind&&<div className={`flow-member-kind ${items[0].lane==='core'?'flow-member-kind-core':''}`}>{kind}</div>}
    {items.map(n=><div key={n.id} className={`flow-overview-member flow-member-${n.category||'part'} ${n.lane==='core'?'flow-member-core':''}`}>
    <button type="button" data-overview-member={n.id} className={`flow-member-title nopan ${reading===n.id?'flow-overview-reading':''}`}
      onClick={event=>{event.stopPropagation();open(n.id,event);}}>{n.title}</button>
    <InputCards inputs={n.inputs} operation={operation} select={open}/>
  </div>)}</section>)}</div>;
}
