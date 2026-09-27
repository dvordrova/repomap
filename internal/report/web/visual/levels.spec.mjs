import {test,expect} from '@playwright/test';

// The levels a camera stands at: 0 the whole map, 1 a component or an
// outside frame open, 2 its areas (or an input collection's groups) open,
// 3 a part's declarations drawn as tiles.
const level=map=>map.evaluate(map=>{
  const v=map.captureViewport();
  if(document.querySelector('.flow-part-deep'))return 3;
  if(v.detailAreas.length||v.communicationsOpen.some(id=>id.includes('~')))return 2;
  return v.openComponents.length||v.communicationsOpen.length?1:0;
});
async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
}
async function ready(page,query=''){
  await page.goto('/'+query);
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await settle(map);
  return map;
}

// "−" steps out one level, as a zoom mark steps in one: from a part's tiles
// to its area, from the area to its component, from the component to the
// whole map. It zoomed out by a fifth and left Redis's readers at the level
// they were on; they reached for "Show whole map" up to nine times. In this
// small map a component's areas read as soon as it opens, so its level
// and its areas' are one.
test('the − control steps out one level at a time',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=await ready(page,'?symbols');
  await map.evaluate(map=>map.focusNode('worker'));await settle(map);
  await page.locator('.react-flow__node[data-id="worker"] .flow-part-zoom').click();await settle(map);
  expect(await level(map)).toBe(3);
  const steps=[];
  for(let press=0;press<3&&(steps.at(-1)?.level??3)>0;press++){
    await page.getByRole('button',{name:'Zoom out',exact:true}).click();await settle(map);
    steps.push({level:await level(map),location:await page.locator('.flow-location').textContent(),fit:(await map.evaluate(map=>map.captureViewport())).fit});
    await testInfo.attach(`journey-0${press+1} — "−" pressed ${press+1} time${press?'s':''}`,{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  }
  expect(steps.map(step=>step.level),'out of the tiles, then out of the areas').toEqual([2,0]);
  expect(steps[0].location,'out of the tiles, the camera is on their area').toContain('Job execution');
  expect(errors).toEqual([]);
});

// The same steps on a map whose component opens before its areas: each
// press leaves exactly one level, down to the whole map.
test('the − control steps from an area to its component, then to the whole map',async({page})=>{
  const map=await ready(page,'?many-external');
  await page.locator('[data-zoom-into="front"]').click();await settle(map);
  expect(await level(map)).toBe(1);
  await page.locator('[data-summary-area="tracking"] strong').click();await settle(map);
  await page.locator('[data-zoom-into="tracking"]').click().catch(()=>{});await settle(map);
  expect(await level(map)).toBe(2);
  const steps=[];
  for(let press=0;press<2;press++){
    await page.getByRole('button',{name:'Zoom out',exact:true}).click();await settle(map);
    steps.push({level:await level(map),location:await page.locator('.flow-location').textContent()});
  }
  expect(steps.map(step=>step.level)).toEqual([1,0]);
  expect(steps[1].location).toBe('System map');
});

// One pinch crosses at most one level boundary; a pause lets the next pinch
// cross the next. Eight ctrl+wheel ticks had carried a Redis reader from
// the whole map past the areas into a part's tiles.
test('one pinch crosses at most one level boundary and a pause lets the next cross one more',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=await ready(page,'?symbols');
  const worker=await page.locator('.react-flow__node[data-id="worker"]').boundingBox();
  const aim={x:worker.x+worker.width/2,y:worker.y+worker.height/2};
  await page.mouse.move(aim.x,aim.y,{steps:4});
  async function pinch(ticks,deltaY){
    const seen=[await level(map)];
    await page.keyboard.down('Control');
    try{for(let tick=0;tick<ticks;tick++){await page.mouse.wheel(0,deltaY);await page.waitForTimeout(60);seen.push(await level(map));}}
    finally{await page.keyboard.up('Control');}
    await settle(map);seen.push(await level(map));
    return seen;
  }
  const crossings=seen=>seen.slice(1).filter((level,i)=>level!==seen[i]).length;
  const first=await pinch(10,-40);
  expect(crossings(first),`one pinch in crosses one boundary: ${first}`).toBe(1);
  expect(first.at(-1)).toBeGreaterThan(0);
  await testInfo.attach('journey-01 — One pinch in: one boundary',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await page.waitForTimeout(500);
  const second=await pinch(10,-40);
  expect(crossings(second),`after a pause the next pinch crosses the next: ${second}`).toBe(1);
  expect(second.at(-1)).toBe(3);
  await testInfo.attach('journey-02 — After a pause, the tiles',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await page.waitForTimeout(500);
  const out=await pinch(14,40);
  expect(crossings(out),`one pinch out crosses one boundary: ${out}`).toBe(1);
  expect(out.at(-1)).toBe(second.at(-1)-1);
  expect(errors).toEqual([]);
});

// The location row stands on the canvas, over the map. A wheel over it
// scrolled the page while a pixel lower it moved the map.
test('a wheel over the canvas location row moves the map, not the page',async({page})=>{
  const map=await ready(page);
  await page.evaluate(()=>{document.body.style.minHeight='3000px';window.scrollTo(0,0);});
  const row=await page.locator('.flow-location').boundingBox();
  await page.mouse.move(row.x+row.width/2,row.y+row.height/2,{steps:4});
  const before=await map.evaluate(map=>map.captureViewport());
  await page.mouse.wheel(0,120);
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().y)).not.toBe(before.y);
  expect(await page.evaluate(()=>window.scrollY),'the page stays').toBe(0);
  expect((await map.evaluate(map=>map.captureViewport())).zoom).toBe(before.zoom);
});

// An overflowing inventory takes the wheel, and keeps it at its end: past
// its last entry the next notch had scrolled the page, and the one after it
// moved the map under a pointer that had not moved.
test('a wheel past the end of an overflowing inventory scrolls neither the page nor the map',async({page})=>{
  const map=await ready(page,'?dense');
  await page.evaluate(()=>{document.body.style.minHeight='3000px';window.scrollTo(0,0);});
  const list=page.locator('.flow-component-areas.flow-scrollable').first();
  const box=await list.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:4});
  const camera=await map.evaluate(map=>map.captureViewport());
  const end=await list.evaluate(list=>list.scrollHeight-list.clientHeight);
  for(let notch=0;notch<Math.ceil(end/200)+3;notch++){await page.mouse.wheel(0,200);await page.waitForTimeout(150);}
  expect(await list.evaluate(list=>list.scrollTop),'the inventory reached its end').toBeGreaterThanOrEqual(end-1);
  expect(await page.evaluate(()=>window.scrollY),'the page stays').toBe(0);
  expect(await map.evaluate(map=>map.captureViewport())).toEqual(camera);
});
