import {visibleSegments} from './semantic.mjs';

// Several original relations can traverse one native ELK route. Paint that
// shared stretch once, without replacing the original edges used for reading,
// numbering and operation selection. Segment order follows source to target.
export function routeDrawing(edges, closed, activeEdges, dim=false, boundary=()=>null) {
  const drawing=new Map(),groups=new Map();
  for(const edge of edges){
    // Cross-system arrows belong to the outer map, even when one frame opens.
    // The original inner endpoints remain on the relation for source reading
    // and emphasis; they do not add a line through another level's drawing.
    const from=edge.outerSegments?null:closed(edge.from)||boundary(edge.from,edge.to),to=edge.outerSegments?null:closed(edge.to)||boundary(edge.to,edge.from);
    const segments=visibleSegments({...edge,segments:edge.outerSegments||edge.segments},from,to);
    const paths=segments.map(points=>points.map((point,index)=>`${index?'L':'M'} ${point.x} ${point.y}`).join(' '));
    if(!paths.length)continue;
    // Certainty belongs to the original relations, not to a second visible
    // arrow. A mixed bundle proves a connection exists; its possible reads or
    // calls still retain their own status in the source inspection.
    const key=JSON.stringify([edge.outerFrom||from?.id||edge.from,edge.outerTo||to?.id||edge.to]);
    let group=groups.get(key);
    if(!group){group={edge,paths,segments,edgeIDs:[],on:false,possible:true};groups.set(key,group);}
    // Prefer an exact native route, then its stable edge ID. Never choose an
    // empty clipped route or invent a replacement line between the endpoints.
    if((group.edge.possible&&!edge.possible)||!!group.edge.possible===!!edge.possible&&edge.id<group.edge.id){
      group.edge=edge;group.paths=paths;group.segments=segments;
    }
    group.edgeIDs.push(edge.id);group.on ||= activeEdges.has(edge.id);group.possible &&= !!edge.possible;
  }
  for(const {edge,paths,segments,edgeIDs,on,possible} of groups.values()){
    paths.forEach((path,index)=>{
      const key=path;
      let route=drawing.get(key);
      if(!route){
        route={id:index?`${edge.id}-segment-${index}`:edge.id,from:edge.from,to:edge.to,
          path,points:segments[index],start:segments[index][0],end:segments[index].at(-1),
          possible:true,edgeIDs:[],on:false,arrow:false};
        drawing.set(key,route);
      }
      for(const id of edgeIDs)if(!route.edgeIDs.includes(id))route.edgeIDs.push(id);
      route.on ||= on;
      route.possible &&= possible;
      route.arrow ||= index===paths.length-1;
    });
  }
  return [...drawing.values()].map(route=>({...route,dim:dim&&!route.on}));
}
