import React, {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow, Handle, Position, ViewportPortal, useViewport, useStore} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {symbolBlocks,symbolRow} from './symbols.mjs';
import {createLook} from './look.mjs';
import {emphasis, focusAncestors} from './emphasis.mjs';
import {createSemanticLayout, detailLayers, firstDetailZoom, componentTextSizes, frameViewport, partViewport, pathViewport, tileViewport, staysOpen, layerFloor, closedContainer, readableFocus, frameInventory, systemViewport} from './semantic.mjs';
import {routeDrawing} from './route-drawing.mjs';
import {singlePartAreas, inputGroupsByPart} from './overview.mjs';
import {prepareCards,wrapText,overviewHeading,overviewScale,groupHeading} from './cards.mjs';
import {overviewInset} from './split-layout.mjs';
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
  const mixed=new Set(blocks.filter(block=>{const all=[block.head,...block.rows].filter(i=>symbols[i].kind!=='more');return all.some(i=>symbols[i].inner)&&all.some(i=>!symbols[i].inner);}).flatMap(block=>[block.head,...block.rows]));
  const row=(i,className,first)=>{
    const symbol=symbols[i],kind=symbol.kind==='field'||symbol.kind==='more'?'flow-symbol-field':'';
    const props={className:`${className} ${kind} ${first?'flow-symbol-first-method':''} ${symbol.key?'flow-symbol-key':''} ${symbol.inner&&mixed.has(i)?'flow-symbol-inner':''} ${tone(i)}`,onMouseEnter:()=>setHot(i)};
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
        {block.more>0&&<span className="flow-symbol-row flow-symbol-rest">… +{block.more}</span>}
      </div>)}
      {hidden>0&&<span className="flow-symbol-more" style={{right:inset,bottom:inset}}>+{hidden}</span>}
    </div>
  </div>;
}
function Part({data}) {
  const heading=data.standaloneHeading,scale=heading?.scale||data.contentScale;
  const box=heading?{width:heading.width,height:heading.height}:data.fill||{width:data.originalWidth||data.width||260,height:data.originalHeight||data.height||88};
  // Only the flip between the two drawings re-renders the card, not every
  // step of a zoom.
  const far=useStore(state=>box.width*(scale||1)*state.transform[2]>=860);
  const deep=far&&!data.activation&&data.symbols?.length>0;
  // A loose part's heading is fitted to its box; its description takes the
  // whole lines left under the title and the zoom mark's row.
  const standaloneLines=heading?Math.floor((heading.height-12-heading.title.split('\n').length*16-16)/15):0;
  const standaloneText=standaloneLines>0;
  if(deep)return <div className={`flow-part flow-part-deep flow-${data.category} ${data.lane==='core'?'flow-core':data.lane==='triggers'?'flow-entry':''}`}
      style={{width:box.width,height:box.height,transform:`scale(${scale||1})`,transformOrigin:'top left'}}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <strong>{data.name||data.title}</strong>
    <PartSymbols symbols={data.symbols} calls={data.symbolCalls} width={box.width} height={box.height}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
  return <div className={`flow-part flow-${data.category} ${data.category==='input'?'':data.lane==='core'?'flow-core':data.lane==='triggers'?'flow-entry':''} ${heading?'flow-standalone-part':''}`} data-input-id={data.activation?data.id:undefined} style={heading?{width:heading.width,height:heading.height,transform:`scale(${scale})`,transformOrigin:'top left'}:data.fill?{width:data.fill.width,height:data.fill.height,transform:`scale(${scale||1})`,transformOrigin:'top left'}:scale&&scale!==1?{width:data.originalWidth,height:data.originalHeight,transform:`scale(${scale})`,transformOrigin:'top left'}:undefined}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    {data.roleLabel&&<span className={`flow-role-symbol flow-role-${data.lane}`} role="img" aria-label={data.roleLabel}/> }
    {data.kindLabel&&!heading&&<div className="flow-kind" data-input-kind={data.activation||undefined}>{data.kindLabel}</div>}
    <strong data-input-name={data.activation?'':undefined}>{heading?.title||data.title}</strong>
    {data.description&&(!heading||standaloneText)&&<div className="flow-description" style={heading?{WebkitLineClamp:standaloneLines,maxHeight:standaloneLines*15}:undefined}>{heading?data.description.replace(/\n/g,' '):data.description}</div>}
    {data.subtitle&&<div className="flow-address">{data.subtitle}</div>}
    {data.symbols?.length>0&&!data.activation&&<button type="button" className="flow-part-zoom nopan" aria-label={t('Zoom into {0}',data.name||data.title)}
      onClick={event=>{event.stopPropagation();data.zoomInto?.();}}>
      <span className="flow-zoom-picture" aria-hidden="true"/></button>}
    {data.number && <span data-badge={data.id} className={`flow-number nopan ${data.badge?.pinned?'flow-number-pinned':''}`} onMouseEnter={event=>data.badge?.enter(event)} onMouseLeave={()=>data.badge?.leave()}
      onClick={event=>{event.stopPropagation();data.badge?.toggle();}}>{data.number}</span>}
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function Area({data}) {
  return <div className={`flow-area ${data.branch==='component'?'flow-component':['communication','communication-group'].includes(data.branch)?'flow-communication':['inputs','inputs-part'].includes(data.branch)?'flow-input-collection':data.branch!=='area'?'':data.lane==='core'?'flow-area-core':data.lane==='triggers'?'flow-area-entry':''}`}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function AreaSummary({node,item,number,badge,heading,enter,select}){
  const {scale,title}=heading;
  // A closed area says what it is: its one-line description takes the whole
  // lines left under its title and above its number.
  const lines=Math.floor((node.height/scale-12-title.split('\n').length*16-32)/15);
  return <div className="flow-area-summary nopan" data-summary-area={node.id}
    style={{transform:`translate(${node.absolute.x}px,${node.absolute.y}px) scale(${scale})`,transformOrigin:'top left',
      width:node.width/scale,height:node.height/scale}}
    onMouseEnter={()=>enter(node.id)} onClick={event=>{event.stopPropagation();select(node.id,event,false);}}>
    <div className="flow-part flow-overview-card flow-overview-compact"><strong>{title}</strong>
      {item?.summary&&lines>0&&<p className="flow-description" style={{WebkitLineClamp:lines,maxHeight:lines*15}}>{item.summary}</p>}
      <footer>
        {number?<span data-badge={node.id} className={`flow-number ${badge?.pinned?'flow-number-pinned':''}`} onMouseEnter={event=>badge?.enter(event)} onMouseLeave={()=>badge?.leave()}
          onClick={event=>{event.stopPropagation();badge?.toggle();}}>{number}</span>:null}
      </footer></div>
    {['core','triggers'].includes(item?.lane)&&<span className={`flow-role-symbol flow-role-${item.lane}`} role="img" aria-label={item.roleLabel||item.lane}/>}
  </div>;
}
function ZoomMark({node,item,enter,select,compactScale,fitScale=Infinity}) {
  const viewport=useViewport(),{zoom}=viewport;
  const area=item.branch==='area',size=34,tall=24,inset=area||['communication','inputs'].includes(item.branch)?8:12;
  // A summary scaled down whole to fit its box takes its mark down with it
  // until zoom gives the mark its ordinary screen size.
  const scale=area?compactScale:Math.min(fitScale,1/zoom);
  // A frame reserved exactly this room is fitted to within float round-off.
  if(node.width<(size+2*inset-.5)*scale||node.height<(tall+2*inset-.5)*scale)return null;
  const point={x:node.absolute.x+node.width-(size+inset)*scale,y:node.absolute.y+inset*scale};
  const name=item.branch==='inputs'?`${t('Inputs')} · ${item.name||item.title}`:item.name||item.title;
  return <button type="button" className="flow-zoom-mark nopan" data-zoom-into={node.id}
    style={{transform:`translate(${point.x}px,${point.y}px) scale(${scale})`,width:size,height:tall}}
    aria-label={t('Zoom into {0}',name)} onMouseEnter={()=>enter(node.id)}
    onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    <span className="flow-zoom-picture" aria-hidden="true"/>
  </button>;
}
function FrameTitle({node,item,focused,enter,select}) {
  const viewport=useViewport();
  const scale=item.summaryScale||1;
  const component=item.branch==='component',communication=item.branch==='communication',inputs=['inputs','inputs-part'].includes(item.branch);
  const x=node.absolute.x+18*scale;
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
  const initial={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false,numbered:true};
  let cameraRevision=0;
  let update, instance, view=initial, hoverArea='', pinnedPart='', pinnedLabels=new Map(), lookOnly, preview='', restorePending, pendingFocus, panning=false, initializing=true;
  let detailed=new Set(),openComponents=new Set(),communicationsOpen=new Set(),arriving=new Set(),locationID='',locationSubject='',componentsOpen=false,zoom=systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom,paintedZoom,overviewFit=false;
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
    const next=detailState(viewport,new Set([...openComponents,...detailed,...communicationsOpen,...arriving]));
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
  // One look for the whole map: a badge, a label, whatever a layer adds.
  const look=createLook();
  // Set by each drawing of the map: showing a look needs the frame being read.
  let lookAt=()=>{};
  const lookBadge=()=>look.key.startsWith('badge:')?look.key.slice(6):'';
  const lookLabel=()=>look.key.startsWith('label:')?look.key.slice(6):'';
  function lookAway(key){
    const ask=look.leave(key,performance.now());
    if(ask)setTimeout(()=>{if(look.tick(performance.now()))update?.();},ask-performance.now()+5);
  }
  function enter(id){
    if(!hover.allowed)return;
    const n=byID.get(id);if(!n||n.display)return;
    // Hover affects the drawing only. The links and description opened by a
    // click stay usable while the pointer crosses other cards to reach them.
    // Pointing anywhere inside a target looks at the target: its frame, its
    // numbers and what it is. Its initialization wiring stays off, as it
    // does for a chosen target: init is drawn for a looked-at end only.
    const area=parentArea(id)||id;
    if(hoverArea!==area){hoverArea=area;update?.();}
  }
  // A chosen input is entered as its path: the part holding its handler and
  // the path's parts nearest it in call depth, as many as stay readable in
  // one camera, with the trace dark from there. Their areas open.
  function focusPath(input,smooth=true){
    const nodes=[...new Set((byID.get(input)?.trace||[]).map(displayed))].map(id=>placed.get(id)).filter(n=>n&&!n.frame);
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
    id=displayed(id);
    const tile=placed.get(id);if(!tile)return;
    if(!instance||initializing){pendingFocus={id,input:true};return;}
    overviewFit=false;hover.pause();preview='';map.clearMapPreview?.();
    const rect=host.getBoundingClientRect(),group=placed.get(tile.parentId);
    communicationsOpen=new Set([...communicationsOpen,rootOf(id)]);
    arrive([id]);
    locationSubject=id;
    const least=layerFloor(layout.nodes,semantic.records,rootOf(id),rect.width,rect.height);
    commitCamera(instance.setViewport(tileViewport(tile,byID.get(group?.id)?.branch==='inputs-part'?group:null,rect.width,rect.height,byID.get(id)?.contentScale||1,{least}),{duration:smooth?420:0}),id);
  }
  function focus(id,center=true,smooth=true){
    id=displayed(id);
    const n=placed.get(id);if(!n)return;
    const record=byID.get(id);
    if(record?.activation&&focusPath(id,smooth))return;
    if(record?.activation){showInput(id,smooth);return;}
    if(!instance||initializing){pendingFocus={id,center};return;}
    overviewFit=false;
    hover.pause();preview='';map.clearMapPreview?.();
    const viewport=instance.getViewport(), rect=host.getBoundingClientRect();
    const contentScale=byID.get(n.id)?.contentScale||1;
    if(!center&&readableFocus(n.id,placed,byID,detailed,componentsOpen,viewport,rect.width,rect.height,communicationsOpen,openComponents))return;
    locationSubject=id;
    if(n.frame){
      // Every frame is entered by the one rule, at the scale its own content is
      // drawn at: a component whole, the others no smaller than readable.
      const branch=byID.get(n.id).branch,component=branch==='component';
      const scale=component?(componentFonts.get(n.id)||20)/20:['communication','inputs'].includes(branch)?communicationScales().get(n.id)||1:scales.has(n.id)?contentScale:1;
      // Entering a frame opens it, so it may be shown as small as an open frame
      // stays open, about twelve pixels of text, rather than as large as a
      // closed one needs to open by itself.
      if(['communication','inputs'].includes(branch))communicationsOpen=new Set([...communicationsOpen,n.id]);
      else if(!component)detailed=new Set([...detailed,n.id]);
      if(!component)arrive([n.id]);
      const least=scales.has(n.id)?layerFloor(layout.nodes,semantic.records,n.id,rect.width,rect.height):Infinity;
      commitCamera(instance.setViewport(frameViewport(n,layout.nodes,rect.width,rect.height,scale,{whole:component,pad:component?12:24,floor:staysOpen,least}),{duration:smooth?420:0}),id);return;
    }
    commitCamera(instance.setViewport(partViewport(n,1/contentScale,rect.width,rect.height),{duration:smooth?420:0}),id);
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
  function ComponentOverview({node:n,fit,zoom}){
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
      const listHeight=areaIDs.length?7+areaIDs.reduce((h,id)=>h+10+textHeight(byID.get(id).overviewTitle||byID.get(id).name||byID.get(id).title,'500 13px system-ui',18),0):0;
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
    return <div key={'component-'+n.id} className={`flow-component-overview nopan ${item.branch==='communication'?'flow-communication-overview':item.branch==='inputs'?'flow-input-collection':''}`}
      data-component-overview={n.id} style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width,maxHeight:screenHeight-16}}
      onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,true);}}>
      {heading.lines.length>0&&<div className="flow-component-overview-heading" style={{maxWidth:heading.width,minHeight:inputs?32:undefined,paddingTop:heading.clearZoom?32:undefined}}>
        <strong style={heading.scale<1?{fontSize:heading.fontSize,lineHeight:`${heading.lineHeight}px`}:undefined}>{heading.lines.join('\n')}</strong></div>}
      {showRole&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
      {!communication&&!inputs&&descriptionLines>=2&&item.description&&<p className="flow-description flow-description-compact" style={{WebkitLineClamp:descriptionLines}}>{item.description.replace(/\n/g,' ')}</p>}
      {inputs&&<InputTypes groups={item.inputGroups}/>}
      {areaIDs.length>0&&<ul className={`flow-component-areas ${listOverflow?'flow-scrollable':''}`} onWheelCapture={scrollInventory}>{areaIDs.map(id=><li key={id}>
        <button type="button" className="nopan" data-overview-area={id} onClick={event=>{event.stopPropagation();select(id,event,true);}}>{byID.get(id).overviewTitle||byID.get(id).name||byID.get(id).title}</button>
      </li>)}</ul>}
      {showCounts&&<div className="flow-inside-counts">{counts}</div>}
    </div>;
  }
  // A display group's frame carries the text its frames all name, once, in
  // its band under the tiles or after them, where no arrow runs. It is laid
  // out at the whole-map camera like their summaries and zooms with the map;
  // once the tiles open it reads at their open frames' title size. It is no
  // participant: nothing to hover, choose or enter.
  function GroupHeading({node:n}){
    const item=byID.get(n.id),zoom=systemViewport(layout.nodes,layoutSize.width,layoutSize.height).zoom,beside=item.side==='right';
    const heading=useMemo(()=>item.headingAt(beside?Infinity:n.width*zoom),[n.width,zoom,beside]);
    const open=item.tiles.some(id=>communicationsOpen.has(id));
    // A band left short of the heading takes it smaller, never over the tiles.
    const scale=Math.min(1,item.band*zoom/(beside?heading.extent:heading.height))/zoom*
      (open?Math.min(1,(byID.get(item.tiles[0])?.summaryScale||1)*17/13*zoom):1);
    const x=beside?n.absolute.x+n.width-item.band+8*scale:n.absolute.x+8*scale;
    const y=beside?n.absolute.y+16:n.absolute.y+n.height-item.band+4*scale;
    return <div className="flow-group-heading" data-group-heading={n.id}
      style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width:heading.width,
        fontSize:heading.fontSize,lineHeight:`${heading.lineHeight}px`}}>{heading.lines.join('\n')}</div>;
  }
  function ComponentPresentation({node,focused}){
    const item=byID.get(node.id),open=item.branch==='component'?openComponents.has(node.id):communicationsOpen.has(node.id);
    const {fit,zoom}=useOverviewFit(node);
    // Open, a plain tile shows its calls under its group's heading alone.
    if(open&&item.displayGroupTitle)return null;
    return open?<FrameTitle node={node} item={item} focused={focused} enter={enter} select={select}/>:<>
      <ComponentOverview node={node} fit={fit} zoom={zoom}/><ZoomMark node={node} item={item} fitScale={fit<1?fit/zoom:undefined} enter={enter} select={select}/>
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
      // The closed card is a title over a foot row, the number and the role mark:
      // the title is fitted into what the foot row leaves.
      return new Map(layout.nodes.filter(n=>scales.has(n.id)).map(n=>[n.id,groupHeading(n,byID.get(n.id).name||byID.get(n.id).title,scale,measure,56,48)]));
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
    const badge=id=>({pinned:pinnedPart===id,enter:()=>lookAt(`badge:${id}`),leave:()=>lookAway(`badge:${id}`),
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
      // A card is read together with the things the look is on, so it takes
      // their scale, wherever its label stands: a label on the target's frame
      // is large, and a card that large beside an area's parts would push the
      // camera far back for nothing.
      const readScale=wholeComponent?labelScale:Math.max(...members.map(id=>byID.get(id)?.contentScale||0),0)||labelScale;
      const cardScale=readScale/labelScale;
      const byNumber=new Map();
      for(const id of group.insides){const k=number.get(numbered(id));if(!byNumber.has(k))byNumber.set(k,{key:numbered(id),ids:[]});byNumber.get(k).ids.push(id);}
      const active=state.mode==='all'?new Set():new Set(layout.edges.filter(e=>group.edges.includes(e.id)&&state.activeEdges.has(e.id))
        .map(e=>number.get(numbered(inside.has(e.from)?e.from:e.to))));
      const bold=active.size<byNumber.size||byNumber.size===1&&state.mode==='hover'&&hoverArea!==area?active:new Set();
      return [{...group,id:`boundary:${area}:${group.key}`,boundary:true,root,point,frameScale,side,labelScale,cardScale,byNumber,bold,order:Math.min(...group.numbers)*1000+(group.incoming?0:1),title:outside.name||outside.title}];
    });
    // Far enough into one part to read its declarations.
    function deepInto(node){
      if(!instance)return;
      const rect=host.getBoundingClientRect(),zoom=Math.min(maxZoom,Math.min((rect.width-80)/node.width,(rect.height-80)/node.height));
      instance.setCenter(node.absolute.x+node.width/2,node.absolute.y+node.height/2,{zoom,duration:420});
    }
    // A look opens a card beside the thing looked at and moves nothing: the
    // pointer stays on what it was on.
    lookAt=key=>{look.enter(key);update?.();};
    useEffect(()=>callbacks.emphasis?.({...state,overview}),[state.mode,state.subject,state.readingOutside,view.scope,overview]);
    // A closed frame stands for the participants hidden inside it.
    const shut=id=>scales.has(id)?!detailed.has(id):byID.get(id)?.branch==='component'?!openComponents.has(id):['communication','inputs'].includes(byID.get(id)?.branch)&&!communicationsOpen.has(id);
    const nodes=drawing.nodes.map(n=>{
      const item=byID.get(n.id),focused=state.focus.has(n.id);
      const reading=view.scope===n.id||view.operation===n.id;
      const contains=n.frame&&leaves(n.id).some(id=>state.participants.has(id));
      const on=state.participants.has(n.id)||contains&&(overview||shut(n.id));
      return {...n,type:n.frame?'area':'part',selected:reading,measured:{width:n.width,height:n.height},
        selectable:false,draggable:false,connectable:false,
        style:{width:n.width,height:n.height,visibility:visible(n.id)?'visible':'hidden'},
        className:`${on||contains||!dim?'':'flow-node-muted'} ${focused?'flow-node-focus':on?'flow-node-connected':''} ${context.has(n.id)?'flow-node-context':''} ${reading?'flow-node-reading':''}`,
        data:{...item,...looseLook(n),operation:view.operation,reading,number:number.get(n.id),badge:badge(n.id),zoomInto:()=>deepInto(n),open:(id,event)=>select(id,event,true)}};
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
        else if(pendingFocus?.path)focusPath(pendingFocus.path);
        else if(pendingFocus?.input)showInput(pendingFocus.id);
        else if(pendingFocus)focus(pendingFocus.id,pendingFocus.center);
        else if(view.scope||view.operation)focus(view.scope||view.operation,true);
        else map.dispatchEvent(new Event('repomap:viewport'));
      })));}}
      onNodeClick={(event,n)=>{event.stopPropagation();select(n.id,event,false);}}
      onNodeMouseEnter={(_,n)=>enter(n.id)}
      onMouseMove={event=>{
        if(panning||!instance)return;
        const looked=lookBadge();
        if(looked&&(event.target.closest?.('.flow-part-summary')||event.target.closest?.('.react-flow__node')?.dataset.id===looked||event.target.closest?.('[data-summary-area]')?.dataset.summaryArea===looked))look.enter(look.key);
        if(look.move(event.clientX,event.clientY,performance.now(),!!event.target.closest?.('[data-badge],.flow-connection-label,.flow-part-summary')))update?.();
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
        const area=drawing.nodes.filter(n=>n.frame&&!n.display&&visible(n.id)&&point.x>=n.absolute.x&&point.x<=n.absolute.x+n.width&&point.y>=n.absolute.y&&point.y<=n.absolute.y+n.height)
          .sort((a,b)=>a.width*a.height-b.width*b.height)[0];
        if(area)enter(area.id);
      }}
      onPaneClick={event=>{
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
        updateDetail(viewport);
        map.querySelectorAll('[data-map-zoom]').forEach(button=>{button.disabled=Number(button.dataset.mapZoom)<1&&viewport.zoom<=minZoom();});
      }}
      onMoveStart={event=>{if(event){overviewFit=false;locationSubject='';arriving=new Set();}panning=true;hover.pause();preview='';map.clearMapPreview?.();}}
      onMoveEnd={event=>{panning=false;hover.pause();if(instance)updateDetail(instance.getViewport());updateLocation(event);map.dispatchEvent(new Event('repomap:viewport'));}}>
      <svg className="flow-defs"><defs>
        <marker id="flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b"/></marker>
        <marker id="flow-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#34445b"/></marker>
      </defs></svg>
      <ViewportPortal>
        {drawing.nodes.filter(n=>n.frame&&!n.display&&visible(n.id)&&!['component','communication','inputs'].includes(byID.get(n.id).branch)&&(componentsOpen||scales.has(n.id)||byID.get(n.id).branch==='inputs-part')&&(!scales.has(n.id)||detailed.has(n.id))).map(n=><FrameTitle key={n.id} node={n} item={byID.get(n.id)} focused={state.focus.has(n.id)||context.has(n.id)} enter={enter} select={select}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&['component','communication','inputs'].includes(byID.get(n.id).branch)).map(n=><ComponentPresentation key={'component-'+n.id} node={n} focused={state.focus.has(n.id)||context.has(n.id)}/>)}
        {drawing.nodes.filter(n=>n.display&&byID.get(n.id)?.headingAt).map(n=><GroupHeading key={'group-'+n.id} node={n}/>)}
        {drawing.nodes.filter(n=>scales.has(n.id)&&visible(n.id)&&!detailed.has(n.id)).map(n=><AreaSummary key={'summary-'+n.id}
          node={n} item={byID.get(n.id)} number={number.get(n.id)} badge={badge(n.id)} heading={groupHeadings.get(n.id)} enter={enter} select={select}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&scales.has(n.id)&&!detailed.has(n.id)).map(n=><ZoomMark key={'zoom-'+n.id} node={n} item={byID.get(n.id)} compactScale={groupHeadings.get(n.id).scale} enter={enter} select={select}/>)}
        {area&&visible(area)&&(detailed.has(area)||openComponents.has(area))&&view.numbered&&labels.map(label=><ConnectionLabel key={label.id} label={label}/>)}
        {[...new Set([pinnedPart,lookBadge()].filter(Boolean))].map(id=><PartSummary key={'summary:'+id} id={id} labels={labels}/>)}
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
  // The calls behind one label, optionally of one number only: what is on
  // the other side, a part with its calls under it or an outside symbol with
  // where it is called.
  function callGroups(label,only){
    const name=id=>byID.get(id)?.name||byID.get(id)?.title||'';
    const ids=only===undefined?null:new Set(label.byNumber.get(only)?.ids||[]);
    const groups=new Map(),seen=new Set();
    for(const relation of label.relations){
      if(ids&&!ids.has(relation.from)&&!ids.has(relation.to))continue;
      const other=name(label.incoming?relation.from:relation.to);
      const calls=relation.calls?.length?relation.calls:[{label:relation.label||relation.summary||'',from:relation.fromSource,to:relation.toSource}];
      for(const call of calls){
        const parts=String(call.label||'').match(/^(\S+) (\S+) (\S+)$/);
        const heading=parts?other:'';
        const row=`${heading}|${parts?call.label:other+'|'+(call.at||call.name||'')}`;
        if(seen.has(row))continue;seen.add(row);
        if(!groups.has(heading))groups.set(heading,[]);
        groups.get(heading).push({parts,call,other});
      }
    }
    return groups;
  }
  function Calls({label,groups,go}){
    const link=(href,text)=>href?<a href={href} target="_blank" rel="noopener" onClick={event=>event.stopPropagation()}>{text}</a>:<span>{text}</span>;
    return <section>
      <header><button type="button" onClick={go}>{label.incoming?'←':'→'} {label.title}</button></header>
      {[...groups].map(([title,rows])=><div key={title}>{title&&title!==label.title&&<strong>{title}</strong>}
        {rows.map(({parts,call,other},i)=><p key={i}>{parts
          ?<>{link(call.from,parts[1])}<i>{parts[2]==='calls'?' → ':` ${parts[2].replace(/_/g,' ')} `}</i>{link(call.to||call.from,parts[3])}</>
          :call.name?<>{link(call.from,other)}<i> → </i>{link(call.to,call.name)}</>:<>{link(call.from||call.to,other)}{call.at&&<em> {call.at}</em>}</>}</p>)}</div>)}
    </section>;
  }
  // The wheel scrolls a card only when the card has something to scroll.
  const wheel=el=>{if(el)el.classList.toggle('nowheel',el.scrollHeight>el.clientHeight+1);};
  const going=label=>event=>{event.stopPropagation();clearHover();select(label.outside,event,true);};
  // A label's card stands diagonally from it, toward the roomier side and end
  // of the canvas, so it never leaves the canvas by standing on a border.
  function ConnectionCalls({label,only,at}){
    const groups=callGroups(label,only);
    if(!groups.size)return null;
    const view=instance?.getViewport()||{x:0,y:0,zoom:1};
    // The roomier side of the label; a card wider than that room is moved
    // back along the label until it stands inside the canvas.
    const outer=(label.labelScale||label.scale||1)*view.zoom,wide=520*outer*(label.cardScale||1);
    const sx=at.x*view.zoom+view.x,right=sx<host.clientWidth/2,down=at.y*view.zoom+view.y<host.clientHeight/2;
    const over=Math.max(0,right?sx+wide-host.clientWidth+8:wide-sx+8)/outer;
    const style={[right?'marginLeft':'marginRight']:-over};
    if(label.cardScale&&Math.abs(label.cardScale-1)>.01)Object.assign(style,{transform:`scale(${label.cardScale})`,transformOrigin:`${down?'top':'bottom'} ${right?'left':'right'}`});
    return <div className={`flow-calls-place flow-calls-${right?'right':'left'} flow-calls-${down?'down':'up'} nopan`} onClick={event=>event.stopPropagation()} style={style}>
      <div ref={wheel} className="flow-connection-calls"><Calls label={label} groups={groups} go={going(label)}/></div></div>;
  }
  // Looking at a part's number: one card beside the part with everything it
  // is joined to outside its frame, neighbour by neighbour. It stands on the
  // side of the part that has more of the canvas, and nothing else moves.
  function PartSummary({id,labels}){
    const node=placed.get(id),key=`badge:${id}`;
    const mine=labels.map(label=>[label,[...label.byNumber].find(([,entry])=>entry.key===id)?.[0]]).filter(([,k])=>k!==undefined)
      .map(([label,k])=>[label,callGroups(label,k)]).filter(([,groups])=>groups.size);
    if(!node||!mine.length)return null;
    const scale=mine[0][0].labelScale*(mine[0][0].cardScale||1);
    const view=instance?.getViewport()||{x:0,y:0,zoom:1};
    const right=(node.absolute.x+node.width/2)*view.zoom+view.x<host.clientWidth/2,down=(node.absolute.y+node.height/2)*view.zoom+view.y<host.clientHeight/2;
    const x=right?node.absolute.x+node.width+10*scale:node.absolute.x-10*scale,y=down?node.absolute.y:node.absolute.y+node.height;
    return <div className="flow-calls-place flow-part-summary nopan" onMouseEnter={()=>lookAt(key)} onMouseLeave={()=>lookAway(key)} onClick={event=>event.stopPropagation()}
      style={{transform:`translate(${x}px,${y}px) scale(${scale}) translate(${right?0:-100}%,${down?0:-100}%)`,transformOrigin:'top left'}}>
      <div ref={wheel} className="flow-connection-calls">{mine.map(([label,groups])=><Calls key={label.id} label={label} groups={groups} go={going(label)}/>)}</div></div>;
  }
  function ConnectionLabel({label}){
    const {zoom}=useViewport();
    const key=`label:${label.id}`,hovered=look.key===key;
    let upright=false;
    let style={transform:`translate(${label.x}px,${label.y}px) scale(${label.scale||1})`,transformOrigin:'top left',width:label.width/(label.scale||1),minHeight:label.height/(label.scale||1)};
    if(label.boundary){
      const p=label.point,side={left:{dx:1,dy:0,tx:0,ty:-50},right:{dx:-1,dy:0,tx:-100,ty:-50},top:{dx:0,dy:1,tx:-50,ty:0},bottom:{dx:0,dy:-1,tx:-50,ty:-100}}[label.side];
      // Along a side border the numbers stand one above another.
      upright=side.dx!==0&&label.numbers.length>1;
      const scale=label.labelScale;
      style={transform:`translate(${p.x+side.dx*6/zoom}px,${p.y+side.dy*6/zoom}px) scale(${scale}) translate(${side.tx}%,${side.ty}%)`,transformOrigin:'top left'};
    }
    // A number's badge on its part opens the cards of that number beside
    // every arrow it has; a click on the badge keeps them open.
    const only=hovered?lookOnly:undefined;
    return <div
          className={`flow-connection-label nopan ${label.boundary?'flow-boundary-label':''} ${upright?'flow-label-upright':''}`}
          onMouseEnter={()=>{if(!hovered)lookOnly=undefined;lookAt(key);}} onMouseLeave={()=>lookAway(key)} data-connection-outside={label.outside} data-connection-label={label.id}
          style={style}
          >
          {!label.boundary&&<span>{label.incoming?'← ':'→ '}{label.labelTitle}</span>}
          <button type="button" aria-label={label.title} className={pinnedLabels.has(label.id)?'flow-label-pinned':''}
            onClick={event=>{event.stopPropagation();if(pinnedLabels.has(label.id))pinnedLabels.delete(label.id);else pinnedLabels.set(label.id,label.area);update?.();}}>
            {label.numbers.map((k,i)=><React.Fragment key={k}>{i>0&&!upright&&<i> · </i>}
              <b className={`${label.bold?.has(k)?'flow-number-active':''} ${hovered&&(lookOnly===k||lookOnly===undefined)?'flow-number-open':''}`} onMouseEnter={()=>{lookOnly=label.numbers.length>1?k:undefined;if(look.key===key)update?.();}}>{k}</b></React.Fragment>)}
          </button>
          {(hovered||pinnedLabels.has(label.id))&&<ConnectionCalls label={label} only={only} at={label.boundary?label.point:label}/>}
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
    else if(button.hasAttribute('data-map-zoom')){overviewFit=false;commitCamera(instance.zoomTo(instance.getZoom()*Number(button.dataset.mapZoom)));}
  });
  return {get layout(){return layout;},focus,showInput,capture,restore,clearHover,overview:()=>fitOverview(420),update(next){
    if(view.scope!==next.scope||view.operation!==next.operation){hover.pause();preview='';map.clearMapPreview?.();}
    view={...initial,...next,scope:displayed(next.scope)||'',
      selected:new Set([...(next.selected||[])].map(displayed)),matched:new Set([...(next.matched||[])].map(displayed))};update();
  }};
};
