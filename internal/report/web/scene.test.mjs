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
import {layoutLevels,cardWords,programWords,units,chipLines} from './levels.mjs';
import {sceneAt,emphasisOf,hitTest,chainOf,levelAfterZoom,connectionOf,bands,enterable,memberView} from './scene.mjs';
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
    if(enterable(model,node.id))levels.push(chainOf(model,node.id));
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
  // Everything the level draws or finds stands somewhere: every box, every
  // declaration's tile, every kind's mark on a closed Inputs card; and the
  // pointer finds something at each box's middle (owner, 2026-10-01: a
  // bucket's kinds, read as marks, had thrown on a click).
  const finite=r=>r&&[r.x,r.y,r.width,r.height].every(Number.isFinite);
  for(const node of scene.nodes){
    if(!finite(node.rect)){problems.push(['rects',`${where}: ${node.id} has no place`]);continue;}
    for(const mark of node.kinds||[])if(!finite(mark?.rect))problems.push(['rects',`${where}: a kind's mark on ${node.id} has no place`]);
    try{hitTest(scene,{x:node.rect.x+node.rect.width/2,y:node.rect.y+node.rect.height/2},zoom,()=>true);}
    catch(error){problems.push(['rects',`${where}: pointing at ${node.id} throws ${error.message}`]);}
  }
  for(const member of scene.members||[])if(!finite(member.rect))problems.push(['rects',`${where}: ${member.part}#${member.index} has no place`]);
  // A card's words stand whole or not at all, in its box (owner,
  // 2026-10-01: casdoor's card had read "Serves the Casd…"); a card is
  // laid out to hold its title.
  const bare=text=>String(text||'').replace(/\s+/g,'');
  // A chip's and a bucket's name stands whole in its box, never cut short
  // (owner, 2026-10-02).
  for(const node of scene.nodes.filter(node=>['chip','bucket'].includes(node.display)&&finite(node.rect))){
    const c=units.chip,bucket=node.display==='bucket',lines=chipLines(node.title,c.width-2*c.pad-(bucket?0:c.mark),measure);
    if((bucket?32:12)+lines*c.line>node.rect.height/node.text+.5)problems.push(['words',`${where}: ${node.id}'s name takes ${lines} lines in ${(node.rect.height/node.text).toFixed(0)}px`]);
  }
  for(const node of scene.nodes){
    if(!['card','area','program'].includes(node.display)||!finite(node.rect))continue;
    const item=model.nodes.get(node.id)?.item,program=node.display==='program';
    const words=program?programWords({...node,item},node.rect.width/node.text,measure):cardWords(node,item?.summary||'',measure,{room:node.enter?24:0});
    if(!words.title.length){problems.push(['words',`${where}: ${node.id}'s title does not stand in its card`]);continue;}
    const blocks=program?[[words.title,node.title],[words.role,item?.role],[words.purpose,item?.summary]]:[[words.title,node.title],[words.lines,item?.summary]];
    for(const [lines,text] of blocks)if(lines.length&&bare(lines.join(''))!==bare(text))problems.push(['words',`${where}: ${node.id} cuts "${lines.join(' ').slice(0,40)}"`]);
    const c=units.programCard,height=program?2*c.pad+words.title.length*c.line+(words.role.length?words.role.length*c.textLine+6:0)+(words.purpose.length?words.purpose.length*c.textLine+6:0)
      :28+words.title.length*21.25+(words.lines.length?6+words.lines.length*18:0);
    if(height>node.rect.height/node.text+1)problems.push(['words',`${where}: ${node.id}'s words take ${height.toFixed(0)} of ${(node.rect.height/node.text).toFixed(0)}`]);
  }
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
  // No arrow runs through a box it does not join.
  for(const edge of scene.edges){
    const ends=new Set([edge.from,edge.to]);
    for(const node of scene.nodes){
      if(node.band!==bands.box||ends.has(node.id))continue;
      const r=node.rect,m=px;
      const through=segments(edge.points).some(({a,b})=>Math.max(a.x,b.x)>r.x+m&&Math.min(a.x,b.x)<r.x+r.width-m&&Math.max(a.y,b.y)>r.y+m&&Math.min(a.y,b.y)<r.y+r.height-m);
      if(through){problems.push(['crosses',`${where} ${edge.id}: runs through ${node.id}`]);break;}
    }
  }
  // Titles at one level within ±10%: the boxes the level's own frame holds,
  // whose titles stand (one faded out below 10.5px is not drawn: on the
  // whole map a program's card may be drawn larger than the frames beside
  // it, levels.mjs programText).
  const own=scene.nodes.filter(node=>node.band===bands.box&&(scene.inner?model.parent(node.id)===scene.inner||scene.inner===scene.program&&model.parent(node.id)===scene.program:!model.parent(node.id))
    &&node.text*17*zoom>=10.5);
  if(own.length){
    const texts=own.map(node=>node.text),low=Math.min(...texts),high=Math.max(...texts);
    if(high>low*1.1)problems.push(['titles',`${where}: titles from ${low} to ${high}`]);
  }
  // Markers: at most three a side, one size at any zoom, beside their box.
  const perSide=new Map();
  for(const marker of scene.markers){const key=`${marker.box}:${marker.side}`;perSide.set(key,(perSide.get(key)||0)+1);}
  for(const [key,count] of perSide)if(count>markersPerSide)problems.push(['markers',`${where}: ${count} markers on ${key}`]);
  const runs=scene.edges.filter(edge=>edge.rest!==false).flatMap(edge=>segments(edge.points).map(s=>({...s,edge:edge.id})));
  for(let i=0;i<50;i++){
    const z=zoom*Math.pow(2,(i/49)*4-1.5);
    for(const item of project(scene,{x:0,y:0,zoom:z})){
      if(item.px<20||item.px>28)problems.push(['markers',`${where}: ${item.id} drawn ${item.px}px`]);
      if(item.type!=='marker'||item.row&&item.index>0)continue;
      const r=item.rect,half=item.size/2,edge=item.side==='in'?r.x:r.x+r.width;
      if(Math.abs(item.x+(item.side==='in'?half:-half)-edge)>1e-6*Math.max(1,r.width))problems.push(['markers',`${where}: ${item.id} does not touch its box`]);
      if(!item.row&&(item.y-half<r.y-1e-6||item.y+half>r.y+r.height+1e-6))problems.push(['markers',`${where}: ${item.id} stands beyond its box at zoom ${z}`]);
      // A marker stands on no arrow: the arrow under it would be lost
      // (owner: what is drawn is what is read).
      if(item.row)continue;
      const h=half-1/z,on=runs.find(({a,b})=>Math.max(a.x,b.x)>item.x-h&&Math.min(a.x,b.x)<item.x+h&&Math.max(a.y,b.y)>item.y-h&&Math.min(a.y,b.y)<item.y+h);
      if(on)problems.push(['markers',`${where}: ${item.id} stands on ${on.edge} at zoom ${z.toFixed(3)}`]);
    }
  }
  // Pointing changes classes and order only, dark arrows after grey; and
  // every arrow resolves to a connection and is found where it is drawn.
  // Every marker pointed at, and the boxes (up to sixty a level: an
  // Inputs frame entered holds a thousand inputs' names).
  const pointers=[null,...scene.nodes.slice(0,60).map(node=>({type:node.band===bands.frame?'frame':'box',id:node.id,node})),
    ...overlayAt(scene,zoom).filter(item=>item.type==='marker').map(item=>({type:'marker',id:item.id,box:item.box,item}))];
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
    // Pointed at anywhere along it, its ends included (a head touching its
    // box, the only part in sight when the camera stands close), an arrow
    // is found; a port at its port end is the port.
    const parts=segments(edge.points),total=parts.reduce((sum,{a,b})=>sum+Math.hypot(b.x-a.x,b.y-a.y),0);
    const along=f=>{let d=f*total;for(const {a,b} of parts){const l=Math.hypot(b.x-a.x,b.y-a.y);if(d<=l){const t=l?d/l:0;return {x:a.x+t*(b.x-a.x),y:a.y+t*(b.y-a.y)};}d-=l;}return edge.points.at(-1);};
    for(const f of [0,.2,.35,.5,.65,.8,1]){
      const hit=hitTest(scene,along(f),zoom,()=>true);
      if(hit?.type!=='edge'&&!(hit?.type==='port'&&edge.port))problems.push(['hits',`${where} ${edge.id}: ${f*100}% along it finds ${hit?.type||'nothing'} ${hit?.id||''}`]);
    }
  }
}

