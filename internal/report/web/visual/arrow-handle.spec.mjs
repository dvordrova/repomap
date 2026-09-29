import {test,expect} from '@playwright/test';

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
const place=map=>map.evaluate(map=>{const {x,y,zoom}=map.captureViewport();return {x,y,zoom};});
// A point halfway along the drawn arrow joining two boxes.
const arrowPoint=(page,from,to)=>page.evaluate(([from,to])=>{
  const g=[...document.querySelectorAll('g.flow-edge')].find(g=>g.dataset.edgeEnds===`${from} ${to}`);
  const path=g?.querySelector('[data-edge-hit]');if(!path)return null;
  const length=path.getTotalLength(),ctm=path.getScreenCTM(),p=path.getPointAtLength(length/2);
  return {x:p.x*ctm.a+p.y*ctm.c+ctm.e,y:p.x*ctm.b+p.y*ctm.d+ctm.f};
},[from,to]);

// The arrow is its own handle (owner, 2026-09-29: the plaques at arrow ends
// had floated beside them, eleven in a row on one border): no plaque is
// drawn, every arrow has a wide hit path, resting on it opens its calls'
// card and a click reads the connection, the camera staying.
test('an arrow opens its card where the pointer rests on it and a click reads it',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=await ready(page);
  await expect(page.locator('.flow-connection-label,.flow-boundary-label')).toHaveCount(0);
  const edges=await page.locator('g.flow-edge').count();
  expect(edges).toBeGreaterThan(0);
  await expect(page.locator('g.flow-edge [data-edge-hit]')).toHaveCount(edges);
  // The service's one arrow to its Outside frame.
  const point=await arrowPoint(page,'backend','backend-outside');
  expect(point,'the service draws one arrow to its Outside frame').not.toBeNull();
  const camera=await place(map);
  await page.mouse.move(point.x,point.y,{steps:6});
  const card=page.locator('.flow-arrow-card .flow-connection-calls');
  await expect(card).toBeVisible({timeout:3000});
  // The card names the frames and the calls; it prints no counts.
  expect(await card.locator('.flow-card-head').innerText()).not.toMatch(/(^|\s)\d+(\s|$)/);
  await expect(page.locator('.react-flow__node.flow-node-muted')).toHaveCount(0);
  await page.mouse.click(point.x,point.y);
  await expect(map).toHaveAttribute('data-opened-connection',/^backend out:backend-outside$/);
  expect(await place(map),'a click reads, the camera stays').toEqual(camera);
  expect(errors).toEqual([]);
});

// A chip is an outside system: pointed at, it outlines the program that
// talks to it and dims nothing; clicked, the column reads it where the
// camera stands.
test('a chip outlines who talks to it and a click reads it without moving the camera',async({page})=>{
  const map=await ready(page);
  const chip=page.locator('.react-flow__node[data-id="postgres"]');
  await expect(chip.locator('.flow-chip-name')).toHaveText('PostgreSQL');
  const box=await chip.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:6});
  await expect(map).toHaveAttribute('data-subject','postgres');
  await expect(page.locator('.react-flow__node[data-id="backend"]')).toHaveClass(/flow-node-connected/);
  await expect(page.locator('.react-flow__node.flow-node-muted')).toHaveCount(0);
  const camera=await place(map);
  await chip.click();
  await expect(page.locator('[data-reading-title]')).toHaveText('PostgreSQL');
  expect(await place(map)).toEqual(camera);
});

// "The card body turns into a magnifier, so a plain click seems to zoom"
// (owner, 2026-09-29): a card's body reads it with the ordinary pointer;
// only its magnifier zooms.
test('a card body reads its card with the ordinary pointer, and only the magnifier zooms',async({page})=>{
  const map=await ready(page);
  for(const [id,title] of [['backend','Job processing service'],['backend-inputs','Job processing service']]){
    const body=page.locator(`[data-component-overview="${id}"]`);
    await expect(body).toHaveCSS('cursor','pointer');
    await expect(page.locator(`[data-zoom-into="${id}"]`)).toHaveCSS('cursor','zoom-in');
    const box=await body.boundingBox(),camera=await place(map);
    // Away from its names: the card's lower right corner.
    await page.mouse.click(box.x+box.width-6,box.y+box.height-6);
    await expect(page.locator('[data-reading-title]')).toHaveText(title);
    expect(await place(map),`${id}: the camera stays`).toEqual(camera);
  }
});

// An input's kind is a small mark before its name, no printed row
// ("Background activity" above a tile read as noise, owner 2026-09-29).
test('an input tile carries one kind mark and no kind text',async({page})=>{
  const map=await ready(page);
  await page.locator('[data-zoom-into="backend-inputs"]').click();await settle(map);
  const tiles=page.locator('[data-input-id]');
  await expect(tiles.first()).toBeVisible();
  for(const tile of await tiles.all()){
    await expect(tile.locator('.flow-kind')).toHaveCount(0);
    await expect(tile.locator('[data-kind-mark]')).toHaveCount(1);
  }
});
