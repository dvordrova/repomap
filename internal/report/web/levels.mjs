// The scene canvas's world (PLAN S2): every level laid out once per canvas
// size by ELK, bottom-up, each level its own graph with final routes. An
// area lays out its parts; a program its areas and loose parts, its links
// to other programs ending on ports of its border; the whole map its
// programs, Inputs and Outside frames. A box at one level is a card holding
// the level inside it at one scale, so zooming in finds the inner level
// where the outer one drew its card. No wrapping, no grid, no size floors
// and no edit after layout: an area too large for the canvas is the
// camera's job. Arrowless collections (chips, inputs' names) are packed in
// rows.
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
// The whole map, its flow from the Inputs through the programs to the
// Outside frames left to right, keeps room for its arrows (owner,
// 2026-09-29): ELK's spacing, its lanes as far apart as its edges.
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
  // A chip names an outside system in one cell of its frame's grid, its
  // name in at most two lines after its kind's mark; a bucket is a cell
  // taller by its systems' marks.
  chip:{font:'600 12px system-ui',line:17,pad:12,mark:20,width:140,height:46,bucket:62},
  // An input's name in its kind's group.
  input:{font:'600 13px system-ui',pad:10,mark:22,min:72,max:320,height:30},
  gap:12,
};
// A program is entered where its boxes' titles read at 12.75 pixels, which
// is four fifths of the zoom that fits it in the canvas; an area where its
// parts' do (PLAN S3: by zoom, with hysteresis, never by a pan).
// On the whole map at rest a program's name reads from eleven pixels, an
// outside system's (a secondary word, as a description and an input's
// name) from nine and a half (owner, 2026-10-02); they are sized for half a
// pixel more, room for the browser's measure.
export const reading={enter:.75,exit:.62,deep:860,deepExit:760,title:11.5,least:11,system:10,systemLeast:9.5};

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
// A part's card holds its words (owner, 2026-10-02: litestream's
// "Replica client backends" had stood its title over four fifths of an
// empty card): its title, and its description whole in up to four lines
// (cardWords) in the card cards.mjs measures; a card whose description is
// left out (none, or longer than four lines) is as tall as its title, with
// a tile's room under it where it holds declarations (the one chosen
// stands there while it is closed, scene-canvas.jsx CardNode).
export function partCardHeight(node,measure){
  const pad=14,tiles=!!node.item?.symbols?.length,room=tiles?24:0,inner=units.part.width-2*pad-room;
  const lines=wholeLines(node.item?.summary,inner,'13px system-ui',measure,4).length;
  if(lines&&node.item?.height)return node.item.height;
  const title=wrapText(node.name??node.title??'',inner,'700 17px system-ui',measure).length;
  return Math.ceil(2*pad+title*21.25+(lines?6+lines*18:tiles?26:0)+2);
}
const partBox=(node,measure)=>({id:node.id,width:units.part.width,height:partCardHeight(node,measure)});

