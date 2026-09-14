// Prepared frontend input, not an alternative analysis or a saved layout.
// The ordinary canvas measures, groups, places and routes every item itself.
export const records = [
  {id:'front', title:'Web application', branch:'component', category:'component',
    role:'Browser interface', summary:'Submits jobs and follows their progress through the backend API.',
    children:['editing','tracking']},
  {id:'editing', title:'Job editing', branch:'area', children:['editor','submission']},
  {id:'editor', title:'Job editor', kind:'Part', lane:'core'},
  {id:'submission', title:'Submission and validation', kind:'Part', lane:'core'},
  {id:'submit', title:'Submit a job', activation:'interaction',componentOwner:'front',componentName:'Web application'},
  {id:'tracking', title:'Progress and results', branch:'area', children:['status','results']},
  {id:'status', title:'Progress updates', kind:'Part', lane:'core'},
  {id:'results', title:'Result rendering', kind:'Part', lane:'core'},
  {id:'backend', title:'Job processing service', branch:'component', category:'component',
    role:'API and background workers', summary:'Accepts jobs, stores their state and sends the resulting files to object storage.',
    children:['requests','execution']},
  {id:'requests', title:'Request handling', branch:'area', children:['routes','auth']},
  {id:'routes', title:'HTTP handlers', kind:'Part'},
  {id:'create', title:'POST /api/jobs', activation:'request',componentOwner:'backend',componentName:'Job processing service'},
  {id:'auth', title:'Authentication and permissions', kind:'Part', lane:'core'},
  {id:'execution', title:'Job execution', branch:'area', children:['queue','worker']},
  {id:'queue', title:'Job scheduling', kind:'Part', lane:'core'},
  {id:'worker', title:'Processing worker', kind:'Part', lane:'core'},
  {id:'consume', title:'Process queued jobs', activation:'continuous',componentOwner:'backend',componentName:'Job processing service'},
  {id:'front-inputs',title:'Web application',kind:'Inputs',branch:'inputs',children:['submit']},
  {id:'backend-inputs',title:'Job processing service',kind:'Inputs',branch:'inputs',children:['create','consume']},
  {id:'api', title:'Backend API', category:'external', branch:'communication', children:['post','get','download']},
  {id:'post', title:'POST /api/jobs', category:'external'},
  {id:'get', title:'GET /api/jobs/{job_id}', category:'external'},
  {id:'download', title:'GET /api/jobs/{job_id}/result', category:'external'},
  {id:'postgres', title:'PostgreSQL', category:'external', branch:'communication', children:['read-jobs','save-jobs']},
  {id:'read-jobs', title:'Read job state and permissions', category:'external'},
  {id:'save-jobs', title:'Save processing results', category:'external'},
  {id:'redis', title:'Redis job queue', category:'external', branch:'communication', children:['publish','claim']},
  {id:'publish', title:'Publish pending jobs', category:'external'},
  {id:'claim', title:'Claim the next job', category:'external'},
  {id:'storage', title:'Amazon S3 object storage', category:'external', branch:'communication', children:['put-object','get-object']},
  {id:'put-object', title:'Upload processed result files', subtitle:'results/{job_id}/output.json', category:'external'},
  {id:'get-object', title:'Download source attachments', category:'external'},
  {id:'telemetry', title:'OpenTelemetry collector', category:'external', branch:'communication', children:['export']},
  {id:'export', title:'Export worker traces and metrics', category:'external'},
];

export const inputOwner = {submit:'submission', create:'routes', consume:'worker'};
export const areas = records.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children}));
export const relations = [
  ['editor','submission'], ['status','results'], ['submission','post'],
  ['status','get'], ['results','download'], ['post','create'], ['get','routes'],
  ['download','routes'], ['routes','auth'], ['routes','queue'], ['queue','worker'],
  ['routes','read-jobs'], ['auth','read-jobs'], ['worker','save-jobs'],
  ['queue','publish'], ['worker','claim'], ['worker','put-object'],
  ['worker','get-object'], ['worker','export'], ['routes','export'],
].map(([from,to])=>({from,to}));
relations.push(
  {from:'submit',to:'submission',label:'implemented in',operations:['submit']},
  {from:'create',to:'routes',label:'implemented in',operations:['create','submit']},
  {from:'consume',to:'worker',label:'implemented in',operations:['consume']},
);
for(const relation of relations){
  if([['submission','post'],['post','create']].some(([from,to])=>relation.from===from&&relation.to===to))relation.operations=['submit'];
}

