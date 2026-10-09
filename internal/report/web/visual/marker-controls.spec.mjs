import {test,expect} from '@playwright/test';
import {writeFile} from 'node:fs/promises';

test.skip(!process.env.REPOMAP_REAL_RUN?.includes('sqlite'),'ordinary saved SQLite report required');
const middle=async locator=>{const b=await locator.boundingBox();return {x:Math.round(b.x+b.width/2),y:Math.round(b.y+b.height/2)};};
async function open(page){
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  page.on('console',m=>{if(['warning','error'].includes(m.type()))errors.push(m.text());});
  await page.goto('/real-report.html');
  await expect(page.locator('[data-map]')).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(page.locator('[data-map]')).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  return errors;
}
async function settle(page){
  expect(await page.evaluate(async()=>{
    let last='',stable=0;
    for(let i=0;i<60&&stable<3;i++){
      const next=document.querySelector('.react-flow__viewport')?.getAttribute('style');
      stable=last===next?stable+1:0;last=next;await new Promise(r=>setTimeout(r,80));
    }return stable;
  })).toBe(3);
}
const level=async page=>(await page.locator('[data-map]').getAttribute('data-scene-level'))||'';

test('real SQLite: all eight closed input buttons and six exact marker inputs open their original reading',async({page},info)=>{
  // The complete eight-kind/six-input journey includes saved rendering and
  // real report startup. The previous 240s driver budget expired 1.65s
  // into the last click after five complete input readings.
  test.setTimeout(300_000);const errors=await open(page),ledger=[];
  const kinds=await page.locator('.scene-inputs-card button[data-input-group-kind]').evaluateAll(es=>es.map(e=>({collection:e.closest('[data-component-overview]').dataset.componentOverview,kind:e.dataset.inputGroupKind,label:e.getAttribute('aria-label')})));
  expect(kinds).toHaveLength(8);
  for(const item of kinds){
    await page.getByRole('link',{name:'Home',exact:true}).click();await settle(page);
    const button=page.locator(`[data-component-overview="${item.collection}"] [data-input-group-kind="${item.kind}"]`),aim=await middle(button);
    const center=await button.evaluate(e=>{const a=e.getBoundingClientRect(),b=e.querySelector('svg').getBoundingClientRect();return Math.hypot((a.left+a.right-b.left-b.right)/2,(a.top+a.bottom-b.top-b.bottom)/2);});expect(center).toBeLessThan(.5);
    await page.mouse.move(aim.x,aim.y);expect(await page.evaluate(p=>getComputedStyle(document.elementFromPoint(p.x,p.y)).cursor,aim)).toBe('pointer');
    await page.mouse.click(aim.x,aim.y);await expect(page).toHaveURL(new RegExp(`#${item.collection}$`));
    await expect(page.locator('.map-card-kind')).toHaveText(`Inputs · ${item.label}`);
    ledger.push({...item,center,clicked:true});
  }
  for(const target of ['t1','t2','t3']){
    const program=`system-component-${target}`;
    const inputs=await page.locator(`[data-node][data-owner="${target}"]`).evaluateAll(es=>es.filter(e=>['stat_init','stat_push'].includes(e.dataset.title)).map(e=>({id:e.id,title:e.dataset.title,source:e.dataset.sourceText,written:e.dataset.written,path:e.dataset.handlerPath,handler:e.dataset.handler,handlerAt:`${e.dataset.handlerPath}:${e.dataset.handlerLine}`})));
    expect(inputs).toHaveLength(2);
    for(const input of inputs){
      await page.getByRole('link',{name:'Home',exact:true}).click();
      await page.locator(`[data-zoom-into="${program}"]`).press('Enter');await settle(page);
      await expect(page.locator('[data-map]')).toHaveAttribute('data-scene-level',program);
      const id=await page.locator('[data-marker-end]').evaluateAll((es,want)=>es.find(e=>e.dataset.markerEnd.split(' ').includes(want))?.dataset.marker,input.id);expect(id).toBeTruthy();
      const marker=page.locator(`[data-marker="${id}"]`);
      let aim=await middle(marker);const canvas=await page.locator('.flow-root').boundingBox();
      // Readable-scale entry may leave a marker offscreen. Bring the
      // original handle into view with an ordinary pan before hovering it.
      for(let attempt=0;attempt<8;attempt++){
        aim=await middle(marker);
        if(aim.x>canvas.x+40&&aim.x<canvas.x+canvas.width-40&&aim.y>canvas.y+40&&aim.y<canvas.y+canvas.height-40)break;
        await page.mouse.move(canvas.x+4,canvas.y+4);
        await page.mouse.wheel(2*(aim.x-(canvas.x+canvas.width*.4)),2*(aim.y-(canvas.y+canvas.height*.4)));await settle(page);
      }
      aim=await middle(marker);
      expect(aim.x).toBeGreaterThan(canvas.x+12);expect(aim.x).toBeLessThan(canvas.x+canvas.width-12);
      expect(aim.y).toBeGreaterThan(canvas.y+12);expect(aim.y).toBeLessThan(canvas.y+canvas.height-12);
      expect(await page.evaluate(p=>document.querySelector('[data-map]').sceneHitAt(p.x,p.y),aim)).toEqual({type:'marker',id});
      const center=await marker.evaluate(e=>{const a=e.getBoundingClientRect(),b=e.querySelector('svg').getBoundingClientRect();return Math.hypot((a.left+a.right-b.left-b.right)/2,(a.top+a.bottom-b.top-b.bottom)/2);});expect(center).toBeLessThan(.5);
      await page.mouse.move(aim.x,aim.y);const card=page.locator(`[data-card="mark:${id}"]`);await expect(card).toBeVisible();
      expect(await card.locator('[data-marker-member]').evaluateAll(es=>es.map(e=>e.dataset.markerMember))).toEqual((await marker.getAttribute('data-marker-end')).split(' '));
      const box=await card.boundingBox(),inside={x:box.x+box.width/2,y:box.y+12};
      for(let i=1;i<=8;i++){await page.mouse.move(aim.x+(inside.x-aim.x)*i/8,aim.y+(inside.y-aim.y)*i/8);await page.waitForTimeout(60);await expect(card).toBeVisible();}
      await page.waitForTimeout(350);await expect(card).toBeVisible();
      await card.locator(`[data-marker-member="${input.id}"]`).click();await expect(page).toHaveURL(new RegExp(`#${input.id}$`));
      const panel=page.getByRole('complementary',{name:'Selected node details',exact:true});
      await expect(panel).toContainText(input.title);await expect(panel).toContainText(input.handler);
      // This saved report has no source URLs; its reading shows the
      // original registration row and file, rather than an invented link.
      await expect(panel).toContainText(input.written);await expect(panel).toContainText(input.path);
      ledger.push({...input,program,center,clicked:true});
    }
  }
  expect(errors).toEqual([]);
  await writeFile(info.outputPath('marker-controls-ledger.json'),JSON.stringify({ledger,errors},null,2));
  await page.screenshot({path:info.outputPath('stat-input-reading.png')});
});

