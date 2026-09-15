import {test,expect} from '@playwright/test';

test('a single-part area is one named card at the first reveal and stays one card closer',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?single-part-area');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await expect(page.locator('.react-flow__node[data-id="requests"]')).toHaveCount(0);
  await expect(page.locator('[data-component-overview="backend"] [data-overview-area="routes"]')).toHaveText('HTTP API surface');
  const canvas=await page.locator('.flow-root').boundingBox();
  const root=await page.locator('.react-flow__node[data-id="backend"]').boundingBox();
  const initial=await map.evaluate(map=>map.captureViewport());
  await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...initial,fit:false,
    x:initial.x+canvas.x+canvas.width/2-root.x-root.width/2,
    y:initial.y+canvas.y+canvas.height/2-root.y-root.height/2});
  await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
  for(let step=0;step<48&&!(await map.evaluate(map=>map.captureViewport().openComponents.length));step++){
    const zoom=await map.evaluate(map=>map.captureViewport().zoom);
    await page.keyboard.down('Control');
    try{await page.mouse.wheel(0,-2);}finally{await page.keyboard.up('Control');}
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
  }
  const part=page.locator('.react-flow__node[data-id="routes"]');
  for(const step of [1,2]){
    if(step===2){
      const viewport=await map.evaluate(map=>map.captureViewport());
      await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...viewport,zoom:viewport.zoom*1.3});
    }
    await expect(part.locator('.flow-part>strong')).toBeVisible();
    await expect(part.locator('.flow-part>strong')).toHaveText('HTTP API surface');
    const text=await part.locator('.flow-part>strong').evaluate(el=>{
      const card=el.parentElement,frame=card.getBoundingClientRect(),range=document.createRange();range.selectNodeContents(el);
      const scale=new DOMMatrixReadOnly(getComputedStyle(card).transform).a*document.querySelector('[data-map]').captureViewport().zoom;
      return {font:parseFloat(getComputedStyle(el).fontSize)*scale,
        fits:[...range.getClientRects()].every(r=>r.left>=frame.left-.5&&r.right<=frame.right+.5&&r.top>=frame.top-.5&&r.bottom<=frame.bottom+.5)};
    });
    expect(text.font,'A direct part starts with a readable heading, like its neighbouring groups').toBeGreaterThanOrEqual(11.9);
    expect(text.fits,'The complete part name fits its card').toBe(true);
    await expect(page.locator('[data-zoom-into="routes"],[data-zoom-into="requests"]')).toHaveCount(0);
    await expect(page.locator('[data-frame-title="requests"],[data-summary-area="requests"]')).toHaveCount(0);
    const image=await page.locator('.map-workspace').screenshot();
    await testInfo.attach(`journey-0${step} — One API card · ${step===1?'first reveal':'closer'}`,{body:image,contentType:'image/png'});
    await expect(page.locator('.map-workspace')).toHaveScreenshot(`single-part-area-${step}.png`);
  }
  await part.click();
  await expect(page.locator('[data-reading-title]')).toHaveText('HTTP API surface');
  expect(errors).toEqual([]);
});