// A card's words stand whole or not at all (owner, 2026-10-01: a card
// never cuts its text mid-way; what does not stand is read on pointing and
// in the column): a block's lines at `width`, none when it takes more than
// `most` lines.
export function wholeLines(text,width,font,measure,most=Infinity){
  if(!text||most<=0)return [];
  const lines=wrapText(text,width,font,measure);
  return lines.length<=most?lines:[];
}
// A program's card on the whole map: its title, its role and its purpose,
// each of them whole in two lines at most or left out; what it holds reads
// where it is entered.
export function programWords(node,width,measure){
  const card=units.programCard,inner=width-2*card.pad;
  return {title:wrapText(node.name??node.title,inner,card.font,measure),role:wholeLines(node.item?.role,inner,card.role,measure,2),
    purpose:wholeLines(node.item?.summary,inner,card.text,measure,2)};
}
export function programCardHeight(node,width,measure){
  const card=units.programCard,words=programWords(node,width,measure);
  return Math.max(card.minHeight,2*card.pad+words.title.length*card.line+
    (words.role.length?words.role.length*card.textLine+6:0)+(words.purpose.length?words.purpose.length*card.textLine+6:0));
}
// A part's or a closed area's card: its title whole in its lines and its
// description whole under it, in at most four lines that stand in the
// card, or left out. A title the card cannot hold whole is not drawn (it is
// named on pointing). `node` {title, rect, text}; `room` the width its
// magnifier takes.
export function cardWords(node,description,measure,{room=0,pad=14,titleFont='700 17px system-ui',titleLine=21.25}={}){
  const width=node.rect.width/node.text-2*pad-room,height=node.rect.height/node.text-2*pad;
  if(width<=8||height<=8)return {title:[],lines:[],hidden:true};
  const title=wrapText(node.title,width,titleFont,measure);
  if(title.length*titleLine>height+1)return {title:[],lines:[],hidden:true};
  const most=Math.max(0,Math.floor((height-title.length*titleLine-6)/18));
  return {title,lines:wholeLines(description,width,'13px system-ui',measure,Math.min(most,4)),hidden:false};
}
// A closed area's card at its program's level, `k` times a part's: its
// title and up to three lines of its purpose, in the proportion of its
// open drawing, so that entered it holds that drawing exactly.
export function areaCard(node,inside,measure,k=1){
  const content=width=>{
    const inner=width-2*14-24;
    const title=wrapText(node.name,inner,'700 17px system-ui',measure).length*21.25;
    const lines=wholeLines(node.item?.summary,inner,'13px system-ui',measure,3).length,purpose=lines?lines*18+6:0;
    return 28+title+purpose;
  };
  const box=boxOf(inside.width/inside.height,units.part.width,content);
  return {id:node.id,width:box.width*k,height:box.height*k};
}
// A chip's box: one cell of its frame's grid, as tall as its name's lines
// (owner, 2026-10-02: a name stands whole, never ellipsized; casdoor's
// "AWS Identity and Access Management" had been cut after two lines).
export function chipBox(node,measure){
  const c=units.chip,lines=chipLines(node.name,c.width-2*c.pad-c.mark,measure);
  return {id:node.id,width:c.width,height:Math.max(c.height,12+lines*c.line)};
}
// A closed bucket's box: its part's card in front of a stack of chips
// (scene.css .scene-bucket-stack, 8 pixels of stack, the card's 10 of
// padding a side), its name's lines over its systems' marks.
export function bucketBox(node,measure){
  const c=units.chip,lines=chipLines(node.name??node.title,c.width-30,measure);
  return {id:node.id,width:c.width,height:Math.max(c.bucket,40+lines*c.line)};
}
export const chipLines=(name,width,measure)=>measure?wrapText(name||'',width,units.chip.font,measure).length:2;
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
    const laid=await layered(area.id,parts.map(part=>partBox(part,measure)),pairs);
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
    const boxes=children.map(child=>child.kind==='area'?areaCard(child,local.get(child.id),measure):partBox(child,measure));
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
      const chips=node.children.map(id=>chipBox(nodes.get(id),measure));
      const laid=pack(chips,{top:units.band(.8),aspect:1.6});
      local.set(node.id,{width:laid.width,height:laid.height,children:new Map(chips.map(chip=>[chip.id,{...laid.at.get(chip.id),width:chip.width,height:chip.height}])),routes:new Map(),ports:[],text:1,kind:'bucket'});
    }
  }
  // A closed bucket: its part's name over its systems' marks, at the size
  // of a chip of that name, grown to the proportion of its open drawing.
  // A closed bucket: its part's name over its systems' marks, a cell of
  // its frame's grid; entered, its systems stand inside it.
  const closedBucket=node=>bucketBox(node,measure);
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
      const items=node.children.map(id=>nodes.get(id)?.kind==='bucket'?closedBucket(nodes.get(id)):chipBox(nodes.get(id),measure));
      const laid=pack(items,{top:units.band(1),aspect});
      local.set(node.id,{width:laid.width,height:laid.height,children:new Map(items.map(item=>[item.id,{...laid.at.get(item.id),width:item.width,height:item.height}])),routes:new Map(),ports:[],text:1,kind:'outside'});
    }
  };

  // 4. The whole map: each program as a card of its own level's
  // proportion, each Inputs frame as its kinds' list, each Outside frame
  // open, with one arrow per pair of them.
  // The whole map names at rest every program and every outside system
  // (owner, 2026-10-02: headscale's, beets' and etcd's whole maps had
  // drawn no word that read, their chips blank): a program's card is drawn
  // at the text size (`programText`) its name reads at (eleven and a half
  // pixels at the camera showing the whole map), an Outside frame's systems
  // at the size (`systemText`) theirs do (ten, a secondary word); an Inputs
  // card, its kinds icons, and a loose box keep their own.
  let programText=1,systemText=1;
  const systems=model.roots.some(id=>nodes.get(id)?.kind==='outside'&&nodes.get(id).children.length);
  const rootBox=id=>{
    const node=nodes.get(id),inside=local.get(id);
    // A program's card holds its summary; entered, its drawing stands
    // inside the card at the scale that fits it.
    if(node.kind==='program')return {id,width:units.programCard.width*programText,height:programCardHeight(node,units.programCard.width,measure)*programText};
    if(node.kind==='inputs')return {id,...inside.closed};
    if(node.kind==='outside')return {id,width:inside.width*systemText,height:inside.height*systemText};
    if(node.kind==='note')return {id,width:260,height:Math.max(80,(node.item?.height||80))};
    if(node.kind==='system')return chipBox(node,measure);
    if(node.kind==='input')return inputBox(node,measure);
    return partBox(node,measure);
  };
  const rootPairs=model.homePairs;
  let map=null,chosen={aspect:1,direction:'RIGHT'};
  const hasOutside=[...nodes.values()].some(node=>node.kind==='outside');
  const layMap=async(aspect,direction)=>{
    packOutside(aspect);
    const roots=model.roots.map(rootBox);
    // Lanes widen with the programs' text, standing as far apart on the
    // screen as the Step 1 drawing's at the camera that reads the names.
    const lanes=Object.fromEntries(['elk.spacing.edgeNode','elk.spacing.edgeEdge','elk.layered.spacing.edgeNodeBetweenLayers','elk.layered.spacing.edgeEdgeBetweenLayers','elk.spacing.portPort']
      .map(key=>[key,String(Number(outer[key])*programText)]));
    return readNode(await native({id:'map',layoutOptions:{...outer,...lanes,'elk.direction':direction},children:roots.map(box=>({id:box.id,width:box.width,height:box.height})),
      edges:rootPairs.map(pair=>({id:pair.key,sources:[pair.from],targets:[pair.to]}))}));
  };
  // Every arrangement tried, or the one chosen again.
  const layAll=async(again=false)=>{
    if(again){map=await layMap(chosen.aspect,chosen.direction);return;}
    map=null;
    for(const aspect of hasOutside?[1,.6,1.6,2.4]:[1])for(const direction of ['RIGHT','DOWN']){
      const laid=await layMap(aspect,direction);
      if(!map||fitZoom(laid.width,laid.height)>fitZoom(map.width,map.height)*1.05){map=laid;chosen={aspect,direction};}
    }
  };
  await layAll();
  // The names' sizes on the screen at the camera showing the whole map.
  const sizes=()=>{
    const zoom=homeCamera({x:0,y:0,width:map.width,height:map.height},canvas).zoom;
    return {program:17*programText*zoom,system:systems?12*systemText*zoom:Infinity};
  };
  // How far the names stand from reading, the smaller of the programs'
  // and the systems' ratios to their own sizes: 1 or more where both read.
  const short=(sized=reading.title,systemSized=reading.system)=>Math.min(sizes().program/sized,sizes().system/systemSized);
  // The map grows as its names do, more slowly where they are a small part
  // of it: the sizes grow while the names come near enough reading at the
  // pace they do; where they cannot reach it (etcd: its programs stand in
  // many layers), the sizes kept are those that brought them nearest, and
  // the camera at rest then shows the part of the map it names (homeView
  // below).
  const steps=20;
  let best={programText,systemText,map,chosen,ratio:short()};
  for(let step=0;step<steps&&short()<1;step++){
    const before=short(),now=sizes();
    if(now.program<reading.title)programText*=reading.title/now.program*1.01;
    if(now.system<reading.system)systemText*=reading.system/now.system*1.01;
    await layAll(step>0);
    if(short()>best.ratio)best={programText,systemText,map,chosen,ratio:short()};
    const gain=short()/before-1;
    if(short()>=1||gain<=0||gain*(steps-step-1)<1/short()-1)break;
  }
  if(short()<best.ratio)({programText,systemText,map,chosen}=best);
  packOutside(chosen.aspect);
  local.set('',{...map,text:1,pairs:rootPairs,kind:'map'});

  // 5. World places: the map's units are the world's; a box drawn at one
  // level holds the level inside it at one scale (`scale`), centred.
  const boxes=new Map(),scales=new Map(),routes=new Map(),ports=new Map();
  const toWorld=(origin,scale)=>point=>({x:origin.x+point.x*scale,y:origin.y+point.y*scale});
  function place(container,rect){
    const inside=local.get(container);if(!inside||!(inside.width>0&&inside.height>0))return;
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
    const box={width:units.part.width,height:partCardHeight(node,measure)};
    grids.set(node.id,{box,grid:tileGrid(node.item.symbols,node.item.symbolCalls||[],box,measure)});
  }
  // 6. The world's unit (owner, 2026-10-02: etcd's storage parts stood 1.2
  // world pixels tall, the browser sizes a box in steps of 1/64 of a
  // pixel, and at the camera entering them their arrows' ends had stood 2px
  // off their borders): the world is drawn `unit` times larger, a power of
  // two keeping every level's entry camera at most four screen pixels to a
  // world pixel.
  const deepest=Math.max(1,...enterZoom.values(),...[...grids].map(([id,{grid,box}])=>11/13*grid.divisor*box.width/(boxes.get(id)?.width||Infinity)));
  const unit=2**Math.max(0,Math.ceil(Math.log2(deepest/4)));
  const big=r=>({...r,x:r.x*unit,y:r.y*unit,width:r.width*unit,height:r.height*unit});
  const far=p=>({...p,x:p.x*unit,y:p.y*unit});
  for(const [id,r] of boxes)boxes.set(id,big(r));
  for(const [id,s] of scales)scales.set(id,{scale:s.scale*unit,origin:far(s.origin),frame:big(s.frame)});
  for(const [id,list] of routes)routes.set(id,list.map(route=>({...route,points:route.points.map(far)})));
  for(const [id,list] of ports)ports.set(id,list.map(port=>({...port,point:far(port.point)})));
  for(const [id,t] of text)text.set(id,t*unit);
  for(const [id,z] of enterZoom)enterZoom.set(id,z/unit);
  for(const [id,z] of exitZoom)exitZoom.set(id,z/unit);
  const bounds={x:0,y:0,width:map.width*unit,height:map.height*unit};
  // The busiest program first: the one most arrows at rest join.
  const joins=id=>model.homePairs.filter(pair=>!pair.uses&&(pair.from===id||pair.to===id)).length;
  const programs=model.roots.filter(id=>nodes.get(id)?.kind==='program'&&boxes.has(id))
    .sort((a,b)=>joins(b)-joins(a)).map(id=>boxes.get(id));
  // Every name the whole map shows at rest: its programs' and its outside
  // systems' (a chip, a closed bucket).
  const names=[...programs,...model.roots.filter(id=>nodes.get(id)?.kind==='outside').flatMap(id=>nodes.get(id).children).map(id=>boxes.get(id)).filter(Boolean)];
  // The whole map's closed boxes, their titles at their top left corners.
  const closed=model.roots.filter(id=>['program','inputs'].includes(nodes.get(id)?.kind)&&boxes.has(id)).map(id=>boxes.get(id));
  return {canvas,local,boxes,scales,routes,ports,text,enterZoom,exitZoom,bounds,grids,unit,programText:programText*unit,
    home:homeView(bounds,canvas,unit,{program:17*programText*unit,system:systems?12*systemText*unit:Infinity},names,programs.length,16,closed),whole:homeCamera(bounds,canvas,16,1/unit)};
}

