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
map.visibleEdges=flow.layout.edges;
map.restoreReadingState=saved=>{
  flow.update({scope:saved.scope||'',selected:saved.scope?selected(saved.scope):new Set()});
  showReading(saved.scope);
  flow.restore(saved.viewport);
};
map.dataset.fixtureReady='true';
