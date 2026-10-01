// The scene canvas (PLAN, the new path behind `?scene=1`): the page's data
// is built into a model once (model.mjs), every level is laid out once per
// canvas size (levels.mjs), and what is drawn is a pure function of the
// level and the reader's choice (scene.mjs). React Flow is the camera and a
// dumb renderer; one store holds the level, the pointer and the choice; one
// hitTest answers hover and click; the overlay keeps markers and ports at
// one screen size (overlay.mjs).
import React,{useEffect,useLayoutEffect,useMemo,useRef,useState,useSyncExternalStore} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow,Handle,Position} from '@xyflow/react';
import {buildModel,inputKindTitles} from './model.mjs';
import {layoutLevels,homeCamera,units} from './levels.mjs';
import {sceneAt,emphasisOf,hitTest,chainOf,levelKey,bands,edgeByID,frameGroups,connectionOf,levelAfterZoom,pinchLimit} from './scene.mjs';
import {project,mark} from './overlay.mjs';
import {createStore,sceneReducer,initialState,createCamera,zoomAction} from './store.mjs';
import {wrapText,descriptionLines} from './cards.mjs';
import {kindIcon,kindNames,systemIcons} from './kind-icons.mjs';
import {callCard} from './call-card.mjs';
import {BriefRows,FrameConnections} from './call-card-view.jsx';
import {PartSymbols} from './part-symbols.jsx';
import {createLook} from './look.mjs';
import {placeCard} from './card-place.mjs';
import './scene.css';

const t=(...args)=>window.rmT?window.rmT(...args):args[0];
const context2d=document.createElement('canvas').getContext('2d');
const measure=(text,font)=>{context2d.font=font;return context2d.measureText(String(text??'')).width;};
const sameSet=(a,b)=>a.size===b.size&&[...a].every(id=>b.has(id));

// The words a box shows at its level's text size: its title in whole
// lines and as many whole lines of its description as stand under it.
function cardWords(node,{titleFont='700 17px system-ui',titleLine=21.25,pad=14,room=0,description=''}={}){
  const width=node.rect.width/node.text-2*pad-room,height=node.rect.height/node.text-2*pad;
  if(width<=8||height<=8)return {title:[],lines:[]};
  const title=wrapText(node.title,width,titleFont,measure);
  const most=Math.max(0,Math.floor((height-title.length*titleLine-6)/18));
  return {title,lines:description?descriptionLines(description,width,Math.min(most,4),measure,'13px system-ui'):[]};
}
function Mark({icon,className='',label=''}){
  if(!icon)return <span className={`scene-dot ${className}`} aria-hidden={label?undefined:'true'}/>;
  return <svg className={`flow-kind-mark ${className}`} viewBox="0 0 16 16" width="14" height="14" aria-hidden={label?undefined:'true'} role={label?'img':undefined} aria-label={label||undefined}>
    {icon.paths.map((d,i)=><path key={i} d={d}/>)}</svg>;
}
const laneClass=lane=>lane==='core'?'flow-core':lane==='triggers'?'flow-entry':'';
// A box drawn at its level's text size: its world rectangle holds its
// content laid out at `text` world units to a pixel.
function Scaled({node,className='',children,style}){
  const {rect,text}=node;
  return <div className={`scene-scaled ${className}`} style={{width:rect.width/text,height:rect.height/text,transform:`scale(${text})`,...style}}>{children}</div>;
}
const handles=<><Handle type="target" position={Position.Top} isConnectable={false}/><Handle type="source" position={Position.Bottom} isConnectable={false}/></>;

