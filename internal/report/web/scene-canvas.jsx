// The scene canvas (PLAN): the page's data
// is built into a model once (model.mjs), every level is laid out once per
// canvas size (levels.mjs), and what is drawn is a pure function of the
// level and the reader's choice (scene.mjs). React Flow is the camera and a
// dumb renderer; one store holds the level, the pointer and the choice; one
// hitTest answers hover and click; the overlay keeps markers and ports at
// one screen size (overlay.mjs).
import React,{createContext,useContext,useEffect,useLayoutEffect,useMemo,useRef,useState,useSyncExternalStore} from 'react';
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
// A tile's declaration as the reading keys it: its link, else its place
// (26-map-members.js symbolKey; a page with no source link).
const symbolKey=symbol=>symbol?symbol.href||symbol.open||(symbol.path?JSON.stringify([symbol.path,symbol.line||0]):''):'';
const sameSet=(a,b)=>a.size===b.size&&[...a].every(id=>b.has(id));

// The words a box shows at its level's text size, each of its title, role
// and description whole or not at all (levels.mjs cardWords, programWords).
const wordsOf=(node,item)=>node.display==='program'?programWords({...node,item},node.rect.width/node.text,measure)
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

// Where the boxes inside a closed box stand, as faint outlines: never a
// blank box (PLAN B).
function Ghosts({node,className=''}){
  if(!node.ghosts?.length)return null;
  // Inside the card's border: the outline overflows nothing.
  const w=node.rect.width/node.text,h=node.rect.height/node.text;
  return <svg className={`scene-ghosts ${className}`} viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" aria-hidden="true">
    {node.ghosts.map((r,i)=><rect key={i} x={(r.x-node.rect.x)/node.text} y={(r.y-node.rect.y)/node.text} width={r.width/node.text} height={r.height/node.text} rx={Math.min(9,r.width/node.text/12)}/>)}</svg>;
}
function CardNode({data}){
  const {node}=data,item=data.item;
  const words=useMemo(()=>wordsOf(node,item),[node.rect.width,node.rect.height,node.text,node.title]);
  const held=useRef(null),inSight=useWordsInSight(node,held);
  // A closed part holding the declaration chosen shows it alone as its
  // tile under its title (owner, 2026-09-29), in place of its description.
  if(data.tile&&words.title.length)return <>{handles}<Scaled node={node} className={`flow-part ${laneClass(node.lane)}`}>
    <div className="scene-words" ref={held} style={inSight}><strong data-box-title={node.id}>{words.title.join('\n')}</strong>{' '}
      <span className="scene-chosen-tile" title={data.tile.full||data.tile.name}>{data.tile.name}</span></div></Scaled></>;
  return <>{handles}<Scaled node={node} className={`flow-part ${node.display==='area'?'scene-area-card':''} ${laneClass(node.lane)}`} data={node.display==='area'?{'data-summary-area':node.id}:undefined}>
    <Ghosts node={node}/>
    {node.display==='area'&&['core','triggers'].includes(node.lane)&&<span className={`flow-role-symbol flow-role-${node.lane}`} aria-hidden="true"/>}
    <div className="scene-words" ref={held} style={inSight}>{words.title.length>0&&<strong data-box-title={node.id}>{words.title.join('\n')}</strong>}{' '}
    {words.lines.length>0&&<div className="flow-description flow-description-lines">{words.lines.join('\n')}</div>}</div>
  </Scaled></>;
}
function ProgramNode({data}){
  const {node}=data,item=data.item;
  const words=useMemo(()=>wordsOf(node,item),[node.rect.width,node.rect.height,node.title]);
  const held=useRef(null),inSight=useWordsInSight(node,held);
  return <>{handles}<Scaled node={node} className="scene-program-card" data={{'data-component-overview':node.id}}>
    <Ghosts node={node} className="scene-program-ghosts"/>
    <div className="scene-words" ref={held} style={inSight}><strong data-box-title={node.id}>{words.title.join('\n')}</strong>{' '}
    {words.role.length>0&&<div className="flow-component-role">{words.role.join('\n')}</div>}{' '}
    {words.purpose.length>0&&<p className="flow-description flow-description-lines">{words.purpose.join('\n')}</p>}</div>
  </Scaled></>;
}
// The camera and the canvas, for what a box keeps in sight (useStuck).
const SceneCamera=createContext(null);
const noSubscribe=()=>()=>{};
// A frame's title stays in sight while its body is (owner via the
// coordinator, 2026-10-02: an entered input's Inputs frame had read
// "puts", othello's program "hello"): where the frame's left edge is out of
// the canvas, its title moves in along its own band, never out of the
// frame (an offset in world units); where its top is, the overlay names it
// at the canvas's top (StuckTitles).
function useStuck(node,titleWidth){
  const scene=useContext(SceneCamera);
  const key=useSyncExternalStore(scene?scene.camera.subscribe:noSubscribe,()=>{
    if(!scene)return 0;
    const v=scene.camera.get(),{width}=scene.size(),z=v.zoom,r=node.rect,pad=6;
    const left=r.x*z+v.x,right=left+r.width*z;
    if(right<titleWidth*z||left>width)return 0;
    return Math.round(Math.max(0,Math.min(pad-left,(r.width-titleWidth)*z))/z);
  });
  return key;
}
// A closed box's words stay in sight while the part of it in sight can
// hold them (owner via the coordinator, 2026-10-02: etcd's "gRPC proxy"
// and casdoor's "Email providers" had lost their names past the canvas's
// edge): where its left or top edge is out of the canvas, its words move
// in, never out of the box (an offset in the box's own pixels); where its
// right or bottom edge is, they move left or up into its padding, never
// past its border (synthetic-no-inputs' Report queue: its neighbour
// "Invoice audit", cut at the canvas's foot, had its words cut 10 pixels
// though the card's top in sight held them; harness table, 60e7ea98).
// The words are measured once, at their own size; drawn larger than 1.6
// times it they are drawn at that (scene.css), a block centred in its box
// (`centred`, a part-group's) staying centred.
function useWordsInSight(node,ref,centred=false){
  const scene=useContext(SceneCamera);
  const [at,setAt]=useState(null);
  // A card's border keeps its screen width (scene.css): its border and
  // padding stand 14 of its pixels deep while the border is the thinner,
  // and the words are measured again when that begins (measured on a far
  // camera, they had stood at the card's very edge).
  const thin=useSyncExternalStore(scene?scene.camera.subscribe:noSubscribe,()=>!!scene&&scene.camera.get().zoom*node.text>=.1);
  useLayoutEffect(()=>{
    const el=ref.current;if(!el)return;
    // At their own size, whatever the camera (scene.css .scene-words-natural).
    el.classList.add('scene-words-natural');
    let x=0,y=0;for(let a=el;a&&!a.classList.contains('scene-scaled');a=a.offsetParent){x+=a.offsetLeft;y+=a.offsetTop;}
    const width=el.offsetWidth,height=el.offsetHeight;
    // The gaps between the words' lines keep their size when the words are
    // drawn smaller (scene.css .scene-words).
    const gaps=Math.max(0,height-[...el.children].reduce((sum,child)=>sum+child.offsetHeight,0));
    el.classList.remove('scene-words-natural');
    // From the box's outer edge, its border counted.
    const scaled=el.closest('.scene-scaled'),edges=scaled?getComputedStyle(scaled):null;
    const bl=edges?parseFloat(edges.borderLeftWidth)||0:0,bt=edges?parseFloat(edges.borderTopWidth)||0:0;
    x+=bl;y+=bt;
    // How far they may go: the inside of the box that holds them (a card's
    // padding, a part-group's front card).
    const holder=el.offsetParent||el.parentElement,style=getComputedStyle(holder);
    const from=holder.classList.contains('scene-scaled')?{x:bl,y:bt}:{x:holder.offsetLeft+bl,y:holder.offsetTop+bt};
    const right=from.x+holder.clientWidth-parseFloat(style.paddingRight||'0'),bottom=from.y+holder.clientHeight-parseFloat(style.paddingBottom||'0');
    setAt({x,y:centred?y+height/2:y,width,height,gaps,right,bottom});
  },[node.rect.width,node.rect.height,node.text,node.title,thin]);
  const key=useSyncExternalStore(scene?scene.camera.subscribe:noSubscribe,()=>{
    if(!scene||!at)return '';
    const v=scene.camera.get(),{width:W,height:H}=scene.size(),z=v.zoom*node.text,r=node.rect,pad=6,border=4,capped=Math.min(1,1.6/z);
    const width=at.width*capped,height=(at.height-at.gaps)*capped+at.gaps,y=centred?at.y-height/2:at.y;
    const left=r.x*v.zoom+v.x+at.x*z,top=r.y*v.zoom+v.y+y*z;
    let dx=Math.max(0,Math.min((pad-left)/z,at.right-at.x-width));
    // Under the frames' names at the canvas's top, never behind them.
    const shifted=left+dx*z,below=scene.under?.(shifted,shifted+width*z)||0;
    let dy=Math.max(0,Math.min((Math.max(pad,below+pad)-top)/z,at.bottom-y-height));
    // Left or up only where the box's part in sight holds them, from its
    // own edge or the canvas's: a box all but out of sight keeps them where
    // they stand (casdoor's Outside buckets at the canvas's foot had had
    // their centred words pulled to their tops).
    const roomX=(W-pad)-Math.max(r.x*v.zoom+v.x,pad),roomY=(H-pad)-Math.max(r.y*v.zoom+v.y,pad);
    if(!dx&&roomX>=width*z)dx=-Math.max(0,Math.min((left+width*z-(W-pad))/z,at.x-border));
    if(!dy&&roomY>=height*z)dy=-Math.max(0,Math.min((top+height*z-(H-pad))/z,y-border));
    return Math.abs(dx)>=.5||Math.abs(dy)>=.5?`${Math.round(dx)}px,${Math.round(dy)}px`:'';
  });
  return key?{transform:`translate(${key})`}:undefined;
}
function FrameNode({data}){
  const {node}=data,kind=node.kind;
  const tone=kind==='program'?'flow-component':kind==='outside'||kind==='bucket'?'flow-communication':kind==='inputs'?'flow-input-collection'
    :node.lane==='core'?'flow-area-core':node.lane==='triggers'?'flow-area-entry':'';
  const title=kind==='outside'?t('Outside'):kind==='inputs'?t('Inputs'):node.title;
  const titleText=node.titleText||node.text,band=units.band(1)*titleText;
  const dx=useStuck(node,Math.min(node.rect.width,(measure(title,'700 17px system-ui')+40)*titleText));
  const scene=useContext(SceneCamera);
  const named=useSyncExternalStore(scene?scene.camera.subscribe:noSubscribe,()=>!!scene?.stuck(node.id));
  return <>{handles}<div className={`flow-area ${tone} ${node.bucket?'scene-bucket-open':''}`} style={{width:node.rect.width,height:node.rect.height}}>
    <div className="scene-frame-title-place" style={dx?{transform:`translate(${dx}px,0)`}:undefined}>
    <Scaled node={{...node,text:titleText,rect:{...node.rect,height:band}}} className="scene-frame-title" style={{'--scene-text':titleText}}>
      <strong data-box-title={node.id} data-frame-title={node.id} data-title-stuck={dx?'':undefined} style={named?{visibility:'hidden'}:undefined}>{title}</strong></Scaled></div>
  </div></>;
}
// An Inputs frame closed: its title over its kinds' marks, each named on
// hover; entered, the kinds' groups name their inputs.
function InputsNode({data}){
  const {node,groups}=data;
  return <>{handles}<Scaled node={node} className="flow-area flow-input-collection scene-inputs-card" data={{'data-component-overview':node.id}}>
    <strong data-box-title={node.id}>{t('Inputs')}</strong>
    {node.kinds.map((entry,i)=><span key={entry.kind} className={`scene-kind-mark ${groups[i]?.lit?'flow-lit':''}`} title={t(inputKindTitles[entry.kind])}
      data-input-group-kind={entry.kind} style={{left:(entry.rect.x-node.rect.x)/node.text,top:(entry.rect.y-node.rect.y)/node.text,width:entry.rect.width/node.text,height:entry.rect.height/node.text}}>
      <Mark icon={kindIcon(entry.kind)} kind={entry.kind}/></span>)}
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
  const held=useRef(null),inSight=useWordsInSight(node,held,true);
  return <>{handles}<Scaled node={node} className="flow-chip scene-bucket scene-bucket-stack" data={focusable(data)}>
    <span className="scene-bucket-face"><span className="scene-bucket-words" ref={held} style={inSight}><span className="flow-chip-name" title={node.title}>{node.title}</span>
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
  // The frames named at the canvas's top (stuckNames), for a frame to hide
  // what is left of its own title under its name there.
  let stuckMemo={list:[]};
  const sceneCamera={camera,size,stuck:id=>stuckNames(sceneOf(store.getState()),camera.get()).some(name=>name.id===id),
    // How far down the frames' names at the canvas's top reach across a
    // stretch of it.
    under:(left,right)=>Math.max(0,...stuckNames(sceneOf(store.getState()),camera.get()).filter(name=>name.x<right&&name.right>left).map(name=>name.bottom))};
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
  function frameCamera(rect,minZoom=0,pad=28,maxZoom=Infinity){
    const {width,height}=size(),fit=Math.min((width-2*pad)/rect.width,(height-2*pad)/rect.height);
    const zoom=Math.max(Math.min(fit,maxZoom),minZoom);
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
  // No closer than its boxes' words read at a third over their reading
  // size: one box framed alone keeps its words at reading size and its
  // neighbours in sight (owner via the coordinator, 2026-10-02: casdoor's
  // Custom Logout Endpoint had filled the canvas in 90-pixel letters).
  const readCap=level=>1.35/(geometry.text.get(level.at(-1))||geometry.unit||1);
  function enter(level,{rect=null,smooth=true}={}){
    store.dispatch({type:'enter',level});
    const scene=sceneOf(store.getState());
    return moveCamera(frameCamera(rect||scene.focus,levelZoom(level),28,readCap(level)),smooth);
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
    const level=node.kind==='part'?chainOf(model,id).filter(at=>at!==id):enterLevelOf(id);
    const rect=geometry.boxes.get(id)||geometry.scales.get(id)?.frame;
    // A thing in sight, or the level itself, is only marked: the camera
    // moves only to what is out of sight (owner, 2026-09-28).
    const current=store.getState().level;
    if(!center&&(current.includes(id)||sceneOf(store.getState()).nodes.some(node=>node.id===id)&&rect&&inSight(rect)))return;
    store.dispatch({type:'enter',level});
    if(rect)moveCamera(frameCamera(rect,levelZoom(level),28,readCap(level)),smooth);
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
    return {part:member.part,names:[symbol.name,symbol.full||'',owner?`${owner}.${symbol.name}`:''].filter(Boolean),sources:[symbol.href,symbol.open,symbolKey(symbol)].filter(Boolean)};
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
    const same=value=>!!value&&[named.key,named.href,named.open].includes(value);
    const index=symbols.findIndex(symbol=>same(symbol.href)||same(symbol.open)||same(symbolKey(symbol)));
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
    if(target.kind){
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
  const onCard=event=>!!event.target.closest?.('.flow-floating-card');
  // A press that moves the pointer or the camera is a drag: the click
  // ending it acts on nothing.
  // A press ends a look not yet opened: an arrow's card does not open under
  // the pointer while it drags the map, at the place it was pressed.
  host.addEventListener('pointerdown',event=>{const v=camera.get();down={x:event.clientX,y:event.clientY,camera:{x:v.x,y:v.y,zoom:v.zoom},moved:false};if(look.pending&&!onCard(event))look.abandon(look.pending);},true);
  host.addEventListener('pointermove',event=>{if(down&&event.buttons&&Math.hypot(event.clientX-down.x,event.clientY-down.y)>4)down.moved=true;},true);
  host.addEventListener('pointermove',event=>{
    if(initializing||event.buttons)return;
    if(onCard(event))return;
    const scene=sceneOf(store.getState()),target=hitTest(scene,worldAt(event),camera.get().zoom,shownEdge);
    host.classList.toggle('scene-pointing-arrow',target?.type==='edge');
    // While a card is open or kept, what is being read stays: the way to
    // the card crosses boxes and empty canvas without changing the emphasis.
    const reading=!!look.key||store.getState().pinned.length>0;
    if(!reading||target?.type==='edge')store.dispatch({type:'point',target:target?.type==='edge'?{...target,at:undefined}:target});
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
  host.addEventListener('pointerleave',event=>{if(pressed||event.buttons)return;host.classList.remove('scene-pointing-arrow');store.dispatch({type:'point',target:null});});
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
    const state=useSyncExternalStore(store.subscribe,store.getState);
    const pointer=state.pointer;
    const tip=pointer&&(pointer.type==='marker'||pointer.type==='port')?items.find(item=>item.id===pointer.id):null;
    // A chip or a bucket pointed at says who calls it, or what it holds, a
    // chip whose name does not read naming it alone; any other box whose
    // words do not read yet is named.
    const pointed=pointer?.type==='box'?pointer.node:null;
    const unread=pointed&&(pointed.text*17*v.zoom<11||['card','area','program'].includes(pointed.display)&&wordsOf(pointed,model.nodes.get(pointed.id)?.item).title.length===0);
    const box=pointed&&(['chip','bucket'].includes(pointed.display)||unread)?pointed:null;
    // The pointer reaches these through the one hit test; the keyboard
    // reaches them as buttons, Enter doing what a click does.
    const press=target=>event=>{if(event.key!=='Enter'&&event.key!==' ')return;event.preventDefault();clickAt(target,event);};
    // Frames named at the canvas's top stand under the marks and the tips,
    // in a layer of their own that holds them whole (StuckTitles).
    return <><div className="scene-stuck-titles"><StuckTitles scene={scene} v={v}/></div><div className="scene-overlay">
      {items.filter(item=>item.type==='zoom').map(item=><button type="button" key={item.id} className="scene-zoom" data-zoom-into={item.box}
        aria-label={t('Zoom into {0}',nameOf(item.box))} onKeyDown={press({type:'zoom',id:item.box})}
        style={{left:item.left-item.px/2,top:item.top-item.px/2,width:item.px,height:item.px}}><span className="flow-zoom-picture"/></button>)}
      {items.filter(item=>item.type!=='zoom').map(item=><button type="button" key={item.id} className={`scene-mark scene-mark-${item.type==='port'?'port':item.side} ${emphasis.lit.has(item.id)||pointer?.id===item.id?'scene-mark-lit':''}`}
        aria-label={item.type==='port'?`${item.way==='out'?'→':'←'} ${nameOf(item.id)}`:item.side==='in'?t(inputKindTitles[item.kind]||'Inputs'):t('Outside')}
        onKeyDown={press({type:item.type,id:item.id,box:item.box||'',item})}
        data-marker={item.type==='marker'?item.id:undefined} data-marker-box={item.type==='marker'?item.box:undefined} data-marker-side={item.type==='marker'?item.side:undefined}
        data-marker-end={item.type==='marker'?(item.side==='out'?item.systems:item.members).join(' '):undefined} data-port={item.type==='port'?item.id:undefined} data-port-end={item.type==='port'?item.id:undefined}
        style={{left:item.left-item.px/2,top:item.top-item.px/2,width:item.px,height:item.px}}>
        <Mark icon={item.type==='port'?systemIcons.program:item.side==='in'?kindIcon(item.kind):systemIcons[item.kind]}/></button>)}
      {tip&&<MarkTip key={tip.id} item={tip}/>}
      {box&&<BoxTip node={box} camera={v}/>}
    </div></>;
  }
  // A frame whose top is out of the canvas while its body fills it is named
  // at the canvas's top, at its title's size, along its own left edge. The
  // frames it sits inside are named first, a row above it where their
  // names would overlap (etcd's "server (executable)" over "Client APIs"),
  // and a frame whose own title such a name would cover is named below it.
  function stuckNames(scene,v){
    const {width,height}=size(),pad=6,z=v.zoom,placed=[],list=[];
    if(stuckMemo.scene===scene&&stuckMemo.v===v&&stuckMemo.width===width&&stuckMemo.height===height)return stuckMemo.list;
    const frames=scene.nodes.filter(node=>node.display==='frame').sort((a,b)=>b.rect.width*b.rect.height-a.rect.width*a.rect.height);
    const across=(p,left,right)=>Math.min(p.right,right)-Math.max(p.left,left)>0;
    // The first place from the canvas's top (hanging from it, over what is
    // left of the frame's own title) where a name of height h stands clear
    // of the names already placed across it.
    const clear=(left,right,h)=>{let y=0;for(let moved=true;moved;){moved=false;for(const p of placed)if(across(p,left,right)&&p.top<y+h&&p.bottom>y){y=p.bottom+2;moved=true;}}return y;};
    for(const node of frames){
      const r=node.rect,titleText=node.titleText||node.text,px=17*titleText*z;
      const left=r.x*z+v.x,top=r.y*z+v.y,right=left+r.width*z,bottom=top+r.height*z,band=units.band(1)*titleText*z;
      if(right<80||left>width-80||top>height||px<9)continue;
      const kind=node.kind,title=kind==='outside'?t('Outside'):kind==='inputs'?t('Inputs'):node.title;
      const font=Math.min(px,22),x=Math.max(left,0)+pad,room=Math.max(80,Math.min(right,width)-Math.max(left,0)-2*pad);
      const w=Math.min(room,measure(title,`700 ${font}px system-ui`)+22),h=font*1.3+4;
      // Its own title stands 10 of its pixels under its top (scene.css
      // .scene-frame-title): named here once any of it is above the canvas.
      const covered=placed.some(p=>across(p,x,x+w)&&p.top<top+band&&p.bottom>top);
      if(!covered&&top+10*titleText*z>=-.5)continue;
      const y=clear(x,x+w,h);
      if(bottom<y+h+2*band)continue;
      placed.push({left:x,right:x+w,top:y,bottom:y+h});
      const tone=kind==='program'?'flow-component':kind==='outside'||kind==='bucket'?'flow-communication':kind==='inputs'?'flow-input-collection':'';
      list.push({id:node.id,title,tone,x,y,font,room,right:x+w,bottom:y+h});
    }
    stuckMemo={scene,v,width,height,list};
    return list;
  }
  function StuckTitles({scene,v}){
    return stuckNames(scene,v).map(name=><div key={name.id} className={`scene-stuck-title ${name.tone}`} data-stuck-title={name.id} title={name.title}
      style={{left:name.x,top:name.y,fontSize:name.font,maxWidth:name.room}}>{name.title}</div>);
  }
  // What a box pointed at is: a chip's callers, a bucket's systems, else,
  // while its words are too small to read, its name and what it does.
  function BoxTip({node,camera:v}){
    const item=model.nodes.get(node.id)?.item;
    let head=node.kind==='inputs'?t('Inputs'):node.kind==='outside'?t('Outside'):node.title,names=[],about='';
    // A chip's name, drawn at twelve pixels, reads at nine and a half, as
    // every secondary word (scene.css --word).
    const faded=node.text*12*v.zoom<9.5;
    if(node.display==='chip'&&faded)return <div className="scene-tip scene-box-tip scene-tip-name" style={{left:(node.rect.x+node.rect.width)*v.zoom+v.x+6,top:Math.max(4,node.rect.y*v.zoom+v.y)}}><b>{head}</b></div>;
    if(node.display==='chip')names=[...new Set([...(model.callers.get(node.id)||[])].map(part=>model.nodes.get(part)?.name||''))].map(name=>`← ${name}`);
    else if(node.display==='bucket')names=model.nodes.get(node.id).children.map(id=>model.nodes.get(id)?.name||'');
    else about=item?.role||item?.summary||'';
    const left=(node.rect.x+node.rect.width)*v.zoom+v.x+6,top=node.rect.y*v.zoom+v.y;
    return <div className={`scene-tip scene-box-tip ${names.length>12?'scene-tip-columns':''}`} style={{left,top:Math.max(4,top)}}>
      <b>{head}</b>{about&&<span>{about}</span>}{names.map((name,i)=><span key={i}>{name}</span>)}</div>;
  }
  // What a marker or a port stands for, one name to a line, each whole,
  // wrapped where its side has no room for it: names told apart at their
  // ends had all read "POST · /v3electionpb.…" (etcd's Server and Client
  // registrations, final journeys 2026-10-02). It stands on its marker's
  // side (an input's left, else right), or on the side with more room
  // where that one has too little, and within the canvas.
  function MarkTip({item}){
    const most=30,ref=useRef(null),[shift,setShift]=useState(0);
    let head='',names=[];
    if(item.type==='port'){head=item.way==='out'?'→':'←';names=item.programs.map(nameOf);}
    else if(item.side==='in'){
      head=t(inputKindTitles[item.kind]||'Inputs');
      names=item.members.map(id=>`${model.nodes.get(id)?.name||''}${item.handled.includes(id)?'':` · ${t('declared here')}`}`);
    }else{
      head=item.kind==='database'?t('Database'):item.kind==='request'?t('Request'):item.kind==='sdk'?t('SDK'):item.kind==='queue'?t('Queue'):item.kind==='started'?t('Runs a program'):t('Outside');
      names=item.systems.map(id=>model.nodes.get(id)?.name||'');
    }
    const shown=names.slice(0,most),{width,height}=size(),gap=6,edge=4,pad=20,between=14;
    const widest=Math.max(measure(head,'650 13px system-ui'),...shown.map(name=>measure(name,'13px system-ui')));
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
    return <div ref={ref} className={`scene-tip scene-mark-tip ${columns>1?'scene-tip-columns':''}`}
      style={{width:tipWidth,top:top+shift,...(side==='right'?{left:item.left+item.px/2+gap}:{right:`calc(100% - ${item.left-item.px/2-gap}px)`})}}>
      <b>{head}</b>{shown.map((name,i)=><span key={i}>{name}</span>)}{names.length>most&&<span>…</span>}</div>;
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
        style:{width:node.rect.width,height:node.rect.height,'--scene-text':node.text},zIndex:node.band===bands.frame?-1:2,
        selectable:false,draggable:false,connectable:false,focusable:false,
        className:`scene-node scene-is-${node.display} ${emphasis.nodeClass.get(node.id)||''}`,
        data:{node,item,groups,lit:lit.has(node.id),tile:node.display==='card'&&state.member.chosen?.part===node.id?item?.symbols?.[state.member.chosen.index]||null:null,keys:node.display==='chip'||node.display==='bucket'?keysOf(node):undefined,
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
    return <SceneCamera.Provider value={sceneCamera}><ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} zIndexMode="manual"
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
    </ReactFlow></SceneCamera.Provider>;
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
