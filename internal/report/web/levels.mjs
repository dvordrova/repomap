// The scene canvas's world (PLAN S2): every level laid out once per canvas
// size by ELK, bottom-up, each level its own graph with final routes. An
// area lays out its parts; a program its areas and loose parts, its links
// to other programs ending on ports of its border; the whole map its
// programs, Inputs and Outside frames. A box at one level is exactly as
// large as the level inside it, so zooming in finds the inner level where
// the outer one drew its box. No wrapping, no grid, no size floors and no
// edit after layout: an area too large for the canvas is the camera's job.
// Arrowless collections (chips, inputs' names) are packed in rows.
import ELK from 'elkjs/lib/elk.bundled.js';
import {wrapText} from './cards.mjs';
import {tileGrid} from './symbols.mjs';

let engine;
// ELK can throw on a graph it places with one option and not another
// (old canvas: a NullPointerException with native ports, RIGHT and layer
// unzipping on Redis). A graph that fails is laid out again with ELK's own
// node placement and layering, then without separating its components;
// the map fails only when none of these places it.
const fallbacks=[
  {},
  {'elk.layered.nodePlacement.bk.fixedAlignment':undefined,'elk.layered.layering.strategy':undefined},
  {'elk.layered.nodePlacement.bk.fixedAlignment':undefined,'elk.separateConnectedComponents':'false'},
];
const without=(options,change)=>{
  const out={...options};
  for(const [name,value] of Object.entries(change))if(value===undefined)delete out[name];else out[name]=value;
  return out;
};
const relaid=(node,change)=>({...node,layoutOptions:node.layoutOptions&&without(node.layoutOptions,change),children:node.children?.map(child=>relaid(child,change))});
async function native(graph){
  let failure;
  for(const [i,change] of fallbacks.entries()){
    try{return await (engine||=new ELK()).layout(i?structuredClone(relaid(graph,change)):graph);}
    catch(error){failure||=error;}
  }
  throw failure;
}

// Inside a program arrows run down: their ends stand on the boxes' tops
// and bottoms, leaving the left and right edges to the markers (PLAN B).
// Boxes beside each other keep room for a marker on each.
const interior={
  'elk.algorithm':'layered','elk.direction':'DOWN','elk.edgeRouting':'ORTHOGONAL','elk.randomSeed':'1',
  'elk.layered.mergeEdges':'false','elk.separateConnectedComponents':'true',
  // Boxes aligned so that more arrows run straight between layers: fewer
  // lanes beside each other (redis's Core 11 to 6).
  'elk.layered.nodePlacement.bk.fixedAlignment':'BALANCED',
  'elk.spacing.nodeNode':'96','elk.spacing.componentComponent':'96',
  'elk.layered.spacing.nodeNodeBetweenLayers':'72',
  'elk.spacing.edgeNode':'32','elk.spacing.edgeEdge':'20',
  'elk.layered.spacing.edgeNodeBetweenLayers':'28','elk.layered.spacing.edgeEdgeBetweenLayers':'20',
  'elk.spacing.portPort':'20',
};
// The whole map keeps ELK's own spacing (owner, 2026-09-29), its flow from
// the Inputs through the programs to the Outside frames left to right.
const outer={
  'elk.algorithm':'layered','elk.direction':'RIGHT','elk.edgeRouting':'ORTHOGONAL','elk.randomSeed':'1',
  'elk.layered.mergeEdges':'false','elk.separateConnectedComponents':'true',
  'elk.spacing.nodeNode':'40','elk.layered.spacing.nodeNodeBetweenLayers':'80',
  'elk.spacing.edgeNode':'24','elk.spacing.edgeEdge':'14',
  'elk.layered.spacing.edgeNodeBetweenLayers':'24','elk.layered.spacing.edgeEdgeBetweenLayers':'14',
  // Arrows leaving one side of a box as far apart as lanes.
  'elk.spacing.portPort':'14',
};