test('real SQLite: native pinch keeps the same world point and painted frame in all three programs',async({page},info)=>{
  test.setTimeout(240_000);const errors=await open(page),ledger=[];
  for(const program of ['system-component-t1','system-component-t2','system-component-t3']){
    await page.getByRole('link',{name:'Home',exact:true}).click();await settle(page);
    const node=page.locator(`.react-flow__node[data-id="${program}"]`),aim=await middle(node);
    const before=await page.evaluate(({program,aim})=>{
      const map=document.querySelector('[data-map]'),s=map.sceneState(),r=map.querySelector('.flow-root').getBoundingClientRect();
      return {rect:s.scene.nodes.find(n=>n.id===program).rect,camera:s.camera,world:{x:(aim.x-r.left-s.camera.x)/s.camera.zoom,y:(aim.y-r.top-s.camera.y)/s.camera.zoom}};
    },{program,aim});
    // The magnifier's corners lie inside the actual rounded top-right curve.
    const rounded=await node.locator('.scene-program-card').evaluate((e,program)=>{
      const b=e.getBoundingClientRect(),r=parseFloat(getComputedStyle(e).borderTopRightRadius)*b.width/e.offsetWidth,z=document.querySelector(`[data-zoom-into="${program}"]`).getBoundingClientRect();
      return [[z.left,z.top],[z.right,z.top],[z.left,z.bottom],[z.right,z.bottom]].every(([x,y])=>x>=b.left&&x<=b.right&&y>=b.top&&y<=b.bottom&&(!(x>b.right-r&&y<b.top+r)||Math.hypot(x-(b.right-r),y-(b.top+r))<=r));
    },program);expect(rounded).toBe(true);
    await page.mouse.move(aim.x,aim.y);await page.keyboard.down('Control');
    for(let i=0;i<40&&!await level(page);i++){await page.mouse.wheel(0,-60);await page.waitForTimeout(30);}
    await page.keyboard.up('Control');await settle(page);await expect(page.locator('[data-map]')).toHaveAttribute('data-scene-level',program);
    const after=await page.evaluate(({program,aim})=>{
      const map=document.querySelector('[data-map]'),s=map.sceneState(),r=map.querySelector('.flow-root').getBoundingClientRect(),b=map.querySelector(`.react-flow__node[data-id="${program}"]`).getBoundingClientRect();
      return {rect:s.scene.nodes.find(n=>n.id===program).rect,camera:s.camera,world:{x:(aim.x-r.left-s.camera.x)/s.camera.zoom,y:(aim.y-r.top-s.camera.y)/s.camera.zoom},bounds:{x:b.left-r.left,y:b.top-r.top,width:b.width,height:b.height}};
    },{program,aim});
    expect(after.rect).toEqual(before.rect);expect(Math.hypot(after.world.x-before.world.x,after.world.y-before.world.y)).toBeLessThan(1e-5);
    for(const [key,value] of Object.entries({x:before.rect.x*after.camera.zoom+after.camera.x,y:before.rect.y*after.camera.zoom+after.camera.y,width:before.rect.width*after.camera.zoom,height:before.rect.height*after.camera.zoom}))expect(Math.abs(after.bounds[key]-value),`${program} DOM ${key}`).toBeLessThan(1);
    await page.keyboard.down('Control');
    for(let i=0;i<48&&await level(page);i++){await page.mouse.wheel(0,60);await page.waitForTimeout(30);}
    await page.keyboard.up('Control');await settle(page);await expect.poll(()=>level(page)).toBe('');
    expect(await page.evaluate(program=>document.querySelector('[data-map]').sceneState().scene.nodes.find(n=>n.id===program).rect,program)).toEqual(before.rect);
    ledger.push({program,rounded,before,after,closed:true});
  }
  expect(errors).toEqual([]);await writeFile(info.outputPath('pinch-ledger.json'),JSON.stringify({ledger,errors},null,2));
  await page.screenshot({path:info.outputPath('program-borders.png')});
});
