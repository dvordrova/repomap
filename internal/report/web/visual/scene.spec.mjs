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

test('an arrow card opens its exact declaration even without an upstream source URL',async({page})=>{
  const errors=await open(page,'symbols&reading-names&no-source-names&both-parts');
  await page.evaluate(()=>document.querySelector('[data-map]').sceneEnter('backend'));await settle(page);
  const id=await page.locator('g[data-edge-ends="requests execution"]').getAttribute('data-edge-id');
  const point=await page.evaluate(id=>window.__inv.hitPoint(id),id);
  expect(point.x,'the caller-to-handler arrow can be reached').toBeTruthy();
  await page.mouse.move(point.x,point.y);
  const name=page.locator('.flow-floating-card').getByRole('button',{name:'processJob',exact:true});
  await expect(name).toBeVisible();
  await name.press('Enter');
  await expect(reading(page)).toHaveText('Processing worker');
  await expect(page.locator('[data-map]')).toHaveAttribute('data-chosen-source','#worker.go-3');
  expect(errors).toEqual([]);
});

test('the magnifier has a zoom cursor on the real pointer surface at every level',async({page})=>{
  const errors=await open(page,'symbols&described');
  for(const [id,next] of [['backend','backend'],['execution','backend/execution'],['worker','backend/execution/worker']]){
    const point=await middle(page.locator(`[data-zoom-into="${id}"]`));
    await page.mouse.move(point.x,point.y);
    // A computed cursor on the transparent button would prove nothing:
    // the browser actually points at the pane beneath the magnifier.
    await expect.poll(()=>page.evaluate(p=>getComputedStyle(document.elementFromPoint(p.x,p.y)).cursor,point)).toBe('zoom-in');
    await page.mouse.click(point.x,point.y);await settle(page);
    expect(await level(page)).toBe(next);
  }
  const canvas=await page.locator('.flow-root').boundingBox();
  const empty={x:canvas.x+2,y:canvas.y+2};
  await page.mouse.move(empty.x,empty.y);
  await expect.poll(()=>page.evaluate(p=>getComputedStyle(document.elementFromPoint(p.x,p.y)).cursor,empty)).not.toBe('zoom-in');
  await page.mouse.move(5,5);
  await expect(page.locator('.scene-pointing-zoom')).toHaveCount(0);
  expect(errors).toEqual([]);
});

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
  const area=await middle(page.locator('[data-box-title="execution"]'));
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

// Trackpad gestures over named rows follow the canvas; a real overflowing
// list keeps vertical scrolling, while pinch still zooms the map.
test('named child rows preserve map pan and pinch without duplicate mouse entry',async({page})=>{
  const errors=await open(page,'described');
  const row=page.locator('[data-component-overview="backend"] [data-program-child]').first();
  expect(await row.locator('..').locator('..').evaluate(el=>el.scrollHeight<=el.clientHeight+1)).toBe(true);
  const before=await page.evaluate(()=>window.__inv.camera());
  let point=await middle(row);await page.mouse.move(point.x,point.y);await page.mouse.wheel(40,35);await settle(page);
  const panned=await page.evaluate(()=>window.__inv.camera());
  expect(Math.hypot(panned.x-before.x,panned.y-before.y)).toBeGreaterThan(10);
  expect(panned.zoom).toBe(before.zoom);expect(await level(page)).toBe('');
  // A pan moves the title and row by exactly the camera delta, including
  // when the title leaves the viewport. No duplicate screen-fixed titles.
  const title=page.locator('[data-box-title="backend"]'),titleBefore=await title.boundingBox();
  point=await middle(row);await page.mouse.move(point.x,point.y);await page.mouse.wheel(0,500);await settle(page);
  const moved=await page.evaluate(()=>window.__inv.camera()),titleAfter=await title.boundingBox();
  expect(Math.abs((titleAfter.y-titleBefore.y)-(moved.y-panned.y))).toBeLessThan(1);
  expect(await page.locator('[data-stuck-title]').count()).toBe(0);
  expect((await page.evaluate(()=>window.__inv.wordAnchor())).every(word=>word.ok)).toBe(true);
  await page.locator('[data-map-fit]').click();await settle(page);
  point=await middle(row);const zoomBefore=(await page.evaluate(()=>window.__inv.camera())).zoom;
  await page.mouse.move(point.x,point.y);await page.keyboard.down('Control');await page.mouse.wheel(0,-5);await page.keyboard.up('Control');await settle(page);
  expect((await page.evaluate(()=>window.__inv.camera())).zoom).toBeGreaterThan(zoomBefore);
  await page.locator('[data-map-fit]').click();await settle(page);
  point=await middle(row);const dragBefore=await page.evaluate(()=>window.__inv.camera());
  await page.mouse.move(point.x,point.y);await page.mouse.down();await page.mouse.move(point.x+50,point.y+20,{steps:8});await page.mouse.up();await settle(page);
  expect(await page.evaluate(()=>window.__inv.camera())).toEqual(dragBefore);
  // Releasing a dragged row must not enter it. A subsequent ordinary click
  // follows its original child exactly once.
  expect(await level(page)).toBe('');
  await row.click();await settle(page);expect(await level(page)).toBe('backend/requests');
  expect(errors).toEqual([]);
});

