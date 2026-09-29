import React from 'react';
import {kindIcon,kindNames} from './kind-icons.mjs';
const t=(...args)=>globalThis.rmT?globalThis.rmT(...args):args[0];
// An input's kind as a small muted mark before its name, the kind's name on
// hover (kind-icons.mjs).
// The same mark for the page's reading column and key (30-map.js,
// 31-reading-column.js, 29-operation-view.js): a decorative element beside
// the kind's written name.
if(typeof window!=='undefined'){
  window.rmKindNames=kindNames;
  window.rmKindMark=kind=>{
    const icon=kindIcon(kind==='background'?'continuous':kind);if(!icon||typeof document==='undefined')return null;
    const ns='http://www.w3.org/2000/svg',svg=document.createElementNS(ns,'svg');
    svg.setAttribute('viewBox','0 0 16 16');svg.setAttribute('width','13');svg.setAttribute('height','13');
    svg.setAttribute('class','flow-kind-mark');svg.setAttribute('aria-hidden','true');svg.dataset.kindMark=kind;
    for(const d of icon.paths){const path=document.createElementNS(ns,'path');path.setAttribute('d',d);svg.appendChild(path);}
    return svg;
  };
}
export function KindMark({kind}){
  const icon=kindIcon(kind);if(!icon)return null;
  const title=t(kindNames[kind]);
  return <svg className="flow-kind-mark" data-kind-mark={kind} viewBox="0 0 16 16" width="14" height="14" role="img" aria-label={title}>
    <title>{title}</title>{icon.paths.map((d,i)=><path key={i} d={d}/>)}</svg>;
}
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
    {...(choose?{role:'button',tabIndex:0,onClick:event=>pick(group,event),onKeyDown:event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();pick(group,event);}}}:{})}><KindMark kind={group.kind==='background'?'continuous':group.kind}/>{group.title}</li>)}</ul>;
}
