import {test,expect} from '@playwright/test';

async function settle(page){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await page.locator('[data-map]').evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[120],timeout:20_000}).toBeGreaterThanOrEqual(3);
}
// Where each input too small to draw stands, inside the canvas.
const hiddenInputs=page=>page.evaluate(()=>{
  const canvas=document.querySelector('.flow-root').getBoundingClientRect();
  return [...document.querySelectorAll('.react-flow__node')].filter(n=>/^command-/.test(n.dataset.id)&&n.style.visibility==='hidden').map(n=>{
    const r=n.getBoundingClientRect();return {id:n.dataset.id,x:r.left+r.width/2,y:r.top+r.height/2};
  }).filter(p=>p.x>canvas.left+4&&p.x<canvas.right-4&&p.y>canvas.top+4&&p.y<canvas.bottom-4);
});

// A click on an input reads its kind (owner, 2026-09-30: "я в колонке не
// вижу, что я тыкнул на канвасе инпут какой-то": Redis's acceptHandler,
// clicked where it stands in the closed Client I/O group of its Inputs, had
// read the handler's part). On the whole map the kind's row; in the
// collection, the place of an input its group is too small to draw.
test('a click on an input reads its kind on the whole map and in its collection',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?many-inputs');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await settle(page);
  await page.locator('[data-component-overview="backend-inputs"] [data-input-group-kind="request"]').click();
  await expect(map,'the whole map\'s kind row reads that kind').toHaveAttribute('data-read-kind','backend-inputs request');
  await page.evaluate(()=>{delete document.querySelector('[data-map]').dataset.readKind;});
  await page.mouse.move(2,2);
  await page.locator('[data-zoom-into="backend-inputs"]').first().click();await settle(page);
  const [input]=await hiddenInputs(page);
  expect(input,'an input its group is too small to draw').toBeTruthy();
  await page.mouse.move(input.x-6,input.y-6);await page.mouse.move(input.x,input.y,{steps:3});await page.mouse.click(input.x,input.y);
  await expect(map,`${input.id}'s place reads its kind, not its handler's part`).toHaveAttribute('data-read-kind','backend-inputs request');
  expect(errors).toEqual([]);
});
