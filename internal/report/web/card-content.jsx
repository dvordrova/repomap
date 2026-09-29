import React from 'react';
// A trackpad pinch is a ctrl+wheel event. Let it reach the map even when
// ordinary scrolling belongs to an overflowing inventory.
export function scrollInventory(event){
  if(!event.ctrlKey&&event.currentTarget.scrollHeight>event.currentTarget.clientHeight)event.stopPropagation();
}
// A collection's kinds of inputs; a kind chosen reads the collection at
// that kind's section (owner, 2026-09-29: "Settings" had opened the
// reading at its 96 requests). Its kinds are its inputs' own, in order:
// Background work is scheduled and continuous work.
export function InputTypes({groups,lit=new Set(),choose}){
  const pick=(group,event)=>{event.stopPropagation();choose([...new Set(group.inputs.map(input=>input.activation))],event);};
  return <ul className="flow-input-types" onWheelCapture={scrollInventory}>{groups.map(group=><li key={group.kind} data-input-group-kind={group.kind}
    className={[choose?'flow-input-kind nopan':'',group.inputs.some(input=>lit.has(input.id))?'flow-lit':''].filter(Boolean).join(' ')||undefined}
    {...(choose?{role:'button',tabIndex:0,onClick:event=>pick(group,event),onKeyDown:event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();pick(group,event);}}}:{})}>{group.title}</li>)}</ul>;
}
