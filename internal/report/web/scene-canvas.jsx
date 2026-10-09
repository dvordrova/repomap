// The scene canvas (PLAN): the page's data
// is built into a model once (model.mjs), every level is laid out once per
// canvas size (levels.mjs), and what is drawn is a pure function of the
// level and the reader's choice (scene.mjs). React Flow is the camera and a
// dumb renderer; one store holds the level, the pointer and the choice; one
// hitTest answers hover and click; the overlay keeps markers and ports at
// one screen size (overlay.mjs).
import React,{useEffect,useLayoutEffect,useMemo,useRef,useState,useSyncExternalStore} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {ReactFlow,Handle,Position,useStore} from '@xyflow/react';
import {buildModel,inputKindTitles} from './model.mjs';
import {layoutLevels,homeCamera,units,cardWords,programWords} from './levels.mjs';
import {sceneAt,emphasisOf,hitTest,chainOf,levelKey,bands,edgeByID,frameGroups,connectionOf,levelAfterZoom,pinchLimit,memberView} from './scene.mjs';
import {project,mark} from './overlay.mjs';
import {createStore,sceneReducer,initialState,createCamera,zoomAction} from './store.mjs';
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
// A tile's declaration as the reading keys it: its identity (decl_key),
// else its link, else its place (26-map-members.js symbolKey).
const symbolKey=symbol=>symbol?symbol.decl_key||symbol.href||symbol.open||(symbol.path?JSON.stringify([symbol.path,symbol.line||0]):''):'';
const sameSet=(a,b)=>a.size===b.size&&[...a].every(id=>b.has(id));

// The words a box shows at its level's text size, each of its title, role
// and description whole or not at all (levels.mjs cardWords, programWords).
const wordsOf=(node,item)=>['program','area'].includes(node.display)?programWords({...node,item},node.rect.width/node.text,measure)
  :cardWords(node,item?.summary||'',measure,{room:node.enter?24:0});
function Mark({icon,className='',label='',kind=''}){
  if(!icon)return <span className={`scene-dot ${className}`} aria-hidden={label?undefined:'true'}/>;
  return <svg className={`flow-kind-mark ${className}`} data-kind-mark={kind||undefined} viewBox="0 0 16 16" width="14" height="14" aria-hidden={label?undefined:'true'} role={label?'img':undefined} aria-label={label||undefined}>
    {icon.paths.map((d,i)=><path key={i} d={d}/>)}</svg>;
}
const laneClass=lane=>lane==='core'?'flow-core':lane==='triggers'?'flow-entry':'';
// A box drawn at its level's text size: its world rectangle holds its
// content laid out at `text` world units to a pixel.
function Scaled({node,className='',children,style,data}){
  const {rect,text}=node;
  return <div className={`scene-scaled ${className}`} {...data} style={{width:rect.width/text,height:rect.height/text,transform:`scale(${text})`,...style}}>{children}</div>;
}
const handles=<><Handle type="target" position={Position.Top} isConnectable={false}/><Handle type="source" position={Position.Bottom} isConnectable={false}/></>;

