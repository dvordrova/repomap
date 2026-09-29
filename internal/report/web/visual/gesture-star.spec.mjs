import {test,expect} from '@playwright/test';
import {singleTargetInventory} from './two-systems-five-externals.mjs';

for(const [query,id] of [['dense','front'],['single-target','backend']]){
  test(`${id} reveals its diagram when pinch brings the frame near viewport size (${query})`,async({page},testInfo)=>{
    test.setTimeout(60000);
    await page.goto(`/?${query}`);
    const map=page.locator('[data-map]');
    await expect(map).toHaveAttribute('data-fixture-ready','true',{timeout:30000});
    const canvas=await page.locator('.flow-root').boundingBox();
    const frame=page.locator(`.react-flow__node[data-id="${id}"]`),box=await frame.boundingBox();
    const initial=await map.evaluate(map=>map.captureViewport());
    // Centre the same frame at overview scale, then use only real pinch events.
    await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...initial,fit:false,
      x:initial.x+canvas.x+canvas.width/2-box.x-box.width/2,
      y:initial.y+canvas.y+canvas.height/2-box.y-box.height/2});
    const world=await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]));
    await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
    await testInfo.attach('journey-01 — Aim at the centre of the target',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
    for(let step=0;step<32;step++){
      const b=await frame.boundingBox();
      if(step>0&&Math.max(b.width/canvas.width,b.height/canvas.height)>=.85)break;
      const zoom=await map.evaluate(map=>map.captureViewport().zoom);
      await page.keyboard.down('Control');try{await page.mouse.wheel(0,-4);}finally{await page.keyboard.up('Control');}
      await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
    }
    await testInfo.attach('journey-02 — The target nearly fills the frame · its diagram must be visible',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
    await expect(page.locator(`[data-component-overview="${id}"]`)).toHaveCount(0);
    const count=await page.evaluate(root=>{
      const canvas=document.querySelector('.flow-root').getBoundingClientRect();
      const frame=document.querySelector(`.react-flow__node[data-id="${root}"]`).getBoundingClientRect();
      return [...document.querySelectorAll('[data-summary-area] .flow-part,.react-flow__node .flow-part')].filter(el=>{
        const box=el.getBoundingClientRect();
        return getComputedStyle(el).visibility!=='hidden'&&box.left>=Math.max(canvas.left,frame.left)&&box.right<=Math.min(canvas.right,frame.right)&&box.top>=Math.max(canvas.top,frame.top)&&box.bottom<=Math.min(canvas.bottom,frame.bottom);
      }).length;
    },id);
    expect(count,'At least one complete inner card is on screen, not merely present in the DOM').toBeGreaterThan(0);
    const opened=await map.evaluate(map=>map.captureViewport());
    await map.evaluate(map=>map.showWholeMap());
    await expect(page.locator(`[data-component-overview="${id}"]`)).toHaveCount(1);
    await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),opened);
    await expect(page.locator(`[data-component-overview="${id}"]`)).toHaveCount(0);
    expect(await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]))).toEqual(world);
  });
}

