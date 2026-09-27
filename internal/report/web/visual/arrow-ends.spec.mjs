import {test,expect} from '@playwright/test';

async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
}
// A fifth out about the canvas's centre, one ctrl+wheel tick: the "−"
// control steps out a whole level now.
async function zoomOutAFifth(page,map){
  const canvas=await page.locator('.flow-root').boundingBox();
  await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
  await page.keyboard.down('Control');
  try{await page.mouse.wheel(0,16.1);}finally{await page.keyboard.up('Control');}
  await page.mouse.move(1430,890);
  await settle(map);
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
  await zoomOutAFifth(page,map);
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

// A click on an arrow end reads its frame's connections in the column,
// that connection open with the calls its card lists; the card's own
// click keeps the card (owner's 3b).
test('a click on an arrow end opens its frame\'s connection in the reading',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?both-parts&symbols');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('queue'));
  await settle(map);
  await zoomOutAFifth(page,map);
  await page.mouse.move(1430,890);
  await pointAt(page,page.locator('[data-frame-title="execution"]>strong'));
  const all=end(page,'boundary:execution:in:requests');
  await pointAt(page,all);
  const card=page.locator('.flow-arrow-card');
  await expect(card).toBeVisible();
  await expect(card.locator('.flow-card-title')).toHaveText('Request handling→Job execution');
  const camera=await map.evaluate(map=>map.captureViewport());
  await all.locator('button').click();
  await expect(map).toHaveAttribute('data-opened-connection','execution in:requests');
  await expect(card,'the click reads the connection instead of keeping the card').toHaveCount(0);
  expect(await map.evaluate(map=>map.captureViewport()),'the camera stays').toEqual(camera);
  const open=page.locator('[data-reading-connections] details[open]');
  await expect(open).toHaveCount(1);
  await expect(open).toHaveAttribute('data-connection-key','in:requests');
  await expect(open.locator('summary')).toContainText('Request handling');
  await expect(open.locator('.flow-card-pair')).toHaveText(['→ Job scheduling1','→ Processing worker1']);
  await expect(page.locator('[data-reading-connections] details:not([open])').first(),'the frame\'s other connections stay closed under it').toBeAttached();
  await testInfo.attach('journey-01 — The arrow end opens its connection in the reading',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});

// A name in a connection of the reading column reads that declaration in
// the report, as its tile does: Redis's "anetTcpGeneri…" opened GitHub for a
// reader who meant to read it. The row's code is its own explicit link, and
// a modifier-click on a name still opens the code it linked to.
test('a name in the reading\'s connection reads its declaration, and its code is an explicit link',async({page,context})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?both-parts&symbols&reading-names');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('queue'));
  await settle(map);
  await zoomOutAFifth(page,map);
  await page.mouse.move(1430,890);
  await pointAt(page,page.locator('[data-frame-title="execution"]>strong'));
  const all=end(page,'boundary:execution:in:requests');
  await pointAt(page,all);
  await all.locator('button').click();
  const row=page.locator('[data-reading-connections] details[open] .flow-card-row',{hasText:'processJob'});
  await expect(row).toHaveText('handleCreate→processJobOpen code ↗');
  await expect(row.locator('a',{hasText:'Open code ↗'})).toHaveAttribute('href','#routes.go-20');
  await expect(row.locator('a,button',{hasText:'handleCreate'}),'a caller the report holds no declaration for is only named').toHaveCount(0);
  const pages=[];context.on('page',opened=>pages.push(opened));
  await pointAt(page,row.locator('a',{hasText:/^processJob$/}));
  await page.mouse.down();await page.mouse.up();
  await expect(map).toHaveAttribute('data-chosen','worker #worker.go-3');
  await page.waitForTimeout(300);
  expect(pages,'the click read the declaration instead of opening its code').toHaveLength(0);
  await map.evaluate(map=>{delete map.dataset.chosen;});
  const code=context.waitForEvent('page');
  await page.keyboard.down('Shift');await page.mouse.down();await page.mouse.up();await page.keyboard.up('Shift');
  await code;
  expect(await map.getAttribute('data-chosen'),'a modifier-click opens the code instead').toBeNull();
  expect(errors).toEqual([]);
});

// Where an arrow meets the box it points into, 5px back from the tip along
// the arrow: a point on the drawn head.
const head=(page,edge,end='end')=>page.evaluate(({edge,end})=>{
  const path=document.querySelector(`.flow-edge[data-edge-ids~="${edge}"] path:not(.flow-edge-casing)`);
  const ctm=path.getScreenCTM(),length=path.getTotalLength(),scale=Math.hypot(ctm.a,ctm.b);
  const at=l=>{const p=path.getPointAtLength(l);return {x:p.x*ctm.a+p.y*ctm.c+ctm.e,y:p.x*ctm.b+p.y*ctm.d+ctm.f};};
  const tip=end==='end'?at(length):at(0),back=end==='end'?at(length-8/scale):at(8/scale),d=Math.hypot(back.x-tip.x,back.y-tip.y);
  return {x:tip.x+(back.x-tip.x)/d*5,y:tip.y+(back.y-tip.y)/d*5};
},{edge,end});
// A real pointer walks there and rests past the card's intent.
async function restAt(page,point){await page.mouse.move(point.x,point.y,{steps:18});await page.waitForTimeout(400);}
// Onto the card, resting past its linger: the card stays.
async function ontoCard(page){
  const card=page.locator('.flow-arrow-card .flow-connection-calls');
  const box=await card.boundingBox();
  await page.mouse.move(box.x+Math.min(80,box.width/2),box.y+Math.min(30,box.height/2),{steps:15});await page.waitForTimeout(700);
  await expect(card,'the pointer reached the card and it stayed').toBeVisible();
}

