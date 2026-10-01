// The scene's invariants (PLAN, "Tests"), on every level of seeded
// synthetic graphs and of the real reports named by REPOMAP_SCENE_PAGES
// (scene-pages.mjs): an arrow is one polyline from its own source's border
// to its own target's, its head pointing in; no two arrows run on one line;
// every end stays in the level's frame or on a port; markers keep their
// screen size and stand beside their box; titles at a level are one size;
// pointing changes only classes and order, dark arrows over grey; every
// arrow resolves to its connection; a pan changes no level.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {buildModel,markersPerSide} from './model.mjs';
import {layoutLevels} from './levels.mjs';
import {sceneAt,emphasisOf,hitTest,chainOf,levelAfterZoom,connectionOf,bands} from './scene.mjs';
import {overlayAt,project,mark} from './overlay.mjs';
import {zoomAction} from './store.mjs';
import {syntheticPages,realPages,measure} from './scene-pages.mjs';

const canvas={width:1056,height:632};
// The smallest room between two lanes in the owner-approved Step 1 drawing.
const laneGap=7.5;
const pages=[...syntheticPages,...realPages()];
const prepared=new Map();
async function prepare(name,page){
  if(!prepared.has(name))prepared.set(name,(async()=>{
    const model=buildModel(page,{measure});
    const geometry=await layoutLevels(model,{...canvas,measure});
    return {model,geometry};
  })());
  return prepared.get(name);
}
// Every level: the whole map, each program, area, Inputs frame and bucket.
function levelsOf(model){
  const levels=[[]];
  for(const node of model.nodes.values())
    if(['program','area','inputs','bucket'].includes(node.kind)||node.kind==='part'&&node.item?.symbols?.length)levels.push(chainOf(model,node.id));
  return levels;
}
// The camera a level is shown at: the whole map's, or entered.
function zoomOf(geometry,scene){
  if(!scene.level.length)return geometry.home.zoom;
  const f=scene.focus,fit=Math.min((canvas.width-56)/f.width,(canvas.height-56)/f.height);
  return Math.max(fit,(geometry.enterZoom.get(scene.inner)||0)*1.02);
}
const eps=1e-3;
// The side of rectangle `r` point `p` stands on, or ''.
function sideOf(p,r,tolerance){
  const within=(v,a,b)=>v>=a-tolerance&&v<=b+tolerance;
  if(Math.abs(p.x-r.x)<=tolerance&&within(p.y,r.y,r.y+r.height))return 'left';
  if(Math.abs(p.x-r.x-r.width)<=tolerance&&within(p.y,r.y,r.y+r.height))return 'right';
  if(Math.abs(p.y-r.y)<=tolerance&&within(p.x,r.x,r.x+r.width))return 'top';
  if(Math.abs(p.y-r.y-r.height)<=tolerance&&within(p.x,r.x,r.x+r.width))return 'bottom';
  return '';
}
const inward={left:[1,0],right:[-1,0],top:[0,1],bottom:[0,-1]};
function segments(points){
  return points.slice(1).map((b,i)=>({a:points[i],b,vertical:Math.abs(points[i].x-b.x)<eps,horizontal:Math.abs(points[i].y-b.y)<eps}));
}

