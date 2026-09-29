import {test,expect} from '@playwright/test';

// The camera settles when two captures in a row agree.
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
const look=(page,id)=>page.locator(`.react-flow__node[data-id="${id}"]`).evaluate(node=>{
  const card=node.firstElementChild,style=getComputedStyle(card);
  // The card's own outline, as no emphasis draws it.
  const probe=document.createElement('div');probe.className=card.className;node.parentElement.appendChild(probe);
  const ownBorder=getComputedStyle(probe).borderColor;probe.remove();
  return {className:node.className,opacity:getComputedStyle(node).opacity,shadow:style.boxShadow,border:style.borderColor,ownBorder,
    frame:style.getPropertyValue('--flow-frame-color').trim(),frameWidth:style.getPropertyValue('--flow-frame-width').trim()};
});

// Pointing at a part makes that part the subject: its own arrows darken and
// the parts across them take the dark outline. Nothing else changes: moving
// across Redis's Data type commands had receded every other part on each
// tile and restored them in the gaps, and the area flickered. Lifted to its
// area, a pointed part had lit the whole area's arrows, and a grey veil made
// the pointed parts look deader than their neighbours.
test('a pointed part is the subject, dark itself, and nothing recedes',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('routes'));
  await settle(map);
  await page.mouse.move(1430,890);
  const quietArea=await look(page,'requests');
  await pointAt(page,page.locator('.react-flow__node[data-id="routes"] strong'));
  await expect(map).toHaveAttribute('data-subject','routes');
  const routes=await look(page,'routes');
  expect(routes.className).toContain('flow-node-focus');
  expect(routes.shadow,'no veil on the pointed part').toBe('none');
  const auth=await look(page,'auth');
  expect(auth.className,'the part across its dark arrow').toContain('flow-node-connected');
  expect(auth.opacity).toBe('1');
  const worker=await look(page,'worker');
  expect(worker.className).not.toContain('flow-node-muted');
  expect(worker.opacity,'a part the pointer does not connect stays as it is').toBe('1');
  expect(routes.border,'the pointed part takes the dark outline').not.toBe(routes.ownBorder);
  expect(worker.border,'a part the pointer does not connect keeps its own').toBe(worker.ownBorder);
  const edges=await map.evaluate(map=>{
    const id=(from,to)=>map.visibleEdges.find(e=>e.from===from&&e.to===to).id;
    const at=edge=>{const g=document.querySelector(`[data-edge-ids~="${edge}"]`);return g&&{active:g.classList.contains('flow-edge-active'),opacity:getComputedStyle(g).opacity};};
    return {own:at(id('routes','auth')),sibling:at(id('queue','worker'))};
  });
  expect(edges.own.active).toBe(true);
  expect(edges.sibling.opacity,'an arrow the pointer does not darken stays as it is').toBe('1');
  await testInfo.attach('journey-01 — The pointed part is dark, the rest stays',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});

  // The area's title looks at the area: its frame darkens, its parts stay as they are.
  await pointAt(page,page.locator('[data-frame-title="requests"]>strong'));
  await expect(map).toHaveAttribute('data-subject','requests');
  const area=await look(page,'requests');
  expect([area.frame,area.frameWidth],'its frame darkens').not.toEqual([quietArea.frame,quietArea.frameWidth]);
  for(const id of ['routes','auth']){
    const part=await look(page,id);
    expect(part.shadow,`${id}: no veil inside the pointed area`).toBe('none');
    expect(part.opacity).toBe('1');
  }
  const inner=await map.evaluate(map=>getComputedStyle(document.querySelector(`[data-edge-ids~="${map.visibleEdges.find(e=>e.from==='routes'&&e.to==='auth').id}"]`)).opacity);
  expect(inner,'an arrow between the area\'s own parts stays as it is').toBe('1');
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

async function deepWorker(page,map){
  await page.goto('/?symbols');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('worker'));
  await settle(map);
  await page.locator('.react-flow__node[data-id="worker"] .flow-part-zoom').click();
  await settle(map);
  return page.locator('.react-flow__node[data-id="worker"]');
}
const tileOf=(part,name)=>part.locator('.flow-symbol-block > *',{hasText:new RegExp('^'+name)}).first();
// The worker's arrows drawn dark. An outer route drawn once for several
// arrows is dark when one of them is; only the worker's own are asked about.
const darkArrows=map=>map.evaluate(map=>[...new Set([...document.querySelectorAll('.flow-edge-active')].flatMap(g=>g.dataset.edgeIds.split(' '))
  .map(id=>map.visibleEdges.find(e=>e.id===id)).filter(e=>e.from==='worker'||e.to==='worker').map(e=>`${e.from}>${e.to}`))].sort());

