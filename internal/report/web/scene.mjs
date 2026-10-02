// What the scene canvas draws at a level (PLAN S3), as pure functions of
// the geometry (levels.mjs), the level and the reader's choice:
//   sceneAt     the boxes, arrows, markers and ports of a level, each with
//               its drawing band, and the level's one text scale;
//   emphasisOf  what the pointer and the choice change: classes and order,
//               never geometry;
//   hitTest     the one answer to "what is under this point", for hover
//               and click alike;
//   levelAfterZoom  the level a zoom gesture brings, with hysteresis; a
//               pan never changes it.
import {emphasis,recedes,focusAncestors} from './emphasis.mjs';
import {overlayAt} from './overlay.mjs';
import {tileRoom,tileHeader} from './symbols.mjs';

// What can be entered: a level of its own opens inside it (a program
// holding nothing has none).
export function enterable(model,id){
  const node=model.nodes.get(id);if(!node)return false;
  if(['program','area','inputs','bucket'].includes(node.kind))return node.children.length>0;
  return node.kind==='part'&&(node.item?.symbols?.length||0)>0;
}
// The level that shows `id`: the enterable boxes from the root down to it.
export function chainOf(model,id){
  if(!id||!model.nodes.has(id))return [];
  return [...model.ancestors(id).reverse(),id].filter(at=>enterable(model,at));
}
export const levelKey=level=>level.join('/');
// Drawing bands, from the bottom: open frames, grey arrows, dark arrows,
// boxes. Markers and ports stand over them in the overlay.
export const bands={frame:0,arrow:1,dark:2,box:3};

