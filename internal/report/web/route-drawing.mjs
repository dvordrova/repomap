import {visibleRoute} from './semantic.mjs';

// Several original relations can traverse one native ELK route. Paint that
// shared stretch once, without replacing the original edges used for reading,
// numbering and operation selection. Segment order follows source to target.
export function routeDrawing(edges, closed, activeEdges, dim=false) {
  const drawing=new Map();
  for(const edge of edges){
    // Cross-system arrows belong to the outer map, even when one frame opens.
    // The original inner endpoints remain on the relation for source reading
    // and emphasis; they do not add a line through another level's drawing.
    const from=edge.outerSegments?null:closed(edge.from),to=edge.outerSegments?null:closed(edge.to);
    const paths=(edge.outerSegments||edge.segments).map(segment=>visibleRoute({...edge,segments:[segment]},from,to)).filter(Boolean);
    paths.forEach((path,index)=>{
      const key=JSON.stringify([path,!!edge.possible]);
      let route=drawing.get(key);
      if(!route){
        route={id:index?`${edge.id}-segment-${index}`:edge.id,from:edge.from,to:edge.to,
          path,possible:!!edge.possible,edgeIDs:[],on:false,arrow:false};
        drawing.set(key,route);
      }
      if(!route.edgeIDs.includes(edge.id))route.edgeIDs.push(edge.id);
      route.on ||= activeEdges.has(edge.id);
      route.arrow ||= index===paths.length-1;
    });
  }
  return [...drawing.values()].map(route=>({...route,dim:dim&&!route.on}));
}
