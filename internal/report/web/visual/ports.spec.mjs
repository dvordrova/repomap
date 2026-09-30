import {test,expect} from '@playwright/test';

// A program entered stands its Inputs and Outside on its own border as
// ports (canvas.jsx PortPill; owner, 2026-09-30: variant B), the whole map
// keeping their frames: its Inputs frame is no longer drawn, each input
// kind is an icon named in one line when pointed at or reached by the
// keyboard, and a click on it reads that kind, the camera staying.
test('an entered program stands its inputs as ports on its border, named and read from there',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await expect(map).not.toHaveClass(/flow-initializing/);
  await expect(page.locator('[data-port]'),'the whole map keeps its frames').toHaveCount(0);
  await page.locator('[data-zoom-into="backend"]').first().click();
  const pill=page.locator('[data-port="inputs"][data-port-frame="backend-inputs"]');
  await expect(pill).toBeVisible();
  await expect(page.locator('.react-flow__node[data-id="backend-inputs"]')).toBeHidden();
  const icons=pill.locator('[data-port-end]');
  await expect(icons).not.toHaveCount(0);
  const before=await map.evaluate(map=>JSON.stringify(map.captureViewport()));
  await icons.first().hover();
  const tip=pill.locator('.flow-port-tip');
  await expect(tip).toHaveCount(1);
  expect((await tip.innerText()).trim()).toMatch(/^[^\n]+$/);
  await icons.first().click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Job processing service');
  await page.mouse.move(2,2);
  await expect(tip).toHaveCount(0);
  await pill.focus();await page.keyboard.press('Tab');
  await expect(icons.first()).toBeFocused();
  await expect(tip,'the keyboard names it too').toHaveCount(1);
  expect(await map.evaluate(map=>JSON.stringify(map.captureViewport())),'the camera stays').toBe(before);
  // Its Outside stands as dots on its other side. At rest no line reaches
  // them; pointed at, each draws its lines from the parts calling it
  // (owner: hover answers what is this). Nothing beyond the program is drawn.
  const dots=page.locator('[data-port="outside"] [data-port-end]');
  await expect(dots).not.toHaveCount(0);
  const ids=await dots.evaluateAll(items=>items.map(item=>item.dataset.portEnd));
  const lines=()=>page.evaluate(()=>[...document.querySelectorAll('g.flow-edge[data-edge-ends]')].map(g=>g.dataset.edgeEnds.split(' ')));
  await page.mouse.move(2,2);
  expect((await lines()).some(ends=>ends.some(end=>ids.includes(end))),'no line to a dot at rest').toBe(false);
  for(const id of ids){
    await page.locator(`[data-port-end="${id}"]`).hover();
    await expect.poll(async()=>(await lines()).some(ends=>ends.includes(id)),{message:`a line reaches ${id} pointed at`}).toBe(true);
  }
  const program=await page.evaluate(()=>{const nodes=document.querySelector('[data-map]').flowGeometry().nodes,parent=new Map(nodes.map(n=>[n.id,n.parentId]));
    const ports=new Set([...document.querySelectorAll('[data-port-end]')].map(item=>item.dataset.portEnd));
    return [...document.querySelectorAll('g.flow-edge[data-edge-ends]')].flatMap(g=>g.dataset.edgeEnds.split(' ')).filter(id=>parent.has(id)&&!ports.has(id)).map(id=>{while(parent.get(id))id=parent.get(id);return id;});});
  expect(new Set(program),'every drawn box end is the program\'s').toEqual(new Set(['backend']));
  expect(errors).toEqual([]);
});