test('pinch over a scrolling target inventory zooms the map',async({page},testInfo)=>{
  await page.goto('/?dense');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  const list=page.locator('[data-component-overview="front"] .flow-component-areas');
  await expect(list).toBeVisible();
  expect(await list.evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
  const listElement=await list.elementHandle();
  const before=await map.evaluate(map=>map.captureViewport());
  const box=await list.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+Math.min(25,box.height/2));
  await testInfo.attach('journey-01 — Aim over the scrolling target inventory',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await page.keyboard.down('Control');
  try{await page.mouse.wheel(0,-30);}finally{await page.keyboard.up('Control');}
  await expect.poll(async()=>(await map.evaluate(map=>map.captureViewport())).zoom).toBeGreaterThan(before.zoom);
  expect(await listElement.evaluate(el=>el.scrollTop)).toBe(0);
  await testInfo.attach('journey-02 — Pinch zooms without scrolling the inventory',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});

test('one target and twenty external systems have readable overview cards',async({page},testInfo)=>{
  test.setTimeout(90000);
  const started=Date.now();
  await page.goto('/?single-target');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true',{timeout:30000});
  await testInfo.attach('Initial placement timing',{body:JSON.stringify({readyMs:Date.now()-started}),contentType:'application/json'});
  await expect(page.locator('[data-component-overview]')).toHaveCount(21);
  await testInfo.attach('journey-01 — One target · twenty external systems · initial overview',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  for(const item of singleTargetInventory().records.filter(n=>n.children&&n.branch!=='area')){
    const frame=await page.locator(`.react-flow__node[data-id="${item.id}"]`).boundingBox();
    const title=page.locator(`[data-component-overview="${item.id}"] strong`);
    await expect(title).toHaveText(item.title);
    const rects=await title.evaluate(el=>{const range=document.createRange();range.selectNodeContents(el);return [...range.getClientRects()].map(r=>({left:r.left,right:r.right,top:r.top,bottom:r.bottom}));});
    for(const rect of rects){
      expect(rect.left,`${item.title}: left edge`).toBeGreaterThanOrEqual(frame.x);
      expect(rect.right,`${item.title}: right edge`).toBeLessThanOrEqual(frame.x+frame.width+1);
      expect(rect.top,`${item.title}: top edge`).toBeGreaterThanOrEqual(frame.y);
      expect(rect.bottom,`${item.title}: bottom edge`).toBeLessThanOrEqual(frame.y+frame.height+1);
    }
  }
  const crossings=await map.evaluate((map,ids)=>{
    const m=new DOMMatrixReadOnly(getComputedStyle(map.querySelector('.react-flow__viewport')).transform);
    const host=map.querySelector('.flow-root').getBoundingClientRect();
    const boxes=ids.map(id=>{const b=map.querySelector(`.react-flow__node[data-id="${id}"]`).getBoundingClientRect();return {id,x:(b.x-host.x-m.e)/m.a,y:(b.y-host.y-m.f)/m.d,w:b.width/m.a,h:b.height/m.d};});
    const hits=[];
    for(const edge of map.visibleEdges.filter(e=>e.outerSegments))for(const points of edge.outerSegments)for(let i=1;i<points.length;i++){
      const a=points[i-1],b=points[i];
      for(const box of boxes){
        const vertical=Math.abs(a.x-b.x)<.01;
        const hit=vertical?a.x>box.x+.1&&a.x<box.x+box.w-.1&&Math.max(a.y,b.y)>box.y+.1&&Math.min(a.y,b.y)<box.y+box.h-.1:
          a.y>box.y+.1&&a.y<box.y+box.h-.1&&Math.max(a.x,b.x)>box.x+.1&&Math.min(a.x,b.x)<box.x+box.w-.1;
        if(hit)hits.push({edge:edge.id,box:box.id});
      }
    }
    return hits;
  },singleTargetInventory().records.filter(n=>n.children&&n.branch!=='area').map(n=>n.id));
  expect(crossings,'Native outer arrows do not pass through any participant').toEqual([]);

  const entry=page.locator('[data-component-overview="backend"] [data-overview-area]').first(),aim=await entry.boundingBox();
  await page.mouse.move(aim.x+aim.width/2,aim.y+aim.height/2);
  const world=await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]));
  for(let step=0;step<24&&await page.locator('[data-component-overview="backend"]').count();step++){
    const zoom=await map.evaluate(map=>map.captureViewport().zoom);
    await page.keyboard.down('Control');try{await page.mouse.wheel(0,-8);}finally{await page.keyboard.up('Control');}
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
  }
  await expect(page.locator('[data-component-overview="backend"]')).toHaveCount(0);
  await expect(page.locator('[data-summary-area="requests"]')).toBeVisible();
  expect(await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]))).toEqual(world);
  await testInfo.attach('journey-02 — Pinch reveals the target interior without its zoom button',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});

