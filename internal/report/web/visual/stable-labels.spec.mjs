import {test,expect} from '@playwright/test';

// Cropping is a camera operation, not a new text layout. The card geometry
// and the line boxes, each against the first, survive both edges of the
// viewport: a closed box's words move as one block to stay in sight
// (scene-canvas.jsx useWordsInSight), never wrapping anew.
test('target, external and input summaries keep their text layout when panned out of view',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  const initial=await map.evaluate(map=>map.captureViewport());
  const cards=page.locator('[data-component-overview]');
  const layout=()=>cards.evaluateAll(elements=>elements.map(element=>{
    const m=new DOMMatrixReadOnly(getComputedStyle(document.querySelector('.react-flow__viewport')).transform);
    const box=element.getBoundingClientRect(),rects=[],walker=document.createTreeWalker(element,NodeFilter.SHOW_TEXT),range=document.createRange();
    for(let node=walker.nextNode();node;node=walker.nextNode()){if(!node.data.trim())continue;range.selectNodeContents(node);rects.push(...range.getClientRects());}
    const first=rects[0]||box;
    return {id:element.dataset.componentOverview,width:element.style.width,transform:element.style.transform,
      lines:rects.map(r=>[(r.left-first.left)/m.a,(r.top-first.top)/m.a,r.width/m.a,r.height/m.a])};
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
