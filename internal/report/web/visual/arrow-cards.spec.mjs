import {test,expect} from '@playwright/test';

// An arrow is its connection's handle at every level (owner, 2026-09-30:
// "the arrows can no longer be hovered": arrows into an area's border and
// from an entered program's ports had opened no card). At each level a
// sample of the drawn arrows, rested on by the pointer at the middle and by
// the head, opens its card.
async function settle(page){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await page.locator('[data-map]').evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[120],timeout:20_000}).toBeGreaterThanOrEqual(3);
}
async function cards(page,level,only=()=>true){
  const hits=(await page.evaluate(()=>{
    const canvas=document.querySelector('.flow-root').getBoundingClientRect();
    return [...document.querySelectorAll('[data-edge-hit]')].flatMap(path=>{
      const length=path.getTotalLength(),ctm=path.getScreenCTM(),ends=path.closest('g.flow-edge')?.dataset.edgeEnds||'';
      return [.5,.9].map(f=>{const p=path.getPointAtLength(length*f);return {id:path.dataset.edgeHit,ends,x:p.x*ctm.a+p.y*ctm.c+ctm.e,y:p.x*ctm.b+p.y*ctm.d+ctm.f};});
    }).filter(p=>p.x>canvas.left+20&&p.x<canvas.right-20&&p.y>canvas.top+20&&p.y<canvas.bottom-20&&document.elementFromPoint(p.x,p.y)?.closest?.('[data-edge-hit]'));
  })).filter(only);
  expect(hits.length,`${level}: arrows to rest on`).toBeGreaterThan(0);
  for(const hit of hits.slice(0,6)){
    await page.mouse.move(hit.x-12,hit.y-12);await page.mouse.move(hit.x,hit.y,{steps:4});
    await expect(page.locator('.flow-floating-card .flow-connection-calls').first(),`${level}: the card of ${hit.id} (${hit.ends})`).toBeVisible({timeout:2000});
    await page.keyboard.press('Escape');await page.mouse.move(2,2);await page.waitForTimeout(120);
  }
}

test('an arrow the pointer rests on opens its card at every level',async({page})=>{
  test.setTimeout(120_000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await expect(map).not.toHaveClass(/flow-initializing/);
  await settle(page);
  await cards(page,'whole map');
  await page.locator('[data-zoom-into="backend"]').first().click();await settle(page);
  await cards(page,'entered program');
  await page.locator('[data-zoom-into="requests"]').first().click();await settle(page);
  await cards(page,'entered area');
  // An outside dot chosen keeps its lines: each opens its card too.
  await map.evaluate(map=>map.showWholeMap());await settle(page);
  await page.locator('[data-zoom-into="backend"]').first().click();await settle(page);
  const dot=page.locator('[data-port="outside"] [data-port-end]').first(),id=await dot.getAttribute('data-port-end');
  await dot.click();await settle(page);
  await cards(page,'an outside dot\'s lines',hit=>hit.ends.split(' ').includes(id));
  const mark=page.locator('[data-port="inputs"] [data-port-end]').first(),kind=await mark.getAttribute('data-port-end');
  await mark.click();await settle(page);
  await cards(page,'an input kind\'s lines',hit=>hit.ends.split(' ').includes(kind));
  // Zoomed into a part, its arrows to its neighbours.
  await page.goto('/?symbols&both-parts');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await map.evaluate(map=>map.focusNode('worker'));await settle(page);
  await page.locator('[aria-label="Zoom into Processing worker"]').click();await settle(page);
  await expect(page.locator('.flow-part-deep')).toHaveCount(1);
  await cards(page,'a part zoomed into');
  expect(errors).toEqual([]);
});

// A dark arrow is whole and on top: pointed at a part, every arrow it
// darkens is one unbroken line drawn over every grey one, and nothing grey
// lies over any stretch of it (owner, 2026-09-30: redis's Core with Core data
// structures pointed at had its dark arrows cut by the grey ones and run
// under them in the gutters, "you can't tell where it comes from").
async function darkWhole(page,level){
  const found=await page.evaluate(()=>{
    const canvas=document.querySelector('.flow-root').getBoundingClientRect(),out=[];
    const groups=[...document.querySelectorAll('g.flow-edge')],dark=groups.filter(g=>g.classList.contains('flow-edge-active'));
    const lastGrey=groups.reduce((last,g,i)=>g.classList.contains('flow-edge-active')?last:i,-1);
    if(dark.length&&groups.indexOf(dark[0])<lastGrey)out.push('a grey arrow is drawn over a dark one');
    for(const g of dark){
      const path=g.querySelector('.flow-edge-hit'),id=path.dataset.edgeHit;
      if((path.getAttribute('d').match(/M/g)||[]).length>1)out.push(`${id} is broken`);
      const length=path.getTotalLength(),ctm=path.getScreenCTM(),step=4/ctm.a;
      for(let at=14/ctm.a;at<length-14/ctm.a;at+=step){
        const p=path.getPointAtLength(at),x=p.x*ctm.a+p.y*ctm.c+ctm.e,y=p.x*ctm.b+p.y*ctm.d+ctm.f;
        if(x<canvas.left+2||x>canvas.right-2||y<canvas.top+2||y>canvas.bottom-2)continue;
        const top=document.elementFromPoint(x,y),hit=top?.closest?.('[data-edge-hit]');
        if(hit&&!hit.closest('g.flow-edge').classList.contains('flow-edge-active')){out.push(`${id} runs under ${hit.dataset.edgeHit} at ${Math.round(x)},${Math.round(y)}`);break;}
      }
    }
    return {dark:dark.length,out};
  });
  expect(found.dark,`${level}: dark arrows`).toBeGreaterThan(0);
  expect(found.out,level).toEqual([]);
}
async function pointBusiest(page){
  const id=await page.evaluate(()=>{
    const count=new Map();
    for(const g of document.querySelectorAll('g.flow-edge'))for(const end of (g.dataset.edgeEnds||'').split(' '))count.set(end,(count.get(end)||0)+1);
    const parts=[...document.querySelectorAll('.react-flow__node')].filter(n=>n.querySelector(':scope>.flow-part')&&n.style.visibility!=='hidden').map(n=>n.dataset.id);
    return parts.sort((a,b)=>(count.get(b)||0)-(count.get(a)||0))[0];
  });
  const box=await page.locator(`.react-flow__node[data-id="${id}"] .flow-part>strong`).first().boundingBox();
  await page.mouse.move(box.x-8,box.y-8);await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:4});
  await page.waitForTimeout(400);
  return id;
}

test('a pointed part\'s dark arrows are whole and over the grey ones',async({page})=>{
  test.setTimeout(90_000);
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await settle(page);
  await page.locator('[data-zoom-into="backend"]').first().click();await settle(page);
  await page.locator('[data-zoom-into="requests"]').first().click();await settle(page);
  await darkWhole(page,`entered area, ${await pointBusiest(page)} pointed at`);
  await expect(page.locator('g.flow-edge.flow-edge-faint').first(),'the grey arrows fade').toBeAttached();
});