// The same participants with a dense internal inventory. No geometry is
// supplied: production measurement and fitting must keep the overview usable.
export function denseInventory() {
  const nodes=structuredClone(records),edges=structuredClone(relations),containers=structuredClone(areas);
  for(const root of ['front','backend']){
    const component=nodes.find(n=>n.id===root),container=containers.find(n=>n.id===root);
    for(let index=0;index<40;index++){
      const id=`${root}-workflow-${index+1}`,part=`${id}-part`;
      component.children.push(id);container.nodes.push(id);
      nodes.push({id,title:`Additional workflow ${index+1}`,branch:'area',children:[part]},
        {id:part,title:`Workflow responsibility ${index+1}`,kind:'Part',lane:'core'});
      containers.push({id,nodes:[part]});
      edges.push({from:root==='front'?'submission':'worker',to:part});
    }
  }
  return {records:nodes,relations:edges,areas:containers,inputOwner};
}

// A separate reported shape: modest component inventories, many destinations,
// and several distinct calls inside each destination. Every call has a caller.
export function manyExternalInventory({inputs=true}={}) {
  const nodes=structuredClone(records),edges=structuredClone(relations);
  const partIDs={front:[],backend:[]};
  const extraAreas={front:['Account management','Workspace settings'],backend:['Result processing','Service administration']};
  for(const root of ['front','backend']){
    const component=nodes.find(n=>n.id===root);
    for(const [index,title] of extraAreas[root].entries()){
      const id=`${root}-area-${index+3}`;
      component.children.push(id);nodes.push({id,title,branch:'area',children:[]});
    }
    for(const areaID of component.children){
      const area=nodes.find(n=>n.id===areaID);
      while(area.children.length<5){
        const index=area.children.length+1,id=`${areaID}-part-${index}`;
        nodes.push({id,title:`${area.title} step ${index}`,kind:'Part',lane:'core'});
        if(area.children.length)edges.push({from:area.children.at(-1),to:id});
        area.children.push(id);
      }
      partIDs[root].push(...area.children);
    }
  }
  const extraDestinations=['Search service','Email delivery','Identity provider','Billing API',
    'Audit log service','Feature flags','Notification gateway','Document conversion',
    'Analytics collector','Secrets service','Webhook delivery','Content delivery'];
  for(const [index,title] of extraDestinations.entries())nodes.push({id:`external-${index+6}`,title,category:'external',branch:'communication',children:[]});
  const destinations=nodes.filter(n=>n.branch==='communication');
  for(const [destinationIndex,destination] of destinations.entries()){
    while(destination.children.length<6){
      const index=destination.children.length+1,id=`${destination.id}-call-${index}`;
      nodes.push({id,title:`${destination.title} request ${index}`,category:'external'});
      destination.children.push(id);
    }
    for(const [index,call] of destination.children.entries()){
      if(edges.some(edge=>edge.to===call))continue;
      const caller=partIDs[destinationIndex%3===0?'front':'backend'][(destinationIndex+index)%20];
      edges.push({from:caller,to:call});
    }
  }
  // Also cover a repository with no observed inputs. This is prepared evidence,
  // not a display filter: no input endpoint or operation tag is supplied.
  const retained=inputs?nodes:nodes.filter(n=>!n.activation&&n.branch!=='inputs');
  const ids=new Set(retained.map(n=>n.id));
  const retainedEdges=edges.filter(e=>ids.has(e.from)&&ids.has(e.to)).map(e=>
    e.operations?{...e,operations:e.operations.filter(id=>ids.has(id))}:e);
  return {records:retained,relations:retainedEdges,areas:retained.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children})),inputOwner:inputs?inputOwner:{}};
}

