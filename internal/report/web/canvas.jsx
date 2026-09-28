import React, {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow, Handle, Position, ViewportPortal, useViewport, useStore} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import {connections, endPlaques, borderCrossing, plaqueCentre, stubEnds} from './layout.mjs';
import {tileGrid,tileRoom,tileHeader} from './symbols.mjs';
import {createLook} from './look.mjs';
import {emphasis, focusAncestors, endEmphasis, recedes} from './emphasis.mjs';
import {createSemanticLayout, detailLayers, firstDetailZoom, componentTextSizes, frameViewport, partViewport, pathViewport, tileViewport, deepViewport, pointViewport, staysOpen, layerFloor, closedContainer, readableFocus, frameInventory, systemViewport, detailLevel, pinchZoom, zoomBelow} from './semantic.mjs';
import {routeDrawing} from './route-drawing.mjs';
import {inputGroupsByPart} from './overview.mjs';
import {prepareCards,wrapText,overviewHeading,overviewScale,groupHeading,cardText} from './cards.mjs';
import {overviewInset} from './split-layout.mjs';
import {HoverGate} from './hover.mjs';
import {placeCard} from './card-place.mjs';
import {InputTypes, scrollInventory} from './card-content.jsx';
import {callCard} from './call-card.mjs';
import {CallRows, cardCount, FrameConnections} from './call-card-view.jsx';
import '@xyflow/react/dist/style.css';
import './canvas.css';