// The scene at `level` (a chain from chainOf). `selection` {scope} names
// the box the reader has chosen: its quiet arrows are drawn.
export function sceneAt(model,geometry,level=[],selection={}){
  const nodes=[],edges=[],markers=[],ports=[],members=[];
  const inner=level.at(-1)||'',top=level[0]||'';
  const program=top&&model.nodes.get(top)?.kind==='program'?top:'';
  const rectOf=id=>geometry.boxes.get(id);
  const frameOf=id=>geometry.scales.get(id)?.frame||rectOf(id);
  const box=(id,display,rect,text,extra={})=>{
    const node=model.nodes.get(id);
    nodes.push({id,kind:node?.kind||'',display,rect,text,band:display==='frame'?bands.frame:bands.box,
      enter:enterable(model,id),title:node?.name||'',...extra});
  };
  // The boxes the reader looks at: their quiet arrows are drawn.
  const looked=new Set([inner,selection.scope].filter(Boolean));
  // The whole map's boxes are drawn at the world's unit, a program's card
  // at the text its name reads at (levels.mjs programText).
  const unit=geometry.unit||1;
  let text=unit;
  if(!program){
    const open=new Set(level);
    text=inner?geometry.text.get(inner)||unit:unit;
    for(const id of model.roots){
      const node=model.nodes.get(id),rect=rectOf(id);if(!rect)continue;
      // A program's card holds its drawing's outline: where its areas and
      // loose parts stand, seen once the card is drawn large.
      if(node.kind==='program')box(id,'program',rect,geometry.programText||unit,{ghosts:node.children.map(child=>rectOf(child)).filter(Boolean)});
      else if(node.kind==='inputs'){
        // Closed: its kinds' marks in a row under its title, each read alone.
        if(!open.has(id)){box(id,'inputs',rect,unit,{kinds:node.children.map((group,i)=>({kind:model.nodes.get(group).inputKind,group,
          rect:{x:rect.x+(16+i*22)*unit,y:rect.y+40*unit,width:18*unit,height:18*unit}}))});continue;}
        // Open, its frame is its box: the map's arrows end on it.
        const t=geometry.text.get(id);
        box(id,'frame',rect,t,{title:node.name});
        for(const group of node.children){
          box(group,'kindgroup',rectOf(group),t,{inputKind:model.nodes.get(group).inputKind});
          for(const input of model.nodes.get(group).children)box(input,'tile',rectOf(input),t,{inputKind:model.nodes.get(input).inputKind});
        }
      }
      else if(node.kind==='outside'){
        // An Outside frame's systems are drawn at the text their names read
        // at, its title at the programs' beside it (levels.mjs systemText).
        const named=geometry.text.get(id)||unit;
        box(id,'frame',rect,named,{title:node.name,titleText:geometry.programText||unit});
        for(const child of node.children){
          const item=model.nodes.get(child);
          if(item.kind==='system')box(child,'chip',rectOf(child),named,{systemKind:item.systemKind});
          else if(item.kind==='bucket'&&open.has(child)){
            const t=geometry.text.get(child);
            box(child,'frame',frameOf(child),t,{bucket:true});
            for(const system of item.children)box(system,'chip',rectOf(system),t,{systemKind:model.nodes.get(system).systemKind});
          }else if(item.kind==='bucket')box(child,'bucket',rectOf(child),named,{systemKinds:[...new Set(item.children.map(s=>model.nodes.get(s).systemKind))]});
        }
      }
      else if(node.kind==='note')box(id,'note',rect,unit);
      else if(node.kind==='system')box(id,'chip',rect,unit,{systemKind:node.systemKind});
      else if(node.kind==='input')box(id,'tile',rect,unit,{inputKind:node.inputKind});
      else box(id,'card',rect,unit);
    }
    // A pair of programs joined only by code use is drawn while one of them
    // is chosen, else only while one is pointed at (emphasisOf).
    for(const route of geometry.routes.get('')||[]){
      const edge=edgeOf(route,model,looked);
      // A part read chooses its program here.
      const chosen=selection.scope?model.rootOf(model.shown(selection.scope)):'';
      if(route.uses&&!looked.has(route.from)&&!looked.has(route.to)&&chosen!==route.from&&chosen!==route.to)edge.rest=false;
      edges.push(edge);
    }
    return finish();
  }
  // Inside a program: its frame, its areas closed but the one entered, its
  // loose parts; one text scale for every title at the level, the entered
  // area's. The program's own boxes beside the area entered keep the
  // program's text, their words as large as their cards (owner,
  // 2026-10-02: litestream's loose "Replica client backends" had read at
  // the area's text in a card twice its parts' size, four fifths empty).
  const programNode=model.nodes.get(program),partText=geometry.scales.get(program)?.scale||1;
  const own=geometry.text.get(program)||partText;
  text=level.length>1?geometry.text.get(level[1])||partText:own;
  box(program,'frame',frameOf(program),own,{program:true});
  for(const child of programNode.children){
    const node=model.nodes.get(child),rect=rectOf(child);if(!node||!rect)continue;
    if(node.kind==='area'&&level[1]===child){
      box(child,'frame',frameOf(child),text,{area:true,lane:node.item?.lane||''});
      for(const part of node.children){
        if(!rectOf(part))continue;
        box(part,level[2]===part?'deep':'card',rectOf(part),text,{lane:model.nodes.get(part).item?.lane||'',ghosts:tilesOf(part)});
        if(level[2]!==part)markersFor(part);else deepFor(part);
      }
      for(const route of geometry.routes.get(child)||[])edges.push(edgeOf(route,model,new Set([...looked,...node.children])));
    }else if(node.kind==='area'){
      // A closed area shows where its parts stand, never a blank box.
      box(child,'area',rect,own,{lane:node.item?.lane||'',ghosts:node.children.map(id=>rectOf(id)).filter(Boolean)});markersFor(child,own);
    }
    else{box(child,level[1]===child?'deep':'card',rect,own,{lane:node.item?.lane||'',ghosts:tilesOf(child)});if(level[1]!==child)markersFor(child,own);else deepFor(child);}
  }
  // A program's own arrows; those of the area entered count as looked at.
  const lookedHere=new Set([...looked,...(level[1]?[level[1]]:[])]);
  for(const route of geometry.routes.get(program)||[])edges.push(edgeOf(route,model,lookedHere));
  for(const port of geometry.ports.get(program)||[])
    ports.push({id:port.id,programs:port.programs,way:port.way,point:port.point,side:port.way==='out'?'east':'west',edges:port.edges});
  return finish();

  // A part entered: its declarations' tiles, each a member the pointer
  // finds, its markers on the declarations they stand for (a row of them
  // beside the tile), the rest on the part's edge.
  // A declaration's tile in its part's card, wherever that card stands.
  function tileRect(r,drawn,row){
    const {grid,box:card}=drawn,s=r.width/card.width,{inset,columnGap:gap}=tileRoom;
    return {x:r.x+s*(1+(inset+row.column*(grid.tileWidth+gap))/grid.divisor),
      y:r.y+s*(1+tileHeader(grid.divisor)+(inset+row.y)/grid.divisor),width:s*grid.tileWidth/grid.divisor,height:s*row.height/grid.divisor};
  }
  // A closed part's card holds its declarations' outline, as a program's
  // card holds its parts' (owner via the coordinator, 2026-10-03): seen
  // while its words cannot fill it, drawn larger than they may grow or too
  // small to read (scene.css .scene-part-ghosts). Its top tiles only, a
  // type's members inside its own.
  function tilesOf(id){
    const drawn=geometry.grids.get(id),r=rectOf(id),symbols=model.nodes.get(id)?.item?.symbols||[];
    if(!drawn||!r)return [];
    return symbols.flatMap((symbol,index)=>{const row=drawn.grid.rows[index];return row&&!symbol.owner?[tileRect(r,drawn,row)]:[];});
  }
  function deepFor(id){
    const drawn=geometry.grids.get(id),r=rectOf(id);
    if(!drawn||!r)return markersFor(id);
    const {grid,box:card}=drawn,s=r.width/card.width;
    const rectOfRow=row=>tileRect(r,drawn,row);
    const tileText=s/grid.divisor*13/17;
    (model.nodes.get(id).item?.symbols||[]).forEach((symbol,index)=>{
      const row=grid.rows[index];
      if(row)members.push({part:id,index,rect:rectOfRow(row),text:tileText,name:symbol.name});
    });
    const own=model.memberMarkersOf(id);
    for(const [index,sides] of own.members){
      const member=members.find(m=>m.index===index);if(!member)continue;
      for(const side of ['in','out'])sides[side].forEach((marker,i)=>markers.push({
        id:`${id}#${index}:${side}:${marker.kind}`,box:`${id}#${index}`,part:id,member:index,side,index:i,count:sides[side].length,rect:member.rect,row:true,text:tileText,...marker}));
    }
    for(const side of ['in','out'])own.rest[side].forEach((marker,i)=>markers.push({
      id:`${id}:${side}:${marker.kind}`,box:id,side,index:i,count:own.rest[side].length,rect:r,...marker}));
  }
  function markersFor(id,boxText){
    const rect=rectOf(id),own=model.markersOf(id);
    for(const side of ['in','out'])own[side].forEach((marker,index)=>markers.push({
      id:`${id}:${side}:${marker.kind}`,box:id,side,index,count:own[side].length,rect,...(boxText?{text:boxText}:{}),...marker}));
  }
  function finish(){
    // Quiet arrows stand only where an end is looked at.
    const drawn=edges.filter(edge=>!edge.quiet||edge.near);
    // `frame` holds everything the level draws; `focus` is what entering
    // it frames: the box entered, or the whole map.
    return {level,key:levelKey(level),inner,program,text,nodes,edges:drawn,markers,ports,members,
      frame:program?frameOf(program):geometry.bounds,focus:inner?frameOf(inner):geometry.bounds};
  }
}

