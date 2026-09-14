import {test,expect} from '@playwright/test';

const geometry=page=>page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(node=>({
  id:node.dataset.id,transform:node.style.transform,width:node.style.width,height:node.style.height,
})));

test('a fully visible area reveals its already readable parts without another zoom',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  const original=await geometry(page),saved=await map.evaluate(map=>map.captureViewport());
  const scene=await page.evaluate(()=>{
    const frame=document.querySelector('.react-flow__node[data-id="editing"]');
    const position=new DOMMatrixReadOnly(frame.style.transform);
    const part=document.querySelector('.react-flow__node[data-id="editor"]>.flow-part');
    const scale=new DOMMatrixReadOnly(getComputedStyle(part).transform).a;
    const font=parseFloat(getComputedStyle(part.querySelector('strong')).fontSize)*scale;
    return {x:position.e,y:position.f,width:parseFloat(frame.style.width),height:parseFloat(frame.style.height),zoom:12.75/font};
  });
  const canvas=await page.locator('.flow-root').boundingBox();
  expect(scene.width*scene.zoom).toBeLessThan(canvas.width-80);
  expect(scene.height*scene.zoom).toBeLessThan(canvas.height-80);
  const partial={...saved,zoom:scene.zoom,x:-40-scene.x*scene.zoom,
    y:(canvas.height-scene.height*scene.zoom)/2-scene.y*scene.zoom,fit:false,
    componentsOpen:true,openComponents:['front'],detailAreas:[],communicationsOpen:[]};
  await map.evaluate((map,viewport)=>map.restoreReadingState({scope:'editing',viewport}),partial);
  const summary=page.locator('[data-summary-area="editing"]');
  const editor=page.locator('.react-flow__node[data-id="editor"]');
  await expect(summary).toBeVisible();await expect(editor).toBeHidden();
  await testInfo.attach('journey-01 — Partly visible area before the pan',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});

  const revision=await map.getAttribute('data-camera-revision');
  // This real pan brings the complete area into view without changing scale.
  await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height-30);
  await page.mouse.down();
  await page.mouse.move(canvas.x+canvas.width/2+100,canvas.y+canvas.height-30,{steps:5});
  await page.mouse.up();
  await expect(map).not.toHaveAttribute('data-camera-revision',revision);
  await testInfo.attach('journey-02 — Whole area at the same readable scale',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await expect(editor).toBeVisible();await expect(summary).toHaveCount(0);
  await expect(page.locator('.react-flow__node[data-id="submission"]')).toBeVisible();
  const internal=await map.evaluate(map=>map.visibleEdges.find(edge=>edge.from==='editor'&&edge.to==='submission').id);
  await expect(page.locator(`[data-edge-ids~="${internal}"] path[marker-end]`)).toHaveCount(1);
  const opened=await map.evaluate(map=>map.captureViewport());
  expect(opened.zoom).toBeCloseTo(partial.zoom,8);
  expect(await geometry(page)).toEqual(original);
  await map.evaluate((map,viewport)=>map.restoreReadingState({scope:'editing',viewport}),partial);
  await expect(editor).toBeHidden();await expect(summary).toBeVisible();
  await map.evaluate((map,viewport)=>map.restoreReadingState({scope:'editing',viewport}),opened);
  await expect(editor).toBeVisible();await expect(summary).toHaveCount(0);
  expect(await geometry(page)).toEqual(original);
  expect(errors).toEqual([]);
});
