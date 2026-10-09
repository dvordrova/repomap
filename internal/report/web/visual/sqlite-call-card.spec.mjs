import {test,expect} from '@playwright/test';
import {invariantKit} from './invariants.mjs';

test.skip(!process.env.REPOMAP_REAL_RUN?.includes('sqlite'),'set REPOMAP_REAL_RUN to the saved SQLite run');
test('SQLite native callees open through brief and complete connection reading',async({page},testInfo)=>{
  test.setTimeout(240_000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  const names=['rtreeShadowName','rtreeBeginTransaction','rtreeSavepoint','rtreeDisconnect','rtreeBestIndex'];
  // Read original identities, containment and call ends already saved in the
  // ordinary page. Area IDs and the part holding R-tree may change per run.
  // No title heuristic or new grouping decides what an arrow represents.
  const cases=await page.evaluate(names=>{
    const map=document.querySelector('[data-map]');
    const records=[...map.querySelectorAll('[data-node][id]')].map(n=>({id:n.id,branch:n.dataset.branch,
      children:(n.dataset.children||'').split(/\s+/).filter(Boolean),symbols:rmPage.data(n,'symbols')||[]}));
    const byID=new Map(records.map(n=>[n.id,n])),parents=new Map();
    for(const node of records)for(const child of node.children)parents.set(child,node.id);
    const chain=id=>{const result=[];for(let at=id;at;at=parents.get(at))result.unshift(at);return result;};
    const edges=[...map.querySelectorAll('svg .map-edge')].map(e=>({from:e.dataset.from,to:e.dataset.to,
      calls:rmPage.data(e,'calls')||[]}));
    for(const program of records.filter(n=>n.branch==='component')){
      const parts=records.filter(n=>!n.branch&&chain(n.id).includes(program.id));
      const chosen=[];
      for(const name of names){
        const matches=parts.flatMap(part=>part.symbols.filter(s=>s.name===name).map(symbol=>({part,symbol})));
        if(matches.length!==1)break;
        const {part,symbol}=matches[0],key=repomapMembers.symbolKey(symbol);
        if(!key||symbol.href||symbol.open)break;
        const native=edges.find(edge=>edge.to===part.id&&edge.from!==part.id&&byID.has(edge.from)&&
          chain(edge.from).includes(program.id)&&edge.calls.some(call=>
            call.callee_name===name&&(call.callee||call.to)===key));
        if(!native)break;
        const from=chain(native.from),to=chain(part.id),common=[];
        for(let i=0;i<Math.min(from.length,to.length)&&from[i]===to[i];i++)common.push(from[i]);
        if(!common.length||common.length>=from.length||common.length>=to.length)break;
        chosen.push({name,key,part:part.id,path:symbol.path,chain:common,
          ends:[from[common.length],to[common.length]],caller:native.from});
      }
      if(chosen.length===names.length)return chosen;
    }
    return [];
  },names);
  expect(cases,'five exact native callees with saved cross-part calls and no upstream URL').toHaveLength(names.length);
  await page.evaluate(`window.__inv=(${invariantKit.toString()})()`);
  async function settle(){
    let previous='',stable=0;
    await expect.poll(async()=>{const view=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=view===previous?stable+1:0;previous=view;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
  }
  const receipts=[];
  try{for(const [index,original] of cases.entries()){
    const {name}=original;
    await page.getByRole('link',{name:'Home',exact:true}).click();await settle();
    for(const [depth,id] of original.chain.entries()){
      const button=page.locator(`[data-zoom-into="${id}"]`);
      if(depth===0){
        const zoom=await button.boundingBox(),aim={x:zoom.x+zoom.width/2,y:zoom.y+zoom.height/2};
        await page.mouse.move(aim.x,aim.y);
        await expect.poll(()=>page.evaluate(p=>getComputedStyle(document.elementFromPoint(p.x,p.y)).cursor,aim)).toBe('zoom-in');
        await page.mouse.click(aim.x,aim.y);
      }else await button.press('Enter');
      await expect(map).toHaveAttribute('data-scene-level',original.chain.slice(0,depth+1).join('/'));await settle();
    }
    const ids=await page.locator('g[data-edge-ends]').evaluateAll((edges,ends)=>edges.filter(e=>{
      const pair=e.dataset.edgeEnds.split(' ');return pair.length===2&&ends.every(id=>pair.includes(id));
    }).map(e=>e.dataset.edgeId),original.ends);
    expect(ids.length,`${name}: saved caller/callee ends have an ordinary drawn arrow`).toBeGreaterThan(0);
    let point=null;
    // A large original scope may require a pan. Use ordinary wheel input to
    // bring its saved arrow into sight; never change the camera directly.
    for(let pan=0;pan<=3&&!point;pan++){
      for(const id of ids){const at=await page.evaluate(id=>window.__inv.hitPoint(id),id);if(Number.isFinite(at.x)){point=at;break;}}
      if(point||pan===3)break;
      const at=await page.evaluate(id=>{
        const path=document.querySelector(`[data-edge-hit="${CSS.escape(id)}"]`),p=path.getPointAtLength(path.getTotalLength()/2),m=path.getScreenCTM();
        const canvas=document.querySelector('.flow-root').getBoundingClientRect();
        return {x:p.x*m.a+p.y*m.c+m.e,y:p.x*m.b+p.y*m.d+m.f,
          left:canvas.left,top:canvas.top,cx:canvas.left+canvas.width/2,cy:canvas.top+canvas.height/2};
      },ids[0]);
      await page.mouse.move(at.left+8,at.top+8);
      await page.mouse.wheel(2*(at.x-at.cx),2*(at.y-at.cy));await settle();
    }
    expect(point,`${name}: original caller/callee arrow has a visible native pointer hit`).not.toBeNull();
    await page.mouse.move(point.x,point.y);
    const card=page.locator('.flow-floating-card').first();
    await expect(card).toBeVisible();
    if(!await card.getByRole('button',{name,exact:true}).count()&&await card.getByRole('button',{name:'Calls the other way',exact:true}).count()){
      await card.getByRole('button',{name:'Calls the other way',exact:true}).click();
    }
    let row=card.getByRole('button',{name,exact:true}),interaction='brief',expanded=false;
    if(!await row.count()){
      // BriefRows advertises only its first twelve names. Opening a caller's
      // area does not narrow a cross-area route. The ordinary arrow click
      // opens the complete connection reading, where every original call
      // remains available; this is explicitly a different interaction.
      const close=card.getByRole('button',{name:'Close',exact:true});
      if(await close.count())await close.click();
      else{const canvas=await page.locator('.flow-root').boundingBox();await page.mouse.move(canvas.x+8,canvas.y+8);}
      await expect(card).not.toBeVisible();
      const into=await page.evaluate(({ids,target})=>{
        const map=document.querySelector('[data-map]'),canvas=document.querySelector('.flow-root').getBoundingClientRect();
        for(const id of ids){
          const edge=map.sceneState().scene.edges.find(e=>e.id===id),path=document.querySelector(`[data-edge-hit="${CSS.escape(id)}"]`);
          if(!edge||!path)continue;
          const length=path.getTotalLength(),first=path.getPointAtLength(0),last=path.getPointAtLength(length),m=path.getScreenCTM();
          const toward=edge.to===target?last:first,away=edge.to===target?first:last;
          for(const f of [.8,.2,.9,.1,.7,.3,.6,.4]){
            const p=path.getPointAtLength(length*f);
            if(Math.hypot(p.x-toward.x,p.y-toward.y)>=Math.hypot(p.x-away.x,p.y-away.y))continue;
            const x=p.x*m.a+p.y*m.c+m.e,y=p.x*m.b+p.y*m.d+m.f;
            if(x<canvas.left+6||x>canvas.right-6||y<canvas.top+6||y>canvas.bottom-6)continue;
            const hit=map.sceneHitAt(x,y),surface=document.elementFromPoint(x,y);
            if(hit?.type==='edge'&&hit.id===id&&surface&&map.contains(surface)&&!surface.closest('.flow-floating-card'))return {x,y};
          }
        }
        return null;
      },{ids,target:original.ends[1]});
      expect(into,`${name}: a real arrow hit reads the original callee's direction`).not.toBeNull();
      await page.mouse.click(into.x,into.y);
      const complete=page.locator('.map-inspector .map-frame-connections');
      await expect(complete).toBeVisible();
      // Ordinary CallRows uses native buttons, not the synthetic fixture's
      // data-chosen-source or a data-decl-key mirror. This program's native
      // symbol inventory already proved the name unique; the canonical key
      // and owner are checked after the actual callback below.
      row=complete.locator(`[data-call-group="${original.caller}"]`).getByRole('button',{name,exact:true}).first();
      if(!await row.isVisible()){
        // Possible callees are ordinary closed disclosures inside this
        // complete reading. Reveal them through its existing reader control;
        // neither missing brief names nor a closed fold means lost evidence.
        await page.locator('.map-inspector').getByRole('button',{name:'Expand all',exact:true}).click();
        expanded=true;
      }
      interaction='complete_connection';
    }
    await expect(row).toBeVisible();
    if(index===0)await page.screenshot({path:testInfo.outputPath('sqlite-call-names.png')});
    receipts.push({name,key:original.key,owner:original.part,caller:original.caller,chain:original.chain,
      interaction,expanded,input:index%2?'Enter':'click'});
    if(index%2)await row.press('Enter');else await row.click();
    await expect.poll(()=>map.evaluate(map=>map.explorerMember?.key||'')).toBe(original.key);
    await expect.poll(()=>map.evaluate(map=>map.explorerMember?.owner||'')).toBe(original.part);
    await expect(page.locator('.map-inspector .map-decl-name .map-decl-code')).toHaveText(name);
    await expect.poll(()=>page.evaluate(()=>location.hash)).toBe(`#${original.part}`);
    await expect(page.locator('.map-inspector')).toContainText(original.path);
    if(index===4)await page.screenshot({path:testInfo.outputPath('sqlite-exact-declaration.png')});
  }}finally{await testInfo.attach('original-callee-interactions',{body:JSON.stringify(receipts,null,2),contentType:'application/json'});}
  expect(errors).toEqual([]);
});