// Sizes, in each level's own units: a part's card is cards.mjs's (260 wide,
// its title at 17px); the whole map's text is 1 unit to a pixel.
export const units={
  part:{width:260},
  title:17,
  pad:40,
  // An open frame's title band, in its level's text units.
  band:text=>Math.round(48*text+12),
  // A program's card on the whole map.
  programCard:{width:240,minHeight:72,font:'700 17px system-ui',line:21.25,role:'600 13px system-ui',text:'13px system-ui',textLine:18,pad:14},
  // A chip names an outside system in at most two lines.
  // A chip names an outside system in one cell of its frame's grid, its
  // name in at most two lines after its kind's mark; a bucket is a cell
  // taller by its systems' marks.
  chip:{font:'600 12px system-ui',line:15,pad:8,mark:20,width:140,height:46,bucket:62},
  // An input's name in its kind's group.
  input:{font:'600 13px system-ui',pad:10,mark:22,min:72,max:320,height:30},
  gap:12,
};
// A program is entered where its boxes' titles read at 12.75 pixels, which
// is four fifths of the zoom that fits it in the canvas; an area where its
// parts' do (PLAN S3: by zoom, with hysteresis, never by a pan).
export const reading={enter:.75,exit:.62,deep:860,deepExit:760};

const pairKey=(a,b)=>a<b?`${a}|${b}`:`${b}|${a}`;

// Rows of boxes of `sizes` [{id,width,height}] in their order, the row
// length chosen so the whole is nearest `aspect` (width over height). `top`
// is the title band above them. Returns {width, height, at: Map id → {x,y}}.
export function pack(sizes,{gap=units.gap,pad=16,top=0,aspect=1.6}={}){
  if(!sizes.length)return {width:2*pad,height:top+2*pad,at:new Map()};
  const arrange=limit=>{
    const at=new Map();let x=0,y=0,row=0,width=0;
    for(const size of sizes){
      if(x>0&&x+size.width>limit){x=0;y+=row+gap;row=0;}
      at.set(size.id,{x:pad+x,y:top+pad+y});
      x+=size.width+gap;row=Math.max(row,size.height);width=Math.max(width,x-gap);
    }
    return {width:width+2*pad,height:top+y+row+2*pad,at};
  };
  const widest=Math.max(...sizes.map(size=>size.width));
  const total=sizes.reduce((sum,size)=>sum+size.width+gap,0);
  let best=null;
  for(let i=0;i<=24;i++){
    const limit=widest+(total-widest)*i/24,result=arrange(limit);
    const score=Math.abs(Math.log(result.width/result.height/aspect));
    if(!best||score<best.score-1e-9)best={...result,score};
  }
  return best;
}

// The smallest box of proportion `aspect` no narrower than `width` and
// tall enough for `height(width)` of content.
function boxOf(aspect,width,height){
  let w=width;
  for(let i=0;i<8;i++){const need=height(w);if(w/aspect>=need-.5)break;w=Math.max(w,need*aspect);}
  return {width:w,height:w/aspect};
}

// Read ELK's node and its edges: children's boxes, ports and routes in the
// node's own coordinates.
function readNode(node){
  const children=new Map((node.children||[]).map(child=>[child.id,{x:child.x,y:child.y,width:child.width,height:child.height}]));
  const ports=(node.ports||[]).map(port=>({id:port.id,x:port.x+port.width/2,y:port.y+port.height/2,side:port.layoutOptions?.['elk.port.side']}));
  const routes=new Map();
  for(const edge of node.edges||[]){
    const offset=edge.container&&edge.container!==node.id?children.get(edge.container)||{x:0,y:0}:{x:0,y:0};
    const sections=edge.sections||[];
    const points=[];
    for(const section of sections)for(const point of [section.startPoint,...section.bendPoints||[],section.endPoint]){
      const p={x:point.x+offset.x,y:point.y+offset.y},last=points.at(-1);
      if(!last||Math.abs(last.x-p.x)>1e-6||Math.abs(last.y-p.y)>1e-6)points.push(p);
    }
    routes.set(edge.id,points);
  }
  return {width:node.width,height:node.height,children,ports,routes};
}