test('overflowing inventories scroll completely and still accept map pinch',async({page})=>{
  const errors=await open(page,'graph=synthetic-loose-40');
  const list=page.locator('[data-component-overview="system-component-t1"] .scene-program-inside');
  expect(await list.evaluate(el=>el.scrollHeight>el.clientHeight+1)).toBe(true);
  const before=await page.evaluate(()=>window.__inv.camera()),point=await middle(list);
  await page.mouse.move(point.x,point.y);await page.mouse.wheel(0,180);await settle(page);
  expect(await list.evaluate(el=>el.scrollTop)).toBeGreaterThan(0);
  expect(await page.evaluate(()=>window.__inv.camera())).toEqual(before);
  await page.keyboard.down('Control');await page.mouse.wheel(0,-5);await page.keyboard.up('Control');await settle(page);
  expect((await page.evaluate(()=>window.__inv.camera())).zoom).toBeGreaterThan(before.zoom);
  const pannedBefore=await page.evaluate(()=>window.__inv.camera()),nextPoint=await middle(list);
  await page.mouse.move(nextPoint.x,nextPoint.y);await page.mouse.wheel(100,0);await settle(page);
  const pannedAfter=await page.evaluate(()=>window.__inv.camera());
  expect(Math.abs(pannedAfter.x-pannedBefore.x)).toBeGreaterThan(20);
  const last=list.locator('[data-program-child]').last(),name=(await last.textContent()).replace(/\s+/g,' ').trim();
  await last.press('Enter');await settle(page);
  await expect(reading(page)).toHaveText(name);
  expect(errors).toEqual([]);
});

test('closed areas expose their complete saved children without unlabeled outlines',async({page})=>{
  const errors=await open(page,'symbols&described');
  await page.locator('[data-zoom-into="backend"]').press('Enter');await settle(page);
  const area=page.locator('[data-summary-area="execution"]');
  expect(await area.locator('[data-program-child]').evaluateAll(rows=>rows.map(row=>row.dataset.programChild))).toEqual(['queue','worker']);
  await expect(area).toContainText('Processing worker');await expect(area).toContainText('Job scheduling');
  await expect(page.locator('.scene-ghosts')).toHaveCount(0);
  const description=await area.locator('[data-box-title]').boundingBox(),list=await area.locator('.scene-program-inside').boundingBox();
  expect(list.y).toBeGreaterThanOrEqual(description.y+description.height-1);
  const zoom=page.locator('[data-zoom-into="execution"]'),small=await zoom.boundingBox();
  await page.locator('[data-map-zoom="1.25"]').click();await settle(page);
  const large=await zoom.boundingBox();expect(large.width).toBeGreaterThanOrEqual(small.width);expect(large.width).toBeLessThanOrEqual(32.1);
  expect((await zoom.locator('.flow-zoom-picture').boundingBox()).width).toBeGreaterThanOrEqual(16);
  await area.locator('[data-program-child="worker"]').click();await settle(page);
  await expect(reading(page)).toHaveText('Processing worker');
  expect(errors).toEqual([]);
});

