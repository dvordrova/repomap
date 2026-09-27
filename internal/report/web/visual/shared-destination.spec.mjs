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

// Entered, one plain tile had read only "gethostbyname", its group's heading
// below the camera; titled one by one, Redis's open DNS tiles then said "DNS
// resolver" three times, each over a frame half empty, above an empty band.
test('an entered tile opens its display group under the one heading, each tile as large as its calls',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?shared-destination');
  await expect(page.locator('[data-map]')).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="dns-backend"]').click();
  const tiles=['dns-front','dns-backend'];
  for(const id of tiles){
    await expect(page.locator(`.react-flow__node[data-id="${id}-call"]`),`${id} opens with the entered tile`).toBeVisible();
    await expect(page.locator(`[data-frame-title="${id}"]`),`${id} repeats no title`).toHaveCount(0);
  }
  const heading=page.locator('[data-group-heading]');
  await expect(heading).toHaveText('DNS resolver');
  await page.waitForTimeout(600);
  const look=await page.evaluate(ids=>{
    const box=el=>el.getBoundingClientRect().toJSON();
    const heading=document.querySelector('[data-group-heading]'),range=document.createRange();range.selectNodeContents(heading);
    // The size a text is drawn at: its font size times every scale above it.
    const drawn=(el,sized)=>parseFloat(getComputedStyle(el).fontSize)*sized.getBoundingClientRect().width/sized.offsetWidth;
    const call=document.querySelector(`.react-flow__node[data-id="${ids[0]}-call"] .flow-part`);
    return {stage:box(document.querySelector('.flow-root')),group:box(document.querySelector('.react-flow__node[data-id^="display-group:"]')),
      text:range.getBoundingClientRect().toJSON(),headingSize:drawn(heading,heading),callSize:drawn(call.querySelector('strong'),call),
      tiles:ids.map(id=>({frame:box(document.querySelector(`.react-flow__node[data-id="${id}"]`)),call:box(document.querySelector(`.react-flow__node[data-id="${id}-call"]`))}))};
  },tiles);
  const inView=r=>r.left>=look.stage.x-.5&&r.right<=look.stage.x+look.stage.width+.5&&r.top>=look.stage.y-.5&&r.bottom<=look.stage.y+look.stage.height+.5;
  expect(inView(look.text),'the group\'s heading is in view').toBe(true);
  // The camera frames the group, not the one tile entered.
  const centre=r=>({x:(r.left+r.right)/2,y:(r.top+r.bottom)/2}),group=centre(look.group),stage={x:look.stage.x+look.stage.width/2,y:look.stage.y+look.stage.height/2};
  expect(inView(look.group)&&Math.abs(group.x-stage.x)<=2&&Math.abs(group.y-stage.y)<=2,
    `the group stands whole in the middle of the canvas: ${JSON.stringify(look.group)} in ${JSON.stringify(look.stage)}`).toBe(true);
  expect(Math.abs(look.headingSize-look.callSize),`the heading reads at ${look.headingSize.toFixed(1)}px beside ${look.callSize.toFixed(1)}px calls`).toBeLessThan(.5);
  for(const [i,{frame,call}] of look.tiles.entries()){
    expect(inView(call),`${tiles[i]}'s call is in view`).toBe(true);
    const left=call.left-frame.left,bottom=frame.bottom-call.bottom;
    expect(bottom,`${tiles[i]} holds its call with its insets alone`).toBeLessThanOrEqual(left+1);
  }
  // The frame ends past its heading, on the band's side, no farther than its
  // tiles stand from its other edges.
  const inset=Math.min(...look.tiles.map(tile=>tile.frame.left))-look.group.left;
  const beside=look.text.left>=Math.max(...look.tiles.map(tile=>tile.frame.right));
  expect(beside?look.group.right-look.text.right:look.group.bottom-look.text.bottom,'the group\'s frame ends past its heading').toBeLessThanOrEqual(inset+1);
  expect(errors).toEqual([]);
});