function CardNode({data}){
  const {node}=data,item=data.item;
  const words=useMemo(()=>wordsOf(node,item),[node.rect.width,node.rect.height,node.text,node.title]);
  // A closed part holding the declaration chosen shows it alone as its
  // tile under its title (owner, 2026-09-29), in place of its description.
  if(data.tile&&words.title.length)return <>{handles}<Scaled node={node} className={`flow-part ${laneClass(node.lane)}`}>
    <div className="scene-words"><strong data-box-title={node.id}>{words.title.join('\n')}</strong>{' '}
      <span className="scene-chosen-tile" title={data.tile.full||data.tile.name}>{data.tile.name}</span></div></Scaled></>;
  return <>{handles}<Scaled node={node} className={`flow-part ${node.display==='area'?'scene-area-card':''} ${laneClass(node.lane)}`} data={node.display==='area'?{'data-summary-area':node.id}:undefined}>
    <div className="scene-words">{words.title.length>0&&<strong data-box-title={node.id}>{words.title.join('\n')}</strong>}{' '}
    {words.lines.length>0&&<div className="flow-description flow-description-lines">{words.lines.join('\n')}</div>}</div>
  </Scaled></>;
}
function ProgramNode({data}){
  const {node}=data,item=data.item;
  const words=useMemo(()=>wordsOf(node,item),[node.rect.width,node.rect.height,node.text,node.title,node.contents,item]);
  return <>{handles}<Scaled node={node} className={`scene-program-card ${node.display==='area'?`flow-part scene-area-card ${laneClass(node.lane)}`:''}`} data={node.display==='area'?{'data-summary-area':node.id}:{'data-component-overview':node.id}}>
    <div className="scene-words"><strong data-box-title={node.id}>{words.title.join('\n')}</strong>{' '}
    {words.role.length>0&&<div className="flow-component-role">{words.role.join('\n')}</div>}{' '}
    {words.purpose.length>0&&<p className="flow-description flow-description-lines">{words.purpose.join('\n')}</p>}
    {words.contents.length>0&&<ul className="scene-program-inside nodrag" onPointerDownCapture={event=>event.stopPropagation()} onMouseDownCapture={event=>event.stopPropagation()} style={{height:node.inventoryHeight}} aria-label={t('Parts')}>
      {words.contents.map(child=><li key={child.id}><button type="button" data-program-child={child.id}
        onClick={event=>{event.stopPropagation();data.open(child.id,event);}}>{child.lines.join('\n')}</button></li>)}
    </ul>}</div>
  </Scaled>
    {node.display==='area'&&['core','triggers'].includes(node.lane)&&<Scaled node={node} className="scene-role-layer" style={{width:0,height:0}}>
      <span className={`flow-role-symbol flow-role-${node.lane}`} aria-hidden="true"/></Scaled>}
  </>;
}
function FrameNode({data}){
  const {node}=data,kind=node.kind;
  const tone=kind==='program'?'flow-component':kind==='outside'||kind==='bucket'?'flow-communication':kind==='inputs'?'flow-input-collection'
    :node.lane==='core'?'flow-area-core':node.lane==='triggers'?'flow-area-entry':'';
  const title=kind==='outside'?t('Outside'):kind==='inputs'?t('Inputs'):node.title;
  const titleText=node.titleText||node.text,band=units.band(1)*titleText;
  return <>{handles}<div className={`flow-area ${tone} ${node.bucket?'scene-bucket-open':''}`} style={{width:node.rect.width,height:node.rect.height}}>
    <div className="scene-frame-title-place">
    <Scaled node={{...node,text:titleText,rect:{...node.rect,height:band}}} className="scene-frame-title" style={{'--scene-text':titleText}}>
      <strong data-box-title={node.id} data-frame-title={node.id}>{title}</strong></Scaled></div>
  </div></>;
}
// An Inputs frame closed: its title over its kinds' marks, each named on
// hover; entered, the kinds' groups name their inputs.
function InputsNode({data}){
  const {node,groups}=data;
  return <>{handles}<Scaled node={node} className="flow-area flow-input-collection scene-inputs-card" data={{'data-component-overview':node.id}}>
    <strong data-box-title={node.id}>{t('Inputs')}</strong>
    {node.kinds.map((entry,i)=><button type="button" key={entry.kind} className={`scene-kind-mark nodrag ${groups[i]?.lit?'flow-lit':''}`} title={t(inputKindTitles[entry.kind])} aria-label={t(inputKindTitles[entry.kind])}
      onPointerDownCapture={event=>event.stopPropagation()} onMouseDownCapture={event=>event.stopPropagation()}
      onClick={event=>{event.stopPropagation();data.kind(entry,event);}}
      data-input-group-kind={entry.kind} style={{left:(entry.rect.x-node.rect.x)/node.text,top:(entry.rect.y-node.rect.y)/node.text,width:entry.rect.width/node.text,height:entry.rect.height/node.text}}>
      <Mark icon={kindIcon(entry.kind)} kind={entry.kind}/></button>)}
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
  return <>{handles}<Scaled node={node} className={`scene-tile ${data.lit?'flow-lit':''}`} data={{'data-input-id':node.id}}><Mark icon={kindIcon(node.inputKind)} kind={node.inputKind}/><span>{node.title}</span></Scaled></>;
}
// A chip and a bucket take the keyboard's focus: focused, a chip whose name
// does not read is named, as pointed at; Enter reads it.
const focusable=data=>data.keys?{tabIndex:0,'aria-label':data.node.title,...data.keys}:undefined;
function ChipNode({data}){
  const {node}=data;
  return <>{handles}<Scaled node={node} className={`flow-chip scene-chip ${data.item?.unestablished?'flow-chip-unestablished':''}`} data={focusable(data)}>
    <Mark icon={systemIcons[node.systemKind]}/><span className="flow-chip-name" title={node.title}>{node.title}</span></Scaled></>;
}
// A closed bucket: its part's name over its systems' marks, never blank.
// A closed part-group: one of our parts (its name on a part's card) in
// front of a stack of the outside systems only it calls, their kinds'
// marks under its name (owner via the coordinator, 2026-10-02, on the
// skeptic's verdict: a group had read as one more outside system).
function BucketNode({data}){
  const {node}=data;
  return <>{handles}<Scaled node={node} className="flow-chip scene-bucket scene-bucket-stack" data={focusable(data)}>
    <span className="scene-bucket-face"><span className="scene-bucket-words"><span className="flow-chip-name" title={node.title}>{node.title}</span>
    <span className="scene-bucket-marks">{node.systemKinds.map(kind=><Mark key={kind} icon={systemIcons[kind]}/>)}</span></span></span></Scaled></>;
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
  return <>{handles}<div className={`flow-part flow-part-deep scene-deep ${laneClass(node.lane)}`} style={{width:box.width,height:box.height,transform:`scale(${s})`,'--scene-card-scale':s}}>
    <strong style={{fontSize:28/grid.divisor,lineHeight:`${40/grid.divisor}px`,padding:`${20/grid.divisor}px ${32/grid.divisor}px 0`}}>{node.title}</strong>
    <PartSymbols symbols={item.symbols} calls={item.symbolCalls} width={box.width} height={box.height} grid={grid} member={member}/>
  </div></>;
}
const nodeTypes={card:CardNode,area:ProgramNode,deep:DeepNode,program:ProgramNode,frame:FrameNode,inputs:InputsNode,kindgroup:GroupNode,tile:TileNode,chip:ChipNode,bucket:BucketNode,note:NoteNode};

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

  // The facts drawn are the saved scene the page carries (REPORT.md
  // "Scene model"), read as saved.
  let saved=null;
  try{saved=JSON.parse(document.getElementById('rm-scene')?.textContent||'null');}catch{saved=null;}
  const model=buildModel({items:records,relations,areas,scene:saved},{measure,t});
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
  const clearPointerCursor=()=>host.classList.remove('scene-pointing-arrow','scene-pointing-zoom','scene-pointing-mark');
  camera.subscribe(clearPointerCursor);
  let instance=null,initializing=true,overviewFit='rest';
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
    if(String(id).startsWith('port:'))return (sceneOf(store.getState()).ports.find(port=>port.id===id)?.programs||[]).map(program=>model.nodes.get(program)?.name||'').join(', ');
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
  // A level larger than the canvas is framed by what it draws (`content`,
  // its boxes' bounds), centred where that fits and from its first corner
  // where it does not: the frame's own corner had been empty margin
  // (nats-server's and Metabase's components opened on blank canvas).
  function frameCamera(rect,minZoom=0,pad=28,maxZoom=Infinity,content=null){
    const {width,height}=size(),fit=Math.min((width-2*pad)/rect.width,(height-2*pad)/rect.height);
    const zoom=Math.max(Math.min(fit,maxZoom),minZoom);
    const along=(start,length,room)=>{
      if(length*zoom<=room-2*pad)return null;
      const inner=content?{start:content[start==='x'?'x':'y'],length:content[start==='x'?'width':'height']}:null;
      if(inner&&inner.length*zoom<=room-2*pad)return (room-inner.length*zoom)/2-inner.start*zoom;
      return pad-(inner?inner.start:rect[start])*zoom;
    };
    const x=along('x',rect.width,width)??(width-rect.width*zoom)/2-rect.x*zoom;
    const y=along('y',rect.height,height)??(height-rect.height*zoom)/2-rect.y*zoom;
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
  // No closer than its boxes' words read at a third over their reading
  // size: one box framed alone keeps its words at reading size and its
  // neighbours in sight (owner via the coordinator, 2026-10-02: casdoor's
  // Custom Logout Endpoint had filled the canvas in 90-pixel letters).
  const readCap=level=>1.35/(geometry.text.get(level.at(-1))||geometry.unit||1);
  function enter(level,{rect=null,smooth=true}={}){
    store.dispatch({type:'enter',level});
    const scene=sceneOf(store.getState());
    return moveCamera(frameCamera(rect||scene.focus,levelZoom(level),28,readCap(level),rect?null:contentOf(scene,level)),smooth);
  }
  // What a level draws inside its frame: the bounds of its own boxes.
  function contentOf(scene,level){
    const inner=level.at(-1),inside=scene.nodes.filter(node=>node.id!==inner&&node.rect&&model.parent(node.id)===inner);
    return inside.length?bounds(inside.map(node=>node.rect)):null;
  }
  // The whole map's two cameras (levels.mjs homeView): at rest, as close as
  // its names read; "Show whole map", all of it. They are one where the
  // whole map's names read (all but etcd's).
  function fitOverview(duration=0,view='rest'){
    overviewFit=view;
    store.dispatch({type:'enter',level:[]});
    if(!instance)return Promise.resolve();
    return Promise.resolve(instance.setViewport(view==='whole'?geometry.whole:geometry.home,{duration})).then(()=>{camera.set(instance.getViewport());map.dispatchEvent(new Event('repomap:viewport'));});
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
    // A thing deeper than the canvas draws open is shown at its nearest
    // drawn level, on its nearest drawn box (levels.mjs units.openDepth).
    const drawn=at=>model.nodes.get(at)?.kind!=='area'||geometry.local.has(at);
    const full=node.kind==='part'?chainOf(model,id).filter(at=>at!==id):enterLevelOf(id);
    const cut=full.findIndex(at=>!drawn(at));
    const level=cut<0?full:full.slice(0,cut);
    let rect=geometry.boxes.get(id)||geometry.scales.get(id)?.frame;
    for(let at=model.parent(id);!rect&&at;at=model.parent(at))rect=geometry.boxes.get(at);
    // A thing in sight, or the level itself, is only marked: the camera
    // moves only to what is out of sight (owner, 2026-09-28).
    const current=store.getState().level;
    if(!center&&(current.includes(id)||sceneOf(store.getState()).nodes.some(node=>node.id===id)&&rect&&inSight(rect)))return;
    store.dispatch({type:'enter',level});
    const entered=level.at(-1)===id?contentOf(sceneOf(store.getState()),level):null;
    if(rect)moveCamera(frameCamera(rect,levelZoom(level),28,readCap(level),entered),smooth);
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
    if(rect)moveCamera(frameCamera(rect,levelZoom([collection]),28,readCap(chainOf(model,collection))),smooth);
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
    // A declaration is known by its identity; a link, which two
    // declarations written on one line share, only where it has none.
    return {part:member.part,names:[symbol.name,symbol.full||'',owner?`${owner}.${symbol.name}`:''].filter(Boolean),sources:symbol.decl_key?[symbol.decl_key]:[symbol.href,symbol.open,symbolKey(symbol)].filter(Boolean)};
  }
  // A declaration clicked is read in its part, named; with a modifier its
  // code opens.
  function chooseMember(part,index,event){
    const symbol=model.nodes.get(part)?.item?.symbols?.[index];if(!symbol||symbol.kind==='more')return;
    if(event&&(event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)){window.open(symbol.code||symbol.href,'_blank');return;}
    store.dispatch({type:'member',member:{chosen:{part,index}}});
    read(part,event);
    setTimeout(()=>map.explainSource?.({key:symbolKey(symbol),href:symbol.href,open:symbol.open}),0);
  }
  // The column names a declaration (Find, a link, a restored visit): its
  // tile is the one chosen. A restored visit keeps its camera; one newly
  // named out of sight is shown as the owner set (scene.mjs memberView): its
  // part across the canvas with the tile centred down it, or a part too
  // dense to read there closed, the declaration alone in its card.
  map.addEventListener('repomap:reading',()=>{
    const named=map.explorerMember;if(!named?.owner)return;
    const part=named.owner,symbols=model.nodes.get(part)?.item?.symbols||[];
    // By its identity when it has one: sbTruncate, written on sbRewind's
    // line, shares its link (review, 2026-10-03).
    const same=value=>!!value&&[named.href,named.open].includes(value);
    const index=named.key?symbols.findIndex(symbol=>symbolKey(symbol)===named.key):symbols.findIndex(symbol=>same(symbol.href)||same(symbol.open));
    const chosen=store.getState().member.chosen;
    if(index<0||chosen?.part===part&&chosen.index===index)return;
    store.dispatch({type:'member',member:{chosen:{part,index}}});
    if(map.readingRestoring)return;
    const view=memberView(model,geometry,part,index,size());
    if(!view)return;
    const scene=sceneOf(store.getState()),here=levelKey(store.getState().level);
    const shown=view.dense?here===levelKey(view.level)&&inSight(geometry.boxes.get(part),0)
      :here===levelKey(view.level)&&inSight(scene.members.find(member=>member.index===index&&member.part===part)?.rect||geometry.boxes.get(part),0);
    if(shown)return;
    store.dispatch({type:'enter',level:view.level});
    moveCamera(view.camera);
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
    if(target.type==='port'){read(target.item.programs[0],event);return;}
    if(target.type==='edge'){
      const edge=target.edge,backward=nearStart(edge,target.at);
      const group=connectionOf(model,edge,backward);
      if(group&&callbacks.openConnection){look.end();store.dispatch({type:'look',key:''});callbacks.openConnection(group.area,group.key);}
      return;
    }
    const node=target.node;
    if(target.group){
      const kinds=[...new Set(model.nodes.get(target.group).children.map(id=>model.nodes.get(id)?.item?.activation).filter(Boolean))];
      if(callbacks.readKind)callbacks.readKind(node.id,kinds);else read(node.id,event);
      return;
    }
    if(node.kind==='kind'){read(model.parent(node.id),event);return;}
    // A part's group reads as the systems only that part calls, the part
    // named (29-operation-view.js readGroup).
    if(node.kind==='bucket'){const bucket=model.nodes.get(node.id);if(bucket&&callbacks.readGroup){callbacks.readGroup(bucket.parent,bucket.part,bucket.children);return;}read(node.id.split('~').pop(),event);return;}
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
  const onCard=event=>!!event.target.closest?.('.flow-floating-card,.scene-program-inside,[data-input-group-kind]');
  // A press that moves the pointer or the camera is a drag: the click
  // ending it acts on nothing.
  // A press ends a look not yet opened: an arrow's card does not open under
  // the pointer while it drags the map, at the place it was pressed.
  host.addEventListener('pointerdown',event=>{clearPointerCursor();const v=camera.get();down={x:event.clientX,y:event.clientY,camera:{x:v.x,y:v.y,zoom:v.zoom},moved:false};if(look.pending&&!onCard(event))look.abandon(look.pending);},true);
  host.addEventListener('pointermove',event=>{if(down&&event.buttons&&Math.hypot(event.clientX-down.x,event.clientY-down.y)>4)down.moved=true;},true);
  host.addEventListener('pointermove',event=>{
    if(initializing||event.buttons)return;
    if(onCard(event)){clearPointerCursor();return;}
    const scene=sceneOf(store.getState()),target=hitTest(scene,worldAt(event),camera.get().zoom,shownEdge);
    host.classList.toggle('scene-pointing-arrow',target?.type==='edge');
    host.classList.toggle('scene-pointing-zoom',target?.type==='zoom');
    host.classList.toggle('scene-pointing-mark',target?.type==='marker'||target?.type==='port');
    // Keyboard focus owns its marker card until blur, selection or Escape.
    const focused=document.activeElement,focusCard=focused.closest?.('.scene-mark-tip'),focusHandle=focused.closest?.('.scene-mark');
    if(host.contains(focused)&&look.key&&(focusCard?.dataset.card===look.key||focusHandle&&`mark:${focusHandle.dataset.marker||focusHandle.dataset.port}`===look.key))return;
    // While a card is open or kept, what is being read stays: the way to
    // the card crosses boxes and empty canvas without changing the emphasis.
    const reading=!!look.key||store.getState().pinned.length>0;
    if(!reading||target?.type==='edge')store.dispatch({type:'point',target:target?.type==='edge'?{...target,at:undefined}:target});
    if(target?.type==='edge'){
      const key=`edge:${target.id}:${nearStart(target.edge,target.at)?'back':'on'}`;
      handleRect={left:event.clientX-12,top:event.clientY-12,right:event.clientX+12,bottom:event.clientY+12,width:24,height:24};
      if(look.aim(key,performance.now()))pump();
    }else if(target?.type==='marker'||target?.type==='port'){
      if(look.aim(`mark:${target.id}`,performance.now()))pump();
    }else if(look.key&&!store.getState().pinned.includes(look.key)){
      const card=host.querySelector(`[data-card="${CSS.escape(look.key)}"]`)?.getBoundingClientRect();
      look.leave(look.key,performance.now(),{x:event.clientX,y:event.clientY},card||null);pump();
    }else if(look.pending)look.abandon(look.pending);
    if(look.move(event.clientX,event.clientY,performance.now(),false))store.dispatch({type:'look',key:look.key});
  });
  // A drag carries what it points at with it, the map moving under the
  // pointer: what is pointed at stays pointed at while the press lasts,
  // even past the canvas's edge, and a pan changes nothing drawn (harness
  // table, 2026-10-02: a drag on etcd's whole map that left the canvas had
  // dropped the quiet arrows of the program pointed at; the owner's first
  // complaint had been inputs vanishing as the mouse moved).
  let pressed=false;
  host.addEventListener('pointerdown',()=>{pressed=true;},true);
  window.addEventListener('pointerup',()=>{pressed=false;},true);
  window.addEventListener('pointercancel',()=>{pressed=false;},true);
  host.addEventListener('pointerleave',event=>{clearPointerCursor();if(pressed||event.buttons)return;store.dispatch({type:'point',target:null});});
  host.addEventListener('click',event=>{
    if(initializing||onCard(event))return;
    const v=camera.get(),press=down;down=null;
    if(press&&(press.moved||Math.hypot(event.clientX-press.x,event.clientY-press.y)>4||Math.abs(v.x-press.camera.x)>.5||Math.abs(v.y-press.camera.y)>.5||v.zoom!==press.camera.zoom))return;
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
    if(event.target.closest?.('.flow-floating-card'))return;
    const inventory=event.target.closest?.('.scene-program-inside');
    // Only a genuinely overflowing inventory owns vertical scrolling.
    // Horizontal movement and every pinch still belong to the map.
    if(!event.ctrlKey&&inventory&&inventory.scrollHeight>inventory.clientHeight+1&&Math.abs(event.deltaY)>Math.abs(event.deltaX)){
      event.stopPropagation();return;
    }
    if(!event.ctrlKey||!instance||initializing)return;
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

  // The keyboard on a chip or a bucket: focus points at it, Enter reads it.
  function keysOf(node){
    const target={type:'box',id:node.id,node};
    return {onFocus:()=>store.dispatch({type:'point',target}),onBlur:()=>store.dispatch({type:'point',target:null}),
      onKeyDown:event=>{if(event.key!=='Enter'&&event.key!==' ')return;event.preventDefault();clickAt(target,event);}};
  }
  // The zoom words fade by (scene.css) is the one React Flow draws at,
  // whatever moved the camera: a gesture, a button, an entry, a resize.
  function ZoomVar(){
    const zoom=useStore(state=>state.transform[2]);
    useLayoutEffect(()=>{host.style.setProperty('--flow-zoom',String(zoom));},[zoom]);
    return null;
  }
  function Overlay({scene,emphasis}){
    const v=useSyncExternalStore(camera.subscribe,camera.get);
    const items=project(scene,v);
    const {width,height}=size();
    // The keyboard reaches marks that are actually in the canvas. A pan
    // brings the rest into this same order; their world hit test is intact.
    const tabIndex=item=>item.left-item.px/2>=0&&item.top-item.px/2>=0&&item.left+item.px/2<=width&&item.top+item.px/2<=height?0:-1;
    const state=useSyncExternalStore(store.subscribe,store.getState);
    const pointer=state.pointer;
    const tip=state.look.startsWith('mark:')?items.find(item=>item.id===state.look.slice(5)):null;
    // A chip or a bucket pointed at says who calls it, or what it holds, a
    // chip whose name does not read naming it alone; any other box whose
    // words do not read yet is named.
    const pointed=pointer?.type==='box'?pointer.node:null;
    const unread=pointed&&(pointed.text*17*v.zoom<11||['card','area','program'].includes(pointed.display)&&wordsOf(pointed,model.nodes.get(pointed.id)?.item).title.length===0);
    const box=pointed&&(['chip','bucket'].includes(pointed.display)||unread)?pointed:null;
    // The pointer reaches these through the one hit test; the keyboard
    // reaches them as buttons, Enter doing what a click does.
    const press=target=>event=>{if(event.key!=='Enter'&&event.key!==' ')return;event.preventDefault();clickAt(target,event);};
    return <div className="scene-overlay">
      {items.filter(item=>item.type==='zoom').map(item=><button type="button" key={item.id} className="scene-zoom" data-zoom-into={item.box}
        tabIndex={tabIndex(item)}
        aria-label={t('Zoom into {0}',nameOf(item.box))} onKeyDown={press({type:'zoom',id:item.box})}
        style={{left:item.left-item.px/2,top:item.top-item.px/2,width:item.px,height:item.px}}><span className="flow-zoom-picture"/></button>)}
      {items.filter(item=>item.type!=='zoom').map(item=><React.Fragment key={item.id}><button type="button" className={`scene-mark scene-mark-${item.type==='port'?'port':item.side} ${emphasis.lit.has(item.id)||pointer?.id===item.id?'scene-mark-lit':''}`}
        tabIndex={tabIndex(item)}
        aria-label={item.type==='port'?`${item.way==='out'?'→':'←'} ${nameOf(item.id)}`:item.side==='in'?t(inputKindTitles[item.kind]||'Inputs'):t('Outside')}
        onKeyDown={press({type:item.type,id:item.id,box:item.box||'',item})}
        onFocus={()=>{look.enter(`mark:${item.id}`);store.dispatch({type:'look',key:look.key});store.dispatch({type:'point',target:{type:item.type,id:item.id,item}});}}
        onBlur={event=>{if(event.relatedTarget?.closest?.(`[data-card="${CSS.escape(`mark:${item.id}`)}"]`))return;look.leave(`mark:${item.id}`,performance.now());pump();}}
        data-marker={item.type==='marker'?item.id:undefined} data-marker-box={item.type==='marker'?item.box:undefined} data-marker-side={item.type==='marker'?item.side:undefined}
        data-marker-end={item.type==='marker'?(item.side==='out'?item.systems:item.members).join(' '):undefined} data-port={item.type==='port'?item.id:undefined} data-port-end={item.type==='port'?item.id:undefined}
        style={{left:item.left-item.px/2,top:item.top-item.px/2,width:item.px,height:item.px}}>
        <Mark icon={item.type==='port'?systemIcons.program:item.side==='in'?kindIcon(item.kind):systemIcons[item.kind]}/></button>
        {tip?.id===item.id&&<MarkTip item={tip}/>}</React.Fragment>)}
      {box&&<BoxTip node={box} camera={v}/>}
    </div>;
  }
  // What a box pointed at is: a chip's callers, a bucket's systems, else,
  // while its words are too small to read, its name and what it does. A
  // chip says the line saved under its name, as its card and the program's
  // catalogue do: a destination of several systems is "one of these,
  // depending on configuration" (its subtitle, said in the page's language).
  function BoxTip({node,camera:v}){
    const item=model.nodes.get(node.id)?.item;
    let head=node.kind==='inputs'?t('Inputs'):node.kind==='outside'?t('Outside'):node.title,names=[],about='';
    const note=node.display==='chip'?item?.subtitle||'':'';
    // A chip's name, drawn at twelve pixels, reads at nine and a half, as
    // every secondary word (scene.css --word).
    const faded=node.text*12*v.zoom<9.5;
    if(node.display==='chip'&&faded)return <div className="scene-tip scene-box-tip scene-tip-name" style={{left:(node.rect.x+node.rect.width)*v.zoom+v.x+6,top:Math.max(4,node.rect.y*v.zoom+v.y)}}><b>{head}</b>{note&&<span className="scene-tip-note">{note}</span>}</div>;
    if(node.display==='chip')names=[...new Set([...(model.callers.get(node.id)||[])].map(part=>model.nodes.get(part)?.name||''))].map(name=>`← ${name}`);
    else if(node.display==='bucket')names=model.nodes.get(node.id).children.map(id=>model.nodes.get(id)?.name||'');
    else about=item?.role||item?.summary||'';
    const left=(node.rect.x+node.rect.width)*v.zoom+v.x+6,top=node.rect.y*v.zoom+v.y;
    return <div className={`scene-tip scene-box-tip ${names.length>12?'scene-tip-columns':''}`} style={{left,top:Math.max(4,top)}}>
      <b>{head}</b>{note&&<span className="scene-tip-note">{note}</span>}{about&&<span>{about}</span>}{names.map((name,i)=><span key={i}>{name}</span>)}</div>;
  }
  // What a marker or a port stands for, one name to a line, each whole,
  // wrapped where its side has no room for it: names told apart at their
  // ends had all read "POST · /v3electionpb.…" (etcd's Server and Client
  // registrations, final journeys 2026-10-02). It stands on its marker's
  // side (an input's left, else right), or on the side with more room
  // where that one has too little, and within the canvas.
  function MarkTip({item}){
    const key=`mark:${item.id}`,ref=useRef(null),[shift,setShift]=useState(0);
    let head='',names=[];
    if(item.type==='port'){head=item.way==='out'?'→':'←';names=item.programs.map(id=>({id,name:nameOf(id)}));}
    else if(item.side==='in'){
      head=t(inputKindTitles[item.kind]||'Inputs');
      names=item.members.map(id=>({id,name:`${model.nodes.get(id)?.name||''}${item.handled.includes(id)?'':` · ${t('declared here')}`}`}));
    }else{
      head=item.kind==='database'?t('Database'):item.kind==='request'?t('Request'):item.kind==='sdk'?t('SDK'):item.kind==='queue'?t('Queue'):item.kind==='started'?t('Runs a program'):t('Outside');
      names=item.systems.map(id=>({id,name:model.nodes.get(id)?.name||''}));
    }
    const shown=names,{width,height}=size(),gap=6,edge=4,pad=20,between=14;
    const widest=shown.reduce((width,row)=>Math.max(width,measure(row.name,'13px system-ui')),measure(head,'650 13px system-ui'));
    const room={left:item.left-item.px/2-gap-edge,right:width-(item.left+item.px/2+gap)-edge};
    const needs=columns=>columns*Math.ceil(widest+1)+(columns-1)*between+pad;
    const own=item.side==='in'?'left':'right',other=own==='left'?'right':'left';
    const side=room[own]>=needs(1)||room[own]>=room[other]?own:other;
    // A long list stands in two columns where each has room for half the
    // widest name.
    const columns=shown.length>12&&room[side]>=2*Math.min(widest,200)+between+pad?2:1;
    const tipWidth=Math.max(120,Math.min(room[side],needs(columns)));
    const top=item.top-item.px/2;
    useLayoutEffect(()=>{
      const h=ref.current?.offsetHeight||0,want=Math.max(edge,Math.min(top,height-h-edge))-top;
      if(Math.abs(want-shift)>.5)setShift(want);
    });
    const keep=()=>{look.enter(key);store.dispatch({type:'look',key});};
    return <div ref={ref} data-card={key} className={`scene-tip scene-mark-tip flow-floating-card nodrag nopan nowheel ${columns>1?'scene-tip-columns':''}`}
      onMouseEnter={keep} onFocusCapture={keep}
      onMouseLeave={event=>{if(event.currentTarget.contains(document.activeElement))return;look.leave(key,performance.now(),{x:event.clientX,y:event.clientY},ref.current?.getBoundingClientRect());pump();}}
      onBlurCapture={event=>{if(!event.currentTarget.contains(event.relatedTarget)){look.leave(key,performance.now());pump();}}}
      onPointerDownCapture={event=>event.stopPropagation()} onMouseDownCapture={event=>event.stopPropagation()}
      style={{width:tipWidth,maxHeight:Math.max(60,height-2*edge),top:top+shift,left:side==='right'?item.left+item.px/2+gap:item.left-item.px/2-gap-tipWidth}}>
      <b>{head}</b>{shown.map(row=><button type="button" className="flow-card-name" data-marker-member={row.id} key={row.id}
        onClick={event=>{event.stopPropagation();look.end();store.dispatch({type:'look',key:''});read(row.id,event);}}>{row.name}</button>)}</div>;
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
    const choose={
      can:(part,key)=>(model.nodes.get(part)?.item?.symbols||[]).some(symbol=>symbolKey(symbol)===key),
      go:(part,key)=>chooseMember(part,(model.nodes.get(part)?.item?.symbols||[]).findIndex(symbol=>symbolKey(symbol)===key)),
      input:id=>read(id),
    };
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
        <div className="flow-card-body"><BriefRows card={card} into={nameOf(into)} choose={choose}/></div>
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
        style:{width:node.rect.width,height:node.rect.height,'--scene-text':node.text},zIndex:node.band===bands.frame?-1:2,
        selectable:false,draggable:false,connectable:false,focusable:false,
        className:`scene-node scene-is-${node.display} ${emphasis.nodeClass.get(node.id)||''}`,
        data:{node,item,groups,kind:(entry,event)=>clickAt({type:'box',id:node.id,node,kind:entry.kind,group:entry.group},event),open:(id,event)=>{if(event.detail>0&&down?.moved)return;clickAt({type:'zoom',id},event);},lit:lit.has(node.id),tile:node.display==='card'&&state.member.chosen?.part===node.id?item?.symbols?.[state.member.chosen.index]||null:null,keys:node.display==='chip'||node.display==='bucket'?keysOf(node):undefined,
          drawn:node.display==='deep'?geometry.grids.get(node.id):undefined,
          member:node.display==='deep'?{hot:pointed?.part===node.id?pointed.index:-1,chosen:chosen?.part===node.id?chosen.index:-1,point:()=>{},choose:()=>{}}:undefined}};
    }),[scene,emphasis,state.lit,pointed?.part,pointed?.index,chosen,state.member.chosen]);
    const edges=useMemo(()=>{
      const byID=new Map(scene.edges.map(edge=>[edge.id,edge])),drawn=new Set(scene.nodes.map(node=>node.id));
      return emphasis.order.filter(id=>!emphasis.edgeState.get(id).hidden).map(id=>{
      const edge=byID.get(id),flags=emphasis.edgeState.get(id);
      const ends=[edge.from,edge.to].map(end=>drawn.has(end)?end:scene.program||scene.nodes[0]?.id);
      return {id,source:ends[0],target:ends[1],type:'scene',zIndex:flags.on?1:0,selectable:false,focusable:false,data:{...edge,...flags}};
    });},[scene,emphasis]);
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
      <ZoomVar/>
      <Overlay scene={scene} emphasis={emphasis}/>
      {cards.map(key=><ArrowCard key={key} edgeKey={key} scene={scene}/>)}
    </ReactFlow>;
  }
  // Far enough into a part to read its declarations, and a little more.
  const minZoom=()=>Math.min(.05/(geometry.unit||1),geometry.home.zoom*.5);
  const maxZoom=()=>Math.max(4/(geometry.unit||1),...[...geometry.enterZoom.values()].map(zoom=>zoom*1.6),...[...geometry.grids.keys()].map(id=>levelZoom([id])*1.6));

  let restorePending=null;
  function capture(){return instance&&!initializing?{...camera.get(),level:store.getState().level,layoutKey,fit:overviewFit}:restorePending;}
  function restore(saved){
    if(!saved)return;
    if(!instance||initializing){restorePending=saved;return;}
    if(saved.fit||saved.layoutKey!==layoutKey||!Number.isFinite(saved.zoom)){fitOverview(0,saved.fit==='whole'?'whole':'rest');return;}
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
  // What the pointer reaches at a point of the page, by the one hit test a
  // hover and a click use (checks: the arrows take no pointer event).
  map.sceneHitAt=(clientX,clientY)=>{
    const target=hitTest(sceneOf(store.getState()),worldAt({clientX,clientY}),camera.get().zoom,shownEdge);
    return target?{type:target.type,id:target.id}:null;
  };
  // Enter the level showing `id`, as its magnifier does (probes and checks).
  map.sceneEnter=id=>enter(chainOf(model,id));
  // The whole map at rest, as the page opens on it (checks).
  map.sceneRest=()=>fitOverview(0,'rest');
  map.querySelector('[data-map-controls]')&&(map.querySelector('[data-map-controls]').hidden=false);
  const overviewButton=map.querySelector('[data-map-fit]');
  if(overviewButton){overviewButton.textContent=t('Show whole map');overviewButton.removeAttribute('title');}
  map.querySelector('[data-map-controls]')?.addEventListener('click',event=>{
    const button=event.target.closest('button');if(!button||!instance)return;
    if(button.hasAttribute('data-map-fit'))fitOverview(420,'whole');
    else if(button.hasAttribute('data-map-zoom')&&Number(button.dataset.mapZoom)<1)stepOut();
    else if(button.hasAttribute('data-map-zoom')){
      // "+" is a zoom at the canvas's centre: it may enter a level.
      const box=host.getBoundingClientRect(),previous=camera.get().zoom;gestureAim={x:box.left+box.width/2,y:box.top+box.height/2};
      Promise.resolve(instance.zoomTo(previous*Number(button.dataset.mapZoom),{duration:200})).then(()=>{
        const viewport=instance.getViewport();camera.set(viewport);lastZoom=viewport.zoom;levelForZoom(previous,viewport);gestureAim=null;});
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
      // The whole map is fitted again; a level entered is framed again in
      // its new place.
      const level=store.getState().level;
      if(overviewFit||!level.length)fitOverview(0,overviewFit||'rest');
      else moveCamera(frameCamera(sceneOf(store.getState()).focus,levelZoom(level)),false);
    },250);
  }).observe(host);

  const mounted=new Map();
  function mountConnections(container,id,open='',choose=null,readInput=null,apart=null){
    const groups=frameConnections(id).map(group=>apart&&apart(group.outside)?{...group,apart:true}:group).sort((a,b)=>(a.apart?1:0)-(b.apart?1:0));
    if(!groups.length)return false;
    for(const [element,mountedRoot] of mounted)if(!element.isConnected){mountedRoot.unmount();mounted.delete(element);}
    const target=mounted.get(container)||createRoot(container);mounted.set(container,target);
    const chooser=choose&&{go:choose,can:(part,key)=>!!key&&(model.nodes.get(part)?.item?.symbols||[]).some(symbol=>symbolKey(symbol)===key),input:readInput};
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
    overview:()=>fitOverview(420,'whole'),
    update(next){
      const chosen=store.getState().member.chosen;
      if(chosen&&model.shown(next.scope||'')!==chosen.part)store.dispatch({type:'member',member:{chosen:null}});
      const view={...next,scope:model.shown(next.scope||''),selected:new Set([...(next.selected||[])].map(model.shown)),matched:new Set([...(next.matched||[])].map(model.shown))};
      store.dispatch({type:'view',view});
    },
  };
}
