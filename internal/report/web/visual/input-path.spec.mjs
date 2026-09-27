import {test,expect} from '@playwright/test';

// Choosing GET centred String commands: four of its nine dark arrows were in
// the camera, none of the parts they reach, and the parts on its path looked
// like every other part.
test('a chosen input shows its path at once: its parts outlined dark and in the camera',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?input-path');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const revision=await map.getAttribute('data-camera-revision');
  await map.evaluate(map=>map.chooseInput('create'));
  await expect(map).not.toHaveAttribute('data-camera-revision',revision);
  await expect.poll(()=>page.locator('.react-flow__node[data-id="routes"]').getAttribute('class')).toContain('flow-node-connected');
  const canvas=await page.locator('.flow-root').boundingBox();
  const inside=box=>box&&box.x>=canvas.x&&box.y>=canvas.y&&box.x+box.width<=canvas.x+canvas.width&&box.y+box.height<=canvas.y+canvas.height;
  for(const id of ['routes','auth','queue','worker']){
    const node=page.locator(`.react-flow__node[data-id="${id}"]`);
    expect(inside(await node.boundingBox()),`${id} on the path stands inside the camera`).toBe(true);
    await expect(node).toBeVisible();
    await expect(node,`${id} is outlined as a participant`).toHaveClass(/flow-node-connected/);
    expect(await node.locator('.flow-part').evaluate(el=>getComputedStyle(el).borderTopColor),`${id} takes the dark of the path's arrows`).toBe('rgb(36, 48, 68)');
  }
  const other=page.locator('.react-flow__node[data-id="submission"] .flow-part');
  if(await other.isVisible())expect(await other.evaluate(el=>getComputedStyle(el).borderTopColor)).not.toBe('rgb(36, 48, 68)');
  const dark=await page.locator('.flow-edge-active').evaluateAll(edges=>edges.map(edge=>{const r=edge.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height};}));
  expect(dark.filter(inside).length,'the path\'s dark arrows are in the camera').toBeGreaterThanOrEqual(2);
  expect(errors).toEqual([]);
});
