import {visibleSegments} from './semantic.mjs';

// Several original relations can traverse one native ELK route. Paint that
// shared stretch once, without replacing the original edges used for reading,
// numbering and operation selection. Segment order follows source to target.
// `recede`: the reader's chosen emphasis (emphasis.mjs), or null. An arrow
// it does not involve recedes unless the pointer darkens it; one between
// the parts of the frame it looks at is no less involved than the dark ones
// leaving it.
export function routeDrawing(edges, closed, activeEdges, recede=null, boundary=()=>null, initVisible=false, lookedAt=null) {
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
    const visibleFrom=edge.outerFrom||from?.id||edge.from,visibleTo=edge.outerTo||to?.id||edge.to;
    const key=JSON.stringify([visibleFrom,visibleTo].sort());
    let group=groups.get(key);
    if(!group){group={edge,paths,segments,ends:[visibleFrom,visibleTo],edgeIDs:[],directions:new Set(),on:false,near:false,kept:false,possible:true,init:true,quiet:true};groups.set(key,group);}
    // Prefer an exact native route, then its stable edge ID. Never choose an
    // empty clipped route or invent a replacement line between the endpoints.
    if((group.edge.possible&&!edge.possible)||!!group.edge.possible===!!edge.possible&&edge.id<group.edge.id){
      group.edge=edge;group.paths=paths;group.segments=segments;group.ends=[visibleFrom,visibleTo];
    }
    group.edgeIDs.push(edge.id);group.directions.add(`${visibleFrom}\0${visibleTo}`);
    group.near ||= !!lookedAt&&(lookedAt.has(edge.from)||lookedAt.has(edge.to));
    group.on ||= activeEdges.has(edge.id);group.possible &&= !!edge.possible;group.init &&= !!edge.init;group.quiet &&= !!edge.quiet;
    group.kept ||= !!recede&&(recede.activeEdges.has(edge.id)||recede.focus.has(edge.from)&&recede.focus.has(edge.to));
  }
  for(const {edge,paths,segments,ends,edgeIDs,directions,on,near,kept,possible,init,quiet} of groups.values()){
    const bidirectional=directions.size>1;
    paths.forEach((path,index)=>{
      const key=path;
      let route=drawing.get(key);
      if(!route){
        route={id:index?`${edge.id}-segment-${index}`:edge.id,from:edge.from,to:edge.to,
          path,points:segments[index],start:segments[index][0],end:segments[index].at(-1),
          possible:true,init:true,quiet:true,edgeIDs:[],on:false,near:false,kept:false,arrow:false,reverseArrow:false};
        drawing.set(key,route);
      }
      for(const id of edgeIDs)if(!route.edgeIDs.includes(id))route.edgeIDs.push(id);
      route.on ||= on;
      route.near ||= near;
      route.kept ||= kept;
      route.possible &&= possible;
      route.init &&= init;
      route.quiet &&= quiet;
      route.arrow ||= index===paths.length-1;
      route.reverseArrow ||= bidirectional&&index===0;
      // The drawn boxes at its two ends: an arrowhead touches the one it
      // points into, and is the handle of that connection.
      route.boxes ||= ends;
    });
  }
  // Initialization wiring is drawn only while one of its ends is the box or
  // area the reader looks at: init is init, and it would double every
  // runtime arrow. Selecting the whole component looks at nothing in particular.
  // A quiet line (an entered program's line to its port, canvas.jsx
  // portEdges) is drawn only while pointed at, focused or chosen: at rest
  // freqtrade's had added eight dashed lines along its gutters.
  return [...drawing.values()].filter(route=>(!route.init||(initVisible&&route.on)||route.near)&&(!route.quiet||route.on)).map(route=>({...route,dim:!!recede&&!route.on&&!route.kept}));
}