// A drawn arrow from a laid-out route: one polyline from its own source's
// border to its own target's, a head at each end an edge goes into.
function edgeOf(route,model,looked){
  const ids=[...route.forward,...route.backward],all=ids.map(id=>edgeByID(model).get(id)).filter(Boolean);
  const end=id=>[id,...model.ancestors(id)];
  return {id:route.id,container:route.container,from:route.from,to:route.to,points:route.points,band:bands.arrow,
    heads:{end:route.forward.length>0,start:route.backward.length>0},edgeIDs:ids,forward:route.forward,backward:route.backward,
    possible:all.length>0&&all.every(edge=>edge.possible),quiet:all.length>0&&all.every(edge=>edge.init),
    near:all.some(edge=>end(edge.from).some(id=>looked.has(id))||end(edge.to).some(id=>looked.has(id))),port:route.port||'',rest:true};
}
const edgeIndex=new WeakMap();
export function edgeByID(model){
  if(!edgeIndex.has(model))edgeIndex.set(model,new Map(model.edges.map(edge=>[edge.id,edge])));
  return edgeIndex.get(model);
}

const inside=(r,p,pad=0)=>p.x>=r.x-pad&&p.x<=r.x+r.width+pad&&p.y>=r.y-pad&&p.y<=r.y+r.height+pad;
function distanceToPolyline(points,p){
  let best=Infinity;
  for(let i=1;i<points.length;i++){
    const a=points[i-1],b=points[i],dx=b.x-a.x,dy=b.y-a.y,length=dx*dx+dy*dy;
    const t=length?Math.max(0,Math.min(1,((p.x-a.x)*dx+(p.y-a.y)*dy)/length)):0;
    best=Math.min(best,Math.hypot(a.x+t*dx-p.x,a.y+t*dy-p.y));
  }
  return best;
}
// What stands under world point `p` at `zoom`, the topmost first: a marker
// or a port, a box (a card, a chip, a closed frame), an arrow within six
// pixels, then an open frame, its title band first. Null on empty canvas.
// Two pixels about a box's border are an arrow's meeting it there: a head
// touching the box is the arrow's.
export function hitTest(scene,p,zoom,shown=edge=>edge.rest!==false){
  for(const item of [...overlayAt(scene,zoom)].reverse())
    if(item.shown&&Math.abs(p.x-item.x)<=item.size/2&&Math.abs(p.y-item.y)<=item.size/2)return {type:item.type,id:item.type==='zoom'?item.box:item.id,box:item.box||'',item};
  let near=null;
  for(const edge of scene.edges){
    if(!shown(edge))continue;
    const d=distanceToPolyline(edge.points,p);
    if(d<=6/zoom&&(!near||d<near.d))near={d,edge};
  }
  const arrow=near&&{type:'edge',id:near.edge.id,edge:near.edge,at:p};
  const rim=2/zoom,onRim=r=>inside(r,p,rim)&&Math.min(Math.abs(p.x-r.x),Math.abs(r.x+r.width-p.x),Math.abs(p.y-r.y),Math.abs(r.y+r.height-p.y))<=rim;
  if(arrow&&near.d<=rim&&scene.nodes.some(node=>node.band===bands.box&&onRim(node.rect)))return arrow;
  const member=(scene.members||[]).find(m=>inside(m.rect,p));
  if(member)return {type:'member',id:`${member.part}#${member.index}`,part:member.part,index:member.index,member};
  const boxes=scene.nodes.filter(node=>node.band===bands.box&&inside(node.rect,p));
  const top=boxes.length?boxes.reduce((a,b)=>b.rect.width*b.rect.height<a.rect.width*a.rect.height?b:a):null;
  if(top){
    const kind=(top.kinds||[]).find(entry=>inside(entry.rect,p,2/zoom));
    return {type:'box',id:top.id,node:top,...(kind?{kind:kind.kind,group:kind.group}:{})};
  }
  if(arrow)return arrow;
  const frames=scene.nodes.filter(node=>node.band===bands.frame&&inside(node.rect,p));
  if(frames.length){
    const top=frames.reduce((a,b)=>b.rect.width*b.rect.height<a.rect.width*a.rect.height?b:a);
    return {type:'frame',id:top.id,node:top};
  }
  return null;
}