// The whole map at rest (owner, 2026-10-01): no arrow of code use between
// programs, and one arrow for every two programs an operation joins,
// however it reaches the other (its part, its input, or through an outside
// system the other serves).
function checkHome(name,model,geometry,problems){
  const scene=sceneAt(model,geometry,[],{});
  // At rest on the whole map every program and every outside system in
  // sight is named (owner, 2026-10-02): a program's title whole in its
  // card at eleven pixels or more, a chip's and a closed bucket's name, a
  // secondary word, at nine and a half.
  // The camera at rest shows them all wherever their names read there
  // (levels.mjs homeView); "Show whole map" shows the whole map.
  const rest=geometry.home,inSight=r=>{const l=r.x*rest.zoom+rest.x,t=r.y*rest.zoom+rest.y;return l>=-.5&&t>=-.5&&l+r.width*rest.zoom<=canvas.width+.5&&t+r.height*rest.zoom<=canvas.height+.5;};
  const named=scene.nodes.filter(node=>['program','chip','bucket'].includes(node.display));
  for(const node of named.filter(node=>inSight(node.rect))){
    const program=node.display==='program',px=(program?17:12)*node.text*rest.zoom;
    const whole=!program||programWords({...node,item:model.nodes.get(node.id)?.item},node.rect.width/node.text,measure).title.join('').replace(/\s+/g,'')===node.title.replace(/\s+/g,'');
    if(px<(program?11:9.5)||!whole)problems.push(['names',`${name}: ${node.id} named at ${px.toFixed(1)}px${whole?'':', cut'}`]);
  }
  if(rest.zoom!==geometry.whole.zoom&&named.filter(node=>inSight(node.rect)).length<1)problems.push(['names',`${name}: the camera at rest frames no name`]);
  const all=geometry.whole,b=geometry.bounds;
  if(b.x*all.zoom+all.x<-.5||b.y*all.zoom+all.y<-.5||(b.x+b.width)*all.zoom+all.x>canvas.width+.5||(b.y+b.height)*all.zoom+all.y>canvas.height+.5)problems.push(['names',`${name}: "Show whole map" does not show the whole map`]);
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
const rules={names:'every program (11px) and outside system (9.5px) in sight is named at rest on the whole map; Show whole map fits it all',words:'a card\'s title, role and description stand whole in it, or are left out',rects:'every box, tile and mark has a place, and pointing at a box throws nothing',ends:'each arrow runs from its own source\'s border to its own target\'s',heads:'a head points into its box',
  frame:'every end stands in the level\'s frame or on a port',shared:'no two arrows share more than 6px of one line',
  gaps:'lanes stand at least as far apart as in the approved Step 1 drawing',
  crosses:'no arrow runs through a box it does not join',
  uses:'the whole map draws no code-use arrow between programs at rest',
  pairs:'every two programs an operation joins have exactly one arrow at rest',titles:'titles at one level are within ±10%',
  markers:'markers keep 20–28px, at most three a side, beside their box, on no arrow',order:'dark arrows are drawn after grey ones',
  hover:'a marker lights only markers sharing its systems',connections:'every arrow resolves to its connection',hits:'an arrow is found all along it, its ends included'};
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
  // A declaration the reading names out of sight (owner, 2026-09-29): its
  // part across the canvas at the zoom its tiles read at, the tile in
  // sight, never deeper than the part's title fits; a part too dense for
  // that closed in its level, its card in sight.
  test(`${name}: a declaration named is shown across its part, or in its part's card when the part is too dense`,async()=>{
    const {model,geometry}=await prepare(name,page),pad=28;
    const parts=[...model.nodes.values()].filter(node=>node.kind==='part'&&geometry.grids.has(node.id)).slice(0,40);
    for(const part of parts){
      const index=Math.floor(part.item.symbols.length/2),view=memberView(model,geometry,part.id,index,canvas),r=geometry.boxes.get(part.id),z=view.camera.zoom;
      const screen=q=>({l:q.x*z+view.camera.x,t:q.y*z+view.camera.y,r:(q.x+q.width)*z+view.camera.x,b:(q.y+q.height)*z+view.camera.y});
      const drawn=geometry.grids.get(part.id),need=Math.max(11/13*drawn.grid.divisor*drawn.box.width/r.width,(geometry.enterZoom.get(part.id)||0)*1.02);
      if(view.dense){
        assert.deepEqual(view.level,chainOf(model,part.id).slice(0,-1),`${part.id}: closed in its level`);
        assert.ok((canvas.width-2*pad)/r.width<need,`${part.id}: closed though its tiles read across the canvas`);
        const s=screen(r);assert.ok(s.l>=-.5&&s.t>=-.5&&s.r<=canvas.width+.5&&s.b<=canvas.height+.5,`${part.id}: its card in sight`);
        continue;
      }
      assert.deepEqual(view.level,chainOf(model,part.id),`${part.id}: entered`);
      assert.ok(z>=need-1e-9,`${part.id}: its tiles read`);
      const s=screen(r);assert.ok(s.l>=pad-.5&&s.r<=canvas.width-pad+.5,`${part.id}: across the canvas, its title whole`);
      const tile=sceneAt(model,geometry,view.level,{}).members.find(member=>member.index===index);
      if(tile){const t=screen(tile.rect);assert.ok(t.t>=-.5&&t.b<=canvas.height+.5,`${part.id}: the declaration in sight`);}
    }
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
      // Hysteresis: zooming out leaves a level only below its exit zoom (a
      // zoom in toward a sibling may leave the level for it, scene.mjs).
      if(!action.zoomingIn&&level.length<state.level.length)for(const id of state.level.slice(level.length))assert.ok(zoom<geometry.exitZoom.get(id),`${id} left above its exit zoom`);
      if(level.length>state.level.length)for(const id of level.slice(state.level.length))assert.ok(zoom>=geometry.enterZoom.get(id),`${id} entered below its entry zoom`);
      state={level,zoom};
    }
  });
}
