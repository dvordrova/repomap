import {prepareInteriors,layoutPrepared,overviewInset} from './split-layout.mjs';

// Interior geometry belongs to this mounted report. A resize places those same
// prepared frames again; camera gestures do not enter either layout stage.
export function createSemanticLayout(items,relations,areas){
  let prepared;
  return async(width,height)=>layoutPrepared(await(prepared ||= prepareInteriors(items,relations,areas,{availableHeight:height-2*overviewInset})),width,height);
}

export function semanticLayout(items,relations,areas,width,height){
  return createSemanticLayout(items,relations,areas)(width,height);
}

// Both part and call headings use 17px before their content and camera scales.
// A complete frame can reveal its already readable drawing without another
// zoom. Partly visible frames retain the ordinary entrance hysteresis.
export function detailedAreas(scales, zoom, previous=new Set(),fullyVisible=new Set()) {
  return new Set([...scales].filter(([id,scale])=>17*zoom*scale>=((previous.has(id)||fullyVisible.has(id))?12:14)).map(([id])=>id));
}

export function fullyVisibleFrames(nodes,viewport,width,height) {
  const {x,y,zoom}=viewport;
  return new Set(nodes.filter(node=>node.frame&&node.absolute.x*zoom+x>=0&&node.absolute.y*zoom+y>=0&&
    (node.absolute.x+node.width)*zoom+x<=width&&(node.absolute.y+node.height)*zoom+y<=height).map(node=>node.id));
}

// Component overview is another view of the same frame, not a smaller graph.
// Closed area headings use 20px text; direct part headings can be smaller.
// The shared presentation threshold must keep both readable.
export function componentTextSize(records) {
  return Math.min(20,...componentTextSizes(records).values());
}

export function componentTextSizes(records) {
  const byID=new Map(records.map(n=>[n.id,n]));
  return new Map(records.filter(n=>n.branch==='component').map(root=>[root.id,
    Math.min(20,...(root.children||[]).map(id=>byID.get(id)).filter(Boolean).map(n=>
      n.branch==='area'?20*(n.summaryScale||1):17*(n.contentScale||1))),
  ]));
}

// Reveal the first level when a reader approaches the participant itself.
// Its smallest descendant font must not keep the entire diagram concealed.
// The whole-map camera retains summaries; the first real approach can reveal
// a frame already occupying most of the viewport. Geometry never changes.
export function framedComponents(nodes,viewport,width,height,previous=new Set()) {
  if(viewport.zoom<=systemViewport(nodes,width,height).zoom)return new Set();
  return new Set(nodes.filter(node=>!node.parentId&&node.frame&&
    Math.max(node.width*viewport.zoom/width,node.height*viewport.zoom/height)>=(previous.has(node.id)?.65:.75)).map(node=>node.id));
}

export function componentDetails(fonts,zoom,previous=new Set(),fullyVisible=new Set(),framed=new Set()) {
  return new Set([...fonts].filter(([id,font])=>framed.has(id)||font*zoom>=((previous.has(id)||fullyVisible.has(id))?12:14)).map(([id])=>id));
}

// Replace unreadable contents before they trigger a competing root summary;
// retain the readable side of hysteresis through Back.
export function componentContents(zoom, previous=false, textSize=20) {
  return textSize*zoom >= (previous ? 12 : 14);
}

export function communicationDetails(scales, zoom, previous=new Set(),fullyVisible=new Set()) {
  return detailedAreas(scales,zoom,previous,fullyVisible);
}

function layerThreshold(frames,byID,width,height,depth,retaining=false){
  const childFonts=frames.flatMap(frame=>(byID.get(frame.id)?.children||[]).map(id=>{
    const child=byID.get(id);return child?.branch==='area'?20*(child.summaryScale||1):17*(child?.contentScale||1);
  }));
  const text=(retaining?12:14)/Math.max(0,...childFonts);
  const approach=depth===0?(retaining?.65:.75)/Math.max(...frames.map(frame=>Math.max(frame.width/width,frame.height/height))):Infinity;
  return Math.min(text,approach);
}

// Closed group labels share the same fixed reading scale as their common
// entrance. A smaller sibling must not begin with microscopic headings.
export function firstDetailZoom(nodes,records,width,height){
  return Math.max(systemViewport(nodes,width,height).zoom,
    layerThreshold(nodes.filter(node=>node.frame&&!node.parentId),new Map(records.map(record=>[record.id,record])),width,height,0));
}

// One decision per hierarchy depth. The first readable interior opens its
// whole layer; camera position and smaller siblings cannot split that layer.
export function detailLayers(nodes,records,viewport,width,height,previous=new Set()) {
  const open=new Set();
  if(viewport.zoom<=systemViewport(nodes,width,height).zoom)return open;
  const byID=new Map(records.map(record=>[record.id,record])),placed=new Map(nodes.map(node=>[node.id,node]));
  const layers=new Map();
  for(const node of nodes.filter(node=>node.frame)){
    let depth=0;for(let at=node.parentId;at;at=placed.get(at)?.parentId)depth++;
    if(!layers.has(depth))layers.set(depth,[]);
    layers.get(depth).push(node);
  }
  for(const [depth,frames] of [...layers].sort((a,b)=>a[0]-b[0])){
    const retaining=frames.some(frame=>previous.has(frame.id));
    if(viewport.zoom<layerThreshold(frames,byID,width,height,depth,retaining))break;
    for(const frame of frames)open.add(frame.id);
  }
  return open;
}

