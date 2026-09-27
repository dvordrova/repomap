import {test,expect} from '@playwright/test';

// Entering an area, or a chosen input's path, lets the camera stand as small
// as an open layer stays open. From the closed whole map, the move's own
// zooms closed the layer on the way, and it then needed the larger zoom that
// opens a closed one: microblog's /explore stood on the closed Web routes
// summary, its title at 45 px, and no part of its path was drawn.
test.use({viewport:{width:900,height:700}});

// The camera rests once two looks a quarter second apart agree.
const rest=async map=>{
  let last='';
  await expect.poll(async()=>{const now=JSON.stringify(await map.evaluate(map=>map.captureViewport()));const same=now===last;last=now;return same;},{intervals:[250]}).toBe(true);
};
const enter=async(page,query,act)=>{
  await page.goto(query);
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await rest(map);
  const revision=await map.getAttribute('data-camera-revision');
  await map.evaluate(act);
  await expect(map).not.toHaveAttribute('data-camera-revision',revision);
  await rest(map);
  return map;
};

test('an area entered from the whole map stands open',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=await enter(page,'/',map=>map.focusNode('requests'));
  expect((await map.evaluate(map=>map.captureViewport())).detailAreas).toContain('requests');
  await expect(page.locator('.react-flow__node[data-id="routes"]')).toBeVisible();
  expect(errors).toEqual([]);
});

test('a chosen input\'s path entered from the whole map stands open',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=await enter(page,'/?input-path',map=>map.chooseInput('create'));
  expect((await map.evaluate(map=>map.captureViewport())).detailAreas).toContain('requests');
  await expect(page.locator('.react-flow__node[data-id="routes"]')).toBeVisible();
  expect(errors).toEqual([]);
});