// Legacy small repository diagrams share the same embedded ELK instance code.
window.ELK = ELK;
const t = (...args) => window.rmT(...args);
// Zoomed far into a part, its declarations stand inside it as tiles, by
// file and in the order the page lists them (tileGrid): a tile is a
// declaration to choose, as wide as the longest name among them. The tiles
// are drawn in the card's own rectangle at the scale that fits them all, so
// nothing on the map moves when they appear.
// The part's drawing box and the scale it is drawn at, as Part draws it.
function partBox(data){
  const heading=data.standaloneHeading,scale=heading?.scale||data.contentScale||1;
  const box=heading?{width:heading.width,height:heading.height}:data.fill||{width:data.originalWidth||data.width||260,height:data.originalHeight||data.height||88};
  return {box,scale};
}
const tileContext=document.createElement('canvas').getContext('2d');
const measureTile=(text,font)=>{tileContext.font=font;return tileContext.measureText(text).width;};
const grids=new Map();
function partGrid(data,box){
  const key=`${data.id}|${box.width}|${box.height}`;
  if(!grids.has(key))grids.set(key,tileGrid(data.symbols||[],data.symbolCalls||[],box,measureTile));
  return grids.get(key);
}
// A marker is drawn in its line's stroke widths, seven of them. Emphasis
// draws a line 2.5px thick instead of 1.5px (canvas.css), so its marker is
// 1.5/2.5 of that and every head stands 10.5px. At seven emphasised strokes
// a head stood 17.5px, nearly three times the area, and covered the plaque
// at the frame.
const arrowHead=7,emphasisedHead=arrowHead*1.5/2.5;
// An arrow end's plaque on screen: the former one-digit chip (canvas.css).
const plaqueSize={width:23,height:20};
function PartSymbols({symbols,calls,width,height,grid,member}){
  const {divisor,inner,tileWidth,blocks,rows,hidden}=grid,{inset,columnGap:gap}=tileRoom;
  const links=calls||[],hot=member?.hot??-1,chosen=member?.chosen??-1;
  const x=column=>inset+column*(tileWidth+gap);
  // A method inside its type's tile needs no line to that type.
  const drawn=links.filter(([from,to])=>rows[from]&&rows[to]&&rows[from].block!==rows[to].block);
  // A declaration with those its links join. Pointing at one darkens its
  // links and recedes nothing; a chosen one recedes what its links do not
  // join, save what the pointer brings forward.
  const joined=at=>new Set(at<0?[]:[at,...drawn.filter(([from,to])=>from===at||to===at).flat().filter(value=>typeof value==='number')]);
  const lit=joined(hot),kept=joined(chosen);
  const tone=i=>chosen>=0&&!kept.has(i)&&!lit.has(i)?'flow-symbol-dim':'';
  // A row reads as a line of a class box: the visibility sign, the name and,
  // in lighter type, what follows it — "(args): Result" or ": Type".
  const mixed=new Set(blocks.filter(block=>{const all=[block.head,...block.rows].filter(i=>symbols[i].kind!=='more');return all.some(i=>symbols[i].inner)&&all.some(i=>!symbols[i].inner);}).flatMap(block=>[block.head,...block.rows]));
  const row=(i,className,first)=>{
    const symbol=symbols[i],kind=symbol.kind==='field'||symbol.kind==='more'?'flow-symbol-field':'';
    // A click chooses the declaration: the reading names it and the camera
    // centres it. Opened with a modifier, a tile's link still opens the code.
    const choose=event=>{
      if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey){event.stopPropagation();return;}
      event.preventDefault();event.stopPropagation();member?.choose(i,event);
    };
    const props={className:`${className} ${kind} ${first?'flow-symbol-first-method':''} ${symbol.key?'flow-symbol-key':''} ${symbol.inner&&mixed.has(i)?'flow-symbol-inner':''} ${i===chosen?'flow-symbol-chosen':''} ${tone(i)}`,
      'data-symbol':i,onMouseEnter:()=>member?.point(i),onClick:symbol.kind==='more'?undefined:choose};
    const body=<>{symbol.name}{symbol.text&&<em>{symbol.text}</em>}</>;
    return symbol.href&&symbol.kind!=='more'?<a key={i} href={symbol.href} target="_blank" rel="noopener" {...props}>{body}</a>
      :<span key={i} {...props}>{body}</span>;
  };
  // A drag anywhere over the declarations pans, as it does over the part: at
  // their reading scale they fill the canvas, and a drag that moved is no
  // click on the tile it started on.
  const header=tileHeader(divisor);
  return <div className="flow-part-symbols" style={{top:header,width,height:height-header}}>
    <div style={{width:inner.width,height:inner.height,transform:`scale(${1/divisor})`,transformOrigin:'top left'}} onMouseLeave={()=>member?.point(-1)}>
      <svg width={inner.width} height={inner.height}>
        <defs>{[['flow-symbol-arrow',arrowHead],['flow-symbol-arrow-hot',emphasisedHead]].map(([id,size])=>
          <marker key={id} id={id} viewBox="0 0 8 8" refX="7" refY="4" markerWidth={size} markerHeight={size} orient="auto"><path d="M0 0L8 4L0 8z"/></marker>)}</defs>
        {drawn.map(([from,to,kind],i)=>{
          const a=rows[from],b=rows[to],forward=b.column>a.column,ay=inset+a.y+a.height/2,by=inset+b.y+b.height/2;
          // Across columns a link leaves the right edge and enters the left;
          // within a column it bows out to the right of both rows.
          const start={x:x(a.column)+tileWidth,y:ay},end=forward?{x:x(b.column),y:by}:{x:x(b.column)+tileWidth,y:by};
          const bend=forward?Math.max(16,(end.x-start.x)/2):26;
          const on=hot>=0&&(from===hot||to===hot),receded=chosen>=0&&!on&&from!==chosen&&to!==chosen;
          return <path key={i} className={`flow-symbol-${kind||'calls'} ${on?'flow-symbol-call-hot':receded?'flow-symbol-call-dim':''}`}
            d={`M${start.x} ${start.y}C${start.x+bend} ${start.y},${forward?end.x-bend:end.x+bend} ${end.y},${end.x} ${end.y}`} markerEnd={`url(#flow-symbol-arrow${on?'-hot':''})`}>
            <title>{`${symbols[from].name} ${kind||'calls'} ${symbols[to].name}`}</title></path>;
        })}
      </svg>
      {blocks.map(block=><div key={block.head} className={`flow-symbol-block ${block.rows.length?'flow-symbol-type':''}`}
        style={{left:x(block.column),top:inset+block.y,width:tileWidth,height:block.height}}>
        {row(block.head,'flow-symbol-head')}
        {block.rows.map((i,k)=>row(i,'flow-symbol-row',k>0&&symbols[i].kind!=='field'&&symbols[i].kind!=='more'&&['field','more'].includes(symbols[block.rows[k-1]].kind)))}
        {block.more>0&&<span className="flow-symbol-row flow-symbol-rest">… +{block.more}</span>}
      </div>)}
      {hidden>0&&<span className="flow-symbol-more" style={{right:inset,bottom:inset}}>+{hidden}</span>}
    </div>
  </div>;
}
function Part({data}) {
  const heading=data.standaloneHeading,{box,scale}=partBox(data);
  // Only the flip between the two drawings re-renders the card, not every
  // step of a zoom.
  const far=useStore(state=>box.width*scale*state.transform[2]>=860);
  const deep=far&&!data.activation&&data.symbols?.length>0,grid=deep?partGrid(data,box):null;
  // A loose part's heading is fitted to its box; its description takes the
  // whole lines left under the title and the zoom mark's row.
  const standaloneLines=heading?Math.floor((heading.height-12-heading.title.split('\n').length*16-16)/15):0;
  const standaloneText=standaloneLines>0;
  if(deep)return <div className={`flow-part flow-part-deep flow-${data.category} ${data.lane==='core'?'flow-core':data.lane==='triggers'?'flow-entry':''}`}
      style={{width:box.width,height:box.height,transform:`scale(${scale})`,transformOrigin:'top left'}}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <strong style={{fontSize:28/grid.divisor,lineHeight:`${40/grid.divisor}px`,padding:`${20/grid.divisor}px ${32/grid.divisor}px 0`}}>{data.name||data.title}</strong>
    <PartSymbols symbols={data.symbols} calls={data.symbolCalls} width={box.width} height={box.height} grid={grid} member={data.member}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
  return <div className={`flow-part flow-${data.category} ${data.category==='input'?'':data.lane==='core'?'flow-core':data.lane==='triggers'?'flow-entry':''} ${heading?'flow-standalone-part':''}`} data-input-id={data.activation?data.id:undefined} style={heading?{width:heading.width,height:heading.height,transform:`scale(${scale})`,transformOrigin:'top left'}:data.fill?{width:data.fill.width,height:data.fill.height,transform:`scale(${scale||1})`,transformOrigin:'top left'}:scale&&scale!==1?{width:data.originalWidth,height:data.originalHeight,transform:`scale(${scale})`,transformOrigin:'top left'}:undefined}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    {data.roleLabel&&<span className={`flow-role-symbol flow-role-${data.lane}`} role="img" aria-label={data.roleLabel}/> }
    {data.kindLabel&&!heading&&<div className="flow-kind" data-input-kind={data.activation||undefined}>{data.kindLabel}</div>}
    <strong data-input-name={data.activation?'':undefined}>{heading?.title||data.title}</strong>
    {/* The description wraps in the column its lines were counted in: a
        browser that draws the 1.5px border 1px wide leaves a 226px column,
        where pykrx's 225.39px "Fetches Korean market fundamentals" fit whole
        and the card kept an empty line. */}
    {data.description&&(!heading||standaloneText)&&<div className="flow-description" style={heading?{WebkitLineClamp:standaloneLines,maxHeight:standaloneLines*15}:{WebkitLineClamp:data.descriptionMost||undefined,maxWidth:cardText}}>{data.description}</div>}
    {data.subtitle&&<div className="flow-address">{data.subtitle}</div>}
    {data.symbols?.length>0&&!data.activation&&<button type="button" className="flow-part-zoom nopan" aria-label={t('Zoom into {0}',data.name||data.title)}
      onClick={event=>{event.stopPropagation();data.zoomInto?.();}}>
      <span className="flow-zoom-picture" aria-hidden="true"/></button>}
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function Area({data}) {
  return <div className={`flow-area ${data.branch==='component'?'flow-component':['communication','communication-group'].includes(data.branch)?'flow-communication':['inputs','inputs-part'].includes(data.branch)?'flow-input-collection':data.branch!=='area'?'':data.lane==='core'?'flow-area-core':data.lane==='triggers'?'flow-area-entry':''}`}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function AreaSummary({node,item,heading,enter,select,muted}){
  const {scale,title}=heading;
  // A closed area says what it is: its one-line description takes the whole
  // lines left under its title.
  const lines=Math.floor((node.height/scale-12-title.split('\n').length*16-32)/15);
  return <div className={`flow-area-summary nopan ${muted?'flow-node-muted':''}`} data-summary-area={node.id}
    style={{transform:`translate(${node.absolute.x}px,${node.absolute.y}px) scale(${scale})`,transformOrigin:'top left',
      width:node.width/scale,height:node.height/scale}}
    onMouseEnter={()=>enter(node.id)} onClick={event=>{event.stopPropagation();select(node.id,event,false);}}>
    <div className="flow-part flow-overview-card flow-overview-compact"><strong>{title}</strong>
      {item?.summary&&lines>0&&<p className="flow-description" style={{WebkitLineClamp:lines,maxHeight:lines*15}}>{item.summary}</p>}</div>
    {['core','triggers'].includes(item?.lane)&&<span className={`flow-role-symbol flow-role-${item.lane}`} role="img" aria-label={item.roleLabel||item.lane}/>}
  </div>;
}
function ZoomMark({node,item,enter,select,compactScale,fitScale=Infinity,muted}) {
  const viewport=useViewport(),{zoom}=viewport;
  // A closed input group's mark stands in its summary's room, as an area's does.
  const area=['area','inputs-part'].includes(item.branch),size=34,tall=24,inset=area||['communication','inputs'].includes(item.branch)?8:12;
  // A summary scaled down whole to fit its box takes its mark down with it
  // until zoom gives the mark its ordinary screen size.
  const scale=area?compactScale:Math.min(fitScale,1/zoom);
  // A frame reserved exactly this room is fitted to within float round-off.
  if(node.width<(size+2*inset-.5)*scale||node.height<(tall+2*inset-.5)*scale)return null;
  const point={x:node.absolute.x+node.width-(size+inset)*scale,y:node.absolute.y+inset*scale};
  const name=item.branch==='inputs'?`${t('Inputs')} · ${item.name||item.title}`:item.name||item.title;
  return <button type="button" className={`flow-zoom-mark nopan ${muted?'flow-node-muted':''}`} data-zoom-into={node.id}
    style={{transform:`translate(${point.x}px,${point.y}px) scale(${scale})`,width:size,height:tall}}
    aria-label={t('Zoom into {0}',name)} onMouseEnter={()=>enter(node.id)}
    onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    <span className="flow-zoom-picture" aria-hidden="true"/>
  </button>;
}
function FrameTitle({node,item,focused,enter,select,muted}) {
  const viewport=useViewport();
  const scale=item.summaryScale||1;
  const component=item.branch==='component',communication=item.branch==='communication',inputs=['inputs','inputs-part'].includes(item.branch);
  const x=node.absolute.x+18*scale;
  return <div className={`flow-area-title nopan ${focused?'flow-area-title-focus':''} ${muted?'flow-node-muted':''} ${component?'flow-component-title':communication?'flow-communication-title':inputs?'flow-input-collection':item.lane==='core'?'flow-core-title':item.lane==='triggers'?'flow-entry-title':''}`}
    data-frame-title={node.id}
    style={{transform:`translate(${x}px,${node.absolute.y+12*scale}px) scale(${scale})`,transformOrigin:'top left',maxWidth:node.width/scale-36,
      '--flow-zoom':viewport.zoom*scale,
      '--flow-secondary-text':viewport.zoom*scale*13>=12?'visible':'hidden',
      '--flow-small-text':viewport.zoom*scale*12>=12?'visible':'hidden'}}
    onMouseEnter={()=>enter(node.id)} onClick={event=>{event.stopPropagation();select(node.id,event,false);}}>
    <strong>{item.heading||item.title}</strong>
    {item.metadata&&<div className="flow-component-meta">{item.metadata}</div>}
    {item.role&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
    {item.description&&<p className="flow-description" style={{maxWidth:cardText}}>{item.description}</p>}
  </div>;
}
function RoutedEdge({id,data}) {
  return <g aria-hidden="true" className={`flow-edge ${data.on?'flow-edge-active':''} ${data.dim?'flow-edge-muted':''}`} data-edge-id={id} data-edge-ids={data.edgeIDs.join(' ')}>
    <path className="flow-edge-casing" d={data.path} vectorEffect="non-scaling-stroke"/>
    <path d={data.path} fill="none" vectorEffect="non-scaling-stroke" style={data.possible||data.init?{strokeDasharray:data.init?'calc(3px / var(--flow-zoom, 1)) calc(5px / var(--flow-zoom, 1))':'calc(7px / var(--flow-zoom, 1)) calc(5px / var(--flow-zoom, 1))'}:undefined} markerStart={data.reverseArrow?`url(#${data.on?'flow-arrow-active':'flow-arrow'})`:undefined} markerEnd={data.arrow?`url(#${data.on?'flow-arrow-active':'flow-arrow'})`:undefined}/>
  </g>;
}
const nodeTypes={part:Part,area:Area}, edgeTypes={routed:RoutedEdge};

window.rmCreateFlow = async function(map, stage, records, relations, areas, inputOwner, callbacks) {
  ({records,areas}=inputGroupsByPart(records,areas,inputOwner));
  const source=stage.querySelector('svg'), host=document.createElement('div');
  host.className='flow-root';stage.appendChild(host);
  map.classList.add('flow-enabled','flow-initializing');source.style.display='none';source.setAttribute('aria-hidden','true');
  host.inert=true;
  const location=document.createElement('div');location.className='flow-location';location.setAttribute('aria-live','polite');stage.insertBefore(location,host);
  const status=document.createElement('p');status.className='flow-loading';status.textContent=t('Arranging the map…');stage.appendChild(status);
  function sizeWorkspace(){
    const workspace=stage.closest('.map-workspace');if(!workspace)return;
    map.style.setProperty('--flow-top',`${workspace.getBoundingClientRect().top+window.scrollY}px`);
  }
  const sizeObserver=new ResizeObserver(sizeWorkspace);
  [document.querySelector('.report-toolbar'),map.querySelector('.system-controls'),map.querySelector('.system-selection')].filter(Boolean).forEach(n=>sizeObserver.observe(n));
  window.addEventListener('resize',sizeWorkspace);requestAnimationFrame(sizeWorkspace);
  sizeWorkspace();
  const context=document.createElement('canvas').getContext('2d');
  const measure=(text,font)=>{context.font=font;return context.measureText(text).width;};
  const items=prepareCards(records,inputOwner,measure,t);
  const initialSize={width:host.clientWidth||1200,height:host.clientHeight||700};
  const place=createSemanticLayout(items,relations,areas);
  let semantic;
  try{semantic=await place(initialSize.width,initialSize.height);}catch(error){sizeObserver.disconnect();window.removeEventListener('resize',sizeWorkspace);host.remove();status.remove();location.remove();map.classList.remove('flow-enabled','flow-initializing');source.style.display='';source.removeAttribute('aria-hidden');throw error;}
  let {layout,scales}=semantic;
  let componentFonts=componentTextSizes(semantic.records);
  let byID=new Map(semantic.records.map(n=>[n.id,n])), placed=new Map(layout.nodes.map(n=>[n.id,n]));
  const geometryKey=nodes=>{
    const geometry=JSON.stringify(nodes.map(n=>[n.id,n.absolute.x,n.absolute.y,n.width,n.height]));
    let hash=0;for(let i=0;i<geometry.length;i++)hash=(Math.imul(31,hash)+geometry.charCodeAt(i))|0;
    return String(hash);
  };
  let layoutKey=geometryKey(layout.nodes),layoutSize=initialSize,fitting,layoutError;
  const initial={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false};
  let cameraRevision=0;
  let update, instance, view=initial, hoverArea='', pinnedLabels=new Map(), preview='', restorePending, pendingFocus, panning=false, initializing=true;
  // A declaration in a zoomed part: the one under the pointer, and the one
  // chosen, which the reading names. Each is {part, index} into the part's
  // symbols.
  let hoverMember=null,memberChoice=null,looseOf=()=>({});
  let detailed=new Set(),openComponents=new Set(),communicationsOpen=new Set(),arriving=new Set(),locationID='',locationSubject='',componentsOpen=false,zoom=systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom,paintedZoom,overviewFit=false;
  const closed=id=>closedContainer(id,placed,byID,detailed,componentsOpen,communicationsOpen,openComponents);
  new ResizeObserver(()=>{if(instance&&!initializing&&overviewFit)fitOverview();}).observe(host);
  // The part the camera stands in, drawing its declarations as tiles: the
  // one nearest the canvas's centre. It is the frame the reader looks at
  // (owner, 2026-09-28): its arrows run out of its own border toward the
  // part or area at their other end, each end marked by its plaque, and
  // the location names it.
  let deepPart='';
  function deepPartAt(v){
    const width=host.clientWidth,height=host.clientHeight,centre={x:(width/2-v.x)/v.zoom,y:(height/2-v.y)/v.zoom};
    let best='',nearest=Infinity;
    for(const n of layout.nodes){
      const item=byID.get(n.id);if(n.frame||item?.activation||!item?.symbols?.length||closed(n.id))continue;
      const {box,scale}=partBox({...item,...looseOf(n)});if(box.width*scale*v.zoom<860)continue;
      const x=n.absolute.x*v.zoom+v.x,y=n.absolute.y*v.zoom+v.y;
      if(x>=width||y>=height||x+n.width*v.zoom<=0||y+n.height*v.zoom<=0)continue;
      const distance=Math.hypot(Math.max(n.absolute.x-centre.x,0,centre.x-n.absolute.x-n.width),Math.max(n.absolute.y-centre.y,0,centre.y-n.absolute.y-n.height));
      if(distance<nearest){best=n.id;nearest=distance;}
    }
    return best;
  }
  function trackDeepPart(v){const next=deepPartAt(v);if(next!==deepPart){deepPart=next;update?.();}}
  // What the camera looks at: the part drawing its tiles, else what the
  // location row names; nothing on the whole map.
  function lookedAt(){return !instance||!openComponents.size&&!communicationsOpen.size?'':deepPart||locationID;}
  // A zoom (a pinch, the magnifier, "+" or "−") that brings another frame
  // before the reader has the column read it, the camera staying (owner,
  // 2026-09-28): zoomed into Command line client, the column had still read
  // redis-cli. A click reads without moving the camera; this is the zoom's.
  function follow(before){
    const now=lookedAt();
    if(now&&now!==before&&!byID.get(now)?.display&&byID.get(now)?.branch!=='inputs-part')callbacks.follow?.(now);
  }
  function updateLocation(event,subject=locationSubject){
    if(!instance)return;
    if(layoutError){location.textContent=t('Could not arrange this map. Reload to try again.');return;}
    if(!openComponents.size&&!communicationsOpen.size){location.textContent=t('System map');return;}
    const v=instance.getViewport(),w=host.clientWidth,h=host.clientHeight;
    // Pinch explores the point under the fingers. Opening the common layer
    // must not rename that context after a different participant at centre.
    const bounds=host.getBoundingClientRect(),pinch=event?.type==='wheel'&&event.ctrlKey;
    const aim=pinch?{x:event.clientX-bounds.left,y:event.clientY-bounds.top}:{x:w/2,y:h/2};
    const point={x:(aim.x-v.x)/v.zoom,y:(aim.y-v.y)/v.zoom};
    const candidates=layout.nodes.filter(n=>!closed(n.id)&&(scales.has(n.id)||byID.get(n.id)?.branch==='component'||['communication','inputs'].includes(byID.get(n.id)?.branch)||(!n.frame&&!semantic.owner(n.id)))).map(n=>{
      const x=n.absolute.x*v.zoom+v.x,y=n.absolute.y*v.zoom+v.y;
      const visible=Math.max(0,Math.min(w,x+n.width*v.zoom)-Math.max(0,x))*Math.max(0,Math.min(h,y+n.height*v.zoom)-Math.max(0,y));
      const dx=Math.max(n.absolute.x-point.x,0,point.x-n.absolute.x-n.width),dy=Math.max(n.absolute.y-point.y,0,point.y-n.absolute.y-n.height);
      let depth=0;for(let at=n.parentId;at;at=placed.get(at)?.parentId)depth++;
      return {id:n.id,visible,distance:dx*dx+dy*dy,depth};
    // The deepest frame under the point: pinched into Core infrastructure
    // until it filled the canvas, the location had still named redis-server,
    // which filled it as well.
    }).filter(n=>n.visible>0).sort((a,b)=>a.distance-b.distance||b.depth-a.depth||b.visible-a.visible);
    if(candidates.length)locationID=candidates.find(n=>n.id===subject)?.id||candidates[0].id;
    if(deepPart)locationID=deepPart;
    // An input collection is named as its heading reads, with its component.
    const nameOf=item=>item?.branch==='inputs'?[t('Inputs'),item.componentName].filter(Boolean).join(' · '):item?.name||item?.title;
    const names=[];for(let id=locationID;id;id=placed.get(id)?.parentId)names.unshift(nameOf(byID.get(id)));
    location.textContent=names.filter(Boolean).join(' / ')||t('System map');
  }
  const isOverview=()=>!detailed.size;
  let maxZoom=Math.max(2,...[...scales.values()].map(s=>1.8/s));
  const minZoom=()=>Math.min(.15,systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom);
  const hover=new HoverGate();
  const remember=event=>hover.remember(event.clientX,event.clientY);
  document.addEventListener('pointermove',remember,{passive:true});
  document.addEventListener('pointerdown',remember,{passive:true});
  const children=new Map(areas.map(a=>[a.id,a.nodes]));
  const inventories=new Map(areas.map(a=>[a.id,frameInventory(a.id,children,byID)]));
  const leaves=id=>children.has(id)?children.get(id).flatMap(leaves):[id];
  const communicationScales=()=>new Map(areas.filter(a=>['communication','inputs','inputs-part'].includes(byID.get(a.id)?.branch))
    .map(a=>[a.id,Math.min(1,...leaves(a.id).map(id=>byID.get(id)?.contentScale||1))]));
  const maximumZoom=()=>Math.max(2,...[...scales.values(),...communicationScales().values(),
    ...[...byID.values()].filter(n=>!n.children?.length).map(n=>n.contentScale||1)].map(scale=>1.8/scale),
    // Far enough into a part to read its declarations, and no farther.
    ...[...byID.values()].filter(n=>n.symbols?.length&&placed.has(n.id)).map(n=>1.125*deepZoom(n.id)));
  // The zoom at which a part's declarations read at their own size (and its
  // tiles are drawn at all).
  function deepZoom(id){
    const data={...byID.get(id),...looseOf(placed.get(id))},{box,scale}=partBox(data),grid=partGrid(data,box);
    return Math.max(grid.divisor/scale,860*1.02/(box.width*scale));
  }
  // What the emphasis needs of a declaration: its part, the names its calls
  // use for it and its source.
  function memberFacts(member){
    const symbols=byID.get(member?.part)?.symbols||[],symbol=symbols[member?.index];
    if(!symbol)return null;
    const owner=symbol.owner?symbols[symbol.owner-1]?.name:'';
    return {part:member.part,names:[symbol.name,owner?`${owner}.${symbol.name}`:''].filter(Boolean),sources:[symbol.href,symbol.open].filter(Boolean)};
  }
  // Centre a declaration at the scale its tile reads at.
  function focusMember(part,index,smooth=true){
    const n=placed.get(part);if(!n||!instance||initializing)return false;
    const data={...byID.get(part),...looseOf(n)},{box,scale}=partBox(data),grid=partGrid(data,box);
    const symbols=data.symbols||[],row=grid.rows[index]||grid.rows[(symbols[index]?.owner||0)-1];
    if(!row)return false;
    const {inset,columnGap:gap}=tileRoom,rect=host.getBoundingClientRect();
    // The declarations stand inside the part's 1px border (canvas.css).
    const point={x:n.absolute.x+scale*(1+(inset+row.column*(grid.tileWidth+gap)+grid.tileWidth/2)/grid.divisor),
      y:n.absolute.y+scale*(1+tileHeader(grid.divisor)+(inset+row.y+row.height/2)/grid.divisor)};
    overviewFit=false;hover.pause();preview='';map.clearMapPreview?.();
    arrive([part]);locationSubject=part;
    commitCamera(instance.setViewport(pointViewport(point,Math.min(maxZoom,deepZoom(part)),rect.width,rect.height),{duration:smooth?420:0}),part);
    return true;
  }
  // Whether a declaration's tile is drawn and in sight at the current
  // camera: a tile chosen there, on the canvas or in the reading, is only
  // marked (owner, 2026-09-28: the camera moves only to what is out of
  // sight).
  function memberInSight(part,index){
    const n=placed.get(part);if(!n||!instance||initializing||closed(part))return false;
    const data={...byID.get(part),...looseOf(n)},{box,scale}=partBox(data),grid=partGrid(data,box),v=instance.getViewport();
    if(box.width*scale*v.zoom<860)return false;
    const symbols=data.symbols||[],row=grid.rows[index]||grid.rows[(symbols[index]?.owner||0)-1];
    if(!row)return false;
    const {inset,columnGap:gap}=tileRoom;
    const x=(n.absolute.x+scale*(1+(inset+row.column*(grid.tileWidth+gap)+grid.tileWidth/2)/grid.divisor))*v.zoom+v.x;
    const y=(n.absolute.y+scale*(1+tileHeader(grid.divisor)+(inset+row.y+row.height/2)/grid.divisor))*v.zoom+v.y;
    return x>24&&y>24&&x<host.clientWidth-24&&y<host.clientHeight-24;
  }
  // Whether a frame (a component, an area, an Inputs collection) is drawn
  // and mostly in sight: then reading it only marks it. Entered by the
  // camera, redis-cli's Inputs had filled the canvas with one tile's word.
  function frameInSight(n){
    if(!instance||initializing||closed(n.id))return false;
    const v=instance.getViewport(),width=host.clientWidth,height=host.clientHeight;
    const left=n.absolute.x*v.zoom+v.x,top=n.absolute.y*v.zoom+v.y,right=left+n.width*v.zoom,bottom=top+n.height*v.zoom;
    const seen=Math.max(0,Math.min(right,width)-Math.max(left,0))*Math.max(0,Math.min(bottom,height)-Math.max(top,0));
    return seen>0&&seen>=.5*Math.min((right-left)*(bottom-top),width*height);
  }
  // The column's pointer on a group of inputs lights their tiles, or, while
  // their collection is closed, its row of that kind: it highlights and
  // dims nothing.
  let lit=new Set();
  function light(ids){const next=new Set(ids||[]);if(next.size===lit.size&&[...next].every(id=>lit.has(id)))return;lit=next;update?.();}
  function pointMember(part,index){
    const next=index<0?null:{part,index};
    if(cardOpen()||(hoverMember?.part===next?.part&&hoverMember?.index===next?.index))return;
    hoverMember=next;update?.();
  }
  // A tile clicked: the part is read with that declaration named, and the
  // camera centres the tile.
  function chooseMember(part,index,event){
    const symbol=byID.get(part)?.symbols?.[index];if(!symbol)return;
    memberChoice={part,index};hoverMember=null;
    select(part,event,false);
    // The reading is on the part once the host has shown it.
    setTimeout(()=>map.explainSource?.({key:symbol.href||symbol.open||'',href:symbol.href,open:symbol.open}),0);
    if(!memberInSight(part,index))focusMember(part,index);
  }
  // The reading column names a declaration (Find, a link in the reading, a
  // restored visit): its tile is the one chosen, and a new one is centred.
  map.addEventListener('repomap:reading',()=>{
    const named=map.explorerMember;if(!named?.owner)return;
    const part=named.owner,symbols=byID.get(part)?.symbols||[];
    const same=value=>!!value&&[named.key,named.href,named.open].includes(value);
    const index=symbols.findIndex(symbol=>same(symbol.href)||same(symbol.open));
    if(index<0||memberChoice?.part===part&&memberChoice.index===index)return;
    memberChoice={part,index};update?.();
    if(!map.readingRestoring&&!memberInSight(part,index))focusMember(part,index);
  });
  maxZoom=maximumZoom();
  function detailState(viewport,previous){
    const open=detailLayers(layout.nodes,semantic.records,viewport,host.clientWidth,host.clientHeight,previous);
    return {
      components:new Set([...open].filter(id=>byID.get(id)?.branch==='component')),
      areas:new Set([...open].filter(id=>byID.get(id)?.branch==='area')),
      communications:new Set([...open].filter(id=>['communication','inputs','inputs-part'].includes(byID.get(id)?.branch))),
    };
  }
  function updateDetail(viewport){
    const next=detailState(viewport,new Set([...openComponents,...detailed,...communicationsOpen,...arriving]));
    const same=(a,b)=>a.size===b.size&&[...a].every(id=>b.has(id));
    if(same(next.components,openComponents)&&same(next.areas,detailed)&&same(next.communications,communicationsOpen))return;
    openComponents=next.components;componentsOpen=!!openComponents.size;detailed=next.areas;communicationsOpen=next.communications;
    hoverArea='';hover.pause();update?.();
  }
  function parentArea(id){while(id){if(byID.get(id)?.branch==='area')return id;id=placed.get(id)?.parentId;}return '';}
  function rootOf(id){while(placed.get(id)?.parentId)id=placed.get(id).parentId;return id;}
  // The frame a thing is read in: its area, or else the open component it
  // stands in. Read by its area alone, a component's own space, border and
  // title were in no frame: on Redis, where entering redis-server opens four
  // components, its arrow ends vanished as the pointer crossed its space on
  // the way to them. A closed component marks nothing, so it does not take
  // the arrow ends from the open one beside it.
  function frameOf(id){
    const area=parentArea(id);if(area)return area;
    const root=rootOf(id);return byID.get(root)?.branch==='component'&&openComponents.has(root)?root:'';
  }
  function boundaryBetween(id,other){
    const shared=new Set();
    for(let at=other;at;at=placed.get(at)?.parentId)shared.add(at);
    for(let at=placed.get(id)?.parentId;at;at=placed.get(at)?.parentId)
      if(byID.get(at)?.branch==='area'&&!shared.has(at))return placed.get(at);
    return null;
  }
  // A frame's parts as its connections count them: an area's, or every part
  // in a component's areas and beside them.
  function frameMembers(id){
    return byID.get(id)?.branch==='component'?layout.nodes.filter(n=>!n.frame&&!byID.get(n.id)?.activation&&rootOf(n.id)===id).map(n=>n.id)
      :leaves(id).filter(leaf=>!byID.get(leaf)?.activation);
  }
  // What stands at a frame's connection's other end: another component, or
  // the outermost frame under a shared parent.
  const outsideOf=frame=>id=>rootOf(id)!==rootOf(frame)?rootOf(id):boundaryBetween(id,frame)?.id||id;
  // The drawn routes that carry some of the given edges, in drawing order.
  function routeIndex(routes){
    const byEdge=new Map();
    routes.forEach((route,i)=>{for(const id of route.edgeIDs){if(!byEdge.has(id))byEdge.set(id,[]);byEdge.get(id).push(i);}});
    return edges=>[...new Set(edges.flatMap(id=>byEdge.get(id)||[]))].sort((a,b)=>a-b).map(i=>routes[i]);
  }
  // The side of a box a point on its border stands on.
  const sideOf=(point,box)=>[['left',Math.abs(point.x-box.absolute.x)],['right',Math.abs(point.x-box.absolute.x-box.width)],
    ['top',Math.abs(point.y-box.absolute.y)],['bottom',Math.abs(point.y-box.absolute.y-box.height)]].sort((a,b)=>a[1]-b[1])[0][0];
  // Where a frame's connection meets the border it is drawn on (`root`):
  // of the ends of its drawn arrows, the one nearest that border, and which
  // side of it that is. Both directions of a pair of frames share one drawn
  // route, so the end a direction would take can be the other frame's: Data
  // type commands' incoming labels stood on Server runtime's border, in
  // the gap where the pointer looks at the whole component.
  function crossing(group,frame,matching){
    // A part looked at marks its own border.
    const root=!placed.get(frame)?.frame?frame:rootOf(group.outside)!==rootOf(frame)?rootOf(frame):boundaryBetween(group.insides[0],group.outside)?.id||frame;
    const route=group.incoming?matching.at(-1):matching[0],box=placed.get(root);
    if(!box)return null;
    // Where the drawn arrow crosses that border: its own route first.
    const rect={x:box.absolute.x,y:box.absolute.y,width:box.width,height:box.height};
    const point=[route,...matching].filter(Boolean).map(drawn=>borderCrossing(drawn.points||[],rect)).find(Boolean);
    if(point)return {root,point,side:sideOf(point,box),obstacles:frameObstacles(root)};
    // A part looked at whose route is drawn from its area's border gets a
    // short arrow of its own (placeStubs).
    return box.frame?null:{root,stub:true,obstacles:frameObstacles(root)};
  }
  // The short arrows of a part looked at, one per connection no drawn
  // route brings to its border (layout.mjs stubEnds): their plaques and the
  // routes that draw them.
  function placeStubs(labels,part){
    const n=placed.get(part),stubs=labels.filter(label=>label.stub);
    if(!n||!stubs.length)return [];
    const box={x:n.absolute.x,y:n.absolute.y,width:n.width,height:n.height};
    const outsides=stubs.map(label=>{const other=placed.get(label.outside);return {key:label.id,incoming:label.incoming,box:{x:other.absolute.x,y:other.absolute.y,width:other.width,height:other.height}};});
    const ends=stubEnds(box,outsides,.06*(n.width+n.height)/2);
    return stubs.map(label=>{
      const end=ends.get(label.id);Object.assign(label,{point:end.point,side:end.side});
      return {id:`stub:${label.id}`,from:part,to:part,edgeIDs:label.edges,points:end.points,path:end.points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' '),
        arrow:true,reverseArrow:false,possible:label.relations.every(relation=>relation.possible),init:false,near:true,kept:true,dim:false};
    });
  }
  // What a plaque on a frame's border must not cover: the boxes the frame
  // holds and the band its title stands in above them.
  function frameObstacles(root){
    const box=placed.get(root),inner=layout.nodes.filter(n=>n.parentId===root);
    if(!box.frame)return partObstacles(box);
    const boxes=inner.map(n=>({left:n.absolute.x,top:n.absolute.y,right:n.absolute.x+n.width,bottom:n.absolute.y+n.height}));
    if(!boxes.length)return boxes;
    return [{left:box.absolute.x,top:box.absolute.y,right:box.absolute.x+box.width,bottom:Math.min(...boxes.map(b=>b.top))},...boxes];
  }
  // Every connection of a frame that its drawn arrows show, where they meet
  // its border.
  function frameLabels(frame,matchingOf){
    return connections(frame,frameMembers(frame),layout.edges,outsideOf(frame)).flatMap(group=>{
      const matching=matchingOf(group.edges),at=crossing(group,frame,matching);
      if(!at)return [];
      const outside=byID.get(group.outside);
      return [{...group,id:`boundary:${frame}:${group.key}`,boundary:true,...at,title:outside?.name||outside?.title||''}];
    });
  }
  // A part's tiles and its title's band, as Part draws them: what a plaque
  // on the border of a part looked at must not cover.
  function partObstacles(n){
    const data={...byID.get(n.id),...looseOf(n)},{box,scale}=partBox(data),grid=partGrid(data,box);
    const {inset,columnGap:gap}=tileRoom,header=tileHeader(grid.divisor);
    const left=column=>n.absolute.x+scale*(1+(inset+column*(grid.tileWidth+gap))/grid.divisor);
    const top=y=>n.absolute.y+scale*(1+header+(inset+y)/grid.divisor);
    return [{left:n.absolute.x,top:n.absolute.y,right:n.absolute.x+n.width,bottom:n.absolute.y+scale*(1+header)},
      ...grid.blocks.map(block=>({left:left(block.column),top:top(block.y),right:left(block.column)+scale*grid.tileWidth/grid.divisor,bottom:top(block.y+block.height)}))];
  }
  // The arrowheads drawn, each the handle of the connection whose arrow it
  // ends: {route, tip, back, into, from}, `back` the point the arrow comes
  // from, `into` the drawn box the head touches and `from` the one at the
  // arrow's other end. The looked-at frame's labels, the routes' index and
  // the connections opened from a head are kept from the last drawing.
  let heads=[],lookedLabels=[],lastMatchingOf=()=>[],onHead=null;
  const headLabels=new Map();
  function drawnHeads(drawn){
    return drawn.flatMap(route=>{
      const [from,into]=route.boxes||[],points=route.points||[];
      if(points.length<2)return [];
      return [route.arrow&&into?{route,tip:points.at(-1),back:points.at(-2),into,from}:null,
        route.reverseArrow&&from?{route,tip:points[0],back:points[1],into:from,from:into}:null].filter(Boolean);
    });
  }
  // The arrowhead under a screen point: within 8px of the 12px of arrow the
  // head is drawn on, the nearest. Its line has no target; the head does.
  function headAt(x,y){
    if(!instance||initializing||!heads.length)return null;
    const zoom=instance.getViewport().zoom,p=instance.screenToFlowPosition({x,y}),length=12/zoom;
    let best=null,nearest=8/zoom;
    for(const head of heads){
      const dx=head.back.x-head.tip.x,dy=head.back.y-head.tip.y,d=Math.hypot(dx,dy)||1;
      const along=Math.max(0,Math.min(length,((p.x-head.tip.x)*dx+(p.y-head.tip.y)*dy)/d));
      const distance=Math.hypot(p.x-head.tip.x-dx/d*along,p.y-head.tip.y-dy/d*along);
      if(distance<nearest){best=head;nearest=distance;}
    }
    return best;
  }
  const within=(id,frame)=>{for(let at=id;at;at=placed.get(at)?.parentId)if(at===frame)return true;return false;};
  const isFrame=id=>['area','component'].includes(byID.get(id)?.branch);
  // The connection an arrowhead ends: the looked-at frame's label standing at
  // the head when there is one, so the head and its chip open one card; else
  // the incoming connection of the area or component the head points into;
  // else, when the head is on a destination, the inputs or a loose part, the
  // outgoing connection of the frame at the arrow's other end.
  function headConnection(head){
    const ids=new Set(head.route.edgeIDs),carries=label=>label.edges.some(id=>ids.has(id));
    const standing=lookedLabels.filter(label=>carries(label)&&Math.hypot(label.point.x-head.tip.x,label.point.y-head.tip.y)<1);
    if(standing.length)return standing.find(label=>label.incoming===within(head.into,label.root))||standing[0];
    const [frame,incoming]=isFrame(head.into)?[head.into,true]:isFrame(head.from)?[head.from,false]:[];
    return frame?frameLabels(frame,lastMatchingOf).find(label=>label.incoming===incoming&&carries(label))||null:null;
  }
  // A connection that is not one of the looked-at frame's is kept for its card.
  function keepHeadLabel(label){if(!lookedLabels.some(other=>other.id===label.id))headLabels.set(label.id,label);}
  // The screen box a card stands by for an arrowhead: 24px about its tip.
  function headRect(tip){
    const v=instance.getViewport(),box=host.getBoundingClientRect(),x=box.left+v.x+tip.x*v.zoom,y=box.top+v.y+tip.y*v.zoom;
    return {left:x-12,top:y-12,right:x+12,bottom:y+12,width:24,height:24};
  }
  // The pointer comes onto an arrowhead, moves along it, or leaves it.
  function pointHead(head,event){
    if(onHead&&head&&onHead.head.route.id===head.route.id&&onHead.head.tip.x===head.tip.x&&onHead.head.tip.y===head.tip.y)return;
    const label=head&&headConnection(head);
    // Onto its own card or chip the pointer has not left the look: their own
    // enter came first and keeps it.
    const own=onHead&&event.target.closest?.(`[data-card="${CSS.escape(onHead.key)}"],[data-connection-label="${CSS.escape(onHead.key.slice(6))}"]`);
    if(onHead&&onHead.key!==`label:${label?.id}`&&!own)leaveHandle(onHead.key,event);
    onHead=null;host.classList.toggle('flow-on-end',!!label);
    if(!label)return;
    const key=`label:${label.id}`;
    keepHeadLabel(label);onHead={key,head};
    aimAt(key,{rect:()=>headRect(head.tip),box:placed.get(label.root),side:label.side});
  }
  function leaveHead(event){if(onHead){leaveHandle(onHead.key,event);onHead=null;host.classList.remove('flow-on-end');}}
  function clearHover(){hoverArea='';preview='';map.clearMapPreview?.();update?.();}
  function commitCamera(movement,subject){
    // React Flow's imperative camera methods need not emit onMoveEnd. Save
    // only after they finish, or Back restores the previous display's camera.
    const revision=++cameraRevision;
    return Promise.resolve(movement).then(()=>{if(revision===cameraRevision&&!initializing){
      if(arriving.size){updateDetail(instance.getViewport());arriving=new Set();}
      updateLocation(undefined,subject);map.dispatchEvent(new Event('repomap:viewport'));}});
  }
  // Entering a frame opens it, so the camera may stand as small as an open
  // frame stays open. The move's own intermediate zooms must not close it on
  // the way: from a closed layer they did, and at the end it needed the larger
  // zoom that opens a closed layer, so microblog's /explore stood on the closed
  // Web routes summary with its title at 45 px. The frames a camera move
  // enters, and their ancestors, stay open through that move; a gesture ends it.
  function arrive(ids){
    arriving=new Set();
    for(const id of ids)for(let at=id;at;at=placed.get(at)?.parentId)if(placed.get(at)?.frame)arriving.add(at);
  }
  // One look for the whole map: an arrow end, whatever a layer adds.
  const look=createLook();
  let lookTimer;
  // Time passing opens a look the pointer rests on and ends one it left.
  function pump(){
    clearTimeout(lookTimer);
    const due=look.due;if(!due)return;
    lookTimer=setTimeout(()=>{if(look.tick(performance.now()))update?.();pump();},Math.max(0,due-performance.now())+5);
  }
  // The pointer on a card's handle: the card opens once it rests there. A
  // connection has two kinds of handle, its chip and its arrowhead: the card
  // stands by the one the pointer rested on, `handle` {rect, box, side}:
  // its screen box, and the frame and side the card goes out through.
  let lookHandle=null;
  function aimAt(key,handle=null){if(look.aim(key,performance.now())&&handle)lookHandle={key,...handle};pump();}
  // The pointer leaving a handle for the card it opened is safe on its way.
  function leaveHandle(key,event){
    const card=host.querySelector(`[data-card="${CSS.escape(key)}"]`)?.getBoundingClientRect();
    look.leave(key,performance.now(),event?{x:event.clientX,y:event.clientY}:null,card||null);pump();
  }
  // The pointer on the card itself keeps it; leaving it lingers.
  function lookAt(key){if(look.enter(key))update?.();pump();}
  function lookAway(key){look.leave(key,performance.now());pump();}
  // A card stands on the map, looked at or kept open: the frame being read
  // stays. A kept card whose frame has closed is not on the map and holds
  // nothing; it stands again when its frame opens.
  const shownCards=new Set();
  const cardOpen=()=>shownCards.size>0;
  function pin(key){
    if(key.startsWith('label:')&&!pinnedLabels.has(key.slice(6)))pinnedLabels.set(key.slice(6),labelAreas.get(key.slice(6))||headLabels.get(key.slice(6))?.area||'');
    update?.();
  }
  let labelAreas=new Map();
  function closeCard(key){
    if(key.startsWith('label:'))pinnedLabels.delete(key.slice(6));
    if(look.key===key)look.end();
    update?.();
  }
  // Escape, a card's ✕ or a click on empty canvas closes every open card.
  function closeCards(){
    const had=cardOpen();
    look.end();pinnedLabels.clear();clearTimeout(lookTimer);
    if(had)update?.();
    return had;
  }
  document.addEventListener('keydown',event=>{if(event.key==='Escape'&&closeCards())event.preventDefault();});
  function enter(id){
    // While a card is open or pinned the frame being read stays: the way to
    // the card crosses other parts, frames and empty canvas.
    if(!hover.allowed||cardOpen())return;
    const n=byID.get(id);if(!n||n.display)return;
    // Hover affects the drawing only. The links and description opened by a
    // click stay usable while the pointer crosses other cards to reach them.
    // What the pointer is on is the subject: a part is looked at alone, and
    // only its own arrows darken; a frame's title, border and empty space
    // look at the frame. Lifted to its area, a pointed part had lit all of
    // Data type commands' arrows. A target's initialization wiring stays
    // off, as it does for a chosen target: init is drawn for a looked-at end
    // only.
    if(hoverArea!==id){hoverArea=id;update?.();}
  }
  // A chosen input is entered as its path: the part holding its handler and
  // the path's parts nearest it in call depth, as many as stay readable in
  // one camera, with the trace dark from there. Their areas open.
  function focusPath(input,smooth=true){
    const nodes=[...new Set(byID.get(input)?.trace||[])].map(id=>placed.get(id)).filter(n=>n&&!n.frame);
    if(!nodes.length)return false;
    if(!instance||initializing){pendingFocus={path:input};return true;}
    overviewFit=false;hover.pause();preview='';map.clearMapPreview?.();
    const rect=host.getBoundingClientRect(),inPath=new Set(nodes.map(n=>n.id));
    // The trace's own arrows between its parts: those the input's code makes.
    const steps=layout.edges.filter(e=>inPath.has(e.from)&&inPath.has(e.to)&&e.relations.some(r=>r.operations?.includes(input))).map(e=>[e.from,e.to]);
    // Areas share one layer: the handler's area, or its component for a
    // loose part, says how far out the parts stay drawn.
    const least=layerFloor(layout.nodes,semantic.records,parentArea(nodes[0].id)||rootOf(nodes[0].id),rect.width,rect.height);
    const {taken,...viewport}=pathViewport(nodes,steps,rect.width,rect.height,byID.get(nodes[0].id)?.contentScale||1,{least});
    detailed=new Set([...detailed,...taken.map(n=>parentArea(n.id)).filter(Boolean)]);
    arrive(taken.map(n=>n.id));
    locationSubject=nodes[0].id;
    commitCamera(instance.setViewport(viewport,{duration:smooth?420:0}),nodes[0].id);
    return true;
  }
  // An input's tile among the inputs its handler's part takes: the group it
  // stands in inside the collection, not the collection's whole wall.
  function showInput(id,smooth=true){
    const tile=placed.get(id);if(!tile)return;
    if(!instance||initializing){pendingFocus={id,input:true};return;}
    overviewFit=false;hover.pause();preview='';map.clearMapPreview?.();
    const rect=host.getBoundingClientRect(),group=placed.get(tile.parentId);
    const grouped=byID.get(group?.id)?.branch==='inputs-part';
    communicationsOpen=new Set([...communicationsOpen,rootOf(id),...(grouped?[group.id]:[])]);
    arrive([id]);
    locationSubject=id;
    const least=layerFloor(layout.nodes,semantic.records,grouped?group.id:rootOf(id),rect.width,rect.height);
    commitCamera(instance.setViewport(tileViewport(tile,byID.get(group?.id)?.branch==='inputs-part'?group:null,rect.width,rect.height,byID.get(id)?.contentScale||1,{least}),{duration:smooth?420:0}),id);
  }
  // Every frame is entered by the one rule, at the scale its own content is
  // drawn at, and whole (owner, 2026-09-28: an area shows all its parts and
  // nothing is cut at the edges), no smaller than its layer stays open; a
  // frame too large even then is entered at its first part. Entered no
  // smaller than its parts' twelve-pixel headings, Redis's Core
  // infrastructure stood cut at both edges.
  function frameView(n,rect){
    const branch=byID.get(n.id).branch,component=branch==='component';
    const scale=component?(componentFonts.get(n.id)||20)/20:['communication','inputs','inputs-part'].includes(branch)?communicationScales().get(n.id)||1:scales.has(n.id)?byID.get(n.id)?.contentScale||1:1;
    const least=scales.has(n.id)||branch==='inputs-part'?layerFloor(layout.nodes,semantic.records,n.id,rect.width,rect.height):Infinity;
    return frameViewport(n,layout.nodes,rect.width,rect.height,scale,{whole:component,pad:component?12:24,floor:staysOpen,least});
  }
  // The level the camera would stand at with viewport `v` (semantic.mjs,
  // detailLevel): the frames open there as a move to it decides them
  // (updateDetail), and whether a part in sight draws its tiles, as Part
  // draws them once it stands 860px wide. With `aim`, the point a pinch
  // began over, the part there reading its title is a level of its own
  // (owner, 2026-09-28): one pinch over Redis's 7px Replication card had
  // gone on into syncRead's tiles, past the card the reader wanted to read.
  function levelAt(v,aim=null){
    const next=detailState(v,new Set([...openComponents,...detailed,...communicationsOpen,...arriving]));
    const shut=id=>closedContainer(id,placed,byID,next.areas,next.components.size>0,next.communications,next.components);
    const width=host.clientWidth,height=host.clientHeight;
    const tiles=layout.nodes.some(n=>{
      const item=byID.get(n.id);if(n.frame||item?.activation||!item?.symbols?.length||shut(n.id))return false;
      const {box,scale}=partBox({...item,...looseOf(n)});
      if(box.width*scale*v.zoom<860)return false;
      const x=n.absolute.x*v.zoom+v.x,y=n.absolute.y*v.zoom+v.y;
      return x<width&&y<height&&x+n.width*v.zoom>0&&y+n.height*v.zoom>0;
    });
    const level=detailLevel(layout.nodes,[...next.components,...next.areas,...next.communications],tiles);
    const under=aim&&layout.nodes.find(n=>{const item=byID.get(n.id);return !n.frame&&!item?.activation&&!shut(n.id)&&
      aim.x>=n.absolute.x&&aim.x<=n.absolute.x+n.width&&aim.y>=n.absolute.y&&aim.y<=n.absolute.y+n.height;});
    return under&&titleSize(under,v.zoom)>=12?level+.5:level;
  }
  // A part's title on screen, in pixels, at a zoom.
  function titleSize(n,zoom){
    const item=byID.get(n.id),{scale}=partBox({...item,...looseOf(n)});
    return (looseOf(n).standaloneHeading?12:17)*scale*zoom;
  }
  // "−" steps out one level, as a zoom mark steps in one: from a part's
  // tiles to the frame holding it, from an open area to its component, from
  // an open component to the whole map. Zooming out by a fifth, it had left
  // Redis's readers at the level they were on; they reached for "Show whole
  // map" up to nine times in a question. The camera takes the frame as
  // entering it would, no closer than the level below the one it leaves.
  function stepOut(){
    const v=instance.getViewport(),rect=host.getBoundingClientRect(),level=levelAt(v);
    if(level===0){overviewFit=false;commitCamera(instance.zoomTo(v.zoom*.8));return;}
    const centre={x:(rect.width/2-v.x)/v.zoom,y:(rect.height/2-v.y)/v.zoom};
    const inView=n=>{const x=n.absolute.x*v.zoom+v.x,y=n.absolute.y*v.zoom+v.y;return x<rect.width&&y<rect.height&&x+n.width*v.zoom>0&&y+n.height*v.zoom>0;};
    const distance=n=>Math.hypot(Math.max(n.absolute.x-centre.x,0,centre.x-n.absolute.x-n.width),Math.max(n.absolute.y-centre.y,0,centre.y-n.absolute.y-n.height));
    const depth=n=>{let d=0;for(let at=n.parentId;at;at=placed.get(at)?.parentId)d++;return d;};
    // What the camera is on at its level: the part drawing its tiles, or
    // else an open frame of the deepest open layer, nearest the centre.
    const deep=layout.nodes.filter(n=>{const item=byID.get(n.id);if(n.frame||item?.activation||!item?.symbols?.length||closed(n.id)||!inView(n))return false;
      const {box,scale}=partBox({...item,...looseOf(n)});return box.width*scale*v.zoom>=860;});
    const open=[...openComponents,...detailed,...communicationsOpen].map(id=>placed.get(id)).filter(n=>n&&!n.display&&inView(n));
    const deepest=Math.max(-1,...open.map(depth));
    const from=(deep.length?deep:open.filter(n=>depth(n)===deepest)).sort((a,b)=>distance(a)-distance(b))[0];
    let up=from?.parentId;while(up&&placed.get(up)?.display)up=placed.get(up).parentId;
    closeCards();hover.pause();preview='';map.clearMapPreview?.();
    if(!up||!from){fitOverview(420);return;}
    const view=frameView(placed.get(up),rect),point={x:(rect.width/2-view.x)/view.zoom,y:(rect.height/2-view.y)/view.zoom};
    const at=zoom=>({x:rect.width/2-point.x*zoom,y:rect.height/2-point.y*zoom,zoom});
    const zoom=zoomBelow(view.zoom,minZoom(),z=>levelAt(at(z)),level-1);
    overviewFit=false;arriving=new Set();locationSubject=up;
    const before=lookedAt();
    commitCamera(instance.setViewport(zoom===view.zoom?view:at(zoom),{duration:420}),up).then(()=>follow(before));
  }
  function focus(id,center=true,smooth=true){
    const n=placed.get(id);if(!n)return;
    // A part read with one of its declarations named is entered at that
    // tile, unless the tile is in sight.
    if(memberChoice?.part===id&&(!center&&memberInSight(id,memberChoice.index)||focusMember(id,memberChoice.index,smooth)))return;
    const record=byID.get(id);
    if(record?.activation&&focusPath(id,smooth))return;
    if(record?.activation){showInput(id,smooth);return;}
    if(!instance||initializing){pendingFocus={id,center};return;}
    overviewFit=false;
    hover.pause();preview='';map.clearMapPreview?.();
    const viewport=instance.getViewport(), rect=host.getBoundingClientRect();
    const contentScale=byID.get(n.id)?.contentScale||1;
    if(!center&&(n.frame&&frameInSight(n)||readableFocus(n.id,placed,byID,detailed,componentsOpen,viewport,rect.width,rect.height,communicationsOpen,openComponents)))return;
    locationSubject=id;
    // A plain tile is entered with its display group: every tile of it opens
    // and the camera frames the group, whose heading names them all. Framed
    // alone, a tile read only "gethostbyname", the heading below the camera.
    const group=record?.displayGroupTitle&&placed.get(`display-group:${record.displayGroup}`);
    if(group){
      const tiles=byID.get(group.id).tiles;
      communicationsOpen=new Set([...communicationsOpen,...tiles]);arrive(tiles);
      // The box the open group draws: its tiles and their heading, at the
      // scale their calls are drawn at. A plain tile grown whole at the fit
      // draws its calls larger than one: taken as one, Redis's group entered
      // at 1280x720 stood 978px wide in an 894px canvas, a call cut off.
      const {width,height}=groupLook(group),scale=Math.min(...tiles.flatMap(tile=>leaves(tile).map(id=>byID.get(id)?.contentScale||1)));
      commitCamera(instance.setViewport(frameViewport({...group,width,height},layout.nodes,rect.width,rect.height,scale,{floor:staysOpen}),{duration:smooth?420:0}),id);return;
    }
    if(n.frame){
      const branch=byID.get(n.id).branch,component=branch==='component';
      // Entering a frame opens it, so it may be shown as small as an open frame
      // stays open, about twelve pixels of text, rather than as large as a
      // closed one needs to open by itself.
      // Entered at its inputs' reading scale, a collection opens its groups
      // too: they stay open there, as a pinch opens them farther in.
      const groups=branch==='inputs'?(children.get(n.id)||[]).filter(id=>byID.get(id)?.branch==='inputs-part'):[];
      if(['communication','inputs','inputs-part'].includes(branch))communicationsOpen=new Set([...communicationsOpen,n.id,rootOf(n.id),...groups]);
      else if(!component)detailed=new Set([...detailed,n.id]);
      if(!component)arrive([n.id,...groups]);
      commitCamera(instance.setViewport(frameView(n,rect),{duration:smooth?420:0}),id);return;
    }
    // A part is framed whole, at its reading scale when it fits there.
    const fit=Math.min((rect.width-48)/n.width,(rect.height-48)/n.height);
    commitCamera(instance.setViewport(partViewport(n,Math.min(1/contentScale,fit),rect.width,rect.height),{duration:smooth?420:0}),id);
  }
  function capture(){return instance&&!initializing?{...instance.getViewport(),layoutKey,overview:isOverview(),detailAreas:[...detailed],componentsOpen,openComponents:[...openComponents],communicationsOpen:[...communicationsOpen],fit:overviewFit}:restorePending||null;}
  function restore(v){if(!v)return;
    if(!instance||initializing){restorePending=v;return;}
    if(v.fit===true||(v.fit===undefined&&v.componentsOpen===false&&!view.scope&&!view.operation)){fitOverview();return;}
    // A regenerated report can have different geometry. Old screen coordinates
    // must not place its still-selected item outside the visible viewport.
    if(!Number.isFinite(v.zoom)||v.layoutKey!==layoutKey){if(view.scope||view.operation)focus(view.scope||view.operation,true);else fitOverview();return;}
    overviewFit=false;
    locationSubject=view.scope||view.operation||'';
    const previous=new Set([...(v.openComponents||[]),...(v.detailAreas||[]),...(v.communicationsOpen||[])]);
    const next=detailState(v,previous);
    detailed=next.areas;openComponents=next.components;communicationsOpen=next.communications;
    componentsOpen=!!openComponents.size;
    zoom=v.zoom;update?.();
    hover.pause();preview='';map.clearMapPreview?.();
    if(instance)commitCamera(instance.setViewport(v));else restorePending=v;
  }
  function fitOverview(duration=0){
    overviewFit=true;
    locationSubject='';
    if(fitting)return fitting;
    fitting=(async()=>{
      while(overviewFit){
        const width=host.clientWidth,height=host.clientHeight;
        // Screen-sized labels need a new initial placement after the window
        // changes shape. Ordinary pan/zoom and selection retain the fixed world.
        if(width!==layoutSize.width||height!==layoutSize.height){
          const next=await place(width,height);
          if(!overviewFit)return;
          if(width!==host.clientWidth||height!==host.clientHeight)continue;
          semantic=next;({layout,scales}=next);
          byID=new Map(next.records.map(n=>[n.id,n]));placed=new Map(layout.nodes.map(n=>[n.id,n]));
          componentFonts=componentTextSizes(next.records);
          layoutKey=geometryKey(layout.nodes);
          maxZoom=maximumZoom();
        }
        layoutSize={width,height};
        detailed=new Set();openComponents=new Set();componentsOpen=false;communicationsOpen=new Set();hoverArea='';update?.();
        const v=systemViewport(layout.nodes,width,height);
        hover.pause();
        await commitCamera(instance.setViewport(v,{duration}));
        layoutError=null;updateLocation();
        if(width===host.clientWidth&&height===host.clientHeight)break;
      }
    })().catch(async error=>{
      // A failed reflow leaves the last complete world usable. In particular,
      // a final-size reflow during startup must not leave that world hidden.
      overviewFit=false;layoutError=error;console.error(error);
      if(initializing)await commitCamera(instance.setViewport(systemViewport(layout.nodes,host.clientWidth,host.clientHeight)));
      updateLocation();
    }).finally(()=>{fitting=null;});
    return fitting;
  }
  function select(id,event,center=false){
    // A group of inputs is named by the part their handlers are in; choosing
    // it reads that part.
    if(byID.get(id)?.branch==='inputs-part')id=byID.get(id).owner;
    hover.remember(event.clientX,event.clientY);hover.pause();hoverArea='';preview='';map.clearMapPreview?.();callbacks.select(id,center);
  }
  // Lay out text once for the whole-map camera. Pan clips that fixed card;
  // zoom scales it with the map instead of rewrapping it at every wheel tick.
  // The whole-map scale at which a summary fits its box whole (see overviewScale).
  function useOverviewFit(n){
    const zoom=systemViewport(layout.nodes,layoutSize.width,layoutSize.height).zoom,item=byID.get(n.id);
    const fit=useMemo(()=>overviewScale(item,n.width*zoom,n.height*zoom,layoutSize.height-2*overviewInset),[n.width,n.height,zoom]);
    return {fit,zoom};
  }
  function ComponentOverview({node:n,fit,zoom,muted}){
    // Short of its reserved room, the summary is laid out at that room and
    // scaled down whole instead of being cut at the frame's edge.
    const item=byID.get(n.id),inventory=inventories.get(n.id),screenWidth=n.width*zoom/fit,screenHeight=n.height*zoom/fit;
    // Scaled whole, even a frame too narrow for its text at full size keeps
    // its complete smaller summary instead of standing blank.
    const visible=n.width*zoom>0,width=Math.min(320,screenWidth-16),contentWidth=Math.max(1,width-16);
    const heading=useMemo(()=>visible?overviewHeading(item,screenWidth,measure):null,[screenWidth]);
    const text=useMemo(()=>{
      if(!visible)return null;
      const communication=item.branch==='communication',inputs=item.branch==='inputs',areaIDs=communication||inputs?[]:
        // Areas first, then the loose parts beside them.
        [...(children.get(n.id)||[])].sort((a,b)=>(byID.get(b)?.branch==='area')-(byID.get(a)?.branch==='area'));
      const textHeight=(text,font,lineHeight)=>wrapText(text,contentWidth,font,measure).length*lineHeight;
      const listHeight=areaIDs.length?7+areaIDs.reduce((h,id)=>h+10+textHeight(byID.get(id).name||byID.get(id).title,'500 13px system-ui',18),0):0;
      const counts=inventory.parts?t('{0} parts',inventory.parts):'';
      const roleHeight=item.role?textHeight(item.role,'600 13px system-ui',18)+10:0;
      const countsHeight=textHeight(counts,'500 12px system-ui',17)+10;
      return {communication,inputs,areaIDs,listHeight,counts,roleHeight,countsHeight,inputHeight:inputs?item.overviewHeightAtWidth(screenWidth):0};
    },[visible,contentWidth]);
    if(!heading)return null;
    const {communication,inputs,areaIDs,listHeight,counts,roleHeight,countsHeight}=text,scale=fit/zoom;
    let remaining=screenHeight-32-heading.height-listHeight-(areaIDs.length?10:0);
    const listOverflow=remaining<0;
    const showRole=!communication&&!inputs&&roleHeight>0&&remaining>=roleHeight;
    if(showRole)remaining-=roleHeight;
    const showCounts=!communication&&!inputs&&remaining>=countsHeight;
    if(showCounts)remaining-=countsHeight;
    const descriptionLines=Math.floor((remaining-10)/18);
    const x=n.absolute.x+8*scale,y=n.absolute.y+8*scale;
    return <div key={'component-'+n.id} className={`flow-component-overview nopan ${item.branch==='communication'?'flow-communication-overview':item.branch==='inputs'?'flow-input-collection':''} ${muted?'flow-node-muted':''}`}
      data-component-overview={n.id} style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width,maxHeight:screenHeight-16}}
      onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,false);}}>
      {heading.lines.length>0&&<div className="flow-component-overview-heading" style={{maxWidth:heading.width,minHeight:inputs?32:undefined,paddingTop:heading.clearZoom?32:undefined}}>
        <strong style={heading.scale<1?{fontSize:heading.fontSize,lineHeight:`${heading.lineHeight}px`}:undefined}>{heading.lines.join('\n')}</strong></div>}
      {showRole&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
      {!communication&&!inputs&&descriptionLines>=2&&item.description&&<p className="flow-description flow-description-compact" style={{WebkitLineClamp:descriptionLines}}>{item.description}</p>}
      {inputs&&<InputTypes groups={item.inputGroups} lit={lit}/>}
      {areaIDs.length>0&&<ul className={`flow-component-areas ${listOverflow?'flow-scrollable':''}`} onWheelCapture={scrollInventory}>{areaIDs.map(id=><li key={id}>
        <button type="button" className="nopan" data-overview-area={id} onClick={event=>{event.stopPropagation();select(id,event,true);}}>{byID.get(id).name||byID.get(id).title}</button>
      </li>)}</ul>}
      {showCounts&&<div className="flow-inside-counts">{counts}</div>}
    </div>;
  }
  // A display group's frame carries the text its frames all name, once, in
  // its band under the tiles or after them, where no arrow runs. It is laid
  // out at the whole-map camera like their summaries and zooms with the map;
  // once the tiles open it reads at their open frames' title size, and the
  // open tiles stay plain under it: three open tiles each titled "DNS
  // resolver" said one thing three times. It is no participant: nothing to
  // hover, choose or enter.
  // The band is the heading's room, so it shrinks with the heading: the
  // frame of an open group wraps its tiles and their heading, and the room
  // the closed heading takes at the whole-map camera is left outside it
  // instead of standing framed and empty under the open calls.
  const groupHeadingLines=new Map();
  function groupLook(n){
    const item=byID.get(n.id),zoom=systemViewport(layout.nodes,layoutSize.width,layoutSize.height).zoom,beside=item.side==='right';
    const room=beside?Infinity:n.width*zoom,key=`${n.id} ${room}`;
    if(!groupHeadingLines.has(key))groupHeadingLines.set(key,item.headingAt(room));
    const heading=groupHeadingLines.get(key);
    const open=item.tiles.some(id=>communicationsOpen.has(id)),need=beside?heading.extent:heading.height;
    // A band left short of the heading takes it smaller, never over the tiles.
    // Open, it takes the 17px of the open frames' titles at their own scale:
    // shrunk by the closed band's shortfall as well, it read at 13.4px over
    // 16.2px calls when Redis's 1280x720 map entered it.
    const closed=Math.min(1,item.band*zoom/need)/zoom;
    const scale=open?Math.min(closed,(byID.get(item.tiles[0])?.summaryScale||1)*17/heading.fontSize):closed;
    const band=open?Math.min(item.band,need*scale):item.band;
    return {item,heading,beside,scale,band,width:beside?n.width-item.band+band:n.width,height:beside?n.height:n.height-item.band+band};
  }
  function GroupHeading({node:n,muted}){
    const {heading,beside,scale,band,width,height}=groupLook(n);
    const x=beside?n.absolute.x+width-band+8*scale:n.absolute.x+8*scale;
    const y=beside?n.absolute.y+16:n.absolute.y+height-band+4*scale;
    return <div className={`flow-group-heading ${muted?'flow-node-muted':''}`} data-group-heading={n.id}
      style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width:heading.width,
        fontSize:heading.fontSize,lineHeight:`${heading.lineHeight}px`}}>{heading.lines.join('\n')}</div>;
  }
  function ComponentPresentation({node,focused,muted}){
    const item=byID.get(node.id),open=item.branch==='component'?openComponents.has(node.id):communicationsOpen.has(node.id);
    const {fit,zoom}=useOverviewFit(node);
    // Open, a plain tile shows its calls under its group's heading alone.
    if(open&&item.displayGroupTitle)return null;
    return open?<FrameTitle node={node} item={item} focused={focused} enter={enter} select={select} muted={muted}/>:<>
      <ComponentOverview node={node} fit={fit} zoom={zoom} muted={muted}/><ZoomMark node={node} item={item} fitScale={fit<1?fit/zoom:undefined} enter={enter} select={select} muted={muted}/>
    </>;
  }
  function App(){
    const [,setVersion]=useState(0);update=()=>setVersion(v=>v+1);
    const pointed=hoverMember&&hoverArea===hoverMember.part?hoverMember:!hoverArea&&memberChoice&&view.scope===memberChoice.part?memberChoice:null;
    const state=emphasis(view,hoverArea,leaves,layout.edges,memberFacts(pointed));
    // What recedes is the reader's own choice, drawn with nothing pointed at;
    // the pointer highlights and never recedes (emphasis.mjs).
    const rest=hoverArea?emphasis(view,'',leaves,layout.edges,memberFacts(memberChoice&&view.scope===memberChoice.part?memberChoice:null)):state;
    const recede=rest.mode==='all'?null:rest;
    const context=focusAncestors(state.focus,placed);
    const visible=id=>!closed(id);
    const drawing=layout;
    const groupHeadings=useMemo(()=>{
      const scale=1/firstDetailZoom(layout.nodes,semantic.records,layoutSize.width,layoutSize.height);
      // The closed card is a title over a foot row and the role mark: the
      // title is fitted into what the foot row leaves.
      return new Map(layout.nodes.filter(n=>scales.has(n.id)||byID.get(n.id)?.branch==='inputs-part').map(n=>[n.id,groupHeading(n,byID.get(n.id).name||byID.get(n.id).title,scale,measure,56,48)]));
    },[layoutKey]);
    const standaloneHeadings=useMemo(()=>{
      const scale=1/firstDetailZoom(layout.nodes,semantic.records,layoutSize.width,layoutSize.height);
      return new Map(layout.nodes.filter(n=>!n.frame&&!byID.get(n.id).activation&&byID.get(n.parentId)?.branch==='component').map(n=>{
        const item=byID.get(n.id),heading=groupHeading(n,item.name||item.title,scale,measure,50,15,58);
        return [n.id,{...heading,width:n.width/heading.scale,height:n.height/heading.scale}];
      }));
    },[layoutKey]);
    const overview=isOverview();
    // A loose part beside areas is a peer of their closed summaries while
    // they are closed: its heading fitted to its box at their scale. Once the
    // areas open it is a peer of their parts: the same card at the same
    // scale, filling its box. Kept at the summary scale, Redis's Debug
    // symbols read 41 px beside 17 px parts.
    const looseLook=n=>{
      const heading=standaloneHeadings.get(n.id);
      if(!heading)return {};
      const holdsAreas=(children.get(placed.get(n.id)?.parentId)||[]).some(id=>byID.get(id)?.branch==='area');
      if(!holdsAreas||!detailed.size)return {standaloneHeading:heading};
      const scale=byID.get(n.id)?.contentScale||1;
      return {fill:{width:n.width/scale,height:n.height/scale}};
    };
    looseOf=looseLook;
    // Every part can be zoomed into until its declarations read at their own size.
    maxZoom=useMemo(()=>maximumZoom(),[layoutKey,detailed.size>0]);
    // Zoomed into one area with nothing hovered or chosen, that area is what
    // the reader is looking at: its arrow ends keep their plaques.
    const zoomedArea=state.mode==='all'&&detailed.size===1?[...detailed][0]:'';
    // Pinned cards belong to the frame their part stands in; that frame stays
    // the one being read while they are pinned.
    const pinnedFrame=[...pinnedLabels.values()].find(id=>byID.get(id)?.branch==='area')||'';
    // A thing pointed at or chosen is read in its frame: an area, or else the
    // open component it stands in.
    // The part the camera stands in is looked at whatever the pointer or the
    // reading is on, unless a card is pinned.
    const deep=pinnedFrame?'':deepPart;
    const chosen=pinnedFrame||deep||(state.mode==='hover'?frameOf(hoverArea):state.mode==='selection'?frameOf(view.scope):zoomedArea);
    // The frame the reader looks at has its arrow ends marked: an open area
    // with its parts, or else the open component with its areas and loose parts.
    const lookedComponent=chosen?rootOf(chosen):openComponents.size===1?[...openComponents][0]:'';
    const area=deep||(chosen&&detailed.has(chosen)?chosen:byID.get(lookedComponent)?.branch==='component'&&openComponents.has(lookedComponent)?lookedComponent:chosen);
    const wholeComponent=byID.get(area)?.branch==='component';
    const childOf=id=>{while(id&&placed.get(id)?.parentId!==area)id=placed.get(id)?.parentId;return id||'';};
    const members=!area?[]:wholeComponent?layout.nodes.filter(n=>!n.frame&&!byID.get(n.id)?.activation&&childOf(n.id)).map(n=>n.id):leaves(area);
    const parts=new Set(wholeComponent?layout.nodes.filter(n=>n.parentId===area).map(n=>n.id):members);
    const initVisible=(state.mode==='hover'&&byID.get(hoverArea)?.branch!=='component')||state.mode==='operation'||(state.mode==='selection'&&byID.get(view.scope)?.branch!=='component');
    // A part looked at draws its arrows from its own border, not from the
    // border of the area holding it.
    const boundary=(id,other)=>id===deep?null:boundaryBetween(id,other);
    const routes=routeDrawing(drawing.edges,closed,state.activeEdges,recede,boundary,initVisible,deep?new Set([deep]):zoomedArea?new Set(leaves(zoomedArea)):null);
    const matchingOf=routeIndex(routes);
    const labels=(area?connections(area,members,layout.edges,outsideOf(area)):[]).flatMap(group=>{
      // A label stands where its arrow meets the frame it marks.
      const matching=matchingOf(group.edges),at=crossing(group,area,matching);
      if(!at)return [];
      const outside=byID.get(group.outside);
      return [{...group,id:`boundary:${area}:${group.key}`,boundary:true,...at,title:outside.name||outside.title}];
    });
    const stubRoutes=deep?placeStubs(labels,deep):[];
    endPlaques(labels,parts,id=>wholeComponent?childOf(id):id);
    const labelsShown=!!area&&visible(area)&&(detailed.has(area)||openComponents.has(area)||area===deep);
    lookedLabels=labelsShown?labels:[];lastMatchingOf=matchingOf;
    // A connection opened from its arrowhead that is not one of the looked-at
    // frame's stays while its card is looked at, about to open or kept open.
    for(const id of headLabels.keys())if(![look.key,look.pending].includes(`label:${id}`)&&!pinnedLabels.has(id))headLabels.delete(id);
    const cardLabels=[...lookedLabels,...[...headLabels.values()].filter(label=>visible(label.area)&&!lookedLabels.some(other=>other.id===label.id))];
    labelAreas=new Map(cardLabels.map(label=>[label.id,label.area]));
    // An arrow end whose card is open, or kept open, outlines in place the
    // parts behind it and its own arrows are dark (owner's 2a); like the
    // pointer, it recedes nothing.
    const endLabel=cardLabels.find(label=>look.key===`label:${label.id}`)||[...pinnedLabels.keys()].reverse().map(id=>cardLabels.find(label=>label.id===id)).find(Boolean);
    const end=endLabel?endEmphasis(endLabel,layout.edges):null;
    const shown=end||state;
    const active=end?end.activeEdges:state.activeEdges;
    const drawn=[...(end?routeDrawing(drawing.edges,closed,end.activeEdges,recede,boundary,initVisible,null):routes),
      ...stubRoutes.map(route=>({...route,on:route.edgeIDs.some(id=>active.has(id))}))];
    heads=drawnHeads(drawn);
    const shownContext=end?focusAncestors(end.focus,placed):context;
    // Far enough into one part to read its declarations.
    // The magnifier enters at the scale the part's declarations read at: the
    // part whole when it fits there, else its head and first column. Fitted
    // to the canvas, Redis's Persistence opened at its title's scale with no
    // declaration drawn.
    // The magnifier frames the part whole where its tiles are drawn (owner,
    // 2026-09-28: nothing cut at the edges; Command line client had stood
    // with its last column off the canvas), and the column reads it.
    function deepInto(node){
      if(!instance)return;
      const rect=host.getBoundingClientRect(),readable=deepZoom(node.id);
      const fit=Math.min((rect.width-48)/node.width,(rect.height-48)/node.height);
      const {box,scale}=partBox({...byID.get(node.id),...looseOf(node)}),tiles=860*1.02/(box.width*scale);
      overviewFit=false;hover.pause();arrive([node.id]);locationSubject=node.id;
      callbacks.follow?.(node.id,true);
      commitCamera(instance.setViewport(deepViewport(node,Math.min(maxZoom,Math.max(tiles,Math.min(fit,readable*1.125))),rect.width,rect.height),{duration:420}),node.id);
    }
    useEffect(()=>callbacks.emphasis?.({...state,overview}),[state.mode,state.subject,view.scope,overview]);
    // A closed frame stands for the participants hidden inside it.
    // An input group closed: its inputs are not readable yet, its name is.
    const closedGroup=id=>byID.get(id)?.branch==='inputs-part'&&!communicationsOpen.has(id);
    const shut=id=>scales.has(id)?!detailed.has(id):byID.get(id)?.branch==='component'?!openComponents.has(id):['communication','inputs','inputs-part'].includes(byID.get(id)?.branch)&&!communicationsOpen.has(id);
    // The subject is dark: a part's outline, a frame's border at the arrows'
    // 2.5px. The parts across its dark arrows take the same outline; a
    // frame's own parts stay as they are.
    // A closed frame stands for the parts behind an end that it hides.
    const subjects=end?new Set([...end.focus].map(id=>closed(id)?.id||id)):state.mode==='search'?state.focus:new Set([state.subject].filter(Boolean));
    // A frame stands for its parts; a display group (the frames sharing one
    // destination's text) for its frames: its frame and heading stay with them.
    const standsFor=id=>{const n=placed.get(id),item=byID.get(id);return [id,...(item?.display?(item.tiles||[]).flatMap(standsFor):n?.frame?leaves(id):[])];};
    const muted=recedes(rest,shown,subjects,standsFor);
    const nodes=drawing.nodes.map(n=>{
      const item=byID.get(n.id),focused=shown.focus.has(n.id);
      // A display group draws the box its heading's band leaves it.
      const box=n.display&&item?.headingAt?groupLook(n):n;
      const reading=view.scope===n.id||view.operation===n.id;
      const contains=n.frame&&leaves(n.id).some(id=>shown.participants.has(id));
      // The other end of a looked-at end stays as it is: only the parts
      // behind the end are outlined.
      const on=!end&&(shown.participants.has(n.id)||contains&&(overview||shut(n.id)));
      return {...n,width:box.width,height:box.height,type:n.frame?'area':'part',selected:reading,measured:{width:box.width,height:box.height},
        selectable:false,draggable:false,connectable:false,
        style:{width:box.width,height:box.height,visibility:visible(n.id)?'visible':'hidden'},
        className:`${muted(n.id)?'flow-node-muted':''} ${subjects.has(n.id)?'flow-node-focus':on&&!focused?'flow-node-connected':''} ${shownContext.has(n.id)?'flow-node-context':''} ${reading?'flow-node-reading':''} ${lit.has(n.id)?'flow-node-lit':''}`,
        data:{...item,...looseLook(n),operation:view.operation,reading,zoomInto:()=>deepInto(n),open:(id,event)=>select(id,event,true),
          member:item?.symbols?.length?{hot:pointed?.part===n.id?pointed.index:-1,chosen:memberChoice?.part===n.id&&view.scope===n.id?memberChoice.index:-1,
            point:index=>pointMember(n.id,index),choose:(index,event)=>chooseMember(n.id,index,event)}:undefined}};
    });
    const edges=drawn.map(route=>{
      return {id:route.id,source:route.from,target:route.to,type:'routed',selectable:false,focusable:false,
        ariaLabel:'',domAttributes:{'aria-hidden':true},
        data:route};
    });
    return <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes}
      nodesDraggable={false} nodesConnectable={false} elementsSelectable={false}
      nodesFocusable={false} edgesFocusable={false} disableKeyboardA11y
      deleteKeyCode={null} selectionKeyCode={null} multiSelectionKeyCode={null} panActivationKeyCode={null} zoomActivationKeyCode={null}
      zoomOnDoubleClick={false} minZoom={minZoom()} maxZoom={maxZoom} panOnScroll preventScrolling
      onInit={flow=>{instance=flow;fitOverview().then(()=>requestAnimationFrame(()=>requestAnimationFrame(()=>{
        initializing=false;
        map.classList.remove('flow-initializing');host.inert=false;status.remove();
        updateLocation();
        if(restorePending)restore(restorePending);
        else if(pendingFocus?.path)focusPath(pendingFocus.path);
        else if(pendingFocus?.input)showInput(pendingFocus.id);
        else if(pendingFocus)focus(pendingFocus.id,pendingFocus.center);
        else if(view.scope||view.operation)focus(view.scope||view.operation,true);
        else map.dispatchEvent(new Event('repomap:viewport'));
      })));}}
      onNodeClick={(event,n)=>{event.stopPropagation();memberChoice=null;select(n.id,event,false);}}
      onNodeMouseEnter={(_,n)=>enter(n.id)}
      onMouseMove={event=>{
        if(panning||!instance)return;
        // An arrowhead on empty canvas is its connection's handle.
        const head=event.target.classList?.contains('react-flow__pane')?headAt(event.clientX,event.clientY):null;
        pointHead(head,event);
        if(look.move(event.clientX,event.clientY,performance.now(),!!onHead||!!event.target.closest?.('.flow-connection-label,.flow-floating-card')))update?.();
        pump();
        if(!hover.move(event.clientX,event.clientY))return;
        // Labels, arrowheads and the cards they open stay with the frame being read.
        if(onHead||event.target.closest('.flow-connection-label,.flow-floating-card'))return;
        // A frame's title looks at the whole frame, as its empty space does.
        const frameTitle=event.target.closest('[data-frame-title]');
        if(frameTitle){enter(frameTitle.dataset.frameTitle);return;}
        const input=event.target.closest('[data-input-id]');
        if(input){enter(input.dataset.inputId);return;}
        const node=event.target.closest('.react-flow__node');
        if(node){enter(node.dataset.id);return;}
        const point=instance.screenToFlowPosition({x:event.clientX,y:event.clientY});
        const area=drawing.nodes.filter(n=>n.frame&&!n.display&&visible(n.id)&&point.x>=n.absolute.x&&point.x<=n.absolute.x+n.width&&point.y>=n.absolute.y&&point.y<=n.absolute.y+n.height)
          .sort((a,b)=>a.width*a.height-b.width*b.height)[0];
        if(area)enter(area.id);
      }}
      onPaneClick={event=>{
        // A click on an arrowhead reads its connection, as its chip does.
        const head=headAt(event.clientX,event.clientY),label=head&&headConnection(head);
        if(label){keepHeadLabel(label);openEnd(label,event);return;}
        // A click on empty canvas first closes an open card; the next one
        // goes where it points.
        if(closeCards())return;
        if(instance){
          const p=instance.screenToFlowPosition({x:event.clientX,y:event.clientY});
          const frame=drawing.nodes.find(n=>n.frame&&!n.display&&visible(n.id)&&byID.get(n.id)?.branch!=='inputs-part'&&
            (byID.get(n.id)?.branch==='component'?!openComponents.has(n.id):!communicationsOpen.has(n.id))&&
            p.x>=n.absolute.x&&p.x<=n.absolute.x+n.width&&p.y>=n.absolute.y&&p.y<=n.absolute.y+n.height);
          if(frame){select(frame.id,event,true);return;}
        }
        clearHover();
      }}
      onMove={(_,viewport)=>{
        // A pan moves the viewport without rebuilding the drawing.
        if(paintedZoom===viewport.zoom)return;
        paintedZoom=viewport.zoom;
        zoom=viewport.zoom;
        host.style.setProperty('--flow-zoom',String(zoom));
        updateDetail(viewport);trackDeepPart(viewport);
        map.querySelectorAll('[data-map-zoom]').forEach(button=>{button.disabled=Number(button.dataset.mapZoom)<1&&viewport.zoom<=minZoom();});
      }}
      onMoveStart={event=>{if(event){overviewFit=false;locationSubject='';arriving=new Set();}panning=true;hover.pause();preview='';map.clearMapPreview?.();}}
      onMoveEnd={event=>{panning=false;hover.pause();if(instance){updateDetail(instance.getViewport());trackDeepPart(instance.getViewport());}updateLocation(event);map.dispatchEvent(new Event('repomap:viewport'));}}>
      <svg className="flow-defs"><defs>
        <marker id="flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth={arrowHead} markerHeight={arrowHead} orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b"/></marker>
        <marker id="flow-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth={emphasisedHead} markerHeight={emphasisedHead} orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#34445b"/></marker>
      </defs></svg>
      <ViewportPortal>
        {drawing.nodes.filter(n=>n.frame&&!n.display&&visible(n.id)&&!['component','communication','inputs'].includes(byID.get(n.id).branch)&&(componentsOpen||scales.has(n.id)||byID.get(n.id).branch==='inputs-part')&&(!scales.has(n.id)||detailed.has(n.id))&&!closedGroup(n.id)).map(n=><FrameTitle key={n.id} node={n} item={byID.get(n.id)} focused={state.focus.has(n.id)||context.has(n.id)} enter={enter} select={select} muted={muted(n.id)}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&['component','communication','inputs'].includes(byID.get(n.id).branch)).map(n=><ComponentPresentation key={'component-'+n.id} node={n} focused={state.focus.has(n.id)||context.has(n.id)} muted={muted(n.id)}/>)}
        {drawing.nodes.filter(n=>n.display&&byID.get(n.id)?.headingAt).map(n=><GroupHeading key={'group-'+n.id} node={n} muted={muted(n.id)}/>)}
        {drawing.nodes.filter(n=>(scales.has(n.id)&&!detailed.has(n.id)||closedGroup(n.id))&&visible(n.id)).map(n=><AreaSummary key={'summary-'+n.id}
          node={n} item={byID.get(n.id)} heading={groupHeadings.get(n.id)} enter={enter} select={select} muted={muted(n.id)}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&scales.has(n.id)&&!detailed.has(n.id)).map(n=><ZoomMark key={'zoom-'+n.id} node={n} item={byID.get(n.id)} compactScale={groupHeadings.get(n.id).scale} enter={enter} select={select} muted={muted(n.id)}/>)}
        {drawing.nodes.filter(n=>closedGroup(n.id)&&visible(n.id)).map(n=><ZoomMark key={'zoom-'+n.id} node={n} item={byID.get(n.id)} compactScale={groupHeadings.get(n.id).scale} enter={enter} muted={muted(n.id)}
          select={(id,event)=>{hover.remember(event.clientX,event.clientY);focus(id);}}/>)}
        {labelsShown&&labels.filter(label=>!label.hidden).map(label=><ConnectionLabel key={label.id} label={label}/>)}
        {cardLabels.filter(label=>look.key===`label:${label.id}`||pinnedLabels.has(label.id)).map(label=><LabelCard key={'card:'+label.id} label={label} frame={placed.get(label.root)} labels={cardLabels}/>)}
      </ViewportPortal>
    </ReactFlow>;
  }
  // What an arrow end stands for: every call behind it, grouped by the part
  // it is made from, then the part it goes into (call-card.mjs).
  const nameOf=id=>byID.get(id)?.branch==='inputs'?t('Inputs'):byID.get(id)?.name||byID.get(id)?.title||'';
  const partsOf=id=>leaves(id).filter(leaf=>!byID.get(leaf)?.activation).length;
  const labelCard=label=>callCard(label.relations,{nameOf,incoming:label.incoming,groupable:id=>!byID.get(id)?.activation});
  // The same frames' calls the other way, when the map has them.
  const reverseOf=(label,labels)=>labels.find(other=>other.area===label.area&&other.outside===label.outside&&other.incoming!==label.incoming);
  // The wheel scrolls a card only when the card has something to scroll.
  const wheel=el=>{if(el)el.classList.toggle('nowheel',el.scrollHeight>el.clientHeight+1);};
  const going=label=>event=>{event.stopPropagation();closeCards();clearHover();select(label.outside,event,true);};
  const screenBox=node=>{
    if(!node||!instance)return null;
    const v=instance.getViewport(),box=host.getBoundingClientRect();
    return {left:box.left+v.x+node.absolute.x*v.zoom,top:box.top+v.y+node.absolute.y*v.zoom,
      right:box.left+v.x+(node.absolute.x+node.width)*v.zoom,bottom:box.top+v.y+(node.absolute.y+node.height)*v.zoom};
  };
  // A floating card stands in the map's coordinates and moves with it, at
  // the screen's own type size: drawn at the parts' scale, a card of 291
  // calls read at 10px beside an area's parts. It is placed once it has its
  // size, and again when the zoom changes: flush with its handle, outside
  // the frame being read, on the side with room and inside the canvas
  // (card-place.mjs). A click on it keeps it open; kept open, it has a ✕.
  function FloatingCard({cardKey,handle,frame,side,content,className='',head,children}){
    const ref=useRef(null),body=useRef(null),[at,setAt]=useState(null),pinned=pinnedLabels.has(cardKey.slice(6));
    const zoom=useStore(state=>state.transform[2]);
    // Beside its frame a card takes the room there is, down to 300px, so it
    // stands outside the frame it explains; with less it keeps its 500px.
    const canvas=instance?host.getBoundingClientRect():null,outer=screenBox(frame);
    const room=canvas&&outer&&side!=='top'&&side!=='bottom'?Math.max(outer.left-canvas.left,canvas.right-outer.right)-16:Infinity;
    const width=room>=300&&room<500?Math.floor(room):500;
    useEffect(()=>{shownCards.add(cardKey);return()=>shownCards.delete(cardKey);},[cardKey]);
    useLayoutEffect(()=>{
      wheel(body.current);
      const el=ref.current,from=handle();if(!el||!instance||!from)return;
      const v=instance.getViewport(),box=host.getBoundingClientRect(),size=el.getBoundingClientRect();
      const place=placeCard({handle:from,frame:screenBox(frame),canvas:box,size:{width:size.width,height:size.height},side});
      setAt({x:(place.x-box.left-v.x)/v.zoom,y:(place.y-box.top-v.y)/v.zoom});
    },[cardKey,pinned,content,zoom,width]);
    return <div ref={ref} data-card={cardKey} className={`flow-calls-place flow-floating-card nopan ${className}`}
      onMouseEnter={()=>lookAt(cardKey)} onMouseLeave={()=>lookAway(cardKey)}
      onClick={event=>{event.stopPropagation();if(!pinned)pin(cardKey);}}
      style={{transform:`translate(${at?.x||0}px,${at?.y||0}px) scale(${1/zoom})`,transformOrigin:'top left',visibility:at?'visible':'hidden'}}>
      <div className={`flow-connection-calls ${pinned?'flow-card-pinned':''}`} style={{width,maxHeight:Math.max(160,Math.min(600,host.clientHeight-16))}}>
        {pinned&&<button type="button" className="flow-card-close" aria-label={t('Close')} title={t('Close')}
          onClick={event=>{event.stopPropagation();closeCard(cardKey);}}>✕</button>}
        {head}
        <div ref={body} className="flow-card-body">{children}</div>
      </div></div>;
  }
  // What a label stands for: every call behind it. Its heading names the two frames the arrow joins, says how
  // many calls from how many of their parts, and leads to the calls the
  // other way; kept open, an index of the parts at each end stands on top.
  function LabelCard({label,frame,labels}){
    const key=`label:${label.id}`,pinned=pinnedLabels.has(label.id);
    const card=labelCard(label);
    if(!card.total)return null;
    const inside=byID.get(label.area),outside=byID.get(label.outside);
    const reverse=reverseOf(label,labels)||frameLabels(label.area,lastMatchingOf).find(other=>other.outside===label.outside&&other.incoming!==label.incoming);
    const [fromFrame,intoFrame]=label.incoming?[label.outside,label.area]:[label.area,label.outside];
    const back=reverse?labelCard(reverse).total:0;
    const openReverse=event=>{
      event.stopPropagation();keepHeadLabel(reverse);
      // The other way's card stands by the same handle.
      if(lookHandle?.key===key)lookHandle={...lookHandle,key:`label:${reverse.id}`};
      closeCard(key);look.enter(`label:${reverse.id}`);pin(`label:${reverse.id}`);
    };
    const name=id=>id===label.outside?<button type="button" onClick={going(label)}>{nameOf(id)}</button>:<span>{nameOf(id)}</span>;
    const toGroup=id=>event=>{event.stopPropagation();const body=event.currentTarget.closest('.flow-connection-calls')?.querySelector('.flow-card-body'),group=body?.querySelector(`[data-call-group="${CSS.escape(id)}"]`);if(body&&group)body.scrollTop=group.offsetTop-body.offsetTop;};
    const head=<header className="flow-card-head">
      <div className="flow-card-title">{name(fromFrame)}<i>→</i>{name(intoFrame)}</div>
      <p className="flow-card-count">{cardCount(card,partsOf(fromFrame),partsOf(intoFrame))}
        {back>0&&<> · <button type="button" onClick={openReverse}>{t('{0} go the other way',back)}</button></>}</p>
    </header>;
    // It stands by the handle the pointer rested on, its chip or its arrowhead.
    const via=lookHandle?.key===key?lookHandle:null,chip=()=>host.querySelector(`[data-connection-label="${CSS.escape(label.id)}"]`)?.getBoundingClientRect();
    return <FloatingCard cardKey={key} side={via?.side||label.side} frame={via?.box||frame} content={String(pinned)} className="flow-arrow-card" head={head}
      handle={()=>via?.rect()||chip()||(label.point&&headRect(label.point))}>
      {pinned&&card.from.length+card.into.length>2&&<div className="flow-card-index">
        <ul>{card.from.map(part=><li key={part.id}><button type="button" onClick={toGroup(part.id)}>{part.name}</button><b>{part.count}</b></li>)}</ul>
        <i>→</i>
        <ul>{card.into.map(part=><li key={part.id}><span>{part.name}</span><b>{part.count}</b></li>)}</ul>
      </div>}
      <CallRows card={card}/></FloatingCard>;
  }
  // A click on an arrow end, its chip or its arrowhead, reads its frame's
  // connections in the column, that connection open (owner's 3b); its card
  // is kept by a click on the card itself.
  function openEnd(label,event){
    const key=`label:${label.id}`;
    if(callbacks.openConnection){closeCards();hover.remember(event.clientX,event.clientY);hover.pause();callbacks.openConnection(label.area,label.key);return;}
    if(pinnedLabels.has(label.id))closeCard(key);else{look.enter(key);pin(key);}
  }
  // An arrow end's plaque sits on the frame's border where its arrow meets
  // it, at the one size of the former one-digit chip whatever the zoom, and
  // steps out only as far as it must to cover nothing the frame holds
  // (owner, 2026-09-28: scaled with the frame, they had grown into large
  // grey pills over the parts). It opens its card once the pointer rests on
  // it, and is dark while its card, or its twin's, is open or kept; a click
  // reads the connection.
  function ConnectionLabel({label}){
    const {zoom}=useViewport();
    const key=`label:${label.id}`,ends=[label.id,label.twin];
    const p=plaqueCentre(label.point,label.side,label.obstacles||[],{x:plaqueSize.width/2/zoom,y:plaqueSize.height/2/zoom});
    const style={transform:`translate(${p.x}px,${p.y}px) scale(${1/zoom}) translate(-50%,-50%)`,transformOrigin:'top left'};
    return <div className="flow-connection-label flow-boundary-label nopan" style={style} data-connection-outside={label.outside} data-connection-label={label.id}
      onMouseEnter={event=>{const chip=event.currentTarget;aimAt(key,{rect:()=>chip.isConnected?chip.getBoundingClientRect():null,box:placed.get(label.root),side:label.side});}}
      onMouseLeave={event=>leaveHandle(key,event)}>
      <button type="button" aria-label={label.title} className={ends.some(id=>look.key===`label:${id}`||pinnedLabels.has(id))?'flow-label-pinned':''}
        onClick={event=>{event.stopPropagation();openEnd(label,event);}}>{label.all?t('all'):''}</button>
    </div>;
  }
  map.classList.add('flow-enabled');source.style.display='none';source.setAttribute('aria-hidden','true');
  const root=createRoot(host);flushSync(()=>root.render(<App/>));
  map.addEventListener('pointerleave',clearHover);
  // Restoring the map's pinned emphasis must not remove connection evidence
  // while the reader moves into the adjacent column to use its source links.
  stage.addEventListener('pointerleave',event=>{leaveHead(event);hoverArea='';update?.();});
  stage.closest('.map-workspace')?.addEventListener('pointerleave',clearHover);
  window.addEventListener('blur',clearHover);
  document.addEventListener('visibilitychange',()=>{if(document.hidden)clearHover();});
  map.querySelector('[data-map-controls]').hidden=false;
  const overviewButton=map.querySelector('[data-map-fit]');overviewButton.textContent=t('Show whole map');overviewButton.removeAttribute('title');
  map.querySelector('[data-map-controls]').addEventListener('click',event=>{
    const button=event.target.closest('button');if(!button||!instance)return;
    // The whole map keeps what is being read: the reading, its emphasis and
    // the input path stay; only the camera goes back. Dropping the reading
    // had sent a reader who zoomed out to look around back to the start.
    if(button.hasAttribute('data-map-fit')){closeCards();hover.pause();fitOverview(420);}
    else if(button.hasAttribute('data-map-zoom')&&Number(button.dataset.mapZoom)<1)stepOut();
    else if(button.hasAttribute('data-map-zoom')){overviewFit=false;const before=lookedAt();commitCamera(instance.zoomTo(instance.getZoom()*Number(button.dataset.mapZoom))).then(()=>follow(before));}
  });
  // One pinch (the wheel with ctrl held, a trackpad's pinch) crosses at most
  // one level boundary (semantic.mjs, pinchZoom), and a pause ends it. A
  // pinch of eight ctrl+wheel ticks had carried Redis's readers from the
  // whole map past the areas into a part's tiles. The zoom a tick asks for
  // is React Flow's own (on a Mac a ctrl+wheel deltaY of 50 halves or
  // doubles it); only a tick that would cross a second boundary is held at
  // the last zoom short of it.
  const gesturePause=300;
  let gesture=null,followTimer;
  host.addEventListener('wheel',event=>{
    if(!event.ctrlKey||!instance||initializing||event.target.closest?.('.nowheel'))return;
    const now=performance.now(),v=instance.getViewport();
    if(!gesture||now-gesture.at>gesturePause){
      const box=host.getBoundingClientRect(),aim={x:(event.clientX-box.left-v.x)/v.zoom,y:(event.clientY-box.top-v.y)/v.zoom};
      gesture={level:levelAt(v,aim),aim,before:lookedAt()};
    }
    gesture.at=now;
    // Once the pinch ends, the column reads the frame it brought.
    const pinch=gesture;clearTimeout(followTimer);followTimer=setTimeout(()=>{if(gesture===pinch)follow(pinch.before);},gesturePause+60);
    const factor=navigator.userAgent.indexOf('Mac')>=0?10:1;
    const asked=v.zoom*Math.pow(2,-event.deltaY*(event.deltaMode===1?.05:event.deltaMode?1:.002)*factor);
    const to=Math.min(maxZoom,Math.max(minZoom(),asked));
    if(to===v.zoom)return;
    const box=host.getBoundingClientRect(),aim={x:event.clientX-box.left,y:event.clientY-box.top};
    const at=zoom=>({x:aim.x-(aim.x-v.x)*zoom/v.zoom,y:aim.y-(aim.y-v.y)*zoom/v.zoom,zoom});
    const zoom=pinchZoom(v.zoom,to,z=>levelAt(at(z),gesture.aim),gesture);
    if(zoom===to)return;
    event.preventDefault();event.stopPropagation();
    if(Math.abs(zoom/v.zoom-1)<1e-9)return;
    overviewFit=false;locationSubject='';arriving=new Set();
    commitCamera(instance.setViewport(at(zoom))).then(()=>updateLocation(event));
  },{capture:true,passive:false});
  // The location row stands on the canvas, over the map: a wheel over it
  // moves the map as it does a pixel lower. It had scrolled the page there.
  location.addEventListener('wheel',event=>{
    const pane=host.querySelector('.react-flow__pane');if(!pane)return;
    event.preventDefault();
    pane.dispatchEvent(new WheelEvent('wheel',{deltaX:event.deltaX,deltaY:event.deltaY,deltaZ:event.deltaZ,deltaMode:event.deltaMode,
      clientX:event.clientX,clientY:event.clientY,screenX:event.screenX,screenY:event.screenY,
      ctrlKey:event.ctrlKey,shiftKey:event.shiftKey,altKey:event.altKey,metaKey:event.metaKey,bubbles:true,cancelable:true}));
  },{passive:false});
  // A frame's connections as its arrow ends group them: by the frame or
  // participant at the other end and the direction, incoming first, each
  // with the calls its card lists.
  function frameConnections(id){
    if(!byID.get(id)||!placed.has(id))return [];
    return connections(id,frameMembers(id),layout.edges,outsideOf(id))
      .map(group=>({...group,title:nameOf(group.outside),card:callCard(group.relations,{nameOf,incoming:group.incoming,groupable:other=>!byID.get(other)?.activation})}))
      .filter(group=>group.card.total).sort((a,b)=>(a.incoming?0:1)-(b.incoming?0:1)||b.card.total-a.card.total||a.title.localeCompare(b.title));
  }
  // The reading column's Connections of a frame, drawn by the same rows as
  // the cards into a container the column owns; `open` names the one to open.
  const mounted=new Map();
  // `choose(part,key)` reads a declaration in the report: a name in the
  // column's rows is chosen when the part it names lists that declaration
  // among its own tiles.
  function mountConnections(container,id,open='',choose=null){
    const groups=frameConnections(id);
    if(!groups.length)return false;
    for(const [element,root] of mounted)if(!element.isConnected){root.unmount();mounted.delete(element);}
    const root=mounted.get(container)||createRoot(container);mounted.set(container,root);
    const chooser=choose&&{go:choose,can:(part,key)=>!!key&&(byID.get(part)?.symbols||[]).some(symbol=>symbol.href===key||symbol.open===key)};
    flushSync(()=>root.render(<FrameConnections groups={groups} open={open} choose={chooser} single={!placed.get(id)?.frame}/>));
    return true;
  }
  // Going up from a declaration to its part (the toolbar's breadcrumb): no
  // tile stays chosen, and the part is entered, not its tile.
  const clearMember=()=>{if(memberChoice){memberChoice=null;update?.();}};
  return {get layout(){return layout;},focus,showInput,capture,restore,clearHover,clearMember,mountConnections,frameConnections,light,overview:()=>fitOverview(420),update(next){
    if(memberChoice&&(next.scope||'')!==memberChoice.part)memberChoice=null;
    if(view.scope!==next.scope||view.operation!==next.operation){hover.pause();preview='';map.clearMapPreview?.();}
    view={...initial,...next,scope:next.scope||'',
      selected:new Set(next.selected||[]),matched:new Set(next.matched||[])};update();
  }};
};
