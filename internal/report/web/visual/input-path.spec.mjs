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
  // A part's outline, and its own as no emphasis draws it.
  const outline=part=>part.evaluate(el=>{const probe=document.createElement('div');probe.className=el.className;el.closest('.flow-root').appendChild(probe);
    const own=getComputedStyle(probe).borderTopColor;probe.remove();return {drawn:getComputedStyle(el).borderTopColor,own};});
  const inside=box=>box&&box.x>=canvas.x&&box.y>=canvas.y&&box.x+box.width<=canvas.x+canvas.width&&box.y+box.height<=canvas.y+canvas.height;
  for(const id of ['routes','auth','queue','worker']){
    const node=page.locator(`.react-flow__node[data-id="${id}"]`);
    expect(inside(await node.boundingBox()),`${id} on the path stands inside the camera`).toBe(true);
    await expect(node).toBeVisible();
    await expect(node,`${id} is outlined as a participant`).toHaveClass(/flow-node-connected/);
    const {drawn,own}=await outline(node.locator('.flow-part'));
    expect(drawn,`${id} is outlined dark`).not.toBe(own);
  }
  const other=page.locator('.react-flow__node[data-id="submission"] .flow-part');
  if(await other.isVisible()){const {drawn,own}=await outline(other);expect(drawn,'a part off the path keeps its own outline').toBe(own);}
  const dark=await page.locator('.flow-edge-active').evaluateAll(edges=>edges.map(edge=>{const r=edge.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height};}));
  expect(dark.filter(inside).length,'the path\'s dark arrows are in the camera').toBeGreaterThanOrEqual(2);
  expect(errors).toEqual([]);
});