// Redis's geometry: entering one component opens its neighbour too, and
// the component's numbers stand on its own border. Chosen, it numbers its
// areas; the pointer crosses its space from an area to a number and the
// number is still there, its card opens and the pointer reaches the card.
// Read by its areas alone, the component's own space was in no frame: its
// numbers vanished on the way, and with two components open none stood at
// all (the tester's 44–47).
test('a component beside another keeps its numbers while the pointer crosses it to them',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?symbols');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const overview=await page.locator('[data-component-overview="backend"] strong').boundingBox();
  await page.mouse.click(overview.x+overview.width/2,overview.y+overview.height/2);await settle(map);
  expect((await map.evaluate(map=>map.captureViewport())).openComponents,'entering the backend opens the front too').toEqual(['front','backend']);
  await page.mouse.move(1430,890);
  const chip=end(page,'boundary:backend:in:api');
  await expect(chip,'the chosen component numbers its areas on its border').toHaveText('1');
  const at=await chip.boundingBox().then(box=>({x:box.x+box.width/2,y:box.y+box.height/2}));
  // Pointed at, an open area numbers its own parts instead; on the way out
  // of it the component's numbers stand again.
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await expect(map).toHaveAttribute('data-subject','requests');
  await restAt(page,at);
  const card=page.locator('.flow-arrow-card');
  await expect(card).toBeVisible();
  await expect(card.locator('.flow-card-title')).toHaveText('Backend API→Job processing service');
  await testInfo.attach('journey-01 — The component\'s number reached across its space',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await ontoCard(page);
  const camera=await map.evaluate(map=>map.captureViewport());
  await chip.locator('button').click();
  await expect(map).toHaveAttribute('data-opened-connection','backend in:api');
  expect(await map.evaluate(map=>map.captureViewport()),'the camera stays').toEqual(camera);
  expect(errors).toEqual([]);
});

// An arrow's head where it meets a frame is its connection's handle, at the
// whole map and inside a component: resting on it opens the card, the
// pointer reaches the card, and a click reads that connection in the column
// with the camera still. The head had no target: the tester's rest opened
// nothing and a click fell through to the frame underneath.
test('an arrowhead at a frame opens its connection\'s card and a click reads it',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?symbols');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const edge=(from,to)=>map.evaluate((map,[from,to])=>map.visibleEdges.find(e=>e.from===from&&e.to===to).id,[from,to]);
  const card=page.locator('.flow-arrow-card');
  // The whole map: the API's arrow ends at the closed backend.
  await page.mouse.move(1430,890);
  const atBackend=await head(page,await edge('get','routes'));
  expect(await page.evaluate(({x,y})=>document.elementFromPoint(x,y).classList.contains('react-flow__pane'),atBackend),'the head stands on empty canvas').toBe(true);
  await restAt(page,atBackend);
  await expect(card).toBeVisible();
  await expect(card.locator('.flow-card-title')).toHaveText('Backend API→Job processing service');
  await testInfo.attach('journey-01 — The arrowhead at the closed backend opens its card',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await ontoCard(page);
  // Inside the backend: the head where the worker's arrow meets the database.
  const overview=await page.locator('[data-component-overview="backend"] strong').boundingBox();
  await page.mouse.click(overview.x+overview.width/2,overview.y+overview.height/2);await settle(map);
  await zoomOutAFifth(page,map);
  await page.mouse.move(1430,890);await page.keyboard.press('Escape');
  await pointAt(page,page.locator('[data-frame-title="execution"]>strong'));
  const atDatabase=await head(page,await edge('worker','save-jobs'));
  await restAt(page,atDatabase);
  await expect(card).toBeVisible();
  await expect(card.locator('.flow-card-title')).toHaveText('Job processing service→PostgreSQL');
  await ontoCard(page);
  await page.mouse.move(atDatabase.x,atDatabase.y,{steps:12});
  const camera=await map.evaluate(map=>map.captureViewport());
  await page.mouse.click(atDatabase.x,atDatabase.y);
  await expect(map,'the click reads the connection, not the frame under the head').toHaveAttribute('data-opened-connection','backend out:postgres');
  expect(await map.evaluate(map=>map.captureViewport()),'the camera stays').toEqual(camera);
  await expect(page.locator('[data-reading-connections] details[open]')).toHaveAttribute('data-connection-key','out:postgres');
  await testInfo.attach('journey-02 — The click on the head reads its connection',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});
