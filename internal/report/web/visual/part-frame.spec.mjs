import {test,expect} from '@playwright/test';

async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
}

// Zoomed into a part, the part is the frame the reader looks at (owner,
// 2026-09-28): redis-cli's Command line client drew no arrow out to Core
// infrastructure and its column still read redis-cli. Each connection to a
// part or area beside it leaves the part's own border, and the column reads
// the part; "−" steps out and the column reads the frame it brings.
test('a part zoomed into draws its connections from its own border and the column follows the zoom',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?symbols&both-parts');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('worker'));await settle(map);
  await page.locator('[aria-label="Zoom into Processing worker"]').click();await settle(map);
  await expect(page.locator('.flow-part-deep')).toHaveCount(1);
  await expect(map,'the magnifier has the column read the part').toHaveAttribute('data-followed','worker');
  const worker=await page.locator('.react-flow__node[data-id="worker"]').boundingBox(),canvas=await page.locator('.flow-root').boundingBox();
  expect(worker.x>=canvas.x-1&&worker.x+worker.width<=canvas.x+canvas.width+1&&worker.y>=canvas.y-1&&worker.y+worker.height<=canvas.y+canvas.height+1,'the part is framed whole').toBe(true);
  // No plaque stands on its border: the arrows are the handles.
  await expect(page.locator('.flow-connection-label,.flow-boundary-label')).toHaveCount(0);
  // An arrow from Request handling reaches the part's own border: the route
  // between the two areas ends at Job execution's, so the part draws a
  // short arrow of its own there.
  const ends=await map.evaluate(map=>{
    const edge=map.visibleEdges.find(e=>e.from==='routes'&&e.to==='worker');
    const m=new DOMMatrixReadOnly(getComputedStyle(document.querySelector('.react-flow__viewport')).transform),host=document.querySelector('.flow-root').getBoundingClientRect();
    return [...document.querySelectorAll(`[data-edge-ids~="${edge.id}"] path:not(.flow-edge-casing)`)].map(path=>{
      const [x,y]=path.getAttribute('d').trim().split(/[ML]/).filter(Boolean).at(-1).trim().split(/\s+/).map(Number);
      return {x:host.x+m.e+x*m.a,y:host.y+m.f+y*m.d};
    });
  });
  const onBorder=tip=>Math.min(Math.abs(tip.x-worker.x),Math.abs(tip.x-worker.x-worker.width),Math.abs(tip.y-worker.y),Math.abs(tip.y-worker.y-worker.height));
  expect(Math.min(...ends.map(onBorder)),'an arrow meets the part itself').toBeLessThan(2);
  await testInfo.attach('journey-01 — A part looked at, its arrows on its own border',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  // Out one level: the column reads the area the camera now looks at.
  await page.locator('[data-map-zoom]').filter({hasText:'−'}).click();await settle(map);
  await expect(map,'"−" has the column read the area it brings').toHaveAttribute('data-followed','execution');
  expect(errors).toEqual([]);
});
