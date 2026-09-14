import {arrange} from './layout.mjs';
import {overviewRecords} from './overview.mjs';

// One world for every zoom. Area summaries and their contents occupy the same
// fixed rectangle; switching detail never replaces its geographic layout.
export async function semanticLayout(items, relations, areas, width, height) {
  const natural=await arrange(items,relations,areas,width,height);
  const summaries=overviewRecords(items,areas);
  const byID=new Map(items.map(n=>[n.id,n])), naturalByID=new Map(natural.nodes.map(n=>[n.id,n]));
  const parent=new Map();areas.forEach(a=>a.nodes.forEach(id=>parent.set(id,a.id)));
  const summaryByID=new Map(summaries.records.map(n=>[n.id,n]));
  const scales=new Map(areas.filter(a=>byID.get(a.id)?.branch==='area').map(a=>[a.id,Math.min(1,400/naturalByID.get(a.id).width)]));
  const owner=id=>{for(let at=parent.get(id);at;at=parent.get(at))if(scales.has(at))return at;return '';};
  const records=items.map(n=>{
    if(n.branch==='communication')return {...n,minimumWidth:1000,minimumHeight:500};
    if(scales.has(n.id))return {...n,contentScale:scales.get(n.id),headerHeight:64,
      minimumWidth:400,minimumHeight:summaryByID.get(n.id)?.height||0};
    const scale=scales.get(owner(n.id))||1;
    return {...n,contentScale:scale,originalWidth:n.width,originalHeight:n.height,
      width:n.width*scale,height:n.height*scale};
  });
  let layout=await arrange(records,relations,areas,width,height);
  // Summary heights are only known after the scaled interiors are placed.
  // Reserve root-header width using that complete world, before displaying it;
  // the natural unscaled layout can substantially overestimate its fit zoom.
  const overviewZoom=systemViewport(layout.nodes,width,height||700).zoom;
  let widened=false;
  for(const node of layout.nodes.filter(n=>!n.parentId)){
    const record=records.find(n=>n.id===node.id);
    const minimum=record.branch==='component'?220:record.branch==='communication'?160:0;
    if(node.width*overviewZoom<minimum){record.minimumWidth=minimum/overviewZoom;widened=true;}
  }
  if(widened)layout=await arrange(records,relations,areas,width,height);
  // Compound routing may leave a short root even though its nested drawing is
  // large. Reserve its measured overview heading and complete area list before
  // showing the fixed world. Recheck after fitting because added space changes
  // the whole-map scale. Bounded placement work never runs during a gesture.
  for(let pass=0;pass<4;pass++){
    const zoom=systemViewport(layout.nodes,width,height||700).zoom;
    let resized=false;
    for(const node of layout.nodes.filter(n=>!n.parentId)){
      const record=records.find(n=>n.id===node.id);
      const needed=record.overviewHeightAtWidth?.(node.width*zoom)||0;
      if(needed&&record.overviewMinWidth>node.width*zoom){record.minimumWidth=(record.overviewMinWidth+2)/zoom;resized=true;}
      if(needed>node.height*zoom+1){record.minimumHeight=(needed+2)/zoom;resized=true;}
    }
    if(!resized)break;
    layout=await arrange(records,relations,areas,width,height);
  }
  return {layout,records,scales,owner,summaries:summaryByID};
}

// Hysteresis avoids flicker around a detail boundary. Scale refers to actual
// on-screen text size, not the number of items in the repository.
export function detailedAreas(scales, zoom, previous=new Set()) {
  return new Set([...scales].filter(([id,scale])=>zoom*scale>=(previous.has(id)?.52:.68)).map(([id])=>id));
}

// Component overview is another view of the same frame, not a smaller graph.
// Keep the threshold separate from part detail and retain it through Back.
export function componentContents(zoom, previous=false) {
  return zoom >= (previous ? .46 : .58);
}

export function systemViewport(nodes,width,height) {
  const roots=nodes.filter(n=>!n.parentId);
  if(!roots.length)return {x:24,y:24,zoom:.4};
  const left=Math.min(...roots.map(n=>n.absolute.x)),top=Math.min(...roots.map(n=>n.absolute.y));
  const right=Math.max(...roots.map(n=>n.absolute.x+n.width)),bottom=Math.max(...roots.map(n=>n.absolute.y+n.height));
  const zoom=Math.min(.44,Math.max(1,width-48)/(right-left),Math.max(1,height-48)/(bottom-top));
  return {x:Math.max(24,(width-(right-left)*zoom)/2)-left*zoom,y:Math.max(24,(height-(bottom-top)*zoom)/2)-top*zoom,zoom};
}

