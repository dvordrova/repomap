import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {overviewRecords} from './overview.mjs';
import {chip,chipGrid} from './cards.mjs';

let engine;
export const overviewInset=16;
const native=graph=>(engine ||= new ELK()).layout(graph);
const options={
  'elk.algorithm':'layered','elk.direction':'RIGHT','elk.edgeRouting':'ORTHOGONAL',
  'elk.hierarchyHandling':'INCLUDE_CHILDREN','elk.randomSeed':'1',
  'elk.padding':'[top=64,left=32,bottom=32,right=32]',
  // Parallel arrows 16 apart, an arrow 28 from a box it passes: at 12 and
  // 24, Core runtime's arrows ran as one band around its parts.
  'elk.spacing.nodeNode':'40','elk.spacing.edgeNode':'28','elk.spacing.edgeEdge':'16',
  'elk.layered.spacing.nodeNodeBetweenLayers':'70',
  'elk.layered.spacing.edgeNodeBetweenLayers':'28','elk.layered.spacing.edgeEdgeBetweenLayers':'16',
  'elk.layered.mergeEdges':'false','elk.separateConnectedComponents':'true',
};
const key=(...parts)=>JSON.stringify(parts);
// The lanes of a packed program's gutters, in its own units where it is
// entered at `zoom`: ten screen pixels apart and sixteen from the cards,
// clear of a 10.5px arrowhead (canvas.jsx).
export const packedRoom=zoom=>({step:10/zoom,margin:16/zoom});
// Whether the arrows (`{sources:[from],targets:[to]}`) among `nodes` run
// round a directed cycle.
export function cyclic(edges,nodes){
  const next=new Map([...nodes].map(id=>[id,[]]));
  for(const edge of edges)if(next.has(edge.sources[0])&&next.has(edge.targets[0]))next.get(edge.sources[0]).push(edge.targets[0]);
  const state=new Map();
  const visit=id=>{
    if(state.get(id)===1)return true;
    if(state.get(id)===2)return false;
    state.set(id,1);
    if(next.get(id).some(visit))return true;
    state.set(id,2);return false;
  };
  return [...next.keys()].some(visit);
}
// The drawing shares one route between the two directions of a pair of ends
// (route-drawing.mjs), so ELK lays out one edge per pair. Laying out both
// made it reverse one of every such pair into an arrow wrapped around the
// area: Server runtime's 21 arrows among six parts had 68 bends.
const pairKey=(a,b)=>a<b?key(a,b):key(b,a);
const reversed=segments=>segments.slice().reverse().map(points=>points.slice().reverse());
// The scale, relative to its reading scale, at which an open area's 17px
// part headings reach the 12px its layer stays open at (semantic.mjs).
export const readableScale=12/17;
const routeLength=node=>(node.edges||[]).reduce((sum,edge)=>sum+(edge.sections||[]).reduce((total,section)=>{
  const points=[section.startPoint,...section.bendPoints||[],section.endPoint];
  return total+points.slice(1).reduce((length,point,i)=>length+Math.abs(point.x-points[i].x)+Math.abs(point.y-points[i].y),0);
},0),0);
const path=segments=>segments.map(points=>points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' ');
const transform=(point,scale,offset)=>({x:offset.x+point.x*scale,y:offset.y+point.y*scale});

// Read only native positions, routes and labels. A wrapper is necessary for
// ELK to import ports owned by the actual root compound; its offset is excluded.
// Tiles no arrow joins are one layer to a layered drawing: a column in a
// frame stretched to its summary's proportion. Rows of the column count
// nearest that proportion fill the frame instead. Arrows that cross the frame
// are drawn by the outer routes, so no interior leg is lost. A frame that
// keeps its content's own box (`fitted`) is not stretched to the proportion.
function gridInterior(frame,ratio,top,fitted=false){
  const gap=24,side=32,tiles=[...frame.children].sort((a,b)=>a.y-b.y||a.x-b.x);
  const arrange=columns=>{
    const widths=Array(columns).fill(0),rows=[];
    tiles.forEach((tile,i)=>{widths[i%columns]=Math.max(widths[i%columns],tile.width);
      rows[Math.floor(i/columns)]=Math.max(rows[Math.floor(i/columns)]||0,tile.height);});
    return {columns,widths,rows,width:2*side+widths.reduce((a,b)=>a+b,0)+gap*(columns-1),
      height:top+side+rows.reduce((a,b)=>a+b,0)+gap*(rows.length-1)};
  };
  let best=null;
  for(let columns=1;columns<=tiles.length;columns++){
    const candidate=arrange(columns),distance=Math.abs(Math.log(candidate.width/candidate.height/ratio));
    if(!best||distance<best.distance)best={...candidate,distance};
  }
  const width=fitted?best.width:Math.max(best.width,best.height*ratio),height=fitted?best.height:Math.max(best.height,best.width/ratio);
  const left=side+(width-best.width)/2,down=top+(height-best.height)/2;
  tiles.forEach((tile,i)=>{
    const column=i%best.columns,row=Math.floor(i/best.columns);
    tile.x=left+best.widths.slice(0,column).reduce((a,b)=>a+b,0)+gap*column;
    tile.y=down+best.rows.slice(0,row).reduce((a,b)=>a+b,0)+gap*row;
  });
  for(const port of frame.ports||[]){
    const at=port.layoutOptions?.['elk.port.side'];
    if(at==='EAST'||at==='WEST'){port.x=at==='EAST'?width:0;port.y*=height/frame.height;}
    else{port.y=at==='SOUTH'?height:0;port.x*=width/frame.width;}
  }
  frame.width=width;frame.height=height;frame.edges=[];
}

