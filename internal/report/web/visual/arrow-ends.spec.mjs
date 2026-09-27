import {test,expect} from '@playwright/test';

async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
}
async function pointAt(page,locator){
  const box=await locator.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:12});
}
const end=(page,id)=>page.locator(`.flow-connection-label[data-connection-label="${id}"]`);

// An arrow end that joins every part of the frame says so once, "all",
// instead of listing each number; an end that joins some keeps its numbers.
// Its card open, the parts behind it are outlined in place and the rest
// recedes (owner's 2a).
test('an end joining every part of its frame is one all mark that outlines them in place',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?both-parts');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('queue'));
  await settle(map);
  await page.locator('[data-map-zoom="0.8"]').click();await settle(map);
  await page.mouse.move(1430,890);
  await pointAt(page,page.locator('[data-frame-title="execution"]>strong'));
  await expect(map).toHaveAttribute('data-subject','execution');
  const all=end(page,'boundary:execution:in:requests');
  await expect(all).toHaveText('all');
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await expect(map).toHaveAttribute('data-subject','requests');
  await expect(end(page,'boundary:requests:out:execution'),'an end joining one of two parts keeps its number').toHaveText('1');
  await pointAt(page,page.locator('[data-frame-title="execution"]>strong'));
  await expect(map).toHaveAttribute('data-subject','execution');
  await testInfo.attach('journey-01 — One all mark where every part is behind the end',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await pointAt(page,all);
  await expect(page.locator('.flow-arrow-card')).toBeVisible();
  for(const id of ['queue','worker'])await expect(page.locator(`.react-flow__node[data-id="${id}"]`),`${id} is behind the end`).toHaveClass(/flow-node-focus/);
  await expect(page.locator('.react-flow__node[data-id="routes"]'),'the other end stays').not.toHaveClass(/flow-node-muted|flow-node-focus/);
  await expect(page.locator('.react-flow__node[data-id="auth"]'),'a part the end does not involve recedes').toHaveClass(/flow-node-muted/);
  await testInfo.attach('journey-02 — The parts behind the end outlined in place',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});