// What the pointer and the reader's choice change: each box's and arrow's
// classes and the arrows' order, dark over grey. `pointer` is hitTest's
// answer; `view` the host's choice {scope, operation, entry, selected,
// matched, searching}; `member` a declaration pointed at or chosen.
export function emphasisOf(scene,model,pointer,view={},member=null){
  const edges=model.edges,leaves=model.leaves;
  const blank={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false};
  const choice={...blank,...view};
  const subject=pointer?.type==='box'||pointer?.type==='frame'?pointer.id:pointer?.type==='marker'?pointer.box:'';
  // An arrow pointed at is dark with the boxes at its ends; its card says
  // what it carries.
  const state=pointer?.type==='edge'?{mode:'hover',subject:'',focus:new Set([pointer.edge.from,pointer.edge.to]),
    participants:new Set([pointer.edge.from,pointer.edge.to]),activeEdges:new Set(pointer.edge.edgeIDs)}:emphasis(choice,subject,leaves,edges,member);
  const rest=emphasis(choice,'',leaves,edges,null);
  const recede=rest.mode==='all'?null:rest;
  const subjects=state.mode==='search'?state.focus:new Set([state.subject,...(pointer?.type==='edge'?[pointer.edge.from,pointer.edge.to]:[])].filter(Boolean));
  const standsFor=id=>[id,...leaves(id)];
  const muted=recedes(rest,state,subjects,standsFor);
  const context=focusAncestors(state.focus,{get:id=>({parentId:model.parent(id)})});
  const nodeClass=new Map();
  for(const node of scene.nodes){
    const contains=leaves(node.id).some(id=>state.participants.has(id));
    const classes=[];
    if(muted(node.id))classes.push('flow-node-muted');
    if(subjects.has(node.id))classes.push('flow-node-focus');
    else if(state.participants.has(node.id)||contains&&node.display!=='frame')classes.push('flow-node-connected');
    if(context.has(node.id))classes.push('flow-node-context');
    if(view.scope===node.id||view.operation===node.id)classes.push('flow-node-reading');
    nodeClass.set(node.id,classes.join(' '));
  }
  // An arrow is dark when it carries an emphasised edge; while one is dark
  // the grey ones fade, but for those between the boxes looked at.
  const active=state.activeEdges;
  const edgeState=new Map(),order=[];
  const anyDark=scene.edges.some(edge=>edge.rest!==false&&edge.edgeIDs.some(id=>active.has(id)));
  // An arrow drawn only for a box pointed at stands while one of its ends
  // is the subject.
  const subjectBoxes=new Set([state.subject,...(pointer?.type==='edge'?[pointer.edge.from,pointer.edge.to]:[])].filter(Boolean));
  for(const edge of scene.edges){
    const hidden=edge.rest===false&&!subjectBoxes.has(edge.from)&&!subjectBoxes.has(edge.to);
    const on=!hidden&&edge.edgeIDs.some(id=>active.has(id));
    const kept=!recede||edge.edgeIDs.some(id=>recede.activeEdges.has(id));
    edgeState.set(edge.id,{on,hidden,dim:!!recede&&!on&&!kept,faint:anyDark&&!on,band:on?bands.dark:bands.arrow});
    order.push(edge.id);
  }
  order.sort((a,b)=>edgeState.get(a).band-edgeState.get(b).band);
  // A marker pointed at lights every marker reaching one of its systems
  // or taking one of its inputs.
  const lit=new Set();
  if(pointer?.type==='marker'){
    const at=pointer.item,keys=new Set(at.side==='out'?at.systems:at.members);
    for(const marker of scene.markers)if(marker.side===at.side&&(marker.side==='out'?marker.systems:marker.members).some(id=>keys.has(id)))lit.add(marker.id);
  }
  return {state,nodeClass,edgeState,order,lit};
}

