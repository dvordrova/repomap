import React from 'react';
// A trackpad pinch is a ctrl+wheel event. Let it reach the map even when
// ordinary scrolling belongs to an overflowing inventory.
export function scrollInventory(event){
  if(!event.ctrlKey&&event.currentTarget.scrollHeight>event.currentTarget.clientHeight)event.stopPropagation();
}
export function InputTypes({groups}){
  return <ul className="flow-input-types" onWheelCapture={scrollInventory}>{groups.map(group=><li key={group.kind} data-input-group-kind={group.kind}>{group.title}</li>)}</ul>;
}
