import {test,expect} from '@playwright/test';

const camera=page=>page.locator('[data-map]').evaluate(map=>map.captureViewport());
const geometry=page=>page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(node=>({
  id:node.dataset.id,transform:node.style.transform,width:node.style.width,height:node.style.height,
})));

async function settled(page){
  let previous='',stable=0;
  await expect.poll(async()=>{
    const value=JSON.stringify(await camera(page));
    stable=value===previous?stable+1:0;previous=value;return stable;
  },{intervals:[100]}).toBeGreaterThanOrEqual(2);
}

test('native routes remain painted through wheel reveal and collapse',async({page},testInfo)=>{
  test.setTimeout(45_000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await settled(page);
  const world=await geometry(page);
  const ids=await map.evaluate(map=>({
    outer:map.visibleEdges.find(edge=>edge.from==='submission'&&edge.to==='post').id,
    between:map.visibleEdges.find(edge=>edge.from==='routes'&&edge.to==='queue').id,
  }));
  await expect(page.locator(`[data-edge-ids~="${ids.outer}"] path[marker-end]`)).toHaveCount(1);
  // Sampling only the settled image misses the frame where React Flow drops
  // every route while rebuilding handle bounds for unchanged native boxes.
  await page.evaluate(ids=>{
    window.routeTransition={frames:[],betweenSeen:false};
    const sample=()=>{
      const viewport=document.querySelector('[data-map]').captureViewport();
      const path=id=>document.querySelector(`[data-edge-ids~="${id}"] path[marker-end]`);
      const outer=path(ids.outer),between=path(ids.between);
      if(between)routeTransition.betweenSeen=true;
      routeTransition.frames.push({zoom:viewport.zoom,outer:!!outer,
        betweenRequired:routeTransition.betweenSeen&&viewport.openComponents.includes('backend'),
        between:between?.getAttribute('d')||null});
      routeTransition.frame=requestAnimationFrame(sample);
    };
    sample();
  },ids);
  try{
    await page.locator('[data-zoom-into="backend"]').click();
    await settled(page);
    const canvas=await page.locator('.flow-root').boundingBox();
    await page.mouse.move(canvas.x+canvas.width*.45,canvas.y+canvas.height*.5);
    async function wheel(delta){
      const before=(await camera(page)).zoom;
      await page.keyboard.down('Control');
      try{await page.mouse.wheel(0,delta);}finally{await page.keyboard.up('Control');}
      await expect.poll(async()=>(await camera(page)).zoom).not.toBe(before);
      await settled(page);
    }
    for(let step=0;step<16&&!(await camera(page)).detailAreas.includes('execution');step++)await wheel(-8);
    expect((await camera(page)).detailAreas).toContain('execution');
    await testInfo.attach('journey-01 — Revealed groups keep their native connections',{
      body:await page.locator('.map-workspace').screenshot(),contentType:'image/png',
    });
    for(let step=0;step<16&&(await camera(page)).detailAreas.length;step++)await wheel(8);
    const collapsed=await camera(page);
    expect(collapsed.detailAreas).toEqual([]);
    expect(collapsed.openComponents).toContain('backend');
    await expect(page.locator('[data-summary-area="requests"]')).toBeVisible();
    await expect(page.locator('[data-summary-area="execution"]')).toBeVisible();
    await expect(page.locator(`[data-edge-ids~="${ids.between}"] path[marker-end]`)).toHaveCount(1);
    await testInfo.attach('journey-02 — Collapsed groups retain the same connection',{
      body:await page.locator('.map-workspace').screenshot(),contentType:'image/png',
    });
    expect(await geometry(page)).toEqual(world);
  }finally{
    const frames=await page.evaluate(()=>{cancelAnimationFrame(routeTransition.frame);return routeTransition.frames;});
    await testInfo.attach('Route paint during every animation frame',{
      body:JSON.stringify(frames,null,2),contentType:'application/json',
    });
    expect(frames.filter(frame=>!frame.outer),'The outer arrow never disappears during a repaint').toEqual([]);
    expect(frames.filter(frame=>frame.betweenRequired&&!frame.between),'The visible group connection never disappears during a repaint').toEqual([]);
    const paths=new Set(frames.filter(frame=>frame.between).map(frame=>frame.between));
    expect(paths.size,'Revealing parts preserves the route between their existing groups').toBe(1);
    expect(errors).toEqual([]);
  }
});
