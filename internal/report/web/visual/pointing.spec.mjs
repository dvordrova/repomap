import {test,expect} from '@playwright/test';

// The camera settles when two captures in a row agree.
async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
}
async function pointAt(page,locator){
  const box=await locator.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:12});
}
const look=(page,id)=>page.locator(`.react-flow__node[data-id="${id}"]`).evaluate(node=>{
  const card=node.firstElementChild,style=getComputedStyle(card);
  return {className:node.className,opacity:getComputedStyle(node).opacity,shadow:style.boxShadow,border:style.borderColor,
    frame:style.getPropertyValue('--flow-frame-color').trim(),frameWidth:style.getPropertyValue('--flow-frame-width').trim()};
});

// Pointing at a part makes that part the subject: its own arrows darken, the
// parts across them take the dark outline, and what is not involved recedes.
// Lifted to its area, a pointed part had lit the whole area's arrows, and a
// grey veil made the pointed parts look deader than their neighbours.
test('a pointed part is the subject, dark itself, and the rest recedes',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('routes'));
  await settle(map);
  await page.mouse.move(1430,890);
  await pointAt(page,page.locator('.react-flow__node[data-id="routes"] strong'));
  await expect(map).toHaveAttribute('data-subject','routes');
  const routes=await look(page,'routes');
  expect(routes.className).toContain('flow-node-focus');
  expect(routes.shadow,'no veil on the pointed part').toBe('none');
  expect(routes.border,'the pointed part takes the dark outline').toBe('rgb(36, 48, 68)');
  const auth=await look(page,'auth');
  expect(auth.className,'the part across its dark arrow').toContain('flow-node-connected');
  expect(auth.opacity).toBe('1');
  const worker=await look(page,'worker');
  expect(worker.className).toContain('flow-node-muted');
  expect(worker.opacity,'a part the emphasis does not involve recedes').toBe('0.4');
  const edges=await map.evaluate(map=>{
    const id=(from,to)=>map.visibleEdges.find(e=>e.from===from&&e.to===to).id;
    const at=edge=>{const g=document.querySelector(`[data-edge-ids~="${edge}"]`);return g&&{active:g.classList.contains('flow-edge-active'),opacity:getComputedStyle(g).opacity};};
    return {own:at(id('routes','auth')),sibling:at(id('queue','worker'))};
  });
  expect(edges.own.active).toBe(true);
  expect(edges.sibling.opacity,'an arrow the emphasis does not involve recedes').toBe('0.4');
  await testInfo.attach('journey-01 — The pointed part is dark, the rest recedes',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});

  // The area's title looks at the area: its frame darkens, its parts stay as they are.
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await expect(map).toHaveAttribute('data-subject','requests');
  const area=await look(page,'requests');
  expect(area.frame).toBe('#243044');
  expect(area.frameWidth).toBe('2.5px');
  for(const id of ['routes','auth']){
    const part=await look(page,id);
    expect(part.shadow,`${id}: no veil inside the pointed area`).toBe('none');
    expect(part.opacity).toBe('1');
  }
  await testInfo.attach('journey-02 — The pointed area\'s frame is dark',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});

// Zoomed into a part, its declarations fill the canvas; a drag that starts
// on them still moves the map.
test('a drag over a part\'s declarations pans the map',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?symbols');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('worker'));
  await settle(map);
  await page.locator('.react-flow__node[data-id="worker"] .flow-part-zoom').click();
  await settle(map);
  const part=page.locator('.react-flow__node[data-id="worker"]');
  await expect(part.locator('.flow-part-deep')).toBeVisible();
  const before=await map.evaluate(map=>map.captureViewport());
  const tile=await part.locator('.flow-symbol-head',{hasText:'processJob'}).boundingBox();
  const at={x:tile.x+tile.width/2,y:tile.y+tile.height/2};
  await page.mouse.move(at.x,at.y);await page.mouse.down();
  await page.mouse.move(at.x-120,at.y-40,{steps:10});await page.mouse.up();
  await settle(map);
  const after=await map.evaluate(map=>map.captureViewport());
  expect(after.x-before.x).toBeCloseTo(-120,0);
  expect(after.y-before.y).toBeCloseTo(-40,0);
  expect(page.context().pages()).toHaveLength(1);
  expect(errors).toEqual([]);
});

