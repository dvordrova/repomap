import * as prepared from './two-systems-five-externals.mjs';

const options=new URLSearchParams(location.search);
const {records,relations,areas,inputOwner}=options.has('single-target')?prepared.singleTargetInventory():options.has('short-names')?prepared.shortNamedInventory({matchedPeer:options.has('matched-peer')}):options.has('many-external')
  ?prepared.manyExternalInventory({inputs:!options.has('no-inputs')}):options.has('dense')?prepared.denseInventory():prepared;
if(options.has('many-external'))document.querySelector('h1').textContent='Two systems · seventeen external participants';
if(options.has('short-names'))document.querySelector('h1').textContent='Short component names · complete initial inventories';
if(options.has('single-target'))document.querySelector('h1').textContent='One system · twenty external participants';
// A part in no area stands beside the backend's areas.
if(options.has('loose-part')){
  records.find(n=>n.id==='backend').children.push('audit');
  areas.find(a=>a.id==='backend').nodes.push('audit');
  records.push({id:'audit',title:'Audit log',kind:'Part',summary:'Records every job decision for later review.'});
  relations.push({from:'worker',to:'audit'});
}

// The handlers hand work to both of the execution area's parts: the arrow
// from Request handling ends at every part of Job execution.
if(options.has('both-parts'))relations.push({from:'routes',to:'worker'});

// Every part says what it is under its name. Redis's Replication and client
// sentences are among them: pre-broken at 228px they wrapped again in the
// card's 225px text column. pykrx's fundamentals sentence starts with a
// 225.39px line: measured apart in the 225px column, it fit whole where the
// browser drew the card's 1.5px border 1px wide.
if(options.has('described')){
  const sentences=['Synchronizes master and replica data over network connections.','Provides an interactive terminal client for sending Redis commands.',
    'Implements Redis set commands and their operations.','Fetches Korean market fundamentals and sector data by date or ticker.','Checks who may change a job and records every decision it takes for the audit trail, then reports refusals.'];
  records.filter(n=>n.kind==='Part'||n.kind==='Entrypoints').forEach((n,i)=>{n.category='part';n.summary=sentences[i%sentences.length];});
}

// A part's declarations: the worker's handler calls two functions and takes
// the job type they return.
if(options.has('symbols')){
  const worker=records.find(n=>n.id==='worker');
  worker.symbols=[{name:'processJob',kind:'function',key:true,text:'(job: Job)',href:'#worker.go-3',path:'worker.go'},{name:'Job',kind:'type',path:'job.go'},{name:'id',kind:'field',owner:2,text:': string',path:'job.go'},
    {name:'claim',kind:'function',text:'(): Job',path:'worker.go'},{name:'save',kind:'function',text:'(job: Job)',href:'#worker.go-30',path:'worker.go'},
    {name:'retryWithExponentialBackoffPolicy',kind:'function',text:'(job: Job, attempts: int)',path:'retry.go'}];
  worker.symbolCalls=[[0,3,'calls'],[0,4,'calls'],[3,1,'returns'],[0,1,'takes']];
  // The worker's arrows name the calls they carry, as the page's do.
  relations.find(r=>r.from==='queue'&&r.to==='worker').calls=[{label:'dispatch calls processJob',from:'#queue.go-12',to:'#worker.go-3'}];
  relations.find(r=>r.from==='worker'&&r.to==='save-jobs').calls=[{label:'save calls saveResult',from:'#worker.go-31',to:'#db.go-9'}];
}

// A service answering many commands, as Redis's server does: its input
// collection holds more inputs than read at the scale it opens at.
if(options.has('many-inputs')){
  const collection=records.find(n=>n.id==='backend-inputs');
  for(let i=0;i<40;i++){
    const id=`command-${i}`,owner=i%2?'routes':'worker';
    records.push({id,title:`command ${i}`,activation:'request',componentOwner:'backend',componentName:'Job processing service'});
    collection.children.push(id);inputOwner[id]=owner;
    relations.push({from:id,to:owner,label:'implemented in',operations:[id]});
  }
  areas.find(a=>a.id==='backend-inputs').nodes=collection.children;
}

