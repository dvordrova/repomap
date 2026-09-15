import {test,expect} from '@playwright/test';

for(const query of ['dense','short-names&matched-peer']){
  test(`first target reveal has named groups and thin frames (${query})`,async({page},testInfo)=>{
    test.setTimeout(60_000);
    await page.goto(`/?${query}`);
    const map=page.locator('[data-map]');
    await expect(map).toHaveAttribute('data-fixture-ready','true');
    const canvas=await page.locator('.flow-root').boundingBox();
    const target=await page.locator('.react-flow__node[data-id="front"]').boundingBox();
    const initial=await map.evaluate(map=>map.captureViewport());
    await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...initial,fit:false,
      x:initial.x+canvas.x+canvas.width/2-target.x-target.width/2,
      y:initial.y+canvas.y+canvas.height/2-target.y-target.height/2});
    const world=await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]));
    await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
    for(let step=0;step<48&&!(await map.evaluate(map=>map.captureViewport().openComponents.length));step++){
      const zoom=await map.evaluate(map=>map.captureViewport().zoom);
      await page.keyboard.down('Control');
      try{await page.mouse.wheel(0,-2);}finally{await page.keyboard.up('Control');}
      await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
    }
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.length)).toBeGreaterThan(0);
    const inspect=()=>page.locator('[data-summary-area]').evaluateAll(elements=>{
      const canvas=document.querySelector('.flow-root').getBoundingClientRect();
      return elements.flatMap(el=>{
        const frame=el.getBoundingClientRect();
        if(frame.left<canvas.left||frame.right>canvas.right||frame.top<canvas.top||frame.bottom>canvas.bottom)return [];
        const title=el.querySelector('strong');
        const node=document.querySelector(`.react-flow__node[data-id="${el.dataset.summaryArea}"]`);
        const native=node.querySelector(':scope > .flow-area'),style=getComputedStyle(native);
        const zoom=document.querySelector('[data-map]').captureViewport().zoom;
        const scale=new DOMMatrixReadOnly(getComputedStyle(el).transform).a*zoom,range=document.createRange();range.selectNodeContents(title);
        return [{id:el.dataset.summaryArea,title:title.textContent,visibility:getComputedStyle(title).visibility,
          frameVisibility:style.visibility,
          width:frame.width,height:frame.height,font:parseFloat(getComputedStyle(title).fontSize)*scale,
          border:parseFloat(style.borderTopWidth),stroke:[...style.boxShadow.matchAll(/(-?[\d.]+)px/g)].map(m=>Number(m[1]))[3]*zoom,
          rects:[...range.getClientRects()].map(r=>({left:r.left-frame.left,right:r.right-frame.left,top:r.top-frame.top,bottom:r.bottom-frame.top}))}];
      });
    });
    for(const step of [1,2]){
      if(step===2){
        const viewport=await map.evaluate(map=>map.captureViewport());
        await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...viewport,zoom:viewport.zoom*1.2});
      }
      const groups=await inspect();
      await testInfo.attach(`journey-0${step} — ${step===1?'First revealed groups':'Slightly closer groups'}`,{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
      await testInfo.attach(`Group text and frame measurements ${step}`,{body:JSON.stringify(groups,null,2),contentType:'application/json'});
      expect(groups.length,'Complete groups actually visible at this entrance').toBeGreaterThan(0);
      for(const group of groups){
        expect.soft(group.visibility,group.title+' must not be a blank rectangle').toBe('visible');
        expect.soft(group.frameVisibility,group.title+' native frame is actually painted').toBe('visible');
        if(query.startsWith('short-names'))expect.soft(Math.round(group.font*10)/10,group.title+' is readable at first reveal (CSS pixels, to 0.1px)').toBeGreaterThanOrEqual(10);
        for(const r of group.rects){
          expect.soft(r.right,group.title+' right text edge').toBeLessThanOrEqual(group.width+.5);
          expect.soft(r.bottom,group.title+' bottom text edge').toBeLessThanOrEqual(group.height+.5);
        }
        expect.soft(group.border,group.title+' has no magnified CSS border').toBe(0);
        expect.soft(group.stroke,group.title+' screen frame width').toBeLessThanOrEqual(2.1);
        expect.soft(group.stroke,group.title+' visible frame').toBeGreaterThanOrEqual(1);
      }
      await expect(page.locator('.map-workspace')).toHaveScreenshot(`group-entrance-${query.startsWith('dense')?'dense':'short-names'}-${step}.png`);
    }
    expect(await page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]))).toEqual(world);
  });
}