// Keep the same frame and camera; put its small entrance in the visible corner.
export function zoomMarkPosition(node,viewport,width,height,inset=12) {
  const {x,y,zoom}=viewport;
  const left=Math.max(0,node.absolute.x*zoom+x),top=Math.max(0,node.absolute.y*zoom+y);
  const right=Math.min(width,(node.absolute.x+node.width)*zoom+x),bottom=Math.min(height,(node.absolute.y+node.height)*zoom+y);
  if(right-left<28+2*inset||bottom-top<28+2*inset)return null;
  return {x:(right-28-inset-x)/zoom,y:(top+inset-y)/zoom};
}

export function systemViewport(nodes,width,height) {
  const roots=nodes.filter(n=>!n.parentId);
  if(!roots.length)return {x:24,y:24,zoom:.4};
  const left=Math.min(...roots.map(n=>n.absolute.x)),top=Math.min(...roots.map(n=>n.absolute.y));
  const right=Math.max(...roots.map(n=>n.absolute.x+n.width)),bottom=Math.max(...roots.map(n=>n.absolute.y+n.height));
  const zoom=Math.min(.44,Math.max(1,width-2*overviewInset)/(right-left),Math.max(1,height-2*overviewInset)/(bottom-top));
  return {x:Math.max(overviewInset,(width-(right-left)*zoom)/2)-left*zoom,y:Math.max(overviewInset,(height-(bottom-top)*zoom)/2)-top*zoom,zoom};
}

export function componentViewport(node,nodes,width,contentScale=1) {
  const first=nodes.filter(n=>n.parentId===node.id).sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];
  const zoom=Math.max(.85,1/contentScale);
  // Compound routing can put the first content far to the right. Focus that
  // content instead of empty frame padding; its component title stays visible.
  const left=first&&(first.absolute.x+first.width-node.absolute.x)*zoom>width-48
    ?Math.max(node.absolute.x,first.absolute.x-32):node.absolute.x;
  return {x:24-left*zoom,y:24-node.absolute.y*zoom,zoom};
}

// External frames can contain scaled call cards. Enter at their real text
// scale, with the first call visible even when routing leaves a large header gap.
export function communicationViewport(node,nodes,width,height,contentScale=1) {
  const base=componentViewport(node,nodes,width),zoom=Math.max(base.zoom,1/contentScale);
  const viewport={x:24+(base.x-24)*zoom/base.zoom,y:24+(base.y-24)*zoom/base.zoom,zoom};
  const first=nodes.filter(n=>n.parentId===node.id).sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];
  if(first){
    if(first.absolute.x*zoom+viewport.x<24||(first.absolute.x+first.width)*zoom+viewport.x>width-24)viewport.x=24-first.absolute.x*zoom;
    if(first.absolute.y*zoom+viewport.y<24||(first.absolute.y+first.height)*zoom+viewport.y>height-24)viewport.y=24-first.absolute.y*zoom;
  }
  return viewport;
}

export function closedContainer(id, placed, records, detailed, componentsOpen, communicationsOpen,openComponents) {
  let closed=null;
  for(let at=placed.get(id)?.parentId;at;at=placed.get(at)?.parentId){
    const branch=records.get(at)?.branch;
    if((branch==='area'&&!detailed.has(at))||
      (['communication','inputs'].includes(branch)&&communicationsOpen&&!communicationsOpen.has(at))||
      (branch==='component'&&(openComponents?!openComponents.has(at):!componentsOpen))||
      (!openComponents&&!componentsOpen&&['communication','inputs'].includes(branch)))closed=placed.get(at);
  }
  return closed;
}

// A hidden child still has world bounds. Keeping those bounds on screen does
// not reveal a search result or a destination reached from another reading.
export function readableFocus(id, placed, records, detailed, componentsOpen, viewport, width, height, communicationsOpen,openComponents) {
  const n=placed.get(id);
  if(!n||closedContainer(id,placed,records,detailed,componentsOpen,communicationsOpen,openComponents))return false;
  if(viewport.zoom*(records.get(id)?.contentScale||1)<.8)return false;
  const {x,y}=n.absolute,v=viewport;
  return x*v.zoom+v.x>=16&&y*v.zoom+v.y>=16&&
    (x+n.width)*v.zoom+v.x<width-16&&(y+n.height)*v.zoom+v.y<height-16;
}

export function frameInventory(id, children, records) {
  const groups=new Set(),parts=new Set();
  function visit(at){
    const item=records.get(at);
    if(item?.branch==='area')groups.add(at);
    const inside=children.get(at)||[];
    if(!inside.length)parts.add(at);
    inside.forEach(visit);
  }
  (children.get(id)||[]).forEach(visit);
  return {groups:groups.size,areaIDs:[...groups],parts:parts.size};
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
export function visibleSegments(edge, fromBox, toBox) {
  if(fromBox&&toBox&&fromBox.id===toBox.id)return [];
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
  return segments.filter(s=>s.length>1);
}

export function visibleRoute(edge,fromBox,toBox){
  return visibleSegments(edge,fromBox,toBox).map(points=>points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' ');
}
