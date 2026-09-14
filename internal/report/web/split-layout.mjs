import ELK from 'elkjs/lib/elk.bundled.js';
import {connections} from './layout.mjs';
import {overviewRecords} from './overview.mjs';

let engine;
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
const path=segments=>segments.map(points=>points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' ');
const transform=(point,scale,offset)=>({x:offset.x+point.x*scale,y:offset.y+point.y*scale});

// Read only native positions, routes and labels. A wrapper is necessary for
// ELK to import ports owned by the actual root compound; its offset is excluded.
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

// Prepare each saved participant independently of the viewport and the other
// participants' geometry. The boundary ports carry the existing directed refs;
// they are layout joins, not new report items or inferred relations.
export async function prepareInteriors(items,relations,areas,{availableHeight=Infinity}={}){
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
    if(!folded.has(identity))folded.set(identity,{id:`e${folded.size}`,from,to,possible:!!relation.possible,relations:[]});
    folded.get(identity).relations.push(relation);
  }
  const edges=[...folded.values()],aggregates=new Map(),ports=new Map(roots.map(root=>[root.id,new Map()]));
  for(const edge of edges){
    const from=rootOf(edge.from),to=rootOf(edge.to);
    if(from===to)continue;
    const identity=key(from,to,edge.possible);
    if(!aggregates.has(identity))aggregates.set(identity,{id:`outer:${identity}`,from,to,possible:edge.possible,edges:[],relations:[]});
    const aggregate=aggregates.get(identity);aggregate.edges.push(edge.id);aggregate.relations.push(...edge.relations);
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
  const leaves=id=>children.has(id)?children.get(id).flatMap(leaves):[id];
  for(const area of areas.filter(area=>byID.get(area.id)?.branch==='area')){
    for(const group of connections(area.id,leaves(area.id),edges)){
      const outside=byID.get(group.outside),id=`label${labels.size}`;
      labels.set(id,{...group,id,root:rootOf(area.id),title:outside.name||outside.title,
        labelTitle:outside.labelTitle||outside.title,width:outside.labelWidth||180,height:(outside.labelHeight||40)+38});
      const edge=edges.find(edge=>edge.id===group.edges[0]);(edge.labels ||= []).push(id);
    }
  }
  const areaScales=new Map(),interiors=new Map(),preparedRecords=new Map();
  const owner=id=>{for(let at=parent.get(id);at;at=parent.get(at))if(byID.get(at)?.branch==='area')return at;return '';};
  for(const root of roots){
    let inputUnzip=false;
    const members=items.filter(item=>rootOf(item.id)===root.id);
    const ownEdges=edges.filter(edge=>rootOf(edge.from)===root.id||rootOf(edge.to)===root.id);
    let localRecords=new Map(members.map(item=>[item.id,{...item,width:item.width||260,height:item.height||90}]));
    function graph(minimum){
      function tree(id){
        const record=localRecords.get(id),scale=record.contentScale||1;
        const local={...options,'elk.padding':`[top=${record.headerHeight||64},left=${32*scale},bottom=${32*scale},right=${32*scale}]`};
        for(const name of Object.keys(local))if(name.includes('spacing.'))local[name]=String(Number(local[name])*scale);
        const derived=id===root.id?minimum:null;
        const min={width:Math.max(record.minimumWidth||0,derived?.width||0,record.branch==='area'?400:0),
          height:Math.max(record.minimumHeight||0,derived?.height||0,record.branch==='area'?summaries.get(id)?.height||0:0)};
        if(min.width||min.height){local['elk.nodeSize.constraints']='MINIMUM_SIZE';local['elk.nodeSize.minimum']=`(${min.width},${min.height})`;}
        const result=children.has(id)?{id,children:children.get(id).map(tree),layoutOptions:local}
          :{id,width:Math.max(record.width,min.width),height:Math.max(record.height,min.height),layoutOptions:local};
        if(id===root.id){
          result.ports=structuredClone([...ports.get(root.id).values()]);
          result.layoutOptions['elk.portConstraints']='FIXED_SIDE';
          if(inputUnzip)result.layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
        }
        return result;
      }
      const actual=tree(root.id);
      actual.edges=ownEdges.flatMap(edge=>{
        const from=rootOf(edge.from),to=rootOf(edge.to),cross=from!==to;
        if(cross&&(!children.has(root.id)||(from===root.id&&edge.from===root.id)||(to===root.id&&edge.to===root.id)))return [];
        const aggregate=cross?aggregates.get(edge.aggregate):null;
        const source=cross&&from!==root.id?aggregate.targetPort:edge.from;
        const target=cross&&to!==root.id?aggregate.sourcePort:edge.to;
        return [{id:edge.id,sources:[source],targets:[target],labels:(edge.labels||[]).map(id=>labels.get(id)).filter(label=>label.root===root.id).map(label=>{
          const scale=areaScales.get(label.area)||1;
          return {id:label.id,text:label.title,width:label.width*scale,height:label.height*scale,layoutOptions:{'elk.edgeLabels.placement':'CENTER'}};
        })}];
      });
      return {id:`interior:${root.id}`,layoutOptions:inputUnzip?{...options,'elk.layered.layerUnzipping.strategy':'ALTERNATING'}:options,children:[actual]};
    }
    let placed=(await native(graph())).children[0];
    if(root.branch==='inputs'){
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
      const naturalByID=new Map(localGeometry(placed).nodes.map(node=>[node.id,node]));
      for(const area of ownAreas)areaScales.set(area.id,Math.min(1,400/naturalByID.get(area.id).width));
      localRecords=new Map(members.map(item=>{
        const scale=areaScales.get(owner(item.id))||1;
        return [item.id,item.branch==='area'?{...item,contentScale:areaScales.get(item.id),headerHeight:64}
          :{...item,contentScale:scale,width:(item.width||260)*scale,height:(item.height||90)*scale}];
      }));
      placed=(await native(graph())).children[0];
    }
    const preferredWidth=root.overviewPreferredWidth||root.overviewMinWidth||(root.branch==='component'?220:160);
    const preferredHeight=root.overviewHeightAtWidth?.(preferredWidth,{availableHeight})||Math.min(180,placed.height);
    const ratio=preferredWidth/preferredHeight;
    const minimum={width:Math.max(placed.width,placed.height*ratio),height:Math.max(placed.height,placed.width/ratio)};
    placed=(await native(graph(minimum))).children[0];
    const local=localGeometry(placed);
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
// exactly, including its routes and boundary ports, with one uniform transform.
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
  const input={id:'world',layoutOptions:outerOptions,children:prepared.roots.map(root=>{
    const interior=prepared.interiors.get(root.id);
    return {id:root.id,width:interior.width,height:interior.height,ports:structuredClone(interior.ports),
      layoutOptions:{'elk.portConstraints':'FIXED_POS'}};
  }),edges:prepared.aggregates.map(edge=>({id:edge.id,sources:[edge.sourcePort],targets:[edge.targetPort]}))};
  let graph,best;
  for(const unzip of [false,true])for(const direction of ['DOWN','RIGHT']){
    const candidate=structuredClone(input);candidate.layoutOptions['elk.direction']=direction;
    if(unzip)candidate.layoutOptions['elk.layered.layerUnzipping.strategy']='ALTERNATING';
    const placed=await native(candidate),roots=placed.children;
    const minX=Math.min(...roots.map(node=>node.x)),minY=Math.min(...roots.map(node=>node.y));
    const maxX=Math.max(...roots.map(node=>node.x+node.width)),maxY=Math.max(...roots.map(node=>node.y+node.height));
    const zoom=Math.min(.44,Math.max(1,width-48)/(maxX-minX),Math.max(1,height-48)/(maxY-minY));
    const readable=Math.min(1,...roots.map(node=>{
      const record=byID.get(node.id),minimum=record.overviewMinWidth||0;
      const needed=record.overviewHeightAtWidth?.(Math.max(minimum,node.width*zoom),{availableHeight:height-48})||0;
      return Math.min(minimum?node.width*zoom/minimum:1,needed?node.height*zoom/needed:1);
    }));
    const overflow=Math.max((maxX-minX)/width,(maxY-minY)/height);
    if(!best||readable>best.readable||readable===best.readable&&overflow<best.overflow){graph=placed;best={readable,overflow};}
  }
  const nodes=[],labels=[],rootOffsets=new Map(graph.children.map(node=>[node.id,{x:node.x,y:node.y}]));
  const routes=new Map((graph.edges||[]).map(edge=>[edge.id,(edge.sections||[]).map(section=>[section.startPoint,...section.bendPoints||[],section.endPoint])]));
  for(const root of graph.children){
    const interior=prepared.interiors.get(root.id),offset=rootOffsets.get(root.id),scale=interior.scale;
    for(const node of interior.local.nodes)nodes.push({...node,
      position:node.parentId?{x:node.position.x*scale,y:node.position.y*scale}:offset,
      absolute:transform(node.absolute,scale,offset),width:node.width*scale,height:node.height*scale});
    for(const label of interior.local.labels){
      const original=prepared.labels.get(label.id),areaScale=byID.get(original.area)?.contentScale||scale;
      labels.push({...original,x:offset.x+label.x*scale,y:offset.y+label.y*scale,width:label.width*scale,height:label.height*scale,scale:areaScale});
    }
  }
  const rootOf=new Map();for(const [root,interior] of prepared.interiors)for(const node of interior.local.nodes)rootOf.set(node.id,root);
  const localRoute=(root,id)=>{
    const interior=prepared.interiors.get(root),offset=rootOffsets.get(root);
    return (interior.local.edges.get(id)||[]).map(segment=>segment.map(point=>transform(point,interior.scale,offset)));
  };
  const edges=prepared.edges.map(edge=>{
    const from=rootOf.get(edge.from),to=rootOf.get(edge.to);
    const segments=from===to?localRoute(from,edge.id):[
      ...localRoute(from,edge.id),...routes.get(`outer:${edge.aggregate}`)||[],...localRoute(to,edge.id),
    ];
    return {...edge,segments,path:path(segments)};
  });
  return {layout:{nodes,edges,labels,width:graph.width,height:graph.height},records:prepared.records,
    scales:prepared.scales,owner:prepared.owner,summaries:prepared.summaries};
}
