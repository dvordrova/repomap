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
// the whole map past the areas into a part's tiles, and one pinch over
// Replication's 7px card went on into syncRead's tiles: the part under the
// pinch reading its title is a level of its own (owner, 2026-09-28).
test('one pinch crosses at most one level boundary and a pause lets the next cross one more',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=await ready(page,'?symbols');
  const worker=await page.locator('.react-flow__node[data-id="worker"]').boundingBox();
  const aim={x:worker.x+worker.width/2,y:worker.y+worker.height/2};
  await page.mouse.move(aim.x,aim.y,{steps:4});
  // The level, and half a level more once the part under the pinch reads
  // its title at twelve pixels.
  const aimed=async()=>{
    const base=await level(map);
    if(base===3)return 3.5;
    // Its font on screen: the CSS size times the scale it is drawn at (the
    // title wraps, so its height is no measure of it).
    // A title read only while its part is drawn: inside a closed area it is
    // hidden however large.
    const title=base<2?0:await page.locator('.react-flow__node[data-id="worker"] .flow-part>strong').evaluate(strong=>getComputedStyle(strong).visibility==='hidden'?0:parseFloat(getComputedStyle(strong).fontSize)*strong.getBoundingClientRect().height/strong.offsetHeight);
    return base+(title>=12?.5:0);
  };
  async function pinch(ticks,deltaY){
    const seen=[await aimed()];
    await page.keyboard.down('Control');
    try{for(let tick=0;tick<ticks;tick++){await page.mouse.wheel(0,deltaY);await page.waitForTimeout(60);seen.push(await aimed());}}
    finally{await page.keyboard.up('Control');}
    await settle(map);seen.push(await aimed());
    return seen;
  }
  const crossings=seen=>seen.slice(1).filter((level,i)=>level!==seen[i]).length;
  // Pinch in, pausing between pinches, until the tiles are drawn: each
  // pinch crosses at most one boundary, and the stop before the tiles is
  // the part reading its title.
  const stops=[];
  for(let pinches=0;pinches<5&&stops.at(-1)!==3.5;pinches++){
    if(pinches)await page.waitForTimeout(500);
    const seen=await pinch(10,-40);
    expect(crossings(seen),`one pinch in crosses at most one boundary: ${seen}`).toBeLessThanOrEqual(1);
    stops.push(seen.at(-1));
    await testInfo.attach(`journey-0${pinches+1} — Pinch ${pinches+1}: level ${seen.at(-1)}`,{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  }
  expect(stops.at(-1),`the tiles are reached: ${stops}`).toBe(3.5);
  expect(stops.at(-2),`before the tiles the part's title reads: ${stops}`).toBe(2.5);
  await page.waitForTimeout(500);
  const out=await pinch(14,40);
  expect(crossings(out),`one pinch out crosses one boundary: ${out}`).toBe(1);
  expect(out.at(-1)).toBe(2.5);
  expect(errors).toEqual([]);
});


// An overflowing inventory takes the wheel, and keeps it at its end: past
// its last entry the next notch had scrolled the page, and the one after it
// moved the map under a pointer that had not moved.
test('a wheel past the end of an overflowing inventory scrolls neither the page nor the map',async({page})=>{
  await page.setViewportSize({width:1100,height:640});
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
