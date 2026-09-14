// Prepared frontend input, not an alternative analysis or a saved layout.
// The ordinary canvas measures, groups, places and routes every item itself.
export const records = [
  {id:'front', title:'Web application', branch:'component', category:'component',
    role:'Browser interface', summary:'Submits jobs and follows their progress through the backend API.',
    children:['editing','tracking']},
  {id:'editing', title:'Job editing', branch:'area', children:['editor','submission']},
  {id:'editor', title:'Job editor', kind:'Part', lane:'core'},
  {id:'submission', title:'Submission and validation', kind:'Part', lane:'core'},
  {id:'submit', title:'Submit a job', activation:'interaction'},
  {id:'tracking', title:'Progress and results', branch:'area', children:['status','results']},
  {id:'status', title:'Progress updates', kind:'Part', lane:'core'},
  {id:'results', title:'Result rendering', kind:'Part', lane:'core'},
  {id:'backend', title:'Job processing service', branch:'component', category:'component',
    role:'API and background workers', summary:'Accepts jobs, stores their state and sends the resulting files to object storage.',
    children:['requests','execution']},
  {id:'requests', title:'Request handling', branch:'area', children:['routes','auth']},
  {id:'routes', title:'HTTP handlers', kind:'Part'},
  {id:'create', title:'POST /api/jobs', activation:'request'},
  {id:'auth', title:'Authentication and permissions', kind:'Part', lane:'core'},
  {id:'execution', title:'Job execution', branch:'area', children:['queue','worker']},
  {id:'queue', title:'Job scheduling', kind:'Part', lane:'core'},
  {id:'worker', title:'Processing worker', kind:'Part', lane:'core'},
  {id:'consume', title:'Process queued jobs', activation:'continuous'},
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
  ['status','get'], ['results','download'], ['post','routes'], ['get','routes'],
  ['download','routes'], ['routes','auth'], ['routes','queue'], ['queue','worker'],
  ['routes','read-jobs'], ['auth','read-jobs'], ['worker','save-jobs'],
  ['queue','publish'], ['worker','claim'], ['worker','put-object'],
  ['worker','get-object'], ['worker','export'], ['routes','export'],
].map(([from,to])=>({from,to}));

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