// An input collection whose inputs stand in part groups: each group is a
// grid of its tiles under its title, and the groups fill the collection's
// rows in their reading order toward the collection's summary proportion.
// Arrows that cross the collection are drawn by the outer routes alone.
const groupProportion=1.6;
function groupedInterior(frame,ratio,top,order,headers){
  const gap=24,side=32,rank=id=>{const at=order.indexOf(id);return at<0?order.length:at;};
  const groups=[...frame.children].sort((a,b)=>rank(a.id)-rank(b.id));
  for(const group of groups){
    if(!group.children?.length)continue;
    const tiles=[...group.children].sort((a,b)=>rank(a.id)-rank(b.id)),head=headers.get(group.id)||40;
    let best=null;
    for(let columns=1;columns<=tiles.length;columns++){
      const widths=Array(columns).fill(0),rows=[];
      tiles.forEach((tile,i)=>{widths[i%columns]=Math.max(widths[i%columns],tile.width);rows[Math.floor(i/columns)]=Math.max(rows[Math.floor(i/columns)]||0,tile.height);});
      const width=2*side+widths.reduce((a,b)=>a+b,0)+gap*(columns-1),height=head+side+rows.reduce((a,b)=>a+b,0)+gap*(rows.length-1);
      // A group of many inputs wraps into rows wider than tall, whatever
      // the collection's summary: to its tall proportion, freqtrade's REST
      // API server had stood 440 by 840 beside groups a fifth its width.
      const distance=Math.abs(Math.log(width/height/groupProportion));
      if(!best||distance<best.distance)best={columns,widths,rows,width,height,distance};
    }
    tiles.forEach((tile,i)=>{
      const column=i%best.columns,row=Math.floor(i/best.columns);
      tile.x=side+best.widths.slice(0,column).reduce((a,b)=>a+b,0)+gap*column;
      tile.y=head+best.rows.slice(0,row).reduce((a,b)=>a+b,0)+gap*row;
    });
    group.width=best.width;group.height=best.height;group.edges=[];
  }
  // Rows of groups, a little wider than tall: stacked to the collection's
  // tall summary, its groups stood in one column taller than the canvas.
  const natural=new Map(groups.map(group=>[group.id,{width:group.width,height:group.height}]));
  let width=0,height=0;
  const wrap=()=>{
    const area=groups.reduce((sum,group)=>sum+(group.width+gap)*(group.height+gap),0);
    const target=Math.max(...groups.map(group=>group.width),Math.sqrt(area*Math.max(ratio,1.2)));
    let x=side,y=top,row=0,right=0;
    for(const group of groups){
      if(x>side&&x+group.width>side+target){x=side;y+=row+gap;row=0;}
      group.x=x;group.y=y;x+=group.width+gap;row=Math.max(row,group.height);right=Math.max(right,group.x+group.width);
    }
    width=right+side;height=y+row+side;
  };
  wrap();
  // Entered whole, the collection shows its groups as closed cards
  // (canvas.jsx frameView): each is at least the card a title reads in
  // there, 150 by 56 screen pixels on an ordinary canvas. Sized by their
  // one or two inputs, Redis's Replication and litestream's Directory
  // monitoring had stood 57 by 28, their names at 4px (reviewer,
  // 2026-09-30). The tiles keep their places at the card's top left.
  for(let pass=0;pass<3;pass++){
    const zoom=Math.min(1000/width,560/height);
    for(const group of groups){
      const own=natural.get(group.id);
      group.width=Math.max(own.width,150/zoom);group.height=Math.max(own.height,56/zoom);
    }
    wrap();
  }
  for(const port of frame.ports||[]){
    const at=port.layoutOptions?.['elk.port.side'];
    if(at==='EAST'||at==='WEST'){port.x=at==='EAST'?width:0;port.y*=height/frame.height;}
    else{port.y=at==='SOUTH'?height:0;port.x*=width/frame.width;}
  }
  frame.width=width;frame.height=height;frame.edges=[];
}

// A program's Outside frame: its chips in the order the page gives them
// (the destinations not established last), in rows of one chip size
// (cards.mjs chipGrid). No arrow joins two chips; the frame's arrows are
// drawn by the outer routes alone.
function chipInterior(frame,order){
  const grid=chipGrid(frame.children.length),rank=id=>{const at=order.indexOf(id);return at<0?order.length:at;};
  [...frame.children].sort((a,b)=>rank(a.id)-rank(b.id)).forEach((tile,i)=>{
    tile.width=chip.width;tile.height=chip.height;
    tile.x=chip.side+(i%grid.columns)*(chip.width+chip.gap);tile.y=chip.top+Math.floor(i/grid.columns)*(chip.height+chip.gap);
  });
  for(const port of frame.ports||[]){
    const at=port.layoutOptions?.['elk.port.side'];
    if(at==='EAST'||at==='WEST'){port.x=at==='EAST'?grid.width:0;port.y*=grid.height/frame.height;}
    else{port.y=at==='SOUTH'?grid.height:0;port.x*=grid.width/frame.width;}
  }
  frame.width=grid.width;frame.height=grid.height;frame.edges=[];
}

// A participant's drawing widened to `ratio` (width over height), its
// contents moved to the middle; its ports stay on their sides.
function widenTo(local,ratio){
  const width=Math.max(local.width,local.height*ratio),dx=(width-local.width)/2;
  if(dx<=0)return local;
  const root=local.nodes.find(node=>!node.parentId);
  const shift=point=>({...point,x:point.x+dx});
  return {...local,width,
    nodes:local.nodes.map(node=>node===root?{...node,width}:{...node,absolute:shift(node.absolute),position:node.parentId===root?.id?shift(node.position):node.position}),
    edges:new Map([...local.edges].map(([id,segments])=>[id,segments.map(points=>points.map(shift))])),
    labels:local.labels.map(shift),
    ports:local.ports.map(port=>{const side=port.layoutOptions?.['elk.port.side'];return side==='EAST'?{...port,x:width}:side==='WEST'?port:{...port,x:port.x+dx};})};
}

function localGeometry(root){
  const nodes=[],edges=new Map(),labels=[],offsets=new Map([[root.id,{x:0,y:0}]]);
  function walk(node,parentId){
    const absolute=offsets.get(node.id);
    nodes.push({id:node.id,parentId,position:parentId?{x:node.x,y:node.y}:{x:0,y:0},absolute,
      width:node.width,height:node.height,frame:!!node.children?.length});
    for(const child of node.children||[]){offsets.set(child.id,{x:absolute.x+child.x,y:absolute.y+child.y});walk(child,node.id);}
  }
  walk(root);
  function routes(node){
    for(const edge of node.edges||[]){
      const offset=offsets.get(edge.container||node.id)||{x:0,y:0};
      const segments=(edge.sections||[]).map(section=>[section.startPoint,...section.bendPoints||[],section.endPoint]
        .map(point=>transform(point,1,offset)));
      edges.set(edge.id,segments);
      for(const label of edge.labels||[])labels.push({...label,x:offset.x+label.x,y:offset.y+label.y});
    }
    for(const child of node.children||[])routes(child);
  }
  routes(root);
  return {nodes,edges,labels,ports:root.ports||[],width:root.width,height:root.height};
}

