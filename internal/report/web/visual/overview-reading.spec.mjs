import {test,expect} from '@playwright/test';
import {shortNamedInventory} from './two-systems-five-externals.mjs';

async function readableText(locator,frame,label,{maximumLines}={}){
  await expect(locator,label).toBeVisible();
  const result=await locator.evaluate(el=>{
    const style=getComputedStyle(el),range=document.createRange();range.selectNodeContents(el);
    const rects=[...range.getClientRects()].filter(rect=>rect.width&&rect.height);
    const words=[],walker=document.createTreeWalker(el,NodeFilter.SHOW_TEXT);
    while(walker.nextNode()){
      const node=walker.currentNode;
      for(const word of node.textContent.matchAll(/\S+/g)){
        range.setStart(node,word.index);range.setEnd(node,word.index+word[0].length);
        if(range.getClientRects().length>1)words.push(word[0]);
      }
    }
    return {font:parseFloat(style.fontSize),brokenWords:words,
      lines:new Set(rects.map(rect=>Math.round(rect.y*10))).size,
      rects:rects.map(rect=>({x:rect.x,y:rect.y,right:rect.right,bottom:rect.bottom}))};
  });
  expect(result.font,`${label} has readable type`).toBeGreaterThanOrEqual(13);
  expect(result.brokenWords,`${label} keeps each whole word on one line`).toEqual([]);
  if(maximumLines)expect(result.lines,`${label} reads as a compact list entry`).toBeLessThanOrEqual(maximumLines);
  for(const rect of result.rects){
    expect(rect.x,`${label} starts inside its frame`).toBeGreaterThanOrEqual(frame.x-.5);
    expect(rect.y,`${label} starts inside its frame`).toBeGreaterThanOrEqual(frame.y-.5);
    expect(rect.right,`${label} fits across its frame`).toBeLessThanOrEqual(frame.x+frame.width+.5);
    expect(rect.bottom,`${label} fits inside its frame`).toBeLessThanOrEqual(frame.y+frame.height+.5);
  }
}

for(const matchedPeer of [false,true])test(`short component names keep complete inventories readable in ordinary report space${matchedPeer?' with a matched peer':''}`,async({page},testInfo)=>{
  const prepared=shortNamedInventory({matchedPeer});
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?short-names'+(matchedPeer?'&matched-peer':''));
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  let previous='',stable=0;
  await expect.poll(async()=>{
    const camera=await map.evaluate(map=>map.captureViewport());
    const current=JSON.stringify(camera);
    stable=camera?.fit&&current===previous?stable+1:0;previous=current;return stable;
  },{intervals:[100]}).toBeGreaterThanOrEqual(2);
  const canvas=await page.locator('.flow-root').boundingBox();
  expect(canvas.width).toBeCloseTo(1054,0);expect(canvas.height).toBeCloseTo(580,0);
  await expect(page.locator('[data-component-overview]')).toHaveCount(matchedPeer?4:5);
  await expect(page.locator('.flow-location')).toHaveText('System map');
  const screenshot=await page.locator('.map-workspace').screenshot();
  await testInfo.attach('journey-01 — Ordinary report space · complete initial inventories',{body:screenshot,contentType:'image/png'});

  for(const item of prepared.records.filter(item=>['component','communication','inputs'].includes(item.branch))){
    const summary=page.locator(`[data-component-overview="${item.id}"]`);
    const frame=await page.locator(`.react-flow__node[data-id="${item.id}"]`).boundingBox();
    expect(frame.x).toBeGreaterThanOrEqual(canvas.x-.5);
    expect(frame.y).toBeGreaterThanOrEqual(canvas.y-.5);
    expect(frame.x+frame.width).toBeLessThanOrEqual(canvas.x+canvas.width+.5);
    expect(frame.y+frame.height).toBeLessThanOrEqual(canvas.y+canvas.height+.5);
    const title=summary.locator('.flow-component-overview-heading>strong');
    await expect(title).toHaveText(item.title);await readableText(title,frame,item.title);
    const list=summary.locator('.flow-component-areas,.flow-input-types');
    if(await list.count()){
      expect(await list.evaluate(el=>el.scrollHeight<=el.clientHeight+1),`${item.title}'s complete inventory is visible initially`).toBe(true);
    }
    for(const child of prepared.records.filter(child=>child.branch==='area'&&item.children.includes(child.id))){
      const entry=summary.locator(`[data-overview-area="${child.id}"]`);
      await expect(entry).toHaveText(child.title);
      await readableText(entry,frame,child.title,{maximumLines:2});
    }
    for(const kind of await summary.locator('[data-input-group-kind]').all()){
      await readableText(kind,frame,await kind.innerText(),{maximumLines:2});
    }
  }
  expect(errors).toEqual([]);
  await expect(page.locator('.map-workspace')).toHaveScreenshot(matchedPeer?'matched-peer-overview.png':'ordinary-space-overview.png');
});
