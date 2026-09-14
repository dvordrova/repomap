import React, {useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow, Handle, Position, ViewportPortal, useViewport} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {emphasis, focusAncestors} from './emphasis.mjs';
import {semanticLayout, detailedAreas, componentContents, componentViewport, closedContainer, readableFocus, frameInventory, visibleRoute, systemViewport} from './semantic.mjs';
import {prepareCards,wrapText,overviewHeading} from './cards.mjs';
import {HoverGate} from './hover.mjs';
import {InputCards, OverviewMembers} from './card-content.jsx';
import '@xyflow/react/dist/style.css';
import './canvas.css';

// Legacy small repository diagrams share the same embedded ELK instance code.
window.ELK = ELK;
const t = (...args) => window.rmT(...args);
function Part({data}) {
  return <div className={`flow-part flow-${data.category} ${data.lane==='core'?'flow-core':''}`} style={data.contentScale&&data.contentScale!==1?{width:data.originalWidth,height:data.originalHeight,transform:`scale(${data.contentScale})`,transformOrigin:'top left'}:undefined}>
    <Handle type="target" position={Position.Top} isConnectable={false}/>
    {(data.kindLabel||data.reading)&&<div className="flow-kind">{data.kindLabel}{data.reading&&<span className="flow-reading-badge">{t('Reading')}</span>}</div>}
    <strong>{data.title}</strong>
    {data.description&&<div className="flow-description">{data.description}</div>}
    {data.subtitle&&<div className="flow-address">{data.subtitle}</div>}
    <InputCards inputs={data.inputs} operation={data.operation} select={data.selectInput}/>
    {data.number && <span className="flow-number">{data.number}</span>}
    <Handle type="source" position={Position.Bottom} isConnectable={false}/>
  </div>;
}
function Area({data}) {
  return <div className={`flow-area ${data.branch==='component'?'flow-component':data.branch==='communication'?'flow-communication':''}`}>
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
function ZoomMark({node,item,enter,select}) {
  const {zoom}=useViewport();
  return <button type="button" className="flow-zoom-mark nopan" data-zoom-into={node.id}
    style={{transform:`translate(${node.absolute.x+node.width-40/zoom}px,${node.absolute.y+12/zoom}px) scale(${1/zoom})`}}
    aria-label={t('Zoom into {0}',item.name||item.title)} onMouseEnter={()=>enter(node.id)}
    onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5M10.5 7.5v6m-3-3h6"/>
    </svg>
  </button>;
}
function FrameTitle({node,item,focused,enter,select}) {
  const viewport=useViewport();
  const component=item.branch==='component',communication=item.branch==='communication';
  const x=component||communication?Math.max(node.absolute.x+18,Math.min(node.absolute.x+node.width-260,(24-viewport.x)/viewport.zoom)):node.absolute.x+18;
  return <div className={`flow-area-title nopan ${focused?'flow-area-title-focus':''} ${component?'flow-component-title':communication?'flow-communication-title':''}`}
    style={{transform:`translate(${x}px,${node.absolute.y+12}px)`,maxWidth:node.width-36}}
    onMouseEnter={()=>enter(node.id)} onClick={event=>{event.stopPropagation();select(node.id,event,true);}}>
    {communication&&<div className="flow-kind">{t('External communication')}</div>}
    <strong>{item.title}</strong>
    {item.metadata&&<div className="flow-component-meta">{item.metadata}</div>}
    {item.role&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
    {item.description&&<p className="flow-description">{item.description}</p>}
  </div>;
}
function RoutedEdge({id,data}) {
  return <g aria-hidden="true" className={`flow-edge ${data.on?'flow-edge-active':''} ${data.dim?'flow-edge-muted':''}`} data-edge-id={id}>
    <path className="flow-edge-casing" d={data.path} vectorEffect="non-scaling-stroke"/>
    <path d={data.path} fill="none" vectorEffect="non-scaling-stroke" strokeDasharray={data.possible?'7 5':undefined} markerEnd={`url(#${data.on?'flow-arrow-active':'flow-arrow'})`}/>
  </g>;
}
const nodeTypes={part:Part,area:Area,overview:OverviewCard}, edgeTypes={routed:RoutedEdge};

window.rmCreateFlow = async function(map, stage, records, relations, areas, inputOwner, callbacks) {
  const source=stage.querySelector('svg'), host=document.createElement('div');
  host.className='flow-root';stage.appendChild(host);
  map.classList.add('flow-enabled');source.style.display='none';source.setAttribute('aria-hidden','true');
  const status=document.createElement('p');status.className='flow-loading';status.textContent=t('Arranging the map…');host.appendChild(status);
  function sizeWorkspace(){
    const workspace=stage.closest('.map-workspace');if(!workspace)return;
    map.style.setProperty('--flow-top',`${workspace.getBoundingClientRect().top+window.scrollY}px`);
  }
  const sizeObserver=new ResizeObserver(sizeWorkspace);
  [document.querySelector('.report-toolbar'),map.querySelector('.system-controls'),map.querySelector('.system-selection')].filter(Boolean).forEach(n=>sizeObserver.observe(n));
  window.addEventListener('resize',sizeWorkspace);requestAnimationFrame(sizeWorkspace);
  const context=document.createElement('canvas').getContext('2d');
  const measure=(text,font)=>{context.font=font;return context.measureText(text).width;};
  const items=prepareCards(records,inputOwner,measure,t);
  let semantic;
  try{semantic=await semanticLayout(items,relations,areas,stage.clientWidth||1200,stage.clientHeight||700);}catch(error){host.remove();map.classList.remove('flow-enabled');source.style.display='';source.removeAttribute('aria-hidden');throw error;}
  const {layout,scales}=semantic;
  const byID=new Map(semantic.records.map(n=>[n.id,n])), placed=new Map(layout.nodes.map(n=>[n.id,n]));
  const geometry=JSON.stringify(layout.nodes.map(n=>[n.id,n.absolute.x,n.absolute.y,n.width,n.height]));
  let geometryHash=0;for(let i=0;i<geometry.length;i++)geometryHash=(Math.imul(31,geometryHash)+geometry.charCodeAt(i))|0;
  const layoutKey=String(geometryHash);
  const initial={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false,numbered:true};
  let cameraRevision=0;
  let update, instance, view=initial, hoverArea='', preview='', restorePending, pendingFocus, panning=false, initializing=true;
  let detailed=new Set(),locationID='',componentsOpen=false,zoom=systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom,overviewFit=false;
  new ResizeObserver(()=>{if(instance&&!initializing&&overviewFit)fitOverview();}).observe(host);
  const location=document.createElement('div');location.className='flow-location';location.setAttribute('aria-live','polite');stage.insertBefore(location,host);
  function updateLocation(){
    if(!instance)return;
    if(!componentsOpen){location.textContent=t('System map');return;}
    const v=instance.getViewport(),w=host.clientWidth,h=host.clientHeight;
    const point={x:(w/2-v.x)/v.zoom,y:(h/2-v.y)/v.zoom};
    const candidates=layout.nodes.filter(n=>!closedContainer(n.id,placed,byID,detailed,componentsOpen)&&(scales.has(n.id)||byID.get(n.id)?.branch==='component'||byID.get(n.id)?.branch==='communication'||(!n.frame&&!semantic.owner(n.id)))).map(n=>{
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
  const maxZoom=Math.max(2,...[...scales.values()].map(s=>1.8/s));
  const minZoom=()=>Math.min(.15,systemViewport(layout.nodes,host.clientWidth,host.clientHeight).zoom);
  const hover=new HoverGate();
  const remember=event=>hover.remember(event.clientX,event.clientY);
  document.addEventListener('pointermove',remember,{passive:true});
  document.addEventListener('pointerdown',remember,{passive:true});
  const children=new Map(areas.map(a=>[a.id,a.nodes]));
  const inventories=new Map(areas.map(a=>[a.id,frameInventory(a.id,children,byID)]));
  const leaves=id=>children.has(id)?children.get(id).flatMap(leaves):[id];
  function parentArea(id){while(id){if(byID.get(id)?.branch==='area')return id;id=placed.get(id)?.parentId;}return '';}
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
    const n=placed.get(inputOwner[id]||id);if(!n)return;
    if(!instance||initializing){pendingFocus={id,center};return;}
    overviewFit=false;
    hover.pause();preview='';map.clearMapPreview?.();
    const {x,y}=n.absolute, viewport=instance.getViewport(), rect=host.getBoundingClientRect();
    const contentScale=byID.get(n.id)?.contentScale||1;
    const input=inputOwner[id]&&host.querySelector(`[data-input-id="${CSS.escape(id)}"]`);
    if(input){const box=input.getBoundingClientRect(),point=instance.screenToFlowPosition({x:box.x+box.width/2,y:box.y+box.height/2});commitCamera(instance.setCenter(point.x,point.y,{zoom:1/contentScale,duration:smooth?420:0}));return;}
    if(!center&&readableFocus(n.id,placed,byID,detailed,componentsOpen,viewport,rect.width,rect.height))return;
    if(n.frame){
      if(['component','communication'].includes(byID.get(n.id).branch)){
        commitCamera(instance.setViewport(componentViewport(n,layout.nodes,rect.width),{duration:smooth?420:0}));return;
      }
      const zoom=scales.has(n.id)?.8/contentScale:Math.min(1,Math.max(.6,Math.min((rect.width-48)/n.width,(rect.height-48)/n.height)));
      commitCamera(instance.setViewport({x:24-x*zoom,y:24-y*zoom,zoom},{duration:smooth?420:0}));return;
    }
    const zoom=1/contentScale;
    commitCamera(instance.setCenter(x+n.width/2,y+Math.min(n.height/2,rect.height/(2*zoom)-24),{zoom,duration:smooth?420:0}));
  }
  function capture(){return instance?{...instance.getViewport(),layoutKey,overview:isOverview(),detailAreas:[...detailed],componentsOpen,fit:overviewFit}:restorePending||null;}
  function restore(v){if(!v)return;
    if(!instance||initializing){restorePending=v;return;}
    // A regenerated report can have different geometry. Old screen coordinates
    // must not place its still-selected item outside the visible viewport.
    if(!Number.isFinite(v.zoom)||v.layoutKey!==layoutKey){focus(view.scope||view.operation,true);return;}
    if(v.fit===true||(v.fit===undefined&&v.componentsOpen===false&&!view.scope&&!view.operation)){fitOverview();return;}
    overviewFit=false;
    detailed=v.detailAreas?new Set(v.detailAreas.filter(id=>scales.has(id))):detailedAreas(scales,v.zoom);
    componentsOpen=componentContents(v.zoom,v.componentsOpen);zoom=v.zoom;update?.();
    hover.pause();preview='';map.clearMapPreview?.();
    if(instance)commitCamera(instance.setViewport(v));else restorePending=v;
  }
  function resetView(){if(!instance)return;if(isOverview()){fitOverview();return;}const first=layout.nodes.filter(n=>!n.frame).sort((a,b)=>a.absolute.y-b.absolute.y||a.absolute.x-b.absolute.x)[0];focus(view.scope||view.operation||first?.id,true);}
  async function fitOverview(duration=0){
    overviewFit=true;
    detailed=new Set();componentsOpen=false;hoverArea='';update?.();
    const v=systemViewport(layout.nodes,host.clientWidth,host.clientHeight);
    hover.pause();
    await commitCamera(instance.setViewport(v,{duration}));
  }
  function select(id,event,center=false){
    hover.remember(event.clientX,event.clientY);hover.pause();hoverArea='';preview='';map.clearMapPreview?.();callbacks.select(id,center);
  }
  function App(){
    const [,setVersion]=useState(0);update=()=>setVersion(v=>v+1);
    const state=emphasis(view,hoverArea,leaves,layout.edges);
    const context=focusAncestors(state.focus,placed);
    const closed=id=>closedContainer(id,placed,byID,detailed,componentsOpen);
    const visible=id=>!closed(id);
    const drawing=layout;
    const overview=isOverview();
    const area=state.mode==='hover'?parentArea(hoverArea):state.mode==='selection'?parentArea(view.scope):'';
    const number=new Map(area&&view.numbered?leaves(area).map((id,i)=>[id,i+1]):[]);
    const dim=state.mode!=='all';
    const labelGroups=area?connections(area,leaves(area),layout.edges.filter(e=>state.activeEdges.has(e.id))):[];
    const labels=labelGroups.map(group=>({...layout.labels.find(l=>l.area===area&&l.key===group.key),...group}));
    useEffect(()=>callbacks.emphasis?.({...state,overview}),[state.mode,state.subject,state.readingOutside,view.scope,overview]);
    const nodes=drawing.nodes.map(n=>{
      const item=byID.get(n.id),focused=state.focus.has(n.id),on=state.participants.has(n.id)||
        (overview&&leaves(n.id).some(id=>state.participants.has(id)));
      const reading=view.scope===n.id;
      const contains=n.frame&&leaves(n.id).some(id=>state.participants.has(id));
      return {...n,type:n.frame?'area':'part',selected:view.scope===n.id,
        selectable:false,draggable:false,connectable:false,
        style:{width:n.width,height:n.height,visibility:visible(n.id)?'visible':'hidden'},
        className:`${on||contains||!dim?'':'flow-node-muted'} ${focused?'flow-node-focus':on?'flow-node-connected':''} ${context.has(n.id)?'flow-node-context':''} ${reading?'flow-node-reading':''}`,
        data:{...item,operation:view.operation,reading,number:number.get(n.id),open:(id,event)=>select(id,event,true),
          selectInput:(id,event)=>select(id,event,false)}};
    });
    const edges=drawing.edges.map(e=>{
      const on=state.activeEdges.has(e.id);
      const path=visibleRoute(e,closed(e.from),closed(e.to));
      return {id:e.id,source:e.from,target:e.to,type:'routed',selectable:false,focusable:false,hidden:!path,
        ariaLabel:'',domAttributes:{'aria-hidden':true},
        data:{path,possible:e.possible,on,dim:dim&&!on}};
    });
    return <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes}
      nodesDraggable={false} nodesConnectable={false} elementsSelectable={false}
      nodesFocusable={false} edgesFocusable={false} disableKeyboardA11y
      deleteKeyCode={null} selectionKeyCode={null} multiSelectionKeyCode={null} panActivationKeyCode={null} zoomActivationKeyCode={null}
      zoomOnDoubleClick={false} minZoom={minZoom()} maxZoom={maxZoom} panOnScroll preventScrolling
      onInit={flow=>{instance=flow;fitOverview().then(()=>{
        initializing=false;
        updateLocation();
        if(restorePending)restore(restorePending);
        else if(pendingFocus)focus(pendingFocus.id,pendingFocus.center);
        else if(view.scope||view.operation)focus(view.scope||view.operation,true);
        else map.dispatchEvent(new Event('repomap:viewport'));
      });}}
      onNodeClick={(event,n)=>{event.stopPropagation();select(n.id,event,false);}}
      onNodeMouseEnter={(_,n)=>enter(n.id)}
      onMouseMove={event=>{
        if(panning||!instance)return;
        if(!hover.move(event.clientX,event.clientY))return;
        const labelElement=event.target.closest('.flow-connection-label');
        if(labelElement){const label=labels.find(l=>l.id===labelElement.dataset.connectionLabel);if(label&&preview!==label.id){preview=label.id;callbacks.connection(label);}return;}
        const input=event.target.closest('[data-input-id]');
        if(input){enter(inputOwner[input.dataset.inputId]||input.dataset.inputId);return;}
        const node=event.target.closest('.react-flow__node');
        if(node){enter(node.dataset.id);return;}
        const point=instance.screenToFlowPosition({x:event.clientX,y:event.clientY});
        const area=drawing.nodes.filter(n=>n.frame&&visible(n.id)&&point.x>=n.absolute.x&&point.x<=n.absolute.x+n.width&&point.y>=n.absolute.y&&point.y<=n.absolute.y+n.height)
          .sort((a,b)=>a.width*a.height-b.width*b.height)[0];
        if(area)enter(area.id);
      }}
      onPaneClick={event=>{
        if(!componentsOpen&&instance){
          const p=instance.screenToFlowPosition({x:event.clientX,y:event.clientY});
          const frame=drawing.nodes.find(n=>n.frame&&visible(n.id)&&p.x>=n.absolute.x&&p.x<=n.absolute.x+n.width&&p.y>=n.absolute.y&&p.y<=n.absolute.y+n.height);
          if(frame){select(frame.id,event,true);return;}
        }
        clearHover();
      }}
      onMove={(_,viewport)=>{
        zoom=viewport.zoom;
        host.style.setProperty('--flow-zoom',String(zoom));
        const open=componentContents(zoom,componentsOpen);
        const next=detailedAreas(scales,viewport.zoom,detailed);
        const changed=open!==componentsOpen||next.size!==detailed.size||[...next].some(id=>!detailed.has(id));
        componentsOpen=open;detailed=next;
        if(changed){hoverArea='';hover.pause();}
        if(changed||!componentsOpen)update?.();
        map.querySelectorAll('[data-map-zoom]').forEach(button=>{button.disabled=Number(button.dataset.mapZoom)<1&&viewport.zoom<=minZoom();});
      }}
      onMoveStart={event=>{if(event)overviewFit=false;panning=true;hover.pause();preview='';map.clearMapPreview?.();}}
      onMoveEnd={()=>{panning=false;hover.pause();updateLocation();map.dispatchEvent(new Event('repomap:viewport'));}}>
      <svg className="flow-defs"><defs>
        <marker id="flow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#94a3b8"/></marker>
        <marker id="flow-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#34445b"/></marker>
      </defs></svg>
      <ViewportPortal>
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&(componentsOpen||scales.has(n.id))&&(!scales.has(n.id)||detailed.has(n.id))).map(n=><FrameTitle key={n.id} node={n} item={byID.get(n.id)} focused={state.focus.has(n.id)||context.has(n.id)} enter={enter} select={select}/>)}
        {!componentsOpen&&drawing.nodes.filter(n=>n.frame&&visible(n.id)&&['component','communication'].includes(byID.get(n.id).branch)).map(n=>{
          const viewport=instance?.getViewport()||{x:0,y:0};
          const left=Math.max(0,n.absolute.x*zoom+viewport.x),right=Math.min(host.clientWidth,(n.absolute.x+n.width)*zoom+viewport.x);
          const item=byID.get(n.id),inventory=inventories.get(n.id),screenWidth=right-left,screenHeight=n.height*zoom;
          if(screenWidth<=32)return null;
          const heading=overviewHeading(item,screenWidth,measure);
          const communication=item.branch==='communication',scale=1/zoom,width=Math.min(320,screenWidth-16);
          const contentWidth=Math.max(1,width-16),areaIDs=communication?[]:inventory.areaIDs;
          const textHeight=(text,font,lineHeight,w=contentWidth)=>wrapText(text,w,font,measure).length*lineHeight;
          const listHeight=areaIDs.length?7+areaIDs.reduce((h,id)=>h+10+textHeight(byID.get(id).name||byID.get(id).title,'500 13px system-ui',18),0):0;
          const counts=[inventory.parts?t('{0} parts',inventory.parts):'',inventory.inputs?t('{0} inputs',inventory.inputs):''].filter(Boolean).join(' · ');
          let remaining=screenHeight-32-heading.height-listHeight-(areaIDs.length?10:0);
          const listOverflow=remaining<0;
          const roleHeight=item.role?textHeight(item.role,'600 13px system-ui',18)+10:0;
          const showRole=!communication&&roleHeight>0&&remaining>=roleHeight;
          if(showRole)remaining-=roleHeight;
          const countsHeight=textHeight(counts,'500 12px system-ui',17)+10;
          const showCounts=!communication&&remaining>=countsHeight;
          if(showCounts)remaining-=countsHeight;
          const descriptionLines=Math.floor((remaining-10)/18);
          // Screen-sized summaries must stay in the visible part of their own
          // frame while approaching its contents. Their world corner can leave
          // the viewport long before the interior reaches readable scale.
          const x=(left+8-viewport.x)/zoom;
          const y=Math.max(n.absolute.y+8/zoom,Math.min(n.absolute.y+n.height-(heading.height+24)/zoom,(40-viewport.y)/zoom));
          return <div key={'component-'+n.id} className={`flow-component-overview nopan ${item.branch==='communication'?'flow-communication-overview':''}`}
            data-component-overview={n.id} style={{transform:`translate(${x}px,${y}px) scale(${scale})`,width,maxHeight:(n.absolute.y+n.height-y)*zoom-8}}
            onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,true);}}>
            <div className="flow-component-overview-heading" style={{maxWidth:heading.width,paddingTop:heading.clearZoom?32:undefined}}><strong>{item.name}</strong></div>
            {showRole&&<div className="flow-component-role" data-display-ref={item.roleRef}>{item.role}</div>}
            {!communication&&descriptionLines>=2&&item.description&&<p className="flow-description flow-description-compact" style={{WebkitLineClamp:descriptionLines}}>{item.description.replace(/\n/g,' ')}</p>}
            {areaIDs.length>0&&<ul className={`flow-component-areas ${listOverflow?'nowheel':''}`}>{areaIDs.map(id=><li key={id}>
              <button type="button" className="nopan" data-overview-area={id} onClick={event=>{event.stopPropagation();select(id,event,true);}}>{byID.get(id).name||byID.get(id).title}</button>
            </li>)}</ul>}
            {showCounts&&<div className="flow-inside-counts">{counts}</div>}
          </div>;
        })}
        {drawing.nodes.filter(n=>scales.has(n.id)&&visible(n.id)&&!detailed.has(n.id)).map(n=><div key={'summary-'+n.id}
          className={`flow-area-summary nopan ${state.focus.has(n.id)?'flow-node-focus':''} ${state.participants.has(n.id)?'flow-node-connected':''}`}
          data-summary-area={n.id} style={{transform:`translate(${n.absolute.x}px,${n.absolute.y}px)`,width:n.width,height:n.height}}
          onMouseEnter={()=>enter(n.id)} onClick={event=>{event.stopPropagation();select(n.id,event,true);}}>
          <OverviewCard data={{...semantic.summaries.get(n.id),reading:view.scope,operation:view.operation,open:(id,event)=>select(id,event,true)}}/>
        </div>)}
        {drawing.nodes.filter(n=>n.frame&&visible(n.id)&&(
          (!componentsOpen&&['component','communication'].includes(byID.get(n.id).branch))||
          (scales.has(n.id)&&!detailed.has(n.id))
        )).map(n=><ZoomMark key={'zoom-'+n.id} node={n} item={byID.get(n.id)} enter={enter} select={select}/>)}
        {area&&visible(area)&&detailed.has(area)&&view.numbered&&labels.map(label=><div key={label.id}
          className="flow-connection-label nopan" data-connection-outside={label.outside} data-connection-label={label.id}
          style={{transform:`translate(${label.x}px,${label.y}px) scale(${label.scale||1})`,transformOrigin:'top left',width:label.width/(label.scale||1),minHeight:label.height/(label.scale||1)}}
          onMouseEnter={()=>{if(hover.allowed){preview=label.id;callbacks.connection(label);}}}>
          <span>{label.incoming?'← ':'→ '}{label.labelTitle}</span>
          <button type="button" aria-label={t('Go to {0}',label.title)} onClick={event=>{event.stopPropagation();clearHover();select(label.outside,event,true);}}>
            {label.numbers.join(' · ')}
          </button>
        </div>)}
      </ViewportPortal>
    </ReactFlow>;
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
  return {layout,focus,capture,restore,clearHover,overview:()=>fitOverview(420),update(next){
    if(view.scope!==next.scope||view.operation!==next.operation){hover.pause();preview='';map.clearMapPreview?.();}
    view={...initial,...next};update();
  }};
};
