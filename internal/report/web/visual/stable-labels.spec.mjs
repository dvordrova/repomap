import {test,expect} from '@playwright/test';

// Cropping is a camera operation, not a new text layout. The exact line boxes
// and card geometry must survive both edges of the viewport.
test('target, external and input summaries keep their text layout when panned out of view',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  const initial=await map.evaluate(map=>map.captureViewport());
  const cards=page.locator('[data-component-overview]');
  const layout=()=>cards.evaluateAll(elements=>elements.map(element=>{
    const range=document.createRange();range.selectNodeContents(element);
    const m=new DOMMatrixReadOnly(getComputedStyle(document.querySelector('.react-flow__viewport')).transform);
    const box=element.getBoundingClientRect();
    return {id:element.dataset.componentOverview,width:element.style.width,transform:element.style.transform,
      lines:[...range.getClientRects()].map(r=>[(r.left-box.left)/m.a,(r.top-box.top)/m.a,r.width/m.a,r.height/m.a])};
  }));
  const before=await layout();
  expect(before.length).toBeGreaterThan(3);
  await testInfo.attach('journey-01 — Stable summary text at the whole map',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  const canvas=await page.locator('.flow-root').boundingBox();
  const input=page.locator('[data-component-overview="front-inputs"]');
  const inputBox=await input.boundingBox();
  for(const [index,x] of [initial.x+canvas.x+canvas.width-45-inputBox.x,initial.x-canvas.width*.65].entries()){
    await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...initial,x,fit:false});
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().x)).toBeCloseTo(x,5);
    const after=await layout();
    expect(after.map(({lines,...card})=>card),'Pan keeps fixed card dimensions').toEqual(before.map(({lines,...card})=>card));
    for(let i=0;i<after.length;i++){
      expect(after[i].lines.length,'Pan preserves every line break').toBe(before[i].lines.length);
      for(let j=0;j<after[i].lines.length;j++)for(let k=0;k<4;k++)
        // DOMRange coordinates include float rounding after a viewport translation.
        expect(after[i].lines[j][k]).toBeCloseTo(before[i].lines[j][k],1);
    }
    await testInfo.attach(`journey-0${index+2} — Text stays fixed at the ${index?'left':'right'} edge`,{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  }
});

test('hover emphasis changes only the outline, never the compact title layout',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const canvas=await page.locator('.flow-root').boundingBox();
  await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
  for(let step=0;step<20;step++){
    if((await map.evaluate(map=>map.captureViewport())).openComponents.length)break;
    const zoom=await map.evaluate(map=>map.captureViewport().zoom);
    await page.keyboard.down('Control');try{await page.mouse.wheel(0,-4);}finally{await page.keyboard.up('Control');}
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
  }
  const summaries=page.locator('[data-summary-area]');
  expect(await summaries.count()).toBeGreaterThan(1);
  const card=summaries.first(),box=await card.boundingBox(),v=await map.evaluate(map=>map.captureViewport());
  await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...v,fit:false,
    x:v.x+canvas.x+canvas.width/2-box.x-box.width/2,y:v.y+canvas.y+canvas.height/2-box.y-box.height/2});
  await page.mouse.move(1430,890);
  const measure=()=>summaries.evaluateAll(elements=>elements.map(el=>{
    const title=el.querySelector('strong'),b=el.getBoundingClientRect();
    const range=document.createRange();range.selectNodeContents(title);
    return {id:el.dataset.summaryArea,text:title.textContent,lines:[...range.getClientRects()].map(r=>
      [r.left-b.left,r.top-b.top,r.width,r.height].map(n=>Math.round(n*100)))};
  }));
  const before=await measure();
  await testInfo.attach('journey-01 — Group titles before emphasis',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await card.hover();
  await expect(map).toHaveAttribute('data-emphasis','hover');
  expect(await measure(),'Focus and connected states cannot change text insets or wrapping').toEqual(before);
  await testInfo.attach('journey-02 — Emphasis preserves the same line boxes',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});

test('selecting a call does not insert a row above its title or move the camera',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="api"]').click();
  const title=page.locator('.react-flow__node[data-id="post"] .flow-part>strong');
  await expect(title).toBeInViewport();
  let previous='',stable=0;
  await expect.poll(async()=>{
    const current=JSON.stringify(await map.evaluate(map=>map.captureViewport()));
    stable=current===previous?stable+1:0;previous=current;return stable;
  },{intervals:[100]}).toBeGreaterThanOrEqual(2);
  const lines=()=>title.evaluate(el=>{const r=document.createRange();r.selectNodeContents(el);return [...r.getClientRects()].map(b=>[b.x,b.y,b.width,b.height]);});
  const before=await lines(),camera=await map.evaluate(map=>map.captureViewport());
  await testInfo.attach('journey-01 — Call title before selection',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await title.click();
  await expect(page.locator('[data-reading-title]')).toHaveText('POST /api/jobs');
  expect(await lines(),'Selection does not add a reading badge row or change the title inset').toEqual(before);
  expect(await map.evaluate(map=>map.captureViewport())).toEqual(camera);
  await testInfo.attach('journey-02 — Selection preserves the same title position',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});