// The level a zoom gesture leaves the camera at. Zooming out leaves a
// level once its text reads under its exit size; zooming in enters the
// enterable box drawn under `aim` (a world point) once its text reads at
// its entry size, a sibling of the level entered included. A pan is no
// zoom: nothing calls this for one.
export function levelAfterZoom(model,geometry,scene,level,zoom,aim,zoomingIn){
  let chain=[...level];
  if(!zoomingIn){
    while(chain.length&&zoom<(geometry.exitZoom.get(chain.at(-1))??0))chain.pop();
    return chain;
  }
  const under=scene.nodes.filter(node=>node.enter&&inside(node.rect,aim));
  if(!under.length)return chain;
  const deepest=under.reduce((a,b)=>chainOf(model,b.id).length>chainOf(model,a.id).length?b:a);
  const target=chainOf(model,deepest.id);
  const qualifies=id=>zoom>=(geometry.enterZoom.get(id)??Infinity);
  let next=[];
  for(const id of target){
    if(chain.includes(id)||qualifies(id))next.push(id);
    else break;
  }
  // Pointing back into what holds the level entered keeps that level.
  if(next.length<chain.length&&chain.slice(0,next.length).every((id,i)=>id===next[i]))return chain;
  return next;
}

// Where a declaration the reading names out of sight is shown (owner,
// 2026-09-29): with its part at the zoom where the part's tiles are drawn
// and read and the part stands whole across the canvas, framed whole when
// it fits, else across with the tile centred down it, never deeper than
// the part's own title fits; a part too dense for its tiles to read there
// is shown as its card, closed at the scale its level reads a part at,
// centred. {level, camera, dense}; null for a part with no tiles.
export function memberView(model,geometry,part,index,canvas,pad=28){
  const rect=geometry.boxes.get(part),drawn=geometry.grids.get(part);
  if(!rect||!drawn)return null;
  const chain=chainOf(model,part),across=(canvas.width-2*pad)/rect.width,whole=Math.min(across,(canvas.height-2*pad)/rect.height);
  const need=Math.max(11/13*drawn.grid.divisor*drawn.box.width/rect.width,(geometry.enterZoom.get(part)||0)*1.02);
  const at=(zoom,x,y)=>({zoom,x:canvas.width/2-x*zoom,y:canvas.height/2-y*zoom});
  if(across<need){
    const level=chain.slice(0,-1),zoom=level.length?(geometry.enterZoom.get(level.at(-1))||0)*1.02:geometry.home.zoom;
    return {level,dense:true,camera:at(zoom,rect.x+rect.width/2,rect.y+rect.height/2)};
  }
  if(whole>=need)return {level:chain,dense:false,camera:at(whole,rect.x+rect.width/2,rect.y+rect.height/2)};
  const tile=sceneAt(model,geometry,chain,{}).members.find(member=>member.index===index)?.rect||rect;
  return {level:chain,dense:false,camera:at(across,rect.x+rect.width/2,tile.y+tile.height/2)};
}