// Each system calls its own "DNS resolver": two frames the page groups for
// display and gives their shared text once.
if(options.has('shared-destination')){
  for(const [id,caller] of [['dns-front','submission'],['dns-backend','worker']]){
    records.push({id,title:'DNS resolver',category:'external',branch:'communication',children:[`${id}-call`],displayGroup:'dns',displayGroupTitle:'DNS resolver'},
      {id:`${id}-call`,title:'gethostbyname',category:'external'});
    areas.push({id,nodes:[`${id}-call`]});relations.push({from:caller,to:`${id}-call`});
  }
}

// A chosen input carries its saved trace, the parts its code reaches in
// call-depth order, and its arrows name it.
if(options.has('input-path')){
  records.find(n=>n.id==='create').trace=['routes','auth','queue','worker'];
  for(const relation of relations)if([['routes','auth'],['routes','queue'],['queue','worker']].some(([from,to])=>relation.from===from&&relation.to===to))relation.operations=['create'];
}

// Only the host callbacks and prepared English labels are supplied here.
// Rendering, measurement, layout, zoom, hover and controls are production code.
window.rmT=(text,...args)=>text.replace(/\{(\d+)\}/g,(_,i)=>String(args[Number(i)]));
const map=document.querySelector('[data-map]'), stage=map.querySelector('.map-stage');
// The real report's header leaves 580px for the canvas at 1440×900, including
// the same 30px location bar above it. Set the host before production measures.
if(options.has('short-names'))map.querySelector('.map-workspace').style.height='610px';
map.dataset.cameraRevision='0';
map.addEventListener('repomap:viewport',()=>{map.dataset.cameraRevision=String(Number(map.dataset.cameraRevision)+1);});
const byID=new Map(records.map(n=>[n.id,n]));
const selected=id=>{
  const result=new Set([id]);
  for(const child of byID.get(id)?.children||[])for(const member of selected(child))result.add(member);
  return result;
};
const showReading=id=>{
  const item=byID.get(id);
  map.querySelector('[data-reading-title]').textContent=item?.title||'System map';
  map.querySelector('[data-reading-description]').textContent=item?.summary||'Explore the applications and their external connections.';
};
let operation='';
const flow=await window.rmCreateFlow(map,stage,records,relations,areas,inputOwner,{
  select(id,center){
    if(byID.get(id)?.activation)operation=id;
    flow.update({scope:byID.get(id)?.activation?'':id,operation,entry:operation,selected:selected(id)});
    showReading(id);
    if(center)flow.focus(id);
  },
  connection(){},
  emphasis(state){map.dataset.emphasis=state.mode;map.dataset.subject=state.subject;},
  // An arrow end clicked: the reading is on its frame, with the frame's
  // connections under it and that one open, as the report's column does.
  openConnection(id,key){
    flow.update({scope:id,selected:selected(id)});showReading(id);
    map.dataset.openedConnection=`${id} ${key}`;
    const holder=document.createElement('div');holder.dataset.readingConnections='';
    map.querySelector('[data-reading-connections]')?.remove();map.querySelector('.map-inspector-content').appendChild(holder);
    flow.mountConnections(holder,id,key);
  },
});
map.showWholeMap=()=>{operation='';flow.update({});showReading('');return flow.overview();};
map.captureViewport=()=>flow.capture();
// Choosing an input as the report does: the reading is on the input and the
// canvas is asked to show it.
map.focusNode=id=>flow.focus(id);
map.chooseInput=id=>{operation=id;flow.update({operation:id,entry:id,selected:selected(id)});showReading(id);flow.focus(id);};
map.visibleEdges=flow.layout.edges;
map.restoreReadingState=saved=>{
  flow.update({scope:saved.scope||'',selected:saved.scope?selected(saved.scope):new Set()});
  showReading(saved.scope);
  flow.restore(saved.viewport);
};
map.dataset.fixtureReady='true';