function checkScene(name,model,geometry,scene,problems){
  const zoom=zoomOf(geometry,scene),px=1/zoom,where=`${name} [${scene.key||'map'}]`;
  const rect=new Map(scene.nodes.map(node=>[node.id,node.rect]));
  const portAt=new Map(scene.ports.map(port=>[port.id,port.point]));
  const frame=scene.frame;
  for(const edge of scene.edges){
    // One polyline from its own source's border to its own target's.
    const ends=[[edge.from,edge.points[0],edge.points[1],edge.heads.start],[edge.to,edge.points.at(-1),edge.points.at(-2),edge.heads.end]];
    for(const [id,end,before,head] of ends){
      if(portAt.has(id)){
        if(Math.hypot(end.x-portAt.get(id).x,end.y-portAt.get(id).y)>px)problems.push(['ends',`${where} ${edge.id}: end off its port ${id}`]);
        continue;
      }
      const r=rect.get(id);
      if(!r){problems.push(['ends',`${where} ${edge.id}: end ${id} is no box drawn`]);continue;}
      const side=sideOf(end,r,Math.max(px*.5,r.width*1e-6));
      if(!side){problems.push(['ends',`${where} ${edge.id}: end off ${id}'s border`]);continue;}
      if(head){
        const [dx,dy]=inward[side],sx=end.x-before.x,sy=end.y-before.y;
        if(sx*dx+sy*dy<=0||Math.abs(sx*dy)+Math.abs(sy*dx)>eps*Math.hypot(sx,sy)+eps)problems.push(['heads',`${where} ${edge.id}: head into ${id} does not point in`]);
      }
    }
    // Every end in the level's frame.
    for(const p of [edge.points[0],edge.points.at(-1)])
      if(p.x<frame.x-px||p.y<frame.y-px||p.x>frame.x+frame.width+px||p.y>frame.y+frame.height+px)problems.push(['frame',`${where} ${edge.id}: end beyond the level's frame`]);
  }
  // No two arrows share more than 6 screen pixels of one line.
  const all=scene.edges.flatMap(edge=>segments(edge.points).map(segment=>({...segment,edge:edge.id,container:edge.container})));
  for(let i=0;i<all.length;i++)for(let j=i+1;j<all.length;j++){
    const s=all[i],t=all[j];if(s.edge===t.edge)continue;
    let overlap=0;
    if(s.vertical&&t.vertical&&Math.abs(s.a.x-t.a.x)<.5*px)overlap=Math.min(Math.max(s.a.y,s.b.y),Math.max(t.a.y,t.b.y))-Math.max(Math.min(s.a.y,s.b.y),Math.min(t.a.y,t.b.y));
    else if(s.horizontal&&t.horizontal&&Math.abs(s.a.y-t.a.y)<.5*px)overlap=Math.min(Math.max(s.a.x,s.b.x),Math.max(t.a.x,t.b.x))-Math.max(Math.min(s.a.x,s.b.x),Math.min(t.a.x,t.b.x));
    if(overlap*zoom>6)problems.push(['shared',`${where}: ${s.edge} and ${t.edge} share ${Math.round(overlap*zoom)}px`]);
  }
  // Lanes keep at least the room between them that the owner-approved
  // Step 1 drawing (daf1231e) kept at an area's entry zoom: 7.5 screen
  // pixels, its smallest (redis's Core server infrastructure, measured on
  // the daf1231e layout code at a 1056x632 canvas). Lanes of one level's
  // own graph, with no box between them.
  const boxes=scene.nodes.filter(node=>node.band===bands.box).map(node=>node.rect);
  const drawnAll=all.filter(s=>scene.edges.find(edge=>edge.id===s.edge).rest!==false);
  let narrowest=Infinity,between='';
  for(let i=0;i<drawnAll.length;i++)for(let j=i+1;j<drawnAll.length;j++){
    const s=drawnAll[i],t=drawnAll[j];if(s.edge===t.edge||s.container!==t.container)continue;
    for(const [axis,other,size] of [['x','y','width'],['y','x','height']]){
      if(!(axis==='x'?s.vertical&&t.vertical:s.horizontal&&t.horizontal))continue;
      const lo=Math.max(Math.min(s.a[other],s.b[other]),Math.min(t.a[other],t.b[other])),hi=Math.min(Math.max(s.a[other],s.b[other]),Math.max(t.a[other],t.b[other]));
      if(hi-lo<=px)continue;
      const d=Math.abs(s.a[axis]-t.a[axis]);if(d<.5*px)continue;
      const a=Math.min(s.a[axis],t.a[axis]),b=Math.max(s.a[axis],t.a[axis]),mid=(lo+hi)/2;
      if(boxes.some(r=>r[axis]>a&&r[axis]+r[size]<b&&mid>=r[other]&&mid<=r[other]+r[other==='x'?'width':'height']))continue;
      if(d<narrowest){narrowest=d;between=`${s.edge} and ${t.edge}`;}
    }
  }
  if(narrowest*zoom<laneGap)problems.push(['gaps',`${where}: lanes ${(narrowest*zoom).toFixed(1)}px apart (${between})`]);
  // Titles at one level within ±10%: the boxes the level's own frame holds.
  const own=scene.nodes.filter(node=>node.band===bands.box&&(scene.inner?model.parent(node.id)===scene.inner||scene.inner===scene.program&&model.parent(node.id)===scene.program:!model.parent(node.id)));
  if(own.length){
    const texts=own.map(node=>node.text),low=Math.min(...texts),high=Math.max(...texts);
    if(high>low*1.1)problems.push(['titles',`${where}: titles from ${low} to ${high}`]);
  }
  // Markers: at most three a side, one size at any zoom, beside their box.
  const perSide=new Map();
  for(const marker of scene.markers){const key=`${marker.box}:${marker.side}`;perSide.set(key,(perSide.get(key)||0)+1);}
  for(const [key,count] of perSide)if(count>markersPerSide)problems.push(['markers',`${where}: ${count} markers on ${key}`]);
  for(let i=0;i<50;i++){
    const z=zoom*Math.pow(2,(i/49)*4-1.5);
    for(const item of project(scene,{x:0,y:0,zoom:z})){
      if(item.px<20||item.px>28)problems.push(['markers',`${where}: ${item.id} drawn ${item.px}px`]);
      if(item.type!=='marker'||item.row&&item.index>0)continue;
      const r=item.rect,half=item.size/2,edge=item.side==='in'?r.x:r.x+r.width;
      if(Math.abs(item.x+(item.side==='in'?half:-half)-edge)>1e-6*Math.max(1,r.width))problems.push(['markers',`${where}: ${item.id} does not touch its box`]);
      if(!item.row&&(item.y-half<r.y-1e-6||item.y+half>r.y+r.height+1e-6))problems.push(['markers',`${where}: ${item.id} stands beyond its box at zoom ${z}`]);
    }
  }
  // Pointing changes classes and order only, dark arrows after grey; and
  // every arrow resolves to a connection and is found where it is drawn.
  const pointers=[null,...scene.nodes.slice(0,25).map(node=>({type:node.band===bands.frame?'frame':'box',id:node.id,node})),
    ...overlayAt(scene,zoom).filter(item=>item.type==='marker').slice(0,25).map(item=>({type:'marker',id:item.id,box:item.box,item}))];
  for(const pointer of pointers){
    const emphasis=emphasisOf(scene,model,pointer,{});
    assert.deepEqual([...emphasis.nodeClass.keys()].sort(),scene.nodes.map(node=>node.id).sort(),`${where}: pointing at ${pointer?.id} keeps the boxes`);
    assert.deepEqual([...emphasis.order].sort(),scene.edges.map(edge=>edge.id).sort(),`${where}: pointing at ${pointer?.id} keeps the arrows`);
    const firstDark=emphasis.order.findIndex(id=>emphasis.edgeState.get(id).on);
    if(firstDark>=0&&emphasis.order.slice(firstDark).some(id=>!emphasis.edgeState.get(id).on))problems.push(['order',`${where}: a grey arrow drawn over a dark one`]);
    if(pointer?.type==='marker')for(const id of emphasis.lit){
      const lit=scene.markers.find(marker=>marker.id===id);
      const shares=pointer.item.side==='out'?lit.systems.some(s=>pointer.item.systems.includes(s)):lit.members.some(m=>pointer.item.members.includes(m));
      if(!shares)problems.push(['hover',`${where}: ${id} lit by ${pointer.id} without a shared system`]);
    }
  }
  for(const edge of scene.edges){
    for(const backward of [false,true]){
      if(!(backward?edge.heads.start:edge.heads.end))continue;
      if(!connectionOf(model,edge,backward))problems.push(['connections',`${where} ${edge.id}: no connection ${backward?'back':'on'}`]);
    }
    const longest=segments(edge.points).sort((a,b)=>Math.hypot(b.b.x-b.a.x,b.b.y-b.a.y)-Math.hypot(a.b.x-a.a.x,a.b.y-a.a.y))[0];
    const middle={x:(longest.a.x+longest.b.x)/2,y:(longest.a.y+longest.b.y)/2};
    const hit=hitTest(scene,middle,zoom,()=>true);
    if(hit?.type!=='box'&&hit?.type!=='marker'&&hit?.type!=='port'&&!(hit?.type==='edge'))problems.push(['hits',`${where} ${edge.id}: its middle finds ${hit?.type||'nothing'}`]);
  }
}

