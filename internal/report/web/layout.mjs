import ELK from 'elkjs/lib/elk.bundled.js';

// This is display layout only: containment and every relation are supplied by
// the saved report. ELK sees the actual endpoints, including across containers.
export async function arrange(items, relations, areas, availableWidth = 1200, availableHeight = null) {
  const byID = new Map(items.map(n => [n.id, n]));
  const parent = new Map();
  areas.forEach(a => a.nodes.forEach(id => parent.set(id, a.id)));
  const edges = new Map();
  for (const r of relations) {
    const from=r.displayFrom||r.from,to=r.displayTo||r.to;
    if (from === to || !byID.has(from) || !byID.has(to)) continue;
    const key = JSON.stringify([from, to, !!r.possible]);
    if (!edges.has(key)) edges.set(key, { id: `e${edges.size}`, from, to, possible:!!r.possible, relations:[] });
    edges.get(key).relations.push(r);
  }
  const folded = [...edges.values()];
  const labels = new Map();
  function leaves(id) {
    const a=areas.find(a=>a.id===id);
    return a ? a.nodes.flatMap(leaves) : [id];
  }
  for(const area of areas.filter(a=>byID.get(a.id)?.branch==='area')) {
    for(const group of connections(area.id, leaves(area.id), folded)) {
      const outside=byID.get(group.outside), id=`label${labels.size}`;
      const scale=byID.get(area.id).contentScale||1;
      labels.set(id, {...group,id,scale,title:outside.name||outside.title,labelTitle:outside.labelTitle||outside.title,width:(outside.labelWidth||180)*scale,height:((outside.labelHeight||40)+38)*scale});
      const edge=folded.find(e=>e.id===group.edges[0]);
      (edge.labels ||= []).push({id,text:outside.title,width:labels.get(id).width,height:labels.get(id).height,
        layoutOptions:{'elk.edgeLabels.placement':'CENTER'}});
    }
  }
  const options = {
    'elk.algorithm':'layered', 'elk.direction':'DOWN',
    'elk.edgeRouting':'ORTHOGONAL', 'elk.hierarchyHandling':'INCLUDE_CHILDREN',
    'elk.randomSeed':'1', 'elk.padding':'[top=64,left=32,bottom=32,right=32]',
    'elk.spacing.nodeNode':'40', 'elk.spacing.edgeNode':'24', 'elk.spacing.edgeEdge':'12',
    'elk.layered.spacing.nodeNodeBetweenLayers':'70',
    'elk.layered.spacing.edgeNodeBetweenLayers':'24',
    'elk.layered.spacing.edgeEdgeBetweenLayers':'12',
    'elk.layered.mergeEdges':'false', 'elk.separateConnectedComponents':'true',
  };
  function tree(id) {
    const n = byID.get(id), area = areas.find(a => a.id === id);
    const scale=n.contentScale||1;
    const local={...options,'elk.padding':`[top=${n.headerHeight||64},left=${32*scale},bottom=${32*scale},right=${32*scale}]`};
    for(const key of Object.keys(local))if(key.includes('spacing.'))local[key]=String(Number(local[key])*scale);
    if(n.minimumWidth||n.minimumHeight){
      local['elk.nodeSize.constraints']='MINIMUM_SIZE';
      // Layered ELK interprets the minimum in its horizontal working axes.
      // A DOWN layout rotates them; supply the screen height first.
      local['elk.nodeSize.minimum']=`(${n.minimumHeight||0},${n.minimumWidth||0})`;
    }
    return area ? {id, children:area.nodes.filter(c=>byID.has(c)).map(tree),layoutOptions:local}
      : {id, width:n.width, height:n.height};
  }
  const input = {id:'root', layoutOptions:options,
    children:items.filter(n=>!parent.has(n.id)).map(n=>tree(n.id)),
    edges:folded.map(e=>({id:e.id, sources:[e.from], targets:[e.to],labels:e.labels||[]}))};
  const elk = new ELK();
  let graph;
  {
    graph = await elk.layout(structuredClone(input));
    // Choose once, before displaying. Resizing and selecting never re-layout.
    if (availableHeight || graph.width > availableWidth * 1.4) {
      function horizontal() {
        const candidate=structuredClone(input);
        function orient(n) {
          if(n.layoutOptions){
            n.layoutOptions['elk.direction']='RIGHT';
            const record=byID.get(n.id);
            if(record?.minimumWidth||record?.minimumHeight)n.layoutOptions['elk.nodeSize.minimum']=`(${record.minimumWidth||0},${record.minimumHeight||0})`;
          }
          n.children?.forEach(orient);
        }
        orient(candidate);return candidate;
      }
      const score = g => {
        const height=availableHeight||Math.max(700,availableWidth*.75);
        const overflow=Math.max(g.width/availableWidth,g.height/height);
        const fit=Math.min((availableWidth-48)/g.width,(height-48)/g.height);
        // A slightly smaller whole-map scale is useful when it leaves wide
        // enough target headers to read. Raw bounding area alone favours thin
        // towers whose hidden interiors leave no room for their descriptions.
        const readable=(g.children||[]).map(n=>{
          const branch=byID.get(n.id)?.branch;
          return branch==='component'?Math.min(n.width*fit/200,n.height*fit/180)
            :branch==='communication'?Math.min(n.width*fit/160,n.height*fit/40):1;
        });
        return overflow/Math.min(1,...readable);
      };
      const candidate=await elk.layout(horizontal());
      if(score(candidate)<score(graph))graph=candidate;
    }
  }
  const nodes = [], offsets = {root:{x:0,y:0}}, paths = [], placedLabels=[];
  function collect(n, parentId) {
    for (const c of n.children || []) {
      const offset = offsets[n.id];
      offsets[c.id] = {x:offset.x+c.x, y:offset.y+c.y};
      nodes.push({id:c.id, parentId:parentId || undefined, position:{x:c.x,y:c.y},
        width:c.width,height:c.height, absolute:offsets[c.id], frame:!!c.children});
      collect(c,c.id);
    }
  }
  collect(graph);
  function collectEdges(n) {
    for (const e of n.edges || []) {
      const offset = offsets[e.container || n.id];
      const segments = (e.sections || []).map(s=>[s.startPoint,...s.bendPoints||[],s.endPoint]
        .map(p=>({x:p.x+offset.x,y:p.y+offset.y})));
      const original = folded.find(f=>f.id===e.id);
      paths.push({...original, segments, path:segments.map(s=>s.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' ')});
      for(const label of e.labels||[]) placedLabels.push({...labels.get(label.id),x:label.x+offset.x,y:label.y+offset.y});
    }
    n.children?.forEach(collectEdges);
  }
  collectEdges(graph);
  return {nodes, edges:paths, labels:placedLabels, width:graph.width, height:graph.height};
}

// Group the original relations by outside identity and direction. Numbering
// identifies inner parts, not an inferred execution order.
export function connections(area, members, edges) {
  const own = new Set(members), numbers = new Map(members.map((id,i)=>[id,i+1])), groups = new Map();
  for (const edge of edges) {
    const from = own.has(edge.from), to = own.has(edge.to);
    if(from === to) continue;
    const outside = from ? edge.to : edge.from, key = `${from?'out':'in'}:${outside}`;
    if(!groups.has(key))groups.set(key,{key,area,outside,incoming:!from,insides:new Set(),relations:[],edges:[]});
    const group = groups.get(key);
    group.insides.add(from?edge.from:edge.to);group.relations.push(...edge.relations);group.edges.push(edge.id);
  }
  return [...groups.values()].map(g=>({...g,insides:[...g.insides],numbers:[...g.insides].map(id=>numbers.get(id)).sort((a,b)=>a-b)}));
}