// ELK prepares each participant and its original boundary ports independently.
// Cross-root continuations provide placement evidence but are not painted.
export async function prepareInteriors(items,relations,areas,{availableHeight=Infinity,canvas=null}={}){
  const byID=new Map(items.map(item=>[item.id,item]));
  const children=new Map(areas.map(area=>[area.id,area.nodes.filter(id=>byID.has(id))]));
  const parent=new Map();for(const [id,members] of children)for(const member of members)parent.set(member,id);
  const rootOf=id=>{while(parent.has(id))id=parent.get(id);return id;};
  const roots=items.filter(item=>!parent.has(item.id));
  const folded=new Map();
  for(const relation of relations){
    const from=relation.displayFrom||relation.from,to=relation.displayTo||relation.to;
    if(from===to||!byID.has(from)||!byID.has(to))continue;
    const identity=key(from,to,!!relation.possible);
    if(!folded.has(identity))folded.set(identity,{id:`e${folded.size}`,from,to,possible:!!relation.possible,init:true,relations:[]});
    const entry=folded.get(identity);entry.relations.push(relation);entry.init&&=!!relation.init;
  }
  // Two participants are joined by one outer route, whatever their arrows
  // between them carry and in whichever direction: the drawing paints one
  // arrow per pair of ends (route-drawing.mjs), headed at each end its
  // arrows go into, dashed only when all of them are possible. Laid out
  // once per direction and certainty, the unpainted routes still took
  // their room between the frames.
  const edges=[...folded.values()],aggregates=new Map(),ports=new Map(roots.map(root=>[root.id,new Map()]));
  for(const edge of edges){
    const from=rootOf(edge.from),to=rootOf(edge.to);
    if(from===to)continue;
    const identity=pairKey(from,to);
    if(!aggregates.has(identity)){
      const portOf=new Map();
      for(const [root,other,side] of [[from,to,'EAST'],[to,from,'WEST']]){
        const port={id:`port:${key(root,other)}`,width:0,height:0,layoutOptions:{'elk.port.side':side}};
        ports.get(root).set(other,port);portOf.set(root,port.id);
      }
      aggregates.set(identity,{id:`outer:${identity}`,from,to,possible:true,init:true,edges:[],relations:[],portOf,
        sourcePort:portOf.get(from),targetPort:portOf.get(to)});
    }
    const aggregate=aggregates.get(identity);aggregate.edges.push(edge.id);aggregate.relations.push(...edge.relations);
    aggregate.init&&=!!edge.init;aggregate.possible&&=!!edge.possible;
    edge.aggregate=identity;edge.againstOuter=from!==aggregate.from;
  }
  const summaries=new Map(overviewRecords(items,areas).records.map(item=>[item.id,item]));
  const labels=new Map();
  const areaScales=new Map(),interiors=new Map(),preparedRecords=new Map();
  const owner=id=>{for(let at=parent.get(id);at;at=parent.get(at))if(byID.get(at)?.branch==='area')return at;return '';};
  for(const root of roots){
    let inputUnzip=false;
    const members=items.filter(item=>rootOf(item.id)===root.id);
    const ownEdges=edges.filter(edge=>rootOf(edge.from)===root.id||rootOf(edge.to)===root.id);
    let localRecords=new Map(members.map(item=>[item.id,{...item,width:item.width||260,height:item.height||90}]));
    const childOfRoot=id=>{while(parent.has(id)&&parent.get(id)!==root.id)id=parent.get(id);return id;};
    // A component's areas are laid out each on its own, from the arrows
    // between its own parts: the component places them as ready rectangles
    // and draws every other arrow between those rectangles. Laid out with the
    // whole component, Server runtime's seven parts took seven global layers
    // and stood as a staircase in a frame twenty times their height.
    const ownInteriors=root.branch==='component';
    // How each area lays out its own parts: its direction and whether a long
    // chain wraps into rows. Chosen per area below.
    const areaLayouts=new Map();
    let twins=new Map();
    function tree(id,minimum){
        const record=localRecords.get(id),scale=record.contentScale||1;
        const local={...options,'elk.padding':`[top=${record.headerHeight||64},left=${32*scale},bottom=${32*scale},right=${32*scale}]`};
        for(const name of Object.keys(local))if(name.includes('spacing.'))local[name]=String(Number(local[name])*scale);
        const derived=id===root.id?minimum:null;
        // A loose part is a peer of the area's summary, not a miniature of
        // an interior card. Give both the same column width before routing.
        const peer=record.branch==='area'||root.branch==='component'&&parent.get(id)===root.id&&!children.has(id);
        const min={width:Math.max(record.minimumWidth||0,derived?.width||0,peer?400:0),
          height:Math.max(record.minimumHeight||0,derived?.height||0,peer&&!children.has(id)?Math.max(record.height,200):0)};
        if(min.width||min.height){local['elk.nodeSize.constraints']='MINIMUM_SIZE';local['elk.nodeSize.minimum']=`(${min.width},${min.height})`;}
        if(ownInteriors&&children.has(id))local['elk.hierarchyHandling']='SEPARATE_CHILDREN';
        // An area grown to its siblings' floor holds its parts in its middle.
        if(ownInteriors&&children.has(id)&&id!==root.id&&record.minimumWidth)local['elk.contentAlignment']='V_CENTER H_CENTER';
        if(ownInteriors&&children.has(id)&&id!==root.id){
          const chosen=areaLayouts.get(id)||{direction:'RIGHT',wrap:true};
          local['elk.direction']=chosen.direction;
          // A chain of parts may wrap into rows toward the canvas proportion
          // instead of one row as long as the chain.
          if(chosen.wrap){local['elk.layered.wrapping.strategy']='MULTI_EDGE';if(canvas)local['elk.aspectRatio']=String(canvas.width/canvas.height);}
        }
        const result=children.has(id)?{id,children:children.get(id).map(child=>tree(child)),layoutOptions:local}
          :{id,width:Math.max(record.width,min.width),height:Math.max(record.height,min.height),layoutOptions:local};
        if(id===root.id){
          result.ports=structuredClone([...ports.get(root.id).values()]);
          result.layoutOptions['elk.portConstraints']='FIXED_SIDE';
          if(inputUnzip)result.layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
        }
        return result;
    }
    // The root's own edges for ELK, one per pair of ends; `twins` gives every
    // other edge of a pair the laid-out one's route.
    function interiorEdges(inside=()=>true){
      const laid=new Map();twins=new Map();
      return ownEdges.flatMap(edge=>{
        const from=rootOf(edge.from),to=rootOf(edge.to),cross=from!==to;
        if(ownInteriors&&(cross||childOfRoot(edge.from)!==childOfRoot(edge.to)))return [];
        if(cross&&(!children.has(root.id)||(from===root.id&&edge.from===root.id)||(to===root.id&&edge.to===root.id)))return [];
        if(!inside(edge))return [];
        const port=cross?aggregates.get(edge.aggregate).portOf.get(root.id):null;
        const source=cross&&from!==root.id?port:edge.from;
        const target=cross&&to!==root.id?port:edge.to;
        const pair=pairKey(source,target),first=laid.get(pair);
        if(first){twins.set(edge.id,{id:first.id,reversed:first.source!==source});return [];}
        laid.set(pair,{id:edge.id,source});
        return [{id:edge.id,sources:[source],targets:[target]}];
      });
    }
    function graph(minimum){
      const actual=tree(root.id,minimum);
      actual.edges=interiorEdges();
      return {id:`interior:${root.id}`,layoutOptions:inputUnzip?{...options,'elk.layered.layerUnzipping.strategy':'ALTERNATING'}:options,children:[actual]};
    }
    let placed=(await native(graph())).children[0];
    const variants=[];
    if(root.branch==='inputs'&&!(children.get(root.id)||[]).some(id=>children.has(id))){
      const width=root.overviewMinWidth||160;
      const height=root.overviewHeightAtWidth?.(width,{availableHeight})||Math.min(180,placed.height),ratio=width/height;
      const filled=node=>Math.min((node.width/node.height)/ratio,ratio/(node.width/node.height));
      // A long one-column catalogue can require an otherwise empty wide frame.
      // Compare the native column alternative against its own summary aspect;
      // short catalogues keep the ordinary arrangement when it wastes less space.
      inputUnzip=true;
      const alternative=(await native(graph())).children[0];
      if(filled(alternative)>filled(placed))placed=alternative;else inputUnzip=false;
    }
    const ownAreas=members.filter(item=>item.branch==='area');
    if(ownAreas.length){
      // Every area draws its parts at their own size, the size of the loose
      // parts beside it: an area is as large as what it holds. Shrunk to a
      // peer's width, Server runtime's parts became postage stamps under a
      // full-size title, with arrowheads larger than the boxes, and opened
      // unreadable beside the areas whose text had opened the layer.
      for(const area of ownAreas)areaScales.set(area.id,1);
      localRecords=new Map(members.map(item=>{
        const scale=areaScales.get(owner(item.id))||1;
        return [item.id,item.branch==='area'?{...item,contentScale:areaScales.get(item.id),headerHeight:64}
          :{...item,contentScale:scale,width:(item.width||260)*scale,height:(item.height||90)*scale}];
      }));
      // Each area takes, of ELK's directions with and without wrapping, the
      // layout that fits this canvas while its parts stay readable, then the
      // one whose arrows run shortest: the fewest detours and arrows wrapped
      // around the area; then the squarer box, whose closed title reads
      // larger. Wrapping had wrapped Persistence's one arrow around it and
      // drawn Server runtime 1300 px wide in a 1214 px canvas.
      if(canvas)for(const area of ownAreas){
        const room={width:Math.max(1,canvas.width-48)/readableScale,height:Math.max(1,canvas.height-48)/readableScale};
        let best=null;
        for(const direction of ['RIGHT','DOWN'])for(const wrap of [false,true]){
          areaLayouts.set(area.id,{direction,wrap});
          const result=await native({id:`area:${area.id}`,layoutOptions:options,children:[tree(area.id)],edges:interiorEdges(edge=>childOfRoot(edge.from)===area.id)});
          const laid=result.children[0],fits=laid.width<=room.width&&laid.height<=room.height;
          const score={direction,wrap,fits,shrink:Math.max(laid.width/room.width,laid.height/room.height),length:routeLength(result),
            square:Math.abs(Math.log(laid.width/laid.height))};
          if(!best||(fits!==best.fits?fits:!fits?score.shrink<best.shrink:score.length!==best.length?score.length<best.length:score.square<best.square))best=score;
        }
        areaLayouts.set(area.id,{direction:best.direction,wrap:best.wrap});
      }
      placed=(await native(graph())).children[0];
      // The closed cards of one component share one size rule: none is
      // smaller than nine twentieths of its largest area on either side, or
      // half again its own size.
      // As large only as what they held, litestream's eight loose parts had
      // stood at a third of Command line interface's width and read at 8px
      // beside its title, and Redis's Client command handling at a fifth.
      const areaBoxes=placed.children.filter(child=>children.has(child.id));
      const floor={width:.45*Math.max(0,...areaBoxes.map(child=>child.width)),height:.45*Math.max(0,...areaBoxes.map(child=>child.height))};
      let floored=false;
      // A card grows at most by half again on a side: forty loose parts
      // beside two areas are forty cards, not forty areas.
      for(const child of placed.children){
        if(child.width>=floor.width&&child.height>=floor.height)continue;
        const record=localRecords.get(child.id);
        record.minimumWidth=Math.max(record.minimumWidth||0,Math.min(floor.width,1.5*child.width));
        record.minimumHeight=Math.max(record.minimumHeight||0,Math.min(floor.height,1.5*child.height));floored=true;
      }
      if(floored)placed=(await native(graph())).children[0];
    }
    const preferredWidth=root.overviewPreferredWidth||root.overviewMinWidth||(root.branch==='component'?220:160);
    const preferredHeight=root.overviewHeightAtWidth?.(preferredWidth,{availableHeight,preferred:true})||Math.min(180,placed.height);
    const ratio=preferredWidth/preferredHeight;
    // Cross-frame arrows are drawn by the outer routes alone, so a frame whose
    // tiles no arrow joins needs no interior legs.
    const grouped=root.branch==='inputs'&&(children.get(root.id)||[]).some(id=>children.has(id));
    const loose=!grouped&&root.branch!=='component'&&(children.get(root.id)||[]).length>2&&!ownAreas.length
      &&(children.get(root.id)||[]).every(id=>!children.has(id))
      &&!ownEdges.some(edge=>rootOf(edge.from)===root.id&&rootOf(edge.to)===root.id&&edge.from!==root.id&&edge.to!==root.id);
    const outside=root.branch==='outside';
    if(outside)chipInterior(placed,children.get(root.id)||[]);
    else if(grouped){
      const order=[...children.get(root.id),...children.get(root.id).flatMap(id=>children.get(id)||[])];
      groupedInterior(placed,ratio,localRecords.get(root.id).headerHeight||64,order,new Map([...localRecords].map(([id,record])=>[id,record.headerHeight||40])));
    }else if(loose)gridInterior(placed,ratio,localRecords.get(root.id).headerHeight||64);
    else if(root.branch!=='component'){
      const minimum={width:Math.max(placed.width,placed.height*ratio),height:Math.max(placed.height,placed.width/ratio)};
      placed=(await native(graph(minimum))).children[0];
    }
    let local=localGeometry(placed);
    for(const [id,twin] of twins)local.edges.set(id,twin.reversed?reversed(local.edges.get(twin.id)||[]):local.edges.get(twin.id)||[]);
    if(root.branch==='component'&&children.get(root.id)?.length){
      // Areas already own their native interiors. Place only these ready
      // rectangles, so an interior edge cannot stretch the whole component.
      const immediate=id=>{
        while(parent.has(id)&&parent.get(id)!==root.id)id=parent.get(id);
        return id;
      };
      const bundled=new Map(),edgeBundle=new Map();
      for(const edge of ownEdges){
        const from=rootOf(edge.from),to=rootOf(edge.to),cross=from!==to;
        if(cross&&((from===root.id&&edge.from===root.id)||(to===root.id&&edge.to===root.id)))continue;
        const port=cross?aggregates.get(edge.aggregate).portOf.get(root.id):null;
        const source=cross&&from!==root.id?port:immediate(edge.from);
        const target=cross&&to!==root.id?port:immediate(edge.to);
        if(source===target)continue;
        const identity=pairKey(source,target);
        if(!bundled.has(identity))bundled.set(identity,{id:`component:${root.id}:${identity}`,sources:[source],targets:[target]});
        edgeBundle.set(edge.id,{id:bundled.get(identity).id,reversed:bundled.get(identity).sources[0]!==source});
      }
      const ready=local.nodes.filter(node=>node.parentId===root.id);
      // The same room for arrows between a component's areas as between
      // the map's frames: the spacing in the unit of the boxes it places,
      // their median height over three part cards. In part units, the arrows
      // between freqtrade's areas ran four pixels from the areas they
      // passed at the zoom its frame is entered at.
      const heights=ready.map(node=>node.height).sort((a,b)=>a-b),unit=Math.max(1,(heights[Math.floor(heights.length/2)]||0)/300);
      const componentOptions={...options,'elk.portConstraints':'FIXED_SIDE',
        'elk.padding':`[top=${localRecords.get(root.id).headerHeight||64},left=32,bottom=32,right=32]`};
      for(const name of Object.keys(componentOptions))if(name.includes('spacing.'))componentOptions[name]=String(Math.round(Number(componentOptions[name])*unit));
      // Arrows into one side of an area meet it at one point, and out of one
      // side leave it at one: one arrowhead per side, not five to eight
      // stacked on an area's top, and a lane per trunk, not per pair. Not
      // when the component's cards call round a cycle: the arrow closing it
      // runs against the others and, forced through their point, went round
      // a third card (redis-server's Client command handling → Core server
      // infrastructure boxed Data type commands in; reviewer, 2026-09-30).
      componentOptions['elk.layered.mergeEdges']=cyclic(bundled.values(),new Set(ready.map(node=>node.id)))?'false':'true';
      if(root.minimumWidth||root.minimumHeight){
        componentOptions['elk.nodeSize.constraints']='MINIMUM_SIZE';
        componentOptions['elk.nodeSize.minimum']=`(${root.minimumWidth||0},${root.minimumHeight||0})`;
      }
      const before=new Map(ready.map(node=>[node.id,node]));
      // A placement of the ready rectangles as the component's whole
      // drawing: every node inside an area moves with its area, and every
      // arrow between two of them takes their bundle's route.
      const finish=compact=>{
        const after=new Map(compact.nodes.map(node=>[node.id,node]));
        const offset=id=>{
          const child=immediate(id),a=before.get(child),b=after.get(child);
          return a&&b?{x:b.absolute.x-a.absolute.x,y:b.absolute.y-a.absolute.y}:{x:0,y:0};
        };
        compact.nodes=local.nodes.map(node=>after.has(node.id)?{...after.get(node.id),frame:node.frame}
          :{...node,absolute:transform(node.absolute,1,offset(node.id))});
        const routes=new Map();
        for(const edge of ownEdges){
          const bundle=edgeBundle.get(edge.id),route=bundle&&(compact.edges.get(bundle.id)||[]);
          routes.set(edge.id,bundle?bundle.reversed?reversed(route):route
            :(local.edges.get(edge.id)||[]).map(segment=>segment.map(point=>transform(point,1,offset(edge.from)))));
        }
        compact.edges=routes;
        return compact;
      };
      let compact,filled=-Infinity;
      // Of both directions, with and without unzipping, the arrangement
      // nearest its closed summary's proportion: laid out only to the right,
      // freqtrade's eight areas stood 3.7 times wider than tall under a
      // summary three fifths as wide as tall, and its frame opened with an
      // empty band four fifths of its height. Every arrangement is kept: the
      // whole-map fit takes the one nearest the box it grows the component
      // to (layoutPrepared), so the frame still hugs its areas.
      for(const direction of ['RIGHT','DOWN'])for(const unzip of [false,true]){
        const layoutOptions={...componentOptions,'elk.direction':direction};
        if(unzip)layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
        const graph={id:`component-interior:${root.id}`,layoutOptions:{...options,'elk.direction':direction},children:[{
          id:root.id,layoutOptions,ports:structuredClone([...ports.get(root.id).values()]),
          children:ready.map(node=>({id:node.id,width:node.width,height:node.height})),
          edges:structuredClone([...bundled.values()]),
        }]};
        const candidate=finish(localGeometry((await native(graph)).children[0]));
        const aspect=candidate.width/candidate.height,score=Math.min(aspect/ratio,ratio/aspect);
        variants.push(candidate);
        if(score>filled){compact=candidate;filled=score;}
      }
      // A program whose areas the arrangement spreads so thin that, entered
      // whole, its smallest card stands under 64 pixels tall (a title at
      // about eleven pixels) packs its cards instead: in a grid toward the
      // canvas's proportion, in the reading order of their first
      // arrangement. Its arrows run in the gutters between the grid's rows
      // and columns; every arrow at one side of a card meets it at one
      // point and runs in the card's one lane there, one trunk until they
      // part, one arrowhead. Laid out by their
      // dependencies, freqtrade's fourteen areas had covered 7% of their
      // frame, entered at 16 pixels tall inside a hundred arrows; drawn
      // straight between the packed cards, the arrows had crossed them.
      const room=canvas?{width:Math.max(1,canvas.width-24),height:Math.max(1,canvas.height-24)}:{width:1030,height:686};
      const legible=layout=>Math.min(...ready.map(node=>layout.nodes.find(other=>other.id===node.id)?.height||0))*Math.min(room.width/layout.width,room.height/layout.height);
      if(ready.length>2&&Math.max(...variants.map(legible))<64){
        const reference=new Map(variants[0].nodes.map(node=>[node.id,node.absolute]));
        const order=[...ready].sort((a,b)=>reference.get(a.id).x-reference.get(b.id).x||reference.get(a.id).y-reference.get(b.id).y);
        const gap=Number(componentOptions['elk.spacing.nodeNode'])*2,top=localRecords.get(root.id).headerHeight||64;
        // Gutter k runs before column (row) k, the last one after the last,
        // `gutters` wide: by default `gap` between cards, and the frame's
        // own 32 padding outside them.
        const pack=(columns,gutters={},room=0)=>{
          const rows=Math.ceil(order.length/columns),cell=new Map(order.map((node,i)=>[node.id,{row:Math.floor(i/columns),column:i%columns}]));
          const widths=Array(columns).fill(0),heights=Array(rows).fill(0);
          order.forEach((node,i)=>{widths[i%columns]=Math.max(widths[i%columns],node.width);heights[Math.floor(i/columns)]=Math.max(heights[Math.floor(i/columns)],node.height);});
          const across=(gutters.column||[]).concat(),down=(gutters.row||[]).concat();
          for(let k=0;k<=columns;k++)across[k]??=k===0||k===columns?32:gap;for(let k=0;k<=rows;k++)down[k]??=k===0||k===rows?32:gap;
          const xs=[across[0]],ys=[top+down[0]];
          widths.forEach((w,k)=>xs.push(xs.at(-1)+w+across[k+1]));heights.forEach((h,k)=>ys.push(ys.at(-1)+h+down[k+1]));
          const boxes=new Map(order.map(node=>{const {row,column}=cell.get(node.id);
            return [node.id,{x:xs[column]+(widths[column]-node.width)/2,y:ys[row]+(heights[row]-node.height)/2,width:node.width,height:node.height,row,column}];}));
          return {columns,rows,room,boxes,xs,ys,size:{row:down,column:across},width:xs.at(-1),height:ys.at(-1)};
        };
        const fit=packed=>Math.min(room.width/packed.width,room.height/packed.height);
        let best=null;
        for(let columns=1;columns<=order.length;columns++){
          const packed=pack(columns),score=Math.min(...[...packed.boxes.values()].map(box=>box.height))*fit(packed);
          if(!best||score>best.score)best={...packed,score};
        }
        const portList=[...ports.get(root.id).values()];
        const middle=box=>({x:box.x+box.width/2,y:box.y+box.height/2});
        // The routes over one packing: a route is its turning points; a
        // coordinate that is a lane in a gutter is settled once every route
        // has claimed its lanes.
        const route=({boxes,xs,ys,size,width,height,columns,rows,room})=>{
          const sides={EAST:[],WEST:[]},portAt=new Map();
          for(const port of portList)(sides[port.layoutOptions['elk.port.side']]||sides.WEST).push(port);
          // Under the title band: an arrow from a port runs down the
          // frame's outer gutter, never through the program's title.
          for(const [name,list] of Object.entries(sides))list.forEach((port,i)=>portAt.set(port.id,{x:name==='EAST'?width:0,y:top+size.row[0]+(height-top-size.row[0])*(i+1)/(list.length+1),east:name==='EAST'}));
          const lanes=new Map(),claim=(key,id)=>{if(!lanes.has(key))lanes.set(key,[]);if(!lanes.get(key).includes(id))lanes.get(key).push(id);return {key,id};};
          // A card meets the gutter above or below it at one point and has
          // one lane in that gutter: every arrow at that side is one trunk
          // there until they part. The point stands an eighth of the card
          // left of the middle below it and right of it above, so the stubs
          // of two cards facing each other across a gutter never run on one
          // line.
          const side=(id,down)=>{const box=boxes.get(id);return {x:middle(box).x+(down?-1:1)*box.width/8,y:down?box.y+box.height:box.y};};
          const lane=(row,id)=>claim(`row:${row}`,id);
          // How many arrows meet a card's side in a gutter: an arrow alone
          // at a card's side goes in along the lane it came by, taking no
          // lane of the card's own (forty cards called from one had widened
          // every gutter by forty lanes).
          const meeting=new Map(),meet=(id,row)=>meeting.set(`${id}:${row}`,(meeting.get(`${id}:${row}`)||0)+1);
          for(const bundle of bundled.values()){
            const a=boxes.get(bundle.sources[0]),b=boxes.get(bundle.targets[0]);
            if(!a||!b)continue;
            meet(bundle.sources[0],b.row>a.row?a.row+1:a.row);meet(bundle.targets[0],b.row>a.row?b.row:b.row===a.row?b.row:b.row+1);
          }
          const plans=new Map();
          for(const bundle of bundled.values()){
            const [source,target]=[bundle.sources[0],bundle.targets[0]],a=boxes.get(source),b=boxes.get(target);
            if(a&&b){
              if(a.row===b.row&&Math.abs(a.column-b.column)===1){
                // Neighbours in a row face each other across one gutter.
                const low=Math.max(a.y,b.y),high=Math.min(a.y+a.height,b.y+b.height);
                if(high-low>gap/2){
                  const right=b.column>a.column,y=(low+high)/2;
                  plans.set(bundle.id,[{x:right?a.x+a.width:a.x,y},{x:right?b.x:b.x+b.width,y}]);continue;
                }
              }
              // Out of the source toward the target's row, along the
              // source's lane, through a column gutter when the rows differ,
              // along the target's lane and into it from the gutter on the
              // source's side.
              const exit=side(source,b.row>a.row),entry=side(target,b.row<a.row);
              const from=b.row>a.row?a.row+1:a.row,to=b.row>a.row?b.row:b.row===a.row?b.row:b.row+1;
              const first=lane(from,source),second=meeting.get(`${target}:${to}`)>1?lane(to,target):from===to?first:lane(to,source);
              if(from===to)plans.set(bundle.id,[exit,{x:exit.x,y:first},{x:entry.x,y:first},{x:entry.x,y:second},entry]);
              else{
                const column=b.column>a.column?b.column:b.column<a.column?b.column+1:a.column+1,across=claim(`column:${column}`,source);
                plans.set(bundle.id,[exit,{x:exit.x,y:first},{x:across,y:first},{x:across,y:second},{x:entry.x,y:second},entry]);
              }
              continue;
            }
            // A port on the frame's side: through the outer column gutter on
            // that side, from the card's lane beside it.
            const card=a?source:target,port=portAt.get(a?target:source);if(!boxes.has(card)||!port)continue;
            const box=boxes.get(card),below=port.y>middle(box).y,exit=side(card,below),own=lane(below?box.row+1:box.row,card),across=claim(`column:${port.east?columns:0}`,card);
            const path=[exit,{x:exit.x,y:own},{x:across,y:own},{x:across,y:port.y},{x:port.x,y:port.y}];
            plans.set(bundle.id,a?path:path.reverse());
          }
          // Each route's lane in a gutter: a gutter between cards shared
          // evenly, a gutter outside them from the cards outward, the frame's
          // padding kept beyond its last lane. In a row gutter the lanes of
          // the cards above it run above the lanes of the cards below it: a
          // card's stub into its lane never runs along the stub of the card
          // facing it across the gutter.
          // Lanes stand `room.step` apart and `room.margin` from the cards,
          // so an arrowhead entering a card crosses no lane (packedRoom).
          const place=({key,id})=>{
            const [kind,at]=key.split(':'),index=Number(at),edges=kind==='row'?ys:xs,wide=size[kind][index],last=kind==='row'?rows:columns;
            const list=kind==='row'?[...lanes.get(key)].sort((a,b)=>(boxes.get(b).row<index)-(boxes.get(a).row<index)):lanes.get(key),k=list.indexOf(id)+1,n=list.length;
            if(!room)return index===0?edges[0]-wide*(n+1-k)/(n+2):index===last?edges[index]-wide+wide*k/(n+2):edges[index]-wide+wide*k/(n+1);
            const {step,margin}=room;
            if(index===0)return edges[0]-margin-step*(n-k);
            if(index===last)return edges[index]-wide+margin+step*(k-1);
            return edges[index]-wide+(wide-step*(n-1))/2+step*(k-1);
          };
          const routes=new Map();
          for(const [id,plan] of plans){
            // A turn that goes on along the same line is no turn: dropped,
            // a lane farther from the card than the arrow's own does not
            // make it double back.
            const points=[];
            for(const point of plan.map(point=>({x:typeof point.x==='object'?place(point.x):point.x,y:typeof point.y==='object'?place(point.y):point.y}))){
              if(points.length&&point.x===points.at(-1).x&&point.y===points.at(-1).y)continue;
              const [p,q]=points.slice(-2);
              if(q&&(p.x===q.x&&q.x===point.x||p.y===q.y&&q.y===point.y))points.pop();
              points.push(point);
            }
            routes.set(id,[points]);
          }
          return {routes,lanes,portAt};
        };
        // A gutter is wide enough for its lanes ten pixels apart and
        // sixteen from the cards where the program is entered: five apart
        // and five from the cards, freqtrade's seven arrows between its two
        // rows had read as one bus, and an arrowhead into a card had crossed
        // the lanes above it into a broken chevron (reviewer, 2026-09-30).
        let packed=best,routed=route(best);
        for(let pass=0;pass<3;pass++){
          const room=packedRoom(fit(packed)),gutters={row:[],column:[]};
          for(const [key,list] of routed.lanes){
            const [kind,at]=key.split(':'),index=Number(at),outer=index===0||index===(kind==='row'?best.rows:best.columns);
            gutters[kind][index]=outer?32+room.margin+room.step*(list.length-1):Math.max(gap,2*room.margin+room.step*(list.length-1));
          }
          packed=pack(best.columns,gutters,room);routed=route(packed);
        }
        const {boxes,width,height}=packed,{routes,portAt}=routed;
        const packedLocal=finish({width,height,labels:[],edges:routes,
          ports:portList.map(port=>({...port,x:portAt.get(port.id).x,y:portAt.get(port.id).y})),
          nodes:[{id:root.id,position:{x:0,y:0},absolute:{x:0,y:0},width,height,frame:true},
            ...ready.map(node=>{const box=boxes.get(node.id);return {id:node.id,parentId:root.id,position:{x:box.x,y:box.y},absolute:{x:box.x,y:box.y},width:box.width,height:box.height,frame:node.frame};})]});
        variants.length=0;variants.push(packedLocal);compact=packedLocal;
      }
      local=compact;
      // A program of one or two parts and no area (a script) takes its
      // summary's proportion, its parts in the middle: laid out to their
      // own, freqtrade's scripts had stood two and a half times as tall as
      // their title and description, an empty card under them (reviewer,
      // 2026-09-30).
      if(ready.length<=2&&ready.every(node=>!node.frame)&&local.width/local.height<ratio){
        const widened=new Map(variants.map(variant=>[variant,widenTo(variant,ratio)]));
        variants.splice(0,variants.length,...variants.map(variant=>widened.get(variant)));
        local=widened.get(local)||widenTo(local,ratio);
      }
    }
    // A fixed unit conversion permits the ordinary .44 overview camera to
    // display the preferred text size. It never uses the eventual fit zoom.
    const scale=Math.max(preferredWidth/local.width,preferredHeight/local.height)/.44;
    const width=local.width*scale,height=local.height*scale;
    interiors.set(root.id,{id:root.id,local,scale,width,height,variants:variants.length?variants:[local],
      ports:local.ports.map(port=>({...port,x:port.x*scale,y:port.y*scale}))});
    for(const item of members){
      const record=localRecords.get(item.id),contentScale=(record.contentScale||1)*scale;
      preparedRecords.set(item.id,{...record,contentScale,summaryScale:scale,
        originalWidth:item.width||260,originalHeight:item.height||90,
        width:(record.width||260)*scale,height:(record.height||90)*scale});
    }
  }
  const records=items.map(item=>preparedRecords.get(item.id));
  const scales=new Map([...areaScales].map(([id])=>[id,preparedRecords.get(id).contentScale]));
  return {roots,records,interiors,edges,aggregates:[...aggregates.values()],labels,scales,owner,summaries};
}

