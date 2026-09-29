// One geometry check of the canvas as it is drawn, at whatever level the
// camera stands: none of the owner's visual bugs of 2026-09-29 was caught by
// a check that read the DOM's text (empty call tiles, a comb of arrows along
// frame borders, "Commands" cut in half under "Inputs", a card's heading
// drawn over its rows, plaques floating beside their arrows). Each finding
// is {kind, element, level}:
//   overlap   two texts' visible boxes intersect;
//   clipped   a text is cut by a box that hides its overflow, with no
//             ellipsis and no title saying the rest;
//   escapes   a box stands out of the frame holding it;
//   crowded   an arrow runs within `near` px of a box it neither starts nor
//             ends at, or through it;
//   coincide  two arrows run on one line;
//   loose-end an arrow's end is not on the border of the box it ends at;
//   label     the canvas prints a kind label, a plaque or a lone number;
//   card      a card's heading and rows, or two rows, intersect;
//   small     a name on the canvas reads under 11 CSS pixels, a part's
//             description under 9;
//   head      an arrowhead is as large as the box it points into.
export const checks={near:3,font:11,head:10};

// Runs in the page. `level` names where the camera stands.
export function lintCanvas(level,near=3){
  const out=[],root=document.querySelector('.flow-root'),map=document.querySelector('[data-map]');
  if(!root||!map?.flowGeometry)return [{kind:'setup',element:'no canvas',level}];
  const canvas=root.getBoundingClientRect(),viewport=root.querySelector('.react-flow__viewport');
  const [,tx,ty,zoom]=/translate\(([-\d.e]+)px, ?([-\d.e]+)px\) scale\(([-\d.e]+)\)/.exec(viewport.style.transform)?.map(Number)||[];
  const screen=p=>({x:canvas.left+tx+p.x*zoom,y:canvas.top+ty+p.y*zoom});
  const name=el=>{const node=el.closest('.react-flow__node');return (node?`${node.dataset.id}:`:'')+((el.className?.baseVal??el.className)||el.tagName).toString().split(' ')[0]+(el.textContent?` "${el.textContent.trim().slice(0,40)}"`:'');};
  const shown=el=>{for(let at=el;at&&at!==root.parentElement;at=at.parentElement){const style=getComputedStyle(at);if(style.visibility==='hidden'||style.display==='none'||Number(style.opacity)===0)return false;}return true;};
  const inside=r=>r.right>canvas.left+1&&r.left<canvas.right-1&&r.bottom>canvas.top+1&&r.top<canvas.bottom-1;
  const meet=(a,b,e=1)=>Math.min(a.right,b.right)-Math.max(a.left,b.left)>e&&Math.min(a.bottom,b.bottom)-Math.max(a.top,b.top)>e;
  const cut=(r,c)=>({left:Math.max(r.left,c.left),top:Math.max(r.top,c.top),right:Math.min(r.right,c.right),bottom:Math.min(r.bottom,c.bottom)});
  // Every visible text on the canvas, as the boxes its lines are drawn in,
  // cut to the boxes that clip it.
  const texts=[];
  const walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);
  for(let node=walker.nextNode();node;node=walker.nextNode()){
    const el=node.parentElement;
    if(!node.textContent.trim()||!el||el.closest('.react-flow__attribution,.flow-floating-card,.flow-loading,svg')||!shown(el))continue;
    const range=document.createRange();range.selectNodeContents(node);
    let clip={left:canvas.left,top:canvas.top,right:canvas.right,bottom:canvas.bottom};
    for(let at=el;at&&at!==root;at=at.parentElement){const style=getComputedStyle(at);if(style.overflow!=='visible')clip=cut(clip,at.getBoundingClientRect());}
    const rects=[...range.getClientRects()].filter(r=>r.width>.5&&r.height>.5).map(r=>cut(r,clip)).filter(r=>r.right-r.left>.5&&r.bottom-r.top>.5);
    if(rects.length&&rects.some(inside))texts.push({el,text:node.textContent,rects});
  }
  // 1: no two texts' boxes intersect.
  for(let i=0;i<texts.length;i++)for(let j=i+1;j<texts.length;j++){
    const a=texts[i],b=texts[j];if(a.el===b.el||a.el.contains(b.el)||b.el.contains(a.el))continue;
    if(a.rects.some(r=>b.rects.some(q=>meet(r,q,1.5))))out.push({kind:'overlap',element:`${name(a.el)} × ${name(b.el)}`,level});
  }
  // 2: no text is cut, unless an ellipsis and a title say the rest.
  const cutBoxes=new Set();
  for(const text of texts)for(let at=text.el;at&&at!==root&&!at.matches(".react-flow__viewport,.react-flow__renderer,.react-flow");at=at.parentElement){
    const style=getComputedStyle(at);
    // A box that hides or scrolls its overflow must hold its text, or end
    // it with an ellipsis and a title: a half line under "Inputs" read as
    // a cut word, scrollable or not.
    if(style.overflow==='visible')continue;
    // Two pixels are a box's border, not a cut line.
    if(at.scrollHeight<=at.clientHeight+2.5&&at.scrollWidth<=at.clientWidth+2.5)continue;
    const ellipsis=style.textOverflow==='ellipsis'||!['none',''].includes(style.webkitLineClamp);
    if(!(ellipsis&&at.closest('[title]'))&&!cutBoxes.has(at)){cutBoxes.add(at);out.push({kind:'clipped',element:name(at),level});}
    break;
  }
  // 3: every placed box stands in the frame holding it.
  const geometry=map.flowGeometry(),placed=new Map(geometry.nodes.map(n=>[n.id,n]));
  const rect=n=>{const a=screen({x:n.x,y:n.y}),b=screen({x:n.x+n.width,y:n.y+n.height});return {left:a.x,top:a.y,right:b.x,bottom:b.y};};
  for(const n of geometry.nodes){
    const parent=placed.get(n.parentId);if(!parent||!n.shown)continue;
    const r=rect(n),p=rect(parent);
    if(r.left<p.left-1||r.top<p.top-1||r.right>p.right+1||r.bottom>p.bottom+1)out.push({kind:'escapes',element:`${n.id} out of ${parent.id}`,level});
  }
  // 3b: a box's own text stays inside it.
  for(const text of texts){
    const node=text.el.closest('.react-flow__node');if(!node)continue;
    const box=node.getBoundingClientRect();
    if(text.rects.some(r=>r.left<box.left-1||r.right>box.right+1||r.top<box.top-1||r.bottom>box.bottom+1))out.push({kind:'escapes',element:`text of ${name(text.el)}`,level});
  }
  // 4: arrows keep their room.
  const ancestors=id=>{const all=new Set();for(let at=id;at;at=placed.get(at)?.parentId)all.add(at);return all;};
  const within=(id,of)=>{for(let at=id;at;at=placed.get(at)?.parentId)if(at===of)return true;return false;};
  const shownBoxes=geometry.nodes.filter(n=>n.shown&&inside(rect(n)));
  const routes=[...root.querySelectorAll('g.flow-edge[data-edge-id]')].filter(shown).map(g=>{
    const d=g.querySelector('path:not(.flow-edge-hit):not(.flow-edge-casing)')?.getAttribute('d')||'';
    const points=[...d.matchAll(/([ML])\s*([-\d.e]+)[ ,]([-\d.e]+)/g)].map(m=>({move:m[1]==='M',...screen({x:+m[2],y:+m[3]})}));
    return {id:g.dataset.edgeId,ends:(g.dataset.edgeEnds||'').split(' ').filter(Boolean),points,hit:!!g.querySelector('[data-edge-hit]')};
  }).filter(route=>route.points.length>1);
  const segments=route=>route.points.slice(1).flatMap((p,i)=>p.move?[]:[[route.points[i],p]]);
  const distance=([a,b],r)=>{
    const x1=Math.min(a.x,b.x),x2=Math.max(a.x,b.x),y1=Math.min(a.y,b.y),y2=Math.max(a.y,b.y);
    return Math.hypot(Math.max(0,r.left-x2,x1-r.right),Math.max(0,r.top-y2,y1-r.bottom));
  };
  for(const route of routes){
    if(!route.hit)out.push({kind:'label',element:`arrow ${route.id} has no hit path`,level});
    const own=new Set(route.ends.flatMap(id=>[...ancestors(id)]));
    for(const box of shownBoxes){
      // Its ends, what holds them, and what they hold, are its own.
      if(own.has(box.id)||route.ends.some(id=>within(box.id,id)))continue;
      const r=rect(box);
      if(segments(route).some(segment=>inside({left:Math.min(segment[0].x,segment[1].x),right:Math.max(segment[0].x,segment[1].x)+1,top:Math.min(segment[0].y,segment[1].y),bottom:Math.max(segment[0].y,segment[1].y)+1})&&distance(segment,r)<near))
        out.push({kind:'crowded',element:`${route.id} (${route.ends.join('→')}) at ${box.id}`,level});
    }
    // 4b: an arrow ends on the border of the boxes at its ends.
    const [from,to]=route.ends.map(id=>placed.get(id));
    for(const [box,point] of [[from,route.points[0]],[to,route.points.at(-1)]]){
      if(!box||!box.shown)continue;
      const r=rect(box),edge=Math.min(Math.abs(point.x-r.left),Math.abs(point.x-r.right),Math.abs(point.y-r.top),Math.abs(point.y-r.bottom));
      const on=point.x>=r.left-1.5&&point.x<=r.right+1.5&&point.y>=r.top-1.5&&point.y<=r.bottom+1.5&&edge<=1.5;
      if(!on&&inside({left:point.x-1,right:point.x+1,top:point.y-1,bottom:point.y+1}))out.push({kind:'loose-end',element:`${route.id} at ${box.id}`,level});
    }
  }
  // 4c: no two arrows run on one line.
  for(let i=0;i<routes.length;i++)for(let j=i+1;j<routes.length;j++)for(const [a,b] of segments(routes[i]))for(const [c,d] of segments(routes[j])){
    const flat=(p,q)=>Math.abs(p.y-q.y)<.5,upright=(p,q)=>Math.abs(p.x-q.x)<.5;
    const span=(p,q,r,s,axis)=>Math.min(Math.max(p[axis],q[axis]),Math.max(r[axis],s[axis]))-Math.max(Math.min(p[axis],q[axis]),Math.min(r[axis],s[axis]));
    if(flat(a,b)&&flat(c,d)&&Math.abs(a.y-c.y)<1&&span(a,b,c,d,'x')>6||upright(a,b)&&upright(c,d)&&Math.abs(a.x-c.x)<1&&span(a,b,c,d,'y')>6){
      if(inside({left:Math.min(a.x,b.x),right:Math.max(a.x,b.x)+1,top:Math.min(a.y,b.y),bottom:Math.max(a.y,b.y)+1})){out.push({kind:'coincide',element:`${routes[i].id} and ${routes[j].id}`,level});break;}
    }
  }
  // 7: every text reads at 11px or more where the camera stands.
  const small=new Set();
  for(const text of texts){
    const el=text.el,size=parseFloat(getComputedStyle(el).fontSize)*(el.getBoundingClientRect().height/(el.offsetHeight||el.getBoundingClientRect().height||1));
    // A part's description is its secondary line: nine pixels.
    const least=el.closest('.flow-description')?9:11;
    if(size<least-.05&&!small.has(el)){small.add(el);out.push({kind:'small',element:`${name(el)} ${size.toFixed(1)}px`,level});}
  }
  // 8: an arrowhead (10px on screen) is smaller than the box it points into.
  for(const route of routes){
    const into=placed.get(route.ends.at(-1));if(!into||!into.shown)continue;
    const r=rect(into);
    if(Math.min(r.right-r.left,r.bottom-r.top)<=10&&inside(r))out.push({kind:'head',element:`${route.id} into ${into.id}`,level});
  }
  // 6: no kind label, plaque or lone number on the canvas.
  for(const el of root.querySelectorAll('.flow-kind,.flow-connection-label,.flow-boundary-label,.flow-inside-counts'))if(shown(el))out.push({kind:'label',element:name(el),level});
  for(const text of texts)if(text.text.split(/[\s·,]+/).some(word=>/^\d+$/.test(word)))out.push({kind:'label',element:`number in ${name(text.el)}`,level});
  return out;
}