// The magnifier enters where the declarations read at their own size, every
// name whole: Redis's Persistence opened at its title's scale with no
// declaration drawn, and its names were cut to "rewriteAppendOnlyFil…".
test('the magnifier enters at the declarations\' reading scale with every name whole',async({page},testInfo)=>{
  const map=page.locator('[data-map]');
  const part=await deepWorker(page,map);
  await expect(part.locator('.flow-part-deep')).toBeVisible();
  const tiles=await part.locator('.flow-symbol-block > :first-child').evaluateAll(rows=>rows.map(row=>{
    const range=document.createRange();range.selectNodeContents(row.firstChild);
    const name=range.getBoundingClientRect(),box=row.getBoundingClientRect();
    return {name:row.firstChild.textContent,size:parseFloat(getComputedStyle(row).fontSize)*box.height/row.offsetHeight,whole:name.right<=box.right-4};
  }));
  expect(tiles.map(tile=>tile.name)).toContain('retryWithExponentialBackoffPolicy');
  for(const tile of tiles){
    expect(tile.size,`${tile.name} reads at its own size`).toBeGreaterThan(12);
    expect(tile.whole,`${tile.name} is not cut`).toBe(true);
  }
  await expect(part.locator('.flow-symbol-more')).toHaveCount(0);
  await testInfo.attach('journey-01 — Entered at the declarations\' reading scale',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});

// A declaration is chosen on its tile: pointing darkens only its own arrows,
// a click reads its part with it named, and a declaration the reading names
// elsewhere (Find, a restored visit) is the one chosen and, out of sight,
// shown with its whole part. A click had opened the code in a new tab, or
// bubbled to the part already selected and changed nothing.
test('a tile points at and chooses its own declaration',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=page.locator('[data-map]');
  const part=await deepWorker(page,map);
  await page.mouse.move(1430,890);
  await pointAt(page,tileOf(part,'processJob'));
  await expect.poll(()=>darkArrows(map)).toEqual(['queue>worker']);
  await pointAt(page,tileOf(part,'save'));
  await expect.poll(()=>darkArrows(map)).toEqual(['worker>save-jobs']);
  const tile=await tileOf(part,'processJob').boundingBox();
  await page.mouse.move(tile.x+tile.width/2,tile.y+tile.height/2,{steps:6});
  const camera=await map.evaluate(map=>map.captureViewport());
  await page.mouse.click(tile.x+tile.width/2,tile.y+tile.height/2);
  await settle(map);
  expect(page.context().pages()).toHaveLength(1);
  await expect(page.locator('[data-reading-title]')).toHaveText('Processing worker');
  await expect(tileOf(part,'processJob')).toHaveClass(/flow-symbol-chosen/);
  // The first click reads; the tile in sight keeps the camera (owner,
  // 2026-09-28).
  expect(await map.evaluate(map=>map.captureViewport()),'a tile in sight is only marked').toEqual(camera);
  await page.mouse.move(1430,890);
  await expect.poll(()=>darkArrows(map)).toEqual(['queue>worker']);
  await testInfo.attach('journey-01 — processJob chosen in place',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  // The reading names another declaration, as Find does: in sight, it is
  // marked where it stands; out of sight, it is shown with its part, the
  // part that fits framed whole and centred (owner, 2026-09-29: a name
  // chosen in the column had zoomed to the tile's own size).
  await map.evaluate(map=>{map.explorerMember={owner:'worker',name:'save',key:'#worker.go-30',href:'#worker.go-30'};map.dispatchEvent(new Event('repomap:reading'));});
  await settle(map);
  await expect(tileOf(part,'save')).toHaveClass(/flow-symbol-chosen/);
  expect(await map.evaluate(map=>map.captureViewport()),'a named tile in sight is only marked').toEqual(camera);
  const canvas=await page.locator('.flow-root').boundingBox();
  await map.evaluate((map,width)=>{const v=map.captureViewport();map.restoreReadingState({scope:'worker',viewport:{...v,fit:false,x:v.x+width*2}});},canvas.width);await settle(map);
  await map.evaluate(map=>{map.explorerMember={owner:'worker',name:'processJob',key:'#worker.go-3',href:'#worker.go-3'};map.dispatchEvent(new Event('repomap:reading'));});
  await settle(map);
  await expect(tileOf(part,'processJob')).toHaveClass(/flow-symbol-chosen/);
  const found=await tileOf(part,'processJob').boundingBox(),whole=await part.boundingBox();
  const inside=box=>box.x>=canvas.x-.5&&box.y>=canvas.y-.5&&box.x+box.width<=canvas.x+canvas.width+.5&&box.y+box.height<=canvas.y+canvas.height+.5;
  expect(inside(found),'the chosen tile is in sight').toBe(true);
  expect(inside(whole),'its part stands whole').toBe(true);
  expect(Math.abs(whole.x+whole.width/2-canvas.x-canvas.width/2)).toBeLessThan(2);
  expect(Math.abs(whole.y+whole.height/2-canvas.y-canvas.height/2)).toBeLessThan(2);
  expect(errors).toEqual([]);
});

// Zoomed in on a part, the reader asks for the whole map to look around:
// what they were reading stays.
test('show whole map keeps the reading and its emphasis',async({page})=>{
  const map=page.locator('[data-map]');
  await page.goto('/');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('auth'));
  await settle(map);
  await page.locator('.react-flow__node[data-id="auth"] strong').click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Authentication and permissions');
  await page.mouse.move(1430,890);
  await page.getByRole('button',{name:'Show whole map',exact:true}).click();
  await expect.poll(async()=>(await map.evaluate(map=>map.captureViewport()))?.fit).toBe(true);
  await expect(page.locator('.flow-location')).toHaveText('System map');
  await expect(page.locator('[data-reading-title]')).toHaveText('Authentication and permissions');
  await expect(map).toHaveAttribute('data-emphasis','selection');
});