// The whole map at rest (owner, 2026-10-01): no arrow of code use between
// programs, and one arrow for every two programs an operation joins,
// however it reaches the other (its part, its input, or through an outside
// system the other serves).
function checkHome(name,model,geometry,problems){
  const scene=sceneAt(model,geometry,[],{});
  const program=id=>model.nodes.get(id)?.kind==='program';
  const atRest=scene.edges.filter(edge=>edge.rest!==false&&program(edge.from)&&program(edge.to));
  for(const edge of atRest){
    const relations=edge.edgeIDs.flatMap(id=>model.edges.find(e=>e.id===id)?.relations||[]);
    const folded=edge.edgeIDs.some(id=>{const e=model.edges.find(x=>x.id===id);return model.nodes.get(e?.from)?.kind==='system';});
    if(!folded&&relations.every(relation=>relation.scope==='structure'))problems.push(['uses',`${name}: ${edge.from} and ${edge.to} joined by code use at rest`]);
  }
  const wanted=new Set();
  for(const edge of model.edges){
    if(!edge.relations.some(relation=>relation.scope==='operation'))continue;
    const ends=model.nodes.get(edge.from)?.kind==='system'?[...(model.callingPrograms.get(edge.from)||[])]:[model.programOf(edge.from)];
    const to=model.programOf(edge.to);
    for(const from of ends)if(program(from)&&program(to)&&from!==to)wanted.add([from,to].sort().join('|'));
  }
  for(const key of wanted){
    const [a,b]=key.split('|'),count=atRest.filter(edge=>[edge.from,edge.to].sort().join('|')===key).length;
    if(count!==1)problems.push(['pairs',`${name}: ${count} arrows at rest between ${a} and ${b}`]);
  }
}

