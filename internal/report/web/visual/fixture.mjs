import {records,relations,areas,inputOwner} from './two-systems-five-externals.mjs';

// Only the host callbacks and prepared English labels are supplied here.
// Rendering, measurement, layout, zoom, hover and controls are production code.
window.rmT=(text,...args)=>text.replace(/\{(\d+)\}/g,(_,i)=>String(args[Number(i)]));
const map=document.querySelector('[data-map]'), stage=map.querySelector('.map-stage');
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
const flow=await window.rmCreateFlow(map,stage,records,relations,areas,inputOwner,{
  select(id,center){
    flow.update({scope:id,selected:selected(id)});
    showReading(id);
    if(center)flow.focus(id);
  },
  connection(){},
});
map.showWholeMap=()=>{flow.update({});showReading('');return flow.overview();};
map.dataset.fixtureReady='true';
