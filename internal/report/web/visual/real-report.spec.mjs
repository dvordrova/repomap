import {test,expect} from '@playwright/test';

// A real report, not the fixture: REPOMAP_REAL_RUN names a saved run
// directory (the Redis run the tester used), which the test server renders
// with the built .bin/repomap and serves as /real-report.html. No provider
// call. Without it the test is skipped:
//   make build && REPOMAP_REAL_RUN=/path/to/run npx playwright test real-report
// On the saved Redis run the arrow ends never opened a card for a real
// pointer: chips vanished as the pointer crossed redis-server's space to
// them, and an arrowhead was the map's background.
test.skip(!process.env.REPOMAP_REAL_RUN,'set REPOMAP_REAL_RUN to a saved run directory');
test.describe.configure({mode:'serial'});

async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100],timeout:30_000}).toBeGreaterThanOrEqual(2);
}
// Every drawn arrowhead on the canvas that touches a frame (an area, a
// component, a destination), with a point on the head 5px back from its tip.
// A head on a part inside one frame joins no frame and is left out.
const framedHeads=page=>page.evaluate(()=>{
  const canvas=document.querySelector('.flow-root').getBoundingClientRect();
  const frames=[...document.querySelectorAll('.react-flow__node-area')].filter(n=>getComputedStyle(n).visibility!=='hidden').map(n=>n.getBoundingClientRect());
  const onBorder=p=>frames.some(r=>p.x>=r.left-2&&p.x<=r.right+2&&p.y>=r.top-2&&p.y<=r.bottom+2&&Math.min(Math.abs(p.x-r.left),Math.abs(p.x-r.right),Math.abs(p.y-r.top),Math.abs(p.y-r.bottom))<=2);
  return [...document.querySelectorAll('.flow-edge path:not(.flow-edge-casing)')].flatMap(path=>{
    const ctm=path.getScreenCTM(),length=path.getTotalLength(),scale=Math.hypot(ctm.a,ctm.b);
    const at=l=>{const p=path.getPointAtLength(l);return {x:p.x*ctm.a+p.y*ctm.c+ctm.e,y:p.x*ctm.b+p.y*ctm.d+ctm.f};};
    const head=(tip,back)=>{const d=Math.hypot(back.x-tip.x,back.y-tip.y)||1;return {edge:path.parentElement.dataset.edgeId,tip,x:tip.x+(back.x-tip.x)/d*5,y:tip.y+(back.y-tip.y)/d*5};};
    return [path.getAttribute('marker-end')?head(at(length),at(length-8/scale)):null,path.getAttribute('marker-start')?head(at(0),at(8/scale)):null].filter(Boolean);
  }).filter(h=>onBorder(h.tip)&&h.x>canvas.left+8&&h.x<canvas.right-8&&h.y>canvas.top+8&&h.y<canvas.bottom-8);
});
// From `from`, a real pointer walks to the handle and rests; its card opens,
// the pointer walks onto the card and rests past the linger; it stays.
async function reach(page,from,point,what){
  await page.mouse.move(from.x,from.y,{steps:6});await page.waitForTimeout(250);
  await page.mouse.move(point.x,point.y,{steps:18});
  const card=page.locator('.flow-arrow-card .flow-connection-calls');
  await expect(card,`${what}: its card opens`).toBeVisible({timeout:1500});
  const box=await card.boundingBox();
  await page.mouse.move(box.x+Math.min(80,box.width/2),box.y+Math.min(30,box.height/2),{steps:15});await page.waitForTimeout(700);
  await expect(card,`${what}: the pointer reached its card`).toBeVisible();
  await page.keyboard.press('Escape');
}
const middle=box=>({x:box.x+box.width/2,y:box.y+box.height/2});

for(const [width,height] of [[1440,900],[1280,800]])test(`every arrow end and chip of a real report opens its card at ${width}×${height}`,async({page})=>{
  test.setTimeout(300_000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.setViewportSize({width,height});
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:60_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:60_000});
  await settle(map);
  const canvas=await page.locator('.flow-root').boundingBox(),corner={x:canvas.x+canvas.width-4,y:canvas.y+canvas.height-4};
  // The whole map: the head of every arrow between its participants.
  const system=await framedHeads(page);
  expect(system.length,'the whole map draws arrows between frames').toBeGreaterThan(0);
  for(const h of system)await reach(page,corner,h,`the whole map's head of ${h.edge}`);
  // Enter the component holding the most areas, as a reader does.
  const component=await page.evaluate(()=>[...document.querySelectorAll('[data-component-overview]')]
    .map(n=>({id:n.dataset.componentOverview,areas:n.querySelectorAll('[data-overview-area]').length})).sort((a,b)=>b.areas-a.areas)[0].id);
  const entrance=middle(await page.locator(`[data-component-overview="${component}"] strong`).first().boundingBox());
  await page.mouse.click(entrance.x,entrance.y);
  await settle(map);await page.mouse.move(corner.x,corner.y);
  const chips=await page.locator('.flow-connection-label').evaluateAll((labels,canvas)=>labels.map(l=>({id:l.dataset.connectionLabel,box:l.getBoundingClientRect().toJSON()}))
    .filter(({box})=>box.x>canvas.x&&box.y>canvas.y&&box.x+box.width<canvas.x+canvas.width&&box.y+box.height<canvas.y+canvas.height),canvas);
  expect(chips.length,'the entered component marks its arrow ends on its border').toBeGreaterThan(0);
  // From inside one of its areas across the component to each plaque.
  const area=await page.locator(`[data-summary-area],[data-frame-title]:not([data-frame-title="${component}"])`).evaluateAll((nodes,canvas)=>nodes.map(n=>n.getBoundingClientRect().toJSON())
    .find(b=>b.x>canvas.x&&b.y>canvas.y&&b.x+b.width<canvas.x+canvas.width&&b.y+b.height<canvas.y+canvas.height),canvas);
  const inside={x:area.x+Math.min(40,area.width/2),y:area.y+Math.min(20,area.height/2)};
  for(const chip of chips)await reach(page,inside,{x:chip.box.x+chip.box.width/2,y:chip.box.y+chip.box.height/2},`chip ${chip.id}`);
  const heads=await framedHeads(page);
  expect(heads.length,'arrows meet the entered component and its areas').toBeGreaterThan(0);
  for(const h of heads)await reach(page,inside,h,`the component's head of ${h.edge}`);
  // A click on an arrowhead, and on a chip, reads that connection in the
  // column; the camera stays.
  for(const point of [heads[0],middle(chips[0].box)]){
    await page.mouse.move(point.x,point.y,{steps:10});await page.waitForTimeout(300);
    const camera=await map.evaluate(map=>map.captureViewport());
    await page.mouse.click(point.x,point.y);
    await expect(page.locator('.map-frame-connections-holder details[open]'),'the column opens that connection').toHaveCount(1);
    expect(await map.evaluate(map=>map.captureViewport()),'the camera stays').toEqual(camera);
    await page.mouse.move(corner.x,corner.y);await page.keyboard.press('Escape');
  }
  expect(errors).toEqual([]);
});