// One analysed service with twenty remote destinations, six calls each. A
// star must remain a usable overview rather than one long layer of boxes.
export function singleTargetInventory() {
  const source=manyExternalInventory({inputs:false}),byID=new Map(source.records.map(n=>[n.id,n]));
  const members=id=>[id,...(byID.get(id).children||[]).flatMap(members)];
  const ids=new Set(members('backend'));
  const nodes=source.records.filter(n=>ids.has(n.id)||n.category==='external');
  for(const [index,title] of ['Run service','Event archive','Artifact registry'].entries()){
    const id=`remote-${index+1}`,children=Array.from({length:6},(_,i)=>`${id}-call-${i+1}`);
    nodes.push({id,title,branch:'communication',category:'external',children},
      ...children.map((id,i)=>({id,title:`${title} request ${i+1}`,category:'external'})));
  }
  const parts=nodes.filter(n=>ids.has(n.id)&&!n.children),destinations=nodes.filter(n=>n.branch==='communication');
  const edges=source.relations.filter(e=>ids.has(e.from)&&ids.has(e.to));
  destinations.forEach((destination,i)=>destination.children.forEach((to,j)=>edges.push({from:parts[(i+j)%parts.length].id,to})));
  return {records:nodes,relations:edges,areas:nodes.filter(n=>n.children).map(n=>({id:n.id,nodes:n.children})),inputOwner:{}};
}

// Ordinary reports have less vertical canvas space than the standalone fixture.
// Short component names must still reserve room for their longer inventories.
export function shortNamedInventory() {
  const nodes=structuredClone(records),edges=structuredClone(relations),owners={...inputOwner};
  const names={
    front:['Application shell and navigation','Page and playground views','Simulation view',
      'Level rendering','Backend API access','Shared domain and utility support','Playground styling'],
    backend:['Backend launch and configuration','HTTP API surface','Level content and schemas',
      'Simulation domain','Submitted code constraints'],
  };
  for(const [root,titles] of Object.entries(names)){
    const component=nodes.find(node=>node.id===root);component.title=root;
    for(const [index,title] of titles.entries()){
      if(index<component.children.length){nodes.find(node=>node.id===component.children[index]).title=title;continue;}
      const id=`${root}-area-${index+1}`,part=`${id}-part`;
      component.children.push(id);
      nodes.push({id,title,branch:'area',children:[part]},
        {id:part,title:`${title} implementation`,kind:'Part',lane:'core'});
      edges.push({from:root==='front'?'submission':'worker',to:part});
    }
    const collection=nodes.find(node=>node.id===`${root}-inputs`);collection.title=root;
  }
  // Eleven front inputs and three requests reproduce the input catalogue shape
  // without copying a saved report or supplying any layout coordinates.
  const frontInputs=['Update job code','Change progress slider','Change refresh interval','Display job list',
    'Load job details','Render error page','Resize results canvas','Run a job','Toggle progress updates','Animate progress'];
  const front=nodes.find(node=>node.id==='front'),frontCollection=nodes.find(node=>node.id==='front-inputs');
  for(const [index,title] of frontInputs.entries()){
    const id=`front-input-${index+2}`,area=nodes.find(node=>node.id===front.children[index%front.children.length]);
    nodes.push({id,title,activation:index===frontInputs.length-1?'continuous':'interaction',componentOwner:'front',componentName:'front'});
    frontCollection.children.push(id);owners[id]=area.children[0];
    edges.push({from:id,to:owners[id],label:'implemented in',operations:[id]});
  }
  const backendCollection=nodes.find(node=>node.id==='backend-inputs');backendCollection.children=['create'];
  for(const [index,title] of ['GET /api/jobs','GET /api/jobs/{job_id}'].entries()){
    const id=`backend-input-${index+2}`;
    nodes.push({id,title,activation:'request',componentOwner:'backend',componentName:'backend'});
    backendCollection.children.push(id);owners[id]='routes';
    edges.push({from:id,to:'routes',label:'implemented in',operations:[id]});
  }
  for(const node of nodes)if(node.activation)node.componentName=node.componentOwner;
  const omitted=new Set(nodes.filter(node=>node.branch==='communication'&&node.id!=='api')
    .flatMap(node=>[node.id,...node.children]));omitted.add('consume');delete owners.consume;
  const retained=nodes.filter(node=>!omitted.has(node.id)),ids=new Set(retained.map(node=>node.id));
  return {records:retained,relations:edges.filter(edge=>ids.has(edge.from)&&ids.has(edge.to)),
    areas:retained.filter(node=>node.children).map(node=>({id:node.id,nodes:node.children})),inputOwner:owners};
}
