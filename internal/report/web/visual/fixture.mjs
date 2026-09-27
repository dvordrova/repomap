import * as prepared from './two-systems-five-externals.mjs';

const options=new URLSearchParams(location.search);
const {records,relations,areas,inputOwner}=options.has('single-target')?prepared.singleTargetInventory():options.has('short-names')?prepared.shortNamedInventory({matchedPeer:options.has('matched-peer')}):options.has('many-external')
  ?prepared.manyExternalInventory({inputs:!options.has('no-inputs')}):options.has('dense')?prepared.denseInventory():prepared;
if(options.has('many-external'))document.querySelector('h1').textContent='Two systems · seventeen external participants';
if(options.has('short-names'))document.querySelector('h1').textContent='Short component names · complete initial inventories';
if(options.has('single-target'))document.querySelector('h1').textContent='One system · twenty external participants';
if(options.has('single-part-area')){
  const group=records.find(n=>n.id==='requests');group.title='HTTP API surface';group.children=['routes'];
  records.find(n=>n.id==='routes').title=group.title;
  records.find(n=>n.id==='backend').children.push('auth');
  areas.find(a=>a.id===group.id).nodes=group.children;
}

// A part in no area stands beside the backend's areas.
if(options.has('loose-part')){
  records.find(n=>n.id==='backend').children.push('audit');
  areas.find(a=>a.id==='backend').nodes.push('audit');
  records.push({id:'audit',title:'Audit log',kind:'Part',summary:'Records every job decision for later review.'});
  relations.push({from:'worker',to:'audit'});
}

// Every part says what it is under its name. Redis's Replication and client
// sentences are among them: pre-broken at 228px they wrapped again in the
// card's 225px text column.
if(options.has('described')){
  const sentences=['Synchronizes master and replica data over network connections.','Provides an interactive terminal client for sending Redis commands.',
    'Implements Redis set commands and their operations.','Checks who may change a job and records every decision it takes for the audit trail, then reports refusals.'];
  records.filter(n=>n.kind==='Part'||n.kind==='Entrypoints').forEach((n,i)=>{n.category='part';n.summary=sentences[i%sentences.length];});
}

// A part's declarations: the worker's handler calls two functions and takes
// the job type they return.
if(options.has('symbols')){
  const worker=records.find(n=>n.id==='worker');
  worker.symbols=[{name:'processJob',kind:'function',key:true,text:'(job: Job)'},{name:'Job',kind:'type'},{name:'id',kind:'field',owner:2,text:': string'},
    {name:'claim',kind:'function',text:'(): Job'},{name:'save',kind:'function',text:'(job: Job)'}];
  worker.symbolCalls=[[0,3,'calls'],[0,4,'calls'],[3,1,'returns'],[0,1,'takes']];
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