function CardNode({data}){
  const {node}=data,item=data.item;
  const words=useMemo(()=>cardWords(node,{description:item?.summary||'',room:node.enter?24:0}),[node.rect.width,node.rect.height,node.text,node.title]);
  return <>{handles}<Scaled node={node} className={`flow-part ${node.display==='area'?'scene-area-card':''} ${laneClass(node.lane)}`}>
    {node.ghosts&&<svg className="scene-ghosts" width={node.rect.width/node.text} height={node.rect.height/node.text} aria-hidden="true">
      {node.ghosts.map((r,i)=><rect key={i} x={(r.x-node.rect.x)/node.text} y={(r.y-node.rect.y)/node.text} width={r.width/node.text} height={r.height/node.text} rx={9*r.width/node.text/260}/>)}</svg>}
    {node.display==='area'&&['core','triggers'].includes(node.lane)&&<span className={`flow-role-symbol flow-role-${node.lane}`} aria-hidden="true"/>}
    <div className="scene-words"><strong data-box-title={node.id}>{words.title.join('\n')}</strong>
    {words.lines.length>0&&<div className="flow-description flow-description-lines" title={item?.summary||undefined}>{words.lines.join('\n')}</div>}</div>
  </Scaled></>;
}
function ProgramNode({data}){
  const {node}=data,item=data.item,c=units.programCard;
  const words=useMemo(()=>{
    const inner=node.rect.width/node.text-2*c.pad;
    const title=wrapText(node.title,inner,c.font,measure);
    const role=item?.role?descriptionLines(item.role,inner,2,measure,c.role):[];
    const room=node.rect.height/node.text-2*c.pad-title.length*c.line-role.length*c.textLine-12;
    return {title,role,purpose:item?.summary?descriptionLines(item.summary,inner,Math.max(0,Math.min(2,Math.floor(room/c.textLine))),measure,c.text):[]};
  },[node.rect.width,node.rect.height,node.title]);
  return <>{handles}<Scaled node={node} className="scene-program-card">
    <strong data-box-title={node.id}>{words.title.join('\n')}</strong>
    {words.role.length>0&&<div className="flow-component-role" title={item.role}>{words.role.join('\n')}</div>}
    {words.purpose.length>0&&<p className="flow-description flow-description-lines" title={item.summary}>{words.purpose.join('\n')}</p>}
  </Scaled></>;
}
function FrameNode({data}){
  const {node}=data,kind=node.kind;
  const tone=kind==='program'?'flow-component':kind==='outside'||kind==='bucket'?'flow-communication':kind==='inputs'?'flow-input-collection'
    :node.lane==='core'?'flow-area-core':node.lane==='triggers'?'flow-area-entry':'';
  const title=kind==='outside'?t('Outside'):kind==='inputs'?t('Inputs'):node.title;
  return <>{handles}<div className={`flow-area ${tone}`} style={{width:node.rect.width,height:node.rect.height}}>
    <Scaled node={{...node,text:node.titleText||node.text,rect:{...node.rect,height:units.band(1)*(node.titleText||node.text)}}} className="scene-frame-title">
      <strong data-box-title={node.id}>{title}</strong></Scaled>
  </div></>;
}
// An Inputs frame closed: its title over its kinds' marks, each named on
// hover; entered, the kinds' groups name their inputs.
function InputsNode({data}){
  const {node,groups}=data;
  return <>{handles}<Scaled node={node} className="flow-area flow-input-collection scene-inputs-card">
    <strong data-box-title={node.id}>{t('Inputs')}</strong>
    {node.kinds.map((entry,i)=><span key={entry.kind} className={`scene-kind-mark ${groups[i]?.lit?'flow-lit':''}`} title={t(inputKindTitles[entry.kind])}
      data-input-group-kind={entry.kind} style={{left:(entry.rect.x-node.rect.x)/node.text,top:(entry.rect.y-node.rect.y)/node.text,width:entry.rect.width/node.text,height:entry.rect.height/node.text}}>
      <Mark icon={kindIcon(entry.kind)}/></span>)}
  </Scaled></>;
}
function GroupNode({data}){
  const {node}=data;
  return <>{handles}<div className="scene-group" style={{width:node.rect.width,height:node.rect.height}}>
    <Scaled node={{...node,rect:{...node.rect,height:units.band(.8)*node.text}}} className="scene-group-title">
      <Mark icon={kindIcon(node.inputKind)}/><strong>{node.title}</strong></Scaled></div></>;
}
function TileNode({data}){
  const {node}=data;
  return <>{handles}<Scaled node={node} className={`scene-tile ${data.lit?'flow-lit':''}`}><Mark icon={kindIcon(node.inputKind)}/><span>{node.title}</span></Scaled></>;
}
function ChipNode({data}){
  const {node}=data;
  return <>{handles}<Scaled node={node} className={`flow-chip scene-chip ${data.item?.unestablished?'flow-chip-unestablished':''}`}>
    <Mark icon={systemIcons[node.systemKind]}/><span className="flow-chip-name" title={node.title}>{node.title}</span></Scaled></>;
}
// A closed bucket: its part's name over its systems' marks, never blank.
function BucketNode({data}){
  const {node}=data;
  return <>{handles}<Scaled node={node} className="flow-chip scene-bucket">
    <span className="flow-chip-name" title={node.title}>{node.title}</span>
    <span className="scene-bucket-marks">{node.kinds.map(kind=><Mark key={kind} icon={systemIcons[kind]}/>)}</span></Scaled></>;
}
function NoteNode({data}){
  const {node}=data,item=data.item;
  return <>{handles}<Scaled node={node} className="flow-part scene-note"><strong data-box-title={node.id}>{node.title}</strong>
    {item?.summary&&<div className="flow-description">{item.summary}</div>}</Scaled></>;
}
// A part entered: its declarations as tiles in its card (part-symbols.jsx),
// the one pointed at and the one chosen marked.
function DeepNode({data}){
  const {node,item,drawn,member}=data;
  if(!drawn)return <CardNode data={data}/>;
  const {grid,box}=drawn,s=node.rect.width/box.width;
  return <>{handles}<div className={`flow-part flow-part-deep scene-deep ${laneClass(node.lane)}`} style={{width:box.width,height:box.height,transform:`scale(${s})`}}>
    <strong data-box-title={node.id} style={{fontSize:28/grid.divisor,lineHeight:`${40/grid.divisor}px`,padding:`${20/grid.divisor}px ${32/grid.divisor}px 0`}}>{node.title}</strong>
    <PartSymbols symbols={item.symbols} calls={item.symbolCalls} width={box.width} height={box.height} grid={grid} member={member}/>
  </div></>;
}
const nodeTypes={card:CardNode,area:CardNode,deep:DeepNode,program:ProgramNode,frame:FrameNode,inputs:InputsNode,kindgroup:GroupNode,tile:TileNode,chip:ChipNode,bucket:BucketNode,note:NoteNode};

const arrowHead=7;
function SceneEdge({id,data}){
  const d=data.points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ');
  const head=data.on?'url(#scene-arrow-active)':'url(#scene-arrow)';
  const heads=data.heads.end&&data.heads.start?'both':data.heads.end?'end':data.heads.start?'start':'none';
  return <g className={`flow-edge ${data.on?'flow-edge-active':''} ${data.dim?'flow-edge-muted':''} ${data.faint?'flow-edge-faint':''}`} data-edge-id={id}
    data-edge-ends={`${data.from} ${data.to}`} data-edge-head={heads} data-edge-dark={data.on?'':undefined}>
    <path className="flow-edge-hit" d={d} data-edge-hit={id}/>
    <path className="flow-edge-casing" d={d}/>
    <path d={d} data-edge-line="" style={data.possible?{strokeDasharray:'calc(7px / var(--flow-zoom, 1)) calc(5px / var(--flow-zoom, 1))'}:undefined}
      markerEnd={data.heads.end?head:undefined} markerStart={data.heads.start?head:undefined}/>
  </g>;
}
const edgeTypes={scene:SceneEdge};

