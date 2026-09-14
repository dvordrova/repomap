// Exactly one reason controls the drawing. Reading a card is independent of
// following an input, and must never add that card's neighbours to the path.
export function emphasis(view, hoverArea, leaves, edges) {
  const mode=view.searching?'search':hoverArea?'hover':view.operation?'operation':view.scope?'selection':'all';
  const subject=mode==='operation'?view.operation:mode==='selection'?view.scope:mode==='hover'?hoverArea:'';
  const focus=new Set(mode==='search'?view.matched:mode==='operation'?[view.entry]:
    mode==='selection'?view.selected:mode==='hover'?[hoverArea,...leaves(hoverArea)]:[]);
  focus.delete('');
  const activeEdges=new Set(),participants=new Set(focus);
  if(mode!=='search'&&mode!=='all')for(const edge of edges){
    const active=mode==='operation'?edge.relations.some(r=>r.operations?.includes(view.operation)):
      focus.has(edge.from)||focus.has(edge.to);
    if(active){activeEdges.add(edge.id);participants.add(edge.from);participants.add(edge.to);}
  }
  const readingOutside=mode==='operation'&&!!view.scope&&!participants.has(view.scope)&&
    !leaves(view.scope).some(id=>participants.has(id));
  return {mode,subject,focus,participants,activeEdges,readingOutside};
}

// Ancestors explain containment only. Never feed them back into the edge
// selection: a hovered utility must not light every connection of its target.
export function focusAncestors(focus, placed) {
  const context=new Set();
  for(const id of focus)for(let at=placed.get(id)?.parentId;at;at=placed.get(at)?.parentId)context.add(at);
  return context;
}