// Each rule's problems on every level of a page, with a box chosen too: a
// chosen box draws its quiet arrows, and the rules hold for them.
const rules={ends:'each arrow runs from its own source\'s border to its own target\'s',heads:'a head points into its box',
  frame:'every end stands in the level\'s frame or on a port',shared:'no two arrows share more than 6px of one line',
  gaps:'lanes stand at least as far apart as in the approved Step 1 drawing',
  uses:'the whole map draws no code-use arrow between programs at rest',
  pairs:'every two programs an operation joins have exactly one arrow at rest',titles:'titles at one level are within ±10%',
  markers:'markers keep 20–28px, at most three a side, beside their box',order:'dark arrows are drawn after grey ones',
  hover:'a marker lights only markers sharing its systems',connections:'every arrow resolves to its connection',hits:'an arrow is found where it is drawn'};
const problemsOf=new Map();
async function problemsFor(name,page){
  if(!problemsOf.has(name))problemsOf.set(name,(async()=>{
    const {model,geometry}=await prepare(name,page),problems=[];
    checkHome(name,model,geometry,problems);
    for(const level of levelsOf(model)){
      const scene=sceneAt(model,geometry,level,{});
      checkScene(name,model,geometry,scene,problems);
      const chosen=scene.nodes.find(node=>node.band===bands.box);
      if(chosen)checkScene(`${name} (${chosen.id} chosen)`,model,geometry,sceneAt(model,geometry,level,{scope:chosen.id}),problems);
    }
    return problems;
  })());
  return problemsOf.get(name);
}
for(const [name,page] of pages){
  for(const [rule,says] of Object.entries(rules))test(`${name}: ${says}`,async()=>{
    const found=(await problemsFor(name,page)).filter(([at])=>at===rule).map(([,text])=>text);
    assert.deepEqual(found.slice(0,12),[],`${found.length} problems`);
  });
  test(`${name}: a pan changes no level, a zoom crosses with hysteresis`,async()=>{
    const {model,geometry}=await prepare(name,page);
    let state={level:[],zoom:geometry.home.zoom};
    const rand=(()=>{let a=7;return ()=>{a=(a*16807)%2147483647;return a/2147483647;};})();
    for(let step=0;step<200;step++){
      const scene=sceneAt(model,geometry,state.level,{});
      const pan=rand()<.5;
      const zoom=pan?state.zoom:state.zoom*Math.pow(2,(rand()-.45)*1.5);
      const aim={x:geometry.bounds.x+rand()*geometry.bounds.width,y:geometry.bounds.y+rand()*geometry.bounds.height};
      const action=zoomAction(state.zoom,{zoom},aim);
      if(pan){assert.equal(action,null,'a pan asks for no level');continue;}
      const level=action?levelAfterZoom(model,geometry,scene,state.level,action.zoom,action.aim,action.zoomingIn):state.level;
      // Hysteresis: zooming out leaves a level only below its exit zoom.
      if(level.length<state.level.length)for(const id of state.level.slice(level.length))assert.ok(zoom<geometry.exitZoom.get(id),`${id} left above its exit zoom`);
      if(level.length>state.level.length)for(const id of level.slice(state.level.length))assert.ok(zoom>=geometry.enterZoom.get(id),`${id} entered below its entry zoom`);
      state={level,zoom};
    }
  });
}