// The camera at rest on the whole map: all of it, unless its names would
// not read there (a program's at eleven pixels, an outside system's at
// nine and a half; `word` their world sizes); then as close as they read,
// framing the busiest program (`names` come with the `programs` first,
// busiest first) with the most other programs, then the most other names,
// a canvas holds, centred on them. "Show whole map" shows all of it
// (geometry.whole).
export function homeView(bounds,canvas,unit,word,names,programs=names.length,pad=16,closed=[]){
  const whole=homeCamera(bounds,canvas,pad,1/unit);
  if(word.program*whole.zoom>=reading.least&&word.system*whole.zoom>=reading.systemLeast||!names.length)return whole;
  const zoom=Math.max(reading.title/word.program,reading.system/word.system),width=(canvas.width-2*pad)/zoom,height=(canvas.height-2*pad)/zoom;
  const within=(r,x,y)=>r.x>=x-1e-9&&r.y>=y-1e-9&&r.x+r.width<=x+width+1e-9&&r.y+r.height<=y+height+1e-9;
  const xs=[...new Set(names.map(r=>r.x))],ys=[...new Set(names.map(r=>r.y))];
  const score=held=>held.filter(r=>names.indexOf(r)<programs).length*1e6+held.length;
  let best=null;
  for(const x of xs)for(const y of ys){
    if(!within(names[0],x,y))continue;
    const held=names.filter(r=>within(r,x,y));
    if(!best||score(held)>score(best))best=held;
  }
  best||=[names[0]];
  const l=Math.min(...best.map(r=>r.x)),t=Math.min(...best.map(r=>r.y)),r=Math.max(...best.map(r=>r.x+r.width)),b=Math.max(...best.map(r=>r.y+r.height));
  return keepTitles({zoom,x:canvas.width/2-(l+r)/2*zoom,y:canvas.height/2-(t+b)/2*zoom},closed,best,canvas,pad);
}

