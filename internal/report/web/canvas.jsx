import React, {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow, Handle, Position, ViewportPortal, useViewport} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {emphasis, focusAncestors} from './emphasis.mjs';
import {createSemanticLayout, detailedAreas, fullyVisibleFrames, componentDetails, componentTextSizes, communicationDetails, componentViewport, communicationViewport, closedContainer, readableFocus, frameInventory, systemViewport, zoomMarkPosition} from './semantic.mjs';
import {routeDrawing} from './route-drawing.mjs';
import {prepareCards,wrapText,overviewHeading} from './cards.mjs';
import {HoverGate} from './hover.mjs';
import {InputTypes, OverviewMembers, scrollInventory} from './card-content.jsx';
import '@xyflow/react/dist/style.css';
import './canvas.css';

// Legacy small repository diagrams share the same embedded ELK instance code.
window.ELK = ELK;
const t = (...args) => window.rmT(...args);
function Part({data}) {
  return <div className={`flow-part flow-${data.category} ${data.lane==='core'?'flow-core':''}`} data-input-id={data.activation?data.id:undefined} style={data.contentScale&&data.contentScale!==1?{width:data.originalWidth,height:data.originalHeight,transform:`scale(${data.contentScale})`,transformOrigin:'top left'}:undefined}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    {(data.kindLabel||data.reading)&&<div className="flow-kind" data-input-kind={data.activation||undefined}>{data.kindLabel}{data.reading&&<span className="flow-reading-badge">{t('Reading')}</span>}</div>}
    <strong data-input-name={data.activation?'':undefined}>{data.title}</strong>
    {data.description&&<div className="flow-description">{data.description}</div>}
    {data.subtitle&&<div className="flow-address">{data.subtitle}</div>}
    {data.number && <span className="flow-number">{data.number}</span>}
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function Area({data}) {
  return <div className={`flow-area ${data.branch==='component'?'flow-component':data.branch==='communication'?'flow-communication':data.branch==='inputs'?'flow-input-collection':''} ${data.summarized?'flow-area-summarized':''}`}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function OverviewCard({data}){
  if(data.single)return <Part data={{...data,reading:data.reading===data.id}}/>;
  return <div className="flow-part flow-overview-card">
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    <div className="flow-kind">{t('Area')}</div><strong>{data.title}</strong>
    <OverviewMembers members={data.members} reading={data.reading} operation={data.operation} open={data.open}/>
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function ZoomMark({node,item,enter,select,width,height}) {
  const viewport=useViewport(),{zoom}=viewport;
  const inset=['communication','inputs'].includes(item.branch)?8:12;
  const point=width?zoomMarkPosition(node,viewport,width,height,inset):{x:node.absolute.x+node.width-(28+inset)/zoom,y:node.absolute.y+inset/zoom};
  if(!point)return null;
  const name=item.branch==='inputs'?`${t('Inputs')} · ${item.name||item.title}`:item.name||item.title;
  return <button type="button" className="flow-zoom-mark nopan" data-zoom-into={node.id}
    style={{transform:`translate(${point.x}px,${point.y}px) scale(${1/zoom})`}}
    aria-label={t('Zoom into {0}',name)} onMouseEnter={()=>enter(node.id)}
    onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5M10.5 7.5v6m-3-3h6"/>
    </svg>
  </button>;
}
function FrameTitle({node,item,focused,enter,select}) {
  const viewport=useViewport();
  const scale=item.summaryScale||1;
  const component=item.branch==='component',communication=item.branch==='communication',inputs=item.branch==='inputs';
  const x=component||communication||inputs?Math.max(node.absolute.x+18*scale,Math.min(node.absolute.x+node.width-260*scale,(24-viewport.x)/viewport.zoom)):node.absolute.x+18*scale;
  return <div className={`flow-area-title nopan ${focused?'flow-area-title-focus':''} ${component?'flow-component-title':communication?'flow-communication-title':inputs?'flow-input-collection':''}`}
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
    <path d={data.path} fill="none" vectorEffect="non-scaling-stroke" style={data.possible?{strokeDasharray:'calc(7px / var(--flow-zoom, 1)) calc(5px / var(--flow-zoom, 1))'}:undefined} markerEnd={data.arrow?`url(#${data.on?'flow-arrow-active':'flow-arrow'})`:undefined}/>
  </g>;
}
const nodeTypes={part:Part,area:Area,overview:OverviewCard}, edgeTypes={routed:RoutedEdge};

window.rmCreateFlow = async function(map, stage, records, relations, areas, inputOwner, callbacks) {
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
  let update, instance, view=initial, hoverArea='', preview='', restorePending, pendingFocus, panning=false, initializing=true;
  let detailed=new Set(),openComponents=new Set(),communicationsOpen=new Set(),locationID='',componentsOpen=false,zoom=systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom,paintedZoom,overviewFit=false;
  const closed=id=>closedContainer(id,placed,byID,detailed,componentsOpen,communicationsOpen,openComponents);
  new ResizeObserver(()=>{if(instance&&!initializing&&overviewFit)fitOverview();}).observe(host);
  function updateLocation(){
    if(!instance)return;
    if(layoutError){location.textContent=t('Could not arrange this map. Reload to try again.');return;}
    if(!openComponents.size&&!communicationsOpen.size){location.textContent=t('System map');return;}
    const v=instance.getViewport(),w=host.clientWidth,h=host.clientHeight;
    const point={x:(w/2-v.x)/v.zoom,y:(h/2-v.y)/v.zoom};
    const candidates=layout.nodes.filter(n=>!closed(n.id)&&(scales.has(n.id)||byID.get(n.id)?.branch==='component'||['communication','inputs'].includes(byID.get(n.id)?.branch)||(!n.frame&&!semantic.owner(n.id)))).map(n=>{
      const x=n.absolute.x*v.zoom+v.x,y=n.absolute.y*v.zoom+v.y;
      const visible=Math.max(0,Math.min(w,x+n.width*v.zoom)-Math.max(0,x))*Math.max(0,Math.min(h,y+n.height*v.zoom)-Math.max(0,y));
      const dx=Math.max(n.absolute.x-point.x,0,point.x-n.absolute.x-n.width),dy=Math.max(n.absolute.y-point.y,0,point.y-n.absolute.y-n.height);
      return {id:n.id,visible,distance:dx*dx+dy*dy};
    }).filter(n=>n.visible>0).sort((a,b)=>a.distance-b.distance||b.visible-a.visible);
    if(candidates[0])locationID=candidates[0].id;
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
  const descendants=id=>(children.get(id)||[]).flatMap(child=>[child,...descendants(child)]);
  const contained=new Map(areas.map(a=>[a.id,descendants(a.id)]));
  const inventories=new Map(areas.map(a=>[a.id,frameInventory(a.id,children,byID)]));
  const leaves=id=>children.has(id)?children.get(id).flatMap(leaves):[id];
  const communicationScales=()=>new Map(areas.filter(a=>['communication','inputs'].includes(byID.get(a.id)?.branch))
    .map(a=>[a.id,Math.min(1,...leaves(a.id).map(id=>byID.get(id)?.contentScale||1))]));
  const maximumZoom=()=>Math.max(2,...[...scales.values(),...communicationScales().values(),
    ...[...byID.values()].filter(n=>!n.children?.length).map(n=>n.contentScale||1)].map(scale=>1.8/scale));
  maxZoom=maximumZoom();
  function updateDetail(viewport){
    const whole=fullyVisibleFrames(layout.nodes,viewport,host.clientWidth,host.clientHeight);
    const open=componentDetails(componentFonts,viewport.zoom,openComponents,whole);
    const next=detailedAreas(scales,viewport.zoom,detailed,whole);
    const communication=communicationDetails(communicationScales(),viewport.zoom,communicationsOpen,whole);
    const changed=open.size!==openComponents.size||[...open].some(id=>!openComponents.has(id))||next.size!==detailed.size||[...next].some(id=>!detailed.has(id))||
      communication.size!==communicationsOpen.size||[...communication].some(id=>!communicationsOpen.has(id));
    openComponents=open;componentsOpen=!!open.size;detailed=next;communicationsOpen=communication;
    if(changed){hoverArea='';hover.pause();update?.();}
  }
  function parentArea(id){while(id){if(byID.get(id)?.branch==='area')return id;id=placed.get(id)?.parentId;}return '';}
  function rootOf(id){while(placed.get(id)?.parentId)id=placed.get(id).parentId;return id;}
  function clearHover(){hoverArea='';preview='';map.clearMapPreview?.();update?.();}
  function commitCamera(movement){
    // React Flow's imperative camera methods need not emit onMoveEnd. Save
    // only after they finish, or Back restores the previous display's camera.
    const revision=++cameraRevision;
    return Promise.resolve(movement).then(()=>{if(revision===cameraRevision&&!initializing){updateLocation();map.dispatchEvent(new Event('repomap:viewport'));}});
  }
  function enter(id){
    if(!hover.allowed)return;
    const n=byID.get(id);if(!n)return;
    // Hover affects the drawing only. The links and description opened by a
    // click stay usable while the pointer crosses other cards to reach them.
    const area=parentArea(id)||id;
    if(hoverArea!==area){hoverArea=area;update?.();}
  }
  function focus(id,center=true,smooth=true){
    const n=placed.get(id);if(!n)return;
    if(!instance||initializing){pendingFocus={id,center};return;}
    overviewFit=false;
    hover.pause();preview='';map.clearMapPreview?.();
    const {x,y}=n.absolute, viewport=instance.getViewport(), rect=host.getBoundingClientRect();
    const contentScale=byID.get(n.id)?.contentScale||1;
    if(!center&&readableFocus(n.id,placed,byID,detailed,componentsOpen,viewport,rect.width,rect.height,communicationsOpen,openComponents))return;
    if(n.frame){
      if(['component','communication','inputs'].includes(byID.get(n.id).branch)){
        const destination=['communication','inputs'].includes(byID.get(n.id).branch)
          ?communicationViewport(n,layout.nodes,rect.width,rect.height,communicationScales().get(n.id)||1)
          :componentViewport(n,layout.nodes,rect.width,(componentFonts.get(n.id)||20)/20);
        commitCamera(instance.setViewport(destination,{duration:smooth?420:0}));return;
      }
      const zoom=scales.has(n.id)?1/contentScale:Math.min(1,Math.max(.6,Math.min((rect.width-48)/n.width,(rect.height-48)/n.height)));
      commitCamera(instance.setViewport({x:24-x*zoom,y:24-y*zoom,zoom},{duration:smooth?420:0}));return;
    }
    const zoom=1/contentScale;
    commitCamera(instance.setCenter(x+n.width/2,y+Math.min(n.height/2,rect.height/(2*zoom)-24),{zoom,duration:smooth?420:0}));
  }
  function capture(){return instance&&!initializing?{...instance.getViewport(),layoutKey,overview:isOverview(),detailAreas:[...detailed],componentsOpen,openComponents:[...openComponents],communicationsOpen:[...communicationsOpen],fit:overviewFit}:restorePending||null;}
  function restore(v){if(!v)return;
    if(!instance||initializing){restorePending=v;return;}
    if(v.fit===true||(v.fit===undefined&&v.componentsOpen===false&&!view.scope&&!view.operation)){fitOverview();return;}
    // A regenerated report can have different geometry. Old screen coordinates
    // must not place its still-selected item outside the visible viewport.
    if(!Number.isFinite(v.zoom)||v.layoutKey!==layoutKey){if(view.scope||view.operation)focus(view.scope||view.operation,true);else fitOverview();return;}
    overviewFit=false;
    const whole=fullyVisibleFrames(layout.nodes,v,host.clientWidth,host.clientHeight);
    detailed=detailedAreas(scales,v.zoom,new Set(v.detailAreas||[]),whole);
    openComponents=componentDetails(componentFonts,v.zoom,new Set(v.openComponents||(v.componentsOpen?[...componentFonts.keys()]:[])),whole);
    componentsOpen=!!openComponents.size;
    communicationsOpen=communicationDetails(communicationScales(),v.zoom,new Set(v.communicationsOpen||[]),whole);
    zoom=v.zoom;update?.();
    hover.pause();preview='';map.clearMapPreview?.();
    if(instance)commitCamera(instance.setViewport(v));else restorePending=v;
  }
  function resetView(){if(!instance)return;if(isOverview()){fitOverview();return;}const first=layout.nodes.filter(n=>!n.frame).sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];focus(view.scope||view.operation||first?.id,true);}
  function fitOverview(duration=0){
    overviewFit=true;
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
  // Screen-sized summaries follow the camera without rebuilding graph props.
  function ComponentOverview({node:n}){
    const viewport=useViewport(),zoom=viewport.zoom;
    const left=Math.max(0,n.absolute.x*zoom+viewport.x),right=Math.min(host.clientWidth,(n.absolute.x+n.width)*zoom+viewport.x);
    const item=byID.get(n.id),inventory=inventories.get(n.id),screenWidth=right-left,screenHeight=n.height*zoom;
    const visible=screenWidth>32,width=Math.min(320,screenWidth-16),contentWidth=Math.max(1,width-16);
    const heading=useMemo(()=>visible?overviewHeading(item,screenWidth,measure):null,[screenWidth]);
    const text=useMemo(()=>{
      if(!visible)return null;
      const communication=item.branch==='communication',inputs=item.branch==='inputs',areaIDs=communication||inputs?[]:inventory.areaIDs;
      const textHeight=(text,font,lineHeight)=>wrapText(text,contentWidth,font,measure).length*lineHeight;
      const listHeight=areaIDs.length?7+areaIDs.reduce((h,id)=>h+10+textHeight(byID.get(id).name||byID.get(id).title,'500 13px system-ui',18),0):0;
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
    // Screen-sized summaries must stay in the visible part of their own
    // frame while approaching its contents. Their world corner can leave
    // the viewport long before the interior reaches readable scale.
    const x=(left+8-viewport.x)/zoom;
    const summaryHeight=inputs?text.inputHeight:heading.height+24;
    const y=Math.max(n.absolute.y+8/zoom,Math.min(n.absolute.y+n.height-summaryHeight/zoom,(8-viewport.y)/zoom));
    return <div key={'component-'+n.id} className={`flow-component-overview nopan ${item.branch==='communication'?'flow-communication-overview':item.branch==='inputs'?'flow-input-collection':''}`}
      data-component-overview={n.id} style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width,maxHeight:(n.absolute.y+n.height-y)*zoom-8}}
      onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,true);}}>
      <div className="flow-component-overview-heading" style={{maxWidth:heading.width,minHeight:inputs?32:undefined,paddingTop:heading.clearZoom?32:undefined}}><strong>{item.name}</strong></div>
      {showRole&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
      {!communication&&!inputs&&descriptionLines>=2&&item.description&&<p className="flow-description flow-description-compact" style={{WebkitLineClamp:descriptionLines}}>{item.description.replace(/\n/g,' ')}</p>}
      {inputs&&<InputTypes groups={item.inputGroups}/>}
      {areaIDs.length>0&&<ul className={`flow-component-areas ${listOverflow?'flow-scrollable':''}`} onWheelCapture={scrollInventory}>{areaIDs.map(id=><li key={id}>
        <button type="button" className="nopan" data-overview-area={id} onClick={event=>{event.stopPropagation();select(id,event,true);}}>{byID.get(id).name||byID.get(id).title}</button>
      </li>)}</ul>}
      {showCounts&&<div className="flow-inside-counts">{counts}</div>}
    </div>;
  }
  function ComponentPresentation({node,focused}){
    const viewport=useViewport();
    const item=byID.get(node.id),open=item.branch==='component'?openComponents.has(node.id):communicationsOpen.has(node.id);
    const [fallback,setFallback]=useState(!open);
    const inspected=useRef(null);
    const left=node.absolute.x*viewport.zoom+viewport.x,top=node.absolute.y*viewport.zoom+viewport.y;
    const intersects=left<host.clientWidth&&top<host.clientHeight&&
      left+node.width*viewport.zoom>0&&top+node.height*viewport.zoom>0;
    useLayoutEffect(()=>{
      if(!open||!intersects){setFallback(!open);return;}
      const inspect=()=>{
        const canvas=host.getBoundingClientRect();
        // Keep a fallback only while the camera sees empty compound padding.
        // Once any real child card is visible, its background already competes
        // with the summary, even if its own heading is clipped by the viewport.
        const visibleContent=(contained.get(node.id)||[]).some(id=>{
          const child=placed.get(id);
          if(!child||closed(id))return false;
          const x=child.absolute.x*viewport.zoom+viewport.x,y=child.absolute.y*viewport.zoom+viewport.y;
          if(x>host.clientWidth||y>host.clientHeight||x+child.width*viewport.zoom<0||y+child.height*viewport.zoom<0)return false;
          const key=CSS.escape(id);
          const content=host.querySelectorAll(`[data-summary-area="${key}"] .flow-part, [data-frame-title="${key}"]>strong, .react-flow__node[data-id="${key}"] .flow-part`);
          return [...content].some(element=>{
            const style=getComputedStyle(element);
            if(style.visibility==='hidden')return false;
            const box=element.getBoundingClientRect();
            return box.width>0&&box.height>0&&box.right>canvas.left&&box.bottom>canvas.top&&box.left<canvas.right&&box.top<canvas.bottom;
          });
        });
        setFallback(!visibleContent);
      };
      inspect();
      // React Flow can publish newly revealed nodes in the next commit. A pan
      // only translates existing text, so it needs no second DOM inspection.
      const previous=inspected.current;
      inspected.current={zoom:viewport.zoom,detailed,layoutKey};
      if(!previous||previous.zoom!==viewport.zoom||previous.detailed!==detailed||previous.layoutKey!==layoutKey){
        const frame=requestAnimationFrame(inspect);
        return()=>cancelAnimationFrame(frame);
      }
    },[viewport.x,viewport.y,viewport.zoom,open,detailed,layoutKey,intersects]);
    return fallback&&(!open||intersects)?<>
      <ComponentOverview node={node}/><ZoomMark node={node} item={item} enter={enter} select={select} width={host.clientWidth} height={host.clientHeight}/>
    </>:open?<FrameTitle node={node} item={item} focused={focused} enter={enter} select={select}/>:null;
  }
  function App(){
    const [,setVersion]=useState(0);update=()=>setVersion(v=>v+1);
    const state=emphasis(view,hoverArea,leaves,layout.edges);
    const context=focusAncestors(state.focus,placed);
    const visible=id=>!closed(id);
    const drawing=layout;
    const overview=isOverview();
    const area=state.mode==='hover'?parentArea(hoverArea):state.mode==='selection'?parentArea(view.scope):'';
    const number=new Map(area&&view.numbered?leaves(area).map((id,i)=>[id,i+1]):[]);
    const dim=state.mode!=='all';
    const labelGroups=area?connections(area,leaves(area),layout.edges.filter(e=>state.activeEdges.has(e.id)),id=>rootOf(id)===rootOf(area)?id:rootOf(id)):[];
    const labels=labelGroups.map(group=>({...layout.labels.find(l=>l.area===area&&l.key===group.key),...group}));
    useEffect(()=>callbacks.emphasis?.({...state,overview}),[state.mode,state.subject,state.readingOutside,view.scope,overview]);
    const nodes=drawing.nodes.map(n=>{
      const item=byID.get(n.id),focused=state.focus.has(n.id),on=state.participants.has(n.id)||
        (overview&&leaves(n.id).some(id=>state.participants.has(id)));
      const reading=view.scope===n.id||view.operation===n.id;
      const contains=n.frame&&leaves(n.id).some(id=>state.participants.has(id));
      return {...n,type:n.frame?'area':'part',selected:reading,
        selectable:false,draggable:false,connectable:false,
        style:{width:n.width,height:n.height,visibility:visible(n.id)?'visible':'hidden'},
        className:`${on||contains||!dim?'':'flow-node-muted'} ${focused?'flow-node-focus':on?'flow-node-connected':''} ${context.has(n.id)?'flow-node-context':''} ${reading?'flow-node-reading':''}`,
        data:{...item,summarized:n.frame&&scales.has(n.id)&&!detailed.has(n.id),operation:view.operation,reading,number:number.get(n.id),open:(id,event)=>select(id,event,true)}};
    });
    const edges=routeDrawing(drawing.edges,closed,state.activeEdges,dim).map(route=>{
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
        if(labelElement){const label=labels.find(l=>l.id===labelElement.dataset.connectionLabel);if(label&&preview!==label.id){preview=label.id;callbacks.connection(label);}return;}
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
        // A pan checks complete frames at gesture end, without rebuilding the
        // drawing on every position update.
        if(paintedZoom===viewport.zoom)return;
        paintedZoom=viewport.zoom;
        zoom=viewport.zoom;
        host.style.setProperty('--flow-zoom',String(zoom));
        updateDetail(viewport);
        map.querySelectorAll('[data-map-zoom]').forEach(button=>{button.disabled=Number(button.dataset.mapZoom)<1&&viewport.zoom<=minZoom();});
      }}
      onMoveStart={event=>{if(event)overviewFit=false;panning=true;hover.pause();preview='';map.clearMapPreview?.();}}
      onMoveEnd={()=>{panning=false;hover.pause();if(instance)updateDetail(instance.getViewport());updateLocation();map.dispatchEvent(new Event('repomap:viewport'));}}>
      <svg className="flow-defs"><defs>
        <marker id="flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#94a3b8"/></marker>
        <marker id="flow-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#34445b"/></marker>
      </defs></svg>
      <ViewportPortal>
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&!['component','communication','inputs'].includes(byID.get(n.id).branch)&&(componentsOpen||scales.has(n.id))&&(!scales.has(n.id)||detailed.has(n.id))).map(n=><FrameTitle key={n.id} node={n} item={byID.get(n.id)} focused={state.focus.has(n.id)||context.has(n.id)} enter={enter} select={select}/>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&['component','communication','inputs'].includes(byID.get(n.id).branch)).map(n=><ComponentPresentation key={'component-'+n.id} node={n} focused={state.focus.has(n.id)||context.has(n.id)}/>)}
        {drawing.nodes.filter(n=>scales.has(n.id)&&visible(n.id)&&!detailed.has(n.id)).map(n=><div key={'summary-'+n.id}
          className={`flow-area-summary nopan ${state.focus.has(n.id)?'flow-node-focus':''} ${state.participants.has(n.id)?'flow-node-connected':''} ${context.has(n.id)?'flow-node-context':''} ${view.scope===n.id?'flow-node-reading':''}`}
          data-summary-area={n.id} style={{transform:`translate(${n.absolute.x}px,${n.absolute.y}px) scale(${byID.get(n.id).summaryScale||1})`,transformOrigin:'top left',width:n.width/(byID.get(n.id).summaryScale||1),height:n.height/(byID.get(n.id).summaryScale||1)}}
          onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,true);}}>
          <OverviewCard data={{...semantic.summaries.get(n.id),reading:view.scope,operation:view.operation,open:(id,event)=>select(id,event,true)}}/>
        </div>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&scales.has(n.id)&&!detailed.has(n.id)).map(n=><ZoomMark key={'zoom-'+n.id} node={n} item={byID.get(n.id)} enter={enter} select={select}/>)}
        {area&&visible(area)&&detailed.has(area)&&view.numbered&&labels.map(label=><ConnectionLabel key={label.id} label={label}/>)}
      </ViewportPortal>
    </ReactFlow>;
  }
  function ConnectionLabel({label}){
    const {zoom}=useViewport();
    let style={transform:`translate(${label.x}px,${label.y}px) scale(${label.scale||1})`,transformOrigin:'top left',width:label.width/(label.scale||1),minHeight:label.height/(label.scale||1)};
    if(label.boundary){
      const frame=placed.get(label.root),p=label.point;
      const sides=[{dx:-1,dy:-1,tx:-100,ty:-100,d:Math.abs(p.x-frame.absolute.x)},
        {dx:1,dy:-1,tx:0,ty:-100,d:Math.abs(p.x-frame.absolute.x-frame.width)},
        {dx:1,dy:-1,tx:0,ty:-100,d:Math.abs(p.y-frame.absolute.y)},
        {dx:1,dy:1,tx:0,ty:0,d:Math.abs(p.y-frame.absolute.y-frame.height)}];
      const side=sides.sort((a,b)=>a.d-b.d)[0];
      style={transform:`translate(${p.x+side.dx*6/zoom}px,${p.y+side.dy*6/zoom}px) scale(${1/zoom}) translate(${side.tx}%,${side.ty}%)`,transformOrigin:'top left'};
    }
    return <div
          className={`flow-connection-label nopan ${label.boundary?'flow-boundary-label':''}`} data-connection-outside={label.outside} data-connection-label={label.id}
          style={style}
          onMouseEnter={()=>{if(hover.allowed){preview=label.id;callbacks.connection(label);}}}>
          {!label.boundary&&<span>{label.incoming?'← ':'→ '}{label.labelTitle}</span>}
          <button type="button" aria-label={t('Go to {0}',label.title)} onClick={event=>{event.stopPropagation();clearHover();select(label.outside,event,true);}}>
            {label.numbers.join(' · ')}
          </button>
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
    view={...initial,...next};update();
  }};
};
