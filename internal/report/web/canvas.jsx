import React, {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow, Handle, Position, ViewportPortal, useViewport, useStore} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {symbolBlocks,symbolRow} from './symbols.mjs';
import {emphasis, focusAncestors} from './emphasis.mjs';
import {createSemanticLayout, detailLayers, firstDetailZoom, componentTextSizes, componentViewport, communicationViewport, closedContainer, readableFocus, frameInventory, systemViewport} from './semantic.mjs';
import {routeDrawing} from './route-drawing.mjs';
import {singlePartAreas} from './overview.mjs';
import {prepareCards,wrapText,overviewHeading,groupHeading} from './cards.mjs';
import {HoverGate} from './hover.mjs';
import {InputTypes, scrollInventory} from './card-content.jsx';
import '@xyflow/react/dist/style.css';
import './canvas.css';

// Legacy small repository diagrams share the same embedded ELK instance code.
window.ELK = ELK;
const t = (...args) => window.rmT(...args);
// Zoomed far into a part, its declarations stand inside it as tiles: the
// keys the model chose first, every tile a link into the code. The tiles are
// drawn in the card's own rectangle at a quarter of its size, so nothing on
// the map moves when they appear.
const deepDivisor=4,deepHeader=20,deepTile={width:190,height:28,columnGap:36,rowGap:8,inset:8};
function PartSymbols({symbols,calls,width,height}){
  const [hot,setHot]=useState(-1);
  const inner={width:width*deepDivisor,height:(height-deepHeader)*deepDivisor},inset=deepTile.inset,gap=deepTile.columnGap;
  const links=calls||[],usable=inner.height-2*inset;
  const columnsAt=w=>Math.max(1,Math.floor((inner.width-2*inset+gap)/(w+gap)));
  // The widest tile that leaves nothing out, with columns enough for a
  // caller, what it calls and what that returns.
  const fits=w=>symbolBlocks(symbols,links,columnsAt(w),usable);
  const tileWidth=[340,260,deepTile.width].find(w=>fits(w).hidden===0&&(!links.length||columnsAt(w)>=3))||deepTile.width;
  const {blocks,rows,hidden}=fits(tileWidth);
  const x=column=>inset+column*(tileWidth+gap);
  // A method inside its type's tile needs no line to that type.
  const drawn=links.filter(([from,to])=>rows[from]&&rows[to]&&rows[from].block!==rows[to].block);
  const near=new Set(hot<0?[]:drawn.filter(([from,to])=>from===hot||to===hot).flat().filter(value=>typeof value==='number'));
  const tone=i=>hot>=0&&i!==hot&&!near.has(i)?'flow-symbol-dim':'';
  // A row reads as a line of a class box: the visibility sign, the name and,
  // in lighter type, what follows it — "(args): Result" or ": Type".
  const row=(i,className,first)=>{
    const symbol=symbols[i],kind=symbol.kind==='field'||symbol.kind==='more'?'flow-symbol-field':'';
    const props={className:`${className} ${kind} ${first?'flow-symbol-first-method':''} ${symbol.key?'flow-symbol-key':''} ${symbol.inner?'flow-symbol-inner':''} ${tone(i)}`,onMouseEnter:()=>setHot(i)};
    const body=<>{symbol.name}{symbol.text&&<em>{symbol.text}</em>}</>;
    return symbol.href?<a key={i} href={symbol.href} target="_blank" rel="noopener" onClick={event=>event.stopPropagation()} {...props}>{body}</a>
      :<span key={i} {...props}>{body}</span>;
  };
  return <div className="flow-part-symbols nopan" style={{top:deepHeader,width,height:height-deepHeader}}>
    <div style={{width:inner.width,height:inner.height,transform:`scale(${1/deepDivisor})`,transformOrigin:'top left'}} onMouseLeave={()=>setHot(-1)}>
      <svg width={inner.width} height={inner.height}>
        <defs><marker id="flow-symbol-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L8 4L0 8z"/></marker></defs>
        {drawn.map(([from,to,kind],i)=>{
          const a=rows[from],b=rows[to],forward=b.column>a.column,ay=inset+a.y+a.height/2,by=inset+b.y+b.height/2;
          // Across columns a link leaves the right edge and enters the left;
          // within a column it bows out to the right of both rows.
          const start={x:x(a.column)+tileWidth,y:ay},end=forward?{x:x(b.column),y:by}:{x:x(b.column)+tileWidth,y:by};
          const bend=forward?Math.max(16,(end.x-start.x)/2):26;
          return <path key={i} className={`flow-symbol-${kind||'calls'} ${hot<0?'':from===hot||to===hot?'flow-symbol-call-hot':'flow-symbol-call-dim'}`}
            d={`M${start.x} ${start.y}C${start.x+bend} ${start.y},${forward?end.x-bend:end.x+bend} ${end.y},${end.x} ${end.y}`} markerEnd="url(#flow-symbol-arrow)">
            <title>{`${symbols[from].name} ${kind||'calls'} ${symbols[to].name}`}</title></path>;
        })}
      </svg>
      {blocks.map(block=><div key={block.head} className={`flow-symbol-block ${block.rows.length?'flow-symbol-type':''}`}
        style={{left:x(block.column),top:inset+block.y,width:tileWidth,height:block.height}}>
        {row(block.head,'flow-symbol-head')}
        {block.rows.map((i,k)=>row(i,'flow-symbol-row',k>0&&symbols[i].kind!=='field'&&symbols[i].kind!=='more'&&['field','more'].includes(symbols[block.rows[k-1]].kind)))}
      </div>)}
      {hidden>0&&<span className="flow-symbol-more" style={{right:inset,bottom:inset}}>+{hidden}</span>}
    </div>
  </div>;
}
function Part({data}) {
  const heading=data.standaloneHeading,scale=heading?.scale||data.contentScale;
  const box=heading?{width:heading.width,height:heading.height}:{width:data.originalWidth||data.width||260,height:data.originalHeight||data.height||88};
  // Only the flip between the two drawings re-renders the card, not every
  // step of a zoom.
  const far=useStore(state=>box.width*(scale||1)*state.transform[2]>=860);
  const deep=far&&!data.activation&&data.symbols?.length>0;
  if(deep)return <div className={`flow-part flow-part-deep flow-${data.category} ${data.lane==='core'?'flow-core':data.lane==='triggers'?'flow-entry':''}`}
      style={{width:box.width,height:box.height,transform:`scale(${scale||1})`,transformOrigin:'top left'}}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <strong>{data.name||data.title}</strong>
    <PartSymbols symbols={data.symbols} calls={data.symbolCalls} width={box.width} height={box.height}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
  return <div className={`flow-part flow-${data.category} ${data.category==='input'?'':data.lane==='core'?'flow-core':data.lane==='triggers'?'flow-entry':''} ${heading?'flow-standalone-part':''}`} data-input-id={data.activation?data.id:undefined} style={heading?{width:heading.width,height:heading.height,transform:`scale(${scale})`,transformOrigin:'top left'}:scale&&scale!==1?{width:data.originalWidth,height:data.originalHeight,transform:`scale(${scale})`,transformOrigin:'top left'}:undefined}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    {data.roleLabel&&<span className={`flow-role-symbol flow-role-${data.lane}`} role="img" aria-label={data.roleLabel}/> }
    {data.kindLabel&&!heading&&<div className="flow-kind" data-input-kind={data.activation||undefined}>{data.kindLabel}</div>}
    <strong data-input-name={data.activation?'':undefined}>{heading?.title||data.title}</strong>
    {data.description&&<div className="flow-description">{data.description}</div>}
    {data.subtitle&&<div className="flow-address">{data.subtitle}</div>}
    {data.symbols?.length>0&&!data.activation&&<button type="button" className="flow-part-zoom nopan" aria-label={t('Zoom into {0}',data.name||data.title)}
      onClick={event=>{event.stopPropagation();data.zoomInto?.();}}>
      <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <rect x="1.5" y="11" width="10.5" height="10.5" rx="1.4"/><circle cx="14" cy="9" r="7" fill="#fff" fillOpacity=".92"/><path d="M19.2 14.2l3.2 3.2" strokeWidth="2.8"/>
      <path d="M9.5 10h5v4.5M12 8.2V4.6m-1.4 1.4L12 4.6 13.4 6M16.4 12.2h3m-1.4-1.4 1.4 1.4-1.4 1.4" strokeWidth="1.1"/></svg></button>}
    {data.number && <span data-badge={data.id} className={`flow-number nopan ${data.badge?.pinned?'flow-number-pinned':''}`} onMouseEnter={event=>data.badge?.enter(event)} onMouseLeave={()=>data.badge?.leave()}
      onClick={event=>{event.stopPropagation();data.badge?.toggle();}}>{data.number}</span>}
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function Area({data}) {
  return <div className={`flow-area ${data.branch==='component'?'flow-component':data.branch==='communication'?'flow-communication':data.branch==='inputs'?'flow-input-collection':data.branch!=='area'?'':data.lane==='core'?'flow-area-core':data.lane==='triggers'?'flow-area-entry':''}`}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function AreaSummary({node,item,number,badge,heading,enter,select}){
  const {scale,title}=heading;
  return <div className="flow-area-summary nopan" data-summary-area={node.id}
    style={{transform:`translate(${node.absolute.x}px,${node.absolute.y}px) scale(${scale})`,transformOrigin:'top left',
      width:node.width/scale,height:node.height/scale}}
    onMouseEnter={()=>enter(node.id)} onClick={event=>{event.stopPropagation();select(node.id,event,false);}}>
    <div className="flow-part flow-overview-card flow-overview-compact"><strong>{title}</strong>
      {['core','triggers'].includes(item?.lane)&&<span className={`flow-role-symbol flow-role-${item.lane}`} role="img" aria-label={item.roleLabel||item.lane}/>}
      {number&&<span data-badge={node.id} className={`flow-number ${badge?.pinned?'flow-number-pinned':''}`} onMouseEnter={event=>badge?.enter(event)} onMouseLeave={()=>badge?.leave()}
        onClick={event=>{event.stopPropagation();badge?.toggle();}}>{number}</span>}</div>
  </div>;
}
function ZoomMark({node,item,enter,select,compactScale}) {
  const viewport=useViewport(),{zoom}=viewport;
  const area=item.branch==='area',size=24,inset=area||['communication','inputs'].includes(item.branch)?8:12;
  const scale=area?compactScale:1/zoom;
  if(node.width<(size+2*inset)*scale||node.height<(size+2*inset)*scale)return null;
  const point={x:node.absolute.x+node.width-(size+inset)*scale,y:node.absolute.y+inset*scale};
  const name=item.branch==='inputs'?`${t('Inputs')} · ${item.name||item.title}`:item.name||item.title;
  return <button type="button" className="flow-zoom-mark nopan" data-zoom-into={node.id}
    style={{transform:`translate(${point.x}px,${point.y}px) scale(${scale})`,width:size,height:size}}
    aria-label={t('Zoom into {0}',name)} onMouseEnter={()=>enter(node.id)}
    onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    <svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
      <rect x="1.5" y="11" width="10.5" height="10.5" rx="1.4"/><circle cx="14" cy="9" r="7" fill="#fff" fillOpacity=".92"/><path d="M19.2 14.2l3.2 3.2" strokeWidth="2.8"/>
      <path d="M9.5 10h5v4.5M12 8.2V4.6m-1.4 1.4L12 4.6 13.4 6M16.4 12.2h3m-1.4-1.4 1.4 1.4-1.4 1.4" strokeWidth="1.1"/>
    </svg>
  </button>;
}
function FrameTitle({node,item,focused,enter,select}) {
  const viewport=useViewport();
  const scale=item.summaryScale||1;
  const component=item.branch==='component',communication=item.branch==='communication',inputs=item.branch==='inputs';
  // The location row keeps the parent name available while its reserved world
  // header is too small for a screen-sized title above the revealed children.
  if(component&&64*scale*viewport.zoom<24)return null;
  const x=component||communication||inputs?Math.max(node.absolute.x+18*scale,Math.min(node.absolute.x+node.width-260*scale,(24-viewport.x)/viewport.zoom)):node.absolute.x+18*scale;
  return <div className={`flow-area-title nopan ${focused?'flow-area-title-focus':''} ${component?'flow-component-title':communication?'flow-communication-title':inputs?'flow-input-collection':item.lane==='core'?'flow-core-title':item.lane==='triggers'?'flow-entry-title':''}`}
    data-frame-title={node.id}
    style={{transform:`translate(${x}px,${node.absolute.y+12*scale}px) scale(${scale})`,transformOrigin:'top left',maxWidth:node.width/scale-36,
      '--flow-zoom':viewport.zoom*scale,
      '--flow-secondary-text':viewport.zoom*scale*13>=12?'visible':'hidden',
      '--flow-small-text':viewport.zoom*scale*12>=12?'visible':'hidden'}}
    onMouseEnter={()=>enter(node.id)} onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    <strong>{item.title}</strong>
    {item.metadata&&<div className="flow-component-meta">{item.metadata}</div>}
    {item.role&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
    {item.description&&<p className="flow-description">{item.description}</p>}
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
  const display=singlePartAreas(records,areas),displayed=id=>display.aliases.get(id)||id;
  records=display.records;areas=display.areas;
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
  const initial={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false,numbered:true};
  let cameraRevision=0;
  let update, instance, view=initial, hoverArea='', cardPart='', pinnedPart='', pinnedLabels=new Map(), revealing={id:'',until:0,zoom:0}, preview='', restorePending, pendingFocus, panning=false, initializing=true;
  let detailed=new Set(),openComponents=new Set(),communicationsOpen=new Set(),locationID='',locationSubject='',componentsOpen=false,zoom=systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom,paintedZoom,overviewFit=false;
  const closed=id=>closedContainer(id,placed,byID,detailed,componentsOpen,communicationsOpen,openComponents);
  new ResizeObserver(()=>{if(instance&&!initializing&&overviewFit)fitOverview();}).observe(host);
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
      return {id:n.id,visible,distance:dx*dx+dy*dy};
    }).filter(n=>n.visible>0).sort((a,b)=>a.distance-b.distance||b.visible-a.visible);
    if(candidates.length)locationID=candidates.find(n=>n.id===subject)?.id||candidates[0].id;
    const names=[];for(let id=locationID;id;id=placed.get(id)?.parentId)names.unshift(byID.get(id)?.name||byID.get(id)?.title);
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
  const communicationScales=()=>new Map(areas.filter(a=>['communication','inputs'].includes(byID.get(a.id)?.branch))
    .map(a=>[a.id,Math.min(1,...leaves(a.id).map(id=>byID.get(id)?.contentScale||1))]));
  const maximumZoom=()=>Math.max(2,...[...scales.values(),...communicationScales().values(),
    ...[...byID.values()].filter(n=>!n.children?.length).map(n=>n.contentScale||1)].map(scale=>1.8/scale),
    // Far enough into a part to read its declarations, and no farther.
    ...[...byID.values()].filter(n=>n.symbols?.length).map(n=>4.5/(n.contentScale||1)));
  maxZoom=maximumZoom();
  function detailState(viewport,previous){
    const open=detailLayers(layout.nodes,semantic.records,viewport,host.clientWidth,host.clientHeight,previous);
    return {
      components:new Set([...open].filter(id=>byID.get(id)?.branch==='component')),
      areas:new Set([...open].filter(id=>byID.get(id)?.branch==='area')),
      communications:new Set([...open].filter(id=>['communication','inputs'].includes(byID.get(id)?.branch))),
    };
  }
  function updateDetail(viewport){
    const next=detailState(viewport,new Set([...openComponents,...detailed,...communicationsOpen]));
    const same=(a,b)=>a.size===b.size&&[...a].every(id=>b.has(id));
    if(same(next.components,openComponents)&&same(next.areas,detailed)&&same(next.communications,communicationsOpen))return;
    openComponents=next.components;componentsOpen=!!openComponents.size;detailed=next.areas;communicationsOpen=next.communications;
    hoverArea='';hover.pause();update?.();
  }
  function parentArea(id){while(id){if(byID.get(id)?.branch==='area')return id;id=placed.get(id)?.parentId;}return '';}
  function rootOf(id){while(placed.get(id)?.parentId)id=placed.get(id).parentId;return id;}
  function boundaryBetween(id,other){
    const shared=new Set();
    for(let at=other;at;at=placed.get(at)?.parentId)shared.add(at);
    for(let at=placed.get(id)?.parentId;at;at=placed.get(at)?.parentId)
      if(byID.get(at)?.branch==='area'&&!shared.has(at))return placed.get(at);
    return null;
  }
  function clearHover(){hoverArea='';preview='';map.clearMapPreview?.();update?.();}
  function commitCamera(movement,subject){
    // React Flow's imperative camera methods need not emit onMoveEnd. Save
    // only after they finish, or Back restores the previous display's camera.
    const revision=++cameraRevision;
    return Promise.resolve(movement).then(()=>{if(revision===cameraRevision&&!initializing){updateLocation(undefined,subject);map.dispatchEvent(new Event('repomap:viewport'));}});
  }
  function enter(id){
    if(!hover.allowed)return;
    const n=byID.get(id);if(!n)return;
    // Hover affects the drawing only. The links and description opened by a
    // click stay usable while the pointer crosses other cards to reach them.
    // Pointing anywhere inside a target looks at the target: its frame, its
    // numbers and what it is. Its initialization wiring stays off, as it
    // does for a chosen target: init is drawn for a looked-at end only.
    const area=parentArea(id)||id;
    if(hoverArea!==area){hoverArea=area;update?.();}
  }
  function focus(id,center=true,smooth=true){
    id=displayed(id);
    const n=placed.get(id);if(!n)return;
    if(!instance||initializing){pendingFocus={id,center};return;}
    overviewFit=false;
    hover.pause();preview='';map.clearMapPreview?.();
    const {x,y}=n.absolute, viewport=instance.getViewport(), rect=host.getBoundingClientRect();
    const contentScale=byID.get(n.id)?.contentScale||1;
    if(!center&&readableFocus(n.id,placed,byID,detailed,componentsOpen,viewport,rect.width,rect.height,communicationsOpen,openComponents))return;
    locationSubject=id;
    if(n.frame){
      if(['component','communication','inputs'].includes(byID.get(n.id).branch)){
        const destination=['communication','inputs'].includes(byID.get(n.id).branch)
          ?communicationViewport(n,layout.nodes,rect.width,rect.height,communicationScales().get(n.id)||1)
          :componentViewport(n,rect.width,rect.height,(componentFonts.get(n.id)||20)/20);
        commitCamera(instance.setViewport(destination,{duration:smooth?420:0}),id);return;
      }
      // An area is shown whole: its parts at their own size when the canvas
      // has room for that, smaller when it has not, never below three
      // quarters. It stands in the middle of the free canvas.
      const fit=Math.min((rect.width-48)/n.width,(rect.height-48)/n.height);
      const zoom=scales.has(n.id)?Math.max(.75/contentScale,Math.min(1/contentScale,fit)):Math.min(1,Math.max(.6,fit));
      const left=Math.max(24,(rect.width-n.width*zoom)/2),top=Math.max(24,(rect.height-n.height*zoom)/2);
      commitCamera(instance.setViewport({x:left-x*zoom,y:top-y*zoom,zoom},{duration:smooth?420:0}),id);return;
    }
    const zoom=1/contentScale;
    commitCamera(instance.setCenter(x+n.width/2,y+Math.min(n.height/2,rect.height/(2*zoom)-24),{zoom,duration:smooth?420:0}),id);
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
  function resetView(){if(!instance)return;if(isOverview()){fitOverview();return;}const first=layout.nodes.filter(n=>!n.frame).sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];focus(view.scope||view.operation||first?.id,true);}
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
    hover.remember(event.clientX,event.clientY);hover.pause();hoverArea='';preview='';map.clearMapPreview?.();callbacks.select(id,center);
  }
  // Lay out text once for the whole-map camera. Pan clips that fixed card;
  // zoom scales it with the map instead of rewrapping it at every wheel tick.
  function ComponentOverview({node:n}){
    const zoom=systemViewport(layout.nodes,layoutSize.width,layoutSize.height).zoom;
    const item=byID.get(n.id),inventory=inventories.get(n.id),screenWidth=n.width*zoom,screenHeight=n.height*zoom;
    const visible=screenWidth>32,width=Math.min(320,screenWidth-16),contentWidth=Math.max(1,width-16);
    const heading=useMemo(()=>visible?overviewHeading(item,screenWidth,measure):null,[screenWidth]);
    const text=useMemo(()=>{
      if(!visible)return null;
      const communication=item.branch==='communication',inputs=item.branch==='inputs',areaIDs=communication||inputs?[]:children.get(n.id)||[];
      const textHeight=(text,font,lineHeight)=>wrapText(text,contentWidth,font,measure).length*lineHeight;
      const listHeight=areaIDs.length?7+areaIDs.reduce((h,id)=>h+10+textHeight(byID.get(id).overviewTitle||byID.get(id).name||byID.get(id).title,'500 13px system-ui',18),0):0;
      const counts=inventory.parts?t('{0} parts',inventory.parts):'';
      const roleHeight=item.role?textHeight(item.role,'600 13px system-ui',18)+10:0;
      const countsHeight=textHeight(counts,'500 12px system-ui',17)+10;
      return {communication,inputs,areaIDs,listHeight,counts,roleHeight,countsHeight,inputHeight:inputs?item.overviewHeightAtWidth(screenWidth):0};
    },[visible,contentWidth]);
    if(!heading)return null;
    const {communication,inputs,areaIDs,listHeight,counts,roleHeight,countsHeight}=text,scale=1/zoom;
    let remaining=screenHeight-32-heading.height-listHeight-(areaIDs.length?10:0);
    const listOverflow=remaining<0;
    const showRole=!communication&&!inputs&&roleHeight>0&&remaining>=roleHeight;
    if(showRole)remaining-=roleHeight;
    const showCounts=!communication&&!inputs&&remaining>=countsHeight;
    if(showCounts)remaining-=countsHeight;
    const descriptionLines=Math.floor((remaining-10)/18);
    const x=n.absolute.x+8*scale,y=n.absolute.y+8*scale;
    return <div key={'component-'+n.id} className={`flow-component-overview nopan ${item.branch==='communication'?'flow-communication-overview':item.branch==='inputs'?'flow-input-collection':''}`}
      data-component-overview={n.id} style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width,maxHeight:(n.absolute.y+n.height-y)*zoom-8}}
      onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,true);}}>
      <div className="flow-component-overview-heading" style={{maxWidth:heading.width,minHeight:inputs?32:undefined,paddingTop:heading.clearZoom?32:undefined}}><strong>{item.name}</strong></div>
      {showRole&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
      {!communication&&!inputs&&descriptionLines>=2&&item.description&&<p className="flow-description flow-description-compact" style={{WebkitLineClamp:descriptionLines}}>{item.description.replace(/\n/g,' ')}</p>}
      {inputs&&<InputTypes groups={item.inputGroups}/>}
      {areaIDs.length>0&&<ul className={`flow-component-areas ${listOverflow?'flow-scrollable':''}`} onWheelCapture={scrollInventory}>{areaIDs.map(id=><li key={id}>
        <button type="button" className="nopan" data-overview-area={id} onClick={event=>{event.stopPropagation();select(id,event,true);}}>{byID.get(id).overviewTitle||byID.get(id).name||byID.get(id).title}</button>
      </li>)}</ul>}
      {showCounts&&<div className="flow-inside-counts">{counts}</div>}
    </div>;
  }
  function ComponentPresentation({node,focused}){
    const item=byID.get(node.id),open=item.branch==='component'?openComponents.has(node.id):communicationsOpen.has(node.id);
    return open?<FrameTitle node={node} item={item} focused={focused} enter={enter} select={select}/>:<>
      <ComponentOverview node={node}/><ZoomMark node={node} item={item} enter={enter} select={select}/>
    </>;
  }
  function App(){
    const [,setVersion]=useState(0);update=()=>setVersion(v=>v+1);
    const state=emphasis(view,hoverArea,leaves,layout.edges);
    const context=focusAncestors(state.focus,placed);
    const visible=id=>!closed(id);
    const drawing=layout;
    const groupHeadings=useMemo(()=>{
      const scale=1/firstDetailZoom(layout.nodes,semantic.records,layoutSize.width,layoutSize.height);
      return new Map(layout.nodes.filter(n=>scales.has(n.id)).map(n=>[n.id,groupHeading(n,byID.get(n.id).name||byID.get(n.id).title,scale,measure)]));
    },[layoutKey]);
    const standaloneHeadings=useMemo(()=>{
      const scale=1/firstDetailZoom(layout.nodes,semantic.records,layoutSize.width,layoutSize.height);
      return new Map(layout.nodes.filter(n=>!n.frame&&!byID.get(n.id).activation&&byID.get(n.parentId)?.branch==='component').map(n=>{
        const item=byID.get(n.id),heading=groupHeading(n,item.name||item.title,scale,measure,(item.roleLabel?65:45)+(item.symbols?.length?22:0),15);
        return [n.id,{...heading,width:n.width/heading.scale,height:n.height/heading.scale}];
      }));
    },[layoutKey]);
    const overview=isOverview();
    // Zoomed into one area with nothing hovered or chosen, that area is what
    // the reader is looking at: its parts keep their numbers.
    const zoomedArea=state.mode==='all'&&detailed.size===1?[...detailed][0]:'';
    // Pinned cards belong to the frame their part stands in; that frame stays
    // the one being read while they are pinned.
    const pinnedFrame=[...pinnedLabels.values()].find(id=>byID.get(id)?.branch==='area')||'';
    const chosen=(pinnedPart&&parentArea(pinnedPart))||pinnedFrame||(state.mode==='hover'?parentArea(hoverArea):state.mode==='selection'?parentArea(view.scope):zoomedArea);
    // What is numbered is the frame the reader looks at: an open area with
    // its parts, or else the open component with its areas and loose parts.
    const lookedComponent=chosen?rootOf(chosen):openComponents.size===1?[...openComponents][0]:'';
    const area=chosen&&detailed.has(chosen)?chosen:byID.get(lookedComponent)?.branch==='component'&&openComponents.has(lookedComponent)?lookedComponent:chosen;
    const wholeComponent=byID.get(area)?.branch==='component';
    const childOf=id=>{while(id&&placed.get(id)?.parentId!==area)id=placed.get(id)?.parentId;return id||'';};
    const members=!area?[]:wholeComponent?layout.nodes.filter(n=>!n.frame&&!byID.get(n.id)?.activation&&childOf(n.id)).map(n=>n.id):leaves(area);
    const number=new Map(!area||!view.numbered?[]:wholeComponent?layout.nodes.filter(n=>n.parentId===area).map((n,i)=>[n.id,i+1]):members.map((id,i)=>[id,i+1]));
    const dim=state.mode!=='all';
    const initVisible=(state.mode==='hover'&&byID.get(hoverArea)?.branch!=='component')||state.mode==='operation'||(state.mode==='selection'&&byID.get(view.scope)?.branch!=='component');
    const routes=routeDrawing(drawing.edges,closed,state.activeEdges,dim,boundaryBetween,initVisible,zoomedArea?new Set(leaves(zoomedArea)):null);
    // The labels of a frame do not change with what is hovered inside it;
    // the numbers the hovered thing owns are set bold.
    const numbered=id=>wholeComponent?childOf(id):id;
    // While the camera draws back the badge slides under the pointer and
    // back; that is not the reader leaving it.
    const badge=id=>({pinned:pinnedPart===id,enter:event=>{cardPart=id;update?.();reveal(id,event);},
      leave:()=>{if(cardPart!==id||performance.now()<revealing.until)return;cardPart='';update?.();},
      toggle:()=>{pinnedPart=pinnedPart===id?'':id;update?.();}});
    const inside=new Set(members);
    const labelGroups=area?connections(area,members,layout.edges,
      id=>rootOf(id)!==rootOf(area)?rootOf(id):boundaryBetween(id,area)?.id||id,id=>number.get(numbered(id))):[];
    const labels=labelGroups.flatMap(group=>{
      const matching=routes.filter(route=>route.edgeIDs.some(id=>group.edges.includes(id)));
      const route=group.incoming?matching.at(-1):matching[0];
      const point=group.incoming?route?.end:route?.start;
      if(!point)return [];
      const outside=byID.get(group.outside),root=rootOf(group.outside)!==rootOf(area)?rootOf(area):boundaryBetween(group.insides[0],group.outside)?.id||area;
      // On the component's frame the label is as large as the numbers of
      // the areas and loose parts it names.
      const frameScale=wholeComponent?Math.max(...layout.nodes.filter(n=>n.parentId===area).map(n=>groupHeadings.get(n.id)?.scale||standaloneHeadings.get(n.id)?.scale||0)):0;
      // Which side of its frame the label stands on and at what scale: the
      // frame's own children's scale, a part's or half a closed area's heading.
      const frame=placed.get(root);
      const sides=[['left',Math.abs(point.x-frame.absolute.x)],['right',Math.abs(point.x-frame.absolute.x-frame.width)],
        ['top',Math.abs(point.y-frame.absolute.y)],['bottom',Math.abs(point.y-frame.absolute.y-frame.height)]];
      const side=sides.sort((a,b)=>a[1]-b[1])[0][0];
      const labelScale=frameScale||Math.max(...layout.nodes.filter(n=>n.parentId===root).map(n=>n.frame?(byID.get(n.id)?.summaryScale||1)/2:byID.get(n.id)?.contentScale||1),
        ...group.insides.map(id=>byID.get(id)?.contentScale||1));
      // The card stands outward of the label; this is the far corner it
      // reaches, in the map's coordinates.
      const reach={width:360*labelScale,height:250*labelScale};
      const cardFar={x:point.x+(side==='left'?-reach.width:side==='right'?reach.width:0),y:point.y+(side==='top'?-reach.height:side==='bottom'?reach.height:0)};
      const cardSpan=side==='left'||side==='right'?[{x:cardFar.x,y:point.y-reach.height/2},{x:cardFar.x,y:point.y+reach.height/2}]
        :[{x:point.x-reach.width/2,y:cardFar.y},{x:point.x+reach.width/2,y:cardFar.y}];
      const byNumber=new Map();
      for(const id of group.insides){const k=number.get(numbered(id));if(!byNumber.has(k))byNumber.set(k,{key:numbered(id),ids:[]});byNumber.get(k).ids.push(id);}
      const active=state.mode==='all'?new Set():new Set(layout.edges.filter(e=>group.edges.includes(e.id)&&state.activeEdges.has(e.id))
        .map(e=>number.get(numbered(inside.has(e.from)?e.from:e.to))));
      const bold=active.size<byNumber.size||byNumber.size===1&&state.mode==='hover'&&hoverArea!==area?active:new Set();
      return [{...group,id:`boundary:${area}:${group.key}`,boundary:true,root,point,frameScale,side,labelScale,cardSpan,byNumber,bold,order:Math.min(...group.numbers)*1000+(group.incoming?0:1),title:outside.name||outside.title}];
    });
    // A number's cards stand beside every arrow it has, some of them off the
    // canvas. The camera draws back around the pointer, so the badge stays
    // under it, until all of them are in view, but not so far that the frame
    // being read closes.
    // Far enough into one part to read its declarations.
    function deepInto(node){
      if(!instance)return;
      const rect=host.getBoundingClientRect(),zoom=Math.min(maxZoom,Math.min((rect.width-80)/node.width,(rect.height-80)/node.height));
      instance.setCenter(node.absolute.x+node.width/2,node.absolute.y+node.height/2,{zoom,duration:420});
    }
    function reveal(id,pointer){
      if(!instance||!pointer)return;
      const event={clientX:pointer.clientX,clientY:pointer.clientY};
      // One draw-back for one look at a badge, measured from where it began.
      const now=performance.now();
      if(revealing.id===id&&now<revealing.until+1500)return;
      const shown=labels.filter(label=>[...label.byNumber.values()].some(entry=>entry.key===id));
      const points=shown.flatMap(label=>[label.point,...label.cardSpan]);
      if(!points.length)return;
      const v=instance.getViewport(),rect=host.getBoundingClientRect(),px=event.clientX-rect.left,py=event.clientY-rect.top;
      const fx=(px-v.x)/v.zoom,fy=(py-v.y)/v.zoom,margin={left:24,right:24,top:24,bottom:24};
      let z=v.zoom;
      for(const point of points){
        const dx=point.x-fx,dy=point.y-fy;
        if(dx>0)z=Math.min(z,(rect.width-margin.right-px)/dx);else if(dx<0)z=Math.min(z,(px-margin.left)/-dx);
        if(dy>0)z=Math.min(z,(rect.height-margin.bottom-py)/dy);else if(dy<0)z=Math.min(z,(py-margin.top)/-dy);
      }
      // Never so far that the cards themselves cannot be read: their text is
      // twelve pixels at the label's scale, and below about eight and a half
      // it is no use bringing them into view. What still does not fit is
      // reached by moving the map, as any other element is.
      // The smallest of the cards being shown sets the limit.
      const cardScale=Math.min(...shown.map(label=>label.labelScale).filter(Boolean));
      z=Math.max(z,Number.isFinite(cardScale)?.75/cardScale:v.zoom*.62);
      // A pointer that does not move gets no new events, and the badge may
      // have been redrawn: ask where the badge is now and whether the pointer
      // still stands on it.
      const settle=()=>{
        const at=[...host.querySelectorAll('[data-badge]')].find(el=>el.dataset.badge===id)?.getBoundingClientRect();
        const on=at&&event.clientX>=at.left-2&&event.clientX<=at.right+2&&event.clientY>=at.top-2&&event.clientY<=at.bottom+2;
        if(cardPart===id&&!on){cardPart='';update?.();}
      };
      if(!(z>0)||z>=v.zoom*.98)return;
      const previous=new Set([...openComponents,...detailed,...communicationsOpen]);
      for(let step=0;step<=8;step++){
        const zoom=Math.max(minZoom(),z+(v.zoom-z)*step/8),next={x:px-fx*zoom,y:py-fy*zoom,zoom};
        if(zoom>=v.zoom*.98)return;
        const open=detailState(next,previous);
        if(wholeComponent?open.components.has(area):open.areas.has(area)){revealing={id,until:now+420,zoom:v.zoom};instance.setViewport(next,{duration:320});setTimeout(settle,460);return;}
      }
    }
    useEffect(()=>callbacks.emphasis?.({...state,overview}),[state.mode,state.subject,state.readingOutside,view.scope,overview]);
    const nodes=drawing.nodes.map(n=>{
      const item=byID.get(n.id),focused=state.focus.has(n.id),on=state.participants.has(n.id)||
        (overview&&leaves(n.id).some(id=>state.participants.has(id)));
      const reading=view.scope===n.id||view.operation===n.id;
      const contains=n.frame&&leaves(n.id).some(id=>state.participants.has(id));
      return {...n,type:n.frame?'area':'part',selected:reading,measured:{width:n.width,height:n.height},
        selectable:false,draggable:false,connectable:false,
        style:{width:n.width,height:n.height,visibility:visible(n.id)?'visible':'hidden'},
        className:`${on||contains||!dim?'':'flow-node-muted'} ${focused?'flow-node-focus':on?'flow-node-connected':''} ${context.has(n.id)?'flow-node-context':''} ${reading?'flow-node-reading':''}`,
        data:{...item,standaloneHeading:standaloneHeadings.get(n.id),operation:view.operation,reading,number:number.get(n.id),badge:badge(n.id),zoomInto:()=>deepInto(n),open:(id,event)=>select(id,event,true)}};
    });
    const edges=routes.map(route=>{
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
        else if(pendingFocus)focus(pendingFocus.id,pendingFocus.center);
        else if(view.scope||view.operation)focus(view.scope||view.operation,true);
        else map.dispatchEvent(new Event('repomap:viewport'));
      })));}}
      onNodeClick={(event,n)=>{event.stopPropagation();select(n.id,event,false);}}
      onNodeMouseEnter={(_,n)=>enter(n.id)}
      onMouseMove={event=>{
        if(panning||!instance)return;
        if(!hover.move(event.clientX,event.clientY))return;
        const labelElement=event.target.closest('.flow-connection-label');
        if(labelElement)return;
        // A frame's title looks at the whole frame, as its empty space does.
        const frameTitle=event.target.closest('[data-frame-title]');
        if(frameTitle){enter(frameTitle.dataset.frameTitle);return;}
        const input=event.target.closest('[data-input-id]');
        if(input){enter(input.dataset.inputId);return;}
        const node=event.target.closest('.react-flow__node');
        if(node){enter(node.dataset.id);return;}
        const point=instance.screenToFlowPosition({x:event.clientX,y:event.clientY});
        const area=drawing.nodes.filter(n=>n.frame&&visible(n.id)&&point.x>=n.absolute.x&&point.x<=n.absolute.x+n.width&&point.y>=n.absolute.y&&point.y<=n.absolute.y+n.height)
          .sort((a,b)=>a.width*a.height-b.width*b.height)[0];
        if(area)enter(area.id);
      }}
      onPaneClick={event=>{
        if(instance){
          const p=instance.screenToFlowPosition({x:event.clientX,y:event.clientY});
          const frame=drawing.nodes.find(n=>n.frame&&visible(n.id)&&
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
        updateDetail(viewport);
        map.querySelectorAll('[data-map-zoom]').forEach(button=>{button.disabled=Number(button.dataset.mapZoom)<1&&viewport.zoom<=minZoom();});
      }}
      onMoveStart={event=>{if(event){overviewFit=false;locationSubject='';}panning=true;hover.pause();preview='';map.clearMapPreview?.();}}
      onMoveEnd={event=>{panning=false;hover.pause();if(instance)updateDetail(instance.getViewport());updateLocation(event);map.dispatchEvent(new Event('repomap:viewport'));}}>
      <svg className="flow-defs"><defs>
        <marker id="flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b"/></marker>
        <marker id="flow-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#34445b"/></marker>
      </defs></svg>
      <ViewportPortal>
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&!['component','communication','inputs'].includes(byID.get(n.id).branch)&&(componentsOpen||scales.has(n.id))&&(!scales.has(n.id)||detailed.has(n.id))).map(n=><FrameTitle key={n.id} node={n} item={byID.get(n.id)} focused={state.focus.has(n.id)||context.has(n.id)} enter={enter} select={select}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&['component','communication','inputs'].includes(byID.get(n.id).branch)).map(n=><ComponentPresentation key={'component-'+n.id} node={n} focused={state.focus.has(n.id)||context.has(n.id)}/>)}
        {drawing.nodes.filter(n=>scales.has(n.id)&&visible(n.id)&&!detailed.has(n.id)).map(n=><AreaSummary key={'summary-'+n.id}
          node={n} item={byID.get(n.id)} number={number.get(n.id)} badge={badge(n.id)} heading={groupHeadings.get(n.id)} enter={enter} select={select}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&scales.has(n.id)&&!detailed.has(n.id)).map(n=><ZoomMark key={'zoom-'+n.id} node={n} item={byID.get(n.id)} compactScale={groupHeadings.get(n.id).scale} enter={enter} select={select}/>)}
        {area&&visible(area)&&(detailed.has(area)||openComponents.has(area))&&view.numbered&&labels.map(label=><ConnectionLabel key={label.id} label={label}/>)}
      </ViewportPortal>
    </ReactFlow>;
  }
  // What a number stands for, beside its label: every call behind it, caller
  // and callee, each a link into the code. The card is drawn over the canvas
  // at screen size and kept inside it, and the pointer can move onto it.
  // A card is an element of the map: it stands beside its label on the outer
  // side of the frame, in the map's own coordinates, so it is always in the
  // same place and zooms and moves with everything else. Each row is a call:
  // caller and callee as links, or the outside symbol with where it is called.
  function ConnectionCalls({label,only,side,go}){
    const name=id=>byID.get(id)?.name||byID.get(id)?.title||'';
    const ids=only===undefined?null:new Set(label.byNumber.get(only)?.ids||[]);
    const groups=new Map(),seen=new Set();
    for(const relation of label.relations){
      if(ids&&!ids.has(relation.from)&&!ids.has(relation.to))continue;
      const calls=relation.calls?.length?relation.calls:[{label:relation.label||relation.summary||'',from:relation.fromSource,to:relation.toSource}];
      for(const call of calls){
        const parts=String(call.label||'').match(/^(\S+) (\S+) (\S+)$/);
        // Calls between parts read under "part → part"; calls to an outside
        // symbol read under the part that makes them.
        const title=parts?`${name(relation.from)} → ${name(relation.to)}`:name(label.incoming?relation.to:relation.from);
        const key=`${title}|${parts?call.label:name(label.incoming?relation.from:relation.to)+'|'+(call.at||'')}`;
        if(seen.has(key))continue;seen.add(key);
        if(!groups.has(title))groups.set(title,[]);
        groups.get(title).push({parts,call,other:name(label.incoming?relation.from:relation.to)});
      }
    }
    if(!groups.size)return null;
    const link=(href,text)=>href?<a href={href} target="_blank" rel="noopener" onClick={event=>event.stopPropagation()}>{text}</a>:<span>{text}</span>;
    return <div className={`flow-connection-calls flow-calls-${side} nopan nowheel`} onClick={event=>event.stopPropagation()}>
      <header><b>{only===undefined?label.numbers.join(' · '):only}</b><button type="button" onClick={go}>{label.incoming?'←':'→'} {label.title}</button></header>
      {[...groups].map(([title,rows])=><div key={title}><strong>{title}</strong>
        {rows.map(({parts,call,other},i)=><p key={i}>{parts
          ?<>{link(call.from,parts[1])}<i>{parts[2]==='calls'?' → ':` ${parts[2].replace(/_/g,' ')} `}</i>{link(call.to||call.from,parts[3])}</>
          :<>{link(call.from||call.to,other)}{call.at&&<em> {call.at}</em>}</>}</p>)}</div>)}
    </div>;
  }
  function ConnectionLabel({label}){
    const {zoom}=useViewport();
    const [hovered,setHovered]=useState(null),element=useRef(null),closing=useRef(0);
    const open=only=>{clearTimeout(closing.current);setHovered({only});};
    const close=()=>{clearTimeout(closing.current);closing.current=setTimeout(()=>setHovered(null),220);};
    useEffect(()=>()=>clearTimeout(closing.current),[]);
    let upright=false,sideName='bottom';
    let style={transform:`translate(${label.x}px,${label.y}px) scale(${label.scale||1})`,transformOrigin:'top left',width:label.width/(label.scale||1),minHeight:label.height/(label.scale||1)};
    if(label.boundary){
      const p=label.point,side={left:{dx:1,dy:0,tx:0,ty:-50},right:{dx:-1,dy:0,tx:-100,ty:-50},top:{dx:0,dy:1,tx:-50,ty:0},bottom:{dx:0,dy:-1,tx:-50,ty:-100}}[label.side];
      // Along a side border the numbers stand one above another.
      upright=side.dx!==0&&label.numbers.length>1;
      sideName=label.side;
      const scale=label.labelScale;
      style={transform:`translate(${p.x+side.dx*6/zoom}px,${p.y+side.dy*6/zoom}px) scale(${scale}) translate(${side.tx}%,${side.ty}%)`,transformOrigin:'top left'};
    }
    // A number's badge on its part opens the cards of that number beside
    // every arrow it has; a click on the badge keeps them open.
    const shown=[pinnedPart,cardPart].filter(Boolean).map(id=>[...(label.byNumber||[])].find(([,entry])=>entry.key===id)?.[0]).find(k=>k!==undefined);
    const only=hovered?hovered.only:shown;
    return <div ref={element}
          className={`flow-connection-label nopan ${label.boundary?'flow-boundary-label':''} ${upright?'flow-label-upright':''}`}
          onMouseEnter={()=>clearTimeout(closing.current)} onMouseLeave={close} data-connection-outside={label.outside} data-connection-label={label.id}
          style={style}
          >
          {!label.boundary&&<span>{label.incoming?'← ':'→ '}{label.labelTitle}</span>}
          <button type="button" aria-label={label.title} className={pinnedLabels.has(label.id)?'flow-label-pinned':''}
            onClick={event=>{event.stopPropagation();if(pinnedLabels.has(label.id))pinnedLabels.delete(label.id);else pinnedLabels.set(label.id,label.area);update?.();}}>
            {label.numbers.map((k,i)=><React.Fragment key={k}>{i>0&&!upright&&<i> · </i>}
              <b className={label.bold?.has(k)?'flow-number-active':''} onMouseEnter={()=>open(label.numbers.length>1?k:undefined)}>{k}</b></React.Fragment>)}
          </button>
          {(hovered||shown!==undefined||pinnedLabels.has(label.id))&&<ConnectionCalls label={label} only={only} side={sideName} go={event=>{event.stopPropagation();clearHover();select(label.outside,event,true);}}/>}
        </div>;
  }
  map.classList.add('flow-enabled');source.style.display='none';source.setAttribute('aria-hidden','true');
  const root=createRoot(host);flushSync(()=>root.render(<App/>));
  map.addEventListener('pointerleave',clearHover);
  // Restoring the map's pinned emphasis must not remove connection evidence
  // while the reader moves into the adjacent column to use its source links.
  stage.addEventListener('pointerleave',()=>{hoverArea='';update?.();});
  stage.closest('.map-workspace')?.addEventListener('pointerleave',clearHover);
  window.addEventListener('blur',clearHover);
  document.addEventListener('visibilitychange',()=>{if(document.hidden)clearHover();});
  map.querySelector('[data-map-controls]').hidden=false;
  const overviewButton=map.querySelector('[data-map-fit]');overviewButton.textContent=t('Show whole map');overviewButton.removeAttribute('title');
  map.querySelector('[data-map-controls]').addEventListener('click',event=>{
    const button=event.target.closest('button');if(!button||!instance)return;
    if(button.hasAttribute('data-map-fit'))map.showWholeMap();
    else if(button.hasAttribute('data-map-reset'))resetView();
    else if(button.hasAttribute('data-map-zoom')){overviewFit=false;commitCamera(instance.zoomTo(instance.getZoom()*Number(button.dataset.mapZoom)));}
  });
  return {get layout(){return layout;},focus,capture,restore,clearHover,overview:()=>fitOverview(420),update(next){
    if(view.scope!==next.scope||view.operation!==next.operation){hover.pause();preview='';map.clearMapPreview?.();}
    view={...initial,...next,scope:displayed(next.scope)||'',
      selected:new Set([...(next.selected||[])].map(displayed)),matched:new Set([...(next.matched||[])].map(displayed))};update();
  }};
};