// The pairs of a container's children that the model's edges join: one
// route per pair, both directions on it (one arrow, a head at each end it
// goes into), `forward` the edges from `from` to `to`.
function pairsOf(model,edges,childOf){
  const pairs=new Map();
  for(const edge of edges){
    const a=childOf(edge.from),b=childOf(edge.to);
    if(!a||!b||a===b)continue;
    const key=pairKey(a,b);
    if(!pairs.has(key))pairs.set(key,{key,from:a,to:b,forward:[],backward:[]});
    const pair=pairs.get(key);(pair.from===a?pair.forward:pair.backward).push(edge.id);
  }
  return [...pairs.values()];
}

// The whole map's pairs (owner, 2026-10-01, on the skeptic's verdict):
// each program's Inputs frame into it, a program into each Outside frame it
// calls, and one arrow per pair of programs. A program reaching another
// program's input reaches that program; a call through an outside system
// served by another program's input (connects_to) is the caller's arrow to
// the served program. A pair of programs joined only by code use
// (scope=structure) is `uses`: drawn only while one of them is pointed at
// or chosen.
export function homePairs(model){
  const {nodes}=model,pairs=new Map();
  const add=(a,b,edge)=>{
    if(!a||!b||a===b)return;
    const key=pairKey(a,b);
    if(!pairs.has(key))pairs.set(key,{key,from:a,to:b,forward:[],backward:[],operation:false});
    const pair=pairs.get(key);
    (pair.from===a?pair.forward:pair.backward).push(edge.id);
    pair.operation||=edge.relations.some(relation=>relation.scope!=='structure');
  };
  for(const edge of model.edges){
    const from=nodes.get(edge.from),to=nodes.get(edge.to);
    let sources=[model.rootOf(edge.from)],target=model.rootOf(edge.to);
    if(to?.kind==='input'&&to.program&&model.rootOf(edge.from)!==to.program)target=to.program;
    if(from?.kind==='input'&&from.program&&model.rootOf(edge.to)!==from.program)sources=[from.program];
    if(from?.kind==='system'&&to?.kind==='input'){
      sources=[...(model.callingPrograms.get(edge.from)||[])];target=to.program||target;
      for(const source of sources)add(source,target,{...edge,relations:[{scope:'operation'}]});
      continue;
    }
    for(const source of sources)add(source,target,edge);
  }
  for(const pair of pairs.values())pair.uses=nodes.get(pair.from)?.kind==='program'&&nodes.get(pair.to)?.kind==='program'&&!pair.operation;
  return [...pairs.values()];
}

// Lay out a container's children with ELK: `boxes` [{id,width,height}],
// `pairs` from pairsOf, `ports` [{id, side}] on its border with `portPairs`
// [{key, child, port, forward, backward}] joining a child to one.
// `band` is its title's room on top, `spacing` how many times ELK's
// spacing its boxes keep (a program's boxes are `k` times a part's).
async function layered(id,boxes,pairs,{band=units.band(1),spacing=1,ports=[],portPairs=[],options=interior}={}){
  const pad=Math.round(units.pad*spacing);
  const own={...options,'elk.padding':`[top=${band},left=${pad},bottom=${pad},right=${pad}]`};
  for(const name of Object.keys(own))if(name.includes('spacing.'))own[name]=String(Math.round(Number(own[name])*spacing));
  if(ports.length)own['elk.portConstraints']='FIXED_SIDE';
  const graph={id:`level:${id}`,layoutOptions:{...options,'elk.hierarchyHandling':'INCLUDE_CHILDREN'},children:[{
    id,layoutOptions:own,
    ports:ports.map(port=>({id:port.id,width:0,height:0,layoutOptions:{'elk.port.side':port.side}})),
    children:boxes.map(box=>({id:box.id,width:box.width,height:box.height})),
    edges:[...pairs.map(pair=>({id:pair.key,sources:[pair.from],targets:[pair.to]})),
      ...portPairs.map(pair=>({id:pair.key,sources:[pair.out?pair.child:pair.port],targets:[pair.out?pair.port:pair.child]}))],
  }]};
  return readNode((await native(graph)).children[0]);
}