// The connection an arrow stands for, in the direction its head nearest
// the pointer gives (`backward` the head at its start): the outgoing
// connection of the box it leaves to what the box it enters stands in.
// A port stands for its program. Null when no group carries its edges.
export function connectionOf(model,edge,backward=false){
  const ends=backward?[edge.to,edge.from]:[edge.from,edge.to];
  const ids=new Set(backward?edge.backward:edge.forward);
  const carries=group=>group.edges.some(id=>ids.has(id));
  const box=id=>String(id).startsWith('port:')?'':id;
  // The box it leaves, else (a port, an Inputs frame, whose inputs are
  // no part) the box it enters.
  // A call through an outside system folded into its caller's arrow is the
  // connection of the Outside frame holding that system.
  const first=edgeByID(model).get([...ids][0]);
  return (box(ends[0])&&model.frameGroups(ends[0]).find(group=>!group.incoming&&carries(group)))||
    (box(ends[1])&&model.frameGroups(ends[1]).find(group=>group.incoming&&carries(group)))||
    (first&&model.frameGroups(model.rootOf(first.from)).find(group=>!group.incoming&&carries(group)))||null;
}

// The zoom a pinch tick may take from `from` toward `to`: one pinch crosses
// one level boundary, going on or back, and stops short of the next
// (owner: a pinch of eight ticks had carried Redis's readers from the whole
// map past the areas into a part's tiles). `depthAt(zoom)` is the level's
// depth the pinch would leave at that zoom; `gesture.depth` the depth it
// began at; `gesture.across`, set here, the depth beyond the boundary.
export function pinchLimit(from,to,depthAt,gesture,steps=30){
  if(gesture.across===undefined){
    if(depthAt(to)===gesture.depth)return to;
    let same=from,other=to;
    for(let i=0;i<steps;i++){const middle=Math.sqrt(same*other);if(depthAt(middle)===gesture.depth)same=middle;else other=middle;}
    gesture.across=depthAt(other);
  }
  const low=Math.min(gesture.depth,gesture.across),high=Math.max(gesture.depth,gesture.across);
  const ok=zoom=>{const depth=depthAt(zoom);return depth>=low&&depth<=high;};
  if(ok(to)||!ok(from))return ok(to)?to:from;
  let good=from,bad=to;
  for(let i=0;i<steps;i++){const middle=Math.sqrt(good*bad);if(ok(middle))good=middle;else bad=middle;}
  return good;
}

// A frame's connections as the reading column groups them (model.mjs).
export const frameGroups=(model,id)=>model.frameGroups(id);
