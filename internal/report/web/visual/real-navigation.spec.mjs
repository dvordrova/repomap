import {test,expect} from '@playwright/test';

// The toolbar's breadcrumb and a declaration's reading on a real report
// (REPOMAP_REAL_RUN, rendered by .bin/repomap render as in
// real-report.spec.mjs); the fixture has neither. On Redis the breadcrumb was
// one link that went nowhere.
test.skip(!process.env.REPOMAP_REAL_RUN,'set REPOMAP_REAL_RUN to a saved run directory');
test.describe.configure({mode:'serial'});

async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100],timeout:30_000}).toBeGreaterThanOrEqual(2);
}
async function open(page,width,height){
  await page.setViewportSize({width,height});
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:60_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:60_000});
  await settle(map);
  return map;
}
const crumbs=page=>page.locator('.reading-map-context a');

for(const [width,height] of [[1440,900],[1280,800]])test(`each breadcrumb segment goes up to its level at ${width}×${height}`,async({page})=>{
  test.setTimeout(120_000);
  const map=await open(page,width,height);
  // The component holding the most areas, one of its areas, one of its parts.
  const component=await page.evaluate(()=>[...document.querySelectorAll('[data-component-overview]')]
    .map(n=>({id:n.dataset.componentOverview,areas:n.querySelectorAll('[data-overview-area]').length})).sort((a,b)=>b.areas-a.areas)[0].id);
  await page.locator(`[data-zoom-into="${component}"]`).click();await settle(map);
  const area=await page.locator('[data-summary-area]').evaluateAll(nodes=>nodes.map(n=>n.dataset.summaryArea)[0]);
  await page.locator(`[data-zoom-into="${area}"]`).click();await settle(map);
  // A part the reader can see: the first in the page's order may stand
  // outside the canvas, where a click lands on the area around it.
  const canvas=await page.locator('.flow-root').boundingBox();
  const boxes=await page.locator(`.react-flow__node[data-id^="n-"]`).filter({has:page.locator('.flow-part-zoom')}).evaluateAll(nodes=>nodes.map(n=>n.getBoundingClientRect().toJSON()));
  const box=boxes.find(b=>b.x>canvas.x&&b.y>canvas.y&&b.x+b.width<canvas.x+canvas.width&&b.y+b.height<canvas.y+canvas.height);
  expect(box,'a whole part stands in the canvas').toBeTruthy();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:8});await page.mouse.click(box.x+box.width/2,box.y+box.height/2);await settle(map);
  const titles=await crumbs(page).allTextContents();
  expect(titles.length,`component, area and part are each a link: ${titles}`).toBe(3);
  await expect(page.locator('.reading-map-context')).toHaveText(titles.join(' / '));
  // Up to the area: it is read and framed.
  await crumbs(page).nth(1).click();await settle(map);
  await expect(page.locator('.map-inspector-heading')).toContainText(titles[1]);
  await expect(crumbs(page)).toHaveCount(2);
  expect((await map.evaluate(map=>map.captureViewport())).detailAreas.length,'the area is open').toBeGreaterThan(0);
  // Up to the component: read and framed whole, its areas closed.
  await crumbs(page).first().click();await settle(map);
  await expect(page.locator('.map-inspector-heading')).toContainText(titles[0]);
  await expect(crumbs(page)).toHaveCount(1);
  await expect(page.locator('.flow-location')).toHaveText(titles[0]);
});

// A name in a declaration's reading reads that declaration and marks its
// tile on the canvas: from tryResizeHashTables, serverCron (owner,
// 2026-09-28). Its tile in sight, the camera stays; Back returns to the
// declaration read before it.
test('a caller named in a declaration\'s reading becomes the canvas\'s chosen tile',async({page})=>{
  test.setTimeout(120_000);
  const map=await open(page,1440,900);
  const part=await page.evaluate(()=>[...document.querySelectorAll('[data-node]')].find(n=>JSON.parse(n.dataset.symbols||'[]').some(s=>s.name==='tryResizeHashTables'))?.dataset.title);
  test.skip(!part,'this run has no tryResizeHashTables');
  await map.evaluate((map,title)=>{const n=[...document.querySelectorAll('[data-node]')].find(n=>n.dataset.title===title);const s=JSON.parse(n.dataset.symbols).find(s=>s.name==='tryResizeHashTables');return map.revealNode(n,false,{key:s.href||s.open});},part);
  await settle(map);
  await expect(page.locator('.map-decl-code')).toHaveText('tryResizeHashTables');
  await expect(page.locator('.flow-symbol-chosen')).toContainText('tryResizeHashTables');
  const camera=await map.evaluate(map=>map.captureViewport());
  await page.locator('.map-decl-reading .map-reading-name',{hasText:/^serverCron\(\)$/}).first().click();await settle(map);
  await expect(page.locator('.map-decl-code'),'serverCron is the reading\'s subject').toHaveText('serverCron');
  await expect(page.locator('.flow-symbol-chosen'),'serverCron is the chosen tile').toContainText('serverCron');
  await expect(page.locator('.reading-map-context')).toContainText('· serverCron');
  expect(await map.evaluate(map=>map.captureViewport()),'its tile was in sight: the camera stays').toEqual(camera);
  await page.goBack();await settle(map);
  await expect(page.locator('.map-decl-code')).toHaveText('tryResizeHashTables');
});
