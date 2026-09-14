// Group the original relations by outside identity and direction. Numbering
// identifies inner parts, not an inferred execution order.
export function connections(area, members, edges, outsideOf=id=>id) {
  const own = new Set(members), numbers = new Map(members.map((id,i)=>[id,i+1])), groups = new Map();
  for (const edge of edges) {
    const from = own.has(edge.from), to = own.has(edge.to);
    if(from === to) continue;
    const outside = outsideOf(from ? edge.to : edge.from), key = `${from?'out':'in'}:${outside}`;
    if(!groups.has(key))groups.set(key,{key,area,outside,incoming:!from,insides:new Set(),relations:[],edges:[]});
    const group = groups.get(key);
    group.insides.add(from?edge.from:edge.to);group.relations.push(...edge.relations);group.edges.push(edge.id);
  }
  return [...groups.values()].map(g=>({...g,insides:[...g.insides],numbers:[...g.insides].map(id=>numbers.get(id)).sort((a,b)=>a-b)}));
}
