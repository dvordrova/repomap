import {test,expect} from '@playwright/test';

// Redis's first screen drew "DNS resolver" three times side by side, each
// cut to "DNS resolve" over a stray letter, and the arrows ran through the
// text once it moved into the group's frame.
test('frames naming one destination read it once beside plain tiles, and no heading splits a word',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?shared-destination');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const heading=page.locator('[data-group-heading]');
  await expect(heading).toHaveCount(1);await expect(heading).toHaveText('DNS resolver');
  const box=element=>element.boundingBox();
  const group=await box(page.locator('.react-flow__node[data-id^="display-group:"]')),text=await heading.evaluate(el=>{
    const range=document.createRange();range.selectNodeContents(el);return range.getBoundingClientRect().toJSON();});
  expect(text.left>=group.x&&text.right<=group.x+group.width&&text.top>=group.y&&text.bottom<=group.y+group.height,'the heading stands in its group').toBe(true);
  for(const id of ['dns-front','dns-backend']){
    await expect(page.locator(`[data-component-overview="${id}"] .flow-component-overview-heading`),`${id} repeats no heading`).toHaveCount(0);
    await expect(page.locator(`[data-zoom-into="${id}"]`),`${id} keeps its zoom mark`).toBeVisible();
    const tile=await box(page.locator(`.react-flow__node[data-id="${id}"]`));
    expect(text.right<=tile.x||text.left>=tile.x+tile.width||text.bottom<=tile.y||text.top>=tile.y+tile.height,`the heading stays off ${id}`).toBe(true);
  }
  const split=await page.locator('[data-component-overview] strong,[data-group-heading]').evaluateAll(elements=>elements.flatMap(el=>{
    const node=el.firstChild,range=document.createRange();if(!node)return [];
    return [...node.textContent.matchAll(/\S+/g)].filter(word=>{range.setStart(node,word.index);range.setEnd(node,word.index+word[0].length);
      return range.getClientRects().length>1;}).map(word=>`${el.textContent}: ${word[0]}`);
  }));
  expect(split,'every heading keeps its words whole').toEqual([]);
  expect(errors).toEqual([]);
});

// Entered, a plain tile opened under no title of its own read only
// "gethostbyname"; its group's heading stood below the camera.
test('an entered tile of a display group names its destination on the canvas',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?shared-destination');
  await expect(page.locator('[data-map]')).toHaveAttribute('data-fixture-ready','true');
  const mark=page.locator('[data-zoom-into="dns-backend"]');
  await mark.click();
  await expect(page.locator('[data-component-overview="dns-backend"]')).toHaveCount(0);
  const title=page.locator('[data-frame-title="dns-backend"]');
  await expect(title).toHaveText('DNS resolver');
  const [text,stage]=await Promise.all([title.evaluate(el=>{const range=document.createRange();range.selectNodeContents(el);return range.getBoundingClientRect().toJSON();}),
    page.locator('.flow-root').boundingBox()]);
  expect(text.left>=stage.x&&text.right<=stage.x+stage.width&&text.top>=stage.y&&text.bottom<=stage.y+stage.height,'the entered tile\'s name is in view').toBe(true);
  expect(errors).toEqual([]);
});
