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

// A drag pans the map wherever on the canvas it starts: from an arrow, a
// chip or a marker as from empty canvas, selecting no text and scrolling
// no page (final journeys, 2026-10-02: a drag from an arrow on etcd's home
// had selected the page's text, 144k characters, and scrolled the window
// 188 px, the camera unmoved). A card not yet open when the press comes
// does not open while the map is dragged.
test('a drag from an arrow, a chip or a marker pans the map, selecting no text and scrolling no page',async({page})=>{
  const errors=await open(page);
  const inSight=`(el=>{const c=document.querySelector('.flow-root').getBoundingClientRect(),r=el.getBoundingClientRect(),x=r.left+r.width/2,y=r.top+r.height/2;
    return r.width>0&&x>c.left+40&&x<c.right-140&&y>c.top+40&&y<c.bottom-100?{x,y}:null;})`;
  const starts={
    arrow:async()=>{for(const id of await page.evaluate(()=>window.__inv.hitIds())){const at=await page.evaluate(id=>window.__inv.hitPoint(id),id);if(at.x)return at;}return null;},
    chip:()=>page.evaluate(`[...document.querySelectorAll('.flow-root .flow-chip:not(.scene-bucket)')].map(${inSight}).find(Boolean)||null`),
    // Markers stand on a program's boxes, entered.
    marker:async()=>{await page.evaluate(()=>document.querySelector('[data-map]').sceneEnter('backend'));await settle(page);
      return page.evaluate(`[...document.querySelectorAll('.flow-root [data-marker]')].map(${inSight}).find(Boolean)||null`);},
  };
  for(const [kind,find] of Object.entries(starts)){
    const what=kind==='arrow'?'an arrow':`a ${kind}`;
    await page.keyboard.press('Escape');await page.mouse.move(2,2);await settle(page);
    const at=await find();
    expect(at,`${kind}: one in sight to drag from`).not.toBeNull();
    const before=await page.evaluate(()=>{
      window.__cardAtPress=null;
      document.addEventListener('pointerdown',()=>{window.__cardAtPress=window.__inv.cardOpen();},{capture:true,once:true});
      return {cam:window.__inv.camera(),scroll:scrollY};
    });
    await page.mouse.move(at.x,at.y,{steps:2});await page.mouse.down();
    await page.mouse.move(at.x+90,at.y+60,{steps:8});await page.mouse.up();await settle(page);
    const after=await page.evaluate(()=>({cam:window.__inv.camera(),scroll:scrollY,selected:String(getSelection()).length,card:window.__cardAtPress===false&&window.__inv.cardOpen()}));
    expect({moved:[Math.round(after.cam.x-before.cam.x),Math.round(after.cam.y-before.cam.y)],selected:after.selected,scrolled:after.scroll-before.scroll,cardOpened:after.card},
      `a drag from ${what}`).toEqual({moved:[90,60],selected:0,scrolled:0,cardOpened:false});
    // Back where it was, dragged from the same thing, now moved with the
    // map (a card open before the press stays where it opened).
    await page.keyboard.press('Escape');await page.mouse.move(at.x+90,at.y+60);
    await page.mouse.down();await page.mouse.move(at.x,at.y,{steps:8});await page.mouse.up();await settle(page);
    const back=await page.evaluate(()=>window.__inv.camera());
    expect([Math.round(back.x-before.cam.x),Math.round(back.y-before.cam.y)],`dragged back from ${what}`).toEqual([0,0]);
  }
  expect(errors).toEqual([]);
});

// A closed box's words move into its part in sight only where that part
// holds them: a box all but out of sight at the canvas's foot keeps its
// words where they stand (harness lints, 2026-10-03: casdoor's Outside
// buckets, a sliver of each in sight, had had their centred words pulled
// to their tops by the rule bringing a cut box's words up into sight).
test('a box all but out of sight at the canvas\'s foot keeps its words where they stand',async({page})=>{
  const errors=await open(page);
  const box=page.locator('.react-flow__node[data-id="front"]');
  const words=box.locator('.scene-words').first();
  await expect(words).toBeVisible();
  const canvas=await page.locator('.flow-root').boundingBox(),at=await box.boundingBox();
  // Dragged down from empty canvas until 12 pixels of it stand in sight.
  const spot=await page.evaluate(seed=>window.__inv.emptySpot(seed),7);
  const by=canvas.y+canvas.height-12-at.y;
  await page.mouse.move(spot.x,spot.y);await page.mouse.down();await page.mouse.move(spot.x,spot.y+by,{steps:10});await page.mouse.up();await settle(page);
  const moved=await box.boundingBox();
  expect(Math.round(canvas.y+canvas.height-moved.y),'12 pixels of the box in sight').toBe(12);
  expect(await words.evaluate(el=>el.style.transform||''),'its words stay where they stand').toBe('');
  expect(errors).toEqual([]);
});