// Runs in the page: the open card's heading and rows, and its rows among
// themselves, never intersect, at the top of its list and scrolled down.
export async function lintCard(level){
  const out=[],card=document.querySelector('.flow-floating-card .flow-connection-calls');
  if(!card)return out;
  const body=card.querySelector('.flow-card-body'),head=card.querySelector('.flow-card-head');
  const check=where=>{
    // A row is seen only where the list shows it: scrolled up, it passes
    // under the list's top edge, not through the heading.
    // A closed fold's contents are not drawn, only its summary.
    const drawn=el=>{const fold=el.closest('details:not([open])');return !fold||el.closest('summary')?.parentElement===fold;};
    const view=body.getBoundingClientRect(),rows=[...body.querySelectorAll('h4,h5,p,summary')].filter(drawn).map(el=>{const b=el.getBoundingClientRect();
      return {el,r:{left:b.left,right:b.right,top:Math.max(b.top,view.top),bottom:Math.min(b.bottom,view.bottom)}};})
      .filter(({r})=>r.bottom-r.top>1);
    const top=head?.getBoundingClientRect();
    for(const [i,{el,r}] of rows.entries()){
      if(top&&Math.min(r.bottom,top.bottom)-Math.max(r.top,top.top)>1)out.push({kind:'card',element:`heading × "${el.textContent.trim().slice(0,30)}" ${where}`,level});
      for(const {el:other,r:q} of rows.slice(i+1)){
        if(el.contains(other)||other.contains(el))continue;
        if(Math.min(r.bottom,q.bottom)-Math.max(r.top,q.top)>1&&Math.min(r.right,q.right)-Math.max(r.left,q.left)>1)
          out.push({kind:'card',element:`"${el.textContent.trim().slice(0,30)}" × "${other.textContent.trim().slice(0,30)}" ${where}`,level});
      }
    }
  };
  check('at the top');
  if(body.scrollHeight>body.clientHeight+1){body.scrollTop=Math.round(body.scrollHeight*.4);await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));check('scrolled');body.scrollTop=0;}
  return out;
}

