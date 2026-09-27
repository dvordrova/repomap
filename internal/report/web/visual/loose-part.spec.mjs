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
