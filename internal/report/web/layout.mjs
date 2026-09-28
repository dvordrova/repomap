// Group the original relations by outside identity and direction. Numbering
// identifies inner parts, not an inferred execution order.
export function connections(area, members, edges, outsideOf=id=>id) {
  const own = new Set(members), groups = new Map();
  for (const edge of edges) {
    const from = own.has(edge.from), to = own.has(edge.to);
    if(from === to) continue;
    const outside = outsideOf(from ? edge.to : edge.from), key = `${from?'out':'in'}:${outside}`;
    if(!groups.has(key))groups.set(key,{key,area,outside,incoming:!from,insides:new Set(),relations:[],edges:[]});
    const group = groups.get(key);
    group.insides.add(from?edge.from:edge.to);group.relations.push(...edge.relations);group.edges.push(edge.id);
  }
  return [...groups.values()].map(g=>({...g,insides:[...g.insides]}));
}

// One plaque per arrow end (owner's 2a, 2026-09-28). Both directions
// between the frame and one outside frame, a two-headed arrow or two
// opposite arrows, end at one plaque on that side, the incoming one's
// (the other is `hidden` and its `twin`): two plaques side by side read as
// two things. It stands for the parts behind both. It says "all" only when
// they are every part of the frame (`parts`, `partOf` a member's part), else
// it is a plain handle: the digits named members in member order and
// changed with the pointer. `labels` are placed connections {outside,
// incoming, insides, edges, root, side, point}; they are completed in place.
export function endPlaques(labels, parts, partOf=id=>id) {
  for(const label of labels){
    const twin=labels.find(other=>other!==label&&other.outside===label.outside&&(other.root===label.root&&other.side===label.side||Math.hypot(other.point.x-label.point.x,other.point.y-label.point.y)<1));
    if(twin){label.hidden=!label.incoming;label.twin=twin.id;label.insides=[...new Set([...label.insides,...twin.insides])];label.edges=[...new Set([...label.edges,...twin.edges])];}
  }
  for(const label of labels){const joined=new Set(label.insides.map(partOf));label.all=parts.size>1&&[...parts].every(id=>joined.has(id));}
  return labels;
}