// Resizing only places ready participant rectangles. Every interior is reused
// exactly with one uniform transform. Outer arrows end at these rectangles.
export async function layoutPrepared(prepared,width=1200,height=700){
  if(!prepared.roots.length)return {layout:{nodes:[],edges:[],labels:[],width:0,height:0},records:prepared.records,
    scales:prepared.scales,owner:prepared.owner,summaries:prepared.summaries};
  const byID=new Map(prepared.records.map(record=>[record.id,record]));
  // The outer graph keeps room for its arrows (owner, 2026-09-29: "и
  // стрелкам место оставлять на главной карте"): three fifths of the
  // interiors' spacing (options) in screen pixels at the camera it is laid
  // out for (`outerOptions`). Laid out at a sixth of the interiors' spacing
  // in world units, the whole map's arrows had run in gaps of a pixel along
  // other frames' borders and bunched into combs. When the fit's correction
  // grows the boxes for a smaller camera the room grows with them, shrinking
  // on screen as the square root of that camera and never below half its
  // size (`screenRoom`): kept whole it left the fit no way to keep its
  // summaries readable, kept in world units it fell to half a pixel on
  // Redis's map. `roomFor` is the camera the room is laid out for.
  const screenRoom=zoom=>Math.max(Math.sqrt(zoom/.44),.5);
  const roomFor=zoom=>zoom/screenRoom(zoom);
  const outerOptions=camera=>{
    const spaced={...options};
    for(const name of Object.keys(spaced))if(name.includes('spacing.'))spaced[name]=String(Math.round(Number(spaced[name])*.6/camera));
    return spaced;
  };
  const leaf=root=>{const interior=prepared.interiors.get(root.id);return {id:root.id,width:interior.width,height:interior.height,layoutOptions:{}};};
  // The arrangement each participant is drawn in: its prepared one, or,
  // for a component grown by the fit, the one nearest its grown box.
  let drawnAs=new Map(prepared.roots.map(root=>[root.id,prepared.interiors.get(root.id).local]));
  const input={id:'world',layoutOptions:outerOptions(.44),children:prepared.roots.map(leaf),
    edges:prepared.aggregates.map(edge=>({id:edge.id,sources:[edge.from],targets:[edge.to]}))};
  const available={width:Math.max(1,width-2*overviewInset),height:Math.max(1,height-2*overviewInset)};
  function metrics(placed){
    const roots=placed.children;
    const span={width:Math.max(...roots.map(node=>node.x+node.width))-Math.min(...roots.map(node=>node.x)),
      height:Math.max(...roots.map(node=>node.y+node.height))-Math.min(...roots.map(node=>node.y))};
    const zoom=Math.min(.44,available.width/span.width,available.height/span.height);
    const readable=Math.min(1,...roots.map(node=>{
      const record=byID.get(node.id),minimum=record.overviewMinWidth||0;
      const needed=record.overviewHeightAtWidth?.(node.width*zoom,{availableHeight:available.height})||0;
      return Math.min(minimum?node.width*zoom/minimum:1,needed?node.height*zoom/needed:1);
    }));
    return {span,zoom,readable,overflow:Math.max(span.width/width,span.height/height)};
  }
  let graph,best,failure;
  // Both choices belong to ELK. Free boundary endpoints avoid a star collapsing
  // into one strip; the prepared native ports can pack several connected
  // targets more compactly. Compare only these eight flat candidates. Neither
  // alternative adds rendered continuations through the participants. ELK can
  // throw on one candidate and place the others: Redis's three programs, each
  // joined to the others through one shared listener, raised a
  // NullPointerException only with native ports, RIGHT and layer unzipping.
  // Such a candidate is left out; the map fails only when none is placed.
  // `sizes` are the boxes' world sizes, the prepared ones at first.
  async function place(sizes=new Map(),zoom=.44,locals=drawnAs){
    let chosen=null;
    for(const nativePorts of [false,true])for(const direction of ['DOWN','RIGHT'])for(const unzip of [false,true]){
      const candidate=structuredClone(input);
      Object.assign(candidate.layoutOptions,outerOptions(roomFor(zoom)));candidate.layoutOptions['elk.direction']=direction;
      for(const node of candidate.children){
        const size=sizes.get(node.id);if(!size)continue;
        node.width=Math.max(node.width,size.width);node.height=Math.max(node.height,size.height);
      }
      if(nativePorts){
        for(const node of candidate.children){
          const drawn=locals.get(node.id);
          node.ports=structuredClone(drawn.ports).map(port=>{
            const side=port.layoutOptions?.['elk.port.side'];
            return {...port,x:side==='EAST'?node.width:side==='WEST'?0:port.x*node.width/drawn.width,
              y:side==='SOUTH'?node.height:side==='NORTH'?0:port.y*node.height/drawn.height};
          });
          node.layoutOptions['elk.portConstraints']='FIXED_POS';
        }
        for(const edge of candidate.edges){const original=prepared.aggregates.find(a=>a.id===edge.id);edge.sources=[original.sourcePort];edge.targets=[original.targetPort];}
      }
      if(unzip)candidate.layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
      let placed;
      try{placed=await native(candidate);}catch(error){failure||=error;continue;}
      const score=metrics(placed);
      if(!chosen||score.readable>chosen.score.readable||score.readable===chosen.score.readable&&score.overflow<chosen.score.overflow)chosen={placed,score};
    }
    return chosen;
  }
  const first=await place();
  if(!first)throw failure;
  ({placed:graph,score:best}=first);
  // The camera the placed graph's room was laid out for.
  let spacedFor=.44;
  for(let correction=0;correction<4&&best.readable<1-1e-7;correction++){
    // Root summaries use physical pixels. A fitted camera below .44 must
    // still reserve their measured minima, including the space their growth
    // removes from that camera. The grown boxes are placed anew, every
    // candidate again: kept to the first one's direction, the ordinary
    // fixture's two programs had stood in one column too tall for the
    // canvas while side by side they fit. Interiors keep their prepared
    // geometry. A lower corrected fit affects every summary, including
    // participants that were already readable before.
    // A box grows to its summary's physical minimum on each side. A
    // component grows whole, in the arrangement of its areas nearest that
    // box (prepareInteriors keeps every one), so its open areas still fill
    // it: grown only in height, freqtrade's frame had opened with its areas
    // in the top fifth and an empty band under them.
    const locals=new Map(drawnAs);
    const growing=graph.children.flatMap(node=>{
      const record=byID.get(node.id);
      if(!record.overviewHeightAtWidth)return [];
      const physicalWidth=Math.max(node.width*best.zoom,record.overviewMinWidth||0);
      const physicalHeight=record.overviewHeightAtWidth?.(physicalWidth,{availableHeight:available.height})||0;
      const needed={width:Math.ceil(physicalWidth),height:Math.ceil(Math.max(node.height*best.zoom,physicalHeight))};
      const variants=prepared.interiors.get(node.id).variants||[];
      if(record.branch==='component'&&variants.length>1&&(needed.width>node.width*best.zoom+.5||needed.height>node.height*best.zoom+.5)){
        const cover=local=>{const aspect=local.width/local.height,width=Math.max(needed.width,needed.height*aspect);return {local,width,height:width/aspect};};
        const chosen=variants.map(cover).sort((a,b)=>a.width*a.height-b.width*b.height)[0];
        locals.set(node.id,chosen.local);
        needed.width=Math.ceil(chosen.width);needed.height=Math.ceil(chosen.height);
      }
      return [{id:node.id,x:node.x,y:node.y,width:node.width,height:node.height,needed}];
    });
    if(!growing.length)break;
    const reserve=axis=>{
      const coordinate=axis==='width'?'x':'y';
      const ordered=[...growing].sort((a,b)=>a[coordinate]+a[axis]-b[coordinate]-b[axis]);
      // Parallel rows share projected space. Only a nonoverlapping chain
      // adds growth along an axis; summing every row over-reserves the frame
      // and can incorrectly report that no readable fit exists.
      // The axis's length no box covers is the arrows' room: laid out again
      // for a camera `zoom`, it takes roomFor(spacedFor)/roomFor(zoom) of
      // its world length, room·screenRoom(zoom) on screen.
      const intervals=graph.children.map(node=>[node[coordinate],node[coordinate]+node[axis]]).sort((a,b)=>a[0]-b[0]);
      let covered=0,reach=-Infinity;
      for(const [start,end] of intervals){if(end>reach){covered+=end-Math.max(start,reach);reach=end;}}
      const room=Math.max(0,best.span[axis]-covered)*roomFor(spacedFor);
      const extent=zoom=>{
        const lengths=[];
        for(const [i,node] of ordered.entries()){
          let preceding=0;
          for(let j=0;j<i;j++)if(ordered[j][coordinate]+ordered[j][axis]<=node[coordinate]+1e-7)
            preceding=Math.max(preceding,lengths[j]);
          lengths.push(preceding+Math.max(0,node.needed[axis]-node[axis]*zoom));
        }
        return Math.min(best.span[axis],covered)*zoom+room*screenRoom(zoom)+Math.max(0,...lengths);
      };
      if(extent(best.zoom)<=available[axis]||extent(0)>=available[axis])return best.zoom;
      // This monotone piecewise-linear envelope is solved in memory.
      let low=0,high=best.zoom;
      for(let i=0;i<48;i++){
        const middle=(low+high)/2;
        if(extent(middle)<=available[axis])low=middle;else high=middle;
      }
      return low;
    };
    const zoom=Math.min(best.zoom,reserve('width'),reserve('height'));
    const sizes=new Map(graph.children.map(node=>[node.id,{width:node.width,height:node.height}]));
    for(const item of growing){
      const local=locals.get(item.id),aspect=local.width/local.height;
      let width=Math.max(item.width,item.needed.width/zoom),height=Math.max(item.height,item.needed.height/zoom);
      // A component's box keeps its arrangement's proportion.
      if(local!==drawnAs.get(item.id)||byID.get(item.id).branch==='component'){width=Math.max(width,height*aspect);height=width/aspect;}
      sizes.set(item.id,{width,height});
    }
    const next=await place(sizes,zoom,locals);
    if(next&&next.score.readable>best.readable){graph=next.placed;best=next.score;spacedFor=zoom;drawnAs=locals;}else break;
  }
  const rootOf=new Map();for(const [root,interior] of prepared.interiors)for(const node of interior.local.nodes)rootOf.set(node.id,root);
  const placedRoots=graph.children;
  const nodes=[],labels=[],rootOffsets=new Map(placedRoots.map(node=>[node.id,{x:node.x,y:node.y}]));
  // The final text reserve can enlarge a participant. Fit its already prepared
  // drawing to that rectangle with one uniform transform instead of leaving a
  // miniature in its corner. No interior layout or zoom-time work is added.
  const interiorScales=new Map(placedRoots.map(root=>{
    const local=drawnAs.get(root.id);
    return [root.id,Math.min(root.width/local.width,root.height/local.height)];
  }));
  const records=prepared.records.map(record=>{
    const root=rootOf.get(record.id),factor=interiorScales.get(root)/prepared.interiors.get(root).scale;
    return {...record,contentScale:(record.contentScale||1)*factor,summaryScale:(record.summaryScale||1)*factor,
      width:record.width*factor,height:record.height*factor};
  });
  for(const record of records)byID.set(record.id,record);
  const scales=new Map([...prepared.scales].map(([id,scale])=>[id,scale*interiorScales.get(rootOf.get(id))/prepared.interiors.get(rootOf.get(id)).scale]));
  const routes=new Map((graph.edges||[]).map(edge=>[edge.id,(edge.sections||[]).map(section=>[section.startPoint,...section.bendPoints||[],section.endPoint])]));
  for(const root of placedRoots){
    const local=drawnAs.get(root.id),offset=rootOffsets.get(root.id),scale=interiorScales.get(root.id);
    for(const node of local.nodes)nodes.push({...node,
      position:node.parentId?{x:node.position.x*scale,y:node.position.y*scale}:offset,
      absolute:transform(node.absolute,scale,offset),width:node.parentId?node.width*scale:root.width,height:node.parentId?node.height*scale:root.height});
    for(const label of local.labels){
      const original=prepared.labels.get(label.id),areaScale=byID.get(original.area)?.contentScale||scale;
      if(rootOf.get(original.outside)!==root.id)continue;
      labels.push({...original,x:offset.x+label.x*scale,y:offset.y+label.y*scale,width:label.width*scale,height:label.height*scale,scale:areaScale});
    }
  }
  const localRoute=(root,id)=>{
    const offset=rootOffsets.get(root);
    return (drawnAs.get(root).edges.get(id)||[]).map(segment=>segment.map(point=>transform(point,interiorScales.get(root),offset)));
  };
  // An arrow the other way along its pair's one route takes that route
  // reversed: it starts where the arrow does.
  const outerRoute=edge=>{const route=routes.get(`outer:${edge.aggregate}`)||[];return edge.againstOuter?reversed(route):route;};
  const edges=prepared.edges.map(edge=>{
    const from=rootOf.get(edge.from),to=rootOf.get(edge.to);
    const segments=from===to?localRoute(from,edge.id):outerRoute(edge);
    return {...edge,segments,outerSegments:from!==to?outerRoute(edge):undefined,
      outerFrom:from!==to?from:undefined,outerTo:from!==to?to:undefined,path:path(segments)};
  });
  const children=new Map();
  for(const node of nodes)if(node.parentId){if(!children.has(node.parentId))children.set(node.parentId,[]);children.get(node.parentId).push(node.id);}
  const leaves=id=>children.has(id)?children.get(id).flatMap(leaves):[id];
  for(const area of prepared.records.filter(record=>record.branch==='area')){
    const root=rootOf.get(area.id);
    for(const group of connections(area.id,leaves(area.id),edges,id=>rootOf.get(id)===root?id:rootOf.get(id))){
      if(rootOf.get(group.outside)===root)continue;
      const edge=edges.find(edge=>edge.id===group.edges[0]),route=edge?.outerSegments;
      const outside=byID.get(group.outside);
      if(route?.length)labels.push({...group,id:`boundary:${area.id}:${group.key}`,boundary:true,root,
        title:outside.name||outside.title,point:group.incoming?route.at(-1).at(-1):route[0][0]});
    }
  }
  return {layout:{nodes,edges,labels,width:graph.width,height:graph.height},records,
    scales,owner:prepared.owner,summaries:prepared.summaries};
}
