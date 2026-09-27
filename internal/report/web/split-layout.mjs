import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {overviewRecords} from './overview.mjs';

let engine;
export const overviewInset=16;
const native=graph=>(engine ||= new ELK()).layout(graph);
const options={
  'elk.algorithm':'layered','elk.direction':'RIGHT','elk.edgeRouting':'ORTHOGONAL',
  'elk.hierarchyHandling':'INCLUDE_CHILDREN','elk.randomSeed':'1',
  'elk.padding':'[top=64,left=32,bottom=32,right=32]',
  'elk.spacing.nodeNode':'40','elk.spacing.edgeNode':'24','elk.spacing.edgeEdge':'12',
  'elk.layered.spacing.nodeNodeBetweenLayers':'70',
  'elk.layered.spacing.edgeNodeBetweenLayers':'24','elk.layered.spacing.edgeEdgeBetweenLayers':'12',
  'elk.layered.mergeEdges':'false','elk.separateConnectedComponents':'true',
};
const key=(...parts)=>JSON.stringify(parts);
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
// are drawn by the outer routes, so no interior leg is lost.
function gridInterior(frame,ratio,top){
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
  const width=Math.max(best.width,best.height*ratio),height=Math.max(best.height,best.width/ratio);
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
      const distance=Math.abs(Math.log(width/height/ratio));
      if(!best||distance<best.distance)best={columns,widths,rows,width,height,distance};
    }
    tiles.forEach((tile,i)=>{
      const column=i%best.columns,row=Math.floor(i/best.columns);
      tile.x=side+best.widths.slice(0,column).reduce((a,b)=>a+b,0)+gap*column;
      tile.y=head+best.rows.slice(0,row).reduce((a,b)=>a+b,0)+gap*row;
    });
    group.width=best.width;group.height=best.height;group.edges=[];
  }
  // Rows of groups: as wide as the collection's proportion asks of their area.
  const area=groups.reduce((sum,group)=>sum+(group.width+gap)*(group.height+gap),0);
  const target=Math.max(...groups.map(group=>group.width),Math.sqrt(area*ratio));
  let x=side,y=top,row=0,right=0;
  for(const group of groups){
    if(x>side&&x+group.width>side+target){x=side;y+=row+gap;row=0;}
    group.x=x;group.y=y;x+=group.width+gap;row=Math.max(row,group.height);right=Math.max(right,group.x+group.width);
  }
  const width=right+side,height=y+row+side;
  for(const port of frame.ports||[]){
    const at=port.layoutOptions?.['elk.port.side'];
    if(at==='EAST'||at==='WEST'){port.x=at==='EAST'?width:0;port.y*=height/frame.height;}
    else{port.y=at==='SOUTH'?height:0;port.x*=width/frame.width;}
  }
  frame.width=width;frame.height=height;frame.edges=[];
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
  const edges=[...folded.values()],aggregates=new Map(),ports=new Map(roots.map(root=>[root.id,new Map()]));
  for(const edge of edges){
    const from=rootOf(edge.from),to=rootOf(edge.to);
    if(from===to)continue;
    const identity=key(from,to,edge.possible);
    if(!aggregates.has(identity))aggregates.set(identity,{id:`outer:${identity}`,from,to,possible:edge.possible,init:true,edges:[],relations:[]});
    const aggregate=aggregates.get(identity);aggregate.edges.push(edge.id);aggregate.relations.push(...edge.relations);aggregate.init&&=!!edge.init;
    for(const [root,other,direction,side] of [[from,to,'out','EAST'],[to,from,'in','WEST']]){
      const portKey=key(other,direction,edge.possible);
      if(!ports.get(root).has(portKey))ports.get(root).set(portKey,{id:`port:${key(root,other,direction,edge.possible)}`,
        width:0,height:0,layoutOptions:{'elk.port.side':side}});
    }
    aggregate.sourcePort=ports.get(from).get(key(to,'out',edge.possible)).id;
    aggregate.targetPort=ports.get(to).get(key(from,'in',edge.possible)).id;
    edge.aggregate=identity;
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
        const aggregate=cross?aggregates.get(edge.aggregate):null;
        const source=cross&&from!==root.id?aggregate.targetPort:edge.from;
        const target=cross&&to!==root.id?aggregate.sourcePort:edge.to;
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
    }
    const preferredWidth=root.overviewPreferredWidth||root.overviewMinWidth||(root.branch==='component'?220:160);
    const preferredHeight=root.overviewHeightAtWidth?.(preferredWidth,{availableHeight})||Math.min(180,placed.height);
    const ratio=preferredWidth/preferredHeight;
    // Cross-frame arrows are drawn by the outer routes alone, so a frame whose
    // tiles no arrow joins needs no interior legs.
    const grouped=root.branch==='inputs'&&(children.get(root.id)||[]).some(id=>children.has(id));
    const loose=!grouped&&root.branch!=='component'&&(children.get(root.id)||[]).length>2&&!ownAreas.length
      &&(children.get(root.id)||[]).every(id=>!children.has(id))
      &&!ownEdges.some(edge=>rootOf(edge.from)===root.id&&rootOf(edge.to)===root.id&&edge.from!==root.id&&edge.to!==root.id);
    if(grouped){
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
        const aggregate=cross?aggregates.get(edge.aggregate):null;
        const source=cross&&from!==root.id?aggregate.targetPort:immediate(edge.from);
        const target=cross&&to!==root.id?aggregate.sourcePort:immediate(edge.to);
        if(source===target)continue;
        const identity=pairKey(source,target);
        if(!bundled.has(identity))bundled.set(identity,{id:`component:${root.id}:${identity}`,sources:[source],targets:[target]});
        edgeBundle.set(edge.id,{id:bundled.get(identity).id,reversed:bundled.get(identity).sources[0]!==source});
      }
      const ready=local.nodes.filter(node=>node.parentId===root.id);
      const componentOptions={...options,'elk.portConstraints':'FIXED_SIDE',
        'elk.padding':`[top=${localRecords.get(root.id).headerHeight||64},left=32,bottom=32,right=32]`};
      if(root.minimumWidth||root.minimumHeight){
        componentOptions['elk.nodeSize.constraints']='MINIMUM_SIZE';
        componentOptions['elk.nodeSize.minimum']=`(${root.minimumWidth||0},${root.minimumHeight||0})`;
      }
      let compact,filled=-Infinity;
      for(const unzip of [false,true]){
        const layoutOptions={...componentOptions};
        if(unzip)layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
        const graph={id:`component-interior:${root.id}`,layoutOptions:options,children:[{
          id:root.id,layoutOptions,ports:structuredClone([...ports.get(root.id).values()]),
          children:ready.map(node=>({id:node.id,width:node.width,height:node.height})),
          edges:structuredClone([...bundled.values()]),
        }]};
        const candidate=localGeometry((await native(graph)).children[0]);
        const aspect=candidate.width/candidate.height,score=Math.min(aspect/ratio,ratio/aspect);
        if(score>filled){compact=candidate;filled=score;}
      }
      const before=new Map(ready.map(node=>[node.id,node]));
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
      local=compact;
    }
    // A fixed unit conversion permits the ordinary .44 overview camera to
    // display the preferred text size. It never uses the eventual fit zoom.
    const scale=Math.max(preferredWidth/local.width,preferredHeight/local.height)/.44;
    const width=local.width*scale,height=local.height*scale;
    interiors.set(root.id,{id:root.id,local,scale,width,height,
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
  // The outer graph has bundled boundary routes and no interior labels. Give
  // those routes their own spacing instead of reserving room for every call's
  // source label again between participants.
  const outerOptions={...options,'elk.spacing.nodeNode':'16','elk.spacing.edgeNode':'8','elk.spacing.edgeEdge':'4',
    'elk.layered.spacing.nodeNodeBetweenLayers':'32',
    'elk.layered.spacing.edgeNodeBetweenLayers':'8','elk.layered.spacing.edgeEdgeBetweenLayers':'4'};
  // Destination frames of different programs that name the same destination
  // stand together in a display group: a compound of the outer graph around
  // them, drawn as a frame that is no participant and ends no arrow. Each
  // frame stays a root with its own program's arrows.
  const members=new Map();
  for(const root of prepared.roots){
    const group=byID.get(root.id)?.displayGroup;if(!group)continue;
    if(!members.has(group))members.set(group,[]);members.get(group).push(root.id);
  }
  for(const [group,ids] of [...members])if(ids.length<2)members.delete(group);
  const groupID=group=>`display-group:${group}`,grouped=new Set([...members.values()].flat());
  const leaf=root=>{const interior=prepared.interiors.get(root.id);return {id:root.id,width:interior.width,height:interior.height,layoutOptions:{}};};
  const input={id:'world',layoutOptions:members.size?{...outerOptions,'elk.hierarchyHandling':'INCLUDE_CHILDREN'}:outerOptions,
    children:[...prepared.roots.filter(root=>!grouped.has(root.id)).map(leaf),
      ...[...members].map(([group,ids])=>({id:groupID(group),layoutOptions:{'elk.padding':'[top=16,left=16,bottom=16,right=16]'},
        children:ids.map(id=>leaf(prepared.roots.find(root=>root.id===id)))}))],
    edges:prepared.aggregates.map(edge=>({id:edge.id,sources:[edge.from],targets:[edge.to]}))};
  // The placed participants, with a group's members at their world position.
  const flat=placed=>placed.children.flatMap(node=>node.children?node.children.map(member=>({...member,x:node.x+member.x,y:node.y+member.y})):[node]);
  const participants=graph=>graph.children.flatMap(node=>node.children||[node]);
  const available={width:Math.max(1,width-2*overviewInset),height:Math.max(1,height-2*overviewInset)};
  function metrics(placed){
    const roots=flat(placed);
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
  let graph,best,bestInput,failure;
  // Both choices belong to ELK. Free boundary endpoints avoid a star collapsing
  // into one strip; the prepared native ports can pack several connected
  // targets more compactly. Compare only these eight flat candidates. Neither
  // alternative adds rendered continuations through the participants. ELK can
  // throw on one candidate and place the others: Redis's three programs, each
  // joined to the others through one shared listener, raised a
  // NullPointerException only with native ports, RIGHT and layer unzipping.
  // Such a candidate is left out; the map fails only when none is placed.
  for(const nativePorts of [false,true])for(const direction of ['DOWN','RIGHT']){
   for(const unzip of [false,true]){
    const candidate=structuredClone(input);candidate.layoutOptions['elk.direction']=direction;
    for(const node of candidate.children)if(node.children)node.layoutOptions['elk.direction']=direction;
    if(nativePorts){
      for(const node of participants(candidate)){node.ports=structuredClone(prepared.interiors.get(node.id).ports);node.layoutOptions['elk.portConstraints']='FIXED_POS';}
      for(const edge of candidate.edges){const original=prepared.aggregates.find(a=>a.id===edge.id);edge.sources=[original.sourcePort];edge.targets=[original.targetPort];}
    }
    if(unzip)candidate.layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
    const template=structuredClone(candidate);
    let placed;
    try{placed=await native(candidate);}catch(error){failure||=error;continue;}
    const score=metrics(placed);
    if(!best||score.readable>best.readable||score.readable===best.readable&&score.overflow<best.overflow){graph=placed;best=score;bestInput=template;}
   }
  }
  if(!best)throw failure;
  for(let correction=0;correction<2&&best.readable<1-1e-7;correction++){
    // Root summaries use physical pixels. A fitted camera below .44 must
    // still reserve their measured minima, including the space their growth
    // removes from that camera. A native placement can change its packing
    // after frames grow; in that case one final correction uses those actual
    // positions. Interiors keep their prepared geometry in both passes.
    // A lower corrected fit affects every summary, including participants
    // that were already readable in the selected candidate.
    const growing=flat(graph).flatMap(node=>{
      const record=byID.get(node.id);
      if(!record.overviewHeightAtWidth)return [];
      const physicalWidth=Math.max(node.width*best.zoom,record.overviewMinWidth||0);
      const physicalHeight=record.overviewHeightAtWidth?.(physicalWidth,{availableHeight:available.height})||0;
      return [{id:node.id,x:node.x,y:node.y,width:node.width,height:node.height,
        needed:{width:Math.ceil(physicalWidth),height:Math.ceil(Math.max(node.height*best.zoom,physicalHeight))}}];
    });
    if(growing.length){
      const reserve=axis=>{
        const coordinate=axis==='width'?'x':'y';
        const ordered=[...growing].sort((a,b)=>a[coordinate]+a[axis]-b[coordinate]-b[axis]);
        // Parallel rows share projected space. Only a nonoverlapping chain
        // adds growth along an axis; summing every row over-reserves the frame
        // and can incorrectly report that no readable fit exists.
        const extent=zoom=>{
          const lengths=[];
          for(const [i,node] of ordered.entries()){
            let preceding=0;
            for(let j=0;j<i;j++)if(ordered[j][coordinate]+ordered[j][axis]<=node[coordinate]+1e-7)
              preceding=Math.max(preceding,lengths[j]);
            lengths.push(preceding+Math.max(0,node.needed[axis]-node[axis]*zoom));
          }
          return best.span[axis]*zoom+Math.max(0,...lengths);
        };
        if(extent(best.zoom)<=available[axis]||extent(0)>=available[axis])return best.zoom;
        // This monotone piecewise-linear envelope is solved in memory. It
        // chooses the reserve for the one existing native correction pass.
        let low=0,high=best.zoom;
        for(let i=0;i<48;i++){
          const middle=(low+high)/2;
          if(extent(middle)<=available[axis])low=middle;else high=middle;
        }
        return low;
      };
      const zoom=Math.min(best.zoom,reserve('width'),reserve('height'));
      const candidate=structuredClone(bestInput);
      for(const node of participants(candidate)){
        const minimum=growing.find(item=>item.id===node.id);if(!minimum)continue;
        node.width=Math.max(node.width,minimum.needed.width/zoom);
        node.height=Math.max(node.height,minimum.needed.height/zoom);
        for(const port of node.ports||[]){
          const side=port.layoutOptions?.['elk.port.side'];
          if(side==='EAST')port.x=node.width;
          if(side==='SOUTH')port.y=node.height;
        }
      }
      const template=structuredClone(candidate);
      let placed;
      // A correction ELK cannot place keeps the placement it would correct.
      try{placed=await native(candidate);}catch{break;}
      const score=metrics(placed);
      if(score.readable>best.readable){graph=placed;best=score;bestInput=template;}else break;
    }
  }
  const rootOf=new Map();for(const [root,interior] of prepared.interiors)for(const node of interior.local.nodes)rootOf.set(node.id,root);
  const placedRoots=flat(graph);
  // A display group is drawn first, under its members.
  const nodes=graph.children.filter(node=>node.children).map(node=>({id:node.id,position:{x:node.x,y:node.y},absolute:{x:node.x,y:node.y},
    width:node.width,height:node.height,frame:true,display:true}));
  const labels=[],rootOffsets=new Map(placedRoots.map(node=>[node.id,{x:node.x,y:node.y}]));
  // The final text reserve can enlarge a participant. Fit its already prepared
  // drawing to that rectangle with one uniform transform instead of leaving a
  // miniature in its corner. No interior layout or zoom-time work is added.
  const interiorScales=new Map(placedRoots.map(root=>{
    const interior=prepared.interiors.get(root.id);
    return [root.id,Math.min(root.width/interior.local.width,root.height/interior.local.height)];
  }));
  const records=prepared.records.map(record=>{
    const root=rootOf.get(record.id),factor=interiorScales.get(root)/prepared.interiors.get(root).scale;
    return {...record,contentScale:(record.contentScale||1)*factor,summaryScale:(record.summaryScale||1)*factor,
      width:record.width*factor,height:record.height*factor};
  });
  for(const record of records)byID.set(record.id,record);
  const scales=new Map([...prepared.scales].map(([id,scale])=>[id,scale*interiorScales.get(rootOf.get(id))/prepared.interiors.get(rootOf.get(id)).scale]));
  // An arrow ELK routes inside a group is given in the group's coordinates.
  const containers=new Map(graph.children.map(node=>[node.id,{x:node.x,y:node.y}]));
  const routes=new Map((graph.edges||[]).map(edge=>{
    const offset=containers.get(edge.container)||{x:0,y:0};
    return [edge.id,(edge.sections||[]).map(section=>[section.startPoint,...section.bendPoints||[],section.endPoint].map(point=>transform(point,1,offset)))];
  }));
  for(const root of placedRoots){
    const interior=prepared.interiors.get(root.id),offset=rootOffsets.get(root.id),scale=interiorScales.get(root.id);
    for(const node of interior.local.nodes)nodes.push({...node,
      position:node.parentId?{x:node.position.x*scale,y:node.position.y*scale}:offset,
      absolute:transform(node.absolute,scale,offset),width:node.parentId?node.width*scale:root.width,height:node.parentId?node.height*scale:root.height});
    for(const label of interior.local.labels){
      const original=prepared.labels.get(label.id),areaScale=byID.get(original.area)?.contentScale||scale;
      if(rootOf.get(original.outside)!==root.id)continue;
      labels.push({...original,x:offset.x+label.x*scale,y:offset.y+label.y*scale,width:label.width*scale,height:label.height*scale,scale:areaScale});
    }
  }
  const localRoute=(root,id)=>{
    const interior=prepared.interiors.get(root),offset=rootOffsets.get(root);
    return (interior.local.edges.get(id)||[]).map(segment=>segment.map(point=>transform(point,interiorScales.get(root),offset)));
  };
  const edges=prepared.edges.map(edge=>{
    const from=rootOf.get(edge.from),to=rootOf.get(edge.to);
    const segments=from===to?localRoute(from,edge.id):routes.get(`outer:${edge.aggregate}`)||[];
    return {...edge,segments,outerSegments:from!==to?routes.get(`outer:${edge.aggregate}`):undefined,
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
  const groups=nodes.filter(node=>node.display).map(node=>({id:node.id,title:'',name:'',branch:'communication-group',category:'external',display:true,children:[]}));
  return {layout:{nodes,edges,labels,width:graph.width,height:graph.height},records:[...records,...groups],
    scales,owner:prepared.owner,summaries:prepared.summaries};
}