test('marker names survive the pointer gap, keep every original input and read exact same-name inputs',async({page})=>{
  const errors=await open(page,'many-inputs&marker-inventory');
  await page.locator('[data-zoom-into="backend"]').press('Enter');await settle(page);
  const target=page.locator('[data-marker-end]').filter({visible:true});
  const id=await target.evaluateAll(es=>es.find(e=>e.dataset.markerEnd.split(' ').includes('command-39'))?.dataset.marker);
  expect(id).toBeTruthy();
  const handle=page.locator(`[data-marker="${id}"]`),aim=await middle(handle);
  const centered=await handle.evaluate(e=>{const a=e.getBoundingClientRect(),b=e.querySelector('svg').getBoundingClientRect();return {x:(a.left+a.right-b.left-b.right)/2,y:(a.top+a.bottom-b.top-b.bottom)/2};});
  expect(Math.abs(centered.x)).toBeLessThan(.5);expect(Math.abs(centered.y)).toBeLessThan(.5);
  await page.mouse.move(aim.x,aim.y);
  const card=page.locator(`[data-card="mark:${id}"]`);await expect(card).toBeVisible();
  const original=(await handle.getAttribute('data-marker-end')).split(' ');
  expect(original.length).toBeGreaterThan(30);
  expect(await card.locator('[data-marker-member]').evaluateAll(es=>es.map(e=>e.dataset.markerMember))).toEqual(original);
  const before=await page.evaluate(()=>({camera:document.querySelector('[data-map]').captureViewport(),title:document.querySelector('[data-reading-title]').textContent}));
  const box=await card.boundingBox(),inside={x:box.x+box.width/2,y:box.y+15};
  for(let i=1;i<=8;i++){await page.mouse.move(aim.x+(inside.x-aim.x)*i/8,aim.y+(inside.y-aim.y)*i/8);await page.waitForTimeout(60);await expect(card).toBeVisible();}
  await page.waitForTimeout(350);await expect(card).toBeVisible();
  expect(await page.evaluate(()=>({camera:document.querySelector('[data-map]').captureViewport(),title:document.querySelector('[data-reading-title]').textContent}))).toEqual(before);
  await card.locator('[data-marker-member="command-39"]').click();
  await expect(page.locator('[data-map]')).toHaveAttribute('data-read-i-d','command-39');
  await expect(reading(page)).toHaveText('Same command');
  await page.mouse.move(aim.x,aim.y);await expect(card).toBeVisible();
  await handle.press('Shift');await page.keyboard.press('Tab');
  expect(await page.evaluate(()=>document.activeElement?.dataset.markerMember)).toBe(original[0]);
  for(let i=0;i<original.indexOf('command-0');i++)await page.keyboard.press('Tab');
  expect(await page.evaluate(()=>document.activeElement?.dataset.markerMember)).toBe('command-0');
  const empty=await page.locator('.flow-root').boundingBox();
  await page.mouse.move(empty.x+2,empty.y+empty.height-2);await page.waitForTimeout(350);
  await expect(card).toBeVisible();expect(await page.evaluate(()=>document.activeElement?.dataset.markerMember)).toBe('command-0');
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-map]')).toHaveAttribute('data-read-i-d','command-0');
  await page.mouse.move(aim.x,aim.y);await expect(card).toBeVisible();
  const root=await page.locator('.flow-root').boundingBox();await page.mouse.move(root.x+2,root.y+root.height-2);await expect(card).not.toBeVisible();
  await handle.press('Shift');await expect(card).toBeVisible();await page.keyboard.press('Escape');await expect(card).not.toBeVisible();
  const edge=await page.locator('g[data-edge-ends="requests execution"]').getAttribute('data-edge-id'),arrow=await page.evaluate(id=>window.__inv.hitPoint(id),edge);
  expect(arrow.x).toBeTruthy();await page.mouse.move(arrow.x,arrow.y);await expect(page.locator('[data-card^="edge:"]')).toBeVisible();
  expect(errors).toEqual([]);
});

test('closed input kinds are native centered buttons with exact kind selection',async({page})=>{
  const errors=await open(page);
  const kinds=page.locator('.react-flow__node[data-id="backend-inputs"] button[data-input-group-kind]');
  const count=await kinds.count();expect(count).toBeGreaterThan(0);
  for(let i=0;i<count;i++){
    const button=kinds.nth(i),kind=await button.getAttribute('data-input-group-kind'),aim=await middle(button);
    const centered=await button.evaluate(e=>{const a=e.getBoundingClientRect(),b=e.querySelector('svg').getBoundingClientRect();return Math.abs((a.left+a.right-b.left-b.right)/2)+Math.abs((a.top+a.bottom-b.top-b.bottom)/2);});expect(centered).toBeLessThan(.5);
    await page.mouse.move(aim.x,aim.y);
    expect(await page.evaluate(p=>getComputedStyle(document.elementFromPoint(p.x,p.y)).cursor,aim)).toBe('pointer');
    await page.mouse.click(aim.x,aim.y);
    await expect(page.locator('[data-map]')).toHaveAttribute('data-read-kind',`backend-inputs ${kind==='entry'?'':kind}`);
  }
  expect(errors).toEqual([]);
});

