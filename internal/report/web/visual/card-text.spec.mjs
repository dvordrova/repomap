import {test,expect} from '@playwright/test';

// Every line of text on every drawn card, in the card's own units: where it
// starts and how many lines each text holds.
const cardText=page=>page.evaluate(()=>[...document.querySelectorAll('.react-flow__node>.flow-part:not(.flow-part-deep)')]
  .filter(el=>getComputedStyle(el.parentElement).visibility!=='hidden'&&el.getBoundingClientRect().width>0).map(el=>{
    const box=el.getBoundingClientRect(),scale=box.width/el.offsetWidth;
    const tops=node=>{const range=document.createRange();range.selectNodeContents(node);
      return [...new Set([...range.getClientRects()].map(r=>Math.round((r.top-box.top)/scale*4)/4))];};
    const title=el.querySelector(':scope>strong'),description=el.querySelector(':scope>.flow-description');
    return {id:el.parentElement.dataset.id,height:el.offsetHeight,title:title.textContent,titleTops:tops(title),
      description:description?.textContent||'',descriptionTops:description?tops(description):[],
      descriptionLines:description?description.offsetHeight/18:0,numbered:!!el.querySelector('.flow-number')};
  }));
const tops=cards=>cards.map(({id,titleTops,descriptionTops})=>({id,titleTops,descriptionTops}));

// The owner's Data type commands: with the pointer inside the area every
// description dropped 18px under a number badge's reserved row, and "and"
// stood alone on a line of Set commands.
test('nothing inside a card moves when an area is looked at or chosen, and text keeps its measured lines',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?described');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="backend"]').click();
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.includes('backend'))).toBe(true);
  // A pinch opens the component's whole layer of areas, so no one area is
  // looked at until the pointer is in it.
  const canvas=await page.locator('.flow-root').boundingBox();
  for(let step=0;step<30;step++){
    if((await map.evaluate(map=>map.captureViewport().detailAreas)).length>=2)break;
    const zoom=await map.evaluate(map=>map.captureViewport().zoom);
    await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
    await page.keyboard.down('Control');try{await page.mouse.wheel(0,-6);}finally{await page.keyboard.up('Control');}
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
  }
  await expect(page.locator('.react-flow__node[data-id="auth"]')).toBeVisible();
  await page.mouse.move(1430,890);
  await expect(page.locator('.react-flow__node[data-id="auth"] .flow-number')).toHaveCount(0);
  const apart=await cardText(page);
  expect(apart.filter(card=>card.description).length).toBeGreaterThan(3);
  await testInfo.attach('journey-01 — Parts with the pointer outside their area',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});

  // Each title and description takes the lines its card was measured for.
  for(const card of apart){
    expect(card.titleTops.length,`${card.id}: the title is drawn in its measured lines`).toBe(card.title.split('\n').length);
    if(!card.description)continue;
    const reserved=(card.height-66-card.title.split('\n').length*22)/18;
    expect(card.descriptionLines,`${card.id}: "${card.description}" fills the lines its card keeps`).toBe(reserved);
    expect(card.description.includes('\n'),`${card.id}: no break is inserted into the sentence`).toBe(false);
  }

  await page.locator('.react-flow__node[data-id="auth"]').hover();
  await expect(page.locator('.react-flow__node[data-id="auth"] .flow-number')).toBeVisible();
  const looked=await cardText(page);
  expect(looked.filter(card=>card.numbered).length).toBeGreaterThan(1);
  expect(tops(looked),'the pointer in an area moves no text in its cards').toEqual(tops(apart));
  await testInfo.attach('journey-02 — The same parts while their area is looked at',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});

  await page.locator('[data-frame-title="requests"]>strong').click();
  await page.mouse.move(1430,890);
  await expect(page.locator('.react-flow__node[data-id="auth"] .flow-number')).toBeVisible();
  expect(tops(await cardText(page)),'choosing the area moves no text in its cards').toEqual(tops(apart));
  await testInfo.attach('journey-03 — The same parts with their area chosen',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});