// `map`, `stage`, `records`, `relations`, `areas`, `inputOwner` and
// `callbacks` are rmCreateFlow's; the result is the same surface.
export async function createSceneFlow(map,stage,records,relations,areas,inputOwner,callbacks){
  const source=stage.querySelector('svg'),host=document.createElement('div');
  host.className='flow-root scene-root';stage.appendChild(host);
  map.classList.add('flow-enabled','flow-initializing');map.dataset.scene='';
  if(source){source.style.display='none';source.setAttribute('aria-hidden','true');}
  const location=document.createElement('div');location.className='flow-location';location.setAttribute('aria-live','polite');stage.insertBefore(location,host);
  const status=document.createElement('p');status.className='flow-loading';status.textContent=t('Arranging the map…');stage.appendChild(status);
  function sizeWorkspace(){
    const workspace=stage.closest('.map-workspace');if(!workspace)return;
    map.style.setProperty('--flow-top',`${workspace.getBoundingClientRect().top+window.scrollY}px`);
  }
  window.addEventListener('resize',sizeWorkspace);sizeWorkspace();

  const model=buildModel({items:records,relations,areas,inputOwner},{measure,t});
  const size=()=>({width:host.clientWidth||1200,height:host.clientHeight||700});
  let geometry;
  try{geometry=await layoutLevels(model,{...size(),measure});}
  catch(error){host.remove();status.remove();location.remove();map.classList.remove('flow-enabled','flow-initializing');if(source){source.style.display='';source.removeAttribute('aria-hidden');}throw error;}
  let layoutKey=String(Math.round(geometry.bounds.width))+'x'+String(Math.round(geometry.bounds.height));

  // The scene of the current level, kept until the level, the choice or
  // the geometry changes.
  let sceneCache={key:'',scene:null};
  const sceneOf=state=>{
    const key=`${levelKey(state.level)}|${state.view.scope}|${state.version}`;
    if(sceneCache.key!==key)sceneCache={key,scene:sceneAt(model,geometry,state.level,{scope:state.view.scope})};
    return sceneCache.scene;
  };
  const store=createStore(sceneReducer({model,geometry:()=>geometry,scene:()=>sceneOf(store.getState())}),initialState);
  const camera=createCamera(geometry.home);
  let instance=null,initializing=true,overviewFit=true;
  const look=createLook();
  let lookTimer,handleRect=null;
  const pump=()=>{
    clearTimeout(lookTimer);
    const due=look.due;if(!due)return;
    lookTimer=setTimeout(()=>{if(look.tick(performance.now()))store.dispatch({type:'look',key:look.key});pump();},Math.max(0,due-performance.now())+5);
  };

  const nameOf=id=>{
    const node=model.nodes.get(id);
    if(node?.kind==='inputs')return [t('Inputs'),model.nodes.get(node.program)?.name].filter(Boolean).join(' · ');
    if(node?.kind==='outside')return [t('Outside'),node.name].filter(Boolean).join(' · ');
    if(String(id).startsWith('port:'))return model.nodes.get(String(id).split(':')[3])?.name||'';
    return node?.name||'';
  };
  // A frame's connections as the reading column reads them, each with the
  // calls its card lists (scene.mjs frameGroups).
  function frameConnections(id){
    return frameGroups(model,id)
      .map(group=>({...group,title:nameOf(group.outside),card:callCard(group.relations,{nameOf,incoming:group.incoming,groupable:other=>model.nodes.get(other)?.kind!=='input'})}))
      .filter(group=>group.card.total).sort((a,b)=>(a.incoming?0:1)-(b.incoming?0:1)||b.card.total-a.card.total||a.title.localeCompare(b.title));
  }
  // The camera: a level's frame whole at its reading size, or its first
  // corner when it is larger than the canvas.
  function frameCamera(rect,minZoom=0,pad=28){
    const {width,height}=size(),fit=Math.min((width-2*pad)/rect.width,(height-2*pad)/rect.height);
    const zoom=Math.max(fit,minZoom);
    const x=rect.width*zoom<=width-2*pad?(width-rect.width*zoom)/2-rect.x*zoom:pad-rect.x*zoom;
    const y=rect.height*zoom<=height-2*pad?(height-rect.height*zoom)/2-rect.y*zoom:pad-rect.y*zoom;
    return {x,y,zoom};
  }
  function moveCamera(view,smooth=true){
    if(!instance)return Promise.resolve();
    overviewFit=false;
    return Promise.resolve(instance.setViewport(view,{duration:smooth?420:0})).then(()=>{camera.set(instance.getViewport());map.dispatchEvent(new Event('repomap:viewport'));});
  }
  // The camera a level is entered at: no smaller than its entry zoom; a
  // part entered at the zoom its declarations read at (eleven pixels), its
  // head and first column in sight when it is larger than the canvas.
  const levelZoom=level=>{
    const inner=level.at(-1);if(!inner)return 0;
    const entry=(geometry.enterZoom.get(inner)||0)*1.02,drawn=geometry.grids.get(inner),rect=geometry.boxes.get(inner);
    if(model.nodes.get(inner)?.kind!=='part'||!drawn||!rect)return entry;
    return Math.max(entry,11/13*drawn.grid.divisor*drawn.box.width/rect.width);
  };
  function enter(level,{rect=null,smooth=true}={}){
    store.dispatch({type:'enter',level});
    const scene=sceneOf(store.getState());
    return moveCamera(frameCamera(rect||scene.focus,levelZoom(level)),smooth);
  }
  function fitOverview(duration=0){
    overviewFit=true;
    store.dispatch({type:'enter',level:[]});
    if(!instance)return Promise.resolve();
    return Promise.resolve(instance.setViewport(geometry.home,{duration})).then(()=>{camera.set(instance.getViewport());map.dispatchEvent(new Event('repomap:viewport'));});
  }
  // Whether a world rectangle is wholly in sight.
  function inSight(rect,margin=16){
    const v=camera.get(),{width,height}=size();
    return rect.x*v.zoom+v.x>=margin&&rect.y*v.zoom+v.y>=margin&&(rect.x+rect.width)*v.zoom+v.x<=width-margin&&(rect.y+rect.height)*v.zoom+v.y<=height-margin;
  }
  // Show a thing the reading names: the level holding it, the thing framed.
  function focus(id,center=true,smooth=true){
    id=model.shown(id);
    const node=model.nodes.get(id);if(!node)return;
    if(initializing||!instance){pending={id,center};return;}
    if(node.kind==='input'){
      const trace=[...new Set(node.item?.trace||[])].filter(part=>model.nodes.get(part)?.kind==='part'&&geometry.boxes.has(part));
      if(trace.length){
        const program=model.programOf(trace[0]),areasOf=new Set(trace.map(part=>model.parent(part)));
        const level=areasOf.size===1&&model.nodes.get([...areasOf][0])?.kind==='area'?chainOf(model,[...areasOf][0]):[program];
        const rect=bounds(trace.map(part=>geometry.boxes.get(part)));
        return enter(level,{rect,smooth});
      }
      return showInput(id,smooth);
    }
    const level=node.kind==='part'?chainOf(model,id).filter(at=>at!==id):enterLevelOf(id);
    const rect=geometry.boxes.get(id)||geometry.scales.get(id)?.frame;
    const current=store.getState().level;
    if(!center&&levelKey(current)===levelKey(level)&&rect&&inSight(rect))return;
    store.dispatch({type:'enter',level});
    if(rect)moveCamera(frameCamera(rect,levelZoom(level)),smooth);
  }
  // A frame is shown entered; a chip or a closed collection at its level.
  function enterLevelOf(id){
    const node=model.nodes.get(id);
    if(['program','area','inputs','bucket'].includes(node.kind))return chainOf(model,id);
    return chainOf(model,id).filter(at=>at!==id);
  }
  function showInput(id,smooth=true){
    const collection=model.rootOf(id);
    store.dispatch({type:'enter',level:chainOf(model,collection)});
    const rect=geometry.boxes.get(model.parent(id))||geometry.boxes.get(id);
    if(rect)moveCamera(frameCamera(rect,levelZoom([collection])),smooth);
  }
  const bounds=rects=>{
    const left=Math.min(...rects.map(r=>r.x)),top=Math.min(...rects.map(r=>r.y));
    return {x:left,y:top,width:Math.max(...rects.map(r=>r.x+r.width))-left,height:Math.max(...rects.map(r=>r.y+r.height))-top};
  };
  let pending=null;

  // The column's names are read by the host; the canvas reads a click.
  function read(id,event,center=false){
    if(!id)return;
    callbacks.select(id,center);
  }
  // What the emphasis needs of a declaration: its part, the names its
  // calls use for it and its source (canvas.jsx memberFacts).
  function memberFacts(member){
    const symbols=model.nodes.get(member?.part)?.item?.symbols||[],symbol=symbols[member?.index];
    if(!symbol)return null;
    const owner=symbol.owner?symbols[symbol.owner-1]?.name:'';
    return {part:member.part,names:[symbol.name,symbol.full||'',owner?`${owner}.${symbol.name}`:''].filter(Boolean),sources:[symbol.href,symbol.open].filter(Boolean)};
  }
  // A declaration clicked is read in its part, named; with a modifier its
  // code opens.
  function chooseMember(part,index,event){
    const symbol=model.nodes.get(part)?.item?.symbols?.[index];if(!symbol||symbol.kind==='more')return;
    if(event&&(event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)){window.open(symbol.code||symbol.href,'_blank');return;}
    store.dispatch({type:'member',member:{chosen:{part,index}}});
    read(part,event);
    setTimeout(()=>map.explainSource?.({key:symbol.href||symbol.open||'',href:symbol.href,open:symbol.open}),0);
  }
  // The column names a declaration (Find, a link, a restored visit): its
  // tile is the one chosen, its part entered when out of sight.
  map.addEventListener('repomap:reading',()=>{
    const named=map.explorerMember;if(!named?.owner)return;
    const part=named.owner,symbols=model.nodes.get(part)?.item?.symbols||[];
    const same=value=>!!value&&[named.key,named.href,named.open].includes(value);
    const index=symbols.findIndex(symbol=>same(symbol.href)||same(symbol.open));
    const chosen=store.getState().member.chosen;
    if(index<0||chosen?.part===part&&chosen.index===index)return;
    store.dispatch({type:'member',member:{chosen:{part,index}}});
    if(!map.readingRestoring&&!levelKey(store.getState().level).split('/').includes(part))enter(chainOf(model,part),{rect:geometry.boxes.get(part)});
  });
  function clickAt(target,event){
    if(!target){
      if(store.getState().pinned.length||look.key){look.end();store.dispatch({type:'look',key:''});store.dispatch({type:'unpin'});return;}
      return;
    }
    if(target.type==='zoom'){const level=chainOf(model,target.id);enter(level);callbacks.follow?.(target.id,true);return;}
    if(target.type==='member'){chooseMember(target.part,target.index,event);return;}
    if(target.type==='marker'){
      const marker=target.item;
      if(marker.side==='in'){
        const collection=model.rootOf(marker.members[0]);
        const kinds=[...new Set(marker.members.map(id=>model.nodes.get(id)?.item?.activation).filter(Boolean))];
        if(callbacks.readKind)callbacks.readKind(collection,kinds);else read(collection,event);
      }else read(marker.systems.length===1?marker.systems[0]:model.rootOf(marker.systems[0]),event);
      return;
    }
    if(target.type==='port'){read(target.item.program,event);return;}
    if(target.type==='edge'){
      const edge=target.edge,backward=nearStart(edge,target.at);
      const group=connectionOf(model,edge,backward);
      if(group&&callbacks.openConnection){look.end();store.dispatch({type:'look',key:''});callbacks.openConnection(group.area,group.key);}
      return;
    }
    const node=target.node;
    if(target.kind){
      const kinds=[...new Set(model.nodes.get(target.group).children.map(id=>model.nodes.get(id)?.item?.activation).filter(Boolean))];
      if(callbacks.readKind)callbacks.readKind(node.id,kinds);else read(node.id,event);
      return;
    }
    if(node.kind==='kind'){read(model.parent(node.id),event);return;}
    if(node.kind==='bucket'){read(node.id.split('~').pop(),event);return;}
    read(node.id,event);
  }
  // Whether a point on an arrow is nearer its start than its end.
  const nearStart=(edge,at)=>edge.heads.start&&(!edge.heads.end||Math.hypot(edge.points[0].x-at.x,edge.points[0].y-at.y)<Math.hypot(edge.points.at(-1).x-at.x,edge.points.at(-1).y-at.y));

  // The arrows drawn now (an arrow drawn for a box pointed at counts while
  // it stands).
  let lastEmphasis=null;
  const shownEdge=edge=>lastEmphasis?.edgeState.get(edge.id)?!lastEmphasis.edgeState.get(edge.id).hidden:edge.rest!==false;
  // One hit test for the pointer and the click.
  let down=null;
  const worldAt=event=>{
    const box=host.getBoundingClientRect(),v=camera.get();
    return {x:(event.clientX-box.left-v.x)/v.zoom,y:(event.clientY-box.top-v.y)/v.zoom};
  };
  const onCard=event=>!!event.target.closest?.('.flow-floating-card');
  host.addEventListener('pointerdown',event=>{down={x:event.clientX,y:event.clientY};},true);
  host.addEventListener('pointermove',event=>{
    if(initializing||event.buttons)return;
    if(onCard(event))return;
    const scene=sceneOf(store.getState()),target=hitTest(scene,worldAt(event),camera.get().zoom,shownEdge);
    store.dispatch({type:'point',target:target?.type==='edge'?{...target,at:undefined}:target});
    if(target?.type==='edge'){
      const key=`edge:${target.id}:${nearStart(target.edge,target.at)?'back':'on'}`;
      handleRect={left:event.clientX-12,top:event.clientY-12,right:event.clientX+12,bottom:event.clientY+12,width:24,height:24};
      if(look.aim(key,performance.now()))pump();
    }else if(look.key&&!store.getState().pinned.includes(look.key)){
      const card=host.querySelector(`[data-card="${CSS.escape(look.key)}"]`)?.getBoundingClientRect();
      look.leave(look.key,performance.now(),{x:event.clientX,y:event.clientY},card||null);pump();
    }else if(look.pending)look.abandon(look.pending);
    if(look.move(event.clientX,event.clientY,performance.now(),false))store.dispatch({type:'look',key:look.key});
  });
  host.addEventListener('pointerleave',()=>{store.dispatch({type:'point',target:null});});
  host.addEventListener('click',event=>{
    if(initializing||onCard(event))return;
    if(down&&Math.hypot(event.clientX-down.x,event.clientY-down.y)>4)return;
    const scene=sceneOf(store.getState());
    clickAt(hitTest(scene,worldAt(event),camera.get().zoom,shownEdge),event);
  });
  document.addEventListener('keydown',event=>{
    if(event.key!=='Escape')return;
    if(look.key||store.getState().pinned.length){look.end();store.dispatch({type:'look',key:''});store.dispatch({type:'unpin'});event.preventDefault();}
  });

  // A zoom gesture may change the level; a pan never does.
  let lastZoom=geometry.home.zoom,gestureAim=null;
  // One pinch (ctrl+wheel, a trackpad's pinch) crosses at most one level
  // boundary and stops short of the next; a pause ends it. The zoom a tick
  // asks for is React Flow's own; only a tick that would cross a second
  // boundary is held short of it.
  let gesture=null;
  host.addEventListener('wheel',event=>{
    gestureAim={x:event.clientX,y:event.clientY};
    if(!event.ctrlKey||!instance||initializing||event.target.closest?.('.flow-floating-card'))return;
    const now=performance.now(),v=camera.get(),box=host.getBoundingClientRect();
    const aim={x:(event.clientX-box.left-v.x)/v.zoom,y:(event.clientY-box.top-v.y)/v.zoom};
    if(!gesture||now-gesture.at>300){
      const start=store.getState().level;
      gesture={start,depth:start.length};
    }
    gesture.at=now;
    const factor=navigator.userAgent.indexOf('Mac')>=0?10:1;
    const asked=v.zoom*Math.pow(2,-event.deltaY*(event.deltaMode===1?.05:event.deltaMode?1:.002)*factor);
    const to=Math.min(maxZoom(),Math.max(minZoom(),asked));
    if(to===v.zoom)return;
    // The level a zoom would bring, entering as deep as it reaches.
    const scenes=gesture.scenes||=new Map();
    const sceneFor=level=>{const key=levelKey(level);if(!scenes.has(key))scenes.set(key,sceneAt(model,geometry,level,{}));return scenes.get(key);};
    const depthAt=zoom=>{
      let level=gesture.start;
      for(let step=0;step<5;step++){
        const next=levelAfterZoom(model,geometry,sceneFor(level),level,zoom,aim,zoom>v.zoom);
        if(levelKey(next)===levelKey(level))break;
        level=next;
      }
      return level.length;
    };
    const zoom=pinchLimit(v.zoom,to,depthAt,gesture);
    if(zoom===to)return;
    event.preventDefault();event.stopPropagation();
    if(Math.abs(zoom/v.zoom-1)<1e-9)return;
    const at={x:event.clientX-box.left,y:event.clientY-box.top};
    const next={x:at.x-(at.x-v.x)*zoom/v.zoom,y:at.y-(at.y-v.y)*zoom/v.zoom,zoom};
    const previous=v.zoom;instance.setViewport(next);camera.set(next);lastZoom=next.zoom;levelForZoom(previous,next);
  },{capture:true,passive:false});
  function moved(event,viewport){
    camera.set(viewport);
    host.style.setProperty('--flow-zoom',String(viewport.zoom));
    const previous=lastZoom;lastZoom=viewport.zoom;
    if(event)levelForZoom(previous,viewport);
  }
  // A zoom gesture's level, with hysteresis (store.mjs zoomAction); a
  // zoom that brings another level has the column read it.
  function levelForZoom(previous,viewport){
    const box=host.getBoundingClientRect(),aimScreen=gestureAim||{x:box.left+box.width/2,y:box.top+box.height/2};
    const action=zoomAction(previous,viewport,{x:(aimScreen.x-box.left-viewport.x)/viewport.zoom,y:(aimScreen.y-box.top-viewport.y)/viewport.zoom});
    if(!action)return;
    const before=levelKey(store.getState().level);
    store.dispatch(action);
    const level=store.getState().level;
    if(levelKey(level)!==before&&level.length)callbacks.follow?.(level.at(-1));
  }

  function Overlay({scene,emphasis}){
    const v=useSyncExternalStore(camera.subscribe,camera.get);
    const items=project(scene,v);
    const state=useSyncExternalStore(store.subscribe,store.getState);
    const pointer=state.pointer;
    const tip=pointer&&(pointer.type==='marker'||pointer.type==='port')?items.find(item=>item.id===pointer.id):null;
    return <div className="scene-overlay" aria-hidden="true">
      {items.filter(item=>item.type==='zoom').map(item=><span key={item.id} className="scene-zoom" data-zoom-into={item.box}
        style={{left:item.left-item.px/2,top:item.top-item.px/2,width:item.px,height:item.px}}><span className="flow-zoom-picture"/></span>)}
      {items.filter(item=>item.type!=='zoom').map(item=><span key={item.id} className={`scene-mark scene-mark-${item.type==='port'?'port':item.side} ${emphasis.lit.has(item.id)||pointer?.id===item.id?'scene-mark-lit':''}`}
        data-marker={item.type==='marker'?item.id:undefined} data-marker-box={item.type==='marker'?item.box:undefined} data-marker-side={item.type==='marker'?item.side:undefined}
        data-marker-end={item.type==='marker'?(item.side==='out'?item.systems:item.members).join(' '):undefined} data-port={item.type==='port'?item.id:undefined} data-port-end={item.type==='port'?item.id:undefined}
        style={{left:item.left-item.px/2,top:item.top-item.px/2,width:item.px,height:item.px}}>
        <Mark icon={item.type==='port'?systemIcons.program:item.side==='in'?kindIcon(item.kind):systemIcons[item.kind]}/></span>)}
      {tip&&<MarkTip item={tip}/>}
    </div>;
  }
  // What a marker or a port stands for, one name to a line.
  function MarkTip({item}){
    const most=8;
    let head='',names=[];
    if(item.type==='port')head=`${item.way==='out'?'→':'←'} ${nameOf(item.program)}`;
    else if(item.side==='in'){
      head=t(inputKindTitles[item.kind]||'Inputs');
      names=item.members.map(id=>`${model.nodes.get(id)?.name||''}${item.handled.includes(id)?'':` · ${t('declared here')}`}`);
    }else{
      head=item.kind==='database'?t('Database'):item.kind==='request'?t('Request'):item.kind==='sdk'?t('SDK'):item.kind==='queue'?t('Queue'):item.kind==='started'?t('Runs a program'):t('Outside');
      names=item.systems.map(id=>model.nodes.get(id)?.name||'');
    }
    const right=item.side!=='in';
    return <div className={`scene-tip ${right?'scene-tip-right':''}`} style={{left:right?item.left+item.px/2+6:undefined,right:right?undefined:`calc(100% - ${item.left-item.px/2-6}px)`,top:item.top-item.px/2}}>
      <b>{head}</b>{names.slice(0,most).map((name,i)=><span key={i}>{name}</span>)}{names.length>most&&<span>…</span>}</div>;
  }
  function ArrowCard({edgeKey,scene}){
    const [,id,way]=/^edge:(.*):(on|back)$/.exec(edgeKey)||[];
    const edge=scene.edges.find(edge=>edge.id===id);
    const ref=useRef(null),[at,setAt]=useState(null);
    const state=useSyncExternalStore(store.subscribe,store.getState);
    const pinned=state.pinned.includes(edgeKey);
    const v=useSyncExternalStore(camera.subscribe,camera.get);
    const backward=way==='back';
    const ids=edge?(backward?edge.backward:edge.forward):[];
    const from=edge?(backward?edge.to:edge.from):'',into=edge?(backward?edge.from:edge.to):'';
    const card=useMemo(()=>callCard(ids.flatMap(id=>edgeByID(model).get(id)?.relations||[]),{nameOf,groupable:other=>model.nodes.get(other)?.kind!=='input'}),[edgeKey]);
    useLayoutEffect(()=>{
      const el=ref.current;if(!el||!handleRect)return;
      const box=host.getBoundingClientRect(),size=el.getBoundingClientRect();
      const placed=placeCard({handle:handleRect,frame:null,canvas:box,size:{width:size.width,height:size.height}});
      setAt({x:placed.x-box.left,y:placed.y-box.top});
    },[edgeKey,pinned]);
    if(!edge||!card.total)return null;
    const other=edge.heads.start&&edge.heads.end;
    const swap=event=>{event.stopPropagation();const next=`edge:${id}:${backward?'on':'back'}`;store.dispatch({type:'unpin',key:edgeKey});look.enter(next);store.dispatch({type:'look',key:next});store.dispatch({type:'pin',key:next});};
    return <div ref={ref} data-card={edgeKey} className="flow-calls-place flow-floating-card scene-card nopan" style={{left:at?.x||0,top:at?.y||0,visibility:at?'visible':'hidden'}}
      onMouseEnter={()=>{look.enter(edgeKey);}} onMouseLeave={event=>{look.leave(edgeKey,performance.now());pump();}}
      onClick={event=>{event.stopPropagation();if(!pinned)store.dispatch({type:'pin',key:edgeKey});}}>
      <div className={`flow-connection-calls ${pinned?'flow-card-pinned':''}`} style={{width:420,maxHeight:Math.max(160,Math.min(520,host.clientHeight-16))}}>
        {pinned&&<button type="button" className="flow-card-close" aria-label={t('Close')} title={t('Close')} onClick={event=>{event.stopPropagation();store.dispatch({type:'unpin',key:edgeKey});look.end();store.dispatch({type:'look',key:''});}}>✕</button>}
        <header className="flow-card-head"><div className="flow-card-title"><span>{nameOf(from)}</span><i>→</i><span>{nameOf(into)}</span></div>
          {other&&<p className="flow-card-count"><button type="button" onClick={swap}>{t('Calls the other way')}</button></p>}</header>
        <div className="flow-card-body"><BriefRows card={card} into={nameOf(into)}/></div>
      </div></div>;
  }

  function App(){
    const state=useSyncExternalStore(store.subscribe,store.getState);
    const scene=sceneOf(state);
    // A declaration pointed at, else the one chosen in the part read.
    const chosen=state.member.chosen&&state.view.scope===state.member.chosen.part?state.member.chosen:null;
    const pointed=state.pointer?.type==='member'?{part:state.pointer.part,index:state.pointer.index}:chosen;
    const facts=memberFacts(pointed);
    const pointer=state.pointer?.type==='member'?{type:'box',id:state.pointer.part}:state.pointer;
    const emphasis=useMemo(()=>emphasisOf(scene,model,pointer,state.view,facts),[scene,state.pointer,state.view,chosen]);
    lastEmphasis=emphasis;
    useEffect(()=>{callbacks.emphasis?.({...emphasis.state,overview:!state.level.length});},[emphasis.state.mode,emphasis.state.subject,state.view.scope,state.level.length]);
    useEffect(()=>{
      if(state.level.length)map.dataset.sceneLevel=levelKey(state.level);else delete map.dataset.sceneLevel;
      if(scene.program)map.dataset.enteredProgram=scene.program;else delete map.dataset.enteredProgram;
      location.textContent=state.level.length?state.level.map(nameOf).join(' / '):t('System map');
    },[scene.key]);
    const lit=new Set(state.lit);
    const nodes=useMemo(()=>scene.nodes.map(node=>{
      const item=model.nodes.get(node.id)?.item;
      const groups=node.display==='inputs'?model.nodes.get(node.id).children.map(id=>({kind:model.nodes.get(id).inputKind,lit:model.nodes.get(id).children.some(input=>lit.has(input))})):undefined;
      return {id:node.id,type:node.display,position:{x:node.rect.x,y:node.rect.y},width:node.rect.width,height:node.rect.height,
        style:{width:node.rect.width,height:node.rect.height},zIndex:node.band===bands.frame?-1:2,
        selectable:false,draggable:false,connectable:false,focusable:false,
        className:`scene-node scene-is-${node.display} ${emphasis.nodeClass.get(node.id)||''}`,
        data:{node,item,groups,lit:lit.has(node.id),
          drawn:node.display==='deep'?geometry.grids.get(node.id):undefined,
          member:node.display==='deep'?{hot:pointed?.part===node.id?pointed.index:-1,chosen:chosen?.part===node.id?chosen.index:-1,point:()=>{},choose:()=>{}}:undefined}};
    }),[scene,emphasis,state.lit,pointed?.part,pointed?.index,chosen]);
    const edges=useMemo(()=>emphasis.order.filter(id=>!emphasis.edgeState.get(id).hidden).map(id=>{
      const edge=scene.edges.find(e=>e.id===id),flags=emphasis.edgeState.get(id);
      const ends=[edge.from,edge.to].map(end=>scene.nodes.some(node=>node.id===end)?end:scene.program||scene.nodes[0]?.id);
      return {id,source:ends[0],target:ends[1],type:'scene',zIndex:flags.on?1:0,selectable:false,focusable:false,data:{...edge,...flags}};
    }),[scene,emphasis]);
    const cards=[...new Set([state.look,...state.pinned].filter(key=>key.startsWith('edge:')))];
    return <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} zIndexMode="manual"
      nodesDraggable={false} nodesConnectable={false} elementsSelectable={false} nodesFocusable={false} edgesFocusable={false} disableKeyboardA11y
      deleteKeyCode={null} selectionKeyCode={null} multiSelectionKeyCode={null} panActivationKeyCode={null} zoomActivationKeyCode={null}
      zoomOnDoubleClick={false} minZoom={minZoom()} maxZoom={maxZoom()} panOnScroll preventScrolling
      onInit={flow=>{instance=flow;fitOverview().then(()=>requestAnimationFrame(()=>{
        initializing=false;map.classList.remove('flow-initializing');status.remove();
        if(restorePending)restore(restorePending);
        else if(pending)focus(pending.id,pending.center);
        else map.dispatchEvent(new Event('repomap:viewport'));
      }));}}
      onMove={(event,viewport)=>moved(event,viewport)}
      onMoveStart={event=>{if(event)overviewFit=false;}}
      onMoveEnd={(event,viewport)=>{camera.set(viewport);gestureAim=null;map.dispatchEvent(new Event('repomap:viewport'));}}>
      <svg className="flow-defs"><defs>
        <marker id="scene-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth={arrowHead} markerHeight={arrowHead} orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b"/></marker>
        <marker id="scene-arrow-active" viewBox="0 0 10 10" refX="9" refY="5" markerWidth={arrowHead*1.5/2.5} markerHeight={arrowHead*1.5/2.5} orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#34445b"/></marker>
      </defs></svg>
      <Overlay scene={scene} emphasis={emphasis}/>
      {cards.map(key=><ArrowCard key={key} edgeKey={key} scene={scene}/>)}
    </ReactFlow>;
  }
  // Far enough into a part to read its declarations, and a little more.
  const minZoom=()=>Math.min(.05,geometry.home.zoom*.5);
  const maxZoom=()=>Math.max(4,...[...geometry.enterZoom.values()].map(zoom=>zoom*1.6),...[...geometry.grids.keys()].map(id=>levelZoom([id])*1.6));

  let restorePending=null;
  function capture(){return instance&&!initializing?{...camera.get(),level:store.getState().level,layoutKey,fit:overviewFit}:restorePending;}
  function restore(saved){
    if(!saved)return;
    if(!instance||initializing){restorePending=saved;return;}
    if(saved.fit||saved.layoutKey!==layoutKey||!Number.isFinite(saved.zoom)){fitOverview();return;}
    store.dispatch({type:'enter',level:(saved.level||[]).filter(id=>model.nodes.has(id))});
    moveCamera({x:saved.x,y:saved.y,zoom:saved.zoom},false);
  }
  function stepOut(){
    const level=store.getState().level;
    if(!level.length){instance?.zoomTo(camera.get().zoom*.8);return;}
    const up=level.slice(0,-1);
    if(!up.length){fitOverview(420);return;}
    enter(up);
  }

  const root=createRoot(host);flushSync(()=>root.render(<App/>));
  map.flowGeometry=()=>{
    const scene=sceneOf(store.getState());
    return {nodes:scene.nodes.map(node=>({id:node.id,parentId:model.parent(node.id),frame:node.display==='frame',branch:node.kind,
      x:node.rect.x,y:node.rect.y,width:node.rect.width,height:node.rect.height,shown:true,contentScale:node.text}))};
  };
  // The scene and the level, for the invariant checks.
  map.sceneState=()=>({level:store.getState().level,scene:sceneOf(store.getState()),camera:camera.get()});
  // Enter the level showing `id`, as its magnifier does (probes and checks).
  map.sceneEnter=id=>enter(chainOf(model,id));
  map.querySelector('[data-map-controls]')&&(map.querySelector('[data-map-controls]').hidden=false);
  const overviewButton=map.querySelector('[data-map-fit]');
  if(overviewButton){overviewButton.textContent=t('Show whole map');overviewButton.removeAttribute('title');}
  map.querySelector('[data-map-controls]')?.addEventListener('click',event=>{
    const button=event.target.closest('button');if(!button||!instance)return;
    if(button.hasAttribute('data-map-fit'))fitOverview(420);
    else if(button.hasAttribute('data-map-zoom')&&Number(button.dataset.mapZoom)<1)stepOut();
    else if(button.hasAttribute('data-map-zoom')){
      const box=host.getBoundingClientRect();gestureAim={x:box.left+box.width/2,y:box.top+box.height/2};
      Promise.resolve(instance.zoomTo(camera.get().zoom*Number(button.dataset.mapZoom),{duration:200})).then(()=>{gestureAim=null;});
    }
  });
  // A resize lays every level out again; the camera keeps the level.
  let resizing=null;
  new ResizeObserver(()=>{
    if(initializing)return;
    clearTimeout(resizing);
    resizing=setTimeout(async()=>{
      const next=await layoutLevels(model,{...size(),measure});
      geometry=next;sceneCache={key:'',scene:null};
      layoutKey=String(Math.round(geometry.bounds.width))+'x'+String(Math.round(geometry.bounds.height));
      store.dispatch({type:'relayout'});
      if(overviewFit)fitOverview();
    },250);
  }).observe(host);

  const mounted=new Map();
  function mountConnections(container,id,open='',choose=null,readInput=null,apart=null){
    const groups=frameConnections(id).map(group=>apart&&apart(group.outside)?{...group,apart:true}:group).sort((a,b)=>(a.apart?1:0)-(b.apart?1:0));
    if(!groups.length)return false;
    for(const [element,mountedRoot] of mounted)if(!element.isConnected){mountedRoot.unmount();mounted.delete(element);}
    const target=mounted.get(container)||createRoot(container);mounted.set(container,target);
    const chooser=choose&&{go:choose,can:(part,key)=>!!key&&(model.nodes.get(part)?.item?.symbols||[]).some(symbol=>symbol.href===key||symbol.open===key),input:readInput};
    flushSync(()=>target.render(<FrameConnections groups={groups} open={open} choose={chooser} single={model.nodes.get(id)?.kind==='part'}/>));
    return true;
  }
  return {
    get layout(){return {nodes:[],edges:model.edges};},
    focus,showInput,capture,restore,
    clearHover(){store.dispatch({type:'point',target:null});},
    clearMember(){store.dispatch({type:'member',member:{chosen:null}});},
    mountConnections,frameConnections,
    light(ids){store.dispatch({type:'lit',ids});},
    overview:()=>fitOverview(420),
    update(next){
      const chosen=store.getState().member.chosen;
      if(chosen&&model.shown(next.scope||'')!==chosen.part)store.dispatch({type:'member',member:{chosen:null}});
      const view={...next,scope:model.shown(next.scope||''),selected:new Set([...(next.selected||[])].map(model.shown)),matched:new Set([...(next.matched||[])].map(model.shown))};
      store.dispatch({type:'view',view});
    },
  };
}
