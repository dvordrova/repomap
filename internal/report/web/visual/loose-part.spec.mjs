import {test,expect} from '@playwright/test';

// Redis's Debug symbols, a part in no area, read 41 px beside 17 px parts
// once the areas beside it opened: it kept the scale of their closed
// summaries.
test('a loose part reads at its peers\' size once the areas beside it open',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?loose-part');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const font=id=>page.locator(`.react-flow__node[data-id="${id}"] .flow-part>strong`).evaluate(el=>parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth);
  const revision=await map.getAttribute('data-camera-revision');
  await map.evaluate(map=>map.focusNode('queue'));
  await expect(map).not.toHaveAttribute('data-camera-revision',revision);
  await expect.poll(async()=>(await map.evaluate(map=>map.captureViewport())).detailAreas.length).toBeGreaterThan(0);
  await expect(page.locator('.react-flow__node[data-id="audit"]')).toBeVisible();
  const peer=await font('queue'),loose=await font('audit');
  expect(Math.abs(loose-peer),`the loose part's title is ${loose.toFixed(1)} px beside ${peer.toFixed(1)} px parts`).toBeLessThan(.5);
  expect(errors).toEqual([]);
});

// On the component overview, with the areas beside it closed, the loose part
// is their summaries' peer: its title reads no smaller than the smallest of
// their titles. Fitted to a card's box, Redis's Debug symbols read 10 px
// beside 15 to 17 px area titles.
test('a loose part reads as large as its closed areas\' titles on the component overview',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?loose-part');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const revision=await map.getAttribute('data-camera-revision');
  await map.evaluate(map=>map.focusNode('backend'));
  await expect(map).not.toHaveAttribute('data-camera-revision',revision);
  await expect(page.locator('.react-flow__node[data-id="audit"] .flow-standalone-part')).toBeVisible();
  const screen=locator=>locator.evaluate(el=>parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth);
  const areas=[];
  for(const id of ['requests','execution'])areas.push(await screen(page.locator(`[data-summary-area="${id}"] strong`)));
  const loose=await screen(page.locator('.react-flow__node[data-id="audit"] .flow-standalone-part>strong'));
  expect(loose,`the loose part's title is ${loose.toFixed(1)} px beside ${areas.map(px=>px.toFixed(1)).join(' and ')} px area titles`).toBeGreaterThanOrEqual(Math.min(...areas)-.5);
  expect(errors).toEqual([]);
});