const overlap=(a,b)=>a.x<b.x+b.width&&b.x<a.x+a.width&&a.y<b.y+b.height&&b.y<a.y+a.height;

// A part's number opens its card when the pointer rests on it. The card
// stands outside the frame being read, level with the number and inside the
// canvas, and the pointer reaches it across the frame's border and empty
// canvas: the designer's paths 07c→07d and 08c→08d lost the card on the way,
// and the owner's 19.png lost it with the pointer on it. A click keeps it
// open; ✕, Escape or a click on empty canvas close it, and that click goes
// nowhere else.
test('a number\'s card is reachable, kept open by a click and closed by ✕, Escape or empty canvas',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('auth'));
  await settle(map);
  // Far enough back that the canvas has room beside the area for a card.
  await page.locator('[data-map-zoom="0.8"]').click();await settle(map);
  await expect(page.locator('.react-flow__node[data-id="routes"]')).toBeVisible();
  await page.mouse.move(1430,890);
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await expect(map).toHaveAttribute('data-subject','requests');
  const badge=page.locator('[data-badge="routes"]');
  // Crossing another number on the way opens nothing.
  const other=await page.locator('[data-badge="auth"]').boundingBox();
  await page.mouse.move(other.x-30,other.y+other.height/2);
  await page.mouse.move(other.x+other.width+30,other.y+other.height/2,{steps:3});
  expect(await page.locator('.flow-part-summary').count(),'a number crossed on the way opens nothing').toBe(0);
  await pointAt(page,badge);
  const card=page.locator('.flow-part-summary');
  await expect(card).toBeVisible();
  const subject=await map.getAttribute('data-subject');
  const frame=await page.locator('.react-flow__node[data-id="requests"]').boundingBox(),canvas=await page.locator('.flow-root').boundingBox();
  const box=await card.boundingBox(),handle=await badge.boundingBox();
  expect(overlap(box,frame),'the card covers none of the frame it explains').toBe(false);
  expect(box.x>=canvas.x&&box.y>=canvas.y&&box.x+box.width<=canvas.x+canvas.width&&box.y+box.height<=canvas.y+canvas.height,'the card stands inside the canvas').toBe(true);
  expect(Math.abs(box.y-handle.y)<1||Math.abs(box.y+box.height-handle.y-handle.height)<1,'the card is level with its number').toBe(true);
  await testInfo.attach('journey-01 — The number\'s card outside its frame',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  // To the card, across the frame's border and the canvas beyond it.
  await page.mouse.move(box.x+box.width/2,box.y+Math.min(40,box.height/2),{steps:30});
  await page.waitForTimeout(500);
  await expect(card).toBeVisible();
  await expect(map,'the way to the card changes nothing it crosses').toHaveAttribute('data-subject',subject);
  // Kept open by a click, it stays when the pointer leaves, and Escape closes it.
  await page.mouse.click(box.x+box.width/2,box.y+Math.min(40,box.height/2));
  await expect(card.locator('.flow-card-close')).toBeVisible();
  await page.mouse.move(1430,890,{steps:10});await page.waitForTimeout(500);
  await expect(card).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(card).toHaveCount(0);
  // Kept open from its number, the ✕ closes it.
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await badge.click();
  await expect(card.locator('.flow-card-close')).toBeVisible();
  await card.locator('.flow-card-close').click();
  await expect(card).toHaveCount(0);
  // A click on empty canvas closes it first and goes nowhere.
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await badge.click();
  await expect(card).toBeVisible();
  const reading=await page.locator('[data-reading-title]').textContent(),camera=await map.evaluate(map=>map.captureViewport());
  await page.mouse.click(canvas.x+8,canvas.y+canvas.height-8);
  await expect(card).toHaveCount(0);
  await expect(page.locator('[data-reading-title]')).toHaveText(reading);
  expect(await map.evaluate(map=>map.captureViewport())).toEqual(camera);
  expect(errors).toEqual([]);
});
