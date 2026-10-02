import {test,expect} from '@playwright/test';
import {invariantKit} from './invariants.mjs';

// What a reader does on the scene canvas, on the fixture (two systems, five
// outside systems, its saved scene read by scene.go's rules): the canvas's
// invariants at every level are invariants.spec.mjs's; these are the
// clicks, controls and gestures that read and move. The scene canvas
// hit-tests the pointer itself, so its marks are clicked where they stand.
async function open(page,query='described'){
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`/?${query}`);
  await expect(page.locator('[data-fixture-ready]')).toHaveAttribute('data-fixture-ready','true');
  await expect(page.locator('[data-map]')).toHaveClass(/flow-enabled/);
  await expect(page.locator('[data-map]')).not.toHaveClass(/flow-initializing/);
  await page.evaluate(`window.__inv=(${invariantKit.toString()})()`);
  await settle(page);
  return errors;
}
async function settle(page){
  let previous='',stable=0;
  for(let i=0;i<60&&stable<3;i++){const now=await page.evaluate(()=>window.__inv.signature());stable=now===previous?stable+1:0;previous=now;await page.waitForTimeout(80);}
}
const level=page=>page.evaluate(()=>document.querySelector('[data-map]').dataset.sceneLevel||'');
const middle=async locator=>{const box=await locator.boundingBox();return {x:box.x+box.width/2,y:box.y+box.height/2};};
const reading=page=>page.locator('[data-reading-title]');

test('an arrow pointed at opens its card, and a click on it reads its connection',async({page})=>{
  const errors=await open(page);
  const ids=await page.evaluate(()=>window.__inv.hitIds());
  expect(ids.length,'arrows drawn on the whole map').toBeGreaterThan(0);
  let point=null;
  for(const id of ids){const at=await page.evaluate(id=>window.__inv.hitPoint(id),id);if(at.x){point=at;break;}}
  expect(point,'an arrow the pointer reaches').not.toBeNull();
  await page.mouse.move(point.x-12,point.y-12);await page.mouse.move(point.x,point.y,{steps:4});
  await expect(page.locator('.flow-floating-card').first()).toBeVisible();
  await page.mouse.click(point.x,point.y);
  await expect.poll(()=>page.evaluate(()=>document.querySelector('[data-map]').dataset.openedConnection||'')).not.toBe('');
  expect(errors).toEqual([]);
});

test('the − control steps out one level at a time: an area to its program, the program to the whole map',async({page})=>{
  const errors=await open(page);
  await page.evaluate(()=>document.querySelector('[data-map]').sceneEnter('execution'));await settle(page);
  expect(await level(page)).toBe('backend/execution');
  const out=page.locator('[data-map-zoom="0.8"]');
  await out.click();await settle(page);
  expect(await level(page)).toBe('backend');
  await out.click();await settle(page);
  expect(await level(page)).toBe('');
  expect(errors).toEqual([]);
});

test('one pinch crosses at most one level boundary, and a pause lets the next cross one more',async({page})=>{
  const errors=await open(page);
  const program=await middle(page.locator('.react-flow__node[data-id="backend"]'));
  await page.mouse.move(program.x,program.y);
  const pinch=async()=>{await page.keyboard.down('Control');for(let i=0;i<14;i++){await page.mouse.wheel(0,-60);await page.waitForTimeout(25);}await page.keyboard.up('Control');await settle(page);};
  await pinch();
  const first=await level(page);
  expect(first.split('/').filter(Boolean).length,`one pinch from the whole map reached "${first}"`).toBeLessThanOrEqual(1);
  await page.waitForTimeout(500);
  await pinch();
  const second=await level(page);
  expect(second.split('/').filter(Boolean).length-first.split('/').filter(Boolean).length,`the next pinch went from "${first}" to "${second}"`).toBeLessThanOrEqual(1);
  expect(errors).toEqual([]);
});

test('"Show whole map" keeps what is read and shows all of the map',async({page})=>{
  const errors=await open(page);
  await page.evaluate(()=>document.querySelector('[data-map]').sceneEnter('backend'));await settle(page);
  const area=await middle(page.locator('.react-flow__node[data-id="execution"]'));
  await page.mouse.click(area.x,area.y);
  await expect(reading(page)).toHaveText('Job execution');
  await page.locator('[data-map-fit]').click();await settle(page);
  expect(await level(page)).toBe('');
  await expect(reading(page)).toHaveText('Job execution');
  const fit=await page.evaluate(()=>window.__inv.wholeFit());
  expect(fit.filter(box=>!box.ok).map(box=>box.id),'every box of the whole map in the canvas').toEqual([]);
  expect(errors).toEqual([]);
});