// The box a part's card takes at its level, `k` times its own size.
const partBox=(node,k=1)=>({id:node.id,width:units.part.width*k,height:(node.item?.height||90)*k});

// A program's card on the whole map: its title, its role and up to two
// lines of its purpose; what it holds reads where it is entered.
export function programCardHeight(node,width,measure){
  const card=units.programCard,inner=width-2*card.pad;
  const title=wrapText(node.name,inner,card.font,measure).length*card.line;
  const role=node.item?.role?Math.min(2,wrapText(node.item.role,inner,card.role,measure).length)*card.textLine+6:0;
  const purpose=node.item?.summary?Math.min(2,wrapText(node.item.summary,inner,card.text,measure).length)*card.textLine+6:0;
  return Math.max(card.minHeight,2*card.pad+title+role+purpose);
}
// A closed area's card at its program's level, `k` times a part's: its
// title and up to three lines of its purpose, in the proportion of its
// open drawing, so that entered it holds that drawing exactly.
export function areaCard(node,inside,measure,k=1){
  const content=width=>{
    const inner=width-2*14-24;
    const title=wrapText(node.name,inner,'700 17px system-ui',measure).length*21.25;
    const purpose=node.item?.summary?Math.min(3,wrapText(node.item.summary,inner,'13px system-ui',measure).length)*18+6:0;
    return 28+title+purpose;
  };
  const box=boxOf(inside.width/inside.height,units.part.width,content);
  return {id:node.id,width:box.width*k,height:box.height*k};
}
// A chip's box: one cell of its frame's grid.
export function chipBox(node){
  return {id:node.id,width:units.chip.width,height:units.chip.height};
}
// An input's tile: its kind's mark and its name on one line, or two when long.
export function inputBox(node,measure){
  const c=units.input,width=Math.min(c.max,Math.max(c.min,Math.ceil(measure(node.name,c.font))+c.mark+2*c.pad));
  const lines=Math.min(2,wrapText(node.name,width-c.mark-2*c.pad,c.font,measure).length);
  return {id:node.id,width,height:c.height+(lines-1)*17};
}

// An Inputs frame closed: its title over its kinds' marks; the kinds'
// names and their inputs read where it is entered.
export function inputsCard(node){
  return {width:Math.max(112,28+node.children.length*22),height:units.band(1)+26};
}