// Drives a report through its levels and gathers every finding: the whole
// map; each program, Inputs and Outside entered by its zoom mark; up to
// `areas` areas of each program; and one part's declarations.
export async function lintReport(page,{repo='',areas=2,cards=3,programs=4}={}){
  const findings=[],map=page.locator('[data-map]');
  const settle=async()=>{
    let previous='',stable=0;
    for(let i=0;i<100&&stable<2;i++){const v=await map.evaluate(m=>JSON.stringify(m.captureViewport?.()));stable=v===previous?stable+1:0;previous=v;await page.waitForTimeout(100);}
    await page.waitForTimeout(150);
  };
  const lint=async(level,withCards=true)=>{
    findings.push(...(await page.evaluate(lintCanvas,`${repo} ${level}`)));
    if(!withCards)return;
    // The cards of a few arrows, opened as a reader's pointer opens them.
    const hits=await page.evaluate(()=>{
      const canvas=document.querySelector('.flow-root').getBoundingClientRect();
      return [...document.querySelectorAll('[data-edge-hit]')].map(path=>{
        const length=path.getTotalLength(),ctm=path.getScreenCTM(),p=path.getPointAtLength(length*.5);
        return {x:p.x*ctm.a+p.y*ctm.c+ctm.e,y:p.x*ctm.b+p.y*ctm.d+ctm.f};
      }).filter(p=>p.x>canvas.left+20&&p.x<canvas.right-20&&p.y>canvas.top+20&&p.y<canvas.bottom-20);
    });
    for(const point of hits.slice(0,cards)){
      await page.mouse.move(point.x,point.y,{steps:4});
      const opened=await page.locator('.flow-floating-card .flow-connection-calls').waitFor({timeout:1200}).then(()=>true,()=>false);
      if(opened)findings.push(...(await page.evaluate(lintCard,`${repo} ${level}`)));
      await page.keyboard.press('Escape');await page.mouse.move(2,2);await page.waitForTimeout(100);
    }
  };
  const whole=async()=>{await map.evaluate(m=>m.showWholeMap());await settle();};
  await whole();await lint('whole map');
  // The programs holding most areas, each with its Inputs, and every
  // Outside frame's program.
  const all=await page.evaluate(()=>{const nodes=document.querySelector('[data-map]').flowGeometry().nodes;
    return nodes.filter(n=>!n.parentId).map(n=>({id:n.id,branch:n.branch,areas:nodes.filter(child=>child.parentId===n.id&&child.branch==='area').length}));});
  const chosen=all.filter(root=>root.branch==='component').sort((a,b)=>b.areas-a.areas).slice(0,programs);
  const roots=all.filter(root=>chosen.includes(root)||root.branch==='inputs'&&chosen.some(program=>root.id.endsWith(program.id.replace('system-component-','-'))));
  for(const root of roots){
    const mark=page.locator(`[data-zoom-into="${root.id}"]`);
    if(!await mark.count())continue;
    await whole();await mark.first().click();await settle();await lint(root.id);
    if(root.branch!=='component')continue;
    const inner=await page.evaluate(id=>document.querySelector('[data-map]').flowGeometry().nodes.filter(n=>n.parentId===id&&n.branch==='area').map(n=>n.id),root.id);
    for(const area of inner.slice(0,areas)){
      await whole();await mark.first().click();await settle();
      const entry=page.locator(`[data-zoom-into="${area}"]`);
      if(!await entry.count())continue;
      await entry.first().click();await settle();await lint(area,false);
    }
  }
  // One part's declarations, entered by its magnifier.
  const part=page.locator('.flow-part-zoom >> visible=true');
  if(await part.count()&&await part.first().click({timeout:5000}).then(()=>true,()=>false)){await settle();await lint('a part');}
  return findings;
}

export function summary(findings){
  const counts={};for(const finding of findings)counts[finding.kind]=(counts[finding.kind]||0)+1;
  return `${findings.length} findings ${JSON.stringify(counts)}\n`+findings.slice(0,60).map(f=>`  ${f.kind.padEnd(9)} ${f.level} · ${f.element}`).join('\n');
}