test('a click on an input kind on the whole map reads that kind in its collection',async({page})=>{
  const errors=await open(page);
  const kind=page.locator('.react-flow__node[data-id="backend-inputs"] [data-input-group-kind]').first();
  await expect(kind).toBeVisible();
  const which=await kind.getAttribute('data-input-group-kind');
  await page.mouse.click(...Object.values(await middle(kind)));
  await expect.poll(()=>page.evaluate(()=>document.querySelector('[data-map]').dataset.readKind||'')).toMatch(new RegExp(`^backend-inputs .*${which==='entry'?'':which}`));
  expect(errors).toEqual([]);
});

test('an arrow keeps its screen width while the camera zooms',async({page})=>{
  const errors=await open(page);
  await page.evaluate(()=>document.querySelector('[data-map]').sceneEnter('backend'));await settle(page);
  const width=()=>page.evaluate(()=>{
    const path=document.querySelector('g[data-edge-id] path[data-edge-line]'),zoom=window.__inv.camera().zoom;
    return path?parseFloat(getComputedStyle(path).strokeWidth)*zoom:0;
  });
  const before=await width();
  expect(before,'an arrow drawn in the program').toBeGreaterThan(0);
  await page.locator('[data-map-zoom="1.25"]').click();await settle(page);
  expect(Math.abs(await width()-before),'its stroke on the screen').toBeLessThan(.6);
  expect(errors).toEqual([]);
});

// A layout worker failing before the first drawing leaves the page's static
// map and no pending canvas.
test('a layout worker failing before the first drawing leaves no pending map',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.addInitScript(()=>{
    const NativeWorker=Worker;
    window.workerFailure={workers:0,terminations:0};
    window.Worker=class extends NativeWorker{
      constructor(...args){super(...args);workerFailure.workers++;}
      postMessage(message,...args){
        if(message.cmd==='layout'&&!workerFailure.failed){
          workerFailure.failed=true;
          queueMicrotask(()=>this.dispatchEvent(new ErrorEvent('error',{cancelable:true,message:'Forced map worker failure'})));
          return;
        }
        return super.postMessage(message,...args);
      }
      terminate(){workerFailure.terminations++;return super.terminate();}
    };
    // The fixture calls the renderer directly: observe its rejection as the
    // report's caller does.
    let create;
    Object.defineProperty(window,'rmCreateFlow',{get:()=>create,set:value=>{
      create=(...args)=>value(...args).catch(error=>{workerFailure.initialError=error.message;return {layout:{edges:[]},capture:()=>null};});
    }});
  });
  await page.goto('/');
  await expect.poll(()=>page.evaluate(()=>workerFailure.initialError)).toBe('Forced map worker failure');
  await expect(page.locator('.flow-root')).toHaveCount(0);
  await expect(page.locator('.map-stage>svg')).not.toHaveCSS('display','none');
  await expect(page.locator('[data-map]')).not.toHaveClass(/flow-initializing/);
  await expect(page.locator('.flow-loading')).toHaveCount(0);
  expect(await page.evaluate(()=>({workers:workerFailure.workers,terminations:workerFailure.terminations}))).toEqual({workers:1,terminations:1});
  expect(errors).toEqual([]);
});

// A drag carries what is pointed at with it (harness table, 2026-10-02: a
// drag on etcd's whole map that left the canvas had dropped the arrows the
// program pointed at drew; a pan changes nothing drawn).
test('a drag that leaves the canvas keeps every arrow drawn, and dark, as the pointing drew it',async({page})=>{
  const errors=await open(page);
  const drawn=()=>page.evaluate(()=>[...document.querySelectorAll('g[data-edge-id]')].map(g=>`${g.dataset.edgeId}${g.hasAttribute('data-edge-dark')?' dark':''}`).sort());
  let pointed=null,lit=[];
  for(const id of await page.evaluate(()=>[...document.querySelectorAll('.react-flow__node[data-id]')].map(n=>n.dataset.id))){
    const node=page.locator(`.react-flow__node[data-id="${id}"]`);
    const at=await middle(node);await page.mouse.move(at.x,at.y,{steps:2});await page.waitForTimeout(200);
    lit=await drawn();if(lit.some(edge=>edge.endsWith(' dark'))){pointed=at;break;}
  }
  expect(pointed,'a box whose pointing darkens its arrows').not.toBeNull();
  const canvas=await page.locator('.flow-root').boundingBox();
  await page.mouse.down();await page.mouse.move(pointed.x,canvas.y+canvas.height+80,{steps:8});await page.mouse.up();await settle(page);
  expect(await drawn(),'the arrows as drawn before the drag').toEqual(lit);
  expect(errors).toEqual([]);
});
