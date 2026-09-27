// Exactly one reason controls the drawing. Reading a card is independent of
// following an input, and must never add that card's neighbours to the path.
export function emphasis(view, hoverArea, leaves, edges, member=null) {
  const mode=view.searching?'search':hoverArea?'hover':view.operation?'operation':view.scope?'selection':'all';
  const subject=mode==='operation'?view.operation:mode==='selection'?view.scope:mode==='hover'?hoverArea:'';
  const focus=new Set(mode==='search'?view.matched:mode==='operation'?[view.entry]:
    mode==='selection'?view.selected:mode==='hover'?[hoverArea,...leaves(hoverArea)]:[]);
  focus.delete('');
  const activeEdges=new Set(),participants=new Set(focus);
  // Looking at a frame (an area or a whole component) asks how it connects
  // outward: its arrows crossing the border are dark, the ones between its
  // own parts stay ordinary. A single part's arrows are all its own.
  const frame=mode!=='operation'&&!!subject&&leaves(subject).some(id=>id!==subject);
  // A declaration pointed at or chosen in its part: only its own arrows.
  const own=member&&member.part===subject&&(mode==='hover'||mode==='selection')?edge=>carries(edge,member):()=>true;
  if(mode!=='search'&&mode!=='all')for(const edge of edges){
    const active=mode==='operation'?edge.relations.some(r=>r.operations?.includes(view.operation)):
      frame?focus.has(edge.from)!==focus.has(edge.to):(focus.has(edge.from)||focus.has(edge.to))&&own(edge);
    if(active){activeEdges.add(edge.id);participants.add(edge.from);participants.add(edge.to);}
  }
  return {mode,subject,focus,participants,activeEdges};
}

// An arrow end looked at: the parts behind it (or behind its one number
// pointed at) are the subject, outlined in place; the end's own arrows to
// them are dark and their other ends are involved; the rest recedes.
// `label` is a boundary group: {insides, edges, byNumber}.
export function endEmphasis(label,only,edges){
  const focus=new Set(only===undefined?label.insides:label.byNumber?.get(only)?.ids||[]);
  const own=new Set(label.edges),activeEdges=new Set(),participants=new Set(focus);
  for(const edge of edges)if(own.has(edge.id)&&(focus.has(edge.from)||focus.has(edge.to))){
    activeEdges.add(edge.id);participants.add(edge.from);participants.add(edge.to);
  }
  return {mode:'hover',subject:'',focus,participants,activeEdges};
}

// Ancestors explain containment only. Never feed them back into the edge
// selection: a hovered utility must not light every connection of its target.
export function focusAncestors(focus, placed) {
  const context=new Set();
  for(const id of focus)for(let at=placed.get(id)?.parentId;at;at=placed.get(at)?.parentId)context.add(at);
  return context;
}

// Whether an arrow of a part carries a call of one of its declarations: a
// call into it names its source as the callee, a call out of it names it as
// the caller. `member` is {part, names, sources} from the page data.
export function carries(edge,member){
  const sources=new Set((member.sources||[]).filter(Boolean)),names=new Set(member.names||[]);
  const into=edge.to===member.part,out=edge.from===member.part;
  return edge.relations.some(relation=>
    (into&&sources.has(relation.toSource))||(out&&sources.has(relation.fromSource))||
    (relation.calls||[]).some(call=>(into&&sources.has(call.to))||(out&&names.has(String(call.label||'').split(' ')[0]))));
}