// `model` from buildModel; `canvas` {width,height} in pixels. Returns the
// geometry every level is drawn from (see the fields at the end).
export async function layoutLevels(model,{width=1200,height=700,measure}={}){
  const canvas={width:Math.max(320,width),height:Math.max(240,height)};
  const {nodes}=model;
  const local=new Map();
  const childOf=container=>id=>{
    for(let at=id;at;at=model.parent(at))if(model.parent(at)===container)return at;
    return '';
  };
  const fitZoom=(w,h,pad=24)=>Math.min((canvas.width-2*pad)/w,(canvas.height-2*pad)/h);

  // 1. Areas: their parts, with the arrows between them.
  for(const area of [...nodes.values()].filter(node=>node.kind==='area')){
    const parts=area.children.map(id=>nodes.get(id)).filter(Boolean);
    const pairs=pairsOf(model,model.edges,childOf(area.id));
    const laid=await layered(area.id,parts.map(part=>partBox(part)),pairs);
    local.set(area.id,{...laid,text:1,pairs,kind:'area'});
  }

  // 2. Programs: their areas closed, each a card of its open drawing's
  // proportion holding its title and purpose, and their loose parts'
  // cards; their links to other programs end on ports of their border,
  // those coming in on the left and those going out on the right.
  // Entered, an area draws its parts inside its card.
  for(const program of [...nodes.values()].filter(node=>node.kind==='program')){
    const children=program.children.map(id=>nodes.get(id)).filter(Boolean);
    const inner=childOf(program.id);
    const pairs=pairsOf(model,model.edges,inner);
    const portMap=new Map(),portPairs=new Map();
    for(const edge of model.edges){
      const fromInside=!!inner(edge.from)||edge.from===program.id,toInside=!!inner(edge.to)||edge.to===program.id;
      if(fromInside===toInside)continue;
      const other=model.programOf(fromInside?edge.to:edge.from);
      if(!other||other===program.id||nodes.get(other)?.kind!=='program')continue;
      // Each box one port a way for the programs it links with: arrows
      // converging on one port had run on one line into it, and a port per
      // program and box had stood forty to a border (etcd's server).
      const way=fromInside?'out':'in',child=inner(fromInside?edge.from:edge.to),port=`port:${program.id}:${way}:${child||'-'}`;
      if(!portMap.has(port))portMap.set(port,{id:port,side:way==='out'?'EAST':'WEST',way,programs:[],edges:[]});
      const entry=portMap.get(port);entry.edges.push(edge.id);if(!entry.programs.includes(other))entry.programs.push(other);
      if(!child)continue;
      const key=`${child}|${port}`;
      if(!portPairs.has(key))portPairs.set(key,{key,child,port,out:way==='out',forward:[],backward:[]});
      portPairs.get(key).forward.push(edge.id);
    }
    const ports=[...portMap.values()];
    // Of both directions, the one showing the program larger in the
    // canvas. Its cards are a part's size: drawn larger, the program grows
    // with them and reads no better where it fits the canvas; one too
    // large to read whole is the camera's job.
    const boxes=children.map(child=>child.kind==='area'?areaCard(child,local.get(child.id),measure):partBox(child));
    const at=direction=>layered(program.id,boxes,pairs,{ports,portPairs:[...portPairs.values()],options:{...interior,'elk.direction':direction}});
    const down=await at('DOWN'),across=await at('RIGHT');
    const direction=fitZoom(across.width,across.height)>fitZoom(down.width,down.height)*1.05?'RIGHT':'DOWN';
    const laid=direction==='RIGHT'?across:down,k=1;
    local.set(program.id,{...laid,text:k,direction,pairs,ports:ports.map(port=>({...port,...laid.ports.find(p=>p.id===port.id)})),
      portPairs:[...portPairs.values()],kind:'program'});
  }

  // 3. Collections, packed: an Inputs frame's kinds, each its inputs'
  // names; an Outside frame's systems and buckets, a bucket's systems.
  for(const node of nodes.values()){
    if(node.kind==='kind'){
      const tiles=node.children.map(id=>inputBox(nodes.get(id),measure));
      const laid=pack(tiles,{top:units.band(.8),aspect:2.2});
      local.set(node.id,{width:laid.width,height:laid.height,children:new Map(tiles.map(tile=>[tile.id,{...laid.at.get(tile.id),width:tile.width,height:tile.height}])),routes:new Map(),ports:[],text:1,kind:'kind'});
    }
    if(node.kind==='bucket'){
      const chips=node.children.map(id=>chipBox(nodes.get(id)));
      const laid=pack(chips,{top:units.band(.8),aspect:1.6});
      local.set(node.id,{width:laid.width,height:laid.height,children:new Map(chips.map(chip=>[chip.id,{...laid.at.get(chip.id),width:chip.width,height:chip.height}])),routes:new Map(),ports:[],text:1,kind:'bucket'});
    }
  }
  // A closed bucket: its part's name over its systems' marks, at the size
  // of a chip of that name, grown to the proportion of its open drawing.
  // A closed bucket: its part's name over its systems' marks, a cell of
  // its frame's grid; entered, its systems stand inside it.
  const closedBucket=node=>({id:node.id,width:units.chip.width,height:units.chip.bucket});
  for(const node of nodes.values()){
    if(node.kind==='inputs'){
      // Closed, it lists its kinds (canvas: the collection's summary); open,
      // its kinds' groups are packed toward that card's proportion.
      const groups=node.children.map(id=>({id,width:local.get(id).width,height:local.get(id).height}));
      const closed=inputsCard(node);
      const laid=pack(groups,{top:units.band(1),aspect:closed.width/closed.height,gap:24});
      local.set(node.id,{width:laid.width,height:laid.height,closed,children:new Map(groups.map(group=>[group.id,{...laid.at.get(group.id),width:group.width,height:group.height}])),routes:new Map(),ports:[],text:1,kind:'inputs'});
    }
  }
  // An Outside frame's cells in rows toward `aspect`; the whole map tries
  // a few and keeps the one it shows largest.
  const packOutside=aspect=>{
    for(const node of nodes.values()){
      if(node.kind!=='outside')continue;
      const items=node.children.map(id=>nodes.get(id)?.kind==='bucket'?closedBucket(nodes.get(id)):chipBox(nodes.get(id)));
      const laid=pack(items,{top:units.band(1),aspect});
      local.set(node.id,{width:laid.width,height:laid.height,children:new Map(items.map(item=>[item.id,{...laid.at.get(item.id),width:item.width,height:item.height}])),routes:new Map(),ports:[],text:1,kind:'outside'});
    }
  };

  // 4. The whole map: each program as a card of its own level's
  // proportion, each Inputs frame as its kinds' list, each Outside frame
  // open, with one arrow per pair of them.
  const rootBox=id=>{
    const node=nodes.get(id),inside=local.get(id);
    // A program's card holds its summary; entered, its drawing stands
    // inside the card at the scale that fits it.
    if(node.kind==='program')return {id,width:units.programCard.width,height:programCardHeight(node,units.programCard.width,measure)};
    if(node.kind==='inputs')return {id,...inside.closed};
    if(node.kind==='outside')return {id,width:inside.width,height:inside.height};
    if(node.kind==='note')return {id,width:260,height:Math.max(80,(node.item?.height||80))};
    if(node.kind==='system')return chipBox(node);
    if(node.kind==='input')return inputBox(node,measure);
    return partBox(node);
  };
  const rootPairs=homePairs(model);
  let map=null,chosen={aspect:1,direction:'RIGHT'};
  const hasOutside=[...nodes.values()].some(node=>node.kind==='outside');
  const spaced=factor=>{
    const options={...outer};
    for(const name of ['elk.spacing.edgeEdge','elk.spacing.edgeNode','elk.layered.spacing.edgeEdgeBetweenLayers','elk.layered.spacing.edgeNodeBetweenLayers'])options[name]=String(Math.round(Number(outer[name])*factor));
    return options;
  };
  const layMap=async(aspect,direction,factor)=>{
    packOutside(aspect);
    const roots=model.roots.map(rootBox);
    return readNode(await native({id:'map',layoutOptions:{...spaced(factor),'elk.direction':direction},children:roots.map(box=>({id:box.id,width:box.width,height:box.height})),
      edges:rootPairs.map(pair=>({id:pair.key,sources:[pair.from],targets:[pair.to]}))}));
  };
  for(const aspect of hasOutside?[1,.6,1.6]:[1])for(const direction of ['RIGHT','DOWN']){
    const laid=await layMap(aspect,direction,1);
    if(!map||fitZoom(laid.width,laid.height)>fitZoom(map.width,map.height)*1.05){map=laid;chosen={aspect,direction};}
  }
  packOutside(chosen.aspect);
  local.set('',{...map,text:1,pairs:rootPairs,kind:'map'});

  // 5. World places: the map's units are the world's; a box drawn at one
  // level holds the level inside it at one scale (`scale`), centred.
  const boxes=new Map(),scales=new Map(),routes=new Map(),ports=new Map();
  const toWorld=(origin,scale)=>point=>({x:origin.x+point.x*scale,y:origin.y+point.y*scale});
  function place(container,rect){
    const inside=local.get(container);if(!inside)return;
    const scale=Math.min(rect.width/inside.width,rect.height/inside.height);
    const origin={x:rect.x+(rect.width-inside.width*scale)/2,y:rect.y+(rect.height-inside.height*scale)/2};
    scales.set(container,{scale,origin,frame:{x:origin.x,y:origin.y,width:inside.width*scale,height:inside.height*scale}});
    const world=toWorld(origin,scale);
    for(const [id,box] of inside.children){
      const at=world(box);
      boxes.set(id,{x:at.x,y:at.y,width:box.width*scale,height:box.height*scale});
    }
    const drawn=[];
    for(const pair of inside.pairs||[]){
      const points=(inside.routes.get(pair.key)||[]).map(world);
      if(points.length>1)drawn.push({id:`${container||'map'}:${pair.key}`,container,from:pair.from,to:pair.to,points,forward:pair.forward,backward:pair.backward,uses:!!pair.uses});
    }
    for(const pair of inside.portPairs||[]){
      const points=(inside.routes.get(pair.key)||[]).map(world);
      if(points.length>1)drawn.push({id:`${container}:${pair.key}`,container,from:pair.out?pair.child:pair.port,to:pair.out?pair.port:pair.child,points,forward:pair.forward,backward:[],port:pair.port});
    }
    routes.set(container,drawn);
    if(inside.ports?.length)ports.set(container,inside.ports.map(port=>({...port,point:world(port)})));
    for(const id of inside.children.keys())if(local.has(id))place(id,boxes.get(id));
  }
  place('',{x:0,y:0,width:map.width,height:map.height});

  // Each container's text drawn open, in world units per pixel; a part's
  // card is drawn at its level's.
  const text=new Map();
  for(const [id,inside] of local)text.set(id,(inside.text||1)*(scales.get(id)?.scale||1));
  const enterZoom=new Map(),exitZoom=new Map();
  for(const [id] of local){
    if(!id)continue;
    enterZoom.set(id,reading.enter/text.get(id));exitZoom.set(id,reading.exit/text.get(id));
  }
  for(const node of nodes.values())if(node.kind==='part'&&boxes.has(node.id)){
    const box=boxes.get(node.id);
    enterZoom.set(node.id,reading.deep/box.width);exitZoom.set(node.id,reading.deepExit/box.width);
  }
  // A part entered draws its declarations as tiles in its card
  // (symbols.mjs tileGrid), laid out once here in the card's own units.
  const grids=new Map();
  for(const node of nodes.values())if(node.kind==='part'&&node.item?.symbols?.length){
    const box={width:units.part.width,height:node.item?.height||90};
    grids.set(node.id,{box,grid:tileGrid(node.item.symbols,node.item.symbolCalls||[],box,measure)});
  }
  const bounds={x:0,y:0,width:map.width,height:map.height};
  return {canvas,local,boxes,scales,routes,ports,text,enterZoom,exitZoom,bounds,grids,
    home:homeCamera(bounds,canvas)};
}

// The camera that shows the whole map, centred, no closer than one unit to
// a pixel.
export function homeCamera(bounds,canvas,pad=16){
  const zoom=Math.min(1,(canvas.width-2*pad)/bounds.width,(canvas.height-2*pad)/bounds.height);
  return {zoom,x:(canvas.width-bounds.width*zoom)/2-bounds.x*zoom,y:(canvas.height-bounds.height*zoom)/2-bounds.y*zoom};
}