// A closed box partly in sight shows its title, at its top left corner
// (owner via the coordinator, 2026-10-02: casdoor's home had cut its
// Inputs box to "ts"). Along each axis where one is cut, the camera moves
// to bring a box's corner in or to take the box wholly out of sight,
// keeping `keep` wholly in sight: the move that leaves the fewest cut,
// then shows the most such boxes, then moves least. Where no move helps,
// it stays.
export function keepTitles(view,closed,keep,canvas,pad=16){
  const out={...view};
  for(const [axis,size,other,across] of [['x','width','y','height'],['y','height','x','width']]){
    const at=(r,d)=>r[axis]*out.zoom+out[axis]+d,end=(r,d)=>at(r,d)+r[size]*out.zoom;
    const level=closed.filter(r=>{const a=r[other]*out.zoom+out[other];return a+r[across]*out.zoom>.5&&a<canvas[across]-.5;});
    const seen=d=>level.filter(r=>end(r,d)>.5&&at(r,d)<canvas[size]-.5);
    const cut=d=>seen(d).filter(r=>at(r,d)< -.5).length;
    if(!cut(0))continue;
    let lo=-Infinity,hi=Infinity;
    for(const r of keep){lo=Math.max(lo,pad-at(r,0));hi=Math.min(hi,canvas[size]-pad-end(r,0));}
    if(lo>hi)continue;
    const moves=[0,...level.flatMap(r=>[pad-at(r,0),-end(r,0)])].filter(d=>d>=lo&&d<=hi);
    const better=(d,a)=>cut(d)-cut(a)||seen(a).length-seen(d).length||Math.abs(d)-Math.abs(a);
    out[axis]+=moves.reduce((a,d)=>better(d,a)<0?d:a,0);
  }
  return out;
}

// The camera that shows the whole map, centred, no closer than one unit to
// a pixel.
export function homeCamera(bounds,canvas,pad=16,most=1){
  const zoom=Math.min(most,(canvas.width-2*pad)/bounds.width,(canvas.height-2*pad)/bounds.height);
  return {zoom,x:(canvas.width-bounds.width*zoom)/2-bounds.x*zoom,y:(canvas.height-bounds.height*zoom)/2-bounds.y*zoom};
}