export function componentViewport(node,nodes,width) {
  const first=nodes.filter(n=>n.parentId===node.id).sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];
  const zoom=Math.max(.85,Math.min(1,(width-80)/(first?.width||node.width)));
  // Compound routing can put the first content far to the right. Focus that
  // content instead of empty frame padding; its component title stays visible.
  const left=first&&(first.absolute.x+first.width-node.absolute.x)*zoom>width-48
    ?Math.max(node.absolute.x,first.absolute.x-32):node.absolute.x;
  return {x:24-left*zoom,y:24-node.absolute.y*zoom,zoom};
}

export function closedContainer(id, placed, records, detailed, componentsOpen) {
  let closed=null;
  for(let at=placed.get(id)?.parentId;at;at=placed.get(at)?.parentId){
    const branch=records.get(at)?.branch;
    if((branch==='area'&&!detailed.has(at))||
      (!componentsOpen&&(branch==='component'||branch==='communication')))closed=placed.get(at);
  }
  return closed;
}

// A hidden child still has world bounds. Keeping those bounds on screen does
// not reveal a search result or a destination reached from another reading.
export function readableFocus(id, placed, records, detailed, componentsOpen, viewport, width, height) {
  const n=placed.get(id);
  if(!n||closedContainer(id,placed,records,detailed,componentsOpen))return false;
  if(viewport.zoom*(records.get(id)?.contentScale||1)<.8)return false;
  const {x,y}=n.absolute,v=viewport;
  return x*v.zoom+v.x>=16&&y*v.zoom+v.y>=16&&
    (x+n.width)*v.zoom+v.x<width-16&&(y+n.height)*v.zoom+v.y<height-16;
}

export function frameInventory(id, children, records) {
  const groups=new Set(),parts=new Set(),inputs=new Set();
  function visit(at){
    const item=records.get(at);
    if(item?.branch==='area')groups.add(at);
    const inside=children.get(at)||[];
    if(!inside.length)parts.add(at);
    for(const input of item?.inputs||[])inputs.add(input.id);
    inside.forEach(visit);
  }
  (children.get(id)||[]).forEach(visit);
  return {groups:groups.size,areaIDs:[...groups],parts:parts.size,inputs:inputs.size};
}

// Fit readable content when it fits; otherwise begin at an actual area, never
// at the empty corner of a component frame stretched by long connections.
export function overviewViewport(nodes, areaIDs, width, height) {
  const byID=new Map(nodes.map(n=>[n.id,n]));
  const insideArea=n=>{for(let parent=n.parentId;parent;parent=byID.get(parent)?.parentId)if(areaIDs.has(parent))return true;return false;};
  const cards=nodes.filter(n=>areaIDs.has(n.id)||(!n.frame&&!insideArea(n)));
  if(!cards.length)return {x:16,y:16,zoom:.6};
  const zoom=.6,margin=24;
  const left=Math.min(...cards.map(n=>n.absolute.x)),top=Math.min(...cards.map(n=>n.absolute.y));
  const right=Math.max(...cards.map(n=>n.absolute.x+n.width)),bottom=Math.max(...cards.map(n=>n.absolute.y+n.height));
  if((right-left)*zoom<=width-2*margin&&(bottom-top)*zoom<=height-2*margin)
    return {x:(width-(right-left)*zoom)/2-left*zoom,y:(height-(bottom-top)*zoom)/2-top*zoom,zoom};
  const first=cards.slice().sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];
  return {x:margin-first.absolute.x*zoom,y:margin-first.absolute.y*zoom,zoom};
}

function contains(box,p){return p.x>=box.absolute.x&&p.x<=box.absolute.x+box.width&&p.y>=box.absolute.y&&p.y<=box.absolute.y+box.height;}
function crossing(box,a,b){
  // ELK's fractional hierarchy offsets can differ in the last decimal even on
  // one vertical segment. Choose its dominant axis, not exact float equality.
  if(Math.abs(b.x-a.x)<Math.abs(b.y-a.y)){const y=b.y>a.y?box.absolute.y+box.height:box.absolute.y;return {x:a.x,y};}
  const x=b.x>a.x?box.absolute.x+box.width:box.absolute.x;return {x,y:a.y};
}

// Trim the existing orthogonal route at a closed area's boundary. We do not
// route a replacement arrow or infer a new connection between its parts.
export function visibleRoute(edge, fromBox, toBox) {
  if(fromBox&&toBox&&fromBox.id===toBox.id)return '';
  const segments=edge.segments.map(points=>{
    let route=points.map(p=>({...p}));
    if(fromBox){
      const outside=route.findIndex(p=>!contains(fromBox,p));
      if(outside<0)return [];
      if(outside>0)route=[crossing(fromBox,route[outside-1],route[outside]),...route.slice(outside)];
    }
    if(toBox){
      const reversed=route.slice().reverse(),outside=reversed.findIndex(p=>!contains(toBox,p));
      if(outside<0)return [];
      if(outside>0)route=[crossing(toBox,reversed[outside-1],reversed[outside]),...reversed.slice(outside)].reverse();
    }
    return route;
  });
  return segments.filter(s=>s.length>1).map(points=>points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' ');
}
