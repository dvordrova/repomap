import {test,expect} from '@playwright/test';
import {records} from './two-systems-five-externals.mjs';

const rootIDs=records.filter(r=>r.branch==='component').map(r=>r.id).sort();
const collectionIDs=records.filter(r=>['communication','inputs'].includes(r.branch)).map(r=>r.id).sort();
const areaIDs=records.filter(r=>r.branch==='area').map(r=>r.id).sort();

test('pinch reveals and closes the complete hierarchy layer together',async({page},testInfo)=>{
  test.setTimeout(60000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const camera=()=>map.evaluate(map=>map.captureViewport());
  const geometry=()=>page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[n.dataset.id,n.style.transform,n.style.width,n.style.height]));
  const original=await geometry(),initial=await camera();
  const canvas=await page.locator('.flow-root').boundingBox();
  const frame=await page.locator('.react-flow__node[data-id="backend"]').boundingBox();
  await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),{...initial,fit:false,
    x:initial.x+canvas.x+canvas.width/2-frame.x-frame.width/2,y:initial.y+canvas.y+canvas.height/2-frame.y-frame.height/2});
  await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height/2);
  const states=[];
  const check=async()=>{
    const state=await camera();states.push(state);
    expect(state.openComponents.slice().sort()).toEqual(state.openComponents.length?rootIDs:[]);
    expect(state.communicationsOpen.slice().sort()).toEqual(state.openComponents.length?collectionIDs:[]);
    expect(state.detailAreas.slice().sort()).toEqual(state.detailAreas.length?areaIDs:[]);
    await expect(page.locator('[data-summary-area] .flow-overview-members'),'A group has no intermediate member-list representation').toHaveCount(0);
    return state;
  };
  const wheel=async delta=>{
    const before=(await camera()).zoom;
    await page.keyboard.down('Control');try{await page.mouse.wheel(0,delta);}finally{await page.keyboard.up('Control');}
    await expect.poll(async()=>(await camera()).zoom).not.toBe(before);
    return check();
  };
  await check();let captured=false;
  await testInfo.attach('journey-01 — All participants show their summaries',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  for(let step=0;step<48;step++){
    const state=await wheel(-4);
    if(state.openComponents.length&&!state.detailAreas.length&&!captured){
      captured=true;
      await testInfo.attach('journey-02 — Every participant opens its first level together',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
    }
    if(state.detailAreas.length)break;
  }
  expect(captured).toBe(true);
  expect((await camera()).detailAreas.slice().sort()).toEqual(areaIDs);
  await testInfo.attach('journey-03 — Every group reveals its objects together',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  const close=await camera();
  for(let step=0;step<48&&(await camera()).openComponents.length;step++)await wheel(4);
  expect((await camera()).openComponents).toEqual([]);
  await map.evaluate((map,viewport)=>map.restoreReadingState({viewport}),close);
  expect((await check()).detailAreas.slice().sort()).toEqual(areaIDs);
  expect(await geometry()).toEqual(original);
  await testInfo.attach('Layer states across pinch and return',{body:JSON.stringify(states),contentType:'application/json'});
  expect(errors).toEqual([]);
});