test('external arrows end at the frame with its plaque beside them',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="front"]').click();
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.includes('front'))).toBe(true);
  await page.locator('[data-summary-area="editing"] strong,[data-frame-title="editing"]>strong').click();
  await expect(page.locator('.react-flow__node[data-id="submission"]')).toBeVisible();
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
  await page.mouse.move(1430,890);
  await page.locator('[data-frame-title="editing"]>strong').hover();
  await expect(map).toHaveAttribute('data-subject','editing');
  const label=page.locator('.flow-boundary-label[data-connection-outside="api"]');
  await expect(label,'an end joining some of the parts is a plain handle').toHaveText('');
  const external=await map.evaluate(map=>map.visibleEdges.find(e=>e.from==='submission'&&e.to==='post'));
  const drawn=await page.locator(`[data-edge-ids~="${external.id}"] path:not(.flow-edge-casing)`).getAttribute('d');
  expect(drawn).toBe(external.outerSegments.map(points=>points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' '));
  await testInfo.attach('journey-01 — External lines stay outside',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  // The outer frame can extend beyond this close area view. Pan to its native
  // endpoint without changing scale or relying on an offscreen DOM assertion.
  const marker=await label.boundingBox(),canvas=await page.locator('.flow-root').boundingBox();
  const viewport=await map.evaluate(map=>map.captureViewport());
  await map.evaluate((map,state)=>map.restoreReadingState(state),{scope:'editing',viewport:{...viewport,
    x:viewport.x+canvas.x+canvas.width/2-marker.x-marker.width/2,
    y:viewport.y+canvas.y+canvas.height/2-marker.y-marker.height/2}});
  await expect(label).toBeInViewport();
  const endpoint=await map.evaluate(map=>{
    const edge=map.visibleEdges.find(e=>e.from==='submission'&&e.to==='post');
    const m=new DOMMatrixReadOnly(getComputedStyle(map.querySelector('.react-flow__viewport')).transform);
    const host=map.querySelector('.flow-root').getBoundingClientRect(),p=edge.outerSegments[0][0];
    return {x:host.x+m.e+p.x*m.a,y:host.y+m.f+p.y*m.d};
  });
  // The plaque sits on the frame's border where its arrow meets it and
  // covers nothing the frame holds (owner, 2026-09-28).
  const markerBox=await label.locator('button').boundingBox(),rootBox=await page.locator('.react-flow__node[data-id="front"]').boundingBox();
  const centre={x:markerBox.x+markerBox.width/2,y:markerBox.y+markerBox.height/2};
  const border=Math.min(Math.abs(centre.x-rootBox.x),Math.abs(centre.x-rootBox.x-rootBox.width),Math.abs(centre.y-rootBox.y),Math.abs(centre.y-rootBox.y-rootBox.height));
  expect(border,'The plaque stands on the frame border').toBeLessThanOrEqual(markerBox.width/2+.5);
  expect(Math.abs(centre.y-endpoint.y),'The plaque is centred on its native connection').toBeLessThan(.5);
  const inner=(await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>({id:n.dataset.id,box:n.getBoundingClientRect().toJSON()}))))
    .filter(n=>n.id!=='front'&&n.box.left>=rootBox.x-.5&&n.box.top>=rootBox.y-.5&&n.box.right<=rootBox.x+rootBox.width+.5&&n.box.bottom<=rootBox.y+rootBox.height+.5);
  expect(inner.length,'the frame holds boxes').toBeGreaterThan(0);
  for(const node of inner){
    const b=node.box,overlap=markerBox.x<b.right&&markerBox.x+markerBox.width>b.left&&markerBox.y<b.bottom&&markerBox.y+markerBox.height>b.top;
    expect(overlap,`The plaque covers no part of the frame: ${node.id}`).toBe(false);
  }
  // Over the arrows: an arrowhead had covered the plaque's "all".
  const [labelLayer,edgeLayer]=await label.evaluate(element=>[Number(getComputedStyle(element).zIndex),Math.max(0,...[...document.querySelectorAll('.react-flow__edges svg')].map(svg=>Number(getComputedStyle(svg).zIndex)||0))]);
  expect(labelLayer,'The plaque stands over the arrows').toBeGreaterThan(edgeLayer);
  await testInfo.attach('journey-02 — Pan to the outer arrow · matching endpoint 2',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});
