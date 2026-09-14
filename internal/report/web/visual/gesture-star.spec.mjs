import {test,expect} from '@playwright/test';
import {singleTargetInventory} from './two-systems-five-externals.mjs';

test('pinch over a scrolling target inventory zooms the map',async({page},testInfo)=>{
  await page.goto('/?dense');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  const list=page.locator('[data-component-overview="front"] .flow-component-areas');
  await expect(list).toBeVisible();
  expect(await list.evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
  const before=await map.evaluate(map=>map.captureViewport());
  const box=await list.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+Math.min(25,box.height/2));
  await testInfo.attach('journey-01 — Aim over the scrolling target inventory',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await page.keyboard.down('Control');
  try{await page.mouse.wheel(0,-30);}finally{await page.keyboard.up('Control');}
  await expect.poll(async()=>(await map.evaluate(map=>map.captureViewport())).zoom).toBeGreaterThan(before.zoom);
  expect(await list.evaluate(el=>el.scrollTop)).toBe(0);
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
  await expect(page.locator('.map-workspace')).toHaveScreenshot('single-target-twenty-externals.png');

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

test('external arrows end at the frame with the matching inner component numbers',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="front"]').click();
  await page.locator('[data-zoom-into="editing"]').click();
  await expect(page.locator('.react-flow__node[data-id="submission"]')).toBeVisible();
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
  await page.mouse.move(1430,890);
  await page.locator('[data-frame-title="editing"]>strong').hover();
  await expect(map).toHaveAttribute('data-subject','editing');
  const label=page.locator('.flow-boundary-label[data-connection-outside="api"]');
  await expect(label).toHaveText('2');
  await expect(page.locator('.react-flow__node[data-id="submission"] .flow-number')).toHaveText('2');
  const external=await map.evaluate(map=>map.visibleEdges.find(e=>e.from==='submission'&&e.to==='post'));
  const drawn=await page.locator(`[data-edge-ids~="${external.id}"] path:not(.flow-edge-casing)`).getAttribute('d');
  expect(drawn).toBe(external.outerSegments.map(points=>points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ')).join(' '));
  await testInfo.attach('journey-01 — Submission is component 2 · external lines stay outside',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  // The outer frame can extend beyond this close area view. Pan to its native
  // endpoint without changing scale or relying on an offscreen DOM assertion.
  const marker=await label.boundingBox(),canvas=await page.locator('.flow-root').boundingBox();
  const viewport=await map.evaluate(map=>map.captureViewport());
  await map.evaluate((map,state)=>map.restoreReadingState(state),{scope:'editing',viewport:{...viewport,
    x:viewport.x+canvas.x+canvas.width/2-marker.x-marker.width/2,
    y:viewport.y+canvas.y+canvas.height/2-marker.y-marker.height/2}});
  await expect(label).toBeInViewport();
  await expect(label).toHaveText('2');
  const covered=await label.evaluate((element,segments)=>{
    const box=element.getBoundingClientRect(),canvas=document.querySelector('.flow-root').getBoundingClientRect();
    const m=new DOMMatrixReadOnly(getComputedStyle(document.querySelector('.react-flow__viewport')).transform);
    const screen=p=>({x:canvas.x+m.e+p.x*m.a,y:canvas.y+m.f+p.y*m.d});
    return segments.some(points=>points.slice(1).some((p,i)=>{
      const a=screen(points[i]),b=screen(p);
      return Math.abs(a.x-b.x)<.1?a.x>box.left&&a.x<box.right&&Math.max(a.y,b.y)>box.top&&Math.min(a.y,b.y)<box.bottom:
        a.y>box.top&&a.y<box.bottom&&Math.max(a.x,b.x)>box.left&&Math.min(a.x,b.x)<box.right;
    }));
  },external.outerSegments);
  expect(covered,'The outer stroke must not erase the number printed beside it').toBe(false);
  await testInfo.attach('journey-02 — Pan to the outer arrow · matching endpoint 2',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});