test('a real pinch preserves the program world border while opening and closing its contents',async({page})=>{
  const errors=await open(page);
  const original=await page.evaluate(()=>document.querySelector('[data-map]').sceneState().scene.nodes.find(n=>n.id==='backend').rect);
  const p=await middle(page.locator('.react-flow__node[data-id="backend"]'));
  await page.mouse.move(p.x,p.y);await page.keyboard.down('Control');
  for(let i=0;i<20&&await level(page)==='';i++){await page.mouse.wheel(0,-60);await page.waitForTimeout(30);}
  await page.keyboard.up('Control');await settle(page);
  expect(await level(page)).toBe('backend');
  expect(await page.evaluate(()=>document.querySelector('[data-map]').sceneState().scene.nodes.find(n=>n.id==='backend').rect)).toEqual(original);
  await page.keyboard.down('Control');
  for(let i=0;i<24&&await level(page)!=='';i++){await page.mouse.wheel(0,60);await page.waitForTimeout(30);}
  await page.keyboard.up('Control');await settle(page);
  expect(await level(page)).toBe('');
  expect(await page.evaluate(()=>document.querySelector('[data-map]').sceneState().scene.nodes.find(n=>n.id==='backend').rect)).toEqual(original);
  expect(errors).toEqual([]);
});

// Check the real painted curve, not just its enclosing rectangle.
test('magnifiers remain inside the painted rounded program card at different map zooms',async({page})=>{
  const errors=await open(page);
  const card=page.locator('[data-component-overview="backend"]');
  let minimum=Infinity,maximum=0;
  for(let i=0;i<5;i++){
    const result=await card.evaluate(e=>{
      const b=e.getBoundingClientRect(),r=parseFloat(getComputedStyle(e).borderTopRightRadius)*b.width/e.offsetWidth;
      const icon=document.querySelector('[data-zoom-into="backend"]').getBoundingClientRect();
      const corners=[[icon.left,icon.top],[icon.right,icon.top],[icon.left,icon.bottom],[icon.right,icon.bottom]];
      return {radius:r,width:b.width,inside:corners.every(([x,y])=>x>=b.left&&x<=b.right&&y>=b.top&&y<=b.bottom&&
        (!(x>b.right-r&&y<b.top+r)||Math.hypot(x-(b.right-r),y-(b.top+r))<=r))};
    });
    expect(result.inside,JSON.stringify(result)).toBe(true);expect(result.radius).toBeGreaterThan(9);expect(result.radius).toBeLessThan(11);
    minimum=Math.min(minimum,result.width);maximum=Math.max(maximum,result.width);
    await page.locator('[data-map-zoom="1.25"]').click();await settle(page);
  }
  expect(maximum/minimum).toBeGreaterThan(2);expect(errors).toEqual([]);
});

test('complete role badges straddle area borders outside the clipped card contents',async({page})=>{
  const errors=await open(page,'symbols&described&role-badges');
  await page.locator('[data-zoom-into="backend"]').press('Enter');await settle(page);
  const roles=await page.locator('.scene-role-layer .flow-role-symbol').evaluateAll(es=>es.map(e=>{
    const b=e.getBoundingClientRect(),card=e.closest('.react-flow__node').querySelector('.scene-program-card').getBoundingClientRect();
    let clipped=false;
    for(let p=e.parentElement;p;p=p.parentElement){
      const s=getComputedStyle(p),r=p.getBoundingClientRect();
      if(['hidden','clip','auto','scroll'].includes(s.overflowX)&&(b.left<r.left-.5||b.right>r.right+.5))clipped=true;
      if(['hidden','clip','auto','scroll'].includes(s.overflowY)&&(b.top<r.top-.5||b.bottom>r.bottom+.5))clipped=true;
    }
    return {kind:e.className,clipped,straddles:b.top<card.top&&b.bottom>card.top};
  }));
  expect(roles).toHaveLength(2);expect(roles.map(r=>r.kind).sort()).toEqual(['flow-role-symbol flow-role-core','flow-role-symbol flow-role-triggers']);for(const role of roles){expect(role.clipped,role.kind).toBe(false);expect(role.straddles).toBe(true);}
  expect(errors).toEqual([]);
});