// Titled with its component's name, Redis's input collection read as a
// second redis-server; pinched open, it was a wall of 95 tiles under group
// names too small to read. It is headed Inputs, opens to its inputs grouped
// by their handler's part, each group named, and a group opens to its inputs
// once they read.
test('the input collection is headed Inputs and opens to its groups before its inputs',async({page},testInfo)=>{
  const map=page.locator('[data-map]');
  await page.goto('/?many-inputs');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const overview=page.locator('[data-component-overview="backend-inputs"]');
  await expect(overview.locator('.flow-component-overview-heading strong')).toHaveText('Inputs');
  await expect(page.locator('[data-zoom-into="backend-inputs"]')).toHaveAttribute('aria-label','Zoom into Inputs · Job processing service');
  const group=page.locator('[data-summary-area="backend-inputs~routes"]'),tile=page.locator('.react-flow__node[data-id="create"]');
  const pinch=async()=>{
    const box=await page.locator('.react-flow__node[data-id="backend-inputs"]').boundingBox(),canvas=await page.locator('.flow-root').boundingBox();
    const x=Math.min(Math.max(box.x+box.width/2,canvas.x+10),canvas.x+canvas.width-10),y=Math.min(Math.max(box.y+box.height/2,canvas.y+10),canvas.y+canvas.height-10);
    const zoom=await map.evaluate(map=>map.captureViewport().zoom);
    await page.mouse.move(x,y);
    await page.keyboard.down('Control');try{await page.mouse.wheel(0,-6);}finally{await page.keyboard.up('Control');}
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeGreaterThan(zoom);
  };
  for(let step=0;step<60&&await overview.count();step++)await pinch();
  await expect(overview).toHaveCount(0);
  await expect(group).toBeVisible();
  await expect(group.locator('strong')).toHaveText('HTTP handlers');
  await expect(tile).toHaveCSS('visibility','hidden');
  await expect(page.locator('.flow-location')).toContainText('Inputs · Job processing service');
  await testInfo.attach('journey-01 — The inputs grouped by their handler\'s part',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  for(let step=0;step<60&&await group.count();step++)await pinch();
  await expect(group).toHaveCount(0);
  await expect(tile).toHaveCSS('visibility','visible');
  await expect(page.locator('[data-frame-title="backend-inputs~routes"]')).toBeVisible();
  await testInfo.attach('journey-02 — A group opened to its inputs',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});

// Frames that share one destination's text stand in one group under one
// heading. The group is involved when one of its frames is: choosing a
// system whose resolver is dark had left the group's frame receded around
// it and its heading at full strength when nothing of it was involved.
test('a group of frames sharing one destination recedes with them and stays with them',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const map=page.locator('[data-map]');
  await page.goto('/?shared-destination');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const heading=page.locator('[data-group-heading]'),group=page.locator('.react-flow__node[data-id^="display-group:"]');
  const resolver=page.locator('.react-flow__node[data-id="dns-backend"]');
  await page.mouse.move(1430,890);
  await map.evaluate(map=>map.restoreReadingState({scope:'backend'}));
  await expect(resolver).toHaveCSS('opacity','1');
  await expect(group,'the group of an involved frame stays').toHaveCSS('opacity','1');
  await expect(heading).toHaveCSS('opacity','1');
  await map.evaluate(map=>map.restoreReadingState({scope:'api'}));
  const opacity=locator=>locator.evaluate(element=>Number(getComputedStyle(element).opacity));
  await expect.poll(()=>opacity(resolver)).toBeLessThan(1);
  await expect.poll(()=>opacity(group),'a group nothing involves recedes').toBeLessThan(1);
  await expect.poll(()=>opacity(heading),'and its heading with it').toBeLessThan(1);
  // The pointer brings forward what it outlines and recedes nothing more.
  await pointAt(page,page.locator('[data-component-overview="backend"]'));
  await expect(map).toHaveAttribute('data-subject','backend');
  await expect(resolver).toHaveCSS('opacity','1');
  await expect(group).toHaveCSS('opacity','1');
  await expect(heading).toHaveCSS('opacity','1');
  expect(errors).toEqual([]);
});
